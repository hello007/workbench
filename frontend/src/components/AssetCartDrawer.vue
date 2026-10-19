<!-- frontend/src/components/AssetCartDrawer.vue -->
<!-- 素材购物栏抽屉（素材管理「购物栏」PR3：跨库页级收集 + 拖拽排序 + 按序合并导出）。
     受控组件契约：条目清单（items，List 快照含 stale 探测标志）/ 导出运行态
    （exporting）/ 导出进度（exportProgress）全由宿主持有，本层只上报 close /
     remove / clear / reorder / export / cancel-export / trace 七个事件，分派
     逻辑留宿主单点（对齐 AssetSelectionBar / AssetPreviewOverlay 受控组件先例）。
     拖拽排序：vue-draggable-plus（AiChatPanel/DirectoryTree 同款依赖与用法）——
     localItems 为 v-model 可变副本（拖拽时原地重排），拖完（@end）上报按键列表
     的 reorder 事件，宿主调 ReorderAssetCart 回写后 List 刷新经 props 回流，
     watch 同步副本（数据源恒为宿主快照，副本仅为拖拽操作载体）。
     缩略图：AssetThumb tier=0 懒加载（画廊同口径，禁整库预取）；条目来源行展示
     「库名/文件名/页码」（跨库条目混装，PRD 来源行契约）。
     失效条目（stale，源文件已不存在）：整行置灰 + 「源文件不存在」标记，不自动
     移除（可手动移除，导出跳过并在结果中报告）；置灰行仍可拖拽（顺序语义不因
     失效改变，导出端跳过）。
     导出运行态：导出/清空/逐条移除禁用（导出快照语义下禁改为第一防线，后端
     E_CART_EXPORT_IN_PROGRESS 为兜底）；进度条复用面板扫描进度条视觉。
     视觉对齐 frontend-visual-conventions：卡片阴影层级、语义色（warning 标记
     失效）、四档间距圆角、面板内覆盖层 absolute 定位（z-index 10 与预览层同档，
     盖过画廊与浮动选择条）。 -->
<template>
  <div v-if="visible" class="cart-drawer">
    <div class="cart-head">
      <span class="cart-title">
        购物栏
        <span class="cart-count">{{ items.length }}</span>
      </span>
      <button class="cart-close" type="button" aria-label="关闭购物栏" @click="emit('close')">&#10005;</button>
    </div>

    <!-- 导出进度条：仅导出中可见（进度载荷由宿主经 assets-cart:export-progress
         事件维护，视觉复用面板扫描进度条模式） -->
    <div v-if="exporting" class="cart-export-progress">
      <el-progress
        class="cart-export-progress-bar"
        :percentage="exportPercent"
        :stroke-width="8"
        :show-text="false"
      />
      <span class="cart-export-progress-text">
        {{ exportProgress ? `合并导出中 ${exportProgress.done}/${exportProgress.total}` : '正在准备合并导出…' }}
      </span>
    </div>

    <!-- 条目列表：拖拽排序（vue-draggable-plus），拖完上报 reorder 按键列表回写 -->
    <div class="cart-list">
      <VueDraggable
        v-model="localItems"
        :animation="200"
        ghost-class="cart-item--ghost"
        handle=".cart-drag-handle"
        @end="onDragEnd"
      >
        <div
          v-for="item in localItems"
          :key="itemKey(item)"
          class="cart-item"
          :class="{ 'is-stale': item.stale }"
          @click="emit('trace', item)"
        >
          <el-icon class="cart-drag-handle" title="拖拽调整顺序"><Rank /></el-icon>
          <AssetThumb
            :library-id="item.libraryId"
            :file-id="item.fileId"
            :slide-num="item.slideNum"
            :tier="0"
            class="cart-thumb"
          />
          <div class="cart-item-info">
            <div class="cart-item-title" :title="item.slideTitle || item.fileName">
              {{ item.slideTitle || item.fileName }}
            </div>
            <div class="cart-item-source" :title="`${item.libraryName}/${item.fileName} · 第 ${item.slideNum} 页`">
              <span class="cart-item-lib">{{ item.libraryName || item.libraryId }}</span>
              /
              {{ item.fileName }}
              · 第 {{ item.slideNum }} 页
            </div>
            <div v-if="item.stale" class="cart-item-stale-mark">源文件不存在，导出将跳过</div>
          </div>
          <el-icon
            class="cart-item-remove"
            title="移出购物栏"
            @click.stop="emit('remove', item)"
          ><Delete /></el-icon>
        </div>
      </VueDraggable>
      <el-empty
        v-if="items.length === 0"
        class="cart-empty"
        description="购物栏是空的：在幻灯片卡片、详情或预览中点「加入购物栏」收集页面"
        :image-size="60"
      />
    </div>

    <!-- 底部操作区：清空 + 导出（导出运行态禁用防并发，取消入口并列） -->
    <div class="cart-footer">
      <el-button
        size="small"
        :disabled="exporting || items.length === 0"
        @click="onClear"
      >清空</el-button>
      <el-button
        size="small"
        type="primary"
        :loading="exporting"
        :disabled="items.length === 0"
        @click="emit('export')"
      >导出（{{ items.length }} 页）</el-button>
      <el-button
        v-if="exporting"
        size="small"
        text
        type="danger"
        @click="emit('cancel-export')"
      >取消导出</el-button>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import { ElMessageBox } from 'element-plus'
