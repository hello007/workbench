package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/server"
	"workbench/service"
	"workbench/util"
)

// webServeDataDir 自签 TLS 证书的落盘目录（data/tls_cert.pem + data/tls_key.pem）。
// 包级变量供测试重定向到临时目录（对齐 serveTokenFile 先例），生产恒为相对 data/。
var webServeDataDir = "data"

// webServeConnectionsEvent 桌面端浏览器连接明细变化事件（前端 useDeviceConnections
// 桌面通道消费）。与浏览器通道 ws-connections:changed 同义不同名刻意分叉：hub
// 计数口径为浏览器 WS 客户端数，桌面视角 = 纯浏览器连接列表（指示条件 len>=1，且
// 须接收空数组熄指示并清空明细），浏览器视角含自身连接（count>1）；桌面事件浏览器端
// 永不到达，前端通道分流零歧义。data[0] 为事件后的连接明细快照（[]server.ConnDetail，
// 空数组 marshal 为 [] 非 null；TokenName 已由装配层经令牌名册映射补全）。载荷为
// 明细数组而非计数：多设备「是谁连着」（IP/token 名/接入时间）可见性，连接数由
// len 派生，单事件单真相无时序窗口；**隐私红线**：明细仅走本桌面事件，浏览器
// 通道 ws-connections:changed 载荷恒为 count 数字逐字节不变。
const webServeConnectionsEvent = "webserve-connections:changed"

// webServeIPBannedEvent 桌面端来源 IP 自动封禁触发事件（连接管控五期，前端 toast
// 通知数据源）。纯桌面 Wails 事件——不经 WSHub Emit、不进环形缓冲，浏览器通道
// 收不到（**隐私红线**：他人 IP 明细仅走桌面出口，与连接明细事件同款口径）。
// data[0] 为 model.IPBanStatus 快照（ip/bannedAt/expiresAt/failCount/expired，
// json tag 小写形态）。
const webServeIPBannedEvent = "webserve-ip-banned"

// webServeManager 浏览器访问通道管理器：桌面模式同开 HTTP 服务与 --serve 无头
// 模式共用的启动/停机入口。
//
// 生命周期：
//   - 桌面模式：startup 按设置创建并 Start（默认开启，失败仅告警降级——浏览器
//     访问为可选增强能力，端口占用等不影响桌面本体）；shutdown Stop 立即断开停机。
//   - --serve 无头模式：runServe 创建并 Start，Wait 按代际阻塞至服务真正退出
//     （浏览器侧经 SetWebServeConfig 重启/停机不终止进程）。
//
// 运行期变更（桌面设置页）：SetWebServeConfig 改地址/开关/TLS 配置经 Start/Stop
// 平滑重启；RegenerateWebToken 在 WebTokenService 层撤旧建新（verify 闭包直读
// 服务状态，无需任何热轮换通知）。
type webServeManager struct {
	app *App
	mu  sync.Mutex
	srv *http.Server
	// ln 当前生效的原始 TCP listener（TLS 模式下传给 Serve 的是 tls.NewListener
	// 包装体，其 Addr() 委托内层与 ln 一致；本字段恒存原始 listener，
	// listenAddr 语义与纯 HTTP 时代完全一致）
	ln net.Listener
	// hub / handler 当前生效的事件广播 hub 与 HTTP handler（Stop 关闭目标）
	hub     *server.WSHub
	handler *server.WebHandler
	listen  string
	// tls 当前生效的 TLS 配置（幂等键组成部分，与 listen 联合比较；零值 =
	// 纯 HTTP）
	tls WebServeTLSConfig
	// done 每次 Start 重建，承载 Serve goroutine 的退出错误（Wait 按代际消费）
	done chan error
	// cond 联动 mu，供 Wait 在 done 为 nil（服务未运行）时阻塞等待下一次 Start
	cond *sync.Cond
}

// WebServeTLSConfig 浏览器访问通道 TLS 配置（Start 幂等键组成部分）。
//
// Enabled=false（含 nil 入参）为纯 HTTP；Enabled=true 时证书供给双形态：
// CertPath/KeyPath 双空走自签引导（EnsureSelfSignedCert，首次生成后持久化
// data/ 复用），双非空加载用户证书（tls.LoadX509KeyPair）。仅填其一为无效
// 配置（E_TLS_CERT_INVALID）。
type WebServeTLSConfig struct {
	Enabled  bool
	CertPath string
	KeyPath  string
}

