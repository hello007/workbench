<!-- frontend/src/components/AssetsPanel.vue -->
<!-- 素材管理面板（活动栏一级入口）：本地 DAM 素材索引库 MVP 前端。
     数据链路：库切换（ListAssetLibraries）→ 文件清单（ListAssetFiles，兼作标签聚合
     与幻灯片溯源的文件底册）+ 幻灯片清单（ListAssetSlides，limit/offset 分页懒加载）
     → 关键词/标签检索（SearchAssets，分页懒加载 + 同 PPT 聚合折叠）→ 文件级元数据
     编辑（UpdateAssetFileMeta 整体替换）→ 扫描/重建（StartAssetScan/RebuildAssetLibrary，
     进度经 assets-scan:* 事件推送、可取消；done 事件仅复位进度，数据刷新由扫描
     Promise 成功分支统一执行防双拉取）。缩略图链路（PR3）：画廊卡片 tier=0 可视区
     懒加载 + 详情侧边栏 tier=1 单张按需（AssetThumb 子组件封装），批量生成进度经
     assets-thumbs:* 事件推送（progress 收集失败文件、done 复位并自增失效代次触发
     miss 卡补图、error 仅复位不弹提示）、可取消；本机无 PowerPoint 时降级提示条
     常驻、跳转入口隐藏。
     异步写回守卫：库代次（librarySeq）与检索代次（searchSeq）双序号，await 写回前
     比对，不一致丢弃旧响应（防切库在途/改词重搜的慢响应跨语境串入）。
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
        <el-button size="small" @click="openThumbSettings">缩略图设置</el-button>
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

    <!-- 缩略图生成进度条：仅批次生成中可见；与扫描进度条同层并存（各自 v-if、内容
         不同）；进度载荷按库 ID 过滤（他库事件不串入），视觉复用扫描进度条模式 -->
    <div v-if="thumbsRunning" class="scan-progress thumbs-progress">
      <el-progress
        class="scan-progress-bar"
        :percentage="thumbsPercent"
        :stroke-width="8"
        :show-text="false"
      />
      <span class="scan-progress-text">
        {{ thumbsProgress ? `生成缩略图 ${thumbsProgress.done}/${thumbsProgress.total}` : '正在准备生成缩略图…' }}
      </span>
      <span v-if="thumbsProgress && thumbsProgress.currentFile" class="scan-progress-file" :title="thumbsProgress.currentFile">
        {{ thumbsProgress.currentFile }}
      </span>
      <el-button size="small" text type="danger" @click="onCancelThumbs">取消生成</el-button>
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

    <!-- 缩略图能力降级提示条：本机无 PowerPoint（探测不可用/探测失败）时常驻提示，
         信息样式非 error 红——环境缺失属正常降级非故障 -->
    <div v-if="hasLibrary && thumbsAvailable === false" class="thumbs-degrade-banner">
      未检测到 PowerPoint，仅文本索引，缩略图与跳转不可用
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
        <!-- 空态三：检索无结果（命中存在但被当前视图类型过滤时给切换引导） -->
        <el-empty v-else-if="searchMode && !loading && displayCards.length === 0" :description="searchEmptyText" />
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
            <!-- 缩略图：画廊一律 tier=0（small 档）可视区懒加载；文件卡取第 1 页封面 -->
            <AssetThumb
              :library-id="currentLibraryId"
              :file-id="card.kind === 'file' ? card.id : card.fileId"
              :slide-num="card.kind === 'slide' ? card.slideNum : 1"
              :tier="0"
              :epoch="thumbEpoch"
              class="asset-card-thumb"
            />
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
              <div v-if="aggLoadingPaths.includes(card.filePath)" class="agg-loading">加载命中页…</div>
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

        <!-- 高清大图：文件与幻灯片两形态共用区块；hd（tier=1）单张按需加载，
             未生成时整块隐藏（hide-on-miss）不占位；失败标记按归属文件路径命中
             批次失败集合显示重试入口 -->
        <div class="detail-section detail-thumb-section">
          <AssetThumb
            :library-id="currentLibraryId"
            :file-id="detailFileId"
            :slide-num="detailSlideNum || 1"
            :tier="1"
            :epoch="thumbEpoch"
            hide-on-miss
            class="detail-thumb"
          />
          <div v-if="isDetailThumbFailed" class="detail-thumb-failed">
            <el-alert type="warning" :closable="false" title="缩略图生成失败（超时/文件被占用/文件损坏）" />
            <el-button size="small" text type="primary" :loading="retrying" @click="onRetryThumb">重试生成</el-button>
          </div>
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
            <!-- 打开跳转：仅本机 PowerPoint 可用时提供（降级态隐藏入口，与提示条联动） -->
            <el-button
              v-if="thumbsAvailable"
              size="small"
              type="primary"
              plain
              :loading="jumping"
              @click="onJumpToSlide"
            >打开并跳到此页</el-button>
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

    <!-- 缩略图设置弹窗：assets 段独立设置（分辨率变更后端自动清空对应档位缓存并
         重新生成；上限变更后台自动 LRU 淘汰）。保存为整体提交：rawSettings 回填
         assets 段，其余设置段原样保留 -->
    <el-dialog
      v-model="thumbSettingsVisible"
      title="缩略图设置"
      width="min(520px, 90vw)"
      :close-on-click-modal="false"
      append-to-body
    >
      <el-form label-width="110px">
        <el-form-item label="自动生成">
          <el-switch v-model="thumbSettingsDraft.auto" />
        </el-form-item>
        <el-form-item label="小图分辨率">
          <el-input-number
            v-model="thumbSettingsDraft.smallWidth"
            :min="120"
            :max="800"
            :step="40"
            :value-on-clear="320"
          />
          <span class="settings-unit">px</span>
        </el-form-item>
        <el-form-item label="高清分辨率">
          <el-input-number
            v-model="thumbSettingsDraft.hdWidth"
            :min="480"
            :max="3840"
            :step="160"
            :value-on-clear="1600"
          />
          <span class="settings-unit">px</span>
        </el-form-item>
        <el-form-item label="小图缓存上限">
          <el-input-number
            v-model="thumbSettingsDraft.smallQuotaMB"
            :min="64"
            :max="8192"
            :step="64"
            :value-on-clear="1024"
          />
          <span class="settings-unit">MB</span>
        </el-form-item>
        <el-form-item label="高清缓存上限">
          <el-input-number
            v-model="thumbSettingsDraft.hdQuotaMB"
            :min="64"
            :max="8192"
            :step="64"
            :value-on-clear="1024"
          />
          <span class="settings-unit">MB</span>
        </el-form-item>
      </el-form>
      <div class="thumb-settings-hint">
        修改分辨率将清空对应档位缓存并自动重新生成；修改上限后后台自动淘汰最久未用的缩略图
      </div>
      <template #footer>
        <el-button :disabled="thumbSettingsSaving" @click="thumbSettingsVisible = false">取消</el-button>
        <el-button type="primary" :loading="thumbSettingsSaving" @click="onSaveThumbSettings">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
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
  OpenDirectoryDialog,
  AssetThumbsAvailable,
  JumpToAssetSlide,
  RetryAssetThumb,
  CancelAssetThumbs,
  GetSettings,
  SaveSettings
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { handleError, ErrorCode } from '../utils/error'
import AssetsIcon from './icons/AssetsIcon.vue'
import AssetThumb from './AssetThumb.vue'

