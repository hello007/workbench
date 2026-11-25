<template>
  <!-- 自然语言创建 AI 功能弹窗（配置管理弹窗左侧入口）：
       需求描述（必填）+ 工作目录（选填）+ 指定 skill（选填，候选 = 全局已发现
       ∪ 所选目录 .claude/skills）→ 调后端同步生成草稿 → 预览字段与警告 →
       「填入配置」把草稿作为新增项交回父组件（不落盘，保存全部时才写文件）。
       尺寸走内容多的大弹窗档 min(720px, 86vw)。 -->
  <el-dialog
    :model-value="visible"
    title="自然语言创建 AI 功能"
    width="min(720px, 86vw)"
    :close-on-click-modal="false"
    append-to-body
    destroy-on-close
    class="ai-create-dialog"
    @update:model-value="onVisibleChange"
    @closed="handleClosed"
  >
    <div class="create-form">
      <div class="form-field">
        <div class="field-label">
          需求描述 <span class="field-required">*</span>
        </div>
        <el-input
          v-model="description"
          type="textarea"
          :autosize="{ minRows: 3, maxRows: 6 }"
          :disabled="generating"
          placeholder="用一句话描述你想要的 AI 功能，如：把会议纪要转成周报"
        />
      </div>

      <div class="form-field">
        <div class="field-label">工作目录（选填）</div>
        <div class="dir-row">
          <el-input
            v-model="cwd"
            :disabled="generating"
            clearable
            placeholder="skill 所在项目根目录；留空则仅用全局已发现 skill"
            @change="refreshSkills"
          />
          <el-button size="small" :disabled="generating" @click="pickDirectory">选择目录</el-button>
        </div>
      </div>

      <div class="form-field">
        <div class="field-label">指定 skill（选填）</div>
        <el-select
          v-model="skillCommand"
          class="skill-select"
          size="small"
          clearable
          filterable
          :disabled="generating"
          :loading="skillsLoading"
          placeholder="不指定则由 AI 自动匹配已有 skill，或生成纯 prompt 功能"
        >
          <el-option
            v-for="s in skills"
            :key="skillKey(s)"
            :label="optionLabel(s)"
            :value="s.command"
          >
            <div class="skill-option">
              <span class="skill-option-name">{{ s.name }}</span>
              <span class="skill-option-desc">{{ s.description }}</span>
              <span class="skill-option-cmd">{{ s.command }}</span>
              <span class="skill-option-src">{{ skillSourceLabel(s) }}</span>
            </div>
          </el-option>
        </el-select>
        <div class="field-hint">
          候选来自全局已发现 skill 与所选目录下的 .claude/skills；指定后即使 AI 判断不匹配也以你的选择为准。
        </div>
      </div>
    </div>

    <div v-if="generating" class="gen-progress">
      <el-icon class="is-loading"><Loading /></el-icon>
      <span class="gen-progress-text">正在生成配置草稿，请稍候…</span>
      <el-button size="small" @click="cancelGenerate">中止生成</el-button>
    </div>

    <div v-if="result" class="draft-preview">
      <div class="preview-title">生成结果预览</div>

      <div v-if="result.renamed" class="preview-alert" data-testid="rename-hint">
        ID「{{ result.originalId }}」与已有功能冲突，已自动改为「{{ result.draft?.id }}」。
      </div>

      <div class="preview-grid">
        <div class="preview-row">
          <span class="preview-key">ID</span>
          <span class="preview-val preview-mono">{{ result.draft?.id || '（空）' }}</span>
        </div>
        <div class="preview-row">
          <span class="preview-key">名称</span>
          <span class="preview-val">{{ result.draft?.name || '（空）' }}</span>
        </div>
        <div class="preview-row">
          <span class="preview-key">描述</span>
          <span class="preview-val">{{ result.draft?.description || '（空）' }}</span>
        </div>
        <div class="preview-row">
          <span class="preview-key">斜杠命令</span>
          <span class="preview-val preview-mono">{{ result.draft?.command || '（无，纯 prompt 功能）' }}</span>
        </div>
        <div class="preview-row">
          <span class="preview-key">工作目录</span>
          <span class="preview-val preview-mono">{{ result.draft?.cwd || '（继承）' }}</span>
        </div>
        <div class="preview-row">
          <span class="preview-key">参数类型</span>
          <span class="preview-val">{{ paramsTypeLabel }}</span>
        </div>
        <div class="preview-row">
          <span class="preview-key">完成动作</span>
          <span class="preview-val">{{ completionLabel }}</span>
        </div>
      </div>

      <div v-if="advancedFields.length" class="preview-advanced" data-testid="advanced-fields">
        <div class="preview-advanced-title">高级字段（草稿已生成）</div>
        <div class="preview-advanced-hint">
          以下字段会随草稿一起保存，请确认内容符合预期后再「填入配置」。
        </div>
        <details
          v-for="f in advancedFields"
          :key="f.key"
          class="preview-advanced-item"
        >
          <summary class="preview-advanced-summary">{{ f.label }}</summary>
          <pre class="preview-advanced-json">{{ f.text }}</pre>
        </details>
      </div>

      <div v-if="warnings.length" class="preview-warnings">
        <div class="preview-warnings-title">可用性提示（不阻断，可继续编辑）</div>
        <ul class="preview-warnings-list">
          <li v-for="(w, i) in warnings" :key="i">{{ w }}</li>
        </ul>
      </div>
    </div>

    <template #footer>
      <el-button @click="closeDialog">取消</el-button>
      <el-button
        v-if="result"
        type="primary"
        plain
        :disabled="generating"
        @click="applyDraft"
      >
        填入配置
      </el-button>
      <el-button
        type="primary"
        :loading="generating"
        :disabled="!description.trim()"
        @click="generate"
      >
        生成
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { Loading } from '@element-plus/icons-vue'
import {
  GenerateAiFunctionDraft,
  CancelAiFunctionDraft,
  GetDiscoveredSkills,
  GetDiscoveredSkillsIn,
  OpenDirectoryDialog
} from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { isBrowserChannel } from '../transport'

