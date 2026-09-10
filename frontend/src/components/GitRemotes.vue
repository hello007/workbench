<template>
  <el-card class="git-remotes-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>Git 远程仓库</span>
        <div class="header-actions">
          <el-button size="small" type="primary" @click="openAddDialog">新增远程</el-button>
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="loadAll"
            :loading="loading"
          />
        </div>
      </div>
    </template>

    <div v-loading="loading" class="remotes-container">
      <div v-if="remotes.length > 0" class="remote-list">
        <div v-for="remote in remotes" :key="remote.name" class="remote-row">
          <div class="remote-main">
            <el-text class="remote-name">{{ remote.name }}</el-text>
            <el-text class="remote-url" :title="remote.url">{{ remote.url }}</el-text>
          </div>
          <div class="remote-actions">
            <el-button
              size="small"
              text
              @click="fetchRemote(remote)"
              :loading="fetchingName === remote.name"
            >拉取</el-button>
            <el-button
              size="small"
              text
              type="danger"
              @click="removeRemote(remote)"
              :loading="removingName === remote.name"
            >删除</el-button>
          </div>
        </div>
      </div>

      <el-empty v-else-if="!loading" description="未配置远程仓库" />
    </div>

    <!-- fetch 全量 + prune 选项 -->
    <div class="fetch-all-bar">
      <el-button
        size="small"
        @click="fetchAll"
        :loading="fetchingAll"
        :disabled="remotes.length === 0"
      >拉取全部</el-button>
      <el-checkbox v-model="pruneOnFetch">清理远端已删分支（--prune）</el-checkbox>
    </div>

    <!-- 设跟踪分支：当前分支 + 选目标 remote -->
    <div class="upstream-bar">
      <el-text class="upstream-label">设跟踪分支（当前分支）</el-text>
      <el-select
        v-model="upstreamRemote"
        placeholder="选择远程"
        size="small"
        class="upstream-select"
        :disabled="!currentBranch || remotes.length === 0"
      >
        <el-option
          v-for="remote in remotes"
          :key="remote.name"
          :label="remote.name"
          :value="remote.name"
        />
      </el-select>
      <el-text v-if="currentBranch" class="upstream-branch" type="info">
        {{ currentBranch }}
      </el-text>
      <el-text v-else type="warning">分离头指针，无法设上游</el-text>
      <el-button
        size="small"
        type="primary"
        @click="setUpstream"
        :loading="settingUpstream"
        :disabled="!currentBranch || !upstreamRemote"
      >设置</el-button>
    </div>

    <!-- 新增远程对话框 -->
    <el-dialog
      v-model="addVisible"
      title="新增远程仓库"
      width="420px"
      :close-on-click-modal="false"
    >
      <el-form label-position="top">
        <el-form-item label="远程名称">
          <el-input v-model="addForm.name" placeholder="如 origin" />
        </el-form-item>
        <el-form-item label="远程地址">
          <el-input v-model="addForm.url" placeholder="https://example.com/repo.git" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" @click="submitAdd" :loading="adding">添加</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import {
  GetRemotes, AddRemote, RemoveRemote, FetchRepo, SetBranchUpstream,
  GetGitRemoteURL
} from '../../wailsjs/go/main/App'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const remotes = ref([])
const loading = ref(false)
const addVisible = ref(false)
const addForm = ref({ name: '', url: '' })
const adding = ref(false)
const removingName = ref('')
const fetchingName = ref('')
const fetchingAll = ref(false)
const pruneOnFetch = ref(false)
const currentBranch = ref('')
const upstreamRemote = ref('')
const settingUpstream = ref(false)

