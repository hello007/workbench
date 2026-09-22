package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workbench/server"
)

// TestServeCookie_AssetCascadeSmoke 首访资产级联缺陷修复冒烟（真实 embed 资产
// 全链路）：浏览器打开 /?token=<t> 后，后续 /assets/*.js 静态资产请求不再携带
// ?token=（查询参数只随单个请求发送），修复前全部 401、应用永远启动不了。
// 链路断言：
//  1. GET /?token=<t> → 200 + Set-Cookie（wb_token 会话）；
//  2. 仅凭 cookie GET /assets/<真实嵌入文件名> → 200（从 embed FS 取真实
//     产物文件名，而非测试造的假名）；
//  3. 无任何凭据 GET /assets/<同一文件> → 401（fail closed 不放宽）；
//  4. 仅凭 cookie POST /api/rpc GetAppVersion → 200（RPC 同受会话贯通）。
func TestServeCookie_AssetCascadeSmoke(t *testing.T) {
	distFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		t.Fatalf("定位前端静态资产失败: %v", err)
	}
	// 从 embed FS 取真实产物资产文件名（Vite 带 content hash，名随构建变化）
	matches, err := fs.Glob(distFS, "assets/*.js")
	if err != nil || len(matches) == 0 {
		t.Fatalf("embed 资产中未找到 JS 产物: %v", err)
	}
	assetName := matches[0]

	handler := server.NewWebHandler(server.WebOptions{
		Assets:    distFS,
		Token:     "smoke-token",
		RPCTarget: NewApp(),
	})

	// 1. 首访首页：query token 认证成功并种会话 cookie
	req := httptest.NewRequest(http.MethodGet, "/?token=smoke-token", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("首访应 200, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "wb_token" || cookies[0].Value != "smoke-token" {
		t.Fatalf("首访应种 wb_token 会话 cookie, got %v", cookies)
	}
	if !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Errorf("会话 cookie 应 HttpOnly + SameSite=Strict, got %v", cookies[0])
	}

	// 2. 浏览器级联资产请求：仅凭 cookie 200
	req = httptest.NewRequest(http.MethodGet, "/"+assetName, nil)
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("cookie 会话资产请求 /%s 应 200, got %d", assetName, rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Errorf("JS 资产 Content-Type 应为 javascript, got %q", rec.Header().Get("Content-Type"))
	}

	// 3. 无任何凭据的同一资产请求：401
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+assetName, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无凭据资产请求应 401, got %d", rec.Code)
	}

	// 4. RPC 经 cookie 会话认证：浏览器 shim 无 localStorage 令牌场景走通
	req = httptest.NewRequest(http.MethodPost, "/api/rpc", strings.NewReader(`{"method":"GetAppVersion","args":[]}`))
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie 会话 RPC 应 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `{"ok":true`) {
		t.Errorf("GetAppVersion 应成功应答, got %q", body)
	}
}
