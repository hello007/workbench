<template>
  <div class="home">
    <el-container style="height: 100vh;">
      <!-- 顶部工具栏 -->
      <el-header style="background-color: #545c64; display: flex; align-items: center; padding: 0 20px;">
        <span style="color: white; font-size: 18px; font-weight: bold;">Git仓库管理工具</span>
        <el-divider direction="vertical" style="margin: 0 20px; border-color: #8c919a;" />
        <el-select
          v-model="selectedDirectoryId"
          placeholder="选择工作目录"
          style="width: 300px;"
          @change="onDirectoryChange"
        >
          <el-option
            v-for="dir in directories"
            :key="dir.id"
            :label="dir.name"
            :value="dir.id"
          />
        </el-select>
        <el-button
          type="primary"
          style="margin-left: 10px;"
          @click="showAddDirectoryDialog"
        >
          添加目录
        </el-button>
        <span
          v-if="selectedDirectoryPath"
          class="current-directory-path"
          :title="selectedDirectoryPath"
        >
          {{ selectedDirectoryPath }}
        </span>
      </el-header>

      <!-- 主体内容 -->
      <el-container class="main-content">
        <!-- 左侧文件树 -->
        <el-aside width="300px" class="file-tree-aside">
          <div class="tree-toolbar">
            <el-button-group>
              <el-button size="small" @click="collapseAll">全部收起</el-button>
            </el-button-group>
          </div>
          <el-tree
            v-if="selectedDirectoryId"
            :key="selectedDirectoryId"
            ref="fileTreeRef"
            :props="treeProps"
            node-key="path"
            lazy
            :load="loadTreeNode"
            @node-click="onNodeClick"
            @node-contextmenu="onNodeContextMenu"
            class="file-tree"
          >
            <template #default="{ node, data }">
              <span class="custom-tree-node">
                <el-icon
                  v-if="data.type === 'directory'"
                  :color="node.expanded ? '#409EFF' : '#909399'"
                  style="margin-right: 5px;"
                >
                  <component :is="node.expanded ? FolderOpened : Folder" />
                </el-icon>
                <el-icon v-else color="#606266" style="margin-right: 5px;">
                  <Document />
                </el-icon>
                <span :style="{
                  color: data.type === 'directory'
                    ? (node.expanded ? '#409EFF' : '#909399')
                    : '#606266'
                }">
                  {{ node.label }}
                </span>
                <el-icon v-if="data.isGitRepo" color="#67C23A" style="margin-left: 5px;">
                  <SuccessFilled />
                </el-icon>
              </span>
            </template>
          </el-tree>
          <el-empty v-else description="请先选择工作目录" :image-size="100" />
        </el-aside>

        <!-- 右侧操作面板 -->
        <el-main>
          <div v-if="selectedNode" style="padding: 20px;">
            <h2>{{ selectedNode.name }}</h2>
            <el-descriptions :column="2" border>
              <el-descriptions-item label="路径">{{ selectedNode.path }}</el-descriptions-item>
              <el-descriptions-item label="类型">{{ selectedNode.type === 'directory' ? '文件夹' : '文件' }}</el-descriptions-item>
            </el-descriptions>

            <!-- Git 拉取更新按钮 -->
            <div v-if="selectedNode.isGitRepo" style="margin-top: 10px;">
              <el-button type="primary" @click="pullRepo" :loading="gitLoading">
                拉取更新
              </el-button>
            </div>

            <el-divider />

            <!-- Git 信息签页 -->
            <el-tabs v-if="selectedNode.isGitRepo" v-model="activeGitTab">
              <el-tab-pane label="仓库信息" name="repo">
                <GitInfo
                  ref="gitInfoRef"
                  :repo-path="selectedNode.path"
                  :latest-commit="latestCommit"
                />
              </el-tab-pane>
              <el-tab-pane label="提交历史" name="commits" lazy>
                <CommitHistory
                  ref="commitHistoryRef"
                  :repo-path="selectedNode.path"
                  @latest-commit="onLatestCommit"
                />
              </el-tab-pane>
            </el-tabs>

            <div v-else-if="selectedNode.type === 'directory'" style="margin-top: 20px;">
              <h3>文件夹操作</h3>
              <el-button-group>
                <el-button @click="showCreateDirectoryDialog">新建文件夹</el-button>
                <el-button @click="showCreateFileDialog">新建文件</el-button>
                <el-button type="success" @click="showCloneDialog">克隆仓库</el-button>
              </el-button-group>
            </div>

            <div v-else-if="selectedNode.type === 'file'" style="margin-top: 20px;">
              <h3>文件操作</h3>
              <el-button-group>
                <el-button type="primary" @click="handleOpenWithDefaultApp">打开</el-button>
                <el-button @click="previewFile">预览</el-button>
                <el-button @click="showRenameDialog">重命名</el-button>
                <el-button type="danger" @click="deleteFile">删除</el-button>
              </el-button-group>

              <div v-if="filePreview.content" style="margin-top: 20px;">
                <h4>文件内容</h4>
                <el-input
                  v-model="filePreview.content"
                  type="textarea"
                  :rows="10"
                  readonly
                  style="font-family: monospace;"
                />
              </div>
            </div>
          </div>
          <el-empty v-else description="请从左侧选择文件或文件夹" />
        </el-main>
      </el-container>
    </el-container>

    <!-- 添加目录对话框 -->
    <el-dialog
      v-model="addDirectoryDialogVisible"
      title="添加工作目录"
      width="500px"
    >
      <el-form :model="newDirectory" label-width="100px">
        <el-form-item label="目录名称">
          <el-input v-model="newDirectory.name" placeholder="例如: 我的工作空间" />
        </el-form-item>
        <el-form-item label="目录路径">
          <el-input v-model="newDirectory.path" placeholder="例如: C:\workspace" />
        </el-form-item>
        <el-form-item label="设为默认">
          <el-switch v-model="newDirectory.isDefault" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addDirectoryDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="addDirectory">确定</el-button>
      </template>
    </el-dialog>

    <!-- 克隆仓库对话框 -->
    <el-dialog
      v-model="cloneDialogVisible"
      title="克隆仓库"
      width="500px"
    >
      <el-form label-width="100px">
        <el-form-item label="目标文件夹">
          <el-input :model-value="selectedNode?.path" disabled />
        </el-form-item>
        <el-form-item label="Git 地址">
          <el-input
            v-model="cloneUrl"
            placeholder="例如: https://github.com/user/repo.git"
            :disabled="cloneLoading"
            @keyup.enter="cloneRepo"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="cloneDialogVisible = false" :disabled="cloneLoading">取消</el-button>
        <el-button type="primary" @click="cloneRepo" :loading="cloneLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 新建文件夹/文件对话框 -->
    <el-dialog
      v-model="createDialogVisible"
      :title="createType === 'directory' ? '新建文件夹' : '新建文件'"
      width="420px"
    >
      <el-form label-width="80px">
        <el-form-item label="父文件夹">
          <el-input :model-value="selectedNode?.path" disabled />
        </el-form-item>
        <el-form-item :label="createType === 'directory' ? '文件夹名' : '文件名'">
          <el-input
            v-model="createName"
            :placeholder="createType === 'directory' ? '例如: src' : '例如: main.go'"
            :disabled="createLoading"
            @keyup.enter="handleCreate"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialogVisible = false" :disabled="createLoading">取消</el-button>
        <el-button type="primary" @click="handleCreate" :loading="createLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重命名对话框 -->
    <el-dialog
      v-model="renameDialogVisible"
      title="重命名"
      width="420px"
    >
      <el-form label-width="80px">
        <el-form-item label="当前名称">
          <el-input :model-value="selectedNode?.name" disabled />
        </el-form-item>
        <el-form-item label="新名称">
          <el-input
            ref="renameInputRef"
            v-model="renameName"
            placeholder="请输入新名称"
            :disabled="renameLoading"
            @keyup.enter="handleRename"
            @keydown.esc="renameDialogVisible = false"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="renameDialogVisible = false" :disabled="renameLoading">取消</el-button>
        <el-button type="primary" @click="handleRename" :loading="renameLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 更新仓库进度弹窗 -->
    <el-dialog
      v-model="pullDialogVisible"
      :title="pullCompleted ? '更新完成' : '更新仓库'"
      width="700px"
      :close-on-click-modal="false"
      :close-on-press-escape="!pullCompleted"
      :show-close="pullCompleted"
    >
      <div style="margin-bottom: 16px;">
        <el-progress
          :percentage="pullProgress.total > 0 ? Math.round(pullProgress.current / pullProgress.total * 100) : 0"
          :format="() => `${pullProgress.current} / ${pullProgress.total}`"
          :status="pullCompleted ? (pullSummary.failed > 0 ? 'warning' : 'success') : undefined"
        />
        <div v-if="pullCompleted" style="margin-top: 8px; color: #909399; font-size: 13px;">
          成功: {{ pullSummary.success }}，失败: {{ pullSummary.failed }}
        </div>
      </div>

      <el-table :data="pullResults" style="width: 100%" max-height="400" size="small">
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-icon v-if="row.success" color="#67C23A"><SuccessFilled /></el-icon>
            <el-icon v-else color="#F56C6C"><CircleCloseFilled /></el-icon>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="仓库名称" min-width="150" show-overflow-tooltip />
        <el-table-column prop="path" label="路径" min-width="250" show-overflow-tooltip />
        <el-table-column label="结果" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.success" style="color: #67C23A;">{{ row.output || '已是最新' }}</span>
            <span v-else style="color: #F56C6C;">{{ row.error }}</span>
          </template>
        </el-table-column>
      </el-table>

      <template #footer>
        <el-button
          type="primary"
          @click="pullDialogVisible = false"
          :disabled="!pullCompleted"
        >
          {{ pullCompleted ? '关闭' : '更新中...' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 自定义右键菜单 -->
    <ul
      v-if="contextMenu.visible"
      class="context-menu"
      :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
      @click.stop
    >
      <template v-if="contextMenu.data?.type === 'directory'">
        <li class="context-menu-item" @click="onMenuCommand('createFile')">
          <el-icon><DocumentAdd /></el-icon>新建文件
        </li>
        <li class="context-menu-item" @click="onMenuCommand('createDir')">
          <el-icon><FolderAdd /></el-icon>新建文件夹
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('rename')">
          <el-icon><Edit /></el-icon>重命名
        </li>
        <li class="context-menu-item" @click="onMenuCommand('delete')">
          <el-icon><Delete /></el-icon>删除
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('copyPath')">
          <el-icon><CopyDocument /></el-icon>复制路径
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
          <el-icon><Monitor /></el-icon>在资源管理器中打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
          <el-icon><EditPen /></el-icon>用 VSCode 打开
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('pullRepos')">
          <el-icon><Refresh /></el-icon>更新仓库
        </li>
      </template>
      <template v-else>
        <li class="context-menu-item" @click="onMenuCommand('rename')">
          <el-icon><Edit /></el-icon>重命名
        </li>
        <li class="context-menu-item" @click="onMenuCommand('delete')">
          <el-icon><Delete /></el-icon>删除
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('copyPath')">
          <el-icon><CopyDocument /></el-icon>复制路径
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyName')">
          <el-icon><CopyDocument /></el-icon>复制文件名
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
          <el-icon><Monitor /></el-icon>在资源管理器中打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
          <el-icon><EditPen /></el-icon>用 VSCode 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openWithDefaultApp')">
          <el-icon><Open /></el-icon>用默认程序打开
        </li>
      </template>
    </ul>
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Folder,
  FolderOpened,
  Document,
  SuccessFilled,
  CircleCloseFilled,
  FolderAdd,
  DocumentAdd,
  Edit,
  Delete,
  CopyDocument,
  Monitor,
  Refresh,
  EditPen,
  Open
} from '@element-plus/icons-vue'
import { debug } from '../utils/debug'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import GitInfo from '../components/GitInfo.vue'
import CommitHistory from '../components/CommitHistory.vue'
import {
  GetDirectories, AddDirectory,
  GetFileTree,
  CreateDirectory, CreateFile, RenameFile, DeleteFile, PreviewFile,
  PullRepo, CloneRepo,
  GetCommitHistory,
  OpenInExplorer,
  OpenInVSCode,
  OpenWithDefaultApp,
  ScanAndPullRepos
} from '../../wailsjs/go/main/App'

// 数据
const directories = ref([])
const selectedDirectoryId = ref('')
const fileTreeData = ref([])
const selectedNode = ref(null)
const fileTreeRef = ref()
const gitLoading = ref(false)

const addDirectoryDialogVisible = ref(false)

const selectedDirectoryPath = computed(() => {
  const dir = directories.value.find(d => d.id === selectedDirectoryId.value)
  return dir ? dir.path : ''
})
const newDirectory = ref({
  name: '',
  path: '',
  isDefault: false
})

const filePreview = ref({
  content: '',
  error: ''
})

const createDialogVisible = ref(false)
const createType = ref('directory')
const createName = ref('')
const createLoading = ref(false)

const renameDialogVisible = ref(false)
const renameName = ref('')
const renameLoading = ref(false)
const renameInputRef = ref()

const cloneDialogVisible = ref(false)
const cloneUrl = ref('')
const cloneLoading = ref(false)

const pullDialogVisible = ref(false)
const pullProgress = reactive({ current: 0, total: 0 })
const pullResults = ref([])
const pullCompleted = ref(false)
const pullSummary = reactive({ success: 0, failed: 0 })

const latestCommit = ref(null)
const activeGitTab = ref('repo')
const gitInfoRef = ref()
const commitHistoryRef = ref()

const treeProps = {
  label: 'name',
  children: 'children',
  isLeaf: 'isLeaf'
}

// 方法
const loadDirectories = async () => {
  const dirs = await GetDirectories()
  directories.value = dirs || []

  // 自动选择默认目录
  const defaultDir = dirs.find(d => d.isDefault)
  if (defaultDir) {
    selectedDirectoryId.value = defaultDir.id
  } else if (dirs.length > 0) {
    selectedDirectoryId.value = dirs[0].id
  }
}

const onDirectoryChange = async () => {
  debug.log('Directory changed to:', selectedDirectoryId.value)
  selectedNode.value = null
}

const refreshNode = (nodePath) => {
  if (!fileTreeRef.value || !nodePath) return

  const treeNode = fileTreeRef.value.store.nodesMap[nodePath]
  if (treeNode) {
    treeNode.loaded = false
    treeNode.loading = false
    treeNode.expand()
  }
}

const refreshSelectedNode = () => {
  refreshNode(selectedNode.value?.path)
}

const loadTreeNode = async (node, resolve) => {
  debug.log('loadTreeNode called, node:', node)
  debug.log('node.level:', node?.level)
  debug.log('node.data:', node?.data)

  let path
  // 判断是否为根节点（level === 0 或者 node.data 为空）
  if (!node || node.level === 0 || !node.data) {
    // 根节点加载 - 获取当前选中的目录路径
    const dir = directories.value.find(d => d.id === selectedDirectoryId.value)
    if (!dir) {
      debug.log('No directory found for ID:', selectedDirectoryId.value)
      resolve([])
      return
    }
    path = dir.path
    debug.log('Loading root node for path:', path)
  } else {
    // 子节点加载
    path = node.data.path
    debug.log('Loading child nodes for path:', path)
  }

  try {
    const nodes = await GetFileTree(path)
    debug.log('Got nodes for path', path, ':', nodes)

    // 确保每个节点都有正确的 isLeaf 属性
    const processedNodes = (nodes || []).map(n => ({
      ...n,
      isLeaf: n.type === 'file' || !n.hasChildren
    }))

    debug.log('Processed nodes:', processedNodes)
    resolve(processedNodes)
  } catch (error) {
    console.error('Error in loadTreeNode:', error)
    ElMessage.error('加载节点失败: ' + (error.message || error))
    resolve([])
  }
}

const onNodeClick = async (data) => {
  selectedNode.value = data

  // 清空文件预览（切换节点时）
  filePreview.value = {
    content: '',
    error: ''
  }

  if (!data.isGitRepo) {
    latestCommit.value = null
  } else {
    latestCommit.value = null
    try {
      const commits = await GetCommitHistory(data.path, 1, 0)
      if (commits && commits.length > 0) {
        latestCommit.value = commits[0]
      }
    } catch {
      // 静默失败，用户仍可通过切换签页查看提交信息
    }
  }
}

const onLatestCommit = (commit) => {
  latestCommit.value = commit
}

// ---- 右键菜单 ----

const contextMenu = reactive({
  visible: false,
  x: 0,
  y: 0,
  data: null
})

const onNodeContextMenu = (event, data) => {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.x = event.clientX
  contextMenu.y = event.clientY
  contextMenu.data = data
  contextMenu.visible = true
}

const closeContextMenu = () => {
  contextMenu.visible = false
}

const onMenuCommand = (command) => {
  const data = contextMenu.data
  closeContextMenu()
  if (!data) return

  switch (command) {
    case 'createFile':
      showCreateFileDialogAt(data)
      break
    case 'createDir':
      showCreateDirectoryDialogAt(data)
      break
    case 'rename':
      showRenameDialogAt(data)
      break
    case 'delete':
      handleDeleteAt(data)
      break
    case 'copyPath':
      copyToClipboard(data.path.replaceAll('\\', '/'), '路径')
      break
    case 'copyName':
      copyToClipboard(data.name, '文件名')
      break
    case 'openExplorer':
      handleOpenExplorer(data.path)
      break
    case 'openInVSCode':
      handleOpenInVSCode(data.path)
      break
    case 'openWithDefaultApp':
      handleOpenWithDefaultApp()
      break
    case 'pullRepos':
      handleBatchPull(data)
      break
  }
}

const onGlobalClick = () => {
  closeContextMenu()
}

const onGlobalContextMenu = () => {
  closeContextMenu()
}

const showCreateFileDialogAt = (data) => {
  selectedNode.value = data
  createType.value = 'file'
  createName.value = ''
  createDialogVisible.value = true
}

const showCreateDirectoryDialogAt = (data) => {
  selectedNode.value = data
  createType.value = 'directory'
  createName.value = ''
  createDialogVisible.value = true
}

const showRenameDialogAt = (data) => {
  selectedNode.value = data
  renameName.value = data.name
  renameDialogVisible.value = true
  setTimeout(() => {
    const input = renameInputRef.value?.input
    if (input) {
      input.focus()
      input.select()
    }
  }, 100)
}

const handleRename = async () => {
  if (!renameName.value.trim()) {
    ElMessage.warning('请输入名称')
    return
  }
  if (!selectedNode.value) return

  renameLoading.value = true
  try {
    const result = await RenameFile(selectedNode.value.path, renameName.value.trim())
    if (result) {
      ElMessage.success('重命名成功')
      renameDialogVisible.value = false
      const targetPath = selectedNode.value.path
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) {
        parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      }
      refreshNode(parentPath)
      selectedNode.value = null
    } else {
      ElMessage.error('重命名失败')
    }
  } catch (error) {
    ElMessage.error('重命名失败: ' + (error.message || String(error)))
  } finally {
    renameLoading.value = false
  }
}

