package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialWS 以指定选项连接测试服务器的 /ws 端点，返回连接与升级响应（失败时
// conn 为 nil，resp 携带 HTTP 拒绝状态码）。
func dialWS(t *testing.T, srv *httptest.Server, query string, subprotocols []string) (*websocket.Conn, *http.Response) {
	t.Helper()
	url := "ws://" + srv.Listener.Addr().String() + wsPath + query
	dialer := &websocket.Dialer{Subprotocols: subprotocols, HandshakeTimeout: 5 * time.Second}
	conn, resp, dialErr := dialer.Dial(url, nil)
	if dialErr != nil && resp == nil {
		t.Fatalf("Dial 失败且无响应可断言: %v", dialErr)
	}
	return conn, resp
}

// mustDialWS 以正确令牌（查询参数通道）建立连接，失败即 Fatal。
func mustDialWS(t *testing.T, srv *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	conn, resp := dialWS(t, srv, query, nil)
	if conn == nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("升级应成功, got err with status %d", status)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readFrame 从连接读取一条事件帧（带读超时），解析后返回。
func readFrame(t *testing.T, conn *websocket.Conn) wsFrame {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取事件帧失败: %v", err)
	}
	var frame wsFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		t.Fatalf("事件帧不是合法 JSON: %v, payload=%q", err, payload)
	}
	return frame
}

// readBusinessFrame 读下一帧业务事件帧（自动跳过连接数事件帧）：连接/断开
// 产生的 ws-connections:changed 计数快照对业务断言无意义；每次连接/断开至多
// 各产生一条，帧数有界不会死循环。
func readBusinessFrame(t *testing.T, conn *websocket.Conn) wsFrame {
	t.Helper()
	for {
		frame := readFrame(t, conn)
		if frame.Event != wsConnectionsEvent {
			return frame
		}
	}
}

// waitClientCount 轮询等待客户端数达到期望值（连接注册与断连注销均为异步）。
func waitClientCount(t *testing.T, h *WSHub, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if h.ClientCount() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待超时: ClientCount = %d, want %d", h.ClientCount(), want)
}

// newTestWSHub 起一个挂载 hub 的 httptest 服务器，返回 hub 与服务器。
// 服务器须在所有连接关闭后再关闭（httptest.Close 等待 hijacked 连接），
// 调用方以 defer 顺序保证：先 defer srv.Close()，连接关闭交由 Cleanup。
func newTestWSHub(t *testing.T, token string) (*WSHub, *httptest.Server) {
	t.Helper()
	hub := NewWSHub(StaticVerifier(token), nil, nil)
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)
	return hub, srv
}

// TestWSHub_UnauthorizedMatrix 升级前认证矩阵：无 token / 错 token（查询参数
// 与子协议两通道）均以 HTTP 401 拒绝，连接不建立（HTTP 层拒绝，勿先升级后踢）。
func TestWSHub_UnauthorizedMatrix(t *testing.T) {
	_, srv := newTestWSHub(t, "right-token")

	cases := []struct {
		name  string
		query string
		subs  []string
	}{
		{"无令牌", "", nil},
		{"查询参数错令牌", "?token=wrong-token", nil},
		{"子协议错令牌", "", []string{"wrong-token"}},
		{"子协议仅前缀匹配", "", []string{"right"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn, resp := dialWS(t, srv, tc.query, tc.subs)
			if conn != nil {
				t.Fatal("未授权升级应失败")
			}
			if resp == nil || resp.StatusCode != http.StatusUnauthorized {
				status := 0
				if resp != nil {
					status = resp.StatusCode
				}
				t.Fatalf("应 HTTP 401 拒绝升级, got %d", status)
			}
			if h := resp.Header.Get("Sec-WebSocket-Protocol"); h != "" {
				t.Errorf("拒绝升级不应回显子协议, got %q", h)
			}
		})
	}
}

// TestWSHub_EmptyTokenFailClosed 服务端令牌未配置（空串）时一切升级请求拒绝。
func TestWSHub_EmptyTokenFailClosed(t *testing.T) {
	_, srv := newTestWSHub(t, "")

	conn, resp := dialWS(t, srv, "?token=whatever", []string{"whatever"})
	if conn != nil {
		t.Fatal("空令牌配置应拒绝一切升级")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("应 HTTP 401, got %v", resp)
	}
}

// TestWSHub_AuthorizedQueryToken 查询参数通道：正确 token 升级成功并收到广播。
func TestWSHub_AuthorizedQueryToken(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)

	hub.Emit("hello", "payload-1", 2)
	frame := readBusinessFrame(t, conn)
	if frame.Event != "hello" {
		t.Errorf("事件名不符: got %q", frame.Event)
	}
	if len(frame.Data) != 2 || frame.Data[0] != "payload-1" || frame.Data[1] != float64(2) {
		t.Errorf("载荷不符: got %#v", frame.Data)
	}
}

// TestWSHub_AuthorizedSubprotocol 子协议通道（浏览器 shim 主通道）：token 以
// Sec-WebSocket-Protocol 携带，升级响应回显选中子协议（缺失时浏览器判定
// 握手失败关闭连接），后续广播正常送达。
func TestWSHub_AuthorizedSubprotocol(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn, _ := dialWS(t, srv, "", []string{"right-token"})
	if conn == nil {
		t.Fatal("子协议通道升级应成功")
	}
	t.Cleanup(func() { _ = conn.Close() })
	if got := conn.Subprotocol(); got != "right-token" {
		t.Fatalf("升级响应应回显选中的子协议, got %q", got)
	}
	waitClientCount(t, hub, 1)

	hub.Emit("push", true)
	frame := readBusinessFrame(t, conn)
	if frame.Event != "push" || frame.Data[0] != true {
		t.Errorf("事件帧不符: got %#v", frame)
	}
}

// TestWSHub_CookieAuthChannel cookie 会话通道：浏览器裸连 WebSocket（无
// protocols 参数）时首访 HTTP 请求种下的 wb_token cookie 随同源握手自动携带。
// 有效 cookie 升级成功收帧；错误 cookie 升级前 401 拒绝。
func TestWSHub_CookieAuthChannel(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")

	// 有效 cookie：裸连（无子协议、无查询参数）升级成功并收到广播
	header := http.Header{}
	header.Set("Cookie", cookieName+"=right-token")
	url := "ws://" + srv.Listener.Addr().String() + wsPath
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).Dial(url, header)
	if err != nil {
		t.Fatalf("cookie 通道升级应成功: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	waitClientCount(t, hub, 1)

	hub.Emit("push", "via-cookie")
	frame := readBusinessFrame(t, conn)
	if frame.Event != "push" || frame.Data[0] != "via-cookie" {
		t.Errorf("事件帧不符: got %#v", frame)
	}

	// 错误 cookie：升级前 401 拒绝
	badHeader := http.Header{}
	badHeader.Set("Cookie", cookieName+"=wrong-token")
	badConn, resp, _ := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).Dial(url, badHeader)
	if badConn != nil {
		t.Fatal("错误 cookie 升级应失败")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("错误 cookie 应 HTTP 401 拒绝升级, got %d", status)
	}
}

