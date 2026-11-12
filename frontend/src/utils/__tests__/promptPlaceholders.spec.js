import { describe, expect, it } from 'vitest'
import {
  PLACEHOLDER_DEFAULT_MAX_LENGTH,
  PLACEHOLDER_NAME_MAX_LENGTH,
  applyPlaceholders,
  extractPlaceholderDefs,
  extractPlaceholders,
  parsePlaceholder,
  segmentPlaceholders
} from '../promptPlaceholders'

describe('extractPlaceholders', () => {
  it('按首次出现顺序提取并去重', () => {
    expect(extractPlaceholders('{{甲}} 与 {{乙}}，再来一次 {{甲}}')).toEqual(['甲', '乙'])
  })

  it('名字允许中文、英文、数字、下划线与连字符、空格', () => {
    expect(extractPlaceholders('{{客户名称}} {{user_name}} {{order-1}} {{ a b }}'))
      .toEqual(['客户名称', 'user_name', 'order-1', 'a b'])
  })

  it('去掉名字首尾空白后去重', () => {
    expect(extractPlaceholders('{{ 名称 }} 和 {{名称}}')).toEqual(['名称'])
  })

  it('空名、纯空白、跨行、超长名字都不算占位符', () => {
    const tooLong = '名'.repeat(PLACEHOLDER_NAME_MAX_LENGTH + 1)
    expect(extractPlaceholders('{{}} {{   }} {{a\nb}} {{' + tooLong + '}}')).toEqual([])
  })

  it('恰好达到长度上限的名字仍被识别', () => {
    const name = '名'.repeat(PLACEHOLDER_NAME_MAX_LENGTH)
    expect(extractPlaceholders('{{' + name + '}}')).toEqual([name])
  })

  it('非字符串输入返回空数组', () => {
    expect(extractPlaceholders(null)).toEqual([])
    expect(extractPlaceholders(undefined)).toEqual([])
    expect(extractPlaceholders('')).toEqual([])
  })
})

describe('parsePlaceholder', () => {
  it('无冒号时默认值为空', () => {
    expect(parsePlaceholder('名称')).toEqual({ name: '名称', defaultValue: '' })
  })

  it('以首个半角冒号分隔，默认值可含冒号', () => {
    expect(parsePlaceholder('时间:10:30')).toEqual({ name: '时间', defaultValue: '10:30' })
  })

  it('名称与默认值都去首尾空白', () => {
    expect(parsePlaceholder(' 页数 : 1 ')).toEqual({ name: '页数', defaultValue: '1' })
    expect(parsePlaceholder('页数:  ')).toEqual({ name: '页数', defaultValue: '' })
  })

  it('名称为空、名称超长、默认值超长返回 null', () => {
    expect(parsePlaceholder(':1')).toBeNull()
    expect(parsePlaceholder('   ')).toBeNull()
    expect(parsePlaceholder('名'.repeat(PLACEHOLDER_NAME_MAX_LENGTH + 1) + ':1')).toBeNull()
    expect(parsePlaceholder('页数:' + '值'.repeat(PLACEHOLDER_DEFAULT_MAX_LENGTH + 1))).toBeNull()
  })

  it('恰好达到长度上限仍有效', () => {
    const name = '名'.repeat(PLACEHOLDER_NAME_MAX_LENGTH)
    const defaultValue = '值'.repeat(PLACEHOLDER_DEFAULT_MAX_LENGTH)
    expect(parsePlaceholder(`${name}:${defaultValue}`)).toEqual({ name, defaultValue })
  })
})

describe('extractPlaceholderDefs', () => {
  it('按首次出现顺序去重并带默认值', () => {
    expect(extractPlaceholderDefs('{{甲:1}} {{乙}} {{甲:9}}'))
      .toEqual([
        { name: '甲', defaultValue: '1' },
        { name: '乙', defaultValue: '' }
      ])
  })

  it('非法默认值整段按普通文本（不出现在定义里）', () => {
    const tooLong = '值'.repeat(PLACEHOLDER_DEFAULT_MAX_LENGTH + 1)
    expect(extractPlaceholderDefs(`{{页数:${tooLong}} {{名称}}`)).toEqual([
      { name: '名称', defaultValue: '' }
    ])
  })

  it('非字符串输入返回空数组', () => {
    expect(extractPlaceholderDefs(null)).toEqual([])
    expect(extractPlaceholderDefs('')).toEqual([])
  })
})