const handleDeleteAt = async (data) => {
  selectedNode.value = data

  try {
    await ElMessageBox.confirm(
      `确定要删除 "${data.name}" 吗？此操作不可撤销。`,
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
  } catch {
    return
  }

  try {
    const targetPath = data.path
    const result = await DeleteFile(targetPath)
    if (result) {
      ElMessage.success('删除成功')
      selectedNode.value = null
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) {
        parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      }
      refreshNode(parentPath)
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || String(error)))
  }
}

const copyToClipboard = async (text, label) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.error('复制失败')
  }
}

const handleOpenExplorer = async (path) => {
  try {
    const result = await OpenInExplorer(path)
    if (!result) {
      ElMessage.error('打开资源管理器失败')
    }
  } catch (error) {
    ElMessage.error('打开资源管理器失败: ' + (error.message || String(error)))
  }
}

const handleOpenInVSCode = async (path) => {
  try {
    const result = await OpenInVSCode(path)
    if (!result) {
      ElMessage.error('打开 VSCode 失败，请确认已安装 VSCode 并将 code 命令加入 PATH')
    }
  } catch (error) {
    ElMessage.error('打开 VSCode 失败: ' + (error.message || String(error)))
  }
}

const handleOpenWithDefaultApp = async () => {
  if (!selectedNode.value || selectedNode.value.type !== 'file') return
  try {
    const result = await OpenWithDefaultApp(selectedNode.value.path)
    if (!result) {
      ElMessage.error('打开文件失败')
    }
  } catch (error) {
    ElMessage.error('打开文件失败: ' + (error.message || String(error)))
  }
}

