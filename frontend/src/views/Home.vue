<template>
  <div class="home">
    <div class="home-layout">
      <ActivityBar @toggle-terminal="uiStore.toggleTerminal" @open-settings="uiStore.settingsVisible = true" />
      <div class="main-area">
        <!-- 上半区：原有 Splitpanes 三栏（AI 功能页 / 工具箱 / AI 对话页 / 素材管理 / 看板 / 仓库统计页
             激活时整体隐藏，v-show 保状态：运行中任务、已选文件、预览内容均保留，切回即恢复）。
             白名单排除：directory 之外的占满面板（toolbox/ai/ai-chat/assets/dashboard/stats）
             激活时三栏隐藏，与 AiFunctionPanel/AiChatPanel 等的互斥占满对称。 -->
        <div v-show="uiStore.activePanel === 'directory'" class="main-panes">
          <Splitpanes class="default-theme splitpanes-container" :push-other-panes="false" :maximize-panes="false">
            <Pane :size="20" :min-size="10">
              <div class="pane-content" style="position:relative;" @mousedown.capture="workspaceStore.lastInteractedTree = 'directory'">
                <DirectoryTree
                  ref="directoryTreeRef"
                  @select="onDirectorySelect"
                  @change="directoryStore.loadDirectories"
                  @contextmenu="onDirectoryContextMenu"
                  @batch-pull="onBatchPull"
                  @open-repo-filter="uiStore.openRepoFilter"
                />
              </div>
            </Pane>
            <Pane :size="30" :min-size="15">
              <div class="pane-content" @mousedown.capture="workspaceStore.lastInteractedTree = 'file'">
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
                  @set-as-work-dir="onSetAsWorkDir"
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
              <div class="pane-content">
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
             v-show 常驻挂载：切走再切回不丢运行中任务/已开 Tab/已录参数。 -->
        <AiFunctionPanel v-show="uiStore.activePanel === 'ai'" />
        <!-- 工具箱：活动栏一级入口，与三栏区互斥独占整个窗口区域（全窗口面板化，
             左侧功能清单 + 右侧选中工具内容面板，与 AiFunctionPanel 同级同互斥语义）。
             v-show 常驻挂载：切走再切回保留选中工具与各面板内已录参数；
             @close 切回 directory（关闭按钮语义对齐其他独占面板）。 -->
        <ToolboxPanel
          v-show="uiStore.activePanel === 'toolbox'"
          @close="uiStore.activePanel = 'directory'"
        />
        <!-- 仓库统计页：活动栏一级入口，与三栏区互斥占满主窗口上半区。
             v-show 常驻挂载：切走再切回不丢已加载统计与档位选择。
             从 workspaceStore.selectedNode 取仓库路径，后端 FindGitRoot 定位 git 根。 -->
        <StatsView v-show="uiStore.activePanel === 'stats'" />
        <!-- 全局状态看板：活动栏一级入口，与三栏区互斥占满主窗口上半区。
             v-show 常驻挂载：切走再切回不丢已加载状态。pin 仓库列表持久化后端。
             @locate 复用 onRepoLocate 跳转范式（切工作目录 + 展开文件树定位）。 -->
        <DashboardView v-show="uiStore.activePanel === 'dashboard'" @locate="onRepoLocate" />
        <!-- AI 对话工作台：活动栏一级入口，与三栏区互斥占满主窗口上半区。
             v-show 常驻挂载：切走再切回不丢选中目录与在途任务态（对齐 AiFunctionPanel）。 -->
        <AiChatPanel v-show="uiStore.activePanel === 'ai-chat'" />
        <!-- 素材管理：活动栏一级入口，与三栏区互斥占满主窗口上半区。
             v-show 常驻挂载：切走再切回不丢选中库/检索条件/详情选中态。 -->
        <AssetsPanel v-show="uiStore.activePanel === 'assets'" />
        <!-- 分支对比：活动栏一级入口，与三栏区互斥占满主窗口上半区。
             v-show 常驻挂载：切走再切回不丢分支选择与对比结果（PRD 非目标：不持久化，
             仅会话内保留）。只读面板：不提供 cherry-pick/merge/rebase 等写操作入口。 -->
        <BranchCompareView v-show="uiStore.activePanel === 'branchcompare'" />
        <!-- 临时待办：活动栏一级入口，与三栏区互斥占满主窗口上半区。
             v-show 常驻挂载：切走再切回保留搜索关键字、展开态与草稿。
             全局快捷键经 ref 调 quickAddFromClipboard，不依赖面板是否可见。 -->
        <TempTodoPanel ref="tempTodoPanelRef" v-show="uiStore.activePanel === 'todo'" />
        <!-- 拖拽分隔条（全屏态隐藏：fixed 覆盖整窗时高度拖拽无意义） -->
        <div
          v-if="uiStore.terminalVisible && !uiStore.terminalFullscreen"
          class="resize-bar"
          @mousedown="onResizeBarMouseDown"
        ></div>
        <!-- 下半区：终端面板（全屏态不输出 inline height，避免压住 .is-fullscreen 的 inset 拉伸） -->
        <TerminalPanel
          :style="uiStore.terminalVisible && !uiStore.terminalFullscreen ? { height: uiStore.terminalHeight + 'px' } : {}"
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
import AiChatPanel from '../components/AiChatPanel.vue'
import AssetsPanel from '../components/AssetsPanel.vue'
import TempTodoPanel from '../components/TempTodoPanel.vue'
import StatsView from './StatsView.vue'
import DashboardView from './DashboardView.vue'
import BranchCompareView from './BranchCompareView.vue'
import SettingsPanel from '../components/SettingsPanel.vue'
import TerminalPanel from '../components/TerminalPanel.vue'
import CommandPalette from '../components/CommandPalette.vue'
import UpdateDialog from '../components/UpdateDialog.vue'
import RepoFilterDialog from '../components/RepoFilterDialog.vue'
import { useRecentAccess } from '../composables/useRecentAccess'
import { restoreSession, applySessionState, startSessionAutoSave } from '../composables/useSessionState'
import { findOwningDirectory } from '../utils/pathMatch'
import { useSettingsStore, useUiStore, useDirectoryStore, useWorkspaceStore, matchShortcut, sidebarShortcutIndex } from '../store'
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
const tempTodoPanelRef = ref()

