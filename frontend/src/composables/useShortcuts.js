import { ref } from 'vue'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'

const DEFAULTS = {
  commandPalette: 'Ctrl+P',
  toggleTerminal: 'Ctrl+`',
  rename: 'F2',
  delete: 'Delete'
}

const shortcutCommandPalette = ref(DEFAULTS.commandPalette)
const shortcutToggleTerminal = ref(DEFAULTS.toggleTerminal)
const shortcutRename = ref(DEFAULTS.rename)
const shortcutDelete = ref(DEFAULTS.delete)

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

export function useShortcuts() {
  return {
    shortcutCommandPalette,
    shortcutToggleTerminal,
    shortcutRename,
    shortcutDelete,
    parseShortcut,
    matchShortcut,
    formatDisplay,
    isValidShortcut,
    shortcutFromEvent,
    loadShortcuts,
    saveShortcuts,
    checkConflict,
    DEFAULTS
  }
}
