// frontend/src/utils/assetFormat.js
// 素材管理展示格式化共享单点（AssetsPanel 网格卡与 AssetListView 列表行共用）。
// 状态映射与大小格式化一旦两处各自实现即出现网格/列表口径漂移（改一处漏一处），
// 故收敛为本 util，消费方 import 使用、禁本地复制。

// ---- 索引状态展示映射（对齐 model/assets.go 状态机取值）----
export const assetStatusLabel = (status) => ({
  processing: '解析中',
  ok: '正常',
  failed: '解析失败',
  encrypted: '已加密',
  denied: '权限不足',
  lost: '丢失'
}[status] || status)

// 状态标签语义色（el-tag type）：解析中 info / 正常 success / 解析失败 danger /
// 已加密、权限不足与丢失 warning（异常提示语义，非 error 红；丢失可经重链/
// 复活/清理处置，属「待处理」而非终态故障）
export const assetStatusTagType = (status) => ({
  processing: 'info',
  ok: 'success',
  failed: 'danger',
  encrypted: 'warning',
  denied: 'warning',
  lost: 'warning'
}[status] || 'info')

// ---- 文件大小格式化（B → KB → MB → GB，整数值不带小数位）----
export const formatAssetSize = (bytes) => {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value % 1 === 0 ? value : value.toFixed(1)} ${units[unit]}`
}
