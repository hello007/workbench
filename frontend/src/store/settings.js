import { ref, computed } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'
import { GetSettings, SaveSettings, GetShellConfigs } from '../../wailsjs/go/main/App'

/**
 * 默认快捷键配置
 */
const DEFAULTS = {
  commandPalette: 'Ctrl+P',
  toggleTerminal: 'Ctrl+`',
  rename: 'F2',
  delete: 'Delete'
}

/**
 * 允许作为"单键快捷键"的功能键（无修饰键）。
 * 字母/数字/符号单键不允许，避免与文本输入冲突。
 */
const ALLOWED_SINGLE_KEYS = new Set([
  'f1', 'f2', 'f3', 'f4', 'f5', 'f6', 'f7', 'f8', 'f9', 'f10', 'f11', 'f12',
  'delete', 'insert', 'home', 'end', 'pageup', 'pagedown',
  'arrowup', 'arrowdown', 'arrowleft', 'arrowright'
])

/**
 * 将快捷键字符串解析为事件匹配对象
 * "Ctrl+P" → { ctrlKey: true, altKey: false, shiftKey: false, key: "p" }
 */
function parseShortcut(str) {
  if (!str) return null
  const parts = str.split('+').map(p => p.trim())
  const result = { ctrlKey: false, altKey: false, shiftKey: false, key: '' }
  for (const part of parts) {
    const lower = part.toLowerCase()
    if (lower === 'ctrl') result.ctrlKey = true
    else if (lower === 'alt') result.altKey = true
    else if (lower === 'shift') result.shiftKey = true
    else result.key = lower
  }
  return result
}

/**
 * 判断键盘事件是否匹配快捷键字符串
 */
function matchShortcut(event, shortcutStr) {
  const parsed = parseShortcut(shortcutStr)
  if (!parsed || !parsed.key) return false
  return (
    event.ctrlKey === parsed.ctrlKey &&
    event.altKey === parsed.altKey &&
    event.shiftKey === parsed.shiftKey &&
    event.key.toLowerCase() === parsed.key
  )
}

/**
 * 将快捷键字符串格式化为显示用数组
 * "Ctrl+P" → ["Ctrl", "P"]；"F2" → ["F2"]
 */
function formatDisplay(str) {
  if (!str) return []
  return str.split('+').map(p => p.trim())
}

/**
 * 验证快捷键字符串是否有效：
 * - 含 Ctrl/Alt/Shift 修饰键时，按键任意非空即可；
 * - 无修饰键时，必须是白名单功能键单键（F1-F12、Delete 等）。
 */
function isValidShortcut(str) {
  const parsed = parseShortcut(str)
  if (!parsed || !parsed.key) return false
  if (parsed.ctrlKey || parsed.altKey || parsed.shiftKey) return true
  return ALLOWED_SINGLE_KEYS.has(parsed.key)
}

/**
 * 从键盘事件生成快捷键字符串
 */
function shortcutFromEvent(event) {
  const parts = []
  if (event.ctrlKey) parts.push('Ctrl')
  if (event.altKey) parts.push('Alt')
  if (event.shiftKey) parts.push('Shift')
  if (event.key && !['Control', 'Alt', 'Shift', 'Meta'].includes(event.key)) {
    parts.push(event.key.length === 1 ? event.key.toUpperCase() : event.key)
  }
  return parts.join('+')
}

/**
 * 设置 store
 *
 * 收敛原 composables/useShortcuts.js 的模块级单例 ref：
 * shortcutCommandPalette / shortcutToggleTerminal / shortcutRename / shortcutDelete。
 * loadShortcuts / saveShortcuts / checkConflict 作为 action 访问 store 内 ref；
 * parseShortcut / matchShortcut / formatDisplay / isValidShortcut / shortcutFromEvent / DEFAULTS
 * 为无状态纯函数与常量，作模块级命名导出，供 spec 与消费方直接 import。
 *
 * 主题态（themeMode / resolvedTheme）同样收敛于此 store：
 * themeMode 三态 system/light/dark，system 模式经 matchMedia 解析为实际生效主题 resolvedTheme。
 * loadTheme/saveTheme 走现有 GetSettings/SaveSettings（settings.json 通用字段 themeMode）。
 */
// 主题模式合法值集合；非法或缺失值统一回退 system
const THEME_MODES = new Set(['system', 'light', 'dark'])

