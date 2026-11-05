/**
 * buildPastePreview 纯函数测试：多行粘贴确认框预览规则
 * 覆盖：前 5 行截断、单行 120 字符截断、总 400 字符截断、未超限原样、
 * truncated 标志、totalLines 计数（含尾行无换行符与末尾换行符场景）
 */
import { describe, it, expect } from 'vitest'
import { buildPastePreview } from '../pastePreview'

describe('buildPastePreview - 多行粘贴预览', () => {
  it('未超限原样返回：内容不变且 truncated=false', () => {
    const r = buildPastePreview('git status\nnpm test')
    expect(r.previewText).toBe('git status\nnpm test')
    expect(r.truncated).toBe(false)
    expect(r.totalLines).toBe(2)
  })

  it('尾行无换行符：totalLines 按内容行计数', () => {
    expect(buildPastePreview('l1\nl2\nl3').totalLines).toBe(3)
  })

  it('末尾换行符不额外计一行（"a\\n" 是一行内容 + 终止符）', () => {
    const r = buildPastePreview('only-one\n')
    expect(r.totalLines).toBe(1)
    expect(r.truncated).toBe(false)
    expect(r.previewText).toBe('only-one')
  })

  it('兼容 \\r\\n 与 \\r 换行切分', () => {
    expect(buildPastePreview('a\r\nb\rc').totalLines).toBe(3)
    expect(buildPastePreview('a\r\nb\rc').previewText).toBe('a\nb\nc')
  })

  it('超 5 行截断：保留前 5 行 + 尾部摘要行「…（共 N 行）」', () => {
    const r = buildPastePreview('l1\nl2\nl3\nl4\nl5\nl6\nl7')
    expect(r.truncated).toBe(true)
    expect(r.totalLines).toBe(7)
    expect(r.previewText).toContain('l1')
    expect(r.previewText).toContain('l5')
    expect(r.previewText).not.toContain('l6')
    expect(r.previewText.endsWith('\n…（共 7 行）')).toBe(true)
  })

  it('恰好 5 行不截断（行数上限边界）', () => {
    const r = buildPastePreview('l1\nl2\nl3\nl4\nl5')
    expect(r.truncated).toBe(false)
    expect(r.previewText).toBe('l1\nl2\nl3\nl4\nl5')
  })

  it('单行超 120 字符截断：120 字符 + 省略号，truncated=true', () => {
    const r = buildPastePreview('x'.repeat(130))
    expect(r.truncated).toBe(true)
    expect(r.totalLines).toBe(1)
    expect(r.previewText.startsWith('x'.repeat(120) + '…\n')).toBe(true)
    expect(r.previewText.endsWith('…（共 1 行）')).toBe(true)
  })

  it('恰好 120 字符不截断（单行上限边界）', () => {
    const r = buildPastePreview('y'.repeat(120))
    expect(r.truncated).toBe(false)
    expect(r.previewText).toBe('y'.repeat(120))
  })

  it('总 400 字符上限：单行截断后仍超总限时整体截到 400 + 省略号', () => {
    // 5 行 × 130 字符 + 第 6 行：单行各截为 121 字符后总长 605 > 400
    const text = Array.from({ length: 5 }, () => 'z'.repeat(130)).join('\n') + '\nlater'
    const r = buildPastePreview(text)
    expect(r.truncated).toBe(true)
    expect(r.totalLines).toBe(6)
    // 摘要行之前的内容部分 = 400 字符 + 省略号 = 401 字符
    const summaryIdx = r.previewText.indexOf('\n…（共')
    const body = r.previewText.slice(0, summaryIdx)
    expect(body.length).toBe(401)
    expect(body.startsWith('z'.repeat(120))).toBe(true)
    expect(body.endsWith('…')).toBe(true)
    expect(r.previewText.endsWith('…（共 6 行）')).toBe(true)
  })
})
