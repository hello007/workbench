<template>
  <el-dialog
    :model-value="modelValue"
    :title="dialogTitle"
    width="720px"
    destroy-on-close
    @update:model-value="onVisibleChange"
    @closed="releaseState"
  >
    <div v-loading="loading" class="file-history-body">
      <div v-if="commits.length > 0" class="history-list">
        <div
          v-for="commit in commits"
          :key="commit.sha"
          class="history-row"
          :class="{ 'is-located': commit.sha === locatedSha }"
          data-action="file-history-row"
          :data-sha="commit.sha"
          @click="openDiff(commit)"
        >
          <div class="history-main">
            <div class="history-message" :title="commit.message">{{ firstLine(commit.message) }}</div>
            <div class="history-meta">
              <el-text size="small" class="history-sha">{{ commit.shortSha }}</el-text>
              <el-text size="small" type="info">{{ commit.author }}</el-text>
              <el-text size="small" type="info">{{ commit.dateTime }}</el-text>
              <!-- 重命名检出标记：tooltip 展示 old → new（--follow 历史中该提交改了文件名） -->
              <el-tooltip
                v-if="commit.renameFrom"
                :content="`重命名：${commit.renameFrom} → ${commit.renameTo}`"
                placement="top"
              >
                <el-tag size="small" type="warning" class="history-rename-tag">R</el-tag>
              </el-tooltip>
            </div>
          </div>
          <el-icon class="history-arrow"><ArrowRight /></el-icon>
        </div>

        <div v-if="hasMore" class="history-more">
          <el-button
            size="small"
            text
            type="primary"
            data-action="file-history-load-more"
            :loading="loadingMore"
            @click="loadHistory(false)"
          >加载更多</el-button>
        </div>
      </div>

      <el-empty
        v-else-if="!loading"
        description="该文件暂无提交历史"
        :image-size="64"
      />
    </div>
  </el-dialog>

  <!-- 单提交单文件 diff：file 用该提交时刻路径（pathAtCommit），--follow 旧段才能取到变更。
       v-if 外控：FileDiffDialog 内也自挂 FileHistoryDialog（其 diff 文件右键追溯），
       二者互相引用成环——静态渲染下（如测试 stub 恒渲染 slot）会无限递归，关闭即卸载切断。 -->
  <FileDiffDialog
    v-if="diffVisible"
    v-model="diffVisible"
    mode="commit"
    :repo-path="repoPath"
    :sha="diffSha"
    :file="diffFile"
  />
</template>

<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { ArrowRight } from '@element-plus/icons-vue'
import { GetFileCommitHistory } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import FileDiffDialog from './FileDiffDialog.vue'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  // 工作目录绝对路径（定位仓库根，供内嵌 diff 弹窗复用）
  repoPath: { type: String, required: true },
  // 目标文件绝对路径（后端定位仓库根并换算仓库相对路径）
  fileAbsPath: { type: String, default: '' },
  // 打开后需定位并高亮的提交 SHA（行归属跳转入口，空串不定位）
  initialSha: { type: String, default: '' }
})

const PAGE_SIZE = 20
// 定位扫描条数上限：目标提交在分页历史深处时逐页追加至多到此为止（25 页）
const locateMaxCommits = 500

const emits = defineEmits(['update:modelValue'])

const commits = ref([])
const loading = ref(false)
const loadingMore = ref(false)
const hasMore = ref(false)
// 定位命中并高亮的提交 SHA（仅展示态，不参与 diff 打开逻辑）
const locatedSha = ref('')

// 内嵌单提交 diff 弹窗状态
const diffVisible = ref(false)
const diffSha = ref('')
const diffFile = ref('')

const dialogTitle = computed(() => {
  const name = props.fileAbsPath ? props.fileAbsPath.split(/[\\/]/).pop() : ''
  return name ? `文件历史 — ${name}` : '文件历史'
})

const firstLine = (message) => (message || '').split('\n')[0]

// 在途响应代次守卫（perf-baseline.md §16.5 第二时机）：关闭即时失效在途响应，
// 防渐隐期 loadMore 响应写回已清空的列表 / 干扰重开弹窗的加载态
let loadSeq = 0

