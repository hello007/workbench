<!-- frontend/src/components/AssetsPanel.vue -->
<!-- 素材管理面板（活动栏一级入口）：本地 DAM 素材索引库 MVP 前端。
     数据链路：库切换（ListAssetLibraries）→ 文件清单（ListAssetFiles，兼作标签聚合
     与幻灯片溯源的文件底册）+ 幻灯片清单（ListAssetSlides，按需加载）→ 关键词/标签
     检索（SearchAssets，分页懒加载 + 同 PPT 聚合折叠）→ 文件级元数据编辑
     （UpdateAssetFileMeta 整体替换）→ 扫描/重建（StartAssetScan/RebuildAssetLibrary，
     进度经 assets-scan:* 事件推送、可取消）。
     事件注销遵循 cross-layer-contracts：EventsOn 返回闭包精准注销，禁 EventsOff 全局移除。
     根元素保持完整 flex 约束链（flex:1 + min-height:0 + min-width:0 + display:flex +
     flex-direction:column + overflow:hidden），防面板塌陷挤压终端（frontend-visual-conventions）。 -->
<template>
  <div class="assets-panel">
    <!-- 顶部工具条：标题 + 库切换 + 库管理 + 扫描入口 -->
    <div class="assets-header">
      <span class="assets-title">
        <el-icon :size="18"><AssetsIcon /></el-icon>
        素材管理
      </span>
      <div class="header-actions">
        <el-select
          v-model="currentLibraryId"
          class="library-select"
          placeholder="选择素材库"
          size="small"
          :disabled="libraries.length === 0 || scanning"
          @change="onLibraryChange"
        >
          <el-option v-for="lib in libraries" :key="lib.id" :label="lib.name" :value="lib.id" />
        </el-select>
        <el-button size="small" @click="openCreateDialog">新建库</el-button>
        <el-button size="small" :disabled="!hasLibrary || scanning" @click="onDeleteLibrary">删除库</el-button>
        <el-button size="small" :disabled="!hasLibrary || scanning" @click="onRebuild">重建索引</el-button>
        <el-button
          size="small"
          type="primary"
          :disabled="!hasLibrary || scanning"
          @click="onScan"
        >扫描</el-button>
      </div>
    </div>

    <!-- 扫描进度条：仅扫描中可见；进度载荷按库 ID 过滤（他库事件不串入） -->
    <div v-if="scanning" class="scan-progress">
      <el-progress
        class="scan-progress-bar"
        :percentage="scanPercent"
        :stroke-width="8"
        :show-text="false"
      />
      <span class="scan-progress-text">
        {{ scanProgress ? `扫描中 ${scanProgress.done}/${scanProgress.total}` : '正在准备扫描…' }}
      </span>
      <span v-if="scanProgress && scanProgress.currentFile" class="scan-progress-file" :title="scanProgress.currentFile">
        {{ scanProgress.currentFile }}
      </span>
      <el-button size="small" text type="danger" @click="onCancelScan">取消扫描</el-button>
    </div>

    <!-- 搜索/筛选区：关键词（回车或按钮触发）+ 标签多选 + 视图切换 + 平铺开关 -->
    <div class="assets-toolbar">
      <el-input
        v-model="keywordInput"
        class="keyword-input"
        placeholder="搜索文件名、标题、页文本、备注…"
        size="small"
        clearable
        :disabled="!hasLibrary"
        @keyup.enter="applySearch"
        @clear="applySearch"
      />
      <el-select
        v-model="selectedTagIds"
        class="tag-filter"
        placeholder="按标签筛选"
        size="small"
        multiple
        collapse-tags
        collapse-tags-tooltip
        clearable
        :disabled="!hasLibrary"
        @change="applySearch"
      >
        <el-option v-for="tag in tagOptions" :key="tag.id" :label="tag.name" :value="tag.id">
          <span class="tag-option-row">
            <!-- 标签色为后端数据（#RRGGBB），动态绑定是唯一呈现方式，非静态行内样式 -->
            <span class="tag-color-dot" :style="{ background: tag.color }"></span>
            {{ tag.name }}
          </span>
        </el-option>
      </el-select>
      <el-radio-group v-model="viewMode" size="small" @change="onViewModeChange">
        <el-radio-button value="file">文件视图</el-radio-button>
        <el-radio-button value="slide">幻灯片视图</el-radio-button>
      </el-radio-group>
      <el-tooltip content="开启后同一 PPT 的多页命中不再折叠为一个条目，逐页平铺展示" placement="bottom">
        <span class="flatten-switch">
          <el-switch v-model="flatten" size="small" :disabled="!hasLibrary" @change="onFlattenChange" />
          平铺
        </span>
      </el-tooltip>
      <el-button size="small" type="primary" plain :disabled="!hasLibrary" @click="applySearch">搜索</el-button>
    </div>

    <!-- 库统计行：口径来自 GetAssetLibraryStats；失败/加密计数仅在 >0 时显示 -->
    <div v-if="stats" class="assets-stats-line">
      <span>文件 {{ stats.fileCount }}</span>
      <span>幻灯片 {{ stats.slideCount }}</span>
      <span v-if="stats.failedCount > 0" class="stats-warn">解析失败 {{ stats.failedCount }}</span>
      <span v-if="stats.encryptedCount > 0" class="stats-warn">已加密 {{ stats.encryptedCount }}</span>
      <span v-if="searchMode" class="stats-hit">命中 {{ searchTotal }} 条</span>
    </div>

    <!-- 主体：画廊网格 + 详情侧边栏 -->
    <div class="assets-body">
      <div ref="galleryRef" class="assets-gallery" @scroll="onGalleryScroll">
        <!-- 空态一：无任何素材库，引导建库 -->
        <el-empty v-if="libraries.length === 0 && !loading" description="还没有素材库，新建一个并选择素材根目录（原始文件不会被修改，仅建索引）">
          <el-button type="primary" @click="openCreateDialog">新建素材库</el-button>
        </el-empty>
        <!-- 空态二：有库但未扫描，引导扫描 -->
        <el-empty
          v-else-if="hasLibrary && !loading && !scanning && !searchMode && browseCards.length === 0 && stats && stats.fileCount === 0"
          description="当前库还没有索引数据，点击「扫描」导入根目录下的 pptx 文件"
        >
          <el-button type="primary" @click="onScan">开始扫描</el-button>
        </el-empty>
        <!-- 空态三：检索无结果 -->
        <el-empty v-else-if="searchMode && !loading && displayCards.length === 0" description="没有匹配的素材，换个关键词或筛选条件试试" />
        <!-- 空态四：视图无数据（如幻灯片视图下未扫描/无页面） -->
        <el-empty v-else-if="hasLibrary && !loading && !searchMode && browseCards.length === 0 && stats && stats.fileCount > 0" description="该视图下暂无条目" />

        <template v-else>
          <div
            v-for="card in displayCards"
            :key="card.key"
            class="asset-card"
            :class="{ 'is-selected': isCardSelected(card), 'is-expandable': card.hitCount > 1 }"
            @click="onCardClick(card)"
          >
            <div class="asset-card-head">
              <el-tag size="small" :type="card.kind === 'file' ? 'primary' : 'success'">
                {{ card.kind === 'file' ? '文件' : '幻灯片' }}
              </el-tag>
              <span v-if="card.kind === 'slide'" class="asset-card-page">第 {{ card.slideNum }} 页</span>
              <span v-if="card.hitCount > 1" class="asset-card-hit">{{ card.hitCount }} 页命中</span>
              <el-tag v-if="card.status && card.status !== 'ok'" size="small" :type="statusTagType(card.status)">
                {{ statusLabel(card.status) }}
              </el-tag>
            </div>
            <div class="asset-card-title" :title="card.title">{{ card.title }}</div>
            <div class="asset-card-sub" :title="card.filePath">{{ card.kind === 'slide' ? `归属：${card.filePath}` : card.filePath }}</div>
            <div v-if="card.snippet" class="asset-card-snippet">{{ card.snippet }}</div>
            <div v-if="card.tags && card.tags.length > 0" class="asset-card-tags">
              <span v-for="tag in card.tags" :key="tag.id" class="card-tag">
                <span class="tag-color-dot" :style="{ background: tag.color }"></span>{{ tag.name }}
              </span>
            </div>
            <!-- 聚合条目展开态：同 PPT 的逐页命中（点击聚合卡切换展开/收起） -->
            <div v-if="card.hitCount > 1 && isAggExpanded(card.filePath)" class="agg-children" @click.stop>
              <div v-if="aggLoading === card.filePath" class="agg-loading">加载命中页…</div>
              <div
                v-for="child in aggChildren[card.filePath] || []"
                :key="child.key"
                class="agg-child"
                @click.stop="onCardClick(child)"
              >
                <span class="agg-child-page">第 {{ child.slideNum }} 页</span>
                <span class="agg-child-title" :title="child.title">{{ child.title }}</span>
                <span v-if="child.snippet" class="agg-child-snippet" :title="child.snippet">{{ child.snippet }}</span>
              </div>
            </div>
          </div>
          <!-- 懒加载提示：检索模式滚动到底部增量加载下一页 -->
          <div v-if="searchMode && loadingMore" class="load-more-hint">加载中…</div>
          <div v-else-if="searchMode && canLoadMore" class="load-more-hint">下拉加载更多</div>
        </template>
      </div>

      <!-- 详情侧边栏：文件可编辑（星级/备注/标签），幻灯片只读 + 溯源跳转 -->
      <div v-if="selected" class="assets-detail">
        <div class="detail-head">
          <span class="detail-title" :title="detailTitle">{{ detailTitle }}</span>
          <span class="detail-close" @click="closeDetail">&#10005;</span>
        </div>

        <!-- 文件详情 -->
        <template v-if="selected.kind === 'file'">
          <template v-if="selected.fileEntry">
            <div class="detail-section">
              <div class="detail-row"><span class="detail-label">路径</span><span class="detail-value" :title="selected.fileEntry.path">{{ selected.fileEntry.path }}</span></div>
              <div class="detail-row"><span class="detail-label">大小</span><span class="detail-value">{{ formatSize(selected.fileEntry.size) }}</span></div>
              <div class="detail-row"><span class="detail-label">修改时间</span><span class="detail-value">{{ selected.fileEntry.modTime }}</span></div>
              <div class="detail-row">
                <span class="detail-label">索引状态</span>
                <span class="detail-value">
                  <el-tag size="small" :type="statusTagType(selected.fileEntry.status)">{{ statusLabel(selected.fileEntry.status) }}</el-tag>
                </span>
              </div>
            </div>
            <div class="detail-section">
              <div class="detail-label">星级</div>
              <el-rate v-model="detailDraft.rating" clearable />
            </div>
            <div class="detail-section">
              <div class="detail-label">标签</div>
              <el-select
                v-model="detailDraft.tags"
                class="detail-tags-select"
                placeholder="选择或输入新标签"
                multiple
                filterable
                allow-create
                default-first-option
                :loading="savingMeta"
              >
                <el-option v-for="tag in tagOptions" :key="tag.id" :label="tag.name" :value="tag.name" />
              </el-select>
            </div>
            <div class="detail-section">
              <div class="detail-label">备注</div>
              <el-input
                v-model="detailDraft.note"
                type="textarea"
                :rows="4"
                placeholder="记录素材用途、来源等备注"
              />
            </div>
            <div class="detail-actions">
              <el-button size="small" type="primary" :loading="savingMeta" @click="onSaveMeta">保存</el-button>
              <el-button size="small" :disabled="savingMeta" @click="resetDraft">重置</el-button>
            </div>
          </template>
          <el-alert v-else type="warning" :closable="false" title="文件索引记录不存在，请重新扫描后操作" />
        </template>

        <!-- 幻灯片详情：只读展示 + 归属文件溯源 + 继承标签只读标记 -->
        <template v-else>
          <div class="detail-section">
            <div class="detail-row"><span class="detail-label">页码</span><span class="detail-value">第 {{ detailSlideNum }} 页</span></div>
            <div class="detail-row"><span class="detail-label">归属文件</span><span class="detail-value" :title="detailFilePath">{{ detailFilePath }}</span></div>
          </div>
          <div v-if="detailSlideText" class="detail-section">
            <div class="detail-label">页文本</div>
            <div class="detail-pre">{{ detailSlideText }}</div>
          </div>
          <div v-if="detailSlideNotes" class="detail-section">
            <div class="detail-label">演讲者备注</div>
            <div class="detail-pre">{{ detailSlideNotes }}</div>
          </div>
          <div v-if="detailSlideSnippet" class="detail-section">
            <div class="detail-label">命中摘要</div>
            <div class="detail-pre">{{ detailSlideSnippet }}</div>
          </div>
          <div class="detail-section">
            <div class="detail-label">标签（继承自归属文件）</div>
            <div v-if="detailTags.length > 0" class="detail-inherit-tags">
              <span v-for="tag in detailTags" :key="tag.id" class="card-tag">
                <span class="tag-color-dot" :style="{ background: tag.color }"></span>{{ tag.name }}<span class="inherit-mark">继承</span>
              </span>
            </div>
            <span v-else class="detail-empty-hint">归属文件暂无标签</span>
          </div>
          <div class="detail-actions">
            <el-button size="small" type="primary" plain @click="traceToSourceFile">查看归属文件</el-button>
          </div>
        </template>
      </div>
    </div>

    <!-- 新建素材库弹窗：名称 + 素材根目录。
         目录支持两种填写方式：调 OpenDirectoryDialog 原生目录对话框选择回填，
         或保留路径输入框手动输入（E2E / 无窗口环境兜底能力） -->
    <el-dialog v-model="createDialogVisible" title="新建素材库" width="min(520px, 80vw)" :close-on-click-modal="false" append-to-body>
      <el-form label-width="88px">
        <el-form-item label="库名称">
          <el-input v-model="createForm.name" placeholder="例如：产品方案素材库" :disabled="creating" />
        </el-form-item>
        <el-form-item label="素材目录">
          <el-input v-model="createForm.rootPath" placeholder="素材根目录绝对路径，扫描只读不修改原始文件" :disabled="creating">
            <template #append>
              <el-button :disabled="creating" @click="onPickRootPath">选择目录</el-button>
            </template>
          </el-input>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button :disabled="creating" @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="onCreateLibrary">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ListAssetLibraries,
  CreateAssetLibrary,
  DeleteAssetLibrary,
  GetAssetLibraryStats,
  StartAssetScan,
  CancelAssetScan,
  RebuildAssetLibrary,
  ListAssetFiles,
  ListAssetSlides,
  UpdateAssetFileMeta,
  SearchAssets,
  OpenDirectoryDialog
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { handleError } from '../utils/error'
import AssetsIcon from './icons/AssetsIcon.vue'