const handleBatchPull = async (data) => {
  try {
    const summary = await ScanAndPullRepos(data.path)

    pullResults.value = []
    pullProgress.current = 0
    pullProgress.total = summary.total
    pullCompleted.value = false
    pullSummary.success = 0
    pullSummary.failed = 0
    pullDialogVisible.value = true
  } catch (error) {
    ElMessage.warning(error || '未找到任何 Git 仓库')
  }
}

const isWailsRuntime = () => !!window.runtime

const cleanupPullEvents = () => {
  if (!isWailsRuntime()) return
  EventsOff("pull-progress")
  EventsOff("pull-complete")
}

const setupPullEvents = () => {
  if (!isWailsRuntime()) return
  cleanupPullEvents()
  EventsOn("pull-progress", (result) => {
    pullResults.value = [...pullResults.value, result]
    pullProgress.current++
  })
  EventsOn("pull-complete", (summary) => {
    pullCompleted.value = true
    pullSummary.success = summary.success || 0
    pullSummary.failed = summary.failed || 0
  })
}

const collapseAll = () => {
  if (fileTreeRef.value) {
    const allNodes = fileTreeRef.value.store.nodesMap
    Object.keys(allNodes).forEach(key => {
      const node = allNodes[key]
      if (node.expanded) {
        node.expanded = false
      }
    })
    ElMessage.success('已全部收起')
  }
}

