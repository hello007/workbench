package main

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"workbench/model"
	"workbench/server"
	"workbench/service"
)

// httpGetBodyWithToken 向 addr 发起带 Bearer 令牌的 GET 请求，返回状态码与
// 响应体（封禁拒连的专用消息断言需要读体，httpGetWithToken 仅返回状态码）。
func httpGetBodyWithToken(t *testing.T, url, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求 %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读响应体失败: %v", err)
	}
	return resp.StatusCode, string(body)
}

// defaultTokenStatus 取名册中「默认」令牌的 Status（NewAppServices 引导产物，
// serveTokenFile 明文兼容文件同源）。
func defaultTokenStatus(t *testing.T, app *App) model.WebTokenStatus {
	t.Helper()
	for _, st := range app.ListWebTokens() {
		if st.Record.Name == service.DefaultWebTokenName {
			return st
		}
	}
	t.Fatal("前置：名册应含「默认」引导令牌")
	return model.WebTokenStatus{}
}

// TestApp_BanUnbanWebToken_Delegate 封禁/解封绑定层：委托服务层状态机
// （语义由 service 测试覆盖），ListWebTokens 封禁状态 join 透出（含 reason/
// 到期），解封后清空；webTokenSvc 未装配（旁路构造）报错不崩。
func TestApp_BanUnbanWebToken_Delegate(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	if app.webTokenSvc == nil {
		t.Fatal("前置：测试 App 应已装配 webTokenSvc")
	}
	_, rec, err := app.webTokenSvc.Create("手机", 0)
	if err != nil {
		t.Fatalf("前置：创建令牌失败: %v", err)
	}

	findStatus := func(id string) (model.WebTokenStatus, bool) {
		for _, st := range app.ListWebTokens() {
			if st.Record.ID == id {
				return st, true
			}
		}
		return model.WebTokenStatus{}, false
	}

	// 未封禁基线
	if st, ok := findStatus(rec.ID); !ok || st.Banned || st.BannedUntil != 0 || st.BanReason != "" {
		t.Fatalf("未封禁基线应全零, got %+v (ok=%v)", st, ok)
	}

	// 封禁（定时档）→ join 透出
	expires := time.Now().Add(2 * time.Hour).Unix()
	if err := app.BanWebToken(rec.ID, expires, "异常行为"); err != nil {
		t.Fatalf("BanWebToken: %v", err)
	}
	st, ok := findStatus(rec.ID)
	if !ok || !st.Banned || st.BannedUntil != expires || st.BanReason != "异常行为" {
		t.Fatalf("封禁状态应 join 透出, got %+v (ok=%v)", st, ok)
	}

	// 解封 → 清空
	if err := app.UnbanWebToken(rec.ID); err != nil {
		t.Fatalf("UnbanWebToken: %v", err)
	}
	if st, ok := findStatus(rec.ID); !ok || st.Banned {
		t.Fatalf("解封后封禁状态应清空, got %+v (ok=%v)", st, ok)
	}

	// webTokenSvc 未装配：报错不崩
	app.webTokenSvc = nil
	if err := app.BanWebToken(rec.ID, 0, ""); err == nil {
		t.Error("webTokenSvc 未装配时封禁应报错")
	}
	if err := app.UnbanWebToken(rec.ID); err == nil {
		t.Error("webTokenSvc 未装配时解封应报错")
	}
	// ListWebTokens 未装配返回空切片非 nil
	if list := app.ListWebTokens(); list == nil || len(list) != 0 {
		t.Errorf("webTokenSvc 未装配时 ListWebTokens 应为空切片, got %+v", list)
	}
}

