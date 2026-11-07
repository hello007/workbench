package service

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// blameTwoCommitRepo 构造两次提交的测试仓库：a.txt 首建 2 行 → 追加第 3 行。
// 返回仓库根与文件绝对路径。
func blameTwoCommitRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "第一行\n第二行\n")
	testutil.RunGit(t, dir, "add", "a.txt")
	testutil.RunGit(t, dir, "commit", "-m", "首建 a.txt")
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "第一行\n第二行\n第三行\n")
	testutil.RunGit(t, dir, "add", "a.txt")
	testutil.RunGit(t, dir, "commit", "-m", "追加第三行")
	return dir, filepath.Join(dir, "a.txt")
}

// TestGetFileBlame_Basic 两次提交的行归属：Lines 与文件行号 1:1、每行映射到正确
// 提交、Commits 去重且中文消息往返无乱码。
func TestGetFileBlame_Basic(t *testing.T) {
	_, file := blameTwoCommitRepo(t)
	svc := NewGitService()

	res, err := svc.GetFileBlame(file)
	if err != nil {
		t.Fatalf("GetFileBlame: %v", err)
	}
	if len(res.Lines) != 3 {
		t.Fatalf("Lines 应与文件行数一致（3 行）: got %d", len(res.Lines))
	}
	if len(res.Commits) != 2 {
		t.Fatalf("两次提交应去重为 2 条: got %d", len(res.Commits))
	}
	// 第 1-2 行归属首建提交，第 3 行归属追加提交
	firstIdx, lastIdx := res.Lines[0], res.Lines[2]
	if firstIdx != res.Lines[1] {
		t.Errorf("前两行应归属同一提交: %d vs %d", firstIdx, res.Lines[1])
	}
	if firstIdx == lastIdx {
		t.Errorf("第 3 行应归属不同提交: %d", lastIdx)
	}
	first, last := res.Commits[firstIdx], res.Commits[lastIdx]
	if !strings.Contains(first.Message, "首建") {
		t.Errorf("首建提交消息往返: got %q", first.Message)
	}
	if !strings.Contains(last.Message, "追加") {
		t.Errorf("追加提交消息往返: got %q", last.Message)
	}
	if first.SHA == "" || len(first.ShortSHA) != 8 || first.ShortSHA != first.SHA[:8] {
		t.Errorf("ShortSHA 应为 8 位: got %q", first.ShortSHA)
	}
	if first.Timestamp <= 0 || first.DateTime == "" {
		t.Errorf("时间字段应填充: ts=%d dt=%q", first.Timestamp, first.DateTime)
	}
	if first.Email == "" {
		t.Errorf("作者邮箱应填充: got %q", first.Email)
	}
}

// TestGetFileBlame_UncommittedLines 已跟踪文件工作树修改：未提交行为零 SHA、
// 已提交行仍归属原提交（工作树 blame 行号对齐预览的关键语义）。
func TestGetFileBlame_UncommittedLines(t *testing.T) {
	_, file := blameTwoCommitRepo(t)
	testutil.WriteFile(t, file, "第一行\n第二行修改\n第三行\n")
	svc := NewGitService()

	res, err := svc.GetFileBlame(file)
	if err != nil {
		t.Fatalf("GetFileBlame: %v", err)
	}
	if len(res.Lines) != 3 {
		t.Fatalf("Lines 应为 3: got %d", len(res.Lines))
	}
	zeroIdx := -1
	for i, c := range res.Commits {
		if strings.Trim(c.SHA, "0") == "" {
			zeroIdx = i
		}
	}
	if zeroIdx < 0 {
		t.Fatalf("应存在零 SHA（未提交）提交条目: %+v", res.Commits)
	}
	if res.Lines[1] != zeroIdx {
		t.Errorf("修改行（第 2 行）应归属未提交: got %d want %d", res.Lines[1], zeroIdx)
	}
	if res.Lines[0] == zeroIdx || res.Lines[2] == zeroIdx {
		t.Errorf("未修改行不应归属未提交: %v", res.Lines)
	}
}

