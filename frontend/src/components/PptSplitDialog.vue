<template>
  <el-dialog
    :model-value="modelValue"
    width="min(640px, 80vw)"
    class="ppt-split-dialog"
    append-to-body
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <template #header>
      <span class="ps-title">PPT 按页拆分</span>
    </template>

    <!-- 源文件路径：手动输入 + 浏览按钮（桌面端原生对话框桥接） -->
    <div class="ps-field">
      <span class="ps-field-label">PPT 路径</span>
      <div class="ps-path-row">
        <el-input
          v-model="srcPath"
          placeholder="输入或浏览选择 .pptx 文件路径"
          :disabled="splitting"
          @keyup.enter="startSplit"
        />
        <el-button :disabled="splitting" @click="browseFile">浏览</el-button>
      </div>
    </div>

    <div class="ps-hint">
      输出到 PPT 同级「原文件名」文件夹，每页一个独立 PPT（命名如 汇报-01.pptx）；
      已存在的同名输出文件将被覆盖。仅支持 .pptx 格式。
    </div>

    <!-- 进度态：监听 pptx-split:progress 事件按 srcPath 过滤更新 -->
    <div v-if="splitting" class="ps-progress">
      {{ progress ? `正在拆分：${progress.done} / ${progress.total} 页` : '正在拆分：准备中…' }}
    </div>

    <!-- 结果展示：输出目录 + 生成文件列表 -->
    <div v-if="result" class="ps-result">
      <div class="ps-result-summary">已拆分 {{ result.pageCount }} 页，输出目录：{{ result.outputDir }}</div>
      <div class="ps-file-list">
        <div v-for="f in result.files" :key="f" class="ps-file-item">{{ f }}</div>
      </div>
    </div>

    <template #footer>
      <el-button v-if="splitting" type="warning" @click="cancelSplit">取消拆分</el-button>
      <el-button :disabled="splitting" @click="$emit('update:modelValue', false)">关闭</el-button>
      <el-button type="primary" :loading="splitting" @click="startSplit">开始拆分</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { SplitPptxPerPage, CancelPptxSplit, OpenFileDialog } from '../../wailsjs/go/main/App'
// 仅引 EventsOn：用其返回的「注销本监听器」闭包在对话框关闭/组件卸载时调用，
// 精准移除本组件的 pptx-split:* 监听器。禁用 EventsOff——Wails v2 EventsOff 按
// eventName 删全部同名监听器，会误删其他监听方（若未来出现共听组件）。
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { handleError } from '../utils/error'

const props = defineProps({
  modelValue: { type: Boolean, default: false }
})
defineEmits(['update:modelValue'])

const srcPath = ref('')
// 是否有本对话框发起的在途拆分
const splitting = ref(false)
// 拆分进度 { done, total }（仅本对话框发起的 srcPath 的 progress 事件写入）
const progress = ref(null)
// 拆分结果 model.PptxSplitResult { outputDir, files, pageCount }
const result = ref(null)
// 本对话框在途拆分的源路径：pptx-split:* 事件载荷 srcPath 的过滤依据，
// 防其他入口（文件树右键等）发起的拆分事件串入本对话框状态
let pendingSrcPath = null

// ---- pptx-split:* 事件监听（对话框打开时注册，关闭/卸载时闭包精准注销）----
let offProgress = null
let offDone = null
let offError = null

// 关闭后后端在途拆分继续跑（不随对话框关闭取消），但前端状态已重置、
// 监听器已注销；重新打开时 pendingSrcPath 为 null，旧拆分事件被 srcPath
// 过滤挡住，旧结果不串入新会话
const resetSplitState = () => {
  splitting.value = false
  progress.value = null
  pendingSrcPath = null
}

const onProgress = (payload) => {
  if (!payload || payload.srcPath !== pendingSrcPath) return
  progress.value = { done: payload.done, total: payload.total }
}

const onDone = (payload) => {
  if (!payload || payload.srcPath !== pendingSrcPath) return
  result.value = payload.result
  resetSplitState()
}

