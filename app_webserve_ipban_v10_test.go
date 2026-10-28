package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workbench/model"
	"workbench/server"
	"workbench/service"
)

// 连接管控十期测试：QueryIPBanTrend 绑定层（委托 + 未装配空结果 + 隐私红线屏蔽
// + SetIPBanConfig 七参窗口热更）。聚合口径与 rebounded 判定语义由 service 层
// 测试覆盖（ip_ban_trend_test.go），本文件只覆盖绑定委托面与 RPCHiddenMethods
// 装配面。时间面依赖审计行 Ts 与状态机 nowFn 同源 time.Now（生产默认构造），
// 断言只做计数/长度/非零相对面，天桶精确分布由纯函数注入时钟用例覆盖。

// TestApp_QueryIPBanTrend_Delegate 趋势查询绑定层（对齐
// TestApp_ListIPBanSuggestions_Delegate 形态）：委托服务层查询（空态恒空结果
// 不报错）；auto/manual 审计行经真实状态机落盘后进聚合桶；SetIPBanConfig 七参
// 「保存+生效」热更趋势窗口；服务未装配（旁路构造）不崩。
func TestApp_QueryIPBanTrend_Delegate(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	// 基线：空审计快照返回空结果（不报错）
	got, err := app.QueryIPBanTrend()
	if err != nil {
		t.Fatalf("QueryIPBanTrend: %v", err)
	}
	if got.WindowDays != service.IPBanTrendWindowDaysDefault || len(got.Buckets) != 0 || got.EarliestTs != 0 {
		t.Fatalf("空态应为默认窗口 + 空桶, got %+v", got)
	}

	// 真实链路：同网段两来源各触发自动封禁（auto ip_ban 审计行）+ 手动封禁一个
	// 新来源（manual 行）
	for _, ip := range []string{"203.0.113.7", "203.0.113.9"} {
		for i := 0; i < service.IPBanThreshold; i++ {
			app.ipBanSvc.RecordFailure(ip)
		}
	}
	if err := app.BanIP("203.0.113.200", 0, ""); err != nil {
		t.Fatalf("BanIP: %v", err)
	}

	got, err = app.QueryIPBanTrend()
	if err != nil {
		t.Fatalf("QueryIPBanTrend: %v", err)
	}
	var bucket *model.IPBanTrendBucket
	for i := range got.Buckets {
		if got.Buckets[i].Key == "203.0.113.0/24" {
			bucket = &got.Buckets[i]
			break
		}
	}
	if bucket == nil {
		t.Fatalf("审计快照应产出 /24 桶, got %+v", got.Buckets)
	}
	if bucket.AutoTotal != 2 || bucket.ManualCount != 1 {
		t.Errorf("桶计数应为 auto 2 + manual 1, got %+v", bucket)
	}
	if len(bucket.Days) != service.IPBanTrendWindowDaysDefault {
		t.Errorf("Days 长度应等于默认窗口 %d, got %d", service.IPBanTrendWindowDaysDefault, len(bucket.Days))
	}
	if got.EarliestTs == 0 {
		t.Error("有 ip_ban 行时 EarliestTs 应非零")
	}

	// SetIPBanConfig 七参「保存+生效」：趋势窗口热更为 2，下次查询即按新窗口
	if err := app.SetIPBanConfig(service.IPBanThreshold, 10, 5, 60, 3, 2, nil); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	got, err = app.QueryIPBanTrend()
	if err != nil {
		t.Fatalf("QueryIPBanTrend: %v", err)
	}
	if got.WindowDays != 2 {
		t.Errorf("热更后窗口应为 2, got %d", got.WindowDays)
	}
	for i := range got.Buckets {
		if len(got.Buckets[i].Days) != 2 {
			t.Errorf("热更后 Days 长度应为 2, got %+v", got.Buckets[i])
		}
	}

	// 服务未装配（旁路构造）：空结果不崩不报错
	app.ipBanSvc = nil
	got, err = app.QueryIPBanTrend()
	if err != nil {
		t.Errorf("服务未装配应返回空结果不报错: %v", err)
	}
	if got.Buckets == nil || len(got.Buckets) != 0 {
		t.Errorf("服务未装配应返回空桶切片（非 nil）, got %#v", got.Buckets)
	}
}

// TestServeRPC_QueryIPBanTrendHiddenForBrowser 趋势查询对浏览器通道屏蔽（隐私
// 红线，连接管控十期，对齐 TestServeRPC_ListIPBanSuggestionsHiddenForBrowser
// 九期先例）：生产装配（startLocked 真实产出 handler）的 RPCHiddenMethods 含
// QueryIPBanTrend，命中与「方法不存在」同响应（不暴露被屏蔽方法存在性）；同
// handler 上非屏蔽方法不受影响；桌面 Wails 绑定不经翻译层（直调委托面，见
// TestApp_QueryIPBanTrend_Delegate），不受屏蔽影响。
func TestServeRPC_QueryIPBanTrendHiddenForBrowser(t *testing.T) {
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
	code, body := post("QueryIPBanTrend")
	if code != http.StatusOK {
		t.Fatalf("屏蔽拒绝应 HTTP 200 协议应答, got %d", code)
	}
	if !strings.Contains(body, server.ErrCodeRPCMethodNotFound) {
		t.Errorf("屏蔽方法应返回 %s, got %q", server.ErrCodeRPCMethodNotFound, body)
	}
	if !strings.Contains(body, "方法不存在: QueryIPBanTrend") {
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

	// 桌面直调不经翻译层：有封禁审计行时趋势照常产出（屏蔽仅浏览器通道面）
	for i := 0; i < service.IPBanThreshold; i++ {
		app.ipBanSvc.RecordFailure("203.0.113.7")
	}
	got, err := app.QueryIPBanTrend()
	if err != nil {
		t.Fatalf("桌面直调 QueryIPBanTrend: %v", err)
	}
	if len(got.Buckets) == 0 {
		t.Errorf("桌面直调不应受 RPCHiddenMethods 屏蔽影响, got %+v", got)
	}
}
