package server

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
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
	// wsMaxMessageSize 上行消息上限：浏览器端业务上行仅重连续传握手帧
	//（{"resume":<序号>}，终端输入走 /api/rpc），其余为协议层 pong，
	// 收紧上限防异常客户端耗内存。
	wsMaxMessageSize int64 = 4096
	// wsResumeBuffer 重连续传环形缓冲容量（帧数）：断线期间的事件帧在此
	// 有界保留，重连握手携带已收序号后补发缺口。溢出即丢最旧帧——补发
	// 判定为断层（ws-gap 控制帧），客户端提示后走全量刷新，不静默截断。
	wsResumeBuffer = 2000
)

// 续传控制帧事件名（无 seq 字段，不参与序号基准；浏览器 transport 在分发层
// 拦截处理，不进入业务监听器）。
const (
	// wsGapEvent 断层标记帧：data[0] 为服务端当前序号。缓存溢出（客户端
	// 断线期间事件量超出环形缓冲）或 hub 重建（webServeManager 重启使序号
	// 归零）导致客户端序号无法衔接时发送，客户端据此重置基准并提示断层。
	wsGapEvent = "ws-gap"
	// wsSyncedEvent 补发完成标记帧：data[0] 为补发快照时的服务端当前序号。
	// 补发帧与重连注册窗口内的实时广播共用同一 send 队列（FIFO 交错到达），
	// 客户端以该标记为界将缓冲帧按序号去重落地，保证业务事件有序且不重不漏。
	wsSyncedEvent = "ws-synced"
	// wsKickedEvent 被踢单播控制帧（连接管控二期）：仅送达被踢连接（hub 首个
	// 非广播投递面），无 seq、不进环形缓冲——resume 补发面不含该帧，防重连后
	// 幻影回放「被踢」。前端 transport 分发层拦截（ws-gap/ws-synced 同位）转
	// connBanner 被踢态提示；帧送达失败（队列满/写失败）静默退化为普通断开，
	// 踢出断开本身恒生效（best-effort 口径，见 DisconnectClient）。
	wsKickedEvent = "ws-kicked"
)

// wsConnectionsEvent 连接数变化事件（多设备语义明示，D1 决策：架构零改动只提示）。
// 每次客户端 register/unregister 成功路径广播一条，data[0] 为事件后的连接总数。
// 事件经常规 Emit 进环形缓冲占用 seq——resume 补发面包含该事件，计数为幂等
// 快照（最后到达值即当前态），重放/乱序对消费方无碍，续传契约不变。
// 桌面 Wails 出口不产生该事件（仅 hub 广播），前端监听空安全。
const wsConnectionsEvent = "ws-connections:changed"

// wsFrame 服务端推送的事件帧。
//
// Data 为变参数组，与前端 Wails `EventsOn(name, callback)` 的回调 ...data
// 形态对齐：桌面模式回调直接收变参，浏览器 transport shim 以
// `callback(...frame.data)` 展开还原，两端业务回调签名一致。
//
// Seq 为 hub 级单调递增事件序号（自 1 起，仅成功广播的事件占号），浏览器端
// 记录已收最大序号，重连握手据此请求缺口补发；omitempty 使无序号的控制帧
// （ws-gap / ws-synced）不携带该字段，向后兼容仅识别 {event,data} 的消费方。
type wsFrame struct {
	Event string `json:"event"`
	Data  []any  `json:"data"`
	Seq   uint64 `json:"seq,omitempty"`
}

// wsRingEntry 环形缓冲条目：已序列化帧字节与其序号成对存储。序号显式落条目
// 使缺口判定不依赖「按位置推算」的实现细节；字节在入环时序列化一次，补发
// 复用同一份，避免重连风暴下的重复 marshal。
type wsRingEntry struct {
	seq   uint64
	frame []byte
}

