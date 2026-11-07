import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import DashboardView from '../../views/DashboardView.vue'
import { useDirectoryStore } from '../../store'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

// 形状对齐 model.RepoStatus（含工作树三字段 dirtyCount/lastCommitUnix/stashCount）
const nowSec = Math.floor(Date.now() / 1000)
const mockStatuses = [
  { path: '/work/repo-a', name: 'repo-a', branch: 'master', dirty: false, dirtyCount: 0, ahead: 2, behind: 0, hasUpstream: true, detached: false, isRepo: true, missing: false, lastCommitUnix: nowSec - 3000, stashCount: 0 },
  { path: '/work/repo-b', name: 'repo-b', branch: 'feature', dirty: true, dirtyCount: 3, ahead: 0, behind: 1, hasUpstream: true, detached: false, isRepo: true, missing: false, lastCommitUnix: nowSec - 86400 * 40, stashCount: 2 },
  { path: '/work/repo-c', name: 'repo-c', branch: '', dirty: false, dirtyCount: 0, ahead: 0, behind: 0, hasUpstream: false, detached: true, isRepo: true, missing: false, lastCommitUnix: nowSec - 7200, stashCount: 0 },
  { path: '/work/repo-d', name: 'repo-d', branch: 'master', dirty: false, dirtyCount: 0, ahead: 0, behind: 0, hasUpstream: false, detached: false, isRepo: false, missing: true, lastCommitUnix: 0, stashCount: 0 }
]

const mockRepos = [
  { name: 'repo-a', path: '/work/repo-a', summary: '', tags: [], readmeSummary: '', missing: false, hasRemote: true, isGitRepo: true },
  { name: 'repo-e', path: '/work/repo-e', summary: '', tags: [], readmeSummary: '', missing: false, hasRemote: false, isGitRepo: true }
]

// vi.mock 工厂被 hoist，不能引用外部变量；用 vi.hoisted 提升可变的 mock 函数集合。
const { wailsMock } = vi.hoisted(() => ({
  wailsMock: {
    GetDashboardStatuses: vi.fn(),
    RemoveDashboardPin: vi.fn(),
    IsDashboardPinned: vi.fn(),
    GetRepoFilterList: vi.fn(),
    AddDashboardPin: vi.fn()
  }
}))

vi.mock('../../../wailsjs/go/main/App', () => wailsMock)

const defaultStubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" v-bind="$attrs"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal'],
    emits: ['update:modelValue']
  },
  'el-select': {
    template: '<select class="el-select"><slot /></select>',
    props: ['modelValue', 'placeholder', 'size'],
    emits: ['update:modelValue', 'change']
  },
  'el-option': {
    template: '<option :value="value">{{ label }}</option>',
    props: ['label', 'value']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="loading || disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['loading', 'size', 'type', 'disabled', 'icon', 'circle', 'link'],
    emits: ['click']
  },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tooltip': { template: '<span><slot /></span>' },
  'el-tag': {
    template: '<span class="el-tag" :data-type="type"><slot /></span>',
    props: { type: String, size: String }
  },
  'el-empty': { template: '<div class="el-empty" />', props: ['description'] },
  'el-alert': { template: '<div class="el-alert"><slot /></div>', props: ['type', 'title', 'closable'] },
  'el-table': {
    template: '<div class="el-table" :data-count="data.length"><slot /></div>',
    props: ['data'],
    emits: ['row-click'],
    // 经 getter 注入行数据：statuses 为 mount 后异步加载（provide 初值空数组），
    // getter 闭包保证列渲染读到最新 prop 值
    provide() {
      return { stubTableRows: () => this.data }
    }
  },
  'el-table-column': {
    // 按列渲染各行单元格（列内插槽带 row 作用域），供新列渲染断言
    template:
      '<span v-for="(row, i) in stubTableRows()" :key="i" class="el-table__cell" :data-col="label"><slot :row="row" /></span>',
    props: ['label', 'width', 'minWidth', 'align'],
    inject: { stubTableRows: { default: () => () => [] } }
  },
  'el-checkbox': {
    template: '<label class="el-checkbox"><input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'change\', $event.target.checked)" /><slot /></label>',
    props: ['modelValue', 'value', 'disabled'],
    emits: ['change', 'update:modelValue']
  },
  'el-checkbox-group': {
    template: '<div class="el-checkbox-group"><slot /></div>',
    props: ['modelValue'],
    emits: ['update:modelValue', 'change']
  }
}

