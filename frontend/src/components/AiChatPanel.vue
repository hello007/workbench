<template>
  <div class="ai-chat-panel">
    <!-- 顶部标题栏：风格对齐 AiFunctionPanel 的 panel-header（渐变背景 + 主色标题图标） -->
    <div class="panel-header">
      <div class="panel-heading">
        <span class="panel-title">
          <el-icon :size="18" class="panel-title-icon"><ChatDotRound /></el-icon>
          AI 对话
        </span>
        <span class="panel-subtitle"><span class="subtitle-sign">&gt;</span>常用目录 · 多会话持续对话</span>
      </div>
      <span class="panel-actions">
        <el-button size="small" @click="showAddDialog">
          <el-icon class="panel-btn-icon"><Plus /></el-icon>添加目录
        </el-button>
      </span>
    </div>

    <div class="chat-layout">
      <!-- 左：常用目录栏（拖拽排序 + 点击选中 + hover 移除） -->
      <div class="chat-dirs">
        <div class="chat-dirs-caption">
          <span>常用目录</span>
          <span v-if="aiChatStore.chatDirectories.length" class="chat-dirs-count">{{ aiChatStore.chatDirectories.length }}</span>
        </div>
        <div class="chat-dirs-list">
          <VueDraggable
            v-model="localDirs"
            :animation="200"
            ghost-class="chat-dir-item--ghost"
            :prevent-on-filter="false"
            @end="onDragEnd"
          >
            <div
              v-for="dir in localDirs"
              :key="dir.id"
              class="chat-dir-item"
              :class="{ 'is-active': dir.id === aiChatStore.selectedChatDirectoryId }"
              @click="handleSelectDirectory(dir)"
            >
              <div class="chat-dir-row">
                <el-icon class="chat-dir-icon"><Folder /></el-icon>
                <span class="chat-dir-name" :title="dir.displayName">{{ dir.displayName }}</span>
                <el-icon class="chat-dir-edit" title="重命名" @click.stop="showRenameDialog(dir)"><Edit /></el-icon>
                <el-icon class="chat-dir-remove" title="移除" @click.stop="handleRemove(dir)"><Delete /></el-icon>
              </div>
              <div class="chat-dir-path" :title="dir.path">{{ shortenPath(dir.path) }}</div>
            </div>
          </VueDraggable>
          <el-empty
            v-if="!aiChatStore.chatDirectories.length"
            class="chat-dirs-empty"
            description="暂无常用目录"
            :image-size="60"
          />
        </div>
        <div v-if="!aiChatStore.chatDirectories.length" class="chat-dirs-hint">
          可右键工作目录或文件树目录「添加到 AI 对话」
        </div>
      </div>

      <!-- 右：对话区（上下分栏：会话工具条 + 消息历史 + 输入区） -->
      <div class="chat-main">
        <!-- 未选目录：占位引导 -->
        <el-empty
          v-if="!aiChatStore.selectedChatDirectory"
          class="chat-main-empty"
          description="从左侧选择目录开始对话"
        />
        <template v-else>
          <!-- 会话工具条：会话下拉（类 /resume）+ 新建 + 权限模式 + 模型 -->
          <div class="chat-toolbar">
            <el-dropdown trigger="click" @command="onSessionCommand">
              <el-button size="small" class="session-btn" :title="currentSessionTitle">
                <el-icon class="session-btn-icon"><ChatLineRound /></el-icon>
                <span class="session-btn-label">{{ currentSessionTitle }}</span>
                <el-icon class="session-btn-arrow"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu class="session-menu">
                  <el-dropdown-item v-if="!aiChatStore.chatSessions.length" disabled>
                    暂无会话
                  </el-dropdown-item>
                  <el-dropdown-item
                    v-for="s in aiChatStore.chatSessions"
                    :key="s.id"
                    :command="s.id"
                    :class="{ 'is-current': s.id === aiChatStore.selectedChatSessionId }"
                  >
                    <span class="session-menu-title">{{ s.title }}</span>
                    <span class="session-menu-time">{{ formatSessionTime(s.updatedAt) }}</span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    v-if="aiChatStore.selectedChatSessionId"
                    divided
                    command="__delete_session__"
                    class="session-menu-delete"
                  >
                    删除当前会话
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button size="small" @click="handleNewSession">
              <el-icon class="panel-btn-icon"><Plus /></el-icon>新建会话
            </el-button>

            <span class="chat-toolbar-spacer"></span>

            <!-- 权限模式：中文标签下拉，说明见 tooltip；bypassPermissions 选中时警告提示 -->
            <el-tooltip :content="permissionModeTip" placement="top">
              <el-select
                v-model="aiChatStore.chatSettings.permissionMode"
                size="small"
                class="chat-config-select"
                @change="onPermissionModeChange"
              >
                <el-option
                  v-for="m in PERMISSION_MODES"
                  :key="m.value"
                  :value="m.value"
                  :label="m.label"
                />
              </el-select>
            </el-tooltip>
            <!-- 模型：常用选项 + allow-create 自定义输入；空值 = claude 默认模型 -->
            <el-select
              v-model="aiChatStore.chatSettings.modelName"
              size="small"
              class="chat-config-select chat-config-model"
              filterable
              allow-create
              default-first-option
              placeholder="默认模型"
              @change="persistChatSettings"
            >
              <el-option
                v-for="m in MODEL_OPTIONS"
                :key="m.value"
                :value="m.value"
                :label="m.label"
              />
            </el-select>
          </div>

          <!-- 消息历史区：气泡列表 + markdown 渲染 + 流式增量（scroll 驱动跟底状态） -->
          <div
            ref="messagesEl"
            class="chat-messages"
            :class="{ 'chat-in-flight': aiChatStore.chatInFlight }"
            @scroll="onMessagesScroll"
          >
            <div v-if="!displayMessages.length" class="chat-messages-empty">
              <el-empty
                v-if="!aiChatStore.chatSessions.length"
                description="该目录暂无会话，点击「新建会话」开始"
                :image-size="60"
              />
              <el-empty v-else description="输入需求开始对话" :image-size="60" />
            </div>
            <div
              v-for="(msg, i) in displayMessages"
              :key="`${msg.taskId || 'msg'}-${i}`"
              class="chat-msg-row"
              :class="msg.role"
            >
              <div class="chat-msg-bubble" :class="{ 'is-streaming': msg.streaming }">
                <!-- assistant：markdown 渲染（html:false 防 XSS；点击拦截防 webview 导航） -->
                <!-- assistant：markdown 渲染（html:false 防 XSS；点击拦截防 webview 导航；
                     data-task-id 供选择题卡片提交回填已答标记） -->
                <div
                  v-if="msg.role === 'assistant'"
                  class="chat-msg-md"
                  :data-task-id="msg.taskId || ''"
                  v-html="renderMsgMarkdown(msg)"
                  @click="onChatMarkdownClick"
                ></div>
                <div v-else class="chat-msg-text">{{ msg.content }}</div>
              </div>
            </div>
          </div>

          <!-- 输入区：模板下拉 + textarea（Enter 发送 / Shift+Enter 换行）+ 发送/停止 -->
          <div class="chat-input-area">
            <div class="chat-input-row">
              <el-dropdown trigger="click" @command="applyTemplate">
                <el-button size="small" text bg class="tpl-btn" title="插入模板">
                  <el-icon><Document /></el-icon>模板
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu class="tpl-menu">
                    <el-dropdown-item v-if="!aiChatStore.chatTemplates.length" disabled>
                      暂无模板
                    </el-dropdown-item>
                    <template v-if="aiChatStore.chatTemplateGroups.directory.length">
                      <el-dropdown-item disabled class="tpl-group-title">本目录模板</el-dropdown-item>
                      <el-dropdown-item
                        v-for="t in aiChatStore.chatTemplateGroups.directory"
                        :key="t.id"
                        :command="t.id"
                        :title="t.content"
                      >
                        {{ t.name }}
                      </el-dropdown-item>
                    </template>
                    <template v-if="aiChatStore.chatTemplateGroups.global.length">
                      <el-dropdown-item divided disabled class="tpl-group-title">全局模板</el-dropdown-item>
                      <el-dropdown-item
                        v-for="t in aiChatStore.chatTemplateGroups.global"
                        :key="t.id"
                        :command="t.id"
                        :title="t.content"
                      >
                        {{ t.name }}
                      </el-dropdown-item>
                    </template>
                    <el-dropdown-item divided command="__manage_templates__" class="tpl-manage-item">
                      <el-icon><Setting /></el-icon>管理模板
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
              <!-- 输入框：固定 rows 默认 6 行 + 原生纵向拖拽调高（CSS 限幅）。
                   不用 autosize：其 JS 接管高度会覆盖拖拽结果，与原生 resize 冲突 -->
              <el-input
                ref="inputRef"
                v-model="inputText"
                type="textarea"
                class="chat-input"
                :rows="6"
                placeholder="输入需求，Enter 发送 / Shift+Enter 换行"
                @keydown.enter="onInputEnter"
              />
              <div class="chat-input-actions">
                <el-button
                  v-if="!aiChatStore.chatInFlight"
                  type="primary"
                  class="chat-send-btn"
                  @click="handleSend"
                >
                  <el-icon><Position /></el-icon>发送
                </el-button>
                <el-button v-else type="danger" plain class="chat-stop-btn" @click="handleStop">
                  <el-icon><VideoPause /></el-icon>停止
                </el-button>
              </div>
            </div>
            <div v-if="aiChatStore.chatInFlight" class="chat-input-status">
              <span class="chat-input-status-dot"></span>
              {{ aiChatStore.chatTask?.status === 'queued' ? '排队等待中…' : '回复生成中…' }}
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- 添加目录对话框：路径手输（对齐工作目录添加模式，后端校验存在性）+ 显示名留空取目录名 -->
    <el-dialog v-model="addDialogVisible" title="添加 AI 对话目录" width="480px" append-to-body>
      <el-form label-width="80px">
        <el-form-item label="目录路径">
          <el-input ref="addPathInputRef" v-model="addForm.path" placeholder="例如: D:\workspace\demo" />
        </el-form-item>
        <el-form-item label="显示名">
          <el-input v-model="addForm.displayName" placeholder="留空则取目录名，例如: 项目管理" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="handleAdd">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重命名对话框：改显示名（仅侧栏展示，不影响路径与会话） -->
    <el-dialog v-model="renameDialogVisible" title="重命名目录" width="420px" append-to-body>
      <el-form label-width="80px">
        <el-form-item label="当前路径">
          <el-input :model-value="renameTarget ? shortenPath(renameTarget.path) : ''" disabled />
        </el-form-item>
        <el-form-item label="显示名">
          <el-input
            ref="renameInputRef"
            v-model="renameName"
            placeholder="请输入显示名"
            :disabled="renameLoading"
            @keyup.enter="handleRename"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="renameDialogVisible = false" :disabled="renameLoading">取消</el-button>
        <el-button type="primary" :loading="renameLoading" @click="handleRename">确定</el-button>
      </template>
    </el-dialog>

    <!-- 模板管理弹窗：列表 + 增删改表单（name/content/scope），
         尺寸与布局对齐设置弹窗（宽 min(960px, 86vw) 分档 + nav/content 二分，见 .tpl-manage-body） -->
    <el-dialog
      v-model="tplDialogVisible"
      title="管理对话模板"
      width="min(960px, 86vw)"
      class="tpl-manage-dialog"
      append-to-body
    >
      <div class="tpl-manage-body">
        <div class="tpl-list">
          <div class="tpl-item tpl-item-new" @click="resetTplForm">
            <el-icon><Plus /></el-icon>
            <span>新增模板</span>
          </div>
          <div
            v-for="t in aiChatStore.chatTemplates"
            :key="t.id"
            class="tpl-item"
            :class="{ 'is-active': tplForm.id === t.id }"
            @click="editTpl(t)"
          >
            <span class="tpl-item-name" :title="t.content">{{ t.name }}</span>
            <el-tag size="small" :type="t.scope === 'directory' ? 'primary' : 'info'">
              {{ t.scope === 'directory' ? '本目录' : '全局' }}
            </el-tag>
            <el-icon class="tpl-item-remove" title="删除" @click.stop="handleRemoveTemplate(t)"><Delete /></el-icon>
          </div>
          <el-empty
            v-if="!aiChatStore.chatTemplates.length"
            class="tpl-list-empty"
            description="暂无模板"
            :image-size="48"
          />
        </div>
        <el-form class="tpl-form" label-width="52px">
          <el-form-item label="名称">
            <el-input v-model="tplForm.name" placeholder="模板名称" />
          </el-form-item>
          <el-form-item label="归属">
            <el-radio-group v-model="tplForm.scope" :disabled="!!tplForm.id">
              <el-radio value="directory">本目录</el-radio>
              <el-radio value="global">全局</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="内容">
            <!-- 正文随弹窗增高加大：默认 12 行 + 原生纵向拖拽调高（CSS 限幅，同 chat-input 风格） -->
            <el-input
              v-model="tplForm.content"
              type="textarea"
              class="tpl-content-input"
              :rows="12"
              placeholder="模板正文（纯文本，点击模板整段填入输入框可再修改）"
            />
          </el-form-item>
          <div class="tpl-form-actions">
            <el-button @click="resetTplForm">重置</el-button>
            <el-button type="primary" :loading="tplSaving" @click="handleSaveTemplate">
              {{ tplForm.id ? '保存修改' : '新增' }}
            </el-button>
          </div>
        </el-form>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ChatDotRound, Plus, Folder, Delete, Edit,
  ArrowDown, ChatLineRound, Document, Setting, Position, VideoPause
} from '@element-plus/icons-vue'
import { VueDraggable } from 'vue-draggable-plus'
import { useAiChatStore, useUiStore } from '../store'
import { handleError } from '../utils/error'
import { shortenPath } from '../utils/pathFormat'
import { renderChatMarkdown } from '../utils/chatMarkdown'
import { EventsOn, BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const aiChatStore = useAiChatStore()
const uiStore = useUiStore()

// ===== 常量选项 =====

// 权限模式可选集（值与 model.ChatPermissionMode* 常量对齐）
const PERMISSION_MODES = [
  { value: 'default', label: '默认权限', tip: 'claude 默认权限行为，敏感操作需逐次确认' },
  { value: 'acceptEdits', label: '自动接受编辑', tip: '自动接受文件编辑，其余操作仍需确认' },
  { value: 'plan', label: '规划模式', tip: '仅分析与规划方案，不执行修改' },
  { value: 'bypassPermissions', label: '跳过确认', tip: '跳过全部权限确认，AI 可直接修改代码与执行命令（高危）' }
]
const permissionModeTip = computed(() => {
  const cur = PERMISSION_MODES.find(m => m.value === aiChatStore.chatSettings.permissionMode)
  return cur ? cur.tip : PERMISSION_MODES[0].tip
})

// 模型下拉常用项（MVP 仅 claude 系；allow-create 支持自定义输入，空值 = claude 默认模型）
const MODEL_OPTIONS = [
  { value: '', label: '默认模型' },
  { value: 'sonnet', label: 'Sonnet' },
  { value: 'opus', label: 'Opus' },
  { value: 'haiku', label: 'Haiku' }
]

// --- 本地目录列表（可变，用于拖拽） ---
// localDirs 为 VueDraggable v-model 的可变副本（参照 DirectoryTree 模式）：
// 拖拽原地重排 + onDragEnd 取序持久化；store 重载后同步覆盖。
const localDirs = ref([...aiChatStore.chatDirectories])
watch(() => aiChatStore.chatDirectories, (val) => {
  localDirs.value = [...val]
})

// 拖拽结束：按新序持久化（失败回滚本地顺序并提示，防 UI 停留在未保存的拖拽结果）
const onDragEnd = async () => {
  const ids = localDirs.value.map(d => d.id)
  try {
    await aiChatStore.reorderChatDirectories(ids)
  } catch (error) {
    localDirs.value = [...aiChatStore.chatDirectories]
    handleError('排序保存失败: ', error)
  }
}

// --- 在途任务时切换目录/会话/新建/删除的统一确认（继续则取消当前任务） ---
// actionLabel 描述后续动作（切换/新建/删除），确认取消任务后继续；取消失败则中止，
// 防承诺「取消任务并继续」时带着仍在运行的任务继续。
const confirmDiscardInFlight = async (actionLabel) => {
  if (!aiChatStore.chatInFlight) return true
  try {
    await ElMessageBox.confirm(
      `当前有对话进行中，${actionLabel}将取消该任务。确定${actionLabel}吗？`,
      `${actionLabel}确认`,
      { confirmButtonText: `取消任务并${actionLabel}`, cancelButtonText: '留在当前', type: 'warning' }
    )
  } catch {
    return false
  }
  try {
    await aiChatStore.cancelChatTask()
  } catch (error) {
    handleError('取消任务失败: ', error)
    return false
  }
  return true
}

// --- 侧栏目录点击（在途任务先确认） ---
const handleSelectDirectory = async (dir) => {
  if (dir.id === aiChatStore.selectedChatDirectoryId) return
  if (!(await confirmDiscardInFlight('切换'))) return
  aiChatStore.selectChatDirectory(dir.id)
}

// 目录切换后：加载该目录会话列表（默认选中最近更新）+ 该目录模板
watch(() => aiChatStore.selectedChatDirectoryId, async (id) => {
  if (!id) return
  await aiChatStore.loadChatSessions()
  aiChatStore.loadChatTemplates()
})

// --- 移除目录项（hover 出按钮 + 确认框） ---
const handleRemove = async (dir) => {
  try {
    await ElMessageBox.confirm(
      `确定移除常用目录 "${dir.displayName}" 吗？移除后该目录下的历史会话将无法再从面板访问（重新添加同路径会生成新目录项，不会恢复它们）。`,
      '移除目录',
      {
        confirmButtonText: '移除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch {
    return
  }
  try {
    await aiChatStore.removeChatDirectory(dir.id)
    ElMessage.success('已移除')
  } catch (error) {
    handleError('移除失败: ', error)
  }
}

// --- 添加目录对话框 ---
const addDialogVisible = ref(false)
const addLoading = ref(false)
const addForm = ref({ path: '', displayName: '' })
const addPathInputRef = ref()

const showAddDialog = () => {
  addForm.value = { path: '', displayName: '' }
  addDialogVisible.value = true
  nextTick(() => {
    const input = addPathInputRef.value?.input
    if (input) {
      input.focus()
    }
  })
}

const handleAdd = async () => {
  const path = addForm.value.path.trim()
  if (!path) {
    ElMessage.warning('请输入目录路径')
    return
  }
  // 前置查重（规范化路径）：后端幂等兜底，此处给可读提示
  if (aiChatStore.containsPath(path)) {
    ElMessage.info('该目录已在常用目录列表中')
    addDialogVisible.value = false
    return
  }
  addLoading.value = true
  try {
    const item = await aiChatStore.addChatDirectory(path, addForm.value.displayName.trim())
    ElMessage.success(item ? `已添加「${item.displayName}」` : '已添加')
    addDialogVisible.value = false
  } catch (error) {
    handleError('添加失败: ', error)
  } finally {
    addLoading.value = false
  }
}

// --- 重命名显示名（hover 出入口 + 对话框） ---
const renameDialogVisible = ref(false)
const renameLoading = ref(false)
const renameName = ref('')
const renameTarget = ref(null)
const renameInputRef = ref()

const showRenameDialog = (dir) => {
  renameTarget.value = dir
  renameName.value = dir.displayName
  renameDialogVisible.value = true
  nextTick(() => {
    const input = renameInputRef.value?.input
    if (input) {
      input.focus()
      input.select()
    }
  })
}

const handleRename = async () => {
  const dir = renameTarget.value
  if (!renameName.value.trim()) {
    ElMessage.warning('请输入显示名')
    return
  }
  if (!dir) return

  renameLoading.value = true
  try {
    await aiChatStore.updateChatDirectory(dir.id, renameName.value.trim())
    ElMessage.success('重命名成功')
    renameDialogVisible.value = false
  } catch (error) {
    handleError('重命名失败: ', error)
  } finally {
    renameLoading.value = false
  }
}

// ===== 会话管理（类 /resume）=====

const currentSessionTitle = computed(() =>
  aiChatStore.selectedChatSession?.title || '选择会话'
)

// 会话下拉时间：当天 HH:mm，否则 M-D HH:mm（手写格式化，不引额外依赖）
const formatSessionTime = (ts) => {
  if (!ts) return ''
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n) => String(n).padStart(2, '0')
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const now = new Date()
  if (d.toDateString() === now.toDateString()) return hm
  return `${d.getMonth() + 1}-${d.getDate()} ${hm}`
}

// 会话下拉命令分派：会话 id = 切换；__delete_session__ = 删除当前会话
const onSessionCommand = async (cmd) => {
  if (cmd === '__delete_session__') {
    await handleDeleteSession()
    return
  }
  await handleSelectSession(cmd)
}

const handleSelectSession = async (id) => {
  if (!id || id === aiChatStore.selectedChatSessionId) return
  if (!(await confirmDiscardInFlight('切换'))) return
  await aiChatStore.selectChatSession(id)
}

const handleNewSession = async () => {
  if (!(await confirmDiscardInFlight('新建'))) return
  try {
    await aiChatStore.createChatSession()
  } catch (error) {
    handleError('新建会话失败: ', error)
  }
}

const handleDeleteSession = async () => {
  const id = aiChatStore.selectedChatSessionId
  if (!id) return
  // 在途任务属于该会话时先确认并取消：防删除后进程继续跑、完成后复活孤儿消息文件
  if (aiChatStore.chatTask && aiChatStore.chatTask.chatSessionId === id) {
    if (!(await confirmDiscardInFlight('删除'))) return
  }
  const title = aiChatStore.selectedChatSession?.title || '该会话'
  try {
    await ElMessageBox.confirm(
      `确定删除会话「${title}」吗？会话消息记录将一并删除，不可恢复。`,
      '删除会话',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await aiChatStore.deleteChatSession(id)
    ElMessage.success('会话已删除')
  } catch (error) {
    handleError('删除会话失败: ', error)
  }
}

// ===== 消息渲染与流式 =====

// 展示消息 = 持久化消息 + 在途任务的流式 assistant 气泡（仅当前会话视图追加）
const displayMessages = computed(() => {
  const base = aiChatStore.chatMessages
  if (!aiChatStore.chatTaskInCurrentSession) return base
  const task = aiChatStore.chatTask
  return [
    ...base,
    { role: 'assistant', content: task.reply, streaming: true, taskId: task.taskId }
  ]
})

const messagesEl = ref(null)

// markdown 气泡链接点击拦截（模式照抄 FilePreviewRenderer.onMarkdownClick）：
// 不拦截 <a> 点击会触发 webview 顶层导航、整个应用被目标页替换；
// 外部链接交系统默认浏览器打开，相对链接/锚点在对话场景无意义仅阻止导航
const isExternalHref = (href) => /^(https?:|file:|mailto:|tel:|ftp:|data:)/i.test(href)

const onChatMarkdownClick = (event) => {
  if (onChatQuestionClick(event)) return
  const a = event.target.closest('a')
  if (!a) return
  const href = (a.getAttribute('href') || '').trim()
  if (!href) return
  event.preventDefault()
  event.stopPropagation()
  if (isExternalHref(href)) {
    BrowserOpenURL(href)
  }
}

// ===== chat-question 选择题卡片交互（事件委托：v-html 内无法绑 Vue 事件） =====

// 已答消息渲染：卡片追加 chat-question-answered 置灰类（renderer 输出该 class 属性
// 固定形态，字符串替换即精准定位；重载历史与提交后重渲染均走此路径保持一致）
const renderMsgMarkdown = (msg) => {
  const html = renderChatMarkdown(msg.content)
  if (
    msg.taskId &&
    aiChatStore.isChatQuestionAnswered(aiChatStore.selectedChatSessionId, msg.taskId)
  ) {
    return html.replace(/class="chat-question"/g, 'class="chat-question chat-question-answered"')
  }
  return html
}

// 卡片内点击分流：处理选项点选与提交，返回 true 表示命中卡片（外层链接拦截逻辑跳过）。
// 选项点选：单选题 radio 语义（清同题他项），多选题 toggle；任一题选中即启用提交按钮。
// 提交：按题序拼「用户通过选项卡片回答」文本走 runChat 常规链路（--resume 回传模型），
// 成功后标记已答（store 内存态 + 卡片 DOM 即时置灰，重渲染时由 renderMsgMarkdown 兜底）。
// 会话在途（chatInFlight）时提交守卫（PRD B9）：置灰由 .chat-in-flight CSS 承担，此处静默忽略。
const onChatQuestionClick = (event) => {
  const card = event.target.closest('.chat-question')
  if (!card) return false

  const option = event.target.closest('.chat-question-option')
  if (option && !card.classList.contains('chat-question-answered')) {
    const item = option.closest('.chat-question-item')
    if (item && !item.hasAttribute('data-multi')) {
      item.querySelectorAll('.chat-question-option.selected').forEach((el) => {
        if (el !== option) el.classList.remove('selected')
      })
    }
    option.classList.toggle('selected')
    syncCardSubmitState(card)
    return true
  }

  const submit = event.target.closest('.chat-question-submit')
  if (submit && !card.classList.contains('chat-question-answered')) {
    submitChatQuestionCard(card, event)
    return true
  }
  return true
}

// 按卡片内各题选中态同步提交按钮可用性：每题至少选中一项才启用
const syncCardSubmitState = (card) => {
  const items = card.querySelectorAll('.chat-question-item')
  const allAnswered = Array.from(items).every((item) =>
    item.querySelector('.chat-question-option.selected')
  )
  const submit = card.querySelector('.chat-question-submit')
  if (submit) submit.disabled = !allAnswered
}

// 提交选择题卡片：拼答案文本 → runChat 回传 → 标记已答置灰。
// 会话在途或已答时静默忽略（按钮置灰由 CSS 与 chat-question-answered 类承担）。
const submitChatQuestionCard = async (card, event) => {
  if (aiChatStore.chatInFlight) return
  const sessionId = aiChatStore.selectedChatSessionId
  const taskId = event.currentTarget?.dataset?.taskId || ''
  if (!sessionId || !taskId) return

  const lines = ['用户通过选项卡片回答：']
  card.querySelectorAll('.chat-question-item').forEach((item, i) => {
    const picks = Array.from(item.querySelectorAll('.chat-question-option.selected')).map(
      (el) => el.textContent.trim()
    )
    if (picks.length) {
      lines.push(`${i + 1}. ${item.dataset.question || ''}：${picks.join('、')}`)
    }
  })

  try {
    await aiChatStore.runChat(lines.join('\n'))
    aiChatStore.markChatQuestionAnswered(sessionId, taskId)
    card.classList.add('chat-question-answered')
  } catch (error) {
    handleError('发送失败: ', error)
  }
}

// 自动跟随底部：距底 ≤40px 视为「在底部」，新内容才自动滚动；
// 用户上翻阅读历史时暂停跟随，滚回底部附近自动恢复
const FOLLOW_BOTTOM_THRESHOLD = 40
let followBottom = true

const isNearBottom = () => {
  const el = messagesEl.value
  if (!el) return true
  return el.scrollHeight - el.scrollTop - el.clientHeight <= FOLLOW_BOTTOM_THRESHOLD
}

const onMessagesScroll = () => {
  followBottom = isNearBottom()
}

const scrollToBottom = (force = false) => {
  if (!force && !followBottom) return
  nextTick(() => {
    const el = messagesEl.value
    if (el) {
      el.scrollTop = el.scrollHeight
      followBottom = true
    }
  })
}
// 消息条数变化 / 流式内容增长时近底部才跟随；会话切换强制滚到底部
watch(() => [aiChatStore.chatMessages.length, aiChatStore.chatTask?.reply], () => scrollToBottom())
watch(() => aiChatStore.selectedChatSessionId, () => scrollToBottom(true))

// ===== 输入与发送 =====

const inputText = ref('')
const inputRef = ref()

// Enter 发送 / Shift+Enter 换行（textarea 内组合键交给默认行为）。
// 中文 IME 选词确认的 Enter（isComposing / keyCode 229）不发送，防把拼音串直发
const onInputEnter = (e) => {
  if (e.isComposing || e.keyCode === 229) return
  if (e.shiftKey) return
  e.preventDefault()
  handleSend()
}

// 发送重入保护：从触发到 RunChat resolve 之间（含无会话首发先建会话的 await）
// 屏蔽重复触发，防双击双会话双发
let sending = false

const handleSend = async () => {
  if (sending) return
  const prompt = inputText.value.trim()
  if (!prompt) {
    ElMessage.warning('请输入对话内容')
    return
  }
  // 进行中禁发（按钮已切换为停止，此处双保护；后端另有 E_CHAT_IN_PROGRESS 拒绝）
  if (aiChatStore.chatInFlight) return
  sending = true
  try {
    if (!aiChatStore.selectedChatSessionId) {
      // 无会话时先建一个再发（空目录直接输入的顺滑路径）
      try {
        await aiChatStore.createChatSession()
      } catch (error) {
        handleError('新建会话失败: ', error)
        return
      }
    }
    try {
      await aiChatStore.runChat(prompt)
      inputText.value = ''
    } catch (error) {
      handleError('发送失败: ', error)
    }
  } finally {
    sending = false
  }
}

const handleStop = async () => {
  try {
    await aiChatStore.cancelChatTask()
  } catch (error) {
    handleError('取消失败: ', error)
  }
}

// ===== 模板 =====

const applyTemplate = (cmd) => {
  if (cmd === '__manage_templates__') {
    tplDialogVisible.value = true
    return
  }
  const tpl = aiChatStore.chatTemplates.find(t => t.id === cmd)
  if (!tpl) return
  // 整段替换填入（用户可再修改），光标回输入框
  inputText.value = tpl.content
  nextTick(() => {
    inputRef.value?.focus?.()
  })
}

// 模板管理弹窗状态：tplForm.id 非空 = 编辑既有项（归属域锁定），空 = 新增
const tplDialogVisible = ref(false)
const tplSaving = ref(false)
const tplForm = ref({ id: '', scope: 'directory', name: '', content: '' })

const editTpl = (t) => {
  tplForm.value = { id: t.id, scope: t.scope, name: t.name, content: t.content }
}

const resetTplForm = () => {
  tplForm.value = { id: '', scope: 'directory', name: '', content: '' }
}

const handleSaveTemplate = async () => {
  const form = tplForm.value
  const name = form.name.trim()
  const content = form.content.trim()
  if (!name) {
    ElMessage.warning('请输入模板名称')
    return
  }
  if (!content) {
    ElMessage.warning('请输入模板内容')
    return
  }
  tplSaving.value = true
  try {
    if (form.id) {
      await aiChatStore.updateChatTemplate(form.id, name, content)
      ElMessage.success('模板已更新')
    } else {
      await aiChatStore.addChatTemplate(form.scope, aiChatStore.selectedChatDirectoryId, name, content)
      ElMessage.success('模板已新增')
    }
    resetTplForm()
  } catch (error) {
    handleError('保存模板失败: ', error)
  } finally {
    tplSaving.value = false
  }
}

const handleRemoveTemplate = async (t) => {
  try {
    await ElMessageBox.confirm(
      `确定删除模板「${t.name}」吗？`,
      '删除模板',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await aiChatStore.removeChatTemplate(t.id)
    if (tplForm.value.id === t.id) {
      resetTplForm()
    }
    ElMessage.success('模板已删除')
  } catch (error) {
    handleError('删除模板失败: ', error)
  }
}

// ===== 权限模式 / 模型配置 =====

const persistChatSettings = async () => {
  try {
    await aiChatStore.saveChatSettings(
      aiChatStore.chatSettings.permissionMode,
      aiChatStore.chatSettings.modelName
    )
  } catch (error) {
    handleError('保存对话设置失败: ', error)
  }
}

const onPermissionModeChange = async () => {
  if (aiChatStore.chatSettings.permissionMode === 'bypassPermissions') {
    ElMessage.warning('已选择「跳过确认」：AI 可直接修改代码与执行命令，请注意风险')
  }
  await persistChatSettings()
}

// ===== chat-task:* 事件监听（闭包精准注销，禁 EventsOff 全局移除） =====

let offQueued = null
let offStarted = null
let offOutput = null
let offDone = null

const onTaskDone = async (result) => {
  const handled = await aiChatStore.handleChatTaskDone(result)
  if (!handled) return
  if (result?.canceled) {
    ElMessage.info('对话已取消')
  } else if (result?.error) {
    ElMessage.error('对话失败: ' + result.error)
  }
}

onMounted(() => {
  // 先同步注册事件监听再异步加载数据：保证卸载路径必能拿到全部 off 闭包
  // （若先 await 加载，await 期间卸载则 off 闭包未赋值、监听器泄漏）
  offQueued = EventsOn('chat-task:queued', aiChatStore.onChatTaskQueued)
  offStarted = EventsOn('chat-task:started', aiChatStore.onChatTaskStarted)
  offOutput = EventsOn('chat-task:output', aiChatStore.onChatTaskOutput)
  offDone = EventsOn('chat-task:done', onTaskDone)

  ;(async () => {
    await aiChatStore.loadChatDirectories()
    aiChatStore.loadChatSettings()
    if (aiChatStore.selectedChatDirectoryId) {
      await aiChatStore.loadChatSessions()
      aiChatStore.loadChatTemplates()
    }
  })()
})

onBeforeUnmount(() => {
  offQueued && offQueued()
  offStarted && offStarted()
  offOutput && offOutput()
  offDone && offDone()
  offQueued = null
  offStarted = null
  offOutput = null
  offDone = null
})

// 面板切走再切回时恢复在途任务状态（事件丢失兜底，防 loading 卡死）
watch(() => uiStore.activePanel, (panel) => {
  if (panel === 'ai-chat') {
    aiChatStore.restoreChatTaskState()
  }
})
</script>

<style scoped>
/* 根元素：占满 Home 主区上半区（与 .main-panes 互斥），高度约束链从此起（对齐 .ai-function-panel）。
   缺失时根元素按 flex:0 1 auto 塌为内容高度（仅占半屏），且展开终端后 flex 收缩分摊会压缩
   TerminalPanel 的 inline height，导致终端显示高度异常。overflow:hidden 使收缩安全让位。 */
.ai-chat-panel {
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
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}
.panel-heading {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}
.panel-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.01em;
  white-space: nowrap;
}
.panel-subtitle {
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.subtitle-sign {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-weight: 600;
  color: var(--primary-color);
  margin-right: 4px;
}
.panel-title-icon {
  color: var(--primary-color);
}
.panel-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding-left: var(--spacing-md);
  margin-left: 12px;
  border-left: 1px solid var(--border-color);
}
.panel-btn-icon {
  margin-right: 4px;
}

.chat-layout {
  flex: 1;
  min-height: 0;
  display: flex;
}

/* 左：常用目录栏 */
.chat-dirs {
  width: 240px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-right: 1px solid var(--border-color);
  background: var(--bg-primary);
}
.chat-dirs-caption {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.04em;
  color: var(--text-tertiary);
}
.chat-dirs-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  font-size: 11px;
  font-weight: 500;
  color: var(--text-secondary);
}
.chat-dirs-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-xs) 0;
}
.chat-dirs-empty {
  margin-top: var(--spacing-xl);
}
.chat-dirs-hint {
  flex-shrink: 0;
  padding: var(--spacing-sm) var(--spacing-md);
  border-top: 1px solid var(--border-color);
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-placeholder);
}

