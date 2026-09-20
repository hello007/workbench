package main

// 提交历史 CLI 采集层测试（解析单测 + 真实 git fixture 行为测试）。
//
// 解析单测：合成 git log 记录块（\x00 记录分隔 / \x01 字段分隔），覆盖消息含换行/
// 中文/0x01、root 提交 100 上限、畸形输入报错等，不依赖 git。
// 行为测试：真实 git fixture 跑通采集链，锚定与原 go-git 采集的语义零漂移
// （Message 字节级含尾部换行、root 文件列表 100 上限、merge 相对第一父提交、
// committer date 序、非本地时区 DateTime）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/service"
	"workbench/util"
	"workbench/util/testutil"
)

// mkChunk 构造单条 git log 记录块（模拟 CLI 输出片段，含格式串尾部 0x01 与
// 记录间换行结构）。parents 为空表示 root 提交。
func mkChunk(sha, parents, author, email, ts, ai, message string, files []string) string {
	filePart := ""
	if len(files) > 0 {
		filePart = "\n" + strings.Join(files, "\n") + "\n"
	}
	return sha + "\x01" + parents + "\x01" + author + "\x01" + email + "\x01" + ts + "\x01" + ai + "\x01" + message + "\x01\n" + filePart
}

// shaHex 生成确定性 40 位小写 hex 字符串（模拟 git %H/%P 真实输出格式；种子仅用于
// 区分不同 SHA，无密码学意义）。空种子按 "seed" 处理。
func shaHex(seed string) string {
	if seed == "" {
		seed = "seed"
	}
	const digits = "0123456789abcdef"
	out := make([]byte, 40)
	for i := range out {
		out[i] = digits[(seed[i%len(seed)]+byte(i))%16]
	}
	return string(out)
}

// TestParseCommitLogChunk_Basic 常规记录：字段解析、Message 保留尾部换行、DateTime
// 取 %ai 首 19 字节、文件列表解析。
func TestParseCommitLogChunk_Basic(t *testing.T) {
	chunk := mkChunk(shaHex("basic"), shaHex("p1")+" "+shaHex("p2"), "Alice", "a@x.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "feat: add login\n\nbody line\n",
		[]string{"src/a.go", "docs/b.md"})
	c, err := parseCommitLogChunk(chunk)
	if err != nil {
		t.Fatalf("parseCommitLogChunk: %v", err)
	}
	if c == nil {
		t.Fatal("chunk 不应为空")
	}
	if c.SHA != shaHex("basic") {
		t.Errorf("SHA: got %q", c.SHA)
	}
	if c.ShortSHA != shaHex("basic")[:8] {
		t.Errorf("ShortSHA 应取前 8 位: got %q", c.ShortSHA)
	}
	if c.Message != "feat: add login\n\nbody line\n" {
		t.Errorf("Message 应字节级保留（含尾部换行）: got %q", c.Message)
	}
	if c.Author != "Alice" || c.Email != "a@x.com" {
		t.Errorf("Author/Email: got %q/%q", c.Author, c.Email)
	}
	if c.Timestamp != 1700000000 {
		t.Errorf("Timestamp: got %d", c.Timestamp)
	}
	if c.DateTime != "2023-11-14 22:13:20" {
		t.Errorf("DateTime 应取 %%ai 首 19 字节: got %q", c.DateTime)
	}
	if len(c.Files) != 2 || c.Files[0] != "src/a.go" || c.Files[1] != "docs/b.md" {
		t.Errorf("Files: got %v", c.Files)
	}
}

