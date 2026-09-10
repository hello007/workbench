package main

import (
	"workbench/model"
	"workbench/util"
)

// ===== 工作目录域 =====

// GetDirectories 获取所有工作目录。
// 启动关键路径：直接返回 Load() 结果，不再同步检测 IsGitRepo（避免 N 次子进程阻塞 UI 渲染）。
// IsGitRepo 取自 directories.json 持久化值（Create/Update 时写入，RefreshDirectoriesGitFlag 启动后异步刷新）。
func (a *App) GetDirectories() []*model.Directory {
	directories, err := a.directorySvc.Load()
	if err != nil {
		println("Error:", err.Error())
		return []*model.Directory{}
	}
	return directories
}

// RefreshDirectoriesGitFlag 重新检测所有工作目录的 IsGitRepo，回写 directories.json，返回刷新后的列表。
// 启动后由前端异步调用，覆盖"目录后来纳管为 git 仓库"等变化。
// 关键：基于最新 Load 合并——只更新 IsGitRepo 字段，保留其他字段最新值，
// 规避"刷新期间用户 AddDirectory，刷新用旧快照 Save 覆盖新目录"的并发竞态。
func (a *App) RefreshDirectoriesGitFlag() []*model.Directory {
	// 1. 基于最新 Load（不使用任何旧快照）
	directories, err := a.directorySvc.Load()
	if err != nil {
		println("Error:", err.Error())
		return []*model.Directory{}
	}
	gitCmd := util.NewGitCommand()
	// 2. 只更新 IsGitRepo 字段（其他字段保留 Load 的最新值）
	for _, d := range directories {
		d.IsGitRepo = gitCmd.IsGitRepository(d.Path)
		if d.IsGitRepo {
			// 同步刷新 HasRemote，供前端灰色图标区分无远程仓库
			_, _, err := gitCmd.GetRemote(d.Path)
			d.HasRemote = err == nil
		}
	}
	// 3. Save 回写（基于最新 Load 的合并结果）
	if err := a.directorySvc.Save(directories); err != nil {
		println("Error:", err.Error())
	}
	return directories
}

// AddDirectory 添加工作目录。
// IsGitRepo 由 service.Create 在持久化时计算并写入 directories.json，此处直接返回。
func (a *App) AddDirectory(name, path string, isDefault bool) *model.Directory {
	dir, err := a.directorySvc.Create(name, path, isDefault)
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}

// UpdateDirectory 更新工作目录。
// IsGitRepo 由 service.Update 在持久化时重算并写入 directories.json，此处直接返回。
func (a *App) UpdateDirectory(id, name, path string, isDefault bool) *model.Directory {
	dir, err := a.directorySvc.Update(id, name, path, isDefault)
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}

// DeleteDirectory 删除工作目录
func (a *App) DeleteDirectory(id string) bool {
	err := a.directorySvc.Delete(id)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// SetDefaultDirectory 设置默认目录
func (a *App) SetDefaultDirectory(id string) bool {
	err := a.directorySvc.SetDefault(id)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}

// GetDefaultDirectory 获取默认目录。
// 读方法不触发检测，直接返回 Load 的持久化结果。
func (a *App) GetDefaultDirectory() *model.Directory {
	dir, err := a.directorySvc.GetDefault()
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}

// ReorderDirectories 重排工作目录顺序
func (a *App) ReorderDirectories(ids []string) bool {
	err := a.directorySvc.Reorder(ids)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}
