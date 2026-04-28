package main

import (
	"context"
	"path/filepath"

	"git-manager/model"
	"git-manager/service"
)

type App struct {
	ctx            context.Context
	directorySvc   *service.DirectoryService
	fileTreeSvc    *service.FileTreeService
	fileOpSvc      *service.FileOperationService
	gitSvc         *service.GitService
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir := "data"
	configPath := filepath.Join(dataDir, "directories.json")

	a.directorySvc = service.NewDirectoryService(configPath)
	a.fileTreeSvc = service.NewFileTreeService()
	a.fileOpSvc = service.NewFileOperationService()
	a.gitSvc = service.NewGitService()

	println("Git Manager started")
}

func (a *App) shutdown(context.Context) {
	println("Git Manager shutting down...")
}

// GetDirectories 获取所有工作目录
func (a *App) GetDirectories() []*model.Directory {
	directories, err := a.directorySvc.Load()
	if err != nil {
		println("Error:", err.Error())
		return []*model.Directory{}
	}
	return directories
}

// AddDirectory 添加工作目录
func (a *App) AddDirectory(name, path string, isDefault bool) *model.Directory {
	dir, err := a.directorySvc.Create(name, path, isDefault)
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}

// UpdateDirectory 更新工作目录
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

// GetDefaultDirectory 获取默认目录
func (a *App) GetDefaultDirectory() *model.Directory {
	dir, err := a.directorySvc.GetDefault()
	if err != nil {
		println("Error:", err.Error())
		return nil
	}
	return dir
}