// TestParseCommitLogChunk_MessageWithSpecialBytes 消息含换行/中文/0x01：0x01 出现在
// 消息内时以「最后一个后随 \n\n 的 0x01」为消息终止符（反向定位），解析仍正确。
func TestParseCommitLogChunk_MessageWithSpecialBytes(t *testing.T) {
	rawMessage := "fix: 修复登录\n\n含分隔符 \x01 的正文行\n第二行\n"
	chunk := mkChunk(shaHex("m1"), shaHex("p1"), "张三", "z@x.com",
		"1700000001", "2023-11-14 22:13:21 +0800", rawMessage, []string{"b.go"})
	c, err := parseCommitLogChunk(chunk)
	if err != nil {
		t.Fatalf("parseCommitLogChunk: %v", err)
	}
	if c.Message != rawMessage {
		t.Errorf("含 0x01 消息应原样解析: got %q want %q", c.Message, rawMessage)
	}
	if len(c.Files) != 1 || c.Files[0] != "b.go" {
		t.Errorf("Files 不应被消息内 0x01 干扰: got %v", c.Files)
	}
}

// TestParseCommitLogChunk_RootCommitFileCap root 提交（parents 空）文件列表超
// rootCommitFileLimit 截断至 100，对齐原 getTreeFiles 上限；非 root 不截断。
func TestParseCommitLogChunk_RootCommitFileCap(t *testing.T) {
	files := make([]string, 120)
	for i := range files {
		files[i] = fmt.Sprintf("file%d.txt", i)
	}
	c, err := parseCommitLogChunk(mkChunk(shaHex("root"), "", "t", "t@t.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "root\n", files))
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	if len(c.Files) != rootCommitFileLimit {
		t.Errorf("root 提交文件应截断至 %d: got %d", rootCommitFileLimit, len(c.Files))
	}

	c2, err := parseCommitLogChunk(mkChunk(shaHex("nr"), shaHex("p"), "t", "t@t.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "non-root\n", files))
	if err != nil {
		t.Fatalf("non-root: %v", err)
	}
	if len(c2.Files) != 120 {
		t.Errorf("非 root 提交文件不截断: got %d", len(c2.Files))
	}
}

// TestParseCommitLogChunk_EmptyAndMalformed 空块返 nil 不报错；字段不足 / SHA 或
// parents 非 40 位 hex / 时间戳非法 / 作者日期非法 / 消息终止符缺失报 error。
func TestParseCommitLogChunk_EmptyAndMalformed(t *testing.T) {
	if c, err := parseCommitLogChunk(""); err != nil || c != nil {
		t.Errorf("空块应返 (nil, nil): got (%v, %v)", c, err)
	}
	if c, err := parseCommitLogChunk("\x00\x00"); err != nil || c != nil {
		t.Errorf("纯分隔块应返 (nil, nil): got (%v, %v)", c, err)
	}
	if _, err := parseCommitLogChunk("sha\x01p\x01author"); err == nil {
		t.Error("头部字段不足应报错")
	}
	// SHA 非 40 位 hex（头部强校验快速失败，防字段错位静默解析）
	if _, err := parseCommitLogChunk(mkChunk("shortsha", shaHex("p"), "a", "e",
		"1700000000", "2023-11-14 22:13:20 +0800", "m\n", nil)); err == nil {
		t.Error("SHA 非 40 位 hex 应报错")
	}
	// parents 非 40 位 hex
	if _, err := parseCommitLogChunk(mkChunk(shaHex("pl"), "badparent", "a", "e",
		"1700000000", "2023-11-14 22:13:20 +0800", "m\n", nil)); err == nil {
		t.Error("parents 非 40 位 hex 应报错")
	}
	// 时间戳非法（SHA/parents 合法，定位到 ts 校验分支）
	if _, err := parseCommitLogChunk(mkChunk(shaHex("ts"), shaHex("p"), "a", "e", "notanumber",
		"2023-11-14 22:13:20 +0800", "m\n", nil)); err == nil {
		t.Error("时间戳非法应报错")
	}
	// 作者日期非法
	if _, err := parseCommitLogChunk(mkChunk(shaHex("ai"), shaHex("p"), "a", "e", "1700000000",
		"bad-date", "m\n", nil)); err == nil {
		t.Error("作者日期非法应报错")
	}
	// 消息终止符缺失（头部合法，既无 "\x01\n\n" 锚点也不以 "\x01\n" 结尾）
	noTerm := shaHex("nt") + "\x01" + shaHex("p") + "\x01a\x01e\x01170000000\x012023-11-14 22:13:20 +0800\x01m\n\x01"
	if _, err := parseCommitLogChunk(noTerm); err == nil {
		t.Error("消息终止符缺失应报错")
	}
}

// TestParseCommitLogOutput_MultiRecords 完整输出（首 0x00 + 多记录）解析。
func TestParseCommitLogOutput_MultiRecords(t *testing.T) {
	out := "\x00" + mkChunk(shaHex("s2"), shaHex("s3"), "a", "e@x.com", "200", "1970-01-01 00:03:20 +0000", "second\n", []string{"2.txt"}) +
		"\x00" + mkChunk(shaHex("s3"), "", "a", "e@x.com", "100", "1970-01-01 00:01:40 +0000", "root\n", []string{"1.txt"}) + "\n"
	commits, err := parseCommitLogOutput(out)
	if err != nil {
		t.Fatalf("parseCommitLogOutput: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("应解析 2 条: got %d", len(commits))
	}
	if commits[0].SHA != shaHex("s2") || commits[1].SHA != shaHex("s3") {
		t.Errorf("顺序应保持: got %s, %s", commits[0].SHA, commits[1].SHA)
	}
	if commits[1].Files[0] != "1.txt" {
		t.Errorf("root 文件列表: got %v", commits[1].Files)
	}
}

// TestTruncateOverflow 超限截断：MaxEntries+1 → 前 MaxEntries 条 + overflow=true；
// 未超限原样返回 overflow=false。
func TestTruncateOverflow(t *testing.T) {
	under := make([]model.Commit, 10)
	got, overflow := truncateOverflow(under)
	if overflow || len(got) != 10 {
		t.Errorf("未超限应原样返回: len=%d overflow=%v", len(got), overflow)
	}
	over := make([]model.Commit, service.CommitHistoryMaxEntries+1)
	got2, overflow2 := truncateOverflow(over)
	if !overflow2 {
		t.Error("超限应返 overflow=true")
	}
	if len(got2) != service.CommitHistoryMaxEntries {
		t.Errorf("超限应截断至 %d: got %d", service.CommitHistoryMaxEntries, len(got2))
	}
}

// TestFetchCommitHistoryFromGit_FilterOffsetStreaming CLI 流式翻页路径：内存过滤 +
// offset 翻页 + 凑够 limit 提前终止，结果与缓存路径语义一致。
func TestFetchCommitHistoryFromGit_FilterOffsetStreaming(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "r")
	makeCommits(t, repoPath, []commitSpec{
		{message: "feat: a", file: "a.go"},
		{message: "fix: b", file: "b.go"},
		{message: "feat: c", file: "src/c.go"},
		{message: "fix: d", file: "src/d.go"},
	})
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}

	// keyword=feat 匹配 a、c，倒序为 c、a；limit=1 offset=1 → a
	commits, err := fetchCommitHistoryFromGit(gitRoot, 1, 1, model.CommitFilter{Keyword: "feat"})
	if err != nil {
		t.Fatalf("fetchCommitHistoryFromGit: %v", err)
	}
	if len(commits) != 1 || commits[0].Message != "feat: a\n" {
		t.Fatalf("feat offset=1 应返 feat: a: got %+v", commits)
	}

	// filePath=src 匹配 c、d
	commits2, err := fetchCommitHistoryFromGit(gitRoot, 10, 0, model.CommitFilter{FilePath: "src"})
	if err != nil {
		t.Fatalf("fetch filePath: %v", err)
	}
	if len(commits2) != 2 || commits2[0].Message != "fix: d\n" || commits2[1].Message != "feat: c\n" {
		t.Fatalf("filePath=src 应返 d、c: got %+v", commits2)
	}

	// 组合过滤 + 提前终止（limit 满即停）：author 无匹配返空
	commits3, err := fetchCommitHistoryFromGit(gitRoot, 10, 0, model.CommitFilter{Author: "nobody"})
	if err != nil {
		t.Fatalf("fetch author: %v", err)
	}
	if len(commits3) != 0 {
		t.Fatalf("author 无匹配应返空: got %+v", commits3)
	}
}

// writeMsgFile 将提交消息写入文件（支持含 0x01/中文/多行的消息），返回 -F 用的路径。
func writeMsgFile(t *testing.T, dir, message string) string {
	t.Helper()
	p := filepath.Join(dir, "commit-msg.txt")
	if err := os.WriteFile(p, []byte(message), 0o644); err != nil {
		t.Fatalf("write msg file: %v", err)
	}
	return p
}

// TestGetCommitHistory_MessageByteCompat 真实 git fixture：含中文/多行 body/0x01 的
// 提交消息经 CLI %B 采集后与 git 对象存储字节级一致（含尾部换行）。
func TestGetCommitHistory_MessageByteCompat(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "r")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	testutil.RunGit(t, repoPath, "init")
	testutil.RunGit(t, repoPath, "config", "user.name", "张三")
	testutil.RunGit(t, repoPath, "config", "user.email", "z@x.com")

	wantMessage := "fix: 修复登录\n\n详细 body 第二行\n含分隔符 \x01 的行\n"
	testutil.WriteFile(t, filepath.Join(repoPath, "a.txt"), "content\n")
	testutil.RunGit(t, repoPath, "add", "a.txt")
	testutil.RunGit(t, repoPath, "commit", "-q", "-F", writeMsgFile(t, t.TempDir(), wantMessage))

	app := newAppWithCommitCache()
	commits, err := app.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("GetCommitHistory: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("应 1 条提交: got %d", len(commits))
	}
	if commits[0].Message != wantMessage {
		t.Errorf("Message 应字节级一致:\n got %q\nwant %q", commits[0].Message, wantMessage)
	}
	if commits[0].Author != "张三" {
		t.Errorf("中文作者名: got %q", commits[0].Author)
	}
	if len(commits[0].Files) != 1 || commits[0].Files[0] != "a.txt" {
		t.Errorf("Files: got %v", commits[0].Files)
	}

	// 缓存未注入路径（fetchCommitHistoryFromGit）同字节级一致
	app2 := NewApp()
	commits2, err := app2.GetCommitHistory(repoPath, 10, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("nil cache GetCommitHistory: %v", err)
	}
	if commits2[0].Message != wantMessage {
		t.Errorf("nil cache Message 应字节级一致: got %q", commits2[0].Message)
	}
}

// TestFullScanCommits_CommitterTimeOrder 时间乱序线性提交：git log 缺省按 committer
// date 弹出（线性链内退化为父子链序），与 go-git LogOrderCommitterTime 行为一致
// （两者均为「待弹出集合按 committer date 取最新」语义）。锚定排序语义不因换采集层漂移。
func TestFullScanCommits_CommitterTimeOrder(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "r")
	makeCommits(t, repoPath, []commitSpec{
		{message: "c1-mar", file: "a.go", date: "2026-03-01 10:00:00"},
		{message: "c2-jan", file: "b.go", date: "2026-01-01 10:00:00"},
		{message: "c3-feb", file: "c.go", date: "2026-02-01 10:00:00"},
	})
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}

	all, overflow, scanErr := fullScanCommits(gitRoot)
	if scanErr != nil {
		t.Fatalf("fullScanCommits: %v", scanErr)
	}
	if overflow {
		t.Fatal("3 条提交不应超限")
	}
	if len(all) != 3 {
		t.Fatalf("应 3 条: got %d", len(all))
	}
	// 线性链：弹出序 = 链序 c3 → c2 → c1（child 先于 parent），与 go-git 一致
	wantOrder := []string{"c3-feb\n", "c2-jan\n", "c1-mar\n"}
	for i, want := range wantOrder {
		if all[i].Message != want {
			t.Errorf("第 %d 条应为 %q: got %q", i, want, all[i].Message)
		}
	}
	// Timestamp = author unix 秒（%at），与 go-git Author.When.Unix() 对齐。
	// 注意：线性链内两实现均按链序弹出（「待弹出集合按 committer date 取最新」在单元素
	// 集合下退化为链序），故首条 c3-feb（2026-02）先于末条 c1-mar（2026-03）但日期更早——
	// 非严格日期降序恰是与 go-git 一致的证据。
	if all[0].Timestamp >= all[2].Timestamp {
		t.Errorf("c3-feb(2026-02) 时间戳应早于 c1-mar(2026-03): %d vs %d", all[0].Timestamp, all[2].Timestamp)
	}
}