function createWrapper() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const directoryStore = useDirectoryStore()
  directoryStore.directories = [{ id: 'dir-1', name: '工作目录1', path: '/work' }]
  directoryStore.selectedDirectoryId = 'dir-1'
  return mount(DashboardView, {
    global: { plugins: [pinia], stubs: defaultStubs }
  })
}

describe('DashboardView.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 重置默认返回值（空状态测试可能改写）
    wailsMock.GetDashboardStatuses.mockReturnValue(Promise.resolve(mockStatuses))
    wailsMock.GetRepoFilterList.mockReturnValue(Promise.resolve(mockRepos))
    wailsMock.IsDashboardPinned.mockReturnValue(Promise.resolve(false))
  })

  it('挂载时加载 pin 仓状态', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    expect(wailsMock.GetDashboardStatuses).toHaveBeenCalledTimes(1)
    // 表格接收到 4 行数据
    const table = wrapper.find('.el-table')
    expect(table.attributes('data-count')).toBe('4')
  })

  it('点击行 emit locate 事件（非失效仓库）', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    // 模拟 el-table 触发 row-click 事件带行数据
    const table = wrapper.findComponent({ name: 'ElTable' }) || wrapper.find('.el-table')
    // stub 组件名可能不匹配，直接通过 vm 触发；改用 findComponent 定位 stub
    const tableComp = wrapper.find('.el-table')
    // 直接调组件内部 onRowClick 验证逻辑（绕过 stub 事件传递）
    const vm = wrapper.vm
    // repo-a 非失效 → 应 emit locate
    vm.onRowClick(mockStatuses[0])
    const locateEvents = wrapper.emitted('locate')
    expect(locateEvents).toBeTruthy()
    expect(locateEvents[0][0]).toBe('/work/repo-a')
  })

  it('点击失效仓库行不跳转并弹 warning', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    const vm = wrapper.vm
    // repo-d missing → 不 emit locate，弹 warning
    vm.onRowClick(mockStatuses[3])
    expect(ElMessage.warning).toHaveBeenCalled()
    expect(wrapper.emitted('locate')).toBeFalsy()
  })

  it('空 pin 列表显示空状态', async () => {
    wailsMock.GetDashboardStatuses.mockReturnValue(Promise.resolve([]))
    const wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('添加弹窗扫描工作目录并展示候选仓库', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    const addBtn = wrapper.findAll('button').find(b => b.text().includes('添加仓库'))
    await addBtn.trigger('click')
    await flushPromises()
    expect(wailsMock.GetRepoFilterList).toHaveBeenCalledWith('dir-1')
    // 候选 checkbox 渲染
    const checkboxes = wrapper.findAll('.el-checkbox')
    expect(checkboxes.length).toBe(2)
  })

  it('未选仓库点确认弹 warning 不调用 AddDashboardPin', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    const addBtn = wrapper.findAll('button').find(b => b.text().includes('添加仓库'))
    await addBtn.trigger('click')
    await flushPromises()
    // 未勾选任何项直接点确认
    const confirmBtn = wrapper.findAll('button').find(b => b.text().includes('加入看板'))
    await confirmBtn.trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalled()
    expect(wailsMock.AddDashboardPin).not.toHaveBeenCalled()
  })

  it('orphaned 标记：路径不属于任何已注册工作目录时派生 orphaned=true', async () => {
    // 工作目录 /work；repo-x 路径 /other/repo-x 不属于任何已注册工作目录 → orphaned
    const statuses = [
      { path: '/other/repo-x', name: 'repo-x', branch: 'master', dirty: false, ahead: 0, behind: 0, hasUpstream: false, detached: false, isRepo: true, missing: false },
      { path: '/work/repo-a', name: 'repo-a', branch: 'master', dirty: false, ahead: 0, behind: 0, hasUpstream: false, detached: false, isRepo: true, missing: false }
    ]
    wailsMock.GetDashboardStatuses.mockReturnValue(Promise.resolve(statuses))
    const wrapper = createWrapper()
    await flushPromises()
    const enriched = wrapper.vm.$.setupState.enrichedStatuses
    expect(enriched[0].orphaned).toBe(true)   // /other/repo-x 不属于 /work
    expect(enriched[1].orphaned).toBe(false)  // /work/repo-a 属于 /work
  })

  it('missing 优先于 orphaned：失效仓库即使路径不属于工作目录也不标 orphaned', async () => {
    // repo-m missing=true 且路径 /other/repo-m 不属于 /work，但 missing 优先 → orphaned=false
    const statuses = [
      { path: '/other/repo-m', name: 'repo-m', branch: '', dirty: false, ahead: 0, behind: 0, hasUpstream: false, detached: false, isRepo: true, missing: true }
    ]
    wailsMock.GetDashboardStatuses.mockReturnValue(Promise.resolve(statuses))
    const wrapper = createWrapper()
    await flushPromises()
    const enriched = wrapper.vm.$.setupState.enrichedStatuses
    expect(enriched[0].missing).toBe(true)
    expect(enriched[0].orphaned).toBe(false)
  })

  it('工作区列：N 处改动 warning / 干净 success / 失效仓库降级「-」', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    const cells = wrapper.findAll('.el-table__cell[data-col="工作区"]')
    expect(cells).toHaveLength(4)
    expect(cells[0].text()).toBe('干净')
    expect(cells[0].find('.el-tag').attributes('data-type')).toBe('success')
    // repo-b dirtyCount 3 → 「3 处改动」warning tag
    expect(cells[1].text()).toBe('3 处改动')
    expect(cells[1].find('.el-tag').attributes('data-type')).toBe('warning')
    // repo-d missing → 三列均降级「-」
    expect(cells[3].text()).toBe('-')
  })

  it('最后提交列：相对时间 / 超 30 天转日期 / 零值降级「-」+ hover 完整时间', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    const cells = wrapper.findAll('.el-table__cell[data-col="最后提交"]')
    expect(cells).toHaveLength(4)
    // repo-a 50 分钟前（nowSec-3000）
    expect(cells[0].text()).toBe('50 分钟前')
    // hover title 显完整本地时间
    expect(cells[0].find('.commit-time').attributes('title')).toMatch(/^\d{4}\/\d{1,2}\/\d{1,2}/)
    // repo-b 40 天前 → 转 zh-CN 日期展示
    expect(cells[1].text()).toMatch(/^\d{4}\/\d{1,2}\/\d{1,2}$/)
    // repo-c 2 小时前
    expect(cells[2].text()).toBe('2 小时前')
    // repo-d lastCommitUnix=0（missing/unborn 形态）→ 「-」
    expect(cells[3].text()).toBe('-')
  })

  it('Stash 列：计数 warning tag / 零值 0 / 失效仓库「-」', async () => {
    const wrapper = createWrapper()
    await flushPromises()
    const cells = wrapper.findAll('.el-table__cell[data-col="Stash"]')
    expect(cells).toHaveLength(4)
    expect(cells[0].text()).toBe('0')
    // repo-b stashCount 2 → warning tag 计数
    expect(cells[1].text()).toBe('2')
    expect(cells[1].find('.el-tag').attributes('data-type')).toBe('warning')
    expect(cells[3].text()).toBe('-')
  })
})
