package service

import (
	"strings"
	"testing"

	"workbench/model"
)

// ===== rename 路径归一（纯函数） =====

// TestNormalizeNumstatPath 覆盖 numstat 路径列的 rename 三形态与普通路径：
// 整路径改名取 new、花括号前缀/中缀改名替换该段、普通路径原样保留。
func TestNormalizeNumstatPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"src/a.go", "src/a.go"},                                    // 普通路径不动
		{"old.go => new.go", "new.go"},                              // 整路径改名
		{"docs/old.md => docs/new.md", "docs/new.md"},               // 整路径改名含目录
		{"{old_dir => new_dir}/x.go", "new_dir/x.go"},               // 前缀改名
		{"a/{sub_old => sub_new}/b.go", "a/sub_new/b.go"},           // 中缀改名
		{"x{aa => bb}y.go", "xbby.go"},                              // 中缀改名无斜杠边界
		{"端/旧 => 端/新.go", "端/新.go"},                                 // 非 ASCII 路径（quotePath=false）
		{"path with space/{o => n}/f.go", "path with space/n/f.go"}, // 含空格目录
		{"{unclosed => oops/x.go", "{unclosed => oops/x.go"},        // 花括号不闭合原样保留
		// 病态混合形态（括号段与整路径箭头并存）：git 单记录不会同时输出两种 rename
		// 形态，口径定为括号段优先且整路径箭头不再二次处理——实现保持简单
		{"a {b => c} d => e/f.go", "a c d => e/f.go"},
	}
	for _, c := range cases {
		if got := normalizeNumstatPath(c.in); got != c.want {
			t.Errorf("normalizeNumstatPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ===== 解析收集 Files 明细 =====

// TestParseCommitNumstatOutput_FilesCollected 验证文件行收集进 Files：
// rename 归一后的新路径、二进制行无明细、普通路径原样、提交级汇总与明细一致。
func TestParseCommitNumstatOutput_FilesCollected(t *testing.T) {
	out := "\x00" + strings.Repeat("c", 40) + "\x01Carol\x01carol@x.com\x011700000200\n" +
		"10\t2\tsrc/a.go\n" +
		"5\t0\t{old => new}/b.go\n" +
		"-\t-\tassets/logo.png\n" +
		"1\t1\told.go => new.go\n"
	records, err := parseCommitNumstatOutput(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("应解析 1 条记录: got %d", len(records))
	}
	r := records[0]
	// 提交级汇总仍只含文本文件（二进制排除）：10+5+1 / 2+0+1
	if r.Insertions != 16 || r.Deletions != 3 {
		t.Errorf("汇总异常: ins=%d del=%d, want 16/3", r.Insertions, r.Deletions)
	}
	if len(r.Files) != 3 {
		t.Fatalf("Files 应 3 条（二进制无明细）: got %d", len(r.Files))
	}
	if r.Files[0].Path != "src/a.go" || r.Files[0].Insertions != 10 || r.Files[0].Deletions != 2 {
		t.Errorf("Files[0] 异常: %+v", r.Files[0])
	}
	// 前缀 rename 归一为 new 前缀
	if r.Files[1].Path != "new/b.go" {
		t.Errorf("Files[1] rename 归一异常: %q, want new/b.go", r.Files[1].Path)
	}
	// 整路径 rename 归一为 new
	if r.Files[2].Path != "new.go" {
		t.Errorf("Files[2] rename 归一异常: %q, want new.go", r.Files[2].Path)
	}
}

// TestDeepCopyLineStats_FilesIsolated 深拷贝后修改拷贝的 Files 不污染原切片
// （缓存 Set/Get 与调用方解耦的前提）。
func TestDeepCopyLineStats_FilesIsolated(t *testing.T) {
	orig := []model.CommitLineStat{{
		SHA: strings.Repeat("d", 40), Author: "Dave", Email: "d@x.com", Timestamp: 1,
		Files: []model.CommitFileLineStat{{Path: "a.go", Insertions: 1, Deletions: 2}},
	}}
	cp := deepCopyLineStats(orig)
	cp[0].Files[0].Path = "mutated.go"
	cp[0].Files[0].Insertions = 999
	cp[0].Files = append(cp[0].Files, model.CommitFileLineStat{Path: "extra.go"})
	if orig[0].Files[0].Path != "a.go" || orig[0].Files[0].Insertions != 1 || len(orig[0].Files) != 1 {
		t.Errorf("深拷贝未隔离 Files: %+v", orig[0].Files)
	}
	// nil Files 保持 nil（零提交明细场景不产生空切片）
	nilCase := deepCopyLineStats([]model.CommitLineStat{{SHA: "x"}})
	if nilCase[0].Files != nil {
		t.Errorf("nil Files 应保持 nil: %+v", nilCase[0].Files)
	}
}

// ===== 目录桶划分 =====

// TestDirBucketOf 目录一级桶：首段为桶、根下散文件归 (根目录)。
func TestDirBucketOf(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"service/a.go", "service"},
		{"a/b/c/d.go", "a"},
		{"README.md", pathLineStatsRootBucket},
		{"中文目录/文件.go", "中文目录"},
	}
	for _, c := range cases {
		if got := dirBucketOf(c.in); got != c.want {
			t.Errorf("dirBucketOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ===== 路径维度聚合 =====

// pathStatsRecords 构造路径聚合测试数据：时间戳 100/200 两窗口边界，
// 文件分布覆盖目录归并、根散文件、跨提交同文件累加。
func pathStatsRecords() []model.CommitLineStat {
	file := func(path string, ins, del int) model.CommitFileLineStat {
		return model.CommitFileLineStat{Path: path, Insertions: ins, Deletions: del}
	}
	return []model.CommitLineStat{
		{SHA: strings.Repeat("1", 40), Timestamp: 100, Files: []model.CommitFileLineStat{
			file("service/a.go", 10, 2), file("web/ui/x.vue", 5, 1), file("README.md", 1, 0),
		}},
		{SHA: strings.Repeat("2", 40), Timestamp: 200, Files: []model.CommitFileLineStat{
			file("service/b.go", 20, 8), file("service/a.go", 3, 3),
		}},
	}
}

// TestAggregateLineStatsByPath_DirRollup 目录一级上卷：跨提交同目录累加、
// 根散文件归 (根目录) 桶、按总量降序。
func TestAggregateLineStatsByPath_DirRollup(t *testing.T) {
	got := AggregateLineStatsByPath(pathStatsRecords(), 0, 0, true, 0)
	if len(got) != 3 {
		t.Fatalf("应 3 个目录桶: %+v", got)
	}
	// service: (10+2)+(20+8)+(3+3)=46；web: 6；(根目录): 1 → 降序 service, web, (根目录)
	if got[0].Path != "service" || got[0].Insertions != 33 || got[0].Deletions != 13 {
		t.Errorf("service 桶异常: %+v", got[0])
	}
	if got[1].Path != "web" || got[1].Insertions != 5 || got[1].Deletions != 1 {
		t.Errorf("web 桶异常: %+v", got[1])
	}
	if got[2].Path != pathLineStatsRootBucket || got[2].Insertions != 1 {
		t.Errorf("根桶异常: %+v", got[2])
	}
}

// TestAggregateLineStatsByPath_FileMode 文件模式：完整路径聚合、Top N 截断、
// 同值按路径升序。
func TestAggregateLineStatsByPath_FileMode(t *testing.T) {
	records := append(pathStatsRecords(), model.CommitLineStat{
		SHA: strings.Repeat("3", 40), Timestamp: 300,
		Files: []model.CommitFileLineStat{
			{Path: "b.go", Insertions: 33, Deletions: 13}, // 与 service 桶总量同值场景单独造
			{Path: "a.go", Insertions: 33, Deletions: 13}, // 同值对：a.go 应排在 b.go 前
		},
	})
	got := AggregateLineStatsByPath(records, 0, 0, false, 2)
	if len(got) != 2 {
		t.Fatalf("Top N 应截断为 2: got %d", len(got))
	}
	// 同值(46)两文件按路径升序 a.go < b.go
	if got[0].Path != "a.go" || got[1].Path != "b.go" {
		t.Errorf("同值排序异常: %+v", got)
	}
	if got[0].Insertions != 33 || got[0].Deletions != 13 {
		t.Errorf("聚合值异常: %+v", got[0])
	}
}

// TestAggregateLineStatsByPath_WindowFilter 时间窗口过滤与贡献者行数口径一致
// （sinceTs 含下界、untilTs 含上界端点）。
func TestAggregateLineStatsByPath_WindowFilter(t *testing.T) {
	// since=200：排除 ts=100 提交，只剩 service/b.go(20,8) + service/a.go(3,3)
	got := AggregateLineStatsByPath(pathStatsRecords(), 200, 0, true, 0)
	if len(got) != 1 || got[0].Path != "service" || got[0].Insertions != 23 {
		t.Errorf("since 过滤异常: %+v", got)
	}
	// until=100：只剩 ts=100 提交（service 桶=a.go(10,2)；README 在根桶）
	got = AggregateLineStatsByPath(pathStatsRecords(), 0, 100, true, 0)
	if len(got) != 3 || got[0].Insertions != 10 {
		t.Errorf("until 过滤异常: %+v", got)
	}
	// 空窗口（since > 全部）返空切片非 nil
	got = AggregateLineStatsByPath(pathStatsRecords(), 9999, 0, true, 0)
	if got == nil || len(got) != 0 {
		t.Errorf("空窗口应返空切片非 nil: %+v", got)
	}
	// 无 Files 明细的记录（二进制-only 提交）不产生桶
	binOnly := []model.CommitLineStat{{SHA: strings.Repeat("4", 40), Timestamp: 100}}
	if got := AggregateLineStatsByPath(binOnly, 0, 0, true, 0); len(got) != 0 {
		t.Errorf("无明细记录不应产生桶: %+v", got)
	}
}
