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
	"runtime"
	"strconv"
	"strings"
	"syscall"
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
//   - 缓存目录（后端空串归一 cache；素材面板前端弹窗默认 replace）：
//     data/assets/<library-id>/derived/<源相对目录>/<源名主干>/
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

// outputDirConflictOutcome 落点目录被另一源文件的拆分记录占用时的统一失败结果
// （source/cache 与 replace 两条链路共用单点，防错误码与文案漂移）。返回 failed
// 状态而非 error：批量入口只填 Item.Code 并继续后续文件，单文件入口升格为
// AppError 供前端按 warning 分流。调用方须保证在任何删除/写入动作之前命中。
func outputDirConflictOutcome(rel string, occupied *assetSplitRecord) splitOutcome {
	return splitOutcome{
		rel: rel, status: model.AssetSplitStatusFailed, code: model.ErrCodeAssetsSplitOutputConflict,
		message: fmt.Sprintf("该源文件的拆分产物目录已被另一源文件占用（%s），请先改名或移动后再拆分", occupied.sourceRelPath),
	}
}

// normalizeSplitTarget 落点参数归一与校验单点（App 层传入的原样字符串）：
// 空串兼容旧调用（后端归一 cache；素材面板前端弹窗默认传 replace）；非法值明确
// 拒绝，避免静默落到意外的落点。
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

	outcome, err := s.splitOneFileWithHash(splitCtx, store, lib, rel, absPath, hash, tgt, 1, 1, entry)
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
// 并以 canceled=true 收尾（注册表 defer 注销无残留）。落点三态同
// SplitAssetFile；replace 需与扫描互斥（锁序 scan → split）。
//
// 取消语义：返回已完成部分（Items 顺序同处理序）+ Canceled=true + nil error，
// 终态发 assets-split:done（不发 error）——取消属「带标记的正常收尾」，用户可
// 核对已完成成果（对齐缩略图批次 Canceled 先例）。取消发生时正在处理的那份
// 不追加 Item：cache/source 保留其已产出、replace 已回滚本文件产物与索引行。
// 真系统性错误（db 读写失败等）仍走 abortAssetSplit（error 事件 + error 返回）。
//
// replace 批量注意：前序文件成功拆分后源行已删除，后续文件的产物索引不会与
// 已删源冲突；快照圈定发生在任何删除之前，故 Total 仍覆盖全部源文件。
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
	// 剔除上次就地替换生成的单页产物：它们是同一源的页级物料，再拆只会得到
	// 「页的页」嵌套派生，并使源与产物失去一一对应（审核修复）。取记录与过滤
	// 分开做：output_dir 存绝对路径，SQL 内拼路径前缀在跨平台分隔符下不可靠。
	replaceOutputs, err := store.listReplaceSplitOutputs()
	if err != nil {
		// 系统性错误（读记录失败）：走统一收口（日志 + error 事件 + 错误返回），
		// 与逐文件链路读记录失败的处置口径一致
		return s.abortAssetSplit(libraryID, "", fmt.Errorf("读取拆分记录失败: %w", err))
	}
	sources = filterReplaceArtifactSources(lib, sources, replaceOutputs)

	batch := model.AssetSplitBatchResult{Total: len(sources),
		Items: make([]model.AssetSplitFileOutcome, 0, len(sources))}
	for i, src := range sources {
		// 文件间取消检查点（对齐扫描链路：单文件拆分内不中断）
		if splitCtx.Err() != nil {
			batch.Canceled = true
			break
		}
		absPath := filepath.Join(lib.RootPath, filepath.FromSlash(src.rel))
		outcome, err := s.splitOneFileWithHash(splitCtx, store, lib, src.rel, absPath, src.hash, tgt, i+1, len(sources), entry)
		if err != nil {
			// 取消：中断剩余（当前文件已由单文件链路自行收口，不追加 Item）
			if errors.Is(err, context.Canceled) {
				Logger().Info("assets split batch canceled mid-file", "libraryId", libraryID, "file", src.rel)
				batch.Canceled = true
				break
			}
			// 系统性错误（db 读写失败等）：整体失败，批量中止
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

	// 竞态补判：取消信号在最后一个文件的处理期间（检查点之后）到达——循环已
	// 自然结束，但用户确实点了取消，仍按取消收尾（否则前端显示「拆分完成」）
	if !batch.Canceled && splitCtx.Err() != nil {
		batch.Canceled = true
	}
	// 收尾定态后立即注销可取消入口（注销按条目指针比对，defer 重复调用无害）：
	// 此刻至返回之间到达的 CancelAssetSplit 必须返回 false——否则前端提示
	// 「已请求取消拆分」而批次随后报完成，正是本次要消除的错位
	s.unregisterSplitCancel(libraryID, entry)

	Logger().Info("assets split batch done", "libraryId", libraryID, "target", tgt, "total", batch.Total,
		"split", batch.Split, "skipped", batch.Skipped, "failed", batch.Failed, "canceled", batch.Canceled)
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
//     Failed++ 继续后续文件；单文件入口由调用方升格为错误）；
//   - 落点目录归属冲突 → status=failed + code=E_ASSETS_SPLIT_OUTPUT_CONFLICT
//     结果返回（同上隔离口径，不删除任何文件）。
//
// snapshotHash 为调用方提供的源内容哈希（单文件入口查 files.hash 列，批量入口
// 快照带出）；为空（权限跳过/旧版本扫描）时现算——无哈希无法做增量判定。
//
// target 为已归一的落点（normalizeSplitTarget 产物）：replace 走独立链路
// splitOneFileReplace（需写索引 + 删源），cache/source 保持原逻辑逐字不变。
//
// entry 为本操作的取消注册条目（删源前复查用，见 splitCancelledAfterCheckpoint）：
// 直接调用（非入口链路）的测试可传 nil，此时只按 ctx 判定取消。
func (s *AssetLibraryService) splitOneFileWithHash(ctx context.Context, store *assetsStore, lib model.AssetLibrary, rel, absPath, snapshotHash string, target string, fileDone, fileTotal int, entry *cancelEntry) (splitOutcome, error) {
	if target == model.AssetSplitTargetReplace {
		return s.splitOneFileReplace(ctx, store, lib, rel, absPath, snapshotHash, fileDone, fileTotal, entry)
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

	// 2.1 落点目录归属校验（P0 数据正确性）：source/cache 落点由当前文件名直接
	//     推导，可能已被另一源文件的拆分记录占用——典型：A 拆分后改名重链为 B
	//     （记录键变 B、output_dir 冻结在 A 目录），用户再放入新的 A.pptx 拆分，
	//     第 4 步的清除会直接删掉并覆盖 B 的产物。此处必须早于任何删除动作拒绝；
	//     同源记录由查询排除，正常重拆（含改名重链后重拆）不受影响。
	if occupied, ownErr := store.splitRecordByOutputDir(outDir, rel); ownErr != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("读取拆分记录失败: %w", ownErr)
	} else if occupied != nil {
		return outputDirConflictOutcome(rel, occupied), nil
	}

	// 3. 增量判定：记录命中 + 哈希一致 + 落点 target 一致 + 产物首页存在（四验）。
	//    落点切换（source↔cache 等）必须真正换落点重拆——仅比哈希会把「已拆分」
	//    误判为最新而跳过，旧落点产物与索引行永不收敛，且跳过结果会回显与实际落点
	//    不符的 Target（2026-11-02 审核修复）。**只比 target 不比 output_dir**：
	//    改名重链后记录落点仍冻结旧目录、本次 outDir 为源新目录，但产物经
	//    splitArtifactPath（直拼 + 页序反查）仍可定位——target 未变即判「已拆未变」
	//    跳过（11-01 契约：改名重链不触发全量重拆，避免批量重命名放大磁盘写入）；
	//    仅用户显式切换落点才重拆。
	record, err := store.getSplitRecord(rel)
	if err != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("读取拆分记录失败: %w", err)
	}
	if record != nil && record.sourceHash == hash && record.target == target {
		if _, artifactOK := splitArtifactPath(record, rel, 1); artifactOK {
			return splitOutcome{
				rel: rel, status: model.AssetSplitStatusSkipped, message: splitSkipReasonUpToDate,
				result: model.AssetSplitResult{
					OutputDir: record.outputDir, PageCount: record.pageCount,
					Skipped: true, SkipReason: splitSkipReasonUpToDate,
					Target: record.target, SourceRemoved: record.sourceRemoved,
				},
				pageDone: record.pageCount, pageTotal: record.pageCount,
			}, nil
		}
	}

	// 4. 重拆清旧改为延迟清理（2026-11-02）：核心调用前不再删除任何旧页，也不再
	//    对 derived 落点整目录 RemoveAll。核心失败/取消时上一次拆分成果不再被
	//    前置删除（「先删后拆」在失败时会连带丢掉上一次成果，与 replace 落点的
	//    延迟清理契约一致）。**已知限制**：核心直接写落点目录，失败前已同名覆盖
	//    写的旧页内容无法回滚（彻底解决需核心写临时目录再原子移动，见任务
	//    Out of Scope）。核心成功后按本次 result.PageCount 删除超界旧页（见
	//    5.1），页序 ≤ PageCount 的旧页由核心同名覆盖写更新。

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

	// 5.1 超界旧页收敛（延迟清理）：source/cache 落点的产物命名恒为
	//     `<源名主干>-NNN.pptx`，页数减少时删除页序 > 本次 PageCount 的旧产物；
	//     页序 ≤ PageCount 但文件名不是当前 `%03d` 形态的（历史 `%02d` 产物）同样
	//     收敛。只删产物命名形态，同目录用户自建内容一律不动；成功删除的路径联动
	//     清理索引行（source 落点在库内，否则留指向已删文件的僵尸素材）；删除失败
	//     仅告警继续，不阻断收尾。mtime 守卫以**上一次**记录的分拆时间为基准（用户
	//     在两次拆分之间放入的同名文件保留）；record 为 nil（首次拆分）时传空串，
	//     守卫不生效，保持旧行为。
	splitAt := ""
	if record != nil {
		splitAt = record.splitAt
	}
	s.removeSplitArtifactsBeyond(store, lib, record, outDir, baseName, result.PageCount, splitAt)

	// 5.2 旧产物目录收尾清理（改名重链 / 跨落点切换 / 同目录改主干等形态）：
	//     决策统一走 splitArtifactCleanupPlan 单点（与 replace 链路同源，防口径
	//     分叉）。刻意放在核心成功之后——核心失败/取消时上一次拆分成果原样保留，
	//     对齐 replace 落点的延迟清理契约（「先删后拆」在失败时会连带丢掉上一次
	//     成果）。
	curStem := baseName
	if dir, excludeStem, ok := splitArtifactCleanupPlan(record, target, curStem, outDir); ok {
		s.cleanupSplitArtifactsInDirByPage(store, lib, record, dir, excludeStem)
	}

	// 5.3 retained 归属凭证消费（v11 跨 target / 同落点收敛）：旧页清理之后再把
	//     「上次清理失败未能删除」的残留页尝试删一次。返回值（仍删不掉的条目）写入
	//     本次记录继续携带，不再静默清空（R1）。dir 取记录的 output_dir——凭证名
	//     相对该目录，跨目录不消费（函数内 samePathFold 守卫）。先按本次新产物名
	//     过滤同名条目：旧凭证若与本次新页同名且内容全等（拆分字节确定性），直接
	//     消费会误删刚生成的产物（与 replace 链路 recordWithoutRetainedNames 同口径）。
	retainedDir := ""
	if record != nil {
		retainedDir = record.outputDir
	}
	retained := s.cleanupRetainedSplitArtifacts(store, lib, recordWithoutRetainedNames(record, result.Files), retainedDir, "")

	// 6. 落拆分记录（成功才记；失败/取消不记，下次触发重走完整判定）。产物主干
	//    持久化（v9）——source/cache 落点核心原生命名恒为 <baseName>-NNN.pptx。
	//    页级指纹（v10）在旧产物清理完成后按本次落盘字节计算：任一页哈希失败降级为
	//    空指纹（回落 mtime 守卫）并 Warn，不回滚已成功的拆分与产物。
	fingerprints := s.splitArtifactFingerprints(lib, rel, result.Files, nil)
	if err := store.upsertSplitRecord(rel, hash, result.OutputDir, result.PageCount,
		time.Now().Format("2006-01-02 15:04:05"), target, baseName,
		false, fingerprints, retained); err != nil {
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
//  1. 主干/落点校验（空主干拒绝、上次 replace 产物拒绝二次拆分）→ 重算源哈希；
//  2. 读取既有记录（replace 不做增量跳过：源行存在即源还原或上次删源失败，
//     两者都须完整重拆）；
//  3. 旧产物登记（延迟清理）：目录确属本文件 target=replace 记录时收集旧产物名
//     集合 oldArtifacts，不删除；记录缺失/落点不符而目录已有同形文件则直接失败；
//  4. 调拆分核心（产物落 outDir，仍为核心原生命名 <base>-NNN.pptx）；
//  5. 逐页 rename 为可索引命名 <NNN>-stem.pptx + Stat 校验（存在且非空）；目标
//     命中 oldArtifacts 时先备份旧文件（回滚可还原），其余既有文件拒绝覆盖；
//  6. 逐页建索引（复用扫描单文件重链 processScanFile，取产物 fileID）；
//  7. 标注预迁移（源文件级 → 每个产物；源第 k 页 → 第 k 个产物第 1 页）；
//  8. 取消检查点 + 拆分取消标志复查 + 源 Stat 复核（被修改则回滚）→ 删除
//     oldArtifacts 中不属于本次新页的旧页（含索引行）→ 源文件移入系统回收站
//     （移入失败不回滚产物：已成功拆分，仅删源失败，可重试）；
//  9. 删源收尾单事务（重迁移晚到标注 + 删源索引行；失败仅告警，下次扫描标 lost）；
//  10. 写拆分记录（target=replace, source_removed=1）；
//  11. 成功结果。
//
// 3-7 任一失败 → 回滚删除本次产物、索引行与缩略图，并把被覆盖的历史产物从备份
// 还原；源文件与源行原样保留。旧产物延迟删除是关键：此前「先删旧产物再拆分」
// 在拆分失败时会连带丢掉上一次的成功成果（审核修复）。
//
// entry 为本操作的拆分取消注册条目（删源前复查，见 splitCancelledAfterCheckpoint）。
func (s *AssetLibraryService) splitOneFileReplace(ctx context.Context, store *assetsStore, lib model.AssetLibrary, rel, absPath, snapshotHash string, fileDone, fileTotal int, entry *cancelEntry) (splitOutcome, error) {
	failed := func(format string, args ...any) (splitOutcome, error) {
		return splitOutcome{rel: rel, status: model.AssetSplitStatusFailed,
			message: fmt.Sprintf(format, args...)}, nil
	}
	base := splitSourceBaseName(rel)
	// 空主干（文件名恰为 .pptx / ..pptx）会让落点退化为源目录本身，产物将直接
	// 写进源目录并可能覆盖同名文件；明确拒绝而不是落到意外落点（审核修复）。
	if !splitStemUsable(base) {
		return failed("源文件名主干为空，无法确定拆分产物目录，请先重命名文件后重试")
	}
	// 产物落点 = 源文件所在目录下与源同名的文件夹（与 source 落点同布局）。
	// 核心原生输出恰为该目录下的 <base>-NNN.pptx，故直接传源路径拆。
	outDir := splitSourceOutputDir(absPath, rel)

	// 上次就地替换生成的单页产物禁止再次 replace 拆分（批量入口已剔除，此处是
	// 单文件入口的兜底）：嵌套拆分把「页」再拆成「页的页」，并使源与产物失去
	// 一一对应。cache/source 落点不受限制（产物不进索引，无嵌套可见面）。
	replaceOutputs, outErr := store.listReplaceSplitOutputs()
	if outErr != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("读取拆分记录失败: %w", outErr)
	}
	if isReplaceArtifactOutput(absPath, replaceOutputs) {
		return failed("该文件是上一次就地替换生成的单页拆分产物，不能再次拆分")
	}

	// 落点目录归属复核（P2 数据正确性，与 source/cache 链路共用 splitRecordByOutputDir
	// 单点）：ownsOutDir 只证明「本记录 target=replace 且落点即为该目录」，无法发现
	// 「旧记录指向该目录但产物已缺失」的形态——此时目录内是另一条记录（同目录新同名
	// 源或改名重链后的记录）的产物，旧记录再次就地替换会按旧 pageCount 删掉它们
	// （「无文件可覆盖、无数据丢失」的结论不完整）。文件名启发式
	// firstReplaceArtifactName 同样只能看出「有同形文件」，看不出归属。故在任何
	// 写入/覆盖动作之前以记录集为权威判定：命中其他源记录即拒绝，目录零改动；
	// 同源记录由查询排除，ownsOutDir 分支语义不变（本记录拥有目录且无其他记录
	// 同时占用时正常重拆放行；两条记录同指一目录属历史冲突数据，按拒绝处理）。
	if occupied, ownErr := store.splitRecordByOutputDir(outDir, rel); ownErr != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("读取拆分记录失败: %w", ownErr)
	} else if occupied != nil {
		return outputDirConflictOutcome(rel, occupied), nil
	}

	// knownArtifacts 核心调用前的目录快照：回滚只删本次新增产物，目录内既有
	// 文件（即使名字恰好匹配产物形态）不动（PRD R9）。backups 记录本次为让位
	// 而挪走的历史产物（最终路径 → 备份路径），回滚时换回原位。
	var knownArtifacts map[string]struct{}
	backups := make(map[string]string)
	rollbackArtifacts := func(reason string) {
		removed := s.rollbackReplaceArtifacts(store, lib, outDir, base, knownArtifacts)
		Logger().Warn("就地替换拆分回滚产物", "libraryId", lib.ID, "file", rel, "removed", len(removed), "reason", reason)
		if len(backups) > 0 {
			s.restoreReplaceArtifactBackups(store, lib, backups)
			Logger().Warn("就地替换回滚还原历史产物", "libraryId", lib.ID, "file", rel, "restored", len(backups))
		}
	}
	// rollbackFailed 回滚产物后返回失败结果（失败页/校验类，批量隔离继续）
	rollbackFailed := func(format string, args ...any) (splitOutcome, error) {
		msg := fmt.Sprintf(format, args...)
		rollbackArtifacts(msg)
		return splitOutcome{rel: rel, status: model.AssetSplitStatusFailed, rolledBack: true,
			message: msg}, nil
	}

	// 1. 源哈希必须重算：快照哈希来自扫描时点，与当前磁盘内容可能已不一致。
	//    用陈旧哈希会误判「已最新」跳过，或把失真的 source_hash 写进拆分记录；
	//    不一致说明用户改了文件但没重扫，让用户先重扫（同时刷新 files.hash）。
	hash, hashErr := hashFileSHA256(absPath)
	if hashErr != nil {
		return failed("读取源文件失败: %v", hashErr)
	}
	if snapshotHash != "" && snapshotHash != hash {
		return failed("源文件内容已变化，请先重新扫描素材库后再拆分")
	}
	// cancelRollback 取消收口：回滚已产出页/索引行后返回带 context.Canceled 链的
	// 错误。核心取消与「核心已返回、取消检查点命中」两处共用——删源前必须经检查点
	// 确认未取消，保证 PRD 验收「取消 → 源保留、已产出页回滚」。
	cancelRollback := func(err error) (splitOutcome, error) {
		rollbackArtifacts("cancel")
		return splitOutcome{rel: rel, rolledBack: true}, fmt.Errorf("拆分已取消: %w", err)
	}

	// 2. 读取既有拆分记录（落点归属判定用）。replace 不做增量跳过：能走到这里的
	//    前提是源 files 行仍存在（入口前置校验 status=ok），而成功替换后源行已被
	//    finalizeReplaceSource 删除，故源行存在只对应两种需重拆的状态——源从回收站
	//    还原（source_removed=1），或上次删源失败但产物完整（source_removed=0，
	//    见步骤 8b 补写记录）。二者都必须完整重拆，跳过会留下源与产物并存。
	record, err := store.getSplitRecord(rel)
	if err != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("读取拆分记录失败: %w", err)
	}

	// 3. 旧产物登记（延迟清理）：只有目录确属本文件的 replace 记录输出目录时，
	//    目录内同形文件才视为历史产物；否则（无记录 / 记录是其他落点 / 记录指向
	//    别处）它们是用户自建内容——出现同形文件即拒绝，禁覆盖或删除用户数据。
	ownsOutDir := record != nil && record.target == model.AssetSplitTargetReplace &&
		samePathFold(record.outputDir, outDir)
	var oldArtifacts map[string]struct{}
	if ownsOutDir {
		oldArtifacts = snapshotReplaceArtifactNames(outDir, base)
		// 核心中间态同名文件保护（R5）：核心按 `<base>-NNN.pptx` 原生命名落盘，目录内
		// 若已存在同形文件（上次核心中断残留、用户自建同名文件等），核心会直接同名
		// 覆盖写——用户内容不可恢复。守卫放在核心调用之前（早于任何写入），命中即
		// 拒绝，核心不调用、目录零改动。仅 ownsOutDir 分支需要：非 owns 分支的 stray
		// 由下方 replaceOutDirOwnedByRecord 放宽判定（凭证内容全等才继续），其余用户
		// 同名文件仍由步骤 5 逐页拒绝。
		if coreName := firstReplaceCoreArtifactName(outDir, base); coreName != "" {
			return failed("目录中已存在与拆分核心产物同名的文件（%s），请先移走或重命名后重试", coreName)
		}
	} else if stray := firstReplaceArtifactName(outDir, base); stray != "" {
		// 落点归属放宽（R1）：记录非空且目录内 stray 文件能经归属凭证（本次指纹或
		// v11 retained）按名命中、且内容（size + SHA-256）全等时，视为本记录拥有该
		// 落点——切 target 后残留页/旧形态产物仍在目录内时，不应按名永久拒绝重拆。
		// oldArtifacts 照常快照并继续流程，其余用户同名文件仍会在步骤 5 逐页拒绝。
		if !s.replaceOutDirOwnedByRecord(record, outDir, base) {
			return failed("目录中已存在与拆分产物同名的文件（%s），请先移走或重命名后重试", stray)
		}
		// 归属放宽只放行「凭证内容全等」的历史产物；核心紧接着仍会写
		// `<base>-NNN.pptx`，目录内无凭证（或内容不符）的核心同名文件是用户内容，
		// 核心同名覆盖写不可恢复——仍须早于核心拒绝（R5 守卫在放宽分支同样生效）。
		if coreName := s.firstUnownedReplaceCoreArtifactName(record, outDir, base); coreName != "" {
			return failed("目录中已存在与拆分核心产物同名的文件（%s），请先移走或重命名后重试", coreName)
		}
		oldArtifacts = snapshotReplaceArtifactNames(outDir, base)
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

	// 落点目录大小写归一（2026-11-02）：Windows NTFS 大小写不敏感但保留原始大小写，
	// 源仅改大小写（abc.pptx → Abc.pptx）后重拆时 outDir 由当前文件名推导（…/Abc），
	// 磁盘上既有产物目录仍是旧形态（…/abc）。索引相对路径若按新形态写入，下次扫描
	// 按磁盘实际名（WalkDir 返回 abc）匹配不上，会把刚建的行标 lost 并另插新行
	// （幽灵行 + 标注挂在不可见行）。按 parent 目录实际条目名归一，索引、记录与磁盘
	// 一致；读取失败或多命中（大小写敏感目录开启）按原值继续。
	outDir = resolveExistingDirCase(outDir)

	// 5. 逐页 rename + Stat 校验（页序以核心产物文件名尾段为准，解析失败回退序号）
	renamed := make([]string, 0, len(result.Files))
	pages := make([]int, 0, len(result.Files))
	for i, src := range result.Files {
		page := pageSeqFromArtifactName(filepath.Base(src))
		if page <= 0 {
			page = i + 1
		}
		dst := filepath.Join(outDir, replaceArtifactName(page, base))
		dstName := filepath.Base(dst)
		if oldName, isOld := foldNameInSet(oldArtifacts, dstName); isOld {
			// 历史产物：先挪到备份名再让新页占位。若直接覆盖，随后的失败回滚
			// 只能保住文件名却保不住旧内容（rename 不可逆）；备份使「失败/取消
			// 时旧产物原样保留」真正成立。
			// 大小写折叠命中时 oldName 为磁盘实际名（如 001-abc.pptx vs 本次
			// 001-Abc.pptx）：按实际名搬迁备份，回滚还原也回到原大小写，索引行
			// 路径不漂移（2026-11-02）。
			oldPath := filepath.Join(outDir, oldName)
			// 归属校验（v10/v11）：oldArtifacts 只按命名形态收集，命中不代表该文件
			// 仍是历史产物——用户可能把某页产物替换成自有内容（同名）。归属凭证经
			// ownershipFingerprintForName 单点查询（先本次指纹、再清理失败保留凭证），
			// 内容（size + SHA-256）全等才允许备份让位。
			//   - 有归属条目但内容不符：用户已修改该历史产物 → R4 内容不一致文案；
			//   - 无任何归属条目且指纹可用（allow-list 语义）：不是本记录产物 → 用户
			//     同名文件文案；
			//   - 指纹不可用且无 retained（v10 前旧记录 / 指纹损坏）：保持按名认定
			//     行为兼容。
			fp, owns := ownershipFingerprintForName(record, oldName)
			switch {
			case owns && !fingerprintMatch(oldPath, fp):
				return rollbackFailed("%s", replaceArtifactMismatchMessage(page, dstName, ""))
			case !owns && fingerprintsUsable(record):
				return rollbackFailed("第 %d 页目标文件「%s」已存在且不属于历史拆分产物，请先移走或重命名后重试", page, dstName)
			case owns:
				// 备份让位 + 复验（R3 TOCTOU 收口：校验与 rename 之间文件可能被改）。
				bak, keptBackup, bakErr := s.backupReplaceArtifactForSurrender(oldPath, outDir, oldName, fp)
				if bakErr != nil {
					if errors.Is(bakErr, errSplitArtifactContentMismatch) {
						// 换回失败时 keptBackup 为仍留在磁盘上的备份路径（不登记进
						// backups，成功/回滚路径均不会自动删除），文案给出绝对路径。
						return rollbackFailed("%s", replaceArtifactMismatchMessage(page, dstName, keptBackup))
					}
					return rollbackFailed("第 %d 页产物重命名失败（备份历史产物）: %v", page, bakErr)
				}
				backups[oldPath] = bak
			default:
				bak := replaceArtifactBackupPath(outDir, oldName)
				if bakErr := os.Rename(oldPath, bak); bakErr != nil {
					return rollbackFailed("第 %d 页产物重命名失败（备份历史产物）: %v", page, bakErr)
				}
				backups[oldPath] = bak
			}
		} else if fileExists(dst) {
			// 非历史产物（用户自建或未索引残留）：禁覆盖，中止并回滚本次新增
			return rollbackFailed("第 %d 页目标文件「%s」已存在且不属于历史拆分产物，请先移走或重命名后重试", page, dstName)
		}
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

	// 页级指纹（v10）：逐页 rename + Stat 通过后按最终文件名与页序计算；删源失败
	// 补写记录与成功记录两处共用同一份。计算失败降级为空指纹（回落 mtime 守卫）
	// 并 Warn，不回滚已成功的拆分与产物（产物可能已落盘甚至已删源）。
	fingerprints := s.splitArtifactFingerprints(lib, rel, renamed, pages)

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
			// 系统性错误（db 写失败）：产物、已写索引行与被覆盖的历史产物一并回滚
			rollbackArtifacts(fmt.Sprintf("索引失败（%s）: %v", prodRelSlash, scanErr))
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
		rollbackArtifacts(fmt.Sprintf("标注迁移失败: %v", err))
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
	// 取消标志复查：CancelAssetSplit 先摘除注册条目再调用 cancel，故「条目已不在
	// 注册表」比 ctx.Err() 更早可见。摘除与 cancel 之间、以及本次复查之后紧接
	// 到达的取消仍无法原子消除（复查后已无回滚动作，源移入系统回收站仍可还原
	// ——可恢复删除是兜底），该残余窗口在 docs/spec/pptx-split.md §9.3 记明。
	if s.splitCancelledAfterCheckpoint(ctx, lib.ID, entry) {
		return cancelRollback(context.Canceled)
	}
	// 历史产物收尾清理延迟到此刻：此前任何失败/取消都能保住旧页（见步骤 3/5）。
	// 本次新页之外的旧页（页数减少/页序变化）连同索引行删除，防僵尸素材；删除
	// 失败保留文件与索引，不阻断删源——失败页的归属凭证（指纹或旧 retained）随
	// 记录写入 retained_fingerprints（v11），使后续增页重拆仍能识别其为历史产物，
	// 不再因「同形文件」被永久拒绝。
	retained := s.cleanupStaleReplaceArtifacts(store, lib, record, outDir, oldArtifacts, renamed)
	// retained 归属凭证消费（R1）：cleanupStaleReplaceArtifacts 只按本次指纹判定，
	// retained 由独立单点消费。先按本次新页名过滤——同名条目此刻描述的是刚生成的
	// 新页，若内容恰与凭证全等（拆分字节确定性）直接消费会误删新页，且该名字已是
	// 本次产物，旧凭证不应再携带。dir 取记录的 output_dir（凭证名相对该目录）。
	consumedRecord := recordWithoutRetainedNames(record, renamed)
	retainedDir := ""
	if record != nil {
		retainedDir = record.outputDir
	}
	retained = mergeSplitFingerprintsByName(retained,
		s.cleanupRetainedSplitArtifacts(store, lib, consumedRecord, retainedDir, ""))
	// 旧产物目录收尾（与 source 路径同一决策单点 splitArtifactCleanupPlan）：异目录
	// 清理旧落点；同目录跨 target 清理旧形态产物；同目录同 target 但记录主干 !=
	// 本次新主干时清理旧主干并以 excludeStem 排除本次新产物；同目录同主干（普通
	// 重拆）不调用，旧页由上方 cleanupStaleReplaceArtifacts 处理。刻意挂在成功收尾
	// （删源前）——失败/取消路径不得删旧产物，与 oldArtifacts 延迟清理契约一致。
	if dir, excludeStem, ok := splitArtifactCleanupPlan(record, model.AssetSplitTargetReplace, replaceArtifactBase(base), outDir); ok {
		s.cleanupSplitArtifactsInDirByPage(store, lib, record, dir, excludeStem)
	}
	// 备份到此已无还原场景（后续唯一动作是移入回收站，失败也不回滚），先删除，
	// 避免成功路径残留备份文件；删除失败仅告警（备份不匹配产物形态、不参与索引）。
	s.removeReplaceArtifactBackups(backups)
	move := s.moveToRecycleBinFn
	if move == nil {
		move = util.MoveToRecycleBin
	}
	if err := move(absPath); err != nil {
		Logger().Warn("就地替换删源失败（产物保留）", "libraryId", lib.ID, "file", rel, "err", err)
		// 删源失败也必须落拆分记录：产物已完整生成并进索引，记录是「目录内同形文件
		// 属本文件产物」的唯一归属证据（步骤 3 归属判定、批量剔除、重试重拆都依赖
		// 它）。source_removed 如实记 0（源仍在）；不记录会让重试被「同形文件」拒绝、
		// 批量把产物当新源再拆成「页的页」。写失败仅告警（与步骤 10 同口径）。
		if recErr := store.upsertSplitRecord(rel, hash, outDir, len(renamed),
			time.Now().Format("2006-01-02 15:04:05"), model.AssetSplitTargetReplace,
			replaceArtifactBase(base), false, fingerprints, retained); recErr != nil {
			Logger().Warn("就地替换删源失败后写拆分记录失败（产物归属缺失，重试与剔除可能失效）",
				"libraryId", lib.ID, "file", rel, "err", recErr)
		}
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
		time.Now().Format("2006-01-02 15:04:05"), model.AssetSplitTargetReplace,
		replaceArtifactBase(base), true, fingerprints, retained); err != nil {
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

// relPathWithinRoot 库内相对路径判定单点：abs 位于 root 内（含 root 自身）时返回
// 正斜杠相对路径与 true；库外（Rel 出错或 `..` 前缀）返回 "" 与 false。与
// assetRelPathWithinRoot 的区别是不返回错误——供「库外即静默跳过」的分支复用。
func relPathWithinRoot(root, abs string) (string, bool) {
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// deleteIndexRowsForPaths 删除已清理产物在索引中的行（含缩略图记账与 PNG 文件）：
// 页数减少/重拆时越界旧页必须从索引消失，否则成为指向已删文件的僵尸素材。
// 路径按库根换算为相对路径查询；不存在行（未索引/已被删）静默跳过。
//
// 库外旧落点（derived 目录位于库根之外，其产物本就不入索引）不做索引行清理：
// 同卷时 Rel 返回 `..` 前缀、跨卷时 Rel 直接出错，两者均视为库外静默跳过，
// 不再逐文件告警（跨卷场景曾被误报为噪音，PRD 11-02-derived-cross-volume-warn-noise）。
func (s *AssetLibraryService) deleteIndexRowsForPaths(store *assetsStore, lib model.AssetLibrary, paths []string) {
	for _, p := range paths {
		rel, inRoot := relPathWithinRoot(lib.RootPath, p)
		if !inRoot {
			Logger().Debug("旧产物位于库根之外，跳过索引行清理", "libraryId", lib.ID, "path", p)
			continue
		}
		rec, exists, err := store.fileRecordByPath(rel)
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
	s.deleteIndexRowsForPaths(store, lib, removed)
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
		// 其余落点为 <源名主干>-<页序>.pptx（核心原生命名）；主干经
		// splitArtifactPath 解析（改名重链后回退为 output_dir 内按页序反查旧产物）
		pagePath, _ = splitArtifactPath(record, rel, slideNum)
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

// splitCancelledAfterCheckpoint 删源前的取消标志复查（只读，不改注册表）：
// ctx 已取消直接命中；否则查注册表中本操作的条目是否仍在——CancelAssetSplit
// 先摘除条目再调用 cancel，二者之间 ctx.Err() 尚为 nil，而条目已消失。entry
// 为 nil（非入口链路直接调用）时只看 ctx。锁形态：仅短持 opMu 读一次，不在
// 持锁期间做任何 IO，不引入新的锁序。
func (s *AssetLibraryService) splitCancelledAfterCheckpoint(ctx context.Context, libraryID string, entry *cancelEntry) bool {
	if ctx.Err() != nil {
		return true
	}
	if entry == nil {
		return false
	}
	key := normalizeRepoKey(libraryID)
	s.opMu.Lock()
	current, ok := s.splitCancels[key]
	s.opMu.Unlock()
	return !ok || current != entry
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

// splitStemUsable 源名主干可用于推导落点：空主干（文件名恰为 `.pptx`）与清理后
// 退化为 `.` / `..` 的形态（`..pptx` / `...pptx`）都会让「源所在目录 + 主干」
// 退化成源所在目录本身，产物将写进源目录；拆分入口与重链搬迁探测共用本判定。
func splitStemUsable(base string) bool {
	return base != "" && base != "." && base != ".."
}

// splitSourceOutputDir 源同目录落点推导：`<源所在目录>/<源名主干>`。source 落点、
// replace 落点（`splitOneFileReplace` 同布局）与重链搬迁探测（
// resolveRelocatedSplitOutDir）共用本单点，禁各处手拼 Join(Dir(...), stem)
// ——布局一旦分叉，探测到的候选目录就不再是拆分实际写出的目录。
func splitSourceOutputDir(absPath, rel string) string {
	return filepath.Join(filepath.Dir(absPath), splitSourceBaseName(rel))
}

// splitArtifactPath 拆分单页产物路径解析单点（打开单页 / 导出 / 增量判定共用）：
// 先按记录键推导源名主干直拼（产物随源同名的常规场景）；直拼不存在时在
// record.outputDir 内按页序反查——源文件经 RelocateAssetFile 改名重链后，记录键
// 的主干（新名）与产物命名主干（旧名）不一致。记录虽已持久化 artifact_stem（v9），
// 但此处保留「当前主干直拼 + 页序反查」的宽容形态：直拼命中即最新产物，反查兜住
// 改名重链与历史记录，语义单点不变。目录由该记录独占，且要求候选唯一（多候选
// 视为不可判定，返回直拼路径供报错指向）。返回 (路径, 是否存在)。
//
// 反查候选还须校验主干归属：页序匹配是唯一定位口径，但目录内可能混入其他源文件
// 的同页序产物（落点归属冲突的历史数据），主干不属于记录候选集合即不采纳——否则
// 增量判定会把他人产物误判为「本记录首页存在」而跳过拆分，打开单页也指向错误文件。
func splitArtifactPath(record *assetSplitRecord, relPath string, slideNum int) (string, bool) {
	direct := splitArtifactPathForStem(record, splitSourceBaseName(relPath), slideNum)
	if fileExists(direct) {
		return direct, true
	}
	if alt, ok := findSplitArtifactByPage(record.outputDir, record.target, slideNum); ok {
		stem, _, parsed := parseSplitArtifactName(filepath.Base(alt), record.target)
		if parsed && splitStemInCandidates(stem, splitArtifactStemCandidates(record, record.outputDir)) {
			return alt, true
		}
	}
	return direct, false
}

// splitArtifactPathForStem 按指定主干拼产物路径（命名形态随落点：replace 为
// <3位页序>-<主干>.pptx，其余为 <主干>-<3位页序>.pptx）。
func splitArtifactPathForStem(record *assetSplitRecord, stem string, slideNum int) string {
	if record.target == model.AssetSplitTargetReplace {
		return filepath.Join(record.outputDir, replaceArtifactName(slideNum, stem))
	}
	return filepath.Join(record.outputDir, fmt.Sprintf("%s-%03d.pptx", stem, slideNum))
}

// parseSplitArtifactName 解析产物文件名：命名主干 + 页序（页序 ≥1）。
// 形态随落点：replace 为 <纯数字页序>-<主干>.pptx（主干须非空）；其余为
// <主干>-<纯数字页序>.pptx（主干可为空，兼容历史 `-001.pptx`）。与
// findSplitArtifactByPage 的形态判定同源，禁另写一套。
func parseSplitArtifactName(name, target string) (stem string, page int, ok bool) {
	if !strings.HasSuffix(name, ".pptx") {
		return "", 0, false
	}
	if target == model.AssetSplitTargetReplace {
		// replace 形态：<纯数字页序>-<主干>.pptx（页序段位数不限，v8 前后兼容）
		idx := strings.IndexByte(name, '-')
		if idx <= 0 || idx >= len(name)-len("-")-len(".pptx") || !isAllDigits(name[:idx]) {
			return "", 0, false
		}
		n, convErr := strconv.Atoi(name[:idx])
		if convErr != nil || n <= 0 {
			return "", 0, false
		}
		return strings.TrimSuffix(name[idx+1:], ".pptx"), n, true
	}
	// source/cache 形态：<主干>-<纯数字页序>.pptx（主干允许为空）
	n := pageSeqFromArtifactName(name)
	if n <= 0 {
		return "", 0, false
	}
	base := strings.TrimSuffix(name, ".pptx")
	idx := strings.LastIndexByte(base, '-')
	if idx < 0 {
		// 不可达：pageSeqFromArtifactName 已保证末尾存在 "-<纯数字>" 段
		return "", 0, false
	}
	return base[:idx], n, true
}

// findSplitArtifactByPage output_dir 内按页序反查唯一产物（改名重链回退路径）。
// 目录不存在/不可读一律视为未命中；同页序候选多于一个视为不可判定（用户在该
// 目录手工放入同页序文件时不猜），返回 false。
func findSplitArtifactByPage(outputDir, target string, slideNum int) (string, bool) {
	if slideNum <= 0 {
		return "", false
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return "", false
	}
	var hit string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, page, ok := parseSplitArtifactName(e.Name(), target); !ok || page != slideNum {
			continue
		}
		if hit != "" {
			return "", false // 多候选不可判定
		}
		hit = filepath.Join(outputDir, e.Name())
	}
	return hit, hit != ""
}

// splitArtifactCandidate 旧产物目录内的形态匹配候选（清理判定用）。
type splitArtifactCandidate struct {
	path string
	stem string
}

// collectSplitArtifactsByPage 一次读目录，收集 1..pageCount 范围内形态匹配候选，
// 返回 page→候选切片 与 stem→该主干命中的页序集合（基名锚定判定用）。目录不可读
// 错误原样上抛（调用方区分「无旧产物可清」的正常路径，禁统一按不可判定告警）。
//
// 候选只取常规文件：符号链接/junction/设备等非常规条目即使名字形态匹配也不参与
// 清理（删除链接目标或特殊文件的风险远大于「漏清一个产物」，同样一律跳过子目录）。
func collectSplitArtifactsByPage(dir, target string, pageCount int) (map[int][]splitArtifactCandidate, map[string]map[int]bool, error) {
	byPage := make(map[int][]splitArtifactCandidate)
	byStem := make(map[string]map[int]bool)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return byPage, byStem, err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		stem, page, ok := parseSplitArtifactName(e.Name(), target)
		if !ok || page > pageCount {
			continue
		}
		byPage[page] = append(byPage[page], splitArtifactCandidate{
			path: filepath.Join(dir, e.Name()), stem: stem,
		})
		pages := byStem[stem]
		if pages == nil {
			pages = make(map[int]bool)
			byStem[stem] = pages
		}
		pages[page] = true
	}
	return byPage, byStem, nil
}

// splitArtifactStemCandidates 产物主干锚点候选：记录持久化的 artifact_stem 为
// 唯一权威（v9 起，清理与重链探测共用）。空串 = v9 前旧记录，回落目录启发式
// {基名, replace 数字主干变体} 保持兼容——replace 纯数字主干经 -p 后缀后与目录
// 基名合法不一致（`123` 目录 ↔ `123-p` 产物），双候选覆盖两形态。
func splitArtifactStemCandidates(record *assetSplitRecord, dir string) []string {
	if record != nil && record.artifactStem != "" {
		return []string{record.artifactStem}
	}
	anchor := filepath.Base(dir)
	cands := []string{anchor}
	if alt := replaceArtifactBase(anchor); alt != anchor {
		cands = append(cands, alt)
	}
	return cands
}

// splitStemInCandidates 主干是否命中候选集合（精确或大小写折叠）——重链探测归属
// 校验用（M2）：候选目录首页产物主干必须属于记录主干，否则该目录属于其他源文件。
//
// 折叠刻意不接 sameNameFold 平台守卫（本任务 Out of Scope）：输入 stem 均由
// parseSplitArtifactName 从产物命名形态解析，属产物主干归属判定而非用户文件名
// 身份判定；若后续发现用户可达的误采纳面，另立任务收口。
func splitStemInCandidates(stem string, candidates []string) bool {
	for _, cand := range candidates {
		if stem == cand || strings.EqualFold(stem, cand) {
			return true
		}
	}
	return false
}

// splitStemGroupPresent 目录内是否存在与锚点候选精确或大小写折叠匹配的候选组。
// 用于区分清理的两种 !ok 结局：候选组不存在 = 无旧产物可清的正常路径（静默，
// 如上次重拆已清理、用户手动清理、M1 同目录重拆时目录内只剩本次新主干产物）；
// 存在候选组但折叠多命中 = 不可判定（告警并零删除）。
func splitStemGroupPresent(byStem map[string]map[int]bool, candidates []string) bool {
	for stem, pages := range byStem {
		if len(pages) == 0 {
			continue
		}
		if splitStemInCandidates(stem, candidates) {
			return true
		}
	}
	return false
}

// dominantSplitArtifactStem 从目录内候选主干中选定产物组主干：按锚点候选集合
// 精确匹配优先；无精确命中时才接受 sameNameFold 唯一命中（仅 Windows 生效：
// 文件系统大小写不敏感，主干大小写变体仍认作同一组；Linux/macOS 大小写敏感，
// 折叠会把用户文件组误选为产物组并删除）。折叠命中多于一个（大小写敏感文件系统
// 上 `Old` 与 `old` 并存）视为不可判定——禁按 map 遍历顺序取首个，否则同一目录
// 状态会随机选中用户文件组并误删。
func dominantSplitArtifactStem(byStem map[string]map[int]bool, candidates []string) (string, bool) {
	for _, anchor := range candidates {
		if len(byStem[anchor]) > 0 {
			return anchor, true
		}
	}
	foldStem := ""
	foldHits := 0
	for stem, pages := range byStem {
		if len(pages) == 0 {
			continue
		}
		for _, anchor := range candidates {
			if sameNameFold(stem, anchor) {
				foldStem, foldHits = stem, foldHits+1
				break
			}
		}
	}
	if foldHits == 1 {
		return foldStem, true
	}
	return "", false
}

// splitArtifactFingerprints 拆分成功后计算产物页级指纹（v10）。pages 为 nil 时按
// 产物文件名解析页序（解析失败回退下标 + 1）；构建失败降级为空指纹并 Warn——产物
// 已成功落盘（replace 甚至可能已删源），指纹记账失败不应回滚成果，失效方向为
// 「回落 mtime 守卫」而非误删。
func (s *AssetLibraryService) splitArtifactFingerprints(lib model.AssetLibrary, rel string, paths []string, pages []int) []splitArtifactFingerprint {
	if len(paths) == 0 {
		return nil
	}
	if pages == nil {
		pages = make([]int, len(paths))
		for i, p := range paths {
			page := pageSeqFromArtifactName(filepath.Base(p))
			if page <= 0 {
				page = i + 1
			}
			pages[i] = page
		}
	}
	fps, err := buildSplitFingerprints(paths, pages)
	if err != nil {
		Logger().Warn("计算拆分产物页级指纹失败，本次记录降级为无指纹（旧产物清理回落 mtime 守卫）",
			"libraryId", lib.ID, "file", rel, "err", err)
		return nil
	}
	return fps
}

// removeSplitArtifactsBeyond 源目录/derived 落点重拆后的旧页收敛（延迟清理）：
// 只删除匹配产物命名形态 `<源名主干>-<纯数字>.pptx` 的文件，删除条件二选一——
//   - 页序 > keepPage（页数减少的越界旧页）；
//   - 页序 ≤ keepPage 但文件名不是当前形态 `fmt.Sprintf("%s-%03d.pptx", baseName, seq)`
//     （历史 %02d 产物，如 `演变-01.pptx` 与新 `演变-001.pptx` 并存）。
//
// 页序段纯数字、不限位数；主干段按 sameNameFold 比较（仅 Windows 折叠大小写，
// Windows 仅改大小写改名后 `Abc-003.pptx` 仍识别为产物；Linux/macOS 大小写敏感，
// 大小写变体是用户文件不识别）。目录内其余内容（用户自建文件、`<源名>-草稿.pptx`
// 等同名异构文件、子目录、非 .pptx）一律不动。
//
// 刻意在拆分核心成功之后调用（keepPage = 本次 result.PageCount）：核心失败/取消
// 时上一次拆分成果不再被前置删除，与 replace 落点的延迟清理契约一致（核心已同名
// 覆盖写的页除外，属已知限制，见 splitOneFileWithHash 步骤 4）。页序 ≤ keepPage
// 且为当前形态的产物由核心同名覆盖写更新，无需删除；keepPage=0 即全量清理产物形态。
//
// 归属判定（v10）：旧记录指纹可用时以指纹为 allow-list——只删指纹条目（页序 >
// keepPage，或页序 ≤ keepPage 但指纹名不是当前形态）且 `size + sha256` 全等的
// 文件；不在指纹中的候选（用户同形文件，任意 mtime）一律保留。指纹缺失/损坏时
// 回落既有 mtime 守卫（splitAt = 上一次拆分记录的分拆时间，候选 mtime 晚于
// splitAt + splitArtifactUserFileGrace 视为用户后放入的同名文件保留）。
//
// 成功删除的路径批量交 deleteIndexRowsForPaths 清理索引行与缩略图（源落点在库内
// 时旧页有 files 行，漏清即留下指向已删文件的僵尸素材；derived 落点库外自动静默
// 跳过）。目录不存在或不可读时静默返回——无旧页可清。删除失败仅告警继续，残留
// 文件保留（可能因占用或权限失败，不再声称「下次重拆自动收敛」）。
func (s *AssetLibraryService) removeSplitArtifactsBeyond(store *assetsStore, lib model.AssetLibrary, record *assetSplitRecord, dir, baseName string, keepPage int, splitAt string) {
	if fingerprintsUsable(record) {
		s.removeSplitArtifactsBeyondByFingerprint(store, lib, record, dir, baseName, keepPage)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	removed := make([]string, 0, 4)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		seq, ok := splitArtifactPageSeq(name, baseName)
		if !ok {
			continue
		}
		n, convErr := strconv.Atoi(seq)
		if convErr != nil {
			continue
		}
		if n <= keepPage && sameNameFold(name, fmt.Sprintf("%s-%03d.pptx", baseName, n)) {
			continue // 当前形态的正常页，核心已同名覆盖写
		}
		path := filepath.Join(dir, name)
		if isSplitArtifactAfterSplitAt(path, splitAt) {
			Logger().Debug("旧页候选 mtime 晚于拆分时间，按用户文件保留",
				"libraryId", lib.ID, "path", path, "splitAt", splitAt)
			continue
		}
		if rmErr := removeSplitArtifactFile(path); rmErr != nil {
			Logger().Warn("删除超界旧拆分产物失败（残留文件需人工清理）",
				"libraryId", lib.ID, "dir", dir, "file", name, "err", rmErr)
			continue
		}
		removed = append(removed, path)
	}
	if len(removed) > 0 {
		s.deleteIndexRowsForPaths(store, lib, removed)
	}
}

// removeSplitArtifactsBeyondByFingerprint v10 指纹分支：只删指纹条目描述的页产物，
// 不扫描目录删其他文件——用户同形文件不在指纹中即保留（任意 mtime）。
func (s *AssetLibraryService) removeSplitArtifactsBeyondByFingerprint(store *assetsStore, lib model.AssetLibrary, record *assetSplitRecord, dir, baseName string, keepPage int) {
	removed := make([]string, 0, 4)
	for _, fp := range record.fingerprints {
		// 页序 ≤ keepPage 且已是当前形态的正常页由核心同名覆盖写更新，不删除；
		// 历史 %02d 形态（指纹名与当前形态不同）仍需收敛。
		if fp.Page <= keepPage && sameNameFold(fp.Name, fmt.Sprintf("%s-%03d.pptx", baseName, fp.Page)) {
			continue
		}
		if !splitFingerprintNameValid(fp.Name) {
			Logger().Warn("拆分记录指纹文件名非法（疑似路径穿越），跳过",
				"libraryId", lib.ID, "dir", dir, "name", fp.Name)
			continue
		}
		path := filepath.Join(dir, fp.Name)
		if !fingerprintMatch(path, fp) {
			continue // 不存在或内容/大小不符：按用户文件保留
		}
		if rmErr := removeSplitArtifactFile(path); rmErr != nil {
			Logger().Warn("删除超界旧拆分产物失败（残留文件需人工清理）",
				"libraryId", lib.ID, "dir", dir, "file", fp.Name, "err", rmErr)
			continue
		}
		removed = append(removed, path)
	}
	if len(removed) > 0 {
		s.deleteIndexRowsForPaths(store, lib, removed)
	}
}

// splitArtifactPageSeq 解析源目录/derived 落点产物文件名 `%s-%s.pptx` 的页序段：
// 返回纯数字页序串与是否命中。主干段按 sameNameFold 比较（仅 Windows 折叠大小写）；
// 从每个 '-' 处切分逐一尝试，兼容主干本身含 '-' 或数字（如 `方案-A`、`2026`）。
// 非产物形态（非 .pptx、后缀/主干不符、页序非纯数字）返回 false。
func splitArtifactPageSeq(name, baseName string) (string, bool) {
	const ext = ".pptx"
	if !strings.HasSuffix(name, ext) {
		return "", false
	}
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < len(stem); i++ {
		if stem[i] != '-' || !sameNameFold(stem[:i], baseName) {
			continue
		}
		if seq := stem[i+1:]; isAllDigits(seq) {
			return seq, true
		}
	}
	return "", false
}

// removeSplitArtifactFile 删除单个旧产物文件，首次失败后间隔 50ms 重试一次——
// Windows 上杀毒/索引服务的短时句柄占用常见，一次重试覆盖瞬时冲突；仍失败返回
// 错误由调用方告警。文件已不存在（并发/用户清理）按成功处理。
func removeSplitArtifactFile(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	time.Sleep(50 * time.Millisecond)
	if retryErr := os.Remove(path); retryErr != nil && !os.IsNotExist(retryErr) {
		return retryErr
	}
	return nil
}

// splitArtifactCleanupPlan 重拆成功后的旧产物清理决策单点（source/cache 与 replace
// 两条链路共用，防口径分叉）。返回要清理的旧目录与需保护的本次新主干：
//  1. 异目录（record.outputDir != 本次落点）：旧落点产物按记录持久化主干清理，
//     不按 target 分叉——cache↔source↔replace 任一切换都须收敛旧产物与索引行
//     （M4）；
//  2. 同目录但记录 target != 本次落点：旧产物命名形态由 record.target 解析（如
//     replace `<页序>-<主干>` 与 source `<主干>-<页序>` 不同），新产物形态不会被
//     收集，无需 excludeStem；
//  3. 同目录同 target 但记录主干 != 本次新主干（改名重链 / 搬迁后）：清理旧主干，
//     以本次新主干 exclude 防误删刚生成的新产物（M1）；仅 Windows 下大小写差异
//     折叠相等视为同一主干不进入——本次产物覆盖同名物理文件，旧页由既有链路处理；
//     Linux/macOS 大小写敏感，大小写变体是不同文件，按改主干清理旧组；
//  4. 同目录同 target 同主干（普通重拆）：不清理，旧页由 removeSplitArtifactsBeyond /
//     cleanupStaleReplaceArtifacts 既有链路处理。
func splitArtifactCleanupPlan(record *assetSplitRecord, currentTarget, currentStem, currentOutDir string) (dir, excludeStem string, ok bool) {
	if record == nil {
		return "", "", false
	}
	switch {
	case !samePathFold(record.outputDir, currentOutDir):
		return record.outputDir, "", true
	case record.target != currentTarget:
		return record.outputDir, "", true
	case record.artifactStem != "" && record.artifactStem != currentStem &&
		!sameNameFold(record.artifactStem, currentStem):
		return record.outputDir, currentStem, true
	default:
		return "", "", false
	}
}

// splitArtifactUserFileGrace 旧产物候选 mtime 与记录拆分时间的容忍窗口：产物在
// 拆分成功后写入记录，故产物 mtime 不晚于 split_at；晚于 split_at + 该容差视为
// 用户后放入的同名文件。窗口吸收文件系统时间戳粒度与拆分收尾耗时。
const splitArtifactUserFileGrace = 2 * time.Second

// isSplitArtifactAfterSplitAt 候选文件是否晚于本次记录的分拆时间（视为用户后放入
// 的同名文件，清理须跳过）。拆分记录只存秒级时间串；splitAt 解析失败时守卫不生效
// （返回 false，保持无守卫的旧行为）；文件 Stat 失败按不可判定保留（返回 true，
// 安全侧不删）。
func isSplitArtifactAfterSplitAt(path, splitAt string) bool {
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", splitAt, time.Local)
	if err != nil {
		return false
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return true
	}
	return info.ModTime().After(ts.Add(splitArtifactUserFileGrace))
}

// cleanupSplitArtifactsInDirByPage 清理旧产物目录中的页产物（含其索引行与缩略图）。
// 用于源文件改名重链 / 跨落点切换 / 同目录改主干等重拆清旧场景：记录键可能已迁到
// 新路径，但 output_dir 与产物命名主干仍是上次拆分的旧值，按当前主干清理必然漏删
// 旧页并留下 files 索引行（旧页还能再次进入批量拆分源）。
//
// 产物组判定 = `split_records.artifact_stem` 锚定（v9 唯一权威）：记录持久化了本次
// 实际产物主干，命中即产物组，只删该组内同页序唯一的常规文件；空值（v9 前旧记录）
// 回落目录启发式 {filepath.Base(dir), replaceArtifactBase(基名)}，覆盖 replace 纯
// 数字主干加 -p 后与目录名合法不一致的形态。excludeStem 与候选主干**精确相等**才
// 跳过——同目录重拆（M1）保护本次刚生成的新主干产物；仅大小写差异视为同一主干，
// 由门控（splitArtifactCleanupPlan）挡在调用之外，此处折叠比较会让旧页整组漏清。
// 页数覆盖不参与选组（覆盖不足如旧产物仅剩页 2 同样清理）。
//
// mtime 守卫（2026-11-02；仅指纹不可用的回落路径）：删除候选前判
// isSplitArtifactAfterSplitAt——候选 mtime 晚于记录 split_at +
// splitArtifactUserFileGrace 视为用户后放入的同名文件保留（v10 前旧记录无内容
// 指纹，mtime 是归属的最低成本判据；失效方向为漏清而非误删）；split_at 解析
// 失败时守卫不生效（保持旧行为），Stat 失败按不可判定保留。指纹可用时改走
// cleanupSplitArtifactsByFingerprint（size + SHA-256 校验，不看 mtime）。
//
// 不可判定（大小写折叠多命中，或回落启发式时目录基名与所有组主干均不匹配）按安全
// 侧失败：整目录零删除并 Warn，残留交人工清理。记录主干非空但目录内已无该主干组
// （上次重拆已清理 / M1 同目录重拆只剩本次新主干产物）属正常路径静默——无旧产物
// 可清，告警只会成为每次改名重链后重拆的噪音。同主干同页序存在多个候选（如
// `<主干>-1.pptx` 与 `<主干>-001.pptx`）时该页跳过并 Warn（不再静默）。目录不存在
// 或目录内无任何形态匹配候选属正常路径（首次重拆无旧产物、用户已自行清理），静默
// 返回；权限拒绝等非不存在读错误会 Warn（防静默失败）。
//
// 调用点均在拆分成功之后（源落点核心返回成功后、replace 删源前），失败与取消
// 路径不触达，上一次拆分成果原样保留。
//
// 删除失败先重试一次（50ms 间隔，removeSplitArtifactFile），仍失败仅告警继续并
// 明确残留需人工清理——本次重拆后记录主干/落点已被新值覆盖，残留页不再被任何
// 记录引用，下次重拆不会自动收敛（勿误称「下次重拆收敛」）。
// 目录删空后尝试删除（不存在/非空静默——目录内用户自建内容一律不动）。
func (s *AssetLibraryService) cleanupSplitArtifactsInDirByPage(store *assetsStore, lib model.AssetLibrary, record *assetSplitRecord, dir, excludeStem string) {
	if record == nil || dir == "" || record.pageCount <= 0 {
		return
	}
	// v10 指纹可用：指纹即 allow-list，逐页校验删除（不再走目录主干启发式——
	// 天然覆盖 source/replace 两种命名形态，也消除「目录基名不匹配」「同页多候选」
	// 不可判定分支）。
	if fingerprintsUsable(record) {
		s.cleanupSplitArtifactsByFingerprint(store, lib, record, dir, excludeStem)
		return
	}
	byPage, byStem, readErr := collectSplitArtifactsByPage(dir, record.target, record.pageCount)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			// 目录不存在：首次重拆无旧产物、或用户已自行清理，属正常路径。
			return
		}
		Logger().Warn("读取旧拆分产物目录失败（跳过清理）",
			"libraryId", lib.ID, "dir", dir, "err", readErr)
		return
	}
	if len(byStem) == 0 {
		// 目录内没有形态匹配候选：无旧产物可清，静默返回；顺手收敛空目录
		// （与原「删空才删目录」语义一致，空壳残留不影响后续拆分）。
		if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
			if rmErr := os.Remove(dir); !isBenignDirRemoveErr(rmErr) {
				Logger().Warn("删除空旧产物目录失败（保留）", "libraryId", lib.ID, "dir", dir, "err", rmErr)
			}
		}
		return
	}
	candidates := splitArtifactStemCandidates(record, dir)
	productStem, ok := dominantSplitArtifactStem(byStem, candidates)
	if !ok {
		if record.artifactStem != "" && !splitStemGroupPresent(byStem, candidates) {
			// 记录主干权威模式下目录内已无该主干组：旧产物已被前次重拆/用户清理，
			// 无旧产物可清。M1 同目录重拆时目录内恰只剩本次新主干产物（被
			// excludeStem 保护），同属正常路径——禁告警，否则每次改名重链后重拆
			// 都刷一条「主干不匹配」噪音。
			return
		}
		Logger().Warn("旧产物目录基名与产物主干不匹配（或多候选择一不可判定），跳过清理（保留全部文件）",
			"libraryId", lib.ID, "dir", dir, "artifactStem", record.artifactStem,
			"pageCount", record.pageCount, "stemGroups", len(byStem))
		return
	}
	removed := make([]string, 0, record.pageCount)
	for page := 1; page <= record.pageCount; page++ {
		// 锚定已确定产物组主干：仅在**同主干**候选内要求同页序唯一；同页序其他
		// 主干（如 replace 形态用户自建的 `001-笔记.pptx`）是用户文件，不参与
		// 判定也不影响产物删除（否则用户文件会遮蔽同页产物，形成新的漏清路径）。
		cands := byPage[page]
		hit := ""
		ambiguous := false
		for _, cand := range cands {
			// 仅同主干候选参与判定；excludeStem 为本次新主干，精确相等即跳过防误删
			// 本次产物（同目录重拆 M1）。大小写折叠相等不在此处理：M1 门控已把「仅
			// 大小写差异」排除在清理调用之外，此处若折叠跳过会让旧页整组漏清。
			if cand.stem != productStem || (excludeStem != "" && cand.stem == excludeStem) {
				continue
			}
			if hit != "" {
				ambiguous = true // 同主干同页序多候选（如 <主干>-1.pptx 与 <主干>-001.pptx）：不可判定
				break
			}
			hit = cand.path
		}
		if ambiguous {
			Logger().Warn("旧产物同页序多候选，跳过该页", "libraryId", lib.ID, "dir", dir,
				"page", page, "stem", productStem)
			continue
		}
		if hit == "" {
			continue
		}
		// mtime 守卫（v10 指纹不可用时的回落）：mtime 是归属的最低成本判据——晚于
		// 记录拆分时间的同名文件视为用户后放入，保留不删（失效方向为漏清而非误删）。
		// 正常路径可高频命中，不告警（Debug 留痕）。
		if isSplitArtifactAfterSplitAt(hit, record.splitAt) {
			Logger().Debug("旧产物候选 mtime 晚于拆分时间，按用户文件保留",
				"libraryId", lib.ID, "path", hit, "splitAt", record.splitAt)
			continue
		}
		if rmErr := removeSplitArtifactFile(hit); rmErr != nil {
			Logger().Warn("删除旧拆分产物失败（残留需人工清理，本次重拆不再自动收敛）",
				"libraryId", lib.ID, "path", hit, "err", rmErr)
			continue
		}
		removed = append(removed, hit)
	}
	if len(removed) > 0 {
		s.deleteIndexRowsForPaths(store, lib, removed)
	}
	// 仅当目录已空才删除：非空 = 用户自建内容，保留且不算异常（不产生告警噪音）。
	if entries, readErr := os.ReadDir(dir); readErr == nil && len(entries) == 0 {
		if rmErr := os.Remove(dir); !isBenignDirRemoveErr(rmErr) {
			Logger().Warn("删除改名重链前的旧产物目录失败（保留）", "libraryId", lib.ID, "dir", dir, "err", rmErr)
		}
	}
}

// cleanupSplitArtifactsByFingerprint v10 指纹分支的旧产物清理（ADR §8.1）：
// 逐页取指纹，按 `dir/<name>` Lstat —— 不存在跳过；非常规文件（符号链接/目录/
// 设备）跳过并 Warn；`size` 或重算 `sha256` 不符按用户文件保留（Debug 留痕）；
// 四条件（页 + 名 + size + sha256）全等才删除。指纹名必须是纯 basename，防脏
// 数据路径穿越。某页无指纹条目 = 不是本记录产物，保留且**不回落 mtime**（避免
// 把用户文件按旧启发式删掉）。
//
// excludeStem 为本次新主干（同目录改主干 M1 保护）：命中同主干候选跳过。正常情况
// 下旧指纹名是旧主干，不会与本次新主干相同；该判定为防御性兜底。
func (s *AssetLibraryService) cleanupSplitArtifactsByFingerprint(store *assetsStore, lib model.AssetLibrary, record *assetSplitRecord, dir, excludeStem string) {
	removed := make([]string, 0, record.pageCount)
	for page := 1; page <= record.pageCount; page++ {
		fp, ok := fingerprintForPage(record, page)
		if !ok {
			continue // 该页无指纹条目：不是本记录产物，保留
		}
		if !splitFingerprintNameValid(fp.Name) {
			Logger().Warn("拆分记录指纹文件名非法（疑似路径穿越），跳过",
				"libraryId", lib.ID, "dir", dir, "page", page, "name", fp.Name)
			continue
		}
		if excludeStem != "" {
			if stem, _, parsed := parseSplitArtifactName(fp.Name, record.target); parsed && stem == excludeStem {
				continue // 本次新产物（M1 同目录改主干），禁删
			}
		}
		path := filepath.Join(dir, fp.Name)
		info, statErr := os.Lstat(path)
		if statErr != nil {
			continue // 不存在（已清理/用户移走）属正常路径
		}
		if !info.Mode().IsRegular() {
			Logger().Warn("旧产物指纹命中非常规文件（符号链接/目录/设备），跳过",
				"libraryId", lib.ID, "path", path)
			continue
		}
		if info.Size() != fp.Size {
			Logger().Debug("旧产物指纹大小不符，按用户文件保留",
				"libraryId", lib.ID, "path", path, "want", fp.Size, "got", info.Size())
			continue
		}
		sum, hashErr := hashFileSHA256(path)
		if hashErr != nil {
			Logger().Debug("旧产物指纹重算哈希失败，按用户文件保留",
				"libraryId", lib.ID, "path", path, "err", hashErr)
			continue
		}
		if !strings.EqualFold(sum, fp.SHA256) {
			Logger().Debug("旧产物指纹内容不符，按用户文件保留",
				"libraryId", lib.ID, "path", path)
			continue
		}
		if rmErr := removeSplitArtifactFile(path); rmErr != nil {
			Logger().Warn("删除旧拆分产物失败（残留需人工清理，本次重拆不再自动收敛）",
				"libraryId", lib.ID, "path", path, "err", rmErr)
			continue
		}
		removed = append(removed, path)
	}
	if len(removed) > 0 {
		s.deleteIndexRowsForPaths(store, lib, removed)
	}
	// 仅当目录已空才删除：非空 = 用户自建内容，保留且不算异常（不产生告警噪音）。
	if entries, readErr := os.ReadDir(dir); readErr == nil && len(entries) == 0 {
		if rmErr := os.Remove(dir); !isBenignDirRemoveErr(rmErr) {
			Logger().Warn("删除改名重链前的旧产物目录失败（保留）", "libraryId", lib.ID, "dir", dir, "err", rmErr)
		}
	}
}

// isBenignDirRemoveErr 目录删除的可静默错误：不存在（已被并发/用户删除）与非空
// （目录内仍有用户保留内容）都不算异常，保留目录即可，不产生告警噪音。
// 非空错误码跨平台不同：Linux/macOS 为 ENOTEMPTY，Windows 为 ERROR_DIR_NOT_EMPTY
// (145)；后者无跨平台常量导出，按 GOOS 分支判断以免破坏非 Windows 构建。
func isBenignDirRemoveErr(err error) bool {
	if err == nil || os.IsNotExist(err) || errors.Is(err, syscall.ENOTEMPTY) {
		return true
	}
	if runtime.GOOS == "windows" {
		const errDirNotEmpty syscall.Errno = 145 // ERROR_DIR_NOT_EMPTY
		return errors.Is(err, errDirNotEmpty)
	}
	return false
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
// 判定精确保守，**仅 Windows 下追加一次大小写折叠**：Windows NTFS 保留大小写但
// 不敏感，用户把源文件仅改大小写（abc.pptx → Abc.pptx）后重拆，上次产物
// `001-abc.pptx` 与本次目标 `001-Abc.pptx` 是同一物理文件——按敏感比较会判为
// 「非历史产物」并因 fileExists 为真而永久回滚失败。Linux/macOS 大小写敏感，
// `001-ABC.pptx` 与 `001-abc.pptx` 是不同文件，折叠会把它收入历史产物并在成功后
// 删备份，造成用户文件内容丢失（P1）。因此折叠经 sameNameFold/ToLower 平台守卫。
//
// 回滚与重拆清旧页共用本判定；目录内其余内容（用户自建文件、草稿等）不动。
func isReplaceArtifactName(name, base string) bool {
	if isReplaceArtifactNameExact(name, base) {
		return true
	}
	if runtime.GOOS != "windows" {
		return false
	}
	// 折叠匹配复用精确逻辑：双侧同源 ToLower 后按**折叠串长度**切片（页序段为
	// ASCII 数字、纯数字主干的 -p 后缀不受大小写影响）。Unicode 大小写映射可能
	// 改变字节长度（如 İ→i̇ 由 2 字节变 3 字节），但 name 与 base 走同一变换，
	// 边界始终落在同一折叠串上，不会截半或越界。
	return isReplaceArtifactNameExact(strings.ToLower(name), strings.ToLower(base))
}

// isReplaceArtifactNameExact 大小写敏感的产物命名形态判定（精确匹配）。
func isReplaceArtifactNameExact(name, base string) bool {
	const ext = ".pptx"
	// 核心中间态：<源名主干>-<纯数字>.pptx（至少一位页序）
	if len(name) > len(base)+len(ext)+1 &&
		name[:len(base)] == base && name[len(base)] == '-' &&
		name[len(name)-len(ext):] == ext {
		seq := name[len(base)+1 : len(name)-len(ext)]
		if isAllDigits(seq) {
			return true
		}
	}
	// 已改名形态：<纯数字>-<名字主干>.pptx
	suffix := "-" + replaceArtifactBase(base) + ext
	if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
		seq := name[:len(name)-len(suffix)]
		return isAllDigits(seq)
	}
	return false
}

// foldNameInSet 集合成员判定，返回命中的实际键。大小写折叠经 sameNameFold（仅
// Windows 生效）：仅大小写差异的文件名在 Windows 上指同一物理文件，按实际名
// （而非查询名）构造路径才能定位到磁盘对象，并保证回滚还原后索引行路径不漂移；
// Linux/macOS 大小写敏感，折叠会把不同文件误判为集合成员（用户文件被当作历史
// 产物删除）。精确命中优先；正常 Windows 目录（大小写不敏感）下折叠命中至多
// 一个（大小写变体指向同一物理条目）。若目录启用了大小写敏感标志导致多命中，
// 结果为遍历顺序取首个——该形态不在本契约内（Windows 上大小写变体按同一物理
// 文件处理，多命中已属异常目录），调用方按取得的实际键操作即可。
func foldNameInSet(set map[string]struct{}, name string) (string, bool) {
	if _, ok := set[name]; ok {
		return name, true
	}
	for k := range set {
		if sameNameFold(k, name) {
			return k, true
		}
	}
	return "", false
}

// firstReplaceArtifactName 目录内第一个匹配产物命名形态的文件名（无则空串）。
// 用于「记录不属于本目录」时拒绝拆分：此刻目录内同形文件只能来自用户或历史
// 残留，覆盖/删除都可能丢用户数据，交回用户处置更安全（记录实际拥有该落点的
// 形态由 replaceOutDirOwnedByRecord 放宽，见下）。
func firstReplaceArtifactName(dir, base string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !isReplaceArtifactName(e.Name(), base) {
			continue
		}
		return e.Name()
	}
	return ""
}

// replaceOutDirOwnedByRecord 落点归属放宽单点（R1）：记录非空时遍历目录内产物
// 形态文件，只要有一个能经 ownershipFingerprintForName 命中（本次指纹或 v11
// retained）且内容（size + SHA-256）全等，即视为本记录拥有该落点——切换 target
// 后残留页/旧形态产物仍在目录内时应继续重拆，而不是按名永久拒绝。
//
// 刻意只认**凭证内容全等**的文件：无任何归属条目的同形用户文件不放宽（首层
// firstReplaceArtifactName 仍拒绝；混合场景下用户文件由步骤 5 逐页归属校验拒绝）。
func (s *AssetLibraryService) replaceOutDirOwnedByRecord(record *assetSplitRecord, outDir, base string) bool {
	if record == nil {
		return false
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !isReplaceArtifactName(e.Name(), base) {
			continue
		}
		fp, owns := ownershipFingerprintForName(record, e.Name())
		if !owns {
			continue
		}
		if fingerprintMatch(filepath.Join(outDir, e.Name()), fp) {
			return true
		}
	}
	return false
}

// firstUnownedReplaceCoreArtifactName 目录内第一个「核心中间态命名但无归属凭证
// （或凭证内容不符）」的文件名（无则空串）。落点归属放宽（replaceOutDirOwnedByRecord）
// 只放行凭证覆盖的历史产物；核心随后仍会按 `<base>-NNN.pptx` 同名覆盖写，无凭证
// 的核心同名文件是用户内容，必须早于核心拒绝（R5 守卫在放宽分支同样生效）。
func (s *AssetLibraryService) firstUnownedReplaceCoreArtifactName(record *assetSplitRecord, dir, base string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !isReplaceCoreArtifactName(e.Name(), base) {
			continue
		}
		fp, owns := ownershipFingerprintForName(record, e.Name())
		if owns && fingerprintMatch(filepath.Join(dir, e.Name()), fp) {
			continue
		}
		return e.Name()
	}
	return ""
}

// firstReplaceCoreArtifactName 目录内第一个匹配**核心中间态**命名
// `<base>-<纯数字>.pptx` 的文件名（无则空串）。大小写折叠与 isReplaceArtifactName
// 同口径（精确优先，仅 Windows 追加一次整名折叠，含扩展名）：核心同名覆盖写会丢失
// 用户内容，故 replace 核心调用前据此守卫。
func firstReplaceCoreArtifactName(dir, base string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isReplaceCoreArtifactName(e.Name(), base) {
			return e.Name()
		}
	}
	return ""
}

