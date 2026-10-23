<!-- frontend/src/components/BranchCompareFileTree.vue -->
<!-- 分支差异文件树（R5）：把扁平 BranchDiffFile[] 按 '/' 分段建目录树；
     状态着色（added 绿 / deleted 红 / renamed·copied 橙 / modified 深橙 / typechange 青 /
     unchanged 灰）+ 状态多选过滤 + 路径子串过滤；rename 显示 old → new 双路径；
     点击文件 emit selectFile。树渲染为自绘递归节点（不走 el-tree，便于单测与树内着色控制）。 -->
<template>
  <div class="bc-file-tree">
    <div class="tree-toolbar">
      <el-input
        v-model="pathKeyword"
        class="input-w-md"
        size="small"
        clearable
        placeholder="按路径过滤"
      />
      <el-select
        v-model="statusFilter"
        class="status-filter"
        size="small"
        multiple
        collapse-tags
        clearable
        placeholder="状态过滤"
      >
        <el-option v-for="s in STATUS_LIST" :key="s.value" :label="s.label" :value="s.value" />
      </el-select>
      <el-checkbox v-model="showUnchanged" size="small">未变更</el-checkbox>
      <span class="file-count">{{ matchedFiles.length }}/{{ files.length }}</span>
    </div>
    <div class="tree-body">
      <div v-if="treeRoots.length === 0" class="tree-empty">无匹配文件</div>
      <FileNode
        v-for="node in treeRoots"
        :key="node.path"
        :node="node"
        :depth="0"
        :selected-path="selectedPath"
        @select-file="onSelectFile"
      />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, reactive, h, defineComponent } from 'vue'
import { CaretRight, CaretBottom, Document, Folder as FolderIcon } from '@element-plus/icons-vue'
import { buildTree, STATUS_LIST } from '../utils/branchFileTree'

defineOptions({ name: 'BranchCompareFileTree' })

const props = defineProps({
  // 扁平文件数组（后端按 path 排序）：{path, oldPath?, status, additions, deletions, binary}
  files: { type: Array, default: () => [] },
  // 当前已选文件路径（高亮）
  selectedPath: { type: String, default: '' }
})

const emit = defineEmits(['selectFile'])

// 过滤态；unchanged 默认隐藏（「未变更」开关或状态过滤显式勾选时显示）
const pathKeyword = ref('')
const statusFilter = ref([])
const showUnchanged = ref(false)

// 过滤后文件集：unchanged 开关 + 状态多选（空 = 不过滤）+ 路径子串（大小写不敏感）
const matchedFiles = computed(() => {
  const kw = pathKeyword.value.trim().toLowerCase()
  const statuses = statusFilter.value
  const allowUnchanged = showUnchanged.value || statuses.includes('unchanged')
  return props.files.filter(f => {
    if (f.status === 'unchanged' && !allowUnchanged) return false
    if (statuses.length > 0 && !statuses.includes(f.status)) return false
    if (kw && !f.path.toLowerCase().includes(kw)) return false
    return true
  })
})

const treeRoots = computed(() => buildTree(matchedFiles.value))

const onSelectFile = (file) => {
  emit('selectFile', file)
}

/**
 * 递归文件节点（函数式组件）：目录行可折叠（展开态局部记忆），文件行点击选中。
 * 用 h() 递归而非独立 SFC，保持单文件交付；着色 class 挂在文件行根节点。
 */
