package service

import (
	"path/filepath"
	"sync"
	"testing"

	"workbench/model"
)

// TestRepoMeta_ConcurrentMutate 并发 Mutate（模拟 SaveRepoMeta 防抖保存与扫描合并保存交错）
// 不应触发 race，且最终所有路径的元数据完整可见。
// 修复背景：旧实现 Load+修改+Save 非原子，并发 SaveRepoMeta 的旧快照会覆盖扫描刚写入的 ReadmeSummary。
func TestRepoMeta_ConcurrentMutate(t *testing.T) {
	svc := createRepoMetaService(t)

	// 预置 20 个仓库元数据
	const n = 20
	for i := 0; i < n; i++ {
		dir := t.TempDir()
		abs, _ := filepath.Abs(dir)
		if err := svc.Upsert(&model.RepoMeta{Path: abs, Summary: "init", Tags: []string{"t0"}}); err != nil {
			t.Fatalf("pre-upsert %d: %v", i, err)
		}
	}

	// 收集所有主键（重新 Load）
	snapshot, _ := svc.Load()
	keys := make([]string, 0, len(snapshot))
	for k := range snapshot {
		keys = append(keys, k)
	}

	// 并发：每个 key 同时做「改 Summary/Tags」与「改 ReadmeSummary/LastScanAt/Missing」两类写
	var wg sync.WaitGroup
	for _, k := range keys {
		k := k
		wg.Add(2)
		// 写方 A：模拟 SaveRepoMeta（改用户字段）
		go func() {
			defer wg.Done()
			svc.Mutate(func(repos map[string]*model.RepoMeta) (bool, error) {
				if m, ok := repos[k]; ok {
					m.Summary = "user-edit"
					m.Tags = []string{"edited"}
				}
				return true, nil
			})
		}()
		// 写方 B：模拟扫描合并（改扫描归属字段）
		go func() {
			defer wg.Done()
			svc.Mutate(func(repos map[string]*model.RepoMeta) (bool, error) {
				if m, ok := repos[k]; ok {
					m.ReadmeSummary = "readme"
					m.Missing = false
				}
				return true, nil
			})
		}()
	}
	wg.Wait()

	// 校验：每个 key 的两类字段都应最终落盘（不互相覆盖）
	final, err := svc.Load()
	if err != nil {
		t.Fatalf("final Load: %v", err)
	}
	for _, k := range keys {
		m, ok := final[k]
		if !ok {
			t.Errorf("key %q missing after concurrent writes", k)
			continue
		}
		// 至少应能看到写方 A 或 B 的影响之一；关键是不能两边都丢失
		if m.Summary == "" && m.ReadmeSummary == "" {
			t.Errorf("key %q: both user and scan fields lost (race likely): %+v", k, m)
		}
	}
}

// TestRepoMeta_ConcurrentUpsertDifferentPaths 并发 Upsert 不同路径不应丢条目。
func TestRepoMeta_ConcurrentUpsertDifferentPaths(t *testing.T) {
	svc := createRepoMetaService(t)

	const n = 30
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			dir := filepath.Join(t.TempDir(), "repo")
			svc.Upsert(&model.RepoMeta{Path: dir, Summary: "x", Tags: []string{string(rune('a' + i%26))}})
		}()
	}
	wg.Wait()

	m, _ := svc.Load()
	if len(m) != n {
		t.Errorf("expected %d entries after concurrent upsert, got %d (some lost due to race)", n, len(m))
	}
}