// isReplaceCoreArtifactName 核心中间态命名判定：`<base>-<纯数字>.pptx`。页序段
// 取最后一个 `-` 之后的纯数字（主干自身含 `-`/数字不受影响）。
//
// 精确匹配优先，**仅 Windows 下追加一次整名大小写折叠**（复用 isReplaceArtifactName
// 的折叠形态）：NTFS 保留大小写但不敏感，`守卫-009.PPTX` 与核心将要写出的
// `守卫-009.pptx` 是同一物理文件——按敏感比较会漏判并让核心静默覆盖用户内容。
// Linux/macOS 大小写敏感，折叠会把不同文件误判为同名（失效方向为误拒正常重拆），
// 故平台守卫不可移除。
func isReplaceCoreArtifactName(name, base string) bool {
	if isReplaceCoreArtifactNameExact(name, base) {
		return true
	}
	if runtime.GOOS != "windows" {
		return false
	}
	return isReplaceCoreArtifactNameExact(strings.ToLower(name), strings.ToLower(base))
}

// isReplaceCoreArtifactNameExact 大小写敏感的核心中间态形态判定（精确匹配）。
// 页序段要求**至少 2 位**纯数字（历史 `%02d` 与当前 `%03d` 都是 2 位以上）：核心
// 永不写 1 位页序，`方案-9.pptx` 这类用户文件不得被守卫误拦（R2）；`方案-09.pptx`
// / `方案-009.pptx` 仍拦截。
func isReplaceCoreArtifactNameExact(name, base string) bool {
	stem, ok := strings.CutSuffix(name, ".pptx")
	if !ok {
		return false
	}
	idx := strings.LastIndexByte(stem, '-')
	if idx <= 0 || idx >= len(stem)-1 {
		return false
	}
	seq := stem[idx+1:]
	if len(seq) < 2 || !isAllDigits(seq) {
		return false
	}
	return stem[:idx] == base
}

