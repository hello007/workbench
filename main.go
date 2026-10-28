package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"workbench/model"
	"workbench/server"
	"workbench/service"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

//go:embed all:frontend/dist
var assets embed.FS

// serveTokenFile serve 模式访问令牌持久化路径（相对工作目录）。
var serveTokenFile = filepath.Join("data", server.DefaultTokenFile)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		consolePrint(fmt.Sprintf("WorkBench v%s (build %s)\n", version, buildTime))
		os.Exit(0)
	}

	// 手动解析 --serve / --listen（与既有 --version 的手动解析风格一致，
	// 规避 flag 包对未知参数报错退出的风险，保证桌面模式参数行为不变）
	serveMode := false
	// listenOverride 记录命令行 --listen 显式指定的监听地址；空串表示未指定，
	// 此时回退 settings.json webServe.bindAddress（Load 补默认回环地址）。
	// 优先级：--listen 命令行参数 > 配置文件（命令行覆盖配置文件）。
	listenOverride := ""
	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--serve":
			serveMode = true
		case strings.HasPrefix(arg, "--listen="):
			if v := strings.TrimPrefix(arg, "--listen="); v != "" {
				listenOverride = v
			}
		}
	}

	// --serve 无头模式：跳过 wails.Run（不创建桌面窗口），起 HTTP 服务阻塞运行
	if serveMode {
		if err := runServe(listenOverride); err != nil {
			slog.Error("serve mode exited with error", "err", err)
			log.Fatalf("serve 模式异常退出: %v", err)
		}
		return
	}

	app := NewApp()
	// 命令行 --listen 覆盖值传给桌面同开 HTTP 服务（空串 = 未指定，走配置文件）
	app.webServeListenOverride = listenOverride

	settingsSvc := service.NewSettingsService(filepath.Join("data", "settings.json"))
	settings, _ := settingsSvc.Load()

	err := wails.Run(&options.App{
		Title:  "WorkBench",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: server.PreviewHandler(),
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		// ErrorFormatter 将后端 error 结构化传前端：
		// AppError -> {code, message}，前端按 code 分流提示级别；
		// 普通 error -> {message}，前端走默认 error 提示。
		// 详见 docs/spec/logging-and-errors.md。
		ErrorFormatter: formatAppError,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewGpuIsDisabled: settings.GpuDisabled,
		},
	})

	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

