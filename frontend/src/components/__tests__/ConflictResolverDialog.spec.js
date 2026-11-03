import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ConflictResolverDialog from '../ConflictResolverDialog.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: {
      confirm: vi.fn(() => Promise.resolve())
    }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetClaudeDefaultModel: vi.fn(() => Promise.resolve('')),
  GetConflictFileContent: vi.fn(),
  SaveConflictResolution: vi.fn(() => Promise.resolve())
}))

const MARKER_TEXT = [
  'pre',
  '<<<<<<< HEAD',
  'ours-1',
  '=======',
  'theirs-1',
  '>>>>>>> feature',
  'mid',
  '<<<<<<< HEAD',
  'ours-2',
  '=======',
  'theirs-2',
  '>>>>>>> feature',
  'post'
].join('\n')

// 含 diff3 base 段的冲突文本（base 解析但不参与合并结果）
const DIFF3_TEXT = [
  '<<<<<<< HEAD',
  'ours-x',
  '||||||| merged',
  'base-x',
  '=======',
  'theirs-x',
  '>>>>>>> feature'
].join('\n')

const stubs = {
  'el-dialog': {
    // append-to-body 场景下仍直接渲染（不走 teleport），便于断言内容
    template: '<div v-if="modelValue" class="el-dialog"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'top', 'appendToBody'],
    emits: ['update:modelValue', 'open']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['loading', 'size', 'type', 'text', 'disabled'],
    emits: ['click']
  },
  'el-tag': { template: '<span class="el-tag"><slot /></span>', props: ['type', 'size'] },
  'el-text': { template: '<span class="el-text"><slot /></span>', props: ['type', 'size'] },
  'el-alert': { template: '<div class="el-alert">{{ title }}</div>', props: ['type', 'title', 'closable', 'showIcon'] },
  'el-empty': { template: '<div class="el-empty">{{ description }}</div>', props: ['description'] },
  'el-collapse': { template: '<div class="el-collapse"><slot /></div>' },
  'el-collapse-item': { template: '<div class="el-collapse-item"><slot /></div>', props: ['title', 'name'] },
  'el-tooltip': { template: '<span><slot /></span>', props: ['content', 'placement'] },
  'el-input': {
    template: '<textarea v-if="type === \'textarea\'" class="el-textarea" :value="modelValue" :rows="rows" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'type', 'rows'],
    emits: ['update:modelValue']
  }
}

function mountDialog(props = {}) {
  return mount(ConflictResolverDialog, {
    props: {
      visible: true,
      repoPath: '/repo/A',
      file: 'src/a.txt',
      conflictType: 'merge',
      ...props
    },
    global: { stubs }
  })
}

const findBtn = (wrapper, text, exact = false) =>
  wrapper.findAll('button').find(b => (exact ? b.text() === text : b.text().includes(text)))

async function mountWithContent(content, props = {}) {
  const { GetConflictFileContent } = await import('../../../wailsjs/go/main/App')
  GetConflictFileContent.mockResolvedValue({
    content, encoding: 'UTF-8', binary: false, tooLarge: false
  })
  const wrapper = mountDialog(props)
  await flushPromises()
  return wrapper
}

