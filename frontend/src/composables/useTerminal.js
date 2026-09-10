import { ref, watch } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import {
  CreateTerminal,
  WriteTerminalInput,
  ChangeTerminalDir,
  ResizeTerminal,
  CloseTerminal
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { useSettingsStore } from '../store'

/**
 * 终端浅色主题：白底深字，光标主色
 */
export const LIGHT_TERMINAL_THEME = {
  background: '#ffffff',
  foreground: '#303133',
  cursor: '#409eff',
  cursorAccent: '#ffffff',
  selectionBackground: '#409eff20',
  selectionForeground: '#303133',
  black: '#303133',
  red: '#f56c6c',
  green: '#67c23a',
  yellow: '#e6a23c',
  blue: '#409eff',
  magenta: '#e066c1',
  cyan: '#00d4aa',
  white: '#909399',
  brightBlack: '#606266',
  brightRed: '#f56c6c',
  brightGreen: '#67c23a',
  brightYellow: '#e6a23c',
  brightBlue: '#409eff',
  brightMagenta: '#e066c1',
  brightCyan: '#00d4aa',
  brightWhite: '#303133'
}

/**
 * 终端暗色主题：深底浅字，光标主色，与 style.css 暗色背景层次一致
 */
export const DARK_TERMINAL_THEME = {
  background: '#1d1e1f',
  foreground: '#e4e4e4',
  cursor: '#409eff',
  cursorAccent: '#1d1e1f',
  selectionBackground: '#409eff40',
  selectionForeground: '#e4e4e4',
  black: '#1d1e1f',
  red: '#f56c6c',
  green: '#67c23a',
  yellow: '#e6a23c',
  blue: '#409eff',
  magenta: '#e066c1',
  cyan: '#00d4aa',
  white: '#e4e4e4',
  brightBlack: '#7a7a7a',
  brightRed: '#f56c6c',
  brightGreen: '#67c23a',
  brightYellow: '#e6a23c',
  brightBlue: '#66b1ff',
  brightMagenta: '#e066c1',
  brightCyan: '#00d4aa',
  brightWhite: '#ffffff'
}

/**
 * 按实际生效主题选取 xterm 主题对象
 * @param {string} resolved - 'dark' | 'light'
 */
export function getTerminalTheme(resolved) {
  return resolved === 'dark' ? DARK_TERMINAL_THEME : LIGHT_TERMINAL_THEME
}

export function useTerminal() {
  const term = ref(null)
  const fitAddon = ref(null)
  const sessionID = ref('')
  const isActive = ref(false)
  const currentDir = ref('')
  const currentShellType = ref('powershell')
  const isExited = ref(false)

  // 主题 store：读取实际生效主题 resolvedTheme，初始化与切换 xterm 主题
  const settingsStore = useSettingsStore()

  // 初始化终端
  async function initTerminal(container, dir, shellType) {
    if (isActive.value && sessionID.value) {
      return
    }

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      lineHeight: 1.2,
      fontFamily: '"Cascadia Code", "Fira Code", Consolas, "Courier New", monospace',
      theme: getTerminalTheme(settingsStore.resolvedTheme),
      allowProposedApi: true
    })

    const fit = new FitAddon()
    terminal.loadAddon(fit)
    terminal.loadAddon(new WebLinksAddon())

    terminal.open(container)
    fit.fit()

    term.value = terminal
    fitAddon.value = fit
    currentDir.value = dir
    currentShellType.value = shellType || 'powershell'

    const cols = terminal.cols
    const rows = terminal.rows

    // 输出缓冲区：CreateTerminal 返回前收到的输出暂存于此
    // 解决 sessionID 尚未设置时事件回调无法匹配的问题
    const outputBuffer = []

    // 先注册事件监听器，再创建终端，避免 Shell 初始 prompt 输出丢失
    EventsOn('terminal-output', (sid, output) => {
      if (sessionID.value && sid === sessionID.value && term.value) {
        term.value.write(output)
      } else if (!sessionID.value && term.value) {
        // sessionID 尚未赋值，暂存输出
        outputBuffer.push({ sid, output })
      }
    })

    EventsOn('terminal-exit', (sid) => {
      if (sid === sessionID.value) {
        isActive.value = false
        isExited.value = true
        if (term.value) {
          term.value.writeln('\r\n\x1b[33m终端进程已退出。点击「重新启动」恢复。\x1b[0m')
        }
      }
    })

    try {
      const sid = await CreateTerminal(dir, currentShellType.value, cols, rows)
      sessionID.value = sid
      isActive.value = true
      isExited.value = false

      // 刷新缓冲区：将 CreateTerminal 返回前暂存的输出写入终端
      for (const item of outputBuffer) {
        if (item.sid === sid && term.value) {
          term.value.write(item.output)
        }
      }
      outputBuffer.length = 0
    } catch (err) {
      terminal.writeln(`\x1b[31m创建终端失败: ${err}\x1b[0m`)
      EventsOff('terminal-output')
      EventsOff('terminal-exit')
      return
    }

    terminal.onData((data) => {
      if (sessionID.value) {
        WriteTerminalInput(sessionID.value, data).catch(() => {})
      }
    })
  }

  // 切换工作目录
  async function changeDir(dir) {
    if (!sessionID.value || !isActive.value) return
    if (dir === currentDir.value) return
    try {
      await ChangeTerminalDir(sessionID.value, dir)
      currentDir.value = dir
    } catch (err) {
      console.error('切换终端目录失败:', err)
    }
  }

  // 调整大小
  async function resize() {
    if (fitAddon.value && term.value) {
      fitAddon.value.fit()
      if (sessionID.value && isActive.value) {
        try {
          await ResizeTerminal(sessionID.value, term.value.cols, term.value.rows)
        } catch (err) {
          console.error('调整终端大小失败:', err)
        }
      }
    }
  }

  // 聚焦终端
  function focus() {
    if (term.value) {
      term.value.focus()
    }
  }

  // 销毁终端
  async function destroyTerminal() {
    EventsOff('terminal-output')
    EventsOff('terminal-exit')

    if (sessionID.value) {
      try {
        await CloseTerminal(sessionID.value)
      } catch (err) {
        console.error('关闭终端失败:', err)
      }
      sessionID.value = ''
    }

    if (term.value) {
      term.value.dispose()
      term.value = null
    }

    fitAddon.value = null
    isActive.value = false
    isExited.value = false
  }

  // 重新启动终端
  async function restartTerminal(container, dir, shellType) {
    await destroyTerminal()
    await initTerminal(container, dir, shellType)
  }

  // 主题切换：终端已初始化时实时切换 xterm 主题（背景/文字/光标色）
  // 未初始化（term.value 为 null）时跳过，下次 initTerminal 按当时主题初始化
  watch(() => settingsStore.resolvedTheme, (resolved) => {
    if (term.value) {
      term.value.options.theme = getTerminalTheme(resolved)
    }
  })

  return {
    term,
    sessionID,
    isActive,
    isExited,
    currentDir,
    currentShellType,
    initTerminal,
    changeDir,
    resize,
    focus,
    destroyTerminal,
    restartTerminal
  }
}
