import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AssetListView from '../AssetListView.vue'

// ---- 测试数据（形状对齐 AssetsPanel displayCards 同构卡片模型与 model/assets.go）----
const entry11 = {
  id: 11,
  path: '方案A/城商行方案.pptx',
  size: 1024 * 1024,
  modTime: '2026-10-12 09:00:00',
  status: 'ok',
  title: '城商行方案',
  rating: 3,
  tags: [{ id: 1, name: '重点', color: '#2563eb', parentId: 0 }]
}

const slideEntry21 = {
  id: 21,
  fileId: 11,
  filePath: '方案A/城商行方案.pptx',
  slideNum: 1,
  title: '封面页',
  ownTags: [{ id: 9, name: '封面页标签', color: '#f59e0b', parentId: 0 }],
  rating: 2
}

const browseFileCard = {
  key: 'file-11',
  kind: 'file',
  id: 11,
  title: '城商行方案',
  filePath: '方案A/城商行方案.pptx',
  status: 'ok',
  tags: entry11.tags,
  hitCount: 1,
  slideNum: 0,
  snippet: '',
  entry: entry11
}

const browseSlideCard = {
  key: 'slide-21',
  kind: 'slide',
  id: 21,
  fileId: 11,
  title: '封面页',
  filePath: '方案A/城商行方案.pptx',
  slideNum: 1,
  tags: [entry11.tags[0]],
  hitCount: 1,
  status: '',
  snippet: '',
  slide: slideEntry21
}

// 检索命中：幻灯片聚合条目（hitCount>1，底册可 join 命中）+ 文件条目（无底册记录 miss）
const searchSlideAggCard = {
  key: 'hit-slide-22',
  kind: 'slide',
  id: 22,
  fileId: 11,
  title: '背景页',
  filePath: '方案A/城商行方案.pptx',
  slideNum: 2,
  snippet: '…城商行获客…',
  hitCount: 3,
  status: '',
  tags: [entry11.tags[0]],
  hit: { refType: 'slide', refId: 22, fileId: 11, hitCount: 3, ownTags: [], rating: 0 }
}

const searchFileMissCard = {
  key: 'hit-file-99',
  kind: 'file',
  id: 99,
  fileId: 99,
  title: '底册缺失文件',
  filePath: '方案C/已删除.pptx',
  slideNum: 0,
  snippet: '',
  hitCount: 1,
  status: '',
  tags: [],
  hit: { refType: 'file', refId: 99, fileId: 99, hitCount: 1, rating: 0, ownTags: [] }
}

// AssetThumb 已有独立 spec 覆盖加载行为，此处 stub 掉排除干扰（仅断言 props 透传）
const AssetThumbStub = {
  template: '<div class="asset-thumb-stub" :data-file-id="fileId" :data-slide-num="slideNum" :data-tier="tier"></div>',
  props: ['libraryId', 'fileId', 'slideNum', 'tier', 'epoch', 'hideOnMiss', 'alt']
}

const stubs = {
  AssetThumb: AssetThumbStub,
  'el-tag': { template: '<span class="el-tag" :data-type="type"><slot /></span>', props: ['type', 'size'] }
}

const mountList = (props = {}) =>
  mount(AssetListView, {
    props: {
      cards: [],
      fileEntries: [],
      libraryId: 'lib-1',
      thumbEpoch: 0,
      searchMode: false,
      ...props
    },
    global: { stubs }
  })

const rowCells = (wrapper, rowIndex) =>
  wrapper.findAll('tbody tr')[rowIndex].findAll('td').map((td) => td.text().trim())