const onError = (payload) => {
  if (!payload || payload.srcPath !== pendingSrcPath) return
  // 错误提示统一由 SplitPptxPerPage 的 Promise reject 走 handleError 分流，
  // 此处仅复位状态，避免同一错误双弹提示
  resetSplitState()
}

const registerListeners = () => {
  offProgress = EventsOn('pptx-split:progress', onProgress)
  offDone = EventsOn('pptx-split:done', onDone)
  offError = EventsOn('pptx-split:error', onError)
}

const unregisterListeners = () => {
  if (offProgress) { offProgress(); offProgress = null }
  if (offDone) { offDone(); offDone = null }
  if (offError) { offError(); offError = null }
}

// 打开注册监听、关闭注销并重置状态。immediate：组件常驻挂载（v-model 控制），
// 首次打开走 change 分支；单测直接以 visible=true 挂载时由 immediate 分支注册
watch(() => props.modelValue, (visible) => {
  if (visible) {
    // 防御性先注销再注册，防重复打开重复注册监听器
    unregisterListeners()
    registerListeners()
  } else {
    unregisterListeners()
    srcPath.value = ''
    result.value = null
    resetSplitState()
  }
}, { immediate: true })

// 拆分完成收尾：done/error 事件先到时状态已复位，finally 兜底幂等复位
const startSplit = async () => {
  const path = srcPath.value.trim()
  if (!path) {
    ElMessage.warning('请输入 PPT 文件路径')
    return
  }
  if (!path.toLowerCase().endsWith('.pptx')) {
    ElMessage.warning('仅支持 .pptx 格式（.ppt 老格式不支持拆分）')
    return
  }
  splitting.value = true
  progress.value = null
  result.value = null
  pendingSrcPath = path
  try {
    const res = await SplitPptxPerPage(path)
    // done 事件可能先于 Promise 到达并已写入结果：已有结果时此处不覆盖
    result.value = res || result.value
  } catch (error) {
    handleError('PPT 拆分失败: ', error)
  } finally {
    resetSplitState()
  }
}

// 取消在途拆分：后端经 pptx-split:error 事件 + Promise reject（E_GIT_CANCELLED，
// handleError 命中 WARNING_CODES 弹中性 warning）双通道收尾
const cancelSplit = async () => {
  try {
    await CancelPptxSplit()
  } catch (error) {
    handleError('取消拆分失败: ', error)
  }
}

// 浏览选择 .pptx 文件：桌面端经后端桥接原生对话框；serve 浏览器通道对话框
// 不可用（后端守卫返回错误），对齐 AiFunctionConfigDialog 先例 catch 静默
// 返回（输入框手动输入路径仍可用）
const browseFile = async () => {
  let path
  try {
    path = await OpenFileDialog('选择 PPT 文件', [{ DisplayName: 'PowerPoint 演示文稿', Pattern: '*.pptx' }])
  } catch {
    return
  }
  if (!path) return
  // OpenFileDialog 可能返回数组或单字符串，统一取首个
  const filePath = Array.isArray(path) ? path[0] : path
  if (filePath) srcPath.value = filePath
}

onBeforeUnmount(() => {
  unregisterListeners()
})
</script>

<style scoped>
.ps-title {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.ps-field {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.ps-field-label {
  font-size: 13px;
  color: var(--text-secondary);
}

.ps-path-row {
  display: flex;
  gap: var(--spacing-sm);
}

.ps-path-row .el-input {
  flex: 1;
}

.ps-hint {
  margin-top: var(--spacing-sm);
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-tertiary);
}

.ps-progress {
  margin-top: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 13px;
  color: var(--primary-color);
  background: var(--primary-bg);
  border-radius: var(--radius-md);
}

.ps-result {
  margin-top: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-md);
}

.ps-result-summary {
  font-size: 13px;
  font-weight: 500;
  color: var(--success-color, var(--el-color-success));
  word-break: break-all;
}

.ps-file-list {
  max-height: 200px;
  overflow-y: auto;
  margin-top: var(--spacing-sm);
}

.ps-file-item {
  padding: 2px 0;
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  font-size: 12px;
  color: var(--text-secondary);
  word-break: break-all;
}
</style>
