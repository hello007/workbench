package service

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// vacuumInto 执行 VACUUM INTO 整库快照备份（素材库备份域，assets_backup.go 调用）。
// 约束（research 实证，.trellis/tasks/10-18-srs-2-7-3/research/sqlite-backup-restore.md）：
//   - 目标文件必须不存在（已存在报 output file already exists），调用方以时间戳
//     命名保证唯一；
//   - 不能在事务内执行（database/sql 自动提交下单条 Exec 无碍，禁包显式 Tx）；
//   - 输出为独立紧凑 db，WAL 未 checkpoint 的数据完整进入备份（无需预 checkpoint）；
//   - 单连接池下执行期间该库其他查询在池上排队（连接占用，60 万行 127ms，
//     素材库量级毫秒~百毫秒级，可接受）。
//
// 路径统一正斜杠（对齐 openAssetsStore DSN 惯例；SQL 字符串字面量内的单引号
// 双写转义防路径含引号破坏语句）。
func (s *assetsStore) vacuumInto(targetPath string) error {
	literal := strings.ReplaceAll(filepath.ToSlash(targetPath), "'", "''")
	_, err := s.db.Exec("VACUUM INTO '" + literal + "'")
	return err
}

// integrityCheck PRAGMA integrity_check 完整性校验。必须 Query 读结果集——
// Exec 方式拿不到结果（不报错但结果被丢弃，形同没校验，research 实证）；健康库
// 返回单行 "ok"，损坏库将全部问题聚合在单行文本内（内嵌换行），首行 != ok 即
// 判失败。须由调用方保证 rows 生命周期（本方法收齐首行后经 defer Close 释放
// 连接，对齐单连接池自锁纪律）。
func (s *assetsStore) integrityCheck() error {
	if s == nil || s.db == nil {
		return errors.New("索引库连接未打开")
	}
	rows, err := s.db.Query("PRAGMA integrity_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return errors.New("integrity_check 未返回结果")
	}
	var line string
	if err := rows.Scan(&line); err != nil {
		return err
	}
	if line != "ok" {
		return fmt.Errorf("integrity_check: %s", line)
	}
	return nil
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

// assetFileRecord 扫描增量判定与重链所需的文件记录最小视图（hash 供重链哈希
// 校验基准，扫描差分不消费）。
type assetFileRecord struct {
	id      int64
	path    string
	size    int64
	mtimeNs int64
	status  string
	hash    string
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
	rows, err := s.db.Query("SELECT id, path, size, mtime_ns, status, hash FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []assetFileRecord
	for rows.Next() {
		var rec assetFileRecord
		if err := rows.Scan(&rec.id, &rec.path, &rec.size, &rec.mtimeNs, &rec.status, &rec.hash); err != nil {
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
//
// 页级标注（slide_tags/slide_meta，键 (file_id, slide_num)）刻意保留不删：与
// file_tags 在文件行存续期间保留的语义一致——文件修复重解析成功后，replaceFileSlides
// 的失配清理仅剔除新页序中不存在的标注，现存页序的标注按位置语义复活（PRD 定案
// Q2：标注跟随页码不跟随内容）。文件记录被删除时两表经 files 外键 CASCADE 清除。
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
	// 页级标注失配清理（增量语义：页数不变时 NOT IN 为空集即无操作，标注天然
	// 保留；页数减少时失配页标注清除；页内容移位标注跟随页码——位置语义，与
	// 缩略图缓存键 file_id+page 同口径）
	if err := deleteStaleSlideAnnotationsTx(tx, fileID); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteStaleSlideAnnotationsTx 清理指定文件的失配页级标注（slides 删旧插新后
// 事务内调用）：slide_num 已不存在于新 slides 的 slide_tags/slide_meta 行删除。
// 须在事务内调用（与页替换同事务，失败整体回滚不留半态）。
func deleteStaleSlideAnnotationsTx(tx *sql.Tx, fileID int64) error {
	for _, table := range []string{"slide_tags", "slide_meta"} {
		if _, err := tx.Exec(
			"DELETE FROM "+table+" WHERE file_id = ? AND slide_num NOT IN "+
				"(SELECT slide_num FROM slides WHERE file_id = ?)", fileID, fileID,
		); err != nil {
			return err
		}
	}
	return nil
}

// pathOwnedByOther 目标路径是否已被其他记录占用（重链/前缀替换的 UNIQUE 冲突
// 预检）：ownerID == fileID 视为未占用（记录自身已指向该路径，重链同路径为
// no-op 放行）。
func (s *assetsStore) pathOwnedByOther(newRel string, fileID int64) (bool, error) {
	var ownerID int64
	err := s.db.QueryRow("SELECT id FROM files WHERE path = ?", newRel).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ownerID != fileID, nil
}

// ---------- 丢失与重链（SRS §2.6.5，素材管理第九批 PR2 数据面） ----------
// （lost 引入前的「删除磁盘已消失记录」实现已由「扫描标记 lost + clearLostFiles
// 手动清理」替代移除；删除语义收口于 clearLostFiles 单点。）

// fileRecordByID 按记录 ID 读单个文件记录（重链链路的现状读取）。
// sql.ErrNoRows 透传，调用方转用户可读错误（QueryRow+Scan 单行口径，单连接池
// 自锁纪律见 libraryStats 注释）。
func (s *assetsStore) fileRecordByID(fileID int64) (assetFileRecord, error) {
	var rec assetFileRecord
	err := s.db.QueryRow(
		"SELECT id, path, size, mtime_ns, status, hash FROM files WHERE id = ?", fileID,
	).Scan(&rec.id, &rec.path, &rec.size, &rec.mtimeNs, &rec.status, &rec.hash)
	return rec, err
}

// markFileLost 扫描发现磁盘文件消失时的标记（lost 态，D1）：仅改 status 与
// updated_at——size/mtime_ns/hash 保持原值，作为文件回归（lost 复活判定
// mtime+size 匹配）与手动重链（哈希校验基准）的依据；slides/标注/标签/缩略图
// 记账全保留（不再级联删除）。仅对非 lost 行生效（幂等：重复扫描不重复计数）。
func (s *assetsStore) markFileLost(fileID int64, now string) (int64, error) {
	res, err := s.db.Exec(
		"UPDATE files SET status = ?, updated_at = ? WHERE id = ? AND status != ?",
		model.AssetFileStatusLost, now, fileID, model.AssetFileStatusLost,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// reviveLostFile lost/excluded 文件直接复活 ok（内容未变旧解析/缩略图/FTS 仍
// 有效，仅翻转状态不重解析）：lost = 文件回归且 mtime+size 未变（SRS §2.6.5）；
// excluded = 规则改宽后磁盘仍在且未变（SRS §2.7.3，两类直复活共用）。调用方须
// 先过 fileHasSlides 资格门槛——本函数不校验解析产物，对 failed/encrypted/denied
// 行直复活会把坏文件翻成 ok。
//
// FTS 文件行同事务重建：非 ok 期间用户经文件底册编辑过备注/标题时（updateFileMeta
// → syncFtsFileRowTx 对非 ok 行先删后不插），FTS 文件行已缺失；若仅翻状态不重建，
// 此后每轮增量扫描 unchanged 跳过永不重解析，FTS 行永久缺失 → 关键词检索永久漏
// 该文件（扫描复活与前缀替换匹配收敛两处调用方共用本函数，一处收口）。页级 FTS
// 行不受影响（updateFileMeta 仅同步文件行），无需重建。syncFtsFileRowTx 读事务内
// 最新 status（已翻 ok）按 files 现存 title/note 重建；行未缺失时先删后插幂等无害。
func (s *assetsStore) reviveLostFile(fileID int64, now string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		"UPDATE files SET status = ?, updated_at = ? WHERE id = ?",
		model.AssetFileStatusOK, now, fileID,
	); err != nil {
		return err
	}
	if err := syncFtsFileRowTx(tx, fileID); err != nil {
		return err
	}
	return tx.Commit()
}

// markFileExcluded 存量索引路径命中用户排除规则时的标记（excluded 态，SRS
// §2.7.3）：仅改 status 与 updated_at——size/mtime_ns/hash 保持原值（规则改宽
// 后的复活判定与 lost 同口径走 mtime+size 匹配）；slides/标注/标签/FTS 行/
// 缩略图记账与 PNG 全保留（无损保留语义，对齐 markFileLost；可见性与 lost 相反：
// excluded 检索/标签/facet 不可见）。仅对非 excluded 行生效（幂等：重复扫描
// 不重复计数）。
func (s *assetsStore) markFileExcluded(fileID int64, now string) (int64, error) {
	res, err := s.db.Exec(
		"UPDATE files SET status = ?, updated_at = ? WHERE id = ? AND status != ?",
		model.AssetFileStatusExcluded, now, fileID, model.AssetFileStatusExcluded,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// fileHasSlides 文件是否存在解析产物幻灯片行——lost/excluded 直复活资格判定
// 单点（扫描复活〔lost 回归与 excluded 解除〕与前缀替换匹配收敛调用方共用，
// 禁分叉）：有产物 = 曾成功解析，旧解析/缩略图/FTS 对 mtime+size 未变的内容
// 仍有效，可直复活；无产物 = failed/encrypted/denied 跳过类或 0 页空演示文稿，
// 直复活会把坏文件谎报为 ok 且此后增量扫描 unchanged 跳过永不纠正，须走重解析
// 按实际内容重新分类。QueryRow+Scan 单行口径（单连接池自锁纪律）。
func (s *assetsStore) fileHasSlides(fileID int64) (bool, error) {
	var n int
	err := s.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM slides WHERE file_id = ?)", fileID,
	).Scan(&n)
	return n == 1, err
}

// relocateFileOK 哈希校验通过的重链落库：改 path + 刷新 size/mtime_ns（新位置
// 文件的 stat 实值——副本/移动可能改 mtime，不刷新则下次扫描误判变更重解析、
// 无谓失效缩略图）+ status=ok（内容同旧，解析/缩略图/FTS 全有效零恢复成本）。
// 零命中 = 行已被并发「清理丢失记录」删除（ClearLostAssetFiles 不持扫描锁，与
// 重链存在并发窗口）：UPDATE 空命中零操作，静默返回会按名册虚报重链成功——以
// sql.ErrNoRows 上抛（对齐 fileRecordByID 无行语义），调用方转可读错误。
func (s *assetsStore) relocateFileOK(fileID int64, newPath string, size, mtimeNs int64, now string) error {
	res, err := s.db.Exec(
		"UPDATE files SET path = ?, size = ?, mtime_ns = ?, status = ?, updated_at = ? WHERE id = ?",
		newPath, size, mtimeNs, model.AssetFileStatusOK, now, fileID,
	)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// relocateFileProcessing 哈希不等（force 确认）或记录无哈希的重链落库第一步：
// 改 path + 标记 processing（mtime/size/hash 暂不动，重解析按新位置写入终态；
// 解析中途崩溃残留 processing 行由下次扫描「必重处理」语义自愈，与扫描链路
// 同口径）。FTS/slides/缩略图由随后的单文件重解析链路处理。零命中 = 行已被
// 并发「清理丢失记录」删除（并发窗口同 relocateFileOK）：sql.ErrNoRows 上抛，
// 调用方转可读错误（且不再进入随后的重解析探测段）。
func (s *assetsStore) relocateFileProcessing(fileID int64, newPath string, now string) error {
	res, err := s.db.Exec(
		"UPDATE files SET path = ?, status = ?, updated_at = ? WHERE id = ?",
		newPath, model.AssetFileStatusProcessing, now, fileID,
	)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// listFilesByPathPrefix 旧前缀命中的文件记录（目录边界语义单点：path 恰等于
// 前缀，或以「前缀 + /」开头——`old` 命中 `old` 与 `old/...`，不误伤 `oldx/...`
// 同头缀目录）。LIKE 通配符转义（%/_/\）防前缀含通配符时扩大命中面。
func (s *assetsStore) listFilesByPathPrefix(oldPrefix string) ([]assetFileRecord, error) {
	escaped := escapeLikePattern(oldPrefix)
	rows, err := s.db.Query(
		"SELECT id, path, size, mtime_ns, status, hash FROM files "+
			"WHERE path = ? OR path LIKE ? ESCAPE '\\' ORDER BY path",
		oldPrefix, escaped+"/%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]assetFileRecord, 0)
	for rows.Next() {
		var rec assetFileRecord
		if err := rows.Scan(&rec.id, &rec.path, &rec.size, &rec.mtimeNs, &rec.status, &rec.hash); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// escapeLikePattern LIKE 模式转义单点：反斜杠（ESCAPE 字符自身）→ % → _。
func escapeLikePattern(raw string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(raw)
}

// countFilesByPathPrefix 前缀命中计数（PreviewAssetPathPrefixReplace 预览面，
// 与 listFilesByPathPrefix 同一 WHERE 构造——预览与执行的命中口径单点禁分叉）。
func (s *assetsStore) countFilesByPathPrefix(oldPrefix string) (int, error) {
	escaped := escapeLikePattern(oldPrefix)
	var n int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM files WHERE path = ? OR path LIKE ? ESCAPE '\\'",
		oldPrefix, escaped+"/%",
	).Scan(&n)
	return n, err
}

// replaceFilePaths 批量路径前缀替换落库（单事务，任一 UPDATE 失败整体回滚——
// 调用方已做 UNIQUE 冲突与越界预检，事务保证是最后的原子性防线）。
type assetPathRename struct {
	fileID  int64
	oldPath string // 替换前相对路径（执行序排序依据：嵌套前缀链式腾位按旧路径长度降序）
	newRel  string
}

func (s *assetsStore) replaceFilePaths(renames []assetPathRename, now string, folderKeyOld, folderKeyNew string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, r := range renames {
		res, err := tx.Exec(
			"UPDATE files SET path = ?, updated_at = ? WHERE id = ?", r.newRel, now, r.fileID,
		)
		if err != nil {
			return err
		}
		// 零命中 = 该行已被并发「清理丢失记录」删除（ClearLostAssetFiles 不持扫描
		// 锁，与替换存在并发窗口）：静默零操作会让 Replaced 按名册虚报计数，且
		// 事务后探测段对幽灵行 processScanFile 落 replaceFileSlides 会因 slides
		// 外键违例上抛模糊错误——此处即中断（事务整体回滚，探测段自然不再进入），
		// 以 sql.ErrNoRows 上抛，调用方转「记录已被清理」可读错误
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return sql.ErrNoRows
		}
	}
	// 文件夹标注键同事务级联改键（PRD R8/PR4：folder_meta/folder_tags 以
	// folder_path 为键，目录重命名/移动时键随路径前缀替换——与 files.path 更新
	// 同事务保证「文件路径更新 + 标注键迁移」原子，分裂则中途失败留标注孤儿键）。
	// 空串 = 调用方无文件夹键语义（防御预留），零行为变化。目录边界语义与
	// listFilesByPathPrefix 同构（恰等 + LIKE 前缀/%，LIKE 转义单点）。
	// substr 偏移必须用 SQL length(?)（字符数）而非 Go len()（字节数）：SQLite
	// substr 对 TEXT 按字符计位，非 ASCII 目录名（中文，字节长 > 字符长）用字节
	// 偏移会跳过错误数量的字符致新键截断错位（如「素材」→ 后缀丢失），故在 SQL
	// 侧对绑定参数求字符长度。
	if folderKeyOld != "" {
		if _, err := tx.Exec(
			"UPDATE folder_meta SET folder_path = ? || substr(folder_path, length(?) + 1), updated_at = ? "+
				"WHERE folder_path = ? OR folder_path LIKE ? ESCAPE '\\'",
			folderKeyNew, folderKeyOld, now, folderKeyOld, escapeLikePattern(folderKeyOld)+"/%",
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			"UPDATE folder_tags SET folder_path = ? || substr(folder_path, length(?) + 1) "+
				"WHERE folder_path = ? OR folder_path LIKE ? ESCAPE '\\'",
			folderKeyNew, folderKeyOld, folderKeyOld, escapeLikePattern(folderKeyOld)+"/%",
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// clearLostFiles 删除全部 lost 状态文件记录（「清理丢失记录」入口，等效 lost
// 引入前的扫描删除语义）：单事务逐条 FTS 行（文件行 + 幻灯片行显式删除，对齐
// deleteFilesByPaths 双保险口径）+ thumb_cache 显式删除 + files 行删除（slides/
// file_tags/slide_tags/slide_meta 随外键级联）。返回实际删除的 fileID 列表与
// 总数（调用方据 fileID 清理缩略图 PNG 与记账 dirty——文件系统操作不可回滚，
// db 先行、文件后删，对齐 deleteFilesByPaths 收口顺序）。
//
// 资格复验不变式：收集（listLostFileRecords）与事务删除之间并发扫描可把行复活
// （lost→ok，扫描复活不经本服务锁），逐条删除前须同事务复验行仍为 lost——已复活
// 行连同其标签/标注/缩略图记账全保留；行已消失（并发清理窗口）同样跳过。同事务
// 读后写在 SQLite 写事务串行化下无复验窗口。
func (s *assetsStore) clearLostFiles() ([]int64, int64, error) {
	// 先收集 lost 记录（rows 收齐显式 Close 后再发写语句，单连接池自锁纪律；
	// 事务内同理——收集在事务外完成，事务只做删除）
	records, err := s.listLostFileRecords()
	if err != nil {
		return nil, 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	deletedIDs := make([]int64, 0, len(records))
	var removed int64
	for _, rec := range records {
		// 资格复验：仅仍为 lost 的行才删（正向窗口防误删已复活行）
		var status string
		err := tx.QueryRow("SELECT status FROM files WHERE id = ?", rec.id).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			continue // 已被并发清理删除：跳过
		}
		if err != nil {
			return nil, 0, err
		}
		if status != model.AssetFileStatusLost {
			continue // 已被并发扫描复活（lost→ok）：保留该行及其全部元数据
		}
		slideIDs, err := selectSlideIDsTx(tx, rec.id)
		if err != nil {
			return nil, 0, err
		}
		for _, id := range slideIDs {
			if err := deleteFtsRowTx(tx, -id); err != nil {
				return nil, 0, err
			}
		}
		if err := deleteFtsRowTx(tx, rec.id); err != nil {
			return nil, 0, err
		}
		if _, err := tx.Exec("DELETE FROM thumb_cache WHERE file_id = ?", rec.id); err != nil {
			return nil, 0, err
		}
		// 删除语句同带 lost 资格条件（与上方复验双保险），按 RowsAffected
		// 实际命中计数——返回数语义 = 实际删除数
		res, err := tx.Exec(
			"DELETE FROM files WHERE id = ? AND status = ?", rec.id, model.AssetFileStatusLost,
		)
		if err != nil {
			return nil, 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, 0, err
		}
		if n == 0 {
			continue // 防御：资格复验已过仍零命中（理论不达），不计入
		}
		deletedIDs = append(deletedIDs, rec.id)
		removed += n
	}
	return deletedIDs, removed, tx.Commit()
}

// listLostFileRecords 全部 lost 状态记录（清理前的收集）。
func (s *assetsStore) listLostFileRecords() ([]assetFileRecord, error) {
	rows, err := s.db.Query(
		"SELECT id, path, size, mtime_ns, status, hash FROM files WHERE status = ?", model.AssetFileStatusLost,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]assetFileRecord, 0)
	for rows.Next() {
		var rec assetFileRecord
		if err := rows.Scan(&rec.id, &rec.path, &rec.size, &rec.mtimeNs, &rec.status, &rec.hash); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// libraryStats 统计：文件总数（含各状态）、幻灯片总数、失败数、加密跳过数、
// 丢失数（status=lost，SRS §2.6.5）、排除规则命中数（status=excluded，SRS
// §2.7.3）。全部走 QueryRow+Scan（自动排空结果集）：单连接池
// （SetMaxOpenConns(1)）下 rows 未关闭就发下一条查询会自锁，禁止
// Query+defer Close 后再查的写法。
func (s *assetsStore) libraryStats() (fileCount, slideCount, failedCount, encryptedCount, lostCount, excludedCount int, err error) {
	err = s.db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(status = ?), 0), COALESCE(SUM(status = ?), 0), COALESCE(SUM(status = ?), 0), COALESCE(SUM(status = ?), 0) FROM files",
		model.AssetFileStatusFailed, model.AssetFileStatusEncrypted, model.AssetFileStatusLost, model.AssetFileStatusExcluded,
	).Scan(&fileCount, &failedCount, &encryptedCount, &lostCount, &excludedCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM slides").Scan(&slideCount)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	return fileCount, slideCount, failedCount, encryptedCount, lostCount, excludedCount, nil
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

// slideAnnotKey 页级标注的复合键（slide_tags/slide_meta 主键口径，struct 作
// map 键）：锚定 (files.id, 页序) 而非 slides.id——重解析删旧插新页记录 ID 全变，
// files.id 稳定（schema v5 设计要点，见 assets_schema.go assetsSchemaV5SQL 注释）。
type slideAnnotKey struct {
	fileID   int64
	slideNum int64
}

// slideMetaAnnot 页级标注的备注/星级载荷（listSlideMetaMap 的值形态）。
type slideMetaAnnot struct {
	note   string
	rating int
}

// listSlideTagGroups 全库页自有标签分组（(file_id, slide_num) → 标签列表，按
// 标签名排序，形态对齐 listFileTagGroups）。
func (s *assetsStore) listSlideTagGroups() (map[slideAnnotKey][]model.AssetTag, error) {
	rows, err := s.db.Query(
		"SELECT st.file_id, st.slide_num, t.id, t.name, t.color, t.parent_id FROM slide_tags st " +
			"JOIN tags t ON t.id = st.tag_id ORDER BY t.name",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make(map[slideAnnotKey][]model.AssetTag)
	for rows.Next() {
		var key slideAnnotKey
		var tag model.AssetTag
		var parentID sql.NullInt64
		if err := rows.Scan(&key.fileID, &key.slideNum, &tag.ID, &tag.Name, &tag.Color, &parentID); err != nil {
			return nil, err
		}
		tag.ParentID = parentID.Int64
		groups[key] = append(groups[key], tag)
	}
	return groups, rows.Err()
}

// listSlideMetaMap 全库页级备注/星级批量（slide_meta 全表）。
func (s *assetsStore) listSlideMetaMap() (map[slideAnnotKey]slideMetaAnnot, error) {
	rows, err := s.db.Query("SELECT file_id, slide_num, note, rating FROM slide_meta")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	metas := make(map[slideAnnotKey]slideMetaAnnot)
	for rows.Next() {
		var key slideAnnotKey
		var m slideMetaAnnot
		if err := rows.Scan(&key.fileID, &key.slideNum, &m.note, &m.rating); err != nil {
			return nil, err
		}
		metas[key] = m
	}
	return metas, rows.Err()
}

// listSlideEntries 幻灯片索引条目分页（按文件路径 + 页序），标签继承归属文件；
// 页自有标注（自有标签/业务备注/星级）随条目携带（详情面板数据源，不新增查询
// 绑定）。分页契约对齐 SearchAssets：Total 为总条目数（不随页变化），翻页以
// offset 递增 limit 驱动。rows 在标签/标注查询前显式关闭：单连接池下未关先查
// 会自锁（见 libraryStats 注释）；COUNT 走 QueryRow+Scan（自动排空）先于分页
// 查询发出。
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
		e.OwnTags = []model.AssetTag{}
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

	// 三条全库批量查询顺序发出（前一条 rows 均已收口，单连接池纪律）
	tagsByFile, err := s.listFileTagGroups()
	if err != nil {
		return model.AssetSlidePage{}, err
	}
	ownTagsBySlide, err := s.listSlideTagGroups()
	if err != nil {
		return model.AssetSlidePage{}, err
	}
	metaBySlide, err := s.listSlideMetaMap()
	if err != nil {
		return model.AssetSlidePage{}, err
	}
	for i := range entries {
		entries[i].Tags = tagsByFile[entries[i].FileID]
		if entries[i].Tags == nil {
			entries[i].Tags = []model.AssetTag{}
		}
		key := slideAnnotKey{fileID: entries[i].FileID, slideNum: int64(entries[i].SlideNum)}
		entries[i].OwnTags = nonNilTags(ownTagsBySlide[key])
		if m, ok := metaBySlide[key]; ok {
			entries[i].Note = m.note
			entries[i].Rating = m.rating
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

// updateSlideMeta 幻灯片页级标注整体替换（单事务）：业务备注/星级 upsert
// slide_meta + 页自有标签集重建（按 (file_id, slide_num) 整体替换，DELETE 后
// ensureTagTx 循环 INSERT OR IGNORE，去空白去重对齐 updateFileMeta）。
//
// 纯标注层约束：不写 FTS（页业务备注不入检索，关键词行为不变）、不碰
// thumb_cache、不触发重解析——页标注生命周期独立于解析产物（SRS §2.2.4 存储层
// 独立契约）。文件或页不存在返回可读错误（对齐 updateFileMeta 形态，非 AppError）。
func (s *assetsStore) updateSlideMeta(fileID, slideNum int64, rating int, note string, tagNames []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 宿主行校验：文件行 + 页行（页标注锚定 (file_id, slide_num)，页不存在的
	// 写入属前端数据过期，引导刷新）
	var exists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM files WHERE id = ?", fileID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("素材文件不存在或已被删除（fileId=%d），请刷新列表", fileID)
	}
	if err := tx.QueryRow(
		"SELECT COUNT(*) FROM slides WHERE file_id = ? AND slide_num = ?", fileID, slideNum,
	).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("幻灯片页不存在或已被重新解析（fileId=%d，slideNum=%d），请刷新列表", fileID, slideNum)
	}

	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO slide_meta(file_id, slide_num, note, rating) VALUES(?, ?, ?, ?)",
		fileID, slideNum, note, rating,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		"DELETE FROM slide_tags WHERE file_id = ? AND slide_num = ?", fileID, slideNum,
	); err != nil {
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
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO slide_tags(file_id, slide_num, tag_id) VALUES(?, ?, ?)",
			fileID, slideNum, tagID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ensureTagTx 取标签 ID，不存在则自动创建（默认色板按标签名哈希确定性分配，
// 同名标签跨库/多次创建颜色稳定）。自动创建为根标签，sort_order 追加根组末尾
// （组内 MAX+1，PRD D8#1 兄弟序契约——避免 DEFAULT 0 插队组内首位）；已存在的
// 标签序号计算结果不使用（多一次组内 MAX 查询，标签量级百级成本可忽略）。
// 须在事务内调用。
func ensureTagTx(tx *sql.Tx, name string) (int64, error) {
	sortOrder, err := nextTagSortOrder(tx, 0)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(
		"INSERT INTO tags(name, color, parent_id, sort_order) VALUES(?, ?, NULL, ?) ON CONFLICT(name) DO NOTHING",
		name, assetTagColorFor(name), sortOrder,
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

// slideOwnTagFilterSet 标签筛选的「页自身绑定」命中集合（slides.id 视角）：
// slide_tags 中绑定任一给定标签的 (file_id, slide_num)，经 slides 唯一索引
// idx_slides_file_num 等价解析为页记录 ID（与 FTS 命中行的 refID 同口径，检索
// 过滤 O(1) 判定，无需为每个命中回查页序）。标签列表为空返回 nil（调用方跳过
// 筛选）；已消失页的残留标注（重解析失配清理前的窗口）经 JOIN slides 自然出局。
//
// 继承合并语义单点：与 listTagFilteredSlides 的 SQL 子查询同源（同两关系表、同
// 标签参数），合并规则（默认【文件绑定 ∪ 页自身绑定】/ OnlyOwn 仅页自身）统一在
// slideTagRowMatches（FTS 路径）与 listTagFilteredSlides WHERE（纯标签浏览路径）
// 表达，禁两套口径。
func (s *assetsStore) slideOwnTagFilterSet(tagIDs []int64) (map[int64]bool, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(tagIDs)), ", ")
	args := make([]any, 0, len(tagIDs))
	for _, id := range tagIDs {
		args = append(args, id)
	}
	rows, err := s.db.Query(
		"SELECT s.id FROM slide_tags st "+
			"JOIN slides s ON s.file_id = st.file_id AND s.slide_num = st.slide_num "+
			"WHERE st.tag_id IN ("+placeholders+")", args...)
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

// slideTagRowMatches 标签筛选的行级命中判定（继承合并语义单点，FTS 检索路径
// 消费）：文件行仅看归属文件绑定（不受 OnlyOwn 影响——文件条目只有 file_tags）；
// 幻灯片行默认【归属文件绑定 ∪ 页自身绑定】，OnlyOwn=true 仅页自身绑定。两个
// 集合均为 nil 表示未启用标签筛选（全放行）。
func slideTagRowMatches(onlyOwn bool, fileFilter, ownSlideFilter map[int64]bool, refType string, refID, fileID int64) bool {
	if fileFilter == nil && ownSlideFilter == nil {
		return true // 未启用标签筛选：全部命中
	}
	if refType == model.AssetRefTypeFile {
		return fileFilter == nil || fileFilter[fileID]
	}
	if ownSlideFilter != nil && ownSlideFilter[refID] {
		return true
	}
	return !onlyOwn && fileFilter != nil && fileFilter[fileID]
}

// tagFilteredFileOrdering 纯标签筛选/facets-only 路径的文件条目全量序（按路径序）：
// 仅 ok 状态且满足筛选条件。mtimeNs/rating 供 mtime/rating 排序档填充排序键
// （SRS §2.5.3，文件级口径）。
type tagFilteredFileOrdering struct {
	id      int64
	path    string
	mtimeNs int64
	rating  int64
}

// listTagFilteredFiles 纯标签浏览的文件条目序（keyword 为空时的数据源）。
// facet 非空时在标签子查询之外追加 facet 条件（同表列判定，单点构造见
// assetsFacetFilter.whereClause）；folder 非空时同层追加 folderPath 前缀条件
// （单点构造见 assetsFolderFilter.whereClause，R7 检索联动）。
func (s *assetsStore) listTagFilteredFiles(tagIDs []int64, facet *assetsFacetFilter, folder *assetsFolderFilter) ([]tagFilteredFileOrdering, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(tagIDs)), ", ")
	args := make([]any, 0, len(tagIDs))
	for _, id := range tagIDs {
		args = append(args, id)
	}
	extraClause := ""
	if facet != nil {
		var facetArgs []any
		extraClause, facetArgs = facet.whereClause("f.")
		args = append(args, facetArgs...)
	}
	if folder != nil {
		var folderArgs []any
		folderClause, folderArgs := folder.whereClause("f.path")
		extraClause += folderClause
		args = append(args, folderArgs...)
	}
	rows, err := s.db.Query(
		"SELECT f.id, f.path, f.mtime_ns, f.rating FROM files f WHERE f.status = ? AND f.id IN "+
			"(SELECT file_id FROM file_tags WHERE tag_id IN ("+placeholders+"))"+
			extraClause+" ORDER BY f.path",
		append([]any{model.AssetFileStatusOK}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]tagFilteredFileOrdering, 0)
	for rows.Next() {
		var r tagFilteredFileOrdering
		if err := rows.Scan(&r.id, &r.path, &r.mtimeNs, &r.rating); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// tagFilteredSlideOrdering 纯标签筛选的幻灯片条目全量序（按文件路径 + 页序）。
// mtimeNs/rating 取归属文件属性（JOIN files 继承口径，供 mtime/rating 排序档
// 填充排序键）。
type tagFilteredSlideOrdering struct {
	id      int64
	fileID  int64
	path    string
	num     int
	mtimeNs int64
	rating  int64
}

// listTagFilteredSlides 纯标签浏览的幻灯片条目序（标签继承合并语义：默认【归属
// 文件绑定 ∪ 页自身绑定】命中，onlyOwn=true 仅页自身绑定——与 FTS 检索路径的
// slideTagRowMatches 同一语义，SQL 子查询与 slideOwnTagFilterSet 同源）。
// onlyOwn 时文件绑定分支整段裁剪（仅页自身标签语义）。
// facet 非空时对归属文件列追加 facet 条件、folder 非空时同层追加 folderPath 前缀
// 条件（幻灯片条目按归属文件 f.path 继承判定，已 JOIN files，单点构造见
// assetsFacetFilter/assetsFolderFilter whereClause）。
func (s *assetsStore) listTagFilteredSlides(tagIDs []int64, onlyOwn bool, facet *assetsFacetFilter, folder *assetsFolderFilter) ([]tagFilteredSlideOrdering, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(tagIDs)), ", ")
	baseArgs := make([]any, 0, len(tagIDs))
	for _, id := range tagIDs {
		baseArgs = append(baseArgs, id)
	}
	extraClause := ""
	var extraArgs []any
	if facet != nil {
		var facetArgs []any
		extraClause, facetArgs = facet.whereClause("f.")
		extraArgs = append(extraArgs, facetArgs...)
	}
	if folder != nil {
		var folderArgs []any
		folderClause, folderArgs := folder.whereClause("f.path")
		extraClause += folderClause
		extraArgs = append(extraArgs, folderArgs...)
	}
	// 双子查询形态的参数序：status 占位 + 标签子查询参数 + 附加条件参数
	//（extraClause 位于两类标签子查询之后，参数须随 SQL 片段位置收尾——
	// placeholders 构造复用现有形态）
	query := "SELECT s.id, s.file_id, f.path, s.slide_num, f.mtime_ns, f.rating FROM slides s JOIN files f ON f.id = s.file_id "
	var args []any
	if onlyOwn {
		query += "WHERE f.status = ? AND (s.file_id, s.slide_num) IN " +
			"(SELECT file_id, slide_num FROM slide_tags WHERE tag_id IN (" + placeholders + "))" +
			extraClause + " ORDER BY f.path, s.slide_num"
		args = append([]any{model.AssetFileStatusOK}, baseArgs...)
		args = append(args, extraArgs...)
	} else {
		query += "WHERE f.status = ? AND (s.file_id IN " +
			"(SELECT file_id FROM file_tags WHERE tag_id IN (" + placeholders + ")) OR " +
			"(s.file_id, s.slide_num) IN " +
			"(SELECT file_id, slide_num FROM slide_tags WHERE tag_id IN (" + placeholders + ")))" +
			extraClause + " ORDER BY f.path, s.slide_num"
		args = append([]any{model.AssetFileStatusOK}, baseArgs...)
		args = append(args, baseArgs...)
		args = append(args, extraArgs...)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]tagFilteredSlideOrdering, 0)
	for rows.Next() {
		var r tagFilteredSlideOrdering
		if err := rows.Scan(&r.id, &r.fileID, &r.path, &r.num, &r.mtimeNs, &r.rating); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// assetFileTagBinding 文件标签绑定的重建备份条目（fileTagBindingsByPath 值形态）：
// hash 为该文件内容 sha256（SRS §14.16 哈希恢复，素材管理第十批 PR2——重建后
// 文件改名/移动导致路径失配时按 hash 兜底重绑）；hash 为空（加密/权限跳过文件）
// 时不参与兜底。
type assetFileTagBinding struct {
	hash   string
	tagIDs []int64
}

// fileTagBindingsByPath 重建前的用户标注备份：文件相对路径 → 标签 ID 列表 + 内容
// hash（重建清 files 会级联清 file_tags，按路径映射在重建后写回，用户标注不丢；
// hash 供路径失配时的哈希兜底匹配，见 rebindFileTagsAfterRebuild）。
func (s *assetsStore) fileTagBindingsByPath() (map[string]assetFileTagBinding, error) {
	rows, err := s.db.Query(
		"SELECT f.path, f.hash, ft.tag_id FROM file_tags ft JOIN files f ON f.id = ft.file_id ORDER BY f.path, ft.tag_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := make(map[string]assetFileTagBinding)
	for rows.Next() {
		var p, hash string
		var tagID int64
		if err := rows.Scan(&p, &hash, &tagID); err != nil {
			return nil, err
		}
		b := bindings[p]
		b.hash = hash
		b.tagIDs = append(b.tagIDs, tagID)
		bindings[p] = b
	}
	return bindings, rows.Err()
}

// clearIndexForRebuild 全量重建前置清理（单事务）：清 files（slides/file_tags/
// slide_tags/slide_meta/thumb_cache 随外键级联清除）+ FTS 全表 + 缩略图记账表
// + 页级标注两表（slide_tags/slide_meta——files 级联本会清，显式删除为对齐
// thumb_cache 先例的双保险，防 pragma 漂移静默漏清致重绑与残留行合并半态；
// 调用方已先经 slideAnnotationsByPath 备份）。FTS 清空用 DELETE FROM（standalone
// FTS5 表不支持 contentless/external content 专用的 'delete-all' 命令）。标签
// 本体（tags 表）保留——重建后经路径映射重绑。
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
	if _, err := tx.Exec("DELETE FROM slide_tags"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM slide_meta"); err != nil {
		return err
	}
	return tx.Commit()
}

// rebindFileTagsAfterRebuild 全量重建后的标签重绑（两级匹配，SRS §14.16 哈希恢复）：
// 先按「文件相对路径 → 标签 ID 列表」写回（重建后 path 相同的文件直接命中，现状
// 语义不变）；path 已消失（重建后文件改名/移动）且备份带内容 hash 时按 hash 兜底
// ——命中多个同内容文件取首个（files 按 id 升序确定性取最小 id）并告警（同内容
// 文件误绑概率低，标注语义损失小，PRD R4 定案）。返回值区分两级命中计数
// （pathRebound 路径命中 / hashRebound 哈希兜底命中，文件数口径，日志可观测）。
func (s *assetsStore) rebindFileTagsAfterRebuild(bindings map[string]assetFileTagBinding) (pathRebound, hashRebound int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	// 新 files 全量：path → id 与 hash → 首个 id（ORDER BY id 保证「取首个」
	// 确定性 = 重建后最小文件 id）。rows 收齐后显式 Close 再发写语句（单连接池
	// 自锁纪律，事务内同理）。
	rows, err := tx.Query("SELECT id, path, hash FROM files ORDER BY id")
	if err != nil {
		return 0, 0, err
	}
	pathToID := make(map[string]int64, len(bindings))
	hashToID := make(map[string]int64)
	hashDupCount := make(map[string]int)
	for rows.Next() {
		var id int64
		var p, hash string
		if err := rows.Scan(&id, &p, &hash); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		pathToID[p] = id
		if hash == "" {
			continue // 无哈希基准（加密/权限跳过类）：不参与兜底索引
		}
		if _, seen := hashToID[hash]; !seen {
			hashToID[hash] = id
			hashDupCount[hash] = 1
		} else {
			hashDupCount[hash]++
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, 0, err
	}
	for p, b := range bindings {
		fileID, ok := pathToID[p]
		if ok {
			pathRebound++
		} else {
			// path miss：hash 兜底（hash 为空 = 无基准，映射丢弃）
			if b.hash == "" {
				continue
			}
			fileID, ok = hashToID[b.hash]
			if !ok {
				continue // 重建后无同内容文件：映射丢弃
			}
			if hashDupCount[b.hash] > 1 {
				Logger().Warn("重建文件标签哈希兜底多命中，取首个（同内容多副本，标注可能归并）",
					"oldPath", p, "hash", b.hash, "candidates", hashDupCount[b.hash], "pickedFileId", fileID)
			}
			hashRebound++
		}
		for _, tagID := range b.tagIDs {
			if _, err := tx.Exec("INSERT OR IGNORE INTO file_tags(file_id, tag_id) VALUES(?, ?)", fileID, tagID); err != nil {
				return 0, 0, err
			}
		}
	}
	return pathRebound, hashRebound, tx.Commit()
}

// slideAnnotation 页级标注备份条目（slideAnnotationsByPath 的值形态）：标签 ID
// 引用 tags 表（重建清 files 不清 tags，标签本体跨重建保留，ID 稳定可直写回）。
type slideAnnotation struct {
	slideNum int64
	tagIDs   []int64
	note     string
	rating   int
}

// assetFileSlideAnnotations 单文件页级标注的重建备份条目（slideAnnotationsByPath
// 值形态）：hash 为归属文件内容 sha256（SRS §14.16 哈希恢复——路径失配时按 hash
// 兜底重绑，见 rebindSlideAnnotationsAfterRebuild），pages 为页标注列表。
type assetFileSlideAnnotations struct {
	hash  string
	pages []slideAnnotation
}

// slideAnnotationsByPath 重建前的页级标注备份：文件相对路径 → 页标注列表 + 内容
// hash（slide_tags + slide_meta 两表 JOIN files 按 path 聚合，形态对齐
// fileTagBindingsByPath）。三条查询各自收齐结果显式 Close 后再发下一条（单连接
// 池自锁纪律，见 libraryStats 注释）。
func (s *assetsStore) slideAnnotationsByPath() (map[string]assetFileSlideAnnotations, error) {
	// 1. 页自有标签绑定（顺带记录各 path 的文件内容 hash）
	tagIDsByPath := make(map[string]map[int64][]int64)
	hashByPath := make(map[string]string)
	rows, err := s.db.Query(
		"SELECT f.path, f.hash, st.slide_num, st.tag_id FROM slide_tags st JOIN files f ON f.id = st.file_id " +
			"ORDER BY f.path, st.slide_num, st.tag_id")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p, hash string
		var num, tagID int64
		if err := rows.Scan(&p, &hash, &num, &tagID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if tagIDsByPath[p] == nil {
			tagIDsByPath[p] = make(map[int64][]int64)
		}
		tagIDsByPath[p][num] = append(tagIDsByPath[p][num], tagID)
		hashByPath[p] = hash
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// 2. 页级备注/星级 + 按路径聚合（标签与元数据两表合一）
	annotations := make(map[string]assetFileSlideAnnotations)
	rows, err = s.db.Query(
		"SELECT f.path, f.hash, sm.slide_num, sm.note, sm.rating FROM slide_meta sm JOIN files f ON f.id = sm.file_id " +
			"ORDER BY f.path, sm.slide_num")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p, hash string
		var num, rating int64
		var note string
		if err := rows.Scan(&p, &hash, &num, &note, &rating); err != nil {
			return nil, err
		}
		fa := annotations[p]
		fa.hash = hash
		fa.pages = append(fa.pages, slideAnnotation{
			slideNum: num,
			note:     note,
			rating:   int(rating),
			tagIDs:   tagIDsByPath[p][num],
		})
		annotations[p] = fa
		hashByPath[p] = hash
		delete(tagIDsByPath[p], num)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 仅有标签无备注/星级的页（slide_meta 无行）：补占位条目
	for p, byNum := range tagIDsByPath {
		for num, tagIDs := range byNum {
			fa := annotations[p]
			fa.hash = hashByPath[p]
			fa.pages = append(fa.pages, slideAnnotation{slideNum: num, tagIDs: tagIDs})
			annotations[p] = fa
		}
	}
	return annotations, nil
}

// rebindSlideAnnotationsAfterRebuild 全量重建后的页级标注重绑（两级匹配，SRS
// §14.16 哈希恢复）：先按「文件相对路径 + 页序」映射写回（slide_meta INSERT OR
// REPLACE + slide_tags INSERT OR IGNORE，单事务；path 未变且页在新 slides 中存在
// 才写回，页已消失的标注静默丢弃——位置语义，对齐缩略图缓存键口径）；文件级
// path miss（重建后改名/移动）且备份带内容 hash 时按 hash 兜底定位新文件再按
// 页序映射，hash 命中多个同内容文件取首个（files 按 id 升序取最小 id）并告警。
// 返回值区分两级命中计数（pathRebound / hashRebound，页数口径）。
//
// 多旧绑定同 hash 的确定性收敛（素材审核 2026-10-18）：两个旧文件内容相同且重建后
// 双双改名（均 path miss）时，全部页标注落同一新文件——slide_meta INSERT OR
// REPLACE 后写覆盖，若按 map 遍历序处理则胜负跨运行不确定。收敛两点：①旧路径
// 字典序升序处理（字典序后者胜，note/rating 后写覆盖；标签为 INSERT OR IGNORE
// 并集语义无冲突，仅 note/rating 有胜负）；②任一 hash 组被多个旧绑定兜底命中时
// slog.Warn 同源路径列表（组级统一告警，不区分组内是否有冲突页——告警语义是
// 「标注可能归并丢失」，与命中页是否重叠无关）。
func (s *assetsStore) rebindSlideAnnotationsAfterRebuild(annotations map[string]assetFileSlideAnnotations) (pathRebound, hashRebound int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. 新 files 全量：hash → 首个 path（ORDER BY id 保证「取首个」确定性；
	// 不取 id 列——旧实现顺带维护的 path→id 映射已删，页级落位经页集合的 file_id）。
	// rows 收齐后显式 Close 再发下一条（单连接池自锁纪律，事务内同理）。
	rows, err := tx.Query("SELECT path, hash FROM files ORDER BY id")
	if err != nil {
		return 0, 0, err
	}
	hashToPath := make(map[string]string)
	hashDupCount := make(map[string]int)
	for rows.Next() {
		var p, hash string
		if err := rows.Scan(&p, &hash); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		if hash == "" {
			continue // 无哈希基准：不参与兜底索引
		}
		if _, seen := hashToPath[hash]; !seen {
			hashToPath[hash] = p
			hashDupCount[hash] = 1
		} else {
			hashDupCount[hash]++
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, 0, err
	}

	// 2. 新 slides 现存页集合：path → (slide_num → file_id)。
	rows, err = tx.Query(
		"SELECT f.path, s.slide_num, s.file_id FROM slides s JOIN files f ON f.id = s.file_id " +
			"ORDER BY f.path, s.slide_num")
	if err != nil {
		return 0, 0, err
	}
	pagesByPath := make(map[string]map[int64]int64)
	for rows.Next() {
		var p string
		var num, fileID int64
		if err := rows.Scan(&p, &num, &fileID); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		if pagesByPath[p] == nil {
			pagesByPath[p] = make(map[int64]int64)
		}
		pagesByPath[p][num] = fileID
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, 0, err
	}

	// 3. 旧绑定解析（确定性：旧路径字典序升序），同 hash 多旧绑定聚组供组级告警。
	sortedPaths := make([]string, 0, len(annotations))
	for p := range annotations {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)
	type resolvedAnnot struct {
		pages  map[int64]int64
		byHash bool
	}
	resolved := make(map[string]resolvedAnnot, len(sortedPaths))
	pathsByHash := make(map[string][]string) // hash 兜底命中的旧路径按 hash 聚组（告警面）
	for _, p := range sortedPaths {
		fa := annotations[p]
		pages, ok := pagesByPath[p]
		if ok {
			resolved[p] = resolvedAnnot{pages: pages}
			continue
		}
		// 文件级 path miss：hash 兜底定位新文件（hash 为空或无同内容文件则整
		// 文件页标注丢弃）
		if fa.hash == "" {
			continue
		}
		matchedPath, ok := hashToPath[fa.hash]
		if !ok {
			continue
		}
		if hashDupCount[fa.hash] > 1 {
			Logger().Warn("重建页级标注哈希兜底多命中，取首个（同内容多副本，标注可能归并）",
				"oldPath", p, "hash", fa.hash, "candidates", hashDupCount[fa.hash], "newPath", matchedPath)
		}
		if len(pagesByPath[matchedPath]) == 0 {
			continue // 同内容新文件无页产物（解析中断半态）：标注丢弃
		}
		resolved[p] = resolvedAnnot{pages: pagesByPath[matchedPath], byHash: true}
		pathsByHash[fa.hash] = append(pathsByHash[fa.hash], p)
	}
	for hash, paths := range pathsByHash {
		if len(paths) > 1 {
			Logger().Warn("重建页级标注多个旧绑定同哈希兜底落同一新文件，按旧路径字典序后者胜（note/rating 后写覆盖，标签并集）",
				"hash", hash, "sourcePaths", paths)
		}
	}

	// 4. 按解析结果写入（旧路径字典序处理，同 (file_id, slide_num) 冲突后者胜）。
	for _, p := range sortedPaths {
		rb, ok := resolved[p]
		if !ok {
			continue
		}
		for _, a := range annotations[p].pages {
			fileID, ok := rb.pages[a.slideNum]
			if !ok {
				continue // 页已消失（重解析页数减少）：该页标注静默丢弃
			}
			if _, err := tx.Exec(
				"INSERT OR REPLACE INTO slide_meta(file_id, slide_num, note, rating) VALUES(?, ?, ?, ?)",
				fileID, a.slideNum, a.note, a.rating,
			); err != nil {
				return 0, 0, err
			}
			for _, tagID := range a.tagIDs {
				if _, err := tx.Exec(
					"INSERT OR IGNORE INTO slide_tags(file_id, slide_num, tag_id) VALUES(?, ?, ?)",
					fileID, a.slideNum, tagID,
				); err != nil {
					return 0, 0, err
				}
			}
			if rb.byHash {
				hashRebound++
			} else {
				pathRebound++
			}
		}
	}
	return pathRebound, hashRebound, tx.Commit()
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

// splitRecordExistsByPathPrefix 目录前缀下是否存在拆分记录（文件夹重命名守卫
// 判定用，PRD PR2 宁严勿错）：source_rel_path 恰等于前缀或以「前缀 + /」开头，
// 目录边界语义与 listFilesByPathPrefix 同构（LIKE 转义单点 escapeLikePattern）。
func (s *assetsStore) splitRecordExistsByPathPrefix(relDir string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM split_records WHERE source_rel_path = ? OR source_rel_path LIKE ? ESCAPE '\\'",
		relDir, escapeLikePattern(relDir)+"/%",
	).Scan(&n)
	return n > 0, err
}

// deleteSplitRecordsByPathPrefix 删除目录前缀下全部拆分记录并返回删除行数
// （文件夹删除后的索引收口：源文件物理已删，记录保留会以陈旧哈希误判复拆增量
// 跳过；派生缓存目录产物成孤儿无害，随库删除统一收口）。目录边界语义与
// splitRecordExistsByPathPrefix 同构（同一 WHERE 构造，禁两套口径）。
func (s *assetsStore) deleteSplitRecordsByPathPrefix(relDir string) (int64, error) {
	res, err := s.db.Exec(
		"DELETE FROM split_records WHERE source_rel_path = ? OR source_rel_path LIKE ? ESCAPE '\\'",
		relDir, escapeLikePattern(relDir)+"/%",
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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

// ---------- 文件夹维度筛选（PRD R7 检索联动，素材管理 PR2） ----------

// assetsFolderFilter folderPath 维度筛选条件（PRD R7：文件夹为独立过滤维度，与
// 关键词/标签/facet AND 叠加）。与 assetsFacetFilter 同层单点：检索三路径（keyword
// 批量判定 / tagsOnly 两查询 / facets-only 数据源）全部消费 whereClause 的同一
// 构造，禁在单一路径内自行拼条件（口径分叉 = 判定不一致）。
//
// 匹配语义 = 目录边界前缀（col 恰等于前缀，或以「前缀 + /」开头）：`a` 命中
// `a` 与 `a/...`，不误伤 `ab/...` 同头缀兄弟目录（LIKE 转义单点 escapeLikePattern，
// 与 listFilesByPathPrefix 同构）。幻灯片条目按归属文件 path 判定（JOIN files 的
// f.path 列，继承口径与 facet 一致）。
type assetsFolderFilter struct {
	prefix string // 库内相对目录前缀（正斜杠，不含尾斜杠）
}

// newAssetsFolderFilter folderPath 入参归一与单点提取（searchAssetsInStore 调用，
// 与 newAssetsFacetFilter 同层）：反斜杠统一正斜杠、修剪首尾斜杠与空白；空串
// = 全库不筛选（返回 nil，全部路径零行为变化）。
func newAssetsFolderFilter(raw string) *assetsFolderFilter {
	p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return &assetsFolderFilter{prefix: p}
}

// whereClause folder 条件 SQL 片段构造（单点）：col 为完整列名（join 语境带表
// 别名，如 "f.path"；无别名查询传 "path"）。返回追加在 WHERE 后的片段（已带
// AND 前导，恒非空——仅非 nil 时调用）与按序参数。
func (f *assetsFolderFilter) whereClause(col string) (string, []any) {
	escaped := escapeLikePattern(f.prefix)
	return " AND (" + col + " = ? OR " + col + " LIKE ? ESCAPE '\\')",
		[]any{f.prefix, escaped + "/%"}
}

// ---------- 文件夹级标注（PRD R8，文件夹树 PR4） ----------

// getFolderMeta 读文件夹标注（QueryRow+Scan 单行 + 标签 JOIN 两查询各自收齐后
// 再发下一条，单连接池自锁纪律）。无标注行 = 零值返回（note 空、Tags 空切片，
// 前端免判空）；标签绑定行 JOIN tags 取本体（ID 口径与文件/页标签同源）。
func (s *assetsStore) getFolderMeta(folderPath string) (note, updatedAt string, tags []model.AssetTag, err error) {
	tags = []model.AssetTag{}
	err = s.db.QueryRow(
		"SELECT note, updated_at FROM folder_meta WHERE folder_path = ?", folderPath,
	).Scan(&note, &updatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", tags, err
	}
	rows, err := s.db.Query(
		"SELECT t.id, t.name, t.color, COALESCE(t.parent_id, 0) FROM folder_tags ft "+
			"JOIN tags t ON t.id = ft.tag_id WHERE ft.folder_path = ? ORDER BY t.name", folderPath,
	)
	if err != nil {
		return "", "", tags, err
	}
	defer rows.Close()
	for rows.Next() {
		var tag model.AssetTag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Color, &tag.ParentID); err != nil {
			return "", "", tags, err
		}
		tags = append(tags, tag)
	}
	return note, updatedAt, tags, rows.Err()
}

// updateFolderMeta 文件夹标注整体替换（单事务，对齐 updateFileMeta 形态）：
// upsert folder_meta + folder_tags 整体替换（DELETE 后 ensureTagTx 循环
// INSERT OR IGNORE，去空白去重同款）；不存在的标签自动创建（对齐文件/页级标注）。
// 纯标注层：不入 FTS（对齐页级口径）、不触碰缩略图与解析链路。
func (s *assetsStore) updateFolderMeta(folderPath, note string, tagNames []string, now string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		"INSERT INTO folder_meta(folder_path, note, updated_at) VALUES(?, ?, ?) "+
			"ON CONFLICT(folder_path) DO UPDATE SET note = excluded.note, updated_at = excluded.updated_at",
		folderPath, note, now,
	); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM folder_tags WHERE folder_path = ?", folderPath); err != nil {
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
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO folder_tags(folder_path, tag_id) VALUES(?, ?)", folderPath, tagID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// deleteFolderMetaByPathPrefix 删除目录前缀下的文件夹标注（meta + 标签绑定，
// DeleteAssetFolder 级联收口：目录物理已删，标注保留成孤儿键）。目录边界语义
// 与 splitRecordExistsByPathPrefix 同构（恰等 + LIKE 前缀/%）。
func (s *assetsStore) deleteFolderMetaByPathPrefix(relDir string) error {
	for _, table := range []string{"folder_meta", "folder_tags"} {
		if _, err := s.db.Exec(
			"DELETE FROM "+table+" WHERE folder_path = ? OR folder_path LIKE ? ESCAPE '\\'",
			relDir, escapeLikePattern(relDir)+"/%",
		); err != nil {
			return err
		}
	}
	return nil
}

// ---------- facet 多维筛选（SRS §2.5，文件维度 + 幻灯片条目继承归属文件属性） ----------

// assetsFacetFilter facet 三维筛选条件（星级/修改时间/文件大小，文件级口径）。
// 单点持有条件语义与 SQL 片段构造：keyword 路径（经 filterFileIDsByFacet 批量
// 判定 FTS 命中文件集合）、tagsOnly 路径（listTagFilteredFiles/listTagFilteredSlides
// SQL 追加）、facets-only 路径（listFacetFilteredFiles）三条 SQL 路径全部消费
// whereClause 的同一构造，禁在单一路径内自行拼条件（口径分叉 = 判定不一致）。
//
// 继承规则（SRS §2.5 D1）：三 facet 均为文件维度——文件条目按自身属性判定，
// 幻灯片条目按归属文件属性判定（页级评分 slide_meta.rating 纯展示不参与筛选，
// 与修改时间/文件大小口径一致）。
type assetsFacetFilter struct {
	ratings   []int64 // nil = 不筛选；含 0 = 未评分（rating=0），非零值等值匹配
	mtimeFrom int64   // 纳秒时间戳，0 = 不设下界
	mtimeTo   int64   // 纳秒时间戳，0 = 不设上界
	sizeFrom  int64   // 字节，0 = 不设下界
	sizeTo    int64   // 字节，0 = 不设上界
}

// newAssetsFacetFilter 从检索入参提取 facet 条件；未启用任何 facet 返回 nil
// （调用方零行为变化，SQL 不追加条件）。
func newAssetsFacetFilter(opts model.AssetSearchOptions) *assetsFacetFilter {
	f := &assetsFacetFilter{
		ratings:   opts.Ratings,
		mtimeFrom: opts.MtimeFrom,
		mtimeTo:   opts.MtimeTo,
		sizeFrom:  opts.SizeFrom,
		sizeTo:    opts.SizeTo,
	}
	if !f.active() {
		return nil
	}
	return f
}

// active 判定是否启用了任一 facet 条件（Ratings 空切片与 nil 同义 = 不筛选）。
func (f *assetsFacetFilter) active() bool {
	return f != nil && (len(f.ratings) > 0 || f.mtimeFrom != 0 || f.mtimeTo != 0 || f.sizeFrom != 0 || f.sizeTo != 0)
}

// whereClause facet 条件 SQL 片段构造（单点）：colPrefix 为列限定前缀（join
// 语境的表别名，如 "f."；无别名查询传 ""）。返回追加在 WHERE 后的片段（已带
// AND 前导，恒非空——仅 active 时调用）与按序参数。Rating 空切片不产生条件，
// 含 0 走 rating=0 分支（未评分语义）。
func (f *assetsFacetFilter) whereClause(colPrefix string) (string, []any) {
	var b strings.Builder
	var args []any
	if len(f.ratings) > 0 {
		nonZero := make([]int64, 0, len(f.ratings))
		hasZero := false
		for _, r := range f.ratings {
			if r == 0 {
				hasZero = true
			} else {
				nonZero = append(nonZero, r)
			}
		}
		switch {
		case hasZero && len(nonZero) == 0:
			b.WriteString(" AND " + colPrefix + "rating = 0")
		case hasZero:
			b.WriteString(" AND (" + colPrefix + "rating = 0 OR " + colPrefix + "rating IN (" +
				strings.TrimSuffix(strings.Repeat("?, ", len(nonZero)), ", ") + "))")
			for _, r := range nonZero {
				args = append(args, r)
			}
		default:
			b.WriteString(" AND " + colPrefix + "rating IN (" +
				strings.TrimSuffix(strings.Repeat("?, ", len(nonZero)), ", ") + ")")
			for _, r := range nonZero {
				args = append(args, r)
			}
		}
	}
	if f.mtimeFrom != 0 {
		b.WriteString(" AND " + colPrefix + "mtime_ns >= ?")
		args = append(args, f.mtimeFrom)
	}
	if f.mtimeTo != 0 {
		b.WriteString(" AND " + colPrefix + "mtime_ns <= ?")
		args = append(args, f.mtimeTo)
	}
	if f.sizeFrom != 0 {
		b.WriteString(" AND " + colPrefix + "size >= ?")
		args = append(args, f.sizeFrom)
	}
	if f.sizeTo != 0 {
		b.WriteString(" AND " + colPrefix + "size <= ?")
		args = append(args, f.sizeTo)
	}
	return b.String(), args
}

// listFacetFilteredFiles facets-only 路径数据源（无关键词无标签但有 facet 或
// folderPath 条件）：按 facet 条件列 ok 状态文件（文件维度筛选天然是文件清单
// 语义，无幻灯片条目、无聚合折叠），按路径序返回。mtimeNs/rating 供 mtime/rating
// 排序档填充排序键；folder 非空时同层追加 folderPath 前缀条件（单点构造见
// assetsFolderFilter.whereClause，R7 检索联动）。
func (s *assetsStore) listFacetFilteredFiles(facet *assetsFacetFilter, folder *assetsFolderFilter) ([]tagFilteredFileOrdering, error) {
	clause, clauseArgs := "", []any(nil)
	if facet != nil {
		clause, clauseArgs = facet.whereClause("f.")
	}
	if folder != nil {
		folderClause, folderArgs := folder.whereClause("f.path")
		clause += folderClause
		clauseArgs = append(clauseArgs, folderArgs...)
	}
	rows, err := s.db.Query(
		"SELECT f.id, f.path, f.mtime_ns, f.rating FROM files f WHERE f.status = ?"+clause+" ORDER BY f.path",
		append([]any{model.AssetFileStatusOK}, clauseArgs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]tagFilteredFileOrdering, 0)
	for rows.Next() {
		var r tagFilteredFileOrdering
		if err := rows.Scan(&r.id, &r.path, &r.mtimeNs, &r.rating); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// filterFileIDsByFacet 批量判定文件 ID 集合的 facet 命中（keyword 路径消费）：
// FTS 命中行携带的 fileID 全集一次 SQL 判定（WHERE id IN + facet 条件），返回
// 通过 facet 的文件 ID 集合；FTS 命中循环按行 fileID 查集合 O(1) 判定（幻灯片
// 条目按归属文件 ID 继承判定），避免逐命中回查的 N+1。分批处理 FTS 大命中量
// （对齐 fileMetaByIDs 的 assetsQueryIDBatch 批量口径）。ID 列表为空直接返回
// 空集合。已消失文件的 ID 不在返回集合中（并发删除窗口的条目自然出局）。
func (s *assetsStore) filterFileIDsByFacet(ids []int64, facet *assetsFacetFilter) (map[int64]bool, error) {
	out := make(map[int64]bool)
	for start := 0; start < len(ids); start += assetsQueryIDBatch {
		end := start + assetsQueryIDBatch
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		clause, facetArgs := facet.whereClause("f.")
		placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(batch)), ", ")
		args := make([]any, 0, len(batch)+len(facetArgs))
		for _, id := range batch {
			args = append(args, id)
		}
		args = append(args, facetArgs...)
		rows, err := s.db.Query(
			"SELECT f.id FROM files f WHERE f.id IN ("+placeholders+")"+clause, args...)
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					return err
				}
				out[id] = true
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

// filterFileIDsByFolder 批量判定文件 ID 集合的 folderPath 前缀命中（keyword 路径
// 消费，R7 检索联动）：FTS 命中行携带的 fileID 全集一次 SQL 判定（WHERE id IN +
// 目录边界前缀条件），返回通过 folder 过滤的文件 ID 集合；命中循环按行 fileID 查
// 集合 O(1) 判定（幻灯片条目按归属文件 ID 继承判定），避免逐命中回查的 N+1。
// 形态对齐 filterFileIDsByFacet（分批 assetsQueryIDBatch 防 IN 参数过量）。ID 列表
// 为空直接返回空集合；已消失文件的 ID 不在返回集合中（并发删除窗口的条目自然出局）。
func (s *assetsStore) filterFileIDsByFolder(ids []int64, folder *assetsFolderFilter) (map[int64]bool, error) {
	out := make(map[int64]bool)
	if folder == nil {
		for _, id := range ids {
			out[id] = true
		}
		return out, nil
	}
	clause, folderArgs := folder.whereClause("path")
	for start := 0; start < len(ids); start += assetsQueryIDBatch {
		end := start + assetsQueryIDBatch
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(batch)), ", ")
		args := make([]any, 0, len(batch)+len(folderArgs))
		for _, id := range batch {
			args = append(args, id)
		}
		args = append(args, folderArgs...)
		rows, err := s.db.Query(
			"SELECT id FROM files WHERE id IN ("+placeholders+")"+clause, args...)
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					return err
				}
				out[id] = true
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

// excludedFileIDSet 批量判定文件 ID 集合中处于 excluded 状态的子集（关键词检索
// 路径消费：excluded 文件 FTS 行保留——无损保留语义，解除排除后免重解析即可恢复
// 检索——但检索入口不可见，SRS §2.7.3；命中行按 fileID 查集合 O(1) 剔除，形态
// 对齐 filterFileIDsByFacet 的批量预取，分批防 IN 参数过量）。ID 列表为空返回
// 空集合；已消失文件的 ID 不在返回集合中。
func (s *assetsStore) excludedFileIDSet(ids []int64) (map[int64]bool, error) {
	out := make(map[int64]bool)
	for start := 0; start < len(ids); start += assetsQueryIDBatch {
		end := start + assetsQueryIDBatch
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?, ", end-start), ", ")
		args := make([]any, 0, end-start+1)
		args = append(args, model.AssetFileStatusExcluded)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		rows, err := s.db.Query(
			"SELECT id FROM files WHERE status = ? AND id IN ("+placeholders+")", args...)
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					return err
				}
				out[id] = true
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

// assetFileSortProps 排序档的文件属性（mtime 排序档消费 mtimeNs，rating 排序档
// 消费 rating；文件级口径单一数据源，幻灯片/聚合条目经 fileID 继承）。
type assetFileSortProps struct {
	mtimeNs int64
	rating  int64
}

// fileSortPropsByIDs 批量取文件排序属性（关键词路径 mtime/rating 排序档消费：
// FTS 命中 fileID 全集一次 IN 查询防逐命中回查 N+1，分批对齐 assetsQueryIDBatch）。
// 返回 map 缺键 = 文件已不存在（并发删除窗口：排序按零值参与，补全层该条目跳过）。
func (s *assetsStore) fileSortPropsByIDs(ids []int64) (map[int64]assetFileSortProps, error) {
	out := make(map[int64]assetFileSortProps, len(ids))
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
			"SELECT id, mtime_ns, rating FROM files WHERE id IN ("+placeholders+")", args...)
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer rows.Close()
			for rows.Next() {
				var id, mtimeNs, rating int64
				if err := rows.Scan(&id, &mtimeNs, &rating); err != nil {
					return err
				}
				out[id] = assetFileSortProps{mtimeNs: mtimeNs, rating: rating}
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
