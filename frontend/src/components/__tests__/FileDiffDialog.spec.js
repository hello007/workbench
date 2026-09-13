import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import FileDiffDialog from '../FileDiffDialog.vue'
import { useSettingsStore } from '../../store/settings'

// Mock element-plus 的 ElMessage（FileDiffDialog catch 分支会调用）
vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetFileDiff: vi.fn(),
  GetCommitFileDiff: vi.fn(),
  GetRangeDiff: vi.fn(),
  OpenInExternalDiff: vi.fn(),
  GetSettings: vi.fn(),
  SaveSettings: vi.fn()
}))

// v-loading 指令 stub（jsdom 无 element-plus 指令注册）
const loadingDirective = () => {}
const directives = { loading: loadingDirective }

// stub element-plus 组件，保留 slot 链路以断言文本；显式吸收 props 避免 fallthrough 警告
const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot name="header" /><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'append', 'destroyOnClose'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'icon', 'loading', 'disabled', 'text'],
    emits: ['click']
  },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  // tooltip 透传 content 供断言，disabled 按钮场景 tooltip 仍展示提示
  'el-tooltip': {
    template: '<span class="el-tooltip" :content="content"><slot /></span>',
    props: ['content', 'disabled', 'placement']
  }
}

let pinia

// FileDiffDialog 的 watch 非 immediate：仅 modelValue/file 变化才触发 loadDiff。
// 故先以 modelValue=false 挂载，再 setProps(true) 模拟"打开"触发加载。
async function createWrapper(props = {}) {
  pinia = createPinia()
  const wrapper = mount(FileDiffDialog, {
    props: { modelValue: false, repoPath: '/repo/A', file: 'src/main.go', ...props },
    global: { plugins: [pinia], stubs, directives }
  })
  await wrapper.setProps({ modelValue: true })
  await flushPromises()
  return wrapper
}

// 以已配置外部 diff 工具的 store 状态挂载（挂载前配置，保证首渲染按钮即可用）
async function createWrapperConfigured(props = {}) {
  pinia = createPinia()
  setActivePinia(pinia)
  const store = useSettingsStore()
  store.diffToolPath = 'C:\\tools\\diff.exe'
  store.diffToolArgs = '{left} {right}'
  const wrapper = mount(FileDiffDialog, {
    props: { modelValue: false, repoPath: '/repo/A', file: 'src/main.go', ...props },
    global: { plugins: [pinia], stubs, directives }
  })
  await wrapper.setProps({ modelValue: true })
  await flushPromises()
  return wrapper
}