// TestApp_BanRejectsConnectionEndToEnd 封禁拒连端到端（真实 HTTP 服务 + WS）：
// 封禁后 HTTP 请求与 WS 握手两认证点均 401 且携带专用消息（与「请携带访问
// 令牌」可区分）；解封后两通道立即可达；审计行 desktop 归因。
func TestApp_BanRejectsConnectionEndToEnd(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)
	auditPath := filepath.Join(dataDir, "web_audit.log")
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	token, err := server.LoadOrCreateToken(serveTokenFile)
	if err != nil {
		t.Fatalf("读访问令牌: %v", err)
	}
	st := defaultTokenStatus(t, app)
	baseURL := "http://" + m.listenAddr()

	// 基线：封禁前可达
	if code, _ := httpGetBodyWithToken(t, baseURL+"/", token); code != http.StatusOK {
		t.Fatalf("封禁前应 200, got %d", code)
	}

	// 封禁（永久）
	if err := app.BanWebToken(st.Record.ID, 0, "测试封禁"); err != nil {
		t.Fatalf("BanWebToken: %v", err)
	}

	// HTTP 认证点：401 + 专用消息
	code, body := httpGetBodyWithToken(t, baseURL+"/", token)
	if code != http.StatusUnauthorized {
		t.Fatalf("封禁后 HTTP 应 401, got %d", code)
	}
	if !strings.Contains(body, "永久封禁") || strings.Contains(body, "请携带访问令牌") {
		t.Errorf("HTTP 401 应携带封禁专用消息, got %q", body)
	}

	// WS 握手认证点：升级前 401 + 专用消息
	conn, resp, _ := (&websocket.Dialer{
		Subprotocols:     []string{token},
		HandshakeTimeout: 5 * time.Second,
	}).Dial("ws://"+m.listenAddr()+"/ws", nil)
	if conn != nil {
		t.Fatal("封禁后 WS 升级应失败")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("WS 握手应 401, got %v", resp)
	}
	if resp.Body != nil {
		wsBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if !strings.Contains(string(wsBody), "永久封禁") {
			t.Errorf("WS 401 应携带封禁专用消息, got %q", string(wsBody))
		}
	}

	// 解封 → 两通道立即可达
	if err := app.UnbanWebToken(st.Record.ID); err != nil {
		t.Fatalf("UnbanWebToken: %v", err)
	}
	if code, _ := httpGetBodyWithToken(t, baseURL+"/", token); code != http.StatusOK {
		t.Fatalf("解封后 HTTP 应 200, got %d", code)
	}
	live, _, err := (&websocket.Dialer{
		Subprotocols:     []string{token},
		HandshakeTimeout: 5 * time.Second,
	}).Dial("ws://"+m.listenAddr()+"/ws", nil)
	if err != nil {
		t.Fatalf("解封后 WS 升级应成功: %v", err)
	}
	_ = live.Close()

	// 审计行：token_ban/token_unban 各一条，桌面归因
	var banRow, unbanRow *service.AuditRecord
	for _, rec := range readAuditRecords(t, auditPath) {
		switch rec.Event {
		case service.AuditEventTokenBan:
			row := rec
			banRow = &row
		case service.AuditEventTokenUnban:
			row := rec
			unbanRow = &row
		}
	}
	if banRow == nil || unbanRow == nil {
		t.Fatalf("应各有一条 token_ban/token_unban 审计行, got ban=%v unban=%v", banRow, unbanRow)
	}
	if banRow.TokenID != st.Record.ID || banRow.Reason != "测试封禁" {
		t.Errorf("token_ban 目标字段不符: %+v", banRow)
	}
	if banRow.ActorSource != service.AuditActorDesktop || banRow.ActorTokenID != "" {
		t.Errorf("桌面封禁归因应为 desktop/空, got %+v", banRow)
	}
	if unbanRow.TokenID != st.Record.ID || unbanRow.ActorSource != service.AuditActorDesktop {
		t.Errorf("token_unban 字段不符: %+v", unbanRow)
	}
	assertAuditNoPlaintext(t, auditPath, token)
}

