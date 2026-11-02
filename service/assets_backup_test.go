package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// ---------- 素材库备份域测试（SRS §2.7.5，素材管理第十批 PR2） ----------
//
// 时钟注入纪律（docs/spec/test-stability.md）：备份文件名秒级时间戳存在同秒冲突，
// 保留淘汰断言依赖时间推进——统一注入 assetBackupClock 假时钟驱动，禁 sleep 等
// 真实跨秒。用例串行运行（不 t.Parallel），假时钟经 t.Cleanup 还原。

// freezeBackupClock 注入固定时钟（同秒深序后缀用例）。
func freezeBackupClock(t *testing.T, at time.Time) {
	t.Helper()
	assetBackupClock = func() time.Time { return at }
	t.Cleanup(func() { assetBackupClock = time.Now })
}

// steppingBackupClock 注入每次调用推进 1s 的假时钟（保留淘汰用例：N 次备份产出
// N 个逐秒递增的时间戳文件名）。
func steppingBackupClock(t *testing.T, start time.Time) {
	t.Helper()
	current := start
	assetBackupClock = func() time.Time {
		current = current.Add(time.Second)
		return current
	}
	t.Cleanup(func() { assetBackupClock = time.Now })
}

// occupyBackupsDir 注入备份失败场景：以同名文件占用备份目录路径，使 createAssetBackup
// 的 os.MkdirAll 确定性失败（不依赖磁盘故障/权限等环境状态）。
func occupyBackupsDir(t *testing.T, svc *AssetLibraryService, libraryID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(svc.assetBackupsDir(libraryID)), 0o755); err != nil {
		t.Fatalf("预建备份父目录: %v", err)
	}
	if err := os.WriteFile(svc.assetBackupsDir(libraryID), []byte("occupied"), 0o644); err != nil {
		t.Fatalf("占用备份目录路径: %v", err)
	}
}

// TestAssetBackup_Manual 手动备份主链路：备份文件落备份目录、命名符合时间戳形态、
// 独立打开完整性通过、条目字段（文件名/时间/大小/路径）自洽。
func TestAssetBackup_Manual(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "手动备份库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)
	freezeBackupClock(t, time.Date(2026, 10, 18, 10, 0, 0, 0, time.Local))

	entry, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("BackupAssetLibrary: %v", err)
	}
	if entry.LibraryID != lib.ID {
		t.Errorf("条目库 ID 不符：%s", entry.LibraryID)
	}
	if entry.FileName != "20261018-100000.db" {
		t.Errorf("备份文件名应为时间戳形态，实际 %q", entry.FileName)
	}
	if entry.CreatedAt != "2026-10-18 10:00:00" {
		t.Errorf("CreatedAt 应与文件名时间戳一致，实际 %q", entry.CreatedAt)
	}
	if entry.SizeBytes <= 0 {
		t.Errorf("备份文件大小应大于 0，实际 %d", entry.SizeBytes)
	}
	if entry.Path != filepath.Join(svc.assetBackupsDir(lib.ID), "20261018-100000.db") {
		t.Errorf("备份路径应在备份目录内，实际 %q", entry.Path)
	}
	// 备份文件可独立打开且 integrity_check 通过（校验走独立连接，非在管连接）
	if err := verifySQLiteIntegrity(entry.Path); err != nil {
		t.Errorf("备份文件完整性校验应通过: %v", err)
	}

	// 清单：恰 1 条、字段与备份条目一致
	lists, err := svc.ListAssetLibraryBackups(lib.ID)
	if err != nil {
		t.Fatalf("ListAssetLibraryBackups: %v", err)
	}
	if len(lists) != 1 || lists[0].FileName != entry.FileName || lists[0].SizeBytes != entry.SizeBytes {
		t.Errorf("备份清单应恰含本次备份，实际：%+v", lists)
	}
}

// TestAssetBackup_Retention 保留淘汰：第 6 份触发淘汰最旧，保留恒 5 份（清单按
// 备份时间倒序）。
func TestAssetBackup_Retention(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "保留淘汰库", t.TempDir())
	steppingBackupClock(t, time.Date(2026, 10, 18, 10, 0, 0, 0, time.Local))

	for i := 0; i < 6; i++ {
		if _, err := svc.BackupAssetLibrary(lib.ID); err != nil {
			t.Fatalf("第 %d 次备份: %v", i+1, err)
		}
	}
	names, err := listBackupFileNames(svc.assetBackupsDir(lib.ID))
	if err != nil {
		t.Fatalf("枚举备份目录: %v", err)
	}
	if len(names) != 5 {
		t.Fatalf("应保留最近 5 份，实际 %d 份：%v", len(names), names)
	}
	sort.Strings(names)
	if names[0] != "20261018-100002.db" {
		t.Errorf("最旧的 100001 应被淘汰，剩余最早应为 100002，实际 %q", names[0])
	}

	// 清单倒序（最新在前）
	lists, err := svc.ListAssetLibraryBackups(lib.ID)
	if err != nil {
		t.Fatalf("ListAssetLibraryBackups: %v", err)
	}
	if len(lists) != 5 {
		t.Fatalf("清单应 5 条，实际 %d", len(lists))
	}
	if lists[0].FileName != "20261018-100006.db" || lists[0].CreatedAt != "2026-10-18 10:00:06" {
		t.Errorf("清单首条应为最新备份，实际：%+v", lists[0])
	}
	if lists[4].FileName != "20261018-100002.db" {
		t.Errorf("清单末条应为最旧保留份，实际 %q", lists[4].FileName)
	}
}

