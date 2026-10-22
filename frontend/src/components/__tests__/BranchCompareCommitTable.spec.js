import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BranchCompareCommitTable from '../BranchCompareCommitTable.vue'

// el-table stub：逐行渲染 + 暴露 rowClassName 结果与 row-click（真实列内插槽不渲染，
// 徽标/单元格内容经 vm.helper 层断言，与 DashboardView.spec 的 el-table stub 同模式）
const elTableStub = {
  props: ['data', 'rowClassName'],
  emits: ['row-click'],
  template: `<div class="el-table" :data-count="data.length">
    <div
      v-for="(row, i) in data"
      :key="i"
      class="el-table__row"
      :data-row-class="rowClassName ? rowClassName({ row }) : ''"
      :data-side="row.side"
      @click="$emit('row-click', row)"
    ></div>
  </div>`
}

const stubs = {
  'el-table': elTableStub,
  'el-table-column': { template: '<td />', props: ['label', 'minWidth', 'width'] },
  'el-input': {
    props: ['modelValue', 'placeholder', 'clearable', 'size', 'prefixIcon'],
    emits: ['update:modelValue'],
    template: '<input class="el-input" :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />'
  },
  'el-tag': { template: '<span class="el-tag" :data-type="type"><slot /></span>', props: ['type', 'size'] }
}

const ROWS = [
  {
    key: 'pair-hash',
    a: { hash: 'aaa111', shortHash: 'aaa111', author: 'alice', email: 'a@x.com', date: 1757400000, subject: 'feat: 登录' },
    b: { hash: 'bbb111', shortHash: 'bbb111', author: 'bob', email: 'b@x.com', date: 1757486400, subject: 'feat: 登录' },
    side: 'matched',
    matchedBy: 'hash'
  },
  {
    key: 'pair-patch',
    a: { hash: 'aaa222', shortHash: 'aaa222', author: 'carol', email: 'c@x.com', date: 1757400000, subject: 'refactor: 抽取工具' },
    b: { hash: 'bbb222', shortHash: 'bbb222', author: 'carol', email: 'c@x.com', date: 1757486400, subject: 'refactor: 抽取工具' },
    side: 'matched',
    matchedBy: 'patch-id'
  },
  {
    key: 'a-only',
    a: { hash: 'aaa333', shortHash: 'aaa333', author: 'alice', email: 'a@x.com', date: 1757400000, subject: 'feat: 仅 A' },
    b: null,
    side: 'a-only',
    matchedBy: 'none'
  },
  {
    key: 'b-only',
    a: null,
    b: { hash: 'bbb444', shortHash: 'bbb444', author: 'dave', email: 'd@x.com', date: 1757486400, subject: 'fix: 仅 B' },
    side: 'b-only',
    matchedBy: 'none'
  }
]

const mountTable = (props = {}) => mount(BranchCompareCommitTable, {
  props: { rows: ROWS, branchA: 'main', branchB: 'feature/x', ...props },
  global: { stubs }
})

describe('BranchCompareCommitTable.vue', () => {
  it('渲染全部对齐行且行侧别 class 正确（matched 灰 / a-only / b-only）', () => {
    const wrapper = mountTable()
    const rows = wrapper.findAll('.el-table__row')
    expect(rows).toHaveLength(4)
    expect(rows[0].attributes('data-row-class')).toBe('row-matched')
    expect(rows[2].attributes('data-row-class')).toBe('row-a-only')
    expect(rows[3].attributes('data-row-class')).toBe('row-b-only')
  })

  it('匹配徽标按匹配方式分级：hash=info 同哈希 / patch-id=warning 疑似拣选 / subject=info', () => {
    const wrapper = mountTable()
    expect(wrapper.vm.badgeOf({ side: 'matched', matchedBy: 'hash' })).toEqual({ type: 'info', label: '同哈希' })
    expect(wrapper.vm.badgeOf({ side: 'matched', matchedBy: 'patch-id' })).toEqual({ type: 'warning', label: '疑似拣选' })
    expect(wrapper.vm.badgeOf({ side: 'matched', matchedBy: 'subject' })).toEqual({ type: 'info', label: '同说明' })
    // 非匹配行无徽标
    expect(wrapper.vm.badgeOf({ side: 'a-only', matchedBy: 'none' })).toBeNull()
    expect(wrapper.vm.badgeOf({ side: 'b-only', matchedBy: 'none' })).toBeNull()
  })

  it('side 标签中文映射', () => {
    const wrapper = mountTable()
    expect(wrapper.vm.sideLabel('matched')).toBe('匹配')
    expect(wrapper.vm.sideLabel('a-only')).toBe('A 独有')
    expect(wrapper.vm.sideLabel('b-only')).toBe('B 独有')
  })

  it('搜索按 hash / subject / author 子串过滤，任一侧命中即整行显示', async () => {
    const wrapper = mountTable()
    // 作者命中（alice 同时出现在 pair-hash 的 a 侧与 a-only 的 a 侧）
    await wrapper.find('input.el-input').setValue('alice')
    expect(wrapper.vm.visibleRows.map(r => r.key)).toEqual(['pair-hash', 'a-only'])

    // subject 命中（b 侧独有提交的说明）
    await wrapper.find('input.el-input').setValue('仅 B')
    expect(wrapper.vm.visibleRows.map(r => r.key)).toEqual(['b-only'])

    // hash 命中
    await wrapper.find('input.el-input').setValue('bbb222')
    expect(wrapper.vm.visibleRows.map(r => r.key)).toEqual(['pair-patch'])

    // 大小写不敏感
    await wrapper.find('input.el-input').setValue('ALICE')
    expect(wrapper.vm.visibleRows.map(r => r.key)).toEqual(['pair-hash', 'a-only'])

    // 清空恢复全量
    await wrapper.find('input.el-input').setValue('')
    expect(wrapper.vm.visibleRows).toHaveLength(4)
  })

  it('行点击 emit selectCommit 并携带行对象', async () => {
    const wrapper = mountTable()
    await wrapper.find('.el-table__row').trigger('click')
    expect(wrapper.emitted('selectCommit')).toBeTruthy()
    expect(wrapper.emitted('selectCommit')[0][0].key).toBe('pair-hash')
  })

  it('rows 为空时计数为 0/0', () => {
    const wrapper = mountTable({ rows: [] })
    expect(wrapper.find('.el-table').attributes('data-count')).toBe('0')
    expect(wrapper.find('.row-count').text()).toBe('0/0')
  })
})