describe('AssetListView', () => {
  it('文件条目列集（浏览）：复选框/缩略图前两列 + 名称/路径/大小/修改时间/星级/标签/状态，底册条目直读属性', () => {
    const wrapper = mountList({ cards: [browseFileCard], fileEntries: [entry11] })
    const headers = wrapper.findAll('thead th').map((th) => th.text().trim())
    // 前两列为复选框与缩略图（无表头文案），状态列殿后
    expect(headers).toEqual(['', '', '名称', '路径', '文件大小', '修改时间', '星级', '标签', '状态'])

    const cells = rowCells(wrapper, 0)
    expect(cells[2]).toBe('城商行方案')
    expect(cells[3]).toBe('方案A/城商行方案.pptx')
    expect(cells[4]).toBe('1 MB')
    expect(cells[5]).toBe('2026-10-12 09:00:00')
    expect(cells[6]).toBe('3 星')
    expect(cells[7]).toContain('重点')
    expect(cells[8]).toBe('正常')
  })

  it('幻灯片条目列集（浏览）：页码列 + 大小/修改时间经底册 join 命中（表头 tooltip 声明归属文件属性）+ 自有标签列', () => {
    const wrapper = mountList({ cards: [browseSlideCard], fileEntries: [entry11] })
    const headers = wrapper.findAll('thead th').map((th) => th.text().trim())
    expect(headers).toEqual(['', '', '页标题', '归属文件', '页码', '文件大小', '修改时间', '星级', '自有标签'])

    // 大小/修改时间表头带「归属文件属性」tooltip（口径声明：列值非页自身属性）
    const sizeTh = wrapper.findAll('thead th')[5]
    expect(sizeTh.attributes('title')).toBe('归属文件属性')
    const timeTh = wrapper.findAll('thead th')[6]
    expect(timeTh.attributes('title')).toBe('归属文件属性')

    const cells = rowCells(wrapper, 0)
    expect(cells[2]).toBe('封面页')
    expect(cells[4]).toBe('第 1 页')
    // 幻灯片条目无 size/modTime 字段，取归属文件底册属性
    expect(cells[5]).toBe('1 MB')
    expect(cells[6]).toBe('2026-10-12 09:00:00')
    // 星级取页条目自身 rating（2），非归属文件的 3
    expect(cells[7]).toBe('2 星')
    // 自有标签列显 ownTags（继承标签不混显）
    expect(cells[8]).toContain('封面页标签')
  })

  it('join miss：filePath 不在底册时大小/修改时间/状态一律显 —（不猜值）', () => {
    const wrapper = mountList({ cards: [searchFileMissCard], fileEntries: [entry11] })
    const cells = rowCells(wrapper, 0)
    expect(cells[4]).toBe('—')
    expect(cells[5]).toBe('—')
    expect(cells[8]).toBe('—')
  })

  it('检索模式：聚合条目（hitCount>1）大小列显 — 但修改时间照常 join；幻灯片条目追加命中摘要列', () => {
    const wrapper = mountList({
      cards: [searchSlideAggCard],
      fileEntries: [entry11],
      searchMode: true
    })
    const headers = wrapper.findAll('thead th').map((th) => th.text().trim())
    // 命中摘要列插在修改时间与星级之间
    expect(headers).toEqual(['', '', '页标题', '归属文件', '页码', '文件大小', '修改时间', '命中摘要', '星级', '自有标签'])

    const cells = rowCells(wrapper, 0)
    // 聚合条目不显大小（多页命中不显单文件大小），修改时间 join 不豁免
    expect(cells[5]).toBe('—')
    expect(cells[6]).toBe('2026-10-12 09:00:00')
    expect(cells[7]).toBe('…城商行获客…')
  })

  it('星级取值链：slide 条目自身 rating 优先，0（未评）显 — 不落入底册 join 兜底', () => {
    const ratedSlide = browseSlideCard
    const noRatingSlide = {
      ...browseSlideCard,
      key: 'slide-31',
      id: 31,
      slide: { ...slideEntry21, id: 31, rating: 0 }
    }
    const wrapper = mountList({ cards: [ratedSlide, noRatingSlide], fileEntries: [entry11] })
    // 幻灯片列集星级列在第 8 格（复选框/缩略图后）：有 rating 显数值；0 为「未评」而非
    // nullish，?? 链停在自己身上显 —（即使归属文件底册有 3 星也不兜底——页级
    // 标注独立于文件标注，兜底会伪造页星级）
    expect(rowCells(wrapper, 0)[7]).toBe('2 星')
    expect(rowCells(wrapper, 1)[7]).toBe('—')
  })

  it('行点击上报 card-click 且载荷为卡片对象（选中/详情分派留宿主）', async () => {
    const wrapper = mountList({ cards: [browseFileCard], fileEntries: [entry11] })
    await wrapper.findAll('tbody tr')[0].trigger('click')
    expect(wrapper.emitted('card-click')).toHaveLength(1)
    // props 经 Vue 响应式代理，载荷为代理对象非原引用，按内容断言
    expect(wrapper.emitted('card-click')[0][0]).toStrictEqual(browseFileCard)
  })

  it('缩略图与网格同口径：文件卡取第 1 页封面（slide-num=1），幻灯片卡取页码', () => {
    const wrapper = mountList({ cards: [browseFileCard, browseSlideCard], fileEntries: [entry11] })
    const thumbs = wrapper.findAll('.asset-thumb-stub')
    expect(thumbs[0].attributes('data-file-id')).toBe('11')
    expect(thumbs[0].attributes('data-slide-num')).toBe('1')
    expect(thumbs[0].attributes('data-tier')).toBe('0')
    expect(thumbs[1].attributes('data-slide-num')).toBe('1')
  })

  it('滚动到底上报 load-more，未到底不上报（防重与分页判定留宿主）', async () => {
    const wrapper = mountList({ cards: [browseFileCard], fileEntries: [entry11] })
    const listEl = wrapper.find('.asset-list')
    Object.defineProperty(listEl.element, 'scrollHeight', { value: 500, configurable: true })
    Object.defineProperty(listEl.element, 'clientHeight', { value: 400, configurable: true })

    // 未到底（距底 150 > 80）不上报
    listEl.element.scrollTop = 0
    await listEl.trigger('scroll')
    expect(wrapper.emitted('load-more')).toBeUndefined()

    // 到底（距底 50 ≤ 80）上报
    listEl.element.scrollTop = 150
    await listEl.trigger('scroll')
    expect(wrapper.emitted('load-more')).toHaveLength(1)
  })

  it('懒加载提示：loadingMore 显「加载中…」，canLoadMore 显「下拉加载更多」', () => {
    const idle = mountList({ cards: [browseFileCard] })
    expect(idle.find('.load-more-hint').exists()).toBe(false)

    const loading = mountList({ cards: [browseFileCard], loadingMore: true, canLoadMore: true })
    expect(loading.find('.load-more-hint').text()).toBe('加载中…')

    const pending = mountList({ cards: [browseFileCard], canLoadMore: true })
    expect(pending.find('.load-more-hint').text()).toBe('下拉加载更多')
  })

  it('聚合展开子行：expandedAggPaths 命中时渲染缩进子行，子行点击上报 card-click(child)', async () => {
    const childCard = {
      key: 'hit-slide-23',
      kind: 'slide',
      id: 23,
      fileId: 11,
      title: '获客页',
      filePath: '方案A/城商行方案.pptx',
      slideNum: 3,
      snippet: '…获客成本…',
      hitCount: 1,
      status: '',
      tags: [],
      hit: { refType: 'slide', refId: 23, hitCount: 1 }
    }
    const wrapper = mountList({
      cards: [searchSlideAggCard],
      fileEntries: [entry11],
      searchMode: true,
      expandedAggPaths: ['方案A/城商行方案.pptx'],
      aggChildren: { '方案A/城商行方案.pptx': [childCard] }
    })
    const childRows = wrapper.findAll('.agg-child-row')
    expect(childRows.length).toBe(1)
    expect(childRows[0].text()).toContain('第 3 页')
    expect(childRows[0].text()).toContain('获客页')

    // 子行点击仅上报子项（.stop 防冒泡触发父行聚合切换语义）
    await childRows[0].trigger('click')
    expect(wrapper.emitted('card-click')).toHaveLength(1)
    expect(wrapper.emitted('card-click')[0][0]).toStrictEqual(childCard)
  })

  // ---- 行首复选框（SRS §2.3 PR3 选中体系，受控契约）----

  it('行首复选框受控渲染：勾选态读 selectedKeys 键集（文件键 file:<id>），change 上报 toggle-select(card)；聚合父卡不渲染复选框（D5#1）', async () => {
    const wrapper = mountList({
      cards: [browseFileCard, searchSlideAggCard],
      fileEntries: [entry11],
      selectedKeys: new Set(['file:11'])
    })
    // 聚合父卡（hitCount=3）无复选框，仅独立文件卡 1 个
    const checks = wrapper.findAll('.asset-row-check')
    expect(checks.length).toBe(1)
    // 勾选态由宿主键集驱动（键形态 file:11 = 共享映射 assetSelectionKey 消费证据）
    expect(checks[0].element.checked).toBe(true)

    // 取消勾选：上报 toggle-select 载荷为卡片对象（写回分派留宿主）
    await checks[0].setValue(false)
    expect(wrapper.emitted('toggle-select')).toHaveLength(1)
    expect(wrapper.emitted('toggle-select')[0][0]).toStrictEqual(browseFileCard)
  })

  it('幻灯片行勾选键形态 slide:<fileId>:<slideNum>（与网格共享单点，禁 slides.id——重解析全变）', () => {
    const wrapper = mountList({
      cards: [browseSlideCard],
      fileEntries: [],
      selectedKeys: new Set(['slide:11:1'])
    })
    expect(wrapper.find('.asset-row-check').element.checked).toBe(true)

    // 键集不含该键时未勾选（slides.id=21 作键不命中，证明键与 slides.id 无关）
    const unselected = mountList({
      cards: [browseSlideCard],
      fileEntries: [],
      selectedKeys: new Set(['slide:21'])
    })
    expect(unselected.find('.asset-row-check').element.checked).toBe(false)
  })

  it('勾选点击不触发行点击（@click.stop 防勾选进详情，D5#7），change 才上报 toggle-select', async () => {
    const wrapper = mountList({ cards: [browseFileCard], fileEntries: [entry11] })
    const check = wrapper.find('.asset-row-check')
    // click 被 .stop 阻断冒泡：行 card-click 不上报（jsdom 切换 checked 但不派发
    // change；VTU setValue 对 checkbox 值未变即跳过 change——勾选语义断言统一走
    // trigger('change')，与宿主组件用法一致）
    await check.trigger('click')
    expect(wrapper.emitted('card-click')).toBeUndefined()
    expect(wrapper.emitted('toggle-select')).toBeUndefined()
    // change 才是勾选语义入口
    await check.trigger('change')
    expect(wrapper.emitted('toggle-select')).toHaveLength(1)
  })
})
