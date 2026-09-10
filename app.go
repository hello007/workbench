package main

import (
	"context"
	"os"
	"path/filepath"

	"workbench/service"
)

// ===== 核心域：App 结构体与生命周期 =====

type App struct {
	ctx               context.Context
	directorySvc      *service.DirectoryService
	fileTreeSvc       *service.FileTreeService
	fileOpSvc         *service.FileOperationService
	gitSvc            *service.GitService
	settingsSvc       *service.SettingsService
	terminalSvc       *service.TerminalService
	searchSvc         *service.SearchService
	favoritesSvc      *service.FavoritesService
	contentSearchSvc  *service.ContentSearchService
	updateSvc         *service.UpdateService
	repoMetaSvc       *service.RepoMetaService
	aiFuncSvc         *service.AiFunctionService
	skillDiscoverySvc *service.SkillDiscoveryService
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir := "data"
	configPath := filepath.Join(dataDir, "directories.json")
	settingsPath := filepath.Join(dataDir, "settings.json")

	a.directorySvc = service.NewDirectoryService(configPath)
	a.fileTreeSvc = service.NewFileTreeService()
	a.fileOpSvc = service.NewFileOperationService()
	// 注入扫描缓存（.git 预筛 + mtime 缓存优化，PRD F12），让 ScanGitRepos 与一键更新同步受益
	a.gitSvc = service.NewGitServiceWithCache(filepath.Join(dataDir, "repo_scan_cache.json"))
	a.settingsSvc = service.NewSettingsService(settingsPath)
	a.terminalSvc = service.NewTerminalService(ctx)

	favoritesPath := filepath.Join(dataDir, "favorites.json")
	a.searchSvc = service.NewSearchService()
	a.favoritesSvc = service.NewFavoritesService(favoritesPath)
	a.contentSearchSvc = service.NewContentSearchService()

	// 仓库筛选器元数据服务（简述/标签持久化，PRD F10）
	a.repoMetaSvc = service.NewRepoMetaService(filepath.Join(dataDir, "repo_meta.json"))

	// AI 功能服务（工具箱「AI 功能」页：skill 聚合触发，data/ai_functions.json）
	a.aiFuncSvc = service.NewAiFunctionService(ctx, filepath.Join(dataDir, "ai_functions.json"))
	// 启动定时清理兜底：周期性清理未归档的运行期输出文件（归档接管已 os.Rename 移走不留残，此处只清异常残留）
	a.aiFuncSvc.StartHistoryCleanup()

	// skill 自动发现服务（配置对话框「导入 skill」入口，扫描用户级/工作目录/插件 skills）
	a.skillDiscoverySvc = service.NewSkillDiscoveryService(a.directorySvc)

	// 更新服务
	a.updateSvc = service.NewUpdateService()
	a.updateSvc.SetContext(ctx)

	// 检查是否有待应用的更新（上次下载但未重启）
	// 如果有待更新文件，会启动批处理脚本替换 exe 后启动新版本，
	// 当前旧进程需要退出，避免同时运行两个实例
	if hasPending, _ := a.updateSvc.CheckPendingUpdate(); hasPending {
		println("发现待更新文件，正在应用更新并退出...")
		os.Exit(0)
	}

	println("WorkBench started")
}

func (a *App) shutdown(context.Context) {
	if a.terminalSvc != nil {
		a.terminalSvc.CloseAll()
	}
	if a.aiFuncSvc != nil {
		a.aiFuncSvc.CloseAll()
	}
	println("WorkBench shutting down...")
}

// GetAppVersion 获取应用版本号
func (a *App) GetAppVersion() string {
	return version
}
