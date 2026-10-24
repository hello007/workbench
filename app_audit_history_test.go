package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"workbench/server"
	"workbench/service"
)

// TestApp_KickConnection_RPCActorAudit RPC 踢出操作者归因端到端（连接管控三期）：
// 浏览器通道 /api/rpc 经 token 中间件身份注入（requireToken → ctx）+ per-request
// 装饰 target（rpcAuditTarget override KickConnection），踢出生效后审计行
// actorSource=rpc + actorTokenId=调用者令牌记录 ID，与桌面行（desktop/空）可
// 区分——多令牌场景可回答「谁踢的」。被踢目标字段（tokenId/name/connId）仍为
// 受害连接侧，与操作者归因正交。
func TestApp_KickConnection_RPCActorAudit(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)
	auditPath := filepath.Join(dataDir, "web_audit.log")
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 操作者令牌（与被踢连接的默认令牌区分的多 token 场景）
	actorPlain, actorRec, err := app.webTokenSvc.Create("操作者", 0)
	if err != nil {
		t.Fatalf("创建操作者令牌: %v", err)
	}

	// 受害连接：默认令牌 WS 接入
	victimToken, err := server.LoadOrCreateToken(serveTokenFile)
	if err != nil {
		t.Fatalf("读访问令牌: %v", err)
	}
	conn, _, err := (&websocket.Dialer{
		Subprotocols:     []string{victimToken},
		HandshakeTimeout: 5 * time.Second,
	}).Dial("ws://"+m.listenAddr()+"/ws", nil)
	if err != nil {
		t.Fatalf("WS 升级失败: %v", err)
	}
	defer conn.Close()
	details := waitConnCacheCount(t, app, 1)
	victimConnID := details[0].ID
	victimTokenID := details[0].TokenID

	// RPC 踢出：操作者令牌 Bearer 通道调用
	body := `{"method":"KickConnection","args":[` + strconv.FormatUint(victimConnID, 10) + `]}`
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("RPC 踢出应 200, got %d", resp.StatusCode)
	}
	var rpcResp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		t.Fatalf("解析 RPC 响应: %v", err)
	}
	if !rpcResp.OK || rpcResp.Error != nil {
		t.Fatalf("RPC 踢出应成功, got ok=%v err=%+v", rpcResp.OK, rpcResp.Error)
	}

	// 审计行断言：恰好一条 conn_kick，操作者归因 rpc/操作者记录 ID，目标字段为受害侧
	var kicks []service.AuditRecord
	for _, rec := range readAuditRecords(t, auditPath) {
		if rec.Event == service.AuditEventConnKick {
			kicks = append(kicks, rec)
		}
	}
	if len(kicks) != 1 {
		t.Fatalf("RPC 踢出生效应恰好写一条 conn_kick 审计, got %d", len(kicks))
	}
	kick := kicks[0]
	if kick.ActorSource != service.AuditActorRPC {
		t.Errorf("RPC 踢出 actorSource 应为 rpc, got %q", kick.ActorSource)
	}
	if kick.ActorTokenID != actorRec.ID {
		t.Errorf("actorTokenId 应为调用者令牌记录 ID %q, got %q", actorRec.ID, kick.ActorTokenID)
	}
	if kick.TokenID != victimTokenID || kick.ConnID != victimConnID {
		t.Errorf("被踢目标字段应为受害连接侧 (tokenId=%q connId=%d), got tokenId=%q connId=%d",
			victimTokenID, victimConnID, kick.TokenID, kick.ConnID)
	}
	// 无明文泄露：审计文件不含操作者明文令牌
	assertAuditNoPlaintext(t, auditPath, actorPlain)

	// 装饰 target 提升路径：非 override 方法（GetAppVersion）经 rpcAuditTarget
	// embed 提升同构可达且行为不变（防漏提升致其余方法 RPC 漂移；字段与提升方法
	// 同名冲突在 Go 语义下编译失败，运行时风险仅剩提升可达性，此处钉住）
	promoteReq, err := http.NewRequest(http.MethodPost, "http://"+m.listenAddr()+"/api/rpc", strings.NewReader(`{"method":"GetAppVersion","args":[]}`))
	if err != nil {
		t.Fatalf("构造提升路径请求: %v", err)
	}
	promoteReq.Header.Set("Authorization", "Bearer "+actorPlain)
	promoteResp, err := http.DefaultClient.Do(promoteReq)
	if err != nil {
		t.Fatalf("提升路径请求失败: %v", err)
	}
	defer promoteResp.Body.Close()
	var promoteRpcResp struct {
		OK   bool   `json:"ok"`
		Data string `json:"data"`
	}
	if err := json.NewDecoder(promoteResp.Body).Decode(&promoteRpcResp); err != nil {
		t.Fatalf("解析提升路径响应: %v", err)
	}
	if !promoteRpcResp.OK || promoteRpcResp.Data != version {
		t.Errorf("非 override 方法经装饰 target 应提升可达且返回版本号, got ok=%v data=%q want %q",
			promoteRpcResp.OK, promoteRpcResp.Data, version)
	}
}