// TestGetCommitHistory_MergeCommitFiles merge 提交文件列表相对第一父提交
// （--diff-merges=first-parent），与 go-git getCommitFiles（Parent(0) diff）语义一致。
func TestGetCommitHistory_MergeCommitFiles(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "r")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	testutil.RunGit(t, repoPath, "init")
	testutil.RunGit(t, repoPath, "config", "user.name", "t")
	testutil.RunGit(t, repoPath, "config", "user.email", "t@t.com")
	testutil.WriteFile(t, filepath.Join(repoPath, "base.txt"), "base\n")
	testutil.RunGit(t, repoPath, "add", "-A")
	testutil.RunGit(t, repoPath, "commit", "-q", "-m", "base")
	testutil.RunGit(t, repoPath, "checkout", "-q", "-b", "feat")
	testutil.WriteFile(t, filepath.Join(repoPath, "feat.txt"), "feat\n")
	testutil.RunGit(t, repoPath, "add", "-A")
	testutil.RunGit(t, repoPath, "commit", "-q", "-m", "feat")
	testutil.RunGit(t, repoPath, "checkout", "-q", "master")
	testutil.WriteFile(t, filepath.Join(repoPath, "master.txt"), "master\n")
	testutil.RunGit(t, repoPath, "add", "-A")
	testutil.RunGit(t, repoPath, "commit", "-q", "-m", "masterchange")
	testutil.RunGit(t, repoPath, "merge", "--no-ff", "feat", "-m", "merge commit")

	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	all, overflow, scanErr := fullScanCommits(gitRoot)
	if scanErr != nil {
		t.Fatalf("fullScanCommits: %v", scanErr)
	}
	if overflow {
		t.Fatal("不应超限")
	}
	var merge *model.Commit
	for i := range all {
		if all[i].Message == "merge commit\n" {
			merge = &all[i]
		}
	}
	if merge == nil {
		t.Fatal("应含 merge commit")
	}
	// 相对第一父提交（masterchange）的变更 = 新增 feat.txt
	if len(merge.Files) != 1 || merge.Files[0] != "feat.txt" {
		t.Errorf("merge 文件列表应相对第一父提交 [feat.txt]: got %v", merge.Files)
	}
}

