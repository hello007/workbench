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
	"strconv"
	"strings"
	"time"

	"workbench/model"
	"workbench/util"
)

// 素材拆分链路（素材管理第三批，SRS §2.2.3 批量目录拆分 / §2.2.5 方式 2）：
// 面板内 PPT 按页拆分（单文件 + 库级批量），复用工具箱拆分核心
// SplitPptxPerPageWithProgress（禁重写，拆分方法论契约见 docs/spec/pptx-split.md）。
// 本文件只做素材域编排：落点选择、增量拆分判定（split_records + 输出存在双验）、
// 重拆清旧页、就地替换的删源与标注迁移、批量异常隔离、取消注册与事件。
//
// 落点三态（B 方案 PR1，落点解析单点 normalizeSplitTarget）：
//   - 缓存目录（默认 cache）：data/assets/<library-id>/derived/<源相对目录>/<源名主干>/
//     <源名>-NNN.pptx（镜像源树：根目录文件恰为 SRS 字面 derived/{源文件名}/ 布局；
//     子目录文件多一层镜像目录，同库不同目录同名 pptx 撞目录结构性消除）。落点在
//     库根之外，扫描天然不重复索引。
//   - 源目录（source）：<源所在目录>/<源名主干>/——工具箱既有布局，
//     isPptxSplitDerivative 已覆盖排除（assets_scan.go），零扩展。
//   - 就地替换（replace）：<源所在目录>/<源名主干>/<3位页序>-<源名主干>.pptx——
//     页序前缀命名不命中 isPptxSplitDerivative，扫描后成为独立素材；全页成功 +
//     逐页 Stat 校验 + 标注迁移事务通过后把源文件移入系统回收站（util.MoveToRecycleBin）。
//
// 并发模型：
//   - 拆分互斥：同库同时仅一个拆分任务（tryLockSplit，独立 splitLocks）。
//   - replace 额外与扫描互斥：该落点需为目标产物写 files/slides 索引（复用扫描
//     单文件重链 processScanFile），与扫描并发写索引会互相覆盖。锁序固定
//     scan → split（与 DeleteLibrary 一致，无反向获取路径不成环）；cache/source
//     落点不写索引，仅取拆分锁（行为与改动前一致）。
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
	// code 失败项错误码（如 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED）：单文件入口
	// 升格为 AppError 返回，批量入口填入明细项供前端区分「已回滚」与「产物保留」
	code string
	// rolledBack replace 落点失败/取消时产物已回滚删除（批量明细展示用）
	rolledBack bool
}

// normalizeSplitTarget 落点参数归一与校验单点（App 层传入的原样字符串）：
// 空串兼容旧调用（等价 cache）；非法值明确拒绝，避免静默落到意外的落点。
func normalizeSplitTarget(target string) (string, error) {
	switch strings.TrimSpace(target) {
	case "", model.AssetSplitTargetCache:
		return model.AssetSplitTargetCache, nil
	case model.AssetSplitTargetSource:
		return model.AssetSplitTargetSource, nil
	case model.AssetSplitTargetReplace:
		return model.AssetSplitTargetReplace, nil
	default:
		return "", fmt.Errorf("拆分落点非法（%q）：仅支持 cache / source / replace", target)
	}
}

// tryLockScanForSplit 就地替换落点前置扫描锁（锁序 scan → split，与 DeleteLibrary
// 一致）：replace 要为目标产物写索引，必须与在途扫描互斥；cache/source 落点不写
// 索引，返回空 release 保持原行为（调用方 defer 调用安全）。
func (s *AssetLibraryService) tryLockScanForSplit(libraryID, target string) (release func(), err error) {
	if target != model.AssetSplitTargetReplace {
		return func() {}, nil
	}
	return s.tryLockScan(libraryID)
}

// assetDerivedDir 库拆分产物根目录单点（data/assets/<library-id>/derived，镜像
// 源树落点根，与 thumbs 平级）。所有 derived 目录来源统一走本方法：落点解析、
// DeleteLibrary 清理，防路径口径漂移（对齐 assetThumbsDir 模式）。
func (s *AssetLibraryService) assetDerivedDir(libraryID string) string {
	return filepath.Join(s.assetsDir, libraryID, "derived")
}