// WSHub WebSocket 事件广播 hub（serve 模式事件出口，实现 service.EventSink）。
//
// 广播语义：一条事件推给全部已连接客户端（个人单用户多标签页场景，各标签
// 共享同一后端状态）。单用户工具不做订阅过滤/单播，保持实现最小面。
type WSHub struct {
	verify              TokenVerifier          // 握手令牌校验函数（构造后只读；nil = 一切拒绝 fail closed）
	banCheck            BanCheck               // 握手封禁判定（构造后只读；nil = 未启用，verify 放行后组合判定）
	onAuth              func(AuthEvent)        // 认证事件出口（可 nil）
	onConnectionsChange func([]ConnDetail)     // 连接明细快照变化回调（桌面事件出口用，可 nil；见 SetOnConnectionsChange）
	upgrader            websocket.Upgrader
	mu                  sync.RWMutex // 保护 clients 与 closing；send channel 的 close 仅发生在持写锁的 unregister
	clients             map[*wsClient]struct{}
	closing             bool // hub 已进入关停（Close 调用后置位）：register/unregister 不再广播连接数事件
	connSeq             atomic.Uint64 // 连接 ID 分配器（自 1 起）：ServeHTTP 升级成功时取号，跨连接单调

	// 续传序号与环形缓冲（seqMu 保护）。锁序恒为 seqMu → mu：Emit 在 seqMu
	// 临界区内完成「占号-入环-广播」，resume 补发在同一临界区取补发集，保证
	// 序号单调、缓冲与序号一致、补发集与实时广播不重不漏；mu 的写侧路径
	//（register/unregister）均不获取 seqMu，无反向嵌套死锁面。
	seqMu     sync.Mutex
	seq       uint64        // 已分配的最大事件序号（自 0 起，广播前自增）
	ring      []wsRingEntry // 环形缓冲：未满时尾追、满后覆写最旧（ringHead 指向最旧条目）
	ringHead  int
	resumeBuf int // 环形缓冲容量（构造后只读）

	// 保活参数（构造后只读，pump goroutine 无锁读取）
	writeWait  time.Duration // 单次写超时：写阻塞超过该时长判定对端不可达
	pongWait   time.Duration // pong 等待期：超时未收到 pong 判定死连接
	pingPeriod time.Duration // ping 周期：pongWait * 9 / 10，留 pong 往返余量
}

// ConnDetail 单个在册 WebSocket 连接的元数据快照（桌面连接明细出口载荷，多设备
// 「是谁连着」可见性）。字段在 ServeHTTP 升级成功时一次性赋值，此后不可变——
// register/unregister 持 mu 写锁遍历 clients 构造全量快照，无并发读改面。
//
// TokenName 由装配层（web_serve.go startLocked 闭包持 WebTokenService）映射填充，
// server 包恒留空：名称映射依赖 service 层令牌名册，server 不反向依赖（分层红线）。
// 映射 miss（令牌已撤销但连接未断）由装配层降级，前端 fallback 展示。
type ConnDetail struct {
	// ID 连接 ID（hub 级自增，自 1 起）：前端列表 key 与快照排序键（即接入顺序）。
	ID uint64 `json:"id"`
	// IP 对端来源地址（RemoteAddr 剥端口；SplitHostPort 失败时原样保留）。
	IP string `json:"ip"`
	// TokenID 命中的令牌记录 ID（authorized 返回的审计归因键）。
	TokenID string `json:"tokenId"`
	// TokenName 令牌名（装配层映射填充，server 恒空；前端空值 fallback「已撤销令牌」）。
	TokenName string `json:"tokenName"`
	// ConnectedAt 本次连接建立时间（重连即新连接条目，取新值；RFC3339 序列化）。
	ConnectedAt time.Time `json:"connectedAt"`
}

// wsClient 单个已连接客户端：send 队列由 writePump 独占消费，保证单连接
// 写串行化（gorilla 禁止并发写同一连接，防 interleaved write panic）。
type wsClient struct {
	hub    *WSHub
	conn   *websocket.Conn
	send   chan []byte
	detail ConnDetail // 连接元数据（ServeHTTP 升级成功时构造，此后不可变）
	// kicked 踢出流程已启动（hub.mu 保护）：DisconnectClient 入队踢出帧后置位，
	// 防重复踢出在 writePump 自关路径上二次入队垃圾帧；置位后重复调用幂等返回
	// 命中（detail 照常返回），不再重复入队。
	kicked bool
}

// NewWSHub 构造 WebSocket hub。verify 为握手令牌校验函数（main 装配传入
// WebTokenService.Verify 闭包，多 token 哈希集合校验由服务层持有，新建/撤销/
// 过期即时生效——单 token 时代的 SetToken 热轮换已随 verify 化删除）；onAuth
// 为认证事件出口（可 nil）；banCheck 为封禁判定函数（连接管控四期，可 nil =
// 未启用；verify 放行后的组合判定，非空消息升级前 401 拒绝，见 BanCheck）。
// verify 为 nil 时所有升级请求 fail closed 拒绝。
func NewWSHub(verify TokenVerifier, onAuth func(AuthEvent), banCheck BanCheck) *WSHub {
	return newWSHubTuned(verify, onAuth, banCheck, defaultWriteWait, defaultPongWait, wsResumeBuffer)
}