const showAddDirectoryDialog = () => {
  newDirectory.value = {
    name: '',
    path: '',
    isDefault: false
  }
  addDirectoryDialogVisible.value = true
}

const addDirectory = async () => {
  if (!newDirectory.value.name || !newDirectory.value.path) {
    ElMessage.error('请填写完整信息')
    return
  }

  const result = await AddDirectory(
    newDirectory.value.name,
    newDirectory.value.path,
    newDirectory.value.isDefault
  )

  if (result) {
    ElMessage.success('添加成功')
    addDirectoryDialogVisible.value = false
    await loadDirectories()
  } else {
    ElMessage.error('添加失败')
  }
}

const pullRepo = async () => {
  if (!selectedNode.value) return

  gitLoading.value = true
  try {
    const result = await PullRepo(selectedNode.value.path)
    ElMessage.success(result || '拉取完成')
    gitInfoRef.value?.handleRefresh()
    commitHistoryRef.value?.handleRefresh()
  } catch (error) {
    ElMessage.error('拉取失败: ' + (error.message || String(error)))
  } finally {
    gitLoading.value = false
  }
}

const showCreateDirectoryDialog = () => {
  createType.value = 'directory'
  createName.value = ''
  createDialogVisible.value = true
}

const showCreateFileDialog = () => {
  createType.value = 'file'
  createName.value = ''
  createDialogVisible.value = true
}

