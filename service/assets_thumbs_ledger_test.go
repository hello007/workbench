package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"workbench/model"
)

// 缩略图记账 ledger / 失效联动 / 补缺链路 / 读取 API 测试（任务 10-14-ppt-com PR2）。
//
// 测试稳定性约束（docs/spec/test-stability.md）：flusher 时序不断言（直接调
// flushLibrary 同步测）；记账时序用显式 last_access_ns 值驱动（禁 NTFS mtime
// 依赖）；ticker 周期不实测；事件等待用 chan 交接替代 sleep。

// ---------- 通用辅助 ----------

// insertThumbLedgerRow 直接插入记账行（显式 lastAccessNs 驱动 LRU 顺序断言，
// 禁依赖时间时序）。
func insertThumbLedgerRow(t *testing.T, store *assetsStore, fileID int64, tier string, page int, bytes, lastAccessNs int64) {
	t.Helper()
	if _, err := store.db.Exec(
		"INSERT INTO thumb_cache(file_id, tier, page, bytes, last_access_ns) VALUES(?, ?, ?, ?, ?)",
		fileID, tier, page, bytes, lastAccessNs,
	); err != nil {
		t.Fatalf("插入记账行(%d,%s,%d): %v", fileID, tier, page, err)
	}
}

// writeThumbPNG 创建缩略图 PNG 假文件（内容任意——记账链路只关心存在性与体积）。
func writeThumbPNG(t *testing.T, thumbsDir, tier string, fileID int64, page int, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(thumbsDir, tier), 0755); err != nil {
		t.Fatalf("建缩略图档位目录: %v", err)
	}
	p := thumbPNGPath(thumbsDir, tier, fileID, page)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("写缩略图文件: %v", err)
	}
	return p
}

// countThumbCacheRows thumb_cache 行数断言辅助。
func countThumbCacheRows(t *testing.T, store *assetsStore) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM thumb_cache").Scan(&n); err != nil {
		t.Fatalf("统计记账行数: %v", err)
	}
	return n
}

// thumbCacheBytesFor 查指定键的记账字节数（记账值断言用）。
func thumbCacheBytesFor(t *testing.T, store *assetsStore, fileID int64, tier string, page int) (int64, bool) {
	t.Helper()
	var bytes int64
	err := store.db.QueryRow(
		"SELECT bytes FROM thumb_cache WHERE file_id = ? AND tier = ? AND page = ?",
		fileID, tier, page,
	).Scan(&bytes)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		t.Fatalf("查询记账行: %v", err)
	}
	return bytes, true
}

// fileIDByPath 按相对路径查 files.id。
func fileIDByPath(t *testing.T, store *assetsStore, relPath string) int64 {
	t.Helper()
	var id int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", relPath).Scan(&id); err != nil {
		t.Fatalf("查询文件 ID(%s): %v", relPath, err)
	}
	return id
}

// insertHostFile 插入 files 宿主行（thumb_cache.file_id 外键依赖——空库直接插
// 记账会被外键约束拒绝），返回自增 ID。
func insertHostFile(t *testing.T, store *assetsStore, relPath string) int64 {
	t.Helper()
	if _, err := store.db.Exec(
		"INSERT INTO files(path, status) VALUES(?, ?)", relPath, model.AssetFileStatusOK,
	); err != nil {
		t.Fatalf("插宿主文件行(%s): %v", relPath, err)
	}
	return fileIDByPath(t, store, relPath)
}

// ---------- schema v3 迁移 ----------

// TestAssetsSchema_V2ToV3CreatesThumbCache v2 存量库升级 v3：thumb_cache 建表且
// 可读写，user_version 升至当前版本（迁移幂等性由 CREATE IF NOT EXISTS 保证）。
func TestAssetsSchema_V2ToV3CreatesThumbCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "upgrade.db")
	// 手工构造 v2 库：v1 基线表 + v2 FTS 表 + user_version=2
	v2, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("打开 v2 构造库: %v", err)
	}
	if _, err := v2.Exec(assetsSchemaSQL); err != nil {
		t.Fatalf("建 v1 基线表: %v", err)
	}
	if _, err := v2.Exec(assetsSchemaV2SQL); err != nil {
		t.Fatalf("建 v2 FTS 表: %v", err)
	}
	if _, err := v2.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatalf("置 v2 版本: %v", err)
	}
	if err := v2.Close(); err != nil {
		t.Fatalf("关闭 v2 构造库: %v", err)
	}

	store, err := openAssetsStore(dbPath)
	if err != nil {
		t.Fatalf("v2 库应可迁移打开: %v", err)
	}
	defer store.Close()
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("读版本: %v", err)
	}
	if version != assetsSchemaVersion {
		t.Fatalf("应迁移到 v%d, got v%d", assetsSchemaVersion, version)
	}
	// thumb_cache 可读写（file_id 外键指向 files，先插宿主行）
	if _, err := store.db.Exec(
		"INSERT INTO files(path, status) VALUES(?, ?)", "迁移库.pptx", model.AssetFileStatusOK,
	); err != nil {
		t.Fatalf("插文件行: %v", err)
	}
	upgradeFileID := fileIDByPath(t, store, "迁移库.pptx")
	insertThumbLedgerRow(t, store, upgradeFileID, "small", 1, 100, 1)
	if n := countThumbCacheRows(t, store); n != 1 {
		t.Fatalf("v3 迁移后 thumb_cache 应可读写，实际 %d 行", n)
	}
}

