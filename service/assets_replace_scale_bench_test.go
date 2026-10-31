package service

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"workbench/model"
)

// 素材拆分 replace 落点的索引规模基线（perf-baseline §19 复测口径）。
//
// 测量对象：一次全量 replace 前后同一库的索引形态差异——
//   - before：53 源文件 / 1208 页（用户真实素材库规模，来源 10-30-hd 任务基线）；
//   - after：1208 个单页产物 / 1208 页（每页一条 files 行 + 一条 slides 行 + FTS 行）。
//
// 三项指标按 §19 登记口径：files 行数、ListFiles（前端 ListAssetFiles 全量
// 拉取）耗时、SearchAssets 典型词耗时；附带统计 GetLibraryStats 耗时。
//
// 语料构造绕过 pptx 解析直接批量写主表 + FTS 双写（1208 页解析量级不可接受），
// FTS 文本经 splitFtsText 切分——与生产写路径同源。命中注入规则沿用
// assets_search_bench_test.go 的页正文/页标题两条（约 2% 页正文植入「城商行」、
// 约 5% 页标题植入「获客」），两档按同一页序注入，命中页数完全相同；**未复制
// 该基准的文件名注入**（真实库里文件名命中走 files 行，与本次测量的索引规模面
// 正交，故不引入）。
//
// after 形态贴合生产落点：产物目录 = 源所在目录下以源主干命名的子目录，文件名
// 为 `<3 位页序>-<源主干>.pptx`（见 assets_split.go 的 replaceArtifactName），
// 即 53 个源目录 / 1208 个单页文件，而非 1208 个独立源。

// replaceScaleState 一档索引形态：名称 + 每源页数分布 + 是否已全量替换。
type replaceScaleState struct {
	name    string
	sources []int // 每个源文件的页数（长度 = 源文件数）
	replace bool  // true = 已全量 replace（每页一个单页产物，files 行 = 页数）
}

// fileCount 本形态的 files 行数：未替换 = 源文件数；已替换 = 页总数。
func (s replaceScaleState) fileCount() int {
	if !s.replace {
		return len(s.sources)
	}
	total := 0
	for _, pages := range s.sources {
		total += pages
	}
	return total
}

// replaceScaleStates 两档形态：同一批 53 个源、1208 页，before 为源文件本身，
// after 为全量 replace 后的单页产物。
func replaceScaleStates() []replaceScaleState {
	const totalPages = 1208
	const sources = 53

	pages := make([]int, sources)
	remaining := totalPages
	for i := range pages {
		// 余量摊到前置源，整体合计严格等于 totalPages
		sourcesLeft := sources - i
		pages[i] = remaining / sourcesLeft
		remaining -= pages[i]
	}

	return []replaceScaleState{
		{name: "before_53files_1208slides", sources: pages, replace: false},
		{name: "after_1208files_1slide", sources: pages, replace: true},
	}
}

