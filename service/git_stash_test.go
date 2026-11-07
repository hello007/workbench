package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/util"
	"workbench/util/testutil"
)

// stashTestRepo 构造带基线提交的 stash 测试仓库：master 分支 + a.txt 基线提交。
func stashTestRepo(t *testing.T) string {
	t.Helper()
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "base\n")
	testutil.RunGit(t, dir, "add", "a.txt")
	testutil.RunGit(t, dir, "commit", "-m", "base")
	return dir
}

func TestParseStashEntries_EmptyOutput(t *testing.T) {
	entries := parseStashEntries("")
	if entries == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseStashEntries_WIPSubjectWithChineseDescription(t *testing.T) {
	// 自动生成的 WIP 主题：分支 + 基点短 SHA + 描述；描述含中文须原样往返
	out := "\x00stash@{2026-11-07 10:30:45 +0800}\x01WIP on master: abc1234 修复登录中文乱码\n"
	entries := parseStashEntries(out)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Index != 0 {
		t.Errorf("index = %d, want 0", e.Index)
	}
	if e.Branch != "master" {
		t.Errorf("branch = %q, want master", e.Branch)
	}
	if e.ShortSha != "abc1234" {
		t.Errorf("shortSha = %q, want abc1234", e.ShortSha)
	}
	if e.Description != "修复登录中文乱码" {
		t.Errorf("description = %q, want 修复登录中文乱码", e.Description)
	}
	if e.Timestamp == 0 || e.DateTime == "" {
		t.Errorf("time fields not parsed: timestamp=%d dateTime=%q", e.Timestamp, e.DateTime)
	}
}

func TestParseStashEntries_OnSubjectCustomMessage(t *testing.T) {
	// 带 -m 自定义描述的主题：On <branch>: <msg>，无基点 SHA 段
	out := "\x00stash@{2026-11-07 10:30:45 +0800}\x01On feature: 切分支前的临时改动\n"
	entries := parseStashEntries(out)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Branch != "feature" {
		t.Errorf("branch = %q, want feature", e.Branch)
	}
	if e.ShortSha != "" {
		t.Errorf("shortSha = %q, want empty", e.ShortSha)
	}
	if e.Description != "切分支前的临时改动" {
		t.Errorf("description = %q, want 切分支前的临时改动", e.Description)
	}
}

func TestParseStashEntries_MultipleEntriesIndexAscending(t *testing.T) {
	out := "\x00stash@{2026-11-07 10:30:45 +0800}\x01WIP on master: abc1234 newest\n" +
		"\x00stash@{2026-11-06 09:00:00 +0800}\x01On dev: older\n"
	entries := parseStashEntries(out)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Index != 0 || entries[1].Index != 1 {
		t.Errorf("index not ascending: %d, %d", entries[0].Index, entries[1].Index)
	}
	if entries[0].Description != "newest" || entries[1].Description != "older" {
		t.Errorf("descriptions = %q, %q", entries[0].Description, entries[1].Description)
	}
}

func TestParseStashEntries_SkipsMalformedRecord(t *testing.T) {
	// 缺字段分隔符的脏记录跳过，不让单条脏数据毁掉整表
	out := "\x00malformed-no-separator\n\x00stash@{2026-11-07 10:30:45 +0800}\x01On master: ok\n"
	entries := parseStashEntries(out)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Description != "ok" {
		t.Errorf("description = %q, want ok", entries[0].Description)
	}
}

func TestListStashes_EmptyRepo(t *testing.T) {
	repo := stashTestRepo(t)
	svc := NewGitService()

	entries, err := svc.ListStashes(repo)
	if err != nil {
		t.Fatalf("ListStashes failed: %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Fatalf("expected empty slice, got %v", entries)
	}
}

func TestListStashes_AfterPushChineseDescriptionRoundTrip(t *testing.T) {
	repo := stashTestRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "work in progress\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "中文描述：切换分支前的改动", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}

	entries, err := svc.ListStashes(repo)
	if err != nil {
		t.Fatalf("ListStashes failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Description != "中文描述：切换分支前的改动" {
		t.Errorf("description round-trip broken: %q", e.Description)
	}
	if e.Branch != "master" {
		t.Errorf("branch = %q, want master", e.Branch)
	}
	// -m 自定义描述的主题为 "On <branch>: <msg>"（无基点 SHA 段），ShortSha 应为空
	if e.ShortSha != "" || e.Timestamp == 0 || e.DateTime == "" {
		t.Errorf("parsed fields unexpected: %+v", e)
	}
}

func TestStashPush_NoLocalChangesSucceedsSilently(t *testing.T) {
	repo := stashTestRepo(t)
	svc := NewGitService()

	// 工作区干净：git stash push 以 exit 0 静默返回，视为成功
	if err := svc.StashPush(repo, "无变更", false); err != nil {
		t.Fatalf("StashPush on clean worktree should succeed, got: %v", err)
	}
	entries, err := svc.ListStashes(repo)
	if err != nil {
		t.Fatalf("ListStashes failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestStashPush_IncludeUntrackedStashesNewFile(t *testing.T) {
	repo := stashTestRepo(t)
	untrackedPath := filepath.Join(repo, "new.txt")
	testutil.WriteFile(t, untrackedPath, "untracked content\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "含未跟踪", true); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}

	// -u 把未跟踪文件一并收进 stash：工作区该文件消失
	if _, err := filepath.Abs(untrackedPath); err != nil {
		t.Fatalf("Abs failed: %v", err)
	}
	entries, err := svc.ListStashes(repo)
	if err != nil {
		t.Fatalf("ListStashes failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// pop 后未跟踪文件回到工作区且内容一致
	if _, err := svc.StashPop(repo, 0); err != nil {
		t.Fatalf("StashPop failed: %v", err)
	}
	content, err := os.ReadFile(untrackedPath)
	if err != nil {
		t.Fatalf("read untracked file after pop: %v", err)
	}
	if strings.ReplaceAll(string(content), "\r\n", "\n") != "untracked content\n" {
		t.Errorf("untracked content = %q, want original", content)
	}
}

func TestStashPop_RestoresChangesAndRemovesEntry(t *testing.T) {
	repo := stashTestRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "work in progress\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}

	result, err := svc.StashPop(repo, 0)
	if err != nil {
		t.Fatalf("StashPop failed: %v", err)
	}
	if len(result.ConflictFiles) != 0 {
		t.Fatalf("expected no conflicts, got %v", result.ConflictFiles)
	}

	content, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if strings.ReplaceAll(string(content), "\r\n", "\n") != "work in progress\n" {
		t.Errorf("a.txt = %q, want restored content", content)
	}

	entries, err := svc.ListStashes(repo)
	if err != nil {
		t.Fatalf("ListStashes failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after pop, got %d", len(entries))
	}
}

func TestStashApply_RestoresChangesAndKeepsEntry(t *testing.T) {
	repo := stashTestRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "work in progress\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}

	result, err := svc.StashApply(repo, 0)
	if err != nil {
		t.Fatalf("StashApply failed: %v", err)
	}
	if len(result.ConflictFiles) != 0 {
		t.Fatalf("expected no conflicts, got %v", result.ConflictFiles)
	}

	entries, err := svc.ListStashes(repo)
	if err != nil {
		t.Fatalf("ListStashes failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("apply should keep the entry, got %d entries", len(entries))
	}
}

func TestStashDrop_RemovesEntry(t *testing.T) {
	repo := stashTestRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "work in progress\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}
	if err := svc.StashDrop(repo, 0); err != nil {
		t.Fatalf("StashDrop failed: %v", err)
	}

	entries, err := svc.ListStashes(repo)
	if err != nil {
		t.Fatalf("ListStashes failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after drop, got %d", len(entries))
	}
}

func TestStashDrop_InvalidIndexReturnsError(t *testing.T) {
	repo := stashTestRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "work in progress\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}
	if err := svc.StashDrop(repo, 5); err == nil {
		t.Fatal("StashDrop with out-of-range index should fail")
	}
}

func TestStashPop_ConflictKeepsStashAndReportsConflictFiles(t *testing.T) {
	repo := stashTestRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "work in progress\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "冲突现场", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}
	// stash 后同文件提交另一内容：pop 时该文件必然冲突
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "committed change\n")
	testutil.RunGit(t, repo, "commit", "-am", "other")

	result, err := svc.StashPop(repo, 0)
	if err != nil {
		t.Fatalf("conflicted pop should return normally (not error), got: %v", err)
	}
	if len(result.ConflictFiles) != 1 || result.ConflictFiles[0] != "a.txt" {
		t.Fatalf("conflictFiles = %v, want [a.txt]", result.ConflictFiles)
	}
	if result.Output == "" {
		t.Fatal("conflict output should carry git output detail")
	}

	// git 默认语义：pop 冲突不 drop，条目保留
	entries, err := svc.ListStashes(repo)
	if err != nil {
		t.Fatalf("ListStashes failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("stash entry should survive conflicted pop, got %d entries", len(entries))
	}
}

func TestStashApply_OverwriteFailureReturnsError(t *testing.T) {
	repo := stashTestRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "work in progress\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}
	// 工作区对同文件有未提交变更：apply 被 git 拒绝（would be overwritten），
	// exit 1 但无未合并文件 → 返回 error（非冲突态）
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "committed change\n")
	if _, err := svc.StashApply(repo, 0); err == nil {
		t.Fatal("apply against dirty worktree should fail with error")
	}
}

func TestUnmergedFiles_ParsesPorcelain(t *testing.T) {
	repo := stashTestRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "work in progress\n")
	svc := NewGitService()

	if err := svc.StashPush(repo, "", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "committed change\n")
	testutil.RunGit(t, repo, "commit", "-am", "other")
	if _, err := svc.StashPop(repo, 0); err != nil {
		t.Fatalf("StashPop failed: %v", err)
	}

	gitRoot, err := util.FindGitRoot(repo)
	if err != nil {
		t.Fatalf("FindGitRoot failed: %v", err)
	}
	files := svc.unmergedFiles(gitRoot)
	if len(files) != 1 || files[0] != "a.txt" {
		t.Fatalf("unmergedFiles = %v, want [a.txt]", files)
	}
}

func TestUnmergedFiles_ChineseFilenameRoundTrip(t *testing.T) {
	// 中文名冲突文件经 quotepath=off 原样往返（禁八进制转义与引号包裹）
	repo := stashTestRepo(t)
	cnPath := filepath.Join(repo, "修复登录乱码.txt")
	testutil.WriteFile(t, cnPath, "work in progress\n")
	testutil.RunGit(t, repo, "add", "修复登录乱码.txt")
	testutil.RunGit(t, repo, "commit", "-m", "add cn file")
	svc := NewGitService()

	testutil.WriteFile(t, cnPath, "work in progress 2\n")
	if err := svc.StashPush(repo, "中文名冲突现场", false); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}
	testutil.WriteFile(t, cnPath, "committed change\n")
	testutil.RunGit(t, repo, "commit", "-am", "other")
	result, err := svc.StashPop(repo, 0)
	if err != nil {
		t.Fatalf("StashPop failed: %v", err)
	}

	if len(result.ConflictFiles) != 1 || result.ConflictFiles[0] != "修复登录乱码.txt" {
		t.Fatalf("conflictFiles = %v, want [修复登录乱码.txt]", result.ConflictFiles)
	}
}

func TestStashOperations_NonRepoPathReturnsError(t *testing.T) {
	dir := t.TempDir()
	svc := NewGitService()

	if _, err := svc.ListStashes(dir); err == nil {
		t.Fatal("ListStashes on non-repo should fail")
	}
	if err := svc.StashPush(dir, "", false); err == nil {
		t.Fatal("StashPush on non-repo should fail")
	}
	if _, err := svc.StashPop(dir, 0); err == nil {
		t.Fatal("StashPop on non-repo should fail")
	}
	if _, err := svc.StashApply(dir, 0); err == nil {
		t.Fatal("StashApply on non-repo should fail")
	}
	if err := svc.StashDrop(dir, 0); err == nil {
		t.Fatal("StashDrop on non-repo should fail")
	}
}

// exit1Failed 行级判定：error: 前缀为真实失败，进度文案（Auto-merging）不算
func TestExit1Failed_LineLevel(t *testing.T) {
	if !exit1Failed("pop", "error: Your local changes would be overwritten\n") {
		t.Fatal("error: line should be treated as failure")
	}
	if exit1Failed("pop", "Auto-merging a.txt\n") {
		t.Fatal("progress-only stderr should not be treated as failure")
	}
	if exit1Failed("pop", "") {
		t.Fatal("empty stderr should not be treated as failure")
	}
	if !exit1Failed("pop", "Auto-merging a.txt\nerror: could not restore untracked files\n") {
		t.Fatal("error: line among progress lines should be treated as failure")
	}
}
