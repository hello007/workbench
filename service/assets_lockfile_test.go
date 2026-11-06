package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Office 临时锁文件（`~$` 前缀）内置排除与解析失败原因落库的用例集。
// 锁文件形态：PowerPoint 打开文档时在同目录生成 `~$<原文件名>`，后缀沿用原
// 文档（.pptx 能进后缀白名单），内容为约百字节的 OLE 指针结构。

// TestIsOfficeLockFileName 前缀判定单点（含不应命中的近似形态）。
func TestIsOfficeLockFileName(t *testing.T) {
	cases := map[string]bool{
		"~$客户端架构-v1.1-2026-8-11.pptx": true,
		"~$a.pptx":                    true,
		"~$":                          true, // 极端形态：前缀本身命中（无后缀文件本就不入白名单）
		"客户端架构-v1.1.pptx":             false,
		"$~a.pptx":                    false, // 前缀顺序不符
		"a~$b.pptx":                   false, // 前缀位置不符
		"~演示.pptx":                    false,
	}
	for name, want := range cases {
		if got := isOfficeLockFileName(name); got != want {
			t.Errorf("isOfficeLockFileName(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestAssetScan_IgnoresOfficeLockFile 收集层排除：目录内正常 pptx 与同名锁文件
// 并存时，只有正常文件入库，锁文件不参与 Total 计数也不产生 failed（锁文件内容
// 非 OOXML 包，若被收录会稳定判损坏——解析失败计数长期非零的常见来源）。
func TestAssetScan_IgnoresOfficeLockFile(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "锁文件库", dir)

	buildAssetsPptx(t, dir, "客户端架构.pptx", []assetsSlideSpec{
		{titlePh: "首页", body: []string{"正文"}},
	})
	lockPath := filepath.Join(dir, "~$客户端架构.pptx")
	if err := os.WriteFile(lockPath, []byte("office-lock-pointer"), 0o644); err != nil {
		t.Fatalf("写锁文件: %v", err)
	}

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if result.Total != 1 || result.New != 1 || result.Failed != 0 {
		t.Errorf("扫描结果不符（锁文件应被忽略）：%+v", result)
	}

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("索引条目数 = %d, want 1：%+v", len(files), files)
	}
	if strings.HasPrefix(filepath.Base(files[0].Path), "~$") {
		t.Errorf("锁文件不应入索引，实际条目：%s", files[0].Path)
	}
}

// TestAssetScan_RemovesExistingLockFileRecords 存量收敛：历史版本已入库的锁文件
// 记录在下次扫描时被删除（不标 lost——锁文件不会出现在收集清单里，若留在
// records 中会被「磁盘消失」段误判为丢失记录，用户需手动清理才消失）。
func TestAssetScan_RemovesExistingLockFileRecords(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "存量锁文件库", dir)

	buildAssetsPptx(t, dir, "正常.pptx", []assetsSlideSpec{{titlePh: "页", body: []string{"文"}}})
	lockPath := filepath.Join(dir, "~$正常.pptx")
	if err := os.WriteFile(lockPath, []byte("office-lock-pointer"), 0o644); err != nil {
		t.Fatalf("写锁文件: %v", err)
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}

	// 模拟历史版本行为：把锁文件手工插入索引（status=failed，与旧版解析失败口径一致）
	store := svc.openStoreForTest(t, lib.ID)
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat 锁文件: %v", err)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := store.insertProcessingFile("~$正常.pptx", "~$正常", info.Size(), info.ModTime().UnixNano(), now); err != nil {
		t.Fatalf("插入存量锁文件记录: %v", err)
	}

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("二次扫描: %v", err)
	}
	if result.LockFilesRemoved != 1 {
		t.Errorf("LockFilesRemoved = %d, want 1：%+v", result.LockFilesRemoved, result)
	}
	if result.Lost != 0 {
		t.Errorf("锁文件记录不应被标为丢失，Lost = %d", result.Lost)
	}

	rec, exists, err := store.fileRecordByPath("~$正常.pptx")
	if err != nil {
		t.Fatalf("查记录: %v", err)
	}
	if exists {
		t.Errorf("锁文件索引行应已删除，实际 status=%s", rec.status)
	}
	// 磁盘锁文件保持原样（删除的是索引行，不动用户磁盘内容）
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("磁盘锁文件应保留，实际: %v", err)
	}
}

