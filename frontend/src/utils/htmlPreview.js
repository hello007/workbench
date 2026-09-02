/**
 * HTML 渲染预览的 srcdoc 组装工具。
 *
 * 渲染架构：iframe srcdoc + sandbox="allow-scripts"（无 allow-same-origin，
 * 脚本运行在 opaque origin，无法访问 parent / Wails 绑定方法）。
 * 相对资源（css/js/图片/字体）通过注入 <base> 指向后端 /preview-raw 路径式
 * 路由解析；外链 http(s) 由注入脚本拦截后 postMessage 通知主页面，
 * 走 BrowserOpenURL 用系统默认浏览器打开。
 */

// /preview-raw 前缀须与 server/preview.go 的 previewRawPrefix 保持一致
const PREVIEW_RAW_PREFIX = '/preview-raw/'

// 拦截外链点击并 postMessage 给 parent 的注入脚本。
// capture 阶段监听，相对链接不拦截（交由 iframe 基于原生导航跳转，目标仍是 /preview-raw）。
const EXTERNAL_LINK_INTERCEPTOR = `<script>(function(){
  document.addEventListener('click', function(e){
    var t = e.target;
    var a = t && t.closest ? t.closest('a') : null;
    if (!a) return;
    var href = a.getAttribute('href') || '';
    if (/^https?:\\/\\//i.test(href)) {
      e.preventDefault();
      e.stopPropagation();
      parent.postMessage({ type: 'workbench-open-external', url: href }, '*');
    }
  }, true);
})();</script>`

// 取本地绝对路径的目录部分（反斜杠统一正斜杠；无分隔符返回空串）
const dirOfPath = (filePath) => {
  const p = (filePath || '').replaceAll('\\', '/')
  const idx = p.lastIndexOf('/')
  return idx >= 0 ? p.slice(0, idx) : ''
}

/**
 * 组装 HTML 渲染预览的完整 srcdoc。
 *
 * 注入顺序固定为 <base> 在前、拦截脚本在后：
 *   - <base> 必须早于文档中任何相对 URL 引用（css/img）才生效；
 *   - 拦截脚本在 head 阶段执行，capture 监听 document，晚于 DOM 构建无碍。
 *
 * 目录整体 encodeURIComponent（含分隔符 /，形如 D%3A%2Ffoo%2Fbar）：
 *   - URL path 语法允许 %2F，Go http server 会解码 r.URL.Path，handler 收到 D:/foo/bar；
 *   - 相对引用 style.css 基于该 base 解析为 /preview-raw/D%3A%2Ffoo%2Fbar%2Fstyle.css。
 *
 * 注入位置：优先插入到 <head ...> 之后（文档标准位置）；无 head 时前置
 * （HTML 解析器会把 <base> 归入隐式 head，同样生效）。
 *
 * @param {string} content HTML 文件原文
 * @param {string} filePath 当前预览文件的本地绝对路径
 * @returns {string} 可直接绑定到 iframe srcdoc 的文档
 */
export const buildHtmlPreviewDoc = (content, filePath) => {
  const doc = content || ''
  const dir = dirOfPath(filePath)
  const baseTag = dir
    ? `<base href="${PREVIEW_RAW_PREFIX}${encodeURIComponent(dir + '/')}">`
    : ''
  const inject = baseTag + EXTERNAL_LINK_INTERCEPTOR

  const headMatch = doc.match(/<head[^>]*>/i)
  if (headMatch) {
    const idx = headMatch.index + headMatch[0].length
    return doc.slice(0, idx) + inject + doc.slice(idx)
  }
  return inject + doc
}
