package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// GetRootNodesForPaths 聚合视图根节点构造测试：
// 覆盖目录/文件/失效三类入参的分流、git 徽标填充、顺序保持与空入参。

func TestGetRootNodesForPaths_DirectoryEntries(t *testing.T) {
	base := t.TempDir()
	dirA := filepath.Join(base, "alpha")
	dirB := filepath.Join(base, "beta")
	mustMkdir(t, dirA)
	mustMkdir(t, dirB)

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{dirA, dirB})

	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Path != dirA || nodes[1].Path != dirB {
		t.Errorf("node order should follow input order, got %s, %s", nodes[0].Path, nodes[1].Path)
	}
	for _, n := range nodes {
		if n.Type != "directory" {
			t.Errorf("node %s type should be directory, got %s", n.Path, n.Type)
		}
		if n.Missing {
			t.Errorf("existing directory %s should not be marked missing", n.Path)
		}
		if n.IsLeaf {
			t.Errorf("directory node %s should not be leaf", n.Path)
		}
		if n.ID != n.Path {
			t.Errorf("node ID should equal path for lazy load, got %s", n.ID)
		}
	}
}

func TestGetRootNodesForPaths_FileEntrySkipped(t *testing.T) {
	base := t.TempDir()
	dirA := filepath.Join(base, "folder")
	mustMkdir(t, dirA)
	fileB := filepath.Join(base, "notes.md")
	mustWriteFile(t, fileB, []byte("content"))
	dirC := filepath.Join(base, "folder-2")
	mustMkdir(t, dirC)

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{dirA, fileB, dirC})

	// 文件类收藏不进聚合树（仅目录类型收藏构成聚合根），且被跳过后顺序保持
	if len(nodes) != 2 {
		t.Fatalf("want 2 directory nodes, got %d", len(nodes))
	}
	if nodes[0].Path != dirA || nodes[1].Path != dirC {
		t.Errorf("remaining nodes should keep input order [dirA, dirC], got [%s, %s]", nodes[0].Path, nodes[1].Path)
	}
}

func TestGetRootNodesForPaths_MissingPathPlaceholder(t *testing.T) {
	base := t.TempDir()
	existing := filepath.Join(base, "exists")
	mustMkdir(t, existing)
	gone := filepath.Join(base, "deleted-dir")

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{existing, gone})

	if len(nodes) != 2 {
		t.Fatalf("missing path should produce placeholder node instead of being dropped, got %d nodes", len(nodes))
	}
	missing := nodes[1]
	if missing.Path != gone {
		t.Errorf("placeholder path mismatch, got %s", missing.Path)
	}
	if !missing.Missing {
		t.Error("placeholder node should have Missing=true")
	}
	if missing.Type != "directory" {
		t.Errorf("placeholder type should stay directory for frontend styling, got %s", missing.Type)
	}
	if missing.Name != "deleted-dir" {
		t.Errorf("placeholder name should be path base, got %s", missing.Name)
	}
	// ID 必须等于 Path：前端 el-tree node-key 与「移除收藏」都按该值定位
	if missing.ID != gone {
		t.Errorf("placeholder ID should equal path, got %s", missing.ID)
	}
	// 失效目录无子节点可加载：hasChildren/isLeaf 必须与 missing 语义一致，
	// 不能沿用 directory 类型的默认值（否则消费面会展开一个不存在的目录）
	if missing.HasChildren {
		t.Error("placeholder node should have HasChildren=false")
	}
	if !missing.IsLeaf {
		t.Error("placeholder node should have IsLeaf=true")
	}
}

// Stat 失败统一产 Missing 占位，同时写 Warn 日志留存原始错误：
// 前端对「不存在/无权限/网络盘不可达」共用同一提示口径，不区分原因，
// 因此排障只能依赖后端日志，缺日志会让失效占位无从解释。
func TestGetRootNodesForPaths_StatFailureLogsWarn(t *testing.T) {
	base := t.TempDir()
	gone := filepath.Join(base, "gone-dir")

	// 捕获告警日志（SetLogger 全局注入；本包串行用例，Cleanup 还原）
	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{gone})

	if len(nodes) != 1 {
		t.Fatalf("stat 失败仍须产出占位节点，实际 %d 个", len(nodes))
	}
	if !nodes[0].Missing {
		t.Errorf("stat 失败产出的节点应带 Missing=true，实际 %+v", nodes[0])
	}

	logged := logBuf.String()
	if !strings.Contains(logged, "level=WARN") {
		t.Errorf("Stat 失败须以 Warn 级别记录，实际日志：%s", logged)
	}
	if !strings.Contains(logged, "aggregated root stat failed") {
		t.Errorf("Warn 日志缺少事件标识，实际日志：%s", logged)
	}
	if !strings.Contains(logged, "gone-dir") {
		t.Errorf("Warn 日志须带路径字段，实际日志：%s", logged)
	}
	if !strings.Contains(logged, "err=") {
		t.Errorf("Warn 日志须带原始 error 字段，实际日志：%s", logged)
	}
}

