<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="emit('update:visible', $event)"
    title="仓库筛选器"
    width="900px"
    :close-on-click-modal="false"
    append-to-body
    class="repo-filter-dialog"
  >
    <div class="repo-filter-body">
      <!-- 顶部工具栏：工作目录下拉 + 搜索 + 标签筛选 + 刷新 + 清理失效 -->
      <div class="repo-toolbar">
        <el-select
          v-model="selectedDirId"
          placeholder="选择工作目录"
          size="small"
          class="toolbar-dir"
          ref="dirSelectRef"
        >
          <el-option
            v-for="d in directories"
            :key="d.id"
            :label="d.name"
            :value="d.id"
          />
        </el-select>
        <el-input
          v-model="searchKeyword"
          placeholder="搜索仓库名 / 路径"
          size="small"
          clearable
          class="toolbar-search"
        />
        <el-select
          v-model="selectedTagFilter"
          multiple
          collapse-tags
          collapse-tags-tooltip
          placeholder="标签筛选（OR）"
          size="small"
          clearable
          class="toolbar-tag"
          ref="tagFilterRef"
        >
          <el-option
            v-for="t in allTags"
            :key="t"
            :label="t"
            :value="t"
          />
        </el-select>
        <el-button
          size="small"
          :loading="loading"
          @click="onRefresh"
        >
          刷新
        </el-button>
        <el-button
          size="small"
          @click="onCleanMissing"
        >
          清理失效
        </el-button>
      </div>

      <!-- Tab：已编辑 / 未编辑，标题带计数 -->
      <el-tabs v-model="activeTab" ref="repoTabsRef" class="repo-tabs">
        <el-tab-pane name="edited">
          <template #label>
            已编辑 <span class="tab-count">({{ editedCount }})</span>
          </template>
        </el-tab-pane>
        <el-tab-pane name="unedited">
          <template #label>
            未编辑 <span class="tab-count">({{ uneditedCount }})</span>
          </template>
        </el-tab-pane>
      </el-tabs>

      <!-- master-detail 两栏：左栏虚拟滚动，右栏独立 Pane 固定不失焦 -->
      <div class="repo-split-wrap">
        <Splitpanes class="default-theme" :push-other-panes="false">
          <!-- 左栏：虚拟滚动列表 -->
          <Pane :size="55" :min-size="35">
            <div class="repo-list-pane">
              <div v-if="loading && allRepos.length === 0" class="repo-list-hint">
                正在加载仓库列表...
              </div>
              <div v-else-if="filteredRepos.length === 0" class="repo-list-hint">
                {{ allRepos.length === 0 ? '该工作目录下暂无 Git 仓库' : '无匹配的仓库' }}
              </div>
              <!-- containerProps 自带 ref / onScroll / overflow 样式；容器需定高 -->
              <div v-bind="containerProps" class="repo-list">
                <div v-bind="wrapperProps">
                  <div
                    v-for="item in list"
                    :key="item.data.path"
                    class="repo-item"
                    :class="{
                      'is-selected': item.data.path === selectedPath,
                      'is-missing': item.data.missing
                    }"
                    @click="onSelect(item.data)"
                  >
                    <div class="repo-item__name" :title="item.data.name">
                      {{ item.data.name }}
                    </div>
                    <div class="repo-item__path" :title="item.data.path">
                      {{ item.data.path }}
                    </div>
                    <div class="repo-item__tags">
                      <el-tag
                        v-for="t in (item.data.tags || []).slice(0, 3)"
                        :key="t"
                        size="small"
                        type="info"
                      >
                        {{ t }}
                      </el-tag>
                      <el-tag
                        v-if="(item.data.tags || []).length > 3"
                        size="small"
                        type="info"
                      >
                        +{{ (item.data.tags || []).length - 3 }}
                      </el-tag>
                      <el-tag v-if="item.data.missing" size="small" type="danger">
                        失效
                      </el-tag>
                      <el-tag
                        v-else-if="item.data.isGitRepo && !item.data.hasRemote"
                        size="small"
                        type="warning"
                      >
                        无远程
                      </el-tag>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </Pane>

          <!-- 右栏：详情编辑区，独立 Pane，不随左栏滚动 -->
          <Pane :size="45" :min-size="30">
            <div class="repo-detail">
              <template v-if="selectedRepo">
                <div class="detail-header">
                  <div class="detail-name" :title="selectedRepo.name">
                    {{ selectedRepo.name }}
                  </div>
                  <div class="detail-path" :title="selectedRepo.path">
                    {{ selectedRepo.path }}
                  </div>
                </div>

                <div class="detail-section">
                  <div class="detail-label">README 摘要</div>
                  <div class="detail-readme">
                    {{ selectedRepo.readmeSummary || '暂无 README' }}
                  </div>
                </div>

                <div class="detail-section">
                  <div class="detail-label">自定义简述</div>
                  <el-input
                    :model-value="editingSummary"
                    type="textarea"
                    :rows="3"
                    placeholder="输入自定义简述，800ms 后自动保存"
                    @update:model-value="onSummaryInput"
                  />
                </div>

                <div class="detail-section">
                  <div class="detail-label">标签</div>
                  <div class="detail-tags">
                    <el-tag
                      v-for="t in editingTags"
                      :key="t"
                      closable
                      size="small"
                      @close="onRemoveTag(t)"
                    >
                      {{ t }}
                    </el-tag>
                    <span v-if="editingTags.length === 0" class="detail-tags-empty">
                      暂无标签
                    </span>
                  </div>
                  <el-input
                    v-model="newTagInput"
                    size="small"
                    placeholder="输入标签后回车添加"
                    @keyup.enter="onAddTag"
                  />
                </div>

                <div class="detail-actions">
                  <el-button
                    type="primary"
                    :disabled="selectedRepo.missing"
                    ref="jumpBtnRef"
                    @click="onJumpClick"
                  >
                    跳转到文件树
                  </el-button>
                  <span v-if="selectedRepo.missing" class="detail-missing-hint">
                    仓库路径已失效，无法跳转
                  </span>
                </div>
              </template>
              <el-empty
                v-else
                description="请从左侧选择一个仓库"
                :image-size="80"
              />
            </div>
          </Pane>
        </Splitpanes>
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useVirtualList, useTimeoutFn } from '@vueuse/core'
import { Splitpanes, Pane } from 'splitpanes'
import {
  GetRepoFilterList,
  RefreshRepoFilterList,
  SaveRepoMeta,
  CleanMissingRepoMeta
} from '../../wailsjs/go/main/App'

