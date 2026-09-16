<!-- frontend/src/views/DashboardView.vue -->
<template>
  <div class="dashboard-view">
    <div class="dashboard-header">
      <span class="dashboard-title">全局状态看板</span>
      <div class="header-actions">
        <el-tooltip content="ahead/behind 基于上次 fetch 的本地引用，非实时远程；点刷新重算" placement="bottom">
          <el-icon class="fresh-tip"><InfoFilled /></el-icon>
        </el-tooltip>
        <el-button :icon="Plus" size="small" @click="openAddDialog">添加仓库</el-button>
        <el-button :icon="Refresh" circle size="small" :loading="loading" @click="loadStatuses" />
      </div>
    </div>

    <div v-loading="loading" class="dashboard-body">
      <!-- 空状态 -->
      <el-empty v-if="!loading && statuses.length === 0" description="尚未 pin 任何仓库，点击「添加仓库」关注核心项目状态">
        <el-button type="primary" :icon="Plus" @click="openAddDialog">添加仓库</el-button>
      </el-empty>

      <!-- 状态表格 -->
      <el-table v-else :data="statuses" class="status-table" :row-class-name="rowClass" @row-click="onRowClick">
        <el-table-column label="仓库" min-width="160">
          <template #default="{ row }">
            <span class="repo-name" :class="{ 'repo-missing': row.missing }">{{ row.name }}</span>
            <span class="repo-path" :title="row.path">{{ row.path }}</span>
          </template>
        </el-table-column>
        <el-table-column label="分支" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.detached" size="small" type="danger">分离头指针</el-tag>
            <el-tag v-else-if="row.branch" size="small">{{ row.branch }}</el-tag>
            <span v-else class="dim">-</span>
          </template>
        </el-table-column>
        <el-table-column label="工作区" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.dirty" size="small" type="warning">有改动</el-tag>
            <el-tag v-else size="small" type="success">干净</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="未推送" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.hasUpstream && row.ahead > 0" size="small" type="danger">{{ row.ahead }}</el-tag>
            <span v-else-if="row.hasUpstream" class="dim">0</span>
            <span v-else class="dim" title="分支无跟踪上游">-</span>
          </template>
        </el-table-column>
        <el-table-column label="未拉取" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.hasUpstream && row.behind > 0" size="small" type="warning">{{ row.behind }}</el-tag>
            <span v-else-if="row.hasUpstream" class="dim">0</span>
            <span v-else class="dim" title="分支无跟踪上游">-</span>
          </template>
        </el-table-column>
        <el-table-column label="上游" width="90" align="center">
          <template #default="{ row }">
            <el-tag v-if="!row.hasUpstream && !row.detached && row.isRepo" size="small" type="info">无上游</el-tag>
            <span v-else class="dim">-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100" align="center">
          <template #default="{ row }">
            <el-button
              v-if="!row.missing"
              size="small"
              link
              type="primary"
              @click.stop="emit('locate', row.path)"
            >跳转</el-button>
            <el-button size="small" link type="danger" @click.stop="onRemovePin(row.path)">取消关注</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 状态异常提示行 -->
      <div v-if="errorRepos.length > 0" class="error-list">
        <el-alert type="warning" :closable="false" show-icon title="部分仓库状态计算异常">
          <div v-for="r in errorRepos" :key="r.path" class="error-item">
            <strong>{{ r.name }}</strong>：{{ r.error }}
          </div>
        </el-alert>
      </div>
    </div>

    <!-- 添加 pin 仓库弹窗：选工作目录 → 扫描多选 → 加入看板 -->
    <el-dialog v-model="addDialogVisible" title="添加关注仓库" width="640px" :close-on-click-modal="false">
      <div class="add-dialog">
        <el-select v-model="addDirId" placeholder="选择工作目录" size="default" @change="scanReposForAdd">
          <el-option v-for="d in directoryStore.directories" :key="d.id" :label="d.name" :value="d.id" />
        </el-select>
        <div v-loading="addScanning" class="add-list">
          <el-empty v-if="!addScanning && addCandidates.length === 0" description="选择工作目录后展示可添加仓库" />
          <el-checkbox-group v-else v-model="addSelectedPaths">
            <div v-for="item in addCandidates" :key="item.path" class="add-item">
              <el-checkbox :value="item.path" :disabled="isPinnedMap[item.path]">
                <span class="add-name">{{ item.name }}</span>
                <span class="add-path" :title="item.path">{{ item.path }}</span>
                <el-tag v-if="isPinnedMap[item.path]" size="small" type="info">已关注</el-tag>
                <el-tag v-else-if="!item.hasRemote" size="small" type="info">无远程</el-tag>
              </el-checkbox>
            </div>
          </el-checkbox-group>
        </div>
      </div>
      <template #footer>
        <el-button @click="addDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="adding" @click="confirmAdd">加入看板</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Refresh, InfoFilled } from '@element-plus/icons-vue'
