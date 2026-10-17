package service

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// 拖拽排序与移动父级（素材管理第六批真机增量，PRD D8#1）+ schema v6 迁移。
//
// 覆盖面：同级排序（before/after 落点 index 语义 + 组内序号连续化）、跨父移动
// （目标组插位 + 旧组剩余兄弟连续化）、防环与参数守卫（自身/后代/父不存在/
// 目标不存在/越界钳制/库不存在）、v6 迁移（旧 v5 库补列 + 按迁移前名称序回填
// 组内序号、新库直建含列）。
//
// fixture 与辅助复用 assets_tags_test.go（mustCreateLibrary/mustCreateTag/
// assetTagListNames）；无 sleep/mtime 依赖（docs/spec/test-stability.md）。

// TestAssetTagMove_SameParentReorder 同级排序：before/after 落点换算后的 index
// 语义（落位后自身在组内的最终序号）生效，整组序号连续化重排 0..n-1（无断号）。
func TestAssetTagMove_SameParentReorder(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "同级排序库", t.TempDir())
	// 创建序即初始组序：甲(0) 乙(1) 丙(2)
	jia := mustCreateTag(t, svc, lib.ID, "标签甲", 0, "")
	mustCreateTag(t, svc, lib.ID, "标签乙", 0, "")
	bing := mustCreateTag(t, svc, lib.ID, "标签丙", 0, "")

	// 丙拖到首位（before 甲 → index=0）：[丙 甲 乙]
	if err := svc.MoveTag(lib.ID, bing.ID, 0, 0); err != nil {
		t.Fatalf("MoveTag 移到首位: %v", err)
	}
	items, err := svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if got := assetTagListNames(items); len(got) != 3 || got[0] != "标签丙" || got[1] != "标签甲" || got[2] != "标签乙" {
		t.Fatalf("同级首位应得 [丙 甲 乙]，实际 %v", got)
	}
	for i, want := range []int{0, 1, 2} {
		if items[i].SortOrder != want {
			t.Errorf("「%s」序号应连续化为 %d，实际 %d", items[i].Name, want, items[i].SortOrder)
		}
	}

	// 甲拖到组尾（after 乙 → index=2）：[丙 乙 甲]
	if err := svc.MoveTag(lib.ID, jia.ID, 0, 2); err != nil {
		t.Fatalf("MoveTag 移到组尾: %v", err)
	}
	items, err = svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if got := assetTagListNames(items); got[0] != "标签丙" || got[1] != "标签乙" || got[2] != "标签甲" {
		t.Errorf("组尾落位应得 [丙 乙 甲]，实际 %v", got)
	}
	if items[0].ParentID != 0 || items[2].ParentID != 0 {
		t.Errorf("同级排序不应变更父级：%+v", items)
	}
}

// TestAssetTagMove_CrossParent 跨父移动：目标组按 index 插位 + 组内连续化，旧组
// 剩余兄弟同样连续化重排（消除断号）。
func TestAssetTagMove_CrossParent(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "跨父移动库", t.TempDir())
	hangye := mustCreateTag(t, svc, lib.ID, "行业", 0, "") // 根组 0
	mustCreateTag(t, svc, lib.ID, "方案", 0, "")           // 根组 1
	mubiao := mustCreateTag(t, svc, lib.ID, "目标", 0, "") // 根组 2
	mustCreateTag(t, svc, lib.ID, "城商行", hangye.ID, "")  // 行业子组 0

	// 「目标」移入「行业」且排在「城商行」之后（after 落点 index=1）：
	// 行业子组 → [城商行 目标]，根组剩余 [行业 方案] 连续化 0..1
	if err := svc.MoveTag(lib.ID, mubiao.ID, hangye.ID, 1); err != nil {
		t.Fatalf("MoveTag 跨父: %v", err)
	}
	items, err := svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	// 平铺序 = 根组（行业 方案）+ 行业子组（城商行 目标）
	wantOrder := []string{"行业", "方案", "城商行", "目标"}
	wantSort := []int{0, 1, 0, 1}
	if got := assetTagListNames(items); len(got) != 4 {
		t.Fatalf("应有 4 个标签，实际 %v", got)
	}
	for i := range wantOrder {
		if items[i].Name != wantOrder[i] || items[i].SortOrder != wantSort[i] {
			t.Errorf("平铺序[%d] 应为 %s(序号 %d)，实际 %s(序号 %d)", i,
				wantOrder[i], wantSort[i], items[i].Name, items[i].SortOrder)
		}
	}

	// 移回根（index=0）：根组 → [目标 行业 方案]，行业子组剩余 [城商行] 连续化 0
	if err := svc.MoveTag(lib.ID, mubiao.ID, 0, 0); err != nil {
		t.Fatalf("MoveTag 移回根: %v", err)
	}
	items, err = svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	wantOrder = []string{"目标", "行业", "方案", "城商行"}
	wantSort = []int{0, 1, 2, 0}
	for i := range wantOrder {
		if items[i].Name != wantOrder[i] || items[i].SortOrder != wantSort[i] {
			t.Errorf("移回根后平铺序[%d] 应为 %s(序号 %d)，实际 %s(序号 %d)", i,
				wantOrder[i], wantSort[i], items[i].Name, items[i].SortOrder)
		}
	}
}

