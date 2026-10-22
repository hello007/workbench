<!-- frontend/src/components/AssetListView.vue -->
<!-- 素材列表视图（SRS §2.3 PR1）：网格呈现之外的表格呈现形态，与画廊共用
     displayCards 同构卡片模型（浏览清单/检索命中统一形态），滚动懒加载复用宿主
     分页链路。
     受控组件契约：无自有业务状态——选中/详情/检索代次/分页判定全留宿主；行点击
     仅上报 card-click（对齐网格 onCardClick 语义），滚动到底仅上报 load-more
     （防重与分页知识在宿主 loadMoreInView 单点），组件不持有分页知识。
     选中体系（SRS §2.3 PR3）：行首复选框列受控渲染——选中键集由宿主 selectedKeys
     传入，勾选仅上报 toggle-select(card)，键映射消费 utils/assetSelection 共享
     单点（与网格卡同一键形态，D5#5 呈现切换选中保留的前提）；勾选经 @click.stop
     阻断行点击语义（D5#7 勾选不进详情）。聚合父卡（hitCount>1）不渲染复选框、
     聚合展开子行保持展示型无复选框（PRD D5#1：聚合卡勾选语义歧义，展开后不勾
     子行——检索聚合子行本就展示型）。
     列集分语境（PRD D3）：文件条目 = 名称/路径/大小/修改时间/星级/标签/状态；
     幻灯片条目 = 页标题/归属文件/页码/文件大小/修改时间/星级/自有标签（检索语境
     追加命中摘要列）。幻灯片条目与检索条目的大小/修改时间经 fileEntries 底册按
     filePath 反查 join（† 归属文件属性口径，表头 tooltip 声明），底册 miss 一律
     显 — 不猜值；聚合条目（hitCount>1）大小列显 —（多页命中不显单文件大小）。
     聚合展开态经宿主透传（expandedAggPaths/aggChildren/aggLoadingPaths + 子页分页
     态 aggTotals/aggLoadingMorePaths）渲染缩进子行与「加载更多」行，保持聚合展开
     交互在列表语境可用（对齐 PRD「现有交互兼容」）。 -->
<template>
  <div ref="listRef" class="asset-list" @scroll="onListScroll">
    <table class="asset-list-table">
      <thead>
        <tr>
          <th class="col-check"></th>
          <th class="col-thumb"></th>
          <th v-for="col in columns" :key="col.key" :class="col.class" :title="col.tooltip || undefined">
            {{ col.label }}
          </th>
        </tr>
      </thead>
      <tbody>
        <template v-for="card in cards" :key="card.key">
          <tr
            class="asset-list-row"
            :draggable="card.kind === 'file'"
            @click="emit('card-click', card, $event)"
            @dragstart="emit('card-drag-start', card, $event)"
          >
            <td class="col-check">
              <!-- 行首复选框（受控）：聚合父卡不渲染（D5#1）；@click.stop 阻断行
                   点击进详情（D5#7），勾选仅上报 toggle-select 由宿主写选中键集 -->
              <input
                v-if="card.hitCount <= 1"
                type="checkbox"
                class="asset-row-check"
                :checked="isRowChecked(card)"
                :aria-label="`选中 ${card.title}`"
                @click.stop
                @change="emit('toggle-select', card)"
              />
            </td>
            <td class="col-thumb">
              <!-- 行首缩略图与网格同口径：tier=0（small 档）懒加载，文件卡取第 1 页封面 -->
              <AssetThumb
                :library-id="libraryId"
                :file-id="card.kind === 'file' ? card.id : card.fileId"
                :slide-num="card.kind === 'slide' ? card.slideNum : 1"
                :tier="0"
                :epoch="thumbEpoch"
                class="asset-list-thumb"
              />
            </td>
            <td
              v-for="col in columns"
              :key="col.key"
              :class="col.class"
              :title="cellTip(card, col.key)"
            >
              <template v-if="col.key === 'title'">{{ card.title }}</template>
              <template v-else-if="col.key === 'path'">{{ card.filePath }}</template>
              <template v-else-if="col.key === 'page'">第 {{ card.slideNum }} 页</template>
              <template v-else-if="col.key === 'size'">{{ sizeText(card) }}</template>
              <template v-else-if="col.key === 'modTime'">{{ modTimeText(card) }}</template>
              <template v-else-if="col.key === 'snippet'">{{ card.snippet }}</template>
              <template v-else-if="col.key === 'rating'">{{ ratingText(card) }}</template>
              <template v-else-if="col.key === 'tags'">
                <!-- 标签色点与宿主网格卡同款模式（色值为后端下发数据，动态绑定呈现） -->
                <span v-if="tagsOf(card).length === 0">—</span>
                <span v-else class="asset-list-tags">
                  <span v-for="tag in tagsOf(card)" :key="tag.id" class="card-tag">
                    <span class="tag-color-dot" :style="{ background: tag.color }"></span>{{ tag.name }}
                  </span>
                </span>
              </template>
              <template v-else-if="col.key === 'status'">
                <el-tag v-if="statusOf(card)" size="small" :type="assetStatusTagType(statusOf(card))">
                  {{ assetStatusLabel(statusOf(card)) }}
                </el-tag>
                <span v-else>—</span>
              </template>
            </td>
          </tr>
          <!-- 聚合条目展开态：同 PPT 逐页命中的缩进子行（展开态/子项数据均为宿主
               透传，组件不持展开知识；@click.stop 防子行点击冒泡触发父行聚合切换）；
               子页超出首窗时尾部「加载更多」行仅上报 agg-load-more（续拉分派与守卫
               知识留宿主，对齐组件受控契约） -->
          <template v-if="card.hitCount > 1 && expandedAggPaths.includes(card.filePath)">
            <tr v-if="aggLoadingPaths.includes(card.filePath)" class="agg-child-row">
              <td :colspan="columns.length + 2">加载命中页…</td>
            </tr>
            <tr
              v-for="child in aggChildren[card.filePath] || []"
              :key="child.key"
              class="agg-child-row is-clickable"
              @click.stop="emit('card-click', child, $event)"
            >
              <td :colspan="columns.length + 2">
                <span class="agg-child-page">第 {{ child.slideNum }} 页</span>
                <span class="agg-child-title" :title="child.title">{{ child.title }}</span>
                <span v-if="child.snippet" class="agg-child-snippet" :title="child.snippet">{{ child.snippet }}</span>
              </td>
            </tr>
            <tr
              v-if="aggHasMore(card.filePath)"
              class="agg-child-row agg-load-more-row"
              @click.stop="emit('agg-load-more', card.filePath)"
            >
              <td :colspan="columns.length + 2">{{ aggLoadMoreLabel(card.filePath) }}</td>
            </tr>
          </template>
        </template>
      </tbody>
    </table>
    <!-- 懒加载提示：与网格 load-more-hint 同文案（检索模式滚动到底增量加载下一页；
         浏览幻灯片清单分页由宿主同一分派函数驱动） -->
    <div v-if="loadingMore" class="load-more-hint">加载中…</div>
    <div v-else-if="canLoadMore" class="load-more-hint">下拉加载更多</div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import AssetThumb from './AssetThumb.vue'