// SetOnConnectionsChange 设置连接明细快照变化回调（桌面事件出口用，main 装配层
// 在 hub 服务开始前注入——web_serve.go startLocked 在 applySink 之后调用）。须在
// ServeHTTP 开始服务前设置：回调为纯函数指针无内部锁，启动后热更换闭包不做线程
// 安全承诺（生产装配一次即定，无运行期热更路径）。回调载荷为全量明细快照（幂等
// 可重放，连接数由 len 派生——明细与计数单事件单真相，无两事件间时序窗口）；回调
// 与 hub 内部 WS 广播（ws-connections:changed，仅 count 数字——他人 IP/token 明细
// 不进浏览器通道，隐私红线）是两条独立出口：空快照（count=0）也须透传（桌面最后
// 一个浏览器断开要熄指示并清空明细列表，WS 广播侧对 count=0 的抑制不影响回调），
// closing 收口期统一抑制（见 notifyConnectionDetails）。
func (h *WSHub) SetOnConnectionsChange(fn func(details []ConnDetail)) {
	h.onConnectionsChange = fn
}

// newWSHubTuned 以指定保活超时与续传缓冲容量构造 hub（测试收紧用，非生产入口）。
func newWSHubTuned(verify TokenVerifier, onAuth func(AuthEvent), banCheck BanCheck, writeWait, pongWait time.Duration, resumeBuffer int) *WSHub {
	if resumeBuffer < 0 {
		resumeBuffer = 0
	}
	return &WSHub{
		verify:   verify,
		banCheck: banCheck,
		onAuth:   onAuth,
		upgrader: websocket.Upgrader{
			// Subprotocols 保持 nil：多 token 语义下回显值 = 命中的子协议候选
			//（固定列表无法枚举动态 token 集合），ServeHTTP 在子协议通道命中时
			// 经 Upgrade 的 responseHeader 传入（gorilla 在 Subprotocols 为 nil
			// 时取 responseHeader 的 Sec-Websocket-Protocol 作为选中子协议回显）
			// CheckOrigin 保留 gorilla 默认同源校验：serve 模式前端与 WS 同源
			// （同一 HTTP 服务），默认拒绝跨源握手可防跨站 WebSocket 劫持
			//（对齐 Jupyter 的 Origin 校验约定）；非浏览器客户端无 Origin 头不受影响。
			ReadBufferSize: 4096,
		},
		clients:    make(map[*wsClient]struct{}),
		resumeBuf:  resumeBuffer,
		ring:       make([]wsRingEntry, 0, resumeBuffer),
		writeWait:  writeWait,
		pongWait:   pongWait,
		pingPeriod: pongWait * 9 / 10,
	}
}

// Emit 实现 service.EventSink：分配单调序号、写入续传环形缓冲后广播一条
// 事件帧给全部已连接客户端。
//
// 整个「占号-入环-广播」在 seqMu 临界区内完成：序号分配与入环同锁防乱序，
// 广播同锁使任一帧的投递先于其后续快照的补发集计算（客户端视角序号单调）。
// 广播投递为非阻塞 select，持锁不引入阻塞放大。
func (h *WSHub) Emit(name string, data ...any) {
	if data == nil {
		data = []any{}
	}
	h.seqMu.Lock()
	h.seq++
	frame, err := json.Marshal(wsFrame{Event: name, Data: data, Seq: h.seq})
	if err != nil {
		// 载荷不可序列化（含 func/channel 等）只影响本条事件，不中断 hub；
		// 序号尚未对外发布即回退，保持「仅成功广播占号」的连续分配不变式，
		// 简化缺口判定与按位置推算的推理
		h.seq--
		h.seqMu.Unlock()
		slog.Error("marshal ws event frame failed", "event", name, "err", err)
		return
	}
	h.ringAppendLocked(wsRingEntry{seq: h.seq, frame: frame})
	h.mu.RLock()
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
	h.mu.RUnlock()
	h.seqMu.Unlock()
}

