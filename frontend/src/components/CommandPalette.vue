<template>
  <el-dialog
    v-model="visible"
    :show-close="false"
    :close-on-click-modal="true"
    :close-on-press-escape="true"
    width="600px"
    top="15vh"
    class="command-palette-dialog"
    @close="onClose"
  >
    <template #header>
      <el-input
        ref="searchInputRef"
        v-model="input"
        placeholder="搜索文件、目录 (#工作目录 @收藏夹 :内容搜索)"
        size="large"
        clearable
        @keydown.down.prevent="moveDown"
        @keydown.up.prevent="moveUp"
        @keydown.enter.prevent="selectCurrent"
        @keydown.esc.prevent="onClose"
        @input="onInput"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
    </template>

    <div class="palette-content">
      <!-- 最近访问 -->
      <div v-if="showRecent" class="result-section">
        <div class="section-title">最近访问</div>
        <div
          v-for="(item, index) in recentItems"
          :key="'recent-' + index"
          class="result-item"
          :class="{ 'result-item--active': index === selectedIndex }"
          @click="selectItem(item)"
          @mouseenter="selectedIndex = index"
        >
          <el-icon class="result-icon">
            <component :is="item.type === 'file' ? Document : Folder" />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ getFileName(item.path) }}</div>
            <div class="result-path">{{ item.path }}</div>
          </div>
          <div class="result-time">{{ formatTime(item.lastAccess) }}</div>
        </div>
      </div>

      <!-- 收藏夹结果 -->
      <div v-if="favoriteResults.length > 0 && (mode === 'favorites' || (mode === 'general' && query))" class="result-section">
        <div class="section-title">收藏夹</div>
        <div
          v-for="(item, index) in favoriteResults"
          :key="'fav-' + index"
          class="result-item"
          :class="{ 'result-item--active': getFavIndex(index) === selectedIndex }"
          @click="selectFavorite(item)"
          @mouseenter="selectedIndex = getFavIndex(index)"
        >
          <el-icon class="result-icon" color="#f59e0b">
            <Star />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ item.alias || getFileName(item.path) }}</div>
            <div class="result-path">{{ item.path }}</div>
          </div>
          <el-icon class="remove-fav-btn" @click.stop="handleRemoveFav(item)">
            <Close />
          </el-icon>
        </div>
      </div>

      <!-- 工作目录切换 -->
      <div v-if="mode === 'workdir'" class="result-section">
        <div class="section-title">工作目录</div>
        <div
          v-for="(dir, index) in filteredWorkDirs"
          :key="'dir-' + dir.id"
          class="result-item"
          :class="{ 'result-item--active': index === selectedIndex }"
          @click="selectWorkDir(dir)"
          @mouseenter="selectedIndex = index"
        >
          <el-icon class="result-icon">
            <Folder />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ dir.name }}</div>
            <div class="result-path">{{ dir.path }}</div>
          </div>
        </div>
      </div>

      <!-- 文件搜索结果 -->
      <div v-if="mode === 'general' && query && fileResults.length > 0" class="result-section">
        <div class="section-title">搜索结果</div>
        <div
          v-for="(file, index) in fileResults"
          :key="'file-' + index"
          class="result-item"
          :class="{ 'result-item--active': getFileIndex(index) === selectedIndex }"
          @click="selectFile(file)"
          @mouseenter="selectedIndex = getFileIndex(index)"
        >
          <el-icon class="result-icon">
            <component :is="file.type === 'file' ? Document : Folder" />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ file.name }}</div>
            <div class="result-path">{{ file.path }}</div>
          </div>
        </div>
      </div>

      <!-- 内容搜索 - 全局搜索提示 -->
      <div v-if="(mode === 'content-global') && !contentSearching && contentGroups.length === 0 && !contentSearchExecuted && contentQuery.keyword" class="result-section">
        <div class="content-search-confirm">
          <el-icon><Search /></el-icon>
          <span>将在 {{ workDirs.length }} 个工作目录中搜索 "<strong>{{ contentQuery.keyword }}</strong>"</span>
          <span class="hint">按 Enter 确认搜索</span>
        </div>
      </div>

      <!-- 内容搜索 - 单目录提示 -->
      <div v-if="mode === 'content' && !contentSearching && contentGroups.length === 0 && !contentSearchExecuted && contentQuery.keyword" class="result-section">
        <div class="content-search-hint">
          <el-icon><Search /></el-icon>
          <span>搜索 "{{ contentQuery.keyword }}"</span>
          <span v-if="contentQuery.fileExt" class="hint-tag">{{ contentQuery.fileExt }}</span>
          <span v-if="contentQuery.subDir" class="hint-tag">{{ contentQuery.subDir }}/</span>
          <span class="hint">按 Enter 搜索</span>
        </div>
      </div>

      <!-- 内容搜索结果 -->
      <div v-if="contentGroups.length > 0" class="result-section">
        <div v-for="group in contentGroups" :key="group.repoName" class="content-group">
          <div class="section-title content-group-title">
            <el-icon><Folder /></el-icon>
            {{ group.repoName }}
          </div>
          <div
            v-for="(item, idx) in group.items"
            :key="group.repoName + '-' + idx"
            class="result-item"
            :class="{ 'result-item--active': getContentItemIndex(group, idx) === selectedIndex }"
            @click="openContentResultInVSCode(item); onClose()"
            @mouseenter="selectedIndex = getContentItemIndex(group, idx)"
          >
            <div class="result-info">
              <div class="result-name content-file-line">{{ item.filePath }}:<span class="line-num">{{ item.lineNum }}</span></div>
              <div class="result-line" v-html="highlightMatch(item.lineText, contentQuery.keyword)"></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 加载和空状态 -->
      <div v-if="searchLoading" class="result-loading">
        <el-icon class="is-loading"><Loading /></el-icon>
        搜索中...
      </div>
      <div v-if="contentSearching" class="result-loading">
        <el-icon class="is-loading"><Loading /></el-icon>
        搜索中...
      </div>
      <div v-if="mode === 'general' && query && !searchLoading && fileResults.length === 0 && favoriteResults.length === 0" class="result-empty">
        未找到匹配项
      </div>
      <div v-if="(mode === 'content' || mode === 'content-global') && !contentSearching && contentGroups.length === 0 && contentSearchExecuted && contentQuery.keyword" class="result-empty">
        未找到匹配内容
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import { Search, Document, Folder, Star, Loading, Close } from '@element-plus/icons-vue'
import { useCommandPalette } from '../composables/useCommandPalette'
import { useRecentAccess } from '../composables/useRecentAccess'
import { useFavorites } from '../composables/useFavorites'

