package util

import (
	"io"
	"os"
	"path/filepath"

	"log/slog"

	"gopkg.in/natefinch/lumberjack.v2"
)

// 日志轮转参数：按大小轮转，保留 N 份，开启压缩。
// 桌面单用户日志量小，5MB/5 份上限约 25MB，足够覆盖崩溃排查窗口。
const (
	logMaxSizeMB    = 5
	logMaxBackups   = 5
	logMaxAgeDays   = 30
	logCompress     = true
	logFileName     = "app.log"
	logFilePerm os.FileMode = 0644
)

// InitLogger 初始化全局 slog 日志器，落盘到 logDir/app.log，按大小轮转。
//
// isDev 为 true 时（wails dev）额外输出到 stdout，便于开发期实时查看；
// 生产构建仅文件输出，避免 GUI 双击启动时 stdout 不可见导致日志丢失。
//
// 返回创建的 logDir 绝对路径，供调用方记录启动日志或打开日志目录。
func InitLogger(logDir string, isDev bool) (string, error) {
	absLogDir, err := filepath.Abs(logDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absLogDir, 0755); err != nil {
		return "", err
	}

	logPath := filepath.Join(absLogDir, logFileName)
	rotator := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    logMaxSizeMB,
		MaxBackups: logMaxBackups,
		MaxAge:     logMaxAgeDays,
		Compress:   logCompress,
		LocalTime:  true,
	}

	var writer io.Writer = rotator
	if isDev {
		// 开发期同时输出到文件与控制台，控制台走 AttachConsole（见 console_windows.go）
		writer = io.MultiWriter(rotator, os.Stdout)
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := slog.New(handler).With("app", "workbench")
	slog.SetDefault(logger)

	return absLogDir, nil
}
