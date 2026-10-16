<!-- frontend/src/components/AssetTagManagerDialog.vue -->
<!-- 素材标签管理弹窗（素材管理第六批，SRS §2.4 / PRD D6/D7 + D8#1 拖拽排序增量）：
     树形展示全部标签（色点 + 名称 + 直接绑定计数 + 操作列），承载标签 CRUD 统一
     入口——新增（根/子）、编辑（重命名/改色/移动父级）、删除（级联删子树 + 解除
     绑定）、拖拽排序（同级排序 + 跨父移动二合一）。
     契约：
     - 数据源 = ListAssetTags(libraryId)（打开即拉，含未使用标签与直接绑定计数），
       CRUD 成功后重拉（树与计数即时刷新）并 emit('changed') 通知宿主执行刷新链
       （底册/检索/树组件重拉由宿主编排，本弹窗不越权）；
     - 删除影响面：确认弹窗「将删除该标签及其 N 个子标签，并解除 M 处绑定」——
       N = 本地树子树后代数（collectSubtreeIds），M = 子树直接绑定计数合计
       （sumSubtreeCounts，删除前快照）；删除成功的 success 提示以后端返回值为
       权威绑定行数（确认时点与执行时点可能并发变更，以后端为准）；
     - 防环双层：父级选项编辑态排除自身与子树（collectSubtreeIds 预拦截，本组件）
       + 后端 updateTagFull/moveTag 沿 parent 链校验（主防线）；重名/防环等后端
       拒绝为普通 error 形态（PR1 参数校验口径，无域错误码），经 handleError
       统一分流；
     - 颜色留空提交空串由后端按标签名分配默认色板（与自动创建标签同源）。
     拖拽排序契约（D8#1）：el-tree draggable，before/after 落点 = 同级排序、
     inner 落点 = 变子级（组尾追加），落点换算与平铺乐观更新收敛
     utils/assetTagTree（resolveTagMoveDrop/applyTagMoveToFlat 单点）；乐观更新
     成功即 emit('changed')（走宿主 refreshAfterTagChange 链），失败回滚 + 重拉
     保权威；拖拽入自身子树被 allowDrop 预拦截（后端 moveTag 防环为主防线）；
     表单展开/保存中/上一次落位在途时暂停拖拽（防编辑态与乐观序冲突）。
     视觉：弹窗尺寸 720 档（内容中等量级，对齐 frontend-visual-conventions 弹窗
     尺寸分档），树区 max-height min(480px, 55vh) 内滚，节点行 hover 高亮，色点
     用标签色数据动态绑定。 -->
