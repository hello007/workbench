import MarkdownIt from 'markdown-it'
import hljs from 'highlight.js/lib/core'
import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import css from 'highlight.js/lib/languages/css'
import jsonLang from 'highlight.js/lib/languages/json'
import sql from 'highlight.js/lib/languages/sql'
import go from 'highlight.js/lib/languages/go'
import yaml from 'highlight.js/lib/languages/yaml'
import markdown from 'highlight.js/lib/languages/markdown'
import bash from 'highlight.js/lib/languages/bash'
import shell from 'highlight.js/lib/languages/shell'
import python from 'highlight.js/lib/languages/python'

// 语言注册与 FilePreviewRenderer 保持同一套（hljs 为模块单例，注册幂等，
// 两处重复注册无副作用，仅为各自独立可加载）。
hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('typescript', typescript)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('html', xml)
hljs.registerLanguage('css', css)
hljs.registerLanguage('json', jsonLang)
hljs.registerLanguage('sql', sql)
hljs.registerLanguage('go', go)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('markdown', markdown)
hljs.registerLanguage('bash', bash)
hljs.registerLanguage('shell', shell)
hljs.registerLanguage('python', python)

// markdown-it 实例：配置对齐 FilePreviewRenderer（html:false 防 XSS、linkify
// 自动链接），代码块走 highlight.js（未注册语言退化为 plaintext）。
// AI 回复中可能出现未闭合的代码围栏（流式中断场景），markdown-it 容错渲染，
// 无需额外处理。
const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true, // 对话场景单个换行即换行（与主流 AI 聊天 UI 一致；文件预览场景 breaks:false 不受影响）
  highlight(code, lang) {
    const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
    try {
      return `<pre class="hljs"><code>${hljs.highlight(code, { language, ignoreIllegals: true }).value}</code></pre>`
    } catch {
      return `<pre class="hljs"><code>${md.utils.escapeHtml(code)}</code></pre>`
    }
  }
})

/**
 * 渲染 markdown 文本为 HTML（AI 对话 assistant 回复专用）。
 * 输入来自 claude CLI 回复文本，html:false 保证原始 HTML 被转义不注入。
 * @param {string} text markdown 文本
 * @returns {string} HTML
 */
export function renderChatMarkdown(text) {
  if (!text) return ''
  return md.render(String(text))
}
