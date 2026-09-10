package service

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// initBareRemote 创建一个 bare git 仓库作为远程，返回其绝对路径（已转正斜杠）。
// 用于 PushTag / Fetch / SetBranchUpstream 的本地真实远程集成测试（不依赖网络）。
func initBareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--bare")
	return dir
}

// findTag 按 name 在标签列表中查找，返回指针（未找到返回 nil）。
func findTag(tags []model.GitTag, name string) *model.GitTag {
	for i := range tags {
		if tags[i].Name == name {
			return &tags[i]
		}
	}
	return nil
}

// findRemote 按 name 在远程列表中查找，返回指针（未找到返回 nil）。
func findRemote(remotes []model.GitRemote, name string) *model.GitRemote {
	for i := range remotes {
		if remotes[i].Name == name {
			return &remotes[i]
		}
	}
	return nil
}

// headSha 读取仓库 HEAD 的完整 SHA（用于校验标签指向的提交）。
func headSha(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s failed: %v", repo, err)
	}
	return strings.TrimSpace(string(out))
}

// ===== ListTags =====

// TestListTags_NonRepo 非仓库目录无法定位根，返回错误。
func TestListTags_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.ListTags(t.TempDir()); err == nil {
		t.Error("非仓库 ListTags 应返回错误")
	}
}

// TestListTags_EmptyRepo 仓库无标签返回空切片。
func TestListTags_EmptyRepo(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	tags, err := svc.ListTags(repo)
	if err != nil {
		t.Fatalf("ListTags empty repo: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("空仓库应无标签, got %d", len(tags))
	}
}

// TestListTags_LightweightAndAnnotated 轻量+注释标签的类型/sha/message/tagger 解析。
func TestListTags_LightweightAndAnnotated(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	// 轻量标签：直接 ref 指向提交
	runGit(t, repo, "tag", "v1.0")
	// 注释标签：tag 对象含 tagger/message
	runGit(t, repo, "tag", "-a", "-m", "release 1.0", "v1.1")

	tags, err := svc.ListTags(repo)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %+v", len(tags), tags)
	}

	head := headSha(t, repo)

	light := findTag(tags, "v1.0")
	if light == nil {
		t.Fatal("v1.0 not found")
	}
	if light.Type != "lightweight" {
		t.Errorf("v1.0 type expected lightweight, got %s", light.Type)
	}
	if light.Sha != head {
		t.Errorf("v1.0 sha expected %s, got %s", head, light.Sha)
	}
	if light.Message != "" {
		t.Errorf("v1.0 message expected empty, got %q", light.Message)
	}
	if light.Tagger != "" {
		t.Errorf("v1.0 tagger expected empty, got %q", light.Tagger)
	}

	annot := findTag(tags, "v1.1")
	if annot == nil {
		t.Fatal("v1.1 not found")
	}
	if annot.Type != "annotated" {
		t.Errorf("v1.1 type expected annotated, got %s", annot.Type)
	}
	if annot.Sha != head {
		t.Errorf("v1.1 sha expected %s (deref to commit), got %s", head, annot.Sha)
	}
	if annot.Message != "release 1.0" {
		t.Errorf("v1.1 message expected 'release 1.0', got %q", annot.Message)
	}
	if annot.Tagger == "" {
		t.Error("v1.1 tagger should not be empty")
	}
	if annot.Date == "" {
		t.Error("v1.1 date should not be empty")
	}
	if len(annot.ShortSha) != 8 {
		t.Errorf("v1.1 shortSha expected 8 chars, got %q", annot.ShortSha)
	}
}

// ===== CreateTag =====

// TestCreateTag_Lightweight message 为空创建轻量标签。
func TestCreateTag_Lightweight(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	if err := svc.CreateTag(repo, "v1.0", ""); err != nil {
		t.Fatalf("CreateTag lightweight: %v", err)
	}
	tags, _ := svc.ListTags(repo)
	light := findTag(tags, "v1.0")
	if light == nil {
		t.Fatal("v1.0 not found after create")
	}
	if light.Type != "lightweight" {
		t.Errorf("expected lightweight, got %s", light.Type)
	}
}

// TestCreateTag_Annotated message 非空创建注释标签。
func TestCreateTag_Annotated(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	if err := svc.CreateTag(repo, "v1.1", "release note"); err != nil {
		t.Fatalf("CreateTag annotated: %v", err)
	}
	tags, _ := svc.ListTags(repo)
	annot := findTag(tags, "v1.1")
	if annot == nil {
		t.Fatal("v1.1 not found after create")
	}
	if annot.Type != "annotated" {
		t.Errorf("expected annotated, got %s", annot.Type)
	}
	if annot.Message != "release note" {
		t.Errorf("expected 'release note', got %q", annot.Message)
	}
}