func TestGetRootNodesForPaths_GitRepoBadge(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	mustMkdir(t, repo)
	mustMkdir(t, filepath.Join(repo, ".git"))
	plain := filepath.Join(base, "plain")
	mustMkdir(t, plain)

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{repo, plain})

	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	if !nodes[0].IsGitRepo {
		t.Error("repo should have IsGitRepo=true")
	}
	if nodes[1].IsGitRepo {
		t.Error("plain dir should have IsGitRepo=false")
	}
}

func TestGetRootNodesForPaths_EmptyAndBlankInputs(t *testing.T) {
	svc := NewFileTreeService()

	nodes := svc.GetRootNodesForPaths(nil)
	if nodes == nil || len(nodes) != 0 {
		t.Errorf("nil input should return empty non-nil slice, got %v", nodes)
	}

	// 空串与纯空白串统一跳过，不产出 Missing 占位：
	// 否则空白脏值与「收藏目录已失效」的兜底语义混淆，用户看到无法移除的空白节点
	nodes = svc.GetRootNodesForPaths([]string{"", "  ", "\t"})
	if len(nodes) != 0 {
		t.Errorf("blank inputs should produce no nodes, got %d", len(nodes))
	}
}

// TestGetRootNodesForPaths_TrailingSeparatorNormalized 守护尾分隔符归一：
// D:\repo\ 与 D:\repo 必须产出同一 Path/ID（前端 el-tree node-key=path），
// 且 git 徽标缓存落在同一个清洗后的键上，不重复缓存。
func TestGetRootNodesForPaths_TrailingSeparatorNormalized(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	mustMkdir(t, repo)
	withTrailing := repo + string(os.PathSeparator)

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{withTrailing, repo})
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	for i, n := range nodes {
		if n.Path != repo {
			t.Errorf("node[%d].Path should be cleaned to %q, got %q", i, repo, n.Path)
		}
		if n.ID != repo {
			t.Errorf("node[%d].ID should equal cleaned path %q, got %q", i, repo, n.ID)
		}
		if n.Name != "repo" {
			t.Errorf("node[%d].Name should be path base, got %q", i, n.Name)
		}
	}

	// 缓存键同源：清洗后路径有项，带尾分隔符的原始写法不得留下缓存项
	if _, ok := svc.gitRepoCache.Load(repo); !ok {
		t.Error("git 徽标缓存应落在清洗后的路径键上")
	}
	if _, ok := svc.gitRepoCache.Load(withTrailing); ok {
		t.Error("带尾分隔符的原始写法不应产生独立缓存键")
	}
}

// TestGetRootNodesForPaths_VolumeRootDisplayName 守护卷根展示名：
// C:\ / / 的 filepath.Base 结果是分隔符本身，须回退 VolumeName（非 Windows 卷
// 再回退清洗路径），不得产出空名或 "."，且节点可展开。
func TestGetRootNodesForPaths_VolumeRootDisplayName(t *testing.T) {
	// 用临时目录所在卷构造卷根，避免硬编码盘符
	base := t.TempDir()
	vol := filepath.VolumeName(base)
	root := vol + string(os.PathSeparator)

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{root})
	if len(nodes) != 1 {
		t.Fatalf("want 1 volume root node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Name == "" || n.Name == "." {
		t.Errorf("volume root should have readable display name, got %q", n.Name)
	}
	// 有卷名的平台（Windows C:\）不得把裸分隔符当名称：去掉回退逻辑后
	// filepath.Base("C:\\") 返回 "\\"，仅靠非空判定无法识别，故显式断言不含分隔符。
	// Linux 根 "/" 的 VolumeName 为空，展示名回退为路径自身 "/"，不参与该断言。
	if filepath.VolumeName(root) != "" && strings.ContainsAny(n.Name, `\/`) {
		t.Errorf("volume root display name should be readable, got %q", n.Name)
	}
	if n.Path != filepath.Clean(root) {
		t.Errorf("volume root path should be cleaned to %q, got %q", filepath.Clean(root), n.Path)
	}
	if n.Missing {
		t.Error("volume root should exist and not be marked missing")
	}
	if n.IsLeaf || !n.HasChildren {
		t.Errorf("volume root should be expandable, got isLeaf=%v hasChildren=%v", n.IsLeaf, n.HasChildren)
	}
}

