<template>
  <div class="temp-todo-panel">
    <!-- 顶部标题栏：风格对齐 AiFunctionPanel 的 .panel-header -->
    <div class="panel-header">
      <div class="panel-heading">
        <span class="panel-title">
          <el-icon :size="18" class="panel-title-icon"><Memo /></el-icon>
          临时待办
        </span>
        <span class="panel-subtitle">
          <span class="subtitle-sign">&gt;</span>{{ quickShortcutTip }} 从剪贴板快速记录
        </span>
      </div>
      <div class="panel-actions">
        <el-input
          v-model="keyword"
          size="small"
          clearable
          placeholder="搜索正文或标签"
          class="todo-search"
        />
        <el-button
          size="small"
          data-action="export"
          :disabled="!todos.length"
          @click="exportTodos"
        >
          <el-icon class="btn-icon"><Download /></el-icon>
          导出
        </el-button>
        <el-button size="small" data-action="import" @click="startImport">
          <el-icon class="btn-icon"><Upload /></el-icon>
          导入
        </el-button>
        <el-button type="primary" size="small" data-action="create" :disabled="creating" @click="startCreate">
          <el-icon class="btn-icon"><Plus /></el-icon>
          新建
        </el-button>
      </div>
    </div>

    <div class="panel-body">
      <!-- 新建卡片：与编辑态共用同一套草稿控件 -->
      <div v-if="creating" class="todo-editor">
        <el-input
          ref="createInputRef"
          v-model="draftText"
          type="textarea"
          :rows="3"
          resize="vertical"
          placeholder="要记的事、提示词片段或待验证的想法（Ctrl+Enter 保存）"
          @keydown.ctrl.enter.prevent="saveDraft"
        />
        <div class="todo-editor-footer">
          <el-input
            v-model="draftTags"
            size="small"
            placeholder="标签，用逗号分隔（如 重构,AI提示词）"
            class="todo-tags-input"
          />
          <div class="todo-editor-actions">
            <el-button size="small" @click="cancelDraft">取消</el-button>
            <el-button size="small" type="primary" :loading="saving" @click="saveDraft">保存</el-button>
          </div>
        </div>
      </div>

      <!-- 待办区 -->
      <div class="todo-section">
        <div class="todo-section-title">
          待办
          <span class="todo-count">{{ pendingTodos.length }}</span>
        </div>

        <div v-if="!pendingTodos.length" class="todo-empty">
          <template v-if="keyword.trim()">没有匹配「{{ keyword.trim() }}」的待办</template>
          <template v-else>暂无待办，按 {{ quickShortcutTip }} 从剪贴板记一条</template>
        </div>

        <div
          v-for="todo in pendingTodos"
          :key="todo.id"
          class="todo-item"
          :data-todo-id="todo.id"
        >
          <el-checkbox
            :model-value="false"
            class="todo-check"
            :disabled="busyId === todo.id"
            @change="toggleCompleted(todo, true)"
          />

          <template v-if="editingId === todo.id">
            <div class="todo-editor todo-editor--inline">
              <el-input
                v-model="draftText"
                type="textarea"
                :rows="3"
                resize="vertical"
                @keydown.ctrl.enter.prevent="saveDraft"
              />
              <div class="todo-editor-footer">
                <el-input
                  v-model="draftTags"
                  size="small"
                  placeholder="标签，用逗号分隔"
                  class="todo-tags-input"
                />
                <div class="todo-editor-actions">
                  <el-button size="small" @click="cancelDraft">取消</el-button>
                  <el-button size="small" type="primary" :loading="saving" @click="saveDraft">保存</el-button>
                </div>
              </div>
            </div>
          </template>

          <template v-else>
            <div class="todo-main">
              <div
                class="todo-text"
                :class="{ 'is-expanded': expandedIDs.has(todo.id) }"
                :title="expandedIDs.has(todo.id) ? '点击收起' : '点击展开全文'"
                @click="toggleExpanded(todo.id)"
              >{{ todo.text }}</div>
              <div class="todo-meta">
                <span class="todo-time" :title="formatFullTime(todo.createdAt / 1000)">
                  {{ formatRelativeTime(todo.createdAt / 1000) }}
                </span>
                <el-tag
                  v-for="tag in todo.tags"
                  :key="tag"
                  size="small"
                  type="info"
                  class="todo-tag"
                >{{ tag }}</el-tag>
              </div>
            </div>
            <div class="todo-actions">
              <el-tooltip content="复制正文" placement="top" :show-after="300">
                <el-button link size="small" data-action="copy" @click="copyText(todo.text)">
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="转提示词填入 AI 对话" placement="top" :show-after="300">
                <el-button link size="small" data-action="to-prompt" @click="toPrompt(todo)">
                  <el-icon><ChatDotRound /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="编辑" placement="top" :show-after="300">
                <el-button link size="small" data-action="edit" @click="startEdit(todo)">
                  <el-icon><Edit /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="删除" placement="top" :show-after="300">
                <el-button link size="small" class="todo-delete-btn" data-action="delete" @click="removeTodo(todo, true)">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </el-tooltip>
            </div>
          </template>
        </div>
      </div>

      <!-- 已完成区：默认折叠，避免归档内容长期占据视线 -->
      <div class="todo-section todo-section--done">
        <div
          class="todo-section-title todo-section-title--toggle"
          @click="showCompleted = !showCompleted"
        >
          <el-icon :size="12"><component :is="showCompleted ? ArrowDown : ArrowRight" /></el-icon>
          已完成
          <span class="todo-count">{{ completedTodos.length }}</span>
        </div>

        <template v-if="showCompleted">
          <div v-if="!completedTodos.length" class="todo-empty">
            还没有已完成的待办
          </div>
          <div
            v-for="todo in completedTodos"
            :key="todo.id"
            class="todo-item todo-item--done"
            :data-todo-id="todo.id"
          >
            <el-checkbox
              :model-value="true"
              class="todo-check"
              :disabled="busyId === todo.id"
              @change="toggleCompleted(todo, false)"
            />
            <div class="todo-main">
              <div
                class="todo-text"
                :class="{ 'is-expanded': expandedIDs.has(todo.id) }"
                @click="toggleExpanded(todo.id)"
              >{{ todo.text }}</div>
              <div class="todo-meta">
                <span class="todo-time" :title="formatFullTime(todo.completedAt / 1000)">
                  完成于 {{ formatRelativeTime(todo.completedAt / 1000) }}
                </span>
                <el-tag
                  v-for="tag in todo.tags"
                  :key="tag"
                  size="small"
                  type="info"
                  class="todo-tag"
                >{{ tag }}</el-tag>
              </div>
            </div>
            <div class="todo-actions">
              <el-tooltip content="复制正文" placement="top" :show-after="300">
                <el-button link size="small" data-action="copy" @click="copyText(todo.text)">
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="转提示词填入 AI 对话" placement="top" :show-after="300">
                <el-button link size="small" data-action="to-prompt" @click="toPrompt(todo)">
                  <el-icon><ChatDotRound /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="删除（已完成的删除不进二次确认）" placement="top" :show-after="300">
                <el-button link size="small" class="todo-delete-btn" data-action="delete" @click="removeTodo(todo, false)">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </el-tooltip>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- 导入预览：先看将导入条数与重复项，确认后由后端一次落盘 -->
    <el-dialog
      v-model="importDialogVisible"
      title="导入待办"
      width="min(640px, 80vw)"
      class="todo-import-dialog"
      @closed="resetImport"
    >
      <div class="import-summary">
        <span>将导入 <b>{{ importWritableCount }}</b> 条</span>
        <span v-if="importSkippedCount" class="import-summary-dup">
          跳过重复 <b>{{ importSkippedCount }}</b> 条
        </span>
        <el-switch
          v-model="importIncludeDuplicates"
          size="small"
          class="import-switch"
          active-text="重复项也导入"
        />
      </div>

      <div class="import-list">
        <div
          v-for="(item, idx) in importPreviewItems"
          :key="idx"
          class="import-item"
          :class="{ 'is-duplicate': item.duplicate && !importIncludeDuplicates }"
        >
          <span class="import-item-time">{{ formatImportTime(item.createdAt) }}</span>
          <span class="import-item-text" :title="item.text">{{ item.text }}</span>
          <el-tag v-if="item.duplicate" size="small" type="warning">已存在</el-tag>
          <el-tag
            v-for="tag in item.tags"
            :key="tag"
            size="small"
            type="info"
            class="todo-tag"
          >{{ tag }}</el-tag>
        </div>
        <div v-if="importTruncated" class="import-more">
          仅展示前 {{ IMPORT_PREVIEW_LIMIT }} 条，确认后按上述规则导入
        </div>
      </div>

      <template #footer>
        <el-button size="small" @click="importDialogVisible = false">取消</el-button>
        <el-button
          size="small"
          type="primary"
          :disabled="!importWritableCount"
          :loading="importing"
          data-action="import-confirm"
          @click="confirmImport"
        >导入 {{ importWritableCount }} 条</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Memo, Plus, CopyDocument, Edit, Delete, ArrowDown, ArrowRight, Download, Upload, ChatDotRound
} from '@element-plus/icons-vue'
import {
  ListTempTodos,
  AddTempTodo,
  AddTempTodoFromClipboard,
  AddTempTodos,
  UpdateTempTodo,
  SetTempTodoCompleted,
  DeleteTempTodo,
  ExportTempTodosMarkdown,
  OpenDirectoryDialog,
  OpenFileDialog,
  ReadFileBytes
} from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { formatRelativeTime, formatFullTime } from '../utils/formatTime'
import { decodeBase64Utf8 } from '../utils/base64'
import { parseTempTodoMarkdown } from '../utils/tempTodoImport'
import { useSettingsStore } from '../store/settings'
import { useUiStore } from '../store/ui'
import { isBrowserChannel } from '../transport'

