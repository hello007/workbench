package service

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"workbench/model"
	"workbench/util"
)

// 行级归属追溯域 GitService 方法（git blame --porcelain 工作树行归属）。
//
// 入参 fileAbsPath 为文件绝对路径：内部 FindGitRoot 定位仓库根并换算仓库相对路径，
// 与 GetFileCommitHistory 同构的单参数自包含形态（前端直传预览文件绝对路径）。
//
// blame 不传 rev = 工作树 blame：输出行号与预览内容行号恒对齐（未提交行 SHA 全零，
// 前端标「未提交」禁跳转）；不用 blame HEAD——文件有未提交增删行时行号错位。
//
// 编码：作者名/提交消息可含中文，git 调用统一前置 utf8Prefix（branch-compare.md 范式）；
// \t 行内容为工作树原始字节（GBK 文件非 UTF-8），解析器跳过不消费，无乱码风险。
//
// 路径参数为字面路径：blame 不走 pathspec 机制，裸路径即精确命中含 glob 元字符的
// 文件名（与 git log 的 :(literal) 需求不同，勿混淆）；路径须 filepath.ToSlash 归一。
//
// 大文件：超过 blameMaxLines 直接拒绝（前端预判 + 后端 bufio 流式数行双保险），
// 不跑 blame 零进程开销；30s 超时由 gitCmd.Execute 兜底。只读不抢 tryLockRepo、
// 不注册取消（对齐 GetFileCommitHistory 只读先例）。

// blameMaxLines 行归属行数上限：超过即视为预览边缘场景（万行级文件），降级拒绝。
// 数量级对齐 fileHistoryMaxFetch。
const blameMaxLines = 10000

// blameSHAHeaderRe porcelain 行头：`<40位小写hex> <origLine> <finalLine> [count]`。
// count 仅在连续块首行出现；\t 内容行以 \t 开头不会命中；40 位 hex + 空格 + 数字
// 的形态足以与详情行（author/summary 等具名前缀）区分。
var blameSHAHeaderRe = regexp.MustCompile(`^([0-9a-f]{40}) (\d+) (\d+)(?: (\d+))?$`)

// GetFileBlame 获取文件工作树行归属（每行最后修改提交，porcelain 机械解析）。
// 返回 Commits 去重列表 + Lines 每行→提交下标数组（下标 = 行号-1，与工作树行号 1:1）。
func (s *GitService) GetFileBlame(fileAbsPath string) (*model.BlameResult, error) {
	if fileAbsPath == "" {
		return nil, fmt.Errorf("文件路径不能为空")
	}

	gitRoot, err := util.FindGitRoot(fileAbsPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	rel, err := filepath.Rel(gitRoot, fileAbsPath)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("文件不在 Git 仓库内: %s", fileAbsPath)
	}

	lineCount, err := countFileLines(fileAbsPath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	if lineCount > blameMaxLines {
		return nil, model.NewAppError(model.ErrCodeBlameFileTooLarge,
			fmt.Sprintf("文件共 %d 行，超过行归属上限 %d 行", lineCount, blameMaxLines))
	}

	// blame 的路径参数是字面路径（非 pathspec 机制，与 git log 不同）：裸路径即可
	// 精确命中含 glob 元字符的文件名（笔记[1].md），禁加 :(literal) 前缀（会被当作
	// 字面文件名导致 no such path）；路径归一正斜杠
	args := append(utf8Prefix(),
		"blame", "--porcelain",
		"--", filepath.ToSlash(rel))
	out, err := s.gitCmd.Execute(gitRoot, args...)
	if err != nil {
		return nil, fmt.Errorf("获取行归属失败: %w", err)
	}

	return parseBlameOutput(out)
}

// countFileLines 流式统计文件行数（按 \n 分割，末尾无换行的残段计一行，与
// CodeMirror 文档行数同口径）。bufio.Reader.ReadString 逐段读取，超长单行
// （压缩 JSON 等）安全；返回的行数同时作为超限判定依据。
func countFileLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	count := 0
	for {
		data, err := reader.ReadString('\n')
		if err == io.EOF {
			// 残段（末行无换行符）计一行；空残段说明以换行结尾或空文件
			if data != "" {
				count++
			}
			return count, nil
		}
		if err != nil {
			return 0, err
		}
		count++
	}
}

// blameRef 单条 porcelain 行记录：文件行号 → 提交下标。
type blameRef struct {
	final  int // finalLine（工作树行号，1-based）
	commit int // 提交在 Commits 中的下标
}