// TestAssetBackup_SameSecondDeepSuffix 同秒多次备份：深序后缀去冲突，两份共存，
// 清单倒序（深序后缀视为更新，排在裸名之前）且 CreatedAt 解析一致。
func TestAssetBackup_SameSecondDeepSuffix(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "同秒备份库", t.TempDir())
	freezeBackupClock(t, time.Date(2026, 10, 18, 10, 0, 0, 0, time.Local))

	first, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("首次备份: %v", err)
	}
	second, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("同秒二次备份: %v", err)
	}
	if first.FileName != "20261018-100000.db" || second.FileName != "20261018-100000_1.db" {
		t.Errorf("同秒二次备份应落 _1 深序后缀，实际 %q / %q", first.FileName, second.FileName)
	}
	lists, err := svc.ListAssetLibraryBackups(lib.ID)
	if err != nil {
		t.Fatalf("ListAssetLibraryBackups: %v", err)
	}
	if len(lists) != 2 || lists[0].FileName != second.FileName || lists[1].FileName != first.FileName {
		t.Errorf("清单应含两份且倒序（深序在后=更新），实际：%+v", lists)
	}
	for _, e := range lists {
		if e.CreatedAt != "2026-10-18 10:00:00" {
			t.Errorf("CreatedAt 应解析自文件名时间戳，实际 %q", e.CreatedAt)
		}
	}
}

// TestAssetBackup_ConcurrentSameSecond 同库并发备份互斥（素材审核 2026-10-18 补口）：
// 固定时钟同秒下 3 个并发备份全部成功且文件名互异——无互斥时同秒选名竞态会让
// 后到者 VACUUM INTO 撞「output file already exists」，其失败清理还会误删先到者
// 刚生成的有效备份；互斥（lockLibraryBackups）下选名串行推进深序后缀。
func TestAssetBackup_ConcurrentSameSecond(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "并发备份库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)
	freezeBackupClock(t, time.Date(2026, 10, 18, 10, 0, 0, 0, time.Local))

	const n = 3
	entries := make([]model.AssetBackupEntry, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			entries[i], errs[i] = svc.BackupAssetLibrary(lib.ID)
		}(i)
	}
	wg.Wait()
	names := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("并发备份第 %d 个失败: %v", i+1, errs[i])
		}
		if names[entries[i].FileName] {
			t.Errorf("并发备份文件名应互异（深序后缀推进），重复：%q", entries[i].FileName)
		}
		names[entries[i].FileName] = true
		if entries[i].SizeBytes <= 0 {
			t.Errorf("备份文件大小应大于 0，实际：%+v", entries[i])
		}
		if _, err := os.Stat(entries[i].Path); err != nil {
			t.Errorf("备份文件应存续: %v", err)
		}
	}
}

// TestAssetBackup_VacuumFailureKeepsOtherBackups VACUUM 失败清理不误删他方备份
// （素材审核 2026-10-18 补口）：预置一份有效备份后注入 VACUUM 失败（关闭在管
// 连接，storeForLocked 复用缓存连接不检活，closed db 上 vacuumInto 确定性报错），
// 断言报错且预置备份原样存续——失败清理的安全边界（removeFailedVacuumTarget）
// 只清理本调用选名后确认不存在的空残留。
func TestAssetBackup_VacuumFailureKeepsOtherBackups(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "失败清理库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)
	freezeBackupClock(t, time.Date(2026, 10, 18, 10, 0, 0, 0, time.Local))

	// 预置有效备份（裸名 .db）+ 关闭库连接注入 VACUUM 失败（后续备份选名 _1.db）
	if _, err := svc.BackupAssetLibrary(lib.ID); err != nil {
		t.Fatalf("预置备份: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.db.Close(); err != nil {
		t.Fatalf("关闭库连接: %v", err)
	}
	if _, err := svc.BackupAssetLibrary(lib.ID); err == nil {
		t.Fatal("连接已关的备份应报错")
	}
	names, err := listBackupFileNames(svc.assetBackupsDir(lib.ID))
	if err != nil {
		t.Fatalf("枚举备份目录: %v", err)
	}
	if len(names) != 1 || names[0] != "20261018-100000.db" {
		t.Errorf("失败清理不应误删预置有效备份（应恰剩裸名 1 份），实际：%v", names)
	}
}

// TestRemoveFailedVacuumTarget 失败清理安全删除判定单点：空文件（SQLite 报错前
// 创建的残留）删；非空文件（跨进程窗口下并发方刚生成的有效备份 / 磁盘满类部分
// 写入）不删；目标不存在无操作不报错。
func TestRemoveFailedVacuumTarget(t *testing.T) {
	dir := t.TempDir()

	absent := filepath.Join(dir, "absent.db")
	removeFailedVacuumTarget(absent) // 不 panic 即可
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Errorf("不存在目标应保持不存在: %v", err)
	}

	empty := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatalf("造空残留: %v", err)
	}
	removeFailedVacuumTarget(empty)
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Errorf("空残留应被清理: %v", err)
	}

	valid := filepath.Join(dir, "valid.db")
	if err := os.WriteFile(valid, []byte("SQLite format 3\x00pretend"), 0o644); err != nil {
		t.Fatalf("造有效备份: %v", err)
	}
	removeFailedVacuumTarget(valid)
	if _, err := os.Stat(valid); err != nil {
		t.Errorf("非空文件可能是他方有效备份，不应删除: %v", err)
	}
}

// TestAssetBackup_LibraryNotFound 库不存在报错（列表过期场景）。
func TestAssetBackup_LibraryNotFound(t *testing.T) {
	svc := newTestAssetService(t)
	if _, err := svc.BackupAssetLibrary("nonexistent"); !errors.Is(err, errAssetLibraryNotFound) {
		t.Fatalf("库不存在应报 E_ASSETS_LIB_NOT_FOUND，实际：%v", err)
	}
	if _, err := svc.ListAssetLibraryBackups("nonexistent"); !errors.Is(err, errAssetLibraryNotFound) {
		t.Fatalf("清单同口径报库不存在，实际：%v", err)
	}
}