// TestWSHub_FrameFormat 帧格式为 {"event":"<事件名>","data":<变参数组>}，
// 与 Wails EventsOn 回调 ...data 形态对齐（shim 以 callback(...frame.data) 还原）。
func TestWSHub_FrameFormat(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)

	hub.Emit("terminal-output", "term-1", "out")
	frame := readBusinessFrame(t, conn)
	if frame.Event != "terminal-output" || frame.Data[0] != "term-1" || frame.Data[1] != "out" {
		t.Errorf("帧字段不符: got %#v", frame)
	}

	// 零载荷事件 data 序列化为空数组（非 null），shim 展开为空变参
	hub.Emit("no-payload")
	frame = readBusinessFrame(t, conn)
	raw, _ := json.Marshal(frame)
	if !strings.Contains(string(raw), `"data":[]`) {
		t.Errorf("零载荷 data 应为空数组: %s", raw)
	}
}

// TestWSHub_BroadcastToAllClients 广播语义：一条事件推给全部已连接客户端。
func TestWSHub_BroadcastToAllClients(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conns := []*websocket.Conn{
		mustDialWS(t, srv, "?token=right-token"),
		mustDialWS(t, srv, "?token=right-token"),
	}
	waitClientCount(t, hub, 2)

	hub.Emit("event-1", "payload-1", 2)
	for i, conn := range conns {
		frame := readBusinessFrame(t, conn)
		if frame.Event != "event-1" || frame.Data[0] != "payload-1" {
			t.Errorf("客户端 %d 收到的帧不符: %#v", i, frame)
		}
	}
}

// TestWSHub_DisconnectCleanup 客户端断连后自动注销，广播不再投递给已断连接。
func TestWSHub_DisconnectCleanup(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)

	_ = conn.Close()
	waitClientCount(t, hub, 0)

	// 注销后广播不应 panic 或投递错误（clients 已不含该连接）
	hub.Emit("after-disconnect", "x")
}

// TestWSHub_ConcurrentEmitAndDisconnectRace 并发安全（-race 抽查）：多
// goroutine 持续广播与客户端反复连接/断开交错，无竞态、无 panic。
func TestWSHub_ConcurrentEmitAndDisconnectRace(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")

	// 客户端反复建连/断开，制造注册/注销与广播的交错窗口
	stop := make(chan struct{})
	var cliWG sync.WaitGroup
	for i := 0; i < 3; i++ {
		cliWG.Add(1)
		go func() {
			defer cliWG.Done()
			dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
			for {
				select {
				case <-stop:
					return
				default:
				}
				conn, _, err := dialer.Dial("ws://"+srv.Listener.Addr().String()+wsPath+"?token=right-token", nil)
				if err != nil {
					continue
				}
				_ = conn.Close()
			}
		}()
	}

	// 广播方并发 Emit
	var emitWG sync.WaitGroup
	for i := 0; i < 4; i++ {
		emitWG.Add(1)
		go func(n int) {
			defer emitWG.Done()
			for j := 0; j < 200; j++ {
				hub.Emit("race", n, j)
			}
		}(i)
	}
	emitWG.Wait()
	close(stop)
	cliWG.Wait()

	waitClientCount(t, hub, 0)
}

// TestWSHub_VerifierSwapRotation 校验基准热更替：替换 verify（对应多 token
// 语义下 RegenerateWebToken 撤旧建新，服务层状态经 verify 闭包即时生效）后，
// 旧令牌两通道均 401、新令牌均升级成功且子协议正确回显（多 token 下回显命中
// 的候选值）；更替前建立的在途连接不在数据面校验令牌，保持有效并继续收到广播
// （热生效语义：不断监听、不断在途连接）。
func TestWSHub_VerifierSwapRotation(t *testing.T) {
	sv := &swapVerifier{}
	sv.set(StaticVerifier("old-token"))
	hub := NewWSHub(sv.verify, nil, nil)
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	// 更替前建立在途连接（旧令牌时期）
	existing := mustDialWS(t, srv, "?token=old-token")
	waitClientCount(t, hub, 1)

	sv.set(StaticVerifier("new-token"))

	// 旧令牌两通道均拒绝
	if conn, resp := dialWS(t, srv, "?token=old-token", nil); conn != nil {
		t.Fatal("轮换后旧令牌查询参数通道应拒绝")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("轮换后旧令牌查询参数通道应 401, got %v", resp)
	}
	if conn, resp := dialWS(t, srv, "", []string{"old-token"}); conn != nil {
		t.Fatal("轮换后旧令牌子协议通道应拒绝")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("轮换后旧令牌子协议通道应 401, got %v", resp)
	}

	// 新令牌两通道均可升级
	mustDialWS(t, srv, "?token=new-token")
	waitClientCount(t, hub, 2)
	connNewSub, _ := dialWS(t, srv, "", []string{"new-token"})
	if connNewSub == nil {
		t.Fatal("轮换后新令牌子协议通道应升级成功")
	}
	t.Cleanup(func() { _ = connNewSub.Close() })
	waitClientCount(t, hub, 3)

	// 在途连接（旧令牌时期建立）保持有效并收到广播
	hub.Emit("after-rotate", "x")
	frame := readBusinessFrame(t, existing)
	if frame.Event != "after-rotate" {
		t.Errorf("在途连接应继续收到广播, got %q", frame.Event)
	}
}

