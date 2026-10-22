package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// 分支对比服务（活动栏「分支对比」面板 batch 1，纯只读，不抢 tryLockRepo 仓级锁）。
//
// 算法移植自 git-branch-compare（Electron 项目），三段：
//  1. 对齐：git merge-base 分叉下界 + 双侧区间日志（B..A / A..B）+ 贪心三级配对
//     （hash → patch-id → 归一化 subject）
//  2. patch-id 批量管道：git log <range> -p --format=%H | git patch-id --stable，
//     失败降级为仅 hash/subject 匹配（patchId 留空），不阻塞主流程
//  3. 文件树并集：两侧 ls-tree -r + 一趟 diff --name-status/--numstat
//
// UTF-8 前缀：本服务所有 git 命令带 -c i18n.logOutputEncoding=UTF-8
// -c i18n.commitEncoding=UTF-8 -c core.quotepath=off（现有 util.GitCommand 不默认带，
// 属既有独立问题，本任务不扩大范围改全局，见任务 PRD 已定决策）。

// branchCompareMaxCommits 单侧区间日志上限（--max-count），触顶置 Truncated 明示截断不静默。
const branchCompareMaxCommits = 2000

// BranchCompareService 分支对比服务。双 GitCommand：常规命令 30s 缺省超时；
// patch-id 管道涉及全量 patch 文本计算，专用 2 分钟长超时（PRD 已定决策）。
type BranchCompareService struct {
	gitCmd         *util.GitCommand // 常规 git 命令（merge-base/log/ls-tree/diff 等，30s）
	pipelineGitCmd *util.GitCommand // patch-id 管道专用（2 分钟长超时）
}

// NewBranchCompareService 创建分支对比服务（纯构造，无副作用）。
func NewBranchCompareService() *BranchCompareService {
	return &BranchCompareService{
		gitCmd:         util.NewGitCommand(),
		pipelineGitCmd: util.NewGitCommandWithTimeout(2 * time.Minute),
	}
}

// utf8Prefix Windows 中文路径/中文提交消息关键的 UTF-8 输出参数前缀：
// 本服务所有 git 调用（含管道两段）统一前置，调用方以 append(utf8Prefix(), 原参数...) 拼接。
func utf8Prefix() []string {
	return []string{
		"-c", "i18n.logOutputEncoding=UTF-8",
		"-c", "i18n.commitEncoding=UTF-8",
		"-c", "core.quotepath=off",
	}
}