// TestFullScanCommits_NonGitRootReturnsNilFalse git log 失败（非仓库目录）返
// (nil, false, err)，底层错误落日志并透传，调用方据此 %w 包裹报错而不误判超限。
func TestFullScanCommits_NonGitRootReturnsNilFalse(t *testing.T) {
	commits, overflow, err := fullScanCommits(t.TempDir())
	if commits != nil || overflow {
		t.Errorf("非仓库应返 (nil, false): got (%v, %v)", commits, overflow)
	}
	if err == nil {
		t.Error("非仓库应透传底层错误供调用方包裹")
	}
}

// TestStreamCommitLog_StopEarly handle 返 false 提前终止不报错，且只回调到终止点。
func TestStreamCommitLog_StopEarly(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "r")
	makeCommits(t, repoPath, []commitSpec{
		{message: "c1", file: "a.go"},
		{message: "c2", file: "b.go"},
		{message: "c3", file: "c.go"},
	})
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}

	seen := 0
	err = streamCommitLog(gitRoot, commitLogArgs(0), func(c model.Commit) bool {
		seen++
		return seen < 2 // 收到第 2 条即停
	})
	if err != nil {
		t.Fatalf("streamCommitLog: %v", err)
	}
	if seen != 2 {
		t.Errorf("应在第 2 条提前终止: got %d", seen)
	}
}

