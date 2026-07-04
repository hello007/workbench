<template>
  <div class="home">
    <div class="home-layout">
      <ActivityBar v-model="activePanel" :terminal-active="terminalVisible" @toggle-terminal="toggleTerminal" @open-settings="settingsVisible = true" />
      <div class="main-area">
        <!-- 上半区：原有 Splitpanes 三栏 -->
        <div class="main-panes">
          <Splitpanes class="default-theme splitpanes-container" :push-other-panes="false" :maximize-panes="false">
            <Pane :size="20" :min-size="10">
              <div class="pane-content" style="position:relative;">
                <DirectoryTree
                  v-show="activePanel === 'directory'"
                  ref="directoryTreeRef"
                  :directories="directories"
                  :selected-id="selectedDirectoryId"
                  :version="appVersion"
                  @select="onDirectorySelect"
                  @change="loadDirectories"
                  @contextmenu="onDirectoryContextMenu"
                  @batch-pull="onBatchPull"
                />
                <ToolboxPanel
                  v-show="activePanel === 'toolbox'"
                  @close="activePanel = 'directory'"
                />
              </div>
            </Pane>
            <Pane :size="30" :min-size="15">
              <div class="pane-content" @mousedown="closeToolbox" @contextmenu="closeToolbox">
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
                  @delete="onDeleteFromFileTree"
                  @add-work-dir="onAddWorkDir"
                  @open-content-search="onOpenContentSearch"
                />
              </div>
            </Pane>
            <Pane :size="50" :min-size="30">
              <div class="pane-content" @mousedown="closeToolbox" @contextmenu="closeToolbox">
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
        <!-- 拖拽分隔条 -->
        <div
          v-if="terminalVisible"
          class="resize-bar"
          @mousedown="onResizeBarMouseDown"
        ></div>
        <!-- 下半区：终端面板 -->
        <TerminalPanel
          :visible="terminalVisible"
          :current-dir="terminalDir"
          :style="{ height: terminalVisible ? terminalHeight + 'px' : '0px' }"
          @toggle="toggleTerminal"
        />
      </div>
    </div>
    <SettingsPanel v-model:visible="settingsVisible" @update-available="onUpdateAvailable" />
    <UpdateDialog v-model:visible="updateDialogVisible" :update-info="updateInfo" />
    <CommandPalette
      v-model="commandPaletteVisible"
      :current-dir="currentDirPath"
      :work-dirs="directories"
      :content-search-init="contentSearchInit"
      @select-file="onPaletteSelectFile"
      @select-favorite="onPaletteSelectFavorite"
      @select-workdir="onPaletteSelectWorkDir"
    />
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, watch, nextTick, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { debug } from '../utils/debug'
import DirectoryTree from '../components/DirectoryTree.vue'
import FileTreePanel from '../components/FileTreePanel.vue'
import ContentPanel from '../components/ContentPanel.vue'
import ActivityBar from '../components/ActivityBar.vue'
import ToolboxPanel from '../components/ToolboxPanel.vue'
import SettingsPanel from '../components/SettingsPanel.vue'
import TerminalPanel from '../components/TerminalPanel.vue'
import CommandPalette from '../components/CommandPalette.vue'
import UpdateDialog from '../components/UpdateDialog.vue'
import { useRecentAccess } from '../composables/useRecentAccess'
import { useShortcuts } from '../composables/useShortcuts'
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
  ReadFromSystemClipboard,
  AddDirectory,
  RefreshDirectoriesGitFlag
} from '../../wailsjs/go/main/App'

// ---- 核心状态 ----
const directories = ref([])
const selectedDirectoryId = ref('')
const selectedNode = ref(null)
const latestCommit = ref(null)
const appVersion = ref('')
const activePanel = ref('directory')

const clipboard = reactive({
  mode: null,
  sourcePath: '',
  sourceName: '',
  sourceType: ''
})

// ---- 终端状态 ----
const terminalVisible = ref(false)
const terminalHeight = ref(200)
const terminalDir = ref('')

// ---- 设置弹窗状态 ----
const settingsVisible = ref(false)
const updateDialogVisible = ref(false)
const updateInfo = ref({})

// ---- Command Palette 状态 ----
const commandPaletteVisible = ref(false)
const contentSearchInit = ref('')
const { record: recordAccess } = useRecentAccess()
const { matchShortcut, loadShortcuts, shortcutCommandPalette, shortcutToggleTerminal } = useShortcuts()

