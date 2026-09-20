package main

// 提交历史采集层（CLI git log 批量/流式实现）。
//
// 背景：原采集层经 go-git 逐提交做树 diff（Patch），每条 ≥2 次对象库读，Windows 上
// 慢 1-2 个数量级（300 提交冷扫 12.8s，见 docs/spec/perf-baseline.md 提交历史维度）。
// 本文件将采集替换为 CLI `git log` 子进程：全量扫批量一次取回（-n 上限+1 探测超限），
// overflow 翻页与增量 prepend 走 stdout 流式读取、凑够/命中即提前终止（不读全量）。
//
// 输出格式契约（鲁棒解析，消息可含换行/中文/0x01）：
//
//	git -c core.quotePath=false log [--diff-merges=first-parent] \
//	     [-n <count>] --format="%x00%H%x01%P%x01%an%x01%ae%x01%at%x01%ai%x01%B%x01" --name-only
//
//	记录分隔 = 0x00；字段分隔 = 0x01。每记录：
//	  \0<SHA>\x01<parents>\x01<author>\x01<email>\x01<authorUnixTs>\x01<authorDateISO>\x01<rawMessage>\x01\n\n<files>\n
//
//	- 0x00 作记录/字段间分隔安全：git 原生拒绝消息含 NUL 字节（"a NUL byte in
//	  commit log message not allowed"），文件系统路径同样不能含 NUL。
//	- 消息用 %B（raw message，含 subject+body+尾部换行）非 %s+%b 拼接，与 go-git
//	  Commit.Message 字节级一致（既有测试断言 Message 带尾部 "\n"）。消息内出现 0x01
//	  时由 parseCommitLogChunk 的终止符反向定位消化（见其函数注释）。
//	- 已知边界（均不产生静默错数据的现实路径）：作者名/邮箱含 0x01 时分隔符错位，
//	  头部强校验快速失败报错（不静默）；文件名以 0x01 结尾（Windows 文件名禁控制
//	  字符不可构造，Linux 极端手工构造）会与「\x01\n 结尾」空提交兜底锚点冲突，
//	  可能误判为无文件提交。
//	- %at = author unix 秒（与 go-git Author.When.Unix() 一致）；%ai 首 19 字节即
//	  作者时区墙钟 "2006-01-02 15:04:05"，与 go-git Author.When.Format(...) 一致
//	  （非本地时区，避免转换漂移）。
//	- --diff-merges=first-parent 使 merge 提交列相对第一父提交的变更文件，与 go-git
//	  getCommitFiles（Parent(0) diff）语义一致（缺省 merge 提交不列文件）。须 git ≥2.31。
//	- core.quotePath=false 输出原始非 ASCII 路径（缺省会转义 \3xx 序列，破坏 FilePath
//	  子串过滤），与 go-git 原始路径一致。
//	- root 提交（parents 空）CLI 列全树文件，截断至 rootCommitFileLimit，对齐原
//	  getTreeFiles 100 上限。
//
// 排序：git log 缺省按 committer date 倒序，与 go-git LogOrderCommitterTime 对齐。
// 过滤：Since/Until/Author/Keyword/FilePath 仍在内存过滤（filterCommits / 流式同谓词），
// 不下推 CLI flags（pathspec 子串与 --author 正则语义均与内存子串匹配不等价）。

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"workbench/model"
	"workbench/service"
	"workbench/util"
)

// commitLogTimeout git log 子进程超时，与 util.GitCommand 缺省 30s 对齐。
const commitLogTimeout = 30 * time.Second

// rootCommitFileLimit root 提交文件列表上限（全树列举），对齐 getTreeFiles。
const rootCommitFileLimit = 100

// commitLogFormat git log --format 格式串，字段序见文件头契约。
const commitLogFormat = "%x00%H%x01%P%x01%an%x01%ae%x01%at%x01%ai%x01%B%x01"

// commitLogArgs 构造 git log 参数。count>0 时加 -n 上限（批量全量扫/超限探测），
// count<=0 不限量（流式路径由调用方提前终止控制读取量）。
func commitLogArgs(count int) []string {
	args := []string{
		"-c", "core.quotePath=false",
		"log",
		"--diff-merges=first-parent",
		"--format=" + commitLogFormat,
		"--name-only",
	}
	if count > 0 {
		args = append(args, "-n", strconv.Itoa(count))
	}
	return args
}

// loadCommitLogBatch 批量执行 git log（缓冲全量 stdout），解析为 model.Commit 切片。
// 供全量扫（-n MaxEntries+1）使用；5000 条约 2-5MB 缓冲可接受。
func loadCommitLogBatch(gitRoot string, count int) ([]model.Commit, error) {
	out, err := util.NewGitCommand().Execute(gitRoot, commitLogArgs(count)...)
	if err != nil {
		return nil, err
	}
	return parseCommitLogOutput(out)
}

