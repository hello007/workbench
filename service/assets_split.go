package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"workbench/model"
)

// 素材拆分链路（素材管理第三批，SRS §2.2.3 批量目录拆分 / §2.2.5 方式 2）：
// 面板内 PPT 按页拆分（单文件 + 库级批量），复用工具箱拆分核心
// SplitPptxPerPageWithProgress（禁重写，拆分方法论契约见 docs/spec/pptx-split.md）。
// 本文件只做素材域编排：落点选择（derived 镜像源树 / 源目录）、增量拆分判定
// （split_records + 输出存在双验）、重拆清旧页、批量异常隔离、取消注册与事件。
//
// 落点布局（PRD Q4）：
//   - 缓存目录（默认）：data/assets/<library-id>/derived/<源相对目录>/<源名主干>/
//     <源名>-NNN.pptx（镜像源树：根目录文件恰为 SRS 字面 derived/{源文件名}/ 布局；
//     子目录文件多一层镜像目录，同库不同目录同名 pptx 撞目录结构性消除）。落点在
//     库根之外，扫描天然不重复索引。
//   - 源目录（outputToSourceDir=true）：<源所在目录>/<源名主干>/——工具箱既有布局，
//     isPptxSplitDerivative 已覆盖排除（assets_scan.go），零扩展。
//
// 并发模型：
//   - 拆分互斥：同库同时仅一个拆分任务（tryLockSplit，独立 splitLocks——拆分与
//     扫描语义可并发：拆分不写 files/slides 索引表，仅读写 split_records，单连接
//     池串行化）；DeleteLibrary 复用本锁与在途拆分互斥（拆分中禁删库）。
//   - 取消注册：独立 splitCancels 注册表（与扫描 cancels 键空间独立）——CancelScan
//     不得误杀拆分，CancelAssetSplit 亦不影响扫描；defer LIFO 先注销后释放锁
//     （docs/spec/logging-and-errors.md 取消链路契约）。取消错误经
//     mapAssetSplitCancelError 映射 E_ASSETS_CANCELLED（复用既有错误码，禁新增；
//     消息按拆分措辞与扫描 sentinel 区分）。
//
// 事件（EventSink，assets-split:* 前缀，前端素材面板独听、与工具箱 pptx-split:*
// 隔离）：progress（文件级 done/total + 页级 pageDone/pageTotal 透传核心回调）/
// done（批量汇总，单文件同形状 Total=1）/ error（整体失败才发；批量内单文件失败
// 走结果统计不发 error）。

// splitSkipReasonUpToDate 增量命中跳过的统一原因文案（结果明细与事件共用）。
const splitSkipReasonUpToDate = "已拆分且源未变化"

// errAssetSplitCancelled 素材拆分被用户取消（复用 E_ASSETS_CANCELLED 错误码——
// 前端 warning 分流按 code；消息按拆分措辞，与扫描 sentinel ErrAssetsCancelled
// 的「扫描已取消」区分）。
var errAssetSplitCancelled = model.NewAppError(model.ErrCodeAssetsCancelled, "拆分已取消")

// mapAssetSplitCancelError 将 ctx 取消错误（%w 包装 context.Canceled 透传）映射为
// 拆分文案的 errAssetSplitCancelled（错误码与扫描取消同码复用，禁新增错误码），
// 其余错误原样透传。
func mapAssetSplitCancelError(err error) error {
	if errors.Is(err, context.Canceled) {
		return errAssetSplitCancelled
	}
	return err
}

// assetsSplitCore 拆分核心注入点（默认 SplitPptxPerPageWithProgress；单测注入
// fake 驱动阻塞/取消与核心调用断言，对齐 parseFn 注入模式）。
type assetsSplitCore func(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error)

// splitOutcome 单文件拆分处理结果（单文件入口与批量逐文件共用的中间形态）。
type splitOutcome struct {
	rel       string                 // 文件相对路径（结果明细用）
	status    string                 // 处理状态（model.AssetSplitStatus* 取值）
	message   string                 // 跳过原因/失败原因（成功为空）
	result    model.AssetSplitResult // 拆分结果（status=split/skipped 时有效）
	pageDone  int                    // 收尾进度事件页级计数（成功/跳过 = 页数；失败 = 0）
	pageTotal int
}

// assetDerivedDir 库拆分产物根目录单点（data/assets/<library-id>/derived，镜像
// 源树落点根，与 thumbs 平级）。所有 derived 目录来源统一走本方法：落点解析、
// DeleteLibrary 清理，防路径口径漂移（对齐 assetThumbsDir 模式）。
func (s *AssetLibraryService) assetDerivedDir(libraryID string) string {
	return filepath.Join(s.assetsDir, libraryID, "derived")
}