<template>
  <el-dialog
    v-model="visible"
    title="标签管理"
    width="min(720px, 86vw)"
    top="12vh"
    :close-on-click-modal="false"
    append-to-body
  >
    <div class="tag-manager-body">
      <!-- 新增/编辑表单区（操作触发时展开，同一表单双模式复用） -->
      <div v-if="formVisible" class="tag-form">
        <span class="tag-form-title">{{ form.mode === 'edit' ? '编辑标签' : '新增标签' }}</span>
        <el-form label-width="64px" class="tag-form-fields">
          <el-form-item label="名称">
            <el-input
              v-model="form.name"
              placeholder="标签名（库内唯一）"
              :disabled="saving"
              @keyup.enter="submitForm"
            />
          </el-form-item>
          <el-form-item label="颜色">
            <el-color-picker v-model="form.color" :predefine="COLOR_PRESETS" :disabled="saving" />
            <span class="tag-form-hint">留空由系统按标签名分配默认色</span>
          </el-form-item>
          <el-form-item label="父级">
            <el-select
              v-model="form.parentId"
              placeholder="不选择则为根标签"
              :disabled="saving"
              clearable
            >
              <el-option v-for="opt in parentOptions" :key="opt.id" :label="opt.name" :value="opt.id">
                <span class="tag-option-row">
                  <span class="tag-color-dot" :style="{ background: opt.color || 'transparent' }"></span>
                  <!-- 深度缩进为树结构运行时数据，动态绑定是唯一呈现方式，非静态行内样式 -->
                  <span :style="{ paddingLeft: opt.depth * 12 + 'px' }">{{ opt.name }}</span>
                </span>
              </el-option>
            </el-select>
          </el-form-item>
        </el-form>
        <div class="tag-form-actions">
          <el-button size="small" :disabled="saving" @click="closeForm">取消</el-button>
          <el-button size="small" type="primary" :loading="saving" @click="submitForm">保存</el-button>
        </div>
      </div>

      <!-- 标签树展示区：节点行 = 色点 + 名称 + 直接绑定计数 + 操作列；拖拽排序
           （D8#1）：同级上下 = 排序、拖入节点 = 变子级 -->
      <div class="tag-tree-head">
        <span class="tag-tree-title">标签树</span>
        <span class="tag-tree-hint">拖拽节点可排序或调整父级</span>
        <el-button size="small" text type="primary" @click="openCreateRoot">新增根标签</el-button>
      </div>
      <div class="tag-tree-area">
        <el-tree
          v-if="treeData.length > 0"
          :data="treeData"
          node-key="id"
          default-expand-all
          :expand-on-click-node="false"
          :draggable="dragEnabled"
          :allow-drop="onAllowDrop"
          @node-drop="onNodeDrop"
        >
          <template #default="scope">
            <!-- scope 守卫：未注册/未桩 el-tree-select 时插槽以无参形态渲染，解构
                 undefined 会炸渲染链（对齐 AssetTagTreeSelect 同款守卫） -->
            <div v-if="scope && scope.data" class="tag-tree-node">
              <span class="tag-color-dot" :style="{ background: scope.data.color || 'transparent' }"></span>
              <span class="tag-tree-name" :title="scope.data.name">{{ scope.data.name }}</span>
              <span v-if="scope.data.count > 0" class="tag-tree-count">{{ scope.data.count }}</span>
              <span class="tag-tree-actions">
                <el-button size="small" text type="primary" @click.stop="openCreateChild(scope.data)">新增子标签</el-button>
                <el-button size="small" text type="primary" @click.stop="openEdit(scope.data)">编辑</el-button>
                <el-button size="small" text type="danger" @click.stop="removeTag(scope.data)">删除</el-button>
              </span>
            </div>
          </template>
        </el-tree>
        <el-empty v-else-if="!loadingTags" description="暂无标签，点击「新增根标签」创建">
          <el-button type="primary" @click="openCreateRoot">新增根标签</el-button>
        </el-empty>
        <div v-else class="tag-tree-loading">加载中…</div>
      </div>
    </div>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  CreateAssetTag,
  DeleteAssetTag,
  ListAssetTags,
  MoveAssetTag,
  UpdateAssetTag
} from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import {
  applyTagMoveToFlat,
  buildTagTree,
  collectSubtreeIds,
  resolveTagMoveDrop,
  sumSubtreeCounts
} from '../utils/assetTagTree'

// 预置色板：与后端默认色板（service/assets_db.go assetTagPalette）取值一致，
// 保持「快捷选色」与标签自动分配色系同源
const COLOR_PRESETS = ['#2563eb', '#059669', '#d97706', '#dc2626', '#7c3aed', '#0891b2']

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  libraryId: { type: String, default: '' },
  // 打开时预填新增根标签表单（详情编辑「+ 新建标签」快捷入口，PRD D6）
  presetCreate: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'changed'])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const tags = ref([])
const loadingTags = ref(false)
const saving = ref(false)
const formVisible = ref(false)
const form = ref({ mode: 'create', id: 0, name: '', color: '', parentId: 0 })
// 拖拽落位在途标记：乐观更新写回前防并发拖拽叠加（在途时暂停 draggable）
const moveBusy = ref(false)

const treeData = computed(() => buildTagTree(tags.value))

// 拖拽开关：表单展开/保存中/上一次落位在途时暂停（防编辑态与乐观序冲突）
const dragEnabled = computed(() => !formVisible.value && !saving.value && !moveBusy.value)

// ---- 拖拽排序（D8#1）：落点换算 + 乐观更新 + 失败回滚重拉 ----