// ---- Props & Emits ----
const props = defineProps({
  visible: { type: Boolean, default: false },
  directories: { type: Array, default: () => [] },
  currentDirId: { type: String, default: '' }
})

const emit = defineEmits(['update:visible', 'locate'])

// ---- 常量 ----
// 等高项高度：必须与 .repo-item 的 height 严格一致，否则虚拟滚动定位偏移
const ITEM_HEIGHT = 84
const SAVE_DEBOUNCE_MS = 800

// ---- 状态 ----
const selectedDirId = ref('')
const allRepos = ref([]) // 后端返回的全部仓库
const loading = ref(false)
const activeTab = ref('edited') // 'edited' | 'unedited'
const searchKeyword = ref('')
const selectedTagFilter = ref([]) // 标签 OR 筛选
const selectedPath = ref('') // 选中态主键（数据驱动高亮）

// 右栏编辑态（与选中项同步，编辑后防抖/即时保存）
const editingSummary = ref('')
const editingTags = ref([])
const newTagInput = ref('')

// ---- computed ----
// Tab 分类计数：有标签为已编辑，无标签为未编辑
const editedCount = computed(() =>
  allRepos.value.filter(r => (r.tags?.length || 0) > 0).length
)
const uneditedCount = computed(() => allRepos.value.length - editedCount.value)

