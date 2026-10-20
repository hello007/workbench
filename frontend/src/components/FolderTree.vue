<!-- frontend/src/components/FolderTree.vue -->
<!-- 素材面板文件夹树（PRD「素材管理：文件夹树与拖拽整理」PR1/PR2）：库根目录真实
     子目录树展示 + 节点管理菜单（新增子目录/重命名/删除）。受控组件——树数据/
     徽标计数/选中态/操作在途态均由宿主持有，本组件承载展示、选中上报与管理意图
     上报（创建/重命名弹层在组件内闭环收集输入，磁盘操作与结果 toast 在宿主单点，
     对齐「弹窗收集 + 宿主分派」惯例）。
     根节点固定「全部素材」（folderPath=''，PRD R2），菜单仅提供「新增子目录」；
     节点徽标 = 递归 PPT 数（宿主经 buildFolderCountMap 由索引底册前缀聚合后传入）；
     排除规则命中（Excluded）与拆分派生容器（Derived）节点禁管理操作（菜单禁用 +
     tooltip 说明，后端守卫为第二防线）。
     展开态保留（PR2）：expandedKeys 随节点展开/折叠维护，经 default-expanded-keys
     在树数据刷新（局部 reload）后复原用户展开面；首次加载全展开（对齐 PR1
     default-expand-all 初览语义），库切换清空重建。
     选中节点经 select 事件上报 folderPath（空串 = 根「全部素材」），过滤逻辑在
     宿主（消费 utils/assetFolder 前缀匹配单点）。拖拽移动入口为 PR3/PR4 范围。 -->
<template>
  <div class="folder-tree">
    <el-tree
      ref="treeRef"
      :data="treeData"
      node-key="path"
      :props="treeProps"
      :expand-on-click-node="false"
      :default-expanded-keys="expandedKeys"
      highlight-current
      :current-node-key="selectedPath"
      @node-click="onNodeClick"
      @node-expand="onNodeExpand"
      @node-collapse="onNodeCollapse"
    >
      <!-- scope 守卫：未注册/未桩 el-tree 的测试环境插槽以无参形态渲染，直接解构
           { data } 会炸渲染链（对齐 AssetTagTreeSelect/AssetTagManagerDialog 先例，
           docs/spec/assets-management.md §9.4） -->
      <template #default="scope">
        <span
          v-if="scope && scope.data"
          class="folder-node"
          :class="{ 'is-root': scope.data.path === '', 'is-excluded': scope.data.excluded, 'is-derived': scope.data.derived, 'is-drop-target': dragOverPath === scope.data.path }"
          :draggable="canDragFolder(scope.data)"
          @dragstart="onNodeDragStart(scope.data, $event)"
          @dragover.prevent="onNodeDragOver(scope.data, $event)"
          @dragleave="onNodeDragLeave(scope.data)"
          @drop.prevent="onNodeDrop(scope.data, $event)"
        >
          <el-icon class="folder-node-icon"><Folder /></el-icon>
          <span class="folder-node-name" :title="scope.data.path === '' ? '全部素材' : scope.data.path">{{ scope.data.name }}</span>
          <span v-if="badgeOf(scope.data) > 0" class="folder-node-badge">{{ badgeOf(scope.data) }}</span>
          <span
            v-if="scope.data.excluded"
            class="folder-node-excluded"
            title="命中索引排除规则，该目录下文件不参与检索"
          >已排除</span>
          <span
            v-else-if="scope.data.derived"
            class="folder-node-excluded"
            title="PPT 拆分派生产物目录，由拆分链路管理"
          >派生</span>
          <!-- 节点管理菜单（PR2）：hover 显现的「⋯」触发下拉；排除/派生/根（除新增）
               禁管理，禁用项 title 说明原因 -->
          <el-dropdown
            trigger="click"
            placement="bottom-end"
            @visible-change="(visible) => (menuOpenPath = visible ? scope.data.path : '')"
            @command="(cmd) => onNodeCommand(cmd, scope.data)"
          >
            <button
              type="button"
              class="folder-node-more"
              :class="{ 'is-visible': menuOpenPath === scope.data.path }"
              :disabled="busy"
              :aria-label="`${scope.data.name} 管理菜单`"
              @click.stop
            >
              <el-icon :size="12"><MoreFilled /></el-icon>
            </button>
            <template #dropdown>
              <el-dropdown-menu class="folder-node-menu">
                <el-dropdown-item command="create" :disabled="!canCreateChild(scope.data)" :title="manageDisabledTip(scope.data, 'create')">新增子目录</el-dropdown-item>
                <el-dropdown-item command="rename" :disabled="!canRename(scope.data)" :title="manageDisabledTip(scope.data, 'rename')">重命名</el-dropdown-item>
                <el-dropdown-item command="annotate" :disabled="!canAnnotate(scope.data)" :title="manageDisabledTip(scope.data, 'annotate')">标注</el-dropdown-item>
                <el-dropdown-item command="delete" :divided="true" :disabled="!canDelete(scope.data)" :title="manageDisabledTip(scope.data, 'delete')">删除</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </span>
      </template>
    </el-tree>
    <!-- 空态：库根目录下暂无子目录（树仅展示根节点）或数据加载中 -->
    <div v-if="loading" class="folder-tree-hint">加载中…</div>
    <div v-else-if="nodes.length === 0" class="folder-tree-hint">库根目录下暂无子文件夹</div>

    <!-- 新建子目录弹层（PR2 R3）：小弹窗 + 单输入（项目惯例对齐 DirectoryTree
         重命名弹窗 420px 档）；提交分派宿主，弹层即时关闭（错误经宿主 toast 透传） -->
    <el-dialog
      v-model="createDialogVisible"
      title="新建子目录"
      width="420px"
      append-to-body
      @closed="createDialogName = ''"
    >
      <el-input
        v-model="createDialogName"
        placeholder="输入文件夹名称"
        maxlength="255"
        @keyup.enter="confirmCreate"
      />
      <template #footer>
        <el-button size="small" @click="createDialogVisible = false">取消</el-button>
        <el-button size="small" type="primary" :disabled="busy || !createDialogName.trim()" @click="confirmCreate">创建</el-button>
      </template>
    </el-dialog>

    <!-- 重命名弹层（PR2 R3）：同款 420px 小弹窗 -->
    <el-dialog
      v-model="renameDialogVisible"
      title="重命名文件夹"
      width="420px"
      append-to-body
      @closed="renameDialogPath = ''"
    >
      <el-input
        v-model="renameDialogName"
        placeholder="输入新名称"
        maxlength="255"
        @keyup.enter="confirmRename"
      />
      <template #footer>
        <el-button size="small" @click="renameDialogVisible = false">取消</el-button>
        <el-button size="small" type="primary" :disabled="busy || !renameDialogName.trim()" @click="confirmRename">重命名</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Folder, MoreFilled } from '@element-plus/icons-vue'