const FileNode = defineComponent({
  name: 'BcFileNode',
  props: {
    node: { type: Object, required: true },
    depth: { type: Number, required: true },
    selectedPath: { type: String, default: '' }
  },
  emits: ['selectFile'],
  setup(nodeProps, { emit: emitFile }) {
    // 展开态按目录路径记忆；必须 reactive——渲染函数闭包内的普通 Map 变更不触发重渲染（点击收放失效根因）
    const expandedMap = reactive(new Map())
    const isExpanded = (path) => expandedMap.get(path) !== false
    const toggle = (path) => expandedMap.set(path, !isExpanded(path))

    const render = (node, depth) => {
      const indent = { paddingLeft: depth * 14 + 8 + 'px' }
      if (node.dir) {
        const expanded = isExpanded(node.path)
        const children = expanded ? node.children.map(child => render(child, depth + 1)) : []
        return h('div', { key: node.path, class: 'tree-node-group' }, [
          h('div', { class: 'tree-node tree-node-dir', style: indent, onClick: () => toggle(node.path) }, [
            h('span', { class: 'tree-caret' }, [
              h(expanded ? CaretBottom : CaretRight, { style: { width: '12px', height: '12px' } })
            ]),
            h(FolderIcon, { style: { width: '14px', height: '14px' } }),
            h('span', { class: 'node-name' }, node.name)
          ]),
          ...children
        ])
      }
      const file = node.file
      // 展示名：优先 file.name，缺省取 path 末段（BranchDiffFile 契约只有 path）
      const displayName = file.name || file.path.split('/').pop()
      return h('div', {
        key: node.path,
        class: ['tree-node', 'tree-node-file', 'status-' + file.status, { 'is-selected': nodeProps.selectedPath === node.path }],
        style: indent,
        onClick: () => emitFile('selectFile', file)
      }, [
        h('span', { class: 'tree-caret' }, [h(Document, { style: { width: '14px', height: '14px' } })]),
        h('span', { class: 'node-name', title: file.oldPath ? file.oldPath + ' → ' + file.path : file.path },
          file.oldPath ? file.oldPath + ' → ' + file.path : displayName),
        file.binary
          ? h('span', { class: 'node-flag' }, '二进制')
          : h('span', { class: 'node-count' }, `+${file.additions || 0} -${file.deletions || 0}`)
      ])
    }
    return () => render(nodeProps.node, nodeProps.depth)
  }
})
</script>

<style scoped>
.bc-file-tree {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm, 8px);
  min-height: 0;
  height: 100%;
}

.tree-toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm, 8px);
}

.input-w-md {
  width: 180px;
}

.status-filter {
  width: 170px;
}

.file-count {
  font-size: 12px;
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.tree-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 4px);
  background: var(--bg-secondary);
  padding: var(--spacing-xs, 4px) 0;
}

.tree-empty {
  padding: 24px;
  text-align: center;
  color: var(--text-tertiary);
  font-size: 13px;
}

.tree-node {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 26px;
  font-size: 13px;
  color: var(--text-primary);
  cursor: default;
  user-select: none;
}

.tree-node-dir {
  color: var(--text-secondary);
  cursor: pointer;
}

.tree-node-file {
  cursor: pointer;
}

.tree-node-file:hover {
  background: var(--primary-bg);
}

.tree-node-file.is-selected {
  background: var(--primary-bg);
}

.tree-caret {
  display: inline-flex;
  align-items: center;
  color: var(--text-tertiary);
}

.node-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-count {
  font-size: 11px;
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}

.node-flag {
  font-size: 11px;
  color: var(--text-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm, 3px);
  padding: 0 4px;
}

/* 状态着色（R5）：语义走设计令牌——added 绿 / deleted 红 / renamed·copied 橙 /
   modified 深橙（warning 与 danger 混合派生）/ typechange 青（info 档）/ unchanged 灰 */
.tree-node-file.status-added .node-name {
  color: var(--success-color);
}

.tree-node-file.status-deleted .node-name {
  color: var(--danger-color);
  text-decoration: line-through;
}

.tree-node-file.status-renamed .node-name,
.tree-node-file.status-copied .node-name {
  color: var(--warning-color);
}

.tree-node-file.status-modified .node-name {
  color: color-mix(in srgb, var(--warning-color) 72%, var(--danger-color));
}

.tree-node-file.status-typechange .node-name {
  color: var(--info-color);
}

.tree-node-file.status-unchanged {
  opacity: 0.55;
}

.tree-node-file.status-unchanged .node-name {
  color: var(--text-tertiary);
}
</style>
