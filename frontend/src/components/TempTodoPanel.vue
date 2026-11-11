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
          <kbd>{{ quickShortcutTip }}</kbd>
          从剪贴板记一条
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
        <el-tooltip content="导出为 Markdown" placement="bottom" :show-after="300">
          <el-button
            size="small"
            data-action="export"
            aria-label="导出待办"
            :disabled="!todos.length"
            @click="exportTodos"
          >
            <el-icon><Download /></el-icon>
          </el-button>
        </el-tooltip>
        <el-tooltip content="从 Markdown 导入" placement="bottom" :show-after="300">
          <el-button size="small" data-action="import" aria-label="导入待办" @click="startImport">
            <el-icon><Upload /></el-icon>
          </el-button>
        </el-tooltip>
        <el-button type="primary" size="small" data-action="create" :disabled="creating" @click="startCreate">
          <el-icon class="btn-icon"><Plus /></el-icon>
          新建
        </el-button>
      </div>
    </div>

    <div ref="panelBodyRef" class="panel-body">
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
          <span class="todo-editor-hint">Ctrl+Enter 保存</span>
          <div class="todo-editor-actions">
            <el-button size="small" data-action="expand-editor" @click="openEditorDialog">
              <el-icon class="btn-icon"><FullScreen /></el-icon>
              全屏编辑
            </el-button>
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
                  <el-button size="small" data-action="expand-editor" @click="openEditorDialog">
                    <el-icon class="btn-icon"><FullScreen /></el-icon>
                    全屏编辑
                  </el-button>
                  <el-button size="small" @click="cancelDraft">取消</el-button>
                  <el-button size="small" type="primary" :loading="saving" @click="saveDraft">保存</el-button>
                </div>
              </div>
            </div>
          </template>

          <template v-else>
            <div class="todo-main">
              <div
                class="todo-text-wrap"
                :class="{ 'has-overflow': overflowIDs.has(todo.id) && !expandedIDs.has(todo.id) }"
              >
                <div
                  class="todo-text"
                  :class="{ 'is-expanded': expandedIDs.has(todo.id) }"
                  :ref="setTextEl(todo.id)"
                >{{ todo.text }}</div>
              </div>
              <el-button
                v-if="overflowIDs.has(todo.id)"
                link
                size="small"
                class="todo-expand-btn"
                data-action="toggle-expand"
                @click="toggleExpanded(todo.id)"
              >
                {{ expandedIDs.has(todo.id) ? '收起' : '展开' }}
                <el-icon class="todo-expand-icon">
                  <component :is="expandedIDs.has(todo.id) ? ArrowUp : ArrowDown" />
                </el-icon>
              </el-button>
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
              </div>
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
                class="todo-text-wrap"
                :class="{ 'has-overflow': overflowIDs.has(todo.id) && !expandedIDs.has(todo.id) }"
              >
                <div
                  class="todo-text"
                  :class="{ 'is-expanded': expandedIDs.has(todo.id) }"
                  :ref="setTextEl(todo.id)"
                >{{ todo.text }}</div>
              </div>
              <el-button
                v-if="overflowIDs.has(todo.id)"
                link
                size="small"
                class="todo-expand-btn"
                data-action="toggle-expand"
                @click="toggleExpanded(todo.id)"
              >
                {{ expandedIDs.has(todo.id) ? '收起' : '展开' }}
                <el-icon class="todo-expand-icon">
                  <component :is="expandedIDs.has(todo.id) ? ArrowUp : ArrowDown" />
                </el-icon>
              </el-button>
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
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- 全屏编辑：行内三行输入区的放大视图，与行内编辑共用同一份草稿。
         X / ESC / 返回编辑只关闭弹窗、草稿保留；仅保存才落库并退出编辑态。 -->
    <el-dialog
      v-model="editorDialogVisible"
      :title="editingId ? '编辑待办' : '新建待办'"
      fullscreen
      class="todo-editor-dialog"
      :close-on-click-modal="false"
      @opened="focusEditorDialog"
      @closed="onEditorDialogClosed"
    >
      <div class="editor-dialog-body">
        <div class="editor-dialog-field">
          <el-input
            ref="dialogInputRef"
            v-model="draftText"
            type="textarea"
            resize="none"
            placeholder="要记的事、提示词片段或待验证的想法（Ctrl+Enter 保存）"
            class="editor-dialog-text"
            @keydown.ctrl.enter.prevent="saveFromDialog"
          />
        </div>
        <div class="editor-dialog-meta">
          <el-input
            v-model="draftTags"
            size="small"
            placeholder="标签，用逗号分隔（如 重构,AI提示词）"
            class="editor-dialog-tags"
          />
          <span class="editor-dialog-hint">Ctrl+Enter 保存</span>
        </div>
      </div>

      <template #footer>
        <el-button size="small" data-action="editor-back" @click="editorDialogVisible = false">返回编辑</el-button>
        <el-button
          size="small"
          type="primary"
          :loading="saving"
          data-action="editor-save"
          @click="saveFromDialog"
        >保存</el-button>
      </template>
    </el-dialog>

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
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Memo, Plus, CopyDocument, Edit, Delete, ArrowDown, ArrowUp, ArrowRight, Download, Upload, ChatDotRound, FullScreen
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
import { isTextOverflowing } from '../utils/todoTextOverflow'
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

