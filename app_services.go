package main

import (
	"context"
	"path/filepath"

	"log/slog"

	"workbench/model"
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
	// numstatLineCache 行数统计缓存（`git log --numstat` 按作者行数，纯内存，
	// headSHA+TTL 二态失效无增量路径）。键同 commitHistoryCacheKey，手动刷新与
	// 提交历史缓存同步清除。
	numstatLineCache  *service.CommitLineStatsCache
	settingsSvc       *service.SettingsService
	terminalSvc       *service.TerminalService
	searchSvc         *service.SearchService
	favoritesSvc      *service.FavoritesService
	contentSearchSvc  *service.ContentSearchService
	updateSvc         *service.UpdateService
	repoMetaSvc       *service.RepoMetaService
	aiFuncSvc         *service.AiFunctionService
	skillDiscoverySvc *service.SkillDiscoveryService
	repoConfigSvc     *service.RepoConfigService
	// 会话快照服务（崩溃恢复 UI 状态，data/session.json）。复用 SettingsService 持久化模式，
	// Load 损坏降级冷启动不阻塞；前端 debounce 写 + shutdown 最终写。见 docs/spec/cross-layer-contracts.md。
	sessionSvc *service.SessionService
	// 全局状态看板服务（pin 仓库列表 + 跨仓状态快照，data/dashboard_pinned.json）。
	// 复用 RepoMetaService 持久化范式；状态计算只读不走仓级锁，批量并发复用 HasRemotesBatch 范式。
	dashboardSvc *service.DashboardService
	// AI 对话服务（AI 对话工作台「AI 对话」页：多会话持续式对话，data/ai_chat/）。
	// 事件推送复用 sinkHolder 范式（chat-task:* 独立前缀），子进程执行经
	// processFactory 抽象（测试注入 fake 不真调 claude CLI）。
	chatSvc *service.ChatService
	// 进程管理服务（工具箱「进程管理」：端口/进程名检索 + 停止/强杀编排）。
	// 枚举与终止经 ProcessProvider 抽象委托 util 平台分叉实现（Windows netstat/tasklist/taskkill，
	// Linux /proc + 信号），测试注入 fake 不依赖真实进程。
	processSvc *service.ProcessService
	// 环境变量查看服务（工具箱「环境变量」：多来源只读枚举 + 脱敏标记 + 组内排序）。
	// 来源枚举经 EnvSourceProvider 抽象委托 util 平台分叉实现（Windows 读注册表区分
	// 系统/用户级 + os.Environ 进程级；Linux 仅进程级），测试注入 fake 不依赖本机
	// 真实环境变量与注册表内容。
	envSvc *service.EnvService
	// hosts 文件查看服务（工具箱「hosts 文件」：本机 hosts 只读读取 + 行解析）。
	// 路径取 util.HostsFilePath 平台常量（Windows drivers/etc/hosts / Linux /etc/hosts），
	// service 测试注入 fixture 路径不依赖本机真实 hosts 内容。
	hostsSvc *service.HostsService
	// PPT 按页拆分服务（工具箱「PPT 拆分」：.pptx 按页拆分为单页文件，无持久化数据）。
	// 拆分经 PptxSplitProvider 抽象（生产实现为纯 Go OOXML 标准库核心，不依赖本机
	// Office），测试注入 fake 验证编排透传；进度/终态事件走 sinkHolder（pptx-split:*
	// 前缀，web_serve.go applySink 须同步切换），可取消（注册表键 = 源文件规范化路径）。
	pptxSplitSvc *service.PptxSplitService
	// 素材管理服务（侧边栏「素材管理」：本地 DAM 素材库 + PPT 单页索引，
	// data/assets/）。注册表 libraries.json 走 util.SaveJSON 原子写；各库 SQLite
	// 索引（modernc.org/sqlite 纯 Go 零 CGO）懒打开缓存；目录扫描进度/终态事件走
	// sinkHolder（assets-scan:* 前缀，web_serve.go applySink 须同步切换），可取消
	// （注册表键 = 库 ID 规范化）；解析为纯 Go OOXML（复用拆分核心读取链路）。
	assetsSvc *service.AssetLibraryService
	// 保存的素材检索条件服务（SRS §2.5 素材管理第七批：命名/调取/删除/覆盖，
	// data/asset_saved_searches.json 全局存储走 util.SaveJSON 原子写；损坏降级
	// 空列表 + slog.Warn，对齐 SettingsService 模式）。
	assetSavedSearchSvc *service.AssetSavedSearchService
	// 多源页合并导出服务（素材购物栏「合并导出」的合并编排：基底注入式合并编排 +
	// 取消注册表 + assets-cart:* 事件出口 sinkHolder——web_serve.go applySink 须
	// 同步切换）。核心为纯 Go OOXML 标准库实现（MergePptxPages，不依赖本机 Office）。
	pptxMergeSvc *service.PptxMergeService
	// 素材购物栏服务（素材管理「购物栏」：跨库页级收集 + 拖拽排序 + 按序合并导出，
	// data/asset_cart.json 走 util.SaveJSON 原子写；损坏降级空购物栏 + slog.Warn）。
	// 合并导出复用 pptxMergeSvc（构造注入），导出互斥 E_CART_EXPORT_IN_PROGRESS，
	// 取消转发其取消注册表。
	assetCartSvc *service.AssetCartService
	// PPTX 预览转换编排服务（文件树 pptx/ppsx 内嵌预览：COM 转 PDF + pdfjs viewer
	// 复用链路，data/pptx_preview_pdf/ 缓存索引 + LRU + 在途防抖）。COM 执行器
	// 复用 assetsSvc 缩略图同一 STA worker 单队列（PptxPdfConverter 注入，避免
	// 双 COM 队列进程互杀）；不发生前端事件，web_serve.go applySink 无需同步。
	pptxPreviewSvc *service.PptxPreviewService
	// 认证审计日志器（浏览器访问通道：认证成功/失败 + token 生命周期事件，
	// JSONL append-only data/web_audit.log，5MB 轮转 .1；append 语义独立实现，
	// util.SaveJSON 原子写不适用 append-only 日志）。server 认证回调（OnAuth）
	// 与 webTokenSvc 生命周期事件都经它落盘。
	webAuditSvc *service.AuditLogger
	// 浏览器访问通道多 token 管理服务（GitHub PAT 式：哈希落盘/命名/撤销/可选
	// 过期，data/web_tokens.json util.SaveJSON 原子写；存量 data/web_token 首载
	// 迁移为首条记录）。Verify 作为 serve 认证链校验函数注入 server.WebOptions
	// 与 WSHub（无状态快照，新建/撤销/过期即时生效，无需热轮换通知）。
	webTokenSvc *service.WebTokenService
	// 来源 IP 自动封禁服务（连接管控五期：认证失败滑动窗口达阈值自动临时封禁，
	// 纯内存态不落盘，重启清零）。RejectMessage 闭包注入 server.WebOptions 与
	// WSHub 的 IPBanCheck（两认证点 verify 之前前置判定）；认证计数经 OnAuth
	// 审计闭包驱动（main.go assembleWebServe）；审计出口与 webAuditSvc 共用。
	ipBanSvc *service.IPBanService
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
	// 注入事件出口（批量拉取 pull-progress/pull-complete 推送；对齐 updateSvc 的 setter 注入模式）
	s.gitSvc.SetContext(ctx)
	// 注入提交历史缓存（纯内存，复用 filetree_cache 范式：HEAD SHA 增量 + TTL + 手动刷新）
	s.commitHistoryCache = service.NewCommitHistoryCache()
	// 注入行数统计缓存（numstat 通道，headSHA+TTL 二态失效，键同提交历史缓存）
	s.numstatLineCache = service.NewCommitLineStatsCache()

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

	// AI 对话服务（AI 对话工作台「AI 对话」页：会话索引 + 每会话消息文件，
	// 独立 data/ai_chat/ 目录不与其他 service 数据交叉）
	s.chatSvc = service.NewChatService(ctx, filepath.Join(dataDir, "ai_chat"))

	// 进程管理服务（工具箱「进程管理」，无持久化数据）
	s.processSvc = service.NewProcessService()

	// 环境变量查看服务（工具箱「环境变量」，无持久化数据）
	s.envSvc = service.NewEnvService()

	// hosts 文件查看服务（工具箱「hosts 文件」，无持久化数据）
	s.hostsSvc = service.NewHostsService()

	// PPT 按页拆分服务（工具箱「PPT 拆分」，无持久化数据；ctx 构造注入 Wails 桌面事件出口）
	s.pptxSplitSvc = service.NewPptxSplitService(ctx)

	// 素材管理服务（侧边栏「素材管理」，data/assets/；ctx 构造注入 Wails 桌面事件出口）
	s.assetsSvc = service.NewAssetLibraryService(ctx, filepath.Join(dataDir, "assets"))
	// 素材缩略图设置读取口（D2 五项）：读 settingsSvc 内存缓存（Load 带缓存，
	// 热路径画廊逐卡片消费不落盘）。
	s.assetsSvc.SetThumbsSettingsProvider(func() model.AssetThumbsSettings {
		cfg, err := s.settingsSvc.Load()
		if err != nil || cfg == nil {
			return model.DefaultAssetThumbsSettings()
		}
		return *cfg.Assets // Load 已 Ensure，段非 nil
	})

	// 保存的素材检索条件服务（SRS §2.5，独立 data/asset_saved_searches.json
	// 不与素材库注册表数据交叉）
	s.assetSavedSearchSvc = service.NewAssetSavedSearchService(filepath.Join(dataDir, "asset_saved_searches.json"))

	// 多源页合并导出服务（素材购物栏「合并导出」；ctx 构造注入 Wails 桌面事件出口）
	s.pptxMergeSvc = service.NewPptxMergeService(ctx)
	// 素材购物栏服务（素材管理「购物栏」，独立 data/asset_cart.json 不与素材库
	// 数据交叉；合并导出复用 pptxMergeSvc）
	s.assetCartSvc = service.NewAssetCartService(filepath.Join(dataDir, "asset_cart.json"), s.pptxMergeSvc)

	// PPTX 预览转换编排服务（文件树 pptx/ppsx 内嵌预览，data/pptx_preview_pdf/
	// 缓存目录；COM 执行器 = assets 缩略图同一 STA worker 单队列导出器）
	s.pptxPreviewSvc = service.NewPptxPreviewService(filepath.Join(dataDir, "pptx_preview_pdf"), s.assetsSvc.PptxPdfConverter())

	// 认证审计日志器 + 多 token 管理服务（浏览器访问通道）。EnsureLoaded 即时
	// 完成存量迁移/初始引导：data/web_token 明文兼容文件在启动期就绪——serve
	// 控制台提示与设置页「查看令牌」链路依赖该文件，不能等首个认证请求懒加载
	//（首请求 401 后才生成的令牌用户无从得知）。
	s.webAuditSvc = service.NewAuditLogger(filepath.Join(dataDir, "web_audit.log"))
	s.webTokenSvc = service.NewWebTokenService(filepath.Join(dataDir, "web_tokens.json"), filepath.Join(dataDir, "web_token"))
	s.webTokenSvc.SetAudit(s.webAuditSvc)
	s.webTokenSvc.EnsureLoaded()

	// 来源 IP 封禁服务（连接管控五期自动封禁 + 七期手动封禁）：审计出口与
	// token 封禁同用 webAuditSvc；生产默认时钟（time.Now），RejectMessage 闭包
	// 由 assembleWebServe 注入两认证点。SetBansPath 注入手动封禁持久化文件
	// （data/ip_bans.json，七期；须在首个 Ban/UnbanIP 之前完成，构造后立即注入
	// 即时载入存量条目）。启动初值经 ApplyConfig 注入设置段归一值（连接管控
	// 六期参数化：Load 首调在此完成缓存建立，后续热更走 SetIPBanConfig 单点，
	// 两通道同一 ApplyConfig 收口）。
	s.ipBanSvc = service.NewIPBanService(nil)
	s.ipBanSvc.SetAudit(s.webAuditSvc)
	s.ipBanSvc.SetBansPath(filepath.Join(dataDir, "ip_bans.json"))
	if cfg, err := s.settingsSvc.Load(); err == nil {
		s.ipBanSvc.ApplyConfig(model.NormalizeIPBanSettings(cfg.IPBan))
	}

	return s
}
