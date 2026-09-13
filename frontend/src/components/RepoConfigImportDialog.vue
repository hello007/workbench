<template>
  <el-dialog
    :model-value="visible"
    title="导入仓库列表配置"
    width="720px"
    append-to-body
    destroy-on-close
    class="repo-import-dialog"
    @update:model-value="$emit('update:visible', $event)"
  >
    <!-- 预览/决策态 -->
    <div v-if="step === 'preview'" v-loading="loading">
      <div class="preview-section" v-if="preview?.newDirectories?.length">
        <div class="preview-section-title">将新增工作目录（{{ preview.newDirectories.length }}）</div>
        <div class="preview-item" v-for="d in preview.newDirectories" :key="'nd-' + d.path">
          <span class="preview-item-name">{{ d.name }}</span>
          <span class="preview-item-path" :title="d.path">{{ d.path }}</span>
          <el-tag v-if="d.isDefault" size="small" type="warning">默认</el-tag>
        </div>
      </div>

      <div class="preview-section" v-if="preview?.conflictDirectories?.length">
        <div class="preview-section-title">
          冲突工作目录（{{ preview.conflictDirectories.length }}）—— 路径已存在，逐项决策
        </div>
        <div class="preview-item preview-conflict" v-for="d in preview.conflictDirectories" :key="'cd-' + d.path">
          <span class="preview-item-name">{{ d.name }}</span>
          <span class="preview-item-path" :title="d.path">{{ d.path }}</span>
          <el-radio-group v-model="dirDecisions[d.path]" size="small">
            <el-radio value="overwrite">覆盖本机</el-radio>
            <el-radio value="saveAsNew">另存为新项</el-radio>
            <el-radio value="skip">跳过</el-radio>
          </el-radio-group>
        </div>
      </div>

      <div class="preview-section" v-if="preview?.newFavorites?.length">
        <div class="preview-section-title">将新增收藏（{{ preview.newFavorites.length }}）</div>
        <div class="preview-item" v-for="f in preview.newFavorites" :key="'nf-' + f.path">
          <span class="preview-item-name">{{ f.alias || f.path }}</span>
          <span class="preview-item-path" :title="f.path">{{ f.path }}</span>
          <el-tag v-if="f.group" size="small" type="info">{{ f.group }}</el-tag>
        </div>
      </div>

      <div class="preview-section" v-if="preview?.conflictFavorites?.length">
        <div class="preview-section-title">
          冲突收藏（{{ preview.conflictFavorites.length }}）—— 路径已收藏，逐项决策
        </div>
        <div class="preview-item preview-conflict" v-for="f in preview.conflictFavorites" :key="'cf-' + f.path">
          <span class="preview-item-name">{{ f.alias || f.path }}</span>
          <span class="preview-item-path" :title="f.path">{{ f.path }}</span>
          <el-radio-group v-model="favDecisions[f.path]" size="small">
            <el-radio value="overwrite">覆盖本机</el-radio>
            <el-radio value="saveAsNew">另存为新项</el-radio>
            <el-radio value="skip">跳过</el-radio>
          </el-radio-group>
        </div>
      </div>

      <div class="preview-section" v-if="preview?.invalid?.length">
        <div class="preview-section-title preview-invalid-title">
          非法项（{{ preview.invalid.length }}）—— 不会导入
        </div>
        <div class="preview-item" v-for="(item, idx) in preview.invalid" :key="'iv-' + idx">
          <span class="preview-item-path">{{ item.name }}</span>
          <span class="preview-item-reason">{{ item.reason }}</span>
        </div>
      </div>

      <div class="preview-empty" v-if="!loading && !hasPreviewItems">无可导入项（配置为空或全部非法）</div>
    </div>

    <!-- 结果汇总态 -->
    <div v-else class="result-summary">
      <div class="result-counts">
        <span class="result-count">新增 {{ result?.added || 0 }}</span>
        <span class="result-count">覆盖 {{ result?.overwritten || 0 }}</span>
        <span class="result-count">跳过 {{ result?.skipped || 0 }}</span>
        <span class="result-count" :class="{ 'result-count--failed': (result?.failed || 0) > 0 }">
          失败 {{ result?.failed || 0 }}
        </span>
      </div>
      <div class="result-failures" v-if="result?.failedReasons?.length">
        <div class="preview-section-title preview-invalid-title">失败明细</div>
        <div class="result-failure-item" v-for="(reason, idx) in result.failedReasons" :key="'fr-' + idx">
          {{ reason }}
        </div>
      </div>
    </div>

    <template #footer>
      <template v-if="step === 'preview'">
        <el-button @click="$emit('update:visible', false)">取消</el-button>
        <el-button
          type="primary"
          :disabled="!hasPreviewItems"
          :loading="applying"
          @click="applyImport"
        >
          确认导入
        </el-button>
      </template>
      <template v-else>
        <el-button type="primary" @click="finish">完成</el-button>
      </template>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { OpenFileDialog, ReadFileBytes, PreviewRepoConfigImport, ApplyRepoConfigImport } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { decodeBase64Utf8 } from '../utils/base64'

