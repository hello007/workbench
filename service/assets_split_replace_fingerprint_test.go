package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- replace 备份让位前的产物指纹校验（11-02-replace） ----------
//
// v10 页级指纹此前只服务于三处清理路径；就地替换的「备份让位」仍按命名形态认定
// 历史产物——用户把容器目录内某页产物替换成自有内容（同名）时，重拆会把它当历史
// 产物备份并在成功后删除，内容不可恢复。本组用例覆盖：同名不同内容拒绝覆盖（含
// 前序页备份还原）、旧产物字节未变正常让位、指纹缺失回落按名覆盖、大小写折叠平台
// 分流。失败路径的校验点在备份 rename 之前，故天然零 .replace-bak 残留。
//
// fixture 一律走 buildAssetsPptx + 注入 fake 核心/解析/回收站，不依赖真实 Office
// 与外部状态（见 docs/spec/test-stability.md）。

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

// TestAssetSplitReplace_FingerprintRejectsUserReplacedPage 用户同名不同内容文件：
// 记录指纹可用时，目标名命中 oldArtifacts 但内容与指纹不符视为用户文件，拒绝覆盖
// 并回滚本次已产出页；用户文件字节不变、前序页备份还原、目录无 .replace-bak、
// 源与源索引行保留。
func TestAssetSplitReplace_FingerprintRejectsUserReplacedPage(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹拒绝覆盖库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "方案.pptx", 2)
	outDir := filepath.Join(dir, "方案")
	userPage := filepath.Join(outDir, "002-方案.pptx")
	if err := os.WriteFile(userPage, []byte("user-content"), 0644); err != nil {
		t.Fatalf("写用户同名文件: %v", err)
	}
	// 前置：记录指纹可用，且第 2 页指纹描述的是旧产物内容（与用户文件不同）
	rec := splitRecordRowForTest(t, svc, lib.ID, "方案.pptx")
	if !fingerprintsUsable(rec) {
		t.Fatalf("前置：首次 replace 记录应带可用指纹，实际 %+v", rec)
	}
	if fp, ok := fingerprintForName(rec, "002-方案.pptx"); !ok || fingerprintMatch(userPage, fp) {
		t.Fatalf("前置：用户文件应与旧指纹不符（fp=%+v ok=%v）", fp, ok)
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
	wantMsg := "第 2 页目标文件「002-方案.pptx」已存在且不属于历史拆分产物"
	if !strings.Contains(outcome.message, wantMsg) {
		t.Errorf("失败消息应与既有目标占用分支一致（含 %q），实际 %q", wantMsg, outcome.message)
	}
	// 用户文件字节不变
	if got, readErr := os.ReadFile(userPage); readErr != nil || string(got) != "user-content" {
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
// 指纹命中（size + SHA-256 全等）时备份让位成功，重拆成功删源，备份在删源前
// 清理，目录无 .replace-bak 残留。
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

// TestAssetSplitReplace_FingerprintNameCaseFoldByPlatform 平台大小写语义：记录指纹
// 名与磁盘实际名仅大小写不同（模拟历史记录写入时的命名大小写）。Windows 下
// fingerprintForName 折叠命中，且 fingerprintMatch 按磁盘实际路径（002-Plan.pptx）
// 的 size+哈希判定，不误判用户文件 → 正常让位；Linux/macOS 不折叠，大小写变体
// 视为用户文件 → 失败回滚。
func TestAssetSplitReplace_FingerprintNameCaseFoldByPlatform(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹大小写库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "Plan.pptx", 2)
	outDir := filepath.Join(dir, "Plan")

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
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	outcome, err := resplitReplaceForTest(t, svc, lib, "Plan.pptx")
	if err != nil {
		t.Fatalf("业务失败应为 nil error，实际：%v", err)
	}
	if runtime.GOOS == "windows" {
		// 折叠命中 + 按实际路径哈希命中 → 正常备份让位
		if outcome.status == model.AssetSplitStatusFailed {
			t.Fatalf("Windows 下指纹名大小写不同应折叠命中并让位：%+v", outcome)
		}
		for _, name := range []string{"001-Plan.pptx", "002-Plan.pptx"} {
			if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "new-bytes" {
				t.Errorf("Windows 让位后产物应为新内容（%s, err=%v, content=%q）", name, readErr, got)
			}
		}
		assertNoReplaceBackup(t, outDir)
		if _, statErr := os.Stat(src); !os.IsNotExist(statErr) {
			t.Errorf("成功后源文件应消失，stat err=%v", statErr)
		}
		if got := recycleCalls(); len(got) != 1 || got[0] != src {
			t.Errorf("重拆成功应把源移入回收站一次，实际 %v", got)
		}
		return
	}
	// 非 Windows：大小写敏感，指纹条目大小写不符 = 不是本记录产物 → 用户文件
	if outcome.status != model.AssetSplitStatusFailed || !outcome.rolledBack {
		t.Fatalf("非 Windows 大小写变体应视为用户文件拒绝覆盖：%+v", outcome)
	}
	wantMsg := "第 2 页目标文件「002-Plan.pptx」已存在且不属于历史拆分产物"
	if !strings.Contains(outcome.message, wantMsg) {
		t.Errorf("失败消息应含 %q，实际 %q", wantMsg, outcome.message)
	}
	for _, name := range []string{"001-Plan.pptx", "002-Plan.pptx"} {
		if got, readErr := os.ReadFile(filepath.Join(outDir, name)); readErr != nil || string(got) != "old-bytes" {
			t.Errorf("拒绝覆盖后旧内容应原样保留（%s, err=%v, content=%q）", name, readErr, got)
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

// TestAssetSplitReplace_FingerprintRollbackRestoresPriorBackups 回滚路径：前序页
// 已备份让位后，后续页指纹校验失败 → 回滚删除本次新产物、把备份逐页还原原内容，
// 目录无 .replace-bak 残留，用户改写的最后一页保持不动，源保留。
func TestAssetSplitReplace_FingerprintRollbackRestoresPriorBackups(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "指纹回滚还原库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "三段.pptx", 3)
	outDir := filepath.Join(dir, "三段")
	userPage := filepath.Join(outDir, "003-三段.pptx")
	if err := os.WriteFile(userPage, []byte("user-third-page"), 0644); err != nil {
		t.Fatalf("写用户同名文件: %v", err)
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
	if got, readErr := os.ReadFile(userPage); readErr != nil || string(got) != "user-third-page" {
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
}
