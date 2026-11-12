/**
 * 提示词占位符解析与替换（{{名称}} / {{名称:默认值}}）。
 *
 * 语法与本仓 AI 功能模板（Go 侧 renderPrompt / BuildStagePrompt）一致，用户只需要
 * 记一套写法。识别规则取宽松单行口径：
 * - {{ 与 }} 之间去掉首尾空白后必须非空；
 * - 名称与默认值均不允许包含花括号或换行（避免正文里贴 JSON / 代码块时跨段误判）；
 * - 名称长度不超过 30 个字符（按 Unicode 码点计，中文名同样适用）；
 * - 默认值长度不超过 200 个字符；默认值为空（{{页数:}}）视为无默认值。
 *
 * 名称与默认值以首个半角冒号分隔，故 {{时间:10:30}} 的默认值是 10:30。
 * 不满足上述规则的片段（{{}}、{{   }}、跨行花括号、超长名称 / 默认值）一律按普通
 * 文本处理，既不触发复制弹窗，也不会在替换时被改写。
 */

/** 占位符名称长度上限（Unicode 码点数） */
export const PLACEHOLDER_NAME_MAX_LENGTH = 30

/** 占位符默认值长度上限（Unicode 码点数） */
export const PLACEHOLDER_DEFAULT_MAX_LENGTH = 200

/** 名称与默认值的分隔符（仅半角冒号） */
export const PLACEHOLDER_SEPARATOR = ':'

// 全局正则：仅被 matchAll 消费——matchAll 内部克隆正则副本（不写回原正则的
// lastIndex），调用后不残留状态。若将来新增 exec / test 调用，必须先手动复位 lastIndex。
const PLACEHOLDER_PATTERN = /\{\{([^{}\r\n]+)\}\}/g

/**
 * 解析单个占位符内容（{{ 与 }} 之间的原文）为名称与默认值。
 *
 * 返回值一定带 defaultValue 字段（无冒号或默认值为空白时为空串），调用方据此判断
 * 是否存在可用默认值。解析失败返回 null，调用方须把整段按普通文本处理。
 *
 * @param {string} raw
 * @returns {{ name: string, defaultValue: string } | null}
 */
export function parsePlaceholder(raw) {
  const text = String(raw == null ? '' : raw)
  const sepIndex = text.indexOf(PLACEHOLDER_SEPARATOR)
  const rawName = sepIndex >= 0 ? text.slice(0, sepIndex) : text
  const rawDefault = sepIndex >= 0 ? text.slice(sepIndex + 1) : ''

  const name = rawName.trim()
  if (!name) return null
  if (Array.from(name).length > PLACEHOLDER_NAME_MAX_LENGTH) return null

  const defaultValue = rawDefault.trim()
  if (Array.from(defaultValue).length > PLACEHOLDER_DEFAULT_MAX_LENGTH) return null

  return { name, defaultValue }
}

/**
 * 提取正文中的占位符定义，按名称首次出现顺序去重。
 *
 * 同名占位符只保留首次出现的默认值定义——两处写了不同默认值时，替换阶段以首次
 * 出现的为准，保证「同一名字在所有位置得到相同结果」。
 *
 * @param {string} text
 * @returns {Array<{ name: string, defaultValue: string }>}
 */
export function extractPlaceholderDefs(text) {
  if (typeof text !== 'string' || !text) return []

  const defs = []
  const seen = new Set()
  for (const match of text.matchAll(PLACEHOLDER_PATTERN)) {
    const def = parsePlaceholder(match[1])
    if (!def || seen.has(def.name)) continue
    seen.add(def.name)
    defs.push(def)
  }
  return defs
}

/**
 * 提取正文中的占位符名字，按首次出现顺序去重。
 * @param {string} text
 * @returns {string[]}
 */
export function extractPlaceholders(text) {
  return extractPlaceholderDefs(text).map(def => def.name)
}

/**
 * 把正文拆成替换预览用的分段序列。
 *
 * 每段 `{ kind, text }`：
 * - `text`：普通文本（含无法识别的 {{...}} 原文）；
 * - `filled`：被用户填写值替换的片段（text 为填写值原文）；
 * - `default`：被默认值替换的片段（text 为默认值）；
 * - `unfilled`：未填且无默认值、保留原样的占位片段。
 *
 * 分段语义与 `applyPlaceholders` 的三级回退严格一致：填写值 trim 后非空 →
 * 默认值非空（取同名首次出现者）→ 原样保留。预览与复制共用本函数，避免两套
 * 实现漂移。
 *
 * @param {string} text
 * @param {Record<string, string>} [values]
 * @returns {Array<{ kind: 'text' | 'filled' | 'default' | 'unfilled', text: string }>}
 */
export function segmentPlaceholders(text, values) {
  if (typeof text !== 'string' || !text) return []

  const source = values && typeof values === 'object' ? values : {}
  const defaults = new Map(extractPlaceholderDefs(text).map(def => [def.name, def.defaultValue]))
  const segments = []
  let lastIndex = 0

  for (const match of text.matchAll(PLACEHOLDER_PATTERN)) {
    const whole = match[0]
    const index = match.index
    if (index > lastIndex) {
      segments.push({ kind: 'text', text: text.slice(lastIndex, index) })
    }

    const def = parsePlaceholder(match[1])
    if (!def) {
      segments.push({ kind: 'text', text: whole })
    } else {
      const value = source[def.name]
      if (value != null && String(value).trim() !== '') {
        segments.push({ kind: 'filled', text: String(value) })
      } else {
        const defaultValue = defaults.get(def.name) || ''
        if (defaultValue) {
          segments.push({ kind: 'default', text: defaultValue })
        } else {
          segments.push({ kind: 'unfilled', text: whole })
        }
      }
    }

    lastIndex = index + whole.length
  }

  if (lastIndex < text.length) {
    segments.push({ kind: 'text', text: text.slice(lastIndex) })
  }
  return segments
}

/**
 * 用录入值替换正文中的占位符。
 *
 * 三级回退（与 Go 侧 renderPrompt 一致）：
 * 1. 填写值（去除首尾空白后非空，输出保留用户原内容）——直接替换；
 * 2. 默认值（非空）——替换为默认值；
 * 3. 保留整段原样（含 {{名称}} 与 {{名称:默认值}} 的原始写法）。
 *
 * 同一名字的多次出现全部替换，默认值取该名字首次出现的定义。
 * 实现委托 `segmentPlaceholders`：预览与复制共用同一分段单点，结果恒等于
 * 各分段文本拼接。
 *
 * @param {string} text
 * @param {Record<string, string>} [values]
 * @returns {string}
 */
export function applyPlaceholders(text, values) {
  return segmentPlaceholders(text, values).map(segment => segment.text).join('')
}
