package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// 临时待办保留策略。已完成列表会随时间单调增长，不设边界则 JSON 与列表渲染
// 都会持续变重；30 天覆盖「过段时间还想找回」的实际回看窗口，500 条兜住
// 高频使用场景（每天记十几条也在一个多月后才触顶）。
const (
	// TempTodoCompletedRetention 已完成条目保留时长，超期在下次读取时清理。
	TempTodoCompletedRetention = 30 * 24 * time.Hour
	// TempTodoCompletedMax 已完成条目硬上限，超出时按完成时间从旧到新裁剪。
	TempTodoCompletedMax = 500
	// TempTodoTextMaxRunes 单条正文长度上限（rune）。剪贴板偶尔会带上整页
	// 日志或 diff，超长文本落盘与列表渲染都会明显变卡；超出截断并记 Warn。
	TempTodoTextMaxRunes = 20000
)

// tempTodoFile data/temp_todos.json 的文件结构。顶层用对象而非裸数组：
// 后续若需加 schemaVersion 等元字段，不必再做一次结构迁移。
type tempTodoFile struct {
	Todos []model.TempTodo `json:"todos"`
}

// TempTodoService 临时待办服务（侧边栏「临时待办」面板的数据面）。
//
// 惰性加载 + 内存缓存：首次访问读盘，此后以内存为准，每次写操作整体落盘。
// 数据量受保留策略约束（已完成 ≤500 条），整体重写成本可忽略。
//
// 注入点（测试用）：now 时钟、readClipboard 剪贴板读取。两者都不允许在
// 生产链路里被轮询调用——剪贴板只在用户按快捷键时读一次。
type TempTodoService struct {
	configPath string

	mu     sync.Mutex
	todos  []model.TempTodo
	loaded bool

	now           func() time.Time
	readClipboard func() (string, error)
}

// NewTempTodoService 创建临时待办服务实例（configPath 通常为 data/temp_todos.json）。
func NewTempTodoService(configPath string) *TempTodoService {
	return &TempTodoService{
		configPath:    configPath,
		now:           time.Now,
		readClipboard: util.ReadClipboardText,
	}
}

// List 返回全部条目。顺序：待办在前（创建时间降序），已完成在后（完成时间降序）。
//
// 读取时顺带执行保留策略：超期或超量的已完成条目被清理并落盘。清理失败只记
// Warn 不阻断读取——用户要的是看到列表，落盘下次写操作还会重试。
func (s *TempTodoService) List() ([]model.TempTodo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return nil, fmt.Errorf("加载临时待办失败: %w", err)
	}
	if s.pruneCompletedLocked(s.now()) {
		if err := s.saveLocked(); err != nil {
			Logger().Warn("save pruned temp todos failed", "path", s.configPath, "err", err)
		}
	}

	out := make([]model.TempTodo, len(s.todos))
	copy(out, s.todos)
	sortTempTodos(out)
	return out, nil
}

