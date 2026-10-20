import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetsPanel from '../AssetsPanel.vue'
import { ErrorCode } from '../../../src/utils/error'
import { ASSET_FILE_DRAG_MIME } from '../../../src/utils/assetFolder'

// ---- 测试数据（形状对齐 model/assets.go 经 Wails 绑定的 JSON 字段）----
const mockLibrary = {
  id: 'lib-1',
  name: '产品素材库',
  rootPath: 'D:/assets',
  createdAt: '2026-10-13 10:00:00',
  updatedAt: '2026-10-13 10:00:00',
  excludeDirs: [],
  excludeExts: []
}

const mockStats = { libraryId: 'lib-1', fileCount: 2, slideCount: 3, failedCount: 0, encryptedCount: 0, excludedCount: 0 }

const mockTags = [
  { id: 1, name: '重点', color: '#2563eb', parentId: 0 },
  { id: 2, name: '方案', color: '#16a34a', parentId: 0 }
]

const mockFiles = [
  {
    id: 11,
    path: '方案A/城商行方案.pptx',
    size: 1024 * 1024,
    modTime: '2026-10-12 09:00:00',
    status: 'ok',
    title: '城商行方案',
    note: '',
    rating: 0,
    tags: [mockTags[0]],
    createdAt: '2026-10-12 09:00:00',
    updatedAt: '2026-10-12 09:00:00'
  },
  {
    id: 12,
    path: '方案B/损坏文件.pptx',
    size: 2048,
    modTime: '2026-10-12 10:00:00',
    status: 'failed',
    title: '损坏文件',
    note: '',
    rating: 0,
    tags: [],
    createdAt: '2026-10-12 10:00:00',
    updatedAt: '2026-10-12 10:00:00'
  }
]

const mockSlides = [
  {
    id: 21,
    fileId: 11,
    filePath: '方案A/城商行方案.pptx',
    slideNum: 1,
    title: '封面页',
    text: '城商行零售转型方案',
    speakerNotes: '开场白',
    tags: [mockTags[0]],
    ownTags: [{ id: 9, name: '封面页标签', color: '#f59e0b', parentId: 0 }],
    note: '开场页业务备注',
    rating: 2
  },
  {
    id: 22,
    fileId: 11,
    filePath: '方案A/城商行方案.pptx',
    slideNum: 2,
    title: '背景页',
    text: '获客成本持续上升',
    speakerNotes: '',
    tags: [mockTags[0]],
    ownTags: [],
    note: '',
    rating: 0
  }
]

// 检索结果页：1 个文件命中 + 1 个聚合幻灯片条目（同 PPT 3 页命中）
const mockSearchPage = {
  items: [
    {
      refType: 'file',
      refId: 11,
      fileId: 11,
      filePath: '方案A/城商行方案.pptx',
      title: '城商行方案',
      slideNum: 0,
      snippet: '…城商行零售转型…',
      score: -3.2,
      hitCount: 1,
      tags: [mockTags[0]]
    },
    {
      refType: 'slide',
      refId: 22,
      fileId: 11,
      filePath: '方案A/城商行方案.pptx',
      title: '背景页',
      slideNum: 2,
      snippet: '…城商行获客…',
      score: -2.1,
      hitCount: 3,
      tags: [mockTags[0]],
      ownTags: [],
      note: '',
      rating: 0
    }
  ],
  total: 2,
  keyword: '城商行',
  flattened: false
}

// AppSettings 完整形状（含 assets 段，形状对齐 frontend/wailsjs/go/models.ts AppSettings）
const mockSettings = {
  gpuDisabled: false,
  defaultShell: '',
  gitBashPath: '',
  wslDistro: '',
  terminalFontSize: 14,
  terminalFontFamily: 'Geist',
  terminalScrollback: 5000,
  searchExcludeDirs: [],
  searchExcludeFiles: [],
  shortcutCommandPalette: 'ctrl+k',
  shortcutToggleTerminal: 'ctrl+`',
  shortcutRename: 'f2',
  shortcutDelete: 'delete',
  obsidianPath: '',
  themeMode: 'system',
  diffToolName: '',
  diffToolPath: '',
  diffToolArgs: '',
  diffToolMergeArgs: '',
  webServe: { enabled: false, bindAddress: '127.0.0.1:36115' },
  assets: { auto: true, smallWidth: 320, hdWidth: 1600, smallQuotaMB: 1024, hdQuotaMB: 1024 }
}

// 保存的检索条件条目工厂（形状对齐 model.AssetSavedSearch 十六字段）：本库条目
// 默认带全维条件（关键词 + 标签 + facet 大小档 + 两开关 + 排序档位），供调取回填断言
const savedSearchEntry = (overrides = {}) => ({
  id: 1,
  name: '近期高分',
  libraryId: 'lib-1',
  keyword: '获客',
  tagIds: [2],
  tagNames: ['方案'],
  ratings: [4],
  mtimeFrom: 0,
  mtimeTo: 0,
  sizeFrom: 50 * 1024 * 1024,
  sizeTo: 0,
  flatten: false,
  slideTagOnlyOwn: true,
  sortBy: 'rating',
  sortDir: 'asc',
  createdAt: '2026-10-16T09:00:00+08:00',
  ...overrides
})

// vi.mock 工厂被 hoist，不能引用外部变量；用 vi.hoisted 提升可变的 mock 函数集合
const { wailsMock } = vi.hoisted(() => ({
  wailsMock: {
    ListAssetLibraries: vi.fn(),
    CreateAssetLibrary: vi.fn(),
    DeleteAssetLibrary: vi.fn(),
    GetAssetLibraryStats: vi.fn(),
    StartAssetScan: vi.fn(),
    CancelAssetScan: vi.fn(),
    RebuildAssetLibrary: vi.fn(),
    ListAssetFiles: vi.fn(),
    ListAssetFolders: vi.fn(),
    CreateAssetFolder: vi.fn(),
    DeleteAssetFolder: vi.fn(),
    RenameAssetFolder: vi.fn(),
    // 文件批量移动（PR3 拖拽移动，R4/R6/R9）：宿主 drop 分派单点
    MoveAssetFiles: vi.fn(),
    // 目录级移动与文件夹标注（PR4）：宿主 drop-folder 分派 + 标注弹窗加载/保存
    MoveAssetFolder: vi.fn(),
    GetAssetFolderMeta: vi.fn(),
    UpdateAssetFolderMeta: vi.fn(),
    ListAssetSlides: vi.fn(),
    UpdateAssetFileMeta: vi.fn(),
    UpdateAssetSlideMeta: vi.fn(),
    SearchAssets: vi.fn(),
    // 保存检索条件（第七批 PR3）：宿主直连三绑定（保存/调取/覆盖/删除）
    ListAssetSavedSearches: vi.fn(),
    SaveAssetSavedSearch: vi.fn(),
    DeleteAssetSavedSearch: vi.fn(),
    // 标签树与标签管理（第六批）：宿主经树形选择组件间接消费 ListAssetTags
    //（本 spec 内该组件被打桩不实拉），Create/Update/Delete 为管理弹窗（同打桩）
    // 后端链，登记保证 mock 表完整性防漏 mock 报 undefined
    ListAssetTags: vi.fn(),
    CreateAssetTag: vi.fn(),
    UpdateAssetTag: vi.fn(),
    DeleteAssetTag: vi.fn(),
    // 批量打标签（第六批 PR3）：宿主 execute 分派回调按 viewMode 调其一
    BatchUpdateAssetFileTags: vi.fn(),
    BatchUpdateAssetSlideTags: vi.fn(),
    // 批量评分（第八批，SRS §2.6 P0）：单一入口混合文件/页目标，宿主 execute
    // 分派回调以 targets（slideNum||0）+ 星级值调用
    BatchSetAssetRating: vi.fn(),
    OpenDirectoryDialog: vi.fn(),
    // 文件选择对话框（重链「重新定位」选库内新位置文件，PRD D6 文件选择交互先例）
    OpenFileDialog: vi.fn(),
    // 元数据导出（第九批 PR1）：宿主 execute 分派回调合并范围字段后调用；
    // SaveFileDialog 为导出 CSV 目标浏览回调消费；CancelAssetExport 为提交中
    // 关闭弹窗（放弃导出）的中止通道
    ExportAssetMetadata: vi.fn(),
    CancelAssetExport: vi.fn(),
    SaveFileDialog: vi.fn(),
    // 文件丢失与重链（第九批 PR2，SRS §2.6.5）：宿主直连四绑定（重链/清理丢失/
    // 前缀替换预览与执行）
    RelocateAssetFile: vi.fn(),
    ClearLostAssetFiles: vi.fn(),
    PreviewAssetPathPrefixReplace: vi.fn(),
    ReplaceAssetPathPrefix: vi.fn(),
    // 索引设置与备份还原（第十批 SRS §2.7.3 / §2.7.5）：宿主直连四绑定（规则保存
    // 自动重扫 / 手动备份 / 备份清单 / 还原）
    UpdateAssetLibraryRules: vi.fn(),
    BackupAssetLibrary: vi.fn(),
    ListAssetLibraryBackups: vi.fn(),
    RestoreAssetLibrary: vi.fn(),
    AssetThumbsAvailable: vi.fn(),
    CancelAssetThumbs: vi.fn(),
    JumpToAssetSlide: vi.fn(),
    RetryAssetThumb: vi.fn(),
    GetAssetThumb: vi.fn(),
    SplitAssetFile: vi.fn(),
    SplitAssetLibrary: vi.fn(),
    CancelAssetSplit: vi.fn(),
    GetAssetSplitRecord: vi.fn(),
    OpenAssetSplitPage: vi.fn(),
    // 购物栏（PR3）：宿主直连七绑定（List/Add/Remove/Clear/Reorder/导出/取消导出）
    ListAssetCartItems: vi.fn(),
    AddAssetCartItems: vi.fn(),
    RemoveAssetCartItems: vi.fn(),
    ClearAssetCart: vi.fn(),
    ReorderAssetCart: vi.fn(),
    MergeAssetCartToPptx: vi.fn(),
    CancelAssetCartExport: vi.fn(),
    GetSettings: vi.fn(),
    SaveSettings: vi.fn()
  }
}))

vi.mock('../../../wailsjs/go/main/App', () => wailsMock)

// 捕获 EventsOn 注册的事件回调：用例内直接调回调模拟后端事件推送；
// off 闭包从 eventHandlers 删除对应项，可断言注销行为（精准注销非 EventsOff 全局移除）
const eventHandlers = {}
let offFns = []
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name, cb) => {
    eventHandlers[name] = cb
    const off = vi.fn(() => { delete eventHandlers[name] })
    offFns.push(off)
    return off
  }),
  EventsOff: vi.fn()
}))

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve(true)) }
  }
})

// ---- Element Plus 组件 stub ----
const ElSelectStub = {
  template: '<select class="el-select"><slot /></select>',
  props: ['modelValue', 'placeholder', 'size', 'multiple', 'collapseTags', 'collapseTagsTooltip', 'clearable', 'filterable', 'allowCreate', 'defaultFirstOption', 'loading', 'disabled'],
  emits: ['update:modelValue', 'change']
}
const ElRadioGroupStub = {
  template: '<div class="el-radio-group"><slot /></div>',
  props: ['modelValue', 'size', 'disabled'],
  emits: ['update:modelValue', 'change']
}
// 拆分确认弹窗落点选择：input click 即选中（emit 值替换语义），checked 态供断言默认选中
const ElRadioStub = {
  template: `<label class="el-radio"><input type="radio" :checked="modelValue === value" @change="$emit('update:modelValue', value)" /><slot /></label>`,
  props: ['modelValue', 'value', 'disabled'],
  emits: ['update:modelValue']
}
const ElSwitchStub = {
  template: '<button class="el-switch" :disabled="disabled" @click="$emit(\'update:modelValue\', !modelValue); $emit(\'change\', !modelValue)"></button>',
  props: ['modelValue', 'size', 'disabled'],
  emits: ['update:modelValue', 'change']
}
const ElRateStub = {
  template: '<div class="el-rate"><button v-for="n in 5" :key="n" class="rate-star" :disabled="disabled" @click="$emit(\'update:modelValue\', n)">{{ n }}</button></div>',
  props: ['modelValue', 'clearable', 'disabled'],
  emits: ['update:modelValue']
}
// 数字输入（缩略图设置弹窗）：valueOnClear 透传供断言防清空写 null 脏值契约
const ElInputNumberStub = {
  template: '<input class="el-input-number" :value="modelValue" @input="$emit(\'update:modelValue\', Number($event.target.value))" />',
  props: ['modelValue', 'min', 'max', 'step', 'valueOnClear', 'disabled'],
  emits: ['update:modelValue']
}
// 卡片大小滑块（D6 真机增量）：range input 形态，input 事件 emit 数字值
// （对齐真实 el-slider 的 v-model 数字语义，min/max/step 透传供边界断言）
const ElSliderStub = {
  template: '<input class="el-slider" type="range" :min="min" :max="max" :step="step" :value="modelValue" @input="$emit(\'update:modelValue\', Number($event.target.value))" />',
  props: ['modelValue', 'min', 'max', 'step', 'size'],
  emits: ['update:modelValue']
}

const stubs = {
  'el-select': ElSelectStub,
  'el-option': { template: '<option :value="value"><slot /></option>', props: ['label', 'value'] },
  'el-radio-group': ElRadioGroupStub,
  'el-radio': ElRadioStub,
  'el-radio-button': { template: '<button class="el-radio-button"><slot /></button>', props: ['value', 'size'] },
  'el-switch': ElSwitchStub,
  'el-rate': ElRateStub,
  'el-input': {
    template: `<textarea v-if="type === 'textarea'" :value="modelValue" :placeholder="placeholder" :rows="rows" :disabled="disabled" @input="$emit('update:modelValue', $event.target.value)"></textarea>
      <span v-else><input :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit('update:modelValue', $event.target.value)" @keyup="$emit('keyup', $event)" /><slot name="append" /></span>`,
    props: ['modelValue', 'placeholder', 'disabled', 'type', 'rows', 'clearable'],
    emits: ['update:modelValue', 'keyup']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
    // emits 必须声明：缺省时父级 onClick 会经 attrs 透传为根元素原生监听器，
    // 与 $emit('click') 各触发一次（handler 双重执行，调用计数类断言必挂）
    emits: ['click']
  },
  'el-dialog': {
    template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'appendToBody'],
    emits: ['update:modelValue']
  },
  'el-tag': { template: '<span class="el-tag" :data-type="type"><slot /></span>', props: ['type', 'size'] },
  // 购物栏入口徽标（PR3）：value>0 且未 hidden 时渲染计数（对齐真实 el-badge 显隐语义）
  'el-badge': {
    template: '<div class="el-badge"><slot /><sup v-if="!hidden && value > 0" class="el-badge__content">{{ value }}</sup></div>',
    props: ['value', 'hidden']
  },
  'el-progress': { template: '<div class="el-progress" :data-percentage="percentage" />', props: ['percentage', 'strokeWidth', 'showText'] },
  'el-empty': { template: '<div class="el-empty">{{ description }}<slot /></div>', props: ['description'] },
  'el-tooltip': { template: '<span><slot /></span>', props: ['content', 'placement'] },
  'el-icon': { template: '<i><slot /></i>', props: ['size'] },
  'el-alert': { template: '<div class="el-alert"><span class="el-alert__title">{{ title }}</span><slot /></div>', props: ['type', 'title', 'closable'] },
  'el-form': { template: '<form><slot /></form>', props: ['labelWidth'] },
  'el-form-item': { template: '<div><slot /></div>', props: ['label'] },
  'el-input-number': ElInputNumberStub,
  'el-slider': ElSliderStub,
  // 标签树组件桩（第六批）：受控 emit 形态对齐真实组件（update:modelValue/change
  // ID 口径 + mounted 模拟首拉完成回传 loaded 固定清单），reload 可 spy 断言刷新链
  AssetTagTreeSelect: {
    name: 'AssetTagTreeSelectStub',
    template: '<div class="asset-tag-tree-select-stub"><slot /></div>',
    props: ['modelValue', 'libraryId', 'placeholder', 'disabled', 'emptyText'],
    emits: ['update:modelValue', 'change', 'loaded'],
    mounted() {
      // 模拟首拉完成：loaded 回传固定标签清单（形状对齐 model.AssetTagListItem，
      // 覆盖文件标签与页自有标签两类），宿主共享列表据此做名称↔ID 映射
      this.$emit('loaded', [
        { id: 1, name: '重点', color: '#2563eb', parentId: 0, count: 2 },
        { id: 2, name: '方案', color: '#16a34a', parentId: 0, count: 1 },
        { id: 9, name: '获客页标签', color: '#f59e0b', parentId: 0, count: 1 }
      ])
    },
    methods: {
      reload() {}
    }
  },
  // 文件夹树桩（PRD PR1）：树数据/徽标计数/选中态受控透出 + select 事件透出
  //（宿主过滤链与树数据接线断言入口；树渲染细节见 FolderTree 独立 spec）
  FolderTree: {
    name: 'FolderTreeStub',
    template: '<div class="folder-tree-stub"></div>',
    props: ['nodes', 'countMap', 'selectedPath', 'loading'],
    emits: ['select']
  },
  // 标签管理弹窗桩：显隐受控 + changed 事件透出（宿主刷新链断言入口）
  AssetTagManagerDialog: {
    name: 'AssetTagManagerDialogStub',
    template: '<div v-if="modelValue" class="asset-tag-manager-dialog-stub"></div>',
    props: ['modelValue', 'libraryId', 'presetCreate'],
    emits: ['update:modelValue', 'changed']
  },
  // 批量打标签弹窗桩（第六批 PR3）：显隐/targets/execute 受控透出 + completed
  // 事件透出（宿主刷新链与选中保留断言入口；targets/execute 形状断言见批量用例）
  AssetBatchTagDialog: {
    name: 'AssetBatchTagDialogStub',
    template: '<div v-if="modelValue" class="asset-batch-tag-dialog-stub"></div>',
    props: ['modelValue', 'libraryId', 'targets', 'execute'],
    emits: ['update:modelValue', 'completed']
  },
  // 批量评分弹窗桩（第八批）：显隐/targets/execute 受控透出 + completed 事件透出
  //（复刻批量打标签弹窗桩形态，targets/execute 形状断言见批量评分用例）
  AssetBatchRatingDialog: {
    name: 'AssetBatchRatingDialogStub',
    template: '<div v-if="modelValue" class="asset-batch-rating-dialog-stub"></div>',
    props: ['modelValue', 'targets', 'execute'],
    emits: ['update:modelValue', 'completed']
  },
  // 元数据导出弹窗桩（第九批 PR1）：显隐/预览数/execute 受控透出（范围解析与
  // 检索快照为宿主职责，弹窗交互与 execute 形状断言见导出弹窗独立 spec；
  // 宿主接线（工具行打开弹窗 / 范围字段下发 / 预览统计链路）由 E2E
  // frontend/e2e/assets-export.spec.js 覆盖，本 spec 不重复断言）
  AssetExportDialog: {
    name: 'AssetExportDialogStub',
    template: '<div v-if="modelValue" class="asset-export-dialog-stub"></div>',
    props: ['modelValue', 'previewCount', 'previewScopeLabel', 'execute', 'pickCsvPath', 'pickMdDir', 'cancel'],
    emits: ['update:modelValue', 'exported']
  },
  // 路径前缀替换弹窗桩（第九批 PR2）：显隐/preview/execute 受控透出 + completed
  // 事件透出（宿主接线与刷新链断言入口；弹窗交互细节见独立 spec）
  AssetPrefixReplaceDialog: {
    name: 'AssetPrefixReplaceDialogStub',
    template: '<div v-if="modelValue" class="asset-prefix-replace-dialog-stub"></div>',
    props: ['modelValue', 'preview', 'execute'],
    emits: ['update:modelValue', 'completed']
  },
  // 索引设置弹窗桩（第十批 PR3）：显隐/library/execute 受控透出 + saved 事件透出
  // （宿主接线与保存后刷新链断言入口；规则编辑交互细节见 AssetRulesDialog 独立 spec）
  AssetRulesDialog: {
    name: 'AssetRulesDialogStub',
    template: '<div v-if="modelValue" class="asset-rules-dialog-stub"></div>',
    props: ['modelValue', 'library', 'execute'],
    emits: ['update:modelValue', 'saved']
  },
  // 备份还原弹窗桩（第十批 PR3）：显隐/libraryId/libraryName/listBackups/
  // executeRestore 受控透出 + restored 事件透出（宿主接线与还原后刷新链断言入口；
  // 清单与确认链细节见 AssetBackupDialog 独立 spec）
  AssetBackupDialog: {
    name: 'AssetBackupDialogStub',
    template: '<div v-if="modelValue" class="asset-backup-dialog-stub"></div>',
    props: ['modelValue', 'libraryId', 'libraryName', 'listBackups', 'executeRestore'],
    emits: ['update:modelValue', 'restored']
  },
  // facet 筛选弹层桩（第七批）：受控 emit 形态对齐真实组件（update:modelValue +
  // change 双事件同步快照）。档位→检索参数换算为组件模块纯函数（独立 spec 以注入
  // now 覆盖），本 spec 只断言宿主接线（SearchAssets 参数 / facets-only 链路切换 /
  // 空态引导 / 语境清空点位）
  AssetFilterPopover: {
    name: 'AssetFilterPopoverStub',
    template: '<div class="asset-filter-popover-stub"></div>',
    props: ['modelValue', 'disabled'],
    emits: ['update:modelValue', 'change']
  },
  // 购物栏抽屉桩（PR3）：显隐/条目/导出运行态受控透出 + 七事件透出（宿主接线断言
  // 入口；抽屉渲染与拖拽交互细节见 AssetCartDrawer 独立 spec）
  AssetCartDrawer: {
    name: 'AssetCartDrawerStub',
    template: '<div v-if="visible" class="asset-cart-drawer-stub" :data-items="items.length"></div>',
    props: ['visible', 'items', 'exporting', 'exportProgress'],
    emits: ['close', 'remove', 'clear', 'reorder', 'export', 'cancel-export', 'trace']
  },
  // 保存检索条件弹层桩（第七批 PR3）：双 slot 全渲染（免触发机制直测内容）；
  // mounted 即 emit show（真实弹层首次打开触发宿主 List 重拉，行数据因此加载）；
  // hide 方法供调取后收起弹层的可选调用（真实 EP popover 实例方法）
  'el-popover': {
    template: '<div class="el-popover-stub"><slot name="reference" /><div class="el-popover-content"><slot /></div></div>',
    props: ['placement', 'width', 'trigger', 'popperClass'],
    emits: ['show'],
    mounted() {
      this.$emit('show')
    },
    methods: { hide() {} }
  },
  // 排序档位下拉桩（SRS §2.5.3）：经 provide 向后代 el-dropdown-item 下发命令
  // 派发函数，使下拉项可 DOM 点击触发 @command（对齐真实 EP 组件的 item 点击
  // 行为，AiChatPanel spec 同款先例）
  'el-dropdown': {
    template: '<div class="el-dropdown"><slot /><slot name="dropdown" /></div>',
    emits: ['command'],
    provide() {
      return { __dropdownCommand: (cmd) => this.$emit('command', cmd) }
    }
  },
  'el-dropdown-menu': { template: '<div class="el-dropdown-menu"><slot /></div>' },
  'el-dropdown-item': {
    template: '<div class="el-dropdown-item" :class="{ \'is-disabled\': disabled }" @click="onClick"><slot /></div>',
    props: ['command', 'disabled', 'divided'],
    inject: { __dropdownCommand: { default: null } },
    methods: {
      onClick() {
        if (this.disabled) return
        if (this.__dropdownCommand) this.__dropdownCommand(this.command)
      }
    }
  }
}

const createWrapper = async () => {
  const wrapper = mount(AssetsPanel, { global: { stubs } })
  await flushPromises()
  return wrapper
}

const clickButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().trim() === text)?.trigger('click')

// facet 快照变更（真实弹层控件交互在组件独立 spec 覆盖）：update:modelValue 与
// change 双事件同步（对齐真实组件 emitState 形态），change 触发宿主语境变更链
const emitFacetChange = async (wrapper, state) => {
  const stub = wrapper.findComponent({ name: 'AssetFilterPopoverStub' })
  stub.vm.$emit('update:modelValue', state)
  await stub.vm.$emit('change', state)
  await flushPromises()
}