// SplitAssetFile 拆分单个素材文件（画廊卡/详情侧边栏入口）：落点二选一，增量
// 命中秒回「已最新」（Skipped=true）不重拆。核心拆分失败（损坏/加密/只读等）
// 升格为整体失败——单文件入口无批量隔离语义，错误返回并发 assets-split:error
// （与扫描链路失败发 error 事件同构）。
func (s *AssetLibraryService) SplitAssetFile(ctx context.Context, libraryID, relPath string, outputToSourceDir bool) (model.AssetSplitResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetSplitResult{}, err
	}
	rel, absPath, err := resolveLibraryRelPath(lib.RootPath, relPath)
	if err != nil {
		return model.AssetSplitResult{}, err
	}

	release, err := s.tryLockSplit(libraryID)
	if err != nil {
		return model.AssetSplitResult{}, err
	}
	defer release()
	// 锁内复验库存在：getLibrary 与拿拆分锁的窗口内库可能被 DeleteLibrary 删除
	//（删除与拆分同锁互斥），对齐 runLibraryScan 锁内复验口径
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetSplitResult{}, err
	}

	splitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	entry := s.registerSplitCancel(libraryID, cancel)
	defer s.unregisterSplitCancel(libraryID, entry)

	store, err := s.storeForScan(libraryID)
	if err != nil {
		return model.AssetSplitResult{}, err
	}

	// 源记录校验 + 源哈希（files.hash 列为扫描增量链路维护的权威；单文件入口
	// 逐条查询，批量入口由快照批量带出）
	hash, status, err := store.fileHashStatusByPath(rel)
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("文件尚未索引或已从素材库移除，请先重新扫描")
	} else if err != nil {
		err = fmt.Errorf("读取文件索引失败（%s）: %w", rel, err)
	} else if status != model.AssetFileStatusOK {
		err = fmt.Errorf("文件当前状态不可拆分（%s），请先重新扫描确认文件可用", status)
	}
	if err != nil {
		Logger().Error("assets split failed", "libraryId", libraryID, "file", rel, "err", err)
		s.emitCurrent("assets-split:error", map[string]any{"libraryId": libraryID, "message": appErrMessage(err)})
		return model.AssetSplitResult{}, err
	}

	outcome, err := s.splitOneFileWithHash(splitCtx, store, lib, rel, absPath, hash, outputToSourceDir, 1, 1)
	if err != nil {
		Logger().Error("assets split failed", "libraryId", libraryID, "file", rel, "err", err)
		err = mapAssetSplitCancelError(err)
		s.emitCurrent("assets-split:error", map[string]any{"libraryId": libraryID, "message": appErrMessage(err)})
		return model.AssetSplitResult{}, err
	}
	if outcome.status == model.AssetSplitStatusFailed {
		// 核心拆分失败：单文件入口升格为整体失败（错误返回 + error 事件）
		err = errors.New(outcome.message)
		Logger().Error("assets split failed", "libraryId", libraryID, "file", rel, "err", err)
		s.emitCurrent("assets-split:error", map[string]any{"libraryId": libraryID, "message": outcome.message})
		return model.AssetSplitResult{}, err
	}
	// 文件收尾进度（done 计入本文件，与批量逐文件口径一致；跳过同样发收尾事件）
	s.emitSplitProgress(libraryID, 1, 1, rel, outcome.pageDone, outcome.pageTotal)
	Logger().Info("assets split done", "libraryId", libraryID, "file", rel,
		"outputDir", outcome.result.OutputDir, "pages", outcome.result.PageCount,
		"skipped", outcome.result.Skipped)
	s.emitCurrent("assets-split:done", map[string]any{"libraryId": libraryID, "result": singleOutcomeBatch(outcome)})
	return outcome.result, nil
}