// SplitAssetFile 拆分单个素材文件（画廊卡/详情侧边栏入口）：落点三选一
// （cache / source / replace，见文件头注释），增量命中秒回「已最新」
// （Skipped=true）不重拆。核心拆分失败（损坏/加密/只读等）升格为整体失败——
// 单文件入口无批量隔离语义，错误返回并发 assets-split:error（与扫描链路失败发
// error 事件同构）；replace 落点的删源失败返回 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED
// （产物与索引保留，源亦保留，可关闭占用程序后重试）。
func (s *AssetLibraryService) SplitAssetFile(ctx context.Context, libraryID, relPath string, target string) (model.AssetSplitResult, error) {
	tgt, err := normalizeSplitTarget(target)
	if err != nil {
		return model.AssetSplitResult{}, err
	}
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetSplitResult{}, err
	}
	rel, absPath, err := resolveLibraryRelPath(lib.RootPath, relPath)
	if err != nil {
		return model.AssetSplitResult{}, err
	}

	// 锁序 scan → split（replace 需要与扫描互斥写索引；与 DeleteLibrary 同序，
	// 无反向获取路径不成环）
	releaseScan, err := s.tryLockScanForSplit(libraryID, tgt)
	if err != nil {
		return model.AssetSplitResult{}, err
	}
	defer releaseScan()

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

	// 在途拆分计数：WaitSplitsDone（关停收口）以此等待全部拆分退出，防 CloseAll
	// 关连接后拆分继续写 split_records（对齐 scanWG 模式）
	s.splitWG.Add(1)
	defer s.splitWG.Done()

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

	outcome, err := s.splitOneFileWithHash(splitCtx, store, lib, rel, absPath, hash, tgt, 1, 1)
	if err != nil {
		Logger().Error("assets split failed", "libraryId", libraryID, "file", rel, "err", err)
		err = mapAssetSplitCancelError(err)
		s.emitCurrent("assets-split:error", map[string]any{"libraryId": libraryID, "message": appErrMessage(err)})
		return model.AssetSplitResult{}, err
	}
	if outcome.status == model.AssetSplitStatusFailed {
		// 核心拆分失败：单文件入口升格为整体失败（错误返回 + error 事件）。
		// 带错误码的失败（删源失败）以 AppError 返回，前端按 code 走 warning 分流
		if outcome.code != "" {
			err = model.NewAppError(outcome.code, outcome.message)
		} else {
			err = errors.New(outcome.message)
		}
		Logger().Error("assets split failed", "libraryId", libraryID, "file", rel, "err", err)
		s.emitCurrent("assets-split:error", map[string]any{"libraryId": libraryID, "message": outcome.message})
		return model.AssetSplitResult{}, err
	}
	// 文件收尾进度（done 计入本文件，与批量逐文件口径一致；跳过同样发收尾事件）
	s.emitSplitProgress(libraryID, 1, 1, rel, outcome.pageDone, outcome.pageTotal)
	Logger().Info("assets split done", "libraryId", libraryID, "file", rel, "target", tgt,
		"outputDir", outcome.result.OutputDir, "pages", outcome.result.PageCount,
		"skipped", outcome.result.Skipped, "sourceRemoved", outcome.result.SourceRemoved)
	s.emitCurrent("assets-split:done", map[string]any{"libraryId": libraryID, "result": singleOutcomeBatch(outcome, tgt)})
	return outcome.result, nil
}