const props = defineProps({
  visible: { type: Boolean, default: false }
})
const emit = defineEmits(['update:visible', 'created'])

const description = ref('')
const cwd = ref('')
const skillCommand = ref('')
const skills = ref([])
const skillsLoading = ref(false)
const generating = ref(false)
const result = ref(null)
// 生成代次（单调递增）：关闭/重开/中止时自增，旧请求回写前比对代次，
// 不匹配即丢弃结果与错误——修复「关闭后旧响应回溯污染新一轮生成」的竞态。
let genSeq = 0
// skill 候选请求代次：目录切换/重新打开时旧响应不得覆盖新候选，
// 也不得按旧候选清单清空用户已选 skill。
let skillsSeq = 0

// 浏览器通道不提供生成能力（后端 RPCHiddenMethods 屏蔽，需本地 claude 子进程）
const browserChannel = isBrowserChannel()

const warnings = computed(() => result.value?.warnings || [])

const paramsTypeLabel = computed(() => {
  const type = result.value?.draft?.params?.type
  return { form: 'form（表单字段）', file: 'file（选择文件）', text: 'text（单行文本）', none: 'none（无参数）' }[type] || type || '（无参数）'
})

const completionLabel = computed(() => {
  const c = result.value?.draft?.completion
  return { none: '无', open_dir: '打开产物目录', preview: '预览 HTML 产物', copy: '复制输出' }[c] || c || '无'
})

// 高级字段可见性：草稿携带 env/mcp/addDirs/followUps/outputSchema 时在预览中显式列出，
// 避免「危险配置看不见地落盘」；空对象/空数组/空白串视为未生成。
const advancedFields = computed(() => {
  const d = result.value?.draft
  if (!d) return []
  const entries = [
    ['env', '环境变量（env）', d.env],
    ['mcp', 'MCP server（mcp）', d.mcp],
    ['addDirs', '授权目录（addDirs）', d.addDirs],
    ['followUps', '后续段（followUps）', d.followUps],
    ['outputSchema', '输出结构（outputSchema）', d.outputSchema]
  ]
  const out = []
  for (const [key, label, value] of entries) {
    if (value == null) continue
    if (Array.isArray(value) && value.length === 0) continue
    if (typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0) continue
    if (typeof value === 'string' && value.trim() === '') continue
    const text = typeof value === 'string' ? value : JSON.stringify(value, null, 2)
    out.push({ key, label, text })
  }
  return out
})

