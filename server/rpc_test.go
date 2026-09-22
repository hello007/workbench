package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"workbench/model"
)

// rpcTestTarget RPC 翻译层的测试调用目标：方法签名覆盖常见参数与返回值形态。
// MethodByName 仅命中导出方法，未导出方法天然不可达。
type rpcTestTarget struct{}

// GetVersion 无参数单返回值（对齐 GetAppVersion 形态）。
func (rpcTestTarget) GetVersion() string { return "test-v1" }

// Add 两整型参数单返回值（对齐算术类方法形态）。
func (rpcTestTarget) Add(a, b int) int { return a + b }

// Resize uint16/uint 参数（对齐终端 resize 类参数）+ (string, error) 双返回值。
func (rpcTestTarget) Resize(w uint16, h uint) (string, error) {
	return fmt.Sprintf("%dx%d", w, h), nil
}

// Echo string/bool/slice 参数 + struct 指针返回值。
func (rpcTestTarget) Echo(text string, flag bool, items []string) *rpcTestEcho {
	return &rpcTestEcho{Text: text, Flag: flag, Items: items}
}

// rpcTestEcho Echo 的返回结构。
type rpcTestEcho struct {
	Text  string   `json:"text"`
	Flag  bool     `json:"flag"`
	Items []string `json:"items"`
}

// FailBiz 返回 AppError（验证业务错误码透传）。
func (rpcTestTarget) FailBiz() (string, error) {
	return "", model.NewAppError("E_TEST_BIZ", "业务拒绝")
}

// FailWrapped 返回包装的 AppError（验证 errors.As 穿透包装链）。
func (rpcTestTarget) FailWrapped() error {
	return model.WrapAppError("E_TEST_WRAPPED", "包装拒绝", errors.New("root cause"))
}

// FailPlain 返回普通 error（验证包装为通用内部错误码）。
func (rpcTestTarget) FailPlain() error { return errors.New("plain boom") }

// OkDataWithError 第二返回值为 nil error（验证 data 正常透出）。
func (rpcTestTarget) OkDataWithError() (string, error) { return "ok-data", nil }

// NoReturn 无返回值（对齐 SaveFile 类形态，data 应为 null）。
func (rpcTestTarget) NoReturn(string) {}

// Variadic 变参方法（翻译层应明确拒绝）。
func (rpcTestTarget) Variadic(args ...string) {}

// postRPC 以 POST JSON 请求体调用 handler 并返回响应记录。
func postRPC(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, rpcPath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// assertRPCError 断言响应为 HTTP 200 + ok:false + 指定错误码（业务失败统一
// HTTP 200，与桌面 ErrorFormatter 的 {code, message} 形态对齐）。
func assertRPCError(t *testing.T, rec *httptest.ResponseRecorder, wantCode, wantMsgPart string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("业务失败应 HTTP 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ok":false`) {
		t.Errorf("响应应含 ok:false: %q", body)
	}
	if !strings.Contains(body, `"`+wantCode+`"`) {
		t.Errorf("响应应含错误码 %s: %q", wantCode, body)
	}
	if wantMsgPart != "" && !strings.Contains(body, wantMsgPart) {
		t.Errorf("响应应含消息片段 %q: %q", wantMsgPart, body)
	}
}

// TestRPCHandler_Success 无参数/多参数调用成功返回 {"ok":true,"data":...}。
func TestRPCHandler_Success(t *testing.T) {
	h := NewRPCHandler(rpcTestTarget{})

	rec := postRPC(t, h, `{"method":"GetVersion","args":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("应 HTTP 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"data":"test-v1"`) {
		t.Errorf("GetVersion 响应不符: %q", body)
	}

	rec = postRPC(t, h, `{"method":"Add","args":[2,3]}`)
	if body := rec.Body.String(); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"data":5`) {
		t.Errorf("Add 响应不符: %q", body)
	}
}

// TestRPCHandler_StructPointerResult struct 指针返回值按 JSON 字段序列化。
func TestRPCHandler_StructPointerResult(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"Echo","args":["你好",true,["a","b"]]}`)
	body := rec.Body.String()
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"text":"你好"`) ||
		!strings.Contains(body, `"flag":true`) || !strings.Contains(body, `"items":["a","b"]`) {
		t.Errorf("Echo 响应不符: %q", body)
	}
}

// TestRPCHandler_UintParams JSON number 到 uint16/uint 的整型转换（终端 resize 类参数）。
func TestRPCHandler_UintParams(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"Resize","args":[800,600]}`)
	if body := rec.Body.String(); !strings.Contains(body, `"data":"800x600"`) {
		t.Errorf("Resize 响应不符: %q", body)
	}
}

// TestRPCHandler_UnknownMethod 未知方法返回明确错误码，HTTP 200。
func TestRPCHandler_UnknownMethod(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"NoSuchMethod","args":[]}`)
	assertRPCError(t, rec, ErrCodeRPCMethodNotFound, "NoSuchMethod")
}

// TestRPCHandler_UnexportedMethodDenied 未导出方法与不存在方法同样拒绝
// （白名单=导出方法，与桌面 webview 暴露面一致）。
func TestRPCHandler_UnexportedMethodDenied(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"getString","args":[]}`)
	assertRPCError(t, rec, ErrCodeRPCMethodNotFound, "")
}

