package service

import (
	"fmt"
	"sort"
	"strings"

	"workbench/model"
)

// 素材检索编排（FTS5 + bm25 + 聚合折叠 + 标签筛选 + 分页）。
//
// 编排顺序（关键词模式）：FTS MATCH 全量命中（SQLite 按 bm25 升序返回）→
// 标签过滤（Go 层，按 FTS 行携带的 file_id）→ 聚合折叠/平铺 → 内存分页 →
// 仅对当前页条目批量补全元数据与标签（IN 查询量受分页 Limit 约束，不随命中
// 总量放大）→ 原文窗口摘要。
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
// 按字切分 phrase + bm25 列加权）与标签筛选（文件直接绑定、幻灯片继承归属文件
// 标签，PRD D4）可组合。关键词为空且给定标签时退化为纯标签浏览（不走 FTS）；
// 关键词与标签均为空返回空结果页（不报错）。
//
// opts.Limit ≤0 取默认 50（上限 200）；Offset <0 按 0 处理。Flatten=false 时
// 同一 PPT 的多页命中折叠为一个聚合条目（HitCount = 命中页数，取最高分页展示，
// 按 bm25 最相关页计分），文件条目独立参与排序（SRS 2.5.3）。
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
	refType  string  // model.AssetRefTypeFile / AssetRefTypeSlide
	refID    int64   // 文件条目 = files.id；幻灯片条目 = slides.id
	fileID   int64   // 归属文件 ID（标签过滤与继承口径）
	score    float64 // bm25 负值口径：越小越相关（纯标签模式恒 0）
	hitCount int     // 聚合条目命中页数（其余恒 1）
	sortPath string  // 纯标签模式确定性排序键（文件相对路径；关键词模式留空按 FTS 序）
	sortNum  int     // 纯标签模式页序次级键（文件条目恒 0 排同路径最前）
}

// searchAssetsInStore 检索主体（store 连接内执行）：分流关键词检索与纯标签浏览。
// 标签筛选入参先经 expandTagDescendants 级联展开（多级标签树命中口径 = 所选
// 标签 ∪ 全部后代，PRD D2；平铺老数据全 parent_id=0 时展开 = 自身集合，行为
// 不变）——单点展开后透传两路径，禁在单一路径内另行展开。标签筛选集合
// （fileIDsForTagFilter/slideOwnTagFilterSet）仅关键词路径预取——其结果只被
// FTS 命中行的 Go 层行级判定消费；纯标签浏览路径的筛选由
// listTagFilteredFiles/listTagFilteredSlides 的 SQL 子查询完成（继承合并语义同
// 源），提前返回避免两条无消费的 JOIN 查询。
func searchAssetsInStore(store *assetsStore, keyword string, opts model.AssetSearchOptions) (model.AssetSearchPage, error) {
	if len(opts.TagIDs) > 0 {
		expanded, err := store.expandTagDescendants(opts.TagIDs)
		if err != nil {
			return model.AssetSearchPage{}, fmt.Errorf("标签子树展开失败: %w", err)
		}
		opts.TagIDs = expanded
	}
	if keyword == "" {
		if len(opts.TagIDs) == 0 {
			// 空查询语义：无关键词且无筛选条件返回空结果页，不报错
			return model.AssetSearchPage{Items: []model.AssetSearchHit{}}, nil
		}
		return searchByTagsOnly(store, opts)
	}
	// 关键词路径：标签筛选集合（继承合并语义，见 slideTagRowMatches）——文件条目
	// 要求自身绑定；幻灯片条目默认【归属文件绑定 ∪ 页自身绑定】，OnlyOwn=true 仅
	// 页自身（开关仅作用于标签筛选，关键词命中与文件条目不受影响）。TagIDs 为空
	// 时两函数短路返回 nil，无查询、行级判定全放行。
	tagFilter, err := store.fileIDsForTagFilter(opts.TagIDs)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("读取标签筛选集合失败: %w", err)
	}
	ownSlideFilter, err := store.slideOwnTagFilterSet(opts.TagIDs)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("读取标签筛选集合失败: %w", err)
	}
	return searchByKeyword(store, keyword, opts, tagFilter, ownSlideFilter)
}

// searchByKeyword 关键词全文检索路径。
func searchByKeyword(store *assetsStore, keyword string, opts model.AssetSearchOptions, tagFilter, ownSlideFilter map[int64]bool) (model.AssetSearchPage, error) {
	matchExpr := buildFtsQuery(keyword)
	if matchExpr == "" {
		// 纯空白/全标点关键词：无有效查询词，空结果不报错
		return model.AssetSearchPage{Items: []model.AssetSearchHit{}}, nil
	}
	hits, err := store.searchFts(matchExpr)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("素材检索失败: %w", err)
	}

	entries := make([]assetSearchEntry, 0, len(hits))
	for _, h := range hits {
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

// searchByTagsOnly 纯标签浏览路径（keyword 为空）：不经 FTS，直接按标签绑定
// 关系列出文件与幻灯片（幻灯片默认【归属文件绑定 ∪ 页自身绑定】命中，
// SlideTagOnlyOwn=true 仅页自身；文件条目不受开关影响），全量取 ID 后内存聚合
// 分页（无相关性语义，Score 恒 0、Snippet 为空；库规模 = 已索引文件/页数，量级可控）。
func searchByTagsOnly(store *assetsStore, opts model.AssetSearchOptions) (model.AssetSearchPage, error) {
	files, err := store.listTagFilteredFiles(opts.TagIDs)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("标签筛选失败: %w", err)
	}
	slides, err := store.listTagFilteredSlides(opts.TagIDs, opts.SlideTagOnlyOwn)
	if err != nil {
		return model.AssetSearchPage{}, fmt.Errorf("标签筛选失败: %w", err)
	}

	entries := make([]assetSearchEntry, 0, len(files)+len(slides))
	for _, f := range files {
		entries = append(entries, assetSearchEntry{
			refType:  model.AssetRefTypeFile,
			refID:    f.id,
			fileID:   f.id,
			hitCount: 1,
			sortPath: f.path,
		})
	}
	for _, sl := range slides {
		entries = append(entries, assetSearchEntry{
			refType:  model.AssetRefTypeSlide,
			refID:    sl.id,
			fileID:   sl.fileID,
			hitCount: 1,
			sortPath: sl.path,
			sortNum:  sl.num,
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

	if !opts.Flatten {
		entries = aggregateSlideEntries(entries)
	}

	items, total, err := pageAndFillEntries(store, entries, opts, "")
	if err != nil {
		return model.AssetSearchPage{}, err
	}
	return model.AssetSearchPage{Items: items, Total: total}, nil
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
