package service

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"workbench/model"
)

// 素材库备份（SRS §2.7.5，素材管理第十批 PR2）：手动备份 + 重大操作（重建索引 /
// 删除库）前自动快照 + 保留淘汰 + 完整性校验 + 还原。
//
// 实现基座（research 实证定稿，.trellis/tasks/10-18-srs-2-7-3/research/sqlite-backup-restore.md）：
//   - VACUUM INTO 单语句产出独立紧凑 db（禁裸拷贝 db 文件——WAL 模式下拷贝主文件
//     会丢未 checkpoint 数据）；WAL 未 checkpoint 数据完整进入备份，无需预
//     checkpoint；不能包事务（单语句 Exec 自动提交即可）。
//   - PRAGMA integrity_check 校验必须 Query 读结果集（Exec 丢结果集形同没校验），
//     健康库首行 "ok"；非 ok 删备份文件并报错（备份失败不静默——带病备份比无备份
//     更危险）。
//   - 备份范围 = 整库 db（索引/标注/标签/FTS/拆分记录全部内容）；缩略图 PNG 与
//     拆分产物目录不在备份范围（还原后缩略图由补缺链路按新索引重导出，残留旧图
//     由 LRU 配额自然淘汰；libraries.json 中的库元数据随库存续，不随备份走）。
//
// 并发口径：同库备份经 lockLibraryBackups 互斥串行（选名 + VACUUM + 校验 + 淘汰
// 全程）——手动备份与重建/删库前自动备份共用 createAssetBackup 且可并发，无互斥时
// 同秒两调用竞态选同一 target，后到者 VACUUM INTO 失败的清理会误删先到者刚生成的
// 有效备份（素材审核 2026-10-18 补口）。备份与扫描/拆分仍可并发：VACUUM INTO 单
// 语句在该库单连接池上排队（执行期间该库其他查询在池上等待，60 万行 127ms，素材
// 库量级毫秒~百毫秒级，research 实证可接受）；删除/重建链路在各自持锁段内调用，
// 备份与主操作的先后由调用方锁序保证。
//
// 还原步骤序列（research §4.1 定稿）：取消在途扫描 → 阻塞等待扫描锁（取消在文件
// 间检查点生效，须等真正退出）→ 拆分互斥（TryLock，拆分中禁还原）→ 取消并等待
// 在途导出退出（导出行键快照跨还原换库会产出混批 CSV）→ 持 s.mu 复查导出注册表
// （非空拒绝——封 CancelExport 遍历与导出注册的竞态窗口，见 RestoreAssetLibrary
// 内注释）→ 关连接 → 删 -wal/-shm 残留 → os.Rename 覆盖 <id>.db（Windows 走
// MoveFileEx REPLACE_EXISTING 直接覆盖，无需先 Remove——util/json.go
// TestSaveJSON_OverwriteExisting 锚定语义）→ 惰性重开 + integrity_check 复验。
// 备份文件本身在覆盖前预检完整性（零半态：损坏备份不动当前库），落库后复验一次
// （覆盖前预检与落库之间理论无写窗口，复验为防呆闭环）。

// assetBackupRetention 每库备份保留份数（超出淘汰最旧；默认值硬编码，配置化后批）。
const assetBackupRetention = 5

// assetBackupClock 备份时间戳时钟（测试注入点：同秒双备份的文件名冲突防御与
// 保留淘汰断言依赖可推进的假时钟，禁 sleep 等真实跨秒，对齐 docs/spec/test-stability.md）。
var assetBackupClock = time.Now

// assetBackupNameRegexp 备份文件名形态（<yyyyMMdd-HHmmss>[_深序].db；_N 为同秒
// 多次备份的深序后缀）。深序分隔符取「_」（0x5F >「.」0x2E）：文件名字典序与
// 时间序严格一致（裸名 < _1 < _2 < 下一秒），保留淘汰与清单排序可纯按名排序
// （用「-」作分隔符时「-1.db」字典序排在裸名「.db」之前，时间序颠倒，禁用）。
var assetBackupNameRegexp = regexp.MustCompile(`^\d{8}-\d{6}(_\d+)?\.db$`)

// assetBackupsDir 库备份根目录单点（data/assets/backups/<library-id>）。所有备份
// 路径来源统一走本方法（创建/清单/还原防呆），防路径口径漂移（对齐 assetThumbsDir
// 模式）。备份目录独立于库 db/thumbs/derived 之外——DeleteLibrary 不清理本目录
// （删库后备份是唯一恢复点，必须存续）。
func (s *AssetLibraryService) assetBackupsDir(libraryID string) string {
	return filepath.Join(s.assetsDir, "backups", libraryID)
}

