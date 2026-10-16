import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetTagTreeSelect from '../AssetTagTreeSelect.vue'

// 形状对齐 model.AssetTagListItem（ListAssetTags 返回，父级分组 + 组内 sortOrder 序；含未使用标签 id=4）
const mockTagList = [
  { id: 1, name: '重点客户', color: '#2563eb', parentId: 0, count: 3 },
  { id: 2, name: '城商行', color: '#0891b2', parentId: 1, count: 2 },
  { id: 4, name: '方案', color: '#16a34a', parentId: 0, count: 0 }
]

// vi.mock 工厂被 hoist，不能引用外部变量；用 vi.hoisted 提升可变的 mock 函数集合
const { wailsMock } = vi.hoisted(() => ({
  wailsMock: {
    ListAssetTags: vi.fn()
  }
}))

vi.mock('../../../wailsjs/go/main/App', () => wailsMock)

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

// el-tree-select 桩：两级树形渲染默认插槽（节点内容 = 色点 + 名称 + 计数徽标）与
// empty 插槽，暴露 visible-change / update:modelValue 触发入口供交互断言
const ElTreeSelectStub = {
  name: 'ElTreeSelectStub',
  template: `
    <div class="el-tree-select-stub">
      <div v-if="data.length === 0" class="stub-empty-slot"><slot name="empty" /></div>
      <ul class="stub-tree">
        <li v-for="node in data" :key="node.value" class="stub-node" :data-node="node.value">
          <slot :data="node" />
          <ul v-if="node.children && node.children.length" class="stub-children">
            <li v-for="child in node.children" :key="child.value" class="stub-node stub-child-node" :data-node="child.value">
              <slot :data="child" />
            </li>
          </ul>
        </li>
      </ul>
      <button class="stub-open" @click="$emit('visible-change', true)">展开下拉</button>
      <button class="stub-check" @click="$emit('update:modelValue', [1, 2]); $emit('change', [1, 2])">勾选</button>
    </div>`,
  props: ['modelValue', 'data', 'placeholder', 'disabled', 'loading', 'multiple', 'filterable', 'clearable', 'showCheckbox', 'checkStrictly', 'defaultExpandAll', 'renderAfterExpand', 'collapseTags', 'collapseTagsTooltip'],
  emits: ['update:modelValue', 'change', 'visible-change']
}

const createWrapper = async (props = {}) => {
  const wrapper = mount(AssetTagTreeSelect, {
    props: { libraryId: 'lib-1', modelValue: [], ...props },
    global: { stubs: { 'el-tree-select': ElTreeSelectStub } }
  })
  await flushPromises()
  return wrapper
}

