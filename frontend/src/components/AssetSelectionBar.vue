<!-- frontend/src/components/AssetSelectionBar.vue -->
<!-- 素材选中浮动操作条（SRS §2.3 PR3，PRD D2）：选中数 > 0 时悬浮于画廊区底部
     居中，展示「已选 N 项」计数 + 全选/反选/批量打标签/批量评分/清空操作。
     受控组件契约：无自有业务状态——计数与全选/反选作用域（当前渲染独立卡片键集，
     PRD D5#8：检索含已加载段，懒加载未加载段不进范围）由宿主计算，本层只上报
     select-all / invert / batch-tag / batch-rating / clear 五个事件，分派逻辑留
     宿主单点（第六批 PR3 批量打标签入口消费 batch-tag，第八批批量评分入口消费
     batch-rating，targets 构建与 API 分派在宿主）。
     层级取舍：z-index 取面板内局部值 5——低于全屏预览层 10（预览打开时浮动条
     沉底不挡预览操作），不触碰全局右键菜单 2000 / EP 弹窗 2001+ 层级表。
     视觉：主色弱化底（--primary-bg）+ 卡片阴影层级（--shadow-md），对齐
     frontend-visual-conventions 语义色与间距档位。 -->
<template>
  <div v-if="count > 0" class="asset-selection-bar">
    <span class="selection-count">已选 {{ count }} 项</span>
    <div class="selection-actions">
      <el-button size="small" text type="primary" @click="emit('select-all')">全选</el-button>
      <el-button size="small" text type="primary" @click="emit('invert')">反选</el-button>
      <!-- 批量打标签（第六批 PR3，SRS §2.4）与批量评分（第八批，SRS §2.6 P0）：
           主操作入口，text+primary 与既有选择类操作同排同风格；targets 构建与
           API 分派留宿主 -->
      <el-button size="small" text type="primary" @click="emit('batch-tag')">批量打标签</el-button>
      <el-button size="small" text type="primary" @click="emit('batch-rating')">评分</el-button>
      <el-button size="small" text @click="emit('clear')">清空</el-button>
    </div>
  </div>
</template>

<script setup>
defineProps({
  // 选中计数（宿主 selectedKeys.size），> 0 时本条浮出
  count: { type: Number, default: 0 }
})

const emit = defineEmits(['select-all', 'invert', 'batch-tag', 'batch-rating', 'clear'])
</script>

<style scoped>
.asset-selection-bar {
  position: absolute;
  bottom: var(--spacing-lg);
  left: 50%;
  transform: translateX(-50%);
  /* 面板内局部层级：低于全屏预览层（z-index:10），高于画廊卡 */
  z-index: 5;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-md);
  background: var(--primary-bg);
  border: 1px solid var(--primary-light);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-md);
}

.selection-count {
  font-size: 13px;
  font-weight: 500;
  color: var(--primary-color);
  white-space: nowrap;
}

.selection-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}
</style>
