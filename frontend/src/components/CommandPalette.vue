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
        placeholder="搜索文件、目录 (#切换工作目录 @收藏夹)"
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

      <!-- 加载和空状态 -->
      <div v-if="searchLoading" class="result-loading">
        <el-icon class="is-loading"><Loading /></el-icon>
        搜索中...
      </div>
      <div v-if="mode === 'general' && query && !searchLoading && fileResults.length === 0 && favoriteResults.length === 0" class="result-empty">
        未找到匹配项
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
  workDirs: Array
})

const emit = defineEmits(['update:modelValue', 'select-file', 'select-favorite', 'select-workdir'])

const searchInputRef = ref(null)
const { input, mode, query, selectedIndex, fileResults, searchLoading, searchFiles, resetSelection } = useCommandPalette()
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

function getFavIndex(index) {
  if (showRecent.value) return recentItems.value.length + index
  return index
}

function getFileIndex(index) {
  return getFavIndex(favoriteResults.value.length) + index
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
  opacity: 0;
  cursor: pointer;
  color: #909399;
  font-size: 14px;
  margin-left: auto;
  padding: 4px;
  border-radius: 4px;
  transition: all 0.2s;
}

.remove-fav-btn:hover {
  color: #f56c6c;
  background: rgba(245, 108, 108, 0.1);
}

.result-item:hover .remove-fav-btn {
  opacity: 1;
}
</style>
