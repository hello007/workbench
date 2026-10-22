<template>
  <div class="ps-panel">
    <!-- 模式切换：单文件 / 文件夹批量，进行中禁切（防在途状态跨模式串扰） -->
    <div class="ps-mode-row">
      <el-radio-group v-model="mode" size="small" :disabled="splitting || batchRunning">
        <el-radio-button value="file">单文件拆分</el-radio-button>
        <el-radio-button value="folder">文件夹批量拆分</el-radio-button>
      </el-radio-group>
    </div>

    <!-- ==================== 单文件模式（原弹窗逻辑迁移） ==================== -->
    <template v-if="mode === 'file'">
      <div class="tool-panel-titlebar">
        <span class="tool-panel-title">PPT 按页拆分</span>
        <span class="tool-panel-titlebar-hint">将 .pptx 按页拆分为多个单页 PPT</span>
      </div>

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

      <!-- 结果展示：输出目录 + 生成文件列表（「打开文件夹」经 OpenInExplorer 桥接资源管理器） -->
      <div v-if="result" class="ps-result">
        <div class="ps-result-head">
          <div class="ps-result-summary">已拆分 {{ result.pageCount }} 页，输出目录：{{ result.outputDir }}</div>
          <el-button size="small" @click="openOutputDir(result.outputDir)">打开文件夹</el-button>
        </div>
        <div class="ps-file-list">
          <div v-for="f in result.files" :key="f" class="ps-file-item">{{ f }}</div>
        </div>
      </div>

      <div class="ps-actions">
        <el-button v-if="splitting" type="warning" @click="cancelSplit">取消拆分</el-button>
        <el-button type="primary" :loading="splitting" @click="startSplit">开始拆分</el-button>
      </div>
    </template>

    <!-- ==================== 文件夹批量模式（递归扫描 → 串行拆分 → 汇总） ==================== -->
    <template v-else>
      <div class="tool-panel-titlebar">
        <span class="tool-panel-title">文件夹批量拆分</span>
        <span class="tool-panel-titlebar-hint">递归扫描文件夹下全部 .pptx 并逐个拆分</span>
      </div>

      <!-- 目录路径：手动输入 + 浏览按钮（目录选择对话框桥接） -->
      <div class="ps-field">
        <span class="ps-field-label">文件夹路径</span>
        <div class="ps-path-row">
          <el-input
            v-model="dirPath"
            placeholder="输入或浏览选择包含 .pptx 的文件夹路径"
            :disabled="batchRunning"
            @keyup.enter="startBatchSplit"
          />
          <el-button :disabled="batchRunning" @click="browseDir">浏览</el-button>
        </div>
      </div>

      <div class="ps-hint">
        递归扫描子目录（自动排除历史拆分产物，产物不会被再次拆分）；逐个串行拆分，
        单文件失败不中断整批；输出到各 PPT 同级「原文件名」文件夹，已存在的输出将被覆盖。
      </div>

      <!-- 双层进度：文件级（第 N/M 个 + 文件名）+ 页级（当前文件 done/total 页） -->
      <div v-if="batchRunning" class="ps-progress">
        <div class="ps-batch-progress-file">
          {{ batchFileProgress ? `正在拆分第 ${batchFileProgress.index} / ${batchFileProgress.total} 个文件：${batchFileProgress.currentFile}` : '正在扫描待拆分文件…' }}
        </div>
        <div v-if="batchPageProgress" class="ps-batch-progress-page">
          当前文件页进度：{{ batchPageProgress.done }} / {{ batchPageProgress.total }} 页
        </div>
      </div>

      <!-- 结果汇总：来自 SplitPptxFolder Promise 返回值（仅成功/失败完成态写入；
           取消时 Promise reject、Wails 通道丢弃返回值，已完成文件仅保留在磁盘不进此列表） -->
      <div v-if="batchResult" class="ps-result">
        <div class="ps-result-summary" :class="{ 'ps-result-summary-partial': batchFailedCount > 0 }">
          共 {{ batchResult.items.length }} / {{ batchResult.total }} 个文件处理完成：成功 {{ batchSucceededCount }} 个、失败 {{ batchFailedCount }} 个
        </div>
        <div class="ps-batch-list">
          <div v-for="item in batchResult.items" :key="item.srcPath" class="ps-batch-item">
            <div class="ps-batch-item-head">
              <span class="ps-batch-item-name">{{ baseName(item.srcPath) }}</span>
              <el-tag size="small" :type="item.error ? 'danger' : 'success'">
                {{ item.error ? '失败' : '成功' }}
              </el-tag>
              <span v-if="!item.error" class="ps-batch-item-pages">{{ item.pageCount }} 页</span>
            </div>
            <div v-if="item.outputDir" class="ps-batch-item-path">
              <span class="ps-batch-item-path-text">输出目录：{{ item.outputDir }}</span>
              <el-button size="small" link type="primary" @click="openOutputDir(item.outputDir)">打开文件夹</el-button>
            </div>
            <div v-if="item.error" class="ps-batch-item-error">失败原因：{{ item.error }}</div>
          </div>
        </div>
      </div>

      <div class="ps-actions">
        <el-button v-if="batchRunning" type="warning" @click="cancelBatchSplit">取消批量拆分</el-button>
        <el-button type="primary" :loading="batchRunning" @click="startBatchSplit">开始批量拆分</el-button>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import {
  SplitPptxPerPage,
  SplitPptxFolder,
  CancelPptxSplit,
  OpenFileDialog,
  OpenDirectoryDialog,
  OpenInExplorer
} from '../../wailsjs/go/main/App'
// 仅引 EventsOn：用其返回的「注销本监听器」闭包在组件卸载时调用，
// 精准移除本组件的 pptx-split:* 监听器。禁用 EventsOff——Wails v2 EventsOff 按
// eventName 删全部同名监听器，会误删其他监听方（文件树右键单文件拆分的 promise 直返
// 不依赖事件，但未来可能新增共听组件）。
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { handleError } from '../utils/error'

