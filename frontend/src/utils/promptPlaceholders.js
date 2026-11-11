/**
 * 提示词占位符解析与替换（{{名称}}）。
 *
 * 语法与本仓 AI 功能模板（Go 侧 renderPrompt / BuildStagePrompt）一致，用户只需要
 * 记一套写法。识别规则取宽松单行口径：
 * - {{ 与 }} 之间去掉首尾空白后必须非空；
 * - 名字不允许包含花括号或换行（避免正文里贴 JSON / 代码块时跨段误判）；
 * - 名字长度不超过 30 个字符（按 Unicode 码点计，中文名同样适用）。
 *
 * 不满足上述规则的片段（{{}}、{{   }}、跨行花括号、超长名字）一律按普通文本处理，
 * 既不触发复制弹窗，也不会在替换时被改写。
 */

/** 占位符名字长度上限（Unicode 码点数） */
export const PLACEHOLDER_NAME_MAX_LENGTH = 30

// 全局正则：仅被 matchAll / replace 消费——matchAll 使用克隆副本，replace 自带
// 复位，均不会残留 lastIndex。若将来新增 exec/test 调用，必须先手动复位 lastIndex。
const PLACEHOLDER_PATTERN = /\{\{([^{}\r\n]+)\}\}/g

/**
 * 归一占位符名字：去首尾空白，空名或超长名返回空串（视为非占位符）。
 * @param {string} raw
 * @returns {string}
 */
function normalizeName(raw) {
  const name = String(raw == null ? '' : raw).trim()
  if (!name) return ''
  if (Array.from(name).length > PLACEHOLDER_NAME_MAX_LENGTH) return ''
  return name
}

/**
 * 提取正文中的占位符名字，按首次出现顺序去重。
 * @param {string} text
 * @returns {string[]}
 */
export function extractPlaceholders(text) {
  if (typeof text !== 'string' || !text) return []

  const names = []
  const seen = new Set()
  for (const match of text.matchAll(PLACEHOLDER_PATTERN)) {
    const name = normalizeName(match[1])
    if (!name || seen.has(name)) continue
    seen.add(name)
    names.push(name)
  }
  return names
}

/**
 * 用录入值替换正文中的占位符。
 *
 * 未填写（undefined / null / 空串）的占位符保留 {{名称}} 原样：漏填在结果里
 * 一眼可见，且与「完全不填直接复制 = 原文」的语义一致。
 * 同一名字的多次出现全部替换。
 *
 * @param {string} text
 * @param {Record<string, string>} [values]
 * @returns {string}
 */
export function applyPlaceholders(text, values) {
  if (typeof text !== 'string') return ''
  if (!text) return ''

  const source = values && typeof values === 'object' ? values : {}
  return text.replace(PLACEHOLDER_PATTERN, (whole, rawName) => {
    const name = normalizeName(rawName)
    if (!name) return whole

    const value = source[name]
    if (value == null || value === '') return whole
    return String(value)
  })
}