// TestCreateTag_EmptyName 标签名为空返回错误。
func TestCreateTag_EmptyName(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	if err := svc.CreateTag(repo, "  ", ""); err == nil {
		t.Error("空标签名应返回错误")
	}
}

// TestCreateTag_Duplicate 重名标签返回错误。
func TestCreateTag_Duplicate(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")

	if err := svc.CreateTag(repo, "v1.0", ""); err != nil {
		t.Fatalf("first CreateTag: %v", err)
	}
	if err := svc.CreateTag(repo, "v1.0", ""); err == nil {
		t.Error("重名标签应返回错误")
	}
}

// ===== DeleteTag =====

// TestDeleteTag_Success 删除后列表不含该标签。
func TestDeleteTag_Success(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")
	runGit(t, repo, "tag", "v1.0")

	if err := svc.DeleteTag(repo, "v1.0"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	tags, _ := svc.ListTags(repo)
	if len(tags) != 0 {
		t.Errorf("删除后应无标签, got %d", len(tags))
	}
}

// TestDeleteTag_NotExists 删除不存在的标签返回错误。
func TestDeleteTag_NotExists(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	if err := svc.DeleteTag(repo, "nonexistent"); err == nil {
		t.Error("删除不存在的标签应返回错误")
	}
}

// ===== PushTag =====

// TestPushTag_NoRemote 无远程配置时推送返回错误。
func TestPushTag_NoRemote(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")
	runGit(t, repo, "tag", "v1.0")

	if _, err := svc.PushTag(repo, "v1.0"); err == nil {
		t.Error("无远程时推送标签应返回错误")
	}
}

// TestPushTag_BareRemote 推送标签到本地 bare 远程成功，远程含该标签。
func TestPushTag_BareRemote(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")
	remote := initBareRemote(t)
	runGit(t, repo, "remote", "add", "origin", filepath.ToSlash(remote))

	if err := svc.CreateTag(repo, "v1.0", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := svc.PushTag(repo, "v1.0"); err != nil {
		t.Fatalf("PushTag: %v", err)
	}

	// 验证 bare 远程含该 tag
	out, err := exec.Command("git", "-C", remote, "tag", "-l", "v1.0").Output()
	if err != nil {
		t.Fatalf("git tag -l in bare remote: %v", err)
	}
	if !strings.Contains(string(out), "v1.0") {
		t.Errorf("expected bare remote to contain v1.0, got: %s", out)
	}
}

// ===== ListRemotes =====

// TestListRemotes_NonRepo 非仓库目录返回错误。
func TestListRemotes_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.ListRemotes(t.TempDir()); err == nil {
		t.Error("非仓库 ListRemotes 应返回错误")
	}
}

// TestListRemotes_NoRemote 仓库无远程返回空切片。
func TestListRemotes_NoRemote(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	remotes, err := svc.ListRemotes(repo)
	if err != nil {
		t.Fatalf("ListRemotes no remote: %v", err)
	}
	if len(remotes) != 0 {
		t.Errorf("无远程仓库应返回空, got %d", len(remotes))
	}
}

// TestListRemotes_WithRemotes 多 remote 去重（fetch+push 行）+ 取首个 URL。
func TestListRemotes_WithRemotes(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	runGit(t, repo, "remote", "add", "origin", "https://example.com/origin.git")
	runGit(t, repo, "remote", "add", "upstream", "https://example.com/upstream.git")

	remotes, err := svc.ListRemotes(repo)
	if err != nil {
		t.Fatalf("ListRemotes: %v", err)
	}
	if len(remotes) != 2 {
		t.Fatalf("expected 2 remotes (deduped), got %d: %+v", len(remotes), remotes)
	}

	origin := findRemote(remotes, "origin")
	if origin == nil {
		t.Fatal("origin not found")
	}
	if origin.URL != "https://example.com/origin.git" {
		t.Errorf("origin URL expected https://example.com/origin.git, got %s", origin.URL)
	}
	upstream := findRemote(remotes, "upstream")
	if upstream == nil {
		t.Fatal("upstream not found")
	}
	if upstream.URL != "https://example.com/upstream.git" {
		t.Errorf("upstream URL expected https://example.com/upstream.git, got %s", upstream.URL)
	}
}

// ===== AddRemote =====

// TestAddRemote_Success 新增后列表含该远程。
func TestAddRemote_Success(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	if err := svc.AddRemote(repo, "origin", "https://example.com/origin.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
	remotes, _ := svc.ListRemotes(repo)
	if findRemote(remotes, "origin") == nil {
		t.Error("新增后列表应含 origin")
	}
}

// TestAddRemote_Duplicate 重名远程返回错误。
func TestAddRemote_Duplicate(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	runGit(t, repo, "remote", "add", "origin", "https://example.com/origin.git")
	if err := svc.AddRemote(repo, "origin", "https://example.com/another.git"); err == nil {
		t.Error("重名远程应返回错误")
	}
}

// TestAddRemote_EmptyArgs 名称或地址为空返回错误。
func TestAddRemote_EmptyArgs(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	if err := svc.AddRemote(repo, "  ", "https://example.com/x.git"); err == nil {
		t.Error("空名称应返回错误")
	}
	if err := svc.AddRemote(repo, "origin", "  "); err == nil {
		t.Error("空地址应返回错误")
	}
}

// ===== RemoveRemote =====

// TestRemoveRemote_Success 删除后列表不含该远程。
func TestRemoveRemote_Success(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	runGit(t, repo, "remote", "add", "origin", "https://example.com/origin.git")
	if err := svc.RemoveRemote(repo, "origin"); err != nil {
		t.Fatalf("RemoveRemote: %v", err)
	}
	remotes, _ := svc.ListRemotes(repo)
	if len(remotes) != 0 {
		t.Errorf("删除后应无远程, got %d", len(remotes))
	}
}

// TestRemoveRemote_NotExists 删除不存在的远程返回错误。
func TestRemoveRemote_NotExists(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	if err := svc.RemoveRemote(repo, "nonexistent"); err == nil {
		t.Error("删除不存在的远程应返回错误")
	}
}

// ===== Fetch =====

// TestFetch_InvalidRemote 指定不存在的 remote 名时 fetch 返回错误。
func TestFetch_InvalidRemote(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	if _, err := svc.Fetch(repo, "nonexistent", false); err == nil {
		t.Error("不存在的 remote 应返回错误")
	}
}

// TestFetch_BareRemote 对本地 bare 远程 fetch 成功（无网络依赖）。
func TestFetch_BareRemote(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	remote := initBareRemote(t)
	runGit(t, repo, "remote", "add", "origin", filepath.ToSlash(remote))

	if _, err := svc.Fetch(repo, "origin", false); err != nil {
		t.Fatalf("Fetch from bare remote: %v", err)
	}
}

// ===== SetBranchUpstream =====

// TestSetBranchUpstream_NoRemote 无远程配置时设上游返回错误。
func TestSetBranchUpstream_NoRemote(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	if err := svc.SetBranchUpstream(repo, "main", "origin"); err == nil {
		t.Error("无远程时设上游应返回错误")
	}
}

// TestSetBranchUpstream_EmptyArgs 分支或远程为空返回错误。
func TestSetBranchUpstream_EmptyArgs(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	if err := svc.SetBranchUpstream(repo, "  ", "origin"); err == nil {
		t.Error("空分支应返回错误")
	}
	if err := svc.SetBranchUpstream(repo, "main", "  "); err == nil {
		t.Error("空远程应返回错误")
	}
}

// TestSetBranchUpstream_Success 推送分支后设上游，HasUpstream 返回 true。
func TestSetBranchUpstream_Success(t *testing.T) {
	repo := initTempRepo(t)
	svc := NewGitService()
	writeFile(t, filepath.Join(repo, "a.txt"), "init")
	runGit(t, repo, "add", "a.txt")
	runGit(t, repo, "commit", "-m", "init")
	remote := initBareRemote(t)
	runGit(t, repo, "remote", "add", "origin", filepath.ToSlash(remote))

	// 动态获取当前分支名（兼容 master/main 默认值差异）
	branch, err := svc.gitCmd.GetBranch(repo)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		t.Fatal("current branch is empty")
	}

	// 推送分支以创建 remote-tracking ref（不带 --set-upstream，故无上游配置）
	runGit(t, repo, "push", "origin", branch)

	has, _ := svc.HasUpstream(repo)
	if has {
		t.Error("普通推送后应无上游配置")
	}

	if err := svc.SetBranchUpstream(repo, branch, "origin"); err != nil {
		t.Fatalf("SetBranchUpstream: %v", err)
	}
	has, err = svc.HasUpstream(repo)
	if err != nil {
		t.Fatalf("HasUpstream after set: %v", err)
	}
	if !has {
		t.Error("设上游后 HasUpstream 应返回 true")
	}
}