const optionLabel = (s) => `${s.name} · ${s.command}`
const skillSourceLabel = (s) =>
  ({ user: '用户级', project: '项目级', plugin: '插件' }[s?.source] || s?.source || '')
// 唯一标识：command + 来源 + 来源目录（同名 command 已在 refreshSkills 按 command 去重）
const skillKey = (s) => `${s?.command || ''}|${s?.source || ''}|${s?.sourceDir || ''}`

// 拉取候选 skill：选定目录走目录扫描，否则用全局已发现清单。
// 旧响应（目录已切换/弹窗已重开）按 skillsSeq 丢弃，不覆盖新候选、不清空用户已选项。
const refreshSkills = async () => {
  const seq = ++skillsSeq
  skillsLoading.value = true
  try {
    const dir = cwd.value.trim()
    const list = dir ? await GetDiscoveredSkillsIn(dir) : await GetDiscoveredSkills()
    if (seq !== skillsSeq) return
    // 同名 command 多来源（全局与目录扫描可能重叠）：按 command 去重，
    // 避免 el-option 重复 key 与「选中的到底是哪一条」歧义；生成只依赖 command。
    const seen = new Set()
    const next = []
    for (const s of list || []) {
      if (!s || !s.command || seen.has(s.command)) continue
      seen.add(s.command)
      next.push(s)
    }
    skills.value = next
    // 目录切换后已选命令可能不在新候选内，清空避免提交失配
    if (skillCommand.value && !skills.value.some((s) => s.command === skillCommand.value)) {
      skillCommand.value = ''
    }
  } catch (e) {
    if (seq !== skillsSeq) return
    handleError('加载 skill 候选失败: ', e)
  } finally {
    if (seq === skillsSeq) skillsLoading.value = false
  }
}

// 选择工作目录回填并即时刷新候选
const pickDirectory = async () => {
  let dir
  try {
    dir = await OpenDirectoryDialog('选择工作目录')
  } catch (e) {
    handleError('打开目录选择器失败: ', e)
    return
  }
  if (!dir || Array.isArray(dir)) return // 取消返回空串
  cwd.value = dir
  await refreshSkills()
}

const resetState = () => {
  // 作废在途生成代次：旧请求未 settle 时重开不应卡住按钮，其结果也不得回写新一轮
  genSeq++
  description.value = ''
  cwd.value = ''
  skillCommand.value = ''
  result.value = null
  generating.value = false
}

watch(
  () => props.visible,
  (v) => {
    if (v) {
      resetState()
      refreshSkills()
    }
  }
)

const generate = async () => {
  if (generating.value) return
  const desc = description.value.trim()
  if (!desc) {
    ElMessage.warning('请先填写需求描述')
    return
  }
  if (browserChannel) {
    ElMessage.warning('自然语言创建仅在桌面模式可用（需本地 claude 子进程）')
    return
  }
  const seq = ++genSeq
  generating.value = true
  result.value = null
  try {
    const res = await GenerateAiFunctionDraft({
      description: desc,
      cwd: cwd.value.trim(),
      skillCommand: skillCommand.value || ''
    })
    if (seq !== genSeq) return // 已被关闭/重开/中止，丢弃旧代次结果
    result.value = res || null
  } catch (e) {
    if (seq !== genSeq) return // 用户主动中止或代次已作废，不弹错误
    // 后端消息已自解释（如「生成超时（上限 2 分钟）」），不再叠加「生成失败:」前缀
    handleError('', e)
  } finally {
    if (seq === genSeq) generating.value = false
  }
}

// 中止在途生成：先作废代次并复位按钮（不等待请求 settle），再通知后端取消 claude 子进程；
// 取消错误与旧结果由代次比对静默丢弃。
const cancelGenerate = async () => {
  if (!generating.value) return
  genSeq++
  generating.value = false
  try {
    await CancelAiFunctionDraft()
  } catch {
    // 取消请求失败不额外提示：后端无在途任务时返回 false 属正常
  }
}