// 拖拽数据载荷单点（dragstart/drop 两端唯一实现，MIME 与解析禁第二处手拼）
import { parseAssetDragPayload, parseFolderDragPayload, buildFolderDragPayload, ASSET_FOLDER_DRAG_MIME } from '../utils/assetFolder'

const props = defineProps({
  // 树数据（ListAssetFolders 返回的 model.AssetFolderNode 平面结构数组，仅库根
  // 一级子目录为根；根「全部素材」节点由本组件固定前置，不经数据源下发）
  nodes: { type: Array, default: () => [] },
  // 徽标递归计数映射（utils/assetFolder buildFolderCountMap 产出，键 = folderPath，
  // 根为 ''；空缺键按 0 处理——不显示徽标）
  countMap: { type: Object, default: () => ({}) },
  // 宿主持有的当前选中 folderPath（受控：'' = 根「全部素材」）
  selectedPath: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  // 管理操作/移动在途（宿主持有）：禁菜单触发与弹层提交，防并发操作叠加
  busy: { type: Boolean, default: false },
  // 文件拖拽移动 drop 目标开关（PRD R4）：true 时树节点接收 HTML5 DnD（宿主
  // 在移动 Promise 期间经 busy 禁再次分派）
  dropEnabled: { type: Boolean, default: false }
})

const emit = defineEmits(['select', 'create', 'rename', 'delete', 'annotate', 'drop-files', 'drop-folder'])

const treeRef = ref(null)

// el-tree 数据契约：根节点固定前置（path='' 与排除/派生态不可能冲突——后端树不含根）
const treeData = computed(() => [
  {
    path: '',
    name: '全部素材',
    children: props.nodes,
    excluded: false,
    derived: false,
    fileCount: 0
  }
])

const treeProps = { label: 'name', children: 'children' }

const badgeOf = (data) => props.countMap?.[data.path] || 0

