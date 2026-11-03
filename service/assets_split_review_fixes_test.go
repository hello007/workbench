package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// 本文件覆盖 0eb330b 审核发现（11-03-split-review-fixes）的修复用例：
//   - P1-1：异目录 retained 删除失败条目丢弃 + Warn（source/cache 与 replace 两链路）；
//   - P2-2：「凭证存在但内容不符」在 ownsOutDir 与落点归属放宽两分支的 E2E；
//   - P2-2：cache→replace 跨 target 端到端收敛。

// TestAssetSplitRetained_SourceCrossDirFailureDiscardedWithWarn P1-1（source/cache
// 链路）：本次落点与凭证目录不同时，旧落点残留页删除失败不得把凭证写入新记录
// （凭证名相对旧 output_dir 解析，写入后即错锚到新落点、旧残留成永久孤儿）。
// 失败条目一律丢弃并 Warn；同目录场景的续携由
// TestAssetSplitRetained_CacheCrossDirConsumesOldDirResidual 等既有用例守护。
//
// 变异验证：去掉调用侧 crossDir 丢弃分支（恢复无条件续携）时，本条「retained
// 清空」与「日志含跨落点告警」断言变红。
func TestAssetSplitRetained_SourceCrossDirFailureDiscardedWithWarn(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "异目录丢弃库", dir)
	_, stalePage, oldOutDir := seedReplaceRecordWithCoreFormRetained(t, svc, lib, dir, "异地.pptx")

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	svc.removeArtifactFn = func(string) error { return errors.New("locked") }
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("cache-bytes")}).split
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "异地.pptx", "cache")
	if err != nil {
		t.Fatalf("切 target=cache 应成功（残留删除失败只告警）：%v", err)
	}
	if res.Skipped || res.Target != model.AssetSplitTargetCache {
		t.Fatalf("落点切换不得跳过：%+v", res)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "异地.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetCache {
		t.Fatalf("记录应切到 cache：%+v", rec)
	}
	if samePathFold(rec.outputDir, oldOutDir) {
		t.Fatalf("前置：cache 新落点应与旧 replace 落点不同")
	}
	if len(rec.retained) != 0 {
		t.Errorf("跨落点删除失败的旧落点条目不得写入新记录：%+v", rec.retained)
	}
	if _, statErr := os.Stat(stalePage); statErr != nil {
		t.Errorf("删除失败的旧落点残留应保留在磁盘（%s），stat err=%v", stalePage, statErr)
	}
	// 新落点与新残留同名的第 3 页必须保留本次产物（R3：跨目录同名不互相吞并）
	cachePage := filepath.Join(rec.outputDir, "异地-003.pptx")
	if got, readErr := os.ReadFile(cachePage); readErr != nil || string(got) != "cache-bytes" {
		t.Errorf("新落点第 3 页应保留本次产物内容（err=%v content=%q）", readErr, got)
	}
	if !strings.Contains(logBuf.String(), "跨落点") {
		t.Errorf("跨落点丢弃应给出明确告警，实际日志：%s", logBuf.String())
	}
}

// TestAssetSplitReplace_CrossDirRetainedFailureDiscardedWithWarn P1-1（replace
// 链路）：记录 output_dir 与本次算出 outDir 不同时（记录落点已迁移/改名重链等），
// cleanupRetainedSplitArtifacts 在旧落点删除失败的条目同样丢弃并 Warn；而
// cleanupStaleReplaceArtifacts 产出的本次落点失败条目不受影响（不被误丢）。
//
// 变异验证：去掉 crossDir 丢弃分支时，本条「新记录 retained 为空」断言变红。
func TestAssetSplitReplace_CrossDirRetainedFailureDiscardedWithWarn(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "replace异目录丢弃库", dir)
	_, stalePage, _ := seedReplaceRecordWithCoreFormRetained(t, svc, lib, dir, "跨地.pptx")

	// 构造记录落点 != 本次 outDir：残留副本放到另一个目录，记录 output_dir 指向它
	otherDir := filepath.Join(dir, "旧落点")
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatalf("建旧落点目录: %v", err)
	}
	oldStale := filepath.Join(otherDir, filepath.Base(stalePage))
	content, readErr := os.ReadFile(stalePage)
	if readErr != nil {
		t.Fatalf("读取残留页: %v", readErr)
	}
	if err := os.WriteFile(oldStale, content, 0644); err != nil {
		t.Fatalf("写旧落点残留页: %v", err)
	}
	seedRec := splitRecordRowForTest(t, svc, lib.ID, "跨地.pptx")
	if seedRec == nil || len(seedRec.retained) != 1 {
		t.Fatalf("前置：记录应携带 retained：%+v", seedRec)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("跨地.pptx", "", otherDir, 2,
		"2026-11-03 10:00:00", model.AssetSplitTargetReplace,
		replaceArtifactBase("跨地"), false, seedRec.fingerprints, seedRec.retained); err != nil {
		t.Fatalf("改记录落点: %v", err)
	}

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	svc.removeArtifactFn = func(string) error { return errors.New("locked") }
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("retry-bytes")}).split
	move, _ := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "跨地.pptx", "replace")
	if err != nil {
		t.Fatalf("replace 重拆应成功（旧落点残留删除失败只告警）：%v", err)
	}
	if !res.SourceRemoved || res.PageCount != 2 {
		t.Fatalf("应完成替换并删源：%+v", res)
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "跨地.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetReplace {
		t.Fatalf("记录应保持 replace：%+v", rec)
	}
	if len(rec.retained) != 0 {
		t.Errorf("跨落点删除失败的旧落点条目不得写入新记录：%+v", rec.retained)
	}
	if _, statErr := os.Stat(oldStale); statErr != nil {
		t.Errorf("删除失败的旧落点残留应保留在磁盘（%s），stat err=%v", oldStale, statErr)
	}
	if !strings.Contains(logBuf.String(), "跨落点") {
		t.Errorf("跨落点丢弃应给出明确告警，实际日志：%s", logBuf.String())
	}
}