// ringAppendLocked 追加条目到续传环形缓冲（seqMu 持有方可调用）：
// 未满尾追、满后覆写最旧并前移 head，容量为构造时给定的 resumeBuf。
func (h *WSHub) ringAppendLocked(e wsRingEntry) {
	if len(h.ring) < h.resumeBuf {
		h.ring = append(h.ring, e)
		return
	}
	if h.resumeBuf == 0 {
		return
	}
	h.ring[h.ringHead] = e
	h.ringHead = (h.ringHead + 1) % h.resumeBuf
}

// ringIterateLocked 按最旧到最新遍历环形缓冲（seqMu 持有方可调用）。
// 未满时 head 恒为 0（从未前移），下标即遍历序；满后从 head 起环形取模。
func (h *WSHub) ringIterateLocked(fn func(e wsRingEntry) bool) {
	n := len(h.ring)
	for i := 0; i < n; i++ {
		if !fn(h.ring[(h.ringHead+i)%n]) {
			return
		}
	}
}

// resumePlanLocked 计算重连补发集（seqMu 持有方可调用）。
//
// 返回 gap=true 表示断层、无法补发，两种成因：
//  1. 缓冲溢出：客户端断线期间事件量超出环形缓冲，缺口帧已被覆写丢弃；
//  2. hub 重建：webServeManager 重启使序号归零，客户端携带的已收序号超前于
//     服务端当前序号（跨代际序号不可比较）。
//
// gap=false 时 frames 为序号大于 lastSeq 的全部缓冲帧（按序号升序）；环形
// 缓冲为空且 lastSeq>0 属成因 2（hub 从未广播过事件而客户端已收过帧）。
func (h *WSHub) resumePlanLocked(lastSeq uint64) (frames []wsRingEntry, gap bool, current uint64) {
	current = h.seq
	if lastSeq > current {
		// 客户端序号超前于服务端：hub 重建（seq 归零）后唯一可能的形态
		return nil, true, current
	}
	if len(h.ring) == 0 {
		// 缓冲为空：lastSeq==current 时本就无缺口（current==0 即全新 hub）；
		// lastSeq<current 时缺口帧不在缓冲中（容量 0 的构造从未入环），无从
		// 补发，按断层处理。环形缓冲满后 len 恒等于容量，非空态不会归空，
		// 故空缓冲只有「从未入环」一种成因
		return nil, lastSeq < current, current
	}
	var oldest uint64
	h.ringIterateLocked(func(e wsRingEntry) bool {
		oldest = e.seq
		return false
	})
	if lastSeq+1 < oldest {
		// lastSeq 与缓冲最旧序号之间存在已被覆写丢弃的帧：溢出断层
		return nil, true, current
	}
	h.ringIterateLocked(func(e wsRingEntry) bool {
		if e.seq > lastSeq {
			frames = append(frames, e)
		}
		return true
	})
	return frames, false, current
}