const loadHistory = async (reset = true) => {
  if (!props.fileAbsPath) return
  const seq = ++loadSeq
  if (reset) {
    loading.value = true
  } else {
    loadingMore.value = true
  }

  try {
    const offset = reset ? 0 : commits.value.length
    const page = await GetFileCommitHistory(props.fileAbsPath, PAGE_SIZE, offset)
    if (seq !== loadSeq) return // 关闭/重开后已过期，丢弃写回
    if (reset) {
      commits.value = page || []
    } else {
      commits.value.push(...(page || []))
    }
    // 与仓库级 CommitHistory 同语义：返回满页即视为还有下一页
    hasMore.value = !!page && page.length === PAGE_SIZE
  } catch (error) {
    if (seq !== loadSeq) return // 过期响应的失败不再提示
    handleError('加载文件历史失败', error)
  } finally {
    if (seq === loadSeq) {
      loading.value = false
      loadingMore.value = false
    }
  }
}

// 弹窗打开即拉取首页（watch 驱动，父组件仅切 visible 不触碰数据面）；
// immediate 覆盖「以 visible=true 初始挂载」的形态（重挂载残留开启态也正常拉取）；
// 关闭分支即时失效在途响应（@closed 渐隐结束才清数据，在途写回必须更早丢弃）；
// 打开且带 initialSha 时首页加载完成后执行定位扫描
watch(
  () => props.modelValue,
  async (visible) => {
    if (visible) {
      await loadHistory(true)
      await locateCommit()
    } else {
      loadSeq++
      locatedSha.value = ''
    }
  },
  { immediate: true }
)

// 定位扫描：initialSha 指定的提交（行归属跳转入口）不在已加载页时逐页追加，
// 上限 locateMaxCommits 条；命中滚动至居中并高亮，超限未命中 info 提示。
// 中止判定用 modelValue（loadHistory 内部自增 loadSeq，外部保存的代次号每轮
// 失效不能复用）；循环另设「页数不增长即停」兜底，防失败页 hasMore 恒真死循环。
const locateCommit = async () => {
  if (!props.initialSha || commits.value.length === 0) return
  let hit = commits.value.find((c) => c.sha === props.initialSha)
  let prevLen = commits.value.length
  while (!hit && hasMore.value && commits.value.length < locateMaxCommits) {
    await loadHistory(false)
    if (!props.modelValue) return
    if (commits.value.length === prevLen) break // 加载失败页数不增长，停止扫描
    prevLen = commits.value.length
    hit = commits.value.find((c) => c.sha === props.initialSha)
  }
  if (!props.modelValue) return
  if (hit) {
    locatedSha.value = hit.sha
    nextTick(() => {
      const el = document.querySelector(`.history-row[data-sha="${hit.sha}"]`)
      if (el) el.scrollIntoView({ block: 'center' })
    })
  } else {
    ElMessage.info(`该提交不在最近 ${commits.value.length} 条提交中`)
  }
}

const openDiff = (commit) => {
  diffSha.value = commit.sha
  diffFile.value = commit.pathAtCommit
  diffVisible.value = true
}

const onVisibleChange = (visible) => emits('update:modelValue', visible)

// 关闭收尾（@closed 渐隐结束后）：清列表与加载态，防上次文件的历史滞留到下次打开
const releaseState = () => {
  commits.value = []
  hasMore.value = false
  loading.value = false
  loadingMore.value = false
  diffVisible.value = false
  diffSha.value = ''
  diffFile.value = ''
  locatedSha.value = ''
}
</script>

<style scoped>
.file-history-body {
  min-height: 200px;
  max-height: 60vh;
  overflow-y: auto;
}

.history-list {
  display: flex;
  flex-direction: column;
}

.history-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 0.15s;
}

.history-row:hover {
  background-color: var(--el-fill-color-light);
}

/* 定位命中高亮（行归属跳转入口）：主色左侧条 + 淡主色底，与 hover 态可叠加 */
.history-row.is-located {
  background-color: var(--el-color-primary-light-9);
  box-shadow: inset 2px 0 0 var(--el-color-primary);
}

.history-main {
  flex: 1;
  min-width: 0;
}

.history-message {
  font-size: 13px;
  color: var(--el-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.history-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}

.history-sha {
  font-family: monospace;
  color: var(--el-color-primary);
}

.history-rename-tag {
  cursor: default;
}

.history-arrow {
  color: var(--el-text-color-placeholder);
  flex-shrink: 0;
}

.history-more {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}
</style>
