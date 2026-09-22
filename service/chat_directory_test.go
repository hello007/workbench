package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ===== 目录项 CRUD（AI 对话侧栏常用目录）=====

// TestChatDirectory_AddAndList 添加 + 列表：默认名取目录末段、SortOrder 追加递增、
// 规范化路径去重（重复添加返回既有项不新增）。
func TestChatDirectory_AddAndList(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	root := t.TempDir()
	dirA := filepath.Join(root, "alpha")
	dirB := filepath.Join(root, "beta")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("构造测试目录失败: %v", err)
		}
	}

	item1, err := svc.AddChatDirectory(dirA, "")
	if err != nil {
		t.Fatalf("添加目录失败: %v", err)
	}
	if item1.DisplayName != "alpha" {
		t.Errorf("默认显示名应取目录末段，got %q", item1.DisplayName)
	}
	if item1.Path != dirA {
		t.Errorf("路径应规范化为绝对路径，got %q", item1.Path)
	}
	if item1.ID == "" {
		t.Error("ID 不应为空")
	}

	item2, err := svc.AddChatDirectory(dirB, "项目管理")
	if err != nil {
		t.Fatalf("添加目录失败: %v", err)
	}
	if item2.DisplayName != "项目管理" {
		t.Errorf("显式显示名应保留，got %q", item2.DisplayName)
	}
	if item2.SortOrder <= item1.SortOrder {
		t.Errorf("追加项 SortOrder 应递增，got %d <= %d", item2.SortOrder, item1.SortOrder)
	}

	// 重复添加（同路径）：返回既有项，列表不增长
	dup, err := svc.AddChatDirectory(dirA, "别名不同")
	if err != nil {
		t.Fatalf("重复添加不应报错: %v", err)
	}
	if dup.ID != item1.ID {
		t.Errorf("重复添加应返回既有项，got %s want %s", dup.ID, item1.ID)
	}

	list, err := svc.ListChatDirectories()
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("列表应含 2 项（重复添加去重），got %d", len(list))
	}
	if list[0].ID != item1.ID || list[1].ID != item2.ID {
		t.Errorf("列表顺序应按 SortOrder 升序，got %s, %s", list[0].ID, list[1].ID)
	}
}

// TestChatDirectory_AddValidation 添加校验：空路径与不存在的路径均报错。
func TestChatDirectory_AddValidation(t *testing.T) {
	svc, _ := newChatSvcForTest(t)

	if _, err := svc.AddChatDirectory("  ", "x"); err == nil {
		t.Error("空路径应报错")
	}
	if _, err := svc.AddChatDirectory(filepath.Join(t.TempDir(), "not-exist"), ""); err == nil {
		t.Error("不存在的路径应报错")
	}
	// 文件（非目录）路径应报错
	file := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("构造测试文件失败: %v", err)
	}
	if _, err := svc.AddChatDirectory(file, ""); err == nil {
		t.Error("文件路径应报错（须为目录）")
	}
}

// TestChatDirectory_UpdateRemove 更新显示名 / 移除：改后生效、移除后列表消失、
// 操作不存在的项报错。
func TestChatDirectory_UpdateRemove(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	dir := t.TempDir()
	item, err := svc.AddChatDirectory(dir, "原名")
	if err != nil {
		t.Fatalf("添加目录失败: %v", err)
	}

	if err := svc.UpdateChatDirectory(item.ID, "  新名  "); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	list, _ := svc.ListChatDirectories()
	if list[0].DisplayName != "新名" {
		t.Errorf("显示名应去空白后更新，got %q", list[0].DisplayName)
	}

	if err := svc.UpdateChatDirectory(item.ID, "   "); err == nil {
		t.Error("空显示名应报错")
	}
	if err := svc.UpdateChatDirectory("chatdir-missing", "x"); err == nil {
		t.Error("更新不存在的项应报错")
	}
	if err := svc.RemoveChatDirectory("chatdir-missing"); err == nil {
		t.Error("移除不存在的项应报错")
	}

	if err := svc.RemoveChatDirectory(item.ID); err != nil {
		t.Fatalf("移除失败: %v", err)
	}
	list, _ = svc.ListChatDirectories()
	if len(list) != 0 {
		t.Errorf("移除后列表应为空，got %d", len(list))
	}
}

// TestChatDirectory_Reorder 重排序：按 ids 顺序重写 SortOrder，重载后列表序跟随；
// 缺失项保持原位不丢失。
func TestChatDirectory_Reorder(t *testing.T) {
	svc, _ := newChatSvcForTest(t)
	root := t.TempDir()
	var ids []string
	for _, name := range []string{"a", "b", "c"} {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("构造测试目录失败: %v", err)
		}
		item, err := svc.AddChatDirectory(d, "")
		if err != nil {
			t.Fatalf("添加目录失败: %v", err)
		}
		ids = append(ids, item.ID)
	}

	// 倒序重排
	if err := svc.ReorderChatDirectories([]string{ids[2], ids[0], ids[1]}); err != nil {
		t.Fatalf("重排失败: %v", err)
	}
	list, _ := svc.ListChatDirectories()
	if list[0].ID != ids[2] || list[1].ID != ids[0] || list[2].ID != ids[1] {
		t.Errorf("重排后顺序不符，got %s, %s, %s", list[0].ID, list[1].ID, list[2].ID)
	}

	// 缺失项（只传前两个）：按原相对顺序稳定排尾部，SortOrder 归一化不丢项
	if err := svc.ReorderChatDirectories([]string{ids[1], ids[0]}); err != nil {
		t.Fatalf("局部重排失败: %v", err)
	}
	list, _ = svc.ListChatDirectories()
	if len(list) != 3 {
		t.Fatalf("局部重排不应丢项，got %d", len(list))
	}
	if list[0].ID != ids[1] || list[1].ID != ids[0] {
		t.Errorf("前两项顺序不符，got %s, %s", list[0].ID, list[1].ID)
	}
	if list[2].ID != ids[2] {
		t.Errorf("缺失项应稳定排尾部，got %s", list[2].ID)
	}
}

// TestChatDirectory_PersistenceReload 持久化：重建 service 后目录项仍在。
func TestChatDirectory_PersistenceReload(t *testing.T) {
	dataRoot := t.TempDir()
	dir := filepath.Join(dataRoot, "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("构造测试目录失败: %v", err)
	}

	svc := NewChatService(context.Background(), filepath.Join(dataRoot, "ai_chat"))
	item, err := svc.AddChatDirectory(dir, "项目管理")
	if err != nil {
		t.Fatalf("添加目录失败: %v", err)
	}

	reloaded := NewChatService(context.Background(), filepath.Join(dataRoot, "ai_chat"))
	list, err := reloaded.ListChatDirectories()
	if err != nil {
		t.Fatalf("重载列表失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != item.ID || list[0].DisplayName != "项目管理" {
		t.Errorf("重载后应保留目录项，got %+v", list)
	}
}
