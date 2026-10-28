package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/service"
)

// 连接管控八期测试（App 层）：CIDR 网段手动封禁 BanCIDR/UnbanCIDR 绑定面
// （委托/校验拒绝/RPC 归因）。服务层状态机语义由 service 层 cidr 测试覆盖，
// 此处只测绑定委托、错误码透传与装配接线（对齐七期 app_webserve_ipban_v7_test
// 形态）。

// TestApp_BanCIDR_Delegate 网段手动封禁绑定层：委托服务层状态机（网段封禁生效/
// List 条目 Kind=cidr/Source=manual）、解封后可达、服务未装配（旁路构造）报错
// 不崩（封禁与解封两入口同测）。
func TestApp_BanCIDR_Delegate(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)
	ip := "203.0.113.44"

	if err := app.BanCIDR("203.0.113.0/24", 0, "网段处置"); err != nil {
		t.Fatalf("BanCIDR: %v", err)
	}
	if app.ipBanSvc.RejectMessage(ip) == "" {
		t.Error("网段封禁后新连接应被拒")
	}
	bans := app.ListIPBans()
	if len(bans) != 1 {
		t.Fatalf("应恰有 1 条网段记录, got %+v", bans)
	}
	if bans[0].Kind != model.IPBanKindCidr || bans[0].IP != "203.0.113.0/24" {
		t.Errorf("网段条目应为 cidr/规范化串, got %+v", bans[0])
	}
	if bans[0].Source != model.IPBanSourceManual {
		t.Errorf("网段条目 Source 应为 manual, got %q", bans[0].Source)
	}

	// 解封：可达 + 记录删除
	if err := app.UnbanCIDR("203.0.113.0/24"); err != nil {
		t.Fatalf("UnbanCIDR: %v", err)
	}
	if app.ipBanSvc.RejectMessage(ip) != "" {
		t.Error("解封后新连接应可达")
	}
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Errorf("解封后记录应删除, got %+v", bans)
	}

	// 服务未装配：报错不崩（封禁/解封两入口）
	app.ipBanSvc = nil
	if err := app.BanCIDR("203.0.113.0/24", 0, ""); err == nil {
		t.Error("ipBanSvc 未装配时网段封禁应报错")
	}
	if err := app.UnbanCIDR("203.0.113.0/24"); err == nil {
		t.Error("ipBanSvc 未装配时网段解封应报错")
	}
}

// TestApp_BanCIDR_ValidationRejects 网段手动封禁校验拒绝透传（AppError 错误码
// 面）：非法网段（E_IP_BAN_CIDR_INVALID）、过期时刻（E_TOKEN_EXPIRY_INVALID）、
// 白名单完全覆盖（E_IP_BAN_CIDR_WHITELISTED）；拒绝形态不产生条目。
func TestApp_BanCIDR_ValidationRejects(t *testing.T) {
	app, _ := newWebServeTestAppDir(t)

	var appErr *model.AppError
	err := app.BanCIDR("not-a-cidr", 0, "")
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeIPBanCidrInvalid {
		t.Errorf("非法网段应返 E_IP_BAN_CIDR_INVALID, got %v", err)
	}

	err = app.BanCIDR("203.0.113.0/24", 1, "")
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTokenExpiryInvalid {
		t.Errorf("过期时刻应返 E_TOKEN_EXPIRY_INVALID, got %v", err)
	}

	// 白名单完全覆盖：先配置覆盖性白名单，再封同网段
	if err := app.SetIPBanConfig(10, 10, 5, 60, 3, 7, []string{"203.0.113.0/24"}); err != nil {
		t.Fatalf("SetIPBanConfig: %v", err)
	}
	err = app.BanCIDR("203.0.113.0/24", 0, "")
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeIPBanCidrWhitelisted {
		t.Errorf("白名单完全覆盖应返 E_IP_BAN_CIDR_WHITELISTED, got %v", err)
	}
	if bans := app.ListIPBans(); len(bans) != 0 {
		t.Errorf("校验拒绝不应产生封禁记录, got %+v", bans)
	}
}

// TestApp_BanCIDR_RPCActorAudit RPC 通道网段封禁/解封（rpcAuditTarget 装饰
// override）：actor=rpc/调用者令牌记录 ID 归因审计行，IP 字段存 CIDR 串，
// reason 空入参生成网段缺省文案。
func TestApp_BanCIDR_RPCActorAudit(t *testing.T) {
	app, dataDir := newWebServeTestAppDir(t)
	auditPath := filepath.Join(dataDir, "web_audit.log")
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	actorPlain, actorRec, err := app.webTokenSvc.Create("网段封禁操作者", 0)
	if err != nil {
		t.Fatalf("创建操作者令牌: %v", err)
	}

	callRPC := func(body string) (bool, string) {
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

	// RPC 永久网段封禁（reason 空 → 网段缺省文案）
	ok, raw := callRPC(`{"method":"BanCIDR","args":["203.0.113.0/24",0,""]}`)
	if !ok {
		t.Fatalf("RPC 网段封禁应成功, got %s", raw)
	}
	if app.ipBanSvc.RejectMessage("203.0.113.10") == "" {
		t.Error("RPC 网段封禁后新连接应被拒")
	}

	// RPC 解封
	if ok, raw := callRPC(`{"method":"UnbanCIDR","args":["203.0.113.0/24"]}`); !ok {
		t.Fatalf("RPC 网段解封应成功, got %s", raw)
	}
	if app.ipBanSvc.RejectMessage("203.0.113.10") != "" {
		t.Error("RPC 解封后新连接应可达")
	}

	// 审计行：actor=rpc/操作者记录 ID，IP 字段存 CIDR 串，缺省文案带「网段」
	var banRows, unbanRows []service.AuditRecord
	for _, rec := range readAuditRecords(t, auditPath) {
		switch rec.Event {
		case service.AuditEventIPBan:
			banRows = append(banRows, rec)
		case service.AuditEventIPUnban:
			unbanRows = append(unbanRows, rec)
		}
	}
	if len(banRows) != 1 || len(unbanRows) != 1 {
		t.Fatalf("应恰有 1 条 ip_ban 与 1 条 ip_unban 审计行, got %d/%d", len(banRows), len(unbanRows))
	}
	ban := banRows[0]
	if ban.IP != "203.0.113.0/24" || ban.ActorSource != service.AuditActorRPC || ban.ActorTokenID != actorRec.ID {
		t.Errorf("RPC 网段封禁归因应为 rpc/%s 且 IP 存 CIDR 串, got %+v", actorRec.ID, ban)
	}
	if ban.Reason != "管理员手动封禁网段（永久）" {
		t.Errorf("reason 空入参应生成网段缺省文案, got %q", ban.Reason)
	}
	unban := unbanRows[0]
	if unban.IP != "203.0.113.0/24" || unban.ActorSource != service.AuditActorRPC || unban.ActorTokenID != actorRec.ID {
		t.Errorf("RPC 网段解封归因应为 rpc/%s 且 IP 存 CIDR 串, got %+v", actorRec.ID, unban)
	}
	assertAuditNoPlaintext(t, auditPath, actorPlain)
}
