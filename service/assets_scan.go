package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"workbench/model"
)

// 目录扫描（素材管理·增量索引）：递归扫库根目录，后缀白名单 .pptx（大小写
// 不敏感）；强制排除拆分派生产物目录 derived/。增量判定 mtime+size（未变跳过，
// 变更才哈希+重解析），单文件解析失败降级标记状态不中断批量；进度/终态事件经
// EventSink（assets-scan:*），取消经注册表派生 ctx 贯通。

// PPT 拆分派生副本排除（PRD：派生产物不入索引，防与源文件重复入库）。拆分输出
// 布局见 service/pptx_split.go SplitPptxPerPage：源 <dir>/<name>.pptx 的产物落
// <dir>/<name>/<name>-<页序号>.pptx（同名文件夹 + 同名-页序命名，页序 %03d 固定
// 三位，本判定只验数字不限位数，两位/三位通吃）。仅排除严格符合该布局的文件；
// 用户自组织的「同名目录/同名-数字.pptx」与拆分产物同构不可区分，同样被排除
// （防重优先的取舍）。
func isPptxSplitDerivative(rel string) bool {
	parentBase := path.Base(path.Dir(rel))
	if parentBase == "." {
		// 根目录直置文件：拆分产物恒落「同名文件夹」内（真实子目录），根目录
		// 形态不可能是派生副本。不提前排除时，形如「.-01.pptx」的文件名主干
		// 以「.」开头恰与 parentBase="." 前缀匹配，被误判为派生而漏索引。
		return false
	}
	name := path.Base(rel)
	stem := strings.TrimSuffix(name, ".pptx")
	if stem == name {
		return false // 非 .pptx 后缀
	}
	sep := len(parentBase)
	if len(stem) < sep+2 || stem[:sep] != parentBase || stem[sep] != '-' {
		return false // 前缀非父目录名或无「-页序」段
	}
	for i := sep + 1; i < len(stem); i++ {
		if stem[i] < '0' || stem[i] > '9' {
			return false // 页序段含非数字
		}
	}
	return true
}

// pptxDiskEntry 磁盘上单个待索引 pptx 的最小视图。
type pptxDiskEntry struct {
	rel     string // 相对库根目录路径（正斜杠分隔，作库内唯一键）
	abs     string // 绝对路径（读取用）
	size    int64
	mtimeNs int64
}

// 单文件处理结果（扫描计数分流）。
type scanOutcome int

const (
	scanOutcomeNew       scanOutcome = iota // 新入库
	scanOutcomeUpdated                      // 变更重解析
	scanOutcomeFailed                       // 解析失败（损坏等）
	scanOutcomeEncrypted                    // 加密跳过
	scanOutcomeDenied                       // 权限不足跳过
)

// StartScan 扫描指定素材库：增量索引根目录下全部 pptx，返回结果汇总。
//
// 可取消：内部派生可取消 ctx 并登记取消注册表（键 = 规范化 libraryID），
// CancelScan 从另一调用线命中；取消错误经 mapAssetsCancelError 统一映射
// ErrAssetsCancelled（E_ASSETS_CANCELLED，前端 warning 中性提示）。
// defer LIFO：unregisterCancel 先于扫描锁释放执行（docs/spec/logging-and-errors.md
// 取消链路契约）。
//
// 串行保护：同库同时仅允许一个扫描（tryLockScan TryLock 互斥拒绝，并发第二次
// 调用返回 E_ASSETS_SCAN_IN_PROGRESS）。
//
// 事件：逐文件 assets-scan:progress（done/total/当前文件）+ 终态 assets-scan:done
// （结果汇总）/ assets-scan:error（用户可读消息；未注入出口时静默跳过）。
func (s *AssetLibraryService) StartScan(ctx context.Context, libraryID string) (model.AssetScanResult, error) {
	return s.runLibraryScan(ctx, libraryID, "assets scan", func(scanCtx context.Context, store *assetsStore, lib model.AssetLibrary) (model.AssetScanResult, error) {
		return s.scanLibrary(scanCtx, store, lib)
	})
}

