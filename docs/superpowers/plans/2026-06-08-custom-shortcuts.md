# 自定义快捷键实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 支持用户自定义「打开命令面板」和「切换终端」的快捷键，设置面板可录制新快捷键，右键菜单显示快捷键提示。

**Architecture:** 新增 `useShortcuts` composable 负责快捷键解析、匹配和持久化。AppSettings 扩展两个快捷键字段。Home.vue 的 `handleGlobalKeydown` 从硬编码改为动态匹配。设置面板快捷键 tab 支持点击录制。FileTreePanel 右键菜单追加快捷键提示文本。

**Tech Stack:** Vue 3 Composition API、Go（AppSettings）、Wails 绑定

---

## 文件结构

| 操作 | 文件 | 职责 |
|---|---|---|
| **新建** | `frontend/src/composables/useShortcuts.js` | 快捷键解析、匹配、加载、保存 |
| **修改** | `model/settings.go` | 新增 `ShortcutCommandPalette`、`ShortcutToggleTerminal` 字段 |
| **修改** | `frontend/src/components/SettingsPanel.vue` | 快捷键 tab 支持录制交互 |
| **修改** | `frontend/src/views/Home.vue` | `handleGlobalKeydown` 改用动态快捷键匹配 |
| **修改** | `frontend/src/components/FileTreePanel.vue` | 右键菜单追加快捷键提示 |

---

### Task 1: 扩展 AppSettings — 快捷键字段

**Files:**
- Modify: `model/settings.go`

- [ ] **Step 1: 新增快捷键字段**

在 `AppSettings` 结构体中追加两个字段：

```go
type AppSettings struct {
	GpuDisabled             bool     `json:"gpuDisabled"`
	DefaultShell            string   `json:"defaultShell"`
	GitBashPath             string   `json:"gitBashPath"`
	WslDistro               string   `json:"wslDistro"`
	SearchExcludeDirs       []string `json:"searchExcludeDirs"`
	SearchExcludeFiles      []string `json:"searchExcludeFiles"`
	ShortcutCommandPalette  string   `json:"shortcutCommandPalette"` // 命令面板快捷键，默认 "Ctrl+P"
	ShortcutToggleTerminal  string   `json:"shortcutToggleTerminal"` // 切换终端快捷键，默认 "Ctrl+`"
}
```

- [ ] **Step 2: 运行测试确认无破坏**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add model/settings.go
git commit -m "feat(shortcuts): add shortcut config fields to AppSettings

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: 新增 useShortcuts composable

**Files:**
- Create: `frontend/src/composables/useShortcuts.js`

- [ ] **Step 1: 创建 useShortcuts.js**

```js
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
 * excludeKey: 排除自身（编辑时自身不算冲突）
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
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/composables/useShortcuts.js
git commit -m "feat(shortcuts): add useShortcuts composable for shortcut parsing and matching

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Home.vue — 动态快捷键匹配

**Files:**
- Modify: `frontend/src/views/Home.vue`

- [ ] **Step 1: 引入 useShortcuts 并替换硬编码按键判断**

在 `<script setup>` 的 import 区域新增：

```js
import { useShortcuts } from '../composables/useShortcuts'
```

在 `commandPaletteVisible` ref 声明之后新增：

```js
const { matchShortcut, loadShortcuts, shortcutCommandPalette, shortcutToggleTerminal } = useShortcuts()
```

在 `onMounted` 中新增 `loadShortcuts()` 调用（在 `loadDirectories()` 之后）：

```js
onMounted(() => {
  loadDirectories()
  loadShortcuts()
  GetAppVersion().then(v => { appVersion.value = v }).catch(() => {})
  document.addEventListener('keydown', handleGlobalKeydown)
  window.addEventListener('focus', handleWindowFocus)
})
```

- [ ] **Step 2: 替换 handleGlobalKeydown 中的硬编码判断**

将现有的 `handleGlobalKeydown` 函数中前两个 if 替换：

替换前：
```js
  // Ctrl+P 打开命令面板
  if (e.ctrlKey && e.key === 'p') {
    e.preventDefault()
    commandPaletteVisible.value = true
    return
  }

  // Ctrl+` 切换终端
  if (e.key === '`' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault()
    toggleTerminal()
    return
  }
