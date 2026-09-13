/**
 * Vitest 测试环境设置
 */

import { vi } from 'vitest'
import { WAILS_MOCK_DEFAULT_RETURN_VALUES } from './wails-mock-defaults'

// jsdom 缺失的浏览器 API 补丁：
// - ResizeObserver：@codemirror/view 顶层模块引用，否则加载即抛 ReferenceError
// - DOMMatrix：兜底防御（曾为 pdfjs-dist 准备，pdfjs 已移除但保留 stub 无副作用）
// - matchMedia：xterm 初始化时调用（终端面板真实渲染时触发），jsdom 无实现需 stub
// 仅在测试环境生效，不影响生产构建。
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = ResizeObserverStub
}
if (!globalThis.DOMMatrix) {
  globalThis.DOMMatrix = class DOMMatrix {
    constructor() { this.a = 1; this.b = 0; this.c = 0; this.d = 1; this.e = 0; this.f = 0 }
    multiply() { return this }
    inverse() { return this }
  }
}
if (!globalThis.matchMedia) {
  globalThis.matchMedia = () => ({
    matches: false,
    media: '',
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {}
  })
}

// Mock Wails绑定（默认返回值表见 wails-mock-defaults.js，与 Playwright E2E 注入共用同一份数据）
vi.mock('../../wailsjs/go/main/App', () =>
  Object.fromEntries(
    Object.entries(WAILS_MOCK_DEFAULT_RETURN_VALUES).map(([method, value]) => [
      method,
      vi.fn(() => Promise.resolve(value))
    ])
  )
)
