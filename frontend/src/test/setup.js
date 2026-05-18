/**
 * Vitest 测试环境设置
 */

import { vi } from 'vitest'

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
  PullRepo: vi.fn(() => Promise.resolve('Success'))
}))
