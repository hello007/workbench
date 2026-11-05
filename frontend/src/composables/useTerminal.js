import { ref, watch } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { WebglAddon } from '@xterm/addon-webgl'
import { SearchAddon } from '@xterm/addon-search'
import { SerializeAddon } from '@xterm/addon-serialize'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { ElMessageBox } from 'element-plus'
import '@xterm/xterm/css/xterm.css'
import {
  CreateTerminal,
  WriteTerminalInput,
  ChangeTerminalDir,
  ResizeTerminal,
  CloseTerminal,
  OpenWithDefaultApp
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useSettingsStore, FALLBACK_SHELL } from '../store'
import { createOscNotifier } from './terminalOscNotify'
import { createFilePathLinkProvider } from '../utils/terminalLinkProvider'

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
// store 异常时仍保证 xterm 拿到合法配置（scrollback 默认与后端
// model.DefaultTerminalScrollback 对齐为 5000）
export const TERMINAL_DEFAULTS = {
  fontSize: 14,
  scrollback: 5000
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
 * addon 安全挂载：构造 + load 全程 try/catch，失败仅 console 降级，
 * 绝不阻塞终端创建（WebGL/WebGL 不可用、addon 缺字段等异常环境终端照常可用）
 */
function safeLoadAddon(terminal, factory, label) {
  try {
    const addon = factory()
    terminal.loadAddon(addon)
    return addon
  } catch (err) {
    console.warn(`${label} 加载失败（功能降级，不影响终端使用）:`, err)
    return null
  }
}

/**
 * WebGL2 支持性探测：WebglAddon 的渲染器构造发生在 terminal.open 的首帧渲染
 * 事件内（loadAddon 同步段不抛错），不支持环境（jsdom、远程桌面无 GPU 等）
 * 会变成 open 链路的未捕获异常并中断首帧渲染——先探测后挂载，不支持则直接
 * 回退 DOM 渲染，终端照常创建。
 */
function isWebglSupported() {
  try {
    const canvas = document.createElement('canvas')
    return !!(canvas.getContext('webgl2') || canvas.getContext('webgl'))
  } catch {
    return false
  }
}

/**
 * 尝试挂载 WebGL 渲染加速：支持性探测不过或加载抛错均回退默认 DOM 渲染；
 * 挂载成功后监听上下文丢失（GPU 驱动重置等），dispose 该 addon 即回退 DOM
 * 渲染，终端无需重建、功能不受损。
 */
function tryLoadWebgl(terminal) {
  if (!isWebglSupported()) {
    return null
  }
  return safeLoadAddon(terminal, () => {
    const addon = new WebglAddon()
    if (typeof addon.onContextLoss === 'function') {
      addon.onContextLoss(() => {
        try {
          addon.dispose()
        } catch (err) {
          console.warn('WebGL 上下文丢失后的 addon 释放失败:', err)
        }
      })
    }
    return addon
  }, 'WebGL 渲染')
}

// WebLinks 点击 handler：统一 Ctrl（macOS Cmd）+Click 才打开——单击打开是
// addon 默认行为，与本终端面板「Ctrl+Click 语义统一」目标冲突（文件路径
// 链接同为 Ctrl+Click）；未按修饰键时静默返回（不弹浏览器）
function openUrlInBrowser(uri) {
  window.open(uri, '_blank', 'noopener,noreferrer')
}

// 文件路径打开：路径可能不存在（形态匹配不代表存在），OpenWithDefaultApp
// 返回 false 静默、Promise 异常亦静默，不打断终端焦点（无存在性预检，PRD Q3）
function openFilePathSilently(path) {
  Promise.resolve(OpenWithDefaultApp(path)).catch(() => {})
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
  // 当前会话 shell 类型：初始空串（尚未创建会话），initTerminal 时按 tab 指定值解析
  const currentShellType = ref('')
  const isExited = ref(false)

  // 本实例的事件注销闭包（EventsOn 返回值），destroyTerminal 时精准摘除
  let offOutput = null
  let offExit = null

  // 搜索 addon：Ctrl+F 呼出时惰性挂载，关闭搜索条时 dispose 释放
  // （无搜索期间零开销；term.dispose 时 xterm 会连带释放已挂载 addon）
  let searchAddon = null
  // Ctrl+F 呼出回调：由面板侧 setSearchShortcutHandler 注入（xterm 按键
  // handler 只负责拦截，UI 归属面板），未注入时按键不拦截
  let searchShortcutHandler = null
  // OSC 通知处理器（每实例独立节流状态），destroyTerminal 时清空节流表
  let oscNotifier = null
  // 多行粘贴确认的捕获阶段监听器（注册于容器），destroyTerminal 时精准摘除
  let onPasteCapture = null
  let attachedPasteContainer = null

  // 主题 store：读取实际生效主题 resolvedTheme，初始化与切换 xterm 主题
  const settingsStore = useSettingsStore()

  // 初始化终端；options.fontSize/fontFamily/scrollback 显式传值时覆盖设置页外观
  // （外观设置接入点，测试与特殊场景用），缺省从 settings store 读取持久化三字段；
  // options.getTabTitle 可选返回 tab 标题（OSC 通知 title 来源，面板注入）
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
    // URL 链接：统一 Ctrl（macOS Cmd）+Click 打开（覆盖 addon 默认单击打开）
    safeLoadAddon(terminal, () => new WebLinksAddon((event, uri) => {
      if (!(event.ctrlKey || event.metaKey)) return
      openUrlInBrowser(uri)
    }), 'URL 链接')
    // 宽字符宽度修正（unicode11 表）：emoji/CJK 混排列对齐不错位，挂载即生效
    safeLoadAddon(terminal, () => new Unicode11Addon(), 'Unicode11 宽字符修正')
    // WebGL 渲染加速：加载失败或上下文丢失自动回退 DOM 渲染
    tryLoadWebgl(terminal)

    // 文件路径链接（Ctrl+Click 用系统默认程序打开）：在 web-links 之后注册，
    // 同一文本命中时先注册的 provider 优先 → URL 优先于路径形态
    try {
      terminal.registerLinkProvider(createFilePathLinkProvider(terminal, { onOpen: openFilePathSilently }))
    } catch (err) {
      console.warn('文件路径链接注册失败（功能降级，不影响终端使用）:', err)
    }

    // Ctrl+F 搜索条呼出：xterm 自定义按键 handler 拦截（焦点在终端时才走此
    // 路径，搜索条输入框等 DOM 焦点不经 xterm），preventDefault 防浏览器默认
    // 查找，返回 false 防止字符写入终端；无 UI 消费方（面板未注入回调）时放行
    // 不劫持浏览器默认行为；keyup/其余按键放行
    terminal.attachCustomKeyEventHandler((e) => {
      if (e.type !== 'keydown') return true
      if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && (e.key === 'f' || e.key === 'F')) {
        if (!searchShortcutHandler) return true
        e.preventDefault()
        searchShortcutHandler()
        return false
      }
      return true
    })

    // 多行粘贴确认（R6）：xterm 6 无 onTextPaste 公开事件（粘贴链直达
    // triggerDataEvent），拦截点取容器捕获阶段 DOM paste 事件——先于 xterm 的
    // textarea/element 监听执行。含换行的粘贴 shell 会立即逐行执行，弹单次
    // 确认框（confirm-dialog.md 单次确认形态）：preventDefault + stopPropagation
    // 拦下 xterm 默认写入，确认后经 terminal.paste 原生链路写入（保留 \n→\r
    // 归一与 bracketed paste 语义），取消丢弃；单行粘贴直通零打扰
    onPasteCapture = (e) => {
      const text = e.clipboardData ? e.clipboardData.getData('text/plain') : ''
      if (!text || !/[\r\n]/.test(text)) return
      e.preventDefault()
      e.stopPropagation()
      ElMessageBox.confirm(
        '粘贴内容包含多行文本，shell 将立即逐行执行，可能触发不可逆操作。',
        '粘贴多行内容',
        { confirmButtonText: '粘贴', cancelButtonText: '取消', type: 'warning' }
      )
        .then(() => {
          if (term.value) term.value.paste(text)
        })
        .catch(() => {})
    }
    // 幂等防御：同实例重复 init（失败重试等路径）时先摘旧监听，防容器上
    // 堆叠两个拦截闭包导致多行粘贴双弹窗
    if (attachedPasteContainer && onPasteCapture) {
      attachedPasteContainer.removeEventListener('paste', onPasteCapture, true)
    }
    container.addEventListener('paste', onPasteCapture, true)
    attachedPasteContainer = container

    // OSC 9/777/99 通知转应用内提示：总开关读 settings store（默认关闭时
    // handler 放行、开启后即时生效），节流同实例同内容 5s 一条
    oscNotifier = createOscNotifier({
      isEnabled: () => !!settingsStore.terminalOscNotifyEnabled,
      getTitle: () => (typeof options.getTabTitle === 'function' ? options.getTabTitle() : '')
    })
    for (const ident of [9, 777, 99]) {
      terminal.parser.registerOscHandler(ident, (data) => oscNotifier.handle(ident, data))
    }

    terminal.open(container)
    fit.fit()

    term.value = terminal
    fitAddon.value = fit
    currentDir.value = dir
    // 空值兜底链：设置页默认 Shell（用户设置或平台默认，App 启动时已预加载）
    // → FALLBACK_SHELL（后端链路均不可用时的最终回退，Windows 感知不变）。
    // 正常路径 TerminalPanel 建 tab 时已传入非空 shellType，此处仅防御直接调用
    currentShellType.value = shellType || settingsStore.defaultShell || FALLBACK_SHELL

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

  // ── 搜索（Ctrl+F 呼出）──
  // 惰性挂载 SearchAddon 并返回（面板订阅其 onDidChangeResults 展示匹配计数）；
  // 终端未初始化时返回 null（面板侧不弹搜索条）
  function openSearchAddon() {
    if (!term.value) return null
    if (!searchAddon) {
      searchAddon = new SearchAddon()
      term.value.loadAddon(searchAddon)
    }
    return searchAddon
  }

  // 关闭搜索条时释放 addon（「关闭释放」契约：无搜索期间零开销）
  function disposeSearchAddon() {
    if (searchAddon) {
      try {
        searchAddon.dispose()
      } catch (err) {
        console.warn('搜索 addon 释放失败:', err)
      }
      searchAddon = null
    }
  }

  async function searchFindNext(query, searchOptions) {
    const addon = openSearchAddon()
    if (!addon) return
    await addon.findNext(query, searchOptions)
  }

  async function searchFindPrevious(query, searchOptions) {
    const addon = openSearchAddon()
    if (!addon) return
    await addon.findPrevious(query, searchOptions)
  }

  // 面板注入 Ctrl+F 呼出回调（实例级：回调闭包捕获对应 tab 的打开动作）
  function setSearchShortcutHandler(fn) {
    searchShortcutHandler = typeof fn === 'function' ? fn : null
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
    disposeSearchAddon()
    searchShortcutHandler = null
    if (oscNotifier) {
      oscNotifier.dispose()
      oscNotifier = null
    }
    if (attachedPasteContainer && onPasteCapture) {
      attachedPasteContainer.removeEventListener('paste', onPasteCapture, true)
      attachedPasteContainer = null
      onPasteCapture = null
    }

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

  // 重新启动终端（R6）：旧缓冲（scrollback 范围内）经 serialize 回放到新会话。
  // 序列化必须在旧实例 dispose 之前获取——term.dispose() 后缓冲即销毁；
  // serialize 无参默认按终端 scrollback 上限全量序列化（含历史行与光标相对
  // 位移，纯文本 ANSI 序列，不含 OSC 等暂态）。回放失败静默不阻塞新会话
  // （PRD Q1：仅 tab 重启回放内存态，不落盘、不入 session.json）。
  async function restartTerminal(container, dir, shellType, options = {}) {
    let replayText = ''
    if (term.value) {
      try {
        const serializer = new SerializeAddon()
        term.value.loadAddon(serializer)
        replayText = serializer.serialize() || ''
        serializer.dispose()
      } catch (err) {
        console.warn('终端缓冲序列化失败，跳过回放:', err)
        replayText = ''
      }
    }
    await destroyTerminal()
    await initTerminal(container, dir, shellType, options)
    if (replayText && term.value) {
      try {
        term.value.write(replayText)
      } catch (err) {
        console.warn('终端缓冲回放失败（不影响新会话）:', err)
      }
    }
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
    restartTerminal,
    openSearchAddon,
    disposeSearchAddon,
    searchFindNext,
    searchFindPrevious,
    setSearchShortcutHandler
  }
}
