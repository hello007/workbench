package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// 素材库文件夹树（ListFolders）用例。夹具 = 临时目录 + 直接落盘空目录/伪 pptx
// 文件（ListFolders 纯只读不解析，文件内容无关紧要）；禁依赖环境外部状态
// （docs/spec/test-stability.md）。

// mustFindFolder 递归查找树中指定相对路径的节点（miss 返回 nil）。
func mustFindFolder(nodes []model.AssetFolderNode, path string) *model.AssetFolderNode {
	for i := range nodes {
		if nodes[i].Path == path {
			return &nodes[i]
		}
		if hit := mustFindFolder(nodes[i].Children, path); hit != nil {
			return hit
		}
	}
	return nil
}

// TestAssetListFolders_EmptyDirVisible 空目录必须在树中可见（D1 树 = 真实文件
// 系统目录，非索引派生）；深层嵌套按父子结构返回，同级按文件系统字典序。
func TestAssetListFolders_EmptyDirVisible(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	// 布局：a/（空）、b/（空）、b/inner/（空）、b/inner/deep/（空）、c/（空）
	// ——字典序 a < b < c；b/inner/deep 三级嵌套守护树装配（值拷贝装配会裁掉深层）
	for _, dir := range []string{"a", "b", filepath.Join("b", "inner"), filepath.Join("b", "inner", "deep"), "c"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	lib := mustCreateLibrary(t, svc, "文件夹树库", root)

	nodes, err := svc.ListFolders(lib.ID)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("一级目录数应 3，实际 %d：%+v", len(nodes), nodes)
	}
	wantOrder := []string{"a", "b", "c"}
	for i, want := range wantOrder {
		if nodes[i].Name != want || nodes[i].Path != want {
			t.Errorf("第 %d 个节点应为 %s，实际 %s（path=%s）", i, want, nodes[i].Name, nodes[i].Path)
		}
		if nodes[i].Excluded {
			t.Errorf("节点 %s 不应带排除标记", want)
		}
		if nodes[i].FileCount != 0 {
			t.Errorf("空目录 %s FileCount 应 0，实际 %d", want, nodes[i].FileCount)
		}
	}
	// 空目录 Children 恒非 nil（前端免判空契约）
	if nodes[0].Children == nil || len(nodes[0].Children) != 0 {
		t.Errorf("空目录 Children 应为非 nil 空切片")
	}
	// 深层嵌套：b/inner 挂在 b 下，b/inner/deep 挂在 b/inner 下（三级全可见）
	inner := mustFindFolder(nodes, "b/inner")
	if inner == nil {
		t.Fatalf("b/inner 应在树中可见：%+v", nodes)
	}
	if deep := mustFindFolder(nodes, "b/inner/deep"); deep == nil {
		t.Errorf("b/inner/deep 应在树中可见（深层装配完整）：%+v", nodes)
	}
}

