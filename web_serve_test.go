package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"workbench/model"
	"workbench/service"
)

// withTempTokenFile 将访问令牌文件重指向临时目录（隔离 data/web_token 工作区
// 真实文件），测试结束恢复。
func withTempTokenFile(t *testing.T) {
	t.Helper()
	old := serveTokenFile
	serveTokenFile = filepath.Join(t.TempDir(), "web_token")
	t.Cleanup(func() { serveTokenFile = old })
}

// newWebServeTestApp 构造浏览器访问域测试用 App：完整 AppServices 装配（applySink
// 需 terminal/update/aiFunc/git 四服务非 nil）+ 令牌文件隔离 + 管理器就绪。
// 不用 t.TempDir：NewAppServices 内 util.InitLogger 的 lumberjack 句柄持有
// app.log，Windows 上 RemoveAll 清理失败（对齐 serve_rpc_smoke_test.go）。
func newWebServeTestApp(t *testing.T) *App {
	t.Helper()
	withTempTokenFile(t)
	dataDir, err := os.MkdirTemp("", "workbench-websrv")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	app := NewApp()
	app.ctx = context.Background()
	app.AppServices = NewAppServices(context.Background(), dataDir, false)
	app.webServe = newWebServeManager(app)
	// Cleanup 判 nil：部分用例（如无管理器降级路径）会将 webServe 置 nil
	t.Cleanup(func() {
		if app.webServe != nil {
			_ = app.webServe.Stop()
		}
	})
	return app
}

// freePort 探测一个空闲 TCP 端口（存在极小 TOCTOU 窗口，测试可接受）。
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("探测空闲端口失败: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// httpGetWithToken 向 addr 发起带 Bearer 令牌的 GET 请求，返回状态码。
func httpGetWithToken(t *testing.T, url, token string) int {
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
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// TestWebServeManager_StartServeAndStop 启动后健康检查可达；停机后端口不再
// 接受连接（桌面 shutdown 优雅停机语义）。
func TestWebServeManager_StartServeAndStop(t *testing.T) {
	app := newWebServeTestApp(t)
	m := app.webServe

	if m.Running() {
		t.Fatal("初始状态不应为运行中")
	}
	if err := m.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !m.Running() {
		t.Fatal("Start 后应处于运行中")
	}

	url := "http://" + m.listenAddr() + "/healthz"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("健康检查请求失败: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("健康检查应 200, got %d", resp.StatusCode)
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.Running() {
		t.Error("Stop 后不应为运行中")
	}
	if _, err := http.Get(url); err == nil {
		t.Error("Stop 后原端口不应再接受连接")
	}
}

// TestWebServeManager_StartSameAddressIdempotent 同地址重复 Start 为 no-op
// （SetWebServeConfig 地址未变化时不重启、不断在途连接）。
func TestWebServeManager_StartSameAddressIdempotent(t *testing.T) {
	app := newWebServeTestApp(t)
	m := app.webServe

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := m.Start(addr); err != nil {
		t.Fatalf("首次 Start: %v", err)
	}
	firstAddr := m.listenAddr()
	if err := m.Start(addr); err != nil {
		t.Fatalf("同地址二次 Start: %v", err)
	}
	if got := m.listenAddr(); got != firstAddr {
		t.Errorf("同地址 Start 不应重启（监听地址应不变）: %s vs %s", firstAddr, got)
	}
}

// TestWebServeManager_PortConflict 端口被占用时 Start 返回错误（桌面 startup
// 侧据此告警降级，不影响桌面本体）。
func TestWebServeManager_PortConflict(t *testing.T) {
	app := newWebServeTestApp(t)
	m := app.webServe

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占用端口失败: %v", err)
	}
	defer blocker.Close()
	port := blocker.Addr().(*net.TCPAddr).Port

	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
		t.Fatal("端口被占用时 Start 应返回错误")
	}
	if m.Running() {
		t.Error("启动失败后不应处于运行中")
	}
}

// TestWebServeManager_AddressSwitch 地址切换平滑重启：旧地址停机、新地址可达
// （SetWebServeConfig 改绑定地址的运行时生效语义）。
func TestWebServeManager_AddressSwitch(t *testing.T) {
	app := newWebServeTestApp(t)
	m := app.webServe

	portA, portB := freePort(t), freePort(t)
	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", portA)); err != nil {
		t.Fatalf("Start A: %v", err)
	}
	urlA := "http://" + m.listenAddr() + "/healthz"

	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", portB)); err != nil {
		t.Fatalf("Start B: %v", err)
	}
	urlB := "http://" + m.listenAddr() + "/healthz"
	if urlA == urlB {
		t.Fatal("地址切换后监听地址应变化")
	}
	if resp, err := http.Get(urlB); err != nil || resp.StatusCode != http.StatusOK {
		t.Errorf("新地址应可达, status=%v err=%v", resp, err)
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if _, err := http.Get(urlA); err == nil {
		t.Error("旧地址切换后不应再接受连接")
	}
}

