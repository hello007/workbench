package service

import (
	"os"
	"sync"
	"time"
)

// 素材缩略图记账 ledger（LRU 记账 / 失效联动 / 配额淘汰，任务 10-14-ppt-com PR2）。
//
// 职责与契约（docs/spec/assets-thumbs-com.md 缓存键契约 + research/lru-thumbnail-cache.md
// §1.2/§5/§6）：缩略图生成与读取的记账（bytes/last_access_ns）先写内存 dirty buffer，
// 攒批（条数阈值或时间间隔）后单事务落盘 thumb_cache 表，后台惰性按档位配额 LRU
// 淘汰。设计动机：单连接池（SetMaxOpenConns(1)）下逐条 UPDATE 是独立事务，会与
// 检索/扫描查询轮流抢唯一连接，画廊一屏滚动 50 张逐条落盘将直接抖动检索延迟——
// 全部落盘走单事务批量（硬约束，禁改回逐条写）。
//
// 缓存键 = files.id + tier + page（slides.id 每轮重解析全变禁作键）；PNG 文件路径
// 经 thumbPNGPath 单点函数，记账反查、读取、淘汰同规则。

// thumbCacheKey thumb_cache 表主键（file_id + tier + page）。
type thumbCacheKey struct {
	fileID int64
	tier   string
	page   int
}

// thumbCacheEntry 记账条目：PNG 字节数与最近访问时间（纳秒，应用层自记——NTFS
// atime 落盘延迟最多 1 小时且受系统策略影响，不可作 LRU 持久化依据）。
type thumbCacheEntry struct {
	bytes        int64
	lastAccessNs int64
}

// thumbLedgerFlushThreshold 单库 dirty 条数阈值：达到即非阻塞通知 flusher 落盘
// （dirty 无界增长由本阈值约束，单库内存占用 ≤ 阈值条级）。
const thumbLedgerFlushThreshold = 200

// thumbLedgerFlushInterval flusher 周期（var 便于测试注入；ticker 周期不实测，
// 测试直接调 flushLibrary 同步断言——test-stability 契约禁 sleep 等待竞态）。
var thumbLedgerFlushInterval = 45 * time.Second

// thumbTierQuotaBytes 单档位缓存配额（small/hd 各 1GB，PR3 起设置链路经 ledger
// 的 quotaFn 注入生效；var 保留作 quotaFn 未注入时的回退与测试注入点）。
var thumbTierQuotaBytes int64 = 1 << 30

// thumbEvictWatermark 淘汰水位：超配额时删至配额的 90%（留余量防抖动——每轮
// flush 都触发淘汰的临界震荡）。
const thumbEvictWatermark = 0.9

// thumbCacheLedger 缩略图记账 ledger（全局单实例，挂 AssetLibraryService 生命周期）。
//
// 并发模型：record/invalidateFile/dropLibrary 只持 mu 写内存（微秒级，读路径
// 零磁盘写）；flushLibrary/evictTier/pruneBeyond 的 db 与文件操作在锁外执行
// （仅快照时持 mu），防长事务阻塞 record 调用方（画廊读取路径）。
type thumbCacheLedger struct {
	mu      sync.Mutex
	dirty   map[string]map[thumbCacheKey]thumbCacheEntry // libraryID -> key -> entry
	storeFn func(libraryID string) (*assetsStore, error) // 取库连接（storeForThumbs；库不存在返回错误）
	dirFn   func(libraryID string) string                // 取库缩略图根目录（assetThumbsDir）
	quotaFn func(tier string) int64                      // 档位配额读取口（设置注入；nil 回退 thumbTierQuotaBytes）
	notify  chan struct{}                                // 缓冲 1 去抖信号：攒批满时通知 flusher（重复通知合并）

	quit    chan struct{} // Shutdown 关闭，flusher 退出
	wg      sync.WaitGroup
	started bool
	stopped bool // Shutdown 置位：此后 record 丢弃（进程退出路径，防 flusher 重启）
}

