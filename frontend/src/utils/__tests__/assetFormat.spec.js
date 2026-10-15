import { describe, it, expect } from 'vitest'
import { assetStatusLabel, assetStatusTagType, formatAssetSize } from '../assetFormat'

describe('assetFormat', () => {
  it('状态文案映射覆盖 model/assets.go 全部状态机取值，未知值原样回显', () => {
    expect(assetStatusLabel('processing')).toBe('解析中')
    expect(assetStatusLabel('ok')).toBe('正常')
    expect(assetStatusLabel('failed')).toBe('解析失败')
    expect(assetStatusLabel('encrypted')).toBe('已加密')
    expect(assetStatusLabel('denied')).toBe('权限不足')
    // 未知状态原样回显（后端新增状态时前端不至于渲染空串）
    expect(assetStatusLabel('unknown')).toBe('unknown')
  })

  it('状态标签语义色映射：解析中 info / 正常 success / 解析失败 danger / 加密与权限 warning，未知回退 info', () => {
    expect(assetStatusTagType('processing')).toBe('info')
    expect(assetStatusTagType('ok')).toBe('success')
    expect(assetStatusTagType('failed')).toBe('danger')
    expect(assetStatusTagType('encrypted')).toBe('warning')
    expect(assetStatusTagType('denied')).toBe('warning')
    expect(assetStatusTagType('unknown')).toBe('info')
  })

  it('大小格式化：0/负数归零，1024 进位，非整数值保留一位小数，GB 为上限档', () => {
    expect(formatAssetSize(0)).toBe('0 B')
    expect(formatAssetSize(-5)).toBe('0 B')
    expect(formatAssetSize(undefined)).toBe('0 B')
    expect(formatAssetSize(512)).toBe('512 B')
    expect(formatAssetSize(1024)).toBe('1 KB')
    expect(formatAssetSize(1024 * 1024)).toBe('1 MB')
    expect(formatAssetSize(1.5 * 1024 * 1024)).toBe('1.5 MB')
    // 超过 GB 的值停在 GB 档继续放大（不新增 TB 档）
    expect(formatAssetSize(2 * 1024 * 1024 * 1024)).toBe('2 GB')
    expect(formatAssetSize(3.2 * 1024 * 1024 * 1024)).toBe('3.2 GB')
  })
})