// GetCommitAlignment 计算两分支的提交对齐结果：merge-base 分叉下界 + 双侧独有提交
// （B..A / A..B，各限 2000 条）+ patch-id 批量管道回填 + 贪心三级配对。
//
// 容错语义：merge-base 失败（unrelated histories） tolerated → MergeBaseHash=""；
// rev-list --count 失败 tolerated → 0；patch-id 管道失败降级（仅告警，patchId 留空），
// 三者均不整体报错。双侧区间日志失败（如分支不存在）才返回错误。
func (s *BranchCompareService) GetCommitAlignment(ctx context.Context, repoPath, branchA, branchB string) (*model.BranchCompareAlignment, error) {
	if branchA == "" {
		return nil, fmt.Errorf("分支 A 不能为空")
	}
	if branchB == "" {
		return nil, fmt.Errorf("分支 B 不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	// merge-base：无共同祖先（orphan 分支）或解析失败时容忍为空串，不整体报错
	mergeBase, _ := s.gitCmd.ExecuteWithCodesWithContext(ctx, gitRoot, map[int]bool{1: true},
		append(utf8Prefix(), "merge-base", branchA, branchB)...)
	mergeBase = strings.TrimSpace(mergeBase)

	// merge-base 之下共同历史计数：失败容忍为 0
	commonHistoryCount := 0
	if mergeBase != "" {
		if out, err := s.gitCmd.ExecuteWithContext(ctx, gitRoot,
			append(utf8Prefix(), "rev-list", "--count", mergeBase)...); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
				commonHistoryCount = n
			}
		}
	}

	// 双侧区间日志并发拉取（git log B..A = A 独有，A..B = B 独有）
	logFormat := "%x00%H%x01%h%x01%an%x01%ae%x01%ct%x01%s%x01%b%x01%P"
	logArgs := func(exclusive, branch string) []string {
		return append(utf8Prefix(), "log", exclusive+".."+branch,
			"--format="+logFormat, "--max-count="+strconv.Itoa(branchCompareMaxCommits), "--topo-order")
	}
	var (
		wg         sync.WaitGroup
		aOut, bOut string
		aErr, bErr error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		aOut, aErr = s.gitCmd.ExecuteWithContext(ctx, gitRoot, logArgs(branchB, branchA)...)
	}()
	go func() {
		defer wg.Done()
		bOut, bErr = s.gitCmd.ExecuteWithContext(ctx, gitRoot, logArgs(branchA, branchB)...)
	}()
	wg.Wait()
	if aErr != nil {
		return nil, fmt.Errorf("获取分支 %s 提交列表失败: %w", branchA, aErr)
	}
	if bErr != nil {
		return nil, fmt.Errorf("获取分支 %s 提交列表失败: %w", branchB, bErr)
	}

	aList := parseCommitRecords(aOut, branchA)
	bList := parseCommitRecords(bOut, branchB)

	// patch-id 批量管道并发（独立 2 分钟超时 GitCommand）；失败降级仅告警，patchId 留空
	var (
		pipeWg         sync.WaitGroup
		aPatch, bPatch map[string]string
	)
	pipeWg.Add(2)
	go func() {
		defer pipeWg.Done()
		p, err := s.fetchPatchIDMap(ctx, gitRoot, branchB+".."+branchA)
		if err != nil {
			Logger().Warn("patch-id 管道失败，降级为仅 hash/subject 匹配", "range", branchB+".."+branchA, "err", err)
			return
		}
		aPatch = p
	}()
	go func() {
		defer pipeWg.Done()
		p, err := s.fetchPatchIDMap(ctx, gitRoot, branchA+".."+branchB)
		if err != nil {
			Logger().Warn("patch-id 管道失败，降级为仅 hash/subject 匹配", "range", branchA+".."+branchB, "err", err)
			return
		}
		bPatch = p
	}()
	pipeWg.Wait()
	backfillPatchIDs(aList, aPatch)
	backfillPatchIDs(bList, bPatch)

	rows := pairCommits(aList, bList)

	return &model.BranchCompareAlignment{
		BranchA:            branchA,
		BranchB:            branchB,
		MergeBaseHash:      mergeBase,
		CommonHistoryCount: commonHistoryCount,
		Rows:               rows,
		Stats:              computeStats(rows, commonHistoryCount),
		Truncated:          len(aList) >= branchCompareMaxCommits || len(bList) >= branchCompareMaxCommits,
	}, nil
}

// fetchPatchIDMap 执行两进程管道 `git log <range> -p --format=%H` | `git patch-id --stable`，
// 解析 "<patchId> <commitHash>" 行为 hash→patchId 映射。调用方对错误降级处理（不整体报错）。
func (s *BranchCompareService) fetchPatchIDMap(ctx context.Context, gitRoot, rangeRev string) (map[string]string, error) {
	first := append(utf8Prefix(), "log", rangeRev, "-p", "--format=%H")
	// second 为 git 参数（不含程序名，ExecutePipeline 统一前置 git）
	second := []string{"patch-id", "--stable"}
	out, err := s.pipelineGitCmd.ExecutePipeline(ctx, gitRoot, first, second)
	if err != nil {
		return nil, err
	}
	return parsePatchIDOutput(out), nil
}

// parsePatchIDOutput 解析 git patch-id 输出（每行 "<patchId> <commitHash>"）。
// 空行与畸形行跳过（patch-id 对 merge 提交无输出，属正常）。
func parsePatchIDOutput(out string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		m[fields[1]] = fields[0]
	}
	return m
}

// backfillPatchIDs 把 hash→patchId 映射回填到提交切片（管道失败时映射为 nil，patchId 保持空串）。
func backfillPatchIDs(commits []model.BranchCompareCommit, patchMap map[string]string) {
	if patchMap == nil {
		return
	}
	for i := range commits {
		if id, ok := patchMap[commits[i].Hash]; ok {
			commits[i].PatchID = id
		}
	}
}

// cherryPickSubjectRe 归一化要剥离的 cherry-pick 尾注（git cherry-pick -x 生成的固定格式）。
var cherryPickSubjectRe = regexp.MustCompile(`\s*\(cherry picked from commit [^)]+\)\s*`)

