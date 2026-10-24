package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// banAll 构造恒封禁的 BanCheck（返回固定拒绝消息，测试注入形态）。
func banAll(msg string) BanCheck {
	return func(tokenID string) string {
		if tokenID == "" {
			return ""
		}
		return msg
	}
}

// banNone 构造恒放行的 BanCheck（非 nil 注入形态：未命中封禁）。
func banNone() BanCheck {
	return func(string) string { return "" }
}

// TestWebHandler_BannedTokenRejected HTTP 认证点封禁拒连：verify 放行但封禁
// 命中以 401 拒绝，响应体携带专用消息（与「请携带访问令牌」可区分），会话
// cookie 不种（封禁令牌的新会话无意义）。
func TestWebHandler_BannedTokenRejected(t *testing.T) {
	h := NewWebHandler(WebOptions{
		Assets:   testAssets(),
		Verify:   StaticVerifier(testToken),
		BanCheck: banAll("该访问令牌已被封禁（永久封禁），请联系管理员解封"),
	})
	rec := get(t, h, "/", map[string]string{"Authorization": "Bearer " + testToken})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("封禁令牌应 401, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "永久封禁") {
		t.Errorf("401 应携带封禁专用消息, got %q", body)
	}
	if strings.Contains(body, "请携带访问令牌") {
		t.Errorf("封禁消息不得与普通未授权文案混淆, got %q", body)
	}
	if cookies := rec.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Errorf("封禁拒连不应种会话 cookie, got %v", cookies)
	}
	// BanCheck 收到的 tokenID 为 verify 命中的记录 ID（StaticVerifier 恒 "static"）
	sawID := ""
	h2 := NewWebHandler(WebOptions{
		Assets: testAssets(),
		Verify: StaticVerifier(testToken),
		BanCheck: func(id string) string {
			sawID = id
			return ""
		},
	})
	if rec := get(t, h2, "/", map[string]string{"Authorization": "Bearer " + testToken}); rec.Code != http.StatusOK {
		t.Fatalf("放行形态应 200, got %d", rec.Code)
	}
	if sawID != "static" {
		t.Errorf("BanCheck 应收到命中记录 ID %q, got %q", "static", sawID)
	}
}

// TestWebHandler_BanCheckNilAndEmpty 放行两形态：BanCheck 未注入（nil）与注入
// 但未命中（空串）均正常放行（封禁为可选增强面，不改变既有认证语义）。
func TestWebHandler_BanCheckNilAndEmpty(t *testing.T) {
	header := map[string]string{"Authorization": "Bearer " + testToken}
	if rec := get(t, newTestHandler(), "/", header); rec.Code != http.StatusOK {
		t.Errorf("nil BanCheck 应放行, got %d", rec.Code)
	}
	h := NewWebHandler(WebOptions{Assets: testAssets(), Verify: StaticVerifier(testToken), BanCheck: banNone()})
	if rec := get(t, h, "/", header); rec.Code != http.StatusOK {
		t.Errorf("空串判定应放行, got %d", rec.Code)
	}
}

// TestWebHandler_BannedAuthEventShape 封禁拒连的审计形态：auth_failure 且
// TokenID 填命中值（与普通认证失败恒空可区分），不发 auth_success。
func TestWebHandler_BannedAuthEventShape(t *testing.T) {
	var mu sync.Mutex
	var events []AuthEvent
	h := NewWebHandler(WebOptions{
		Assets:   testAssets(),
		Verify:   StaticVerifier(testToken),
		BanCheck: banAll("banned"),
		OnAuth: func(ev AuthEvent) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, ev)
		},
	})
	get(t, h, "/", map[string]string{"Authorization": "Bearer " + testToken})

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("封禁拒连应恰发一条认证事件, got %d", len(events))
	}
	ev := events[0]
	if ev.Event != AuthEventFailure {
		t.Errorf("封禁拒连应发 auth_failure, got %q", ev.Event)
	}
	if ev.TokenID != "static" {
		t.Errorf("封禁形态 TokenID 应填命中值 %q, got %q", "static", ev.TokenID)
	}
}

// TestWSHub_BannedHandshakeRejected WS 握手封禁拒连：升级前 401 + 专用消息，
// 连接不建立；未封禁令牌照常升级（同 hub 对照）；解封后恢复可达。
func TestWSHub_BannedHandshakeRejected(t *testing.T) {
	banned := true
	hub := NewWSHub(StaticVerifier("right-token"), nil, func(id string) string {
		if banned && id == "static" {
			return "该访问令牌已被封禁，至 2026-10-25 12:00 自动解封"
		}
		return ""
	})
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	// 封禁中：查询参数与子协议两通道均升级前 401
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
				t.Fatal("封禁令牌升级应失败")
			}
			if resp == nil || resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("应 HTTP 401 拒绝升级, got %v", resp)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if !strings.Contains(string(body), "自动解封") {
				t.Errorf("401 应携带封禁专用消息, got %q", string(body))
			}
		})
	}

	// 解封后恢复可达
	banned = false
	mustDialWS(t, srv, "?token=right-token")
	waitClientCount(t, hub, 1)
}

// TestWSHub_BannedHandshakeAuthEvent WS 封禁拒连的审计形态（auth_failure +
// tokenID 非空，不发 auth_success），与 HTTP 认证点同款口径。
func TestWSHub_BannedHandshakeAuthEvent(t *testing.T) {
	var mu sync.Mutex
	var events []AuthEvent
	hub := NewWSHub(StaticVerifier("right-token"), func(ev AuthEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}, banAll("banned"))
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	if conn, resp := dialWS(t, srv, "?token=right-token", nil); conn != nil {
		t.Fatal("封禁令牌升级应失败")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("应 401, got %v", resp)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("应恰发一条认证事件, got %d", len(events))
	}
	if events[0].Event != AuthEventFailure || events[0].TokenID != "static" {
		t.Errorf("应发 auth_failure 且 TokenID 非空, got %+v", events[0])
	}
	if events[0].At.IsZero() {
		t.Error("事件时间应就绪")
	}
}