const props = defineProps({
  modelValue: Boolean,
  currentDir: String,
  workDirs: Array,
  contentSearchInit: { type: String, default: '' }
})

const emit = defineEmits(['update:modelValue', 'select-file', 'select-favorite', 'select-workdir'])

const searchInputRef = ref(null)
const {
  input, mode, query, selectedIndex,
  fileResults, searchLoading,
  contentQuery, contentGroups, contentSearching, contentSearchExecuted,
  searchFiles, executeContentSearch, resetSelection
} = useCommandPalette()
const { getRecent } = useRecentAccess()
const { favorites, loadFavorites, searchFavorites, removeFavorite } = useFavorites()

const recentItems = ref([])
const favoriteResults = ref([])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const showRecent = computed(() => mode.value === 'general' && !query.value && recentItems.value.length > 0)

const filteredWorkDirs = computed(() => {
  if (!query.value) return props.workDirs || []
  const q = query.value.toLowerCase()
  return (props.workDirs || []).filter(d =>
    d.name.toLowerCase().includes(q) || d.path.toLowerCase().includes(q)
  )
})

// 获取内容搜索结果总数
const contentTotalItems = computed(() => {
  return contentGroups.value.reduce((sum, g) => sum + g.items.length, 0)
})

function getFavIndex(index) {
  if (showRecent.value) return recentItems.value.length + index
  return index
}

