import { describe, it, expect } from 'vitest'
import { filterTreeNodesByName } from '../treeFilter'

const makeNode = (name, type = 'file') => ({ name, path: `/p/${name}`, type })

describe('filterTreeNodesByName', () => {
  const nodes = [
    makeNode('index.js'),
    makeNode('README.md'),
    makeNode('src', 'directory'),
    makeNode('IndexHelper.go')
  ]

  it('子串命中：大小写不敏感', () => {
    const r = filterTreeNodesByName(nodes, 'INDEX')
    expect(r.map(n => n.name)).toEqual(['index.js', 'IndexHelper.go'])
  })

  it('目录与文件混合过滤，保持后端返回顺序', () => {
    const r = filterTreeNodesByName(nodes, 'src')
    expect(r).toHaveLength(1)
    expect(r[0].type).toBe('directory')
  })

  it('空关键词返回原列表（引用相同）', () => {
    expect(filterTreeNodesByName(nodes, '')).toBe(nodes)
    expect(filterTreeNodesByName(nodes, '   ')).toBe(nodes)
  })

  it('非数组输入兜底为空数组', () => {
    expect(filterTreeNodesByName(null, 'a')).toEqual([])
    expect(filterTreeNodesByName(undefined, 'a')).toEqual([])
  })

  it('无命中返回空数组', () => {
    expect(filterTreeNodesByName(nodes, 'nope')).toEqual([])
  })

  it('name 非字符串的脏数据节点跳过（不抛错）', () => {
    const dirty = [{ path: '/p/x' }, makeNode('abc.js')]
    expect(filterTreeNodesByName(dirty, 'abc')).toHaveLength(1)
  })

  it('截断场景语义：2050 项大目录按名筛出 2000 名外文件', () => {
    const big = Array.from({ length: 2050 }, (_, i) => makeNode(`chunk-${i}.dat`))
    // 前 2000 项在树中可见，chunk-2000.dat 及以后只能靠筛选定位（盲区场景）
    const r = filterTreeNodesByName(big, 'chunk-2049')
    expect(r).toHaveLength(1)
    expect(r[0].name).toBe('chunk-2049.dat')
  })
})