// TestAssetBackup_IntegrityCheckDetectsCorruption integrity_check 损坏检出：破坏
// 备份文件中部字节后校验必须失败（备份失败不静默——带病备份比无备份更危险）。
func TestAssetBackup_IntegrityCheckDetectsCorruption(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "损坏检出库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)

	entry, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("备份: %v", err)
	}
	// 多点破坏（页 1 头部之后的 schema 区 + 后续 b-tree 页起始），任一点被检出即失败
	f, err := os.OpenFile(entry.Path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("打开备份: %v", err)
	}
	for _, offset := range []int64{100, 4096, 8192} {
		if _, err := f.WriteAt([]byte{0xFF, 0xFF, 0xFF, 0xFF}, offset); err != nil {
			t.Fatalf("写入破坏字节（offset=%d）: %v", offset, err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("关闭备份: %v", err)
	}
	if err := verifySQLiteIntegrity(entry.Path); err == nil {
		t.Error("破坏后的备份文件应被 integrity_check 检出（校验失败）")
	}
}

// TestRebuildScan_CreatesBackupBeforeClear 重建前自动备份：RebuildScan 成功后备份
// 目录恰生成 1 份，且清单可读（重大操作自动快照挂点接线）。
func TestRebuildScan_CreatesBackupBeforeClear(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重建备份库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)

	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}
	lists, err := svc.ListAssetLibraryBackups(lib.ID)
	if err != nil {
		t.Fatalf("ListAssetLibraryBackups: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("重建应自动落 1 份备份，实际 %d 份", len(lists))
	}
	if err := verifySQLiteIntegrity(lists[0].Path); err != nil {
		t.Errorf("重建前备份完整性应通过: %v", err)
	}
}

// TestRebuildScan_BackupFailureDoesNotAbort 重建前备份失败仅告警不中止重建
// （research §5：重建可重扫恢复，best-effort 备份不放大故障面）。
func TestRebuildScan_BackupFailureDoesNotAbort(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重建备份失败库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)
	occupyBackupsDir(t, svc, lib.ID)

	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("备份失败不应中止重建，实际：%v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 || files[0].Status != model.AssetFileStatusOK {
		t.Errorf("重建应正常完成（索引 1 文件 ok），实际：%+v", files)
	}
}

// TestDeleteLibrary_BackupConfirmFlow 删库前自动备份失败升级确认（PRD R3）：
// allowNoBackup=false 备份失败返回 E_ASSETS_DELETE_BACKUP_FAILED 确认型拒绝
// （删除未发生，库仍在）；allowNoBackup=true 用户已知情确认，删除继续。
func TestDeleteLibrary_BackupConfirmFlow(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "删库确认库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)
	occupyBackupsDir(t, svc, lib.ID)

	err := svc.DeleteLibrary(lib.ID, false)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsDeleteBackupFailed {
		t.Fatalf("删库前备份失败应返回 E_ASSETS_DELETE_BACKUP_FAILED 确认型拒绝，实际：%v", err)
	}
	libs, err := svc.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(libs) != 1 || libs[0].ID != lib.ID {
		t.Errorf("确认型拒绝后库应仍在注册表，实际：%+v", libs)
	}
	if _, err := os.Stat(svc.libraryDBPath(lib.ID)); err != nil {
		t.Errorf("确认型拒绝后库 db 文件应未被删除: %v", err)
	}

	// 用户二次确认（allowNoBackup=true）：备份仍失败，删除继续
	if err := svc.DeleteLibrary(lib.ID, true); err != nil {
		t.Fatalf("确认后删除应继续: %v", err)
	}
	libs, err = svc.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries: %v", err)
	}
	if len(libs) != 0 {
		t.Errorf("删除后注册表应为空，实际：%+v", libs)
	}
}

// TestDeleteLibrary_CreatesBackupAndKeepsIt 删库前自动备份生成且备份存续：
// 删除成功后备份文件保留（删库后唯一恢复点，备份目录不随库删除清理）。
func TestDeleteLibrary_CreatesBackupAndKeepsIt(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "删库备份库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)

	if err := svc.DeleteLibrary(lib.ID, false); err != nil {
		t.Fatalf("删除（备份成功路径）: %v", err)
	}
	names, err := listBackupFileNames(svc.assetBackupsDir(lib.ID))
	if err != nil || len(names) == 0 {
		t.Fatalf("删库后备份文件应存续（唯一恢复点），names=%v err=%v", names, err)
	}
	if err := verifySQLiteIntegrity(filepath.Join(svc.assetBackupsDir(lib.ID), names[0])); err != nil {
		t.Errorf("存续备份完整性应通过: %v", err)
	}
}

