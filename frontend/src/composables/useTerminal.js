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
import { EventsOn } from '../../wailsjs/runtime/runtime'
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

// 终端兜底默认外观（字号/回滚行数）。正常路径由设置页持久化值
// （settings store terminalFontSize/terminalFontFamily/terminalScrollback）覆盖；
// store 异常时仍保证 xterm 拿到合法配置
export const TERMINAL_DEFAULTS = {
  fontSize: 14,
  scrollback: 1000
}

/**
 * 构建终端 fontFamily：设置页选值（或默认 Cascadia Code 栈）为主字体，
 * 补齐等宽 fallback 链，保证缺字形字符仍可渲染
 * @param {string} fontFamily - 设置页保存的字体族名；空串 = 默认 Cascadia Code 栈
 */
export function buildTerminalFontFamily(fontFamily) {
  const primary = fontFamily ? `"${fontFamily}"` : '"Cascadia Code", "Fira Code"'
  return `${primary}, Consolas, "Courier New", monospace`
}

/**
 * 单终端会话 composable：一次实例对应一个 shell 进程（一个 tab）。
 * 多终端 = 多次调用 useTerminal() 各自持有独立 sessionID/事件监听。
 * 事件监听用 EventsOn 返回的闭包精准注销，禁全局 EventsOff
 * （会误删其他实例的同名监听器，见 docs/spec/cross-layer-contracts.md）。
 */
export function useTerminal() {
  const term = ref(null)
  const fitAddon = ref(null)
  const sessionID = ref('')
  const isActive = ref(false)
  const currentDir = ref('')
  const currentShellType = ref('powershell')
  const isExited = ref(false)

  // 本实例的事件注销闭包（EventsOn 返回值），destroyTerminal 时精准摘除
  let offOutput = null
  let offExit = null

  // 主题 store：读取实际生效主题 resolvedTheme，初始化与切换 xterm 主题
  const settingsStore = useSettingsStore()

  // 初始化终端；options.fontSize/fontFamily/scrollback 显式传值时覆盖设置页外观
  // （外观设置接入点，测试与特殊场景用），缺省从 settings store 读取持久化三字段
  async function initTerminal(container, dir, shellType, options = {}) {
    if (isActive.value && sessionID.value) {
      return
    }

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: options.fontSize ?? settingsStore.terminalFontSize,
      scrollback: options.scrollback ?? settingsStore.terminalScrollback,
      lineHeight: 1.2,
      fontFamily: options.fontFamily ?? buildTerminalFontFamily(settingsStore.terminalFontFamily),
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
    // 解决 sessionID 尚未设置时事件回调无法匹配的问题；
    // 多实例并存时各自缓冲、按 sid 过滤刷新，互不干扰
    const outputBuffer = []

    // 先注册事件监听器，再创建终端，避免 Shell 初始 prompt 输出丢失；
    // EventsOn 返回注销闭包，仅摘除本实例监听器
    offOutput = EventsOn('terminal-output', (sid, output) => {
      if (sessionID.value && sid === sessionID.value && term.value) {
        term.value.write(output)
      } else if (!sessionID.value && term.value) {
        // sessionID 尚未赋值，暂存输出
        outputBuffer.push({ sid, output })
      }
    })

    offExit = EventsOn('terminal-exit', (sid) => {
      if (sid === sessionID.value) {
        isActive.value = false
        isExited.value = true
        // 退出提示由 TerminalPanel 面板内叠加提示条呈现（warning 语义色 + 重启入口），
        // 不再向终端缓冲写裸色转义文案
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
      unregisterEvents()
      return
    }

    terminal.onData((data) => {
      if (sessionID.value) {
        WriteTerminalInput(sessionID.value, data).catch(() => {})
      }
    })
  }

  // 精准注销本实例的事件监听器（闭包模式，禁 EventsOff 全局移除）
  function unregisterEvents() {
    if (offOutput) {
      offOutput()
      offOutput = null
    }
    if (offExit) {
      offExit()
      offExit = null
    }
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
    unregisterEvents()

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

  // 终端外观热更：设置页控件 / 工具栏 A-/A+ 修改字号或字体后，已开终端实时生效。
  // xterm 支持 options 热更；字号/字体变化改变 cols/rows 计算，须 refit 并通知后端
  // PTY 同步尺寸（复用 resize() 的 fit + ResizeTerminal 链路）。
  // scrollback 不热更（新建终端生效），未初始化时跳过（下次 initTerminal 按当时配置）
  watch(
    [() => settingsStore.terminalFontSize, () => settingsStore.terminalFontFamily],
    () => {
      if (!term.value) return
      term.value.options.fontSize = settingsStore.terminalFontSize
      term.value.options.fontFamily = buildTerminalFontFamily(settingsStore.terminalFontFamily)
      resize()
    }
  )

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