// TestAssetsSchema_FreshV3AndFkCascade 全新库直建 v3 + 外键级联兜底：删 files 行
// 时 thumb_cache 随 ON DELETE CASCADE 清除（代码显式 DELETE 为第一道，级联为兜底）。
func TestAssetsSchema_FreshV3AndFkCascade(t *testing.T) {
	store, err := openAssetsStore(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("打开新库: %v", err)
	}
	defer store.Close()
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("读版本: %v", err)
	}
	if version != assetsSchemaVersion {
		t.Fatalf("新库应直建 v%d, got v%d", assetsSchemaVersion, version)
	}
	if _, err := store.db.Exec(
		"INSERT INTO files(path, status) VALUES(?, ?)", "甲.pptx", model.AssetFileStatusOK,
	); err != nil {
		t.Fatalf("插文件行: %v", err)
	}
	fileID := fileIDByPath(t, store, "甲.pptx")
	insertThumbLedgerRow(t, store, fileID, "small", 1, 100, 1)
	if _, err := store.db.Exec("DELETE FROM files WHERE id = ?", fileID); err != nil {
		t.Fatalf("删文件行: %v", err)
	}
	if n := countThumbCacheRows(t, store); n != 0 {
		t.Fatalf("外键级联应清除记账行，残留 %d 行", n)
	}
}

// ---------- ledger 记账 / 落盘 / 失效 ----------

// TestThumbLedger_FlushPersistsAndClears 攒批落盘：record 攒批 → flushLibrary
// 单事务落盘（行数与值断言）→ 重复 flush 幂等（dirty 已清空，无重复写入）。
func TestThumbLedger_FlushPersistsAndClears(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "记账库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)
	l := svc.thumbLedger
	t.Cleanup(l.Shutdown)

	hostID := insertHostFile(t, store, "甲.pptx")
	l.record(lib.ID, hostID, "small", 1, 100)
	l.record(lib.ID, hostID, "small", 2, 200)
	l.record(lib.ID, hostID, "hd", 1, 300)
	l.flushLibrary(lib.ID)

	if n := countThumbCacheRows(t, store); n != 3 {
		t.Fatalf("落盘后应 3 行, got %d", n)
	}
	for _, tc := range []struct {
		tier string
		page int
		want int64
	}{{"small", 1, 100}, {"small", 2, 200}, {"hd", 1, 300}} {
		got, ok := thumbCacheBytesFor(t, store, hostID, tc.tier, tc.page)
		if !ok {
			t.Fatalf("记账行缺失：%+v", tc)
		}
		if got != tc.want {
			t.Errorf("记账 bytes 不符：%+v got %d", tc, got)
		}
	}
	var lastAccess int64
	if err := store.db.QueryRow(
		"SELECT last_access_ns FROM thumb_cache WHERE file_id = ? AND tier = 'small' AND page = 1", hostID,
	).Scan(&lastAccess); err != nil || lastAccess <= 0 {
		t.Fatalf("last_access_ns 应为正数（LRU 依据）: %v %d", err, lastAccess)
	}

	// 重复 flush：dirty 已清空，行数不变（幂等）
	l.flushLibrary(lib.ID)
	if n := countThumbCacheRows(t, store); n != 3 {
		t.Fatalf("重复 flush 应幂等, got %d 行", n)
	}
}

// TestThumbLedger_InvalidateFileAndDropLibrary 失效清理：invalidateFile 清该文件
// 全部条目（其余文件保留）；dropLibrary 清该库全部 dirty。
func TestThumbLedger_InvalidateFileAndDropLibrary(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "清理库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)
	l := svc.thumbLedger
	t.Cleanup(l.Shutdown)

	f1 := insertHostFile(t, store, "甲.pptx")
	f2 := insertHostFile(t, store, "乙.pptx")
	l.record(lib.ID, f1, "small", 1, 100)
	l.record(lib.ID, f1, "hd", 1, 100)
	l.record(lib.ID, f2, "small", 1, 100)
	l.invalidateFile(lib.ID, f1)
	l.flushLibrary(lib.ID)
	if n := countThumbCacheRows(t, store); n != 1 {
		t.Fatalf("invalidate 后落盘应仅剩文件乙的 1 行, got %d", n)
	}
	if _, ok := thumbCacheBytesFor(t, store, f2, "small", 1); !ok {
		t.Fatal("文件乙的记账不应被 invalidate 误清")
	}

	l.record(lib.ID, f1, "small", 1, 100)
	l.dropLibrary(lib.ID)
	l.flushLibrary(lib.ID)
	if n := countThumbCacheRows(t, store); n != 1 {
		t.Fatalf("dropLibrary 后 flush 不应写新行, got %d 行", n)
	}
}

// TestThumbLedger_FlushDropsDirtyWhenStoreUnavailable 库不可用（已删等）时
// flushLibrary 丢弃该库 dirty 仅告警：不重建孤儿 db、不 panic；后续恢复后新
// 记账正常落盘（丢弃的旧条目不复活）。
func TestThumbLedger_FlushDropsDirtyWhenStoreUnavailable(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "降级库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)

	var failStore atomic.Bool
	failStore.Store(true)
	l := newThumbCacheLedger(
		func(string) (*assetsStore, error) {
			if failStore.Load() {
				return nil, errors.New("库已删除")
			}
			return store, nil
		},
		func(string) string { return svc.assetThumbsDir(lib.ID) },
		nil,
	)
	t.Cleanup(l.Shutdown)

	dropped := insertHostFile(t, store, "被丢.pptx")
	kept := insertHostFile(t, store, "保留.pptx")
	l.record("任意库", dropped, "small", 1, 100)
	l.flushLibrary("任意库") // storeFn 报错 → dirty 丢弃（不写库）
	if n := countThumbCacheRows(t, store); n != 0 {
		t.Fatalf("库不可用时不应落盘, got %d 行", n)
	}

	failStore.Store(false)
	l.record("任意库", kept, "small", 1, 100)
	l.flushLibrary("任意库") // 恢复后正常落盘
	if _, ok := thumbCacheBytesFor(t, store, dropped, "small", 1); ok {
		t.Fatal("被丢弃的旧条目不应复活")
	}
	if _, ok := thumbCacheBytesFor(t, store, kept, "small", 1); !ok {
		t.Fatal("恢复后新记账应正常落盘")
	}
}