// newThumbCacheLedger 构造 ledger（storeFn/dirFn/quotaFn 由宿主
// AssetLibraryService 注入；quotaFn nil 回退包级 thumbTierQuotaBytes）。
func newThumbCacheLedger(storeFn func(libraryID string) (*assetsStore, error), dirFn func(libraryID string) string, quotaFn func(tier string) int64) *thumbCacheLedger {
	return &thumbCacheLedger{
		dirty:   make(map[string]map[thumbCacheKey]thumbCacheEntry),
		storeFn: storeFn,
		dirFn:   dirFn,
		quotaFn: quotaFn,
		notify:  make(chan struct{}, 1),
		quit:    make(chan struct{}),
	}
}

// quotaFor 档位配额字节：quotaFn 注入优先（设置链路），nil 回退包级 var
// （单测回退与测试注入点）。
func (l *thumbCacheLedger) quotaFor(tier string) int64 {
	if l.quotaFn != nil {
		return l.quotaFn(tier)
	}
	return thumbTierQuotaBytes
}

// start 惰性启动 flusher goroutine（首次 record 触发——对齐 thumbQueue.start 的
// 惰性模式：常驻 goroutine 为结构内在资源，构造保持纯 new，app-services-assembly
// 契约「构造纯 new，生命周期副作用留 startup/shutdown」的折中）。
func (l *thumbCacheLedger) start() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.started || l.stopped {
		return
	}
	l.started = true
	l.wg.Add(1)
	go l.run()
}

// run flusher 主循环：周期或去抖信号触发时快照有 dirty 的库列表逐库落盘。
func (l *thumbCacheLedger) run() {
	defer l.wg.Done()
	ticker := time.NewTicker(thumbLedgerFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.notify:
		case <-ticker.C:
		case <-l.quit:
			return
		}
		for _, id := range l.dirtyLibraryIDs() {
			l.flushLibrary(id)
		}
	}
}

// record 记账一条缩略图缓存（生成后统计 PNG 体积 / 读取命中时 touch LRU 新鲜度）。
// 仅写内存 dirty，攒批落盘零同步写（写放大治理，research §5）。
func (l *thumbCacheLedger) record(libraryID string, fileID int64, tier string, page int, bytes int64) {
	l.start()
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return // Shutdown 后：进程退出路径，丢弃防内存残留
	}
	byKey, ok := l.dirty[libraryID]
	if !ok {
		byKey = make(map[thumbCacheKey]thumbCacheEntry)
		l.dirty[libraryID] = byKey
	}
	byKey[thumbCacheKey{fileID: fileID, tier: tier, page: page}] = thumbCacheEntry{
		bytes:        bytes,
		lastAccessNs: time.Now().UnixNano(),
	}
	needFlush := len(byKey) >= thumbLedgerFlushThreshold
	l.mu.Unlock()
	if needFlush {
		select {
		case l.notify <- struct{}{}:
		default: // 已有未消费信号：合并，flusher 会一并处理
		}
	}
}

// invalidateFile 清除内存 dirty 中指定文件的全部记账条目（失效联动：重解析/删除
// 文件时防 dirty 复活已删行）。flush 快照与清理的竞态窗口可能残留幽灵行，无害：
// 同 key 会被重导出的 record 覆盖、指向已删文件时读取 API 不查 db、淘汰路径
// os.Remove 对 NotExist 忽略。
func (l *thumbCacheLedger) invalidateFile(libraryID string, fileID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k := range l.dirty[libraryID] {
		if k.fileID == fileID {
			delete(l.dirty[libraryID], k)
		}
	}
}

// dropLibrary 清除指定库的全部 dirty（删库联动：库已删，落盘无意义且会经
// storeFn 惰性重建孤儿 db——双保险之一，storeForThumbs 的存在性校验为另一道）。
func (l *thumbCacheLedger) dropLibrary(libraryID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.dirty, libraryID)
}