const handleCreate = async () => {
  if (!createName.value.trim()) {
    ElMessage.warning(createType.value === 'directory' ? '请输入文件夹名称' : '请输入文件名称')
    return
  }
  if (!selectedNode.value) return

  createLoading.value = true
  try {
    let result
    if (createType.value === 'directory') {
      result = await CreateDirectory(selectedNode.value.path, createName.value.trim())
    } else {
      result = await CreateFile(selectedNode.value.path, createName.value.trim(), '')
    }
    if (result) {
      ElMessage.success(createType.value === 'directory' ? '文件夹创建成功' : '文件创建成功')
      createDialogVisible.value = false
      refreshSelectedNode()
    } else {
      ElMessage.error('创建失败')
    }
  } catch (error) {
    ElMessage.error('创建失败: ' + (error.message || String(error)))
  } finally {
    createLoading.value = false
  }
}

const showRenameDialog = () => {
  if (!selectedNode.value) return
  showRenameDialogAt(selectedNode.value)
}

const showCloneDialog = () => {
  cloneUrl.value = ''
  cloneDialogVisible.value = true
}

const cloneRepo = async () => {
  if (!cloneUrl.value.trim()) {
    ElMessage.warning('请输入 Git 仓库地址')
    return
  }
  if (!selectedNode.value) return

  cloneLoading.value = true
  try {
    const result = await CloneRepo(cloneUrl.value.trim(), selectedNode.value.path)
    if (result.includes('成功')) {
      ElMessage.success(result)
      cloneDialogVisible.value = false
      refreshSelectedNode()
    } else {
      ElMessage.error(result)
    }
  } catch (error) {
    ElMessage.error('克隆失败: ' + (error.message || String(error)))
  } finally {
    cloneLoading.value = false
  }
}

