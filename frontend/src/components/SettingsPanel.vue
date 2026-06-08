<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="设置"
    width="760px"
    :close-on-click-modal="true"
    :close-on-press-escape="true"
    class="settings-dialog"
    append-to-body
  >
    <div class="settings-body">
      <!-- 左侧导航栏 -->
      <div class="settings-nav">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          class="settings-nav-item"
          :class="{ 'is-active': activeTab === tab.id }"
          @click="activeTab = tab.id"
        >
          {{ tab.label }}
        </div>
      </div>
      <!-- 右侧内容区 -->
      <div class="settings-content">
        <!-- 通用页 -->
        <div v-show="activeTab === 'general'">
          <div class="settings-section-title">通用</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">GPU 加速</div>
              <div class="settings-item-desc">使用 GPU 渲染 WebView 界面，关闭后可降低 GPU 占用</div>
            </div>
            <el-switch
              v-model="gpuEnabled"
              active-text="开启"
              inactive-text="关闭"
              @change="onGpuChange"
            />
          </div>
          <div v-if="needsRestart" class="settings-restart-hint">
            <el-icon :size="14"><WarningFilled /></el-icon>
            <span>GPU 设置已变更，需重启应用后生效</span>
          </div>
        </div>
        <!-- 终端页 -->
        <div v-show="activeTab === 'terminal'">
          <div class="settings-section-title">终端</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">默认 Shell</div>
              <div class="settings-item-desc">终端面板使用的 Shell 类型</div>
            </div>
            <el-select v-model="defaultShell" size="small" style="width: 140px;" @change="onSettingsChange">
              <el-option label="PowerShell" value="powershell" />
              <el-option label="CMD" value="cmd" />
              <el-option label="Git Bash" value="gitbash" />
              <el-option label="WSL" value="wsl" />
            </el-select>
          </div>
          <div v-if="defaultShell === 'gitbash'" class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">Git Bash 路径</div>
              <div class="settings-item-desc">自定义 Git Bash 可执行文件路径</div>
            </div>
            <el-input v-model="gitBashPath" size="small" style="width: 240px;" @change="onSettingsChange" />
          </div>
          <div v-if="defaultShell === 'wsl'" class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">WSL 发行版</div>
              <div class="settings-item-desc">指定 WSL 发行版名称（留空使用默认）</div>
            </div>
            <el-input v-model="wslDistro" size="small" style="width: 240px;" @change="onSettingsChange" />
          </div>
        </div>
        <!-- 搜索页 -->
        <div v-show="activeTab === 'search'">
          <div class="settings-section-title">搜索</div>
          <div class="settings-item settings-item--column">
            <div class="settings-item-info">
              <div class="settings-item-label">排除目录</div>
              <div class="settings-item-desc">搜索时跳过这些目录</div>
            </div>
            <div class="settings-tags">
              <el-tag
                v-for="dir in excludeDirs"
                :key="dir"
                closable
                size="small"
                @close="removeExcludeDir(dir)"
              >{{ dir }}</el-tag>
              <el-input
                v-model="newExcludeDir"
                size="small"
                style="width: 120px;"
                placeholder="添加目录"
                @keyup.enter="addExcludeDir"
              />
              <el-button size="small" @click="addExcludeDir">添加</el-button>
            </div>
          </div>
          <div class="settings-item settings-item--column">
            <div class="settings-item-info">
              <div class="settings-item-label">排除文件</div>
              <div class="settings-item-desc">搜索时跳过这些扩展名的文件</div>
            </div>
            <div class="settings-tags">
              <el-tag
                v-for="file in excludeFiles"
                :key="file"
                closable
                size="small"
                @close="removeExcludeFile(file)"
              >{{ file }}</el-tag>
              <el-input
                v-model="newExcludeFile"
                size="small"
                style="width: 120px;"
                placeholder="如 .log"
                @keyup.enter="addExcludeFile"
              />
              <el-button size="small" @click="addExcludeFile">添加</el-button>
            </div>
          </div>
        </div>
        <!-- 快捷键页 -->
        <div v-show="activeTab === 'shortcuts'" ref="shortcutsTabRef" tabindex="-1" @keydown="handleRecordingKeydown">
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
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { WarningFilled, Key } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'
import { useShortcuts } from '../composables/useShortcuts'

const props = defineProps({
  visible: { type: Boolean, default: false }
})

defineEmits(['update:visible'])

