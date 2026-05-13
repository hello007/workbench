<template>
  <div class="home">
    <el-container style="height: 100vh;">
      <el-aside width="200px" class="directory-aside">
        <DirectoryTree
          :directories="directories"
          :selected-id="selectedDirectoryId"
          @select="onDirectorySelect"
          @change="loadDirectories"
        />
      </el-aside>
      <el-aside width="280px" class="file-tree-aside">
        <FileTreePanel
          ref="fileTreePanelRef"
          :directories="directories"
          :selected-dir-id="selectedDirectoryId"
          :clipboard="clipboard"
          @select="onNodeSelect"
          @batch-pull="onBatchPull"
          @copy="handleCopy"
          @cut="handleCut"
          @paste="handlePaste"
        />
      </el-aside>
      <el-main class="content-main">
        <ContentPanel
          ref="contentPanelRef"
          :selected-node="selectedNode"
          :latest-commit="latestCommit"
          :clipboard="clipboard"
          @latest-commit="commit => latestCommit = commit"
          @refresh-node="onRefreshNode"
          @create-directory="node => fileTreePanelRef.showCreateAt(node, 'directory')"
          @create-file="node => fileTreePanelRef.showCreateAt(node, 'file')"
          @rename="onRenameFromContent"
          @delete="onDeleteFromContent"
          @copy="handleCopy"
          @cut="handleCut"
          @paste="handlePaste"
        />
      </el-main>
    </el-container>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { debug } from '../utils/debug'
import DirectoryTree from '../components/DirectoryTree.vue'
import FileTreePanel from '../components/FileTreePanel.vue'
import ContentPanel from '../components/ContentPanel.vue'
import {
  GetDirectories,
  ScanAndPullRepos,
  DeleteFile,
  CopyItem,
  MoveItem
} from '../../wailsjs/go/main/App'

// ---- 核心状态 ----
const directories = ref([])
const selectedDirectoryId = ref('')
const selectedNode = ref(null)
const latestCommit = ref(null)

const clipboard = reactive({
  mode: null,
  sourcePath: '',
  sourceName: '',
  sourceType: ''
})

// ---- 子组件 ref ----
const fileTreePanelRef = ref()
const contentPanelRef = ref()

// ---- 加载目录列表 ----
const loadDirectories = async () => {
  try {
    const dirs = await GetDirectories()
    directories.value = dirs || []

    // 自动选择默认目录
    const defaultDir = dirs.find(d => d.isDefault)
    if (defaultDir) {
      selectedDirectoryId.value = defaultDir.id
    } else if (dirs.length > 0) {
      selectedDirectoryId.value = dirs[0].id
    }
  } catch (error) {
    debug.log('加载目录失败:', error)
  }
}

// ---- 切换工作目录 ----
const onDirectorySelect = (dirId) => {
  selectedDirectoryId.value = dirId
  selectedNode.value = null
  latestCommit.value = null
  contentPanelRef.value?.clearPreview()
}

// ---- 选中文件树节点 ----
const onNodeSelect = (data) => {
  selectedNode.value = data
  contentPanelRef.value?.clearPreview()
}

// ---- 刷新文件树节点 ----
const onRefreshNode = (path) => {
  fileTreePanelRef.value?.refreshNode(path)
}

// ---- 批量拉取 ----
const onBatchPull = async (data) => {
  try {
    const summary = await ScanAndPullRepos(data.path)
    contentPanelRef.value?.startBatchPull(summary)
  } catch (error) {
    ElMessage.warning(error || '未找到任何 Git 仓库')
  }
}

// ---- ContentPanel 重命名 ----
const onRenameFromContent = (node) => {
  fileTreePanelRef.value?.showRenameAt(node)
}

// ---- ContentPanel 删除 ----
const onDeleteFromContent = async (node) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除 "${node.name}" 吗？此操作不可撤销。`,
      '警告',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch {
    return
  }

  try {
    const result = await DeleteFile(node.path)
    if (result) {
      ElMessage.success('删除成功')
      // 刷新父节点
      const lastSep = Math.max(node.path.lastIndexOf('\\'), node.path.lastIndexOf('/'))
      const parentPath = lastSep > 0 ? node.path.substring(0, lastSep) : ''
      if (parentPath) {
        fileTreePanelRef.value?.refreshNode(parentPath)
      }
      selectedNode.value = null
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || String(error)))
  }
}

// ---- 剪贴板操作 ----
const clearClipboard = () => {
  clipboard.mode = null
  clipboard.sourcePath = ''
  clipboard.sourceName = ''
  clipboard.sourceType = ''
}

const handleCopy = (data) => {
  clipboard.mode = 'copy'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 复制成功`)
}

const handleCut = (data) => {
  clipboard.mode = 'cut'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 剪切成功`)
}

const resolveTargetDir = (data) => {
  if (data.type === 'directory') {
    return data.path
  }
  const lastSep = Math.max(data.path.lastIndexOf('\\'), data.path.lastIndexOf('/'))
  return lastSep > 0 ? data.path.substring(0, lastSep) : ''
}

const handlePaste = async (targetData) => {
  if (!clipboard.mode || !clipboard.sourcePath) return

  const targetDir = resolveTargetDir(targetData)
  if (!targetDir) return

  try {
    let result
    if (clipboard.mode === 'copy') {
      result = await CopyItem(clipboard.sourcePath, targetDir)
    } else {
      result = await MoveItem(clipboard.sourcePath, targetDir)
    }

    if (result && !result.startsWith('错误')) {
      ElMessage.success(`粘贴成功：${result.replaceAll('\\', '/')}`)
      fileTreePanelRef.value?.refreshNode(targetDir)

      if (clipboard.mode === 'cut') {
        const srcLastSep = Math.max(clipboard.sourcePath.lastIndexOf('\\'), clipboard.sourcePath.lastIndexOf('/'))
        const srcParent = srcLastSep > 0 ? clipboard.sourcePath.substring(0, srcLastSep) : ''
        if (srcParent && srcParent !== targetDir) {
          fileTreePanelRef.value?.refreshNode(srcParent)
        }
        clearClipboard()
      }
    } else {
      ElMessage.error(result || '粘贴失败')
    }
  } catch (error) {
    ElMessage.error('粘贴失败: ' + (error.message || String(error)))
  }
}

// ---- 生命周期 ----
watch(() => selectedDirectoryId.value, () => {
  clearClipboard()
})

onMounted(() => {
  loadDirectories()
})
</script>

<style scoped>
.home {
  font-family: 'Microsoft YaHei', Arial, sans-serif;
}
.directory-aside {
  border-right: 1px solid #e6e6e6;
  overflow: hidden;
}
.file-tree-aside {
  border-right: 1px solid #e6e6e6;
  overflow: hidden;
}
.content-main {
  padding: 0;
  overflow-y: auto;
}
</style>
