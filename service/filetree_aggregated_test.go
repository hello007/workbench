package service

import (
	"encoding/json"
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

	// 切到空临时目录执行：空白串是相对路径，避免进程 cwd 下恰好存在同名目录导致分流失效
	t.Chdir(t.TempDir())
	// 空串路径跳过不产出节点（防御收藏数据脏值）
	nodes = svc.GetRootNodesForPaths([]string{"", "  "})
	// "  " 为非法路径 Stat 失败 -> Missing 占位（不静默丢弃，前端可见可移除）
	if len(nodes) != 1 {
		t.Fatalf("blank path should produce missing placeholder, got %d nodes", len(nodes))
	}
	if !nodes[0].Missing {
		t.Error("blank path placeholder should be marked missing")
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
