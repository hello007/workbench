import { describe, it, expect, vi, afterEach } from 'vitest'
import { formatRelativeTime, formatFullTime } from '../formatTime'

// 固定系统时间驱动相对时间边界（禁 sleep 等待），afterEach 恢复真实时钟
afterEach(() => {
  vi.useRealTimers()
})

const NOW = new Date('2026-11-07T12:00:00')
const secAt = (iso) => Math.floor(new Date(iso).getTime() / 1000)

describe('utils/formatTime', () => {
  describe('formatRelativeTime', () => {
    it('无数据输入（0/undefined）降级占位符', () => {
      vi.useFakeTimers()
      vi.setSystemTime(NOW)
      expect(formatRelativeTime(0)).toBe('-')
      expect(formatRelativeTime(undefined)).toBe('-')
    })

    it('1 小时内按分钟展示', () => {
      vi.useFakeTimers()
      vi.setSystemTime(NOW)
      expect(formatRelativeTime(secAt('2026-11-07T11:55:00'))).toBe('5 分钟前')
      expect(formatRelativeTime(secAt('2026-11-07T11:01:00'))).toBe('59 分钟前')
    })

    it('1 小时到 24 小时按小时展示', () => {
      vi.useFakeTimers()
      vi.setSystemTime(NOW)
      expect(formatRelativeTime(secAt('2026-11-07T11:00:00'))).toBe('1 小时前')
      expect(formatRelativeTime(secAt('2026-11-06T13:00:00'))).toBe('23 小时前')
    })

    it('24 小时到 30 天按天展示', () => {
      vi.useFakeTimers()
      vi.setSystemTime(NOW)
      expect(formatRelativeTime(secAt('2026-11-06T12:00:00'))).toBe('1 天前')
      expect(formatRelativeTime(secAt('2026-10-09T12:00:00'))).toBe('29 天前')
    })

    it('超 30 天转 zh-CN 日期', () => {
      vi.useFakeTimers()
      vi.setSystemTime(NOW)
      expect(formatRelativeTime(secAt('2026-10-08T12:00:00'))).toMatch(/^\d{4}\/\d{1,2}\/\d{1,2}$/)
      expect(formatRelativeTime(secAt('2025-01-01T00:00:00'))).toMatch(/^2025\//)
    })
  })

  describe('formatFullTime', () => {
    it('输出含年月日时分秒的本地时间串', () => {
      vi.useFakeTimers()
      vi.setSystemTime(NOW)
      const full = formatFullTime(secAt('2026-11-07T11:55:00'))
      expect(full).toMatch(/^2026\/11\/7\s/)
    })

    it('无数据输入降级占位符', () => {
      expect(formatFullTime(0)).toBe('-')
    })
  })
})
