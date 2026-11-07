package service

import (
	"path/filepath"
	"strings"
	"testing"

	"workbench/util/testutil"
)

// fileHistoryRenameRepo 构造带重命名链的文件历史测试仓库：
// file.txt 两次内容提交 → 重命名为 renamed.txt → 新路径下再提交一次。
// 返回仓库根与 renamed.txt 绝对路径。
func fileHistoryRenameRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "file.txt"), "v1\n")
	testutil.RunGit(t, dir, "add", "file.txt")
	testutil.RunGit(t, dir, "commit", "-m", "首次创建 file.txt")
	testutil.WriteFile(t, filepath.Join(dir, "file.txt"), "v2\n")
	testutil.RunGit(t, dir, "add", "file.txt")
	testutil.RunGit(t, dir, "commit", "-m", "更新 file.txt 内容")
	testutil.RunGit(t, dir, "mv", "file.txt", "renamed.txt")
	testutil.RunGit(t, dir, "commit", "-m", "重命名为 renamed.txt")
	testutil.WriteFile(t, filepath.Join(dir, "renamed.txt"), "v3\n")
	testutil.RunGit(t, dir, "add", "renamed.txt")
	testutil.RunGit(t, dir, "commit", "-m", "renamed.txt 后续修改")
	return dir, filepath.Join(dir, "renamed.txt")
}

// TestGetFileCommitHistory_FollowRename 跨重命名追溯：--follow 下以重命名后路径
// 查询应拿到重命名前的提交（历史不断链），rename 提交带 RenameFrom/RenameTo 检出，
// 各段 PathAtCommit 呈现该提交时刻的实际路径。
func TestGetFileCommitHistory_FollowRename(t *testing.T) {
	_, file := fileHistoryRenameRepo(t)
	svc := NewGitService()

	commits, err := svc.GetFileCommitHistory(file, 20, 0)
	if err != nil {
		t.Fatalf("GetFileCommitHistory: %v", err)
	}
	if len(commits) != 4 {
		t.Fatalf("应追溯 4 条提交（含重命名前 2 条）: got %d", len(commits))
	}

	// 倒序：最新在前
	if commits[0].Message == "" || !strings.Contains(commits[0].Message, "后续修改") {
		t.Errorf("首条应为最新提交: got %q", commits[0].Message)
	}

	// rename 提交检出（第 2 条）：R 行 old→new
	rename := commits[1]
	if rename.RenameFrom != "file.txt" || rename.RenameTo != "renamed.txt" {
		t.Errorf("rename 检出: got %q→%q, want file.txt→renamed.txt", rename.RenameFrom, rename.RenameTo)
	}
	if rename.PathAtCommit != "renamed.txt" {
		t.Errorf("rename 提交 PathAtCommit 应为新路径: got %q", rename.PathAtCommit)
	}

	// rename 前的旧段提交：PathAtCommit 呈现旧路径（--follow 追溯生效的铁证）
	if commits[2].PathAtCommit != "file.txt" {
		t.Errorf("旧段提交 PathAtCommit 应为 file.txt: got %q", commits[2].PathAtCommit)
	}
	if commits[3].PathAtCommit != "file.txt" {
		t.Errorf("首建提交 PathAtCommit 应为 file.txt: got %q", commits[3].PathAtCommit)
	}
	// 旧段提交无 rename 检出
	if commits[2].RenameFrom != "" || commits[3].RenameFrom != "" {
		t.Errorf("普通提交不应带 rename 检出: got %+v", commits[2:4])
	}
}

