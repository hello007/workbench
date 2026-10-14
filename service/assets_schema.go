package service

import (
	"workbench/model"
)

// 素材索引库 SQLite schema（侧边栏「素材管理」MVP）。
//
// 每个素材库一个独立 SQLite 文件（data/assets/<library-id>.db），标签体系随库
// 文件天然隔离。FTS5 全文检索（PR2）为 standalone 独立表 + 事务双写（research
// 结论：导入是批量事务场景，双写即可同步，无需触发器；external content 需配
// 触发器/rebuild，漏同步即静默漏检，不取）。
//
// FTS rowid 编码约定（检索与删除均按 rowid O(1) 定位，禁对 FTS 表做非 MATCH
// 的 WHERE 扫描——FTS5 表名隐藏列对普通条件不索引）：
//   - 文件行 rowid = files.id（正数）
//   - 幻灯片行 rowid = -slides.id（负数，符号区分类型）

// assetsSchemaVersion 当前 schema 版本。表结构变更时递增，并在
// assetsMigrations 中补充对应版本的迁移 SQL（旧库按版本逐级执行）。
const assetsSchemaVersion = 4

// assetsSchemaSQL v1 基线 schema（全部 CREATE IF NOT EXISTS，重复执行幂等）。
//
// 设计要点：
//   - files.path 存相对库根目录的正斜杠路径：库根目录移动/换机后索引仍有效，
//     且 Windows/Linux 路径形态统一（UNIQUE 约束作增量判定的连接键）；
//   - mtime 以 UnixNano 整数存储：比较与判等无时区/格式解析歧义；
//   - slides(file_id) ON DELETE CASCADE：删除文件记录时幻灯片同步清除（PRD
//     增量语义「删除文件后其幻灯片记录同步清除」由外键保证，PRAGMA foreign_keys
//     经 DSN 对每个连接启用）；
//   - tags.parent_id 预留多级标签（本批恒 NULL，模型层以 0 表达）；
//   - 状态取值见 model/assets.go 的 AssetFileStatus* 常量（普通 string 常量，
//     非具名类型，不经 Wails 绑定层）。
const assetsSchemaSQL = `
CREATE TABLE IF NOT EXISTS files (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	path       TEXT    NOT NULL UNIQUE,
	size       INTEGER NOT NULL DEFAULT 0,
	mtime_ns   INTEGER NOT NULL DEFAULT 0,
	hash       TEXT    NOT NULL DEFAULT '',
	status     TEXT    NOT NULL DEFAULT 'processing',
	title      TEXT    NOT NULL DEFAULT '',
	note       TEXT    NOT NULL DEFAULT '',
	rating     INTEGER NOT NULL DEFAULT 0,
	created_at TEXT    NOT NULL DEFAULT '',
	updated_at TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS slides (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	file_id       INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	slide_num     INTEGER NOT NULL,
	title         TEXT    NOT NULL DEFAULT '',
	text          TEXT    NOT NULL DEFAULT '',
	speaker_notes TEXT    NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_slides_file_num ON slides(file_id, slide_num);

CREATE TABLE IF NOT EXISTS tags (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	name      TEXT    NOT NULL UNIQUE,
	color     TEXT    NOT NULL DEFAULT '',
	parent_id INTEGER
);

CREATE TABLE IF NOT EXISTS file_tags (
	file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	tag_id  INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
	PRIMARY KEY (file_id, tag_id)
);
`

// assetsSchemaV2SQL v2 迁移：FTS5 standalone 检索表。
//
// 设计要点：
//   - 列顺序即 bm25() 加权参数顺序（UNINDEXED 列不参与评分仅占位），权重口径
//     文件名 > 幻灯片标题 > 幻灯片文本 > 备注（见 assetsFtsBm25Args）；
//   - ref_type/ref_id 为冗余记录列（UNINDEXED）：检索结果类型经 rowid 符号解码，
//     列值用于诊断与迁移回填校验，不参与 MATCH；
//   - file_id（UNINDEXED）：幻灯片行的归属文件 ID，标签筛选（继承语义）直接
//     取自 FTS 行，避免为过滤逐条回查主表；
//   - detail=full 必须显式保留：phrase 查询依赖 token 位置信息，detail=none/1
//     省索引会静默破坏按字短语匹配（research 坑 3）；
//   - tokenize 仅 unicode61（默认分词器）：切分由应用层 splitFtsText 统一完成
//     （写入与查询同源），分词器自身不承担中文切词。
const assetsSchemaV2SQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(
	ref_type UNINDEXED,
	ref_id UNINDEXED,
	file_id UNINDEXED,
	file_name,
	file_note,
	slide_title,
	slide_text,
	slide_notes,
	tokenize='unicode61', detail=full
);
`

// assetsSchemaV3SQL v3 迁移：缩略图记账表（LRU 淘汰 + 失效联动的事实源）。
//
// 设计要点（docs/spec/assets-thumbs-com.md 缓存键契约 + 任务 10-14-ppt-com
// research/lru-thumbnail-cache.md）：
//   - 主键 = file_id + tier + page：缓存键锚定 files.id（重解析仅 UPDATE 同行，
//     稳定不变）；slides.id 为 AUTOINCREMENT 删旧插新、每轮重解析全变，禁作键。
//   - 外键 ON DELETE CASCADE 删文件时级联清记账（兜底），代码在删除/重解析链路
//     仍显式 DELETE（双保险：单连接池 + WAL 下级联触发时机不受控，显式删除保证
//     记账与 PNG 文件同批收口）。
//   - last_access_ns 为 LRU 依据：NTFS atime 落盘延迟最多 1 小时且受系统策略与
//     第三方读流量影响（research §1.2），不可作持久化记账依据，应用层自记访问时间；
//     idx_thumb_cache_access 支撑淘汰游标按最旧优先扫描。
//   - 新表空启动：无 Go 层回填（存量库无缩略图，首次扫描后由记账链路自然填充）。
const assetsSchemaV3SQL = `
CREATE TABLE IF NOT EXISTS thumb_cache (
    file_id        INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    tier           TEXT    NOT NULL,
    page           INTEGER NOT NULL,
    bytes          INTEGER NOT NULL DEFAULT 0,
    last_access_ns INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (file_id, tier, page)
);

