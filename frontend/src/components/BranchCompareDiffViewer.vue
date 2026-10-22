<!-- frontend/src/components/BranchCompareDiffViewer.vue -->
<!-- 分支差异 diff 查看器（R6）：props.diffText 经 utils/diffParse 解析渲染，
     双栏（默认）/ 统一视图切换；二进制占位提示；大 diff 截断保护（>5000 行只渲染
     前 5000 行并提示）。着色走设计令牌 CSS 变量（暗色主题安全）。 -->
<template>
  <div class="bc-diff-viewer">
    <div class="viewer-toolbar">
      <el-radio-group v-model="viewMode" size="small">
        <el-radio-button value="side">双栏</el-radio-button>
        <el-radio-button value="unified">统一</el-radio-button>
      </el-radio-group>
      <span v-if="truncated" class="truncated-tip">diff 过大，仅显示前 {{ MAX_LINES }} 行</span>
    </div>

    <!-- 二进制占位（R6） -->
    <div v-if="isBinary" class="viewer-empty">二进制文件不支持文本对比</div>
    <div v-else-if="!diffText || rowsCount === 0" class="viewer-empty">无差异</div>

    <!-- 双栏视图 -->
    <div v-else-if="viewMode === 'side'" class="diff-table">
      <div class="diff-col diff-col-left">
        <div class="diff-col-header">{{ baseLabel }}</div>
        <div class="diff-col-body">
          <div
            v-for="(line, idx) in leftLines"
            :key="'l' + idx"
            class="diff-line"
            :class="lineClass(line)"
          ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
        </div>
      </div>
      <div class="diff-col diff-col-right">
        <div class="diff-col-header">{{ targetLabel }}</div>
        <div class="diff-col-body">
          <div
            v-for="(line, idx) in rightLines"
            :key="'r' + idx"
            class="diff-line"
            :class="lineClass(line)"
          ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
        </div>
      </div>
    </div>

    <!-- 统一视图：左右数组按位合并（parseDiff 逐行 1:1 填充，del/add 对位还原） -->
    <div v-else class="diff-unified">
      <div
        v-for="(line, idx) in unifiedLines"
        :key="'u' + idx"
        class="diff-line"
        :class="lineClass(line)"
      ><span class="diff-line-sign">{{ line.sign }}</span><span class="diff-line-text">{{ line.text }}</span></div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { parseDiff } from '../utils/diffParse'

defineOptions({ name: 'BranchCompareDiffViewer' })

const props = defineProps({
  // 后端 GetBranchCompareFileDiff 返回的 unified diff 原文（''/null = 无差异）
  diffText: { type: String, default: '' },
  // 双栏列头（分支对比场景为 A/B 分支名）
  baseLabel: { type: String, default: '旧版本' },
  targetLabel: { type: String, default: '新版本' }
})

// 大 diff 截断阈值（行）：超过后仅渲染前 MAX_LINES 行，防长列表卡死
const MAX_LINES = 5000

const viewMode = ref('side')

// 二进制判定：git diff 对二进制文件输出 "Binary files ... differ" 样式文本
const isBinary = computed(() => /^Binary files /m.test(props.diffText || ''))

const parsed = computed(() => parseDiff(props.diffText || ''))

// 总行数（左右等长，取其一）用于截断判定
const rowsCount = computed(() => parsed.value.leftLines.length)

// 大 diff 截断保护
const truncated = computed(() => rowsCount.value > MAX_LINES)

const leftLines = computed(() => {
  const all = parsed.value.leftLines
  return truncated.value ? all.slice(0, MAX_LINES) : all
})

const rightLines = computed(() => {
  const all = parsed.value.rightLines
  return truncated.value ? all.slice(0, MAX_LINES) : all
})

// 统一行：parseDiff 双栏逐行 1:1 对位（del 行右栏为 empty、add 行左栏为 empty、
// context 两侧均有），按位合并即还原 unified 顺序
const unifiedLines = computed(() => {
  const { leftLines: left, rightLines: right } = parsed.value
  const out = []
  const total = truncated.value ? MAX_LINES : left.length
  for (let i = 0; i < total; i++) {
    const l = left[i]
    const r = right[i]
    if (!l) break
    if (l.kind === 'del') {
      out.push({ sign: '-', text: l.text, kind: 'del' })
    } else if (r && r.kind === 'add') {
      out.push({ sign: '+', text: r.text, kind: 'add' })
    } else if (l.kind === 'hunk') {
      out.push({ sign: '', text: l.text, kind: 'hunk' })
    } else if (l.kind === 'context' && r && r.kind === 'context') {
      out.push({ sign: '', text: l.text, kind: 'context' })
    }
    // 其余为占位 empty 行，跳过
  }
  return out
})

const lineClass = (line) => {
  if (line.kind === 'add') return 'diff-line-add'
  if (line.kind === 'del') return 'diff-line-del'
  if (line.kind === 'hunk') return 'diff-line-hunk'
  if (line.kind === 'empty') return 'diff-line-empty'
  return 'diff-line-context'
}
</script>

<style scoped>
.bc-diff-viewer {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm, 8px);
  min-height: 0;
  height: 100%;
}

.viewer-toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm, 8px);
}

.truncated-tip {
  font-size: 12px;
  color: var(--warning-color);
}

.viewer-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
  color: var(--text-tertiary);
  font-size: 14px;
  border: 1px dashed var(--border-color);
  border-radius: var(--radius-sm, 4px);
}

.diff-table {
  display: flex;
  gap: var(--spacing-sm, 8px);
  align-items: stretch;
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.diff-col {
  flex: 1 1 50%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
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

.diff-col-body,
.diff-unified {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12.5px;
  background: var(--bg-secondary);
  overflow: auto;
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

.diff-line-sign {
  display: inline-block;
  width: 18px;
  text-align: center;
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

/* 增删行底色用语义色 color-mix 派生（透明度叠加，双主题安全，禁硬编码 hex） */
.diff-line-add {
  background: color-mix(in srgb, var(--success-color) 14%, transparent);
}
.diff-line-add .diff-line-text {
  color: var(--success-color);
}

.diff-line-del {
  background: color-mix(in srgb, var(--danger-color) 14%, transparent);
}
.diff-line-del .diff-line-text {
  color: var(--danger-color);
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