// TestInvalidateCache_RefreshesGitBadge 守护 git 徽标缓存失效通道：
// 首次探测把仓库缓存为 false 后，git init / git remote add 再经 InvalidateCache，
// 手动刷新须立即看到新徽标（否则用户刷新后仍是旧结论）。
func TestInvalidateCache_RefreshesGitBadge(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	mustMkdir(t, repo)

	svc := NewFileTreeService()
	// 预热：仓库尚未初始化，缓存 IsGitRepo=false
	before := svc.GetRootNodesForPaths([]string{repo})
	if len(before) != 1 || before[0].IsGitRepo {
		t.Fatalf("前置验证失败：未初始化前应为非 git 仓库: %+v", before)
	}

	testutil.RunGit(t, repo, "init")
	testutil.RunGit(t, repo, "remote", "add", "origin", "https://example.com/repo.git")
	svc.InvalidateCache(repo)

	after := svc.GetRootNodesForPaths([]string{repo})
	if len(after) != 1 {
		t.Fatalf("want 1 node, got %d", len(after))
	}
	if !after[0].IsGitRepo {
		t.Error("InvalidateCache 后 git init 应立即反映为 IsGitRepo=true")
	}
	if !after[0].HasRemote {
		t.Error("InvalidateCache 后 git remote add 应立即反映为 HasRemote=true")
	}
}

// TestInvalidateCache_ClearsDirectChildGitBadge 守护前缀匹配范围：
// 刷新父目录时直接子目录徽标同步失效（git init 后刷新父目录即可见），
// 孙级不级联清空（避免刷新父目录清空整棵子树的 git 探测缓存），
// 同前缀兄弟目录（parent 与 parent-other）不被误删。
func TestInvalidateCache_ClearsDirectChildGitBadge(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	child := filepath.Join(parent, "child")
	grand := filepath.Join(child, "grand")
	// 与 parent 共享前缀串的兄弟目录：前缀判定必须带分隔符，否则会误删其缓存
	sibling := filepath.Join(base, "parent-other")
	mustMkdir(t, parent)
	mustMkdir(t, child)
	mustMkdir(t, grand)
	mustMkdir(t, sibling)

	svc := NewFileTreeService()
	// 预热：GetChildren(parent) 探测直接子目录 child，GetChildren(child) 探测孙级 grand，
	// GetRootNodesForPaths(sibling) 探测同前缀兄弟目录
	if _, err := svc.GetChildren(parent); err != nil {
		t.Fatalf("预热 GetChildren(parent): %v", err)
	}
	if _, err := svc.GetChildren(child); err != nil {
		t.Fatalf("预热 GetChildren(child): %v", err)
	}
	if _, ok := svc.gitRepoCache.Load(child); !ok {
		t.Fatal("前置验证失败：child 应已缓存 git 徽标")
	}
	if _, ok := svc.gitRepoCache.Load(grand); !ok {
		t.Fatal("前置验证失败：grand 应已缓存 git 徽标")
	}
	svc.GetRootNodesForPaths([]string{sibling})
	if _, ok := svc.gitRepoCache.Load(sibling); !ok {
		t.Fatal("前置验证失败：sibling 应已缓存 git 徽标")
	}

	testutil.RunGit(t, child, "init")
	testutil.RunGit(t, grand, "init")
	svc.InvalidateCache(parent)

	if nodes := svc.GetRootNodesForPaths([]string{child}); !nodes[0].IsGitRepo {
		t.Error("刷新父目录后直接子目录徽标应失效重探（child 应变为 git 仓库）")
	}
	// 孙级保持旧缓存：只清直接子目录，不级联
	if nodes := svc.GetRootNodesForPaths([]string{grand}); nodes[0].IsGitRepo {
		t.Error("孙级 git 徽标不应被父目录刷新级联清空")
	}
	// 同前缀兄弟目录保持缓存：C:\a\parent 的前缀不得命中 C:\a\parent-other
	if _, ok := svc.gitRepoCache.Load(sibling); !ok {
		t.Error("同前缀兄弟目录的 git 徽标缓存不应被误删")
	}
}

