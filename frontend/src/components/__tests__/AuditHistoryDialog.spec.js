import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

// 覆盖面：打开即拉令牌名册 + 审计行（时间格式化/事件标签/来源/操作者与目标令牌
// 名映射、已撤销降级、未知事件回退）、事件过滤与时间窗变更回第 1 页重查（查询
// 参数 Events/From 断言）、分页 Offset 透传与越界页回退末页、查询失败 handleError
// 分流（弹窗不崩）。
// 后端读侧过滤分页/坏行跳过语义由 service 层测试覆盖（audit_log_test.go），本
// spec 专注组件渲染与参数编排。

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../utils/error', () => ({
  handleError: vi.fn()
}))

import { QueryAuditLog, ListWebTokens } from '../../../wailsjs/go/main/App'
import { handleError } from '../../utils/error'
import AuditHistoryDialog from '../AuditHistoryDialog.vue'

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="stub-dialog"><slot /></div>',
    props: ['modelValue', 'title', 'width', 'top'],
    emits: ['update:modelValue', 'open'],
    // 真实 el-dialog 打开时 emit open；stub 以 mounted（初始即开）+ watch 覆盖
    // 两种进入路径
    mounted() {
      if (this.modelValue) this.$emit('open')
    },
    watch: {
      modelValue(v) {
        if (v) this.$emit('open')
      }
    }
  },
  'el-button': {
    template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'type', 'size', 'loading', 'text'],
    emits: ['click']
  },
  'el-table': {
    template: '<div class="el-table"><slot /></div>',
    props: ['data', 'size', 'emptyText']
  },
  // 行 stub：遍历 el-table 的 data 逐行渲染作用域插槽（AssetBackupDialog.spec
  // 同款 $parent.data 形态；本组件全部列走 #default 模板列）
  'el-table-column': {
    props: ['label', 'width', 'minWidth'],
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><div v-for="(row, i) in $parent.data" :key="i" class="table-row"><slot :row="row" /></div></div>'
  },
  'el-tag': {
    template: '<span class="el-tag"><slot /></span>',
    props: ['size', 'type']
  },
  'el-select': {
    template: '<div class="stub-select"><slot /></div>',
    props: ['modelValue', 'multiple', 'collapseTags', 'collapseTagsTooltip', 'clearable', 'placeholder', 'size'],
    emits: ['update:modelValue', 'change']
  },
  'el-option': {
    template: '<div class="stub-option"><slot /></div>',
    props: ['label', 'value']
  },
  'el-pagination': {
    template: '<div class="stub-pagination" />',
    props: ['currentPage', 'pageSize', 'total', 'pageSizes', 'layout', 'small'],
    emits: ['current-change', 'size-change', 'update:currentPage', 'update:pageSize']
  }
}

const mockTokens = [
  { record: { id: 'tok-default', name: '默认', hash: '0'.repeat(64), createdAt: 1, lastUsedAt: 0, expiresAt: 0, revoked: false }, onlineConns: 0 },
  { record: { id: 'tok-phone', name: '手机', hash: '1'.repeat(64), createdAt: 1, lastUsedAt: 0, expiresAt: 0, revoked: false }, onlineConns: 0 }
]

const mockRecords = [
  { ts: 1735689700, event: 'conn_kick', tokenId: 'tok-default', name: '默认', remoteAddr: '192.168.1.5', connId: 1, actorSource: 'desktop' },
  { ts: 1735689650, event: 'conn_kick', tokenId: 'tok-phone', name: '手机', remoteAddr: '192.168.1.6', connId: 2, actorSource: 'rpc', actorTokenId: 'tok-default' },
  { ts: 1735689600, event: 'auth_success', tokenId: 'tok-phone', remoteAddr: '192.168.1.6', userAgent: 'Mozilla/5.0' }
]

