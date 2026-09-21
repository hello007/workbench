package server

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WS 库选型：gorilla/websocket（research/wails-browser-bridge.md 二选一结论）。
// 理由：
//  1. 该库已在本项目依赖树中（wails 的 indirect 依赖，go.sum 在案），升为直接
//     依赖零新增模块；melody 需新增依赖，违背个人工具最小依赖面原则。
//  2. 升级前认证需要精细控制握手流程（401 拒绝须发生在 Upgrade 之前，HTTP 层
//     可控）；gorilla 的 Upgrader 显式分离「认证判断」与「Upgrade 动作」，
//     melody 封装了 handler 挂载，认证前置语义反而要绕其 HandleRequest 约定。
//  3. 连接管理（register/unregister/广播/慢消费者踢出）参照其官方 chat example
//     的 hub 范式，单连接写串行化（独立发送队列）是该模式已验证的核心设计。
const wsPath = "/ws"

// WebSocket 保活与背压默认参数。超时/队列以 hub 实例字段下发（构造后不变，
// writePump/readPump goroutine 无锁可安全读；测试经 newWSHubTuned 收紧，
// 不用包级可变全局——避免测试改写与在途连接 goroutine 读取的数据竞态）。
const (
	// defaultWriteWait 单次写超时：写阻塞超过该时长判定对端不可达，断开连接。
	defaultWriteWait = 10 * time.Second
	// defaultPongWait pong 等待期：超时未收到 pong 判定死连接，读侧断开。
	defaultPongWait = 60 * time.Second
	// wsSendBuffer 每连接发送队列容量：单连接写串行化的缓冲窗口，
	// 溢出即判慢消费者踢出（见 WSHub.Emit）。
	wsSendBuffer = 64
	// wsMaxMessageSize 上行消息上限：浏览器端一期仅接收事件（终端输入走
	// /api/rpc），上行仅协议层 pong，收紧上限防异常客户端耗内存。
	wsMaxMessageSize int64 = 4096
)

// wsFrame 服务端推送的事件帧。
//
// Data 为变参数组，与前端 Wails `EventsOn(name, callback)` 的回调 ...data
// 形态对齐：桌面模式回调直接收变参，浏览器 transport shim（后续 PR）以
// `callback(...frame.data)` 展开还原，两端业务回调签名一致。
type wsFrame struct {
	Event string `json:"event"`
	Data  []any  `json:"data"`
}

// WSHub WebSocket 事件广播 hub（serve 模式事件出口，实现 service.EventSink）。
//
// 广播语义：一条事件推给全部已连接客户端（个人单用户多标签页场景，各标签
// 共享同一后端状态）。单用户工具不做订阅过滤/单播，保持实现最小面。
type WSHub struct {
	token    string
	upgrader websocket.Upgrader
	mu       sync.RWMutex // 保护 clients；send channel 的 close 仅发生在持写锁的 unregister
	clients  map[*wsClient]struct{}

	// 保活参数（构造后只读，pump goroutine 无锁读取）
	writeWait  time.Duration // 单次写超时：写阻塞超过该时长判定对端不可达
	pongWait   time.Duration // pong 等待期：超时未收到 pong 判定死连接
	pingPeriod time.Duration // ping 周期：pongWait * 9 / 10，留 pong 往返余量
}

// wsClient 单个已连接客户端：send 队列由 writePump 独占消费，保证单连接
// 写串行化（gorilla 禁止并发写同一连接，防 interleaved write panic）。
type wsClient struct {
	hub  *WSHub
	conn *websocket.Conn
	send chan []byte
}

// NewWSHub 构造 WebSocket hub。token 为访问令牌（main.runServe 传入
// LoadOrCreateToken 结果），空串视为未配置，所有升级请求 fail closed 拒绝。
func NewWSHub(token string) *WSHub {
	return newWSHubTuned(token, defaultWriteWait, defaultPongWait)
}

// newWSHubTuned 以指定保活超时构造 hub（测试收紧用，非生产入口）。
func newWSHubTuned(token string, writeWait, pongWait time.Duration) *WSHub {
	return &WSHub{
		token: token,
		upgrader: websocket.Upgrader{
			// 客户端以 Sec-WebSocket-Protocol 子协议携带 token 时，升级响应须
			// 回显选中的子协议，否则浏览器判定握手失败主动关闭连接
			Subprotocols: []string{token},
			// CheckOrigin 保留 gorilla 默认同源校验：serve 模式前端与 WS 同源
			// （同一 HTTP 服务），默认拒绝跨源握手可防跨站 WebSocket 劫持
			//（对齐 Jupyter 的 Origin 校验约定）；非浏览器客户端无 Origin 头不受影响。
			ReadBufferSize: 4096,
		},
		clients:    make(map[*wsClient]struct{}),
		writeWait:  writeWait,
		pongWait:   pongWait,
		pingPeriod: pongWait * 9 / 10,
	}
}