// TestAssetTagMove_Guards 防环与参数守卫：父选自身/自身后代拒绝（沿用
// updateTagFull 链路语义）、父不存在与目标不存在拒绝、index 越界钳制、库不存在
// 走 E_ASSETS_LIB_NOT_FOUND 语义。
func TestAssetTagMove_Guards(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "移动守卫库", t.TempDir())
	root := mustCreateTag(t, svc, lib.ID, "行业", 0, "")
	child := mustCreateTag(t, svc, lib.ID, "城商行", root.ID, "")
	grand := mustCreateTag(t, svc, lib.ID, "对公", child.ID, "")

	// 直接环：父选自身
	if err := svc.MoveTag(lib.ID, root.ID, root.ID, 0); err == nil || !strings.Contains(err.Error(), "自身") {
		t.Errorf("父选自身应拒绝，实际 err=%v", err)
	}
	// 间接环：移入自身子代/孙代
	if err := svc.MoveTag(lib.ID, root.ID, child.ID, 0); err == nil || !strings.Contains(err.Error(), "循环") {
		t.Errorf("移入子代（间接环）应拒绝，实际 err=%v", err)
	}
	if err := svc.MoveTag(lib.ID, root.ID, grand.ID, 0); err == nil || !strings.Contains(err.Error(), "循环") {
		t.Errorf("移入孙代（间接环）应拒绝，实际 err=%v", err)
	}
	// 父不存在 / 目标不存在
	if err := svc.MoveTag(lib.ID, root.ID, 99999, 0); err == nil || !strings.Contains(err.Error(), "父标签不存在") {
		t.Errorf("父不存在应拒绝，实际 err=%v", err)
	}
	if err := svc.MoveTag(lib.ID, 99999, 0, 0); err == nil || !strings.Contains(err.Error(), "标签不存在") {
		t.Errorf("目标不存在应拒绝，实际 err=%v", err)
	}
	// 库不存在
	if err := svc.MoveTag("不存在的库", root.ID, 0, 0); !errors.Is(err, errAssetLibraryNotFound) {
		t.Errorf("库不存在应返回 E_ASSETS_LIB_NOT_FOUND 语义，实际 err=%v", err)
	}

	// 环校验均拒绝未落位：根组现序仍为 [行业]（唯一根，城商行/对公在其子树）
	// index 越界钳制：负值钳 0（单元素组序号不变）、超组尾钳组尾（防御，正常
	// 前端落点不越界）
	if err := svc.MoveTag(lib.ID, root.ID, 0, -5); err != nil {
		t.Fatalf("负 index 应钳到 0: %v", err)
	}
	items, err := svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if items[0].Name != "行业" || items[0].SortOrder != 0 {
		t.Errorf("负 index 钳 0 应保持组首序号 0，实际 %v", assetTagListNames(items))
	}
	if err := svc.MoveTag(lib.ID, root.ID, 0, 99); err != nil {
		t.Fatalf("超界 index 应钳到组尾: %v", err)
	}
	items, err = svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if items[0].Name != "行业" || items[0].SortOrder != 0 {
		t.Errorf("单元素组移到组尾应保持序号 0，实际 %v", assetTagListNames(items))
	}
}

// ---------- schema v6 迁移（sort_order 列 + 名称序回填） ----------