// dirtyLibraryIDs 快照有 dirty 条目的库列表（flusher 周期触发用）。
func (l *thumbCacheLedger) dirtyLibraryIDs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids := make([]string, 0, len(l.dirty))
	for id, m := range l.dirty {
		if len(m) > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// flushLibrary 将指定库的 dirty 记账单事务落盘（快照 dirty → 持 mu 取出后释放 →
// 锁外执行 db 与文件操作）。落盘成功后对本批涉及的档位做一次配额检查（惰性
// 淘汰：只在后台 flush 路径发生，禁在 record/flush 写入路径同步淘汰——research §2）。
// 库不可用（已删除等）时丢弃该库 dirty 仅告警：防 flusher 在删库后经 storeFn
// 惰性重建孤儿 db（storeForThumbs 的存在性校验为第一道，本处丢弃为收口）。
func (l *thumbCacheLedger) flushLibrary(libraryID string) {
	l.mu.Lock()
	byKey, ok := l.dirty[libraryID]
	if !ok || len(byKey) == 0 {
		l.mu.Unlock()
		return
	}
	entries := make(map[thumbCacheKey]thumbCacheEntry, len(byKey))
	for k, e := range byKey {
		entries[k] = e
	}
	delete(l.dirty, libraryID)
	l.mu.Unlock()

	store, err := l.storeFn(libraryID)
	if err != nil {
		Logger().Warn("缩略图记账落盘跳过（库不可用，丢弃攒批）", "libraryId", libraryID, "err", err)
		return
	}
	persisted, err := store.flushThumbEntries(entries)
	if err != nil {
		Logger().Warn("缩略图记账落盘失败", "libraryId", libraryID, "entries", len(entries), "err", err)
		// flush 瞬态失败（busy_timeout 耗尽/IO 瞬态）：整批回灌 dirty 重试下轮
		// flush，不直接丢弃（丢弃会令 PNG 在而记账永久缺失，SUM 低估致配额淘汰
		// 长期失准——独立审核遗留项 1）
		l.requeueDirty(libraryID, entries)
		return
	}
	if dropped := int64(len(entries)) - persisted; dropped > 0 {
		// 幽灵键被 WHERE EXISTS 过滤（file_id 已不在 files 表——失效联动删 PNG
		// 失败后的 stale touch / 快照竞态 / 重建在途批次旧 ID）：记账缺失由读取
		// 路径 touch 自愈，仅告警登记
		Logger().Warn("缩略图记账含幽灵键已过滤", "libraryId", libraryID, "dropped", dropped)
	}

	// 本批涉及的档位逐个做配额检查（超配额 LRU 淘汰至水位）
	thumbsDir := l.dirFn(libraryID)
	seen := make(map[string]bool, 2)
	for k := range entries {
		if seen[k.tier] {
			continue
		}
		seen[k.tier] = true
		l.evictTier(store, libraryID, thumbsDir, k.tier)
	}
}

// requeueDirty flush 瞬态失败批回灌（独立审核遗留项 1）：storeFn 成功但落盘失败
// （busy_timeout 耗尽/IO 瞬态）时整批 dirty 已被快照取出，直接丢弃会令 PNG 在而
// 记账永久缺失，SUM 低估致配额淘汰长期失准。回灌仅在未 stopped 时进行；flush
// 窗口内同键新 record（更新鲜）优先，回灌不覆盖已存在的键。
func (l *thumbCacheLedger) requeueDirty(libraryID string, entries map[thumbCacheKey]thumbCacheEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return
	}
	byKey, ok := l.dirty[libraryID]
	if !ok {
		byKey = make(map[thumbCacheKey]thumbCacheEntry)
		l.dirty[libraryID] = byKey
	}
	for k, e := range entries {
		if _, exists := byKey[k]; !exists {
			byKey[k] = e
		}
	}
}

