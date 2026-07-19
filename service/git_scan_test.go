package service

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanGitRepos_GitFileAsWorktree worktree/submodule 的 .git 是文件（非目录），
// 预筛应识别（不要求 IsDir），这是本优化相对旧 fork git rev-parse 的关键覆盖点。
func TestScanGitRepos_GitFileAsWorktree(t *testing.T) {
	root := t.TempDir()
	worktreeDir := filepath.Join(root, "worktree-repo")
	if err := os.MkdirAll(worktreeDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 模拟 worktree 的 .git 文件（内容形如 "gitdir: /path/..."）
	if err := os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte("gitdir: /some/path/.git/worktrees/x"), 0644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}

	svc := NewGitService()
	repos := svc.ScanGitRepos(root)

	found := false
	for _, r := range repos {
		if r == worktreeDir {
			found = true
		}
	}
	if !found {
		t.Errorf("expected worktree repo (.git file) to be detected, got: %v", repos)
	}
}

// TestScanGitRepos_WithCache 注入缓存后扫描结果与无缓存一致，二次扫描（缓存命中）结果不变。
func TestScanGitRepos_WithCache(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "repo-a")
	os.MkdirAll(repoA, 0755)
	runGit(t, repoA, "init")

	cachePath := filepath.Join(t.TempDir(), "scan_cache.json")
	svc := NewGitServiceWithCache(cachePath)

	// 首次扫描（写入缓存）
	repos1 := svc.ScanGitRepos(root)
	if len(repos1) != 1 {
		t.Fatalf("first scan: expected 1 repo, got %d: %v", len(repos1), repos1)
	}

	// 二次扫描（缓存命中）应返回相同结果
	repos2 := svc.ScanGitRepos(root)
	if len(repos2) != 1 {
		t.Fatalf("second scan (cached): expected 1 repo, got %d: %v", len(repos2), repos2)
	}
	if repos2[0] != repoA {
		t.Errorf("cached scan path: got %q, want %q", repos2[0], repoA)
	}

	// 清除缓存后扫描仍正确
	svc.ClearScanCache(root)
	repos3 := svc.ScanGitRepos(root)
	if len(repos3) != 1 {
		t.Fatalf("after clear cache: expected 1 repo, got %d: %v", len(repos3), repos3)
	}
}

// TestScanGitRepos_CachePersists 缓存落盘后，新实例加载缓存仍能给出正确结果（缓存命中场景）。
func TestScanGitRepos_CachePersists(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "repo-a")
	os.MkdirAll(repoA, 0755)
	runGit(t, repoA, "init")

	cachePath := filepath.Join(t.TempDir(), "scan_cache.json")
	svc1 := NewGitServiceWithCache(cachePath)
	svc1.ScanGitRepos(root) // 首次扫描写入缓存并落盘

	// 新实例从磁盘加载缓存
	svc2 := NewGitServiceWithCache(cachePath)
	repos := svc2.ScanGitRepos(root)
	if len(repos) != 1 {
		t.Fatalf("new instance with persisted cache: expected 1 repo, got %d: %v", len(repos), repos)
	}
}

// TestScanGitRepos_CacheDetectsNewRepo 新增仓库后，父目录 mtime 变化 -> 缓存失效 -> 重新扫描发现新仓库。
func TestScanGitRepos_CacheDetectsNewRepo(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "repo-a")
	os.MkdirAll(repoA, 0755)
	runGit(t, repoA, "init")

	cachePath := filepath.Join(t.TempDir(), "scan_cache.json")
	svc := NewGitServiceWithCache(cachePath)

	if repos := svc.ScanGitRepos(root); len(repos) != 1 {
		t.Fatalf("initial scan: expected 1 repo, got %d", len(repos))
	}

	// 新增第二个仓库（root 的直接子项，root mtime 变化）
	repoB := filepath.Join(root, "repo-b")
	os.MkdirAll(repoB, 0755)
	runGit(t, repoB, "init")

	repos := svc.ScanGitRepos(root)
	if len(repos) != 2 {
		t.Fatalf("after adding repo-b: expected 2 repos, got %d: %v", len(repos), repos)
	}
}

// TestHasRemotesBatch 批量远程检测：有远程/无远程/非仓库 三种情况。
func TestHasRemotesBatch(t *testing.T) {
	repoWithRemote := initTempRepo(t)
	runGit(t, repoWithRemote, "remote", "add", "origin", "https://example.com/repo.git")

	repoNoRemote := initTempRepo(t)

	nonRepo := t.TempDir()

	svc := NewGitService()
	result := svc.HasRemotesBatch([]string{repoWithRemote, repoNoRemote, nonRepo})

	if !result[repoWithRemote] {
		t.Error("expected repo with remote to be true")
	}
	if result[repoNoRemote] {
		t.Error("expected repo without remote to be false")
	}
	if result[nonRepo] {
		t.Error("expected non-repo path to be false")
	}
}

// TestHasRemotesBatch_Empty 空输入应返回空 map 不报错。
func TestHasRemotesBatch_Empty(t *testing.T) {
	svc := NewGitService()
	result := svc.HasRemotesBatch(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map for nil input, got %d entries", len(result))
	}
}

// TestClearScanCache_NoCache 未注入缓存时 ClearScanCache 为空操作不 panic。
func TestClearScanCache_NoCache(t *testing.T) {
	svc := NewGitService() // 无缓存
	// 不应 panic
	svc.ClearScanCache(t.TempDir())
}
