import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { getToken, setToken, clearToken } from '../token'

/**
 * 用例间经 clearToken 复位状态（同时清模块级 memoryToken 兜底与 localStorage），
 * 不用 vi.resetModules 重载模块（避免连带重评估 element-plus 依赖链拖慢并行执行）。
 */

describe('token 单点存取', () => {
  beforeEach(() => {
    clearToken()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    clearToken()
  })

  it('setToken 持久化到 localStorage，getToken 读回一致', () => {
    setToken('tok-abc')
    expect(window.localStorage.getItem('workbench.web.token')).toBe('tok-abc')
    expect(getToken()).toBe('tok-abc')
  })

  it('未设置时 getToken 返回空串', () => {
    expect(getToken()).toBe('')
  })

  it('localStorage 写失败降级为内存态，当前会话仍可读取', () => {
    const original = window.localStorage
    Object.defineProperty(window, 'localStorage', {
      value: {
        getItem: () => null,
        setItem: () => {
          throw new Error('quota exceeded')
        },
        removeItem: () => {}
      },
      configurable: true
    })
    try {
      expect(() => setToken('tok-mem')).not.toThrow()
      expect(getToken()).toBe('tok-mem')
    } finally {
      Object.defineProperty(window, 'localStorage', { value: original, configurable: true })
    }
  })

  it('localStorage 读失败按未设置处理（返回空串）', () => {
    const original = window.localStorage
    original.setItem('workbench.web.token', 'tok-x')
    Object.defineProperty(window, 'localStorage', {
      value: {
        getItem: () => {
          throw new Error('storage blocked')
        },
        setItem: () => {},
        removeItem: () => {}
      },
      configurable: true
    })
    try {
      expect(getToken()).toBe('')
    } finally {
      Object.defineProperty(window, 'localStorage', { value: original, configurable: true })
    }
  })

  it('内存态优先：已设置令牌时不受外部持久化改写影响', () => {
    setToken('tok-1')
    window.localStorage.setItem('workbench.web.token', 'tok-2')
    expect(getToken()).toBe('tok-1')
  })

  it('clearToken 同时清除内存态与持久化', () => {
    setToken('tok-abc')
    clearToken()
    expect(getToken()).toBe('')
    expect(window.localStorage.getItem('workbench.web.token')).toBeNull()
  })
})
