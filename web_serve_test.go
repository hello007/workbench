package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"workbench/model"
	"workbench/server"
	"workbench/service"
)

// newWebServeTestApp 构造浏览器访问域测试用 App：完整 AppServices 装配（applySink
// 需 terminal/update/aiFunc/git/chat/pptxSplit 六服务非 nil）+ 令牌文件隔离 + 管理器就绪。
// 不用 t.TempDir：NewAppServices 内 util.InitLogger 的 lumberjack 句柄持有
// app.log，Windows 上 RemoveAll 清理失败（对齐 serve_rpc_smoke_test.go）。
//
// serveTokenFile 重指向 dataDir 内（先于 NewAppServices）：多 token 服务的
// legacyPath（存量迁移源 + 明文兼容文件）以 dataDir/web_token 构造，令牌文件
// 必须与其一致，否则引导产物与「查看令牌」读取路径脱节。
func newWebServeTestApp(t *testing.T) *App {
	t.Helper()
	dataDir, err := os.MkdirTemp("", "workbench-websrv")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	old := serveTokenFile
	serveTokenFile = filepath.Join(dataDir, "web_token")
	t.Cleanup(func() { serveTokenFile = old })
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

// freePort 探测一个空闲 TCP 端口，专供「必须预知具体端口」的用例（地址切换、
// 改址重启等被测语义依赖具体地址字符串变化触发真实重启路径，127.0.0.1:0 同串
// 会命中 Start 幂等 no-op 而触发不了）。Close 到 Start 绑定之间存在极小 TOCTOU
// 窗口：端口可能被同机其他进程抢占，危害限于该用例误报一次（Start 返回错误），
// 重跑即过；已排查无静态端口冲突源（内核 :0 分配本身跳过已监听端口），定性
// 接受。能直接绑 :0 的用例一律直传 "127.0.0.1:0"，禁新增非必要 freePort 调用。
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
	if err := m.Start("127.0.0.1:0", nil); err != nil {
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

	// 幂等判断走配置地址字符串比较，:0 同串即可覆盖「同地址不重启」语义，
	// 且内核分配端口无探测 TOCTOU 窗口
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("首次 Start: %v", err)
	}
	firstAddr := m.listenAddr()
	if err := m.Start("127.0.0.1:0", nil); err != nil {
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

	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", port), nil); err == nil {
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
	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", portA), nil); err != nil {
		t.Fatalf("Start A: %v", err)
	}
	urlA := "http://" + m.listenAddr() + "/healthz"

	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", portB), nil); err != nil {
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
	if err := app.webServe.Start("127.0.0.1:0", nil); err != nil {
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
	if err := app.webServe.Start("127.0.0.1:0", nil); err != nil {
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
	urls := webAccessUrls("127.0.0.1:36115", false)
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

	if got := webAccessUrls("no-port", false); got != nil {
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
		// 经 settings 注入随机端口：默认绑定 127.0.0.1:36115，本机 workbench.exe
		// 常驻监听同端口会使 Start 走告警降级路径，误报为「设置开启未启动」；
		// 不用 webServeListenOverride，保住 settings BindAddress 选址分支的覆盖
		if err := app.settingsSvc.Save(&model.AppSettings{
			WebServe: &model.WebServeSettings{Enabled: true, BindAddress: "127.0.0.1:0"},
		}); err != nil {
			t.Fatalf("写入开启配置: %v", err)
		}
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

// assertWaitBlocked 断言 Wait goroutine 在窗口期内未返回（--serve 主进程
// 存活语义：重启/停机不得使 Wait 返回）。
func assertWaitBlocked(t *testing.T, errCh <-chan error, window time.Duration) {
	t.Helper()
	select {
	case err := <-errCh:
		t.Fatalf("Wait 不应在此场景返回（进程须存活）, got err=%v", err)
	case <-time.After(window):
	}
}

// TestWebServeManager_WaitGenerationSurvivesRestart 代际 Wait（B1 回归）：
// --serve 主流程阻塞于 Wait，浏览器经 SetWebServeConfig 触发的重启/停机/
// 重新开启均不得使 Wait 返回（旧实现读到旧代际 done 的 ErrServerClosed 即
// 返回 → runServe 返回 → 进程意外退出）；仅代际未变的真实 Serve 故障退出
// 才返回错误交调用方终止进程。
func TestWebServeManager_WaitGenerationSurvivesRestart(t *testing.T) {
	app := newWebServeTestApp(t)
	m := app.webServe

	// 用具体端口构造地址变化：127.0.0.1:0 字符串相同会命中 Start 幂等 no-op，
	// 触发不了真实重启路径
	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", freePort(t)), nil); err != nil {
		t.Fatalf("Start A: %v", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- m.Wait() }()
	assertWaitBlocked(t, errCh, 200*time.Millisecond)

	// 改绑定地址重启：旧代际 Serve 退出被代际核对消化，Wait 继续等新代际
	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", freePort(t)), nil); err != nil {
		t.Fatalf("Start B（重启）: %v", err)
	}
	assertWaitBlocked(t, errCh, 200*time.Millisecond)

	// 关开关停机：done 置 nil，Wait 阻塞等下一次 Start（进程存活）
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	assertWaitBlocked(t, errCh, 200*time.Millisecond)

	// 重新开启：Wait 被唤醒继续监视新代际
	if err := m.Start(fmt.Sprintf("127.0.0.1:%d", freePort(t)), nil); err != nil {
		t.Fatalf("Start C（重新开启）: %v", err)
	}
	assertWaitBlocked(t, errCh, 200*time.Millisecond)

	// 真实故障退出：绕过管理器直接关 listener，Serve 因 Accept 错误退出且
	// 代际未变 → Wait 返回该错误（--serve 以非零状态终止，runServe 上抛）
	m.mu.Lock()
	ln := m.ln
	m.mu.Unlock()
	if ln == nil {
		t.Fatal("运行中 listener 不应为 nil")
	}
	_ = ln.Close()
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("真实 Serve 故障退出应返回非 nil 错误")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener 关闭后 Wait 应退出")
	}
}

// TestWebServeManager_StopClosesWSClients 停机关闭在途 WS 客户端（B2 集成
// 回归）：http.Server.Close 不跟踪 hijacked 连接，stopLocked 须经 hub.Close
// 显式断开——Stop 后客户端读侧感知连接关闭（无僵尸客户端、pump goroutine 经
// readPump unregister 收尾）。
// TestWebServeManager_StopClosesWSClients Stop 后在途 WS 客户端应感知断开。
// 同时覆盖桌面连接明细收口链路（webServeConnectionsEvent）：stopLocked 在
// hub.Close 后经 Wails 出口显式补发空明细快照——测试 app.ctx 为 Background
//（非 Wails 上下文），wailsEventSink 静默跳过，本用例验证该 emit 分支在
// --serve 无头形态下执行且不 panic；桌面 emit 的事件名/载荷契约由
// webServeConnectionsEvent 常量单点与 hub 回调单测（server 包
// TestWSHub_OnConnectionsChangeCallback）覆盖，manager 级不做载荷断言（伪造 Wails
// 运行时上下文的基建过重，非必要）。
func TestWebServeManager_StopClosesWSClients(t *testing.T) {
	app := newWebServeTestApp(t)
	m := app.webServe
	if err := m.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	token, err := server.LoadOrCreateToken(serveTokenFile)
	if err != nil {
		t.Fatalf("读访问令牌: %v", err)
	}
	conn, _, err := (&websocket.Dialer{
		Subprotocols:     []string{token},
		HandshakeTimeout: 5 * time.Second,
	}).Dial("ws://"+m.listenAddr()+"/ws", nil)
	if err != nil {
		t.Fatalf("WS 升级失败: %v", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	for m.hub.ClientCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("等待 WS 客户端注册超时")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("Stop 后在途 WS 客户端应读到连接关闭错误（无僵尸客户端）")
	}
}

// TestApp_SetWebServeConfig_ServeWaitSurvives --serve 端到端（B1 回归）：
// 浏览器经 /api/rpc 同款入口 SetWebServeConfig 改址重启与关开关后，Wait 不
// 返回（进程存活），且新地址服务可用（旧地址拒绝连接）。
func TestApp_SetWebServeConfig_ServeWaitSurvives(t *testing.T) {
	app := newWebServeTestApp(t)

	// 具体端口构造地址变化（127.0.0.1:0 同串会命中 Start 幂等 no-op）
	if err := app.SetWebServeConfig(true, fmt.Sprintf("127.0.0.1:%d", freePort(t))); err != nil {
		t.Fatalf("开启: %v", err)
	}
	urlA := "http://" + app.webServe.listenAddr() + "/healthz"

	errCh := make(chan error, 1)
	go func() { errCh <- app.webServe.Wait() }()
	assertWaitBlocked(t, errCh, 200*time.Millisecond)

	// 改绑定地址：平滑重启，Wait 不返回，新地址可达、旧地址关闭
	if err := app.SetWebServeConfig(true, fmt.Sprintf("127.0.0.1:%d", freePort(t))); err != nil {
		t.Fatalf("改址重启: %v", err)
	}
	assertWaitBlocked(t, errCh, 200*time.Millisecond)
	urlB := "http://" + app.webServe.listenAddr() + "/healthz"
	if urlA == urlB {
		t.Fatal("改址后监听地址应变化")
	}
	if code := httpGetWithToken(t, urlB, ""); code != http.StatusOK {
		t.Errorf("重启后新地址健康检查应 200（免认证）, got %d", code)
	}
	if _, err := http.Get(urlA); err == nil {
		t.Error("重启后旧地址不应再接受连接")
	}

	// 关开关：停机后 Wait 仍不返回（进程存活等待重新开启）
	if err := app.SetWebServeConfig(false, "127.0.0.1:0"); err != nil {
		t.Fatalf("关闭: %v", err)
	}
	assertWaitBlocked(t, errCh, 200*time.Millisecond)
}

// TestApp_RegenerateWebToken_ConcurrentConsistency 并发轮换一致性（B3 回归）：
// 多 goroutine 并发 RegenerateWebToken（生成→落盘→热轮换全程互斥）后，磁盘
// 令牌与 handler 校验基准、hub 升级基准三处一致：最终令牌对 RPC 200、WS 子
// 协议升级成功，其余并发产生的令牌对 RPC 一律 401。
func TestApp_RegenerateWebToken_ConcurrentConsistency(t *testing.T) {
	app := newWebServeTestApp(t)
	if err := app.webServe.Start("127.0.0.1:0", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	const n = 8
	tokens := make(chan string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := app.RegenerateWebToken()
			if err != nil {
				t.Errorf("RegenerateWebToken: %v", err)
				return
			}
			tokens <- tok
		}()
	}
	wg.Wait()
	close(tokens)

	seen := make(map[string]struct{}, n)
	for tok := range tokens {
		if len(tok) != 64 {
			t.Errorf("令牌应为 64 位 hex, got %q", tok)
		}
		seen[tok] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("并发轮换应产生 %d 个互异令牌, got %d", n, len(seen))
	}

	// 磁盘最终值（互斥串行下为最后一个持锁完成的调用所写）
	persisted, err := os.ReadFile(serveTokenFile)
	if err != nil {
		t.Fatalf("读令牌文件: %v", err)
	}
	disk := strings.TrimSpace(string(persisted))
	if _, ok := seen[disk]; !ok {
		t.Fatalf("磁盘令牌 %q 应是本轮并发产生的令牌之一", disk)
	}

	// handler 校验基准 == 磁盘：最终令牌 200，其余令牌 401
	rpcURL := "http://" + app.webServe.listenAddr() + "/api/rpc"
	postRPC := func(token string) int {
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
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	if code := postRPC(disk); code != http.StatusOK {
		t.Errorf("磁盘令牌（最终生效）应通过 RPC 认证 200, got %d", code)
	}
	for tok := range seen {
		if tok == disk {
			continue
		}
		if code := postRPC(tok); code != http.StatusUnauthorized {
			t.Errorf("被后续轮换覆盖的令牌应 401, got %d", code)
		}
	}

	// hub 升级基准 == 磁盘：最终令牌经 WS 子协议通道升级成功
	wsConn, wsResp, err := (&websocket.Dialer{
		Subprotocols:     []string{disk},
		HandshakeTimeout: 5 * time.Second,
	}).Dial("ws://"+app.webServe.listenAddr()+"/ws", nil)
	if err != nil {
		if wsResp != nil {
			_ = wsResp.Body.Close()
		}
		t.Fatalf("磁盘令牌应通过 WS 子协议升级, err=%v status=%v", err, wsResp)
	}
	_ = wsConn.Close()
}

// ===== TLS（浏览器通道内置 TLS，三期 PR2）=====

// redirectTLSDataDir 将自签证书落盘目录重定向到临时目录（对齐 serveTokenFile
// 先例的包级变量重定向），返回目录路径。
func redirectTLSDataDir(t *testing.T) string {
	t.Helper()
	old := webServeDataDir
	webServeDataDir = t.TempDir()
	t.Cleanup(func() { webServeDataDir = old })
	return webServeDataDir
}

// tlsHTTPClient 构造信任自签证书的 HTTP 客户端（仅测试内使用；
// InsecureSkipVerify 不下放生产配置）。
func tlsHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // 自签证书测试信任，仅测试内
		},
	}
}

// httpGetStatus 向 url 发 GET 返回状态码；传输层错误返回 -1。
func httpGetStatus(t *testing.T, client *http.Client, url string) int {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// TestWebServeManager_TLSHttpsServe TLS 启用后 https 可达、明文 HTTP 请求不再
// 得到正常 200（健康检查在 TLS 链路上同语义）。
func TestWebServeManager_TLSHttpsServe(t *testing.T) {
	app := newWebServeTestApp(t)
	tlsDir := redirectTLSDataDir(t)
	m := app.webServe

	if err := m.Start("127.0.0.1:0", &WebServeTLSConfig{Enabled: true}); err != nil {
		t.Fatalf("Start（自签引导）: %v", err)
	}
	// 自签证书应已生成并持久化（下次启动复用同一信任）
	if _, err := os.Stat(filepath.Join(tlsDir, server.DefaultTLSCertFile)); err != nil {
		t.Fatalf("自签证书应已落盘: %v", err)
	}

	url := "https://" + m.listenAddr() + "/healthz"
	if code := httpGetStatus(t, tlsHTTPClient(), url); code != http.StatusOK {
		t.Errorf("TLS 启用后 https 健康检查应 200, got %d", code)
	}
	// 明文 HTTP 客户端访问 TLS 端口：Go 内置兜底响应 400（非 200 即可证明走了 TLS）
	if code := httpGetStatus(t, http.DefaultClient, "http://"+m.listenAddr()+"/healthz"); code == http.StatusOK {
		t.Error("明文 HTTP 客户端访问 TLS 端口不应得到 200")
	}
}

// TestWebServeManager_TLSSameAddressToggleRestarts 同地址开/关 TLS 必须触发真实
// 重启（幂等键 = listen + TLS 配置联合比较；漏扩则 TLS 开关「保存了但不生效」）：
//   - 同地址同 TLS 配置重复 Start → no-op（hub 不重建）；
//   - 同地址开 TLS → 重启（hub 重建）且 https 生效；
//   - 同地址关 TLS → 重启且恢复纯 HTTP。
func TestWebServeManager_TLSSameAddressToggleRestarts(t *testing.T) {
	app := newWebServeTestApp(t)
	redirectTLSDataDir(t)
	m := app.webServe

	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t)) // 同串地址 + :0 同串幂等语义依赖具体端口
	if err := m.Start(addr, nil); err != nil {
		t.Fatalf("Start HTTP: %v", err)
	}
	m.mu.Lock()
	hub1 := m.hub
	m.mu.Unlock()

	// 同地址同配置（nil 与零值等价）：no-op
	if err := m.Start(addr, nil); err != nil {
		t.Fatalf("Start HTTP 重复: %v", err)
	}
	m.mu.Lock()
	hub2 := m.hub
	m.mu.Unlock()
	if hub1 != hub2 {
		t.Error("同地址同 TLS 配置重复 Start 应为 no-op（hub 不重建）")
	}

	// 同地址开 TLS：真实重启，https 生效
	if err := m.Start(addr, &WebServeTLSConfig{Enabled: true}); err != nil {
		t.Fatalf("Start TLS（同地址）: %v", err)
	}
	m.mu.Lock()
	hub3 := m.hub
	m.mu.Unlock()
	if hub3 == hub2 {
		t.Fatal("同地址开启 TLS 应触发真实重启（幂等键须含 TLS 配置）")
	}
	if code := httpGetStatus(t, tlsHTTPClient(), "https://"+m.listenAddr()+"/healthz"); code != http.StatusOK {
		t.Errorf("开启 TLS 后 https 应可达, got %d", code)
	}

	// 同地址重复同 TLS 配置：no-op
	if err := m.Start(addr, &WebServeTLSConfig{Enabled: true}); err != nil {
		t.Fatalf("Start TLS 重复: %v", err)
	}
	m.mu.Lock()
	hub4 := m.hub
	m.mu.Unlock()
	if hub4 != hub3 {
		t.Error("同地址同 TLS 配置重复 Start 应为 no-op")
	}

	// 同地址关 TLS：恢复纯 HTTP
	if err := m.Start(addr, nil); err != nil {
		t.Fatalf("Start HTTP（关 TLS）: %v", err)
	}
	if code := httpGetStatus(t, http.DefaultClient, "http://"+m.listenAddr()+"/healthz"); code != http.StatusOK {
		t.Errorf("关闭 TLS 后纯 http 应恢复可达, got %d", code)
	}
}