// ClientCount 当前已连接客户端数（测试与运行期诊断用）。
func (h *WSHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Close 关闭 hub 当前全部在途客户端连接（服务停机路径调用，杜绝 Shutdown 不
// 关 hijacked 连接导致的僵尸客户端与 readPump/writePump goroutine 泄漏）。
//
// 只关底层 conn，不直接 close(c.send)：conn 关闭使 readPump 的 ReadMessage
// 返回错误，走既有 unregister（写锁下 delete + close(send)，仅当客户端仍在册）
// 收尾，天然防双重 close 与向已 close channel 发送；Close 后再 Emit 亦安全
// （clients 随各连接 unregister 逐步清空，残留连接的 send 投递失败由 Emit 的
// 溢出分支 conn.Close 兜底，net.Conn.Close 幂等）。
func (h *WSHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	// 置位关停标记：此后 readPump defer 触发的逐连接 unregister 不再广播
	// 连接数事件（收口期无意义，见 notifyConnectionDetails 注释）
	h.closing = true
	for c := range h.clients {
		_ = c.conn.Close()
	}
}

// DisconnectClient 按连接 ID 踢出单个在途客户端（桌面明细面板处置入口，main 层
// webServeManager.KickConnection 委托）。mu 写锁内遍历 clients 定位目标，命中后
// 不再直接 conn.Close：向该连接的 send 队列非阻塞入队（1）ws-kicked 单播控制帧
// （无 seq，不进环形缓冲）与（2）nil 哨兵——writePump（单写者）按 FIFO 先写出
// 踢出帧，见哨兵即发 RFC Close 帧后退出（defer conn.Close() 收尾），被踢端因此
// 先收到「被主机断开」提示帧再感知连接关闭；readPump 的 ReadMessage 感知断连后
// 走既有 defer unregister（写锁下 delete + close(send)，仅当客户端仍在册）自然
// 收尾，并经 notifyConnectionDetails 触发明细快照双出口（桌面事件 + 浏览器
// count 广播），**零新增广播事件**。
//
// 降级（best-effort 口径）：入队失败（队列满——慢消费者形态）回退直接
// conn.Close()，踢出帧丢失退化为普通断开，踢出断开本身恒生效；writePump 写帧
// 失败同理。nil 哨兵在踢出帧之后单独入队，第二次入队失败（并发 Emit 恰好填满
// 队列的极窄窗口）同样回退 conn.Close()——此时踢出帧可能已入队但无人写出，conn
// 关闭使 writePump 在途写失败退出，帧丢弃、断开恒达成。
//
// 返回 (detail, ok)：ok=false 为 miss（连接不存在/已断开）；ok=true 时 detail 为
// 被踢连接的元数据快照（TokenID/IP/接入时间，供调用方审计留痕——审计在调用方
// 命中后落盘，miss 不写）。
//
// 幂等：已注销连接不在 clients，重复踢出天然 miss 返 (零值, false)；哨兵已入队、
// readPump 尚未收尾的窄窗口内重复踢出命中在册连接，kicked 标记挡住二次入队
//（防 writePump 自关路径上堆积无人消费的垃圾帧），detail 照常返回、ok=true（与
// 旧实现「窗口内重复踢出返 true」的语义一致）。本方法自身不在锁内调 notify/Emit
//（锁序红线：Emit 内部按 seqMu → mu.RLock 加锁，mu 写锁临界区内直调会形成同
// goroutine 重入 RLock 死锁）——断连收尾的 notify 由 readPump 自己的 goroutine
// 出锁后触发，与本调用无并发锁窗口。
//
// 踢出语义 = 断开当前连接，对端同令牌可重连（重连须用有效令牌），非封禁；防重
// 连由撤销令牌承担（撤销即时生效于新连接，两手段互补不互替）。
func (h *WSHub) DisconnectClient(connID uint64) (ConnDetail, bool) {
	h.mu.Lock()
	var target *wsClient
	for c := range h.clients {
		if c.detail.ID == connID {
			target = c
			break
		}
	}
	var detail ConnDetail
	if target != nil {
		detail = target.detail
		if !target.kicked {
			target.kicked = true
			if !h.enqueueKickFrames(target) {
				// 队列满（慢消费者）或帧序列化失败：回退直关，踢出帧丢失退化为
				// 普通断开（断开恒生效）；net.Conn.Close 幂等，与读写泵关闭路径
				// 并发安全
				_ = target.conn.Close()
			}
		}
	}
	h.mu.Unlock()
	return detail, target != nil
}

// enqueueKickFrames 向被踢连接的 send 队列非阻塞入队 ws-kicked 单播帧与 nil 哨兵
//（调用方须持 mu 写锁，且仅对未 kicked 的连接调用一次）。入队成功返回 true：
// writePump 按 FIFO 先写出踢出帧、见哨兵后发 Close 帧退出，踢出帧先于连接关闭
// 送达；任一步失败返回 false，调用方回退直接 conn.Close()。
//
// 踢出帧不经 Emit（无 seq、不进环形缓冲——resume 补发面不含该帧，防重连幻影
// 回放），与 ws-gap/ws-synced 控制帧同款口径；在锁内做非阻塞入队与序列化安全
//（channel send 非阻塞、marshal 纯内存，且 send 的 close 仅发生在同锁的
// unregister，无并发窗口）。
func (h *WSHub) enqueueKickFrames(c *wsClient) bool {
	frame, err := json.Marshal(wsFrame{Event: wsKickedEvent, Data: []any{}})
	if err != nil {
		// 固定字段载荷不会失败，防御保留
		slog.Error("marshal ws kick frame failed", "connId", c.detail.ID, "err", err)
		return false
	}
	select {
	case c.send <- frame:
	default:
		return false
	}
	select {
	case c.send <- nil:
		return true
	default:
		return false
	}
}

// ServeHTTP 处理 GET /ws 升级请求。
//
// 认证必须在升级前完成：失败以 HTTP 401 拒绝（若先升级再踢，客户端已进入
// WebSocket 上下文，拿不到明确 HTTP 状态码，错误语义劣化）。浏览器 WebSocket
// API 无法携带自定义 Authorization 头，支持三种令牌通道：
//   - Sec-WebSocket-Protocol 子协议（浏览器 shim 主通道）：new WebSocket(url,
//     [token]) 以子协议名携带，升级响应回显（多 token 下回显命中的候选值）；
//   - wb_token cookie（浏览器会话通道）：首访 HTTP 请求种下的会话 cookie 随
//     同源 WS 握手自动携带，支撑前端无 localStorage 令牌时的裸连形态；
//   - ?token= 查询参数（非浏览器客户端与手工验证便利；风险与 PR1 HTTP 通道
//     的查询参数形态一致——可能进访问日志，校验后即弃，不落日志）。
//
// 成功/失败各触发一次 onAuth 认证事件（装配层注入审计落盘）。已建立的连接
// 不在数据面校验令牌，保持有效至断开重连（重连须用有效令牌）。
func (h *WSHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	matched, tokenID, ok := h.authorized(r)
	if !ok {
		emitAuthEvent(h.onAuth, AuthEventFailure, "", r)
		// 只记来源不记完整 URL：?token= 查询参数形式的令牌不得进日志
		slog.Warn("unauthorized websocket request", "remote", r.RemoteAddr)
		writePreviewError(w, http.StatusUnauthorized, "未授权访问：请携带访问令牌")
		return
	}
	// 封禁组合判定（连接管控四期，verify 放行后）：命中生效中的封禁以 HTTP 401
	// 拒绝升级并透出含解封时间的专用消息（与「请携带访问令牌」可区分）。审计走
	// auth_failure 且 tokenID 填命中值——封禁形态非空，与普通认证失败（恒空）可
	// 区分；不发 auth_success（认证未放行）。升级前拒绝语义与认证失败一致
	//（先升级再踢拿不到明确 HTTP 状态码，错误语义劣化）。
	if h.banCheck != nil {
		if msg := h.banCheck(tokenID); msg != "" {
			emitAuthEvent(h.onAuth, AuthEventFailure, tokenID, r)
			// 只记来源不记完整 URL：查询参数通道的令牌不得进日志
			slog.Warn("banned websocket handshake", "remote", r.RemoteAddr)
			writePreviewError(w, http.StatusUnauthorized, msg)
			return
		}
	}
	emitAuthEvent(h.onAuth, AuthEventSuccess, tokenID, r)
	// 子协议通道命中的凭据须回显该子协议（浏览器 shim 依赖回显判定握手成功；
	// 缺失时浏览器主动关闭连接）。多 token 下回显值 = 命中的 candidate 而非
	// 固定令牌，经 responseHeader 传递（见 NewWSHub 的 Subprotocols=nil 注释）
	var respHeader http.Header
	for _, p := range websocket.Subprotocols(r) {
		if p == matched {
			respHeader = http.Header{"Sec-Websocket-Protocol": []string{matched}}
			break
		}
	}
	conn, err := h.upgrader.Upgrade(w, r, respHeader)
	if err != nil {
		// Upgrade 失败（非 WebSocket 握手/Origin 校验拒绝等）时已自行写出
		// HTTP 错误响应，此处仅记日志
		slog.Warn("websocket upgrade failed", "remote", r.RemoteAddr, "err", err)
		return
	}
	c := &wsClient{hub: h, conn: conn, send: make(chan []byte, wsSendBuffer), detail: newConnDetail(h, r, tokenID)}
	h.register(c)
	go c.writePump()
	h.readPump(c)
}

// newConnDetail 从升级请求采集连接元数据（ServeHTTP 升级成功后调用一次，此后
// 不可变——register/unregister 的快照构造仅读该值，无并发写面）。IP 取 RemoteAddr
// 剥端口（SplitHostPort 失败原样保留，Unix socket 等无端口形态兜底）；连接 ID 取
// hub 级自增号（跨连接单调，即接入顺序，兼作快照排序键保证载荷顺序确定）。
func newConnDetail(h *WSHub, r *http.Request, tokenID string) ConnDetail {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ip = host
	}
	return ConnDetail{
		ID:          h.connSeq.Add(1),
		IP:          ip,
		TokenID:     tokenID,
		ConnectedAt: time.Now(),
	}
}

