import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import TerminalPanel from '../TerminalPanel.vue'
import { useUiStore } from '../../store'

// Mock useTerminal（pty 终端本质依赖后端运行时，mock 为可控函数）。
// 每次 useTerminal() 返回全新实例对象（对齐多 tab 多实例），共享同一组 mock 函数；
// capturedInstances 捕获实例引用，供用例驱动 isExited 回流与实例数断言。
const terminalMock = {
  initTerminal: vi.fn(() => Promise.resolve()),
  changeDir: vi.fn(),
  resize: vi.fn(),
  focus: vi.fn(),
  destroyTerminal: vi.fn(() => Promise.resolve()),
  restartTerminal: vi.fn(() => Promise.resolve())
}
const capturedInstances = []
vi.mock('../../composables/useTerminal', async () => {
  const { ref } = await import('vue')
  return {
    useTerminal: () => {
      const inst = {
        isActive: ref(false),
        isExited: ref(false),
        currentDir: ref(''),
        ...terminalMock
      }
      capturedInstances.push(inst)
      return inst
    }
  }
})

vi.mock('../../../wailsjs/go/main/App', () => ({
  GetShellConfigs: vi.fn(),
  GetSettings: vi.fn()
}))

vi.mock('@element-plus/icons-vue', () => ({
  Folder: { template: '<i class="i-folder" />' },
  RefreshRight: { template: '<i class="i-refresh" />' }
}))

const stubs = {
  'el-icon': { template: '<i><slot /></i>', props: ['size'] },
  'el-button': {
    template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot /><slot name="icon" /></button>',
    props: ['type', 'size', 'text', 'loading'],
    emits: ['click']
  }
}