// SplitAssetLibrary 库级批量拆分（库面板「拆分全部 PPT」入口）：启动时快照圈定
// 全部 ok 状态文件逐个处理（对齐 SRS「批量目录拆分」；快照后文件变更/删除不
// 影响本批圈定，重拆判定按当批快照哈希）。单文件失败（损坏/加密/只读/磁盘等）
// 逐文件记日志跳过，批量不中断（异常隔离）；取消在文件间检查点生效，中断剩余
// 并整体失败返回（E_ASSETS_CANCELLED，注册表 defer 注销无残留）。
func (s *AssetLibraryService) SplitAssetLibrary(ctx context.Context, libraryID string, outputToSourceDir bool) (model.AssetSplitBatchResult, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetSplitBatchResult{}, err
	}

	release, err := s.tryLockSplit(libraryID)
	if err != nil {
		return model.AssetSplitBatchResult{}, err
	}
	defer release()
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetSplitBatchResult{}, err
	}

	splitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	entry := s.registerSplitCancel(libraryID, cancel)
	defer s.unregisterSplitCancel(libraryID, entry)

	store, err := s.storeForScan(libraryID)
	if err != nil {
		return model.AssetSplitBatchResult{}, err
	}

	// 快照圈定：全部 ok 状态文件（相对路径 + 哈希，路径序——进度与结果顺序确定）
	sources, err := store.listOKFileSplitSources()
	if err != nil {
		return model.AssetSplitBatchResult{}, fmt.Errorf("读取文件清单失败: %w", err)
	}

	batch := model.AssetSplitBatchResult{Total: len(sources),
		Items: make([]model.AssetSplitFileOutcome, 0, len(sources))}
	for i, src := range sources {
		// 文件间取消检查点（对齐扫描链路：单文件拆分内不中断）
		if err := splitCtx.Err(); err != nil {
			return s.abortAssetSplit(libraryID, src.rel, fmt.Errorf("拆分已取消: %w", err))
		}
		absPath := filepath.Join(lib.RootPath, filepath.FromSlash(src.rel))
		outcome, err := s.splitOneFileWithHash(splitCtx, store, lib, src.rel, absPath, src.hash, outputToSourceDir, i+1, len(sources))
		if err != nil {
			// 系统性错误（取消/db 读写失败）：整体失败，批量中止
			return s.abortAssetSplit(libraryID, src.rel, err)
		}
		batch.Items = append(batch.Items, model.AssetSplitFileOutcome{
			RelPath: src.rel, Status: outcome.status, Message: outcome.message,
		})
		switch outcome.status {
		case model.AssetSplitStatusSplit:
			batch.Split++
		case model.AssetSplitStatusSkipped:
			batch.Skipped++
		default:
			// 批量异常隔离：单文件失败记日志跳过，不中断批量（SRS §2.2.3）
			batch.Failed++
			Logger().Warn("素材拆分单文件失败（批量隔离继续）",
				"libraryId", libraryID, "file", src.rel, "reason", outcome.message)
		}
		// 文件收尾进度（done 计入本文件，完成数口径对齐 assets-scan:progress）
		s.emitSplitProgress(lib.ID, i+1, len(sources), src.rel, outcome.pageDone, outcome.pageTotal)
	}

	Logger().Info("assets split batch done", "libraryId", libraryID, "total", batch.Total,
		"split", batch.Split, "skipped", batch.Skipped, "failed", batch.Failed)
	s.emitCurrent("assets-split:done", map[string]any{"libraryId": libraryID, "result": batch})
	return batch, nil
}

// abortAssetSplit 批量整体失败收口：日志记根因 + 取消映射 + error 事件，错误
// 返回（对齐 runLibraryScan 失败路径的「日志原始错误、事件与返回用映射后用户
// 可读形态」口径）。
func (s *AssetLibraryService) abortAssetSplit(libraryID, file string, err error) (model.AssetSplitBatchResult, error) {
	Logger().Error("assets split aborted", "libraryId", libraryID, "file", file, "err", err)
	err = mapAssetSplitCancelError(err)
	s.emitCurrent("assets-split:error", map[string]any{"libraryId": libraryID, "message": appErrMessage(err)})
	return model.AssetSplitBatchResult{}, err
}