// normalizeSubject 提交标题归一化：剥离 "(cherry picked from commit <sha>)" 尾注、
// trim、lowercase，供第三级 subject 匹配。与源项目语义一致。
func normalizeSubject(subject string) string {
	return strings.ToLower(strings.TrimSpace(cherryPickSubjectRe.ReplaceAllString(subject, "")))
}

// pairCommits 贪心三级配对（纯函数）：两侧各按日期降序（同秒按 hash 兜底保证确定性），
// 对每个 A 提交依序在 B 侧找第一个未使用的匹配：(1) hash 精确 → (2) patchId 非空且相等
// → (3) 归一化 subject 相等；都不中 → a-only。剩余 B → b-only。
// 一配一无回溯（非全局最优，行为可预测，对齐源项目）。
// 返回行按 max(a.Date, b.Date) 降序（同值按 Key 兜底）。
func pairCommits(aList, bList []model.BranchCompareCommit) []model.BranchCompareRow {
	aSorted := make([]model.BranchCompareCommit, len(aList))
	copy(aSorted, aList)
	bSorted := make([]model.BranchCompareCommit, len(bList))
	copy(bSorted, bList)
	sort.SliceStable(aSorted, func(i, j int) bool {
		if aSorted[i].Date != aSorted[j].Date {
			return aSorted[i].Date > aSorted[j].Date
		}
		return aSorted[i].Hash < aSorted[j].Hash
	})
	sort.SliceStable(bSorted, func(i, j int) bool {
		if bSorted[i].Date != bSorted[j].Date {
			return bSorted[i].Date > bSorted[j].Date
		}
		return bSorted[i].Hash < bSorted[j].Hash
	})

	used := make([]bool, len(bSorted))
	rows := make([]model.BranchCompareRow, 0, len(aSorted)+len(bSorted))
	for i := range aSorted {
		a := aSorted[i]
		matched := -1
		matchedBy := model.RowMatchedByNone
		// 三级依序匹配，每级独立扫描找「第一个未使用」的 B 提交
		for tier, check := range []func(a, b model.BranchCompareCommit) bool{
			func(a, b model.BranchCompareCommit) bool { return a.Hash == b.Hash },
			func(a, b model.BranchCompareCommit) bool { return a.PatchID != "" && a.PatchID == b.PatchID },
			func(a, b model.BranchCompareCommit) bool {
				return normalizeSubject(a.Subject) != "" && normalizeSubject(a.Subject) == normalizeSubject(b.Subject)
			},
		} {
			for j := range bSorted {
				if used[j] {
					continue
				}
				if check(a, bSorted[j]) {
					matched = j
					switch tier {
					case 0:
						matchedBy = model.RowMatchedByHash
					case 1:
						matchedBy = model.RowMatchedByPatchID
					default:
						matchedBy = model.RowMatchedBySubject
					}
					break
				}
			}
			if matched >= 0 {
				break
			}
		}
		if matched >= 0 {
			used[matched] = true
			b := bSorted[matched]
			rows = append(rows, model.BranchCompareRow{
				Key:       a.Hash + ".." + b.Hash,
				A:         &a,
				B:         &b,
				Side:      model.RowSideMatched,
				MatchedBy: matchedBy,
			})
		} else {
			rows = append(rows, model.BranchCompareRow{
				Key:       a.Hash,
				A:         &a,
				B:         nil,
				Side:      model.RowSideAOnly,
				MatchedBy: model.RowMatchedByNone,
			})
		}
	}
	for j := range bSorted {
		if used[j] {
			continue
		}
		b := bSorted[j]
		rows = append(rows, model.BranchCompareRow{
			Key:       b.Hash,
			A:         nil,
			B:         &b,
			Side:      model.RowSideBOnly,
			MatchedBy: model.RowMatchedByNone,
		})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		di, dj := rowDate(rows[i]), rowDate(rows[j])
		if di != dj {
			return di > dj
		}
		return rows[i].Key < rows[j].Key
	})
	return rows
}

// rowDate 行排序时间键：max(a.Date, b.Date)（配对行取双侧较新，独有行取自身）。
func rowDate(r model.BranchCompareRow) int64 {
	if r.A != nil && r.B != nil {
		if r.A.Date >= r.B.Date {
			return r.A.Date
		}
		return r.B.Date
	}
	if r.A != nil {
		return r.A.Date
	}
	if r.B != nil {
		return r.B.Date
	}
	return 0
}