// TestThumbLedger_PruneBeyond 页数缩减对账：5 页记账 prune 到 3——越界记账行与
// 越界 PNG 文件清除，界内保留。
func TestThumbLedger_PruneBeyond(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "对账库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)
	thumbsDir := svc.assetThumbsDir(lib.ID)

	hostID := insertHostFile(t, store, "缩减.pptx")
	for page := 1; page <= 5; page++ {
		insertThumbLedgerRow(t, store, hostID, "small", page, 10, int64(page))
		writeThumbPNG(t, thumbsDir, "small", hostID, page, "x")
	}
	svc.thumbLedger.pruneBeyond(lib.ID, thumbsDir, hostID, "small", 3)

	if n := countThumbCacheRows(t, store); n != 3 {
		t.Fatalf("prune 后应余 3 行, got %d", n)
	}
	for page := 1; page <= 3; page++ {
		if _, ok := thumbCacheBytesFor(t, store, hostID, "small", page); !ok {
			t.Errorf("界内记账 page=%d 不应被清", page)
		}
		if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", hostID, page)); err != nil {
			t.Errorf("界内 PNG page=%d 不应被删: %v", page, err)
		}
	}
	for page := 4; page <= 5; page++ {
		if _, ok := thumbCacheBytesFor(t, store, hostID, "small", page); ok {
			t.Errorf("越界记账 page=%d 应被清", page)
		}
		if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", hostID, page)); !os.IsNotExist(err) {
			t.Errorf("越界 PNG page=%d 应被删, stat err=%v", page, err)
		}
	}
}

// TestThumbLedger_EvictTierUnderQuota 配额 LRU 淘汰：注入小配额（400B，构造时
// 注入 quotaFn 闭包——var 补丁方式会跨用例污染包级状态）+ 5 条各 100B 记账
// （显式 last_access_ns 递增定序）→ 超配额删最旧至 90% 水位（500-200=300
// ≤ 360），被淘汰条目的 PNG 文件同步删除。
func TestThumbLedger_EvictTierUnderQuota(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "配额库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)
	thumbsDir := svc.assetThumbsDir(lib.ID)

	l := newThumbCacheLedger(
		svc.storeForThumbs,
		func(string) string { return thumbsDir },
		func(string) int64 { return 400 }, // 小配额注入
	)
	t.Cleanup(l.Shutdown)

	for i := 1; i <= 5; i++ {
		id := insertHostFile(t, store, fmt.Sprintf("素材%d.pptx", i))
		insertThumbLedgerRow(t, store, id, "small", 1, 100, int64(i)) // last_access_ns = i（1 最旧）
		writeThumbPNG(t, thumbsDir, "small", id, 1, "x")
	}
	l.evictTier(store, lib.ID, thumbsDir, "small")

	if n := countThumbCacheRows(t, store); n != 3 {
		t.Fatalf("淘汰后应余 3 行（300B 不超 90%% 水位 360B）, got %d", n)
	}
	for id := int64(1); id <= 2; id++ { // 最旧 2 条被淘汰
		if _, ok := thumbCacheBytesFor(t, store, id, "small", 1); ok {
			t.Errorf("最旧记账 fileID=%d 应被淘汰", id)
		}
		if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", id, 1)); !os.IsNotExist(err) {
			t.Errorf("被淘汰条目的 PNG fileID=%d 应同步删除, stat err=%v", id, err)
		}
	}
	for id := int64(3); id <= 5; id++ { // 较新 3 条保留
		if _, ok := thumbCacheBytesFor(t, store, id, "small", 1); !ok {
			t.Errorf("较新记账 fileID=%d 不应被淘汰", id)
		}
		if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", id, 1)); err != nil {
			t.Errorf("保留条目的 PNG fileID=%d 不应被删: %v", id, err)
		}
	}
}

// TestThumbLedger_FlushFailureRequeuesDirty flush 瞬态失败批回灌（独立审核遗留
// 项 1）：storeFn 成功但落盘失败（已 Close 的 store 模拟 sql: database is closed）
// 时整批回灌 dirty，恢复后再次 flush 完整落盘；flush 窗口内同键新 record（更新鲜）
// 优先，回灌不覆盖已存在的键。
func TestThumbLedger_FlushFailureRequeuesDirty(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "回灌库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)

	// 构造已关闭 store：flushThumbEntries 首个 db 操作即报 database is closed
	// （落盘瞬态失败模拟，不依赖真实 IO 故障时序）
	dead, err := openAssetsStore(filepath.Join(t.TempDir(), "dead.db"))
	if err != nil {
		t.Fatalf("打开构造用 store: %v", err)
	}
	if err := dead.Close(); err != nil {
		t.Fatalf("关闭构造用 store: %v", err)
	}

	hostID := insertHostFile(t, store, "甲.pptx")
	var failFlush atomic.Bool
	failFlush.Store(true)
	l := newThumbCacheLedger(
		func(string) (*assetsStore, error) {
			if failFlush.Load() {
				return dead, nil
			}
			return store, nil
		},
		func(string) string { return svc.assetThumbsDir(lib.ID) },
		nil,
	)
	t.Cleanup(l.Shutdown)

	l.record(lib.ID, hostID, "small", 1, 100)
	l.record(lib.ID, hostID, "hd", 2, 200)
	l.flushLibrary(lib.ID) // 落盘失败 → 整批回灌 dirty（不写活库）
	if n := countThumbCacheRows(t, store); n != 0 {
		t.Fatalf("落盘失败时不应写入活库, got %d 行", n)
	}

	// flush 窗口内同键新 record（更新鲜）：回灌不得覆盖
	l.record(lib.ID, hostID, "small", 1, 999)

	failFlush.Store(false)
	l.flushLibrary(lib.ID) // 恢复后回灌批落盘成功
	if n := countThumbCacheRows(t, store); n != 2 {
		t.Fatalf("回灌批应完整落盘 2 行, got %d", n)
	}
	if bytes, ok := thumbCacheBytesFor(t, store, hostID, "small", 1); !ok || bytes != 999 {
		t.Fatalf("flush 窗口内同键新 record 应优先（不被回灌覆盖）, ok=%v bytes=%d", ok, bytes)
	}
	if bytes, ok := thumbCacheBytesFor(t, store, hostID, "hd", 2); !ok || bytes != 200 {
		t.Fatalf("回灌批的 hd 条目应落盘, ok=%v bytes=%d", ok, bytes)
	}
}