// TestRPCHandler_ArgCountMismatch 参数个数不匹配返回明确错误码。
func TestRPCHandler_ArgCountMismatch(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"Add","args":[1]}`)
	assertRPCError(t, rec, ErrCodeRPCArgCountMismatch, "需要 2 个参数")
}

// TestRPCHandler_ArgTypeMismatch 参数类型转换失败返回可读错误（string 传给 int）。
func TestRPCHandler_ArgTypeMismatch(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"Add","args":["a",2]}`)
	assertRPCError(t, rec, ErrCodeRPCArgTypeMismatch, "第 1 个参数")
}

// TestRPCHandler_FloatToIntRejected 浮点值传给整型参数被 json 边界检查拒绝。
func TestRPCHandler_FloatToIntRejected(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"Resize","args":[1.5,3]}`)
	assertRPCError(t, rec, ErrCodeRPCArgTypeMismatch, "")
}

// TestRPCHandler_AppErrorPassthrough AppError 透传业务错误码（非 internal）。
func TestRPCHandler_AppErrorPassthrough(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"FailBiz","args":[]}`)
	assertRPCError(t, rec, "E_TEST_BIZ", "业务拒绝")
}

// TestRPCHandler_WrappedAppErrorPassthrough 包装的 AppError 经 errors.As 穿透透传。
func TestRPCHandler_WrappedAppErrorPassthrough(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"FailWrapped","args":[]}`)
	assertRPCError(t, rec, "E_TEST_WRAPPED", "包装拒绝")
}

// TestRPCHandler_PlainErrorInternalCode 普通 error 包装为通用内部错误码。
func TestRPCHandler_PlainErrorInternalCode(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"FailPlain","args":[]}`)
	assertRPCError(t, rec, ErrCodeRPCInternal, "plain boom")
}

// TestRPCHandler_NilErrorKeepsData (data, error) 双返回值且 error 为 nil 时 data 正常透出。
func TestRPCHandler_NilErrorKeepsData(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"OkDataWithError","args":[]}`)
	if body := rec.Body.String(); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"data":"ok-data"`) {
		t.Errorf("nil error 时 data 应透出: %q", body)
	}
}

// TestRPCHandler_NoReturnNullData 无返回值方法 data 为 null。
func TestRPCHandler_NoReturnNullData(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"NoReturn","args":["x"]}`)
	if body := rec.Body.String(); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"data":null`) {
		t.Errorf("无返回值方法 data 应为 null: %q", body)
	}
}

// TestRPCHandler_VariadicRejected 变参方法明确拒绝（避免误调用语义不清）。
func TestRPCHandler_VariadicRejected(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"method":"Variadic","args":["a"]}`)
	assertRPCError(t, rec, ErrCodeRPCMethodUnsupported, "")
}

// TestRPCHandler_MethodNotAllowed 非 POST 请求返回 405 与 Allow 头。
func TestRPCHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, rpcPath, nil)
	rec := httptest.NewRecorder()
	NewRPCHandler(rpcTestTarget{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 应 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("Allow 头应为 POST, got %q", allow)
	}
}

// TestRPCHandler_InvalidJSON 请求体非合法 JSON 返回明确错误码。
func TestRPCHandler_InvalidJSON(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `not-json`)
	assertRPCError(t, rec, ErrCodeRPCBadRequest, "")
}

// TestRPCHandler_BodyTooLarge 请求体超过 maxRPCBodyBytes 上限被拒绝，
// 返回区别于普通非法 JSON 的可读错误（防超大 body 反序列化耗尽内存）。
func TestRPCHandler_BodyTooLarge(t *testing.T) {
	big := `{"method":"GetVersion","args":["` + strings.Repeat("a", maxRPCBodyBytes) + `"]}`
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), big)
	assertRPCError(t, rec, ErrCodeRPCBadRequest, "大小上限")
}

// TestRPCHandler_MissingMethod 缺 method 字段返回明确错误码。
func TestRPCHandler_MissingMethod(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(rpcTestTarget{}), `{"args":[]}`)
	assertRPCError(t, rec, ErrCodeRPCBadRequest, "缺少 method")
}

// TestRPCHandler_NilTarget target 未装配时明确报错不 panic。
func TestRPCHandler_NilTarget(t *testing.T) {
	rec := postRPC(t, NewRPCHandler(nil), `{"method":"GetVersion","args":[]}`)
	assertRPCError(t, rec, ErrCodeRPCInternal, "")
}

// TestWebHandler_RPCRequiresToken 经 NewWebHandler 装配后 /api/rpc 未携带令牌返回 401。
func TestWebHandler_RPCRequiresToken(t *testing.T) {
	h := NewWebHandler(WebOptions{Assets: testAssets(), Token: testToken, RPCTarget: rpcTestTarget{}})
	rec := postRPC(t, h, `{"method":"GetVersion","args":[]}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未认证 RPC 应 401, got %d", rec.Code)
	}
}

