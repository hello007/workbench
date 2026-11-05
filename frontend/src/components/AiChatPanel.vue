<template>
  <div ref="panelEl" class="ai-chat-panel">
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
          <!-- 会话工具条：会话下拉（类 /resume）+ 新建 + 启动命令 + 模型
               （权限模式已迁至输入盒底栏的分段式按钮组） -->
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

            <!-- 启动命令：默认 claude·自动 + PATH 扫描候选（name + path）+ 指定路径…；
                 切换即持久化；协议口径说明见 tooltip -->
            <el-dropdown trigger="click" @command="onLauncherCommand">
              <el-button size="small" class="launcher-btn" :title="launcherTip">
                <el-icon class="panel-btn-icon"><Setting /></el-icon>
                <span class="launcher-btn-label">{{ launcherButtonLabel }}</span>
                <el-icon class="session-btn-arrow"><ArrowDown /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu class="launcher-menu">
                  <el-dropdown-item
                    command="__launcher_default__"
                    :class="{ 'is-current': !aiChatStore.chatSettings.launcher }"
                  >
                    <span class="launcher-menu-name">claude·自动</span>
                    <span class="launcher-menu-path">PATH 解析（默认）</span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    v-for="c in aiChatStore.chatCliCandidates"
                    :key="c.path"
                    :command="c.path"
                    :class="{ 'is-current': aiChatStore.chatSettings.launcher === c.path }"
                  >
                    <span class="launcher-menu-name">{{ c.name }}</span>
                    <span class="launcher-menu-path" :title="c.path">{{ c.path }}</span>
                  </el-dropdown-item>
                  <el-dropdown-item divided command="__launcher_pick__">
                    指定路径…
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <!-- 模型：常用选项 + allow-create 自定义输入；空值 = claude 默认模型
                 （首选项 label 动态回显 claude 设置解析的真实默认值） -->
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
                v-for="m in modelOptions"
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
                <!-- 思考块：仅流式在途任务携带（done 即丢弃，历史消息无此块）。
                     灰色折叠样式：流式中默认展开，点标题手动收起/展开（组件本地态）；
                     无思考输出时不渲染，气泡形态与现状一致 -->
                <div v-if="msg.thinking" class="chat-thinking">
                  <div class="chat-thinking-head" title="点击收起/展开思考过程" @click="toggleThinking(msg.taskId)">
                    <span class="chat-thinking-spinner"></span>
                    <span class="chat-thinking-title">思考中…</span>
                    <el-icon class="chat-thinking-arrow" :class="{ 'is-collapsed': isThinkingCollapsed(msg.taskId) }"><ArrowDown /></el-icon>
                  </div>
                  <div v-show="!isThinkingCollapsed(msg.taskId)" class="chat-thinking-body">{{ msg.thinking }}</div>
                </div>
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

          <!-- 对话状态栏：模型 │ 上下文渐变进度条（精简版：仅保留两段，
               分支/目录/时间段已按需求移除）。上下文 token 由后端每轮 result
               采集回写会话元数据（contextTokens），渐变条 0% 青 → 50% 黄 → 100% 红 -->
          <div v-if="aiChatStore.selectedChatSession" class="chat-statusline">
            <span class="chat-statusline-model">{{ statuslineModel }}</span>
            <span class="chat-statusline-ctx">
              <span class="chat-statusline-bar">
                <span class="chat-statusline-bar-fill" :style="statuslineBarStyle"></span>
              </span>
              <span class="chat-statusline-ctx-text" :style="{ color: statuslineCtxColor }">{{ statuslineCtxText }}</span>
            </span>
          </div>

          <!-- 输入区：带内嵌底栏的输入盒（claude.ai 同款布局）——textarea 主体 +
               盒内下沿操作条：模板 / 权限模式分段组靠左，停止+发送靠右 -->
          <div class="chat-input-area">
            <div ref="inputBoxEl" class="chat-input-box">
              <!-- skill 斜杠补全弹层：行首 / 触发（对齐 claude CLI），向上展开。
                   列表项四列：命令 / 名称 / 描述 / 来源标签；点击（mousedown.prevent
                   防抢 textarea 焦点）与 Enter/Tab 补全，↑↓ 移动高亮，Esc 关闭。
                   弹层同时承担「查看已装 skill」入口职责，刷新按钮强制重扫 -->
              <div v-if="skillPopupVisible" class="skill-popup" :style="{ maxHeight: skillPopupMaxHeight + 'px' }">
                <div class="skill-popup-head">
                  <span class="skill-popup-title">已安装 Skill</span>
                  <el-button size="small" text bg class="skill-popup-refresh" @click="onSkillRefresh">
                    <el-icon><Refresh /></el-icon>刷新
                  </el-button>
                </div>
                <ul class="skill-popup-list">
                  <li
                    v-for="(s, i) in filteredSkillItems"
                    :key="s.command"
                    :class="{ 'is-active': i === skillActiveIndex }"
                    @mousedown.prevent="confirmSkillCompletion(s)"
                  >
                    <span class="skill-popup-cmd">{{ s.command }}</span>
                    <span class="skill-popup-name">{{ s.name }}</span>
                    <span class="skill-popup-desc" :title="s.description">{{ s.description }}</span>
                    <span class="skill-popup-source">{{ skillSourceLabel(s.source) }}</span>
                  </li>
                </ul>
              </div>
              <!-- 输入框：固定 rows 默认 6 行 + 原生纵向拖拽调高（CSS 限幅）。
                   不用 autosize：其 JS 接管高度会覆盖拖拽结果，与原生 resize 冲突 -->
              <el-input
                ref="inputRef"
                v-model="inputText"
                type="textarea"
                class="chat-input"
                :rows="6"
                placeholder="输入需求，Enter 发送 / Shift+Enter 换行"
                @input="onChatInput"
                @keydown.enter="onInputEnter"
                @keydown.up="onSkillArrow(-1, $event)"
                @keydown.down="onSkillArrow(1, $event)"
                @keydown.tab="onSkillTab"
                @keydown.esc="onSkillEsc"
              />
              <div class="chat-input-box-footer">
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
                <!-- 权限模式：分段式按钮组（四档同排平铺，当前档高亮），悬停各档
                     在该档位置出各自说明（tooltip 包在每个按钮上，非整组居中）；
                     bypassPermissions 选中仍警告提示；非默认档警示色强调（is-elevated） -->
                <el-radio-group
                  v-model="aiChatStore.chatSettings.permissionMode"
                  size="small"
                  class="chat-perm-seg"
                  :class="{ 'is-elevated': aiChatStore.chatSettings.permissionMode !== 'default' }"
                  @change="onPermissionModeChange"
                >
                  <el-tooltip
                    v-for="m in PERMISSION_MODES"
                    :key="m.value"
                    :content="m.tip"
                    placement="top"
                  >
                    <el-radio-button :value="m.value">{{ m.short }}</el-radio-button>
                  </el-tooltip>
                </el-radio-group>
                <div class="chat-input-spacer"></div>
                <!-- 持续会话模型：运行中发送 = 排队注入（插话），发送按钮恒可用；
                     有在途轮次时并列展示停止按钮（中断当前轮 + 摘除排队轮） -->
                <el-button
                  v-if="aiChatStore.chatInFlight"
                  type="danger"
                  plain
                  size="small"
                  class="chat-stop-btn"
                  @click="handleStop"
                >
                  <el-icon><VideoPause /></el-icon>停止
                </el-button>
                <el-button type="primary" size="small" class="chat-send-btn" @click="handleSend">
                  <el-icon><Position /></el-icon>发送
                </el-button>
              </div>
            </div>
            <div v-if="aiChatStore.chatInFlight" class="chat-input-status">
              <span class="chat-input-status-dot"></span>
              {{ aiChatStore.currentSessionChatTasks.some(t => t.status === 'running') ? '回复生成中…' : '排队等待中…' }}
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
  ArrowDown, ChatLineRound, Document, Setting, Position, VideoPause, Refresh
} from '@element-plus/icons-vue'
import { VueDraggable } from 'vue-draggable-plus'
import { useAiChatStore, useUiStore } from '../store'
import { handleError } from '../utils/error'
import { shortenPath } from '../utils/pathFormat'
import { renderChatMarkdown, CMD_RUN_BTN_TITLE, CMD_RUN_BTN_TITLE_NO_CWD } from '../utils/chatMarkdown'
import { extractSlashToken, applyCompletion, filterSkills } from '../utils/skillComplete'
import { debug } from '../utils/debug'
import { RespondChatApproval, OpenFileDialog } from '../../wailsjs/go/main/App'
import { EventsOn, BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const aiChatStore = useAiChatStore()
const uiStore = useUiStore()

// ===== 常量选项 =====

// 权限模式可选集（值与 model.ChatPermissionMode* 常量对齐）
// PERMISSION_MODES：分段式按钮组档位（short 为组内短标签，tip 悬停在该档位置弹出；
// bypassPermissions 选中另有警告提示）。
// 六档对齐 claude CLI --permission-mode 枚举（2026-11-03）：
// acceptEdits/auto/bypassPermissions/manual/dontAsk/plan（default 即 CLI 的 Manual，
// 仍被 CLI 接受且作归一默认值，排首位）
const PERMISSION_MODES = [
  { value: 'default', short: '手动', label: '手动确认', tip: '逐次确认：每个敏感操作都需手动批准（CLI 的 Manual/default 档）' },
  { value: 'acceptEdits', short: '接受编辑', label: '自动接受编辑', tip: '自动接受文件编辑，其余操作仍需确认' },
  { value: 'plan', short: '规划', label: '规划模式', tip: '仅分析与规划方案，不执行修改' },
  { value: 'auto', short: '自动', label: '自动模式', tip: 'CLI 智能判断：低风险操作自动放行，敏感操作仍走审批' },
  { value: 'dontAsk', short: '不询问', label: '不询问', tip: '需确认的操作自动拒绝（无人值守只读场景，AI 不会执行敏感操作）' },
  { value: 'bypassPermissions', short: '跳过确认', label: '跳过确认（高危）', tip: '跳过全部权限确认，AI 可直接修改代码与执行命令（高危）' }
]

// 模型下拉常用项（MVP 仅 claude 系；allow-create 支持自定义输入，空值 = claude 默认模型）。
// 首选项 label 动态：claude 设置文件解析出默认模型时回显真实值
// （「默认（claude 设置：glm-5.3-flash）」），未解析到回退「默认模型」
const MODEL_OPTIONS = [
  { value: '', label: '默认模型' },
  { value: 'sonnet', label: 'Sonnet' },
  { value: 'opus', label: 'Opus' },
  { value: 'haiku', label: 'Haiku' }
]

const modelOptions = computed(() => {
  const def = aiChatStore.claudeDefaultModel
  const fallbackLabel = def ? `默认（claude 设置：${def}）` : '默认模型'
  return MODEL_OPTIONS.map(m =>
    m.value === '' ? { ...m, label: fallbackLabel } : m
  )
})

// 启动命令下拉命令分派常量（与 el-dropdown-item command 对号）
const LAUNCHER_CMD_DEFAULT = '__launcher_default__'
const LAUNCHER_CMD_PICK = '__launcher_pick__'
// 协议口径提示（R5）：启动项需兼容 claude CLI headless 协议，第三方 CLI 不保证可用
const launcherTip = '启动命令：候选为 PATH 扫描结果；启动项需兼容 claude CLI headless 协议（-p stream-json），第三方 CLI 不保证可用'

// 启动命令按钮标签：默认项 / 命中候选名 / 自定义绝对路径显示文件名
const launcherButtonLabel = computed(() => {
  const launcher = aiChatStore.chatSettings.launcher
  if (!launcher) return 'claude·自动'
  const cand = aiChatStore.chatCliCandidates.find(c => c.path === launcher || c.name === launcher)
  if (cand) return cand.name
  const base = launcher.split(/[\\/]/).pop() || launcher
  return `自定义：${base}`
})

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

// --- 侧栏目录点击 ---
// 持续会话进程模型：切换目录/会话不再打断在途对话（后台会话继续运行），
// 原确认取消链（confirmDiscardInFlight）随之撤销。
const handleSelectDirectory = async (dir) => {
  if (dir.id === aiChatStore.selectedChatDirectoryId) return
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
  await aiChatStore.selectChatSession(id)
}

const handleNewSession = async () => {
  try {
    await aiChatStore.createChatSession()
  } catch (error) {
    handleError('新建会话失败: ', error)
  }
}

const handleDeleteSession = async () => {
  const id = aiChatStore.selectedChatSessionId
  if (!id) return
  // 在途轮次取消由 store.deleteChatSession 统一处理（防删除后进程继续写孤儿消息文件）
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

// 展示消息 = 持久化消息 + 在途轮次的流式 assistant 气泡（仅当前会话视图追加，
// 多条在途轮次各一气泡，插入序 = 提交先后；thinking 随任务对象带给思考块，
// done 后任务摘除即丢弃——历史消息无思考块）
const displayMessages = computed(() => {
  const base = aiChatStore.chatMessages
  const tasks = aiChatStore.currentSessionChatTasks
  if (!tasks.length) return base
  return [
    ...base,
    ...tasks.map(t => ({ role: 'assistant', content: t.reply, thinking: t.thinking || '', streaming: true, taskId: t.taskId }))
  ]
})

// ===== 思考块收起状态（组件本地瞬时态，不进 store/不持久化）=====
// 流式中默认展开；点标题手动收起/展开；按 taskId 记录（多在途轮次各一思考块）
const collapsedThinkingTasks = ref(new Set())
const isThinkingCollapsed = (taskId) => collapsedThinkingTasks.value.has(taskId || '')
const toggleThinking = (taskId) => {
  const key = taskId || ''
  const next = new Set(collapsedThinkingTasks.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  collapsedThinkingTasks.value = next
}

const messagesEl = ref(null)

// markdown 气泡链接点击拦截（模式照抄 FilePreviewRenderer.onMarkdownClick）：
// 不拦截 <a> 点击会触发 webview 顶层导航、整个应用被目标页替换；
// 外部链接交系统默认浏览器打开，相对链接/锚点在对话场景无意义仅阻止导航
const isExternalHref = (href) => /^(https?:|file:|mailto:|tel:|ftp:|data:)/i.test(href)

const onChatMarkdownClick = (event) => {
  if (onChatQuestionClick(event)) return
  // 「在终端执行」按钮分流（置于链接拦截前命中返回）：命令原文取自容器内
  // pre>code 的 textContent（勿用 data 属性存命令——多行/引号/长度受限，
  // textContent 天然还原 HTML 转义与高亮 span）
  const cmdBtn = event.target.closest('[data-cmd-run]')
  if (cmdBtn) {
    onCmdRunClick(cmdBtn)
    return
  }
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

// 代码块「在终端执行」（AI 对话 × 终端联动一期）：打开全局底部终端并定位到
// 会话 cwd，命令经 uiStore pending 信箱通道送 TerminalPanel 写入 PTY 输入行
// （不带换行不自动回车，回车执行权在用户；不落会话消息记录）。
// 无选中会话 / 会话无 cwd 时静默返回（按钮置灰在渲染层，此处为运行时兜底）
const onCmdRunClick = (btn) => {
  const cwd = aiChatStore.selectedChatSession?.cwd
  if (!cwd) return
  const code = btn.closest('.cmd-run-wrap')?.querySelector('pre code')
  // markdown 围栏内容固定带结尾换行（textContent 会带上），原样写入 PTY 等于
  // 自动回车执行——剥掉结尾换行落实「不自动回车」；多行命令中间换行保留原样
  const cmd = code ? code.textContent.replace(/\n+$/, '') : ''
  if (!cmd.trim()) return
  uiStore.terminalVisible = true
  uiStore.terminalDir = cwd
  uiStore.pushTerminalPendingCmd(cwd, cmd)
}

// ===== chat-question 选择题卡片交互（事件委托：v-html 内无法绑 Vue 事件） =====

// 已答消息渲染：卡片追加 chat-question-answered 置灰类（renderer 输出该 class 属性
// 固定形态，字符串替换即精准定位；重载历史与提交后重渲染均走此路径保持一致）
const renderMsgMarkdown = (msg) => {
  const html = renderChatMarkdown(msg.content)
  let out = html
  if (
    msg.taskId &&
    aiChatStore.isChatQuestionAnswered(aiChatStore.selectedChatSessionId, msg.taskId)
  ) {
    out = out.replace(/class="chat-question"/g, 'class="chat-question chat-question-answered"')
  }
  // 无 cwd 会话：代码块「在终端执行」按钮置灰 + tooltip 改为「当前会话无工作目录」
  // 说明（PRD R3；注入形态固定，字符串替换即精准定位；点击 handler 内仍有 cwd
  // 兜底拦截，双保险）
  if (!aiChatStore.selectedChatSession?.cwd) {
    out = out.replace(/class="cmd-run-btn"/g, 'class="cmd-run-btn is-disabled"')
    out = out.replaceAll(`title="${CMD_RUN_BTN_TITLE}"`, `title="${CMD_RUN_BTN_TITLE_NO_CWD}"`)
  }
  return out
}

// 卡片内点击分流：处理选项点选与提交，返回 true 表示命中卡片（外层链接拦截逻辑跳过）。
// 选项点选：单选题 radio 语义（清同题他项），多选题 toggle；任一题选中即启用提交按钮。
// 提交：按题序拼「用户通过选项卡片回答」文本走 runChat 常规链路（--resume 回传模型），
// 成功后标记已答（store 内存态 + 卡片 DOM 即时置灰，重渲染时由 renderMsgMarkdown 兜底）。
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
// 已答时静默忽略（置灰由 chat-question-answered 类承担）；运行中提交合法
// （排队注入，答案轮次在前序轮 result 后执行）。
const submitChatQuestionCard = async (card, event) => {
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
// 消息条数变化 / 任一在途轮次流式内容（正文或思考）增长时近底部才跟随；
// 会话切换强制滚到底部
watch(
  () => [
    aiChatStore.chatMessages.length,
    aiChatStore.currentSessionChatTasks.map(t => t.reply).join('\u0000'),
    aiChatStore.currentSessionChatTasks.map(t => t.thinking).join('\u0000')
  ],
  () => scrollToBottom()
)
watch(() => aiChatStore.selectedChatSessionId, () => scrollToBottom(true))

// ===== 对话状态栏（精简版：仅模型 + 上下文占用两段）=====

// 模型段：会话执行配置优先，回退 claude 设置文件解析的默认模型
const statuslineModel = computed(() =>
  aiChatStore.chatSettings.modelName || aiChatStore.claudeDefaultModel || 'claude'
)

// 上下文段：窗口大小 glm-5 系 1M / 其余 200k（对齐 statusline 脚本口径），
// 占用取会话元数据的最近一轮 contextTokens（0 = 尚无数据，进度条空态）
const statuslineCtxLimit = computed(() => {
  const m = String(statuslineModel.value).toLowerCase()
  return m.includes('glm-5') ? 1000000 : 200000
})
const statuslineCtxUsed = computed(() => aiChatStore.selectedChatSession?.contextTokens || 0)
const statuslineCtxPct = computed(() =>
  Math.max(0, Math.min(100, (statuslineCtxUsed.value / statuslineCtxLimit.value) * 100))
)

// 渐变色：0% 青 (0,255,200) → 50% 黄 (255,220,0) → 100% 红 (255,60,60)，
// 插值公式与 statusline 脚本 gradientColor 一致
const ctxGradientColor = (pct) => {
  const p = Math.max(0, Math.min(100, pct))
  let r, g, b
  if (p < 50) {
    const t = p / 50
    r = Math.round(255 * t)
    g = Math.round(255 - 35 * t)
    b = Math.round(200 - 200 * t)
  } else {
    const t = (p - 50) / 50
    r = 255
    g = Math.round(220 - 160 * t)
    b = Math.round(60 * t)
  }
  return `rgb(${r}, ${g}, ${b})`
}
const statuslineBarStyle = computed(() => ({
  width: statuslineCtxPct.value + '%',
  background: ctxGradientColor(statuslineCtxPct.value)
}))
const statuslineCtxColor = computed(() => ctxGradientColor(statuslineCtxPct.value))

// token 格式化：30.2k / 200k / 1M（对齐 statusline 脚本 fmtTokens）
const fmtCtxTokens = (n) => {
  if (n >= 1000000) return (n / 1000000).toFixed(n % 1000000 === 0 ? 0 : 1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(n >= 100000 ? 0 : 1) + 'k'
  return String(n)
}
const statuslineCtxText = computed(() =>
  `${fmtCtxTokens(statuslineCtxUsed.value)}/${fmtCtxTokens(statuslineCtxLimit.value)}`
)

// ===== 输入与发送 =====

const inputText = ref('')

// 按会话隔离的输入草稿（组件内部瞬时态，不进 store）：切换会话时保存旧会话
// 草稿、载入新会话草稿。键统一 `sessionId ?? ''`（无选中会话归空串键，避免
// undefined 键泄漏）。会话删除不订阅清理事件——删除路径 selectedChatSessionId
// 变化的本 watch 已覆盖保存面，过期键残留无害（会话 id 不复用）。
const chatDrafts = new Map()
const chatDraftKey = (sessionId) => sessionId ?? ''
watch(
  () => aiChatStore.selectedChatSessionId,
  (newId, oldId) => {
    chatDrafts.set(chatDraftKey(oldId), inputText.value)
    inputText.value = chatDrafts.get(chatDraftKey(newId)) ?? ''
    // 滚动复位由上方 selectedChatSessionId 的既有 watch 统一处理，此处不重复调用
  }
)
const inputRef = ref()

// ===== skill 斜杠补全 =====

// 弹层高亮下标与「Esc 已驳回的 token 指纹」是仅有的本地态：可见性由
//「行首 / token 仍活跃 + 非 Esc 驳回 + 有匹配项」纯推导（computed），
// token 失活（输入空白/删除 /）弹层自动消失，无需手动开关状态机
const skillActiveIndex = ref(0)
// Esc 驳回标记：token 指纹（start:query）。Esc 关闭后同一 token 不再弹，
// 任何新输入事件清空标记（继续打字重新出现，对齐 CLI）
const skillEscDismissed = ref('')
// 最近一次输入事件的光标位置（extractSlashToken 需要 caret 判断 token 活跃性）
const lastCaret = ref(0)

// 当前活跃的斜杠 token（行首 / 且光标在 token 内）
const skillToken = computed(() => extractSlashToken(inputText.value, lastCaret.value))
// token 过滤后的候选列表（弹层关闭时为空数组，避免无谓遍历）
const filteredSkillItems = computed(() => {
  if (!skillToken.value.active) return []
  return filterSkills(aiChatStore.availableDiscoveredSkills, skillToken.value.query)
})
// 弹层可见：token 活跃、非 Esc 驳回、有匹配项（无 skill / 无匹配不弹层）
const skillPopupVisible = computed(() =>
  skillToken.value.active &&
  filteredSkillItems.value.length > 0 &&
  skillEscDismissed.value !== `${skillToken.value.start}:${skillToken.value.query}`
)

// 弹层动态限高：向上展开空间不足（输入框距面板顶很近 / textarea 拖高）时，
// 固定 max-height 会被面板 overflow:hidden 裁掉顶部——而列表内部滚动的正是
// 被裁区域，表现为「只能看到靠下几条且滚不动」。按「输入盒顶沿到面板顶沿的
// 可用距离」实测限高（留 12px 呼吸位），上限 280、下限 120 保证可用
const panelEl = ref(null)
const inputBoxEl = ref(null)
const skillPopupMaxHeight = ref(280)
const measureSkillPopupHeight = () => {
  const box = inputBoxEl.value
  const panel = panelEl.value
  if (!box || !panel) return
  const available = box.getBoundingClientRect().top - panel.getBoundingClientRect().top - 12
  skillPopupMaxHeight.value = Math.max(120, Math.min(280, Math.round(available)))
}
// 弹层打开时测量（flush: post = 渲染后测量，改 maxHeight 触发二次收敛）；
// 输入事件中同步重测，覆盖弹层开着拖高 textarea 的场景
watch(skillPopupVisible, (v) => {
  if (v) measureSkillPopupHeight()
}, { flush: 'post' })
// 高亮越界守卫：候选收缩（继续输入过滤）时高亮夹回末项
watch(filteredSkillItems, (list) => {
  if (skillActiveIndex.value >= list.length) skillActiveIndex.value = Math.max(0, list.length - 1)
})

// skill 来源标签（对齐 AiFunctionConfigDialog 的 source 语义；builtin = claude 内置命令）
const skillSourceLabel = (source) =>
  ({ user: '用户级', project: '项目级', plugin: '插件', builtin: '内置' }[source] || source)

// 输入事件：记录 caret 供 token 解析并解除 Esc 驳回；重测弹层可用高度
// （textarea 拖高/面板布局变化后弹层开着时的空间补偿）。
// 懒加载不在此处触发——v-model 的 update:modelValue 晚于本 handler 执行，
// 此刻 inputText 还是旧值、token 解析失真；改由下方 token 活跃性 watch 驱动
const onChatInput = (e) => {
  lastCaret.value = e?.target?.selectionStart ?? 0
  skillEscDismissed.value = ''
  measureSkillPopupHeight()
}

// token 变活跃（行首敲入 /）即懒加载 skill 列表（store 内 skillsLoaded 防重入，
// 不挂载即拉）；已加载后重复触发为空操作
watch(() => skillToken.value.active, (active) => {
  if (active) aiChatStore.loadDiscoveredSkills()
})

// 补全：token 片段整体替换为 <command> + 尾随空格（空格使 token 结束、弹层随之关闭），
// 光标落位到空格之后，焦点收回输入框
const confirmSkillCompletion = (skill) => {
  if (!skill || !skillToken.value.active) return
  const { text, caret } = applyCompletion(inputText.value, skillToken.value, skill.command)
  inputText.value = text
  // caret 同步到新落位：token 因尾随空格结束、extractSlashToken 失活，弹层随之关闭
  lastCaret.value = caret
  skillActiveIndex.value = 0
  // 真实 EP ElInput 暴露 textarea ref，测试 stub 暴露原生 input——双通道尽力恢复 caret
  nextTick(() => {
    const ta = inputRef.value?.textarea ?? inputRef.value?.input
    ta?.setSelectionRange?.(caret, caret)
    ta?.focus?.()
  })
}

// ↑↓：弹层打开时移动高亮（拦默认防 textarea 光标移动）；关闭时零副作用
const onSkillArrow = (delta, e) => {
  if (!skillPopupVisible.value || e.isComposing || e.keyCode === 229) return
  e.preventDefault()
  const len = filteredSkillItems.value.length
  skillActiveIndex.value = (skillActiveIndex.value + delta + len) % len
}

// Tab：弹层打开时补全（拦默认焦点跳转）；关闭时不拦
const onSkillTab = (e) => {
  if (!skillPopupVisible.value || e.isComposing || e.keyCode === 229) return
  e.preventDefault()
  confirmSkillCompletion(filteredSkillItems.value[skillActiveIndex.value])
}

// Esc：弹层打开时记 token 驳回指纹关闭（继续打字/新输入可再弹，对齐 CLI）；
// 关闭时零副作用，不干扰上层对话框等 Esc 链路
const onSkillEsc = (e) => {
  if (!skillPopupVisible.value) return
  e.preventDefault()
  skillActiveIndex.value = 0
  skillEscDismissed.value = `${skillToken.value.start}:${skillToken.value.query}`
}

// 弹层「刷新」：强制重扫（SKILL.md 内容修改不触发目录 mtime，缓存可能陈旧）。
// 错误分流提示；弹层可见性是纯推导，刷新后候选变化自动反映
const onSkillRefresh = async () => {
  try {
    await aiChatStore.refreshDiscoveredSkills()
  } catch (error) {
    handleError('刷新 skill 列表失败: ', error)
  }
}

// Enter 发送 / Shift+Enter 换行（textarea 内组合键交给默认行为）。
// 中文 IME 选词确认的 Enter（isComposing / keyCode 229）不发送，防把拼音串直发。
// 弹层打开时 Enter 改为补全（不发送）：先判弹层再判 IME——IME 组合中 Enter
// 连补全也跳过（保持默认选词行为），弹层关闭路径一行未动
const onInputEnter = (e) => {
  if (skillPopupVisible.value) {
    if (e.isComposing || e.keyCode === 229) return
    e.preventDefault()
    confirmSkillCompletion(filteredSkillItems.value[skillActiveIndex.value])
    return
  }
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
  // 持续会话模型：进行中发送 = 排队注入（后端排队，前序轮 result 后执行），
  // 不再禁发
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
      // 发送成功清当前会话草稿；无会话首发路径（先建会话）切换时草稿被存入
      // 空串键，一并清除防残留复活
      const sendKey = chatDraftKey(aiChatStore.selectedChatSessionId)
      chatDrafts.delete(sendKey)
      if (sendKey !== '') chatDrafts.delete('')
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

// ===== 权限模式 / 模型 / 启动命令配置 =====

const persistChatSettings = async () => {
  try {
    await aiChatStore.saveChatSettings(
      aiChatStore.chatSettings.permissionMode,
      aiChatStore.chatSettings.modelName,
      aiChatStore.chatSettings.launcher
    )
  } catch (error) {
    handleError('保存对话设置失败: ', error)
  }
}

// 启动命令切换持久化（与权限模式/模型共用保存链路）
const setLauncher = async (launcher) => {
  try {
    await aiChatStore.saveChatSettings(
      aiChatStore.chatSettings.permissionMode,
      aiChatStore.chatSettings.modelName,
      launcher
    )
  } catch (error) {
    handleError('保存对话设置失败: ', error)
  }
}

// 启动命令下拉命令分派：默认项清空配置回默认 claude；候选项回填 PATH；
// 「指定路径…」经文件对话框选 exe 回填绝对路径（取消则保持现状）
const onLauncherCommand = async (cmd) => {
  if (cmd === LAUNCHER_CMD_DEFAULT) {
    await setLauncher('')
    return
  }
  if (cmd === LAUNCHER_CMD_PICK) {
    try {
      const picked = await OpenFileDialog('选择 AI 启动程序', [
        { DisplayName: '可执行文件', Pattern: '*.exe;*.cmd;*.bat' }
      ])
      if (!picked) return
      await setLauncher(picked)
    } catch (error) {
      // 对话框层错误（含取消）：静默降级，保留当前配置
      debug.log('选择启动程序失败:', error)
    }
    return
  }
  await setLauncher(cmd)
}

const onPermissionModeChange = async () => {
  if (aiChatStore.chatSettings.permissionMode === 'bypassPermissions') {
    ElMessage.warning('已选择「跳过确认」：AI 可直接修改代码与执行命令，请注意风险')
  }
  await persistChatSettings()
}

// ===== chat-task:* / chat-approval:* 事件监听（闭包精准注销，禁 EventsOff 全局移除） =====

let offQueued = null
let offStarted = null
let offOutput = null
let offDone = null
let offApprovalRequest = null
let offApprovalCancelled = null

const onTaskDone = async (result) => {
  const handled = await aiChatStore.handleChatTaskDone(result)
  if (!handled) return
  if (result?.canceled) {
    ElMessage.info('对话已取消')
  } else if (result?.error) {
    ElMessage.error('对话失败: ' + result.error)
  }
}

// ===== 工具授权审批弹窗（chat-approval:* 事件驱动） =====

// 审批请求 FIFO 队列 + 串行处理（ElMessageBox 单实例，逐个弹窗应答）
const approvalQueue = ref([])
const approvalBusy = ref(false)

const onApprovalRequest = (req) => {
  if (!req?.requestId) return
  approvalQueue.value = [...approvalQueue.value, req]
  processApprovalQueue()
}

// chat-approval:cancelled：轮结束/进程退出自动拒绝——撤队列中的待弹项；
// 正在展示的弹窗保留（用户应答时后端报「已失效」，静默降级不提示）
const onApprovalCancelled = (ev) => {
  approvalQueue.value = approvalQueue.value.filter(r => r.requestId !== ev?.requestId)
}

const processApprovalQueue = async () => {
  if (approvalBusy.value || !approvalQueue.value.length) return
  approvalBusy.value = true
  const req = approvalQueue.value[0]
  const inputText = (() => {
    try {
      const json = JSON.stringify(req.input ?? {}, null, 2)
      return json === '{}' ? '' : json
    } catch {
      return ''
    }
  })()
  const message = [
    req.description || `AI 请求调用工具「${req.toolName}」`,
    inputText
  ].filter(Boolean).join('\n')
  let allowed = false
  try {
    await ElMessageBox.confirm(message, `工具授权：${req.toolName}`, {
      confirmButtonText: '允许',
      cancelButtonText: '拒绝',
      type: 'warning'
    })
    allowed = true
  } catch {
    allowed = false
  }
  try {
    await RespondChatApproval(req.requestId, allowed)
  } catch (error) {
    // 请求已失效（轮结束/进程退出自动拒绝）属预期竞态，静默降级
    if (String(error?.message || error).includes('失效')) {
      debug.log('审批请求已失效:', error)
    } else {
      handleError('审批应答失败: ', error)
    }
  } finally {
    approvalQueue.value = approvalQueue.value.filter(r => r.requestId !== req.requestId)
    approvalBusy.value = false
    processApprovalQueue()
  }
}

onMounted(() => {
  // 先同步注册事件监听再异步加载数据：保证卸载路径必能拿到全部 off 闭包
  // （若先 await 加载，await 期间卸载则 off 闭包未赋值、监听器泄漏）
  offQueued = EventsOn('chat-task:queued', aiChatStore.onChatTaskQueued)
  offStarted = EventsOn('chat-task:started', aiChatStore.onChatTaskStarted)
  offOutput = EventsOn('chat-task:output', aiChatStore.onChatTaskOutput)
  offDone = EventsOn('chat-task:done', onTaskDone)
  offApprovalRequest = EventsOn('chat-approval:request', onApprovalRequest)
  offApprovalCancelled = EventsOn('chat-approval:cancelled', onApprovalCancelled)

  ;(async () => {
    await aiChatStore.loadChatDirectories()
    aiChatStore.loadChatSettings()
    // 启动命令候选：面板挂载扫描一次即可（纯只读，失败静默空列表）
    aiChatStore.loadChatCliCandidates()
    // claude 设置默认模型：模型下拉「默认」选项回显（纯只读，失败静默空串）
    aiChatStore.loadClaudeDefaultModel()
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
  offApprovalRequest && offApprovalRequest()
  offApprovalCancelled && offApprovalCancelled()
  offQueued = null
  offStarted = null
  offOutput = null
  offDone = null
  offApprovalRequest = null
  offApprovalCancelled = null
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

/* 启动命令下拉（会话工具条）：按钮标签限宽省略，菜单项 name + path 两列 */
.launcher-btn {
  max-width: 220px;
}
.launcher-btn-label {
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.launcher-menu .launcher-menu-name {
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
  vertical-align: bottom;
  font-weight: 500;
}
.launcher-menu .launcher-menu-path {
  margin-left: 10px;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
  vertical-align: bottom;
  font-size: 11px;
  font-family: Consolas, 'Courier New', monospace;
  color: var(--text-placeholder);
}
.launcher-menu .is-current .launcher-menu-name {
  color: var(--primary-color);
}

/* 权限模式（分段式按钮组）：非默认档警示色强调——选中项底色换 warning，
   默认档保持主色（弱化显示）。EP checked 态走 is-active */
.chat-perm-seg.is-elevated :deep(.el-radio-button.is-active .el-radio-button__inner) {
  background: var(--warning-color);
  border-color: var(--warning-color);
  box-shadow: -1px 0 0 0 var(--warning-color);
}
/* 分段组紧凑化：小字号 + 收窄内边距，防四档撑满底栏 */
.chat-perm-seg :deep(.el-radio-button__inner) {
  font-size: 12px;
  padding: 5px 10px;
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

/* 思考块（仅流式在途任务，done 即丢弃）：正文上方灰色折叠块，
   内容等宽小字灰字 pre-wrap；头部转圈 + 「思考中…」+ 收起箭头 */
.chat-thinking {
  margin-bottom: var(--spacing-sm);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  background: var(--bg-secondary);
}
.chat-thinking-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px var(--spacing-sm);
  cursor: pointer;
  user-select: none;
  font-size: 12px;
  color: var(--text-secondary);
}
.chat-thinking-spinner {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  border: 1.5px solid var(--border-color);
  border-top-color: var(--text-secondary);
  animation: chatThinkingSpin 1s linear infinite;
}
@keyframes chatThinkingSpin {
  to { transform: rotate(360deg); }
}
.chat-thinking-title {
  font-weight: 500;
}
.chat-thinking-arrow {
  margin-left: auto;
  color: var(--text-tertiary);
  transition: transform var(--transition-fast);
}
.chat-thinking-arrow.is-collapsed {
  transform: rotate(-90deg);
}
.chat-thinking-body {
  margin: 0 var(--spacing-sm) var(--spacing-sm);
  padding-top: var(--spacing-xs);
  border-top: 1px dashed var(--border-color);
  max-height: 200px;
  overflow-y: auto;
  font-family: Consolas, 'Monaco', monospace;
  font-size: 11px;
  line-height: 1.6;
  color: var(--text-tertiary);
  white-space: pre-wrap;
  word-break: break-word;
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

/* 代码块「在终端执行」操作条：普通代码块外包容器 + 头部次要操作行，
   边框/圆角上移到容器（pre 原有 margin/border 在容器内置零防双边框），
   按钮形态对齐 chat-question-option（描边小按钮，hover 主色） */
.chat-msg-md :deep(.cmd-run-wrap) {
  margin: 0.5em 0;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  overflow: hidden;
}
.chat-msg-md :deep(.cmd-run-wrap pre.hljs) {
  margin: 0;
  border: none;
  border-radius: 0;
}
.chat-msg-md :deep(.cmd-run-bar) {
  display: flex;
  justify-content: flex-end;
  padding: 2px 6px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-secondary);
}
.chat-msg-md :deep(.cmd-run-btn) {
  padding: 2px 10px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 1.5;
  cursor: pointer;
  transition: border-color var(--transition-fast), color var(--transition-fast);
}
.chat-msg-md :deep(.cmd-run-btn:hover) {
  border-color: var(--primary-color);
  color: var(--primary-color);
}
/* 无 cwd 会话置灰态（渲染层注入 is-disabled；点击 handler 内另有兜底拦截） */
.chat-msg-md :deep(.cmd-run-btn.is-disabled) {
  opacity: 0.45;
  cursor: not-allowed;
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
/* 输入盒：textarea 主体 + 盒内下沿操作条（模板/权限分段/停止发送），
   聚焦时边框转主色（claude.ai 同款容器语义） */
/* 对话状态栏：模型 │ 上下文渐变条（单行紧凑，精简版仅两段） */
.chat-statusline {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 3px 12px;
  font-size: 11px;
  line-height: 1.4;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
}
.chat-statusline-model {
  flex-shrink: 0;
  color: var(--text-primary);
  font-weight: 600;
}
.chat-statusline-ctx {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.chat-statusline-bar {
  width: 90px;
  height: 6px;
  border-radius: 3px;
  background: var(--border-color);
  overflow: hidden;
}
.chat-statusline-bar-fill {
  display: block;
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s ease;
}
.chat-statusline-ctx-text {
  font-variant-numeric: tabular-nums;
}
.chat-input-box {
  position: relative; /* skill 补全弹层绝对定位锚点（bottom:100% 向上展开） */
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: border-color var(--transition-fast);
}
.chat-input-box:focus-within {
  border-color: var(--primary-color);
}
/* skill 斜杠补全弹层：贴输入盒上沿向上展开，浮于消息区之上。
   max-height + 内部滚动兜底超长列表；视觉对齐模板下拉（同底色/边框/圆角） */
.skill-popup {
  position: absolute;
  bottom: calc(100% + 6px);
  left: 0;
  right: 0;
  max-height: 280px;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg, 0 4px 16px rgba(0, 0, 0, 0.12));
  z-index: 30;
  overflow: hidden;
}
.skill-popup-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
}
.skill-popup-title {
  font-size: 12px;
  color: var(--text-secondary);
}
.skill-popup-list {
  list-style: none;
  margin: 0;
  padding: 4px 0;
  overflow-y: auto;
  min-height: 0;
}
/* 显式滚动条：超长列表内部滚动可见可滚（对齐消息区滚动条形态） */
.skill-popup-list::-webkit-scrollbar {
  width: 8px;
}
.skill-popup-list::-webkit-scrollbar-thumb {
  background: var(--border-color);
  border-radius: 4px;
}
.skill-popup-list::-webkit-scrollbar-thumb:hover {
  background: var(--text-tertiary);
}
.skill-popup-list li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  cursor: pointer;
  font-size: 12px;
  line-height: 1.4;
}
.skill-popup-list li:hover,
.skill-popup-list li.is-active {
  background: var(--bg-hover, rgba(0, 0, 0, 0.06));
}
.skill-popup-cmd {
  color: var(--primary-color);
  font-weight: 600;
  flex-shrink: 0;
  font-family: var(--font-mono, monospace);
}
.skill-popup-name {
  flex-shrink: 0;
  color: var(--text-primary);
}
.skill-popup-desc {
  flex: 1;
  min-width: 0;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.skill-popup-source {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--text-tertiary, var(--text-secondary));
  border: 1px solid var(--border-color);
  border-radius: 3px;
  padding: 0 5px;
  line-height: 18px;
}
.tpl-btn {
  flex-shrink: 0;
}
.chat-input {
  min-width: 0;
}
/* 输入框高度：默认 6 行（rows），允许原生纵向拖拽调高。
   下限约 2 行防拖得过扁，上限 40vh 防拖到占满会话区；
   拖拽高度为会话内临时态，不持久化、不入会话快照。
   盒内无边框化：外层 .chat-input-box 承担边框与聚焦态，inner 透明融入
   EP 非 autosize 挂载时会写内联 min-height:31px，!important 压过运行时内联值 */
.chat-input :deep(.el-textarea__inner) {
  resize: vertical;
  min-height: 52px !important;
  max-height: 40vh;
  border: none;
  box-shadow: none;
  background: transparent;
}
/* 盒内底栏：模板 + 权限分段组靠左，停止/发送靠右 */
.chat-input-box-footer {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-top: 1px solid var(--border-color);
  background: var(--bg-secondary);
}
.chat-input-spacer {
  flex: 1;
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
  .chat-input-status-dot,
  .chat-thinking-spinner {
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