// Add 新建一条待办（不会完成态）。空正文返回 E_TEMP_TODO_TEXT_REQUIRED。
func (s *TempTodoService) Add(text string, tags []string) (model.TempTodo, error) {
	text = normalizeTempTodoText(text)
	if text == "" {
		return model.TempTodo{}, model.NewAppError(model.ErrCodeTempTodoTextRequired, "待办内容不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return model.TempTodo{}, fmt.Errorf("加载临时待办失败: %w", err)
	}

	id, err := newTempTodoID()
	if err != nil {
		return model.TempTodo{}, err
	}
	todo := model.TempTodo{
		ID:        id,
		Text:      text,
		Tags:      normalizeTempTodoTags(tags),
		CreatedAt: s.now().UnixMilli(),
	}

	prev := len(s.todos)
	s.todos = append(s.todos, todo)
	if err := s.saveLocked(); err != nil {
		s.todos = s.todos[:prev] // 落盘失败回滚内存态，避免界面显示未持久化的条目
		return model.TempTodo{}, fmt.Errorf("保存临时待办失败: %w", err)
	}
	return todo, nil
}

// AddFromClipboard 读取系统剪贴板文本并直接落一条待办（快速记录唯一入口）。
//
// 剪贴板语义分流交给 util：空剪贴板与「有内容但无文本」返回不同哨兵，
// 这里翻译成不同错误码，前端据此给出可区分的提示。
func (s *TempTodoService) AddFromClipboard() (model.TempTodo, error) {
	text, err := s.readClipboard()
	if err != nil {
		switch {
		case errors.Is(err, util.ErrClipboardEmpty):
			return model.TempTodo{}, model.NewAppError(model.ErrCodeTempTodoClipboardEmpty, "剪贴板为空，未记录")
		case errors.Is(err, util.ErrClipboardNoText):
			return model.TempTodo{}, model.NewAppError(model.ErrCodeTempTodoClipboardNoText, "剪贴板不含文本（可能复制的是文件），未记录")
		default:
			Logger().Warn("read clipboard text failed", "err", err)
			return model.TempTodo{}, model.WrapAppError(model.ErrCodeTempTodoClipboardFailed, "读取剪贴板失败", err)
		}
	}
	return s.Add(text, nil)
}

// Update 修改正文与标签。不改 CreatedAt，也不改完成态。
func (s *TempTodoService) Update(id, text string, tags []string) error {
	text = normalizeTempTodoText(text)
	if text == "" {
		return model.NewAppError(model.ErrCodeTempTodoTextRequired, "待办内容不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return fmt.Errorf("加载临时待办失败: %w", err)
	}

	idx := s.indexOfLocked(id)
	if idx < 0 {
		return model.NewAppError(model.ErrCodeTempTodoNotFound, "待办条目不存在，可能已被删除")
	}

	prev := s.todos[idx]
	s.todos[idx].Text = text
	s.todos[idx].Tags = normalizeTempTodoTags(tags)
	if err := s.saveLocked(); err != nil {
		s.todos[idx] = prev
		return fmt.Errorf("保存临时待办失败: %w", err)
	}
	return nil
}

// SetCompleted 完成 / 取消完成。重复设置同一状态幂等返回成功。
func (s *TempTodoService) SetCompleted(id string, completed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return fmt.Errorf("加载临时待办失败: %w", err)
	}

	idx := s.indexOfLocked(id)
	if idx < 0 {
		return model.NewAppError(model.ErrCodeTempTodoNotFound, "待办条目不存在，可能已被删除")
	}

	if (s.todos[idx].CompletedAt != nil) == completed {
		return nil
	}

	prev := s.todos[idx].CompletedAt
	if completed {
		now := s.now().UnixMilli()
		s.todos[idx].CompletedAt = &now
	} else {
		s.todos[idx].CompletedAt = nil
	}
	if err := s.saveLocked(); err != nil {
		s.todos[idx].CompletedAt = prev
		return fmt.Errorf("保存临时待办失败: %w", err)
	}
	return nil
}

// Delete 物理删除一条待办（待办区与已完成区共用）。
func (s *TempTodoService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return fmt.Errorf("加载临时待办失败: %w", err)
	}

	idx := s.indexOfLocked(id)
	if idx < 0 {
		return model.NewAppError(model.ErrCodeTempTodoNotFound, "待办条目不存在，可能已被删除")
	}

	removed := s.todos[idx]
	s.todos = append(s.todos[:idx], s.todos[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		// 落盘失败须把条目插回原位：只回滚切片长度会让内存态与磁盘态分叉
		// （提示删除失败但条目已不在列表，下一次任意写操作把删除持久化）。
		// 重建切片而非原地 append，避免与删除后切片的底层数组别名相互覆盖。
		restored := make([]model.TempTodo, 0, len(s.todos)+1)
		restored = append(restored, s.todos[:idx]...)
		restored = append(restored, removed)
		restored = append(restored, s.todos[idx:]...)
		s.todos = restored
		return fmt.Errorf("保存临时待办失败: %w", err)
	}
	return nil
}

// loadLocked 惰性加载（调用方须持 s.mu）。文件缺失视为空列表；损坏时留底
// 备份后降级空列表，不阻塞面板打开（对齐 ChatService 索引加载范式）。
func (s *TempTodoService) loadLocked() error {
	if s.loaded {
		return nil
	}

	raw, err := os.ReadFile(s.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.todos = []model.TempTodo{}
			s.loaded = true
			return nil
		}
		return err
	}

	var file tempTodoFile
	if uerr := json.Unmarshal(raw, &file); uerr != nil {
		_, _ = util.BackupCorruptFile(s.configPath, raw)
		Logger().Warn("temp todo file corrupt, fallback to empty list",
			"path", s.configPath, "err", uerr)
		s.todos = []model.TempTodo{}
		s.loaded = true
		return nil
	}

	todos := make([]model.TempTodo, 0, len(file.Todos))
	for _, t := range file.Todos {
		// 无 ID 或空正文的脏条目无法被前端操作（勾选/删除都按 ID），直接剔除
		if t.ID == "" || strings.TrimSpace(t.Text) == "" {
			continue
		}
		t.Tags = normalizeTempTodoTags(t.Tags)
		todos = append(todos, t)
	}
	s.todos = todos
	s.loaded = true
	return nil
}

