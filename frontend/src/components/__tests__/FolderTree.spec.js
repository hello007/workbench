import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick, reactive, watch } from 'vue'
import ElementPlus from 'element-plus'
import FolderTree from '../FolderTree.vue'
import { ASSET_FILE_DRAG_MIME, ASSET_FOLDER_DRAG_MIME } from '../../utils/assetFolder'

// ElMessage 桩（drop 拒绝提示断言面）
vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
  }
})

// 文件夹树受控组件（PRD PR1）：树渲染 / 徽标递归计数 / 排除标记 / 根「全部素材」
// 固定节点 / 选中 emit（folderPath 语义：根 = 空串）。树数据/计数/选中态均由宿主
// 持有，本 spec 只验证展示与上报契约；全展/全收 + 双击切换为组件内自治交互
// （经 el-tree store Node 实例操作），reload 后展开面断言守护 expandedKeys 同步。

// 形状对齐 model.AssetFolderNode（models.ts class，plain object 直传）
const nodeA = { path: '方案A', name: '方案A', children: [], excluded: false, derived: false, fileCount: 2 }
const nodeB = { path: '归档', name: '归档', children: [
  { path: '归档/2025', name: '2025', children: [], excluded: false, derived: false, fileCount: 1 }
], excluded: false, derived: false, fileCount: 1 }
const nodeExcluded = { path: 'secret', name: 'secret', children: [
  { path: 'secret/inner', name: 'inner', children: [], excluded: true, derived: false, fileCount: 0 }
], excluded: true, derived: false, fileCount: 0 }
const nodeDerived = { path: '拆分产物', name: '拆分产物', children: [], excluded: false, derived: true, fileCount: 0 }

// 局部 reload 模拟：宿主重新拉取 ListAssetFolders 后 nodes 数组整体替换（全新对象
// 引用、同内容），驱动 el-tree store 按 default-expanded-keys 回放展开面
const makeNodes = () => JSON.parse(JSON.stringify([nodeA, nodeB, nodeExcluded, nodeDerived]))

// el-tree 桩：递归渲染 default 插槽并注入 { data } 作用域（对齐真实 el-tree 插槽
// 契约；组件 slot 守卫依赖 scope.data 形态，桩必须同样注入）。点击叶子节点
// stopPropagation 防冒泡到祖先节点的 onClick 双触发。
// 扩展（全展/全收 + 双击切换批）：暴露近似真实 el-tree 的 store.nodesMap
// （node-key 值 → { expanded }，reactive 驱动重渲染），供组件经 store 直赋操作与
// 用例断言；子层级按 expanded 渲染（近似真实 el-tree 的 v-show 显隐，收起子树
// 不出 DOM）。default-expanded-keys 仅在（重）建节点时播种——对齐真实语义
// 「已挂载 Node 不响应该 prop 变化」：树数据刷新（局部 reload，data 引用变更）
// 时按当前 defaultExpandedKeys 重建全表，expandedKeys 同步正确性（漂移守护）
// 经 reload 后断言验证
const ElTreeStub = {
  name: 'ElTreeStub',
  props: ['data', 'nodeKey', 'props', 'currentNodeKey', 'expandOnClickNode', 'defaultExpandAll', 'defaultExpandedKeys', 'highlightCurrent'],
  emits: ['node-click'],
  setup(stubProps, { slots, emit, expose }) {
    const labelKey = stubProps.props?.label || 'label'
    const childrenKey = stubProps.props?.children || 'children'
    const keyOf = (node) => String(node[stubProps.nodeKey || 'path'])
    const nodesMap = reactive({})
    const seedNodesMap = (nodes) => {
      Object.keys(nodesMap).forEach((key) => delete nodesMap[key])
      const walk = (list) => {
        ;(list || []).forEach((node) => {
          nodesMap[keyOf(node)] = { expanded: (stubProps.defaultExpandedKeys || []).includes(keyOf(node)) }
          walk(node[childrenKey])
        })
      }
      walk(nodes)
    }
    watch(() => [stubProps.data, stubProps.defaultExpandedKeys], () => seedNodesMap(stubProps.data), { immediate: true })
    // expose：setup 返回渲染函数形态下，模板 ref 只能取到 expose 对象——FolderTree
    // 经 treeRef.value.store.nodesMap 操作 store 全靠这里（与真实 el-tree 的
    // ref.store 同构）
    expose({ store: { nodesMap } })
    const renderNodes = (nodes) =>
      (nodes || []).map((node) => {
        const record = nodesMap[keyOf(node)]
        return h(
          'div',
          {
            class: 'el-tree-node-stub',
            onClick: (event) => {
              event.stopPropagation()
              emit('node-click', node)
            }
          },
          [
            slots.default ? slots.default({ data: node }) : h('span', node[labelKey]),
            ...(record?.expanded ? renderNodes(node[childrenKey] || []) : [])
          ]
        )
      })
    return () => h('div', { class: 'el-tree-stub' }, renderNodes(stubProps.data))
  }
}

