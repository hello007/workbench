<!-- frontend/src/components/AssetPreviewOverlay.vue -->
<!-- 素材幻灯片全屏预览层（SRS §2.3 PR2）：面板内覆盖画廊列的受控组件（非 fixed
     全窗）——宿主 .assets-view-wrap 为 relative 语境，本层 absolute inset:0 铺满
     画廊区，详情侧栏位于覆盖层之外保持可见可编辑（PRD D5#6 预览即详情导航）。
     受控组件契约：翻页序列（slides = displayCards 过滤 kind==='slide' 子序列）与
     当前下标（index）全由宿主持有，本层只上报 update:index（翻页）/ close（关闭）/
     load-more（懒加载边界续拉）/ add-to-cart（加入购物栏，PR3）四个事件；分页判定
     与详情装配都在宿主。
     层级取舍：z-index 取面板内局部值 10（盖过画廊卡与列表 sticky 表头 z-index:1，
     不触碰全局右键菜单 2000 / EP 弹窗 2001+ 层级表，真机确认后 spec 沉淀定稿）。
     大图消费纪律：AssetThumb 恒 tier=1（hd 档）+ hide-on-miss，禁改组件——miss 时
     组件不渲染，底层常驻「暂无高清图」弱化占位透出（非空白），生成可用后随页切换
     自动补图（epoch 代次透传）。
     键盘守卫对齐终端全屏 ESC 惯例（terminal-multi-session 契约）：过滤
     defaultPrevented（他组件已消费）与目标位于 .el-overlay（EP 弹层）/
     .xterm-helper-textarea（终端辅助区）的事件；监听随组件卸载注销（受控组件
     v-if 挂载/卸载即挂/摘监听）。
     懒加载边界取舍（PRD D1）：末页「→」不预翻——canLoadMore 为真时上报 load-more
     触发宿主续拉（写回后序列变长用户再点翻页），为假时按钮禁用。 -->
<template>
  <div v-if="current" class="asset-preview-overlay">
    <div class="preview-toolbar">
      <span class="preview-counter">{{ index + 1 }} / {{ slides.length }}</span>
      <div class="preview-actions">
        <!-- 加入购物栏（PR3 预览层入口）：已收集态图标反馈（in-cart 由宿主键判定），
             点击上报 add-to-cart 分派宿主幂等入栏 -->
        <button
          class="preview-cart-btn"
          :class="{ 'is-in-cart': inCart }"
          type="button"
          :title="inCart ? '已在购物栏' : '加入购物栏'"
          :aria-label="inCart ? '已在购物栏' : '加入购物栏'"
          @click="emit('add-to-cart')"
        >{{ inCart ? '✓ 已在购物栏' : '＋ 加入购物栏' }}</button>
        <button class="preview-close" type="button" aria-label="关闭预览" @click="emit('close')">&#10005;</button>
      </div>
    </div>
    <div class="preview-stage">
      <!-- miss 兜底占位：常驻底层，AssetThumb（hide-on-miss）loading/ready 时覆盖其上 -->
      <div class="preview-miss-hint">暂无高清图</div>
      <AssetThumb
        :library-id="libraryId"
        :file-id="current.fileId"
        :slide-num="current.slideNum"
        :tier="1"
        :epoch="thumbEpoch"
        hide-on-miss
        :alt="current.title"
        class="preview-thumb"
      />
    </div>
    <div class="preview-info">
      <span class="preview-title" :title="current.title">{{ current.title }}</span>
      <span class="preview-path" :title="current.filePath">{{ current.filePath }}</span>
    </div>
    <button
      class="preview-nav preview-nav-prev"
      type="button"
      aria-label="上一页"
      :disabled="!canPrev"
      @click="goPrev"
    >&#8592;</button>
    <button
      class="preview-nav preview-nav-next"
      type="button"
      aria-label="下一页"
      :disabled="!canNext"
      @click="goNext"
    >&#8594;</button>
  </div>
</template>

<script setup>
import { computed, onMounted, onBeforeUnmount } from 'vue'
import AssetThumb from './AssetThumb.vue'

const props = defineProps({
  // 翻页序列：displayCards 过滤 kind==='slide' 的子序列（宿主 computed，含检索命中
  // 与聚合父卡，不含聚合展开子项——子项不在 displayCards，PRD D1 取舍登记）
  slides: { type: Array, default: () => [] },
  // 当前页下标（受控：宿主 previewIndex ref，本层不持有翻页状态）
  index: { type: Number, default: 0 },
  libraryId: { type: String, default: '' },
  thumbEpoch: { type: Number, default: 0 },
  // 懒加载边界判定（宿主 listCanLoadMore 同源）：末页「→」为真时上报 load-more 续拉
  canLoadMore: { type: Boolean, default: false },
  // 当前预览页是否已在购物栏（PR3：宿主键判定传入，本层只做图标反馈）
  inCart: { type: Boolean, default: false }
})