// TestGetFileBlame_StagedNewFile 已 add 未 commit 的新文件：全行零 SHA 正常返回
// （HEAD 须存在——unborn 空仓 blame 直接 fatal，走错误矩阵）。
func TestGetFileBlame_StagedNewFile(t *testing.T) {
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "base.txt"), "基线\n")
	testutil.RunGit(t, dir, "add", "base.txt")
	testutil.RunGit(t, dir, "commit", "-m", "基线提交保证 HEAD 存在")
	name := "草稿.md"
	testutil.WriteFile(t, filepath.Join(dir, name), "草稿第一行\n草稿第二行\n")
	testutil.RunGit(t, dir, "add", name)
	svc := NewGitService()

	res, err := svc.GetFileBlame(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("已 stage 新文件应正常返回: %v", err)
	}
	if len(res.Lines) != 2 {
		t.Fatalf("Lines 应为 2: got %d", len(res.Lines))
	}
	for _, idx := range res.Lines {
		if strings.Trim(res.Commits[idx].SHA, "0") != "" {
			t.Errorf("全部行应归属零 SHA（未提交）: got %q", res.Commits[idx].SHA)
		}
	}
}

// TestGetFileBlame_UntrackedFile 未跟踪（未 add）文件：git blame 报错走错误矩阵
// （no such path in HEAD，不降级不静默）。
func TestGetFileBlame_UntrackedFile(t *testing.T) {
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "未跟踪.txt"), "内容\n")
	svc := NewGitService()

	if _, err := svc.GetFileBlame(filepath.Join(dir, "未跟踪.txt")); err == nil {
		t.Error("未跟踪文件应报错")
	}
}

// TestGetFileBlame_ChineseAuthor 中文作者名与邮箱经 utf8Prefix 往返无乱码。
func TestGetFileBlame_ChineseAuthor(t *testing.T) {
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "行一\n")
	testutil.RunGit(t, dir, "add", "a.txt")
	testutil.RunGit(t, dir, "-c", "user.name=张三", "-c", "user.email=zhangsan@example.com",
		"commit", "-m", "中文作者提交")
	svc := NewGitService()

	res, err := svc.GetFileBlame(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("GetFileBlame: %v", err)
	}
	if len(res.Commits) != 1 {
		t.Fatalf("应 1 条提交: got %d", len(res.Commits))
	}
	if res.Commits[0].Author != "张三" {
		t.Errorf("中文作者应原样往返: got %q", res.Commits[0].Author)
	}
	if res.Commits[0].Email != "zhangsan@example.com" {
		t.Errorf("邮箱应去尖括号: got %q", res.Commits[0].Email)
	}
}

// TestGetFileBlame_GlobCharsInName 元字符文件名按裸路径字面精确命中（blame 路径
// 参数非 pathspec 机制，禁加 :(literal) 前缀——与本仓 log 域的 pathspec 魔术相反）。
func TestGetFileBlame_GlobCharsInName(t *testing.T) {
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	name := "笔记[1].md"
	testutil.WriteFile(t, filepath.Join(dir, name), "内容行\n")
	testutil.RunGit(t, dir, "add", name)
	testutil.RunGit(t, dir, "commit", "-m", "创建带元字符文件")
	svc := NewGitService()

	res, err := svc.GetFileBlame(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("裸路径字面匹配应命中: %v", err)
	}
	if len(res.Lines) != 1 || len(res.Commits) != 1 {
		t.Fatalf("应 1 行 1 提交: lines=%d commits=%d", len(res.Lines), len(res.Commits))
	}
}

// TestGetFileBlame_TooLarge 超行数上限返回 E_BLAME_FILE_TOO_LARGE（warning 降级
// 非真失败），且不跑 blame（错误码而非 git 失败包装）。
func TestGetFileBlame_TooLarge(t *testing.T) {
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "big.txt"), strings.Repeat("行\n", blameMaxLines+1))
	svc := NewGitService()

	_, err := svc.GetFileBlame(filepath.Join(dir, "big.txt"))
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeBlameFileTooLarge {
		t.Fatalf("应返回 E_BLAME_FILE_TOO_LARGE: got %v", err)
	}
}

// TestGetFileBlame_ErrorMatrix 错误矩阵：空路径 / 仓库外路径 / 非仓库目录。
func TestGetFileBlame_ErrorMatrix(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.GetFileBlame(""); err == nil {
		t.Error("空路径应报错")
	}

	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	if _, err := svc.GetFileBlame(filepath.Dir(dir) + string(filepath.Separator) + "outside.txt"); err == nil {
		t.Error("仓库外路径应报错")
	}

	plain := t.TempDir()
	testutil.WriteFile(t, filepath.Join(plain, "plain.txt"), "内容\n")
	if _, err := svc.GetFileBlame(filepath.Join(plain, "plain.txt")); err == nil {
		t.Error("非仓库目录应报错")
	}
}

