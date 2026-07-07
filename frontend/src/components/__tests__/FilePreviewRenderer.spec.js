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

// mermaid 在 jsdom 下无法真实渲染 SVG（依赖 DOM 测量/canvas），stub 掉
vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    run: vi.fn(() => Promise.resolve())
  }
}))

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
      'el-button': { template: '<button v-bind="$attrs"><slot /></button>' },
      // el-tag 真实渲染根元素为 span，stub 保留 class 以便断言数组值徽章
      'el-tag': { template: '<span class="el-tag"><slot /></span>' }
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

describe('FilePreviewRenderer.vue - mermaid 渲染', () => {
  let wrapper

  beforeEach(() => { vi.clearAllMocks() })
  afterEach(() => { if (wrapper) { wrapper.unmount(); wrapper = null } })

  it('mermaid 代码块渲染为 pre.mermaid（不走 highlight.js）', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'diagram.md',
      content: '```mermaid\ngraph TD\nA-->B\n```'
    })
    await flushPromises()
    const mermaidPre = wrapper.find('.markdown-body pre.mermaid')
    expect(mermaidPre.exists()).toBe(true)
    expect(mermaidPre.text()).toContain('graph TD')
  })

  it('调用 mermaid.run 渲染图形节点', async () => {
    const mermaid = (await import('mermaid')).default
    wrapper = mountRenderer({
      kind: 'text', fileName: 'diagram.md',
      content: '```mermaid\ngraph TD\nA-->B\n```'
    })
    await flushPromises()
    expect(mermaid.run).toHaveBeenCalled()
  })

  it('普通代码块仍走 highlight.js（不产生 pre.mermaid）', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'code.md',
      content: '```js\nconst a = 1\n```'
    })
    await flushPromises()
    expect(wrapper.find('.markdown-body pre.mermaid').exists()).toBe(false)
    expect(wrapper.find('.markdown-body pre.hljs').exists()).toBe(true)
  })
})

describe('FilePreviewRenderer.vue - 标题目录 TOC', () => {
  let wrapper

  beforeEach(() => { vi.clearAllMocks() })
  afterEach(() => { if (wrapper) { wrapper.unmount(); wrapper = null } })

  it('默认（showToc=false）不显示 TOC', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '# 一级标题\n## 二级标题'
    })
    await flushPromises()
    expect(wrapper.find('.preview-toc').exists()).toBe(false)
  })

  it('showToc=true 时提取标题并显示 TOC 侧边栏', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md', showToc: true,
      content: '# 一级标题\n## 二级标题\n### 三级标题'
    })
    await flushPromises()
    const toc = wrapper.find('.preview-toc')
    expect(toc.exists()).toBe(true)
    const items = wrapper.findAll('.toc-item')
    expect(items.length).toBe(3)
    expect(items[0].text()).toBe('一级标题')
    expect(items[1].classes()).toContain('toc-level-2')
  })

  it('showToc=true 但无标题时显示空态提示', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'plain.md', showToc: true,
      content: '正文段落，无标题。'
    })
    await flushPromises()
    expect(wrapper.find('.preview-toc').exists()).toBe(true)
    expect(wrapper.findAll('.toc-item').length).toBe(0)
    expect(wrapper.find('.toc-empty').exists()).toBe(true)
  })

  it('点击 TOC 项调用标题 scrollIntoView', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md', showToc: true,
      content: '# 标题A\n## 标题B'
    })
    await flushPromises()
    const headings = wrapper.find('.markdown-body').element.querySelectorAll('h1,h2,h3,h4,h5,h6')
    const spy = vi.fn()
    headings.forEach(h => { h.scrollIntoView = spy })
    await wrapper.findAll('.toc-item')[1].trigger('click')
    expect(spy).toHaveBeenCalled()
  })

  it('点击 X 关闭图标 emit closeToc', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md', showToc: true,
      content: '# 标题A'
    })
    await flushPromises()
    await wrapper.find('.toc-close-icon').trigger('click')
    expect(wrapper.emitted('closeToc')).toBeTruthy()
  })
})

