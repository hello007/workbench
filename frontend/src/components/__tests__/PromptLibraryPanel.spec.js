import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PromptLibraryPanel from '../PromptLibraryPanel.vue'

const mocks = vi.hoisted(() => ({
  message: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  confirm: vi.fn(() => Promise.resolve()),
  list: vi.fn(),
  add: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  writeText: vi.fn(() => Promise.resolve())
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: mocks.message,
    ElMessageBox: { confirm: (...args) => mocks.confirm(...args) }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  ListPrompts: (...args) => mocks.list(...args),
  AddPrompt: (...args) => mocks.add(...args),
  UpdatePrompt: (...args) => mocks.update(...args),
  DeletePrompt: (...args) => mocks.remove(...args)
}))

vi.mock('@element-plus/icons-vue', () => {
  const stub = (name) => ({ name, template: `<span class="i-${name}" />` })
  return {
    Collection: stub('collection'),
    Plus: stub('plus'),
    CopyDocument: stub('copy'),
    Edit: stub('edit'),
    Delete: stub('delete')
  }
})

const stubs = {
  'el-icon': { template: '<i><slot /></i>' },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'loading', 'size', 'type', 'link'],
    emits: ['click']
  },
  'el-input': {
    template: `<textarea v-if="type === 'textarea'" :value="modelValue" :placeholder="placeholder" @input="$emit('update:modelValue', $event.target.value)" />
      <input v-else :value="modelValue" :placeholder="placeholder" @input="$emit('update:modelValue', $event.target.value)" />`,
    props: ['modelValue', 'placeholder', 'type', 'rows', 'clearable', 'size', 'resize', 'autosize', 'maxlength', 'showWordLimit'],
    emits: ['update:modelValue']
  },
  'el-tooltip': { template: '<span><slot /></span>', props: ['content', 'placement', 'showAfter'] },
  'el-dialog': {
    template: `<div v-if="modelValue" class="el-dialog">
      <div class="dialog-title">{{ title }}</div>
      <slot />
      <slot name="footer" />
    </div>`,
    props: ['modelValue', 'title', 'width'],
    emits: ['update:modelValue', 'opened', 'closed'],
    watch: {
      // 对齐真实 EP：打开后 emit opened（延后一 tick 等输入框渲染完），关闭后 emit closed
      modelValue(value) {
        if (value) this.$nextTick(() => this.$emit('opened'))
        else this.$emit('closed')
      }
    }
  }
}

const samplePrompts = [
  {
    id: 'p1',
    title: '代码评审',
    content: '请评审 {{文件路径}} 的改动',
    createdAt: 1000,
    updatedAt: 2000
  },
  {
    id: 'p2',
    title: '周报模板',
    content: '本周完成：\n- 事项',
    createdAt: 500,
    updatedAt: 800
  }
]

const mountPanel = async (prompts = samplePrompts) => {
  mocks.list.mockResolvedValue(prompts)
  const wrapper = mount(PromptLibraryPanel, { global: { stubs } })
  await flushPromises()
  return wrapper
}

