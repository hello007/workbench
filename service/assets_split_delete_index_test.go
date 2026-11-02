package service

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"workbench/util/testutil"
)

// TestDeleteIndexRowsForPaths_OutsideRootSilent 旧落点位于库根之外（derived 目录
// 形态）时静默跳过：库内路径照常删索引行；库外路径零删行且不产生「计算旧产物
// 相对路径失败」Warn——同卷时 Rel 返回 `..` 前缀、跨卷时 Rel 直接出错，两者均
// 属库外而非异常（11-02-derived-cross-volume-warn-noise）。
func TestDeleteIndexRowsForPaths_OutsideRootSilent(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "库外清理静默库", dir)
	store := svc.openStoreForTest(t, lib.ID)

	// 库内旧产物：落盘 + 索引行，应被正常清理。
	inRel := "产物/旧名-001.pptx"
	inAbs := filepath.Join(dir, filepath.FromSlash(inRel))
	testutil.WriteFile(t, inAbs, "old")
	insertIndexRowForTest(t, svc, lib.ID, 9701, inRel)

	// 干扰行：与库外文件同基名的库根文件。库外路径若被误判为库内并退化为
	// 基名匹配，会误删该行——用它锁定「库外零删行」验收点。
	decoyRel := "旧名-001.pptx"
	insertIndexRowForTest(t, svc, lib.ID, 9702, decoyRel)

	// 库外旧产物（derived 形态）：位于另一个临时目录，与库根不同父目录，
	// Rel 必产生 `..` 前缀（跨卷环境则直接出错），两条路径均不得告警。
	outsideDir := t.TempDir()
	outAbs := filepath.Join(outsideDir, "derived", decoyRel)
	testutil.WriteFile(t, outAbs, "derived")

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { SetLogger(prevLogger) })

	svc.deleteIndexRowsForPaths(store, lib, []string{inAbs, outAbs})

	if n := countIndexRowsByPath(t, svc, lib.ID, inRel); n != 0 {
		t.Errorf("库内旧产物索引行应被删除，实际 %d 行", n)
	}
	if n := countIndexRowsByPath(t, svc, lib.ID, decoyRel); n != 1 {
		t.Errorf("库外路径不得删除库内同基名索引行，实际剩余 %d 行（应为 1）", n)
	}
	if got := logBuf.String(); strings.Contains(got, "计算旧产物相对路径失败") {
		t.Errorf("库外路径不应告警「计算旧产物相对路径失败」，实际日志：%s", got)
	}
	if got := logBuf.String(); strings.Contains(got, "level=WARN") {
		t.Errorf("库外旧落点跳过应零告警，实际日志：%s", got)
	}
}

// TestRelPathWithinRoot 库内判定单点：库内路径返回正斜杠相对路径；`..` 逃逸与
// Rel 不可比对（相对根 + 绝对路径）均返回 false，不返回错误。
func TestRelPathWithinRoot(t *testing.T) {
	root := t.TempDir()
	absIn := filepath.Join(root, "x.pptx")
	cases := []struct {
		name   string
		root   string
		abs    string
		want   string
		wantOK bool
	}{
		{"库内文件", root, filepath.Join(root, "方案", "旧名-001.pptx"), "方案/旧名-001.pptx", true},
		{"库根本身", root, root, ".", true},
		{"父目录逃逸", root, filepath.Join(root, "..", "outside.pptx"), "", false},
		{"兄弟目录", root, filepath.Join(filepath.Dir(root), "sibling", "x.pptx"), "", false},
		{"Rel 不可比对", "relative/root", absIn, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel, ok := relPathWithinRoot(tc.root, tc.abs)
			if ok != tc.wantOK {
				t.Fatalf("库内判定应为 %v，实际 %v（rel=%q）", tc.wantOK, ok, rel)
			}
			if rel != tc.want {
				t.Errorf("相对路径应为 %q，实际 %q", tc.want, rel)
			}
		})
	}
}
