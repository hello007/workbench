<template>
  <div v-if="visible" class="terminal-panel">
    <!-- 工具栏 -->
    <div class="terminal-toolbar">
      <div class="terminal-toolbar-left">
        <el-select
          v-model="shellType"
          size="small"
          class="shell-select"
          @change="onShellChange"
        >
          <el-option
            v-for="config in shellConfigs"
            :key="config.type"
            :label="config.displayName"
            :value="config.type"
          />
        </el-select>
        <span class="terminal-dir" :title="currentDir">{{ currentDir }}</span>
      </div>
      <div class="terminal-toolbar-right">
        <el-button
          v-if="isExited"
          size="small"
          type="primary"
          text
          @click="onRestart"
        >
          重新启动
        </el-button>
        <span class="toolbar-btn" @click="$emit('toggle')">─</span>
      </div>
    </div>
    <!-- 终端区域 -->
    <div ref="terminalContainer" class="terminal-container"></div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
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

// 监听 visible 和 settingsReady 变化，首次打开时初始化终端
// 使用 flush: 'post' 确保 DOM 更新后再执行，避免 v-if 导致 terminalContainer 为 null
// 等待 settingsReady 确保默认 Shell 设置已加载
watch(
  [() => props.visible, settingsReady],
  async ([val, ready]) => {
    if (val && ready && !hasInitialized.value && terminalContainer.value) {
      await nextTick()
      const dir = props.currentDir || 'C:\\'
      await initTerminal(terminalContainer.value, dir, shellType.value)
      hasInitialized.value = true
    }
    // 展开时重新调整大小
    if (val && isActive.value) {
      await nextTick()
      resize()
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
})

onBeforeUnmount(async () => {
  if (resizeObserver) {
    resizeObserver.disconnect()
  }
  await destroyTerminal()
})
</script>

<style scoped>
.terminal-panel {
  display: flex;
  flex-direction: column;
  background-color: #1e1e1e;
  border-top: 1px solid var(--border-color, #3c3c3c);
  height: 100%;
}

.terminal-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  padding: 0 8px;
  background: #252526;
  border-bottom: 1px solid #3c3c3c;
}

.terminal-toolbar-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}

.shell-select {
  width: 130px;
}

.shell-select :deep(.el-input__wrapper) {
  background: #3c3c3c;
  box-shadow: none;
  border: 1px solid #4c4c4c;
}

.shell-select :deep(.el-input__inner) {
  color: #d4d4d4;
  font-size: 12px;
}

.terminal-dir {
  font-size: 12px;
  color: #888;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.terminal-toolbar-right {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.toolbar-btn {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #888;
  cursor: pointer;
  border-radius: 4px;
  font-size: 14px;
  transition: all 0.15s;
}

.toolbar-btn:hover {
  background: #3c3c3c;
  color: #d4d4d4;
}

.terminal-container {
  flex: 1;
  min-height: 0;
  padding: 4px 8px;
}

.terminal-container :deep(.xterm) {
  height: 100%;
}

.terminal-container :deep(.xterm-viewport) {
  overflow-y: auto !important;
}
</style>
