package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
)

// ---------- 保存检索条件测试（SRS §2.5，data/asset_saved_searches.json CRUD） ----------

// newSavedSearchTestService 构造注入临时路径的保存条件服务（无缓存、每次读盘，
// 持久化断言直接经新构造实例读回验证）。
func newSavedSearchTestService(t *testing.T) (*AssetSavedSearchService, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "asset_saved_searches.json")
	return NewAssetSavedSearchService(path), path
}

// TestAssetSavedSearch_SaveListDelete 保存/列表/删除闭环：新增生成自增 ID 与
// createdAt（RFC3339）；列表返回深拷贝（改快照不影响内存态）；删除后列表不含，
// 删除不存在条目报错。
func TestAssetSavedSearch_SaveListDelete(t *testing.T) {
	svc, _ := newSavedSearchTestService(t)

	saved, err := svc.Save(model.AssetSavedSearch{
		Name:            "城商行重点",
		LibraryID:       "lib-a",
		Keyword:         "城商行",
		TagIDs:          []int64{1, 2},
		TagNames:        []string{"重点", "渠道"},
		Ratings:         []int64{0, 4},
		MtimeFrom:       1700000000_000000000,
		SizeFrom:        1024,
		Flatten:         true,
		SlideTagOnlyOwn: true,
	})
	if err != nil {
		t.Fatalf("Save 新增: %v", err)
	}
	if saved.ID != 1 {
		t.Errorf("新增应生成自增 ID=1，实际 %d", saved.ID)
	}
	if _, err := time.Parse(time.RFC3339, saved.CreatedAt); err != nil {
		t.Errorf("createdAt 应为 RFC3339 格式，实际 %q: %v", saved.CreatedAt, err)
	}

	// 覆盖：同 ID 更新，保留原 createdAt
	updated, err := svc.Save(model.AssetSavedSearch{
		ID:        saved.ID,
		Name:      "城商行重点",
		LibraryID: "lib-a",
		Keyword:   "获客",
		Ratings:   []int64{3},
	})
	if err != nil {
		t.Fatalf("Save 覆盖: %v", err)
	}
	if updated.ID != saved.ID || updated.Keyword != "获客" || updated.CreatedAt != saved.CreatedAt {
		t.Errorf("覆盖应保 ID 与 createdAt、更新字段，实际：%+v", updated)
	}

	// 二次新增：自增 ID 递增
	second, err := svc.Save(model.AssetSavedSearch{Name: "另一条", LibraryID: "lib-b"})
	if err != nil {
		t.Fatalf("Save 二次新增: %v", err)
	}
	if second.ID != 2 {
		t.Errorf("第二次新增 ID 应为 2，实际 %d", second.ID)
	}

	// 列表：返回全部，修改快照不影响服务内存态
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(list))
	}
	list[0].Keyword = "篡改"
	again, _ := svc.List()
	if again[0].Keyword == "篡改" {
		t.Error("List 应返回快照，修改返回值不得影响服务内存态")
	}

	// 删除后列表不含；删除不存在报错
	if err := svc.Delete(saved.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ = svc.List()
	if len(list) != 1 || list[0].ID != second.ID {
		t.Errorf("删除后应剩 1 条（ID=%d），实际：%+v", second.ID, list)
	}
	if err := svc.Delete(saved.ID); err == nil {
		t.Error("删除不存在的条目应报错")
	}
}