// TestRestoreAssetLibrary_RoundTrip 还原回路：改数据 → 备份 → 再改 → 还原 →
// 数据回到备份时点（新增文件消失、文件级标注回退），还原后库可直接查询且
// integrity_check 通过（还原链内置复验已通过，此处白盒再验落库文件）。
func TestRestoreAssetLibrary_RoundTrip(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "还原库", dir)
	buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{
		{titlePh: "甲一", body: []string{"内容"}},
		{titlePh: "甲二", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if err := svc.UpdateFileMeta(lib.ID, files[0].ID, model.AssetFileMetaUpdate{
		Rating: 4, Note: "备份时备注", Tags: []string{"文标"},
	}); err != nil {
		t.Fatalf("备份前标注: %v", err)
	}

	backup, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("备份: %v", err)
	}

	// 备份后再改：改文件级标注 + 新增文件乙并入库
	if err := svc.UpdateFileMeta(lib.ID, files[0].ID, model.AssetFileMetaUpdate{
		Rating: 1, Note: "备份后修改",
	}); err != nil {
		t.Fatalf("备份后修改: %v", err)
	}
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{{titlePh: "乙一", body: []string{"内容"}}})
	mustScanIndex(t, svc, lib)
	if files, err := svc.ListFiles(lib.ID); err != nil || len(files) != 2 {
		t.Fatalf("还原前应有 2 个文件，实际 %d 个（err=%v）", len(files), err)
	}

	if err := svc.RestoreAssetLibrary(lib.ID, backup.Path); err != nil {
		t.Fatalf("还原: %v", err)
	}

	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("还原后 ListFiles: %v", err)
	}
	if len(files) != 1 || files[0].Path != "甲.pptx" {
		t.Fatalf("还原应回到备份时点（仅甲.pptx），实际：%+v", files)
	}
	if files[0].Rating != 4 || files[0].Note != "备份时备注" || len(files[0].Tags) != 1 || files[0].Tags[0].Name != "文标" {
		t.Errorf("文件级标注应回到备份时点，实际：%+v", files[0])
	}
	if page, err := svc.ListSlides(lib.ID, 200, 0); err != nil || page.Total != 2 {
		t.Errorf("幻灯片索引应随还原回到 2 页，实际 total=%d err=%v", page.Total, err)
	}
	// 白盒再验落库文件完整性（还原链内置复验通过后的双保险断言）
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.integrityCheck(); err != nil {
		t.Errorf("还原后库文件完整性应通过: %v", err)
	}
}

// TestRestoreAssetLibrary_Guards 还原防呆面：备份路径越界（备份目录外）拒绝、
// 空路径拒绝、目录形态拒绝、备份文件不存在拒绝、损坏备份拒绝（预检零半态，
// 当前库原样）、库不存在报错。
func TestRestoreAssetLibrary_Guards(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "还原防呆库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)

	if err := svc.RestoreAssetLibrary(lib.ID, filepath.Join(dir, "内容.pptx")); err == nil {
		t.Error("备份目录外的路径应被拒绝（防任意文件覆盖 <id>.db）")
	}
	if err := svc.RestoreAssetLibrary(lib.ID, ""); err == nil {
		t.Error("空路径应被拒绝")
	}
	if err := svc.RestoreAssetLibrary(lib.ID, filepath.Join(svc.assetBackupsDir(lib.ID), "missing.db")); err == nil {
		t.Error("备份文件不存在应被拒绝")
	}

	backup, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("备份: %v", err)
	}
	if err := svc.RestoreAssetLibrary(lib.ID, svc.assetBackupsDir(lib.ID)); err == nil {
		t.Error("目录形态路径应被拒绝")
	}

	// 损坏备份拒绝还原：预检失败当前库原样（数据仍可查询）
	f, err := os.OpenFile(backup.Path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("打开备份: %v", err)
	}
	if _, err := f.WriteAt([]byte{0xFF, 0xFF, 0xFF, 0xFF}, 4096); err != nil {
		t.Fatalf("破坏备份: %v", err)
	}
	_ = f.Close()
	if err := svc.RestoreAssetLibrary(lib.ID, backup.Path); err == nil {
		t.Error("损坏备份应被预检拒绝")
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil || len(files) != 1 || files[0].Path != "内容.pptx" {
		t.Errorf("损坏备份拒绝后当前库应原样可查询，实际：%+v（err=%v）", files, err)
	}

	if err := svc.RestoreAssetLibrary("nonexistent", backup.Path); !errors.Is(err, errAssetLibraryNotFound) {
		t.Errorf("库不存在应报 E_ASSETS_LIB_NOT_FOUND，实际：%v", err)
	}
}