// ---- 分页与检索参数（对齐后端 assetSearchDefaultLimit）----
const PAGE_SIZE = 50

// ---- 库状态 ----
const libraries = ref([])
const currentLibraryId = ref('')
const stats = ref(null)

// ---- 视图状态 ----
const viewMode = ref('file') // 'file' | 'slide'
const flatten = ref(false) // 检索聚合折叠/平铺开关（SearchAssets opts.flatten）
const keywordInput = ref('') // 输入框绑定值（未应用）
const activeKeyword = ref('') // 已应用关键词（非空即检索模式）
const selectedTagIds = ref([]) // 标签筛选（非空即检索模式）

// ---- 数据 ----
const fileEntries = ref([]) // 文件清单底册（文件视图数据源 + 标签聚合 + 溯源定位）
const slideEntries = ref([]) // 幻灯片清单（幻灯片视图数据源，按需加载）
const searchItems = ref([]) // 检索结果累计条目（懒加载追加）
const searchTotal = ref(0) // 检索总条目数（过滤/聚合后口径）
const loading = ref(false)
const loadingMore = ref(false)

// ---- 详情与编辑 ----
const selected = ref(null) // { kind: 'file'|'slide', fileEntry?, slideEntry?, hit? }
const detailDraft = ref({ rating: 0, note: '', tags: [] })
const savingMeta = ref(false)