// evictTier 档位配额 LRU 淘汰：SUM 超配额时按 last_access_ns 最旧优先游标收集
// 待删条目，删至配额水位（90%）止；单事务批量 DELETE 记账 + 删 PNG 文件
// （NotExist 忽略——记账与文件的删除非原子，文件已消失属正常形态）。
// 仅由 flushLibrary / checkQuota 后台路径调用；淘汰只在此发生，禁在写入路径同步淘汰。
func (l *thumbCacheLedger) evictTier(store *assetsStore, libraryID, thumbsDir, tierKey string) {
	quota := l.quotaFor(tierKey)
	if quota <= 0 {
		return
	}
	var total int64
	if err := store.db.QueryRow(
		"SELECT COALESCE(SUM(bytes), 0) FROM thumb_cache WHERE tier = ?", tierKey,
	).Scan(&total); err != nil {
		Logger().Warn("缩略图配额统计失败", "tier", tierKey, "err", err)
		return
	}
	target := int64(float64(quota) * thumbEvictWatermark)
	if total <= target {
		return
	}

	// 快照该库该档 dirty 在册键（收集游标前）：dirty 键即将以新 last_access 落盘
	// （flush 快照取出后并发 record 的新条目 / flush 窗口重导出记账），按 db 旧行
	// 淘汰会删掉刚被重写的同路径 PNG，随后新 dirty 行落盘形成「记账称存在而文件
	// 缺」——排除后本轮回收量可能不足水位，下次 flush 落盘后自然重查（自愈，
	// 非错误；独立审核遗留项 2）。
	l.mu.Lock()
	dirtyKeys := make(map[thumbCacheKey]struct{})
	for k := range l.dirty[libraryID] {
		if k.tier == tierKey {
			dirtyKeys[k] = struct{}{}
		}
	}
	l.mu.Unlock()

	// 游标收集待删条目（rows 收集完毕显式 Close 后再开删除事务——单连接池
	// SetMaxOpenConns(1) 下 rows 未关先查会自锁，docs/spec/assets-management.md）
	rows, err := store.db.Query(
		"SELECT file_id, page, bytes FROM thumb_cache WHERE tier = ? ORDER BY last_access_ns ASC", tierKey)
	if err != nil {
		Logger().Warn("缩略图淘汰游标查询失败", "tier", tierKey, "err", err)
		return
	}
	type victim struct {
		key   thumbCacheKey
		bytes int64
	}
	victims := make([]victim, 0, 64)
	reclaimed := int64(0)
	for rows.Next() {
		var v victim
		v.key.tier = tierKey
		if err := rows.Scan(&v.key.fileID, &v.key.page, &v.bytes); err != nil {
			_ = rows.Close()
			Logger().Warn("缩略图淘汰游标读取失败", "tier", tierKey, "err", err)
			return
		}
		if _, isDirty := dirtyKeys[v.key]; isDirty {
			continue // dirty 在册键跳过淘汰（注释见上方快照处）
		}
		victims = append(victims, v)
		reclaimed += v.bytes
		if total-reclaimed <= target {
			break // 已删至水位：停止收集（剩余行随游标关闭丢弃）
		}
	}
	if err := rows.Close(); err != nil {
		Logger().Warn("缩略图淘汰游标关闭失败", "tier", tierKey, "err", err)
		return
	}
	if len(victims) == 0 {
		return // 单条即超水位以下的防御分支（理论不可达：total > target 才会进来）
	}

	keys := make([]thumbCacheKey, 0, len(victims))
	for _, v := range victims {
		keys = append(keys, v.key)
	}
	if err := store.deleteThumbCacheKeys(keys); err != nil {
		Logger().Warn("缩略图淘汰记账删除失败", "tier", tierKey, "count", len(keys), "err", err)
		return
	}
	for _, v := range victims {
		if err := os.Remove(thumbPNGPath(thumbsDir, tierKey, v.key.fileID, v.key.page)); err != nil && !os.IsNotExist(err) {
			Logger().Warn("缩略图淘汰删文件失败", "tier", tierKey, "fileId", v.key.fileID, "page", v.key.page, "err", err)
		}
	}
	Logger().Info("缩略图档位配额淘汰", "tier", tierKey, "evicted", len(victims),
		"reclaimedBytes", reclaimed, "quotaBytes", quota)
}

