<template>
  <div class="home">
    <div class="splitpanes-wrapper">
      <Splitpanes class="default-theme splitpanes-container" :push-other-panes="false" :maximize-panes="false">
        <Pane :size="20" :min-size="10">
          <div class="pane-content">
            <DirectoryTree
              ref="directoryTreeRef"
              :directories="directories"
              :selected-id="selectedDirectoryId"
              :version="appVersion"
              @select="onDirectorySelect"
              @change="loadDirectories"
              @contextmenu="onDirectoryContextMenu"
            />
          </div>
        </Pane>
        <Pane :size="30" :min-size="15">
          <div class="pane-content">
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
              @copy-to="handleCopyTo"
              @contextmenu="onFileTreeContextMenu"
            />
          </div>
        </Pane>
        <Pane :size="50" :min-size="30">
          <div class="pane-content">
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
              @copy-to="node => fileTreePanelRef.showCopyToDialog(node)"
              @batch-pull="onBatchPull"
            />
          </div>
        </Pane>
      </Splitpanes>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { debug } from '../utils/debug'
import DirectoryTree from '../components/DirectoryTree.vue'
import FileTreePanel from '../components/FileTreePanel.vue'
import ContentPanel from '../components/ContentPanel.vue'
import { Splitpanes, Pane } from 'splitpanes'
import 'splitpanes/dist/splitpanes.css'
import {
  GetDirectories,
  GetAppVersion,
  ScanAndPullRepos,
  DeleteFile,
  CopyItem,
  CopyTo,
  MoveItem,
  CopyToSystemClipboard,
  CutToSystemClipboard,
  ReadFromSystemClipboard
} from '../../wailsjs/go/main/App'

// ---- 核心状态 ----
const directories = ref([])
const selectedDirectoryId = ref('')
const selectedNode = ref(null)
const latestCommit = ref(null)
const appVersion = ref('')

const clipboard = reactive({
  mode: null,
  sourcePath: '',
  sourceName: '',
  sourceType: ''
})

// ---- 子组件 ref ----
const directoryTreeRef = ref()
const fileTreePanelRef = ref()
const contentPanelRef = ref()

// ---- 右键菜单事件处理 ----
// 当点击 DirectoryTree 右键菜单时，关闭 FileTreePanel 的菜单
const onDirectoryContextMenu = () => {
  fileTreePanelRef.value?.closeMenu()
}

// 当点击 FileTreePanel 右键菜单时，关闭 DirectoryTree 的菜单
const onFileTreeContextMenu = () => {
  directoryTreeRef.value?.closeMenu()
}

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

// ---- 键盘快捷键 ----
const handleGlobalKeydown = (e) => {
  if (e.key === 'F5') {
    e.preventDefault()
    if (selectedNode.value) {
      fileTreePanelRef.value?.refreshNode(selectedNode.value.path)
    }
    return
  }

  if (!selectedNode.value) return
  if (!(e.ctrlKey || e.metaKey)) return

  const tag = e.target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA') return

  if (e.key === 'c') {
    e.preventDefault()
    handleCopy(selectedNode.value)
  } else if (e.key === 'x') {
    e.preventDefault()
    handleCut(selectedNode.value)
  } else if (e.key === 'v') {
    e.preventDefault()
    handlePaste(selectedNode.value)
  }
}

// ---- 剪贴板操作 ----
const clearClipboard = () => {
  clipboard.mode = null
  clipboard.sourcePath = ''
  clipboard.sourceName = ''
  clipboard.sourceType = ''
}

const handleCopy = async (data) => {
  clipboard.mode = 'copy'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 复制成功`)
  CopyToSystemClipboard(data.path).catch(() => {})
}

const handleCut = async (data) => {
  clipboard.mode = 'cut'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 剪切成功`)
  CutToSystemClipboard(data.path).catch(() => {})
}

const resolveTargetDir = (data) => {
  if (data.type === 'directory') {
    return data.path
  }
  const lastSep = Math.max(data.path.lastIndexOf('\\'), data.path.lastIndexOf('/'))
  return lastSep > 0 ? data.path.substring(0, lastSep) : ''
}

const handlePaste = async (targetData) => {
  const targetDir = resolveTargetDir(targetData)
  if (!targetDir) return

  try {
    const result = await ReadFromSystemClipboard()
    if (!result) {
      ElMessage.info('剪贴板中没有可粘贴的内容')
      return
    }

    const clipData = JSON.parse(result)
    const paths = clipData.paths || []
    const isCut = clipData.isCut || false

    if (paths.length === 0) {
      ElMessage.info('剪贴板中没有可粘贴的内容')
      return
    }

    let successCount = 0
    for (const srcPath of paths) {
      let res
      if (isCut) {
        res = await MoveItem(srcPath, targetDir)
      } else {
        res = await CopyItem(srcPath, targetDir)
      }
      if (res && !res.startsWith('错误')) {
        successCount++
      }
    }

    if (successCount > 0) {
      ElMessage.success(`粘贴成功：${successCount} 个项目`)
      fileTreePanelRef.value?.refreshNode(targetDir)
      if (isCut) clearClipboard()
    } else {
      ElMessage.error('粘贴失败')
    }
  } catch (error) {
    ElMessage.error('粘贴失败: ' + (error.message || String(error)))
  }
}