// computeStats 统计条聚合（纯函数）：A/B 独有、配对总数、疑似拣选数、共同历史数。
func computeStats(rows []model.BranchCompareRow, commonBelow int) model.BranchCompareStats {
	stats := model.BranchCompareStats{CommonBelow: commonBelow}
	for _, r := range rows {
		switch r.Side {
		case model.RowSideAOnly:
			stats.AOnly++
		case model.RowSideBOnly:
			stats.BOnly++
		case model.RowSideMatched:
			stats.Matched++
			if r.MatchedBy == model.RowMatchedByPatchID {
				stats.PatchMatched++
			}
		}
	}
	return stats
}

// parseCommitRecords 解析区间日志输出（\x00 记录分隔、\x01 字段分隔，格式串
// "%x00%H%x01%h%x01%an%x01%ae%x01%ct%x01%s\x01%b\x01%P"）为提交切片。
//
// 防注入：%b（正文）可含多行与理论上 \x01 字节，正文之后不再有独立字段——
// 以「末段 = parents」锚定，中间多出的段全部折叠回正文；首段 40 位 hex 校验
// 兜底错位（与 commit_history_cli.go 的头部强校验同思路，不静默产出错数据）。
// 容忍末记录缺尾部分隔符与前后空记录（首记录前导 \x00 切出空块，跳过）。
func parseCommitRecords(out, branchName string) []model.BranchCompareCommit {
	records := strings.Split(out, "\x00")
	commits := make([]model.BranchCompareCommit, 0, len(records))
	for _, rec := range records {
		rec = strings.TrimPrefix(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		fields := strings.Split(rec, "\x01")
		// 8 个基础字段：H h an ae ct s b P；正文注入 \x01 时段数更多
		if len(fields) < 8 {
			continue
		}
		if len(fields[0]) != 40 || !isHex(fields[0]) {
			continue
		}
		ts, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			continue
		}
		subject := fields[5]
		body := strings.Join(fields[6:len(fields)-1], "\x01")
		parents := fields[len(fields)-1]
		commit := model.BranchCompareCommit{
			Hash:       fields[0],
			ShortHash:  fields[1],
			Author:     fields[2],
			Email:      fields[3],
			Date:       ts,
			Subject:    subject,
			Body:       body,
			BranchName: branchName,
		}
		if parents = strings.TrimSpace(parents); parents != "" {
			commit.ParentHashes = strings.Fields(parents)
		}
		commits = append(commits, commit)
	}
	return commits
}

// isHex 判定字符串是否全为十六进制字符（parseCommitRecords 的 hash 强校验用）。
func isHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// nameStatusEntry git diff --name-status 单条目：原始状态字母 + R/C 的旧路径。
type nameStatusEntry struct {
	Status  string // 原始状态字母（A/D/M/T/R/C，含分数如 R100 已剥离分数）
	OldPath string // R/C 时的旧路径，其余为空
}

// parseNameStatus 解析 `git diff --name-status` 输出：
//   - 普通行 "<status>\t<path>"（A/D/M/T）
//   - 重命名/复制行 "<status><score>\t<old>\t<new>"（R/C，如 "R100\told\tnew"）
//
// 返回 map：键 = 新路径（R/C）或路径本身（其余），值含状态字母与 R/C 旧路径。
// 畸形行跳过（空行等）。core.quotepath=off 前提下路径为原始非转义文本。
func parseNameStatus(out string) map[string]nameStatusEntry {
	m := make(map[string]nameStatusEntry)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		status := parts[0]
		if status == "" {
			continue
		}
		letter := string(status[0])
		switch {
		case len(parts) >= 3 && (letter == "R" || letter == "C"):
			// 双路径行：旧路径可能含 \t？不可能——git 输出以 \t 分隔且路径不含 \t
			m[parts[2]] = nameStatusEntry{Status: letter, OldPath: parts[1]}
		case len(parts) == 2:
			m[parts[1]] = nameStatusEntry{Status: letter}
		default:
			// 畸形行（含引号转义残留等极端形态）跳过，不静默产出错数据
		}
	}
	return m
}

// numstatEntry git diff --numstat 单条目统计。
type numstatEntry struct {
	Additions int  // 新增行数
	Deletions int  // 删除行数
	Binary    bool // 二进制（行数位为 "-"）
}