const replaceArtifactBackupSuffix = ".replace-bak"

// errSplitArtifactContentMismatch 备份让位复验不符（R3 TOCTOU 收口）：rename 后
// 备份文件内容与归属指纹不一致，调用方按 R4「内容与拆分记录不一致」文案拒绝覆盖。
var errSplitArtifactContentMismatch = errors.New("拆分产物内容与拆分记录不一致")

// replaceArtifactMismatchMessage 备份让位归属不符的失败文案（R3）：keptBackup 为空
// 表示已换回原路径（无备份残留），非空表示备份仍留在磁盘上，文案须给出绝对路径，
// 让用户人工确认后移走或还原（备份不登记进 backups，不会被自动删除）。
func replaceArtifactMismatchMessage(page int, name, keptBackup string) string {
	if keptBackup != "" {
		return fmt.Sprintf("第 %d 页目标文件「%s」内容与拆分记录不一致（可能已被修改），备份已保留在「%s」，请人工确认后移走或还原", page, name, keptBackup)
	}
	return fmt.Sprintf("第 %d 页目标文件「%s」内容与拆分记录不一致（可能已被修改），请确认后移走或还原", page, name)
}

// replaceArtifactBackupPath 历史产物让位备份的落点：同目录、原名加
// `.replace-bak` 后缀（不匹配 .pptx 产物形态，不会被回滚扫描或扫描索引认出）。
// 已存在（上次崩溃残留）时追加序号，避免覆盖未知内容。
func replaceArtifactBackupPath(dir, name string) string {
	p := filepath.Join(dir, name+replaceArtifactBackupSuffix)
	for n := 1; fileExists(p); n++ {
		p = filepath.Join(dir, fmt.Sprintf("%s%s.%d", name, replaceArtifactBackupSuffix, n))
	}
	return p
}