// TestInvalidateCache_ClearsDirectChildGitBadgeAtVolumeRoot 守护卷根的前缀匹配：
// filepath.Clean("C:\\") 仍带尾分隔符，直接拼接会得到「C:\\」，HasPrefix 对
// 「C:\Windows」恒为 false，卷根下一级子目录的 git 徽标永不失效。直接操作私有
// 缓存 map，不依赖磁盘真实目录（只验证缓存键的匹配范围）。
func TestInvalidateCache_ClearsDirectChildGitBadgeAtVolumeRoot(t *testing.T) {
	vol := filepath.VolumeName(t.TempDir())
	root := vol + string(os.PathSeparator) // Windows: C:\；Unix 卷名为空: /
	if vol == "" {
		root = string(os.PathSeparator)
	}

	child := filepath.Join(root, "workbench-child-dir")
	grand := filepath.Join(child, "grand")
	// 根之外的深层键：Unix 下所有绝对路径都以「/」开头，故取两层以上深度，
	// 确保其不在卷根的直接子目录范围内。
	outside := filepath.Join(t.TempDir(), "outside", "nested")

	svc := NewFileTreeService()
	for _, cache := range []*sync.Map{&svc.gitRepoCache, &svc.gitRemoteCache} {
		cache.Store(child, true)
		cache.Store(grand, true)
		cache.Store(outside, true)
	}

	svc.InvalidateCache(root)

	checkCache := func(name string, cache *sync.Map) {
		if _, ok := cache.Load(child); ok {
			t.Errorf("%s: 卷根的直接子目录缓存应被清除", name)
		}
		if _, ok := cache.Load(grand); !ok {
			t.Errorf("%s: 孙级缓存不应被级联清除", name)
		}
		if _, ok := cache.Load(outside); !ok {
			t.Errorf("%s: 卷根之外的键不应被误删", name)
		}
	}
	checkCache("gitRepoCache", &svc.gitRepoCache)
	checkCache("gitRemoteCache", &svc.gitRemoteCache)
}

// TestClearAllCache_RefreshesGitBadge 守护全量刷新的 git 徽标通道：
// ClearAllCache 须逐键清空 gitRepoCache / gitRemoteCache。
func TestClearAllCache_RefreshesGitBadge(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	mustMkdir(t, repo)

	svc := NewFileTreeService()
	before := svc.GetRootNodesForPaths([]string{repo})
	if len(before) != 1 || before[0].IsGitRepo {
		t.Fatalf("前置验证失败：未初始化前应为非 git 仓库: %+v", before)
	}

	testutil.RunGit(t, repo, "init")
	testutil.RunGit(t, repo, "remote", "add", "origin", "https://example.com/repo.git")
	svc.ClearAllCache()

	after := svc.GetRootNodesForPaths([]string{repo})
	if len(after) != 1 {
		t.Fatalf("want 1 node, got %d", len(after))
	}
	if !after[0].IsGitRepo || !after[0].HasRemote {
		t.Errorf("ClearAllCache 后徽标应重新探测: IsGitRepo=%v HasRemote=%v", after[0].IsGitRepo, after[0].HasRemote)
	}
}

func TestGetRootNodesForPaths_EmptyInputNotNilSlice(t *testing.T) {
	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{})
	if nodes == nil {
		t.Fatal("empty input should return non-nil slice for JSON serialization as []")
	}
}

// TestGetRootNodesForPaths_HasRemoteBadge 守护 HasRemote 分支：伪造 .git 目录时
// `git remote -v` 恒失败使断言无判别力（删除 node.HasRemote 赋值仍全绿），故用真实
// 仓库区分「有 remote」「无 remote」两种形态。徽标误判会让聚合视图把无远端的本地
// 仓库显示为可推送，误导用户点击同步。
func TestGetRootNodesForPaths_HasRemoteBadge(t *testing.T) {
	base := t.TempDir()
	withRemote := filepath.Join(base, "with-remote")
	withoutRemote := filepath.Join(base, "without-remote")
	mustMkdir(t, withRemote)
	mustMkdir(t, withoutRemote)

	testutil.RunGit(t, withRemote, "init")
	testutil.RunGit(t, withRemote, "remote", "add", "origin", "https://example.com/repo.git")
	testutil.RunGit(t, withoutRemote, "init")

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths([]string{withRemote, withoutRemote})

	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	if !nodes[0].IsGitRepo {
		t.Error("带 remote 的仓库应 IsGitRepo=true")
	}
	if !nodes[0].HasRemote {
		t.Error("配置了 origin 的仓库应 HasRemote=true")
	}
	if !nodes[1].IsGitRepo {
		t.Error("无 remote 的仓库仍应 IsGitRepo=true")
	}
	if nodes[1].HasRemote {
		t.Error("未配置 remote 的仓库应 HasRemote=false")
	}
}