const tabs = [
  { id: 'general', label: '通用' },
  { id: 'terminal', label: '终端' },
  { id: 'search', label: '搜索' },
  { id: 'shortcuts', label: '快捷键' }
]

const activeTab = ref('general')
const gpuEnabled = ref(true)
const needsRestart = ref(false)
const defaultShell = ref('powershell')
const gitBashPath = ref('C:\\Program Files\\Git\\bin\\bash.exe')
const wslDistro = ref('')
const excludeDirs = ref([])
const excludeFiles = ref([])
const newExcludeDir = ref('')
const newExcludeFile = ref('')

const { shortcutCommandPalette, shortcutToggleTerminal, formatDisplay, isValidShortcut, shortcutFromEvent, checkConflict, loadShortcuts, saveShortcuts } = useShortcuts()

const shortcutsTabRef = ref(null)
const recordingKey = ref(null)
const recordingText = ref('')

const fixedShortcuts = [
  { action: '刷新当前节点', keys: ['F5'] },
  { action: '复制选中项', keys: ['Ctrl', 'C'] },
  { action: '剪切选中项', keys: ['Ctrl', 'X'] },
  { action: '粘贴', keys: ['Ctrl', 'V'] }
]

const customizableShortcuts = computed(() => [
  { action: '打开命令面板', key: 'commandPalette', keys: formatDisplay(shortcutCommandPalette.value), customizable: true },
  { action: '切换终端面板', key: 'toggleTerminal', keys: formatDisplay(shortcutToggleTerminal.value), customizable: true }
])

function startRecording(key) {
  recordingKey.value = key
  recordingText.value = ''
  nextTick(() => {
    shortcutsTabRef.value?.focus()
  })
}

function cancelRecording() {
  recordingKey.value = null
  recordingText.value = ''
}