describe('applyPlaceholders', () => {
  it('同名占位符全部替换', () => {
    expect(applyPlaceholders('{{名}} 和 {{名}}', { 名: 'X' })).toBe('X 和 X')
  })

  it('未填写的占位符保留原样', () => {
    expect(applyPlaceholders('{{甲}} {{乙}}', { 甲: 'A' })).toBe('A {{乙}}')
    expect(applyPlaceholders('{{甲}}', { 甲: '' })).toBe('{{甲}}')
    expect(applyPlaceholders('{{甲}}', { 甲: null })).toBe('{{甲}}')
  })

  // 默认值语义用例组：与 Go service/ai_function_test.go 的 TestRenderPrompt_DefaultValue
  // 保持同一组「名称 / 默认值 / 期望输出」，防跨语言实现漂移。
  it('未填时用默认值替换', () => {
    expect(applyPlaceholders('页数{{页数:1}}', {})).toBe('页数1')
  })

  it('填写值覆盖默认值', () => {
    expect(applyPlaceholders('页数{{页数:1}}', { 页数: '3' })).toBe('页数3')
  })

  it('空默认值（{{页数:}}）未填时保留整段原样', () => {
    expect(applyPlaceholders('{{页数:}}', {})).toBe('{{页数:}}')
  })

  it('默认值可含冒号', () => {
    expect(applyPlaceholders('{{时间:10:30}}', {})).toBe('10:30')
  })

  it('同名多处不同默认值以首次出现为准', () => {
    expect(applyPlaceholders('{{页数:1}} … {{页数:5}}', {})).toBe('1 … 1')
  })

  it('默认值去首尾空白', () => {
    expect(applyPlaceholders('{{页数: 1 }}', {})).toBe('1')
  })

  it('首次定义为空默认值时，后续同名默认值不生效（两处均保留原文）', () => {
    expect(applyPlaceholders('{{页数:}} … {{页数:5}}', {})).toBe('{{页数:}} … {{页数:5}}')
    expect(extractPlaceholderDefs('{{页数:}} … {{页数:5}}'))
      .toEqual([{ name: '页数', defaultValue: '' }])
  })

  it('默认值跨行不识别为占位符，按普通文本保留', () => {
    expect(applyPlaceholders('{{页数:1\n2}}', {})).toBe('{{页数:1\n2}}')
    expect(extractPlaceholderDefs('{{页数:1\n2}}')).toEqual([])
    expect(extractPlaceholders('{{页数:1\n2}}')).toEqual([])
  })

  it('超长名称或默认值不识别为占位符', () => {
    const tooLongName = '名'.repeat(PLACEHOLDER_NAME_MAX_LENGTH + 1)
    const tooLongDefault = '值'.repeat(PLACEHOLDER_DEFAULT_MAX_LENGTH + 1)
    expect(applyPlaceholders(`{{${tooLongName}:1}}`, {})).toBe(`{{${tooLongName}:1}}`)
    expect(applyPlaceholders(`{{页数:${tooLongDefault}}}`, {})).toBe(`{{页数:${tooLongDefault}}}`)
  })

  it('纯空白填写视为未填，走默认值', () => {
    expect(applyPlaceholders('{{页数:1}}', { 页数: '   ' })).toBe('1')
  })

  it('填写值保留用户原内容（仅判空时 trim）', () => {
    expect(applyPlaceholders('{{名称:默认}}', { 名称: ' 张三 ' })).toBe(' 张三 ')
  })

  it('不带花括号差异地匹配原文（含空格的写法按归一名字取值）', () => {
    expect(applyPlaceholders('{{ 名称 }}', { 名称: '张三' })).toBe('张三')
  })

  it('非法占位符原样保留', () => {
    expect(applyPlaceholders('{{}} {{   }} {{a\nb}}', { '': 'X' })).toBe('{{}} {{   }} {{a\nb}}')
  })

  it('值会被转成字符串（0 不算未填写）', () => {
    expect(applyPlaceholders('{{n}}', { n: 0 })).toBe('0')
  })

  it('空正文与非字符串输入安全返回', () => {
    expect(applyPlaceholders('', { a: 'b' })).toBe('')
    expect(applyPlaceholders(null, { a: 'b' })).toBe('')
  })
})

