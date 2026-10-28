package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workbench/server"
	"workbench/service"
)

// 连接管控九期测试：ListIPBanSuggestions 绑定层（委托 + 隐私红线屏蔽）。
// 聚合口径与阈值判定语义由 service 层测试覆盖（ip_ban_aggregate_test.go），
// 本文件只覆盖绑定委托面与 RPCHiddenMethods 装配面。

// TestApp_ListIPBanSuggestions_Delegate 聚合建议绑定层（对齐
// TestApp_UnbanIP_ListIPBans_Delegate 形态）：委托服务层查询（空态恒非 nil 空
// 切片）；建议随自动封禁条目与阈值热更联动（SetIPBanConfig 六参「保存+生效」
// 单点）；解封条目后建议收敛；服务未装配（旁路构造）不崩。
func TestApp_ListIPBanSuggestions_Delegate(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	// 基线：空态返回空切片（非 nil）
	if got := app.ListIPBanSuggestions(); got == nil || len(got) != 0 {
		t.Fatalf("初始建议应为空切片, got %#v", got)
	}

	// 同网段两个自动封禁来源：默认阈值 3 下不产出建议
	for _, ip := range []string{"203.0.113.7", "203.0.113.9"} {
		for i := 0; i < service.IPBanThreshold; i++ {
			app.ipBanSvc.RecordFailure(ip)
		}
	}
	if got := app.ListIPBanSuggestions(); len(got) != 0 {
		t.Fatalf("默认阈值 3 下同网段两条不应产出建议, got %+v", got)
	}

	// SetIPBanConfig 六参热更聚合阈值 2（「保存+生效」单点）：存量条目下次查询即产出
	if err := app.SetIPBanConfig(service.IPBanThreshold, 10, 5, 60, 2, nil); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	got := app.ListIPBanSuggestions()
	if len(got) != 1 || got[0].CIDR != "203.0.113.0/24" || got[0].Count != 2 {
		t.Fatalf("热更阈值 2 后应产出一条建议, got %+v", got)
	}

	// 解封其中一个来源：桶计数降为 1，低于阈值建议收敛
	if err := app.UnbanIP("203.0.113.9"); err != nil {
		t.Fatalf("UnbanIP: %v", err)
	}
	if got := app.ListIPBanSuggestions(); len(got) != 0 {
		t.Errorf("解封后低于阈值建议应收敛, got %+v", got)
	}

	// 服务未装配（旁路构造）：空切片不崩
	app.ipBanSvc = nil
	if got := app.ListIPBanSuggestions(); got == nil || len(got) != 0 {
		t.Errorf("服务未装配应返回空切片（非 nil）, got %#v", got)
	}
}

// TestServeRPC_ListIPBanSuggestionsHiddenForBrowser 聚合建议对浏览器通道屏蔽
// （隐私红线，连接管控九期，对齐 TestServeRPC_QueryAuditLogHiddenForBrowser
// 三期先例）：生产装配（startLocked 真实产出 handler）的 RPCHiddenMethods 含
// ListIPBanSuggestions，命中与「方法不存在」同响应（不暴露被屏蔽方法存在性）；
// 同 handler 上非屏蔽方法不受影响；桌面 Wails 绑定不经翻译层（直调委托面，见
// TestApp_ListIPBanSuggestions_Delegate），不受屏蔽影响。
func TestServeRPC_ListIPBanSuggestionsHiddenForBrowser(t *testing.T) {
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
	code, body := post("ListIPBanSuggestions")
	if code != http.StatusOK {
		t.Fatalf("屏蔽拒绝应 HTTP 200 协议应答, got %d", code)
	}
	if !strings.Contains(body, server.ErrCodeRPCMethodNotFound) {
		t.Errorf("屏蔽方法应返回 %s, got %q", server.ErrCodeRPCMethodNotFound, body)
	}
	if !strings.Contains(body, "方法不存在: ListIPBanSuggestions") {
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

	// 桌面直调不经翻译层：同 App 有封禁条目时建议照常产出（屏蔽仅浏览器通道面；
	// 聚合阈值 clamp 下限 2，驱动两个来源达阈值）
	for _, ip := range []string{"203.0.113.7", "203.0.113.9"} {
		for i := 0; i < service.IPBanThreshold; i++ {
			app.ipBanSvc.RecordFailure(ip)
		}
	}
	if err := app.SetIPBanConfig(service.IPBanThreshold, 10, 5, 60, 2, nil); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	if got := app.ListIPBanSuggestions(); len(got) != 1 {
		t.Errorf("桌面直调不应受 RPCHiddenMethods 屏蔽影响, got %+v", got)
	}
}
