<template>
  <div class="ai-runner">
    <div class="runner-body">
      <!-- 顶部详情区：名称/描述 + 关键配置只读展示 -->
      <div class="runner-head">
        <div class="runner-title">
          <el-icon :size="20" class="runner-icon"><component :is="iconComp" /></el-icon>
          <span class="runner-name">{{ fn.name }}</span>
        </div>
        <div v-if="fn.description" class="runner-desc">{{ fn.description }}</div>
        <div class="runner-meta">
          <!-- 值为指令/路径语义：包 span 用等宽字体呈现，与正文字体的灰色键名形成 key-value 视觉差 -->
          <div class="meta-item"><span class="meta-label">命令</span><span class="meta-value">{{ fn.command || '—' }}</span></div>
          <div class="meta-item"><span class="meta-label">工作目录</span><span class="meta-value">{{ fn.cwd || '—' }}</span></div>
          <div class="meta-item"><span class="meta-label">超时</span><span class="meta-value">{{ timeoutText }}</span></div>
          <div class="meta-item"><span class="meta-label">完成动作</span><span class="meta-value">{{ completionText }}</span></div>
        </div>
      </div>

      <!-- 参数录入区（按 fn.params.type 分支） -->
      <!-- text：单值输入 -->
      <el-form v-if="paramType === 'text'" class="runner-params" label-width="110px" @submit.prevent>
        <el-form-item :label="paramsSpec.label || '参数'">
          <el-input
            v-model="textValue"
            :placeholder="paramsSpec.placeholder || '请输入'"
            @keyup.enter="handleRun"
          />
        </el-form-item>
      </el-form>

      <!-- file：文件多选（手输完整路径 + 浏览树点选，均进已选列表） -->
      <el-form v-else-if="paramType === 'file'" class="runner-params" label-width="110px" @submit.prevent>
        <el-form-item :label="paramsSpec.label || '选择文件'">
          <div class="file-row">
            <el-input
              v-model="textValue"
              placeholder="输入文件完整路径，回车或点「添加」"
              @keyup.enter="addManualFile"
            />
            <el-button @click="addManualFile">添加</el-button>
            <el-button @click="browserVisible = true">浏览</el-button>
          </div>
          <div v-if="selectedFiles.length" class="file-tags">
            <el-tag
              v-for="(f, i) in selectedFiles"
              :key="f"
              :title="f"
              closable
              disable-transitions
              @close="removeSelectedFile(i)"
            >
              {{ f }}
            </el-tag>
          </div>
          <div v-if="paramsSpec.extensions?.length" class="ext-hint">
            支持类型：{{ paramsSpec.extensions.join(' / ') }}
          </div>
        </el-form-item>
      </el-form>

      <!-- form：字段化表单 -->
      <el-form v-else-if="paramType === 'form'" class="runner-params" label-width="110px" @submit.prevent>
        <el-form-item
          v-for="f in paramsSpec.fields || []"
          :key="f.key"
          :label="f.label"
          :required="f.required"
        >
          <el-input-number
            v-if="f.type === 'number'"
            v-model="formValues[f.key]"
            :min="0.5"
            :step="0.5"
            :placeholder="f.placeholder"
            style="width: 220px"
          />
          <el-date-picker
            v-else-if="f.type === 'datetime'"
            v-model="formValues[f.key]"
            type="datetime"
            format="YYYY-MM-DD HH:mm"
            value-format="YYYY-MM-DD HH:mm"
            :placeholder="f.placeholder || '选择日期时间'"
            style="width: 220px"
          />
          <el-input
            v-else
            v-model="formValues[f.key]"
            :placeholder="f.placeholder"
            @keyup.enter="handleRun"
          />
        </el-form-item>
      </el-form>

      <!-- none / 未配置：无参数说明 -->
      <div v-else class="runner-params no-params">
        该功能无需参数，点击下方「运行」直接执行。
      </div>
    </div>

    <!-- 底部运行按钮（主色渐变主行动作） -->
    <div class="runner-footer">
      <el-button type="primary" class="run-btn" @click="handleRun">运行</el-button>
    </div>

    <!-- 简易文件浏览选择（多点累积加入已选，不关弹窗） -->
    <el-dialog v-model="browserVisible" title="选择文件" width="520px" append-to-body>
      <div class="browser-toolbar">
        <el-input v-model="browserPath" size="small" @keyup.enter="loadBrowserNodes">
          <template #append>
            <el-button @click="loadBrowserNodes">前往</el-button>
            <el-button @click="addBrowserPathAsSelected">添加为已选</el-button>
          </template>
        </el-input>
      </div>
      <el-tree
        :data="browserNodes"
        node-key="path"
        highlight-current
        :props="{ label: 'name', isLeaf: (data) => !data.hasChildren }"
        lazy
        :load="loadTreeNode"
        @node-click="onBrowserNodeClick"
        style="max-height: 360px; overflow: auto"
      />
      <div class="browser-selected">已选 {{ selectedFiles.length }} 个文件（点击文件加入，点击目录切换）</div>
      <template #footer>
        <el-button type="primary" @click="browserVisible = false">完成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import * as Icons from '@element-plus/icons-vue'
