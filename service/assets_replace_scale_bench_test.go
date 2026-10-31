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

// 素材拆分 replace 落点的索引规模基线（perf-baseline §19 复测口径）。
//
// 测量对象：一次全量 replace 前后同一库的索引形态差异——
//   - before：53 文件 / 1208 页（用户真实素材库规模，来源 10-30-hd 任务基线）；
//   - after：1208 文件 / 1208 页（每页一条 files 行 + 一条 slides 行 + FTS 行）。
//
// 三项指标按 §19 登记口径：files 行数、ListFiles（前端 ListAssetFiles 全量
// 拉取）耗时、SearchAssets 典型词耗时；附带统计 GetLibraryStats 耗时。
//
// 语料构造绕过 pptx 解析直接批量写主表 + FTS 双写（1208 页解析量级不可接受），
// FTS 文本经 splitFtsText 切分——与生产写路径同源；命中语料注入规则与
// assets_search_bench_test.go 一致（约 2% 页正文植入「城商行」、约 5% 页标题
// 植入「获客」），两档命中页数相同，保证检索结果可比。

// replaceScaleState 一档索引形态：名称 + 每文件页数分布。
type replaceScaleState struct {
	name     string
	pagesPer []int // 每个文件的页数（长度 = files 行数）
}

// replaceScaleStates 两档形态：before 为真实库分布（1208 页摊到 53 文件），
// after 为全量 replace 后（每页一个单页文件）。
func replaceScaleStates() []replaceScaleState {
	const totalPages = 1208
	const beforeFiles = 53

	pages := make([]int, beforeFiles)
	remaining := totalPages
	for i := range pages {
		// 余量摊到前置文件，整体合计严格等于 totalPages
		filesLeft := beforeFiles - i
		pages[i] = remaining / filesLeft
		remaining -= pages[i]
	}

	afterPages := make([]int, totalPages)
	for i := range afterPages {
		afterPages[i] = 1
	}

	return []replaceScaleState{
		{name: "before_53files_1208slides", pagesPer: pages},
		{name: "after_1208files_1slide", pagesPer: afterPages},
	}
}

// buildReplaceScaleCorpus 程序化构造一档索引形态（单事务批量写）。
// 路径形态贴合实际：before 为源文件本身，after 为 replace 产物目录下的单页文件
// （<源主干>/<源主干>-NNN.pptx）。
func buildReplaceScaleCorpus(tb testing.TB, store *assetsStore, state replaceScaleState) {
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

	rng := rand.New(rand.NewSource(42)) // 确定性语料（与检索基准同源）
	seq := 0
	for f, pages := range state.pagesPer {
		// 路径：before 摊成单层源文件；after 每源目录下放单页产物
		var relPath, fileName string
		if pages == 1 {
			relPath = fmt.Sprintf("素材源/源%04d/源%04d-001.pptx", f, f)
			fileName = fmt.Sprintf("源%04d-001", f)
		} else {
			relPath = fmt.Sprintf("素材源/源%03d.pptx", f)
			fileName = fmt.Sprintf("源%03d", f)
		}
		res, err := insFile.Exec(relPath, model.AssetFileStatusOK, fileName)
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
		for p := 1; p <= pages; p++ {
			seq++
			text := benchRandomText(rng, 120)
			if seq%50 == 0 {
				text = text[:60] + "城商行" + text[60:]
			}
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

// newReplaceScaleService 构造一档形态的素材库服务（库走真实建库迁移链路，
// 语料直插 SQL；构造耗时由调用方排除计时）。
func newReplaceScaleService(tb testing.TB, state replaceScaleState) (*AssetLibraryService, string, *assetsStore) {
	tb.Helper()
	svc := NewAssetLibraryService(context.Background(), filepath.Join(tb.TempDir(), "assets"))
	tb.Cleanup(svc.CloseAll)
	lib := mustCreateLibrary(tb, svc, state.name, tb.TempDir())
	store := svc.openStoreForTest(tb, lib.ID)
	buildReplaceScaleCorpus(tb, store, state)
	return svc, lib.ID, store
}

// replaceScaleCounts 索引行数三口径（files / slides / FTS）。
func replaceScaleCounts(tb testing.TB, store *assetsStore) (files, slides, fts int) {
	tb.Helper()
	queries := []struct {
		sql  string
		dest *int
	}{
		{"SELECT COUNT(*) FROM files", &files},
		{"SELECT COUNT(*) FROM slides", &slides},
		{"SELECT COUNT(*) FROM search_fts", &fts},
	}
	for _, q := range queries {
		if err := store.db.QueryRow(q.sql).Scan(q.dest); err != nil {
			tb.Fatalf("统计 %q: %v", q.sql, err)
		}
	}
	return files, slides, fts
}

// BenchmarkAssetReplaceScale replace 前后两档形态的读取链路基准（files 行数由
// TestAssetReplaceScale_Report 输出；本基准只测耗时不测计数）。
func BenchmarkAssetReplaceScale(b *testing.B) {
	for _, state := range replaceScaleStates() {
		svc, libID, _ := newReplaceScaleService(b, state)
		b.Run(state.name+"/ListFiles", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				entries, err := svc.ListFiles(libID)
				if err != nil {
					b.Fatalf("ListFiles: %v", err)
				}
				if len(entries) != len(state.pagesPer) {
					b.Fatalf("ListFiles 条目数 %d，期望 %d", len(entries), len(state.pagesPer))
				}
			}
		})
		b.Run(state.name+"/SearchAssets_正文词", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				page, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 50})
				if err != nil {
					b.Fatalf("SearchAssets: %v", err)
				}
				if page.Total == 0 {
					b.Fatal("基准查询零命中（语料或索引异常）")
				}
			}
		})
		b.Run(state.name+"/SearchAssets_标题词", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				page, err := svc.SearchAssets(libID, "获客", model.AssetSearchOptions{Flatten: false, Limit: 50})
				if err != nil {
					b.Fatalf("SearchAssets: %v", err)
				}
				if page.Total == 0 {
					b.Fatal("基准查询零命中（语料或索引异常）")
				}
			}
		})
		b.Run(state.name+"/LibraryStats", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				stats, err := svc.GetLibraryStats(libID)
				if err != nil {
					b.Fatalf("GetLibraryStats: %v", err)
				}
				if stats.FileCount != len(state.pagesPer) {
					b.Fatalf("统计文件数 %d，期望 %d", stats.FileCount, len(state.pagesPer))
				}
			}
		})
	}
}

