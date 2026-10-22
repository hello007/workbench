import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
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
// 持有，本 spec 只验证展示与上报契约。

// 形状对齐 model.AssetFolderNode（models.ts class，plain object 直传）
const nodeA = { path: '方案A', name: '方案A', children: [], excluded: false, derived: false, fileCount: 2 }
const nodeB = { path: '归档', name: '归档', children: [
  { path: '归档/2025', name: '2025', children: [], excluded: false, derived: false, fileCount: 1 }
], excluded: false, derived: false, fileCount: 1 }
const nodeExcluded = { path: 'secret', name: 'secret', children: [
  { path: 'secret/inner', name: 'inner', children: [], excluded: true, derived: false, fileCount: 0 }
], excluded: true, derived: false, fileCount: 0 }
const nodeDerived = { path: '拆分产物', name: '拆分产物', children: [], excluded: false, derived: true, fileCount: 0 }

// el-tree 桩：递归渲染 default 插槽并注入 { data } 作用域（对齐真实 el-tree 插槽
// 契约；组件 slot 守卫依赖 scope.data 形态，桩必须同样注入）。点击叶子节点
// stopPropagation 防冒泡到祖先节点的 onClick 双触发
const ElTreeStub = {
  name: 'ElTreeStub',
  props: ['data', 'nodeKey', 'props', 'currentNodeKey', 'expandOnClickNode', 'defaultExpandAll', 'highlightCurrent'],
  emits: ['node-click'],
  setup(stubProps, { slots, emit }) {
    const labelKey = stubProps.props?.label || 'label'
    const childrenKey = stubProps.props?.children || 'children'
    const renderNodes = (nodes) =>
      (nodes || []).map((node) =>
        h(
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
            ...renderNodes(node[childrenKey] || [])
          ]
        )
      )
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
      'el-dialog': { template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>', props: ['modelValue', 'title', 'width', 'appendToBody'], emits: ['update:modelValue', 'open'], watch: { modelValue(v, o) { if (v && !o) this.$emit('open') } } },
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

// 点击节点管理菜单项（经 el-dropdown-item 桩的 command 透传）
const COMMAND_LABEL = { create: '新增子目录', rename: '重命名', annotate: '标注', delete: '删除' }
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
  it('新建子目录弹窗打开后输入框自动获焦（@open + nextTick）', async () => {
    const wrapper = mountTree({}, { attachTo: document.body })
    await clickMenuItem(wrapper, '方案A', 'create')
    await nextTick()
    expect(wrapper.find('.stub-dialog input').element).toBe(document.activeElement)
    wrapper.unmount()
  })

  it('重命名弹窗打开后输入框自动获焦（两弹窗对称复用同一 helper）', async () => {
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
})