// parseNumstat 解析 `git diff --numstat` 输出（单趟行数统计，不做源项目 N+1 逐文件调用）：
// 每行 "<add>\t<del>\t<path>"，add/del 为 "-" 表示二进制。
// rename 行路径为 "<old> => <new>" 或含公共前缀的 "<dir/{old => new}>" 形态，
// 解析后在新旧两个路径键下都登记（文件树构建按任一路径命中）。
// 返回 map：路径 → 统计条目。
func parseNumstat(out string) map[string]numstatEntry {
	m := make(map[string]numstatEntry)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		entry := numstatEntry{}
		if parts[0] != "-" {
			if n, err := strconv.Atoi(parts[0]); err == nil {
				entry.Additions = n
			}
		} else {
			entry.Binary = true
		}
		if parts[1] != "-" {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				entry.Deletions = n
			}
		} else {
			entry.Binary = true
		}
		pathPart := strings.Join(parts[2:], "\t")
		for _, p := range expandNumstatRenamePath(pathPart) {
			m[p] = entry
		}
	}
	return m
}

// expandNumstatRenamePath 展开 numstat 的 rename 路径形态为旧/新路径列表：
//   - 普通路径原样返回单元素
//   - "old => new" 直排形态返回 [old, new]
//   - "dir/{oldName => newName}ext" 花括号形态（git 把公共前缀/后缀提到花括号外），
//     返回 [dir/oldName/ext, dir/newName/ext]
func expandNumstatRenamePath(p string) []string {
	open := strings.Index(p, "{")
	sep := strings.Index(p, "=>")
	close := strings.Index(p, "}")
	if open < 0 || close < open || sep < open || sep > close {
		// 无花括号：直排 "old => new" 或普通路径
		if sep >= 0 {
			return []string{
				strings.TrimSpace(p[:sep]),
				strings.TrimSpace(p[sep+2:]),
			}
		}
		return []string{p}
	}
	// 花括号形态：公共前缀在 { 前，公共后缀在 } 后，新旧名在括号内以 => 分隔
	prefix := p[:open]
	inside := p[open+1 : close]
	suffix := p[close+1:]
	sepIdx := strings.Index(inside, "=>")
	if sepIdx < 0 {
		return []string{p}
	}
	old := strings.TrimSpace(inside[:sepIdx])
	new := strings.TrimSpace(inside[sepIdx+2:])
	return []string{prefix + old + suffix, prefix + new + suffix}
}

// branchDiffStatusMap git name-status 字母 → 前端状态常量映射。
var branchDiffStatusMap = map[string]string{
	"A": model.BranchDiffStatusAdded,
	"D": model.BranchDiffStatusDeleted,
	"R": model.BranchDiffStatusRenamed,
	"C": model.BranchDiffStatusCopied,
	"T": model.BranchDiffStatusTypechange,
	"M": model.BranchDiffStatusModified,
}