// newWebServeManager 构造管理器。app 供装配（RPC target 与事件出口切换）使用。
func newWebServeManager(app *App) *webServeManager {
	m := &webServeManager{app: app}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Start 启动（或按新地址/新 TLS 配置重启）浏览器访问通道 HTTP 服务。
//
// 幂等语义：已在运行且「地址 + TLS 配置」均未变化时为 no-op（同地址开/关 TLS
// 或换证书路径都会触发真实重启——幂等键刻意扩为 listen + TLS 配置联合比较，
// 否则 TLS 开关在设置页表现为「保存了但不生效」）；任一变化时先停旧实例（立即
// 断开，见 stopLocked）再重启（hub/handler 全部重建——校验基准经 verify 闭包
// 直读 WebTokenService，重启前后天然一致）。监听失败（端口占用等）同步返回
// 错误，由调用方决定降级（桌面告警）或上抛（--serve fatal）。
//
// 无半死态保证：TLS 证书加载/生成发生在停旧实例与任何字段赋值之前——证书
// 配置无效（路径错、文件损坏、过期）时正在运行的旧实例不受影响，Start 同步
// 返回 E_TLS_CERT_INVALID。这是把证书加载放在 Start（而非 startLocked）的
// 原因：改地址/配置路径下 stopLocked 先于 startLocked 执行，加载放在
// startLocked 内已无法满足「失败在停旧之前」。
//
// tlsCfg 为 nil 等价于 Enabled=false（纯 HTTP，测试与既有调用方便捷形态）。
func (m *webServeManager) Start(listen string, tlsCfg *WebServeTLSConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := WebServeTLSConfig{}
	if tlsCfg != nil {
		cfg = *tlsCfg
	}
	if m.srv != nil {
		if m.listen == listen && m.tls == cfg {
			return nil
		}
	}
	// 证书加载/生成在任何停旧动作之前：失败时旧实例原样存活（无半死态）
	tlsConf, err := buildWebServeTLSConfig(cfg)
	if err != nil {
		return err
	}
	if m.srv != nil {
		// 地址或 TLS 配置变化：优雅停旧实例后重启（停机错误不阻塞重启，服务即将被替换）
		_ = m.stopLocked()
	}
	return m.startLocked(listen, tlsConf, cfg)
}

// buildWebServeTLSConfig 按 TLS 配置构造 *tls.Config；Enabled=false 返回
// (nil, nil) 表示纯 HTTP。证书加载/生成失败统一包装为 E_TLS_CERT_INVALID
// AppError（前端经 ErrorFormatter 拿到 {code,message} 分流 error 提示）。
func buildWebServeTLSConfig(cfg WebServeTLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	certPath, keyPath := cfg.CertPath, cfg.KeyPath
	if (certPath == "") != (keyPath == "") {
		return nil, model.NewAppError(model.ErrCodeTLSCertInvalid,
			"TLS 证书与私钥路径须同时配置（两者都留空则使用自签证书）")
	}
	if certPath == "" {
		// 双空：自签引导（已有证书对复用，缺失生成并持久化 data/）
		var err error
		if certPath, keyPath, err = server.EnsureSelfSignedCert(webServeDataDir); err != nil {
			return nil, model.WrapAppError(model.ErrCodeTLSCertInvalid, "生成自签 TLS 证书失败", err)
		}
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, model.WrapAppError(model.ErrCodeTLSCertInvalid,
			fmt.Sprintf("加载 TLS 证书失败（%s / %s），请检查证书与私钥路径", certPath, keyPath), err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		// 钉死 ALPN 为 http/1.1：gorilla/websocket 仅支持 HTTP/1.1 升级，
		// ServeTLS 默认协商 h2 会把 WebSocket 引入「浏览器回退行为」分叉面
		//（Go bundled h2 默认禁 extended CONNECT，但该回退属浏览器实现细节），
		// 自建 tls.Config + tls.NewListener 路径下 ALPN 完全由本配置决定。
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	}, nil
}

// startLocked 以指定地址与 TLS 配置启动服务（调用方须持 mu）。
// tlsConf 非 nil 时监听走 TLS（tls.NewListener 包装，Serve 收包装体）。
func (m *webServeManager) startLocked(listen string, tlsConf *tls.Config, cfg WebServeTLSConfig) error {
	hub, handler, err := assembleWebServe(m.app)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("监听 %s 失败（端口可能被占用）: %w", listen, err)
	}
	// m.ln 恒存原始 TCP listener（listenAddr 语义不变）；TLS 模式下传给 Serve
	// 的是 tls.NewListener 包装体（其 Addr() 委托内层，两者一致）
	serveLn := net.Listener(ln)
	if tlsConf != nil {
		serveLn = tls.NewListener(ln, tlsConf)
	}
	// 事件出口切换为复合 sink：Wails 桌面出口（窗口内事件）+ WS hub 广播（浏览器）。
	// Wails 出口对非 Wails 上下文（--serve 无头）静默跳过，等效仅 hub 广播。
	m.applySink(hub)
	// 桌面连接明细回调：hub 明细快照变化经 Wails 出口发桌面事件（纯事件面，
	// Wails 绑定零新增）。sink 在闭包外构造一次捕获，勿每次回调重复新建；回调
	// 仅由 hub register/unregister 出 mu 锁后触发（须在 hub 服务开始前注入，见
	// SetOnConnectionsChange），--serve 无头模式下 wailsEventSink 对非 Wails ctx
	// 静默跳过（既有 sink 行为，无新分支）。
	wailsSink := service.NewWailsEventSink(m.app.ctx)
	hub.SetOnConnectionsChange(func(details []server.ConnDetail) {
		named := m.app.webTokenNamedDetails(details)
		// 连接明细缓存同步写入（令牌在线数聚合数据源，ListWebTokens 消费）：
		// 回调载荷即最新全量快照，整体替换语义
		m.app.rememberConnDetails(named)
		wailsSink.Emit(webServeConnectionsEvent, named)
	})
	// IP 自动封禁 toast 通知注入（连接管控五期）：IPBanService 为进程级单例
	//（计数与封禁状态跨 hub 代际保留，与 hub 生命周期无关），每次 Start 重新注入
	// 同形态闭包（捕获新 sink，语义等价，幂等无泄漏——回调字段整体替换）。回调在
	// 服务层 mu 出锁后同步触发，此处仅经 Wails 出口发桌面事件；serve 无头模式对
	// 非 Wails ctx 静默跳过（既有 sink 行为，无新分支）。停机无收口动作：HTTP
	// 服务停机后认证点不存在，不会再有新封禁触发。
	if m.app.ipBanSvc != nil {
		m.app.ipBanSvc.SetOnBanned(func(status model.IPBanStatus) {
			wailsSink.Emit(webServeIPBannedEvent, status)
		})
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// TLS 握手超时：Go 1.26 起 http.Server 无独立 TLSHandshakeTimeout 字段，
		// 握手期限取 min(ReadHeaderTimeout, ReadTimeout, WriteTimeout)——上方
		// ReadHeaderTimeout=10s 已同时覆盖握手期慢连接（slowloris）防护，
		// 无需额外字段（研究清单「显式设 TLSHandshakeTimeout」按新语义已达成）
	}
	m.srv, m.ln, m.hub, m.handler, m.listen, m.tls = srv, ln, hub, handler, listen, cfg
	m.done = make(chan error, 1)
	// 唤醒 Wait 中因服务未运行（done 为 nil）而阻塞等待的 goroutine（--serve 模式
	// 浏览器经 SetWebServeConfig 停机后重新开启的路径）
	m.cond.Broadcast()
	// done 以局部变量捕获：Serve goroutine 与 stopLocked 置 nil m.done 并发，
	// 直接读字段构成数据竞态（-race 实测），局部捕获后 goroutine 不再触碰 manager 字段
	done := m.done
	go func() {
		done <- srv.Serve(serveLn)
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
	// 桌面连接明细显式清空：hub.Close 收口期回调被抑制（closing 置位后逐连接
	// unregister 不触发回调），浏览器仍连接时停机桌面须熄指示并清空明细列表。
	// 连接明细缓存同处清零（停机后在途连接全部断开，ListWebTokens 在线数归零；
	// 回调被抑制不会自动清，须显式写空快照）。仅在 hub 曾在册时发（m.srv 非 nil
	// 早退已保证）；--serve 无头模式下该 emit 对非 Wails ctx 静默跳过
	if m.hub != nil {
		m.app.rememberConnDetails([]server.ConnDetail{})
		service.NewWailsEventSink(m.app.ctx).Emit(webServeConnectionsEvent, []server.ConnDetail{})
	}
	m.srv, m.ln, m.hub, m.handler, m.listen, m.done = nil, nil, nil, nil, "", nil
	m.tls = WebServeTLSConfig{}
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

// RotateToken 令牌热轮换（已随多 token 改造删除）：校验基准经 verify 闭包直读
// WebTokenService，RegenerateWebToken 的撤旧建新在服务层即时生效，本方法失去
// 存在意义，不再保留。

// KickConnection 踢出指定 WS 连接（桌面明细面板处置入口，App 绑定委托本方法）。
// mu 内取当前 hub 引用后立即释放，断开动作委托 hub.DisconnectClient（其内部
// 自持锁，锁序无嵌套）；服务停机/未启用（hub nil）返回 E_WEBSERVE_NOT_RUNNING，
// 连接已断开/不存在（明细列表过期或并发断开竞态）返回 E_CONNECTION_NOT_FOUND。
// 命中时返回被踢连接的元数据快照（供 App 层审计留痕），miss 时返回零值。
// 踢出语义 = 断开当前连接（对端同令牌可重连，非封禁；防重连由撤销令牌承担）。
func (m *webServeManager) KickConnection(connID uint64) (server.ConnDetail, error) {
	m.mu.Lock()
	hub := m.hub
	m.mu.Unlock()
	if hub == nil {
		return server.ConnDetail{}, model.NewAppError(model.ErrCodeWebServeNotRunning,
			"浏览器访问服务未运行，无法踢出连接")
	}
	detail, ok := hub.DisconnectClient(connID)
	if !ok {
		return server.ConnDetail{}, model.NewAppError(model.ErrCodeConnNotFound,
			"连接不存在或已断开（列表已过期，请刷新后重试）")
	}
	return detail, nil
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

// applySink 将八个持事件出口的服务（terminal/update/ai_function/git/chat/pptx
// split/assets/pptx merge）统一切换：sink 非 nil → 复合出口（Wails + hub 广播）；
// nil → 恢复纯 Wails 出口。SetEventSink 并发安全（sinkHolder 读写锁），输出泵等
// goroutine 读侧已受保护。新增持事件 service 须在此补行，漏加则浏览器通道
// 静默收不到该服务全部事件。
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
	m.app.chatSvc.SetEventSink(sink)
	m.app.pptxSplitSvc.SetEventSink(sink)
	m.app.assetsSvc.SetEventSink(sink)
	m.app.pptxMergeSvc.SetEventSink(sink)
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
	tlsCfg := &WebServeTLSConfig{
		Enabled:  cfg.WebServe.TLS,
		CertPath: cfg.WebServe.TLSCertPath,
		KeyPath:  cfg.WebServe.TLSKeyPath,
	}
	if err := a.webServe.Start(listen, tlsCfg); err != nil {
		slog.Warn("浏览器访问服务启动失败，桌面功能不受影响（可检查端口是否被占用、TLS 证书配置或修改设置中的绑定地址）", "addr", listen, "err", err)
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
// 设备可达）。tlsEnabled 时 scheme 输出 https://（提示地址与证书 SAN 同源覆盖，
// 枚举复用 util.LocalIPv4Addresses 单点）。地址解析失败（缺端口）时返回 nil。
func webAccessUrls(bindAddress string, tlsEnabled bool) []string {
	_, port, err := net.SplitHostPort(bindAddress)
	if err != nil || port == "" {
		return nil
	}
	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	urls := []string{scheme + "://127.0.0.1:" + port}
	ips, err := util.LocalIPv4Addresses()
	if err != nil {
		return urls
	}
	for _, ip := range ips {
		urls = append(urls, scheme+"://"+ip+":"+port)
	}
	return urls
}