// 全屏编辑弹窗：与行内编辑共用 draftText / draftTags，本身只是「放大视图」，
// 因此不复制草稿；弹窗关闭时是否复位编辑态由 editorDialogSaveDone 区分。
const editorDialogVisible = ref(false)
const dialogInputRef = ref(null)
const editorDialogSaveDone = ref(false)

// 展开态按条目 ID 记录：长文默认折叠成三行，展开后限高内部滚动
const expandedIDs = ref(new Set())

// 截断判定结果：只有被截断的条目才显示「展开」按钮。
// 面板 v-show 隐藏时 clientHeight 为 0 无法判定，切回可见后由 refreshOverflow 重算
const overflowIDs = ref(new Set())
const textEls = new Map()
const panelBodyRef = ref(null)
let resizeObserver = null

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

// 全屏编辑不复制草稿：弹窗内外的 textarea 绑定同一份 draftText / draftTags
const openEditorDialog = () => {
  editorDialogSaveDone.value = false
  editorDialogVisible.value = true
}

const focusEditorDialog = () => {
  dialogInputRef.value?.focus?.()
}

// 关闭弹窗的默认语义是回到原编辑态（草稿保留）；仅「保存」成功才复位草稿并退出编辑
const onEditorDialogClosed = () => {
  if (!editorDialogSaveDone.value) return
  editorDialogSaveDone.value = false
  resetDraft()
}

// 落库只做写入与错误处理，不复位草稿：行内保存与全屏保存的退出时机不同
const persistDraft = async () => {
  const text = draftText.value.trim()
  if (!text) {
    ElMessage.warning('待办内容不能为空')
    return false
  }
  saving.value = true
  try {
    if (editingId.value) {
      await UpdateTempTodo(editingId.value, text, parseTags(draftTags.value))
    } else {
      await AddTempTodo(text, parseTags(draftTags.value))
    }
    return true
  } catch (e) {
    handleError('保存待办失败: ', e)
    return false
  } finally {
    saving.value = false
  }
}

const saveDraft = async () => {
  if (!(await persistDraft())) return false
  resetDraft()
  await load()
  return true
}