// 工具箱内嵌面板（替代原弹窗）：无 modelValue/visible 概念，事件监听随组件挂载注册、
// 卸载注销（工具切换 = 卸载重挂）。关闭工具箱（组件卸载）时在途批量/单文件拆分后台
// 继续跑属可接受语义（与原弹窗「关闭不取消」一致），不强制的说明见 cancelBatchSplit。

// ---- 模式 ----
const mode = ref('file')

// ==================== 单文件拆分（原弹窗逻辑原样迁移） ====================
const srcPath = ref('')
// 是否有本面板发起的在途拆分
const splitting = ref(false)
// 拆分进度 { done, total }（仅本面板发起的 srcPath 的 progress 事件写入）
const progress = ref(null)
// 拆分结果 model.PptxSplitResult { outputDir, files, pageCount }
const result = ref(null)
// 本面板在途拆分的源路径：pptx-split:* 事件载荷 srcPath 的过滤依据，
// 防其他入口（文件树右键、批量循环内单文件等）发起的拆分事件串入本面板状态
let pendingSrcPath = null

const resetSplitState = () => {
  splitting.value = false
  progress.value = null
  pendingSrcPath = null
}

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

// ==================== 文件夹批量拆分（PR3） ====================
const dirPath = ref('')
// 批量在途态：Promise 未决期间为 true（取消经 CancelPptxSplit 全量取消）
const batchRunning = ref(false)
// 文件级进度 { index, total, currentFile }（pptx-split:batch:progress 按 dirPath 过滤）
const batchFileProgress = ref(null)
// 页级进度 { done, total }（批量循环内单文件的 pptx-split:progress 按 currentFile 过滤）
const batchPageProgress = ref(null)
// 批量汇总 model.PptxBatchResult { dirPath, total, items }（Promise 返回值，仅完成态写入；
// 取消时 reject 不写——Wails 通道 error 非 null 即丢弃返回值，已完成文件只留在磁盘）
const batchResult = ref(null)
// 在途批量的目录路径（绝对路径，后端事件载荷 dirPath 与之一致）：batch 事件过滤依据
let pendingDirPath = null
// 当前正在拆分的文件路径：批量模式下页级 progress 事件的过滤依据
let batchCurrentFile = null

const batchSucceededCount = computed(() =>
  (batchResult.value?.items || []).filter((item) => !item.error).length
)
const batchFailedCount = computed(() =>
  (batchResult.value?.items || []).filter((item) => !!item.error).length
)

const baseName = (p) => (p || '').replaceAll('\\', '/').split('/').pop() || p

const startBatchSplit = async () => {
  const dir = dirPath.value.trim()
  if (!dir) {
    ElMessage.warning('请输入要拆分的文件夹路径')
    return
  }
  batchRunning.value = true
  batchFileProgress.value = null
  batchPageProgress.value = null
  batchResult.value = null
  pendingDirPath = dir
  batchCurrentFile = null
  try {
    const res = await SplitPptxFolder(dir)
    batchResult.value = res || null
    if (res) {
      const failed = (res.items || []).filter((item) => item.error).length
      if (failed > 0) {
        ElMessage.warning(`批量拆分完成：${res.items.length - failed} 个成功、${failed} 个失败（详见列表）`)
      } else {
        ElMessage.success(`批量拆分完成：${res.items.length} 个文件全部成功`)
      }
    }
  } catch (error) {
    // 取消（E_GIT_CANCELLED）走 warning 中性提示；空扫描等引导级错误也经此分流
    handleError('批量拆分失败: ', error)
  } finally {
    batchRunning.value = false
    batchFileProgress.value = null
    batchPageProgress.value = null
    pendingDirPath = null
    batchCurrentFile = null
  }
}