import { assetStatusLabel, assetStatusTagType, formatAssetSize } from '../utils/assetFormat'
// 选中键映射共享单点（与网格卡同一键形态，D5#5 呈现切换选中保留的前提）
import { assetSelectionKey } from '../utils/assetSelection'

// join miss 展示符（底册反查不命中时不猜值，空态文案与文件属性缺失统一口径）
const DASH = '—'

const props = defineProps({
  // displayCards 同构卡片数组（浏览清单/检索命中统一形态，宿主已按视图过滤）
  cards: { type: Array, default: () => [] },
  // 文件底册（大小/修改时间/状态 join 数据源；幻灯片与检索条目无文件属性字段）
  fileEntries: { type: Array, default: () => [] },
  libraryId: { type: String, default: '' },
  thumbEpoch: { type: Number, default: 0 },
  searchMode: { type: Boolean, default: false },
  // 懒加载提示态（宿主判定，组件只渲染提示不持分页知识）
  loadingMore: { type: Boolean, default: false },
  canLoadMore: { type: Boolean, default: false },
  // 聚合展开态（宿主透传；浏览语境为空值，仅检索聚合条目消费）
  expandedAggPaths: { type: Array, default: () => [] },
  aggChildren: { type: Object, default: () => ({}) },
  aggLoadingPaths: { type: Array, default: () => [] },
  // 聚合子页分页态（宿主透传，与网格「加载更多」同口径）：totals = 单文件命中页
  // 总数（首拉 page.total），loadingMorePaths = 在途续拉防重集合
  aggTotals: { type: Object, default: () => ({}) },
  aggLoadingMorePaths: { type: Array, default: () => [] },
  // 选中键集（宿主 selectedKeys Set 整体替换透传，本组件只读；键形态见
  // utils/assetSelection 共享单点）
  selectedKeys: { type: Set, default: () => new Set() }
})

const emit = defineEmits(['card-click', 'load-more', 'agg-load-more', 'toggle-select', 'card-drag-start'])

const listRef = ref(null)

// 行勾选态判定：消费共享键映射查宿主选中键集（受控只读，写回经 toggle-select）
const isRowChecked = (card) => props.selectedKeys.has(assetSelectionKey(card))

