/**
 * skill 斜杠补全纯函数单测。
 *
 * 覆盖 extractSlashToken（行首/多行行首/非行首/空白失活/光标越界）、
 * applyCompletion（片段替换 + 尾随空格 + caret 落位）、filterSkills
 * （command/name/description 三字段命中、大小写不敏感、空 query 全量）。
 */
import { describe, it, expect } from 'vitest'
import { extractSlashToken, applyCompletion, filterSkills } from '../skillComplete'

describe('extractSlashToken', () => {
  it('行首 / 活跃：query 为 / 之后片段，end 为 token 终点', () => {
    // text = '/dra'，caret 在末尾
    expect(extractSlashToken('/dra', 4)).toEqual({ active: true, start: 0, end: 4, query: 'dra' })
  })

  it('空 query（只敲了 /）也活跃', () => {
    expect(extractSlashToken('/', 1)).toEqual({ active: true, start: 0, end: 1, query: '' })
  })

  it('token 后已有空格：光标在 token 内仍活跃（query 到空白为止）', () => {
    // '/dr 剩余'：token = '/dr'，end=3（空格处）
    expect(extractSlashToken('/dr 剩余', 2)).toEqual({ active: true, start: 0, end: 3, query: 'dr' })
  })

  it('光标越过空白（进入正文）失活', () => {
    expect(extractSlashToken('/dr 剩余', 5)).toEqual({ active: false, start: -1, end: -1, query: '' })
  })

  it('行中 /（非行首）不触发', () => {
    expect(extractSlashToken('看看 /dr', 7)).toEqual({ active: false, start: -1, end: -1, query: '' })
  })

  it('多行输入：第二行行首 / 活跃', () => {
    const text = '第一行\n/skill'
    // caret 在第二行末尾
    expect(extractSlashToken(text, text.length)).toEqual({
      active: true, start: 4, end: text.length, query: 'skill'
    })
  })

  it('多行输入：第二行非行首 / 不触发', () => {
    const text = '第一行\n写 /skill'
    expect(extractSlashToken(text, text.length)).toEqual({
      active: false, start: -1, end: -1, query: ''
    })
  })

  it('空文本 / 越界 caret / null 安全失活', () => {
    expect(extractSlashToken('', 0).active).toBe(false)
    expect(extractSlashToken('/a', 99).active).toBe(false)
    expect(extractSlashToken(null, 0).active).toBe(false)
  })
})

describe('applyCompletion', () => {
  it('片段整体替换为 command + 尾随空格，caret 落在空格后', () => {
    expect(applyCompletion('/dra', { start: 0, end: 4 }, '/drawio')).toEqual({
      text: '/drawio ', caret: 8
    })
  })

  it('token 后的正文保留不动', () => {
    expect(applyCompletion('/dr 帮我画图', { start: 0, end: 3 }, '/drawio')).toEqual({
      text: '/drawio 帮我画图', caret: 8
    })
  })

  it('多行场景：只替换所在行 token，后续行不动', () => {
    expect(applyCompletion('前言\n/sk 正文\n尾行', { start: 3, end: 6 }, '/skill-x')).toEqual({
      text: '前言\n/skill-x 正文\n尾行',
      caret: 12
    })
  })

  it('插件命名空间命令照常替换', () => {
    expect(applyCompletion('/ab ', { start: 0, end: 3 }, '/ab-office:agree-slides')).toEqual({
      text: '/ab-office:agree-slides ', caret: 24
    })
  })
})

describe('filterSkills', () => {
  const skills = [
    { command: '/drawio', name: 'drawio', description: '画图工具' },
    { command: '/ab-office:agree-slides', name: 'agree-slides', description: '生成幻灯片' },
    { command: '/release', name: 'release', description: '发版流程' }
  ]

  it('空 query 返回全量（保序拷贝）', () => {
    expect(filterSkills(skills, '')).toHaveLength(3)
    expect(filterSkills(skills, '')).not.toBe(skills)
  })

  it('command 命中', () => {
    expect(filterSkills(skills, 'dr')).toEqual([skills[0]])
  })

  it('description 命中', () => {
    expect(filterSkills(skills, '幻灯')).toEqual([skills[1]])
  })

  it('name 命中（插件命名空间内的裸名）', () => {
    expect(filterSkills(skills, 'agree')).toEqual([skills[1]])
  })

  it('大小写不敏感', () => {
    expect(filterSkills(skills, 'RELEASE')).toEqual([skills[2]])
  })

  it('无命中返回空数组', () => {
    expect(filterSkills(skills, 'zzz')).toEqual([])
  })
})