// ---- 子组件 ref ----
const directoryTreeRef = ref()
const fileTreePanelRef = ref()
const contentPanelRef = ref()

// ---- computed ----
const currentDirPath = computed(() => {
  const dir = directories.value.find(d => d.id === selectedDirectoryId.value)
  return dir ? dir.path : ''
})

// ---- 右键菜单事件处理 ----
const closeToolbox = () => {
  if (activePanel.value === 'toolbox') {
    activePanel.value = 'directory'
  }
}

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

// ---- 启动后异步刷新工作目录的 git 标识 ----
// GetDirectories 启动时直接返回 directories.json 持久化的 IsGitRepo（秒回），
// 这里再调一次后端刷新覆盖"目录后来才纳管为 git"等变化。
// 仅替换 directories 列表（左栏 git 标记会因 dir.isGitRepo 更新自动刷新），
// 不动 selectedDirectoryId / selectedNode，避免打断用户已选中的目录与右栏状态。
const refreshGitFlags = async () => {
  try {
    const fresh = await RefreshDirectoriesGitFlag()
    if (fresh && fresh.length) {
      directories.value = fresh
    }
  } catch (error) {
    // 刷新失败不影响主流程，缓存值仍可用
    debug.log('刷新工作目录 git 标识失败:', error)
  }
}

// ---- 切换工作目录 ----
const onDirectorySelect = async (dirId) => {
  // 1. 保存当前工作目录的树状态
  if (selectedDirectoryId.value) {
    const currentDir = directories.value.find(d => d.id === selectedDirectoryId.value)
    if (currentDir) {
      fileTreePanelRef.value?.saveCurrentState(currentDir.path)
    }
  }

  // 2. 先查目标目录（directories 列表已就绪，不依赖 nextTick）
  const newDir = directories.value.find(d => d.id === dirId)

  // 3. 直接切到目标 selectedNode，避免 null 中间态导致 content-inner 卸载再挂载（双刷新）
  //    ContentPanel 模板 v-if="selectedNode" 在 null 时会卸载整个面板，
  //    若先置 null 再设 git 节点，会触发"先卸载后挂载"两次刷新。
  //    这里按 newDir.isGitRepo 一次性算出目标值，使 gitA→gitB 切换时面板始终挂载，
  //    仅 GitInfo.repoPath 变化触发 watch 单次 loadGitInfo（与文件树切换一致）。
  selectedDirectoryId.value = dirId
  latestCommit.value = null
  contentPanelRef.value?.clearPreview()
  selectedNode.value = newDir?.isGitRepo
    ? {
        id: newDir.id,
        path: newDir.path,
        name: newDir.name,
        type: 'directory',
        isGitRepo: true
      }
    : null

  // 4. 等文件树按新 selectedDirectoryId 重渲染后恢复树状态
  await nextTick()
  if (newDir) {
    fileTreePanelRef.value?.restoreTreeState(newDir.path)
  }
}

