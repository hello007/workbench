import { describe, it, expect } from 'vitest'
import {
  buildTrendOption,
  buildHeatmapOption,
  buildContributorOption,
  HEATMAP_PIECES
} from '../repoStatsOptions'

describe('repoStatsOptions - HEATMAP_PIECES', () => {
  it('应为 5 档 GitHub 色阶', () => {
    expect(HEATMAP_PIECES).toHaveLength(5)
    expect(HEATMAP_PIECES[0]).toMatchObject({ min: 0, max: 0 })
    expect(HEATMAP_PIECES[4]).toMatchObject({ min: 10 })
  })
})

describe('buildTrendOption', () => {
  it('应将 trend 映射为 x/y 数据', () => {
    const trend = [
      { date: '2026-09-10', count: 2 },
      { date: '2026-09-11', count: 5 }
    ]
    const opt = buildTrendOption(trend, 'day')
    expect(opt.xAxis.data).toEqual(['2026-09-10', '2026-09-11'])
    expect(opt.series[0].data).toEqual([2, 5])
    expect(opt.series[0].type).toBe('line')
  })

  it('桶数 > 30 应切换为 bar 类型', () => {
    const trend = Array.from({ length: 31 }, (_, i) => ({ date: `d${i}`, count: i }))
    const opt = buildTrendOption(trend, 'day')
    expect(opt.series[0].type).toBe('bar')
  })

  it('桶数 > 12 时 X 轴标签应旋转 45 度', () => {
    const trend = Array.from({ length: 13 }, (_, i) => ({ date: `d${i}`, count: i }))
    const opt = buildTrendOption(trend, 'day')
    expect(opt.xAxis.axisLabel.rotate).toBe(45)
  })

  it('空数据应返回空序列不报错', () => {
    const opt = buildTrendOption([], 'day')
    expect(opt.xAxis.data).toEqual([])
    expect(opt.series[0].data).toEqual([])
  })

  it('null 输入应兜底为空', () => {
    const opt = buildTrendOption(null, 'day')
    expect(opt.series[0].data).toEqual([])
  })
})

describe('buildHeatmapOption', () => {
  it('应将 heatmap 映射为 [date, count] 序列', () => {
    const heatmap = [
      { date: '2026-09-10', count: 0 },
      { date: '2026-09-11', count: 3 }
    ]
    const opt = buildHeatmapOption(heatmap)
    expect(opt.series[0].data).toEqual([
      ['2026-09-10', 0],
      ['2026-09-11', 3]
    ])
    expect(opt.calendar.range).toEqual(['2026-09-10', '2026-09-11'])
  })

  it('visualMap 应使用 5 档 piecewise', () => {
    const opt = buildHeatmapOption([{ date: '2026-01-01', count: 1 }])
    expect(opt.visualMap.type).toBe('piecewise')
    expect(opt.visualMap.pieces).toHaveLength(5)
  })

  it('series 类型应为 heatmap + calendar 坐标系', () => {
    const opt = buildHeatmapOption([{ date: '2026-01-01', count: 1 }])
    expect(opt.series[0].type).toBe('heatmap')
    expect(opt.series[0].coordinateSystem).toBe('calendar')
  })

  it('空数据应回退默认范围不报错', () => {
    const opt = buildHeatmapOption([])
    expect(opt.series[0].data).toEqual([])
    expect(opt.calendar.range).toHaveLength(2)
  })
})

describe('buildContributorOption', () => {
  it('应将贡献者映射为横向柱状数据（yAxis inverse:true 使最高在顶，不 reverse 数组）', () => {
    const contributors = [
      { author: 'Alice', email: 'a@x.com', count: 10 },
      { author: 'Bob', email: 'b@x.com', count: 5 }
    ]
    const opt = buildContributorOption(contributors)
    // 不 reverse：原序 Alice 在前，靠 yAxis inverse:true 渲染到顶部
    expect(opt.yAxis.data).toEqual(['Alice', 'Bob'])
    expect(opt.series[0].data).toEqual([10, 5])
    expect(opt.series[0].type).toBe('bar')
    expect(opt.yAxis.inverse).toBe(true)
  })

  it('应限制展示数量为 limit', () => {
    const contributors = Array.from({ length: 20 }, (_, i) => ({
      author: `u${i}`,
      email: `u${i}@x.com`,
      count: 20 - i
    }))
    const opt = buildContributorOption(contributors, 10)
    expect(opt.yAxis.data).toHaveLength(10)
  })

  it('空数据应返回空序列不报错', () => {
    const opt = buildContributorOption([])
    expect(opt.series[0].data).toEqual([])
    expect(opt.yAxis.data).toEqual([])
  })

  it('null 输入应兜底为空', () => {
    const opt = buildContributorOption(null)
    expect(opt.series[0].data).toEqual([])
  })
})