describe('AssetTagTreeSelect', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    wailsMock.ListAssetTags.mockResolvedValue(mockTagList)
  })

  it('挂载即拉取 ListAssetTags，树按 parentId 构建两级（子节点挂父节点下）', async () => {
    const wrapper = await createWrapper()
    expect(wailsMock.ListAssetTags).toHaveBeenCalledWith('lib-1')
    const roots = wrapper.findAll('.stub-tree > .stub-node')
    expect(roots.length).toBe(2)
    expect(roots[0].text()).toContain('重点客户')
    // 子节点渲染在父节点内
    expect(roots[0].find('.stub-child-node').text()).toContain('城商行')
    expect(roots[1].text()).toContain('方案')
  })

  it('标签总数少于阈值（20）时下拉树默认全部展开，达到阈值后保持收起', async () => {
    // 少标签（mockTagList 3 个）：default-expand-all 透传为 true，免逐级点开
    const few = await createWrapper()
    expect(few.findComponent(ElTreeSelectStub).props('defaultExpandAll')).toBe(true)

    // 达到阈值（20 个平铺标签）：保持默认收起，防长列表全展开刷屏
    const manyTags = Array.from({ length: 20 }, (_, i) => ({
      id: i + 1, name: `标签${i + 1}`, color: '#2563eb', parentId: 0, count: 0
    }))
    wailsMock.ListAssetTags.mockResolvedValue(manyTags)
    const many = await createWrapper()
    expect(many.findComponent(ElTreeSelectStub).props('defaultExpandAll')).toBe(false)
  })

  it('节点呈现：色点用标签色数据 + 直接绑定计数徽标仅 count>0 渲染', async () => {
    const wrapper = await createWrapper()
    const root = wrapper.find('.stub-node[data-node="1"]')
    // jsdom 将 #2563eb 归一化为 rgb 形态，断言同值
    expect(root.find('.tag-node-dot').attributes('style')).toContain('rgb(37, 99, 235)')
    expect(root.find('.tag-node-count').text()).toBe('3')
    // 未使用标签（count=0）不渲染徽标
    const unused = wrapper.find('.stub-node[data-node="4"]')
    expect(unused.find('.tag-node-count').exists()).toBe(false)
  })

  it('勾选回传统一 ID 口径（update:modelValue 与 change 均为数组整体写回）', async () => {
    const wrapper = await createWrapper()
    await wrapper.find('.stub-check').trigger('click')
    expect(wrapper.emitted('update:modelValue')[0]).toEqual([[1, 2]])
    expect(wrapper.emitted('change')[0]).toEqual([[1, 2]])
  })

  it('下拉展开即重拉（防管理弹窗新建标签后树陈旧）', async () => {
    const wrapper = await createWrapper()
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(1)
    await wrapper.find('.stub-open').trigger('click')
    await flushPromises()
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(2)
    expect(wailsMock.ListAssetTags).toHaveBeenLastCalledWith('lib-1')
  })

  it('暴露 reload() 供宿主标签 CRUD 后强制刷新', async () => {
    const wrapper = await createWrapper()
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(1)
    await wrapper.vm.reload()
    await flushPromises()
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(2)
  })

  it('切库重拉且 loaded 事件回传平铺清单（宿主共享列表数据源）', async () => {
    const wrapper = await createWrapper()
    expect(wrapper.emitted('loaded')[0][0]).toEqual(mockTagList)
    wailsMock.ListAssetTags.mockResolvedValue([])
    await wrapper.setProps({ libraryId: 'lib-2' })
    await flushPromises()
    expect(wailsMock.ListAssetTags).toHaveBeenLastCalledWith('lib-2')
  })

  it('无库（libraryId 空）不拉取且树恒空，loaded 回传空清单', async () => {
    const wrapper = await createWrapper({ libraryId: '' })
    expect(wailsMock.ListAssetTags).not.toHaveBeenCalled()
    expect(wrapper.emitted('loaded')[0][0]).toEqual([])
  })

  it('空态：下拉内空态提示，文案支持消费方传入（筛选与详情语境不同）', async () => {
    wailsMock.ListAssetTags.mockResolvedValue([])
    const wrapper = await createWrapper()
    expect(wrapper.find('.tag-tree-empty').text()).toContain('暂无标签')
    await wrapper.unmount()

    const custom = await mount(AssetTagTreeSelect, {
      props: { libraryId: 'lib-1', modelValue: [], emptyText: '点「+ 新建标签」创建' },
      global: { stubs: { 'el-tree-select': ElTreeSelectStub } }
    })
    await flushPromises()
    expect(custom.find('.tag-tree-empty').text()).toBe('点「+ 新建标签」创建')
  })

  it('拉取失败经 handleError 分流为 error 提示且树降级为空（可重开重试）', async () => {
    wailsMock.ListAssetTags.mockRejectedValue(new Error('database is closed'))
    const wrapper = await createWrapper()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('加载标签列表失败: database is closed')
    expect(wrapper.findAll('.stub-node').length).toBe(0)
  })

  it('库不存在（E_ASSETS_LIB_NOT_FOUND）静默清空不弹错（宿主列表自愈负责）', async () => {
    wailsMock.ListAssetTags.mockRejectedValue({ code: 'E_ASSETS_LIB_NOT_FOUND', message: '素材库不存在' })
    const wrapper = await createWrapper()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).not.toHaveBeenCalled()
    expect(wrapper.findAll('.stub-node').length).toBe(0)
  })
})
