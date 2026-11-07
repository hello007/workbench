/**
 * Unix 时间戳的相对时间与完整时间格式化工具。
 *
 * 相对时间口径对齐 CommitHistory.vue 既有 formatTime 先例（X 分钟/小时/天前，
 * 超 30 天转 zh-CN 日期），抽出为可复用工具供全局状态看板等新消费方使用；
 * CommitHistory/GitInfo 既有重复实现保持不动（避免回归扩大影响面），后续如需
 * 收敛为单一数据源另行统一。
 */

/**
 * Unix 秒 → 相对时间描述。
 * @param {number} unixSeconds 提交时间 Unix 秒（0/undefined 视为无数据）
 * @returns {string} "X 分钟前" / "X 小时前" / "X 天前"；超 30 天转 zh-CN 日期；无数据返回 '-'
 */
export function formatRelativeTime(unixSeconds) {
  if (!unixSeconds) return '-'
  const diff = Date.now() - unixSeconds * 1000
  const minutes = Math.floor(diff / (1000 * 60))
  const hours = Math.floor(diff / (1000 * 60 * 60))
  const days = Math.floor(diff / (1000 * 60 * 60 * 24))

  if (minutes < 60) return `${minutes} 分钟前`
  if (hours < 24) return `${hours} 小时前`
  if (days < 30) return `${days} 天前`
  return new Date(unixSeconds * 1000).toLocaleDateString('zh-CN')
}

/**
 * Unix 秒 → 完整本地时间字符串（列表 hover title 用）。
 * @param {number} unixSeconds 提交时间 Unix 秒（0/undefined 视为无数据）
 * @returns {string} 本地日期时间串，如 "2026/11/7 14:30:00"；无数据返回 '-'
 */
export function formatFullTime(unixSeconds) {
  if (!unixSeconds) return '-'
  return new Date(unixSeconds * 1000).toLocaleString('zh-CN')
}
