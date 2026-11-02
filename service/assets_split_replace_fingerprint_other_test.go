//go:build !windows

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// TestAssetSplitReplace_FingerprintNameCaseFoldNonWindows 非 Windows 大小写敏感：
// 记录指纹名与磁盘实际名大小写不同（002-plan.pptx vs 002-Plan.pptx）是不同文件，
// sameNameFold 不折叠 → 归属不可判定，按用户文件拒绝覆盖并回滚；旧产物原样保留、
// 源保留、无 .replace-bak 残留。
func TestAssetSplitReplace_FingerprintNameCaseFoldNonWindows(t *testing.T) {
	svc, lib, src, outDir, recycleCalls := prepareReplaceFingerprintCaseFoldFixture(t)

	outcome, err := resplitReplaceForTest(t, svc, lib, "Plan.pptx")
	if err != nil {
		t.Fatalf("业务失败应为 nil error，实际：%v", err)
	}
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

// TestIsReplaceCoreArtifactName_NoFoldNonWindows 非 Windows 大小写敏感：核心中间态
// 判定只做精确匹配（折叠会把不同物理文件误判为同名，失效方向为误拒正常重拆）。
func TestIsReplaceCoreArtifactName_NoFoldNonWindows(t *testing.T) {
	if isReplaceCoreArtifactName("守卫-009.PPTX", "守卫") {
		t.Error("非 Windows 不应折叠扩展名大小写")
	}
	if isReplaceCoreArtifactName("GUARD-009.pptx", "guard") {
		t.Error("非 Windows 不应折叠主干大小写")
	}
	if !isReplaceCoreArtifactName("守卫-009.pptx", "守卫") {
		t.Error("精确形态在非 Windows 也应命中")
	}
}
