package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
	"workbench/service"
)

// 连接管控七期测试（App 层）：手动封禁 BanIP 绑定面（委托/校验拒绝/RPC 归因）
// 与 SetIPBanConfig 五参白名单保存链。服务层状态机语义由 service 层 v7 测试覆盖，
// 此处只测绑定委托、错误码透传与装配接线。

// TestApp_BanIP_Delegate 手动封禁绑定层：委托服务层状态机（封禁生效/列表
// Source=manual/FailCount=0/strikes 不动），覆盖在途自动封禁（bannedUntil 顺延、
// source 置 manual），解封后可达，服务未装配（旁路构造）报错不崩。
func TestApp_BanIP_Delegate(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	ip := testAuthFailIP

	// 触发自动封禁打底（strikes=1），手动封禁覆盖
	for i := 0; i < service.IPBanThreshold; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	if app.ipBanSvc.RejectMessage(ip) == "" {
		t.Fatal("前置：自动封禁应生效")
	}

	// 手动永久封禁：覆盖自动条目，恒拒连（永久文案）
	if err := app.BanIP(ip, 0, ""); err != nil {
		t.Fatalf("BanIP: %v", err)
	}
	msg := app.ipBanSvc.RejectMessage(ip)
	if !strings.Contains(msg, "永久") {
		t.Errorf("永久封禁拒连消息应含「永久」, got %q", msg)
	}
	bans := app.ListIPBans()
	if len(bans) != 1 {
		t.Fatalf("覆盖封禁记录应恒唯一, got %+v", bans)
	}
	if bans[0].Source != model.IPBanSourceManual {
		t.Errorf("手动封禁后 Source 应为 manual, got %q", bans[0].Source)
	}
	if bans[0].ExpiresAt != 0 || bans[0].Expired {
		t.Errorf("永久封禁 ExpiresAt 应为 0 且未过期, got %+v", bans[0])
	}
	if bans[0].Strike != 1 {
		t.Errorf("手动封禁不应改写 strikes（保留自动史档位）, got %d", bans[0].Strike)
	}
	if bans[0].FailCount != 0 {
		t.Errorf("手动封禁 FailCount 应清零（无自动触发语境）, got %d", bans[0].FailCount)
	}

	// 定时档：expiresAt 晚于当前时刻合法
	future := time.Now().Add(time.Hour).Unix()
	if err := app.BanIP(ip, future, "异常行为"); err != nil {
		t.Fatalf("BanIP 定时档: %v", err)
	}
	bans = app.ListIPBans()
	if len(bans) != 1 || bans[0].ExpiresAt != future {
		t.Fatalf("定时封禁到期时刻应刷新, got %+v", bans)
	}

	// 解封：可达 + 记录删除
	if err := app.UnbanIP(ip); err != nil {
		t.Fatalf("UnbanIP: %v", err)
	}
	if app.ipBanSvc.RejectMessage(ip) != "" {
		t.Error("解封后新连接应可达")
	}

	// 服务未装配：报错不崩
	app.ipBanSvc = nil
	if err := app.BanIP(ip, 0, ""); err == nil {
		t.Error("ipBanSvc 未装配时手动封禁应报错")
	}
}

// TestApp_BanIP_ValidationAndWhitelistRejects 手动封禁校验拒绝透传（AppError
// 错误码面）：空 IP（E_IP_BAN_IP_REQUIRED）、过期时刻（E_TOKEN_EXPIRY_INVALID）、
// 白名单/回环 IP 禁封（E_IP_BAN_WHITELISTED）。
func TestApp_BanIP_ValidationAndWhitelistRejects(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	// 空 IP
	err := app.BanIP("  ", 0, "")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeIPBanIPRequired {
		t.Errorf("空 IP 应返 E_IP_BAN_IP_REQUIRED, got %v", err)
	}

	// 过期时刻（非 0 且不晚于当前）
	err = app.BanIP(testAuthFailIP, 1, "")
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTokenExpiryInvalid {
		t.Errorf("过期时刻应返 E_TOKEN_EXPIRY_INVALID, got %v", err)
	}

	// 白名单 IP 禁封：先配置白名单（SetIPBanConfig 五参），再封命中 IP
	if err := app.SetIPBanConfig(10, 10, 5, 60, 3, 7, []string{"192.0.2.99"}); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	err = app.BanIP("192.0.2.99", 0, "")
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeIPBanWhitelisted {
		t.Errorf("白名单 IP 应返 E_IP_BAN_WHITELISTED, got %v", err)
	}

	// 回环 IP 禁封（回环豁免收编同一判定）
	err = app.BanIP("127.0.0.1", 0, "")
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeIPBanWhitelisted {
		t.Errorf("回环 IP 应返 E_IP_BAN_WHITELISTED, got %v", err)
	}
	// 拒绝形态不产生条目
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Errorf("校验拒绝不应产生封禁记录, got %+v", bans)
	}
}