// splitOneFileWithHash 单文件拆分完整链路（单文件入口与批量逐文件共用）：增量
// 判定 → 重拆清旧页 → 调核心拆分（页级进度透传）→ 落拆分记录。
//
// 错误语义（批量异常隔离的关键分叉）：
//   - 系统性错误（取消/db 读写失败）→ error 返回，批量中止；
//   - 核心拆分失败（损坏/加密/只读/磁盘等）→ status=failed 结果返回（批量计
//     Failed++ 继续后续文件；单文件入口由调用方升格为错误）。
//
// snapshotHash 为调用方提供的源内容哈希（单文件入口查 files.hash 列，批量入口
// 快照带出）；为空（权限跳过/旧版本扫描）时现算——无哈希无法做增量判定。
func (s *AssetLibraryService) splitOneFileWithHash(ctx context.Context, store *assetsStore, lib model.AssetLibrary, rel, absPath, snapshotHash string, outputToSourceDir bool, fileDone, fileTotal int) (splitOutcome, error) {
	failed := func(format string, args ...any) (splitOutcome, error) {
		return splitOutcome{rel: rel, status: model.AssetSplitStatusFailed,
			message: fmt.Sprintf(format, args...)}, nil
	}

	// 1. 源哈希：快照为空时现算（文件不可读时哈希与拆分同样不可得，归 failed）
	hash := snapshotHash
	if hash == "" {
		h, hashErr := hashFileSHA256(absPath)
		if hashErr != nil {
			return failed("读取源文件失败: %v", hashErr)
		}
		hash = h
	}

	// 2. 落点解析与源名主干（产物命名 `<源名主干>-NNN.pptx`，与核心同口径）
	baseName := splitSourceBaseName(rel)
	outDir := s.splitOutputDir(lib, rel, absPath, outputToSourceDir)

	// 3. 增量判定：记录命中 + 哈希一致 + 产物首页存在（双验，对齐缩略图补缺
	//    「PNG 存在唯一权威」先例——手删产物误判已拆被文件存在检查兜住）
	record, err := store.getSplitRecord(rel)
	if err != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("读取拆分记录失败: %w", err)
	}
	if record != nil && record.sourceHash == hash &&
		fileExists(splitFirstPagePath(record.outputDir, baseName)) {
		return splitOutcome{
			rel: rel, status: model.AssetSplitStatusSkipped, message: splitSkipReasonUpToDate,
			result: model.AssetSplitResult{
				OutputDir: record.outputDir, PageCount: record.pageCount,
				Skipped: true, SkipReason: splitSkipReasonUpToDate,
			},
			pageDone: record.pageCount, pageTotal: record.pageCount,
		}, nil
	}

	// 4. 重拆清旧页（PRD：防页数减少残留 + 旧 %02d 产物收敛）。删除失败仅告警
	//    继续拆（同名单文件覆盖写幂等，残留由下次重拆收敛）
	if err := os.RemoveAll(outDir); err != nil {
		Logger().Warn("清空拆分产物目录失败（覆盖写继续，残留由下次重拆收敛）",
			"libraryId", lib.ID, "dir", outDir, "err", err)
	}

	// 5. 调拆分核心（页级进度透传：文件级 done = 已完成文件数，当前文件进行中；
	//    文件收尾进度由调用方在完成后补发）。
	// 核心输出目录由「源文件所在目录 + 源文件名主干」决定（拆分方法论契约「源
	// 同级同名文件夹」，核心入口签名禁改）——derived 落点经「源副本代拆」实现：
	// 把源文件复制到目标父目录下的 <源名主干>.pptx，核心即在 <目标父目录>/<源名
	// 主干>/ 产出（恰为落点 outDir），拆分结束（成败）删副本。代价 = 源文件一次
	// 拷贝，换得产物全程不落源目录（无并发扫描观察窗口）且无跨卷搬移。源目录
	// 落点核心原生输出恰为目标，直调零拷贝。
	splitSrc := absPath
	if !outputToSourceDir {
		parent := filepath.Dir(outDir)
		if err := os.MkdirAll(parent, 0755); err != nil {
			return splitOutcome{rel: rel}, fmt.Errorf("创建拆分产物目录失败: %w", err)
		}
		splitSrc = filepath.Join(parent, baseName+path.Ext(rel))
		if err := copyFileStream(absPath, splitSrc); err != nil {
			return failed("复制源文件副本失败: %v", err)
		}
		defer os.Remove(splitSrc) // 成败均删副本（NotExist 忽略）；崩溃残留无害，下次重拆覆盖
	}
	core := s.splitCoreFn
	if core == nil {
		core = SplitPptxPerPageWithProgress
	}
	result, err := core(ctx, splitSrc, func(done, total int) {
		s.emitSplitProgress(lib.ID, fileDone-1, fileTotal, rel, done, total)
	})
	if err != nil {
		// 取消：整批中止语义（核心 %w 包装 context.Canceled 透传，errors.Is 可判
		// ——不把取消计为单文件失败隔离继续）
		if errors.Is(err, context.Canceled) {
			return splitOutcome{rel: rel}, fmt.Errorf("拆分已取消: %w", err)
		}
		return failed("%s", appErrMessage(err))
	}

	// 6. 落拆分记录（成功才记；失败/取消不记，下次触发重走完整判定）
	if err := store.upsertSplitRecord(rel, hash, result.OutputDir, result.PageCount,
		time.Now().Format("2006-01-02 15:04:05")); err != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("写入拆分记录失败（%s）: %w", rel, err)
	}
	return splitOutcome{
		rel: rel, status: model.AssetSplitStatusSplit,
		result: model.AssetSplitResult{
			OutputDir: result.OutputDir, Files: result.Files, PageCount: result.PageCount,
		},
		pageDone: result.PageCount, pageTotal: result.PageCount,
	}, nil
}