// backupReplaceArtifactForSurrender 历史产物「备份让位」单点（R3）：把旧产物挪到
// 备份名后**对备份文件复验归属指纹**，收口「校验与 rename 之间文件被外部修改」的
// TOCTOU 窗口。返回值：(bak, keptBackup, err)——
//   - 复验相符：bak = 备份路径（调用方登记、回滚可还原、成功前清理），keptBackup 空；
//   - rename 失败：三者均空/err；
//   - 复验不符且**换回原路径成功**：bak 与 keptBackup 均空，err 为内容不一致；
//   - 复验不符且**换回失败**：keptBackup = 仍留在磁盘上的备份路径（调用方在失败
//     文案里告知用户备份位置，备份不登记进 backups、不自动删除，保留人工恢复入口）。
func (s *AssetLibraryService) backupReplaceArtifactForSurrender(oldPath, outDir, oldName string, fp splitArtifactFingerprint) (bak, keptBackup string, err error) {
	bak = replaceArtifactBackupPath(outDir, oldName)
	if err := os.Rename(oldPath, bak); err != nil {
		return "", "", err
	}
	if fingerprintMatch(bak, fp) {
		return bak, "", nil
	}
	if rbErr := os.Rename(bak, oldPath); rbErr != nil {
		Logger().Warn("备份复验不符且换回原路径失败（备份保留待人工恢复）",
			"path", oldPath, "backup", bak, "err", rbErr)
		return "", bak, errSplitArtifactContentMismatch
	}
	return "", "", errSplitArtifactContentMismatch
}

