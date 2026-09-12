package main

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		consolePrint(fmt.Sprintf("WorkBench v%s (build %s)\n", version, buildTime))
		os.Exit(0)
	}

	app := NewApp()

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