```

替换后：
```js
  // 打开命令面板（快捷键可自定义）
  if (matchShortcut(e, shortcutCommandPalette.value)) {
    e.preventDefault()
    commandPaletteVisible.value = true
    return
  }

  // 切换终端（快捷键可自定义）
  if (matchShortcut(e, shortcutToggleTerminal.value)) {
    e.preventDefault()
    toggleTerminal()
    return
  }
```

注意：F5、Ctrl+C/X/V 保持原样不变。

- [ ] **Step 3: Commit**

```bash
git add frontend/src/views/Home.vue
git commit -m "feat(shortcuts): use dynamic shortcut matching in handleGlobalKeydown

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: SettingsPanel — 快捷键录制交互

**Files:**
- Modify: `frontend/src/components/SettingsPanel.vue`

- [ ] **Step 1: 引入 useShortcuts**

在 import 区域新增：

```js
import { useShortcuts } from '../composables/useShortcuts'
```

在 `newExcludeFile` ref 声明之后新增：

```js
const { shortcutCommandPalette, shortcutToggleTerminal, formatDisplay, isValidShortcut, shortcutFromEvent, checkConflict, loadShortcuts, saveShortcuts, DEFAULTS } = useShortcuts()

const recordingKey = ref(null) // 当前正在录制的快捷键 key: 'commandPalette' | 'toggleTerminal' | null
const recordingText = ref('')

// 固定快捷键列表（不可自定义）
const fixedShortcuts = [
  { action: '刷新当前节点', keys: ['F5'] },
  { action: '复制选中项', keys: ['Ctrl', 'C'] },
  { action: '剪切选中项', keys: ['Ctrl', 'X'] },
  { action: '粘贴', keys: ['Ctrl', 'V'] }
]

// 可自定义快捷键列表
const customizableShortcuts = computed(() => [
  { action: '打开命令面板', key: 'commandPalette', keys: formatDisplay(shortcutCommandPalette.value), customizable: true },
  { action: '切换终端面板', key: 'toggleTerminal', keys: formatDisplay(shortcutToggleTerminal.value), customizable: true }
])

// 获取快捷键显示值
function getShortcutDisplay(item) {
  if (recordingKey.value === item.key) return recordingText.value ? formatDisplay(recordingText.value) : ['请按下新快捷键...']
  return item.keys
}

// 开始录制
function startRecording(key) {
  recordingKey.value = key
  recordingText.value = ''
}

// 取消录制
function cancelRecording() {
  recordingKey.value = null
  recordingText.value = ''
}

// 处理录制按键
function handleRecordingKeydown(e) {
  if (!recordingKey.value) return false

  // Escape 取消录制
  if (e.key === 'Escape') {
    cancelRecording()
    return true
  }

  const shortcut = shortcutFromEvent(e)
  if (!shortcut || !isValidShortcut(shortcut)) return true

  // 检查冲突
  const conflict = checkConflict(shortcut, recordingKey.value)
  if (conflict) {
    ElMessage.warning(`快捷键冲突：与"${conflict.key === 'commandPalette' ? '打开命令面板' : '切换终端面板'}"相同`)
    return true
  }

  // 保存
  if (recordingKey.value === 'commandPalette') {
    shortcutCommandPalette.value = shortcut
  } else if (recordingKey.value === 'toggleTerminal') {
    shortcutToggleTerminal.value = shortcut
  }

  recordingKey.value = null
  recordingText.value = ''
  saveShortcuts()
  return true
}
```

需要确保 import 中有 `ElMessage`（已存在）和 `computed`（已存在）。

- [ ] **Step 2: 替换快捷键 tab 模板**

将当前的快捷键 tab 替换为支持录制的版本：

```html
        <div v-show="activeTab === 'shortcuts'" class="shortcuts-tab" @keydown="handleRecordingKeydown" tabindex="-1">
          <div class="settings-section-title">快捷键</div>
          <div class="shortcut-list">
            <!-- 可自定义快捷键 -->
            <div
              v-for="item in customizableShortcuts"
              :key="item.key"
              class="shortcut-item"
              :class="{ 'shortcut-item--recording': recordingKey === item.key }"
            >
              <div class="shortcut-action">{{ item.action }}</div>
              <div
                class="shortcut-keys shortcut-keys--editable"
                @click="startRecording(item.key)"
              >
                <template v-if="recordingKey === item.key">
                  <kbd class="recording-hint">请按下新快捷键...</kbd>
                </template>
                <template v-else>
                  <kbd v-for="key in item.keys" :key="key">{{ key }}</kbd>
                </template>
              </div>
            </div>

            <!-- 固定快捷键 -->
            <div v-for="s in fixedShortcuts" :key="s.action" class="shortcut-item shortcut-item--fixed">
              <div class="shortcut-action">{{ s.action }}</div>
              <div class="shortcut-keys">
                <kbd v-for="key in s.keys" :key="key">{{ key }}</kbd>
              </div>
            </div>
          </div>
        </div>
```

