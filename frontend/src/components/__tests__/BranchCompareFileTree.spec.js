import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BranchCompareFileTree from '../BranchCompareFileTree.vue'
import { buildTree, STATUS_LIST } from '../../utils/branchFileTree'

const stubs = {
  'el-input': {
    props: ['modelValue', 'placeholder', 'clearable', 'size', 'prefixIcon'],
    emits: ['update:modelValue'],
    template: '<input class="el-input" :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />'
  },
  'el-select': { template: '<div class="el-select"><slot /></div>', props: ['modelValue', 'multiple', 'placeholder', 'size', 'collapseTags', 'clearable'] },
  'el-option': { template: '<div class="el-option" />', props: ['label', 'value'] },
  'el-checkbox': {
    props: ['modelValue', 'size', 'label'],
    emits: ['update:modelValue'],
    template: '<label class="el-checkbox"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />{{ label }}</label>'
  }
}

const FILES = [
  { path: 'src/auth/login.js', status: 'added', additions: 30, deletions: 0, binary: false },
  { path: 'src/auth/logout.js', status: 'modified', additions: 2, deletions: 1, binary: false },
  { path: 'README.md', status: 'modified', additions: 4, deletions: 2, binary: false },
  { path: 'src/legacy.js', status: 'deleted', additions: 0, deletions: 18, binary: false },
  { path: 'src/logo.png', status: 'modified', additions: 0, deletions: 0, binary: true },
  { path: 'src/util.js', status: 'unchanged', additions: 0, deletions: 0, binary: false },
  { path: 'src/helpers/format.js', oldPath: 'src/format.js', status: 'renamed', additions: 3, deletions: 1, binary: false }
]

const mountTree = (props = {}) => mount(BranchCompareFileTree, {
  props: { files: FILES, ...props },
  global: { stubs }
})

describe('utils/branchFileTree.buildTree', () => {
  it('按 / 分段聚合目录，目录在前同级按名称排序', () => {
    const tree = buildTree([
      { path: 'src/b.js', status: 'added' },
      { path: 'README.md', status: 'modified' },
      { path: 'src/a/c.js', status: 'deleted' },
      { path: 'src/a/d.js', status: 'added' }
    ])
    // 目录 src 在前，文件 README.md 在后
    expect(tree.map(n => n.name)).toEqual(['src', 'README.md'])
    const src = tree[0]
    expect(src.dir).toBe(true)
    // src 下：子目录 a 在前，文件按名称排序
    expect(src.children.map(n => n.name)).toEqual(['a', 'b.js'])
    expect(src.children[0].children.map(n => n.name)).toEqual(['c.js', 'd.js'])
  })

  it('文件节点携带原始 file 对象与绝对相对路径', () => {
    const tree = buildTree([{ path: 'a/b/c.txt', status: 'added' }])
    const fileNode = tree[0].children[0].children[0]
    expect(fileNode.dir).toBe(false)
    expect(fileNode.path).toBe('a/b/c.txt')
    expect(fileNode.file.status).toBe('added')
  })

  it('空列表返回空树', () => {
    expect(buildTree([])).toEqual([])
    expect(buildTree(null)).toEqual([])
  })
})

