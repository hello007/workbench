package service

import (
	"fmt"
	"strings"

	"workbench/model"
)

// diff 截断阈值（research 1 建议，claude 200K tokens 上下文窗口保护）。
// 超任一阈值即截断，避免大 diff 撑爆 claude 上下文导致审查/生成质量下降。
// 阈值为包级常量，MVP 硬编码；后续随 schema v2 入配置文件可调时再外提。
const (
	maxDiffBytes = 200 * 1024 // 200KB：字节硬上限（约 50K tokens，留半窗口给 prompt/历史 few-shot）
	maxDiffFiles = 20         // 文件数上限：超此截断剩余文件，避免单次审查范围过宽
	maxDiffLines = 50 * 1000  // 50K 行：行数上限，行边界完整截断不破坏 hunk
)

// TruncateDiff 按行数上限截断 diff 文本，返回截断后文本、是否截断、丢弃行数。
// 纯函数便于单测。按行截断保行边界完整（不截断半行 hunk）。
// maxLines <= 0 时不截断（调用方禁用）。
func TruncateDiff(text string, maxLines int) (result string, truncated bool, droppedLines int) {
	if maxLines <= 0 {
		return text, false, 0
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text, false, 0
	}
	droppedLines = len(lines) - maxLines
	return strings.Join(lines[:maxLines], "\n"), true, droppedLines
}

// AggregateStagedDiff 聚合暂存区所有文件 diff 为单段文本，含截断保护。
//
// 链路：GetLocalChanges 筛 Staged + 逐文件 GetStagedDiff（git diff --cached），
// 按 maxDiffFiles/maxDiffLines/maxDiffBytes 三阈值截断，截断时文末拼提示
// 「[已截断：剩余 N 文件 M 行未纳入]」供模型感知上下文不完整。
//
// 暂存区为空返回 AppError{E_GIT_NO_STAGED_CHANGES}，前端按 code 禁用按钮 + 提示。
// 文本格式：每文件 diff 前拼文件头「=== <path> ===」便于模型定位文件边界。
//
// 用于 AI 提交信息生成：diff 经 RunAiFunction params 注入 BuildStagePrompt 的
// {{diff}} 占位符。截断保护避免大 diff 超 claude 上下文窗口。
func (s *GitService) AggregateStagedDiff(repoPath string) (string, error) {
	return s.aggregateDiff(repoPath, true)
}

// AggregateUncommittedDiff 聚合未提交全量变更（staged + unstaged）diff 为单段文本，含截断保护。
//
// 与 AggregateStagedDiff 同构但含未暂存：不筛 Staged，逐文件 GetDiff（git diff HEAD，
// 已跟踪文件对比 HEAD 与工作区含暂存+未暂存；未跟踪文件 --no-index 展示新增全文）。
// 按 maxDiffFiles/maxDiffLines/maxDiffBytes 三阈值截断，截断时文末拼提示
// 「[已截断：剩余 N 文件 M 行未纳入]」供模型感知上下文不完整。
//
// 无任何本地变更返回 AppError{E_GIT_NO_STAGED_CHANGES}（复用 PR1 错误码，前端按 code
// 走 handleGitError warning 提示「无本地变更可审查」）。文本格式与 AggregateStagedDiff
// 一致：每文件 diff 前拼「=== <path> ===」头。
//
// 用于 AI 代码审查（审未提交变更）：diff 经 RunAiFunction('code-review', { diff }) 注入
// form PromptTemplate 的 {{diff}} 占位符。截断保护避免大 diff 超 claude 上下文窗口。
func (s *GitService) AggregateUncommittedDiff(repoPath string) (string, error) {
	return s.aggregateDiff(repoPath, false)
}

// aggregateDiff 是 AggregateStagedDiff / AggregateUncommittedDiff 的共用实现。
//
// stagedOnly=true 走暂存区聚合（筛 Staged + GetStagedDiff），用于提交信息生成；
// stagedOnly=false 走未提交全量聚合（不筛 Staged + GetDiff），用于代码审查。
// 两者共享截断阈值与文件头格式，仅数据源与筛选不同。空变更返回 AppError。
func (s *GitService) aggregateDiff(repoPath string, stagedOnly bool) (string, error) {
	changes, err := s.GetLocalChanges(repoPath)
	if err != nil {
		return "", err
	}
	files := make([]string, 0, len(changes))
	for _, c := range changes {
		if stagedOnly && !c.Staged {
			continue
		}
		files = append(files, c.Path)
	}
	if len(files) == 0 {
		msg := "无暂存文件，请先 git add 要提交的变更"
		if !stagedOnly {
			msg = "无本地变更可审查"
		}
		return "", model.NewAppError(model.ErrCodeGitNoStagedChanges, msg)
	}

	var sb strings.Builder
	totalBytes := 0
	totalLines := 0
	includedFiles := 0
	droppedFiles := 0
	droppedLines := 0
	truncated := false

	for _, file := range files {
		if truncated {
			// 前序文件已触发截断，后续文件全部丢弃计数
			droppedFiles++
			continue
		}
		// 文件数阈值：已纳入文件数达上限，当前及后续截断
		if includedFiles >= maxDiffFiles {
			droppedFiles++
			truncated = true
			continue
		}
		var diff string
		if stagedOnly {
			diff, err = s.GetStagedDiff(repoPath, file)
		} else {
			diff, err = s.GetDiff(repoPath, file)
		}
		if err != nil {
			return "", err
		}
		if diff == "" {
			continue // 文件无 diff（如改动改回 HEAD 一致），跳过不计数
		}
		fileBlock := fmt.Sprintf("=== %s ===\n%s\n", file, diff)
		blockLines := strings.Count(fileBlock, "\n")

		// 行数阈值：当前块纳入会超上限，部分纳入剩余行后截断
		if totalLines+blockLines > maxDiffLines {
			remaining := maxDiffLines - totalLines
			if remaining > 0 {
				lines := strings.Split(fileBlock, "\n")
				sb.WriteString(strings.Join(lines[:remaining], "\n"))
				sb.WriteString("\n")
				droppedLines += blockLines - remaining
			} else {
				droppedLines += blockLines
			}
			droppedFiles++
			truncated = true
			continue
		}
		// 字节阈值：当前块纳入会超字节上限，部分纳入后截断
		if totalBytes+len(fileBlock) > maxDiffBytes {
			remaining := maxDiffBytes - totalBytes
			if remaining > 0 {
				sb.WriteString(fileBlock[:remaining])
			}
			droppedLines += blockLines
			droppedFiles++
			truncated = true
			continue
		}

		sb.WriteString(fileBlock)
		totalBytes += len(fileBlock)
		totalLines += blockLines
		includedFiles++
	}

	result := sb.String()
	if truncated {
		result += fmt.Sprintf("\n[已截断：剩余 %d 文件 %d 行未纳入，完整 diff 请手动审查]\n", droppedFiles, droppedLines)
	}
	return result, nil
}