const deleteFile = async () => {
  if (!selectedNode.value) return
  await handleDeleteAt(selectedNode.value)
}

const previewFile = async () => {
  if (!selectedNode.value) return

  const preview = await PreviewFile(selectedNode.value.path)
  filePreview.value = preview

  if (preview.error) {
    ElMessage.error('预览失败: ' + preview.error)
  } else if (preview.tooLarge) {
    ElMessage.warning('文件过大，无法预览')
  } else if (preview.isBinary) {
    ElMessage.warning('二进制文件，无法预览')
  }
}

// 生命周期
onMounted(async () => {
  await loadDirectories()
  debug.log('Directories loaded:', directories.value)
  debug.log('Selected directory ID:', selectedDirectoryId.value)
  document.addEventListener('click', onGlobalClick)
  document.addEventListener('contextmenu', onGlobalContextMenu)
  setupPullEvents()
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onGlobalClick)
  document.removeEventListener('contextmenu', onGlobalContextMenu)
  cleanupPullEvents()
})
</script>

<style scoped>
.home {
  font-family: 'Microsoft YaHei', Arial, sans-serif;
}
.current-directory-path {
  color: #e0e4ea;
  font-size: 14px;
  margin-left: 14px;
  padding: 4px 10px;
  background-color: rgba(255, 255, 255, 0.08);
  border-radius: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 500px;
}