/**
 * 外部 diff 工具预设模板（G10 平台化，按 diffToolPresetsFor(isWindows) 分流）：
 * 选中预设即填充默认路径与参数模板，二者仍可手动修改。
 * Windows 表（既有，逐字节保留）：path 为常用默认安装路径（vscode 依赖 PATH 中的 code 命令，与 OpenInVSCode 一致）。
 * Linux 表（G10 新增）：path 用 `command -v` 可命中的裸命令名（meld/kdiff3/bcompare 的
 * deb 包安装后均落 /usr/bin），避免选中即写入不存在的 Windows 路径；beyondcompare key
 * 两表共用，保证 loadDiffTool 空值回退名与存量 settings.json 在两平台下拉中均有对应项不裸值。
 * args 均须包含 {left} {right} 占位符，由后端渲染为实际文件路径。
 */
export const DIFF_TOOL_PRESETS = {
  beyondcompare: { label: 'Beyond Compare', path: 'C:\\Program Files\\Beyond Compare 5\\BComp.exe', args: '{left} {right}' },
  winmerge: { label: 'WinMerge', path: 'C:\\Program Files\\WinMerge\\WinMergeU.exe', args: '{left} {right}' },
  vscode: { label: 'VSCode diff', path: 'code', args: '--diff --wait {left} {right}' },
  custom: { label: '自定义', path: '', args: '{left} {right}' }
}

/** Linux 平台 diff 工具预设表（G10，消费方经 diffToolPresetsFor 取表，勿直接引用） */
export const DIFF_TOOL_PRESETS_LINUX = {
  beyondcompare: { label: 'Beyond Compare', path: 'bcompare', args: '{left} {right}' },
  meld: { label: 'Meld', path: 'meld', args: '{left} {right}' },
  kdiff3: { label: 'KDiff3', path: 'kdiff3', args: '{left} {right}' },
  vscode: { label: 'VSCode diff', path: 'code', args: '--diff --wait {left} {right}' },
  custom: { label: '自定义', path: '', args: '{left} {right}' }
}

/**
 * 按平台返回 diff 工具预设表（G10）：isWindows 由消费方传 settings store 的
 * isWindowsPlatform（与 R7 平台判定同源），Windows 保留既有表零行为变化。
 */
export function diffToolPresetsFor(isWindows) {
  return isWindows ? DIFF_TOOL_PRESETS : DIFF_TOOL_PRESETS_LINUX
}

/**
 * 终端外观默认值与取值边界（与后端 model.AppSettings 常量对齐）：
 * 字号 10-24（默认 14）、回滚行数 1000-10000（默认 1000）。
 * fontFamily 空串 = 默认 Cascadia Code 字体栈（useTerminal buildTerminalFontFamily 消费）。
 */
export const TERMINAL_APPEARANCE_DEFAULTS = {
  fontSize: 14,
  fontFamily: '',
  scrollback: 1000
}
export const TERMINAL_FONT_SIZE_MIN = 10
export const TERMINAL_FONT_SIZE_MAX = 24
export const TERMINAL_SCROLLBACK_MIN = 1000
export const TERMINAL_SCROLLBACK_MAX = 10000

/** 终端字体下拉选项（value 空串 = 默认 Cascadia Code 栈，含 Fira Code/Consolas fallback 链） */
export const TERMINAL_FONT_OPTIONS = [
  { label: '默认（Cascadia Code）', value: '' },
  { label: 'Consolas', value: 'Consolas' },
  { label: 'Courier New', value: 'Courier New' }
]

/**
 * 默认 Shell 最终兜底值：用户设置与平台 Shell 列表均不可用时回退 powershell
 * （Windows 现状不变；Linux 正常链路下 GetShellConfigs 必返回 bash 首项，不会触达）
 */
export const FALLBACK_SHELL = 'powershell'

/**
 * 解析默认 Shell（跨平台残留设置收敛，单一实现供 store 与组件消费方共用）：
 * - 列表可用且用户设置值在列表内 → 采用用户设置
 * - 列表可用但用户设置未设置、或为跨平台残留值（如 settings.json 存着 Windows 时期的
 *   gitbash/wsl 而当前为 Linux 主机）→ 收敛到平台默认（列表首项：Windows=powershell、
 *   Linux=bash），保证 el-select 显示值 / 条件渲染 / tab 副标签 / 实际 PTY shellType
 *   四处一致，不留裸值
 * - 列表不可用（RPC 失败 / 空列表）：保留用户设置（Windows 感知零变化），未设置才回退
 *   FALLBACK_SHELL
 */
export function resolveDefaultShell(userShell, configs) {
  if (!Array.isArray(configs) || configs.length === 0) {
    return userShell || FALLBACK_SHELL
  }
  if (userShell && configs.some(c => c.type === userShell)) return userShell
  return configs[0].type
}

