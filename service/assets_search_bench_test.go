package service

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"workbench/model"
)

// 素材检索基准（PRD 验收「检索 P95：10 万幻灯片行 < 1s」的守护测试）。
//
// 语料规格对齐 research/sqlite-fts5-chinese.md 的 10 万行实测：正文随机汉字
// ~120 字/页，约 2% 页植入「城商行」（正文命中）、5% 页植入「获客」（标题命中），
// 确定性随机源（rand.NewSource(42)）保证跨次构造可复现。
//
// 语料构造绕过 pptx 解析直接 SQL 批量写主表 + FTS 双写（10 万页解析量级不可
// 接受），FTS 文本经 splitFtsText 切分——与生产写路径同源（research 坑 1）。
//
// 两档规模（对齐 perf-baseline 的 benchmark 惯例）：
//   - BenchmarkAssetSearch：默认 10 万页（手动 `go test -bench=BenchmarkAssetSearch
//     -run='^$' ./service/` 采集），-short 缩减为 1 万页；
//   - TestAssetSearch_PerfBudget：CI 常规守护档 1 万页（构造秒级），热缓存 30 次
//     采样 p95 < 1s 断言——防 FTS 索引失效退化为全表扫描类灾难回归（1 万页正常
//     p95 在 10ms 量级，1s 断言是宽松兜底），-short 跳过。

// benchHanChars 基准语料汉字池（高频常用字，覆盖检索命中语境）。
const benchHanChars = "数据平台渠道银行服务客户经营管理分析报告系统架构设计开发测试部署运维监控日志安全网络存储计算虚拟化容器编排调度消息队列缓存数据库中间件微服务治理注册配置中心网关路由负载均衡熔断降级限流超时重试幂等事务一致性可用分区容错协同办公营销活动产品介绍解决方案实践总结规划"

// assetSearchCorpusScale 基准语料规模：默认 1000 文件 × 100 页 = 10 万页；
// -short 缩减为 100 × 100 = 1 万页。
func assetSearchCorpusScale(short bool) (fileCount, slidesPerFile int) {
	if short {
		return 100, 100
	}
	return 1000, 100
}

// benchRandomText 确定性随机汉字串（汉字池 rune 切片包级缓存，避免每次取样转换）。
var benchHanRunes = []rune(benchHanChars)

func benchRandomText(rng *rand.Rand, n int) string {
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = benchHanRunes[rng.Intn(len(benchHanRunes))]
	}
	return string(runes)
}

