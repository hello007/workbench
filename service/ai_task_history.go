package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// AiTaskHistoryService AI 任务历史归档服务：元数据持久化 + 输出文件零拷贝接管 + 查询/清理。
// 存储方案 B：data/ai_task_history.json 存元数据（轻量快速加载），
// data/ai_task_history/<id>.txt 存完整输出（按需懒加载）。
// 归档时输出文件经 os.Rename 从 data/ai_task_output/ 零拷贝移入，无大对象过 IPC。
type AiTaskHistoryService struct {
	mu      sync.Mutex
	dataDir string // data 目录绝对路径（元数据 JSON 与输出文件目录的父目录）
}

// NewAiTaskHistoryService 创建历史归档服务。dataDir 为 data 目录绝对路径。
func NewAiTaskHistoryService(dataDir string) *AiTaskHistoryService {
	return &AiTaskHistoryService{dataDir: dataDir}
}

// historyMetaPath 元数据 JSON 文件路径（data/ai_task_history.json）。
func (h *AiTaskHistoryService) historyMetaPath() string {
	return filepath.Join(h.dataDir, aiTaskHistoryFileName)
}

// historyDir 归档输出文件目录（data/ai_task_history/）。
func (h *AiTaskHistoryService) historyDir() string {
	return filepath.Join(h.dataDir, aiTaskHistoryDirName)
}

// historyFilePath 单条归档输出文件路径（data/ai_task_history/<id>.txt）。
func (h *AiTaskHistoryService) historyFilePath(id string) string {
	return filepath.Join(h.historyDir(), id+".txt")
}

// loadAll 加载全部历史元数据。文件不存在返回空切片（首条归档时创建）。
func (h *AiTaskHistoryService) loadAll() ([]*model.AiTaskHistory, error) {
	if !util.FileExists(h.historyMetaPath()) {
		return []*model.AiTaskHistory{}, nil
	}
	var list []*model.AiTaskHistory
	if err := util.LoadJSON(h.historyMetaPath(), &list); err != nil {
		return nil, fmt.Errorf("加载历史元数据失败: %w", err)
	}
	return list, nil
}

// saveAll 全量重写元数据 JSON。量小（元数据不含输出全文）可接受；
// 超 aiTaskHistoryMaxCount 条由 enforceRetention 先清理再写。
func (h *AiTaskHistoryService) saveAll(list []*model.AiTaskHistory) error {
	return util.SaveJSON(h.historyMetaPath(), list)
}

// Archive 归档一条任务：输出文件 os.Rename 零拷贝移入历史目录，元数据追加落盘。
// 调用方为 AiFunctionService.archiveTask（pumpOutput 末尾，emit done 前）。
// 返回归档后的元数据；输出文件不存在（创建失败的任务）时 OutputFile 留空，仍归档元数据。
func (h *AiTaskHistoryService) Archive(entry *model.AiTaskHistory, srcOutputPath string) (*model.AiTaskHistory, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 确保归档目录存在
	if err := os.MkdirAll(h.historyDir(), 0o755); err != nil {
		return nil, fmt.Errorf("创建历史目录失败: %w", err)
	}

	// 输出文件零拷贝接管：os.Rename 从运行期目录移入历史目录。
	// 同卷原子操作；跨卷退化为复制+删除（Windows 单 data 目录同卷，一般走原子路径）。
	// 源文件不存在（创建失败的任务）时跳过，OutputFile 留空，仅归档元数据。
	dstPath := h.historyFilePath(entry.ID)
	if srcOutputPath != "" && util.FileExists(srcOutputPath) {
		if err := os.Rename(srcOutputPath, dstPath); err != nil {
			// Rename 失败（跨卷或占用）退化为复制+删除，保证归档不丢输出
			if copyErr := copyFile(srcOutputPath, dstPath); copyErr == nil {
				_ = os.Remove(srcOutputPath)
			}
		}
		// 更新元数据指向归档后的相对路径
		rel, err := filepath.Rel(h.dataDir, dstPath)
		if err == nil {
			entry.OutputFile = filepath.ToSlash(rel)
		} else {
			entry.OutputFile = aiTaskHistoryDirName + "/" + entry.ID + ".txt"
		}
	}

	list, err := h.loadAll()
	if err != nil {
		return nil, err
	}
	list = append(list, entry)
	// 保留策略：超上限先清理最旧，再写
	list = h.enforceRetentionLocked(list)
	if err := h.saveAll(list); err != nil {
		return nil, fmt.Errorf("写入历史元数据失败: %w", err)
	}
	return entry, nil
}

// copyFile 复制文件（os.Rename 跨卷失败时的退化路径）。覆盖目标文件。
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// enforceRetentionLocked 保留策略：超 aiTaskHistoryMaxCount 条删最旧，超 aiTaskHistoryMaxDays 条删过期。
// 返回清理后的列表；同步删除对应输出文件避免孤儿。调用方须持锁。
func (h *AiTaskHistoryService) enforceRetentionLocked(list []*model.AiTaskHistory) []*model.AiTaskHistory {
	if len(list) == 0 {
		return list
	}
	now := time.Now()
	cutoff := now.AddDate(0, 0, -aiTaskHistoryMaxDays).UnixMilli()

	// 按完成时间升序排序，便于从最旧开始清理
	sort.Slice(list, func(i, j int) bool {
		return list[i].FinishedAt < list[j].FinishedAt
	})

	kept := make([]*model.AiTaskHistory, 0, len(list))
	removed := []*model.AiTaskHistory{}
	for _, e := range list {
		expired := e.FinishedAt > 0 && e.FinishedAt < cutoff
		if expired {
			removed = append(removed, e)
			continue
		}
		kept = append(kept, e)
	}
	// 条数上限：保留最近 N 条（kept 已按时间升序，超出的从头部即最旧删）
	if len(kept) > aiTaskHistoryMaxCount {
		overflow := len(kept) - aiTaskHistoryMaxCount
		removed = append(removed, kept[:overflow]...)
		kept = kept[overflow:]
	}
	// 删除被清理条目的输出文件
	for _, e := range removed {
		if e.OutputFile != "" {
			_ = os.Remove(filepath.Join(h.dataDir, filepath.FromSlash(e.OutputFile)))
		}
	}
	return kept
}

