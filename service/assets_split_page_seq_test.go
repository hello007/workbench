package service

import (
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// 本文件覆盖 11-03-split-review-fixes 的页序位数单点化（R2/P1-2）用例：
//   - 所有解析点统一「页序 ≥2 位」，1 位页序用户文件在清理/定位/剔除路径均不被
//     当作产物；2/3/4 位（历史 %02d、当前 %03d、≥1000 页）全部命中；
//   - R1 修复后的清理路径只删产物、保留 1 位页序用户文件与其索引行。

// TestArtifactPageSeqParsers_PageSeqDigits 页序位数单点化表驱动：四个解析入口
// （parseSplitArtifactName 两形态、splitArtifactPageSeq、pageSeqFromArtifactName、
// isPptxSplitDerivative）对 1 位页序返回未命中/0/false，2/3/4 位命中。
func TestArtifactPageSeqParsers_PageSeqDigits(t *testing.T) {
	type parseCase struct {
		name string
		page int
		ok   bool
		desc string
	}
	replaceCases := []parseCase{
		{"9-方案.pptx", 0, false, "1 位页序（replace 形态）不算产物"},
		{"09-方案.pptx", 9, true, "历史 %02d 页序"},
		{"009-方案.pptx", 9, true, "当前 %03d 页序"},
		{"1234-方案.pptx", 1234, true, "≥1000 页 4 位页序"},
		{"方案-01.pptx", 0, false, "source 形态不得被 replace 分支命中"},
	}
	for _, tc := range replaceCases {
		stem, page, ok := parseSplitArtifactName(tc.name, model.AssetSplitTargetReplace)
		if ok != tc.ok || page != tc.page {
			t.Errorf("parseSplitArtifactName(replace) %q = (%q,%d,%v)，want (_,%d,%v)（%s）",
				tc.name, stem, page, ok, tc.page, tc.ok, tc.desc)
		}
	}

	sourceCases := []parseCase{
		{"方案-1.pptx", 0, false, "1 位页序（source 形态）不算产物"},
		{"方案-01.pptx", 1, true, "历史 %02d 页序"},
		{"方案-0001.pptx", 1, true, "4 位页序"},
		{"9-方案.pptx", 0, false, "replace 形态不得被 source 分支命中"},
	}
	for _, tc := range sourceCases {
		stem, page, ok := parseSplitArtifactName(tc.name, model.AssetSplitTargetSource)
		if ok != tc.ok || page != tc.page {
			t.Errorf("parseSplitArtifactName(source) %q = (%q,%d,%v)，want (_,%d,%v)（%s）",
				tc.name, stem, page, ok, tc.page, tc.ok, tc.desc)
		}
	}

	for _, tc := range []struct {
		name, base string
		wantSeq    string
		wantOK     bool
	}{
		{"方案-1.pptx", "方案", "", false},       // 1 位页序：用户文件
		{"方案-01.pptx", "方案", "01", true},     // 历史两位
		{"方案-0001.pptx", "方案", "0001", true}, // 4 位
	} {
		seq, ok := splitArtifactPageSeq(tc.name, tc.base)
		if ok != tc.wantOK || seq != tc.wantSeq {
			t.Errorf("splitArtifactPageSeq(%q,%q) = (%q,%v)，want (%q,%v)",
				tc.name, tc.base, seq, ok, tc.wantSeq, tc.wantOK)
		}
	}

	for _, tc := range []struct {
		name string
		want int
	}{
		{"方案-1.pptx", 0},
		{"方案-01.pptx", 1},
		{"方案-0001.pptx", 1},
		{"方案-1234.pptx", 1234},
	} {
		if got := pageSeqFromArtifactName(tc.name); got != tc.want {
			t.Errorf("pageSeqFromArtifactName(%q) = %d，want %d", tc.name, got, tc.want)
		}
	}

	for _, tc := range []struct {
		rel  string
		want bool
	}{
		{"方案/方案-1.pptx", false},   // 1 位页序用户文件不排除
		{"方案/方案-01.pptx", true},   // 历史两位产物
		{"方案/方案-0001.pptx", true}, // 4 位产物
	} {
		if got := isPptxSplitDerivative(tc.rel); got != tc.want {
			t.Errorf("isPptxSplitDerivative(%q) = %v，want %v", tc.rel, got, tc.want)
		}
	}
}

// TestReplaceArtifactNameAnyStem_PageSeqDigits 已改名形态 anyStem 判定：1 位页序
// 不算产物（不进入移动拦截/批量剔除/二次拆分拒绝），2/3/4 位命中。变异验证：
// 恢复任意位数判定时「9-方案.pptx」用例变红。
func TestReplaceArtifactNameAnyStem_PageSeqDigits(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"9-方案.pptx", false},
		{"09-方案.pptx", true},
		{"009-方案.pptx", true},
		{"1234-方案.pptx", true},
		{"方案-01.pptx", false}, // 非页序前缀形态
		{"09-.pptx", false},   // 主干为空
		{"09-方案.txt", false},  // 非 pptx
	} {
		if got := isReplaceArtifactNameAnyStem(tc.name); got != tc.want {
			t.Errorf("isReplaceArtifactNameAnyStem(%q) = %v，want %v", tc.name, got, tc.want)
		}
	}
}

