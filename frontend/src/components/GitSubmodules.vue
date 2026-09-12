<template>
  <el-card class="git-submodules-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>Git 子模块</span>
        <div class="header-actions">
          <el-button size="small" type="primary" @click="openAddDialog">添加子模块</el-button>
          <el-button
            size="small"
            @click="initAll"
            :loading="initializing"
            :disabled="submodules.length === 0"
          >初始化</el-button>
          <el-button
            size="small"
            @click="openUpdateDialog('')"
            :disabled="submodules.length === 0"
          >更新</el-button>
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="loadSubmodules"
            :loading="loading"
          />
        </div>
      </div>
    </template>

    <div v-loading="loading" class="submodules-container">
      <div v-if="submodules.length > 0" class="submodule-list">
        <div v-for="sm in submodules" :key="sm.path" class="submodule-row">
          <div class="sm-main">
            <el-text class="sm-path" :title="sm.path">{{ sm.path }}</el-text>
            <el-text class="sm-sha" :title="sm.sha" @click="copyText(sm.sha)">{{ sm.shortSha }}</el-text>
            <el-tag size="small" :type="statusInfo(sm).type">{{ statusInfo(sm).label }}</el-tag>
            <el-tag v-if="sm.detached" size="small" type="warning">detached</el-tag>
            <el-text v-if="sm.describe" class="sm-describe" type="info" :title="sm.describe">{{ sm.describe }}</el-text>
          </div>
          <div v-if="sm.url" class="sm-url" :title="sm.url">{{ sm.url }}</div>
          <div class="sm-actions">
            <el-button
              v-if="sm.detached"
              size="small"
              text
              :disabled="!sm.branch"
              :title="sm.branch ? `切换到跟踪分支 ${sm.branch}` : '未配置跟踪分支'"
              :loading="checkingOutPath === sm.path"
              @click="checkoutBranch(sm)"
            >切换跟踪分支</el-button>
            <el-button
              size="small"
              text
              @click="openUpdateDialog(sm.path)"
            >更新</el-button>
            <el-button
              size="small"
              text
              type="danger"
              :loading="removingPath === sm.path"
              @click="removeSubmodule(sm)"
            >删除</el-button>
          </div>
        </div>
      </div>

      <el-empty v-else-if="!loading" description="暂无子模块" />
    </div>

    <!-- 添加子模块对话框：url / path / branch（branch 可空） -->
    <el-dialog
      v-model="addVisible"
      title="添加子模块"
      width="480px"
      :close-on-click-modal="false"
    >
      <el-form label-position="top">
        <el-form-item label="仓库地址">
          <el-input v-model="addForm.url" placeholder="https://example.com/repo.git" />
        </el-form-item>
        <el-form-item label="子模块路径">
          <el-input v-model="addForm.path" placeholder="如 libs/sub-repo" />
        </el-form-item>
        <el-form-item label="跟踪分支（可选，留空使用默认分支）">
          <el-input v-model="addForm.branch" placeholder="如 main" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">取消</el-button>
        <el-button type="primary" @click="submitAdd" :loading="adding">添加</el-button>
      </template>
    </el-dialog>

    <!-- 更新子模块对话框：mode + recursive + init；subPath 非空为单行更新 -->
    <el-dialog
      v-model="updateVisible"
      :title="updateForm.subPath ? '更新子模块' : '更新全部子模块'"
      width="480px"
      :close-on-click-modal="false"
    >
      <el-form label-position="top">
        <el-form-item v-if="updateForm.subPath" label="目标子模块">
          <el-input :model-value="updateForm.subPath" disabled />
        </el-form-item>
        <el-form-item label="更新模式">
          <el-select v-model="updateForm.mode" class="mode-select">
            <el-option label="检出（checkout，默认）" value="checkout" />
            <el-option label="合并（merge）" value="merge" />
            <el-option label="变基（rebase）" value="rebase" />
            <el-option label="远端最新（remote）" value="remote" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="updateForm.recursive">递归更新嵌套子模块（--recursive）</el-checkbox>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="updateForm.init">初始化未拉取的子模块（--init）</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="updateVisible = false">取消</el-button>
        <el-button type="primary" @click="submitUpdate" :loading="updating">更新</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import {
  GetSubmodules, UpdateSubmodules,
  AddSubmodule, RemoveSubmodule, CheckoutSubmoduleBranch
} from '../../wailsjs/go/main/App'
import { handleGitError } from '../utils/gitError'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const submodules = ref([])
const loading = ref(false)
// 全量初始化进行中（header 按钮 loading）
const initializing = ref(false)
// 更新弹窗提交进行中（弹窗 footer 按钮 loading）
const updating = ref(false)
// 行级删除进行中标记（按 path 匹配行 loading）
const removingPath = ref('')
// detached 行切换分支进行中标记（按 path 匹配行 loading）
const checkingOutPath = ref('')

// 添加子模块弹窗状态
const addVisible = ref(false)
const addForm = ref({ url: '', path: '', branch: '' })
const adding = ref(false)

// 更新子模块弹窗状态：mode 默认 checkout、recursive 默认 true、init 默认 true（走 update --init）
// subPath 非空表示单行更新，空串表示全量更新
const updateVisible = ref(false)
const updateForm = ref({ mode: 'checkout', recursive: true, init: true, subPath: '' })

