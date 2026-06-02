import { ref } from 'vue'
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

export function useTerminal() {
  const term = ref(null)
  const fitAddon = ref(null)
  const sessionID = ref('')
  const isActive = ref(false)
  const currentDir = ref('')
  const currentShellType = ref('powershell')
  const isExited = ref(false)

  // 初始化终端
  async function initTerminal(container, dir, shellType) {
    if (isActive.value && sessionID.value) {
      return
    }

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Consolas, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        selectionBackground: '#264f78'
      },
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

    try {
      const sid = await CreateTerminal(dir, currentShellType.value, cols, rows)
      sessionID.value = sid
      isActive.value = true
      isExited.value = false
    } catch (err) {
      terminal.writeln(`\x1b[31m创建终端失败: ${err}\x1b[0m`)
      return
    }

    terminal.onData((data) => {
      if (sessionID.value) {
        WriteTerminalInput(sessionID.value, data).catch(() => {})
      }
    })

    EventsOn('terminal-output', (sid, output) => {
      if (sid === sessionID.value && term.value) {
        term.value.write(output)
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
    destroyTerminal,
    restartTerminal
  }
}
