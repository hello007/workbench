package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"workbench/server"
	"workbench/service"
)

// webServeManager 浏览器访问通道管理器：桌面模式同开 HTTP 服务与 --serve 无头
// 模式共用的启动/停机/令牌热轮换入口。
//
// 生命周期：
//   - 桌面模式：startup 按设置创建并 Start（默认开启，失败仅告警降级——浏览器
//     访问为可选增强能力，端口占用等不影响桌面本体）；shutdown Stop 优雅停机。
//   - --serve 无头模式：runServe 创建并 Start，Wait 阻塞至服务退出。
//
// 运行期变更（桌面设置页）：SetWebServeConfig 改地址/开关经 Start/Stop 平滑
// 重启；RegenerateWebToken 经 RotateToken 热轮换（不断监听、不断在途连接）。
type webServeManager struct {
	app *App
	mu  sync.Mutex
	srv *http.Server
	ln  net.Listener
	// hub / handler 当前生效的事件广播 hub 与 HTTP handler（RotateToken 目标）
	hub     *server.WSHub
	handler *server.WebHandler
	listen  string
	// done 每次 Start 重建，承载 Serve goroutine 的退出错误（Wait 消费）
	done chan error
}

// newWebServeManager 构造管理器。app 供装配（RPC target 与事件出口切换）使用。
func newWebServeManager(app *App) *webServeManager {
	return &webServeManager{app: app}
}

// Start 启动（或按新地址重启）浏览器访问通道 HTTP 服务。
//
// 幂等语义：已在运行且地址未变时为 no-op；地址变化时先优雅停机再以新地址重启
// （hub/handler 全部重建，令牌重新从 data/web_token 读取——令牌轮换后经重启
// 或 RotateToken 均能取到最新值）。监听失败（端口占用等）同步返回错误，由调用
// 方决定降级（桌面告警）或上抛（--serve fatal）。
func (m *webServeManager) Start(listen string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srv != nil {
		if m.listen == listen {
			return nil
		}
		// 地址变化：优雅停旧实例后重启（停机错误不阻塞重启，服务即将被替换）
		_ = m.stopLocked()
	}
	return m.startLocked(listen)
}

// startLocked 以指定地址启动服务（调用方须持 mu）。
func (m *webServeManager) startLocked(listen string) error {
	hub, handler, err := assembleWebServe(m.app)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("监听 %s 失败（端口可能被占用）: %w", listen, err)
	}
	// 事件出口切换为复合 sink：Wails 桌面出口（窗口内事件）+ WS hub 广播（浏览器）。
	// Wails 出口对非 Wails 上下文（--serve 无头）静默跳过，等效仅 hub 广播。
	m.applySink(hub)
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	m.srv, m.ln, m.hub, m.handler, m.listen = srv, ln, hub, handler, listen
	m.done = make(chan error, 1)
	// done 以局部变量捕获：Serve goroutine 与 stopLocked 置 nil m.done 并发，
	// 直接读字段构成数据竞态（-race 实测），局部捕获后 goroutine 不再触碰 manager 字段
	done := m.done
	go func() {
		done <- srv.Serve(ln)
	}()
	return nil
}

// Stop 优雅停机：等待在途请求收尾（上限 3s），恢复 service 事件出口为纯
// Wails 出口。未运行时为 no-op。
func (m *webServeManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

// stopLocked 优雅停机（调用方须持 mu）。
func (m *webServeManager) stopLocked() error {
	if m.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := m.srv.Shutdown(ctx)
	// 出口恢复为纯 Wails（hub 随服务停用，避免事件投递到僵尸 hub）
	m.applySink(nil)
	m.srv, m.ln, m.hub, m.handler, m.listen, m.done = nil, nil, nil, nil, "", nil
	return err
}

// Wait 阻塞等待当前 Serve goroutine 退出并返回其错误；未运行时立即返回 nil。
func (m *webServeManager) Wait() error {
	m.mu.Lock()
	done := m.done
	m.mu.Unlock()
	if done == nil {
		return nil
	}
	return <-done
}

// RotateToken 令牌热轮换：更新 HTTP 认证与 WS 握手的校验基准，立即生效且不断开
// 监听。已建立的 WS 连接不在数据面校验令牌，保持有效至断开重连（重连须用新令牌）。
func (m *webServeManager) RotateToken(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hub != nil {
		m.hub.SetToken(token)
	}
	if m.handler != nil {
		m.handler.SetToken(token)
	}
}

// Running 返回 HTTP 服务当前是否运行中。
func (m *webServeManager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.srv != nil
}

// listenAddr 返回当前实际监听地址（127.0.0.1:0 动态端口场景取内核分配值）；
// 未运行时返回空串。
func (m *webServeManager) listenAddr() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ln == nil {
		return ""
	}
	return m.ln.Addr().String()
}

