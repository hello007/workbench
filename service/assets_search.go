package service

import (
	"fmt"
	"sort"
	"strings"

	"workbench/model"
)

// 素材检索编排（FTS5 + bm25 + 聚合折叠 + 标签筛选 + facet 多维筛选 + 分页）。
//
// 编排顺序（关键词模式）：FTS MATCH 全量命中（SQLite 按 bm25 升序返回）→
// facet 过滤（命中 fileID 全集一次批量 SQL 判定，Go 层按 fileID 查命中集合）→
// 标签过滤（Go 层，按 FTS 行携带的 file_id）→ 聚合折叠/平铺 → 内存分页 →
// 仅对当前页条目批量补全元数据与标签（IN 查询量受分页 Limit 约束，不随命中
// 总量放大）→ 原文窗口摘要。
//
// facet（SRS §2.5）为文件维度条件（星级/修改时间/文件大小），幻灯片条目按归属
// 文件属性继承判定；无关键词且无标签但有 facet 时走 facets-only 文件清单路径
// （searchByFacetsOnly）。
//
// 命中全量载入的量级依据：单命中仅 rowid+fileID+score 三元组（~24 字节），
// 10 万行命中 ≈ 数 MB 内存，桌面场景可接受；聚合折叠语义（同 PPT 多页命中
// 折叠）依赖全量命中集合，无法下推 SQL LIMIT。

// 分页默认值与上限（画廊懒加载：单页条目数固定，翻页以 offset 递增驱动）。
const (
	assetSearchDefaultLimit = 50
	assetSearchMaxLimit     = 200
)

// SearchAssets 素材检索：关键词全文检索（文件名/备注/页标题/页文本/备注，CJK
// 按字切分 phrase + bm25 列加权）、标签筛选（文件有效标签 = 自身绑定 ∪ 归属
// 文件夹级联继承的文件夹标签，幻灯片条目经归属文件继承同一有效标签集合，PRD D4
// + 文件夹标签级联检索）与 facet 三维筛选（星级/修改时间/文件大小，文件维度 +
// 幻灯片继承归属文件属性，SRS §2.5）可组合。关键词为空且给定标签时退化为纯标签
// 浏览（不走 FTS）；关键词与标签均为空但有 facet 时走 facets-only 文件清单路径
// （返回文件条目）；三者全空返回空结果页（不报错）。
//
// opts.Limit ≤0 取默认 50（上限 200）；Offset <0 按 0 处理。Flatten=false 时
// 同一 PPT 的多页命中折叠为一个聚合条目（HitCount = 命中页数，取最高分页展示，
// 按 bm25 最相关页计分），文件条目独立参与排序（SRS 2.5.3）。
//
// 排序档位（opts.SortBy，SRS §2.5.3）：相关性（默认，三路径现状序：关键词 FTS
// bm25 序 / 标签与 facets-only 路径序）；mtime（files.mtime_ns）与 rating
// （files.rating，文件级口径，幻灯片/聚合条目继承归属文件属性）两档在聚合与
// 分页之前对全量命中 Go 层稳定排序，SortDir 控制升降（默认降序），次级键确定性
// 见 sortEntriesByOptions。非法档位/方向值按相关性/降序处理。
func (s *AssetLibraryService) SearchAssets(libraryID, keyword string, opts model.AssetSearchOptions) (model.AssetSearchPage, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetSearchPage{}, err
	}
	keyword = strings.TrimSpace(keyword)
	if opts.Limit <= 0 {
		opts.Limit = assetSearchDefaultLimit
	}
	if opts.Limit > assetSearchMaxLimit {
		opts.Limit = assetSearchMaxLimit
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	var page model.AssetSearchPage
	err := s.withStore(libraryID, func(store *assetsStore) (err error) {
		page, err = searchAssetsInStore(store, keyword, opts)
		return err
	})
	if err != nil {
		return model.AssetSearchPage{}, err
	}
	page.Keyword = keyword
	page.Flattened = opts.Flatten
	return page, nil
}