// 批量取消：复用 CancelPptxSplit 全量取消语义——当前文件中止、剩余文件不再开始、
// 已完成文件保留；整批经 Promise reject（E_GIT_CANCELLED）收尾
const cancelBatchSplit = async () => {
  try {
    await CancelPptxSplit()
  } catch (error) {
    handleError('取消批量拆分失败: ', error)
  }
}

// 打开输出目录（桌面端资源管理器桥接，OpenInExplorer 绑定）：打开失败（返回
// false）与调用异常（serve 浏览器通道后端守卫返回错误）均 warning 中性提示，
// 对齐组件既有 toast 风格（DirectoryTree handleOpenExplorer 先例），不阻塞拆分主流程
const openOutputDir = async (dir) => {
  if (!dir) return
  try {
    const ok = await OpenInExplorer(dir)
    if (!ok) ElMessage.warning('打开输出文件夹失败')
  } catch {
    ElMessage.warning('打开输出文件夹失败')
  }
}

// 浏览选择文件夹：桌面端目录选择对话框桥接；serve 浏览器通道不可用时 catch 静默
// 返回（输入框手动输入路径仍可用），对齐 browseFile 先例
const browseDir = async () => {
  let dir
  try {
    dir = await OpenDirectoryDialog('选择要拆分的文件夹')
  } catch {
    return
  }
  if (dir) dirPath.value = dir
}

// ==================== pptx-split:* 事件监听（挂载注册 / 卸载闭包精准注销） ====================
const onProgress = (payload) => {
  if (!payload || !payload.srcPath) return
  // 批量在途：progress 事件来自批量循环内的单文件拆分，按当前文件过滤写入页级进度
  if (batchRunning.value && pendingDirPath) {
    if (payload.srcPath === batchCurrentFile) {
      batchPageProgress.value = { done: payload.done, total: payload.total }
    }
    return
  }
  // 单文件模式：仅消费本面板发起的在途拆分
  if (payload.srcPath !== pendingSrcPath) return
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

const onBatchProgress = (payload) => {
  if (!payload || payload.dirPath !== pendingDirPath) return
  batchFileProgress.value = { index: payload.index, total: payload.total, currentFile: payload.currentFile }
  batchCurrentFile = payload.currentFile
  batchPageProgress.value = null
}

let offProgress = null
let offDone = null
let offError = null
let offBatchProgress = null

const registerListeners = () => {
  offProgress = EventsOn('pptx-split:progress', onProgress)
  offDone = EventsOn('pptx-split:done', onDone)
  offError = EventsOn('pptx-split:error', onError)
  offBatchProgress = EventsOn('pptx-split:batch:progress', onBatchProgress)
}

const unregisterListeners = () => {
  if (offProgress) { offProgress(); offProgress = null }
  if (offDone) { offDone(); offDone = null }
  if (offError) { offError(); offError = null }
  if (offBatchProgress) { offBatchProgress(); offBatchProgress = null }
}

onMounted(() => {
  registerListeners()
})

onBeforeUnmount(() => {
  unregisterListeners()
})
</script>

<style scoped>
.ps-panel {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.ps-mode-row {
  display: flex;
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
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-tertiary);
}

.ps-progress {
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 13px;
  color: var(--primary-color);
  background: var(--primary-bg);
  border-radius: var(--radius-md);
}

.ps-batch-progress-file {
  word-break: break-all;
}

.ps-batch-progress-page {
  margin-top: var(--spacing-xs);
  font-size: 12px;
  color: var(--text-secondary);
}

.ps-result {
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-md);
}

/* 结果汇总行 + 「打开文件夹」入口（两端对齐，路径长文本占主位） */
.ps-result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
}

.ps-result-summary {
  font-size: 13px;
  font-weight: 500;
  color: var(--success-color, var(--el-color-success));
  word-break: break-all;
}

/* 存在失败项时汇总行转警告语义色（成功/失败混合态不该显示纯成功绿） */
.ps-result-summary-partial {
  color: var(--warning-color);
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

.ps-batch-list {
  margin-top: var(--spacing-sm);
  max-height: 320px;
  overflow-y: auto;
}

.ps-batch-item {
  padding: var(--spacing-sm) 0;
  border-bottom: 1px solid var(--border-light);
}

.ps-batch-item:last-child {
  border-bottom: none;
}

.ps-batch-item-head {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.ps-batch-item-name {
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-primary);
  word-break: break-all;
}

.ps-batch-item-pages {
  font-size: 12px;
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.ps-batch-item-path {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: 2px;
  font-size: 12px;
  color: var(--text-tertiary);
  word-break: break-all;
}

.ps-batch-item-path-text {
  flex: 1;
  min-width: 0;
}

.ps-batch-item-error {
  margin-top: 2px;
  font-size: 12px;
  color: var(--danger-color, var(--el-color-danger));
  word-break: break-all;
}

.ps-actions {
  display: flex;
  gap: var(--spacing-sm);
}
</style>
