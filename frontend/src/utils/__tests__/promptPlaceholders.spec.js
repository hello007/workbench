import { describe, expect, it } from 'vitest'
import {
  PLACEHOLDER_NAME_MAX_LENGTH,
  applyPlaceholders,
  extractPlaceholders
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

describe('applyPlaceholders', () => {
  it('同名占位符全部替换', () => {
    expect(applyPlaceholders('{{名}} 和 {{名}}', { 名: 'X' })).toBe('X 和 X')
  })

  it('未填写的占位符保留原样', () => {
    expect(applyPlaceholders('{{甲}} {{乙}}', { 甲: 'A' })).toBe('A {{乙}}')
    expect(applyPlaceholders('{{甲}}', { 甲: '' })).toBe('{{甲}}')
    expect(applyPlaceholders('{{甲}}', { 甲: null })).toBe('{{甲}}')
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
