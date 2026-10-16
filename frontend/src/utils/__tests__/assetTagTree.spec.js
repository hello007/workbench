import { describe, it, expect } from 'vitest'
import { applyTagMoveToFlat, buildTagTree, collectSubtreeIds, resolveTagMoveDrop, sumSubtreeCounts } from '../assetTagTree'

// 形状对齐 model.AssetTagListItem（ListAssetTags 返回，父级分组 + 组内 sortOrder 序）
const flatTags = [
  { id: 1, name: '重点客户', color: '#2563eb', parentId: 0, count: 3 },
  { id: 2, name: '城商行', color: '#0891b2', parentId: 1, count: 2 },
  { id: 3, name: '农信社', color: '#d97706', parentId: 1, count: 0 },
  { id: 4, name: '方案', color: '#16a34a', parentId: 0, count: 5 }
]

describe('buildTagTree', () => {
  it('按 parentId 构建两级树，节点携带 value/label 默认键位与原始字段', () => {
    const tree = buildTagTree(flatTags)
    expect(tree.map(node => node.id)).toEqual([1, 4])
    const parent = tree[0]
    expect(parent.value).toBe(1)
    expect(parent.label).toBe('重点客户')
    expect(parent.count).toBe(3)
    expect(parent.children.map(node => node.id)).toEqual([2, 3])
    expect(tree[1].children).toEqual([])
  })

  it('父引用指向不存在的标签（悬空引用）按根处理防丢节点', () => {
    const tree = buildTagTree([
      { id: 7, name: '孤儿', color: '', parentId: 999, count: 0 },
      { id: 8, name: '根', color: '', parentId: 0, count: 1 }
    ])
    expect(tree.map(node => node.id)).toEqual([7, 8])
  })

  it('存量异常自引用（parentId 等于自身 id）按根处理不产生自环 children', () => {
    const tree = buildTagTree([{ id: 5, name: '自引用', color: '', parentId: 5, count: 0 }])
    expect(tree.map(node => node.id)).toEqual([5])
    expect(tree[0].children).toEqual([])
  })

  it('空清单与缺省入参返回空数组（下拉空态由组件据此提示）', () => {
    expect(buildTagTree([])).toEqual([])
    expect(buildTagTree(null)).toEqual([])
  })

  it('color/count 缺失归一为空串与 0（兜底聚合数据形态不炸渲染）', () => {
    const tree = buildTagTree([{ id: 9, name: '裸标签', parentId: 0 }])
    expect(tree[0].color).toBe('')
    expect(tree[0].count).toBe(0)
  })
})

describe('collectSubtreeIds', () => {
  it('收集自身与全部后代（多层级联），子树 ID 集供删除影响面与防环排除共用', () => {
    expect(collectSubtreeIds(flatTags, 1)).toEqual([1, 2, 3])
    expect(collectSubtreeIds(flatTags, 2)).toEqual([2])
    expect(collectSubtreeIds(flatTags, 4)).toEqual([4])
  })

  it('目标 id 不在清单中时返回仅含自身的集合（删除已失效引用安全收口）', () => {
    expect(collectSubtreeIds(flatTags, 999)).toEqual([999])
  })

  it('存量成环数据 visited 兜底不死循环（A→B→A 互指）', () => {
    const cyclic = [
      { id: 1, parentId: 2 },
      { id: 2, parentId: 1 }
    ]
    expect(collectSubtreeIds(cyclic, 1).sort((a, b) => a - b)).toEqual([1, 2])
  })
})

describe('sumSubtreeCounts', () => {
  it('子树直接绑定计数合计（删除确认弹窗影响面 M 口径：按绑定行计）', () => {
    expect(sumSubtreeCounts(flatTags, 1)).toBe(5) // 3 + 2 + 0
    expect(sumSubtreeCounts(flatTags, 4)).toBe(5)
  })

  it('目标节点缺失计数或 id 不存在时按 0 兜底不抛错', () => {
    expect(sumSubtreeCounts([{ id: 1, parentId: 0 }], 1)).toBe(0)
    expect(sumSubtreeCounts(flatTags, 999)).toBe(0)
  })
})

