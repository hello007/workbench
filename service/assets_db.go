package service

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	// 纯 Go SQLite 驱动（零 CGO，GOOS=linux 交叉编译可用；FTS5 默认内置，PR2 使用）。
	// 驱动注册名 "sqlite"。选型依据见 .trellis/tasks/10-13-dam-ppt/research/sqlite-fts5-chinese.md。
	_ "modernc.org/sqlite"

	"workbench/model"
)

// assetsStore 单个素材库的 SQLite 索引库访问层。
//
// 连接策略（research 结论）：单写连接（SetMaxOpenConns(1)）规避写并发 SQLITE_BUSY，
// journal_mode(WAL) 提升读并发，busy_timeout 兜底锁等待，foreign_keys 经 DSN 对
// 每个连接启用（slides/file_tags 的 ON DELETE CASCADE 依赖它）。DSN 路径统一转
// 正斜杠（Windows 反斜杠在 file: URI 中语义不稳）。
//
// 生命周期：由 AssetLibraryService 懒打开并缓存（openDbs），库删除/服务关停时
// Close；并发关闭保护见 assets_library.go（库删除经扫描 TryLock 与在途扫描互斥，
// CloseAll 仅关停期调用）。
type assetsStore struct {
	db *sql.DB
}

// openAssetsStore 打开（或创建）素材索引库并执行 schema 迁移。
// 打开失败或迁移失败时返回的 store 为 nil，连接由本函数负责收口。
func openAssetsStore(dbPath string) (*assetsStore, error) {
	// 索引数据目录不存在时先建（首次运行/首次建库：data/assets 目录由首个库文件创建）
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("创建素材索引目录失败: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(dbPath) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开素材索引库失败: %w", err)
	}
	// 单写连接：本地桌面场景读写同连接串行，规避多连接写锁竞争（research §1.3）
	db.SetMaxOpenConns(1)
	store := &assetsStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close 关闭底层连接（WAL 文件在正常关闭时由 SQLite checkpoint 收口）。
func (s *assetsStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// migrate 按 PRAGMA user_version 递增执行迁移：版本相同幂等跳过；库文件版本
// 高于当前程序支持时拒绝打开（由更新版本应用创建，盲写会损坏数据）。
// 迁移 SQL 之外需 Go 层逻辑的版本（v2 的 FTS 存量回填）在对应版本 SQL 执行
// 成功后立即执行，失败时整体迁移失败（不留半态，下次打开重试）。
func (s *assetsStore) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("读取素材索引库版本失败: %w", err)
	}
	if version > assetsSchemaVersion {
		return fmt.Errorf("素材索引库版本过新（v%d > v%d），请升级 WorkBench 后再打开", version, assetsSchemaVersion)
	}
	for v := version + 1; v <= assetsSchemaVersion; v++ {
		migration, ok := assetsMigrations[v]
		if !ok {
			continue // 版本空洞（理论不可达）：缺迁移定义时保持版本号连续性由后续版本处理
		}
		if _, err := s.db.Exec(migration); err != nil {
			return fmt.Errorf("素材索引库迁移到 v%d 失败: %w", v, err)
		}
		if v == 2 {
			if err := s.backfillFtsOnUpgradeV2(); err != nil {
				return fmt.Errorf("素材索引库 v2 检索索引回填失败: %w", err)
			}
		}
	}
	if version != assetsSchemaVersion {
		if _, err := s.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", assetsSchemaVersion)); err != nil {
			return fmt.Errorf("写入素材索引库版本失败: %w", err)
		}
	}
	return nil
}

// assetFileRecord 扫描增量判定所需的文件记录最小视图。
type assetFileRecord struct {
	id      int64
	path    string
	size    int64
	mtimeNs int64
	status  string
}

// ---------- FTS 双写辅助（写路径统一收口，事务内调用） ----------

