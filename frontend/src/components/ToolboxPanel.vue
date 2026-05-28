<template>
  <div class="toolbox-panel">
    <div class="toolbox-header">
      <span class="toolbox-title"><el-icon :size="18" style="margin-right:4px;vertical-align:middle;"><SetUp /></el-icon>工具箱</span>
      <span class="toolbox-close" @click="$emit('close')">&#10005;</span>
    </div>
    <div class="toolbox-content">
      <div
        v-for="tool in tools"
        :key="tool.id"
        class="toolbox-item"
        @click="tool.handler"
      >
        <div class="toolbox-item-header">
          <el-icon :size="16"><component :is="tool.icon" /></el-icon>
          <span class="toolbox-item-name">{{ tool.name }}</span>
        </div>
        <div class="toolbox-item-desc">{{ tool.description }}</div>
      </div>
    </div>

    <!-- 拷贝到对话框 -->
    <el-dialog
      v-model="copyToDialogVisible"
      title="拷贝到"
      width="480px"
      append-to-body
    >
      <el-form label-width="100px">
        <el-form-item label="原地址">
          <el-input
            v-model="copyToSourcePath"
            placeholder="请输入原文件或文件夹路径"
            :disabled="copyToLoading"
          />
        </el-form-item>
        <el-form-item label="目标地址">
          <el-input
            ref="copyToTargetInputRef"
            v-model="copyToTargetPath"
            placeholder="请输入目标文件夹路径"
            :disabled="copyToLoading"
            @keyup.enter="handleCopyTo"
          />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="copyToWholeDir" :disabled="copyToLoading">
            包含文件夹本身
          </el-checkbox>
        </el-form-item>
      </el-form>
      <div v-if="copyToPreview" class="copy-to-preview">
        <div class="copy-to-preview-label">拷贝效果预览</div>
        <div class="copy-to-preview-row">{{ copyToPreview.from }}</div>
        <div class="copy-to-preview-arrow">↓</div>
        <div class="copy-to-preview-row">{{ copyToPreview.to }}</div>
      </div>
      <template #footer>
        <el-button @click="copyToDialogVisible = false" :disabled="copyToLoading">取消</el-button>
        <el-button type="primary" @click="handleCopyTo" :loading="copyToLoading">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, SetUp } from '@element-plus/icons-vue'
import { CopyTo } from '../../wailsjs/go/main/App'

defineEmits(['close'])

// ---- 工具项定义 ----
const copyToDialogVisible = ref(false)
const copyToSourcePath = ref('')
const copyToTargetPath = ref('')
const copyToWholeDir = ref(true)
const copyToLoading = ref(false)
const copyToTargetInputRef = ref()

const showCopyToDialog = () => {
  copyToSourcePath.value = ''
  copyToTargetPath.value = ''
  copyToWholeDir.value = true
  copyToLoading.value = false
  copyToDialogVisible.value = true
  nextTick(() => {
    const input = copyToTargetInputRef.value?.input
    if (input) input.focus()
  })
}

const copyToPreview = computed(() => {
  const src = copyToSourcePath.value.trim().replaceAll('\\', '/')
  const dst = copyToTargetPath.value.trim().replaceAll('\\', '/')
  if (!src || !dst) return null
  const srcName = src.split('/').pop() || ''
  const normalizedDst = dst.replace(/\/+$/, '')
  if (copyToWholeDir.value) {
    return { from: src, to: normalizedDst + '/' + srcName }
  }
  return { from: src + '/*', to: normalizedDst + '/*' }
})

const handleCopyTo = async () => {
  if (!copyToSourcePath.value.trim()) {
    ElMessage.warning('请输入原地址')
    return
  }
  if (!copyToTargetPath.value.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }
  copyToLoading.value = true
  try {
    const result = await CopyTo(copyToSourcePath.value, copyToTargetPath.value, copyToWholeDir.value)
    if (result && result.startsWith('错误')) {
      ElMessage.error(result)
    } else {
      ElMessage.success('拷贝成功')
      copyToDialogVisible.value = false
    }
  } catch (error) {
    ElMessage.error('拷贝失败: ' + (error.message || String(error)))
  } finally {
    copyToLoading.value = false
  }
}

const tools = [
  {
    id: 'copyTo',
    name: '拷贝到',
    icon: CopyDocument,
    description: '将文件/文件夹拷贝到指定目录',
    handler: showCopyToDialog
  }
]
</script>

<style scoped>
.toolbox-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  background-color: var(--bg-primary);
}

.toolbox-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

.toolbox-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: 0.5px;
}

.toolbox-close {
  font-size: 16px;
  color: var(--text-tertiary);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  transition: all var(--transition-normal);
}

.toolbox-close:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

.toolbox-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-sm);
}

.toolbox-item {
  padding: 10px 12px;
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-color);
  cursor: pointer;
  margin-bottom: 6px;
  transition: all var(--transition-normal);
}

.toolbox-item:hover {
  border-color: var(--primary-light);
  box-shadow: var(--shadow-sm);
  background: #ecf5ff;
}

.toolbox-item-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-primary);
  font-weight: 500;
}

.toolbox-item:hover .toolbox-item-header {
  color: var(--primary-color);
}

.toolbox-item-desc {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 3px;
}

.copy-to-preview {
  margin: 0 0 16px;
  padding: var(--spacing-md);
  background-color: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
}

.copy-to-preview-label {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-bottom: 8px;
  font-weight: 500;
}

.copy-to-preview-row {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: var(--text-primary);
  word-break: break-all;
}

.copy-to-preview-arrow {
  text-align: center;
  color: var(--primary-color);
  font-size: 16px;
  margin: 4px 0;
}
</style>
