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

// 提示词字段上限。标题超长会撑坏列表行，正文超长落盘与渲染都会变卡；
// 后者与 TempTodoTextMaxRunes 取同一口径（剪贴板/日志类长文本的实际体验边界）。
const (
	// PromptTitleMaxRunes 标题长度上限（rune），超出截断并记 Warn。
	PromptTitleMaxRunes = 100
	// PromptContentMaxRunes 正文长度上限（rune），超出截断并记 Warn。
	PromptContentMaxRunes = 20000
)

// promptFile data/prompts.json 的文件结构。顶层用对象而非裸数组：后续加
// schemaVersion 等元字段时不必再做结构迁移（对齐 temp_todos.json）。
type promptFile struct {
	Prompts []model.Prompt `json:"prompts"`
}

// PromptService 提示词服务（侧边栏「提示词」面板的数据面）。
//
// 惰性加载 + 内存缓存：首次访问读盘，此后以内存为准，每次写操作整体落盘。
// 条目数量由用户手工维护（通常几十条），整体重写成本可忽略。
//
// 占位符（{{名称}}）的解析与替换全部在前端完成，本服务只存取原文，
// 不感知模板语义。
type PromptService struct {
	configPath string

	mu      sync.Mutex
	prompts []model.Prompt
	loaded  bool

	now func() time.Time
}

// NewPromptService 创建提示词服务实例（configPath 通常为 data/prompts.json）。
func NewPromptService(configPath string) *PromptService {
	return &PromptService{
		configPath: configPath,
		now:        time.Now,
	}
}

