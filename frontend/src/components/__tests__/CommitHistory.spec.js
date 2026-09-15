import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import CommitHistory from '../CommitHistory.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetCommitHistory: vi.fn(),
  GetCommitFileDiff: vi.fn(),
  GetRangeDiff: vi.fn(),
  InvalidateCommitHistoryCache: vi.fn(),
  RunAiFunction: vi.fn(),
  CancelAiTask: vi.fn()
}))

// ai-task:done 事件总线：EventsOn 记录 handler 供测试手动触发（vi.hoisted 避免工厂函数 TDZ）
const aiEventBus = vi.hoisted(() => ({ handlers: {} }))

vi.mock('../../../wailsjs/runtime/runtime', () => ({
  // 对齐 Wails v2：EventsOn 返回「注销本监听器」闭包，onUnmounted 调它精准移除（不清同名全部）
  EventsOn: vi.fn((event, handler) => {
    aiEventBus.handlers[event] = handler
    return () => {
      if (aiEventBus.handlers[event] === handler) delete aiEventBus.handlers[event]
    }
  }),
  EventsOff: vi.fn((event) => {
    delete aiEventBus.handlers[event]
  })
}))

vi.mock('@element-plus/icons-vue', () => ({
  Refresh: { template: '<i class="i-refresh" />' },
  DocumentCopy: { template: '<i class="i-copy" />' },
  ArrowUp: { template: '<i class="i-up" />' },
  ArrowDown: { template: '<i class="i-down" />' },
  User: { template: '<i class="i-user" />' },
  Search: { template: '<i class="i-search" />' },
  View: { template: '<i class="i-view" />' }
}))

const stubs = {
  'el-card': { template: '<div class="el-card"><slot name="header" /><slot /></div>' },
  'el-input': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keydown="$emit(\'keydown\', $event)" />',
    props: ['modelValue', 'placeholder', 'size', 'clearable', 'prefixIcon'],
    emits: ['update:modelValue', 'input']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\', $event)"><slot /><i v-if="$slots.icon"><slot name="icon" /></i></button>',
    props: ['icon', 'loading', 'size', 'type', 'circle', 'plain', 'disabled'],
    // 声明 emits 让父级 onClick 不进 $attrs，避免与模板 @click 双触发
    // 透传 $event 供父级 .stop 修饰符调 stopPropagation（审查此 commit 按钮需 .stop 阻冒泡到 commit-card）
    emits: ['click']
  },
  'el-icon': { template: '<i><slot /></i>' },
  // el-text 用 v-bind="$attrs" 让 class="sha-text" 落到 span 上，便于 .sha-text 选择器命中
  'el-text': { template: '<span v-bind="$attrs"><slot /></span>', props: ['type', 'size', 'strong'] },
  'el-tag': {
    template: '<span class="el-tag"><slot /></span>',
    props: ['type', 'size']
  },
  'el-checkbox': {
    template: '<input type="checkbox" class="el-checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
    props: ['modelValue', 'disabled'],
    emits: ['update:modelValue', 'change']
  },
  'el-empty': { template: '<div class="el-empty" :data-description="description" />', props: ['description'] },
  'el-date-picker': {
    name: 'ElDatePicker',
    template: '<input class="el-date-picker" />',
    props: ['modelValue', 'type', 'size', 'rangeSeparator', 'startPlaceholder', 'endPlaceholder', 'valueFormat'],
    emits: ['update:modelValue', 'change']
  },
  'el-descriptions': { template: '<div class="el-descriptions"><slot /></div>', props: ['column', 'size', 'border'] },
  'el-descriptions-item': { template: '<div class="el-desc-item"><slot /></div>', props: ['label'] },
  'el-collapse-transition': { template: '<div class="el-collapse"><slot /></div>' },
  // FileDiffDialog stub：捕获 props 即可，不渲染内部 diff 逻辑
  FileDiffDialog: {
    name: 'FileDiffDialog',
    template: '<div class="file-diff-stub" />',
    props: ['modelValue', 'repoPath', 'file', 'sha', 'baseSha', 'headSha', 'mode']
  },
  // CodeReviewResult stub：捕获 issues/summary/loading props，emit locate-file 供测试
  CodeReviewResult: {
    name: 'CodeReviewResult',
    template: '<div class="code-review-stub" v-if="modelValue" :data-loading="loading"><span v-for="(i, idx) in issues" :key="idx" class="stub-issue" @click="$emit(\'locate-file\', i.file)">{{ i.file }}</span></div>',
    props: ['modelValue', 'issues', 'summary', 'loading'],
    emits: ['update:modelValue', 'locate-file']
  }
}
const directives = { loading: () => {} }

