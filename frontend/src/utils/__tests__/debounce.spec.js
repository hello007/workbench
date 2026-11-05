/**
 * debounce 工具测试（fake timers 驱动时序）
 * 覆盖：尾部触发合并、窗口重置、cancel 取消防抖回调、参数携带
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { debounce } from '../debounce'

describe('debounce', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('窗口内连续调用合并为尾部一次执行（100ms 防抖收口）', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 100)
    debounced()
    vi.advanceTimersByTime(50)
    debounced()
    vi.advanceTimersByTime(50)
    debounced()
    // 窗口未到期：零执行
    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(100)
    // 尾部收口：仅一次
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('尾部触发携带最后一次调用参数', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 100)
    debounced('a')
    debounced('b')
    debounced('c')
    vi.advanceTimersByTime(100)
    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith('c')
  })

  it('cancel 取消防抖：不执行挂起回调（卸载路径防悬挂）', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 100)
    debounced('x')
    debounced.cancel()
    vi.advanceTimersByTime(500)
    expect(fn).not.toHaveBeenCalled()
    // cancel 后可重新防抖（幂等可复用）
    debounced('y')
    vi.advanceTimersByTime(100)
    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith('y')
  })

  it('cancel 后重复调用幂等安全', () => {
    const fn = vi.fn()
    const debounced = debounce(fn, 100)
    debounced.cancel()
    debounced.cancel()
    vi.advanceTimersByTime(200)
    expect(fn).not.toHaveBeenCalled()
  })
})