// TestThumbLedger_EvictSkipsDirtyKeys evict 排除 dirty 在册键（独立审核遗留项 2）：
// db 旧行 A（旧 access）/B（新 access）+ PNG 双双在盘，A 重导出/读取命中产生
// dirty（最新 last_access）未 flush；直调 evictTier——A 因 dirty 在册被排除
// （行与 PNG 保留，即将以新 last_access 落盘），B 作为唯一非 dirty 候选被回收
// （记账 + PNG 同删）。无排除时 A 会被按 db 旧行误删，形成「记账称存在而文件缺」。
func TestThumbLedger_EvictSkipsDirtyKeys(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "排除库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)
	thumbsDir := svc.assetThumbsDir(lib.ID)

	l := newThumbCacheLedger(
		svc.storeForThumbs,
		func(string) string { return thumbsDir },
		func(string) int64 { return 400 }, // 小配额：600B 总量超 90% 水位 360B
	)
	t.Cleanup(l.Shutdown)

	dirtyID := insertHostFile(t, store, "重导出.pptx") // A：db 旧行 + dirty 在册
	plainID := insertHostFile(t, store, "普通.pptx")  // B：仅 db 旧行
	insertThumbLedgerRow(t, store, dirtyID, "small", 1, 300, 1)
	writeThumbPNG(t, thumbsDir, "small", dirtyID, 1, "a")
	insertThumbLedgerRow(t, store, plainID, "small", 1, 300, 2)
	writeThumbPNG(t, thumbsDir, "small", plainID, 1, "b")
	// A 产生 dirty（flush 快照取出后并发 record / flush 窗口重导出记账的模拟）
	l.record(lib.ID, dirtyID, "small", 1, 300)

	l.evictTier(store, lib.ID, thumbsDir, "small")

	if _, ok := thumbCacheBytesFor(t, store, plainID, "small", 1); ok {
		t.Fatal("非 dirty 的 B 行应被淘汰回收")
	}
	if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", plainID, 1)); !os.IsNotExist(err) {
		t.Errorf("被淘汰 B 的 PNG 应同步删除, stat err=%v", err)
	}
	if _, ok := thumbCacheBytesFor(t, store, dirtyID, "small", 1); !ok {
		t.Fatal("dirty 在册键 A 不应被淘汰（即将以新 last_access 落盘）")
	}
	if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", dirtyID, 1)); err != nil {
		t.Errorf("dirty 在册键 A 的 PNG 不应被删: %v", err)
	}
}

// TestThumbLedger_DropTier dropTier 仅清指定库指定档 dirty，其余档保留
// （分辨率变更清档联动，防旧分辨率记账复活）。
func TestThumbLedger_DropTier(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "清档库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)
	l := svc.thumbLedger
	t.Cleanup(l.Shutdown)

	hostID := insertHostFile(t, store, "甲.pptx")
	l.record(lib.ID, hostID, "small", 1, 100)
	l.record(lib.ID, hostID, "hd", 1, 200)
	l.dropTier(lib.ID, "small")
	l.flushLibrary(lib.ID)

	if n := countThumbCacheRows(t, store); n != 1 {
		t.Fatalf("dropTier(small) 后 flush 应仅落盘 hd 1 行, got %d", n)
	}
	if _, ok := thumbCacheBytesFor(t, store, hostID, "hd", 1); !ok {
		t.Fatal("hd 档 dirty 不应被 dropTier(small) 误清")
	}
}

// TestStore_DeleteThumbCacheTier deleteThumbCacheTier 仅删指定档记账行
// （分辨率变更清档的 db 侧单点）。
func TestStore_DeleteThumbCacheTier(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "删档库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)

	hostID := insertHostFile(t, store, "甲.pptx")
	insertThumbLedgerRow(t, store, hostID, "small", 1, 100, 1)
	insertThumbLedgerRow(t, store, hostID, "small", 2, 100, 2)
	insertThumbLedgerRow(t, store, hostID, "hd", 1, 200, 1)

	if err := store.deleteThumbCacheTier("small"); err != nil {
		t.Fatalf("deleteThumbCacheTier: %v", err)
	}
	if n := countThumbCacheRows(t, store); n != 1 {
		t.Fatalf("清 small 档后应仅余 hd 1 行, got %d", n)
	}
	if _, ok := thumbCacheBytesFor(t, store, hostID, "hd", 1); !ok {
		t.Fatal("hd 档记账不应被误删")
	}
}

// TestThumbLedger_CheckQuota checkQuota 上限变更触发的一次性惰性淘汰（复用
// evictTier 语义：超配额删至水位）；库不可用静默跳过不 panic。
func TestThumbLedger_CheckQuota(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "查额库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)
	thumbsDir := svc.assetThumbsDir(lib.ID)

	l := newThumbCacheLedger(
		svc.storeForThumbs,
		func(string) string { return thumbsDir },
		func(string) int64 { return 400 },
	)
	t.Cleanup(l.Shutdown)

	for i := 1; i <= 5; i++ {
		id := insertHostFile(t, store, fmt.Sprintf("素材%d.pptx", i))
		insertThumbLedgerRow(t, store, id, "small", 1, 100, int64(i))
		writeThumbPNG(t, thumbsDir, "small", id, 1, "x")
	}
	l.checkQuota(lib.ID, "small")
	if n := countThumbCacheRows(t, store); n != 3 {
		t.Fatalf("checkQuota 应淘汰至水位（余 3 行 300B ≤ 360B）, got %d", n)
	}

	// 库不可用：storeFn 报错静默跳过（不 panic、不惰性重建孤儿 db）
	l.checkQuota("不存在的库", "small")
}