// ---- 分页与检索参数（对齐后端 assetSearchDefaultLimit）----
const PAGE_SIZE = 50

// ---- 缩略图设置默认值（与后端 model.AssetThumbsSettings 默认一致；老配置 assets
// 段缺失/字段零值时逐字段回退，防 el-input-number 绑到 undefined/null 脏值）----
const THUMB_SETTINGS_DEFAULTS = {
  auto: true,
  smallWidth: 320,
  hdWidth: 1600,
  smallQuotaMB: 1024,
  hdQuotaMB: 1024
}

// ---- 异步写回守卫（跨库串入防护，参照 useCommandPalette searchVersion 模式）----
// librarySeq：库数据代次，库切换落定 / 扫描刷新（resetAndReloadView）时自增；
// 统计与清单写回前比对，代次已变（期间又切库/刷新）即丢弃旧响应。
// searchSeq：检索代次，语境变更点（applySearch/切库/扫描刷新/平铺切换/退出检索）
// 自增；检索结果写回前比对，不一致丢弃——防慢响应把旧库/旧词结果串入新语境。
let librarySeq = 0
let searchSeq = 0

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
const slideEntries = ref([]) // 幻灯片清单累计条目（幻灯片视图数据源，分页懒加载追加）
const slideTotal = ref(0) // 幻灯片清单总条目数（分页懒加载判定，对齐后端 ListAssetSlides total）
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

