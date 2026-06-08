import { ref } from 'vue'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'

const DEFAULTS = {
  commandPalette: 'Ctrl+P',
  toggleTerminal: 'Ctrl+`'
}

const shortcutCommandPalette = ref(DEFAULTS.commandPalette)
const shortcutToggleTerminal = ref(DEFAULTS.toggleTerminal)

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
 * "Ctrl+P" → ["Ctrl", "P"]
 */
function formatDisplay(str) {
  if (!str) return []
  return str.split('+').map(p => p.trim())
}

/**
 * 验证快捷键字符串是否有效（必须含修饰键 + 按键）
 */
function isValidShortcut(str) {
  const parsed = parseShortcut(str)
  if (!parsed || !parsed.key) return false
  if (!parsed.ctrlKey && !parsed.altKey && !parsed.shiftKey) return false
  return true
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
  } catch {
    shortcutCommandPalette.value = DEFAULTS.commandPalette
    shortcutToggleTerminal.value = DEFAULTS.toggleTerminal
  }
}

/**
 * 保存快捷键配置到后端
 */
async function saveShortcuts() {
  const settings = await GetSettings()
  settings.shortcutCommandPalette = shortcutCommandPalette.value
  settings.shortcutToggleTerminal = shortcutToggleTerminal.value
  await SaveSettings(settings)
}

/**
 * 检查快捷键是否与其他快捷键冲突
 */
function checkConflict(shortcutStr, excludeKey) {
  const all = [
    { key: 'commandPalette', value: shortcutCommandPalette.value },
    { key: 'toggleTerminal', value: shortcutToggleTerminal.value }
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
