package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// httptest.NewRequest 的默认来源地址（非回环）：IP 封禁判定按此 IP 匹配注入桩。
const testIPBannedAddr = "192.0.2.1"

// ipBanStub 构造按来源 IP 精确匹配的 IPBanCheck 测试桩：命中 bannedIP 返回固定
// 拒绝消息，其余放行（fake 注入形态，不依赖 service.IPBanService——server 不知
// service，注入面以本函数签名隔离）。
func ipBanStub(bannedIP, msg string) IPBanCheck {
	return func(ip string) string {
		if bannedIP != "" && strings.HasPrefix(ip, bannedIP) {
			return msg
		}
		return ""
	}
}

// verifyCountingStub 构造带调用计数的 verify 包装（StaticVerifier 语义 + 计数），
// 供「IP 封禁命中时 verify 不被调用」断言使用。
func verifyCountingStub(validToken string) (TokenVerifier, func() int) {
	var mu sync.Mutex
	calls := 0
	fn := func(candidate string) (string, bool) {
		mu.Lock()
		calls++
		mu.Unlock()
		if candidate == validToken {
			return "static", true
		}
		return "", false
	}
	return fn, func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
}

// TestWebHandler_IPBannedRejected HTTP 认证点 IP 封禁拒连：命中生效中的自动封禁
// 以 401 拒绝并透出含自动恢复时刻的专用消息（与「请携带访问令牌」可区分），会话
// cookie 不种（被封禁来源的新会话无意义）；BanCheck（tokenID 维度）未注入也不影响
// IP 维度独立判定（两道防线并存）。
func TestWebHandler_IPBannedRejected(t *testing.T) {
	h := NewWebHandler(WebOptions{
		Assets:     testAssets(),
		Verify:     StaticVerifier(testToken),
		IPBanCheck: ipBanStub(testIPBannedAddr, "该来源 IP 已被临时封禁，至 2026-10-26 12:00 自动恢复"),
	})
	rec := get(t, h, "/", map[string]string{"Authorization": "Bearer " + testToken})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("被封禁来源 IP 应 401, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "自动恢复") {
		t.Errorf("401 应携带 IP 封禁专用消息, got %q", body)
	}
	if strings.Contains(body, "请携带访问令牌") {
		t.Errorf("IP 封禁消息不得与普通未授权文案混淆, got %q", body)
	}
	if cookies := rec.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Errorf("IP 封禁拒连不应种会话 cookie, got %v", cookies)
	}
}

// TestWebHandler_IPBanCheckNilAndEmpty 放行三形态：IPBanCheck 未注入（nil）、注入
// 但未命中（空串）、未被封禁的其他来源 IP——均正常放行（IP 封禁为可选增强面，
// 不改变既有认证语义）。
func TestWebHandler_IPBanCheckNilAndEmpty(t *testing.T) {
	header := map[string]string{"Authorization": "Bearer " + testToken}
	if rec := get(t, newTestHandler(), "/", header); rec.Code != http.StatusOK {
		t.Errorf("nil IPBanCheck 应放行, got %d", rec.Code)
	}
	h := NewWebHandler(WebOptions{Assets: testAssets(), Verify: StaticVerifier(testToken), IPBanCheck: ipBanStub("", "")})
	if rec := get(t, h, "/", header); rec.Code != http.StatusOK {
		t.Errorf("空串判定应放行, got %d", rec.Code)
	}
	// 其他来源 IP 未被封禁：放行（httptest.NewRequest 的 RemoteAddr 为
	// 192.0.2.1:1234，桩只封 198.51.100.5）
	h2 := NewWebHandler(WebOptions{Assets: testAssets(), Verify: StaticVerifier(testToken), IPBanCheck: ipBanStub("198.51.100.5", "banned")})
	if rec := get(t, h2, "/", header); rec.Code != http.StatusOK {
		t.Errorf("未命中封禁的来源 IP 应放行, got %d", rec.Code)
	}
}