const props = defineProps({
  visible: { type: Boolean, default: false }
})
const emit = defineEmits(['update:visible', 'imported'])

// step: preview（预览+决策）/ result（执行结果汇总）
const step = ref('preview')
const loading = ref(false)
const applying = ref(false)
const preview = ref(null)
const result = ref(null)
// 冲突决策表：path -> 'overwrite' | 'saveAsNew' | 'skip'（默认跳过，保守避免误覆盖）
const dirDecisions = reactive({})
const favDecisions = reactive({})
// 导入文件文本：预览与执行共用（执行时后端重新解析，保证决策与数据一致）
const importJsonText = ref('')

const hasPreviewItems = computed(() => {
  const p = preview.value
  if (!p) return false
  return (
    (p.newDirectories?.length || 0) > 0 ||
    (p.conflictDirectories?.length || 0) > 0 ||
    (p.newFavorites?.length || 0) > 0 ||
    (p.conflictFavorites?.length || 0) > 0
  )
})

watch(
  () => props.visible,
  (v) => {
    if (v) startImport()
  }
)

// 入口即选文件：对话框打开后立即弹文件选择（调用方只开关本对话框）
const startImport = async () => {
  step.value = 'preview'
  loading.value = false
  applying.value = false
  preview.value = null
  result.value = null
  Object.keys(dirDecisions).forEach((k) => delete dirDecisions[k])
  Object.keys(favDecisions).forEach((k) => delete favDecisions[k])

  let path
  try {
    path = await OpenFileDialog('选择仓库列表配置文件', [{ DisplayName: 'JSON 文件', Pattern: '*.json' }])
  } catch {
    emit('update:visible', false)
    return
  }
  if (!path) {
    // 用户取消（返回空串），静默关闭
    emit('update:visible', false)
    return
  }
  const filePath = Array.isArray(path) ? path[0] : path
  if (!filePath) {
    emit('update:visible', false)
    return
  }

  loading.value = true
  try {
    const bytes = await ReadFileBytes(filePath)
    if (bytes?.error) {
      ElMessage.error('读取文件失败: ' + bytes.error)
      emit('update:visible', false)
      return
    }
    if (bytes?.tooLarge) {
      ElMessage.error('文件过大（超 50MB），无法导入')
      emit('update:visible', false)
      return
    }
    importJsonText.value = bytes?.base64 ? decodeBase64Utf8(bytes.base64) : ''
    const p = await PreviewRepoConfigImport(importJsonText.value)
    preview.value = p
    for (const d of p?.conflictDirectories || []) dirDecisions[d.path] = 'skip'
    for (const f of p?.conflictFavorites || []) favDecisions[f.path] = 'skip'
  } catch (e) {
    // 非法 JSON / 不支持版本等全局错误经 handleError 分流（AppError code 命中则 error 级提示）
    handleError('导入解析失败: ', e)
    emit('update:visible', false)
  } finally {
    loading.value = false
  }
}

// 确认导入：决策表随文件文本交后端执行合并落盘，成功切换到结果汇总态
const applyImport = async () => {
  applying.value = true
  try {
    result.value = await ApplyRepoConfigImport(importJsonText.value, {
      directories: { ...dirDecisions },
      favorites: { ...favDecisions }
    })
    step.value = 'result'
    emit('imported')
  } catch (e) {
    handleError('导入执行失败: ', e)
  } finally {
    applying.value = false
  }
}

const finish = () => {
  emit('update:visible', false)
}
</script>

<style scoped>
.preview-section {
  margin-bottom: 16px;
}
.preview-section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 6px;
}
.preview-invalid-title {
  color: var(--danger-color, #f56c6c);
}
.preview-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  font-size: 13px;
}
.preview-conflict {
  flex-wrap: wrap;
}
.preview-item-name {
  font-weight: 500;
  flex-shrink: 0;
}
.preview-item-path {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 280px;
}
.preview-item-reason {
  font-size: 12px;
  color: var(--danger-color, #f56c6c);
}
.preview-empty {
  text-align: center;
  color: var(--text-tertiary);
  padding: 24px 0;
}
.result-summary {
  padding: 8px 0;
}
.result-counts {
  display: flex;
  gap: 20px;
  font-size: 14px;
  margin-bottom: 12px;
}
.result-count--failed {
  color: var(--danger-color, #f56c6c);
  font-weight: 600;
}
.result-failure-item {
  font-size: 12px;
  color: var(--text-secondary);
  padding: 2px 0;
}
</style>

<style>
/* 弹窗外壳圆角：append-to-body 后 .el-dialog 挂在 body 下，scoped 无法命中 */
.repo-import-dialog {
  border-radius: var(--radius-md);
  overflow: hidden;
}
</style>