// BackupAssetLibrary 手动备份（库操作「备份」入口）：整库 VACUUM INTO 快照 +
// integrity_check 校验 + 保留淘汰，返回本次备份条目（落点
// data/assets/backups/<libId>/<yyyyMMdd-HHmmss>.db）。库不存在报错；
// 同秒多次备份以 _N 深序后缀防文件名冲突（VACUUM INTO 目标已存在直接报错）。
func (s *AssetLibraryService) BackupAssetLibrary(libraryID string) (model.AssetBackupEntry, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetBackupEntry{}, err
	}
	entry, err := s.createAssetBackup(libraryID)
	if err != nil {
		return model.AssetBackupEntry{}, err
	}
	Logger().Info("素材库手动备份完成", "libraryId", libraryID,
		"file", entry.FileName, "bytes", entry.SizeBytes)
	return entry, nil
}

// backupLibraryForCriticalOp 重大操作（重建索引 / 删除库）前自动快照：与手动备份
// 共用同一实现（createAssetBackup）。失败返回错误，策略由调用方定夺——
//   - 重建前（rebuildLibrary）：失败仅 slog.Warn 不中止（重建可重扫恢复，标注有
//     哈希兜底，备份缺失损失有限——research §5「best-effort 备份对可逆操作不放大
//     故障面」）；
//   - 删库前（DeleteLibrary）：失败不静默，返回 E_ASSETS_DELETE_BACKUP_FAILED
//     确认型拒绝（删库不可逆，零恢复点删除须经用户知情确认后带 allowNoBackup
//     重调——research §5 残余风险以「升级告知强度」缓解而非阻断）。
func (s *AssetLibraryService) backupLibraryForCriticalOp(libraryID string) error {
	_, err := s.createAssetBackup(libraryID)
	return err
}

// createAssetBackup 备份核心（手动与重大操作自动共用唯一实现，禁第二口径）：
// 同库互斥 → 建目录 → 时间戳命名（同秒 _N 深序）→ VACUUM INTO → integrity_check
// （非 ok 删备份报错）→ 保留淘汰（每库最近 N 份，超出删最旧）→ 返回备份条目。
func (s *AssetLibraryService) createAssetBackup(libraryID string) (model.AssetBackupEntry, error) {
	// 同库备份互斥（选名 + VACUUM + 校验 + 淘汰全程串行）：与扫描/拆分锁独立，
	// 见文件头「并发口径」——无互斥时同秒并发两调用会竞态选同一 target，后到者
	// 失败清理误删先到者的有效备份
	releaseBackup := s.lockLibraryBackups(libraryID)
	defer releaseBackup()
	dir := s.assetBackupsDir(libraryID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return model.AssetBackupEntry{}, fmt.Errorf("创建备份目录失败: %w", err)
	}
	// 目标命名：时间戳天然不与既有目标冲突（VACUUM INTO 目标已存在报 output
	// file already exists）；同秒多次备份以 _1/_2 深序后缀去冲突（存在性探测推进；
	// 分隔符选型见 assetBackupNameRegexp 注释——字典序须与时间序一致）。
	// 时钟单次读取：CreatedAt 与文件名时间戳同源一致（清单展示两口径不漂移）。
	now := assetBackupClock()
	base := now.Format("20060102-150405")
	target := filepath.Join(dir, base+".db")
	for seq := 1; ; seq++ {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			break
		}
		target = filepath.Join(dir, fmt.Sprintf("%s_%d.db", base, seq))
	}

	// VACUUM INTO（单语句；单连接池下该库其他查询在池上排队，research 实证）。
	// 失败清理经 removeFailedVacuumTarget 安全边界（本调用选名确认不存在后的
	// 空残留才删，防误删他方备份）。
	if err := s.withStore(libraryID, func(store *assetsStore) error {
		return store.vacuumInto(target)
	}); err != nil {
		removeFailedVacuumTarget(target)
		return model.AssetBackupEntry{}, fmt.Errorf("生成备份失败: %w", err)
	}

	// 完整性校验：非 ok 删备份并报错（备份失败不静默）。
	if err := verifySQLiteIntegrity(target); err != nil {
		_ = os.Remove(target)
		return model.AssetBackupEntry{}, fmt.Errorf("备份完整性校验未通过，已删除无效备份: %w", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		return model.AssetBackupEntry{}, fmt.Errorf("读取备份文件失败: %w", err)
	}

	s.purgeOldBackups(dir, libraryID)

	return model.AssetBackupEntry{
		LibraryID: libraryID,
		FileName:  filepath.Base(target),
		Path:      target,
		SizeBytes: info.Size(),
		CreatedAt: now.Format("2006-01-02 15:04:05"),
	}, nil
}