// RebuildScan 手动全量重建（PRD：索引损坏/统计口径漂移时的自愈入口）：清该库
// files/slides/FTS 记录后走一次全量扫描。用户标注（文件标签绑定）按「文件相对
// 路径 → 标签 ID」映射在重建后写回，path 未变的文件标签不丢；标签本体（tags 表）
// 与星级/备注随 files 清除（元数据重建语义，注释明示不做保留）。
// 串行锁/取消注册/事件与增量扫描共用同一链路（assets-scan:* 前缀）。
func (s *AssetLibraryService) RebuildScan(ctx context.Context, libraryID string) (model.AssetScanResult, error) {
	return s.runLibraryScan(ctx, libraryID, "assets rebuild", s.rebuildLibrary)
}

// runLibraryScan 扫描操作公共骨架（增量扫描与全量重建共用）：库校验 → 扫描
// 互斥锁 → 取消注册 → 库连接 → op 执行 → 统一日志与终态事件。op 内 ctx 取消
// 在文件间检查点生效。
func (s *AssetLibraryService) runLibraryScan(ctx context.Context, libraryID, opName string, op func(scanCtx context.Context, store *assetsStore, lib model.AssetLibrary) (model.AssetScanResult, error)) (model.AssetScanResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetScanResult{}, err
	}

	release, err := s.tryLockScan(libraryID)
	if err != nil {
		return model.AssetScanResult{}, err
	}
	defer release()

	// 锁内复验库存在：getLibrary 与拿扫描锁的窗口内库可能被 DeleteLibrary 删除
	// （删除与扫描同锁互斥），不复验则 storeForScan 会以 sqlite 惰性建库语义
	// 重建已删库文件，残留注册表外的孤儿 db 文件。
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetScanResult{}, err
	}

	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	entry := s.registerCancel(libraryID, cancel)
	defer s.unregisterCancel(libraryID, entry)

	// 在途扫描计数：WaitScansDone（关停收口）以此等待全部扫描退出，防 CloseAll
	// 关连接后扫描继续写库报 sql: database is closed 噪音
	s.scanWG.Add(1)
	defer s.scanWG.Done()

	store, err := s.storeForScan(libraryID)
	if err != nil {
		return model.AssetScanResult{}, err
	}

	result, err := op(scanCtx, store, lib)
	if err != nil {
		Logger().Error(opName+" failed", "libraryId", libraryID, "rootPath", lib.RootPath, "err", err)
		err = mapAssetsCancelError(err)
		s.emitCurrent("assets-scan:error", map[string]any{"libraryId": libraryID, "message": appErrMessage(err)})
		return model.AssetScanResult{}, err
	}
	Logger().Info(opName+" done", "libraryId", libraryID, "rootPath", lib.RootPath,
		"total", result.Total, "new", result.New, "updated", result.Updated,
		"unchanged", result.Unchanged, "failed", result.Failed,
		"encrypted", result.Encrypted, "denied", result.Denied, "removed", result.Removed)
	s.emitCurrent("assets-scan:done", map[string]any{"libraryId": libraryID, "result": result})
	// 扫描完成自动入队缩略图补缺（本函数运行于扫描 goroutine，且 scanWG 计数
	// 尚未归零——结构性保证 WaitScansDone join 时批次已注册，CloseAll 不会先于
	// 入队关连接）。异步入队不阻塞扫描返回。
	s.enqueueThumbsBacklog(store, lib)
	return result, nil
}