// GetAssetSplitRecord 查询文件拆分记录（跳转方式 2 判定面：前端据 Exists 展示
// 「打开拆分单页」按钮、据 UpToDate 提示「已最新」）。查询面宽容语义：文件索引
// 记录缺失（已删/未扫描）不报错，UpToDate 按否处理。
func (s *AssetLibraryService) GetAssetSplitRecord(libraryID, relPath string) (model.AssetSplitRecordView, error) {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetSplitRecordView{}, err
	}
	rel, _, err := resolveLibraryRelPath(lib.RootPath, relPath)
	if err != nil {
		return model.AssetSplitRecordView{}, err
	}
	var view model.AssetSplitRecordView
	err = s.withStore(libraryID, func(store *assetsStore) error {
		record, err := store.getSplitRecord(rel)
		if err != nil || record == nil {
			return err // 未拆分：Exists=false 零值返回（非错误）
		}
		view = model.AssetSplitRecordView{
			Exists:    true,
			OutputDir: record.outputDir,
			PageCount: record.pageCount,
			SplitAt:   record.splitAt,
		}
		// UpToDate：记录哈希与当前源哈希一致（files.hash 列为基准，为空现算；
		// 哈希不可得按否处理，不阻塞查询语义）
		hash, _, err := store.fileHashStatusByPath(rel)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // 文件索引记录已消失：UpToDate=false
		}
		if err != nil {
			return err
		}
		if hash == "" {
			h, hashErr := hashFileSHA256(filepath.Join(lib.RootPath, filepath.FromSlash(rel)))
			if hashErr != nil {
				return nil
			}
			hash = h
		}
		view.UpToDate = record.sourceHash == hash
		return nil
	})
	if err != nil {
		return model.AssetSplitRecordView{}, err
	}
	return view, nil
}