// ---- 扫描状态 ----
const scanning = ref(false)
const scanProgress = ref(null) // assets-scan:progress 载荷 {libraryId, done, total, currentFile}

// ---- 聚合展开态 ----
const expandedAggPaths = ref([]) // 已展开聚合条目的文件路径集合
const aggChildren = ref({}) // 文件路径 -> 展开的逐页命中卡片
const aggLoading = ref('') // 正在加载命中页的文件路径

// ---- 新建库弹窗 ----
const createDialogVisible = ref(false)
const creating = ref(false)
const createForm = ref({ name: '', rootPath: '' })

const galleryRef = ref(null)

const hasLibrary = computed(() => !!currentLibrary.value)
const currentLibrary = computed(() => libraries.value.find(l => l.id === currentLibraryId.value) || null)

// 检索模式判定：关键词与标签任一非空（对齐后端「空查询返回空结果页」语义，
// 前端直接不发起空检索，回落清单浏览）
const searchMode = computed(() => activeKeyword.value.trim() !== '' || selectedTagIds.value.length > 0)

const scanPercent = computed(() => {
  if (!scanProgress.value || !scanProgress.value.total) return 0
  return Math.round((scanProgress.value.done / scanProgress.value.total) * 100)
})

// 标签选项：后端未提供独立标签列表接口，从当前库文件底册聚合去重
// （注释说明：接口补齐后可替换为独立数据源，聚合逻辑无消费方感知）
const tagOptions = computed(() => {
  const byId = new Map()
  for (const entry of fileEntries.value) {
    for (const tag of entry.tags || []) {
      if (!byId.has(tag.id)) byId.set(tag.id, tag)
    }
  }
  return Array.from(byId.values())
})