// TestWSHub_OnAuthEvents 握手认证事件回调：成功/失败各触发一次，事件字段完整
// （Event/TokenID/RemoteAddr/UserAgent/At）；OnAuth 未注入（nil）时认证链路
// 正常不 panic。
func TestWSHub_OnAuthEvents(t *testing.T) {
	var mu sync.Mutex
	var events []AuthEvent
	hub := NewWSHub(StaticVerifier("right-token"), func(ev AuthEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}, nil)
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	if conn, resp := dialWS(t, srv, "?token=wrong-token", nil); conn != nil {
		t.Fatal("错令牌升级应失败")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错令牌应 401, got %v", resp)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 {
		t.Fatalf("应触发恰好两次认证事件, got %d", len(events))
	}
	if events[0].Event != AuthEventSuccess || events[0].TokenID != "static" {
		t.Errorf("成功事件不符: %+v", events[0])
	}
	if events[1].Event != AuthEventFailure || events[1].TokenID != "" {
		t.Errorf("失败事件不符: %+v", events[1])
	}
	for _, ev := range events {
		if ev.RemoteAddr == "" || ev.At.IsZero() {
			t.Errorf("事件字段不完整: %+v", ev)
		}
	}
}

// TestWSHub_CloseDisconnectsAllClients hub 停机关闭：Close 后全部在途客户端
// 读侧感知连接关闭、readPump 走 unregister 收尾清空 clients；Close 幂等（二次
// 调用不 panic），Close 后 Emit 不 panic（对应 webServeManager.stopLocked 的
// 停机关闭路径，防僵尸客户端与 pump goroutine 泄漏）。
func TestWSHub_CloseDisconnectsAllClients(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conns := []*websocket.Conn{
		mustDialWS(t, srv, "?token=right-token"),
		mustDialWS(t, srv, "?token=right-token"),
	}
	waitClientCount(t, hub, 2)

	hub.Close()

	// 各客户端读侧感知服务端关闭：先排空 Close 前已入队的帧（连接建立期的
	// 计数事件），随后必读到错误（send 队列 close → Close 帧/连接断开）
	for _, conn := range conns {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}
	// readPump 感知断连后 unregister，clients 清空（goroutine 收尾完成）
	waitClientCount(t, hub, 0)

	// 幂等：二次 Close 不 panic（clients 已空，遍历为空操作）
	hub.Close()

	// Close 后广播不 panic：残留投递目标已注销，空投递安全
	hub.Emit("after-close", "x")
}

// TestWSHub_SlowConsumerDropped 写超时/背压防护：客户端不读帧，发送队列
// 与 TCP 缓冲塞满后写超时触发断连，慢消费者被踢出（不拖垮广播方与其余客户端）。
func TestWSHub_SlowConsumerDropped(t *testing.T) {
	// 收紧写超时加速断连判定（hub 实例字段，构造后只读，无跨 goroutine 竞态）
	hub := newWSHubTuned(StaticVerifier("right-token"), nil, nil, 200*time.Millisecond, defaultPongWait, wsResumeBuffer)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	conn := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)

	// 不读取 conn，持续广播大帧填满发送队列与 TCP 接收缓冲
	bulk := strings.Repeat("x", 8192)
	deadline := time.Now().Add(10 * time.Second)
	for hub.ClientCount() > 0 && time.Now().Before(deadline) {
		hub.Emit("bulk", bulk)
		time.Sleep(10 * time.Millisecond)
	}
	if hub.ClientCount() != 0 {
		t.Fatal("慢消费者未被踢出")
	}

	// 被踢出的连接读侧应以错误收场（服务端已关闭）
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	// 广播方未被拖垮：踢出后新客户端照常可用
	mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
}

