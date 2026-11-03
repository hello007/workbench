package service

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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

// TestDeleteIndexRowsForPaths_CrossVolumeSilent 跨卷分支守护：库根与候选产物位于
// 不同卷时 filepath.Rel 直接出错（区别于同卷的 `..` 前缀），同属库外静默跳过——
// 零 Warn 且有 Debug 留痕。Windows 用第二可用盘符构造（无第二盘则 skip）；非
// Windows 用「相对库根 + 绝对路径」触发 Rel 错误，覆盖同一分支。
func TestDeleteIndexRowsForPaths_CrossVolumeSilent(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "跨卷清理静默库", root)
	store := svc.openStoreForTest(t, lib.ID)

	// 干扰行：与跨卷文件同基名的库内索引行，库外路径若退化为基名匹配会误删。
	decoyRel := "跨卷-001.pptx"
	insertIndexRowForTest(t, svc, lib.ID, 9711, decoyRel)

	var outsideAbs string
	if runtime.GOOS == "windows" {
		outsideAbs = crossVolumePathForTest(t, root, decoyRel)
	} else {
		lib.RootPath = filepath.Join("relative", "root")
		outsideAbs = filepath.Join(root, "derived", decoyRel)
	}

	var logBuf bytes.Buffer
	prevLogger := Logger()
	SetLogger(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { SetLogger(prevLogger) })

	svc.deleteIndexRowsForPaths(store, lib, []string{outsideAbs})

	if n := countIndexRowsByPath(t, svc, lib.ID, decoyRel); n != 1 {
		t.Errorf("跨卷路径不得删除库内同基名索引行，实际剩余 %d 行（应为 1）", n)
	}
	got := logBuf.String()
	if strings.Contains(got, "level=WARN") {
		t.Errorf("跨卷库外跳过应零告警，实际日志：%s", got)
	}
	if !strings.Contains(got, "旧产物位于库根之外") {
		t.Errorf("跨卷分支应有 Debug 留痕，实际日志：%s", got)
	}
}

// crossVolumePathForTest 返回位于另一可用盘符的绝对路径（Windows 专用，用于
// 触发 filepath.Rel 跨卷错误）；无第二可用盘符时 skip 并说明原因。
func crossVolumePathForTest(t *testing.T, root, rel string) string {
	t.Helper()
	cur := filepath.VolumeName(root)
	for c := 'A'; c <= 'Z'; c++ {
		drive := string(c) + ":"
		if strings.EqualFold(drive, cur) {
			continue
		}
		if _, err := os.Stat(drive + string(filepath.Separator)); err != nil {
			continue
		}
		return filepath.Join(drive+string(filepath.Separator), "workbench-cross-volume", rel)
	}
	t.Skip("本机无第二可用盘符，跳过 Windows 跨卷分支")
	return ""
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
