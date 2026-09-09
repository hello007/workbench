<template>
  <div class="home">
    <div class="home-layout">
      <ActivityBar @toggle-terminal="uiStore.toggleTerminal" @open-settings="uiStore.settingsVisible = true" />
      <div class="main-area">
        <!-- 上半区：原有 Splitpanes 三栏（AI 功能页激活时整体隐藏，v-show 保状态：
             运行中任务、已选文件、预览内容均保留，切回即恢复） -->
        <div v-show="uiStore.activePanel !== 'ai'" class="main-panes">
          <Splitpanes class="default-theme splitpanes-container" :push-other-panes="false" :maximize-panes="false">
            <Pane :size="20" :min-size="10">
              <div class="pane-content" style="position:relative;" @mousedown.capture="workspaceStore.lastInteractedTree = 'directory'">
                <DirectoryTree
                  v-show="uiStore.activePanel === 'directory'"
                  ref="directoryTreeRef"
                  @select="onDirectorySelect"
                  @change="directoryStore.loadDirectories"
                  @contextmenu="onDirectoryContextMenu"
                  @batch-pull="onBatchPull"
                  @open-repo-filter="uiStore.openRepoFilter"
                />
                <ToolboxPanel
                  v-show="uiStore.activePanel === 'toolbox'"
                  @close="uiStore.activePanel = 'directory'"
                />
              </div>
            </Pane>
            <Pane :size="30" :min-size="15">
              <div class="pane-content" @mousedown.capture="workspaceStore.lastInteractedTree = 'file'" @mousedown="closeToolbox" @contextmenu="closeToolbox">
                <FileTreePanel
                  ref="fileTreePanelRef"
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
                  @open-repo-filter="uiStore.openRepoFilter()"
                >
                  <template #toolbar-extra>
                    <el-button size="small" @click="uiStore.openRepoFilter()">
                      仓库筛选
                    </el-button>
                  </template>
                </FileTreePanel>
              </div>
            </Pane>
            <Pane :size="50" :min-size="30">
              <div class="pane-content" @mousedown="closeToolbox" @contextmenu="closeToolbox">
                <ContentPanel
                  ref="contentPanelRef"
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
        <!-- AI 功能页：活动栏一级入口，与三栏区互斥占满主窗口上半区。
             v-show 常驻挂载：切走再切回不丢运行中任务/已开 Tab/已录参数。
             与 .main-panes 同级，点击本面板不会冒泡进三栏 pane 的
             closeToolbox handler（该 handler 仅绑定在 Splitpanes 内部 pane 上） -->
        <AiFunctionPanel v-show="uiStore.activePanel === 'ai'" />
        <!-- 拖拽分隔条 -->
        <div
          v-if="uiStore.terminalVisible"
          class="resize-bar"
          @mousedown="onResizeBarMouseDown"
        ></div>
        <!-- 下半区：终端面板 -->
        <TerminalPanel
          :style="{ height: uiStore.terminalVisible ? uiStore.terminalHeight + 'px' : '0px' }"
          @toggle="uiStore.toggleTerminal"
        />
      </div>
    </div>
    <SettingsPanel @update-available="onUpdateAvailable" />
    <UpdateDialog />
    <CommandPalette
      @select-file="onPaletteSelectFile"
      @select-favorite="onPaletteSelectFavorite"
      @select-workdir="onPaletteSelectWorkDir"
    />
    <RepoFilterDialog
      @locate="onRepoLocate"
    />
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import DirectoryTree from '../components/DirectoryTree.vue'
import FileTreePanel from '../components/FileTreePanel.vue'
import ContentPanel from '../components/ContentPanel.vue'
import ActivityBar from '../components/ActivityBar.vue'
import ToolboxPanel from '../components/ToolboxPanel.vue'
import AiFunctionPanel from '../components/AiFunctionPanel.vue'
import SettingsPanel from '../components/SettingsPanel.vue'
import TerminalPanel from '../components/TerminalPanel.vue'
import CommandPalette from '../components/CommandPalette.vue'
import UpdateDialog from '../components/UpdateDialog.vue'
import RepoFilterDialog from '../components/RepoFilterDialog.vue'
import { useRecentAccess } from '../composables/useRecentAccess'
import { useSettingsStore, useUiStore, useDirectoryStore, useWorkspaceStore, matchShortcut } from '../store'
import { Splitpanes, Pane } from 'splitpanes'
import 'splitpanes/dist/splitpanes.css'
import {
  GetAppVersion,
  ScanAndPullRepos,
  DeleteFile,
  CopyItem,
  CopyTo,
  MoveItem,
  CopyToSystemClipboard,
  CutToSystemClipboard,
  ReadFromSystemClipboard,
  AddDirectory
} from '../../wailsjs/go/main/App'