// SplitAssetLibrary 库级批量拆分（库面板「拆分全部 PPT」入口）：启动时快照圈定
// 全部 ok 状态文件逐个处理（对齐 SRS「批量目录拆分」；快照后文件变更/删除不
// 影响本批圈定，重拆判定按当批快照哈希）。单文件失败（损坏/加密/只读/磁盘等）
// 逐文件记日志跳过，批量不中断（异常隔离）；取消在文件间检查点生效，中断剩余
// 并整体失败返回（E_ASSETS_CANCELLED，注册表 defer 注销无残留）。落点三态同
// SplitAssetFile；replace 需与扫描互斥（锁序 scan → split）。
//
// replace 批量注意：前序文件成功拆分后源行已删除，后续文件的产物索引不会与
// 已删源冲突；快照圈定发生在任何删除之前，故 Items 仍覆盖全部源文件。
func (s *AssetLibraryService) SplitAssetLibrary(ctx context.Context, libraryID string, target string) (model.AssetSplitBatchResult, error) {
	tgt, err := normalizeSplitTarget(target)
	if err != nil {
		return model.AssetSplitBatchResult{}, err
	}
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return model.AssetSplitBatchResult{}, err
	}

	// 锁序 scan → split（replace 与扫描互斥；cache/source 仅取拆分锁）
	releaseScan, err := s.tryLockScanForSplit(libraryID, tgt)
	if err != nil {
		return model.AssetSplitBatchResult{}, err
	}
	defer releaseScan()

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

	// 在途拆分计数：WaitSplitsDone（关停收口）以此等待全部拆分退出（对齐
	// scanWG 模式，见 SplitAssetFile 同款注释）
	s.splitWG.Add(1)
	defer s.splitWG.Done()

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
		outcome, err := s.splitOneFileWithHash(splitCtx, store, lib, src.rel, absPath, src.hash, tgt, i+1, len(sources))
		if err != nil {
			// 系统性错误（取消/db 读写失败）：整体失败，批量中止
			return s.abortAssetSplit(libraryID, src.rel, err)
		}
		batch.Items = append(batch.Items, model.AssetSplitFileOutcome{
			RelPath: src.rel, Status: outcome.status, Message: outcome.message,
			Target: tgt, SourceRemoved: outcome.result.SourceRemoved,
			RolledBack: outcome.rolledBack, Code: outcome.code,
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
				"libraryId", libraryID, "file", src.rel, "reason", outcome.message, "code", outcome.code)
		}
		// 文件收尾进度（done 计入本文件，完成数口径对齐 assets-scan:progress）
		s.emitSplitProgress(lib.ID, i+1, len(sources), src.rel, outcome.pageDone, outcome.pageTotal)
	}

	Logger().Info("assets split batch done", "libraryId", libraryID, "target", tgt, "total", batch.Total,
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
//
// target 为已归一的落点（normalizeSplitTarget 产物）：replace 走独立链路
// splitOneFileReplace（需写索引 + 删源），cache/source 保持原逻辑逐字不变。
func (s *AssetLibraryService) splitOneFileWithHash(ctx context.Context, store *assetsStore, lib model.AssetLibrary, rel, absPath, snapshotHash string, target string, fileDone, fileTotal int) (splitOutcome, error) {
	if target == model.AssetSplitTargetReplace {
		return s.splitOneFileReplace(ctx, store, lib, rel, absPath, snapshotHash, fileDone, fileTotal)
	}
	outputToSourceDir := target == model.AssetSplitTargetSource
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
				Target: target, SourceRemoved: record.sourceRemoved,
			},
			pageDone: record.pageCount, pageTotal: record.pageCount,
		}, nil
	}

	// 4. 重拆清旧页（PRD：防页数减少残留 + 旧 %02d 产物收敛）。derived 落点是
	//    应用自属目录（data/assets/<id>/derived），整目录清空安全；源目录落点与
	//    用户自建目录共享，按产物命名形态逐文件删除（其余内容一律不动）。删除
	//    失败仅告警继续拆（同名单文件覆盖写幂等，残留由下次重拆收敛）
	if outputToSourceDir {
		removeSplitArtifacts(outDir, baseName)
	} else if err := os.RemoveAll(outDir); err != nil {
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
		time.Now().Format("2006-01-02 15:04:05"), target, false); err != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("写入拆分记录失败（%s）: %w", rel, err)
	}
	return splitOutcome{
		rel: rel, status: model.AssetSplitStatusSplit,
		result: model.AssetSplitResult{
			OutputDir: result.OutputDir, Files: result.Files, PageCount: result.PageCount,
			Target: target,
		},
		pageDone: result.PageCount, pageTotal: result.PageCount,
	}, nil
}

