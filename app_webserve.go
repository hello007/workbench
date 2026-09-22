package main

import (
	"fmt"
	"log/slog"
	"net"
	"strings"

	"workbench/model"
	"workbench/server"
)

// ===== 浏览器访问域 =====
//
// 桌面设置页「网络访问」分区与浏览器通道运行期变更的绑定方法：
//   - GetWebServeConfig：配置 + 运行状态 + 候选访问地址（设置页展示）；
//   - SetWebServeConfig：持久化并使运行时生效（改地址/开关平滑重启服务）；
//   - GetWebServeToken / RegenerateWebToken：访问令牌查看与轮换。
//
// 令牌轮换热生效语义：更新 HTTP 认证与 WS 握手校验基准（不断监听、不断在途
// 连接）；已建立的 WS 连接不在数据面校验令牌，保持有效至断开重连，重连须用
// 新令牌（设置页重新生成确认文案已说明）。

// GetWebServeConfig 获取浏览器访问通道配置与运行状态。
// 配置段经 EnsureWebServeDefaults 补全默认值（段缺失 → 默认开启 + 回环地址）；
// AccessUrls 为候选访问地址（回环 + 本机局域网 IPv4，同网段设备可达）。
func (a *App) GetWebServeConfig() *model.WebServeConfig {
	settings := a.GetSettings()
	cfg := &model.WebServeConfig{
		Enabled:     settings.WebServe.Enabled,
		BindAddress: settings.WebServe.BindAddress,
		AccessUrls:  webAccessUrls(settings.WebServe.BindAddress),
	}
	if a.webServe != nil {
		cfg.Running = a.webServe.Running()
	}
	return cfg
}

// SetWebServeConfig 保存浏览器访问通道配置并使运行时生效。
//
// 生效语义：enabled=true 时若服务未运行则以新地址启动、地址变化则平滑重启
// （优雅停旧实例 → 新地址重启）；enabled=false 时停机（保留配置供下次开启）。
// 空地址回退默认回环地址；地址格式须为 host:port。改绑非回环地址不在此阻止
// （桌面设置页已弹风险确认，--serve 场景由 --listen 显式表达），仅记录告警日志。
func (a *App) SetWebServeConfig(enabled bool, bindAddress string) error {
	addr := strings.TrimSpace(bindAddress)
	if addr == "" {
		addr = model.DefaultWebServeBindAddress
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("绑定地址格式无效（须为 host:port，如 127.0.0.1:36115）: %w", err)
	}

	// 持久化（以磁盘现有设置为基底合并写，避免覆盖其他字段）
	settings := a.GetSettings()
	settings.WebServe = &model.WebServeSettings{Enabled: enabled, BindAddress: addr}
	if err := a.settingsSvc.Save(settings); err != nil {
		return fmt.Errorf("保存设置失败: %w", err)
	}

	// 运行时生效（webServe 为 nil 时仅落盘，下次启动按配置生效）：
	// 开启 → 启动/平滑重启到新地址；关闭 → 优雅停机
	if a.webServe != nil {
		if enabled {
			if err := a.webServe.Start(addr); err != nil {
				return fmt.Errorf("重启浏览器访问服务失败: %w", err)
			}
		} else if err := a.webServe.Stop(); err != nil {
			return fmt.Errorf("停止浏览器访问服务失败: %w", err)
		}
	}

	if enabled && !isLoopbackBind(addr) {
		// 非回环绑定：整机文件/终端/AI agent 能力向局域网暴露，记告警留痕
		slog.Warn("浏览器访问服务绑定到非回环地址，本机文件/终端/agent 能力将对局域网可达", "addr", addr)
	}
	return nil
}

// GetWebServeToken 读取当前访问令牌（首次调用时若令牌文件不存在则生成并持久化）。
func (a *App) GetWebServeToken() (string, error) {
	return server.LoadOrCreateToken(serveTokenFile)
}

// RegenerateWebToken 重新生成访问令牌：生成 → 持久化 → 运行中服务热轮换，
// 返回新令牌。全程持 webTokenMu 互斥——并发轮换（浏览器多标签页同时触发）
// 若不加锁， goroutine 1 落盘 token A 与 goroutine 2 落盘 token B 可交错于
// RotateToken 之前，最终磁盘与 handler/hub 内存基准不一致（旧令牌残留在某
// 一侧）。互斥后「落盘 + 热轮换」原子完成，最后一个完成的调用决定三处一致
// 的最终态。旧令牌对后续全部 HTTP 与 WS 握手请求立即失效；已建立的 WS 连接
// 保持有效至断开重连（见文件头注释的热生效语义）。
func (a *App) RegenerateWebToken() (string, error) {
	a.webTokenMu.Lock()
	defer a.webTokenMu.Unlock()

	token, err := server.GenerateToken()
	if err != nil {
		return "", fmt.Errorf("生成访问令牌失败: %w", err)
	}
	if err := server.SaveToken(serveTokenFile, token); err != nil {
		return "", fmt.Errorf("持久化访问令牌失败: %w", err)
	}
	if a.webServe != nil {
		a.webServe.RotateToken(token)
	}
	slog.Info("web serve token regenerated")
	return token, nil
}
