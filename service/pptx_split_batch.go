package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"workbench/model"
)

// 文件夹批量拆分（工具箱·PPT 拆分）：递归扫描目录下全部 .pptx 并串行逐个拆分。
//
// 与单文件 Split 共用取消注册表（键 = normalizeRepoKey(目录路径)，CancelPptxSplit
// 全量取消同时命中单文件与批量在途项）与事件出口；批量任务整体持一个可取消 ctx，
// 循环内逐文件调 provider.Split 继承该 ctx——取消命中即当前文件中止、剩余文件
// 不再开始、已完成文件保留。单文件失败记录原因后继续（不中断整批），终态经
// 返回值汇总（批量单次调用天然有返回值，不依赖 done 事件）。

// SplitFolder 递归扫描 dirPath 下全部 .pptx 并串行逐个拆分，返回汇总结果。
//
// 扫描：扩展名大小写不敏感（.PPTX 接受）；按 filepath.WalkDir 字典序稳定推进；
// 排除拆分产物（`<源文件名>/<源文件名>-<页序>.pptx` 同名文件夹布局）——复用素材
// 域 isPptxSplitDerivative 单点判定（位数无关，两位/三位历史产物通吃），防对同一
// 目录重复批量拆分时产物被再拆；目录不可入（权限/消失）降级跳过记 Warn 不中断。
//
// 校验：dirPath 非空、目录存在且确为目录，否则返回引导级错误；递归扫描为空
// （无可拆文件）同样报错。
//
// 进度：文件级 pptx-split:batch:progress（{dirPath, index, total, currentFile}，
// index 从 1 计，每文件开始前推送）+ 页级 pptx-split:progress（沿用单文件载荷，
// 单文件消费方不受影响）；批量终态仅经返回值汇总，不逐文件发 done/error 事件。
//
// 可取消（docs/spec/logging-and-errors.md 取消链路契约）：整批一个可取消 ctx 登记
// 取消注册表（defer LIFO：先注销后 cancel）；每个文件开始前与单文件错误返回后
// 均查整批 ctx——已取消即停止后续文件，经 mapCancelError 统一映射
// ErrOperationCancelled（E_GIT_CANCELLED，前端 warning 中性提示）上抛，Items 保留
// 已完成文件；单文件业务失败（非取消形态）经 appErrMessage 转用户可读消息记入
// Item.Error（日志保留根因链）后继续下一个。
func (s *PptxSplitService) SplitFolder(ctx context.Context, dirPath string) (model.PptxBatchResult, error) {
	if strings.TrimSpace(dirPath) == "" {
		return model.PptxBatchResult{}, errors.New("请先选择要拆分的文件夹")
	}
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return model.PptxBatchResult{}, fmt.Errorf("解析文件夹路径失败（%s）: %w", dirPath, err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return model.PptxBatchResult{}, fmt.Errorf("文件夹不存在或不可访问（%s）: %w", dirPath, err)
	}
	if !info.IsDir() {
		return model.PptxBatchResult{}, fmt.Errorf("路径不是文件夹（%s）", dirPath)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	entry := s.registerCancel(dirPath, cancel)
	defer s.unregisterCancel(dirPath, entry)

	files, err := scanPptxFilesForSplit(absDir)
	if err != nil {
		return model.PptxBatchResult{}, err
	}
	if len(files) == 0 {
		return model.PptxBatchResult{}, fmt.Errorf("目录下未找到 PPT 文件（%s）", absDir)
	}

	result := model.PptxBatchResult{DirPath: absDir, Total: len(files), Items: make([]model.PptxBatchItem, 0, len(files))}
	for i, src := range files {
		// 每个文件开始前先查整批 ctx：已取消即剩余文件不再开始（Items 保留已完成）
		if err := ctx.Err(); err != nil {
			return result, mapCancelError(err)
		}
		s.emitCurrent("pptx-split:batch:progress", map[string]any{
			"dirPath": absDir, "index": i + 1, "total": len(files), "currentFile": src,
		})

		item := model.PptxBatchItem{SrcPath: src}
		splitResult, err := s.provider.Split(ctx, src, func(done, total int) {
			s.emitCurrent("pptx-split:progress", map[string]any{"srcPath": src, "done": done, "total": total})
		})
		if err != nil {
			// 取消形态（当前文件被中止）不算单文件业务失败：整批按取消收口，
			// 剩余文件不再开始，已完成 Items 保留
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				if !errors.Is(err, context.Canceled) {
					err = ctx.Err()
				}
				return result, mapCancelError(err)
			}
			// 单文件失败不中断：记用户可读原因后继续下一个
			Logger().Error("pptx batch split item failed", "srcPath", src, "err", err)
			item.Error = appErrMessage(err)
		} else {
			item.OutputDir = splitResult.OutputDir
			item.PageCount = splitResult.PageCount
		}
		result.Items = append(result.Items, item)
	}

	succeeded, failed := 0, 0
	for _, item := range result.Items {
		if item.Error != "" {
			failed++
		} else {
			succeeded++
		}
	}
	Logger().Info("pptx batch split done", "dirPath", absDir, "total", len(files), "succeeded", succeeded, "failed", failed)
	return result, nil
}

// scanPptxFilesForSplit 递归收集 root 下全部待拆分 .pptx 绝对路径（WalkDir
// 字典序，串行拆分顺序可预期）。拆分产物经 isPptxSplitDerivative 单点排除；
// 目录不可入（权限/消失）降级跳过记 Warn 不中断扫描。
func scanPptxFilesForSplit(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// 目录不可入（权限/消失等）：剪除该枝继续扫其余目录，不中断批量
			Logger().Warn("pptx batch scan entry inaccessible", "path", p, "err", err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(p), ".pptx") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("计算相对路径失败（%s）: %w", p, err)
		}
		// isPptxSplitDerivative 以正斜杠 rel 路径为入参（path.Base/Dir 解析），
		// Windows 下 filepath.Rel 产反斜杠分隔须先归一
		if isPptxSplitDerivative(filepath.ToSlash(rel)) {
			return nil // 拆分产物不再拆（重复批量拆分防护）
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描目录失败（%s）: %w", root, err)
	}
	return files, nil
}
