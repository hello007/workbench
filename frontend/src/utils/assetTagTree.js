/**
 * 素材标签树构建纯函数集（素材管理第六批，SRS §2.4 / PRD D1/D2/D7 + D8#1 拖拽
 * 排序增量）。
 *
 * 数据源为 ListAssetTags 返回的平铺清单（model.AssetTagListItem：
 * {id, name, color, parentId, count, sortOrder}，后端按「父级分组 + 组内
 * sortOrder 升序、同序按 id」返回）。父节点子树合计与删除影响面由前端按树结构
 * 自算（后端只给直接绑定计数，见 model 注释），树构建 / 子树收集 / 计数合计 /
 * 拖拽落点换算四个消费点（AssetTagTreeSelect 树数据、AssetTagManagerDialog 删除
 * 确认与防环父级排除与拖拽排序）共用本模块，禁在消费组件内重复实现防口径漂移。
 *
 * 口径约定：
 * - parentId 0（或父引用缺失）= 根标签，与后端 COALESCE(parent_id, 0) 读出形态
 *   一致；父引用指向不存在的标签（已被删除的悬空引用）按根处理，防孤儿节点丢失；
 * - 存量异常自引用（parentId === id）按根处理，不产生自环 children；
 * - 后端防环校验（service updateTagFull/moveTag 沿 parent 链向上遍历）为主防线，
 *   本模块的子树排除是前端体验层预拦截（编辑态父级选项不含自身与后代），两层
 *   独立；
 * - 树构建保序消费：buildTagTree 按平铺出现序挂 children / roots，后端排序即
 *   兄弟展示序的单一来源，前端不重排。
 */

/**
 * 平铺标签清单构建 el-tree-select 树形数据。
 *
 * 节点字段 value/label 为 el-tree-select 默认键位（nodeKey 缺省取 'value'，
 * filterable 检索匹配 label），其余字段供自定义节点内容（色点、计数徽标）与
 * 管理弹窗操作列消费。
 *
 * @param {Array<{id: number, name: string, color: string, parentId: number, count: number}>} items
 * @returns {Array<{value: number, label: string, id: number, name: string, color: string, count: number, parentId: number, children: Array}>}
 */
export function buildTagTree(items) {
  const list = items || []
  const nodes = new Map()
  for (const item of list) {
    nodes.set(item.id, {
      value: item.id,
      label: item.name,
      id: item.id,
      name: item.name,
      color: item.color || '',
      count: item.count || 0,
      parentId: item.parentId || 0,
      children: []
    })
  }
  const roots = []
  for (const item of list) {
    const node = nodes.get(item.id)
    // 自引用（存量异常数据）与悬空父引用均按根处理
    const parent = node.parentId && node.parentId !== node.id ? nodes.get(node.parentId) || null : null
    if (parent) {
      parent.children.push(node)
    } else {
      roots.push(node)
    }
  }
  return roots
}

/**
 * 收集标签子树 ID 集（含自身）。
 *
 * 管理弹窗删除影响面标签数（N）、编辑态父级选项排除（防环预拦截）共用单点。
 * 沿「父 → 子」方向 DFS，visited 集合兜底存量成环数据不死循环（与后端
 * expandTagEdgesToClosure 同思路）；输出按 ID 升序（与后端闭包函数同口径，
 * 断言与展示两消费点均不依赖遍历序）。
 *
 * @param {Array<{id: number, parentId: number}>} items
 * @param {number} rootId
 * @returns {number[]}
 */
export function collectSubtreeIds(items, rootId) {
  const childrenOf = new Map()
  for (const item of items || []) {
    const parentId = item.parentId || 0
    // 自引用不建边（与 buildTagTree 同口径，防自环）
    if (parentId && parentId !== item.id) {
      if (!childrenOf.has(parentId)) childrenOf.set(parentId, [])
      childrenOf.get(parentId).push(item.id)
    }
  }
  const out = []
  const seen = new Set()
  const stack = [rootId]
  while (stack.length > 0) {
    const id = stack.pop()
    if (seen.has(id)) continue
    seen.add(id)
    out.push(id)
    for (const child of childrenOf.get(id) || []) stack.push(child)
  }
  return out.sort((a, b) => a - b)
}

/**
 * 子树直接绑定计数合计。
 *
 * AssetTagManagerDialog 删除确认弹窗影响面 M 的本地口径：各节点直接绑定行数
 * 之和，与后端 deleteTagTree 实际解除的绑定行数同口径（同一条目绑多标签按行计，
 * 不做条目去重）。删除成功后的 success 提示以后端返回值为权威口径——本合计是
 * 删除前快照，并发变更下两者可能不同。
 *
 * @param {Array<{id: number, count: number, parentId: number}>} items
 * @param {number} rootId
 * @returns {number}
 */