// ---- 缩略图状态（assets-thumbs:*）----
const thumbsAvailable = ref(null) // null=探测中，false=降级（提示条常驻），true=可用
const thumbsRunning = ref(false)
const thumbsProgress = ref(null) // assets-thumbs:progress 载荷 {libraryId, done, total, currentFile, failed}
const thumbEpoch = ref(0) // 失效代次：done 事件自增，触发 miss 缩略图自动补图
const failedThumbPaths = ref([]) // 批次内失败文件相对路径集合（数组当 Set 用，避免响应式 Set 兼容问题）
let lastThumbsFailedCount = 0 // 上次 progress 的累计失败数（非响应式，仅增量比对）
const jumping = ref(false)
const retrying = ref(false)

// ---- 缩略图设置弹窗 ----
const thumbSettingsVisible = ref(false)
const thumbSettingsSaving = ref(false)
const thumbSettingsDraft = ref({ ...THUMB_SETTINGS_DEFAULTS })
const rawSettings = ref(null) // GetSettings 返回的完整设置对象（保存时回填 assets 段整体提交，避免覆盖其他设置段）

// ---- 聚合展开态 ----
const expandedAggPaths = ref([]) // 已展开聚合条目的文件路径集合
const aggChildren = ref({}) // 文件路径 -> 展开的逐页命中卡片
const aggLoadingPaths = ref([]) // 正在加载命中页的文件路径集合（Set 语义，并发展开互不覆盖）

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

const thumbsPercent = computed(() => {
  if (!thumbsProgress.value || !thumbsProgress.value.total) return 0
  return Math.round((thumbsProgress.value.done / thumbsProgress.value.total) * 100)
})