// TestApp_QueryAuditLog 审计读侧桌面绑定（连接管控三期）：委托 AuditLogger 读侧
// （过滤/分页语义由 service 层测试覆盖，此处验证绑定层委托与空值收敛）；
// webAuditSvc 未装配（旁路构造）返回空结果不报错（审计为旁路观测面）。
func TestApp_QueryAuditLog(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	// 基线：NewAppServices 引导/迁移可能已落 token_migrate 行，以下断言相对基线
	base, err := app.QueryAuditLog(service.AuditQueryOptions{})
	if err != nil {
		t.Fatalf("基线查询: %v", err)
	}

	app.webAuditSvc.Log(service.AuditEventAuthSuccess, "tok-1", "", "127.0.0.1:1", "ua")
	app.webAuditSvc.LogConnKick(service.KickAudit{
		ActorSource:   service.AuditActorDesktop,
		TargetTokenID: "tok-2",
		TargetName:    "手机",
		RemoteAddr:    "127.0.0.1:2",
		ConnID:        9,
	})

	// 全量：基线 + 2
	res, err := app.QueryAuditLog(service.AuditQueryOptions{})
	if err != nil {
		t.Fatalf("查询: %v", err)
	}
	if res.Total != base.Total+2 || len(res.Records) != base.Total+2 {
		t.Fatalf("全量应基线+2 行, got total=%d base=%d", res.Total, base.Total)
	}
	// 时间倒序（最新在前）：conn_kick 为后写行
	if res.Records[0].Event != service.AuditEventConnKick {
		t.Errorf("倒序首行应为 conn_kick, got %+v", res.Records[0])
	}

	// 事件过滤（引导期无 auth_success 行，恰好命中本用例写入的 1 条）
	res, err = app.QueryAuditLog(service.AuditQueryOptions{Events: []string{service.AuditEventAuthSuccess}})
	if err != nil {
		t.Fatalf("过滤查询: %v", err)
	}
	if res.Total != 1 || res.Records[0].Event != service.AuditEventAuthSuccess {
		t.Errorf("事件过滤应仅 auth_success: total=%d", res.Total)
	}

	// Records 恒非 nil（前端消费契约）：越界页仍返回空切片
	res, err = app.QueryAuditLog(service.AuditQueryOptions{Offset: 99})
	if err != nil {
		t.Fatalf("越界查询: %v", err)
	}
	if res.Records == nil || len(res.Records) != 0 {
		t.Errorf("越界页 Records 应为空切片非 nil: %+v", res.Records)
	}

	// webAuditSvc 未装配：空结果不报错
	app.webAuditSvc = nil
	res, err = app.QueryAuditLog(service.AuditQueryOptions{})
	if err != nil {
		t.Fatalf("未装配查询不应报错: %v", err)
	}
	if res.Total != 0 || len(res.Records) != 0 {
		t.Errorf("未装配应空结果, got %+v", res)
	}
}

// TestServeRPC_QueryAuditLogHiddenForBrowser 审计读侧对浏览器通道屏蔽（隐私
// 红线，连接管控三期）：RPCHiddenMethods 命中返回与「方法不存在」同响应（不
// 暴露被屏蔽方法存在性）；同 handler 上非屏蔽方法不受影响；桌面 Wails 绑定不经
// 翻译层（App.QueryAuditLog 直调读侧，见 TestApp_QueryAuditLog），不受屏蔽影响。
func TestServeRPC_QueryAuditLogHiddenForBrowser(t *testing.T) {
	handler := server.NewWebHandler(server.WebOptions{
		Assets:           nil,
		Verify:           server.StaticVerifier("hidden-test-token"),
		RPCTarget:        NewApp(),
		RPCHiddenMethods: []string{"QueryAuditLog"},
	})

	post := func(method string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"`+method+`","args":[]}`))
		req.Header.Set("Authorization", "Bearer hidden-test-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	// 屏蔽方法：与真不存在同响应（同错误码、同消息形态）
	code, body := post("QueryAuditLog")
	if code != http.StatusOK {
		t.Fatalf("屏蔽拒绝应 HTTP 200 协议应答, got %d", code)
	}
	if !strings.Contains(body, server.ErrCodeRPCMethodNotFound) {
		t.Errorf("屏蔽方法应返回 %s, got %q", server.ErrCodeRPCMethodNotFound, body)
	}
	_, NoSuchBody := post("NoSuchMethodDefinitely")
	if !strings.Contains(NoSuchBody, server.ErrCodeRPCMethodNotFound) {
		t.Fatalf("前置：真不存在方法应返回 %s", server.ErrCodeRPCMethodNotFound)
	}
	// 同消息形态（错误码与消息模板一致，仅方法名回显不同——回显请求方输入不泄露存在性）
	if !strings.Contains(body, "方法不存在: QueryAuditLog") {
		t.Errorf("屏蔽响应应与不存在方法同消息形态: %q", body)
	}

	// 非屏蔽方法不受影响
	code, body = post("GetAppVersion")
	if code != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Errorf("非屏蔽方法应正常调用, got %d %q", code, body)
	}
}

// assertAuditNoPlaintext 断言审计文件不含给定明文令牌（无敏感泄露红线）。
func assertAuditNoPlaintext(t *testing.T, path, plain string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读审计日志: %v", err)
	}
	if strings.Contains(string(raw), plain) {
		t.Error("审计日志不得包含明文 token")
	}
}