// List 按筛选条件查询历史，按完成时间降序返回（最新在前）。filter 为 nil 返回全部。
func (h *AiTaskHistoryService) List(filter *model.AiTaskHistoryFilter) ([]*model.AiTaskHistory, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list, err := h.loadAll()
	if err != nil {
		return nil, err
	}
	result := make([]*model.AiTaskHistory, 0, len(list))
	for _, e := range list {
		if filter != nil {
			if filter.FunctionID != "" && e.FunctionID != filter.FunctionID {
				continue
			}
			if filter.Status != "" && e.Status != filter.Status {
				continue
			}
			if filter.From > 0 && e.StartedAt < filter.From {
				continue
			}
			if filter.To > 0 && e.StartedAt > filter.To {
				continue
			}
		}
		result = append(result, e)
	}
	// 降序：最新在前
	sort.Slice(result, func(i, j int) bool {
		return result[i].FinishedAt > result[j].FinishedAt
	})
	return result, nil
}

// GetOutput 读取单条历史的归档输出文件全文（详情查看输出走此路径）。
// 文件不存在返回错误，供前端判空降级。
func (h *AiTaskHistoryService) GetOutput(id string) (string, error) {
	return readOutputFile(h.historyFilePath(id))
}

// Delete 删除单条历史：删元数据 + 归档输出文件。返回是否删除成功。
func (h *AiTaskHistoryService) Delete(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	list, err := h.loadAll()
	if err != nil {
		return false
	}
	found := false
	kept := make([]*model.AiTaskHistory, 0, len(list))
	for _, e := range list {
		if e.ID == id {
			found = true
			if e.OutputFile != "" {
				_ = os.Remove(filepath.Join(h.dataDir, filepath.FromSlash(e.OutputFile)))
			}
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return false
	}
	_ = h.saveAll(kept)
	return true
}

// Clear 按条件批量清理。criteria 字段为空表示不限；OlderThanDays 与 KeepRecent 任一生效即可。
// 返回清理条数。
func (h *AiTaskHistoryService) Clear(criteria *model.AiTaskHistoryClearCriteria) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list, err := h.loadAll()
	if err != nil {
		return 0, err
	}
	if criteria == nil {
		criteria = &model.AiTaskHistoryClearCriteria{}
	}
	now := time.Now()
	var cutoffMs int64
	if criteria.OlderThanDays > 0 {
		cutoffMs = now.AddDate(0, 0, -criteria.OlderThanDays).UnixMilli()
	}
	kept := make([]*model.AiTaskHistory, 0, len(list))
	removed := 0
	for _, e := range list {
		if criteria.FunctionID != "" && e.FunctionID != criteria.FunctionID {
			kept = append(kept, e)
			continue
		}
		older := criteria.OlderThanDays > 0 && e.FinishedAt > 0 && e.FinishedAt < cutoffMs
		if older {
			if e.OutputFile != "" {
				_ = os.Remove(filepath.Join(h.dataDir, filepath.FromSlash(e.OutputFile)))
			}
			removed++
			continue
		}
		kept = append(kept, e)
	}
	// KeepRecent：仅保留最近 N 条（按完成时间），清理其余
	if criteria.KeepRecent > 0 && len(kept) > criteria.KeepRecent {
		sort.Slice(kept, func(i, j int) bool {
			return kept[i].FinishedAt < kept[j].FinishedAt
		})
		overflow := kept[:len(kept)-criteria.KeepRecent]
		for _, e := range overflow {
			if e.OutputFile != "" {
				_ = os.Remove(filepath.Join(h.dataDir, filepath.FromSlash(e.OutputFile)))
			}
			removed++
		}
		kept = kept[len(kept)-criteria.KeepRecent:]
	}
	if removed > 0 {
		_ = h.saveAll(kept)
	}
	return removed, nil
}

// CleanStaleOutputFiles 定时清理兜底：清理未归档的运行期输出文件（data/ai_task_output/）。
// 归档接管已 os.Rename 移走不留残，此路径只清理异常残留（进程崩溃、归档失败等）。
// 删除 mtime 早于 ttl 的文件。返回清理条数。
func (h *AiTaskHistoryService) CleanStaleOutputFiles(ttl time.Duration) int {
	outputDir := filepath.Join(h.dataDir, aiTaskOutputDirName)
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-ttl)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(outputDir, entry.Name())); err == nil {
				removed++
			}
		}
	}
	return removed
}

// buildPromptPreview 历史元数据的 prompt 预览：截断到 maxRunes 字符，超长加省略号。
// 避免长 prompt 全量进元数据 JSON 拖慢列表加载。
func buildPromptPreview(prompt string, maxRunes int) string {
	runes := []rune(prompt)
	if len(runes) <= maxRunes {
		return prompt
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}
