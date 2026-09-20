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

// TestParseCommitLogChunk_Basic 常规记录：字段解析、Message 保留尾部换行、DateTime
// 取 %ai 首 19 字节、文件列表解析。
func TestParseCommitLogChunk_Basic(t *testing.T) {
	chunk := mkChunk("abc12345def", "parent1 parent2", "Alice", "a@x.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "feat: add login\n\nbody line\n",
		[]string{"src/a.go", "docs/b.md"})
	c, err := parseCommitLogChunk(chunk)
	if err != nil {
		t.Fatalf("parseCommitLogChunk: %v", err)
	}
	if c == nil {
		t.Fatal("chunk 不应为空")
	}
	if c.SHA != "abc12345def" {
		t.Errorf("SHA: got %q", c.SHA)
	}
	if c.ShortSHA != "abc12345" {
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
// 消息内时以记录内最后一个 0x01 为消息终止符，解析仍正确。
func TestParseCommitLogChunk_MessageWithSpecialBytes(t *testing.T) {
	rawMessage := "fix: 修复登录\n\n含分隔符 \x01 的正文行\n第二行\n"
	chunk := mkChunk("sha1", "p1", "张三", "z@x.com",
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
	c, err := parseCommitLogChunk(mkChunk("rootsha", "", "t", "t@t.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "root\n", files))
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	if len(c.Files) != rootCommitFileLimit {
		t.Errorf("root 提交文件应截断至 %d: got %d", rootCommitFileLimit, len(c.Files))
	}

	c2, err := parseCommitLogChunk(mkChunk("sha", "p", "t", "t@t.com",
		"1700000000", "2023-11-14 22:13:20 +0800", "non-root\n", files))
	if err != nil {
		t.Fatalf("non-root: %v", err)
	}
	if len(c2.Files) != 120 {
		t.Errorf("非 root 提交文件不截断: got %d", len(c2.Files))
	}
}

// TestParseCommitLogChunk_EmptyAndMalformed 空块返 nil 不报错；字段不足 / 时间戳非法 /
// 消息终止符缺失报 error。
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
	if _, err := parseCommitLogChunk(mkChunk("sha", "p", "a", "e", "notanumber",
		"2023-11-14 22:13:20 +0800", "m\n", nil)); err == nil {
		t.Error("时间戳非法应报错")
	}
	if _, err := parseCommitLogChunk("sha\x01p\x01a\x01e\x01" + "1700000000" + "\x01" + "ai"); err == nil {
		t.Error("消息终止符（最后一个 0x01）缺失应报错")
	}
}

// TestParseCommitLogOutput_MultiRecords 完整输出（首 0x00 + 多记录）解析。
func TestParseCommitLogOutput_MultiRecords(t *testing.T) {
	out := "\x00" + mkChunk("sha2", "sha3", "a", "e@x.com", "200", "1970-01-01 00:03:20 +0000", "second\n", []string{"2.txt"}) +
		"\x00" + mkChunk("sha3", "", "a", "e@x.com", "100", "1970-01-01 00:01:40 +0000", "root\n", []string{"1.txt"}) + "\n"
	commits, err := parseCommitLogOutput(out)
	if err != nil {
		t.Fatalf("parseCommitLogOutput: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("应解析 2 条: got %d", len(commits))
	}
	if commits[0].SHA != "sha2" || commits[1].SHA != "sha3" {
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

	all, overflow := fullScanCommits(gitRoot)
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
	all, overflow := fullScanCommits(gitRoot)
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
// (nil, false)，调用方据此报错而不误判超限。
func TestFullScanCommits_NonGitRootReturnsNilFalse(t *testing.T) {
	commits, overflow := fullScanCommits(t.TempDir())
	if commits != nil || overflow {
		t.Errorf("非仓库应返 (nil, false): got (%v, %v)", commits, overflow)
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
