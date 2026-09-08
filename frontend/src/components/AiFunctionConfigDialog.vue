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
          <el-form-item label="参数/后续段">
            <el-input
              v-model="advancedJSON"
              type="textarea"
              :autosize="{ minRows: 6, maxRows: 16 }"
              placeholder="params（参数输入）与 followUps（多段编排按钮）的 JSON 定义"
            />
            <div class="adv-error" v-if="advError">{{ advError }}</div>
          </el-form-item>
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
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { GetAiFunctions, SaveAiFunctions } from '../../wailsjs/go/main/App'

const props = defineProps({
  visible: { type: Boolean, default: false }
})
const emit = defineEmits(['update:visible', 'saved'])

const functions = ref([])
const selectedId = ref('')
const editing = ref(null)
const addDirsText = ref('')
const saving = ref(false)

// 高级字段（params/followUps/env/mcp）合并 JSON 编辑
const advancedJSON = ref('{}')
const advError = ref('')

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
  // 深拷贝进编辑态
  editing.value = JSON.parse(JSON.stringify(f))
  addDirsText.value = (f.addDirs || []).join(', ')
  advancedJSON.value = JSON.stringify(
    {
      params: f.params || null,
      followUps: f.followUps || [],
      env: f.env || null,
      mcp: f.mcp || null
    },
    null,
    2
  )
  advError.value = ''
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
  advancedJSON.value = JSON.stringify({ params: null, followUps: [], env: null, mcp: null }, null, 2)
  advError.value = ''
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
  // 高级 JSON 校验
  let adv
  try {
    adv = JSON.parse(advancedJSON.value || '{}')
  } catch (e) {
    advError.value = 'JSON 语法错误: ' + e.message
    return
  }
  advError.value = ''
  item.id = item.id.trim()
  item.addDirs = addDirsText.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean)
  item.params = adv.params || null
  item.followUps = adv.followUps || []
  item.env = adv.env || null
  item.mcp = adv.mcp || null

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
.adv-error {
  color: var(--danger-color);
  font-size: 12px;
  margin-top: 4px;
}
/* JSON 编辑区等宽字体（JSON 为代码语义） */
.config-form :deep(.el-textarea__inner) {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 12px;
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
