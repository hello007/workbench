<template>
  <div class="file-tree-aside">
    <div class="tree-toolbar">
      <el-button-group>
        <el-button size="small" @click="refreshAll">刷新</el-button>
        <el-button size="small" @click="expandAll" :loading="expanding">全部展开</el-button>
        <el-button size="small" @click="collapseAll">全部收起</el-button>
      </el-button-group>
    </div>
    <div class="tree-content" @contextmenu.prevent="onBlankAreaContextMenu">
      <el-tree
        v-if="selectedDirId"
        :key="treeKey"
        ref="fileTreeRef"
        :props="treeProps"
        node-key="path"
        lazy
        :expand-on-click-node="false"
        highlight-current
        :load="loadTreeNode"
        @node-click="onNodeClick"
        @node-contextmenu="onNodeContextMenu"
        @node-expand="closeContextMenu"
        @node-collapse="closeContextMenu"
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
            <template v-else>
              <img
                v-if="getIconForFile(data.name)"
                :src="getIconForFile(data.name)"
                class="tree-node-file-icon"
                :alt="data.name"
              />
              <el-icon v-else color="#606266" style="margin-right: 5px;">
                <Document />
              </el-icon>
            </template>
            <span :style="{
              color: data.type === 'directory'
                ? (node.expanded ? '#409EFF' : '#909399')
                : '#606266'
            }">
              {{ node.label }}
            </span>
            <img
              v-if="data.isGitRepo"
              :src="gitIcon"
              class="tree-node-git-img"
              alt="Git 仓库"
              title="Git 仓库"
            />
          </span>
        </template>
      </el-tree>
      <el-empty v-else description="请先选择工作目录" :image-size="100" />
    </div>

    <!-- 新建文件夹/文件对话框 -->
    <el-dialog
      v-model="createDialogVisible"
      :title="createType === 'directory' ? '新建文件夹' : '新建文件'"
      width="420px"
      append-to-body
    >
      <el-form label-width="80px">
        <el-form-item label="父文件夹">
          <el-input :model-value="createParentPath" disabled />
        </el-form-item>
        <el-form-item :label="createType === 'directory' ? '文件夹名' : '文件名'">
          <el-input
            ref="createInputRef"
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
      append-to-body
    >
      <el-form label-width="80px">
        <el-form-item label="当前名称">
          <el-input :model-value="renameNode?.name" disabled />
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

    <!-- 拷贝到对话框 -->
    <el-dialog
      v-model="copyToDialogVisible"
      title="拷贝到"
      width="480px"
      append-to-body
    >
      <el-form label-width="100px">
        <el-form-item label="原地址">
          <el-input
            v-model="copyToSourcePath"
            placeholder="请输入原文件或文件夹路径"
            :disabled="copyToLoading"
          />
        </el-form-item>
        <div class="swap-row">
          <el-button
            text
            size="small"
            :disabled="copyToLoading"
            @click="swapCopyToPaths"
          >
            <el-icon class="swap-icon"><Sort /></el-icon>
            互换
          </el-button>
        </div>
        <el-form-item label="目标地址">
          <el-input
            ref="copyToTargetInputRef"
            v-model="copyToTargetPath"
            placeholder="请输入目标文件夹路径"
            :disabled="copyToLoading"
            @keyup.enter="handleCopyTo"
          />
        </el-form-item>
        <el-form-item>
          <el-checkbox
            v-model="copyToWholeDir"
            :disabled="copyToLoading"
          >
            包含文件夹本身
          </el-checkbox>
        </el-form-item>
      </el-form>
      <div v-if="copyToPreview" class="copy-to-preview">
        <div class="copy-to-preview-label">拷贝效果预览</div>
        <div class="copy-to-preview-row">{{ copyToPreview.from }}</div>
        <div class="copy-to-preview-arrow">↓</div>
        <div class="copy-to-preview-row">{{ copyToPreview.to }}</div>
      </div>
      <template #footer>
        <el-button @click="copyToDialogVisible = false" :disabled="copyToLoading">取消</el-button>
        <el-button type="primary" @click="handleCopyTo" :loading="copyToLoading">确定</el-button>
      </template>
    </el-dialog>

    <!-- 自定义右键菜单 -->
    <ul
      v-if="contextMenu.visible"
      class="context-menu"
      :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
      @click.stop
      @mousedown.stop
    >
      <template v-if="contextMenu.isBlankArea">
        <li class="context-menu-item" @click="onMenuCommand('createFile')">
          <el-icon><DocumentAdd /></el-icon>新建文件
        </li>
        <li class="context-menu-item" @click="onMenuCommand('createDir')">
          <el-icon><FolderAdd /></el-icon>新建文件夹
        </li>
      </template>
      <template v-else-if="contextMenu.data?.type === 'directory'">
        <li class="context-menu-item" @click="onMenuCommand('createFile')">
          <el-icon><DocumentAdd /></el-icon>新建文件
        </li>
        <li class="context-menu-item" @click="onMenuCommand('createDir')">
          <el-icon><FolderAdd /></el-icon>新建文件夹
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('rename')">
          <el-icon><Edit /></el-icon>重命名
          <span class="context-menu-shortcut">{{ shortcutRename }}</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('delete')">
          <el-icon><Delete /></el-icon>删除
          <span class="context-menu-shortcut">{{ shortcutDelete }}</span>
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
          <span class="context-menu-shortcut">Ctrl+X</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
          <span class="context-menu-shortcut">Ctrl+C</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
          <span class="context-menu-shortcut">Ctrl+V</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyTo')">
          <el-icon><FolderAdd /></el-icon>拷贝到...
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('copyPath')">
          <el-icon><CopyDocument /></el-icon>复制路径
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
          <img :src="explorerIcon" class="context-menu-img-icon" alt="资源管理器" />在资源管理器中打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
          <img :src="vscodeIcon" class="context-menu-img-icon" alt="VSCode" />用 VSCode 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInWarp')">
          <img :src="warpIcon" class="context-menu-img-icon" alt="Warp" />用 Warp 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInObsidian')">
          <img :src="obsidianIcon" class="context-menu-img-icon" alt="Obsidian" />用 Obsidian 打开
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('refresh')">
          <el-icon><Refresh /></el-icon>刷新
          <span class="context-menu-shortcut">F5</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('pullRepos')">
          <el-icon><Refresh /></el-icon>更新仓库
        </li>
        <li class="context-menu-item" @click="onMenuCommand('contentSearch')">
          <el-icon><Search /></el-icon>在此目录中搜索
        </li>
        <li class="context-menu-divider" />
        <li v-if="isFavorited" class="context-menu-item" @click="onMenuCommand('removeFavorite')">
          <el-icon><StarFilled /></el-icon>取消收藏
        </li>
        <li v-else class="context-menu-item" @click="onMenuCommand('addFavorite')">
          <el-icon><Star /></el-icon>添加到收藏
        </li>
        <li class="context-menu-item" @click="onMenuCommand('addAsWorkDir')">
          <el-icon><FolderAdd /></el-icon>添加为工作目录
        </li>
      </template>
      <template v-else>
        <li class="context-menu-item" @click="onMenuCommand('rename')">
          <el-icon><Edit /></el-icon>重命名
          <span class="context-menu-shortcut">{{ shortcutRename }}</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('delete')">
          <el-icon><Delete /></el-icon>删除
          <span class="context-menu-shortcut">{{ shortcutDelete }}</span>
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
          <span class="context-menu-shortcut">Ctrl+X</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
          <span class="context-menu-shortcut">Ctrl+C</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
          <span class="context-menu-shortcut">Ctrl+V</span>
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyTo')">
          <el-icon><FolderAdd /></el-icon>拷贝到...
        </li>
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('copyPath')">
          <el-icon><CopyDocument /></el-icon>复制路径
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copyName')">
          <el-icon><CopyDocument /></el-icon>复制文件名
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openExplorer')">
          <img :src="explorerIcon" class="context-menu-img-icon" alt="资源管理器" />在资源管理器中打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
          <img :src="vscodeIcon" class="context-menu-img-icon" alt="VSCode" />用 VSCode 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInWarp')">
          <img :src="warpIcon" class="context-menu-img-icon" alt="Warp" />用 Warp 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openInObsidian')">
          <img :src="obsidianIcon" class="context-menu-img-icon" alt="Obsidian" />用 Obsidian 打开
        </li>
        <li class="context-menu-item" @click="onMenuCommand('openWithDefaultApp')">
          <el-icon><Open /></el-icon>用默认程序打开
        </li>
        <li class="context-menu-divider" />
        <li v-if="isFavorited" class="context-menu-item" @click="onMenuCommand('removeFavorite')">
          <el-icon><StarFilled /></el-icon>取消收藏
        </li>
        <li v-else class="context-menu-item" @click="onMenuCommand('addFavorite')">
          <el-icon><Star /></el-icon>添加到收藏
        </li>
      </template>
    </ul>
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Folder,
  FolderOpened,
  Document,
  FolderAdd,
  DocumentAdd,
  Edit,
  Delete,
  CopyDocument,
  Refresh,
  Open,
  Scissor,
  DocumentCopy,
  Star,
  StarFilled,
  Search,
  Sort
} from '@element-plus/icons-vue'
import { debug } from '../utils/debug'
import { getIconForFile } from '../utils/fileIconMap'
import { useTreeState } from '../composables/useTreeState'
import { useFavorites } from '../composables/useFavorites'
import { useShortcuts } from '../composables/useShortcuts'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  GetFileTree,
  CreateDirectory, CreateFile, RenameFile, DeleteFile,
  OpenInExplorer,
  OpenInVSCode,
  OpenInWarp,
  OpenWithDefaultApp,
  OpenInObsidian,
  OpenObsidianVaultManager,
  CopyObsidianVaultPath,
  AutoRegisterAndOpen,
  ScanAndPullRepos
} from '../../wailsjs/go/main/App'
import obsidianIcon from '../assets/icons/obsidian.png'
import explorerIcon from '../assets/icons/explorer.png'
import vscodeIcon from '../assets/icons/vscode.ico'
import warpIcon from '../assets/icons/warp.ico'
import gitIcon from '../assets/icons/git.png'