function getFileIndex(index) {
  return getFavIndex(favoriteResults.value.length) + index
}

// 根据全局 index 获取内容搜索结果项
function getContentItemByIndex(index) {
  let offset = 0
  for (const group of contentGroups.value) {
    if (index < offset + group.items.length) {
      return group.items[index - offset]
    }
    offset += group.items.length
  }
  return null
}

// 获取分组内项目的全局 index
function getContentItemIndex(group, localIdx) {
  let offset = 0
  for (const g of contentGroups.value) {
    if (g.repoPath === group.repoPath) {
      return offset + localIdx
    }
    offset += g.items.length
  }
  return offset + localIdx
}

// 用 VSCode 打开内容搜索结果（跳转到行号）
function openContentResultInVSCode(item) {
  const fullPath = item.repoPath + '\\' + item.filePath.replace(/\//g, '\\')
  window.open(`vscode://file/${fullPath}:${item.lineNum}`)
}

// 关键词高亮
function highlightMatch(text, keyword) {
  if (!keyword) return escapeHtml(text)
  const escaped = escapeHtml(text)
  const keywordEscaped = escapeHtml(keyword).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return escaped.replace(new RegExp(keywordEscaped, 'gi'), '<mark>$&</mark>')
}

function escapeHtml(str) {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

let searchTimer = null

function onInput() {
  resetSelection()
  clearTimeout(searchTimer)

  if (mode.value === 'favorites') {
    favoriteResults.value = searchFavorites(query.value)
  } else if (mode.value === 'general' && query.value) {
    favoriteResults.value = searchFavorites(query.value).slice(0, 5)
    searchTimer = setTimeout(() => {
      searchFiles(props.currentDir)
    }, 300)
  } else if (mode.value === 'content' || mode.value === 'content-global') {
    // 内容搜索不在输入时触发，仅清空上次结果
    favoriteResults.value = []
    fileResults.value = []
    contentSearchExecuted.value = false
  } else {
    favoriteResults.value = []
    fileResults.value = []
  }
}

function moveDown() {
  selectedIndex.value++
}

function moveUp() {
  if (selectedIndex.value > 0) selectedIndex.value--
}

function selectCurrent() {
  // 内容搜索模式：Enter 触发搜索或选择结果
  if (mode.value === 'content' || mode.value === 'content-global') {
    if (contentSearching.value) return
    if (contentGroups.value.length > 0) {
      const item = getContentItemByIndex(selectedIndex.value)
      if (item) {
        openContentResultInVSCode(item)
        onClose()
        return
      }
    }
    if (contentQuery.value.keyword) {
      executeContentSearch()
    }
    return
  }

  if (showRecent.value && selectedIndex.value < recentItems.value.length) {
    selectItem(recentItems.value[selectedIndex.value])
  } else if (mode.value === 'workdir') {
    const dir = filteredWorkDirs.value[selectedIndex.value]
    if (dir) selectWorkDir(dir)
  } else if (mode.value === 'favorites') {
    const fav = favoriteResults.value[selectedIndex.value]
    if (fav) selectFavorite(fav)
  } else {
    const favOffset = showRecent.value ? recentItems.value.length : 0
    const favIdx = selectedIndex.value - favOffset
    if (favIdx >= 0 && favIdx < favoriteResults.value.length) {
      selectFavorite(favoriteResults.value[favIdx])
    } else {
      const fileIdx = selectedIndex.value - favOffset - favoriteResults.value.length
      if (fileIdx >= 0 && fileIdx < fileResults.value.length) {
        selectFile(fileResults.value[fileIdx])
      }
    }
  }
}

function selectItem(item) {
  emit('select-file', item)
  onClose()
}

function selectFile(file) {
  emit('select-file', { path: file.path, type: file.type })
  onClose()
}

function selectFavorite(fav) {
  emit('select-favorite', fav)
  onClose()
}

async function handleRemoveFav(item) {
  await removeFavorite(item.path)
  favoriteResults.value = favoriteResults.value.filter(f => f.path !== item.path)
}

function selectWorkDir(dir) {
  emit('select-workdir', dir)
  onClose()
}

function onClose() {
  visible.value = false
  input.value = ''
  fileResults.value = []
  favoriteResults.value = []
  resetSelection()
}

function getFileName(path) {
  const parts = path.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1]
}

function formatTime(timestamp) {
  const now = Date.now()
  const diff = now - timestamp
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  return `${Math.floor(hours / 24)}天前`
}

watch(visible, async (val) => {
  if (val) {
    recentItems.value = getRecent(10)
    await loadFavorites()
    if (props.contentSearchInit) {
      input.value = props.contentSearchInit
    }
    await nextTick()
    searchInputRef.value?.focus()
  }
})
</script>

<style scoped>
.command-palette-dialog :deep(.el-dialog__header) {
  padding: 15px 20px 10px;
  margin-right: 0;
}

.command-palette-dialog :deep(.el-dialog__body) {
  padding: 0 0 15px 0;
  max-height: 400px;
  overflow-y: auto;
}

.palette-content {
  min-height: 60px;
}

.result-section {
  margin-bottom: 8px;
}

.section-title {
  font-size: 12px;
  color: #909399;
  padding: 8px 20px 5px;
  font-weight: 500;
}

.result-item {
  display: flex;
  align-items: center;
  padding: 10px 20px;
  cursor: pointer;
  transition: background 0.15s;
}

.result-item:hover,
.result-item--active {
  background: #f5f7fa;
}

.result-icon {
  font-size: 18px;
  margin-right: 12px;
  flex-shrink: 0;
}

.result-info {
  flex: 1;
  min-width: 0;
}

.result-name {
  font-size: 14px;
  color: #303133;
  margin-bottom: 2px;
}

.result-path {
  font-size: 12px;
  color: #909399;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.result-time {
  font-size: 12px;
  color: #c0c4cc;
  margin-left: 10px;
  flex-shrink: 0;
}

.result-loading {
  padding: 20px;
  text-align: center;
  color: #909399;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.result-empty {
  padding: 20px;
  text-align: center;
  color: #909399;
}

.remove-fav-btn {
  cursor: pointer;
  color: #f56c6c;
  font-size: 18px;
  margin-left: auto;
  padding: 6px;
  border-radius: 6px;
  background: rgba(245, 108, 108, 0.08);
  transition: all 0.2s;
}

.remove-fav-btn:hover {
  color: #f56c6c;
  background: rgba(245, 108, 108, 0.2);
}

/* 内容搜索确认提示 */
.content-search-confirm {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 20px;
  background: #fdf6ec;
  color: #e6a23c;
  font-size: 13px;
}

.content-search-confirm strong {
  color: #303133;
}

.content-search-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 20px;
  color: #909399;
  font-size: 13px;
}

.hint {
  color: #c0c4cc;
  font-size: 12px;
  margin-left: auto;
}

.hint-tag {
  background: #f0f2f5;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
  color: #606266;
}

/* 内容搜索分组 */
.content-group {
  margin-bottom: 4px;
}

.content-group-title {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #409eff !important;
  font-weight: 500;
}

/* 匹配行内容 */
.content-file-line {
  font-size: 12px !important;
  color: #606266 !important;
}

.line-num {
  color: #409eff;
  font-weight: 600;
}

.result-line {
  font-size: 12px;
  color: #303133;
  font-family: 'Consolas', 'Monaco', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background: #f5f7fa;
  padding: 3px 6px;
  border-radius: 3px;
  margin-top: 3px;
}

.result-line :deep(mark) {
  background: #fde68a;
  color: #92400e;
  padding: 0 1px;
  border-radius: 2px;
}
</style>