// cleanupStaleReplaceArtifacts 本次新页之外的旧页收尾清理（删源前调用）：删除
// 文件成功才删索引行（删除失败先经注入点 s.removeArtifactFn / 生产实现
// removeSplitArtifactFile 重试一次，仍失败保留文件与索引）。返回「清理失败仍属本
// 记录历史产物」的凭证列表（v11 retained）：调用方写入本次 upsert，使后续增页
// 重拆仍能识别该文件归属（否则残留文件脱离 allow-list，被永久当作用户同名文件
// 拒绝）。成功删除或文件已不存在不保留（下标清理由 stale 列表联动完成）。
//
// 归属判定（v10）：经 fingerprintForName 单点只查本次拆分指纹（allow-list），
// `size + sha256` 全等才删；无归属条目（用户同形文件）或内容不符一律保留，任意
// mtime 均不删。指纹不可用时保持既有按名字删除行为（兼容 v10 前记录）。**retained
// 归属凭证不在此消费**——由 cleanupRetainedSplitArtifacts 独立单点处理，避免同一
// 文件在两处重复尝试与重复登记（R1）。
func (s *AssetLibraryService) cleanupStaleReplaceArtifacts(store *assetsStore, lib model.AssetLibrary, record *assetSplitRecord, outDir string, oldArtifacts map[string]struct{}, newFiles []string) []splitArtifactFingerprint {
	retainedOut := make([]splitArtifactFingerprint, 0)
	if len(oldArtifacts) == 0 {
		return retainedOut
	}
	newNames := make(map[string]struct{}, len(newFiles))
	for _, p := range newFiles {
		newNames[filepath.Base(p)] = struct{}{}
	}
	usable := fingerprintsUsable(record)
	remove := s.removeArtifactFn
	if remove == nil {
		remove = removeSplitArtifactFile
	}
	stale := make([]string, 0, len(oldArtifacts))
	for name := range oldArtifacts {
		// 大小写不敏感比对：Windows 上旧产物 `001-abc.pptx` 与本次新页
		// `001-Abc.pptx` 是同一物理文件，精确比对会把它当陈旧页删除（删掉的
		// 正是刚生成的新页）。
		if kept, keep := foldNameInSet(newNames, name); keep {
			if kept != name {
				// 仅大小写差异：物理文件已由本次新页覆盖（旧文件先备份再让位），
				// 旧索引行（路径大小写与磁盘实际名不同）须一并删除——保留它下次
				// 扫描按磁盘实际名匹配不上，会标成幽灵 lost 行且标注挂在不可见行
				// 上（2026-11-02）。只删行不删文件：Windows 上旧路径仍解析到新页。
				stale = append(stale, filepath.Join(outDir, name))
			}
			continue
		}
		p := filepath.Join(outDir, name)
		// 只查本次拆分指纹（v10 allow-list）；retained 归属凭证据由
		// cleanupRetainedSplitArtifacts 单点消费（R1：两处同查会对同一文件重复尝试、
		// 重复登记）。
		ownership, owns := fingerprintForName(record, name)
		switch {
		case owns:
			if !fingerprintMatch(p, ownership) {
				continue // 内容/大小不符：用户文件，保留
			}
		case usable:
			continue // 指纹 allow-list 语义：不在清单中 = 用户文件，保留
		}
		// 与 removeSplitArtifactsBeyond / cleanupSplitArtifactsInDirByPage 同口径：
		// 瞬时句柄占用经一次重试覆盖，仍失败保留文件与索引（不再承诺下次重拆收敛）。
		if rmErr := remove(p); rmErr != nil {
			Logger().Warn("删除历史拆分产物失败（重试一次后仍失败，保留文件与索引，归属凭证写入记录供后续重拆消费）",
				"libraryId", lib.ID, "path", p, "err", rmErr)
			if owns {
				retainedOut = append(retainedOut, ownership)
			}
			continue
		}
		stale = append(stale, p)
	}
	if len(stale) > 0 {
		s.deleteIndexRowsForPaths(store, lib, stale)
	}
	return retainedOut
}