// ---- Props & Emits ----
const props = defineProps({
  directories: { type: Array, default: () => [] },
  selectedDirId: { type: String, default: '' },
  clipboard: { type: Object, default: () => ({ mode: null }) }
})

const emit = defineEmits(['select', 'batchPull', 'copy', 'cut', 'paste', 'copyTo', 'contextmenu', 'delete', 'add-work-dir', 'open-content-search'])

const { saveState, restoreState } = useTreeState()
const { addFavorite, removeFavorite, favorites, loadFavorites } = useFavorites()
const { shortcutRename, shortcutDelete } = useShortcuts()

// ---- Refs ----
const currentSelectedPath = ref('')
const fileTreeRef = ref()
const refreshCounter = ref(0)
const treeKey = computed(() => `${props.selectedDirId}_${refreshCounter.value}`)

let treeReadyResolve = null
let treeReadyPromise = new Promise(r => { treeReadyResolve = r })

function resetTreeReady() {
  treeReadyPromise = new Promise(r => { treeReadyResolve = r })
}

watch(treeKey, () => {
  resetTreeReady()
})

const treeProps = {
  label: 'name',
  children: 'children',
  isLeaf: 'isLeaf'
}

// ---- 右键菜单状态 ----
const contextMenu = reactive({
  visible: false,
  x: 0,
  y: 0,
  data: null,
  isBlankArea: false
})