// splitOneFileReplace 就地替换落点完整链路（严格按序，每步失败口径见 PRD D4）：
//
//  1. 源哈希、Stat 基线与落点（<源目录>/<源名主干>，与源目录落点同布局）；
//  2. 增量判定（记录落点/哈希/未删源标记/首页产物存在四验，源还原后重拆）；
//  3. 清旧产物（核心中间态 <base>-NNN.pptx + 已改名 <NNN>-stem.pptx，后者只删
//     已索引路径并删索引行，防重复素材且不误删用户文件）；
//  4. 调拆分核心（产物落 outDir，仍为核心原生命名 <base>-NNN.pptx）；
//  5. 逐页 rename 为可索引命名 <NNN>-stem.pptx + Stat 校验（存在且非空）；
//  6. 逐页建索引（复用扫描单文件重链 processScanFile，取产物 fileID）；
//  7. 标注预迁移（源文件级 → 每个产物；源第 k 页 → 第 k 个产物第 1 页）；
//  8. 取消检查点 + 源 Stat 复核（被修改则回滚）→ 源文件移入系统回收站
//     （移入失败不回滚产物：已成功拆分，仅删源失败，可重试）；
//  9. 删源收尾单事务（重迁移晚到标注 + 删源索引行；失败仅告警，下次扫描标 lost）；
//  10. 写拆分记录（target=replace, source_removed=1）；
//  11. 成功结果。
//
// 3-7 任一失败 → 回滚删除本次产物、索引行与缩略图，源文件与源行原样保留。
func (s *AssetLibraryService) splitOneFileReplace(ctx context.Context, store *assetsStore, lib model.AssetLibrary, rel, absPath, snapshotHash string, fileDone, fileTotal int) (splitOutcome, error) {
	failed := func(format string, args ...any) (splitOutcome, error) {
		return splitOutcome{rel: rel, status: model.AssetSplitStatusFailed,
			message: fmt.Sprintf(format, args...)}, nil
	}
	base := splitSourceBaseName(rel)
	// 产物落点 = 源文件所在目录下与源同名的文件夹（与 source 落点同布局）。
	// 核心原生输出恰为该目录下的 <base>-NNN.pptx，故直接传源路径拆。
	outDir := filepath.Join(filepath.Dir(absPath), base)
	// knownArtifacts 核心调用前的目录快照：回滚只删本次新增产物，目录内既有
	// 用户文件（即使名字恰好匹配产物形态）不动（PRD R9）。
	var knownArtifacts map[string]struct{}
	// rollbackFailed 回滚产物后返回失败结果（失败页/校验类，批量隔离继续）
	rollbackFailed := func(format string, args ...any) (splitOutcome, error) {
		removed := s.rollbackReplaceArtifacts(store, lib, outDir, base, knownArtifacts)
		Logger().Warn("就地替换拆分回滚产物", "libraryId", lib.ID, "file", rel, "removed", len(removed))
		out := splitOutcome{rel: rel, status: model.AssetSplitStatusFailed, rolledBack: true,
			message: fmt.Sprintf(format, args...)}
		return out, nil
	}

	// 1. 源哈希（快照为空时现算；文件不可读时哈希与拆分同样不可得，归 failed）
	hash := snapshotHash
	if hash == "" {
		h, hashErr := hashFileSHA256(absPath)
		if hashErr != nil {
			return failed("读取源文件失败: %v", hashErr)
		}
		hash = h
	}
	// cancelRollback 取消收口：回滚已产出页/索引行后返回带 context.Canceled 链的
	// 错误。核心取消与「核心已返回、取消检查点命中」两处共用——删源前必须经检查点
	// 确认未取消，保证 PRD 验收「取消 → 源保留、已产出页回滚」。
	cancelRollback := func(err error) (splitOutcome, error) {
		removed := s.rollbackReplaceArtifacts(store, lib, outDir, base, knownArtifacts)
		Logger().Warn("就地替换拆分取消回滚产物", "libraryId", lib.ID, "file", rel, "removed", len(removed))
		return splitOutcome{rel: rel, rolledBack: true}, fmt.Errorf("拆分已取消: %w", err)
	}

	// 2. 增量判定：记录落点、哈希、删源标记与首页产物存在四验。源已删时正常路径
	//    不可达（files 行已不存在，入口前置校验会拒绝），此处保留对称语义。
	record, err := store.getSplitRecord(rel)
	if err != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("读取拆分记录失败: %w", err)
	}
	if record != nil && record.target == model.AssetSplitTargetReplace &&
		record.sourceHash == hash && !record.sourceRemoved &&
		fileExists(replaceFirstPagePath(outDir, base)) {
		return splitOutcome{
			rel: rel, status: model.AssetSplitStatusSkipped, message: splitSkipReasonUpToDate,
			result: model.AssetSplitResult{
				OutputDir: outDir, PageCount: record.pageCount,
				Skipped: true, SkipReason: splitSkipReasonUpToDate,
				Target: model.AssetSplitTargetReplace, SourceRemoved: record.sourceRemoved,
			},
			pageDone: record.pageCount, pageTotal: record.pageCount,
		}, nil
	}

	// 3. 清旧产物：只删「已在 files 索引中登记」的已改名形态（成功拆分的旧页必已
	//    索引），目录内用户自建文件与未索引的崩溃残留不动——核心中间态命名本就
	//    命中派生排除，残留不参与索引（PRD R9）
	stale := s.removeReplaceArtifacts(store, lib, outDir, base)
	if len(stale) > 0 {
		s.deleteIndexRowsForReplacePaths(store, lib, stale)
	}

	// 核心调用前快照目录内既有产物名：回滚只删本次新增（PRD R9）
	knownArtifacts = snapshotReplaceArtifactNames(outDir, base)
	// 源文件拆分期基线（size + mtime）：核心读取到移入回收站之间若被修改，
	// 产物与回收站内容将不一致，删源前复核并回滚（PRD R2）
	srcBefore, statErr := os.Stat(absPath)
	if statErr != nil {
		return failed("读取源文件状态失败: %v", statErr)
	}

	// 4. 调拆分核心（页级进度透传；产物落 outDir，仍为核心原生命名）
	core := s.splitCoreFn
	if core == nil {
		core = SplitPptxPerPageWithProgress
	}
	result, err := core(ctx, absPath, func(done, total int) {
		s.emitSplitProgress(lib.ID, fileDone-1, fileTotal, rel, done, total)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// 取消：先回滚已产出页（否则会进索引与仍在的源形成重复素材），再上抛
			return cancelRollback(err)
		}
		return rollbackFailed("%s", appErrMessage(err))
	}

	// 取消检查点（核心返回后、改名/建索引前）：取消落在核心最后一页之后时核心
	// 会正常返回，若不判 ctx 将继续进入删源流程，与「取消 → 源保留」验收冲突。
	if err := ctx.Err(); err != nil {
		return cancelRollback(err)
	}

	// 5. 逐页 rename + Stat 校验（页序以核心产物文件名尾段为准，解析失败回退序号）
	renamed := make([]string, 0, len(result.Files))
	pages := make([]int, 0, len(result.Files))
	for i, src := range result.Files {
		page := pageSeqFromArtifactName(filepath.Base(src))
		if page <= 0 {
			page = i + 1
		}
		dst := filepath.Join(outDir, replaceArtifactName(page, base))
		if err := os.Rename(src, dst); err != nil {
			return rollbackFailed("第 %d 页产物重命名失败: %v", page, err)
		}
		info, statErr := os.Stat(dst)
		if statErr != nil {
			return rollbackFailed("第 %d 页产物校验失败: %v", page, statErr)
		}
		if info.Size() <= 0 {
			return rollbackFailed("第 %d 页产物校验失败: 文件为空", page)
		}
		renamed = append(renamed, dst)
		pages = append(pages, page)
	}

	// 6. 逐页建索引（复用扫描单文件重链）：源 fileID 供标注迁移，产物 fileID 由
	//    写入后回读取得（newFileID 只有索引行存在才可得，见 PRD D3）
	srcRec, srcExists, err := store.fileRecordByPath(rel)
	if err != nil {
		return rollbackFailed("读取源文件索引失败: %v", err)
	}
	if !srcExists {
		return rollbackFailed("源文件索引记录不存在（%s）", rel)
	}
	targets := make([]splitAnnotationTarget, 0, len(renamed))
	// replace 产物要进索引，须与扫描主循环用同一排除规则判定：命中时拒绝并回滚，
	// 禁先写行、下次扫描又转 excluded（PRD R6）
	matcher := newAssetExcludeMatcher(lib.ExcludeDirs, lib.ExcludeExts)
	for idx, abs := range renamed {
		page := pages[idx]
		info, statErr := os.Stat(abs)
		if statErr != nil {
			return rollbackFailed("第 %d 页产物校验失败: %v", page, statErr)
		}
		prodRel, relErr := filepath.Rel(lib.RootPath, abs)
		if relErr != nil {
			return rollbackFailed("计算产物相对路径失败（%s）: %v", abs, relErr)
		}
		prodRelSlash := filepath.ToSlash(prodRel)
		if matcher.matchPath(prodRelSlash) {
			return rollbackFailed("第 %d 页产物命中素材库排除规则（%s），请调整落点或排除规则后重试", page, prodRelSlash)
		}
		rec, exists, recErr := store.fileRecordByPath(prodRelSlash)
		if recErr != nil {
			return rollbackFailed("读取产物索引失败（%s）: %v", prodRelSlash, recErr)
		}
		outcome, scanErr := s.processScanFile(store, lib, pptxDiskEntry{
			rel: prodRelSlash, abs: abs, size: info.Size(), mtimeNs: info.ModTime().UnixNano(),
		}, rec, exists)
		if scanErr != nil {
			// 系统性错误（db 写失败）：产物与已写索引行回滚后上抛
			removed := s.rollbackReplaceArtifacts(store, lib, outDir, base, knownArtifacts)
			Logger().Warn("就地替换拆分索引失败回滚产物", "libraryId", lib.ID, "file", rel, "removed", len(removed))
			return splitOutcome{rel: rel, rolledBack: true}, fmt.Errorf("单页产物索引失败（%s）: %w", prodRelSlash, scanErr)
		}
		if outcome == scanOutcomeFailed || outcome == scanOutcomeEncrypted || outcome == scanOutcomeDenied {
			return rollbackFailed("第 %d 页产物索引失败（%s）：解析结果不可用", page, prodRelSlash)
		}
		rec, exists, recErr = store.fileRecordByPath(prodRelSlash)
		if recErr != nil || !exists {
			return rollbackFailed("第 %d 页产物索引行回读失败（%s）", page, prodRelSlash)
		}
		targets = append(targets, splitAnnotationTarget{pageNum: page, fileID: rec.id})
	}

	// 7. 标注迁移（单事务；失败则整文件不删源，回滚已产出页与索引行再上抛——
	//    迁移失败通常是事务级 db 错误，属系统性错误，批量应中止而非继续）
	if err := store.migrateSplitAnnotations(srcRec.id, targets); err != nil {
		removed := s.rollbackReplaceArtifacts(store, lib, outDir, base, knownArtifacts)
		Logger().Warn("就地替换拆分标注迁移失败回滚产物", "libraryId", lib.ID, "file", rel, "removed", len(removed))
		return splitOutcome{rel: rel, rolledBack: true},
			fmt.Errorf("标注迁移失败（%s）: %w", rel, err)
	}

	// 8. 源文件移入回收站（D4：失败不回滚产物——拆分已成功，仅删源失败；源与源行
	//    保留，用户可关闭占用程序后重新拆分覆盖产物）
	// 删源前最后取消检查点：建索引/迁移可能耗时数秒，取消信号必须在此生效，
	// 否则会把源移入回收站（源保留验收被破坏）。
	if err := ctx.Err(); err != nil {
		return cancelRollback(err)
	}
	// 源文件 TOCTOU 复核：核心读取到此处之间若被外部保存/替换，回收站内容与
	// 产物将不一致；回滚产物与索引行，源保留让用户重试（PRD R2）
	srcNow, statErr := os.Stat(absPath)
	if statErr != nil {
		return rollbackFailed("读取源文件状态失败（可能已被移动或删除）: %v", statErr)
	}
	if srcNow.Size() != srcBefore.Size() || !srcNow.ModTime().Equal(srcBefore.ModTime()) {
		return rollbackFailed("源文件在拆分期间被修改，为避免产物与源不一致已回滚，请重新拆分")
	}
	move := s.moveToRecycleBinFn
	if move == nil {
		move = util.MoveToRecycleBin
	}
	if err := move(absPath); err != nil {
		Logger().Warn("就地替换删源失败（产物保留）", "libraryId", lib.ID, "file", rel, "err", err)
		return splitOutcome{
			rel: rel, status: model.AssetSplitStatusFailed, code: model.ErrCodeAssetsSplitSourceDeleteFailed,
			message: fmt.Sprintf("源文件无法移入回收站（可能被 PowerPoint/WPS 占用或只读）：%v，产物已保留，可关闭占用程序后重新拆分", err),
			result: model.AssetSplitResult{
				OutputDir: outDir, Files: renamed, PageCount: len(renamed),
				Target: model.AssetSplitTargetReplace,
			},
		}, nil
	}

	// 9. 删源收尾（单事务）：重读源标注幂等迁移到产物 + 删源索引行，封死
	//    「迁移后删行前」的晚到标注写入丢失窗口（PRD R3）。先清缩略图记账与 PNG；
	//    收尾失败仅告警——源物理已在回收站，残留行下次扫描标 lost 可手动清理。
	s.invalidateFileThumbs(lib, store, srcRec.id)
	if err := store.finalizeReplaceSource(srcRec.id, targets); err != nil {
		Logger().Warn("就地替换删源收尾失败（下次扫描会标记丢失，可手动清理）",
			"libraryId", lib.ID, "file", rel, "fileId", srcRec.id, "err", err)
	}

	// 10. 写拆分记录（target=replace、source_removed=1；落点与删源标记回显）
	// 此步位于删源之后：源已进回收站、产物已索引，记录仅剩留痕职责，写失败降级为
	// 告警而非整体失败（否则批量会因「已完成的工作」报错，且重试入口因源行缺失
	// 不可用）。
	if err := store.upsertSplitRecord(rel, hash, outDir, len(renamed),
		time.Now().Format("2006-01-02 15:04:05"), model.AssetSplitTargetReplace, true); err != nil {
		Logger().Warn("就地替换写拆分记录失败（不影响已完成结果）",
			"libraryId", lib.ID, "file", rel, "err", err)
	}

	// 11. 成功结果（产物绝对路径按页序）
	return splitOutcome{
		rel: rel, status: model.AssetSplitStatusSplit,
		result: model.AssetSplitResult{
			OutputDir: outDir, Files: renamed, PageCount: len(renamed),
			Target: model.AssetSplitTargetReplace, SourceRemoved: true,
		},
		pageDone: len(renamed), pageTotal: len(renamed),
	}, nil
}

