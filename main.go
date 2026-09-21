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

	listen := listenOverride
	if listen == "" {
		// Load 已补 webServe 段默认值（回环 127.0.0.1:36115，与历史 --serve
		// 无参默认行为一致）
		settings, _ := app.settingsSvc.Load()
		listen = settings.WebServe.BindAddress
	}

	app.webServe = newWebServeManager(app)
	if err := app.webServe.Start(listen); err != nil {
		return err
	}

	consolePrint(fmt.Sprintf("WorkBench serve 模式运行中: http://%s （访问令牌见 %s）\n", listen, serveTokenFile))
	slog.Info("serve mode listening", "addr", listen)

	// 阻塞至 HTTP 服务退出（Serve 错误或进程被杀）；ErrServerClosed 视为正常停机
	if err := app.webServe.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// assembleWebServe 装配浏览器访问通道全套组件（静态资产源 / 访问令牌 / WS hub /
// WebHandler），桌面同开与 --serve 无头共用，避免两份装配漂移。
//
// 首次调用生成随机访问令牌并持久化到 data/web_token（权限 0600），后续启动读取。
// 事件出口切换策略由调用方决定（webServeManager.applySink 统一为复合 sink：
// Wails 出口 + WS hub 广播）。
func assembleWebServe(app *App) (*server.WSHub, *server.WebHandler, error) {
	distFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return nil, nil, fmt.Errorf("定位前端静态资产失败: %w", err)
	}
	token, err := server.LoadOrCreateToken(serveTokenFile)
	if err != nil {
		return nil, nil, fmt.Errorf("准备访问令牌失败: %w", err)
	}
	hub := server.NewWSHub(token)
	handler := server.NewWebHandler(server.WebOptions{
		Assets:    distFS,
		Token:     token,
		RPCTarget: app,
		WSHub:     hub,
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
