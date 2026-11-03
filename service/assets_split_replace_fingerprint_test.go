package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- replace 备份让位前的产物指纹校验（11-02-replace / followup） ----------
//
// v10 页级指纹此前只服务于三处清理路径；就地替换的「备份让位」仍按命名形态认定
// 历史产物——用户把容器目录内某页产物替换成自有内容（同名）时，重拆会把它当历史
// 产物备份并在成功后删除，内容不可恢复。本组用例覆盖：同名不同内容拒绝覆盖（含
// 前序页备份还原）、旧产物字节未变正常让位、指纹缺失回落按名覆盖、大小写折叠多
// 命中与平台分流、核心中间态同名文件守卫、备份 rename 后复验（TOCTOU）。
//
// 拒绝类用例的用户内容与旧产物**等长**（9 字节 userbytes vs old-bytes），确保拒绝
// 判定必须走到 SHA-256 比较，而非被 size 预筛拦下（followup R1）；变异验证：
// 把 fingerprintMatch 的 SHA-256 比较改为恒真，拒绝类用例必须变红。
//
// fixture 一律走 buildAssetsPptx + 注入 fake 核心/解析/回收站，不依赖真实 Office
// 与外部状态（见 docs/spec/test-stability.md）。平台大小写断言拆至
// assets_split_replace_fingerprint_windows_test.go / _other_test.go，共享夹具留本文件。

// resplitReplaceForTest 经内部链路驱动 replace 重拆，便于直接观察 rolledBack
// 标记（单文件公开入口把业务失败升格为 error）。快照哈希取库存值（重扫后与磁盘
// 一致）；不取扫描/拆分锁，与既有内部链路用例口径一致。
func resplitReplaceForTest(t *testing.T, svc *AssetLibraryService, lib model.AssetLibrary, rel string) (splitOutcome, error) {
	t.Helper()
	store := svc.openStoreForTest(t, lib.ID)
	hash, status, err := store.fileHashStatusByPath(rel)
	if err != nil {
		t.Fatalf("读源哈希（%s）: %v", rel, err)
	}
	if status != model.AssetFileStatusOK {
		t.Fatalf("前置：源文件应为 ok 状态（%s），实际 %s", rel, status)
	}
	abs := filepath.Join(lib.RootPath, filepath.FromSlash(rel))
	return svc.splitOneFileWithHash(context.Background(), store, lib, rel, abs, hash, "replace", 1, 1, nil)
}

// fileSizeHashForTest 读取指定库内相对路径的 files.size / files.hash（回滚后索引
// 行自愈断言用）。
func fileSizeHashForTest(t *testing.T, svc *AssetLibraryService, libraryID, rel string) (int64, string) {
	t.Helper()
	var size int64
	var hash string
	if err := svc.openStoreForTest(t, libraryID).db.QueryRow(
		"SELECT size, hash FROM files WHERE path = ?", rel).Scan(&size, &hash); err != nil {
		t.Fatalf("读取索引行（%s）: %v", rel, err)
	}
	return size, hash
}