// TestGetFileCommitHistory_ChineseFileName 中文文件名 + 中文提交消息往返无乱码
// （UTF-8 前缀 + quotePath=off 生效）。
func TestGetFileCommitHistory_ChineseFileName(t *testing.T) {
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "设计文档.md"), "# 初稿\n")
	testutil.RunGit(t, dir, "add", "设计文档.md")
	testutil.RunGit(t, dir, "commit", "-m", "创建设计文档")
	testutil.RunGit(t, dir, "mv", "设计文档.md", "评审稿.md")
	testutil.RunGit(t, dir, "commit", "-m", "更名为评审稿")

	svc := NewGitService()
	commits, err := svc.GetFileCommitHistory(filepath.Join(dir, "评审稿.md"), 20, 0)
	if err != nil {
		t.Fatalf("GetFileCommitHistory: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("应追溯 2 条: got %d", len(commits))
	}
	if commits[0].Message != "更名为评审稿\n" {
		t.Errorf("中文提交消息应原样往返: got %q", commits[0].Message)
	}
	if commits[0].PathAtCommit != "评审稿.md" {
		t.Errorf("PathAtCommit: got %q, want 评审稿.md", commits[0].PathAtCommit)
	}
	if commits[0].RenameFrom != "设计文档.md" || commits[0].RenameTo != "评审稿.md" {
		t.Errorf("中文 rename 检出: got %q→%q", commits[0].RenameFrom, commits[0].RenameTo)
	}
}

