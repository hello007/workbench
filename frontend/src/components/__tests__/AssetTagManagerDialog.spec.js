import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetTagManagerDialog from '../AssetTagManagerDialog.vue'

// 形状对齐 model.AssetTagListItem：两级样例（根 1 含子 2/3、根 4），计数覆盖
// 0 / 非 0 两形态（删除影响面合计与徽标渲染断言基础）
const mockTagList = [
  { id: 1, name: '重点客户', color: '#2563eb', parentId: 0, count: 3 },
  { id: 2, name: '城商行', color: '#0891b2', parentId: 1, count: 2 },
  { id: 3, name: '农信社', color: '#d97706', parentId: 1, count: 0 },
  { id: 4, name: '方案', color: '#16a34a', parentId: 0, count: 5 }
]

const { wailsMock } = vi.hoisted(() => ({
  wailsMock: {
    ListAssetTags: vi.fn(),
    CreateAssetTag: vi.fn(),
    UpdateAssetTag: vi.fn(),
    DeleteAssetTag: vi.fn(),
    MoveAssetTag: vi.fn()
  }
}))

vi.mock('../../../wailsjs/go/main/App', () => wailsMock)

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve(true)) }
  }
})

// ---- Element Plus 组件 stub（模式对齐 AssetsPanel.spec）----
const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'appendToBody'],
    emits: ['update:modelValue']
  },
  'el-form': { template: '<form><slot /></form>', props: ['labelWidth'] },
  'el-form-item': { template: '<div class="form-item"><slot /></div>', props: ['label'] },
  'el-input': {
    template: '<input :value="modelValue" :placeholder="placeholder" :disabled="disabled" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup="$emit(\'keyup\', $event)" />',
    props: ['modelValue', 'placeholder', 'disabled'],
    emits: ['update:modelValue', 'keyup']
  },
  'el-color-picker': {
    name: 'ElColorPickerStub',
    template: '<button class="color-picker" @click="$emit(\'update:modelValue\', \'#123456\')"></button>',
    props: ['modelValue', 'predefine', 'disabled'],
    emits: ['update:modelValue']
  },
  'el-select': {
    template: '<div class="stub-select"><slot /></div>',
    props: ['modelValue', 'placeholder', 'disabled', 'clearable'],
    emits: ['update:modelValue', 'change']
  },
  'el-option': { template: '<option :value="value"><slot /></option>', props: ['label', 'value'] },
  // el-button 桩透传原生事件对象：树节点操作列的 @click.stop 修饰符（withModifiers）
  // 需要真实 event 调 stopPropagation，裸 emit 无事件对象会抛错
  'el-button': {
    template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\', $event)"><slot /></button>',
    props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
    emits: ['click']
  },
  // 树桩：两级渲染节点插槽（操作列按钮经插槽透出），data-id 供定位；拖拽相关
  // props（draggable/allowDrop/onNodeDrop）声明为 prop 供用例直读直调（node-drop
  // 在 jsdom 无真实拖拽，经 props 调用即等价于 el-tree 回调）
  'el-tree': {
    template: `<div class="el-tree-stub">
      <div v-for="node in data" :key="node.id" class="tree-node" :data-id="node.id">
        <slot :node="node" :data="node" />
        <div v-for="child in node.children || []" :key="child.id" class="tree-node tree-child-node" :data-id="child.id">
          <slot :node="child" :data="child" />
        </div>
      </div>
    </div>`,
    props: ['data', 'nodeKey', 'defaultExpandAll', 'expandOnClickNode', 'draggable', 'allowDrop', 'onNodeDrop']
  },
  'el-empty': { template: '<div class="el-empty">{{ description }}<slot /></div>', props: ['description'] }
}

