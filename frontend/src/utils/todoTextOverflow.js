/**
 * 判断折叠中的待办正文是否被截断（决定是否显示「展开」按钮）。
 *
 * 抽成纯函数便于单测：jsdom 不做布局，元素尺寸恒为 0，
 * 组件里无法验证真实截断，只能直接向本函数传入带尺寸的模拟元素。
 *
 * @param {HTMLElement|null} el 折叠态正文元素
 * @returns {boolean}
 */
export const isTextOverflowing = (el) => {
  if (!el) return false
  const clientHeight = el.clientHeight || 0
  // 面板 v-show 隐藏或元素尚未布局时 clientHeight 为 0，无法判定；
  // 此时按「未截断」处理，避免隐藏期间误判出一批「展开」按钮
  if (clientHeight <= 0) return false
  // 留 1px 容差，规避亚像素换行导致的假阳性
  return (el.scrollHeight || 0) > clientHeight + 1
}