/**
 * 读取系统是否偏好暗色主题。
 * SSR / 无 matchMedia 环境（jsdom 未注入时）回退 false。
 */
function getSystemPrefersDark() {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  try {
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  } catch {
    return false
  }
}

/**
 * 注册系统主题变化监听器，返回注销函数。
 * 优先 addEventListener（标准），回退 addListener（旧 Safari）。
 * 无 matchMedia 环境返回空函数。
 */
function watchSystemTheme(onChange) {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return () => {}
  let mql
  try {
    mql = window.matchMedia('(prefers-color-scheme: dark)')
  } catch {
    return () => {}
  }
  const handler = (e) => onChange(e.matches)
  if (typeof mql.addEventListener === 'function') {
    mql.addEventListener('change', handler)
    return () => mql.removeEventListener('change', handler)
  }
  if (typeof mql.addListener === 'function') {
    mql.addListener(handler)
    return () => mql.removeListener(handler)
  }
  return () => {}
}

export const useSettingsStore = defineStore('settings', () => {
  const shortcutCommandPalette = ref(DEFAULTS.commandPalette)
  const shortcutToggleTerminal = ref(DEFAULTS.toggleTerminal)
  const shortcutRename = ref(DEFAULTS.rename)
  const shortcutDelete = ref(DEFAULTS.delete)

  // 主题模式：system(跟随系统) / light / dark，默认 system
  const themeMode = ref('system')

  // 外部 diff 工具配置：预设名 / 可执行文件路径 / 参数模板（{left} {right} 占位符）。
  // 收敛于此 store 供 SettingsPanel（配置）与 FileDiffDialog（按钮态判断）共享。
  const diffToolName = ref('beyondcompare')
  const diffToolPath = ref('')
  const diffToolArgs = ref('')
  // 是否已配置：与后端校验对称——路径非空且参数模板含 {left}{right} 占位符，
  // 避免按钮可用但点击即报「参数模板须包含占位符」
  const diffToolConfigured = computed(() =>
    diffToolPath.value.trim() !== '' &&
    diffToolArgs.value.includes('{left}') &&
    diffToolArgs.value.includes('{right}')
  )
  // 系统当前是否偏好暗色（system 模式下决定 resolvedTheme）
  const systemPrefersDark = ref(getSystemPrefersDark())
  // 监听系统主题变化（store 单例生命周期内常驻；system 模式实时跟随）
  watchSystemTheme((isDark) => {
    systemPrefersDark.value = isDark
  })

  // 终端外观三字段：字号 / 字体族 / 回滚行数。
  // SettingsPanel（设置页控件）与 TerminalPanel（A-/A+ 快捷调节）共享同一 store 字段实现双向同步；
  // useTerminal 消费三字段初始化 xterm，字号/字体变化经 watch 热更已开终端
  const terminalFontSize = ref(TERMINAL_APPEARANCE_DEFAULTS.fontSize)
  const terminalFontFamily = ref(TERMINAL_APPEARANCE_DEFAULTS.fontFamily)
  const terminalScrollback = ref(TERMINAL_APPEARANCE_DEFAULTS.scrollback)

  // 默认 Shell 类型：settings.json defaultShell（用户设置）优先；未设置时取平台默认
  // （GetShellConfigs 首项：Windows=powershell、Linux=bash）；后端均不可用回退 FALLBACK_SHELL。
  // App 启动时经 loadDefaultShell 预加载，useTerminal 以此兜底空 shellType 参数，
  // 避免 useTerminal 内散落 'powershell' 硬编码（Linux 下会误开 PowerShell）
  const defaultShell = ref('')

  // 平台 Shell 列表缓存（G8-G10 平台信号单一数据源）：loadDefaultShell / SettingsPanel
  // loadSettings 拉到列表时写入，作 isWindowsPlatform 判定输入，避免多组件各自 RPC。
  // 拉取失败保留旧值（启动早期为空数组）
  const shellConfigsCache = ref([])

  // 平台判定信号（R7 同款，禁 navigator.userAgent）：PTY/文件操作跑在应用进程所在主机，
  // serve 模式下与浏览器端设备可能不同机，UA 反映浏览器端平台方向相反不可用。
  // 判定：列表首项 type 为 powershell ⇔ Windows（GetShellConfigs 按服务端 GOOS 返回，
  // Windows=powershell 首项、Linux=bash 首项）。列表未加载（空）时保守按 Windows 处理：
  // Windows 用户可见行为零变化（占位符/预设均为历史文案），Linux 下 loadDefaultShell
  // 正常完成后必为非空列表，仅在 RPC 失败的降级场景短暂回退现状
  const isWindowsPlatform = computed(() => {
    const first = shellConfigsCache.value[0]
    return !first || first.type === 'powershell'
  })

  /**
   * 缓存平台 Shell 列表（G8-G10 信号源写入点）：loadDefaultShell 与 SettingsPanel
   * loadSettings 拉到非空列表时调用，供 isWindowsPlatform 判定与平台化占位符/预设消费。
   */
  function cacheShellConfigs(configs) {
    if (Array.isArray(configs) && configs.length > 0) {
      shellConfigsCache.value = configs
    }
  }

  // 字号 clamp：非正数/非法值回退默认，界内取整后收敛到 [10, 24]
  function clampTerminalFontSize(v) {
    const n = Number(v)
    if (!Number.isFinite(n) || n <= 0) return TERMINAL_APPEARANCE_DEFAULTS.fontSize
    return Math.min(Math.max(Math.round(n), TERMINAL_FONT_SIZE_MIN), TERMINAL_FONT_SIZE_MAX)
  }

  // 回滚行数 clamp：非正数/非法值回退默认，界内取整后收敛到 [1000, 10000]
  function clampTerminalScrollback(v) {
    const n = Number(v)
    if (!Number.isFinite(n) || n <= 0) return TERMINAL_APPEARANCE_DEFAULTS.scrollback
    return Math.min(Math.max(Math.round(n), TERMINAL_SCROLLBACK_MIN), TERMINAL_SCROLLBACK_MAX)
  }

  /**
   * 从后端加载终端外观三字段（后端 Load 已补默认值，此处仍 clamp 兜底
   * 手改配置文件越界值与 GetSettings 异常返回的 0 值）
   */
  async function loadTerminalAppearance() {
    try {
      const settings = await GetSettings()
      terminalFontSize.value = clampTerminalFontSize(settings.terminalFontSize)
      terminalFontFamily.value = settings.terminalFontFamily || ''
      terminalScrollback.value = clampTerminalScrollback(settings.terminalScrollback)
    } catch {
      // 读取失败保持当前值（默认或上次加载值），不阻塞设置页
    }
  }

  /**
   * 保存终端外观三字段到后端（合并写，避免覆盖其他字段）
   */
  async function saveTerminalAppearance() {
    const settings = await GetSettings()
    settings.terminalFontSize = terminalFontSize.value
    settings.terminalFontFamily = terminalFontFamily.value
    settings.terminalScrollback = terminalScrollback.value
    await SaveSettings(settings)
  }

  /**
   * A-/A+ 字号步进：clamp 后写 store 并异步持久化。
   * 越界（10/24 边界外步进）不变不存，返回 false 供调用方禁用按钮。
   */
  function stepTerminalFontSize(delta) {
    const next = clampTerminalFontSize(terminalFontSize.value + delta)
    if (next === terminalFontSize.value) return false
    terminalFontSize.value = next
    saveTerminalAppearance().catch(() => {})
    return true
  }

  /**
   * 加载默认 Shell 类型（App 启动时调用）：
   * 1. settings.json defaultShell（用户显式设置）优先——但须在平台 Shell 列表内，
   *    跨平台残留值（如 Windows 时期设置的 gitbash/wsl 在 Linux 主机）收敛到平台默认
   * 2. 未设置或残留值时取 GetShellConfigs 首项（后端按平台返回：Windows=powershell、Linux=bash）
   * 3. 后端均不可用（RPC 桥异常等）保留用户设置、未设置则回退 FALLBACK_SHELL，Windows 感知零变化
   * 解析逻辑单一实现在模块级 resolveDefaultShell（与组件消费方共用），此处仅负责取数。
   */
  async function loadDefaultShell() {
    let configs = null
    try {
      const c = await GetShellConfigs()
      if (Array.isArray(c) && c.length > 0) configs = c
    } catch {
      // 平台 Shell 列表不可用，configs 保持 null（按不可校验处理）
    }
    let userShell = ''
    try {
      const settings = await GetSettings()
      if (settings && settings.defaultShell) userShell = settings.defaultShell
    } catch {
      // 用户设置读取失败，按未设置处理
    }
    defaultShell.value = resolveDefaultShell(userShell, configs)
    // G8-G10 平台信号：缓存列表供 isWindowsPlatform 判定（拉取失败保留旧值/空 → 按 Windows 兜底）
    cacheShellConfigs(configs)
  }

  // 实际生效主题：light/dark。system 模式按系统偏好解析，light/dark 直接取值
  const resolvedTheme = computed(() => {
    if (themeMode.value === 'light') return 'light'
    if (themeMode.value === 'dark') return 'dark'
    return systemPrefersDark.value ? 'dark' : 'light'
  })

  /**
   * 从后端加载主题配置；缺失或非法值回退 system
   */
  async function loadTheme() {
    try {
      const settings = await GetSettings()
      themeMode.value = THEME_MODES.has(settings.themeMode) ? settings.themeMode : 'system'
    } catch {
      themeMode.value = 'system'
    }
  }

  /**
   * 保存主题配置到后端（合并写：先读现有 settings 再覆盖 themeMode，避免覆盖其他字段）
   */
  async function saveTheme() {
    const settings = await GetSettings()
    settings.themeMode = themeMode.value
    await SaveSettings(settings)
  }

  /**
   * 从后端加载外部 diff 工具配置，空值回退默认预设名
   */
  async function loadDiffTool() {
    try {
      const settings = await GetSettings()
      diffToolName.value = settings.diffToolName || 'beyondcompare'
      diffToolPath.value = settings.diffToolPath || ''
      diffToolArgs.value = settings.diffToolArgs || ''
    } catch {
      diffToolName.value = 'beyondcompare'
      diffToolPath.value = ''
      diffToolArgs.value = ''
    }
  }

  /**
   * 保存外部 diff 工具配置到后端（合并写，避免覆盖其他字段）
   */
  async function saveDiffTool() {
    const settings = await GetSettings()
    settings.diffToolName = diffToolName.value
    settings.diffToolPath = diffToolPath.value
    settings.diffToolArgs = diffToolArgs.value
    await SaveSettings(settings)
  }

  /**
   * 从后端加载快捷键配置，空值填默认
   */
  async function loadShortcuts() {
    try {
      const settings = await GetSettings()
      shortcutCommandPalette.value = settings.shortcutCommandPalette || DEFAULTS.commandPalette
      shortcutToggleTerminal.value = settings.shortcutToggleTerminal || DEFAULTS.toggleTerminal
      shortcutRename.value = settings.shortcutRename || DEFAULTS.rename
      shortcutDelete.value = settings.shortcutDelete || DEFAULTS.delete
    } catch {
      shortcutCommandPalette.value = DEFAULTS.commandPalette
      shortcutToggleTerminal.value = DEFAULTS.toggleTerminal
      shortcutRename.value = DEFAULTS.rename
      shortcutDelete.value = DEFAULTS.delete
    }
  }

  /**
   * 保存快捷键配置到后端
   */
  async function saveShortcuts() {
    const settings = await GetSettings()
    settings.shortcutCommandPalette = shortcutCommandPalette.value
    settings.shortcutToggleTerminal = shortcutToggleTerminal.value
    settings.shortcutRename = shortcutRename.value
    settings.shortcutDelete = shortcutDelete.value
    await SaveSettings(settings)
  }

  /**
   * 检查快捷键是否与其他快捷键冲突
   */
  function checkConflict(shortcutStr, excludeKey) {
    const all = [
      { key: 'commandPalette', value: shortcutCommandPalette.value },
      { key: 'toggleTerminal', value: shortcutToggleTerminal.value },
      { key: 'rename', value: shortcutRename.value },
      { key: 'delete', value: shortcutDelete.value }
    ]
    for (const item of all) {
      if (item.key === excludeKey) continue
      if (item.value === shortcutStr) return item
    }
    return null
  }

  return {
    shortcutCommandPalette,
    shortcutToggleTerminal,
    shortcutRename,
    shortcutDelete,
    themeMode,
    systemPrefersDark,
    resolvedTheme,
    diffToolName,
    diffToolPath,
    diffToolArgs,
    diffToolConfigured,
    terminalFontSize,
    terminalFontFamily,
    terminalScrollback,
    defaultShell,
    shellConfigsCache,
    isWindowsPlatform,
    cacheShellConfigs,
    loadShortcuts,
    saveShortcuts,
    checkConflict,
    loadTheme,
    saveTheme,
    loadDiffTool,
    saveDiffTool,
    loadTerminalAppearance,
    saveTerminalAppearance,
    loadDefaultShell,
    stepTerminalFontSize
  }
})

// 无状态纯函数与常量作命名导出，供 spec 与消费方直接 import（store/index.js 一并 re-export）
export { DEFAULTS, parseShortcut, matchShortcut, formatDisplay, isValidShortcut, shortcutFromEvent }

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useSettingsStore, import.meta.hot))
}