/* 目录项：nav active 左侧指示条（left:0 贴项左缘，防父级 overflow 裁剪） */
.chat-dir-item {
  position: relative;
  padding: var(--spacing-sm) var(--spacing-md);
  margin: 2px var(--spacing-sm);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--transition-fast);
}
.chat-dir-item:hover {
  background: var(--bg-tertiary);
}
.chat-dir-item.is-active {
  background: var(--primary-bg);
}
.chat-dir-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  background: var(--primary-light);
}
.chat-dir-item--ghost {
  opacity: 0.6;
  background: color-mix(in srgb, var(--success-color) 12%, transparent);
  border: 1px dashed var(--success-color);
  border-radius: var(--radius-sm);
}
.chat-dir-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
.chat-dir-icon {
  flex-shrink: 0;
  color: var(--text-tertiary);
}
.chat-dir-item.is-active .chat-dir-icon {
  color: var(--primary-color);
}
.chat-dir-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}
/* 重命名/移除按钮：hover 出现（占用右侧尾部，不挤压名称省略） */
.chat-dir-edit,
.chat-dir-remove {
  flex-shrink: 0;
  color: var(--text-tertiary);
  opacity: 0;
  transition: color var(--transition-fast), opacity var(--transition-fast);
}
.chat-dir-item:hover .chat-dir-edit,
.chat-dir-item:hover .chat-dir-remove {
  opacity: 1;
}
.chat-dir-edit:hover {
  color: var(--primary-color);
}
.chat-dir-remove:hover {
  color: var(--danger-color);
}
.chat-dir-path {
  margin-top: 2px;
  padding-left: 22px;
  font-size: 12px;
  font-family: Consolas, 'Courier New', monospace;
  color: var(--text-tertiary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 右：对话区（白底卡片浮于面板底色，上下分栏） */
.chat-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: var(--bg-secondary);
}
.chat-main-empty {
  margin: auto;
}

/* 会话工具条 */
.chat-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
}
.session-btn {
  max-width: 240px;
}
.session-btn-icon {
  margin-right: 4px;
  color: var(--primary-color);
}
.session-btn-label {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.session-btn-arrow {
  margin-left: 4px;
  color: var(--text-tertiary);
}
.chat-toolbar-spacer {
  flex: 1;
}
.chat-config-select {
  width: 128px;
}
.chat-config-model {
  width: 140px;
}

/* 会话下拉项：标题 + 更新时间两列 */
.session-menu .session-menu-title {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
  vertical-align: bottom;
}
.session-menu .session-menu-time {
  margin-left: 12px;
  font-size: 11px;
  color: var(--text-placeholder);
}
.session-menu .is-current .session-menu-title {
  color: var(--primary-color);
  font-weight: 600;
}
.session-menu-delete {
  color: var(--danger-color);
}

/* 消息历史区：纵向滚动 */
.chat-messages {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-md) var(--spacing-lg);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}
.chat-messages-empty {
  margin: auto;
}

