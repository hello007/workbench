package server

import (
	"crypto/subtle"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

// 认证事件名（OnAuth 回调的 Event 取值；service 层审计常量 AuditEventAuth*
// 与此同值，字符串由装配层透传落盘）。
const (
	AuthEventSuccess = "auth_success"
	AuthEventFailure = "auth_failure"
)

// AuthEvent 认证事件（requireToken 与 /ws 握手两处认证点各触发一次成功/失败
// 回调，serve 装配层注入审计落盘）。
type AuthEvent struct {
	Event      string    // AuthEventSuccess / AuthEventFailure
	TokenID    string    // 命中的 token 记录 ID（失败为空）
	RemoteAddr string    // 来源地址（r.RemoteAddr，直连形态不做 XFF 信任）
	UserAgent  string    // 客户端 User-Agent
	At         time.Time // 事件时间
}

// emitAuthEvent 认证事件出口（WebHandler 与 WSHub 共用）：onAuth 为 nil 时
// 静默跳过（nil 安全）。调用方不持任何锁（onAuth 实现方自行保证并发安全）。
func emitAuthEvent(onAuth func(AuthEvent), event, tokenID string, r *http.Request) {
	if onAuth == nil {
		return
	}
	onAuth(AuthEvent{
		Event:      event,
		TokenID:    tokenID,
		RemoteAddr: r.RemoteAddr,
		UserAgent:  r.UserAgent(),
		At:         time.Now(),
	})
}

// WebOptions 浏览器访问模式（serve 模式）HTTP 服务配置。
type WebOptions struct {
	// Assets 静态资产源（frontend/dist 子文件系统），由调用方（main 包）
	// 从 embed.FS 派生传入；nil 时静态路径返回 404。
	Assets fs.FS
	// Verify 令牌校验函数（见 TokenVerifier）。多 token 记录的哈希集合校验由
	// 实现方持有（生产为 service.WebTokenService.Verify），本层无状态快照——
	// 新建/撤销/过期在实现方即时生效，无需热轮换通知。nil 时除 /healthz 外
	// 全部请求拒绝（fail closed）。
	Verify TokenVerifier
	// OnAuth 认证事件回调：requireToken 与 /ws 握手两处认证点在成功/失败时各
	// 回调一次（serve 装配层注入审计落盘）。nil 安全。
	OnAuth func(AuthEvent)
	// RPCTarget /api/rpc 通用翻译层的调用目标（App 实例，any + reflect 路由，
	// 避免 server 包反向依赖 main 包）。非 nil 时挂载 /api/rpc（经 token 认证）；
	// nil 时 /api/rpc 不挂载（请求落入静态路径分支被 405 拒绝）。
	RPCTarget any
	// WSHub WebSocket 事件广播 hub（serve 模式事件出口）。非 nil 时挂载 /ws
	// 端点——认证在 hub 内升级前完成（支持 Sec-WebSocket-Protocol 子协议 /
	// wb_token cookie / ?token= 三种通道，见 ws.go），不套 requireToken（其
	// header/查询参数提取不覆盖子协议通道）；nil 时 /ws 不挂载（落入静态路径
	// 分支被 405 拒绝）。
	WSHub *WSHub
}

// healthPath 健康检查路径，无敏感信息，豁免 token 认证（供探活/手工连通性验证）。
const healthPath = "/healthz"

// cookieName 浏览器会话 cookie 名：首访经 ?token= 查询参数或 header 通道认证
// 成功后种下，浏览器后续的静态资产、/api/rpc、/ws 请求自动携带，免去再次输
// 令牌。查询参数只随单个请求发送，静态资产级联请求无凭据会全部 401（首访
// 资产级联缺陷的修复点），cookie 是贯通浏览器会话的凭据载体。
const cookieName = "wb_token"

// cookieMaxAge 会话 cookie 有效期（30 天）。与服务端令牌无续期联动：令牌热
// 轮换后旧 cookie 值认证不过即失效，浏览器重新经 ?token= 验证种新会话。
const cookieMaxAge = 30 * 24 * time.Hour

// WebHandler serve 模式 HTTP handler：token 认证 + 静态资产 + SPA fallback。
//
// 校验函数 verify/onAuth 构造后不可变（多 token 语义下撤销/过期经 verify 闭包
// 直读 WebTokenService 状态即时生效，本层无热轮换写路径，原 SetToken 已随
// verify 化删除）。
type WebHandler struct {
	files  fs.FS
	verify TokenVerifier   // 令牌校验函数（nil = 一切拒绝 fail closed）
	onAuth func(AuthEvent) // 认证事件出口（可 nil）
	// mux 实际路由表（各端点挂载），ServeHTTP 委托转发
	mux http.Handler
}

// NewWebHandler 构建浏览器访问模式 HTTP handler。
//
// 路由分层：
//   - /healthz 免认证健康检查（仅返回固定 JSON 状态，不暴露版本/路径等信息）；
//   - /api/rpc 通用 RPC 翻译层（RPCTarget 非 nil 时挂载），经 token 认证中间件；
//   - /ws WebSocket 事件广播端点（WSHub 非 nil 时挂载），认证在 hub 内升级前完成；
//   - /preview-pdf、/preview-raw/ 文件预览路由，认证语义与桌面模式一致（桌面经
//     AssetServer.Handler 挂载且无 token：本地信任模型，文件路径由用户在 UI 中
//     选择，且 iframe 加载无法携带 Authorization 头，套 token 会让 PDF/HTML 预览
//     在浏览器模式不可用）。serve 模式下该信任模型的网络面防护依赖默认回环
//     绑定，改绑非回环地址须 --listen 显式指定或设置页确认（见 model/settings.go
//     DefaultWebServeBindAddress 注释）；
//   - 其余路径经 token 认证中间件（Authorization: Bearer / X-Auth-Token 头 /
//     wb_token cookie / ?token= 查询参数，便于首次手工验证；认证成功且会话
//     cookie 缺失或不一致时种下 cookie，见 issueSessionCookie）后进入静态资产
//     服务；未命中静态文件的路径 SPA fallback 到 index.html（前端 history 路由
//     可达）。
func NewWebHandler(opts WebOptions) *WebHandler {
	h := &WebHandler{files: opts.Assets, verify: opts.Verify, onAuth: opts.OnAuth}
	mux := http.NewServeMux()
	mux.HandleFunc(healthPath, h.serveHealth)
	if opts.RPCTarget != nil {
		mux.Handle(rpcPath, h.requireToken(NewRPCHandler(opts.RPCTarget)))
	}
	if opts.WSHub != nil {
		mux.Handle(wsPath, opts.WSHub)
	}
	preview := PreviewHandler()
	mux.Handle("/preview-pdf", requireSameOrigin(preview))
	mux.Handle(previewRawPrefix, requireSameOrigin(preview))
	mux.Handle("/", h.requireToken(http.HandlerFunc(h.serveStatic)))
	h.mux = mux
	return h
}

// ServeHTTP 委托内部路由表处理请求（实现 http.Handler）。
func (h *WebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// serveHealth 健康检查端点（免认证）。
func (h *WebHandler) serveHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// requireSameOrigin serve 模式预览路由的同源校验中间件：Sec-Fetch-Site 为
// same-origin 放行，cross-site / none / 缺失一律 403 拒绝。
//
// 取舍说明：
//   - 预览端点（/preview-pdf、/preview-raw/）可读取任意本地绝对路径，免 token
//     的信任模型是「调用方为用户在 UI 中主动预览的同源页面」。该信任在 serve
//     模式下面向网络，须以浏览器上下文约束收敛：预览仅由前端页面内嵌框架
//     （iframe/embed 媒体元素）加载，同源加载必带 Sec-Fetch-Site: same-origin
//     （Chromium/Firefox/Safari 自 2020 年起全量默认携带，含 WebView2），不误伤。
//   - cross-site 拒绝恶意网站跨站嵌入/表单导航探测本地文件；none 拒绝地址栏
//     直接导航；缺失即非浏览器客户端（curl/脚本探测），一并拒绝——端点合法
//     调用方仅有同源页面，无「合法但缺头」的调用形态。
//   - 桌面模式零变化：桌面预览经 AssetServer.Handler 直挂 PreviewHandler
//     （本地信任模型，见 NewWebHandler 路由分层注释），不经本中间件。
func requireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if site := r.Header.Get("Sec-Fetch-Site"); site != "same-origin" {
			slog.Warn("preview request rejected by sec-fetch-site check",
				"path", r.URL.Path, "site", site, "remote", r.RemoteAddr)
			writePreviewError(w, http.StatusForbidden, "预览请求仅允许同源页面发起")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireToken token 认证中间件：未授权返回 401 JSON（安全失败关闭）；授权
// 成功后经 issueSessionCookie 贯通浏览器会话。成功/失败各触发一次 OnAuth
// 认证事件（装配层注入审计落盘；静态资产每请求都过本中间件，认证事件为
// 每请求粒度，lastUsedAt 的写放大由 WebTokenService 的 debounce 落盘消化）。
func (h *WebHandler) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		matched, tokenID, ok := h.authorized(r)
		if !ok {
			emitAuthEvent(h.onAuth, AuthEventFailure, "", r)
			// 只记路径不记完整 URL：?token= 查询参数形式的令牌不得进日志
			slog.Warn("unauthorized request", "path", r.URL.Path, "remote", r.RemoteAddr)
			writePreviewError(w, http.StatusUnauthorized, "未授权访问：请携带访问令牌")
			return
		}
		emitAuthEvent(h.onAuth, AuthEventSuccess, tokenID, r)
		h.issueSessionCookie(w, r, matched)
		next.ServeHTTP(w, r)
	})
}

