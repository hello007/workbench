// frontend/src/utils/assetCart.js
// 素材购物栏条目键映射共享单点（PR3 购物栏）：AssetsPanel 已收集态判定/批量构造
// 与 AssetCartDrawer 拖拽回写消费同一映射函数，禁两份口径漂移（对齐 assetSelection.js
// / assetFormat.js 的共享单点先例）。
//
// 键形态契约与后端 model.AssetCartItem.Key 同构（跨层键形态契约，两处禁漂移）：
//   `<libraryId>|<fileId>|<slideNum>`
// libraryId 为后端随机 hex、fileId/slideNum 为数字，竖线分隔无歧义。

/**
 * 购物栏条目键（Remove/Reorder 与已收集态判定的统一形态）。
 * @param {{libraryId: string, fileId: number, slideNum: number}} item
 * @returns {string}
 */
export const assetCartItemKey = (item) =>
  `${item.libraryId}|${item.fileId}|${item.slideNum}`
