<template>
  <el-dialog
    :model-value="visible"
    title="AI 功能配置管理"
    width="860px"
    class="ai-config-dialog"
    append-to-body
    destroy-on-close
    @update:model-value="$emit('update:visible', $event)"
  >
    <div class="config-layout">
      <!-- 左：功能项列表 -->
      <div class="config-list">
        <div
          v-for="f in functions"
          :key="f.id"
          class="config-item"
          :class="{ active: f.id === selectedId }"
          @click="select(f.id)"
        >
          <div class="config-item-name">{{ f.name }}</div>
          <div class="config-item-id">{{ f.id }}</div>
        </div>
        <el-button class="add-btn" size="small" @click="addNew">+ 新增功能</el-button>
      </div>

      <!-- 右：编辑表单 -->
      <div class="config-form" v-if="editing">
        <el-form label-width="92px" size="small">
          <el-form-item label-width="0" class="import-row">
            <el-button type="primary" plain size="small" @click="openImport">从已发现 skill 导入</el-button>
            <span class="import-hint">导入后用下方表单补全参数/完成动作等字段</span>
          </el-form-item>
          <el-form-item label="ID" required>
            <el-input v-model="editing.id" placeholder="唯一标识，如 my-skill" />
          </el-form-item>
          <el-form-item label="名称" required>
            <el-input v-model="editing.name" />
          </el-form-item>
          <el-form-item label="描述">
            <el-input v-model="editing.description" />
          </el-form-item>
          <el-form-item label="斜杠命令" required>
            <el-input v-model="editing.command" placeholder="如 /ab-weekly-report（插件 skill 带命名空间）" />
          </el-form-item>
          <el-form-item label="工作目录" required>
            <el-input v-model="editing.cwd" placeholder="skill 所在项目根目录（决定 skill 发现与 env 生效）" />
          </el-form-item>
          <el-form-item label="授权目录">
            <el-input v-model="addDirsText" placeholder="额外授权目录，逗号分隔（--add-dir）" />
          </el-form-item>
          <el-form-item label="权限模式">
            <el-select v-model="editing.permissionMode" style="width: 220px">
              <el-option label="bypassPermissions（放行）" value="bypassPermissions" />
              <el-option label="acceptEdits（放行编辑）" value="acceptEdits" />
              <el-option label="default（默认询问）" value="default" />
            </el-select>
          </el-form-item>
          <el-form-item label="超时(分钟)">
            <el-input-number v-model="editing.timeoutMinutes" :min="1" :max="120" />
          </el-form-item>
          <el-form-item label="完成动作">
            <el-select v-model="editing.completion" style="width: 220px">
              <el-option label="无" value="none" />
              <el-option label="打开产物目录" value="open_dir" />
              <el-option label="预览 HTML 产物" value="preview" />
              <el-option label="复制输出" value="copy" />
            </el-select>
          </el-form-item>
          <el-form-item label="图标">
            <el-input v-model="editing.icon" placeholder="Element Plus 图标名，如 MagicStick" style="width: 220px" />
          </el-form-item>
          <el-form-item label="标签">
            <el-input v-model="tagsText" placeholder="业务域标签，逗号分隔（如 周报,会议,文档）" />
          </el-form-item>
          <el-form-item label="置顶">
            <el-switch v-model="editing.pinned" />
            <span class="form-hint">置顶功能始终排在列表最前</span>
          </el-form-item>

          <!-- 高级字段：四块折叠面板 + 末项原始 JSON 兜底视图 -->
          <el-collapse v-model="activeNames" class="adv-collapse">
            <el-collapse-item title="参数（params）" name="params">
              <ParamsEditor ref="paramsEditorRef" v-model="editing.params" />
            </el-collapse-item>
            <el-collapse-item title="后续段（followUps）" name="followUps">
              <FollowUpsEditor ref="followUpsEditorRef" v-model="editing.followUps" />
            </el-collapse-item>
            <el-collapse-item title="环境变量（env）" name="env">
              <EnvEditor v-model="editing.env" />
            </el-collapse-item>
            <el-collapse-item title="MCP server（mcp）" name="mcp">
              <McpEditor ref="mcpEditorRef" v-model="editing.mcp" />
            </el-collapse-item>
            <el-collapse-item title="原始 JSON（高级）" name="raw">
              <el-input
                v-model="rawJsonText"
                type="textarea"
                :autosize="{ minRows: 6, maxRows: 16 }"
                :class="{ 'raw-error': rawJsonError }"
                @change="onRawJsonChange"
              />
              <div class="raw-error-msg" v-if="rawJsonError">{{ rawJsonError }}</div>
              <div class="raw-hint" v-else>
                与上方表单双向同步：表单改 → JSON 自动更新；JSON 编辑后失焦解析回写表单，解析失败标红且表单保持原值。
              </div>
            </el-collapse-item>
          </el-collapse>
        </el-form>
      </div>
      <div class="config-form config-empty" v-else>
        左侧选择或新增功能项
      </div>
    </div>

    <template #footer>
      <el-button
        v-if="editing"
        type="danger"
        plain
        @click="removeSelected"
      >
        删除该功能
      </el-button>
      <el-button @click="$emit('update:visible', false)">关闭</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存全部</el-button>
    </template>

    <!-- 导入 skill 对话框：列出用户级/项目级/插件 skill，模糊搜索，选中回填 -->
    <el-dialog
      v-model="importVisible"
      title="从已发现 skill 导入"
      width="880px"
      append-to-body
      destroy-on-close
      class="ai-import-dialog"
    >
      <div class="import-toolbar">
        <el-input
          v-model="importKeyword"
          placeholder="按名称/描述模糊搜索"
          clearable
          size="small"
          style="width: 280px"
        />
        <el-button size="small" :loading="importLoading" @click="refreshImport">刷新</el-button>
        <span class="import-count">共 {{ filteredSkills.length }} 项</span>
      </div>
      <el-table
        :data="filteredSkills"
        v-loading="importLoading"
        height="400"
        size="small"
        highlight-current-row
        empty-text="未发现 skill（用户级 ~/.claude/skills、各工作目录 .claude/skills、已安装插件）"
        @current-change="onImportSelect"
        @row-dblclick="confirmImport"
      >
        <el-table-column prop="name" label="名称" width="160" show-overflow-tooltip />
        <el-table-column prop="description" label="描述" min-width="240" show-overflow-tooltip />
        <el-table-column label="来源" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="sourceTagType(row.source)">{{ sourceLabel(row.source) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="command" label="命令" width="180" show-overflow-tooltip />
        <el-table-column label="目录" width="200" show-overflow-tooltip>
          <template #default="{ row }">{{ row.cwd || '（任意/待补）' }}</template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="importVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!importSelected" @click="confirmImport">导入</el-button>
      </template>
    </el-dialog>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { GetAiFunctions, SaveAiFunctions, GetDiscoveredSkills, RefreshDiscoveredSkills } from '../../wailsjs/go/main/App'
import ParamsEditor from './ParamsEditor.vue'
import FollowUpsEditor from './FollowUpsEditor.vue'
import EnvEditor from './EnvEditor.vue'
import McpEditor from './McpEditor.vue'

const props = defineProps({
  visible: { type: Boolean, default: false }
})
const emit = defineEmits(['update:visible', 'saved'])

const functions = ref([])
const selectedId = ref('')
const editing = ref(null)
const addDirsText = ref('')
const tagsText = ref('')
const saving = ref(false)

// 折叠面板：默认展开 params，其余收起
const activeNames = ref(['params'])

// 原始 JSON 兜底视图：与四块表单双向同步
const rawJsonText = ref('{}')
const rawJsonError = ref('')

// 子组件 ref（收集字段级校验）
const paramsEditorRef = ref()
const followUpsEditorRef = ref()
const mcpEditorRef = ref()

// 导入 skill 对话框状态
const importVisible = ref(false)
const importLoading = ref(false)
const importKeyword = ref('')
const importSkills = ref([])
const importSelected = ref(null)

// 模糊搜索：按名称/描述过滤（不区分大小写）
const filteredSkills = computed(() => {
  const kw = importKeyword.value.trim().toLowerCase()
  if (!kw) return importSkills.value
  return importSkills.value.filter(
    (s) =>
      (s.name || '').toLowerCase().includes(kw) ||
      (s.description || '').toLowerCase().includes(kw)
  )
})

// 打开导入对话框并拉取已发现 skill 列表（GetDiscoveredSkills 带 mtime 缓存，二次打开瞬时）
const openImport = async () => {
  importVisible.value = true
  importKeyword.value = ''
  importSelected.value = null
  importSkills.value = []
  importLoading.value = true
  try {
    importSkills.value = (await GetDiscoveredSkills()) || []
  } catch (e) {
    ElMessage.error('加载 skill 列表失败: ' + (e?.message || String(e)))
  } finally {
    importLoading.value = false
  }
}

// 强制重扫（清除缓存），供导入对话框「刷新」按钮调用
const refreshImport = async () => {
  importLoading.value = true
  try {
    importSkills.value = (await RefreshDiscoveredSkills()) || []
    importSelected.value = null
  } catch (e) {
    ElMessage.error('刷新失败: ' + (e?.message || String(e)))
  } finally {
    importLoading.value = false
  }
}

// el-table 选中行变更
const onImportSelect = (row) => {
  importSelected.value = row || null
}

// 确认导入：command/cwd 覆盖（导入核心目的是修正命名空间与目录），
// description/name 仅在当前为空时回填（保留用户已编辑的名称与描述）
const confirmImport = () => {
  const s = importSelected.value
  if (!s || !editing.value) return
  editing.value.command = s.command
  editing.value.cwd = s.cwd || ''
  if (!editing.value.description) editing.value.description = s.description || ''
  if (!editing.value.name) editing.value.name = s.name
  importVisible.value = false
  ElMessage.success(`已导入 ${s.command}`)
}

const sourceLabel = (src) => ({ user: '用户级', project: '项目级', plugin: '插件' }[src] || src)
const sourceTagType = (src) => ({ user: 'info', project: 'success', plugin: 'warning' }[src] || '')

const selected = computed(() => functions.value.find((f) => f.id === selectedId.value) || null)

const load = async () => {
  try {
    functions.value = (await GetAiFunctions()) || []
    if (functions.value.length && !selectedId.value) {
      select(functions.value[0].id)
    }
  } catch (e) {
    ElMessage.error('加载配置失败: ' + (e?.message || String(e)))
  }
}

watch(
  () => props.visible,
  (v) => {
    if (v) {
      selectedId.value = ''
      editing.value = null
      load()
    }
  }
)

const select = (id) => {
  selectedId.value = id
  const f = functions.value.find((x) => x.id === id)
  if (!f) return
  // 深拷贝进编辑态（子组件直接改 editing.value.xxx 属性，reactive 触发更新）
  editing.value = JSON.parse(JSON.stringify(f))
  addDirsText.value = (f.addDirs || []).join(', ')
  tagsText.value = (f.tags || []).join(', ')
  activeNames.value = ['params']
  syncRawFromForm()
}

const addNew = () => {
  const item = {
    id: '',
    name: '',
    description: '',
    icon: 'MagicStick',
    command: '',
    cwd: '',
    addDirs: [],
    tags: [],
    pinned: false,
    env: null,
    mcp: null,
    permissionMode: 'bypassPermissions',
    timeoutMinutes: 10,
    completion: 'none',
    params: null,
    followUps: []
  }
  functions.value.push(item)
  editing.value = item
  selectedId.value = ''
  addDirsText.value = ''
  tagsText.value = ''
  activeNames.value = ['params']
  syncRawFromForm()
}

const removeSelected = async () => {
  if (!editing.value) return
  const target = editing.value
  try {
    await ElMessageBox.confirm(`确定删除功能「${target.name || target.id}」？`, '删除确认', {
      type: 'warning'
    })
  } catch {
    return
  }
  const idx = functions.value.indexOf(target)
  if (idx >= 0) functions.value.splice(idx, 1)
  editing.value = null
  selectedId.value = ''
}

// 表单 → 原始 JSON：四块字段任一变化即序列化；序列化结果与当前文本一致则不覆盖（避免 JSON 编辑时回写触发抖动）
const syncRawFromForm = () => {
  if (!editing.value) return
  const next = JSON.stringify(
    {
      params: editing.value.params || null,
      followUps: editing.value.followUps || [],
      env: editing.value.env || null,
      mcp: editing.value.mcp || null
    },
    null,
    2
  )
  if (next !== rawJsonText.value) {
    rawJsonText.value = next
  }
}

// 四块字段 deep watch：子组件改属性/增删即触发同步原始 JSON
watch(
  [
    () => editing.value?.params,
    () => editing.value?.followUps,
    () => editing.value?.env,
    () => editing.value?.mcp
  ],
  syncRawFromForm,
  { deep: true }
)

// 原始 JSON → 表单：失焦时解析，成功回写四块字段，失败标红且表单保持原值不回写
const onRawJsonChange = () => {
  try {
    const parsed = JSON.parse(rawJsonText.value || '{}')
    if (editing.value) {
      editing.value.params = parsed.params ?? null
      editing.value.followUps = parsed.followUps ?? []
      editing.value.env = parsed.env ?? null
      editing.value.mcp = parsed.mcp ?? null
    }
    rawJsonError.value = ''
  } catch (e) {
    rawJsonError.value = 'JSON 解析失败: ' + (e?.message || String(e)) + '（表单保持原值，修正后再同步）'
  }
}

// 收集字段级校验错误：followUps / mcp 阻断保存；params 占位符不一致仅告警不阻断（ParamsEditor 内部已展示）
const collectErrors = () => {
  const errs = []
  const fuErrs = followUpsEditorRef.value?.validationErrors || []
  errs.push(...fuErrs)
  const mcpErrs = mcpEditorRef.value?.validationErrors || []
  errs.push(...mcpErrs)
  return errs
}

const save = async () => {
  const item = editing.value
  if (!item) {
    // 未选中任何项也可保存（纯删除场景）
    await doSave()
    return
  }
  if (!item.id.trim() || !item.name.trim() || !item.command.trim() || !item.cwd.trim()) {
    ElMessage.warning('ID、名称、斜杠命令、工作目录为必填')
    return
  }
  // id 唯一性
  const dup = functions.value.filter((f) => f.id === item.id.trim())
  if (dup.length > 1) {
    ElMessage.warning(`ID「${item.id}」重复`)
    return
  }
  // 字段级校验：失败定位到具体字段（如 followUps[1].label 不能为空）
  const errors = collectErrors()
  if (errors.length) {
    ElMessage.warning('配置校验失败：\n' + errors.join('\n'))
    return
  }
  item.id = item.id.trim()
  item.addDirs = addDirsText.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean)
  item.tags = tagsText.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean)
  // params/followUps/env/mcp 已被子组件直接改 editing.value，无需再赋值；followUps 兜底为 []
  item.followUps = item.followUps || []
  await doSave()
}

