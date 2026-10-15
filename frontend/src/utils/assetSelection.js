// frontend/src/utils/assetSelection.js
// 素材选中键映射共享单点（SRS §2.3 PR3，PRD D2）：网格卡（AssetsPanel 宿主）与
// 列表行（AssetListView）消费同一映射函数，禁两份口径漂移（对齐 assetFormat.js
// 的共享单点先例）。
//
// 键形态契约（第六批批量操作跨批消费以此为准）：
// - 文件条目：'file:<fileId>'
// - 幻灯片条目：'slide:<fileId>:<slideNum>'
//
// 禁用 slides.id 作选中键：slides 每轮重解析删旧插新全变（files.id 同行稳定），
// 与缩略图缓存键用 files.id 同理由；类型前缀防第六批批量消费时的类型歧义，
// 键反查条目经宿主底册（fileEntries）/ 清单（slideEntries + searchItems）。
export const assetSelectionKey = (card) => {
  if (!card) return ''
  if (card.kind === 'file') return `file:${card.id}`
  return `slide:${card.fileId}:${card.slideNum}`
}
