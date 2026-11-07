<template>
  <el-dialog
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    @closed="releaseDiffData"
    width="80%"
    append-to-body
    destroy-on-close
  >
    <template #header>
      <div class="diff-dialog-header">
        <span
          class="diff-dialog-title"
          :title="traceableFile ? '右键：文件历史 / 行归属' : ''"
          @contextmenu.prevent.stop="onHeaderContextMenu($event)"
        >{{ dialogTitle }}</span>
        <el-tooltip :content="externalDiffTooltip" placement="top">
          <span>
            <el-button
              size="small"
              :disabled="!externalDiffAvailable || mode === 'range'"
              :loading="openingExternal"
              @click="openExternalDiff()"
            >用外部工具打开</el-button>
          </span>
        </el-tooltip>
      </div>
    </template>
    <div v-loading="loading" class="diff-body">
      <!-- 错误 -->
      <div v-if="error" class="diff-empty">{{ error }}</div>

      <!-- 空差异 / 二进制 -->
      <div v-else-if="!loading && left.length === 0 && right.length === 0 && fileGroups.length === 0" class="diff-empty">
        {{ binaryHint ? binaryHint : '无差异' }}
      </div>

      <!-- range 模式：多文件分组双栏 -->
      <div v-else-if="mode === 'range' && fileGroups.length > 0" class="range-groups">
        <div v-for="(group, gi) in fileGroups" :key="'g' + gi" class="range-file-group">
          <div class="range-file-header">
            <span
              class="range-file-name"
              title="右键：文件历史 / 行归属"
              @contextmenu.prevent.stop="openTraceMenu($event, group.file)"
            >{{ group.file }}</span>
            <el-tooltip :content="externalDiffTooltip" placement="top">
              <span>
                <el-button
                  size="small"
                  text
                  :disabled="!settingsStore.diffToolConfigured || group.binary"
                  @click="openExternalDiff(group.file)"
                >用外部工具打开</el-button>
              </span>
            </el-tooltip>
          </div>
          <div v-if="group.binary" class="diff-empty range-binary">该文件为二进制文件，不支持文本 diff 展示</div>
          <div v-else class="diff-table">
            <div class="diff-col diff-col-left">
              <div class="diff-col-header">{{ leftHeader }}</div>
              <div class="diff-col-body">
                <div
                  v-for="(line, idx) in group.left"
                  :key="'gl' + gi + idx"
                  class="diff-line"
                  :class="lineClass(line)"
                ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
              </div>
            </div>
            <div class="diff-col diff-col-right">
              <div class="diff-col-header">{{ rightHeader }}</div>
              <div class="diff-col-body">
                <div
                  v-for="(line, idx) in group.right"
                  :key="'gr' + gi + idx"
                  class="diff-line"
                  :class="lineClass(line)"
                ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 单文件双栏 diff -->
      <div v-else class="diff-table">
        <div class="diff-col diff-col-left">
          <div class="diff-col-header">{{ leftHeader }}</div>
          <div class="diff-col-body">
            <div
              v-for="(line, idx) in left"
              :key="'l' + idx"
              class="diff-line"
              :class="lineClass(line)"
            ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
          </div>
        </div>
        <div class="diff-col diff-col-right">
          <div class="diff-col-header">{{ rightHeader }}</div>
          <div class="diff-col-body">
            <div
              v-for="(line, idx) in right"
              :key="'r' + idx"
              class="diff-line"
              :class="lineClass(line)"
            ><span class="diff-line-no">{{ line.no || '' }}</span><span class="diff-line-text">{{ line.text }}</span></div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button type="primary" @click="$emit('update:modelValue', false)">关闭</el-button>
    </template>
  </el-dialog>

  <!-- diff 文件追溯右键菜单：查看文件历史 / 行归属（行归属经 uiStore 信箱联动预览） -->
  <TraceFileContextMenu ref="traceMenuRef" @command="onTraceCommand" />

  <!-- 文件提交历史弹窗（右键「查看文件历史」）：repoPath=fileAbsPath 同传绝对路径
       （对齐 FileTreePanel 先例），initialSha 按 mode 取所在/区间终点提交定位。
       v-if 外控：FileHistoryDialog 内也自挂 FileDiffDialog，二者互相引用成环——
       静态渲染下（如测试 stub 恒渲染 slot）会无限递归，关闭即卸载切断。 -->
  <FileHistoryDialog
    v-if="traceHistoryVisible"
    v-model="traceHistoryVisible"
    :repo-path="traceHistoryAbsPath"
    :file-abs-path="traceHistoryAbsPath"
    :initial-sha="traceHistorySha"
  />