// selectSlideIDsTx 事务内取文件现有幻灯片 ID 集合（slides 整体替换/删除前收集，
// 用于按 rowid 编码约定定位并清除对应 FTS 行，防旧页索引残留静默漏检）。
func selectSlideIDsTx(tx *sql.Tx, fileID int64) ([]int64, error) {
	rows, err := tx.Query("SELECT id FROM slides WHERE file_id = ?", fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// deleteFtsRowTx 按 rowid 删除 FTS 行（编码约定见 assets_schema.go：文件行
// rowid = files.id，幻灯片行 rowid = -slides.id）。rowid 精确命中，O(1) 定位。
func deleteFtsRowTx(tx *sql.Tx, rowid int64) error {
	_, err := tx.Exec("DELETE FROM search_fts WHERE rowid = ?", rowid)
	return err
}

// syncFtsFileRowTx 维护文件的 FTS 行（rowid = fileID，先删后插）：以 files 表
// 当前 title/note 为准（经 splitFtsText 同源切分）。仅 ok 状态文件保留 FTS 行
// （失败/加密/权限文件无可检索内容，与检索口径一致）；行不存在时幂等清 FTS 行。
func syncFtsFileRowTx(tx *sql.Tx, fileID int64) error {
	var title, note, status string
	err := tx.QueryRow("SELECT title, note, status FROM files WHERE id = ?", fileID).
		Scan(&title, &note, &status)
	if errors.Is(err, sql.ErrNoRows) {
		// 文件行已删除（并发删除窗口）：连带清 FTS 行防悬挂
		return deleteFtsRowTx(tx, fileID)
	}
	if err != nil {
		return err
	}
	if err := deleteFtsRowTx(tx, fileID); err != nil {
		return err
	}
	if status != model.AssetFileStatusOK {
		return nil // 非 ok 终态不入检索索引
	}
	_, err = tx.Exec(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) "+
			"VALUES(?, 'file', ?, ?, ?, ?, '', '', '')",
		fileID, fileID, fileID, splitFtsText(title), splitFtsText(note))
	return err
}

// insertFtsSlideRowTx 新幻灯片的 FTS 行写入（rowid = -slideID，切分与写路径同源）。
func insertFtsSlideRowTx(tx *sql.Tx, slideID, fileID int64, title, text, notes string) error {
	_, err := tx.Exec(
		"INSERT INTO search_fts(rowid, ref_type, ref_id, file_id, file_name, file_note, slide_title, slide_text, slide_notes) "+
			"VALUES(?, 'slide', ?, ?, '', '', ?, ?, ?)",
		-slideID, slideID, fileID, splitFtsText(title), splitFtsText(text), splitFtsText(notes))
	return err
}

// listFileRecords 全量文件记录（库规模 = 素材 pptx 数，桌面场景千级以内，全量载入内存做增量差集）。
func (s *assetsStore) listFileRecords() ([]assetFileRecord, error) {
	rows, err := s.db.Query("SELECT id, path, size, mtime_ns, status FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []assetFileRecord
	for rows.Next() {
		var rec assetFileRecord
		if err := rows.Scan(&rec.id, &rec.path, &rec.size, &rec.mtimeNs, &rec.status); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// insertProcessingFile 新文件入库占位（status=processing）：解析前先落行，
// 崩溃残留的 processing 行在下次扫描按「必重处理」语义自愈（见 assets_scan.go
// 的增量判定条件）。返回新记录 ID（slides 外键需要）。
func (s *assetsStore) insertProcessingFile(relPath, title string, size, mtimeNs int64, now string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO files(path, size, mtime_ns, status, title, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)",
		relPath, size, mtimeNs, model.AssetFileStatusProcessing, title, now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// markProcessing 已有记录进入重处理（status=processing，mtime/size 暂不动：
// 解析中途崩溃时下次扫描仍能经由 mtime 失配重处理）。
func (s *assetsStore) markProcessing(fileID int64, now string) error {
	_, err := s.db.Exec("UPDATE files SET status = ?, updated_at = ? WHERE id = ?",
		model.AssetFileStatusProcessing, now, fileID)
	return err
}

// setFileOutcome 写入单文件解析终态（成功之外的分支：失败/加密/权限）。旧 slides
// 同步清除（文件内容已变，旧页索引不可再信），FTS 行同步清除（非 ok 终态不入
// 检索索引，含原 ok 状态降级为失败的场景）。权限跳过时 hash 为空（文件不可读）。
func (s *assetsStore) setFileOutcome(fileID int64, size, mtimeNs int64, hash, status, now string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		"UPDATE files SET size = ?, mtime_ns = ?, hash = ?, status = ?, updated_at = ? WHERE id = ?",
		size, mtimeNs, hash, status, now, fileID,
	); err != nil {
		return err
	}
	// FTS 清理先行（需旧 slides ID 定位幻灯片行），再删主表行
	slideIDs, err := selectSlideIDsTx(tx, fileID)
	if err != nil {
		return err
	}
	for _, id := range slideIDs {
		if err := deleteFtsRowTx(tx, -id); err != nil {
			return err
		}
	}
	if err := deleteFtsRowTx(tx, fileID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM slides WHERE file_id = ?", fileID); err != nil {
		return err
	}
	return tx.Commit()
}

// replaceFileSlides 解析成功的落库（单事务）：文件行更新为 ok 终态 + 幻灯片
// 整体替换（删旧插新，页序由 slide_num 唯一索引保证）+ FTS 双写（文件行与全部
// 新页行同事务写入；旧页 FTS 行先清防残留）。
func (s *assetsStore) replaceFileSlides(fileID int64, size, mtimeNs int64, hash, now string, slides []parsedSlide) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		"UPDATE files SET size = ?, mtime_ns = ?, hash = ?, status = ?, updated_at = ? WHERE id = ?",
		size, mtimeNs, hash, model.AssetFileStatusOK, now, fileID,
	); err != nil {
		return err
	}
	// 旧页 FTS 行清理（整体替换语义：旧页索引不可信），先于主表删除
	slideIDs, err := selectSlideIDsTx(tx, fileID)
	if err != nil {
		return err
	}
	for _, id := range slideIDs {
		if err := deleteFtsRowTx(tx, -id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("DELETE FROM slides WHERE file_id = ?", fileID); err != nil {
		return err
	}
	// 文件 FTS 行同步（status 已更新为 ok，读回 title/note 写入）
	if err := syncFtsFileRowTx(tx, fileID); err != nil {
		return err
	}
	for _, slide := range slides {
		res, err := tx.Exec(
			"INSERT INTO slides(file_id, slide_num, title, text, speaker_notes) VALUES(?, ?, ?, ?, ?)",
			fileID, slide.Num, slide.Title, slide.Text, slide.Notes,
		)
		if err != nil {
			return err
		}
		slideID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := insertFtsSlideRowTx(tx, slideID, fileID, slide.Title, slide.Text, slide.Notes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// deleteFilesByPaths 删除磁盘已消失的文件记录（slides/file_tags/thumb_cache 由
// 外键级联清除），FTS 行（文件行 + 全部幻灯片行）与缩略图记账（显式 DELETE，
// 外键级联为兜底的双保险口径）同步清除防脏数据。返回删除的 fileID 列表与总数
// （调用方据 fileID 清理缩略图 PNG 文件——文件系统操作不可回滚，db 先行、文件
// 后删，对齐「建库回滚先关连接再删文件」的收口顺序）。
func (s *assetsStore) deleteFilesByPaths(relPaths []string) ([]int64, int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var removed int64
	deletedIDs := make([]int64, 0)
	for _, p := range relPaths {
		var fileID int64
		err := tx.QueryRow("SELECT id FROM files WHERE path = ?", p).Scan(&fileID)
		if errors.Is(err, sql.ErrNoRows) {
			continue // 记录已不存在：幂等跳过
		}
		if err != nil {
			return nil, 0, err
		}
		// FTS 清理先行（需幻灯片 ID 定位），再删主表行（级联清 slides）
		slideIDs, err := selectSlideIDsTx(tx, fileID)
		if err != nil {
			return nil, 0, err
		}
		for _, id := range slideIDs {
			if err := deleteFtsRowTx(tx, -id); err != nil {
				return nil, 0, err
			}
		}
		if err := deleteFtsRowTx(tx, fileID); err != nil {
			return nil, 0, err
		}
		if _, err := tx.Exec("DELETE FROM thumb_cache WHERE file_id = ?", fileID); err != nil {
			return nil, 0, err
		}
		res, err := tx.Exec("DELETE FROM files WHERE id = ?", fileID)
		if err != nil {
			return nil, 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, 0, err
		}
		removed += n
		deletedIDs = append(deletedIDs, fileID)
	}
	return deletedIDs, removed, tx.Commit()
}

// libraryStats 统计：文件总数（含各状态）、幻灯片总数、失败数、加密跳过数。
// 全部走 QueryRow+Scan（自动排空结果集）：单连接池（SetMaxOpenConns(1)）下
// rows 未关闭就发下一条查询会自锁，禁止 Query+defer Close 后再查的写法。
func (s *assetsStore) libraryStats() (fileCount, slideCount, failedCount, encryptedCount int, err error) {
	err = s.db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(status = ?), 0), COALESCE(SUM(status = ?), 0) FROM files",
		model.AssetFileStatusFailed, model.AssetFileStatusEncrypted,
	).Scan(&fileCount, &failedCount, &encryptedCount)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM slides").Scan(&slideCount)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return fileCount, slideCount, failedCount, encryptedCount, nil
}

// listFileEntries 文件索引条目（按相对路径序），标签经二次查询按 file_id 聚合。
// rows 在标签查询前显式关闭：单连接池下未关先查会自锁（见 libraryStats 注释）。
func (s *assetsStore) listFileEntries() ([]model.AssetFileEntry, error) {
	rows, err := s.db.Query(
		"SELECT id, path, size, mtime_ns, status, title, note, rating, created_at, updated_at FROM files ORDER BY path",
	)
	if err != nil {
		return nil, err
	}

	entries := make([]model.AssetFileEntry, 0)
	idIndex := make(map[int64]int)
	for rows.Next() {
		var e model.AssetFileEntry
		var mtimeNs int64
		if err := rows.Scan(&e.ID, &e.Path, &e.Size, &mtimeNs, &e.Status, &e.Title, &e.Note, &e.Rating, &e.CreatedAt, &e.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		e.ModTime = formatAssetsTime(mtimeNs)
		e.Tags = []model.AssetTag{}
		idIndex[e.ID] = len(entries)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	tagsByFile, err := s.listFileTagGroups()
	if err != nil {
		return nil, err
	}
	for fileID, tags := range tagsByFile {
		if idx, ok := idIndex[fileID]; ok {
			entries[idx].Tags = tags
		}
	}
	return entries, nil
}

// listFileTagGroups 全库文件-标签分组（file_id -> 标签列表，按标签名排序）。
// parent_id 列可空（预留多级标签，本批恒 NULL），经 sql.NullInt64 归一为 0。
func (s *assetsStore) listFileTagGroups() (map[int64][]model.AssetTag, error) {
	rows, err := s.db.Query(
		"SELECT ft.file_id, t.id, t.name, t.color, t.parent_id FROM file_tags ft " +
			"JOIN tags t ON t.id = ft.tag_id ORDER BY t.name",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make(map[int64][]model.AssetTag)
	for rows.Next() {
		var fileID int64
		var tag model.AssetTag
		var parentID sql.NullInt64
		if err := rows.Scan(&fileID, &tag.ID, &tag.Name, &tag.Color, &parentID); err != nil {
			return nil, err
		}
		tag.ParentID = parentID.Int64
		groups[fileID] = append(groups[fileID], tag)
	}
	return groups, rows.Err()
}

// listSlideEntries 幻灯片索引条目分页（按文件路径 + 页序），标签继承归属文件。
// 分页契约对齐 SearchAssets：Total 为总条目数（不随页变化），翻页以 offset 递增
// limit 驱动。rows 在标签查询前显式关闭：单连接池下未关先查会自锁（见
// libraryStats 注释）；COUNT 走 QueryRow+Scan（自动排空）先于分页查询发出。
func (s *assetsStore) listSlideEntries(limit, offset int) (model.AssetSlidePage, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM slides").Scan(&total); err != nil {
		return model.AssetSlidePage{}, err
	}
	rows, err := s.db.Query(
		"SELECT s.id, s.file_id, f.path, s.slide_num, s.title, s.text, s.speaker_notes "+
			"FROM slides s JOIN files f ON f.id = s.file_id ORDER BY f.path, s.slide_num LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return model.AssetSlidePage{}, err
	}

	entries := make([]model.AssetSlideEntry, 0)
	idIndex := make(map[int64]int)
	for rows.Next() {
		var e model.AssetSlideEntry
		if err := rows.Scan(&e.ID, &e.FileID, &e.FilePath, &e.SlideNum, &e.Title, &e.Text, &e.SpeakerNotes); err != nil {
			_ = rows.Close()
			return model.AssetSlidePage{}, err
		}
		e.Tags = []model.AssetTag{}
		idIndex[e.ID] = len(entries)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return model.AssetSlidePage{}, err
	}
	if err := rows.Close(); err != nil {
		return model.AssetSlidePage{}, err
	}

	tagsByFile, err := s.listFileTagGroups()
	if err != nil {
		return model.AssetSlidePage{}, err
	}
	for i := range entries {
		entries[i].Tags = tagsByFile[entries[i].FileID]
		if entries[i].Tags == nil {
			entries[i].Tags = []model.AssetTag{}
		}
	}
	return model.AssetSlidePage{Items: entries, Total: total}, nil
}

// updateFileMeta 文件级元数据整体替换（单事务）：星级/备注 + 标签集重建；
// 不存在的标签自动创建（默认色板按标签名确定性分配）。文件不存在返回
// sql.ErrNoRows 语义错误，由服务层转用户可读提示。
func (s *assetsStore) updateFileMeta(fileID int64, rating int, note string, tagNames []string, now string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec("UPDATE files SET note = ?, rating = ?, updated_at = ? WHERE id = ?", note, rating, now, fileID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("素材文件不存在或已被删除（fileId=%d），请刷新列表", fileID)
	}

	if _, err := tx.Exec("DELETE FROM file_tags WHERE file_id = ?", fileID); err != nil {
		return err
	}
	seen := make(map[string]bool, len(tagNames))
	for _, name := range tagNames {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		tagID, err := ensureTagTx(tx, name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT OR IGNORE INTO file_tags(file_id, tag_id) VALUES(?, ?)", fileID, tagID); err != nil {
			return err
		}
	}
	// 备注参与全文检索（FTS file_note 列）：元数据变更后同步文件 FTS 行
	// （内部按 files.status 分流，非 ok 文件幂等清行为空操作）
	if err := syncFtsFileRowTx(tx, fileID); err != nil {
		return err
	}
	return tx.Commit()
}

// ensureTagTx 取标签 ID，不存在则自动创建（默认色板按标签名哈希确定性分配，
// 同名标签跨库/多次创建颜色稳定）。须在事务内调用。
func ensureTagTx(tx *sql.Tx, name string) (int64, error) {
	if _, err := tx.Exec(
		"INSERT INTO tags(name, color) VALUES(?, ?) ON CONFLICT(name) DO NOTHING",
		name, assetTagColorFor(name),
	); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow("SELECT id FROM tags WHERE name = ?", name).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// assetTagPalette 默认标签色板（前端展示 #RRGGBB，取值与设计令牌语义色同族，
// 避免标签色与主界面色系割裂）。
var assetTagPalette = []string{
	"#2563eb", // 靛蓝（主色）
	"#059669", // 绿
	"#d97706", // 橙
	"#dc2626", // 红
	"#7c3aed", // 紫
	"#0891b2", // 青
}

// assetTagColorFor 按标签名确定性取默认色（跨库同名同色，UI 认知稳定）。
func assetTagColorFor(name string) string {
	var h uint32
	for _, r := range name {
		h = h*31 + uint32(r)
	}
	return assetTagPalette[h%uint32(len(assetTagPalette))]
}

// formatAssetsTime mtime（UnixNano）转展示格式（与提交历史 DateTime 同款格式）。
func formatAssetsTime(mtimeNs int64) string {
	return time.Unix(mtimeNs/1e9, mtimeNs%1e9).Format("2006-01-02 15:04:05")
}

// ---------- 检索查询（assets_search.go 编排的数据源） ----------

// assetsFtsHit FTS 命中行最小视图：rowid 按编码约定解码条目类型与记录 ID。
type assetsFtsHit struct {
	rowid  int64
	fileID int64   // 归属文件 ID（文件行 = 自身 ID；幻灯片行 = 归属 files.id）
	score  float64 // bm25 负值口径：越小越相关
}

// assetsFtsBm25Args bm25() 列加权参数（顺序 = search_fts 列定义顺序，见
// assets_schema.go 的 assetsSchemaV2SQL）：ref_type/ref_id/file_id 三个 UNINDEXED
// 列占位 0 不参与评分；file_name 4.0 > slide_title 3.0 > slide_text 2.0 >
// file_note/slide_notes 1.0（文件名 > 幻灯片标题 > 幻灯片文本 > 备注）。
const assetsFtsBm25Args = "0.0, 0.0, 0.0, 4.0, 1.0, 3.0, 2.0, 1.0"

// searchFts FTS5 MATCH 检索：返回按 bm25 相关性升序（最相关在前）的全部命中。
// 结果全量载入（rowid+fileID+score 三元组，10 万行量级内存 ~数 MB），聚合折叠
// 与分页在 Go 层完成（聚合条目数依赖全量命中，无法下推 SQL LIMIT）。
func (s *assetsStore) searchFts(matchExpr string) ([]assetsFtsHit, error) {
	rows, err := s.db.Query(
		"SELECT rowid, file_id, bm25(search_fts, "+assetsFtsBm25Args+") AS score "+
			"FROM search_fts WHERE search_fts MATCH ? ORDER BY score",
		matchExpr,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := make([]assetsFtsHit, 0, 64)
	for rows.Next() {
		var h assetsFtsHit
		if err := rows.Scan(&h.rowid, &h.fileID, &h.score); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// fileMetaByIDs 批量取文件元数据（检索结果条目补全用，单批 IN 参数量受分页
// Limit 上限约束）。返回 map 缺键即文件已不存在（并发删除窗口，条目跳过）。
func (s *assetsStore) fileMetaByIDs(ids []int64) (map[int64]model.AssetFileEntry, error) {
	out := make(map[int64]model.AssetFileEntry, len(ids))
	for start := 0; start < len(ids); start += assetsQueryIDBatch {
		end := start + assetsQueryIDBatch
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?, ", end-start), ", ")
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		rows, err := s.db.Query(
			"SELECT id, path, size, mtime_ns, status, title, note, rating, created_at, updated_at FROM files "+
				"WHERE id IN ("+placeholders+")", args...)
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				var e model.AssetFileEntry
				var mtimeNs int64
				if err := rows.Scan(&e.ID, &e.Path, &e.Size, &mtimeNs, &e.Status, &e.Title, &e.Note, &e.Rating, &e.CreatedAt, &e.UpdatedAt); err != nil {
					return err
				}
				e.ModTime = formatAssetsTime(mtimeNs)
				out[e.ID] = e
			}
			return rows.Err()
		}()
		if err != nil {
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// slideMetaRow 检索结果补全所需的幻灯片元数据（含归属文件路径，溯源展示用）。
type slideMetaRow struct {
	id     int64
	fileID int64
	path   string
	num    int
	title  string
	text   string
	notes  string
}

// slideMetaByIDs 批量取幻灯片元数据（join files 取归属路径）。返回 map 缺键即
// 幻灯片已不存在（并发删除窗口，条目跳过）。
func (s *assetsStore) slideMetaByIDs(ids []int64) (map[int64]slideMetaRow, error) {
	out := make(map[int64]slideMetaRow, len(ids))
	for start := 0; start < len(ids); start += assetsQueryIDBatch {
		end := start + assetsQueryIDBatch
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?, ", end-start), ", ")
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		rows, err := s.db.Query(
			"SELECT s.id, s.file_id, f.path, s.slide_num, s.title, s.text, s.speaker_notes "+
				"FROM slides s JOIN files f ON f.id = s.file_id WHERE s.id IN ("+placeholders+")", args...)
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				var r slideMetaRow
				if err := rows.Scan(&r.id, &r.fileID, &r.path, &r.num, &r.title, &r.text, &r.notes); err != nil {
					return err
				}
				out[r.id] = r
			}
			return rows.Err()
		}()
		if err != nil {
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// assetsQueryIDBatch 批量 IN 查询的每批参数上限（SQLite 默认变量上限 999，
// 取安全余量；调用方数据量受分页 Limit 上限约束，通常单批完成）。
const assetsQueryIDBatch = 500

// fileIDsForTagFilter 标签筛选命中的文件 ID 集合（直接绑定任一给定标签）。
// 标签列表为空返回 nil（调用方跳过筛选）。
func (s *assetsStore) fileIDsForTagFilter(tagIDs []int64) (map[int64]bool, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(tagIDs)), ", ")
	args := make([]any, 0, len(tagIDs))
	for _, id := range tagIDs {
		args = append(args, id)
	}
	rows, err := s.db.Query(
		"SELECT DISTINCT file_id FROM file_tags WHERE tag_id IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = true
	}
	return set, rows.Err()
}

// tagFilteredFileOrdering 纯标签筛选的文件条目全量序（按路径序）：仅 ok 状态
// 且直接绑定任一给定标签。
type tagFilteredFileOrdering struct {
	id   int64
	path string
}

// listTagFilteredFiles 纯标签浏览的文件条目序（keyword 为空时的数据源）。
func (s *assetsStore) listTagFilteredFiles(tagIDs []int64) ([]tagFilteredFileOrdering, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(tagIDs)), ", ")
	args := make([]any, 0, len(tagIDs))
	for _, id := range tagIDs {
		args = append(args, id)
	}
	rows, err := s.db.Query(
		"SELECT f.id, f.path FROM files f WHERE f.status = ? AND f.id IN "+
			"(SELECT file_id FROM file_tags WHERE tag_id IN ("+placeholders+")) ORDER BY f.path",
		append([]any{model.AssetFileStatusOK}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]tagFilteredFileOrdering, 0)
	for rows.Next() {
		var r tagFilteredFileOrdering
		if err := rows.Scan(&r.id, &r.path); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// tagFilteredSlideOrdering 纯标签筛选的幻灯片条目全量序（按文件路径 + 页序）。
type tagFilteredSlideOrdering struct {
	id     int64
	fileID int64
	path   string
	num    int
}

// listTagFilteredSlides 纯标签浏览的幻灯片条目序（标签继承语义：归属文件绑定
// 即命中）。
func (s *assetsStore) listTagFilteredSlides(tagIDs []int64) ([]tagFilteredSlideOrdering, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(tagIDs)), ", ")
	args := make([]any, 0, len(tagIDs))
	for _, id := range tagIDs {
		args = append(args, id)
	}
	rows, err := s.db.Query(
		"SELECT s.id, s.file_id, f.path, s.slide_num FROM slides s JOIN files f ON f.id = s.file_id "+
			"WHERE f.status = ? AND s.file_id IN "+
			"(SELECT file_id FROM file_tags WHERE tag_id IN ("+placeholders+")) ORDER BY f.path, s.slide_num",
		append([]any{model.AssetFileStatusOK}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]tagFilteredSlideOrdering, 0)
	for rows.Next() {
		var r tagFilteredSlideOrdering
		if err := rows.Scan(&r.id, &r.fileID, &r.path, &r.num); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// fileTagBindingsByPath 重建前的用户标注备份：文件相对路径 → 标签 ID 列表
// （重建清 files 会级联清 file_tags，按路径映射在重建后写回，用户标注不丢）。
func (s *assetsStore) fileTagBindingsByPath() (map[string][]int64, error) {
	rows, err := s.db.Query(
		"SELECT f.path, ft.tag_id FROM file_tags ft JOIN files f ON f.id = ft.file_id ORDER BY f.path, ft.tag_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := make(map[string][]int64)
	for rows.Next() {
		var p string
		var tagID int64
		if err := rows.Scan(&p, &tagID); err != nil {
			return nil, err
		}
		bindings[p] = append(bindings[p], tagID)
	}
	return bindings, rows.Err()
}

// clearIndexForRebuild 全量重建前置清理（单事务）：清 files（slides/file_tags
// 随外键级联清除）+ FTS 全表 + 缩略图记账表（thumb_cache——files 级联本会清，
// 显式删除与调用方「thumbs 目录整体清空」同口径的双保险）。FTS 清空用 DELETE
// FROM（standalone FTS5 表不支持 contentless/external content 专用的 'delete-all'
// 命令）。标签本体（tags 表）保留——重建后经路径映射重绑。
func (s *assetsStore) clearIndexForRebuild() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec("DELETE FROM files"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM search_fts"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM thumb_cache"); err != nil {
		return err
	}
	return tx.Commit()
}

// rebindFileTagsAfterRebuild 全量重建后的标签重绑：按「文件相对路径 → 标签 ID
// 列表」映射将重建前保存的用户标注写回新文件记录（重建后 path 相同的文件保留
// 标签，path 已消失的映射自然丢弃）。返回重绑文件数。
func (s *assetsStore) rebindFileTagsAfterRebuild(bindings map[string][]int64) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query("SELECT id, path FROM files")
	if err != nil {
		return 0, err
	}
	pathToID := make(map[string]int64, len(bindings))
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			_ = rows.Close()
			return 0, err
		}
		pathToID[p] = id
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	rebound := 0
	for p, tagIDs := range bindings {
		fileID, ok := pathToID[p]
		if !ok {
			continue // 重建后文件已消失（扫描期间被删）：映射丢弃
		}
		for _, tagID := range tagIDs {
			if _, err := tx.Exec("INSERT OR IGNORE INTO file_tags(file_id, tag_id) VALUES(?, ?)", fileID, tagID); err != nil {
				return 0, err
			}
		}
		rebound++
	}
	return rebound, tx.Commit()
}

// ---------- 拆分记录（split_records 表，assets_split.go 编排的数据源） ----------

// assetSplitRecord 拆分记录行（split_records 表最小视图，列语义见
// assets_schema.go assetsSchemaV4SQL 注释）。
type assetSplitRecord struct {
	sourceRelPath string
	sourceHash    string
	outputDir     string
	pageCount     int
	splitAt       string
}

// getSplitRecord 按相对路径读单条拆分记录。未命中返回 (nil, nil)——调用方以
// nil 判定「尚未拆分」，sql.ErrNoRows 不上抛（QueryRow+Scan 单行口径，单连接
// 池下自动排空结果集，见 libraryStats 注释）。
func (s *assetsStore) getSplitRecord(relPath string) (*assetSplitRecord, error) {
	var r assetSplitRecord
	err := s.db.QueryRow(
		"SELECT source_rel_path, source_hash, output_dir, page_count, split_at FROM split_records WHERE source_rel_path = ?",
		relPath,
	).Scan(&r.sourceRelPath, &r.sourceHash, &r.outputDir, &r.pageCount, &r.splitAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// upsertSplitRecord 写入/覆盖拆分记录（重拆覆盖语义，主键 upsert 单语句，
// SQLite 自动提交保证原子性——多语句写才走显式事务，对齐 markProcessing 口径）。
func (s *assetsStore) upsertSplitRecord(relPath, sourceHash, outputDir string, pageCount int, splitAt string) error {
	_, err := s.db.Exec(
		"INSERT INTO split_records(source_rel_path, source_hash, output_dir, page_count, split_at) VALUES(?, ?, ?, ?, ?) "+
			"ON CONFLICT(source_rel_path) DO UPDATE SET source_hash = excluded.source_hash, output_dir = excluded.output_dir, "+
			"page_count = excluded.page_count, split_at = excluded.split_at",
		relPath, sourceHash, outputDir, pageCount, splitAt)
	return err
}

// deleteSplitRecord 删除拆分记录（预留清理口：当前无删除链路消费——拆分记录
// 随源文件路径变化由 upsert 覆盖、随库删除级联清理 db 文件；保留单点实现防
// 后续清理链路各写一套 SQL）。
func (s *assetsStore) deleteSplitRecord(relPath string) error {
	_, err := s.db.Exec("DELETE FROM split_records WHERE source_rel_path = ?", relPath)
	return err
}

// fileHashStatusByPath 按相对路径取文件记录的哈希与状态（拆分增量判定的源
// 哈希权威源）。sql.ErrNoRows 透传，调用方转用户可读错误。QueryRow+Scan 单行
// 口径（单连接池自锁纪律见 libraryStats 注释）。
func (s *assetsStore) fileHashStatusByPath(relPath string) (hash, status string, err error) {
	err = s.db.QueryRow("SELECT hash, status FROM files WHERE path = ?", relPath).Scan(&hash, &status)
	return hash, status, err
}

// assetSplitSource 拆分批量快照条目：相对路径 + 扫描链路维护的内容哈希。
type assetSplitSource struct {
	rel  string
	hash string
}

// listOKFileSplitSources 拆分批量快照：全部 ok 状态文件的（相对路径, 哈希）
// 清单，按路径序（进度与结果顺序确定，对齐 collectPptxEntries 排序口径）。
// 哈希列为拆分增量判定基准（为空时调用方经 hashFileSHA256 现算）。rows 收齐
// 后显式 Close（单连接池自锁纪律）。
func (s *assetsStore) listOKFileSplitSources() ([]assetSplitSource, error) {
	rows, err := s.db.Query("SELECT path, hash FROM files WHERE status = ? ORDER BY path", model.AssetFileStatusOK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := make([]assetSplitSource, 0, 64)
	for rows.Next() {
		var src assetSplitSource
		if err := rows.Scan(&src.rel, &src.hash); err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}
	return sources, rows.Err()
}