// ---- 展开态保留（PR2）：expandedKeys 随展开/折叠事件维护，default-expanded-keys
// 在树数据刷新后复原；首次加载全展开（收集全部路径），库切换（数据清空）复位
const expandedKeys = ref([''])
let treeEverLoaded = false
const collectAllPaths = (nodes, acc = []) => {
  for (const node of nodes || []) {
    acc.push(node.path)
    collectAllPaths(node.children, acc)
  }
  return acc
}
watch(() => props.nodes, (nodes, prev) => {
  if (!nodes || nodes.length === 0) {
    // 库切换/刷新清空：展开态整体复位（新库重新首展全展开）
    if (prev && prev.length > 0) {
      expandedKeys.value = ['']
      treeEverLoaded = false
    }
    return
  }
  if (!treeEverLoaded) {
    expandedKeys.value = [''].concat(collectAllPaths(nodes))
    treeEverLoaded = true
  }
}, { immediate: true })

const onNodeExpand = (data) => {
  if (!expandedKeys.value.includes(data.path)) {
    expandedKeys.value = expandedKeys.value.concat(data.path)
  }
}
const onNodeCollapse = (data) => {
  expandedKeys.value = expandedKeys.value.filter(k => k !== data.path)
}

const onNodeClick = (data) => {
  emit('select', data.path || '')
}

// 菜单展开中的节点路径（触发按钮保持显现，防 hover 离开即消失）
const menuOpenPath = ref('')

// ---- 节点管理菜单（PR2）----
// 管理资格单点（分操作）：根节点（path=''）仅新增可用；排除命中目录禁新增/删除
// ——重命名刻意放行（重命名出排除名是 excluded 文件恢复索引可见的唯一目录级
// 出口，后端按受影响文件重过 matcher 双向翻转收敛）；拆分派生容器全禁（由拆分
// 链路管理）。后端守卫为第二防线，菜单禁用为第一防线
const canCreateChild = (data) => !data.excluded && !data.derived
const canRename = (data) => data.path !== '' && !data.derived
const canDelete = (data) => data.path !== '' && !data.excluded && !data.derived
// 标注资格（PRD R8/PR4）：与禁管理同口径——库根/排除命中/派生容器禁标注
const canAnnotate = (data) => data.path !== '' && !data.excluded && !data.derived
const manageDisabledTip = (data, cmd) => {
  if (data.path === '') return '根节点不可重命名、标注或删除'
  if (data.derived) return 'PPT 拆分派生产物目录由拆分链路管理，禁用管理操作'
  if (data.excluded) {
    if (cmd === 'rename') return ''
    return '命中索引排除规则的目录禁用此操作（可在「索引设置」中调整规则，或重命名/移出排除名恢复）'
  }
  return ''
}

const onNodeCommand = (cmd, data) => {
  if (cmd === 'create') {
    if (!canCreateChild(data)) return
    createDialogPath.value = data.path || ''
    createDialogName.value = ''
    createDialogVisible.value = true
    return
  }
  if (cmd === 'rename') {
    if (!canRename(data)) return
    renameDialogPath.value = data.path
    renameDialogName.value = data.name
    renameDialogVisible.value = true
    return
  }
  if (cmd === 'annotate') {
    if (!canAnnotate(data)) return
    emit('annotate', data)
    return
  }
  if (cmd === 'delete') {
    if (!canDelete(data)) return
    emit('delete', data)
  }
}

// ---- 拖拽移动 drop 目标（PRD R4 文件 / R5 目录，PR3/PR4）----
// 树节点即 drop 落点（HTML5 DnD，不经 el-tree 内建节点拖拽 API——那是树节点
// 排序语义）；根「全部素材」= 库根为合法落点，排除命中/派生容器在 drop 事件内
// 拒绝（ElMessage warning，后端守卫为第二防线）。拖拽悬停高亮经 dragOverPath。
// 双载荷分派：目录载荷（ASSET_FOLDER_DRAG_MIME）优先 → drop-folder；文件载荷 →
// drop-files；两者皆无 = 非本面板来源静默忽略。
const dragOverPath = ref('')

// 目录节点可拖性（PRD R5）：根不可拖（无磁盘对应物）；派生容器不可拖（由拆分
// 链路管理）；排除命中目录可拖（移出排除名是恢复索引可见的出口，后端双向翻转
// 收敛——与重命名放行同口径）
const canDragFolder = (data) => data.path !== '' && !data.derived

const onNodeDragStart = (data, event) => {
  if (!canDragFolder(data) || !event?.dataTransfer) return
  event.dataTransfer.setData(ASSET_FOLDER_DRAG_MIME, buildFolderDragPayload(data.path))
  event.dataTransfer.effectAllowed = 'move'
}