// enqueueThumbsBacklog 扫描完成后自动补缺入队：按 thumb_cache 记账 + PNG 文件
// 存在性比对出缺失缩略图的文件清单，整批入队（文件级粒度——COM 打开粒度为
// 文件，ExportPresentation 无页清单参数，任一页缺失即整文件重导出，已有页
// 覆盖写幂等）。无 PowerPoint 静默跳过（D1 降级）；补缺收集失败仅告警不上抛
// （不阻断扫描成功语义，下次扫描重试）。
func (s *AssetLibraryService) enqueueThumbsBacklog(store *assetsStore, lib model.AssetLibrary) {
	// D1 自动生成开关（设置段）：关闭时扫描完成不入队（用户显式操作的单文件
	// 重试链路 D4 不受限——RetryAssetThumb 不经本短路）
	if !s.thumbsSettings().Auto {
		return
	}
	if !s.thumbQ.exporter.Available() {
		return
	}
	thumbsDir := s.assetThumbsDir(lib.ID)
	backlog, err := s.collectThumbsBacklog(store, lib, thumbsDir)
	if err != nil {
		Logger().Warn("缩略图补缺收集失败（下次扫描重试）", "libraryId", lib.ID, "err", err)
		return
	}
	if len(backlog) == 0 {
		return
	}
	Logger().Info("缩略图补缺入队", "libraryId", lib.ID, "files", len(backlog))
	s.EnqueueAssetThumbs(lib.ID, backlog, thumbsDir)
}

// collectThumbsBacklog 收集缺失缩略图的文件清单：ok 状态文件 × 现有页数，逐档
// 逐页以 PNG 文件存在性判定页已生成（存在性为唯一权威口径，见步骤 3 注释），
// 任一页缺失即该文件入队。
// 两条查询各自收齐结果并显式 Close 后再发下一条（单连接池 SetMaxOpenConns(1)
// 下 rows 未关先查会自锁，docs/spec/assets-management.md Gotcha）。
//
// 性能边界（有意取舍）：同步跑在扫描 goroutine 内（runLibraryScan 返回前），
// ok 文件 × 页数 × 2 档逐页 os.Stat——千文件 × 20 页 ≈ 4 万次 stat，秒级；不移
// 后台 goroutine 是为保住「入队在扫描 goroutine 内」的结构性保证（WaitScansDone
// join 时批次必已注册，WaitThumbsDone 才取消得到）。设计目标为桌面千级库
// （listFileRecords 同口径）；万级库补缺 Stat 十秒级，与扫描本身同量级，不做
// 异步化（若未来成为问题，优先方案 = worker 消费批次时再做补缺过滤）。
func (s *AssetLibraryService) collectThumbsBacklog(store *assetsStore, lib model.AssetLibrary, thumbsDir string) ([]thumbFileSpec, error) {
	// 1. ok 状态文件清单
	type okFile struct {
		id   int64
		path string
	}
	okFiles := make([]okFile, 0, 64)
	rows, err := store.db.Query("SELECT id, path FROM files WHERE status = ?", model.AssetFileStatusOK)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f okFile
		if err := rows.Scan(&f.id, &f.path); err != nil {
			_ = rows.Close()
			return nil, err
		}
		okFiles = append(okFiles, f)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// 2. 各文件现有页数（slides 表页数权威口径）
	pageByFile := make(map[int64]int, len(okFiles))
	rows, err = store.db.Query("SELECT file_id, MAX(slide_num) FROM slides GROUP BY file_id")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var fileID int
		var maxNum int
		if err := rows.Scan(&fileID, &maxNum); err != nil {
			_ = rows.Close()
			return nil, err
		}
		pageByFile[int64(fileID)] = maxNum
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// 3. 逐文件比对：任一档任一页 PNG 缺失 → 整文件入队（文件级粒度，注释见
	// enqueueThumbsBacklog）。页命中判定 = PNG 文件存在（唯一权威口径）：
	//   - 记账（thumb_cache）不参与补缺判定——记账落盘是攒批异步的（dirty 未
	//     flush 时 db 查不到），参与判定会因 flush 时序把已生成页误判缺失，
	//     连续两次扫描重复入队重导出（真机冒烟实测抓出）；
	//   - 「记账命中但文件缺失」同样判缺失（崩溃/手动清理可致记账与文件脱钩，
	//     画廊无图可展示，物理事实优先）；
	//   - 记账 miss 而文件在的页不漏记账：读取路径 GetAssetThumb touch 会补
	//     记账（LRU 口径自愈），补缺判定与 LRU 记账解耦。
	backlog := make([]thumbFileSpec, 0, len(okFiles))
	tiers := s.resolvedThumbTiers() // 当前生效档位收集前解析一次（设置注入）
	for _, f := range okFiles {
		n := pageByFile[f.id]
		if n <= 0 {
			continue // 无幻灯片记录（页解析为空等）：无可导出页
		}
		missing := false
		for _, tier := range tiers {
			for page := 1; page <= n; page++ {
				if _, err := os.Stat(thumbPNGPath(thumbsDir, tier.Key, f.id, page)); err != nil {
					missing = true
					break
				}
			}
			if missing {
				break
			}
		}
		if !missing {
			continue
		}
		backlog = append(backlog, thumbFileSpec{
			FileID:  f.id,
			Path:    f.path,
			AbsPath: filepath.Join(lib.RootPath, filepath.FromSlash(f.path)),
		})
	}
	return backlog, nil
}

