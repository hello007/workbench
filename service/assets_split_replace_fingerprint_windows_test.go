//go:build windows

package service

import (
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// TestAssetSplitReplace_FingerprintNameCaseFoldWindows Windows 大小写语义：记录
// 指纹名与磁盘实际名仅大小写不同（模拟历史记录写入时的大小写），sameNameFold
// 折叠命中，fingerprintMatch 按磁盘实际路径（002-Plan.pptx）的 size+哈希判定，
// 不误判用户文件 → 正常备份让位并删源。
//
// 该分支仅 Windows 可执行，由 CI windows-latest job 覆盖，见
// docs/spec/pptx-split.md §9.7。
func TestAssetSplitReplace_FingerprintNameCaseFoldWindows(t *testing.T) {
	svc, lib, src, outDir, recycleCalls := prepareReplaceFingerprintCaseFoldFixture(t)

	outcome, err := resplitReplaceForTest(t, svc, lib, "Plan.pptx")
	if err != nil {
		t.Fatalf("Windows 下指纹名大小写不同应折叠命中并让位，实际：%v", err)
	}
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
}

// TestIsReplaceCoreArtifactName_WindowsFold Windows 整名大小写折叠：核心中间态
// 判定须折叠主干段与扩展名（`守卫-009.PPTX` 与核心将写出的 `守卫-009.pptx` 是
// 同一物理文件，漏判会让核心静默覆盖用户内容）。
//
// 该分支仅 Windows 可执行，由 CI windows-latest job 覆盖，见
// docs/spec/pptx-split.md §9.7。
func TestIsReplaceCoreArtifactName_WindowsFold(t *testing.T) {
	cases := []struct {
		name, base string
	}{
		{"守卫-009.PPTX", "守卫"},
		{"GUARD-009.pptx", "guard"},
		{"方案-2-007.PPTX", "方案-2"},
		{"GUARD-009.PPTX", "guard"},
	}
	for _, tc := range cases {
		if !isReplaceCoreArtifactName(tc.name, tc.base) {
			t.Errorf("Windows 下应折叠命中：isReplaceCoreArtifactName(%q, %q) = false", tc.name, tc.base)
		}
	}
}