// TestRestoreAssetLibrary_RejectsNewerBackup 版本预检（P0 数据安全）：备份库
// user_version 高于当前支持时拒绝还原，且当前索引与备份文件均保持原样。缺预检
// 时 rename 先消费备份、withStore→migrate 才报「版本过新」，备份与原库双失不可逆。
func TestRestoreAssetLibrary_RejectsNewerBackup(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "版本预检库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)

	backup, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("备份: %v", err)
	}

	// 构造版本过新备份：直接把备份库 user_version 提到当前 +1（模拟高版本应用产出）
	newer := assetsSchemaVersion + 1
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(backup.Path))
	if err != nil {
		t.Fatalf("打开备份库: %v", err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", newer)); err != nil {
		_ = db.Close()
		t.Fatalf("写入备份库版本: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭备份库: %v", err)
	}

	restoreErr := svc.RestoreAssetLibrary(lib.ID, backup.Path)
	if restoreErr == nil {
		t.Fatal("版本过新的备份应被拒绝还原")
	}
	if msg := restoreErr.Error(); !strings.Contains(msg, fmt.Sprintf("v%d", newer)) ||
		!strings.Contains(msg, "升级 WorkBench") || !strings.Contains(msg, "当前索引未改动") {
		t.Errorf("版本预检错误文案应含版本号与升级引导，实际：%v", restoreErr)
	}

	// 备份文件与当前索引均保持原样（版本预检须发生在 rename 之前）
	if _, statErr := os.Stat(backup.Path); statErr != nil {
		t.Errorf("备份文件应仍存在（未被 rename 消费）: %v", statErr)
	}
	if _, statErr := os.Stat(svc.libraryDBPath(lib.ID)); statErr != nil {
		t.Errorf("当前索引文件应仍存在: %v", statErr)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil || len(files) != 1 || files[0].Path != "内容.pptx" {
		t.Errorf("拒绝后当前索引记录应仍可读，实际：%+v（err=%v）", files, err)
	}
}

// TestRestoreAssetLibrary_PreservesArtifactStem 正常版本备份还原成功且 v9 新列
// artifact_stem 随备份原样回退（备份为整库 db 快照，新列不因还原丢失）。
func TestRestoreAssetLibrary_PreservesArtifactStem(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "还原新列库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)

	const rel = "内容.pptx"
	outDir := filepath.Join(svc.assetsDir, "derived", "内容")
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord(rel, "hash-a", outDir, 2,
		"2026-11-02 10:00:00", model.AssetSplitTargetSource, "内容", false); err != nil {
		t.Fatalf("写入备份时点拆分记录: %v", err)
	}

	backup, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("备份: %v", err)
	}

	// 备份后改写记录（主干/落点均变化），还原应回退到备份时点
	store = svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord(rel, "hash-b", outDir+"-changed", 9,
		"2026-11-02 11:00:00", model.AssetSplitTargetReplace, "改后主干", true); err != nil {
		t.Fatalf("备份后改写拆分记录: %v", err)
	}

	if err := svc.RestoreAssetLibrary(lib.ID, backup.Path); err != nil {
		t.Fatalf("正常版本备份还原应成功: %v", err)
	}

	store = svc.openStoreForTest(t, lib.ID)
	record, err := store.getSplitRecord(rel)
	if err != nil || record == nil {
		t.Fatalf("还原后拆分记录应存在: record=%+v err=%v", record, err)
	}
	if record.artifactStem != "内容" || record.pageCount != 2 ||
		record.target != model.AssetSplitTargetSource || record.sourceRemoved {
		t.Errorf("v9 新列与落点字段应随备份回退，实际：%+v", record)
	}
}

// TestVerifyBackupDatabase 备份预检单元边界：
//   - 非 SQLite 文件按「无法校验」拒绝（安全侧，不猜测放行）；
//   - 合法空 SQLite（user_version=0、无核心表）拒绝——此前只校验版本会放行，
//     还原后索引被清空为 0 文件（数据不可逆）；
//   - 含核心表且版本 <= 当前放行（过旧版本 migrate 逐级升级属既有语义）；
//   - 版本 > 当前拒绝。
func TestVerifyBackupDatabase(t *testing.T) {
	dir := t.TempDir()
	writeDB := func(name string, version int, tables []string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
		if err != nil {
			t.Fatalf("建库 %s: %v", name, err)
		}
		for _, table := range tables {
			if _, err := db.Exec("CREATE TABLE " + table + " (id INTEGER PRIMARY KEY)"); err != nil {
				_ = db.Close()
				t.Fatalf("建表 %s.%s: %v", name, table, err)
			}
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			_ = db.Close()
			t.Fatalf("写 user_version %s: %v", name, err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("关闭 %s: %v", name, err)
		}
		return path
	}

	// 非 SQLite 文件：读取失败拒绝（安全侧）
	bogus := filepath.Join(dir, "bogus.db")
	if err := os.WriteFile(bogus, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatalf("写入伪库文件: %v", err)
	}
	if err := verifyBackupDatabase(bogus); err == nil || !strings.Contains(err.Error(), "无法校验备份库") {
		t.Errorf("非 SQLite 文件应按无法校验拒绝，实际：%v", err)
	}

	// 合法空库：版本号合法但无核心表，放行会把索引清空为 0 文件
	empty := writeDB("empty.db", 0, nil)
	if err := verifyBackupDatabase(empty); err == nil || !strings.Contains(err.Error(), "缺少核心表") {
		t.Errorf("空 SQLite 应按缺少核心表拒绝，实际：%v", err)
	}
	// 只含一张核心表同样拒绝（须 files 与 split_records 同时存在）
	partial := writeDB("partial.db", assetsSchemaVersion, []string{"files"})
	if err := verifyBackupDatabase(partial); err == nil || !strings.Contains(err.Error(), "缺少核心表") {
		t.Errorf("仅含 files 表应拒绝，实际：%v", err)
	}

	for _, version := range []int{assetsSchemaVersion - 1, assetsSchemaVersion} {
		path := writeDB(fmt.Sprintf("v%d.db", version), version, []string{"files", "split_records"})
		if err := verifyBackupDatabase(path); err != nil {
			t.Errorf("v%d（<= 当前 v%d）含核心表应放行，实际：%v", version, assetsSchemaVersion, err)
		}
	}

	newer := writeDB("newer.db", assetsSchemaVersion+1, []string{"files", "split_records"})
	err := verifyBackupDatabase(newer)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("v%d", assetsSchemaVersion+1)) ||
		!strings.Contains(err.Error(), "升级 WorkBench") {
		t.Errorf("版本过新应拒绝并引导升级，实际：%v", err)
	}
}

