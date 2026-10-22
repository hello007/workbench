/**
 * 分支差异文件树构建与状态映射（纯函数，与组件解耦）。
 *
 * BranchCompareFileTree 组件消费：扁平 BranchDiffFile[] → 目录树节点；
 * 状态列表（过滤选项）与状态着色 class 的单一映射源。
 */

// 状态中文标签（多选过滤选项）；与后端 status 取值一一对应
export const STATUS_LIST = [
  { value: 'added', label: '新增' },
  { value: 'deleted', label: '删除' },
  { value: 'renamed', label: '重命名' },
  { value: 'copied', label: '复制' },
  { value: 'typechange', label: '类型变更' },
  { value: 'modified', label: '修改' },
  { value: 'unchanged', label: '未变更' }
]

/**
 * 扁平文件列表 → 目录树节点（按 '/' 分段聚合）。
 * 节点形态：目录 {dir: true, name, path, children}；文件 {dir: false, name, path, file}。
 * 排序：目录在前、同级按名称字典序（后端已按 path 排序，此处仅聚合展示顺序）。
 *
 * @param {Array<{path: string, [k: string]: any}>} files
 * @returns {Array} 树根节点数组
 */
export function buildTree(files) {
  const root = { dir: true, name: '', path: '', children: new Map() }
  for (const file of files || []) {
    const segs = file.path.split('/')
    let cur = root
    for (let i = 0; i < segs.length - 1; i++) {
      const seg = segs[i]
      const path = segs.slice(0, i + 1).join('/')
      if (!cur.children.has(seg)) {
        cur.children.set(seg, { dir: true, name: seg, path, children: new Map() })
      }
      cur = cur.children.get(seg)
    }
    const name = segs[segs.length - 1]
    cur.children.set(name, { dir: false, name, path: file.path, file })
  }
  const toNodes = (mapNode) => {
    const nodes = Array.from(mapNode.children.values()).map(n =>
      n.dir ? { ...n, children: toNodes(n) } : { ...n }
    )
    nodes.sort((a, b) => {
      if (a.dir !== b.dir) return a.dir ? -1 : 1
      return a.name < b.name ? -1 : a.name > b.name ? 1 : 0
    })
    return nodes
  }
  return toNodes(root)
}
