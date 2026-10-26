package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/server"
	"workbench/service"
)

// httptest.NewRequest 的默认来源地址（非回环，RemoteAddr 形态 "192.0.2.1:1234"）：
// 认证失败计数端到端用例按此 IP 断言（回环来源被服务豁免，无法驱动封禁）。
const testAuthFailIP = "192.0.2.1"

// requestViaHandler 以指定令牌经装配产物 handler 发起一次请求（模拟非回环来源
// 的浏览器请求，RemoteAddr 取 httptest.NewRequest 默认值），返回状态码与响应体。
// 不经网络直调 handler——装配接线（IPBanCheck 注入 / OnAuth 计数驱动）由
// startLocked 真实产出，接线本身即被测面。
func requestViaHandler(t *testing.T, h http.Handler, path, token string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// TestApp_UnbanIP_ListIPBans_Delegate 解封/列表绑定层：委托服务层状态机（语义由
// service 测试覆盖），UnbanIP 幂等 no-op 返 nil，ListIPBans 恒非 nil 空切片；
// 解封生效（新连接可达）且写 ip_unban 审计行（desktop 归因）；服务未装配（旁路
// 构造）不崩。
func TestApp_UnbanIP_ListIPBans_Delegate(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)
	auditPath := filepath.Join(dataDir, "web_audit.log")
	ip := testAuthFailIP

	// 基线：空列表（非 nil）
	if bans := app.ListIPBans(); bans == nil || len(bans) != 0 {
		t.Fatalf("初始列表应为空切片, got %+v", bans)
	}

	// 触发自动封禁
	for i := 0; i < service.IPBanThreshold; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	bans := app.ListIPBans()
	if len(bans) != 1 || bans[0].IP != ip || bans[0].FailCount != service.IPBanThreshold {
		t.Fatalf("封禁列表应含 1 条记录, got %+v", bans)
	}
	if app.ipBanSvc.RejectMessage(ip) == "" {
		t.Fatal("前置：封禁应生效")
	}

	// 解封：生效（新连接可达）+ 幂等（重复解封/无记录 IP 均 no-op 返 nil）
	if err := app.UnbanIP(ip); err != nil {
		t.Fatalf("UnbanIP: %v", err)
	}
	if app.ipBanSvc.RejectMessage(ip) != "" {
		t.Error("解封后新连接应可达")
	}
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Errorf("解封应删除封禁记录, got %+v", bans)
	}
	if err := app.UnbanIP(ip); err != nil {
		t.Errorf("重复解封应幂等 no-op 返 nil, got %v", err)
	}
	if err := app.UnbanIP("198.51.100.5"); err != nil {
		t.Errorf("无记录 IP 解封应 no-op 返 nil, got %v", err)
	}

	// 审计行：恰一条 ip_unban，desktop 归因
	var unbanRows []service.AuditRecord
	for _, rec := range readAuditRecords(t, auditPath) {
		if rec.Event == service.AuditEventIPUnban {
			unbanRows = append(unbanRows, rec)
		}
	}
	if len(unbanRows) != 1 {
		t.Fatalf("应恰有 1 条 ip_unban 审计行, got %d", len(unbanRows))
	}
	if unbanRows[0].IP != ip || unbanRows[0].ActorSource != service.AuditActorDesktop {
		t.Errorf("ip_unban 行字段不符: %+v", unbanRows[0])
	}
}