const isFavorited = computed(() => {
  const favList = favorites.value
  const path = contextMenu.data?.path
  if (!path) return false
  return favList.some(f => f.path === path)
})

// ---- 新建对话框状态 ----
const createDialogVisible = ref(false)
const createType = ref('directory')
const createName = ref('')
const createLoading = ref(false)
const createParentData = ref(null)
const createParentPath = computed(() => createParentData.value?.path || '')

// ---- 重命名对话框状态 ----
const renameDialogVisible = ref(false)
const renameName = ref('')
const renameLoading = ref(false)
const createInputRef = ref()
const renameInputRef = ref()
const renameNode = ref(null)

// ---- 拷贝到对话框状态 ----
const copyToDialogVisible = ref(false)
const copyToSourcePath = ref('')
const copyToTargetPath = ref('')
const copyToWholeDir = ref(true)
const copyToLoading = ref(false)

const copyToTargetInputRef = ref()

const copyToPreview = computed(() => {
  const src = copyToSourcePath.value.trim().replaceAll('\\', '/')
  const dst = copyToTargetPath.value.trim().replaceAll('\\', '/')
  if (!src || !dst) return null

  const srcName = src.split('/').pop() || ''
  const normalizedDst = dst.replace(/\/+$/, '')
  if (copyToWholeDir.value) {
    return { from: src, to: normalizedDst + '/' + srcName }
  }
  return { from: src + '/*', to: normalizedDst + '/*' }
})

// 互换原地址与目标地址
const swapCopyToPaths = () => {
  const temp = copyToSourcePath.value
  copyToSourcePath.value = copyToTargetPath.value
  copyToTargetPath.value = temp
}