// TestAssetSavedSearch_NameValidation 名称校验与重名拒绝：空白名拒绝；不同条目
// 重名拒绝；覆盖自身现名不拒绝（重名判定排除自身 ID）。
func TestAssetSavedSearch_NameValidation(t *testing.T) {
	svc, _ := newSavedSearchTestService(t)

	if _, err := svc.Save(model.AssetSavedSearch{Name: "   "}); err == nil {
		t.Error("空白名称应拒绝")
	}
	first, err := svc.Save(model.AssetSavedSearch{Name: "同名条件", LibraryID: "lib-a"})
	if err != nil {
		t.Fatalf("Save first: %v", err)
	}
	if _, err := svc.Save(model.AssetSavedSearch{Name: "同名条件", LibraryID: "lib-b"}); err == nil {
		t.Error("不同条目重名应拒绝")
	} else if !strings.Contains(err.Error(), "同名") {
		t.Errorf("重名错误应含「同名」引导，实际 %v", err)
	}
	// 覆盖自身现名不拒绝（排除自身 ID）
	if _, err := svc.Save(model.AssetSavedSearch{ID: first.ID, Name: "同名条件", Keyword: "x"}); err != nil {
		t.Errorf("覆盖自身现名不应拒绝: %v", err)
	}
	// 覆盖为他人现名拒绝
	second, err := svc.Save(model.AssetSavedSearch{Name: "第二条"})
	if err != nil {
		t.Fatalf("Save second: %v", err)
	}
	if _, err := svc.Save(model.AssetSavedSearch{ID: second.ID, Name: "同名条件"}); err == nil {
		t.Error("覆盖为他人现名应拒绝")
	}
	// 更新不存在的 ID 报错
	if _, err := svc.Save(model.AssetSavedSearch{ID: 999, Name: "幽灵"}); err == nil {
		t.Error("更新不存在的 ID 应报错")
	}
}

// TestAssetSavedSearch_PersistenceAcrossInstances 持久化闭环：Save 落盘后用新
// 构造实例（同 configPath）读回一致（原子写 + 无缓存直读路径验证）。
func TestAssetSavedSearch_PersistenceAcrossInstances(t *testing.T) {
	svc, path := newSavedSearchTestService(t)
	saved, err := svc.Save(model.AssetSavedSearch{
		Name:      "跨实例条件",
		LibraryID: "lib-a",
		Keyword:   "评审",
		TagNames:  []string{"审核"},
		Ratings:   []int64{2},
		Flatten:   true,
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	reborn := NewAssetSavedSearchService(path)
	list, err := reborn.List()
	if err != nil {
		t.Fatalf("新实例 List: %v", err)
	}
	if len(list) != 1 || list[0].ID != saved.ID || list[0].Keyword != "评审" || list[0].Name != "跨实例条件" {
		t.Errorf("新实例读回应与保存一致，实际：%+v", list)
	}
}

// TestAssetSavedSearch_CorruptFileDegrades 文件损坏降级：非法 JSON 读回空列表不
// 报错（复用 SettingsService 损坏降级模式），降级后 Save 以空列表重建文件可继续用。
func TestAssetSavedSearch_CorruptFileDegrades(t *testing.T) {
	svc, path := newSavedSearchTestService(t)
	if err := os.WriteFile(path, []byte("{这不是合法 JSON"), 0644); err != nil {
		t.Fatalf("写入损坏文件: %v", err)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("损坏文件 List 不应报错: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("损坏文件应降级为空列表，实际 %d 条", len(list))
	}

	// 降级后可继续保存（以空列表重建），且不与幽灵内容混合
	saved, err := svc.Save(model.AssetSavedSearch{Name: "重建首条"})
	if err != nil {
		t.Fatalf("降级后 Save: %v", err)
	}
	if saved.ID != 1 {
		t.Errorf("降级后新增 ID 应从 1 起，实际 %d", saved.ID)
	}
	list, _ = svc.List()
	if len(list) != 1 || list[0].Name != "重建首条" {
		t.Errorf("降级后保存应重建文件，实际：%+v", list)
	}
}

// TestAssetSavedSearch_AppDelegation App 委托方法透传面（app_assets.go 三个绑定
// 方法消费的服务层语义：List/Save upsert/Delete，此处锚定服务层可直接支撑绑定层
// 的最小契约形态）。
func TestAssetSavedSearch_AppDelegation(t *testing.T) {
	svc, _ := newSavedSearchTestService(t)
	if _, err := svc.Save(model.AssetSavedSearch{Name: "委托目标"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("应返回 1 条，实际 %d", len(list))
	}
	if err := svc.Delete(list[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if list, _ = svc.List(); len(list) != 0 {
		t.Error("删除后应空列表")
	}
}