// rebuildLibrary 全量重建主体：备份标签绑定 → 清 files/FTS → 全量扫描（记录
// 已清空，全部文件按新增处理）→ 按路径映射重绑标签。任何一步失败即中断
// （部分重建态由再次重建自愈；清理与扫描非同一事务，中断后索引可能为空，
// 语义与「重建后重新扫描」一致，不阻塞增量扫描修复）。
func (s *AssetLibraryService) rebuildLibrary(ctx context.Context, store *assetsStore, lib model.AssetLibrary) (model.AssetScanResult, error) {
	// 0. 取消该库在途缩略图批次（独立审核发现）：不清则 worker 在清表+删目录后
	// 继续导出——MkdirAll 重建 tier 目录、旧 fileID PNG 落盘（AUTOINCREMENT 旧 ID
	// 永不复用，既不匹配新索引补缺也不进配额记账，超配额永久残留）。取消异步、
	// 在途单文件仍可能写入（同 DeleteLibrary 口径：窗口缩小，残留可手动清理）
	s.CancelAssetThumbs(lib.ID)

	// 1. 备份用户标签绑定（重建清 files 时 file_tags 级联清除）
	bindings, err := store.fileTagBindingsByPath()
	if err != nil {
		return model.AssetScanResult{}, fmt.Errorf("备份文件标签绑定失败: %w", err)
	}

	// 2. 清索引数据（files/slides/file_tags + FTS 全表 + 缩略图记账表）
	if err := store.clearIndexForRebuild(); err != nil {
		return model.AssetScanResult{}, fmt.Errorf("清空素材索引失败: %w", err)
	}

	// 2.1 缩略图整目录清空重来（research §4.2：重建语义下逐条保留无意义——
	// 重解析后页 id 全变、页数可能变，记账已清，残留 PNG 由补缺链路重导出
	// 幂等覆盖；目录删除失败仅告警不中断，残留文件无害）
	if err := os.RemoveAll(s.assetThumbsDir(lib.ID)); err != nil {
		Logger().Warn("清空缩略图目录失败（重建后残留文件将被覆盖）", "libraryId", lib.ID, "err", err)
	}
	// 2.2 同步清内存 dirty 旧条目：重建后旧 fileID 的 slides 已删除，残留 dirty
	// flush 时会因外键约束失败整批丢弃（含重建后新记账）并产生告警噪音
	s.thumbLedger.dropLibrary(lib.ID)

	// 3. 全量扫描（库存记录已空，全部文件走新增解析路径，FTS 随写路径双写）
	result, err := s.scanLibrary(ctx, store, lib)
	if err != nil {
		return model.AssetScanResult{}, err
	}

	// 4. 标签重绑（path 未变的文件恢复用户标注）
	if len(bindings) > 0 {
		rebound, err := store.rebindFileTagsAfterRebuild(bindings)
		if err != nil {
			return model.AssetScanResult{}, fmt.Errorf("重绑文件标签失败: %w", err)
		}
		Logger().Info("assets rebuild tags rebound", "libraryId", lib.ID, "rebound", rebound)
	}
	return result, nil
}