// authorized 校验升级请求令牌：三种通道任一命中即通过——Sec-WebSocket-Protocol
// 子协议（浏览器 shim 主通道，显式凭据）> wb_token cookie（浏览器会话，裸连
// WebSocket 无 protocols 参数时依赖）> ?token= 查询参数（手工验证便利）；
// 逐候选调用 verify（哈希比对恒定时间由实现方保证）；verify 未配置（nil）时
// 一律拒绝（fail closed）。
//
// 返回 matched 为命中的明文凭据候选（子协议回显与调试日志不落完整 URL 均以
// 候选值为准），tokenID 为命中记录 ID（审计归因键）。
func (h *WSHub) authorized(r *http.Request) (matched, tokenID string, ok bool) {
	if h.verify == nil {
		return "", "", false
	}
	for _, p := range websocket.Subprotocols(r) {
		if p == "" {
			continue
		}
		if id, valid := h.verify(p); valid {
			return p, id, true
		}
	}
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		if id, valid := h.verify(c.Value); valid {
			return c.Value, id, true
		}
	}
	if t := r.URL.Query().Get("token"); t != "" {
		if id, valid := h.verify(t); valid {
			return t, id, true
		}
	}
	return "", "", false
}

// register 登记新客户端。写锁与 unregister 互斥保证：广播方（读锁）视角内
// map 中客户端的 send channel 均未 close。
func (h *WSHub) register(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	details := h.snapshotDetailsLocked()
	notify := !h.closing
	h.mu.Unlock()
	h.notifyConnectionDetails(details, notify)
}