// assetSearchEntry 检索中间条目（聚合/排序/分页处理单元，元数据补全前的最小
// 形态）。
type assetSearchEntry struct {
	refType     string  // model.AssetRefTypeFile / AssetRefTypeSlide
	refID       int64   // 文件条目 = files.id；幻灯片条目 = slides.id
	fileID      int64   // 归属文件 ID（标签过滤与继承口径）
	score       float64 // bm25 负值口径：越小越相关（纯标签模式恒 0）
	hitCount    int     // 聚合条目命中页数（其余恒 1）
	sortPath    string  // 纯标签模式确定性排序键（文件相对路径；关键词模式留空按 FTS 序）
	sortNum     int     // 纯标签模式页序次级键（文件条目恒 0 排同路径最前）
	sortMtimeNs int64   // mtime 排序键：归属文件 mtime_ns（幻灯片条目继承；仅 mtime 档填充消费）
	sortRating  int64   // rating 排序键：归属文件星级（幻灯片条目继承；仅 rating 档填充消费）
}

// searchAssetsInStore 检索主体（store 连接内执行）：分流关键词检索、纯标签浏览
// 与 facets-only 文件清单三条路径。标签筛选入参先经 expandTagDescendants 级联
// 展开（多级标签树命中口径 = 所选标签 ∪ 全部后代，PRD D2；平铺老数据全
// parent_id=0 时展开 = 自身集合，行为不变）——单点展开后透传两路径，禁在单一
// 路径内另行展开。标签筛选集合（fileIDsForTagFilter/slideOwnTagFilterSet）仅
// 关键词路径预取——其结果只被 FTS 命中行的 Go 层行级判定消费；纯标签浏览路径的
// 筛选由 listTagFilteredFiles/listTagFilteredSlides 的 SQL 子查询完成（继承合并
// 语义同源），提前返回避免两条无消费的 JOIN 查询。
//
// 空查询语义（SRS §2.5 facet 批次重定义）：无关键词且无标签但有 facet 条件 →
// facets-only 文件清单路径（文件维度筛选天然是文件清单语义）；三者全空仍返回
// 空结果页（不报错，行为不变）。
func searchAssetsInStore(store *assetsStore, keyword string, opts model.AssetSearchOptions) (model.AssetSearchPage, error) {
	if len(opts.TagIDs) > 0 {
		expanded, err := store.expandTagDescendants(opts.TagIDs)
		if err != nil {
			return model.AssetSearchPage{}, fmt.Errorf("标签子树展开失败: %w", err)
		}
		opts.TagIDs = expanded
	}
	// facet 条件单点提取：三条路径共用同一 assetsFacetFilter（nil = 未启用任何
	// facet，全部路径零行为变化）
	facet := newAssetsFacetFilter(opts)
	// folderPath 维度单点提取（R7 检索联动，与 facet 同层）：三条路径共用同一
	// assetsFolderFilter（nil = 未启用，全部路径零行为变化）
	folder := newAssetsFolderFilter(opts.FolderPath)
	if keyword == "" {
		if len(opts.TagIDs) == 0 {
			if facet == nil && folder == nil {
				// 空查询语义：无关键词、无标签且无 facet/folder 条件返回空结果页，
				// 不报错
				return model.AssetSearchPage{Items: []model.AssetSearchHit{}}, nil
			}
			return searchByFacetsOnly(store, opts, facet, folder)
		}
		return searchByTagsOnly(store, opts, facet, folder)
	}
	// 关键词路径：标签筛选集合（继承合并语义，见 slideTagRowMatches）——文件条目
	// 按有效标签命中（自身绑定 ∪ 归属文件夹级联继承，fileIDsForTagFilter 单点
	// 扩展）；幻灯片条目默认【归属文件有效标签 ∪ 页自身绑定】，OnlyOwn=true 仅
	// 页自身（文件夹级联继承同被裁剪；开关仅作用于标签筛选，关键词命中与文件条目
	// 不受影响）。TagIDs 为空时两函数短路返回 nil，无查询、行级判定全放行。
	tagFilter, err := store.fileIDsForTagFilter(opts.TagIDs)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("读取标签筛选集合失败: %w", err)
	}
	ownSlideFilter, err := store.slideOwnTagFilterSet(opts.TagIDs)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("读取标签筛选集合失败: %w", err)
	}
	return searchByKeyword(store, keyword, opts, facet, folder, tagFilter, ownSlideFilter)
}

