package main

import (
	"workbench/model"
)

// ===== 文件树与文件操作域 =====

// GetFileTree 获取文件树
func (a *App) GetFileTree(path string) []*model.FileTreeNode {
	nodes, err := a.fileTreeSvc.GetChildren(path)
	if err != nil {
		println("Error:", err.Error())
		return []*model.FileTreeNode{}
	}
	return nodes
}

// GetFileTreeRecursive 获取完整树
func (a *App) GetFileTreeRecursive(path string, maxDepth int) []*model.FileTreeNode {
	nodes, err := a.fileTreeSvc.GetTree(path, maxDepth)
	if err != nil {
		println("Error:", err.Error())
		return []*model.FileTreeNode{}
	}
	return nodes
}

// RefreshFileTree 清除指定路径的文件树缓存并返回最新子节点。
// 供前端 refreshNode（右键刷新/F5/文件操作后）绕过缓存强刷单点目录。
func (a *App) RefreshFileTree(path string) []*model.FileTreeNode {
	nodes, err := a.fileTreeSvc.RefreshChildren(path)
	if err != nil {
		println("Error:", err.Error())
		return []*model.FileTreeNode{}
	}
	return nodes
}

// ClearAllFileTreeCache 清除全部文件树缓存。
// 供前端 refreshAll（工具栏"刷新"按钮，el-tree 整体重建）前置调用，确保逐节点重拉拿到最新数据。
func (a *App) ClearAllFileTreeCache() {
	a.fileTreeSvc.ClearAllCache()
}

// CreateDirectory 创建文件夹
func (a *App) CreateDirectory(parentPath, name string) bool {
	err := a.fileOpSvc.CreateDirectory(parentPath, name)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// CreateFile 创建文件
func (a *App) CreateFile(parentPath, name, content string) bool {
	err := a.fileOpSvc.CreateFile(parentPath, name, content)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// RenameFile 重命名
func (a *App) RenameFile(oldPath, newName string) bool {
	err := a.fileOpSvc.Rename(oldPath, newName)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// DeleteFile 删除
func (a *App) DeleteFile(path string) bool {
	err := a.fileOpSvc.Delete(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}