// unregister 注销客户端并关闭其发送队列。close 仅在此处（写锁下、且仅当
// 客户端仍在册时）执行一次，杜绝向已 close channel 发送与双重 close。
func (h *WSHub) unregister(c *wsClient) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	details := h.snapshotDetailsLocked()
	notify := !h.closing
	h.mu.Unlock()
	h.notifyConnectionDetails(details, notify)
}

// snapshotDetailsLocked 构造当前在册连接的全量明细快照（mu 写锁持有方可调用）。
// 各连接 detail 构造后不可变，遍历仅读；按连接 ID 升序排序使事件载荷顺序确定
//（即接入顺序，前端列表无需再排）。
func (h *WSHub) snapshotDetailsLocked() []ConnDetail {
	details := make([]ConnDetail, 0, len(h.clients))
	for c := range h.clients {
		details = append(details, c.detail)
	}
	sort.Slice(details, func(i, j int) bool { return details[i].ID < details[j].ID })
	return details
}

// notifyConnectionDetails 连接集合变化的双出口分发（多设备语义明示，多端共享
// 同一后端状态的前端可见性提示）：
//   - 桌面回调出口（onConnectionsChange）：全量明细快照透传（含空快照——桌面视角
//     明细为纯浏览器连接列表，最后一个浏览器断开须熄指示并清空列表；桌面事件
//     webserve-connections:changed 由 main 装配层闭包发射（载荷经令牌名映射补全），
//     server 包不知 Wails，分层红线）；
//   - hub 内部 WS 广播（ws-connections:changed）：仅 count>0 时广播且载荷恒为
//     count 数字——浏览器视角计数含自身连接（指示条件 count>1），count=0 时无
//     接收者；**隐私红线**：他人 IP/token 明细不进浏览器通道，浏览器载荷逐字节
//     不变。两出口语义分叉刻意不同名不同判定，前端 useDeviceConnections 双通道
//     消费分流。
//
// **锁序红线**：调用方必须在释放 mu 写锁之后调用——Emit 内部按 seqMu → mu.RLock
// 加锁，若在 mu 写锁临界区内调用会形成同 goroutine 重入 RLock 死锁（RWMutex
// 写锁持有方再取读锁不被允许）。明细快照在写锁临界区内构造出局部变量，出锁后
// 触发回调与广播。
//
// WS 广播事件经常规 Emit 进环形缓冲占用 seq：resume 补发面包含该事件，计数为
// 幂等快照（最后到达值即当前态），重放无碍，续传契约不变。并发连接/断开场景
// 下两条 changed 事件的发送顺序可能与计数变化顺序不一致（各自独立加 seqMu），
// 事件仅信息提示用途可接受。共同抑制态：hub 关停（closing）后不触发——Close
// 收口期逐连接 unregister 的递减序列对前端无意义（连接即将随服务停机断开），
// 桌面归零由 manager 停机路径显式补发空快照（web_serve.go stopLocked）。
func (h *WSHub) notifyConnectionDetails(details []ConnDetail, notify bool) {
	if !notify {
		return
	}
	if fn := h.onConnectionsChange; fn != nil {
		fn(details)
	}
	if len(details) == 0 {
		return
	}
	h.Emit(wsConnectionsEvent, len(details))
}