const applyDraft = () => {
  if (!result.value?.draft) return
  emit('created', result.value.draft)
  emit('update:visible', false)
}

const closeDialog = () => {
  emit('update:visible', false)
}

const onVisibleChange = (v) => emit('update:visible', v)

// 关闭收尾：仍在生成则中止，防 claude 子进程与并发槽位泄漏
const handleClosed = () => {
  if (generating.value) cancelGenerate()
}

onBeforeUnmount(() => {
  if (generating.value) cancelGenerate()
})
</script>

<style scoped>
.create-form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}
.form-field {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}
.field-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
}
.field-required {
  color: var(--danger-color, #f56c6c);
}
.field-hint {
  font-size: 12px;
  color: var(--text-tertiary);
  line-height: 1.5;
}
.dir-row {
  display: flex;
  gap: var(--spacing-sm);
  align-items: center;
}
.dir-row .el-input {
  flex: 1;
}
.skill-select {
  width: 100%;
}
.skill-option {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  overflow: hidden;
}
.skill-option-name {
  font-weight: 500;
}
.skill-option-desc {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-tertiary);
  font-size: 12px;
}
.skill-option-cmd {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-tertiary);
}
/* 候选项来源标签：同名命令多来源被去重后，仍标明该候选的发现来源 */
.skill-option-src {
  flex-shrink: 0;
  padding: 0 var(--spacing-xs);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  font-size: 11px;
  color: var(--text-tertiary);
}
/* 生成中进度条：左侧 loading 图标 + 文案 + 中止按钮 */
.gen-progress {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-md);
  background: var(--primary-bg);
  color: var(--primary-color);
}
.gen-progress-text {
  flex: 1;
  font-size: 13px;
}
/* 草稿预览：标题 + 重命名提示 + 字段网格 + 警告列表 */
.draft-preview {
  margin-top: var(--spacing-md);
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  background: var(--bg-tertiary);
}
.preview-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: var(--spacing-sm);
}
.preview-alert {
  margin-bottom: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-left: 3px solid var(--warning-color);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  font-size: 12px;
  color: var(--text-primary);
}
.preview-grid {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}
.preview-row {
  display: flex;
  gap: var(--spacing-sm);
  font-size: 12px;
  line-height: 1.5;
}
.preview-key {
  flex-shrink: 0;
  width: 72px;
  color: var(--text-tertiary);
}
.preview-val {
  flex: 1;
  min-width: 0;
  color: var(--text-primary);
  word-break: break-all;
}
.preview-mono {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
}
/* 高级字段：可折叠 JSON + 警示文案，避免危险配置「看不见地」落盘 */
.preview-advanced {
  margin-top: var(--spacing-sm);
  padding-top: var(--spacing-sm);
  border-top: 1px solid var(--border-color);
}
.preview-advanced-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}
.preview-advanced-hint {
  margin-top: var(--spacing-xs);
  font-size: 12px;
  color: var(--warning-color);
  line-height: 1.5;
}
.preview-advanced-item {
  margin-top: var(--spacing-xs);
  font-size: 12px;
}
.preview-advanced-summary {
  cursor: pointer;
  color: var(--text-secondary);
  font-weight: 500;
}
.preview-advanced-json {
  margin: var(--spacing-xs) 0 0;
  padding: var(--spacing-sm);
  border-radius: var(--radius-sm);
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 160px;
  overflow-y: auto;
}
.preview-warnings {
  margin-top: var(--spacing-sm);
  padding-top: var(--spacing-sm);
  border-top: 1px solid var(--border-color);
}
.preview-warnings-title {
  font-size: 12px;
  font-weight: 500;
  color: var(--warning-color);
  margin-bottom: var(--spacing-xs);
}
.preview-warnings-list {
  margin: 0;
  padding-left: var(--spacing-md);
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}
</style>

<style>
/* 弹窗外壳圆角：append-to-body 后 .el-dialog 挂在 body 下，需全局类命中 */
.ai-create-dialog {
  border-radius: var(--radius-md);
  overflow: hidden;
}
</style>
