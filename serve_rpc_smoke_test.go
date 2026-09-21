package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"workbench/server"
	"workbench/util/testutil"
)

// TestServeRPC_GetAppVersionSmoke --serve 冒烟：/api/rpc 通用翻译层对真实
// *App 走通无副作用方法 GetAppVersion。未携带令牌 401 拒绝；携带令牌返回
// {"ok":true,"data":<version>}，与桌面 webview 绑定调用结果一致。
func TestServeRPC_GetAppVersionSmoke(t *testing.T) {
	handler := server.NewWebHandler(server.WebOptions{
		Assets:    nil, // 冒烟不涉及静态资产
		Token:     "smoke-token",
		RPCTarget: NewApp(),
	})

	// 未携带令牌：401 拒绝
	req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"GetAppVersion","args":[]}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未携带令牌应 401, got %d", rec.Code)
	}

	// 携带令牌（Authorization: Bearer）：走通并返回版本号
	req = httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"GetAppVersion","args":[]}`))
	req.Header.Set("Authorization", "Bearer smoke-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("携带令牌应 200, got %d", rec.Code)
	}
	want := `{"ok":true,"data":"` + version + `"`
	if body := rec.Body.String(); !strings.Contains(body, want) {
		t.Errorf("GetAppVersion 响应应含 %s, got %q", want, body)
	}

	// 携带令牌（?token= 查询参数兜底）：同样走通
	req = httptest.NewRequest(http.MethodPost, "/api/rpc?token=smoke-token", strings.NewReader(`{"method":"GetAppVersion","args":[]}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("查询参数令牌应 200, got %d", rec.Code)
	}

	// 未知方法：HTTP 200 + 明确错误码（业务失败属正常协议应答）
	req = httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"NoSuchMethod","args":[]}`))
	req.Header.Set("Authorization", "Bearer smoke-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("未知方法应 HTTP 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, server.ErrCodeRPCMethodNotFound) {
		t.Errorf("未知方法应返回 %s, got %q", server.ErrCodeRPCMethodNotFound, body)
	}
}

// TestServeRPC_DialogMethodsRejectedInServeMode serve 无头模式下对话框方法被
// 守卫前置拒绝：NewApp 的 a.ctx 为 nil（runServe 兜底 Background 同样无 frontend
// 键），wails runtime 对该形态会 log.Fatalf 退出整个进程，守卫必须在到达 runtime
// 调用前返回普通错误（映射 E_RPC_INTERNAL）。测试正常走完（进程未被 os.Exit
// 杀死）即证明防护生效。
func TestServeRPC_DialogMethodsRejectedInServeMode(t *testing.T) {
	handler := server.NewWebHandler(server.WebOptions{
		Assets:    nil,
		Token:     "smoke-token",
		RPCTarget: NewApp(), // ctx 为 nil，模拟 serve 模式无 Wails startup
	})

	for _, method := range []string{"SaveFileDialog", "OpenFileDialog"} {
		body := `{"method":"` + method + `","args":["a.csv",[{"DisplayName":"CSV 文件","Pattern":"*.csv"}]]}`
		req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer smoke-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s 应 HTTP 200 协议应答, got %d", method, rec.Code)
		}
		respBody := rec.Body.String()
		if !strings.Contains(respBody, `"ok":false`) || !strings.Contains(respBody, server.ErrCodeRPCInternal) {
			t.Errorf("%s 应以 %s 拒绝: %q", method, server.ErrCodeRPCInternal, respBody)
		}
		if !strings.Contains(respBody, "无头服务模式下原生对话框不可用") {
			t.Errorf("%s 错误消息应可读: %q", method, respBody)
		}
	}
}

// TestServeWS_EventBroadcastSmoke serve 模式事件链端到端冒烟：完整 serve 装配
// （NewAppServices + SetEventSink 切换为 WS hub + WebHandler 挂载 /ws 与
// /api/rpc），浏览器侧 WS 客户端携 token 升级连接后，经 /api/rpc 调
// ScanAndPullRepos（异步 goroutine 内 gitSvc.BatchPull）触发事件，断言 WS
// 收到 JSON 事件帧（{"event","data"}）。未授权 WS 升级在装配层同样被 401 拒绝。
func TestServeWS_EventBroadcastSmoke(t *testing.T) {
	token := "smoke-ws-token"
	app := NewApp()
	// 不用 t.TempDir：NewAppServices 内 util.InitLogger 的 lumberjack 句柄持有
	// app.log，Windows 上 RemoveAll 清理失败（对齐 perf_bench_test.go perfDataDir）
	dataDir, err := os.MkdirTemp("", "workbench-ws-smoke")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	app.AppServices = NewAppServices(context.Background(), dataDir, false)
	// serve 模式 sink 切换（与 runServe 装配行同构）
	hub := server.NewWSHub(token)
	app.terminalSvc.SetEventSink(hub)
	app.updateSvc.SetEventSink(hub)
	app.aiFuncSvc.SetEventSink(hub)
	app.gitSvc.SetEventSink(hub)

	srv := httptest.NewServer(server.NewWebHandler(server.WebOptions{
		Assets:    nil, // 冒烟不涉及静态资产
		Token:     token,
		RPCTarget: app,
		WSHub:     hub,
	}))
	// 清理顺序：先关 WS 连接（解除 hijack），后关服务器（等待连接收尾）
	defer srv.Close()

	// 未授权升级：401 拒绝
	unauthDialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	_, resp, _ := unauthDialer.Dial("ws://"+srv.Listener.Addr().String()+"/ws", nil)
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未授权 WS 升级应 401, got %v", resp)
	}

	// 正确 token（查询参数通道）升级成功
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).
		Dial("ws://"+srv.Listener.Addr().String()+"/ws?token="+token, nil)
	if err != nil {
		t.Fatalf("WS 升级失败: %v", err)
	}
	defer conn.Close()

	// 等连接完成注册后再触发事件（注册在升级后的 handler goroutine 内）
	regDeadline := time.Now().Add(5 * time.Second)
	for hub.ClientCount() == 0 {
		if time.Now().After(regDeadline) {
			t.Fatal("等待 WS 客户端注册超时")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// RPC 触发事件：扫描根下单个无远程仓库，ScanAndPullRepos 异步 BatchPull
	// 走跳过路径，仍推 pull-progress + pull-complete（事件经切换后的 hub 广播）
	scanRoot := t.TempDir()
	repo := filepath.Join(scanRoot, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("创建仓库目录失败: %v", err)
	}
	testutil.RunGit(t, repo, "init")
	scanArgs, _ := json.Marshal([]any{scanRoot})
	rpcReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/rpc?token="+token,
		strings.NewReader(`{"method":"ScanAndPullRepos","args":`+string(scanArgs)+`}`))
	rpcResp, err := http.DefaultClient.Do(rpcReq)
	if err != nil {
		t.Fatalf("ScanAndPullRepos RPC 请求失败: %v", err)
	}
	_ = rpcResp.Body.Close()
	if rpcResp.StatusCode != http.StatusOK {
		t.Fatalf("ScanAndPullRepos RPC 应 200, got %d", rpcResp.StatusCode)
	}

	// 断言 WS 收到事件帧
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	seen := map[string]bool{}
	for len(seen) < 2 {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("读取事件帧失败（已收 %v）: %v", seen, err)
		}
		var frame struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(payload, &frame); err != nil {
			t.Fatalf("事件帧不是合法 JSON: %v, payload=%q", err, payload)
		}
		seen[frame.Event] = true
	}
	if !seen["pull-progress"] || !seen["pull-complete"] {
		t.Errorf("应收到 pull-progress 与 pull-complete 帧, got %v", seen)
	}
}
