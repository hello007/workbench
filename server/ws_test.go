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
	hub := newWSHubTuned("right-token", 200*time.Millisecond, defaultPongWait, wsResumeBuffer)
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
	want := []uint64{1, 2, 3}
	for i, conn := range conns {
		for j := 0; j < 3; j++ {
			frame := readFrame(t, conn)
			if frame.Event != "e" || frame.Seq != want[j] {
				t.Errorf("客户端 %d 第 %d 帧不符: event=%q seq=%d, want seq=%d", i, j, frame.Event, frame.Seq, want[j])
			}
		}
	}
}

// TestWSHub_ResumeFillExact 重连精确补发：断线期间的缺口帧按序补发，
// 无缺失无重复，补发完成后收到 ws-synced 同步完成标记。
func TestWSHub_ResumeFillExact(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")

	// 第一段连接收 seq1..3 后断线
	conn1 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	for i := 1; i <= 3; i++ {
		hub.Emit("e", i)
		frame := readFrame(t, conn1)
		if frame.Seq != uint64(i) {
			t.Fatalf("第 %d 帧序号不符: %d", i, frame.Seq)
		}
	}
	_ = conn1.Close()
	waitClientCount(t, hub, 0)

	// 断线期间产生 seq4..5 缺口
	hub.Emit("e", 4)
	hub.Emit("e", 5)

	// 重连握手 resume=3：补发 seq4..5 + ws-synced 标记
	conn2 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	sendResume(t, conn2, 3)
	for i := 4; i <= 5; i++ {
		frame := readFrame(t, conn2)
		if frame.Event != "e" || frame.Seq != uint64(i) {
			t.Errorf("补发帧应为 e/seq%d, got event=%q seq=%d", i, frame.Event, frame.Seq)
		}
	}
	synced := readFrame(t, conn2)
	if synced.Event != wsSyncedEvent {
		t.Fatalf("补发后应收到同步完成标记, got %q", synced.Event)
	}
	if len(synced.Data) != 1 || synced.Data[0] != float64(5) {
		t.Errorf("ws-synced data[0] 应为服务端当前序号 5, got %#v", synced.Data)
	}
	// 补发不影响后续实时广播
	hub.Emit("e", 6)
	if frame := readFrame(t, conn2); frame.Seq != 6 {
		t.Errorf("补发后实时帧 seq 应为 6, got %d", frame.Seq)
	}
}

// TestWSHub_ResumeCaughtUp 追平形态：resume 序号等于服务端当前序号时无帧
// 可补，仅发 ws-synced 标记（客户端凭该标记结束同步模式）。
func TestWSHub_ResumeCaughtUp(t *testing.T) {
	hub, srv := newTestWSHub(t, "right-token")
	conn := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)

	hub.Emit("e", 1)
	readFrame(t, conn)
	sendResume(t, conn, 1)

	synced := readFrame(t, conn)
	if synced.Event != wsSyncedEvent || len(synced.Data) != 1 || synced.Data[0] != float64(1) {
		t.Fatalf("追平形态应仅收到 ws-synced[1], got %#v", synced)
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
	frame := readFrame(t, conn)
	if frame.Event != "live" || frame.Seq != 3 {
		t.Errorf("新连接首帧应为 live/seq3, got event=%q seq=%d", frame.Event, frame.Seq)
	}
}

// TestWSHub_ResumeGapOnOverflow 缓冲溢出断层：续传缓冲容量收紧为 4，客户端
// 断线期间产生超量事件致缺口帧被覆写，重连握手返回 ws-gap 断层标记帧
//（data[0] 为服务端当前序号），此后广播从当前序号正常继续。
func TestWSHub_ResumeGapOnOverflow(t *testing.T) {
	hub := newWSHubTuned("right-token", defaultWriteWait, defaultPongWait, 4)
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	conn1 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	hub.Emit("e", 1)
	readFrame(t, conn1)
	_ = conn1.Close()
	waitClientCount(t, hub, 0)

	// 断线期间 10 条事件（seq2..11），容量 4 的环形缓冲仅保留 seq8..11
	for i := 2; i <= 11; i++ {
		hub.Emit("e", i)
	}

	conn2 := mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
	sendResume(t, conn2, 1)
	gap := readFrame(t, conn2)
	if gap.Event != wsGapEvent {
		t.Fatalf("溢出应返回断层标记帧, got %q", gap.Event)
	}
	if len(gap.Data) != 1 || gap.Data[0] != float64(11) {
		t.Errorf("ws-gap data[0] 应为服务端当前序号 11, got %#v", gap.Data)
	}
	// 断层后广播从当前序号正常继续
	hub.Emit("e", 12)
	if frame := readFrame(t, conn2); frame.Event != "e" || frame.Seq != 12 {
		t.Errorf("断层后实时帧应为 e/seq12, got event=%q seq=%d", frame.Event, frame.Seq)
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
		readFrame(t, conn1)
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
	gap := readFrame(t, conn2)
	if gap.Event != wsGapEvent {
		t.Fatalf("跨代际重连应返回断层标记帧, got %q", gap.Event)
	}
	if len(gap.Data) != 1 || gap.Data[0] != float64(1) {
		t.Errorf("ws-gap data[0] 应为新 hub 当前序号 1, got %#v", gap.Data)
	}
	hub2.Emit("e", "after-gap")
	if frame := readFrame(t, conn2); frame.Event != "e" || frame.Seq != 2 {
		t.Errorf("断层后实时帧应为 e/seq2, got event=%q seq=%d", frame.Event, frame.Seq)
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
	if frame := readFrame(t, conn); frame.Event != "still-alive" {
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
