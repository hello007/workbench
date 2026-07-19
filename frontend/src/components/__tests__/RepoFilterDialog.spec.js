import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { computed, unref, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import RepoFilterDialog from '../RepoFilterDialog.vue'

// jsdom 无布局，useVirtualList 计算 containerHeight=0 -> list 为空，测试无法断言 .repo-item。
// 改为返回全部项，绕过虚拟化裁剪（仅测试分类/筛选/跳转逻辑，不测虚拟滚动本身）。
vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual('@vueuse/core')
  return {
    ...actual,
    useVirtualList: (source) => {
      const list = computed(() => {
        const arr = unref(source) || []
        return arr.map((data, index) => ({ data, index }))
      })
      return {
        list,
        containerProps: { ref: null, onScroll: () => {}, style: {} },
        wrapperProps: computed(() => ({ style: {} })),
        scrollTo: () => {}
      }
    }
  }
})

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve()) }
  }
})

const mockRepos = [
  { name: 'repo-a', path: '/work/repo-a', summary: '', tags: ['前端'], readmeSummary: 'A 的 README', missing: false, hasRemote: true, isGitRepo: true },
  { name: 'repo-b', path: '/work/repo-b', summary: '', tags: [], readmeSummary: '', missing: false, hasRemote: false, isGitRepo: true },
  { name: 'repo-c', path: '/work/repo-c', summary: '', tags: ['后端'], readmeSummary: 'C 的 README', missing: false, hasRemote: true, isGitRepo: true },
  { name: 'repo-d', path: '/work/repo-d', summary: '', tags: ['已弃用'], readmeSummary: '', missing: true, hasRemote: false, isGitRepo: true }
]

const mockDirs = [
  { id: 'dir-1', name: '工作目录1', path: '/work' },
  { id: 'dir-2', name: '工作目录2', path: '/other' }
]

vi.mock('../../../wailsjs/go/main/App', () => ({
  // 返回深拷贝，避免 doSave 回写 repo.tags 污染模块级 mockRepos 影响后续测试
  GetRepoFilterList: vi.fn(() => Promise.resolve(mockRepos.map(r => ({ ...r, tags: [...(r.tags || [])] })))),
  RefreshRepoFilterList: vi.fn(() => Promise.resolve(mockRepos.map(r => ({ ...r, tags: [...(r.tags || [])] })))),
  SaveRepoMeta: vi.fn(() => Promise.resolve()),
  CleanMissingRepoMeta: vi.fn(() => Promise.resolve(1)),
  GetRepoReadme: vi.fn(() => Promise.resolve('# 完整 README\n\n正文内容'))
}))

const defaultStubs = {
  // v-bind="$attrs" 透传 class，使主弹窗 .repo-filter-dialog 与二级弹窗 .repo-readme-full-dialog 可区分
  'el-dialog': {
    template: '<div v-if="modelValue" v-bind="$attrs"><slot /></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'appendTobody'],
    emits: ['update:modelValue']
  },
  'el-select': {
    template: '<select class="el-select" :multiple="multiple"><slot /></select>',
    props: ['modelValue', 'multiple', 'placeholder', 'size', 'clearable', 'collapseTags', 'collapseTagsTooltip'],
    emits: ['update:modelValue']
  },
  'el-option': {
    template: '<option :value="value">{{ label }}</option>',
    props: ['label', 'value']
  },
  'el-input': {
    template: `<template v-if="type === 'textarea'"><textarea class="el-input" :value="modelValue" :rows="rows" @input="$emit('update:modelValue', $event.target.value)" /></template><template v-else><input class="el-input" :value="modelValue" :placeholder="placeholder" @input="$emit('update:modelValue', $event.target.value)" @keyup="$emit('keyup', $event)" /></template>`,
    props: ['modelValue', 'placeholder', 'size', 'clearable', 'type', 'rows'],
    emits: ['update:modelValue', 'keyup']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="loading || disabled" @click="$emit(\'click\')"><slot /></button>',
    props: ['loading', 'size', 'type', 'disabled'],
    emits: ['click']
  },
  'el-icon': { template: '<i><slot /></i>' },
  'el-tabs': {
    template: '<div class="el-tabs"><slot /></div>',
    props: ['modelValue'],
    emits: ['update:modelValue', 'tab-click', 'tab-change']
  },
  'el-tab-pane': {
    template: '<div class="el-tab-pane" :data-name="name"><slot name="label" /></div>',
    props: ['label', 'name']
  },
  'el-tag': {
    template: '<span class="el-tag" :data-type="type"><slot /><i v-if="closable" class="tag-close" @click.stop="$emit(\'close\')" /></span>',
    props: { type: String, size: String, closable: { type: Boolean, default: false } },
    emits: ['close']
  },
  'el-empty': { template: '<div class="el-empty" />', props: ['description', 'imageSize'] },
  Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
  Pane: { template: '<div class="pane"><slot /></div>' },
  // stub 掉 FilePreviewRenderer，避免 mermaid / hljs 等副作用，仅断言收到 content
  FilePreviewRenderer: {
    template: '<div class="file-preview-renderer-stub" :data-content="content" />',
    props: ['kind', 'fileName', 'content']
  }
}