// Emit 实现 service.EventSink：广播一条事件帧给全部已连接客户端。
func (h *WSHub) Emit(name string, data ...any) {
	if data == nil {
		data = []any{}
	}
	frame, err := json.Marshal(wsFrame{Event: name, Data: data})
	if err != nil {
		// 载荷不可序列化（含 func/channel 等）只影响本条事件，不中断 hub
		slog.Error("marshal ws event frame failed", "event", name, "err", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- frame:
		default:
			// 慢消费者发送队列溢出：直接关闭连接踢出。读泵感知断连后注销。
			// 慢客户端（长时间不读帧，如休眠标签页）不得拖垮广播方与其余
			// 客户端；net.Conn.Close 幂等，与读泵/写泵的关闭路径并发安全。
			_ = c.conn.Close()
		}
	}
}

// ClientCount 当前已连接客户端数（测试与运行期诊断用）。
func (h *WSHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// SetToken 热轮换访问令牌：更新握手校验基准与升级子协议回显列表。
//
// 语义：已建立的连接不在数据面校验令牌，轮换后保持有效至断开重连（重连须用
// 新令牌）。upgrader 结构体在锁下复制快照后再用于 Upgrade（Upgrade 为阻塞
// 操作不持锁），与本方法写 Subprotocols 互斥，消除并发读写竞态；SetToken
// 每次替换全新 slice（不改旧 slice 元素），快照持有的旧 slice 恒定不可变。
func (h *WSHub) SetToken(token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.token = token
	h.upgrader.Subprotocols = []string{token}
}

// ServeHTTP 处理 GET /ws 升级请求。
//
// 认证必须在升级前完成：失败以 HTTP 401 拒绝（若先升级再踢，客户端已进入
// WebSocket 上下文，拿不到明确 HTTP 状态码，错误语义劣化）。浏览器 WebSocket
// API 无法携带自定义 Authorization 头，支持两种令牌通道：
//   - ?token= 查询参数（非浏览器客户端与手工验证便利；风险与 PR1 HTTP 通道
//     的查询参数形态一致——可能进访问日志，恒定时间比较后即弃，不落日志）；
//   - Sec-WebSocket-Protocol 子协议（浏览器 shim 主通道）：new WebSocket(url,
//     [token]) 以子协议名携带，升级响应回显（upgrader.Subprotocols）。
func (h *WSHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		// 只记来源不记完整 URL：?token= 查询参数形式的令牌不得进日志
		slog.Warn("unauthorized websocket request", "remote", r.RemoteAddr)
		writePreviewError(w, http.StatusUnauthorized, "未授权访问：请携带访问令牌")
		return
	}
	// 锁下取 upgrader 快照（见 SetToken 注释），Upgrade 在锁外执行
	h.mu.RLock()
	upgrader := h.upgrader
	h.mu.RUnlock()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade 失败（非 WebSocket 握手/Origin 校验拒绝等）时已自行写出
		// HTTP 错误响应，此处仅记日志
		slog.Warn("websocket upgrade failed", "remote", r.RemoteAddr, "err", err)
		return
	}
	c := &wsClient{hub: h, conn: conn, send: make(chan []byte, wsSendBuffer)}
	h.register(c)
	go c.writePump()
	h.readPump(c)
}

// authorized 校验升级请求令牌：两种通道任一命中即通过；恒定时间比较防
// 时序侧信道；服务端令牌未配置时一律拒绝（fail closed）。
func (h *WSHub) authorized(r *http.Request) bool {
	h.mu.RLock()
	token := h.token
	h.mu.RUnlock()
	if token == "" {
		return false
	}
	if t := r.URL.Query().Get("token"); t != "" &&
		subtle.ConstantTimeCompare([]byte(t), []byte(token)) == 1 {
		return true
	}
	for _, p := range websocket.Subprotocols(r) {
		if subtle.ConstantTimeCompare([]byte(p), []byte(token)) == 1 {
			return true
		}
	}
	return false
}

// register 登记新客户端。写锁与 unregister 互斥保证：广播方（读锁）视角内
// map 中客户端的 send channel 均未 close。
func (h *WSHub) register(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

// unregister 注销客户端并关闭其发送队列。close 仅在此处（写锁下、且仅当
// 客户端仍在册时）执行一次，杜绝向已 close channel 发送与双重 close。
func (h *WSHub) unregister(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
}

// readPump 读泵：在升级请求的 goroutine 内运行，仅消费协议层 pong 做保活
// 判定；断连（对端关闭/超时/踢出）时注销自身并关闭底层连接。
func (h *WSHub) readPump(c *wsClient) {
	defer func() {
		h.unregister(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(wsMaxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(c.hub.pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.hub.pongWait))
	})
	for {
		// 一期浏览器端无业务上行（终端输入走 /api/rpc），非控制帧一律忽略
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

// writePump 写泵：独占本连接全部写操作（事件帧 + 协议 ping），单连接写
// 串行化的执行端。send 关闭（已注销）时发送 Close 帧后退出。
func (c *wsClient) writePump() {
	ticker := time.NewTicker(c.hub.pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case frame, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.hub.writeWait))
			if !ok {
				// hub 已注销本连接：按 RFC 6455 语义发送 Close 帧后收尾
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.hub.writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