// TestParseCommitLogChunk_AuthorEmailWithFieldSeparator 作者名/邮箱含 0x01（分隔符
// 错位）：头部强校验（SHA/parents 40 位 hex、ts 纯数字、ai 固定格式）快速失败报错，
// 不得静默解析出错位的错误数据。
func TestParseCommitLogChunk_AuthorEmailWithFieldSeparator(t *testing.T) {
	if c, err := parseCommitLogChunk(mkChunk(shaHex("an"), shaHex("p"), "bad\x01name", "e@x.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "m\n", []string{"f.go"})); err == nil || c != nil {
		t.Errorf("作者名含 0x01 应报错不静默解析: got (%v, %v)", c, err)
	}
	if c, err := parseCommitLogChunk(mkChunk(shaHex("ae"), shaHex("p"), "author", "e\x01@x.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "m\n", []string{"f.go"})); err == nil || c != nil {
		t.Errorf("邮箱含 0x01 应报错不静默解析: got (%v, %v)", c, err)
	}
}

// TestParseCommitLogChunk_FileNameWithFieldSeparator 文件名含 0x01（core.quotePath=false
// 原样输出）：终止符从记录尾部按 "\x01\n\n" 固定结构反向定位，文件名内 0x01 不干扰，
// 消息与文件列表均正确解析，无静默吞并。
func TestParseCommitLogChunk_FileNameWithFieldSeparator(t *testing.T) {
	chunk := mkChunk(shaHex("fn"), shaHex("p"), "a", "e@x.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "feat: files\n", []string{"bad\x01name.txt", "b.go"})
	c, err := parseCommitLogChunk(chunk)
	if err != nil {
		t.Fatalf("parseCommitLogChunk: %v", err)
	}
	if c.Message != "feat: files\n" {
		t.Errorf("Message 不应被文件名内 0x01 吞并文件列表: got %q", c.Message)
	}
	if len(c.Files) != 2 || c.Files[0] != "bad\x01name.txt" || c.Files[1] != "b.go" {
		t.Errorf("含 0x01 文件名应原样解析: got %v", c.Files)
	}
}

// TestParseCommitLogChunk_TerminatorAnchorPriority 消息区含 "\x01\n\n" 与空提交兜底
// 锚点的优先级：两锚点同时命中取更靠后者——无文件（空）提交的消息内可含 "\x01\n\n"，
// 须以记录尾部真实终止符为准；消息以 0x01 结尾且有文件时反向锚点不受干扰。
func TestParseCommitLogChunk_TerminatorAnchorPriority(t *testing.T) {
	// 空提交 + 消息含 "\x01\n\n"：应以尾部 "\x01\n" 兜底锚点为准，消息完整、无文件
	chunk := mkChunk(shaHex("ec"), shaHex("p"), "a", "e@x.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "m\x01\n\nrest\n", nil)
	c, err := parseCommitLogChunk(chunk)
	if err != nil {
		t.Fatalf("empty commit parse: %v", err)
	}
	if c.Message != "m\x01\n\nrest\n" {
		t.Errorf("空提交消息应完整保留: got %q", c.Message)
	}
	if c.Files != nil {
		t.Errorf("空提交不应解析出文件列表: got %v", c.Files)
	}

	// 消息以 0x01 结尾且有文件："\x01\n\n" 反向锚点落在格式终止符，解析正确
	chunk2 := mkChunk(shaHex("me"), shaHex("p"), "a", "e@x.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "m\x01\n", []string{"f.go"})
	c2, err := parseCommitLogChunk(chunk2)
	if err != nil {
		t.Fatalf("message-ends-with-0x01 parse: %v", err)
	}
	if c2.Message != "m\x01\n" {
		t.Errorf("以 0x01 结尾的消息应原样保留: got %q", c2.Message)
	}
	if len(c2.Files) != 1 || c2.Files[0] != "f.go" {
		t.Errorf("文件列表不受消息内 0x01 干扰: got %v", c2.Files)
	}
}