export function sumSubtreeCounts(items, rootId) {
  const byId = new Map((items || []).map(item => [item.id, item]))
  return collectSubtreeIds(items, rootId).reduce(
    (sum, id) => sum + (byId.get(id)?.count || 0),
    0
  )
}

/**
 * el-tree 拖拽落点换算为后端 MoveAssetTag 入参（PRD D8#1，AssetTagManagerDialog
 * node-drop 单点消费）。
 *
 * 落点语义（el-tree node-drop 第三参 dropType）：
 * - 'before' / 'after'：同级排序——落点为 dropNode 所在兄弟组，index 为落位后
 *   拖拽节点在该组（排除自身后插入）的最终序号；
 * - 'inner'：变子级——拖拽节点成为 dropNode 的最后一个子节点，index = 目标组
 *   排除自身后的长度（组尾追加）。
 *
 * 返回 { newParentID, index }；落点节点不在树内等异常形态返回 null（调用方直接
 * 忽略本次 drop）。兄弟列表取自树结构（buildTagTree 产物，兄弟序 = 后端返回序
 * = 管理弹窗展示序），与后端 moveTag 的「现序排除自身后按 index 插入」语义一致。
 *
 * @param {Array} treeNodes buildTagTree 产物（节点含 id/parentId/children）
 * @param {number} tagId 拖拽节点 ID
 * @param {number} dropNodeId 落点节点 ID
 * @param {string} dropType el-tree dropType（before/after/inner）
 * @returns {{newParentID: number, index: number}|null}
 */
export function resolveTagMoveDrop(treeNodes, tagId, dropNodeId, dropType) {
  const found = findTagNodeWithSiblings(treeNodes || [], dropNodeId)
  if (!found) return null
  const { siblings, parentId, node } = found
  if (dropType === 'inner') {
    const restCount = (node.children || []).filter(child => child.id !== tagId).length
    return { newParentID: dropNodeId, index: restCount }
  }
  const rest = siblings.filter(sibling => sibling.id !== tagId)
  const dropIdx = rest.findIndex(sibling => sibling.id === dropNodeId)
  if (dropIdx < 0) return null
  return { newParentID: parentId, index: dropType === 'before' ? dropIdx : dropIdx + 1 }
}

/**
 * 在树中查找节点及其所属兄弟列表（含父 ID）。roots 的 parentId 为 0。
 *
 * @param {Array} treeNodes buildTagTree 产物
 * @param {number} targetId 目标节点 ID
 * @returns {{node: object, siblings: Array, parentId: number}|null}
 */
function findTagNodeWithSiblings(treeNodes, targetId) {
  const walk = (nodes, parentId) => {
    for (const node of nodes) {
      if (node.id === targetId) return { node, siblings: nodes, parentId }
      const hit = node.children && node.children.length > 0 ? walk(node.children, node.id) : null
      if (hit) return hit
    }
    return null
  }
  return walk(treeNodes, 0)
}

/**
 * 把拖拽落位应用到平铺清单（乐观更新，AssetTagManagerDialog 消费）：返回新数组
 * （不改入参——整体替换的响应式纪律），拖拽节点 parentId 更新为目标父，且目标
 * 兄弟组在平铺数组中的相对顺序变为「排除自身后按 index 插入自身」的最终序
 * （buildTagTree 只依赖组内相对序，其余组顺序保持原位不动）。
 *
 * rest 序取自平铺清单内目标组的出现序——与后端 siblingTagIDsTx 的
 * (sort_order, id) 序一致（每次落位后整组连续化重排，平铺序即序号序）。
 *
 * @param {Array<{id: number, parentId: number}>} items ListAssetTags 平铺清单
 * @param {number} tagId 拖拽节点 ID
 * @param {number} newParentID 目标父 ID（0 = 根）
 * @param {number} index 落位后自身在目标组内的最终序号（resolveTagMoveDrop 产物）
 * @returns {Array} 新平铺数组
 */
export function applyTagMoveToFlat(items, tagId, newParentID, index) {
  const list = items || []
  const rest = list
    .filter(item => (item.parentId || 0) === newParentID && item.id !== tagId)
    .map(item => item.id)
  const clamped = Math.max(0, Math.min(index, rest.length))
  const finalIds = [...rest.slice(0, clamped), tagId, ...rest.slice(clamped)]
  // 目标组全体成员按 finalIds 序回填到平铺数组中该组原有占位（其余条目原位不动）
  const byId = new Map(
    list.map(item => (item.id === tagId ? [item.id, { ...item, parentId: newParentID }] : [item.id, item]))
  )
  let cursor = 0
  return list.map(item => {
    if ((item.parentId || 0) === newParentID || item.id === tagId) {
      const next = byId.get(finalIds[cursor])
      cursor += 1
      return next
    }
    return byId.get(item.id)
  })
}
