import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import AiFunctionRunner from '../AiFunctionRunner.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetFileTree: vi.fn()
}))

// Icons: AiFunctionRunner 用 Icons[fn.icon] 动态取，mock 全量图标
vi.mock('@element-plus/icons-vue', () => ({
  MagicStick: { template: '<i class="i-magic" />' },
  Cpu: { template: '<i class="i-cpu" />' },
  Document: { template: '<i class="i-doc" />' }
}))

const stubs = {
  'el-icon': { template: '<i><slot /></i>', props: ['size', 'color'] },
  'el-form': { template: '<form @submit.prevent><slot /></form>', props: ['labelWidth'] },
  'el-form-item': { template: '<div class="el-form-item"><slot /></div>', props: ['label', 'required'] },
  'el-input': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup="$emit(\'keyup\', $event)" /><slot name="append" />',
    props: ['modelValue', 'placeholder', 'type', 'rows', 'size'],
    emits: ['update:modelValue', 'input', 'keyup', 'change']
  },
  'el-button': {
    template: '<button v-bind="$attrs" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['type', 'loading', 'disabled', 'size'],
    emits: ['click']
  },
  'el-tag': {
    template: '<span class="el-tag" @click="$emit(\'close\')"><slot /><span class="tag-close" @click.stop="$emit(\'close\')" /></span>',
    props: ['closable', 'disableTransitions', 'title'],
    emits: ['close']
  },
  'el-dialog': {
    template: '<div v-if="modelValue" class="el-dialog"><slot /><slot name="footer" /></div>',
    props: ['modelValue', 'title', 'width', 'append'],
    emits: ['update:modelValue']
  },
  'el-tree': {
    template: '<div class="el-tree"><div v-for="n in data" :key="n.path" class="tree-node" @click="$emit(\'node-click\', n)">{{ n.name }}</div></div>',
    props: ['data', 'nodeKey', 'highlightCurrent', 'props', 'lazy', 'load'],
    emits: ['node-click']
  },
  'el-input-number': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', Number($event.target.value))" />',
    props: ['modelValue', 'min', 'step', 'placeholder'],
    emits: ['update:modelValue']
  },
  'el-date-picker': {
    template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'type', 'format', 'valueFormat', 'placeholder'],
    emits: ['update:modelValue']
  }
}

const baseFn = (over = {}) => ({
  name: '示例功能',
  description: '一个示例',
  command: 'echo hi',
  cwd: 'D:\\proj',
  timeoutMinutes: 5,
  completion: 'open_dir',
  icon: 'MagicStick',
  params: null,
  ...over
})

function createWrapper(fn = baseFn()) {
  return mount(AiFunctionRunner, {
    props: { fn },
    global: { stubs }
  })
}