// TestApp_SetWebServeConfig_RuntimeAndPersist SetWebServeConfig 三重语义：
// 持久化到 settings.json、运行时启停生效、地址格式校验。
func TestApp_SetWebServeConfig_RuntimeAndPersist(t *testing.T) {
	app := newWebServeTestApp(t)

	// 开启：运行时启动 + 持久化
	if err := app.SetWebServeConfig(true, "127.0.0.1:0"); err != nil {
		t.Fatalf("SetWebServeConfig(true): %v", err)
	}
	if !app.webServe.Running() {
		t.Fatal("开启后服务应运行中")
	}
	saved, err := app.settingsSvc.Load()
	if err != nil {
		t.Fatalf("读回设置: %v", err)
	}
	if saved.WebServe == nil || !saved.WebServe.Enabled {
		t.Errorf("持久化配置应记录开启: %+v", saved.WebServe)
	}

	// 关闭：运行时停机 + 持久化保留关闭态
	if err := app.SetWebServeConfig(false, "127.0.0.1:0"); err != nil {
		t.Fatalf("SetWebServeConfig(false): %v", err)
	}
	if app.webServe.Running() {
		t.Fatal("关闭后服务应停机")
	}
	saved, _ = app.settingsSvc.Load()
	if saved.WebServe == nil || saved.WebServe.Enabled {
		t.Errorf("持久化配置应记录关闭: %+v", saved.WebServe)
	}

	// 空地址回退默认回环地址（持久化层验证）
	if err := app.SetWebServeConfig(false, "  "); err != nil {
		t.Fatalf("SetWebServeConfig 空地址: %v", err)
	}
	saved, _ = app.settingsSvc.Load()
	if saved.WebServe == nil || saved.WebServe.BindAddress != model.DefaultWebServeBindAddress {
		t.Errorf("空地址应回退默认回环地址, got %+v", saved.WebServe)
	}

	// 非法地址格式：报错且不落盘
	if err := app.SetWebServeConfig(true, "no-port-address"); err == nil {
		t.Fatal("非法地址应报错")
	}
	saved, _ = app.settingsSvc.Load()
	if saved.WebServe == nil || saved.WebServe.BindAddress != model.DefaultWebServeBindAddress {
		t.Errorf("非法地址不应改写持久化配置, got %+v", saved.WebServe)
	}
}

// TestApp_SetWebServeConfig_NoManager webServe 为 nil（测试零值构造等场景）
// 时仅落盘不 panic（下次启动按配置生效）。
func TestApp_SetWebServeConfig_NoManager(t *testing.T) {
	app := newWebServeTestApp(t)
	app.webServe = nil

	if err := app.SetWebServeConfig(true, "127.0.0.1:36999"); err != nil {
		t.Fatalf("SetWebServeConfig（无管理器）: %v", err)
	}
	saved, _ := app.settingsSvc.Load()
	if saved.WebServe == nil || !saved.WebServe.Enabled {
		t.Errorf("无管理器时也应持久化配置: %+v", saved.WebServe)
	}
}

