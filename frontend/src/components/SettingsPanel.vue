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
        <!-- 快捷键页 -->
        <div v-show="activeTab === 'shortcuts'">
          <div class="settings-section-title">快捷键</div>
          <div class="settings-empty">
            <el-icon :size="32" color="#555"><Key /></el-icon>
            <p>暂无可配置快捷键</p>
          </div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, watch, onMounted } from 'vue'
import { WarningFilled, Key } from '@element-plus/icons-vue'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'

const props = defineProps({
  visible: { type: Boolean, default: false }
})

defineEmits(['update:visible'])

const tabs = [
  { id: 'general', label: '通用' },
  { id: 'terminal', label: '终端' },
  { id: 'shortcuts', label: '快捷键' }
]

const activeTab = ref('general')
const gpuEnabled = ref(true)
const needsRestart = ref(false)
const defaultShell = ref('powershell')
const gitBashPath = ref('C:\\Program Files\\Git\\bin\\bash.exe')
const wslDistro = ref('')

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
  } catch {
    gpuEnabled.value = true
  }
}

const onGpuChange = async (val) => {
  try {
    await SaveSettings({
      gpuDisabled: !val,
      defaultShell: defaultShell.value,
      gitBashPath: gitBashPath.value,
      wslDistro: wslDistro.value
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
      wslDistro: wslDistro.value
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