// TestWebServeManager_TLSCertInvalidOldInstanceSurvives 半死态防护（核心回归）：
// 运行中旧实例上以无效证书配置 Start（同地址改 TLS 配置 → 幂等不命中）须同步
// 返回 E_TLS_CERT_INVALID，且证书加载失败发生在停旧实例之前——旧实例原样存活、
// 服务不受影响。仅填证书/私钥其一同样拒绝。
func TestWebServeManager_TLSCertInvalidOldInstanceSurvives(t *testing.T) {
	app := newWebServeTestApp(t)
	m := app.webServe

	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	if err := m.Start(addr, nil); err != nil {
		t.Fatalf("Start HTTP: %v", err)
	}
	url := "http://" + m.listenAddr() + "/healthz"

	// 双路径均不存在：加载失败，旧实例不受影响
	err := m.Start(addr, &WebServeTLSConfig{
		Enabled:  true,
		CertPath: filepath.Join(t.TempDir(), "missing-cert.pem"),
		KeyPath:  filepath.Join(t.TempDir(), "missing-key.pem"),
	})
	if err == nil {
		t.Fatal("用户证书路径无效时 Start 应返回错误")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTLSCertInvalid {
		t.Errorf("错误应为 E_TLS_CERT_INVALID AppError, got %v", err)
	}
	if !m.Running() {
		t.Fatal("证书加载失败后旧实例应仍运行（无半死态）")
	}
	if code := httpGetStatus(t, http.DefaultClient, url); code != http.StatusOK {
		t.Errorf("证书加载失败后旧实例应继续服务, got %d", code)
	}

	// 仅填证书路径其一：配置校验拒绝，旧实例不受影响
	err = m.Start(addr, &WebServeTLSConfig{Enabled: true, CertPath: "cert-only.pem"})
	if err == nil {
		t.Fatal("仅填证书路径应返回错误")
	}
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeTLSCertInvalid {
		t.Errorf("单边路径错误应为 E_TLS_CERT_INVALID AppError, got %v", err)
	}
	if !m.Running() {
		t.Fatal("配置校验失败后旧实例应仍运行")
	}
}

// TestWebServeManager_TLSUserCertPaths 用户自带证书形态：cert/key PEM 路径
// 非空时跳过自签，按路径加载并生效。
func TestWebServeManager_TLSUserCertPaths(t *testing.T) {
	app := newWebServeTestApp(t)
	m := app.webServe

	// 以前置生成的证书对模拟用户证书（EnsureSelfSignedCert 产物即合法 PEM 对）
	certDir := t.TempDir()
	certPath, keyPath, err := server.EnsureSelfSignedCert(certDir)
	if err != nil {
		t.Fatalf("构造用户证书 fixture: %v", err)
	}

	if err := m.Start("127.0.0.1:0", &WebServeTLSConfig{
		Enabled:  true,
		CertPath: certPath,
		KeyPath:  keyPath,
	}); err != nil {
		t.Fatalf("Start（用户证书）: %v", err)
	}
	if code := httpGetStatus(t, tlsHTTPClient(), "https://"+m.listenAddr()+"/healthz"); code != http.StatusOK {
		t.Errorf("用户证书路径下 https 应可达, got %d", code)
	}
	// 用户证书目录不应生成自签文件（双非空跳过自签引导）
	if _, err := os.Stat(filepath.Join(webServeDataDir, server.DefaultTLSCertFile)); !os.IsNotExist(err) {
		t.Error("用户证书路径生效时不应触发自签引导落盘")
	}
}

// TestWebServeManager_TLSWSSOverTLS wss 客户端经 TLS 链路升级收发帧：gorilla
// 升级对 tls.Conn 透明（Hijack 交接底层连接），hub 广播帧经 wss 到达。
func TestWebServeManager_TLSWSSOverTLS(t *testing.T) {
	app := newWebServeTestApp(t)
	redirectTLSDataDir(t)
	m := app.webServe

	if err := m.Start("127.0.0.1:0", &WebServeTLSConfig{Enabled: true}); err != nil {
		t.Fatalf("Start TLS: %v", err)
	}
	token, err := server.LoadOrCreateToken(serveTokenFile)
	if err != nil {
		t.Fatalf("读访问令牌: %v", err)
	}
	conn, resp, err := (&websocket.Dialer{
		Subprotocols:     []string{token},
		HandshakeTimeout: 5 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // 自签证书测试信任
	}).Dial("wss://"+m.listenAddr()+"/ws", nil)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("wss 升级失败: %v", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	for m.hub.ClientCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("等待 WS 客户端注册超时")
		}
		time.Sleep(5 * time.Millisecond)
	}

	m.hub.Emit("tls-ws-test", "payload-1")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	// 连接建立期广播的连接数事件帧先到达（跳过），读到目标帧为止
	var msg []byte
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("wss 收帧失败: %v", err)
		}
		if !strings.Contains(string(payload), "ws-connections:changed") {
			msg = payload
			break
		}
	}
	if !strings.Contains(string(msg), "tls-ws-test") || !strings.Contains(string(msg), "payload-1") {
		t.Errorf("wss 帧内容不符: %s", msg)
	}
}