async function createWrapper(shellConfigsOver = null, settingsOver = null) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const { GetShellConfigs, GetSettings } = await import('../../../wailsjs/go/main/App')
  if (shellConfigsOver === null) {
    GetShellConfigs.mockResolvedValue([{ type: 'powershell', displayName: 'PowerShell' }, { type: 'cmd', displayName: 'CMD' }])
  } else if (shellConfigsOver === 'fail') {
    GetShellConfigs.mockRejectedValue(new Error('fail'))
  } else {
    GetShellConfigs.mockResolvedValue(shellConfigsOver)
  }
  if (settingsOver === null) {
    GetSettings.mockResolvedValue({ defaultShell: 'powershell' })
  } else if (settingsOver === 'fail') {
    GetSettings.mockRejectedValue(new Error('settings fail'))
  } else {
    GetSettings.mockResolvedValue(settingsOver)
  }
  const uiStore = useUiStore()
  uiStore.terminalVisible = false
  const wrapper = mount(TerminalPanel, { global: { stubs, plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

/** 打开面板并等第一个 tab 建好（首次可见自动建 tab + init） */
async function openPanel(wrapper, dir = 'D:\\proj') {
  const uiStore = useUiStore()
  uiStore.terminalDir = dir
  uiStore.terminalVisible = true
  await nextTick()
  await flushPromises()
  return uiStore
}

/** 在已开面板基础上点 + 新建一个 tab */
async function addTab(wrapper) {
  await wrapper.find('.add-tab-btn').trigger('click')
  await flushPromises()
}

describe('TerminalPanel.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    capturedInstances.length = 0
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
  })

  it('挂载时加载 Shell 配置（displayName 映射数据源）', async () => {
    wrapper = await createWrapper()
    expect(wrapper.vm.$.setupState.shellConfigs).toHaveLength(2)
  })

  it('GetShellConfigs 失败时回退默认 4 种 Shell', async () => {
    wrapper = await createWrapper('fail')
    expect(wrapper.vm.$.setupState.shellConfigs).toHaveLength(4)
    expect(wrapper.vm.$.setupState.shellConfigs.map(c => c.type)).toEqual([
      'powershell', 'cmd', 'gitbash', 'wsl'
    ])
  })

  it('GetSettings 返回 defaultShell 时作为新建 tab 的 Shell 类型', async () => {
    wrapper = await createWrapper(null, { defaultShell: 'gitbash' })
    expect(wrapper.vm.$.setupState.defaultShell).toBe('gitbash')
  })

  it('GetSettings 失败时保持默认 powershell', async () => {
    wrapper = await createWrapper(null, 'fail')
    expect(wrapper.vm.$.setupState.defaultShell).toBe('powershell')
  })

  it('terminalVisible 首次为 true 时自动建第一个 tab 并初始化（单 tab 隐藏 tab 栏）', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\proj')
    expect(terminalMock.initTerminal).toHaveBeenCalledTimes(1)
    expect(terminalMock.initTerminal).toHaveBeenCalledWith(
      expect.any(Object),
      'D:\\proj',
      'powershell'
    )
    expect(terminalMock.focus).toHaveBeenCalled()
    // D1：单 tab 隐藏 tab 栏，+ 按钮可用
    expect(wrapper.find('.tab-strip').exists()).toBe(false)
    expect(wrapper.find('.add-tab-btn').exists()).toBe(true)
    expect(wrapper.find('.add-tab-btn.is-disabled').exists()).toBe(false)
  })

  it('文件树目录变化不再触发已建终端 changeDir（D2 行为变更）', async () => {
    wrapper = await createWrapper()
    const uiStore = await openPanel(wrapper)
    expect(terminalMock.initTerminal).toHaveBeenCalledTimes(1)
    uiStore.terminalDir = 'D:\\new-dir'
    await nextTick()
    await flushPromises()
    expect(terminalMock.changeDir).not.toHaveBeenCalled()
  })

  it('点击 + 新建 tab：tab 栏出现、新 tab 激活并初始化、目录继承文件树当前目录', async () => {
    wrapper = await createWrapper()
    const uiStore = await openPanel(wrapper, 'D:\\work\\first')
    uiStore.terminalDir = 'D:\\work\\second'
    await addTab(wrapper)

    const tabEls = wrapper.findAll('.terminal-tab')
    expect(tabEls).toHaveLength(2)
    expect(wrapper.find('.tab-strip').exists()).toBe(true)
    // 新 tab 激活并显示目录尾段
    expect(tabEls[1].classes()).toContain('is-active')
    expect(tabEls[1].text()).toContain('second')
    // 新 tab 初始化：目录继承点击 + 时的文件树当前目录（D2 继承语义）
    expect(terminalMock.initTerminal).toHaveBeenCalledTimes(2)
    expect(terminalMock.initTerminal).toHaveBeenLastCalledWith(
      expect.any(Object),
      'D:\\work\\second',
      'powershell'
    )
  })

  it('新建 tab 的 Shell 类型取设置页 defaultShell（title 用 displayName 映射）', async () => {
    wrapper = await createWrapper(null, { defaultShell: 'cmd' })
    await openPanel(wrapper, 'D:\\proj')
    await addTab(wrapper)

    const titles = wrapper.findAll('.terminal-tab .tab-title').map(t => t.text())
    expect(titles).toEqual(['CMD', 'CMD'])
    expect(terminalMock.initTerminal).toHaveBeenLastCalledWith(
      expect.any(Object),
      expect.any(String),
      'cmd'
    )
  })

  it('切换回已初始化的 tab：refit + focus，不重复 init', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    await addTab(wrapper)
    expect(wrapper.vm.$.setupState.activeId).toBe('tab-2')
    terminalMock.initTerminal.mockClear()
    terminalMock.resize.mockClear()
    terminalMock.focus.mockClear()

    await wrapper.findAll('.terminal-tab')[0].trigger('click')
    await flushPromises()

    expect(wrapper.vm.$.setupState.activeId).toBe('tab-1')
    // 已 init 的 tab 切换仅 refit + focus，不重新初始化
    expect(terminalMock.initTerminal).not.toHaveBeenCalled()
    expect(terminalMock.resize).toHaveBeenCalled()
    expect(terminalMock.focus).toHaveBeenCalled()
  })

  it('关闭非活动 tab：列表减少且活动不变，回到单 tab 模式', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    await addTab(wrapper)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(2)

    // 活动为 tab-2，点第一个 tab 的 × 关闭 tab-1
    await wrapper.findAll('.terminal-tab')[0].find('.tab-close').trigger('click')
    await flushPromises()

    // 剩单 tab：tab 栏隐藏（D1），列表仅 tab-2 且仍活动
    expect(wrapper.vm.$.setupState.tabs).toHaveLength(1)
    expect(wrapper.find('.tab-strip').exists()).toBe(false)
    expect(wrapper.vm.$.setupState.activeId).toBe('tab-2')
    // 单 tab 徽章模式显示其 shell 名
    expect(wrapper.find('.shell-name').text()).toBe('PowerShell')
  })

  it('关闭活动 tab 后自动激活邻位 tab', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    await addTab(wrapper)
    // 活动为 tab-2，关闭之
    await wrapper.findAll('.terminal-tab')[1].find('.tab-close').trigger('click')
    await flushPromises()

    expect(wrapper.vm.$.setupState.tabs).toHaveLength(1)
    expect(wrapper.vm.$.setupState.activeId).toBe('tab-1')
    // 2 次 = tab-1 / tab-2 各自首次 init；关闭活动 tab 后邻位 tab-1 复用已有会话，无新增 init
    expect(terminalMock.initTerminal).toHaveBeenCalledTimes(2)
  })

  it('鼠标中键关闭 tab', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    await addTab(wrapper)
    await wrapper.findAll('.terminal-tab')[0].trigger('auxclick', { button: 1 })
    await flushPromises()

    expect(wrapper.vm.$.setupState.tabs).toHaveLength(1)
    expect(wrapper.vm.$.setupState.activeId).toBe('tab-2')
  })

  it('达 8 个上限后 + 按钮禁用并提示，点击不再新建', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    for (let i = 0; i < 7; i++) {
      await addTab(wrapper)
    }
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(8)
    expect(wrapper.find('.add-tab-btn.is-disabled').exists()).toBe(true)
    expect(wrapper.find('.add-tab-btn').attributes('title')).toContain('最多支持')

    await addTab(wrapper)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(8)
    expect(capturedInstances).toHaveLength(8)
  })

  it('tab 关闭时销毁对应 xterm 实例', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    await addTab(wrapper)
    expect(capturedInstances).toHaveLength(2)
    expect(terminalMock.destroyTerminal).not.toHaveBeenCalled()

    await wrapper.findAll('.terminal-tab')[0].find('.tab-close').trigger('click')
    await flushPromises()

    expect(terminalMock.destroyTerminal).toHaveBeenCalledTimes(1)
    expect(capturedInstances).toHaveLength(2) // 捕获记录不减（历史实例），实例表内已移除
  })

  it('会话退出回流 tab 状态并展示重新启动按钮，点击针对活动 tab 重启', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\proj')
    expect(capturedInstances).toHaveLength(1)

    // terminal-exit 事件在 useTerminal 实例内置 isExited，经同步 watch 回流 tab 状态
    capturedInstances[0].isExited.value = true
    await nextTick()
    const restartBtn = wrapper.findAll('button').find(b => b.text().includes('重新启动'))
    expect(restartBtn).toBeTruthy()

    await restartBtn.trigger('click')
    await flushPromises()
    expect(terminalMock.restartTerminal).toHaveBeenCalledWith(
      expect.any(Object),
      'D:\\proj',
      'powershell'
    )
  })

  it('重启成功后退出态复位：状态点恢复、重启按钮消失', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\proj')
    capturedInstances[0].isExited.value = true
    await nextTick()
    // 单 tab 徽章模式：状态点转警示色
    expect(wrapper.find('.shell-dot.is-exited').exists()).toBe(true)
    expect(wrapper.findAll('button').some(b => b.text().includes('重新启动'))).toBe(true)

    // restartTerminal 内部 destroy/init 将实例 isExited 置回 false，须回流复位 tab 状态
    capturedInstances[0].isExited.value = false
    await nextTick()
    expect(wrapper.find('.shell-dot.is-exited').exists()).toBe(false)
    expect(wrapper.findAll('button').some(b => b.text().includes('重新启动'))).toBe(false)
  })

  it('dirBasename 目录尾段提取兼容根目录与正反斜杠', async () => {
    wrapper = await createWrapper()
    const dirBasename = wrapper.vm.$.setupState.dirBasename
    expect(dirBasename('D:\\work\\demo')).toBe('demo')
    expect(dirBasename('D:/work/demo')).toBe('demo')
    expect(dirBasename('C:\\')).toBe('C:')
    expect(dirBasename('/')).toBe('/')
    expect(dirBasename('')).toBe('')
  })

  it('tab 溢出时滚轮驱动横向滚动（滚动条隐藏后的鼠标滚动方式）', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    await addTab(wrapper)
    await addTab(wrapper)
    const strip = wrapper.find('.tab-strip')
    expect(strip.exists()).toBe(true)

    await strip.trigger('wheel', { deltaY: 120 })
    expect(strip.element.scrollLeft).toBe(120)

    await strip.trigger('wheel', { deltaX: -50, deltaY: 5 })
    expect(strip.element.scrollLeft).toBe(70)
  })

  it('单 tab 退出态在 tab 栏显示黄色退出标记', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    await addTab(wrapper)
    await addTab(wrapper)
    capturedInstances[2].isExited.value = true
    await nextTick()

    const exitedTab = wrapper.findAll('.terminal-tab')[2]
    expect(exitedTab.classes()).toContain('is-exited')
    expect(exitedTab.find('.tab-exit-dot').exists()).toBe(true)
    // 非退出 tab 无标记
    expect(wrapper.findAll('.terminal-tab')[0].find('.tab-exit-dot').exists()).toBe(false)
  })

  it('点击收起按钮 emit toggle', async () => {
    wrapper = await createWrapper()
    await wrapper.find('.minimize-btn').trigger('click')
    expect(wrapper.emitted('toggle')).toBeTruthy()
  })

  it('卸载时销毁全部 tab 实例', async () => {
    wrapper = await createWrapper()
    await openPanel(wrapper, 'D:\\a')
    await addTab(wrapper)
    expect(capturedInstances).toHaveLength(2)

    wrapper.unmount()
    wrapper = null
    await flushPromises()
    expect(terminalMock.destroyTerminal).toHaveBeenCalledTimes(2)
  })

  describe('崩溃恢复快照钩子（session.json 多 tab 升级）', () => {
    /** 打开面板前注入恢复镜像（模拟 useSessionState 恢复流程写入） */
    async function openPanelWithSnapshot(wrapper, tabs, activeIndex = 0) {
      const uiStore = useUiStore()
      uiStore.terminalTabsSnapshot = tabs
      uiStore.terminalActiveIndex = activeIndex
      uiStore.terminalVisible = true
      await nextTick()
      await flushPromises()
      return uiStore
    }

    it('按快照循环新建全部 tab，仅活动 tab 初始化（惰性 init）', async () => {
      wrapper = await createWrapper()
      const uiStore = await openPanelWithSnapshot(wrapper, [
        { workDir: 'D:\\a', shellType: 'powershell' },
        { workDir: 'D:\\b', shellType: 'cmd' }
      ])

      expect(wrapper.findAll('.terminal-tab')).toHaveLength(2)
      expect(capturedInstances).toHaveLength(2)
      // 活动下标 0：仅第 1 个 tab 建 xterm 会话，第 2 个留待首次激活
      expect(terminalMock.initTerminal).toHaveBeenCalledTimes(1)
      expect(terminalMock.initTerminal).toHaveBeenCalledWith(
        expect.any(Object),
        'D:\\a',
        'powershell'
      )
      // 活动下标还原 + 镜像消费后被保存钩子回写为当前 tab 投影（原快照不再残留）
      expect(uiStore.terminalActiveIndex).toBe(0)
      expect(uiStore.terminalTabsSnapshot).toEqual([
        { workDir: 'D:\\a', shellType: 'powershell' },
        { workDir: 'D:\\b', shellType: 'cmd' }
      ])
    })

    it('活动下标指向第 2 个 tab 时还原激活并初始化该 tab', async () => {
      wrapper = await createWrapper()
      await openPanelWithSnapshot(
        wrapper,
        [
          { workDir: 'D:\\a', shellType: 'powershell' },
          { workDir: 'D:\\b', shellType: 'cmd' }
        ],
        1
      )

      expect(wrapper.findAll('.terminal-tab')[1].classes()).toContain('is-active')
      expect(terminalMock.initTerminal).toHaveBeenCalledTimes(1)
      expect(terminalMock.initTerminal).toHaveBeenCalledWith(
        expect.any(Object),
        'D:\\b',
        'cmd'
      )
    })

    it('快照超上限 8 个时截断（createTab 达上限返回 null 后停止）', async () => {
      wrapper = await createWrapper()
      const uiStore = await openPanelWithSnapshot(
        wrapper,
        Array.from({ length: 9 }, (_, i) => ({
          workDir: `D:\\dir-${i}`,
          shellType: 'powershell'
        })),
        3
      )

      const tabEls = wrapper.findAll('.terminal-tab')
      expect(tabEls).toHaveLength(8)
      expect(capturedInstances).toHaveLength(8)
      // 截断后原活动下标 3 仍在界内，clamp 不偏移：第 4 个 tab 激活
      expect(tabEls[3].classes()).toContain('is-active')
      expect(uiStore.terminalActiveIndex).toBe(3)
    })

    it('旧版快照降级单 tab：shellType 为空时回退设置页默认 shell', async () => {
      wrapper = await createWrapper()
      await openPanelWithSnapshot(wrapper, [{ workDir: 'D:\\legacy', shellType: '' }])

      // 单 tab 走徽章模式（D1 隐藏 tab 栏），tab 状态断言组件内 tabs
      expect(wrapper.vm.$.setupState.tabs).toHaveLength(1)
      expect(wrapper.find('.tab-strip').exists()).toBe(false)
      expect(wrapper.find('.shell-name').text()).toBe('PowerShell')
      expect(terminalMock.initTerminal).toHaveBeenCalledWith(
        expect.any(Object),
        'D:\\legacy',
        'powershell' // GetSettings mock 默认 defaultShell
      )
    })

    it('镜像为空时走默认单 tab（冷启动 / 无可恢复快照）', async () => {
      wrapper = await createWrapper()
      await openPanel(wrapper, 'D:\\fresh')

      expect(wrapper.vm.$.setupState.tabs).toHaveLength(1)
      expect(terminalMock.initTerminal).toHaveBeenCalledWith(
        expect.any(Object),
        'D:\\fresh',
        'powershell'
      )
    })

    it('保存钩子：tab 新建/切换同步镜像（workDir/shellType/活动下标）', async () => {
      wrapper = await createWrapper()
      const uiStore = await openPanel(wrapper, 'D:\\first')
      expect(uiStore.terminalTabsSnapshot).toEqual([
        { workDir: 'D:\\first', shellType: 'powershell' }
      ])
      expect(uiStore.terminalActiveIndex).toBe(0)

      uiStore.terminalDir = 'D:\\second'
      await addTab(wrapper)
      expect(uiStore.terminalTabsSnapshot).toEqual([
        { workDir: 'D:\\first', shellType: 'powershell' },
        { workDir: 'D:\\second', shellType: 'powershell' }
      ])
      expect(uiStore.terminalActiveIndex).toBe(1)

      // 切回第 1 个 tab：活动下标回流镜像
      await wrapper.findAll('.terminal-tab')[0].trigger('click')
      await flushPromises()
      expect(uiStore.terminalActiveIndex).toBe(0)
    })

    it('保存钩子：关闭 tab 后镜像同步收缩', async () => {
      wrapper = await createWrapper()
      const uiStore = await openPanel(wrapper, 'D:\\a')
      await addTab(wrapper)
      expect(uiStore.terminalTabsSnapshot).toHaveLength(2)

      await wrapper.findAll('.terminal-tab')[0].find('.tab-close').trigger('click')
      await flushPromises()

      expect(uiStore.terminalTabsSnapshot).toEqual([
        { workDir: 'D:\\a', shellType: 'powershell' }
      ])
      expect(uiStore.terminalActiveIndex).toBe(0)
    })
  })
})