// TestApp_BanWebToken_RPCActorAudit RPC 封禁/解封操作者归因端到端（连接管控
// 四期，对齐 TestApp_KickConnection_RPCActorAudit 三期形态）：浏览器通道
// /api/rpc 经 token 中间件身份注入 + per-request 装饰 target（rpcAuditTarget
// override BanWebToken/UnbanWebToken），审计行 actorSource=rpc +
// actorTokenId=调用者令牌记录 ID；桌面路径（TestApp_BanRejectsConnectionEndToEnd）
// 为 desktop/空可区分。
func TestApp_BanWebToken_RPCActorAudit(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)
	auditPath := filepath.Join(dataDir, "web_audit.log")
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 操作者令牌（与被封禁目标的默认令牌区分）
	actorPlain, actorRec, err := app.webTokenSvc.Create("操作者", 0)
	if err != nil {
		t.Fatalf("创建操作者令牌: %v", err)
	}
	st := defaultTokenStatus(t, app)

	// RPC 封禁：操作者令牌 Bearer 通道调用（永久档 + reason）
	callRPC := func(method string, body string) (bool, string) {
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

	ok, raw := callRPC("BanWebToken", `{"method":"BanWebToken","args":["`+st.Record.ID+`",0,"rpc 封禁"]}`)
	if !ok {
		t.Fatalf("RPC 封禁应成功, got %s", raw)
	}

	// RPC 解封
	ok, raw = callRPC("UnbanWebToken", `{"method":"UnbanWebToken","args":["`+st.Record.ID+`"]}`)
	if !ok {
		t.Fatalf("RPC 解封应成功, got %s", raw)
	}

	// 审计行断言：恰一条 token_ban + 一条 token_unban，操作者归因 rpc/操作者
	// 记录 ID，目标字段为被封禁令牌侧
	var banRow, unbanRow *service.AuditRecord
	for _, rec := range readAuditRecords(t, auditPath) {
		switch rec.Event {
		case service.AuditEventTokenBan:
			row := rec
			banRow = &row
		case service.AuditEventTokenUnban:
			row := rec
			unbanRow = &row
		}
	}
	if banRow == nil || unbanRow == nil {
		t.Fatalf("应各有一条 token_ban/token_unban 审计行, got ban=%v unban=%v", banRow, unbanRow)
	}
	if banRow.ActorSource != service.AuditActorRPC || banRow.ActorTokenID != actorRec.ID {
		t.Errorf("RPC 封禁归因应为 rpc/%s, got %+v", actorRec.ID, banRow)
	}
	if banRow.TokenID != st.Record.ID || banRow.Reason != "rpc 封禁" {
		t.Errorf("token_ban 目标字段应为被封禁令牌侧, got %+v", banRow)
	}
	if unbanRow.ActorSource != service.AuditActorRPC || unbanRow.ActorTokenID != actorRec.ID {
		t.Errorf("RPC 解封归因应为 rpc/%s, got %+v", actorRec.ID, unbanRow)
	}
	// 无明文泄露
	assertAuditNoPlaintext(t, auditPath, actorPlain)
}

// TestApp_BanWebToken_DelegationArgTypes RPC 位置参数形态核对：args 数组按
// BanWebToken(id string, expiresAt int64, reason string) 逐位解析（int64 到期
// 参数经 strconv 构造非零值，防参数错位回归——对齐 wailsjs 三处同步契约的
// 运行时锚点）。
func TestApp_BanWebToken_DelegationArgTypes(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)
	auditPath := filepath.Join(dataDir, "web_audit.log")
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	operatorPlain, _, err := app.webTokenSvc.Create("操作者", 0)
	if err != nil {
		t.Fatalf("创建操作者令牌: %v", err)
	}
	st := defaultTokenStatus(t, app)

	expires := time.Now().Add(time.Hour).Unix()
	// args 逐位解析：id(string) / expiresAt(JSON number → int64) / reason(string)
	body := `{"method":"BanWebToken","args":["` + st.Record.ID + `",` + strconv.FormatInt(expires, 10) + `,"参数形态"]}`
	req, err := http.NewRequest(http.MethodPost, "http://"+m.listenAddr()+"/api/rpc", strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造 RPC 请求: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+operatorPlain)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("RPC 请求失败: %v", err)
	}
	defer resp.Body.Close()
	var parsed struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatalf("解析 RPC 响应: %v", err)
	}
	if !parsed.OK || parsed.Error != nil {
		t.Fatalf("定时封禁 RPC 应成功, got %+v", parsed)
	}
	// 审计行核对 expiresAt 逐位解析正确（非字符串错位形态）
	for _, rec := range readAuditRecords(t, auditPath) {
		if rec.Event == service.AuditEventTokenBan {
			if rec.ExpiresAt != expires || rec.Reason != "参数形态" {
				t.Errorf("expiresAt/reason 应逐位解析正确, got %+v", rec)
			}
			return
		}
	}
	t.Fatal("应有 token_ban 审计行")
}