// 列集按首卡 kind 判定（displayCards 经宿主视图过滤后单次渲染 kind 单一；空列表
// 时列集无消费面，取文件列集兜底）
const isSlideCards = computed(() => props.cards[0]?.kind === 'slide')

const columns = computed(() => {
  const slide = isSlideCards.value
  const cols = [
    { key: 'title', label: slide ? '页标题' : '名称', class: 'col-title' },
    { key: 'path', label: slide ? '归属文件' : '路径', class: 'col-path' }
  ]
  if (slide) cols.push({ key: 'page', label: '页码', class: 'col-page' })
  cols.push({ key: 'size', label: '文件大小', class: 'col-size', tooltip: slide ? '归属文件属性' : '' })
  cols.push({ key: 'modTime', label: '修改时间', class: 'col-time', tooltip: slide ? '归属文件属性' : '' })
  // 命中摘要列仅检索语境的幻灯片条目提供（文件命中首行即含关键词语境，摘要增量小）
  if (slide && props.searchMode) cols.push({ key: 'snippet', label: '命中摘要', class: 'col-snippet' })
  cols.push({ key: 'rating', label: '星级', class: 'col-rating' })
  cols.push({ key: 'tags', label: slide ? '自有标签' : '标签', class: 'col-tags' })
  if (!slide) cols.push({ key: 'status', label: '状态', class: 'col-status' })
  return cols
})

// 底册反查映射：构建一次 filePath → entry（O(n)），行级 join O(1)——逐行 find 在
// 千行清单 × 千条底册下是 O(n²)，表格渲染每行两列消费，须避免
const entryByPath = computed(() => {
  const map = new Map()
  for (const entry of props.fileEntries || []) {
    // 同路径重复记录（理论不存在）取首条，防 Map 后写覆盖造成展示抖动
    if (!map.has(entry.path)) map.set(entry.path, entry)
  }
  return map
})

const joinEntry = (card) => entryByPath.value.get(card.filePath) || null

// 大小列：浏览文件卡直读底册条目；幻灯片/检索条目经 join；聚合条目（多页命中）
// 不显大小（PRD D3）；join miss 一律 —
const sizeText = (card) => {
  if (card.hitCount > 1) return DASH
  if (card.entry) return formatAssetSize(card.entry.size)
  const joined = joinEntry(card)
  return joined ? formatAssetSize(joined.size) : DASH
}

// 修改时间列：聚合条目不豁免（文件属性仍可 join 命中），join miss 显 —
const modTimeText = (card) => {
  if (card.entry) return card.entry.modTime || DASH
  return joinEntry(card)?.modTime || DASH
}

// 星级取值链：浏览文件卡 entry → 浏览幻灯片卡 slide → 检索命中 hit → 底册 join
// 兜底（检索文件条目 hit 无标注时以底册标注为准）；0 = 未评显 —
const ratingText = (card) => {
  const rating = card.entry?.rating ?? card.slide?.rating ?? card.hit?.rating ?? joinEntry(card)?.rating ?? 0
  return rating > 0 ? `${rating} 星` : DASH
}

// 标签列：文件条目显文件标签（entry.tags / hit.tags），幻灯片条目显页自有标签
//（ownTags 与继承标签口径区分——继承标签在详情侧栏只读区呈现，列表不混显）
const tagsOf = (card) => {
  if (isSlideCards.value) {
    return card.slide?.ownTags ?? card.hit?.ownTags ?? []
  }
  return card.entry?.tags ?? card.hit?.tags ?? card.tags ?? []
}

// 状态列：浏览文件卡直读条目状态；检索文件条目经 join（底册 miss 显 —）；
// 幻灯片条目无状态列
const statusOf = (card) => {
  if (card.entry) return card.entry.status || ''
  return joinEntry(card)?.status || ''
}

// 聚合子页「加载更多」：可见性与文案均由宿主透传态派生（分页判定知识在宿主
// loadMoreAggChildren，这里只做展示投影，与网格同口径）
const aggHasMore = (filePath) =>
  (props.aggTotals[filePath] || 0) > (props.aggChildren[filePath]?.length || 0)

const aggLoadMoreLabel = (filePath) => props.aggLoadingMorePaths.includes(filePath)
  ? '加载中…'
  : `加载更多（已显 ${props.aggChildren[filePath]?.length || 0} / 共 ${props.aggTotals[filePath] || 0} 页）`

// 单元格悬浮提示：截断列给全文（snippet 同时是检索语境的关键命中上下文）
const cellTip = (card, key) => {
  if (key === 'snippet') return card.snippet || undefined
  if (key === 'title' || key === 'path') return card[key]
  return undefined
}