// buildAssetSearchCorpus 程序化构造检索语料（单事务批量写）：fileCount 个文件
// 各 slidesPerFile 页；约 2% 页正文植入「城商行」、约 5% 页标题植入「获客」，
// FTS 文本经 splitFtsText 同源切分。
func buildAssetSearchCorpus(tb testing.TB, store *assetsStore, fileCount, slidesPerFile int) {
	tb.Helper()
	tx, err := store.db.Begin()
	if err != nil {
		tb.Fatalf("开启语料事务: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	insFile, err := tx.Prepare(
		"INSERT INTO files(path, size, mtime_ns, status, title, note) VALUES(?, 1024, 1, ?, ?, '')")
	if err != nil {
		tb.Fatalf("准备文件语句: %v", err)
	}
	defer insFile.Close()
	insFtsFile, err := tx.Prepare(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) " +
			"VALUES(?, 'file', ?, ?, ?, '', '', '', '')")
	if err != nil {
		tb.Fatalf("准备文件 FTS 语句: %v", err)
	}
	defer insFtsFile.Close()
	insSlide, err := tx.Prepare(
		"INSERT INTO slides(file_id, slide_num, title, text, speaker_notes) VALUES(?, ?, ?, ?, '')")
	if err != nil {
		tb.Fatalf("准备幻灯片语句: %v", err)
	}
	defer insSlide.Close()
	insFtsSlide, err := tx.Prepare(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) " +
			"VALUES(?, 'slide', ?, ?, '', '', ?, ?, '')")
	if err != nil {
		tb.Fatalf("准备幻灯片 FTS 语句: %v", err)
	}
	defer insFtsSlide.Close()

	rng := rand.New(rand.NewSource(42)) // 确定性语料（research 同款固定源）
	for f := 0; f < fileCount; f++ {
		// 文件名：约 5% 植入「获客」（文件名列命中形态）
		fileName := fmt.Sprintf("材料%05d", f)
		if f%20 == 0 {
			fileName = "获客" + fileName
		}
		res, err := insFile.Exec(fmt.Sprintf("目录%03d/%s.pptx", f/100, fileName),
			model.AssetFileStatusOK, fileName)
		if err != nil {
			tb.Fatalf("写语料文件: %v", err)
		}
		fileID, err := res.LastInsertId()
		if err != nil {
			tb.Fatalf("取文件 ID: %v", err)
		}
		if _, err := insFtsFile.Exec(fileID, fileID, fileID, splitFtsText(fileName)); err != nil {
			tb.Fatalf("写语料文件 FTS: %v", err)
		}
		for p := 1; p <= slidesPerFile; p++ {
			seq := f*slidesPerFile + p
			// 正文：120 随机汉字，约 2% 页植入「城商行」
			text := benchRandomText(rng, 120)
			if seq%50 == 0 {
				text = text[:60] + "城商行" + text[60:]
			}
			// 标题：模板 + 约 5% 页植入「获客」
			title := fmt.Sprintf("%s 第%03d页", fileName, p)
			if seq%20 == 0 {
				title = "获客" + title
			}
			res, err := insSlide.Exec(fileID, p, title, text)
			if err != nil {
				tb.Fatalf("写语料幻灯片: %v", err)
			}
			slideID, err := res.LastInsertId()
			if err != nil {
				tb.Fatalf("取幻灯片 ID: %v", err)
			}
			if _, err := insFtsSlide.Exec(-slideID, slideID, fileID,
				splitFtsText(title), splitFtsText(text)); err != nil {
				tb.Fatalf("写语料幻灯片 FTS: %v", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatalf("提交语料事务: %v", err)
	}
}

// newAssetSearchBenchService 构造带语料的检索基准服务（库走真实建库迁移链路，
// 语料直插 SQL；构造耗时由调用方 StopTimer/ResetTimer 排除计时）。
func newAssetSearchBenchService(tb testing.TB, fileCount, slidesPerFile int) (*AssetLibraryService, string) {
	tb.Helper()
	svc := NewAssetLibraryService(context.Background(), filepath.Join(tb.TempDir(), "assets"))
	tb.Cleanup(svc.CloseAll)
	lib := mustCreateLibrary(tb, svc, "检索基准库", tb.TempDir())
	store := svc.openStoreForTest(tb, lib.ID)
	buildAssetSearchCorpus(tb, store, fileCount, slidesPerFile)
	return svc, lib.ID
}

// BenchmarkAssetSearch 检索链路基准（FTS MATCH + bm25 排序 + 聚合折叠 + 分页 +
// 元数据补全 + 摘要，service 层全链路）：默认 10 万页语料，-short 缩减 1 万页。
func BenchmarkAssetSearch(b *testing.B) {
	fileCount, slidesPerFile := assetSearchCorpusScale(testing.Short())
	svc, libID := newAssetSearchBenchService(b, fileCount, slidesPerFile)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 50})
		if err != nil {
			b.Fatalf("检索: %v", err)
		}
		if page.Total == 0 {
			b.Fatal("基准查询零命中（语料或索引异常）")
		}
	}
}

// TestAssetSearch_PerfBudget CI 性能守护：1 万页语料（默认档构造秒级），热缓存
// 30 次采样 p95 < 1s（PRD 验收口径的守护档；10 万页精确数字由
// BenchmarkAssetSearch 手动采集）。-short 跳过（快速档不含性能守护）。
func TestAssetSearch_PerfBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过性能守护（由 BenchmarkAssetSearch 手动采集完整规格）")
	}
	svc, libID := newAssetSearchBenchService(t, 100, 100)

	// 预热 1 次（排除首次查询的惰性初始化：连接池/页缓存）
	if _, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 50}); err != nil {
		t.Fatalf("预热检索: %v", err)
	}

	const samples = 30
	durations := make([]float64, 0, samples)
	var lastTotal int
	for i := 0; i < samples; i++ {
		start := time.Now()
		page, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 50})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("采样检索: %v", err)
		}
		if page.Total == 0 {
			t.Fatal("基准查询零命中（语料或索引异常）")
		}
		lastTotal = page.Total
		durations = append(durations, float64(elapsed))
	}
	sort.Float64s(durations)
	p95 := time.Duration(durations[int(0.95*float64(len(durations)-1))])
	p50 := time.Duration(durations[len(durations)/2])
	t.Logf("1 万页语料热缓存 %d 次采样：p50=%v p95=%v（命中 %d 条目）", samples, p50, p95, lastTotal)
	if p95 > time.Second {
		t.Errorf("检索 p95 = %v，超出 1s 预算（PRD 验收口径）", p95)
	}
}