// searchByKeyword 关键词全文检索路径。facet 非空时对 FTS 命中 fileID 全集一次
// 批量 SQL 判定（filterFileIDsByFacet，防逐命中 N+1），命中循环按行 fileID 查
// 命中集合 O(1) 判定——文件条目按自身属性、幻灯片条目按归属文件属性（继承），
// 判定位置与 slideTagRowMatches 同层（FTS 命中 → entries 构建处，分页前完成）。
// excluded 过滤（SRS §2.7.3）：excluded 文件 FTS 行保留（无损保留语义）但检索
// 不可见——命中 fileID 全集一次批量取排除集合（excludedFileIDSet），循环内剔除
// （与 facet 过滤同层同形态；lost 行 FTS 亦保留且有意可见引导重链，此处仅剔
// excluded）。
func searchByKeyword(store *assetsStore, keyword string, opts model.AssetSearchOptions, facet *assetsFacetFilter, folder *assetsFolderFilter, tagFilter, ownSlideFilter map[int64]bool) (model.AssetSearchPage, error) {
	matchExpr := buildFtsQuery(keyword)
	if matchExpr == "" {
		// 纯空白/全标点关键词：无有效查询词，空结果不报错
		return model.AssetSearchPage{Items: []model.AssetSearchHit{}}, nil
	}
	hits, err := store.searchFts(matchExpr)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("素材检索失败: %w", err)
	}

	// 命中 fileID 去重全集一次收集（excluded / facet / folder 三个批量判定共用）
	fileIDs := make([]int64, 0, len(hits))
	seen := make(map[int64]bool, len(hits))
	for _, h := range hits {
		if !seen[h.fileID] {
			seen[h.fileID] = true
			fileIDs = append(fileIDs, h.fileID)
		}
	}
	excludedSet, err := store.excludedFileIDSet(fileIDs)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("读取排除文件集合失败: %w", err)
	}

	// facet 命中文件集合批量预取（分页前完成判定，Total 按过滤后口径计）
	var facetHits map[int64]bool
	if facet != nil {
		facetHits, err = store.filterFileIDsByFacet(fileIDs, facet)
		if err != nil {
			return model.AssetSearchPage{}, fmt.Errorf("facet 筛选失败: %w", err)
		}
	}
	// folderPath 命中文件集合批量预取（R7 检索联动，与 facet 同层同形态；幻灯片
	// 条目按归属文件 ID 继承判定）
	var folderHits map[int64]bool
	if folder != nil {
		folderHits, err = store.filterFileIDsByFolder(fileIDs, folder)
		if err != nil {
			return model.AssetSearchPage{}, fmt.Errorf("文件夹筛选失败: %w", err)
		}
	}

	entries := make([]assetSearchEntry, 0, len(hits))
	for _, h := range hits {
		if excludedSet[h.fileID] {
			continue // excluded 文件（幻灯片条目随归属文件一并出局，SRS §2.7.3）
		}
		if facetHits != nil && !facetHits[h.fileID] {
			continue // facet 未命中（幻灯片条目随归属文件一并出局）
		}
		if folderHits != nil && !folderHits[h.fileID] {
			continue // folderPath 未命中（幻灯片条目随归属文件一并出局）
		}
		refType, refID := decodeFtsRowid(h.rowid)
		if !slideTagRowMatches(opts.SlideTagOnlyOwn, tagFilter, ownSlideFilter, refType, refID, h.fileID) {
			continue
		}
		entries = append(entries, assetSearchEntry{
			refType:  refType,
			refID:    refID,
			fileID:   h.fileID,
			score:    h.score,
			hitCount: 1,
		})
	}

	// mtime/rating 排序档：facet/标签过滤后 entries 的 fileID 全集一次批量取文件
	// 排序属性（fileSortPropsByIDs），填充排序键后 Go 层稳定排序（在聚合之前执行，
	// 聚合组首 = 排序后首现条目，文件维度单值继承天然正确）。相关性档不触发查询，
	// FTS bm25 序现状不动。
	if key, _ := assetSortKey(opts); key != "" {
		fileIDs := make([]int64, 0, len(entries))
		seen := make(map[int64]bool, len(entries))
		for _, e := range entries {
			if !seen[e.fileID] {
				seen[e.fileID] = true
				fileIDs = append(fileIDs, e.fileID)
			}
		}
		props, err := store.fileSortPropsByIDs(fileIDs)
		if err != nil {
			return model.AssetSearchPage{}, fmt.Errorf("读取排序文件属性失败: %w", err)
		}
		// props 缺键 = 文件行已消失（并发删除窗口）：直接剔除该 fileID 的全部条目，
		// 避免零值 mtime/rating 把已删文件排到序端形成用户可见异常序；与
		// pageAndFillEntries 填充层「元数据消失条目跳过」语义一致（剔除发生在
		// Total 计数与分页之前，total 口径随剔除收敛为存活条目）。
		filtered := entries[:0]
		for _, e := range entries {
			if _, ok := props[e.fileID]; ok {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
		for i := range entries {
			p := props[entries[i].fileID]
			entries[i].sortMtimeNs = p.mtimeNs
			entries[i].sortRating = p.rating
		}
		sortEntriesByOptions(entries, opts)
	}

	ordered := entries
	if !opts.Flatten {
		ordered = aggregateSlideEntries(entries)
	}

	items, total, err := pageAndFillEntries(store, ordered, opts, keyword)
	if err != nil {
		return model.AssetSearchPage{}, err
	}
	return model.AssetSearchPage{Items: items, Total: total}, nil
}

// searchByFacetsOnly facets-only 路径（无关键词无标签但有 facet 或 folderPath
// 条件，SRS §2.5）：按 facet/folder 条件列 ok 状态文件，返回文件条目（文件维度
// 筛选天然是文件清单语义，无幻灯片条目、无聚合折叠；Flatten 开关对结果无影响）。
// 相关性档按路径序（无相关性语义，Score 恒 0、Snippet 为空）；mtime/rating 档在
// 分页前按文件维度属性重排（sortEntriesByOptions），分页复用 pageAndFillEntries
// （对齐纯标签浏览的内存分页契约）。
func searchByFacetsOnly(store *assetsStore, opts model.AssetSearchOptions, facet *assetsFacetFilter, folder *assetsFolderFilter) (model.AssetSearchPage, error) {
	files, err := store.listFacetFilteredFiles(facet, folder)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("facet 筛选失败: %w", err)
	}
	entries := make([]assetSearchEntry, 0, len(files))
	for _, f := range files {
		entries = append(entries, assetSearchEntry{
			refType:     model.AssetRefTypeFile,
			refID:       f.id,
			fileID:      f.id,
			hitCount:    1,
			sortPath:    f.path,
			sortMtimeNs: f.mtimeNs,
			sortRating:  f.rating,
		})
	}
	// mtime/rating 排序档重排（相关性档该函数原样返回，路径序现状不动）
	sortEntriesByOptions(entries, opts)
	items, total, err := pageAndFillEntries(store, entries, opts, "")
	if err != nil {
		return model.AssetSearchPage{}, err
	}
	return model.AssetSearchPage{Items: items, Total: total}, nil
}