// allowDrop 预拦截：目标在拖拽节点自身子树内一律拒绝（inner 拖入后代 / before、
// after 挂到后代旁都会成环）——后端 moveTag 沿 parent 链校验为主防线，本守卫
// 是拖拽体验层拦截。before/after 落点经 resolveTagMoveDrop 换算目标父（与
// node-drop 同一换算单点，不依赖 el-tree 节点对象上的 parentId 形态）。
const onAllowDrop = (draggingNode, dropNode, dropType) => {
  const subtree = new Set(collectSubtreeIds(tags.value, draggingNode.data.id))
  if (dropType === 'inner') return !subtree.has(dropNode.data.id)
  const move = resolveTagMoveDrop(treeData.value, draggingNode.data.id, dropNode.data.id, dropType)
  return !!move && !subtree.has(move.newParentID)
}

const onNodeDrop = async (draggingNode, dropNode, dropType) => {
  const tagId = draggingNode.data.id
  const move = resolveTagMoveDrop(treeData.value, tagId, dropNode.data.id, dropType)
  if (!move || moveBusy.value) return
  const prev = tags.value
  // 乐观更新：树立即呈现落位结果（后端成功即无需等待重拉）
  tags.value = applyTagMoveToFlat(tags.value, tagId, move.newParentID, move.index)
  moveBusy.value = true
  try {
    await MoveAssetTag(props.libraryId, tagId, move.newParentID, move.index)
    // 排序变更 emit changed 走既有 refreshAfterTagChange 链（树组件下拉展开
    // 重拉已天然覆盖，宿主底册/检索不受纯排序影响但链路统一触发无害）
    emit('changed')
  } catch (error) {
    // 失败回滚乐观态 + 重拉保权威（并发变更下以服务端为准）
    tags.value = prev
    handleError('调整标签顺序失败: ', error)
    await loadTags()
  } finally {
    moveBusy.value = false
  }
}

const loadTags = async () => {
  if (!props.libraryId) {
    tags.value = []
    return
  }
  loadingTags.value = true
  try {
    tags.value = (await ListAssetTags(props.libraryId)) || []
  } catch (error) {
    tags.value = []
    handleError('加载标签列表失败: ', error)
  } finally {
    loadingTags.value = false
  }
}

// ---- 表单 ----

const openCreateRoot = () => {
  form.value = { mode: 'create', id: 0, name: '', color: '', parentId: 0 }
  formVisible.value = true
}

const openCreateChild = (node) => {
  form.value = { mode: 'create', id: 0, name: '', color: '', parentId: node.id }
  formVisible.value = true
}

const openEdit = (node) => {
  form.value = {
    mode: 'edit',
    id: node.id,
    name: node.name,
    color: node.color || '',
    parentId: node.parentId || 0
  }
  formVisible.value = true
}

const closeForm = () => {
  formVisible.value = false
}

// 打开即拉取（含 presetCreate 预填新增表单）；关闭复位表单态防下次打开闪旧值。
// immediate 兜底以 modelValue=true 直接挂载的形态（测试注入 / 宿主保持打开态）。
// 置于表单函数定义之后：immediate 回调同步执行，先于初始化的 const 会触发 TDZ
watch(
  visible,
  (val) => {
    if (!val) {
      formVisible.value = false
      form.value = { mode: 'create', id: 0, name: '', color: '', parentId: 0 }
      return
    }
    if (props.presetCreate) openCreateRoot()
    loadTags()
  },
  { immediate: true }
)

// 父级选项：树序深度缩进展示；编辑态排除自身与子树（防环预拦截，后端
// updateTagFull 为主防线）；新增态全量可选（新建标签无子代不可能成环）
const parentOptions = computed(() => {
  const excluded = form.value.mode === 'edit'
    ? new Set(collectSubtreeIds(tags.value, form.value.id))
    : new Set()
  const options = []
  const walk = (nodes, depth) => {
    for (const node of nodes) {
      options.push({ id: node.id, name: node.name, color: node.color, depth })
      if (node.children.length > 0) walk(node.children, depth + 1)
    }
  }
  walk(treeData.value, 0)
  return options.filter(option => !excluded.has(option.id))
})