// ---------- 补缺收集 ----------

// setupBacklogFixture 补缺用例公共语料：两 pptx（甲 2 页 / 乙 1 页）扫描入库，
// 返回服务与库连接、缩略图目录。
func setupBacklogFixture(t *testing.T) (*AssetLibraryService, *assetsStore, model.AssetLibrary, string) {
	t.Helper()
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "补缺库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{
		{titlePh: "甲一", body: []string{"内容"}},
		{titlePh: "甲二", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{{titlePh: "乙一", body: []string{"内容"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	return svc, store, lib, svc.assetThumbsDir(lib.ID)
}

// fillThumbsComplete 为语料全部文件全部页全部档补齐记账与 PNG（全齐态构造）。
func fillThumbsComplete(t *testing.T, store *assetsStore, lib model.AssetLibrary, thumbsDir string) {
	t.Helper()
	for _, rel := range []string{"甲.pptx", "乙.pptx"} {
		fileID := fileIDByPath(t, store, rel)
		n := 2
		if rel == "乙.pptx" {
			n = 1
		}
		for _, tier := range thumbTiers {
			for page := 1; page <= n; page++ {
				insertThumbLedgerRow(t, store, fileID, tier.Key, page, 50, int64(page))
				writeThumbPNG(t, thumbsDir, tier.Key, fileID, page, "x")
			}
		}
	}
}

// assertBacklogFiles 断言补缺清单恰为给定文件集合（按相对路径比对，AbsPath 为
// 库根目录拼接）。
func assertBacklogFiles(t *testing.T, backlog []thumbFileSpec, lib model.AssetLibrary, want []string) {
	t.Helper()
	if len(backlog) != len(want) {
		t.Fatalf("补缺清单应为 %v, got %d 项：%+v", want, len(backlog), backlog)
	}
	got := make(map[string]thumbFileSpec, len(backlog))
	for _, b := range backlog {
		got[b.Path] = b
	}
	for _, rel := range want {
		b, ok := got[rel]
		if !ok {
			t.Fatalf("补缺清单缺 %s：%+v", rel, backlog)
		}
		if wantAbs := filepath.Join(lib.RootPath, filepath.FromSlash(rel)); b.AbsPath != wantAbs {
			t.Errorf("%s AbsPath 应为 %s, got %s", rel, wantAbs, b.AbsPath)
		}
		if b.FileID == 0 {
			t.Errorf("%s 的 FileID 应非零", rel)
		}
	}
}

// TestThumbsBacklog_CollectAllMissing 全缺：无记账无文件 → 全部 ok 文件入队。
func TestThumbsBacklog_CollectAllMissing(t *testing.T) {
	svc, store, lib, thumbsDir := setupBacklogFixture(t)
	backlog, err := svc.collectThumbsBacklog(store, lib, thumbsDir)
	if err != nil {
		t.Fatalf("collectThumbsBacklog: %v", err)
	}
	assertBacklogFiles(t, backlog, lib, []string{"甲.pptx", "乙.pptx"})
}

// TestThumbsBacklog_SkipWhenComplete 全齐：记账与 PNG 全命中 → 空清单。
func TestThumbsBacklog_SkipWhenComplete(t *testing.T) {
	svc, store, lib, thumbsDir := setupBacklogFixture(t)
	fillThumbsComplete(t, store, lib, thumbsDir)
	backlog, err := svc.collectThumbsBacklog(store, lib, thumbsDir)
	if err != nil {
		t.Fatalf("collectThumbsBacklog: %v", err)
	}
	if len(backlog) != 0 {
		t.Fatalf("全齐态应空清单, got %+v", backlog)
	}
}

// TestThumbsBacklog_MissingLedgerEntry 文件权威口径（真机冒烟抓出的幂等回归）：
// 记账 miss 但 PNG 在 → 不入队（记账攒批异步落盘，补缺判定不参与记账——幂等
// 不得依赖 flush 时序，读取路径 touch 自愈补记账）；PNG 缺失 → 入队。
func TestThumbsBacklog_MissingLedgerEntry(t *testing.T) {
	svc, store, lib, thumbsDir := setupBacklogFixture(t)
	fillThumbsComplete(t, store, lib, thumbsDir)
	jiaID := fileIDByPath(t, store, "甲.pptx")
	if _, err := store.db.Exec(
		"DELETE FROM thumb_cache WHERE file_id = ? AND tier = 'small' AND page = 2", jiaID,
	); err != nil {
		t.Fatalf("删记账行: %v", err)
	}
	// 仅记账 miss（PNG 在）：OR 口径命中，不入队
	backlog, err := svc.collectThumbsBacklog(store, lib, thumbsDir)
	if err != nil {
		t.Fatalf("collectThumbsBacklog: %v", err)
	}
	assertBacklogFiles(t, backlog, lib, nil)

	// 记账与文件双缺失：入队
	if err := os.Remove(thumbPNGPath(thumbsDir, "small", jiaID, 2)); err != nil {
		t.Fatalf("删 PNG: %v", err)
	}
	backlog, err = svc.collectThumbsBacklog(store, lib, thumbsDir)
	if err != nil {
		t.Fatalf("collectThumbsBacklog: %v", err)
	}
	assertBacklogFiles(t, backlog, lib, []string{"甲.pptx"})
}

// TestThumbsBacklog_MissingFileDespiteLedger 记账命中但 PNG 文件缺失（崩溃/手动
// 清理致记账与文件脱钩）→ 该文件入队重导出。
func TestThumbsBacklog_MissingFileDespiteLedger(t *testing.T) {
	svc, store, lib, thumbsDir := setupBacklogFixture(t)
	fillThumbsComplete(t, store, lib, thumbsDir)
	yiID := fileIDByPath(t, store, "乙.pptx")
	if err := os.Remove(thumbPNGPath(thumbsDir, "hd", yiID, 1)); err != nil {
		t.Fatalf("删 PNG 文件: %v", err)
	}
	backlog, err := svc.collectThumbsBacklog(store, lib, thumbsDir)
	if err != nil {
		t.Fatalf("collectThumbsBacklog: %v", err)
	}
	assertBacklogFiles(t, backlog, lib, []string{"乙.pptx"})
}

// TestThumbsBacklog_NonOKStatusSkipped 非 ok 状态不入队：failed 文件无可导出语义
// （无 PowerPoint 可渲染的有效源状态），即使记账文件全齐也排除在清单外。
func TestThumbsBacklog_NonOKStatusSkipped(t *testing.T) {
	svc, store, lib, thumbsDir := setupBacklogFixture(t)
	fillThumbsComplete(t, store, lib, thumbsDir)
	if _, err := store.db.Exec(
		"UPDATE files SET status = ? WHERE path = ?", model.AssetFileStatusFailed, "乙.pptx",
	); err != nil {
		t.Fatalf("篡改状态: %v", err)
	}
	backlog, err := svc.collectThumbsBacklog(store, lib, thumbsDir)
	if err != nil {
		t.Fatalf("collectThumbsBacklog: %v", err)
	}
	assertBacklogFiles(t, backlog, lib, nil)
}

// TestThumbsBacklog_UnavailableExporterSkips 无 COM 环境静默：扫描完成不触发入队
// （无事件无导出，D1 降级契约在补缺链路同样成立）。
func TestThumbsBacklog_UnavailableExporterSkips(t *testing.T) {
	svc, _, _, _ := setupBacklogFixture(t)
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)
	fake := svc.thumbQ.exporter.(*fakeThumbExporter)
	exports, _, _, _ := fake.stats()
	if exports != 0 {
		t.Fatalf("无 COM 环境不应触发导出, got %d", exports)
	}
	if len(sink.calls) != 0 {
		t.Fatalf("补缺静默跳过不应发事件, got %#v", sink.calls)
	}
}

// TestThumbsBacklog_EnqueueAfterScan 有 COM 环境：扫描完成后自动补缺入队（批次
// 经 chan 交接确定性等待执行，断言两文件全部导出）。
func TestThumbsBacklog_EnqueueAfterScan(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	svc := newTestAssetService(t)
	svc.thumbQ = newThumbQueue(fake, svc.eventSink, svc.thumbLedger, nil)
	t.Cleanup(svc.thumbLedger.Shutdown)
	t.Cleanup(func() { s := svc; s.WaitThumbsDone() })

	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "入队库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{{titlePh: "一", body: []string{"内容"}}})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{{titlePh: "一", body: []string{"内容"}}})

	exportStarted := make(chan struct{}, 4)
	releaseCh := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	defer release() // 任何失败退出路径都放行 worker（防 cleanup WaitThumbsDone 永挂）
	fake.exportFn = func(e *fakeThumbExporter, _ context.Context, _ thumbFileSpec, _ string, _ []thumbTier, _ func() *thumbJumpRequest) (int, error) {
		exportStarted <- struct{}{}
		<-releaseCh
		return 1, nil
	}

	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	// 批次串行：第 1 个文件开始导出 = 批次已入队且 worker 已消费（chan 交接 =
	// 确定性同步，禁 sleep 轮询）；放行后第 2 个文件继续进入导出——两文件均入队
	// 才证明补缺清单完整。
	select {
	case <-exportStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内无文件进入导出（补缺入队链路失效）")
	}
	release() // 幂等放行（closed chan 对后续 <-releaseCh 恒可读）
	select {
	case <-exportStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("5s 内第二个文件未进入导出（补缺清单不完整）")
	}
}

// ---------- 失效联动（重解析 / 删除 / 重建） ----------

// setupInvalidationFixture 失效联动用例公共语料：单 pptx 扫描入库 + 模拟旧缩略图
// （记账 3 行 + PNG 3 个），返回 fileID 与缩略图目录。
func setupInvalidationFixture(t *testing.T) (*AssetLibraryService, *assetsStore, model.AssetLibrary, int64, string) {
	t.Helper()
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "失效库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{{titlePh: "一", body: []string{"内容"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	fileID := fileIDByPath(t, store, "甲.pptx")
	thumbsDir := svc.assetThumbsDir(lib.ID)
	for page := 1; page <= 3; page++ {
		insertThumbLedgerRow(t, store, fileID, "small", page, 10, int64(page))
		writeThumbPNG(t, thumbsDir, "small", fileID, page, "x")
		writeThumbPNG(t, thumbsDir, "hd", fileID, page, "x")
	}
	return svc, store, lib, fileID, thumbsDir
}

// assertThumbsKept 断言指定文件的记账行与全部 PNG 保留（lost 态存续语义）。
func assertThumbsKept(t *testing.T, store *assetsStore, thumbsDir string, fileID int64) {
	t.Helper()
	var n int
	if err := store.db.QueryRow(
		"SELECT COUNT(*) FROM thumb_cache WHERE file_id = ?", fileID,
	).Scan(&n); err != nil {
		t.Fatalf("统计记账行: %v", err)
	}
	if n == 0 {
		t.Fatal("该文件的记账行应保留")
	}
	for _, tier := range thumbTiers {
		entries, err := os.ReadDir(filepath.Join(thumbsDir, tier.Key))
		if os.IsNotExist(err) {
			t.Fatalf("缩略图档位目录应存在: %s", tier.Key)
		}
		if err != nil {
			t.Fatalf("枚举缩略图目录: %v", err)
		}
		prefix := fmt.Sprintf("%d-", fileID)
		found := false
		for _, e := range entries {
			if len(e.Name()) > len(prefix) && e.Name()[:len(prefix)] == prefix {
				found = true
			}
		}
		if !found {
			t.Errorf("缩略图文件应保留: %s 档无 %d-* 文件", tier.Key, fileID)
		}
	}
}

// assertThumbsGone 断言指定文件的记账行与全部 PNG 已清除。
func assertThumbsGone(t *testing.T, store *assetsStore, thumbsDir string, fileID int64) {
	t.Helper()
	var n int
	if err := store.db.QueryRow(
		"SELECT COUNT(*) FROM thumb_cache WHERE file_id = ?", fileID,
	).Scan(&n); err != nil {
		t.Fatalf("统计记账行: %v", err)
	}
	if n != 0 {
		t.Fatalf("该文件的记账行应全部清除, 残留 %d", n)
	}
	for _, tier := range thumbTiers {
		entries, err := os.ReadDir(filepath.Join(thumbsDir, tier.Key))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("枚举缩略图目录: %v", err)
		}
		prefix := fmt.Sprintf("%d-", fileID)
		for _, e := range entries {
			if len(e.Name()) > len(prefix) && e.Name()[:len(prefix)] == prefix {
				t.Errorf("缩略图文件应已删除: %s/%s", tier.Key, e.Name())
			}
		}
	}
}

// markStaleForRescan 注入陈旧记录（mtime 回拨 1 小时）驱动重解析（对齐
// docs/spec/test-stability.md 的「注入陈旧缓存」范式，禁 NTFS mtime 时序依赖）。
func markStaleForRescan(t *testing.T, store *assetsStore, fileID int64) {
	t.Helper()
	if _, err := store.db.Exec(
		"UPDATE files SET mtime_ns = mtime_ns - 3600000000000 WHERE id = ?", fileID,
	); err != nil {
		t.Fatalf("注入陈旧记录: %v", err)
	}
}

// TestThumbInvalidation_OnRescanUpdated 重解析成功路径：旧记账与旧 PNG 全部清除
// （文件内容已变旧图必失效——失效发生在解析之前，成败两分支均生效）。
func TestThumbInvalidation_OnRescanUpdated(t *testing.T) {
	svc, store, lib, fileID, thumbsDir := setupInvalidationFixture(t)
	markStaleForRescan(t, store, fileID)

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("重扫: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("应按变更重处理, got %+v", result)
	}
	assertThumbsGone(t, store, thumbsDir, fileID)
}

// TestThumbInvalidation_OnRescanParseFailed 解析失败路径：内容已变旧图必失效，
// 重扫降级 failed 状态后旧记账与旧 PNG 同样清除（失效先于解析的位置语义验证）。
func TestThumbInvalidation_OnRescanParseFailed(t *testing.T) {
	svc, store, lib, fileID, thumbsDir := setupInvalidationFixture(t)
	markStaleForRescan(t, store, fileID)
	svc.parseFn = func(string, string) ([]parsedSlide, error) {
		return nil, errors.New("内容已损坏")
	}

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("重扫（失败降级不中断批量）: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("应降级 failed, got %+v", result)
	}
	assertThumbsGone(t, store, thumbsDir, fileID)
}

// TestThumbInvalidation_OnFileDeleted 文件删除联动（SRS §2.6.5 lost 态语义迁移）：
// 磁盘文件消失 → 扫描标记 lost，记账行与 PNG 全保留（不再清除）；「清理丢失
// 记录」手动入口执行旧删除语义（记账事务清除 + PNG 事后删除）。
func TestThumbInvalidation_OnFileDeleted(t *testing.T) {
	svc, store, lib, fileID, thumbsDir := setupInvalidationFixture(t)
	if err := os.Remove(filepath.Join(lib.RootPath, "甲.pptx")); err != nil {
		t.Fatalf("删除源文件: %v", err)
	}

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("重扫: %v", err)
	}
	if result.Lost != 1 || result.Removed != 0 {
		t.Fatalf("应标记 1 条丢失且不删除, got %+v", result)
	}
	// lost 态：记账与 PNG 保留（夹具 1 文件 3 页 small 档 = 3 行记账）
	if n := countThumbCacheRows(t, store); n != 3 {
		t.Fatalf("lost 态记账行应保留 3 行, got %d", n)
	}
	assertThumbsKept(t, store, thumbsDir, fileID)

	// 清理丢失记录：等效旧删除语义
	removed, err := svc.ClearLostAssetFiles(lib.ID)
	if err != nil || removed != 1 {
		t.Fatalf("清理丢失记录: removed=%d err=%v", removed, err)
	}
	assertThumbsGone(t, store, thumbsDir, fileID)
}

// TestThumbInvalidation_OnRebuild 全量重建联动：记账表清空 + thumbs 目录整体
// 清空（重建语义逐条保留无意义）。
func TestThumbInvalidation_OnRebuild(t *testing.T) {
	svc, store, _, _, thumbsDir := setupInvalidationFixture(t)

	if _, err := svc.RebuildScan(context.Background(), svc.libs0ID()); err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}
	if n := countThumbCacheRows(t, store); n != 0 {
		t.Fatalf("重建后记账表应清空, got %d 行", n)
	}
	if _, err := os.ReadDir(thumbsDir); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(thumbsDir)
		t.Fatalf("重建后 thumbs 目录应整体清空, err=%v 残留=%d", err, len(entries))
	}
}