</template>

<script setup>
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { GetFileDiff, GetCommitFileDiff, GetRangeDiff, OpenInExternalDiff } from '../../wailsjs/go/main/App'
import { useSettingsStore } from '../store'
import { handleError } from '../utils/error'
// unified diff 解析共享 util（原内嵌实现原样抽出，分支对比 BranchCompareDiffViewer 同用一份）
import { parseDiff, parseRangeDiff } from '../utils/diffParse'
import TraceFileContextMenu from './TraceFileContextMenu.vue'
import FileHistoryDialog from './FileHistoryDialog.vue'
import { joinRepoPath } from '../utils/pathMatch'
import { requestTraceBlame, blameCapableExt } from '../composables/useTraceBlame'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  repoPath: { type: String, required: true },
  file: { type: String, default: '' },
  // diff 来源模式：
  //   'workspace'（默认）：工作区单文件，调 GetFileDiff(repoPath, file)
  //   'commit'：指定提交中单文件，调 GetCommitFileDiff(repoPath, sha, file)
  //   'range'：两个提交区间全文件，调 GetRangeDiff(repoPath, baseSha, headSha)
  mode: { type: String, default: 'workspace' },
  sha: { type: String, default: '' },
  baseSha: { type: String, default: '' },
  headSha: { type: String, default: '' }
})

defineEmits(['update:modelValue'])

const loading = ref(false)
const error = ref('')
const binaryHint = ref('')
const left = ref([])
const right = ref([])
// range 模式：按 diff --git 头拆分的多文件分组，每项 { file, left, right }
const fileGroups = ref([])

const settingsStore = useSettingsStore()

// 外部 diff 打开中状态（按钮 loading，防重复点击）
const openingExternal = ref(false)

// 单文件模式当前 diff 是否为二进制（二进制文件外部工具无法按文本对比，禁用按钮；
// 「无差异」空文本不禁用——留按钮可点，由后端正常处理）
const isBinaryFile = ref(false)

// 按钮可用性：工具已配置且当前文件非二进制（range 段级二进制由组内按钮单独判断）
const externalDiffAvailable = computed(() => settingsStore.diffToolConfigured && !isBinaryFile.value)

// tooltip：未配置引导去设置；range 模式引导用文件组行内按钮；已配置说明按钮用途
const externalDiffTooltip = computed(() => {
  if (!settingsStore.diffToolConfigured) return '未配置外部 diff 工具，请在设置中配置'
  if (props.mode === 'range') return '区间差异请使用各文件行内的「用外部工具打开」按钮'
  return '用外部 diff 工具打开左右版本文件'
})

/**
 * 用外部 diff 工具打开当前 diff 的左右版本。
 * range 模式传入文件组路径逐文件打开；单文件模式用 props.file。
 * 未配置时按钮已置灰不会触发；后端启动失败经 AppError 结构化返回，
 * handleError 按 code 分流（NotConfigured=warning 引导设置，其余=error）。
 */
const openExternalDiff = async (fileOverride) => {
  const file = fileOverride || props.file
  if (!file || openingExternal.value) return
  openingExternal.value = true
  try {
    await OpenInExternalDiff(props.repoPath, props.mode, file, props.sha, props.baseSha, props.headSha)
  } catch (e) {
    handleError('打开外部 diff 失败: ', e)
  } finally {
    openingExternal.value = false
  }
}

const dialogTitle = computed(() => {
  if (props.mode === 'range') {
    return `区间差异 (${props.baseSha.slice(0, 8)} → ${props.headSha.slice(0, 8)})`
  }
  const name = props.file ? props.file.split(/[\\/]/).pop() : ''
  return name ? `文件差异 - ${name}` : '文件差异'
})