// authorized 校验请求携带的令牌：credentialTokens 的候选逐一独立比对，任一
// 命中即通过（与 ws.go authorized 的多通道语义一致）。候选有序但不短路——
// 任一通道命中即须持有有效令牌，通道间无降级面；反之若高优先级通道存在即
// 短路提取，令牌轮换后浏览器携带的旧 cookie 会遮蔽 ?token= 里的新令牌，
// 「重新经 ?token= 验证种新会话」的轮换恢复路径永远 401。逐候选调用 verify
// （哈希比对恒定时间由实现方保证）；verify 未配置（nil）时一律拒绝。
//
// 返回值 matched 为命中的明文凭据候选（客户端已自证持有）：调用方
// issueSessionCookie 以该值种会话 cookie（多 token 语义下 cookie 跟随客户端
// 实际使用的令牌，而非「服务端唯一令牌」快照——多 token 下不存在唯一令牌），
// tokenID 为命中记录 ID（审计归因键）。
func (h *WebHandler) authorized(r *http.Request) (matched, tokenID string, ok bool) {
	if h.verify == nil {
		return "", "", false
	}
	for _, provided := range credentialTokens(r) {
		if provided == "" {
			continue
		}
		if id, valid := h.verify(provided); valid {
			return provided, id, true
		}
	}
	return "", "", false
}