describe('AssetsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 重置默认实现（个别用例的 mockReturnValue/mockRejectedValue 会永久覆盖）
    wailsMock.ListAssetLibraries.mockResolvedValue([mockLibrary])
    wailsMock.GetAssetLibraryStats.mockResolvedValue(mockStats)
    wailsMock.ListAssetFiles.mockResolvedValue(mockFiles)
    // 文件夹树（PRD PR1）默认空树（节点形状 model.AssetFolderNode），按用例覆盖
    wailsMock.ListAssetFolders.mockResolvedValue([])
    // 文件夹管理（PRD PR2）默认安全形态：新建返回空节点、删除 0 条 lost、重命名零同步
    wailsMock.CreateAssetFolder.mockResolvedValue({ path: '', name: '', children: [], excluded: false, derived: false, fileCount: 0 })
    wailsMock.DeleteAssetFolder.mockResolvedValue(0)
    wailsMock.RenameAssetFolder.mockResolvedValue({ oldPath: '', newPath: '', replaced: 0, excluded: 0, revived: 0 })
    // 文件批量移动（PR3）默认空汇总（形状对齐 model.AssetBatchMoveResult）
    wailsMock.MoveAssetFiles.mockResolvedValue({ total: 0, succeeded: 0, failed: 0, items: [] })
    // 目录级移动与文件夹标注（PR4）默认安全形态
    wailsMock.MoveAssetFolder.mockResolvedValue({ oldPath: '', newPath: '', replaced: 0, excluded: 0, revived: 0 })
    wailsMock.GetAssetFolderMeta.mockResolvedValue({ folderPath: '', note: '', tags: [], updatedAt: '' })
    wailsMock.UpdateAssetFolderMeta.mockResolvedValue(null)
    wailsMock.ListAssetSlides.mockResolvedValue({ items: mockSlides, total: mockSlides.length })
    wailsMock.SearchAssets.mockResolvedValue(mockSearchPage)
    wailsMock.StartAssetScan.mockResolvedValue({ total: 2, new: 1, updated: 1, unchanged: 0, failed: 0, encrypted: 0, denied: 0, removed: 0 })
    wailsMock.RebuildAssetLibrary.mockResolvedValue({ total: 2, new: 2, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, removed: 0 })
    wailsMock.CancelAssetScan.mockResolvedValue(true)
    wailsMock.CreateAssetLibrary.mockResolvedValue({ ...mockLibrary, id: 'lib-2', name: '新库' })
    wailsMock.DeleteAssetLibrary.mockResolvedValue(null)
    wailsMock.UpdateAssetFileMeta.mockResolvedValue(null)
    wailsMock.UpdateAssetSlideMeta.mockResolvedValue(null)
    // ---- 标签树/管理（第六批）默认值：树组件已打桩不实拉，登记安全 no-op 供
    // 管理弹窗桩语境外的直接断言兜底 ----
    wailsMock.ListAssetTags.mockResolvedValue([])
    wailsMock.CreateAssetTag.mockResolvedValue({ id: 99, name: '新标签', color: '#7c3aed', parentId: 0 })
    wailsMock.UpdateAssetTag.mockResolvedValue(null)
    wailsMock.DeleteAssetTag.mockResolvedValue(0)
    // 批量打标签（第六批 PR3）默认全成功空明细（形状对齐 model.AssetBatchTagResult）
    wailsMock.BatchUpdateAssetFileTags.mockResolvedValue({ total: 0, succeeded: 0, failed: 0, items: [] })
    wailsMock.BatchUpdateAssetSlideTags.mockResolvedValue({ total: 0, succeeded: 0, failed: 0, items: [] })
    // 批量评分（第八批）默认全成功空明细（形状对齐 model.AssetBatchRatingResult）
    wailsMock.BatchSetAssetRating.mockResolvedValue({ total: 0, succeeded: 0, failed: 0, items: [] })
    // 保存检索条件（第七批 PR3）默认值：空清单 + upsert 返回带 ID 对象 + 删除 no-op
    wailsMock.ListAssetSavedSearches.mockResolvedValue([])
    wailsMock.SaveAssetSavedSearch.mockResolvedValue(savedSearchEntry())
    wailsMock.DeleteAssetSavedSearch.mockResolvedValue(null)
    // 目录对话框默认空串 = 用户取消（安全 no-op）
    wailsMock.OpenDirectoryDialog.mockResolvedValue('')
    // 文件选择对话框默认空串 = 用户取消（重链选文件按用例覆盖）
    wailsMock.OpenFileDialog.mockResolvedValue('')
    // ---- 文件丢失与重链（第九批 PR2）默认值：重链默认哈希匹配直复活形态 +
    // 清理零条 + 预览零命中 + 替换空结果（形状对齐 model.AssetRelocateResult /
    // AssetPrefixReplaceResult），按用例覆盖；文件对话框默认空串 = 用户取消 ----
    wailsMock.SaveFileDialog.mockResolvedValue('')
    wailsMock.RelocateAssetFile.mockResolvedValue({ fileId: 11, oldPath: '方案A/城商行方案.pptx', newPath: '新目录/城商行方案.pptx', hashMatched: true, skippedHashCheck: false, reparsed: false })
    wailsMock.ClearLostAssetFiles.mockResolvedValue(0)
    wailsMock.PreviewAssetPathPrefixReplace.mockResolvedValue(0)
    wailsMock.ReplaceAssetPathPrefix.mockResolvedValue({ libraryId: 'lib-1', oldPrefix: '', newPrefix: '', replaced: 0, revived: 0, reparsed: 0, stillLost: 0, scan: { total: 0, new: 0, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, lost: 0, revived: 0, removed: 0 } })
    // ---- 索引设置与备份还原（第十批）默认值：规则保存零排除汇总 + 备份回执 +
    // 空清单 + 还原 no-op（形状对齐 model.AssetScanResult / AssetBackupEntry），
    // 按用例覆盖 ----
    wailsMock.UpdateAssetLibraryRules.mockResolvedValue({ total: 0, new: 0, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, excluded: 0, lost: 0, revived: 0, removed: 0 })
    wailsMock.BackupAssetLibrary.mockResolvedValue({ libraryId: 'lib-1', fileName: '20261018-100000.db', path: 'D:/data/assets/backups/lib-1/20261018-100000.db', sizeBytes: 1024, createdAt: '2026-10-18 10:00:00' })
    wailsMock.ListAssetLibraryBackups.mockResolvedValue([])
    wailsMock.RestoreAssetLibrary.mockResolvedValue(null)
    // ---- 缩略图（PR3）默认值：Available=false 走降级提示条 + 占位卡稳定路径，按用例覆盖 ----
    wailsMock.AssetThumbsAvailable.mockResolvedValue(false)
    wailsMock.GetAssetThumb.mockResolvedValue('')
    wailsMock.CancelAssetThumbs.mockResolvedValue(false)
    wailsMock.JumpToAssetSlide.mockResolvedValue(null)
    wailsMock.RetryAssetThumb.mockResolvedValue(null)
    // ---- 拆分（SRS §2.2.3/§2.2.5 方式 2）默认值：空结果汇总 + 无拆分记录（方式 2
    // 按钮隐藏），按用例覆盖 ----
    wailsMock.SplitAssetFile.mockResolvedValue({ outputDir: '', files: [], pageCount: 0, skipped: false, skipReason: '' })
    wailsMock.SplitAssetLibrary.mockResolvedValue({ total: 0, split: 0, skipped: 0, failed: 0, items: [] })
    wailsMock.CancelAssetSplit.mockResolvedValue(false)
    wailsMock.GetAssetSplitRecord.mockResolvedValue({ exists: false, outputDir: '', pageCount: 0, splitAt: '', upToDate: false })
    wailsMock.OpenAssetSplitPage.mockResolvedValue(null)
    wailsMock.GetSettings.mockResolvedValue(mockSettings)
    wailsMock.SaveSettings.mockResolvedValue(null)
    // ---- 购物栏（PR3）默认值：空购物栏 + 新增 1 条回执 + 导出成功 2 页结果 +
    // 变更/取消 no-op（形状对齐 model.AssetCartItem / AssetCartAddResult /
    // PptxMergeResult），按用例覆盖 ----
    wailsMock.ListAssetCartItems.mockResolvedValue([])
    wailsMock.AddAssetCartItems.mockResolvedValue({ added: 1, duplicates: 0 })
    wailsMock.RemoveAssetCartItems.mockResolvedValue(null)
    wailsMock.ClearAssetCart.mockResolvedValue(null)
    wailsMock.ReorderAssetCart.mockResolvedValue(null)
    wailsMock.MergeAssetCartToPptx.mockResolvedValue({ outputPath: 'D:/out/合并导出.pptx', pages: [], pageCount: 2, skippedItems: [], sldSzMixed: false })
    wailsMock.CancelAssetCartExport.mockResolvedValue(false)
    // AssetThumb 懒加载确定性：移除 IO（若未来 jsdom 提供）使画廊缩略图走挂载即加载
    // 兜底路径——卡片在测试中不会进入可视区，走 IO 路径则缩略图用例不可判定
    delete globalThis.IntersectionObserver
    for (const key of Object.keys(eventHandlers)) delete eventHandlers[key]
    offFns = []
  })

  it('挂载注册 assets-scan / assets-thumbs / assets-split / assets-cart 共十二个事件监听，卸载时闭包精准注销且未走 EventsOff', async () => {
    const { EventsOff } = await import('../../../wailsjs/runtime/runtime')
    const wrapper = await createWrapper()
    expect(Object.keys(eventHandlers)).toEqual(
      expect.arrayContaining([
        'assets-scan:progress',
        'assets-scan:done',
        'assets-scan:error',
        'assets-thumbs:progress',
        'assets-thumbs:done',
        'assets-thumbs:error',
        'assets-split:progress',
        'assets-split:done',
        'assets-split:error',
        'assets-cart:export-progress',
        'assets-cart:export-done',
        'assets-cart:export-error'
      ])
    )
    expect(offFns.length).toBe(12)

    wrapper.unmount()
    expect(offFns.filter((off) => off.mock.calls.length > 0).length).toBe(12)
    expect(Object.keys(eventHandlers)).toHaveLength(0)
    expect(EventsOff).not.toHaveBeenCalled()
  })

  it('无库空态：渲染引导建库文案与新建按钮', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([])
    const wrapper = await createWrapper()
    expect(wrapper.find('.el-empty').text()).toContain('还没有素材库')
    expect(wrapper.text()).toContain('新建素材库')
  })

  it('有库：默认文件视图渲染 ListAssetFiles 卡片（文件类型标记、标题、路径、状态标签）', async () => {
    const wrapper = await createWrapper()
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledWith('lib-1')
    expect(wailsMock.GetAssetLibraryStats).toHaveBeenCalledWith('lib-1')

    const cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(2)
    expect(cards[0].text()).toContain('文件')
    expect(cards[0].text()).toContain('城商行方案')
    expect(cards[0].text()).toContain('方案A/城商行方案.pptx')
    // 解析失败文件展示 danger 状态标签
    expect(cards[1].text()).toContain('解析失败')
    // 库统计行回显
    expect(wrapper.find('.assets-stats-line').text()).toContain('文件 2')
    expect(wrapper.find('.assets-stats-line').text()).toContain('幻灯片 3')
  })

  // ---- 文件夹树（PRD「素材管理：文件夹树与拖拽整理」PR1）----

  it('文件夹树加载：挂载即以当前库调 ListAssetFolders，树数据与徽标计数映射传入 FolderTree', async () => {
    wailsMock.ListAssetFolders.mockResolvedValue([
      { path: '方案A', name: '方案A', children: [], excluded: false, fileCount: 1 }
    ])
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    expect(wailsMock.ListAssetFolders).toHaveBeenCalledWith('lib-1')
    expect(tree.props('nodes')).toHaveLength(1)
    expect(tree.props('nodes')[0].path).toBe('方案A')
    // 徽标计数映射随底册聚合（utils/assetFolder 单点）：根含全部、目录递归计数
    expect(tree.props('countMap')['']).toBe(2)
    expect(tree.props('countMap')['方案A']).toBe(1)
  })

  it('文件夹树选中过滤：文件视图按选中目录前缀递归过滤主区卡片（a vs ab 边界）', async () => {
    // 前缀边界夹具：方案A 与 方案AB 为兄弟目录，「方案A」选中不得命中 方案AB 下文件
    wailsMock.ListAssetFiles.mockResolvedValue([
      ...mockFiles,
      {
        id: 13, path: '方案AB/兄弟目录文件.pptx', size: 1, modTime: '', status: 'ok',
        title: '兄弟目录文件', note: '', rating: 0, tags: [], createdAt: '', updatedAt: ''
      }
    ])
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })

    await tree.vm.$emit('select', '方案A')
    await flushPromises()
    let cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(1)
    expect(cards[0].text()).toContain('城商行方案')

    // 递归含子目录（D3）：选中带子目录的前缀命中任意深度后代
    wailsMock.ListAssetFiles.mockResolvedValue([
      mockFiles[0],
      {
        id: 14, path: '方案A/子目录/深层文件.pptx', size: 1, modTime: '', status: 'ok',
        title: '深层文件', note: '', rating: 0, tags: [], createdAt: '', updatedAt: ''
      }
    ])
    const wrapper2 = await createWrapper()
    const tree2 = wrapper2.findComponent({ name: 'FolderTreeStub' })
    await tree2.vm.$emit('select', '方案A')
    await flushPromises()
    cards = wrapper2.findAll('.asset-card')
    expect(cards.length).toBe(2) // 方案A 自身文件 + 子目录内文件

    // 回根「全部素材」（folderPath=''）恢复全量
    await tree2.vm.$emit('select', '')
    await flushPromises()
    expect(wrapper2.findAll('.asset-card').length).toBe(2)
  })

  it('检索语境树过滤叠加：选中目录后检索，folderPath 随 opts 下发（R7 AND 叠加）且树选中保留', async () => {
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('select', '方案A')
    await flushPromises()
    expect(tree.props('selectedPath')).toBe('方案A')

    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    // 检索联动（PR2）：opts.folderPath 随检索下发，与关键词 AND 叠加（mock 不做
    // 真实过滤，卡片数断言仅验证渲染链路）；树选中保留不清空（§8.7 矩阵）
    expect(wrapper.findAll('.asset-card').length).toBe(1) // 文件视图下仅文件命中
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行',
      expect.objectContaining({ folderPath: '方案A' }))
    expect(wrapper.findComponent({ name: 'FolderTreeStub' }).props('selectedPath')).toBe('方案A')
  })

  // ---- 文件夹树管理操作（PRD PR2 R3：新增/重命名/删除，分派与刷新在宿主）----

  it('新建子目录：分派 CreateAssetFolder（父路径 + 名称）并局部刷新树', async () => {
    const wrapper = await createWrapper()
    const callsBefore = wailsMock.ListAssetFolders.mock.calls.length
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('create', '方案A', '新子目录')
    await flushPromises()

    expect(wailsMock.CreateAssetFolder).toHaveBeenCalledWith('lib-1', '方案A', '新子目录')
    expect(wailsMock.ListAssetFolders.mock.calls.length).toBe(callsBefore + 1)
  })

  it('重命名目录：分派 RenameAssetFolder 并按返回前缀改写选中目录（浏览语境保留）', async () => {
    wailsMock.RenameAssetFolder.mockResolvedValue({ oldPath: '方案A', newPath: '方案A2', replaced: 2, excluded: 0, revived: 0 })
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('select', '方案A')
    await flushPromises()

    await tree.vm.$emit('rename', '方案A', '方案A2')
    await flushPromises()

    expect(wailsMock.RenameAssetFolder).toHaveBeenCalledWith('lib-1', '方案A', '方案A2')
    // 选中目录位于重命名前缀下：按新前缀改写（folderPath = newPath + 子路径）
    expect(wrapper.findComponent({ name: 'FolderTreeStub' }).props('selectedPath')).toBe('方案A2')
  })

  it('删除目录：非空（递归 PPT 数 > 0）先二次确认后以 force=true 分派，选中位于删除前缀下复位根', async () => {
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('select', '方案A')
    await flushPromises()

    await tree.vm.$emit('delete', { path: '方案A', name: '方案A' })
    await flushPromises()

    // mockFiles 中方案A 下有 1 个 PPT（countMap 递归计数 > 0）→ 前端预确认 + force=true
    expect(wailsMock.DeleteAssetFolder).toHaveBeenCalledWith('lib-1', '方案A', true)
    expect(wailsMock.ListAssetFolders).toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'FolderTreeStub' }).props('selectedPath')).toBe('')
  })

  it('删除目录：后端实测非空返回 E_ASSETS_FOLDER_NOT_EMPTY 时二次确认后以 force=true 重调', async () => {
    wailsMock.DeleteAssetFolder.mockRejectedValueOnce({
      code: ErrorCode.AssetsFolderNotEmpty,
      message: '目录「空目录」不是空目录（含 3 个条目），确定要连同内容一起删除吗？'
    })
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    // 空目录（countMap 计数 0）跳过前端预确认，直接以 force=false 首调
    await tree.vm.$emit('delete', { path: '空目录', name: '空目录' })
    await flushPromises()

    expect(wailsMock.DeleteAssetFolder).toHaveBeenNthCalledWith(1, 'lib-1', '空目录', false)
    // 命中确认型拒绝：二次确认后 force=true 重调
    expect(wailsMock.DeleteAssetFolder).toHaveBeenNthCalledWith(2, 'lib-1', '空目录', true)
  })

  it('目录选中矩阵复位面：清详情/预览/选中/文件过滤/聚合展开缓存（§8.7 登记点位）', async () => {
    const wrapper = await createWrapper()
    // 先装配详情 + 选中态
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    const check = wrapper.find('.asset-card-check')
    await check.setValue(true)
    await flushPromises()
    expect(wrapper.findComponent({ name: 'AssetSelectionBar' }).props('count')).toBe(1)

    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('select', '方案A')
    await flushPromises()

    // 数据过滤语境变更：详情/预览/选中/文件过滤清空（对齐 onFacetChange 同类点位）
    expect(wrapper.find('.assets-detail').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'AssetSelectionBar' }).props('count')).toBe(0)
  })

  // ---- 文件拖拽移动（PRD PR3 R4/R6/R9：HTML5 DnD 编排，分派在宿主单点）----

  const makeDataTransfer = () => ({ setData: vi.fn(), effectAllowed: null })

  it('Ctrl+点击轻量多选：勾选切换进选中集（普通点击详情语义不变）', async () => {
    const wrapper = await createWrapper()
    const cards = wrapper.findAll('.asset-card')
    await cards[0].trigger('click', { ctrlKey: true })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'AssetSelectionBar' }).props('count')).toBe(1)
    // Ctrl+点击进选中集后不进详情
    expect(wrapper.find('.assets-detail').exists()).toBe(false)
    // 普通点击维持详情语义
    await cards[1].trigger('click', {})
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
  })

  it('dragstart 载荷：拖拽卡在选中集内带整个选中集文件 ID，否则仅该卡', async () => {
    const wrapper = await createWrapper()
    const cards = wrapper.findAll('.asset-card')
    // 选中两张（Ctrl+点击）
    await cards[0].trigger('click', { ctrlKey: true })
    await cards[1].trigger('click', { ctrlKey: true })
    await flushPromises()
    const dt1 = makeDataTransfer()
    await cards[0].trigger('dragstart', { dataTransfer: dt1 })
    expect(dt1.setData).toHaveBeenCalledWith(
      ASSET_FILE_DRAG_MIME,
      JSON.stringify({ fileIds: [11, 12] })
    )
    // 未选中卡拖拽：仅该卡
    const dt2 = makeDataTransfer()
    const wrapper2 = await createWrapper()
    await wrapper2.findAll('.asset-card')[0].trigger('dragstart', { dataTransfer: dt2 })
    expect(dt2.setData).toHaveBeenCalledWith(
      ASSET_FILE_DRAG_MIME,
      JSON.stringify({ fileIds: [11] })
    )
  })

  it('drop 分派：MoveAssetFiles（库 + fileIDs + 目标）+ 底册/树刷新 + 选中集清空 + 成功 toast', async () => {
    wailsMock.MoveAssetFiles.mockResolvedValue({ total: 1, succeeded: 1, failed: 0, items: [{ fileId: 11, status: 'ok', label: '方案A/城商行方案.pptx' }] })
    const wrapper = await createWrapper()
    const listCallsBefore = wailsMock.ListAssetFiles.mock.calls.length
    const folderCallsBefore = wailsMock.ListAssetFolders.mock.calls.length
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('select', '方案A')
    await flushPromises()
    await tree.vm.$emit('drop-files', '归档', [11])
    await flushPromises()

    expect(wailsMock.MoveAssetFiles).toHaveBeenCalledWith('lib-1', [11], '归档')
    // 数据变更点位：底册与树徽标刷新，folderPath 保留（§8.7 登记）
    expect(wailsMock.ListAssetFiles.mock.calls.length).toBe(listCallsBefore + 1)
    expect(wailsMock.ListAssetFolders.mock.calls.length).toBe(folderCallsBefore + 1)
    expect(tree.props('selectedPath')).toBe('方案A')
    // 拖拽多选收口：选中集清空
    expect(wrapper.findComponent({ name: 'AssetSelectionBar' }).props('count')).toBe(0)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalled()
  })

  it('drop 汇总分流：部分失败 warning 带首个失败原因（对齐批量打标签汇总口径）', async () => {
    wailsMock.MoveAssetFiles.mockResolvedValue({
      total: 2, succeeded: 1, failed: 1,
      items: [
        { fileId: 11, status: 'ok', label: 'a' },
        { fileId: 12, status: 'failed', message: '目标目录已存在同名文件「b.pptx」' }
      ]
    })
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('drop-files', '归档', [11, 12])
    await flushPromises()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith(
      expect.stringContaining('目标目录已存在同名文件「b.pptx」')
    )
  })

  it('drop 在途防重：移动 Promise 期间再次 drop 被拒绝（busy 标记）', async () => {
    let resolveMove
    wailsMock.MoveAssetFiles.mockImplementation(() => new Promise((resolve) => { resolveMove = resolve }))
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('drop-files', '归档', [11])
    await tree.vm.$emit('drop-files', '归档', [12])
    await flushPromises()
    expect(wailsMock.MoveAssetFiles).toHaveBeenCalledTimes(1)
    resolveMove({ total: 1, succeeded: 1, failed: 0, items: [] })
    await flushPromises()
    expect(wailsMock.MoveAssetFiles).toHaveBeenCalledTimes(1)
  })

  // ---- 目录级拖拽移动与文件夹标注（PRD R5/R8，PR4）----

  it('drop-folder 分派：MoveAssetFolder + 选中目录按新前缀改写 + 树刷新', async () => {
    wailsMock.MoveAssetFolder.mockResolvedValue({ oldPath: '方案A', newPath: '归档/方案A', replaced: 1, excluded: 0, revived: 0 })
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('select', '方案A')
    await flushPromises()
    const folderCallsBefore = wailsMock.ListAssetFolders.mock.calls.length

    await tree.vm.$emit('drop-folder', '方案A', '归档')
    await flushPromises()

    expect(wailsMock.MoveAssetFolder).toHaveBeenCalledWith('lib-1', '方案A', '归档')
    // 选中目录位于源前缀下：按新前缀改写（浏览语境保留）
    expect(wrapper.findComponent({ name: 'FolderTreeStub' }).props('selectedPath')).toBe('归档/方案A')
    expect(wailsMock.ListAssetFolders.mock.calls.length).toBe(folderCallsBefore + 1)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalled()
  })

  it('文件夹标注：菜单入口加载预填（标签 ID 口径映射），保存按名称数组整体替换', async () => {
    wailsMock.GetAssetFolderMeta.mockResolvedValue({
      folderPath: '方案A', note: '原有备注',
      tags: [{ id: 1, name: '重点', color: '#2563eb', parentId: 0 }],
      updatedAt: '2026-10-20 10:00:00'
    })
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('annotate', { path: '方案A', name: '方案A' })
    await flushPromises()

    expect(wailsMock.GetAssetFolderMeta).toHaveBeenCalledWith('lib-1', '方案A')
    // 弹窗预填：备注 + 标签 ID（经共享标签列表映射回显）
    const dialog = wrapper.find('.stub-dialog')
    expect(dialog.exists()).toBe(true)

    await wrapper.find('textarea').setValue('更新后的备注')
    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.UpdateAssetFolderMeta).toHaveBeenCalledWith('lib-1', '方案A', {
      note: '更新后的备注',
      tagNames: ['重点'] // ID 1 → 名称映射（AssetTagTreeSelect 桩 loaded 回传的共享标签表）
    })
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalled()
  })

  it('切库复位：树选中与树数据属库语境，库切换落定后回「全部素材」并重拉树', async () => {
    // lib-2 须在库列表内（切库后 hasLibrary 判定依赖库列表命中）
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '第二库' }
    ])
    const wrapper = await createWrapper()
    const tree = wrapper.findComponent({ name: 'FolderTreeStub' })
    await tree.vm.$emit('select', '方案A')
    await flushPromises()

    wailsMock.ListAssetFolders.mockClear()
    const select = wrapper.findComponent(ElSelectStub)
    // 切库经 el-select stub change 事件（onLibraryChange → resetAndReloadView）
    await select.vm.$emit('update:modelValue', 'lib-2')
    await select.vm.$emit('change', 'lib-2')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'FolderTreeStub' }).props('selectedPath')).toBe('')
    expect(wailsMock.ListAssetFolders).toHaveBeenCalledWith('lib-2')
  })

  it('视图切换到幻灯片视图：按分页参数调用 ListAssetSlides 并渲染页码卡片', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    expect(wailsMock.ListAssetSlides).toHaveBeenCalledWith('lib-1', 50, 0)
    const cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(2)
    expect(cards[0].text()).toContain('幻灯片')
    expect(cards[0].text()).toContain('第 1 页')
  })

  it('幻灯片视图分页懒加载：滚动到底部以 offset 追加下一页', async () => {
    wailsMock.ListAssetSlides.mockImplementation((libId, limit, offset) => {
      if (offset === 0) return Promise.resolve({ items: mockSlides, total: 4 })
      return Promise.resolve({ items: [{ ...mockSlides[0], id: 23, slideNum: 3 }], total: 4 })
    })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    expect(wrapper.findAll('.asset-card').length).toBe(2)

    // 滚动到底部触发下一页（offset = 已加载条目数）
    const gallery = wrapper.find('.assets-gallery')
    Object.defineProperty(gallery.element, 'scrollHeight', { value: 500, configurable: true })
    Object.defineProperty(gallery.element, 'clientHeight', { value: 400, configurable: true })
    gallery.element.scrollTop = 150
    await gallery.trigger('scroll')
    await flushPromises()

    expect(wailsMock.ListAssetSlides).toHaveBeenLastCalledWith('lib-1', 50, 2)
    expect(wrapper.findAll('.asset-card').length).toBe(3)
  })

  it('视图切换兜底：目标视图清单为空即重新拉取（文件空底册切走再切回）', async () => {
    wailsMock.ListAssetFiles.mockResolvedValue([]) // 文件底册为空（如空库）
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    // 切到幻灯片视图（幻灯片清单加载）
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    // 切回文件视图：底册为空应触发兜底拉取（审核 9 同根因）
    await group.vm.$emit('update:modelValue', 'file')
    await group.vm.$emit('change', 'file')
    await flushPromises()
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledTimes(2)
  })

  it('搜索（文件视图）：以当前库与关键词调用 SearchAssets（分页参数对齐），仅渲染文件命中并带类型标记与命中摘要', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    expect(wailsMock.SearchAssets).toHaveBeenCalledWith('lib-1', '城商行', {
      flatten: true,
      tagIds: [],
      slideTagOnlyOwn: false,
      ratings: [],
      mtimeFrom: 0,
      mtimeTo: 0,
      sizeFrom: 0,
      sizeTo: 0,
      sortBy: 'relevance',
      sortDir: 'desc',
      folderPath: '',
      limit: 50,
      offset: 0
    })
    // 文件视图下仅显示文件类型命中（幻灯片命中经视图切换查看）
    const cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(1)
    expect(cards[0].text()).toContain('文件')
    expect(cards[0].text()).toContain('…城商行零售转型…')
    // 统计行展示命中总数（未按视图过滤的总口径）
    expect(wrapper.find('.assets-stats-line').text()).toContain('命中 2 条')
  })

  it('搜索（幻灯片视图）：渲染幻灯片命中与「N 页命中」聚合徽标', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    const cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(1)
    expect(cards[0].text()).toContain('幻灯片')
    expect(cards[0].text()).toContain('第 2 页')
    expect(cards[0].text()).toContain('3 页命中')
    expect(cards[0].text()).toContain('…城商行获客…')
  })

  it('检索无结果：渲染空态提示', async () => {
    wailsMock.SearchAssets.mockResolvedValue({ items: [], total: 0, keyword: '不存在', flattened: false })
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('不存在')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    expect(wrapper.find('.el-empty').text()).toContain('没有匹配的素材')
  })

  it('聚合折叠：幻灯片视图下点击聚合卡以 flatten 检索展开逐页命中，再点收起', async () => {
    const wrapper = await createWrapper()
    // 聚合条目为幻灯片命中，切到幻灯片视图后可见
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    // 展开前检索调用计数（视图切换走 ListAssetSlides 不调 SearchAssets）
    const callsBefore = wailsMock.SearchAssets.mock.calls.length
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()

    // 展开触发 flatten 检索（limit 200 全量取回后客户端按文件路径过滤）
    expect(wailsMock.SearchAssets).toHaveBeenNthCalledWith(callsBefore + 1, 'lib-1', '城商行', {
      flatten: true,
      tagIds: [],
      slideTagOnlyOwn: false,
      limit: 200,
      offset: 0
    })
    const children = wrapper.findAll('.agg-child')
    expect(children.length).toBe(1)
    expect(children[0].text()).toContain('第 2 页')

    // 再点收起：不再发检索
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.agg-child').exists()).toBe(false)
    expect(wailsMock.SearchAssets.mock.calls.length).toBe(callsBefore + 1)
  })

  // ---- 排序档位（SRS §2.5.3，D1）：工具行下拉切换 / 同档再点翻转升降 / 切档回
  // 默认降序 / 相关性无向 / 非检索语境仅记录 / 走 applySearch 同路径 ----

  it('排序切换：mtime 档以 sortBy=mtime/sortDir=desc 重跑检索，同档再点翻转升序，切档回默认降序，相关性回默认', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    const sortItem = (text) =>
      wrapper.findAll('.sort-menu .el-dropdown-item').find((i) => i.text().includes(text))
    await sortItem('修改时间').trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行',
      expect.objectContaining({ sortBy: 'mtime', sortDir: 'desc' }))
    // 触发按钮标注当前档位与方向（人读反馈）
    expect(wrapper.find('.sort-trigger').text()).toContain('修改时间')

    // 同档再点翻转升降（D1）
    await sortItem('修改时间').trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行',
      expect.objectContaining({ sortBy: 'mtime', sortDir: 'asc' }))

    // 切档回默认降序
    await sortItem('评分').trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行',
      expect.objectContaining({ sortBy: 'rating', sortDir: 'desc' }))

    // 切回相关性档（默认无向）
    await sortItem('相关性').trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行',
      expect.objectContaining({ sortBy: 'relevance', sortDir: 'desc' }))
  })

  it('排序变更走 applySearch 同路径：检索语境下语境复位面统一执行（§8.2 矩阵，不重复清理）', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    // 选中一张命中卡（排序变更应被 applySearch 清掉——驻留即幽灵选中）
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)

    const sortItem = (text) =>
      wrapper.findAll('.sort-menu .el-dropdown-item').find((i) => i.text().includes(text))
    await sortItem('评分').trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(false)
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行',
      expect.objectContaining({ sortBy: 'rating', sortDir: 'desc' }))
  })

  it('排序变更（非检索语境）：仅记录待下次检索生效不触发清单重载；相关性同档再点无翻转（无向）', async () => {
    const wrapper = await createWrapper()
    const callsBefore = wailsMock.SearchAssets.mock.calls.length
    const sortItem = (text) =>
      wrapper.findAll('.sort-menu .el-dropdown-item').find((i) => i.text().includes(text))
    await sortItem('评分').trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets.mock.calls.length).toBe(callsBefore) // 浏览态不重载
    // 切到检索语境后按已记录档位发起
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行',
      expect.objectContaining({ sortBy: 'rating', sortDir: 'desc' }))

    // 切回相关性档（切档语义，正常重跑）；相关性同档再点：无向不翻转、不重跑
    await sortItem('相关性').trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行',
      expect.objectContaining({ sortBy: 'relevance', sortDir: 'desc' }))
    const callsAfterSwitch = wailsMock.SearchAssets.mock.calls.length
    await sortItem('相关性').trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets.mock.calls.length).toBe(callsAfterSwitch)
  })

  it('保存弹窗摘要含排序档位行：切档后摘要与快照口径一致（人读形态）', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    const sortItem = (text) =>
      wrapper.findAll('.sort-menu .el-dropdown-item').find((i) => i.text().includes(text))
    await sortItem('修改时间').trigger('click')
    await flushPromises()
    await sortItem('修改时间').trigger('click') // 翻转为升序
    await flushPromises()
    await clickButton(wrapper, '保存搜索')
    const summary = wrapper.find('.save-search-summary')
    expect(summary.text()).toContain('排序')
    expect(summary.text()).toContain('修改时间·升序')
  })

  it('调取旧快照（无排序字段）：容错回退默认相关性/降序（D3 向后兼容无版本号）', async () => {
    wailsMock.ListAssetSavedSearches.mockResolvedValue([
      savedSearchEntry({ id: 11, name: '旧条件', sortBy: undefined, sortDir: undefined })
    ])
    const wrapper = await createWrapper()
    wailsMock.SearchAssets.mockClear()
    await clickButton(wrapper, '旧条件')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(1)
    const [, , opts] = wailsMock.SearchAssets.mock.calls[0]
    expect(opts.sortBy).toBe('relevance')
    expect(opts.sortDir).toBe('desc')
  })

  it('检索命中文件点击：底册未加载时兜底拉取 ListAssetFiles 并显示详情（非「记录不存在」告警）', async () => {
    // 挂载时底册加载为空（第 1 次），检索命中文件卡片点击触发兜底重拉（第 2 次）
    wailsMock.ListAssetFiles.mockResolvedValueOnce([]).mockResolvedValue(mockFiles)
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()

    expect(wailsMock.ListAssetFiles).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    expect(wrapper.find('.assets-detail').text()).toContain('城商行方案')
    expect(wrapper.find('.assets-detail').text()).not.toContain('文件索引记录不存在')
  })

  it('平铺开关：默认开启，检索态下回关后以 flatten=false 重跑检索（D8#2 默认值翻转适配）', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    // 首次检索即以默认平铺口径发起
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行', {
      flatten: true,
      tagIds: [],
      slideTagOnlyOwn: false,
      ratings: [],
      mtimeFrom: 0,
      mtimeTo: 0,
      sizeFrom: 0,
      sizeTo: 0,
      sortBy: 'relevance',
      sortDir: 'desc',
      folderPath: '',
      limit: 50,
      offset: 0
    })

    // 平铺开关选择器排除同类的幻灯片自身标签开关（后者恒渲染占位，DOM 序在前）；
    // 回关后立即以 flatten=false 重跑检索
    const switchStub = wrapper.find('.flatten-switch:not(.slide-own-switch)').findComponent(ElSwitchStub)
    await switchStub.vm.$emit('update:modelValue', false)
    await switchStub.vm.$emit('change', false)
    await flushPromises()

    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行', {
      flatten: false,
      tagIds: [],
      slideTagOnlyOwn: false,
      ratings: [],
      mtimeFrom: 0,
      mtimeTo: 0,
      sizeFrom: 0,
      sizeTo: 0,
      sortBy: 'relevance',
      sortDir: 'desc',
      folderPath: '',
      limit: 50,
      offset: 0
    })
  })

  it('详情编辑回写：星级/备注/标签经 UpdateAssetFileMeta 保存并刷新底册', async () => {
    const wrapper = await createWrapper()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    expect(wrapper.find('.assets-detail').text()).toContain('方案A/城商行方案.pptx')

    // 打 3 星
    await wrapper.findAll('.rate-star')[2].trigger('click')
    // 填备注
    await wrapper.find('textarea').setValue('季度汇报用')
    // 选标签（树形选择器统一 ID 口径：emit [1,2] 经宿主映射写回草稿名 ['重点','方案']，
    // 保存契约 UpdateAssetFileMeta 入参标签名数组不变）
    const tagsSelect = wrapper.findComponent('.detail-tags-select')
    await tagsSelect.vm.$emit('update:modelValue', [1, 2])
    await flushPromises()

    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.UpdateAssetFileMeta).toHaveBeenCalledWith('lib-1', 11, {
      rating: 3,
      note: '季度汇报用',
      tags: ['重点', '方案']
    })
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已保存')
    // 保存后重拉文件底册（取回新建标签的完整对象）
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledTimes(2)
  })

  it('标签管理入口：工具条「管理标签」打开管理弹窗，详情「+ 新建标签」以预填新增表单态打开', async () => {
    const wrapper = await createWrapper()
    // 工具条入口：默认非预填态
    await clickButton(wrapper, '管理标签')
    await flushPromises()
    const manager = wrapper.findComponent({ name: 'AssetTagManagerDialogStub' })
    expect(manager.props('modelValue')).toBe(true)
    expect(manager.props('presetCreate')).toBe(false)

    // 详情快捷入口：预填新增根标签表单态（PRD D6）
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    await clickButton(wrapper, '+ 新建标签')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'AssetTagManagerDialogStub' }).props('presetCreate')).toBe(true)
  })

  it('标签管理变更刷新链：changed 事件触发树组件 reload + 底册重拉 + 检索态重跑，选中保留（数据变更点位不清选中，PRD D5 同性质）', async () => {
    const wrapper = await createWrapper()
    // 预置：进入检索态（改词搜索）
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledTimes(1)
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(1)
    // 预置：勾选检索结果中的文件卡（选中集合非空）
    await wrapper.find('.asset-card-check').setValue(true)

    const tagSelect = wrapper.findComponent({ name: 'AssetTagTreeSelectStub' })
    const reloadSpy = vi.spyOn(tagSelect.vm, 'reload')
    await wrapper.findComponent({ name: 'AssetTagManagerDialogStub' }).vm.$emit('changed')
    await flushPromises()

    expect(reloadSpy).toHaveBeenCalledTimes(1)
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledTimes(2)
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(2)
    // 数据变更点位不清选中：勾选保留（浮动条计数不变）
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)
  })

  it('幻灯片标注保存（浏览模式）：星级/业务备注/自有标签经 UpdateAssetSlideMeta 整体替换保存并重拉幻灯片清单，无扫描/缩略图副作用', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    // 打开无标注基线的第 2 页详情：草稿取条目零值标注初始化
    await wrapper.findAll('.asset-card')[1].trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(0)
    expect(wrapper.find('textarea').element.value).toBe('')

    // 编辑：打 3 星 + 填业务备注 + 选自有标签（树形选择器 ID 口径：emit [1,2] 映射回
    // 草稿名 ['重点','方案']，保存契约 tagNames 数组不变）
    await wrapper.findAll('.rate-star')[2].trigger('click')
    await wrapper.find('textarea').setValue('适配分行场景')
    const slideTagsSelect = wrapper.findComponent('.slide-tags-select')
    await slideTagsSelect.vm.$emit('update:modelValue', [1, 2])
    await flushPromises()

    await clickButton(wrapper, '保存')
    await flushPromises()

    // 入参形状对齐 model.AssetSlideMetaUpdate（note/rating/tagNames），定位键 fileId+slideNum
    expect(wailsMock.UpdateAssetSlideMeta).toHaveBeenCalledWith('lib-1', 11, 2, {
      rating: 3,
      note: '适配分行场景',
      tagNames: ['重点', '方案']
    })
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已保存')
    // 浏览语境刷新路径：重拉幻灯片清单（对齐文件保存后重拉底册），检索不重跑
    expect(wailsMock.ListAssetSlides).toHaveBeenCalledTimes(2)
    expect(wailsMock.SearchAssets).not.toHaveBeenCalled()
    // 纯标注层写入：不触发重解析与缩略图失效（断言无副作用）
    expect(wailsMock.StartAssetScan).not.toHaveBeenCalled()
    expect(wailsMock.RetryAssetThumb).not.toHaveBeenCalled()
  })

  it('检索命中幻灯片详情：草稿取命中条目自有标注初始化（自有标签不在文件聚合仍正常显示），保存后重跑检索', async () => {
    // 单页命中（hitCount=1 可直接点进详情），自带独立标注三字段
    wailsMock.SearchAssets.mockResolvedValue({
      items: [{
        refType: 'slide',
        refId: 22,
        fileId: 11,
        filePath: '方案A/城商行方案.pptx',
        title: '背景页',
        slideNum: 2,
        snippet: '…获客…',
        score: -1,
        hitCount: 1,
        tags: [mockTags[0]],
        ownTags: [{ id: 9, name: '获客页标签', color: '#f59e0b', parentId: 0 }],
        note: '命中页备注',
        rating: 4
      }],
      total: 1,
      keyword: '获客',
      flattened: false
    })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.find('input').setValue('获客')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()

    const detail = wrapper.find('.assets-detail')
    // 草稿初始化（检索命中条目路径）：星级/业务备注取命中条目标注
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(4)
    expect(detail.find('textarea').element.value).toBe('命中页备注')
    // 页自有标签经共享标签列表（ListAssetTags 全量口径，含页自有标签）正常回显：
    // 树形选择器收到草稿名映射出的 ID（获客页标签 → id 9）
    expect(wrapper.findComponent('.slide-tags-select').props('modelValue')).toEqual([9])

    // 编辑星级后保存：检索语境重跑检索刷新命中数据（不重拉清单）
    await wrapper.findAll('.rate-star')[4].trigger('click')
    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.UpdateAssetSlideMeta).toHaveBeenCalledWith('lib-1', 11, 2, {
      rating: 5,
      note: '命中页备注',
      tagNames: ['获客页标签']
    })
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(2)
    expect(wailsMock.ListAssetSlides).toHaveBeenCalledTimes(1)
  })

  it('保存后刷新 miss（浏览语境，条目不在复位首页）：草稿保留刚保存值且 selected 引用按提交值回填（审查 P1：防旧快照回退二次写回）', async () => {
    // 首次加载返回 2 条；保存后刷新复位首页仅返回第 1 条（模拟深页条目 miss）
    wailsMock.ListAssetSlides
      .mockResolvedValueOnce({ items: mockSlides, total: 4 })
      .mockResolvedValue({ items: [mockSlides[0]], total: 4 })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    // 打开第 2 条（id=22，无标注基线）详情并编辑保存
    await wrapper.findAll('.asset-card')[1].trigger('click')
    await flushPromises()
    await wrapper.findAll('.rate-star')[2].trigger('click')
    await wrapper.find('textarea').setValue('适配分行场景')
    const slideTagsSelect = wrapper.findComponent('.slide-tags-select')
    await slideTagsSelect.vm.$emit('update:modelValue', [1, 2])
    await flushPromises()
    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.UpdateAssetSlideMeta).toHaveBeenCalledTimes(1)
    expect(wailsMock.ListAssetSlides).toHaveBeenCalledTimes(2)

    // miss 分支：草稿不回读旧快照（保持刚保存值，无回退）
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(3)
    expect(wrapper.find('textarea').element.value).toBe('适配分行场景')

    // selected 引用已按提交值就地回填：点「重置」（草稿从 selected 引用重读）仍是刚保存值
    //（草稿名 ['重点','方案'] 经映射回显为 ID [1,2]）
    await clickButton(wrapper, '重置')
    await flushPromises()
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(3)
    expect(wrapper.find('textarea').element.value).toBe('适配分行场景')
    expect(wrapper.findComponent('.slide-tags-select').props('modelValue')).toEqual([1, 2])
  })

  it('标注保存（过滤浏览语境）：清单复位首页后自动续拉到位，画廊不全空且保存条目回读服务端数据', async () => {
    // 初始首页即目标文件页（进入过滤无入口续拉）；保存刷新复位首页模拟为其他文件
    // 段——无兜底时客户端 fileFilterPath 过滤收敛 0 张（真机冒烟 bug 同形态：
    // 过滤视图点卡片进详情保存，成功提示后画廊全空）
    const slideOfOther = { ...mockSlides[0], id: 31, fileId: 12, filePath: '方案B/损坏文件.pptx', slideNum: 1, title: '损坏页一' }
    // 续拉页返回保存后服务端权威数据（第 1 页星级 3），供命中分支回读断言
    const savedFirstPage = [{ ...mockSlides[0], rating: 3 }, mockSlides[1]]
    wailsMock.ListAssetSlides
      .mockResolvedValueOnce({ items: mockSlides, total: 3 })
      .mockImplementation((libId, limit, offset) => {
        if (offset === 0) return Promise.resolve({ items: [slideOfOther], total: 3 })
        return Promise.resolve({ items: savedFirstPage, total: 3 })
      })
    const wrapper = await createWrapper()

    // 双击文件卡进入过滤浏览，点第 1 页卡进详情编辑星级保存
    await wrapper.findAll('.asset-card')[0].trigger('dblclick')
    await flushPromises()
    expect(wrapper.find('.file-filter-banner').exists()).toBe(true)
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    await wrapper.findAll('.rate-star')[2].trigger('click')
    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.UpdateAssetSlideMeta).toHaveBeenCalledWith('lib-1', 11, 1, expect.objectContaining({ rating: 3 }))
    // 复位首页（offset 0，其他文件段）+ 兜底续拉（offset 1）——无兜底时清单停在
    // 其他文件段，客户端过滤收敛 0 张画廊全空
    expect(wailsMock.ListAssetSlides).toHaveBeenLastCalledWith('lib-1', 50, 1)
    const cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(2)
    expect(cards.every((c) => c.text().includes('城商行方案.pptx'))).toBe(true)
    expect(wrapper.find('.file-filter-banner').text()).toContain('2 页')
    // 保存条目回到已加载段走命中分支：草稿以服务端权威数据回读（星级 3）
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(3)
  })

  it('保存后刷新 miss（检索语境，命中不在重跑结果页）：草稿保留刚保存值且 hit 引用按提交值回填（审查 P1）', async () => {
    const hitItem = {
      refType: 'slide',
      refId: 22,
      fileId: 11,
      filePath: '方案A/城商行方案.pptx',
      title: '背景页',
      slideNum: 2,
      snippet: '…获客…',
      score: -1,
      hitCount: 1,
      tags: [mockTags[0]],
      ownTags: [],
      note: '',
      rating: 0
    }
    // 首次检索返回命中；保存后重跑检索返回空页（模拟保存条目不在返回页）
    wailsMock.SearchAssets
      .mockResolvedValueOnce({ items: [hitItem], total: 1, keyword: '获客', flattened: false })
      .mockResolvedValue({ items: [], total: 0, keyword: '获客', flattened: false })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.find('input').setValue('获客')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(0)

    // 编辑星级（5 星）后保存：重跑检索 miss
    await wrapper.findAll('.rate-star')[4].trigger('click')
    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.UpdateAssetSlideMeta).toHaveBeenCalledWith('lib-1', 11, 2, {
      rating: 5,
      note: '',
      tagNames: []
    })
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(2)

    // miss 分支：星级保持 5 不回退；点「重置」（草稿从 selected.hit 重读）仍是刚保存值
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(5)
    await clickButton(wrapper, '重置')
    await flushPromises()
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(5)
  })

  it('保存刷新期间检索语境变更（切仅自身标签开关）：守卫判定退出，不写回 selected 也不重置草稿（审查 risk 1）', async () => {
    const initialPage = {
      items: [{
        refType: 'slide',
        refId: 22,
        fileId: 11,
        filePath: '方案A/城商行方案.pptx',
        title: '背景页',
        slideNum: 2,
        snippet: '…获客…',
        score: -1,
        hitCount: 1,
        tags: [mockTags[0]],
        ownTags: [{ id: 9, name: '获客页标签', color: '#f59e0b', parentId: 0 }],
        note: '命中页备注',
        rating: 4
      }],
      total: 1,
      keyword: '获客',
      flattened: false
    }
    // 开关重跑后的新语境页：同 refId 但标注值不同——若无守卫，保存链写回会把
    // 该新语境 hit 同步进 selected 并 reset 草稿（表单回退为新语境值）
    const switchedPage = {
      items: [{ ...initialPage.items[0], rating: 1, note: '语境切换后数据' }],
      total: 1,
      keyword: '获客',
      flattened: false
    }
    // 请求按发起序分流：第 1 次 = 初始检索（快落定）；第 2 次 = 保存后的重跑检索
    // （挂起，制造在途窗口）；第 3 次 = 在途窗口内切开关触发的重跑（快落定）。
    // 保存刷新慢响应最后放行——此时 searchSeq 已变，runSearch 内部守卫丢弃响应，
    // onSaveSlideMeta 的守卫快照比对应整体退出
    let searchCalls = 0
    let resolveSaveRefresh
    wailsMock.SearchAssets.mockImplementation((libId, keyword, opts) => {
      searchCalls++
      if (searchCalls === 2) {
        return new Promise((resolve) => { resolveSaveRefresh = () => resolve(switchedPage) })
      }
      return Promise.resolve(searchCalls === 1 ? initialPage : switchedPage)
    })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.find('input').setValue('获客')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(4)

    // 编辑星级（5）后保存：保存刷新检索挂起在途
    await wrapper.findAll('.rate-star')[4].trigger('click')
    await clickButton(wrapper, '保存')
    await flushPromises()
    expect(searchCalls).toBe(2)

    // 在途窗口内切「仅幻灯片自身标签」开关（searchSeq++ 且不关详情，自行重跑检索）
    const ownSwitch = wrapper.find('.slide-own-switch').findComponent(ElSwitchStub)
    await ownSwitch.vm.$emit('update:modelValue', true)
    await ownSwitch.vm.$emit('change', true)
    await flushPromises()
    expect(searchCalls).toBe(3)

    // 保存刷新慢响应后到：守卫判定退出，草稿不被 reset 也不被新语境数据写回
    resolveSaveRefresh()
    await flushPromises()
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(5)
    expect(wrapper.find('.assets-detail textarea').element.value).toBe('命中页备注')
    // 保存调用本身已发生（成功语义不变），无二次写回
    expect(wailsMock.UpdateAssetSlideMeta).toHaveBeenCalledTimes(1)
    expect(wailsMock.UpdateAssetSlideMeta).toHaveBeenCalledWith('lib-1', 11, 2, {
      rating: 5,
      note: '命中页备注',
      tagNames: ['获客页标签']
    })
  })

  it('保存成功后清空聚合展开态：重展开重新按当前口径拉取（审查 risk 2：旧 aggChildren 缓存持有保存前标注）', async () => {
    // 聚合命中（hitCount=3）；flatten 聚合检索返回逐页子项（保存前标注基线 rating 0）
    const childHit = {
      refType: 'slide',
      refId: 22,
      fileId: 11,
      filePath: '方案A/城商行方案.pptx',
      title: '背景页',
      slideNum: 2,
      snippet: '…城商行获客…',
      score: -1,
      hitCount: 1,
      tags: [mockTags[0]],
      ownTags: [],
      note: '',
      rating: 0
    }
    const aggPage = { items: [mockSearchPage.items[1]], total: 1, keyword: '城商行', flattened: false }
    wailsMock.SearchAssets.mockImplementation((libId, keyword, opts) => {
      if (opts.flatten) return Promise.resolve({ items: [childHit], total: 1, keyword, flattened: true })
      return Promise.resolve(aggPage)
    })
    const wrapper = await createWrapper()
    // D8#2 平铺默认开：聚合展开链路用例先回关平铺，使检索返回聚合卡（命中态
    // 点击展开才走 flatten:true 子项检索）；检索前回关不触发 runSearch（非检索态）
    const flattenSwitchOff = wrapper.find('.flatten-switch:not(.slide-own-switch)').findComponent(ElSwitchStub)
    await flattenSwitchOff.vm.$emit('update:modelValue', false)
    await flattenSwitchOff.vm.$emit('change', false)
    await flushPromises()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    // 展开聚合卡（flatten 检索写入 aggChildren），点子项进详情，编辑星级保存
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.agg-child').length).toBe(1)
    await wrapper.find('.agg-child').trigger('click')
    await flushPromises()
    await wrapper.findAll('.rate-star')[3].trigger('click')
    const callsBeforeSave = wailsMock.SearchAssets.mock.calls.length
    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.UpdateAssetSlideMeta).toHaveBeenCalledTimes(1)
    // 保存成功（命中分支）：重跑检索已发生
    expect(wailsMock.SearchAssets.mock.calls.length).toBe(callsBeforeSave + 1)

    // 聚合展开态已随保存清空：子项区收起（旧缓存不清时此处置位仍展开）
    expect(wrapper.find('.agg-child').exists()).toBe(false)

    // 重展开重新发起 flatten 检索（不受旧 aggChildren 早退），带当前检索口径
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行', {
      flatten: true,
      tagIds: [],
      slideTagOnlyOwn: false,
      limit: 200,
      offset: 0
    })
    expect(wrapper.findAll('.agg-child').length).toBe(1)
  })

  it('继承筛选开关：恒渲染占位（文件视图禁用不跳动），切幻灯片视图启用，切换后 SearchAssets 收到 slideTagOnlyOwn=true 且立即重跑检索', async () => {
    const wrapper = await createWrapper()
    // 文件视图（检索结果只含文件条目，开关无可见效果）下渲染占位但禁用（防视图切换同行控件跳动）
    const fileViewSwitch = wrapper.find('.slide-own-switch').findComponent(ElSwitchStub)
    expect(fileViewSwitch.exists()).toBe(true)
    expect(fileViewSwitch.props('disabled')).toBe(true)
    expect(wrapper.find('.slide-own-switch').classes()).toContain('is-scope-disabled')

    // 切幻灯片视图：开关启用且默认关闭（占位不重建，无布局跳动）
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    expect(wrapper.find('.slide-own-switch').exists()).toBe(true)
    expect(wrapper.find('.slide-own-switch').text()).toContain('仅幻灯片自身标签')
    const ownSwitch = wrapper.find('.slide-own-switch').findComponent(ElSwitchStub)
    expect(ownSwitch.props('modelValue')).toBe(false)
    expect(ownSwitch.props('disabled')).toBe(false)
    expect(wrapper.find('.slide-own-switch').classes()).not.toContain('is-scope-disabled')

    // 纯标签筛选触发检索（选中标签即进入检索模式）
    const tagSelect = wrapper.findComponent('.tag-filter')
    await tagSelect.vm.$emit('update:modelValue', [1])
    await tagSelect.vm.$emit('change', [1])
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(1)

    // 切开关：立即以 slideTagOnlyOwn=true 重跑检索（纯标签浏览路径同样生效）
    await ownSwitch.vm.$emit('update:modelValue', true)
    await ownSwitch.vm.$emit('change', true)
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '', {
      flatten: true,
      tagIds: [1],
      slideTagOnlyOwn: true,
      ratings: [],
      mtimeFrom: 0,
      mtimeTo: 0,
      sizeFrom: 0,
      sizeTo: 0,
      sortBy: 'relevance',
      sortDir: 'desc',
      folderPath: '',
      limit: 50,
      offset: 0
    })
  })

  it('幻灯片详情：文件标签（继承）只读弱化带「继承」标记，三层独立标注编辑区与双备注命名并存，溯源跳转保留', async () => {
    const wrapper = await createWrapper()
    // 切幻灯片视图
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()

    const detail = wrapper.find('.assets-detail')
    expect(detail.text()).toContain('第 1 页')
    expect(detail.text()).toContain('方案A/城商行方案.pptx')
    // 继承区：只读弱化（标题改名 + 无输入控件），标签带「继承」标记
    expect(detail.text()).toContain('文件标签（继承）')
    expect(detail.find('.detail-inherit-tags .inherit-mark').exists()).toBe(true)
    expect(detail.find('.detail-inherit-tags input, .detail-inherit-tags textarea, .detail-inherit-tags select').exists()).toBe(false)
    // 编辑区：星级/幻灯片标签/业务备注三控件齐备（对齐文件详情编辑形态）
    expect(detail.find('.el-rate').exists()).toBe(true)
    expect(detail.find('.slide-tags-select').exists()).toBe(true)
    // 双备注命名并存：业务备注（用户标注 textarea）与演讲者备注（解析自带只读）严格区分
    expect(detail.text()).toContain('业务备注')
    expect(detail.text()).toContain('演讲者备注')
    expect(detail.find('textarea').exists()).toBe(true)

    // 草稿初始化（画廊清单条目路径）：星级/业务备注/自有标签取条目自带标注
    expect(wrapper.findComponent(ElRateStub).props('modelValue')).toBe(2)
    expect(detail.find('textarea').element.value).toBe('开场页业务备注')

    // 溯源：切回文件视图并选中归属文件（id=11）
    await clickButton(wrapper, '查看归属文件')
    await flushPromises()

    const selectedCard = wrapper.find('.asset-card.is-selected')
    expect(selectedCard.exists()).toBe(true)
    expect(selectedCard.text()).toContain('城商行方案')
    // 详情切换为文件编辑形态
    expect(wrapper.find('.el-rate').exists()).toBe(true)
  })

  it('扫描：进度事件按库过滤更新进度条，done 事件仅复位进度（数据刷新由 Promise 分支统一执行防双拉取），可取消', async () => {
    let resolveScan
    wailsMock.StartAssetScan.mockImplementation(() => new Promise((resolve) => { resolveScan = resolve }))
    const wrapper = await createWrapper()
    const filesCallsBefore = wailsMock.ListAssetFiles.mock.calls.length

    await clickButton(wrapper, '扫描')
    await flushPromises()
    expect(wrapper.find('.scan-progress').exists()).toBe(true)
    expect(wrapper.find('.scan-progress').text()).toContain('正在准备扫描')

    // 本库进度事件更新进度条
    eventHandlers['assets-scan:progress']({ libraryId: 'lib-1', done: 2, total: 5, currentFile: '方案A/城商行方案.pptx' })
    await flushPromises()
    expect(wrapper.find('.scan-progress').text()).toContain('扫描中 2/5')

    // 他库事件不串入
    eventHandlers['assets-scan:progress']({ libraryId: 'lib-other', done: 9, total: 9, currentFile: '' })
    await flushPromises()
    expect(wrapper.find('.scan-progress').text()).toContain('扫描中 2/5')

    // done 事件到达：仅复位进度展示，不触发数据刷新（审核 12：双刷新二选一，
    // 防大库全量清单拉取翻倍）
    eventHandlers['assets-scan:done']({ libraryId: 'lib-1', result: { total: 5, new: 2, updated: 1, unchanged: 2, failed: 0, encrypted: 0, denied: 0, removed: 0 } })
    await flushPromises()
    expect(wailsMock.ListAssetFiles.mock.calls.length).toBe(filesCallsBefore)
    expect(wrapper.find('.scan-progress').text()).toContain('正在准备扫描')

    // 扫描 Promise 仍在途（scanning=true）：取消入口可用
    await clickButton(wrapper, '取消扫描')
    expect(wailsMock.CancelAssetScan).toHaveBeenCalledWith('lib-1')

    // Promise 落定：统一执行数据刷新（唯一刷新路径）
    resolveScan({ total: 5, new: 2, updated: 1, unchanged: 2, failed: 0, encrypted: 0, denied: 0, removed: 0 })
    await flushPromises()
    expect(wailsMock.ListAssetFiles.mock.calls.length).toBe(filesCallsBefore + 1)
    expect(wailsMock.GetAssetLibraryStats.mock.calls.length).toBe(2)
    expect(wrapper.find('.scan-progress').exists()).toBe(false)
  })

  it('扫描完成：Promise 返回结果汇总提示（新增/更新口径）', async () => {
    const wrapper = await createWrapper()
    await clickButton(wrapper, '扫描')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('扫描完成'))
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('新增 1'))
    expect(wrapper.find('.scan-progress').exists()).toBe(false)
  })

  it('扫描取消错误经 handleError 分流为 warning 中性提示', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.StartAssetScan.mockRejectedValue({ code: 'E_ASSETS_CANCELLED', message: '扫描已取消' })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '扫描')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('扫描已取消')
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(wrapper.find('.scan-progress').exists()).toBe(false)
  })

  it('扫描真失败经 handleError 分流为 error（带前缀）', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.StartAssetScan.mockRejectedValue({ code: 'E_ASSETS_LIB_NOT_FOUND', message: '素材库不存在或已被删除，请刷新素材库列表' })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '扫描')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith('扫描失败: 素材库不存在或已被删除，请刷新素材库列表')
    expect(ElMessage.warning).not.toHaveBeenCalled()
  })

  it('新建库：弹窗填写名称与目录后创建并自动切换到新库', async () => {
    const wrapper = await createWrapper()
    await clickButton(wrapper, '新建库')
    await flushPromises()
    expect(wrapper.find('.stub-dialog').exists()).toBe(true)

    const dialogInput = wrapper.find('.stub-dialog input')
    await dialogInput.setValue('新库')
    // 弹窗内第二个输入框为素材目录
    await wrapper.findAll('.stub-dialog input')[1].setValue('D:/new-assets')
    await clickButton(wrapper, '创建')
    await flushPromises()

    expect(wailsMock.CreateAssetLibrary).toHaveBeenCalledWith('新库', 'D:/new-assets')
    expect(wailsMock.GetAssetLibraryStats).toHaveBeenCalledWith('lib-2')
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledWith('lib-2')
  })

  it('新建库弹窗「选择目录」：调 OpenDirectoryDialog 并将返回路径回填素材目录输入框', async () => {
    wailsMock.OpenDirectoryDialog.mockResolvedValue('D:/picked-assets')
    const wrapper = await createWrapper()
    await clickButton(wrapper, '新建库')
    await flushPromises()

    await clickButton(wrapper, '选择目录')
    await flushPromises()

    expect(wailsMock.OpenDirectoryDialog).toHaveBeenCalledWith('选择素材根目录')
    const inputs = wrapper.findAll('.stub-dialog input')
    expect(inputs[1].element.value).toBe('D:/picked-assets')
  })

  it('新建库弹窗「选择目录」取消（返回空串）：不覆盖手动已输入的路径', async () => {
    const wrapper = await createWrapper()
    await clickButton(wrapper, '新建库')
    await flushPromises()

    const inputs = wrapper.findAll('.stub-dialog input')
    await inputs[1].setValue('D:/manual-path')
    await clickButton(wrapper, '选择目录')
    await flushPromises()

    expect(wailsMock.OpenDirectoryDialog).toHaveBeenCalledWith('选择素材根目录')
    expect(inputs[1].element.value).toBe('D:/manual-path')
  })

  it('新建库弹窗「选择目录」失败：经 handleError 分流为 error 提示', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.OpenDirectoryDialog.mockRejectedValue({ message: '无头服务模式下原生对话框不可用' })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '新建库')
    await flushPromises()

    await clickButton(wrapper, '选择目录')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith('选择目录失败: 无头服务模式下原生对话框不可用')
  })

  it('删除库：确认后调用 DeleteAssetLibrary 并回退到剩余首库', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '备份库' }
    ])
    const wrapper = await createWrapper()
    await clickButton(wrapper, '删除库')
    await flushPromises()

    expect(wailsMock.DeleteAssetLibrary).toHaveBeenCalledWith('lib-1', false)
    // 删除后自动切换到剩余库并加载其数据
    expect(wailsMock.GetAssetLibraryStats).toHaveBeenCalledWith('lib-2')
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledWith('lib-2')
  })

  // ---- 删库备份失败升级确认（第十批 SRS §2.7.5，PRD R3 零恢复点删除残余风险缓解）----

  it('删除库备份失败：升级强确认（删除后无法恢复文案）后带 true 重调', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '备份库' }
    ])
    wailsMock.DeleteAssetLibrary
      .mockRejectedValueOnce({ code: 'E_ASSETS_DELETE_BACKUP_FAILED', message: '删除前自动备份失败' })
      .mockResolvedValueOnce(null)
    const wrapper = await createWrapper()
    await clickButton(wrapper, '删除库')
    await flushPromises()

    // 首调 allowNoBackup=false 被确认型拒绝 → 升级 error 语义强确认框 → 确认后
    // 带 allowNoBackup=true 重调并完成删除
    const { ElMessage, ElMessageBox } = await import('element-plus')
    expect(wailsMock.DeleteAssetLibrary).toHaveBeenCalledTimes(2)
    expect(wailsMock.DeleteAssetLibrary).toHaveBeenNthCalledWith(1, 'lib-1', false)
    expect(wailsMock.DeleteAssetLibrary).toHaveBeenNthCalledWith(2, 'lib-1', true)
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('删除后无法恢复'),
      '无备份删除确认',
      expect.objectContaining({ type: 'error' })
    )
    expect(ElMessage.success).toHaveBeenCalledWith('素材库已删除')
  })

  it('删除库备份失败后取消强确认：不重调不误删', async () => {
    const { ElMessageBox } = await import('element-plus')
    // 首次删除确认通过、升级强确认取消
    ElMessageBox.confirm.mockResolvedValueOnce(true)
    ElMessageBox.confirm.mockRejectedValueOnce('cancel')
    wailsMock.DeleteAssetLibrary.mockRejectedValue({ code: 'E_ASSETS_DELETE_BACKUP_FAILED', message: '删除前自动备份失败' })
    const wrapper = await createWrapper()
    const statsCallsBefore = wailsMock.GetAssetLibraryStats.mock.calls.length
    await clickButton(wrapper, '删除库')
    await flushPromises()

    expect(wailsMock.DeleteAssetLibrary).toHaveBeenCalledTimes(1)
    // 取消后终止：无库切换重载（统计未被再次拉取，当前库保持）
    expect(wailsMock.GetAssetLibraryStats.mock.calls.length).toBe(statsCallsBefore)
  })

  it('删除库其他真失败：走 handleError 分流不进升级确认', async () => {
    const { ElMessage, ElMessageBox } = await import('element-plus')
    wailsMock.DeleteAssetLibrary.mockRejectedValue({ code: 'E_ASSETS_SCAN_IN_PROGRESS', message: '该素材库已有扫描进行中' })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '删除库')
    await flushPromises()

    expect(wailsMock.DeleteAssetLibrary).toHaveBeenCalledTimes(1)
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1) // 仅首次删除确认，无升级确认
    expect(ElMessage.warning).toHaveBeenCalledWith('该素材库已有扫描进行中')
  })

  // ---- 索引设置（第十批 SRS §2.7.3）：排除规则弹窗接线 ----

  it('索引设置弹窗接线：打开传当前库对象回填，保存分派 UpdateAssetLibraryRules 并刷新', async () => {
    const wrapper = await createWrapper()
    await clickButton(wrapper, '索引设置')
    await flushPromises()

    const rulesStub = wrapper.findComponent({ name: 'AssetRulesDialogStub' })
    expect(rulesStub.exists()).toBe(true)
    expect(rulesStub.props('library')).toMatchObject({ id: 'lib-1' })
    expect(typeof rulesStub.props('execute')).toBe('function')

    // execute 分派：目录/后缀规则透传（弹窗内交互由独立 spec 覆盖）
    await rulesStub.props('execute')(['*草稿*'], ['.tmp'])
    await flushPromises()
    expect(wailsMock.UpdateAssetLibraryRules).toHaveBeenCalledWith('lib-1', ['*草稿*'], ['.tmp'])

    // saved 刷新链：库列表重拉（规则回显同步）+ 库数据刷新（统计/底册）
    const listCallsBefore = wailsMock.ListAssetLibraries.mock.calls.length
    await rulesStub.vm.$emit('saved', { total: 2, excluded: 1 })
    await flushPromises()
    expect(wailsMock.ListAssetLibraries.mock.calls.length).toBeGreaterThan(listCallsBefore)
    expect(wailsMock.GetAssetLibraryStats).toHaveBeenCalledWith('lib-1')
  })

  it('规则保存分派期间置扫描态：重扫进度走现有扫描事件链，落定后复位', async () => {
    let resolveRules
    wailsMock.UpdateAssetLibraryRules.mockImplementation(() => new Promise((resolve) => {
      resolveRules = () => resolve({ total: 0, new: 0, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, excluded: 0, lost: 0, revived: 0, removed: 0 })
    }))
    const wrapper = await createWrapper()
    await clickButton(wrapper, '索引设置')
    await flushPromises()
    const rulesStub = wrapper.findComponent({ name: 'AssetRulesDialogStub' })
    const pending = rulesStub.props('execute')([], [])
    await flushPromises()
    // 重扫期间进度条可见（scanning 置位，progress 事件链复用），落定后复位
    expect(wrapper.find('.scan-progress').exists()).toBe(true)
    resolveRules()
    await pending
    await flushPromises()
    expect(wrapper.find('.scan-progress').exists()).toBe(false)
  })

  // ---- 备份与还原（第十批 SRS §2.7.5）----

  it('备份按钮：调 BackupAssetLibrary 且成功提示含落点路径', async () => {
    const { ElMessage } = await import('element-plus')
    const wrapper = await createWrapper()
    await clickButton(wrapper, '备份')
    await flushPromises()

    expect(wailsMock.BackupAssetLibrary).toHaveBeenCalledWith('lib-1')
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('D:/data/assets/backups/lib-1/20261018-100000.db'))
  })

  it('备份失败经 handleError 分流为 error', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.BackupAssetLibrary.mockRejectedValue({ message: '备份生成失败' })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '备份')
    await flushPromises()

    expect(ElMessage.error).toHaveBeenCalledWith('备份失败: 备份生成失败')
  })

  it('备份按钮在途 loading 防连点：backingUp 驱动 loading 态，完成即恢复可点', async () => {
    let resolveBackup
    wailsMock.BackupAssetLibrary.mockImplementation(() => new Promise((resolve) => { resolveBackup = resolve }))
    const wrapper = await createWrapper()
    const backupBtn = wrapper.findAll('button').find((b) => b.text().trim() === '备份')
    await backupBtn.trigger('click')
    await flushPromises()

    // 在途：按钮 loading（el-button 桩以 disabled 承载 loading），重复点击被
    // onBackupLibrary 的 backingUp 守卫跳过（只发一次请求）
    expect(backupBtn.attributes('disabled')).toBeDefined()
    await backupBtn.trigger('click')
    expect(wailsMock.BackupAssetLibrary).toHaveBeenCalledTimes(1)

    resolveBackup({ libraryId: 'lib-1', fileName: 'x.db', path: 'D:/data/assets/backups/lib-1/x.db' })
    await flushPromises()
    expect(backupBtn.attributes('disabled')).toBeUndefined()
  })

  it('还原弹窗接线：打开传库 ID 与清单/还原分派，restored 触发库数据刷新链', async () => {
    const wrapper = await createWrapper()
    await clickButton(wrapper, '还原')
    await flushPromises()

    const backupStub = wrapper.findComponent({ name: 'AssetBackupDialogStub' })
    expect(backupStub.exists()).toBe(true)
    expect(backupStub.props('libraryId')).toBe('lib-1')
    expect(typeof backupStub.props('listBackups')).toBe('function')
    expect(typeof backupStub.props('executeRestore')).toBe('function')

    // 分派回调透传绑定（清单按库 ID、还原按条目绝对路径）
    await backupStub.props('listBackups')('lib-1')
    expect(wailsMock.ListAssetLibraryBackups).toHaveBeenCalledWith('lib-1')
    await backupStub.props('executeRestore')('D:/data/assets/backups/lib-1/20261017-090000.db')
    await flushPromises()
    expect(wailsMock.RestoreAssetLibrary).toHaveBeenCalledWith('lib-1', 'D:/data/assets/backups/lib-1/20261017-090000.db')

    // restored 刷新链：统计/底册重拉（索引回备份时点）
    const statsCallsBefore = wailsMock.GetAssetLibraryStats.mock.calls.length
    await backupStub.vm.$emit('restored', { path: 'x' })
    await flushPromises()
    expect(wailsMock.GetAssetLibraryStats.mock.calls.length).toBeGreaterThan(statsCallsBefore)
  })

  it('还原成功刷新链：索引整体回滚，详情/预览/选中语境全清（语境变更点位，非数据变更）', async () => {
    const wrapper = await createWrapper()
    // 预置还原前语境：打开幻灯片详情 + 勾选一卡 + 全屏预览
    await openSlideDetail(wrapper)
    await wrapper.findAll('.asset-card')[0].find('.asset-card-check').trigger('change')
    await flushPromises()
    await clickButton(wrapper, '全屏预览')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(true)
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)

    await clickButton(wrapper, '还原')
    await flushPromises()
    const backupStub = wrapper.findComponent({ name: 'AssetBackupDialogStub' })
    await backupStub.vm.$emit('restored', { path: 'x' })
    await flushPromises()

    // 还原前选中的条目可能已不在回滚后索引：详情侧栏驻留陈旧条目会让后续
    // 备注/评分编辑向已不存在条目写回，故详情/预览/选中随刷新链一并清除
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(false)
    expect(wrapper.find('.assets-detail').exists()).toBe(false)
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
    wrapper.unmount()
  })

  // ---- excluded 呈现（第十批 SRS §2.7.3）----

  it('统计行排除计数：excludedCount 非零显示「已排除 N」，零值不显示', async () => {
    wailsMock.GetAssetLibraryStats.mockResolvedValue({ ...mockStats, excludedCount: 3 })
    let wrapper = await createWrapper()
    expect(wrapper.find('.assets-stats-line').text()).toContain('已排除 3')

    wailsMock.GetAssetLibraryStats.mockResolvedValue({ ...mockStats, excludedCount: 0 })
    wrapper = await createWrapper()
    expect(wrapper.find('.assets-stats-line').text()).not.toContain('已排除')
  })

  it('扫描汇总含排除计数（规则重扫/增量扫描 excluded 非零时追加）', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.StartAssetScan.mockResolvedValue({ total: 5, new: 0, updated: 1, unchanged: 2, failed: 0, encrypted: 0, denied: 0, excluded: 2, lost: 0, revived: 0, removed: 0 })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '扫描')
    await flushPromises()

    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('排除 2'))
  })

  // ---- 异步写回守卫（审核 8：跨库串入 / 慢响应覆写防护）----

  it('版本守卫：库切换在途时旧库统计响应写回被丢弃（不覆写新库统计）', async () => {
    const stats2 = { libraryId: 'lib-2', fileCount: 7, slideCount: 9, failedCount: 0, encryptedCount: 0 }
    const stats3 = { libraryId: 'lib-3', fileCount: 1, slideCount: 1, failedCount: 0, encryptedCount: 0 }
    let resolveStats2
    wailsMock.GetAssetLibraryStats.mockImplementation((id) => {
      if (id === 'lib-2') return new Promise((resolve) => { resolveStats2 = () => resolve(stats2) })
      return Promise.resolve(id === 'lib-3' ? stats3 : mockStats)
    })
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '库二' },
      { ...mockLibrary, id: 'lib-3', name: '库三' }
    ])
    const wrapper = await createWrapper()
    const select = wrapper.findComponent(ElSelectStub)

    // 切到 lib-2（统计响应挂起），未落定即再切 lib-3（快速落定）
    await select.vm.$emit('update:modelValue', 'lib-2')
    await select.vm.$emit('change', 'lib-2')
    await flushPromises()
    await select.vm.$emit('update:modelValue', 'lib-3')
    await select.vm.$emit('change', 'lib-3')
    await flushPromises()
    expect(wrapper.find('.assets-stats-line').text()).toContain('文件 1') // lib-3

    // lib-2 慢响应后到：库代次已变，不得覆写 lib-3 统计
    resolveStats2()
    await flushPromises()
    expect(wrapper.find('.assets-stats-line').text()).toContain('文件 1')
    expect(wrapper.find('.assets-stats-line').text()).not.toContain('文件 7')
  })

  it('版本守卫：检索在途时改词重搜，旧检索响应写回被丢弃', async () => {
    let resolveSlowSearch
    wailsMock.SearchAssets.mockImplementation((libId, keyword) => {
      if (keyword === '慢词') {
        return new Promise((resolve) => { resolveSlowSearch = () => resolve({ ...mockSearchPage, total: 99, keyword }) })
      }
      return Promise.resolve({ items: [], total: 5, keyword, flattened: false })
    })
    const wrapper = await createWrapper()

    // 「慢词」检索在途，改词重搜「快词」（快速落定 命中 5）
    await wrapper.find('input').setValue('慢词')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await wrapper.find('input').setValue('快词')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    expect(wrapper.find('.assets-stats-line').text()).toContain('命中 5 条')

    // 旧词慢响应后到：检索代次已变，不得覆写新词结果
    resolveSlowSearch()
    await flushPromises()
    expect(wrapper.find('.assets-stats-line').text()).toContain('命中 5 条')
    expect(wrapper.find('.assets-stats-line').text()).not.toContain('命中 99')
  })

  it('版本守卫：幻灯片滚动追加在途时切库，旧库追加响应丢弃（滚动路径受库代次守卫）', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '库二' }
    ])
    let resolveSlowAppend
    wailsMock.ListAssetSlides.mockImplementation((libId, limit, offset) => {
      if (libId === 'lib-1' && offset > 0) {
        return new Promise((resolve) => {
          resolveSlowAppend = () => resolve({ items: [{ ...mockSlides[0], id: 23, slideNum: 3 }], total: 4 })
        })
      }
      if (libId === 'lib-2') return Promise.resolve({ items: [], total: 0 })
      return Promise.resolve({ items: mockSlides, total: 4 })
    })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    expect(wrapper.findAll('.asset-card').length).toBe(2)

    // 滚动触发下一页追加（响应挂起在途）
    const gallery = wrapper.find('.assets-gallery')
    Object.defineProperty(gallery.element, 'scrollHeight', { value: 500, configurable: true })
    Object.defineProperty(gallery.element, 'clientHeight', { value: 400, configurable: true })
    gallery.element.scrollTop = 150
    await gallery.trigger('scroll')

    // 追加在途切到 lib-2（空库）
    const select = wrapper.findComponent(ElSelectStub)
    await select.vm.$emit('update:modelValue', 'lib-2')
    await select.vm.$emit('change', 'lib-2')
    await flushPromises()
    expect(wrapper.findAll('.asset-card').length).toBe(0)

    // 旧库追加慢响应后到：库代次已变，不得拼入 lib-2 视图
    resolveSlowAppend()
    await flushPromises()
    expect(wrapper.findAll('.asset-card').length).toBe(0)
    expect(wailsMock.ListAssetSlides).toHaveBeenLastCalledWith('lib-2', 50, 0)
  })

  it('幻灯片滚动追加在途防重：loadingMore 置位拦截在途窗口内的连续滚动触发（不重复追加同 offset）', async () => {
    let appendCalls = 0
    let resolveAppend
    wailsMock.ListAssetSlides.mockImplementation((libId, limit, offset) => {
      if (offset === 0) return Promise.resolve({ items: mockSlides, total: 4 })
      appendCalls++
      return new Promise((resolve) => {
        resolveAppend = () => resolve({ items: [{ ...mockSlides[0], id: 23, slideNum: 3 }], total: 4 })
      })
    })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    const gallery = wrapper.find('.assets-gallery')
    Object.defineProperty(gallery.element, 'scrollHeight', { value: 500, configurable: true })
    Object.defineProperty(gallery.element, 'clientHeight', { value: 400, configurable: true })
    gallery.element.scrollTop = 150
    // 首次 scroll 触发追加（响应挂起保持 loadingMore=true），在途窗口内第二次
    // scroll 应被防重检查拦截——不发同 offset 的重复请求
    await gallery.trigger('scroll')
    await gallery.trigger('scroll')
    expect(appendCalls).toBe(1)

    resolveAppend()
    await flushPromises()
    expect(wrapper.findAll('.asset-card').length).toBe(3)
  })

  it('检索空态引导：命中存在但被当前视图类型过滤为空时提示切换视图（审核 11）', async () => {
    // 命中全为幻灯片类型（文件视图下被类型过滤清空），total > 0
    wailsMock.SearchAssets.mockResolvedValue({
      items: [{ ...mockSearchPage.items[1] }],
      total: 3,
      keyword: '城商行',
      flattened: false
    })
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    expect(wrapper.find('.el-empty').exists()).toBe(true)
    expect(wrapper.find('.el-empty').text()).toContain('已命中 3 条')
    expect(wrapper.find('.el-empty').text()).toContain('切换文件/幻灯片视图')
  })

  it('聚合展开在途守卫：检索条件变更后旧聚合响应丢弃，重新展开重新拉取（审核 13）', async () => {
    let resolveAgg
    wailsMock.SearchAssets.mockImplementation((libId, keyword, opts) => {
      if (opts.flatten) {
        return new Promise((resolve) => { resolveAgg = () => resolve(mockSearchPage) })
      }
      return Promise.resolve(mockSearchPage)
    })
    const wrapper = await createWrapper()
    // D8#2 平铺默认开：聚合展开链路用例先回关平铺，使检索返回聚合卡（命中态
    // 点击展开才走 flatten:true 子项检索）；检索前回关不触发 runSearch（非检索态）
    const flattenSwitchOff = wrapper.find('.flatten-switch:not(.slide-own-switch)').findComponent(ElSwitchStub)
    await flattenSwitchOff.vm.$emit('update:modelValue', false)
    await flattenSwitchOff.vm.$emit('change', false)
    await flushPromises()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    // 展开聚合卡：flatten 检索挂起
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()

    // 改词重搜（检索条件变更），随后旧聚合响应才返回
    await wrapper.find('input').setValue('新词')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    resolveAgg()
    await flushPromises()

    // 守卫生效：旧响应未写入 aggChildren → 重新展开应再次发起 flatten 检索
    const callsBefore = wailsMock.SearchAssets.mock.calls.length
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets.mock.calls.length).toBe(callsBefore + 1)
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '新词', {
      flatten: true,
      tagIds: [],
      slideTagOnlyOwn: false,
      limit: 200,
      offset: 0
    })
  })

  it('聚合展开在途守卫：仅自身标签开关切换后旧聚合响应丢弃，重新展开按新口径重拉（审查 P2：contextChanged 快照含 slideTagOnlyOwn）', async () => {
    let resolveAgg
    wailsMock.SearchAssets.mockImplementation((libId, keyword, opts) => {
      if (opts.flatten) {
        return new Promise((resolve) => { resolveAgg = () => resolve(mockSearchPage) })
      }
      return Promise.resolve(mockSearchPage)
    })
    const wrapper = await createWrapper()
    // D8#2 平铺默认开：聚合展开链路用例先回关平铺，使检索返回聚合卡（命中态
    // 点击展开才走 flatten:true 子项检索）；检索前回关不触发 runSearch（非检索态）
    const flattenSwitchOff = wrapper.find('.flatten-switch:not(.slide-own-switch)').findComponent(ElSwitchStub)
    await flattenSwitchOff.vm.$emit('update:modelValue', false)
    await flattenSwitchOff.vm.$emit('change', false)
    await flushPromises()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    // 展开聚合卡：flatten 检索挂起
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()

    // 展开在途切「仅幻灯片自身标签」开关（清空 aggChildren 的语境变更点），
    // 随后旧口径聚合响应才返回
    const ownSwitch = wrapper.find('.slide-own-switch').findComponent(ElSwitchStub)
    await ownSwitch.vm.$emit('update:modelValue', true)
    await ownSwitch.vm.$emit('change', true)
    await flushPromises()
    resolveAgg()
    await flushPromises()

    // 守卫生效：旧响应未复活 aggChildren → 重新展开应再次发起 flatten 检索，
    // 且以新开关口径发起（快照值透传）
    const callsBefore = wailsMock.SearchAssets.mock.calls.length
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets.mock.calls.length).toBe(callsBefore + 1)
    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行', {
      flatten: true,
      tagIds: [],
      slideTagOnlyOwn: true,
      limit: 200,
      offset: 0
    })
  })

  it('聚合展开在途守卫：平铺开关切换后旧聚合响应丢弃，重新展开重新拉取（审查 P2：contextChanged 快照含 flatten）', async () => {
    // flatten 请求按发起序挂起：pendingAgg[0] = toggleAgg 的聚合请求；切平铺开关
    // 后 onFlattenChange 触发的 runSearch 也是 flatten 形态（v-model 先更新）会入队
    // pendingAgg[1]，须精确 resolve 第一个才能驱动 toggleAgg 的在途写回分支
    const pendingAgg = []
    wailsMock.SearchAssets.mockImplementation((libId, keyword, opts) => {
      if (opts.flatten) {
        return new Promise((resolve) => { pendingAgg.push(resolve) })
      }
      return Promise.resolve(mockSearchPage)
    })
    const wrapper = await createWrapper()
    // D8#2 平铺默认开：聚合展开链路用例先回关平铺，使检索返回聚合卡（命中态
    // 点击展开才走 flatten:true 子项检索）；检索前回关不触发 runSearch（非检索态）
    const flattenSwitchOff = wrapper.find('.flatten-switch:not(.slide-own-switch)').findComponent(ElSwitchStub)
    await flattenSwitchOff.vm.$emit('update:modelValue', false)
    await flattenSwitchOff.vm.$emit('change', false)
    await flushPromises()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    // 展开聚合卡：flatten 检索挂起（pendingAgg[0]）
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(pendingAgg.length).toBe(1)

    // 展开在途切平铺开关（onFlattenChange 清空 aggChildren 的语境变更点；开关自身
    // 触发的检索入队 pendingAgg[1] 挂起），随后旧聚合响应才返回
    const flattenSwitch = wrapper.find('.flatten-switch:not(.slide-own-switch)').findComponent(ElSwitchStub)
    await flattenSwitch.vm.$emit('update:modelValue', true)
    await flattenSwitch.vm.$emit('change', true)
    await flushPromises()
    expect(pendingAgg.length).toBe(2)
    pendingAgg[0](mockSearchPage)
    await flushPromises()

    // 守卫生效：旧响应未复活 aggChildren → 重新展开应再次发起 flatten 检索
    const callsBefore = wailsMock.SearchAssets.mock.calls.length
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wailsMock.SearchAssets.mock.calls.length).toBe(callsBefore + 1)
  })

  it('库不存在自愈：统计报 E_ASSETS_LIB_NOT_FOUND 时静默重拉库列表并落定剩余库（审核 14）', async () => {
    const remainingLib = { ...mockLibrary, id: 'lib-2', name: '剩余库' }
    let listCalls = 0
    wailsMock.ListAssetLibraries.mockImplementation(() => {
      listCalls++
      // 首拉含已删库（前端列表过期），重拉仅剩剩余库（模拟外部删除后自愈）
      return listCalls === 1
        ? Promise.resolve([{ ...mockLibrary, id: 'lib-gone', name: '已删库' }, remainingLib])
        : Promise.resolve([remainingLib])
    })
    let statsCalls = 0
    wailsMock.GetAssetLibraryStats.mockImplementation((id) => {
      statsCalls++
      if (id === 'lib-gone') {
        return Promise.reject({ code: 'E_ASSETS_LIB_NOT_FOUND', message: '素材库不存在或已被删除，请刷新素材库列表' })
      }
      return Promise.resolve({ libraryId: id, fileCount: 3, slideCount: 3, failedCount: 0, encryptedCount: 0 })
    })
    const wrapper = await createWrapper()
    await flushPromises()

    // 自愈链：重拉库列表 → 当前库不在新列表 → 自动落定剩余库并加载其数据
    expect(wailsMock.ListAssetLibraries).toHaveBeenCalledTimes(2)
    expect(wailsMock.GetAssetLibraryStats).toHaveBeenCalledWith('lib-2')
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledWith('lib-2')
    expect(wrapper.find('.assets-stats-line').text()).toContain('文件 3')
    // 自愈路径静默（不弹 error 误导用户）
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  // ---- 缩略图链路（PR3：懒加载 / 降级 / 进度与失败收集 / 跳转 / 设置）----

  it('降级提示条：本机无 PowerPoint（Available=false）常驻信息提示，可用时不显示', async () => {
    const wrapper = await createWrapper()
    expect(wrapper.find('.thumbs-degrade-banner').exists()).toBe(true)
    expect(wrapper.find('.thumbs-degrade-banner').text()).toContain('未检测到 PowerPoint')
    expect(wrapper.find('.thumbs-degrade-banner').text()).toContain('缩略图与跳转不可用')

    // 探测可用：不显示提示条
    wailsMock.AssetThumbsAvailable.mockResolvedValue(true)
    const okWrapper = await createWrapper()
    expect(okWrapper.find('.thumbs-degrade-banner').exists()).toBe(false)
  })

  it('画廊缩略图：命中返回串渲染 data URL 图片（前缀 data:image/png;base64,，画廊一律 tier=0），空串回退无预览占位', async () => {
    wailsMock.GetAssetThumb.mockResolvedValue('QUFB')
    const wrapper = await createWrapper()
    const imgs = wrapper.findAll('.asset-card .asset-thumb img')
    expect(imgs.length).toBe(2)
    expect(imgs[0].attributes('src')).toBe('data:image/png;base64,QUFB')
    // 文件卡取第 1 页封面、档位 small（tier=0）
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledWith('lib-1', 11, 1, 0)
    expect(wailsMock.GetAssetThumb).toHaveBeenCalledWith('lib-1', 12, 1, 0)

    // 空串 = 未生成：占位文案
    wailsMock.GetAssetThumb.mockResolvedValue('')
    const missWrapper = await createWrapper()
    const placeholders = missWrapper.findAll('.asset-card .asset-thumb-placeholder')
    expect(placeholders.length).toBe(2)
    expect(placeholders[0].text()).toBe('无预览')
  })

  it('缩略图进度与失败收集：progress 事件更新进度条并按增量收集失败文件，详情侧边栏提供重试入口，done 事件复位', async () => {
    const wrapper = await createWrapper()

    // 本库进度事件：failed 0→1，当前文件即新失败文件；他库事件不串入
    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-other', done: 1, total: 2, currentFile: '其他库文件.pptx', failed: 1 })
    await flushPromises()
    expect(wrapper.find('.thumbs-progress').exists()).toBe(false)

    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-1', done: 1, total: 2, currentFile: '方案B/损坏文件.pptx', failed: 1 })
    await flushPromises()
    expect(wrapper.find('.thumbs-progress').exists()).toBe(true)
    expect(wrapper.find('.thumbs-progress').text()).toContain('生成缩略图 1/2')
    expect(wrapper.find('.thumbs-progress').text()).toContain('方案B/损坏文件.pptx')

    // 打开失败文件（id=12）详情：出现失败告警与重试入口
    await wrapper.findAll('.asset-card')[1].trigger('click')
    await flushPromises()
    const failed = wrapper.find('.detail-thumb-failed')
    expect(failed.exists()).toBe(true)
    expect(failed.text()).toContain('缩略图生成失败')
    expect(failed.text()).toContain('重试生成')

    // 点重试：以当前库 + 详情文件 ID + 归属路径重新入队
    await clickButton(wrapper, '重试生成')
    await flushPromises()
    expect(wailsMock.RetryAssetThumb).toHaveBeenCalledWith('lib-1', 12, '方案B/损坏文件.pptx')
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已重新入队生成')

    // done 事件：进度复位 + 失败集合清空（详情告警随之消失）+ 失效代次自增触发补图
    const epochBefore = wrapper.findComponent({ name: 'AssetThumb' })
    eventHandlers['assets-thumbs:done']({ libraryId: 'lib-1', total: 2, generated: 1, skipped: 0, failed: 1, canceled: false })
    await flushPromises()
    expect(wrapper.find('.thumbs-progress').exists()).toBe(false)
    expect(wrapper.find('.detail-thumb-failed').exists()).toBe(false)
    // epoch 自增：详情大图 AssetThumb 收到新代次（补图入口）
    expect(epochBefore.props('epoch')).not.toBe(0)
  })

  it('打开跳转：幻灯片详情以根路径+相对路径与页码调用 JumpToAssetSlide，BUSY 预期拒绝分流 warning 无 error', async () => {
    wailsMock.AssetThumbsAvailable.mockResolvedValue(true)
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    // 幻灯片详情：跳转按钮可见（Available=true）
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    const jumpBtn = wrapper.findAll('button').find((b) => b.text().trim() === '打开并跳到此页')
    expect(jumpBtn).toBeTruthy()

    // 成功：绝对路径 = 库根路径（去尾斜杠）+ 相对路径（正斜杠保持），页码取详情页码
    await jumpBtn.trigger('click')
    await flushPromises()
    expect(wailsMock.JumpToAssetSlide).toHaveBeenCalledWith('D:/assets/方案A/城商行方案.pptx', 1)

    // BUSY 预期拒绝：handleError 分流 warning（不拼前缀），无 error
    const { ElMessage } = await import('element-plus')
    wailsMock.JumpToAssetSlide.mockRejectedValue({ code: 'E_ASSETS_THUMBS_BUSY', message: '缩略图队列忙，请稍后重试' })
    await jumpBtn.trigger('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('缩略图队列忙，请稍后重试')
    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  it('打开跳转：Available=false（降级态）时跳转按钮不渲染', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    const jumpBtn = wrapper.findAll('button').find((b) => b.text().trim() === '打开并跳到此页')
    expect(jumpBtn).toBeUndefined()
  })

  it('缩略图设置弹窗：打开拉取设置，老配置无 assets 段时草稿回退默认值，保存回填 assets 段整体提交', async () => {
    // 老配置无 assets 段：draft 逐字段回退默认（320/1600/1024/1024/auto true）
    wailsMock.GetSettings.mockResolvedValue({ themeMode: 'system', terminalFontSize: 14 })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '缩略图设置')
    await flushPromises()

    expect(wailsMock.GetSettings).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.stub-dialog').exists()).toBe(true)
    const inputNumbers = wrapper.findAllComponents(ElInputNumberStub)
    expect(inputNumbers.length).toBe(4)
    expect(inputNumbers[0].props('modelValue')).toBe(320) // 小图分辨率
    expect(inputNumbers[1].props('modelValue')).toBe(1600) // 高清分辨率
    expect(inputNumbers[2].props('modelValue')).toBe(1024) // 小图缓存上限
    expect(inputNumbers[3].props('modelValue')).toBe(1024) // 高清缓存上限
    // 防清空写 null 脏值契约：value-on-clear 必带
    expect(inputNumbers[0].props('valueOnClear')).toBe(320)
    expect(inputNumbers[1].props('valueOnClear')).toBe(1600)
    expect(inputNumbers[2].props('valueOnClear')).toBe(1024)
    expect(inputNumbers[3].props('valueOnClear')).toBe(1024)

    // 修改高清分辨率后保存：SaveSettings 收到含 assets 段的完整设置对象，其余段保留
    await inputNumbers[1].vm.$emit('update:modelValue', 1920)
    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.SaveSettings).toHaveBeenCalledTimes(1)
    const saved = wailsMock.SaveSettings.mock.calls[0][0]
    expect(saved.assets).toEqual({ auto: true, smallWidth: 320, hdWidth: 1920, smallQuotaMB: 1024, hdQuotaMB: 1024 })
    expect(saved.themeMode).toBe('system')
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('缩略图设置已保存，分辨率变更将自动重新生成')
    expect(wrapper.find('.stub-dialog').exists()).toBe(false)
  })

  it('缩略图设置弹窗：含 assets 段配置回显真实值（非默认值）', async () => {
    wailsMock.GetSettings.mockResolvedValue({
      ...mockSettings,
      assets: { auto: false, smallWidth: 480, hdWidth: 2560, smallQuotaMB: 2048, hdQuotaMB: 4096 }
    })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '缩略图设置')
    await flushPromises()

    const inputNumbers = wrapper.findAllComponents(ElInputNumberStub)
    expect(inputNumbers[0].props('modelValue')).toBe(480)
    expect(inputNumbers[1].props('modelValue')).toBe(2560)
    expect(inputNumbers[2].props('modelValue')).toBe(2048)
    expect(inputNumbers[3].props('modelValue')).toBe(4096)
    const { ElMessage } = await import('element-plus')
    // 保存失败经 handleError 分流（真失败 error 带前缀）
    wailsMock.SaveSettings.mockRejectedValue({ message: '磁盘写入失败' })
    await clickButton(wrapper, '保存')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('保存设置失败: 磁盘写入失败')
    expect(wrapper.find('.stub-dialog').exists()).toBe(true) // 保存失败弹窗不关
  })

  it('缩略图设置弹窗：GetSettings 失败时基底缺失拦截保存（整体替换语义防空壳清掉其他段）', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.GetSettings.mockRejectedValue({ message: '磁盘读取失败' })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '缩略图设置')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('加载设置失败: 磁盘读取失败')

    // 基底 null：保存被拦截提示重开，不调 SaveSettings（空壳提交会清掉其他设置段）
    await clickButton(wrapper, '保存')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('设置加载失败，请关闭后重新打开设置')
    expect(wailsMock.SaveSettings).not.toHaveBeenCalled()
    expect(wrapper.find('.stub-dialog').exists()).toBe(true)
  })

  it('切库复位缩略图运行态：旧库批次进行中切库后进度条消失（防终态事件被 libraryId 过滤致永挂）', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '备份库' }
    ])
    const wrapper = await createWrapper()
    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-1', done: 1, total: 3, currentFile: '方案A/城商行方案.pptx', failed: 1 })
    await flushPromises()
    expect(wrapper.find('.thumbs-progress').exists()).toBe(true)

    // 切到 lib-2：进度条复位（旧库 done/error 事件按 libraryId 过滤不会到达）
    const select = wrapper.findComponent(ElSelectStub)
    await select.vm.$emit('update:modelValue', 'lib-2')
    await select.vm.$emit('change', 'lib-2')
    await flushPromises()
    expect(wrapper.find('.thumbs-progress').exists()).toBe(false)

    // 失败集合随切库清空：旧库失败路径不归因到新库语境
    expect(wrapper.text()).not.toContain('缩略图生成失败')
  })

  it('取消缩略图生成：CancelAssetThumbs 按 true/false 分流 info 与 warning 提示', async () => {
    const wrapper = await createWrapper()
    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-1', done: 1, total: 2, currentFile: '方案A/城商行方案.pptx', failed: 0 })
    await flushPromises()
    expect(wrapper.find('.thumbs-progress').exists()).toBe(true)

    // 有在途批次：true → info 中性提示
    wailsMock.CancelAssetThumbs.mockResolvedValue(true)
    await clickButton(wrapper, '取消生成')
    await flushPromises()
    expect(wailsMock.CancelAssetThumbs).toHaveBeenCalledWith('lib-1')
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.info).toHaveBeenCalledWith('已请求取消缩略图生成')

    // 无可取消任务：false → warning
    wailsMock.CancelAssetThumbs.mockResolvedValue(false)
    await clickButton(wrapper, '取消生成')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('当前没有可取消的生成任务')
  })

  it('缩略图 error 事件：仅复位进度不弹提示（契约：错误详情后端 slog 已记录，防与 Promise catch 双弹）', async () => {
    const { ElMessage } = await import('element-plus')
    const wrapper = await createWrapper()
    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-1', done: 1, total: 2, currentFile: '方案A/城商行方案.pptx', failed: 0 })
    await flushPromises()
    expect(wrapper.find('.thumbs-progress').exists()).toBe(true)

    eventHandlers['assets-thumbs:error']({ libraryId: 'lib-1', message: '入队队列满' })
    await flushPromises()
    expect(wrapper.find('.thumbs-progress').exists()).toBe(false)
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(ElMessage.warning).not.toHaveBeenCalled()
  })

  // ---- PPT 拆分链路（SRS §2.2.3 / §2.2.5 方式 2）----

  const mockSplitBatch = {
    total: 2,
    split: 1,
    skipped: 1,
    failed: 0,
    items: [
      { relPath: '方案A/城商行方案.pptx', status: 'split', message: '' },
      { relPath: '方案B/损坏文件.pptx', status: 'skipped', message: '已拆分且源未变化' }
    ]
  }

  it('库级拆分主链路：「拆分全部 PPT」打开确认弹窗（落点默认缓存目录），确认调 SplitAssetLibrary 并汇总提示', async () => {
    wailsMock.SplitAssetLibrary.mockResolvedValue(mockSplitBatch)
    const wrapper = await createWrapper()

    await clickButton(wrapper, '拆分全部 PPT')
    await flushPromises()
    const dialog = wrapper.find('.stub-dialog')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('全部正常状态的 PPT')

    // 落点 radio 默认「素材库缓存目录」（弹窗内 radio-group 的 v-model 初值；stub 的
    // group/radio 无 provide 透传，选中态经 group 组件 props 断言），两个选项文本渲染
    const dialogGroup = wrapper.findAllComponents(ElRadioGroupStub).at(-1)
    expect(dialogGroup.props('modelValue')).toBe('cache')
    const radios = dialog.findAll('input[type=radio]')
    expect(radios.length).toBe(2)
    expect(dialog.text()).toContain('素材库缓存目录（推荐）')
    expect(dialog.text()).toContain('源文件所在目录')
    // 默认提示为缓存目录说明（非源目录警示）
    expect(dialog.find('.split-source-warning').exists()).toBe(false)
    expect(dialog.find('.split-cache-hint').exists()).toBe(true)

    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    expect(wailsMock.SplitAssetLibrary).toHaveBeenCalledWith('lib-1', false)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('拆分完成：共 2 个文件'))
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('跳过 1'))
    // 拆分完成弹窗关闭
    expect(wrapper.find('.stub-dialog').exists()).toBe(false)
  })

  it('落点选择源目录：展示自动排除索引警示，确认以 outputToSourceDir=true 调用', async () => {
    wailsMock.SplitAssetLibrary.mockResolvedValue(mockSplitBatch)
    const wrapper = await createWrapper()
    await clickButton(wrapper, '拆分全部 PPT')
    await flushPromises()

    // 弹窗内落点 radio-group（DOM 序最后一个）：emit 选中「源文件所在目录」→ 警示出现
    const dialogGroup = wrapper.findAllComponents(ElRadioGroupStub).at(-1)
    await dialogGroup.vm.$emit('update:modelValue', 'source')
    await flushPromises()
    const dialog = wrapper.find('.stub-dialog')
    expect(dialog.find('.split-source-warning').exists()).toBe(true)
    expect(dialog.find('.split-source-warning').text()).toContain('已自动排除索引')
    expect(dialog.find('.split-cache-hint').exists()).toBe(false)

    await clickButton(wrapper, '开始拆分')
    await flushPromises()
    expect(wailsMock.SplitAssetLibrary).toHaveBeenCalledWith('lib-1', true)
  })

  it('库级拆分失败明细：failed>0 时汇总提示并自动弹出逐文件明细（异常隔离原因可见）', async () => {
    wailsMock.SplitAssetLibrary.mockResolvedValue({
      total: 3,
      split: 1,
      skipped: 0,
      failed: 2,
      items: [
        { relPath: '方案A/城商行方案.pptx', status: 'split', message: '' },
        { relPath: '方案B/损坏文件.pptx', status: 'failed', message: 'zip: not a valid zip file' },
        { relPath: '方案C/加密文件.pptx', status: 'failed', message: '文件已加密' }
      ]
    })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '拆分全部 PPT')
    await flushPromises()
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('失败 2'))
    // 确认弹窗已关、明细弹窗自动打开（stub 不渲染 title，按唯一可见弹窗断言内容）：
    // 逐文件展示状态与失败原因
    const dialogs = wrapper.findAll('.stub-dialog')
    expect(dialogs.length).toBe(1)
    expect(dialogs[0].text()).toContain('方案B/损坏文件.pptx')
    expect(dialogs[0].text()).toContain('zip: not a valid zip file')
    expect(dialogs[0].text()).toContain('方案C/加密文件.pptx')

    // 关闭明细弹窗
    await clickButton(wrapper, '关闭')
    await flushPromises()
    expect(wrapper.find('.stub-dialog').exists()).toBe(false)
  })

  it('拆分进度事件：按库过滤更新进度条（文件级 + 页级文案），取消按钮按 true/false 分流提示', async () => {
    const { ElMessage } = await import('element-plus')
    const wrapper = await createWrapper()

    // 他库进度事件不串入
    eventHandlers['assets-split:progress']({ libraryId: 'lib-other', done: 1, total: 3, currentFile: '其他.pptx', pageDone: 1, pageTotal: 5 })
    await flushPromises()
    expect(wrapper.find('.split-progress').exists()).toBe(false)

    // 本库进度事件：进度条出现（文件级 + 页级 + 当前文件）
    eventHandlers['assets-split:progress']({ libraryId: 'lib-1', done: 1, total: 3, currentFile: '方案A/城商行方案.pptx', pageDone: 2, pageTotal: 5 })
    await flushPromises()
    expect(wrapper.find('.split-progress').exists()).toBe(true)
    expect(wrapper.find('.split-progress').text()).toContain('拆分中 1/3')
    expect(wrapper.find('.split-progress').text()).toContain('第 2/5 页')
    expect(wrapper.find('.split-progress').text()).toContain('方案A/城商行方案.pptx')

    // 取消：有在途任务 true → info 中性提示
    wailsMock.CancelAssetSplit.mockResolvedValue(true)
    await clickButton(wrapper, '取消拆分')
    await flushPromises()
    expect(wailsMock.CancelAssetSplit).toHaveBeenCalledWith('lib-1')
    expect(ElMessage.info).toHaveBeenCalledWith('已请求取消拆分')

    // 无可取消任务 false → warning
    wailsMock.CancelAssetSplit.mockResolvedValue(false)
    await clickButton(wrapper, '取消拆分')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('当前没有可取消的拆分任务')
  })

  it('拆分 done/error 事件：仅复位进度不弹提示（汇总与错误分流由 Promise 分支统一执行防双弹）', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.SplitAssetLibrary.mockImplementation(() => new Promise(() => {})) // 挂起不落定
    const wrapper = await createWrapper()
    await clickButton(wrapper, '拆分全部 PPT')
    await flushPromises()
    await clickButton(wrapper, '开始拆分')
    await flushPromises()
    expect(wrapper.find('.split-progress').exists()).toBe(true)

    // done 事件先于 Promise 返回：仅复位进度展示（进度文案回「正在准备拆分」），
    // 不弹汇总提示（Promise 分支负责）
    eventHandlers['assets-split:done']({ libraryId: 'lib-1', result: mockSplitBatch })
    await flushPromises()
    expect(ElMessage.success).not.toHaveBeenCalled()
    expect(wrapper.find('.split-progress').exists()).toBe(true)
    expect(wrapper.find('.split-progress').text()).toContain('正在准备拆分')

    // error 事件：仅复位进度不弹（Promise catch 的 handleError 负责分流）
    eventHandlers['assets-split:error']({ libraryId: 'lib-1', message: '拆分已取消' })
    await flushPromises()
    expect(ElMessage.warning).not.toHaveBeenCalled()
    expect(ElMessage.error).not.toHaveBeenCalled()
  })

  it('拆分取消错误：Promise reject E_ASSETS_CANCELLED 经 handleError 分流 warning 中性提示', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.SplitAssetLibrary.mockRejectedValue({ code: 'E_ASSETS_CANCELLED', message: '拆分已取消' })
    const wrapper = await createWrapper()
    await clickButton(wrapper, '拆分全部 PPT')
    await flushPromises()
    await clickButton(wrapper, '开始拆分')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('拆分已取消')
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(wrapper.find('.split-progress').exists()).toBe(false)
  })

  it('单文件拆分入口：文件详情「拆分此 PPT」打开共用弹窗，确认调 SplitAssetFile 并提示页数；增量命中提示已最新', async () => {
    const wrapper = await createWrapper()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()

    // 文件详情（.pptx）出现单文件拆分入口，打开同一确认弹窗（单文件语境文案）
    await clickButton(wrapper, '拆分此 PPT')
    await flushPromises()
    const dialog = wrapper.find('.stub-dialog')
    expect(dialog.exists()).toBe(true)
    expect(dialog.text()).toContain('城商行方案')

    // 成功：拆出页数提示
    wailsMock.SplitAssetFile.mockResolvedValue({ outputDir: 'D:/data/derived/x', files: ['a-001.pptx'], pageCount: 3, skipped: false, skipReason: '' })
    await clickButton(wrapper, '开始拆分')
    await flushPromises()
    expect(wailsMock.SplitAssetFile).toHaveBeenCalledWith('lib-1', '方案A/城商行方案.pptx', false)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('拆分完成：共 3 页')
    expect(wrapper.find('.stub-dialog').exists()).toBe(false)

    // 增量命中（skipped）：info「已最新」提示而非成功汇总
    await clickButton(wrapper, '拆分此 PPT')
    await flushPromises()
    wailsMock.SplitAssetFile.mockResolvedValue({ outputDir: '', files: [], pageCount: 3, skipped: true, skipReason: '已拆分且源未变化' })
    await clickButton(wrapper, '开始拆分')
    await flushPromises()
    expect(ElMessage.info).toHaveBeenCalledWith('已拆分且源未变化')
  })

  it('单文件拆分入口：非 pptx 文件详情不提供「拆分此 PPT」按钮', async () => {
    wailsMock.ListAssetFiles.mockResolvedValue([
      mockFiles[0],
      { ...mockFiles[1], path: '方案B/说明文档.txt', title: '说明文档' }
    ])
    const wrapper = await createWrapper()
    await wrapper.findAll('.asset-card')[1].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('button').some((b) => b.text().trim() === '拆分此 PPT')).toBe(false)
  })

  it('方式 2 打开拆分单页：已拆分文件的幻灯片详情显示按钮，点击以库/相对路径/页码调用；未拆分隐藏', async () => {
    // 第一次查询（打开详情）返回无记录 → 按钮隐藏；拆分完成后刷新返回有记录 → 显示
    wailsMock.GetAssetSplitRecord
      .mockResolvedValueOnce({ exists: false, outputDir: '', pageCount: 0, splitAt: '', upToDate: false })
      .mockResolvedValue({ exists: true, outputDir: 'D:/data/assets/lib-1/derived/方案A/城商行方案', pageCount: 2, splitAt: '2026-10-14 10:00:00', upToDate: true })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    // 查询以详情归属文件相对路径发起；无记录时按钮隐藏（非禁用）
    expect(wailsMock.GetAssetSplitRecord).toHaveBeenCalledWith('lib-1', '方案A/城商行方案.pptx')
    expect(wrapper.findAll('button').some((b) => b.text().trim() === '打开拆分单页')).toBe(false)

    // 拆分 done 事件（本库）：刷新拆分记录 → 按钮出现
    eventHandlers['assets-split:done']({ libraryId: 'lib-1', result: mockSplitBatch })
    await flushPromises()
    expect(wailsMock.GetAssetSplitRecord).toHaveBeenCalledTimes(2)
    const openBtn = wrapper.findAll('button').find((b) => b.text().trim() === '打开拆分单页')
    expect(openBtn).toBeTruthy()

    // 点击：以当前库 + 归属文件相对路径 + 详情页码调用；成功不弹提示
    await openBtn.trigger('click')
    await flushPromises()
    expect(wailsMock.OpenAssetSplitPage).toHaveBeenCalledWith('lib-1', '方案A/城商行方案.pptx', 1)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).not.toHaveBeenCalled()

    // 失败：handleError 分流 error（带前缀）
    wailsMock.OpenAssetSplitPage.mockRejectedValue({ message: '拆分产物不存在或已被清理' })
    await openBtn.trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('打开拆分单页失败: 拆分产物不存在或已被清理')
  })

  it('方式 2：他库拆分 done 事件不刷新当前详情拆分记录（libraryId 过滤）', async () => {
    wailsMock.GetAssetSplitRecord.mockResolvedValue({ exists: false, outputDir: '', pageCount: 0, splitAt: '', upToDate: false })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wailsMock.GetAssetSplitRecord).toHaveBeenCalledTimes(1)

    eventHandlers['assets-split:done']({ libraryId: 'lib-other', result: mockSplitBatch })
    await flushPromises()
    expect(wailsMock.GetAssetSplitRecord).toHaveBeenCalledTimes(1)
  })

  it('切库复位拆分任务态：在途任务进行中切库后进度条消失（防终态事件被 libraryId 过滤致永挂）', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '备份库' }
    ])
    const wrapper = await createWrapper()
    eventHandlers['assets-split:progress']({ libraryId: 'lib-1', done: 1, total: 3, currentFile: '方案A/城商行方案.pptx', pageDone: 1, pageTotal: 2 })
    await flushPromises()
    expect(wrapper.find('.split-progress').exists()).toBe(true)

    const select = wrapper.findComponent(ElSelectStub)
    await select.vm.$emit('update:modelValue', 'lib-2')
    await select.vm.$emit('change', 'lib-2')
    await flushPromises()
    expect(wrapper.find('.split-progress').exists()).toBe(false)
  })

  it('拆分请求序号守卫：切库后旧库在途 Promise 落定，新库拆分态不被清除且旧库 toast 不弹', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '备份库' }
    ])
    let resolveOldSplit
    wailsMock.SplitAssetLibrary.mockImplementation((libId) => {
      if (libId === 'lib-1') {
        return new Promise((resolve) => { resolveOldSplit = () => resolve(mockSplitBatch) })
      }
      return Promise.resolve(mockSplitBatch)
    })
    const wrapper = await createWrapper()

    // lib-1 拆分在途（Promise 挂起，进度条展示）
    await clickButton(wrapper, '拆分全部 PPT')
    await flushPromises()
    await clickButton(wrapper, '开始拆分')
    await flushPromises()
    expect(wrapper.find('.split-progress').exists()).toBe(true)

    // 切到 lib-2：任务态复位；随即在新库发起新拆分（快速落定）
    const select = wrapper.findComponent(ElSelectStub)
    await select.vm.$emit('update:modelValue', 'lib-2')
    await select.vm.$emit('change', 'lib-2')
    await flushPromises()
    expect(wrapper.find('.split-progress').exists()).toBe(false)

    await clickButton(wrapper, '拆分全部 PPT')
    await flushPromises()
    await clickButton(wrapper, '开始拆分')
    await flushPromises()
    expect(wailsMock.SplitAssetLibrary).toHaveBeenLastCalledWith('lib-2', false)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.split-progress').exists()).toBe(false)

    // 旧库慢 Promise 后落定：拆分代次已变——不得弹旧库 toast、不得改写任务态
    resolveOldSplit()
    await flushPromises()
    expect(ElMessage.success).toHaveBeenCalledTimes(1)
    expect(ElMessage.info).not.toHaveBeenCalled()
    expect(wrapper.find('.split-progress').exists()).toBe(false)
  })

  // ---- 列表视图（SRS §2.3 PR1）：displayMode 呈现维度切换 ----

  // 呈现维度切换控件是第二个 el-radio-group（DOM 序在视图切换之后）
  const switchDisplayMode = async (wrapper, mode) => {
    const group = wrapper.findAllComponents(ElRadioGroupStub)[1]
    await group.vm.$emit('update:modelValue', mode)
    await flushPromises()
  }

  it('呈现维度切换：网格/列表互斥渲染，数据语境同一（列表行同源卡片模型）', async () => {
    const wrapper = await createWrapper()
    // 默认网格：画廊容器存在、列表容器不存在
    expect(wrapper.find('.assets-gallery').exists()).toBe(true)
    expect(wrapper.find('.asset-list').exists()).toBe(false)

    await switchDisplayMode(wrapper, 'list')
    expect(wrapper.find('.asset-list').exists()).toBe(true)
    expect(wrapper.find('.assets-gallery').exists()).toBe(false)
    // 列表行消费同一 displayCards：文件条目标题/路径/大小在行内可见
    const rows = wrapper.findAll('.asset-list-row')
    expect(rows.length).toBe(2)
    expect(rows[0].text()).toContain('城商行方案')
    expect(rows[0].text()).toContain('1 MB')

    // 切回网格照常渲染（双向切换）
    await switchDisplayMode(wrapper, 'grid')
    expect(wrapper.find('.assets-gallery').exists()).toBe(true)
    expect(wrapper.findAll('.asset-card').length).toBe(2)
  })

  it('列表行点击进详情（与网格 onCardClick 同一语义）', async () => {
    const wrapper = await createWrapper()
    await switchDisplayMode(wrapper, 'list')
    expect(wrapper.find('.assets-detail').exists()).toBe(false)

    await wrapper.findAll('.asset-list-row')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    expect(wrapper.find('.assets-detail').text()).toContain('城商行方案')
  })

  it('列表滚动懒加载（浏览幻灯片）：到底后走与网格同一分派（ListAssetSlides offset 追加）', async () => {
    wailsMock.ListAssetSlides.mockImplementation((libId, limit, offset) => {
      if (offset === 0) return Promise.resolve({ items: mockSlides, total: 4 })
      return Promise.resolve({ items: [{ ...mockSlides[0], id: 23, slideNum: 3 }], total: 4 })
    })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await switchDisplayMode(wrapper, 'list')
    expect(wrapper.findAll('.asset-list-row').length).toBe(2)

    // 列表容器滚动到底：分派逻辑在宿主 loadMoreInView 单点（组件只上报事件）
    const listEl = wrapper.find('.asset-list')
    Object.defineProperty(listEl.element, 'scrollHeight', { value: 500, configurable: true })
    Object.defineProperty(listEl.element, 'clientHeight', { value: 400, configurable: true })
    listEl.element.scrollTop = 150
    await listEl.trigger('scroll')
    await flushPromises()

    expect(wailsMock.ListAssetSlides).toHaveBeenLastCalledWith('lib-1', 50, 2)
    expect(wrapper.findAll('.asset-list-row').length).toBe(3)
  })

  it('列表滚动懒加载（检索模式）：到底后追加检索下一页', async () => {
    let searchCalls = 0
    wailsMock.SearchAssets.mockImplementation(() => {
      searchCalls++
      if (searchCalls === 1) {
        return Promise.resolve({ items: mockSearchPage.items, total: 3, keyword: '城商行', flattened: false })
      }
      return Promise.resolve({
        items: [{ ...mockSearchPage.items[0], refId: 12, title: '第二页命中' }],
        total: 3,
        keyword: '城商行',
        flattened: false
      })
    })
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await switchDisplayMode(wrapper, 'list')
    expect(wrapper.findAll('.asset-list-row').length).toBe(1)

    const listEl = wrapper.find('.asset-list')
    Object.defineProperty(listEl.element, 'scrollHeight', { value: 500, configurable: true })
    Object.defineProperty(listEl.element, 'clientHeight', { value: 400, configurable: true })
    listEl.element.scrollTop = 150
    await listEl.trigger('scroll')
    await flushPromises()

    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行', {
      flatten: true,
      tagIds: [],
      slideTagOnlyOwn: false,
      ratings: [],
      mtimeFrom: 0,
      mtimeTo: 0,
      sizeFrom: 0,
      sizeTo: 0,
      sortBy: 'relevance',
      sortDir: 'desc',
      folderPath: '',
      limit: 50,
      offset: 2
    })
    expect(wrapper.findAll('.asset-list-row').length).toBe(2)
  })

  // ---- 全屏预览（SRS §2.3 PR2）：AssetPreviewOverlay 宿主接线 ----

  // 切到幻灯片视图并打开第一张卡详情（预览入口的前置语境）
  const openSlideDetail = async (wrapper) => {
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
  }

  it('全屏预览入口：文件视图序列空，详情不提供「全屏预览」按钮（隐藏非禁用）', async () => {
    const wrapper = await createWrapper()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    expect(wrapper.findAll('button').some((b) => b.text().trim() === '全屏预览')).toBe(false)
    wrapper.unmount()
  })

  it('全屏预览：详情入口打开覆盖层（页码/信息行），键盘翻页详情跟随，ESC 关闭详情保留', async () => {
    const wrapper = await createWrapper()
    await openSlideDetail(wrapper)

    const previewBtn = wrapper.findAll('button').find((b) => b.text().trim() === '全屏预览')
    expect(previewBtn).toBeTruthy()
    await previewBtn.trigger('click')
    await flushPromises()

    // 打开点 = 当前详情页在序列中的下标（第 1 页 → index 0）
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(true)
    expect(wrapper.find('.preview-counter').text()).toBe('1 / 2')
    expect(wrapper.find('.preview-title').text()).toBe('封面页')

    // 键盘 → 翻页：详情侧栏同步跟随当前页（D5#6 预览即详情导航）
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }))
    await flushPromises()
    expect(wrapper.find('.preview-counter').text()).toBe('2 / 2')
    expect(wrapper.find('.assets-detail').text()).toContain('背景页')
    expect(wrapper.find('.assets-detail').text()).toContain('第 2 页')

    // ESC 关闭：预览消失，详情保留在最后预览页
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(false)
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    expect(wrapper.find('.assets-detail').text()).toContain('背景页')
    wrapper.unmount()
  })

  it('全屏预览语境变更关闭：视图切换与检索触发 previewIndex 复位', async () => {
    const wrapper = await createWrapper()
    await openSlideDetail(wrapper)
    await clickButton(wrapper, '全屏预览')
    await flushPromises()
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(true)

    // 视图切换（文件 ↔ 幻灯片，D5#3）关闭预览
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'file')
    await group.vm.$emit('change', 'file')
    await flushPromises()
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(false)

    // 重开（幻灯片视图）→ 检索语境变更（D5#2）关闭预览
    await openSlideDetail(wrapper)
    await clickButton(wrapper, '全屏预览')
    await flushPromises()
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(true)

    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(false)
    wrapper.unmount()
  })

  it('全屏预览懒加载边界：末页「→」触发续拉不预翻，写回后序列变长可继续翻', async () => {
    wailsMock.ListAssetSlides.mockImplementation((libId, limit, offset) => {
      if (offset === 0) return Promise.resolve({ items: mockSlides, total: 4 })
      return Promise.resolve({ items: [{ ...mockSlides[0], id: 23, slideNum: 3 }], total: 4 })
    })
    const wrapper = await createWrapper()
    await openSlideDetail(wrapper)
    await clickButton(wrapper, '全屏预览')
    await flushPromises()
    expect(wrapper.find('.preview-counter').text()).toBe('1 / 2')

    // 键盘翻到已加载段末页
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }))
    await flushPromises()
    expect(wrapper.find('.preview-counter').text()).toBe('2 / 2')

    // 末页「→」：canLoadMore 为真按钮保持可用，点击触发续拉（offset=2）不翻页
    expect(wrapper.find('.preview-nav-next').attributes('disabled')).toBeUndefined()
    await wrapper.find('.preview-nav-next').trigger('click')
    await flushPromises()
    expect(wailsMock.ListAssetSlides).toHaveBeenLastCalledWith('lib-1', 50, 2)
    expect(wrapper.find('.preview-counter').text()).toBe('2 / 3')

    // 序列变长后同按钮继续翻页正常
    await wrapper.find('.preview-nav-next').trigger('click')
    await flushPromises()
    expect(wrapper.find('.preview-counter').text()).toBe('3 / 3')
    wrapper.unmount()
  })

  // ---- 选中体系（SRS §2.3 PR3）：selectedKeys + 复选框 + 浮动操作条 ----

  it('网格勾选：复选框写入选中集合并浮出操作条（计数正确），勾选不进详情、主体点击照常进详情', async () => {
    const wrapper = await createWrapper()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)

    // 勾选第 1 张卡：浮动条浮出、计数 1；@click.stop 阻断进详情（D5#7）
    await wrapper.find('.asset-card-check').trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)
    expect(wrapper.find('.selection-count').text()).toBe('已选 1 项')
    expect(wrapper.find('.assets-detail').exists()).toBe(false)

    // 点卡片主体照常进详情，勾选不受影响（勾选与浏览两套交互并行不互斥）
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    expect(wrapper.find('.selection-count').text()).toBe('已选 1 项')

    // 再勾选同卡取消：计数归零浮动条收起
    await wrapper.find('.asset-card-check').trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
  })

  it('网格/列表同键：网格勾选后切列表呈现选中保留（D5#5），列表行取消勾选与网格共享同一键', async () => {
    const wrapper = await createWrapper()
    await wrapper.findAll('.asset-card')[0].find('.asset-card-check').trigger('change')
    await switchDisplayMode(wrapper, 'list')

    // 呈现切换不清空选中：列表行首复选框保持勾选（键 file:11，网格列表同一映射）
    const rowCheck = wrapper.findAll('.asset-list-row')[0].find('.asset-row-check')
    expect(rowCheck.exists()).toBe(true)
    expect(rowCheck.element.checked).toBe(true)
    expect(wrapper.find('.selection-count').text()).toBe('已选 1 项')

    // 列表行取消勾选：与网格同一键，浮动条收起
    await rowCheck.trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
  })

  it('聚合父卡与聚合子行无复选框（D5#1），浏览态独立卡有复选框', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    // 浏览幻灯片清单：全部独立卡有复选框
    expect(wrapper.findAll('.asset-card').length).toBe(2)
    expect(wrapper.findAll('.asset-card-check').length).toBe(2)

    // 检索聚合：聚合卡（hitCount=3）无复选框；展开子行保持展示型也无复选框
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    const aggCard = wrapper.findAll('.asset-card')[0]
    expect(aggCard.classes()).toContain('is-expandable')
    expect(aggCard.find('.asset-card-check').exists()).toBe(false)
    await aggCard.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.agg-child').length).toBe(1)
    expect(wrapper.find('.agg-child .asset-card-check').exists()).toBe(false)
  })

  it('全选/反选/清空：作用域为当前渲染独立卡键集（D5#8），反选为渲染集内取反', async () => {
    const wrapper = await createWrapper()
    // 先勾 1 张卡浮动条才浮出（D2 交互：全选入口在浮动条上，选中数 > 0 才可见）
    await wrapper.findAll('.asset-card')[0].find('.asset-card-check').trigger('change')
    await flushPromises()

    // 全选：2 张独立卡全部选中（计数 = 渲染独立卡数）
    await clickButton(wrapper, '全选')
    await flushPromises()
    expect(wrapper.find('.selection-count').text()).toBe('已选 2 项')
    const allChecks = wrapper.findAll('.asset-card-check')
    expect(allChecks.every((c) => c.element.checked)).toBe(true)

    // 反选：渲染集内全部取反 → 全部取消（浮动条收起）
    await clickButton(wrapper, '反选')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)

    // 重新勾第 1 张后反选：仅第 2 张被选中（渲染集内取反语义）
    await wrapper.findAll('.asset-card')[0].find('.asset-card-check').trigger('change')
    await flushPromises()
    await clickButton(wrapper, '反选')
    await flushPromises()
    const checks = wrapper.findAll('.asset-card-check')
    expect(checks[0].element.checked).toBe(false)
    expect(checks[1].element.checked).toBe(true)
    expect(wrapper.find('.selection-count').text()).toBe('已选 1 项')

    // 清空：浮动条收起
    await clickButton(wrapper, '清空')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
  })

  it('清空点位：检索条件变更与视图切换清空选中（D5#2/#3）', async () => {
    const wrapper = await createWrapper()

    // 勾选后检索：清空
    await wrapper.find('.asset-card-check').trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)

    // 检索态（文件视图 1 个命中）勾选后切幻灯片视图：清空（选中维度不通约）
    const hitCardCheck = wrapper.find('.asset-card-check')
    expect(hitCardCheck.exists()).toBe(true)
    await hitCardCheck.trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
  })

  it('清空点位：平铺开关切换清空选中（D5#2 检索语境变更同批点位）', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('.asset-card-check').trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)

    const flattenSwitch = wrapper.find('.flatten-switch:not(.slide-own-switch)').findComponent(ElSwitchStub)
    await flattenSwitch.vm.$emit('update:modelValue', true)
    await flattenSwitch.vm.$emit('change', true)
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
  })

  it('清空点位：仅自身标签开关切换清空选中（D5#2）', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.findAll('.asset-card')[0].find('.asset-card-check').trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)

    const ownSwitch = wrapper.find('.slide-own-switch').findComponent(ElSwitchStub)
    await ownSwitch.vm.$emit('update:modelValue', true)
    await ownSwitch.vm.$emit('change', true)
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
  })

  it('清空点位：切库与扫描刷新清空选中（D5#2/#4），呈现切换保留不受影响', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '备份库' }
    ])
    const wrapper = await createWrapper()
    await wrapper.find('.asset-card-check').trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)

    // 切库清空
    const select = wrapper.findComponent(ElSelectStub)
    await select.vm.$emit('update:modelValue', 'lib-2')
    await select.vm.$emit('change', 'lib-2')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)

    // 扫描刷新清空（resetAndReloadView 走 D5#4 点位）：新库重新勾选后扫描完成
    const lib2Check = wrapper.find('.asset-card-check')
    expect(lib2Check.exists()).toBe(true)
    await lib2Check.trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)

    await clickButton(wrapper, '扫描')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
  })

  it('清空点位：溯源跳转清空选中（D5#2/#3 同命中——直赋路径不经 applySearch/onViewModeChange）', async () => {
    const wrapper = await createWrapper()
    // 切幻灯片视图勾选一张卡（选中键属幻灯片维度）
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.findAll('.asset-card')[0].find('.asset-card-check').trigger('change')
    await flushPromises()
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)

    // 打开详情后点「查看归属文件」：溯源直赋 viewMode='file' + 退出检索，两个
    // 清空触发点同命中——漏清则幻灯片选中键驻留成幽灵选中（文件视图无可见勾选行）
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    await clickButton(wrapper, '查看归属文件')
    await flushPromises()

    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
    // 溯源语义保持：文件视图 + 归属文件选中
    expect(wrapper.find('.asset-card.is-selected').exists()).toBe(true)
  })

  // ---- 批量打标签（第六批 PR3，PRD D3/D4/D5/D7）：入口反查分派 + completed 刷新链 ----

  it('批量入口文件模式：targets 由选中键反查构建（fileId+label），execute 分派 BatchUpdateAssetFileTags（fileIDs）', async () => {
    wailsMock.BatchUpdateAssetFileTags.mockResolvedValue({ total: 2, succeeded: 2, failed: 0, items: [] })
    const wrapper = await createWrapper()
    const checks = wrapper.findAll('.asset-card-check')
    await checks[0].trigger('change')
    await checks[1].trigger('change')
    await flushPromises()
    expect(wrapper.find('.selection-count').text()).toBe('已选 2 项')

    // 打开弹窗：targets 为文件模式形态（fileId + label = 文件路径，无 slideNum）
    await clickButton(wrapper, '批量打标签')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'AssetBatchTagDialogStub' })
    expect(dialog.props('modelValue')).toBe(true)
    expect(dialog.props('libraryId')).toBe('lib-1')
    expect(dialog.props('targets')).toEqual([
      { fileId: 11, label: '方案A/城商行方案.pptx' },
      { fileId: 12, label: '方案B/损坏文件.pptx' }
    ])

    // execute 分派回调：文件模式调 BatchUpdateAssetFileTags（fileIDs 数组 + op）
    const result = await dialog.props('execute')({ addTags: ['重点'], removeTags: [] })
    expect(wailsMock.BatchUpdateAssetFileTags).toHaveBeenCalledTimes(1)
    expect(wailsMock.BatchUpdateAssetFileTags).toHaveBeenCalledWith(
      'lib-1', [11, 12], { addTags: ['重点'], removeTags: [] }
    )
    expect(wailsMock.BatchUpdateAssetSlideTags).not.toHaveBeenCalled()
    expect(result).toEqual({ total: 2, succeeded: 2, failed: 0, items: [] })
  })

  it('批量入口幻灯片模式：targets 携带 (fileId, slideNum) 与页定位文案，execute 分派 BatchUpdateAssetSlideTags', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    const checks = wrapper.findAll('.asset-card-check')
    expect(checks.length).toBe(2)
    await checks[0].trigger('change')
    await checks[1].trigger('change')
    await flushPromises()

    await clickButton(wrapper, '批量打标签')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'AssetBatchTagDialogStub' })
    expect(dialog.props('targets')).toEqual([
      { fileId: 11, slideNum: 1, label: '方案A/城商行方案.pptx · 第 1 页' },
      { fileId: 11, slideNum: 2, label: '方案A/城商行方案.pptx · 第 2 页' }
    ])
    // execute 分派回调：幻灯片模式调 BatchUpdateAssetSlideTags（(fileId, slideNum)
    // 目标数组，键契约禁 slides.id）
    await dialog.props('execute')({ addTags: [], removeTags: ['重点'] })
    expect(wailsMock.BatchUpdateAssetSlideTags).toHaveBeenCalledTimes(1)
    expect(wailsMock.BatchUpdateAssetSlideTags).toHaveBeenCalledWith(
      'lib-1',
      [{ fileId: 11, slideNum: 1 }, { fileId: 11, slideNum: 2 }],
      { addTags: [], removeTags: ['重点'] }
    )
    expect(wailsMock.BatchUpdateAssetFileTags).not.toHaveBeenCalled()

    // completed 刷新链（浏览幻灯片模式）：重拉幻灯片清单复位首页（ownTags 快照
    // 随批量更新，详情草稿回读数据源）+ 选中保留（D5）
    const slidesCallsBefore = wailsMock.ListAssetSlides.mock.calls.length
    await dialog.vm.$emit('completed')
    await flushPromises()
    expect(wailsMock.ListAssetSlides.mock.calls.length).toBe(slidesCallsBefore + 1)
    expect(wailsMock.ListAssetSlides).toHaveBeenLastCalledWith('lib-1', 50, 0)
    expect(wrapper.find('.selection-count').text()).toBe('已选 2 项')
  })

  it('批量入口陈旧选中：反查 miss 计数提示并跳过，部分失效仍开弹窗；全部失效不开弹窗', async () => {
    const wrapper = await createWrapper()
    const checks = wrapper.findAll('.asset-card-check')
    await checks[0].trigger('change')
    await checks[1].trigger('change')
    await flushPromises()

    // 标签管理变更刷新链（数据变更点位不清选中，对齐 D5）重拉底册：文件 12 已不在
    const manager = wrapper.findComponent({ name: 'AssetTagManagerDialogStub' })
    wailsMock.ListAssetFiles.mockResolvedValue([mockFiles[0]])
    await manager.vm.$emit('changed')
    await flushPromises()
    // 选中保留但渲染集内仅文件 11 有卡（文件 12 键已陈旧）
    expect(wrapper.find('.selection-count').text()).toBe('已选 2 项')

    await clickButton(wrapper, '批量打标签')
    await flushPromises()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith('1 项已失效跳过')
    const dialog = wrapper.findComponent({ name: 'AssetBatchTagDialogStub' })
    expect(dialog.props('modelValue')).toBe(true)
    expect(dialog.props('targets')).toEqual([{ fileId: 11, label: '方案A/城商行方案.pptx' }])

    // 全部失效：关闭弹窗后底册刷新为空 → 不开弹窗仅提示
    await dialog.vm.$emit('update:modelValue', false)
    await flushPromises()
    wailsMock.ListAssetFiles.mockResolvedValue([])
    await manager.vm.$emit('changed')
    await flushPromises()
    await clickButton(wrapper, '批量打标签')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenLastCalledWith('2 项已失效跳过')
    expect(dialog.props('modelValue')).toBe(false)
  })

  it('批量 completed 刷新链（文件浏览模式）：底册重拉 + 树组件 reload，选中保留（D5 数据变更点位不清空）', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('.asset-card-check').trigger('change')
    await flushPromises()
    await clickButton(wrapper, '批量打标签')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'AssetBatchTagDialogStub' })

    const reloadSpy = vi.spyOn(
      wrapper.findAllComponents({ name: 'AssetTagTreeSelectStub' })[0].vm, 'reload'
    )
    const filesCallsBefore = wailsMock.ListAssetFiles.mock.calls.length
    await dialog.vm.$emit('completed')
    await flushPromises()

    expect(wailsMock.ListAssetFiles.mock.calls.length).toBe(filesCallsBefore + 1)
    expect(reloadSpy).toHaveBeenCalledTimes(1)
    // 选中保留（D5）：批量完成不清选中，用户可继续对这批操作
    expect(wrapper.find('.selection-count').text()).toBe('已选 1 项')
  })

  it('批量 completed 检索语境：重跑检索 + 清聚合展开缓存（对齐 onSaveSlideMeta 双代次守卫先例）', async () => {
    // 检索 mock 改为幻灯片双条目（1 独立命中可勾选 + 1 聚合卡）——聚合卡仅在
    // 幻灯片视图渲染（检索结果按视图类型过滤），文件视图勾选文件命中卡的重搜
    // 路径已由文件模式 completed 用例同链路覆盖
    wailsMock.SearchAssets.mockResolvedValue({
      items: [
        {
          refType: 'slide', refId: 21, fileId: 11, filePath: '方案A/城商行方案.pptx',
          title: '封面页', slideNum: 1, snippet: '…城商行零售转型…', score: -1.5,
          hitCount: 1, tags: [mockTags[0]], ownTags: [], note: '', rating: 0
        },
        {
          refType: 'slide', refId: 22, fileId: 11, filePath: '方案A/城商行方案.pptx',
          title: '背景页', slideNum: 2, snippet: '…城商行获客…', score: -2.1,
          hitCount: 3, tags: [mockTags[0]], ownTags: [], note: '', rating: 0
        }
      ],
      total: 2,
      keyword: '城商行',
      flattened: false
    })
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    // 幻灯片检索态：独立命中卡可勾选，聚合卡展开制造缓存残留
    await wrapper.find('.asset-card-check').trigger('change')
    const aggCard = wrapper.findAll('.asset-card').find((c) => c.classes().includes('is-expandable'))
    await aggCard.trigger('click')
    await flushPromises()
    expect(wrapper.find('.agg-children').exists()).toBe(true)

    await clickButton(wrapper, '批量打标签')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'AssetBatchTagDialogStub' })
    const searchCallsBefore = wailsMock.SearchAssets.mock.calls.length
    await dialog.vm.$emit('completed')
    await flushPromises()

    // 检索态重搜（命中条目标签对象同步）+ 页自有标签变化使命中口径改变，展开缓存
    // 失效强制重展开按当前口径重拉
    expect(wailsMock.SearchAssets.mock.calls.length).toBe(searchCallsBefore + 1)
    expect(wrapper.find('.agg-children').exists()).toBe(false)
    // 选中保留（D5）
    expect(wrapper.find('.selection-count').text()).toBe('已选 1 项')
  })

  it('批量 completed 过滤浏览语境：清单复位首页后自动续拉到位，目标文件页重新可见（客户端过滤不再收敛 0 张）', async () => {
    // 初始首页即目标文件页（进入过滤无入口续拉）；completed 后模拟全局清单复位为
    // 其他文件段——无兜底时客户端 fileFilterPath 过滤收敛 0 张（真机冒烟 bug 形态：
    // 批量成功提示后当前页全部卡片消失，watch(fileFilterPath) 因过滤值未变不触发）
    const slideOfOther = { ...mockSlides[0], id: 31, fileId: 12, filePath: '方案B/损坏文件.pptx', slideNum: 1, title: '损坏页一' }
    wailsMock.ListAssetSlides
      .mockResolvedValueOnce({ items: mockSlides, total: 3 })
      .mockImplementation((libId, limit, offset) => {
        if (offset === 0) return Promise.resolve({ items: [slideOfOther], total: 3 })
        return Promise.resolve({ items: mockSlides, total: 3 })
      })
    const wrapper = await createWrapper()

    // 双击文件卡进入过滤浏览，全选当前过滤页打开批量弹窗
    await wrapper.findAll('.asset-card')[0].trigger('dblclick')
    await flushPromises()
    expect(wrapper.find('.file-filter-banner').exists()).toBe(true)
    const checks = wrapper.findAll('.asset-card-check')
    expect(checks.length).toBe(2)
    await checks[0].trigger('change')
    await checks[1].trigger('change')
    await flushPromises()
    await clickButton(wrapper, '批量打标签')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'AssetBatchTagDialogStub' })

    // 批量完成：清单复位首页为其他文件段 → 自动续拉（offset 1）目标页重新可见
    await dialog.vm.$emit('completed')
    await flushPromises()
    expect(wailsMock.ListAssetSlides).toHaveBeenLastCalledWith('lib-1', 50, 1)
    const cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(2)
    expect(cards.every((c) => c.text().includes('城商行方案.pptx'))).toBe(true)
    expect(wrapper.find('.file-filter-banner').text()).toContain('2 页')
    // 选中保留（PRD D5 数据变更点位不清空，过滤语境同样成立）
    expect(wrapper.find('.selection-count').text()).toBe('已选 2 项')
  })

  // ---- 批量评分（第八批，SRS §2.6 P0，PRD D4/D5）：入口反查分派 + completed 刷新链 ----

  it('批量评分入口文件模式：targets 复用反查链路，execute 分派 BatchSetAssetRating（slideNum 0 = 文件条目）', async () => {
    wailsMock.BatchSetAssetRating.mockResolvedValue({
      total: 2, succeeded: 2, failed: 0,
      items: [
        { fileId: 11, status: 'ok', label: '方案A/城商行方案.pptx' },
        { fileId: 12, status: 'ok', label: '方案B/损坏文件.pptx' }
      ]
    })
    const wrapper = await createWrapper()
    const checks = wrapper.findAll('.asset-card-check')
    await checks[0].trigger('change')
    await checks[1].trigger('change')
    await flushPromises()

    // 打开弹窗：targets 与批量打标签同款形态（fileId + label = 文件路径）
    await clickButton(wrapper, '评分')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'AssetBatchRatingDialogStub' })
    expect(dialog.props('modelValue')).toBe(true)
    expect(dialog.props('targets')).toEqual([
      { fileId: 11, label: '方案A/城商行方案.pptx' },
      { fileId: 12, label: '方案B/损坏文件.pptx' }
    ])

    // execute 分派回调：调 BatchSetAssetRating（targets 的 slideNum 缺省归一 0 =
    // 文件条目契约 + 星级值），打标签两 API 不触碰
    const result = await dialog.props('execute')(4)
    expect(wailsMock.BatchSetAssetRating).toHaveBeenCalledTimes(1)
    expect(wailsMock.BatchSetAssetRating).toHaveBeenCalledWith(
      'lib-1',
      [{ fileId: 11, slideNum: 0 }, { fileId: 12, slideNum: 0 }],
      4
    )
    expect(wailsMock.BatchUpdateAssetFileTags).not.toHaveBeenCalled()
    expect(wailsMock.BatchUpdateAssetSlideTags).not.toHaveBeenCalled()
    expect(result.total).toBe(2)
  })

  it('批量评分入口幻灯片模式：targets 携带 (fileId, slideNum)，execute 分派保留页序', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    const checks = wrapper.findAll('.asset-card-check')
    await checks[0].trigger('change')
    await checks[1].trigger('change')
    await flushPromises()

    await clickButton(wrapper, '评分')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'AssetBatchRatingDialogStub' })
    expect(dialog.props('targets')).toEqual([
      { fileId: 11, slideNum: 1, label: '方案A/城商行方案.pptx · 第 1 页' },
      { fileId: 11, slideNum: 2, label: '方案A/城商行方案.pptx · 第 2 页' }
    ])
    // execute 分派：页条目 slideNum 原样透传（键契约禁 slides.id，页级评
    // slide_meta.rating）
    await dialog.props('execute')(5)
    expect(wailsMock.BatchSetAssetRating).toHaveBeenCalledWith(
      'lib-1',
      [{ fileId: 11, slideNum: 1 }, { fileId: 11, slideNum: 2 }],
      5
    )
  })

  it('批量评分 completed 刷新链：底册重拉 + 标签树不 reload（评分不涉标签变更）+ 选中保留', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('.asset-card-check').trigger('change')
    await flushPromises()
    await clickButton(wrapper, '评分')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'AssetBatchRatingDialogStub' })

    const reloadSpy = vi.spyOn(
      wrapper.findAllComponents({ name: 'AssetTagTreeSelectStub' })[0].vm, 'reload'
    )
    const filesCallsBefore = wailsMock.ListAssetFiles.mock.calls.length
    await dialog.vm.$emit('completed')
    await flushPromises()

    expect(wailsMock.ListAssetFiles.mock.calls.length).toBe(filesCallsBefore + 1)
    // 评分不涉标签变更：跳过标签树 reload（与批量打标签刷新链的差异点）
    expect(reloadSpy).not.toHaveBeenCalled()
    // 选中保留（D5 数据变更点位不清空）：用户可继续对这批操作（如再打标签）
    expect(wrapper.find('.selection-count').text()).toBe('已选 1 项')
  })

  // ---- 失败缩略图自动补（SRS §2.3 PR3，PRD D4）----

  it('失败缩略图自动补：done 后失败集自动重试恰好一轮（fileId 从底册反查），第二轮 done 不再触发', async () => {
    const wrapper = await createWrapper()
    expect(wailsMock.RetryAssetThumb).not.toHaveBeenCalled()

    // 批次 progress 收集 1 个失败文件 → done：自动重试恰一轮（底册反查 fileId=12）
    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-1', done: 1, total: 2, currentFile: '方案B/损坏文件.pptx', failed: 1 })
    eventHandlers['assets-thumbs:done']({ libraryId: 'lib-1', total: 2, generated: 1, skipped: 0, failed: 1, canceled: false })
    await flushPromises()
    expect(wailsMock.RetryAssetThumb).toHaveBeenCalledTimes(1)
    expect(wailsMock.RetryAssetThumb).toHaveBeenCalledWith('lib-1', 12, '方案B/损坏文件.pptx')

    // 第二轮批次仍失败（progress 重新收集）→ done：不再自动触发（恰好一轮，D4）
    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-1', done: 1, total: 2, currentFile: '方案B/损坏文件.pptx', failed: 1 })
    eventHandlers['assets-thumbs:done']({ libraryId: 'lib-1', total: 2, generated: 0, skipped: 0, failed: 1, canceled: false })
    await flushPromises()
    expect(wailsMock.RetryAssetThumb).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('失败缩略图自动补：底册反查 miss 的失败路径跳过（无 fileId 不重试）', async () => {
    const wrapper = await createWrapper()
    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-1', done: 1, total: 2, currentFile: '不存在/未知.pptx', failed: 1 })
    eventHandlers['assets-thumbs:done']({ libraryId: 'lib-1', total: 2, failed: 1 })
    await flushPromises()
    // 底册（mockFiles 两条路径）无该文件：跳过不发起重试
    expect(wailsMock.RetryAssetThumb).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('失败缩略图自动补：切库复位一轮资格，新库批次可再触发（D4 新库新批次语义）', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { ...mockLibrary, id: 'lib-2', name: '备份库' }
    ])
    const wrapper = await createWrapper()

    // lib-1 批次失败 → done 自动重试一轮
    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-1', done: 1, total: 2, currentFile: '方案B/损坏文件.pptx', failed: 1 })
    eventHandlers['assets-thumbs:done']({ libraryId: 'lib-1', total: 2, failed: 1 })
    await flushPromises()
    expect(wailsMock.RetryAssetThumb).toHaveBeenCalledTimes(1)

    // 切 lib-2：资格复位；lib-2 批次失败 → done 再次自动重试（以 lib-2 底册反查）
    const select = wrapper.findComponent(ElSelectStub)
    await select.vm.$emit('update:modelValue', 'lib-2')
    await select.vm.$emit('change', 'lib-2')
    await flushPromises()

    eventHandlers['assets-thumbs:progress']({ libraryId: 'lib-2', done: 1, total: 2, currentFile: '方案B/损坏文件.pptx', failed: 1 })
    eventHandlers['assets-thumbs:done']({ libraryId: 'lib-2', total: 2, failed: 1 })
    await flushPromises()
    expect(wailsMock.RetryAssetThumb).toHaveBeenCalledTimes(2)
    expect(wailsMock.RetryAssetThumb).toHaveBeenLastCalledWith('lib-2', 12, '方案B/损坏文件.pptx')
    wrapper.unmount()
  })

  // ---- D6 真机增量：固定卡宽网格 + 缩放滑块 + 双击交互 + 文件内过滤 + 详情文本下沉 ----

  it('固定卡宽：画廊容器注入 --asset-card-w CSS 变量（默认 280），滑块变更后变量跟随，列表视图滑块不渲染', async () => {
    const wrapper = await createWrapper()
    const gallery = wrapper.find('.assets-gallery')
    expect(gallery.attributes('style')).toContain('--asset-card-w: 280px')

    // 网格视图滑块存在，边界对齐 PRD D4（220-420 步长 20）
    const slider = wrapper.find('.card-size-slider')
    expect(slider.exists()).toBe(true)
    const sliderComp = slider.findComponent(ElSliderStub)
    expect(sliderComp.props('min')).toBe(220)
    expect(sliderComp.props('max')).toBe(420)
    expect(sliderComp.props('step')).toBe(20)
    // 滑块变更后 CSS 变量跟随（详情栏开合只减列数不改卡宽的宽度来源）
    await sliderComp.vm.$emit('update:modelValue', 360)
    await flushPromises()
    expect(wrapper.find('.assets-gallery').attributes('style')).toContain('--asset-card-w: 360px')

    // 列表视图滑块不渲染（列表行固定尺寸无缩放对象，防占位跳动）
    await switchDisplayMode(wrapper, 'list')
    expect(wrapper.find('.card-size-slider').exists()).toBe(false)
  })

  it('双击幻灯片卡进全屏预览：以该卡定位序列下标（页码指示匹配）', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()

    // 双击第 2 张卡（背景页）：浏览器双击序 click→click→dblclick，首击已装配详情，
    // dblclick 以当前详情定位序列下标
    const card = wrapper.findAll('.asset-card')[1]
    await card.trigger('click')
    await card.trigger('dblclick')
    await flushPromises()

    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(true)
    expect(wrapper.find('.preview-counter').text()).toBe('2 / 2')
    expect(wrapper.find('.preview-title').text()).toBe('背景页')
    wrapper.unmount()
  })

  it('双击文件卡进入文件内过滤浏览：切幻灯片视图 + 过滤条出现 + 仅显示该文件页，清除过滤恢复全量', async () => {
    const slideOfOther = { ...mockSlides[0], id: 31, fileId: 12, filePath: '方案B/损坏文件.pptx', slideNum: 1, title: '损坏页一' }
    wailsMock.ListAssetSlides.mockResolvedValue({ items: [...mockSlides, slideOfOther], total: 3 })
    const wrapper = await createWrapper()

    // 文件视图双击第一个文件卡（城商行方案）
    await wrapper.findAll('.asset-card')[0].trigger('dblclick')
    await flushPromises()

    // 切到幻灯片视图（卡片为幻灯片形态）且过滤生效
    const cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(2)
    expect(cards[0].text()).toContain('幻灯片')
    expect(cards.every((c) => c.text().includes('城商行方案.pptx'))).toBe(true)
    const banner = wrapper.find('.file-filter-banner')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('城商行方案.pptx')
    expect(banner.text()).toContain('2 页')

    // 清除过滤：过滤条消失，恢复全量（3 页含其他文件）
    await clickButton(wrapper, '清除过滤')
    await flushPromises()
    expect(wrapper.find('.file-filter-banner').exists()).toBe(false)
    expect(wrapper.findAll('.asset-card').length).toBe(3)
  })

  it('过滤空自动续拉：目标文件页不在已加载段时逐页拉取直到出现（或全局拉完）', async () => {
    const page2 = [{ ...mockSlides[0], id: 31, fileId: 12, filePath: '方案B/损坏文件.pptx', slideNum: 1, title: '损坏页一' }]
    wailsMock.ListAssetSlides.mockImplementation((libId, limit, offset) => {
      if (offset === 0) return Promise.resolve({ items: mockSlides, total: 3 })
      return Promise.resolve({ items: page2, total: 3 })
    })
    const wrapper = await createWrapper()

    // 文件视图双击「损坏文件」卡（其页在第 2 页分页流中）
    await wrapper.findAll('.asset-card')[1].trigger('dblclick')
    await flushPromises()

    // 自动续拉到位（首页 offset=0 + 续拉 offset=2），过滤结果出现且计数正确
    expect(wailsMock.ListAssetSlides).toHaveBeenLastCalledWith('lib-1', 50, 2)
    const cards = wrapper.findAll('.asset-card')
    expect(cards.length).toBe(1)
    expect(cards[0].text()).toContain('损坏文件')
    expect(wrapper.find('.file-filter-banner').text()).toContain('1 页')
  })

  it('检索态双击文件卡：先退检索再进入过滤浏览（过滤与检索互斥，退检索语境清空条件）', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    expect(wrapper.find('.assets-stats-line').text()).toContain('命中 2 条')

    // 检索态（文件视图命中 1 卡）双击
    await wrapper.findAll('.asset-card')[0].trigger('dblclick')
    await flushPromises()

    // 退检索：统计行命中口径消失、输入框清空；进入过滤态
    expect(wrapper.find('.assets-stats-line').text()).not.toContain('命中')
    expect(wrapper.find('input').element.value).toBe('')
    expect(wrapper.find('.file-filter-banner').exists()).toBe(true)
  })

  it('幻灯片详情解析文本下沉：页文本/演讲者备注/命中摘要区位于操作按钮区之后（标注编辑优先）', async () => {
    const wrapper = await createWrapper()
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()

    // DOM 序断言（jsdom 无布局，样式断言不可行）：含解析文本（detail-pre）的
    // section 全部在最后一个操作区（detail-actions）之后。浏览态无命中摘要
    // （hit 为空 snippet 恒空，该区 v-if 不渲染），pre 区为页文本 + 演讲者备注
    const detail = wrapper.find('.assets-detail')
    const blocks = detail.findAll('.detail-section, .detail-actions')
    const preIndexes = []
    const actionIndexes = []
    blocks.forEach((b, i) => {
      if (b.element.querySelector('.detail-pre')) preIndexes.push(i)
      if (b.classes().includes('detail-actions')) actionIndexes.push(i)
    })
    expect(preIndexes.length).toBe(2)
    expect(detail.text()).toContain('页文本')
    expect(detail.text()).toContain('演讲者备注')
    expect(actionIndexes.length).toBe(2) // 保存/重置区 + 跳转按钮区
    expect(Math.min(...preIndexes)).toBeGreaterThan(Math.max(...actionIndexes))
  })

  it('过滤语境清空点位：手动切视图清过滤（过滤条消失且恢复全量），聚合父卡双击不动作', async () => {
    const slideOfOther = { ...mockSlides[0], id: 31, fileId: 12, filePath: '方案B/损坏文件.pptx', slideNum: 1, title: '损坏页一' }
    wailsMock.ListAssetSlides.mockResolvedValue({ items: [...mockSlides, slideOfOther], total: 3 })
    const wrapper = await createWrapper()
    await wrapper.findAll('.asset-card')[0].trigger('dblclick')
    await flushPromises()
    expect(wrapper.find('.file-filter-banner').exists()).toBe(true)

    // 手动切视图（文件 → 幻灯片 → 文件）：离开过滤语境
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'file')
    await group.vm.$emit('change', 'file')
    await flushPromises()
    expect(wrapper.find('.file-filter-banner').exists()).toBe(false)
    expect(wrapper.find('.assets-stats-line').text()).not.toContain('筛选')

    // 聚合父卡（检索态 hitCount>1）双击不动作：不切视图不进预览（单击语义是展开/收起）
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    const aggCard = wrapper.findAll('.asset-card')[0]
    expect(aggCard.classes()).toContain('is-expandable')
    await aggCard.trigger('dblclick')
    await flushPromises()
    expect(wrapper.find('.asset-preview-overlay').exists()).toBe(false)
    expect(wrapper.find('.file-filter-banner').exists()).toBe(false)
    // 双击后聚合展开态未被两次 click 语义误触发（dblclick 处理器 early return，
    // trigger('dblclick') 不派发 click，展开态保持收起）
    expect(wrapper.find('.agg-child').exists()).toBe(false)
  })

  // ---- facet 多维筛选（第七批 SRS §2.5 PR2）：双语境接入 / 链路切换 / 空态引导 /
  // 语境清空点位。档位→参数换算纯函数（注入 now）在 AssetFilterPopover 独立 spec
  // 覆盖，此处只断言宿主接线 ----

  it('浏览语境 facet 激活：空关键词+facet 条件走 SearchAssets 分页链路（facets-only，D3）', async () => {
    const wrapper = await createWrapper()
    wailsMock.SearchAssets.mockClear()
    await emitFacetChange(wrapper, {
      ratings: [4],
      mtimePreset: 'any',
      mtimeRange: null,
      sizePreset: '1to10'
    })
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(1)
    const [libId, keyword, opts] = wailsMock.SearchAssets.mock.calls[0]
    expect(libId).toBe('lib-1')
    expect(keyword).toBe('')
    // facet 五字段由组件换算单点产出（大小档 1-10MB → 字节闭区间），其余检索参数原样
    expect(opts.ratings).toEqual([4])
    expect(opts.sizeFrom).toBe(1024 * 1024)
    expect(opts.sizeTo).toBe(10 * 1024 * 1024 - 1)
    expect(opts.mtimeFrom).toBe(0)
    expect(opts.mtimeTo).toBe(0)
    expect(opts.flatten).toBe(true)
    expect(opts.tagIds).toEqual([])
    expect(opts.limit).toBe(50)
    expect(opts.offset).toBe(0)
    // 检索链路接管渲染：统计行命中口径出现，文件视图下只显文件条目（聚合幻灯片卡被类型过滤）
    expect(wrapper.find('.assets-stats-line').text()).toContain('命中 2 条')
    expect(wrapper.findAll('.asset-card').length).toBe(1)
  })

  it('相对档「今天」在检索调用时换算为本地零点起的绝对纳秒区间（现算不冻结）', async () => {
    const wrapper = await createWrapper()
    await emitFacetChange(wrapper, { ratings: [], mtimePreset: 'today', mtimeRange: null, sizePreset: 'any' })
    const opts = wailsMock.SearchAssets.mock.calls[0][2]
    // 零点边界当日稳定（断言与调用时刻跨毫秒无竞争），上界为调用时刻
    expect(opts.mtimeFrom).toBe(new Date(new Date().setHours(0, 0, 0, 0)).getTime() * 1e6)
    expect(opts.mtimeTo).toBeGreaterThan(opts.mtimeFrom)
    expect(opts.mtimeTo).toBeLessThanOrEqual(Date.now() * 1e6)
  })

  it('检索语境 facet 组合：关键词与 facet 条件并入同一次 SearchAssets', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    wailsMock.SearchAssets.mockClear()
    await emitFacetChange(wrapper, { ratings: [0, 5], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    const [libId, keyword, opts] = wailsMock.SearchAssets.mock.calls[0]
    expect(libId).toBe('lib-1')
    expect(keyword).toBe('城商行')
    expect(opts.ratings).toEqual([0, 5])
    expect(opts.tagIds).toEqual([])
  })

  it('浏览语境 facet 清除：回底册链路（ListAssetFiles 重新加载，D3）', async () => {
    const wrapper = await createWrapper()
    await emitFacetChange(wrapper, { ratings: [4], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    expect(wrapper.find('.assets-stats-line').text()).toContain('命中')
    const callsBefore = wailsMock.ListAssetFiles.mock.calls.length
    await emitFacetChange(wrapper, { ratings: [], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    expect(wailsMock.ListAssetFiles.mock.calls.length).toBeGreaterThan(callsBefore)
    // 底册重新成为数据源（浏览卡片全量回归，命中口径消失）
    expect(wrapper.findAll('.asset-card').length).toBe(2)
    expect(wrapper.find('.assets-stats-line').text()).not.toContain('命中')
  })

  it('facet 激活零命中：空态文案 + 「清除全部筛选」一键复位 facet 与标签（D6）', async () => {
    wailsMock.SearchAssets.mockResolvedValue({ items: [], total: 0, keyword: '', flattened: false })
    const wrapper = await createWrapper()
    // 先落一个标签筛选（一并验证「清除全部筛选」的标签复位面）
    const tagSelect = wrapper.findComponent({ name: 'AssetTagTreeSelectStub' })
    await tagSelect.vm.$emit('update:modelValue', [1])
    await tagSelect.vm.$emit('change', [1])
    await flushPromises()
    await emitFacetChange(wrapper, { ratings: [3], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    const empty = wrapper.find('.el-empty')
    expect(empty.text()).toContain('当前筛选条件下无命中，可调整筛选条件。')
    const clearBtn = wrapper.findAll('button').find((b) => b.text().trim() === '清除全部筛选')
    expect(clearBtn).toBeTruthy()
    await clearBtn.trigger('click')
    await flushPromises()
    // facet 快照与标签选择一并复位；关键词本就为空 → 全清回底册（空态消失）
    const facetStub = wrapper.findComponent({ name: 'AssetFilterPopoverStub' })
    expect(facetStub.props('modelValue')).toEqual({ ratings: [], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    expect(wrapper.findComponent({ name: 'AssetTagTreeSelectStub' }).props('modelValue')).toEqual([])
    expect(wrapper.find('.el-empty').exists()).toBe(false)
    expect(wrapper.findAll('.asset-card').length).toBe(2)
  })

  it('关键词仍在时「清除全部筛选」：facet+标签复位后退回关键词检索', async () => {
    wailsMock.SearchAssets.mockResolvedValue({ items: [], total: 0, keyword: '', flattened: false })
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    wailsMock.SearchAssets.mockClear()
    await emitFacetChange(wrapper, { ratings: [3], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    await clickButton(wrapper, '清除全部筛选')
    await flushPromises()
    // 两次调用 = facet 变更重查 + 清除全部筛选后退回关键词检索（关键词保留语义）
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(2)
    const [, keyword, opts] = wailsMock.SearchAssets.mock.calls[1]
    expect(keyword).toBe('城商行')
    expect(opts.ratings).toEqual([])
  })

  it('facet 变更为语境变更点：关闭详情侧栏（复位面对齐检索条件变更同类点位）', async () => {
    const wrapper = await createWrapper()
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    await emitFacetChange(wrapper, { ratings: [4], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    expect(wrapper.find('.assets-detail').exists()).toBe(false)
  })

  it('双击文件卡进入过滤浏览：连 facet 一并复位（过滤浏览与检索/facet 语境互斥）', async () => {
    const wrapper = await createWrapper()
    await emitFacetChange(wrapper, { ratings: [4], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    wailsMock.ListAssetSlides.mockClear()
    await wrapper.findAll('.asset-card')[0].trigger('dblclick')
    await flushPromises()
    // facet 已清 → searchMode 失效 → 幻灯片视图浏览链路接管（loadSlidesPage 首页）
    expect(wailsMock.ListAssetSlides).toHaveBeenCalledWith('lib-1', 50, 0)
    const facetStub = wrapper.findComponent({ name: 'AssetFilterPopoverStub' })
    expect(facetStub.props('modelValue').ratings).toEqual([])
    expect(wrapper.find('.file-filter-banner').exists()).toBe(true)
  })

  it('溯源跳转清 facet：直赋文件视图路径连 facet 一并复位（§8.2 矩阵登记点位）', async () => {
    // 检索页固定单页独立幻灯片命中（hitCount=1，点击直达详情不经聚合展开）
    wailsMock.SearchAssets.mockResolvedValue({
      items: [
        {
          refType: 'slide',
          refId: 21,
          fileId: 11,
          filePath: '方案A/城商行方案.pptx',
          title: '封面页',
          slideNum: 1,
          snippet: '',
          score: -1.0,
          hitCount: 1,
          tags: [mockTags[0]],
          ownTags: [],
          note: '',
          rating: 0
        }
      ],
      total: 1,
      keyword: '城商行',
      flattened: false
    })
    const wrapper = await createWrapper()
    // 切幻灯片视图让命中幻灯片条目可见，再叠关键词与 facet（组合检索语境）
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await emitFacetChange(wrapper, { ratings: [4], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    await clickButton(wrapper, '查看归属文件')
    await flushPromises()
    // facet 复位 + 落到文件底册详情（溯源语义完整，画廊回底册文件清单）
    const facetStub = wrapper.findComponent({ name: 'AssetFilterPopoverStub' })
    expect(facetStub.props('modelValue')).toEqual({ ratings: [], mtimePreset: 'any', mtimeRange: null, sizePreset: 'any' })
    expect(wrapper.find('.assets-detail').text()).toContain('城商行方案')
    expect(wrapper.findAll('.asset-card').length).toBe(2)
  })

  // ---- 保存检索条件（第七批 SRS §2.5 PR3）：保存快照 D7 全维 / 命名校验与重名
  // 拒绝透传 / 调取回填与异库按名重映射降级 / 本库异库分组管理 / 覆盖与删除链 ----

  it('「保存搜索」入口可用性：非检索语境禁用，检索语境可用（浏览语境 facet 激活同样可保存）', async () => {
    const wrapper = await createWrapper()
    const saveBtn = wrapper.findAll('button').find((b) => b.text().trim() === '保存搜索')
    expect(saveBtn.attributes('disabled')).toBeDefined()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    expect(saveBtn.attributes('disabled')).toBeUndefined()
  })

  it('保存弹窗摘要预览：展示快照全维人读形态（facet 时间维为解析后的绝对区间，与还原口径一致）', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await emitFacetChange(wrapper, {
      ratings: [4],
      mtimePreset: 'custom',
      mtimeRange: [new Date(2026, 9, 1), new Date(2026, 9, 15)],
      sizePreset: '1to10'
    })
    await clickButton(wrapper, '保存搜索')
    const summary = wrapper.find('.save-search-summary')
    expect(summary.text()).toContain('城商行')
    expect(summary.text()).toContain('4 星')
    expect(summary.text()).toContain('2026-10-01 ~ 2026-10-15')
    expect(summary.text()).toContain('1-10MB')
    expect(summary.text()).toContain('开启（命中逐页平铺）')
    expect(summary.text()).toContain('仅幻灯片自身标签')
  })

  it('保存快照 D7 全维：SaveAssetSavedSearch 收到全字段（tagNames 冗余 + facet 区间原始值 + 两开关 + libraryId），成功后关弹窗并重拉清单', async () => {
    const wrapper = await createWrapper()
    // 组合语境：关键词 + 标签 + facet 三维（自定义区间）
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    const tagSelect = wrapper.findComponent({ name: 'AssetTagTreeSelectStub' })
    await tagSelect.vm.$emit('update:modelValue', [1])
    await tagSelect.vm.$emit('change', [1])
    await flushPromises()
    await emitFacetChange(wrapper, {
      ratings: [4],
      mtimePreset: 'custom',
      mtimeRange: [new Date(2026, 9, 1), new Date(2026, 9, 15)],
      sizePreset: '1to10'
    })
    // 切幻灯片视图开「仅幻灯片自身标签」+ 关平铺（快照全维覆盖）
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
    const ownSwitch = wrapper.find('.slide-own-switch').findComponent(ElSwitchStub)
    await ownSwitch.vm.$emit('update:modelValue', true)
    await ownSwitch.vm.$emit('change', true)
    await flushPromises()
    const flattenSwitch = wrapper.find('.flatten-switch:not(.slide-own-switch)').findComponent(ElSwitchStub)
    await flattenSwitch.vm.$emit('update:modelValue', false)
    await flattenSwitch.vm.$emit('change', false)
    await flushPromises()

    await clickButton(wrapper, '保存搜索')
    await wrapper.find('.stub-dialog input').setValue('组合条件')
    await clickButton(wrapper, '保存')
    await flushPromises()
    expect(wailsMock.SaveAssetSavedSearch).toHaveBeenCalledTimes(1)
    expect(wailsMock.SaveAssetSavedSearch).toHaveBeenCalledWith({
      id: 0,
      name: '组合条件',
      libraryId: 'lib-1',
      keyword: '城商行',
      tagIds: [1],
      tagNames: ['重点'],
      ratings: [4],
      mtimeFrom: new Date(2026, 9, 1).getTime() * 1e6,
      mtimeTo: (new Date(2026, 9, 16).getTime() - 1) * 1e6,
      sizeFrom: 1024 * 1024,
      sizeTo: 10 * 1024 * 1024 - 1,
      flatten: false,
      slideTagOnlyOwn: true,
      sortBy: 'relevance',
      sortDir: 'desc'
    })
    expect(wrapper.find('.stub-dialog').exists()).toBe(false)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('检索条件已保存')
    // 挂载 @show 重拉一次 + 保存成功重拉一次
    expect(wailsMock.ListAssetSavedSearches).toHaveBeenCalledTimes(2)
  })

  it('保存名称必填校验：空名不调 Save 仅警告', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await clickButton(wrapper, '保存搜索')
    await clickButton(wrapper, '保存')
    await flushPromises()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入保存条件名称')
    expect(wailsMock.SaveAssetSavedSearch).not.toHaveBeenCalled()
  })

  it('重名保存被后端拒绝：错误 toast 透传且弹窗保持打开（用户可改名重试）', async () => {
    wailsMock.SaveAssetSavedSearch.mockRejectedValue({
      message: '已存在同名保存条件「组合条件」，请换一个名称或直接覆盖原条件'
    })
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await clickButton(wrapper, '保存搜索')
    await wrapper.find('.stub-dialog input').setValue('组合条件')
    await clickButton(wrapper, '保存')
    await flushPromises()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith(
      '保存检索条件失败: 已存在同名保存条件「组合条件」，请换一个名称或直接覆盖原条件'
    )
    expect(wrapper.find('.stub-dialog').exists()).toBe(true)
  })

  it('已存搜索弹层按库分组：本库条件可覆盖（非检索语境禁用），其他库条目标注库名且覆盖恒禁用（D4 全局存储带库归属）', async () => {
    wailsMock.ListAssetLibraries.mockResolvedValue([
      mockLibrary,
      { id: 'lib-2', name: '市场素材库', rootPath: 'D:/market', createdAt: '', updatedAt: '' }
    ])
    wailsMock.ListAssetSavedSearches.mockResolvedValue([
      savedSearchEntry({ id: 1, name: '本库条件A' }),
      savedSearchEntry({ id: 2, name: '本库条件B' }),
      savedSearchEntry({ id: 3, name: '他库条件', libraryId: 'lib-2', tagIds: [9], tagNames: [] })
    ])
    const wrapper = await createWrapper()
    const groups = wrapper.findAll('.saved-search-group')
    expect(groups.length).toBe(2)
    expect(groups[0].text()).toContain('本库条件')
    expect(groups[0].text()).toContain('本库条件A')
    expect(groups[0].text()).toContain('本库条件B')
    expect(groups[1].text()).toContain('其他库条件')
    expect(groups[1].text()).toContain('他库条件')
    // 异库条目标注归属库名（库注册表命中）
    expect(groups[1].find('.saved-search-lib-mark').text()).toBe('市场素材库')
    // 非检索语境：本库覆盖禁用（无可快照条件）
    const ownOverwrite = groups[0].findAll('button').find((b) => b.text().trim() === '覆盖')
    expect(ownOverwrite.attributes('disabled')).toBeDefined()
    // 异库覆盖恒禁用（覆盖会按当前库重写条目 libraryId 使条目静默搬家）
    const otherOverwrite = groups[1].findAll('button').find((b) => b.text().trim() === '覆盖')
    expect(otherOverwrite.attributes('disabled')).toBeDefined()
  })

  it('调取本库条目：全维条件回填并触发检索链（keyword/tagIds/两开关/facet 快照反推，D7 完整还原）', async () => {
    wailsMock.ListAssetSavedSearches.mockResolvedValue([
      savedSearchEntry({
        id: 7,
        name: '组合条件',
        keyword: '获客',
        tagIds: [2],
        tagNames: ['方案'],
        ratings: [4],
        mtimeFrom: 0,
        mtimeTo: 0,
        sizeFrom: 50 * 1024 * 1024,
        sizeTo: 0,
        flatten: false,
        slideTagOnlyOwn: true,
        sortBy: 'rating',
        sortDir: 'asc'
      })
    ])
    const wrapper = await createWrapper()
    wailsMock.SearchAssets.mockClear()
    await clickButton(wrapper, '组合条件')
    await flushPromises()
    expect(wailsMock.SearchAssets).toHaveBeenCalledTimes(1)
    const [libId, keyword, opts] = wailsMock.SearchAssets.mock.calls[0]
    expect(libId).toBe('lib-1')
    expect(keyword).toBe('获客')
    expect(opts.tagIds).toEqual([2])
    expect(opts.slideTagOnlyOwn).toBe(true)
    expect(opts.flatten).toBe(false)
    expect(opts.ratings).toEqual([4])
    // 排序快照还原（SRS §2.5.3，D3）：调取后检索按快照档位/方向发起（round-trip）
    expect(opts.sortBy).toBe('rating')
    expect(opts.sortDir).toBe('asc')
    // gt50 档快照区间经反推回 UI 档位后检索参数与保存值一致（round-trip）
    expect(opts.sizeFrom).toBe(50 * 1024 * 1024)
    expect(opts.sizeTo).toBe(0)
    expect(opts.mtimeFrom).toBe(0)
    expect(opts.mtimeTo).toBe(0)
    // 关键词输入框回填（后续手动改词在此基础上）
    expect(wrapper.find('input').element.value).toBe('获客')
  })

  it('调取异库条目：tagIds 失效按 tagNames 重映射（miss 丢弃计数提示，关键词/facet 照回填，D4 降级）', async () => {
    wailsMock.ListAssetSavedSearches.mockResolvedValue([
      savedSearchEntry({
        id: 8,
        name: '他库条件',
        libraryId: 'lib-other',
        keyword: '方案',
        tagIds: [77, 88],
        tagNames: ['重点', '不存在的标签'],
        ratings: [5]
      })
    ])
    const wrapper = await createWrapper()
    wailsMock.SearchAssets.mockClear()
    const applyBtn = wrapper.findAll('.saved-search-apply').find((b) => b.text().includes('他库条件'))
    await applyBtn.trigger('click')
    await flushPromises()
    // 「重点」在当前库标签表命中 id=1，「不存在的标签」miss 丢弃并计数提示；
    // tagIds [77,88] 属保存时所在库不直接复用
    const [, keyword, opts] = wailsMock.SearchAssets.mock.calls[0]
    expect(keyword).toBe('方案')
    expect(opts.tagIds).toEqual([1])
    expect(opts.ratings).toEqual([5])
    expect(opts.slideTagOnlyOwn).toBe(true)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith('1 个标签在当前库不存在，已忽略')
  })

  it('调取为语境变更点：关闭详情侧栏并清空选中（§8.2 矩阵「保存条件调取」点位，复位面由 applySearch 统一执行）', async () => {
    wailsMock.ListAssetSavedSearches.mockResolvedValue([
      savedSearchEntry({ id: 3, name: '条件C', keyword: '城商行', ratings: [], sizeFrom: 0, sizeTo: 0 })
    ])
    const wrapper = await createWrapper()
    // 建立详情与选中态
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    await wrapper.find('.asset-card-check').trigger('change')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(true)
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(true)
    await clickButton(wrapper, '条件C')
    await flushPromises()
    expect(wrapper.find('.assets-detail').exists()).toBe(false)
    expect(wrapper.find('.asset-selection-bar').exists()).toBe(false)
  })

  it('覆盖更新：以当前检索条件 Save 同 id（名称沿用原名），成功后重拉清单', async () => {
    wailsMock.ListAssetSavedSearches.mockResolvedValue([savedSearchEntry({ id: 5, name: '旧条件', keyword: '旧词' })])
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('新关键词')
    await clickButton(wrapper, '搜索')
    await flushPromises()
    await clickButton(wrapper, '覆盖')
    await flushPromises()
    expect(wailsMock.SaveAssetSavedSearch).toHaveBeenCalledTimes(1)
    const arg = wailsMock.SaveAssetSavedSearch.mock.calls[0][0]
    expect(arg.id).toBe(5)
    expect(arg.name).toBe('旧条件')
    expect(arg.keyword).toBe('新关键词')
    expect(arg.libraryId).toBe('lib-1')
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已覆盖更新「旧条件」')
    // 挂载 @show 重拉一次 + 覆盖成功重拉一次
    expect(wailsMock.ListAssetSavedSearches).toHaveBeenCalledTimes(2)
  })

  it('删除保存条件：确认弹窗后 Delete 并重拉清单', async () => {
    wailsMock.ListAssetSavedSearches.mockResolvedValue([savedSearchEntry({ id: 9, name: '待删除' })])
    const wrapper = await createWrapper()
    await clickButton(wrapper, '删除')
    await flushPromises()
    const { ElMessageBox } = await import('element-plus')
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      '确定删除保存的检索条件「待删除」？',
      '删除保存条件',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
    expect(wailsMock.DeleteAssetSavedSearch).toHaveBeenCalledWith(9)
    expect(ElMessageBox.confirm.mock.calls.length).toBeGreaterThan(0)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已删除')
    // 挂载 @show 重拉一次 + 删除成功重拉一次
    expect(wailsMock.ListAssetSavedSearches).toHaveBeenCalledTimes(2)
  })

  // ---- 文件丢失与重链（第九批 PR2，SRS §2.6.5）----

  it('lost 呈现：统计行丢失计数、卡片丢失标签、丢失文件详情提供「重新定位」入口', async () => {
    wailsMock.GetAssetLibraryStats.mockResolvedValue({ ...mockStats, lostCount: 1 })
    const wrapper = await createWrapper()
    // 统计行
    expect(wrapper.find('.assets-stats-line').text()).toContain('丢失 1')
    // 丢失文件卡片带丢失标签（warning 语义色）
    const lostFile = { ...mockFiles[0], id: 13, path: '方案C/已移走.pptx', status: 'lost', title: '已移走' }
    wailsMock.ListAssetFiles.mockResolvedValue([...mockFiles, lostFile])
    const wrapper2 = await createWrapper()
    const cards = wrapper2.findAll('.asset-card')
    const lostCard = cards.find((c) => c.text().includes('已移走'))
    expect(lostCard).toBeTruthy()
    expect(lostCard.findAll('.el-tag').map((t) => t.text())).toContain('丢失')
    // 详情侧边栏：丢失文件提供「重新定位」，正常文件不提供
    await lostCard.trigger('click')
    await flushPromises()
    expect(wrapper2.find('.assets-detail').exists()).toBe(true)
    expect(wrapper2.findAll('.detail-actions button').some((b) => b.text().includes('重新定位'))).toBe(true)
    const okCard = cards.find((c) => c.text().includes('城商行方案'))
    await okCard.trigger('click')
    await flushPromises()
    expect(wrapper2.findAll('.detail-actions button').some((b) => b.text().includes('重新定位'))).toBe(false)
  })

  it('手动重链主链路：选文件 → 哈希匹配直复活 toast → 刷新（Relocate 参数与刷新链断言）', async () => {
    const lostFile = { ...mockFiles[0], id: 13, path: '方案C/已移走.pptx', status: 'lost', title: '已移走' }
    wailsMock.ListAssetFiles.mockResolvedValue([...mockFiles, lostFile])
    const wrapper = await createWrapper()
    const cards = wrapper.findAll('.asset-card')
    await cards.find((c) => c.text().includes('已移走')).trigger('click')
    await flushPromises()
    // 文件对话框选中库内新位置（重链走 OpenFileDialog 文件选择）
    wailsMock.OpenFileDialog.mockResolvedValue('D:/assets/新目录/已移走.pptx')
    const relocateBtn = wrapper.findAll('.detail-actions button').find((b) => b.text().includes('重新定位'))
    await relocateBtn.trigger('click')
    await flushPromises()
    expect(wailsMock.RelocateAssetFile).toHaveBeenCalledWith('lib-1', 13, 'D:/assets/新目录/已移走.pptx', false)
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已重链：文件内容与原记录一致，元数据直接恢复')
    // 终态刷新：底册重拉（首次挂载 + 刷新 ≥ 2 次）+ 统计重拉
    expect(wailsMock.ListAssetFiles.mock.calls.length).toBeGreaterThanOrEqual(2)
    expect(wailsMock.GetAssetLibraryStats.mock.calls.length).toBeGreaterThanOrEqual(2)
    // 用户取消对话框（空串）不触发重链
    wailsMock.RelocateAssetFile.mockClear()
    wailsMock.OpenFileDialog.mockResolvedValue('')
    await relocateBtn.trigger('click')
    await flushPromises()
    expect(wailsMock.RelocateAssetFile).not.toHaveBeenCalled()
  })

  it('哈希不等确认链路：E_ASSETS_RELINK_MISMATCH → ElMessageBox 确认 → force=true 重调', async () => {
    const lostFile = { ...mockFiles[0], id: 13, path: '方案C/已移走.pptx', status: 'lost', title: '已移走' }
    wailsMock.ListAssetFiles.mockResolvedValue([...mockFiles, lostFile])
    wailsMock.RelocateAssetFile
      .mockRejectedValueOnce({ code: 'E_ASSETS_RELINK_MISMATCH', message: '目标文件内容与原记录不一致' })
      .mockResolvedValueOnce({ fileId: 13, oldPath: '', newPath: '', hashMatched: false, skippedHashCheck: false, reparsed: true })
    const wrapper = await createWrapper()
    const cards = wrapper.findAll('.asset-card')
    await cards.find((c) => c.text().includes('已移走')).trigger('click')
    await flushPromises()
    wailsMock.OpenFileDialog.mockResolvedValue('D:/assets/另一份.pptx')
    const relocateBtn = wrapper.findAll('.detail-actions button').find((b) => b.text().includes('重新定位'))
    await relocateBtn.trigger('click')
    await flushPromises()
    const { ElMessageBox, ElMessage } = await import('element-plus')
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('目标文件内容与原记录不一致'),
      '强制重链确认',
      expect.objectContaining({ type: 'warning', confirmButtonText: '强制重链' })
    )
    // 确认后 force=true 重调 + 强制重链分型 toast
    expect(wailsMock.RelocateAssetFile).toHaveBeenLastCalledWith('lib-1', 13, 'D:/assets/另一份.pptx', true)
    expect(ElMessage.success).toHaveBeenCalledWith('已强制重链并按新文件重新解析（标签与标注保留）')
  })

  it('重链分型先判 reparsed：processing 残留行（hashMatched=true 且 reparsed=true）走重解析文案，不误报元数据直接恢复', async () => {
    const lostFile = { ...mockFiles[0], id: 13, path: '方案C/已移走.pptx', status: 'lost', title: '已移走' }
    wailsMock.ListAssetFiles.mockResolvedValue([...mockFiles, lostFile])
    // 后端 processing 残留行重链形态：HashMatched=true 是哈希事实陈述，
    // Reparsed=true 才是必经自愈动作——toast 分型须先判 reparsed
    wailsMock.RelocateAssetFile.mockResolvedValue({ fileId: 13, oldPath: '', newPath: '', hashMatched: true, skippedHashCheck: false, reparsed: true })
    const wrapper = await createWrapper()
    const cards = wrapper.findAll('.asset-card')
    await cards.find((c) => c.text().includes('已移走')).trigger('click')
    await flushPromises()
    wailsMock.OpenFileDialog.mockResolvedValue('D:/assets/新目录/已移走.pptx')
    const relocateBtn = wrapper.findAll('.detail-actions button').find((b) => b.text().includes('重新定位'))
    await relocateBtn.trigger('click')
    await flushPromises()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已重链并重新解析（内容与原记录一致，补齐解析产物）')
    expect(ElMessage.success).not.toHaveBeenCalledWith('已重链：文件内容与原记录一致，元数据直接恢复')
  })

  it('重链分型：空哈希跳过校验（skippedHashCheck=true）走未做内容校验文案', async () => {
    const lostFile = { ...mockFiles[0], id: 13, path: '方案C/已移走.pptx', status: 'lost', title: '已移走' }
    wailsMock.ListAssetFiles.mockResolvedValue([...mockFiles, lostFile])
    wailsMock.RelocateAssetFile.mockResolvedValue({ fileId: 13, oldPath: '', newPath: '', hashMatched: false, skippedHashCheck: true, reparsed: true })
    const wrapper = await createWrapper()
    const cards = wrapper.findAll('.asset-card')
    await cards.find((c) => c.text().includes('已移走')).trigger('click')
    await flushPromises()
    wailsMock.OpenFileDialog.mockResolvedValue('D:/assets/新目录/已移走.pptx')
    const relocateBtn = wrapper.findAll('.detail-actions button').find((b) => b.text().includes('重新定位'))
    await relocateBtn.trigger('click')
    await flushPromises()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已重链并重新解析（原记录无哈希，未做内容校验）')
  })

  it('维护菜单：清理丢失确认弹窗列出记录数并 Clear + 刷新；前缀替换打开弹窗（零丢失时清理项禁用）', async () => {
    wailsMock.GetAssetLibraryStats.mockResolvedValue({ ...mockStats, lostCount: 2 })
    wailsMock.ClearLostAssetFiles.mockResolvedValue(2)
    const wrapper = await createWrapper()
    // el-dropdown 桩常显 dropdown slot：菜单项直接 DOM 可见，点击触发命令分派
    const clearItem = wrapper.findAll('.el-dropdown-item').find((i) => i.text().includes('清理丢失记录'))
    expect(clearItem).toBeTruthy()
    expect(clearItem.classes()).not.toContain('is-disabled')
    await clearItem.trigger('click')
    await flushPromises()
    const { ElMessageBox, ElMessage } = await import('element-plus')
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('将删除 2 条丢失记录'),
      '清理丢失记录',
      expect.objectContaining({ type: 'warning' })
    )
    expect(wailsMock.ClearLostAssetFiles).toHaveBeenCalledWith('lib-1')
    expect(ElMessage.success).toHaveBeenCalledWith('已清理 2 条丢失记录')
    // 前缀替换：菜单项打开弹窗
    const replaceItem = wrapper.findAll('.el-dropdown-item').find((i) => i.text().includes('路径前缀替换'))
    await replaceItem.trigger('click')
    await flushPromises()
    expect(wrapper.find('.asset-prefix-replace-dialog-stub').exists()).toBe(true)
    // 零丢失：清理项禁用
    wailsMock.GetAssetLibraryStats.mockResolvedValue({ ...mockStats, lostCount: 0 })
    const wrapper2 = await createWrapper()
    const clearItem2 = wrapper2.findAll('.el-dropdown-item').find((i) => i.text().includes('清理丢失记录'))
    expect(clearItem2.classes()).toContain('is-disabled')
  })

  // ---- 素材购物栏（PR3：三处加号入口 + 批量加页 + 溯源定位 + 合并导出）----

  // 购物栏条目工厂（形状对齐 model.AssetCartItem；stale 为 List 探测态；filePath
  // 为溯源定位消费的库相对路径，与后端 srcPath 快照来源一致）
  const cartItemOf = (overrides = {}) => ({
    libraryId: 'lib-1',
    fileId: 11,
    slideNum: 1,
    filePath: '方案A/城商行方案.pptx',
    srcPath: 'D:/assets/方案A/城商行方案.pptx',
    fileName: '城商行方案.pptx',
    slideTitle: '封面页',
    libraryName: '产品素材库',
    addedAt: '2026-10-19T10:00:00+08:00',
    ...overrides
  })

  const switchToSlideView = async (wrapper) => {
    const group = wrapper.findComponent(ElRadioGroupStub)
    await group.vm.$emit('update:modelValue', 'slide')
    await group.vm.$emit('change', 'slide')
    await flushPromises()
  }

  it('挂载即拉购物栏：ListAssetCartItems 调用 + 工具行入口徽标计数（跨库全局清单切库不重载）', async () => {
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf(), cartItemOf({ fileId: 22, slideNum: 3 })])
    const wrapper = await createWrapper()
    expect(wailsMock.ListAssetCartItems).toHaveBeenCalledTimes(1)
    const badge = wrapper.find('.cart-entry-badge .el-badge__content')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('2')

    // 切库不重载（购物栏为跨库全局清单）：统计与底册重拉而购物栏不重复拉取
    const callsBefore = wailsMock.ListAssetCartItems.mock.calls.length
    await wrapper.find('.el-select').setValue('lib-1')
    await flushPromises()
    expect(wailsMock.ListAssetCartItems.mock.calls.length).toBe(callsBefore)
  })

  it('页卡 hover 加号（PRD 入口一）：未收集卡点击入栏（条目快照形状对齐后端契约），已收集卡幂等提示不重复调 Add', async () => {
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf()]) // 封面页已在栏
    const wrapper = await createWrapper()
    await switchToSlideView(wrapper)
    const cartBtns = wrapper.findAll('.asset-card-cart')
    expect(cartBtns.length).toBe(2)

    // 已收集态：封面页卡（slideNum 1）常显对勾，点击幂等不调 Add
    expect(cartBtns[0].classes()).toContain('is-in-cart')
    expect(cartBtns[0].text()).toBe('✓')
    await cartBtns[0].trigger('click')
    await flushPromises()
    expect(wailsMock.AddAssetCartItems).not.toHaveBeenCalled()

    // 未收集卡（背景页 slideNum 2）：点击入栏，快照字段取当下底册数据
    expect(cartBtns[1].text()).toBe('＋')
    await cartBtns[1].trigger('click')
    await flushPromises()
    expect(wailsMock.AddAssetCartItems).toHaveBeenCalledWith([
      {
        libraryId: 'lib-1',
        fileId: 11,
        slideNum: 2,
        srcPath: 'D:/assets/方案A/城商行方案.pptx',
        fileName: '城商行方案.pptx',
        slideTitle: '背景页',
        libraryName: '产品素材库'
      }
    ])
  })

  it('批量加页（PRD 入口四·选中浮动条）：幻灯片视图勾选多页一次入栏（追加顺序 = 渲染集顺序）', async () => {
    const wrapper = await createWrapper()
    await switchToSlideView(wrapper)
    const checks = wrapper.findAll('.asset-card-check')
    await checks[0].setValue(true)
    await checks[1].setValue(true)
    await flushPromises()
    const cartBtn = wrapper.findAll('button').find((b) => b.text().trim() === '加入购物栏')
    await cartBtn.trigger('click')
    await flushPromises()
    expect(wailsMock.AddAssetCartItems).toHaveBeenCalledTimes(1)
    const items = wailsMock.AddAssetCartItems.mock.calls[0][0]
    expect(items.map((i) => i.slideNum)).toEqual([1, 2])
    expect(items[0].srcPath).toBe('D:/assets/方案A/城商行方案.pptx')
  })

  it('详情侧栏加号（PRD 入口二）：幻灯片详情展示加入按钮与已收集态', async () => {
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf()])
    const wrapper = await createWrapper()
    await switchToSlideView(wrapper)
    await wrapper.findAll('.asset-card')[0].trigger('click')
    await flushPromises()
    const detailActions = wrapper.findAll('.detail-actions .el-button')
    const cartBtn = detailActions.find((b) => b.text().includes('已在购物栏'))
    expect(cartBtn).toBeTruthy()
    await cartBtn.trigger('click')
    await flushPromises()
    expect(wailsMock.AddAssetCartItems).not.toHaveBeenCalled()
  })

  it('拖拽排序回写：抽屉上报按键列表经 ReorderAssetCart 持久化并重拉清单', async () => {
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf(), cartItemOf({ fileId: 22, slideNum: 3 })])
    const wrapper = await createWrapper()
    const drawer = wrapper.findComponent({ name: 'AssetCartDrawerStub' })
    await drawer.vm.$emit('reorder', ['lib-1|22|3', 'lib-1|11|1'])
    await flushPromises()
    expect(wailsMock.ReorderAssetCart).toHaveBeenCalledWith(['lib-1|22|3', 'lib-1|11|1'])
    // 回写后重拉（props 快照与后端一致）
    expect(wailsMock.ListAssetCartItems.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it('溯源定位：抽屉条目点击关闭抽屉、切幻灯片视图过滤浏览并选中目标页进详情', async () => {
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf({ fileId: 11, slideNum: 2, slideTitle: '背景页', fileName: '城商行方案.pptx' })])
    const wrapper = await createWrapper()
    const drawer = wrapper.findComponent({ name: 'AssetCartDrawerStub' })
    await drawer.vm.$emit('trace', cartItemOf({ fileId: 11, slideNum: 2 }))
    await flushPromises()
    expect(drawer.props('visible')).toBe(false)
    // 幻灯片视图 + 该文件页已加载（过滤语境详情命中目标页）
    expect(wailsMock.ListAssetSlides).toHaveBeenCalled()
    const detail = wrapper.find('.assets-detail')
    expect(detail.exists()).toBe(true)
    expect(detail.text()).toContain('第 2 页')
  })

  it('合并导出：SaveFileDialog 预填默认名 → MergeAssetCartToPptx → 成功确认弹窗提供可选清空动作（保留为默认）', async () => {
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf(), cartItemOf({ fileId: 22, slideNum: 3 })])
    wailsMock.SaveFileDialog.mockResolvedValue('D:/out/合并导出-20261019-2330.pptx')
    const wrapper = await createWrapper()
    const drawer = wrapper.findComponent({ name: 'AssetCartDrawerStub' })
    await drawer.vm.$emit('export')
    await flushPromises()
    expect(wailsMock.SaveFileDialog).toHaveBeenCalledWith(
      expect.stringMatching(/^合并导出-\d{8}-\d{4}\.pptx$/),
      [{ DisplayName: 'PowerPoint 演示文稿', Pattern: '*.pptx' }]
    )
    expect(wailsMock.MergeAssetCartToPptx).toHaveBeenCalledWith('D:/out/合并导出-20261019-2330.pptx')
    // 成功确认弹窗（可选清空动作）：确认 → 清空 + 重拉
    const { ElMessageBox } = await import('element-plus')
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('已导出 2 页'),
      '合并导出完成',
      expect.objectContaining({ confirmButtonText: '清空购物栏' })
    )
    expect(wailsMock.ClearAssetCart).toHaveBeenCalledTimes(1)
    expect(wailsMock.ListAssetCartItems.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it('合并导出成功确认取消 = 保留购物栏（不清空）', async () => {
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockRejectedValueOnce(new Error('cancel'))
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf()])
    wailsMock.SaveFileDialog.mockResolvedValue('D:/out/x.pptx')
    const wrapper = await createWrapper()
    const drawer = wrapper.findComponent({ name: 'AssetCartDrawerStub' })
    await drawer.vm.$emit('export')
    await flushPromises()
    expect(wailsMock.MergeAssetCartToPptx).toHaveBeenCalled()
    expect(wailsMock.ClearAssetCart).not.toHaveBeenCalled()
    expect(wailsMock.ListAssetCartItems.mock.calls.length).toBe(1)
  })

  it('合并导出跳过项与尺寸混排结果报告（SkippedItems 计数 + SldSzMixed warning）', async () => {
    const { ElMessage } = await import('element-plus')
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf()])
    wailsMock.SaveFileDialog.mockResolvedValue('D:/out/x.pptx')
    wailsMock.MergeAssetCartToPptx.mockResolvedValue({
      outputPath: 'D:/out/x.pptx',
      pages: [],
      pageCount: 1,
      skippedItems: [{ srcPath: 'D:/gone.pptx', pageNum: 9, reason: '页码超出范围' }],
      sldSzMixed: true
    })
    const wrapper = await createWrapper()
    const drawer = wrapper.findComponent({ name: 'AssetCartDrawerStub' })
    await drawer.vm.$emit('export')
    await flushPromises()
    const { ElMessageBox } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith(expect.stringContaining('尺寸不一致'))
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('跳过 1 项'),
      '合并导出完成',
      expect.anything()
    )
  })

  it('导出事件仅复位进度（终态 toast 由 Promise 分支统一执行防双弹），取消转发后端', async () => {
    wailsMock.ListAssetCartItems.mockResolvedValue([cartItemOf()])
    wailsMock.SaveFileDialog.mockResolvedValue('D:/out/x.pptx')
    wailsMock.MergeAssetCartToPptx.mockImplementation(() => new Promise((resolve) => setTimeout(() => resolve({ outputPath: 'D:/out/x.pptx', pages: [], pageCount: 1, skippedItems: [], sldSzMixed: false }), 20)))
    const wrapper = await createWrapper()
    const drawer = wrapper.findComponent({ name: 'AssetCartDrawerStub' })
    drawer.vm.$emit('export')
    await flushPromises()
    // 导出运行态透传抽屉（进度事件驱动）
    await eventHandlers['assets-cart:export-progress']({ outPath: 'D:/out/x.pptx', done: 1, total: 1 })
    await flushPromises()
    expect(drawer.props('exporting')).toBe(true)
    expect(drawer.props('exportProgress')).toEqual({ outPath: 'D:/out/x.pptx', done: 1, total: 1 })
    // done 事件仅复位进度，不弹提示（对齐扫描契约）
    const { ElMessage } = await import('element-plus')
    const successCalls = ElMessage.success.mock.calls.length
    await eventHandlers['assets-cart:export-done']({ outPath: 'D:/out/x.pptx', result: {} })
    await flushPromises()
    expect(drawer.props('exportProgress')).toBe(null)
    expect(ElMessage.success.mock.calls.length).toBe(successCalls)
    // Promise 落定后终态提示由本分支统一弹出（清空确认）
    await new Promise((r) => setTimeout(r, 40))
    await flushPromises()
    expect(ElMessage.success.mock.calls.length).toBeGreaterThan(successCalls)

    // 取消导出：转发后端（无在途 false → warning 引导）
    const drawer2 = wrapper.findComponent({ name: 'AssetCartDrawerStub' })
    await drawer2.vm.$emit('cancel-export')
    await flushPromises()
    expect(wailsMock.CancelAssetCartExport).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})