// ---------- diff 文件追溯（右键「查看文件历史 / 行归属」） ----------
// 单文件模式（workspace/commit）头部标题可右键；range 模式各文件分组名可右键。
// commit 文件路径是该提交之后的 pathAtCommit（GetCommitFileDiff 同口径），拼接
// 仓库根即为正确绝对路径；range 分组路径取 head 提交口径。
const traceableFile = computed(() => props.mode !== 'range' && !!props.file)

const traceMenuRef = ref(null)
// 右键上下文：菜单 command 发出时事件对象已不可得，open 时快照 file 供分流
const traceMenuCtx = ref({ file: '' })

// 追溯历史弹窗状态（自挂 FileHistoryDialog，repoPath=fileAbsPath 对齐 FileTreePanel 先例）
const traceHistoryVisible = ref(false)
const traceHistoryAbsPath = ref('')
const traceHistorySha = ref('')

// initialSha 按 diff 来源模式取定位提交：commit=所在提交；range=区间终点（head）；
// workspace 无单提交语义不定位
const traceShaForMode = () => {
  if (props.mode === 'commit') return props.sha
  if (props.mode === 'range') return props.headSha
  return ''
}

const openTraceMenu = (event, file) => {
  traceMenuCtx.value = { file }
  traceMenuRef.value?.open(event, { blameAvailable: blameCapableExt(file) })
}

const openTraceHistory = (file) => {
  traceHistorySha.value = traceShaForMode()
  traceHistoryAbsPath.value = joinRepoPath(props.repoPath, file)
  traceHistoryVisible.value = true
}

const onTraceCommand = (command) => {
  const { file } = traceMenuCtx.value
  if (command === 'history') {
    openTraceHistory(file)
    return
  }
  if (command === 'blame') {
    // 降级闭包复用同一弹窗：带 initialSha 定位（commit/range 口径），追溯不断链
    requestTraceBlame(joinRepoPath(props.repoPath, file), file, () => openTraceHistory(file))
  }
}

const onHeaderContextMenu = (event) => {
  if (!traceableFile.value || !props.file) return
  openTraceMenu(event, props.file)
}

// 双栏标题按 diff 来源模式区分：
//   workspace：旧=HEAD/工作区前，新=工作区
//   commit：旧=父提交，新=该提交（root commit 时旧=空树，呈现全增）
//   range：旧=base 提交，新=head 提交
const leftHeader = computed(() => {
  if (props.mode === 'commit') return '父提交版本'
  if (props.mode === 'range') return `base (${props.baseSha.slice(0, 8)})`
  return '旧版本（HEAD / 工作区前）'
})

const rightHeader = computed(() => {
  if (props.mode === 'commit') return `该提交 (${props.sha.slice(0, 8)})`
  if (props.mode === 'range') return `head (${props.headSha.slice(0, 8)})`
  return '新版本（工作区）'
})

const lineClass = (line) => {
  if (line.kind === 'add') return 'diff-line-add'
  if (line.kind === 'del') return 'diff-line-del'
  if (line.kind === 'hunk') return 'diff-line-hunk'
  return 'diff-line-context'
}

// 加载序号：快速切换文件时旧请求后到会覆盖新状态（含 isBinaryFile/left/right），
// 过期响应整体丢弃
let loadSeq = 0

const loadDiff = async () => {
  if (!props.repoPath) return
  const seq = ++loadSeq
  loading.value = true
  error.value = ''
  binaryHint.value = ''
  isBinaryFile.value = false
  left.value = []
  right.value = []
  fileGroups.value = []
  try {
    let text
    if (props.mode === 'commit') {
      if (!props.sha || !props.file) return
      text = await GetCommitFileDiff(props.repoPath, props.sha, props.file)
    } else if (props.mode === 'range') {
      if (!props.baseSha || !props.headSha) return
      text = await GetRangeDiff(props.repoPath, props.baseSha, props.headSha)
    } else {
      // workspace：工作区单文件
      if (!props.file) return
      text = await GetFileDiff(props.repoPath, props.file)
    }
    if (seq !== loadSeq) return
    if (!text || !text.trim()) {
      // 无 diff 文本：可能是二进制或无差异
      binaryHint.value = '无差异，或该文件类型不支持文本 diff 展示（二进制 / 图片）'
      return
    }
    // 单文件二进制兜底（range 模式按段各自判二进制，不走此分支）
    if (props.mode !== 'range' && /^Binary files /m.test(text)) {
      isBinaryFile.value = true
      binaryHint.value = '该文件为二进制文件，不支持文本 diff 展示'
      return
    }
    if (props.mode === 'range') {
      fileGroups.value = parseRangeDiff(text)
      if (fileGroups.value.length === 0) {
        binaryHint.value = '无差异'
      }
      return
    }
    const { leftLines, rightLines } = parseDiff(text)
    left.value = leftLines
    right.value = rightLines
  } catch (e) {
    if (seq !== loadSeq) return
    error.value = '加载差异失败: ' + (e?.message || String(e))
    ElMessage.error(error.value)
  } finally {
    if (seq === loadSeq) {
      loading.value = false
    }
  }
}

