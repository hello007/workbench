/**
 * 多行粘贴预览提取（纯函数）：终端多行粘贴确认框预览块内容构造。
 * 规则（任务 11-05-url R2）：取前 5 行、单行截 120 字符、总 400 字符上限；
 * 任一超限则 previewText 尾部附省略摘要行「…（共 N 行）」且 truncated 置 true。
 * 预览仅影响确认框展示，确认粘贴仍写入完整原文（截断不影响实际粘贴内容）。
 */

// 预览上限：行数 / 单行字符 / 总字符（固定规则，非设置项）
const MAX_LINES = 5
const MAX_LINE_CHARS = 120
const MAX_TOTAL_CHARS = 400

/**
 * 构建待粘贴文本的预览内容
 * @param {string} text - 剪贴板原始文本（调用方已保证非空且含换行）
 * @returns {{ previewText: string, truncated: boolean, totalLines: number }}
 *   previewText：预览块展示文本（截断时含省略摘要行）；truncated：是否发生截断；
 *   totalLines：总行数（按换行切分，末尾单个换行符不额外计一行）
 */
export function buildPastePreview(text) {
  const lines = text.split(/\r\n|\r|\n/)
  // 末尾换行符产生的空尾元素不计行（'a\n' 是一行内容 + 终止符，不是两行）
  if (lines.length > 1 && lines[lines.length - 1] === '') {
    lines.pop()
  }
  const totalLines = lines.length
  const overLines = totalLines > MAX_LINES
  const lineTruncated = lines.some((line) => line.length > MAX_LINE_CHARS)

  const shown = lines
    .slice(0, MAX_LINES)
    .map((line) => (line.length > MAX_LINE_CHARS ? line.slice(0, MAX_LINE_CHARS) + '…' : line))
  let body = shown.join('\n')

  // 总字符上限兜底：单行截断后 5 行 × 121 字符仍可能超 400
  const overTotal = body.length > MAX_TOTAL_CHARS
  if (overTotal) {
    body = body.slice(0, MAX_TOTAL_CHARS) + '…'
  }

  const truncated = overLines || lineTruncated || overTotal
  const previewText = truncated ? `${body}\n…（共 ${totalLines} 行）` : body
  return { previewText, truncated, totalLines }
}