// 当点击 DirectoryTree 右键菜单时，关闭 FileTreePanel 的菜单
const onDirectoryContextMenu = () => {
  fileTreePanelRef.value?.closeMenu()
}

// 当点击 FileTreePanel 右键菜单时，关闭 DirectoryTree 的菜单
const onFileTreeContextMenu = () => {
  directoryTreeRef.value?.closeMenu()
}

// ---- 切换工作目录 ----
// 选择工作目录 = 退出聚合视图并切到该目录（聚合模式仅由工具栏切换按钮进入）。
// 该判定落在本统一入口，目录树点击、命令面板「工作目录」跳转、收藏跳转等调用方
// 自动获得同一语义，无需各自补退出逻辑。
// 三件常驻约束：
//   1) skipSave：调用方已完成旧目录状态保存（如从聚合视图退出后切目录）时跳过，
//      否则会把刚重建、全部收起的普通树空状态覆盖到旧目录的历史展开记忆上；
//   2) dirSelectSeq 请求序号：每个 await 之后校验代次，过期请求直接 return
//      （不改选中、不恢复状态），防「退出聚合期间连点目录」时先发请求在过渡态回写；
//   3) dirSelectInFlight 在途标志：在途期间的并发调用不执行 saveCurrentState——
//      此刻树处于「旧树已拆、新树未就绪」的过渡态，保存必然写错；仅最新代次复位该标志。
// 选中写入刻意提前到退出聚合的 await 之前：聚合树 treeKey 与 selectedDirectoryId
// 无关、不会提前重建，普通树切换本就同步写入；使「退出聚合重建」与「切目录重建」
// 合并为同一次 el-tree 重建，也消除选中被推迟到旧树根加载完成的问题。
// saveCurrentState 仍只允许在入口同步阶段执行（不能在 await 之后）；
// 聚合态进入时的树状态由 switchTreeMode 负责保存，本入口跳过。
let dirSelectSeq = 0
let dirSelectInFlight = false

