/**
 * 临时待办 Markdown 导入 E2E（三期）。
 *
 * 链路：活动栏切入「临时待办」→ 导入按钮 → OpenFileDialog 选文件 →
 * ReadFileBytes 读原始字节 → 前端 parseTempTodoMarkdown 解析 → 预览汇总 →
 * 确认 AddTempTodos 一次落库。
 *
 * 覆盖用户指定的三条契约：
 * 1. 预览汇总数字（将导入 N 条 / 跳过重复 M 条）
 * 2. 「重复项也导入」开关切换后的计数变化
 * 3. 确认后 AddTempTodos 的入参（形状对齐 model.TempTodoDraft）
 *
 * fixture 设计：文件名取历史日期 2020-01-05.md，使行内时间按「文件名日期 + 时刻」
 * 解析且恒早于当前时刻，规避 parseTempTodoMarkdown 的未来时刻钳制（否则 createdAt
 * 会随本地运行时钟漂移）；内容同时覆盖带标签条目、续行多行正文、导出格式已完成行、
 * 普通列表项、与已存待办重复行、文件内自身重复行。
 */
import { test, expect, getWailsCalls, openPanel } from './fixtures'

const FILE_PATH = 'D:/e2e-import/2020-01-05.md'

/** 已存待办：正文与 fixture 第一行相同，用于触发跨文件重复判定 */
const EXISTING_TODOS = [
  { id: 'todo-existing', text: '已有的事', tags: [], createdAt: 1735689600000, completedAt: null }
]

/**
 * 导入 fixture：5 条可识别条目，其中 2 条重复（1 条与已存待办重复、1 条文件内重复），
 * 默认过滤重复后应导入 3 条。
 */
const IMPORT_MARKDOWN = [
  '# 2020-01-05',
  '',
  '- 10:15 已有的事',
  '- 11:30 新增任务  #工作',
  '- 08:05 多行任务',
  '  第二行',
  '- 13:00 已完成导出项 （已完成 13:30）',
  '- 普通列表项',
  '- 12:45 文件内重复',
  '- 12:46 文件内重复',
  ''
].join('\n')

const READ_BYTES = { base64: Buffer.from(IMPORT_MARKDOWN, 'utf-8').toString('base64'), tooLarge: false }

/** 共用 mock 基底：文件选择固定路径 + 读取返回 fixture 文本 */
const BASE_OVERRIDES = {
  ListTempTodos: EXISTING_TODOS,
  OpenFileDialog: FILE_PATH,
  ReadFileBytes: READ_BYTES
}

/** 打开首页并切到临时待办面板，等已存待办渲染（确认 ListTempTodos 已 resolve） */
async function openTodoPanel(page) {
  await page.goto('/')
  await openPanel(page, 'todo')
  await expect(page.locator('.temp-todo-panel').getByText('已有的事')).toBeVisible()
}

/** 点击导入并返回预览对话框定位器 */
async function openImportPreview(page) {
  await page.locator('.temp-todo-panel [data-action="import"]').click()
  const dialog = page.locator('.todo-import-dialog')
  await expect(dialog).toBeVisible()
  return dialog
}

test.describe('临时待办 Markdown 导入：预览汇总与重复开关', () => {
  test.use({ wailsOverrides: BASE_OVERRIDES })

  test('预览汇总数字随「重复项也导入」开关切换', async ({ page }) => {
    await openTodoPanel(page)
    const dialog = await openImportPreview(page)

    // 文件桥接参数：选文件 → 按所选路径读取原始字节
    expect(await getWailsCalls(page, 'OpenFileDialog')).toEqual([
      { method: 'OpenFileDialog', args: ['选择待办 Markdown 文件', [{ DisplayName: 'Markdown 文件', Pattern: '*.md' }]] }
    ])
    expect((await getWailsCalls(page, 'ReadFileBytes'))[0].args[0]).toBe(FILE_PATH)

    // 默认过滤重复：3 条可导入、2 条重复
    const summary = dialog.locator('.import-summary')
    await expect(summary).toContainText('将导入 3 条')
    await expect(summary).toContainText('跳过重复 2 条')
    // 重复项在预览列表中带「已存在」标记
    await expect(dialog.locator('.import-item.is-duplicate')).toHaveCount(2)

    // 打开「重复项也导入」：5 条全部可导入，跳过重复块消失
    await dialog.locator('.import-switch').click()
    await expect(summary).toContainText('将导入 5 条')
    await expect(dialog.locator('.import-summary-dup')).toHaveCount(0)

    // 再关回：恢复 3/2
    await dialog.locator('.import-switch').click()
    await expect(summary).toContainText('将导入 3 条')
    await expect(summary).toContainText('跳过重复 2 条')
  })
})

test.describe('临时待办 Markdown 导入：确认写入入参', () => {
  test.use({ wailsOverrides: { ...BASE_OVERRIDES, AddTempTodos: 3 } })

  test('默认过滤重复时 AddTempTodos 收到 3 条草稿', async ({ page }) => {
    await openTodoPanel(page)
    const dialog = await openImportPreview(page)

    await dialog.locator('[data-action="import-confirm"]').click()

    // 成功提示条数与 mock 返回一致，对话框关闭
    await expect(page.locator('.el-message--success')).toContainText('已导入 3 条待办')
    await expect(dialog).not.toBeVisible()

    // 入参形状对齐 model.TempTodoDraft：text/tags/createdAt；
    // createdAt = 文件名日期 2020-01-05 + 行内时刻（本地时区毫秒）
    const calls = await getWailsCalls(page, 'AddTempTodos')
    expect(calls).toHaveLength(1)
    expect(calls[0].args[0]).toEqual([
      { text: '新增任务', tags: ['工作'], createdAt: new Date(2020, 0, 5, 11, 30).getTime() },
      { text: '多行任务\n第二行', tags: [], createdAt: new Date(2020, 0, 5, 8, 5).getTime() },
      { text: '文件内重复', tags: [], createdAt: new Date(2020, 0, 5, 12, 45).getTime() }
    ])
  })
})

test.describe('临时待办 Markdown 导入：重复项也写入', () => {
  test.use({ wailsOverrides: { ...BASE_OVERRIDES, AddTempTodos: 5 } })

  test('打开重复开关后确认，入参包含重复项（5 条）', async ({ page }) => {
    await openTodoPanel(page)
    const dialog = await openImportPreview(page)

    await dialog.locator('.import-switch').click()
    await expect(dialog.locator('.import-summary')).toContainText('将导入 5 条')
    await dialog.locator('[data-action="import-confirm"]').click()

    await expect(page.locator('.el-message--success')).toContainText('已导入 5 条待办')

    const calls = await getWailsCalls(page, 'AddTempTodos')
    expect(calls).toHaveLength(1)
    expect(calls[0].args[0].map(item => item.text)).toEqual([
      '已有的事',
      '新增任务',
      '多行任务\n第二行',
      '文件内重复',
      '文件内重复'
    ])
    // 每条都只含 TempTodoDraft 三个字段（不带前端 duplicate 标记）
    for (const item of calls[0].args[0]) {
      expect(Object.keys(item).sort()).toEqual(['createdAt', 'tags', 'text'])
    }
  })
})
