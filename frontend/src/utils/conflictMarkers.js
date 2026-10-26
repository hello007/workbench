/**
 * 冲突标记块解析（纯展示逻辑，前端单点）。
 *
 * 标记形态（行首列 0 匹配，与 git 语义一致）：
 *   `<<<<<<< label` 冲突块开始
 *   `||||||| label` diff3 base 段（merge.conflictStyle=diff3 时出现，解析但默认不采用）
 *   `=======`       ours/theirs 分隔
 *   `>>>>>>> label` 冲突块结束
 *
 * 输出模型：{ before, blocks: [{ours, base, theirs, resolved}], after, hasMarkers }
 * - before/after：冲突块外的上下文行
 * - resolved 初始 null（未决策），决策为 { kind: 'ours'|'theirs'|'both'|'custom', lines: string[] }
 * - base 段解析保留供展示，但默认不参与合并结果
 */

const MARKER_START = /^<{7} ?/
const MARKER_BASE = /^\|{7} ?/
// 允许行尾 \r：Windows CRLF 文件的分隔行是 `=======\r`，锚定 `$` 不吃 \r 会漏配
const MARKER_SEP = /^={7}\r?$/
const MARKER_END = /^>{7} ?/

/**
 * 解析文本中的冲突标记块。
 * 防御性处理：未闭合块到 EOF 视为块内容收尾；游离分隔符（无 `<<<<<<<` 包裹）
 * 按普通行处理。
 * @param {string} text 工作区文件全文（UTF-8 解码后）
 * @returns {{before: string[], blocks: Array<{ours: string[], base: string[], theirs: string[], resolved: null|{kind: string, lines: string[]}}>, after: string[], hasMarkers: boolean}}
 */
export function parseConflictMarkers(text) {
  const lines = (text || '').split('\n')
  const before = []
  const after = []
  const blocks = []

  // 当前块状态：null = 块外；否则记录收集段（ours/base/theirs）
  let current = null
  let section = null
  // 块间上下文：上一块结束后、下一块开始前的普通行（挂到下一块的 lead，保持合并顺序）
  let pending = []

  for (const line of lines) {
    if (!current && MARKER_START.test(line)) {
      // 进入冲突块；pending 中的块间上下文挂为本块的前导行
      current = { lead: pending, ours: [], base: [], theirs: [] }
      pending = []
      section = 'ours'
      continue
    }
    if (current) {
      if (MARKER_BASE.test(line)) {
        section = 'base'
        continue
      }
      if (MARKER_SEP.test(line)) {
        // ours/base 段结束，进入 theirs
        section = 'theirs'
        continue
      }
      if (MARKER_END.test(line)) {
        blocks.push({ ...current, resolved: null })
        current = null
        section = null
        continue
      }
      current[section].push(line)
      continue
    }
    // 块外：首块之前进 before；块间行先挂 pending（随下一块 lead 走）；末块之后进 after
    if (blocks.length === 0) {
      before.push(line)
    } else {
      pending.push(line)
    }
  }

  // 末块之后未消费的块间上下文归入 after
  after.push(...pending)

  // 防御：EOF 仍未闭合的块也作为块内容收尾
  if (current) {
    blocks.push({ ...current, resolved: null })
  }

  return { before, blocks, after, hasMarkers: blocks.length > 0 }
}

/**
 * 计算合并结果文本：before + 各块（已决策取决策行，未决策保留 ours+theirs 原样）+ after。
 * 未决策块保留原始两侧内容（含标记语义的两侧正文，不含标记行本身），
 * 使预览能清晰看出「仍有未解决块」。
 * @param {object} parsed parseConflictMarkers 的返回值
 * @returns {string} 合并结果全文
 */
export function mergeConflictResult(parsed) {
  const parts = [...parsed.before]
  for (const block of parsed.blocks) {
    // 块前导上下文（块间普通行）按原位保留
    if (block.lead) parts.push(...block.lead)
    if (block.resolved) {
      parts.push(...block.resolved.lines)
    } else {
      parts.push(...block.ours, ...block.theirs)
    }
  }
  parts.push(...parsed.after)
  return parts.join('\n')
}

/**
 * 保存前软校验：文本行首是否仍含冲突标记（`<<<<<<<` 或 `>>>>>>>`）。
 * @param {string} text
 * @returns {boolean}
 */
export function containsConflictMarkers(text) {
  const lines = (text || '').split('\n')
  return lines.some((l) => MARKER_START.test(l) || MARKER_END.test(l))
}