function handleRecordingKeydown(e) {
  if (!recordingKey.value) return

  // 录制模式下拦截所有按键，阻止冒泡到 Home.vue 的全局处理器
  e.preventDefault()
  e.stopPropagation()

  if (e.key === 'Escape') {
    cancelRecording()
    return true
  }

  const shortcut = shortcutFromEvent(e)
  if (!shortcut || !isValidShortcut(shortcut)) return true

  const conflict = checkConflict(shortcut, recordingKey.value)
  if (conflict) {
    ElMessage.warning(`快捷键冲突：与"${conflict.key === 'commandPalette' ? '打开命令面板' : '切换终端面板'}"相同`)
    return true
  }

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

// 弹窗打开时加载设置
watch(() => props.visible, async (val) => {
  if (val) {
    await loadSettings()
  }
})

onMounted(async () => {
  await loadSettings()
})

async function loadSettings() {
  try {
    const settings = await GetSettings()
    gpuEnabled.value = !settings.gpuDisabled
    defaultShell.value = settings.defaultShell || 'powershell'
    gitBashPath.value = settings.gitBashPath || 'C:\\Program Files\\Git\\bin\\bash.exe'
    wslDistro.value = settings.wslDistro || ''
    excludeDirs.value = settings.searchExcludeDirs || []
    excludeFiles.value = settings.searchExcludeFiles || []
    await loadShortcuts()
  } catch {
    gpuEnabled.value = true
  }
}

const addExcludeDir = () => {
  const val = newExcludeDir.value.trim()
  if (val && !excludeDirs.value.includes(val)) {
    excludeDirs.value.push(val)
    onSettingsChange()
  }
  newExcludeDir.value = ''
}

const removeExcludeDir = (tag) => {
  excludeDirs.value = excludeDirs.value.filter(d => d !== tag)
  onSettingsChange()
}

const addExcludeFile = () => {
  const val = newExcludeFile.value.trim()
  if (val && !excludeFiles.value.includes(val)) {
    excludeFiles.value.push(val)
    onSettingsChange()
  }
  newExcludeFile.value = ''
}

const removeExcludeFile = (tag) => {
  excludeFiles.value = excludeFiles.value.filter(f => f !== tag)
  onSettingsChange()
}

const onGpuChange = async (val) => {
  try {
    await SaveSettings({
      gpuDisabled: !val,
      defaultShell: defaultShell.value,
      gitBashPath: gitBashPath.value,
      wslDistro: wslDistro.value,
      searchExcludeDirs: excludeDirs.value,
      searchExcludeFiles: excludeFiles.value
    })
    needsRestart.value = true
  } catch {
    gpuEnabled.value = !gpuEnabled.value
  }
}

const onSettingsChange = async () => {
  try {
    await SaveSettings({
      gpuDisabled: !gpuEnabled.value,
      defaultShell: defaultShell.value,
      gitBashPath: gitBashPath.value,
      wslDistro: wslDistro.value,
      searchExcludeDirs: excludeDirs.value,
      searchExcludeFiles: excludeFiles.value
    })
  } catch {
    // 回滚
  }
}
</script>

<style scoped>
/* 弹窗内容区背景 */
.settings-body {
  display: flex;
  height: 420px;
  margin: -20px;  /* 抵消 el-dialog 默认 padding */
}

/* 左侧导航栏 */
.settings-nav {
  width: 200px;
  flex-shrink: 0;
  background: #ffffff;
  border-right: 1px solid var(--border-color, #ebeef5);
  padding: 12px 0;
}

.settings-nav-item {
  padding: 10px 20px;
  font-size: 14px;
  color: var(--text-secondary, #606266);
  cursor: pointer;
  border-left: 2px solid transparent;
  transition: all 0.15s;
}

.settings-nav-item:hover {
  background: var(--bg-tertiary, #f0f2f5);
  color: var(--text-primary, #303133);
}

.settings-nav-item.is-active {
  color: var(--primary-color, #409eff);
  background: var(--primary-bg, #ecf5ff);
  border-left-color: var(--primary-color, #409eff);
}

/* 右侧内容区 */
.settings-content {
  flex: 1;
  padding: 20px 28px;
  overflow-y: auto;
  background: #ffffff;
}

.settings-section-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary, #303133);
  margin-bottom: 20px;
}

.settings-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background: var(--bg-secondary, #ffffff);
  border-radius: 8px;
  border: 1px solid var(--border-color, #ebeef5);
  margin-bottom: 12px;
  transition: border-color 0.15s;
}

.settings-item:hover {
  border-color: var(--primary-light, #66b1ff);
}

.settings-item-info {
  flex: 1;
  min-width: 0;
  margin-right: 16px;
}

.settings-item-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary, #303133);
}

.settings-item-desc {
  font-size: 12px;
  color: var(--text-secondary, #606266);
  margin-top: 4px;
}

.settings-restart-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  padding: 8px 12px;
  background: rgba(230, 162, 60, 0.1);
  border: 1px solid rgba(230, 162, 60, 0.3);
  border-radius: 8px;
  color: #e6a23c;
  font-size: 12px;
}

/* 快捷键空状态 */
.settings-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: var(--text-tertiary, #909399);
}

.settings-empty p {
  margin-top: 12px;
  font-size: 14px;
}

.settings-item--column {
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
}

.settings-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  width: 100%;
}

/* 快捷键列表 */
.shortcut-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.shortcut-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-radius: 8px;
  border: 1px solid var(--border-color, #ebeef5);
  transition: border-color 0.15s;
}

.shortcut-item:hover {
  border-color: var(--primary-light, #66b1ff);
}

.shortcut-action {
  font-size: 14px;
  color: var(--text-primary, #303133);
}

.shortcut-keys {
  display: flex;
  gap: 4px;
}

.shortcut-keys kbd {
  display: inline-block;
  padding: 3px 8px;
  font-size: 12px;
  font-family: 'Consolas', 'Monaco', monospace;
  color: var(--text-primary, #303133);
  background: var(--bg-tertiary, #f0f2f5);
  border: 1px solid var(--border-color, #dcdfe6);
  border-radius: 4px;
  box-shadow: 0 1px 0 var(--border-color, #dcdfe6);
}

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
</style>

<style>
/* 全局：el-dialog 浅色主题覆盖 */
.settings-dialog .el-dialog {
  background: #ffffff;
  border: 1px solid var(--border-color, #ebeef5);
  border-radius: 8px;
}

.settings-dialog .el-dialog__header {
  background: #ffffff;
  border-bottom: 1px solid var(--border-color, #ebeef5);
  border-radius: 8px 8px 0 0;
  padding: 14px 20px;
}

.settings-dialog .el-dialog__title {
  color: var(--text-primary, #303133);
  font-size: 16px;
  font-weight: 600;
}

.settings-dialog .el-dialog__headerbtn .el-dialog__close {
  color: var(--text-tertiary, #909399);
}

.settings-dialog .el-dialog__headerbtn:hover .el-dialog__close {
  color: var(--text-primary, #303133);
}

.settings-dialog .el-dialog__body {
  padding: 20px;
}

.settings-dialog .el-overlay {
  background-color: rgba(0, 0, 0, 0.5);
}
</style>