// TestSQLiteReadOnlyDSN 只读 DSN 单点：路径中的 %、?、# 必须百分号编码（file:
// URI 解析器会把它们当转义前缀/查询分隔符，未编码时可能打开/校验到另一个文件，
// 预检形同虚设），编码后仍能读到真实文件。
func TestSQLiteReadOnlyDSN(t *testing.T) {
	dir := t.TempDir()

	// 编码断言（纯字符串：Windows 文件名不允许 ?，不能落盘验证）
	const suffix = "?mode=ro&_pragma=busy_timeout(5000)"
	got := sqliteReadOnlyDSN(filepath.Join(dir, "a%b#c?.db"))
	if !strings.HasSuffix(got, suffix) {
		t.Fatalf("DSN 应以只读参数结尾，实际：%s", got)
	}
	wantPath := "file:" + filepath.ToSlash(filepath.Join(dir, "a%25b%23c%3F.db"))
	if pathPart := strings.TrimSuffix(got, suffix); pathPart != wantPath {
		t.Errorf("路径特殊字符编码不符：want %q, got %q", wantPath, pathPart)
	}

	// 真实文件可读（# 与 % 在 Windows/Linux 均为合法文件名字符）
	plain := filepath.Join(dir, "plain.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(plain))
	if err != nil {
		t.Fatalf("建库: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 7"); err != nil {
		_ = db.Close()
		t.Fatalf("写 user_version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭库: %v", err)
	}
	special := filepath.Join(dir, "a%b#c.db")
	if err := os.Rename(plain, special); err != nil {
		t.Fatalf("改名为特殊字符路径: %v", err)
	}
	ro, err := sql.Open("sqlite", sqliteReadOnlyDSN(special))
	if err != nil {
		t.Fatalf("按只读 DSN 打开: %v", err)
	}
	defer ro.Close()
	var version int
	if err := ro.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("特殊字符路径应能读到真实库: %v", err)
	}
	if version != 7 {
		t.Errorf("读到 user_version = %d, want 7", version)
	}
}

// TestRestoreAssetLibrary_RejectsNonAssetBackup 还原预检：合法空 SQLite（user_version=0、
// 无核心表）此前能通过完整性与版本检查，放行后 rename 消费备份、索引被清空为 0
// 文件。修复后须拒绝，且当前索引、空库文件与正常备份均保持原样；正常备份仍可还原。
func TestRestoreAssetLibrary_RejectsNonAssetBackup(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "非素材备份拒还原库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)

	backup, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("备份: %v", err)
	}

	// 在备份目录内放一个合法空 SQLite（模拟用户误选其他应用 db）
	backupDir := svc.assetBackupsDir(lib.ID)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("建备份目录: %v", err)
	}
	emptyPath := filepath.Join(backupDir, "empty.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(emptyPath))
	if err != nil {
		t.Fatalf("建空库: %v", err)
	}
	// sql.Open 惰性连接：须实际执行一条语句才会创建库文件
	if _, execErr := db.Exec("PRAGMA user_version = 0"); execErr != nil {
		_ = db.Close()
		t.Fatalf("初始化空库: %v", execErr)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭空库: %v", err)
	}

	restoreErr := svc.RestoreAssetLibrary(lib.ID, emptyPath)
	if restoreErr == nil {
		t.Fatal("非素材库 SQLite 备份应被拒绝还原")
	}
	if msg := restoreErr.Error(); !strings.Contains(msg, "缺少核心表") ||
		!strings.Contains(msg, "当前索引未做改动") {
		t.Errorf("错误文案应说明非素材库备份且当前索引未改动，实际：%v", restoreErr)
	}

	// 当前索引未被清空，记录仍可读
	files, err := svc.ListFiles(lib.ID)
	if err != nil || len(files) != 1 || files[0].Path != "内容.pptx" {
		t.Errorf("拒绝后当前索引记录应仍可读，实际：%+v（err=%v）", files, err)
	}
	// 空库文件与正常备份均保持原样（预检在 rename 之前）
	if _, statErr := os.Stat(emptyPath); statErr != nil {
		t.Errorf("空库文件应仍存在: %v", statErr)
	}
	if _, statErr := os.Stat(backup.Path); statErr != nil {
		t.Errorf("正常备份文件应仍存在: %v", statErr)
	}
	// 正常备份（含核心表）仍可放行
	if err := svc.RestoreAssetLibrary(lib.ID, backup.Path); err != nil {
		t.Fatalf("正常备份还原应成功: %v", err)
	}
}

// TestRestoreAssetLibrary_RejectsWhenExportPending 导出注册表非空拒还原（换库窗口
// 封死，素材审核 2026-10-18 复查 FAIL 补口）：CancelExport 遍历与导出注册存在竞态，
// 旧库物化行键的导出若在还原取 s.mu 前漏网，后续批次水合新库即静默混批——还原链
// 须在关连接前持 s.mu 复查 exportCancels，非空拒绝。构造：首个注册条目被
// CancelExport 消费（delete + 同步调用 cancel），cancel 闭包内再注册一条，确定性
// 驱动「遍历未见到、复查前注册完成」的窗口形态（真实纳秒级竞态不可定时构造，
// 注册表 map 直接操作对齐 TestCancelExport_HitsRegisteredCancel 白盒先例）。
// 当前库未被替换用内容探针断言（备份后修改的备注仍在；mtime/文件大小断言依赖
// FS 时序，docs/spec/test-stability.md 禁用）。
func TestRestoreAssetLibrary_RejectsWhenExportPending(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "导出竞态拒还原库", dir)
	buildAssetsPptx(t, dir, "内容.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	mustScanIndex(t, svc, lib)

	backup, err := svc.BackupAssetLibrary(lib.ID)
	if err != nil {
		t.Fatalf("备份: %v", err)
	}
	// 备份后修改文件级备注，作为「当前库未被替换」的内容探针（还原若发生，备注
	// 会回退为备份时点的空值）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if err := svc.UpdateFileMeta(lib.ID, files[0].ID, model.AssetFileMetaUpdate{Note: "备份后修改"}); err != nil {
		t.Fatalf("备份后修改备注: %v", err)
	}

	var reentry *cancelEntry
	entry := svc.registerExportCancel(lib.ID, func() {
		if reentry == nil {
			reentry = svc.registerExportCancel(lib.ID, func() {})
		}
	})
	t.Cleanup(func() {
		svc.unregisterExportCancel(lib.ID, entry)
		if reentry != nil {
			svc.unregisterExportCancel(lib.ID, reentry)
		}
	})

	err = svc.RestoreAssetLibrary(lib.ID, backup.Path)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsScanInProgress {
		t.Fatalf("导出注册表非空应拒还原（复用 E_ASSETS_SCAN_IN_PROGRESS，消息按导出措辞），实际：%v", err)
	}

	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("拒绝后 ListFiles: %v", err)
	}
	if len(files) != 1 || files[0].Note != "备份后修改" {
		t.Errorf("拒绝后当前库应未被替换（备份后修改的备注仍在），实际：%+v", files)
	}
}