// TestAssetListFolders_ExcludedMarking 排除规则命中目录带标记且整棵子树继承
// （对齐扫描 SkipDir 剪除整棵子树的语义）；未命中目录不带标记。
func TestAssetListFolders_ExcludedMarking(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	for _, dir := range []string{"secret", filepath.Join("secret", "inner"), "normal"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	lib := mustCreateLibrary(t, svc, "排除标记库", root)
	// 直接改注册表规则（绕过 UpdateLibraryRules 的自动重扫，本链路与索引无关）
	if err := svc.setLibraryRules(lib.ID, []string{"secret"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}

	nodes, err := svc.ListFolders(lib.ID)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	secret := mustFindFolder(nodes, "secret")
	if secret == nil || !secret.Excluded {
		t.Fatalf("secret 应带排除标记：%+v", nodes)
	}
	inner := mustFindFolder(nodes, "secret/inner")
	if inner == nil || !inner.Excluded {
		t.Errorf("被排除祖先下的子目录应继承排除标记：%+v", nodes)
	}
	normal := mustFindFolder(nodes, "normal")
	if normal == nil || normal.Excluded {
		t.Errorf("未命中目录不应带排除标记：%+v", nodes)
	}
}

// TestAssetListFolders_RootExempt 库根目录自身豁免匹配（对齐 §12.1 root 豁免，
// 根名恰为排除模式时不得整库标排除）。
func TestAssetListFolders_RootExempt(t *testing.T) {
	svc := newTestAssetService(t)
	root := filepath.Join(t.TempDir(), "secret")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "根豁免库", root)
	if err := svc.setLibraryRules(lib.ID, []string{"secret"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}

	nodes, err := svc.ListFolders(lib.ID)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("应返回一级子目录 1 个，实际 %d：%+v", len(nodes), nodes)
	}
	if nodes[0].Excluded {
		t.Errorf("根名命中模式时子目录不应被标排除（root 豁免）：%+v", nodes[0])
	}
}

// TestAssetListFolders_FileCountDirectOnly FileCount 只计直属 PPT（不含子目录
// 内文件），且排除规则后缀命中与拆分派生副本不入计数（与扫描收集口径同源）。
func TestAssetListFolders_FileCountDirectOnly(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	// 布局：
	//   media/          直属 a.pptx、b.PPTX（大小写不敏感）、skip.tmp（后缀规则命中）、
	//                   media-001.pptx（拆分派生布局命中）→ 计 2
	//   media/sub/      c.pptx → 不计入 media
	//   other/          直属 d.pptx → 计 1
	media := filepath.Join(root, "media")
	sub := filepath.Join(media, "sub")
	other := filepath.Join(root, "other")
	for _, dir := range []string{media, sub, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	files := map[string]string{
		filepath.Join(media, "a.pptx"):         "a",
		filepath.Join(media, "b.PPTX"):         "b",
		filepath.Join(media, "skip.tmp"):       "skip",
		filepath.Join(media, "media-001.pptx"): "derived",
		filepath.Join(sub, "c.pptx"):           "c",
		filepath.Join(other, "d.pptx"):         "d",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
	}
	lib := mustCreateLibrary(t, svc, "计数库", root)
	if err := svc.setLibraryRules(lib.ID, nil, []string{".tmp"}); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}

	nodes, err := svc.ListFolders(lib.ID)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	mediaNode := mustFindFolder(nodes, "media")
	if mediaNode == nil {
		t.Fatalf("media 应在树中：%+v", nodes)
	}
	if mediaNode.FileCount != 2 {
		t.Errorf("media FileCount 应 2（a.pptx + b.PPTX，排除 tmp 与拆分派生，不含子目录），实际 %d", mediaNode.FileCount)
	}
	otherNode := mustFindFolder(nodes, "other")
	if otherNode == nil || otherNode.FileCount != 1 {
		t.Errorf("other FileCount 应 1，实际 %d", otherNode.FileCount)
	}
}

// TestAssetListFolders_LibNotFound 库不存在报 E_ASSETS_LIB_NOT_FOUND（前端自愈
// 重拉库列表同口径）。
func TestAssetListFolders_LibNotFound(t *testing.T) {
	svc := newTestAssetService(t)
	_, err := svc.ListFolders("不存在的库")
	if !errors.Is(err, errAssetLibraryNotFound) {
		t.Fatalf("应返回 errAssetLibraryNotFound，实际：%v", err)
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsLibNotFound {
		t.Errorf("错误码应为 E_ASSETS_LIB_NOT_FOUND，实际：%v", err)
	}
}

// TestAssetListFolders_RootMissing 库根目录不存在（磁盘被删/移动）返回空树非
// 错误——对齐扫描侧 collectPptxEntries 根不可入的降级语义，不阻塞面板渲染。
func TestAssetListFolders_RootMissing(t *testing.T) {
	svc := newTestAssetService(t)
	root := filepath.Join(t.TempDir(), "gone")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "根消失库", root)
	if err := os.Remove(root); err != nil {
		t.Fatalf("Remove(root): %v", err)
	}

	nodes, err := svc.ListFolders(lib.ID)
	if err != nil {
		t.Fatalf("根目录不存在应返回空树非错误，实际 err：%v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("根目录不存在应返回空树，实际 %d 节点", len(nodes))
	}
}