// TestAssetSplitReplace_FingerprintRejectsUserReplacedPage 用户同名不同内容文件：
// 记录归属凭证可用时，目标名命中 oldArtifacts 但内容与凭证不符视为用户文件，拒绝
// 覆盖并回滚本次已产出页。R1：用户内容与旧产物**等长不同哈希**（9 字节），拒绝
// 只能由 SHA-256 比较产生。R4：文案区分为「内容与拆分记录不一致」。
func TestAssetSplitReplace_FingerprintRejectsUserReplacedPage(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹拒绝覆盖库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "方案.pptx", 2)
	outDir := filepath.Join(dir, "方案")
	userPage := filepath.Join(outDir, "002-方案.pptx")
	// 与并发常量 old-bytes 等长（9 字节）但内容不同：size 预筛放行，必须走到 SHA-256
	if err := os.WriteFile(userPage, []byte("userbytes"), 0644); err != nil {
		t.Fatalf("写用户同名文件: %v", err)
	}
	// 前置：记录归属凭证可用，且第 2 页指纹描述的是旧产物内容（与用户文件同长异哈希）
	rec := splitRecordRowForTest(t, svc, lib.ID, "方案.pptx")
	if !fingerprintsUsable(rec) {
		t.Fatalf("前置：首次 replace 记录应带可用指纹，实际 %+v", rec)
	}
	fp, ok := fingerprintForName(rec, "002-方案.pptx")
	if !ok {
		t.Fatalf("前置：记录应有第 2 页指纹，实际 %+v", rec.fingerprints)
	}
	const oldLen, userLen = len("old-bytes"), len("userbytes")
	if oldLen != userLen {
		t.Fatalf("前置：用户内容与旧产物必须等长（%d != %d）", userLen, oldLen)
	}
	if fp.Size != int64(userLen) {
		t.Fatalf("前置：指纹 size 应等于等长内容字节数，实际 %d", fp.Size)
	}
	sum, err := hashFileSHA256(userPage)
	if err != nil {
		t.Fatalf("计算用户文件哈希: %v", err)
	}
	if strings.EqualFold(sum, fp.SHA256) || fingerprintMatch(userPage, fp) {
		t.Fatalf("前置：同长内容必须与指纹哈希不同（sum=%s fp=%s）", sum, fp.SHA256)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	outcome, err := resplitReplaceForTest(t, svc, lib, "方案.pptx")
	if err != nil {
		t.Fatalf("指纹不符属业务失败（failed 明细 + nil error），实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed || !outcome.rolledBack {
		t.Fatalf("指纹不符应失败并回滚：%+v", outcome)
	}
	wantMsg := "第 2 页目标文件「002-方案.pptx」内容与拆分记录不一致（可能已被修改），请确认后移走或还原"
	if !strings.Contains(outcome.message, wantMsg) {
		t.Errorf("失败消息应按 R4 区分内容不符，含 %q，实际 %q", wantMsg, outcome.message)
	}
	// 用户文件字节不变
	if got, readErr := os.ReadFile(userPage); readErr != nil || string(got) != "userbytes" {
		t.Errorf("用户文件应原样保留（err=%v, content=%q）", readErr, got)
	}
	// 前序页已备份让位 → 回滚还原旧内容
	if got, readErr := os.ReadFile(filepath.Join(outDir, "001-方案.pptx")); readErr != nil || string(got) != "old-bytes" {
		t.Errorf("回滚应还原第 1 页旧产物（err=%v, content=%q）", readErr, got)
	}
	assertNoReplaceBackup(t, outDir)
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("失败应保留源文件：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("失败不得移入回收站，实际 %v", got)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, exists, err := store.fileRecordByPath("方案.pptx"); err != nil || !exists {
		t.Errorf("失败应保留源索引行，exists=%v err=%v", exists, err)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "方案/002-方案.pptx"); n != 1 {
		t.Errorf("回滚不应删除既有产物索引行，实际 %d 行", n)
	}
	recAfter, err := store.getSplitRecord("方案.pptx")
	if err != nil || recAfter == nil {
		t.Fatalf("失败不应删除拆分记录：rec=%+v err=%v", recAfter, err)
	}
	if !fingerprintsUsable(recAfter) {
		t.Errorf("失败不应改写指纹：%+v", recAfter.fingerprints)
	}
}

// TestAssetSplitReplace_FingerprintAllowsUnchangedOldArtifact 旧产物字节未变：
// 归属凭证命中（size + SHA-256 全等）时备份让位成功，重拆成功删源，备份在删源前
// 清理，目录无 .replace-bak 残留。与上条「同长异哈希拒绝」构成 R1 正负对照（命中
// 放行 / 不符拒绝）。
func TestAssetSplitReplace_FingerprintAllowsUnchangedOldArtifact(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹让位库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "保留.pptx", 2)
	outDir := filepath.Join(dir, "保留")
	rec := splitRecordRowForTest(t, svc, lib.ID, "保留.pptx")
	if !fingerprintsUsable(rec) {
		t.Fatalf("前置：记录应有可用指纹，实际 %+v", rec)
	}
	for _, fp := range rec.fingerprints {
		if !fingerprintMatch(filepath.Join(outDir, fp.Name), fp) {
			t.Fatalf("前置：指纹应与磁盘旧产物一致（%+v）", fp)
		}
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "保留.pptx", "replace")
	if err != nil {
		t.Fatalf("指纹命中的历史产物应允许备份让位：%v", err)
	}
	if res.Skipped || !res.SourceRemoved || res.PageCount != 2 {
		t.Fatalf("应完成 2 页重拆并删源：%+v", res)
	}
	for _, name := range []string{"001-保留.pptx", "002-保留.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "new-bytes" {
			t.Errorf("新产物应按新内容占位（%s, err=%v, content=%q）", name, readErr, got)
		}
	}
	assertNoReplaceBackup(t, outDir)
	if _, statErr := os.Stat(src); !os.IsNotExist(statErr) {
		t.Errorf("成功后源文件应消失，stat err=%v", statErr)
	}
	if got := recycleCalls(); len(got) != 1 || got[0] != src {
		t.Errorf("重拆成功应把源移入回收站一次，实际 %v", got)
	}
}