// 滚动到底判定复刻宿主 onGalleryScroll 同一公式（scrollTop+clientHeight ≥
// scrollHeight-80）：防重与分页分派在宿主，这里只做距离预过滤减少无谓上报
const onListScroll = () => {
  const el = listRef.value
  if (!el) return
  if (el.scrollTop + el.clientHeight < el.scrollHeight - 80) return
  emit('load-more')
}
</script>

<style scoped>
/* 滚动容器：宿主 .assets-view-wrap（flex column）内占满剩余空间，min-height:0
     链防溢出撑开面板（frontend-visual-conventions 一级面板约束链的容器级延续） */
.asset-list {
  flex: 1;
  min-height: 0;
  min-width: 0;
  overflow-y: auto;
}

.asset-list-table {
  width: 100%;
  /* 固定布局：列宽由表头定义驱动，行内长文本（路径/摘要）截断不撑开表格 */
  table-layout: fixed;
  border-collapse: collapse;
  font-size: 12px;
}

.asset-list-table th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--bg-secondary);
  text-align: left;
  font-weight: 500;
  color: var(--text-tertiary);
  padding: var(--spacing-xs) var(--spacing-sm);
  border-bottom: 1px solid var(--border-color);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.asset-list-table td {
  padding: var(--spacing-xs) var(--spacing-sm);
  border-bottom: 1px solid var(--border-light);
  color: var(--text-secondary);
  vertical-align: middle;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 行 hover 轻量高亮（对齐 frontend-visual-conventions：--primary-bg 禁主色实色
     填充压字），行点击语义与网格卡一致（进详情） */
.asset-list-row {
  cursor: pointer;
  transition: background var(--transition-fast);
}

.asset-list-row:hover {
  background: var(--primary-bg);
}

.asset-list-table th.col-check,
.asset-list-table td.col-check {
  width: 36px;
  text-align: center;
}

/* 行首复选框：accent-color 跟随主色（亮暗主题自动适配原生勾选态视觉） */
.asset-row-check {
  width: 15px;
  height: 15px;
  accent-color: var(--primary-color);
  cursor: pointer;
  vertical-align: middle;
}

.asset-list-table th.col-thumb,
.asset-list-table td.col-thumb {
  width: 96px;
  padding: var(--spacing-xs);
}

/* 行首缩略图固定小尺寸（16:9 由 AssetThumb 根元素 aspect-ratio 自适应出 45px 高） */
.asset-list-thumb {
  width: 80px;
}

.asset-list-table th.col-title,
.asset-list-table td.col-title {
  width: 16%;
  color: var(--text-primary);
  font-weight: 500;
}

.asset-list-table td.col-title {
  font-size: 13px;
}

.asset-list-table th.col-path,
.asset-list-table td.col-path {
  width: 18%;
  color: var(--text-tertiary);
}

.asset-list-table th.col-page,
.asset-list-table td.col-page {
  width: 56px;
  color: var(--text-tertiary);
}

.asset-list-table th.col-size,
.asset-list-table td.col-size {
  width: 84px;
}

.asset-list-table th.col-time,
.asset-list-table td.col-time {
  width: 150px;
  color: var(--text-tertiary);
}

.asset-list-table th.col-snippet,
.asset-list-table td.col-snippet {
  color: var(--text-secondary);
}

.asset-list-table th.col-rating,
.asset-list-table td.col-rating {
  width: 64px;
}

.asset-list-table th.col-tags,
.asset-list-table td.col-tags {
  width: 15%;
}

.asset-list-table th.col-status,
.asset-list-table td.col-status {
  width: 76px;
}

.asset-list-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

/* 标签色点与宿主网格卡同款视觉（Vue scoped 隔离，样式声明无法跨组件继承） */
.card-tag {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  padding: 1px var(--spacing-xs);
}

.tag-color-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

/* ---- 聚合展开子行（缩进弱化，层级低于主行）---- */
.agg-child-row td {
  padding-left: var(--spacing-xl);
  background: var(--bg-tertiary);
  color: var(--text-tertiary);
}

.agg-child-row.is-clickable {
  cursor: pointer;
}

.agg-child-row.is-clickable:hover {
  background: var(--primary-bg);
}

/* 聚合子页「加载更多」行：可点弱化层（文案居中，hover 同子行高亮） */
.agg-load-more-row {
  cursor: pointer;
  text-align: center;
  color: var(--text-secondary);
}

.agg-child-page {
  color: var(--primary-color);
  margin-right: var(--spacing-sm);
}

.agg-child-title {
  color: var(--text-primary);
  margin-right: var(--spacing-sm);
}

.agg-child-snippet {
  color: var(--text-tertiary);
}

/* 懒加载提示：与网格 load-more-hint 同文案同层级弱化 */
.load-more-hint {
  text-align: center;
  font-size: 12px;
  color: var(--text-placeholder);
  padding: var(--spacing-sm) 0;
}
</style>