// storeForScan 扫描专用连接获取（锁内取缓存连接）。扫描期间连接的关闭保护：
// DeleteLibrary 经 tryLockScan 与在途扫描互斥，不会并发关闭本连接。
func (s *AssetLibraryService) storeForScan(libraryID string) (*assetsStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	store, err := s.storeForLocked(libraryID)
	if err != nil {
		return nil, fmt.Errorf("打开素材索引库失败: %w", err)
	}
	return store, nil
}

// scanLibrary 扫描主体：磁盘清单收集 → 与库存记录差分 → 逐文件处理 → 清理
// 已删记录。ctx 取消在文件间检查点生效（单文件处理内不中断）。
func (s *AssetLibraryService) scanLibrary(ctx context.Context, store *assetsStore, lib model.AssetLibrary) (model.AssetScanResult, error) {
	entries, err := collectPptxEntries(lib.RootPath)
	if err != nil {
		return model.AssetScanResult{}, err
	}

	records, err := store.listFileRecords()
	if err != nil {
		return model.AssetScanResult{}, fmt.Errorf("读取文件索引失败: %w", err)
	}
	recordByID := make(map[string]assetFileRecord, len(records))
	for _, rec := range records {
		recordByID[rec.path] = rec
	}

	result := model.AssetScanResult{Total: len(entries)}
	for i, e := range entries {
		if err := ctx.Err(); err != nil {
			return model.AssetScanResult{}, fmt.Errorf("扫描已取消: %w", err)
		}
		rec, exists := recordByID[e.rel]
		// 增量判定：mtime+size 均未变且非 processing 残留（上次扫描中断的
		// processing 行必重处理，自愈崩溃半态）→ 跳过
		if exists && rec.status != model.AssetFileStatusProcessing &&
			rec.size == e.size && rec.mtimeNs == e.mtimeNs {
			result.Unchanged++
			s.emitScanProgress(lib.ID, i+1, len(entries), e.rel)
			continue
		}
		outcome, err := s.processScanFile(store, lib, e, rec, exists)
		if err != nil {
			return model.AssetScanResult{}, err
		}
		switch outcome {
		case scanOutcomeNew:
			result.New++
		case scanOutcomeUpdated:
			result.Updated++
		case scanOutcomeFailed:
			result.Failed++
		case scanOutcomeEncrypted:
			result.Encrypted++
		case scanOutcomeDenied:
			result.Denied++
		}
		s.emitScanProgress(lib.ID, i+1, len(entries), e.rel)
	}

	// 清理磁盘已消失的文件记录（幻灯片/文件标签/缩略图记账随事务清除；缩略图
	// PNG 文件在事务提交后删除——文件系统操作不可回滚，db 先行、文件后删）
	var removed int64
	gone := make([]string, 0)
	diskPaths := make(map[string]bool, len(entries))
	for _, e := range entries {
		diskPaths[e.rel] = true
	}
	for _, rec := range records {
		if !diskPaths[rec.path] {
			gone = append(gone, rec.path)
		}
	}
	if len(gone) > 0 {
		if err := ctx.Err(); err != nil {
			return model.AssetScanResult{}, fmt.Errorf("扫描已取消: %w", err)
		}
		deletedIDs, removedN, err := store.deleteFilesByPaths(gone)
		if err != nil {
			return model.AssetScanResult{}, fmt.Errorf("清理已删除文件记录失败: %w", err)
		}
		removed = removedN
		thumbsDir := s.assetThumbsDir(lib.ID)
		for _, fileID := range deletedIDs {
			// 记账 dirty 同步失效（防 flush 复活已删行——幽灵行虽无害，显式清
			// 更精确）；PNG 文件删除失败仅告警不中断（残留孤儿文件无害，下次
			// 同 ID 复用场景由重导出覆盖）
			s.thumbLedger.invalidateFile(lib.ID, fileID)
			removeThumbFilesForFile(thumbsDir, fileID)
		}
	}
	result.Removed = int(removed)
	return result, nil
}