import {
  GetDashboardStatuses,
  RemoveDashboardPin,
  IsDashboardPinned,
  GetRepoFilterList,
  AddDashboardPin
} from '../../wailsjs/go/main/App'
import { useDirectoryStore } from '../store'
import { handleError } from '../utils/error'

const emit = defineEmits(['locate'])

const directoryStore = useDirectoryStore()

const loading = ref(false)
const statuses = ref([])

// 状态计算异常的仓库（非致命 Error 字段非空）
const errorRepos = computed(() => statuses.value.filter(r => r.error))

// 行样式：失效仓库灰显
function rowClass({ row }) {
  return row.missing ? 'row-missing' : ''
}

// 加载全部 pin 仓状态
async function loadStatuses() {
  loading.value = true
  try {
    statuses.value = await GetDashboardStatuses()
  } catch (e) {
    handleError('加载看板状态失败：', e)
    statuses.value = []
  } finally {
    loading.value = false
  }
}

// 点击行跳转（失效仓库不可跳转）
function onRowClick(row) {
  if (row.missing) {
    ElMessage.warning('仓库路径已失效，无法跳转')
    return
  }
  emit('locate', row.path)
}

// 取消关注
async function onRemovePin(path) {
  try {
    await RemoveDashboardPin(path)
    ElMessage.success('已取消关注')
    await loadStatuses()
  } catch (e) {
    handleError('取消关注失败：', e)
  }
}

// ---- 添加 pin 弹窗 ----
const addDialogVisible = ref(false)
const addDirId = ref('')
const addCandidates = ref([])
const addSelectedPaths = ref([])
const addScanning = ref(false)
const adding = ref(false)
const isPinnedMap = ref({})

async function openAddDialog() {
  isPinnedMap.value = {}
  addDirId.value = directoryStore.selectedDirectoryId || ''
  addCandidates.value = []
  addSelectedPaths.value = []
  addDialogVisible.value = true
  if (addDirId.value) {
    scanReposForAdd(addDirId.value)
  }
}

// 扫描选中工作目录下的仓库，填充候选列表 + 标记已 pin
async function scanReposForAdd(dirId) {
  if (!dirId) {
    addCandidates.value = []
    return
  }
  addScanning.value = true
  try {
    const items = await GetRepoFilterList(dirId)
    addCandidates.value = items || []
    // 逐项查 pin 状态（pin 数量少，开销可控）
    const map = {}
    for (const item of addCandidates.value) {
      try {
        map[item.path] = await IsDashboardPinned(item.path)
      } catch {
        map[item.path] = false
      }
    }
    isPinnedMap.value = map
  } catch (e) {
    handleError('扫描仓库失败：', e)
    addCandidates.value = []
  } finally {
    addScanning.value = false
  }
}

async function confirmAdd() {
  if (addSelectedPaths.value.length === 0) {
    ElMessage.warning('请至少选择一个仓库')
    return
  }
  adding.value = true
  try {
    for (const p of addSelectedPaths.value) {
      await AddDashboardPin(p)
    }
    ElMessage.success(`已添加 ${addSelectedPaths.value.length} 个仓库`)
    addDialogVisible.value = false
    await loadStatuses()
  } catch (e) {
    handleError('添加关注失败：', e)
  } finally {
    adding.value = false
  }
}

onMounted(() => {
  loadStatuses()
})

defineExpose({ loadStatuses })
</script>

<style scoped>
.dashboard-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 12px 16px;
  overflow: hidden;
}

.dashboard-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
  flex-shrink: 0;
}

.dashboard-title {
  font-size: 16px;
  font-weight: 600;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.fresh-tip {
  color: var(--el-text-color-secondary);
  cursor: help;
}

.dashboard-body {
  flex: 1;
  overflow: auto;
}

.status-table {
  width: 100%;
}

.status-table :deep(.row-missing) {
  opacity: 0.5;
}

.repo-name {
  display: block;
  font-weight: 500;
}

.repo-name.repo-missing {
  text-decoration: line-through;
}

.repo-path {
  display: block;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.dim {
  color: var(--el-text-color-placeholder);
}

.error-list {
  margin-top: 12px;
}

.error-item {
  font-size: 13px;
  line-height: 1.6;
}

.add-dialog {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.add-list {
  min-height: 200px;
  max-height: 360px;
  overflow: auto;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 4px;
  padding: 8px;
}

.add-item {
  padding: 4px 0;
}

.add-name {
  font-weight: 500;
  margin-right: 8px;
}

.add-path {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
</style>