const settingsStore = useSettingsStore()
const uiStore = useUiStore()

const quickShortcutTip = computed(() => settingsStore.shortcutQuickTodo || 'Ctrl+Alt+N')

const todos = ref([])
const keyword = ref('')
const showCompleted = ref(false)

// 新建/编辑共用一份草稿态：creating=true 表示顶部新建卡片，editingId 非空表示行内编辑
const creating = ref(false)
const editingId = ref('')
const draftText = ref('')
const draftTags = ref('')
const saving = ref(false)
const busyId = ref('')
const createInputRef = ref(null)

// 展开态按条目 ID 记录：长文默认折叠成两行，避免侧边栏被单条占满
const expandedIDs = ref(new Set())

// 导入预览态：解析结果 + 「重复项也导入」开关（默认关，避免制造重复脏数据）。
// 条目多时只渲染前 200 条，避免大文件把对话框撑爆；写入仍按全量。
const IMPORT_PREVIEW_LIMIT = 200
const importDialogVisible = ref(false)
const importParsed = ref(null)
const importIncludeDuplicates = ref(false)
const importing = ref(false)

const importWritableItems = computed(() => {
  const entries = importParsed.value?.entries || []
  return importIncludeDuplicates.value ? entries : entries.filter(item => !item.duplicate)
})
const importWritableCount = computed(() => importWritableItems.value.length)
const importSkippedCount = computed(() =>
  importIncludeDuplicates.value ? 0 : (importParsed.value?.duplicateCount || 0)
)
const importPreviewItems = computed(() =>
  (importParsed.value?.entries || []).slice(0, IMPORT_PREVIEW_LIMIT)
)
const importTruncated = computed(() =>
  (importParsed.value?.entries.length || 0) > IMPORT_PREVIEW_LIMIT
)

