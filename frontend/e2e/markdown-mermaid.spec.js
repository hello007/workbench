/**
 * markdown 预览 mermaid 渲染 E2E 冒烟（mermaid 12 升级专项，PRD 验收项）。
 *
 * 背景：mermaid 11.17.2 → 12.2.0 major 升级后，markdown 预览内嵌图表链路
 * （markdown-it highlight 钩子输出 <pre class="mermaid"> → DOM 更新后动态
 * import mermaid 并 mermaid.run 渲染为 SVG）必须保持真实出图。jsdom 无布局
 * 引擎测不了真实渲染（vitest 面被 mock 屏蔽），本用例在真实 Chromium 覆盖
 * 三类代表性图同屏渲染：流程图（dagre 布局管线）、时序图（mermaid 12 重构
 * 的渲染管线）、甘特图（dayjs 日期解析）——SVG 真实生成即证明 mermaid 12
 * 动态 import、initialize（layout:'dagre' / look:'classic' 视觉保持双键）
 * 与 run 渲染管线全线贯通。
 *
 * 链路：单击文件树 README.md 节点 → Home.onNodeSelect（type==='file' 直接
 * previewFile）→ PreviewFile(mock) 返回含三个 mermaid 代码块的 markdown →
 * FilePreviewRenderer markdown 分支（kind='text' + 扩展名 md）→ mermaid.run
 * 出 SVG。
 */
import { test, expect, getWailsCalls } from './fixtures'
import { E2E_REPO_PATH, GIT_REPO_DIRECTORY, GIT_FLOW_OVERRIDES } from './git-fixtures'

/** 含三个 mermaid 代码块的 markdown 文本（流程图 / 时序图 / 甘特图各一；甘特图日期用最简 ISO 形态避免解析歧义） */
const MERMAID_MD = [
  '# mermaid 升级冒烟',
  '',
  '流程图：',
  '',
  '```mermaid',
  'flowchart TD',
  '    A[开始] --> B[结束]',
  '```',
  '',
  '时序图：',
  '',
  '```mermaid',
  'sequenceDiagram',
  '    Alice->>Bob: 你好',
  '    Bob-->>Alice: 收到',
  '```',
  '',
  '甘特图：',
  '',
  '```mermaid',
  'gantt',
  '    title 简单计划',
  '    dateFormat YYYY-MM-DD',
  '    section 开发',
  '    任务一 :a1, 2026-01-01, 10d',
  '```',
  ''
].join('\n')

test.describe('markdown 预览 mermaid 渲染冒烟', () => {
  // PreviewFile 返回形状对齐 model.FilePreview 真实字段（content/error/kind），
  // kind='text' 是 markdown 分流必要字段（前端 isMarkdown 判定 props.kind==='text'）
  test.use({
    wailsOverrides: {
      ...GIT_FLOW_OVERRIDES,
      PreviewFile: { content: MERMAID_MD, error: '', kind: 'text' }
    }
  })

  test('三类图同屏渲染：三个 mermaid 代码块均真实产出 SVG', async ({ page }) => {
    // 前置：打开首页选中工作目录，等文件树根节点渲染（同 file-tree.spec 的 openFileTreePage）
    await page.setViewportSize({ width: 1920, height: 1080 })
    await page.goto('/')
    const dirItem = page.locator('.dir-item', { hasText: GIT_REPO_DIRECTORY.name })
    await expect(dirItem).toBeVisible()
    await dirItem.click()
    const readMeNode = page.locator('.el-tree-node', { hasText: 'README.md' })
    await expect(readMeNode).toBeVisible()

    // 普通树节点单击即触发预览（FileTreePanel.onNodeClick emit select，非双击语义）
    await readMeNode.click()

    // 预览链路目标正确：PreviewFile 收到 README.md 完整路径
    // （onNodeSelect 同步调 previewFile，click 返回后调用记录已就位）
    const previewCalls = await getWailsCalls(page, 'PreviewFile')
    expect(previewCalls[0].args[0]).toBe(`${E2E_REPO_PATH}/README.md`)

    // mermaid.run 异步（动态 import + 渲染），expect 自动重试直至 SVG 全部就位，禁 waitForTimeout。
    // svg 选择器只匹配渲染产物（mermaid.run 前节点内是纯文本），count=3 即三个图全部真实出图
    const svgs = page.locator('.markdown-body pre.mermaid svg')
    await expect(svgs).toHaveCount(3)
    for (let i = 0; i < 3; i++) {
      const svg = svgs.nth(i)
      await expect(svg).toBeVisible()
      // viewBox 是 mermaid 渲染管线产物特征（真实布局坐标写入），文本态不存在该元素
      await expect(svg).toHaveAttribute('viewBox', /.+/)
    }
  })
})
