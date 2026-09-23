import { describe, it, expect } from 'vitest'
import {
  buildTrendOption,
  buildHeatmapOption,
  buildContributorOption,
  buildContributorLineOption,
  buildPathLineOption,
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

describe('buildContributorLineOption', () => {
  it('应按总变更行数（新增+删除）降序排序', () => {
    const contributors = [
      { author: 'Alice', email: 'a@x.com', count: 1, insertions: 10, deletions: 5 },
      { author: 'Bob', email: 'b@x.com', count: 9, insertions: 100, deletions: 20 },
      { author: 'Carol', email: 'c@x.com', count: 5, insertions: 3, deletions: 2 }
    ]
    const opt = buildContributorLineOption(contributors)
    // Bob 120 > Alice 15 > Carol 5（提交数排序不影响行数视图重排）
    expect(opt.yAxis.data).toEqual(['Bob', 'Alice', 'Carol'])
    expect(opt.series[0].data).toEqual([120, 15, 5])
    expect(opt.series[0].type).toBe('bar')
    expect(opt.yAxis.inverse).toBe(true)
  })

  it('总变更相同时应按作者名升序（与后端 AggregateLineStats 排序规则一致）', () => {
    const contributors = [
      { author: 'carol', email: 'c@x.com', count: 1, insertions: 10, deletions: 0 },
      { author: 'bob', email: 'b@x.com', count: 1, insertions: 5, deletions: 5 },
      { author: 'alice', email: 'a@x.com', count: 1, insertions: 2, deletions: 8 }
    ]
    const opt = buildContributorLineOption(contributors)
    expect(opt.yAxis.data).toEqual(['alice', 'bob', 'carol'])
  })

  it('排序应在副本上做，不修改入参数组（props 不可原地变更）', () => {
    const contributors = [
      { author: 'Alice', email: 'a@x.com', count: 1, insertions: 10, deletions: 5 },
      { author: 'Bob', email: 'b@x.com', count: 9, insertions: 100, deletions: 20 }
    ]
    buildContributorLineOption(contributors)
    expect(contributors.map(c => c.author)).toEqual(['Alice', 'Bob'])
  })

  it('tooltip 应展示 +新增/−删除 细分', () => {
    const contributors = [
      { author: 'Alice', email: 'a@x.com', count: 1, insertions: 120, deletions: 30 }
    ]
    const opt = buildContributorLineOption(contributors)
    const html = opt.tooltip.formatter([{ dataIndex: 0 }])
    expect(html).toContain('+120 新增')
    expect(html).toContain('−30 删除')
    expect(html).toContain('总变更: <b>150</b>')
    expect(html).toContain('a@x.com')
  })

  it('行数字段缺失应兜底为 0 不报错', () => {
    const contributors = [{ author: 'Legacy', email: 'l@x.com', count: 3 }]
    const opt = buildContributorLineOption(contributors)
    expect(opt.series[0].data).toEqual([0])
    const html = opt.tooltip.formatter([{ dataIndex: 0 }])
    expect(html).toContain('+0 新增')
    expect(html).toContain('−0 删除')
  })

  it('应限制展示数量为 limit（排序后再截断，保留总变更最高者）', () => {
    const contributors = [
      { author: 'u0', email: 'u0@x.com', count: 1, insertions: 1, deletions: 0 },
      { author: 'u1', email: 'u1@x.com', count: 1, insertions: 500, deletions: 0 },
      { author: 'u2', email: 'u2@x.com', count: 1, insertions: 2, deletions: 0 }
    ]
    const opt = buildContributorLineOption(contributors, 2)
    expect(opt.yAxis.data).toEqual(['u1', 'u2'])
  })

  it('空数据应返回空序列不报错', () => {
    const opt = buildContributorLineOption([])
    expect(opt.series[0].data).toEqual([])
    expect(opt.yAxis.data).toEqual([])
  })

  it('null 输入应兜底为空', () => {
    const opt = buildContributorLineOption(null)
    expect(opt.series[0].data).toEqual([])
  })
})

describe('buildPathLineOption', () => {
  it('应保留后端排序不重排，柱值为总变更行数', () => {
    const entries = [
      { path: 'service', insertions: 33, deletions: 13 },
      { path: 'web', insertions: 5, deletions: 1 },
      { path: '(根目录)', insertions: 1, deletions: 0 }
    ]
    const opt = buildPathLineOption(entries)
    expect(opt.yAxis.data).toEqual(['service', 'web', '(根目录)'])
    expect(opt.series[0].data).toEqual([46, 6, 1])
    expect(opt.series[0].type).toBe('bar')
    expect(opt.yAxis.inverse).toBe(true)
  })

  it('tooltip 应展示路径与 +新增/−删除 细分', () => {
    const entries = [{ path: 'service/a.go', insertions: 10, deletions: 2 }]
    const opt = buildPathLineOption(entries)
    const html = opt.tooltip.formatter([{ dataIndex: 0 }])
    expect(html).toContain('service/a.go')
    expect(html).toContain('总变更: <b>12</b>')
    expect(html).toContain('+10 新增')
    expect(html).toContain('−2 删除')
  })

  it('tooltip 路径应转义 HTML 特殊字符', () => {
    const entries = [{ path: 'a<b>&"c.go', insertions: 1, deletions: 0 }]
    const opt = buildPathLineOption(entries)
    const html = opt.tooltip.formatter([{ dataIndex: 0 }])
    expect(html).toContain('a&lt;b&gt;&amp;&quot;c.go')
    expect(html).not.toContain('a<b>')
  })

  it('超长路径轴标签应中段截断，短路径原样', () => {
    const longPath = 'verylongdirectoryname/sub/anotherlongname/file.go'
    const opt = buildPathLineOption([{ path: longPath, insertions: 1, deletions: 0 }, { path: 'short.go', insertions: 1, deletions: 0 }])
    const labels = opt.yAxis.data.map(v => opt.yAxis.axisLabel.formatter(v))
    expect(labels[0]).toBe(longPath.slice(0, 12) + '…' + longPath.slice(-11))
    expect(labels[0]).not.toBe(longPath)
    expect(labels[1]).toBe('short.go')
  })

  it('行数字段缺失应兜底为 0 不报错', () => {
    const opt = buildPathLineOption([{ path: 'legacy.go' }])
    expect(opt.series[0].data).toEqual([0])
  })

  it('空数组与 null 输入应返回空序列不报错', () => {
    expect(buildPathLineOption([]).series[0].data).toEqual([])
    expect(buildPathLineOption(null).series[0].data).toEqual([])
  })
})