// TestAssetScan_RecordsParseError 解析失败原因落库：非 OOXML 的 .pptx 被判 failed
// 且 files.parse_error 写入可读根因（不再只有日志可查）。
func TestAssetScan_RecordsParseError(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "失败原因库", dir)

	if err := os.WriteFile(filepath.Join(dir, "坏文件.pptx"), []byte("not a zip package"), 0o644); err != nil {
		t.Fatalf("写坏文件: %v", err)
	}
	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("Failed = %d, want 1：%+v", result.Failed, result)
	}

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("条目数 = %d, want 1", len(files))
	}
	got := files[0]
	if got.Status != "failed" {
		t.Errorf("状态 = %q, want failed", got.Status)
	}
	if strings.TrimSpace(got.ParseError) == "" {
		t.Fatal("parse_error 为空：失败原因未落库")
	}
	if !strings.Contains(got.ParseError, "损坏") {
		t.Errorf("失败原因应含分类语义「损坏」，实际：%q", got.ParseError)
	}
}

// TestAssetScan_ClearsParseErrorAfterSuccessfulReparse 成功重解析必须清空失败原因：
// 文件由「坏包」替换为有效 pptx 后重扫描，状态回 ok 且 parse_error 归零（残留会
// 让详情页出现「状态=正常 但 原因=损坏」的自相矛盾）。
func TestAssetScan_ClearsParseErrorAfterSuccessfulReparse(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "失败转成功库", dir)

	target := filepath.Join(dir, "演变.pptx")
	if err := os.WriteFile(target, []byte("not a zip package"), 0o644); err != nil {
		t.Fatalf("写坏文件: %v", err)
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 || files[0].ParseError == "" {
		t.Fatalf("前置失败态未建立：%+v", files)
	}

	// 覆盖为有效演示文稿（尺寸变化即触发重解析，无需依赖 mtime 精度）
	buildAssetsPptx(t, dir, "演变.pptx", []assetsSlideSpec{{titlePh: "恢复", body: []string{"正文"}}})

	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("二次扫描: %v", err)
	}
	if result.Updated != 1 || result.Failed != 0 {
		t.Fatalf("重解析结果不符：%+v", result)
	}
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("条目数 = %d, want 1", len(files))
	}
	if files[0].Status != "ok" {
		t.Errorf("状态 = %q, want ok", files[0].Status)
	}
	if files[0].ParseError != "" {
		t.Errorf("成功解析后 parse_error 应清空，实际：%q", files[0].ParseError)
	}
}

// TestParseErrorText 入库文案截断按字符而非字节（防 UTF-8 多字节字符被切半分
// 产生乱码），nil 安全。
func TestParseErrorText(t *testing.T) {
	if got := parseErrorText(nil); got != "" {
		t.Errorf("nil 应返回空串，实际 %q", got)
	}
	short := "文件已损坏（不是有效的演示文稿包）"
	if got := parseErrorText(errors.New(short)); got != short {
		t.Errorf("短文案应原样返回，实际 %q", got)
	}
	long := strings.Repeat("损", 600)
	got := parseErrorText(errors.New(long))
	runes := []rune(got)
	if len(runes) != 501 || runes[500] != '…' {
		t.Errorf("超长文案应按 500 字符截断并追加省略号，实际长度 %d", len(runes))
	}
	if strings.ContainsRune(got, '�') {
		t.Error("截断后不应出现 UTF-8 替换字符（说明按字节切断）")
	}
}
