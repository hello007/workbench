package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	hub := NewWSHub(token)
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
	frame := readFrame(t, conn)
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
	frame := readFrame(t, conn)
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
	frame := readFrame(t, conn)
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
	frame := readFrame(t, conn)
	if frame.Event != "terminal-output" || frame.Data[0] != "term-1" || frame.Data[1] != "out" {
		t.Errorf("帧字段不符: got %#v", frame)
	}

	// 零载荷事件 data 序列化为空数组（非 null），shim 展开为空变参
	hub.Emit("no-payload")
	frame = readFrame(t, conn)
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
		frame := readFrame(t, conn)
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

// TestWSHub_SetTokenRotation 令牌热轮换：轮换后旧令牌两通道均 401、新令牌均
// 升级成功；轮换前建立的在途连接不在数据面校验令牌，保持有效并继续收到广播
// （RegenerateWebToken 的热生效语义：不断监听、不断在途连接）。
func TestWSHub_SetTokenRotation(t *testing.T) {
	hub, srv := newTestWSHub(t, "old-token")

	// 轮换前建立在途连接（旧令牌时期）
	existing := mustDialWS(t, srv, "?token=old-token")
	waitClientCount(t, hub, 1)

	hub.SetToken("new-token")

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
	frame := readFrame(t, existing)
	if frame.Event != "after-rotate" {
		t.Errorf("在途连接应继续收到广播, got %q", frame.Event)
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

	// 各客户端读侧感知服务端关闭（连接被断，读到错误而非事件帧）
	for i, conn := range conns {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, _, err := conn.ReadMessage(); err == nil {
			t.Errorf("客户端 %d 在 hub Close 后应读到连接关闭错误", i)
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
	hub := newWSHubTuned("right-token", 200*time.Millisecond, defaultPongWait)
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
