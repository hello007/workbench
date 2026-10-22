package main

// 分支对比域 App 薄委托（活动栏「分支对比」面板 batch 1，纯只读）。
// 跨层契约见 docs/spec/cross-layer-contracts.md；服务实现见 service/branch_compare.go。
//
// 方法一律不带 ctx：对齐 GetCommitHistory 等现有模式，内部 context.Background()
// 下传（service 层 ctx 全贯穿，为 batch 2 取消预留）。返回错误为 error（经 Wails
// ErrorFormatter 结构化传前端），前端按 E_GIT_TIMEOUT / E_GIT_CANCELLED / E_GIT_LOCKED
// 既有错误码分流（本任务零新错误码）。

import (
	"context"

	"workbench/model"
)

// GetBranchCompareAlignment 获取两分支的提交对齐结果（双列对齐表 + 统计条数据）：
// merge-base 分叉下界 + 双侧区间日志（各限 2000 条，触顶 Truncated 明示）+
// patch-id 批量管道（失败降级仅 hash/subject 匹配）+ 贪心三级配对（hash → patch-id → subject）。
func (a *App) GetBranchCompareAlignment(repoPath, branchA, branchB string) (*model.BranchCompareAlignment, error) {
	return a.branchCompareSvc.GetCommitAlignment(context.Background(), repoPath, branchA, branchB)
}

// GetBranchCompareFileTree 获取两分支差异文件树（两侧 ls-tree 并集，含 unchanged，
// 状态着色 + rename 新旧双路径 + 一趟 numstat 行数统计）。
func (a *App) GetBranchCompareFileTree(repoPath, branchA, branchB string) ([]model.BranchDiffFile, error) {
	return a.branchCompareSvc.GetBranchFileTree(context.Background(), repoPath, branchA, branchB)
}

// GetBranchCompareFileDiff 获取指定文件在 base→target 两分支间的 unified diff 文本
// （前端双栏/统一视图的原始输入，二进制文件为空串由前端占位提示）。
func (a *App) GetBranchCompareFileDiff(repoPath, base, target, filePath string) (string, error) {
	return a.branchCompareSvc.GetBranchFileDiff(context.Background(), repoPath, base, target, filePath)
}

// GetCommitChangedFiles 获取指定提交的变更文件列表（对齐表提交行详情弹层），
// 复用本地变更域 model.FileChange（Staged 恒 false）。
func (a *App) GetCommitChangedFiles(repoPath, sha string) ([]model.FileChange, error) {
	return a.branchCompareSvc.GetCommitChangedFiles(context.Background(), repoPath, sha)
}
