/**
 * FilePreviewRenderer.vue 右键复制菜单测试
 * 覆盖：仅 text/markdown 启用、复制项禁用态、复制写入剪贴板、全选关闭菜单
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ElMessage } from 'element-plus'
import FilePreviewRenderer from '../FilePreviewRenderer.vue'

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() }
  }
})

// Office 依赖在 jsdom 下真实加载易崩，stub 掉
vi.mock('docx-preview', () => ({ renderAsync: vi.fn(() => Promise.resolve()) }))

// markdown 外部链接拦截后走 runtime.BrowserOpenURL，stub 掉避免 jsdom 无 window.runtime
vi.mock('../../../wailsjs/runtime/runtime', () => ({
  BrowserOpenURL: vi.fn()
}))
vi.mock('xlsx', () => ({
  read: vi.fn(() => ({ SheetNames: [], Sheets: {} })),
  utils: { sheet_to_json: vi.fn(() => []) }
}))

// CodeMirror 6 依赖 ResizeObserver，jsdom 未实现，补空实现避免构造报错
class ResizeObserverMock { observe() {} unobserve() {} disconnect() {} }
global.ResizeObserver = global.ResizeObserver || ResizeObserverMock

const mountRenderer = (props) => mount(FilePreviewRenderer, {
  props,
  global: {
    stubs: {
      'el-icon': { template: '<i><slot /></i>' },
      'el-button': { template: '<button v-bind="$attrs"><slot /></button>' }
    }
  }
})

describe('FilePreviewRenderer.vue - markdown 链接点击分发', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
  })

  it('点击 ./ 相对链接 → emit openLink 带解析后的绝对路径', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'intro.md',
      filePath: 'D:/repo/docs/intro.md',
      content: '[other](./other.md)'
    })
    await flushPromises()
    await wrapper.find('.markdown-body a').trigger('click')
    expect(wrapper.emitted('openLink')).toEqual([['D:/repo/docs/other.md']])
  })

  it('点击 ../ 链接 → 正确解析到上级目录', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'intro.md',
      filePath: 'D:/repo/docs/intro.md',
      content: '[readme](../README.md)'
    })
    await flushPromises()
    await wrapper.find('.markdown-body a').trigger('click')
    expect(wrapper.emitted('openLink')).toEqual([['D:/repo/README.md']])
  })

  it('filePath 含 Windows 反斜杠时也按正斜杠规范化解析', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'intro.md',
      filePath: 'D:\\repo\\docs\\intro.md',
      content: '[other](sub/other.md)'
    })
    await flushPromises()
    await wrapper.find('.markdown-body a').trigger('click')
    expect(wrapper.emitted('openLink')).toEqual([['D:/repo/docs/sub/other.md']])
  })

  it('点击 http(s) 外部链接 → 调用 BrowserOpenURL 且不 emit openLink', async () => {
    const { BrowserOpenURL } = await import('../../../wailsjs/runtime/runtime')
    wrapper = mountRenderer({
      kind: 'text', fileName: 'intro.md',
      filePath: 'D:/repo/docs/intro.md',
      content: '[ext](https://example.com)'
    })
    await flushPromises()
    await wrapper.find('.markdown-body a').trigger('click')
    expect(BrowserOpenURL).toHaveBeenCalledWith('https://example.com')
    expect(wrapper.emitted('openLink')).toBeFalsy()
  })

  it('点击 #锚点 → 对应标题 scrollIntoView 定位', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'intro.md',
      filePath: 'D:/repo/docs/intro.md',
      content: '[go](#intro)\n\n# Intro'
    })
    await flushPromises()
    const h1 = wrapper.find('.markdown-body h1').element
    h1.scrollIntoView = vi.fn()
    await wrapper.find('.markdown-body a').trigger('click')
    expect(h1.scrollIntoView).toHaveBeenCalled()
  })

  // markdown-it 默认 normalizeLink 会把链接里的中文等非 ASCII 做 percent-encoding，
  // 中文文件名链接需先 decode 还原为原始字符，再解析路径，否则 Windows 按 percent
  // 编码字面名找不到中文文件名。
  it('中文文件名相对链接 → 还原 percent-encoding 后正确解析', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'README.md',
      filePath: 'D:/repo/README.md',
      content: '[功能说明](docs/功能说明.md)'
    })
    await flushPromises()
    await wrapper.find('.markdown-body a').trigger('click')
    expect(wrapper.emitted('openLink')).toEqual([['D:/repo/docs/功能说明.md']])
  })
})

describe('FilePreviewRenderer.vue - 右键复制菜单', () => {
  let wrapper

  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    if (wrapper) { wrapper.unmount(); wrapper = null }
    vi.restoreAllMocks()
  })

  const openMenu = async (clientX = 10, clientY = 10) => {
    await wrapper.find('.file-preview-renderer').trigger('contextmenu', { clientX, clientY })
    await flushPromises()
  }

  it('kind=text 右键显示复制菜单，含"复制"与"全选"', async () => {
    wrapper = mountRenderer({ kind: 'text', fileName: 'a.js', content: 'hello world' })
    await flushPromises()
    expect(wrapper.find('.context-menu').exists()).toBe(false)

    await openMenu()

    const menu = wrapper.find('.context-menu')
    expect(menu.exists()).toBe(true)
    const items = wrapper.findAll('.context-menu-item')
    expect(items).toHaveLength(2)
    expect(items[0].text()).toContain('复制')
    expect(items[1].text()).toContain('全选')
  })

  it('kind=image 右键不显示复制菜单（保留原生右键）', async () => {
    wrapper = mountRenderer({ kind: 'image', fileName: 'a.png', base64: '' })
    await flushPromises()

    await openMenu()

    expect(wrapper.find('.context-menu').exists()).toBe(false)
  })

  it('无选中文本时"复制"项禁用', async () => {
    vi.spyOn(window, 'getSelection').mockReturnValue({ toString: () => '' })
    wrapper = mountRenderer({ kind: 'text', fileName: 'a.js', content: 'hello' })
    await flushPromises()

    await openMenu()

    expect(wrapper.findAll('.context-menu-item')[0].classes()).toContain('is-disabled')
  })

  it('有选中文本时点击"复制"写入剪贴板并提示成功', async () => {
    wrapper = mountRenderer({ kind: 'text', fileName: 'a.js', content: 'hello' })
    await flushPromises()

    // mock 选区：anchorNode 落在 CodeMirror 宿主内，使预览区内选区判定为真
    const cmHostEl = wrapper.find('.cm-host').element
    vi.spyOn(window, 'getSelection').mockReturnValue({
      toString: () => '选中的代码片段',
      anchorNode: cmHostEl
    })
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    await openMenu()

    const copyItem = wrapper.findAll('.context-menu-item')[0]
    expect(copyItem.classes()).not.toContain('is-disabled')

    await copyItem.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('选中的代码片段')
    expect(ElMessage.success).toHaveBeenCalled()
  })

  it('点击"全选"后菜单关闭', async () => {
    wrapper = mountRenderer({ kind: 'text', fileName: 'a.js', content: 'hello' })
    await flushPromises()

    await openMenu()
    expect(wrapper.find('.context-menu').exists()).toBe(true)

    await wrapper.findAll('.context-menu-item')[1].trigger('click')
    await flushPromises()

    expect(wrapper.find('.context-menu').exists()).toBe(false)
  })

  // 复制前对选中文本做 trim：前后空白应被去除后再写入剪贴板
  it('选中文本前后含空格时，复制写入剪贴板前做 trim', async () => {
    wrapper = mountRenderer({ kind: 'text', fileName: 'a.js', content: 'hello world' })
    await flushPromises()

    // mock 选区：anchorNode 落在 CodeMirror 宿主内，使预览区内选区判定为真
    const cmHostEl = wrapper.find('.cm-host').element
    vi.spyOn(window, 'getSelection').mockReturnValue({
      toString: () => '  hello world  ',
      anchorNode: cmHostEl
    })
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    await openMenu()

    const copyItem = wrapper.findAll('.context-menu-item')[0]
    expect(copyItem.classes()).not.toContain('is-disabled')

    await copyItem.trigger('click')
    await flushPromises()

    // trim 后写入剪贴板的应为 'hello world'
    expect(writeText).toHaveBeenCalledWith('hello world')
    expect(ElMessage.success).toHaveBeenCalled()
  })

  // 缓存回退：先「全选」缓存选区文本，右键 mousedown 清除了 DOM 选区后，
  // 再次右键「复制」仍应可用（canCopy=true，复制项非禁用）。
  it('右键清除 DOM 选区后，缓存回退使"复制"项仍可用', async () => {
    wrapper = mountRenderer({ kind: 'text', fileName: 'a.js', content: 'hello world' })
    await flushPromises()

    // 取 CodeMirror 宿主元素作为选区 anchorNode，使 contains 判定为真
    const cmHostEl = wrapper.find('.cm-host').element

    // 第一阶段：selectionchange 触发时 DOM 选区非空且 anchorNode 在预览区内 → 缓存文本
    vi.spyOn(window, 'getSelection').mockReturnValue({
      toString: () => 'hello world',
      anchorNode: cmHostEl
    })
    document.dispatchEvent(new Event('selectionchange'))
    await flushPromises()

    // 第二阶段：右键 mousedown 清除了 DOM 选区 → contextmenu 触发时读到空
    vi.spyOn(window, 'getSelection').mockReturnValue({
      toString: () => '',
      anchorNode: null
    })

    await openMenu()

    // 缓存回退：复制项不应被禁用
    const copyItem = wrapper.findAll('.context-menu-item')[0]
    expect(copyItem.classes()).not.toContain('is-disabled')

    // 点击复制应使用缓存的文本写入剪贴板
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    await copyItem.trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('hello world')
    expect(ElMessage.success).toHaveBeenCalled()
  })
})