describe('AiFunctionRunner.vue', () => {
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

  it('渲染功能名/描述/命令/工作目录/超时/完成动作 meta', () => {
    wrapper = createWrapper()
    expect(wrapper.find('.runner-name').text()).toBe('示例功能')
    expect(wrapper.text()).toContain('echo hi')
    expect(wrapper.text()).toContain('D:\\proj')
    expect(wrapper.text()).toContain('5 分钟')
    expect(wrapper.text()).toContain('打开产物目录')
  })

  it('超时未配置时展示默认 10 分钟', () => {
    wrapper = createWrapper(baseFn({ timeoutMinutes: 0 }))
    expect(wrapper.text()).toContain('默认 10 分钟')
  })

  it('completion 未知值时回退原值展示', () => {
    wrapper = createWrapper(baseFn({ completion: 'custom_x' }))
    expect(wrapper.text()).toContain('custom_x')
  })

  it('无参数（none）时展示无需参数说明，运行 emit 空对象', async () => {
    wrapper = createWrapper(baseFn({ params: { type: 'none' } }))
    expect(wrapper.find('.no-params').exists()).toBe(true)
    await wrapper.findAll('button').find(b => b.text() === '运行').trigger('click')
    expect(wrapper.emitted('run')).toBeTruthy()
    expect(wrapper.emitted('run')[0]).toEqual([{}])
  })

  it('text 参数：空值时弹警告不 emit', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper(baseFn({ params: { type: 'text', label: '指令', textFieldKey: 'cmd' } }))
    await wrapper.findAll('button').find(b => b.text() === '运行').trigger('click')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入指令')
    expect(wrapper.emitted('run')).toBeFalsy()
  })

  it('text 参数：有值时 emit {cmd: value}', async () => {
    wrapper = createWrapper(baseFn({ params: { type: 'text', label: '指令', textFieldKey: 'cmd' } }))
    const input = wrapper.find('input')
    await input.setValue('ls -la')
    await wrapper.findAll('button').find(b => b.text() === '运行').trigger('click')
    expect(wrapper.emitted('run')[0]).toEqual([{ cmd: 'ls -la' }])
  })

  it('text 参数：无 textFieldKey 时用默认 key text', async () => {
    wrapper = createWrapper(baseFn({ params: { type: 'text', label: '参数' } }))
    await wrapper.find('input').setValue('hello')
    await wrapper.findAll('button').find(b => b.text() === '运行').trigger('click')
    expect(wrapper.emitted('run')[0]).toEqual([{ text: 'hello' }])
  })

  it('file 参数：无已选文件时弹警告', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper(baseFn({ params: { type: 'file', label: '选择文件' } }))
    await wrapper.findAll('button').find(b => b.text() === '运行').trigger('click')
    expect(ElMessage.warning).toHaveBeenCalledWith('请至少选择一个文件')
  })

  it('file 参数：手输路径回车添加 + 运行 emit 换行拼接', async () => {
    wrapper = createWrapper(baseFn({ params: { type: 'file', textFieldKey: 'files' } }))
    const inputs = wrapper.findAll('input')
    // file 模式第一个 input 是手输路径框
    await inputs[0].setValue('D:\\a.txt')
    // 点"添加"按钮
    await wrapper.findAll('button').find(b => b.text() === '添加').trigger('click')
    expect(wrapper.findAll('.el-tag').length).toBe(1)
    // 再加一个
    await inputs[0].setValue('D:\\b.txt')
    await wrapper.findAll('button').find(b => b.text() === '添加').trigger('click')
    // 运行
    await wrapper.findAll('button').find(b => b.text() === '运行').trigger('click')
    expect(wrapper.emitted('run')[0]).toEqual([{ files: 'D:\\a.txt\nD:\\b.txt' }])
  })

  it('file 参数：重复路径不重复添加', async () => {
    wrapper = createWrapper(baseFn({ params: { type: 'file' } }))
    const input = wrapper.findAll('input')[0]
    await input.setValue('same.txt')
    await wrapper.findAll('button').find(b => b.text() === '添加').trigger('click')
    await input.setValue('same.txt')
    await wrapper.findAll('button').find(b => b.text() === '添加').trigger('click')
    expect(wrapper.findAll('.el-tag').length).toBe(1)
  })

  it('file 参数：点击 tag close 移除已选', async () => {
    wrapper = createWrapper(baseFn({ params: { type: 'file' } }))
    const input = wrapper.findAll('input')[0]
    await input.setValue('rm.txt')
    await wrapper.findAll('button').find(b => b.text() === '添加').trigger('click')
    expect(wrapper.findAll('.el-tag').length).toBe(1)
    await wrapper.find('.tag-close').trigger('click')
    expect(wrapper.findAll('.el-tag').length).toBe(0)
  })

  it('file 参数：浏览弹窗打开时加载目录节点', async () => {
    const { GetFileTree } = await import('../../../wailsjs/go/main/App')
    GetFileTree.mockResolvedValue([
      { name: 'a.go', path: 'D:\\a.go', type: 'file' },
      { name: 'sub', path: 'D:\\sub', type: 'dir', hasChildren: true }
    ])
    wrapper = createWrapper(baseFn({ params: { type: 'file', startDir: 'D:\\' } }))
    await wrapper.findAll('button').find(b => b.text() === '浏览').trigger('click')
    await flushPromises()
    expect(GetFileTree).toHaveBeenCalledWith('D:\\')
    expect(wrapper.findAll('.tree-node').length).toBe(2)
  })

  it('file 参数：浏览弹窗点文件加入已选，点目录切换路径', async () => {
    const { GetFileTree } = await import('../../../wailsjs/go/main/App')
    GetFileTree.mockResolvedValue([
      { name: 'a.go', path: 'D:\\a.go', type: 'file' },
      { name: 'sub', path: 'D:\\sub', type: 'dir', hasChildren: true }
    ])
    wrapper = createWrapper(baseFn({ params: { type: 'file', startDir: 'D:\\' } }))
    await wrapper.findAll('button').find(b => b.text() === '浏览').trigger('click')
    await flushPromises()
    // 点文件 → 加入已选
    const nodes = wrapper.findAll('.tree-node')
    await nodes[0].trigger('click')
    expect(wrapper.findAll('.el-tag').length).toBe(1)
    // 点目录 → 切换 browserPath（不加入已选）
    await nodes[1].trigger('click')
    expect(wrapper.findAll('.el-tag').length).toBe(1)
  })

  it('file 参数：浏览弹窗"添加为已选"按钮将当前路径加入', async () => {
    const { GetFileTree } = await import('../../../wailsjs/go/main/App')
    GetFileTree.mockResolvedValue([])
    wrapper = createWrapper(baseFn({ params: { type: 'file', startDir: 'D:\\' } }))
    await wrapper.findAll('button').find(b => b.text() === '浏览').trigger('click')
    await flushPromises()
    // 浏览弹窗内路径输入框 + "添加为已选"按钮
    const dialogInputs = wrapper.find('.el-dialog').findAll('input')
    await dialogInputs[0].setValue('D:\\manual.txt')
    await wrapper.findAll('button').find(b => b.text() === '添加为已选').trigger('click')
    expect(wrapper.findAll('.el-tag').length).toBe(1)
  })

  it('file 参数：浏览加载目录失败时弹错误', async () => {
    const { ElMessage } = await import('element-plus')
    const { GetFileTree } = await import('../../../wailsjs/go/main/App')
    GetFileTree.mockRejectedValue(new Error('read fail'))
    wrapper = createWrapper(baseFn({ params: { type: 'file', startDir: 'D:\\' } }))
    await wrapper.findAll('button').find(b => b.text() === '浏览').trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith(expect.stringContaining('read fail'))
  })

  it('form 参数：number 字段默认 1，datetime 字段默认下一整点', () => {
    wrapper = createWrapper(baseFn({
      params: {
        type: 'form',
        fields: [
          { key: 'dur', label: '时长', type: 'number' },
          { key: 'when', label: '时间', type: 'datetime' },
          { key: 'note', label: '备注', type: 'text' }
        ]
      }
    }))
    const inputs = wrapper.findAll('input')
    // number 默认 1
    expect(inputs[0].element.value).toBe('1')
    // datetime 默认下一整点（格式 YYYY-MM-DD HH:mm）
    expect(inputs[1].element.value).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:00$/)
    // text 默认空
    expect(inputs[2].element.value).toBe('')
  })

  it('form 参数：必填字段为空时弹警告', async () => {
    const { ElMessage } = await import('element-plus')
    wrapper = createWrapper(baseFn({
      params: {
        type: 'form',
        fields: [{ key: 'name', label: '名称', type: 'text', required: true }]
      }
    }))
    await wrapper.findAll('button').find(b => b.text() === '运行').trigger('click')
    expect(ElMessage.warning).toHaveBeenCalledWith('请填写「名称」')
  })

  it('form 参数：运行 emit 全字段字符串化', async () => {
    wrapper = createWrapper(baseFn({
      params: {
        type: 'form',
        fields: [
          { key: 'dur', label: '时长', type: 'number' },
          { key: 'name', label: '名称', type: 'text' }
        ]
      }
    }))
    // 修改 name 字段
    const inputs = wrapper.findAll('input')
    await inputs[1].setValue('test')
    await wrapper.findAll('button').find(b => b.text() === '运行').trigger('click')
    expect(wrapper.emitted('run')[0]).toEqual([{ dur: '1', name: 'test' }])
  })

  it('icon 未配置时回退 MagicStick', () => {
    wrapper = createWrapper(baseFn({ icon: '' }))
    expect(wrapper.find('.runner-icon').exists()).toBe(true)
  })
})
