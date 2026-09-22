package main

import (
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
//     访问为可选增强能力，端口占用等不影响桌面本体）；shutdown Stop 立即断开停机。
//   - --serve 无头模式：runServe 创建并 Start，Wait 按代际阻塞至服务真正退出
//     （浏览器侧经 SetWebServeConfig 重启/停机不终止进程）。
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
	// done 每次 Start 重建，承载 Serve goroutine 的退出错误（Wait 按代际消费）
	done chan error
	// cond 联动 mu，供 Wait 在 done 为 nil（服务未运行）时阻塞等待下一次 Start
	cond *sync.Cond
}

// newWebServeManager 构造管理器。app 供装配（RPC target 与事件出口切换）使用。
func newWebServeManager(app *App) *webServeManager {
	m := &webServeManager{app: app}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Start 启动（或按新地址重启）浏览器访问通道 HTTP 服务。
//
// 幂等语义：已在运行且地址未变时为 no-op；地址变化时先停旧实例（立即断开，
// 见 stopLocked）再以新地址重启（hub/handler 全部重建，令牌重新从 data/web_token
// 读取——令牌轮换后经重启或 RotateToken 均能取到最新值）。监听失败（端口占用
// 等）同步返回错误，由调用方决定降级（桌面告警）或上抛（--serve fatal）。
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
	// 唤醒 Wait 中因服务未运行（done 为 nil）而阻塞等待的 goroutine（--serve 模式
	// 浏览器经 SetWebServeConfig 停机后重新开启的路径）
	m.cond.Broadcast()
	// done 以局部变量捕获：Serve goroutine 与 stopLocked 置 nil m.done 并发，
	// 直接读字段构成数据竞态（-race 实测），局部捕获后 goroutine 不再触碰 manager 字段
	done := m.done
	go func() {
		done <- srv.Serve(ln)
	}()
	return nil
}

// Stop 停机：立即关闭监听与在途连接（含 WS），恢复 service 事件出口为纯
// Wails 出口。未运行时为 no-op。
func (m *webServeManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

// stopLocked 停机（调用方须持 mu）。
//
// 停机语义为立即断开（srv.Close）而非优雅等待（Shutdown）：SetWebServeConfig
// 可由浏览器经 /api/rpc 在 HTTP 处理器内触发停机/重启，此刻当前请求所在的
// 连接正是 Shutdown 要等待的在途请求——自己等自己必然空转到超时上限（实测
// 3s），且超时后旧连接仍滞留为僵尸。立即断开同时消除自等空转与僵尸连接，
// 代价是触发停机的 RPC 响应可能随连接断开而不可达——操作已实际生效，前端
// transport 具备失败提示与重连能力（改址后浏览器本就要重连新地址，可接受）。
// 注意 http.Server.Close 不跟踪 hijacked 的 WS 连接，在途 WS 客户端由下方
// hub.Close 显式关闭。
func (m *webServeManager) stopLocked() error {
	if m.srv == nil {
		return nil
	}
	// 先关闭在途 WS 客户端（在 applySink 前调用：hub 引用尚在册，conn 关闭后
	// readPump 走既有 unregister 收尾，防停机后事件投递到僵尸 hub 的客户端与
	// readPump/writePump goroutine 泄漏）
	if m.hub != nil {
		m.hub.Close()
	}
	err := m.srv.Close()
	// 出口恢复为纯 Wails（hub 随服务停用，避免事件投递到僵尸 hub）
	m.applySink(nil)
	m.srv, m.ln, m.hub, m.handler, m.listen, m.done = nil, nil, nil, nil, "", nil
	// 唤醒 Wait：其消费的旧代际 done 即将收到退出错误，须感知代际已更替
	m.cond.Broadcast()
	return err
}

// Wait 按代际阻塞等待当前 Serve goroutine 退出并返回其错误；服务未运行时
// 阻塞等待下一次 Start（--serve 主进程不得因浏览器侧停机/重启而退出）。
//
// 代际语义：done channel 每次 Start 重建、Stop 置 nil，引用即代际标识。Wait
// 持有当前代际的 done 等待退出错误，收到后核对代际——服务仍在（重启场景新
// done 已建、停机场景 done 为 nil）则说明本次退出由 SetWebServeConfig 触发
// （--serve 模式浏览器改绑定地址/关开关），非进程退出信号，继续等待当前代际；
// 仅当代际未变（Serve 因真实错误自行退出）时才返回错误交由调用方终止进程。
// 由此消除旧实现的缺陷：浏览器触发重启时旧 done 收到 ErrServerClosed，Wait
// 误判为正常停机返回 → runServe 返回 → 进程意外退出。
func (m *webServeManager) Wait() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for {
		for m.done == nil {
			// 服务未运行（浏览器侧 SetWebServeConfig 已停机）：--serve 主进程
			// 存活等待下一次 Start；Start 成功后 cond.Broadcast 唤醒本处
			m.cond.Wait()
		}
		done := m.done
		m.mu.Unlock()
		err := <-done
		m.mu.Lock()
		if m.done == done {
			// 代际未变：Serve 非管理器触发地退出（真实错误），返回给调用方
			return err
		}
		// 代际已更替（重启/停机触发的旧实例退出）：继续等待当前代际
	}
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
