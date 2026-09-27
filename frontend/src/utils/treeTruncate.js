/**
 * 文件树单层节点截断（perf-baseline §15.4 大目录展开优化）
 *
 * 背景：el-tree 无虚拟滚动，展开单层大目录全量渲染——10k 节点 ~4.4s / JSHeap +311MB，
 * 100k 节点 ~49.2s / +3.14GB（可致 WebView2 OOM）。截断后渲染量与本层目录规模解耦。
 *
 * 阈值为前端渲染性能参数，硬编码常量（ADR 见任务 09-27-tree-large-dir-expand PRD）：
 * settings 化成本（model.AppSettings 字段 + wailsjs 绑定三处同步 + SettingsPanel UI）
 * 不抵收益；后续若需可配置，改为此常量读 store 一处即可。
 *
 * 超出截断名单的文件定位：哨兵节点文案引导两条路径——工具栏按名筛选（对该层
 * GetFileTree 全量拉取后前端过滤，不受截断限制；SearchFiles 跳过 node_modules，
 * 对扁平 node_modules 场景无效）与命令面板（Ctrl+P）文件名搜索。
 */

/** 单层渲染节点数上限（不含哨兵提示节点） */
export const FILE_TREE_NODE_LIMIT = 2000

/** 哨兵提示节点类型标记（FileTreePanel 据此过滤点击/右键菜单并套弱化样式） */
export const TRUNCATION_HINT_TYPE = 'truncation-hint'

/**
 * 截断单层节点列表，超限时在末尾追加哨兵提示节点。
 *
 * @param {Array<{path: string, type: string, [k: string]: any}>} nodes 后端返回的单层节点
 * @param {string} parentPath 被截断层的目录路径（用于派生哨兵唯一 node-key；根层传工作目录路径）
 * @param {number} [limit=FILE_TREE_NODE_LIMIT]
 * @returns {{nodes: Array, truncated: boolean, total: number, hiddenCount: number}}
 *   nodes 为可直接交给 el-tree resolve 的列表（未超限时原样返回）
 */
export function truncateTreeNodes(nodes, parentPath, limit = FILE_TREE_NODE_LIMIT) {
  const list = Array.isArray(nodes) ? nodes : []
  if (list.length <= limit) {
    return { nodes: list, truncated: false, total: list.length, hiddenCount: 0 }
  }

  const hiddenCount = list.length - limit
  const displayNodes = list.slice(0, limit)
  displayNodes.push({
    // node-key 要求全树唯一：以父路径为命名空间派生合成 key。用 NUL（\0）拼接——
    // NUL 是 OS 路径的非法字符（Windows/Linux 均禁），后端返回的真实 path 物理上
    // 不可能含 NUL，故合成 key 与任何真实节点零冲突（Linux 文件名合法含 `:`，
    // `::` 之类可见分隔符存在被真实文件名撞 key 的理论可能，NUL 无此面）
    path: `${parentPath}\0truncation-hint`,
    name: `已显示前 ${limit} 项，其余 ${hiddenCount} 项未加载（点击此处按名筛选，或 Ctrl+P 搜索）`,
    type: TRUNCATION_HINT_TYPE,
    isLeaf: true
  })
  return { nodes: displayNodes, truncated: true, total: list.length, hiddenCount }
}