// TestAssetSplitRetained_OwnsOutDirCoreNameContentMismatchRejected P2-2：ownsOutDir
// 分支的核心同名守卫在「凭证存在但内容不符」时仍须拒绝——用户把 `<stem>-003.pptx`
// 改成**等长不同哈希**（9 字节）后，凭证名命中不足以放行，内容全等才算归属。
// 核心零调用、文件零改动。变异验证：把 fingerprintMatch 改为恒真时本条变红。
func TestAssetSplitRetained_OwnsOutDirCoreNameContentMismatchRejected(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "owns 内容不符库", dir)
	_, stalePage, _ := seedReplaceRecordWithCoreFormRetained(t, svc, lib, dir, "改过.pptx")
	if err := os.WriteFile(stalePage, []byte("old-bytez"), 0644); err != nil {
		t.Fatalf("改写残留页为等长异哈希: %v", err)
	}

	core := &fakeReplaceCore{pages: 2, payload: []byte("retry-bytes")}
	svc.splitCoreFn = core.split
	svc.parseFn = oneSlideParse()
	outcome, err := resplitReplaceForTest(t, svc, lib, "改过.pptx")
	if err != nil {
		t.Fatalf("守卫命中属业务失败结果（nil error），实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed || !strings.Contains(outcome.message, "改过-003.pptx") {
		t.Fatalf("凭证内容不符应早于核心拒绝核心同名文件：%+v", outcome)
	}
	if got, readErr := os.ReadFile(stalePage); readErr != nil || string(got) != "old-bytez" {
		t.Errorf("用户内容不得被改动（err=%v content=%q）", readErr, got)
	}
	if core.calls != 0 {
		t.Errorf("守卫命中时核心不应调用，实际 %d 次", core.calls)
	}
}

