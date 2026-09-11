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
      <div v-else-if="!loading && left.length === 0 && right.length === 0 && fileGroups.length === 0" class="diff-empty">
        {{ binaryHint ? binaryHint : '无差异' }}
      </div>

      <!-- range 模式：多文件分组双栏 -->
      <div v-else-if="mode === 'range' && fileGroups.length > 0" class="range-groups">
        <div v-for="(group, gi) in fileGroups" :key="'g' + gi" class="range-file-group">
          <div class="range-file-header">{{ group.file }}</div>
          <div v-if="group.binary" class="diff-empty range-binary">该文件为二进制文件，不支持文本 diff 展示</div>
          <div v-else class="diff-table">
            <div class="diff-col diff-col-left">
              <div class="diff-col-header">{{ leftHeader }}</div>
              <div class="diff-col-body">
                <div
                  v-for="(line, idx) in group.left"
                  :key="'gl' + gi + idx"
                  class="diff-line"
                  :class="lineClass(line)"
                ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
              </div>
            </div>
            <div class="diff-col diff-col-right">
              <div class="diff-col-header">{{ rightHeader }}</div>
              <div class="diff-col-body">
                <div
                  v-for="(line, idx) in group.right"
                  :key="'gr' + gi + idx"
                  class="diff-line"
                  :class="lineClass(line)"
                ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 单文件双栏 diff -->
      <div v-else class="diff-table">
        <div class="diff-col diff-col-left">
          <div class="diff-col-header">{{ leftHeader }}</div>
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
          <div class="diff-col-header">{{ rightHeader }}</div>
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
import { GetFileDiff, GetCommitFileDiff, GetRangeDiff } from '../../wailsjs/go/main/App'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  repoPath: { type: String, required: true },
  file: { type: String, default: '' },
  // diff 来源模式：
  //   'workspace'（默认）：工作区单文件，调 GetFileDiff(repoPath, file)
  //   'commit'：指定提交中单文件，调 GetCommitFileDiff(repoPath, sha, file)
  //   'range'：两个提交区间全文件，调 GetRangeDiff(repoPath, baseSha, headSha)
  mode: { type: String, default: 'workspace' },
  sha: { type: String, default: '' },
  baseSha: { type: String, default: '' },
  headSha: { type: String, default: '' }
})

defineEmits(['update:modelValue'])

const loading = ref(false)
const error = ref('')
const binaryHint = ref('')
const left = ref([])
const right = ref([])
// range 模式：按 diff --git 头拆分的多文件分组，每项 { file, left, right }
const fileGroups = ref([])

const dialogTitle = computed(() => {
  if (props.mode === 'range') {
    return `区间差异 (${props.baseSha.slice(0, 8)} → ${props.headSha.slice(0, 8)})`
  }
  const name = props.file ? props.file.split(/[\\/]/).pop() : ''
  return name ? `文件差异 - ${name}` : '文件差异'
})

// 双栏标题按 diff 来源模式区分：
//   workspace：旧=HEAD/工作区前，新=工作区
//   commit：旧=父提交，新=该提交（root commit 时旧=空树，呈现全增）
//   range：旧=base 提交，新=head 提交
const leftHeader = computed(() => {
  if (props.mode === 'commit') return '父提交版本'
  if (props.mode === 'range') return `base (${props.baseSha.slice(0, 8)})`
  return '旧版本（HEAD / 工作区前）'
})

const rightHeader = computed(() => {
  if (props.mode === 'commit') return `该提交 (${props.sha.slice(0, 8)})`
  if (props.mode === 'range') return `head (${props.headSha.slice(0, 8)})`
  return '新版本（工作区）'
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
    // 跳过 diff 文件头行：diff --git / index / +++ / ---，避免污染双栏
    // （+++ 与 --- 首字符为 + / -，若不排除会被误当 add / del 行）
    if (raw.startsWith('diff --git') || raw.startsWith('index ') ||
        raw.startsWith('+++') || raw.startsWith('---')) {
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

/**
 * 解析 range diff（多文件 unified diff）为按文件分组的双栏结构。
 * git diff <base> <head> 输出形如：
 *   diff --git a/path b/path
 *   index ...
 *   --- a/path
 *   +++ b/path
 *   @@ -l,l +r,r @@
 *   ...
 * 按 `diff --git` 头拆段，每段取 b/ 路径为文件名，段内行喂 parseDiff。
 * 二进制段（含 "Binary files ... differ"）单独标记，不进双栏。
 */
const parseRangeDiff = (text) => {
  const groups = []
  if (!text) return groups

  // 按 "diff --git" 拆段，首段可能为空（文本以 diff --git 开头时）
  const segments = text.split(/^diff --git /m)
  for (const seg of segments) {
    if (!seg.trim()) continue
    // seg 形如 "a/path b/path\nindex ...\n--- a/path\n+++ b/path\n@@ ..."
    const headerMatch = seg.match(/^a\/\S+ b\/(.+?)(?:\r?\n)/)
    const file = headerMatch ? headerMatch[1] : '(未知文件)'
    const segText = 'diff --git ' + seg
    if (/^Binary files /m.test(segText)) {
      groups.push({ file, binary: true, left: [], right: [] })
      continue
    }
    const { leftLines, rightLines } = parseDiff(segText)
    if (leftLines.length === 0 && rightLines.length === 0) continue
    groups.push({ file, binary: false, left: leftLines, right: rightLines })
  }
  return groups
}

const loadDiff = async () => {
  if (!props.repoPath) return
  loading.value = true
  error.value = ''
  binaryHint.value = ''
  left.value = []
  right.value = []
  fileGroups.value = []
  try {
    let text
    if (props.mode === 'commit') {
      if (!props.sha || !props.file) return
      text = await GetCommitFileDiff(props.repoPath, props.sha, props.file)
    } else if (props.mode === 'range') {
      if (!props.baseSha || !props.headSha) return
      text = await GetRangeDiff(props.repoPath, props.baseSha, props.headSha)
    } else {
      // workspace：工作区单文件
      if (!props.file) return
      text = await GetFileDiff(props.repoPath, props.file)
    }
    if (!text || !text.trim()) {
      // 无 diff 文本：可能是二进制或无差异
      binaryHint.value = '无差异，或该文件类型不支持文本 diff 展示（二进制 / 图片）'
      return
    }
    // 单文件二进制兜底（range 模式按段各自判二进制，不走此分支）
    if (props.mode !== 'range' && /^Binary files /m.test(text)) {
      binaryHint.value = '该文件为二进制文件，不支持文本 diff 展示'
      return
    }
    if (props.mode === 'range') {
      fileGroups.value = parseRangeDiff(text)
      if (fileGroups.value.length === 0) {
        binaryHint.value = '无差异'
      }
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
  () => [props.modelValue, props.file, props.sha, props.baseSha, props.headSha, props.mode],
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

/* range 模式：多文件分组 */
.range-groups {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md, 16px);
}
.range-file-group {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  overflow: hidden;
}
.range-file-header {
  padding: 6px 10px;
  background: var(--bg-tertiary);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-color);
  word-break: break-all;
}
.range-binary {
  padding: 16px;
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