// ---- 懒加载 ----
const loadTreeNode = async (node, resolve) => {
  debug.log('loadTreeNode called, node:', node)
  debug.log('node.level:', node?.level)
  debug.log('node.data:', node?.data)

  let path
  if (!node || node.level === 0 || !node.data) {
    const dir = props.directories.find(d => d.id === props.selectedDirId)
    if (!dir) {
      debug.log('No directory found for ID:', props.selectedDirId)
      resolve([])
      return
    }
    path = dir.path
    debug.log('Loading root node for path:', path)
  } else {
    path = node.data.path
    debug.log('Loading child nodes for path:', path)
  }

  try {
    const nodes = await GetFileTree(path)
    debug.log('Got nodes for path', path, ':', nodes)

    const processedNodes = (nodes || []).map(n => ({
      ...n,
      isLeaf: n.type === 'file' || !n.hasChildren
    }))

    debug.log('Processed nodes:', processedNodes)
    resolve(processedNodes)

    if (!node || node.level === 0 || !node.data) {
      nextTick(() => treeReadyResolve?.())
    }
  } catch (error) {
    console.error('Error in loadTreeNode:', error)
    ElMessage.error('加载节点失败: ' + (error.message || error))
    resolve([])
  }
}

// ---- 节点点击 ----
const onNodeClick = (data, node) => {
  const clickedPath = data.path.replace(/\\/g, '/')
  const prevPath = currentSelectedPath.value.replace(/\\/g, '/')

  currentSelectedPath.value = data.path
  emit('select', data)

  if (data.isLeaf || data.type === 'file') return

  const isAncestor = prevPath.length > clickedPath.length
    && prevPath.startsWith(clickedPath + '/')

  if (isAncestor) return

  if (node.expanded) {
    node.collapse()
  } else {
    node.expand()
  }
}

