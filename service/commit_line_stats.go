package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// 贡献者行数统计通道（`git log --numstat` 按作者聚合新增/删除行数）。
//
// 与提交历史通道（commit_history_cache + commit_history_cli.go）平行：
//   - 数据通道独立：numstat 输出含每提交的新增/删除行数，提交历史缓存的全量快照
//     （model.Commit）不含行数字段，不扩展其格式（提交列表页零影响）。
//   - 缓存二态：行数是聚合值无增量 prepend 路径，失效判定比提交历史缓存更简单——
//     headSHA 相同 + TTL 内命中，否则全量重拉。
//   - 采样口径对齐：拉取受 CommitHistoryMaxEntries（5000）条上限约束，与提交历史
//     超限采样口径一致；Sampled 标记由 app 层取两路 overflow 之或。
//
// 输出格式契约：
//
//	git -c core.quotePath=false log --numstat --format="%x00%H%x01%an%x01%ae%x01%at" -n <count>
//
//	记录分隔 = 0x00。每记录首行 = 头部 4 字段（SHA/作者/邮箱/作者 unix 秒，0x01 分隔），
//	其后每行 = 文件统计行 `<ins>\t<del>\t<path>`。
//	  - ins/del 为 "-" 表示二进制文件，该行不计入行数。
//	  - merge commit 缺省无文件统计行，自然计 0 行（merge 不引入自身行变更，与主流
//	    统计口径一致）；不加 --diff-merges。
//	  - rename 路径形如 `old => new` / `{a => b}/x.go` 仅影响路径列，本通道只取数值列。
//	  - core.quotePath=false 输出原始非 ASCII 路径（与 commit_history_cli.go 一致；
//	    路径列本通道不消费，仅为格式完整性保留该参数）。
//	  - %at = author unix 秒，与提交历史通道 Timestamp 字段同源，窗口过滤口径一致。

// commitLineStatsCacheTTL 行数统计缓存 TTL，与 commitHistoryCacheTTL 同取 5min：
// HEAD SHA 已覆盖 commit/pull 失效，TTL 兜底 HEAD ref 未变但历史被外部改写的极端场景。
const commitLineStatsCacheTTL = 5 * time.Minute

// commitNumstatFormat numstat 通道 git log --format 格式串，字段序见文件头契约。
const commitNumstatFormat = "%x00%H%x01%an%x01%ae%x01%at"

// commitNumstatArgs 构造 numstat 通道 git log 参数。count>0 时加 -n 上限。
func commitNumstatArgs(count int) []string {
	args := []string{
		"-c", "core.quotePath=false",
		"log",
		"--numstat",
		"--format=" + commitNumstatFormat,
	}
	if count > 0 {
		args = append(args, "-n", strconv.Itoa(count))
	}
	return args
}

// FetchCommitLineStats 拉取并解析最近 count 条提交的 numstat 行数记录。
// count 取 CommitHistoryMaxEntries（5000），与提交历史采样口径对齐。
// git 失败（库损坏/超时等）向上透传错误（util.GitCommand 错误含 args 与 stderr 摘要），
// 由调用方决定降级或报错；本函数不静默吞错。
func FetchCommitLineStats(gitRoot string) ([]model.CommitLineStat, error) {
	out, err := util.NewGitCommand().Execute(gitRoot, commitNumstatArgs(CommitHistoryMaxEntries)...)
	if err != nil {
		return nil, err
	}
	return parseCommitNumstatOutput(out)
}

// parseCommitNumstatOutput 解析 numstat 通道 git log 完整输出（\x00 记录分隔）。
// 空输入（空仓库无提交）返空切片非 nil，调用方无需判 nil。
func parseCommitNumstatOutput(out string) ([]model.CommitLineStat, error) {
	chunks := strings.Split(out, "\x00")
	records := make([]model.CommitLineStat, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.Trim(chunk, "\x00")
		if chunk == "" {
			continue // 首记录前的空块
		}
		r, err := parseCommitNumstatChunk(chunk)
		if err != nil {
			return nil, err
		}
		records = append(records, *r)
	}
	return records, nil
}

