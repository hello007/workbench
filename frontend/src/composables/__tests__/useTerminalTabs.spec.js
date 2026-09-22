import { describe, it, expect } from 'vitest'
import { useTerminalTabs, MAX_TERMINAL_TABS } from '../useTerminalTabs'

/**
 * 建满 n 个 tab 的辅助：返回 composable 实例
 */
function createTabsOf(n) {
  const store = useTerminalTabs()
  for (let i = 0; i < n; i++) {
    store.createTab({ dir: `D:\\work\\proj-${i + 1}`, shellType: 'powershell', title: 'PowerShell' })
  }
  return store
}

describe('useTerminalTabs', () => {
  it('createTab 新建 tab 并激活，字段完整', () => {
    const store = useTerminalTabs()
    const tab = store.createTab({ dir: 'D:\\work\\demo', shellType: 'powershell', title: 'PowerShell' })

    expect(tab).not.toBeNull()
    expect(tab.id).toBe('tab-1')
    expect(tab.title).toBe('PowerShell')
    expect(tab.dir).toBe('D:\\work\\demo')
    expect(tab.shellType).toBe('powershell')
    expect(tab.isActive).toBe(true)
    expect(tab.isExited).toBe(false)
    expect(store.tabs.value).toHaveLength(1)
    expect(store.activeId.value).toBe('tab-1')
  })

  it('连续新建 id 自增，且仅最新 tab 活动', () => {
    const store = createTabsOf(3)
    expect(store.tabs.value.map(t => t.id)).toEqual(['tab-1', 'tab-2', 'tab-3'])
    expect(store.activeId.value).toBe('tab-3')
    expect(store.tabs.value.filter(t => t.isActive)).toHaveLength(1)
    expect(store.tabs.value[2].isActive).toBe(true)
  })

  it('达上限 8 后 createTab 返回 null，atLimit 为 true', () => {
    const store = createTabsOf(MAX_TERMINAL_TABS)
    expect(store.tabs.value).toHaveLength(8)
    expect(store.atLimit.value).toBe(true)

    const overflow = store.createTab({ dir: 'D:\\x', shellType: 'cmd', title: 'CMD' })
    expect(overflow).toBeNull()
    expect(store.tabs.value).toHaveLength(8)
  })

  it('activateTab 切换活动态流转（单活动）', () => {
    const store = createTabsOf(3)
    store.activateTab('tab-1')
    expect(store.activeId.value).toBe('tab-1')
    expect(store.tabs.value[0].isActive).toBe(true)
    expect(store.tabs.value[2].isActive).toBe(false)
  })

  it('activateTab 传入不存在的 id 时保持现状（安全 no-op）', () => {
    const store = createTabsOf(2)
    store.activateTab('tab-not-exist')
    expect(store.activeId.value).toBe('tab-2')
  })

  it('关闭活动 tab 后自动激活右邻位', () => {
    const store = createTabsOf(3)
    store.activateTab('tab-2')
    const closed = store.closeTab('tab-2')

    expect(closed).toBe(true)
    expect(store.tabs.value.map(t => t.id)).toEqual(['tab-1', 'tab-3'])
    expect(store.activeId.value).toBe('tab-3')
    expect(store.tabs.value[1].isActive).toBe(true)
  })

  it('关闭末尾活动 tab 后自动激活左邻位', () => {
    const store = createTabsOf(3)
    // 当前活动为 tab-3（末尾）
    store.closeTab('tab-3')

    expect(store.tabs.value.map(t => t.id)).toEqual(['tab-1', 'tab-2'])
    expect(store.activeId.value).toBe('tab-2')
  })

  it('关闭非活动 tab 不改变活动 tab', () => {
    const store = createTabsOf(3)
    // 活动为 tab-3，关闭 tab-1
    store.closeTab('tab-1')

    expect(store.tabs.value.map(t => t.id)).toEqual(['tab-2', 'tab-3'])
    expect(store.activeId.value).toBe('tab-3')
    expect(store.tabs.value[1].isActive).toBe(true)
  })

  it('关闭全部 tab 后 activeId 置空', () => {
    const store = createTabsOf(2)
    store.closeTab('tab-2')
    store.closeTab('tab-1')

    expect(store.tabs.value).toHaveLength(0)
    expect(store.activeId.value).toBe('')
    expect(store.atLimit.value).toBe(false)
  })

  it('closeTab 传入不存在的 id 返回 false 且列表不变', () => {
    const store = createTabsOf(2)
    const closed = store.closeTab('tab-not-exist')

    expect(closed).toBe(false)
    expect(store.tabs.value).toHaveLength(2)
  })

  it('markExited 标记会话退出态', () => {
    const store = createTabsOf(2)
    store.markExited('tab-1')

    expect(store.tabs.value[0].isExited).toBe(true)
    expect(store.tabs.value[1].isExited).toBe(false)
  })

  it('markExited 传入不存在的 id 时安全 no-op', () => {
    const store = createTabsOf(1)
    store.markExited('tab-not-exist')
    expect(store.tabs.value[0].isExited).toBe(false)
  })

  it('markRunning 撤销退出标记（重启成功回流，复位 isExited）', () => {
    const store = createTabsOf(2)
    store.markExited('tab-1')
    expect(store.tabs.value[0].isExited).toBe(true)

    store.markRunning('tab-1')
    expect(store.tabs.value[0].isExited).toBe(false)
    // 其余 tab 不受影响
    expect(store.tabs.value[1].isExited).toBe(false)
  })

  it('markRunning 传入不存在的 id 时安全 no-op', () => {
    const store = createTabsOf(1)
    store.markExited('tab-1')
    store.markRunning('tab-not-exist')
    expect(store.tabs.value[0].isExited).toBe(true)
  })

  it('关闭 tab 后上限解除，可再次新建', () => {
    const store = createTabsOf(MAX_TERMINAL_TABS)
    expect(store.atLimit.value).toBe(true)

    store.closeTab('tab-1')
    expect(store.atLimit.value).toBe(false)

    const tab = store.createTab({ dir: 'D:\\x', shellType: 'cmd', title: 'CMD' })
    expect(tab).not.toBeNull()
    expect(tab.id).toBe('tab-9')
    expect(store.tabs.value).toHaveLength(8)
  })
})