// TestAssetSplitReplace_FingerprintUnusableFallsBackToName 指纹不可用保持现行按名
// 覆盖行为（兼容 v10 前记录）：空串 / 条目数与 page_count 不符 / JSON 损坏三种
// 形态一律回落，目标命中 oldArtifacts 即备份让位，重拆成功并重写指纹。
// 每例都把目标页产物改成用户内容——只有回落分支才允许覆盖成功，从而使「回落」
// 与「指纹命中放行」可区分（该覆盖风险是 v10 前记录的既定行为，本用例显式钉住）。
func TestAssetSplitReplace_FingerprintUnusableFallsBackToName(t *testing.T) {
	cases := []struct {
		name string
		// rewrite 依据首次拆分记录产出要写入 artifact_fingerprints 的列值
		rewrite func(rec *assetSplitRecord) string
	}{
		{
			name:    "空串（v10 前记录）",
			rewrite: func(*assetSplitRecord) string { return "" },
		},
		{
			name: "条目数与 page_count 不符",
			rewrite: func(rec *assetSplitRecord) string {
				return encodeSplitFingerprints(rec.fingerprints[:1])
			},
		},
		{
			name:    "JSON 损坏",
			rewrite: func(*assetSplitRecord) string { return "{not-json" },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestAssetService(t)
			dir := t.TempDir()
			lib := mustCreateLibrary(t, svc, "指纹缺失回落库", dir)
			src, _ := resplitFixture(t, svc, lib, dir, "旧记录.pptx", 2)
			outDir := filepath.Join(dir, "旧记录")
			rec := splitRecordRowForTest(t, svc, lib.ID, "旧记录.pptx")
			if !fingerprintsUsable(rec) {
				t.Fatalf("前置：首次拆分记录应有可用指纹，实际 %+v", rec)
			}
			store := svc.openStoreForTest(t, lib.ID)
			if _, err := store.db.Exec(
				"UPDATE split_records SET artifact_fingerprints = ? WHERE source_rel_path = ?", tc.rewrite(rec), "旧记录.pptx",
			); err != nil {
				t.Fatalf("改写记录指纹: %v", err)
			}
			if rec := splitRecordRowForTest(t, svc, lib.ID, "旧记录.pptx"); fingerprintsUsable(rec) {
				t.Fatalf("前置：指纹应不可用，实际 %+v", rec)
			}
			userPage := filepath.Join(outDir, "002-旧记录.pptx")
			if err := os.WriteFile(userPage, []byte("user-content"), 0644); err != nil {
				t.Fatalf("写用户同名文件: %v", err)
			}
			svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
			move, recycleCalls := fakeRecycleBin(t)
			svc.moveToRecycleBinFn = move

			res, err := svc.SplitAssetFile(context.Background(), lib.ID, "旧记录.pptx", "replace")
			if err != nil {
				t.Fatalf("指纹不可用应保持按名覆盖（兼容 v10 前记录）：%v", err)
			}
			if res.Skipped || !res.SourceRemoved {
				t.Fatalf("指纹不可用时重拆应成功：%+v", res)
			}
			for _, name := range []string{"001-旧记录.pptx", "002-旧记录.pptx"} {
				if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "new-bytes" {
					t.Errorf("旧产物应按名让位并写入新内容（%s, err=%v, content=%q）", name, readErr, got)
				}
			}
			assertNoReplaceBackup(t, outDir)
			if got := recycleCalls(); len(got) != 1 || got[0] != src {
				t.Errorf("重拆成功应把源移入回收站一次，实际 %v", got)
			}
			// 重拆成功重写指纹，后续清理恢复内容判定
			if rec := splitRecordRowForTest(t, svc, lib.ID, "旧记录.pptx"); !fingerprintsUsable(rec) {
				t.Errorf("成功重拆应写入可用指纹：%+v", rec)
			}
		})
	}
}

