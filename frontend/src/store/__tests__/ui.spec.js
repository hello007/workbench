import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useUiStore } from '..'

describe('ui store', () => {
  beforeEach(() => {
    // 每个 it 独立 pinia 实例，避免 store 状态跨用例污染
    setActivePinia(createPinia())
  })

  describe('初始状态', () => {
    it('活动面板默认 directory', () => {
      const store = useUiStore()
      expect(store.activePanel).toBe('directory')
    })

    it('终端默认隐藏，高度 200，跟随目录为空', () => {
      const store = useUiStore()
      expect(store.terminalVisible).toBe(false)
      expect(store.terminalHeight).toBe(200)
      expect(store.terminalDir).toBe('')
    })

    it('终端 tab 快照镜像默认空列表、活动下标 0', () => {
      const store = useUiStore()
      expect(store.terminalTabsSnapshot).toEqual([])
      expect(store.terminalActiveIndex).toBe(0)
    })

    it('终端整窗全屏默认 false', () => {
      const store = useUiStore()
      expect(store.terminalFullscreen).toBe(false)
    })

    it('全部弹窗 visible 默认 false', () => {
      const store = useUiStore()
      expect(store.settingsVisible).toBe(false)
      expect(store.updateDialogVisible).toBe(false)
      expect(store.commandPaletteVisible).toBe(false)
      expect(store.repoFilterVisible).toBe(false)
    })

    it('updateInfo 默认空对象，其余派生态默认空', () => {
      const store = useUiStore()
      expect(store.updateInfo).toEqual({})
      expect(store.contentSearchInit).toBe('')
      expect(store.repoFilterInitialDirId).toBe('')
      expect(store.appVersion).toBe('')
    })
  })

  describe('toggleTerminal', () => {
    it('连续切换 false → true → false', () => {
      const store = useUiStore()
      expect(store.terminalVisible).toBe(false)
      store.toggleTerminal()
      expect(store.terminalVisible).toBe(true)
      store.toggleTerminal()
      expect(store.terminalVisible).toBe(false)
    })

    it('全屏态收起时同步还原全屏态（维护 fullscreen ⇒ visible 不变式）', () => {
      const store = useUiStore()
      store.toggleTerminal()
      store.toggleTerminalFullscreen()
      expect(store.terminalVisible).toBe(true)
      expect(store.terminalFullscreen).toBe(true)
      // 快捷键 toggleTerminal 在全屏态下收起：若只切 visible 会留下
      // fullscreen=true/visible=false 非法组合（快照仍记全屏）
      store.toggleTerminal()
      expect(store.terminalVisible).toBe(false)
      expect(store.terminalFullscreen).toBe(false)
    })

    it('非全屏态收起行为不变（fullscreen 保持 false）', () => {
      const store = useUiStore()
      store.toggleTerminal()
      store.toggleTerminal()
      expect(store.terminalVisible).toBe(false)
      expect(store.terminalFullscreen).toBe(false)
    })
  })

  describe('toggleTerminalFullscreen', () => {
    it('连续切换 false → true → false', () => {
      const store = useUiStore()
      expect(store.terminalFullscreen).toBe(false)
      store.toggleTerminalFullscreen()
      expect(store.terminalFullscreen).toBe(true)
      store.toggleTerminalFullscreen()
      expect(store.terminalFullscreen).toBe(false)
    })

    it('面板隐藏时进入全屏先展开面板（防御 fullscreen ⇒ visible 非法组合）', () => {
      const store = useUiStore()
      expect(store.terminalVisible).toBe(false)
      store.toggleTerminalFullscreen()
      expect(store.terminalVisible).toBe(true)
      expect(store.terminalFullscreen).toBe(true)
    })
  })

  describe('openRepoFilter', () => {
    it('携带 dirId 时锁定到该目录并打开弹窗', () => {
      const store = useUiStore()
      store.openRepoFilter('dir-1')
      expect(store.repoFilterInitialDirId).toBe('dir-1')
      expect(store.repoFilterVisible).toBe(true)
    })

    it('无 dirId 时显式重置 initialDirId 为空（避免残留上次右键锁定目录）', () => {
      const store = useUiStore()
      // 先模拟上次右键锁定一个目录
      store.openRepoFilter('dir-stale')
      expect(store.repoFilterInitialDirId).toBe('dir-stale')
      // 再以无 dirId 入口打开（FileTreePanel 空白右键 / 工具栏按钮）
      store.openRepoFilter()
      expect(store.repoFilterInitialDirId).toBe('')
      expect(store.repoFilterVisible).toBe(true)
    })
  })

  describe('terminalPendingCmd 命令送入通道（AI 对话 × 终端联动一期）', () => {
    it('初始为 null（一次性信箱，消费即清空）', () => {
      const store = useUiStore()
      expect(store.terminalPendingCmd).toBeNull()
    })

    it('pushTerminalPendingCmd 写入 { dir, cmd, seq }，seq 从 1 起单调递增', () => {
      const store = useUiStore()
      store.pushTerminalPendingCmd('D:\\a', 'git status')
      expect(store.terminalPendingCmd).toEqual({ dir: 'D:\\a', cmd: 'git status', seq: 1 })
      store.pushTerminalPendingCmd('D:\\b', 'npm test')
      expect(store.terminalPendingCmd).toEqual({ dir: 'D:\\b', cmd: 'npm test', seq: 2 })
    })

    it('同命令重复写入 seq 仍递增（防 watch 依赖比较去重丢失第二次点击）', () => {
      const store = useUiStore()
      store.pushTerminalPendingCmd('D:\\a', 'git status')
      store.pushTerminalPendingCmd('D:\\a', 'git status')
      expect(store.terminalPendingCmd.seq).toBe(2)
      expect(store.terminalPendingCmd.cmd).toBe('git status')
    })

    it('消费即清空：置 null 后再写入 seq 不回退（跨消费单调）', () => {
      const store = useUiStore()
      store.pushTerminalPendingCmd('D:\\a', 'cmd1')
      // TerminalPanel 消费路径直接置 null 清空信箱
      store.terminalPendingCmd = null
      expect(store.terminalPendingCmd).toBeNull()
      store.pushTerminalPendingCmd('D:\\a', 'cmd2')
      expect(store.terminalPendingCmd).toEqual({ dir: 'D:\\a', cmd: 'cmd2', seq: 2 })
    })

    it('命令原文原样写入不做转义加工（多语言/引号按原文送入，PRD R4）', () => {
      const store = useUiStore()
      const cmd = 'echo "你好" && grep -n "a|b" file'
      store.pushTerminalPendingCmd('D:\\a', cmd)
      expect(store.terminalPendingCmd.cmd).toBe(cmd)
    })
  })
})
