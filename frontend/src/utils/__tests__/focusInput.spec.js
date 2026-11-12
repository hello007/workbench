// focusInput.js 输入聚焦与快捷键判定测试
// 覆盖：原生元素、ElInput 实例代理、stub 根元素回退、光标末尾、滚动到底、保存组合键矩阵
import { describe, it, expect, vi, afterEach } from 'vitest'
import { focusInputAtEnd, isSaveShortcut, resolveInputElement } from '../focusInput'

const createTextarea = (value = '') => {
  const el = document.createElement('textarea')
  el.value = value
  document.body.appendChild(el)
  return el
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

describe('resolveInputElement', () => {
  it('优先取 ElInput 实例暴露的 textarea', () => {
    const textarea = createTextarea()
    const component = { textarea, input: document.createElement('input') }
    expect(resolveInputElement(component)).toBe(textarea)
  })

  it('无 textarea 时取 input', () => {
    const input = document.createElement('input')
    const component = { input }
    expect(resolveInputElement(component)).toBe(input)
  })

  it('原生元素直接返回', () => {
    const textarea = createTextarea()
    expect(resolveInputElement(textarea)).toBe(textarea)
  })

  it('stub 组件回退到根元素内的可编辑控件', () => {
    const wrapper = document.createElement('div')
    const textarea = document.createElement('textarea')
    wrapper.appendChild(textarea)
    expect(resolveInputElement({ $el: wrapper })).toBe(textarea)
  })

  it('空值与非输入元素返回 null', () => {
    expect(resolveInputElement(null)).toBeNull()
    expect(resolveInputElement(document.createElement('div'))).toBeNull()
  })
})

describe('focusInputAtEnd', () => {
  it('聚焦后光标落在文本末尾', () => {
    const textarea = createTextarea('已有内容')
    // jsdom 写 value 时会把光标放到末尾，先归零才能验证工具自身把光标移回末尾
    textarea.setSelectionRange(0, 0)
    const focusSpy = vi.spyOn(textarea, 'focus')

    expect(focusInputAtEnd(textarea)).toBe(true)
    expect(focusSpy).toHaveBeenCalledTimes(1)
    expect(textarea.selectionStart).toBe(4)
    expect(textarea.selectionEnd).toBe(4)
  })

  it('长文本同时滚动到底部', () => {
    const textarea = createTextarea('x'.repeat(200))
    // jsdom 不做布局，scrollHeight 恒 0；用属性覆写模拟溢出
    Object.defineProperty(textarea, 'scrollHeight', { value: 480, configurable: true })
    focusInputAtEnd(textarea)
    expect(textarea.scrollTop).toBe(480)
  })

  it('ElInput 实例走其 textarea 并定位末尾', () => {
    const textarea = createTextarea('abc')
    textarea.setSelectionRange(0, 0)
    expect(focusInputAtEnd({ textarea })).toBe(true)
    expect(textarea.selectionStart).toBe(3)
  })

  it('找不到可编辑元素时返回 false 且不抛错', () => {
    expect(focusInputAtEnd(null)).toBe(false)
    expect(focusInputAtEnd({})).toBe(false)
  })
})

describe('isSaveShortcut', () => {
  const event = (overrides) => ({
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    key: 's',
    ...overrides
  })

  it('Ctrl+S 与 Cmd+S 命中，大小写不敏感', () => {
    expect(isSaveShortcut(event({ ctrlKey: true }))).toBe(true)
    expect(isSaveShortcut(event({ metaKey: true }))).toBe(true)
    expect(isSaveShortcut(event({ ctrlKey: true, key: 'S' }))).toBe(true)
  })

  it('缺少 Ctrl/Cmd 或带 Alt/Shift 时不命中', () => {
    expect(isSaveShortcut(event({}))).toBe(false)
    expect(isSaveShortcut(event({ ctrlKey: true, altKey: true }))).toBe(false)
    expect(isSaveShortcut(event({ ctrlKey: true, shiftKey: true }))).toBe(false)
  })

  it('非 S 键与空事件不命中', () => {
    expect(isSaveShortcut(event({ ctrlKey: true, key: 'a' }))).toBe(false)
    expect(isSaveShortcut(null)).toBe(false)
  })

  it('输入法组合中不命中', () => {
    expect(isSaveShortcut(event({ ctrlKey: true, isComposing: true }))).toBe(false)
    expect(isSaveShortcut(event({ ctrlKey: true, keyCode: 229 }))).toBe(false)
  })
})
