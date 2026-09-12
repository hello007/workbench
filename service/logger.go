package service

import "log/slog"

// service 包级日志器。
//
// 设计：package-level logger + SetLogger setter，由 app_services.go 的
// NewAppServices 在装配时调用 SetLogger 注入。未注入时 Logger() 返回
// slog.Default() 兜底（InitLogger 已设 slog.SetDefault），保证 service
// 单测与启动早期均有可用日志器，不 panic。
//
// 不改各 service 构造签名（NewXxxService 均不接受 logger 参数），避免
// 触动 76% 覆盖率基线与 14 个构造器调用点。详见
// docs/spec/app-services-assembly.md 装配范式。
var appLogger *slog.Logger

// SetLogger 注入 service 包级日志器，由 NewAppServices 装配时调用。
func SetLogger(l *slog.Logger) {
	appLogger = l
}

// Logger 返回 service 包级日志器。未注入时回退 slog.Default()，
// 保证任意调用路径（含单测）均有可用日志器。
func Logger() *slog.Logger {
	if appLogger == nil {
		return slog.Default()
	}
	return appLogger
}