// deleteIndexRowsForReplacePaths 删除就地替换旧产物在索引中的行（含缩略图记账与
// PNG 文件）：页数减少/重拆时越界旧页必须从索引消失，否则成为指向已删文件的
// 僵尸素材。路径按库根换算为相对路径查询；不存在行（未索引/已被删）静默跳过。
func (s *AssetLibraryService) deleteIndexRowsForReplacePaths(store *assetsStore, lib model.AssetLibrary, paths []string) {
	for _, p := range paths {
		rel, err := filepath.Rel(lib.RootPath, p)
		if err != nil {
			Logger().Warn("计算旧产物相对路径失败（跳过索引行清理）", "libraryId", lib.ID, "path", p, "err", err)
			continue
		}
		rec, exists, err := store.fileRecordByPath(filepath.ToSlash(rel))
		if err != nil {
			Logger().Warn("读取旧产物索引失败（跳过索引行清理）", "libraryId", lib.ID, "path", p, "err", err)
			continue
		}
		if !exists {
			continue
		}
		s.invalidateFileThumbs(lib, store, rec.id)
		if err := store.deleteFileRecord(rec.id); err != nil {
			Logger().Warn("删除旧产物索引行失败", "libraryId", lib.ID, "path", p, "fileId", rec.id, "err", err)
		}
	}
}

