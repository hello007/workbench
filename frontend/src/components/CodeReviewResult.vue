<template>
  <el-dialog
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    title="AI 代码审查结果"
    width="680px"
    :close-on-click-modal="false"
    append-to-body
    destroy-on-close
  >
    <!-- 加载态：生成中 -->
    <div v-if="loading" class="review-loading">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span>审查中...</span>
    </div>

    <template v-else>
      <!-- 整体结论 -->
      <div v-if="summary" class="review-summary">{{ summary }}</div>

      <!-- 问题清单为空：降级提示 -->
      <el-empty
        v-if="issues.length === 0"
        description="AI 未发现问题或输出格式异常"
        :image-size="60"
      />

      <!-- 问题清单按级别分组（critical 置顶 → warning → info） -->
      <div v-else class="review-groups">
        <div
          v-for="group in groupedIssues"
          :key="group.severity"
          class="review-group"
        >
          <div class="review-group-header">
            <el-tag :type="severityTagType(group.severity)" size="small" effect="dark">
              {{ severityLabel(group.severity) }}
            </el-tag>
            <span class="review-group-count">{{ group.items.length }} 个问题</span>
          </div>
          <div
            v-for="(issue, idx) in group.items"
            :key="group.severity + '-' + idx"
            class="review-issue"
          >
            <div class="review-issue-head">
              <el-tag :type="categoryTagType(issue.category)" size="small">
                {{ categoryLabel(issue.category) }}
              </el-tag>
              <span
                v-if="issue.file"
                class="review-issue-file"
                :title="'点击定位 ' + issue.file"
                @click="onLocateFile(issue)"
              >{{ issue.file }}<template v-if="issue.line">:{{ issue.line }}</template></span>
            </div>
            <div class="review-issue-desc">{{ issue.description }}</div>
            <div v-if="issue.suggestion" class="review-issue-suggestion">
              <span class="review-suggestion-label">建议：</span>{{ issue.suggestion }}
            </div>
          </div>
        </div>
      </div>
    </template>

    <template #footer>
      <el-button size="small" @click="$emit('update:modelValue', false)">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed } from 'vue'
import { Loading } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  // structuredOutput.issues 数组，每项 { file, line, severity, category, confidence, description, suggestion }
  issues: { type: Array, default: () => [] },
  // structuredOutput.summary 整体审查结论
  summary: { type: String, default: '' },
  // 生成中加载态：true 显示「审查中...」，false 展示问题清单
  loading: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'locate-file'])

// 按级别分组（critical 置顶 → warning → info），组内保留模型原始顺序。
// severity 取值对齐 code-review skill OutputSchema 枚举 critical|warning|info。
const groupedIssues = computed(() => {
  const order = ['critical', 'warning', 'info']
  const buckets = { critical: [], warning: [], info: [] }
  for (const issue of props.issues || []) {
    const sev = buckets[issue.severity] ? issue.severity : 'info'
    buckets[sev].push(issue)
  }
  return order
    .map(severity => ({ severity, items: buckets[severity] }))
    .filter(g => g.items.length > 0)
})

const severityTagType = (severity) => {
  if (severity === 'critical') return 'danger'
  if (severity === 'warning') return 'warning'
  return 'info'
}

const severityLabel = (severity) => {
  if (severity === 'critical') return '严重'
  if (severity === 'warning') return '警告'
  return '提示'
}

const categoryTagType = (category) => {
  if (category === 'bug') return 'danger'
  if (category === 'security') return 'danger'
  if (category === 'performance') return 'warning'
  return 'info'
}

const categoryLabel = (category) => {
  const map = {
    bug: '缺陷',
    style: '规范',
    security: '安全',
    performance: '性能',
    improvement: '改进'
  }
  return map[category] || category
}

// 点击问题文件路径：emit locate-file 供父组件打开 FileDiffDialog 定位文件。
// 行号定位 FileDiffDialog 暂不支持，仅打开文件（task 约定：不支持则仅打开文件）。
const onLocateFile = (issue) => {
  if (!issue.file) return
  emit('locate-file', issue.file)
}
</script>

<style scoped>
.review-loading {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 24px 0;
  color: var(--text-secondary);
  justify-content: center;
}

.review-summary {
  padding: 10px 12px;
  margin-bottom: 12px;
  background: var(--bg-tertiary);
  border-radius: 4px;
  font-size: 13px;
  color: var(--text-secondary);
  border-left: 3px solid var(--primary-color);
}

.review-groups {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 60vh;
  overflow-y: auto;
}

.review-group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.review-group-count {
  font-size: 12px;
  color: var(--text-tertiary);
}

.review-issue {
  padding: 10px 12px;
  border: 1px solid var(--border-color);
  border-radius: 4px;
  margin-bottom: 8px;
  background: var(--bg-secondary);
}

.review-issue-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}

.review-issue-file {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: var(--primary-color);
  cursor: pointer;
  word-break: break-all;
}

.review-issue-file:hover {
  text-decoration: underline;
}

.review-issue-desc {
  font-size: 14px;
  color: var(--text-primary);
  line-height: 1.5;
  word-break: break-word;
}

.review-issue-suggestion {
  margin-top: 6px;
  padding: 6px 8px;
  background: var(--bg-tertiary);
  border-radius: 3px;
  font-size: 13px;
  color: var(--text-secondary);
  line-height: 1.5;
  word-break: break-word;
}

.review-suggestion-label {
  color: var(--primary-color);
  font-weight: 600;
}
</style>
