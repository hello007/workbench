/**
 * 防抖工具：waitMs 窗口内的连续调用合并为尾部一次执行。
 *
 * 供 ResizeObserver 回调等高频触发面消费（如终端区拖拽/全屏切换期间
 * 收口 refit + ResizeTerminal IPC，terminal-multi-session 契约）。
 * 不引入 perfect-debounce 等未登记依赖（前端依赖分档钳制约束），手写最小面：
 * - 尾部触发：窗口内最后一次调用后 waitMs 才执行，携带该次参数
 * - cancel()：取消防抖并清空计时器（组件卸载/实例销毁时防悬挂回调）
 *
 * @param {Function} fn 待防抖函数
 * @param {number} waitMs 防抖窗口毫秒
 * @returns {Function & { cancel: () => void }} 带 cancel 方法的防抖函数
 */
export function debounce(fn, waitMs) {
  let timer = null
  function debounced(...args) {
    if (timer !== null) {
      clearTimeout(timer)
    }
    timer = setTimeout(() => {
      timer = null
      fn(...args)
    }, waitMs)
  }
  debounced.cancel = () => {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
  }
  return debounced
}
