/**
 * Vitest 测试环境设置
 */

import { vi } from 'vitest'

// jsdom 缺失的浏览器 API 补丁：
// - ResizeObserver：@codemirror/view 顶层模块引用，否则加载即抛 ReferenceError
// - DOMMatrix：兜底防御（曾为 pdfjs-dist 准备，pdfjs 已移除但保留 stub 无副作用）
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

// Mock Wails绑定
vi.mock('../../wailsjs/go/main/App', () => ({
  GetDirectories: vi.fn(() => Promise.resolve([])),
  AddDirectory: vi.fn(() => Promise.resolve({ id: 'dir-test', name: 'test', path: '/test', isDefault: false })),
  UpdateDirectory: vi.fn(() => Promise.resolve(true)),
  DeleteDirectory: vi.fn(() => Promise.resolve(true)),
  SetDefaultDirectory: vi.fn(() => Promise.resolve(true)),
  GetFileTree: vi.fn(() => Promise.resolve([])),
  GetGitInfo: vi.fn(() => Promise.resolve({})),
  CreateDirectory: vi.fn(() => Promise.resolve(true)),
  CreateFile: vi.fn(() => Promise.resolve(true)),
  RenameFile: vi.fn(() => Promise.resolve(true)),
  DeleteFile: vi.fn(() => Promise.resolve(true)),
  PreviewFile: vi.fn(() => Promise.resolve({ content: '', error: '' })),
  PullRepo: vi.fn(() => Promise.resolve('Success')),
  ScanAndPullRepos: vi.fn(() => Promise.resolve({ total: 0 })),
  GetAppVersion: vi.fn(() => Promise.resolve('dev')),
  OpenInWarp: vi.fn(() => Promise.resolve(true)),
  OpenWithDefaultApp: vi.fn(() => Promise.resolve(true)),
  CopyTo: vi.fn(() => Promise.resolve('')),
  GetFavorites: vi.fn(() => Promise.resolve([])),
  AddFavorite: vi.fn(() => Promise.resolve('')),
  RemoveFavorite: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(''))
}))