// TestApp_BanIP_RPCActorAudit RPC 通道手动封禁（rpcAuditTarget 装饰 override）：
// actor=rpc/调用者令牌记录 ID 归因 ip_ban 审计行，reason 空入参生成缺省文案，
// 无明文 token 泄露。
func TestApp_BanIP_RPCActorAudit(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)
	auditPath := filepath.Join(dataDir, "web_audit.log")
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	actorPlain, actorRec, err := app.webTokenSvc.Create("封禁操作者", 0)
	if err != nil {
		t.Fatalf("创建操作者令牌: %v", err)
	}
	ip := testAuthFailIP

	callBanIP := func(body string) (bool, string) {
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

	// RPC 永久封禁（reason 空 → 缺省文案）
	ok, raw := callBanIP(`{"method":"BanIP","args":["` + ip + `",0,""]}`)
	if !ok {
		t.Fatalf("RPC 手动封禁应成功, got %s", raw)
	}
	if app.ipBanSvc.RejectMessage(ip) == "" {
		t.Error("RPC 手动封禁后新连接应被拒")
	}

	// 审计行：actor=rpc/操作者记录 ID，reason 为缺省文案，FailCount/Strike 不落键
	var banRows []service.AuditRecord
	for _, rec := range readAuditRecords(t, auditPath) {
		if rec.Event == service.AuditEventIPBan {
			banRows = append(banRows, rec)
		}
	}
	if len(banRows) != 1 {
		t.Fatalf("应恰有 1 条 ip_ban 审计行, got %d", len(banRows))
	}
	row := banRows[0]
	if row.IP != ip || row.ActorSource != service.AuditActorRPC || row.ActorTokenID != actorRec.ID {
		t.Errorf("RPC 手动封禁归因应为 rpc/%s, got %+v", actorRec.ID, row)
	}
	if row.Reason != "管理员手动封禁（永久）" {
		t.Errorf("reason 空入参应生成缺省文案, got %q", row.Reason)
	}
	if row.FailCount != 0 || row.Strike != 0 {
		t.Errorf("手动封禁审计行不应携带自动触发计数, got %+v", row)
	}
	assertAuditNoPlaintext(t, auditPath, actorPlain)
}

// TestApp_SetIPBanConfig_Whitelist 白名单五参保存链：归一（非法 CIDR 条目剔除、
// 合法条目保留原串）持久化 + ApplyConfig 热更即时生效（白名单 IP 认证失败不计数
// 不封禁）+ 生效中封禁追溯放行/移除恢复拒连 + 其他指针段保留。
func TestApp_SetIPBanConfig_Whitelist(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	ip := testAuthFailIP

	// 封禁生效打底（追溯放行断言前置）
	for i := 0; i < service.IPBanThreshold; i++ {
		app.ipBanSvc.RecordFailure(ip)
	}
	if app.ipBanSvc.RejectMessage(ip) == "" {
		t.Fatal("前置：封禁应生效")
	}

	// 白名单含命中 IP（裸 IP 形态）+ 非法条目（剔除）+ 合法 CIDR（保留原串）
	whitelist := []string{ip, "not-a-cidr", "10.0.0.0/8", "  "}
	if err := app.SetIPBanConfig(10, 10, 5, 60, 3, 7, whitelist); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}

	// 持久化归一：非法剔除、合法保留原串（不做 CIDR 规范化改写）
	got := app.GetSettings().IPBan
	if got == nil {
		t.Fatal("ipban 段应存在")
	}
	want := []string{ip, "10.0.0.0/8"}
	if len(got.Whitelist) != len(want) {
		t.Fatalf("白名单应剔除非法条目后保留 %v, got %v", want, got.Whitelist)
	}
	for i := range want {
		if got.Whitelist[i] != want[i] {
			t.Errorf("白名单条目[%d] 应为 %q（保留原串）, got %q", i, want[i], got.Whitelist[i])
		}
	}

	// 追溯放行：生效中封禁命中白名单即放行（在途记录不删）
	if msg := app.ipBanSvc.RejectMessage(ip); msg != "" {
		t.Errorf("白名单命中应追溯放行生效中封禁, got %q", msg)
	}
	bans := app.ListIPBans()
	if len(bans) != 1 {
		t.Fatalf("追溯放行不应删除在途记录, got %+v", bans)
	}

	// 白名单 IP 认证失败不计数：另一白名单段内 IP 连续失败不触发封禁
	for i := 0; i < service.IPBanThreshold*2; i++ {
		app.ipBanSvc.RecordFailure("10.1.2.3")
	}
	if msg := app.ipBanSvc.RejectMessage("10.1.2.3"); msg != "" {
		t.Errorf("白名单网段内 IP 失败不应计数封禁, got %q", msg)
	}

	// 移除白名单 → 生效中封禁恢复拒连（未过期）
	if err := app.SetIPBanConfig(10, 10, 5, 60, 3, 7, []string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("SetIPBanConfig 移除: %v", err)
	}
	if msg := app.ipBanSvc.RejectMessage(ip); msg == "" {
		t.Error("白名单移除后未过期封禁应恢复拒连")
	}

	// webServe 段保留（指针段复制纪律，与六期用例同口径）
	if err := app.SetWebServeConfig(false, "127.0.0.1:36115"); err != nil {
		t.Fatalf("SetWebServeConfig: %v", err)
	}
	if err := app.SetIPBanConfig(10, 10, 5, 60, 3, 7, nil); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	cfg := app.GetSettings()
	if cfg.WebServe == nil || cfg.WebServe.Enabled {
		t.Errorf("webServe 段应保留（enabled=false）, got %+v", cfg.WebServe)
	}
	if cfg.IPBan == nil || len(cfg.IPBan.Whitelist) != 0 {
		t.Errorf("白名单空列表应写入（恒非 nil）, got %+v", cfg.IPBan)
	}
}