/* 消息气泡：user 右对齐主色淡底 / assistant 左对齐弱层底 */
.chat-msg-row {
  display: flex;
}
.chat-msg-row.user {
  justify-content: flex-end;
}
.chat-msg-row.assistant {
  justify-content: flex-start;
}
.chat-msg-bubble {
  max-width: 82%;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  font-size: 13px;
  line-height: 1.7;
  word-break: break-word;
}
.chat-msg-row.user .chat-msg-bubble {
  background: var(--primary-bg);
  color: var(--text-primary);
  border: 1px solid color-mix(in srgb, var(--primary-color) 18%, transparent);
}
.chat-msg-row.assistant .chat-msg-bubble {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  border: 1px solid var(--border-color);
}

/* 流式生成中：气泡尾部闪烁光标（仅装饰，reduced-motion 下禁用） */
.chat-msg-bubble.is-streaming .chat-msg-md > *:last-child::after {
  content: '▍';
  color: var(--primary-color);
  animation: chatCursor 1s steps(2) infinite;
}
@keyframes chatCursor {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

/* assistant markdown 排版：标题/列表/代码块/表格/引用 */
.chat-msg-md :deep(h1),
.chat-msg-md :deep(h2),
.chat-msg-md :deep(h3),
.chat-msg-md :deep(h4) {
  font-weight: 600;
  letter-spacing: -0.01em;
  margin: 0.6em 0 0.3em;
  line-height: 1.4;
}
.chat-msg-md :deep(h1) { font-size: 1.25em; }
.chat-msg-md :deep(h2) { font-size: 1.15em; }
.chat-msg-md :deep(h3) { font-size: 1.05em; }
.chat-msg-md :deep(p) {
  margin: 0.35em 0;
}
.chat-msg-md :deep(ul),
.chat-msg-md :deep(ol) {
  margin: 0.35em 0;
  padding-left: 1.4em;
}
.chat-msg-md :deep(li) {
  margin: 0.15em 0;
}
.chat-msg-md :deep(code) {
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  font-size: 0.92em;
  padding: 1px 5px;
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--text-primary) 8%, transparent);
}
.chat-msg-md :deep(pre.hljs) {
  margin: 0.5em 0;
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  overflow-x: auto;
}
.chat-msg-md :deep(pre.hljs code) {
  padding: 0;
  background: transparent;
  font-size: 12px;
  line-height: 1.6;
}
.chat-msg-md :deep(blockquote) {
  margin: 0.5em 0;
  padding: 0.1em 0.8em;
  border-left: 3px solid var(--primary-light);
  color: var(--text-secondary);
  background: color-mix(in srgb, var(--primary-color) 5%, transparent);
}
.chat-msg-md :deep(table) {
  border-collapse: collapse;
  margin: 0.5em 0;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.chat-msg-md :deep(th),
.chat-msg-md :deep(td) {
  border: 1px solid var(--border-color);
  padding: 4px 10px;
}
.chat-msg-md :deep(th) {
  background: var(--bg-primary);
  font-weight: 600;
}
.chat-msg-md :deep(a) {
  color: var(--primary-color);
}
.chat-msg-md :deep(hr) {
  border: none;
  border-top: 1px solid var(--border-color);
  margin: 0.6em 0;
}

/* chat-question 选择题卡片：内联于消息流（非模态，不阻塞会话/目录切换）。
   选中态用 primary 色描边+浅底，禁用/已答统一降透明度弱化 */
.chat-msg-md :deep(.chat-question) {
  margin: 0.5em 0;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
}
.chat-msg-md :deep(.chat-question-item) {
  margin: 0.35em 0;
}
.chat-msg-md :deep(.chat-question-title) {
  font-weight: 500;
  margin-bottom: 0.3em;
}
.chat-msg-md :deep(.chat-question-hint) {
  font-weight: 400;
  font-size: 0.85em;
  color: var(--text-secondary);
}
.chat-msg-md :deep(.chat-question-options) {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.chat-msg-md :deep(.chat-question-option) {
  padding: 4px 12px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 12px;
  cursor: pointer;
  transition: border-color var(--transition-fast), background var(--transition-fast);
}
.chat-msg-md :deep(.chat-question-option:hover) {
  border-color: var(--primary-color);
}
.chat-msg-md :deep(.chat-question-option.selected) {
  border-color: var(--primary-color);
  background: color-mix(in srgb, var(--primary-color) 12%, transparent);
  color: var(--primary-color);
}
.chat-msg-md :deep(.chat-question-actions) {
  margin-top: 0.5em;
  display: flex;
  justify-content: flex-end;
}
.chat-msg-md :deep(.chat-question-submit) {
  padding: 4px 14px;
  border: none;
  border-radius: var(--radius-sm);
  background: var(--primary-color);
  color: #fff;
  font-size: 12px;
  cursor: pointer;
}
.chat-msg-md :deep(.chat-question-submit:disabled) {
  opacity: 0.45;
  cursor: not-allowed;
}
/* 会话在途：卡片提交按钮置灰禁点（点击委托另有 chatInFlight 守卫双保护） */
.chat-messages.chat-in-flight .chat-msg-md :deep(.chat-question-submit:not(:disabled)) {
  opacity: 0.45;
  cursor: not-allowed;
}
/* 已答卡片：整体弱化且禁点（含选项），提交后防重复作答 */
.chat-msg-md :deep(.chat-question-answered) {
  opacity: 0.65;
  pointer-events: none;
}
.chat-msg-md :deep(.chat-question-answered .chat-question-option.selected) {
  border-color: var(--border-color);
  background: color-mix(in srgb, var(--text-primary) 8%, transparent);
  color: var(--text-primary);
}

/* 输入区 */
.chat-input-area {
  flex-shrink: 0;
  padding: var(--spacing-sm) var(--spacing-md);
  border-top: 1px solid var(--border-color);
  background: var(--bg-secondary);
}
.chat-input-row {
  display: flex;
  align-items: flex-end;
  gap: var(--spacing-sm);
}
.tpl-btn {
  flex-shrink: 0;
}
.chat-input {
  flex: 1;
  min-width: 0;
}
/* 输入框高度：默认 6 行（rows），允许原生纵向拖拽调高。
   下限约 2 行防拖得过扁，上限 40vh 防拖到占满会话区；
   拖拽高度为会话内临时态，不持久化、不入会话快照。
   EP 非 autosize 挂载时会写内联 min-height:31px，!important 压过运行时内联值 */
.chat-input :deep(.el-textarea__inner) {
  resize: vertical;
  min-height: 52px !important;
  max-height: 40vh;
}
.chat-input-actions {
  flex-shrink: 0;
}
/* 进行中状态行：呼吸圆点 + 文案 */
.chat-input-status {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: var(--spacing-xs);
  font-size: 12px;
  color: var(--text-tertiary);
}
.chat-input-status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary-color);
  animation: chatPulse 1.4s ease-in-out infinite;
}
@keyframes chatPulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