// TestApp_WebTokenNamedDetails 连接明细快照的令牌名映射（装配层单点
// webTokenNamedDetails）：命中名册补全 TokenName 且其余字段原样保留；名册
// miss（令牌已撤销但连接未断）降级空串；webTokenSvc 未装配全部降级空串。
// 三形态均不 panic、不阻断明细呈现（前端对空串 fallback「已撤销令牌」）。
func TestApp_WebTokenNamedDetails(t *testing.T) {
	app := newWebServeTestApp(t)
	if app.webTokenSvc == nil {
		t.Fatal("前置：测试 App 应已装配 webTokenSvc")
	}
	_, rec, err := app.webTokenSvc.Create("手机", 0)
	if err != nil {
		t.Fatalf("前置：创建令牌失败: %v", err)
	}
	details := []server.ConnDetail{
		{ID: 1, IP: "192.168.1.5", TokenID: rec.ID},
		{ID: 2, IP: "10.0.0.8", TokenID: "revoked-token-id"},
	}
	named := app.webTokenNamedDetails(details)
	if len(named) != 2 {
		t.Fatalf("映射后明细条数应不变, got %d", len(named))
	}
	if named[0].TokenName != "手机" {
		t.Fatalf("命中名册应补全 TokenName=手机, got %q", named[0].TokenName)
	}
	if named[0].ID != 1 || named[0].IP != "192.168.1.5" || named[0].TokenID != rec.ID {
		t.Fatalf("映射不应改动其余字段, got %+v", named[0])
	}
	if named[1].TokenName != "" {
		t.Fatalf("名册 miss（令牌已撤销）应降级空串, got %q", named[1].TokenName)
	}

	// webTokenSvc 未装配：全部降级空串不 panic
	app.webTokenSvc = nil
	named = app.webTokenNamedDetails(details)
	for i, d := range named {
		if d.TokenName != "" {
			t.Fatalf("webTokenSvc 未装配时 TokenName 应为空串, got [%d]=%q", i, d.TokenName)
		}
	}
}