// runServe 启动无头 HTTP 服务模式并阻塞运行。
//
// 与桌面模式差异：跳过 wails.Run（不创建窗口），自建 net/http 服务对外提供
// 前端静态资产（复用桌面模式同一份 embed 资产）、/api/rpc 通用 RPC 翻译层
// （149 个绑定方法）、/ws WebSocket 事件广播、/preview-pdf 与 /preview-raw
// 文件预览及健康检查。装配集中走 NewAppServices（logger 初始化与全部 service
// 纯构造，与桌面 startup 同构）；serve 模式无 Wails 上下文，传
// context.Background()，事件出口经 webServeManager 切换为复合 sink（wails
// 出口对非 Wails 上下文静默跳过，等效仅 WS hub 广播给浏览器）。
// startup 生命周期副作用（StartHistoryCleanup / CheckPendingUpdate 等）暂不
// 执行，属无头模式后续完善项。
//
// 监听地址：listenOverride 非空（命令行 --listen）优先；空串读 settings.json
// webServe.bindAddress（命令行覆盖配置文件）。
func runServe(listenOverride string) error {
	app := NewApp()
	// serve 模式无 Wails startup，ctx 兜底为 Background。注意：wails runtime 对
	// 缺 frontend 键的 ctx（含 Background）直接 log.Fatalf 退出进程，不能靠 ctx
	// 形态规避——对话框等 runtime 依赖方法须经 wailsRuntimeUnavailable 守卫前置
	// 拒绝（app.go）；service 层事件出口经下方 manager 切换为复合 sink。
	app.ctx = context.Background()
	app.AppServices = NewAppServices(context.Background(), "data", version == "dev")

	// 设置读取不再仅在 listen 未指定时执行：TLS 配置（开关 + 证书路径）恒取自
	// settings.json webServe 段（无命令行覆盖面，单一来源），须无条件读出
	settings, _ := app.settingsSvc.Load()
	listen := listenOverride
	if listen == "" {
		// Load 已补 webServe 段默认值（回环 127.0.0.1:36115，与历史 --serve
		// 无参默认行为一致）
		listen = settings.WebServe.BindAddress
	}

	app.webServe = newWebServeManager(app)
	tlsCfg := &WebServeTLSConfig{
		Enabled:  settings.WebServe.TLS,
		CertPath: settings.WebServe.TLSCertPath,
		KeyPath:  settings.WebServe.TLSKeyPath,
	}
	if err := app.webServe.Start(listen, tlsCfg); err != nil {
		return err
	}

	scheme := "http"
	if settings.WebServe.TLS {
		scheme = "https"
	}
	consolePrint(fmt.Sprintf("WorkBench serve 模式运行中: %s://%s （访问令牌见 %s）\n", scheme, listen, serveTokenFile))
	slog.Info("serve mode listening", "addr", listen)

	// 按代际阻塞至服务真正退出：浏览器经 SetWebServeConfig 触发的重启/停机由
	// Wait 内部代际核对消化（进程存活，继续服务新地址/等待重新开启）；仅 Serve
	// 因真实错误退出时才返回。ErrServerClosed 判断保留为防御兜底（正常路径下
	// 管理器触发的退出不会再透出该错误）。
	if err := app.webServe.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// assembleWebServe 装配浏览器访问通道全套组件（静态资产源 / 令牌校验 / WS hub /
// WebHandler），桌面同开与 --serve 无头共用，避免两份装配漂移。
//
// 令牌校验经 verify 闭包直读 WebTokenService（无状态快照）：新建/撤销/过期在
// 服务层即时生效，HTTP 与 WS 握手校验无需任何热轮换通知（多 token 语义下单
// token 时代 RotateToken 的「更新校验基准」职责由此消解）。存量迁移与初始
// 引导已在 NewAppServices 的 EnsureLoaded 完成启动期就绪。服务未装配（测试
// 零值构造等）时 verify 恒拒绝（fail closed）。认证事件经 OnAuth 回调落审计
// 日志（webAuditSvc 未装配时静默跳过）。
// 事件出口切换策略由调用方决定（webServeManager.applySink 统一为复合 sink：
// Wails 出口 + WS hub 广播）。
func assembleWebServe(app *App) (*server.WSHub, *server.WebHandler, error) {
	distFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return nil, nil, fmt.Errorf("定位前端静态资产失败: %w", err)
	}
	verify := func(candidate string) (string, bool) {
		if app.webTokenSvc == nil {
			return "", false
		}
		return app.webTokenSvc.Verify(candidate)
	}
	// 封禁判定闭包（连接管控四期）：verify 放行后的组合判定，直读服务层
	// BanRejectMessage（懒过期即时生效）。webTokenSvc 未装配时返回空串放行
	// ——此时 verify 已先行 fail closed 拒绝一切请求，封禁判定不可达，防御保留。
	banCheck := func(tokenID string) string {
		if app.webTokenSvc == nil {
			return ""
		}
		return app.webTokenSvc.BanRejectMessage(tokenID)
	}
	// 来源 IP 封禁判定闭包（连接管控五期）：verify 之前的前置判定，直读服务层
	// RejectMessage（入参 RemoteAddr 原型串，剥端口归一在服务层单点完成）。
	// ipBanSvc 未装配时返回空串放行（旁路构造的测试形态）。
	ipBanCheck := func(ip string) string {
		if app.ipBanSvc == nil {
			return ""
		}
		return app.ipBanSvc.RejectMessage(ip)
	}
	onAuth := func(ev server.AuthEvent) {
		if app.webAuditSvc != nil {
			app.webAuditSvc.Log(ev.Event, ev.TokenID, "", ev.RemoteAddr, ev.UserAgent)
		}
		// 认证失败自动封禁计数驱动（连接管控五期，与审计同闭包单点分流）：
		//   - auth_success 清零该来源 IP 的失败计数（WS 与 HTTP 合并计数——
		//     两认证点共用本回调与同一 IPBanService 实例，单例天然合并）；
		//   - auth_failure 且 TokenID 为空 = 普通认证失败（凭据无效），计 1；
		//   - auth_failure 且 TokenID 非空 = 四期封禁拒连形态（凭据有效、被
		//     tokenID 维度封禁拒绝），非认证失败不计——封禁期间的重复请求不
		//     累计，对齐「封禁中不累计」语义。
		// 回环地址（本机访问）由 IPBanService 内部豁免，此处不重复分流。
		if app.ipBanSvc != nil {
			switch ev.Event {
			case server.AuthEventSuccess:
				app.ipBanSvc.RecordSuccess(ev.RemoteAddr)
			case server.AuthEventFailure:
				if ev.TokenID == "" {
					app.ipBanSvc.RecordFailure(ev.RemoteAddr)
				}
			}
		}
	}
	hub := server.NewWSHub(verify, onAuth, banCheck, ipBanCheck)
	handler := server.NewWebHandler(server.WebOptions{
		Assets: distFS,
		Verify: verify,
		// 连接管控四期/五期：HTTP 与 WS 两认证点同接两类封禁判定（tokenID 维度
		// verify 放行后组合判定 + IP 维度 verify 之前前置判定，漏一即绕过）
		BanCheck:   banCheck,
		IPBanCheck: ipBanCheck,
		OnAuth:     onAuth,
		// 连接管控三期：per-request 装饰 target（RPC 调用者令牌 ID 注入踢出审计
		// actor 归因）+ 审计读侧对浏览器通道屏蔽（隐私红线，见 WebOptions 注释）
		RPCTarget:         app,
		RPCTargetForActor: app.rpcTargetForActor,
		// 隐私红线屏蔽面：QueryAuditLog（审计历史，含全谱 IP/令牌明细）、
		// ListIPBans（连接管控五期 IP 封禁列表，他人 IP 明细仅走桌面出口）与
		// ListIPBanSuggestions（连接管控九期网段聚合建议，派生自他人 IP 封禁
		// 条目，载荷仅网段串+计数仍整体收口——无 count 降级形态）
		RPCHiddenMethods: []string{"QueryAuditLog", "ListIPBans", "ListIPBanSuggestions"},
		WSHub:            hub,
	})
	return hub, handler, nil
}

// formatAppError 是 Wails ErrorFormatter 实现，将后端 error 转为前端可结构化解析的对象。
//
// 返回 map（序列化为 JSON 对象传前端）：
//   - AppError: {"code": "E_GIT_IN_PROGRESS", "message": "..."}
//   - 普通 error: {"message": "..."}
//
// 前端 error.js handleError 读 error.code 分流，无 code 走默认 error。
func formatAppError(err error) any {
	var appErr *model.AppError
	if errors.As(err, &appErr) {
		return map[string]string{
			"code":    appErr.Code,
			"message": appErr.Message,
		}
	}
	return map[string]string{
		"message": err.Error(),
	}
}