// ---- 核心状态（workspace 域已迁 workspaceStore；directory 域已迁 directoryStore）----
const directoryStore = useDirectoryStore()
const workspaceStore = useWorkspaceStore()

const { record: recordAccess } = useRecentAccess()
const settingsStore = useSettingsStore()
// UI 临时态（activePanel/终端3/弹窗 visible×6/appVersion）已迁 ui store
const uiStore = useUiStore()

// ---- 子组件 ref ----
const directoryTreeRef = ref()
const fileTreePanelRef = ref()
const contentPanelRef = ref()

// ---- 右键菜单事件处理 ----
const closeToolbox = () => {
  if (uiStore.activePanel === 'toolbox') {
    uiStore.activePanel = 'directory'
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

// ---- 切换工作目录 ----
const onDirectorySelect = async (dirId) => {
  // 1. 保存当前工作目录的树状态
  if (directoryStore.selectedDirectoryId) {
    const currentDir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
    if (currentDir) {
      fileTreePanelRef.value?.saveCurrentState(currentDir.path)
    }
  }

  // 2. 先查目标目录（directories 列表已就绪，不依赖 nextTick）
  const newDir = directoryStore.directories.find(d => d.id === dirId)

  // 3. 直接切到目标 selectedNode，避免 null 中间态导致 content-inner 卸载再挂载（双刷新）
  //    ContentPanel 模板 v-if="workspaceStore.selectedNode" 在 null 时会卸载整个面板，
  //    若先置 null 再设 git 节点，会触发"先卸载后挂载"两次刷新。
  //    这里按 newDir.isGitRepo 一次性算出目标值，使 gitA→gitB 切换时面板始终挂载，
  //    仅 GitInfo.repoPath 变化触发 watch 单次 loadGitInfo（与文件树切换一致）。
  directoryStore.selectedDirectoryId = dirId
  workspaceStore.latestCommit = null
  contentPanelRef.value?.clearPreview()
  workspaceStore.selectedNode = newDir?.isGitRepo
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
  workspaceStore.selectedNode = data
  // 切换文件树节点时清零 latestCommit，避免上一个仓库（经"提交历史"tab emit）
  // 的提交残留到新选中仓库的 GitInfo 面板（与 GitInfo.watch(repoPath) 协同）。
  workspaceStore.latestCommit = null
  // 按节点类型主动驱动预览：
  //   - file：直接 previewFile，使「同节点再点」（链接跳转后再点原节点）也能重新加载，
  //     不再依赖 ContentPanel 内 watch(selectedNode) 的引用变化判定。
  //   - 非文件：清空预览。
  //   「未保存修改」检查已在 previewFile 内部统一处理。
  //   显式传入 data.path / data.name：selectedNode 在 workspace store，子组件 ContentPanel 直读
  //   store 的响应式更新时机与原 props 一致（Vue 在 nextTick 才 patch），
  //   若用无参 previewFile()，其内部 `targetPath = overridePath || workspaceStore.selectedNode?.path`
  //   读到的仍是【旧节点】路径 → 预览到上一个文件。传 data.path 直接绕开更新时机。
  if (data.type === 'file') {
    contentPanelRef.value?.previewFile(data.path, data.name)
  } else {
    contentPanelRef.value?.clearPreview()
  }
  recordAccess({ path: data.path, type: data.type, workDir: directoryStore.currentDirPath })
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
      await directoryStore.loadDirectories()
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
  if (!workspaceStore.selectedNode) return
  const deletedPath = node.path.replace(/\\/g, '/')
  const selectedPath = workspaceStore.selectedNode.path.replace(/\\/g, '/')
  if (selectedPath === deletedPath || selectedPath.startsWith(deletedPath + '/')) {
    workspaceStore.selectedNode = null
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
      workspaceStore.selectedNode = null
    } else {
      ElMessage.error('删除失败')
    }
  } catch (error) {
    ElMessage.error('删除失败: ' + (error.message || String(error)))
  }
}

// ---- Command Palette 事件处理 ----
function onPaletteSelectFile(item) {
  recordAccess({ path: item.path, type: item.type, workDir: directoryStore.currentDirPath })
  if (item.path.startsWith(directoryStore.currentDirPath)) {
    fileTreePanelRef.value?.locateNode(item.path)
  } else {
    const targetDir = directoryStore.directories.find(d => item.path.startsWith(d.path))
    if (targetDir) {
      onDirectorySelect(targetDir.id)
      nextTick(() => fileTreePanelRef.value?.locateNode(item.path))
    }
  }
}

function onPaletteSelectFavorite(fav) {
  recordAccess({ path: fav.path, type: 'dir', workDir: directoryStore.currentDirPath })
  if (fav.path.startsWith(directoryStore.currentDirPath)) {
    fileTreePanelRef.value?.locateNode(fav.path)
  } else {
    const targetDir = directoryStore.directories.find(d => fav.path.startsWith(d.path))
    if (targetDir) {
      onDirectorySelect(targetDir.id)
      nextTick(() => fileTreePanelRef.value?.locateNode(fav.path))
    }
  }
}

function onPaletteSelectWorkDir(dir) {
  onDirectorySelect(dir.id)
}

// ---- 仓库筛选器：跳转定位（跨工作目录衔接）----
// 时序严格参考 research/cross-workdir-locate.md：
//   1. 规范化路径（\ -> / + toLowerCase）查找 targetDir，规避 locateNode 内 startsWith 未处理大小写的静默失败
//   2. 尽早关闭弹窗，避免遮挡文件树
//   3. 跨工作目录：await onDirectorySelect 触发 treeKey 变化 -> treeReadyPromise 重置 -> el-tree 重建，
//      让 restoreTreeState（历史展开）先完成，再 locateNode，避免并发竞争同一节点 expand/loadData
//   4. locateNode 内部 await treeReadyPromise 兜底等新树就绪，再沿父路径逐级展开 + setCurrentKey + scrollBy
//   同工作目录：跳过步骤 3，treeKey 不变、treeReadyPromise 旧值已 resolve，locateNode 立即执行
const onRepoLocate = async (repoPath) => {
  if (!repoPath) return

  const norm = (p) => (p || '').replace(/\\/g, '/').toLowerCase()
  const normTarget = norm(repoPath)
  const targetDir = directoryStore.directories.find(d => normTarget.startsWith(norm(d.path)))
  if (!targetDir) {
    ElMessage.warning('未找到该仓库所属的工作目录')
    return
  }

  // 关闭弹窗
  uiStore.repoFilterVisible = false

  // 跨工作目录：先切换（触发文件树重建）
  if (targetDir.id !== directoryStore.selectedDirectoryId) {
    await onDirectorySelect(targetDir.id)
  }

  // 定位到目标节点（内部 await treeReadyPromise 兜底）
  await fileTreePanelRef.value?.locateNode(repoPath)
}

// ---- 仓库筛选器：统一打开入口 ----
// 三个入口均走 uiStore.openRepoFilter action（封装 initialDirId + visible 赋值）：
//   1) DirectoryTree 右键"仓库筛选器" -> 携带 dirId，锁定到右键所选项
//   2) FileTreePanel 空白右键"仓库筛选器" -> 无 dirId，回退当前选中目录
//   3) FileTreePanel 工具栏按钮 -> 无 dirId，回退当前选中目录
// 关键：无 dirId 入口必须显式重置 initialDirId 为空，否则会残留上次右键锁定的目录，
// 导致"工具栏按钮打开"仍定位到旧目录而非当前选中目录。
// RepoFilterDialog 内 watch(repoFilterVisible) 按 initialDirId || currentDirId 优先级取值。

function onOpenContentSearch(subDir) {
  uiStore.contentSearchInit = subDir ? ':' + subDir.replace(/\\/g, '/') + '/ ' : ':'
  uiStore.commandPaletteVisible = true
}

// ---- 键盘快捷键 ----
// ---- 快捷键焦点判定：避免在输入框/对话框/终端中误触树操作（尤其 Del 永久删除文件）----
const isEditableTarget = (el) => {
  if (!el) return false
  const tag = el.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (el.isContentEditable) return true
  return false
}

const isTerminalFocused = () => {
  const el = document.activeElement
  if (!el) return false
  return !!el.closest('.terminal-panel, .xterm, .xterm-helper-textarea, .xterm-screen')
}

const isAnyOverlayOpen = () => {
  // Element Plus 对话框/消息框/抽屉可见时存在这些类
  const nodes = document.querySelectorAll('.el-dialog, .el-message-box, .el-drawer')
  for (const n of nodes) {
    if (n.offsetParent !== null) return true
  }
  return false
}

const handleGlobalKeydown = (e) => {
  // 打开命令面板（快捷键可自定义）
  if (matchShortcut(e, settingsStore.shortcutCommandPalette)) {
    e.preventDefault()
    uiStore.commandPaletteVisible = true
    return
  }

  // 切换终端（快捷键可自定义）
  if (matchShortcut(e, settingsStore.shortcutToggleTerminal)) {
    e.preventDefault()
    uiStore.toggleTerminal()
    return
  }

  if (e.key === 'F5') {
    e.preventDefault()
    if (workspaceStore.selectedNode) {
      fileTreePanelRef.value?.refreshNode(workspaceStore.selectedNode.path)
    }
    return
  }

  // 重命名 / 删除（快捷键可自定义，作用于最近交互的树面板）
  if (matchShortcut(e, settingsStore.shortcutRename) || matchShortcut(e, settingsStore.shortcutDelete)) {
    // 焦点判定：输入框/对话框/终端聚焦时不触发，避免误触（Del 文件树为永久删除）
    if (isEditableTarget(e.target) || isAnyOverlayOpen() || isTerminalFocused()) return
    e.preventDefault()
    const isRename = matchShortcut(e, settingsStore.shortcutRename)
    if (workspaceStore.lastInteractedTree === 'directory') {
      if (isRename) directoryTreeRef.value?.triggerRenameCurrent()
      else directoryTreeRef.value?.triggerDeleteCurrent()
    } else {
      if (isRename) fileTreePanelRef.value?.triggerRenameCurrent()
      else fileTreePanelRef.value?.triggerDeleteCurrent()
    }
    return
  }

  if (!workspaceStore.selectedNode) return
  if (!(e.ctrlKey || e.metaKey)) return

  const tag = e.target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA') return

  if (e.key === 'c') {
    // 预览区选中文本时，交还浏览器原生复制，避免被劫持为复制文件路径
    const selection = window.getSelection()
    if (selection && selection.toString()) return
    e.preventDefault()
    handleCopy(workspaceStore.selectedNode)
  } else if (e.key === 'x') {
    e.preventDefault()
    handleCut(workspaceStore.selectedNode)
  } else if (e.key === 'v') {
    e.preventDefault()
    handlePaste(workspaceStore.selectedNode)
  }
}

// ---- 更新 ----
function onUpdateAvailable(info) {
  uiStore.updateInfo = info
  uiStore.updateDialogVisible = true
}

// 更新终端跟随目录
watch(() => workspaceStore.selectedNode, (node) => {
  if (node && node.type === 'directory') {
    uiStore.terminalDir = node.path
  } else if (node && node.type === 'file') {
    const lastSep = Math.max(node.path.lastIndexOf('\\'), node.path.lastIndexOf('/'))
    uiStore.terminalDir = lastSep > 0 ? node.path.substring(0, lastSep) : node.path
  }
})

watch(() => directoryStore.selectedDirectoryId, () => {
  const dir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
  if (dir && !workspaceStore.selectedNode) {
    uiStore.terminalDir = dir.path
  }
})

// 拖拽分隔条
const onResizeBarMouseDown = (e) => {
  e.preventDefault()
  const startY = e.clientY
  const startHeight = uiStore.terminalHeight

  const onMouseMove = (moveEvent) => {
    const delta = startY - moveEvent.clientY
    const newHeight = Math.max(100, Math.min(startHeight + delta, window.innerHeight - 200))
    uiStore.terminalHeight = newHeight
  }

  const onMouseUp = () => {
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
  }

  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

// ---- 剪贴板操作 ----
const handleCopy = async (data) => {
  workspaceStore.clipboard.mode = 'copy'
  workspaceStore.clipboard.sourcePath = data.path
  workspaceStore.clipboard.sourceName = data.name
  workspaceStore.clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 复制成功`)
  CopyToSystemClipboard(data.path).catch(() => {})
}

const handleCut = async (data) => {
  workspaceStore.clipboard.mode = 'cut'
  workspaceStore.clipboard.sourcePath = data.path
  workspaceStore.clipboard.sourceName = data.name
  workspaceStore.clipboard.sourceType = data.type
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
      if (isCut) workspaceStore.clearClipboard()
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
    const result = await CopyTo(data.sourcePath, data.targetPath, data.targetName || '', data.copyWholeDir)
    if (result && result.startsWith('错误')) {
      ElMessage.error(result)
    } else {
      ElMessage.success('拷贝成功')
      fileTreePanelRef.value?.closeCopyToDialog()
      // 刷新目标文件夹（命中后自动展开并加载最新子节点，解决拷贝后目标收起问题）
      await fileTreePanelRef.value?.refreshNode(data.targetPath)
    }
  } catch (error) {
    ElMessage.error('拷贝失败: ' + (error.message || String(error)))
  } finally {
    fileTreePanelRef.value?.setCopyToLoading(false)
  }
}

// ---- 生命周期 ----
watch(() => directoryStore.selectedDirectoryId, () => {
  workspaceStore.clearClipboard()
})

onMounted(() => {
  // 启动流程：先用缓存渲染列表（秒回），再异步刷新 git 标记。
  directoryStore.loadDirectories().then(() => directoryStore.refreshGitFlags())
  settingsStore.loadShortcuts()
  GetAppVersion().then(v => { uiStore.appVersion = v }).catch(() => {})
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