describe('PromptLibraryPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.confirm.mockImplementation(() => Promise.resolve())
    mocks.writeText.mockImplementation(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: (...args) => mocks.writeText(...args) },
      configurable: true
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('挂载后渲染列表、时间与占位符徽标', async () => {
    const wrapper = await mountPanel()
    expect(mocks.list).toHaveBeenCalledTimes(1)

    const items = wrapper.findAll('.prompt-item')
    expect(items).toHaveLength(2)
    expect(items[0].text()).toContain('代码评审')
    expect(items[0].text()).toContain('1 个占位符')
    expect(items[0].find('.prompt-time').text()).not.toBe('--')
    expect(items[1].text()).toContain('周报模板')
    expect(wrapper.find('.panel-subtitle').text()).toContain('共 2 条')
  })

  it('关键字同时匹配标题与正文，清空后恢复全量', async () => {
    const wrapper = await mountPanel()
    const search = wrapper.find('.prompt-search')

    await search.setValue('周报')
    expect(wrapper.findAll('.prompt-item')).toHaveLength(1)

    await search.setValue('文件路径')
    expect(wrapper.findAll('.prompt-item')).toHaveLength(1)
    expect(wrapper.find('.prompt-item').text()).toContain('代码评审')

    await search.setValue('不存在的关键字')
    expect(wrapper.find('.prompt-empty').text()).toContain('没有匹配')

    await search.setValue('')
    expect(wrapper.findAll('.prompt-item')).toHaveLength(2)
  })

  it('无占位符时点击复制直接写剪贴板，不弹窗', async () => {
    const wrapper = await mountPanel()
    const item = wrapper.findAll('.prompt-item')[1]

    await item.find('[data-action="copy"]').trigger('click')
    await flushPromises()

    expect(mocks.writeText).toHaveBeenCalledWith('本周完成：\n- 事项')
    expect(mocks.message.success).toHaveBeenCalled()
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })

  it('有占位符时弹窗按去重后的名字生成输入框', async () => {
    const wrapper = await mountPanel([
      { id: 'p3', title: '多占位符', content: '{{甲}} {{乙}} {{甲}}', createdAt: 1, updatedAt: 1 }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()

    const rows = wrapper.findAll('.prompt-fill-row')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('甲')
    expect(rows[1].text()).toContain('乙')
  })

  it('部分填写后替换并复制，未填项保留原样', async () => {
    const wrapper = await mountPanel([
      { id: 'p3', title: '多占位符', content: '{{甲}} / {{乙}}', createdAt: 1, updatedAt: 1 }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()

    const inputs = wrapper.findAll('.prompt-fill-row textarea')
    await inputs[0].setValue('A')
    await wrapper.find('[data-action="copy-filled"]').trigger('click')
    await flushPromises()

    expect(mocks.writeText).toHaveBeenCalledWith('A / {{乙}}')
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })

  it('填写弹窗渲染实时预览，按填写值/默认值/未填写分类高亮', async () => {
    const wrapper = await mountPanel([
      {
        id: 'p4',
        title: '预览',
        content: '称呼 {{客户名称}}，页数 {{页数:1}}，时间 {{时间}}',
        createdAt: 1,
        updatedAt: 1
      }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()

    const preview = wrapper.find('[data-preview]')
    expect(preview.exists()).toBe(true)
    expect(preview.text()).toContain('实际内容预览')

    const body = preview.find('.prompt-fill-preview-body')
    expect(body.find('.ph-default').text()).toBe('1')
    expect(body.find('.ph-default').attributes('title')).toBe('使用默认值')
    expect(body.find('.ph-filled').exists()).toBe(false)

    const unfilled = body.findAll('.ph-unfilled')
    expect(unfilled).toHaveLength(2)
    expect(unfilled[0].text()).toBe('{{客户名称}}')
    expect(unfilled[0].attributes('title')).toBe('未填写，保留占位符')
    expect(body.text()).toBe('称呼 {{客户名称}}，页数 1，时间 {{时间}}')

    // 输入后预览随 fillValues 实时更新
    await wrapper.find('[data-fill-name="客户名称"]').setValue('张三')
    await flushPromises()

    expect(body.find('.ph-filled').text()).toBe('张三')
    expect(body.findAll('.ph-unfilled')).toHaveLength(1)
    expect(body.text()).toBe('称呼 张三，页数 1，时间 {{时间}}')
  })

  it('复制原文不受预览影响，仍输出占位符原文', async () => {
    const wrapper = await mountPanel([
      { id: 'p4', title: '预览', content: '称呼 {{客户名称}}', createdAt: 1, updatedAt: 1 }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-preview]').exists()).toBe(true)
    await wrapper.find('[data-fill-name="客户名称"]').setValue('张三')
    await wrapper.find('[data-action="copy-original"]').trigger('click')
    await flushPromises()

    expect(mocks.writeText).toHaveBeenCalledWith('称呼 {{客户名称}}')
  })

  it('占位符有默认值时填写框提示默认值，无默认值沿用原文案', async () => {
    const wrapper = await mountPanel([
      {
        id: 'p3',
        title: '默认值',
        content: '页数 {{页数:1}}，称呼 {{客户名称}}',
        createdAt: 1,
        updatedAt: 1
      }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()

    const rows = wrapper.findAll('.prompt-fill-row')
    expect(rows).toHaveLength(2)
    expect(rows[0].find('textarea').attributes('placeholder'))
      .toBe('输入「页数」的内容（可留空，默认 1）')
    expect(rows[1].find('textarea').attributes('placeholder'))
      .toBe('输入「客户名称」的内容（可留空）')
  })

  it('占位符未填时用默认值替换', async () => {
    const wrapper = await mountPanel([
      { id: 'p3', title: '默认值', content: '页数 {{页数:1}}', createdAt: 1, updatedAt: 1 }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-action="copy-filled"]').trigger('click')
    await flushPromises()

    expect(mocks.writeText).toHaveBeenCalledWith('页数 1')
  })

  it('占位符填写值覆盖默认值', async () => {
    const wrapper = await mountPanel([
      { id: 'p3', title: '默认值', content: '页数 {{页数:1}}', createdAt: 1, updatedAt: 1 }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('.prompt-fill-row textarea')[0].setValue('3')
    await wrapper.find('[data-action="copy-filled"]').trigger('click')
    await flushPromises()

    expect(mocks.writeText).toHaveBeenCalledWith('页数 3')
  })

  it('复制原文保留含默认值的占位符写法', async () => {
    const wrapper = await mountPanel([
      { id: 'p3', title: '默认值', content: '页数 {{页数:1}}', createdAt: 1, updatedAt: 1 }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-action="copy-original"]').trigger('click')
    await flushPromises()

    expect(mocks.writeText).toHaveBeenCalledWith('页数 {{页数:1}}')
  })

  it('复制原文跳过替换', async () => {
    const wrapper = await mountPanel([
      { id: 'p3', title: '多占位符', content: '前缀 {{甲}}', createdAt: 1, updatedAt: 1 }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('.prompt-fill-row textarea')[0].setValue('A')
    await wrapper.find('[data-action="copy-original"]').trigger('click')
    await flushPromises()

    expect(mocks.writeText).toHaveBeenCalledWith('前缀 {{甲}}')
  })

  it('剪贴板写入失败时提示错误且保留弹窗', async () => {
    mocks.writeText.mockRejectedValue(new Error('denied'))
    const wrapper = await mountPanel([
      { id: 'p3', title: '多占位符', content: '{{甲}}', createdAt: 1, updatedAt: 1 }
    ])

    await wrapper.find('[data-action="copy"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-action="copy-filled"]').trigger('click')
    await flushPromises()

    expect(mocks.message.error).toHaveBeenCalled()
    expect(wrapper.find('.el-dialog').exists()).toBe(true)
  })

  it('新建：空标题/空正文被前端拦截，不调用后端', async () => {
    const wrapper = await mountPanel()

    await wrapper.find('[data-action="create"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-action="save"]').trigger('click')
    expect(mocks.message.warning).toHaveBeenCalledWith('标题不能为空')

    await wrapper.find('[data-field="title"]').setValue('新话术')
    await wrapper.find('[data-action="save"]').trigger('click')
    expect(mocks.message.warning).toHaveBeenCalledWith('正文不能为空')
    expect(mocks.add).not.toHaveBeenCalled()
  })

  it('新建保存成功后刷新列表', async () => {
    const wrapper = await mountPanel([])
    mocks.add.mockResolvedValue({ id: 'new' })

    await wrapper.find('[data-action="create"]').trigger('click')
    await wrapper.find('[data-field="title"]').setValue('新话术')
    await wrapper.find('[data-field="content"]').setValue('正文 {{变量}}')
    await wrapper.find('[data-action="save"]').trigger('click')
    await flushPromises()

    expect(mocks.add).toHaveBeenCalledWith('新话术', '正文 {{变量}}')
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it('编辑回填并在保存时调用 UpdatePrompt', async () => {
    const wrapper = await mountPanel()
    mocks.update.mockResolvedValue()

    await wrapper.findAll('.prompt-item')[0].find('[data-action="edit"]').trigger('click')
    await flushPromises()

    const dialog = wrapper.find('.el-dialog')
    expect(dialog.text()).toContain('编辑提示词')
    expect(dialog.find('[data-field="title"]').element.value).toBe('代码评审')

    await dialog.find('[data-field="content"]').setValue('改后的正文')
    await dialog.find('[data-action="save"]').trigger('click')
    await flushPromises()

    expect(mocks.update).toHaveBeenCalledWith('p1', '代码评审', '改后的正文')
  })

  it('保存命中 E_PROMPT_NOT_FOUND 时关闭弹窗并刷新列表', async () => {
    const wrapper = await mountPanel()
    mocks.update.mockRejectedValue({ code: 'E_PROMPT_NOT_FOUND', message: '提示词不存在' })

    await wrapper.findAll('.prompt-item')[0].find('[data-action="edit"]').trigger('click')
    await flushPromises()
    await wrapper.find('.el-dialog [data-action="save"]').trigger('click')
    await flushPromises()

    expect(mocks.update).toHaveBeenCalled()
    expect(mocks.message.warning).toHaveBeenCalledWith('提示词不存在')
    // 关闭编辑弹窗 + 重新拉取列表（首挂载 1 次 + 刷新 1 次）
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it('删除命中 E_PROMPT_NOT_FOUND 时刷新列表收敛', async () => {
    const wrapper = await mountPanel()
    mocks.remove.mockRejectedValue({ code: 'E_PROMPT_NOT_FOUND', message: '提示词不存在' })

    await wrapper.findAll('.prompt-item')[0].find('[data-action="delete"]').trigger('click')
    await flushPromises()

    expect(mocks.message.warning).toHaveBeenCalledWith('提示词不存在')
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it('删除需确认，确认后调用 DeletePrompt 并刷新', async () => {
    const wrapper = await mountPanel()

    await wrapper.findAll('.prompt-item')[0].find('[data-action="delete"]').trigger('click')
    await flushPromises()

    expect(mocks.confirm).toHaveBeenCalled()
    expect(mocks.remove).toHaveBeenCalledWith('p1')
    expect(mocks.list).toHaveBeenCalledTimes(2)
  })

  it('删除确认框取消时不调用后端', async () => {
    mocks.confirm.mockImplementation(() => Promise.reject(new Error('cancel')))
    const wrapper = await mountPanel()

    await wrapper.findAll('.prompt-item')[0].find('[data-action="delete"]').trigger('click')
    await flushPromises()

    expect(mocks.remove).not.toHaveBeenCalled()
  })

  it('长正文显示展开/收起切换', async () => {
    const wrapper = await mountPanel()
    const item = wrapper.findAll('.prompt-item')[1]
    const content = item.find('.prompt-content')

    expect(content.classes()).not.toContain('is-expanded')
    await item.find('[data-action="toggle-expand"]').trigger('click')
    expect(item.find('.prompt-content').classes()).toContain('is-expanded')
  })

  it('加载失败走 handleError 提示', async () => {
    mocks.list.mockRejectedValue({ code: 'E_RPC_INTERNAL', message: '加载失败' })
    const wrapper = mount(PromptLibraryPanel, { global: { stubs } })
    await flushPromises()

    expect(mocks.message.error).toHaveBeenCalled()
    expect(wrapper.findAll('.prompt-item')).toHaveLength(0)
  })

  it('新建弹窗打开后聚焦标题输入框，光标在末尾', async () => {
    const selSpy = vi.spyOn(HTMLInputElement.prototype, 'setSelectionRange')
    mocks.list.mockResolvedValue([])
    // attachTo 后 focus 才真正作用于 document.activeElement，避免空输入框恒为 0 的假阳性断言
    const wrapper = mount(PromptLibraryPanel, { global: { stubs }, attachTo: document.body })
    await flushPromises()

    await wrapper.find('[data-action="create"]').trigger('click')
    await flushPromises()

    const title = wrapper.find('[data-field="title"]').element
    expect(document.activeElement).toBe(title)
    expect(selSpy).toHaveBeenCalledWith(0, 0)
    wrapper.unmount()
  })

  it('编辑弹窗打开后光标落在原标题末尾', async () => {
    // jsdom 写入 value 时会把光标放到末尾，以显式 setSelectionRange 入参为准
    const selSpy = vi.spyOn(HTMLInputElement.prototype, 'setSelectionRange')
    const wrapper = await mountPanel()

    await wrapper.findAll('.prompt-item')[0].find('[data-action="edit"]').trigger('click')
    await flushPromises()

    const title = wrapper.find('[data-field="title"]').element
    expect(title.value).toBe('代码评审')
    expect(selSpy).toHaveBeenCalledWith('代码评审'.length, '代码评审'.length)
  })

  it('Ctrl+S / Cmd+S 在标题或正文输入时保存并阻止默认行为', async () => {
    const wrapper = await mountPanel([])
    mocks.add.mockResolvedValue({ id: 'new' })

    await wrapper.find('[data-action="create"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-field="title"]').setValue('快捷键话术')
    await wrapper.find('[data-field="content"]').setValue('正文内容')

    const ctrlS = new KeyboardEvent('keydown', { key: 's', ctrlKey: true, bubbles: true, cancelable: true })
    wrapper.find('[data-field="title"]').element.dispatchEvent(ctrlS)
    await flushPromises()

    expect(ctrlS.defaultPrevented).toBe(true)
    expect(mocks.add).toHaveBeenCalledWith('快捷键话术', '正文内容')

    // 再次新建后从正文输入框按 Cmd+S，验证容器级监听覆盖两个输入框
    await wrapper.find('[data-action="create"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-field="title"]').setValue('第二段')
    await wrapper.find('[data-field="content"]').setValue('正文二')
    const cmdS = new KeyboardEvent('keydown', { key: 'S', metaKey: true, bubbles: true, cancelable: true })
    wrapper.find('[data-field="content"]').element.dispatchEvent(cmdS)
    await flushPromises()

    expect(cmdS.defaultPrevented).toBe(true)
    expect(mocks.add).toHaveBeenCalledWith('第二段', '正文二')
  })

  it('保存在途时关闭并重新打开，完成回调不关闭新弹窗也不清空新草稿', async () => {
    let resolveAdd
    mocks.add.mockImplementationOnce(() => new Promise((resolve) => { resolveAdd = resolve }))
    const wrapper = await mountPanel([])

    await wrapper.find('[data-action="create"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-field="title"]').setValue('第一条')
    await wrapper.find('[data-field="content"]').setValue('正文一')
    await wrapper.find('[data-action="save"]').trigger('click')

    // 保存未完成：关闭弹窗（ESC / X 均走 @closed 复位）
    wrapper.findComponent('.el-dialog').vm.$emit('update:modelValue', false)
    await flushPromises()

    // 重新打开并输入新内容
    await wrapper.find('[data-action="create"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-field="title"]').setValue('第二条')

    resolveAdd({ id: 'new' })
    await flushPromises()

    expect(wrapper.find('.el-dialog').exists()).toBe(true)
    expect(wrapper.find('[data-field="title"]').element.value).toBe('第二条')
  })

  it('弹窗关闭即复位草稿，重新打开不带入上次内容', async () => {
    const wrapper = await mountPanel([])

    await wrapper.find('[data-action="create"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-field="title"]').setValue('未保存的标题')
    await wrapper.find('[data-field="content"]').setValue('未保存的正文')

    // 真实 EP 由 ESC/X/取消按钮关闭弹窗并触发 closed，stub 手动置 false 模拟
    wrapper.findComponent('.el-dialog').vm.$emit('update:modelValue', false)
    await flushPromises()

    await wrapper.find('[data-action="create"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-field="title"]').element.value).toBe('')
    expect(wrapper.find('[data-field="content"]').element.value).toBe('')
    expect(wrapper.find('.dialog-title').text()).toBe('新建提示词')
  })
})
