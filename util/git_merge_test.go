package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/util/testutil"
)

// TestMerge_FF_Success 可快进场景 merge ff 成功，无冲突态。
func TestMerge_FF_Success(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	g := NewGitCommand()
	out, err := g.Merge(dir, "feature", "ff")
	if err != nil {
		t.Fatalf("Merge ff: %v (out=%s)", err, out)
	}
	if g.IsMergeInProgress(dir) {
		t.Error("ff 合并不应进入冲突态")
	}
	files, err := g.ListConflictFiles(dir)
	if err != nil {
		t.Fatalf("ListConflictFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("ff 合并应无冲突文件, got %v", files)
	}
}

// TestMerge_NoFF_ProducesMergeCommit no-ff 模式产生合并提交，仍无冲突。
func TestMerge_NoFF_ProducesMergeCommit(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	g := NewGitCommand()
	if _, err := g.Merge(dir, "feature", "no-ff"); err != nil {
		t.Fatalf("Merge no-ff: %v", err)
	}
	if g.IsMergeInProgress(dir) {
		t.Error("no-ff 合并不应停留在冲突态")
	}
	// 合并提交消息默认 "Merge branch 'feature'"
	log, err := g.Execute(dir, "log", "--oneline", "-1")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if !strings.Contains(log, "Merge branch 'feature'") {
		t.Errorf("no-ff 应产生合并提交, got %s", strings.TrimSpace(log))
	}
}

// TestMerge_Squash_ProducesStagedChange squash 压缩为暂存变更，不产生合并提交。
func TestMerge_Squash_ProducesStagedChange(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	g := NewGitCommand()
	if _, err := g.Merge(dir, "feature", "squash"); err != nil {
		t.Fatalf("Merge squash: %v", err)
	}
	// squash 后变更在暂存区但未提交，工作区有暂存内容
	status, err := g.Execute(dir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(status, "a.txt") {
		t.Errorf("squash 应留下暂存变更, got %q", status)
	}
}

// TestMerge_Conflict 冲突场景 merge 以 exit 1 正常返回，进入合并冲突态，冲突文件含 a.txt。
func TestMerge_Conflict(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	g := NewGitCommand()
	out, err := g.Merge(dir, "feature", "ff")
	// 冲突 exit 1 被 ExecuteWithCodes 接受，err 应为 nil
	if err != nil {
		t.Fatalf("冲突应被接受为正常返回, got err=%v out=%s", err, out)
	}
	if !g.IsMergeInProgress(dir) {
		t.Error("冲突后应处于合并进行中态")
	}
	files, err := g.ListConflictFiles(dir)
	if err != nil {
		t.Fatalf("ListConflictFiles: %v", err)
	}
	found := false
	for _, f := range files {
		if strings.TrimSpace(f) == "a.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("冲突文件应含 a.txt, got %v", files)
	}
}

// TestMergeAbort 清理冲突态：merge 冲突后 abort，退出合并态，a.txt 回到 master 版本。
func TestMergeAbort(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	g := NewGitCommand()
	g.Merge(dir, "feature", "ff")
	if !g.IsMergeInProgress(dir) {
		t.Fatal("前置：应处于合并冲突态")
	}
	if _, err := g.MergeAbort(dir); err != nil {
		t.Fatalf("MergeAbort: %v", err)
	}
	if g.IsMergeInProgress(dir) {
		t.Error("abort 后应退出合并冲突态")
	}
	content, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if !strings.Contains(string(content), "master-line2") {
		t.Error("abort 后 a.txt 应回到 master 版本")
	}
}

// TestMergeContinue 冲突解决后 continue 完成合并提交，退出冲突态。
func TestMergeContinue(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	g := NewGitCommand()
	g.Merge(dir, "feature", "ff")
	// 解决冲突：覆盖为确定内容并 add
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "line1\nresolved-line2\nline3\n")
	testutil.RunGit(t, dir, "add", "a.txt")
	if _, err := g.MergeContinue(dir); err != nil {
		t.Fatalf("MergeContinue: %v", err)
	}
	if g.IsMergeInProgress(dir) {
		t.Error("continue 后应退出合并冲突态")
	}
}

// TestIsMergeInProgress_NoMerge 无合并进行时返回 false。
func TestIsMergeInProgress_NoMerge(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	g := NewGitCommand()
	if g.IsMergeInProgress(dir) {
		t.Error("无合并时应返回 false")
	}
}

// TestIsRebaseInProgress_NoRebase 无变基进行时返回 false。
func TestIsRebaseInProgress_NoRebase(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	g := NewGitCommand()
	if g.IsRebaseInProgress(dir) {
		t.Error("无变基时应返回 false")
	}
}

// TestIsCherryPickInProgress_NoCherryPick 无拣选进行时返回 false。
func TestIsCherryPickInProgress_NoCherryPick(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	g := NewGitCommand()
	if g.IsCherryPickInProgress(dir) {
		t.Error("无拣选时应返回 false")
	}
}

// TestRebase_Conflict 变基冲突进入变基态。
func TestRebase_Conflict(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	g := NewGitCommand()
	// 在 master 上 rebase feature：master 改 line2 与 feature 改 line2 冲突
	_, err := g.Rebase(dir, "feature")
	if err != nil {
		t.Fatalf("rebase 冲突应被接受为正常返回: %v", err)
	}
	if !g.IsRebaseInProgress(dir) {
		t.Error("rebase 冲突后应处于变基态")
	}
}

// TestRebaseAbort 变基冲突后 abort 回滚。
func TestRebaseAbort(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	g := NewGitCommand()
	g.Rebase(dir, "feature")
	if !g.IsRebaseInProgress(dir) {
		t.Fatal("前置：应处于变基态")
	}
	if _, err := g.RebaseAbort(dir); err != nil {
		t.Fatalf("RebaseAbort: %v", err)
	}
	if g.IsRebaseInProgress(dir) {
		t.Error("abort 后应退出变基态")
	}
}

// TestCherryPick_Success 无冲突拣选成功。
func TestCherryPick_Success(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	g := NewGitCommand()
	// 取 feature 最新提交 SHA 拣选到 master
	shaOut, err := g.Execute(dir, "rev-parse", "feature")
	if err != nil {
		t.Fatalf("rev-parse feature: %v", err)
	}
	sha := strings.TrimSpace(shaOut)
	if _, err := g.CherryPick(dir, sha); err != nil {
		t.Fatalf("CherryPick: %v", err)
	}
	if g.IsCherryPickInProgress(dir) {
		t.Error("无冲突拣选不应进入拣选冲突态")
	}
}

// TestCherryPick_Conflict 冲突拣选进入拣选态。
func TestCherryPick_Conflict(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	g := NewGitCommand()
	// feature 改 line2 的提交拣选到 master（master 也改 line2）必冲突
	shaOut, err := g.Execute(dir, "rev-parse", "feature")
	if err != nil {
		t.Fatalf("rev-parse feature: %v", err)
	}
	sha := strings.TrimSpace(shaOut)
	if _, err := g.CherryPick(dir, sha); err != nil {
		t.Fatalf("cherry-pick 冲突应被接受为正常返回: %v", err)
	}
	if !g.IsCherryPickInProgress(dir) {
		t.Error("冲突后应处于拣选冲突态")
	}
}

// TestCherryPickAbort 拣选冲突后 abort 回滚。
func TestCherryPickAbort(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	g := NewGitCommand()
	shaOut, _ := g.Execute(dir, "rev-parse", "feature")
	sha := strings.TrimSpace(shaOut)
	g.CherryPick(dir, sha)
	if !g.IsCherryPickInProgress(dir) {
		t.Fatal("前置：应处于拣选冲突态")
	}
	if _, err := g.CherryPickAbort(dir); err != nil {
		t.Fatalf("CherryPickAbort: %v", err)
	}
	if g.IsCherryPickInProgress(dir) {
		t.Error("abort 后应退出拣选冲突态")
	}
}

// TestListConflictFiles_NoConflict 无冲突时返回空切片（非 nil）。
func TestListConflictFiles_NoConflict(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	g := NewGitCommand()
	files, err := g.ListConflictFiles(dir)
	if err != nil {
		t.Fatalf("ListConflictFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("无冲突应返回空切片, got %v", files)
	}
}