const handleCopyTo = async (data) => {
  fileTreePanelRef.value?.setCopyToLoading(true)
  try {
    const result = await CopyTo(data.sourcePath, data.targetPath, data.copyWholeDir)
    if (result && result.startsWith('错误')) {
      ElMessage.error(result)
    } else {
      ElMessage.success('拷贝成功')
      fileTreePanelRef.value?.closeCopyToDialog()
      fileTreePanelRef.value?.refreshNode(data.targetPath)
    }
  } catch (error) {
    ElMessage.error('拷贝失败: ' + (error.message || String(error)))
  } finally {
    fileTreePanelRef.value?.setCopyToLoading(false)
  }
}

// 窗口获得焦点时，从系统剪贴板同步内部状态
const handleWindowFocus = async () => {
  try {
    const result = await ReadFromSystemClipboard()
    if (!result) {
      clearClipboard()
      return
    }
    const clipData = JSON.parse(result)
    const paths = clipData.paths || []
    if (paths.length === 0) {
      clearClipboard()
      return
    }
    clipboard.mode = clipData.isCut ? 'cut' : 'copy'
    clipboard.sourcePath = paths[0]
    clipboard.sourceName = paths[0].split(/[\\/]/).pop()
    clipboard.sourceType = ''
  } catch {
    clearClipboard()
  }
}

// ---- 生命周期 ----
watch(() => selectedDirectoryId.value, () => {
  clearClipboard()
})

onMounted(() => {
  loadDirectories()
  GetAppVersion().then(v => { appVersion.value = v }).catch(() => {})
  document.addEventListener('keydown', handleGlobalKeydown)
  window.addEventListener('focus', handleWindowFocus)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleGlobalKeydown)
  window.removeEventListener('focus', handleWindowFocus)
})
</script>

<style scoped>
.home {
  font-family: 'Microsoft YaHei', Arial, sans-serif;
  height: 100vh;
  width: 100%;
  overflow: hidden !important;
  margin: 0;
  padding: 0;
  position: relative;
  display: flex;
  flex-direction: column;
}
.splitpanes-wrapper {
  flex: 1;
  height: 100%;
  width: 100%;
  overflow: hidden !important;
  position: relative;
}
.splitpanes-container {
  height: 100%;
  width: 100%;
  overflow: hidden !important;
}
.pane-content {
  height: 100%;
  width: 100%;
  overflow: hidden !important;
  display: flex;
  flex-direction: column;
}
</style>

<style>
/* 分隔线样式 - 强制覆盖默认样式 */
.default-theme.splitpanes--vertical > .splitpanes__splitter,
.splitpanes--vertical > .splitpanes__splitter {
  background-color: #e8eaed !important;
  border-left: none !important;
  width: 1px !important;
  margin-left: 0 !important;
  transition: all var(--transition-normal);
  position: relative !important;
  box-shadow: none !important;
}
.default-theme.splitpanes--vertical > .splitpanes__splitter:hover,
.splitpanes--vertical > .splitpanes__splitter:hover {
  background-color: var(--primary-color) !important;
  border-left: none !important;
  cursor: col-resize !important;
  width: 2px !important;
  box-shadow: 0 0 5px rgba(64, 158, 255, 0.3) !important;
}
/* 隐藏默认的分隔线装饰 - 最强优先级 */
* .splitpanes__splitter:before,
* .splitpanes__splitter:after,
.splitpanes__splitter:before,
.splitpanes__splitter:after,
.default-theme.splitpanes--vertical > .splitpanes__splitter:before,
.default-theme.splitpanes--vertical > .splitpanes__splitter:after,
.splitpanes--vertical > .splitpanes__splitter:before,
.splitpanes--vertical > .splitpanes__splitter:after,
.default-theme.splitpanes .splitpanes--vertical > .splitpanes__splitter:before,
.default-theme.splitpanes .splitpanes--vertical > .splitpanes__splitter:after,
.splitpanes .splitpanes--vertical > .splitpanes__splitter:before,
.splitpanes .splitpanes--vertical > .splitpanes__splitter:after,
.default-theme.splitpanes .splitpanes__splitter:before,
.default-theme.splitpanes .splitpanes__splitter:after {
  display: none !important;
  content: none !important;
  width: 0 !important;
  height: 0 !important;
  background-color: transparent !important;
  border: none !important;
}
/* 确保面板背景一致 */
.splitpanes.default-theme .splitpanes__pane {
  background-color: var(--bg-primary);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}
</style>