describe('ConflictResolverDialog.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
  })

  it('打开时加载文件内容并按标记渲染冲突块卡片', async () => {
    wrapper = await mountWithContent(MARKER_TEXT)

    expect(wrapper.findAll('.conflict-block')).toHaveLength(2)
    expect(wrapper.text()).toContain('冲突块 1/2')
    expect(wrapper.text()).toContain('当前分支（ours）')
    expect(wrapper.text()).toContain('传入分支（theirs）')
    expect(wrapper.text()).toContain('ours-1')
    expect(wrapper.text()).toContain('theirs-2')
  })

  it('两块都点「用当前」：合并预览仅含 ours 行，保存传合并内容与编码', async () => {
    const { SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    wrapper = await mountWithContent(MARKER_TEXT)

    // 两块都选 ours
    const oursBtns = wrapper.findAll('button').filter(b => b.text() === '用当前')
    expect(oursBtns).toHaveLength(2)
    await oursBtns[0].trigger('click')
    await oursBtns[1].trigger('click')
    await flushPromises()

    // 决策后块卡片显示结果标签
    expect(wrapper.text()).toContain('已用当前')

    // 合并预览实时更新（不含 theirs 行）
    expect(wrapper.find('.preview-code').text()).toContain('ours-1')
    expect(wrapper.find('.preview-code').text()).not.toContain('theirs-1')

    await findBtn(wrapper, '保存并标记已解决').trigger('click')
    await flushPromises()

    expect(SaveConflictResolution).toHaveBeenCalledWith(
      '/repo/A', 'src/a.txt', 'pre\nours-1\nmid\nours-2\npost', 'UTF-8'
    )
    // 保存成功：resolved 上报 + 弹窗关闭
    expect(wrapper.emitted('resolved')).toBeTruthy()
    expect(wrapper.emitted('update:visible')).toBeTruthy()
    expect(wrapper.emitted('update:visible').at(-1)).toEqual([false])
  })

  it('「用传入」：块内容替换为 theirs 行', async () => {
    const { SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    wrapper = await mountWithContent(MARKER_TEXT)

    await findBtn(wrapper, '用传入', true).trigger('click')
    await flushPromises()

    await findBtn(wrapper, '保存并标记已解决').trigger('click')
    await flushPromises()

    expect(SaveConflictResolution).toHaveBeenCalledWith(
      '/repo/A', 'src/a.txt', 'pre\ntheirs-1\nmid\nours-2\ntheirs-2\npost', 'UTF-8'
    )
  })

  it('diff3 base 段解析展示但不参与合并结果', async () => {
    const { SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    wrapper = await mountWithContent(DIFF3_TEXT)

    // base 段内容在卡片中可见
    expect(wrapper.text()).toContain('base-x')

    await findBtn(wrapper, '用当前', true).trigger('click')
    await findBtn(wrapper, '保存并标记已解决').trigger('click')
    await flushPromises()

    expect(SaveConflictResolution).toHaveBeenCalledWith('/repo/A', 'src/a.txt', 'ours-x', 'UTF-8')
  })

  it('未决策块保存：合并结果保留两侧内容且不弹确认（标记行已剔除）', async () => {
    const { SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox } = await import('element-plus')
    wrapper = await mountWithContent(MARKER_TEXT)

    await findBtn(wrapper, '保存并标记已解决').trigger('click')
    await flushPromises()

    expect(ElMessageBox.confirm).not.toHaveBeenCalled()
    expect(SaveConflictResolution).toHaveBeenCalledWith(
      '/repo/A', 'src/a.txt', 'pre\nours-1\ntheirs-1\nmid\nours-2\ntheirs-2\npost', 'UTF-8'
    )
  })

  it('全文编辑模式保存仍含标记时弹确认；取消则不保存', async () => {
    const { SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox } = await import('element-plus')
    wrapper = await mountWithContent(MARKER_TEXT)

    // 切到全文编辑
    await findBtn(wrapper, '全文编辑', true).trigger('click')
    await flushPromises()
    const textarea = wrapper.find('textarea')
    expect(textarea.exists()).toBe(true)

    // 保留冲突标记的内容
    await textarea.setValue('a\n<<<<<<< HEAD\nb\n=======\nc\n>>>>>>> x')
    // 用户取消确认
    ElMessageBox.confirm.mockRejectedValueOnce('cancel')

    await findBtn(wrapper, '保存并标记已解决').trigger('click')
    await flushPromises()

    // 软校验：确认框弹出，用户取消 → 不保存
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    expect(SaveConflictResolution).not.toHaveBeenCalled()
  })

  it('全文编辑模式保存仍含标记时确认强制保存 → 保存成功', async () => {
    const { SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    const { ElMessageBox } = await import('element-plus')
    wrapper = await mountWithContent(MARKER_TEXT)

    await findBtn(wrapper, '全文编辑', true).trigger('click')
    await flushPromises()
    await wrapper.find('textarea').setValue('<<<<<<< HEAD\nkeep\n>>>>>>> x')
    ElMessageBox.confirm.mockResolvedValueOnce('confirm')

    await findBtn(wrapper, '保存并标记已解决').trigger('click')
    await flushPromises()

    expect(SaveConflictResolution).toHaveBeenCalledWith(
      '/repo/A', 'src/a.txt', '<<<<<<< HEAD\nkeep\n>>>>>>> x', 'UTF-8'
    )
  })

  it('二进制文件：不渲染编辑器，提示走选边或外部工具', async () => {
    const { GetConflictFileContent } = await import('../../../wailsjs/go/main/App')
    GetConflictFileContent.mockResolvedValue({
      content: '', encoding: 'UTF-8', binary: true, tooLarge: false
    })
    wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
    expect(wrapper.text()).toContain('二进制冲突文件不支持内嵌编辑')
    // 无保存按钮（footer 保存按钮 v-if 拦截）
    expect(findBtn(wrapper, '保存并标记已解决')).toBeUndefined()
  })

  it('超大文件：同样拦截不进编辑器', async () => {
    const { GetConflictFileContent } = await import('../../../wailsjs/go/main/App')
    GetConflictFileContent.mockResolvedValue({
      content: '', encoding: 'UTF-8', binary: false, tooLarge: true
    })
    wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.text()).toContain('文件过大')
  })

  it('无标记文件（R3）：直接进全文编辑模式', async () => {
    const { SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    wrapper = await mountWithContent('外部已改干净的文本\n第二行')

    // 无冲突块卡片，直接全文编辑 textarea
    expect(wrapper.find('.conflict-block').exists()).toBe(false)
    expect(wrapper.text()).toContain('未检测到冲突标记')
    expect(wrapper.find('textarea').exists()).toBe(true)
    expect(wrapper.find('textarea').element.value).toBe('外部已改干净的文本\n第二行')

    await findBtn(wrapper, '保存并标记已解决').trigger('click')
    await flushPromises()

    expect(SaveConflictResolution).toHaveBeenCalledWith(
      '/repo/A', 'src/a.txt', '外部已改干净的文本\n第二行', 'UTF-8'
    )
  })

  it('GBK 编码文件：保存回传原编码', async () => {
    const { SaveConflictResolution } = await import('../../../wailsjs/go/main/App')
    const { GetConflictFileContent } = await import('../../../wailsjs/go/main/App')
    GetConflictFileContent.mockResolvedValue({
      content: '无标记文本', encoding: 'GBK', binary: false, tooLarge: false
    })
    wrapper = mountDialog()
    await flushPromises()

    await findBtn(wrapper, '保存并标记已解决').trigger('click')
    await flushPromises()

    expect(SaveConflictResolution).toHaveBeenCalledWith('/repo/A', 'src/a.txt', '无标记文本', 'GBK')
  })

  it('rebase 冲突：顶部显示 ours 语义反转提示', async () => {
    wrapper = await mountWithContent(MARKER_TEXT, { conflictType: 'rebase' })

    expect(wrapper.text()).toContain('变基目标侧')
  })

  it('加载失败：显示错误信息且无保存按钮', async () => {
    const { GetConflictFileContent } = await import('../../../wailsjs/go/main/App')
    GetConflictFileContent.mockRejectedValue(new Error('boom'))
    wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.find('.resolver-error').text()).toContain('boom')
    expect(findBtn(wrapper, '保存并标记已解决')).toBeUndefined()
  })

  it('「两者都保留」与「撤销决策」', async () => {
    wrapper = await mountWithContent('<<<<<<< HEAD\no\n=======\nt\n>>>>>>> x')

    await findBtn(wrapper, '两者都保留', true).trigger('click')
    expect(wrapper.find('.preview-code').text()).toContain('o')
    expect(wrapper.find('.preview-code').text()).toContain('t')

    // 撤销决策回到未决策态
    await findBtn(wrapper, '撤销决策', true).trigger('click')
    expect(wrapper.text()).not.toContain('已保留两者')
  })
})