// TestParseBlameOutput_Blocks 手写 porcelain 输出解析矩阵：连续块 count 字段、
// 详情仅首行出现、\t 内容跳过、boundary/previous/filename/committer* 静默跳过、
// 双时区墙钟换算（+0800/+0530）。
func TestParseBlameOutput_Blocks(t *testing.T) {
	sha1 := strings.Repeat("a", 40)
	sha2 := strings.Repeat("b", 40)
	out := strings.Join([]string{
		sha1 + " 1 1 2",
		"author 张三",
		"author-mail <z@x.com>",
		"author-time 1700000000",
		"author-tz +0800",
		"committer 张三",
		"committer-mail <z@x.com>",
		"committer-time 1700000000",
		"committer-tz +0800",
		"summary 首次提交",
		"filename a.txt",
		"\t" + "第一行",
		sha1 + " 2 2",
		"\t" + "第二行",
		sha2 + " 3 3 1",
		"author 李四",
		"author-mail <l@x.com>",
		"author-time 1700000100",
		"author-tz +0530",
		"summary 添加第三行",
		"boundary",
		"previous " + strings.Repeat("c", 40) + " old.txt",
		"filename a.txt",
		"\t" + "第三行",
		"",
	}, "\n")

	res, err := parseBlameOutput(out)
	if err != nil {
		t.Fatalf("parseBlameOutput: %v", err)
	}
	if len(res.Commits) != 2 {
		t.Fatalf("应去重 2 条提交: got %d", len(res.Commits))
	}
	if len(res.Lines) != 3 || res.Lines[0] != 0 || res.Lines[1] != 0 || res.Lines[2] != 1 {
		t.Fatalf("Lines 映射错误: %v", res.Lines)
	}
	c1, c2 := res.Commits[0], res.Commits[1]
	if c1.Author != "张三" || c1.Email != "z@x.com" || c1.Message != "首次提交" {
		t.Errorf("提交 1 元数据错误: %+v", c1)
	}
	// 1700000000 UTC = 2023-11-14 22:13:20；+0800 墙钟 = 2023-11-15 06:13:20
	if c1.DateTime != "2023-11-15 06:13:20" {
		t.Errorf("+0800 墙钟换算: got %q want 2023-11-15 06:13:20", c1.DateTime)
	}
	// +0530 墙钟 = UTC + 5h30m = 2023-11-15 03:45:00（1700000100 = 22:15:00 UTC）
	if c2.DateTime != "2023-11-15 03:45:00" {
		t.Errorf("+0530 墙钟换算: got %q want 2023-11-15 03:45:00", c2.DateTime)
	}
	if c1.Timestamp != 1700000000 || c2.Timestamp != 1700000100 {
		t.Errorf("时间戳错误: %d %d", c1.Timestamp, c2.Timestamp)
	}
}

// TestParseBlameOutput_ZeroSHAUncommitted 零 SHA 未提交块正常解析（author 为
// Not Committed Yet、无 boundary），SHA 原样保留交前端判定。
func TestParseBlameOutput_ZeroSHAUncommitted(t *testing.T) {
	zero := strings.Repeat("0", 40)
	out := strings.Join([]string{
		zero + " 1 1 1",
		"author Not Committed Yet",
		"author-mail <not.committed.yet>",
		"author-time 1794051932",
		"author-tz +0800",
		"summary Version of a.txt from a.txt",
		"previous " + strings.Repeat("c", 40) + " a.txt",
		"filename a.txt",
		"\t未提交内容",
		"",
	}, "\n")

	res, err := parseBlameOutput(out)
	if err != nil {
		t.Fatalf("parseBlameOutput: %v", err)
	}
	if len(res.Commits) != 1 || res.Commits[0].SHA != zero {
		t.Fatalf("零 SHA 应原样保留: %+v", res.Commits)
	}
}