// GetBranchFileTree 计算两分支差异文件树（R5）：两侧 `ls-tree -r --name-only` 并集
// （含 unchanged）+ `diff --name-status`（R/C 解析新旧双路径）+ 一趟 `diff --numstat`
// 行数统计。不在 diff 集的并集路径 → unchanged；rename 旧路径不在并集则补回；
// 按 Path 排序返回。分支解析失败返回中文错误（区别于对齐服务的容错语义）。
func (s *BranchCompareService) GetBranchFileTree(ctx context.Context, repoPath, branchA, branchB string) ([]model.BranchDiffFile, error) {
	if branchA == "" {
		return nil, fmt.Errorf("分支 A 不能为空")
	}
	if branchB == "" {
		return nil, fmt.Errorf("分支 B 不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	// 两侧 ls-tree 并发（分支解析失败即整体报错：分支名拼写错误须显式暴露）
	var (
		wg           sync.WaitGroup
		treeA, treeB string
		errA, errB   error
	)
	lsTree := func(branch string) (string, error) {
		out, err := s.gitCmd.ExecuteWithContext(ctx, gitRoot,
			append(utf8Prefix(), "ls-tree", "-r", "--name-only", branch)...)
		if err != nil {
			return "", fmt.Errorf("分支 %s 不存在或无法解析: %w", branch, err)
		}
		return out, nil
	}
	wg.Add(2)
	go func() { defer wg.Done(); treeA, errA = lsTree(branchA) }()
	go func() { defer wg.Done(); treeB, errB = lsTree(branchB) }()
	wg.Wait()
	if errA != nil {
		return nil, errA
	}
	if errB != nil {
		return nil, errB
	}

	// name-status 与 numstat 串行两趟（同一次 diff 计算，git 自身缓存极快，无需并发）
	statusOut, err := s.gitCmd.ExecuteWithContext(ctx, gitRoot,
		append(utf8Prefix(), "diff", "--name-status", branchA, branchB)...)
	if err != nil {
		return nil, fmt.Errorf("获取分支差异状态失败: %w", err)
	}
	numstatOut, err := s.gitCmd.ExecuteWithContext(ctx, gitRoot,
		append(utf8Prefix(), "diff", "--numstat", branchA, branchB)...)
	if err != nil {
		return nil, fmt.Errorf("获取分支差异行数统计失败: %w", err)
	}

	diffMap := parseNameStatus(statusOut)
	numstatMap := parseNumstat(numstatOut)

	// 路径并集：两侧 ls-tree + diff 集键 + rename 旧路径补回
	pathSet := make(map[string]bool)
	addLines := func(out string) {
		for _, line := range strings.Split(out, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				pathSet[line] = true
			}
		}
	}
	addLines(treeA)
	addLines(treeB)
	for path, entry := range diffMap {
		pathSet[path] = true
		if entry.OldPath != "" {
			pathSet[entry.OldPath] = true
		}
	}

	files := make([]model.BranchDiffFile, 0, len(pathSet))
	for path := range pathSet {
		file := model.BranchDiffFile{Path: path, Status: model.BranchDiffStatusUnchanged}
		if entry, ok := diffMap[path]; ok {
			if mapped, ok := branchDiffStatusMap[entry.Status]; ok {
				file.Status = mapped
			} else {
				// 未识别状态字母降级为小写字母本身，不静默归为 modified
				file.Status = strings.ToLower(entry.Status)
			}
			file.OldPath = entry.OldPath
		}
		if ns, ok := numstatMap[path]; ok {
			file.Additions = ns.Additions
			file.Deletions = ns.Deletions
			file.Binary = ns.Binary
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// GetBranchFileDiff 获取指定文件在 base→target 两分支间的 unified diff 文本
// （R6：前端双栏/统一视图的原始输入）。返回原文（含 "diff --git" 头），
// 空串表示无差异。
func (s *BranchCompareService) GetBranchFileDiff(ctx context.Context, repoPath, base, target, filePath string) (string, error) {
	if filePath == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}
	out, err := s.gitCmd.ExecuteWithContext(ctx, gitRoot,
		append(utf8Prefix(), "diff", base, target, "--", filePath)...)
	if err != nil {
		return "", fmt.Errorf("获取文件差异失败: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// GetCommitChangedFiles 获取指定提交的变更文件列表（R7 提交详情）：
// `git diff-tree --no-commit-id --name-status -r <sha>`，root commit 等场景该命令
// 失败时回退 `git show --format= --name-status <sha>`。
// 复用本地变更域的 model.FileChange（Path/Status/Staged），Staged 恒 false（历史提交无暂存语义）。
func (s *BranchCompareService) GetCommitChangedFiles(ctx context.Context, repoPath, sha string) ([]model.FileChange, error) {
	if sha == "" {
		return nil, fmt.Errorf("提交 SHA 不能为空")
	}
	gitRoot, err := util.FindGitRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("无法定位 Git 仓库根目录: %w", err)
	}

	out, err := s.gitCmd.ExecuteWithContext(ctx, gitRoot,
		append(utf8Prefix(), "diff-tree", "--no-commit-id", "--name-status", "-r", sha)...)
	if err != nil {
		// 回退路径：root commit（diff-tree 对无 parent 提交输出差异，个别 git 版本
		// 异常）/浅克隆等场景 show 仍可给出 name-status
		out, err = s.gitCmd.ExecuteWithContext(ctx, gitRoot,
			append(utf8Prefix(), "show", "--format=", "--name-status", sha)...)
		if err != nil {
			return nil, fmt.Errorf("获取提交变更文件失败: %w", err)
		}
	}

	statusMap := parseNameStatus(out)
	files := make([]model.FileChange, 0, len(statusMap))
	for path, entry := range statusMap {
		files = append(files, model.FileChange{Path: path, Status: entry.Status, Staged: false})
	}
	// map 遍历序不定：按路径排序保证返回稳定
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