// parseCommitLogOutput 解析 git log 完整输出（\x00 记录分隔）为提交切片。
func parseCommitLogOutput(out string) ([]model.Commit, error) {
	chunks := strings.Split(out, "\x00")
	commits := make([]model.Commit, 0, len(chunks))
	for _, chunk := range chunks {
		c, err := parseCommitLogChunk(chunk)
		if err != nil {
			return nil, err
		}
		if c != nil {
			commits = append(commits, *c)
		}
	}
	return commits, nil
}

// parseCommitLogChunk 解析单条 git log 记录块（不含前导 0x00，流式路径可能含两端
// 0x00，统一 Trim 消化）。首记录前空块返 (nil, nil)。
//
// 解析策略（任一字段含 0x01/0x0a 时要么正确解析要么报错，不静默产出错数据）：
//  1. 头部强校验：前 6 个 0x01 分隔出 SHA/parents/author/email/ts/authorDateISO 后，
//     校验 SHA 与 parents 为 40 位 hex、ts 为纯数字、authorDateISO 为固定格式。
//     作者名/邮箱含 0x01 会使分隔符错位，错位后的字段几乎必然不满足上述格式，
//     快速失败报错（此前策略仅对最后 0x01 取 LastIndex，错位时可静默解析错数据）。
//  2. 消息终止符与文件列表从记录尾部反向定位：有文件提交的终止符后随 "\n\n"
//     （格式换行 + --name-only 段前空行，实测 git 输出固定结构），取最后一个
//     "\x01\n\n" 为终止锚点，其后即文件行——消息与文件名内的 0x01 均不干扰；
//     无文件提交（空提交，--name-only 无输出）终止符后仅一个 "\n"，以「记录以
//     "\x01\n" 结尾」为兜底锚点。两锚点同时命中取更靠后者（无文件提交的消息内
//     可含 "\x01\n\n"，须以记录尾部真实终止符为准）。
func parseCommitLogChunk(chunk string) (*model.Commit, error) {
	chunk = strings.Trim(chunk, "\x00")
	if chunk == "" {
		return nil, nil
	}

	// 定位第 6 个 0x01（其前为头部 6 字段 SHA/parents/author/email/ts/authorDateISO，
	// 其后为消息区；6 字段间共 5 个分隔符，第 6 个为 ai 与消息间的分隔符）
	headerEnd := -1
	seps := 0
	for i := 0; i < len(chunk); i++ {
		if chunk[i] == '\x01' {
			seps++
			if seps == 6 {
				headerEnd = i
				break
			}
		}
	}
	if seps < 6 {
		return nil, fmt.Errorf("git log 记录头部字段不完整: %q", truncateForLog(chunk))
	}
	header := strings.SplitN(chunk[:headerEnd], "\x01", 6)
	if !isHexSHA(header[0]) {
		return nil, fmt.Errorf("git log 记录 SHA 非法（字段可能错位）: %q", truncateForLog(header[0]))
	}
	if !isHexSHAList(header[1]) {
		return nil, fmt.Errorf("git log 记录 parents 非法（字段可能错位）: %q", truncateForLog(header[1]))
	}
	ts, err := strconv.ParseInt(header[4], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("git log 提交时间戳解析失败: %q", header[4])
	}
	if !isAuthorDateISO(header[5]) {
		return nil, fmt.Errorf("git log 记录作者日期非法（字段可能错位）: %q", truncateForLog(header[5]))
	}

	// 消息终止符反向定位（两种锚点取更靠后者，-1 表示未命中）
	termFiles := strings.LastIndex(chunk, "\x01\n\n")
	termEmpty := -1
	if strings.HasSuffix(chunk, "\x01\n") {
		termEmpty = len(chunk) - 2
	}
	var message string
	var files []string
	switch {
	case termFiles >= 0 && termFiles > termEmpty:
		message = chunk[headerEnd+1 : termFiles]
		files = parseCommitLogFiles(chunk[termFiles+len("\x01\n\n"):])
	case termEmpty >= 0 && termEmpty > termFiles:
		message = chunk[headerEnd+1 : termEmpty]
	default:
		return nil, fmt.Errorf("git log 记录消息终止符缺失: %q", truncateForLog(chunk))
	}
	// root 提交（无 parent）CLI 列全树文件，对齐 getTreeFiles 的 100 上限
	if header[1] == "" && len(files) > rootCommitFileLimit {
		files = files[:rootCommitFileLimit]
	}

	return &model.Commit{
		SHA:       header[0],
		ShortSHA:  shortSHA(header[0]),
		Message:   message,
		Author:    header[2],
		Email:     header[3],
		Timestamp: ts,
		DateTime:  authorDateWallClock(header[5]),
		Files:     files,
	}, nil
}