// ---- 卡片视图模型 ----
// 浏览模式卡片（清单数据），检索模式卡片（命中条目）统一为同构形态供画廊渲染
const browseCards = computed(() => {
  if (viewMode.value === 'file') {
    return fileEntries.value.map(entry => ({
      key: `file-${entry.id}`,
      kind: 'file',
      id: entry.id,
      title: entry.title,
      filePath: entry.path,
      status: entry.status,
      tags: entry.tags || [],
      hitCount: 1,
      slideNum: 0,
      snippet: '',
      entry
    }))
  }
  return slideEntries.value.map(slide => ({
    key: `slide-${slide.id}`,
    kind: 'slide',
    id: slide.id,
    fileId: slide.fileId,
    title: slide.title,
    filePath: slide.filePath,
    slideNum: slide.slideNum,
    tags: slide.tags || [],
    hitCount: 1,
    status: '',
    snippet: '',
    slide
  }))
})

// 检索模式卡片：按视图切换做类型过滤（检索结果天然含文件/幻灯片混合类型）
const searchCards = computed(() => {
  const want = viewMode.value === 'file' ? 'file' : 'slide'
  return searchItems.value
    .filter(card => card.kind === want)
})

const displayCards = computed(() => (searchMode.value ? searchCards.value : browseCards.value))

const canLoadMore = computed(() => searchMode.value && searchItems.value.length < searchTotal.value)

// ---- 详情面板展示计算 ----
const detailTitle = computed(() => {
  if (!selected.value) return ''
  if (selected.value.kind === 'file') {
    return selected.value.fileEntry ? selected.value.fileEntry.title : (selected.value.hit?.title || '')
  }
  return selected.value.slideEntry ? selected.value.slideEntry.title : (selected.value.hit?.title || '')
})
const detailSlideNum = computed(() => selected.value?.slideEntry?.slideNum ?? selected.value?.hit?.slideNum ?? 0)
const detailFilePath = computed(() => selected.value?.slideEntry?.filePath ?? selected.value?.hit?.filePath ?? '')
const detailSlideText = computed(() => selected.value?.slideEntry?.text || '')
const detailSlideNotes = computed(() => selected.value?.slideEntry?.speakerNotes || '')
const detailSlideSnippet = computed(() => selected.value?.hit?.snippet || '')
// 幻灯片标签为继承口径（本批幻灯片无自有标签），只读展示不提供编辑
const detailTags = computed(() => selected.value?.slideEntry?.tags || selected.value?.hit?.tags || [])