// credentialTokens 按优先级收集请求携带的全部凭据候选（可同时多个，空候选
// 不收）：Authorization: Bearer > X-Auth-Token 头 > wb_token cookie（浏览器
// 会话）> ?token= 查询参数（便于首次手工验证）。
func credentialTokens(r *http.Request) []string {
	var candidates []string
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if t := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")); t != "" {
			candidates = append(candidates, t)
		}
	}
	if t := r.Header.Get("X-Auth-Token"); t != "" {
		candidates = append(candidates, t)
	}
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		candidates = append(candidates, c.Value)
	}
	if t := r.URL.Query().Get("token"); t != "" {
		candidates = append(candidates, t)
	}
	return candidates
}

// issueSessionCookie 浏览器会话贯通：凭据经 header/query 通道（而非 cookie）
// 认证成功且会话 cookie 缺失或与命中的令牌不一致时，种下 wb_token cookie
// （值为命中的明文凭据候选，多 token 下 cookie 跟随客户端实际使用的令牌），
// 使浏览器后续静态资产、/api/rpc、/ws 请求自动携带凭据。cookie 已一致的
// 请求不重复 Set-Cookie。token 为 authorized 返回的命中候选（见 authorized
// 注释）。令牌值经 HttpOnly 阻断脚本读取，SameSite=Strict 收敛跨站携带面；
// HTTPS 链路（r.TLS 非 nil）追加 Secure。
func (h *WebHandler) issueSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	if token == "" || h.hasValidCookie(r, token) {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(cookieMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})
}

// hasValidCookie 判断请求携带的会话 cookie 是否与服务端当前令牌一致
// （恒定时间比较，与其它通道同防时序侧信道）。一致则不再重复 Set-Cookie。
func (h *WebHandler) hasValidCookie(r *http.Request, token string) bool {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(token)) == 1
}

// serveStatic 静态资产服务，未命中文件（含目录与 SPA 前端路由）fallback 到 index.html。
func (h *WebHandler) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writePreviewError(w, http.StatusMethodNotAllowed, "仅支持 GET/HEAD 请求")
		return
	}
	if h.files == nil {
		writePreviewError(w, http.StatusNotFound, "静态资产未加载")
		return
	}

	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	entry, err := fs.Stat(h.files, name)
	if err != nil || entry.IsDir() {
		// 未命中静态文件：SPA 前端路由与目录路径统一回退到 index.html，
		// 不暴露目录列表
		h.serveIndex(w, r)
		return
	}
	http.FileServer(http.FS(h.files)).ServeHTTP(w, r)
}

// serveIndex 返回 SPA 入口 index.html；资产源缺失该文件时返回 404。
func (h *WebHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	f, err := h.files.Open("index.html")
	if err != nil {
		writePreviewError(w, http.StatusNotFound, "index.html 不存在")
		return
	}
	defer f.Close()
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, "index.html", time.Time{}, rs)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.Copy(w, f)
}