const emit = defineEmits(['update:index', 'close', 'load-more', 'add-to-cart'])

// 当前页卡片：下标越界（序列收缩竞态兜底）时为 null，整层不渲染（宿主侧另有越界
// 关闭 watch，此处为组件级防御）
const current = computed(() => props.slides[props.index] || null)

const canPrev = computed(() => props.index > 0)
// 末页边界：canLoadMore 为真时按钮保持可用（点击触发续拉不预翻），否则禁用
const canNext = computed(() => props.index < props.slides.length - 1 || props.canLoadMore)

const goPrev = () => {
  if (props.index > 0) emit('update:index', props.index - 1)
}

const goNext = () => {
  if (props.index < props.slides.length - 1) {
    emit('update:index', props.index + 1)
    return
  }
  if (props.canLoadMore) emit('load-more')
}

// 可编辑元素判定：详情侧栏备注 textarea / 标签输入与预览并存（D5#6 编辑照常可用），
// 方向键在这些元素内是文本光标移动语义，翻页会打断编辑
const isEditableTarget = (target) => {
  if (!target || typeof target.tagName !== 'string') return false
  const tag = target.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || !!target.isContentEditable
}

// 键盘处理：ESC 关闭走终端全屏同款守卫（defaultPrevented + EP 弹层/xterm 辅助区
// 过滤）；左右翻页在其上追加可编辑元素聚焦跳过
const onKeydown = (e) => {
  if (e.defaultPrevented) return
  const target = e.target
  if (typeof target?.closest === 'function' && target.closest('.el-overlay, .xterm-helper-textarea')) return
  if (e.key === 'Escape') {
    emit('close')
    return
  }
  if (e.key === 'ArrowLeft' && !isEditableTarget(target)) {
    goPrev()
    return
  }
  if (e.key === 'ArrowRight' && !isEditableTarget(target)) {
    goNext()
  }
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
})
</script>

<style scoped>
/* 面板内覆盖层（非 fixed 全窗）：absolute 铺满宿主 .assets-view-wrap（relative
     语境），完整 flex 约束链防内容溢出（frontend-visual-conventions） */
.asset-preview-overlay {
  position: absolute;
  inset: 0;
  /* 面板内局部层级：盖过画廊卡与列表 sticky 表头（z-index:1），不触碰全局右键
       菜单 2000 / EP 弹窗 2001+ 层级表（真机确认后 spec 沉淀定稿） */
  z-index: 10;
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  background: var(--bg-primary);
}

.preview-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
}

.preview-counter {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  font-variant-numeric: tabular-nums;
}

.preview-close {
  border: none;
  background: transparent;
  color: var(--text-tertiary);
  font-size: 14px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  transition: all var(--transition-normal);
}

.preview-close:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

/* 工具行右侧动作组（PR3）：加入购物栏 + 关闭同排 */
.preview-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

/* 加入购物栏按钮（PR3 预览层入口）：键帽/工具行小按钮形态；已收集态主色弱化底
     反馈（semantic primary，正向动作） */
.preview-cart-btn {
  border: 1px solid var(--border-color);
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-size: 12px;
  cursor: pointer;
  padding: 3px 10px;
  border-radius: var(--radius-sm);
  transition: all var(--transition-fast);
}

.preview-cart-btn:hover {
  border-color: var(--primary-light);
  color: var(--primary-color);
}

.preview-cart-btn.is-in-cart {
  border-color: var(--primary-light);
  background: var(--primary-bg);
  color: var(--primary-color);
  cursor: default;
}

.preview-stage {
  flex: 1;
  min-height: 0;
  min-width: 0;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-md);
  overflow: hidden;
}

/* miss 兜底占位（弱化非空白）：常驻底层，AssetThumb 渲染时被上层大图覆盖 */
.preview-miss-hint {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  color: var(--text-placeholder);
}

/* 大图占满 stage：AssetThumb 根元素仅有宽度 + 16:9 aspect-ratio，此处补显式高度
     （双维约束下 aspect-ratio 自然失效），img 自带 object-fit: contain 保比例；
     z-index 压过 miss 占位 */
.preview-thumb {
  position: relative;
  z-index: 1;
  width: 100%;
  height: 100%;
}

.preview-info {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  border-top: 1px solid var(--border-color);
}

.preview-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-path {
  font-size: 12px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 左右翻页按钮：垂直居中悬浮于 stage 两侧；z-index 须压过 .preview-thumb 的
     z-index:1（hd 图 ready 后铺满 stage，漏设则按钮被图片绘制层盖死不可点） */
.preview-nav {
  z-index: 2;
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: 1px solid var(--border-color);
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 16px;
  line-height: 1;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-sm);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.preview-nav:hover:not(:disabled) {
  border-color: var(--primary-light);
  box-shadow: var(--shadow-md);
}

.preview-nav:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.preview-nav-prev {
  left: var(--spacing-md);
}

.preview-nav-next {
  right: var(--spacing-md);
}
</style>