// TestApp_RegenerateWebToken_HotRotate 令牌轮换热生效：轮换后旧令牌 401、
// 新令牌 200，新令牌持久化落盘（真实 HTTP 链路验证 RotateToken → hub/handler）。
func TestApp_RegenerateWebToken_HotRotate(t *testing.T) {
	app := newWebServeTestApp(t)
	if err := app.webServe.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	oldToken, err := app.GetWebServeToken()
	if err != nil {
		t.Fatalf("GetWebServeToken: %v", err)
	}
	rpcURL := "http://" + app.webServe.listenAddr() + "/api/rpc"
	postRPC := func(token string) (int, string) {
		req, err := http.NewRequest(http.MethodPost, rpcURL,
			strings.NewReader(`{"method":"GetAppVersion","args":[]}`))
		if err != nil {
			t.Fatalf("构造 RPC 请求失败: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("RPC 请求失败: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	if code, _ := postRPC(oldToken); code != http.StatusOK {
		t.Fatalf("轮换前旧令牌应 200, got %d", code)
	}

	newToken, err := app.RegenerateWebToken()
	if err != nil {
		t.Fatalf("RegenerateWebToken: %v", err)
	}
	if len(newToken) != 64 || newToken == oldToken {
		t.Fatalf("新令牌应为 64 位 hex 且不同于旧令牌: %q", newToken)
	}

	// 持久化落盘
	persisted, err := os.ReadFile(serveTokenFile)
	if err != nil {
		t.Fatalf("读令牌文件: %v", err)
	}
	if got := strings.TrimSpace(string(persisted)); got != newToken {
		t.Errorf("新令牌应已落盘, got %q", got)
	}

	if code, _ := postRPC(oldToken); code != http.StatusUnauthorized {
		t.Errorf("轮换后旧令牌应 401, got %d", code)
	}
	if code, body := postRPC(newToken); code != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Errorf("轮换后新令牌应 200 且调用成功, got %d %q", code, body)
	}
}

// TestApp_GetWebServeConfig GetWebServeConfig 组装：配置 + 运行状态 + 候选
// 访问地址（含回环 URL、端口与绑定地址一致）。
func TestApp_GetWebServeConfig(t *testing.T) {
	app := newWebServeTestApp(t)

	// 未启动：Running=false，配置为 Load 补默认后的值
	cfg := app.GetWebServeConfig()
	if cfg == nil {
		t.Fatal("GetWebServeConfig 不应返回 nil")
	}
	if cfg.Running {
		t.Error("未启动时 Running 应为 false")
	}
	if !cfg.Enabled || cfg.BindAddress != model.DefaultWebServeBindAddress {
		t.Errorf("默认配置应为开启 + 回环地址: %+v", cfg)
	}
	if len(cfg.AccessUrls) == 0 || cfg.AccessUrls[0] != "http://127.0.0.1:36115" {
		t.Errorf("候选访问地址应含回环 URL: %v", cfg.AccessUrls)
	}

	// 启动后 Running=true
	if err := app.webServe.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if cfg := app.GetWebServeConfig(); !cfg.Running {
		t.Error("启动后 Running 应为 true")
	}
}

// TestApp_GetWebServeToken_CreatesOnDemand 令牌不存在时按需生成并持久化，
// 再次读取复用同一值。
func TestApp_GetWebServeToken_CreatesOnDemand(t *testing.T) {
	app := newWebServeTestApp(t)

	token1, err := app.GetWebServeToken()
	if err != nil {
		t.Fatalf("GetWebServeToken: %v", err)
	}
	if len(token1) != 64 {
		t.Errorf("生成令牌应为 64 位 hex, got %q", token1)
	}
	token2, err := app.GetWebServeToken()
	if err != nil {
		t.Fatalf("GetWebServeToken 第二次: %v", err)
	}
	if token1 != token2 {
		t.Error("已有令牌文件应复用同一令牌")
	}
}

// TestWebAccessUrls 候选访问地址生成：回环恒在首位、端口取自绑定地址、
// 非法地址返回 nil。
func TestWebAccessUrls(t *testing.T) {
	urls := webAccessUrls("127.0.0.1:36115")
	if len(urls) < 1 || urls[0] != "http://127.0.0.1:36115" {
		t.Fatalf("回环 URL 应在首位: %v", urls)
	}
	for _, u := range urls[1:] {
		if !strings.HasSuffix(u, ":36115") {
			t.Errorf("局域网 URL 端口应与绑定地址一致: %q", u)
		}
		if strings.Contains(u, "127.0.0.1") {
			t.Errorf("回环地址不应重复出现: %q", u)
		}
	}

	if got := webAccessUrls("no-port"); got != nil {
		t.Errorf("非法绑定地址应返回 nil, got %v", got)
	}
}

// TestApp_StartWebServe_StartupMatrix 桌面 startup 集成路径矩阵：
//   - 设置开启（默认）→ 服务启动；
//   - 设置关闭 → 不启动且不报错；
//   - 端口被占用 → 告警降级不 panic（桌面本体不受影响，Running=false）。
func TestApp_StartWebServe_StartupMatrix(t *testing.T) {
	t.Run("设置开启默认启动", func(t *testing.T) {
		app := newWebServeTestApp(t)
		app.startWebServe()
		if !app.webServe.Running() {
			t.Fatal("设置默认开启时应启动浏览器访问服务")
		}
	})

	t.Run("设置关闭不启动", func(t *testing.T) {
		app := newWebServeTestApp(t)
		if err := app.settingsSvc.Save(&model.AppSettings{
			WebServe: &model.WebServeSettings{Enabled: false},
		}); err != nil {
			t.Fatalf("写入关闭配置: %v", err)
		}
		app.startWebServe()
		if app.webServe.Running() {
			t.Fatal("设置关闭时不应启动服务")
		}
	})

	t.Run("端口占用降级不阻塞", func(t *testing.T) {
		app := newWebServeTestApp(t)
		blocker, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("占用端口失败: %v", err)
		}
		defer blocker.Close()
		port := blocker.Addr().(*net.TCPAddr).Port
		app.webServeListenOverride = fmt.Sprintf("127.0.0.1:%d", port)

		// 不 panic、Running=false 即证明降级路径成立（桌面功能不受影响）
		app.startWebServe()
		if app.webServe.Running() {
			t.Fatal("端口占用时不应处于运行中")
		}
	})
}

// TestIsLoopbackBind 回环绑定判断矩阵：127.0.0.1 / localhost / ::1 判回环，
// 0.0.0.0 / 局域网 IP / 非法地址判非回环。
func TestIsLoopbackBind(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:36115", true},
		{"localhost:8080", true},
		{"[::1]:36115", true},
		{"0.0.0.0:36115", false},
		{"192.168.1.5:36115", false},
		{"no-port", false},
	}
	for _, tc := range cases {
		if got := isLoopbackBind(tc.addr); got != tc.want {
			t.Errorf("isLoopbackBind(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

// TestMulticastSink 复合出口依次投递：全部下游收到同一条事件。
func TestMulticastSink(t *testing.T) {
	type record struct {
		mu   sync.Mutex
		name string
		data []any
	}
	newSink := func() (*record, service.EventSink) {
		r := &record{}
		return r, service.EventSink(sinkFunc(func(name string, data ...any) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.name, r.data = name, data
		}))
	}
	r1, s1 := newSink()
	r2, s2 := newSink()

	m := &multicastSink{sinks: []service.EventSink{s1, s2}}
	m.Emit("terminal-output", "term-1", "out")

	for i, r := range []*record{r1, r2} {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.name != "terminal-output" || len(r.data) != 2 || r.data[0] != "term-1" {
			t.Errorf("下游 %d 未收到完整事件: %q %v", i, r.name, r.data)
		}
	}
}

// sinkFunc 函数适配 EventSink 接口（测试用）。
type sinkFunc func(name string, data ...any)

func (f sinkFunc) Emit(name string, data ...any) { f(name, data...) }
