/**
 * codemirrorHighlight 单测
 * 覆盖：主题选样式、深色样式与深色预览底色的对比度、关键 token（属性名/变量名）配色
 */
import { describe, it, expect } from 'vitest'
import { defaultHighlightStyle } from '@codemirror/language'
import { tags as t } from '@lezer/highlight'
import {
  codeHighlightStyleFor,
  darkCodeHighlightStyle
} from '../codemirrorHighlight'

// 深色模式预览底色（style.css html.dark --bg-secondary）
const DARK_BG = '#1e293b'

const parseHex = (hex) => {
  const h = hex.replace('#', '')
  return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255)
}

const relativeLuminance = (hex) => {
  const [r, g, b] = parseHex(hex).map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

const contrastRatio = (foreground, background) => {
  const a = relativeLuminance(foreground)
  const b = relativeLuminance(background)
  const [light, dark] = a > b ? [a, b] : [b, a]
  return (light + 0.05) / (dark + 0.05)
}

// 找出覆盖指定 tag 的样式颜色
const colorOfTag = (tag) => {
  const spec = darkCodeHighlightStyle.specs.find((s) =>
    Array.isArray(s.tag) ? s.tag.includes(tag) : s.tag === tag
  )
  return spec && spec.color
}

describe('codemirrorHighlight', () => {
  it('浅色主题沿用 CodeMirror 内置 defaultHighlightStyle（既有观感不变）', () => {
    expect(codeHighlightStyleFor('light')).toBe(defaultHighlightStyle)
  })

  it('深色主题使用自定义暗色样式', () => {
    expect(codeHighlightStyleFor('dark')).toBe(darkCodeHighlightStyle)
  })

  it('深色样式每个声明色与深色预览底色对比度不低于 3:1', () => {
    for (const spec of darkCodeHighlightStyle.specs) {
      if (!spec.color) continue
      expect(contrastRatio(spec.color, DARK_BG)).toBeGreaterThanOrEqual(3)
    }
  })

  it('属性名与变量名使用浅色高亮色，不再回落到深蓝色板', () => {
    expect(colorOfTag(t.propertyName)).toBe('#9cdcfe')
    expect(colorOfTag(t.variableName)).toBe('#9cdcfe')
    expect(colorOfTag(t.attributeName)).toBe('#9cdcfe')
  })

  it('字符串与关键字配色与 hljs 暗色主题一致', () => {
    expect(colorOfTag(t.string)).toBe('#ce9178')
    expect(colorOfTag(t.keyword)).toBe('#569cd6')
    expect(colorOfTag(t.comment)).toBe('#6a9955')
  })

  it('类名与内建类型有显式配色，不落到默认正文灰', () => {
    expect(colorOfTag(t.className)).toBe('#dcdcaa')
    expect(colorOfTag(t.typeName)).toBe('#ce9178')
    expect(colorOfTag(t.standard(t.typeName))).toBe('#ce9178')
  })
})
