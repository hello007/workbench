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
// <dir>/<name>/<name>-<两位页序号>.pptx（同名文件夹 + 同名-页序命名，页序 %02d
// 对 ≥100 页输出三位数字）。仅排除严格符合该布局的文件；用户自组织的「同名目录/
// 同名-数字.pptx」与拆分产物同构不可区分，同样被排除（防重优先的取舍）。
func isPptxSplitDerivative(rel string) bool {
	parentBase := path.Base(path.Dir(rel))
	name := path.Base(rel)
	stem := strings.TrimSuffix(name, ".pptx")
	if stem == name {
		return false // 非 .pptx 后缀
	}
	sep := len(parentBase)
	if len(stem) < sep+2 || stem[:sep] != parentBase || stem[sep] != '-' {
		return false // 前缀非父目录名或无「-页序」段（含根目录文件：parentBase="."）
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
	return result, nil
}

// rebuildLibrary 全量重建主体：备份标签绑定 → 清 files/FTS → 全量扫描（记录
// 已清空，全部文件按新增处理）→ 按路径映射重绑标签。任何一步失败即中断
// （部分重建态由再次重建自愈；清理与扫描非同一事务，中断后索引可能为空，
// 语义与「重建后重新扫描」一致，不阻塞增量扫描修复）。
func (s *AssetLibraryService) rebuildLibrary(ctx context.Context, store *assetsStore, lib model.AssetLibrary) (model.AssetScanResult, error) {
	// 1. 备份用户标签绑定（重建清 files 时 file_tags 级联清除）
	bindings, err := store.fileTagBindingsByPath()
	if err != nil {
		return model.AssetScanResult{}, fmt.Errorf("备份文件标签绑定失败: %w", err)
	}

	// 2. 清索引数据（files/slides/file_tags + FTS 全表）
	if err := store.clearIndexForRebuild(); err != nil {
		return model.AssetScanResult{}, fmt.Errorf("清空素材索引失败: %w", err)
	}

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

	// 清理磁盘已消失的文件记录（幻灯片/文件标签随外键级联清除）
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
		if removed, err = store.deleteFilesByPaths(gone); err != nil {
			return model.AssetScanResult{}, fmt.Errorf("清理已删除文件记录失败: %w", err)
		}
	}
	result.Removed = int(removed)
	return result, nil
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