describe('BranchCompareFileTree.vue', () => {
  it('渲染扁平文件列表为目录树（目录节点 + 文件节点）', () => {
    const wrapper = mountTree()
    const dirNodes = wrapper.findAll('.tree-node-dir')
    const fileNodes = wrapper.findAll('.tree-node-file')
    expect(dirNodes.length).toBeGreaterThanOrEqual(1)
    // unchanged 默认隐藏：7 个文件渲染 6 个
    expect(fileNodes).toHaveLength(6)
    expect(wrapper.find('.file-count').text()).toBe('6/7')
    // 目录名
    expect(dirNodes[0].text()).toContain('src')
  })

  it('unchanged 默认隐藏，「未变更」开关或状态过滤显式勾选时显示', async () => {
    const wrapper = mountTree()
    expect(wrapper.find('.status-unchanged.tree-node-file').exists()).toBe(false)
    wrapper.vm.showUnchanged = true
    await nextTick()
    expect(wrapper.find('.status-unchanged.tree-node-file').exists()).toBe(true)
    expect(wrapper.findAll('.tree-node-file')).toHaveLength(7)
    // 回关开关、改走状态过滤显式勾选 unchanged 亦可见
    wrapper.vm.showUnchanged = false
    wrapper.vm.statusFilter = ['unchanged']
    await nextTick()
    expect(wrapper.find('.status-unchanged.tree-node-file').exists()).toBe(true)
  })

  it('目录行点击收放（点击后子树隐藏、再点恢复）', async () => {
    const wrapper = mountTree()
    const dir = wrapper.find('.tree-node-dir')
    expect(dir.text()).toContain('src')
    // 默认展开：src 下可见 auth 等子节点
    expect(wrapper.find('.status-added.tree-node-file').exists()).toBe(true)
    await dir.trigger('click')
    await nextTick()
    expect(wrapper.find('.status-added.tree-node-file').exists()).toBe(false)
    await dir.trigger('click')
    await nextTick()
    expect(wrapper.find('.status-added.tree-node-file').exists()).toBe(true)
  })

  it('文件节点按状态着色（status-* class）', async () => {
    const wrapper = mountTree()
    wrapper.vm.showUnchanged = true
    await nextTick()
    expect(wrapper.find('.status-added .node-name').text()).toBe('login.js')
    expect(wrapper.find('.status-deleted .node-name').text()).toBe('legacy.js')
    expect(wrapper.find('.status-unchanged .node-name').text()).toBe('util.js')
  })

  it('rename 文件显示 old → new 双路径', () => {
    const wrapper = mountTree()
    const renamed = wrapper.find('.status-renamed .node-name')
    expect(renamed.text()).toBe('src/format.js → src/helpers/format.js')
    expect(renamed.attributes('title')).toBe('src/format.js → src/helpers/format.js')
  })

  it('二进制文件展示「二进制」标记而非行数', () => {
    const wrapper = mountTree()
    const bin = wrapper.find('.status-modified.tree-node-file .node-flag')
    expect(bin.exists()).toBe(true)
    expect(bin.text()).toBe('二进制')
  })

  it('点击文件节点 emit selectFile 并携带文件对象', async () => {
    const wrapper = mountTree()
    await wrapper.find('.status-added.tree-node-file').trigger('click')
    expect(wrapper.emitted('selectFile')).toBeTruthy()
    expect(wrapper.emitted('selectFile')[0][0].path).toBe('src/auth/login.js')
  })

  it('路径子串过滤：命中文件数与计数联动', async () => {
    const wrapper = mountTree()
    await wrapper.find('input.el-input').setValue('auth')
    await nextTick()
    expect(wrapper.findAll('.tree-node-file')).toHaveLength(2)
    expect(wrapper.find('.file-count').text()).toBe('2/7')
  })

  it('状态多选过滤：仅保留所选状态文件', async () => {
    const wrapper = mountTree()
    // script setup 绑定经 vm 代理可直改（过滤为响应式 computed）
    wrapper.vm.statusFilter = ['added', 'deleted']
    await nextTick()
    expect(wrapper.findAll('.tree-node-file')).toHaveLength(2)
    expect(wrapper.find('.status-added.tree-node-file').exists()).toBe(true)
    expect(wrapper.find('.status-deleted.tree-node-file').exists()).toBe(true)
  })

  it('过滤无命中时展示空态', async () => {
    const wrapper = mountTree()
    wrapper.vm.pathKeyword = '不存在的路径'
    await nextTick()
    expect(wrapper.find('.tree-empty').exists()).toBe(true)
    expect(wrapper.find('.tree-empty').text()).toContain('无匹配文件')
  })

  it('STATUS_LIST 覆盖七种状态且中文标签齐备', () => {
    expect(STATUS_LIST.map(s => s.value)).toEqual([
      'added', 'deleted', 'renamed', 'copied', 'typechange', 'modified', 'unchanged'
    ])
    expect(STATUS_LIST.every(s => s.label.length > 0)).toBe(true)
  })
})
