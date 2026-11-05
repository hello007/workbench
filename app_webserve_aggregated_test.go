package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workbench/server"
)

// TestServeRPC_GetAggregatedRootNodesHiddenForBrowser 聚合视图根节点对浏览器通道
// 屏蔽（聚合视图红线，对齐 QueryAuditLog/QueryIPBanTrend 先例）。
//
// 该方法载荷含任意入参路径的存在性信息，浏览器侧若可达即成为磁盘路径探测器，
// 故登记于 main.go 的 RPCHiddenMethods。断言：屏蔽与「方法不存在」同响应（不暴露
// 方法存在性）、同 handler 非屏蔽方法不受影响、桌面直调正常（聚合视图的复制路径、
// 新建/重命名等操作依赖该数据面）。
func TestServeRPC_GetAggregatedRootNodesHiddenForBrowser(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	token, err := server.LoadOrCreateToken(serveTokenFile)
	if err != nil {
		t.Fatalf("读访问令牌: %v", err)
	}
	h := m.handler
	if h == nil {
		t.Fatal("前置：装配产物 handler 应就绪")
	}

	post := func(method string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"`+method+`","args":[]}`))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	// 屏蔽方法：与真不存在同响应（同错误码、同消息形态）
	code, body := post("GetAggregatedRootNodes")
	if code != http.StatusOK {
		t.Fatalf("屏蔽拒绝应 HTTP 200 协议应答, got %d", code)
	}
	if !strings.Contains(body, server.ErrCodeRPCMethodNotFound) {
		t.Errorf("屏蔽方法应返回 %s, got %q", server.ErrCodeRPCMethodNotFound, body)
	}
	if !strings.Contains(body, "方法不存在: GetAggregatedRootNodes") {
		t.Errorf("屏蔽响应应与不存在方法同消息形态: %q", body)
	}
	_, notFoundBody := post("NoSuchMethodDefinitely")
	if !strings.Contains(notFoundBody, server.ErrCodeRPCMethodNotFound) {
		t.Fatalf("前置：真不存在方法应返回 %s", server.ErrCodeRPCMethodNotFound)
	}

	// 同 handler 非屏蔽方法不受影响
	code, body = post("GetAppVersion")
	if code != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Errorf("非屏蔽方法应正常调用, got %d %q", code, body)
	}

	// 桌面直调不经翻译层：存在的目录正常返回非 Missing 节点
	dir := t.TempDir()
	nodes := app.GetAggregatedRootNodes([]string{dir})
	if len(nodes) != 1 {
		t.Fatalf("桌面直调应返回 1 个根节点, got %d", len(nodes))
	}
	if nodes[0].Missing {
		t.Error("存在的目录不应标记 Missing")
	}
}