// OpenAssetSplitPage 打开拆分单页产物（SRS §2.2.5 方式 2，无 PowerPoint COM 依赖）：
// 查拆分记录（无记录报「尚未拆分」）→ 产物路径 = <record.output_dir>/<源名主干>-
// <slideNum %03d>.pptx → 校验页序 ∈ [1, record.page_count] 且文件存在 → 系统默认
// 关联程序打开（openWithSystemHandler 跨平台实现，经注入点隔离测试）。
func (s *AssetLibraryService) OpenAssetSplitPage(libraryID, relPath string, slideNum int) error {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return err
	}
	rel, _, err := resolveLibraryRelPath(lib.RootPath, relPath)
	if err != nil {
		return err
	}
	if slideNum < 1 {
		return fmt.Errorf("页序非法（slideNum=%d，从 1 计）", slideNum)
	}
	var pagePath string
	err = s.withStore(libraryID, func(store *assetsStore) error {
		record, err := store.getSplitRecord(rel)
		if err != nil {
			return err
		}
		if record == nil {
			return errors.New("该文件尚未拆分，请先执行拆分")
		}
		if slideNum > record.pageCount {
			return fmt.Errorf("页序超出已拆分页数（slideNum=%d，已拆分 %d 页）", slideNum, record.pageCount)
		}
		pagePath = filepath.Join(record.outputDir,
			fmt.Sprintf("%s-%03d.pptx", splitSourceBaseName(rel), slideNum))
		return nil
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(pagePath); err != nil {
		return fmt.Errorf("拆分产物不存在或已被清理（%s）: %w", filepath.Base(pagePath), err)
	}
	open := s.openSplitPageFn
	if open == nil {
		open = NewFileOperationService().OpenWithSystemHandler
	}
	return open(pagePath)
}

// CancelAssetSplit 取消指定库的在途拆分（对齐 CancelScan 交互语义）。返回是否
// 存在可取消的在途拆分；无在途拆分幂等返回 false（CancelFunc 幂等，取消与注销
// 的竞态无危害）。
func (s *AssetLibraryService) CancelAssetSplit(libraryID string) bool {
	key := normalizeRepoKey(libraryID)
	s.opMu.Lock()
	entry, ok := s.splitCancels[key]
	if ok {
		delete(s.splitCancels, key)
	}
	s.opMu.Unlock()
	if !ok {
		return false
	}
	entry.cancel()
	return true
}

// ---------- 内部辅助 ----------

// singleOutcomeBatch 单文件结果转批量形状（done 事件单/批同形状：Total=1）。
func singleOutcomeBatch(o splitOutcome) model.AssetSplitBatchResult {
	batch := model.AssetSplitBatchResult{Total: 1, Items: []model.AssetSplitFileOutcome{{
		RelPath: o.rel, Status: o.status, Message: o.message,
	}}}
	switch o.status {
	case model.AssetSplitStatusSplit:
		batch.Split = 1
	case model.AssetSplitStatusSkipped:
		batch.Skipped = 1
	default:
		batch.Failed = 1
	}
	return batch
}

// emitSplitProgress 拆分进度事件（文件级 done/total + 页级 pageDone/pageTotal，
// 载荷形状见 model.AssetSplitProgress 注释）。
func (s *AssetLibraryService) emitSplitProgress(libraryID string, done, total int, currentFile string, pageDone, pageTotal int) {
	s.emitCurrent("assets-split:progress", model.AssetSplitProgress{
		LibraryID: libraryID, Done: done, Total: total,
		CurrentFile: currentFile, PageDone: pageDone, PageTotal: pageTotal,
	})
}

// splitSourceBaseName 源文件名去扩展名（拆分产物命名主干，与 pptx_split.go 的
// baseName 同口径）。
func splitSourceBaseName(rel string) string {
	name := path.Base(rel)
	return strings.TrimSuffix(name, path.Ext(name))
}

// splitFirstPagePath 产物首页路径（增量判定的产物存在双验锚点：核心拆分从页 1
// 顺序写盘，001 恒存在；手删产物目录时「记录命中」误判已拆被本检查兜住）。
func splitFirstPagePath(outputDir, baseName string) string {
	return filepath.Join(outputDir, fmt.Sprintf("%s-%03d.pptx", baseName, 1))
}

// splitOutputDir 落点解析（PRD Q4 落点契约，见文件头注释布局说明）：
//   - 缓存目录（默认）：derived 镜像源树，目录段经 FromSlash 转平台分隔符；
//   - 源目录（outputToSourceDir=true）：<源所在目录>/<源名主干>/。
func (s *AssetLibraryService) splitOutputDir(lib model.AssetLibrary, rel, absPath string, outputToSourceDir bool) string {
	name := splitSourceBaseName(rel)
	if outputToSourceDir {
		return filepath.Join(filepath.Dir(absPath), name)
	}
	if dirPart := path.Dir(rel); dirPart != "." && dirPart != "/" && dirPart != "" {
		return filepath.Join(s.assetDerivedDir(lib.ID), filepath.FromSlash(dirPart), name)
	}
	return filepath.Join(s.assetDerivedDir(lib.ID), name)
}

// fileExists 文件存在性检查（增量双验的产物存在性口径；Stat 错误一律视为不存在
// ——权限异常态下按缺失处理重拆，重拆自身会暴露真实错误）。
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// copyFileStream 文件字节流式复制（derived 落点的源副本代拆用；目标已存在覆盖写，写失败
// 清理半成品）。
func copyFileStream(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

// resolveLibraryRelPath relPath 越界遏制（对齐 RetryAssetThumb 先例）：relPath
// 来自前端（浏览器通道可达），`..` 或绝对路径可逃逸库根拆分/读取库外文件。
// FromSlash 后拒 IsAbs，Join 后 Rel 校验无 `..` 前缀才放行；返回 Rel 清洗后的
// 正斜杠库内相对路径（顺带归一 "./x.pptx" 形态）与绝对路径。
func resolveLibraryRelPath(rootPath, relPath string) (rel, abs string, err error) {
	if strings.TrimSpace(relPath) == "" {
		return "", "", errors.New("文件相对路径不能为空")
	}
	native := filepath.FromSlash(relPath)
	if filepath.IsAbs(native) {
		return "", "", errors.New("文件路径非法（须为素材根目录内的相对路径）")
	}
	abs = filepath.Join(rootPath, native)
	back, relErr := filepath.Rel(rootPath, abs)
	if relErr != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", "", errors.New("文件路径越界（须位于素材根目录内）")
	}
	return filepath.ToSlash(back), abs, nil
}