// libs0ID 取服务首个注册库 ID（失效联动语料只有单库；测试辅助，绕开 getLibrary
// 的锁语义直接读内存态）。
func (s *AssetLibraryService) libs0ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.libs[0].ID
}

// ---------- 读取 API ----------

// TestAssetThumb_ReadAndTouch 读取命中（base64 解码与原字节一致 + 记账 touch 落盘）、
// 未命中空串非错误、非法参数报错。
func TestAssetThumb_ReadAndTouch(t *testing.T) {
	svc, store, lib, thumbsDir := setupBacklogFixture(t)
	t.Cleanup(svc.thumbLedger.Shutdown) // 命中路径 record 会启动 flusher，收口 goroutine
	jiaID := fileIDByPath(t, store, "甲.pptx")

	// 未命中：("", nil)——前端回退占位卡，非错误
	got, err := svc.GetAssetThumb(lib.ID, jiaID, 1, 0)
	if got != "" || err != nil {
		t.Fatalf("未命中应返回空串非错误, got %q err=%v", got, err)
	}
	if n := countThumbCacheRows(t, store); n != 0 {
		t.Fatalf("未命中不应记账, got %d 行", n)
	}

	// 命中 small（tierIndex=0）：base64 解码与原字节一致（二进制保真，含非 ASCII）
	content := "\x89PNG-fake-bytes-\xe4\xb8\xad\xe6\x96\x87"
	p := writeThumbPNG(t, thumbsDir, "small", jiaID, 1, content)
	_ = p
	got, err = svc.GetAssetThumb(lib.ID, jiaID, 1, 0)
	if err != nil {
		t.Fatalf("命中读取: %v", err)
	}
	decoded, decErr := base64.StdEncoding.DecodeString(got)
	if decErr != nil {
		t.Fatalf("返回应为标准 base64: %v", decErr)
	}
	if string(decoded) != content {
		t.Fatalf("base64 解码应与原字节一致:\n got %q\nwant %q", decoded, content)
	}

	// 命中 hd（tierIndex=1）
	writeThumbPNG(t, thumbsDir, "hd", jiaID, 2, "hd-content")
	if got, err = svc.GetAssetThumb(lib.ID, jiaID, 2, 1); err != nil || got == "" {
		t.Fatalf("hd 档命中应成功, got %q err=%v", got, err)
	}

	// 记账 touch：flush 后落盘（bytes = PNG 字节数）
	svc.thumbLedger.flushLibrary(lib.ID)
	if bytes, ok := thumbCacheBytesFor(t, store, jiaID, "small", 1); !ok || bytes != int64(len(content)) {
		t.Fatalf("读取命中应记账 touch（bytes=%d）, got ok=%v bytes=%d", len(content), ok, bytes)
	}
	if _, ok := thumbCacheBytesFor(t, store, jiaID, "hd", 2); !ok {
		t.Fatal("hd 档命中也应记账")
	}

	// 非法参数
	for _, tc := range []struct {
		name           string
		fileID         int64
		slideNum, tier int
	}{{"档位越界上", jiaID, 1, len(thumbTiers)}, {"档位负", jiaID, 1, -1},
		{"页序 0", jiaID, 0, 0}, {"页序负", jiaID, -2, 0},
		{"文件 ID 0", 0, 1, 0}, {"文件 ID 负", -5, 1, 0}} {
		if _, err := svc.GetAssetThumb(lib.ID, tc.fileID, tc.slideNum, tc.tier); err == nil {
			t.Errorf("%s 应报错（%s=%d tier=%d）", tc.name, tc.name, tc.fileID, tc.tier)
		}
	}
	// 库不存在
	if _, err := svc.GetAssetThumb("不存在的库", jiaID, 1, 0); err == nil {
		t.Error("库不存在应报错")
	}
}