// TestRebuildScan_HashRecoveryAfterRename 哈希恢复端到端（SRS §14.16 补链）：
// 重建后文件改名（路径 miss）→ 按 hash 兜底恢复文件标签与页级标注。
func TestRebuildScan_HashRecoveryAfterRename(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "哈希恢复库", dir)
	srcA := buildAssetsPptx(t, dir, "甲.pptx", []assetsSlideSpec{
		{titlePh: "甲一", body: []string{"内容"}},
		{titlePh: "甲二", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "乙.pptx", []assetsSlideSpec{{titlePh: "乙一", body: []string{"内容"}}})
	mustScanIndex(t, svc, lib)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idsByPath := make(map[string]int64, len(files))
	for _, f := range files {
		idsByPath[f.Path] = f.ID
	}
	if err := svc.UpdateFileMeta(lib.ID, idsByPath["甲.pptx"], model.AssetFileMetaUpdate{Tags: []string{"文标"}}); err != nil {
		t.Fatalf("文件打标签: %v", err)
	}
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["甲.pptx"], 1, model.AssetSlideMetaUpdate{
		Rating: 4, Note: "甲一备注", TagNames: []string{"页标A"},
	})

	// 改名（内容不变 → hash 不变）：重建后路径「甲.pptx」miss，走哈希兜底
	if err := os.Rename(srcA, filepath.Join(dir, "丙.pptx")); err != nil {
		t.Fatalf("改名: %v", err)
	}
	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}

	restFiles, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("重建后 ListFiles: %v", err)
	}
	found := false
	for _, f := range restFiles {
		if f.Path == "丙.pptx" {
			found = true
			if len(f.Tags) != 1 || f.Tags[0].Name != "文标" {
				t.Errorf("文件标签应经哈希兜底恢复到改名后文件，实际：%+v", f)
			}
		}
		if f.Path == "甲.pptx" {
			t.Error("改名后原路径不应存在索引记录")
		}
	}
	if !found {
		t.Fatal("改名后文件 丙.pptx 应在索引中")
	}
	kept := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "丙.pptx", 1)
	if kept.Rating != 4 || kept.Note != "甲一备注" || len(kept.OwnTags) != 1 || kept.OwnTags[0].Name != "页标A" {
		t.Errorf("页级标注应经哈希兜底恢复，实际：%+v", kept)
	}
}

