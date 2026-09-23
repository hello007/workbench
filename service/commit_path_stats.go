package service

import (
	"sort"
	"strings"

	"workbench/model"
)

// 路径维度行数分布聚合（numstat 通道第三消费方）：把 CommitLineStat.Files 明细
// 按目录一级上卷或完整文件路径归并，供统计页「行数分布」图的目录/文件两个维度。
// 时间窗口过滤口径与 AggregateLineStats 一致（sinceTs=0 不限下界、untilTs>0 含上界
// 端点）；二进制文件行在解析层已排除（Files 明细不含）。

// pathLineStatsRootBucket 仓库根下散文件（路径无目录段）在目录维度归入的桶标签。
const pathLineStatsRootBucket = "(根目录)"

// TopFileLineStatsLimit 文件维度 Top N 截断上限，与前端 buildContributorLineOption
// 默认 limit=15 对齐（后端截断减少 Wails 序列化与深拷贝开销，前端不再二次截断）。
const TopFileLineStatsLimit = 15

// AggregateLineStatsByPath 聚合行数记录为路径维度行数分布（纯函数）。
// byDir=true 按目录一级上卷（路径首段为桶，根下散文件归入 pathLineStatsRootBucket）；
// byDir=false 按完整文件路径聚合。topN>0 时按总量降序截断前 N 条（文件维度用
// topFileLineStatsLimit；目录维度传 0 不截断）；0/负值不截断。
// 排序规则：Insertions+Deletions 降序，同值按 Path 升序（与贡献者排名排序精神一致）。
func AggregateLineStatsByPath(records []model.CommitLineStat, sinceTs, untilTs int64, byDir bool, topN int) []model.PathLineStat {
	agg := make(map[string]*model.PathLineStat)
	for _, r := range records {
		if sinceTs > 0 && r.Timestamp < sinceTs {
			continue
		}
		if untilTs > 0 && r.Timestamp > untilTs {
			continue
		}
		for _, f := range r.Files {
			key := f.Path
			if byDir {
				key = dirBucketOf(f.Path)
			}
			p, ok := agg[key]
			if !ok {
				p = &model.PathLineStat{Path: key}
				agg[key] = p
			}
			p.Insertions += f.Insertions
			p.Deletions += f.Deletions
		}
	}

	out := make([]model.PathLineStat, 0, len(agg))
	for _, p := range agg {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := out[i].Insertions+out[i].Deletions, out[j].Insertions+out[j].Deletions
		if ti != tj {
			return ti > tj
		}
		return out[i].Path < out[j].Path
	})
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

// dirBucketOf 取路径的目录一级桶：首段为桶名；无目录段（仓库根下散文件）归入
// pathLineStatsRootBucket。git numstat 路径恒为正斜杠分隔（与运行平台无关）。
func dirBucketOf(path string) string {
	if i := strings.IndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return pathLineStatsRootBucket
}