const onNodeDragOver = (data, event) => {
  if (!props.dropEnabled || props.busy) return
  event.dataTransfer.dropEffect = 'move'
  dragOverPath.value = data.path || ''
}

const onNodeDragLeave = (data) => {
  if (dragOverPath.value === (data.path || '')) dragOverPath.value = ''
}

const onNodeDrop = (data, event) => {
  const path = data.path || ''
  if (dragOverPath.value === path) dragOverPath.value = ''
  if (!props.dropEnabled || props.busy) return
  // 禁落点：排除命中（禁管理同口径）与派生容器（产物由拆分链路管理）；后端
  // MoveAssetFiles/MoveAssetFolder 目标校验为第二防线，此处拒绝不让请求出网
  if (data.excluded || data.derived) {
    ElMessage.warning(data.derived
      ? 'PPT 拆分派生产物目录不能作为移动目标'
      : '命中索引排除规则的目录不能作为移动目标')
    return
  }
  // 目录载荷优先（PRD R5 目录 onto 目录）
  const sourcePath = parseFolderDragPayload(event.dataTransfer)
  if (sourcePath) {
    if (sourcePath === path || path.startsWith(sourcePath + '/')) {
      // 前端侧软拦截：目标不得位于源目录自身子树内 / 源目标相同（后端核心同款校验）
      ElMessage.warning(sourcePath === path ? '不能移动到自身' : '目标目录位于源目录内部，不能移动到自身子树')
      return
    }
    emit('drop-folder', sourcePath, path)
    return
  }
  const fileIds = parseAssetDragPayload(event.dataTransfer)
  if (fileIds.length === 0) return // 非本面板来源的拖拽：静默忽略
  emit('drop-files', path, fileIds)
}

// ---- 新建/重命名弹层（PR2）：弹层内收集输入，提交经事件分派宿主执行 ----
const createDialogVisible = ref(false)
const createDialogPath = ref('')
const createDialogName = ref('')
const renameDialogVisible = ref(false)
const renameDialogPath = ref('')
const renameDialogName = ref('')

const confirmCreate = () => {
  const name = createDialogName.value.trim()
  if (!name || props.busy) return
  emit('create', createDialogPath.value, name)
  createDialogVisible.value = false
}
const confirmRename = () => {
  const name = renameDialogName.value.trim()
  if (!name || props.busy) return
  emit('rename', renameDialogPath.value, name)
  renameDialogVisible.value = false
}

defineExpose({ treeRef })
</script>

<style scoped>
.folder-tree {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-sm);
}

.folder-tree :deep(.el-tree-node__content) {
  height: 28px;
  border-radius: var(--radius-sm);
}

.folder-node {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  min-width: 0;
  flex: 1;
  font-size: 13px;
  color: var(--text-primary);
}

.folder-node.is-root .folder-node-name {
  font-weight: 600;
  letter-spacing: -0.01em;
}

.folder-node.is-excluded,
.folder-node.is-derived {
  opacity: 0.55;
}

.folder-node-icon {
  flex-shrink: 0;
  color: var(--text-tertiary);
}

.folder-node-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.folder-node-badge {
  flex-shrink: 0;
  font-size: 11px;
  line-height: 1;
  padding: 3px 6px;
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.folder-node-excluded {
  flex-shrink: 0;
  font-size: 10px;
  line-height: 1;
  padding: 3px 4px;
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  color: var(--warning-color);
}

/* 节点管理菜单触发按钮（PR2）：hover 节点或菜单展开时显现，禁 busy 期间触发 */
.folder-node-more {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  margin-left: auto;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  opacity: 0;
  transition: opacity var(--transition-fast), background var(--transition-fast), color var(--transition-fast);
}

.folder-node:hover .folder-node-more,
.folder-node-more.is-visible,
.folder-node-more:focus-visible {
  opacity: 1;
}

.folder-node-more:hover {
  background: var(--bg-tertiary);
  color: var(--primary-color);
}

/* 拖拽悬停高亮（PRD R4）：drop 落点主色弱化底 + 主色边框（对齐卡片选中态视觉） */
.folder-node.is-drop-target {
  background: var(--primary-bg);
  outline: 1px solid var(--primary-light);
  border-radius: var(--radius-sm);
}

.folder-tree-hint {
  padding: var(--spacing-md);
  font-size: 12px;
  color: var(--text-placeholder);
  text-align: center;
}
</style>