// replaceScaleSamples 每轮采样次数（3 轮中位取中位数的中位数，抵噪声）。
const replaceScaleSamples = 20

// replaceScaleSampleWindow 单次采样累计窗口：逐次计时会被 Windows 时钟粒度
// 量化成 1ms 台阶（出现 0s / 999.9µs 这类失真值），故每次采样在窗口内跑多轮
// 取平均单次耗时。
const replaceScaleSampleWindow = 10 * time.Millisecond

// medianDuration 取样本中位耗时。
func medianDuration(durations []time.Duration) time.Duration {
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

// measureReplaceScaleRounds 单操作跑 3 轮，每轮 replaceScaleSamples 次采样，
// 每次采样在 replaceScaleSampleWindow 窗口内重复执行取平均单次耗时，最后取
// 3 轮中位（§19 复测口径的交付数字）。
func measureReplaceScaleRounds(op func() error) (time.Duration, error) {
	rounds := make([]time.Duration, 0, 3)
	for round := 0; round < 3; round++ {
		samples := make([]time.Duration, 0, replaceScaleSamples)
		for i := 0; i < replaceScaleSamples; i++ {
			start := time.Now()
			ops := 0
			for time.Since(start) < replaceScaleSampleWindow {
				if err := op(); err != nil {
					return 0, err
				}
				ops++
			}
			samples = append(samples, time.Since(start)/time.Duration(ops))
		}
		rounds = append(rounds, medianDuration(samples))
	}
	return medianDuration(rounds), nil
}

// TestAssetReplaceScale_Report 输出 §19 复测三项指标的两档对照表（3 轮中位）。
// 慢档构造约秒级到十秒级，-short 跳过；数字回填 docs/spec/perf-baseline.md §19。
func TestAssetReplaceScale_Report(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过 replace 索引规模基线报告（由 BenchmarkAssetReplaceScale 手动采集）")
	}
	type reportRow struct {
		state  string
		files  int
		slides int
		fts    int
		list   time.Duration
		body   time.Duration
		title  time.Duration
		stats  time.Duration
	}
	rows := make([]reportRow, 0, 2)

	for _, state := range replaceScaleStates() {
		svc, libID, store := newReplaceScaleService(t, state)
		files, slides, fts := replaceScaleCounts(t, store)

		// 预热一次（排除首次查询的惰性初始化：连接池/页缓存）
		if _, err := svc.ListFiles(libID); err != nil {
			t.Fatalf("%s ListFiles 预热: %v", state.name, err)
		}
		if _, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 50}); err != nil {
			t.Fatalf("%s SearchAssets 预热: %v", state.name, err)
		}

		listDur, err := measureReplaceScaleRounds(func() error {
			_, err := svc.ListFiles(libID)
			return err
		})
		if err != nil {
			t.Fatalf("%s ListFiles: %v", state.name, err)
		}
		bodyDur, err := measureReplaceScaleRounds(func() error {
			page, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 50})
			if err == nil && page.Total == 0 {
				return fmt.Errorf("零命中")
			}
			return err
		})
		if err != nil {
			t.Fatalf("%s SearchAssets(正文词): %v", state.name, err)
		}
		titleDur, err := measureReplaceScaleRounds(func() error {
			page, err := svc.SearchAssets(libID, "获客", model.AssetSearchOptions{Flatten: false, Limit: 50})
			if err == nil && page.Total == 0 {
				return fmt.Errorf("零命中")
			}
			return err
		})
		if err != nil {
			t.Fatalf("%s SearchAssets(标题词): %v", state.name, err)
		}
		statsDur, err := measureReplaceScaleRounds(func() error {
			_, err := svc.GetLibraryStats(libID)
			return err
		})
		if err != nil {
			t.Fatalf("%s GetLibraryStats: %v", state.name, err)
		}

		rows = append(rows, reportRow{
			state: state.name, files: files, slides: slides, fts: fts,
			list: listDur, body: bodyDur, title: titleDur, stats: statsDur,
		})
	}

	t.Log("replace 索引规模基线（3 轮中位）：形态 | files | slides | FTS | ListFiles | 检索(正文词) | 检索(标题词) | 统计")
	for _, r := range rows {
		t.Logf("%s | %d | %d | %d | %v | %v | %v | %v",
			r.state, r.files, r.slides, r.fts, r.list, r.body, r.title, r.stats)
	}
}