// ---- 选中文件树节点 ----
const onNodeSelect = (data) => {
  selectedNode.value = data
  // 切换文件树节点时清零 latestCommit，避免上一个仓库（经"提交历史"tab emit）
  // 的提交残留到新选中仓库的 GitInfo 面板（与 GitInfo.watch(repoPath) 协同）。
  latestCommit.value = null
  contentPanelRef.value?.clearPreview()
  recordAccess({ path: data.path, type: data.type, workDir: currentDirPath.value })
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

// ---- 添加为工作目录 ----
const onAddWorkDir = async (data) => {
  try {
    const dir = await AddDirectory(data.name, data.path, false)
    if (dir) {
      await loadDirectories()
      ElMessage.success('已添加为工作目录')
    } else {
      ElMessage.error('添加工作目录失败')
    }
  } catch (error) {
    ElMessage.error('添加工作目录失败: ' + (error.message || String(error)))
  }
}

// ---- ContentPanel 重命名 ----
const onRenameFromContent = (node) => {
  fileTreePanelRef.value?.showRenameAt(node)
}

// ---- FileTreePanel 删除 ----
const onDeleteFromFileTree = (node) => {
  if (!selectedNode.value) return
  const deletedPath = node.path.replace(/\\/g, '/')
  const selectedPath = selectedNode.value.path.replace(/\\/g, '/')
  if (selectedPath === deletedPath || selectedPath.startsWith(deletedPath + '/')) {
    selectedNode.value = null
    contentPanelRef.value?.clearPreview()
  }
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

// ---- Command Palette 事件处理 ----
function onPaletteSelectFile(item) {
  recordAccess({ path: item.path, type: item.type, workDir: currentDirPath.value })
  if (item.path.startsWith(currentDirPath.value)) {
    fileTreePanelRef.value?.locateNode(item.path)
  } else {
    const targetDir = directories.value.find(d => item.path.startsWith(d.path))
    if (targetDir) {
      onDirectorySelect(targetDir.id)
      nextTick(() => fileTreePanelRef.value?.locateNode(item.path))
    }
  }
}

function onPaletteSelectFavorite(fav) {
  recordAccess({ path: fav.path, type: 'dir', workDir: currentDirPath.value })
  if (fav.path.startsWith(currentDirPath.value)) {
    fileTreePanelRef.value?.locateNode(fav.path)
  } else {
    const targetDir = directories.value.find(d => fav.path.startsWith(d.path))
    if (targetDir) {
      onDirectorySelect(targetDir.id)
      nextTick(() => fileTreePanelRef.value?.locateNode(fav.path))
    }
  }
}

function onPaletteSelectWorkDir(dir) {
  onDirectorySelect(dir.id)
}

function onOpenContentSearch(subDir) {
  contentSearchInit.value = subDir ? ':' + subDir.replace(/\\/g, '/') + '/ ' : ':'
  commandPaletteVisible.value = true
}

// ---- 键盘快捷键 ----
const handleGlobalKeydown = (e) => {
  // 打开命令面板（快捷键可自定义）
  if (matchShortcut(e, shortcutCommandPalette.value)) {
    e.preventDefault()
    commandPaletteVisible.value = true
    return
  }

  // 切换终端（快捷键可自定义）
  if (matchShortcut(e, shortcutToggleTerminal.value)) {
    e.preventDefault()
    toggleTerminal()
    return
  }

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
    // 预览区选中文本时，交还浏览器原生复制，避免被劫持为复制文件路径
    const selection = window.getSelection()
    if (selection && selection.toString()) return
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

// ---- 更新 ----
function onUpdateAvailable(info) {
  updateInfo.value = info
  updateDialogVisible.value = true
}

// ---- 终端 ----
const toggleTerminal = () => {
  terminalVisible.value = !terminalVisible.value
}

// 更新终端跟随目录
watch(() => selectedNode.value, (node) => {
  if (node && node.type === 'directory') {
    terminalDir.value = node.path
  } else if (node && node.type === 'file') {
    const lastSep = Math.max(node.path.lastIndexOf('\\'), node.path.lastIndexOf('/'))
    terminalDir.value = lastSep > 0 ? node.path.substring(0, lastSep) : node.path
  }
})

watch(() => selectedDirectoryId.value, () => {
  const dir = directories.value.find(d => d.id === selectedDirectoryId.value)
  if (dir && !selectedNode.value) {
    terminalDir.value = dir.path
  }
})

// 拖拽分隔条
const onResizeBarMouseDown = (e) => {
  e.preventDefault()
  const startY = e.clientY
  const startHeight = terminalHeight.value

  const onMouseMove = (moveEvent) => {
    const delta = startY - moveEvent.clientY
    const newHeight = Math.max(100, Math.min(startHeight + delta, window.innerHeight - 200))
    terminalHeight.value = newHeight
  }

  const onMouseUp = () => {
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
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

// ---- 生命周期 ----
watch(() => selectedDirectoryId.value, () => {
  clearClipboard()
})

onMounted(() => {
  // 启动流程：先用缓存渲染列表（秒回），再异步刷新 git 标记。
  loadDirectories().then(() => refreshGitFlags())
  loadShortcuts()
  GetAppVersion().then(v => { appVersion.value = v }).catch(() => {})
  document.addEventListener('keydown', handleGlobalKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleGlobalKeydown)
})
</script>

<style scoped>
.home {
  height: 100vh;
  width: 100%;
  overflow: hidden !important;
  margin: 0;
  padding: 0;
  position: relative;
}

.home-layout {
  display: flex;
  height: 100%;
  width: 100%;
}

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
}

.main-panes {
  flex: 1;
  min-height: 0;
  overflow: hidden !important;
  position: relative;
}

.resize-bar {
  flex-shrink: 0;
  height: 3px;
  background: var(--border-color, #3c3c3c);
  cursor: ns-resize;
  transition: background 0.15s;
}

.resize-bar:hover {
  background: var(--primary-color, #409eff);
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
  background-color: var(--border-color) !important;
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
  box-shadow: 0 0 6px rgba(64, 158, 255, 0.25) !important;
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