// TestAssetSplitReplace_FingerprintUsableMissingPageEntryRejects 指纹可用（条目数
// 与 page_count 相等）但目标页无对应条目（脏数据）：按名查不到该页指纹即不可
// 判定归属，按用户文件拒绝覆盖并回滚；目标页与源原样保留，前序页备份还原。
func TestAssetSplitReplace_FingerprintUsableMissingPageEntryRejects(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹缺页库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "缺页.pptx", 2)
	outDir := filepath.Join(dir, "缺页")
	rec := splitRecordRowForTest(t, svc, lib.ID, "缺页.pptx")
	if !fingerprintsUsable(rec) {
		t.Fatalf("前置：首次拆分记录应有可用指纹，实际 %+v", rec)
	}
	// 两条条目仍满足 len == page_count（整体判定为可用），但都不叫第 2 页目标名
	fps := []splitArtifactFingerprint{
		rec.fingerprints[0],
		{Page: 2, Name: "002-缺页-不存在.pptx", Size: 1, SHA256: strings.Repeat("0", 64)},
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec(
		"UPDATE split_records SET artifact_fingerprints = ? WHERE source_rel_path = ?",
		encodeSplitFingerprints(fps), "缺页.pptx",
	); err != nil {
		t.Fatalf("改写记录指纹: %v", err)
	}
	if rec := splitRecordRowForTest(t, svc, lib.ID, "缺页.pptx"); !fingerprintsUsable(rec) {
		t.Fatalf("前置：改写后指纹应仍判定可用（len 与 page_count 相等），实际 %+v", rec)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	outcome, err := resplitReplaceForTest(t, svc, lib, "缺页.pptx")
	if err != nil {
		t.Fatalf("业务失败应为 nil error，实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed || !outcome.rolledBack {
		t.Fatalf("目标页无指纹条目应拒绝覆盖：%+v", outcome)
	}
	if !strings.Contains(outcome.message, "第 2 页目标文件「002-缺页.pptx」") {
		t.Errorf("失败消息应指向第 2 页，实际 %q", outcome.message)
	}
	// 第 1 页指纹命中已让位，回滚后还原旧内容；第 2 页无条目从未备份
	for _, name := range []string{"001-缺页.pptx", "002-缺页.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "old-bytes" {
			t.Errorf("拒绝覆盖后旧产物应原样保留（%s, err=%v, content=%q）", name, readErr, got)
		}
	}
	assertNoReplaceBackup(t, outDir)
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("失败应保留源文件：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("失败不得移入回收站，实际 %v", got)
	}
}

// replaceFingerprintCaseFoldFixture 平台大小写用例共享夹具（R7：共享 fixture 留
// 主文件，平台断言拆 _windows/_other）：首次 replace 2 页后把记录第 2 页指纹名改成
// 与磁盘实际名仅大小写不同（模拟历史记录写入的大小写），并注入核心/回收站 fake。
func prepareReplaceFingerprintCaseFoldFixture(t *testing.T) (svc *AssetLibraryService, lib model.AssetLibrary, src, outDir string, recycleCalls func() []string) {
	t.Helper()
	svc = newTestAssetService(t)
	dir := t.TempDir()
	lib = mustCreateLibrary(t, svc, "指纹大小写库", dir)
	src, _ = resplitFixture(t, svc, lib, dir, "Plan.pptx", 2)
	outDir = filepath.Join(dir, "Plan")

	store := svc.openStoreForTest(t, lib.ID)
	rec, err := store.getSplitRecord("Plan.pptx")
	if err != nil || rec == nil {
		t.Fatalf("读拆分记录：rec=%+v err=%v", rec, err)
	}
	fps := append([]splitArtifactFingerprint(nil), rec.fingerprints...)
	flipped := false
	for i := range fps {
		if fps[i].Page == 2 {
			fps[i].Name = "002-plan.pptx" // 与磁盘实际名 002-Plan.pptx 仅大小写不同
			flipped = true
		}
	}
	if !flipped {
		t.Fatalf("前置：记录应有第 2 页指纹，实际 %+v", rec.fingerprints)
	}
	if _, err := store.db.Exec(
		"UPDATE split_records SET artifact_fingerprints = ? WHERE source_rel_path = ?",
		encodeSplitFingerprints(fps), "Plan.pptx",
	); err != nil {
		t.Fatalf("改写指纹名大小写: %v", err)
	}

	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	move, calls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	return svc, lib, src, outDir, calls
}

// TestAssetSplitReplace_FingerprintForNameMultiHitUnresolvable 同名多命中视为
// 不可判定：记录中折叠同名的多条条目无法确定归属，调用方按用户文件拒绝覆盖。
func TestAssetSplitReplace_FingerprintForNameMultiHitUnresolvable(t *testing.T) {
	record := &assetSplitRecord{fingerprints: []splitArtifactFingerprint{
		{Page: 2, Name: "002-同名.pptx", Size: 1, SHA256: "aa"},
		{Page: 3, Name: "002-同名.pptx", Size: 1, SHA256: "bb"},
	}}
	if _, ok := fingerprintForName(record, "002-同名.pptx"); ok {
		t.Error("同名多命中应返回不可判定（ok=false）")
	}
}

// TestAssetSplitReplace_FingerprintNameMultiHitEndToEnd 折叠多命中端到端（R7）：
// 记录写两条同名指纹（精确重复），经 splitOneFileReplace 断言拒绝并回滚——
// 第 1 页指纹正常、第 2 页按名不可判定 → 视为用户文件拒绝，前序页备份还原、
// 用户页与源原样保留、无 .replace-bak 残留。
func TestAssetSplitReplace_FingerprintNameMultiHitEndToEnd(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同名多命中库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "同名.pptx", 3)
	outDir := filepath.Join(dir, "同名")
	rec := splitRecordRowForTest(t, svc, lib.ID, "同名.pptx")
	if !fingerprintsUsable(rec) {
		t.Fatalf("前置：首次拆分记录应有可用指纹，实际 %+v", rec)
	}
	// 第 1 页保留正确条目；第 2/3 页指纹都写成「002-同名.pptx」（精确重复）→
	// 第 2 页按名不可判定，第 3 页页名仍存在于 oldArtifacts 但不会被走到。
	fps := []splitArtifactFingerprint{
		rec.fingerprints[0],
		{Page: 2, Name: "002-同名.pptx", Size: rec.fingerprints[1].Size, SHA256: rec.fingerprints[1].SHA256},
		{Page: 3, Name: "002-同名.pptx", Size: rec.fingerprints[2].Size, SHA256: rec.fingerprints[2].SHA256},
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec(
		"UPDATE split_records SET artifact_fingerprints = ? WHERE source_rel_path = ?",
		encodeSplitFingerprints(fps), "同名.pptx",
	); err != nil {
		t.Fatalf("改写记录指纹: %v", err)
	}
	if rec := splitRecordRowForTest(t, svc, lib.ID, "同名.pptx"); !fingerprintsUsable(rec) {
		t.Fatalf("前置：改写后指纹应仍判定可用，实际 %+v", rec)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("new-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	outcome, err := resplitReplaceForTest(t, svc, lib, "同名.pptx")
	if err != nil {
		t.Fatalf("业务失败应为 nil error，实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed || !outcome.rolledBack {
		t.Fatalf("同名多命中应拒绝并回滚：%+v", outcome)
	}
	if !strings.Contains(outcome.message, "第 2 页目标文件「002-同名.pptx」") {
		t.Errorf("失败消息应指向第 2 页，实际 %q", outcome.message)
	}
	if !strings.Contains(outcome.message, "不属于历史拆分产物") {
		t.Errorf("多命中不可判定应按用户文件文案，实际 %q", outcome.message)
	}
	// 第 1 页已备份让位 → 回滚还原；第 2/3 页未触碰
	for _, name := range []string{"001-同名.pptx", "002-同名.pptx", "003-同名.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "old-bytes" {
			t.Errorf("拒绝后旧产物应原样保留（%s, err=%v, content=%q）", name, readErr, got)
		}
	}
	assertNoReplaceBackup(t, outDir)
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("失败应保留源文件：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("失败不得移入回收站，实际 %v", got)
	}
}

// TestAssetSplitReplace_CoreIntermediateNameGuardRejects 核心中间态同名文件守卫
// （R5）：ownsOutDir 为真且目录内已有 `<base>-<纯数字>.pptx` 时，核心调用前即拒绝，
// 核心零调用、该文件字节不变、源保留、返回 failed（不进入回滚写动作）。
func TestAssetSplitReplace_CoreIntermediateNameGuardRejects(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "中间态守卫库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "守卫.pptx", 2)
	outDir := filepath.Join(dir, "守卫")
	// 核心中间态命名（<源主干>-<纯数字>.pptx）与产品已改名形态并存
	coreState := filepath.Join(outDir, "守卫-009.pptx")
	if err := os.WriteFile(coreState, []byte("user-core-bytes"), 0644); err != nil {
		t.Fatalf("写核心中间态同名文件: %v", err)
	}
	coreCalled := false
	svc.splitCoreFn = func(context.Context, string, func(done, total int)) (model.PptxSplitResult, error) {
		coreCalled = true
		return model.PptxSplitResult{}, errors.New("不应调用拆分核心")
	}
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	outcome, err := resplitReplaceForTest(t, svc, lib, "守卫.pptx")
	if err != nil {
		t.Fatalf("守卫拒绝属业务失败（nil error），实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed {
		t.Fatalf("中间态同名文件应拒绝拆分：%+v", outcome)
	}
	wantMsg := "目录中已存在与拆分核心产物同名的文件（守卫-009.pptx）"
	if !strings.Contains(outcome.message, wantMsg) {
		t.Errorf("失败消息应含 %q，实际 %q", wantMsg, outcome.message)
	}
	if coreCalled {
		t.Error("中间态守卫命中不得调用拆分核心")
	}
	if got, readErr := os.ReadFile(coreState); readErr != nil || string(got) != "user-core-bytes" {
		t.Errorf("同名文件应原样保留（err=%v, content=%q）", readErr, got)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("守卫拒绝应保留源文件：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("守卫拒绝不得移入回收站，实际 %v", got)
	}
}

// TestAssetSplitReplace_BackupReverifyMismatchRejects 备份 rename 后复验（R3
// TOCTOU 收口）：rename 后对备份文件复验指纹，不符则换回原路径并返回内容不一致
// 错误；文件回到原位、无 .replace-bak 残留。正向对照：指纹相符返回备份路径。
func TestAssetSplitReplace_BackupReverifyMismatchRejects(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "001-复验.pptx")
	if err := os.WriteFile(oldPath, []byte("old-bytes"), 0644); err != nil {
		t.Fatalf("写历史产物: %v", err)
	}
	// 同长（9 字节）但哈希不符：复验必须走到 SHA-256 比较
	mismatch := splitArtifactFingerprint{
		Page: 1, Name: "001-复验.pptx", Size: int64(len("old-bytes")),
		SHA256: strings.Repeat("0", 64),
	}
	if _, kept, err := svc.backupReplaceArtifactForSurrender(oldPath, dir, "001-复验.pptx", mismatch); !errors.Is(err, errSplitArtifactContentMismatch) {
		t.Fatalf("复验不符应返回内容不一致错误，实际：%v", err)
	} else if kept != "" {
		t.Errorf("换回成功时不应返回保留备份路径，实际 %q", kept)
	}
	if got, readErr := os.ReadFile(oldPath); readErr != nil || string(got) != "old-bytes" {
		t.Errorf("复验不符应换回原路径（err=%v, content=%q）", readErr, got)
	}
	assertNoReplaceBackup(t, dir)

	// 正向对照：指纹相符 → 备份路径存在、原路径消失
	match := splitFingerprintOfTest(t, oldPath, 1)
	bak, kept, err := svc.backupReplaceArtifactForSurrender(oldPath, dir, "001-复验.pptx", match)
	if err != nil {
		t.Fatalf("复验相符应允许备份让位：%v", err)
	}
	if kept != "" {
		t.Errorf("复验相符不应返回保留备份路径，实际 %q", kept)
	}
	if _, statErr := os.Stat(bak); statErr != nil {
		t.Errorf("备份文件应存在：%v", statErr)
	}
	if _, statErr := os.Stat(oldPath); !os.IsNotExist(statErr) {
		t.Errorf("原路径应已让位，stat err=%v", statErr)
	}
}

// TestAssetSplitReplace_FingerprintRollbackRestoresPriorBackups 回滚路径：前序页
// 已备份让位后，后续页归属校验失败 → 回滚删除本次新产物、把备份逐页还原原内容，
// 目录无 .replace-bak 残留，用户改写的最后一页保持不动，源保留；R7：还原页的
// files.size/hash 与旧值一致（索引行自愈，不只断条数）。
func TestAssetSplitReplace_FingerprintRollbackRestoresPriorBackups(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹回滚还原库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "三段.pptx", 3)
	outDir := filepath.Join(dir, "三段")
	userPage := filepath.Join(outDir, "003-三段.pptx")
	// 与旧产物 old-bytes 等长（9 字节）不同内容：拒绝必须由 SHA-256 判定产生
	if err := os.WriteFile(userPage, []byte("userbytes"), 0644); err != nil {
		t.Fatalf("写用户同名文件: %v", err)
	}
	// 记录回滚前的索引基线（size/hash），回滚还原后应自愈一致
	type sizeHash struct {
		size int64
		hash string
	}
	before := make(map[string]sizeHash, 3)
	for _, rel := range []string{"三段/001-三段.pptx", "三段/002-三段.pptx", "三段/003-三段.pptx"} {
		size, hash := fileSizeHashForTest(t, svc, lib.ID, rel)
		before[rel] = sizeHash{size: size, hash: hash}
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("new-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	outcome, err := resplitReplaceForTest(t, svc, lib, "三段.pptx")
	if err != nil {
		t.Fatalf("指纹不符属业务失败，实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed || !outcome.rolledBack {
		t.Fatalf("第 3 页指纹不符应失败并回滚：%+v", outcome)
	}
	if !strings.Contains(outcome.message, "第 3 页目标文件「003-三段.pptx」") {
		t.Errorf("失败消息应指向第 3 页，实际 %q", outcome.message)
	}
	// 前两页均已备份让位，回滚后逐页还原旧内容
	for _, name := range []string{"001-三段.pptx", "002-三段.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "old-bytes" {
			t.Errorf("回滚应还原前序页旧产物（%s, err=%v, content=%q）", name, readErr, got)
		}
	}
	// 用户改写的第 3 页保持不动
	if got, readErr := os.ReadFile(userPage); readErr != nil || string(got) != "userbytes" {
		t.Errorf("用户文件应原样保留（err=%v, content=%q）", readErr, got)
	}
	assertNoReplaceBackup(t, outDir)
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("失败应保留源文件：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("失败不得移入回收站，实际 %v", got)
	}
	// 既有三条产物索引行完整保留（还原时经 processScanFile 自愈）
	for _, rel := range []string{"三段/001-三段.pptx", "三段/002-三段.pptx", "三段/003-三段.pptx"} {
		if n := countIndexRowsByPath(t, svc, lib.ID, rel); n != 1 {
			t.Errorf("回滚后产物索引行应保留（%s），实际 %d 行", rel, n)
		}
	}
	// R7：还原页索引行 size/hash 与旧值一致（第 1/2 页经重解析自愈；第 3 页未触碰）
	for _, rel := range []string{"三段/001-三段.pptx", "三段/002-三段.pptx", "三段/003-三段.pptx"} {
		size, hash := fileSizeHashForTest(t, svc, lib.ID, rel)
		if size != before[rel].size || hash != before[rel].hash {
			t.Errorf("还原页索引行应自愈到旧值（%s）：got size=%d hash=%q want size=%d hash=%q",
				rel, size, hash, before[rel].size, before[rel].hash)
		}
	}
}

// TestIsReplaceCoreArtifactNameExactForms 核心中间态命名判定的精确形态表（平台无关
// 部分）：主干含 `-`/数字、纯数字主干、历史页序位数、已改名产物形态均不得误判。
// Windows 整名折叠断言拆至 _windows_test.go / _other_test.go（平台约定）。
func TestIsReplaceCoreArtifactNameExactForms(t *testing.T) {
	cases := []struct {
		name, base string
		want       bool
	}{
		{"守卫-009.pptx", "守卫", true},
		{"守卫-09.pptx", "守卫", true},        // 历史 %02d 形态：2 位页序仍拦截
		{"守卫-9.pptx", "守卫", false},        // 1 位页序：核心永不写，用户文件不得误拦（R2）
		{"方案-9.pptx", "方案", false},        // R2 显式场景：1 位页序不拦
		{"方案-09.pptx", "方案", true},        // R2 显式场景：2 位页序拦
		{"方案-009.pptx", "方案", true},       // 当前 %03d 形态拦
		{"方案-2.pptx", "方案", false},        // 1 位页序（用户文件/非核心形态）不拦
		{"守卫-abc.pptx", "守卫", false},      // 页序段非纯数字
		{"守卫-009-.pptx", "守卫", false},     // 页序段为空
		{"-009.pptx", "守卫", false},        // 主干段为空
		{"守卫.pptx", "守卫", false},          // 无页序段
		{"守卫-009.txt", "守卫", false},       // 非 pptx
		{"001-守卫-2.pptx", "守卫-2", false},  // 已改名形态，不得误判为核心中间态
		{"守卫-2-003.pptx", "守卫-2", true},   // 主干含 `-`/尾数字仍正确切分
		{"123-007.pptx", "123", true},     // 纯数字主干（核心用源主干，不加 -p）
		{"001-123-p.pptx", "123", false},  // 纯数字主干的已改名产物
		{"123-p-007.pptx", "123-p", true}, // replaceArtifactBase 场景显式主干
	}
	for _, tc := range cases {
		if got := isReplaceCoreArtifactName(tc.name, tc.base); got != tc.want {
			t.Errorf("isReplaceCoreArtifactName(%q, %q) = %v, want %v", tc.name, tc.base, got, tc.want)
		}
	}
}

// TestAssetSplitReplace_CoreGuardNotTriggeredByFinalArtifacts 端到端回归：主干含
// `-`/数字（`方案-2`）时正常重拆不得被核心中间态守卫误拦——上次产物
// `001-方案-2.pptx` 的最后一段虽是纯数字，但前缀 `001-方案` 不等于主干，守卫须放行。
func TestAssetSplitReplace_CoreGuardNotTriggeredByFinalArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "主干含横线库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "方案-2.pptx", 3)
	outDir := filepath.Join(dir, "方案-2")
	for _, name := range []string{"001-方案-2.pptx", "002-方案-2.pptx", "003-方案-2.pptx"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("前置：上次产物应存在（%s）：%v", name, err)
		}
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("new-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案-2.pptx", "replace")
	if err != nil {
		t.Fatalf("主干含 `-`/数字时正常重拆不得被守卫误拦：%v", err)
	}
	if res.Skipped || !res.SourceRemoved || res.PageCount != 3 {
		t.Fatalf("应完成 3 页重拆并删源：%+v", res)
	}
	for _, name := range []string{"001-方案-2.pptx", "002-方案-2.pptx", "003-方案-2.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "new-bytes" {
			t.Errorf("新产物应按新内容占位（%s, err=%v, content=%q）", name, readErr, got)
		}
	}
	assertNoReplaceBackup(t, outDir)
	if got := recycleCalls(); len(got) != 1 || got[0] != src {
		t.Errorf("重拆成功应把源移入回收站一次，实际 %v", got)
	}
}

// TestAssetSplitReplace_SingleDigitUserFileKeptOnLegacyRecord R1 高危回归：1 位
// 页序用户文件（`方案-9.pptx`）不得进入 oldArtifacts。指纹不可用的 v10 前旧记录
// 在重拆成功时会按名删除 oldArtifacts 中的文件——若产物形态判定接受任意位数，
// 用户文件即被误删且不可恢复。口径与 isReplaceCoreArtifactName 统一为 ≥2 位后，
// 该文件既不被守卫拒绝、也不进历史产物集合。
//
// 变异验证：把 isReplaceArtifactNameExact 的页序长度条件改回「任意位数」，本条
// 断言「用户文件保留」变红（文件被按名删除）。
func TestAssetSplitReplace_SingleDigitUserFileKeptOnLegacyRecord(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "单位页序用户文件库", dir)
	resplitFixture(t, svc, lib, dir, "方案.pptx", 2)
	store := svc.openStoreForTest(t, lib.ID)
	// 模拟 v10 前旧记录：产物指纹不可用 → cleanupStaleReplaceArtifacts 回落按名删除
	if _, err := store.db.Exec(
		"UPDATE split_records SET artifact_fingerprints = '', retained_fingerprints = '' WHERE source_rel_path = ?",
		"方案.pptx"); err != nil {
		t.Fatalf("清空旧记录指纹: %v", err)
	}
	outDir := filepath.Join(dir, "方案")
	userPage := filepath.Join(outDir, "方案-9.pptx")
	if err := os.WriteFile(userPage, []byte("user-bytes"), 0644); err != nil {
		t.Fatalf("写用户 1 位页序文件: %v", err)
	}

	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace")
	if err != nil {
		t.Fatalf("1 位页序用户文件不得被守卫拒绝：%v", err)
	}
	if res.Skipped || !res.SourceRemoved || res.PageCount != 2 {
		t.Fatalf("应完成 2 页重拆并删源：%+v", res)
	}
	if got, readErr := os.ReadFile(userPage); readErr != nil || string(got) != "user-bytes" {
		t.Errorf("1 位页序用户文件不得进入历史产物集合（err=%v content=%q）", readErr, got)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "方案/方案-9.pptx"); n != 0 {
		t.Errorf("用户文件不应被索引，实际 %d 行", n)
	}
	for _, name := range []string{"001-方案.pptx", "002-方案.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "new-bytes" {
			t.Errorf("新产物应按新内容占位（%s, err=%v, content=%q）", name, readErr, got)
		}
	}
	assertNoReplaceBackup(t, outDir)
	if got := recycleCalls(); len(got) != 1 {
		t.Errorf("重拆成功应把源移入回收站一次，实际 %v", got)
	}
}
