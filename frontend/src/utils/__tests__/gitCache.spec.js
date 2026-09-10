import { describe, it, expect, beforeEach, vi } from 'vitest'
import { gitCache, getCacheKey } from '../gitCache'

describe('gitCache', () => {
  beforeEach(() => {
    gitCache.clear()
    vi.useRealTimers()
  })

  it('set/get 正常存取', () => {
    gitCache.set('k', 'v')
    expect(gitCache.get('k')).toBe('v')
  })

  it('get 不存在返回 null', () => {
    expect(gitCache.get('missing')).toBeNull()
  })

  it('get 过期返回 null 并删除条目', () => {
    vi.useFakeTimers()
    gitCache.set('k', 'v')
    vi.advanceTimersByTime(6 * 60 * 1000) // 6 分钟 > 5 分钟过期
    expect(gitCache.get('k')).toBeNull()
    // 二次 get 仍为 null（已删除）
    expect(gitCache.get('k')).toBeNull()
    vi.useRealTimers()
  })

  it('get 未过期返回值', () => {
    vi.useFakeTimers()
    gitCache.set('k', 'v')
    vi.advanceTimersByTime(4 * 60 * 1000) // 4 分钟 < 5 分钟
    expect(gitCache.get('k')).toBe('v')
    vi.useRealTimers()
  })

  it('clear 清空所有条目', () => {
    gitCache.set('k1', 'v1')
    gitCache.set('k2', 'v2')
    gitCache.clear()
    expect(gitCache.get('k1')).toBeNull()
    expect(gitCache.get('k2')).toBeNull()
  })

  it('delete 删除指定键', () => {
    gitCache.set('k', 'v')
    gitCache.delete('k')
    expect(gitCache.get('k')).toBeNull()
  })

  it('覆盖同 key 的值', () => {
    gitCache.set('k', 'v1')
    gitCache.set('k', 'v2')
    expect(gitCache.get('k')).toBe('v2')
  })
})

describe('getCacheKey', () => {
  it('type:path 格式拼接', () => {
    expect(getCacheKey('info', '/work/repo')).toBe('info:/work/repo')
  })
  it('空参数返回冒号', () => {
    expect(getCacheKey('', '')).toBe(':')
  })
})