const loadRemotes = async () => {
  loading.value = true
  try {
    remotes.value = await GetRemotes(props.repoPath) || []
    // 默认选中第一个远程作为设上游目标
    if (!upstreamRemote.value && remotes.value.length > 0) {
      upstreamRemote.value = remotes.value[0].name
    }
  } catch (error) {
    ElMessage.error('加载远程列表失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
  }
}

const loadCurrentBranch = async () => {
  try {
    const info = await GetGitRemoteURL(props.repoPath)
    // isDetached=true 时 branch 为 detached 标记，前端禁用设上游
    currentBranch.value = info && !info.isDetached ? (info.branch || '') : ''
  } catch {
    currentBranch.value = ''
  }
}

const loadAll = async () => {
  await Promise.all([loadRemotes(), loadCurrentBranch()])
}

const openAddDialog = () => {
  addForm.value = { name: '', url: '' }
  addVisible.value = true
}

const submitAdd = async () => {
  const name = addForm.value.name.trim()
  const url = addForm.value.url.trim()
  if (!name) {
    ElMessage.warning('远程名称不能为空')
    return
  }
  if (!url) {
    ElMessage.warning('远程地址不能为空')
    return
  }
  adding.value = true
  try {
    await AddRemote(props.repoPath, name, url)
    ElMessage.success('远程仓库已添加')
    addVisible.value = false
    await loadRemotes()
  } catch (error) {
    ElMessage.error('添加远程失败: ' + (error.message || String(error)))
  } finally {
    adding.value = false
  }
}

const removeRemote = async (remote) => {
  try {
    await ElMessageBox.confirm(`确定删除远程「${remote.name}」？`, '删除远程', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch {
    return
  }
  removingName.value = remote.name
  try {
    await RemoveRemote(props.repoPath, remote.name)
    ElMessage.success('远程仓库已删除')
    if (upstreamRemote.value === remote.name) {
      upstreamRemote.value = ''
    }
    await loadRemotes()
  } catch (error) {
    ElMessage.error('删除远程失败: ' + (error.message || String(error)))
  } finally {
    removingName.value = ''
  }
}

const fetchRemote = async (remote) => {
  fetchingName.value = remote.name
  try {
    const out = await FetchRepo(props.repoPath, remote.name, pruneOnFetch.value)
    ElMessage.success('拉取成功' + (out ? `\n${out}` : ''))
  } catch (error) {
    ElMessage.error('拉取失败: ' + (error.message || String(error)))
  } finally {
    fetchingName.value = ''
  }
}

const fetchAll = async () => {
  fetchingAll.value = true
  try {
    const out = await FetchRepo(props.repoPath, '', pruneOnFetch.value)
    ElMessage.success('拉取全部成功' + (out ? `\n${out}` : ''))
  } catch (error) {
    ElMessage.error('拉取全部失败: ' + (error.message || String(error)))
  } finally {
    fetchingAll.value = false
  }
}

const setUpstream = async () => {
  if (!currentBranch.value || !upstreamRemote.value) return
  settingUpstream.value = true
  try {
    await SetBranchUpstream(props.repoPath, currentBranch.value, upstreamRemote.value)
    ElMessage.success(`已为分支「${currentBranch.value}」设置上游：${upstreamRemote.value}/${currentBranch.value}`)
  } catch (error) {
    ElMessage.error('设置上游分支失败: ' + (error.message || String(error)))
  } finally {
    settingUpstream.value = false
  }
}

watch(() => props.repoPath, () => {
  remotes.value = []
  upstreamRemote.value = ''
  currentBranch.value = ''
  loadAll()
})

loadAll()

defineExpose({ loadAll })
</script>

<style scoped>
.git-remotes-card {
  border-radius: var(--radius-md);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 600;
  font-size: 16px;
  color: var(--text-primary);
}
.header-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
.remotes-container {
  min-height: 60px;
}
.remote-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}
.remote-row {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--border-light);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: all var(--transition-fast);
}
.remote-row:hover {
  box-shadow: var(--shadow-md);
  border-left-color: var(--primary-color);
}
.remote-main {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}
.remote-name {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  font-weight: 600;
  color: var(--primary-color);
}
.remote-url {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-secondary);
  word-break: break-all;
}
.remote-actions {
  margin-top: 6px;
  display: flex;
  gap: var(--spacing-xs);
  justify-content: flex-end;
}
.fetch-all-bar {
  margin-top: var(--spacing-md);
  padding-top: var(--spacing-sm);
  border-top: 1px solid var(--border-color);
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}
.upstream-bar {
  margin-top: var(--spacing-sm);
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}
.upstream-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}
.upstream-select {
  width: 140px;
}
.upstream-branch {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
}
</style>