// saveLocked 整体原子落盘（调用方须持 s.mu）。
func (s *TempTodoService) saveLocked() error {
	file := tempTodoFile{Todos: s.todos}
	if file.Todos == nil {
		file.Todos = []model.TempTodo{}
	}
	return util.SaveJSON(s.configPath, file)
}

// indexOfLocked 按 ID 查找下标，未命中返回 -1（调用方须持 s.mu）。
func (s *TempTodoService) indexOfLocked(id string) int {
	for i := range s.todos {
		if s.todos[i].ID == id {
			return i
		}
	}
	return -1
}

// pruneCompletedLocked 执行保留策略并返回是否有条目被移除（调用方须持 s.mu）。
//
// 先按时间清理超期项，再按数量裁剪最旧的已完成项。待办区条目永不被清理。
func (s *TempTodoService) pruneCompletedLocked(now time.Time) bool {
	cutoff := now.Add(-TempTodoCompletedRetention).UnixMilli()

	kept := make([]model.TempTodo, 0, len(s.todos))
	changed := false
	for _, t := range s.todos {
		if t.CompletedAt != nil && *t.CompletedAt < cutoff {
			changed = true
			continue
		}
		kept = append(kept, t)
	}

	// 已完成下标按完成时间升序（最旧的在前），超出上限时从头部裁剪
	completedIdx := make([]int, 0, len(kept))
	for i := range kept {
		if kept[i].CompletedAt != nil {
			completedIdx = append(completedIdx, i)
		}
	}
	if excess := len(completedIdx) - TempTodoCompletedMax; excess > 0 {
		sort.Slice(completedIdx, func(i, j int) bool {
			return *kept[completedIdx[i]].CompletedAt < *kept[completedIdx[j]].CompletedAt
		})
		drop := make(map[int]struct{}, excess)
		for _, i := range completedIdx[:excess] {
			drop[i] = struct{}{}
		}
		trimmed := make([]model.TempTodo, 0, len(kept)-excess)
		for i, t := range kept {
			if _, ok := drop[i]; ok {
				continue
			}
			trimmed = append(trimmed, t)
		}
		kept = trimmed
		changed = true
	}

	if changed {
		s.todos = kept
	}
	return changed
}

// sortTempTodos 待办在前按创建时间降序，已完成在后按完成时间降序。
// 排序在前端也可做，但放在后端可保证「同一份数据在任何渲染路径下顺序一致」。
func sortTempTodos(todos []model.TempTodo) {
	sort.SliceStable(todos, func(i, j int) bool {
		a, b := todos[i], todos[j]
		aDone, bDone := a.CompletedAt != nil, b.CompletedAt != nil
		if aDone != bDone {
			return !aDone // 待办整体排在已完成之前
		}
		if aDone {
			return *a.CompletedAt > *b.CompletedAt
		}
		return a.CreatedAt > b.CreatedAt
	})
}

// normalizeTempTodoText 正文归一：换行统一为 \n、去除首尾空白、超长截断。
func normalizeTempTodoText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)

	runes := []rune(text)
	if len(runes) > TempTodoTextMaxRunes {
		Logger().Warn("temp todo text truncated", "runeLen", len(runes), "limit", TempTodoTextMaxRunes)
		text = string(runes[:TempTodoTextMaxRunes])
	}
	return text
}

// normalizeTempTodoTags 标签归一：去首尾空白、丢弃空项、按原顺序去重。
// 不排序：用户输入顺序往往带有分组意图，排序会打乱。
func normalizeTempTodoTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

// newTempTodoID 生成 16 位小写 hex 随机 ID；熵源不可用时退化为纳秒时间戳
// （宁可 ID 不够随机，也不能让用户记不下这条待办）。
func newTempTodoID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano()), nil
	}
	return hex.EncodeToString(buf), nil
}