// buildReplaceScaleCorpus 程序化构造一档索引形态（单事务批量写）。
// 页序跨源连续递增，两档同一源同一页拿到同一注入结果，命中集合完全一致。
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

	// insertFile 写一条 files 行 + 对应文件级 FTS 行。
	insertFile := func(relPath, fileName string) int64 {
		tb.Helper()
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
		return fileID
	}
	// insertSlide 写一条 slides 行 + 对应幻灯片级 FTS 行（rowid 取 -slideID，
	// 与生产写路径同一约定）。
	insertSlide := func(fileID int64, slideNum int, title, text string) {
		tb.Helper()
		res, err := insSlide.Exec(fileID, slideNum, title, text)
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

	rng := rand.New(rand.NewSource(42)) // 确定性语料（与检索基准同源）
	seq := 0
	for src, pages := range state.sources {
		base := fmt.Sprintf("源%03d", src)
		if !state.replace {
			fileID := insertFile(fmt.Sprintf("素材源/%s.pptx", base), base)
			for p := 1; p <= pages; p++ {
				seq++
				insertSlide(fileID, p, replaceScaleTitle(base, p, seq), replaceScaleText(rng, seq))
			}
			continue
		}
		for p := 1; p <= pages; p++ {
			seq++
			// 生产命名：<3 位页序>-<源主干>.pptx，位于以源主干命名的子目录
			artifact := fmt.Sprintf("%03d-%s", p, base)
			fileID := insertFile(fmt.Sprintf("素材源/%s/%s.pptx", base, artifact), artifact)
			insertSlide(fileID, 1, replaceScaleTitle(base, p, seq), replaceScaleText(rng, seq))
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatalf("提交语料事务: %v", err)
	}
}

// replaceScaleText 页正文：120 随机汉字，约 2% 页植入「城商行」。
func replaceScaleText(rng *rand.Rand, seq int) string {
	text := benchRandomText(rng, 120)
	if seq%50 == 0 {
		text = text[:60] + "城商行" + text[60:]
	}
	return text
}

// replaceScaleTitle 页标题：模板 + 约 5% 页植入「获客」。
func replaceScaleTitle(base string, page, seq int) string {
	title := fmt.Sprintf("%s 第%03d页", base, page)
	if seq%20 == 0 {
		title = "获客" + title
	}
	return title
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
				if len(entries) != state.fileCount() {
					b.Fatalf("ListFiles 条目数 %d，期望 %d", len(entries), state.fileCount())
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
				if stats.FileCount != state.fileCount() {
					b.Fatalf("统计文件数 %d，期望 %d", stats.FileCount, state.fileCount())
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

// replaceScaleStateEnv 指定本次测量哪一档（before|after）。
// replaceScaleStateEnv 指定本次测量哪一档（before|after）。必须分进程执行：
// 同一进程连续测量两档时，后测档的同类查询会整体放慢约 2 倍（连裸 SQL 也会，
// 与 GC 无关），跨档同进程串跑得到的倍数不可信。
const replaceScaleStateEnv = "ASSET_REPLACE_SCALE_STATE"

// replaceScaleStateByName 按环境变量取档；未设置时跳过（常规 `go test` 不受
// 影响），取值非法则报错。
func replaceScaleStateByName(tb testing.TB) (replaceScaleState, bool) {
	tb.Helper()
	states := replaceScaleStates()
	switch os.Getenv(replaceScaleStateEnv) {
	case "before":
		return states[0], true
	case "after":
		return states[1], true
	case "":
		return replaceScaleState{}, false
	default:
		tb.Fatalf("%s 取值非法：需 before|after", replaceScaleStateEnv)
		return replaceScaleState{}, false
	}
}

// TestAssetReplaceScale_Report 输出单档的 §19 复测口径数据（3 轮中位），按
// `ASSET_REPLACE_SCALE_STATE=before|after` 分两次进程执行，数字回填
// docs/spec/perf-baseline.md §19.1。-short 跳过（快速档不含性能测量）。
func TestAssetReplaceScale_Report(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过 replace 索引规模基线报告（由 BenchmarkAssetReplaceScale 手动采集）")
	}
	state, ok := replaceScaleStateByName(t)
	if !ok {
		t.Skipf("未设置 %s（before|after），跳过；两档须分进程执行，同进程连测会互相放慢", replaceScaleStateEnv)
	}
	svc, libID, store := newReplaceScaleService(t, state)
	files, slides, fts := replaceScaleCounts(t, store)

	// 预热一次（排除首次查询的惰性初始化：连接池/页缓存）
	if _, err := svc.ListFiles(libID); err != nil {
		t.Fatalf("%s ListFiles 预热: %v", state.name, err)
	}
	if _, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 50}); err != nil {
		t.Fatalf("%s SearchAssets 预热: %v", state.name, err)
	}
	// 命中页数取平铺口径（折叠口径的条目数随文件分布变化，属 replace 语义本身
	// 的结果，不能用来判断语料同源）
	hitBody, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: true, Limit: 50})
	if err != nil {
		t.Fatalf("%s SearchAssets 平铺(正文词): %v", state.name, err)
	}
	hitTitle, err := svc.SearchAssets(libID, "获客", model.AssetSearchOptions{Flatten: true, Limit: 50})
	if err != nil {
		t.Fatalf("%s SearchAssets 平铺(标题词): %v", state.name, err)
	}
	if hitBody.Total == 0 || hitTitle.Total == 0 {
		t.Fatalf("%s 命中为零（语料注入异常）", state.name)
	}

	listDur, err := measureReplaceScaleRounds(func() error {
		_, err := svc.ListFiles(libID)
		return err
	})
	if err != nil {
		t.Fatalf("%s ListFiles: %v", state.name, err)
	}
	bodyDur, err := measureReplaceScaleRounds(func() error {
		_, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: false, Limit: 50})
		return err
	})
	if err != nil {
		t.Fatalf("%s SearchAssets(正文词): %v", state.name, err)
	}
	titleDur, err := measureReplaceScaleRounds(func() error {
		_, err := svc.SearchAssets(libID, "获客", model.AssetSearchOptions{Flatten: false, Limit: 50})
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

	t.Logf("%s | %d | %d | %d | %v | %v | %v | %v | %d/%d",
		state.name, files, slides, fts, listDur, bodyDur, titleDur, statsDur, hitBody.Total, hitTitle.Total)
}

// TestAssetReplaceScale_HitParity 两档命中页数必须一致（同源语料的可比性前提）：
// 不同则耗时不可比。本用例只比计数不测耗时，可同进程跑，-short 跳过。
func TestAssetReplaceScale_HitParity(t *testing.T) {
	if testing.Short() {
		t.Skip("-short 跳过 replace 索引规模命中一致性校验")
	}
	hits := make([]string, 0, 2)
	for _, state := range replaceScaleStates() {
		svc, libID, _ := newReplaceScaleService(t, state)
		body, err := svc.SearchAssets(libID, "城商行", model.AssetSearchOptions{Flatten: true, Limit: 50})
		if err != nil {
			t.Fatalf("%s SearchAssets(正文词): %v", state.name, err)
		}
		title, err := svc.SearchAssets(libID, "获客", model.AssetSearchOptions{Flatten: true, Limit: 50})
		if err != nil {
			t.Fatalf("%s SearchAssets(标题词): %v", state.name, err)
		}
		hits = append(hits, fmt.Sprintf("%d/%d", body.Total, title.Total))
	}
	if hits[0] != hits[1] {
		t.Fatalf("两档命中页数不一致（正文/标题）：%s vs %s，语料注入不同源，耗时不可比", hits[0], hits[1])
	}
	t.Logf("两档命中页数一致：%s", hits[0])
}