// invalidateFileThumbs 缩略图失效联动（文件重解析前调用，parse 成败两分支均生效）：
// 记账表单语句删除该文件全部行 + 清 ledger dirty（防复活）+ 删 small/hd 两档旧
// PNG 文件。外键 ON DELETE CASCADE 为兜底，显式 DELETE 为契约（双保险：级联触发
// 时机不受连接池调度影响，显式删除保证记账与 PNG 文件同批收口）。PNG 前缀用
// `fileID + "-"` 完整形态（禁裸数字前缀——`102` 会误伤 `1023-…`，research §4.3）。
// 任一步失败仅告警不中断扫描：缓存清理幂等可自愈（下次扫描重处理再清）。
func (s *AssetLibraryService) invalidateFileThumbs(lib model.AssetLibrary, store *assetsStore, fileID int64) {
	if err := store.deleteThumbCacheForFile(fileID); err != nil {
		Logger().Warn("清除缩略图记账失败", "libraryId", lib.ID, "fileId", fileID, "err", err)
	}
	s.thumbLedger.invalidateFile(lib.ID, fileID)
	removeThumbFilesForFile(s.assetThumbsDir(lib.ID), fileID)
}

// removeThumbFilesForFile 删除指定文件的全部缩略图 PNG（small/hd 两档子目录遍历，
// `fileID + "-"` 精确前缀匹配防误伤）。失效联动与文件删除联动共用的单点函数。
func removeThumbFilesForFile(thumbsDir string, fileID int64) {
	prefix := strconv.FormatInt(fileID, 10) + "-"
	for _, tier := range thumbTiers {
		dir := filepath.Join(thumbsDir, tier.Key)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				Logger().Warn("枚举缩略图目录失败", "dir", dir, "err", err)
			}
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
				continue
			}
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !os.IsNotExist(err) {
				Logger().Warn("删除缩略图文件失败", "path", filepath.Join(dir, e.Name()), "err", err)
			}
		}
	}
}

// emitScanProgress 逐文件进度事件（done 从 1 计，含未变跳过的文件）。
func (s *AssetLibraryService) emitScanProgress(libraryID string, done, total int, currentFile string) {
	s.emitCurrent("assets-scan:progress", model.AssetScanProgress{
		LibraryID:   libraryID,
		Done:        done,
		Total:       total,
		CurrentFile: currentFile,
	})
}

