<template>
  <div v-show="visible" class="terminal-panel">
    <!-- 工具栏 -->
    <div class="terminal-toolbar">
      <div class="terminal-toolbar-left">
        <div class="shell-badge">
          <span class="shell-dot"></span>
          <el-select
            v-model="shellType"
            size="small"
            class="shell-select"
            popper-class="shell-select-popper"
            @change="onShellChange"
          >
            <el-option
              v-for="config in shellConfigs"
              :key="config.type"
              :label="config.displayName"
              :value="config.type"
            />
          </el-select>
        </div>
        <div class="terminal-path">
          <el-icon :size="12" class="path-icon"><Folder /></el-icon>
          <span class="path-text" :title="currentDir">{{ currentDir }}</span>
        </div>
      </div>
      <div class="terminal-toolbar-right">
        <transition name="fade">
          <el-button
            v-if="isExited"
            size="small"
            type="primary"
            text
            class="restart-btn"
            @click="onRestart"
          >
            <el-icon :size="14"><RefreshRight /></el-icon>
            重新启动
          </el-button>
        </transition>
        <div class="toolbar-actions">
          <span class="toolbar-btn minimize-btn" @click="$emit('toggle')" title="收起终端">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
              <rect x="2" y="7" width="10" height="1.5" rx="0.75" fill="currentColor"/>
            </svg>
          </span>
        </div>
      </div>
    </div>
    <!-- 终端区域 -->
    <div ref="terminalContainer" class="terminal-container"></div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Folder, RefreshRight } from '@element-plus/icons-vue'
import { useTerminal } from '../composables/useTerminal'
import { GetShellConfigs, GetSettings } from '../../wailsjs/go/main/App'

const props = defineProps({
  visible: { type: Boolean, default: false },
  currentDir: { type: String, default: '' }
})

defineEmits(['toggle'])

const {
  isActive,
  isExited,
  currentDir: terminalDir,
  initTerminal,
  changeDir,
  resize,
  focus,
  destroyTerminal,
  restartTerminal
} = useTerminal()

const terminalContainer = ref(null)
const shellType = ref('powershell')
const shellConfigs = ref([])
const hasInitialized = ref(false)
const settingsReady = ref(false)

// 加载 Shell 配置和默认 Shell 设置
onMounted(async () => {
  try {
    shellConfigs.value = await GetShellConfigs()
  } catch {
    shellConfigs.value = [
      { type: 'powershell', displayName: 'PowerShell' },
      { type: 'cmd', displayName: 'CMD' },
      { type: 'gitbash', displayName: 'Git Bash' },
      { type: 'wsl', displayName: 'WSL' }
    ]
  }
  // 读取用户设置的默认 Shell 类型
  try {
    const settings = await GetSettings()
    if (settings.defaultShell) {
      shellType.value = settings.defaultShell
    }
  } catch {
    // 读取失败则保持默认 powershell
  }
  settingsReady.value = true
})

// 监听 visible 和 settingsReady 变化，首次可见时初始化终端
// 使用 v-show 保留 DOM，避免收起再展开时 xterm 丢失挂载点
// 首次初始化需等待 visible=true（xterm 在 display:none 下无法正确 fit）
watch(
  [() => props.visible, settingsReady],
  async ([val, ready]) => {
    // 首次可见时初始化终端
    if (val && ready && !hasInitialized.value && terminalContainer.value) {
      await nextTick()
      const dir = props.currentDir || 'C:\\'
      await initTerminal(terminalContainer.value, dir, shellType.value)
      hasInitialized.value = true
      focus()
    }
    // 展开时重新调整大小并聚焦（v-show 隐藏后尺寸变化，需重新 fit）
    if (val && isActive.value) {
      await nextTick()
      resize()
      focus()
    }
  },
  { flush: 'post' }
)

// 监听目录变化，自动跟随
watch(() => props.currentDir, (newDir) => {
  if (newDir && isActive.value) {
    changeDir(newDir)
  }
})

// Shell 类型切换
async function onShellChange(newType) {
  if (!terminalContainer.value) return
  const dir = terminalDir.value || props.currentDir || 'C:\\'
  await restartTerminal(terminalContainer.value, dir, newType)
}

// 重新启动
async function onRestart() {
  if (!terminalContainer.value) return
  const dir = terminalDir.value || props.currentDir || 'C:\\'
  await restartTerminal(terminalContainer.value, dir, shellType.value)
}

// 窗口 resize 监听
let resizeObserver = null

onMounted(() => {
  resizeObserver = new ResizeObserver(() => {
    if (props.visible && isActive.value) {
      resize()
    }
  })
  // 观察 xterm 的直接父容器，检测拖拽调整高度等尺寸变化
  if (terminalContainer.value) {
    resizeObserver.observe(terminalContainer.value)
  }
})