// readPump 读泵：在升级请求的 goroutine 内运行，消费协议层 pong 做保活判定，
// 并解析客户端上行帧——目前唯一业务上行是重连续传握手 {"resume":<已收序号>}；
// 断连（对端关闭/超时/踢出）时注销自身并关闭底层连接。
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
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		h.handleClientFrame(c, payload)
	}
}

// handleClientFrame 解析客户端上行帧。非法 JSON、非对象或缺少 resume 字段
// 的帧一律静默忽略（客户端协议外的上行不存在，不响应不记日志防刷屏）。
func (h *WSHub) handleClientFrame(c *wsClient, payload []byte) {
	var msg struct {
		Resume *uint64 `json:"resume"`
	}
	if json.Unmarshal(payload, &msg) != nil || msg.Resume == nil {
		return
	}
	h.resumeClient(c, *msg.Resume)
}

// resumeClient 处理重连续传握手：按客户端已收序号补发环形缓冲内的缺口帧，
// 再发同步完成标记；断层（缓冲溢出 / hub 重建序号归零）则只发断层标记帧，
// 由客户端重置基准、提示断层后走全量刷新，此后广播从当前序号正常继续。
//
// 并发安全论证（补发写入路径选型：经既有 send 队列，与广播同路）：
//   - send channel 的 close 仅发生在 unregister（写锁下），而 unregister 只由
//     本 goroutine（readPump）的 defer 调用，补发投递与 close 无并发窗口；
//   - 补发投递与广播投递同为 channel send（无需持锁），writePump 单写者消费
//     保持单连接写串行化，不引入 gorilla 禁止的并发 conn 写；
//   - 补发帧与重连注册窗口内已入队的实时广播帧在 FIFO 队列中交错到达，
//     由客户端以 ws-synced 标记为界按序号去重落地（见 events.js sync 模式），
//     服务端无需额外排序协调。
//
// 补发投递为非阻塞 select：队列满说明客户端消费不及时，阻塞式灌入会拖住
// readPump（无法读 pong 被保活判定断开，且注销 defer 无法执行形成 goroutine
// 停滞），故按慢消费者同语义关连接终止补发，客户端重连后重新走 resume 流程。
func (h *WSHub) resumeClient(c *wsClient, lastSeq uint64) {
	h.seqMu.Lock()
	fill, gap, current := h.resumePlanLocked(lastSeq)
	h.seqMu.Unlock()
	if gap {
		h.sendControlFrame(c, wsGapEvent, current)
		return
	}
	for _, e := range fill {
		select {
		case c.send <- e.frame:
		default:
			_ = c.conn.Close()
			return
		}
	}
	// 补发完成（含空补发的已追平形态）必发同步完成标记：客户端据此把补发
	// 窗口内的缓冲帧按序落地并恢复实时分发；标记无 seq，不推进序号基准
	h.sendControlFrame(c, wsSyncedEvent, current)
}

// sendControlFrame 向单客户端投递一条无序号控制帧（ws-gap / ws-synced）。
// 投递失败（队列满）同补发路径：关连接终止，客户端重连恢复。
func (h *WSHub) sendControlFrame(c *wsClient, event string, current uint64) {
	frame, err := json.Marshal(wsFrame{Event: event, Data: []any{current}})
	if err != nil {
		// 固定字段载荷不会失败，防御保留
		slog.Error("marshal ws control frame failed", "event", event, "err", err)
		return
	}
	select {
	case c.send <- frame:
	default:
		_ = c.conn.Close()
	}
}

// writePump 写泵：独占本连接全部写操作（事件帧 + 协议 ping），单连接写
// 串行化的执行端。send 关闭（已注销）时发送 Close 帧后退出；读到 nil 哨兵
//（DisconnectClient 踢出收口）时同样发送 Close 帧后退出——此刻队列中先于
// 哨兵的帧（含 ws-kicked 踢出帧）均已按 FIFO 写出，被踢端先收到踢出提示帧
// 再感知连接关闭（单写者内保序）。
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
			if frame == nil {
				// 踢出收口哨兵（DisconnectClient 入队）：发送 Close 帧后收尾，
				// 随后 readPump 感知断连走既有 unregister
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