const createWrapper = async () => {
  const wrapper = mount(AuditHistoryDialog, {
    props: { modelValue: true },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

const rows = (wrapper) => wrapper.findAll('.table-row')

// 按列标签取该列文本（列 stub 各自渲染行，跨列断言须经 col-label 定位列容器）
const colText = (wrapper, label) => {
  const col = wrapper.findAll('.el-table-col').find((c) => c.find('.col-label').text() === label)
  return col ? col.text() : ''
}

beforeEach(() => {
  vi.clearAllMocks()
  QueryAuditLog.mockResolvedValue({ Total: mockRecords.length, Records: mockRecords })
  ListWebTokens.mockResolvedValue(mockTokens)
})

describe('AuditHistoryDialog', () => {
  it('打开即拉令牌名册与审计行，操作者/目标令牌名映射渲染', async () => {
    const wrapper = await createWrapper()

    expect(ListWebTokens).toHaveBeenCalled()
    expect(QueryAuditLog).toHaveBeenCalledTimes(1)
    const arg = QueryAuditLog.mock.calls[0][0]
    expect(arg).toMatchObject({ Events: [], From: 0, To: 0, Offset: 0, Limit: 20 })

    const text = wrapper.text()
    // 桌面踢出行：操作者=桌面用户、目标=名册令牌名
    expect(text).toContain('桌面用户')
    // RPC 踢出行：操作者经名册映射=默认
    expect(text).toContain('浏览器')
    // 事件标签与来源
    expect(text).toContain('踢出连接')
    expect(text).toContain('认证成功')
    expect(text).toContain('桌面')
  })

  it('名册 miss 降级「已撤销令牌」，未知事件回退原文', async () => {
    QueryAuditLog.mockResolvedValue({
      Total: 2,
      Records: [
        { ts: 1735689700, event: 'conn_kick', tokenId: 'tok-gone', name: '', remoteAddr: '10.0.0.1', connId: 3, actorSource: 'rpc', actorTokenId: 'tok-gone' },
        { ts: 1735689600, event: 'brand_new_event', tokenId: '', remoteAddr: '' }
      ]
    })
    const wrapper = await createWrapper()

    const actorCol = colText(wrapper, '操作者')
    const targetCol = colText(wrapper, '目标令牌')
    // RPC 踢出行：操作者与目标令牌均名册 miss → 已撤销令牌降级
    expect(actorCol).toContain('已撤销令牌')
    expect(targetCol).toContain('已撤销令牌')
    // 未知事件回退原文展示
    expect(colText(wrapper, '事件')).toContain('brand_new_event')
  })

  it('事件过滤变更：Events 透传 + 回第 1 页', async () => {
    const wrapper = await createWrapper()
    QueryAuditLog.mockClear()

    const selects = wrapper.findAllComponents('.stub-select')
    // 事件多选下拉（第一个 stub-select）：v-model 更新 + change 触发
    const eventsSelect = selects[0]
    eventsSelect.vm.$emit('update:modelValue', ['conn_kick'])
    eventsSelect.vm.$emit('change', ['conn_kick'])
    await flushPromises()

    expect(QueryAuditLog).toHaveBeenCalledTimes(1)
    const arg = QueryAuditLog.mock.calls[0][0]
    expect(arg.Events).toEqual(['conn_kick'])
    expect(arg.Offset).toBe(0)
  })

  it('时间窗变更：From 按当前时间换算（相对档）', async () => {
    vi.useFakeTimers()
    try {
      vi.setSystemTime(new Date('2026-10-24T12:00:00') )
      const wrapper = await createWrapper()
      QueryAuditLog.mockClear()

      const selects = wrapper.findAllComponents('.stub-select')
      const timeSelect = selects[1]
      timeSelect.vm.$emit('update:modelValue', 24 * 3600)
      timeSelect.vm.$emit('change', 24 * 3600)
      await flushPromises()

      const arg = QueryAuditLog.mock.calls[0][0]
      expect(arg.From).toBe(Math.floor(Date.now() / 1000) - 24 * 3600)
    } finally {
      vi.useRealTimers()
    }
  })

  it('翻页：Offset 按 (page-1)*size 透传', async () => {
    const wrapper = await createWrapper()
    QueryAuditLog.mockClear()

    const pager = wrapper.findComponent('.stub-pagination')
    pager.vm.$emit('update:currentPage', 3)
    pager.vm.$emit('current-change', 3)
    await flushPromises()

    const arg = QueryAuditLog.mock.calls[0][0]
    expect(arg.Offset).toBe(40)
    expect(arg.Limit).toBe(20)
  })

  it('越界页回退末页重查（过滤变严后 Total 收缩）', async () => {
    // mockReset 清 Once 队列后按调用序编排：挂载首查 → 第 3 页越界（收缩空页）
    // → 回退第 1 页重查
    QueryAuditLog.mockReset()
    QueryAuditLog.mockResolvedValueOnce({ Total: 60, Records: mockRecords })
    QueryAuditLog.mockResolvedValueOnce({ Total: 1, Records: [] })
    QueryAuditLog.mockResolvedValue({ Total: 1, Records: mockRecords.slice(0, 1) })
    const wrapper = await createWrapper()
    QueryAuditLog.mockClear()

    const pager = wrapper.findComponent('.stub-pagination')
    pager.vm.$emit('update:currentPage', 3)
    pager.vm.$emit('current-change', 3)
    await flushPromises()

    expect(QueryAuditLog).toHaveBeenCalledTimes(2)
    const second = QueryAuditLog.mock.calls[1][0]
    expect(second.Offset).toBe(0)
  })

  it('查询失败 handleError 分流，弹窗不崩', async () => {
    // mockReset 清残留 Once 队列（前序用例未消费的编排不受 clearAllMocks 清除）
    QueryAuditLog.mockReset()
    QueryAuditLog.mockRejectedValue(new Error('read failed'))
    const wrapper = await createWrapper()

    expect(handleError).toHaveBeenCalled()
    expect(wrapper.find('.stub-dialog').exists()).toBe(true)
  })
})

// ---- IP 自动封禁事件展示（连接管控五期：ip_ban/ip_unban 两事件 + system 归因）----

describe('AuditHistoryDialog - IP 自动封禁事件', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    QueryAuditLog.mockResolvedValue({ Total: 2, Records: [
      {
        ts: 1735689700,
        event: 'ip_ban',
        ip: '203.0.113.7',
        failCount: 10,
        expiresAt: 1735689900,
        reason: '10 分钟内认证失败 10 次，自动临时封禁',
        actorSource: 'system'
      },
      { ts: 1735689800, event: 'ip_unban', ip: '203.0.113.7', actorSource: 'desktop' }
    ] })
    ListWebTokens.mockResolvedValue(mockTokens)
  })

  it('ip_ban/ip_unban 行渲染：事件标签、system 归因来源与操作者、IP 列含失败计数', async () => {
    const wrapper = await createWrapper()

    const text = wrapper.text()
    expect(text).toContain('自动封禁IP')
    expect(text).toContain('解封IP')
    // 来源/操作者列：system → 系统（自动触发无人类操作者）；解封行 desktop → 桌面/桌面用户
    expect(colText(wrapper, '来源')).toContain('系统')
    expect(colText(wrapper, '操作者')).toContain('系统')
    expect(colText(wrapper, '操作者')).toContain('桌面用户')
    // 来源 IP 列：ip_ban 带 ip 字段（剥端口归一形态）+ 失败计数；ip_unban 仅 IP
    const ipCol = colText(wrapper, '来源 IP')
    expect(ipCol).toContain('203.0.113.7（失败 10 次）')
    expect(ipCol).toContain('203.0.113.7')
  })

  it('过滤下拉登记 ip_ban/ip_unban 选项（审计历史可过滤新事件）', async () => {
    const wrapper = await createWrapper()
    const values = wrapper.findAllComponents('.stub-option').map((o) => o.props('value'))
    expect(values).toContain('ip_ban')
    expect(values).toContain('ip_unban')
  })

  it('按 ip_ban 过滤：Events 透传新事件值', async () => {
    const wrapper = await createWrapper()
    QueryAuditLog.mockClear()

    const eventsSelect = wrapper.findAllComponents('.stub-select')[0]
    eventsSelect.vm.$emit('update:modelValue', ['ip_ban'])
    eventsSelect.vm.$emit('change', ['ip_ban'])
    await flushPromises()

    expect(QueryAuditLog).toHaveBeenCalledTimes(1)
    expect(QueryAuditLog.mock.calls[0][0].Events).toEqual(['ip_ban'])
  })
})