// cleanupRetainedSplitArtifacts 消费记录携带的 retained 归属凭证（v11）：把「上次
// 清理失败未能删除、但仍属本记录历史产物」的文件在**任意 target 的成功拆分**后再
// 尝试删除一次，使切换落点（replace → source/cache 或反向）不再产生永久残留，也
// 不再让再切回 replace 被按名永久拒绝（R1）。
//
// 约束：
//   - 只处理 record.retained；dir 必须与 record.outputDir 折叠相等（samePathFold），
//     否则原样返回——凭证名是相对 output_dir 的 basename，跨目录消费会误删同形文件。
//   - 名字非法（splitFingerprintNameValid）、命中 excludeStem（本次新主干产物）、
//     内容与凭证不符（文件已被替换/移走）一律丢弃条目（该名字已不再描述本记录产物）。
//   - 文件不存在 → 条目丢弃（自愈）；删除成功 → 条目丢弃并联动索引行清理；删除失败
//     → 保留原条目，由调用方写入本次 upsert 继续随记录携带（不静默清空）。
//   - 删除经 s.removeArtifactFn（nil 回落生产实现 removeSplitArtifactFile），与三处
//     清理同口径。
//
// 调用点均在拆分成功收尾（source/cache 旧页清理之后、replace 删源前），失败/取消
// 路径不触达。
func (s *AssetLibraryService) cleanupRetainedSplitArtifacts(store *assetsStore, lib model.AssetLibrary, record *assetSplitRecord, dir, excludeStem string) []splitArtifactFingerprint {
	if record == nil || len(record.retained) == 0 || dir == "" {
		return nil
	}
	if !samePathFold(record.outputDir, dir) {
		return append([]splitArtifactFingerprint(nil), record.retained...)
	}
	remove := s.removeArtifactFn
	if remove == nil {
		remove = removeSplitArtifactFile
	}
	retainedOut := make([]splitArtifactFingerprint, 0, len(record.retained))
	removed := make([]string, 0, len(record.retained))
	for _, fp := range record.retained {
		if !splitFingerprintNameValid(fp.Name) {
			Logger().Warn("拆分保留凭证文件名非法（疑似路径穿越），丢弃条目",
				"libraryId", lib.ID, "dir", dir, "name", fp.Name)
			continue
		}
		if excludeStem != "" {
			if stem, _, parsed := parseSplitArtifactName(fp.Name, record.target); parsed && stem == excludeStem {
				continue // 该名字已是本次新产物，旧凭证不再描述任何归属
			}
		}
		path := filepath.Join(dir, fp.Name)
		if _, statErr := os.Lstat(path); statErr != nil {
			continue // 文件已不存在：条目丢弃（自愈）
		}
		if !fingerprintMatch(path, fp) {
			continue // 内容不符：文件已被替换/移走，条目丢弃
		}
		if rmErr := remove(path); rmErr != nil {
			Logger().Warn("删除保留归属的历史拆分产物失败（条目继续随记录携带）",
				"libraryId", lib.ID, "path", path, "err", rmErr)
			retainedOut = append(retainedOut, fp)
			continue
		}
		removed = append(removed, path)
	}
	if len(removed) > 0 {
		s.deleteIndexRowsForPaths(store, lib, removed)
	}
	return retainedOut
}

