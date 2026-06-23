package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// PreviewHandler 处理前端对本地文件的预览请求。
//
// 当前用于 POC-1（PDF 内嵌预览方案 B 的基础验证）：
// 前端 iframe 以 `/preview-pdf?path=<URL编码的本地绝对路径>` 发起请求，
// handler 解析 path、做路径安全校验后用 http.ServeFile 返回 PDF 二进制
// （ServeFile 原生支持 HTTP Range / Last-Modified，满足大 PDF 按需读取）。
//
// 设计要点：
//   - 仅放行 .pdf 扩展名，避免被当作通用文件下载器（路径穿越/越权读取）。
//   - 解析后用 filepath.Clean + Abs 规范化，校验必须是普通文件（非目录）。
//   - 不在 handler 内做权限模型判断（本地桌面应用，文件路径由用户在 UI 选择），
//     但限定扩展名可显著降低误用风险。
//
// 注意：AssetServer.Handler 仅在 Assets（embed.FS）未命中时被调用，
// 因此 /preview-pdf 这类非静态资源路由不会被前端 dist 拦截。
func PreviewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 仅接受 GET / HEAD（ServeFile 内部也依赖这两个方法做 Range）
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writePreviewError(w, http.StatusMethodNotAllowed, "仅支持 GET/HEAD 请求")
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
