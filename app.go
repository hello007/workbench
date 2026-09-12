package main

import (
	"context"
	"os"
)

// ===== 核心域：App 结构体与生命周期 =====

// App 内嵌 *AppServices，借助 Go 字段提升直接持有全部 service 字段
// (a.directorySvc 等)，使 133 个委托方法零 diff。装配见 NewAppServices。
type App struct {
	ctx context.Context
	*AppServices
}

// NewApp 构造空 App。AppServices 初始化为空 struct（非 nil），
// 保证 startup 前若有方法调用（如测试零值构造）访问 a.xxxSvc 字段提升
// 不 panic，各 service 仍为 nil 由方法内 nil 防护处理。
func NewApp() *App {
	return &App{AppServices: &AppServices{}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 集中装配全部 service（纯构造，副作用见下方）
	a.AppServices = NewAppServices(ctx, "data")

	// 启动定时清理兜底：周期性清理未归档的运行期输出文件（归档接管已 os.Rename 移走不留残，此处只清异常残留）
	a.aiFuncSvc.StartHistoryCleanup()

	// 更新服务 ctx 注入（NewUpdateService 无参构造，ctx 后置 setter）
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