// rollbackReplaceArtifacts 回滚就地替换本次产出的产物：删除 outDir 下两种命名
// 形态（核心中间态 <base>-NNN.pptx 与已改名 <NNN>-stem.pptx）中**不在 keep 快照
// 内**的文件、清理其索引行与缩略图，目录因此变空时尝试删除目录（非空/失败忽略
// ——用户自建内容不动）。keep 为拆分核心调用前的目录快照，防用户自建的同形文件
// 被误删（PRD R9）。返回删除的文件清单供日志与批量明细的「已回滚」计数。
func (s *AssetLibraryService) rollbackReplaceArtifacts(store *assetsStore, lib model.AssetLibrary, outDir, base string, keep map[string]struct{}) []string {
	removed := make([]string, 0, 4)
	entries, err := os.ReadDir(outDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !isReplaceArtifactName(e.Name(), base) {
				continue
			}
			if _, existed := keep[e.Name()]; existed {
				continue // 拆分前已存在：用户文件或历史残留，回滚不动
			}
			p := filepath.Join(outDir, e.Name())
			if rmErr := os.Remove(p); rmErr != nil && !os.IsNotExist(rmErr) {
				Logger().Warn("回滚删除拆分产物失败", "path", p, "err", rmErr)
				continue
			}
			removed = append(removed, p)
		}
	}
	s.deleteIndexRowsForReplacePaths(store, lib, removed)
	// 仅当目录已空才删除：先 ReadDir 判空（非空 = 用户自建内容，保留且不算异常，
	// 不产生告警噪音）；判空与 Remove 之间的并发写入由 Remove 自身失败兜底。
	if entries, readErr := os.ReadDir(outDir); readErr == nil && len(entries) == 0 {
		if rmErr := os.Remove(outDir); rmErr != nil && !os.IsNotExist(rmErr) {
			Logger().Warn("回滚后空产物目录删除失败（保留）", "dir", outDir, "err", rmErr)
		}
	}
	return removed
}