function createWrapper(props = {}) {
  return mount(RepoFilterDialog, {
    props: { visible: true, directories: mockDirs, currentDirId: 'dir-1', ...props },
    global: { stubs: defaultStubs }
  })
}

describe('RepoFilterDialog.vue', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  afterEach(() => {
    if (wrapper) {
      wrapper.unmount()
      wrapper = null
    }
    vi.useRealTimers()
  })

  it('弹窗可见时加载仓库列表并渲染', async () => {
    const { GetRepoFilterList } = await import('../../../wailsjs/go/main/App')
    wrapper = createWrapper()
    await flushPromises()
    expect(GetRepoFilterList).toHaveBeenCalledWith('dir-1')
    expect(wrapper.find('.repo-filter-dialog').exists()).toBe(true)
    // 默认选中第一项，右栏应显示仓库名
    expect(wrapper.find('.detail-name').text()).toBe('repo-a')
  })

  it('默认"已编辑"Tab 仅显示有标签的仓库', async () => {
    wrapper = createWrapper()
    await flushPromises()
    const items = wrapper.findAll('.repo-item')
    // repo-a / repo-c / repo-d 有标签，repo-b 无标签被排除
    expect(items.length).toBe(3)
    const names = items.map(el => el.find('.repo-item__name').text())
    expect(names).toEqual(expect.arrayContaining(['repo-a', 'repo-c', 'repo-d']))
    expect(names).not.toContain('repo-b')
  })

  it('切换到"未编辑"Tab 显示无标签的仓库', async () => {
    wrapper = createWrapper()
    await flushPromises()
    const tabs = wrapper.findComponent({ ref: 'repoTabsRef' })
    await tabs.vm.$emit('update:modelValue', 'unedited')
    await flushPromises()
    const items = wrapper.findAll('.repo-item')
    expect(items.length).toBe(1)
    expect(items[0].find('.repo-item__name').text()).toBe('repo-b')
  })

  it('标签 OR 筛选：选多个标签时任一命中即显示', async () => {
    wrapper = createWrapper()
    await flushPromises()
    const select = wrapper.findComponent({ ref: 'tagFilterRef' })
    await select.vm.$emit('update:modelValue', ['前端', '后端'])
    await flushPromises()
    const items = wrapper.findAll('.repo-item')
    // repo-a(前端) + repo-c(后端) 命中，repo-d(已弃用) 被筛掉
    expect(items.length).toBe(2)
    const names = items.map(el => el.find('.repo-item__name').text())
    expect(names).toEqual(expect.arrayContaining(['repo-a', 'repo-c']))
    expect(names).not.toContain('repo-d')
  })

  it('点击跳转按钮 emit locate 事件（携带仓库路径）', async () => {
    wrapper = createWrapper()
    await flushPromises()
    // 默认选中第一项 repo-a
    const jumpBtn = wrapper.findAll('button').find(b => b.text().includes('跳转到文件树'))
    expect(jumpBtn).toBeTruthy()
    await jumpBtn.trigger('click')
    expect(wrapper.emitted('locate')).toBeTruthy()
    expect(wrapper.emitted('locate')[0]).toEqual(['/work/repo-a'])
  })

  it('失效仓库左栏灰显（is-missing class）', async () => {
    wrapper = createWrapper()
    await flushPromises()
    const missingItem = wrapper.findAll('.repo-item').find(el => el.classes().includes('is-missing'))
    expect(missingItem).toBeTruthy()
    expect(missingItem.find('.repo-item__name').text()).toBe('repo-d')
  })

  it('失效仓库跳转按钮禁用，点击不 emit locate 并给出警告', async () => {
    wrapper = createWrapper()
    await flushPromises()
    // 点击 repo-d 选中（onSelect 为 async，需多次 flush 让 selectedPath 与右栏更新）
    const repoDItem = wrapper.findAll('.repo-item').find(el => el.text().includes('repo-d'))
    await repoDItem.trigger('click')
    await flushPromises()
    await nextTick()
    await flushPromises()
    // 右栏选中 repo-d，跳转按钮 disabled 且显示失效提示
    const jumpBtn = wrapper.findAll('button').find(b => b.text().includes('跳转到文件树'))
    expect(jumpBtn.attributes('disabled')).toBeDefined()
    expect(wrapper.find('.detail-missing-hint').exists()).toBe(true)
    // 通过组件层 emit click 绕过 jsdom 下 disabled 按钮不触发 DOM click 的限制
    const jumpBtnComp = wrapper.findComponent({ ref: 'jumpBtnRef' })
    await jumpBtnComp.vm.$emit('click')
    await flushPromises()
    expect(ElMessage.warning).toHaveBeenCalledWith('该仓库路径已失效，无法跳转')
    expect(wrapper.emitted('locate')).toBeFalsy()
  })

  it('删除标签后即时调用 SaveRepoMeta 保存', async () => {
    const { SaveRepoMeta } = await import('../../../wailsjs/go/main/App')
    wrapper = createWrapper()
    await flushPromises()
    // 默认选中 repo-a（标签"前端"），点击其右栏 tag 的 close
    const closeIcons = wrapper.findAll('.detail-tags .tag-close')
    expect(closeIcons.length).toBeGreaterThan(0)
    await closeIcons[0].trigger('click')
    // onRemoveTag 内部 flushPendingSave().then(...) 链式，需多次 flush
    await flushPromises()
    await flushPromises()
    expect(SaveRepoMeta).toHaveBeenCalled()
    // 最后一次保存的 tags 应不含被删标签
    const lastCall = SaveRepoMeta.mock.calls[SaveRepoMeta.mock.calls.length - 1]
    expect(lastCall[0]).toBe('/work/repo-a')
    expect(lastCall[2]).toEqual([])
  })

  it('简述防抖 800ms 后自动保存', async () => {
    const { SaveRepoMeta } = await import('../../../wailsjs/go/main/App')
    wrapper = createWrapper()
    await flushPromises()
    // 确认默认选中 repo-a 且带标签"前端"
    expect(wrapper.findAll('.detail-tags .tag-close').length).toBe(1)
    // 右栏简述 textarea 输入
    const textarea = wrapper.find('textarea')
    await textarea.setValue('新的简述')
    await flushPromises()
    // 防抖未到期，尚未保存
    expect(SaveRepoMeta).not.toHaveBeenCalled()
    // 推进 800ms 触发防抖
    await vi.advanceTimersByTimeAsync(800)
    await flushPromises()
    expect(SaveRepoMeta).toHaveBeenCalledWith('/work/repo-a', '新的简述', ['前端'])
  })

  it('点击清理失效按钮调用 CleanMissingRepoMeta 并刷新列表', async () => {
    const { CleanMissingRepoMeta, GetRepoFilterList } = await import('../../../wailsjs/go/main/App')
    wrapper = createWrapper()
    await flushPromises()
    GetRepoFilterList.mockClear()
    const cleanBtn = wrapper.findAll('button').find(b => b.text().includes('清理失效'))
    await cleanBtn.trigger('click')
    await flushPromises()
    expect(CleanMissingRepoMeta).toHaveBeenCalled()
    // 确认后应刷新列表
    expect(GetRepoFilterList).toHaveBeenCalled()
  })

  it('切换工作目录下拉重新加载列表', async () => {
    const { GetRepoFilterList } = await import('../../../wailsjs/go/main/App')
    wrapper = createWrapper()
    await flushPromises()
    GetRepoFilterList.mockClear()
    // 模拟 el-select 切换工作目录
    const dirSelect = wrapper.findComponent({ ref: 'dirSelectRef' })
    await dirSelect.vm.$emit('update:modelValue', 'dir-2')
    await flushPromises()
    expect(GetRepoFilterList).toHaveBeenCalledWith('dir-2')
  })

  it('切换 Tab 时右栏 detail 跟随新 Tab 首项（优化3）', async () => {
    wrapper = createWrapper()
    await flushPromises()
    // 默认"已编辑"Tab，选中首项 repo-a
    expect(wrapper.find('.detail-name').text()).toBe('repo-a')
    // 切换到"未编辑"Tab
    const tabs = wrapper.findComponent({ ref: 'repoTabsRef' })
    await tabs.vm.$emit('update:modelValue', 'unedited')
    await flushPromises()
    await nextTick()
    await flushPromises()
    // 右栏 detail 应跟随到未编辑 Tab 首项 repo-b（而非残留 repo-a）
    expect(wrapper.find('.detail-name').text()).toBe('repo-b')
  })

  it('首次进入"已编辑"Tab 为空时不显示未编辑仓库（优化2）', async () => {
    const { GetRepoFilterList } = await import('../../../wailsjs/go/main/App')
    // 仅返回无标签仓库：已编辑 Tab 为空，不应选中未编辑仓库
    GetRepoFilterList.mockResolvedValueOnce([
      { name: 'repo-x', path: '/work/repo-x', summary: '', tags: [], readmeSummary: 'X 摘要', missing: false, hasRemote: true, isGitRepo: true }
    ])
    wrapper = createWrapper()
    await flushPromises()
    // 默认 activeTab='edited'，已编辑 Tab 无匹配 -> 左栏无项
    expect(wrapper.findAll('.repo-item').length).toBe(0)
    // 右栏应显示"请从左侧选择"（el-empty），不显示未编辑仓库详情
    expect(wrapper.find('.detail-name').exists()).toBe(false)
    expect(wrapper.find('.el-empty').exists()).toBe(true)
  })

  it('点击"查看完整 README"调用 GetRepoReadme 并打开二级弹窗（优化4d）', async () => {
    const { GetRepoReadme } = await import('../../../wailsjs/go/main/App')
    wrapper = createWrapper()
    await flushPromises()
    // 默认选中 repo-a（有 readmeSummary），按钮可点击
    const btn = wrapper.findAll('button').find(b => b.text().includes('查看完整 README'))
    expect(btn).toBeTruthy()
    expect(btn.attributes('disabled')).toBeUndefined()
    await btn.trigger('click')
    await flushPromises()
    expect(GetRepoReadme).toHaveBeenCalledWith('/work/repo-a')
    // 二级弹窗应可见，FilePreviewRenderer 收到完整 README 文本
    expect(wrapper.find('.repo-readme-full-dialog').exists()).toBe(true)
    const renderer = wrapper.find('.file-preview-renderer-stub')
    expect(renderer.exists()).toBe(true)
    expect(renderer.attributes('data-content')).toBe('# 完整 README\n\n正文内容')
  })

  it('README 摘要为空时"查看完整 README"按钮禁用（优化4d）', async () => {
    const { GetRepoFilterList } = await import('../../../wailsjs/go/main/App')
    // 选中项 readmeSummary 为空 -> 按钮禁用
    GetRepoFilterList.mockResolvedValueOnce([
      { name: 'repo-y', path: '/work/repo-y', summary: '', tags: ['标签'], readmeSummary: '', missing: false, hasRemote: true, isGitRepo: true }
    ])
    wrapper = createWrapper()
    await flushPromises()
    const btn = wrapper.findAll('button').find(b => b.text().includes('查看完整 README'))
    expect(btn.attributes('disabled')).toBeDefined()
  })
})