// removeFailedVacuumTarget VACUUM 失败后目标文件的清理守卫（安全边界单点）：
// 仅删除「空文件」——SQLite 报错前可能已创建空目标残留，须清；非空文件必非本次
// 调用成功产物（VACUUM 未完成写入），进程内互斥下不可达，跨进程理论窗口（同机
// 另一实例同秒备份）下可能是并发方刚生成的有效备份——误删他方有效备份比残留
// 冗余文件危害大（冗余文件进清单后由还原前 integrity_check 预检拒载，无损；
// 磁盘满类部分写入的残留属本调用产物但不删，同理由非空判定兜住，下次备份以
// 深序后缀绕开）。
func removeFailedVacuumTarget(target string) {
	info, err := os.Stat(target)
	if err != nil || info.Size() != 0 {
		return
	}
	_ = os.Remove(target)
}

// purgeOldBackups 保留淘汰：每库最近 assetBackupRetention 份，超出淘汰最旧。
// 文件名字典序 = 时间序（时间戳命名 + _N 深序分隔符选型保证，见
// assetBackupNameRegexp 注释）。淘汰失败仅告警不报错（保留淘汰是辅助动作，
// 失败不影响本次备份已落盘的事实，下次备份重试）。
func (s *AssetLibraryService) purgeOldBackups(dir, libraryID string) {
	names, err := listBackupFileNames(dir)
	if err != nil {
		Logger().Warn("枚举备份目录失败（保留淘汰跳过）", "libraryId", libraryID, "err", err)
		return
	}
	if len(names) <= assetBackupRetention {
		return
	}
	sort.Strings(names)
	for _, name := range names[:len(names)-assetBackupRetention] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			Logger().Warn("淘汰旧备份失败（下次备份重试）", "libraryId", libraryID, "file", name, "err", err)
		} else {
			Logger().Info("淘汰旧备份", "libraryId", libraryID, "file", name)
		}
	}
}

// listBackupFileNames 枚举目录内的备份文件名（正斜杠无关心智；仅收
// assetBackupNameRegexp 形态的 .db 文件，用户误放的其他文件不进清单与淘汰面）。
func listBackupFileNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !assetBackupNameRegexp.MatchString(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

// ListAssetLibraryBackups 备份清单（前端还原入口选份用）：按备份时间倒序（最新
// 在前），条目含文件名/时间/大小。从未备份（目录不存在）返回空清单非错误。
func (s *AssetLibraryService) ListAssetLibraryBackups(libraryID string) ([]model.AssetBackupEntry, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return nil, err
	}
	dir := s.assetBackupsDir(libraryID)
	names, err := listBackupFileNames(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.AssetBackupEntry{}, nil
		}
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}
	// 倒序（最新在前）：文件名字典序即时间序，反向排列
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	out := make([]model.AssetBackupEntry, 0, len(names))
	for _, name := range names {
		info, statErr := os.Stat(filepath.Join(dir, name))
		if statErr != nil {
			continue // 枚举与 stat 之间被删（并发淘汰窗口）：跳过不进清单
		}
		out = append(out, model.AssetBackupEntry{
			LibraryID: libraryID,
			FileName:  name,
			Path:      filepath.Join(dir, name),
			SizeBytes: info.Size(),
			CreatedAt: parseAssetBackupTime(name),
		})
	}
	return out, nil
}