describe('segmentPlaceholders', () => {
  it('普通文本返回单个 text 片段', () => {
    expect(segmentPlaceholders('普通文本', {})).toEqual([
      { kind: 'text', text: '普通文本' }
    ])
  })

  it('填写值替换为 filled 片段', () => {
    expect(segmentPlaceholders('你好 {{名称}}', { 名称: '张三' })).toEqual([
      { kind: 'text', text: '你好 ' },
      { kind: 'filled', text: '张三' }
    ])
  })

  it('未填时使用默认值生成 default 片段', () => {
    expect(segmentPlaceholders('页数 {{页数:1}}', {})).toEqual([
      { kind: 'text', text: '页数 ' },
      { kind: 'default', text: '1' }
    ])
  })

  it('未填且无默认值生成 unfilled 片段并保留原文', () => {
    expect(segmentPlaceholders('{{客户名称}}', {})).toEqual([
      { kind: 'unfilled', text: '{{客户名称}}' }
    ])
    expect(segmentPlaceholders('{{ 名称 }}', {})).toEqual([
      { kind: 'unfilled', text: '{{ 名称 }}' }
    ])
  })

  it('混合多段按原文顺序拆分', () => {
    expect(segmentPlaceholders('称呼 {{客户名称}}，页数 {{页数:1}}，时间 {{时间}}', { 客户名称: '张三' }))
      .toEqual([
        { kind: 'text', text: '称呼 ' },
        { kind: 'filled', text: '张三' },
        { kind: 'text', text: '，页数 ' },
        { kind: 'default', text: '1' },
        { kind: 'text', text: '，时间 ' },
        { kind: 'unfilled', text: '{{时间}}' }
      ])
  })

  it('保留换行且相邻占位符之间不产生空片段', () => {
    expect(segmentPlaceholders('第一行\n{{甲}}{{乙}}', { 甲: 'A' })).toEqual([
      { kind: 'text', text: '第一行\n' },
      { kind: 'filled', text: 'A' },
      { kind: 'unfilled', text: '{{乙}}' }
    ])
  })

  it('无效占位符按 text 片段保留原文', () => {
    expect(segmentPlaceholders('{{}} 与 {{a\nb}}', { '': 'X' })).toEqual([
      { kind: 'text', text: '{{}} 与 {{a\nb}}' }
    ])
  })

  it('纯空白填写视为未填，走默认值', () => {
    expect(segmentPlaceholders('{{页数:1}}', { 页数: '   ' })).toEqual([
      { kind: 'default', text: '1' }
    ])
  })

  it('非字符串值转成字符串，0 不算未填写', () => {
    expect(segmentPlaceholders('{{n}}', { n: 0 })).toEqual([
      { kind: 'filled', text: '0' }
    ])
  })

  it('applyPlaceholders 恒等于分段文本拼接', () => {
    const text = '称呼 {{客户名称}}，页数 {{页数:1}}，时间 {{时间}}'
    const cases = [{}, { 客户名称: '张三' }, { 页数: '3', 时间: '10:30' }]
    for (const values of cases) {
      expect(applyPlaceholders(text, values))
        .toBe(segmentPlaceholders(text, values).map(segment => segment.text).join(''))
    }
  })

  it('空正文与非字符串输入返回空数组', () => {
    expect(segmentPlaceholders('', {})).toEqual([])
    expect(segmentPlaceholders(null, {})).toEqual([])
  })
})
