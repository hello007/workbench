/**
 * 临时待办 Markdown 导入解析（纯函数，无副作用、无 Vue 依赖）。
 *
 * 解析目标是对齐「临时待办」二期导出的流水式格式，形成导出 → 导入可逆闭环：
 *   - HH:MM 正文  #标签
 *   - 已完成条目行尾追加「（已完成 HH:MM）」——导入时整条跳过，避免已完成项
 *     被当成正文重新灌回待办区。
 *   - 多行正文从第二行起缩进两空格——导入时拼回同一条待办。
 *
 * 识别范围只有两类待办行：`- [ ] 正文` 与 `- HH:MM 正文  #标签`。标题、段落、
 * `- [x]`、普通列表项一律静默跳过，任何输入都不抛错。
 *
 * 时间只有 HH:MM 没有日期：日期优先取文件名中的 YYYY-MM-DD，否则取导入当天；
 * 组合结果晚于当前时刻时钳到当前时刻。无行内时间的条目 createdAt 返回 0，
 * 由后端在落盘时统一填导入时刻（避免前后端时钟口径分叉）。
 */

// 顶层列表项：行首至多 3 个空格缩进（4+ 空格按续行处理，保持导出格式可逆）
const TOP_ITEM_RE = /^ {0,3}- +(.+)$/
// 续行：去掉恰好两个空格或一个制表符（导出时第二行起缩进两空格；更深的
// 缩进属于正文自身的相对缩进，须原样保留）
const CONTINUATION_RE = /^(?: {2}|\t)(.*)$/
// 复选框：未完成 / 已完成。只认标记本身，不吞其后的空白——后续空白可能是
// 标签的必要前缀（`- [ ]   #标签` 剥离后仍须能识别出行尾标签）
const CHECKBOX_TODO_RE = /^\[[ \t]\](?=\s)/
const CHECKBOX_DONE_RE = /^\[[xX]\]\s*/
// 合法行内时间前缀，要求时间后跟空白
const TIME_PREFIX_RE = /^([01]\d|2[0-3]):([0-5]\d)\s+/
// 导出的已完成标记（全角括号为主，兼容半角）
const DONE_MARK_RE = /[（(]已完成\s*\d{1,2}:\d{2}[）)]\s*$/
// 行尾标签：前置空白 + # + 非空白串（标签内允许空格时无法表达，与导出一致）
const TRAILING_TAG_RE = /\s+#(\S+)\s*$/
// 文件名日期
const FILE_DATE_RE = /^(\d{4})-(\d{2})-(\d{2})\.md$/i

/**
 * 解析 Markdown 文本为待办草稿。
 *
 * @param {string} raw Markdown 原文
 * @param {{ now?: number, fileName?: string, existingTexts?: string[] }} [options]
 *   now：导入时刻（毫秒，默认 Date.now()，测试注入用）
 *   fileName：所选文件路径或文件名，用于提取 YYYY-MM-DD 日期
 *   existingTexts：现有全部待办正文，用于标记疑似重复
 * @returns {{
 *   entries: Array<{ text: string, tags: string[], createdAt: number, duplicate: boolean }>,
 *   importableCount: number,
 *   duplicateCount: number
 * }}
 */