const doSave = async () => {
  saving.value = true
  try {
    await SaveAiFunctions(functions.value)
    ElMessage.success('配置已保存')
    emit('saved')
    emit('update:visible', false)
  } catch (e) {
    ElMessage.error('保存失败: ' + (e?.message || String(e)))
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.config-layout {
  display: flex;
  gap: 12px;
  height: 520px;
}
.config-list {
  width: 200px;
  flex-shrink: 0;
  border-right: 1px solid var(--border-color);
  padding-right: 12px;
  overflow: auto;
}
/* 列表项 active 态：主色淡底 + 左侧 3px 主色指示条（伪元素实现） */
.config-item {
  position: relative;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  margin-bottom: 4px;
  transition: background 0.15s ease;
}
.config-item:hover {
  background: var(--bg-tertiary);
}
.config-item.active {
  background: var(--primary-bg);
}
.config-item.active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 8px;
  bottom: 8px;
  width: 3px;
  border-radius: 2px;
  background: var(--primary-color);
}
.config-item-name {
  font-size: 13px;
  font-weight: 500;
}
.config-item-id {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 11px;
  color: var(--text-tertiary);
}
.add-btn {
  width: 100%;
  margin-top: 4px;
}
.config-form {
  flex: 1;
  overflow: auto;
}
.config-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary);
}
/* 导入 skill 入口与对话框样式 */
.import-row {
  margin-bottom: 8px;
}
.import-row .el-button {
  margin-right: 8px;
}
.import-hint {
  font-size: 12px;
  color: var(--text-tertiary);
}
.form-hint {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-left: 8px;
}
.import-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}
.import-count {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-left: auto;
}
/* 高级字段折叠面板：无外框，与上方 el-form 一体 */
.adv-collapse {
  border: none;
  margin-top: 4px;
}
.adv-collapse :deep(.el-collapse-item__header) {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-color);
}
.adv-collapse :deep(.el-collapse-item__wrap) {
  border-bottom: none;
}
.adv-collapse :deep(.el-collapse-item__content) {
  padding: 12px 0 4px;
}
/* 原始 JSON 兜底视图：等宽字体（代码语义），解析失败标红边框 */
.adv-collapse :deep(.el-textarea__inner) {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 12px;
}
.raw-error :deep(.el-textarea__inner) {
  border-color: var(--danger-color, #f56c6c);
}
.raw-error-msg {
  color: var(--danger-color, #f56c6c);
  font-size: 12px;
  margin-top: 4px;
}
.raw-hint {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 4px;
  line-height: 1.5;
}

/* 动画可访问性：用户系统偏好减少动效时，禁用装饰过渡 */
@media (prefers-reduced-motion: reduce) {
  .config-item {
    transition: none !important;
  }
}
</style>

<style>
/* 弹窗外壳圆角：append-to-body 后 .el-dialog 挂在 body 下，
   scoped 选择器无法命中，用全局唯一样式类控制 */
.ai-config-dialog {
  border-radius: var(--radius-md);
  overflow: hidden;
}
</style>