// TestApp_TokenLifecycleClearsIPBans 令牌生命周期联动清零（连接管控五期）：撤销
// 与轮换成功后 IP 失败计数与封禁记录一并清空（轮换/撤销引发的旧凭据 401 失败潮
// 防误伤叠加），解封动作本身不写联动审计（预防性动作）。
func TestApp_TokenLifecycleClearsIPBans(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	ip := testAuthFailIP

	// 封禁生效
	for i := 0; i < service.IPBanThreshold; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	if app.ipBanSvc.RejectMessage(ip) == "" {
		t.Fatal("前置：封禁应生效")
	}

	// 撤销令牌 → 联动清零
	st := defaultTokenStatus(t, app)
	if err := app.RevokeWebToken(st.Record.ID); err != nil {
		t.Fatalf("RevokeWebToken: %v", err)
	}
	if msg := app.ipBanSvc.RejectMessage(ip); msg != "" {
		t.Errorf("撤销令牌后 IP 封禁应联动清空, got %q", msg)
	}
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Errorf("撤销令牌后封禁列表应为空, got %+v", bans)
	}

	// 再次封禁 → 轮换令牌 → 联动清零（新令牌可达为轮换编排副产品，此处只断言清零）
	for i := 0; i < service.IPBanThreshold; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	if app.ipBanSvc.RejectMessage(ip) == "" {
		t.Fatal("前置：再次封禁应生效")
	}
	if _, err := app.RegenerateWebToken(); err != nil {
		t.Fatalf("RegenerateWebToken: %v", err)
	}
	if msg := app.ipBanSvc.RejectMessage(ip); msg != "" {
		t.Errorf("轮换令牌后 IP 封禁应联动清空, got %q", msg)
	}
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Errorf("轮换令牌后封禁列表应为空, got %+v", bans)
	}

	// 联动清零后从零重新计数：封禁清除后的失败不叠加历史窗口
	for i := 0; i < service.IPBanThreshold-1; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	if app.ipBanSvc.RejectMessage(ip) != "" {
		t.Error("联动清零后 9 次失败不应触发封禁（应从零起算）")
	}
}

// TestApp_AuthFailureCountingDrivesIPBan 认证失败计数端到端（装配接线级）：
// 非回环来源连续认证失败达阈值自动封禁后，同来源携带有效令牌的新连接也在
// verify 之前被拒（401 专用消息）；认证成功清零计数（未达阈值不触发）。
func TestApp_AuthFailureCountingDrivesIPBan(t *testing.T) {
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

	// 9 次失败：未达阈值不封禁（但每次都是 401 普通认证失败）
	for i := 0; i < service.IPBanThreshold-1; i++ {
		code, body := requestViaHandler(t, h, "/", "wrong-token")
		if code != http.StatusUnauthorized || !strings.Contains(body, "请携带访问令牌") {
			t.Fatalf("第 %d 次失败应为普通 401, got %d %q", i+1, code, body)
		}
	}
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Fatalf("未达阈值不应产生封禁记录, got %+v", bans)
	}

	// 认证成功清零：再 9 次失败不触发（累计语义被成功打断）
	code, _ := requestViaHandler(t, h, "/", token)
	if code != http.StatusOK {
		t.Fatalf("有效令牌应 200, got %d", code)
	}
	for i := 0; i < service.IPBanThreshold-1; i++ {
		requestViaHandler(t, h, "/", "wrong-token")
	}
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Fatalf("成功清零后 9 次失败不应触发封禁, got %+v", bans)
	}

	// 第 10 次失败触发自动封禁
	requestViaHandler(t, h, "/", "wrong-token")
	bans := app.ListIPBans()
	if len(bans) != 1 || bans[0].IP != testAuthFailIP || bans[0].FailCount != service.IPBanThreshold {
		t.Fatalf("达阈值应自动封禁, got %+v", bans)
	}
	if bans[0].Expired {
		t.Error("刚触发的封禁不应标记已过期")
	}

	// 封禁中：携带有效令牌的新连接也在 verify 之前被拒（401 专用消息，非普通
	// 「请携带访问令牌」）
	code, body := requestViaHandler(t, h, "/", token)
	if code != http.StatusUnauthorized {
		t.Fatalf("封禁后有效令牌请求也应 401, got %d", code)
	}
	if !strings.Contains(body, "临时封禁") || strings.Contains(body, "请携带访问令牌") {
		t.Errorf("401 应携带 IP 封禁专用消息, got %q", body)
	}
}

