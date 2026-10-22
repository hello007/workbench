import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BranchCompareDiffViewer from '../BranchCompareDiffViewer.vue'

const stubs = {
  'el-radio-group': { template: '<div class="el-radio-group"><slot /></div>', props: ['modelValue', 'size'] },
  'el-radio-button': { template: '<span class="el-radio-btn" />', props: ['value'] }
}

const mountViewer = (props = {}) => mount(BranchCompareDiffViewer, {
  props,
  global: { stubs }
})

const SAMPLE_DIFF = [
  'diff --git a/src/a.js b/src/a.js',
  'index 1111111..2222222 100644',
  '--- a/src/a.js',
  '+++ b/src/a.js',
  '@@ -1,3 +1,3 @@',
  ' context line',
  '-added line',
  '+new line',
  '+tail line',
  '\\ No newline at end of file'
].join('\n')

describe('BranchCompareDiffViewer.vue', () => {
  it('默认双栏视图：解析后 hunk/context/del/add 分栏渲染且跳过文件头', () => {
    const wrapper = mountViewer({ diffText: SAMPLE_DIFF, baseLabel: 'A', targetLabel: 'B' })
    const left = wrapper.find('.diff-col-left')
    const right = wrapper.find('.diff-col-right')
    expect(left.exists()).toBe(true)
    // 左栏：hunk + context + del + 2×empty 占位 = 5 行；文件头 4 行不进双栏
    expect(left.findAll('.diff-line')).toHaveLength(5)
    expect(left.findAll('.diff-line-del')).toHaveLength(1)
    expect(left.findAll('.diff-line-context')).toHaveLength(1)
    // 右栏：hunk + empty + 2×add + empty 占位 = 5 行
    expect(right.findAll('.diff-line-add')).toHaveLength(2)
    // "No newline" 提示行不渲染
    expect(wrapper.text()).not.toContain('No newline')
    // 列头为分支名
    expect(wrapper.find('.diff-col-left .diff-col-header').text()).toBe('A')
    expect(wrapper.find('.diff-col-right .diff-col-header').text()).toBe('B')
  })

  it('统一视图：del/add/context 按原 diff 顺序合并（hunk 头一并保留）', async () => {
    const wrapper = mountViewer({ diffText: SAMPLE_DIFF })
    wrapper.vm.viewMode = 'unified'
    await nextTick()
    const unified = wrapper.find('.diff-unified')
    expect(unified.exists()).toBe(true)
    const lines = unified.findAll('.diff-line')
    expect(lines).toHaveLength(5) // hunk + context + del + add + add
    expect(lines[0].classes()).toContain('diff-line-hunk')
    expect(lines[1].classes()).toContain('diff-line-context')
    expect(lines[2].classes()).toContain('diff-line-del')
    expect(lines[2].text()).toContain('-added line')
    expect(lines[3].classes()).toContain('diff-line-add')
    expect(lines[3].text()).toContain('+new line')
  })

  it('二进制 diff 展示占位提示，不渲染双栏', () => {
    const wrapper = mountViewer({ diffText: 'Binary files a/logo.png and b/logo.png differ' })
    expect(wrapper.find('.viewer-empty').text()).toContain('二进制文件不支持文本对比')
    expect(wrapper.find('.diff-table').exists()).toBe(false)
  })

  it('空 diff 展示无差异占位', () => {
    const wrapper = mountViewer({ diffText: '' })
    expect(wrapper.find('.viewer-empty').text()).toContain('无差异')
  })

  it('大 diff 截断保护：超过 5000 行仅渲染前 5000 行并提示', { timeout: 20000 }, async () => {
    // 5010 个 add 行 + hunk 头（5000 行 jsdom 渲染较慢，覆盖率插桩下更甚，放宽超时）
    const big = '@@ -1,5010 +1,5010 @@\n' + Array.from({ length: 5010 }, (_, i) => `+line ${i}`).join('\n')
    const wrapper = mountViewer({ diffText: big })
    expect(wrapper.vm.truncated).toBe(true)
    await nextTick()
    expect(wrapper.find('.truncated-tip').text()).toContain('仅显示前 5000 行')
    expect(wrapper.find('.diff-col-right').findAll('.diff-line')).toHaveLength(5000)
  })
})