- [ ] **Step 3: 更新 loadSettings 函数**

在 `loadSettings` 函数末尾追加：

```js
await loadShortcuts()
```

- [ ] **Step 4: 添加录制相关 CSS**

在 `<style scoped>` 中追加：

```css
/* 可自定义快捷键 */
.shortcut-keys--editable {
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 6px;
  transition: background 0.15s;
}

.shortcut-keys--editable:hover {
  background: #ecf5ff;
}

.shortcut-item--recording {
  border-color: #409eff !important;
  background: #fafcff;
}

.shortcut-item--fixed .shortcut-keys kbd {
  opacity: 0.7;
}

.recording-hint {
  color: #409eff;
  font-style: italic;
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
  animation: blink 1.2s ease-in-out infinite;
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/SettingsPanel.vue
git commit -m "feat(shortcuts): add shortcut recording UI in Settings panel

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: FileTreePanel — 右键菜单快捷键提示

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue`

- [ ] **Step 1: 在右键菜单项中追加快捷键提示**

在右键菜单中，对「刷新」「剪切」「复制」「粘贴」菜单项追加快捷键提示。

找到「刷新」菜单项（约 212-213 行）：

```html
        <li class="context-menu-item" @click="onMenuCommand('refresh')">
          <el-icon><Refresh /></el-icon>刷新
        </li>
```

替换为：

```html
        <li class="context-menu-item" @click="onMenuCommand('refresh')">
          <el-icon><Refresh /></el-icon>刷新
          <span class="context-menu-shortcut">F5</span>
        </li>
```

找到「剪切」菜单项：

```html
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
        </li>
```

替换为：

```html
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
          <span class="context-menu-shortcut">Ctrl+X</span>
        </li>
```

找到「复制」菜单项：

```html
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
        </li>
```

替换为：

```html
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
          <span class="context-menu-shortcut">Ctrl+C</span>
        </li>
```

找到「粘贴」菜单项（有两处，目录和文件各一处）：

```html
        <li class="context-menu-item" :class="{ 'is-disabled': !clipboard.mode }" @click="clipboard.mode && onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
        </li>
```

两处都替换为：

```html
        <li class="context-menu-item" :class="{ 'is-disabled': !clipboard.mode }" @click="clipboard.mode && onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
          <span class="context-menu-shortcut">Ctrl+V</span>
        </li>
```

- [ ] **Step 2: 添加快捷键提示样式**

在 `<style scoped>` 中追加：

```css
.context-menu-shortcut {
  margin-left: auto;
  padding-left: 24px;
  font-size: 11px;
  color: #adb2b8;
  font-family: 'Consolas', 'Monaco', monospace;
  white-space: nowrap;
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat(shortcuts): show keyboard shortcut hints in context menu

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 6: 集成验证

**Files:** 无新增

- [ ] **Step 1: 运行后端测试**

Run: `go test ./...`
Expected: 全部 PASS

- [ ] **Step 2: 启动应用手动验证**

Run: `wails dev`

验证清单：
1. 设置 → 快捷键 tab → 显示 6 个快捷键，其中前 2 个可点击
2. 点击「打开命令面板」的快捷键区域 → 显示"请按下新快捷键..."
3. 按下 `Ctrl+K` → 快捷键更新为 Ctrl+K
4. 按 `Ctrl+K` → 命令面板打开
5. 设置 → 快捷键 tab → 点击「切换终端面板」→ 按 `Ctrl+T` → 更新成功
6. 按 `Ctrl+T` → 终端面板切换
7. 录制时按 Escape → 取消录制
8. 右键菜单 → 「刷新」后显示 `F5`，「剪切」后显示 `Ctrl+X` 等
9. 重启应用 → 自定义快捷键持久化

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "feat(shortcuts): custom keyboard shortcuts - complete integration

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```
