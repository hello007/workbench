/**
 * 文件树目录内按名筛选（截断优化配套，perf-baseline §15.6 盲区补齐）
 *
 * 背景：单层超 2000 项截断后，2000 名外文件在树中无入口；哨兵文案引导的
 * Ctrl+P 命令面板走 SearchFiles（service/search.go 跳过 node_modules），
 * 而扁平 node_modules 恰是截断触发面之一。树内筛选对该层 GetFileTree 全量
 * 拉取（Go 侧缓存命中）后按名过滤，不受截断限制。
 *
 * 过滤为纯函数：与组件/后端解耦，可独立单测；匹配口径为节点 name 子串
 * 包含、大小写不敏感（独立于 SearchFiles 的 fuzzyMatch 评分语义）。
 */

/**
 * 按名称子串过滤节点列表（大小写不敏感，保持后端返回顺序不重排）。
 *
 * @param {Array<{name: string, [k: string]: any}>} nodes GetFileTree 返回的单层节点
 * @param {string} keyword 筛选关键词；空/纯空白串返回原列表（数组化兜底）
 * @returns {Array} 命中节点
 */
export function filterTreeNodesByName(nodes, keyword) {
  const list = Array.isArray(nodes) ? nodes : []
  const kw = (keyword || '').trim().toLowerCase()
  if (!kw) return list
  return list.filter(n => typeof n?.name === 'string' && n.name.toLowerCase().includes(kw))
}
