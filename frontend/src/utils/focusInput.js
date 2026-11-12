/**
 * 输入控件聚焦并定位光标到文本末尾。
 *
 * 新建/编辑入口打开后统一走这里，避免三处各写一遍 focus + setSelectionRange：
 * - 原生 input / textarea 直接传入即可；
 * - Element Plus ElInput 实例取它暴露的 textarea / input；stub 组件没有该属性时
 *   回退到根元素内的 textarea/input，单测无需为 stub 额外实现转发；
 * - 长文本聚焦后滚动到底部，光标位置与可视区域保持一致。
 */

/**
 * 解析输入组件对应的原生可编辑元素。
 *
 * @param {object|HTMLElement|null} target ElInput 实例 / 组件实例 / 原生元素
 * @returns {HTMLElement|null}
 */
export const resolveInputElement = (target) => {
  if (!target) return null
  if (target.textarea) return target.textarea
  if (target.input) return target.input
  if (typeof target.matches === 'function') {
    return target.matches('textarea, input') ? target : target.querySelector('textarea, input')
  }
  const root = target.$el
  if (!root) return null
  if (typeof root.matches === 'function' && root.matches('textarea, input')) return root
  return typeof root.querySelector === 'function' ? root.querySelector('textarea, input') : null
}

/**
 * 聚焦输入控件，把光标放到文本末尾并滚动到底部。
 *
 * 调用方只传入文本输入。`setSelectionRange` 的能力判断用于兼容不同渲染环境
 * （stub / 非输入元素），不是类型白名单——number 等类型同样有该方法但会抛错，
 * 本工具不接收此类输入。
 *
 * @param {object|HTMLElement|null} target ElInput 实例 / 组件实例 / 原生元素
 * @returns {boolean} 是否找到并可聚焦原生元素
 */
export const focusInputAtEnd = (target) => {
  const el = resolveInputElement(target)
  if (!el || typeof el.focus !== 'function') return false

  el.focus()
  const end = typeof el.value === 'string' ? el.value.length : 0
  if (typeof el.setSelectionRange === 'function') {
    el.setSelectionRange(end, end)
  }
  if (typeof el.scrollTop === 'number') {
    el.scrollTop = el.scrollHeight
  }
  return true
}

/**
 * 判断是否为 Ctrl/Cmd + S 保存组合键（不区分 S 大小写）。
 * Shift、Alt 参与时不命中，把 Ctrl+Shift+S 等系统「另存为」留给系统；
 * 输入法组合中（isComposing）不命中，与其他快捷键入口保持同一口径。
 *
 * @param {KeyboardEvent} event
 * @returns {boolean}
 */
export const isSaveShortcut = (event) => {
  if (!event) return false
  if (event.isComposing || event.keyCode === 229) return false
  if (!(event.ctrlKey || event.metaKey)) return false
  if (event.altKey || event.shiftKey) return false
  return String(event.key).toLowerCase() === 's'
}
