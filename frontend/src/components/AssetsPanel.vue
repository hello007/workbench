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
     拆分链路（SRS §2.2.3 / §2.2.5 方式 2）：库级「拆分全部 PPT」+ 文件详情「拆分
     此 PPT」两入口共用落点确认弹窗（素材库缓存目录 derived 镜像源树为默认落点，
     源目录落点带自动排除索引警示）；进度经 assets-split:* 事件推送（文件级
     done/total + 页级 pageDone/pageTotal 透传）、可取消（取消走 warning 中性提示）；
     done/error 事件仅复位进度展示——汇总提示与错误分流由拆分 Promise 分支统一
     执行（防双弹双刷新，对齐扫描契约）；已拆分文件的幻灯片详情提供「打开拆分单页」
     （GetAssetSplitRecord exists 判定显隐 + OpenAssetSplitPage 打开，与 COM 跳转
     方式并存）。
     幻灯片独立标注（SRS §2.2.4）：单页三层标注（星级/幻灯片标签/业务备注）经
     UpdateAssetSlideMeta 整体替换保存（存储 slide_tags/slide_meta，键 file_id +
     slide_num，与文件标签存储完全独立、不触发重解析与缩略图失效）；详情面板
     「文件标签（继承）」只读弱化展示归属文件标签，与自有标签编辑区上下分区；
     标签筛选「仅幻灯片自身标签」开关（opts.slideTagOnlyOwn）切换继承合并/仅自身
     两种命中口径（默认合并向后兼容，仅作用于标签筛选，关键词命中不受影响）。
     标签树与标签管理（第六批 SRS §2.4）：标签数据源切换为 ListAssetTags 独立
     API（PRD D1，含未使用标签与直接绑定计数，底册聚合降级为兜底）；筛选与详情
     编辑消费 AssetTagTreeSelect 单组件（ID 口径，保存契约标签名数组在宿主侧
     双向映射）；「管理标签」弹窗（AssetTagManagerDialog）承载标签 CRUD，变更
     成功经 changed 事件触发宿主刷新链（树重拉 + 底册重拉 + 检索态重跑，数据
     变更点位不清选中）。批量打标签（第六批 PR3，PRD D3/D4/D5/D7）：入口挂
     AssetSelectionBar batch-tag 事件，targets 由选中键经当前渲染卡反查
     （assetSelectionKey 单点映射，miss 跳过计数）；AssetBatchTagDialog 弹窗不
     感知 viewMode——宿主 execute 分派回调按 viewMode 分派
     BatchUpdateAssetFileTags（fileIDs）/ BatchUpdateAssetSlideTags（(fileId,
     slideNum) 目标，禁 slides.id）；completed 触发刷新链（树重拉 + 底册重拉 +
     检索态重搜 + 幻灯片浏览清单补拉，双代次守卫防在途写回污染），选中保留
     （D5：数据变更点位非语境变更）。批量评分（第八批，SRS §2.6 P0，PRD D4/D5）：
     入口挂 AssetSelectionBar batch-rating 事件，targets 反查链路复用批量打标签
     同款（assetSelectionKey 单点映射）；AssetBatchRatingDialog 弹窗不感知
     viewMode——宿主 execute 分派回调以 targets 调 BatchSetAssetRating（单一
     入口混合文件/页目标，slideNum 0 = 文件条目，0 星 = 清除）；completed 走
     同款刷新链（底册重拉 + 检索态重搜 + 幻灯片浏览清单补拉，标签树不涉评分子
     变更故不重拉），选中保留（D5）。
     facet 多维筛选（第七批 SRS §2.5 PR2）：工具行「筛选」弹层三维条件（星级/
     修改时间/文件大小，文件维度口径，幻灯片条目继承归属文件属性判定）与关键
     词/标签组合检索；浏览语境 facet 激活（任一维度生效）时放开「空关键词+空
     标签返回空页」语义改走 SearchAssets 的 facets-only 分页链路（后端返回文件
     条目，底册退位），清除后回底册。UI 档位快照宿主持有（facetState 受控），
     档位→绝对纳秒/字节区间换算单点在 AssetFilterPopover 模块具名导出
     （buildFacetOptions 每次检索调用时现算，相对档边界随检索时刻刷新）；facet
     变更为数据过滤语境变更——清详情/预览/选中/文件过滤/聚合展开缓存后按检索
     语境分流重查，清空点位登记见 docs/spec/assets-management.md §8.2。
     保存检索条件（第七批 SRS §2.5 PR3，PRD D4/D7）：工具行「保存搜索」把当前
     检索语境快照命名保存（检索语境与浏览语境 facet 激活组合均可保存——两者都
     是合法检索条件，全空禁用），快照 = AssetSavedSearch 全维字段（keyword/
     tagIds/tagNames 冗余/facet 区间原始值/flatten/slideTagOnlyOwn/libraryId）；
     「已存搜索」弹层按本库/异库分组管理（行点击调取、覆盖 Save 同 id、删除确认
     后 Delete，变更后 List 重拉）；调取完整还原检索语境（D7，facet 区间经
     restoreFacetStateFromSnapshot 反推 UI 档位——相对档保存即冻结为绝对区间）
     并走 applySearch 与手动改条件同路径（searchSeq 代次守卫天然覆盖，复位面由
     其统一执行）；异库条目降级调取（D4）——tagIds 属保存时所在库不可复用，按
     tagNames 在当前库标签表重查 ID，miss 丢弃该标签并计数提示；覆盖会按当前库
     重写条目 libraryId（静默搬家）故仅本库条目可覆盖。调取为语境变更点位
     （§8.2 矩阵登记），命名弹窗重名拒绝由后端 Save 返回、handleError 透传。
     异步写回守卫：库代次（librarySeq）与检索代次（searchSeq）双序号，await 写回前
     比对，不一致丢弃旧响应（防切库在途/改词重搜的慢响应跨语境串入）。
     呈现维度（SRS §2.3 PR1）：displayMode 网格/列表两形态共用同一 displayCards
     卡片模型与数据语境（切换不改分页累计/检索结果）；列表呈现为受控子组件
     AssetListView（无自有业务状态），行点击/滚动到底仅上报事件，分派复用
     onGalleryScroll 的同一 loadMoreInView 单点；空态四组与呈现维度无关，外提共用。
     全屏预览（SRS §2.3 PR2）：AssetPreviewOverlay 受控层覆盖画廊列（详情侧栏在
     层外保持可见可编辑，D5#6 预览即详情导航）；序列 = displayCards 过滤幻灯片子
     序列（不含聚合展开子项），翻页写回 previewIndex 经 watch 同步详情侧栏，末页
     「→」经 load-more 触发续拉不预翻；语境变更点位（检索/视图/平铺/切库/刷新）
     统一关闭预览。
     选中体系（SRS §2.3 PR3，PRD D2/D5）：selectedKeys Set（整体替换驱动响应式，
     不依赖 Vue 对 Set 变异方法的拦截），键形态 'file:<fileId>' / 'slide:<fileId>:<slideNum>'
     （禁 slides.id——重解析全变，映射收敛 utils/assetSelection 共享单点）；网格卡
     左上角与列表行首常驻复选框（@click.stop 防进详情，D5#7），聚合父卡不渲染复选
     框、聚合子行保持展示型（D5#1）；底部浮动操作条（AssetSelectionBar 受控组件）
     选中数 > 0 浮出，全选/反选作用域 = 当前渲染独立卡键集（D5#8），反选 = 渲染集
     内取反且渲染集外旧选中一并清除（防幽灵选中）；清空点位与预览关闭同批语境
     变更函数（D5#2/#3/#4：检索/视图/平铺/仅自身标签/切库/扫描刷新），呈现切换
     不清空（D5#5）。
     失败缩略图自动补（SRS §2.3 PR3，PRD D4）：assets-thumbs:done 后失败集逐个
     RetryAssetThumb 自动重新入队，恰好一轮——普通 let 标记防下轮 done 再触发
     （手动重试不受限），切库 watch 复位（新库新批次重新获得资格）；fire-and-forget
     allSettled 收口不弹提示，fileId 从文件底册按路径反查，miss 跳过。
     文件内过滤与卡片尺寸（D6 真机增量）：网格固定卡宽（CSS 变量 --asset-card-w
     驱动 auto-fill，详情栏开合只减列数不改卡宽，消除 minmax+1fr 满铺在详情栏
     挤压画廊时的重排突变）+ 工具条缩放滑块（220-420 步长 20 默认 280，会话态
     不持久化，仅网格呈现提供）；文件卡双击进入「仅看该文件页」过滤浏览（检索态
     先退检索，目标页不在已加载段时自动续拉直到出现或全局拉完），幻灯片卡双击
     直接进全屏预览（双击序 click→click→dblclick，首击已装配详情）；幻灯片详情
     解析文本（页文本/演讲者备注/命中摘要）下沉到操作按钮区之后（标注编辑与
     跳转操作优先，解析自带内容垫底）。
     事件注销遵循 cross-layer-contracts：EventsOn 返回闭包精准注销，禁 EventsOff 全局移除。
     （拆分监听器属面板层——进度条与弹窗互斥常态：确认弹窗可随时关闭、任务继续
     在后台执行且面板层进度可见，故监听器生命周期 = 面板组件生命周期，注销时机 =
     onBeforeUnmount；弹窗非监听器属主，无「弹窗关闭注销点」，对比 PptSplitDialog
     的弹窗内监听双时机形态。）
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
          :disabled="!hasLibrary || scanning || splitting"
          @click="openSplitDialog('library')"
        >拆分全部 PPT</el-button>
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

    <!-- 拆分进度条：仅拆分任务进行中可见；与扫描/缩略图进度条同层并存。文件级
         done/total 驱动主进度，页级 pageDone/pageTotal（当前文件拆到第几页）追加
         展示；进度载荷按库 ID 过滤（他库事件不串入）。确认弹窗可随时关闭，任务
         继续在后台执行且本进度条持续可见 -->
    <div v-if="splitting" class="scan-progress split-progress">
      <el-progress
        class="scan-progress-bar"
        :percentage="splitPercent"
        :stroke-width="8"
        :show-text="false"
      />
      <span class="scan-progress-text">
        {{ splitProgress ? `拆分中 ${splitProgress.done}/${splitProgress.total}` : '正在准备拆分…' }}
      </span>
      <span v-if="splitPageText" class="scan-progress-text">{{ splitPageText }}</span>
      <span v-if="splitProgress && splitProgress.currentFile" class="scan-progress-file" :title="splitProgress.currentFile">
        {{ splitProgress.currentFile }}
      </span>
      <el-button size="small" text type="danger" @click="onCancelSplit">取消拆分</el-button>
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
      <!-- 标签筛选（第六批起树形化）：AssetTagTreeSelect 单组件 ID 口径（后端
           SearchAssets opts.tagIds 既有入参口径不变，级联展开选父含后代由后端
           检索链完成，前端不展开）；拉取成功经 loaded 事件回传共享标签列表 -->
      <AssetTagTreeSelect
        ref="filterTagSelectRef"
        v-model="selectedTagIds"
        class="tag-filter"
        :library-id="currentLibraryId"
        placeholder="按标签筛选"
        size="small"
        :disabled="!hasLibrary"
        @change="applySearch"
        @loaded="onAssetTagsLoaded"
      />
      <!-- facet 多维筛选（第七批 SRS §2.5，D2/D3）：弹层三维控件（星级/修改时间/
           文件大小），badge 常显生效维度数；受控组件——UI 档位快照宿主持有
           （facetState），档位→检索参数换算在组件模块单点，检索时经
           buildFacetOptions 现算并入 SearchAssets opts；change 触发语境变更链 -->
      <AssetFilterPopover
        v-model="facetState"
        class="facet-filter"
        :disabled="!hasLibrary"
        @change="onFacetChange"
      />
      <!-- 排序档位下拉（SRS §2.5.3，D1）：检索工具行独立控件，与 facet 低频筛选
           不同频、高频切换直出工具栏。相关性默认（FTS bm25 序，无向）；mtime/
           rating 档默认降序，同档再点翻转升降（菜单项即时标注当前方向）。变更走
           applySearch 同路径（排序属检索条件变更点位，§8.2 矩阵复位面统一覆盖） -->
      <el-dropdown trigger="click" @command="onSortCommand">
        <el-button size="small" text type="primary" :disabled="!hasLibrary" class="sort-trigger">
          {{ sortTriggerLabel }}
          <el-icon class="sort-trigger-arrow"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu class="sort-menu">
            <el-dropdown-item command="relevance">相关性（默认）</el-dropdown-item>
            <el-dropdown-item command="mtime">
              修改时间{{ sortBy === 'mtime' ? (sortDir === 'desc' ? '（降序）' : '（升序）') : '' }}
            </el-dropdown-item>
            <el-dropdown-item command="rating">
              评分{{ sortBy === 'rating' ? (sortDir === 'desc' ? '（降序）' : '（升序）') : '' }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <!-- 保存检索条件（第七批 PR3，D7）：当前检索语境一键命名快照。检索语境
           （关键词/标签）与浏览语境 facet 激活组合均可保存——两者都是合法检索
           条件；全空（非检索语境）禁用，无可快照内容 -->
      <el-button
        size="small"
        text
        type="primary"
        :disabled="!hasLibrary || !searchMode"
        @click="openSaveSearchDialog"
      >保存搜索</el-button>
      <!-- 已存检索条件入口（D4 全局存储带库归属）：弹层按本库/异库分组列出，
           行点击调取（异库按名重映射降级），行内覆盖/删除管理操作；打开即重拉
           最新清单，变更后重拉（List 权威，本地不缓存增删） -->
      <el-popover
        ref="savedSearchPopoverRef"
        placement="bottom-start"
        :width="340"
        trigger="click"
        popper-class="asset-saved-search-popper"
        @show="reloadSavedSearches"
      >
        <template #reference>
          <el-button size="small" text type="primary" :disabled="!hasLibrary">已存搜索</el-button>
        </template>
        <div class="saved-search-panel">
          <div v-if="savedSearchesLoading" class="saved-search-hint">加载中…</div>
          <div v-else-if="savedSearches.length === 0" class="saved-search-hint">
            暂无保存的检索条件，点「保存搜索」把当前条件存起来
          </div>
          <template v-else>
            <div v-if="currentLibrarySavedSearches.length > 0" class="saved-search-group">
              <div class="saved-search-group-title">本库条件</div>
              <div v-for="item in currentLibrarySavedSearches" :key="item.id" class="saved-search-item">
                <button
                  type="button"
                  class="saved-search-apply"
                  :title="item.keyword ? `关键词：${item.keyword}` : '点击调取此条件'"
                  @click="applySavedSearch(item)"
                >{{ item.name }}</button>
                <el-button
                  size="small"
                  text
                  type="primary"
                  :disabled="!canOverwriteSavedSearch(item)"
                  :title="canOverwriteSavedSearch(item) ? '以当前检索条件覆盖此条目' : '覆盖需在条目所在库且当前有检索条件时可用'"
                  @click="overwriteSavedSearch(item)"
                >覆盖</el-button>
                <el-button
                  size="small"
                  text
                  type="danger"
                  :disabled="savedSearchMutatingId !== 0"
                  @click="deleteSavedSearch(item)"
                >删除</el-button>
              </div>
            </div>
            <div v-if="otherLibrarySavedSearches.length > 0" class="saved-search-group">
              <div class="saved-search-group-title">其他库条件</div>
              <div v-for="item in otherLibrarySavedSearches" :key="item.id" class="saved-search-item">
                <button
                  type="button"
                  class="saved-search-apply"
                  :title="`保存于「${savedSearchLibraryName(item)}」，调取时标签按名重映射`"
                  @click="applySavedSearch(item)"
                >
                  {{ item.name }}
                  <span class="saved-search-lib-mark">{{ savedSearchLibraryName(item) }}</span>
                </button>
                <el-button
                  size="small"
                  text
                  type="primary"
                  :disabled="true"
                  title="覆盖会按当前库重写条目归属，请切换到条目所在库后操作"
                >覆盖</el-button>
                <el-button
                  size="small"
                  text
                  type="danger"
                  :disabled="savedSearchMutatingId !== 0"
                  @click="deleteSavedSearch(item)"
                >删除</el-button>
              </div>
            </div>
          </template>
        </div>
      </el-popover>
      <!-- 标签管理入口（PRD D6）：标签 CRUD 统一弹窗，筛选/详情编辑的标签消费点
           共用同一数据链路（弹窗变更后经 changed 事件触发宿主刷新链） -->
      <el-button size="small" text type="primary" :disabled="!hasLibrary" @click="openTagManager">
        <el-icon><CollectionTag /></el-icon>
        管理标签
      </el-button>
      <!-- 幻灯片标签继承开关（SRS §2.2.4）：false = 幻灯片条目按【页自身标签 ∪ 归属
           文件标签】合并命中（默认向后兼容）；true = 仅页自身标签命中。恒渲染占位
           防视图切换时同行控件跳动；文件视图下检索结果只含文件条目（开关不产生
           可见效果），禁用 + tooltip 声明作用域 -->
      <el-tooltip :content="slideTagOwnTooltip" placement="bottom">
        <span class="flatten-switch slide-own-switch" :class="{ 'is-scope-disabled': !isSlideView }">
          <el-switch v-model="slideTagOnlyOwn" size="small" :disabled="!isSlideView" @change="onSlideTagOwnChange" />
          仅幻灯片自身标签
        </span>
      </el-tooltip>
      <el-radio-group v-model="viewMode" size="small" @change="onViewModeChange">
        <el-radio-button value="file">文件视图</el-radio-button>
        <el-radio-button value="slide">幻灯片视图</el-radio-button>
      </el-radio-group>
      <!-- 呈现维度切换（网格/列表，SRS §2.3 PR1）：纯呈现形态切换，数据语境与
           分页累计不变（对齐 PRD D5#5）；无 @change 处理器即无数据侧动作 -->
      <el-radio-group v-model="displayMode" size="small">
        <el-radio-button value="grid">网格</el-radio-button>
        <el-radio-button value="list">列表</el-radio-button>
      </el-radio-group>
      <el-tooltip content="开启后同一 PPT 的多页命中不再折叠为一个条目，逐页平铺展示" placement="bottom">
        <span class="flatten-switch">
          <el-switch v-model="flatten" size="small" :disabled="!hasLibrary" @change="onFlattenChange" />
          平铺
        </span>
      </el-tooltip>
      <!-- 卡片大小滑块（D6 真机增量，PRD D4）：固定卡宽网格的宽度调节入口，经
           --asset-card-w CSS 变量驱动画廊；仅网格呈现提供（列表行固定尺寸无缩放
           对象，v-if 防列表视图占位跳动）；会话态不持久化，重开应用回默认 280 -->
      <el-tooltip v-if="displayMode === 'grid'" content="卡片大小" placement="bottom">
        <div class="card-size-slider">
          <el-slider v-model="cardWidth" :min="220" :max="420" :step="20" size="small" />
        </div>
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

    <!-- 主体：画廊网格/列表（呈现维度切换）+ 详情侧边栏。空态四组与呈现维度无关
         （判定仅依赖库/清单/检索语境），外提到网格与列表分支共用，防两分支模板重复 -->
    <div class="assets-body">
      <div class="assets-view-wrap">
        <!-- 文件内过滤提示条（D6 真机增量）：双击文件卡进入「仅看该文件页」浏览
             语境的弱化提示（info 色对齐缩略图降级条，非警示红）；N = 过滤后已加载
             页数，自动续拉完成前可能暂小于该文件实际页数 -->
        <div v-if="isFileFilterActive" class="file-filter-banner">
          <span>正在筛选：{{ fileFilterName }} 的幻灯片页（{{ filteredSlideCount }} 页）</span>
          <el-button size="small" text type="primary" @click="clearFileFilter">清除过滤</el-button>
        </div>
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
        <!-- 空态三：检索无结果（命中存在但被当前视图类型过滤时给切换引导；facet
             激活时零命中/视图过滤均给「清除全部筛选」一键复位，D6 两语境共用） -->
        <el-empty v-else-if="searchMode && !loading && displayCards.length === 0" :description="searchEmptyText">
          <el-button v-if="facetActiveCount > 0" @click="clearAllFilters">清除全部筛选</el-button>
        </el-empty>
        <!-- 空态四：视图无数据（如幻灯片视图下未扫描/无页面） -->
        <el-empty v-else-if="hasLibrary && !loading && !searchMode && browseCards.length === 0 && stats && stats.fileCount > 0" description="该视图下暂无条目" />

        <!-- 网格呈现（默认）：滚动容器 + 卡片网格。固定卡宽（--asset-card-w 经
             auto-fill 排列，禁 minmax+1fr 满铺——详情栏开合挤压画廊时 1fr 拉伸
             致卡片尺寸突变，D6 真机修复）；卡片宽度由滑块经 CSS 变量注入 -->
        <div
          v-else-if="displayMode === 'grid'"
          ref="galleryRef"
          class="assets-gallery"
          :style="{ '--asset-card-w': cardWidth + 'px' }"
          @scroll="onGalleryScroll"
        >
          <div
            v-for="card in displayCards"
            :key="card.key"
            class="asset-card"
            :class="{ 'is-selected': isCardSelected(card), 'is-expandable': card.hitCount > 1 }"
            @click="onCardClick(card)"
            @dblclick="onCardDblclick(card)"
          >
            <!-- 勾选复选框（SRS §2.3 PR3，常驻左上角）：聚合父卡不渲染（D5#1）；
                 @click.stop 阻断进详情（D5#7 勾选与浏览两套交互并行不互斥） -->
            <input
              v-if="card.hitCount <= 1"
              type="checkbox"
              class="asset-card-check"
              :checked="isCardChecked(card)"
              :aria-label="`选中 ${card.title}`"
              @click.stop
              @change="toggleCardSelection(card)"
            />
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
        </div>

        <!-- 列表呈现：受控组件（选中/详情/分页判定留宿主），行点击与滚动到底仅上报
             事件，分派复用网格 onGalleryScroll 的同一分派函数防两份判定漂移 -->
        <AssetListView
          v-else
          :cards="displayCards"
          :file-entries="fileEntries"
          :library-id="currentLibraryId"
          :thumb-epoch="thumbEpoch"
          :search-mode="searchMode"
          :loading-more="loadingMore"
          :can-load-more="listCanLoadMore"
          :expanded-agg-paths="expandedAggPaths"
          :agg-children="aggChildren"
          :agg-loading-paths="aggLoadingPaths"
          :selected-keys="selectedKeys"
          @card-click="onCardClick"
          @load-more="loadMoreInView"
          @toggle-select="toggleCardSelection"
        />

        <!-- 全屏预览层（SRS §2.3 PR2）：面板内覆盖画廊列（详情侧栏在外保持可见可编辑，
             D5#6）；受控组件——序列/下标在宿主，翻页写回经 watch 同步详情，末页「→」
             触发 loadMoreInView 续拉不预翻（懒加载边界写入回后用户再点翻页） -->
        <AssetPreviewOverlay
          v-if="previewIndex !== null && previewSlides.length > 0"
          :slides="previewSlides"
          :index="previewIndex"
          :library-id="currentLibraryId"
          :thumb-epoch="thumbEpoch"
          :can-load-more="listCanLoadMore"
          @update:index="previewIndex = $event"
          @load-more="loadMoreInView"
          @close="closePreview"
        />

        <!-- 选中浮动操作条（SRS §2.3 PR3，PRD D2）：选中数 > 0 浮出于画廊区底部
             居中（absolute 挂 .assets-view-wrap relative 语境，z-index 5 低于预览
             层 10）；受控组件——计数传入选中集合大小，全选/反选/清空分派留宿主
             （作用域 = 当前渲染独立卡键集，D5#8），批量打标签（第六批 PR3）与
             批量评分（第八批）分别经 batch-tag / batch-rating 事件触发宿主
             targets 构建与弹窗打开 -->
        <AssetSelectionBar
          :count="selectedKeys.size"
          @select-all="selectAllRendered"
          @invert="invertSelection"
          @batch-tag="openBatchTagDialog"
          @batch-rating="openBatchRatingDialog"
          @clear="clearSelection"
        />
      </div>

      <!-- 详情侧边栏：文件与幻灯片均可编辑（文件级星级/备注/标签；幻灯片级星级/
           业务备注/自有标签），幻灯片另带归属文件标签（继承）只读区与溯源跳转 -->
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
          <!-- 全屏预览入口（SRS §2.3 PR2）：仅幻灯片详情提供（预览序列 = 当前视图
               幻灯片子序列，文件视图序列空入口隐藏非禁用）；打开即以当前详情页
               在序列中的下标定位（详情已定位到页） -->
          <el-button
            v-if="canPreview"
            size="small"
            text
            type="primary"
            class="detail-preview-btn"
            @click="openPreview"
          >全屏预览</el-button>
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
              <div class="detail-tags-row">
                <!-- 树形标签选择（PRD D6，去 allow-create）：组件统一 ID 口径，保存
                     契约 UpdateAssetFileMeta 入参为标签名数组不变——detailTagIds
                     计算属性在宿主侧做 ID↔名称双向映射（见脚本注释） -->
                <AssetTagTreeSelect
                  ref="detailTagSelectRef"
                  v-model="detailTagIds"
                  class="detail-tags-select"
                  :library-id="currentLibraryId"
                  placeholder="选择标签"
                  empty-text="暂无标签，点「+ 新建标签」创建"
                  :disabled="savingMeta"
                  @loaded="onAssetTagsLoaded"
                />
                <!-- 新建标签快捷入口（PRD D6：替代原 allow-create 的输入即建，
                     预填管理弹窗为新增根标签表单态） -->
                <el-button size="small" text type="primary" :disabled="savingMeta" @click="openTagManagerForCreate">+ 新建标签</el-button>
              </div>
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
              <!-- 单文件拆分入口（对齐库级入口共用同一确认弹窗）：仅 pptx 文件提供 -->
              <el-button
                v-if="isDetailFilePptx"
                size="small"
                plain
                :disabled="splitting"
                @click="openSplitDialog('file')"
              >拆分此 PPT</el-button>
            </div>
          </template>
          <el-alert v-else type="warning" :closable="false" title="文件索引记录不存在，请重新扫描后操作" />
        </template>

        <!-- 幻灯片详情：三层独立标注（星级/幻灯片标签/业务备注）可编辑（对齐文件
             详情编辑形态）+ 归属文件标签（继承）只读弱化 + 解析自带内容只读展示 +
             溯源跳转。业务备注与演讲者备注严格命名区分（前者用户标注、后者解析自带）。
             解析文本三区（页文本/演讲者备注/命中摘要）下沉到操作按钮区之后（D6 真机
             增量：标注编辑与跳转操作优先，解析自带内容垫底——内容长时不再把编辑区
             与操作区顶出可视范围） -->
        <template v-else>
          <div class="detail-section">
            <div class="detail-row"><span class="detail-label">页码</span><span class="detail-value">第 {{ detailSlideNum }} 页</span></div>
            <div class="detail-row"><span class="detail-label">归属文件</span><span class="detail-value" :title="detailFilePath">{{ detailFilePath }}</span></div>
          </div>
          <!-- 文件标签（继承）：只读弱化区（无输入控件），与下方幻灯片自有标签编辑区上下分区 -->
          <div class="detail-section">
            <div class="detail-label">文件标签（继承）</div>
            <div v-if="detailTags.length > 0" class="detail-inherit-tags">
              <span v-for="tag in detailTags" :key="tag.id" class="card-tag">
                <span class="tag-color-dot" :style="{ background: tag.color }"></span>{{ tag.name }}<span class="inherit-mark">继承</span>
              </span>
            </div>
            <span v-else class="detail-empty-hint">归属文件暂无标签</span>
          </div>
          <!-- 独立标注编辑区（存储于 slide_tags/slide_meta，键 file_id+slide_num，
               与文件标签存储完全独立；保存不触发重解析与缩略图失效） -->
          <div class="detail-section">
            <div class="detail-label">星级</div>
            <el-rate v-model="slideDraft.rating" clearable />
          </div>
          <div class="detail-section">
            <div class="detail-label">幻灯片标签</div>
            <div class="detail-tags-row">
              <!-- 树形标签选择（对齐文件详情，第六批去 allow-create）：slideTagIds
                   计算属性在宿主侧做 ID↔名称双向映射（保存契约 tagNames 数组不变） -->
              <AssetTagTreeSelect
                ref="detailTagSelectRef"
                v-model="slideTagIds"
                class="detail-tags-select slide-tags-select"
                :library-id="currentLibraryId"
                placeholder="选择页标签"
                empty-text="暂无标签，点「+ 新建标签」创建"
                :disabled="savingMeta"
                @loaded="onAssetTagsLoaded"
              />
              <el-button size="small" text type="primary" :disabled="savingMeta" @click="openTagManagerForCreate">+ 新建标签</el-button>
            </div>
          </div>
          <div class="detail-section">
            <div class="detail-label">业务备注</div>
            <el-input
              v-model="slideDraft.note"
              type="textarea"
              :rows="4"
              placeholder="记录本页素材用途、适配场景等业务备注（区别于演讲者备注）"
            />
          </div>
          <div class="detail-actions">
            <el-button size="small" type="primary" :loading="savingMeta" @click="onSaveSlideMeta">保存</el-button>
            <el-button size="small" :disabled="savingMeta" @click="resetSlideDraft">重置</el-button>
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
            <!-- 打开拆分单页（SRS §2.2.5 方式 2，无 PowerPoint 依赖）：仅已拆分文件
                 提供（GetAssetSplitRecord exists 判定显隐，隐藏非禁用）；与方式 1
                 （COM 打开跳转）并存 -->
            <el-button
              v-if="detailSplitExists"
              size="small"
              type="primary"
              plain
              :loading="openingSplitPage"
              @click="onOpenSplitPage"
            >打开拆分单页</el-button>
            <el-button size="small" type="primary" plain @click="traceToSourceFile">查看归属文件</el-button>
          </div>
          <!-- 解析自带内容只读展示区（下沉到操作区之后，展示优先级低于标注编辑） -->
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

    <!-- 拆分确认弹窗：单文件与库级批量两入口共用（splitDialogCtx 区分语境）。
         落点二选一：素材库缓存目录（derived 镜像源树，默认推荐——库根之外天然
         隔离扫描索引）；源文件所在目录（与源同目录，后端 isPptxSplitDerivative
         已自动排除索引，选中时展示警示说明）。进行中态防重复提交 -->
    <el-dialog
      v-model="splitDialogVisible"
      title="拆分 PPT"
      width="min(520px, 90vw)"
      :close-on-click-modal="false"
      append-to-body
      @closed="onSplitDialogClosed"
    >
      <div class="split-dialog-scope">
        <span v-if="splitDialogCtx.scope === 'file'">将把「{{ splitDialogCtx.fileName }}」按页拆分为多个独立 PPT（每页一个文件）。</span>
        <span v-else>将对当前素材库中全部正常状态的 PPT 文件按页拆分（每页一个独立文件），已拆分且未变化的文件自动跳过。</span>
      </div>
      <el-form label-width="88px">
        <el-form-item label="产物位置">
          <el-radio-group v-model="splitTarget" :disabled="splitConfirming">
            <el-radio value="cache">素材库缓存目录（推荐）</el-radio>
            <el-radio value="source">源文件所在目录</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <div v-if="splitTarget === 'source'" class="split-source-warning">
        拆分产物将与源文件同目录存放（已自动排除索引，不会被重复收录）。
      </div>
      <div v-else class="split-cache-hint">
        产物存入素材库缓存目录（按源目录结构镜像存放），与原始文件隔离，不会被重复索引。
      </div>
      <template #footer>
        <el-button :disabled="splitConfirming" @click="splitDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="splitConfirming" @click="onConfirmSplit">开始拆分</el-button>
      </template>
    </el-dialog>

    <!-- 拆分结果明细弹窗：批量拆分存在失败文件时自动弹出（异常隔离明细逐文件
         展示：路径 + 处理结果 + 跳过/失败原因），仅展示可关闭 -->
    <el-dialog
      v-model="splitOutcomeVisible"
      title="拆分结果明细"
      width="min(600px, 90vw)"
      append-to-body
    >
      <div class="split-outcome-list">
        <div v-for="(item, i) in splitOutcomeItems" :key="i" class="split-outcome-row">
          <el-tag size="small" :type="splitOutcomeTagType(item.status)">{{ splitOutcomeLabel(item.status) }}</el-tag>
          <span class="split-outcome-path" :title="item.relPath">{{ item.relPath }}</span>
          <span v-if="item.message" class="split-outcome-msg" :title="item.message">{{ item.message }}</span>
        </div>
      </div>
      <template #footer>
        <el-button type="primary" @click="splitOutcomeVisible = false">关闭</el-button>
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

    <!-- 标签管理弹窗（PRD D6）：标签 CRUD 统一入口；变更成功经 changed 事件触发
         宿主刷新链（树组件重拉 + 底册/检索刷新，见 refreshAfterTagChange）。
         presetCreate 供详情「+ 新建标签」快捷入口预填新增表单态 -->
    <AssetTagManagerDialog
      v-model="tagManagerVisible"
      :library-id="currentLibraryId"
      :preset-create="tagManagerPresetCreate"
      @changed="refreshAfterTagChange"
    />

    <!-- 批量打标签弹窗（第六批 PR3，PRD D3/D4/D5）：targets 由选中键经当前渲染
         卡反查构建（assetSelectionKey 单点映射），execute 按 viewMode 分派两批量
         API（弹窗不感知 viewMode）；completed 触发刷新链（底册/检索/树重拉），
         选中保留（D5 数据变更点位不清空） -->
    <AssetBatchTagDialog
      v-model="batchTagDialogVisible"
      :library-id="currentLibraryId"
      :targets="batchTagTargets"
      :execute="executeBatchTagUpdate"
      @completed="refreshAfterBatchTag"
    />

    <!-- 批量评分弹窗（第八批，SRS §2.6 P0，PRD D4/D5）：targets 反查与构建链路
         复用批量打标签同款；execute 以星级值分派 BatchSetAssetRating（单一入口
         混合文件/页目标，slideNum 0 = 文件条目）；completed 走同款刷新链
         （评分不涉标签变更，标签树不重拉），选中保留（D5） -->
    <AssetBatchRatingDialog
      v-model="batchRatingDialogVisible"
      :targets="batchRatingTargets"
      :execute="executeBatchRating"
      @completed="refreshAfterBatchRating"
    />

    <!-- 保存检索条件命名弹窗（第七批 PR3，PRD D4/D7）：名称必填（重名拒绝由后端
         Save 返回，handleError 透传 toast）；条件摘要预览展示快照全维（facet 时间
         维展示解析后的绝对区间——相对档保存即冻结为区间，摘要与调取还原口径一致）。
         小弹窗分档（min() 响应式，frontend-visual-conventions） -->
    <el-dialog
      v-model="saveSearchDialogVisible"
      title="保存检索条件"
      width="min(480px, 80vw)"
      :close-on-click-modal="false"
      append-to-body
    >
      <el-form label-width="88px" @submit.prevent>
        <el-form-item label="条件名称">
          <el-input
            v-model="saveSearchForm.name"
            placeholder="例如：近期高分方案"
            maxlength="50"
            :disabled="savingSearch"
            @keyup.enter="onConfirmSaveSearch"
          />
        </el-form-item>
      </el-form>
      <div class="save-search-summary">
        <div class="save-search-summary-title">将保存以下检索条件</div>
        <div v-for="row in saveSearchSummaryRows" :key="row.label" class="save-search-summary-row">
          <span class="save-search-summary-label">{{ row.label }}</span>
          <span class="save-search-summary-value">{{ row.value }}</span>
        </div>
      </div>
      <template #footer>
        <el-button :disabled="savingSearch" @click="saveSearchDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingSearch" @click="onConfirmSaveSearch">保存</el-button>
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
  UpdateAssetSlideMeta,
  BatchUpdateAssetFileTags,
  BatchUpdateAssetSlideTags,
  BatchSetAssetRating,
  SearchAssets,
  ListAssetSavedSearches,
  SaveAssetSavedSearch,
  DeleteAssetSavedSearch,
  OpenDirectoryDialog,
  AssetThumbsAvailable,
  JumpToAssetSlide,
  RetryAssetThumb,
  CancelAssetThumbs,
  SplitAssetFile,
  SplitAssetLibrary,
  CancelAssetSplit,
  GetAssetSplitRecord,
  OpenAssetSplitPage,
  GetSettings,
  SaveSettings
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { handleError, ErrorCode } from '../utils/error'
// 素材展示格式化共享单点（网格卡与列表行同一映射，禁本地复制防两份口径漂移）
import {
  assetStatusLabel as statusLabel,
  assetStatusTagType as statusTagType,
  formatAssetSize as formatSize
} from '../utils/assetFormat'
// 选中键映射共享单点（网格卡勾选/列表行勾选/全选/反选四处消费同一映射）
import { assetSelectionKey } from '../utils/assetSelection'
import AssetsIcon from './icons/AssetsIcon.vue'
import AssetThumb from './AssetThumb.vue'
import AssetListView from './AssetListView.vue'
import AssetPreviewOverlay from './AssetPreviewOverlay.vue'
import AssetSelectionBar from './AssetSelectionBar.vue'
// 标签树消费组件（第六批，PRD D6）：筛选/详情编辑复用同一树形多选组件 + 标签管理弹窗
import AssetTagTreeSelect from './AssetTagTreeSelect.vue'
import AssetTagManagerDialog from './AssetTagManagerDialog.vue'
// 批量打标签弹窗（第六批 PR3，PRD D3/D4/D5）：targets 构建与两批量 API 分派在宿主，
// 弹窗不感知 viewMode（只收 targets 与 execute 分派回调）
import AssetBatchTagDialog from './AssetBatchTagDialog.vue'
// 批量评分弹窗（第八批，SRS §2.6 P0，PRD D4/D5）：形态复刻批量打标签弹窗，
// targets 反查链路共用，execute 分派回调以星级值调 BatchSetAssetRating
import AssetBatchRatingDialog from './AssetBatchRatingDialog.vue'
// facet 多维筛选弹层（第七批 SRS §2.5）：受控组件 + 档位→检索参数换算单点（具名
// 导出）；buildFacetOptions 每次检索调用时现算（相对档边界随检索时刻刷新），
// countActiveFacets 驱动 searchMode 判定与 badge，createDefaultFacetState 供
// 初始化与「清除全部筛选」复位（形态单一来源）
import AssetFilterPopover, {
  buildFacetOptions,
  countActiveFacets,
  createDefaultFacetState,
  restoreFacetStateFromSnapshot,
  describeFacetState
} from './AssetFilterPopover.vue'
// 标签管理入口按钮图标（EP 图标族，浅色工具条容器内用 el-icon 承载）
import { CollectionTag, ArrowDown } from '@element-plus/icons-vue'

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
// 呈现维度（SRS §2.3 PR1）：'grid' | 'list'，仅切换呈现形态不改数据语境（分页
// 累计/检索结果保留，对齐 PRD D5#5）；会话态不持久化（不进 SaveSettings 链路，
// 重开应用回默认网格）
const displayMode = ref('grid')
// 全屏预览态（SRS §2.3 PR2）：null=关闭，数字=预览序列当前下标。序列与详情装配见
// previewSlides/selectSlideCard；语境变更点位（applySearch/onViewModeChange/
// onFlattenChange/onSlideTagOwnChange/切库 watch/resetAndReloadView）统一关闭——
// 与 PR3 选中清空同批点位，本批只做预览关闭
const previewIndex = ref(null)
// 卡片大小滑块（D6 真机增量，PRD D4）：经 --asset-card-w CSS 变量驱动网格固定
// 卡宽，会话态不持久化（不进 SaveSettings 链路，切库/重开应用回默认 280）
const cardWidth = ref(280)
// 文件内过滤态（D6 真机增量）：双击文件卡进入「仅看该文件页」浏览语境，值 =
// 过滤目标文件相对路径；空串 = 非过滤态。清空点位与预览/选中同批语境变更函数
// （切库/检索/视图切换/平铺/仅自身标签/扫描刷新）
const fileFilterPath = ref('')
// 选中集合（SRS §2.3 PR3，PRD D2）：Set 形态，键映射见 utils/assetSelection 共享
// 单点。响应式策略：整体替换新 Set（new Set(selectedKeys.value) 拷贝后增删再赋值）
// 触发引用变化——不依赖 Vue 对 Set 变异方法（add/delete）的响应式拦截（项目既有
// 「数组当 Set 用」先例 failedThumbPaths 同款规避思路；整体替换口径变更点少、
// 语义直白，放弃数组口径的理由是全选/反选需要集合运算，Set 原生语义更贴切）
const selectedKeys = ref(new Set())
const flatten = ref(true) // 检索聚合折叠/平铺开关（SearchAssets opts.flatten；D8#2 默认平铺打开——同一 PPT 多页命中逐页平铺为主路径，聚合折叠按需关回）
// 排序档位与方向（SRS §2.5.3，SearchAssets opts.sortBy/sortDir，取值对齐 model
// AssetSortBy* 常量）：'relevance' 相关性（默认，FTS bm25 序，无向）/'mtime'
// 修改时间/'rating' 星级——两档均为文件级口径（幻灯片/聚合条目继承归属文件属性，
// 与星级 facet 口径同源，页级 slide_meta.rating 不参与排序），排序在后端分页前
// 对全量命中执行（分页懒加载下前端内存排序必错段）。'desc' 降序默认/'asc' 升序，
// 仅 mtime/rating 档有意义。排序属检索条件维度：跨库保留（对齐关键词/标签/facet
// 先例，切库 watch 不复位）；快照随检索条件持久化（buildSavedSearchSnapshot，
// D3），不做用户偏好持久化
const sortBy = ref('relevance')
const sortDir = ref('desc')
const keywordInput = ref('') // 输入框绑定值（未应用）
const activeKeyword = ref('') // 已应用关键词（非空即检索模式）
// 标签筛选（第六批起树形多选）：ID 数组口径与后端 opts.tagIds 一致（非空即检索
// 模式）；级联展开（选父含后代）由后端检索链完成，前端不展开
const selectedTagIds = ref([])
// 标签筛选继承开关（SearchAssets opts.slideTagOnlyOwn）：false = 幻灯片条目【页自
// 身标签 ∪ 归属文件标签】合并命中（默认向后兼容）；true = 仅页自身标签命中
const slideTagOnlyOwn = ref(false)
// facet 三维筛选快照（第七批 SRS §2.5，D1/D2/D3）：UI 档位形态的受控快照（组件
// 受控、宿主持有），检索参数经 buildFacetOptions 在检索调用时现算——相对档边界
// 随检索时刻刷新，禁在此缓存换算结果。星级/时间/大小为库无关条件，切库保留
//（对齐关键词/标签跨库保留先例，见 watch(currentLibraryId) 注释与 §8.2 矩阵）
const facetState = ref(createDefaultFacetState())
// facet 生效维度数（0-3，badge 口径）：>0 即 searchMode 激活（浏览语境切
// facets-only 分页链路，D3），与后端 assetsFacetFilter.active 判定口径对齐
const facetActiveCount = computed(() => countActiveFacets(facetState.value))

// 排序触发按钮人读标签（相关性无向不标方向；mtime/rating 标当前升降）
const sortTriggerLabel = computed(() => {
  if (sortBy.value === 'mtime') return `修改时间 ${sortDir.value === 'asc' ? '↑' : '↓'}`
  if (sortBy.value === 'rating') return `评分 ${sortDir.value === 'asc' ? '↑' : '↓'}`
  return '排序：相关性'
})

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
// 幻灯片独立标注草稿（SRS §2.2.4，与文件级 detailDraft 相互独立）：tags 为页自
// 有标签名列表（保存经 UpdateAssetSlideMeta 整体替换）
const slideDraft = ref({ rating: 0, note: '', tags: [] })
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
// 失败缩略图自动补一轮标记（SRS §2.3 PR3，PRD D4，非响应式）：本库批次已自动
// 重试过即置位，下轮 done 不再自动触发（恰好一轮；手动重试不受限）；切库 watch
// 复位——新库新批次重新获得一轮自动补资格
let thumbAutoRetried = false
const jumping = ref(false)
const retrying = ref(false)

// ---- 拆分状态（assets-split:*，SRS §2.2.3 / §2.2.5 方式 2）----
const splitting = ref(false)
const splitProgress = ref(null) // assets-split:progress 载荷 {libraryId, done, total, currentFile, pageDone, pageTotal}
const splitDialogVisible = ref(false)
const splitDialogCtx = ref({ scope: 'library', relPath: '', fileName: '' }) // 确认弹窗语境（scope: 'file'|'library'）
const splitTarget = ref('cache') // 落点选择：'cache'=素材库缓存目录（默认推荐）| 'source'=源文件所在目录
const splitConfirming = ref(false) // 确认提交中（防重复提交）
const splitOutcomeVisible = ref(false)
const splitOutcomeItems = ref([]) // 批量拆分逐文件明细（failed>0 时弹窗展示）
const splitRecord = ref(null) // 详情文件的拆分记录（GetAssetSplitRecord 结果，exists 决定方式 2 按钮显隐）
const openingSplitPage = ref(false)
// 拆分代次：拆分启动 / 切库复位时自增；在途 Promise 落定写回前比对，代次已变
// 即丢弃（旧库任务态/提示不得污染新库语境，模式对齐本文件 librarySeq）
let splitSeq = 0

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

// 检索模式判定：关键词/标签/facet 任一生效（对齐后端空查询语义重定义——无关键
// 词无标签但有 facet 走 facets-only 文件清单路径，全空返回空结果页；前端 facet
// 生效即不回落清单浏览，改走检索分页链路）
const searchMode = computed(
  () => activeKeyword.value.trim() !== '' || selectedTagIds.value.length > 0 || facetActiveCount.value > 0
)

const scanPercent = computed(() => {
  if (!scanProgress.value || !scanProgress.value.total) return 0
  return Math.round((scanProgress.value.done / scanProgress.value.total) * 100)
})

const thumbsPercent = computed(() => {
  if (!thumbsProgress.value || !thumbsProgress.value.total) return 0
  return Math.round((thumbsProgress.value.done / thumbsProgress.value.total) * 100)
})

const splitPercent = computed(() => {
  if (!splitProgress.value || !splitProgress.value.total) return 0
  return Math.round((splitProgress.value.done / splitProgress.value.total) * 100)
})

// 页级进度文案（当前文件拆到第几页；无页级计数时不展示）
const splitPageText = computed(() => {
  const p = splitProgress.value
  if (!p || !p.pageTotal) return ''
  return `第 ${p.pageDone}/${p.pageTotal} 页`
})

// 单文件拆分入口显隐：文件详情 + .pptx 扩展名（大小写不敏感；后端对状态/索引
// 记录仍有最终校验，此处仅收窄入口减少必然失败的提交）
const isDetailFilePptx = computed(() => {
  if (selected.value?.kind !== 'file') return false
  return /\.pptx$/i.test(selected.value?.fileEntry?.path || '')
})

// 方式 2 按钮显隐：详情为幻灯片且归属文件已有拆分记录（隐藏非禁用）
const detailSplitExists = computed(() => {
  if (selected.value?.kind !== 'slide') return false
  return !!splitRecord.value?.exists
})

// 切库复位缩略图运行态：done/error 事件按 libraryId 过滤，批次运行中切库后旧库
// 终态事件不会命中新语境——不复位则进度条在新库页面永挂，且「取消生成」打到新库
// 恒返 false 与可见进度条自相矛盾。失败集合为旧库相对路径口径，一并清空防误归因
// （切回原库后由下一批 progress/done 重新收集，重试链路可恢复）。
// 拆分任务态同理复位：在途任务不被打断（后台继续跑），仅复位 UI 态——旧库
// done/error 事件因 libraryId 不匹配被忽略，不复位则进度条永挂。splitSeq 自增
// 使旧库在途拆分 Promise 的落定写回全部失效（防误清新库任务进度态 + 旧库
// toast 误弹，onConfirmSplit 写回前比对）。
watch(currentLibraryId, () => {
  splitSeq++
  // facet 快照与关键词/标签同批保留（§8.2 矩阵登记）：星级/时间/大小为库无关
  // 条件，切库落定后按检索语境在新库重查（facets-only 或组合检索随语境自然分流）
  closePreview() // 语境变更点：切库清预览（序列属库语境，驻留即串库页）
  clearSelection() // 语境变更点：切库清选中（键属库语境，驻留即幽灵选中，D5#2）
  fileFilterPath.value = '' // 语境变更点：切库清文件过滤（过滤目标属库内路径）
  thumbAutoRetried = false // 新库新批次：恢复一轮失败缩略图自动补资格（D4）
  thumbsRunning.value = false
  thumbsProgress.value = null
  lastThumbsFailedCount = 0
  failedThumbPaths.value = []
  splitConfirming.value = false
  splitting.value = false
  splitProgress.value = null
  splitRecord.value = null
})

// ---- 标签共享列表（第六批起独立数据源，PRD D1：ListAssetTags 独立标签 API）----
// 树形选择组件（AssetTagTreeSelect）拉取成功后经 loaded 事件回传平铺清单，宿主
// 据此维持一份共享列表，供详情编辑 ID↔名称映射、页自有标签回构（buildOwnTags
// FromNames）等场景消费（含未使用标签与直接绑定计数，替代原「接口补齐前从文件
// 底册聚合」的明账）。底册聚合降级为兜底：组件尚未完成首拉或拉取失败且底册已
// 就绪时以聚合值顶上，保底选项不断链；聚合口径仅含文件标签（页自有标签缺席），
// 兜底期详情映射可能少选项，属降级可接受面（重新打开下拉即重拉恢复）
const assetTags = ref([])
const onAssetTagsLoaded = (items) => {
  assetTags.value = items || []
}

const tagOptions = computed(() => {
  if (assetTags.value.length > 0) return assetTags.value
  const byId = new Map()
  for (const entry of fileEntries.value) {
    for (const tag of entry.tags || []) {
      if (!byId.has(tag.id)) byId.set(tag.id, tag)
    }
  }
  return Array.from(byId.values())
})

// 标签名↔ID 双向映射（详情编辑树形选择的适配层，见 detailTagIds/slideTagIds 注释）
const tagIdByName = computed(() => new Map(tagOptions.value.map(t => [t.name, t.id])))
const tagNameById = computed(() => new Map(tagOptions.value.map(t => [t.id, t.name])))

// 「仅使用幻灯片自身标签」开关作用域：仅幻灯片视图可操作（SRS §2.2.4 开关只
// 作用于幻灯片条目筛选；文件视图下检索结果只含文件条目，开关不产生可见效果，
// 禁用 + tooltip 声明作用域——恒渲染占位，防视图切换时同行控件跳动）
const isSlideView = computed(() => viewMode.value === 'slide')
const slideTagOwnTooltip = computed(() =>
  isSlideView.value
    ? '开启后标签筛选仅匹配幻灯片页自身标签，不再合并归属文件标签（文件条目不受影响）'
    : '仅作用于幻灯片条目：请切换到幻灯片视图后使用'
)

// 详情编辑树形选择的双向映射适配层（PRD D6：组件统一 ID 口径，消费方映射）：
// get 把草稿标签名数组映射为 ID 数组供组件勾选回显，set 把组件勾选的 ID 数组
// 映射回名称写回草稿——保存契约（UpdateAssetFileMeta 的 tags / UpdateAssetSlideMeta
// 的 tagNames 均为标签名数组）在宿主侧保持不变，组件与存储两侧口径互不渗透。
// 草稿名恒来自后端标签对象（entry.tags / 命中 ownTags），共享标签列表为全量标签
// 表（页自有标签与文件标签同表存储，原 slideTagOptions 并集兜底随之删除），正常
// 均能解析；映射不中的名（共享列表尚未加载完成的极短窗口）在 get 端被丢弃——
// 窗口内组件树尚未渲染无用户交互可能，注释登记不设防
const detailTagIds = computed({
  get: () => detailDraft.value.tags
    .map(name => tagIdByName.value.get(name))
    .filter(id => id != null),
  set: (ids) => {
    detailDraft.value = {
      ...detailDraft.value,
      tags: (ids || []).map(id => tagNameById.value.get(id)).filter(Boolean)
    }
  }
})

const slideTagIds = computed({
  get: () => slideDraft.value.tags
    .map(name => tagIdByName.value.get(name))
    .filter(id => id != null),
  set: (ids) => {
    slideDraft.value = {
      ...slideDraft.value,
      tags: (ids || []).map(id => tagNameById.value.get(id)).filter(Boolean)
    }
  }
})

// ---- 标签管理弹窗（第六批，PRD D6/D7）----

const tagManagerVisible = ref(false)
const tagManagerPresetCreate = ref(false)
const filterTagSelectRef = ref(null)
const detailTagSelectRef = ref(null) // 文件/幻灯片详情两分支互斥渲染，同一 ref 复用

const openTagManager = () => {
  tagManagerPresetCreate.value = false
  tagManagerVisible.value = true
}

// 详情编辑「+ 新建标签」快捷入口（PRD D6）：预填管理弹窗为新增根标签表单态
const openTagManagerForCreate = () => {
  tagManagerPresetCreate.value = true
  tagManagerVisible.value = true
}

// 标签管理变更（新增/重命名/删除成功）后的刷新链（对齐 onSaveMeta 先例）：
// 1) 树组件 reload——打开时重拉 ListAssetTags，loaded 事件回传刷新宿主共享列表
//    （详情编辑映射与页自有标签回构随之取到最新标签表）；
// 2) 文件底册重拉——卡片/详情展示的标签名与绑定同步；
// 3) 检索态重跑——命中条目携带的标签对象同步（对齐保存链重搜口径）。
// 数据变更点位不清选中（PRD D5 同性质：批量打标签完成保留选中）；已打开详情的
// 草稿不自动重算（防覆盖未保存编辑，重新点选条目即得新数据——重命名后草稿中的
// 旧名需手动调整，属已登记边界）
const refreshAfterTagChange = async () => {
  filterTagSelectRef.value?.reload()
  detailTagSelectRef.value?.reload()
  if (!hasLibrary.value) return
  // 双代次守卫（对齐 refreshAfterBatchTag/onSaveSlideMeta 先例）：刷新 await 期间
  // 切库/扫描刷新（librarySeq 自增）或检索语境变更（searchSeq 自增）时，旧响应
  // 已被各加载函数内部守卫丢弃，本函数随之中止
  const libSeqAtStart = librarySeq
  const searchSeqAtStart = searchSeq
  try {
    const entries = await ListAssetFiles(currentLibraryId.value)
    if (libSeqAtStart !== librarySeq) return // 期间切库/扫描刷新：旧库底册丢弃
    fileEntries.value = entries || []
    if (searchMode.value) {
      await runSearch(true)
      if (searchSeqAtStart !== searchSeq) return // 检索语境已变：变更方已自清展开态
      // 标签变更（删除页自有标签等）改变标签合并命中口径，聚合展开缓存必须失效
      //（对齐 onSaveSlideMeta risk 2 清法：旧 aggChildren 早退命中=回读变更前口径）
      expandedAggPaths.value = []
      aggChildren.value = {}
      aggLoadingPaths.value = []
    }
  } catch (error) {
    if (isLibNotFound(error)) {
      await loadLibraries()
      return
    }
    handleError('刷新标签数据失败: ', error)
  }
}

// ---- 批量打标签（第六批 PR3，SRS §2.4，PRD D3/D4/D5/D7）与批量评分
//（第八批，SRS §2.6 P0，PRD D4/D5）---- 两批量弹窗共用 targets 反查构建与
// 刷新链（差异点：标签树 reload 仅打标签需要），分派回调各按语义独立。

const batchTagDialogVisible = ref(false)
// 批量目标清单 [{ fileId, slideNum?, label }]：label 为按 viewMode 拼装的定位展示
// 文案（文件模式 = 文件路径；幻灯片模式 = 「文件路径 · 第 N 页」），弹窗失败明细
// 行定位展示与结果条目反查共用（弹窗不感知 viewMode，契约见 AssetBatchTagDialog）
const batchTagTargets = ref([])
const batchRatingDialogVisible = ref(false)
const batchRatingTargets = ref([])

// 选中集目标反查单点（两批量弹窗共用）：selectedKeys 经当前渲染独立卡反查目标——
// 消费 assetSelectionKey 共享单点做键映射（禁第二处手拼键，utils/assetSelection
// 键形态契约），渲染集内匹配天然滤掉陈旧选中键（反查 miss 跳过并计数）
const collectSelectionTargets = () => {
  const targets = []
  for (const card of selectableCards.value) {
    if (!selectedKeys.value.has(assetSelectionKey(card))) continue
    if (viewMode.value === 'file') {
      targets.push({ fileId: card.id, label: card.filePath })
    } else {
      targets.push({ fileId: card.fileId, slideNum: card.slideNum, label: `${card.filePath} · 第 ${card.slideNum} 页` })
    }
  }
  // 反查 miss = 陈旧选中（键在集合但条目已不在渲染集，如底册刷新后文件被删），
  // 跳过并计数提示；全部失效不开弹窗（空目标提交无意义）
  const missCount = selectedKeys.value.size - targets.length
  if (missCount > 0) {
    ElMessage.warning(`${missCount} 项已失效跳过`)
  }
  return targets
}

const openBatchTagDialog = () => {
  const targets = collectSelectionTargets()
  if (targets.length === 0) return
  batchTagTargets.value = targets
  batchTagDialogVisible.value = true
}

const openBatchRatingDialog = () => {
  const targets = collectSelectionTargets()
  if (targets.length === 0) return
  batchRatingTargets.value = targets
  batchRatingDialogVisible.value = true
}

// 批量执行分派回调（弹窗经 props.execute 调用，op = {addTags, removeTags} 标签名
// 数组）：按 viewMode 分派两独立 API（PRD D3 两套存储形态在签名内显式化）——
// 文件模式提交 fileIDs 数组，幻灯片模式提交 (fileId, slideNum) 目标数组
//（键契约禁 slides.id，与 slide_tags 存储键同口径）。弹窗打开为模态，viewMode
// 在提交期间不会被切换，分派读取当下 viewMode 即与打开时构建的 targets 一致
const executeBatchTagUpdate = (op) => {
  if (viewMode.value === 'file') {
    return BatchUpdateAssetFileTags(
      currentLibraryId.value,
      batchTagTargets.value.map(t => t.fileId),
      op
    )
  }
  return BatchUpdateAssetSlideTags(
    currentLibraryId.value,
    batchTagTargets.value.map(t => ({ fileId: t.fileId, slideNum: t.slideNum })),
    op
  )
}

// 批量评分分派回调（弹窗经 props.execute 调用，入参 = 星级 0-5，0 = 清除）：
// 以打开时构建的 targets 调 BatchSetAssetRating（单一入口混合文件/页目标，
// slideNum 0 = 文件条目——targets 幻灯片形态携带 slideNum，文件形态字段缺省经
// `|| 0` 归一，键契约禁 slides.id）。弹窗打开为模态，viewMode 在提交期间不会被
// 切换，分派读取当下 viewMode 构建的反查 targets 与打开时一致
const executeBatchRating = (rating) => {
  return BatchSetAssetRating(
    currentLibraryId.value,
    batchRatingTargets.value.map(t => ({ fileId: t.fileId, slideNum: t.slideNum || 0 })),
    rating
  )
}

// 批量完成刷新链（弹窗 completed 事件，全部成功/部分成功均触发——每条目一事务
// 成功条目已写入不回滚，PRD D3）：对齐 refreshAfterTagChange/onSaveMeta 先例——
// 标签树 reload（仅打标签链需要：标签直接绑定计数可能变；评分不涉标签变更跳过）
// + 底册重拉 + 检索态重搜；幻灯片浏览模式补拉幻灯片清单首页（ownTags/页星级
// 快照随批量更新，详情草稿回读数据源，对齐 onSaveSlideMeta 浏览分支刷新口径），
// 过滤浏览语境复位首页后接续拉兜底（见分支内注释）。选中保留（PRD D5：数据变更
// 点位非语境变更，键 file:<id>/slide:<fileId>:<slideNum> 稳定不受重解析影响）。
// 双代次守卫（对齐 onSaveSlideMeta 先例）：刷新 await 期间切库/扫描刷新
//（librarySeq 自增）或检索语境变更（searchSeq 自增）时，旧响应已被各加载函数
// 内部守卫丢弃，本函数随之中止，不再触碰新语境的展开缓存
const refreshAfterBatchAnnotation = async ({ reloadTagTrees = true } = {}) => {
  if (reloadTagTrees) {
    filterTagSelectRef.value?.reload()
    detailTagSelectRef.value?.reload()
  }
  if (!hasLibrary.value) return
  const libSeqAtStart = librarySeq
  const searchSeqAtStart = searchSeq
  try {
    const entries = await ListAssetFiles(currentLibraryId.value)
    if (libSeqAtStart !== librarySeq) return // 期间切库/扫描刷新：旧库底册丢弃
    fileEntries.value = entries || []
    if (searchMode.value) {
      await runSearch(true)
      if (searchSeqAtStart !== searchSeq) return // 检索语境已变：变更方已自清展开态
      // 页自有标签变化改变标签合并命中口径，聚合展开缓存必须失效（对齐
      // onSaveSlideMeta risk 2 清法：旧 aggChildren 早退命中会回读保存前标注）
      expandedAggPaths.value = []
      aggChildren.value = {}
      aggLoadingPaths.value = []
    } else if (viewMode.value === 'slide') {
      await loadSlidesPage(true)
      // 过滤浏览语境兜底（真机冒烟修复）：复位首页把全局分页流拉回首页段，目标
      // 文件的页大概率不在已加载段，客户端 fileFilterPath 过滤收敛为 0 张——批量
      // 成功提示后当前页全部卡片消失。watch(fileFilterPath) 只在过滤值变化时触发
      // 续拉，刷新场景过滤值不变接不上，须在此显式兜底：对齐 filterByFile 进入
      // 路径接 ensureFilteredSlidesLoaded 续拉，直到目标文件页重新出现或全局拉完；
      // 非过滤语境 fileFilterPath 为空时该函数直接返回（复位首页语义不变），await
      // 期间清过滤/切库/切视图竞态由其内部每轮校验（过滤目标未变 + loadingMore
      // 防重）自然退出
      await ensureFilteredSlidesLoaded()
    }
  } catch (error) {
    if (isLibNotFound(error)) {
      await loadLibraries()
      return
    }
    handleError('刷新标签数据失败: ', error)
  }
}

// 批量打标签 completed 入口（含标签树 reload）
const refreshAfterBatchTag = () => refreshAfterBatchAnnotation({ reloadTagTrees: true })
// 批量评分 completed 入口（评分不涉标签变更，跳过标签树 reload）
const refreshAfterBatchRating = () => refreshAfterBatchAnnotation({ reloadTagTrees: false })

// ---- 保存检索条件（第七批 SRS §2.5 PR3，PRD D4/D7）----

const saveSearchDialogVisible = ref(false)
const savingSearch = ref(false)
const saveSearchForm = ref({ name: '' })
// 已存清单（「已存搜索」弹层打开即重拉，变更后重拉——List 恒权威，本地不缓存增删）
const savedSearches = ref([])
const savedSearchesLoading = ref(false)
const savedSearchPopoverRef = ref(null)
// 行级防重占位：覆盖/删除在途条目 ID（0 = 空闲），防同条目连点重复提交
const savedSearchMutatingId = ref(0)

// 当前检索状态快照 → model.AssetSavedSearch 形态（D7 全维入快照）。tagNames 从
// 共享标签列表按 id 反查冗余（与 tagIds 同序对应，映射 miss 的成对丢弃防两份数据
// 错位——正常态 id 恒来自同一标签表不会 miss）；facet 三维经换算单点存区间原始值
//（AssetSavedSearch 字段口径，ratings/mtimeFrom/mtimeTo/sizeFrom/sizeTo，调取时
// 经 restoreFacetStateFromSnapshot 反推 UI 档位）；createdAt 由后端权威生成不传
const buildSavedSearchSnapshot = (name, id = 0) => {
  const tagPairs = selectedTagIds.value
    .map(tagId => ({ tagId, name: tagNameById.value.get(tagId) }))
    .filter(pair => pair.name)
  return {
    id,
    name,
    libraryId: currentLibraryId.value,
    keyword: activeKeyword.value.trim(),
    tagIds: tagPairs.map(pair => pair.tagId),
    tagNames: tagPairs.map(pair => pair.name),
    ...buildFacetOptions(facetState.value),
    flatten: flatten.value,
    slideTagOnlyOwn: slideTagOnlyOwn.value,
    // 排序快照（SRS §2.5.3，D3）：档位与方向随检索条件一并保存，调取时还原；
    // 旧快照缺字段调取侧容错回退默认（applySavedSearch）
    sortBy: sortBy.value,
    sortDir: sortDir.value
  }
}

const openSaveSearchDialog = () => {
  saveSearchForm.value = { name: '' }
  saveSearchDialogVisible.value = true
}

// 保存弹窗条件摘要（D7 预览面）：快照字段的人读形态，与 buildSavedSearchSnapshot
// 同一状态源（activeKeyword/selectedTagIds/tagOptions/facetState/两开关/排序档位），
// 摘要与实存内容不会漂移；facet 摘要经 describeFacetState 单点产出，排序摘要经
// sortSummaryText 单点产出
// 排序摘要人读形态（保存弹窗条件摘要行）：与快照 sortBy/sortDir 同一状态源，
// 摘要与实存内容不会漂移
const sortSummaryText = computed(() => {
  if (sortBy.value === 'mtime') return `修改时间·${sortDir.value === 'asc' ? '升序' : '降序'}`
  if (sortBy.value === 'rating') return `评分·${sortDir.value === 'asc' ? '升序' : '降序'}`
  return '相关性（默认）'
})

const saveSearchSummaryRows = computed(() => {
  const facetText = describeFacetState(facetState.value)
  const tagNames = selectedTagIds.value
    .map(id => tagNameById.value.get(id))
    .filter(Boolean)
  return [
    { label: '关键词', value: activeKeyword.value.trim() || '无' },
    { label: '标签', value: tagNames.length > 0 ? tagNames.join('、') : '无' },
    { label: '星级', value: facetText.ratingText },
    { label: '修改时间', value: facetText.mtimeText },
    { label: '文件大小', value: facetText.sizeText },
    { label: '平铺聚合', value: flatten.value ? '开启（命中逐页平铺）' : '关闭（同 PPT 命中折叠）' },
    { label: '仅幻灯片自身标签', value: slideTagOnlyOwn.value ? '开启' : '关闭' },
    { label: '排序', value: sortSummaryText.value }
  ]
})

const onConfirmSaveSearch = async () => {
  const name = saveSearchForm.value.name.trim()
  if (!name) {
    ElMessage.warning('请输入保存条件名称')
    return
  }
  savingSearch.value = true
  try {
    // 新增（id=0）走后端 upsert 新建分支：重名拒绝/名称校验由后端返回，经
    // handleError 透传 toast（文案含换名或覆盖引导）
    await SaveAssetSavedSearch(buildSavedSearchSnapshot(name))
    ElMessage.success('检索条件已保存')
    saveSearchDialogVisible.value = false
    await reloadSavedSearches()
  } catch (error) {
    handleError('保存检索条件失败: ', error)
  } finally {
    savingSearch.value = false
  }
}

const reloadSavedSearches = async () => {
  savedSearchesLoading.value = true
  try {
    savedSearches.value = (await ListAssetSavedSearches()) || []
  } catch (error) {
    handleError('加载保存的检索条件失败: ', error)
  } finally {
    savedSearchesLoading.value = false
  }
}

// 本库/异库分组（D4 全局存储带 libraryId）：本库组可调取可覆盖；异库组仅调取
//（降级）与删除——覆盖以当前检索状态整体替换条目并按当前库重写 libraryId，
// 在异库语境操作会让条目静默搬家，禁用（见弹层按钮 title 说明）
const currentLibrarySavedSearches = computed(() =>
  savedSearches.value.filter(item => item.libraryId === currentLibraryId.value)
)
const otherLibrarySavedSearches = computed(() =>
  savedSearches.value.filter(item => item.libraryId !== currentLibraryId.value)
)

// 异库条目的归属库名展示（库已删除时降级「未知库」——注册表已无该 ID）
const savedSearchLibraryName = (item) =>
  libraries.value.find(lib => lib.id === item.libraryId)?.name || '未知库'

// 覆盖可用性：有检索条件（可快照）+ 本库条目 + 无在途行级变更
const canOverwriteSavedSearch = (item) =>
  searchMode.value &&
  item.libraryId === currentLibraryId.value &&
  savedSearchMutatingId.value === 0

// 调取保存条件：完整还原检索语境（D7）——关键词/标签/facet/平铺/仅自身标签回填
// 后走 applySearch 与手动改条件同路径（searchSeq 代次守卫 + 清详情/预览/选中/文件
// 过滤/聚合展开缓存复位面由其统一执行，§8.2 矩阵登记「保存条件调取」点位）。
// 异库条目降级（D4）：tagIds 属保存时所在库不可复用，按 tagNames 在当前库标签表
// 重查 ID，miss 丢弃该标签并计数提示；同库条目 tagIds 直接可用（保存时所在库即
// 当前库，标签删除导致的陈旧 ID 由检索零命中自然呈现，可手动清除）
const applySavedSearch = async (item) => {
  let tagIds
  if (item.libraryId === currentLibraryId.value) {
    tagIds = Array.isArray(item.tagIds) ? [...item.tagIds] : []
  } else {
    const names = Array.isArray(item.tagNames) ? item.tagNames : []
    const remapped = names
      .map(name => tagIdByName.value.get(name))
      .filter(id => id != null)
    if (remapped.length < names.length) {
      ElMessage.warning(`${names.length - remapped.length} 个标签在当前库不存在，已忽略`)
    }
    tagIds = remapped
  }
  keywordInput.value = item.keyword || ''
  selectedTagIds.value = tagIds
  slideTagOnlyOwn.value = !!item.slideTagOnlyOwn
  flatten.value = !!item.flatten
  // 排序快照还原（SRS §2.5.3，D3）：旧快照缺字段（JSON 容错反序列化为 undefined）
  // 回退默认相关性/降序，向后兼容无版本号
  sortBy.value = item.sortBy || 'relevance'
  sortDir.value = item.sortDir || 'desc'
  facetState.value = restoreFacetStateFromSnapshot(item)
  // 调取后面板收起（结果区让位）；桩环境无该方法时静默跳过
  savedSearchPopoverRef.value?.hide?.()
  await applySearch()
}

// 覆盖更新：以当前检索状态快照整体替换条目内容（Save 同 id 走后端更新分支，
// 名称沿用条目原名——MVP 无改名语义，createdAt 由后端保留原值）
const overwriteSavedSearch = async (item) => {
  if (!canOverwriteSavedSearch(item)) return
  savedSearchMutatingId.value = item.id
  try {
    await SaveAssetSavedSearch(buildSavedSearchSnapshot(item.name, item.id))
    ElMessage.success(`已覆盖更新「${item.name}」`)
    await reloadSavedSearches()
  } catch (error) {
    handleError('覆盖更新失败: ', error)
  } finally {
    savedSearchMutatingId.value = 0
  }
}

const deleteSavedSearch = async (item) => {
  if (savedSearchMutatingId.value !== 0) return
  try {
    await ElMessageBox.confirm(
      `确定删除保存的检索条件「${item.name}」？`,
      '删除保存条件',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  savedSearchMutatingId.value = item.id
  try {
    await DeleteAssetSavedSearch(item.id)
    ElMessage.success('已删除')
    await reloadSavedSearches()
  } catch (error) {
    handleError('删除保存条件失败: ', error)
  } finally {
    savedSearchMutatingId.value = 0
  }
}

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
  // 文件内过滤（D6 真机增量）：过滤态仅显示目标文件的页（非过滤态全量透传）；
  // 过滤是全局分页流上的客户端收敛——目标页不在已加载段时由 ensureFilteredSlidesLoaded
  // 自动续拉（canLoadMoreSlides 判定与统计行口径均不受过滤影响）
  return slideEntries.value
    .filter(s => !fileFilterPath.value || s.filePath === fileFilterPath.value)
    .map(slide => ({
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

// 全屏预览序列（PRD D1）：displayCards 过滤幻灯片子序列——浏览幻灯片清单已加载段 /
// 检索命中幻灯片条目（含聚合父卡）。聚合展开子项（aggChildren）不在 displayCards，
// 不进序列（取舍登记：预览只翻独立卡片）；文件视图 displayCards 全为文件条目 →
// 序列空 → 预览入口隐藏非禁用
const previewSlides = computed(() => displayCards.value.filter(card => card.kind === 'slide'))

const canLoadMore = computed(() => searchMode.value && searchItems.value.length < searchTotal.value)

// 浏览模式幻灯片视图的分页懒加载判定（文件视图为全量清单无分页）
const canLoadMoreSlides = computed(() =>
  !searchMode.value && viewMode.value === 'slide' && slideEntries.value.length < slideTotal.value
)

// 列表呈现的懒加载提示判定：检索模式对齐网格 canLoadMore；浏览幻灯片对齐
// canLoadMoreSlides（网格浏览分页无提示是既有形态，列表提示补齐不回改网格行为）
const listCanLoadMore = computed(() => (searchMode.value ? canLoadMore.value : canLoadMoreSlides.value))

// 文件过滤激活态（D6 真机增量）：过滤目标存在且处于幻灯片视图。过滤属幻灯片
// 浏览语境——溯源切到文件视图后残留值由本条件屏蔽（文件视图不显示过滤条，
// 清空在手动切回幻灯片视图的 onViewModeChange 兜底）
const isFileFilterActive = computed(() => !!fileFilterPath.value && viewMode.value === 'slide')
// 过滤条文件名：相对路径末段（目录层级不进提示行，长路径截断观感差）
const fileFilterName = computed(() => {
  const p = fileFilterPath.value
  if (!p) return ''
  const segs = p.split(/[\\/]/)
  return segs[segs.length - 1] || p
})
// 过滤后已加载页数（过滤条计数；自动续拉完成前可能暂小于该文件实际页数，
// 续拉到位后自动增长至该文件总页数或全局拉完）
const filteredSlideCount = computed(() =>
  slideEntries.value.filter(s => s.filePath === fileFilterPath.value).length
)

// 检索空态文案：命中存在但当前视图类型过滤后为空时给引导。审核 11 的最小改法
// 取舍——选引导文案而非把 searchTotal 改为按视图过滤口径：searchTotal 同时驱动
// 懒加载判定（canLoadMore 依赖后端混合类型 total），改口径会破坏分页判定；切换
// 视图 / 开启平铺即可见全部命中，文案引导直达。facets-only 语境（无关键词无标签
// 纯 facet）后端只返回文件条目，幻灯片视图下恒空——文案直达「切文件视图」（平铺
// /幻灯片视图对该路径无意义）；facet 激活零命中给「调整筛选条件」文案（D6，配套
// 空态模板的「清除全部筛选」按钮，两语境共用同一空态）。
const searchEmptyText = computed(() => {
  if (searchTotal.value > 0) {
    if (activeKeyword.value.trim() === '' && selectedTagIds.value.length === 0) {
      return `已按当前筛选条件命中 ${searchTotal.value} 个文件，切换到「文件视图」查看`
    }
    return `已命中 ${searchTotal.value} 条，但当前视图类型下没有可展示条目，试试切换文件/幻灯片视图或开启「平铺」`
  }
  if (facetActiveCount.value > 0) {
    return '当前筛选条件下无命中，可调整筛选条件。'
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
// 幻灯片详情继承标签（归属文件标签，继承语义不变）：只读弱化展示，编辑走
// slideDraft.tags（页自有标签，存储独立）
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

// 索引状态展示映射与大小格式化已收敛至 utils/assetFormat 共享单点（网格卡与
// 列表行消费同一映射），此处 import 别名保持模板既有引用名不变

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
  closePreview() // 语境变更点：扫描刷新/切库落定清预览（底册重解析条目序列已变）
  clearSelection() // 语境变更点：扫描刷新清选中（条目重解析底册 id 变，D5#4）
  fileFilterPath.value = '' // 语境变更点：扫描刷新清文件过滤（底册重解析路径集合已变）
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

// 排序档位变更（SRS §2.5.3，D1）：同档再点翻转升降（相关性无向不翻转），切档回
// 默认降序。排序属检索条件变更点位（§8.2 矩阵登记）：走 applySearch 同路径——
// searchSeq 代次守卫与清详情/预览/选中/文件过滤/聚合展开缓存复位面由其统一执行，
// 勿重复清理；浏览语境（非检索态）排序仅记录待下次检索生效，不触发清单重载
const onSortCommand = async (command) => {
  if (command === sortBy.value) {
    if (command === 'relevance') return
    sortDir.value = sortDir.value === 'desc' ? 'asc' : 'desc'
  } else {
    sortBy.value = command
    sortDir.value = 'desc'
  }
  if (searchMode.value) {
    await applySearch()
  }
}

// 应用搜索条件：输入框值落定为 activeKeyword；关键词与标签全空则退出检索回浏览
const applySearch = async () => {
  searchSeq++ // 语境变更点：改词/改筛选使在途检索写回失效
  activeKeyword.value = keywordInput.value.trim()
  closeDetail()
  closePreview() // 语境变更点：检索条件变更清预览（命中序列已变）
  clearSelection() // 语境变更点：检索语境变更清选中（D5#2，驻留即幽灵选中）
  fileFilterPath.value = '' // 语境变更点：检索与文件过滤互斥（filterByFile 退检索的同款语义）
  expandedAggPaths.value = []
  aggChildren.value = {}
  aggLoadingPaths.value = []
  if (!searchMode.value) {
    await loadView()
    return
  }
  await runSearch(true)
}

// facet 变更（弹层三维任一调整，双语境共用入口，D3）：数据过滤语境变更——复位面
// 对齐 applySearch 同类点位（清详情/预览/选中/文件过滤/聚合展开缓存），检索语境
// （关键词/标签在）并入组合重查；浏览语境 facet 激活切 facets-only 分页链路
//（SearchAssets 空关键词+空标签+facet，后端返回文件条目，底册退位）、facet 清除
// 回底册。代次守卫：searchSeq 先自增使在途检索写回失效，runSearch 内部快照比对
const onFacetChange = async () => {
  searchSeq++ // 语境变更点：facet 变更使在途检索写回失效
  closeDetail()
  closePreview() // 语境变更点：facet 变更清预览（命中序列已变）
  clearSelection() // 语境变更点：数据过滤语境变更清选中（§8.2 同类点位先例）
  fileFilterPath.value = '' // 语境变更点：facet 与文件过滤互斥（对齐检索语境同款语义）
  expandedAggPaths.value = []
  aggChildren.value = {}
  aggLoadingPaths.value = []
  if (searchMode.value) {
    await runSearch(true)
  } else {
    await loadView()
  }
}

// 清除全部筛选（空态引导按钮，D6）：facet + 标签一键复位（关键词保留——属「搜索」
// 非「筛选」，仍在则回关键词检索，全清回底册）。复位面对齐 onFacetChange 同批点位
const clearAllFilters = async () => {
  searchSeq++ // 语境变更点：筛选条件清空使在途检索写回失效
  facetState.value = createDefaultFacetState()
  selectedTagIds.value = []
  closeDetail()
  closePreview() // 语境变更点：筛选清空清预览（命中序列已变）
  clearSelection() // 语境变更点：检索语境变更清选中（§8.2 同类点位先例）
  fileFilterPath.value = '' // 语境变更点：对齐检索语境变更同批清空文件过滤
  expandedAggPaths.value = []
  aggChildren.value = {}
  aggLoadingPaths.value = []
  if (searchMode.value) {
    await runSearch(true)
  } else {
    await loadView()
  }
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
      slideTagOnlyOwn: slideTagOnlyOwn.value,
      // facet 三维条件（第七批）：检索调用时经换算单点现算（相对档边界随检索
      // 时刻刷新），spread 展开 ratings/mtimeFrom/mtimeTo/sizeFrom/sizeTo 五字段
      ...buildFacetOptions(facetState.value),
      // 排序档位（SRS §2.5.3）：档位与升降随检索下发，后端在分页前对全量命中
      // 排序保证跨页顺序一致（禁前端内存排序——分页懒加载下必错段）
      sortBy: sortBy.value,
      sortDir: sortDir.value,
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

// 懒加载分派单点（网格滚动与列表组件 load-more 事件共用）：检索模式加载检索
// 下一页；浏览模式幻灯片视图加载清单下一页（文件视图为全量清单无分页）。防重
// （loading/loadingMore）与分页判定集中此处——列表组件只上报「已滚动到底」事件，
// 不持分页知识，网格/列表两呈现共用同一份判定防漂移
const loadMoreInView = () => {
  if (loading.value) return
  if (searchMode.value) {
    if (!loadingMore.value && canLoadMore.value) runSearch(false)
  } else if (viewMode.value === 'slide' && !loadingMore.value && canLoadMoreSlides.value) {
    loadSlidesPage(false)
  }
}

const onGalleryScroll = () => {
  const el = galleryRef.value
  if (!el) return
  if (el.scrollTop + el.clientHeight < el.scrollHeight - 80) return
  loadMoreInView()
}

const onFlattenChange = () => {
  // 平铺开关只影响检索聚合形态；展开态随形态切换失效
  searchSeq++ // 语境变更点：聚合形态变化使在途检索写回失效
  expandedAggPaths.value = []
  aggChildren.value = {}
  aggLoadingPaths.value = []
  closePreview() // 语境变更点：聚合形态变更清预览（命中序列口径已变）
  clearSelection() // 语境变更点：检索语境变更清选中（D5#2）
  fileFilterPath.value = '' // 语境变更点：对齐检索语境变更同批清空文件过滤
  if (searchMode.value) {
    runSearch(true)
  }
}

// 「仅使用幻灯片自身标签」开关变更（对齐 onFlattenChange 的语境重置面）：检索
// 条件变更点——在途检索写回失效 + 聚合展开态清空（子项按旧筛选条件拉取）；
// 检索态立即重跑检索，浏览态仅记录条件待下次检索生效（开关不改变 searchMode
// 判定，无检索可重跑）
const onSlideTagOwnChange = () => {
  searchSeq++
  expandedAggPaths.value = []
  aggChildren.value = {}
  aggLoadingPaths.value = []
  closePreview() // 语境变更点：标签口径变更清预览（命中序列已变）
  clearSelection() // 语境变更点：检索语境变更清选中（D5#2）
  fileFilterPath.value = '' // 语境变更点：对齐检索语境变更同批清空文件过滤
  if (searchMode.value) {
    runSearch(true)
  }
}

const onViewModeChange = () => {
  // 视图切换：检索模式为客户端类型过滤（displayCards 已响应）；浏览模式目标清单
  // 为空即拉取——覆盖「库切换/扫描落定在幻灯片视图后切回文件视图」时文件底册未
  // 加载（审核 9 同根因）与幻灯片清单未加载两种形态
  closeDetail()
  closePreview() // 语境变更点：文件/幻灯片视图互换清预览（选中维度不通约）
  clearSelection() // 语境变更点：选中维度互换不通约，切视图清选中（D5#3）
  fileFilterPath.value = '' // 语境变更点：手动切视图即离开过滤语境（filterByFile 直赋不经此处，无自清问题）
  if (searchMode.value) return
  const targetEmpty = viewMode.value === 'file'
    ? fileEntries.value.length === 0
    : slideEntries.value.length === 0
  if (targetEmpty) loadView()
}

// ================= 选中体系（SRS §2.3 PR3，PRD D2/D5）=================

// 可勾选卡片集：当前渲染卡片剔除聚合父卡（hitCount>1 勾选语义歧义不渲染复选框，
// D5#1）——全选/反选作用域与浮动条计数口径的同一数据源（D5#8：检索含已加载段，
// 懒加载未加载段不进范围；聚合展开子项不在 displayCards，天然不进范围）
const selectableCards = computed(() => displayCards.value.filter(card => card.hitCount <= 1))

// 勾选态判定：消费共享键映射查选中集合（网格卡与列表行同键，D5#5 呈现切换保留）
const isCardChecked = (card) => selectedKeys.value.has(assetSelectionKey(card))

// 单卡勾选切换（网格卡与列表行 toggle-select 共用出口）：整体替换新 Set 触发响应
const toggleCardSelection = (card) => {
  const key = assetSelectionKey(card)
  if (!key) return
  const next = new Set(selectedKeys.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  selectedKeys.value = next
}

// 全选：当前渲染独立卡键集并入选中集合（已选中的重复并入幂等）
const selectAllRendered = () => {
  const next = new Set(selectedKeys.value)
  for (const card of selectableCards.value) {
    next.add(assetSelectionKey(card))
  }
  selectedKeys.value = next
}

// 反选：渲染键集内取反。口径登记（PRD D2「对称差」的落地收窄）：反选后选中集合
// = 渲染集 ∖ 当前选中——严格对称差中的「渲染集外旧选中」部分一并清除（从渲染集
// 重建新集合，结构上不可能保留渲染集外键），防语境残留的幽灵选中；渲染集外键
// 在 D5#2/#3/#4 清空点位兜底下理论不存在，此处为防御性收口
const invertSelection = () => {
  const next = new Set()
  for (const card of selectableCards.value) {
    const key = assetSelectionKey(card)
    if (!selectedKeys.value.has(key)) next.add(key)
  }
  selectedKeys.value = next
}

// 清空选中：D5#2/#3/#4 清空点位的统一出口（各语境变更函数内一行调用）
const clearSelection = () => {
  if (selectedKeys.value.size === 0) return
  selectedKeys.value = new Set()
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
  // 平铺/仅自身标签开关同属检索语境快照面（审查 P2）：在途展开期间切换（对应
  // onFlattenChange/onSlideTagOwnChange 已清空 aggChildren）时，旧口径响应若照常
  // 写回会让重展开在 aggChildren 缓存上早退，恒显旧筛选语义子项——快照纳入
  // contextChanged 比对（对齐 tagsAtStart 形态），发起参数同取快照值
  const flattenAtStart = flatten.value
  const onlyOwnAtStart = slideTagOnlyOwn.value
  const libSeqAtStart = librarySeq
  const contextChanged = () => libSeqAtStart !== librarySeq ||
    keywordAtStart !== activeKeyword.value.trim() ||
    tagsAtStart.length !== selectedTagIds.value.length ||
    tagsAtStart.some((tagId, i) => tagId !== selectedTagIds.value[i]) ||
    flattenAtStart !== flatten.value ||
    onlyOwnAtStart !== slideTagOnlyOwn.value
  try {
    // 聚合子项查询刻意不带 facet 条件（第七批）：facet 为文件维度判定，聚合父卡
    // 能出现即归属文件已通过 facet 筛选，同文件全部页继承同一判定结果——带上
    // facet 是语义 no-op，省一次重复换算；关键词/标签/平铺/仅自身标签仍需携带。
    // 排序参数同理不传（SRS §2.5.3）：mtime/rating 为文件级口径，同文件子项排序
    // 主键同值，后端次级键（score→path）序与相关性档 FTS 序一致，不带零行为差异
    const page = await SearchAssets(currentLibraryId.value, keywordAtStart, {
      flatten: true,
      tagIds: tagsAtStart,
      slideTagOnlyOwn: onlyOwnAtStart,
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
    selectSlideCard(card)
  }
}

// 幻灯片卡片详情装配单点：画廊/列表点击与预览翻页共用（D5#6 预览即详情导航——
// 翻页时详情侧栏同步跟随当前页）；选中切换即重置幻灯片标注草稿（画廊清单条目与
// 检索命中条目两条路径均初始化，数据源取条目自带 ownTags/note/rating）
const selectSlideCard = (card) => {
  const slide = card.slide || slideEntries.value.find(s => s.id === card.id) || null
  selected.value = { kind: 'slide', slideEntry: slide, hit: card.slide ? null : card.hit }
  resetSlideDraft()
}

const closeDetail = () => {
  selected.value = null
  savingMeta.value = false
}

// ---- 全屏预览（SRS §2.3 PR2）----

// 预览入口可用性：幻灯片详情 + 当前视图存在可预览序列（文件视图序列空 → 隐藏非禁用）
const canPreview = computed(() => {
  if (selected.value?.kind !== 'slide') return false
  return previewSlides.value.length > 0
})

// 打开预览：以当前详情页在序列中的下标定位（详情已定位到页，打开点语义）；聚合
// 展开子项进详情后不在序列（子项不进 previewSlides），按归属文件路径回退定位
// （聚合父卡同路径命中），仍 miss 落序列首页
const openPreview = () => {
  if (!canPreview.value) return
  const currentId = selected.value?.slideEntry?.id ?? selected.value?.hit?.refId
  let idx = previewSlides.value.findIndex(card => card.id === currentId)
  if (idx < 0 && detailFilePath.value) {
    idx = previewSlides.value.findIndex(card => card.filePath === detailFilePath.value)
  }
  previewIndex.value = idx >= 0 ? idx : 0
}

const closePreview = () => {
  previewIndex.value = null
}

// ---- 网格卡双击分派（D6 真机增量）----

// 文件卡双击 = 文件内过滤浏览；幻灯片卡双击 = 全屏预览（浏览器双击序
// click→click→dblclick，首击已装配该卡详情，openPreview 以当前详情定位序列
// 下标——语义闭环）；聚合父卡（hitCount>1）不动作：单击语义是展开/收起，双击
// 介入会与两次 click 的展开-收起交错混乱
const onCardDblclick = (card) => {
  if (card.hitCount > 1) return
  if (card.kind === 'file') {
    filterByFile(card)
  } else {
    openPreview()
  }
}

// 文件内过滤浏览（D6 真机增量）：双击文件卡切幻灯片视图仅显示该文件的页。
// 检索态先退检索（清全部检索条件并失效在途写回——过滤属浏览语境，两者互斥）；
// 直赋 viewMode 不经 onViewModeChange（编程赋值不触发 radio @change，无自清
// 问题，对齐 traceToSourceFile 直赋先例），选中/预览/详情按语境变更同批清空
// （D5#3 视图切换清选中，直赋路径须手动执行）
const filterByFile = async (card) => {
  const path = card.entry?.path ?? card.filePath
  if (!path) return
  if (searchMode.value) {
    searchSeq++ // 语境变更点：退出检索使在途检索写回失效
    activeKeyword.value = ''
    keywordInput.value = ''
    selectedTagIds.value = []
    facetState.value = createDefaultFacetState() // 语境变更点：过滤浏览与检索/facet 语境互斥，连 facet 一并复位（§8.2 矩阵登记）
    expandedAggPaths.value = []
    aggChildren.value = {}
    aggLoadingPaths.value = []
  }
  fileFilterPath.value = path
  viewMode.value = 'slide'
  closeDetail()
  closePreview() // 语境变更点：进入过滤浏览清预览（序列已按过滤收敛）
  clearSelection() // 语境变更点：视图切换清选中（D5#3，直赋路径手动清）
  // 幻灯片清单未拉过（切库/扫描落定后未进过幻灯片视图）：slideTotal 为 0 时
  // canLoadMoreSlides 恒假，先 await 拉首页才有续拉基准（不 await 则续拉判定
  // 读到旧 total=0 直接退出）；已拉过则直接续拉。await 后 ensureFilteredSlidesLoaded
  // 内部按 target 逐轮校验，期间清过滤/切库竞态自然退出
  if (slideEntries.value.length === 0 && slideTotal.value === 0) {
    await loadSlidesPage(true)
  }
  await ensureFilteredSlidesLoaded()
}

// 清除文件过滤：恢复该库幻灯片清单（已加载段照常展示，滚动懒加载续拉不变——
// 过滤是浏览流上的客户端收敛，清除只是撤掉收敛条件不重置分页）
const clearFileFilter = () => {
  fileFilterPath.value = ''
}

// 过滤空自动续拉：目标文件的页可能不在幻灯片清单已加载段（全局分页流），逐页
// 拉取直到目标文件页出现或全局拉完。每轮循环校验过滤目标未变（await 期间清除
// 过滤/切换语境的快速竞态立即退出）；loadingMore 条件防与滚动懒加载并发重复
// 请求（loadSlidesPage 本身无防重，防重在调用方——复用同款约定）
const ensureFilteredSlidesLoaded = async () => {
  const target = fileFilterPath.value
  if (!target) return
  while (
    fileFilterPath.value === target &&
    !slideEntries.value.some(s => s.filePath === target) &&
    canLoadMoreSlides.value &&
    !loadingMore.value
  ) {
    await loadSlidesPage(false)
  }
}

// 过滤目标变化即触发续拉（filterByFile 已手动调用，此处兜底其他写入路径；
// 幂等——续拉中 loadingMore 置位使重复触发的循环直接退出）
watch(fileFilterPath, (path) => {
  if (path) ensureFilteredSlidesLoaded()
})

// 预览翻页写回：详情侧栏同步跟随（D5#6 预览即详情导航）。已定位同页（打开预览的
// 进入点 / 翻回已选中页）不重选——防重置未保存的标注草稿
watch(previewIndex, (idx) => {
  if (idx === null) return
  const card = previewSlides.value[idx]
  if (!card || isCardSelected(card)) return
  selectSlideCard(card)
})

// 序列收缩兜底：保存刷新（loadSlidesPage(true) 复位首页）/ 检索重跑可能使序列短于
// 当前下标——越界即关闭预览，防「previewIndex 非空但 overlay 内层 v-if 不渲染」的
// 不可见卡死态（覆盖层不渲染时无法翻页也无法关闭）
watch(() => previewSlides.value.length, (len) => {
  if (previewIndex.value !== null && previewIndex.value >= len) closePreview()
})

const resetDraft = () => {
  const entry = selected.value?.fileEntry
  detailDraft.value = {
    rating: entry?.rating || 0,
    note: entry?.note || '',
    tags: (entry?.tags || []).map(t => t.name)
  }
}

// 幻灯片标注草稿重置：数据源优先 hit（检索命中详情保存后 hit 已刷新为最新标注；
// 浏览模式 hit 为空自然落到 slideEntry 清单条目），两路径（画廊条目/检索命中条目）
// 选中时草稿都能初始化
const resetSlideDraft = () => {
  const hit = selected.value?.hit
  const entry = selected.value?.slideEntry
  slideDraft.value = {
    rating: hit?.rating ?? entry?.rating ?? 0,
    note: hit?.note ?? entry?.note ?? '',
    tags: (hit?.ownTags ?? entry?.ownTags ?? []).map(t => t.name)
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

// 浏览清单双引用同步：检索语境保存后 slideEntries 可能仍持旧标注快照（清单未
// 随检索刷新），以刷新后的命中数据原地同步标注三字段（ownTags 为后端规范完整
// 对象，无形态偏差），防切回浏览视图再开该页时草稿读到保存前数据
const syncSlideEntryAnnotation = (entryId, hit) => {
  slideEntries.value = slideEntries.value.map(s =>
    s.id === entryId ? { ...s, rating: hit.rating, note: hit.note, ownTags: hit.ownTags || [] } : s
  )
}

// 按标签名回构页自有标签对象（形态对齐后端标签对象 {id,name,color,parentId}）：
// 从标签共享列表（ListAssetTags 数据源，第六批起含全量标签）尽力匹配完整对象
//（含真实 id/颜色），名不在列表中时合成占位对象（id 仅供 v-for key，展示层以
// name 为主），待下次刷新由服务端规范数据覆盖
const buildOwnTagsFromNames = (tagNames) => {
  const known = new Map(tagOptions.value.map(t => [t.name, t]))
  return (tagNames || []).map((name, i) =>
    known.get(name) || { id: `own-submitted-${i}-${name}`, name, color: '', parentId: 0 }
  )
}

// 刷新 miss 分支的就地回填：以刚保存提交值为权威源补齐条目标注三字段（字段面与
// syncSlideEntryAnnotation 一致），防详情草稿从保存前旧快照回退（审查 P1）
const applySubmittedAnnotation = (target, submitted) => ({
  ...target,
  rating: submitted.rating,
  note: submitted.note,
  ownTags: buildOwnTagsFromNames(submitted.tagNames)
})

// 保存幻灯片页级标注（整体替换语义对齐文件 onSaveMeta，入参形状对齐
// model.AssetSlideMetaUpdate）：星级/业务备注/页自有标签以本次提交值为准；
// 纯标注层写入（slide_tags/slide_meta），不触发重解析与缩略图失效。保存成功后
// 刷新当前条目所在清单（对齐文件 onSaveMeta 成功后的刷新模式：浏览模式重拉
// 幻灯片清单、检索模式重跑检索——取回含新建标签完整对象的规范数据），详情引用
// 同步指向新条目。刷新后按命中与否分流（审查 P1）：命中 → 引用同步为服务端
// 新数据后草稿重读权威数据；miss（保存条目在懒加载深页/聚合子项，刷新复位首页
// 后不在返回页）→ selected 引用仍持保存前旧快照，此时 resetSlideDraft 会回读
// 旧值（刚保存的标注在表单回退、再点保存即写回旧数据）——以提交值为权威源就地
// 回填引用三字段，草稿保留提交值不动（表单与数据一致，无回退）。
// 双守卫（审查 risk 1）：刷新 await 期间检索语境（平铺/仅自身标签开关切换——
// searchSeq 自增且不关详情）或库代次（切库/扫描刷新，librarySeq 自增）可能变化，
// 刷新响应已被 runSearch/loadSlidesPage 内部守卫丢弃（searchItems/slideEntries
// 仍持保存前旧数据），写回前比对发起时快照，语境已变即整体退出：不写回 selected
// （find 命中的是旧数据，写回即保存前快照）、不 reset 草稿（防表单回退旧值被再
// 次保存写回）；守卫计数器为普通 let 变量（非 ref），快照对齐 toggleAgg
// libSeqAtStart 既有形态
const onSaveSlideMeta = async () => {
  const slideEntry = selected.value?.slideEntry
  const hit = selected.value?.hit
  const fileId = slideEntry?.fileId ?? hit?.fileId
  const slideNum = slideEntry?.slideNum ?? hit?.slideNum
  if (!fileId || !slideNum) return
  savingMeta.value = true
  // 提交值快照：刷新 await 期间草稿可能被重置，miss 分支回填以保存时刻提交值为准
  const submitted = {
    rating: slideDraft.value.rating,
    note: slideDraft.value.note,
    tagNames: [...slideDraft.value.tags]
  }
  // 守卫快照：searchSeq 覆盖检索语境（切库/扫描刷新路径同样自增），librarySeq
  // 覆盖浏览语境的库切换/扫描刷新（浏览态 slideEntries 不受检索开关影响）
  const searchSeqAtStart = searchSeq
  const libSeqAtStart = librarySeq
  try {
    await UpdateAssetSlideMeta(currentLibraryId.value, fileId, slideNum, submitted)
    ElMessage.success('已保存')
    const entryId = slideEntry?.id ?? hit?.refId
    if (searchMode.value) {
      // 检索语境：重跑检索，详情命中引用同步指向刷新后的数据
      await runSearch(true)
      if (searchSeq !== searchSeqAtStart) return // 语境已变：在途响应已被守卫丢弃，不写回不重置
      // 页标注变化改变命中口径（页自身标签参与标签合并命中），旧展开缓存必须失效：
      // 聚合子项保存后重展开若经 aggChildren 早退命中旧缓存，详情回读保存前标注、
      // 再保存即写回旧数据——清空展开态强制重展开按当前口径重拉（审查 risk 2，
      // 对齐 onSlideTagOwnChange 清法）；置于守卫之后：语境已变 return 分支无需
      // 重复清（开关切换路径自身已清过）
      expandedAggPaths.value = []
      aggChildren.value = {}
      aggLoadingPaths.value = []
      const card = searchItems.value.find(c => c.kind === 'slide' && c.id === entryId)
      if (card && selected.value?.hit?.refId === entryId) {
        // 命中：引用已同步为服务端新数据，草稿重读权威数据
        selected.value = { ...selected.value, hit: card.hit }
        syncSlideEntryAnnotation(entryId, card.hit)
        resetSlideDraft()
      } else if (selected.value?.hit?.refId === entryId) {
        // miss：hit 按提交值回填（草稿保留提交值不动），浏览清单旧快照同步防回退
        const patched = applySubmittedAnnotation(selected.value.hit, submitted)
        selected.value = { ...selected.value, hit: patched }
        syncSlideEntryAnnotation(entryId, patched)
      }
    } else {
      // 浏览语境：重拉幻灯片清单（复位首页），详情条目引用同步指向新数据
      await loadSlidesPage(true)
      if (libSeqAtStart !== librarySeq) return // 保存期间切库/刷新：旧清单响应已被守卫丢弃，不写回不重置
      // 过滤浏览语境兜底（同 refreshAfterBatchTag 真机修复根因）：复位首页后目标
      // 文件的页大概率不在已加载段，客户端 fileFilterPath 过滤收敛 0 张致画廊全空
      // ——过滤视图点卡片进详情编辑保存即触发。对齐 filterByFile 进入路径接续拉
      // 兜底（内部每轮校验过滤目标未变 + loadingMore 防重，await 期间语境变更自然
      // 退出）；续拉到位后保存条目回到已加载段走命中分支以服务端数据回读，非过滤
      // 语境为空操作不改变既有 miss 回填语义。期间切库/扫描刷新路径 selected 已被
      // resetAndReloadView 同步前缀 closeDetail 置空，下方写回判定自然跳过
      await ensureFilteredSlidesLoaded()
      const updated = slideEntries.value.find(s => s.id === entryId)
      if (updated && selected.value?.slideEntry?.id === entryId) {
        // 命中：引用已同步为服务端新数据，草稿重读权威数据
        selected.value = { ...selected.value, slideEntry: updated }
        resetSlideDraft()
      } else if (selected.value?.slideEntry?.id === entryId) {
        // miss：条目按提交值回填（草稿保留提交值不动）
        selected.value = {
          ...selected.value,
          slideEntry: applySubmittedAnnotation(selected.value.slideEntry, submitted)
        }
      }
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
  facetState.value = createDefaultFacetState() // 语境变更点：直赋路径不经 applySearch，facet 须在此一并复位（§8.2 矩阵登记）
  viewMode.value = 'file'
  closeDetail()
  closePreview() // 语境变更点：溯源切文件视图清预览（文件视图无预览序列）
  // 溯源同时命中 D5#2（退出检索）与 D5#3（切文件视图）两个清空触发点，但走直赋
  // 不经 applySearch/onViewModeChange——漏清则幻灯片选中键驻留成幽灵选中（文件
  // 视图无可见勾选行，浮动条清空是唯一出口）
  clearSelection()
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

// ================= PPT 拆分（SRS §2.2.3 / §2.2.5 方式 2）=================

// 打开拆分确认弹窗（单文件 / 库级两入口共用）：重置落点为默认缓存目录
const openSplitDialog = (scope) => {
  if (splitting.value) return
  splitTarget.value = 'cache'
  if (scope === 'file') {
    const entry = selected.value?.fileEntry
    if (!entry) return
    splitDialogCtx.value = { scope, relPath: entry.path, fileName: entry.title || entry.path }
  } else {
    splitDialogCtx.value = { scope, relPath: '', fileName: '' }
  }
  splitDialogVisible.value = true
}

// 弹窗关闭回调：仅复位提交中态。在途拆分任务继续后台执行，进度经面板层进度条
// 持续展示（拆分监听器属面板层，随组件生命周期注销——见文件头注释取舍说明）
const onSplitDialogClosed = () => {
  splitConfirming.value = false
}

// 确认拆分：调对应绑定（单文件 SplitAssetFile / 库级 SplitAssetLibrary），汇总
// 提示由本 Promise 分支统一弹出（done 事件仅复位进度，防双提示）；进行中态防
// 重复提交。取消（E_ASSETS_CANCELLED）与真失败经 handleError 自动分流。
// 请求序号守卫：启动时捕获拆分代次，await 落定写回（任务态/提示/弹窗）前比对
// ——切库自增 splitSeq 后旧库 Promise 落定全部丢弃（防旧库 toast 误弹、误清
// 新库在途任务进度态）
const onConfirmSplit = async () => {
  if (splitConfirming.value || splitting.value) return
  const ctx = splitDialogCtx.value
  const toSource = splitTarget.value === 'source'
  const seq = ++splitSeq
  splitConfirming.value = true
  splitting.value = true
  splitProgress.value = null
  try {
    if (ctx.scope === 'file') {
      const result = await SplitAssetFile(currentLibraryId.value, ctx.relPath, toSource)
      if (seq !== splitSeq) return
      if (result?.skipped) {
        ElMessage.info(result.skipReason || '该文件已拆分且未变化，无需重复拆分')
      } else {
        ElMessage.success(`拆分完成：共 ${result?.pageCount ?? 0} 页`)
      }
    } else {
      const batch = await SplitAssetLibrary(currentLibraryId.value, toSource)
      if (seq !== splitSeq) return
      if (!batch || !batch.total) {
        ElMessage.info('当前库没有可拆分的 PPT 文件')
      } else {
        ElMessage.success(
          `拆分完成：共 ${batch.total} 个文件（拆分 ${batch.split ?? 0}，跳过 ${batch.skipped ?? 0}，失败 ${batch.failed ?? 0}）`
        )
        // 失败明细 >0：弹出逐文件明细（异常隔离原因：损坏/加密/只读等）
        if ((batch.failed ?? 0) > 0 && Array.isArray(batch.items) && batch.items.length > 0) {
          splitOutcomeItems.value = batch.items
          splitOutcomeVisible.value = true
        }
      }
    }
    if (seq !== splitSeq) return
    splitDialogVisible.value = false
  } catch (error) {
    if (seq !== splitSeq) return // 旧语境失败静默丢弃（handleError 不弹旧库 toast）
    handleError('拆分失败: ', error)
  } finally {
    // splitConfirming 归属本次提交点击，恒复位（旧 Promise 延迟落定不得把
    // 「提交中」残留阻塞后续提交）；splitting/splitProgress 属拆分任务运行态，
    // 代次已变时不复位（切库 watch 已复位过，且新库在途任务进度态不可被清除）
    splitConfirming.value = false
    if (seq === splitSeq) {
      splitting.value = false
      splitProgress.value = null
    }
  }
}

const onCancelSplit = async () => {
  if (!hasLibrary.value) return
  try {
    const requested = await CancelAssetSplit(currentLibraryId.value)
    if (requested) {
      ElMessage.info('已请求取消拆分')
    } else {
      ElMessage.warning('当前没有可取消的拆分任务')
    }
  } catch (error) {
    handleError('取消拆分失败: ', error)
  }
}

// ---- 方式 2：拆分记录查询与打开单页 ----

let splitRecordReqSeq = 0

// 查询详情归属文件的拆分记录（方式 2 按钮显隐判定面）：详情切换/拆分完成时刷新。
// 写回守卫：请求序号比对，详情快速切换时旧响应丢弃防旧文件记录误标新文件。
const refreshSplitRecord = async () => {
  if (selected.value?.kind !== 'slide' || !hasLibrary.value) {
    splitRecord.value = null
    return
  }
  const seq = ++splitRecordReqSeq
  try {
    const record = await GetAssetSplitRecord(currentLibraryId.value, detailFilePath.value)
    if (seq !== splitRecordReqSeq) return
    splitRecord.value = record || null
  } catch {
    // 显隐判定查询失败静默降级（按钮隐藏）：弹错会打断浏览主流程，重新打开
    // 详情即可重试（登记口径，对齐 AssetThumb 读取失败静默回退）
    if (seq === splitRecordReqSeq) splitRecord.value = null
  }
}

// 详情切换即刷新拆分记录（幻灯片详情拉取、其他形态复位隐藏）
watch(selected, () => {
  refreshSplitRecord()
})

// 打开拆分单页产物（系统默认关联程序，无 PowerPoint 依赖）；成功不弹提示——
// 产物文件已被系统打开，弹窗反而抢焦点
const onOpenSplitPage = async () => {
  if (!hasLibrary.value || openingSplitPage.value) return
  const relPath = detailFilePath.value
  const slideNum = detailSlideNum.value
  if (!relPath || !slideNum) return
  openingSplitPage.value = true
  try {
    await OpenAssetSplitPage(currentLibraryId.value, relPath, slideNum)
  } catch (error) {
    handleError('打开拆分单页失败: ', error)
  } finally {
    openingSplitPage.value = false
  }
}

// 拆分结果明细展示映射（对齐 model.AssetSplitStatus* 取值）
const splitOutcomeLabel = (status) => ({
  split: '已拆分',
  skipped: '已跳过',
  failed: '失败'
}[status] || status)
const splitOutcomeTagType = (status) => ({
  split: 'success',
  skipped: 'info',
  failed: 'danger'
}[status] || 'info')

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
  // 失败集合为内存态，批次结束即清（PRD 认可：失败标记只覆盖当前批次语境）；
  // 自动补在清空前取快照发起
  const failedSnapshot = failedThumbPaths.value
  failedThumbPaths.value = []
  // 失效代次自增：触发 miss 缩略图卡自动补图
  thumbEpoch.value++
  autoRetryFailedThumbs(failedSnapshot)
}

// 失败缩略图自动补一轮（SRS §2.3 PR3，PRD D4）：done 后失败集逐个 RetryAssetThumb
// 自动重新入队，恰好一轮——thumbAutoRetried 标记防下轮 done 再触发（手动重试不受
// 限），切库 watch 复位（新库新批次重新获得资格）。fire-and-forget：allSettled 收口
// 不弹提示（覆盖瞬态失败如 COM 队列占用；损坏类必然再失败但一轮即止不放大），成功
// 与否由下一批 done 事件 epoch 自增照常驱动 miss 卡补图。fileId 从文件底册按相对
// 路径反查（底册未含该路径——已删除/未加载——跳过不重试）。
const autoRetryFailedThumbs = (failedPaths) => {
  if (thumbAutoRetried || !failedPaths.length || !hasLibrary.value) return
  thumbAutoRetried = true
  const libId = currentLibraryId.value
  const retries = failedPaths
    .map((relPath) => {
      const entry = fileEntries.value.find(f => f.path === relPath)
      if (!entry) return null // 底册 miss：跳过（无 fileId 无法定位重入队）
      return RetryAssetThumb(libId, entry.id, relPath).catch(() => {}) // 单文件失败静默（后台链路）
    })
    .filter(Boolean)
  if (retries.length > 0) {
    Promise.allSettled(retries)
  }
}

const onThumbsError = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 契约：仅复位进度不弹提示（错误详情后端 slog 已记录；防与 Promise catch 双弹）
  thumbsRunning.value = false
  thumbsProgress.value = null
}

// ================= 拆分事件（assets-split:*）=================
// EventsOn 返回注销闭包，onBeforeUnmount 精准注销本组件监听器（禁 EventsOff 全局
// 移除）。监听器属面板层（确认弹窗可随时关闭、在途任务进度持续可见），故注销
// 时机 = 组件卸载，弹窗关闭无注销点（取舍见文件头注释）
let offSplitProgress = null
let offSplitDone = null
let offSplitError = null

const onSplitProgress = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  splitting.value = true
  splitProgress.value = payload
}

const onSplitDone = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 仅复位进度展示；汇总提示由拆分 Promise 分支统一弹出（done 事件先于 Promise
  // 返回到达，双路径各提示一次会重复，对齐扫描契约）；本库拆分完成后刷新方式 2
  // 拆分记录（幻灯片详情的「打开拆分单页」按钮随之出现）
  splitProgress.value = null
  refreshSplitRecord()
}

const onSplitError = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 契约：仅复位进度不弹提示（error 事件与 Promise reject 同刻到达，错误分流由
  // Promise catch 的 handleError 统一执行，防双弹——对齐 assets-scan 契约）
  splitProgress.value = null
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
  offSplitProgress = EventsOn('assets-split:progress', onSplitProgress)
  offSplitDone = EventsOn('assets-split:done', onSplitDone)
  offSplitError = EventsOn('assets-split:error', onSplitError)
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
  offSplitProgress && offSplitProgress()
  offSplitDone && offSplitDone()
  offSplitError && offSplitError()
  offProgress = null
  offDone = null
  offError = null
  offThumbsProgress = null
  offThumbsDone = null
  offThumbsError = null
  offSplitProgress = null
  offSplitDone = null
  offSplitError = null
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
  /* 窄窗口收缩策略（D8#4）：控件组整体换行而非组内收缩换行——radio 组等不可
       压缩控件以整组为单位落到下一行，宽窗口单行排布与原行为一致 */
  flex-wrap: wrap;
}

/* radio 组整组换行（D8#4）：禁收缩 + 禁组内文本换行，防「文件视图/幻灯片视图」
     label 在非最大化窗口下被压成上下堆叠 */
.assets-toolbar :deep(.el-radio-group) {
  flex-shrink: 0;
  white-space: nowrap;
}

.keyword-input {
  width: 280px;
}

.tag-filter {
  width: 220px;
}

/* facet 筛选弹层触发按钮（badge 包裹）：工具行内联对齐，badge 上标不破坏基线 */
.facet-filter {
  display: inline-flex;
  align-items: center;
}

/* ---- 保存检索条件弹层（第七批 PR3）：本库/异库分组 + 条目行（名称可点调取 +
     行内覆盖/删除）。弹层内容由本组件渲染（EP popper 传送后仍带 scoped 标记，
     对齐 AssetFilterPopover 弹层样式先例） ---- */
.saved-search-panel {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs);
}

.saved-search-hint {
  font-size: 12px;
  color: var(--text-tertiary);
  padding: var(--spacing-xs) 0;
}

.saved-search-group {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.saved-search-group-title {
  font-size: 12px;
  color: var(--text-tertiary);
}

.saved-search-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

/* 条目名称按钮：原生 button 承载点击调取（长名截断防撑破弹层），hover 主色反馈 */
.saved-search-apply {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  border: none;
  background: transparent;
  padding: 0;
  font-size: 13px;
  color: var(--text-primary);
  text-align: left;
  cursor: pointer;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  transition: color var(--transition-fast);
}

.saved-search-apply:hover {
  color: var(--primary-color);
}

.saved-search-lib-mark {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--text-tertiary);
}

.flatten-switch {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
}

/* 开关作用域禁用态（文件视图）：整体弱化 + 禁用光标，与 EP switch disabled 样式同语义 */
.slide-own-switch.is-scope-disabled {
  opacity: 0.5;
  cursor: not-allowed;
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

/* ---- 文件内过滤提示条（D6 真机增量）：弱化形态对齐缩略图降级条（info 色非警示红）---- */
.file-filter-banner {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-md);
  font-size: 12px;
  color: var(--info-color);
  background: var(--primary-bg);
  border-bottom: 1px solid var(--border-color);
}

/* ---- 卡片大小滑块（D6 真机增量）：工具条内固定窄宽（真实 el-slider 在容器内
     自占满），垂直居中对齐工具条既有控件 ---- */
.card-size-slider {
  width: 90px;
  display: flex;
  align-items: center;
}

/* ---- 主体：画廊/列表 + 详情 ---- */
.assets-body {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

/* 视图包裹层：空态与网格/列表分支的共同父级（flex column 约束链），网格与列表
     两个滚动分支在其余量空间内各自滚动；relative 为全屏预览层（AssetPreviewOverlay
     absolute inset:0）的定位语境——预览只铺满画廊列，详情侧栏在覆盖层外保持可见
     可编辑（PRD D5#6） */
.assets-view-wrap {
  position: relative;
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 空态（el-empty）在包裹层 flex 容器内居中（呈现维度无关，网格/列表共用） */
.assets-view-wrap > .el-empty {
  margin: auto;
}

.assets-gallery {
  flex: 1;
  min-height: 0;
  min-width: 0;
  overflow-y: auto;
  padding: var(--spacing-md);
  display: grid;
  /* 固定卡宽（D6 真机修复）：禁 minmax+1fr 满铺——详情栏（300px flex 兄弟）开合
       挤压画廊时 auto-fill 重排列数、每列 1fr 拉伸致卡片尺寸突变；固定轨宽下
       详情开关只减列数不改卡宽，右侧留白为固定卡宽网格常态（Eagle 式画廊）。
       变量由宿主经滑块注入，缺省 280 兜底 */
  grid-template-columns: repeat(auto-fill, var(--asset-card-w, 280px));
  gap: var(--spacing-sm);
  align-content: start;
}

.load-more-hint {
  grid-column: 1 / -1;
  text-align: center;
  font-size: 12px;
  color: var(--text-placeholder);
  padding: var(--spacing-sm) 0;
}

.asset-card {
  position: relative;
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

/* 勾选复选框（常驻左上角）：accent-color 跟随主色（亮暗主题自动适配原生勾选态
     视觉）；z-index 盖过缩略图区防遮挡 */
.asset-card-check {
  position: absolute;
  top: var(--spacing-sm);
  left: var(--spacing-sm);
  z-index: 1;
  width: 15px;
  height: 15px;
  accent-color: var(--primary-color);
  cursor: pointer;
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

/* ---- 详情标签编辑行（第六批树形化）：树形选择器占满余量 + 新建快捷按钮固定内容宽 ---- */
.detail-tags-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.detail-tags-row .detail-tags-select {
  flex: 1;
  min-width: 0;
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
  /* 换行（D6 真机修复）：300px 栏内「打开并跳到此页/打开拆分单页/查看归属文件」
       三按钮单行溢出被裁切，wrap 后折行完整可见 */
  flex-wrap: wrap;
  gap: var(--spacing-sm);
}

/* ---- 详情缩略图与失败标记 ---- */
.detail-thumb-failed {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--spacing-xs);
}

/* 全屏预览入口（弱化样式）：detail-section 为 flex column，默认拉伸全宽，左对齐 */
.detail-preview-btn {
  align-self: flex-start;
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

/* ---- 保存检索条件弹窗摘要（第七批 PR3）：内嵌弱化区（bg-tertiary 第三层）承载
     快照全维预览，标签列固定宽对齐标签值列 ---- */
.save-search-summary {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-md);
  margin: 0 var(--spacing-md) var(--spacing-sm);
}

.save-search-summary-title {
  font-size: 12px;
  color: var(--text-tertiary);
}

.save-search-summary-row {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
  font-size: 12px;
}

.save-search-summary-label {
  flex-shrink: 0;
  width: 110px;
  color: var(--text-tertiary);
}

.save-search-summary-value {
  flex: 1;
  min-width: 0;
  color: var(--text-primary);
  word-break: break-all;
}

/* ---- 拆分确认弹窗与结果明细 ---- */
.split-dialog-scope {
  font-size: 13px;
  color: var(--text-primary);
  line-height: 1.6;
  padding: 0 var(--spacing-md) var(--spacing-sm);
}

/* 源目录落点警示（warning 语义色：产物与源同目录存放的行为提示） */
.split-source-warning {
  margin: 0 var(--spacing-md);
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: 12px;
  line-height: 1.6;
  color: var(--warning-color);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.split-cache-hint {
  margin: 0 var(--spacing-md);
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.split-outcome-list {
  max-height: 320px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  padding: 0 var(--spacing-md);
}

.split-outcome-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: 12px;
  padding: var(--spacing-xs) 0;
  border-bottom: 1px dashed var(--border-color);
}

.split-outcome-row:last-child {
  border-bottom: none;
}

.split-outcome-path {
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex-shrink: 1;
}

.split-outcome-msg {
  color: var(--text-tertiary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex-shrink: 2;
  min-width: 0;
}
</style>