.el-tree-node__content {
  transition: background-color 0.2s ease;
  border-radius: 4px;
  margin: 2px 0;
}
.el-tree-node__content:hover {
  background-color: #F5F7FA !important;
}
.is-current > .el-tree-node__content {
  background-color: #E6F7FF !important;
  font-weight: 500;
}
.custom-tree-node {
  flex: 1;
  display: flex;
  align-items: center;
  font-size: 14px;
  cursor: default;
  user-select: none;
}
.el-tree-node__children {
  transition: all 0.3s ease;
  overflow: hidden;
}
.el-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.el-header {
  padding: 0 20px;
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}

.main-content {
  flex: 1;
  min-height: 0;
}

.file-tree-aside {
  display: flex;
  flex-direction: column;
  height: 100%;
  border-right: 1px solid #e6e6e6;
  background-color: #f5f7fa;
  overflow: hidden;
}

.tree-toolbar {
  flex-shrink: 0;
  padding: 10px;
  border-bottom: 1px solid #ebeef5;
}

.file-tree {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  background: transparent;
}

.el-main {
  background-color: #fff;
  overflow-y: auto;
}

/* 右键菜单样式 */
.context-menu {
  position: fixed;
  z-index: 2000;
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  padding: 4px 0;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.12);
  min-width: 160px;
  margin: 0;
  list-style: none;
}

.context-menu-item {
  display: flex;
  align-items: center;
  padding: 5px 16px;
  font-size: 14px;
  color: #606266;
  cursor: pointer;
  white-space: nowrap;
}

.context-menu-item:hover {
  background-color: #ecf5ff;
  color: #409eff;
}

.context-menu-item .el-icon {
  margin-right: 6px;
}

.context-menu-divider {
  height: 1px;
  background-color: #e4e7ed;
  margin: 4px 0;
}
</style>