// applySink 将四个持事件出口的服务（terminal/update/ai_function/git）统一切换：
// sink 非 nil → 复合出口（Wails + hub 广播）；nil → 恢复纯 Wails 出口。
// SetEventSink 并发安全（sinkHolder 读写锁），输出泵等 goroutine 读侧已受保护。
func (m *webServeManager) applySink(hub *server.WSHub) {
	var sink service.EventSink
	if hub != nil {
		sink = &multicastSink{sinks: []service.EventSink{
			service.NewWailsEventSink(m.app.ctx),
			hub,
		}}
	} else {
		sink = service.NewWailsEventSink(m.app.ctx)
	}
	m.app.terminalSvc.SetEventSink(sink)
	m.app.updateSvc.SetEventSink(sink)
	m.app.aiFuncSvc.SetEventSink(sink)
	m.app.gitSvc.SetEventSink(sink)
}

// multicastSink 复合事件出口：同一条事件依次投递给多个 sink。
// 各 sink 的 nil / 非 Wails 上下文防护由其自身实现保证（wailsEventSink 对无
// events 键的 ctx 静默跳过，WSHub 无客户端时 Emit 为空投递）。
type multicastSink struct {
	sinks []service.EventSink
}

// Emit 依次投递事件给全部下游 sink。
func (m *multicastSink) Emit(name string, data ...any) {
	for _, s := range m.sinks {
		s.Emit(name, data...)
	}
}

// startWebServe 桌面模式按设置启动浏览器访问通道（默认开启）。
//
// 监听地址优先级：命令行 --listen（webServeListenOverride）> settings.json
// webServe.bindAddress（命令行覆盖配置文件）。启动失败（端口占用等）仅告警
// 降级不中断桌面启动：浏览器访问为可选增强能力，桌面本体不受影响，日志给出
// 可读提示供排查。
func (a *App) startWebServe() {
	cfg, err := a.settingsSvc.Load()
	if err != nil || cfg == nil || cfg.WebServe == nil || !cfg.WebServe.Enabled {
		if err != nil {
			slog.Warn("读取设置失败，浏览器访问通道未启动", "err", err)
		}
		return
	}
	listen := a.webServeListenOverride
	if listen == "" {
		listen = cfg.WebServe.BindAddress
	}
	a.webServe = newWebServeManager(a)
	if err := a.webServe.Start(listen); err != nil {
		slog.Warn("浏览器访问服务启动失败，桌面功能不受影响（可检查端口是否被占用或修改设置中的绑定地址）", "addr", listen, "err", err)
		return
	}
	slog.Info("web serve listening", "addr", listen)
}

// isLoopbackBind 判断监听地址的 host 部分是否为回环（仅本机可达）。
// 接受 127.0.0.1 / ::1 / localhost 三种形态。
func isLoopbackBind(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" || host == "::1" || host == "[::1]" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// webAccessUrls 计算候选访问地址：回环地址 + 本机全部非回环 IPv4（提示同网段
// 设备可达）。地址解析失败（缺端口）时返回 nil。
func webAccessUrls(bindAddress string) []string {
	_, port, err := net.SplitHostPort(bindAddress)
	if err != nil || port == "" {
		return nil
	}
	urls := []string{"http://127.0.0.1:" + port}
	ips, err := localIPv4Addresses()
	if err != nil {
		return urls
	}
	for _, ip := range ips {
		urls = append(urls, "http://"+ip+":"+port)
	}
	return urls
}

// localIPv4Addresses 枚举本机全部启用的非回环接口的 IPv4 地址。
// 枚举失败返回错误（调用方降级为仅回环地址）；无可用地址返回空切片。
func localIPv4Addresses() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips, nil
}