// 内容面板状态随选中目录同步：按 newDir.isGitRepo 一次性算出目标值，避免 null 中间态
// 导致 content-inner 卸载再挂载（双刷新）。ContentPanel 模板 v-if="workspaceStore.selectedNode"
// 在 null 时会卸载整个面板，若先置 null 再设 git 节点，会触发"先卸载后挂载"两次刷新。
// 一次性算目标值使 gitA→gitB 切换时面板始终挂载，仅 GitInfo.repoPath 变化触发
// watch 单次 loadGitInfo（与文件树切换一致）。
const applyDirectorySelection = (newDir) => {
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
}

const onDirectorySelect = async (dirId, options = {}) => {
  const seq = ++dirSelectSeq
  // 聚合状态必须在本轮首个 await 前读取：switchTreeMode 会在其第一个 await 前
  // 同步翻转 aggregatedMode，await 后再读只能得到翻转后的值
  const wasAggregated = fileTreePanelRef.value?.isAggregatedMode?.() === true

  // 1. 保存当前工作目录的树状态：仅入口同步阶段、非在途并发调用、且非聚合态
  //    （聚合态树状态已在进入时由 switchTreeMode 保存；退出后重建的是空树，保存会覆盖记忆）
  if (!dirSelectInFlight && !options.skipSave && !wasAggregated && directoryStore.selectedDirectoryId) {
    const currentDir = directoryStore.directories.find(d => d.id === directoryStore.selectedDirectoryId)
    if (currentDir) {
      fileTreePanelRef.value?.saveCurrentState(currentDir.path)
    }
  }

  // 2. 先查目标目录（directories 列表已就绪，不依赖 nextTick）
  const newDir = directoryStore.directories.find(d => d.id === dirId)
  const prevDirId = directoryStore.selectedDirectoryId

  // 3. 提前写选中（见上方合并重建说明）：普通树无 await 中断，内容面板状态同步更新；
  //    聚合态则等退出完成后再写，避免用户随即切回聚合时还要回滚内容面板状态
  directoryStore.selectedDirectoryId = dirId
  if (!wasAggregated) applyDirectorySelection(newDir)

  dirSelectInFlight = true
  try {
    if (wasAggregated) {
      // 4. 聚合模式下先退出（不恢复旧树状态：聚合态树状态已在 switchTreeMode 内保存）
      await fileTreePanelRef.value?.exitAggregatedMode?.({ restoreState: false })
      if (seq !== dirSelectSeq) return
      // 退出期间用户又点工具栏切回聚合：switchTreeMode 代次守卫让本次 exit 提前返回，
      // 此时「选择工作目录 = 退出聚合」的语义已作废——放弃本次选目录并回滚提前写入的
      // 选中（聚合树 treeKey 与目录无关，回滚不触发重建）
      if (fileTreePanelRef.value?.isAggregatedMode?.() === true) {
        directoryStore.selectedDirectoryId = prevDirId
        return
      }
      applyDirectorySelection(newDir)
    }

    // 5. 等文件树按新 selectedDirectoryId 重渲染后恢复树状态
    await nextTick()
    if (seq !== dirSelectSeq) return
    if (newDir) {
      fileTreePanelRef.value?.restoreTreeState(newDir.path)
    }
  } finally {
    // 仅最新代次复位：过期请求不得清掉仍在途请求的标志
    if (seq === dirSelectSeq) dirSelectInFlight = false
  }
}