const mountTree = (props = {}, extra = {}) =>
  mount(FolderTree, {
    attachTo: extra.attachTo,
    props: {
      nodes: [nodeA, nodeB, nodeExcluded, nodeDerived],
      countMap: { '': 4, 方案A: 2, 归档: 1, '归档/2025': 1 },
      selectedPath: '',
      loading: false,
      ...props
    },
    global: { stubs: {
      'el-tree': ElTreeStub,
      // 聚焦事件须按 opened（组件 @opened 绑定）：@open 时点早于 EP focus-trap 初始聚焦，
      // 真机焦点会被夺回 wrapper——桩无 focus-trap 行为，watch modelValue 翻真即 emit
      'el-dialog': { template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>', props: ['modelValue', 'title', 'width', 'appendToBody'], emits: ['update:modelValue', 'opened'], watch: { modelValue(v, o) { if (v && !o) this.$emit('opened') } } },
      'el-input': {
        template: '<input class="el-input" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup="$emit(\'keyup\', $event)" />',
        props: ['modelValue', 'placeholder', 'maxlength'],
        emits: ['update:modelValue', 'keyup'],
        // 对齐真实 el-input 对外暴露的 focus()（转发到原生 input，jsdom 可断言 activeElement）
        methods: { focus() { this.$el && this.$el.focus && this.$el.focus() } }
      },
      'el-button': {
        template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
        props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
        emits: ['click']
      },
      'el-dropdown': {
        template: '<div class="el-dropdown"><slot /><slot name="dropdown" /></div>',
        emits: ['command', 'visible-change'],
        provide() { return { __folderMenuCommand: (cmd) => this.$emit('command', cmd) } }
      },
      'el-dropdown-menu': { template: '<div class="el-dropdown-menu"><slot /></div>' },
      'el-dropdown-item': {
        template: '<div class="el-dropdown-item" :class="{ \'is-disabled\': disabled }" :title="title" @click="onClick"><slot /></div>',
        props: ['command', 'disabled', 'divided', 'title'],
        inject: { __folderMenuCommand: { default: null } },
        methods: {
          onClick() {
            if (this.disabled) return
            this.__folderMenuCommand && this.__folderMenuCommand(this.command)
          }
        }
      }
    } }
  })

const nodeTexts = (wrapper) => wrapper.findAll('.folder-node-name').map(n => n.text())

// 节点行定位（按显示名），供菜单交互用例取节点上下文
const findNodeRow = (wrapper, name) =>
  wrapper.findAll('.folder-node').find(n => n.text().includes(name))

// 读取桩暴露的 store（expose 形态；组件自身经 treeRef.value.store 同源访问）
const stubStore = (wrapper) => wrapper.findComponent({ name: 'ElTreeStub' }).vm.$.exposed.store

// 树头部操作区两按钮（[全部展开, 全部收起]）
const headerButtons = (wrapper) => wrapper.findAll('.folder-tree-actions button')

// 点击节点管理菜单项（经 el-dropdown-item 桩的 command 透传）
const COMMAND_LABEL = { create: '新增子目录', rename: '重命名', annotate: '标注', rebuild: '重建索引', delete: '删除' }
const clickMenuItem = async (wrapper, nodeName, command) => {
  const row = findNodeRow(wrapper, nodeName)
  const item = row.findAll('.el-dropdown-item').find(i => i.text().trim() === COMMAND_LABEL[command])
  await item.trigger('click')
}

describe('FolderTree', () => {
  it('渲染根「全部素材」固定节点与后端子目录树（嵌套层级完整）', () => {
    const wrapper = mountTree()
    const texts = nodeTexts(wrapper)
    expect(texts).toContain('全部素材')
    expect(texts).toContain('方案A')
    expect(texts).toContain('归档')
    expect(texts).toContain('2025') // 深层子节点可见（default-expand-all）
    expect(texts).toContain('secret')
    expect(texts).toContain('inner')
  })

  it('徽标显示递归 PPT 计数（countMap 按节点 path 取值，0 不显示）', () => {
    const wrapper = mountTree()
    const badges = wrapper.findAll('.folder-node-badge').map(n => Number(n.text()))
    expect(badges).toContain(4) // 根
    expect(badges).toContain(2) // 方案A
    expect(badges).toContain(1)
    // secret 递归计数 0：不渲染徽标
    const secretNode = wrapper.findAll('.folder-node').find(n => n.text().includes('secret'))
    expect(secretNode.find('.folder-node-badge').exists()).toBe(false)
  })

  it('排除规则命中节点带「已排除」标记（含子树继承），拆分派生容器带「派生」标记', () => {
    const wrapper = mountTree()
    expect(wrapper.findAll('.folder-node-excluded').length).toBe(3) // secret + secret/inner + 派生容器
    const derivedNode = findNodeRow(wrapper, '拆分产物')
    expect(derivedNode.find('.folder-node-excluded').text()).toBe('派生')
    const normalNode = findNodeRow(wrapper, '方案A')
    expect(normalNode.find('.folder-node-excluded').exists()).toBe(false)
  })

  // ---- 节点管理菜单（PR2 R3）：管理意图经 command 上报，弹层输入在组件内收集 ----

  it('新增子目录：菜单打开弹层，输入名称后 emit create（parentPath + 名称）', async () => {
    const wrapper = mountTree()
    await clickMenuItem(wrapper, '方案A', 'create')
    const input = wrapper.find('.stub-dialog input')
    await input.setValue('新子目录')
    await wrapper.findAll('.dialog-footer button').find(b => b.text() === '创建').trigger('click')
    expect(wrapper.emitted('create')[0]).toEqual(['方案A', '新子目录'])
  })

  it('重命名：emit rename（节点 path + 新名称，输入预填当前名）', async () => {
    const wrapper = mountTree()
    await clickMenuItem(wrapper, '方案A', 'rename')
    expect(wrapper.find('.stub-dialog input').element.value).toBe('方案A')
    await wrapper.find('.stub-dialog input').setValue('方案A2')
    await wrapper.findAll('.dialog-footer button').find(b => b.text() === '重命名').trigger('click')
    expect(wrapper.emitted('rename')[0]).toEqual(['方案A', '方案A2'])
  })

  // 聚焦断言依赖元素在文档树内（jsdom 对游离元素 focus 不生效），须 attachTo 挂载
  // 用例名与断言面按 @opened 时点（打开动画结束、focus-trap 落定后聚焦输入框）
  it('新建子目录弹窗打开后输入框自动获焦（@opened + nextTick）', async () => {
    const wrapper = mountTree({}, { attachTo: document.body })
    await clickMenuItem(wrapper, '方案A', 'create')
    await nextTick()
    expect(wrapper.find('.stub-dialog input').element).toBe(document.activeElement)
    wrapper.unmount()
  })

  it('重命名弹窗打开后输入框自动获焦（两弹窗对称复用同一 helper，@opened 时点）', async () => {
    const wrapper = mountTree({}, { attachTo: document.body })
    await clickMenuItem(wrapper, '方案A', 'rename')
    await nextTick()
    expect(wrapper.find('.stub-dialog input').element).toBe(document.activeElement)
    wrapper.unmount()
  })

  it('删除：emit delete 上报节点对象（确认弹窗与 API 分派在宿主）', async () => {
    const wrapper = mountTree()
    await clickMenuItem(wrapper, '方案A', 'delete')
    expect(wrapper.emitted('delete').length).toBe(1)
    expect(wrapper.emitted('delete')[0][0].path).toBe('方案A')
  })

  it('禁管理节点分操作：排除命中禁新增/删除但重命名放行（翻出排除名出口），派生容器全禁，根仅新增可用', async () => {
    const wrapper = mountTree()
    const menuDisabledOf = (nodeName) => {
      const row = findNodeRow(wrapper, nodeName)
      const items = row.findAll('.el-dropdown-item')
      return {
        create: items.find(i => i.text().includes('新增')).classes(),
        rename: items.find(i => i.text().includes('重命名')).classes(),
        del: items.find(i => i.text().includes('删除')).classes()
      }
    }
    // 排除命中：新增/删除禁用（后端守卫同口径），重命名放行（翻出排除名 =
    // excluded 文件恢复索引可见的唯一目录级出口）
    const excluded = menuDisabledOf('secret')
    expect(excluded.create).toContain('is-disabled')
    expect(excluded.del).toContain('is-disabled')
    expect(excluded.rename).not.toContain('is-disabled')
    // 拆分派生容器：全禁
    const derived = menuDisabledOf('拆分产物')
    expect(derived.create).toContain('is-disabled')
    expect(derived.rename).toContain('is-disabled')
    expect(derived.del).toContain('is-disabled')
    // 根：仅新增可用
    const root = menuDisabledOf('全部素材')
    expect(root.rename).toContain('is-disabled')
    expect(root.del).toContain('is-disabled')
    expect(root.create).not.toContain('is-disabled')
  })

  it('点击节点 emit select 且上报 folderPath（根上报空串）', async () => {
    const wrapper = mountTree()
    const nodes = wrapper.findAll('.folder-node')
    await nodes.find(n => n.text().includes('方案A')).trigger('click')
    expect(wrapper.emitted('select')[0]).toEqual(['方案A'])
    await nodes.find(n => n.text().includes('全部素材')).trigger('click')
    expect(wrapper.emitted('select')[1]).toEqual([''])
  })

  it('空树仅显示根节点与空态提示', () => {
    const wrapper = mountTree({ nodes: [], countMap: {} })
    expect(nodeTexts(wrapper)).toEqual(['全部素材'])
    expect(wrapper.find('.folder-tree-hint').text()).toContain('暂无子文件夹')
  })

  it('loading 态显示加载提示且不显示空态', () => {
    const wrapper = mountTree({ loading: true })
    expect(wrapper.find('.folder-tree-hint').text()).toContain('加载中')
  })

  // ---- 文件拖拽移动 drop 目标（PRD R4，PR3）----

  const makeDT = (fileIds) => ({
    getData: vi.fn((mime) =>
      mime === ASSET_FILE_DRAG_MIME && fileIds ? JSON.stringify({ fileIds }) : ''),
    dropEffect: null
  })

  it('drop 合法落点：解析载荷 emit drop-files（目标 path + fileIDs）', async () => {
    const wrapper = mountTree({ dropEnabled: true })
    const row = findNodeRow(wrapper, '方案A')
    const dt = makeDT([11, 12])
    await row.trigger('dragover', { dataTransfer: dt })
    expect(row.classes()).toContain('is-drop-target') // 悬停高亮
    await row.trigger('drop', { dataTransfer: dt })
    expect(wrapper.emitted('drop-files')[0]).toEqual(['方案A', [11, 12]])
  })

  it('根「全部素材」= 库根合法落点（path 空串上报）', async () => {
    const wrapper = mountTree({ dropEnabled: true })
    const row = findNodeRow(wrapper, '全部素材')
    await row.trigger('drop', { dataTransfer: makeDT([11]) })
    expect(wrapper.emitted('drop-files')[0]).toEqual(['', [11]])
  })

  it('排除命中与派生容器落点拒绝：ElMessage warning 且不 emit（后端守卫为第二防线）', async () => {
    const wrapper = mountTree({ dropEnabled: true })
    const { ElMessage } = await import('element-plus')
    await findNodeRow(wrapper, 'secret').trigger('drop', { dataTransfer: makeDT([11]) })
    expect(ElMessage.warning).toHaveBeenCalled()
    await findNodeRow(wrapper, '拆分产物').trigger('drop', { dataTransfer: makeDT([11]) })
    expect(ElMessage.warning).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('drop-files')).toBeUndefined()
  })

  it('非本面板来源拖拽（MIME 不匹配）静默忽略；dropEnabled=false 不响应不悬停', async () => {
    const wrapper = mountTree({ dropEnabled: true })
    await findNodeRow(wrapper, '方案A').trigger('drop', { dataTransfer: makeDT(null) })
    expect(wrapper.emitted('drop-files')).toBeUndefined()

    const disabled = mountTree({ dropEnabled: false })
    await findNodeRow(disabled, '方案A').trigger('dragover', { dataTransfer: makeDT([11]) })
    expect(findNodeRow(disabled, '方案A').classes()).not.toContain('is-drop-target')
    await findNodeRow(disabled, '方案A').trigger('drop', { dataTransfer: makeDT([11]) })
    expect(disabled.emitted('drop-files')).toBeUndefined()
  })

  // ---- 目录级拖拽移动（PRD R5/D，PR4）----

  const makeFolderDT = (folderPath) => ({
    getData: vi.fn((mime) =>
      mime === ASSET_FOLDER_DRAG_MIME && folderPath != null
        ? JSON.stringify({ folderPath })
        : ''),
    dropEffect: null
  })

  it('dragstart 目录载荷：非根非派生节点携带 ASSET_FOLDER_DRAG_MIME 载荷', async () => {
    const wrapper = mountTree({ dropEnabled: true })
    const dt = { setData: vi.fn(), effectAllowed: null }
    await findNodeRow(wrapper, '方案A').trigger('dragstart', { dataTransfer: dt })
    expect(dt.setData).toHaveBeenCalledWith(ASSET_FOLDER_DRAG_MIME, JSON.stringify({ folderPath: '方案A' }))
    // 根不可拖（无磁盘对应物）：不写载荷
    const rootDT = { setData: vi.fn(), effectAllowed: null }
    await findNodeRow(wrapper, '全部素材').trigger('dragstart', { dataTransfer: rootDT })
    expect(rootDT.setData).not.toHaveBeenCalled()
  })

  it('drop 目录载荷分派 drop-folder（源 → 目标）；自身与自身子树软拦截拒绝', async () => {
    const wrapper = mountTree({ dropEnabled: true })
    await findNodeRow(wrapper, '归档').trigger('drop', { dataTransfer: makeFolderDT('方案A') })
    expect(wrapper.emitted('drop-folder')[0]).toEqual(['方案A', '归档'])

    // 目标位于源自身子树内：warning 且不 emit（后端核心同款校验的前端软拦截）
    await findNodeRow(wrapper, '2025').trigger('drop', { dataTransfer: makeFolderDT('归档') })
    const { ElMessage } = await import('element-plus')
    expect(ElMessage.warning).toHaveBeenCalled()
    expect(wrapper.emitted('drop-folder').length).toBe(1)
  })

  it('菜单「标注」项：emit annotate 上报节点（弹窗与保存分派在宿主）', async () => {
    const wrapper = mountTree()
    await clickMenuItem(wrapper, '方案A', 'annotate')
    expect(wrapper.emitted('annotate').length).toBe(1)
    expect(wrapper.emitted('annotate')[0][0].path).toBe('方案A')
  })

  it('菜单「重建索引」项：正常目录 emit rebuild，根/排除/派生禁用（全量重建走工具栏入口）', async () => {
    const wrapper = mountTree()
    // 正常目录：emit rebuild 上报节点（确认弹窗与 API 分派在宿主）
    await clickMenuItem(wrapper, '方案A', 'rebuild')
    expect(wrapper.emitted('rebuild').length).toBe(1)
    expect(wrapper.emitted('rebuild')[0][0].path).toBe('方案A')
    // 禁用面：根（走工具栏全量重建）/排除命中/派生容器
    const rootRow = findNodeRow(wrapper, '全部素材')
    const rootItem = rootRow.findAll('.el-dropdown-item').find(i => i.text().trim() === '重建索引')
    expect(rootItem.classes()).toContain('is-disabled')
    const excludedRow = findNodeRow(wrapper, 'secret')
    const excludedItem = excludedRow.findAll('.el-dropdown-item').find(i => i.text().trim() === '重建索引')
    expect(excludedItem.classes()).toContain('is-disabled')
    expect(excludedItem.attributes('title')).toContain('排除')
    const derivedRow = findNodeRow(wrapper, '拆分产物')
    const derivedItem = derivedRow.findAll('.el-dropdown-item').find(i => i.text().trim() === '重建索引')
    expect(derivedItem.classes()).toContain('is-disabled')
  })

  // ---- 全部展开/全部收起 + 双击切换（树头部操作区，宿主零改动）----
  // 核心契约：批量/双击经 store Node 实例直赋 expanded，操作后显式重算 expandedKeys
  // （store 直赋不触发 node-expand/node-collapse 事件）；树数据局部 reload 时
  // default-expanded-keys 回放的正是 expandedKeys——reload 后展开面保持 = 同步正确

  it('树头部提供「全部展开」「全部收起」按钮，loading/busy 态禁用', () => {
    const wrapper = mountTree()
    const btns = headerButtons(wrapper)
    expect(btns.map(b => b.text().trim())).toEqual(['全部展开', '全部收起'])
    expect(btns[0].attributes('disabled')).toBeUndefined()
    expect(btns[1].attributes('disabled')).toBeUndefined()

    for (const disabledProps of [{ loading: true }, { busy: true }]) {
      const w = mountTree(disabledProps)
      headerButtons(w).forEach(b => expect(b.attributes('disabled')).toBeDefined())
      w.unmount()
    }
  })

  it('全部展开：store 全部节点 expanded=true（含根），局部 reload 后展开面不漂移', async () => {
    const wrapper = mountTree()
    const [, collapseBtn] = headerButtons(wrapper)
    await collapseBtn.trigger('click') // 先收起，制造「非全展开」起点
    expect(nodeTexts(wrapper)).toEqual(['全部素材'])
    await headerButtons(wrapper)[0].trigger('click')
    expect(nodeTexts(wrapper)).toContain('2025') // 深层子节点回显
    const map = stubStore(wrapper).nodesMap
    expect(Object.keys(map)).toContain('') // 根「全部素材」也在 store 注册表内
    Object.keys(map).forEach((key) => expect(map[key].expanded).toBe(true))
    // 局部 reload（同内容新数组引用）：default-expanded-keys 回放操作后展开面
    await wrapper.setProps({ nodes: makeNodes() })
    const reloadedMap = stubStore(wrapper).nodesMap
    Object.keys(reloadedMap).forEach((key) => expect(reloadedMap[key].expanded).toBe(true))
    expect(nodeTexts(wrapper)).toContain('2025')
  })

  it('全部收起：含根在内全部 expanded=false（仅剩根行可见），reload 后保持；收起子树内选中保留不报错', async () => {
    const wrapper = mountTree()
    // 当前选中节点在收起子树内：选中态与主区过滤由宿主持有，组件侧要求收起不报错
    await findNodeRow(wrapper, '归档').trigger('click')
    expect(wrapper.emitted('select')[0]).toEqual(['归档'])
    await headerButtons(wrapper)[1].trigger('click')
    expect(nodeTexts(wrapper)).toEqual(['全部素材'])
    const map = stubStore(wrapper).nodesMap
    Object.keys(map).forEach((key) => expect(map[key].expanded).toBe(false))
    // reload 后不回放旧展开面（expandedKeys 已同步为空）
    await wrapper.setProps({ nodes: makeNodes() })
    const reloadedMap = stubStore(wrapper).nodesMap
    Object.keys(reloadedMap).forEach((key) => expect(reloadedMap[key].expanded).toBe(false))
    expect(nodeTexts(wrapper)).toEqual(['全部素材'])
    expect(wrapper.emitted('select').length).toBe(1) // 收起不改选中上报
  })

  it('双击有子节点的目录名切换展开/收起，叶子目录双击不动作', async () => {
    const wrapper = mountTree()
    // 双击「归档」：展开 → 收起（子层级 2025 随之隐藏）
    await findNodeRow(wrapper, '归档').trigger('dblclick')
    expect(stubStore(wrapper).nodesMap['归档'].expanded).toBe(false)
    expect(nodeTexts(wrapper)).not.toContain('2025')
    // reload 后保持收起（expandedKeys 已同步），兄弟展开面不受影响
    await wrapper.setProps({ nodes: makeNodes() })
    expect(stubStore(wrapper).nodesMap['归档'].expanded).toBe(false)
    expect(stubStore(wrapper).nodesMap['方案A'].expanded).toBe(true)
    // 再双击展开回来
    await findNodeRow(wrapper, '归档').trigger('dblclick')
    expect(stubStore(wrapper).nodesMap['归档'].expanded).toBe(true)
    expect(nodeTexts(wrapper)).toContain('2025')
    // 叶子（children 为空）双击不动作
    await findNodeRow(wrapper, '方案A').trigger('dblclick')
    expect(stubStore(wrapper).nodesMap['方案A'].expanded).toBe(true)
  })

  it('单击节点仅选中过滤（select 事件行为不变），不切换展开态', async () => {
    const wrapper = mountTree()
    await findNodeRow(wrapper, '归档').trigger('click')
    expect(wrapper.emitted('select')[0]).toEqual(['归档'])
    expect(stubStore(wrapper).nodesMap['归档'].expanded).toBe(true)
    expect(nodeTexts(wrapper)).toContain('2025')
  })

  // ---- 真实 el-tree 用例（2026-10-29 bugfix）：旧桩未实现 EP「default-expanded-keys
  // 变化回放 setDefaultExpandedKeys → 逐 key expand(null, autoExpandParent=true) →
  // 祖先链强制展开」语义，双击收起被回放翻回的真机 bug 在桩下假绿。本组用例
  // mount 真实 EP 组件（仅 stub el-dialog/el-input/el-dropdown 等无关件，el-tree
  // 用真实实现），双击事件须打在 .folder-node 行元素上（真机事件路径——@dblclick
  // 绑定面），断言窗口跨多个 nextTick 覆盖回放 flush。禁改回桩形态（假绿回归）。
  describe('真实 el-tree 交互（双击收起/全收全展/reload 防回放翻回）', () => {
    const mountRealTree = async (props = {}) => {
      const wrapper = mount(FolderTree, {
        props: {
          nodes: makeNodes(),
          countMap: { '': 4, 方案A: 2, 归档: 1, '归档/2025': 1 },
          selectedPath: '',
          loading: false,
          ...props
        },
        global: { plugins: [ElementPlus] }
      })
      // 真实树挂载链：treeData computed → store 建节点 → default-expanded-keys
      // 播种 expand → 组件 watch expanded → nextTick 落视图，多 tick 等稳定
      for (let i = 0; i < 4; i++) await nextTick()
      return wrapper
    }
    // 真机事件路径：dblclick 绑在 .folder-node 行元素（.el-tree-node__content 的
    // 子节点），trigger 必须命中该元素而非父容器（冒泡路径不经过子节点）
    const realRow = (wrapper, name) =>
      wrapper.findAll('.el-tree-node__content')
        .find((r) => r.text().includes(name))
        .find('.folder-node')
    // 可见行文本集合：jsdom 无布局（offsetParent 恒 null），按「自身到根链上无
    // display:none」判定——v-show 隐藏直接落在 .el-tree-node__children 容器 style
    const isVisible = (el) => {
      let cur = el
      while (cur) {
        if (cur.style?.display === 'none') return false
        cur = cur.parentElement
      }
      return true
    }
    const visibleTexts = (wrapper) =>
      wrapper.findAll('.folder-node-name')
        .filter((n) => isVisible(n.element))
        .map((n) => n.text())
    const realStore = (wrapper) => wrapper.vm.treeRef?.store

    it('双击展开中目录收起：子层级隐藏且多 tick 后不翻回（祖先链回放防线）', async () => {
      const wrapper = await mountRealTree()
      expect(visibleTexts(wrapper)).toContain('2025')
      await realRow(wrapper, '归档').trigger('dblclick')
      // 多 tick：覆盖 setDefaultExpandedKeys 回放 + expand 祖先链写 + nextTick 视图
      for (let i = 0; i < 6; i++) await nextTick()
      expect(realStore(wrapper).nodesMap['归档'].expanded).toBe(false)
      expect(visibleTexts(wrapper)).not.toContain('2025')
    })

    it('双击收起中间层目录：其展开中的子孙不回放翻回该目录（可达展开面过滤）', async () => {
      const wrapper = await mountRealTree()
      // 「归档」展开且其子「2025」也在展开面——旧实现回放 expand(2025, true) 会
      // 强制展开祖先「归档」，本用例守护可达过滤剔除子孙后的收起稳定性
      await realRow(wrapper, '归档').trigger('dblclick')
      for (let i = 0; i < 8; i++) await nextTick()
      expect(realStore(wrapper).nodesMap['归档'].expanded).toBe(false)
      expect(realStore(wrapper).nodesMap['2025']?.expanded ?? false).toBe(false)
      // 收起面稳定后再双击展开恢复
      await realRow(wrapper, '归档').trigger('dblclick')
      for (let i = 0; i < 6; i++) await nextTick()
      expect(realStore(wrapper).nodesMap['归档'].expanded).toBe(true)
      expect(visibleTexts(wrapper)).toContain('2025')
    })

    it('全部收起后仅剩根行可见，全部展开恢复全部层级', async () => {
      const wrapper = await mountRealTree()
      await wrapper.findAll('.folder-tree-actions button').find((b) => b.text().includes('全部收起')).trigger('click')
      for (let i = 0; i < 6; i++) await nextTick()
      expect(visibleTexts(wrapper)).toEqual(['全部素材'])
      await wrapper.findAll('.folder-tree-actions button').find((b) => b.text().includes('全部展开')).trigger('click')
      for (let i = 0; i < 6; i++) await nextTick()
      const texts = visibleTexts(wrapper)
      expect(texts).toContain('方案A')
      expect(texts).toContain('归档')
      expect(texts).toContain('2025')
    })

    it('收起后树数据局部 reload（新引用）展开面不漂移（不翻回展开态）', async () => {
      const wrapper = await mountRealTree()
      await realRow(wrapper, '归档').trigger('dblclick')
      for (let i = 0; i < 6; i++) await nextTick()
      expect(realStore(wrapper).nodesMap['归档'].expanded).toBe(false)
      await wrapper.setProps({ nodes: makeNodes() })
      for (let i = 0; i < 6; i++) await nextTick()
      expect(realStore(wrapper).nodesMap['归档'].expanded).toBe(false)
      expect(visibleTexts(wrapper)).not.toContain('2025')
      // 兄弟展开面 reload 后保持
      expect(visibleTexts(wrapper)).toContain('方案A')
    })
  })
})
