package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workbench/server"
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