// TestGetRootNodesForPaths_ConcurrentHundredPreservesOrder 守护批量并发化：
// 100 个混合入参（正常目录 / 不存在路径 / 文件 / git 仓库）经两阶段并发探测后，
// 输出顺序必须严格等于「入参过滤后的顺序」，不得随 goroutine 完成次序漂移；
// 缺失项产 Missing 占位、文件项被跳过、git/remote 徽标按各自目录正确回填。
// 每个目录的入参位置与其创建序号错开（倒序 + 定点覆盖），保证顺序断言有判别力。
func TestGetRootNodesForPaths_ConcurrentHundredPreservesOrder(t *testing.T) {
	const total = 100
	base := t.TempDir()

	dirs := make([]string, total)
	wantRepo := make([]bool, total)
	wantRemote := make([]bool, total)
	for i := 0; i < total; i++ {
		d := filepath.Join(base, fmt.Sprintf("dir-%03d", i))
		mustMkdir(t, d)
		dirs[i] = d
		// 每 10 个一个 git 仓库，每 20 个再加一个 origin 远程
		if i%10 == 0 {
			testutil.RunGit(t, d, "init")
			wantRepo[i] = true
			if i%20 == 0 {
				testutil.RunGit(t, d, "remote", "add", "origin", "https://example.com/repo.git")
				wantRemote[i] = true
			}
		}
	}
	filePath := filepath.Join(base, "notes.md")
	mustWriteFile(t, filePath, []byte("content"))

	// 入参倒序排列，再在固定下标覆盖缺失项与文件项：
	// 若实现按并发完成顺序 append，倒序输入的输出会被打乱并被下方逐位断言捕获
	inputs := make([]string, total)
	for i := 0; i < total; i++ {
		inputs[i] = dirs[total-1-i]
	}
	const (
		missingAtA = 0
		fileAt     = 10
		missingAtB = 50
		missingAtC = 99
	)
	missingA := filepath.Join(base, "missing-a")
	missingB := filepath.Join(base, "missing-b")
	missingC := filepath.Join(base, "missing-c")
	inputs[missingAtA] = missingA
	inputs[fileAt] = filePath
	inputs[missingAtB] = missingB
	inputs[missingAtC] = missingC

	svc := NewFileTreeService()
	nodes := svc.GetRootNodesForPaths(inputs)

	// 100 项中 1 个文件项被跳过，其余（含 3 个缺失占位）全部产出
	if len(nodes) != total-1 {
		t.Fatalf("want %d nodes (file entry skipped), got %d", total-1, len(nodes))
	}

	out := 0
	for i := 0; i < total; i++ {
		if i == fileAt {
			// 文件项不产节点；若被误当节点输出，后续逐位路径断言会整体错位失败
			continue
		}
		n := nodes[out]
		out++
		if n.Path != inputs[i] {
			t.Fatalf("node[%d].Path = %q, want %q（输出顺序须与入参一致）", out-1, n.Path, inputs[i])
		}
		if n.ID != inputs[i] {
			t.Errorf("node[%d].ID should equal path %q, got %q", out-1, inputs[i], n.ID)
		}

		if i == missingAtA || i == missingAtB || i == missingAtC {
			if !n.Missing {
				t.Errorf("missing input %q should produce placeholder with Missing=true", inputs[i])
			}
			if n.HasChildren || !n.IsLeaf {
				t.Errorf("missing placeholder %q should be leaf, got hasChildren=%v isLeaf=%v", inputs[i], n.HasChildren, n.IsLeaf)
			}
			continue
		}

		if n.Missing {
			t.Errorf("existing directory %q should not be marked missing", inputs[i])
		}
		src := total - 1 - i
		if n.IsGitRepo != wantRepo[src] {
			t.Errorf("dir %q IsGitRepo = %v, want %v", inputs[i], n.IsGitRepo, wantRepo[src])
		}
		if n.HasRemote != wantRemote[src] {
			t.Errorf("dir %q HasRemote = %v, want %v", inputs[i], n.HasRemote, wantRemote[src])
		}
	}
	if out != len(nodes) {
		t.Errorf("consumed %d nodes but got %d（存在多余或缺失节点）", out, len(nodes))
	}
}

// TestFileTreeNode_MissingJSONTag 锚定 wire 契约：tag 笔误（如 json:"isMissing"）
// 会让前端 n.missing 恒为 undefined（失效节点不标灰、仍可展开）而后端测试全绿，
// 故显式断言序列化键名。
func TestFileTreeNode_MissingJSONTag(t *testing.T) {
	b, err := json.Marshal(model.FileTreeNode{Missing: true})
	if err != nil {
		t.Fatalf("marshal FileTreeNode: %v", err)
	}
	if !strings.Contains(string(b), `"missing":true`) {
		t.Errorf("FileTreeNode.Missing should serialize as \"missing\":true, got %s", string(b))
	}
}
