package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"workbench/model"
	"workbench/util"
)

// stash 域 GitService 方法（list/push/pop/apply/drop）。
//
// 变更类操作（push/pop/apply/drop）对齐 Commit/DiscardChanges 先例：tryLockRepo
// 仓级互斥 + FindGitRoot 定位 + Execute（30s 超时兜底，锁占用识别 E_GIT_LOCKED）；
// stash 为秒级单命令，与 CommitFiles 同级不注册独立取消入口。列表只读不抢仓级锁。
//
// 编码：stash 描述可含中文（-m 自定义），全部 git 调用统一前置 UTF-8 前缀
// （docs/spec/branch-compare.md，复用 utf8Prefix）。
//
// 冲突语义（git 默认，如实转译）：pop/apply 冲突时保留 stash 条目（pop 亦不
// drop），工作区留下未合并文件；以 git status --porcelain 的未合并 XY 组合判定
// 冲突（git 自身标记，权威），冲突态走正常返回通道（ConflictFiles 非空），冲突
// 解决引导至「合并/变基」tab 既有链路，零新增错误码。

// stashListFormat stash list 输出格式：\x00 记录分隔、\x01 字段分隔；
// %gd reflog 选择器（--date=iso 下形如 stash@{2026-11-07 14:30:45 +0800}，
// 借此取创建时间）、%gs reflog 主题（含分支与自定义描述）。
const stashListFormat = "%x00%gd%x01%gs"

// stashTimeRe 从 --date=iso 的 reflog 选择器中提取大括号内的时间文本。
var stashTimeRe = regexp.MustCompile(`\{(.+)\}$`)

// stashSubjectRe 解析 reflog 主题的分支与基点短 SHA。默认主题形如
// "WIP on master: abc1234 msg"（自动生成）、"On master: msg"（含 -m 描述）、
// "index on master: abc1234 msg"（--keep-index/含 untracked 分量时产生的伴生条目）。
var stashSubjectRe = regexp.MustCompile(`^(?:WIP on|index on|On) ([^:]+): (?:([0-9a-f]+) )?(.*)$`)

// ListStashes 列出仓库 stash 条目（stash@{0} 最新在前，与 git stash list 顺序一致）。
// 无条目返回空切片（非 nil，前端消费零值稳定）。
func (s *GitService) ListStashes(repoPath string) ([]model.StashEntry, error) {
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	args := append(utf8Prefix(), "stash", "list", "--date=iso", "--format="+stashListFormat)
	out, err := s.gitCmd.Execute(gitRoot, args...)
	if err != nil {
		return nil, fmt.Errorf("获取 stash 列表失败: %w", err)
	}
	return parseStashEntries(out), nil
}

// parseStashEntries 解析 stash list 输出（\x00 记录分隔、\x01 字段分隔）。
// index 取记录序号（stash list 顺序即 stash@{n}，n 从 0 起）；单条记录字段缺失
// 时跳过，不让脏数据毁掉整表。
func parseStashEntries(out string) []model.StashEntry {
	entries := []model.StashEntry{}
	for _, rec := range strings.Split(out, "\x00") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		fields := strings.Split(rec, "\x01")
		if len(fields) < 2 {
			continue
		}

		entry := model.StashEntry{Index: len(entries)}
		if m := stashTimeRe.FindStringSubmatch(fields[0]); m != nil {
			if ts, err := time.Parse("2006-01-02 15:04:05 -0700", m[1]); err == nil {
				entry.Timestamp = ts.Unix()
				entry.DateTime = ts.Local().Format("2006-01-02 15:04")
			}
		}
		if m := stashSubjectRe.FindStringSubmatch(fields[1]); m != nil {
			entry.Branch = m[1]
			entry.ShortSha = m[2]
			entry.Description = m[3]
		} else {
			entry.Description = fields[1]
		}
		entries = append(entries, entry)
	}
	return entries
}

// StashPush 暂存工作现场到 stash：默认全部 tracked 变更，includeUntracked 追加
// -u 一并暂存未跟踪文件。message 为空时使用 git 默认文案（"WIP on ..."）。
// 无变更时 git 以 exit 0 静默返回（"No local changes to save"），视为成功，
// 真实状态由前端列表刷新反馈。
func (s *GitService) StashPush(repoPath, message string, includeUntracked bool) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	args := append(utf8Prefix(), "stash", "push")
	if strings.TrimSpace(message) != "" {
		args = append(args, "-m", message)
	}
	if includeUntracked {
		args = append(args, "-u")
	}
	if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
		return fmt.Errorf("stash 暂存失败: %w", err)
	}
	return nil
}