// TestWebHandler_RPCWithToken 带令牌的 RPC 请求走通并返回业务结果。
func TestWebHandler_RPCWithToken(t *testing.T) {
	h := NewWebHandler(WebOptions{Assets: testAssets(), Token: testToken, RPCTarget: rpcTestTarget{}})
	req := httptest.NewRequest(http.MethodPost, rpcPath, strings.NewReader(`{"method":"Add","args":[20,22]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("认证后 RPC 应 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"data":42`) {
		t.Errorf("认证后 RPC 响应不符: %q", body)
	}
}

// TestWebHandler_RPCNotMounted RPCTarget 为 nil 时 /api/rpc 不挂载：
// 请求落入静态路径分支——未带令牌先被认证中间件 401 拒绝，带令牌后被
// 静态分支 405 拒绝（仅支持 GET/HEAD），均不可达 RPC 能力。
func TestWebHandler_RPCNotMounted(t *testing.T) {
	h := NewWebHandler(WebOptions{Assets: testAssets(), Token: testToken})

	rec := postRPC(t, h, `{"method":"GetVersion","args":[]}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未挂载且未带令牌应 401, got %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPost, rpcPath, strings.NewReader(`{"method":"GetVersion","args":[]}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("未挂载且带令牌应 405, got %d", rec.Code)
	}
}

// TestWebHandler_PreviewRoutesMountedNoAuth 预览路由已挂载且不做 token 认证，
// 但须携带同源标记（Sec-Fetch-Site: same-origin，前端页面内嵌框架的加载形态；
// 同源校验矩阵详见 TestWebHandler_PreviewSecFetchSiteMatrix）：路由命中以「同源
// 请求未带令牌仍进入 preview 业务校验（400 缺参数）」而非 401/403/SPA fallback 为准。
func TestWebHandler_PreviewRoutesMountedNoAuth(t *testing.T) {
	h := NewWebHandler(WebOptions{Assets: testAssets(), Token: testToken, RPCTarget: rpcTestTarget{}})
	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin"}

	rec := get(t, h, "/preview-pdf", sameOrigin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("/preview-pdf 应命中 preview 路由返回 400（缺 path 参数）, got %d: %q", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "缺少 path 参数") {
		t.Errorf("/preview-pdf 响应不符: %q", body)
	}

	rec = get(t, h, "/preview-raw/", sameOrigin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("/preview-raw/ 应命中 preview 路由返回 400（缺资源路径）, got %d", rec.Code)
	}
}

// TestWebHandler_PreviewPDFSmoke preview 挂载 smoke：以临时 PDF 走通
// /preview-pdf 完整链路（同源关 + path 参数校验 + ServeFile 返回内容）。
func TestWebHandler_PreviewPDFSmoke(t *testing.T) {
	pdfPath := newPreviewTestFile(t, "doc.pdf", "%PDF-smoke")
	h := NewWebHandler(WebOptions{Assets: testAssets(), Token: testToken})

	rec := get(t, h, "/preview-pdf?path="+pdfPath, map[string]string{"Sec-Fetch-Site": "same-origin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("合法 PDF 预览应 200, got %d: %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/pdf") {
		t.Errorf("Content-Type 应为 application/pdf, got %q", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "%PDF-smoke") {
		t.Errorf("PDF 内容不符: %q", body)
	}
}