const submitForm = async () => {
  const name = form.value.name.trim()
  if (!name) {
    ElMessage.warning('请输入标签名')
    return
  }
  saving.value = true
  const isEdit = form.value.mode === 'edit'
  try {
    if (isEdit) {
      await UpdateAssetTag(props.libraryId, form.value.id, name, form.value.parentId || 0, form.value.color || '')
      ElMessage.success('标签已保存')
    } else {
      await CreateAssetTag(props.libraryId, name, form.value.parentId || 0, form.value.color || '')
      ElMessage.success('标签已创建')
    }
    formVisible.value = false
    await loadTags()
    emit('changed')
  } catch (error) {
    // 重名/防环等后端拒绝为普通 error 形态（PR1 校验口径，无域错误码），统一分流
    handleError(isEdit ? '保存标签失败: ' : '创建标签失败: ', error)
  } finally {
    saving.value = false
  }
}

// ---- 删除（级联删子树 + 解除绑定，PRD D7）----

const removeTag = async (node) => {
  // 影响面前置计算：N = 子树后代数（不含自身）；M = 子树直接绑定计数合计
  //（删除前快照，success 提示以后端返回值为权威口径——见文件头契约）
  const childCount = collectSubtreeIds(tags.value, node.id).length - 1
  const bindingCount = sumSubtreeCounts(tags.value, node.id)
  const scopeText = childCount > 0 ? `及其 ${childCount} 个子标签` : ''
  try {
    await ElMessageBox.confirm(
      `将删除标签「${node.name}」${scopeText}，并解除 ${bindingCount} 处绑定（删除标签不影响素材本身）。`,
      '删除标签',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    const affected = await DeleteAssetTag(props.libraryId, node.id)
    ElMessage.success(`已删除标签${scopeText}，解除 ${affected} 处绑定`)
    await loadTags()
    emit('changed')
  } catch (error) {
    handleError('删除标签失败: ', error)
  }
}
</script>

<style scoped>
.tag-manager-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

/* ---- 表单区 ---- */
.tag-form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  background: var(--bg-tertiary);
  border-radius: var(--radius-md);
}

.tag-form-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.tag-form-fields :deep(.el-form-item) {
  margin-bottom: var(--spacing-sm);
}

.tag-form-hint {
  margin-left: var(--spacing-sm);
  font-size: 12px;
  color: var(--text-tertiary);
}

.tag-form-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
}

/* ---- 树展示区 ---- */
.tag-tree-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
}

.tag-tree-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

/* 拖拽操作提示（D8#1）：弱化形态的 affordance 说明，挤占次要位置不抢操作焦点 */
.tag-tree-hint {
  margin-right: auto;
  font-size: 12px;
  color: var(--text-placeholder);
}

/* 树区内滚：弹窗高度分档的上限约束（内容多大弹窗滚动，对齐弹窗尺寸策略） */
.tag-tree-area {
  max-height: min(480px, 55vh);
  overflow-y: auto;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-xs);
}

.tag-tree-node {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  min-width: 0;
  flex: 1;
  padding-right: var(--spacing-sm);
}

/* 节点行 hover 高亮（D8#1 样式优化）：整行弱底色 + 操作按钮主色强化，层级缩进
   由 el-tree 原生嵌套缩进承担，此处只做行内视觉反馈 */
.tag-tree-area :deep(.el-tree-node__content) {
  border-radius: var(--radius-sm);
  transition: background-color var(--transition-fast);
}

.tag-tree-area :deep(.el-tree-node__content:hover) {
  background: var(--bg-tertiary);
}

.tag-tree-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tag-tree-count {
  flex-shrink: 0;
  font-size: 11px;
  line-height: 1;
  padding: 2px 6px;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.tag-tree-actions {
  flex-shrink: 0;
  margin-left: auto;
}

.tag-tree-loading {
  padding: var(--spacing-md);
  font-size: 12px;
  color: var(--text-tertiary);
  text-align: center;
}

/* 标签色点与选项行（父级下拉）：与 AssetsPanel 既有 .tag-color-dot 同形态，
   弹窗样式独立作用域内敛不复用宿主类 */
.tag-color-dot {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: var(--radius-sm);
}

.tag-option-row {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
}
</style>
