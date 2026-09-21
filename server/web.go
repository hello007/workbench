package server

import (
	"crypto/subtle"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// WebOptions 浏览器访问模式（serve 模式）HTTP 服务配置。
type WebOptions struct {
	// Assets 静态资产源（frontend/dist 子文件系统），由调用方（main 包）
	// 从 embed.FS 派生传入；nil 时静态路径返回 404。
	Assets fs.FS
	// Token 访问令牌（LoadOrCreateToken 生成/读取）。空串视为未配置，
	// 此时除 /healthz 外全部请求拒绝（fail closed）。
	Token string
	// RPCTarget /api/rpc 通用翻译层的调用目标（App 实例，any + reflect 路由，
	// 避免 server 包反向依赖 main 包）。非 nil 时挂载 /api/rpc（经 token 认证）；
	// nil 时 /api/rpc 不挂载（请求落入静态路径分支被 405 拒绝）。
	RPCTarget any
	// WSHub WebSocket 事件广播 hub（serve 模式事件出口）。非 nil 时挂载 /ws
	// 端点——认证在 hub 内升级前完成（支持 ?token= 与 Sec-WebSocket-Protocol
	// 子协议两种浏览器通道，见 ws.go），不套 requireToken（其 header/查询参数
	// 提取不覆盖子协议通道）；nil 时 /ws 不挂载（落入静态路径分支被 405 拒绝）。
	WSHub *WSHub
}

// healthPath 健康检查路径，无敏感信息，豁免 token 认证（供探活/手工连通性验证）。
const healthPath = "/healthz"

// WebHandler serve 模式 HTTP handler：token 认证 + 静态资产 + SPA fallback。
// 导出类型以支持 SetToken 令牌热轮换（桌面设置页「重新生成令牌」运行期生效）。
type WebHandler struct {
	files fs.FS
	mu    sync.RWMutex // 保护 token，认证读与轮换写互斥
	token string
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
//     ?token= 查询参数，便于首次手工验证）后进入静态资产服务；
//     未命中静态文件的路径 SPA fallback 到 index.html（前端 history 路由可达）。
func NewWebHandler(opts WebOptions) *WebHandler {
	h := &WebHandler{files: opts.Assets, token: opts.Token}
	mux := http.NewServeMux()
	mux.HandleFunc(healthPath, h.serveHealth)
	if opts.RPCTarget != nil {
		mux.Handle(rpcPath, h.requireToken(NewRPCHandler(opts.RPCTarget)))
	}
	if opts.WSHub != nil {
		mux.Handle(wsPath, opts.WSHub)
	}
	preview := PreviewHandler()
	mux.Handle("/preview-pdf", preview)
	mux.Handle(previewRawPrefix, preview)
	mux.Handle("/", h.requireToken(http.HandlerFunc(h.serveStatic)))
	h.mux = mux
	return h
}

// ServeHTTP 委托内部路由表处理请求（实现 http.Handler）。
func (h *WebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// SetToken 热轮换访问令牌：立即生效于后续全部认证判断（旧令牌随即失效）。
// 已建立的 WebSocket 连接不在数据面校验令牌，保持有效至断开重连。
func (h *WebHandler) SetToken(token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.token = token
}

// serveHealth 健康检查端点（免认证）。
func (h *WebHandler) serveHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// requireToken token 认证中间件：未授权返回 401 JSON（安全失败关闭）。
func (h *WebHandler) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.authorized(r) {
			// 只记路径不记完整 URL：?token= 查询参数形式的令牌不得进日志
			slog.Warn("unauthorized request", "path", r.URL.Path, "remote", r.RemoteAddr)
			writePreviewError(w, http.StatusUnauthorized, "未授权访问：请携带访问令牌")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authorized 校验请求携带的令牌。恒定时间比较防时序侧信道；
// 服务端令牌为空（未配置）时一律拒绝。
func (h *WebHandler) authorized(r *http.Request) bool {
	h.mu.RLock()
	token := h.token
	h.mu.RUnlock()
	if token == "" {
		return false
	}
	provided := extractToken(r)
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

// extractToken 从请求提取令牌，优先级：Authorization: Bearer > X-Auth-Token 头
// > ?token= 查询参数（便于首次手工验证）。
func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if t := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")); t != "" {
			return t
		}
	}
	if t := r.Header.Get("X-Auth-Token"); t != "" {
		return t
	}
	return r.URL.Query().Get("token")
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