// 全屏保存：先关弹窗（渐隐期间正文仍在），@closed 后再复位编辑态，避免弹窗内容瞬间清空
const saveFromDialog = async () => {
  if (!(await persistDraft())) return
  if (!editorDialogVisible.value) {
    // 保存请求在途时用户已按 ESC / X 关闭弹窗：@closed 早于此处触发且当时未标记保存完成，
    // 不会再复位编辑态，就地复位，避免残留草稿被再次保存出新条目
    resetDraft()
    await load()
    return
  }
  editorDialogSaveDone.value = true
  editorDialogVisible.value = false
  await load()
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

// 正文元素注册：ref 回调在卸载时传 null，须同步移出集合，避免测量到已销毁节点
const setTextEl = (id) => (el) => {
  if (el) textEls.set(id, el)
  else textEls.delete(id)
}

const sameSet = (a, b) => a.size === b.size && [...a].every(item => b.has(item))

/**
 * 重算「哪些条目被折叠截断」。
 * 展开态不参与重新测量：此时可见高度由 max-height 限制，再按 scrollHeight
 * 比较会把长文持续判为截断；能展开本身说明历史上已被判定过
 */
const refreshOverflow = () => {
  const next = new Set()
  for (const [id, el] of textEls) {
    if (expandedIDs.value.has(id)) {
      if (overflowIDs.value.has(id)) next.add(id)
      continue
    }
    if (isTextOverflowing(el)) next.add(id)
  }
  if (!sameSet(next, overflowIDs.value)) overflowIDs.value = next
}

const detachResizeObserver = () => {
  resizeObserver?.disconnect()
  resizeObserver = null
}

// 面板常驻 v-show：隐藏时观察器得到 0 宽直接跳过，切回可见由
// activePanel 的 watch 重新测量并重建观察器，避免隐藏期间空转
const attachResizeObserver = () => {
  if (resizeObserver || typeof ResizeObserver === 'undefined') return
  const target = panelBodyRef.value
  if (!target) return
  resizeObserver = new ResizeObserver(() => {
    if (!target.clientWidth) return
    requestAnimationFrame(refreshOverflow)
  })
  resizeObserver.observe(target)
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

// 列表或搜索条件变化后重测截断（flush post 保证 DOM 已更新）
watch([() => todos.value, keyword, expandedIDs], () => {
  nextTick(refreshOverflow)
}, { flush: 'post' })

// 面板可见性切换：隐藏期间无法判定截断，切回待办时重测并挂载宽度观察器
watch(() => uiStore.activePanel, (panel) => {
  if (panel === 'todo') {
    nextTick(() => {
      refreshOverflow()
      attachResizeObserver()
    })
  } else {
    detachResizeObserver()
  }
})

onBeforeUnmount(detachResizeObserver)

onMounted(async () => {
  await load()
  if (uiStore.activePanel === 'todo') {
    await nextTick()
    refreshOverflow()
    attachResizeObserver()
  }
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
  /* 内容限宽 1000px 居中、背景铺满：calc 为负时由 max 兜底为常规内边距 */
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

.panel-subtitle kbd {
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  font-size: 11px;
  padding: 2px 6px;
  border: 1px solid var(--border-light);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.panel-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex: 0 1 auto;
  min-width: 0;
}

.todo-search {
  flex: 0 1 240px;
  min-width: 160px;
}

/* 窄窗口优先隐藏快捷键提示，把横向空间让给搜索框与主操作 */
@media (max-width: 900px) {
  .panel-subtitle {
    display: none;
  }
}

.btn-icon {
  margin-right: var(--spacing-xs);
}

.panel-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  /* 与 header 同一限宽基线：滚动条仍贴面板右缘 */
  padding: var(--spacing-md) max(var(--spacing-md), calc((100% - 1000px) / 2)) var(--spacing-lg);
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
  /* 正文底部渐隐用卡片自身底色，已完成条目换档后渐变自然跟随 */
  --todo-card-bg: var(--bg-secondary);
  display: flex;
  align-items: flex-start;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-md);
  background: var(--todo-card-bg);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

/* 悬停只轻提边框与阴影，不整圈主色填充 */
.todo-item:hover {
  border-color: var(--border-color);
  box-shadow: var(--shadow-sm);
}

.todo-item--done {
  --todo-card-bg: var(--bg-tertiary);
}

.todo-item--done .todo-text {
  color: var(--text-secondary);
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

/* 正文：默认折叠三行；点击不再切换展开，避免与「选中文字复制」冲突 */
.todo-text-wrap {
  position: relative;
}

/* 只在确实被截断且未展开时显示底部渐隐 */
.todo-text-wrap.has-overflow::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 20px;
  background: linear-gradient(to bottom, transparent, var(--todo-card-bg));
  pointer-events: none;
}

.todo-text {
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-word;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  overflow: hidden;
}

/* 展开后限高内部滚动：一条长日志不会把整个列表撑开 */
.todo-text.is-expanded {
  display: block;
  -webkit-line-clamp: unset;
  overflow-y: auto;
  max-height: 50vh;
}

.todo-expand-btn {
  align-self: flex-start;
  padding: 0;
  font-size: 12px;
  color: var(--primary-color);
}

.todo-expand-icon {
  margin-left: var(--spacing-xs);
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

/* 操作图标常驻 meta 行右端：默认低对比度，悬停卡片或图标本身时提亮 */
.todo-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.todo-actions .el-button {
  padding: var(--spacing-xs);
  height: auto;
  color: var(--text-tertiary);
}

.todo-item:hover .todo-actions .el-button {
  color: var(--text-secondary);
}

.todo-actions .el-button:hover {
  color: var(--text-primary);
}

.todo-actions .todo-delete-btn:hover {
  color: var(--el-color-danger);
}

/* ---- 编辑器 ---- */
/* 新建卡片：主色左边条 + 浅主色底，与列表卡片区分开 */
.todo-editor {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  width: 100%;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-light);
  border-left: 3px solid var(--primary-color);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--primary-color) 5%, var(--bg-secondary));
}

/* 行内编辑退化为无壳，避免卡片里再套一层卡片 */
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

.todo-editor-hint {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--text-tertiary);
}