// searchByTagsOnly 纯标签浏览路径（keyword 为空）：不经 FTS，直接按标签绑定
// 关系列出文件与幻灯片（文件按有效标签命中 = 自身绑定 ∪ 归属文件夹级联继承，
// 幻灯片默认【归属文件有效标签 ∪ 页自身绑定】命中，SlideTagOnlyOwn=true 仅页
// 自身；文件条目不受开关影响），全量取 ID 后内存聚合
// 分页（无相关性语义，Score 恒 0、Snippet 为空；库规模 = 已索引文件/页数，量级可控）。
// facet 非空时两查询各追加 facet 条件（文件条目按自身属性、幻灯片条目经 JOIN
// files 按归属文件属性判定，继承语义与关键词路径同源）。
func searchByTagsOnly(store *assetsStore, opts model.AssetSearchOptions, facet *assetsFacetFilter, folder *assetsFolderFilter) (model.AssetSearchPage, error) {
	files, err := store.listTagFilteredFiles(opts.TagIDs, facet, folder)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("标签筛选失败: %w", err)
	}
	slides, err := store.listTagFilteredSlides(opts.TagIDs, opts.SlideTagOnlyOwn, facet, folder)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("标签筛选失败: %w", err)
	}

	entries := make([]assetSearchEntry, 0, len(files)+len(slides))
	for _, f := range files {
		entries = append(entries, assetSearchEntry{
			refType:     model.AssetRefTypeFile,
			refID:       f.id,
			fileID:      f.id,
			hitCount:    1,
			sortPath:    f.path,
			sortMtimeNs: f.mtimeNs,
			sortRating:  f.rating,
		})
	}
	for _, sl := range slides {
		entries = append(entries, assetSearchEntry{
			refType:     model.AssetRefTypeSlide,
			refID:       sl.id,
			fileID:      sl.fileID,
			hitCount:    1,
			sortPath:    sl.path,
			sortNum:     sl.num,
			sortMtimeNs: sl.mtimeNs,
			sortRating:  sl.rating,
		})
	}

	// 排序：路径序 → 文件条目排同路径最前（文件本体先于其页）→ 页序
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.sortPath != b.sortPath {
			return a.sortPath < b.sortPath
		}
		rankA, rankB := entryTypeRank(a.refType), entryTypeRank(b.refType)
		if rankA != rankB {
			return rankA < rankB
		}
		return a.sortNum < b.sortNum
	})
	// mtime/rating 排序档在路径序基础上按主键重排（同路径文件/页条目主键同值，
	// 次级键 sortNum 显式保留上述路径序的「文件本体先于其页」分组语义，见
	// sortEntriesByOptions 注释；相关性档该函数原样返回，现状不动），聚合与
	// 分页均消费重排后的最终序
	sortEntriesByOptions(entries, opts)

	if !opts.Flatten {
		entries = aggregateSlideEntries(entries)
	}

	items, total, err := pageAndFillEntries(store, entries, opts, "")
	if err != nil {
		return model.AssetSearchPage{}, err
	}
	return model.AssetSearchPage{Items: items, Total: total}, nil
}