describe('FileDiffDialog.vue', () => {
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

  it('标题取 file 末尾文件名', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('')
    wrapper = await createWrapper({ file: 'src/utils/helper.js' })
    expect(wrapper.find('.diff-dialog-title').text()).toBe('文件差异 - helper.js')
  })

  it('file 为空时标题回退通用文案', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('')
    wrapper = await createWrapper({ file: '' })
    expect(wrapper.find('.diff-dialog-title').text()).toBe('文件差异')
  })

  it('后端返回空文本时展示二进制/无差异提示', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('   ')
    wrapper = await createWrapper()
    expect(wrapper.find('.diff-empty').text()).toContain('无差异')
  })

  it('后端返回 Binary files differ 时展示二进制提示', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('Binary files a/x b/y differ')
    wrapper = await createWrapper()
    expect(wrapper.find('.diff-empty').text()).toContain('二进制文件')
  })

  it('解析 unified diff 为左右两栏并按 kind 打 class', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    const diff = [
      '@@ -1,2 +1,2 @@',
      ' context',
      '-old',
      '+new'
    ].join('\n')
    GetFileDiff.mockResolvedValue(diff)
    wrapper = await createWrapper()

    expect(wrapper.find('.diff-col-left').exists()).toBe(true)
    expect(wrapper.find('.diff-col-right').exists()).toBe(true)

    const leftLines = wrapper.find('.diff-col-left').findAll('.diff-line')
    const rightLines = wrapper.find('.diff-col-right').findAll('.diff-line')
    expect(leftLines.length).toBe(4) // hunk + context + del + empty(占位)
    expect(rightLines.length).toBe(4) // hunk + context + empty + add

    expect(leftLines[0].classes()).toContain('diff-line-hunk')
    expect(leftLines[1].classes()).toContain('diff-line-context')
    expect(leftLines[2].classes()).toContain('diff-line-del')
    expect(rightLines[3].classes()).toContain('diff-line-add')
  })

  it('context 行号自 hunk 头起始并逐行递增', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    const diff = ['@@ -10,2 +20,2 @@', ' ctx-a', ' ctx-b'].join('\n')
    GetFileDiff.mockResolvedValue(diff)
    wrapper = await createWrapper()
    const leftNos = wrapper.find('.diff-col-left').findAll('.diff-line-no')
    expect(leftNos[1].text()).toBe('10')
    expect(leftNos[2].text()).toBe('11')
  })

  it('No newline 提示行被忽略不进双栏', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    const diff = ['@@ -1,1 +1,1 @@', ' ctx', '\\ No newline at end of file'].join('\n')
    GetFileDiff.mockResolvedValue(diff)
    wrapper = await createWrapper()
    const leftLines = wrapper.find('.diff-col-left').findAll('.diff-line')
    expect(leftLines.length).toBe(2) // hunk + ctx，No newline 被忽略
  })

  it('GetFileDiff 抛错时展示错误文案并调 ElMessage.error', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockRejectedValue(new Error('git boom'))
    wrapper = await createWrapper()
    expect(wrapper.find('.diff-empty').text()).toContain('加载差异失败: git boom')
    expect(ElMessage.error).toHaveBeenCalled()
  })

  it('关闭按钮 emit update:modelValue=false', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('')
    wrapper = await createWrapper()
    const closeBtn = wrapper.findAll('button').find(b => b.text() === '关闭')
    await closeBtn.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeTruthy()
    expect(wrapper.emitted('update:modelValue')[0]).toEqual([false])
  })

  it('repoPath 或 file 缺失时不发起加载', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    wrapper = await createWrapper({ repoPath: '', file: '' })
    expect(GetFileDiff).not.toHaveBeenCalled()
  })

  it('commit 模式调 GetCommitFileDiff 并以 sha/file 传参', async () => {
    const { GetCommitFileDiff } = await import('../../../wailsjs/go/main/App')
    const diff = ['@@ -1,1 +1,2 @@', ' ctx', '+new'].join('\n')
    GetCommitFileDiff.mockResolvedValue(diff)
    wrapper = await createWrapper({ mode: 'commit', sha: 'abcdef12', file: 'src/a.go' })
    expect(GetCommitFileDiff).toHaveBeenCalledWith('/repo/A', 'abcdef12', 'src/a.go')
    // 双栏渲染正常
    expect(wrapper.find('.diff-col-left').exists()).toBe(true)
    expect(wrapper.find('.diff-col-right').findAll('.diff-line-add').length).toBe(1)
  })

  it('range 模式调 GetRangeDiff 并以 base/head 传参', async () => {
    const { GetRangeDiff } = await import('../../../wailsjs/go/main/App')
    const diff = ['@@ -1,1 +1,2 @@', ' ctx', '+new'].join('\n')
    GetRangeDiff.mockResolvedValue(diff)
    wrapper = await createWrapper({ mode: 'range', baseSha: 'base1234', headSha: 'head5678', file: '' })
    expect(GetRangeDiff).toHaveBeenCalledWith('/repo/A', 'base1234', 'head5678')
  })

  it('range 模式多文件按 diff --git 头拆分分组', async () => {
    const { GetRangeDiff } = await import('../../../wailsjs/go/main/App')
    // 两文件 diff：a.go 有新增行，b.go 二进制
    const diff = [
      'diff --git a/src/a.go b/src/a.go',
      'index 111..222 100644',
      '--- a/src/a.go',
      '+++ b/src/a.go',
      '@@ -1,1 +1,2 @@',
      ' ctx',
      '+new',
      'diff --git a/bin.dat b/bin.dat',
      'Binary files a/bin.dat and b/bin.dat differ'
    ].join('\n')
    GetRangeDiff.mockResolvedValue(diff)
    wrapper = await createWrapper({ mode: 'range', baseSha: 'base1234', headSha: 'head5678', file: '' })
    // 两个文件分组
    const groups = wrapper.findAll('.range-file-group')
    expect(groups.length).toBe(2)
    // 第一组：a.go，有双栏
    expect(groups[0].find('.range-file-name').text()).toBe('src/a.go')
    expect(groups[0].find('.diff-col-left').exists()).toBe(true)
    expect(groups[0].findAll('.diff-line-add').length).toBe(1)
    // 第二组：bin.dat，二进制提示，无双栏
    expect(groups[1].find('.range-file-name').text()).toBe('bin.dat')
    expect(groups[1].find('.range-binary').exists()).toBe(true)
    expect(groups[1].find('.diff-col-left').exists()).toBe(false)
  })

  it('range 模式无差异返回空分组时展示无差异提示', async () => {
    const { GetRangeDiff } = await import('../../../wailsjs/go/main/App')
    GetRangeDiff.mockResolvedValue('  ')
    wrapper = await createWrapper({ mode: 'range', baseSha: 'base1234', headSha: 'head5678', file: '' })
    expect(wrapper.find('.diff-empty').text()).toContain('无差异')
  })

  it('commit 模式 sha 或 file 缺失时不发起加载', async () => {
    const { GetCommitFileDiff } = await import('../../../wailsjs/go/main/App')
    wrapper = await createWrapper({ mode: 'commit', sha: '', file: 'src/a.go' })
    expect(GetCommitFileDiff).not.toHaveBeenCalled()
  })

  // ---- 外部 diff 工具按钮 ----

  it('工具未配置时按钮置灰且 tooltip 提示去设置', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('')
    wrapper = await createWrapper()
    const btn = wrapper.findAll('button').find(b => b.text() === '用外部工具打开')
    expect(btn.attributes('disabled')).not.toBeUndefined()
    const tooltip = wrapper.find('.diff-dialog-header .el-tooltip')
    expect(tooltip.attributes('content')).toBe('未配置外部 diff 工具，请在设置中配置')
  })

  it('工具已配置时按钮可用且 tooltip 说明用途', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('@@ -1,1 +1,1 @@\n ctx')
    wrapper = await createWrapperConfigured()
    const btn = wrapper.findAll('button').find(b => b.text() === '用外部工具打开')
    expect(btn.attributes('disabled')).toBeUndefined()
    const tooltip = wrapper.find('.diff-dialog-header .el-tooltip')
    expect(tooltip.attributes('content')).toBe('用外部 diff 工具打开左右版本文件')
  })

  it('点击按钮以 mode/file/sha 六参调用 OpenInExternalDiff', async () => {
    const { GetCommitFileDiff, OpenInExternalDiff } = await import('../../../wailsjs/go/main/App')
    GetCommitFileDiff.mockResolvedValue('@@ -1,1 +1,1 @@\n ctx')
    OpenInExternalDiff.mockResolvedValue(undefined)
    wrapper = await createWrapperConfigured({ mode: 'commit', sha: 'abcdef12', file: 'src/a.go' })
    const btn = wrapper.findAll('button').find(b => b.text() === '用外部工具打开')
    await btn.trigger('click')
    await flushPromises()
    expect(OpenInExternalDiff).toHaveBeenCalledWith('/repo/A', 'commit', 'src/a.go', 'abcdef12', '', '')
  })

  it('打开失败时经 handleError 分流提示', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetFileDiff, OpenInExternalDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('@@ -1,1 +1,1 @@\n ctx')
    // 结构化 AppError（未配置）：warning 分流
    OpenInExternalDiff.mockRejectedValue({ code: 'E_DIFF_TOOL_NOT_CONFIGURED', message: '未配置外部 diff 工具，请在设置中配置' })
    wrapper = await createWrapperConfigured()
    const btn = wrapper.findAll('button').find(b => b.text() === '用外部工具打开')
    await btn.trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('未配置外部 diff 工具，请在设置中配置')
  })

  it('二进制文件场景按钮禁用', async () => {
    const { GetFileDiff } = await import('../../../wailsjs/go/main/App')
    GetFileDiff.mockResolvedValue('Binary files a/x b/y differ')
    wrapper = await createWrapperConfigured()
    const btn = wrapper.findAll('button').find(b => b.text() === '用外部工具打开')
    expect(btn.attributes('disabled')).not.toBeUndefined()
  })

  it('range 模式文件组按钮以组内文件名调用', async () => {
    const { GetRangeDiff, OpenInExternalDiff } = await import('../../../wailsjs/go/main/App')
    const diff = [
      'diff --git a/src/a.go b/src/a.go',
      'index 111..222 100644',
      '--- a/src/a.go',
      '+++ b/src/a.go',
      '@@ -1,1 +1,2 @@',
      ' ctx',
      '+new'
    ].join('\n')
    GetRangeDiff.mockResolvedValue(diff)
    OpenInExternalDiff.mockResolvedValue(undefined)
    wrapper = await createWrapperConfigured({ mode: 'range', baseSha: 'base1234', headSha: 'head5678', file: '' })
    const groupBtn = wrapper.find('.range-file-header button')
    await groupBtn.trigger('click')
    await flushPromises()
    expect(OpenInExternalDiff).toHaveBeenCalledWith('/repo/A', 'range', 'src/a.go', '', 'base1234', 'head5678')
  })
})
