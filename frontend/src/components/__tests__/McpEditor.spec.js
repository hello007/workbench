import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import McpEditor from '../McpEditor.vue'

const stubs = {
  'el-input': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @change="$emit(\'change\', $event.target.value)" />',
    props: ['modelValue', 'placeholder'],
    emits: ['update:modelValue', 'change']
  },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'size', 'link'],
    emits: ['click']
  },
  'el-select': {
    template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><slot /></select>',
    props: ['modelValue', 'class'],
    emits: ['update:modelValue']
  },
  'el-option': { template: '<option :value="value">{{ label }}</option>', props: ['label', 'value'] },
  // EnvEditor 子组件隔离，避免其内部 watch/rows 干扰
  EnvEditor: { template: '<div class="env-editor-stub" />', props: ['modelValue'] }
}

function createWrapper(model) {
  return mount(McpEditor, {
    props: { modelValue: model },
    global: { stubs }
  })
}

describe('McpEditor.vue', () => {
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

  it('渲染已有 server 卡片', async () => {
    const model = reactive({ mcpServers: { fs: { type: 'http', url: 'https://x', headers: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.findAll('.server-card').length).toBe(1)
  })

  it('http 分支渲染 url 输入 + 请求头子标题', async () => {
    const model = reactive({ mcpServers: { fs: { type: 'http', url: '', headers: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.text()).toContain('请求头')
  })

  it('stdio 分支渲染 command + args + env 子标题', async () => {
    const model = reactive({ mcpServers: { s1: { type: 'stdio', command: '', args: [], env: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    // stdio 子标题（div 文本）；http 分支为「请求头」，stdio 为「环境变量」
    expect(wrapper.text()).toContain('环境变量')
    expect(wrapper.text()).not.toContain('请求头')
  })

  it('addServer：默认名 server，重名时递增 server1/server2', async () => {
    const model = reactive({ mcpServers: { server: { type: 'http', url: '', headers: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click')
    expect(model.mcpServers.server1).toBeTruthy()
    // 再加一个 → server2
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click')
    expect(model.mcpServers.server2).toBeTruthy()
  })

  it('addServer：model 为 null 时初始化结构', async () => {
    wrapper = createWrapper(null)
    await nextTick()
    await wrapper.findAll('button').find(b => b.text().includes('新增')).trigger('click')
    const emits = wrapper.emitted('update:modelValue')
    expect(emits).toBeTruthy()
    const last = emits.at(-1)[0]
    expect(last.mcpServers).toBeTruthy()
    expect(Object.keys(last.mcpServers).length).toBeGreaterThan(0)
  })

  it('removeServer：删除指定 server', async () => {
    const model = reactive({ mcpServers: { fs: { type: 'http', url: '', headers: {} }, s1: { type: 'stdio', command: '', env: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    // 第一张卡片的删除按钮
    await wrapper.findAll('button').filter(b => b.text() === '删除')[0].trigger('click')
    await nextTick()
    expect(model.mcpServers.fs).toBeUndefined()
    expect(model.mcpServers.s1).toBeTruthy()
  })

  it('onNameChange：重命名 map key（删旧加新）', async () => {
    const model = reactive({ mcpServers: { old: { type: 'http', url: '', headers: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    const nameInput = wrapper.findAll('input')[0]
    await nameInput.setValue('newname')
    await nameInput.trigger('change')
    await nextTick()
    expect(model.mcpServers.old).toBeUndefined()
    expect(model.mcpServers.newname).toBeTruthy()
  })

  it('onNameChange：新名为空时忽略', async () => {
    const model = reactive({ mcpServers: { old: { type: 'http', url: '', headers: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    const nameInput = wrapper.findAll('input')[0]
    await nameInput.setValue('')
    await nameInput.trigger('change')
    await nextTick()
    expect(model.mcpServers.old).toBeTruthy()
  })

  it('onNameChange：目标名已存在时忽略（避免覆盖）', async () => {
    const model = reactive({ mcpServers: { a: { type: 'http', url: '', headers: {} }, b: { type: 'stdio', command: '', env: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    const nameInput = wrapper.findAll('input')[0]
    await nameInput.setValue('b')
    await nameInput.trigger('change')
    await nextTick()
    // a 仍在，b 未被覆盖
    expect(model.mcpServers.a).toBeTruthy()
    expect(model.mcpServers.b.type).toBe('stdio')
  })

  it('onTypeChange：切换 server 类型', async () => {
    const model = reactive({ mcpServers: { fs: { type: 'http', url: '', headers: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    const select = wrapper.find('select')
    await select.setValue('stdio')
    await nextTick()
    expect(model.mcpServers.fs.type).toBe('stdio')
  })

  it('splitArgs：逗号分隔 + 中英文逗号 + 去空白', async () => {
    const model = reactive({ mcpServers: { s: { type: 'stdio', command: 'npx', args: [], env: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    // stdio 分支输入顺序：name(0) → command(1) → args(2)
    const argsInput = wrapper.findAll('input')[2]
    await argsInput.setValue('-y, @mcp/fs，D:\\docs')
    await argsInput.trigger('change')
    await nextTick()
    expect(model.mcpServers.s.args).toEqual(['-y', '@mcp/fs', 'D:\\docs'])
  })

  it('validationErrors：http 缺 url 报错', async () => {
    const model = reactive({ mcpServers: { fs: { type: 'http', url: '', headers: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.vm.validationErrors).toEqual(expect.arrayContaining([expect.stringContaining('fs')]))
    expect(wrapper.find('.warn-box').exists()).toBe(true)
  })

  it('validationErrors：stdio 缺 command 报错', async () => {
    const model = reactive({ mcpServers: { s: { type: 'stdio', command: '', env: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.vm.validationErrors).toEqual(expect.arrayContaining([expect.stringContaining('stdio')]))
  })

  it('validationErrors：字段齐全时无告警', async () => {
    const model = reactive({ mcpServers: { fs: { type: 'http', url: 'https://x', headers: {} } } })
    wrapper = createWrapper(model)
    await nextTick()
    expect(wrapper.vm.validationErrors.length).toBe(0)
    expect(wrapper.find('.warn-box').exists()).toBe(false)
  })
})
