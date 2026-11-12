<template>
  <div class="prompt-panel">
    <!-- 顶部标题栏：风格对齐 TempTodoPanel / AiFunctionPanel 的 .panel-header -->
    <div class="panel-header">
      <div class="panel-heading">
        <span class="panel-title">
          <el-icon :size="18" class="panel-title-icon"><Collection /></el-icon>
          提示词
        </span>
        <span class="panel-subtitle">
          共 {{ prompts.length }} 条<template v-if="keyword.trim()">，匹配 {{ filteredPrompts.length }} 条</template>
        </span>
      </div>
      <div class="panel-actions">
        <el-input
          v-model="keyword"
          size="small"
          clearable
          placeholder="搜索标题或正文"
          class="prompt-search"
        />
        <el-button type="primary" size="small" data-action="create" @click="startCreate">
          <el-icon class="btn-icon"><Plus /></el-icon>
          新建
        </el-button>
      </div>
    </div>

    <div class="panel-body">
      <div v-if="loading" class="prompt-empty">正在加载提示词…</div>
      <div v-else-if="!filteredPrompts.length" class="prompt-empty">
        <template v-if="keyword.trim()">没有匹配「{{ keyword.trim() }}」的提示词</template>
        <template v-else>暂无提示词，点击右上角「新建」添加常用话术</template>
      </div>

      <div
        v-for="prompt in filteredPrompts"
        :key="prompt.id"
        class="prompt-item"
        :data-prompt-id="prompt.id"
      >
        <div class="prompt-title">{{ prompt.title }}</div>
        <div class="prompt-content" :class="{ 'is-expanded': expandedIDs.has(prompt.id) }">{{ prompt.content }}</div>

        <div class="prompt-meta">
          <span class="prompt-time" :title="formatFullTime(prompt.updatedAt)">{{ formatTime(prompt.updatedAt) }}</span>
          <span v-if="placeholderCountOf(prompt.content)" class="prompt-badge">
            {{ placeholderCountOf(prompt.content) }} 个占位符
          </span>
          <div class="prompt-actions">
            <el-button
              v-if="isLongContent(prompt.content)"
              link
              size="small"
              data-action="toggle-expand"
              @click="toggleExpanded(prompt.id)"
            >
              {{ expandedIDs.has(prompt.id) ? '收起' : '展开' }}
            </el-button>
            <el-tooltip content="复制" placement="top" :show-after="300">
              <el-button link size="small" data-action="copy" @click="copyPrompt(prompt)">
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </el-tooltip>
            <el-tooltip content="编辑" placement="top" :show-after="300">
              <el-button link size="small" data-action="edit" @click="startEdit(prompt)">
                <el-icon><Edit /></el-icon>
              </el-button>
            </el-tooltip>
            <el-tooltip content="删除" placement="top" :show-after="300">
              <el-button link size="small" class="prompt-delete-btn" data-action="delete" @click="removePrompt(prompt)">
                <el-icon><Delete /></el-icon>
              </el-button>
            </el-tooltip>
          </div>
        </div>
      </div>
    </div>

    <!-- 新建 / 编辑弹窗：正文较长，用弹窗承载（列表项内不做行内编辑） -->
    <el-dialog
      v-model="editorVisible"
      :title="editorTitle"
      width="min(720px, 86vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <el-input
        v-model="draftTitle"
        maxlength="100"
        show-word-limit
        placeholder="标题（如：代码评审话术）"
        data-field="title"
      />
      <el-input
        v-model="draftContent"
        type="textarea"
        :rows="10"
        maxlength="20000"
        show-word-limit
        resize="vertical"
        class="prompt-editor-content"
        placeholder="正文。可用双花括号占位符标记复制时需要填写的内容，支持「名称:默认值」写法"
        data-field="content"
      />
      <div class="prompt-editor-hint" v-pre>占位符写法：{{名称}} 或 {{名称:默认值}}。复制时弹窗填写，留空使用默认值（无默认值则保留原样）。</div>
      <template #footer>
        <el-button @click="editorVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" data-action="save" @click="savePrompt">保存</el-button>
      </template>
    </el-dialog>

    <!-- 占位符填写弹窗：留空的项使用默认值，无默认值保留原样；「复制原文」跳过替换 -->
    <el-dialog
      v-model="fillVisible"
      title="填写占位符"
      width="min(640px, 85vw)"
      append-to-body
      :close-on-click-modal="false"
    >
      <div class="prompt-fill-tip" v-pre>留空的项按默认值填入（无默认值保留 {{名称}} 原样），可复制后手动补填。</div>
      <div v-for="def in fillDefs" :key="def.name" class="prompt-fill-row">
        <label class="prompt-fill-label">{{ def.name }}</label>
        <el-input
          v-model="fillValues[def.name]"
          type="textarea"
          :autosize="{ minRows: 1, maxRows: 6 }"
          :placeholder="fillPlaceholder(def)"
          :data-fill-name="def.name"
        />
      </div>
      <template #footer>
        <el-button data-action="copy-original" @click="copyOriginal">复制原文</el-button>
        <el-button type="primary" data-action="copy-filled" @click="copyFilled">替换并复制</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Collection, Plus, CopyDocument, Edit, Delete } from '@element-plus/icons-vue'