// TestAssetsStore_MigrateV6 迁移链：旧 v5 库升级后 tags 补 sort_order 列并按
// 迁移前展示序（同父内按名称升序、同名按 id）回填组内序号——升级后兄弟展示序
// 与 v5 的 ORDER BY name 视觉顺序一致，用户再拖拽自定义；新库直建即含列。
func TestAssetsStore_MigrateV6(t *testing.T) {
	t.Run("旧库 v5 升级按名称序回填", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "v5.db")
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
		if err != nil {
			t.Fatalf("打开原始库: %v", err)
		}
		for _, v := range []int{1, 2, 3, 4, 5} {
			if _, err := db.Exec(assetsMigrations[v]); err != nil {
				t.Fatalf("手工执行 v%d 迁移: %v", v, err)
			}
		}
		if _, err := db.Exec("PRAGMA user_version = 5"); err != nil {
			t.Fatalf("钉版本: %v", err)
		}
		// 插入序与名称序刻意错开（丙 1 → 甲 2 → 乙 3 → 根二 4），子标签挂「丙」下：
		// 回填后根组序应为 甲(0) 乙(1) 根二(2) 丙(3)（对齐 v5 的 ORDER BY name），
		// 丙子组 [子一] 序 0
		for _, name := range []string{"丙", "甲", "乙", "根二"} {
			if _, err := db.Exec("INSERT INTO tags(name, color, parent_id) VALUES(?, '', NULL)", name); err != nil {
				t.Fatalf("插入根标签 %s: %v", name, err)
			}
		}
		var bingID int64
		if err := db.QueryRow("SELECT id FROM tags WHERE name = '丙'").Scan(&bingID); err != nil {
			t.Fatalf("读丙 ID: %v", err)
		}
		if _, err := db.Exec("INSERT INTO tags(name, color, parent_id) VALUES('子一', '', ?)", bingID); err != nil {
			t.Fatalf("插入子标签: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("关闭原始库: %v", err)
		}

		store, err := openAssetsStore(dbPath)
		if err != nil {
			t.Fatalf("升级打开: %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })

		var version int
		if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != assetsSchemaVersion {
			t.Errorf("版本应钉到 v%d，实际 v%d（err=%v）", assetsSchemaVersion, version, err)
		}
		type tagRow struct {
			name      string
			sortOrder int
		}
		rows, err := store.db.Query(
			"SELECT name, sort_order FROM tags ORDER BY COALESCE(parent_id, 0), sort_order, id",
		)
		if err != nil {
			t.Fatalf("读迁移后标签: %v", err)
		}
		defer rows.Close()
		got := make([]tagRow, 0)
		for rows.Next() {
			var r tagRow
			if err := rows.Scan(&r.name, &r.sortOrder); err != nil {
				t.Fatalf("扫描行: %v", err)
			}
			got = append(got, r)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("遍历行: %v", err)
		}
		want := []tagRow{
			// 根组按名称序 = SQLite BINARY（UTF-8 字节序，与 v5 ORDER BY name 同口径）：
			// 丙(E4B899) < 乙(E4B999) < 根二(E6A0B9) < 甲(E794B2)
			{"丙", 0}, {"乙", 1}, {"根二", 2}, {"甲", 3},
			{"子一", 0}, // 丙子组
		}
		if len(got) != len(want) {
			t.Fatalf("应有 %d 行，实际 %v", len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("回填序[%d] 应为 %s(序号 %d)，实际 %s(序号 %d)", i,
					want[i].name, want[i].sortOrder, got[i].name, got[i].sortOrder)
			}
		}
	})

	t.Run("新库直建含排序列", func(t *testing.T) {
		store, err := openAssetsStore(filepath.Join(t.TempDir(), "fresh-v6.db"))
		if err != nil {
			t.Fatalf("新库打开: %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		if _, err := store.db.Exec("INSERT INTO tags(name) VALUES('新库标签')"); err != nil {
			t.Fatalf("插标签: %v", err)
		}
		var sortOrder int
		if err := store.db.QueryRow("SELECT sort_order FROM tags WHERE name = '新库标签'").Scan(&sortOrder); err != nil {
			t.Fatalf("新库 tags 表应含 sort_order 列: %v", err)
		}
		if sortOrder != 0 {
			t.Errorf("DEFAULT 0 应回填 0，实际 %d", sortOrder)
		}
	})
}
