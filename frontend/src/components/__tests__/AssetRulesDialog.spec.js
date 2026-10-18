import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetRulesDialog from '../AssetRulesDialog.vue'

// 覆盖面：打开回填当前库规则（excludeDirs/excludeExts 归一存储值）、目录规则
// 增删与查重（忽略大小写）、路径分隔符前端第一防线拦截、后缀三形态归一带点
// 小写（与后端 normalizeAssetExcludeExt 存储口径一致）、保存 execute 分派契约
// （目录原样 + 后缀归一数组）、成功 toast 汇总分型 + saved 事件 + 关弹窗、失败
// handleError 分流弹窗保持、回填「关闭→打开」沿守卫（打开期间 library 对象被
// 宿主替换跳过回填，防覆盖未保存编辑）。
// 宿主职责（UpdateAssetLibraryRules 绑定与刷新链）不经本 spec——宿主接线由
// AssetsPanel.spec 打桩消费。

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve(true)) }
  }
})

import { ElMessage } from 'element-plus'

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'appendToBody'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
    emits: ['click']
  },
  'el-input': {
    template: '<input class="el-input" :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup.enter="$emit(\'keyup.enter\')" />',
    props: ['modelValue', 'placeholder', 'disabled'],
    emits: ['update:modelValue', 'keyup.enter']
  },
  'el-tag': {
    template: '<span class="el-tag">{{ text }}<button v-if="closable" class="tag-close" @click="$emit(\'close\')">×</button></span>',
    // closable 须声明 Boolean 类型：无值 attribute 传给未声明类型的 prop 得空串
    //（falsy），v-if 恒假致关闭按钮不渲染
    props: { closable: { type: Boolean, default: false }, type: String, size: String, disableTransitions: Boolean },
    emits: ['close'],
    computed: {
      // 插槽内容为纯文本（规则值）
      text() {
        return this.$slots.default ? this.$slots.default().map(n => n.children).join('') : ''
      }
    }
  }
}

const libWithRules = {
  id: 'lib-1',
  name: '素材库',
  rootPath: 'D:/assets',
  excludeDirs: ['*草稿*', 'archive'],
  excludeExts: ['.tmp', '.bak']
}

const okScanResult = { total: 5, new: 0, updated: 0, unchanged: 3, failed: 0, encrypted: 0, denied: 0, excluded: 2, lost: 0, revived: 0, removed: 0 }