onBeforeUnmount(async () => {
  if (resizeObserver) {
    resizeObserver.disconnect()
  }
  await destroyTerminal()
})
</script>

<style scoped>
/* ── 面板容器 ── */
.terminal-panel {
  display: flex;
  flex-direction: column;
  background-color: #f5f7fa;
  border-top: 1px solid var(--border-color, #ebeef5);
  height: 100%;
  position: relative;
}

/* 顶部光晕装饰线 */
.terminal-panel::before {
  content: '';
  position: absolute;
  top: 0;
  left: 10%;
  right: 10%;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(64, 158, 255, 0.2), transparent);
}

/* ── 工具栏 ── */
.terminal-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 36px;
  padding: 0 12px;
  background: linear-gradient(180deg, #ffffff 0%, #f9fafb 100%);
  border-bottom: 1px solid var(--border-color, #ebeef5);
}

.terminal-toolbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1;
}

/* Shell 徽章 */
.shell-badge {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 4px;
}

.shell-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #67c23a;
  box-shadow: 0 0 6px rgba(103, 194, 58, 0.4);
  animation: pulse 2.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

/* Shell 下拉框 */
.shell-select {
  width: 120px;
}

.shell-select :deep(.el-input__wrapper) {
  background: #ffffff;
  box-shadow: none;
  border: 1px solid var(--border-light, #dcdfe6);
  border-radius: 6px;
  transition: all 0.2s ease;
}

.shell-select :deep(.el-input__wrapper:hover) {
  border-color: var(--primary-color, #409eff);
}

.shell-select :deep(.el-input__inner) {
  color: var(--text-primary, #303133);
  font-size: 12px;
  font-weight: 500;
}

.shell-select :deep(.el-input__suffix) {
  color: var(--text-tertiary, #909399);
}

/* 路径指示 */
.terminal-path {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 3px 10px;
  background: #ffffff;
  border-radius: 4px;
  border: 1px solid var(--border-color, #ebeef5);
  min-width: 0;
}

.path-icon {
  color: #409eff;
  flex-shrink: 0;
}

.path-text {
  font-size: 12px;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  color: var(--text-secondary, #606266);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  letter-spacing: 0.3px;
}

/* 右侧操作区 */
.terminal-toolbar-right {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.restart-btn {
  color: var(--warning-color, #e6a23c) !important;
  font-size: 12px;
}

.restart-btn:hover {
  color: #e6a23c !important;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.toolbar-btn {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary, #909399);
  cursor: pointer;
  border-radius: 6px;
  transition: all 0.2s ease;
}

.toolbar-btn:hover {
  background: var(--bg-tertiary, #f0f2f5);
  color: var(--text-primary, #303133);
}

.toolbar-btn:active {
  transform: scale(0.92);
}

/* 淡入淡出 */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* ── 终端内容区 ── */
.terminal-container {
  flex: 1;
  min-height: 0;
  padding: 2px 0 0;
  background: #ffffff;
}

.terminal-container :deep(.xterm) {
  height: 100%;
  padding: 0 4px;
}

.terminal-container :deep(.xterm-viewport) {
  background-color: #ffffff !important;
  overflow-y: auto !important;
}

/* xterm 滚动条 */
.terminal-container :deep(.xterm-viewport)::-webkit-scrollbar {
  width: 6px;
}

.terminal-container :deep(.xterm-viewport)::-webkit-scrollbar-track {
  background: transparent;
}

.terminal-container :deep(.xterm-viewport)::-webkit-scrollbar-thumb {
  background: var(--border-light, #dcdfe6);
  border-radius: 3px;
}

.terminal-container :deep(.xterm-viewport)::-webkit-scrollbar-thumb:hover {
  background: var(--text-tertiary, #909399);
}
</style>

<style>
/* Shell 下拉弹出框浅色主题 */
.shell-select-popper {
  background: #ffffff !important;
  border: 1px solid var(--border-color, #ebeef5) !important;
  border-radius: 8px !important;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.1) !important;
}

.shell-select-popper .el-select-dropdown__item {
  color: var(--text-primary, #303133) !important;
  font-size: 13px;
  font-weight: 500;
  padding: 0 16px;
  height: 34px;
  line-height: 34px;
  border-radius: 4px;
  margin: 2px 4px;
  width: calc(100% - 8px);
}

.shell-select-popper .el-select-dropdown__item:hover,
.shell-select-popper .el-select-dropdown__item.hover {
  background: var(--primary-bg, #ecf5ff) !important;
  color: var(--primary-color, #409eff) !important;
}

.shell-select-popper .el-select-dropdown__item.is-selected {
  color: var(--primary-color, #409eff) !important;
  font-weight: 600;
}

.shell-select-popper .el-popper__arrow::before {
  background: #ffffff !important;
  border-color: var(--border-color, #ebeef5) !important;
}
</style>