const createWrapper = async (props = {}) => {
  const wrapper = mount(AssetTagManagerDialog, {
    props: { modelValue: false, libraryId: 'lib-1', ...props },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

const openDialog = async (wrapper) => {
  await wrapper.setProps({ modelValue: true })
  await flushPromises()
}

const findButton = (scope, text) =>
  scope.findAll('button').find((b) => b.text().trim() === text)

describe('AssetTagManagerDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    wailsMock.ListAssetTags.mockResolvedValue(mockTagList)
    wailsMock.CreateAssetTag.mockResolvedValue({ id: 99, name: '新标签', color: '#7c3aed', parentId: 0, sortOrder: 0 })
    wailsMock.UpdateAssetTag.mockResolvedValue(null)
    wailsMock.DeleteAssetTag.mockResolvedValue(4)
    wailsMock.MoveAssetTag.mockResolvedValue(null)
  })

  it('打开即拉取 ListAssetTags，树形渲染色点/名称/直接绑定计数', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    expect(wailsMock.ListAssetTags).toHaveBeenCalledWith('lib-1')
    const nodes = wrapper.findAll('.tree-node')
    expect(nodes.length).toBe(4)
    const root = wrapper.find('.tree-node[data-id="1"]')
    // jsdom 将 #2563eb 归一化为 rgb 形态，断言同值
    expect(root.find('.tag-color-dot').attributes('style')).toContain('rgb(37, 99, 235)')
    expect(root.text()).toContain('重点客户')
    expect(root.text()).toContain('3')
    // 未使用标签不渲染计数徽标
    expect(wrapper.find('.tree-node[data-id="3"]').find('.tag-tree-count').exists()).toBe(false)
  })

  it('新增根标签：表单填写后保存，CreateAssetTag 收到 parentId=0 与颜色，成功后重拉并 emit changed', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    await findButton(wrapper, '新增根标签').trigger('click')
    expect(wrapper.find('.tag-form-title').text()).toBe('新增标签')

    await wrapper.find('input').setValue('自定义标签')
    await findButton(wrapper, '保存').trigger('click')
    await flushPromises()

    expect(wailsMock.CreateAssetTag).toHaveBeenCalledWith('lib-1', '自定义标签', 0, '')
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.success).toHaveBeenCalledWith('标签已创建')
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('changed')).toBeTruthy()
    // 成功后表单收起
    expect(wrapper.find('.tag-form').exists()).toBe(false)
  })

  it('预置色板与后端默认色板同源（assetTagPalette 对齐），颜色选择回填提交', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    await findButton(wrapper, '新增根标签').trigger('click')
    const picker = wrapper.findComponent({ name: 'ElColorPickerStub' })
    expect(picker.props('predefine')).toContain('#2563eb')

    await picker.vm.$emit('update:modelValue', '#123456')
    await wrapper.find('input').setValue('带色标签')
    await findButton(wrapper, '保存').trigger('click')
    await flushPromises()
    expect(wailsMock.CreateAssetTag).toHaveBeenCalledWith('lib-1', '带色标签', 0, '#123456')
  })

  it('新增子标签：节点操作预填父级，CreateAssetTag 收到该节点 id', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    const root = wrapper.find('.tree-node[data-id="1"]')
    await findButton(root, '新增子标签').trigger('click')
    await wrapper.find('input').setValue('细分标签')
    await findButton(wrapper, '保存').trigger('click')
    await flushPromises()
    expect(wailsMock.CreateAssetTag).toHaveBeenCalledWith('lib-1', '细分标签', 1, '')
  })

  it('编辑：表单回显当前值，UpdateAssetTag 整体提交（重命名/改色/移动父级）', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    await findButton(wrapper.find('.tree-node[data-id="4"]'), '编辑').trigger('click')
    expect(wrapper.find('.tag-form-title').text()).toBe('编辑标签')
    expect(wrapper.find('input').element.value).toBe('方案')

    await wrapper.find('input').setValue('活动方案')
    await findButton(wrapper, '保存').trigger('click')
    await flushPromises()
    expect(wailsMock.UpdateAssetTag).toHaveBeenCalledWith('lib-1', 4, '活动方案', 0, '#16a34a')
    expect(wrapper.emitted('changed')).toBeTruthy()
  })

  it('防环预拦截：编辑态父级选项排除自身与子树（后端 updateTagFull 主防线的前置层）', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    // 编辑节点 1（含子 2/3）：父级选项仅剩根 4
    await findButton(wrapper.find('.tree-node[data-id="1"]'), '编辑').trigger('click')
    const optionValues = wrapper.findAll('option').map((o) => Number(o.element.value))
    expect(optionValues).toEqual([4])
  })

  it('删除确认影响面：N=子树后代数（本地树计算）+ M=子树绑定计数合计，确认后以返回值为权威提示', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    await findButton(wrapper.find('.tree-node[data-id="1"]'), '删除').trigger('click')
    await flushPromises()

    const { ElMessageBox, ElMessage } = await import('element-plus')
    // N = 2（城商行/农信社），M = 3 + 2 + 0 = 5
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('将删除标签「重点客户」及其 2 个子标签，并解除 5 处绑定'),
      '删除标签',
      expect.objectContaining({ type: 'warning' })
    )
    expect(wailsMock.DeleteAssetTag).toHaveBeenCalledWith('lib-1', 1)
    // 成功提示以后端返回值（4 行绑定）为权威口径
    expect(ElMessage.success).toHaveBeenCalledWith('已删除标签及其 2 个子标签，解除 4 处绑定')
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('changed')).toBeTruthy()
  })

  it('删除叶标签：确认文案不含子标签数，成功提示按后端返回值', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    await findButton(wrapper.find('.tree-node[data-id="4"]'), '删除').trigger('click')
    await flushPromises()

    const { ElMessageBox, ElMessage } = await import('element-plus')
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('将删除标签「方案」，并解除 5 处绑定'),
      '删除标签',
      expect.objectContaining({ type: 'warning' })
    )
    expect(ElMessage.success).toHaveBeenCalledWith('已删除标签，解除 4 处绑定')
  })

  it('删除取消：确认弹窗取消不调用 DeleteAssetTag', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    const { ElMessageBox } = await import('element-plus')
    ElMessageBox.confirm.mockRejectedValueOnce(new Error('cancel'))
    await findButton(wrapper.find('.tree-node[data-id="1"]'), '删除').trigger('click')
    await flushPromises()
    expect(wailsMock.DeleteAssetTag).not.toHaveBeenCalled()
    expect(wrapper.emitted('changed')).toBeFalsy()
  })

  it('创建失败（重名等后端拒绝，普通 error 形态）经 handleError 分流带前缀提示', async () => {
    wailsMock.CreateAssetTag.mockRejectedValue(new Error('标签「自定义标签」已存在'))
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    await findButton(wrapper, '新增根标签').trigger('click')
    await wrapper.find('input').setValue('自定义标签')
    await findButton(wrapper, '保存').trigger('click')
    await flushPromises()

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('创建标签失败: 标签「自定义标签」已存在')
    // 失败不收表单不重拉（用户可直接改名重试）
    expect(wrapper.find('.tag-form').exists()).toBe(true)
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(1)
  })

  it('空态：无标签时渲染引导空态与新增入口', async () => {
    wailsMock.ListAssetTags.mockResolvedValue([])
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    expect(wrapper.find('.el-empty').text()).toContain('暂无标签')
    expect(findButton(wrapper, '新增根标签')).toBeTruthy()
  })

  it('presetCreate 预填：打开即展开新增根标签表单（详情「+ 新建标签」快捷入口）', async () => {
    const wrapper = await createWrapper({ modelValue: true, presetCreate: true })
    await flushPromises()
    expect(wrapper.find('.tag-form-title').text()).toBe('新增标签')
    expect(wailsMock.ListAssetTags).toHaveBeenCalledWith('lib-1')
  })

  it('名称为空提交被拦截（前端防线），不发创建请求', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    await findButton(wrapper, '新增根标签').trigger('click')
    await findButton(wrapper, '保存').trigger('click')
    await flushPromises()
    expect(wailsMock.CreateAssetTag).not.toHaveBeenCalled()
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalledWith('请输入标签名')
  })

  // ---- 拖拽排序（D8#1）：node-drop 落点换算 + 乐观更新 + 失败回滚 ----

  // node-drop 在 jsdom 无真实拖拽：经树桩 props 直调 onNodeDrop（等价 el-tree 回调），
  // 节点参数形态对齐 el-tree Node（回调只读 .data.id）
  const dropOnTree = async (wrapper, dragId, dropId, dropType) => {
    const tree = wrapper.findComponent('.el-tree-stub')
    await tree.props('onNodeDrop')({ data: { id: dragId } }, { data: { id: dropId } }, dropType)
    await flushPromises()
  }

  it('拖拽同级排序（after 落点）：换算 (newParentID, index) 调 MoveAssetTag，乐观更新即时生效并 emit changed', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    // 拖「方案」(4) 到「重点客户」(1) 之后：rest=[1] → index 1 → 根组 [重点客户, 方案]
    await dropOnTree(wrapper, 4, 1, 'after')

    expect(wailsMock.MoveAssetTag).toHaveBeenCalledTimes(1)
    expect(wailsMock.MoveAssetTag).toHaveBeenCalledWith('lib-1', 4, 0, 1)
    // 乐观更新：重拉未被触发（LoadAssetTags 仍为打开时那一次），树序已变
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(1)
    const rootIds = wrapper.findAll('.el-tree-stub > .tree-node').map((n) => Number(n.attributes('data-id')))
    expect(rootIds).toEqual([1, 4])
    expect(wrapper.emitted('changed')).toBeTruthy()
  })

  it('拖拽变子级（inner 落点）：目标组尾追加，parentId 乐观切换', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    // 拖「方案」(4) 拖入「重点客户」(1)：重点客户子组 [城商行, 农信社] 排除自身后长 2 → index 2
    await dropOnTree(wrapper, 4, 1, 'inner')

    expect(wailsMock.MoveAssetTag).toHaveBeenCalledWith('lib-1', 4, 1, 2)
    const rootIds = wrapper.findAll('.el-tree-stub > .tree-node').map((n) => Number(n.attributes('data-id')))
    expect(rootIds).toEqual([1])
    // 子行挂到重点客户下（乐观态），节点 4 从根组消失
    const parent = wrapper.find('.tree-node[data-id="1"]')
    const childIds = parent.findAll('.tree-child-node').map((n) => Number(n.attributes('data-id')))
    expect(childIds).toContain(4)
  })

  it('拖拽失败：乐观态回滚 + 重拉保权威 + handleError 分流', async () => {
    wailsMock.MoveAssetTag.mockRejectedValue(new Error('父标签不能选择自身或其后代（会形成循环引用）'))
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    await dropOnTree(wrapper, 4, 1, 'after')

    const { ElMessage } = await import('element-plus')
    expect(ElMessage.error).toHaveBeenCalledWith('调整标签顺序失败: 父标签不能选择自身或其后代（会形成循环引用）')
    // 回滚：根组恢复原序（重点客户在前）
    const rootIds = wrapper.findAll('.el-tree-stub > .tree-node').map((n) => Number(n.attributes('data-id')))
    expect(rootIds).toEqual([1, 4])
    // 重拉保权威
    expect(wailsMock.ListAssetTags).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('changed')).toBeFalsy()
  })

  it('allowDrop 防环预拦截：目标在拖拽节点自身子树内一律拒绝（后端 moveTag 主防线的前置层）', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    const tree = wrapper.findComponent('.el-tree-stub')
    const allowDrop = tree.props('allowDrop')
    // 拖「重点客户」(1，含子 2/3)：inner 拖入子代 2 拒绝；before/after 挂到子代 2 旁拒绝
    expect(allowDrop({ data: { id: 1 } }, { data: { id: 2 } }, 'inner')).toBe(false)
    expect(allowDrop({ data: { id: 1 } }, { data: { id: 2 } }, 'after')).toBe(false)
    // 拖到无关根「方案」(4)：各落点均放行
    expect(allowDrop({ data: { id: 1 } }, { data: { id: 4 } }, 'inner')).toBe(true)
    expect(allowDrop({ data: { id: 1 } }, { data: { id: 4 } }, 'before')).toBe(true)
  })

  it('表单展开时暂停拖拽（dragEnabled），关闭表单恢复', async () => {
    const wrapper = await createWrapper()
    await openDialog(wrapper)
    const tree = wrapper.findComponent('.el-tree-stub')
    expect(tree.props('draggable')).toBe(true)
    await findButton(wrapper.find('.tree-node[data-id="4"]'), '编辑').trigger('click')
    expect(tree.props('draggable')).toBe(false)
    await findButton(wrapper, '取消').trigger('click')
    expect(tree.props('draggable')).toBe(true)
  })
})
