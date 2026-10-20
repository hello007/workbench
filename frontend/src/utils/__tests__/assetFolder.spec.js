import { describe, it, expect } from 'vitest'
import { folderPrefixMatch, buildFolderCountMap } from '../assetFolder'

// 文件夹树共享单点（PRD PR1）：前缀匹配边界与徽标递归计数聚合。
// 路径口径 = 相对库根目录正斜杠路径，folderPath 空串 = 库根「全部素材」。

describe('folderPrefixMatch', () => {
  it('空 folderPath（库根）恒匹配任意条目', () => {
    expect(folderPrefixMatch('a/x.pptx', '')).toBe(true)
    expect(folderPrefixMatch('x.pptx', '')).toBe(true)
  })

  it('命中目录自身与其任意深度子目录内文件', () => {
    expect(folderPrefixMatch('a', 'a')).toBe(true)
    expect(folderPrefixMatch('a/x.pptx', 'a')).toBe(true)
    expect(folderPrefixMatch('a/b/x.pptx', 'a')).toBe(true)
  })

  it('前缀边界：a 不得误匹配 ab 开头的兄弟目录（slash 边界）', () => {
    expect(folderPrefixMatch('ab/x.pptx', 'a')).toBe(false)
    expect(folderPrefixMatch('ab', 'a')).toBe(false)
    expect(folderPrefixMatch('ab/c/x.pptx', 'a')).toBe(false)
    // 反向同构：ab 不得误匹配 a 开头段
    expect(folderPrefixMatch('a/x.pptx', 'ab')).toBe(false)
  })

  it('不同目录互不匹配', () => {
    expect(folderPrefixMatch('b/x.pptx', 'a')).toBe(false)
  })
})

describe('buildFolderCountMap', () => {
  it('每个文件计入其全部祖先目录（含库根），递归口径', () => {
    const entries = [
      { path: 'a/x.pptx' },
      { path: 'a/b/y.pptx' },
      { path: 'z.pptx' } // 库根直置文件
    ]
    const map = buildFolderCountMap(entries)
    expect(map['']).toBe(3) // 根 = 全部
    expect(map['a']).toBe(2) // a/x + a/b/y
    expect(map['a/b']).toBe(1)
  })

  it('根直置文件只计根不计目录键', () => {
    const map = buildFolderCountMap([{ path: 'z.pptx' }])
    expect(map['']).toBe(1)
    expect(Object.keys(map).filter(k => k !== '').length).toBe(0)
  })

  it('空清单返回仅含根键的零计数', () => {
    expect(buildFolderCountMap([])).toEqual({ '': 0 })
    expect(buildFolderCountMap(null)).toEqual({ '': 0 })
  })

  it('计数为纯数字键值（FolderTree badgeOf 直接取值）', () => {
    const map = buildFolderCountMap([{ path: 'a/b/x.pptx' }, { path: 'a/b/y.pptx' }])
    expect(map['a/b']).toBe(2)
    expect(map['a']).toBe(2)
  })
})