import { ListPrompts, AddPrompt, UpdatePrompt, DeletePrompt } from '../../wailsjs/go/main/App'
import { ErrorCode, handleError } from '../utils/error'
import { applyPlaceholders, extractPlaceholderDefs, extractPlaceholders } from '../utils/promptPlaceholders'

const prompts = ref([])
const loading = ref(false)
const keyword = ref('')
const expandedIDs = ref(new Set())

// 编辑弹窗：新建与编辑共用同一套草稿控件，editingId 为空表示新建
const editorVisible = ref(false)
const editingId = ref('')
const draftTitle = ref('')
const draftContent = ref('')
const saving = ref(false)
const editorTitle = computed(() => (editingId.value ? '编辑提示词' : '新建提示词'))

// 占位符填写弹窗
const fillVisible = ref(false)
const fillTarget = ref(null)
// 按名称去重的占位符定义（含默认值），填写弹窗逐项渲染
const fillDefs = ref([])
const fillValues = ref({})

/** 填写框提示：有默认值时提示可留空并展示默认值，无默认值沿用原文案。 */
const fillPlaceholder = (def) => (def.defaultValue
  ? `输入「${def.name}」的内容（可留空，默认 ${def.defaultValue}）`
  : `输入「${def.name}」的内容（可留空）`)

const filteredPrompts = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return prompts.value
  return prompts.value.filter(p =>
    p.title.toLowerCase().includes(kw) || p.content.toLowerCase().includes(kw)
  )
})

const load = async () => {
  loading.value = true
  try {
    prompts.value = await ListPrompts()
    // 列表刷新后清理已不存在的展开态，避免 ID 复用/删除后残留
    const ids = new Set(prompts.value.map(p => p.id))
    const next = new Set()
    for (const id of expandedIDs.value) {
      if (ids.has(id)) next.add(id)
    }
    expandedIDs.value = next
  } catch (e) {
    handleError('加载提示词失败：', e)
  } finally {
    loading.value = false
  }
}

const placeholderCountOf = (content) => extractPlaceholders(content).length

const isLongContent = (content) => content.length > 120 || content.includes('\n')