// TestAssetSplitRetained_RelaxedOwnershipContentMismatchRejected P2-2：落点归属
// 放宽分支在「凭证存在但内容不符」时返回 false，stray 仍按名拒绝；目录内全部同形
// 文件都内容不符才拒绝（单个命中即放行）。文件零改动、核心零调用。变异验证：
// 去掉 replaceOutDirOwnedByRecord 的 fingerprintMatch 时本条变红。
func TestAssetSplitRetained_RelaxedOwnershipContentMismatchRejected(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "放宽内容不符库", dir)
	_, _, oldOutDir := seedReplaceRecordWithCoreFormRetained(t, svc, lib, dir, "放宽.pptx")

	// 记录落点改到别处 → ownsOutDir 为假，走 firstReplaceArtifactName 放宽分支
	otherDir := filepath.Join(dir, "别处")
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatalf("建别处目录: %v", err)
	}
	seedRec := splitRecordRowForTest(t, svc, lib.ID, "放宽.pptx")
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("放宽.pptx", "", otherDir, 2,
		"2026-11-03 10:00:00", model.AssetSplitTargetReplace,
		replaceArtifactBase("放宽"), false, seedRec.fingerprints, nil); err != nil {
		t.Fatalf("改记录落点: %v", err)
	}
	// 目录内全部同形产物改成等长异哈希：归属存在但内容不符 → 放宽失败
	var changed []string
	entries, err := os.ReadDir(oldOutDir)
	if err != nil {
		t.Fatalf("读落点目录: %v", err)
	}
	for _, e := range entries {
		// 只改写两页已改名产物（`001-放宽` / `002-放宽`）；核心中间态残留
		// `放宽-003`（seed 的 retained 凭证描述文件）不是本用例的 stray 判定面。
		if e.IsDir() || !isReplaceArtifactName(e.Name(), "放宽") ||
			isReplaceCoreArtifactName(e.Name(), "放宽") {
			continue
		}
		p := filepath.Join(oldOutDir, e.Name())
		if err := os.WriteFile(p, []byte("user-byte"), 0644); err != nil {
			t.Fatalf("改写同形文件（%s）: %v", e.Name(), err)
		}
		changed = append(changed, p)
	}
	if len(changed) != 2 {
		t.Fatalf("前置：应改写两页旧产物，实际 %d", len(changed))
	}

	core := &fakeReplaceCore{pages: 2, payload: []byte("retry-bytes")}
	svc.splitCoreFn = core.split
	svc.parseFn = oneSlideParse()
	outcome, err := resplitReplaceForTest(t, svc, lib, "放宽.pptx")
	if err != nil {
		t.Fatalf("放宽失败属业务失败结果（nil error），实际：%v", err)
	}
	if outcome.status != model.AssetSplitStatusFailed ||
		!strings.Contains(outcome.message, "已存在与拆分产物同名的文件") {
		t.Fatalf("凭证内容不符时 stray 应按名拒绝：%+v", outcome)
	}
	for _, p := range changed {
		if got, readErr := os.ReadFile(p); readErr != nil || string(got) != "user-byte" {
			t.Errorf("用户内容不得被改动（%s, err=%v content=%q）", p, readErr, got)
		}
	}
	if core.calls != 0 {
		t.Errorf("守卫命中时核心不应调用，实际 %d 次", core.calls)
	}
}

// TestAssetSplit_TargetSwitchCacheToReplace P2-2：cache→replace 跨 target
// 端到端——源未变不得跳过，旧 derived 落点按 splitArtifactCleanupPlan 清理，
// 新 replace 落点产物进索引且记录不携带 retained。
//
// 变异验证：恢复「跳过判定不比 target」或去掉跨落点清理时，本条
// 「旧 derived 产物收敛」断言变红。
func TestAssetSplit_TargetSwitchCacheToReplace(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "缓存到替换切换库", dir)
	buildAssetsPptx(t, dir, "切换替换.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	first, err := svc.SplitAssetFile(context.Background(), lib.ID, "切换替换.pptx", "cache")
	if err != nil {
		t.Fatalf("首次 cache 拆分: %v", err)
	}
	if first.Skipped || first.Target != model.AssetSplitTargetCache {
		t.Fatalf("首次拆分应成功且落点 cache：%+v", first)
	}
	oldOut := filepath.Join(svc.assetsDir, lib.ID, "derived", "切换替换")
	mustExist(t, filepath.Join(oldOut, "切换替换-001.pptx"))
	mustExist(t, filepath.Join(oldOut, "切换替换-002.pptx"))

	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("replace-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	second, err := svc.SplitAssetFile(context.Background(), lib.ID, "切换替换.pptx", "replace")
	if err != nil {
		t.Fatalf("切换 replace 重拆: %v", err)
	}
	if second.Skipped || second.Target != model.AssetSplitTargetReplace || !second.SourceRemoved {
		t.Fatalf("cache→replace 应完成替换并删源：%+v", second)
	}
	if got := recycleCalls(); len(got) != 1 {
		t.Errorf("源应移入回收站一次，实际 %v", got)
	}
	mustNotExist(t, filepath.Join(oldOut, "切换替换-001.pptx"))
	mustNotExist(t, filepath.Join(oldOut, "切换替换-002.pptx"))
	wantOut := filepath.Join(dir, "切换替换")
	for _, page := range []int{1, 2} {
		mustExist(t, filepath.Join(wantOut, fmt.Sprintf("%03d-切换替换.pptx", page)))
	}
	rec := splitRecordRowForTest(t, svc, lib.ID, "切换替换.pptx")
	if rec == nil || rec.target != model.AssetSplitTargetReplace ||
		!samePathFold(rec.outputDir, wantOut) || len(rec.retained) != 0 {
		t.Errorf("重拆后记录应为 replace 落点且不携带 retained：%+v", rec)
	}
}
