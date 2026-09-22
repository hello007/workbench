package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// testToken 测试用访问令牌。
const testToken = "test-token-123"

// testAssets 构造含 index.html 与静态子目录的内存资产源。
func testAssets() fs.FS {
	return fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<html>spa-index</html>")},
		"assets/app.js":    &fstest.MapFile{Data: []byte("console.log('app')")},
		"assets/style.css": &fstest.MapFile{Data: []byte("body{}")},
	}
}

func newTestHandler() http.Handler {
	return NewWebHandler(WebOptions{Assets: testAssets(), Token: testToken})
}

// get 以指定 header 请求 handler 并返回响应。
func get(t *testing.T, h http.Handler, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestWebHandler_Healthz_NoAuth 健康检查免认证，未携带令牌也返回 200。
func TestWebHandler_Healthz_NoAuth(t *testing.T) {
	rec := get(t, newTestHandler(), healthPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("健康检查应 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != `{"status":"ok"}` {
		t.Errorf("健康检查响应不符: %q", body)
	}
}

// TestWebHandler_UnauthorizedWithoutToken 未携带令牌返回 401 JSON。
func TestWebHandler_UnauthorizedWithoutToken(t *testing.T) {
	rec := get(t, newTestHandler(), "/", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未认证应 401, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("401 应返回 JSON, Content-Type=%q", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"error"`) {
		t.Errorf("401 响应应含 error 字段: %q", body)
	}
}

// TestWebHandler_WrongToken 令牌错误返回 401。
func TestWebHandler_WrongToken(t *testing.T) {
	rec := get(t, newTestHandler(), "/?token=wrong-token", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错误令牌应 401, got %d", rec.Code)
	}
}

// TestWebHandler_BearerHeaderServesIndex Authorization: Bearer 认证后根路径返回 index.html。
func TestWebHandler_BearerHeaderServesIndex(t *testing.T) {
	rec := get(t, newTestHandler(), "/", map[string]string{"Authorization": "Bearer " + testToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("Bearer 认证应 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>spa-index</html>" {
		t.Errorf("根路径应返回 index.html 内容, got %q", body)
	}
}

// TestWebHandler_XAuthTokenHeader X-Auth-Token 头认证可用。
func TestWebHandler_XAuthTokenHeader(t *testing.T) {
	rec := get(t, newTestHandler(), "/assets/app.js", map[string]string{"X-Auth-Token": testToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("X-Auth-Token 认证应 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "console.log('app')" {
		t.Errorf("静态文件内容不符: %q", body)
	}
}

// TestWebHandler_QueryToken ?token= 查询参数认证可用（首次手工验证便利）。
func TestWebHandler_QueryToken(t *testing.T) {
	rec := get(t, newTestHandler(), "/?token="+testToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("查询参数令牌应 200, got %d", rec.Code)
	}
}

// TestWebHandler_SPAFallback 未命中静态文件的路径 fallback 到 index.html（SPA 前端路由）。
func TestWebHandler_SPAFallback(t *testing.T) {
	rec := get(t, newTestHandler(), "/settings/network", map[string]string{"Authorization": "Bearer " + testToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA fallback 应 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>spa-index</html>" {
		t.Errorf("SPA fallback 应返回 index.html 内容, got %q", body)
	}
}

// TestWebHandler_DirectoryPath_NoListing 目录路径回退到 index.html，不暴露目录列表。
func TestWebHandler_DirectoryPath_NoListing(t *testing.T) {
	rec := get(t, newTestHandler(), "/assets/", map[string]string{"Authorization": "Bearer " + testToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("目录路径应 fallback 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "<html>spa-index</html>" {
		t.Errorf("目录路径应返回 index.html 而非目录列表, got %q", body)
	}
}

// TestWebHandler_EmptyTokenDeniesAll 令牌未配置（空串）时除健康检查外全部拒绝（fail closed）。
func TestWebHandler_EmptyTokenDeniesAll(t *testing.T) {
	h := NewWebHandler(WebOptions{Assets: testAssets(), Token: ""})
	if rec := get(t, h, "/", map[string]string{"Authorization": "Bearer whatever"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("空令牌配置应拒绝携带任意令牌的请求, got %d", rec.Code)
	}
	if rec := get(t, h, "/?token="+testToken, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("空令牌配置应拒绝查询参数认证, got %d", rec.Code)
	}
	if rec := get(t, h, healthPath, nil); rec.Code != http.StatusOK {
		t.Errorf("健康检查应保持豁免, got %d", rec.Code)
	}
}

// TestWebHandler_MethodNotAllowed 非 GET/HEAD 静态请求返回 405。
func TestWebHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	newTestHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST 应 405, got %d", rec.Code)
	}
}

// TestWebHandler_IndexMissing_SPAFallback404 资产源缺 index.html 时 fallback 返回 404。
func TestWebHandler_IndexMissing_SPAFallback404(t *testing.T) {
	h := NewWebHandler(WebOptions{
		Assets: fstest.MapFS{"assets/app.js": &fstest.MapFile{Data: []byte("x")}},
		Token:  testToken,
	})
	rec := get(t, h, "/missing-route", map[string]string{"Authorization": "Bearer " + testToken})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("缺 index.html 的 fallback 应 404, got %d", rec.Code)
	}
}

// TestWebHandler_NilAssets 静态资产未加载时返回 404 不 panic。
func TestWebHandler_NilAssets(t *testing.T) {
	h := NewWebHandler(WebOptions{Assets: nil, Token: testToken})
	rec := get(t, h, "/", map[string]string{"Authorization": "Bearer " + testToken})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("资产未加载应 404, got %d", rec.Code)
	}
}

// TestWebHandler_SetTokenRotation 令牌热轮换：轮换后旧令牌立即 401、新令牌 200
// （桌面设置页「重新生成令牌」运行期生效语义，经 RotateToken → SetToken 到达）。
func TestWebHandler_SetTokenRotation(t *testing.T) {
	h := NewWebHandler(WebOptions{Assets: testAssets(), Token: "old-token"})

	// 轮换前旧令牌可用
	if rec := get(t, h, "/", map[string]string{"Authorization": "Bearer old-token"}); rec.Code != http.StatusOK {
		t.Fatalf("轮换前旧令牌应 200, got %d", rec.Code)
	}

	h.SetToken("new-token")

	if rec := get(t, h, "/", map[string]string{"Authorization": "Bearer old-token"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("轮换后旧令牌应 401, got %d", rec.Code)
	}
	if rec := get(t, h, "/?token=old-token", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("轮换后旧令牌查询参数通道应 401, got %d", rec.Code)
	}
	if rec := get(t, h, "/", map[string]string{"Authorization": "Bearer new-token"}); rec.Code != http.StatusOK {
		t.Errorf("轮换后新令牌应 200, got %d", rec.Code)
	}
}

// TestWebHandler_PreviewSecFetchSiteMatrix serve 模式预览路由同源校验矩阵：
// 仅 Sec-Fetch-Site: same-origin（前端页面内嵌框架加载形态）放行；cross-site
// （恶意网站跨站嵌入）、none（地址栏直接导航）、缺失（curl/脚本探测）一律 403。
// 放行判定以「过了同源关到达 PreviewHandler」为证：/preview-pdf 缺 path 参数
// 返回 400（同源关后的业务校验），与 403（同源关拒绝）状态码可区分。
func TestWebHandler_PreviewSecFetchSiteMatrix(t *testing.T) {
	cases := []struct {
		name string
		site string
		want int
	}{
		{"同源放行", "same-origin", http.StatusBadRequest},
		{"跨站拒绝", "cross-site", http.StatusForbidden},
		{"直接导航拒绝", "none", http.StatusForbidden},
		{"缺失头拒绝", "", http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header := map[string]string{}
			if tc.site != "" {
				header["Sec-Fetch-Site"] = tc.site
			}
			rec := get(t, newTestHandler(), "/preview-pdf", header)
			if rec.Code != tc.want {
				t.Fatalf("Sec-Fetch-Site=%q 应返回 %d, got %d", tc.site, tc.want, rec.Code)
			}
		})
	}

	// /preview-raw/ 路由同样受同源关约束
	rec := get(t, newTestHandler(), "/preview-raw/C:/x/style.css", map[string]string{"Sec-Fetch-Site": "cross-site"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("/preview-raw 跨站请求应 403, got %d", rec.Code)
	}
}

// TestExtractToken 令牌提取优先级与边界。
func TestExtractToken(t *testing.T) {
	newReq := func(path string, header map[string]string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		return req
	}

	cases := []struct {
		name string
		req  *http.Request
		want string
	}{
		{"Bearer 头", newReq("/", map[string]string{"Authorization": "Bearer t1"}), "t1"},
		{"Bearer 头优先于 X-Auth-Token", newReq("/", map[string]string{"Authorization": "Bearer t1", "X-Auth-Token": "t2"}), "t1"},
		{"非 Bearer 的 Authorization 忽略", newReq("/", map[string]string{"Authorization": "Basic abc"}), ""},
		{"Bearer 空值回退 X-Auth-Token", newReq("/", map[string]string{"Authorization": "Bearer ", "X-Auth-Token": "t2"}), "t2"},
		{"仅 X-Auth-Token", newReq("/", map[string]string{"X-Auth-Token": "t2"}), "t2"},
		{"查询参数兜底", newReq("/?token=t3", nil), "t3"},
		{"头优先于查询参数", newReq("/?token=t3", map[string]string{"X-Auth-Token": "t2"}), "t2"},
		{"全空", newReq("/", nil), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractToken(tc.req); got != tc.want {
				t.Errorf("extractToken = %q, want %q", got, tc.want)
			}
		})
	}
}