/* 模板管理弹窗主体：列表 + 表单左右二分（对齐 SettingsPanel 的 settings-body）。
   margin 负值与 el-dialog__body padding（同为 --spacing-lg）配对抵消，
   叠加弹窗容器 padding 归零（见 .el-dialog.tpl-manage-dialog 的 padding:0），
   三层配对方能使 tpl-manage-body 边缘到边缘填满 dialog body */
.tpl-manage-body {
  display: flex;
  height: min(560px, 78vh);
  margin: calc(-1 * var(--spacing-lg));
}
/* 左侧模板列表栏（对齐 settings-nav 风格：固定宽 + 次层背景 + 右缘分隔线 + 纵向滚动） */
.tpl-list {
  width: 200px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border-color);
  padding: var(--spacing-sm) 0;
}
/* 模板项：position:relative 供 ::before 指示条定位；
   指示条 left:0 贴 item 左缘（禁负 left，防父级 overflow 裁剪，同项目既有规范） */
.tpl-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  cursor: pointer;
  transition: background var(--transition-fast);
}
.tpl-item:hover {
  background: var(--bg-tertiary);
}
.tpl-item.is-active {
  background: var(--primary-bg);
}
.tpl-item.is-active:hover {
  background: var(--primary-bg);
}
.tpl-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  background: var(--primary-light);
}
.tpl-item-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}
.tpl-item-remove {
  flex-shrink: 0;
  color: var(--text-tertiary);
}
.tpl-item-remove:hover {
  color: var(--danger-color);
}
/* 新增模板项：保留列表顶部，虚线边框区分操作项与数据项（内缩留出通栏底色） */
.tpl-item-new {
  margin: 0 var(--spacing-sm);
  color: var(--primary-color);
  font-size: 13px;
  border: 1px dashed var(--border-color);
  border-radius: var(--radius-sm);
}
.tpl-item-new:hover {
  border-color: var(--primary-light);
}
.tpl-list-empty {
  margin-top: var(--spacing-lg);
}
/* 右侧表单区：吃掉剩余宽（对齐 settings-content），内容超高时表单区滚动兜底 */
.tpl-form {
  flex: 1;
  min-width: 0;
  padding: var(--spacing-lg);
  overflow-y: auto;
}
/* 模板正文：默认 12 行，原生纵向拖拽调高限幅（同 chat-input 风格），下限约 5 行防拖得过扁；
   EP 非 autosize 挂载时会写内联 min-height:31px，!important 压过运行时内联值 */