// parseCommitNumstatChunk 解析单条 numstat 记录块：首行为提交头（4 字段 0x01 分隔），
// 其余非空行为文件统计行。解析原则与 commit_history_cli.go 一致——头部强校验快速
// 失败，不静默产出错数据（作者名/邮箱含 0x01 致字段错位时，SHA/ts 校验几乎必然
// 不满足）。文件行要求「新增列为数字或 -」，不满足视为格式异常报错。
func parseCommitNumstatChunk(chunk string) (*model.CommitLineStat, error) {
	lines := strings.Split(chunk, "\n")

	// 头行：SHA/author/email/ts 共 4 字段
	header := strings.SplitN(lines[0], "\x01", 4)
	if len(header) != 4 {
		return nil, fmt.Errorf("numstat 记录头部字段不完整: %q", truncateNumstatForLog(lines[0]))
	}
	if !isHexSHA40(header[0]) {
		return nil, fmt.Errorf("numstat 记录 SHA 非法（字段可能错位）: %q", truncateNumstatForLog(header[0]))
	}
	ts, err := strconv.ParseInt(header[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("numstat 记录时间戳解析失败: %q", header[3])
	}

	r := &model.CommitLineStat{
		SHA:       header[0],
		Author:    header[1],
		Email:     header[2],
		Timestamp: ts,
	}

	// 文件统计行：`<ins>\t<del>\t<path>`；ins/del 为 "-"（二进制）跳过该行数值
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("numstat 文件行格式异常: %q", truncateNumstatForLog(line))
		}
		if parts[0] == "-" || parts[1] == "-" {
			continue // 二进制文件行不计入行数
		}
		ins, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("numstat 新增行数解析失败: %q", truncateNumstatForLog(line))
		}
		del, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("numstat 删除行数解析失败: %q", truncateNumstatForLog(line))
		}
		r.Insertions += ins
		r.Deletions += del
	}
	return r, nil
}

// isHexSHA40 判定 40 位小写 hex SHA-1（numstat 头部强校验；与 main 包 isHexSHA
// 语义一致，跨包不复用故本地实现）。
func isHexSHA40(s string) bool {
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

// truncateNumstatForLog 解析失败信息截断，防超长记录撑爆错误日志。
func truncateNumstatForLog(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

// CommitLineStatsCache numstat 行数统计缓存（纯内存，不落盘）。
//
// 缓存粒度与键同提交历史缓存：仓库根 + HEAD ref 复合键（调用方 commitHistoryCacheKey
// 构造传入），存全量 []model.CommitLineStat（按提交粒度，非聚合值——时间窗口过滤在
// 内存聚合时做，切档位不重扫）。
//
// 失效策略（二态，比提交历史缓存简单——行数是聚合值无增量 prepend 路径）：
//   - key 存在 + headSHA 相同 + TTL 内 -> 命中，返深拷贝
//   - key 不存在 / headSHA 不同（commit/pull 后 HEAD 前移）/ TTL 过期 -> miss，
//     调用方全量重拉后 Set 覆盖
//
// 并发安全：mu 互斥锁在 Get/Set/ClearByGitRoot/ClearAll 整次操作期间持有，
// 同 CommitHistoryCache 策略。Set 存深拷贝、Get 返深拷贝，调用方修改返回切片
// （窗口过滤产生新切片，理论上不修改入参）不污染缓存。
type CommitLineStatsCache struct {
	mu      sync.Mutex
	entries map[string]commitLineStatsCacheEntry
}

// commitLineStatsCacheEntry 单个仓库 + HEAD ref 的行数统计缓存快照。
type commitLineStatsCacheEntry struct {
	headSHA  string
	records  []model.CommitLineStat
	cachedAt time.Time
}

// NewCommitLineStatsCache 创建行数统计缓存。
func NewCommitLineStatsCache() *CommitLineStatsCache {
	return &CommitLineStatsCache{entries: make(map[string]commitLineStatsCacheEntry)}
}

// Get 取缓存。仅当 key 存在、headSHA 与当前一致且 TTL 内时命中（found=true），
// 返条目深拷贝；headSHA 不同直接 miss（无增量路径，调用方全量重拉）；
// TTL 过期驱逐条目返 found=false。
func (c *CommitLineStatsCache) Get(key, headSHA string) ([]model.CommitLineStat, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	// TTL 过期 -> 兜底 HEAD 未变但历史被外部改写的极端场景；驱逐条目防内存无界增长
	if time.Since(entry.cachedAt) > commitLineStatsCacheTTL {
		delete(c.entries, key)
		return nil, false
	}
	if entry.headSHA != headSHA {
		return nil, false
	}
	return deepCopyLineStats(entry.records), true
}

// Set 回写缓存。存储深拷贝与调用方切片解耦。条数超 CommitHistoryMaxEntries 拒绝写
// （防御性：numstat 拉取已受 -n 上限约束，正常不触达）。
func (c *CommitLineStatsCache) Set(key, headSHA string, records []model.CommitLineStat) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(records) > CommitHistoryMaxEntries {
		return
	}
	c.entries[key] = commitLineStatsCacheEntry{
		headSHA:  headSHA,
		records:  deepCopyLineStats(records),
		cachedAt: time.Now(),
	}
}

// ClearByGitRoot 清除指定仓库根路径下的全部缓存条目，供 InvalidateCommitHistoryCache
// 手动强刷时同步清行数缓存（前端 handleRefresh 后两路都全量重扫）。
func (c *CommitLineStatsCache) ClearByGitRoot(gitRoot string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := gitRoot + "|"
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
}

// ClearAll 清除全部缓存，供 ClearAllCommitHistoryCache 全量刷新（预留入口）。
func (c *CommitLineStatsCache) ClearAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]commitLineStatsCacheEntry)
}