// assetSortKey 排序入参归一化（SRS §2.5.3）：SortBy 合法档位为 mtime/rating，
// 其余取值（空串、relevance 与非法值）一律按相关性处理；SortDir 仅 "asc" 为
// 升序，其余取值（空串、desc 与非法值）一律按降序处理。返回排序主键（"" =
// 相关性/现状序）与是否升序。
func assetSortKey(opts model.AssetSearchOptions) (key string, asc bool) {
	switch opts.SortBy {
	case model.AssetSortByMtime, model.AssetSortByRating:
		key = opts.SortBy
	}
	return key, opts.SortDir == model.AssetSortDirAsc
}

// entrySortPrimary 条目排序主键取值：mtime 档 = 归属文件 mtime_ns；rating 档 =
// 归属文件星级（文件级口径单一数据源，幻灯片/聚合条目经 fileID 继承，与星级
// facet 口径同源）。
func entrySortPrimary(e assetSearchEntry, key string) int64 {
	if key == model.AssetSortByRating {
		return e.sortRating
	}
	return e.sortMtimeNs
}

// sortEntriesByOptions mtime/rating 排序档的 Go 层稳定排序（SRS §2.5.3）：调用方
// 须在聚合与分页之前执行——聚合组首 = 排序后首现条目，分页切片基于最终全量序。
// 主键方向随 SortDir 翻转；次级键方向恒定不随 SortDir 翻转（score 升序 = 越相关
// 越前 → sortPath 升序 → sortNum 升序 → refID 升序），保证主键同值条目跨分页顺序
// 稳定。sortNum 次级键显式化「文件本体先于其页」分组语义（文件条目恒 0，标签路径
// 页条目为页序）——同路径文件/页条目主键同值且 refID 属独立自增序列，缺该键时
// 相对序依赖 ID 分配顺序（重解析后页 ID 大幅增长才成立），属脆弱隐式依赖；关键词
// 路径各条目 sortNum 恒 0，补键零行为变化。相关性档
// （key == ""）原样返回不重排：关键词路径保持 FTS bm25 序、标签/facets-only 路径
// 保持调用方既有路径序（现状不动）。
func sortEntriesByOptions(entries []assetSearchEntry, opts model.AssetSearchOptions) {
	key, asc := assetSortKey(opts)
	if key == "" {
		return
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		pa, pb := entrySortPrimary(a, key), entrySortPrimary(b, key)
		if pa != pb {
			if asc {
				return pa < pb
			}
			return pa > pb
		}
		if a.score != b.score {
			return a.score < b.score
		}
		if a.sortPath != b.sortPath {
			return a.sortPath < b.sortPath
		}
		if a.sortNum != b.sortNum {
			return a.sortNum < b.sortNum
		}
		return a.refID < b.refID
	})
}