// TestIsReplaceArtifactOutput_PageSeqDigits 产物归属判定（批量剔除/移动拦截）：
// 1 位页序用户文件不再被认作上次 replace 的产物；两位及以上仍命中。
func TestIsReplaceArtifactOutput_PageSeqDigits(t *testing.T) {
	dir := filepath.Join("D:", "库", "方案")
	outputs := []replaceSplitOutput{{sourceRel: "方案.pptx", outputDir: dir}}
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"9-方案.pptx", false}, // 1 位页序：用户文件
		{"方案-9.pptx", false}, // 1 位页序核心形态：用户文件
		{"09-方案.pptx", true}, // 已改名产物
		{"方案-09.pptx", true}, // 核心中间态
	} {
		if got := isReplaceArtifactOutput(filepath.Join(dir, tc.name), outputs); got != tc.want {
			t.Errorf("isReplaceArtifactOutput(%q) = %v，want %v", tc.name, got, tc.want)
		}
	}
}

// TestCleanupSplitArtifactsInDirByPage_OneDigitUserFileKept R1/R2：指纹不可用的
// 旧记录清理时，目录内 `旧名-001.pptx`（产物，删）与 `旧名-1.pptx`（1 位页序
// 用户文件，保留）并存——1 位页序不参与候选解析，用户文件与其索引行零改动。
//
// 变异验证：恢复 parseSplitArtifactName/splitArtifactPageSeq 的任意位数判定时，
// 用户文件被当作第 1 页候选删除、本条断言变红。
func TestCleanupSplitArtifactsInDirByPage_OneDigitUserFileKept(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "一位页序用户文件库", dir)
	store := svc.openStoreForTest(t, lib.ID)
	outDir := filepath.Join(dir, "旧名")
	product := filepath.Join(outDir, "旧名-001.pptx")
	userFile := filepath.Join(outDir, "旧名-1.pptx")
	writeSplitArtifactAt(t, product, "product", "2026-10-30 10:00:00")
	testutil.WriteFile(t, userFile, "user")
	insertIndexRowForTest(t, svc, lib.ID, 9601, "旧名/旧名-001.pptx")
	insertIndexRowForTest(t, svc, lib.ID, 9602, "旧名/旧名-1.pptx")

	record := &assetSplitRecord{
		sourceRelPath: "新名.pptx",
		outputDir:     outDir,
		pageCount:     1,
		target:        model.AssetSplitTargetSource,
		artifactStem:  "旧名",
		splitAt:       "2026-10-30 10:00:00",
	}
	svc.cleanupSplitArtifactsInDirByPage(store, lib, record, outDir, "")

	mustNotExist(t, product)
	mustExist(t, userFile)
	if got, readErr := os.ReadFile(userFile); readErr != nil || string(got) != "user" {
		t.Errorf("1 位页序用户文件内容不得被改动（err=%v content=%q）", readErr, got)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/旧名-001.pptx"); n != 0 {
		t.Errorf("产物索引行应被清理，实际 %d 行", n)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, "旧名/旧名-1.pptx"); n != 1 {
		t.Errorf("用户文件索引行不得被清理，实际 %d 行", n)
	}
}