// TestGetFileCommitHistory_GlobCharsInName 文件名含 glob 元字符（[:literal) 魔术
// 前缀按字节精确匹配，不被解释为字符组模式。
func TestGetFileCommitHistory_GlobCharsInName(t *testing.T) {
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	name := "笔记[1].md"
	testutil.WriteFile(t, filepath.Join(dir, name), "内容\n")
	testutil.RunGit(t, dir, "add", name)
	testutil.RunGit(t, dir, "commit", "-m", "创建带元字符文件")

	svc := NewGitService()
	commits, err := svc.GetFileCommitHistory(filepath.Join(dir, name), 20, 0)
	if err != nil {
		t.Fatalf("GetFileCommitHistory: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("literal pathspec 应精确命中 1 条: got %d", len(commits))
	}
	if commits[0].PathAtCommit != name {
		t.Errorf("PathAtCommit: got %q, want %q", commits[0].PathAtCommit, name)
	}
}

// TestGetFileCommitHistory_Pagination 分页窗口：limit 截取 + offset 越界返回空切片。
func TestGetFileCommitHistory_Pagination(t *testing.T) {
	_, file := fileHistoryRenameRepo(t)
	svc := NewGitService()

	// limit=2 首页：最新 2 条
	first, err := svc.GetFileCommitHistory(file, 2, 0)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 2 || !strings.Contains(first[0].Message, "后续修改") {
		t.Fatalf("首页应为最新 2 条: got %d, first=%q", len(first), first[0].Message)
	}

	// offset=2 第 2 页：重命名前 2 条
	second, err := svc.GetFileCommitHistory(file, 2, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("第 2 页应 2 条: got %d", len(second))
	}
	// 两页 SHA 无交集（窗口 [offset, offset+limit) 不与前页重叠）
	firstSHAs := make(map[string]bool, len(first))
	for _, c := range first {
		firstSHAs[c.SHA] = true
	}
	for _, c := range second {
		if firstSHAs[c.SHA] {
			t.Errorf("第 2 页与首页重叠: %s 同时出现在两页", c.SHA)
		}
	}
	if first[1].RenameFrom != "file.txt" || first[1].RenameTo != "renamed.txt" {
		t.Errorf("首页末条应为 rename 提交: got %+v", first[1])
	}
	if second[0].RenameFrom != "" || second[1].RenameFrom != "" {
		t.Errorf("第 2 页为 rename 前旧段，不应带 rename 检出: got %+v", second)
	}

	// offset 越界：空切片非 nil
	tail, err := svc.GetFileCommitHistory(file, 20, 4)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if tail == nil || len(tail) != 0 {
		t.Errorf("offset 越界应返回空切片: got %v", tail)
	}
}

// TestGetFileCommitHistory_NonexistentFile 不存在的路径返回空切片不报错
// （git log 对无历史路径空输出）。
func TestGetFileCommitHistory_NonexistentFile(t *testing.T) {
	dir, _ := fileHistoryRenameRepo(t)
	svc := NewGitService()

	commits, err := svc.GetFileCommitHistory(filepath.Join(dir, "不存在.txt"), 20, 0)
	if err != nil {
		t.Fatalf("不存在路径不应报错: %v", err)
	}
	if commits == nil || len(commits) != 0 {
		t.Errorf("应返回空切片: got %v", commits)
	}
}

// TestGetFileCommitHistory_EmptyPath 空路径与仓库外路径报错。
func TestGetFileCommitHistory_EmptyPath(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.GetFileCommitHistory("", 20, 0); err == nil {
		t.Error("空路径应报错")
	}

	dir, _ := fileHistoryRenameRepo(t)
	// 仓库外路径（父目录文件）应拒绝
	if _, err := svc.GetFileCommitHistory(filepath.Dir(dir)+string(filepath.Separator)+"outside.txt", 20, 0); err == nil {
		t.Error("仓库外路径应报错")
	}
}

// TestParseFileCommitHistory_MessageWithSeparator 消息含 \x01 与 "\x01\n\n" 序列时
// 反向定位锚点仍正确（消息不截断、状态段不受污染）。
func TestParseFileCommitHistory_MessageWithSeparator(t *testing.T) {
	sha := strings.Repeat("a", 40)
	rawMessage := "fix: 修复登录\n\n正文含分隔符 \x01 与锚点形序列 \x01\n\n 的行\n"
	out := "\x00" + sha + "\x01张三\x01z@x.com\x011700000000\x012023-11-14 22:13:20 +0800\x01" + rawMessage + "\x01\n\nM\tsrc/a.go\n"
	commits := parseFileCommitHistory(out)
	if len(commits) != 1 {
		t.Fatalf("应解析 1 条: got %d", len(commits))
	}
	c := commits[0]
	if c.Message != rawMessage {
		t.Errorf("含分隔符消息应原样解析: got %q want %q", c.Message, rawMessage)
	}
	if c.PathAtCommit != "src/a.go" || c.RenameFrom != "" {
		t.Errorf("状态段不应被消息内锚点污染: got path=%q rename=%q", c.PathAtCommit, c.RenameFrom)
	}
}

// TestParseFileCommitHistory_MalformedRecordSkipped 单条脏记录跳过不阻断整体
// （列表展示场景 fail-soft，对齐 parseNameStatus 畸形行跳过先例）。
func TestParseFileCommitHistory_MalformedRecordSkipped(t *testing.T) {
	goodSHA := strings.Repeat("b", 40)
	out := "\x00not-a-sha\x01short\x01" + // 头部不完整（脏记录）
		"\x00" + goodSHA + "\x01作者\x01e@x.com\x011700000001\x012023-11-14 22:13:21 +0800\x01正常提交\n\x01\n\nA\tnew.txt\n"
	commits := parseFileCommitHistory(out)
	if len(commits) != 1 {
		t.Fatalf("脏记录应跳过、好记录应保留: got %d", len(commits))
	}
	if commits[0].SHA != goodSHA {
		t.Errorf("保留的应为好记录: got %q", commits[0].SHA)
	}
}

// TestParseFileStatusLines 状态行解析矩阵：普通单路径 / R/C 双路径 / 空段 / 多行取首。
func TestParseFileStatusLines(t *testing.T) {
	cases := []struct {
		name             string
		raw              string
		path, from, to   string
	}{
		{"普通修改", "M\tsrc/a.go\n", "src/a.go", "", ""},
		{"新增", "A\tnew.txt\n", "new.txt", "", ""},
		{"删除", "D\told.txt\n", "old.txt", "", ""},
		{"重命名", "R100\told.txt\tnew.txt\n", "new.txt", "old.txt", "new.txt"},
		{"复制", "C80\tsrc.txt\tdst.txt\n", "dst.txt", "src.txt", "dst.txt"},
		{"空段", "\n", "", "", ""},
		{"多行取首", "M\tfirst.txt\nM\tsecond.txt\n", "first.txt", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, from, to := parseFileStatusLines(tc.raw)
			if path != tc.path || from != tc.from || to != tc.to {
				t.Errorf("got (%q,%q,%q) want (%q,%q,%q)", path, from, to, tc.path, tc.from, tc.to)
			}
		})
	}
}