// 当前列表所有标签去重，供标签筛选器选项
const allTags = computed(() => {
  const set = new Set()
  allRepos.value.forEach(r => (r.tags || []).forEach(t => set.add(t)))
  return [...set]
})

// 左栏展示列表：Tab 分类 + 搜索 + 标签 OR 筛选
const filteredRepos = computed(() => {
  let list = allRepos.value
  if (activeTab.value === 'edited') {
    list = list.filter(r => (r.tags?.length || 0) > 0)
  } else {
    list = list.filter(r => (r.tags?.length || 0) === 0)
  }
  const kw = searchKeyword.value.trim().toLowerCase()
  if (kw) {
    list = list.filter(r =>
      (r.name || '').toLowerCase().includes(kw) ||
      (r.path || '').toLowerCase().includes(kw)
    )
  }
  if (selectedTagFilter.value.length > 0) {
    // OR 语义：任一标签命中即保留
    list = list.filter(r => {
      const tags = r.tags || []
      return selectedTagFilter.value.some(t => tags.includes(t))
    })
  }
  return list
})

// 选中项（从全量列表查找，避免被筛选/Tab 切换清空右栏）
const selectedRepo = computed(() =>
  allRepos.value.find(r => r.path === selectedPath.value) || null
)

// ---- 虚拟滚动 ----
const { list, containerProps, wrapperProps, scrollTo } = useVirtualList(
  filteredRepos,
  { itemHeight: ITEM_HEIGHT, overscan: 10 }
)

// ---- 防抖自动保存（F16）----
// useDebounceFn 在 @vueuse 12 未暴露 flush/cancel，改用 useTimeoutFn 手动管控：
// scheduleSave 记录最新参数并 start（重启计时器，符合 debounce 语义）；
// flushPendingSave 在切换选中项 / 关闭弹窗前显式触发立即保存。
let pendingSaveArgs = null
const { start: startSaveTimer, stop: stopSaveTimer, isPending: savePending } = useTimeoutFn(
  async () => {
    const args = pendingSaveArgs
    pendingSaveArgs = null
    if (args) await doSave(args)
  },
  SAVE_DEBOUNCE_MS,
  { immediate: false }
)

async function doSave({ path, summary, tags }) {
  try {
    await SaveRepoMeta(path, summary, tags)
    // 回写 allRepos，使 Tab 计数 / 标签筛选 / 左栏预览实时更新
    const repo = allRepos.value.find(r => r.path === path)
    if (repo) {
      repo.summary = summary
      repo.tags = tags
    }
  } catch (e) {
    ElMessage.error('保存失败: ' + (e.message || String(e)))
  }
}

function scheduleSave(path, summary, tags) {
  pendingSaveArgs = { path, summary, tags }
  startSaveTimer()
}

async function flushPendingSave() {
  if (!savePending.value || !pendingSaveArgs) return
  stopSaveTimer()
  const args = pendingSaveArgs
  pendingSaveArgs = null
  await doSave(args)
}

// ---- 编辑态同步 ----
function syncEditState(repo) {
  editingSummary.value = repo?.summary || ''
  editingTags.value = repo?.tags ? [...repo.tags] : []
  newTagInput.value = ''
}

// ---- 选中项 ----
async function onSelect(item) {
  if (item.path === selectedPath.value) return
  // 切换前 flush 保存旧选中项的编辑态（防抖窗口内的未提交修改）
  await flushPendingSave()
  selectedPath.value = item.path
  syncEditState(item)
  // 选中项滚回可视区（数据已就绪，nextTick 确保虚拟列表 slice 更新）
  await nextTick()
  const idx = filteredRepos.value.findIndex(r => r.path === item.path)
  if (idx >= 0) scrollTo(idx)
}

// ---- 简述编辑 ----
function onSummaryInput(val) {
  editingSummary.value = val
  if (selectedPath.value) {
    scheduleSave(selectedPath.value, editingSummary.value, editingTags.value)
  }
}

