<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="发现新版本"
    width="480px"
    :close-on-click-modal="false"
    :close-on-press-escape="!downloading"
    class="update-dialog"
    append-to-body
  >
    <!-- 新版本信息 -->
    <div v-if="!downloading && !downloaded" class="update-info">
      <div class="update-version">
        <span class="version-label">新版本</span>
        <span class="version-number">v{{ updateInfo.latestVer }}</span>
      </div>
      <div class="update-current">
        当前版本：v{{ updateInfo.currentVer }}
      </div>
      <div v-if="updateInfo.releaseNotes" class="update-notes">
        <div class="update-notes-title">更新内容</div>
        <div class="update-notes-body">{{ updateInfo.releaseNotes }}</div>
      </div>
      <div class="update-meta">
        <span v-if="updateInfo.fileSize">文件大小：{{ formatSize(updateInfo.fileSize) }}</span>
      </div>
    </div>

    <!-- 下载进度 -->
    <div v-if="downloading" class="update-downloading">
      <div class="download-title">正在下载更新...</div>
      <el-progress
        :percentage="Math.round(progress.percent)"
        :stroke-width="12"
        :format="() => `${Math.round(progress.percent)}%`"
      />
      <div class="download-detail">
        <span>{{ formatSize(progress.downloaded) }} / {{ formatSize(progress.totalBytes) }}</span>
        <span v-if="progress.speed" class="download-speed">{{ progress.speed }}</span>
      </div>
    </div>

    <!-- 下载完成 -->
    <div v-if="downloaded" class="update-done">
      <el-icon :size="40" color="#67c23a"><CircleCheckFilled /></el-icon>
      <div class="done-text">更新已下载完成</div>
      <div class="done-hint">需要重启应用以完成更新，是否立即重启？</div>
    </div>

    <template #footer>
      <div class="update-footer">
        <!-- 新版本信息状态 -->
        <template v-if="!downloading && !downloaded">
          <el-button @click="handleClose">稍后再说</el-button>
          <el-button type="primary" @click="handleStartDownload">立即更新</el-button>
        </template>

        <!-- 下载中状态 -->
        <template v-if="downloading">
          <el-button @click="handleCancelDownload">取消下载</el-button>
        </template>

        <!-- 下载完成状态 -->
        <template v-if="downloaded">
          <el-button @click="handleRestartLater">稍后重启</el-button>
          <el-button type="primary" @click="handleRestartNow">立即重启</el-button>
        </template>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { CircleCheckFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { DownloadUpdate, CancelDownload, ApplyUpdate } from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const props = defineProps({
  visible: { type: Boolean, default: false },
  updateInfo: { type: Object, default: () => ({}) }
})

const emit = defineEmits(['update:visible'])

const downloading = ref(false)
const downloaded = ref(false)

// 弹窗打开时重置状态
watch(() => props.visible, (val) => {
  if (val) {
    downloading.value = false
    downloaded.value = false
    progress.value = { totalBytes: 0, downloaded: 0, percent: 0, speed: '', completed: false }
  }
})
const progress = ref({
  totalBytes: 0,
  downloaded: 0,
  percent: 0,
  speed: '',
  completed: false
})

onMounted(() => {
  // 监听下载进度事件
  EventsOn('update:download-progress', (data) => {
    progress.value = data
    if (data.completed) {
      downloading.value = false
      downloaded.value = true
    }
  })
})

onBeforeUnmount(() => {
  EventsOff('update:download-progress')
})

async function handleStartDownload() {
  if (!props.updateInfo.downloadUrl) {
    ElMessage.error('下载地址无效')
    return
  }
  downloading.value = true
  downloaded.value = false
  progress.value = { totalBytes: 0, downloaded: 0, percent: 0, speed: '', completed: false }

  try {
    await DownloadUpdate(props.updateInfo.downloadUrl)
  } catch (e) {
    downloading.value = false
    ElMessage.error('下载失败: ' + (e.message || String(e)))
  }
}

function handleCancelDownload() {
  CancelDownload()
  downloading.value = false
  handleClose()
}

async function handleRestartNow() {
  try {
    await ApplyUpdate()
  } catch (e) {
    ElMessage.error('更新失败: ' + (e.message || String(e)))
  }
}

function handleRestartLater() {
  handleClose()
}

function handleClose() {
  emit('update:visible', false)
  // 重置状态
  if (!downloading.value) {
    downloaded.value = false
  }
}

function formatSize(bytes) {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let size = bytes
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024
    i++
  }
  return `${size.toFixed(1)} ${units[i]}`
}
</script>

<style scoped>
.update-info {
  padding: 4px 0;
}

.update-version {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.version-label {
  font-size: 13px;
  color: var(--text-secondary, #606266);
}

.version-number {
  font-size: 20px;
  font-weight: 600;
  color: var(--primary-color, #409eff);
}

.update-current {
  font-size: 13px;
  color: var(--text-secondary, #606266);
  margin-bottom: 16px;
}

.update-notes {
  margin-bottom: 16px;
}

.update-notes-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary, #303133);
  margin-bottom: 8px;
}

.update-notes-body {
  font-size: 13px;
  color: var(--text-secondary, #606266);
  line-height: 1.6;
  max-height: 200px;
  overflow-y: auto;
  padding: 12px;
  background: var(--bg-tertiary, #f5f7fa);
  border-radius: 6px;
  white-space: pre-wrap;
  word-break: break-word;
}

.update-meta {
  font-size: 12px;
  color: var(--text-tertiary, #909399);
}

.update-downloading {
  padding: 16px 0;
}

.download-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary, #303133);
  margin-bottom: 16px;
}

.download-detail {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-secondary, #606266);
}

.download-speed {
  color: var(--primary-color, #409eff);
}

.update-done {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 24px 0;
}

.done-text {
  font-size: 16px;
  font-weight: 500;
  color: var(--text-primary, #303133);
  margin-top: 12px;
}

.done-hint {
  font-size: 13px;
  color: var(--text-secondary, #606266);
  margin-top: 8px;
}

.update-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