// entryTypeRank 排序辅助：文件条目（0）先于幻灯片条目（1）。
func entryTypeRank(refType string) int {
	if refType == model.AssetRefTypeFile {
		return 0
	}
	return 1
}

// decodeFtsRowid FTS rowid 编码解码（约定见 assets_schema.go）：正数 = 文件行，
// 负数 = 幻灯片行（绝对值 = slides.id）。
func decodeFtsRowid(rowid int64) (refType string, refID int64) {
	if rowid >= 0 {
		return model.AssetRefTypeFile, rowid
	}
	return model.AssetRefTypeSlide, -rowid
}

// aggregateSlideEntries 聚合折叠：幻灯片条目按归属文件分组折叠为聚合条目
// （score = 组内最高分页 = 序列中首个出现的页，HitCount = 命中页数），文件条目
// 独立保留。聚合条目出现在其组首页的原序列位置（两次遍历实现，杜绝 map 遍历
// 随机序破坏排序确定性），关键词模式与纯标签模式的输入序各自保持。
func aggregateSlideEntries(entries []assetSearchEntry) []assetSearchEntry {
	counts := make(map[int64]int)
	for _, e := range entries {
		if e.refType == model.AssetRefTypeSlide {
			counts[e.fileID]++
		}
	}
	seen := make(map[int64]bool, len(counts))
	ordered := make([]assetSearchEntry, 0, len(entries))
	for _, e := range entries {
		if e.refType != model.AssetRefTypeSlide {
			ordered = append(ordered, e)
			continue
		}
		if seen[e.fileID] {
			continue // 组内后续页：已被组首聚合条目代表
		}
		seen[e.fileID] = true
		e.hitCount = counts[e.fileID]
		ordered = append(ordered, e)
	}
	return ordered
}