// ---- 标签编辑（增删即时保存）----
function onAddTag() {
  const t = newTagInput.value.trim()
  if (!t || !selectedPath.value) return
  if (editingTags.value.includes(t)) {
    newTagInput.value = ''
    return
  }
  // 增删走即时保存：先 flush 挂起的防抖，再用新标签数组立即保存
  flushPendingSave().then(() => {
    editingTags.value = [...editingTags.value, t]
    newTagInput.value = ''
    scheduleSave(selectedPath.value, editingSummary.value, editingTags.value)
    flushPendingSave()
  })
}

function onRemoveTag(tag) {
  if (!selectedPath.value) return
  flushPendingSave().then(() => {
    editingTags.value = editingTags.value.filter(t => t !== tag)
    scheduleSave(selectedPath.value, editingSummary.value, editingTags.value)
    flushPendingSave()
  })
}

// ---- 跳转 ----
function onJumpClick() {
  const repo = selectedRepo.value
  if (!repo) return
  if (repo.missing) {
    ElMessage.warning('该仓库路径已失效，无法跳转')
    return
  }
  // 仅 emit 路径，跨工作目录切换 / 等待树就绪 / 定位由 Home.vue 编排
  emit('locate', repo.path)
}

// ---- 加载列表 ----
async function loadList(useRefresh = false) {
  // 重载前显式 flush 挂起的防抖保存：切换工作目录/刷新/清理失效均会重置选中态或重拉列表，
  // 若不先 flush，旧的 pendingSaveArgs 会被后续编辑覆盖（scheduleSave 仅保留最新一份）导致丢保存。
  await flushPendingSave()
  if (!selectedDirId.value) {
    allRepos.value = []
    return
  }
  loading.value = true
  try {
    const list = useRefresh
      ? await RefreshRepoFilterList(selectedDirId.value)
      : await GetRepoFilterList(selectedDirId.value)
    allRepos.value = list || []
    // 选中第一项（刷新时若旧选中仍在列表则保持）
    const keepRepo = allRepos.value.find(r => r.path === selectedPath.value)
    if (keepRepo) {
      syncEditState(keepRepo)
      await nextTick()
      const idx = filteredRepos.value.findIndex(r => r.path === keepRepo.path)
      if (idx >= 0) scrollTo(idx)
    } else if (allRepos.value.length > 0) {
      selectedPath.value = allRepos.value[0].path
      syncEditState(allRepos.value[0])
    } else {
      selectedPath.value = ''
      syncEditState(null)
    }
  } catch (e) {
    ElMessage.error('加载仓库列表失败: ' + (e.message || String(e)))
    allRepos.value = []
  } finally {
    loading.value = false
  }
}

// ---- 刷新按钮 ----
async function onRefresh() {
  await loadList(true)
  ElMessage.success('已刷新仓库列表')
}

