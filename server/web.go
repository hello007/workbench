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

// WebOptions 浏览器访问模式（serve 模式）HTTP 服务配置。
type WebOptions struct {
	// Assets 静态资产源（frontend/dist 子文件系统），由调用方（main 包）
	// 从 embed.FS 派生传入；nil 时静态路径返回 404。
	Assets fs.FS
	// Token 访问令牌（LoadOrCreateToken 生成/读取）。空串视为未配置，
	// 此时除 /healthz 外全部请求拒绝（fail closed）。
	Token string
}

// healthPath 健康检查路径，无敏感信息，豁免 token 认证（供探活/手工连通性验证）。
const healthPath = "/healthz"

// webHandler serve 模式 HTTP handler：token 认证 + 静态资产 + SPA fallback。
type webHandler struct {
	files fs.FS
	token string
}

// NewWebHandler 构建浏览器访问模式 HTTP handler。
//
// 路由分层：
//   - /healthz 免认证健康检查（仅返回固定 JSON 状态，不暴露版本/路径等信息）；
//   - 其余路径经 token 认证中间件（Authorization: Bearer / X-Auth-Token 头 /
//     ?token= 查询参数，便于首次手工验证）后进入静态资产服务；
//     未命中静态文件的路径 SPA fallback 到 index.html（前端 history 路由可达）。
func NewWebHandler(opts WebOptions) http.Handler {
	h := &webHandler{files: opts.Assets, token: opts.Token}
	mux := http.NewServeMux()
	mux.HandleFunc(healthPath, h.serveHealth)
	mux.Handle("/", h.requireToken(http.HandlerFunc(h.serveStatic)))
	return mux
}

// serveHealth 健康检查端点（免认证）。
func (h *webHandler) serveHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// requireToken token 认证中间件：未授权返回 401 JSON（安全失败关闭）。
func (h *webHandler) requireToken(next http.Handler) http.Handler {
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
func (h *webHandler) authorized(r *http.Request) bool {
	if h.token == "" {
		return false
	}
	provided := extractToken(r)
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(h.token)) == 1
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
func (h *webHandler) serveStatic(w http.ResponseWriter, r *http.Request) {
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
func (h *webHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
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
