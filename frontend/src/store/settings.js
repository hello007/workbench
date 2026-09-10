import { ref, computed } from 'vue'
import { acceptHMRUpdate, defineStore } from 'pinia'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'

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
  // 系统当前是否偏好暗色（system 模式下决定 resolvedTheme）
  const systemPrefersDark = ref(getSystemPrefersDark())
  // 监听系统主题变化（store 单例生命周期内常驻；system 模式实时跟随）
  watchSystemTheme((isDark) => {
    systemPrefersDark.value = isDark
  })

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
    loadShortcuts,
    saveShortcuts,
    checkConflict,
    loadTheme,
    saveTheme
  }
})

// 无状态纯函数与常量作命名导出，供 spec 与消费方直接 import（store/index.js 一并 re-export）
export { DEFAULTS, parseShortcut, matchShortcut, formatDisplay, isValidShortcut, shortcutFromEvent }

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useSettingsStore, import.meta.hot))
}