const createWrapper = async (props = {}) => {
  const wrapper = mount(AssetRulesDialog, {
    props: {
      modelValue: true,
      library: libWithRules,
      execute: vi.fn(() => Promise.resolve(okScanResult)),
      ...props
    },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().trim() === text)

const tagTexts = (wrapper) => wrapper.findAll('.el-tag').map(t => t.text().replace(/×$/, ''))

// 添加按钮按行定位（两个输入行各有同名「添加」按钮，findButton 恒取首个会打错行）
const addDirButton = (wrapper) => wrapper.findAll('.rules-input-row .el-button').at(0)
const addExtButton = (wrapper) => wrapper.findAll('.rules-input-row .el-button').at(1)

const setInputValue = async (wrapper, index, value) => {
  const input = wrapper.findAll('.el-input').at(index)
  input.element.value = value
  await input.trigger('input')
  await flushPromises()
}

describe('AssetRulesDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('打开弹窗回填当前库规则（目录原样 + 后缀归一存储值）', async () => {
    const wrapper = await createWrapper()
    expect(tagTexts(wrapper)).toEqual(['*草稿*', 'archive', '.tmp', '.bak'])
  })

  it('无规则库回填空列表显示「未配置」占位', async () => {
    const wrapper = await createWrapper({ library: { id: 'lib-1', excludeDirs: [], excludeExts: [] } })
    expect(wrapper.findAll('.el-tag')).toHaveLength(0)
    expect(wrapper.findAll('.rules-empty')).toHaveLength(2)
  })

  it('添加目录规则：回车与按钮双通道加入并清空输入', async () => {
    const wrapper = await createWrapper({ library: { id: 'lib-1', excludeDirs: [], excludeExts: [] } })
    await setInputValue(wrapper, 0, '  营销草稿  ')
    await addDirButton(wrapper).trigger('click')
    await flushPromises()
    expect(tagTexts(wrapper)).toContain('营销草稿')
    // 输入清空（首个输入为目录输入行）
    expect(wrapper.findAll('.el-input').at(0).element.value).toBe('')

    await setInputValue(wrapper, 0, 'archive')
    await wrapper.findAll('.el-input').at(0).trigger('keyup.enter')
    await flushPromises()
    expect(tagTexts(wrapper)).toContain('archive')
  })

  it('目录含路径分隔符被拦截（前端第一防线）且空白输入不加入', async () => {
    const execute = vi.fn(() => Promise.resolve(okScanResult))
    const wrapper = await createWrapper({ library: { id: 'lib-1', excludeDirs: [], excludeExts: [] }, execute })
    await setInputValue(wrapper, 0, 'a/b')
    await addDirButton(wrapper).trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('排除文件夹按名称匹配，不能包含路径分隔符')
    expect(tagTexts(wrapper)).toHaveLength(0)
    expect(execute).not.toHaveBeenCalled()

    await setInputValue(wrapper, 0, '   ')
    await addDirButton(wrapper).trigger('click')
    await flushPromises()
    expect(tagTexts(wrapper)).toHaveLength(0)
  })

  it('目录查重忽略大小写（匹配侧大小写不敏感，同形异写视为重复）', async () => {
    const wrapper = await createWrapper({ library: { id: 'lib-1', excludeDirs: ['Archive'], excludeExts: [] } })
    await setInputValue(wrapper, 0, 'archive')
    await addDirButton(wrapper).trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('该规则已存在')
    expect(tagTexts(wrapper)).toEqual(['Archive'])
  })

  it('后缀三形态归一带点小写（tmp / .TMP / *.Zip 与后端存储口径一致），空归一值丢弃', async () => {
    const wrapper = await createWrapper({ library: { id: 'lib-1', excludeDirs: [], excludeExts: [] } })
    await setInputValue(wrapper, 1, 'tmp')
    await addExtButton(wrapper).trigger('click')
    await setInputValue(wrapper, 1, '*.zip')
    await addExtButton(wrapper).trigger('click')
    await flushPromises()
    expect(tagTexts(wrapper)).toEqual(['.tmp', '.zip'])

    // 输入「.」归一为空：丢弃不加入
    await setInputValue(wrapper, 1, '.')
    await addExtButton(wrapper).trigger('click')
    await flushPromises()
    expect(tagTexts(wrapper)).toEqual(['.tmp', '.zip'])
  })

  it('后缀查重基于归一值（.tmp 与 *.tmp 同值不重复加入）', async () => {
    const wrapper = await createWrapper({ library: { id: 'lib-1', excludeDirs: [], excludeExts: ['.tmp'] } })
    await setInputValue(wrapper, 1, '*.tmp')
    await addExtButton(wrapper).trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('该后缀已存在')
    expect(tagTexts(wrapper)).toEqual(['.tmp'])
  })

  it('标签关闭按钮删除规则', async () => {
    const wrapper = await createWrapper()
    await wrapper.findAll('.tag-close').at(0).trigger('click')
    await flushPromises()
    expect(tagTexts(wrapper)).toEqual(['archive', '.tmp', '.bak'])
    await wrapper.findAll('.tag-close').at(1).trigger('click')
    await flushPromises()
    expect(tagTexts(wrapper)).toEqual(['archive', '.bak'])
  })

  it('保存 execute 收目录原样 + 后缀归一数组，成功带扫描汇总 toast + saved 事件 + 关弹窗', async () => {
    const execute = vi.fn(() => Promise.resolve(okScanResult))
    const wrapper = await createWrapper({ execute })
    await findButton(wrapper, '保存并重新扫描').trigger('click')
    await flushPromises()
    expect(execute).toHaveBeenCalledTimes(1)
    expect(execute).toHaveBeenCalledWith(['*草稿*', 'archive'], ['.tmp', '.bak'])
    // excluded=2 非零：toast 带排除汇总
    expect(ElMessage.success).toHaveBeenCalledWith('规则已保存并重新扫描（新排除 2 个文件）')
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('saved')[0][0]).toEqual(okScanResult)
    expect(wrapper.emitted('update:modelValue')).toContainEqual([false])
  })

  it('扫描汇总零值省略（无排除无复活时为通用文案）', async () => {
    const execute = vi.fn(() => Promise.resolve({ total: 3, new: 0, updated: 0, unchanged: 3, failed: 0, encrypted: 0, denied: 0, excluded: 0, lost: 0, revived: 0, removed: 0 }))
    const wrapper = await createWrapper({ execute })
    await findButton(wrapper, '保存并重新扫描').trigger('click')
    await flushPromises()
    expect(ElMessage.success).toHaveBeenCalledWith('规则已保存并重新扫描')
  })

  it('保存失败 handleError 分流：弹窗保持打开、saved 不上报、可改后重试', async () => {
    const execute = vi.fn(() => Promise.reject({ code: 'E_ASSETS_SCAN_IN_PROGRESS', message: '该素材库已有扫描进行中' }))
    const wrapper = await createWrapper({ execute })
    await findButton(wrapper, '保存并重新扫描').trigger('click')
    await flushPromises()
    // 确认型 warning 码经 handleError 分流 warning（不拼前缀）
    expect(ElMessage.warning).toHaveBeenCalledWith('该素材库已有扫描进行中')
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.emitted('update:modelValue') ?? []).not.toContainEqual([false])
    expect(wrapper.find('.stub-dialog').exists()).toBe(true)
  })

  it('弹窗打开期间 library 对象被宿主替换：跳过回填不覆盖未保存编辑', async () => {
    const wrapper = await createWrapper()
    // 用户编辑：删一条目录规则
    await wrapper.findAll('.tag-close').at(0).trigger('click')
    await flushPromises()
    expect(tagTexts(wrapper)).toEqual(['archive', '.tmp', '.bak'])

    // 宿主刷新链（loadLibraries 等）整体替换 library 对象引用：回填仅在
    // 「关闭→打开」沿触发，打开期间替换不覆盖编辑区
    await wrapper.setProps({ library: { ...libWithRules, excludeDirs: ['*草稿*', 'archive', 'other'] } })
    await flushPromises()
    expect(tagTexts(wrapper)).toEqual(['archive', '.tmp', '.bak'])
  })

  it('关闭后重新打开：按「关闭→打开」沿以最新 library 权威数据重新回填', async () => {
    const wrapper = await createWrapper()
    await wrapper.findAll('.tag-close').at(0).trigger('click')
    await flushPromises()

    await wrapper.setProps({ modelValue: false })
    await flushPromises()
    // 关闭期间 library 被替换：opened=false 早退不回填，重开沿才取权威值
    await wrapper.setProps({ library: { ...libWithRules, excludeDirs: ['新建规则'] } })
    await flushPromises()
    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(tagTexts(wrapper)).toEqual(['新建规则', '.tmp', '.bak'])
  })
})