// snapshotReplaceArtifactNames 记录目录内当前匹配产物命名形态的文件名集合。
// 目录不存在或不可读返回空集合（此时本次产物全为新文件，回滚可全删）。
func snapshotReplaceArtifactNames(dir, base string) map[string]struct{} {
	set := make(map[string]struct{})
	entries, err := os.ReadDir(dir)
	if err != nil {
		return set
	}
	for _, e := range entries {
		if e.IsDir() || !isReplaceArtifactName(e.Name(), base) {
			continue
		}
		set[e.Name()] = struct{}{}
	}
	return set
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
			Exists:        true,
			OutputDir:     record.outputDir,
			PageCount:     record.pageCount,
			SplitAt:       record.splitAt,
			Target:        record.target,
			SourceRemoved: record.sourceRemoved,
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
		// 产物命名随落点不同：replace 为 <页序>-<源名主干>.pptx（可索引命名），
		// 其余落点为 <源名主干>-<页序>.pptx（核心原生命名）
		if record.target == model.AssetSplitTargetReplace {
			pagePath = filepath.Join(record.outputDir, replaceArtifactName(slideNum, splitSourceBaseName(rel)))
		} else {
			pagePath = filepath.Join(record.outputDir,
				fmt.Sprintf("%s-%03d.pptx", splitSourceBaseName(rel), slideNum))
		}
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
func singleOutcomeBatch(o splitOutcome, target string) model.AssetSplitBatchResult {
	batch := model.AssetSplitBatchResult{Total: 1, Items: []model.AssetSplitFileOutcome{{
		RelPath: o.rel, Status: o.status, Message: o.message,
		Target: target, SourceRemoved: o.result.SourceRemoved,
		RolledBack: o.rolledBack, Code: o.code,
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

// removeSplitArtifacts 源目录落点的重拆清旧页：仅删除匹配产物命名形态
// `<源名主干>-<纯数字>.pptx` 的文件（页序段纯数字、不限位数——旧 %02d 产物与
// 越界页一并收敛，口径与 isPptxSplitDerivative 的数字判定一致），目录内其余
// 内容（用户自建文件、`<源名>-草稿.pptx` 等同名异构文件、子目录）一律不动。
// 目录不存在（首次拆分）或不可读时静默返回——无旧页可清，覆盖写继续。
func removeSplitArtifacts(dir, baseName string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := baseName + "-"
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".pptx") {
			continue
		}
		seq := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".pptx")
		if seq == "" || !isAllDigits(seq) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			Logger().Warn("删除旧拆分产物失败（覆盖写继续）", "dir", dir, "file", name, "err", err)
		}
	}
}

// isAllDigits 非空纯数字串判定（重拆清旧页的页序段口径）。
func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// replaceArtifactBase 就地替换产物的名字主干：源名主干为「≥3 位纯数字」时追加
// 「-p」后缀。
//
// 为什么需要：isPptxSplitDerivative 判定「父目录名 + '-' + 纯数字」。产物命名
// 为 `%03d-<base>` 且父目录名恰为 <base>，当 <base> 与页序段逐字符相同即命中
// 排除规则（product 被扫描静默跳过、不进索引）。页序段在 <1000 页时恰 3 位、
// ≥1000 页时不补零（如第 1234 页 → "1234"），故碰撞面为「len(base) ≥ 3 的纯
// 数字主干」：base=123 第 123 页 → "123-123.pptx"；base=1234 第 1234 页 →
// "1234-1234.pptx"。1~2 位数字不可能与 ≥3 位页序段等长，无需后缀。追加 -p 后
// 页序段含字母不命中判定；同文件全部页统一使用该主干，命名保持一致。
func replaceArtifactBase(base string) string {
	if len(base) >= 3 && isAllDigits(base) {
		return base + "-p"
	}
	return base
}

// replaceArtifactName 就地替换产物的单页文件名：<3 位页序>-<名字主干>.pptx
// （页序前缀命名，扫描后作为独立素材进索引，见 PRD R2）。
func replaceArtifactName(page int, base string) string {
	return fmt.Sprintf("%03d-%s.pptx", page, replaceArtifactBase(base))
}

// replaceFirstPagePath 就地替换产物的首页路径（增量判定与回滚锚点）。
func replaceFirstPagePath(outDir, base string) string {
	return filepath.Join(outDir, replaceArtifactName(1, base))
}

// pageSeqFromArtifactName 从核心产物文件名尾段解析页序（`<base>-NNN.pptx` →
// NNN）。解析失败返回 0，调用方回退「结果切片序号 + 1」。
func pageSeqFromArtifactName(name string) int {
	stem := strings.TrimSuffix(name, ".pptx")
	idx := strings.LastIndexByte(stem, '-')
	if idx < 0 || idx == len(stem)-1 {
		return 0
	}
	seq := stem[idx+1:]
	if !isAllDigits(seq) {
		return 0
	}
	n, err := strconv.Atoi(seq)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// isReplaceArtifactName 就地替换链路在 outDir 下可能存在的两种产物形态判定：
//   - 核心中间态：<源名主干>-<纯数字>.pptx（核心原生命名，rename 前）；
//   - 已改名：<纯数字>-<名字主干>.pptx（replaceArtifactName 产物）。
//
// 回滚与重拆清旧页共用本判定；目录内其余内容（用户自建文件、草稿等）不动。
func isReplaceArtifactName(name, base string) bool {
	if strings.HasPrefix(name, base+"-") && strings.HasSuffix(name, ".pptx") {
		// 核心中间态：中间段为纯数字
		seq := strings.TrimSuffix(strings.TrimPrefix(name, base+"-"), ".pptx")
		if isAllDigits(seq) {
			return true
		}
	}
	// 已改名形态：<纯数字>-<名字主干>.pptx
	suffix := "-" + replaceArtifactBase(base) + ".pptx"
	if strings.HasSuffix(name, suffix) {
		seq := strings.TrimSuffix(name, suffix)
		return isAllDigits(seq)
	}
	return false
}

// removeReplaceArtifacts 就地替换落点重拆前的旧产物清理：仅删除已改名形态
// `<纯数字>-<名字主干>.pptx` 且**已存在于 files 索引**的路径（核心中间态命中
// 派生排除，残留不参与索引，无需清理）。目录内用户自建文件与未索引残留一律
// 不动（PRD R9）。目录不存在或不可读时静默返回；返回被删除的文件绝对路径，
// 供调用方联动清理索引行。
func (s *AssetLibraryService) removeReplaceArtifacts(store *assetsStore, lib model.AssetLibrary, dir, base string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	suffix := "-" + replaceArtifactBase(base) + ".pptx"
	removed := make([]string, 0, 4)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		seq := strings.TrimSuffix(name, suffix)
		if !isAllDigits(seq) {
			continue
		}
		p := filepath.Join(dir, name)
		rel, relErr := filepath.Rel(lib.RootPath, p)
		if relErr != nil {
			Logger().Warn("计算旧就地替换产物相对路径失败（保留）", "path", p, "err", relErr)
			continue
		}
		_, indexed, recErr := store.fileRecordByPath(filepath.ToSlash(rel))
		if recErr != nil {
			Logger().Warn("读取旧就地替换产物索引失败（保留）", "path", p, "err", recErr)
			continue
		}
		if !indexed {
			continue // 未索引：用户自建文件或崩溃残留，禁删
		}
		if err := os.Remove(p); err != nil {
			Logger().Warn("删除旧就地替换产物失败（重拆覆盖继续）", "dir", dir, "file", name, "err", err)
			continue
		}
		removed = append(removed, p)
	}
	return removed
}

// splitOutputDir cache/source 两态落点解析（PRD Q4 落点契约，见文件头注释布局
// 说明）；replace 落点与 source 同布局但命名不同，在 splitOneFileReplace 内自行
// 计算（不走本函数）：
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