// ---- 索引状态展示映射（对齐 model/assets.go 状态机取值）----
const statusLabel = (status) => ({
  processing: '解析中',
  ok: '正常',
  failed: '解析失败',
  encrypted: '已加密',
  denied: '权限不足'
}[status] || status)
const statusTagType = (status) => ({
  processing: 'info',
  ok: 'success',
  failed: 'danger',
  encrypted: 'warning',
  denied: 'warning'
}[status] || 'info')

const formatSize = (bytes) => {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value % 1 === 0 ? value : value.toFixed(1)} ${units[unit]}`
}

// ================= 数据加载 =================

const loadLibraries = async () => {
  try {
    libraries.value = (await ListAssetLibraries()) || []
    // 当前库不在列表（被删除/首次加载）时回退首项
    if (!libraries.value.some(l => l.id === currentLibraryId.value)) {
      currentLibraryId.value = libraries.value[0]?.id || ''
      await onLibrarySelectionSettled()
    }
  } catch (error) {
    handleError('加载素材库列表失败: ', error)
  }
}

// 库选择落定后的统一加载：统计 + 文件底册 + 当前视图（清旧库残留数据防串库）
const onLibrarySelectionSettled = async () => {
  stats.value = null
  fileEntries.value = []
  slideEntries.value = []
  searchItems.value = []
  searchTotal.value = 0
  expandedAggPaths.value = []
  aggChildren.value = {}
  closeDetail()
  if (!hasLibrary.value) return
  await Promise.all([loadStats(), loadView()])
}

const loadStats = async () => {
  if (!hasLibrary.value) return
  try {
    stats.value = await GetAssetLibraryStats(currentLibraryId.value)
  } catch (error) {
    handleError('加载库统计失败: ', error)
  }
}

const onLibraryChange = () => onLibrarySelectionSettled()

// 当前视图数据加载：检索模式重跑检索（复位分页），浏览模式按视图拉清单
const loadView = async () => {
  if (searchMode.value) {
    await runSearch(true)
    return
  }
  loading.value = true
  try {
    if (viewMode.value === 'file') {
      fileEntries.value = (await ListAssetFiles(currentLibraryId.value)) || []
    } else {
      slideEntries.value = (await ListAssetSlides(currentLibraryId.value)) || []
    }
  } catch (error) {
    handleError('加载素材列表失败: ', error)
  } finally {
    loading.value = false
  }
}

// 文件底册加载：标签聚合 / 溯源定位 / 文件视图共用（库切换与扫描完成后均刷新）
const ensureFileEntries = async () => {
  if (fileEntries.value.length > 0) return
  try {
    fileEntries.value = (await ListAssetFiles(currentLibraryId.value)) || []
  } catch (error) {
    handleError('加载文件列表失败: ', error)
  }
}

// ================= 检索 =================

// 应用搜索条件：输入框值落定为 activeKeyword；关键词与标签全空则退出检索回浏览
const applySearch = async () => {
  activeKeyword.value = keywordInput.value.trim()
  closeDetail()
  expandedAggPaths.value = []
  aggChildren.value = {}
  if (!searchMode.value) {
    await loadView()
    return
  }
  await runSearch(true)
}

// 检索执行：reset=true 复位分页从头加载；false 追加下一页（懒加载）
const runSearch = async (reset) => {
  if (!hasLibrary.value) return
  if (reset) {
    loading.value = true
  } else {
    loadingMore.value = true
  }
  try {
    const page = await SearchAssets(currentLibraryId.value, activeKeyword.value.trim(), {
      flatten: flatten.value,
      tagIds: selectedTagIds.value,
      limit: PAGE_SIZE,
      offset: reset ? 0 : searchItems.value.length
    })
    const items = (page?.items || []).map(hit => ({
      key: `hit-${hit.refType}-${hit.refId}`,
      kind: hit.refType,
      id: hit.refId,
      fileId: hit.fileId,
      title: hit.title,
      filePath: hit.filePath,
      slideNum: hit.slideNum,
      snippet: hit.snippet,
      hitCount: hit.hitCount,
      status: '',
      tags: hit.tags || [],
      hit
    }))
    if (reset) {
      searchItems.value = items
    } else {
      searchItems.value = searchItems.value.concat(items)
    }
    searchTotal.value = page?.total || 0
  } catch (error) {
    handleError('检索失败: ', error)
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

// 画廊滚动到底部增量加载（仅检索模式分页；浏览模式为全量清单无分页）
const onGalleryScroll = () => {
  const el = galleryRef.value
  if (!el || !searchMode.value || loadingMore.value || loading.value || !canLoadMore.value) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 80) {
    runSearch(false)
  }
}

const onFlattenChange = () => {
  // 平铺开关只影响检索聚合形态；展开态随形态切换失效
  expandedAggPaths.value = []
  aggChildren.value = {}
  if (searchMode.value) {
    runSearch(true)
  }
}

const onViewModeChange = () => {
  // 视图切换：检索模式为客户端类型过滤（displayCards 已响应），浏览模式需拉对应清单
  closeDetail()
  if (!searchMode.value && viewMode.value === 'slide') {
    loadView()
  }
}

// ---- 聚合条目展开/收起：按归属文件路径懒加载逐页命中（flatten 检索后客户端过滤）----
const isAggExpanded = (filePath) => expandedAggPaths.value.includes(filePath)

const toggleAgg = async (filePath) => {
  if (isAggExpanded(filePath)) {
    expandedAggPaths.value = expandedAggPaths.value.filter(p => p !== filePath)
    return
  }
  expandedAggPaths.value = expandedAggPaths.value.concat(filePath)
  if (aggChildren.value[filePath]) return
  aggLoading.value = filePath
  try {
    const page = await SearchAssets(currentLibraryId.value, activeKeyword.value.trim(), {
      flatten: true,
      tagIds: selectedTagIds.value,
      limit: 200,
      offset: 0
    })
    const children = (page?.items || [])
      .filter(hit => hit.refType === 'slide' && hit.filePath === filePath)
      .map(hit => ({
        key: `hit-slide-${hit.refId}`,
        kind: 'slide',
        id: hit.refId,
        fileId: hit.fileId,
        title: hit.title,
        filePath: hit.filePath,
        slideNum: hit.slideNum,
        snippet: hit.snippet,
        hitCount: 1,
        status: '',
        tags: hit.tags || [],
        hit
      }))
    aggChildren.value = { ...aggChildren.value, [filePath]: children }
  } catch (error) {
    handleError('加载命中页失败: ', error)
  } finally {
    aggLoading.value = ''
  }
}

// ================= 详情与编辑 =================

const isCardSelected = (card) => {
  if (!selected.value) return false
  if (card.kind !== selected.value.kind) return false
  return card.kind === 'file'
    ? selected.value.fileEntry?.id === card.id
    : (selected.value.slideEntry?.id === card.id || selected.value.hit?.refId === card.id)
}

const onCardClick = async (card) => {
  // 聚合条目：点击切换展开/收起，不进详情
  if (card.hitCount > 1) {
    await toggleAgg(card.filePath)
    return
  }
  if (card.kind === 'file') {
    // 检索命中的文件条目按 fileId 解析完整底册记录（含星级/备注，供编辑）；
    // 底册未加载时经 resolveFileEntry 兜底拉取（直接查空底册会误报「记录不存在」）
    const entry = card.entry || (await resolveFileEntry(card.fileId))
    selected.value = { kind: 'file', fileEntry: entry, hit: card.entry ? null : card.hit }
    if (entry) resetDraft()
  } else {
    const slide = card.slide || slideEntries.value.find(s => s.id === card.id) || null
    selected.value = { kind: 'slide', slideEntry: slide, hit: card.slide ? null : card.hit }
  }
}

const closeDetail = () => {
  selected.value = null
  savingMeta.value = false
}

const resetDraft = () => {
  const entry = selected.value?.fileEntry
  detailDraft.value = {
    rating: entry?.rating || 0,
    note: entry?.note || '',
    tags: (entry?.tags || []).map(t => t.name)
  }
}

// 检索命中文件而底册未加载时的兜底解析（桌面量级一次全量拉取可接受）
const resolveFileEntry = async (fileId) => {
  await ensureFileEntries()
  return fileEntries.value.find(f => f.id === fileId) || null
}

// 保存文件级元数据（整体替换语义：星级/备注/标签以本次提交值为准）
const onSaveMeta = async () => {
  const entry = selected.value?.fileEntry
  if (!entry) return
  savingMeta.value = true
  try {
    await UpdateAssetFileMeta(currentLibraryId.value, entry.id, {
      rating: detailDraft.value.rating,
      note: detailDraft.value.note,
      tags: detailDraft.value.tags
    })
    ElMessage.success('已保存')
    // 保存后重拉文件底册：取回含新建标签完整对象（id/颜色）的规范数据，
    // 标签聚合选项随之刷新；详情引用同步指向新条目
    fileEntries.value = (await ListAssetFiles(currentLibraryId.value)) || []
    const updated = fileEntries.value.find(f => f.id === entry.id) || null
    if (updated && selected.value?.fileEntry?.id === entry.id) {
      selected.value = { ...selected.value, fileEntry: updated }
    }
    resetDraft()
    if (searchMode.value) {
      await runSearch(true)
    }
  } catch (error) {
    handleError('保存失败: ', error)
  } finally {
    savingMeta.value = false
  }
}

// 溯源：幻灯片条目跳转归属文件——退出检索态切文件视图并选中该文件条目
const traceToSourceFile = async () => {
  const fileId = selected.value?.slideEntry?.fileId ?? selected.value?.hit?.fileId
  if (!fileId) return
  activeKeyword.value = ''
  keywordInput.value = ''
  selectedTagIds.value = []
  viewMode.value = 'file'
  closeDetail()
  loading.value = true
  try {
    await ensureFileEntries()
    const entry = fileEntries.value.find(f => f.id === fileId) || null
    if (entry) {
      selected.value = { kind: 'file', fileEntry: entry, hit: null }
      resetDraft()
    } else {
      ElMessage.warning('归属文件的索引记录不存在，请重新扫描')
    }
  } finally {
    loading.value = false
  }
  await nextTick()
}

// ================= 库管理 =================

const openCreateDialog = () => {
  createForm.value = { name: '', rootPath: '' }
  createDialogVisible.value = true
}

// 原生目录对话框选择素材根目录，成功后回填输入框（取消返回空串不覆盖已有值；
// 手动输入能力保留，兜底 E2E / 无窗口环境——对话框方法在 serve 无头模式被后端拒绝）
const onPickRootPath = async () => {
  try {
    const dir = await OpenDirectoryDialog('选择素材根目录')
    if (dir) {
      createForm.value.rootPath = dir
    }
  } catch (error) {
    handleError('选择目录失败: ', error)
  }
}

const onCreateLibrary = async () => {
  const name = createForm.value.name.trim()
  const rootPath = createForm.value.rootPath.trim()
  if (!name) {
    ElMessage.warning('请输入素材库名称')
    return
  }
  if (!rootPath) {
    ElMessage.warning('请输入素材根目录路径')
    return
  }
  creating.value = true
  try {
    const lib = await CreateAssetLibrary(name, rootPath)
    ElMessage.success('素材库已创建，点击「扫描」建立索引')
    createDialogVisible.value = false
    libraries.value = libraries.value.concat(lib)
    currentLibraryId.value = lib.id
    await onLibrarySelectionSettled()
  } catch (error) {
    handleError('新建素材库失败: ', error)
  } finally {
    creating.value = false
  }
}

const onDeleteLibrary = async () => {
  const lib = currentLibrary.value
  if (!lib) return
  try {
    await ElMessageBox.confirm(
      `确定删除素材库「${lib.name}」？索引数据将被删除，原始文件不受影响。`,
      '删除素材库',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  try {
    await DeleteAssetLibrary(lib.id)
    ElMessage.success('素材库已删除')
    libraries.value = libraries.value.filter(l => l.id !== lib.id)
    currentLibraryId.value = libraries.value[0]?.id || ''
    await onLibrarySelectionSettled()
  } catch (error) {
    handleError('删除素材库失败: ', error)
  }
}

// ================= 扫描 / 重建 / 取消 =================

const onScan = async () => {
  if (!hasLibrary.value || scanning.value) return
  scanning.value = true
  scanProgress.value = null
  try {
    const result = await StartAssetScan(currentLibraryId.value)
    ElMessage.success(scanResultSummary('扫描完成', result))
    await refreshAfterScan()
  } catch (error) {
    handleError('扫描失败: ', error)
  } finally {
    scanning.value = false
    scanProgress.value = null
  }
}

const onRebuild = async () => {
  const lib = currentLibrary.value
  if (!lib || scanning.value) return
  try {
    await ElMessageBox.confirm(
      '重建将清空当前库的全部索引记录并重新扫描（文件标签按路径保留，星级与备注不保留）。确定继续？',
      '重建索引',
      { confirmButtonText: '重建', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  scanning.value = true
  scanProgress.value = null
  try {
    const result = await RebuildAssetLibrary(lib.id)
    ElMessage.success(scanResultSummary('重建完成', result))
    await refreshAfterScan()
  } catch (error) {
    handleError('重建索引失败: ', error)
  } finally {
    scanning.value = false
    scanProgress.value = null
  }
}

const onCancelScan = async () => {
  if (!hasLibrary.value) return
  try {
    const requested = await CancelAssetScan(currentLibraryId.value)
    if (requested) {
      ElMessage.info('已请求取消扫描')
    } else {
      ElMessage.warning('当前没有可取消的扫描')
    }
  } catch (error) {
    handleError('取消扫描失败: ', error)
  }
}

// 扫描结果摘要文案（新增/更新/跳过口径对齐 model.AssetScanResult）
const scanResultSummary = (prefix, result) => {
  if (!result) return prefix
  const parts = [`新增 ${result.new ?? 0}`, `更新 ${result.updated ?? 0}`, `未变 ${result.unchanged ?? 0}`]
  if (result.failed) parts.push(`失败 ${result.failed}`)
  if (result.encrypted) parts.push(`加密 ${result.encrypted}`)
  if (result.denied) parts.push(`权限不足 ${result.denied}`)
  if (result.removed) parts.push(`清理 ${result.removed}`)
  return `${prefix}：共 ${result.total ?? 0} 个文件（${parts.join('，')}）`
}

// 扫描/重建完成后刷新：统计 + 文件底册 + 当前视图（清聚合展开态与详情防陈旧）
const refreshAfterScan = async () => {
  expandedAggPaths.value = []
  aggChildren.value = {}
  closeDetail()
  fileEntries.value = []
  slideEntries.value = []
  searchItems.value = []
  searchTotal.value = 0
  await Promise.all([loadStats(), loadView()])
}

// ================= 扫描事件（assets-scan:*）=================
// EventsOn 返回注销闭包，onBeforeUnmount 精准注销本组件监听器（禁 EventsOff 全局移除）
let offProgress = null
let offDone = null
let offError = null

const onScanProgress = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  scanProgress.value = payload
}

const onScanDone = async (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 终态数据刷新（提示文案由扫描 Promise 统一给出，此处不弹消息防双弹）
  await refreshAfterScan()
}

const onScanError = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 仅复位进度展示；错误提示由扫描 Promise 的 handleError 统一分流（防双弹）
  scanProgress.value = null
}

onMounted(() => {
  // 先注册事件监听再异步加载数据：保证卸载路径必能拿到全部 off 闭包
  offProgress = EventsOn('assets-scan:progress', onScanProgress)
  offDone = EventsOn('assets-scan:done', onScanDone)
  offError = EventsOn('assets-scan:error', onScanError)
  loadLibraries()
})

onBeforeUnmount(() => {
  offProgress && offProgress()
  offDone && offDone()
  offError && offError()
  offProgress = null
  offDone = null
  offError = null
})

defineExpose({ loadLibraries, refreshAfterScan })
</script>

<style scoped>
/* 根元素：占满 Home 主区上半区（与 .main-panes 互斥），完整 flex 约束链防塌陷挤压终端 */
.assets-panel {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg-primary);
}

/* ---- 顶部工具条 ---- */
.assets-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-secondary);
}

.assets-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.library-select {
  width: 200px;
}

/* ---- 扫描进度 ---- */
.scan-progress {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-md);
  background: var(--primary-bg);
  border-bottom: 1px solid var(--border-color);
}

.scan-progress-bar {
  width: 220px;
}

.scan-progress-text {
  font-size: 12px;
  color: var(--text-primary);
  white-space: nowrap;
}

.scan-progress-file {
  font-size: 12px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

/* ---- 搜索/筛选区 ---- */
.assets-toolbar {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
}

.keyword-input {
  width: 280px;
}

.tag-filter {
  width: 220px;
}

.flatten-switch {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
}

/* ---- 库统计行 ---- */
.assets-stats-line {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-xs) var(--spacing-md);
  font-size: 12px;
  color: var(--text-tertiary);
  border-bottom: 1px solid var(--border-color);
}

.stats-warn {
  color: var(--warning-color);
}

.stats-hit {
  color: var(--primary-color);
  font-weight: 500;
}

/* ---- 主体：画廊 + 详情 ---- */
.assets-body {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

.assets-gallery {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: var(--spacing-md);
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: var(--spacing-sm);
  align-content: start;
}

/* 空态（el-empty）占满整行避免被网格切分 */
.assets-gallery > .el-empty {
  grid-column: 1 / -1;
  margin: auto;
}

.load-more-hint {
  grid-column: 1 / -1;
  text-align: center;
  font-size: 12px;
  color: var(--text-placeholder);
  padding: var(--spacing-sm) 0;
}

.asset-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-sm);
  padding: var(--spacing-sm);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.asset-card:hover {
  border-color: var(--primary-light);
  box-shadow: var(--shadow-md);
}

.asset-card.is-selected {
  border-color: var(--primary-color);
  background: var(--primary-bg);
}

.asset-card-head {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.asset-card-page {
  font-size: 12px;
  color: var(--text-tertiary);
  white-space: nowrap;
}

.asset-card-hit {
  font-size: 12px;
  font-weight: 500;
  color: var(--primary-color);
  white-space: nowrap;
}

.asset-card-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.asset-card-sub {
  font-size: 12px;
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.asset-card-snippet {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.asset-card-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

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

/* 标签色圆点：颜色为后端下发的数据值，动态绑定呈现 */
.tag-color-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.tag-option-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

/* ---- 聚合展开子页 ---- */
.agg-children {
  border-top: 1px dashed var(--border-color);
  padding-top: var(--spacing-xs);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.agg-loading {
  font-size: 12px;
  color: var(--text-placeholder);
}

.agg-child {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: 12px;
  padding: var(--spacing-xs);
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.agg-child:hover {
  background: var(--bg-tertiary);
}

.agg-child-page {
  color: var(--primary-color);
  white-space: nowrap;
  flex-shrink: 0;
}

.agg-child-title {
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 0;
  max-width: 45%;
}

.agg-child-snippet {
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

/* ---- 详情侧边栏 ---- */
.assets-detail {
  flex-shrink: 0;
  width: 300px;
  border-left: 1px solid var(--border-color);
  background: var(--bg-secondary);
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.detail-head {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--border-color);
}

.detail-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-close {
  font-size: 14px;
  color: var(--text-tertiary);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  transition: all var(--transition-normal);
}

.detail-close:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

.detail-section {
  padding: var(--spacing-sm) var(--spacing-md);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  border-bottom: 1px solid var(--border-color);
}

.detail-row {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
}

.detail-label {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--text-tertiary);
}

.detail-value {
  font-size: 12px;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.detail-tags-select {
  width: 100%;
}

.detail-pre {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 160px;
  overflow-y: auto;
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
}

.detail-inherit-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
}

.inherit-mark {
  font-size: 10px;
  color: var(--info-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  padding: 0 2px;
  margin-left: 2px;
}

.detail-empty-hint {
  font-size: 12px;
  color: var(--text-placeholder);
}

.detail-actions {
  padding: var(--spacing-md);
  display: flex;
  gap: var(--spacing-sm);
}
</style>