CREATE INDEX IF NOT EXISTS idx_thumb_cache_access ON thumb_cache(tier, last_access_ns);
`

// assetsSchemaV4SQL v4 迁移：PPT 拆分记录表（素材管理第三批，SRS §2.2.3——增量
// 拆分判定 + 跳转方式 2 产物路径反查的事实源）。
//
// 设计要点：
//   - source_rel_path 主键 = 相对库根目录的正斜杠路径，与 files.path 同口径
//     （库内唯一键）：同库不同目录同名 pptx 拆分记录天然区分；路径键而非 files.id
//     外键——拆分记录的生命周期独立于索引重建（重建清 files 不清拆分记录，产物
//     落在 derived 目录不受重建影响，重拆判定按内容哈希自愈）。
//   - source_hash 为拆分时源文件 sha256（增量判定基准）；output_dir 记录产物
//     目录（跳转方式 2 反查路径）；page_count + split_at 供前端展示。
//   - 无 Go 层回填（存量库无拆分记录，首次拆分自然填充）；删库随 db 文件级联
//     清理（表不落独立文件），与 thumb_cache 同口径。
const assetsSchemaV4SQL = `
CREATE TABLE IF NOT EXISTS split_records (
	source_rel_path TEXT    PRIMARY KEY,
	source_hash     TEXT    NOT NULL,
	output_dir      TEXT    NOT NULL,
	page_count      INTEGER NOT NULL,
	split_at        TEXT    NOT NULL
);
`

// assetsMigrations 版本递增迁移表：键为目标版本，值为从上一版本升到该版本的
// 迁移 SQL。v1 为基线 schema（幂等建表），历史库缺失表时同样可安全补齐。
// v2 建 FTS 表后由 Go 层对存量数据一次性回填（backfillFtsOnUpgradeV2：切分须
// 经 Go 函数，纯 SQL 无法完成同源切分）。
var assetsMigrations = map[int]string{
	1: assetsSchemaSQL,
	2: assetsSchemaV2SQL,
	3: assetsSchemaV3SQL,
	4: assetsSchemaV4SQL,
}

// backfillFtsOnUpgradeV2 v1→v2 迁移的存量数据一次性回填：全部幻灯片行 + ok 状态
// 文件行写入 FTS（切分文本与写路径同源经 splitFtsText）。迁移仅执行一次
// （user_version 门禁），回填失败时整体迁移失败、库打开失败，不留半态
// （下次打开重走 v2 迁移重试）。
func (s *assetsStore) backfillFtsOnUpgradeV2() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	insertFile, err := tx.Prepare(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) " +
			"VALUES(?, 'file', ?, ?, ?, ?, '', '', '')")
	if err != nil {
		return err
	}
	defer insertFile.Close()
	insertSlide, err := tx.Prepare(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) " +
			"VALUES(?, 'slide', ?, ?, '', '', ?, ?, ?)")
	if err != nil {
		return err
	}
	defer insertSlide.Close()

	// 文件行：仅 ok 状态可检索（失败/加密/权限文件无有效内容，与写路径口径一致）
	rows, err := tx.Query("SELECT id, title, note FROM files WHERE status = ?", model.AssetFileStatusOK)
	if err != nil {
		return err
	}
	type fileRow struct {
		id    int64
		title string
		note  string
	}
	fileRows := make([]fileRow, 0)
	for rows.Next() {
		var r fileRow
		if err := rows.Scan(&r.id, &r.title, &r.note); err != nil {
			_ = rows.Close()
			return err
		}
		fileRows = append(fileRows, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, r := range fileRows {
		if _, err := insertFile.Exec(r.id, r.id, r.id,
			splitFtsText(r.title), splitFtsText(r.note)); err != nil {
			return err
		}
	}

	// 幻灯片行：全部页（归属文件必为 ok——失败文件的 slides 已被清空）
	rows, err = tx.Query("SELECT id, file_id, title, text, speaker_notes FROM slides")
	if err != nil {
		return err
	}
	type slideRow struct {
		id     int64
		fileID int64
		title  string
		text   string
		notes  string
	}
	slideRows := make([]slideRow, 0)
	for rows.Next() {
		var r slideRow
		if err := rows.Scan(&r.id, &r.fileID, &r.title, &r.text, &r.notes); err != nil {
			_ = rows.Close()
			return err
		}
		slideRows = append(slideRows, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, r := range slideRows {
		if _, err := insertSlide.Exec(-r.id, r.id, r.fileID,
			splitFtsText(r.title), splitFtsText(r.text), splitFtsText(r.notes)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// countFtsRows FTS 行数统计（FTS 同步完整性断言与诊断用）。
func (s *assetsStore) countFtsRows() (int, error) {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM search_fts").Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