// TestParseBlameOutput_DetailFieldDirty 单条详情字段脏 fail-soft：author-time
// 非法 → 时间字段留零值，条目与其他字段保留。
func TestParseBlameOutput_DetailFieldDirty(t *testing.T) {
	sha := strings.Repeat("d", 40)
	out := strings.Join([]string{
		sha + " 1 1 1",
		"author 作者",
		"author-time not-a-number",
		"author-tz +0800",
		"summary 脏时间提交",
		"filename a.txt",
		"\t内容",
		"",
	}, "\n")

	res, err := parseBlameOutput(out)
	if err != nil {
		t.Fatalf("脏时间字段不应整体报错: %v", err)
	}
	if len(res.Commits) != 1 {
		t.Fatalf("应保留 1 条: got %d", len(res.Commits))
	}
	if res.Commits[0].Timestamp != 0 || res.Commits[0].DateTime != "" {
		t.Errorf("脏时间字段应留零值: %+v", res.Commits[0])
	}
	if res.Commits[0].Author != "作者" || res.Commits[0].Message != "脏时间提交" {
		t.Errorf("其余字段应保留: %+v", res.Commits[0])
	}
}

// TestParseBlameOutput_BadHeaderLine sha 行头行号非法 → 整体报错（porcelain 为
// 机械格式，输出契约破坏时行号对齐不可信，fail-fast）。
func TestParseBlameOutput_BadHeaderLine(t *testing.T) {
	sha := strings.Repeat("e", 40)
	out := sha + " 1 zero 1\n\t内容\n"
	if _, err := parseBlameOutput(out); err == nil {
		t.Error("行头行号非法应报错")
	}
}

// TestParseBlameOutput_MissingLines 行记录缺失（行号跳号）→ 解析不完整报错，
// 禁止静默错位。
func TestParseBlameOutput_MissingLines(t *testing.T) {
	sha := strings.Repeat("f", 40)
	out := sha + " 1 1 1\n\t第一行\n" + sha + " 3 3 1\n\t第三行\n"
	if _, err := parseBlameOutput(out); err == nil {
		t.Error("行号跳号应报错")
	}
}

// TestParseBlameOutput_Empty 空输出（空文件）返回空结果非错误。
func TestParseBlameOutput_Empty(t *testing.T) {
	res, err := parseBlameOutput("")
	if err != nil {
		t.Fatalf("空输出不应报错: %v", err)
	}
	if res == nil || len(res.Commits) != 0 || len(res.Lines) != 0 {
		t.Errorf("应返回空结果: %+v", res)
	}
}

// TestParseBlameOutput_CRLF CRLF 行尾兼容：\t 内容行的 \r 剥离不影响解析。
func TestParseBlameOutput_CRLF(t *testing.T) {
	sha := strings.Repeat("9", 40)
	out := sha + " 1 1 1\r\nauthor 作者\r\nauthor-time 1700000000\r\nauthor-tz +0800\r\nsummary 标题\r\nfilename a.txt\r\n\t内容\r\n"
	res, err := parseBlameOutput(out)
	if err != nil {
		t.Fatalf("CRLF 输出不应报错: %v", err)
	}
	if len(res.Lines) != 1 || res.Commits[0].Author != "作者" {
		t.Errorf("CRLF 解析错误: %+v", res)
	}
}

// TestBlameDateTime 时区换算矩阵：正偏移 / 负偏移带分钟 / 非法 tz 回退 UTC。
func TestBlameDateTime(t *testing.T) {
	ts := time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC).Unix()
	cases := []struct {
		tz   string
		want string
	}{
		{"+0800", "2023-11-15 06:13:20"},
		{"-0530", "2023-11-14 16:43:20"},
		{"+0000", "2023-11-14 22:13:20"},
		{"bad", "2023-11-14 22:13:20"}, // 非法回退 UTC
		{"", "2023-11-14 22:13:20"},
	}
	for _, tc := range cases {
		if got := blameDateTime(ts, tc.tz); got != tc.want {
			t.Errorf("tz=%q: got %q want %q", tc.tz, got, tc.want)
		}
	}
}

// TestCountFileLines 行数口径与 CodeMirror 文档行数一致：空文件 0、末行无换行
// 计一行、尾换行不多计、超长单行安全。
func TestCountFileLines(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"空文件", "", 0},
		{"单行无尾换行", "abc", 1},
		{"单行尾换行", "abc\n", 1},
		{"多行", "a\nb\nc\n", 3},
		{"多行末行无换行", "a\nb\nc", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "f.txt")
			testutil.WriteFile(t, path, tc.content)
			got, err := countFileLines(path)
			if err != nil {
				t.Fatalf("countFileLines: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}
