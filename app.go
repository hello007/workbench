package main

import (
	"context"
	"path/filepath"

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