// pruneBeyond 页数缩减对账：导出页数 < 记账页数时清除越界记账行 + 越界 PNG 文件
// （页码带连字符形态无法从文件名单独判定越界页，以记账表为准确删——research §6.2）。
// 仅成功导出路径调用：失败文件旧图仍有效，内容失效已由扫描失效联动处理。
func (l *thumbCacheLedger) pruneBeyond(libraryID string, thumbsDir string, fileID int64, tier string, maxPage int) {
	store, err := l.storeFn(libraryID)
	if err != nil {
		return // 库不可用（已删除等竞态）：静默跳过
	}
	pages, err := store.thumbPagesBeyond(fileID, tier, maxPage)
	if err != nil {
		Logger().Warn("缩略图越界页查询失败", "libraryId", libraryID, "fileId", fileID, "tier", tier, "err", err)
		return
	}
	if len(pages) == 0 {
		return
	}
	keys := make([]thumbCacheKey, 0, len(pages))
	for _, p := range pages {
		keys = append(keys, thumbCacheKey{fileID: fileID, tier: tier, page: p})
	}
	if err := store.deleteThumbCacheKeys(keys); err != nil {
		Logger().Warn("缩略图越界页删除失败", "libraryId", libraryID, "fileId", fileID, "tier", tier, "err", err)
		return
	}
	for _, p := range pages {
		if err := os.Remove(thumbPNGPath(thumbsDir, tier, fileID, p)); err != nil && !os.IsNotExist(err) {
			Logger().Warn("删除越界缩略图文件失败", "libraryId", libraryID, "fileId", fileID, "page", p, "err", err)
		}
	}

	// 清 dirty 中同键越界条目：防后续 flush 把已删行复活（幽灵行无害但徒增对账噪音）
	l.mu.Lock()
	for k := range l.dirty[libraryID] {
		if k.fileID == fileID && k.tier == tier && k.page > maxPage {
			delete(l.dirty[libraryID], k)
		}
	}
	l.mu.Unlock()
}

// dropTier 清指定库指定档的 dirty 条目（分辨率变更清档联动，防旧分辨率记账
// 复活——ApplyThumbsSettingsChange 清档链路调用）。
func (l *thumbCacheLedger) dropTier(libraryID, tier string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k := range l.dirty[libraryID] {
		if k.tier == tier {
			delete(l.dirty[libraryID], k)
		}
	}
}

// checkQuota 上限变更触发的一次性惰性配额检查（后台语义与 flushLibrary 尾部
// 同源，复用 evictTier；库不可用静默跳过）。
func (l *thumbCacheLedger) checkQuota(libraryID, tier string) {
	store, err := l.storeFn(libraryID)
	if err != nil {
		return // 库不可用（已删除等竞态）：静默跳过
	}
	l.evictTier(store, libraryID, l.dirFn(libraryID), tier)
}

// Shutdown 关停收口（WaitThumbsDone 尾部、库连接关闭前调用）：先同步 flush 全部
// 有 dirty 的库（此刻连接仍开着），再关 quit 等 flusher 退出。置位 stopped 后
// record 丢弃、start 不再重启（幂等，重复调用直接返回）。
func (l *thumbCacheLedger) Shutdown() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.stopped = true
	ids := make([]string, 0, len(l.dirty))
	for id, m := range l.dirty {
		if len(m) > 0 {
			ids = append(ids, id)
		}
	}
	l.mu.Unlock()
	for _, id := range ids {
		l.flushLibrary(id)
	}
	close(l.quit)
	l.wg.Wait()
}