import { GetFileTree } from '../../wailsjs/go/main/App'

const props = defineProps({
  // AiFunction 配置对象（GetAiFunctions 返回项）
  fn: { type: Object, required: true }
})
const emit = defineEmits(['run'])

// 参数规格：fn.params 可能为 null（无参数直跑）
const paramsSpec = computed(() => props.fn?.params || {})
const paramType = computed(() => paramsSpec.value?.type || '')

const iconComp = computed(
  () => (props.fn?.icon && Icons[props.fn.icon]) || Icons.MagicStick
)

// 超时展示：0/未配置 表示用后端默认值 10 分钟
const timeoutText = computed(() =>
  props.fn?.timeoutMinutes ? `${props.fn.timeoutMinutes} 分钟` : '默认 10 分钟'
)

const COMPLETION_TEXT = {
  none: '无',
  open_dir: '打开产物目录',
  preview: '预览产物',
  copy: '复制输出'
}
const completionText = computed(
  () => COMPLETION_TEXT[props.fn?.completion] || props.fn?.completion || '无'
)

const textValue = ref('')
const formValues = ref({})
// file 模式已选文件路径列表（多选），以换行拼接后作为参数值
const selectedFiles = ref([])
const browserVisible = ref(false)
const browserPath = ref('')
const browserNodes = ref([])

// 手写日期时间格式化（不引 dayjs 等额外依赖），输出与 el-date-picker 的
// value-format 'YYYY-MM-DD HH:mm' 一致
const pad2 = (n) => String(n).padStart(2, '0')
const formatDateTime = (d) =>
  `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`

// 当前时间的下一个整点（如 10:37 → 11:00；恰为整点 10:00 → 11:00，
// +1 分钟缓冲保证整点瞬间 ceil 后落到下一小时而非当前整点）
const nextHourText = () =>
  formatDateTime(new Date(Math.ceil((Date.now() + 60000) / 3600000) * 3600000))

// 挂载时按参数规格初始化默认值：
//   form number 字段默认 1（时长等场景的基准值，用户可改 ≥0.5 的任意值）
//   form datetime 字段默认下一整点，用户可改
// 功能 Tab 常驻复用，重复运行不清空已录内容（用户改完再跑）
const initDefaults = () => {
  if (paramType.value === 'form') {
    for (const f of paramsSpec.value.fields || []) {
      if (f.type === 'number') {
        formValues.value[f.key] = 1
      } else if (f.type === 'datetime') {
        formValues.value[f.key] = nextHourText()
      } else {
        formValues.value[f.key] = ''
      }
    }
  }
}
initDefaults()

// 校验 + 组装参数后 emit('run', params)。
// 组装规则与原 AiParamDialog.handleConfirm 一致：
// file 用 \n join、form 全字段字符串化、text 单值 key（textFieldKey 兜底 text/file）。
const handleRun = () => {
  const spec = paramsSpec.value
  const type = spec?.type
  if (!type || type === 'none') {
    emit('run', {})
    return
  }
  if (type === 'text') {
    if (!textValue.value.trim()) {
      ElMessage.warning(`请输入${spec.label || '参数'}`)
      return
    }
    emit('run', { [spec.textFieldKey || 'text']: textValue.value.trim() })
    return
  }
  if (type === 'file') {
    if (selectedFiles.value.length === 0) {
      ElMessage.warning('请至少选择一个文件')
      return
    }
    emit('run', { [spec.textFieldKey || 'file']: selectedFiles.value.join('\n') })
    return
  }
  if (type === 'form') {
    for (const f of spec.fields || []) {
      if (f.required && String(formValues.value[f.key] || '').trim() === '') {
        ElMessage.warning(`请填写「${f.label}」`)
        return
      }
    }
    const params = {}
    for (const f of spec.fields || []) {
      params[f.key] = String(formValues.value[f.key] ?? '')
    }
    emit('run', params)
    return
  }
  emit('run', {})
}

// ===== file 多选 =====
const addManualFile = () => {
  const p = textValue.value.trim()
  if (!p) return
  if (!selectedFiles.value.includes(p)) selectedFiles.value.push(p)
  textValue.value = ''
}

