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
      lineHeight: 1.2,
      fontFamily: '"Cascadia Code", "Fira Code", Consolas, "Courier New", monospace',
      theme: {
        background: '#1a1b26',
        foreground: '#a9b1d6',
        cursor: '#c0caf5',
        cursorAccent: '#1a1b26',
        selectionBackground: '#33467c',
        selectionForeground: '#c0caf5',
        black: '#15161e',
        red: '#f7768e',
        green: '#9ece6a',
        yellow: '#e0af68',
        blue: '#7aa2f7',
        magenta: '#bb9af7',
        cyan: '#7dcfff',
        white: '#a9b1d6',
        brightBlack: '#414868',
        brightRed: '#f7768e',
        brightGreen: '#9ece6a',
        brightYellow: '#e0af68',
        brightBlue: '#7aa2f7',
        brightMagenta: '#bb9af7',
        brightCyan: '#7dcfff',
        brightWhite: '#c0caf5'
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