// 子模块状态标签四色映射：
//   灰（info）=未初始化、绿（success）=干净、橙（warning）=已修改或 SHA 未同步、红（danger）=冲突。
// 优先级：未初始化 > 冲突 > 已修改 > SHA 未同步 > 干净，
// 冲突比 dirty 更严重需红色突出，未初始化是前置态单独灰色提示。
const statusInfo = (sm) => {
  if (!sm.initialized) return { type: 'info', label: '未初始化' }
  if (sm.conflict) return { type: 'danger', label: '冲突' }
  if (sm.dirty) return { type: 'warning', label: '已修改' }
  if (sm.shaMismatch) return { type: 'warning', label: '未同步' }
  return { type: 'success', label: '干净' }
}

const loadSubmodules = async () => {
  loading.value = true
  try {
    submodules.value = await GetSubmodules(props.repoPath) || []
  } catch (error) {
    ElMessage.error('加载子模块列表失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
  }
}

const openAddDialog = () => {
  addForm.value = { url: '', path: '', branch: '' }
  addVisible.value = true
}

const submitAdd = async () => {
  const url = addForm.value.url.trim()
  const subPath = addForm.value.path.trim()
  if (!url) {
    ElMessage.warning('仓库地址不能为空')
    return
  }
  if (!subPath) {
    ElMessage.warning('子模块路径不能为空')
    return
  }
  // branch 可空：留空时后端走 git submodule add 默认分支
  const branch = addForm.value.branch.trim()
  adding.value = true
  try {
    await AddSubmodule(props.repoPath, url, subPath, branch)
    ElMessage.success('子模块已添加')
    addVisible.value = false
    await loadSubmodules()
  } catch (error) {
    handleGitError('添加子模块失败: ', error)
  } finally {
    adding.value = false
  }
}

// 打开更新弹窗：subPath 非空为单行更新（行内按钮触发），空串为全量更新（header 按钮触发）
const openUpdateDialog = (subPath) => {
  updateForm.value = { mode: 'checkout', recursive: true, init: true, subPath: subPath || '' }
  updateVisible.value = true
}

const submitUpdate = async () => {
  const { mode, recursive, init, subPath } = updateForm.value
  updating.value = true
  try {
    const out = await UpdateSubmodules(props.repoPath, mode, recursive, init, subPath)
    ElMessage.success('子模块更新成功' + (out ? `\n${out}` : ''))
    updateVisible.value = false
    await loadSubmodules()
  } catch (error) {
    handleGitError('更新子模块失败: ', error)
  } finally {
    updating.value = false
  }
}

// 全量初始化：git submodule update --init --recursive，一步完成注册 + 检出未初始化子模块。
// 不走仅注册的 InitSubmodules（git submodule init）：仅注册不检出，submodule status 前导码仍为
// '-'（未初始化），用户点「初始化」后状态标签不变，体验上等同无反应。
// mode=checkout 钉 index SHA 不漂移，recursive 下探嵌套。抢锁由后端 service 处理。
const initAll = async () => {
  initializing.value = true
  try {
    const out = await UpdateSubmodules(props.repoPath, 'checkout', true, true, '')
    ElMessage.success('子模块初始化成功' + (out ? `\n${out}` : ''))
    await loadSubmodules()
  } catch (error) {
    handleGitError('初始化子模块失败: ', error)
  } finally {
    initializing.value = false
  }
}

// 删除子模块：deinit -f + git rm -f + 清理 .git/modules/<name>，二次确认防误删
const removeSubmodule = async (sm) => {
  try {
    await ElMessageBox.confirm(
      `确定删除子模块「${sm.path}」？将执行 deinit + git rm + 清理 .git/modules，不可撤销。`,
      '删除子模块',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
  } catch {
    return // 用户取消
  }
  removingPath.value = sm.path
  try {
    await RemoveSubmodule(props.repoPath, sm.path)
    ElMessage.success('子模块已删除')
    await loadSubmodules()
  } catch (error) {
    handleGitError('删除子模块失败: ', error)
  } finally {
    removingPath.value = ''
  }
}

// detached HEAD 子模块切换回跟踪分支：读 .gitmodules 配置的 branch，
// 调 git -C <path> checkout <branch>，避免用户在 detached 上开发丢提交。
// branch 为空（未配置跟踪分支）时按钮已禁用，此处兜底校验。
const checkoutBranch = async (sm) => {
  if (!sm.branch) {
    ElMessage.warning('该子模块未配置跟踪分支，无法切换')
    return
  }
  checkingOutPath.value = sm.path
  try {
    const out = await CheckoutSubmoduleBranch(props.repoPath, sm.path, sm.branch)
    ElMessage.success(`已切换到分支 ${sm.branch}` + (out ? `\n${out}` : ''))
    await loadSubmodules()
  } catch (error) {
    handleGitError('切换子模块分支失败: ', error)
  } finally {
    checkingOutPath.value = ''
  }
}

const copyText = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

watch(() => props.repoPath, () => {
  submodules.value = []
  loadSubmodules()
})

loadSubmodules()

defineExpose({ loadSubmodules })
</script>

<style scoped>
.git-submodules-card {
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
.submodules-container {
  min-height: 60px;
}
.submodule-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}
.submodule-row {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--border-light);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: all var(--transition-fast);
}
.submodule-row:hover {
  box-shadow: var(--shadow-md);
  border-left-color: var(--primary-color);
}
.sm-main {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}
.sm-path {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  font-weight: 600;
  color: var(--primary-color);
}
.sm-sha {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-secondary);
  cursor: pointer;
}
.sm-sha:hover {
  text-decoration: underline;
}
.sm-describe {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
}
.sm-url {
  margin-top: 4px;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-tertiary);
  word-break: break-all;
}
.sm-actions {
  margin-top: 6px;
  display: flex;
  gap: var(--spacing-xs);
  justify-content: flex-end;
}
.mode-select {
  width: 100%;
}
</style>
