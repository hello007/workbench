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
//     Failed++ 继续后续文件；单文件入口由调用方升格为错误）。
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

	// 3. 增量判定：记录命中 + 哈希一致 + 产物首页存在（双验，对齐缩略图补缺
	//    「PNG 存在唯一权威」先例——手删产物误判已拆被文件存在检查兜住）。产物
	//    存在性经 splitArtifactPath 判定（直拼 + 改名重链回退反查）：源文件被
	//    改名重链后旧产物仍能识别为「已拆未变」，避免重复拆分与孤儿产物。
	record, err := store.getSplitRecord(rel)
	if err != nil {
		return splitOutcome{rel: rel}, fmt.Errorf("读取拆分记录失败: %w", err)
	}
	if record != nil && record.sourceHash == hash {
		if _, artifactOK := splitArtifactPath(record, rel, 1); artifactOK {
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

	// 5.1 旧产物目录收尾清理（改名重链场景）：记录 output_dir 冻结在旧产物目录、
	//     产物主干仍是旧源名，按当前主干清理对新目录是空操作，旧页只能按记录页数
	//     在旧目录内逐页反查清理（复用 findSplitArtifactByPage 单点）。刻意放在
	//     核心成功之后——核心失败/取消时上一次拆分成果原样保留，对齐 replace 落点
	//     的延迟清理契约（「先删后拆」在失败时会连带丢掉上一次成果）。
	if outputToSourceDir && record != nil && !samePathFold(record.outputDir, outDir) {
		s.cleanupSplitArtifactsInDirByPage(store, lib, record, record.outputDir)
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
	if base == "" || base == "." || base == ".." {
		return failed("源文件名主干为空，无法确定拆分产物目录，请先重命名文件后重试")
	}
	// 产物落点 = 源文件所在目录下与源同名的文件夹（与 source 落点同布局）。
	// 核心原生输出恰为该目录下的 <base>-NNN.pptx，故直接传源路径拆。
	outDir := filepath.Join(filepath.Dir(absPath), base)

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
	} else if stray := firstReplaceArtifactName(outDir, base); stray != "" {
		return failed("目录中已存在与拆分产物同名的文件（%s），请先移走或重命名后重试", stray)
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
		dstName := filepath.Base(dst)
		if _, isOld := oldArtifacts[dstName]; isOld {
			// 历史产物：先挪到备份名再让新页占位。若直接覆盖，随后的失败回滚
			// 只能保住文件名却保不住旧内容（rename 不可逆）；备份使「失败/取消
			// 时旧产物原样保留」真正成立。
			bak := replaceArtifactBackupPath(outDir, dstName)
			if bakErr := os.Rename(dst, bak); bakErr != nil {
				return rollbackFailed("第 %d 页产物重命名失败（备份历史产物）: %v", page, bakErr)
			}
			backups[dst] = bak
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
	// 失败保留文件与索引（下次重拆收敛），不阻断删源。
	s.cleanupStaleReplaceArtifacts(store, lib, outDir, oldArtifacts, renamed)
	// 改名重链后旧产物目录（记录 output_dir）与新落点不同：旧页文件与索引行按
	// 记录页数逐页反查清理。刻意挂在成功收尾（删源前）——失败/取消路径不得删
	// 旧产物，与 oldArtifacts 延迟清理契约一致。不限定记录 target：output_dir 恒为
	// 本源文件上次产物的独占目录，落点切换（如 source→replace）同样须收敛旧产物。
	if record != nil && !samePathFold(record.outputDir, outDir) {
		s.cleanupSplitArtifactsInDirByPage(store, lib, record, record.outputDir)
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
			time.Now().Format("2006-01-02 15:04:05"), model.AssetSplitTargetReplace, false); recErr != nil {
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

// deleteIndexRowsForPaths 删除已清理产物在索引中的行（含缩略图记账与 PNG 文件）：
// 页数减少/重拆时越界旧页必须从索引消失，否则成为指向已删文件的僵尸素材。
// 路径按库根换算为相对路径查询；不存在行（未索引/已被删）静默跳过。
func (s *AssetLibraryService) deleteIndexRowsForPaths(store *assetsStore, lib model.AssetLibrary, paths []string) {
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

// splitArtifactPath 拆分单页产物路径解析单点（打开单页 / 导出 / 增量判定共用）：
// 先按记录键推导源名主干直拼（产物随源同名的常规场景）；直拼不存在时在
// record.outputDir 内按页序反查——源文件经 RelocateAssetFile 改名重链后，记录键
// 的主干（新名）与产物命名主干（旧名）不一致，而 split_records 未存主干列
// （不改表结构），只能以目录内命名形态回推。目录由该记录独占，且要求候选唯一
// （多候选视为不可判定，返回直拼路径供报错指向）。返回 (路径, 是否存在)。
func splitArtifactPath(record *assetSplitRecord, relPath string, slideNum int) (string, bool) {
	direct := splitArtifactPathForStem(record, splitSourceBaseName(relPath), slideNum)
	if fileExists(direct) {
		return direct, true
	}
	if alt, ok := findSplitArtifactByPage(record.outputDir, record.target, slideNum); ok {
		return alt, true
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

// findSplitArtifactByPage output_dir 内按页序反查唯一产物（改名重链回退路径）。
// 目录不存在/不可读一律视为未命中；同页序候选多于一个视为不可判定（用户在该
// 目录手工放入同页序文件时不猜），返回 false。
func findSplitArtifactByPage(outputDir, target string, slideNum int) (string, bool) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return "", false
	}
	var hit string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pptx") {
			continue
		}
		name := e.Name()
		matched := false
		if target == model.AssetSplitTargetReplace {
			// replace 形态：<纯数字页序>-<主干>.pptx（页序段位数不限，v8 前后兼容）
			if idx := strings.IndexByte(name, '-'); idx > 0 && idx < len(name)-len("-")-len(".pptx") &&
				isAllDigits(name[:idx]) {
				if n, convErr := strconv.Atoi(name[:idx]); convErr == nil && n == slideNum {
					matched = true
				}
			}
		} else if pageSeqFromArtifactName(name) == slideNum {
			matched = true
		}
		if !matched {
			continue
		}
		if hit != "" {
			return "", false // 多候选不可判定
		}
		hit = filepath.Join(outputDir, name)
	}
	return hit, hit != ""
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

// cleanupSplitArtifactsInDirByPage 按记录页数逐页反查并清理旧产物目录中的页产物
// （含其索引行与缩略图）。用于源文件改名重链后的重拆清旧：记录键已迁到新路径，
// 但 output_dir 冻结在旧产物目录、产物命名主干仍是旧源名，按当前主干清理必然漏删
// 旧页并留下 files 索引行（旧页还能再次进入批量拆分源）。
//
// 为什么按页序反查而不按主干推导：旧主干需要从记录反向还原（含 replace 的 -p
// 后缀与历史 %02d 形态），判定面大且极易误删；目录由该记录独占，按页序在
// output_dir 内反查唯一候选即可精确定位（复用 findSplitArtifactByPage 单点）。
// 同页序出现多个候选时视为不可判定——跳过该页（安全侧失败），不牵连其余页。
//
// 调用点均在拆分成功之后（源落点核心返回成功后、replace 删源前），失败与取消
// 路径不触达，上一次拆分成果原样保留。
//
// 删除成功才收集路径并批量删索引行；删除失败仅告警继续（残留由下次重拆收敛）。
// 目录删空后尝试删除（非空/失败忽略——目录内用户自建内容一律不动）。
func (s *AssetLibraryService) cleanupSplitArtifactsInDirByPage(store *assetsStore, lib model.AssetLibrary, record *assetSplitRecord, dir string) {
	if record == nil || dir == "" || record.pageCount <= 0 {
		return
	}
	removed := make([]string, 0, record.pageCount)
	for page := 1; page <= record.pageCount; page++ {
		p, ok := findSplitArtifactByPage(dir, record.target, page)
		if !ok {
			continue
		}
		if rmErr := os.Remove(p); rmErr != nil && !os.IsNotExist(rmErr) {
			Logger().Warn("删除改名重链前的旧拆分产物失败（下次重拆收敛）",
				"libraryId", lib.ID, "path", p, "err", rmErr)
			continue
		}
		removed = append(removed, p)
	}
	if len(removed) > 0 {
		s.deleteIndexRowsForPaths(store, lib, removed)
	}
	// 仅当目录已空才删除：非空 = 用户自建内容，保留且不算异常（不产生告警噪音）。
	if entries, readErr := os.ReadDir(dir); readErr == nil && len(entries) == 0 {
		if rmErr := os.Remove(dir); rmErr != nil && !os.IsNotExist(rmErr) {
			Logger().Warn("删除改名重链前的旧产物目录失败（保留）", "libraryId", lib.ID, "dir", dir, "err", rmErr)
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

// firstReplaceArtifactName 目录内第一个匹配产物命名形态的文件名（无则空串）。
// 用于「记录不属于本目录」时拒绝拆分：此刻目录内同形文件只能来自用户或历史
// 残留，覆盖/删除都可能丢用户数据，交回用户处置更安全。
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

const replaceArtifactBackupSuffix = ".replace-bak"

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

// cleanupStaleReplaceArtifacts 本次新页之外的旧页收尾清理（删源前调用）：删除
// 文件成功才删索引行（删除失败保留文件与索引，下次重拆收敛）。调用时机在全部
// 校验与标注迁移之后，失败/取消路径不会走到这里，旧页在此之前始终保留。
func (s *AssetLibraryService) cleanupStaleReplaceArtifacts(store *assetsStore, lib model.AssetLibrary, outDir string, oldArtifacts map[string]struct{}, newFiles []string) {
	if len(oldArtifacts) == 0 {
		return
	}
	newNames := make(map[string]struct{}, len(newFiles))
	for _, p := range newFiles {
		newNames[filepath.Base(p)] = struct{}{}
	}
	stale := make([]string, 0, len(oldArtifacts))
	for name := range oldArtifacts {
		if _, keep := newNames[name]; keep {
			continue
		}
		p := filepath.Join(outDir, name)
		if rmErr := os.Remove(p); rmErr != nil && !os.IsNotExist(rmErr) {
			Logger().Warn("删除历史拆分产物失败（保留文件与索引，下次重拆收敛）",
				"libraryId", lib.ID, "path", p, "err", rmErr)
			continue
		}
		stale = append(stale, p)
	}
	if len(stale) > 0 {
		s.deleteIndexRowsForPaths(store, lib, stale)
	}
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

// samePathFold 绝对路径等价判定：Clean 后等值即相同；Windows 文件系统大小写
// 不敏感，追加 EqualFold 兜底（进入本判定的两侧均来自同一条库根路径，其余平台
// 保持大小写敏感，避免 Linux 上误判同名不同目录）。
func samePathFold(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	return runtime.GOOS == "windows" && strings.EqualFold(a, b)
}

// splitOutputDir cache/source 两态落点解析（PRD Q4 落点契约，见文件头注释布局
// 说明）；replace 落点与 source 同布局但命名不同，在 splitOneFileReplace 内自行
// 计算（不走本函数）：
//   - 缓存目录（后端空串归一 cache；前端弹窗默认 replace）：derived 镜像源树，
//     目录段经 FromSlash 转平台分隔符；
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
