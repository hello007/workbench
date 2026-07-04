<template>
  <el-dialog
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    :title="dialogTitle"
    width="80%"
    append-to-body
    destroy-on-close
  >
    <div v-loading="loading" class="diff-body">
      <!-- 错误 -->
      <div v-if="error" class="diff-empty">{{ error }}</div>

      <!-- 空差异 / 二进制 -->
      <div v-else-if="!loading && left.length === 0 && right.length === 0" class="diff-empty">
        {{ binaryHint ? binaryHint : '无差异' }}
      </div>

      <!-- 双栏 diff -->
      <div v-else class="diff-table">
        <div class="diff-col diff-col-left">
          <div class="diff-col-header">旧版本（HEAD / 工作区前）</div>
          <div class="diff-col-body">
            <div
              v-for="(line, idx) in left"
              :key="'l' + idx"
              class="diff-line"
              :class="lineClass(line)"
            ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
          </div>
        </div>
        <div class="diff-col diff-col-right">
          <div class="diff-col-header">新版本（工作区）</div>
          <div class="diff-col-body">
            <div
              v-for="(line, idx) in right"
              :key="'r' + idx"
              class="diff-line"
              :class="lineClass(line)"
            ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button type="primary" @click="$emit('update:modelValue', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { GetFileDiff } from '../../wailsjs/go/main/App'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  repoPath: { type: String, required: true },
  file: { type: String, default: '' }
})

defineEmits(['update:modelValue'])

const loading = ref(false)
const error = ref('')
const binaryHint = ref('')
const left = ref([])
const right = ref([])

const dialogTitle = computed(() => {
  const name = props.file ? props.file.split(/[\\/]/).pop() : ''
  return name ? `文件差异 - ${name}` : '文件差异'
})

const lineClass = (line) => {
  if (line.kind === 'add') return 'diff-line-add'
  if (line.kind === 'del') return 'diff-line-del'
  if (line.kind === 'hunk') return 'diff-line-hunk'
  return 'diff-line-context'
}

/**
 * 解析 unified diff 文本为左右两栏行数组。
 * - 以 `@@` hunk 头为分隔，记录左右起始行号。
 * - 空格行：左右都加（行号递增）。
 * - `-` 行：进左栏，右栏补空占位。
 * - `+` 行：进右栏，左栏补空占位。
 * - 行号取 hunk 头中的真实值，便于对照。
 */
const parseDiff = (text) => {
  const leftLines = []
  const rightLines = []
  if (!text) return { leftLines, rightLines }

  let leftNo = 0
  let rightNo = 0
  const lines = text.split(/\r?\n/)

  for (const raw of lines) {
    if (!raw) continue
    // hunk 头：@@ -lStart,lLen +rStart,rLen @@
    if (raw.startsWith('@@')) {
      const m = raw.match(/@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s+@@/)
      if (m) {
        leftNo = parseInt(m[1], 10)
        rightNo = parseInt(m[2], 10)
      }
      leftLines.push({ kind: 'hunk', no: '', text: raw })
      rightLines.push({ kind: 'hunk', no: '', text: raw })
      continue
    }
    // 普通行首字符为 diff 标记
    const tag = raw[0]
    const content = raw.slice(1)
    if (tag === '\\') {
      // "\ No newline at end of file" 提示，挂到对应侧最后一行（这里直接忽略，避免对齐复杂度）
      continue
    }
    if (tag === ' ') {
      leftLines.push({ kind: 'context', no: String(leftNo++), text: content })
      rightLines.push({ kind: 'context', no: String(rightNo++), text: content })
    } else if (tag === '-') {
      leftLines.push({ kind: 'del', no: String(leftNo++), text: content })
      rightLines.push({ kind: 'empty', no: '', text: '' })
    } else if (tag === '+') {
      rightLines.push({ kind: 'add', no: String(rightNo++), text: content })
      leftLines.push({ kind: 'empty', no: '', text: '' })
    } else {
      // 其他行（如 "diff --git" "index .." "+++" "---" 文件头）忽略，避免污染双栏
    }
  }

  return { leftLines, rightLines }
}

const loadDiff = async () => {
  if (!props.repoPath || !props.file) return
  loading.value = true
  error.value = ''
  binaryHint.value = ''
  left.value = []
  right.value = []
  try {
    const text = await GetFileDiff(props.repoPath, props.file)
    if (!text || !text.trim()) {
      // 无 diff 文本：可能是二进制或无差异
      binaryHint.value = '无差异，或该文件类型不支持文本 diff 展示（二进制 / 图片）'
      return
    }
    // git diff 对二进制文件会输出 "Binary files ... differ"
    if (/^Binary files /m.test(text)) {
      binaryHint.value = '该文件为二进制文件，不支持文本 diff 展示'
      return
    }
    const { leftLines, rightLines } = parseDiff(text)
    left.value = leftLines
    right.value = rightLines
  } catch (e) {
    error.value = '加载差异失败: ' + (e?.message || String(e))
    ElMessage.error(error.value)
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.modelValue, props.file],
  ([visible]) => {
    if (visible) loadDiff()
  }
)
</script>

<style scoped>
.diff-body {
  min-height: 240px;
  max-height: 65vh;
  overflow: auto;
}

.diff-empty {
  padding: 32px;
  text-align: center;
  color: var(--text-tertiary);
  font-size: 14px;
}

.diff-table {
  display: flex;
  gap: 8px;
  align-items: stretch;
}

.diff-col {
  flex: 1 1 50%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.diff-col-header {
  padding: 6px 10px;
  background: var(--bg-tertiary);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-color);
}

.diff-col-body {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12.5px;
  background: var(--bg-secondary);
  overflow-x: auto;
}

.diff-line {
  display: flex;
  white-space: pre;
  line-height: 1.55;
}

.diff-line-no {
  display: inline-block;
  min-width: 42px;
  padding: 0 8px;
  text-align: right;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-right: 1px solid var(--border-color);
  user-select: none;
  flex-shrink: 0;
}

.diff-line-text {
  padding: 0 8px;
  flex: 1;
  white-space: pre;
}

.diff-line-context .diff-line-text {
  color: var(--text-primary);
}

.diff-line-add {
  background: rgba(103, 194, 58, 0.15);
}
.diff-line-add .diff-line-text {
  color: #67c23a;
}

.diff-line-del {
  background: rgba(245, 108, 108, 0.15);
}
.diff-line-del .diff-line-text {
  color: #f56c6c;
}

.diff-line-empty {
  background: var(--bg-tertiary);
}

.diff-line-hunk {
  background: var(--primary-bg);
  color: var(--primary-color);
  font-weight: 600;
}
.diff-line-hunk .diff-line-text {
  color: var(--primary-color);
}
</style>