const filteredTodos = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return todos.value
  return todos.value.filter(t => {
    if (t.text.toLowerCase().includes(kw)) return true
    return (t.tags || []).some(tag => tag.toLowerCase().includes(kw))
  })
})

const pendingTodos = computed(() => filteredTodos.value.filter(t => !t.completedAt))
const completedTodos = computed(() => filteredTodos.value.filter(t => t.completedAt))

const parseTags = (raw) => (raw || '')
  .split(/[,，]/)
  .map(t => t.trim())
  .filter(Boolean)

const load = async () => {
  try {
    const list = await ListTempTodos()
    // 保留策略（30 天 / 500 条）由后端在 List 时执行并落盘，前端只渲染结果
    todos.value = list || []
  } catch (e) {
    handleError('加载临时待办失败: ', e)
  }
}

// 转提示词：把条目正文经一次性信箱交给 AiChatPanel 填入输入框（切面板由消费侧完成）
const toPrompt = (todo) => {
  uiStore.pushTodoPrompt(todo.text)
}

// 导出 Markdown：桌面模式经原生目录对话框选目录，后端追加写入当天 YYYY-MM-DD.md。
// 浏览器通道不提供（OpenDirectoryDialog 在 serve 模式不可用，后端方法亦被屏蔽）。
const exportTodos = async () => {
  if (!todos.value.length) {
    ElMessage.info('暂无可导出的待办')
    return
  }
  if (isBrowserChannel()) {
    ElMessage.warning('该功能仅桌面模式可用')
    return
  }
  try {
    const dir = await OpenDirectoryDialog('选择待办导出目录')
    if (!dir) return
    const file = await ExportTempTodosMarkdown(dir)
    if (!file) {
      ElMessage.info('暂无可导出的待办')
      return
    }
    ElMessage.success(`已导出到 ${file}`)
  } catch (e) {
    handleError('导出待办失败: ', e)
  }
}