// sendResume 向服务端发送重连续传握手帧 {"resume":<lastSeq>}。
func sendResume(t *testing.T, conn *websocket.Conn, lastSeq uint64) {
	t.Helper()
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"resume":`+strconv.FormatUint(lastSeq, 10)+`}`)); err != nil {
		t.Fatalf("发送 resume 握手失败: %v", err)
	}
}

// TestWSHub_EmitSeqMonotonicShared 序号契约：每条广播帧带 hub 级单调序号，
// 多客户端收到的同一条事件帧序号一致（帧共享同一份序列化字节）。
func TestWSHub_EmitSeqMonotonicShared(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conns := []*websocket.Conn{
		mustDialWS(t, srv, "?token=right-token"),
		mustDialWS(t, srv, "?token=right-token"),
	}
	waitClientCount(t, hub, 2)

	for i := 0; i < 3; i++ {
		hub.Emit("e", i)
	}
	// 基线 = 首条业务帧序号（各连接的 register 计数帧已先占号，业务帧自基线
	// 起连续推进；两客户端共享同一序列化帧，序号应一致）
	base := uint64(0)
	for i, conn := range conns {
		for j := 0; j < 3; j++ {
			frame := readBusinessFrame(t, conn)
			if base == 0 {
				base = frame.Seq
			}
			if frame.Event != "e" || frame.Seq != base+uint64(j) {
				t.Errorf("客户端 %d 第 %d 帧不符: event=%q seq=%d, want seq=%d", i, j, frame.Event, frame.Seq, base+uint64(j))
			}
		}
	}
}

// TestWSHub_ResumeFillExact 重连精确补发：断线期间的缺口帧按序补发，
// 无缺失无重复，补发完成后收到 ws-synced 同步完成标记。
func TestWSHub_ResumeFillExact(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")

	// 第一段连接收 seq1..3 后断线
	// 第一段连接收 3 条业务帧后断线（各连接的 register 计数帧先占号，
	// 业务帧序号自基线 base 起连续推进，下同）
	conn1 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	base := uint64(0)
	for i := 1; i <= 3; i++ {
		hub.Emit("e", i)
		frame := readBusinessFrame(t, conn1)
		if base == 0 {
			base = frame.Seq - 1
		}
		if frame.Seq != base+uint64(i) {
			t.Fatalf("第 %d 帧序号不符: %d (base=%d)", i, frame.Seq, base)
		}
	}
	_ = conn1.Close()
	waitClientCount(t, hub, 0)

	// 断线期间产生 2 帧缺口
	hub.Emit("e", 4)
	hub.Emit("e", 5)

	// 重连握手 resume=base+3：补发 2 帧缺口 + ws-synced 标记
	conn2 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	sendResume(t, conn2, base+3)
	for i := 4; i <= 5; i++ {
		frame := readBusinessFrame(t, conn2)
		if frame.Event != "e" || frame.Seq != base+uint64(i) {
			t.Errorf("补发帧应为 e/seq%d, got event=%q seq=%d", base+uint64(i), frame.Event, frame.Seq)
		}
	}
	synced := readBusinessFrame(t, conn2)
	if synced.Event != wsSyncedEvent {
		t.Fatalf("补发后应收到同步完成标记, got %q", synced.Event)
	}
	if len(synced.Data) != 1 || synced.Data[0] != float64(base+6) {
		// 当前序号含 conn2 register 计数帧（在补发计算前占号）
		t.Errorf("ws-synced data[0] 应为服务端当前序号 %d, got %#v", base+6, synced.Data)
	}
	// 补发不影响后续实时广播（conn2 的 register 计数帧占 synced 与实时帧间的序号）
	hub.Emit("e", 6)
	if frame := readBusinessFrame(t, conn2); frame.Event != "e" || frame.Seq != base+7 {
		t.Errorf("补发后实时帧 seq 应为 %d, got event=%q seq=%d", base+7, frame.Event, frame.Seq)
	}
}

// TestWSHub_ResumeCaughtUp 追平形态：resume 序号等于服务端当前序号时无帧
// 可补，仅发 ws-synced 标记（客户端凭该标记结束同步模式）。
func TestWSHub_ResumeCaughtUp(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)

	hub.Emit("e", 1)
	lastSeq := readBusinessFrame(t, conn).Seq
	sendResume(t, conn, lastSeq)

	synced := readBusinessFrame(t, conn)
	if synced.Event != wsSyncedEvent || len(synced.Data) != 1 || synced.Data[0] != float64(lastSeq) {
		t.Fatalf("追平形态应仅收到 ws-synced[%d], got %#v", lastSeq, synced)
	}
}

// TestWSHub_FreshConnectionNoReplay 新连接不重放历史：连接前产生的事件帧
// 不补发，连接后从当前序号起正常接收。
func TestWSHub_FreshConnectionNoReplay(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	hub.Emit("history", 1)
	hub.Emit("history", 2)

	conn := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	hub.Emit("live", 3)
	frame := readBusinessFrame(t, conn)
	// 序号 4：连接前两条历史帧占 seq1..2，本连接 register 计数帧占 seq3
	if frame.Event != "live" || frame.Seq != 4 {
		t.Errorf("新连接首帧应为 live/seq4（不重放历史）, got event=%q seq=%d", frame.Event, frame.Seq)
	}
}

// TestWSHub_ResumeGapOnOverflow 缓冲溢出断层：续传缓冲容量收紧为 4，客户端
// 断线期间产生超量事件致缺口帧被覆写，重连握手返回 ws-gap 断层标记帧
// （data[0] 为服务端当前序号），此后广播从当前序号正常继续。
func TestWSHub_ResumeGapOnOverflow(t *testing.T) {
	hub := newWSHubTuned(StaticVerifier("right-token"), nil, nil, defaultWriteWait, defaultPongWait, 4)
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	conn1 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	hub.Emit("e", 1)
	base := readBusinessFrame(t, conn1).Seq - 1 // 基线：register 计数帧占 seq base+1
	_ = conn1.Close()
	waitClientCount(t, hub, 0)

	// 断线期间 10 条事件（seq base+2..base+11），容量 4 的环形缓冲仅保留末 4 帧
	for i := 2; i <= 11; i++ {
		hub.Emit("e", i)
	}

	conn2 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	sendResume(t, conn2, base+1)
	gap := readBusinessFrame(t, conn2)
	if gap.Event != wsGapEvent {
		t.Fatalf("溢出应返回断层标记帧, got %q", gap.Event)
	}
	if len(gap.Data) != 1 || gap.Data[0] != float64(base+12) {
		// 当前序号含 conn2 register 计数帧（在补发计算前占号）
		t.Errorf("ws-gap data[0] 应为服务端当前序号 %d, got %#v", base+12, gap.Data)
	}
	// 断层后广播从当前序号正常继续（conn2 的 register 计数帧占 gap 后首个序号）
	hub.Emit("e", 12)
	if frame := readBusinessFrame(t, conn2); frame.Event != "e" || frame.Seq != base+13 {
		t.Errorf("断层后实时帧应为 e/seq%d, got event=%q seq=%d", base+13, frame.Event, frame.Seq)
	}
}

// TestWSHub_ResumeGapOnHubRestart hub 重建断层：webServeManager 重启重建 hub
// 使序号归零，客户端携带旧代际序号（超前于新 hub 当前序号）重连，返回
// ws-gap 标记帧；此后新代际序号正常推进。
func TestWSHub_ResumeGapOnHubRestart(t *testing.T) {
	hub1, srv1 := newTestWSHub(t, "right-token")
	conn1 := mustDialWS(t, srv1, "?token=right-token")
	waitClientCount(t, hub1, 1)
	for i := 1; i <= 3; i++ {
		hub1.Emit("e", i)
		readBusinessFrame(t, conn1)
	}
	_ = conn1.Close()
	waitClientCount(t, hub1, 0)
	// srv1.Close 等待 hijacked 连接收尾，先行关闭让位新服务器
	srv1.Close()

	// hub 重建（新实例、序号从 0 起），期间已产生 seq1
	hub2, srv2 := newTestWSHub(t, "right-token")
	hub2.Emit("e", "post-restart")

	conn2 := mustDialWS(t, srv2, "?token=right-token")
	waitClientCount(t, hub2, 1)
	sendResume(t, conn2, 3)
	gap := readBusinessFrame(t, conn2)
	if gap.Event != wsGapEvent {
		t.Fatalf("跨代际重连应返回断层标记帧, got %q", gap.Event)
	}
	if len(gap.Data) != 1 || gap.Data[0] != float64(2) {
		// 当前序号含 conn2 register 计数帧（hub2 首条事件占 seq1，计数帧占 seq2）
		t.Errorf("ws-gap data[0] 应为新 hub 当前序号 2, got %#v", gap.Data)
	}
	hub2.Emit("e", "after-gap")
	// 断层标记后：gap data[0]=1，conn2 的 register 计数帧占 seq2，实时帧 seq3
	if frame := readBusinessFrame(t, conn2); frame.Event != "e" || frame.Seq != 3 {
		t.Errorf("断层后实时帧应为 e/seq3, got event=%q seq=%d", frame.Event, frame.Seq)
	}
}

// TestWSHub_ClientFrameGarbageIgnored 非协议上行帧静默忽略：连接保持存活，
// 后续广播照常送达（resume 握手是唯一业务上行，异常帧不响应不记日志）。
func TestWSHub_ClientFrameGarbageIgnored(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)

	for _, payload := range []string{`not json`, `{"other":1}`, `{"resume":"NaN"}`, `[1,2]`, `{"resume":-1}`} {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
			t.Fatalf("写入测试帧失败: %v", err)
		}
	}
	hub.Emit("still-alive", "x")
	if frame := readBusinessFrame(t, conn); frame.Event != "still-alive" {
		t.Errorf("异常上行帧后连接应存活且广播可达, got %q", frame.Event)
	}
}

// TestWSHub_ResumeConcurrentWithEmitRace 并发安全（-race 抽查）：持续广播与
// 反复重连握手（resume 补发）交错，无竞态、无 panic；收到的补发/实时帧序号
// 单调不回退（重复序号合法——注册窗口广播与补发集经同一 FIFO 队列交叠，
// 由客户端按序号去重）。
func TestWSHub_ResumeConcurrentWithEmitRace(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")

	stop := make(chan struct{})
	var emitWG sync.WaitGroup
	emitWG.Add(1)
	go func() {
		defer emitWG.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			hub.Emit("race", i)
		}
	}()

	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	for round := 0; round < 5; round++ {
		conn, _, err := dialer.Dial("ws://"+srv.Listener.Addr().String()+wsPath+"?token=right-token", nil)
		if err != nil {
			continue
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var lastSeen uint64
		for i := 0; i < 20; i++ {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				break
			}
			var frame wsFrame
			if json.Unmarshal(payload, &frame) != nil {
				continue
			}
			if frame.Seq > 0 && frame.Seq < lastSeen {
				t.Fatalf("序号回退: lastSeen=%d, got %d", lastSeen, frame.Seq)
			}
			if frame.Seq > 0 {
				lastSeen = frame.Seq
			}
		}
		// 以当前已收序号发起续传握手后立即断开，制造补发与广播的交错窗口
		sendResume(t, conn, lastSeen)
		_ = conn.Close()
	}
	close(stop)
	emitWG.Wait()
}

// frameDataCount 提取连接数事件 data[0] 的计数值（JSON 反序列化后为 float64，
// 转回 int 比较；非数值形态返回 ok=false）。
func frameDataCount(frame wsFrame) (int, bool) {
	if len(frame.Data) != 1 {
		return 0, false
	}
	n, ok := frame.Data[0].(float64)
	return int(n), ok
}

// TestWSHub_ConnectionCountEvents 连接数变化事件（多设备语义明示）：每次
// register/unregister 成功路径向全部客户端广播 ws-connections:changed，
// data[0] 为事件后的连接总数；事件经常规 Emit 进环形缓冲占用 seq（续传
// 契约不变）。
func TestWSHub_ConnectionCountEvents(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")

	// 首个连接：自身收到 count=1
	connA := mustDialWS(t, srv, "?token=right-token")
	frame := readFrame(t, connA)
	if frame.Event != wsConnectionsEvent {
		t.Fatalf("首个连接应广播连接数事件, got %q", frame.Event)
	}
	if n, ok := frameDataCount(frame); !ok || n != 1 {
		t.Fatalf("data[0] 应为事件后连接总数 1, got %v", frame.Data)
	}
	if frame.Seq == 0 {
		t.Fatal("连接数事件应占用序号（进环形缓冲，resume 补发面包含该事件）")
	}

	// 第二个连接：两个客户端均收到 count=2
	connB := mustDialWS(t, srv, "?token=right-token")
	for label, conn := range map[string]*websocket.Conn{"A": connA, "B": connB} {
		frame := readFrame(t, conn)
		if n, ok := frameDataCount(frame); !ok || frame.Event != wsConnectionsEvent || n != 2 {
			t.Fatalf("客户端 %s 应收到 count=2 事件, got event=%q data=%v", label, frame.Event, frame.Data)
		}
	}

	// A 断开：B 收到 count=1（异步注销，先等计数再读帧）
	_ = connA.Close()
	waitClientCount(t, hub, 1)
	frame = readFrame(t, connB)
	if n, ok := frameDataCount(frame); !ok || frame.Event != wsConnectionsEvent || n != 1 {
		t.Fatalf("断开后应广播 count=1 事件, got event=%q data=%v", frame.Event, frame.Data)
	}
}

// TestWSHub_CloseSuppressesConnectionEvents hub.Close 收口期不再广播连接数
// 事件：逐连接 unregister 的递减序列对前端无意义（连接即将随服务停机断开）。
func TestWSHub_CloseSuppressesConnectionEvents(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")

	conn := mustDialWS(t, srv, "?token=right-token")
	frame := readFrame(t, conn)
	if n, ok := frameDataCount(frame); !ok || frame.Event != wsConnectionsEvent || n != 1 {
		t.Fatalf("前置：应先收到 count=1 事件, got event=%q data=%v", frame.Event, frame.Data)
	}

	hub.Close()
	// 收口期连接被关闭：读循环读到错误（或 Close 帧）为止，期间不得再出现
	// 任何连接数事件帧
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var f wsFrame
		if json.Unmarshal(payload, &f) != nil {
			continue
		}
		if f.Event == wsConnectionsEvent {
			t.Fatalf("Close 收口期不应广播连接数事件, got data=%v", f.Data)
		}
	}
	waitClientCount(t, hub, 0)
}

// TestWSHub_OnConnectionsChangeCallback 桌面连接明细快照回调（SetOnConnectionsChange）：
// register/unregister 出 mu 写锁后触发，全量明细透传——含最后一个浏览器断开的空
// 快照（桌面据此熄指示并清空明细列表）。回调与 hub 内部 WS 广播是两条独立出口：
// WS 广播侧对 count=0 的抑制不影响回调透传，且广播载荷恒为 count 数字（隐私红线：
// 他人 IP/token 明细不进浏览器通道）。
//
// 快照内容断言与触发者无关：快照为 hub 级全量状态，两次注册必然先后产生 1、2 条
// 明细，两次断开必然先后产生 1、0 条（与哪条连接的 readPump 先感知无关），channel
// 收值顺序确定，无时序依赖。连接 ID 按 dial 顺序单调分配（ServeHTTP 串行处理升级，
// dial 返回即 Upgrade 完成后），快照按 ID 升序即接入顺序。
func TestWSHub_OnConnectionsChangeCallback(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	callbacks := make(chan []ConnDetail, 8)
	hub.SetOnConnectionsChange(func(details []ConnDetail) { callbacks <- details })

	connA := mustDialWS(t, srv, "?token=right-token")
	first := <-callbacks
	if len(first) != 1 {
		t.Fatalf("首个连接回调应为 1 条明细, got %d", len(first))
	}
	// 元数据字段断言：ID 单调分配、IP 剥端口（httptest 客户端恒 127.0.0.1）、
	// tokenID 为命中记录 ID（StaticVerifier 伪 ID "static"）、接入时间非零值
	d := first[0]
	if d.ID != 1 {
		t.Fatalf("首个连接 ID 应为 1, got %d", d.ID)
	}
	if d.IP != "127.0.0.1" {
		t.Fatalf("IP 应剥端口为 127.0.0.1, got %q", d.IP)
	}
	if d.TokenID != "static" {
		t.Fatalf("TokenID 应为 StaticVerifier 伪 ID static, got %q", d.TokenID)
	}
	if d.ConnectedAt.IsZero() {
		t.Fatal("接入时间不应为零值")
	}
	if d.TokenName != "" {
		t.Fatalf("server 侧 TokenName 应恒空（装配层映射）, got %q", d.TokenName)
	}

	connB := mustDialWS(t, srv, "?token=right-token")
	second := <-callbacks
	if len(second) != 2 {
		t.Fatalf("第二个连接回调应为 2 条明细, got %d", len(second))
	}
	// 快照按 ID 升序（接入顺序），载荷顺序确定
	if second[0].ID != 1 || second[1].ID != 2 {
		t.Fatalf("快照应按连接 ID 升序, got [%d, %d]", second[0].ID, second[1].ID)
	}

	// A 断开：回调剩 B（ID=2）；B 断开：空快照（关键：空快照须透传给桌面熄指示
	// 并清空明细列表）
	_ = connA.Close()
	afterClose := <-callbacks
	if len(afterClose) != 1 || afterClose[0].ID != 2 {
		t.Fatalf("第一个断开后应剩连接 ID=2 一条明细, got %v", afterClose)
	}
	_ = connB.Close()
	empty := <-callbacks
	if empty == nil || len(empty) != 0 {
		t.Fatalf("最后一个浏览器断开回调应为空快照（非 nil）, got %v", empty)
	}
	waitClientCount(t, hub, 0)

	// 隐私红线：浏览器通道载荷恒为 count 数字（int），不含明细
	if n, ok := frameDataCount(readFrame(t, mustDialWS(t, srv, "?token=right-token"))); !ok || n != 1 {
		t.Fatalf("浏览器通道广播载荷应为 count 数字, got n=%d ok=%v", n, ok)
	}
}

// TestWSHub_CloseSuppressesOnConnectionsChangeCallback hub.Close 收口期回调与 WS
// 广播同受 closing 抑制：逐连接 unregister 不再触发回调。manager 停机路径据契约
// 在 hub.Close 后经 Wails 出口显式补发空明细快照（web_serve.go stopLocked），
// hub 侧不重复发。
func TestWSHub_CloseSuppressesOnConnectionsChangeCallback(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	callbacks := make(chan []ConnDetail, 8)
	hub.SetOnConnectionsChange(func(details []ConnDetail) { callbacks <- details })

	conn := mustDialWS(t, srv, "?token=right-token")
	if got := <-callbacks; len(got) != 1 {
		t.Fatalf("前置：register 回调应为 1 条明细, got %d", len(got))
	}
	hub.Close()
	waitClientCount(t, hub, 0)
	select {
	case got := <-callbacks:
		t.Fatalf("Close 收口期不应触发连接明细回调, got %v", got)
	case <-time.After(100 * time.Millisecond):
	}
	_ = conn.Close()
}

// TestWSHub_OnConnectionsChangeNilSafe 未设置回调（nil）时 register/unregister 正常：
// 连接数 WS 广播路径零波及（既有用例均未设置回调，此处显式钉住 nil 安全契约）。
func TestWSHub_OnConnectionsChangeNilSafe(t *testing.T) {
	_, srv := newTestWSHub(t, "right-token")
	conn := mustDialWS(t, srv, "?token=right-token")
	frame := readFrame(t, conn)
	if n, ok := frameDataCount(frame); !ok || frame.Event != wsConnectionsEvent || n != 1 {
		t.Fatalf("未设置回调时连接数广播应照常, got event=%q data=%v", frame.Event, frame.Data)
	}
}

// ===== DisconnectClient（连接管控：按 ID 踢出单个在途连接）=====

// TestWSHub_DisconnectClientHit 踢出命中：目标连接读侧感知断开（ws-kicked 帧
// 送达后连接关闭），明细回调自动更新（零新增广播事件——unregister 既有收尾路径
// 触发快照双出口），其余连接收到 ws-connections:changed count 回落广播；命中返回
// 被踢连接元数据快照（审计留痕数据源）。
func TestWSHub_DisconnectClientHit(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	callbacks := make(chan []ConnDetail, 8)
	hub.SetOnConnectionsChange(func(details []ConnDetail) { callbacks <- details })

	connA := mustDialWS(t, srv, "?token=right-token")
	connB := mustDialWS(t, srv, "?token=right-token")
	first := <-callbacks
	if len(first) != 1 {
		t.Fatalf("前置：首个连接回调应为 1 条明细, got %d", len(first))
	}
	if got := <-callbacks; len(got) != 2 {
		t.Fatalf("前置：第二个连接回调应为 2 条明细, got %d", len(got))
	}

	detail, ok := hub.DisconnectClient(1)
	if !ok {
		t.Fatal("在册连接踢出应命中返 true")
	}
	// 返回的被踢连接元数据快照：与明细回调首条一致（审计留痕数据源）
	if detail != first[0] {
		t.Errorf("命中应返回被踢连接明细快照, got %+v, want %+v", detail, first[0])
	}

	// 被踢连接读侧感知断开：conn.Close 前已入队帧（注册窗口 count 广播）可先
	// 正常送达，循环读到错误为止（读超时 5s 兜底防永挂）
	_ = connA.SetReadDeadline(time.Now().Add(5 * time.Second))
	kickSeen := false
	for {
		if _, _, err := connA.ReadMessage(); err != nil {
			kickSeen = true
			break
		}
	}
	if !kickSeen {
		t.Error("被踢连接读侧应感知断开（ReadMessage 返回错误）")
	}

	// 明细回调自动更新：剩 B（ID=2）一条；行消失靠事件驱动（零新增事件）
	afterKick := <-callbacks
	if len(afterKick) != 1 || afterKick[0].ID != 2 {
		t.Fatalf("踢出后明细回调应剩 ID=2 一条, got %v", afterKick)
	}
	waitClientCount(t, hub, 1)

	// 其余连接收到 count 回落广播：B 先后收到注册窗口 count=2 与踢出后 count=1
	f1 := readFrame(t, connB)
	if n, ok := frameDataCount(f1); !ok || f1.Event != wsConnectionsEvent || n != 2 {
		t.Fatalf("B 首帧应为注册窗口 count=2, got event=%q data=%v", f1.Event, f1.Data)
	}
	f2 := readFrame(t, connB)
	if n, ok := frameDataCount(f2); !ok || f2.Event != wsConnectionsEvent || n != 1 {
		t.Fatalf("踢出后 B 应收到 count=1 广播, got event=%q data=%v", f2.Event, f2.Data)
	}

	_ = connB.Close()
	empty := <-callbacks
	if len(empty) != 0 {
		t.Fatalf("B 断开后回调应为空快照, got %v", empty)
	}
	waitClientCount(t, hub, 0)
}

// TestWSHub_DisconnectClientMissAndIdempotent miss 与幂等：不存在的 ID 返 false；
// 重复踢出第二次 miss（已注销连接不在 clients，天然幂等），不误伤其余连接。
func TestWSHub_DisconnectClientMissAndIdempotent(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	callbacks := make(chan []ConnDetail, 8)
	hub.SetOnConnectionsChange(func(details []ConnDetail) { callbacks <- details })

	connA := mustDialWS(t, srv, "?token=right-token")
	connB := mustDialWS(t, srv, "?token=right-token")
	<-callbacks
	<-callbacks

	if _, ok := hub.DisconnectClient(999); ok {
		t.Error("不存在的连接 ID 应 miss 返 false")
	}
	if _, ok := hub.DisconnectClient(1); !ok {
		t.Fatal("首连（ID=1）踢出应命中")
	}
	waitClientCount(t, hub, 1)
	if got := <-callbacks; len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("踢出后应剩 ID=2, got %v", got)
	}
	// 重复踢出同一 ID：幂等 miss，不产生第二次明细回调
	if _, ok := hub.DisconnectClient(1); ok {
		t.Error("重复踢出已注销连接应幂等 miss 返 false")
	}
	select {
	case got := <-callbacks:
		t.Fatalf("幂等 miss 不应触发明细回调, got %v", got)
	case <-time.After(100 * time.Millisecond):
	}
	// B 未被误伤：仍能收到广播
	hub.Emit("probe", "x")
	f := readBusinessFrame(t, connB)
	if f.Event != "probe" {
		t.Fatalf("未被踢连接应继续收到广播, got event=%q", f.Event)
	}
	_ = connA.Close()
	_ = connB.Close()
}

// TestWSHub_DisconnectClientConcurrentRace 踢出与并发断开/广播竞态（-race 形态）：
// 踢出方、客户端反复建连/断开、广播方三方并发，收口后 hub 归零且无 goroutine
// 泄漏症状（竞态由 -race 暴露，本用例钉住交错窗口不 panic、计数收敛）。
func TestWSHub_DisconnectClientConcurrentRace(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")

	// 客户端反复建连/断开（对齐 TestWSHub_ConcurrentEmitAndDisconnectRace 范式）
	stop := make(chan struct{})
	var cliWG sync.WaitGroup
	for i := 0; i < 3; i++ {
		cliWG.Add(1)
		go func() {
			defer cliWG.Done()
			dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
			for {
				select {
				case <-stop:
					return
				default:
				}
				conn, _, err := dialer.Dial("ws://"+srv.Listener.Addr().String()+wsPath+"?token=right-token", nil)
				if err != nil {
					continue
				}
				_ = conn.Close()
			}
		}()
	}

	// 踢出方并发按 ID 踢（ID 空间 1..200 覆盖连接分配区间，miss/命中混合）
	var kickWG sync.WaitGroup
	for i := 0; i < 2; i++ {
		kickWG.Add(1)
		go func(n int) {
			defer kickWG.Done()
			for id := uint64(1); id <= 200; id++ {
				if n%2 == 0 && id%3 == 0 {
					continue // 制造两踢出方节奏差异
				}
				hub.DisconnectClient(id)
			}
		}(i)
	}

	// 广播方并发 Emit
	var emitWG sync.WaitGroup
	for i := 0; i < 4; i++ {
		emitWG.Add(1)
		go func(n int) {
			defer emitWG.Done()
			for j := 0; j < 200; j++ {
				hub.Emit("race", n, j)
			}
		}(i)
	}
	emitWG.Wait()
	close(stop)
	kickWG.Wait()
	cliWG.Wait()

	waitClientCount(t, hub, 0)
}

// TestWSHub_DisconnectClientKickFrameBeforeClose 被踢单播帧时序（连接管控二期）：
// ws-kicked 帧先于连接关闭送达被踢端（writePump 单写者 FIFO 保序——踢出帧写出后
// 见 nil 哨兵才发 Close 帧退出）；踢出帧为无 seq 控制帧（不推进序号基准）且 data
// 为空数组（帧格式与广播事件一致）。
func TestWSHub_DisconnectClientKickFrameBeforeClose(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn := mustDialWS(t, srv, "?token=right-token")
	// 排空注册窗口计数帧，保证下一帧即踢出帧
	if frame := readFrame(t, conn); frame.Event != wsConnectionsEvent {
		t.Fatalf("前置：应先收到注册计数帧, got %q", frame.Event)
	}

	if _, ok := hub.DisconnectClient(1); !ok {
		t.Fatal("在册连接踢出应命中")
	}
	kick := readFrame(t, conn)
	if kick.Event != wsKickedEvent {
		t.Fatalf("踢出帧应先于连接关闭送达, got %q", kick.Event)
	}
	if kick.Seq != 0 {
		t.Errorf("踢出帧为控制帧不应携带 seq, got %d", kick.Seq)
	}
	if len(kick.Data) != 0 {
		t.Errorf("踢出帧 data 应为空数组, got %v", kick.Data)
	}

	// 踢出帧之后连接关闭（writePump 发 Close 帧退出 + readPump unregister 收尾）
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("踢出帧送达后连接应关闭（ReadMessage 返回错误）")
	}
	waitClientCount(t, hub, 0)
}

// TestWSHub_DisconnectClientUnicastIsolation 单播隔离：ws-kicked 管控帧仅送达
// 被踢连接，未被踢的其他连接收不到任何管控帧（只有既有连接数广播），后续广播
// 照常送达（单播不误伤）。
func TestWSHub_DisconnectClientUnicastIsolation(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	connA := mustDialWS(t, srv, "?token=right-token")
	connB := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 2)

	if _, ok := hub.DisconnectClient(1); !ok {
		t.Fatal("踢出 A 应命中")
	}

	// A：注册窗口计数帧之后到达踢出帧（逐帧读取直至命中，读超时兜底防永挂）
	_ = connA.SetReadDeadline(time.Now().Add(5 * time.Second))
	sawKicked := false
	for i := 0; i < 16 && !sawKicked; i++ {
		if readFrame(t, connA).Event == wsKickedEvent {
			sawKicked = true
		}
	}
	if !sawKicked {
		t.Fatal("被踢连接应收到 ws-kicked 帧")
	}
	if _, _, err := connA.ReadMessage(); err == nil {
		t.Error("踢出帧后 A 应感知断开")
	}

	// B（未被踢）：仅收到既有连接数广播（注册窗口 count=2 与踢出后 count=1），
	// 无任何 ws-kicked 管控帧
	waitClientCount(t, hub, 1)
	for i, want := range []int{2, 1} {
		f := readFrame(t, connB)
		if f.Event == wsKickedEvent {
			t.Fatalf("未被踢连接不应收到管控帧, got %v", f)
		}
		if n, ok := frameDataCount(f); !ok || f.Event != wsConnectionsEvent || n != want {
			t.Fatalf("B 第 %d 帧应为 count=%d 连接数广播, got event=%q data=%v", i+1, want, f.Event, f.Data)
		}
	}
	// 未误伤：后续广播照常送达
	hub.Emit("probe", "x")
	if f := readBusinessFrame(t, connB); f.Event != "probe" {
		t.Fatalf("未被踢连接应继续收到广播, got %q", f.Event)
	}
	_ = connB.Close()
}

// TestWSHub_KickFrameNotInResumeReplay 踢出帧不进环形缓冲：被踢连接重连握手
//（resume 补发）不含 ws-kicked——踢出提示是一次性控制帧，重放会在重连瞬间
// 误报「被踢」（幻影回放）。
func TestWSHub_KickFrameNotInResumeReplay(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn1 := mustDialWS(t, srv, "?token=right-token")
	lastSeq := readFrame(t, conn1).Seq // 注册计数帧占号，构成序号基准
	hub.Emit("e1", "x")
	lastSeq = readBusinessFrame(t, conn1).Seq

	if _, ok := hub.DisconnectClient(1); !ok {
		t.Fatal("踢出应命中")
	}
	// 排空被踢连接（读至断开），确保 unregister 收尾完成再重连
	_ = conn1.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn1.ReadMessage(); err != nil {
			break
		}
	}
	waitClientCount(t, hub, 0)

	// 断线期间产生业务帧（应被补发）
	hub.Emit("e2", "y")

	conn2 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	sendResume(t, conn2, lastSeq)
	sawSynced, sawE2 := false, false
	for i := 0; i < 64 && !sawSynced; i++ {
		f := readFrame(t, conn2)
		switch f.Event {
		case wsKickedEvent:
			t.Fatal("resume 补发面不应包含踢出帧（不进环形缓冲）")
		case "e2":
			sawE2 = true
		case wsSyncedEvent:
			sawSynced = true
		}
	}
	if !sawSynced || !sawE2 {
		t.Fatalf("补发应含断线期帧并以 ws-synced 收尾, sawE2=%v sawSynced=%v", sawE2, sawSynced)
	}
	// 补发完成后实时广播正常
	hub.Emit("e3", "z")
	if f := readBusinessFrame(t, conn2); f.Event != "e3" {
		t.Fatalf("补发后实时帧应正常, got %q", f.Event)
	}
	_ = conn2.Close()
}

// TestWSHub_DisconnectClientQueueFullFallback 队列满降级与 kicked 幂等：send 队列
// 填满时踢出帧入队失败，回退直接 conn.Close()（帧丢失退化为普通断开，断开恒生效）；
// kicked 标记挡住窗口内重复踢出的二次入队（writePump 自关路径垃圾帧防御），幂等
// 返回命中。手工在册客户端确定性构造——不经 ServeHTTP（无 writePump 消费队列，
// 容量恒满，不依赖慢消费者时序）。
func TestWSHub_DisconnectClientQueueFullFallback(t *testing.T) {
	hub := NewWSHub(StaticVerifier("right-token"), nil, nil)

	// 真实升级一条连接，取服务端侧 conn 供手工 wsClient 持有（conn.Close 可在
	// 拨号侧观察：读到错误）。该连接不注册进 hub，仅作底层承载。
	up := websocket.Upgrader{}
	serverConns := make(chan *websocket.Conn, 1)
	srvDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sc, err := up.Upgrade(w, r, nil); err == nil {
			serverConns <- sc
		}
		<-srvDone // 挂住直至测试收尾（hijacked 连接请求以 handler 返回为完成）
	}))
	defer func() {
		close(srvDone)
		srv.Close()
	}()
	clientConn, _, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).Dial("ws://"+srv.Listener.Addr().String(), nil)
	if err != nil {
		t.Fatalf("底层连接升级失败: %v", err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })
	sconn := <-serverConns

	c := &wsClient{hub: hub, conn: sconn, send: make(chan []byte, wsSendBuffer), detail: ConnDetail{ID: 42, IP: "127.0.0.1", ConnectedAt: time.Now()}}
	hub.mu.Lock()
	hub.clients[c] = struct{}{}
	hub.mu.Unlock()
	t.Cleanup(func() { hub.Close() })

	// 确定性填满 send 队列（无 writePump 消费，容量恒满）
	for i := 0; i < wsSendBuffer; i++ {
		c.send <- []byte("queued")
	}

	detail, ok := hub.DisconnectClient(42)
	if !ok || detail.ID != 42 {
		t.Fatalf("命中应返 (detail.ID=42, true), got (%+v, %v)", detail, ok)
	}
	// 踢出帧未入队（队列满回退直关），连接断开恒生效：拨号侧读出错误
	if len(c.send) != wsSendBuffer {
		t.Errorf("队列满时踢出帧不应入队, len=%d", len(c.send))
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := clientConn.ReadMessage(); err != nil {
			break
		}
	}

	// 窗口内重复踢出：kicked 标记挡住二次入队，幂等返回命中
	if _, ok := hub.DisconnectClient(42); !ok {
		t.Error("窗口内重复踢出应幂等返回命中")
	}
	if len(c.send) != wsSendBuffer {
		t.Errorf("kicked 标记应挡住二次入队, len=%d", len(c.send))
	}
}