// ---- 选中文件树节点 ----
// opts.autoBlame：追溯联动场景（提交历史「行归属」）透传 previewFile，预览完成后自动开启行归属
const onNodeSelect = (data, opts = {}) => {
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
    // opts 仅追溯联动场景携带 autoBlame；普通树点击保持两参调用（不透传空 opts）
    if (opts.autoBlame) {
      contentPanelRef.value?.previewFile(data.path, data.name, opts)
    } else {
      contentPanelRef.value?.previewFile(data.path, data.name)
    }
  } else {
    contentPanelRef.value?.clearPreview()
  }
  recordAccess({ path: data.path, type: data.type, workDir: directoryStore.currentDirPath })
}

// ---- 追溯联动信箱消费（提交历史/文件 diff 的「行归属」→ 预览自动开 blame） ----
// 一次性信箱：消费即清空（置 null）。构造最小文件节点形状走 onNodeSelect 全链路
// （selectedNode 切换/latestCommit 清零/previewFile/recordAccess 与文件树点击一致）。
watch(() => uiStore.traceBlameRequest, (req) => {
  if (!req || !req.absPath) return
  uiStore.traceBlameRequest = null
  const lastSep = Math.max(req.absPath.lastIndexOf('/'), req.absPath.lastIndexOf('\\'))
  onNodeSelect({
    type: 'file',
    path: req.absPath,
    name: lastSep >= 0 ? req.absPath.slice(lastSep + 1) : req.absPath
  }, { autoBlame: true })
})

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