// parseAssetBackupTime 从备份文件名解析备份时间（"2006-01-02 15:04:05" 展示
// 口径）。文件名前 15 字符恒为 <yyyyMMdd>-<HHmmss>（同秒深序后缀在其后），解析
// 失败（理论不可达，正则已门禁）退空串。
func parseAssetBackupTime(name string) string {
	if len(name) < len("20060102-150405")+len(".db") {
		return ""
	}
	t, err := time.Parse("20060102-150405", name[:15])
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// RestoreAssetLibrary 还原备份（前端确认弹窗「覆盖当前索引与标注」后调用，后端
// 直接执行）：备份文件整体替换该库 <id>.db，还原后前端提示重扫（增量扫描按
// mtime+size 与还原后索引收敛，无自动重扫——还原是回退动作，重扫时机归用户）。
//
// 防呆：backupFile 必须位于该库备份目录内（Abs + Rel 越界判定，防任意文件覆盖
// <id>.db）；覆盖前对备份文件预检完整性（零半态：校验失败时当前库原样不动）。
//
// 步骤序列见文件头注释。还原后缩略图 PNG 不清理：还原回退 fileID 体系，现存 PNG
// 中还原点之后新生成的部分键失配（不匹配新索引补缺、进 LRU 配额自然淘汰），缺失
// 部分由扫描后的补缺链路重导出，无需人工干预。
func (s *AssetLibraryService) RestoreAssetLibrary(libraryID, backupFile string) error {
	if _, err := s.getLibrary(libraryID); err != nil {
		return err
	}
	backupFile = strings.TrimSpace(backupFile)
	if backupFile == "" {
		return errors.New("备份文件路径不能为空")
	}
	absBackup, err := filepath.Abs(backupFile)
	if err != nil {
		return fmt.Errorf("备份文件路径无效: %w", err)
	}
	// 越界遏制：备份文件必须位于该库备份目录内（防呆——任意路径经本方法可覆盖
	// <id>.db；还原入口仅消费 ListAssetLibraryBackups 返回的 Path，正常 UI 不会
	// 触发，校验为防御面）
	backupDir := s.assetBackupsDir(libraryID)
	rel, err := filepath.Rel(backupDir, absBackup)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("备份文件路径非法（须位于该库备份目录内）: %s", backupFile)
	}
	info, err := os.Stat(absBackup)
	if err != nil {
		return fmt.Errorf("备份文件不存在或无法访问: %w", err)
	}
	if info.IsDir() {
		return errors.New("备份路径是目录，须选择 .db 备份文件")
	}
	// 覆盖前预检备份完整性（零半态）：损坏备份不覆盖当前库，用户可改选其他备份
	if err := verifySQLiteIntegrity(absBackup); err != nil {
		return fmt.Errorf("备份文件完整性校验未通过，当前索引未做改动: %w", err)
	}

	// 取消在途扫描（幂等）→ 阻塞等待扫描锁：取消信号在扫描文件间检查点生效，
	// TryLock 会立即拒绝未退出的在途扫描，故走阻塞 Lock 等真正退出（还原窗口期
	// 禁扫描写已关连接，research §4.3）
	s.CancelScan(libraryID)
	release, err := s.lockScanBlocking(libraryID)
	if err != nil {
		return err
	}
	defer release()

	// 拆分互斥（TryLock，对齐 DeleteLibrary：拆分中禁还原，防拆分写已关连接；
	// 先扫描锁后拆分锁顺序获取，与 DeleteLibrary 同序无反向成环）
	releaseSplit, err := s.tryLockSplit(libraryID)
	if err != nil {
		return err
	}
	defer releaseSplit()

	// 锁内复验库存在（等待在途扫描退出的窗口内库可能被删）
	if _, err := s.getLibrary(libraryID); err != nil {
		return err
	}

	// 取消在途缩略图批次（同 DeleteLibrary 口径：缩小「worker 向还原中索引写
	// PNG」窗口；取消异步不等待，在途单文件残留由配额淘汰兜底）
	s.CancelAssetThumbs(libraryID)

	// 取消在途元数据导出并等待退出（exportCancels 链路，素材审核 2026-10-18 补口）：
	// 导出为「物化行键快照 + 分批短会话水合」，每批经 withStore→storeForLocked
	// （assets_library.go）懒重开——还原换库后，旧库物化的行键会在新库按 fileID
	// 静默水合出错误行（还原回滚后 fileID 空间重叠指向不同内容），无声无错产出
	// 混批 CSV。ctx 检查点在行键分批环顶部（assets_export.go「中止语义」段），
	// 取消后当前批次水合完成即退出，等待上界 = 单批时长。命中在途导出才等待；
	// WaitExportsDone 为全局计数（导出无按库等待原语），罕见并发他库导出时最多
	// 多等一个批次。此处不持 s.mu，导出批间短会话可正常取锁收尾，无死锁环。
	if s.CancelExport(libraryID) {
		s.WaitExportsDone()
	}

	// 换库窗口封死（CancelExport 遍历与导出注册存在纳秒级竞态，素材审核
	// 2026-10-18 复查补口）：遍历未见到、随后注册完成的导出，其行键物化若恰在
	// 还原取 s.mu 前于旧库完成，后续批次水合新库即混批（机制见上段注释）。
	// 「db 已关报错」兜底不成立——导出每批新开短会话（storeForLocked 懒重开），
	// rename 后打开的是还原后的新库，无声无错。封死手段 = 持 s.mu 复查本库
	// exportCancels 注册表（hasPendingExportLocked，锁序 s.mu→opMu 单向无 ABBA 环；
	// 复查必须与关连接/rename 同锁串行，放锁再查会复开「查空 → 导出注册并抢先
	// 物化旧库 → 取锁 rename」窗口）：
	//   - 非空 → 拒绝还原（复用 E_ASSETS_SCAN_IN_PROGRESS warning 引导级——素材域
	//     「操作进行中」既有一类，前端 warning 分流同款，对齐 tryLockSplit 复用
	//     先例，消息按导出措辞区分），引导用户稍后重试。推理链：危险形态 = 「行键
	//     已在旧库物化 + 批次未水合完」，此态下导出未到函数出口，注册条目必仍在
	//     map（注销在导出出口 defer），复查必命中——「物化于旧库 + 复查放行」两
	//     条件不可同真；而「已注册但尚未物化」的导出被 s.mu 阻塞，物化只能发生在
	//     还原完成后的新库上（本可自愈，拒绝仅为给用户确定性的重试入口）。
	//   - 空 → 继续关连接 + rename（全程持 s.mu）。此后不产混批：①旧库物化的导出
	//     若未结束则复查必非空（见上），复查放行 ⇒ 旧库物化的导出均已完全结束
	//     （无在途批次，无混批面）；②复查读取之后才注册的导出，其物化（程序序
	//     严格后于注册）被 s.mu 阻塞至 rename 完成，只能从新库物化，全部批次水合
	//     同一新库（自洽）。
	s.mu.Lock()
	if s.hasPendingExportLocked(libraryID) {
		s.mu.Unlock()
		return model.NewAppError(model.ErrCodeAssetsScanInProgress,
			"检测到该素材库有元数据导出进行中，请稍后重试还原")
	}

	// 关连接（还原目标在库文件上，必须先关：WAL 伴生文件在连接正常关闭时收口）
	if store, ok := s.open[libraryID]; ok {
		if err := store.Close(); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("关闭素材索引库失败: %w", err)
		}
		delete(s.open, libraryID)
	}
	// 防御性删除 WAL/SHM 残留（异常退出可能残留；必须在重开前删，防旧 WAL 与
	// 还原后的 db 内容错位恢复出错误状态；research §4.1 步骤 4）
	dbPath := s.libraryDBPath(libraryID)
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			s.mu.Unlock()
			return fmt.Errorf("清理索引库 WAL 残留失败: %w", err)
		}
	}
	// 备份文件原子覆盖 <id>.db（Windows os.Rename 走 MoveFileEx REPLACE_EXISTING
	// 直接覆盖已存在目标，无需先 Remove——先 Remove 再 Rename 会引入「Remove 成功、
	// Rename 失败」的丢文件中间态；备份与库文件同在 data/assets/ 卷，满足同卷
	// 原子替换前提）
	if err := os.Rename(absBackup, dbPath); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("还原备份文件失败: %w", err)
	}
	s.mu.Unlock()

	// 惰性重开（withStore 按需 openAssetsStore，migrate 幂等）+ 落库后 integrity_check
	// 复验一次（防备份文件本身损坏入库的最后一道闭环；失败时还原已生效，报错引导
	// 用户改选其他备份）
	if err := s.withStore(libraryID, func(store *assetsStore) error {
		return store.integrityCheck()
	}); err != nil {
		return fmt.Errorf("还原后完整性复验未通过，请尝试其他备份: %w", err)
	}
	Logger().Info("素材库还原完成", "libraryId", libraryID, "backup", filepath.Base(absBackup))
	return nil
}

// verifySQLiteIntegrity 独立打开 SQLite 文件做 integrity_check（备份文件校验用，
// 文件不属于任何在管连接）。校验完即关；损坏文件在首条查询上报错或返回非 ok 行。
func verifySQLiteIntegrity(path string) error {
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer db.Close()
	return (&assetsStore{db: db}).integrityCheck()
}