const removeSelectedFile = (i) => {
  selectedFiles.value.splice(i, 1)
}

// 浏览弹窗路径输入框内容直接作为文件路径加入已选（不校验存在）
const addBrowserPathAsSelected = () => {
  const p = browserPath.value.trim()
  if (!p) return
  if (!selectedFiles.value.includes(p)) selectedFiles.value.push(p)
}

// ===== 文件浏览 =====
const initBrowser = () => {
  browserPath.value = paramsSpec.value?.startDir || 'D:\\'
  loadBrowserNodes()
}

const loadBrowserNodes = async () => {
  try {
    browserNodes.value = await GetFileTree(browserPath.value) || []
  } catch (e) {
    ElMessage.error('读取目录失败: ' + (e?.message || String(e)))
  }
}

const loadTreeNode = async (node, resolve) => {
  if (node.level === 0) return resolve([])
  try {
    resolve(await GetFileTree(node.data.path) || [])
  } catch {
    resolve([])
  }
}

const onBrowserNodeClick = (data) => {
  if (data.type === 'file') {
    // 点文件即加入已选（不关弹窗，可继续选）
    if (!selectedFiles.value.includes(data.path)) selectedFiles.value.push(data.path)
  } else if (data.type === 'dir') {
    browserPath.value = data.path
  }
}

// 打开浏览时初始化（watch browserVisible 上升沿）
watch(browserVisible, (v) => {
  if (v) initBrowser()
})
</script>

<style scoped>
/* 根元素吃满 tab-pane 高度：body 内部滚动，运行按钮常驻底部，参数多时不撑破弹窗 */
.ai-runner {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.runner-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.runner-head {
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 10px;
  margin-bottom: 12px;
}
.runner-title {
  display: flex;
  align-items: center;
  gap: 8px;
}
.runner-icon {
  color: var(--primary-color);
}
.runner-name {
  font-weight: 600;
  font-size: 15px;
}
.runner-desc {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 4px;
  line-height: 1.5;
}
.runner-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 18px;
  margin-top: 8px;
}
.meta-item {
  font-size: 12px;
  color: var(--text-secondary);
}
.meta-label {
  color: var(--text-tertiary);
  margin-right: 4px;
}
/* 指令/路径值：等宽字体呈现（与键名的正文字体形成对比） */
.meta-value {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  font-size: 12px;
}
.runner-params {
  max-width: 640px;
}
/* 参数字段 label 小节风格：前置主色竖条用 border-left 实现——
   不占用 label 的 ::before（该伪元素承载 Element Plus required 必填星号，避免覆盖丢失） */
.runner-params :deep(.el-form-item__label) {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  padding-left: 8px;
  border-left: 3px solid var(--primary-color);
  border-radius: 1px;
}
.no-params {
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
  padding: 10px 12px;
}
.runner-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 10px;
  margin-top: 8px;
  border-top: 1px solid var(--border-color);
}
/* 运行按钮（主行动作）：加大主色渐变、白字、hover 上浮、active 按压 */
.run-btn {
  padding: 10px 28px;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 2px;
  border: none;
  border-radius: var(--radius-md);
  color: #fff;
  background: linear-gradient(135deg, var(--primary-light), var(--primary-color));
  transition: transform 0.15s ease, box-shadow 0.2s ease;
}
.run-btn:hover,
.run-btn:focus {
  border: none;
  color: #fff;
  background: linear-gradient(135deg, var(--primary-light), var(--primary-color));
  transform: translateY(-1px);
  box-shadow: var(--shadow-md);
}
.run-btn:active {
  border: none;
  color: #fff;
  background: linear-gradient(135deg, var(--primary-light), var(--primary-color));
  transform: translateY(0) scale(0.97);
  box-shadow: none;
}
.file-row {
  display: flex;
  gap: 8px;
  width: 100%;
}
.file-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
  width: 100%;
}
/* 长路径 tag 不撑破面板：tag 宽度封顶容器，内部文本省略号截断（完整路径经 title 悬浮可见） */
.file-tags .el-tag {
  max-width: 100%;
}
.file-tags :deep(.el-tag__content) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0; /* el-tag 为 inline-flex，解除子项 min-width:auto 才能收缩出省略号 */
}
.ext-hint {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 4px;
}
.browser-toolbar {
  margin-bottom: 8px;
}
.browser-selected {
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-secondary);
  word-break: break-all;
}

/* 动画可访问性：用户系统偏好减少动效时，禁用本组件装饰动画与过渡 */
@media (prefers-reduced-motion: reduce) {
  .run-btn,
  .run-btn:hover,
  .run-btn:active {
    animation: none !important;
    transition: none !important;
    transform: none !important;
  }
}
</style>
