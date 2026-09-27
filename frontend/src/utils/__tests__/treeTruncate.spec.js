import { describe, it, expect } from 'vitest'
import { truncateTreeNodes, FILE_TREE_NODE_LIMIT, TRUNCATION_HINT_TYPE } from '../treeTruncate'

const makeNodes = n => Array.from({ length: n }, (_, i) => ({
  path: `C:/data/file${i}.txt`,
  name: `file${i}.txt`,
  type: 'file',
  isLeaf: true
}))

describe('truncateTreeNodes', () => {
  it('未超限原样返回，不追加哨兵', () => {
    const nodes = makeNodes(10)
    const r = truncateTreeNodes(nodes, 'C:/data', 20)
    expect(r.truncated).toBe(false)
    expect(r.nodes).toBe(nodes)
    expect(r.total).toBe(10)
    expect(r.hiddenCount).toBe(0)
    expect(r.nodes.some(x => x.type === TRUNCATION_HINT_TYPE)).toBe(false)
  })

  it('恰好等于 limit 不截断（边界）', () => {
    const nodes = makeNodes(100)
    const r = truncateTreeNodes(nodes, 'C:/data', 100)
    expect(r.truncated).toBe(false)
    expect(r.nodes).toHaveLength(100)
  })

  it('超限截断为 limit + 1 个哨兵', () => {
    const r = truncateTreeNodes(makeNodes(3000), 'C:/data', 2000)
    expect(r.truncated).toBe(true)
    expect(r.nodes).toHaveLength(2001)
    expect(r.total).toBe(3000)
    expect(r.hiddenCount).toBe(1000)
    // 前 limit 项保持原顺序原引用
    expect(r.nodes[0].name).toBe('file0.txt')
    expect(r.nodes[1999].name).toBe('file1999.txt')
  })

  it('哨兵节点为叶子类型且 path 以父路径为命名空间派生（NUL 拼接）', () => {
    const r = truncateTreeNodes(makeNodes(3000), 'C:/logs', 2000)
    const hint = r.nodes[r.nodes.length - 1]
    expect(hint.type).toBe(TRUNCATION_HINT_TYPE)
    expect(hint.isLeaf).toBe(true)
    expect(hint.path).toBe('C:/logs\0truncation-hint')
    expect(hint.path).not.toBe('C:/logs/file0.txt')
  })

  it('不同父路径的哨兵 path 不冲突（node-key 唯一性）', () => {
    const a = truncateTreeNodes(makeNodes(3000), 'C:/logs', 2000)
    const b = truncateTreeNodes(makeNodes(3000), 'C:/data', 2000)
    const hintA = a.nodes[a.nodes.length - 1]
    const hintB = b.nodes[b.nodes.length - 1]
    expect(hintA.path).not.toBe(hintB.path)
  })

  it('哨兵 path 含 NUL：OS 路径非法字符，与任何真实文件名（含 Linux 合法的 :: 字符）零冲突', () => {
    // Linux 文件名合法含 ':'：真实文件 'b::truncation-hint' 与 '/a/b' 层哨兵
    // 若用可见分隔符拼接会撞 node-key；NUL 拼接从字符集层面杜绝
    const r = truncateTreeNodes(makeNodes(3000), '/a/b', 2000)
    const hint = r.nodes[r.nodes.length - 1]
    expect(hint.path.includes('\0')).toBe(true)
    // 真实节点 path 均不含 NUL，合成 key 不可能与任何真实 path 相等
    r.nodes.slice(0, -1).forEach(n => expect(n.path.includes('\0')).toBe(false))
    expect(hint.path).not.toBe('/a/b::truncation-hint')
  })

  it('哨兵文案包含隐藏数量', () => {
    const r = truncateTreeNodes(makeNodes(3000), 'C:/data', 2000)
    const hint = r.nodes[r.nodes.length - 1]
    expect(hint.name).toContain('1000')
  })

  it('默认 limit 为导出常量 FILE_TREE_NODE_LIMIT', () => {
    expect(FILE_TREE_NODE_LIMIT).toBe(2000)
    const r = truncateTreeNodes(makeNodes(FILE_TREE_NODE_LIMIT + 5), 'C:/data')
    expect(r.truncated).toBe(true)
    expect(r.nodes).toHaveLength(FILE_TREE_NODE_LIMIT + 1)
    expect(r.hiddenCount).toBe(5)
  })

  it('非数组入参降级为空列表不抛错', () => {
    expect(truncateTreeNodes(null, 'C:/data')).toEqual({ nodes: [], truncated: false, total: 0, hiddenCount: 0 })
    expect(truncateTreeNodes(undefined, 'C:/data')).toEqual({ nodes: [], truncated: false, total: 0, hiddenCount: 0 })
  })

  it('空数组不截断', () => {
    const r = truncateTreeNodes([], 'C:/data')
    expect(r.truncated).toBe(false)
    expect(r.nodes).toHaveLength(0)
  })
})