// ---- 沿父路径向上回溯，找到第一个已展开的祖先节点 ----
const findExpandedAncestor = (nodePath, store) => {
  const dir = props.directories.find(d => d.id === props.selectedDirId)
  if (!dir) return null

  const rootPath = dir.path
  const sep = rootPath.includes('\\') ? '\\' : '/'
  const normalizedRoot = sep === '\\' ? rootPath.replace(/\//g, '\\') : rootPath.replace(/\\/g, '/')
  const normalizedTarget = sep === '\\' ? nodePath.replace(/\//g, '\\') : nodePath.replace(/\\/g, '/')

  if (!normalizedTarget.startsWith(normalizedRoot)) return null

  let segments = normalizedTarget.split(sep)
  while (segments.length > 1) {
    segments = segments.slice(0, -1)
    const parentPath = segments.join(sep)
    if (parentPath.length < normalizedRoot.length) return null
    const parent = store.nodesMap[parentPath]
    if (parent && parent.expanded === true) return parent
    if (parentPath === normalizedRoot) return null
  }
  return null
}

// ---- 刷新节点 ----
const refreshNode = (nodePath) => {
  if (!fileTreeRef.value || !nodePath) return

  const store = fileTreeRef.value.store
  const direct = store.nodesMap[nodePath]
  if (direct) {
    direct.loaded = false
    direct.loading = false
    direct.expand()
    return
  }

  const ancestor = findExpandedAncestor(nodePath, store)
  if (ancestor) {
    ancestor.loaded = false
    ancestor.loading = false
    ancestor.expand()
  }
}

// ---- 全部刷新 ----
const refreshAll = () => {
  refreshCounter.value++
  ElMessage.success('文件树已刷新')
}

// ---- 全部展开 ----
const expanding = ref(false)

const expandAll = async () => {
  if (!fileTreeRef.value) return

  expanding.value = true
  try {
    const expandNode = (node) => {
      return new Promise(resolve => {
        if (node.isLeaf) {
          resolve()
          return
        }
        node.expand(() => {
          Promise.all(node.childNodes.map(child => expandNode(child))).then(resolve)
        })
      })
    }

    const root = fileTreeRef.value.store.root
    await Promise.all(root.childNodes.map(child => expandNode(child)))
    ElMessage.success('已全部展开')
  } catch (error) {
    ElMessage.error('展开失败: ' + (error.message || String(error)))
  } finally {
    expanding.value = false
  }
}

// ---- 全部收起 ----
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

// ---- 右键菜单 ----
const onNodeContextMenu = (event, data) => {
  event.preventDefault()
  event.stopPropagation() // 恢复 stopPropagation()，防止事件冒泡

  // 通知父组件关闭另一个组件的菜单
  emit('contextmenu')

  // 先设置菜单位置
  let x = event.clientX
  let y = event.clientY

  contextMenu.x = x
  contextMenu.y = y
  contextMenu.data = data
  contextMenu.isBlankArea = false
  contextMenu.visible = true

  // 等菜单渲染完成后测量实际高度并调整位置
  nextTick(() => {
    const menuElement = document.querySelector('.context-menu')
    if (menuElement) {
      const rect = menuElement.getBoundingClientRect()
      const menuWidth = rect.width
      const menuHeight = rect.height

      // 重新检查并调整边界
      let adjustedX = x
      let adjustedY = y

      // 检查右侧边界
      if (adjustedX + menuWidth > window.innerWidth) {
        adjustedX = window.innerWidth - menuWidth - 5
      }

      // 检查底部边界
      if (adjustedY + menuHeight > window.innerHeight) {
        adjustedY = window.innerHeight - menuHeight - 5
      }

      // 检查左侧边界
      if (adjustedX < 5) {
        adjustedX = 5
      }

      // 检查顶部边界
      if (adjustedY < 5) {
        adjustedY = 5
      }

      // 如果位置有变化，更新菜单位置
      if (adjustedX !== x || adjustedY !== y) {
        contextMenu.x = adjustedX
        contextMenu.y = adjustedY
      }
    }
  })
}

const closeContextMenu = () => {
  contextMenu.visible = false
  contextMenu.isBlankArea = false
}

const onGlobalClick = () => {
  closeContextMenu()
}

// ---- 空白区域右键菜单 ----
const onBlankAreaContextMenu = (event) => {
  event.stopPropagation()

  const dir = props.directories.find(d => d.id === props.selectedDirId)
  if (!dir) return

  emit('contextmenu')

  let x = event.clientX
  let y = event.clientY

  contextMenu.x = x
  contextMenu.y = y
  contextMenu.data = { path: dir.path, name: dir.name, type: 'directory' }
  contextMenu.isBlankArea = true
  contextMenu.visible = true

  nextTick(() => {
    const menuElement = document.querySelector('.context-menu')
    if (menuElement) {
      const rect = menuElement.getBoundingClientRect()
      let adjustedX = x
      let adjustedY = y
      if (adjustedX + rect.width > window.innerWidth) adjustedX = window.innerWidth - rect.width - 5
      if (adjustedY + rect.height > window.innerHeight) adjustedY = window.innerHeight - rect.height - 5
      if (adjustedX < 5) adjustedX = 5
      if (adjustedY < 5) adjustedY = 5
      if (adjustedX !== x || adjustedY !== y) {
        contextMenu.x = adjustedX
        contextMenu.y = adjustedY
      }
    }
  })
}

const onGlobalContextMenu = () => {
  closeContextMenu()
}

// ---- 菜单命令分发 ----
const onMenuCommand = (command) => {
  const data = contextMenu.data
  closeContextMenu()
  if (!data) return

  switch (command) {
    case 'createFile':
      showCreateAt(data, 'file')
      break
    case 'createDir':
      showCreateAt(data, 'directory')
      break
    case 'rename':
      showRenameAt(data)
      break
    case 'delete':
      handleDeleteAt(data)
      break
    case 'cut':
      emit('cut', data)
      break
    case 'copy':
      emit('copy', data)
      break
    case 'paste':
      emit('paste', data)
      break
    case 'copyTo':
      showCopyToDialog(data)
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
    case 'openInWarp':
      handleOpenInWarp(data.path)
      break
    case 'openInObsidian':
      handleOpenObsidian(data.path)
      break
    case 'openWithDefaultApp':
      handleOpenWithDefaultApp(data.path)
      break
    case 'refresh':
      refreshNode(data.path)
      break
    case 'pullRepos':
      handleBatchPull(data)
      break
    case 'addFavorite':
      handleAddFavorite(data)
      break
    case 'removeFavorite':
      handleRemoveFavorite(data)
      break
    case 'addAsWorkDir':
      handleAddAsWorkDir(data)
      break
    case 'contentSearch': {
      const currentWorkDir = props.directories.find(d => d.id === props.selectedDirId)
      if (currentWorkDir && data.path.startsWith(currentWorkDir.path)) {
        const relPath = data.path.slice(currentWorkDir.path.length).replace(/^[\\\/]/, '')
        emit('open-content-search', relPath)
      } else {
        emit('open-content-search', '')
      }
      break
    }
  }
}

// ---- 新建文件/文件夹 ----
const showCreateAt = (data, type) => {
  createParentData.value = data
  createType.value = type
  createName.value = ''
  createDialogVisible.value = true
  setTimeout(() => {
    const input = createInputRef.value?.input
    if (input) {
      input.focus()
    }
  }, 100)
}

const handleCreate = async () => {
  if (!createName.value.trim()) {
    ElMessage.warning(createType.value === 'directory' ? '请输入文件夹名称' : '请输入文件名称')
    return
  }
  if (!createParentData.value) return

  createLoading.value = true
  try {
    let result
    if (createType.value === 'directory') {
      result = await CreateDirectory(createParentData.value.path, createName.value.trim())
    } else {
      result = await CreateFile(createParentData.value.path, createName.value.trim(), '')
    }
    if (result) {
      ElMessage.success(createType.value === 'directory' ? '文件夹创建成功' : '文件创建成功')
      createDialogVisible.value = false
      refreshNode(createParentData.value.path)
    } else {
      ElMessage.error('创建失败')
    }
  } catch (error) {
    ElMessage.error('创建失败: ' + (error.message || String(error)))
  } finally {
    createLoading.value = false
  }
}

// ---- 重命名 ----
const showRenameAt = (data) => {
  renameNode.value = data
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
  if (!renameNode.value) return

  renameLoading.value = true
  try {
    const result = await RenameFile(renameNode.value.path, renameName.value.trim())
    if (result) {
      ElMessage.success('重命名成功')
      renameDialogVisible.value = false
      const targetPath = renameNode.value.path
      let parentPath = targetPath.substring(0, targetPath.lastIndexOf('\\'))
      if (!parentPath) {
        parentPath = targetPath.substring(0, targetPath.lastIndexOf('/'))
      }
      refreshNode(parentPath)
    } else {
      ElMessage.error('重命名失败')
    }
  } catch (error) {
    ElMessage.error('重命名失败: ' + (error.message || String(error)))
  } finally {
    renameLoading.value = false
  }
}

// ---- 删除 ----
const handleDeleteAt = async (data) => {
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
      emit('delete', data)
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

// ---- 拷贝到对话框 ----
const showCopyToDialog = (data) => {
  copyToSourcePath.value = data.path.replaceAll('\\', '/')
  copyToTargetPath.value = ''
  copyToWholeDir.value = data.type === 'directory'
  copyToLoading.value = false
  copyToDialogVisible.value = true
  setTimeout(() => {
    const input = copyToTargetInputRef.value?.input
    if (input) {
      input.focus()
    }
  }, 100)
}

const handleCopyTo = () => {
  if (!copyToSourcePath.value.trim()) {
    ElMessage.warning('请输入原地址')
    return
  }
  if (!copyToTargetPath.value.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }

  emit('copyTo', {
    sourcePath: copyToSourcePath.value,
    targetPath: copyToTargetPath.value,
    copyWholeDir: copyToWholeDir.value
  })
}

// ---- 复制到剪贴板 ----
const copyToClipboard = async (text, label) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.error('复制失败')
  }
}

// ---- 外部工具 ----
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

const handleOpenInWarp = async (path) => {
  try {
    const result = await OpenInWarp(path)
    if (!result) {
      ElMessage.error('打开 Warp 失败，请确认已安装 Warp 终端')
    }
  } catch (error) {
    ElMessage.error('打开 Warp 失败: ' + (error.message || String(error)))
  }
}

const handleOpenObsidian = async (path) => {
  try {
    const status = await OpenInObsidian(path)
    if (status === 'not-installed') {
      ElMessage.error('未检测到 Obsidian，请在【设置 → 通用 → 外部应用】中配置 Obsidian 程序路径，或安装 Obsidian 并至少运行一次')
    } else if (status === 'not-registered') {
      try {
        await ElMessageBox.confirm('该目录未注册为 Obsidian vault。', '提示', {
          confirmButtonText: '自动注册并打开',
          cancelButtonText: '打开仓库管理器',
          distinguishCancelAndClose: true,
          type: 'info'
        })
        // 用户点「自动注册并打开」-> 二次确认（预告信任提示 + 备份 + 运行中需关闭）
        try {
          await ElMessageBox.confirm(
            '即将把该目录注册为 Obsidian vault 并打开。<br>• 首次打开会弹出信任提示，请选择「Trust author and enable plugins」<br>• Obsidian 配置已自动备份<br>• 若 Obsidian 正在运行，需先关闭后重试',
            '确认自动注册',
            { confirmButtonText: '继续', cancelButtonText: '取消', type: 'warning', dangerouslyUseHTMLString: true }
          )
          const regStatus = await AutoRegisterAndOpen(path)
          if (regStatus === '') {
            ElMessage.success('已注册为 Obsidian vault 并打开，首次会弹信任提示请确认')
          } else if (regStatus === 'running') {
            ElMessage.warning('Obsidian 正在运行，请关闭所有 Obsidian 窗口后重试')
          } else if (regStatus === 'not-installed') {
            ElMessage.error('未检测到 Obsidian，请在【设置 -> 通用 -> 外部应用】中配置')
          } else {
            ElMessage.error('自动注册失败，请重试或手动添加')
          }
        } catch {
          // 二次确认取消，不处理
        }
      } catch (action) {
        if (action === 'cancel') {
          // 「打开仓库管理器」：复制路径 + 提示 + 跳转 choose-vault
          const copied = await CopyObsidianVaultPath(path)
          if (copied) ElMessage.success('已复制目录路径到剪贴板')
          else ElMessage.warning('复制路径失败')
          const opened = await OpenObsidianVaultManager()
          if (opened) ElMessage.info('已打开 Obsidian 仓库管理器，请将该目录添加为 vault')
          else ElMessage.error('打开 Obsidian 仓库管理器失败')
        }
        // action === 'close' -> 关闭（X），不处理
      }
    }
  } catch (error) {
    ElMessage.error('打开 Obsidian 失败: ' + (error.message || String(error)))
  }
}

const handleOpenWithDefaultApp = async (path) => {
  try {
    const result = await OpenWithDefaultApp(path)
    if (!result) {
      ElMessage.error('打开文件失败')
    }
  } catch (error) {
    ElMessage.error('打开文件失败: ' + (error.message || String(error)))
  }
}

// ---- 批量拉取 ----
const handleBatchPull = (data) => {
  emit('batchPull', data)
}

// ---- 添加到收藏 ----
const handleAddFavorite = async (node) => {
  const err = await addFavorite(node.path, '', '默认')
  if (err) {
    ElMessage.warning(err)
  } else {
    ElMessage.success('已添加到收藏')
  }
}

// ---- 取消收藏 ----
const handleRemoveFavorite = async (node) => {
  const err = await removeFavorite(node.path)
  if (err) {
    ElMessage.warning(err)
  } else {
    ElMessage.success('已取消收藏')
  }
}

// ---- 添加为工作目录 ----
const handleAddAsWorkDir = (node) => {
  emit('add-work-dir', { path: node.path, name: node.name })
}

// ---- 树状态记忆 ----
function getExpandedPaths() {
  const tree = fileTreeRef.value
  if (!tree) return []
  const store = tree.store
  const paths = []
  function walk(node) {
    if (node.expanded && node.data && node.data.path) {
      paths.push(node.data.path)
    }
    if (node.childNodes) {
      node.childNodes.forEach(walk)
    }
  }
  walk(store.root)
  return paths
}

function saveCurrentState(dirPath) {
  const treeEl = document.querySelector('.tree-content')
  saveState(dirPath, {
    expandedPaths: getExpandedPaths(),
    scrollTop: treeEl ? treeEl.scrollTop : 0,
    selectedPath: null
  })
}

async function restoreTreeState(dirPath) {
  const state = restoreState(dirPath)
  if (state.expandedPaths.length === 0) return

  const tree = fileTreeRef.value
  if (!tree) return

  await waitUntil(() => tree.store.root.childNodes.length > 0, 3000)

  const depthGroups = new Map()
  for (const path of state.expandedPaths) {
    const depth = path.split(/[\\/]/).length
    if (!depthGroups.has(depth)) depthGroups.set(depth, [])
    depthGroups.get(depth).push(path)
  }

  const sortedDepths = [...depthGroups.keys()].sort((a, b) => a - b)

  for (const depth of sortedDepths) {
    const paths = depthGroups.get(depth)
    const pending = []

    for (const path of paths) {
      const node = tree.getNode(path)
      if (!node || node.expanded) continue
      node.expand()
      if (!node.loaded) pending.push(waitForNodeLoaded(node, 2000))
    }

    if (pending.length > 0) await Promise.all(pending)
  }

  if (state.scrollTop > 0) {
    await nextTick()
    const treeEl = document.querySelector('.tree-content')
    if (treeEl) treeEl.scrollTop = state.scrollTop
  }
}

function waitUntil(condition, timeout = 2000) {
  return new Promise(resolve => {
    if (condition()) { resolve(); return }
    const start = Date.now()
    const check = () => {
      if (condition() || Date.now() - start > timeout) {
        resolve()
      } else {
        setTimeout(check, 16)
      }
    }
    check()
  })
}

function waitForNodeLoaded(node, timeout = 2000) {
  return new Promise(resolve => {
    if (node.loaded) { resolve(); return }
    const start = Date.now()
    const check = () => {
      if (node.loaded || Date.now() - start > timeout) {
        resolve()
      } else {
        setTimeout(check, 16)
      }
    }
    check()
  })
}

async function locateNode(targetPath) {
  await treeReadyPromise
  const tree = fileTreeRef.value
  if (!tree) return

  const dir = props.directories.find(d => d.id === props.selectedDirId)
  if (!dir) return

  const rootPath = dir.path
  const sep = rootPath.includes('\\') ? '\\' : '/'
  const normalizedTarget = sep === '\\' ? targetPath.replace(/\//g, '\\') : targetPath.replace(/\\/g, '/')
  const normalizedRoot = sep === '\\' ? rootPath.replace(/\//g, '\\') : rootPath.replace(/\\/g, '/')

  if (!normalizedTarget.startsWith(normalizedRoot)) return

  const relative = normalizedTarget.slice(normalizedRoot.length).replace(/^[\\/]/, '')
  if (!relative) return

  const segments = relative.split(/[\\/]/)
  let currentPath = normalizedRoot

  for (let i = 0; i < segments.length; i++) {
    currentPath += sep + segments[i]
    const node = tree.getNode(currentPath)
    if (!node) break

    if (!node.expanded && !node.isLeaf) {
      node.expand()
      if (!node.loaded) {
        await waitForNodeLoaded(node, 3000)
      }
      await nextTick()
    }
  }

  await nextTick()
  const finalNode = tree.getNode(normalizedTarget) || tree.getNode(targetPath)
  if (finalNode) {
    tree.setCurrentKey(finalNode.data.path)
    await new Promise(r => setTimeout(r, 50))
    const treeContainer = document.querySelector('.tree-content')
    const nodeEl = treeContainer?.querySelector('.el-tree-node.is-current')
    if (nodeEl && treeContainer) {
      const containerRect = treeContainer.getBoundingClientRect()
      const nodeRect = nodeEl.getBoundingClientRect()
      const offset = nodeRect.top - containerRect.top - containerRect.height / 2
      treeContainer.scrollBy({ top: offset, behavior: 'smooth' })
    }
  }
}

// ---- 键盘快捷键入口：作用于当前高亮节点 ----
const triggerRenameCurrent = () => {
  const node = fileTreeRef.value?.getCurrentNode()
  if (node) showRenameAt(node)
}

const triggerDeleteCurrent = () => {
  const node = fileTreeRef.value?.getCurrentNode()
  if (node) handleDeleteAt(node)
}

// ---- 暴露方法 ----
defineExpose({
  refreshNode,
  expandAll,
  collapseAll,
  showRenameAt,
  showCreateAt,
  handleDeleteAt,
  showCopyToDialog,
  triggerRenameCurrent,
  triggerDeleteCurrent,
  setCopyToLoading: (val) => { copyToLoading.value = val },
  closeCopyToDialog: () => { copyToDialogVisible.value = false },
  closeMenu: () => { contextMenu.visible = false },
  saveCurrentState,
  restoreTreeState,
  locateNode
})

// ---- 生命周期 ----
onMounted(() => {
  document.addEventListener('mousedown', onGlobalClick)
  document.addEventListener('contextmenu', onGlobalContextMenu)
  loadFavorites()
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onGlobalClick)
  document.removeEventListener('contextmenu', onGlobalContextMenu)
})
</script>

<style scoped>
.file-tree-aside {
  display: flex;
  flex-direction: column;
  height: 100%;
  border-right: 1px solid var(--border-color);
  background-color: var(--bg-primary);
  overflow: hidden;
}

.tree-toolbar {
  flex-shrink: 0;
  padding: var(--spacing-md) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
}

.tree-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.file-tree {
  background: transparent;
}

.el-tree-node__content {
  transition: all var(--transition-normal);
  border-radius: var(--radius-sm);
  margin: var(--spacing-sm) var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
}
.el-tree-node__content:hover {
  background-color: var(--bg-tertiary) !important;
  box-shadow: var(--shadow-sm);
}
.is-current > .el-tree-node__content {
  background-color: rgba(64, 158, 255, 0.1) !important;
  font-weight: 500;
  box-shadow: var(--shadow-md);
  border-left: 3px solid var(--primary-color);
}
.custom-tree-node {
  flex: 1;
  display: flex;
  align-items: center;
  font-size: 13px;
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

/* 文件树节点 git 仓库标记 img（替代原 SuccessFilled，保留 margin-left:5px 对齐） */
.tree-node-git-img {
  width: 14px;
  height: 14px;
  margin-left: 5px;
  vertical-align: middle;
  object-fit: contain;
}

/* 文件树节点文件类型图标（按后缀映射，与 Document/el-icon 视觉对齐） */
.tree-node-file-icon {
  width: 14px;
  height: 14px;
  margin-right: 5px;
  vertical-align: middle;
  object-fit: contain;
}

/* 互换按钮行 */
.swap-row {
  display: flex;
  justify-content: center;
  margin: -8px 0 0;
}

.swap-row .el-button {
  color: var(--text-tertiary, #909399);
  font-size: 12px;
}

.swap-row .el-button:hover {
  color: var(--primary-color, #409eff);
}

.swap-row .el-button:hover .swap-icon {
  transform: rotate(180deg);
}

.swap-icon {
  transition: transform 0.3s ease;
}

</style>
