package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// PreviewHandler 处理前端对本地文件的预览请求。
//
// 两条路由：
//   - `/preview-pdf?path=<URL编码的本地绝对路径>`：PDF 内嵌预览（POC-1，方案 B），
//     handler 解析 path、做路径安全校验后用 http.ServeFile 返回 PDF 二进制
//     （ServeFile 原生支持 HTTP Range / Last-Modified，满足大 PDF 按需读取）。
//   - `/preview-raw/<URL编码的本地绝对路径>`：HTML 渲染预览的静态资源路由
//     （css/js/图片/字体等），供 iframe srcdoc 注入的 <base> 相对解析。
//     必须用路径式而非 query 式：相对 URL 解析会丢弃 base 的 query 串，
//     只有路径式 base（/preview-raw/<dir>/）能让 style.css 等相对引用落到本路由。
//
// 设计要点：
//   - /preview-pdf 仅放行 .pdf、/preview-raw 仅放行静态资源扩展名白名单，
//     避免被当作通用文件下载器（路径穿越/越权读取）。
//   - /preview-raw 不做「HTML 入口所在目录子树」约束：iframe 内相对引用经浏览器
//     解析 ../ 后服务端已无法还原入口目录；安全模型与 /preview-pdf 对齐——
//     白名单扩展 + 普通文件校验。本地桌面应用中 PreviewFile 本就允许用户预览
//     任意本地路径，白名单扩展不扩大既有能力面；且 iframe sandbox（opaque
//     origin）内脚本无法用 fetch 读取响应，仅 <link>/<script>/<img> 等标签加载。
//   - 解析后用 filepath.Clean + Abs 规范化，校验必须是普通文件（非目录）。
//   - 不在 handler 内做权限模型判断（本地桌面应用，文件路径由用户在 UI 选择），
//     但限定扩展名可显著降低误用风险。
//
// 注意：AssetServer.Handler 仅在 Assets（embed.FS）未命中时被调用，
// 因此 /preview-pdf、/preview-raw 这类非静态资源路由不会被前端 dist 拦截。
func PreviewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 仅接受 GET / HEAD（ServeFile 内部也依赖这两个方法做 Range）
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writePreviewError(w, http.StatusMethodNotAllowed, "仅支持 GET/HEAD 请求")
			return
		}

		if strings.HasPrefix(r.URL.Path, previewRawPrefix) {
			servePreviewRaw(w, r)
			return
		}

		rawPath := r.URL.Query().Get("path")
		if rawPath == "" {
			writePreviewError(w, http.StatusBadRequest, "缺少 path 参数")
			return
		}

		// 规范化路径：filepath.Clean 处理 ..、多余分隔符；Abs 转绝对路径。
		cleaned := filepath.Clean(rawPath)
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			writePreviewError(w, http.StatusBadRequest, "路径解析失败: "+err.Error())
			return
		}

		// 扩展名白名单：当前仅允许 PDF（POC-1 范围）
		ext := strings.ToLower(filepath.Ext(abs))
		if ext != ".pdf" {
			writePreviewError(w, http.StatusBadRequest, "仅支持预览 .pdf 文件")
			return
		}

		// 校验文件存在且为普通文件（防目录穿越到目录本身）
		info, err := os.Stat(abs)
		if err != nil {
			writePreviewError(w, http.StatusNotFound, "文件不存在或无法访问: "+err.Error())
			return
		}
		if info.IsDir() {
			writePreviewError(w, http.StatusBadRequest, "指定的路径是目录，无法预览")
			return
		}

		// http.ServeFile 自动处理 Content-Type、Range、Last-Modified、ETag。
		// 对大 PDF，WebView2 / pdfjs 会发 Range 请求按需读取，ServeFile 原生支持。
		http.ServeFile(w, r, abs)
	})
}

// previewRawPrefix 是 HTML 渲染预览静态资源路由的路径前缀。
const previewRawPrefix = "/preview-raw/"

// previewRawExts 为 /preview-raw 放行的扩展名白名单（小写、含点）。
// 覆盖：HTML 页面本体（iframe 内相对链接跳转目标）与常见静态资源
// （样式/脚本/图片/字体/数据），使本地 HTML 的相对引用能完整渲染。
var previewRawExts = map[string]struct{}{
	".html": {}, ".htm": {},
	".css": {}, ".js": {}, ".mjs": {}, ".map": {},
	".json": {}, ".txt": {}, ".xml": {}, ".csv": {},
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {},
	".svg": {}, ".webp": {}, ".bmp": {}, ".ico": {},
	".woff": {}, ".woff2": {}, ".ttf": {}, ".otf": {}, ".eot": {},
}

// servePreviewRaw 处理 /preview-raw/<URL编码的本地绝对路径>：
// 白名单扩展 + 绝对路径 + 普通文件校验后 ServeFile 返回。
// r.URL.Path 为 Go http server 解码后的路径（中文等非 ASCII 直接可用）。
func servePreviewRaw(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimPrefix(r.URL.Path, previewRawPrefix)
	if rawPath == "" {
		writePreviewError(w, http.StatusBadRequest, "缺少资源路径")
		return
	}

	cleaned := filepath.Clean(rawPath)
	abs, err := filepath.Abs(cleaned)
	if err != nil || !filepath.IsAbs(cleaned) {
		// Clean 后必须已是绝对路径（盘符开头）；相对路径/解析失败一律拒绝，
		// 防止相对服务进程工作目录的任意读取。
		writePreviewError(w, http.StatusBadRequest, "资源路径必须是绝对路径")
		return
	}

	ext := strings.ToLower(filepath.Ext(abs))
	if _, ok := previewRawExts[ext]; !ok {
		writePreviewError(w, http.StatusBadRequest, "不支持预览该资源类型: "+ext)
		return
	}

	info, err := os.Stat(abs)
	if err != nil {
		writePreviewError(w, http.StatusNotFound, "资源不存在或无法访问: "+err.Error())
		return
	}
	if info.IsDir() {
		writePreviewError(w, http.StatusBadRequest, "指定的路径是目录，无法加载")
		return
	}

	// 用 ServeContent 而非 ServeFile：ServeFile 对 basename 为 index.html 的请求
	// 会 301 重定向到目录（Go 标准库的 index 行为），导致 iframe 内跳 index.html
	// 落到目录请求被 400。ServeContent 按 abs 直接输出，仍支持 Range 与
	// Last-Modified，Content-Type 由 name 的扩展名推断。
	f, err := os.Open(abs)
	if err != nil {
		writePreviewError(w, http.StatusNotFound, "资源无法打开: "+err.Error())
		return
	}
	defer f.Close()
	http.ServeContent(w, r, filepath.Base(abs), info.ModTime(), f)
}

// writePreviewError 以简短 JSON 返回错误信息，便于前端/排查定位。
func writePreviewError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + escapeJSON(msg) + `"}`))
}

// escapeJSON 对错误消息做最小转义，避免破坏 JSON 结构。
func escapeJSON(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