// TestWebHandler_IPBanCheckBeforeVerify verify 前置判定断言：来源 IP 被封时即使
// 携带完全有效的令牌也 401，且 verify 一次都不被调用（凭据校验不发生——与四期
// BanCheck 的 verify 放行后组合判定在位置上可区分，两道防线独立）。
func TestWebHandler_IPBanCheckBeforeVerify(t *testing.T) {
	verify, calls := verifyCountingStub(testToken)
	h := NewWebHandler(WebOptions{
		Assets:     testAssets(),
		Verify:     verify,
		IPBanCheck: ipBanStub(testIPBannedAddr, "该来源 IP 已被临时封禁，至 2026-10-26 12:00 自动恢复"),
	})
	rec := get(t, h, "/", map[string]string{"Authorization": "Bearer " + testToken})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("被封禁来源携带有效令牌也应 401, got %d", rec.Code)
	}
	if n := calls(); n != 0 {
		t.Errorf("IP 封禁命中时 verify 不应被调用, got %d 次", n)
	}
}

// TestWebHandler_IPBannedNoAuthEvent IP 封禁拒连的认证事件形态：不发
// auth_failure 也不发 auth_success（判定发生在任何凭据校验之前，凭据未验无认证
// 成败语义，也不经 OnAuth 参与失败计数），与四期封禁形态（auth_failure 且
// TokenID 非空）在审计面可区分。
func TestWebHandler_IPBannedNoAuthEvent(t *testing.T) {
	var mu sync.Mutex
	var events []AuthEvent
	h := NewWebHandler(WebOptions{
		Assets:     testAssets(),
		Verify:     StaticVerifier(testToken),
		IPBanCheck: ipBanStub(testIPBannedAddr, "banned"),
		OnAuth: func(ev AuthEvent) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, ev)
		},
	})
	get(t, h, "/", map[string]string{"Authorization": "Bearer " + testToken})
	get(t, h, "/", nil)

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 0 {
		t.Fatalf("IP 封禁拒连不应触发任何认证事件, got %+v", events)
	}
}

// TestWSHub_IPBannedHandshakeRejected WS 握手 IP 封禁拒连：升级前 401 + 专用
// 消息，连接不建立（查询参数与子协议两通道同款）；解封后（桩改放行）恢复可达。
func TestWSHub_IPBannedHandshakeRejected(t *testing.T) {
	banned := true
	hub := NewWSHub(StaticVerifier("right-token"), nil, nil, func(ip string) string {
		if banned {
			return "该来源 IP 已被临时封禁，至 2026-10-26 12:00 自动恢复"
		}
		return ""
	})
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name  string
		query string
		subs  []string
	}{
		{"查询参数通道", "?token=right-token", nil},
		{"子协议通道", "", []string{"right-token"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, resp := dialWS(t, srv, tc.query, tc.subs)
			if conn != nil {
				t.Fatal("被封禁来源 IP 升级应失败")
			}
			if resp == nil || resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("应 HTTP 401 拒绝升级, got %v", resp)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if !strings.Contains(string(body), "自动恢复") {
				t.Errorf("401 应携带 IP 封禁专用消息, got %q", string(body))
			}
		})
	}

	// 解封（懒过期放行形态）后恢复可达
	banned = false
	mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
}

// TestWSHub_IPBannedNoAuthEvent WS 握手 IP 封禁拒连的认证事件形态：不发
// auth_failure/auth_success（verify 之前判定，凭据未验），与 HTTP 认证点同款
// 口径。httptest 客户端来源恒为回环地址，桩按无条件封禁注入（对齐 banAll 先例）。
func TestWSHub_IPBannedNoAuthEvent(t *testing.T) {
	var mu sync.Mutex
	var events []AuthEvent
	verify, calls := verifyCountingStub("right-token")
	hub := NewWSHub(verify, func(ev AuthEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}, nil, func(string) string { return "banned" })
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	if conn, resp := dialWS(t, srv, "?token=right-token", nil); conn != nil {
		t.Fatal("被封禁来源 IP 升级应失败")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("应 401, got %v", resp)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 0 {
		t.Fatalf("IP 封禁拒连不应触发任何认证事件, got %+v", events)
	}
	if n := calls(); n != 0 {
		t.Errorf("IP 封禁命中时 verify 不应被调用, got %d 次", n)
	}
}