const commit = (over = {}) => ({
  sha: 'abcdef1234567890abcdef1234567890abcdef12',
  shortSha: 'abcdef12',
  message: 'fix: 修复登录问题',
  author: '张三',
  email: 'zhangsan@example.com',
  timestamp: Math.floor((Date.now() - 3600 * 1000) / 1000), // 1 小时前
  dateTime: '2026-09-10 10:00',
  files: ['src/a.go', 'src/b.go'],
  ...over
})

function createWrapper(props = {}) {
  return mount(CommitHistory, {
    props: { repoPath: '/repo/A', ...props },
    global: { stubs, directives }
  })
}

describe('CommitHistory.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    Object.keys(aiEventBus.handlers).forEach(k => delete aiEventBus.handlers[k])
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('挂载时加载提交历史并 emit latest-commit', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const c = commit()
    GetCommitHistory.mockResolvedValue([c])
    wrapper = createWrapper()
    await flushPromises()
    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/A', 20, 0, { author: '', keyword: '', filePath: '' })
    expect(wrapper.findAll('.commit-card').length).toBe(1)
    expect(wrapper.text()).toContain('张三')
    expect(wrapper.emitted('latest-commit')).toBeTruthy()
    expect(wrapper.emitted('latest-commit')[0]).toEqual([c])
  })

  it('搜索关键词触发服务端过滤重载（keyword 传入 filter）', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([
      commit({ sha: 'aaa111', shortSha: 'aaa111', message: 'fix: 登录', author: '张三' }),
      commit({ sha: 'bbb222', shortSha: 'bbb222', message: 'feat: 新功能', author: '李四' })
    ])
    wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.findAll('.commit-card').length).toBe(2)

    // 输入关键词 → applyFilter 防抖 300ms 后服务端重载（仅返回匹配提交）
    GetCommitHistory.mockClear()
    GetCommitHistory.mockResolvedValue([commit({ message: 'fix: 登录', author: '张三' })])
    await wrapper.find('input').setValue('登录')
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()

    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/A', 20, 0, { author: '', keyword: '登录', filePath: '' })
    expect(wrapper.findAll('.commit-card').length).toBe(1)
    expect(wrapper.findAll('.commit-card')[0].text()).toContain('张三')
  })

  it('点击提交卡片展开/收起详情', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    const card = wrapper.find('.commit-card')
    // 初始未展开
    expect(card.classes()).not.toContain('is-expanded')
    // 点击展开
    await card.trigger('click')
    expect(wrapper.find('.commit-card').classes()).toContain('is-expanded')
    // 再次点击收起
    await card.trigger('click')
    expect(wrapper.find('.commit-card').classes()).not.toContain('is-expanded')
  })

  it('展开后展示完整 SHA / 作者邮箱 / 提交时间', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const c = commit()
    GetCommitHistory.mockResolvedValue([c])
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.commit-card').trigger('click')
    const detail = wrapper.find('.commit-detail')
    expect(detail.text()).toContain(c.sha)
    expect(detail.text()).toContain(c.email)
    expect(detail.text()).toContain(c.dateTime)
    // 变更文件 tag
    expect(detail.text()).toContain('src/a.go')
  })

  it('copyToClipboard 调用 clipboard.writeText 并成功提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const writeText = vi.fn(() => Promise.resolve())
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    // 点击 shortSha 文本触发复制
    await wrapper.find('.sha-text').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalled()
    expect(ElMessage.success).toHaveBeenCalledWith('已复制到剪贴板')
    vi.unstubAllGlobals()
  })

  it('clipboard 写入失败时弹错误提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const writeText = vi.fn(() => Promise.reject(new Error('denied')))
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.sha-text').trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('复制失败')
    vi.unstubAllGlobals()
  })

  it('加载失败时弹错误提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockRejectedValue(new Error('git error'))
    wrapper = createWrapper()
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('git error'))
  })

  it('空仓库展示暂无提交记录', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([])
    wrapper = createWrapper()
    await flushPromises()
    const empties = wrapper.findAll('.el-empty')
    expect(empties.length).toBeGreaterThan(0)
    expect(empties[0].attributes('data-description')).toBe('暂无提交记录')
  })

  it('过滤后无匹配展示未找到匹配的提交', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    // 设作者过滤后服务端返回空
    GetCommitHistory.mockClear()
    GetCommitHistory.mockResolvedValue([])
    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('不存在的人')
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()
    const empties = wrapper.findAll('.el-empty')
    expect(empties.length).toBeGreaterThan(0)
    expect(empties[0].attributes('data-description')).toBe('未找到匹配的提交')
  })

  it('刷新按钮清空展开态、清缓存并重新加载', async () => {
    const { GetCommitHistory, InvalidateCommitHistoryCache } = await import('../../../wailsjs/go/main/App')
    InvalidateCommitHistoryCache.mockResolvedValue()
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    // 展开一条
    await wrapper.find('.commit-card').trigger('click')
    expect(wrapper.find('.commit-card').classes()).toContain('is-expanded')
    // 点刷新（header-actions 末尾的 Refresh 按钮，前面有 compare-btn）
    GetCommitHistory.mockClear()
    InvalidateCommitHistoryCache.mockClear()
    GetCommitHistory.mockResolvedValue([commit()])
    const refreshBtn = wrapper.findAll('.el-card .header-actions button').pop()
    await refreshBtn.trigger('click')
    await flushPromises()
    // 前置清缓存（绕过命中与增量，确保全量重扫）
    expect(InvalidateCommitHistoryCache).toHaveBeenCalledWith('/repo/A')
    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/A', 20, 0, { author: '', keyword: '', filePath: '' })
    // 展开态被清空
    expect(wrapper.find('.commit-card').classes()).not.toContain('is-expanded')
  })

  it('切换 repoPath 重新加载并清空搜索关键字与过滤条件', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('input').setValue('关键字')
    await new Promise(r => setTimeout(r, 350))
    GetCommitHistory.mockClear()
    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()
    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/B', 20, 0, { author: '', keyword: '', filePath: '' })
    expect(wrapper.find('input').element.value).toBe('')
  })

  it('满页返回时展示加载更多按钮，点击追加并维护 hasMore', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    // 首页满 20 条 → hasMore=true
    const firstPage = Array.from({ length: 20 }, (_, i) => commit({ sha: String(i).padStart(40, '0') }))
    GetCommitHistory.mockResolvedValueOnce(firstPage)
    wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.find('.load-more').exists()).toBe(true)

    // 点击加载更多：offset=20，返回 5 条（不足 20 → hasMore=false）
    const nextPage = Array.from({ length: 5 }, (_, i) => commit({ sha: String(i + 20).padStart(40, '0') }))
    GetCommitHistory.mockResolvedValueOnce(nextPage)
    await wrapper.find('.load-more button').trigger('click')
    await flushPromises()
    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/A', 20, 20, { author: '', keyword: '', filePath: '' })
    // 累计 25 条
    expect(wrapper.findAll('.commit-card').length).toBe(25)
    // 第二页不足 20 → 不再展示加载更多
    expect(wrapper.find('.load-more').exists()).toBe(false)
  })

  it('defineExpose 暴露 loadCommits 与 handleRefresh', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    expect(typeof wrapper.vm.loadCommits).toBe('function')
    expect(typeof wrapper.vm.handleRefresh).toBe('function')
  })

  it('作者过滤触发服务端重载（author 传入 filter）', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    GetCommitHistory.mockClear()
    GetCommitHistory.mockResolvedValue([commit({ author: '张三' })])
    // filter-bar 作者 input（inputs[0] 为搜索框，[1] 为作者）
    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('张三')
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()
    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/A', 20, 0, { author: '张三', keyword: '', filePath: '' })
  })

  it('文件路径过滤触发服务端重载（filePath 传入 filter）', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    GetCommitHistory.mockClear()
    GetCommitHistory.mockResolvedValue([commit({ files: ['src/a.go'] })])
    // inputs: [0]搜索框 [1]作者 [2]date-picker [3]文件路径
    const inputs = wrapper.findAll('input')
    await inputs[3].setValue('src')
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()
    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/A', 20, 0, { author: '', keyword: '', filePath: 'src' })
  })

  it('日期区间过滤触发服务端重载（since/until 传入 filter）', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    GetCommitHistory.mockClear()
    GetCommitHistory.mockResolvedValue([])
    const dp = wrapper.findComponent({ name: 'ElDatePicker' })
    // 先 emit update:modelValue 让 v-model 更新 dateRange，再 emit change 触发 applyFilter
    dp.vm.$emit('update:modelValue', ['2026-01-01', '2026-01-31'])
    await nextTick()
    dp.vm.$emit('change')
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()
    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/A', 20, 0, { author: '', keyword: '', filePath: '', since: '2026-01-01', until: '2026-01-31' })
  })

  it('过滤后加载更多带当前 filter 参数', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const firstPage = Array.from({ length: 20 }, (_, i) => commit({ sha: String(i).padStart(40, '0') }))
    GetCommitHistory.mockResolvedValueOnce(firstPage)
    wrapper = createWrapper()
    await flushPromises()
    // 设作者过滤后重载首页
    GetCommitHistory.mockClear()
    GetCommitHistory.mockResolvedValueOnce(firstPage)
    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('张三')
    await new Promise(r => setTimeout(r, 350))
    await flushPromises()
    // 加载更多：带 author=张三 filter
    GetCommitHistory.mockClear()
    const nextPage = Array.from({ length: 5 }, (_, i) => commit({ sha: String(i + 20).padStart(40, '0') }))
    GetCommitHistory.mockResolvedValueOnce(nextPage)
    await wrapper.find('.load-more button').trigger('click')
    await flushPromises()
    expect(GetCommitHistory).toHaveBeenCalledWith('/repo/A', 20, 20, { author: '张三', keyword: '', filePath: '' })
  })

  it('formatTime 对近期时间输出相对文案', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const now = Math.floor(Date.now() / 1000)
    GetCommitHistory.mockResolvedValue([commit({ timestamp: now - 30 })]) // 30 秒前 → 0 分钟前
    wrapper = createWrapper()
    await flushPromises()
    expect(wrapper.find('.commit-time').text()).toContain('分钟前')
  })

  it('展开 commit 点击变更文件 tag 弹出 commit 文件 diff 弹窗，传 sha 与 file', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const c = commit({ files: ['src/a.go', 'src/b.go'] })
    GetCommitHistory.mockResolvedValue([c])
    wrapper = createWrapper()
    await flushPromises()
    // 展开详情面板
    await wrapper.find('.commit-card').trigger('click')
    // 点击第一个变更文件 tag
    const tags = wrapper.findAll('.files-section .el-tag')
    expect(tags.length).toBe(2)
    await tags[0].trigger('click')
    await nextTick()
    // FileDiffDialog stub 收到 mode=commit + sha + file
    const stub = wrapper.findComponent({ name: 'FileDiffDialog' })
    expect(stub.props('modelValue')).toBe(true)
    expect(stub.props('mode')).toBe('commit')
    expect(stub.props('sha')).toBe(c.sha)
    expect(stub.props('file')).toBe('src/a.go')
  })

  it('勾选两个提交后对比按钮启用，点击弹 range diff 传 base/head', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const c1 = commit({ sha: '1111111111111111111111111111111111111111' })
    const c2 = commit({ sha: '2222222222222222222222222222222222222222' })
    GetCommitHistory.mockResolvedValue([c1, c2])
    wrapper = createWrapper()
    await flushPromises()

    const cards = wrapper.findAll('.commit-card')
    expect(cards.length).toBe(2)
    // 未选满：按钮 disabled
    const disabledBtn = wrapper.find('.compare-btn[disabled]')
    expect(disabledBtn.exists()).toBe(true)

    // 勾选两条
    const checkboxes = wrapper.findAll('.el-checkbox')
    expect(checkboxes.length).toBe(2)
    await checkboxes[0].setValue(true)
    await checkboxes[1].setValue(true)
    await nextTick()
    // 选满 2：按钮变启用（无 disabled 属性）
    expect(wrapper.find('.compare-btn[disabled]').exists()).toBe(false)

    // 点对比按钮
    await wrapper.find('.compare-btn').trigger('click')
    await nextTick()
    // 找到 range 模式的 FileDiffDialog stub（两个 stub，取 mode=range）
    const stubs = wrapper.findAllComponents({ name: 'FileDiffDialog' })
    const rangeStub = stubs.find(s => s.props('mode') === 'range')
    expect(rangeStub).toBeTruthy()
    expect(rangeStub.props('modelValue')).toBe(true)
    expect(rangeStub.props('baseSha')).toBe(c1.sha)
    expect(rangeStub.props('headSha')).toBe(c2.sha)
  })

  it('勾选第三个提交时弹出最早的，保持限选 2 个', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const c1 = commit({ sha: '1111111111111111111111111111111111111111' })
    const c2 = commit({ sha: '2222222222222222222222222222222222222222' })
    const c3 = commit({ sha: '3333333333333333333333333333333333333333' })
    GetCommitHistory.mockResolvedValue([c1, c2, c3])
    wrapper = createWrapper()
    await flushPromises()

    const checkboxes = wrapper.findAll('.el-checkbox')
    await checkboxes[0].setValue(true) // c1
    await checkboxes[1].setValue(true) // c2
    await nextTick()
    expect(wrapper.vm.selectedShas).toEqual([c1.sha, c2.sha])

    // 勾第三个 → 弹出 c1，保留 c2、c3
    await checkboxes[2].setValue(true) // c3
    await nextTick()
    expect(wrapper.vm.selectedShas).toEqual([c2.sha, c3.sha])
    // c1 复选框应取消勾选
    expect(checkboxes[0].element.checked).toBe(false)
  })

  // ===== AI 代码审查（审指定 commit） =====

  it('展开 commit 显示「审查此 commit」按钮', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.commit-card').trigger('click')
    const btn = wrapper.findAll('button').find(b => b.text().includes('审查此 commit'))
    expect(btn).toBeTruthy()
  })

  it('点击「审查此 commit」调 GetCommitFileDiff(sha, 空串) + RunAiFunction 注入 diff', async () => {
    const { GetCommitHistory, GetCommitFileDiff, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    const c = commit()
    GetCommitHistory.mockResolvedValue([c])
    GetCommitFileDiff.mockResolvedValue('=== src/a.go ===\n+const x = 1\n')
    RunAiFunction.mockResolvedValue('task-cr-1')
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.commit-card').trigger('click')
    await wrapper.findAll('button').find(b => b.text().includes('审查此 commit')).trigger('click')
    await flushPromises()
    // file 传空串 → 全 commit diff
    expect(GetCommitFileDiff).toHaveBeenCalledWith('/repo/A', c.sha, '')
    expect(RunAiFunction).toHaveBeenCalledWith('code-review', { diff: '=== src/a.go ===\n+const x = 1\n' })
  })

  it('切仓库时取消在途 AI 审查任务并重置态（防旧仓库结果串入新仓库 + loading 卡死）', async () => {
    const { GetCommitHistory, GetCommitFileDiff, RunAiFunction, CancelAiTask } = await import('../../../wailsjs/go/main/App')
    const c = commit()
    GetCommitHistory.mockResolvedValue([c])
    GetCommitFileDiff.mockResolvedValue('diff')
    RunAiFunction.mockResolvedValue('task-cr-stale')
    CancelAiTask.mockResolvedValue(true)
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.commit-card').trigger('click')
    await wrapper.findAll('button').find(b => b.text().includes('审查此 commit')).trigger('click')
    await flushPromises()
    expect(CancelAiTask).not.toHaveBeenCalled()
    // 审查弹窗已开（审查中）：CodeReviewResult 常驻挂载，断 modelValue=true
    expect(wrapper.findComponent({ name: 'CodeReviewResult' }).props('modelValue')).toBe(true)
    // 切仓库：触发 resetAiState
    await wrapper.setProps({ repoPath: '/repo/B' })
    await flushPromises()
    // 在途审查任务被取消
    expect(CancelAiTask).toHaveBeenCalledWith('task-cr-stale')
    // 审查弹窗关闭：modelValue=false
    expect(wrapper.findComponent({ name: 'CodeReviewResult' }).props('modelValue')).toBe(false)
    // 旧 taskId 的 done 事件不再匹配本组件 → 不渲染问题清单（防旧仓库结果串入新仓库）
    const doneHandler = aiEventBus.handlers['ai-task:done']
    doneHandler({ taskId: 'task-cr-stale', structuredOutput: { issues: [{ file: 'x', severity: 'info', category: 'style', description: 'y' }] }, error: '', canceled: false })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'CodeReviewResult' }).props('modelValue')).toBe(false)
  })

  it('done 事件 structuredOutput.issues 渲染问题清单到 CodeReviewResult', async () => {
    const { GetCommitHistory, GetCommitFileDiff, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    const c = commit()
    GetCommitHistory.mockResolvedValue([c])
    GetCommitFileDiff.mockResolvedValue('diff')
    RunAiFunction.mockResolvedValue('task-cr-2')
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.commit-card').trigger('click')
    await wrapper.findAll('button').find(b => b.text().includes('审查此 commit')).trigger('click')
    await flushPromises()
    const doneHandler = aiEventBus.handlers['ai-task:done']
    expect(doneHandler).toBeTruthy()
    doneHandler({
      taskId: 'task-cr-2',
      structuredOutput: {
        issues: [
          { file: 'src/a.go', line: 1, severity: 'critical', category: 'bug', description: '空指针', suggestion: '判空' }
        ],
        summary: '1 个问题'
      },
      error: '',
      canceled: false
    })
    await flushPromises()
    const stub = wrapper.findComponent({ name: 'CodeReviewResult' })
    expect(stub.props('modelValue')).toBe(true)
    expect(stub.props('issues').length).toBe(1)
    expect(stub.props('summary')).toBe('1 个问题')
    expect(stub.props('loading')).toBe(false)
  })

  it('done 事件 error 时弹错误并关闭审查弹窗', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetCommitHistory, GetCommitFileDiff, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    GetCommitHistory.mockResolvedValue([commit()])
    GetCommitFileDiff.mockResolvedValue('diff')
    RunAiFunction.mockResolvedValue('task-cr-err')
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.commit-card').trigger('click')
    await wrapper.findAll('button').find(b => b.text().includes('审查此 commit')).trigger('click')
    await flushPromises()
    const doneHandler = aiEventBus.handlers['ai-task:done']
    doneHandler({ taskId: 'task-cr-err', error: '超时', canceled: false, structuredOutput: null })
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('超时'))
    const stub = wrapper.findComponent({ name: 'CodeReviewResult' })
    expect(stub.props('modelValue')).toBe(false)
  })

  it('CodeReviewResult locate-file 打开 commit 模式 FileDiffDialog（传 sha + file）', async () => {
    const { GetCommitHistory, GetCommitFileDiff, RunAiFunction } = await import('../../../wailsjs/go/main/App')
    const c = commit()
    GetCommitHistory.mockResolvedValue([c])
    GetCommitFileDiff.mockResolvedValue('diff')
    RunAiFunction.mockResolvedValue('task-cr-loc')
    wrapper = createWrapper()
    await flushPromises()
    await wrapper.find('.commit-card').trigger('click')
    await wrapper.findAll('button').find(b => b.text().includes('审查此 commit')).trigger('click')
    await flushPromises()
    const doneHandler = aiEventBus.handlers['ai-task:done']
    doneHandler({ taskId: 'task-cr-loc', structuredOutput: { issues: [{ file: 'src/a.go', severity: 'info', category: 'style', description: 'x' }] }, error: '', canceled: false })
    await flushPromises()
    const reviewStub = wrapper.findComponent({ name: 'CodeReviewResult' })
    await reviewStub.vm.$emit('locate-file', 'src/a.go')
    await nextTick()
    // commit 模式 FileDiffDialog 打开，sha + file 定位
    const diffStub = wrapper.findComponent({ name: 'FileDiffDialog' })
    expect(diffStub.exists()).toBe(true)
    expect(diffStub.props('mode')).toBe('commit')
    expect(diffStub.props('sha')).toBe(c.sha)
    expect(diffStub.props('file')).toBe('src/a.go')
  })

  it('卸载时用 EventsOn 返回闭包注销本组件监听器（禁 EventsOff 清同名全部）', async () => {
    const { GetCommitHistory } = await import('../../../wailsjs/go/main/App')
    const { EventsOn, EventsOff } = await import('../../../wailsjs/runtime/runtime')
    GetCommitHistory.mockResolvedValue([commit()])
    wrapper = createWrapper()
    await flushPromises()
    expect(EventsOn).toHaveBeenCalledWith('ai-task:done', expect.any(Function))
    expect(aiEventBus.handlers['ai-task:done']).toBeTruthy()
    EventsOff.mockClear()
    wrapper.unmount()
    // 卸载后本组件监听器被精准移除
    expect(aiEventBus.handlers['ai-task:done']).toBeUndefined()
    // 未调 EventsOff（会清同名全部监听器，误伤 AiFunctionPanel / LocalChanges）
    expect(EventsOff).not.toHaveBeenCalled()
    wrapper = null
  })
})