// ---- 拖拽落点换算与平铺乐观更新（D8#1，AssetTagManagerDialog 消费）----

describe('resolveTagMoveDrop', () => {
  // 树形：1(根) → 2/3（子），4(根)
  const dragItems = [
    { id: 1, parentId: 0 },
    { id: 2, parentId: 1 },
    { id: 3, parentId: 1 },
    { id: 4, parentId: 0 }
  ]
  const dragTree = buildTagTree(dragItems)

  it('before/after 落点 = 同级排序：index 为落位后拖拽节点在目标组（排除自身）的最终序号', () => {
    // 拖 4 到根组 1 之前：rest=[1] → index 0
    expect(resolveTagMoveDrop(dragTree, 4, 1, 'before')).toEqual({ newParentID: 0, index: 0 })
    // 拖 4 到根组 1 之后：rest=[1] → index 1
    expect(resolveTagMoveDrop(dragTree, 4, 1, 'after')).toEqual({ newParentID: 0, index: 1 })
    // 同组内排序：拖 3 到兄弟 2 之后（rest=[2] → index 1）
    expect(resolveTagMoveDrop(dragTree, 3, 2, 'after')).toEqual({ newParentID: 1, index: 1 })
  })

  it('inner 落点 = 变子级组尾追加：index 为目标子组排除拖拽节点的长度', () => {
    expect(resolveTagMoveDrop(dragTree, 4, 1, 'inner')).toEqual({ newParentID: 1, index: 2 })
    // 拖入无子节点：index 0（组尾即组首）
    expect(resolveTagMoveDrop(dragTree, 4, 3, 'inner')).toEqual({ newParentID: 3, index: 0 })
  })

  it('拖拽节点已在目标组内时排除自身再计序（同级原位拖拽不失真）', () => {
    // 拖 2 到兄弟 3 之后：rest=[3] → index 1
    expect(resolveTagMoveDrop(dragTree, 2, 3, 'after')).toEqual({ newParentID: 1, index: 1 })
  })

  it('落点节点不在树内返回 null（调用方忽略本次 drop）', () => {
    expect(resolveTagMoveDrop(dragTree, 4, 999, 'after')).toBeNull()
    expect(resolveTagMoveDrop(dragTree, 4, 999, 'inner')).toBeNull()
  })
})

describe('applyTagMoveToFlat', () => {
  const flatItems = [
    { id: 1, name: '行业', parentId: 0 },
    { id: 2, name: '方案', parentId: 0 },
    { id: 3, name: '目标', parentId: 0 },
    { id: 4, name: '城商行', parentId: 1 }
  ]

  it('跨父移动：拖拽节点 parentId 更新且目标组按插入序重排，其余条目原位不动', () => {
    const next = applyTagMoveToFlat(flatItems, 3, 1, 1)
    expect(next.map(item => item.id)).toEqual([1, 2, 4, 3])
    expect(next.find(item => item.id === 3).parentId).toBe(1)
    // 入参不被修改（整体替换的响应式纪律）
    expect(flatItems.find(item => item.id === 3).parentId).toBe(0)
  })

  it('同级排序：目标组整体按 finalIds 序回填原占位', () => {
    const next = applyTagMoveToFlat(flatItems, 1, 0, 2) // 行业移到根组尾
    expect(next.map(item => item.id)).toEqual([2, 3, 1, 4])
  })

  it('index 越界钳制到 [0, 组尾]（防御形态）', () => {
    expect(applyTagMoveToFlat(flatItems, 3, 0, -5).map(item => item.id)).toEqual([3, 1, 2, 4])
    expect(applyTagMoveToFlat(flatItems, 3, 0, 99).map(item => item.id)).toEqual([1, 2, 3, 4])
  })

  it('拖入自身所在组的原位移动（parentId 不变序号前移）', () => {
    const items = [
      { id: 1, parentId: 0 },
      { id: 2, parentId: 0 },
      { id: 3, parentId: 0 }
    ]
    expect(applyTagMoveToFlat(items, 1, 0, 2).map(item => item.id)).toEqual([2, 3, 1])
  })
})