// 导入预览里的时间：0 表示无行内时间，落盘时由后端取导入时刻
const formatImportTime = (createdAt) => {
  if (!createdAt) return '导入时刻'
  const d = new Date(createdAt)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// 导入 Markdown：桌面模式选文件 → 读原始字节还原 UTF-8 → 前端解析 → 打开预览。
// 浏览器通道不提供（文件对话框与本地文件读取均属桌面能力）。
const startImport = async () => {
  if (isBrowserChannel()) {
    ElMessage.warning('该功能仅桌面模式可用')
    return
  }
  try {
    const path = await OpenFileDialog('选择待办 Markdown 文件', [
      { DisplayName: 'Markdown 文件', Pattern: '*.md' }
    ])
    if (!path) return

    const bytes = await ReadFileBytes(path)
    if (bytes?.error) {
      ElMessage.error('读取文件失败: ' + bytes.error)
      return
    }
    if (bytes?.tooLarge) {
      ElMessage.error('文件过大（超 50MB），无法导入')
      return
    }

    const text = bytes?.base64 ? decodeBase64Utf8(bytes.base64) : ''
    const parsed = parseTempTodoMarkdown(text, {
      fileName: path,
      existingTexts: todos.value.map(t => t.text)
    })
    if (!parsed.entries.length) {
      ElMessage.info('未找到可导入的待办行')
      return
    }

    importParsed.value = parsed
    importIncludeDuplicates.value = false
    importDialogVisible.value = true
  } catch (e) {
    handleError('导入待办失败: ', e)
  }
}

const confirmImport = async () => {
  const items = importWritableItems.value.map(item => ({
    text: item.text,
    tags: item.tags,
    createdAt: item.createdAt
  }))
  if (!items.length) return

  importing.value = true
  try {
    const added = await AddTempTodos(items)
    ElMessage.success(`已导入 ${added} 条待办`)
    importDialogVisible.value = false
    // 导入后清空搜索：否则新条目可能被当前关键词过滤掉，用户看不到结果
    keyword.value = ''
    await load()
  } catch (e) {
    handleError('导入待办失败: ', e)
  } finally {
    importing.value = false
  }
}

const resetImport = () => {
  importParsed.value = null
  importIncludeDuplicates.value = false
  importing.value = false
}

const resetDraft = () => {
  creating.value = false
  editingId.value = ''
  draftText.value = ''
  draftTags.value = ''
}

const startCreate = async () => {
  editingId.value = ''
  creating.value = true
  draftText.value = ''
  draftTags.value = ''
  await nextTick()
  createInputRef.value?.focus?.()
}

const startEdit = (todo) => {
  creating.value = false
  editingId.value = todo.id
  draftText.value = todo.text
  draftTags.value = (todo.tags || []).join(', ')
}

const cancelDraft = () => resetDraft()

const saveDraft = async () => {
  const text = draftText.value.trim()
  if (!text) {
    ElMessage.warning('待办内容不能为空')
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await UpdateTempTodo(editingId.value, text, parseTags(draftTags.value))
    } else {
      await AddTempTodo(text, parseTags(draftTags.value))
    }
    resetDraft()
    await load()
  } catch (e) {
    handleError('保存待办失败: ', e)
  } finally {
    saving.value = false
  }
}

const toggleCompleted = async (todo, completed) => {
  busyId.value = todo.id
  try {
    await SetTempTodoCompleted(todo.id, completed)
    await load()
  } catch (e) {
    handleError(completed ? '标记完成失败: ' : '取消完成失败: ', e)
  } finally {
    busyId.value = ''
  }
}