// processScanFile 单文件处理：占位/标记 processing → 哈希（仅重处理时计算）→
// 解析 → 终态落库。解析失败按分类降级为 failed/encrypted/denied 状态并返回
// 对应 outcome（不中断批量）；DB 写失败为系统性错误上抛中断扫描。
func (s *AssetLibraryService) processScanFile(store *assetsStore, lib model.AssetLibrary, e pptxDiskEntry, rec assetFileRecord, exists bool) (scanOutcome, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	baseName := strings.TrimSuffix(path.Base(e.rel), path.Ext(e.rel))

	fileID := rec.id
	if !exists {
		id, err := store.insertProcessingFile(e.rel, baseName, e.size, e.mtimeNs, now)
		if err != nil {
			return 0, fmt.Errorf("写入文件占位记录失败（%s）: %w", e.rel, err)
		}
		fileID = id
	} else if err := store.markProcessing(fileID, now); err != nil {
		return 0, fmt.Errorf("更新文件处理状态失败（%s）: %w", e.rel, err)
	} else {
		// 缩略图失效联动（重解析清旧图）：文件内容已变旧图必失效，置于解析
		// 之前——parse 成败两分支全覆盖（解析失败降级 failed/encrypted 状态的
		// 文件同样不得残留旧缩略图）。任一步失败仅告警不中断扫描：缓存清理
		// 幂等可自愈（下次扫描重处理再清）。新文件分支无旧图，不触发。
		s.invalidateFileThumbs(lib, store, fileID)
	}

	parse := s.parseFn
	if parse == nil {
		parse = parsePptxSlides
	}
	slides, parseErr := parse(e.abs, baseName)

	// 哈希：仅在判定需要重处理（新文件/mtime+size 变动）时计算（增量契约：
	// 未变文件不重读内容）。文件不可读（权限不足）时哈希留空。
	hash := ""
	if parseErr == nil || !isDeniedParseError(parseErr) {
		if h, err := hashFileSHA256(e.abs); err != nil {
			Logger().Warn("assets scan hash failed", "libraryId", lib.ID, "file", e.rel, "err", err)
		} else {
			hash = h
		}
	}

	if parseErr != nil {
		status := model.AssetFileStatusFailed
		outcome := scanOutcomeFailed
		var pe *assetParseError
		if errors.As(parseErr, &pe) {
			switch pe.kind {
			case assetParseEncrypted:
				status = model.AssetFileStatusEncrypted
				outcome = scanOutcomeEncrypted
			case assetParseDenied:
				status = model.AssetFileStatusDenied
				outcome = scanOutcomeDenied
			}
		}
		Logger().Warn("assets scan file not indexed", "libraryId", lib.ID, "file", e.rel,
			"status", status, "err", parseErr)
		if err := store.setFileOutcome(fileID, e.size, e.mtimeNs, hash, status, now); err != nil {
			return 0, fmt.Errorf("写入文件状态失败（%s）: %w", e.rel, err)
		}
		return outcome, nil
	}

	if err := store.replaceFileSlides(fileID, e.size, e.mtimeNs, hash, now, slides); err != nil {
		return 0, fmt.Errorf("写入文件与幻灯片索引失败（%s）: %w", e.rel, err)
	}
	if exists {
		return scanOutcomeUpdated, nil
	}
	return scanOutcomeNew, nil
}

// isDeniedParseError 权限不足分类判定（权限下文件不可读，哈希跳过）。
func isDeniedParseError(err error) bool {
	var pe *assetParseError
	if errors.As(err, &pe) {
		return pe.kind == assetParseDenied
	}
	return false
}

// hashFileSHA256 流式计算文件 sha256（hex），仅增量重处理路径调用。
func hashFileSHA256(absPath string) (string, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// collectPptxEntries 递归收集库根目录下待索引 pptx 清单（相对路径字典序，进度
// 与结果顺序确定）。目录级异常（无权限子目录）降级跳过不中断；PPT 拆分派生
// 副本（同名文件夹布局）排除不索引。
func collectPptxEntries(root string) ([]pptxDiskEntry, error) {
	var entries []pptxDiskEntry
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// 目录不可入（权限/消失等）：剪除该枝继续扫其余目录，不中断批量
			Logger().Warn("assets scan entry inaccessible", "path", p, "err", err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(path.Ext(p), ".pptx") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			Logger().Warn("assets scan stat failed", "path", p, "err", err)
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("计算相对路径失败（%s）: %w", p, err)
		}
		relSlash := filepath.ToSlash(rel)
		if isPptxSplitDerivative(relSlash) {
			return nil // 拆分派生副本：不入索引（防与源文件重复入库）
		}
		entries = append(entries, pptxDiskEntry{
			rel:     relSlash,
			abs:     p,
			size:    info.Size(),
			mtimeNs: info.ModTime().UnixNano(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描素材目录失败: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	return entries, nil
}