// TestRebindAfterRebuild_HashMultiHitTakesFirst 哈希兜底多命中取首个（单测聚焦
// 匹配逻辑，直调存储层重绑函数）：同 hash 两文件，备份条目（旧路径已消失）绑定
// 落在首个（files 按 id 升序 = 先插入者），计数 path=0 hash=1。
func TestRebindAfterRebuild_HashMultiHitTakesFirst(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "哈希多命中库", dir)
	buildAssetsPptx(t, dir, "a.pptx", []assetsSlideSpec{{titlePh: "A1", body: []string{"内容"}}})
	mustScanIndex(t, svc, lib)
	tag, err := svc.CreateTag(lib.ID, "文标", 0, "")
	if err != nil {
		t.Fatalf("建标签: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)

	// 模拟重建后状态：清索引 + 两个同 hash 文件 x/y（先 x 后 y，id 递增）
	if err := store.clearIndexForRebuild(); err != nil {
		t.Fatalf("清索引: %v", err)
	}
	const now = "2026-01-01 00:00:00"
	xID, err := store.insertProcessingFile("x.pptx", "x", 128, 64, now)
	if err != nil {
		t.Fatalf("插占位 x: %v", err)
	}
	if err := store.replaceFileSlides(xID, 128, 64, "hash-same", now, []parsedSlide{
		{Num: 1, Title: "X1", Text: "X1 内容"},
	}); err != nil {
		t.Fatalf("重灌 x: %v", err)
	}
	yID, err := store.insertProcessingFile("y.pptx", "y", 128, 64, now)
	if err != nil {
		t.Fatalf("插占位 y: %v", err)
	}
	if err := store.replaceFileSlides(yID, 128, 64, "hash-same", now, []parsedSlide{
		{Num: 1, Title: "Y1", Text: "Y1 内容"},
	}); err != nil {
		t.Fatalf("重灌 y: %v", err)
	}

	bindings := map[string]assetFileTagBinding{
		"old.pptx": {hash: "hash-same", tagIDs: []int64{tag.ID}},
	}
	pathHit, hashHit, err := store.rebindFileTagsAfterRebuild(bindings)
	if err != nil {
		t.Fatalf("重绑文件标签: %v", err)
	}
	if pathHit != 0 || hashHit != 1 {
		t.Errorf("路径应 miss、哈希兜底应命中 1，实际 path=%d hash=%d", pathHit, hashHit)
	}
	restFiles, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range restFiles {
		want := f.Path == "x.pptx" // 首个（最小 id）文件承接绑定
		if got := len(f.Tags) == 1; got != want {
			t.Errorf("标签应仅绑定 x.pptx（取首个），%s 实际绑定=%v", f.Path, got)
		}
	}

	// 页级标注同口径：多命中取首个文件
	annots := map[string]assetFileSlideAnnotations{
		"old.pptx": {hash: "hash-same", pages: []slideAnnotation{{slideNum: 1, note: "旧备注", rating: 3}}},
	}
	slidePathHit, slideHashHit, err := store.rebindSlideAnnotationsAfterRebuild(annots)
	if err != nil {
		t.Fatalf("重绑页级标注: %v", err)
	}
	if slidePathHit != 0 || slideHashHit != 1 {
		t.Errorf("页级标注路径应 miss、哈希兜底应命中 1 页，实际 path=%d hash=%d", slidePathHit, slideHashHit)
	}
	entries := mustListSlides(t, svc, lib.ID)
	kept := slideEntryByNum(t, entries, "x.pptx", 1)
	if kept.Note != "旧备注" || kept.Rating != 3 {
		t.Errorf("页级标注应落在首个文件 x.pptx，实际：%+v", kept)
	}
	if y := slideEntryByNum(t, entries, "y.pptx", 1); y.Note != "" || y.Rating != 0 {
		t.Errorf("第二份同内容文件不应被误绑，实际：%+v", y)
	}
}

// TestRebindAfterRebuild_SameHashMultiOldBindingsDeterministic 多旧绑定同 hash
// 确定性落点（素材审核 2026-10-18 补口）：两个旧文件内容相同且重建后都改名（双双
// path miss）时全部页标注落同一新文件——slide_meta INSERT OR REPLACE 后写覆盖，
// 修复前由 map 遍历序定胜负（跨运行不确定）。收敛：旧路径字典序升序处理，字典序
// 后者胜（old-b > old-a → old-b 的 note/rating 落地）；任一 hash 组多旧绑定时
// slog.Warn 同源路径列表（区别于既有「多命中取首个」单绑定告警——后者针对新文件
// 侧同 hash 多副本，本告警针对旧绑定侧同 hash 多源）。
func TestRebindAfterRebuild_SameHashMultiOldBindingsDeterministic(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "多旧绑定库", dir)
	buildAssetsPptx(t, dir, "a.pptx", []assetsSlideSpec{{titlePh: "A1", body: []string{"内容"}}})
	mustScanIndex(t, svc, lib)
	store := svc.openStoreForTest(t, lib.ID)

	// 模拟重建后状态：清索引 + 两个同 hash 文件 x/y（先 x 后 y，id 递增）
	if err := store.clearIndexForRebuild(); err != nil {
		t.Fatalf("清索引: %v", err)
	}
	const now = "2026-01-01 00:00:00"
	for _, p := range []string{"x.pptx", "y.pptx"} {
		id, err := store.insertProcessingFile(p, p, 128, 64, now)
		if err != nil {
			t.Fatalf("插占位 %s: %v", p, err)
		}
		if err := store.replaceFileSlides(id, 128, 64, "hash-same", now, []parsedSlide{
			{Num: 1, Title: "P1", Text: "P1 内容"},
		}); err != nil {
			t.Fatalf("重灌 %s: %v", p, err)
		}
	}

	// 捕获告警日志（SetLogger 全局注入；本包用例串行运行，Cleanup 还原）
	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	annots := map[string]assetFileSlideAnnotations{
		"old-a.pptx": {hash: "hash-same", pages: []slideAnnotation{{slideNum: 1, note: "前序注", rating: 1}}},
		"old-b.pptx": {hash: "hash-same", pages: []slideAnnotation{{slideNum: 1, note: "后序注", rating: 5}}},
	}
	pathHit, hashHit, err := store.rebindSlideAnnotationsAfterRebuild(annots)
	if err != nil {
		t.Fatalf("重绑页级标注: %v", err)
	}
	if pathHit != 0 || hashHit != 2 {
		t.Errorf("两旧绑定均应 path miss + 哈希兜底各 1 页，实际 path=%d hash=%d", pathHit, hashHit)
	}
	entries := mustListSlides(t, svc, lib.ID)
	kept := slideEntryByNum(t, entries, "x.pptx", 1)
	if kept.Note != "后序注" || kept.Rating != 5 {
		t.Errorf("多旧绑定同 hash 应按旧路径字典序后者胜（old-b > old-a），实际：%+v", kept)
	}
	if y := slideEntryByNum(t, entries, "y.pptx", 1); y.Note != "" || y.Rating != 0 {
		t.Errorf("第二份同内容新文件不应被绑，实际：%+v", y)
	}
	// 组级告警触发：同源路径列表
	logs := logBuf.String()
	if !strings.Contains(logs, "多个旧绑定同哈希兜底落同一新文件") {
		t.Errorf("多旧绑定同 hash 应触发组级 slog.Warn，日志：%s", logs)
	}
	if !strings.Contains(logs, "old-a.pptx") || !strings.Contains(logs, "old-b.pptx") {
		t.Errorf("组级告警应含同源路径列表，日志：%s", logs)
	}
}

// TestAssetBackup_EmptyLibraryList 从未备份的库返回空清单非错误（还原入口空态）。
func TestAssetBackup_EmptyLibraryList(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "空备份库", t.TempDir())
	lists, err := svc.ListAssetLibraryBackups(lib.ID)
	if err != nil {
		t.Fatalf("从未备份应返回空清单非错误: %v", err)
	}
	if len(lists) != 0 {
		t.Errorf("应返回空清单，实际：%+v", lists)
	}
	if !strings.Contains(svc.assetBackupsDir(lib.ID), filepath.Join("backups", lib.ID)) {
		t.Errorf("备份目录应为 data/assets/backups/<libId> 形态，实际 %q", svc.assetBackupsDir(lib.ID))
	}
}
