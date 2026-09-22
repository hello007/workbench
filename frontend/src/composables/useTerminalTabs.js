import { ref, computed } from 'vue'

/**
 * 终端 tab 数量上限：每 tab 一个独立 shell 进程（PTY 资源），
 * scrollback 内存按实例线性增长，软上限保护进程与内存资源
 */
export const MAX_TERMINAL_TABS = 8

/**
 * 终端 tab 列表状态管理（纯状态，不含 xterm 实例）。
 *
 * 归属说明：tab 列表是终端面板私有状态，唯一消费方为 TerminalPanel
 * （渲染 tab 栏与 v-for 容器），与 xterm 实例 Map（tabId → useTerminal()）
 * 同生命周期同归属，故以 composable 而非 pinia store 承载——
 * store 适合跨组件共享态（如 terminalVisible 被 ActivityBar/Home/TerminalPanel
 * 三方消费），tab 列表无此需求。
 *
 * xterm 实例本身仍由 TerminalPanel 内每 tab 一个 useTerminal() 持有，
 * 本 composable 仅管理 tab 状态对象：
 * { id, title(shell 显示名), dir, shellType, isActive, isExited }
 */
export function useTerminalTabs() {
  /** tab 列表 */
  const tabs = ref([])
  /** 活动 tab id */
  const activeId = ref('')
  /** 是否已达数量上限 */
  const atLimit = computed(() => tabs.value.length >= MAX_TERMINAL_TABS)

  let seq = 0

  /**
   * 新建 tab 并激活；达上限返回 null（调用方据 returns 判断是否创建成功）
   * @param {object} payload - { dir, shellType, title }
   */
  function createTab({ dir, shellType, title }) {
    if (atLimit.value) return null
    seq += 1
    const tab = {
      id: `tab-${seq}`,
      title: title || shellType || 'shell',
      dir,
      shellType,
      isActive: false,
      isExited: false
    }
    tabs.value.push(tab)
    activateTab(tab.id)
    return tab
  }

  /**
   * 激活指定 tab（isActive 单活动流转）；id 不存在时安全 no-op
   */
  function activateTab(id) {
    if (!tabs.value.some(t => t.id === id)) return
    activeId.value = id
    tabs.value.forEach(t => {
      t.isActive = t.id === id
    })
  }

  /**
   * 关闭 tab；关闭活动 tab 后自动激活邻位（优先右侧，关闭末尾 tab 则左侧）。
   * 关闭全部后 activeId 置空。id 不存在时返回 false。
   */
  function closeTab(id) {
    const idx = tabs.value.findIndex(t => t.id === id)
    if (idx === -1) return false
    const wasActive = tabs.value[idx].id === activeId.value
    tabs.value.splice(idx, 1)
    if (tabs.value.length === 0) {
      activeId.value = ''
    } else if (wasActive) {
      const next = tabs.value[Math.min(idx, tabs.value.length - 1)]
      activateTab(next.id)
    }
    return true
  }

  /**
   * 标记会话退出（terminal-exit 事件回流至 tab 状态，供 tab 栏渲染退出标记）
   */
  function markExited(id) {
    setExited(id, true)
  }

  /**
   * 标记会话恢复运行（重启成功后回流，撤销退出标记；
   * 不复位则黄点/重启按钮在 restartTerminal 后常驻）
   */
  function markRunning(id) {
    setExited(id, false)
  }

  function setExited(id, exited) {
    const tab = tabs.value.find(t => t.id === id)
    if (tab) {
      tab.isExited = exited
    }
  }

  return { tabs, activeId, atLimit, createTab, activateTab, closeTab, markExited, markRunning }
}
