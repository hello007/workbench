package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