describe('FilePreviewRenderer.vue - YAML frontmatter 属性面板', () => {
  let wrapper

  beforeEach(() => { vi.clearAllMocks() })
  afterEach(() => { if (wrapper) { wrapper.unmount(); wrapper = null } })

  it('含 frontmatter 的 md 显示属性面板，数组值显示为 el-tag', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '---\ntitle: 测试文档\ntags:\n  - vue\n  - wails\n---\n\n# 正文标题\n'
    })
    await flushPromises()
    expect(wrapper.find('.frontmatter-panel').exists()).toBe(true)
    expect(wrapper.find('.fm-table').exists()).toBe(true)
    // key-value 表格行
    const keys = wrapper.findAll('.fm-key')
    expect(keys.length).toBe(2)
    expect(keys[0].text()).toBe('title')
    expect(keys[1].text()).toBe('tags')
    // 标量值原样展示
    expect(wrapper.find('.fm-scalar').text()).toBe('测试文档')
    // 数组值渲染为 el-tag 徽章
    const tags = wrapper.findAll('.fm-value .el-tag')
    expect(tags.length).toBe(2)
    expect(tags[0].text()).toBe('vue')
    expect(tags[1].text()).toBe('wails')
  })

  it('frontmatter 中的 --- 不再被渲染为 <hr>，key: value 不再显示为段落文本', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '---\ntitle: 测试\n---\n\n# 正文\n'
    })
    await flushPromises()
    const content = wrapper.find('.markdown-content')
    // 正文区无 hr（frontmatter 的 --- 不再变 <hr>）
    expect(content.find('hr').exists()).toBe(false)
    // 正文区不含 frontmatter 的 key: value 文本（不再作为段落渲染）
    expect(content.text()).not.toContain('title: 测试')
    // 属性面板展示 key/value
    expect(wrapper.find('.fm-key').text()).toBe('title')
    expect(wrapper.find('.fm-scalar').text()).toBe('测试')
  })

  it('frontmatter 解析失败时降级为 YAML 高亮代码块，不影响正文', async () => {
    // 未闭合引号 → js-yaml 抛异常 → 降级为原文高亮
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '---\ntitle: "unclosed quote\n---\n\n# 正文\n'
    })
    await flushPromises()
    expect(wrapper.find('.fm-fallback').exists()).toBe(true)
    expect(wrapper.find('.fm-fallback-tip').text()).toContain('解析失败')
    // 复用已注册 hljs yaml 高亮代码块
    expect(wrapper.find('.fm-fallback pre.hljs').exists()).toBe(true)
    expect(wrapper.find('.fm-fallback pre.hljs code').exists()).toBe(true)
    // 正文仍正常渲染
    expect(wrapper.find('.markdown-content h1').text()).toBe('正文')
  })

  it('无 frontmatter 的 md 不显示属性面板（无回归）', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '# 标题\n\n正文段落\n'
    })
    await flushPromises()
    expect(wrapper.find('.frontmatter-panel').exists()).toBe(false)
    expect(wrapper.find('.markdown-content h1').text()).toBe('标题')
    expect(wrapper.find('.markdown-content p').text()).toBe('正文段落')
  })

  it('TOC 标题提取不受 frontmatter 影响', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md', showToc: true,
      content: '---\ntitle: 测试\n---\n\n# 一级\n## 二级\n'
    })
    await flushPromises()
    const items = wrapper.findAll('.toc-item')
    expect(items.length).toBe(2)
    expect(items[0].text()).toBe('一级')
    expect(items[1].text()).toBe('二级')
  })

  // ---------- 边界情况 ----------
  it('BOM 开头的 md 仍能识别 frontmatter（strip BOM 后匹配正则）', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '﻿---\ntitle: BOM 文档\n---\n\n# 正文\n'
    })
    await flushPromises()
    expect(wrapper.find('.frontmatter-panel').exists()).toBe(true)
    expect(wrapper.find('.fm-key').text()).toBe('title')
    expect(wrapper.find('.fm-scalar').text()).toBe('BOM 文档')
    // 正文不被 BOM 干扰
    expect(wrapper.find('.markdown-content h1').text()).toBe('正文')
  })

  it('空 frontmatter（---\\n---）不显示属性面板', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '---\n---\n\n# 正文\n'
    })
    await flushPromises()
    // 空 frontmatter 不识别为属性面板
    expect(wrapper.find('.frontmatter-panel').exists()).toBe(false)
    // 正文标题仍正常渲染
    expect(wrapper.find('.markdown-content h1').text()).toBe('正文')
  })

  it('--- 无匹配结束符时不视为 frontmatter，正文不被误吞', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '---\nkey: value\n\n# 正文\n'
    })
    await flushPromises()
    // 无结束 --- 不识别为 frontmatter
    expect(wrapper.find('.frontmatter-panel').exists()).toBe(false)
    // 正文标题仍渲染（不被误吞）
    expect(wrapper.find('.markdown-content h1').text()).toBe('正文')
  })

  it('嵌套对象值降级为 JSON 文本展示', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '---\nauthor:\n  name: liu\n  email: x@x\n---\n\n# 正文\n'
    })
    await flushPromises()
    expect(wrapper.find('.fm-key').text()).toBe('author')
    // 嵌套对象 → JSON.stringify（MVP 不递归子表格）
    expect(wrapper.find('.fm-scalar').text()).toBe(JSON.stringify({ name: 'liu', email: 'x@x' }))
  })

  it('多行字符串值原样展示（保留换行）', async () => {
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '---\ndescription: |\n  第一行\n  第二行\n---\n\n# 正文\n'
    })
    await flushPromises()
    const scalar = wrapper.find('.fm-scalar')
    expect(scalar.exists()).toBe(true)
    // 多行字符串原样展示，换行由 .fm-scalar { white-space: pre-wrap } 保留
    expect(scalar.text()).toContain('第一行')
    expect(scalar.text()).toContain('第二行')
  })

  it('frontmatter 顶层为非对象结构（数组/标量/null）时降级为代码块', async () => {
    // 顶层是数组 → js-yaml 返回数组 → 非普通对象 → 降级为原文高亮
    wrapper = mountRenderer({
      kind: 'text', fileName: 'doc.md',
      content: '---\n[1,2,3]\n---\n\n# 正文\n'
    })
    await flushPromises()
    expect(wrapper.find('.fm-fallback').exists()).toBe(true)
    expect(wrapper.find('.fm-fallback pre.hljs').exists()).toBe(true)
    // 正文仍正常渲染
    expect(wrapper.find('.markdown-content h1').text()).toBe('正文')
  })
})