// pageAndFillEntries 分页 + 元数据补全（仅当前页条目触发 IN 查询，查询量与命中
// 总量解耦）。返回结果页条目与过滤/聚合后的总条目数。
func pageAndFillEntries(store *assetsStore, entries []assetSearchEntry, opts model.AssetSearchOptions, keyword string) ([]model.AssetSearchHit, int, error) {
	total := len(entries)
	start := opts.Offset
	if start > total {
		start = total
	}
	end := start + opts.Limit
	if end > total {
		end = total
	}
	pageEntries := entries[start:end]

	items := make([]model.AssetSearchHit, 0, len(pageEntries))
	if len(pageEntries) == 0 {
		return items, total, nil
	}

	// 批量元数据：文件条目与幻灯片条目分别收集
	fileIDSet := make(map[int64]bool)
	slideIDSet := make(map[int64]bool)
	for _, e := range pageEntries {
		if e.refType == model.AssetRefTypeFile {
			fileIDSet[e.refID] = true
		} else {
			slideIDSet[e.refID] = true
		}
	}
	fileMetas, err := store.fileMetaByIDs(setToInt64s(fileIDSet))
	if err != nil {
		return nil, 0, fmt.Errorf("读取检索结果文件元数据失败: %w", err)
	}
	slideMetas, err := store.slideMetaByIDs(setToInt64s(slideIDSet))
	if err != nil {
		return nil, 0, fmt.Errorf("读取检索结果幻灯片元数据失败: %w", err)
	}
	tagsByFile, err := store.listFileTagGroups()
	if err != nil {
		return nil, 0, fmt.Errorf("读取检索结果标签失败: %w", err)
	}
	ownTagsBySlide, err := store.listSlideTagGroups()
	if err != nil {
		return nil, 0, fmt.Errorf("读取检索结果页标签失败: %w", err)
	}
	metaBySlide, err := store.listSlideMetaMap()
	if err != nil {
		return nil, 0, fmt.Errorf("读取检索结果页标注失败: %w", err)
	}

	for _, e := range pageEntries {
		hit := model.AssetSearchHit{
			RefType:  e.refType,
			RefID:    e.refID,
			Score:    e.score,
			HitCount: e.hitCount,
			Tags:     []model.AssetTag{},
			// 文件条目无页自有标注，空切片保证序列化为 [] 而非 null（nonNilTags 同款契约）
			OwnTags: []model.AssetTag{},
		}
		if e.refType == model.AssetRefTypeFile {
			m, ok := fileMetas[e.refID]
			if !ok {
				continue // 元数据已消失（并发删除窗口）：条目跳过，Total 不回改
			}
			hit.FileID = m.ID
			hit.FilePath = m.Path
			hit.Title = m.Title
			hit.Snippet = snippetFromTexts(keyword, []string{m.Title, m.Note})
			hit.Tags = nonNilTags(tagsByFile[m.ID])
		} else {
			m, ok := slideMetas[e.refID]
			if !ok {
				continue // 并发删除窗口（同上）
			}
			hit.FileID = m.fileID
			hit.FilePath = m.path
			hit.Title = m.title
			hit.SlideNum = m.num
			hit.Snippet = snippetFromTexts(keyword, []string{m.title, m.text, m.notes})
			hit.Tags = nonNilTags(tagsByFile[m.fileID])
			// 页自有标注（详情面板数据源；文件条目不填充——文件自有标注经
			// ListAssetFiles 的 AssetFileEntry 携带）
			key := slideAnnotKey{fileID: m.fileID, slideNum: int64(m.num)}
			hit.OwnTags = nonNilTags(ownTagsBySlide[key])
			if annot, ok := metaBySlide[key]; ok {
				hit.Note = annot.note
				hit.Rating = annot.rating
			}
		}
		items = append(items, hit)
	}
	return items, total, nil
}

// setToInt64s map 键转切片（IN 查询参数）。
func setToInt64s(set map[int64]bool) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

// nonNilTags 标签切片非空保证（Wails 绑定层 JSON 序列化为 [] 而非 null）。
func nonNilTags(tags []model.AssetTag) []model.AssetTag {
	if tags == nil {
		return []model.AssetTag{}
	}
	return tags
}