const removeTodo = async (todo, needConfirm) => {
  if (needConfirm) {
    try {
      await ElMessageBox.confirm(
        `确定删除这条待办吗？此操作不可撤销。\n\n${todo.text.slice(0, 80)}`,
        '删除待办',
        { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
      )
    } catch {
      return
    }
  }
  busyId.value = todo.id
  try {
    await DeleteTempTodo(todo.id)
    await load()
  } catch (e) {
    handleError('删除待办失败: ', e)
  } finally {
    busyId.value = ''
  }
}

const copyText = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

const toggleExpanded = (id) => {
  // Set 内部变更不触发响应式，须整体替换引用
  const next = new Set(expandedIDs.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedIDs.value = next
}

/**
 * 剪贴板快速记录（全局快捷键入口，由 Home.vue 经 ref 调用）。
 * 成功后不切换面板、不抢焦点：用户此时多半正看着 AI 输出，打断视线比「看不见结果」更烦。
 * 预览取正文首个非空行前 20 字，用来确认记的是不是刚复制的那段。
 */
const quickAddFromClipboard = async () => {
  saving.value = true
  try {
    const todo = await AddTempTodoFromClipboard()
    const firstLine = (todo?.text || '').split('\n').find(line => line.trim()) || ''
    const preview = firstLine.length > 20 ? `${firstLine.slice(0, 20)}…` : firstLine
    ElMessage.success(preview ? `已记录：${preview}` : '已记录到临时待办')
    await load()
  } catch (e) {
    handleError('快速记录失败: ', e)
  } finally {
    saving.value = false
  }
}

defineExpose({ quickAddFromClipboard })

// 跨组件写入（AI 回复 / 代码预览右键「存为待办」）后重拉：面板以 v-show 常驻，
// onMounted 只加载一次，需依赖该计数器感知外部变更。
watch(() => uiStore.tempTodoChangeSeq, () => {
  load()
})

onMounted(() => {
  load()
})
</script>

<style scoped>
/* 根元素：占满 Home 主区上半区（与三栏区互斥），高度约束链从此起 */
.temp-todo-panel {
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
  padding: var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

.panel-heading {
  display: flex;
  align-items: baseline;
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
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.subtitle-sign {
  margin-right: var(--spacing-xs);
  color: var(--primary-color);
  font-weight: 600;
}

.panel-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-shrink: 0;
}

.todo-search {
  width: 200px;
}

.btn-icon {
  margin-right: var(--spacing-xs);
}

.panel-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-md) var(--spacing-md) var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

/* ---- 区块 ---- */
.todo-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.todo-section-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  letter-spacing: 0.02em;
}

.todo-section-title--toggle {
  cursor: pointer;
  user-select: none;
}

.todo-section-title--toggle:hover {
  color: var(--text-primary);
}

.todo-count {
  /* 徽章类微元素自身内边距允许字面像素特例（对齐 SettingsPanel kbd 先例） */
  padding: 0 6px;
  border-radius: var(--radius-md);
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
  font-weight: 500;
}

.todo-empty {
  padding: var(--spacing-lg);
  border: 1px dashed var(--border-color);
  border-radius: var(--radius-md);
  text-align: center;
  font-size: 12px;
  color: var(--text-tertiary);
}

/* ---- 条目 ---- */
.todo-item {
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: border-color var(--transition-fast), background var(--transition-fast);
}

.todo-item:hover {
  border-color: var(--primary-color);
}

.todo-item--done {
  background: var(--bg-tertiary);
}

.todo-item--done .todo-text {
  color: var(--text-tertiary);
}

.todo-check {
  flex-shrink: 0;
  margin-top: 2px;
}

.todo-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.todo-text {
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-word;
  cursor: pointer;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
}

.todo-text.is-expanded {
  display: block;
  -webkit-line-clamp: unset;
  overflow: visible;
}

.todo-meta {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  font-size: 11px;
  color: var(--text-tertiary);
}

.todo-time {
  white-space: nowrap;
}

.todo-tag {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.todo-actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  opacity: 0;
  transition: opacity var(--transition-fast);
}

.todo-item:hover .todo-actions {
  opacity: 1;
}

.todo-delete-btn:hover {
  color: var(--el-color-danger);
}

/* ---- 编辑器 ---- */
.todo-editor {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
}

.todo-editor--inline {
  padding: 0;
  border: none;
  background: transparent;
}

.todo-editor-footer {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.todo-tags-input {
  flex: 1;
  min-width: 0;
}

.todo-editor-actions {
  display: flex;
  gap: var(--spacing-xs);
  flex-shrink: 0;
}

/* ---- 导入预览 ---- */
.import-summary {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
  font-size: 13px;
  color: var(--text-secondary);
}

.import-summary b {
  color: var(--text-primary);
}

.import-summary-dup {
  color: var(--el-color-warning);
}

.import-switch {
  margin-left: auto;
}

.import-list {
  max-height: 360px;
  overflow-y: auto;
  padding: var(--spacing-sm);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
}

.import-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: 13px;
  border-radius: var(--radius-sm);
}

.import-item.is-duplicate {
  opacity: 0.55;
}

.import-item-time {
  flex: none;
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.import-item-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
}

.import-more {
  padding: var(--spacing-sm);
  text-align: center;
  font-size: 12px;
  color: var(--text-tertiary);
}
</style>