// ---------- thumb_cache 表访问（store 层，供 ledger 与失效联动链路共用） ----------

// flushThumbEntries 单事务批量落盘记账（INSERT OR REPLACE 全量覆盖写：同 key 重复
// 记账以最新一条为准）。硬约束：SetMaxOpenConns(1) 下禁逐条 UPDATE——全部落盘
// 走单事务批量（见文件头注释）。
//
// 幽灵键过滤（INSERT ... SELECT ... WHERE EXISTS）：外键已启用（DSN foreign_keys(1)），
// file_id 已不在 files 表的幽灵条目（失效联动删 PNG 失败后前端 stale 列表 touch、
// flush 快照与清理竞态、重建在途批次旧 ID record——独立审核发现）若直接 INSERT 会
// 触发外键约束失败使整批事务丢弃；WHERE EXISTS 在单事务内静默过滤毒键，落盘行数
// 由调用方比对 RowsAffected 差额告警（幽灵键的 PNG 已随失效联动清理，记账缺失由
// 读取路径 touch 自愈，无需重试）。
func (s *assetsStore) flushThumbEntries(entries map[thumbCacheKey]thumbCacheEntry) (int64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(
		"INSERT OR REPLACE INTO thumb_cache(file_id, tier, page, bytes, last_access_ns) " +
			"SELECT ?, ?, ?, ?, ? WHERE EXISTS(SELECT 1 FROM files WHERE id = ?)")
	if err != nil {
		return 0, err
	}
	var persisted int64
	for k, e := range entries {
		res, err := stmt.Exec(k.fileID, k.tier, k.page, e.bytes, e.lastAccessNs, k.fileID)
		if err != nil {
			_ = stmt.Close()
			return persisted, err
		}
		if n, err := res.RowsAffected(); err != nil {
			_ = stmt.Close()
			return persisted, err
		} else {
			persisted += n
		}
	}
	if err := stmt.Close(); err != nil {
		return persisted, err
	}
	return persisted, tx.Commit()
}

// thumbPagesBeyond 查询某文件某档位超过 maxPage 的记账页（页数缩减对账用）。
func (s *assetsStore) thumbPagesBeyond(fileID int64, tier string, maxPage int) ([]int, error) {
	rows, err := s.db.Query(
		"SELECT page FROM thumb_cache WHERE file_id = ? AND tier = ? AND page > ?", fileID, tier, maxPage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := make([]int, 0)
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// deleteThumbCacheKeys 单事务批量删除记账行（越界对账 / 失效联动 / 配额淘汰共用
// 出口；外键级联为兜底，代码显式删除保证与 PNG 文件删除同批收口——双保险口径）。
func (s *assetsStore) deleteThumbCacheKeys(keys []thumbCacheKey) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare("DELETE FROM thumb_cache WHERE file_id = ? AND tier = ? AND page = ?")
	if err != nil {
		return err
	}
	for _, k := range keys {
		if _, err := stmt.Exec(k.fileID, k.tier, k.page); err != nil {
			_ = stmt.Close()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteThumbCacheForFile 单语句清除指定文件的全部记账行（重解析/删除文件的失效
// 联动；外键级联为兜底，显式删除为契约——见 invalidateFileThumbs 注释）。
func (s *assetsStore) deleteThumbCacheForFile(fileID int64) error {
	_, err := s.db.Exec("DELETE FROM thumb_cache WHERE file_id = ?", fileID)
	return err
}

// deleteThumbCacheTier 清指定档位全部记账行（分辨率变更清档；单语句，外键无涉）。
func (s *assetsStore) deleteThumbCacheTier(tier string) error {
	_, err := s.db.Exec("DELETE FROM thumb_cache WHERE tier = ?", tier)
	return err
}