.tpl-content-input :deep(.el-textarea__inner) {
  resize: vertical;
  min-height: 120px !important;
  max-height: 40vh;
}
.tpl-form-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-sm);
}

/* 动画可访问性：用户系统偏好减少动效时禁用装饰动画 */
@media (prefers-reduced-motion: reduce) {
  .chat-msg-bubble.is-streaming .chat-msg-md > *:last-child::after,
  .chat-input-status-dot {
    animation: none !important;
  }
}
</style>

<style>
/* 模板管理弹窗主题：对齐 settings-dialog（背景/圆角/header/body padding/遮罩着色）。
   el-dialog 为 append-to-body 传送至组件树外，scoped 样式无法作用，须用非 scoped 块 + 专属 class 圈定。
   class 经 attrs 透传与 .el-dialog 落在同一元素，须用联合选择器，后代选择器匹配不到自身；
   EP 2.14.5 容器自带 padding，此处归零，防负 margin 抵消 body padding 后仍残留边缘框。
   body padding 显式固定为 --spacing-lg，与 .tpl-manage-body 的负 margin 配对抵消 */
.el-dialog.tpl-manage-dialog {
  padding: 0;
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
}

.tpl-manage-dialog .el-dialog__header {
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-color);
  border-radius: var(--radius-lg) var(--radius-lg) 0 0;
  padding: var(--spacing-md) var(--spacing-lg);
}

.tpl-manage-dialog .el-dialog__title {
  color: var(--text-primary);
  font-size: 16px;
  font-weight: 600;
}

.tpl-manage-dialog .el-dialog__headerbtn .el-dialog__close {
  color: var(--text-tertiary);
}

.tpl-manage-dialog .el-dialog__headerbtn:hover .el-dialog__close {
  color: var(--text-primary);
}

.tpl-manage-dialog .el-dialog__body {
  padding: var(--spacing-lg);
}

/* el-overlay 蓝灰着色（禁纯黑）：:has() 仅作用于含 tpl-manage-dialog 的遮罩，不影响其他弹窗；
   亮色 Slate-900 着色，暗色更深一档 Slate-950 着色 */
.el-overlay:has(.tpl-manage-dialog) {
  background-color: rgba(15, 23, 42, 0.5);
}

html.dark .el-overlay:has(.tpl-manage-dialog) {
  background-color: rgba(2, 6, 23, 0.6);
}
</style>
