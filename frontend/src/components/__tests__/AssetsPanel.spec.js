import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetsPanel from '../AssetsPanel.vue'

// ---- 测试数据（形状对齐 model/assets.go 经 Wails 绑定的 JSON 字段）----
const mockLibrary = {
  id: 'lib-1',
  name: '产品素材库',
  rootPath: 'D:/assets',
  createdAt: '2026-10-13 10:00:00',
  updatedAt: '2026-10-13 10:00:00'
}

const mockStats = { libraryId: 'lib-1', fileCount: 2, slideCount: 3, failedCount: 0, encryptedCount: 0 }

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
    tags: [mockTags[0]]
  },
  {
    id: 22,
    fileId: 11,
    filePath: '方案A/城商行方案.pptx',
    slideNum: 2,
    title: '背景页',
    text: '获客成本持续上升',
    speakerNotes: '',
    tags: [mockTags[0]]
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
      tags: [mockTags[0]]
    }
  ],
  total: 2,
  keyword: '城商行',
  flattened: false
}

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
    ListAssetSlides: vi.fn(),
    UpdateAssetFileMeta: vi.fn(),
    SearchAssets: vi.fn(),
    OpenDirectoryDialog: vi.fn()
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
  props: ['modelValue', 'size'],
  emits: ['update:modelValue', 'change']
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
const stubs = {
  'el-select': ElSelectStub,
  'el-option': { template: '<option :value="value"><slot /></option>', props: ['label', 'value'] },
  'el-radio-group': ElRadioGroupStub,
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
  'el-progress': { template: '<div class="el-progress" :data-percentage="percentage" />', props: ['percentage', 'strokeWidth', 'showText'] },
  'el-empty': { template: '<div class="el-empty">{{ description }}<slot /></div>', props: ['description'] },
  'el-tooltip': { template: '<span><slot /></span>', props: ['content', 'placement'] },
  'el-icon': { template: '<i><slot /></i>', props: ['size'] },
  'el-alert': { template: '<div class="el-alert"><slot /></div>', props: ['type', 'title', 'closable'] },
  'el-form': { template: '<form><slot /></form>', props: ['labelWidth'] },
  'el-form-item': { template: '<div><slot /></div>', props: ['label'] }
}

const createWrapper = async () => {
  const wrapper = mount(AssetsPanel, { global: { stubs } })
  await flushPromises()
  return wrapper
}

const clickButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().trim() === text)?.trigger('click')

describe('AssetsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // 重置默认实现（个别用例的 mockReturnValue/mockRejectedValue 会永久覆盖）
    wailsMock.ListAssetLibraries.mockResolvedValue([mockLibrary])
    wailsMock.GetAssetLibraryStats.mockResolvedValue(mockStats)
    wailsMock.ListAssetFiles.mockResolvedValue(mockFiles)
    wailsMock.ListAssetSlides.mockResolvedValue({ items: mockSlides, total: mockSlides.length })
    wailsMock.SearchAssets.mockResolvedValue(mockSearchPage)
    wailsMock.StartAssetScan.mockResolvedValue({ total: 2, new: 1, updated: 1, unchanged: 0, failed: 0, encrypted: 0, denied: 0, removed: 0 })
    wailsMock.RebuildAssetLibrary.mockResolvedValue({ total: 2, new: 2, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, removed: 0 })
    wailsMock.CancelAssetScan.mockResolvedValue(true)
    wailsMock.CreateAssetLibrary.mockResolvedValue({ ...mockLibrary, id: 'lib-2', name: '新库' })
    wailsMock.DeleteAssetLibrary.mockResolvedValue(null)
    wailsMock.UpdateAssetFileMeta.mockResolvedValue(null)
    // 目录对话框默认空串 = 用户取消（安全 no-op）
    wailsMock.OpenDirectoryDialog.mockResolvedValue('')
    for (const key of Object.keys(eventHandlers)) delete eventHandlers[key]
    offFns = []
  })

  it('挂载注册 assets-scan 三个事件监听（progress/done/error），卸载时闭包精准注销且未走 EventsOff', async () => {
    const { EventsOff } = await import('../../../wailsjs/runtime/runtime')
    const wrapper = await createWrapper()
    expect(Object.keys(eventHandlers)).toEqual(
      expect.arrayContaining(['assets-scan:progress', 'assets-scan:done', 'assets-scan:error'])
    )
    expect(offFns.length).toBe(3)

    wrapper.unmount()
    expect(offFns.filter((off) => off.mock.calls.length > 0).length).toBe(3)
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
      flatten: false,
      tagIds: [],
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

  it('平铺开关：检索态下切换后以 flatten=true 重跑检索', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('input').setValue('城商行')
    await clickButton(wrapper, '搜索')
    await flushPromises()

    const switchStub = wrapper.findComponent(ElSwitchStub)
    await switchStub.vm.$emit('update:modelValue', true)
    await switchStub.vm.$emit('change', true)
    await flushPromises()

    expect(wailsMock.SearchAssets).toHaveBeenLastCalledWith('lib-1', '城商行', {
      flatten: true,
      tagIds: [],
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
    // 选标签（详情标签选择器：emit 数组替换语义）
    const tagsSelect = wrapper.findComponent('.detail-tags-select')
    await tagsSelect.vm.$emit('update:modelValue', ['重点', '新标签'])
    await flushPromises()

    await clickButton(wrapper, '保存')
    await flushPromises()

    expect(wailsMock.UpdateAssetFileMeta).toHaveBeenCalledWith('lib-1', 11, {
      rating: 3,
      note: '季度汇报用',
      tags: ['重点', '新标签']
    })
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('已保存')
    // 保存后重拉文件底册（取回新建标签的完整对象）
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledTimes(2)
  })

  it('幻灯片详情：继承标签只读展示带「继承」标记，溯源按钮跳文件视图并选中归属文件', async () => {
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
    expect(detail.text()).toContain('继承')
    // 幻灯片详情无编辑表单（星级/备注/标签编辑为文件级）
    expect(wrapper.find('.el-rate').exists()).toBe(false)

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

    expect(wailsMock.DeleteAssetLibrary).toHaveBeenCalledWith('lib-1')
    // 删除后自动切换到剩余库并加载其数据
    expect(wailsMock.GetAssetLibraryStats).toHaveBeenCalledWith('lib-2')
    expect(wailsMock.ListAssetFiles).toHaveBeenCalledWith('lib-2')
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
      limit: 200,
      offset: 0
    })
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
})
