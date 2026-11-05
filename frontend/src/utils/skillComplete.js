/**
 * skill 斜杠补全纯函数工具。
 *
 * AI 对话输入框行首 `/` 触发已装 skill 补全弹层（对齐 claude CLI 交互），
 * 本模块抽出 token 解析 / 补全替换 / 过滤三个纯函数，独立单测、组件内消费。
 * 语义约定：skill 作为 slash command 需位于消息行首（与 CLI 一致），行中 `/` 不触发。
 */

/**
 * 解析光标处的斜杠 token。
 *
 * 规则：光标所在行行首为 '/' 且光标位于 '/' 到该行第一个空白/换行之间（含两端）
 * 才 active；query 为 '/' 之后的片段（可空）。多行输入任一行行首均可触发。
 *
 * @param {string} text 输入框全文
 * @param {number} caret 光标位置（selectionStart）
 * @returns {{active: boolean, start: number, end: number, query: string}}
 *   active=false 时 start/end/query 无意义（均为 -1/''）
 */
export function extractSlashToken(text, caret) {
  if (!text || caret == null || caret < 0 || caret > text.length) {
    return { active: false, start: -1, end: -1, query: '' }
  }
  // 光标所在行的起点：向前找最近换行
  let lineStart = 0
  for (let i = caret - 1; i >= 0; i--) {
    if (text[i] === '\n') {
      lineStart = i + 1
      break
    }
  }
  // 行首必须是 '/'
  if (text[lineStart] !== '/') {
    return { active: false, start: -1, end: -1, query: '' }
  }
  // token 终点：行内第一个空白（空格/tab），无空白则到行尾
  let end = lineStart + 1
  while (end < text.length && text[end] !== ' ' && text[end] !== '\t' && text[end] !== '\n') {
    end++
  }
  // 光标必须在 token 范围内（[lineStart, end]；end 允许 = 光标停在 token 尾后）
  if (caret > end || caret < lineStart) {
    return { active: false, start: -1, end: -1, query: '' }
  }
  return { active: true, start: lineStart, end, query: text.slice(lineStart + 1, end) }
}

/**
 * 应用补全：把 [start, end) 片段整体替换为 command + 尾随空格。
 * 尾随空格使 token 自然结束（弹层随之关闭，与 CLI 一致），并提示可继续输入正文；
 * token 终点若恰为空格（extractSlashToken 的 end 落在空白上、空白未被消费），
 * 吸收该空格避免产出双空格。
 *
 * @param {string} text 输入框全文
 * @param {{start: number, end: number}} range extractSlashToken 返回的替换范围
 * @param {string} command skill 命令（如 /drawio、/ab-office:agree-slides）
 * @returns {{text: string, caret: number}} 替换后全文与光标位置（空格之后）
 */
export function applyCompletion(text, range, command) {
  const end = text[range.end] === ' ' ? range.end + 1 : range.end
  const completed = command + ' '
  return {
    text: text.slice(0, range.start) + completed + text.slice(end),
    caret: range.start + completed.length
  }
}

/**
 * 按关键字过滤 skill 列表：command/name/description 任一字段小写包含即命中。
 * 空 query 返回全量。
 *
 * @param {Array<{command: string, name: string, description: string}>} skills
 * @param {string} query '/' 之后的过滤片段
 * @returns {Array} 过滤后子集（保序）
 */
export function filterSkills(skills, query) {
  const kw = (query || '').trim().toLowerCase()
  if (!kw) return skills.slice()
  return skills.filter(s =>
    (s.command || '').toLowerCase().includes(kw) ||
    (s.name || '').toLowerCase().includes(kw) ||
    (s.description || '').toLowerCase().includes(kw)
  )
}