export function parseTempTodoMarkdown(raw, options = {}) {
  const now = Number.isFinite(options.now) ? options.now : Date.now()
  const fileDate = resolveFileDate(options.fileName || '')
  const existingSet = new Set(
    (options.existingTexts || [])
      .map(text => dedupeKey(text))
      .filter(Boolean)
  )

  const entries = []
  const seenInFile = new Set()
  let current = null

  // 结束当前待办：此时正文已由续行补全，才能做去空白全等的重复判定
  const finalize = () => {
    if (!current) return
    const todo = current
    current = null

    const text = todo.text.trim()
    if (!text) return

    const key = dedupeKey(text)
    if (!key) return
    const duplicate = existingSet.has(key) || seenInFile.has(key)
    // 重复条目也加入 seen：同一文件内第三条相同项同样应被标为重复
    seenInFile.add(key)
    entries.push({
      text,
      tags: currentTags(todo),
      createdAt: todo.createdAt,
      duplicate
    })
  }

  for (const rawLine of String(raw ?? '').split('\n')) {
    const line = rawLine.replace(/\r$/, '')

    if (!line.trim()) {
      finalize()
      continue
    }

    const top = TOP_ITEM_RE.exec(line)
    if (top) {
      finalize()

      let content = top[1]
      if (CHECKBOX_DONE_RE.test(content)) continue

      let isTodo = false
      if (CHECKBOX_TODO_RE.test(content)) {
        isTodo = true
        // 只去掉复选框后的一个分隔空白：再多就是正文/标签自身的缩进
        content = content.replace(CHECKBOX_TODO_RE, '').replace(/^[ \t]/, '')
      }
      if (DONE_MARK_RE.test(content)) continue

      const time = matchLeadingTime(content)
      if (time) {
        isTodo = true
        content = time.rest
      }
      if (!isTodo) continue

      const parsed = extractTrailingTags(content)
      if (!parsed.text) continue

      current = {
        text: parsed.text,
        tags: parsed.tags,
        createdAt: resolveCreatedAt(now, fileDate, time)
      }
      continue
    }

    const continuation = CONTINUATION_RE.exec(line)
    if (continuation) {
      // 无宿主的续行（文件开头就缩进）静默跳过
      if (current) current.text += '\n' + continuation[1]
      continue
    }

    // 标题、普通段落等：结束当前待办并跳过
    finalize()
  }
  finalize()

  const duplicateCount = entries.filter(entry => entry.duplicate).length
  return {
    entries,
    importableCount: entries.length - duplicateCount,
    duplicateCount
  }
}

// currentTags 取值兜底：理论上创建时必为数组，防御 undefined 进入渲染层
function currentTags(current) {
  return Array.isArray(current.tags) ? current.tags : []
}

// dedupeKey 重复判定键：删除所有空白字符（含全角空格）后全等
function dedupeKey(text) {
  return String(text ?? '').replace(/\s+/g, '')
}

// matchLeadingTime 提取行首合法 HH:MM 时间，返回剩余内容
function matchLeadingTime(content) {
  const m = TIME_PREFIX_RE.exec(content)
  if (!m) return null
  return {
    hour: Number(m[1]),
    minute: Number(m[2]),
    rest: content.slice(m[0].length)
  }
}

// extractTrailingTags 从行尾反复剥离 #标签，正文中间的 # 保留（如 issue 号）
function extractTrailingTags(content) {
  const tags = []
  let text = content
  let m = TRAILING_TAG_RE.exec(text)
  while (m) {
    tags.unshift(m[1])
    text = text.slice(0, m.index)
    m = TRAILING_TAG_RE.exec(text)
  }
  return { text: text.trim(), tags }
}

// resolveFileDate 从文件名提取日期；非法日期（如 2026-02-30）视为无日期
function resolveFileDate(fileName) {
  const base = String(fileName).split(/[\\/]/).pop() || ''
  const m = FILE_DATE_RE.exec(base)
  if (!m) return null
  const year = Number(m[1])
  const month = Number(m[2])
  const day = Number(m[3])
  const date = new Date(year, month - 1, day)
  if (
    date.getFullYear() !== year ||
    date.getMonth() !== month - 1 ||
    date.getDate() !== day
  ) {
    return null
  }
  return date
}

// resolveCreatedAt 行内时间 + 日期 → 时间戳；晚于当前时刻钳到当前时刻
function resolveCreatedAt(now, fileDate, time) {
  if (!time) return 0 // 0 = 由后端填导入时刻
  const base = fileDate || new Date(now)
  const ts = new Date(
    base.getFullYear(),
    base.getMonth(),
    base.getDate(),
    time.hour,
    time.minute
  ).getTime()
  return ts > now ? now : ts
}
