import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ToolboxPanel from '../ToolboxPanel.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

// 七个内容面板均被 stub（各自交互面由自身 spec 覆盖），仅需补齐 import 链所需导出
vi.mock('../../../wailsjs/go/main/App', () => ({
  CopyTo: vi.fn(),
  ListProcesses: vi.fn(() => Promise.resolve([])),
  StopProcess: vi.fn(() => Promise.resolve({ stopped: true, method: 'forced' })),
  KillProcess: vi.fn(() => Promise.resolve()),
  ListEnvVars: vi.fn(() => Promise.resolve([])),
  GetHosts: vi.fn(() => Promise.resolve({ path: '', entries: [], error: '' })),
  SplitPptxPerPage: vi.fn(() => Promise.resolve({ outputDir: '', files: [], pageCount: 0 })),
  SplitPptxFolder: vi.fn(() => Promise.resolve({ dirPath: '', total: 0, items: [] })),
  CancelPptxSplit: vi.fn(() => Promise.resolve(false)),
  OpenFileDialog: vi.fn(() => Promise.resolve('')),
  OpenDirectoryDialog: vi.fn(() => Promise.resolve(''))
}))

// PptSplitPanel 随父组件 import EventsOn（事件监听随其挂载注册）
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn()
}))

// 图标按需 mock：ToolboxPanel 用 CopyDocument / Cpu / Connection / Document / Key / Guide / Files
// （标题栏图标为本地 ToolboxIcon 组件，不走 element-plus）
vi.mock('@element-plus/icons-vue', () => ({
  CopyDocument: { template: '<i class="i-copy" />' },
  Cpu: { template: '<i class="i-cpu" />' },
  Connection: { template: '<i class="i-connection" />' },
  Document: { template: '<i class="i-document" />' },
  Key: { template: '<i class="i-key" />' },
  Guide: { template: '<i class="i-guide" />' },
  Files: { template: '<i class="i-files" />' }
}))

const stubs = {
  'el-icon': { template: '<i><slot /></i>', props: ['size'] },
  // 七个内容面板：桩仅以特征 class 反映挂载与否（v-if 切换 = 卸载/挂载）
  CopyToPanel: { template: '<div class="copy-to-panel-stub" />' },
  ProcessManagerPanel: { template: '<div class="process-manager-panel-stub" />' },
  PortListPanel: { template: '<div class="port-list-panel-stub" />' },
  JsonYamlToolPanel: { template: '<div class="json-yaml-tool-panel-stub" />' },
  EnvVarPanel: { template: '<div class="env-var-panel-stub" />' },
  HostsPanel: { template: '<div class="hosts-panel-stub" />' },
  PptSplitPanel: { template: '<div class="ppt-split-panel-stub" />' }
}

const createWrapper = () => mount(ToolboxPanel, { global: { stubs } })

const panelStubClass = {
  copyTo: '.copy-to-panel-stub',
  processManager: '.process-manager-panel-stub',
  portList: '.port-list-panel-stub',
  jsonYamlTool: '.json-yaml-tool-panel-stub',
  envVars: '.env-var-panel-stub',
  hosts: '.hosts-panel-stub',
  pptSplit: '.ppt-split-panel-stub'
}

describe('ToolboxPanel', () => {
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

  it('渲染标题栏与 7 个功能清单项', () => {
    wrapper = createWrapper()
    expect(wrapper.find('.panel-header').text()).toContain('工具箱')
    expect(wrapper.findAll('.toolbox-nav-item').length).toBe(7)
    const names = wrapper.findAll('.toolbox-nav-name').map((n) => n.text())
    expect(names).toEqual([
      '拷贝到',
      '进程管理',
      '端口占用',
      'JSON/YAML 工具',
      '环境变量',
      'hosts 文件',
      'PPT 拆分'
    ])
  })

  it('默认选中首个工具（拷贝到）并渲染其内容面板', () => {
    wrapper = createWrapper()
    expect(wrapper.find(panelStubClass.copyTo).exists()).toBe(true)
    // 其余面板不渲染（v-if 卸载切换）
    expect(wrapper.find(panelStubClass.pptSplit).exists()).toBe(false)
    // 默认项带 active 态（左侧指示条依赖的 class）
    expect(wrapper.findAll('.toolbox-nav-item')[0].classes()).toContain('is-active')
  })

  it('点击清单项切换右侧内容面板（弹窗化改为面板内嵌切换）', async () => {
    wrapper = createWrapper()
    // 点「PPT 拆分」→ 渲染 PptSplitPanel、拷贝到面板卸载
    const pptItem = wrapper.findAll('.toolbox-nav-item').find((el) => el.text().includes('PPT 拆分'))
    await pptItem.trigger('click')
    expect(wrapper.find(panelStubClass.pptSplit).exists()).toBe(true)
    expect(wrapper.find(panelStubClass.copyTo).exists()).toBe(false)
    expect(wrapper.findAll('.toolbox-nav-item')[6].classes()).toContain('is-active')

    // 点「进程管理」→ 再次切换
    const processItem = wrapper.findAll('.toolbox-nav-item').find((el) => el.text().includes('进程管理'))
    await processItem.trigger('click')
    expect(wrapper.find(panelStubClass.processManager).exists()).toBe(true)
    expect(wrapper.find(panelStubClass.pptSplit).exists()).toBe(false)
  })

  it('逐一切换 7 个工具均渲染对应内容面板（全程无弹窗）', async () => {
    wrapper = createWrapper()
    for (const [id, stubClass] of Object.entries(panelStubClass)) {
      const item = wrapper.findAll('.toolbox-nav-item').find((el) => {
        const key = Object.keys(panelStubClass).find((k) => panelStubClass[k] === stubClass)
        const toolNames = {
          copyTo: '拷贝到',
          processManager: '进程管理',
          portList: '端口占用',
          jsonYamlTool: 'JSON/YAML',
          envVars: '环境变量',
          hosts: 'hosts',
          pptSplit: 'PPT 拆分'
        }
        return el.text().includes(toolNames[id])
      })
      await item.trigger('click')
      expect(wrapper.find(stubClass).exists()).toBe(true)
    }
    // 全程无 el-dialog（去弹窗化）
    expect(wrapper.find('.el-dialog').exists()).toBe(false)
  })

  it('点击关闭按钮 emit close（Home 切回 directory）', async () => {
    wrapper = createWrapper()
    await wrapper.find('.toolbox-close').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })
})