// List 返回全部条目，按创建时间降序。返回值是副本，调用方可自由排序。
func (s *PromptService) List() ([]model.Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return nil, fmt.Errorf("加载提示词失败: %w", err)
	}

	out := make([]model.Prompt, len(s.prompts))
	copy(out, s.prompts)
	// 创建时间降序；同毫秒时按 ID 稳定排序，保证跨端渲染顺序一致
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// Add 新建一条提示词。标题与正文都不能为空（分别返回对应错误码）。
func (s *PromptService) Add(title, content string) (model.Prompt, error) {
	title = normalizePromptTitle(title)
	if title == "" {
		return model.Prompt{}, model.NewAppError(model.ErrCodePromptTitleRequired, "提示词标题不能为空")
	}
	content = normalizePromptContent(content)
	if strings.TrimSpace(content) == "" {
		return model.Prompt{}, model.NewAppError(model.ErrCodePromptContentRequired, "提示词正文不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return model.Prompt{}, fmt.Errorf("加载提示词失败: %w", err)
	}

	id, err := newPromptID()
	if err != nil {
		return model.Prompt{}, err
	}
	now := s.now().UnixMilli()
	prompt := model.Prompt{
		ID:        id,
		Title:     title,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}

	prev := len(s.prompts)
	s.prompts = append(s.prompts, prompt)
	if err := s.saveLocked(); err != nil {
		s.prompts = s.prompts[:prev] // 落盘失败回滚内存态，避免界面显示未持久化的条目
		return model.Prompt{}, fmt.Errorf("保存提示词失败: %w", err)
	}
	return prompt, nil
}

// Update 修改标题与正文。不改 CreatedAt；成功后刷新 UpdatedAt。
func (s *PromptService) Update(id, title, content string) error {
	title = normalizePromptTitle(title)
	if title == "" {
		return model.NewAppError(model.ErrCodePromptTitleRequired, "提示词标题不能为空")
	}
	content = normalizePromptContent(content)
	if strings.TrimSpace(content) == "" {
		return model.NewAppError(model.ErrCodePromptContentRequired, "提示词正文不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return fmt.Errorf("加载提示词失败: %w", err)
	}

	idx := s.indexOfLocked(id)
	if idx < 0 {
		return model.NewAppError(model.ErrCodePromptNotFound, "提示词不存在，可能已被删除")
	}

	prev := s.prompts[idx]
	s.prompts[idx].Title = title
	s.prompts[idx].Content = content
	s.prompts[idx].UpdatedAt = s.now().UnixMilli()
	if err := s.saveLocked(); err != nil {
		s.prompts[idx] = prev
		return fmt.Errorf("保存提示词失败: %w", err)
	}
	return nil
}

// Delete 物理删除一条提示词。
func (s *PromptService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.loadLocked(); err != nil {
		return fmt.Errorf("加载提示词失败: %w", err)
	}

	idx := s.indexOfLocked(id)
	if idx < 0 {
		return model.NewAppError(model.ErrCodePromptNotFound, "提示词不存在，可能已被删除")
	}

	removed := s.prompts[idx]
	s.prompts = append(s.prompts[:idx], s.prompts[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		// 落盘失败须把条目插回原位：只回滚切片长度会让内存态与磁盘态分叉。
		// 重建切片而非原地 append，避免与删除后切片的底层数组别名相互覆盖。
		restored := make([]model.Prompt, 0, len(s.prompts)+1)
		restored = append(restored, s.prompts[:idx]...)
		restored = append(restored, removed)
		restored = append(restored, s.prompts[idx:]...)
		s.prompts = restored
		return fmt.Errorf("保存提示词失败: %w", err)
	}
	return nil
}

// loadLocked 惰性加载（调用方须持 s.mu）。文件缺失视为空列表；损坏时留底
// 备份后降级空列表，不阻塞面板打开（对齐 TempTodoService 的加载范式）。
func (s *PromptService) loadLocked() error {
	if s.loaded {
		return nil
	}

	raw, err := os.ReadFile(s.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.prompts = []model.Prompt{}
			s.loaded = true
			return nil
		}
		return err
	}

	var file promptFile
	if uerr := json.Unmarshal(raw, &file); uerr != nil {
		_, _ = util.BackupCorruptFile(s.configPath, raw)
		Logger().Warn("prompt file corrupt, fallback to empty list",
			"path", s.configPath, "err", uerr)
		s.prompts = []model.Prompt{}
		s.loaded = true
		return nil
	}

	prompts := make([]model.Prompt, 0, len(file.Prompts))
	for i, p := range file.Prompts {
		// 无 ID 或空标题/空正文的脏条目无法被前端操作（编辑/删除都按 ID），直接剔除。
		// 记录 Warn 便于排障：字段齐全但内容为空通常来自手改 data/prompts.json。
		if reason := dirtyPromptReason(p); reason != "" {
			Logger().Warn("prompt entry skipped", "index", i, "id", p.ID, "reason", reason)
			continue
		}
		// 老数据缺 UpdatedAt 时回填 CreatedAt，避免前端显示 1970
		if p.UpdatedAt <= 0 {
			p.UpdatedAt = p.CreatedAt
		}
		prompts = append(prompts, p)
	}
	s.prompts = prompts
	s.loaded = true
	return nil
}

// saveLocked 整体原子落盘（调用方须持 s.mu）。
func (s *PromptService) saveLocked() error {
	file := promptFile{Prompts: s.prompts}
	if file.Prompts == nil {
		file.Prompts = []model.Prompt{}
	}
	return util.SaveJSON(s.configPath, file)
}

// indexOfLocked 按 ID 查找下标，未命中返回 -1（调用方须持 s.mu）。
func (s *PromptService) indexOfLocked(id string) int {
	for i := range s.prompts {
		if s.prompts[i].ID == id {
			return i
		}
	}
	return -1
}

// normalizePromptTitle 标题归一：去首尾空白、超长截断。
func normalizePromptTitle(title string) string {
	title = strings.TrimSpace(title)
	runes := []rune(title)
	if len(runes) > PromptTitleMaxRunes {
		Logger().Warn("prompt title truncated", "runeLen", len(runes), "limit", PromptTitleMaxRunes)
		title = string(runes[:PromptTitleMaxRunes])
	}
	return title
}

// dirtyPromptReason 判定加载期脏条目，返回原因（空串 = 干净）。
func dirtyPromptReason(p model.Prompt) string {
	switch {
	case p.ID == "":
		return "missing id"
	case strings.TrimSpace(p.Title) == "":
		return "empty title"
	case strings.TrimSpace(p.Content) == "":
		return "empty content"
	default:
		return ""
	}
}

// normalizePromptContent 正文归一：换行统一为 \n、裁掉首尾空行与行尾空白、超长截断。
//
// 刻意不用 strings.TrimSpace：提示词/代码模板的首行缩进有语义（用户贴进来的
// 代码块、嵌套列表），整体 TrimSpace 会把首行缩进吃掉。空正文判定改由调用方用
// TrimSpace 兜底（见 Add / Update）。
func normalizePromptContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = strings.TrimLeft(content, "\n")
	content = strings.TrimRight(content, " \t\n")

	runes := []rune(content)
	if len(runes) > PromptContentMaxRunes {
		Logger().Warn("prompt content truncated", "runeLen", len(runes), "limit", PromptContentMaxRunes)
		content = string(runes[:PromptContentMaxRunes])
	}
	return content
}

// newPromptID 生成 16 位小写 hex 随机 ID；熵源不可用时退化为纳秒时间戳。
func newPromptID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("p%d", time.Now().UnixNano()), nil
	}
	return hex.EncodeToString(buf), nil
}