import { Delete, Rank } from '@element-plus/icons-vue'
import AssetThumb from './AssetThumb.vue'
import { assetCartItemKey } from '../utils/assetCart'

const props = defineProps({
  // 显隐受控（宿主 cartDrawerVisible）
  visible: { type: Boolean, default: false },
  // 购物栏条目清单（宿主 List 快照，含 stale 探测标志）
  items: { type: Array, default: () => [] },
  // 合并导出运行态（禁改第一防线 + 进度条显隐）
  exporting: { type: Boolean, default: false },
  // 导出进度载荷（assets-cart:export-progress：{outPath, done, total}）
  exportProgress: { type: Object, default: null }
})

const emit = defineEmits(['close', 'remove', 'clear', 'reorder', 'export', 'cancel-export', 'trace'])

const itemKey = (item) => assetCartItemKey(item)

// localItems 为 VueDraggable v-model 的可变副本（AiChatPanel/DirectoryTree 同款
// 模式：拖拽时原地重排 + onDragEnd 上报持久化），数据源恒为宿主快照
const localItems = ref([])
watch(
  () => props.items,
  (items) => {
    localItems.value = [...items]
  },
  { immediate: true, deep: true }
)

// 拖完回写：上报新顺序的按键列表（宿主调 ReorderAssetCart 后 List 刷新经 props
// 回流同步副本；回写失败宿主重拉 List，副本随 watch 还原）
const onDragEnd = () => {
  emit('reorder', localItems.value.map(itemKey))
}

const exportPercent = computed(() => {
  if (!props.exportProgress || !props.exportProgress.total) return 0
  return Math.round((props.exportProgress.done / props.exportProgress.total) * 100)
})

// 清空需确认（对齐删除类操作惯例）：确认后上报 clear，宿主调 ClearAssetCart
const onClear = async () => {
  try {
    await ElMessageBox.confirm(
      `确定清空购物栏的全部 ${props.items.length} 项？`,
      '清空购物栏',
      { confirmButtonText: '清空', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  emit('clear')
}
</script>

<style scoped>
/* 面板内覆盖层（absolute 铺满宿主 .assets-view-wrap，z-index 10 与预览层同档：
     盖过画廊卡与浮动选择条，不触碰全局右键菜单 2000 / EP 弹窗层级表） */
.cart-drawer {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  background: var(--bg-primary);
}

.cart-head {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-secondary);
}

.cart-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: 15px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.cart-count {
  font-size: 12px;
  font-weight: 500;
  color: var(--primary-color);
  background: var(--primary-bg);
  border-radius: var(--radius-sm);
  padding: 0 var(--spacing-sm);
}

.cart-close {
  border: none;
  background: transparent;
  color: var(--text-tertiary);
  font-size: 14px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  transition: all var(--transition-normal);
}

.cart-close:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

.cart-export-progress {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-md);
  background: var(--primary-bg);
  border-bottom: 1px solid var(--border-color);
}

.cart-export-progress-bar {
  width: 220px;
}

.cart-export-progress-text {
  font-size: 12px;
  color: var(--text-primary);
  white-space: nowrap;
}

.cart-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--spacing-md);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.cart-empty {
  margin: auto;
}

/* 条目行：缩略图 + 来源信息 + 拖拽手柄/移除；点击行体溯源定位（宿主分派） */
.cart-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-sm);
  cursor: pointer;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.cart-item:hover {
  box-shadow: var(--shadow-md);
  border-color: var(--primary-light);
}

/* 拖拽占位态 */
.cart-item--ghost {
  opacity: 0.4;
}

/* 失效条目：整体置灰（对齐失效行 opacity 0.5 弱化惯例），仍可点击/拖拽/移除 */
.cart-item.is-stale {
  opacity: 0.5;
}

.cart-drag-handle {
  flex-shrink: 0;
  color: var(--text-placeholder);
  cursor: grab;
}

.cart-drag-handle:active {
  cursor: grabbing;
}

.cart-thumb {
  flex-shrink: 0;
  width: 96px;
}

.cart-item-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.cart-item-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cart-item-source {
  font-size: 12px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cart-item-lib {
  color: var(--text-secondary);
}

.cart-item-stale-mark {
  font-size: 12px;
  color: var(--warning-color);
}

.cart-item-remove {
  flex-shrink: 0;
  color: var(--text-placeholder);
  cursor: pointer;
  transition: color var(--transition-fast);
}

.cart-item-remove:hover {
  color: var(--danger-color);
}

.cart-footer {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border-top: 1px solid var(--border-color);
  background: var(--bg-secondary);
}
</style>