const toggleExpanded = (id) => {
  const next = new Set(expandedIDs.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedIDs.value = next
}

const formatTime = (ms) => {
  if (!ms) return '--'
  const d = new Date(ms)
  if (Number.isNaN(d.getTime())) return '--'
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

const formatFullTime = (ms) => {
  if (!ms) return '--'
  const d = new Date(ms)
  if (Number.isNaN(d.getTime())) return '--'
  return d.toLocaleString('zh-CN', { hour12: false })
}

const startCreate = () => {
  editingId.value = ''
  draftTitle.value = ''
  draftContent.value = ''
  editorVisible.value = true
}

const startEdit = (prompt) => {
  editingId.value = prompt.id
  draftTitle.value = prompt.title
  draftContent.value = prompt.content
  editorVisible.value = true
}

const savePrompt = async () => {
  if (!draftTitle.value.trim()) {
    ElMessage.warning('标题不能为空')
    return
  }
  if (!draftContent.value.trim()) {
    ElMessage.warning('正文不能为空')
    return
  }

  saving.value = true
  try {
    if (editingId.value) {
      await UpdatePrompt(editingId.value, draftTitle.value, draftContent.value)
    } else {
      await AddPrompt(draftTitle.value, draftContent.value)
    }
    editorVisible.value = false
    await load()
  } catch (e) {
    handleError('保存提示词失败：', e)
    // 列表已过期（条目被其他端删除）：刷新后关闭编辑弹窗，避免对不存在的
    // 条目反复重试；刷新还能把新数据带回列表。
    if (e?.code === ErrorCode.PromptNotFound) {
      editorVisible.value = false
      await load()
    }
  } finally {
    saving.value = false
  }
}

const removePrompt = async (prompt) => {
  try {
    await ElMessageBox.confirm(
      `确定删除提示词「${prompt.title}」吗？此操作不可撤销。`,
      '删除提示词',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }

  try {
    await DeletePrompt(prompt.id)
    await load()
  } catch (e) {
    handleError('删除提示词失败：', e)
    // 条目已被其他端删除：本地列表过期，重新拉取即可收敛（无需额外提示）
    if (e?.code === ErrorCode.PromptNotFound) {
      await load()
    }
  }
}

const writeClipboard = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
    return true
  } catch {
    ElMessage.error('复制失败，请检查剪贴板权限后重试')
    return false
  }
}

const copyPrompt = async (prompt) => {
  const defs = extractPlaceholderDefs(prompt.content)
  if (!defs.length) {
    await writeClipboard(prompt.content)
    return
  }

  fillTarget.value = prompt
  fillDefs.value = defs
  fillValues.value = Object.fromEntries(defs.map(def => [def.name, '']))
  fillVisible.value = true
}

const copyOriginal = async () => {
  if (!fillTarget.value) return
  if (await writeClipboard(fillTarget.value.content)) {
    fillVisible.value = false
  }
}

const copyFilled = async () => {
  if (!fillTarget.value) return
  const text = applyPlaceholders(fillTarget.value.content, fillValues.value)
  if (await writeClipboard(text)) {
    fillVisible.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.prompt-panel {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
  overflow: hidden;
}

.panel-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding-block: var(--spacing-md);
  padding-inline: max(var(--spacing-md), calc((100% - 1000px) / 2));
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

.panel-heading {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-width: 0;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  white-space: nowrap;
}

.panel-title-icon {
  color: var(--primary-color);
}

.panel-subtitle {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.panel-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex: 0 1 auto;
  min-width: 0;
}

.prompt-search {
  flex: 0 1 240px;
  min-width: 160px;
}

.btn-icon {
  margin-right: 2px;
}

.panel-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-md) max(var(--spacing-md), calc((100% - 1000px) / 2));
}

.prompt-empty {
  padding: 32px 16px;
  text-align: center;
  font-size: 13px;
  color: var(--text-tertiary);
}

.prompt-item {
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: border-color var(--transition-fast), background var(--transition-fast);
}

.prompt-item:hover {
  border-color: var(--border-color);
  background: var(--bg-tertiary);
}

.prompt-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  word-break: break-word;
}

.prompt-content {
  margin-top: 6px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 展开态限高可滚动（对齐 TempTodoPanel 展开口径）：超长正文不把列表项撑高，
   收起态仍靠上方三行 clamp */
.prompt-content.is-expanded {
  display: block;
  -webkit-line-clamp: unset;
  max-height: 50vh;
  overflow-y: auto;
}

.prompt-meta {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-sm);
  min-height: 24px;
}

.prompt-time {
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
}

.prompt-badge {
  font-size: 11px;
  color: var(--primary-color);
  background: var(--primary-light);
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  white-space: nowrap;
}

/* 操作图标常驻 meta 行右端（对齐 TempTodoPanel .todo-actions 做法）：复制是面板
   主操作，不依赖 hover 才可见；默认低对比度，悬停卡片或图标本身时提亮 */
.prompt-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.prompt-actions .el-button {
  padding: var(--spacing-xs);
  height: auto;
  color: var(--text-tertiary);
}

.prompt-item:hover .prompt-actions .el-button {
  color: var(--text-secondary);
}

.prompt-actions .el-button:hover {
  color: var(--text-primary);
}

.prompt-actions .prompt-delete-btn:hover {
  color: var(--danger-color);
}

.prompt-editor-content {
  margin-top: var(--spacing-md);
}

.prompt-editor-hint {
  margin-top: var(--spacing-sm);
  font-size: 12px;
  color: var(--text-tertiary);
}

/* 标题/正文字数提示统一弱化（EP 2.14.5 实际 DOM 为 .el-input__count > .el-input__count-inner）：
   只改颜色与字号，不动位置/高度。编辑弹窗 append-to-body 后仍生效——el-input 根元素
   携带本组件 scope id，:deep 编译为 [data-v-x] .el-input__count 后代选择器 */
:deep(.el-input__count) {
  color: var(--text-tertiary);
  font-size: 12px;
}

.prompt-fill-tip {
  margin-bottom: var(--spacing-md);
  font-size: 12px;
  color: var(--text-tertiary);
}

.prompt-fill-row {
  margin-bottom: var(--spacing-md);
}

.prompt-fill-label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  word-break: break-word;
}

@media (max-width: 900px) {
  .panel-subtitle {
    display: none;
  }

  .prompt-search {
    flex-basis: 160px;
  }
}
</style>
