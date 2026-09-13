import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ElementPlus, { ElMessage, ElLoading } from 'element-plus'
import RepoConfigImportDialog from '../RepoConfigImportDialog.vue'

// ElMessage 部分 mock：断言错误提示（error.js 经此模块弹提示）
vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: {
      error: vi.fn(),
      success: vi.fn(),
      warning: vi.fn(),
      info: vi.fn()
    }
  }
})

// 导入预览：1 新增目录 + 1 冲突目录 + 1 新增收藏 + 1 冲突收藏 + 2 非法项
const previewPayload = {
  newDirectories: [{ name: '新目录', path: 'D:/ws/new-dir', isDefault: false }],
  conflictDirectories: [{ name: '冲突目录', path: 'D:/ws/conflict', isDefault: true }],
  newFavorites: [{ path: 'D:/ws/fav-new', alias: '新收藏', group: '分组A', createdAt: 1 }],
  conflictFavorites: [{ path: 'D:/ws/fav-conflict', alias: '冲突收藏', group: '默认', createdAt: 2 }],
  invalid: [
    { kind: 'directory', name: 'D:/ws/ghost', reason: '路径在本机不存在' },
    { kind: 'favorite', name: '', reason: '路径为空' }
  ]
}

const openDialogMock = vi.fn(() => Promise.resolve('D:/tmp/repo_config.json'))
const readFileBytesMock = vi.fn(() => Promise.resolve({ base64: btoa('{"manifestVersion":1}'), error: '', tooLarge: false }))
const previewMock = vi.fn(() => Promise.resolve(previewPayload))
const applyMock = vi.fn(() => Promise.resolve({ added: 2, overwritten: 0, skipped: 2, failed: 1, failedReasons: ['收藏 D:/x：收藏夹已满（最多 100 条）'] }))

vi.mock('../../../wailsjs/go/main/App', () => ({
  OpenFileDialog: (...args) => openDialogMock(...args),
  ReadFileBytes: (...args) => readFileBytesMock(...args),
  PreviewRepoConfigImport: (...args) => previewMock(...args),
  ApplyRepoConfigImport: (...args) => applyMock(...args)
}))

// stub 弹窗外壳与单选组（teleport/transition 在 jsdom 下的问题），其余走真实 Element Plus
const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="dlg"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'appendToBody', 'destroyOnClose'],
    emits: ['update:modelValue']
  },
  'el-radio-group': {
    template: `<div class="radio-group" role="radiogroup"><slot /></div>`,
    props: ['modelValue', 'size'],
    emits: ['update:modelValue']
  },
  'el-radio': {
    template: `<label class="radio" @click="$parent.$emit('update:modelValue', value)"><slot /></label>`,
    props: ['value', 'size'],
    emits: ['update:modelValue']
  },
  'el-tag': { template: '<span class="tag"><slot /></span>', props: ['size', 'type'] }
}

function createWrapper() {
  return mount(RepoConfigImportDialog, {
    props: { visible: false },
    global: { plugins: [ElementPlus, ElLoading], stubs }
  })
}

const findBtn = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().includes(text))

