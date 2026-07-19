package service

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestScanGitRepos_ConcurrentSameRoot 并发扫描同一 rootPath 不应触发 map 读写竞态。
// 修复背景：旧实现 RepoScanCache.Entries 无锁保护，并发同 rootPath 扫描会并发写 Entries map。
func TestScanGitRepos_ConcurrentSameRoot(t *testing.T) {
	root := t.TempDir()
	// 建几个子仓库
	for _, name := range []string{"repo-a", "repo-b", "repo-c"} {
		repo := filepath.Join(root, name)
		if err := os.MkdirAll(repo, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", repo, err)
		}
		runGit(t, repo, "init")
	}

	cachePath := filepath.Join(t.TempDir(), "scan_cache.json")
	svc := NewGitServiceWithCache(cachePath)

	const concurrency = 8
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			repos := svc.ScanGitRepos(root)
			if len(repos) != 3 {
				t.Errorf("expected 3 repos, got %d: %v", len(repos), repos)
			}
		}()
	}
	wg.Wait()
}

// TestScanGitRepos_ConcurrentDifferentRoots 并发扫描不同 rootPath（落盘会序列化所有缓存）
// 不应触发 Entries map 读写竞态。
func TestScanGitRepos_ConcurrentDifferentRoots(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "scan_cache.json")
	svc := NewGitServiceWithCache(cachePath)

	roots := make([]string, 4)
	for i := range roots {
		root := t.TempDir()
		repo := filepath.Join(root, "repo")
		if err := os.MkdirAll(repo, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", repo, err)
		}
		runGit(t, repo, "init")
		roots[i] = root
	}

	var wg sync.WaitGroup
	for _, root := range roots {
		root := root
		wg.Add(1)
		go func() {
			defer wg.Done()
			repos := svc.ScanGitRepos(root)
			if len(repos) != 1 {
				t.Errorf("expected 1 repo for %s, got %d: %v", root, len(repos), repos)
			}
		}()
	}
	wg.Wait()
}

// TestScanGitRepos_ConcurrentClearAndScan 并发 ClearScanCache 与 ScanGitRepos 不应 panic。
func TestScanGitRepos_ConcurrentClearAndScan(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", repo, err)
	}
	runGit(t, repo, "init")

	cachePath := filepath.Join(t.TempDir(), "scan_cache.json")
	svc := NewGitServiceWithCache(cachePath)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			svc.ScanGitRepos(root)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			svc.ClearScanCache(root)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			svc.ScanGitRepos(root)
		}
	}()
	wg.Wait()
}