// removeReplaceArtifactBackups 删除历史产物备份（删源前调用，此后已无回滚场景；
// 回滚路径另行还原备份）。删除失败仅告警：残留备份不匹配产物形态，不参与索引。
func (s *AssetLibraryService) removeReplaceArtifactBackups(backups map[string]string) {
	for _, bak := range backups {
		if err := os.Remove(bak); err != nil && !os.IsNotExist(err) {
			Logger().Warn("删除历史拆分产物备份失败（残留不参与索引，可手动清理）", "path", bak, "err", err)
		}
	}
}

// restoreReplaceArtifactBackups 回滚还原被本次覆盖的历史产物：备份换回原位后
// 尽力重解析刷新索引行。为什么重解析：失败前 processScanFile 可能已把该行更新
// 为新内容，还原旧文件后行内 size/mtime/hash/幻灯片与磁盘不符；重解析让展示
// 立刻自洽，失败仅告警（下次增量扫描按 mtime 差异自愈）。换回失败时保留备份
// 文件（不删，供人工恢复），不阻断其余还原。
func (s *AssetLibraryService) restoreReplaceArtifactBackups(store *assetsStore, lib model.AssetLibrary, backups map[string]string) {
	for dst, bak := range backups {
		if err := os.Rename(bak, dst); err != nil {
			Logger().Warn("回滚还原历史拆分产物失败（备份保留待人工恢复）",
				"libraryId", lib.ID, "path", dst, "backup", bak, "err", err)
			continue
		}
		rel, relErr := filepath.Rel(lib.RootPath, dst)
		if relErr != nil {
			continue
		}
		relSlash := filepath.ToSlash(rel)
		rec, exists, recErr := store.fileRecordByPath(relSlash)
		if recErr != nil || !exists {
			continue
		}
		info, statErr := os.Stat(dst)
		if statErr != nil {
			continue
		}
		if _, err := s.processScanFile(store, lib, pptxDiskEntry{
			rel: relSlash, abs: dst, size: info.Size(), mtimeNs: info.ModTime().UnixNano(),
		}, rec, true); err != nil {
			Logger().Warn("回滚还原后重解析历史产物失败（下次扫描自愈，标注不受影响）",
				"libraryId", lib.ID, "path", relSlash, "err", err)
		}
	}
}