.todo-editor-actions {
  display: flex;
  gap: var(--spacing-xs);
  flex-shrink: 0;
}

@media (max-width: 900px) {
  .todo-editor-hint {
    display: none;
  }
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

<!-- 全屏编辑弹窗外壳与内部结构由 Element Plus 渲染，scoped 无法稳定命中其子节点；
     采用非 scoped 块 + 专属 class 圈定（对齐 AiChatPanel 的 tpl-manage-dialog 先例）。 -->
<style>
/* EP 2.14.5 容器自带 padding，标题栏/底栏背景要贴边必须归零（同 tpl-manage-dialog 先例） */
.el-dialog.todo-editor-dialog {
  padding: 0;
}

.el-dialog.todo-editor-dialog.is-fullscreen {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg-primary);
}

.el-dialog.todo-editor-dialog .el-dialog__title {
  color: var(--text-primary);
  font-size: 16px;
  font-weight: 600;
}

.el-dialog.todo-editor-dialog .el-dialog__headerbtn .el-dialog__close {
  color: var(--text-tertiary);
}

.el-dialog.todo-editor-dialog .el-dialog__headerbtn:hover .el-dialog__close {
  color: var(--text-primary);
}

.el-dialog.todo-editor-dialog .el-dialog__header {
  flex-shrink: 0;
  margin-right: 0;
  padding: var(--spacing-md) var(--spacing-lg);
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-secondary);
}

.el-dialog.todo-editor-dialog .el-dialog__body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: var(--spacing-lg);
}

.el-dialog.todo-editor-dialog .el-dialog__footer {
  flex-shrink: 0;
  padding: var(--spacing-md) var(--spacing-lg);
  border-top: 1px solid var(--border-color);
  background: var(--bg-secondary);
}

/* 内部结构规则统一圈定在专属弹窗 class 下，避免非 scoped 块污染全局命名空间 */
.el-dialog.todo-editor-dialog .editor-dialog-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.el-dialog.todo-editor-dialog .editor-dialog-field {
  flex: 1;
  min-height: 0;
  display: flex;
}

.el-dialog.todo-editor-dialog .editor-dialog-text {
  flex: 1;
  min-height: 0;
  display: flex;
}

/* 输入区内层 textarea 撑满剩余高度：行内三行 → 全屏整页的差异主要在这一条 */
.el-dialog.todo-editor-dialog .editor-dialog-text .el-textarea__inner {
  flex: 1;
  min-height: 0;
  height: auto;
  resize: none;
  font-family: inherit;
  font-size: 14px;
  line-height: 1.7;
}

.el-dialog.todo-editor-dialog .editor-dialog-meta {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.el-dialog.todo-editor-dialog .editor-dialog-tags {
  flex: 1;
  min-width: 0;
}

.el-dialog.todo-editor-dialog .editor-dialog-hint {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}
</style>