describe('RepoConfigImportDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('打开即选文件并展示预览五段（新增/冲突/非法）', async () => {
    const wrapper = createWrapper()
    await wrapper.setProps({ visible: true })
    await flushPromises()

    expect(openDialogMock).toHaveBeenCalledWith('选择仓库列表配置文件', [
      { DisplayName: 'JSON 文件', Pattern: '*.json' }
    ])
    expect(readFileBytesMock).toHaveBeenCalledWith('D:/tmp/repo_config.json')
    expect(previewMock).toHaveBeenCalledWith('{"manifestVersion":1}')
    const html = wrapper.html()
    expect(html).toContain('将新增工作目录（1）')
    expect(html).toContain('冲突工作目录（1）')
    expect(html).toContain('将新增收藏（1）')
    expect(html).toContain('冲突收藏（1）')
    expect(html).toContain('非法项（2）')
    expect(html).toContain('路径在本机不存在')
  })

  it('用户取消选文件：静默关闭且不读文件', async () => {
    openDialogMock.mockResolvedValueOnce('')
    const wrapper = createWrapper()
    await wrapper.setProps({ visible: true })
    await flushPromises()

    expect(readFileBytesMock).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:visible')).toEqual([[false]])
  })

  it('读取文件失败：提示错误并关闭', async () => {
    readFileBytesMock.mockResolvedValueOnce({ base64: '', error: '文件不存在', tooLarge: false })
    const wrapper = createWrapper()
    await wrapper.setProps({ visible: true })
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith('读取文件失败: 文件不存在')
    expect(wrapper.emitted('update:visible')).toEqual([[false]])
  })

  it('全局校验失败（非法 JSON）：handleError 分流 error 提示并关闭', async () => {
    previewMock.mockRejectedValueOnce({ code: 'E_REPO_CONFIG_INVALID_JSON', message: '导入文件不是合法的仓库列表配置 JSON' })
    const wrapper = createWrapper()
    await wrapper.setProps({ visible: true })
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(
      '导入解析失败: 导入文件不是合法的仓库列表配置 JSON'
    )
    expect(wrapper.emitted('update:visible')).toEqual([[false]])
  })

  it('冲突项默认跳过；决策后确认导入按决策表调用后端并展示结果汇总', async () => {
    const wrapper = createWrapper()
    await wrapper.setProps({ visible: true })
    await flushPromises()

    // 默认决策 = skip
    expect(applyMock).not.toHaveBeenCalled()
    // 把冲突目录决策改为 saveAsNew、冲突收藏改为 overwrite
    wrapper.vm.dirDecisions['D:/ws/conflict'] = 'saveAsNew'
    wrapper.vm.favDecisions['D:/ws/fav-conflict'] = 'overwrite'

    await findBtn(wrapper, '确认导入').trigger('click')
    await flushPromises()

    expect(applyMock).toHaveBeenCalledWith('{"manifestVersion":1}', {
      directories: { 'D:/ws/conflict': 'saveAsNew' },
      favorites: { 'D:/ws/fav-conflict': 'overwrite' }
    })
    // 切换到结果态：计数 + 失败明细
    const html = wrapper.html()
    expect(html).toContain('新增 2')
    expect(html).toContain('覆盖 0')
    expect(html).toContain('跳过 2')
    expect(html).toContain('失败 1')
    expect(html).toContain('收藏夹已满')
    // 已通知父组件刷新
    expect(wrapper.emitted('imported')).toBeTruthy()
  })

  it('无可导入项时确认按钮禁用；完成按钮关闭对话框', async () => {
    previewMock.mockResolvedValueOnce({
      newDirectories: [],
      conflictDirectories: [],
      newFavorites: [],
      conflictFavorites: [],
      invalid: []
    })
    const wrapper = createWrapper()
    await wrapper.setProps({ visible: true })
    await flushPromises()

    const confirm = findBtn(wrapper, '确认导入')
    expect(confirm.attributes('disabled')).toBeDefined()

    // 结果态需要先走 apply；这里直接验证完成按钮在结果态关闭
    wrapper.vm.step = 'result'
    wrapper.vm.result = { added: 0, overwritten: 0, skipped: 0, failed: 0 }
    await flushPromises()
    await findBtn(wrapper, '完成').trigger('click')
    expect(wrapper.emitted('update:visible')).toEqual([[false]])
  })

  it('执行导入失败：handleError 提示且不切换结果态', async () => {
    applyMock.mockRejectedValueOnce({ code: 'E_REPO_CONFIG_INVALID_JSON', message: '导入文件不是合法的仓库列表配置 JSON' })
    const wrapper = createWrapper()
    await wrapper.setProps({ visible: true })
    await flushPromises()

    await findBtn(wrapper, '确认导入').trigger('click')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith(
      '导入执行失败: 导入文件不是合法的仓库列表配置 JSON'
    )
    expect(wrapper.vm.step).toBe('preview')
    expect(wrapper.emitted('imported')).toBeFalsy()
  })
})
