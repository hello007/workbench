import { describe, it, expect } from 'vitest'
import { isTextOverflowing } from '../todoTextOverflow'

const fakeEl = (clientHeight, scrollHeight) => ({ clientHeight, scrollHeight })

describe('isTextOverflowing', () => {
  it('元素为空时视为未截断', () => {
    expect(isTextOverflowing(null)).toBe(false)
    expect(isTextOverflowing(undefined)).toBe(false)
  })

  it('元素未布局（clientHeight=0）时视为未截断', () => {
    expect(isTextOverflowing(fakeEl(0, 120))).toBe(false)
  })

  it('内容高度大于可见高度时判定为截断', () => {
    expect(isTextOverflowing(fakeEl(60, 120))).toBe(true)
  })

  it('内容高度等于可见高度时判定为未截断', () => {
    expect(isTextOverflowing(fakeEl(60, 60))).toBe(false)
  })

  it('仅相差 1px 的亚像素误差不判定为截断', () => {
    expect(isTextOverflowing(fakeEl(60, 61))).toBe(false)
  })

  it('scrollHeight 缺失时按 0 处理，不判定为截断', () => {
    expect(isTextOverflowing({ clientHeight: 60 })).toBe(false)
  })
})