// StashPop 恢复指定 stash 条目并从列表删除；冲突时 git 默认保留条目（不 drop），
// 返回 ConflictFiles 非空的结果（非 error），工作区冲突交由「合并/变基」tab 处理。
func (s *GitService) StashPop(repoPath string, index int) (*model.StashApplyResult, error) {
	return s.stashApply(repoPath, index, "pop")
}

// StashApply 恢复指定 stash 条目但保留在列表中；冲突语义同 StashPop。
func (s *GitService) StashApply(repoPath string, index int) (*model.StashApplyResult, error) {
	return s.stashApply(repoPath, index, "apply")
}

// stashApply pop/apply 统一实现：冲突时 git 以 exit 1 退出且详情走 stderr，
// 白名单接受 exit 1 后以 porcelain 未合并文件分流——非空即冲突态（正常返回
// ConflictFiles），空则 exit 1 属非冲突失败（如本地变更会被覆盖），stderr 即
// 错误原文返回 error。
func (s *GitService) stashApply(repoPath string, index int, action string) (*model.StashApplyResult, error) {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return nil, err
	}
	defer release()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	args := append(utf8Prefix(), "stash", action, fmt.Sprintf("stash@{%d}", index))
	stdout, stderr, err := s.gitCmd.ExecuteWithCodesAndStderr(gitRoot, map[int]bool{1: true}, args...)
	if err != nil {
		return nil, fmt.Errorf("stash %s 失败: %w", action, err)
	}

	conflicts := s.unmergedFiles(gitRoot)
	// stdout 与 stderr 以换行拼接（TrimSpace 收尾），防两流末行无换行时粘连
	output := strings.TrimSpace(stdout + "\n" + stderr)
	if len(conflicts) > 0 {
		return &model.StashApplyResult{Output: output, ConflictFiles: conflicts}, nil
	}
	if exit1Failed(action, stderr) {
		return nil, fmt.Errorf("stash %s 失败: %s", action, strings.TrimSpace(stderr))
	}
	return &model.StashApplyResult{Output: output}, nil
}

// exit1Failed 判定 exit 1 白名单命中且无未合并文件时是否为真实失败：
// stderr 含 "error:" 行即 git 明确报错（如 error: Your local changes ... would
// be overwritten）；仅进度文案（Auto-merging 等）视为正常完成。
func exit1Failed(action, stderr string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "error:") {
			return true
		}
	}
	return false
}

// StashDrop 删除指定 stash 条目。
func (s *GitService) StashDrop(repoPath string, index int) error {
	release, err := s.tryLockRepo(repoPath)
	if err != nil {
		return err
	}
	defer release()

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	args := append(utf8Prefix(), "stash", "drop", fmt.Sprintf("stash@{%d}", index))
	if _, err := s.gitCmd.Execute(gitRoot, args...); err != nil {
		return fmt.Errorf("stash 删除失败: %w", err)
	}
	return nil
}

// unmergedFiles 查询工作区未合并文件列表（porcelain v1 的 8 种未合并 XY 组合）。
// pop/apply 冲突判定的权威来源（git 自身标记的未合并态）。查询失败返回 nil
// （不误报冲突；真实失败已由 git 非 1 退出码走错误路径）。status 输出含文件
// 路径，中文名文件须 UTF-8 前缀（core.quotepath=off）防八进制转义（见
// branch-compare.md git 调用硬约束）。
func (s *GitService) unmergedFiles(gitRoot string) []string {
	args := append(utf8Prefix(), "status", "--porcelain")
	out, err := s.gitCmd.Execute(gitRoot, args...)
	if err != nil {
		return nil
	}
	// porcelain v1 未合并 XY 组合（git-status 文档）：DD/AU/UD/UA/DU/DA/UU/AA
	unmerged := map[string]bool{
		"DD": true, "AU": true, "UD": true, "UA": true,
		"DU": true, "DA": true, "UU": true, "AA": true,
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 {
			continue
		}
		if unmerged[line[:2]] {
			files = append(files, strings.TrimSpace(line[3:]))
		}
	}
	return files
}
