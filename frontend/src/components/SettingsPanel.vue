<template>
  <div class="settings-panel">
    <div class="settings-header">
      <span class="settings-title"><el-icon :size="18" style="margin-right:4px;vertical-align:middle;"><Setting /></el-icon>设置</span>
      <span class="settings-close" @click="$emit('close')">&#10005;</span>
    </div>
    <div class="settings-content">
      <div class="settings-section">
        <div class="settings-section-title">渲染</div>
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
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Setting, WarningFilled } from '@element-plus/icons-vue'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'

defineEmits(['close'])

const gpuEnabled = ref(true)
const needsRestart = ref(false)

onMounted(async () => {
  try {
    const settings = await GetSettings()
    gpuEnabled.value = !settings.gpuDisabled
  } catch {
    gpuEnabled.value = true
  }
})

const onGpuChange = async (val) => {
  try {
    await SaveSettings({ gpuDisabled: !val })
    needsRestart.value = true
  } catch {
    // 回滚
    gpuEnabled.value = !gpuEnabled.value
  }
}
</script>

<style scoped>
.settings-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  background-color: var(--bg-primary);
}

.settings-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

.settings-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.5px;
}

.settings-close {
  font-size: 16px;
  color: var(--text-tertiary);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  transition: all var(--transition-normal);
}

.settings-close:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

.settings-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-sm);
}

.settings-section {
  margin-bottom: 16px;
}

.settings-section-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 4px 8px 8px;
}

.settings-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-color);
  transition: all var(--transition-normal);
}

.settings-item:hover {
  border-color: var(--primary-light);
}

.settings-item-info {
  flex: 1;
  min-width: 0;
}

.settings-item-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
}

.settings-item-desc {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 2px;
}

.settings-restart-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  padding: 8px 12px;
  background: rgba(230, 162, 60, 0.1);
  border: 1px solid rgba(230, 162, 60, 0.3);
  border-radius: var(--radius-md);
  color: #e6a23c;
  font-size: 12px;
}
</style>
