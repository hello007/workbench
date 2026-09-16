package main

import (
	"context"
	"path/filepath"

	"log/slog"

	"workbench/service"
	"workbench/util"
)

// AppServices 集中持有 App 的全部 service 与缓存实例。
//
// 设计：App 内嵌 *AppServices，借助 Go 字段提升使 138 个委托方法
// (a.directorySvc 等) 直接可达，装配集中化同时零委托方法 diff。
//
// 包归属：须定义在 package main。Go 跨包内嵌时未导出字段不提升——
// 若放 service 包且字段小写，main 包内 a.directorySvc 编译失败；
// 字段大写则 138 委托方法全改。放 main 包字段保持小写，提升同包可见。
//
// 字段顺序与原 App struct 一致，便于对照与排查。
type AppServices struct {
	directorySvc       *service.DirectoryService
	fileTreeSvc        *service.FileTreeService
	fileOpSvc          *service.FileOperationService
	gitSvc             *service.GitService
	commitHistoryCache *service.CommitHistoryCache // 提交历史全量快照缓存（纯内存，HEAD SHA 增量 + TTL + 手动刷新）
	settingsSvc        *service.SettingsService
	terminalSvc        *service.TerminalService
	searchSvc          *service.SearchService
	favoritesSvc       *service.FavoritesService
	contentSearchSvc   *service.ContentSearchService
	updateSvc          *service.UpdateService
	repoMetaSvc        *service.RepoMetaService
	aiFuncSvc          *service.AiFunctionService
	skillDiscoverySvc  *service.SkillDiscoveryService
	repoConfigSvc      *service.RepoConfigService
	// 会话快照服务（崩溃恢复 UI 状态，data/session.json）。复用 SettingsService 持久化模式，
	// Load 损坏降级冷启动不阻塞；前端 debounce 写 + shutdown 最终写。见 docs/spec/cross-layer-contracts.md。
	sessionSvc *service.SessionService
	// 全局状态看板服务（pin 仓库列表 + 跨仓状态快照，data/dashboard_pinned.json）。
	// 复用 RepoMetaService 持久化范式；状态计算只读不走仓级锁，批量并发复用 HasRemotesBatch 范式。
	dashboardSvc *service.DashboardService
}

// NewAppServices 集中装配 App 的全部 service 与缓存。
//
// 仅做纯构造，不执行启动期副作用（启 goroutine / setter 注入 / 退出判定）。
// 以下副作用保留在 App.startup，避免构造与生命周期耦合：
//   - aiFuncSvc.StartHistoryCleanup()：定时清理兜底 goroutine
//   - updateSvc.SetContext(ctx)：ctx setter 注入
//   - updateSvc.CheckPendingUpdate()：命中待更新则 os.Exit(0)
//
// 跨依赖：skillDiscoverySvc 依赖 directorySvc，构造顺序须 directorySvc 先。
//
// isDev 控制 logger 是否额外输出 stdout（wails dev 时 true，生产构建 false）。
func NewAppServices(ctx context.Context, dataDir string, isDev bool) *AppServices {
	s := &AppServices{}

	// 优先初始化全局日志器并注入 service 包，后续 service 构造与运行期日志均可落盘。
	// 落盘 data/logs/app.log，按 5MB 轮转保留 5 份；dev 模式额外 stdout。
	logDir := filepath.Join(dataDir, "logs")
	if absLogDir, err := util.InitLogger(logDir, isDev); err != nil {
		// 日志初始化失败不阻断启动，回退 slog 默认 stderr，启动后仍可运行
		slog.Error("init logger failed", "dir", logDir, "err", err)
	} else {
		service.SetLogger(slog.Default())
		slog.Info("logger initialized", "logDir", absLogDir, "dev", isDev)
	}

	// 工作目录配置（data/directories.json）
	configPath := filepath.Join(dataDir, "directories.json")
	s.directorySvc = service.NewDirectoryService(configPath)

	s.fileTreeSvc = service.NewFileTreeService()
	s.fileOpSvc = service.NewFileOperationService()

	// 注入扫描缓存（.git 预筛 + mtime 缓存优化，PRD F12），让 ScanGitRepos 与一键更新同步受益
	s.gitSvc = service.NewGitServiceWithCache(filepath.Join(dataDir, "repo_scan_cache.json"))
	// 注入提交历史缓存（纯内存，复用 filetree_cache 范式：HEAD SHA 增量 + TTL + 手动刷新）
	s.commitHistoryCache = service.NewCommitHistoryCache()

	// 设置面板配置（data/settings.json）
	settingsPath := filepath.Join(dataDir, "settings.json")
	s.settingsSvc = service.NewSettingsService(settingsPath)

	s.terminalSvc = service.NewTerminalService(ctx)

	// 收藏夹配置（data/favorites.json）
	favoritesPath := filepath.Join(dataDir, "favorites.json")
	s.searchSvc = service.NewSearchService()
	s.favoritesSvc = service.NewFavoritesService(favoritesPath)
	s.contentSearchSvc = service.NewContentSearchService()

	// 仓库筛选器元数据服务（简述/标签持久化，PRD F10）
	s.repoMetaSvc = service.NewRepoMetaService(filepath.Join(dataDir, "repo_meta.json"))

	// AI 功能服务（工具箱「AI 功能」页：skill 聚合触发，data/ai_functions.json）
	s.aiFuncSvc = service.NewAiFunctionService(ctx, filepath.Join(dataDir, "ai_functions.json"))

	// skill 自动发现服务（配置对话框「导入 skill」入口，扫描用户级/工作目录/插件 skills）
	// 跨依赖：依赖 directorySvc，须在其构造之后
	s.skillDiscoverySvc = service.NewSkillDiscoveryService(s.directorySvc)

	// 更新服务（检查更新与自动更新，ctx 由 startup 调 SetContext 注入）
	s.updateSvc = service.NewUpdateService()

	// 仓库列表配置导入导出服务（聚合工作目录 + 收藏夹两个数据源，跨依赖须在其后构造）
	s.repoConfigSvc = service.NewRepoConfigService(s.directorySvc, s.favoritesSvc)

	// 会话快照服务（崩溃恢复 UI 状态，独立 data/session.json 不与 settings.json 耦合）
	s.sessionSvc = service.NewSessionService(filepath.Join(dataDir, "session.json"))

	// 全局状态看板服务（pin 仓库列表 + 跨仓状态快照，独立 data/dashboard_pinned.json）
	s.dashboardSvc = service.NewDashboardService(filepath.Join(dataDir, "dashboard_pinned.json"))

	return s
}