// 切库复位缩略图运行态：done/error 事件按 libraryId 过滤，批次运行中切库后旧库
// 终态事件不会命中新语境——不复位则进度条在新库页面永挂，且「取消生成」打到新库
// 恒返 false 与可见进度条自相矛盾。失败集合为旧库相对路径口径，一并清空防误归因
// （切回原库后由下一批 progress/done 重新收集，重试链路可恢复）。
watch(currentLibraryId, () => {
  thumbsRunning.value = false
  thumbsProgress.value = null
  lastThumbsFailedCount = 0
  failedThumbPaths.value = []
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

// 浏览模式幻灯片视图的分页懒加载判定（文件视图为全量清单无分页）
const canLoadMoreSlides = computed(() =>
  !searchMode.value && viewMode.value === 'slide' && slideEntries.value.length < slideTotal.value
)

// 检索空态文案：命中存在但当前视图类型过滤后为空时给引导。审核 11 的最小改法
// 取舍——选引导文案而非把 searchTotal 改为按视图过滤口径：searchTotal 同时驱动
// 懒加载判定（canLoadMore 依赖后端混合类型 total），改口径会破坏分页判定；切换
// 视图 / 开启平铺即可见全部命中，文案引导直达。
const searchEmptyText = computed(() => {
  if (searchTotal.value > 0) {
    return `已命中 ${searchTotal.value} 条，但当前视图类型下没有可展示条目，试试切换文件/幻灯片视图或开启「平铺」`
  }
  return '没有匹配的素材，换个关键词或筛选条件试试'
})

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

// 详情条目对应的文件 ID 兜底链：文件详情取底册 id，幻灯片/检索命中取 fileId
//（缩略图缓存键以 files.id 为准，0 = 无底册记录不发请求）
const detailFileId = computed(
  () => selected.value?.fileEntry?.id ?? selected.value?.slideEntry?.fileId ?? selected.value?.hit?.fileId ?? 0
)

// 缩略图失败标记/重试/跳转的路径口径：文件详情取底册 path，幻灯片/检索命中取 filePath
const detailThumbPath = computed(
  () => selected.value?.fileEntry?.path ?? selected.value?.slideEntry?.filePath ?? selected.value?.hit?.filePath ?? ''
)

// 当前详情条目的缩略图是否在本批次生成失败（按归属文件路径命中失败集合）
const isDetailThumbFailed = computed(() => {
  const path = detailThumbPath.value
  return !!path && failedThumbPaths.value.includes(path)
})

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

// 库不存在（已在别处删除、前端列表过期）判定：自愈路径静默重拉库列表并重新
// 落定剩余库，不弹错误（error.js 对 E_ASSETS_LIB_NOT_FOUND 的行为承诺落地）
const isLibNotFound = (error) => error?.code === ErrorCode.AssetsLibNotFound

// 库代次守卫判定：seq 为 null 的调用方（无守卫语境）恒允许写回；否则须与当前
// 库代次一致（期间又切库/刷新即丢弃旧响应）
const librarySeqAlive = (seq) => seq === null || seq === librarySeq

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

// 库选择落定后的统一加载（库切换 / 新建库 / 删除库回退共用入口）
const onLibrarySelectionSettled = () => resetAndReloadView()

// 库数据整体重载（库切换落定 / 扫描重建完成共用）：清全部视图状态 + 并行拉取
// 统计 / 当前视图 / 文件底册。底册是标签聚合（tagOptions）与溯源定位的共用数据
// 源，幻灯片/检索视图下也预载，防标签筛选选项恒空与点击兜底反复触发；文件浏览
// 视图下由 loadView 自带底册加载，不重复拉取。seq 守卫：期间再次切库/刷新时旧
// 库响应全部丢弃（审核 8：跨库串入防护）。
const resetAndReloadView = async () => {
  const seq = ++librarySeq
  searchSeq++ // 语境变更点：在途检索/旧库清单写回全部失效
  stats.value = null
  fileEntries.value = []
  slideEntries.value = []
  slideTotal.value = 0
  searchItems.value = []
  searchTotal.value = 0
  expandedAggPaths.value = []
  aggChildren.value = {}
  aggLoadingPaths.value = []
  closeDetail()
  if (!hasLibrary.value) return
  const fileViewLoadedHere = viewMode.value === 'file' && !searchMode.value
  const tasks = [loadStats(seq), loadView(seq)]
  if (!fileViewLoadedHere) tasks.push(ensureFileEntries(seq))
  await Promise.all(tasks)
}

const loadStats = async (seq = null) => {
  if (!hasLibrary.value) return
  try {
    const result = await GetAssetLibraryStats(currentLibraryId.value)
    if (!librarySeqAlive(seq)) return // 库代次已变：旧库统计丢弃防串库
    stats.value = result
  } catch (error) {
    if (!librarySeqAlive(seq)) return
    if (isLibNotFound(error)) {
      await loadLibraries()
      return
    }
    handleError('加载库统计失败: ', error)
  }
}

const onLibraryChange = () => onLibrarySelectionSettled()

// 当前视图数据加载：检索模式重跑检索（复位分页），浏览模式按视图拉清单
// （文件视图全量、幻灯片视图分页首页）
const loadView = async (seq = null) => {
  if (searchMode.value) {
    await runSearch(true)
    return
  }
  loading.value = true
  try {
    if (viewMode.value === 'file') {
      const entries = await ListAssetFiles(currentLibraryId.value)
      if (!librarySeqAlive(seq)) return
      fileEntries.value = entries || []
    } else {
      await loadSlidesPage(true, seq)
    }
  } catch (error) {
    if (!librarySeqAlive(seq)) return
    if (isLibNotFound(error)) {
      await loadLibraries()
      return
    }
    handleError('加载素材列表失败: ', error)
  } finally {
    loading.value = false
  }
}

// 幻灯片清单分页加载（后端分页契约对齐 SearchAssets：limit/offset 驱动翻页，
// total 恒为总条目数）。reset=true 拉首页并复位累计；false 追加下一页（懒加载）。
// seq 库代次守卫：写回前比对，期间切库即丢弃旧库响应；未显式传代的调用点
// （画廊滚动追加）在调用时快照当前库代次，滚动加载在途切库同样被拦截。
const loadSlidesPage = async (reset, seq = null) => {
  if (seq === null) seq = librarySeq
  // 追加页在途置 loadingMore：滚动事件在途窗口内连续触发时被 onGalleryScroll
  // 的防重检查拦截（offset 以已加载条目数计算，重复触发会追加重复条目）
  if (!reset) loadingMore.value = true
  try {
    const page = await ListAssetSlides(
      currentLibraryId.value,
      PAGE_SIZE,
      reset ? 0 : slideEntries.value.length
    )
    if (!librarySeqAlive(seq)) return
    const items = page?.items || []
    slideEntries.value = reset ? items : slideEntries.value.concat(items)
    slideTotal.value = page?.total || 0
  } catch (error) {
    if (!librarySeqAlive(seq)) return
    if (isLibNotFound(error)) {
      await loadLibraries()
      return
    }
    handleError('加载幻灯片列表失败: ', error)
  } finally {
    if (!reset) loadingMore.value = false
  }
}

// 文件底册加载：标签聚合 / 溯源定位 / 文件视图共用（库切换与扫描完成后均刷新）。
// seq 未显式传代（如溯源点击）时在调用时快照当前库代次，await 期间切库写回被拦截。
const ensureFileEntries = async (seq = null) => {
  if (seq === null) seq = librarySeq
  if (fileEntries.value.length > 0) return
  try {
    const entries = await ListAssetFiles(currentLibraryId.value)
    if (!librarySeqAlive(seq)) return
    fileEntries.value = entries || []
  } catch (error) {
    if (!librarySeqAlive(seq)) return
    if (isLibNotFound(error)) {
      await loadLibraries()
      return
    }
    handleError('加载文件列表失败: ', error)
  }
}

// ================= 检索 =================

// 应用搜索条件：输入框值落定为 activeKeyword；关键词与标签全空则退出检索回浏览
const applySearch = async () => {
  searchSeq++ // 语境变更点：改词/改筛选使在途检索写回失效
  activeKeyword.value = keywordInput.value.trim()
  closeDetail()
  expandedAggPaths.value = []
  aggChildren.value = {}
  aggLoadingPaths.value = []
  if (!searchMode.value) {
    await loadView()
    return
  }
  await runSearch(true)
}

// 检索执行：reset=true 复位分页从头加载；false 追加下一页（懒加载）。
// 检索代次守卫：语境变更点已自增 searchSeq，此处捕获当前值，写回前比对，不一致
// （期间改词重搜/切库/刷新）丢弃旧响应，防慢响应把旧语境结果串入新视图。
const runSearch = async (reset) => {
  if (!hasLibrary.value) return
  if (reset) {
    loading.value = true
  } else {
    loadingMore.value = true
  }
  const capturedSeq = searchSeq
  try {
    const page = await SearchAssets(currentLibraryId.value, activeKeyword.value.trim(), {
      flatten: flatten.value,
      tagIds: selectedTagIds.value,
      limit: PAGE_SIZE,
      offset: reset ? 0 : searchItems.value.length
    })
    if (searchSeq !== capturedSeq) return // 语境已变：旧检索响应丢弃
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
    if (searchSeq !== capturedSeq) return // 语境已变：旧检索失败静默丢弃（错误也属旧语境）
    handleError('检索失败: ', error)
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

// 画廊滚动到底部增量加载：检索模式加载检索下一页；浏览模式幻灯片视图加载清单
// 下一页（文件视图为全量清单无分页）
const onGalleryScroll = () => {
  const el = galleryRef.value
  if (!el || loading.value) return
  if (el.scrollTop + el.clientHeight < el.scrollHeight - 80) return
  if (searchMode.value) {
    if (!loadingMore.value && canLoadMore.value) runSearch(false)
  } else if (viewMode.value === 'slide' && !loadingMore.value && canLoadMoreSlides.value) {
    loadSlidesPage(false)
  }
}

const onFlattenChange = () => {
  // 平铺开关只影响检索聚合形态；展开态随形态切换失效
  searchSeq++ // 语境变更点：聚合形态变化使在途检索写回失效
  expandedAggPaths.value = []
  aggChildren.value = {}
  aggLoadingPaths.value = []
  if (searchMode.value) {
    runSearch(true)
  }
}

const onViewModeChange = () => {
  // 视图切换：检索模式为客户端类型过滤（displayCards 已响应）；浏览模式目标清单
  // 为空即拉取——覆盖「库切换/扫描落定在幻灯片视图后切回文件视图」时文件底册未
  // 加载（审核 9 同根因）与幻灯片清单未加载两种形态
  closeDetail()
  if (searchMode.value) return
  const targetEmpty = viewMode.value === 'file'
    ? fileEntries.value.length === 0
    : slideEntries.value.length === 0
  if (targetEmpty) loadView()
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
  // 加载态按路径记录（Set 语义，并发展开多卡片互不覆盖，审核 13）；写回前比对
  // 发起时的检索条件与库代次，条件已变（改词/改筛选/切库）丢弃旧响应，防陈旧
  // 命中页写入新语境（aggChildren 残留会被 toggleAgg 早退命中，同路径跨库串数据）
  aggLoadingPaths.value = aggLoadingPaths.value.concat(filePath)
  const keywordAtStart = activeKeyword.value.trim()
  const tagsAtStart = [...selectedTagIds.value]
  const libSeqAtStart = librarySeq
  const contextChanged = () => libSeqAtStart !== librarySeq ||
    keywordAtStart !== activeKeyword.value.trim() ||
    tagsAtStart.length !== selectedTagIds.value.length ||
    tagsAtStart.some((tagId, i) => tagId !== selectedTagIds.value[i])
  try {
    const page = await SearchAssets(currentLibraryId.value, keywordAtStart, {
      flatten: true,
      tagIds: tagsAtStart,
      limit: 200,
      offset: 0
    })
    if (contextChanged()) return // 检索条件已变：旧聚合响应丢弃
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
    if (contextChanged()) return // 条件已变：旧聚合失败静默丢弃
    handleError('加载命中页失败: ', error)
  } finally {
    aggLoadingPaths.value = aggLoadingPaths.value.filter(p => p !== filePath)
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
  searchSeq++ // 语境变更点：退出检索使在途检索写回失效
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

// 扫描/重建完成后刷新（数据刷新唯一入口：done 事件回调仅复位进度态，审核 12
// 防双路径各拉一次全量清单）；语义与库切换落定一致
const refreshAfterScan = () => resetAndReloadView()

// ================= 缩略图生成 / 跳转 / 设置 =================

// 素材绝对路径拼接：根路径去尾斜杠 + '/' + 相对路径（相对路径保持正斜杠形态直接拼）
const joinAssetPath = (rootPath, relPath) => {
  const root = (rootPath || '').replace(/[\\/]+$/, '')
  if (!root) return relPath || ''
  if (!relPath) return root
  return `${root}/${relPath}`
}

const onCancelThumbs = async () => {
  if (!hasLibrary.value) return
  try {
    const requested = await CancelAssetThumbs(currentLibraryId.value)
    if (requested) {
      ElMessage.info('已请求取消缩略图生成')
    } else {
      ElMessage.warning('当前没有可取消的生成任务')
    }
  } catch (error) {
    handleError('取消缩略图生成失败: ', error)
  }
}

// 失败缩略图重新入队（后台重新生成，done 事件经 epoch 自增驱动 miss 卡自动补图）
const onRetryThumb = async () => {
  if (!hasLibrary.value || retrying.value) return
  const path = detailThumbPath.value
  if (!detailFileId.value || !path) return
  retrying.value = true
  try {
    await RetryAssetThumb(currentLibraryId.value, detailFileId.value, path)
    ElMessage.success('已重新入队生成')
  } catch (error) {
    handleError('重试缩略图生成失败: ', error)
  } finally {
    retrying.value = false
  }
}

// 打开 PPT 并跳到当前幻灯片页（后端 COM 打开；成功不弹提示——PPT 窗口已打开，
// 弹窗反而抢焦点；BUSY 预期拒绝经 handleError 自动分流 warning 可重试）
const onJumpToSlide = async () => {
  if (!hasLibrary.value || jumping.value) return
  const path = detailThumbPath.value
  const slideNum = detailSlideNum.value
  if (!path || !slideNum) return
  jumping.value = true
  try {
    const absPath = joinAssetPath(currentLibrary.value?.rootPath, path)
    await JumpToAssetSlide(absPath, slideNum)
  } catch (error) {
    handleError('打开跳转失败: ', error)
  } finally {
    jumping.value = false
  }
}

// 打开设置弹窗：先以默认值填充草稿（防上一次脏值闪烁），再异步拉取当前设置回填。
// 基底先行置 null：上次打开的旧基底不可复用（期间设置可能已变），拉取失败保持
// null 由保存侧拦截（空壳整体提交会清掉其他设置段）
const openThumbSettings = async () => {
  thumbSettingsDraft.value = { ...THUMB_SETTINGS_DEFAULTS }
  rawSettings.value = null
  thumbSettingsVisible.value = true
  try {
    const settings = await GetSettings()
    rawSettings.value = settings || null
    thumbSettingsDraft.value = normalizeThumbSettings(rawSettings.value?.assets)
  } catch (error) {
    // 拉取失败保留默认草稿可继续编辑，保存时经基底缺失拦截提示重开
    handleError('加载设置失败: ', error)
  }
}

// assets 段归一化：缺失/字段零值逐字段回退默认（老配置无 assets 段场景）
const normalizeThumbSettings = (assets) => ({
  auto: typeof assets?.auto === 'boolean' ? assets.auto : THUMB_SETTINGS_DEFAULTS.auto,
  smallWidth: assets?.smallWidth > 0 ? assets.smallWidth : THUMB_SETTINGS_DEFAULTS.smallWidth,
  hdWidth: assets?.hdWidth > 0 ? assets.hdWidth : THUMB_SETTINGS_DEFAULTS.hdWidth,
  smallQuotaMB: assets?.smallQuotaMB > 0 ? assets.smallQuotaMB : THUMB_SETTINGS_DEFAULTS.smallQuotaMB,
  hdQuotaMB: assets?.hdQuotaMB > 0 ? assets.hdQuotaMB : THUMB_SETTINGS_DEFAULTS.hdQuotaMB
})

const onSaveThumbSettings = async () => {
  // 基底缺失禁止保存：SaveSettings 整体替换语义下，仅含 assets 段的空壳提交会
  // 清掉 defaultShell/themeMode/快捷键等全部其他段（后端 Ensure 仅补 webServe/
  // terminal/assets 三段）。提示重开弹窗重拉完整基底。
  if (!rawSettings.value) {
    ElMessage.warning('设置加载失败，请关闭后重新打开设置')
    return
  }
  thumbSettingsSaving.value = true
  try {
    // 回填 assets 段整体提交：其余设置段原样保留（SaveSettings 整体替换语义）
    rawSettings.value.assets = { ...thumbSettingsDraft.value }
    await SaveSettings(rawSettings.value)
    ElMessage.success('缩略图设置已保存，分辨率变更将自动重新生成')
    thumbSettingsVisible.value = false
  } catch (error) {
    handleError('保存设置失败: ', error)
  } finally {
    thumbSettingsSaving.value = false
  }
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

const onScanDone = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 仅复位进度展示；数据刷新由扫描/重建 Promise 成功分支的 refreshAfterScan 统一
  // 执行——done 事件先于 Promise 返回到达，双路径各刷一次会使大库全量清单拉取
  // 翻倍（审核 12：二选一，Promise 分支为准）
  scanProgress.value = null
}

const onScanError = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 仅复位进度展示；错误提示由扫描 Promise 的 handleError 统一分流（防双弹）
  scanProgress.value = null
}

// ================= 缩略图事件（assets-thumbs:*）=================
// EventsOn 返回注销闭包，onBeforeUnmount 精准注销本组件监听器（禁 EventsOff 全局移除）
let offThumbsProgress = null
let offThumbsDone = null
let offThumbsError = null

const onThumbsProgress = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  thumbsRunning.value = true
  thumbsProgress.value = payload
  // 失败收集：failed 为批次内累计失败数，增量大于上次时当前文件即新失败文件
  //（后端串行队列，failed 递增时刻的 currentFile 即刚失败文件）
  if (payload.failed > lastThumbsFailedCount && payload.currentFile) {
    if (!failedThumbPaths.value.includes(payload.currentFile)) {
      failedThumbPaths.value = failedThumbPaths.value.concat(payload.currentFile)
    }
    lastThumbsFailedCount = payload.failed
  }
}

const onThumbsDone = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  thumbsRunning.value = false
  thumbsProgress.value = null
  lastThumbsFailedCount = 0
  // 失败集合为内存态，批次结束即清（PRD 认可：失败标记只覆盖当前批次语境）
  failedThumbPaths.value = []
  // 失效代次自增：触发 miss 缩略图卡自动补图
  thumbEpoch.value++
}

const onThumbsError = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 契约：仅复位进度不弹提示（错误详情后端 slog 已记录；防与 Promise catch 双弹）
  thumbsRunning.value = false
  thumbsProgress.value = null
}

// 探测本机 PowerPoint 缩略图能力：失败（含绑定缺失等同步抛错场景）按降级处理，
// 不弹错、提示条常驻引导；await + try/catch 保证同步异常同样被降级吸收（.catch()
// 链只能捕获 rejected promise，捕获不了调用本身的同步 throw，会炸挂载链）
const probeThumbsAvailable = async () => {
  try {
    thumbsAvailable.value = await AssetThumbsAvailable()
  } catch {
    thumbsAvailable.value = false
  }
}

onMounted(() => {
  // 先注册事件监听再异步加载数据：保证卸载路径必能拿到全部 off 闭包
  offProgress = EventsOn('assets-scan:progress', onScanProgress)
  offDone = EventsOn('assets-scan:done', onScanDone)
  offError = EventsOn('assets-scan:error', onScanError)
  offThumbsProgress = EventsOn('assets-thumbs:progress', onThumbsProgress)
  offThumbsDone = EventsOn('assets-thumbs:done', onThumbsDone)
  offThumbsError = EventsOn('assets-thumbs:error', onThumbsError)
  probeThumbsAvailable()
  loadLibraries()
})

onBeforeUnmount(() => {
  offProgress && offProgress()
  offDone && offDone()
  offError && offError()
  offThumbsProgress && offThumbsProgress()
  offThumbsDone && offThumbsDone()
  offThumbsError && offThumbsError()
  offProgress = null
  offDone = null
  offError = null
  offThumbsProgress = null
  offThumbsDone = null
  offThumbsError = null
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

/* ---- 缩略图能力降级提示条（信息样式：环境缺失属正常降级非故障，勿用 error 红）---- */
.thumbs-degrade-banner {
  flex-shrink: 0;
  padding: var(--spacing-xs) var(--spacing-md);
  font-size: 12px;
  color: var(--info-color);
  background: var(--primary-bg);
  border-bottom: 1px solid var(--border-color);
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

/* ---- 详情缩略图与失败标记 ---- */
.detail-thumb-failed {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--spacing-xs);
}

/* ---- 缩略图设置弹窗 ---- */
.thumb-settings-hint {
  font-size: 12px;
  color: var(--text-tertiary);
  line-height: 1.6;
  padding: 0 var(--spacing-md) var(--spacing-sm);
}

.settings-unit {
  margin-left: var(--spacing-sm);
  font-size: 12px;
  color: var(--text-tertiary);
}
</style>