// parseBlameOutput 解析 git blame --porcelain 输出。格式：每行一条记录，以
// `<sha> <orig> <final> [count]` 行头开始；提交详情（author/author-mail/
// author-time/summary/filename 等）仅在该 SHA 首次出现时插入，\t 前缀为行内容。
//
// 分层容错：sha 行头解析失败或最终存在未归属行 → 整体报错（porcelain 为纯机械
// 格式，脏输出说明 git 输出异常，行号对齐契约破坏，结果不可信）；单条详情字段
// 脏（如 author-time 非法）→ Warn 跳过该字段，提交条目保留（fail-soft 对齐
// parseFileCommitHistory 先例）。
func parseBlameOutput(out string) (*model.BlameResult, error) {
	commits := []model.BlameCommit{}
	commitIdx := map[string]int{} // SHA → Commits 下标
	refs := []blameRef{}
	cur := -1 // 当前详情行归属的提交下标（sha 行切换）

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		if m := blameSHAHeaderRe.FindStringSubmatch(line); m != nil {
			sha := m[1]
			idx, seen := commitIdx[sha]
			if !seen {
				idx = len(commits)
				commitIdx[sha] = idx
				commits = append(commits, model.BlameCommit{SHA: sha, ShortSHA: sha[:8]})
			}
			cur = idx
			final, err := strconv.Atoi(m[3])
			if err != nil || final < 1 {
				return nil, fmt.Errorf("行归属行头行号非法: %s", truncateNumstatForLog(line))
			}
			refs = append(refs, blameRef{final: final, commit: idx})
			continue
		}

		if strings.HasPrefix(line, "\t") {
			continue // 行内容：前端已有 content，跳过不消费（GBK 字节无乱码风险）
		}

		// 提交详情行（仅 SHA 首次出现时）；填充 cur 指向的条目
		if cur >= 0 {
			applyBlameDetail(&commits[cur], line)
			continue
		}
		Logger().Warn("行归属未知输出行跳过", "line", truncateNumstatForLog(line))
	}

	if len(refs) == 0 {
		// 完全空输出（空文件）合法：空结果非错误，前端零值稳定；
		// 有输出但零行头记录（如行头全部脏）属契约破坏，报错
		if strings.TrimSpace(out) != "" {
			return nil, fmt.Errorf("行归属输出无有效行记录")
		}
		return &model.BlameResult{Commits: []model.BlameCommit{}, Lines: []int{}}, nil
	}

	total := 0
	for _, r := range refs {
		if r.final > total {
			total = r.final
		}
	}
	lines := make([]int, total)
	for i := range lines {
		lines[i] = -1
	}
	missing := 0
	for _, r := range refs {
		lines[r.final-1] = r.commit
	}
	for _, v := range lines {
		if v < 0 {
			missing++
		}
	}
	if missing > 0 {
		return nil, fmt.Errorf("行归属解析不完整: %d/%d 行未归属", missing, total)
	}

	return &model.BlameResult{Commits: commits, Lines: lines}, nil
}

// applyBlameDetail 将单条详情行写入提交条目：author / author-mail / author-time
// <unixTs>（时区在后续独立行 author-tz <tz>，到齐合成 DateTime）/ summary。其余
// 字段（committer*/filename/boundary/previous 等）不消费，静默跳过（防御 git 新
// 版本新增字段）。author-time 非法时 Warn 跳过该字段（Timestamp/DateTime 留零值），
// 不阻断整体。
func applyBlameDetail(c *model.BlameCommit, line string) {
	switch {
	case strings.HasPrefix(line, "author "):
		c.Author = strings.TrimPrefix(line, "author ")
	case strings.HasPrefix(line, "author-mail "):
		c.Email = strings.Trim(strings.TrimSuffix(strings.TrimPrefix(line, "author-mail "), ">"), "<")
	case strings.HasPrefix(line, "author-time "):
		// porcelain 格式 author-time 与时区分行：author-time <unixTs>、author-tz <tz>
		ts, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "author-time ")), 10, 64)
		if err != nil {
			Logger().Warn("行归属时间戳解析失败，跳过", "line", truncateNumstatForLog(line))
			return
		}
		c.Timestamp = ts
	case strings.HasPrefix(line, "author-tz "):
		// 时区行晚于 author-time 行（porcelain 固定顺序），到齐后合成墙钟时间串
		if c.Timestamp != 0 {
			c.DateTime = blameDateTime(c.Timestamp, strings.TrimSpace(strings.TrimPrefix(line, "author-tz ")))
		}
	case strings.HasPrefix(line, "summary "):
		c.Message = strings.TrimPrefix(line, "summary ")
	}
}

// blameDateTime 作者时区墙钟时间串：ts + 时区偏移后按 UTC 格式化，口径对齐
// FileCommit.DateTime（%ai 首 19 字节 = 作者时区墙钟，非本地时区避免转换漂移）。
// tz 形如 "+0800"/"-0530"；非法 tz 回退 UTC。
func blameDateTime(ts int64, tz string) string {
	offset := int64(0)
	if len(tz) == 5 && (tz[0] == '+' || tz[0] == '-') && isDigits(tz[1:]) {
		h := int64(tz[1]-'0')*10 + int64(tz[2]-'0')
		m := int64(tz[3]-'0')*10 + int64(tz[4]-'0')
		offset = h*3600 + m*60
		if tz[0] == '-' {
			offset = -offset
		}
	} else if tz != "" {
		Logger().Warn("行归属时区偏移非法，按 UTC 呈现", "tz", tz)
	}
	return time.Unix(ts+offset, 0).UTC().Format("2006-01-02 15:04:05")
}

// isDigits 判定字符串是否全为 ASCII 数字（时区偏移字段校验用）。
func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}
