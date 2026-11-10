import { describe, it, expect } from 'vitest'
import { parseTempTodoMarkdown } from '../tempTodoImport'

// 固定导入时刻，时间断言不依赖真实时钟
const NOW = new Date(2026, 10, 10, 12, 0, 0, 0).getTime() // 2026-11-10 12:00 本地

const localTs = (year, month, day, hour, minute) =>
  new Date(year, month - 1, day, hour, minute).getTime()

describe('utils/tempTodoImport', () => {
  it('只导入两类待办行，标题/段落/已完成/普通列表静默跳过', () => {
    const raw = [
      '# 2026-11-10',
      '',
      '今天做了不少事。',
      '- [x] 已完成的事',
      '- [ ] 未完成的事',
      '- 09:30 带标签的事  #重构 #AI提示词',
      '- 普通列表项',
      '- 15:00 已完成导出  #标签（已完成 15:30）',
      ''
    ].join('\n')

    const result = parseTempTodoMarkdown(raw, { now: NOW })

    expect(result.entries.map(entry => entry.text)).toEqual([
      '未完成的事',
      '带标签的事'
    ])
    expect(result.entries[1].tags).toEqual(['重构', 'AI提示词'])
    expect(result.entries[0].createdAt).toBe(0)
    expect(result.entries[1].createdAt).toBe(localTs(2026, 11, 10, 9, 30))
    expect(result.importableCount).toBe(2)
    expect(result.duplicateCount).toBe(0)
  })

  it('复选框待办行可带时间前缀', () => {
    const result = parseTempTodoMarkdown('- [ ] 10:15 带时间的复选框 #标签', { now: NOW })

    expect(result.entries).toHaveLength(1)
    expect(result.entries[0]).toMatchObject({
      text: '带时间的复选框',
      tags: ['标签'],
      createdAt: localTs(2026, 11, 10, 10, 15)
    })
  })

  it('两空格缩进续行拼回同一条待办，空行结束当前条目', () => {
    const raw = [
      '- 09:30 第一行',
      '  第二行',
      '    第四层缩进保留',
      '',
      '  游离续行无宿主',
      '- 10:00 下一条'
    ].join('\n')

    const result = parseTempTodoMarkdown(raw, { now: NOW })

    expect(result.entries).toHaveLength(2)
    expect(result.entries[0].text).toBe('第一行\n第二行\n  第四层缩进保留')
    expect(result.entries[1].text).toBe('下一条')
  })

  it('行尾连续标签全部提取，正文中间的 # 保留', () => {
    const raw = '- 11:00 修复 #123 号问题  #bug #前端'

    const [entry] = parseTempTodoMarkdown(raw, { now: NOW }).entries

    expect(entry.text).toBe('修复 #123 号问题')
    expect(entry.tags).toEqual(['bug', '前端'])
  })

  it('文件名日期优先于导入当天', () => {
    const text = '- 08:30 历史条目\n- [ ] 无时间条目'
    const result = parseTempTodoMarkdown(text, {
      now: NOW,
      fileName: 'D:/notes/2026-11-09.md'
    })

    expect(result.entries[0].createdAt).toBe(localTs(2026, 11, 9, 8, 30))
    expect(result.entries[1].createdAt).toBe(0)
  })

  it('文件名不是合法日期时回退导入当天', () => {
    const result = parseTempTodoMarkdown('- 08:30 条目', {
      now: NOW,
      fileName: '2026-02-30.md'
    })

    expect(result.entries[0].createdAt).toBe(localTs(2026, 11, 10, 8, 30))
  })

  it('组合时间晚于当前时刻时钳到当前时刻', () => {
    const sameDayLate = parseTempTodoMarkdown('- 23:00 今晚', { now: NOW })
    expect(sameDayLate.entries[0].createdAt).toBe(NOW)

    const futureFile = parseTempTodoMarkdown('- 08:30 明天', {
      now: NOW,
      fileName: '2026-11-11.md'
    })
    expect(futureFile.entries[0].createdAt).toBe(NOW)
  })

  it('与现有条目重复时标记 duplicate，去空白后全等即命中', () => {
    const result = parseTempTodoMarkdown('- [ ] 已存在 的  事\n- [ ] 全新的事', {
      now: NOW,
      existingTexts: ['已存在的事']
    })

    expect(result.entries.map(entry => entry.duplicate)).toEqual([true, false])
    expect(result.importableCount).toBe(1)
    expect(result.duplicateCount).toBe(1)
  })

  it('文件内部自身重复全部标记，保留第一条', () => {
    const raw = ['- [ ] 重复条目', '- [ ] 重复 条目', '- [ ] 重复条目'].join('\n')

    const result = parseTempTodoMarkdown(raw, { now: NOW })

    expect(result.entries.map(entry => entry.duplicate)).toEqual([false, true, true])
    expect(result.importableCount).toBe(1)
    expect(result.duplicateCount).toBe(2)
  })

  it('CRLF 与空输入不抛错', () => {
    expect(parseTempTodoMarkdown('', { now: NOW }).entries).toEqual([])
    expect(parseTempTodoMarkdown('   \n\n  ', { now: NOW }).entries).toEqual([])

    const crlf = parseTempTodoMarkdown('- 09:30 第一行\r\n  第二行\r\n', { now: NOW })
    expect(crlf.entries).toHaveLength(1)
    expect(crlf.entries[0].text).toBe('第一行\n第二行')
  })

  it('只有标签没有正文的待办行跳过', () => {
    const result = parseTempTodoMarkdown('- [ ]   #只有标签\n- 09:30', { now: NOW })

    expect(result.entries).toEqual([])
  })
})