// isHexSHA 判定 40 位小写 hex SHA-1（git %H/%P 输出格式；SHA-256 仓库不在支持范围，
// 与 go-git plumbing 的 SHA-1 用法一致）。
func isHexSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// isHexSHAList 判定 %P 输出：空串（root 提交无 parent）或 40 位 hex SHA 以单个空格
// 分隔（merge 多父）。
func isHexSHAList(s string) bool {
	if s == "" {
		return true
	}
	for _, p := range strings.Split(s, " ") {
		if !isHexSHA(p) {
			return false
		}
	}
	return true
}

// isAuthorDateISO 判定 %ai 输出固定格式 "2006-01-02 15:04:05 +0800"（25 字节，
// 时区为 +/-HHMM）。
func isAuthorDateISO(s string) bool {
	if len(s) != 25 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		var ok bool
		switch i {
		case 4, 7: // 年-月 间的连字符
			ok = c == '-'
		case 10, 19: // 日期与时间、时间与时区 间的空格
			ok = c == ' '
		case 13, 16: // 时:分:秒 间的冒号
			ok = c == ':'
		case 20: // 时区符号
			ok = c == '+' || c == '-'
		default: // 其余位置均为数字
			ok = c >= '0' && c <= '9'
		}
		if !ok {
			return false
		}
	}
	return true
}

// parseCommitLogFiles 解析记录尾部的文件列表段（"\n\n<file>\n..."）。空列表返 nil，
// 与原 getCommitFiles 无文件时返 nil 切片一致。逐行剔除可能的 \r（防御性）。
func parseCommitLogFiles(raw string) []string {
	raw = strings.Trim(raw, "\n")
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	files := make([]string, 0, len(lines))
	for _, l := range lines {
		if l = strings.TrimRight(l, "\r"); l != "" {
			files = append(files, l)
		}
	}
	if len(files) == 0 {
		return nil
	}
	return files
}

// shortSHA 取前 8 位短 SHA（对齐 toModelCommit 原逻辑），SHA 异常短时原样返回。
func shortSHA(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

// authorDateWallClock 取 %ai（如 "2020-01-01 12:00:00 +0500"）首 19 字节墙钟，
// 与 go-git Author.When.Format("2006-01-02 15:04:05")（作者时区非本地）字节级一致。
func authorDateWallClock(ai string) string {
	if len(ai) >= 19 {
		return ai[:19]
	}
	return ai
}

// truncateForLog 解析失败信息截断，防超长记录撑爆错误日志。
func truncateForLog(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

// streamCommitLog 流式执行 git log，逐记录（\x00 分隔）解析并回调 handle；
// handle 返回 false 即提前终止（关闭 stdout 管道，git 写失败自行退出，不读全量）。
//
// 返回 error 仅表示真实失败（启动失败 / 解析失败 / 非提前终止的 git 退出错误）；
// 提前终止属正常控制流，返 nil。
func streamCommitLog(gitRoot string, args []string, handle func(model.Commit) bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), commitLogTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = gitRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	util.HideCommandWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建 git log 管道失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 git log 失败: %w, stderr: %s", err, stderr.String())
	}

	// finish 收尾（cancel 强杀 + Wait），提前终止时 git 被强制结束属预期，忽略其错误。
	// 须先 cancel 再 Wait：cancel 幂等且立即生效，保证 git 未随管道关闭退出时 Wait
	// 不会阻塞到 30s 超时之外（defer cancel 在本函数返回后才执行，无法兜底 Wait）。
	finish := func() {
		cancel()
		_ = cmd.Wait()
	}

	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		chunk, readErr := reader.ReadBytes('\x00')
		if len(chunk) > 0 {
			c, parseErr := parseCommitLogChunk(string(chunk))
			if parseErr != nil {
				finish()
				return parseErr
			}
			if c != nil && !handle(*c) {
				finish()
				return nil
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break // 历史读完（最后一条记录无尾随 0x00，随 EOF 返回）
			}
			finish()
			return fmt.Errorf("读取 git log 输出失败: %w", readErr)
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git log failed: %w, stderr: %s", err, stderr.String())
	}
	return nil
}

// truncateOverflow 全量扫超限截断：达 MaxEntries+1 说明溢出，返前 MaxEntries 条 +
// overflow=true（调用方走不缓存翻页路径或采样统计复用）。独立纯函数便于单测。
func truncateOverflow(commits []model.Commit) ([]model.Commit, bool) {
	if len(commits) > service.CommitHistoryMaxEntries {
		return commits[:service.CommitHistoryMaxEntries], true
	}
	return commits, false
}