// filterReplaceArtifactSources 从批量源清单剔除上次就地替换生成的单页产物：
// 它们位于某条 target=replace 记录的 output_dir 之下且命名匹配产物形态，二次
// 批量会把「页」再拆成「页的页」（嵌套派生），并使源与产物失去一一对应。
// 比较在 Go 侧做：output_dir 存绝对路径，SQL 内拼前缀在跨平台分隔符下不可靠。
func filterReplaceArtifactSources(lib model.AssetLibrary, sources []assetSplitSource, outputs []replaceSplitOutput) []assetSplitSource {
	if len(outputs) == 0 {
		return sources
	}
	kept := sources[:0]
	for _, src := range sources {
		abs := filepath.Join(lib.RootPath, filepath.FromSlash(src.rel))
		if isReplaceArtifactOutput(abs, outputs) {
			continue
		}
		kept = append(kept, src)
	}
	return kept
}

// isReplaceArtifactOutput 绝对路径是否为上次就地替换的产物：父目录等于某条
// target=replace 记录的 output_dir，且文件名匹配该记录的产物命名形态（主干由
// 记录的源相对路径推导，与 splitSourceBaseName 同口径）。源文件经改名重链后
// 记录键主干（新名）与产物命名主干（旧名）不一致，追加目录独占前提下的宽松
// 形态判定 `<纯数字页序>-<任意主干>.pptx`——否则旧产物不再被剔除，批量二次拆分
// 会把「页」再拆成「页的页」。
func isReplaceArtifactOutput(absPath string, outputs []replaceSplitOutput) bool {
	clean := filepath.Clean(absPath)
	dir := filepath.Dir(clean)
	name := filepath.Base(clean)
	for _, o := range outputs {
		if !samePathFold(o.outputDir, dir) {
			continue
		}
		if isReplaceArtifactName(name, splitSourceBaseName(o.sourceRel)) || isReplaceArtifactNameAnyStem(name) {
			return true
		}
	}
	return false
}

// isReplaceArtifactNameAnyStem 已改名形态判定：`<纯数字页序>-<非空主干>.pptx`。
// 仅用于「目录由 replace 记录独占」的调用点（output_dir = <源目录>/<主干>，
// 用户自建文件混入概率低）；核心中间态 `<主干>-<数字>.pptx` 不在此列（需主干
// 精确匹配，防误把 `<文件名>-草稿.pptx` 当产物）。
func isReplaceArtifactNameAnyStem(name string) bool {
	stem, ok := strings.CutSuffix(name, ".pptx")
	if !ok {
		return false
	}
	idx := strings.IndexByte(stem, '-')
	if idx <= 0 || idx >= len(stem)-1 {
		return false
	}
	return isAllDigits(stem[:idx])
}

// resolveExistingDirCase 把目录路径归一为磁盘上的实际条目名大小写：父目录下与
// 路径基名大小写不敏感相等且唯一的目录条目即目标（Windows NTFS 保留创建时的大小写，
// 后续按不同大小写访问不会改写磁盘名）。用于重拆落点：源文件仅改大小写后由当前
// 文件名推导的落点大小写与磁盘既有目录不一致，索引相对路径会随之漂移。
//
// 读取失败、无命中或多命中（大小写敏感目录标志开启时 abc 与 Abc 可并存）一律返回
// 原值——不可判定时不冒险改选目录。
//
// 仅 Windows 有意义：Linux/macOS 大小写敏感，父目录下 `abc` 与 `Abc` 是两个不同
// 目录，折叠归一必定误选（P1）。非 Windows 直接原样返回，不读父目录。
func resolveExistingDirCase(dir string) string {
	if runtime.GOOS != "windows" {
		return dir
	}
	parent := filepath.Dir(dir)
	want := filepath.Base(dir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return dir
	}
	hit := ""
	for _, e := range entries {
		if !e.IsDir() || !sameNameFold(e.Name(), want) {
			continue
		}
		if hit != "" {
			return dir
		}
		hit = e.Name()
	}
	if hit == "" || hit == want {
		return dir
	}
	return filepath.Join(parent, hit)
}

// sameNameFold 文件名/目录名等价判定：精确相等，或（仅 Windows）大小写折叠相等。
// Windows 文件系统（NTFS）保留大小写但不敏感，两个大小写变体指向同一物理文件；
// Linux/macOS 默认大小写敏感，折叠会把不同文件误判为同一个（历史产物识别会因此
// 删除用户文件，P1 数据丢失）。与 samePathFold 同口径，文件名/路径判定禁裸用
// strings.EqualFold。
func sameNameFold(a, b string) bool {
	if a == b {
		return true
	}
	return runtime.GOOS == "windows" && strings.EqualFold(a, b)
}

// samePathFold 绝对路径等价判定：Clean 后走 sameNameFold（仅 Windows 折叠大小写）。
// 进入本判定的两侧均来自同一条库根路径；其余平台保持大小写敏感，避免 Linux 上
// 误判同名不同目录。
func samePathFold(a, b string) bool {
	return sameNameFold(filepath.Clean(a), filepath.Clean(b))
}

// splitOutputDir cache/source 两态落点解析（PRD Q4 落点契约，见文件头注释布局
// 说明）；replace 落点与 source 同布局但命名不同，在 splitOneFileReplace 内自行
// 计算（不走本函数，布局经 splitSourceOutputDir 单点复用）：
//   - 缓存目录（后端空串归一 cache；前端弹窗默认 replace）：derived 镜像源树，
//     目录段经 FromSlash 转平台分隔符；
//   - 源目录（outputToSourceDir=true）：<源所在目录>/<源名主干>/。
func (s *AssetLibraryService) splitOutputDir(lib model.AssetLibrary, rel, absPath string, outputToSourceDir bool) string {
	name := splitSourceBaseName(rel)
	if outputToSourceDir {
		return splitSourceOutputDir(absPath, rel)
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