// TestApp_UnbanIP_RPCActorAudit RPC 解封操作者归因端到端（连接管控五期，对齐
// TestApp_BanWebToken_RPCActorAudit 四期形态）：浏览器通道 /api/rpc 经 token 中间
// 件身份注入 + per-request 装饰 target（rpcAuditTarget override UnbanIP），实际
// 解除的 ip_unban 审计行 actorSource=rpc + actorTokenId=调用者令牌记录 ID；no-op
// 解封（无封禁记录）不写审计行（「动作生效后才写审计」纪律）；桌面路径为
// desktop/空（TestApp_UnbanIP_ListIPBans_Delegate 覆盖）可区分。
func TestApp_UnbanIP_RPCActorAudit(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)
	auditPath := filepath.Join(dataDir, "web_audit.log")
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 操作者令牌（与被封禁目标来源 IP 无关，仅作 RPC 调用者身份）
	actorPlain, actorRec, err := app.webTokenSvc.Create("解封操作者", 0)
	if err != nil {
		t.Fatalf("创建操作者令牌: %v", err)
	}
	ip := testAuthFailIP

	callUnbanIP := func(body string) (bool, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, "http://"+m.listenAddr()+"/api/rpc", strings.NewReader(body))
		if err != nil {
			t.Fatalf("构造 RPC 请求: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+actorPlain)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("RPC 请求失败: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("RPC 应答应 200, got %d body=%s", resp.StatusCode, raw)
		}
		var parsed struct {
			OK    bool `json:"ok"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("解析 RPC 响应: %v", err)
		}
		return parsed.OK && parsed.Error == nil, string(raw)
	}

	// no-op 解封（未封禁 IP）：no-op 成功且不写审计行
	ok, raw := callUnbanIP(`{"method":"UnbanIP","args":["198.51.100.5"]}`)
	if !ok {
		t.Fatalf("no-op 解封应幂等成功, got %s", raw)
	}

	// 触发自动封禁后 RPC 解封：实际解除
	for i := 0; i < service.IPBanThreshold; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	if app.ipBanSvc.RejectMessage(ip) == "" {
		t.Fatal("前置：封禁应生效")
	}
	ok, raw = callUnbanIP(`{"method":"UnbanIP","args":["` + ip + `"]}`)
	if !ok {
		t.Fatalf("RPC 解封应成功, got %s", raw)
	}
	if app.ipBanSvc.RejectMessage(ip) != "" {
		t.Error("RPC 解封后新连接应可达")
	}
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Errorf("RPC 解封应删除封禁记录, got %+v", bans)
	}

	// 审计行断言：仅实际解除写一条 ip_unban（no-op 不写），归因 rpc/操作者记录 ID
	var unbanRows []service.AuditRecord
	for _, rec := range readAuditRecords(t, auditPath) {
		if rec.Event == service.AuditEventIPUnban {
			unbanRows = append(unbanRows, rec)
		}
	}
	if len(unbanRows) != 1 {
		t.Fatalf("应恰有 1 条 ip_unban 审计行（no-op 不写）, got %d", len(unbanRows))
	}
	if unbanRows[0].IP != ip {
		t.Errorf("ip_unban 行 IP 应为 %q, got %+v", ip, unbanRows[0])
	}
	if unbanRows[0].ActorSource != service.AuditActorRPC || unbanRows[0].ActorTokenID != actorRec.ID {
		t.Errorf("RPC 解封归因应为 rpc/%s, got %+v", actorRec.ID, unbanRows[0])
	}
	// 无明文泄露
	assertAuditNoPlaintext(t, auditPath, actorPlain)
}

// TestApp_OnBannedToastPayload 自动封禁触发回调（桌面 toast 数据源）：封禁写入
// 内存后同步触发，载荷为刚写入的封禁状态快照（IP/触发时刻/到期时刻/窗口计数）；
// 未达阈值不触发。测试覆盖注入自定义闭包断言载荷（装配层闭包为一行 emit，
// 载荷经本回调透传）。
func TestApp_OnBannedToastPayload(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	ip := testAuthFailIP

	var got []model.IPBanStatus
	app.ipBanSvc.SetOnBanned(func(status model.IPBanStatus) {
		got = append(got, status)
	})

	for i := 0; i < service.IPBanThreshold-1; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	if len(got) != 0 {
		t.Fatalf("未达阈值不应触发回调, got %+v", got)
	}
	app.ipBanSvc.RecordFailure(ip)
	if len(got) != 1 {
		t.Fatalf("达阈值应恰触发一次回调, got %+v", got)
	}
	st := got[0]
	if st.IP != ip || st.FailCount != service.IPBanThreshold {
		t.Errorf("回调载荷不符: %+v", st)
	}
	if st.BannedAt == 0 || st.ExpiresAt <= st.BannedAt {
		t.Errorf("回调载荷时间戳不符: %+v", st)
	}
	if st.Expired {
		t.Error("刚触发的封禁载荷不应标记已过期")
	}
	// 与 List 记录一致（单事件单真相）
	bans := app.ListIPBans()
	if len(bans) != 1 || bans[0].BannedAt != st.BannedAt || bans[0].ExpiresAt != st.ExpiresAt {
		t.Errorf("回调载荷应与列表记录一致: callback=%+v list=%+v", st, bans)
	}
}
