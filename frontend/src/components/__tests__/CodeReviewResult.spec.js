import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import CodeReviewResult from '../CodeReviewResult.vue'

vi.mock('@element-plus/icons-vue', () => ({
  Loading: { template: '<i class="i-loading" />' }
}))

const stubs = {
  'el-dialog': {
    template: '<div class="el-dialog" v-if="modelValue"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width']
  },
  'el-tag': {
    template: '<span class="el-tag" :data-type="type" :data-effect="effect"><slot /></span>',
    props: ['type', 'size', 'effect']
  },
  'el-empty': {
    template: '<div class="el-empty" :data-description="description" />',
    props: ['description', 'imageSize']
  },
  'el-icon': { template: '<i><slot /></i>' },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
    props: ['size'],
    emits: ['click']
  }
}

const issues = [
  { file: 'src/a.go', line: 10, severity: 'warning', category: 'performance', confidence: 0.6, description: 'N+1 查询', suggestion: '预加载' },
  { file: 'src/b.go', line: 20, severity: 'critical', category: 'bug', confidence: 0.9, description: '空指针解引用', suggestion: '判空' },
  { file: 'src/c.go', line: 30, severity: 'info', category: 'style', confidence: 0.5, description: '命名不清', suggestion: '改名' },
  { file: 'src/d.go', line: 40, severity: 'critical', category: 'security', confidence: 0.8, description: '密码明文日志', suggestion: '脱敏' }
]

function createWrapper(props = {}) {
  return mount(CodeReviewResult, {
    props: { modelValue: true, issues: [], summary: '', loading: false, ...props },
    global: { stubs }
  })
}

describe('CodeReviewResult.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('加载态显示审查中', () => {
    wrapper = createWrapper({ loading: true })
    expect(wrapper.find('.review-loading').exists()).toBe(true)
    expect(wrapper.text()).toContain('审查中')
    // 加载态不渲染问题清单
    expect(wrapper.find('.review-groups').exists()).toBe(false)
  })

  it('issues 为空时降级 el-empty 提示', () => {
    wrapper = createWrapper({ issues: [], loading: false })
    const empty = wrapper.find('.el-empty')
    expect(empty.exists()).toBe(true)
    expect(empty.attributes('data-description')).toBe('AI 未发现问题或输出格式异常')
  })

  it('summary 非空时渲染整体结论', () => {
    wrapper = createWrapper({ issues: [], loading: false, summary: '审查 2 文件无问题' })
    expect(wrapper.find('.review-summary').exists()).toBe(true)
    expect(wrapper.text()).toContain('审查 2 文件无问题')
  })

  it('问题按级别分组：critical 置顶 → warning → info', () => {
    wrapper = createWrapper({ issues, loading: false })
    const groups = wrapper.findAll('.review-group')
    expect(groups.length).toBe(3)
    // 第一组 critical（含 2 个问题：b.go bug + d.go security）
    expect(groups[0].find('.el-tag').text()).toBe('严重')
    expect(groups[0].findAll('.review-issue').length).toBe(2)
    // 第二组 warning
    expect(groups[1].find('.el-tag').text()).toBe('警告')
    expect(groups[1].findAll('.review-issue').length).toBe(1)
    // 第三组 info
    expect(groups[2].find('.el-tag').text()).toBe('提示')
    expect(groups[2].findAll('.review-issue').length).toBe(1)
  })

  it('每个问题渲染文件路径、行号、类别 tag、描述、建议', () => {
    wrapper = createWrapper({ issues, loading: false })
    const firstIssue = wrapper.findAll('.review-issue')[0]
    // critical 组首项 = b.go（bug）
    expect(firstIssue.text()).toContain('src/b.go:20')
    expect(firstIssue.text()).toContain('空指针解引用')
    expect(firstIssue.text()).toContain('判空')
    // 类别 tag 渲染
    expect(firstIssue.find('.el-tag').text()).toBe('缺陷')
  })

  it('点击问题文件路径 emit locate-file', async () => {
    wrapper = createWrapper({ issues, loading: false })
    const fileEl = wrapper.findAll('.review-issue-file')[0]
    await fileEl.trigger('click')
    const events = wrapper.emitted('locate-file')
    expect(events).toBeTruthy()
    expect(events[0]).toEqual(['src/b.go'])
  })

  it('无 suggestion 的问题不渲染建议块', () => {
    const noSuggestion = [{ file: 'src/x.go', line: 1, severity: 'info', category: 'style', description: '仅描述无建议' }]
    wrapper = createWrapper({ issues: noSuggestion, loading: false })
    expect(wrapper.find('.review-issue-suggestion').exists()).toBe(false)
    expect(wrapper.text()).toContain('仅描述无建议')
  })

  it('未知 severity 归入 info 组（兜底）', () => {
    const unknownSev = [{ file: 'src/y.go', line: 1, severity: 'weird', category: 'bug', description: '未知级别兜底' }]
    wrapper = createWrapper({ issues: unknownSev, loading: false })
    const groups = wrapper.findAll('.review-group')
    expect(groups.length).toBe(1)
    expect(groups[0].find('.el-tag').text()).toBe('提示')
  })

  it('关闭按钮 emit update:modelValue false', async () => {
    wrapper = createWrapper({ issues: [], loading: false })
    await wrapper.find('button').trigger('click')
    const events = wrapper.emitted('update:modelValue')
    expect(events).toBeTruthy()
    expect(events[0]).toEqual([false])
  })
})