// ---- 清理失效 ----
async function onCleanMissing() {
  try {
    await ElMessageBox.confirm(
      '确定清理所有失效的仓库元数据记录吗？此操作不可撤销。',
      '提示',
      { confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    const n = await CleanMissingRepoMeta()
    ElMessage.success(`已清理 ${n} 条失效记录`)
    await loadList()
  } catch (e) {
    ElMessage.error('清理失败: ' + (e.message || String(e)))
  }
}

// ---- 弹窗打开 / 关闭 ----
let suppressDirWatch = false
watch(
  () => props.visible,
  async (v) => {
    if (v) {
      // 打开：同步当前工作目录 + 重置筛选
      suppressDirWatch = true
      selectedDirId.value = props.currentDirId
      await nextTick()
      suppressDirWatch = false
      searchKeyword.value = ''
      selectedTagFilter.value = []
      activeTab.value = 'edited'
      await loadList()
    } else {
      // 关闭：flush 未保存的编辑态
      await flushPendingSave()
    }
  },
  { immediate: true }
)

// ---- 工作目录切换：重新加载列表 ----
watch(selectedDirId, async (newVal, oldVal) => {
  if (suppressDirWatch) return
  if (!props.visible) return
  if (newVal === oldVal) return
  // 切换工作目录清空选中，避免跨目录残留
  selectedPath.value = ''
  syncEditState(null)
  await loadList()
})

// 组件卸载兜底：flush 未保存编辑
onBeforeUnmount(() => {
  flushPendingSave()
})
</script>

<style scoped>
/* 弹窗 body 收紧 padding，让工具栏 + splitpanes 撑满 */
.repo-filter-body {
  display: flex;
  flex-direction: column;
  height: 600px;
}

/* 顶部工具栏 */
.repo-toolbar {
  flex-shrink: 0;
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  padding-bottom: 8px;
}
.toolbar-dir {
  width: 200px;
}
.toolbar-search {
  width: 200px;
}
.toolbar-tag {
  width: 220px;
}

/* Tab 隐藏内容区，仅用头部切换（splitpanes 在下方独立） */
.repo-tabs {
  flex-shrink: 0;
}
.repo-tabs :deep(.el-tabs__content) {
  display: none;
}
.repo-tabs :deep(.el-tabs__header) {
  margin: 0 0 8px;
}
.tab-count {
  color: var(--text-secondary, #909399);
  font-size: 12px;
}

/* splitpanes 容器 */
.repo-split-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
}
.repo-split-wrap :deep(.splitpanes) {
  width: 100%;
  height: 100%;
}
.repo-split-wrap :deep(.splitpanes__pane) {
  display: flex;
  min-height: 0;
}

/* 左栏：虚拟滚动容器必须定高 */
.repo-list-pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  position: relative;
}
.repo-list {
  height: 100%;
  width: 100%;
}
.repo-list-hint {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-secondary, #909399);
  font-size: 13px;
  pointer-events: none;
}

/* 虚拟列表项：高度必须与 itemHeight 严格一致 */
.repo-item {
  height: 84px;
  box-sizing: border-box;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 4px;
  cursor: pointer;
  border-bottom: 1px solid var(--border-color, #ebeef5);
  overflow: hidden;
  transition: background-color 0.15s;
}
.repo-item:hover {
  background-color: var(--bg-tertiary, #f5f7fa);
}
.repo-item.is-selected {
  background-color: rgba(64, 158, 255, 0.1);
  border-left: 3px solid var(--primary-color, #409eff);
  padding-left: 9px;
}
.repo-item.is-missing {
  opacity: 0.5;
}
.repo-item__name {
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
  color: var(--text-primary, #303133);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.repo-item__path {
  font-size: 12px;
  line-height: 18px;
  color: var(--text-secondary, #909399);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.repo-item__tags {
  display: flex;
  gap: 4px;
  height: 22px;
  align-items: center;
  overflow: hidden;
}

/* 右栏：详情编辑区 */
.repo-detail {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 4px 8px 4px 12px;
  min-height: 0;
  overflow-y: auto;
}
.detail-header {
  margin-bottom: 12px;
}
.detail-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary, #303133);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.detail-path {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-secondary, #909399);
  word-break: break-all;
}
.detail-section {
  margin-bottom: 14px;
}
.detail-label {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-tertiary, #909399);
  margin-bottom: 6px;
}
.detail-readme {
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-secondary, #606266);
  background: var(--bg-tertiary, #f5f7fa);
  border-radius: 6px;
  padding: 10px 12px;
  max-height: 120px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-word;
}
.detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
  min-height: 24px;
  align-items: center;
}
.detail-tags-empty {
  font-size: 12px;
  color: var(--text-tertiary, #c0c4cc);
}
.detail-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 4px;
}
.detail-missing-hint {
  font-size: 12px;
  color: var(--el-color-danger, #f56c6c);
}
</style>

<style>
/* 弹窗 body padding 收紧（非 scoped，覆盖 element-plus 默认） */
.repo-filter-dialog .el-dialog__body {
  padding: 12px 20px 16px;
}
</style>