// ---------- 队列满 error 事件 ----------

// errorNotifySink 包装 fakeEventSink：assets-thumbs:error 到达时向 chan 通知
// （chan 交接替代 sleep 轮询）。
type errorNotifySink struct {
	inner *fakeEventSink
	errCh chan map[string]any
}

func (s *errorNotifySink) Emit(name string, data ...any) {
	s.inner.Emit(name, data...)
	if name == "assets-thumbs:error" {
		if m, ok := data[0].(map[string]any); ok {
			s.errCh <- m
		}
	}
}

// TestThumbQueue_EnqueueFullEmitsErrorEvent 队列满分支补发 error 事件（载荷
// {libraryId, message}，沿用 assets-scan:error 契约）：首个批次阻塞 worker 后
// 逐一入队直至撞满缓冲（batchCh 容量 8 + worker 在途 1），确定性等待事件到达。
func TestThumbQueue_EnqueueFullEmitsErrorEvent(t *testing.T) {
	fake := &fakeThumbExporter{available: true}
	s := NewAssetLibraryService(nil, filepath.Join(t.TempDir(), "assets"))
	sink := &errorNotifySink{inner: &fakeEventSink{}, errCh: make(chan map[string]any, 4)}
	s.SetEventSink(sink)
	s.thumbQ = newThumbQueue(fake, s.eventSink, s.thumbLedger, nil)
	t.Cleanup(s.thumbLedger.Shutdown)
	t.Cleanup(func() { s.WaitThumbsDone() })

	block := make(chan struct{})
	fake.exportFn = func(e *fakeThumbExporter, _ context.Context, _ thumbFileSpec, _ string, _ []thumbTier, _ func() *thumbJumpRequest) (int, error) {
		<-block
		return 1, nil
	}

	thumbsDir := t.TempDir()
	var emitted map[string]any
	for i := 1; i <= 12; i++ {
		s.EnqueueAssetThumbs(fmt.Sprintf("lib%02d", i), thumbSpecs(1), thumbsDir)
		select {
		case m := <-sink.errCh:
			emitted = m
			i = 100 // 事件已到达，停止入队
		default:
		}
	}
	if emitted == nil {
		t.Fatal("连续入队应触发队列满 error 事件")
	}
	if emitted["libraryId"] == nil || emitted["libraryId"] == "" {
		t.Errorf("error 载荷应带 libraryId: %#v", emitted)
	}
	if msg, _ := emitted["message"].(string); msg == "" {
		t.Errorf("error 载荷应带用户可读 message: %#v", emitted)
	}

	close(block) // 放行全部批次快速完成，cleanup WaitThumbsDone 收口
}