// ---- 设为工作目录（聚合视图右键收藏目录，FileTreePanel set-as-work-dir 事件）----
// 两步编排，复用既有链路：
//   1. 目标目录已在工作目录列表（路径归一比较）则复用；否则 AddDirectory 添加后刷新列表
//   2. onDirectorySelect 切换：该统一入口在聚合模式下自动退出聚合（不恢复旧树状态），
//      本次跳过旧目录状态保存（旧目录普通树状态已在进入聚合时保存）、改选中、恢复目标目录树状态
// 不再调 locateNode：普通树的工作目录根不在树节点中，relative 为空恒早退，属无效调用。
// 路径比较按 Windows 语义归一：分隔符统一、去尾分隔符、大小写不敏感；
// 后端 AddDirectory 用 filepath.Abs 归一后精确去重，尾分隔符差异会被判重返回 nil，
// 因此前端须先按同口径命中，避免「目录已存在却报添加失败」
const normalizeDirPath = (p) => (p || '').replace(/\//g, '\\').replace(/\\+$/, '').toLowerCase()
const isSameDirPath = (a, b) => normalizeDirPath(a) === normalizeDirPath(b)

const onSetAsWorkDir = async (data) => {
  if (!data?.path) return
  try {
    let target = directoryStore.directories.find(d => isSameDirPath(d.path, data.path))
    if (!target) {
      const dir = await AddDirectory(data.name, data.path, false)
      await directoryStore.loadDirectories()
      // 后端判重时 AddDirectory 返回 nil（路径写法差异仍属同一目录）：回查一次列表再判定
      target = (dir && directoryStore.directories.find(d => d.id === dir.id))
        || directoryStore.directories.find(d => isSameDirPath(d.path, data.path))
      if (!target) {
        ElMessage.error('添加工作目录失败')
        return
      }
    }
    if (!target) {
      ElMessage.error('未找到目标工作目录')
      return
    }
    // 退出聚合与跳过旧状态保存由 onDirectorySelect 统一入口处理（见其入口注释）
    await onDirectorySelect(target.id, { skipSave: true })
    ElMessage.success('已设为工作目录')
  } catch (error) {
    ElMessage.error('设为工作目录失败: ' + (error.message || String(error)))
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
// 跨目录跳转必须 await onDirectorySelect 完成后再 locateNode：
// 聚合态下该入口先 await 退出聚合（选中虽已提前写入，但新树重建与状态恢复在 await 之后），
// 旧实现只 nextTick 会在旧工作目录/聚合树上下文中定位，跨目录目标前缀判定失败而静默返回。
// 同目录分支（无需切换）保持原有同步 locateNode 行为不变。
async function onPaletteSelectFile(item) {
  recordAccess({ path: item.path, type: item.type, workDir: directoryStore.currentDirPath })
  if (item.path.startsWith(directoryStore.currentDirPath)) {
    fileTreePanelRef.value?.locateNode(item.path)
  } else {
    const targetDir = directoryStore.directories.find(d => item.path.startsWith(d.path))
    if (targetDir) {
      await onDirectorySelect(targetDir.id)
      fileTreePanelRef.value?.locateNode(item.path)
    }
  }
}

async function onPaletteSelectFavorite(fav) {
  recordAccess({ path: fav.path, type: 'dir', workDir: directoryStore.currentDirPath })
  if (fav.path.startsWith(directoryStore.currentDirPath)) {
    fileTreePanelRef.value?.locateNode(fav.path)
  } else {
    const targetDir = directoryStore.directories.find(d => fav.path.startsWith(d.path))
    if (targetDir) {
      await onDirectorySelect(targetDir.id)
      fileTreePanelRef.value?.locateNode(fav.path)
    }
  }
}

function onPaletteSelectWorkDir(dir) {
  onDirectorySelect(dir.id)
}

// ---- 仓库筛选器 / 状态看板：跳转定位（跨工作目录衔接）----
// 时序严格参考 research/cross-workdir-locate.md：
//   1. 规范化路径（\ -> / + toLowerCase）查找 targetDir，规避 locateNode 内 startsWith 未处理大小写的静默失败
//   2. 尽早关闭弹窗，避免遮挡文件树
//   3. 跨工作目录：await onDirectorySelect 触发 treeKey 变化 -> treeReadyPromise 重置 -> el-tree 重建，
//      让 restoreTreeState（历史展开）先完成，再 locateNode，避免并发竞争同一节点 expand/loadData
//   4. locateNode 内部 await treeReadyPromise 兜底等新树就绪，再沿父路径逐级展开 + setCurrentKey + scrollBy
//   同工作目录：跳过步骤 3，treeKey 不变、treeReadyPromise 旧值已 resolve，locateNode 立即执行
// 状态看板入口（activePanel==='dashboard'）须切到 directory：看板与三栏 .main-panes v-show 互斥
// （仅 directory 显示三栏），看板下不切则文件树 display:none、locateNode 对隐藏树不可见 = 无反应。
// 仓库筛选器入口仅存在于三栏内（DirectoryTree/FileTreePanel），activePanel==='directory' 时
// 三栏可见，保持当前面板不强制切换；工具箱等独占面板下三栏隐藏、无此入口，不涉及本函数。
// 工作目录查找用 findOwningDirectory 最长前缀匹配，嵌套工作目录（D:\projects 与 D:\projects\sub 并存）
// 取最具体者，避免选中外层导致 locateNode 在错误树里定位。
const onRepoLocate = async (repoPath) => {
  if (!repoPath) return

  const targetDir = findOwningDirectory(repoPath, directoryStore.directories)
  if (!targetDir) {
    ElMessage.warning('未找到该仓库所属的工作目录')
    return
  }

  // 关闭弹窗（仓库筛选器场景；看板场景无弹窗，赋值幂等）
  uiStore.repoFilterVisible = false

  // 仅看板入口切面板：directory 下赋值幂等，toolbox 下保持工具箱不退出
  if (uiStore.activePanel === 'dashboard') {
    uiStore.activePanel = 'directory'
  }

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

  // 临时待办：从剪贴板快速记录（快捷键可自定义）。输入框/对话框/终端聚焦时放行——
  // 既不打断正在进行的输入，也不劫持终端程序自身的组合键。记录不切换面板、不抢焦点，
  // 用户此刻多半正看着 AI 输出，强制跳转比「看不见结果」更打断。
  if (matchShortcut(e, settingsStore.shortcutQuickTodo)) {
    if (isEditableTarget(e.target) || isAnyOverlayOpen() || isTerminalFocused()) return
    e.preventDefault()
    tempTodoPanelRef.value?.quickAddFromClipboard()
    return
  }

  // 切换侧边栏面板（Alt+数字按当前顺序编号，顺序可在设置中调整）。焦点判定：
  // 输入框/对话框/终端聚焦时不触发——终端程序（tmux 等）常用 Alt+N 切 tab，
  // 劫持会破坏终端内快捷键。序号超出面板数（如面板缩减后按 Alt+9）无语义，
  // 不 preventDefault 直接放行
  const sidebarIdx = sidebarShortcutIndex(e)
  if (sidebarIdx >= 0) {
    const panels = settingsStore.orderedPanels
    if (sidebarIdx < panels.length) {
      if (isEditableTarget(e.target) || isAnyOverlayOpen() || isTerminalFocused()) return
      e.preventDefault()
      uiStore.activePanel = panels[sidebarIdx].id
    }
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
    // 素材管理面板内无粘贴语义：全局 Ctrl+V 只服务工作目录文件树，素材面板激活
    // 时不阻止默认也不向工作区树投递——否则用户在素材树选中的目录会被忽略，
    // 粘贴落到用户没在看的「工作目录」树选中节点（串台）
    if (uiStore.activePanel === 'assets') return
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
      // 统一刷新出口：FileTreePanel 内部按筛选态分流（筛选激活保语境重拉命中列表，树语境现状）
      fileTreePanelRef.value?.refreshAfterFileOp(targetDir)
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
      // 统一刷新出口：FileTreePanel 内部按筛选态分流（筛选激活保语境重拉命中列表，
      // 树语境现状）；命中后自动展开并加载最新子节点，解决拷贝后目标收起问题
      await fileTreePanelRef.value?.refreshAfterFileOp(data.targetPath)
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

// 会话快照自动保存句柄（debounce + beforeunload），onBeforeUnmount 时 dispose 释放监听
let sessionAutoSave = null

onMounted(() => {
  // 启动会话快照自动保存（debounce 写 + beforeunload 最终写），恢复完成后才启用避免恢复写回触发保存
  sessionAutoSave = startSessionAutoSave()

  // 启动流程：先用缓存渲染列表（秒回），再异步刷新 git 标记 + 恢复上次会话 UI 状态。
  directoryStore.loadDirectories().then(async () => {
    directoryStore.refreshGitFlags()
    // 恢复上次会话 UI 状态（崩溃/异常退出后启动时恢复；首次启动或无快照时走默认冷启动）
    const snapshot = await restoreSession()
    if (snapshot) {
      let restored = applySessionState(snapshot)
      // 恢复当前工作目录：仅当该目录仍存在于列表中时，复用 onDirectorySelect 切换
      // （顺带触发文件树重载 + useTreeState 的 localStorage 展开状态还原，不纳入 session.json 重复持久化）
      if (snapshot.selectedDirectoryId) {
        const exists = directoryStore.directories.some(d => d.id === snapshot.selectedDirectoryId)
        if (exists && snapshot.selectedDirectoryId !== directoryStore.selectedDirectoryId) {
          await onDirectorySelect(snapshot.selectedDirectoryId)
          restored = true
        }
      }
      if (restored) {
        ElMessage.success({ message: '已恢复上次会话', duration: 1500 })
      }
    }
    // 恢复阶段完成，启用自动保存
    sessionAutoSave.markRestored()
  })
  settingsStore.loadShortcuts()
  GetAppVersion().then(v => { uiStore.appVersion = v }).catch(() => {})
  document.addEventListener('keydown', handleGlobalKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleGlobalKeydown)
  if (sessionAutoSave) {
    sessionAutoSave.dispose()
    sessionAutoSave = null
  }
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
