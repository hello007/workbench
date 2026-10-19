<template>
  <div class="ct-panel">
    <div class="tool-panel-titlebar">
      <span class="tool-panel-title">拷贝到</span>
      <span class="tool-panel-titlebar-hint">将文件/文件夹拷贝到指定目录</span>
    </div>

    <el-form label-width="100px" class="ct-form">
      <el-form-item label="原地址">
        <el-input
          v-model="sourcePath"
          placeholder="请输入原文件或文件夹路径"
          :disabled="loading"
        />
      </el-form-item>
      <div class="ct-swap-row">
        <el-button
          text
          size="small"
          :disabled="loading"
          @click="swapPaths"
        >
          <el-icon class="ct-swap-icon"><Sort /></el-icon>
          互换
        </el-button>
      </div>
      <el-form-item label="目标地址">
        <el-input
          v-model="targetPath"
          placeholder="请输入目标文件夹路径"
          :disabled="loading"
          @keyup.enter="handleCopyTo"
        />
      </el-form-item>
      <el-form-item label="文件名">
        <el-input
          v-model="targetName"
          placeholder="默认与源同名（仅文件生效），可修改"
          :disabled="loading"
          clearable
          @keyup.enter="handleCopyTo"
        />
      </el-form-item>
      <el-form-item>
        <el-checkbox v-model="wholeDir" :disabled="loading">
          包含文件夹本身
        </el-checkbox>
      </el-form-item>
    </el-form>

    <div v-if="preview" class="ct-preview">
      <div class="ct-preview-label">拷贝效果预览</div>
      <div class="ct-preview-row">{{ preview.from }}</div>
      <div class="ct-preview-arrow">↓</div>
      <div class="ct-preview-row">{{ preview.to }}</div>
    </div>

    <div class="ct-actions">
      <el-button type="primary" :loading="loading" @click="handleCopyTo">开始拷贝</el-button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Sort } from '@element-plus/icons-vue'
import { CopyTo } from '../../wailsjs/go/main/App'

// 拷贝到面板（工具箱内嵌，替代原弹窗表单）：逻辑自 ToolboxPanel 原样迁移。
// 面板常驻无「关闭」语义，成功后重置表单（等价原「成功后关闭对话框」的下一轮起点）。

const sourcePath = ref('')
const targetPath = ref('')
// 自定义目标文件名：源路径手填无类型信息，输入框始终显示，默认取源路径最后一段；
// 源实际是目录时由后端忽略并在返回串中提示
const targetName = ref('')
const wholeDir = ref(true)
const loading = ref(false)

const resetForm = () => {
  sourcePath.value = ''
  targetPath.value = ''
  targetName.value = ''
  wholeDir.value = true
}

const defaultCopyToName = () => {
  const src = sourcePath.value.trim().replaceAll('\\', '/')
  return src.split('/').pop() || ''
}

// 源路径变化时同步默认名：仅当用户未自行改名（当前值为空或等于旧默认名）时跟随，
// 避免覆盖用户已输入的自定义名
watch(sourcePath, (newVal, oldVal) => {
  const oldSrc = (oldVal || '').trim().replaceAll('\\', '/')
  const oldDefault = oldSrc.split('/').pop() || ''
  if (!targetName.value || targetName.value === oldDefault) {
    targetName.value = defaultCopyToName()
  }
})

const preview = computed(() => {
  const src = sourcePath.value.trim().replaceAll('\\', '/')
  const dst = targetPath.value.trim().replaceAll('\\', '/')
  if (!src || !dst) return null
  const srcName = src.split('/').pop() || ''
  const normalizedDst = dst.replace(/\/+$/, '')
  if (wholeDir.value) {
    const finalName = targetName.value.trim() || srcName
    return { from: src, to: normalizedDst + '/' + finalName }
  }
  return { from: src + '/*', to: normalizedDst + '/*' }
})

// 互换原地址与目标地址；文件名重置为新源路径的默认值（源已变，旧自定义名不再适用）
const swapPaths = () => {
  const temp = sourcePath.value
  sourcePath.value = targetPath.value
  targetPath.value = temp
  targetName.value = defaultCopyToName()
}

// 文件名非法字符校验（与 Windows 资源管理器一致 + 路径分隔符）
const ILLEGAL_NAME_RE = /[\\/:*?"<>|]/

const handleCopyTo = async () => {
  if (!sourcePath.value.trim()) {
    ElMessage.warning('请输入原地址')
    return
  }
  if (!targetPath.value.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }
  const name = targetName.value.trim()
  if (name && (ILLEGAL_NAME_RE.test(name) || name === '.' || name === '..')) {
    ElMessage.warning('文件名含非法字符（\\ / : * ? " < > |）')
    return
  }
  loading.value = true
  try {
    const result = await CopyTo(sourcePath.value, targetPath.value, name, wholeDir.value)
    if (result && result.startsWith('错误')) {
      ElMessage.error(result)
    } else {
      // 源为目录且自定义名被忽略时，后端在结果串附提示，完整展示给用户
      ElMessage.success(result ? `拷贝成功 ${result}` : '拷贝成功')
      resetForm()
    }
  } catch (error) {
    ElMessage.error('拷贝失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.ct-panel {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.ct-form {
  max-width: 560px;
}

/* 互换按钮行 */
.ct-swap-row {
  display: flex;
  justify-content: center;
  margin: -8px 0 0;
}

.ct-swap-row .el-button {
  color: var(--text-tertiary);
  font-size: 12px;
}

.ct-swap-row .el-button:hover {
  color: var(--primary-color);
}

.ct-swap-row .el-button:hover .ct-swap-icon {
  transform: rotate(180deg);
}

.ct-swap-icon {
  transition: transform 0.3s ease;
}

.ct-preview {
  max-width: 560px;
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-md);
}

.ct-preview-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-tertiary);
  margin-bottom: var(--spacing-xs);
}

.ct-preview-row {
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  font-size: 12px;
  color: var(--text-secondary);
  word-break: break-all;
}

.ct-preview-arrow {
  font-size: 12px;
  color: var(--text-tertiary);
  padding: 2px 0;
}

.ct-actions {
  display: flex;
  gap: var(--spacing-sm);
}
</style>