// deepCopyLineStats 深拷贝行数记录切片。model.CommitLineStat 为纯值 struct（无切片
// 字段），slice 复制即完成深拷贝——元素为值拷贝，修改拷贝切片元素不影响原切片。
func deepCopyLineStats(in []model.CommitLineStat) []model.CommitLineStat {
	if in == nil {
		return nil
	}
	out := make([]model.CommitLineStat, len(in))
	copy(out, in)
	return out
}

// AggregateLineStats 聚合行数记录为贡献者行数排名（纯函数）。
// 窗口过滤与 filterCommitsByTime 同逻辑（sinceTs=0 不限下界、untilTs>0 含上界端点），
// 按 Author+Email 归一分组求和（同名不同邮箱视为不同贡献者，与 git log 作者身份
// 一致），按 Insertions+Deletions 降序、同值按作者名升序（与 aggregateContributors
// 排序规则一致）。返回的 Contributor 仅填充 Author/Email/Insertions/Deletions
// （Count=0），提交数由提交历史通道聚合后在 app 层合并。
func AggregateLineStats(records []model.CommitLineStat, sinceTs, untilTs int64) []model.Contributor {
	type key struct {
		author, email string
	}
	agg := make(map[key]*model.Contributor)
	for _, r := range records {
		if sinceTs > 0 && r.Timestamp < sinceTs {
			continue
		}
		if untilTs > 0 && r.Timestamp > untilTs {
			continue
		}
		k := key{r.Author, r.Email}
		c, ok := agg[k]
		if !ok {
			c = &model.Contributor{Author: r.Author, Email: r.Email}
			agg[k] = c
		}
		c.Insertions += r.Insertions
		c.Deletions += r.Deletions
	}

	out := make([]model.Contributor, 0, len(agg))
	for _, c := range agg {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := out[i].Insertions+out[i].Deletions, out[j].Insertions+out[j].Deletions
		if ti != tj {
			return ti > tj
		}
		return out[i].Author < out[j].Author
	})
	return out
}

// MergeContributorLineStats 将行数统计按 Author+Email 合并进贡献者列表（就地填充
// Insertions/Deletions），行数通道无对应贡献者的条目保持 0（提交窗口与行数窗口
// 口径一致时不应出现，防御性保留）。返回入参切片便于链式书写；排序沿用 contributors
// 原序（提交数降序），行数维度排序展示由前端按 Insertions+Deletions 计算。
func MergeContributorLineStats(contributors []model.Contributor, lineStats []model.Contributor) []model.Contributor {
	type key struct {
		author, email string
	}
	m := make(map[key]model.Contributor, len(lineStats))
	for _, ls := range lineStats {
		m[key{ls.Author, ls.Email}] = ls
	}
	for i := range contributors {
		if ls, ok := m[key{contributors[i].Author, contributors[i].Email}]; ok {
			contributors[i].Insertions = ls.Insertions
			contributors[i].Deletions = ls.Deletions
		}
	}
	return contributors
}
