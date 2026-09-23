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
import plaintext from 'highlight.js/lib/languages/plaintext'

// 语言注册与 FilePreviewRenderer 同一套常规语言（hljs 为模块单例，注册幂等，
// 两处重复注册无副作用，仅为各自独立可加载）；plaintext 为额外注册项：
// 下方 highlight 回调对未识别语言固定回退 language:'plaintext'，漏注册会在
// 渲染时稳定触发 hljs「Could not find the language 'plaintext'」报错。
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
hljs.registerLanguage('plaintext', plaintext)

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

// ===== chat-question 选择题卡片渲染 =====
// 约定：模型在回复中输出 ```chat-question JSON``` 围栏承载选择题（格式见
// ChatService.chatQuestionPreamble），此处拦截该围栏渲染为可点选卡片；
// 点击与提交交互由 AiChatPanel 在气泡容器上事件委托处理（v-html 内无法绑 Vue 事件）。

// 默认 fence 渲染器（markdown-it 内置规则，显式保存以便普通代码块/非法 JSON 降级复用）
const defaultFence =
  md.renderer.rules.fence ||
  ((tokens, idx, options, env, self) => self.renderToken(tokens, idx, options))

/**
 * 校验并解析 chat-question 块 JSON。
 * @param {string} content 围栏内文本
 * @returns {Array|null} 合法返回 questions 数组（每项 {question, options:[{label,description?}], multiSelect?}），非法返回 null
 */
function parseChatQuestion(content) {
  let data
  try {
    data = JSON.parse(content)
  } catch {
    return null
  }
  const questions = data && data.questions
  if (!Array.isArray(questions) || questions.length === 0) return null
  for (const q of questions) {
    if (!q || typeof q.question !== 'string' || !q.question.trim()) return null
    if (!Array.isArray(q.options) || q.options.length === 0) return null
    for (const opt of q.options) {
      if (!opt || typeof opt.label !== 'string' || !opt.label.trim()) return null
    }
  }
  return questions
}

/**
 * 把合法 questions 渲染为选择题卡片 HTML（属性与文本均经 escapeHtml，防注入）。
 * 选项 data-q/data-idx 供点击委托定位；多选题项带 data-multi 与提示文案；
 * 提交按钮初始 disabled，选中任一项后由点击委托启用。
 * @param {Array} questions 已校验的问题数组
 * @returns {string} HTML
 */
function renderChatQuestionCard(questions) {
  const esc = md.utils.escapeHtml
  const items = questions
    .map((q, qi) => {
      const multi = q.multiSelect === true
      const opts = q.options
        .map((opt, oi) => {
          const title = typeof opt.description === 'string' && opt.description.trim()
            ? ` title="${esc(opt.description)}"`
            : ''
          return (
            `<button type="button" class="chat-question-option" data-q="${qi}" data-idx="${oi}"` +
            `${title}>${esc(opt.label)}</button>`
          )
        })
        .join('')
      const hint = multi ? '<span class="chat-question-hint">（可多选）</span>' : ''
      return (
        `<div class="chat-question-item" data-question="${esc(q.question)}"${multi ? ' data-multi="true"' : ''}>` +
        `<div class="chat-question-title">${esc(q.question)}${hint}</div>` +
        `<div class="chat-question-options">${opts}</div>` +
        `</div>`
      )
    })
    .join('')
  return (
    '<div class="chat-question">' +
    items +
    '<div class="chat-question-actions">' +
    '<button type="button" class="chat-question-submit" disabled>发送选择</button>' +
    '</div>' +
    '</div>'
  )
}

// fence 渲染分流：info 为 chat-question 且 JSON 合法渲染交互卡片；
// 非法 JSON / 其他语言走默认代码块渲染（未闭合围栏在流式期间同样按普通代码块容错显示）
md.renderer.rules.fence = (tokens, idx, options, env, self) => {
  const token = tokens[idx]
  const lang = (token.info || '').trim().split(/\s+/)[0]
  if (lang !== 'chat-question') {
    return defaultFence(tokens, idx, options, env, self)
  }
  const questions = parseChatQuestion(token.content)
  if (!questions) {
    return defaultFence(tokens, idx, options, env, self)
  }
  return renderChatQuestionCard(questions)
}
