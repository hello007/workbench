<template>
  <el-dialog
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    title="推送结果"
    width="600px"
    append-to-body
    destroy-on-close
  >
    <pre class="push-result-output">{{ output }}</pre>
    <template #footer>
      <el-button :icon="CopyDocument" @click="copyOutput">复制</el-button>
      <el-button type="primary" @click="$emit('update:modelValue', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  output: { type: String, default: '' }
})

defineEmits(['update:modelValue'])

const copyOutput = async () => {
  try {
    await navigator.clipboard.writeText(props.output)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}
</script>

<style scoped>
.push-result-output {
  max-height: 400px;
  overflow-y: auto;
  padding: var(--spacing-md);
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