watch(
  () => [props.modelValue, props.file, props.sha, props.baseSha, props.headSha, props.mode],
  ([visible]) => {
    if (visible) {
      loadDiff()
      return
    }
    // 关闭立即做两件事（不等动画）：loadSeq++ 使在途请求过期（防渐隐期间旧响应
    // 写回行数据重新驻留/闪现），loading 复位（防秒开新文件时残留加载态）。
    // 行数据清空不在此处——destroy-on-close 渐隐期间 slot 未销毁，立即清空会让
    // 大 diff 关闭动画期间闪空态，释放点挂 @closed（perf-baseline §16）
    loadSeq++
    loading.value = false
  }
)

// 对话框关闭动画结束（slot 已销毁）后释放行数据：大 diff 双栏行对象属「不再使用
// 仍驻留」（实测 4000 行 diff 关闭后行数组驻留 ~2.3MB，perf-baseline §16），
// 此刻组件内数组引用是唯一残留，清空即完成释放
function releaseDiffData() {
  left.value = []
  right.value = []
  fileGroups.value = []
  error.value = ''
  binaryHint.value = ''
  isBinaryFile.value = false
}
</script>

<style scoped>
.diff-dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-right: 24px;
}

.diff-dialog-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  word-break: break-all;
}

.range-file-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.range-file-name {
  flex: 1;
  min-width: 0;
}

.diff-body {
  min-height: 240px;
  max-height: 65vh;
  overflow: auto;
}

.diff-empty {
  padding: 32px;
  text-align: center;
  color: var(--text-tertiary);
  font-size: 14px;
}

/* range 模式：多文件分组 */
.range-groups {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md, 16px);
}
.range-file-group {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  overflow: hidden;
}
.range-file-header {
  padding: 6px 10px;
  background: var(--bg-tertiary);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary);
  border-bottom: 1px solid var(--border-color);
  word-break: break-all;
}
.range-file-header .range-file-name {
  word-break: break-all;
}
.range-binary {
  padding: 16px;
}

.diff-table {
  display: flex;
  gap: 8px;
  align-items: stretch;
}

.diff-col {
  flex: 1 1 50%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.diff-col-header {
  padding: 6px 10px;
  background: var(--bg-tertiary);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-color);
}

.diff-col-body {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12.5px;
  background: var(--bg-secondary);
  overflow-x: auto;
}

.diff-line {
  display: flex;
  white-space: pre;
  line-height: 1.55;
}

.diff-line-no {
  display: inline-block;
  min-width: 42px;
  padding: 0 8px;
  text-align: right;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-right: 1px solid var(--border-color);
  user-select: none;
  flex-shrink: 0;
}

.diff-line-text {
  padding: 0 8px;
  flex: 1;
  white-space: pre;
}

.diff-line-context .diff-line-text {
  color: var(--text-primary);
}

.diff-line-add {
  background: rgba(103, 194, 58, 0.15);
}
.diff-line-add .diff-line-text {
  color: #67c23a;
}

.diff-line-del {
  background: rgba(245, 108, 108, 0.15);
}
.diff-line-del .diff-line-text {
  color: #f56c6c;
}

.diff-line-empty {
  background: var(--bg-tertiary);
}

.diff-line-hunk {
  background: var(--primary-bg);
  color: var(--primary-color);
  font-weight: 600;
}
.diff-line-hunk .diff-line-text {
  color: var(--primary-color);
}
</style>
