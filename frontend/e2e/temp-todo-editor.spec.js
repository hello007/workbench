/**
 * 临时待办全屏编辑 E2E：真实 Element Plus 弹窗下的聚焦、快捷键与关闭分流。
 *
 * 链路：活动栏切入「临时待办」→ 条目「编辑」进入行内编辑 → 「全屏编辑」打开
 * `el-dialog fullscreen` → 断言焦点与光标位置 → 分别走 `Ctrl+S` 保存、
 * `ESC` 取消整条编辑、右上角 `X` 保留草稿三条出口。
 *
 * 与组件单测的分工：单测用 stub 替换 el-dialog 并手工 $emit('opened'/'closed')，
 * 只能证明「调用了 setSelectionRange」与「标记位传参」；本文件验证真实浏览器中
 * `@opened` 动画结束后 document.activeElement 确为正文 textarea、光标确在末尾，
 * 以及 ESC 由 Element Plus 的 document 级监听关闭后 `@closed` 的复位分支。
 *
 * fixture 设计：ListTempTodos 返回一条含中文正文与标签的条目，使「光标在末尾」
 * 与「位置 0」在数值上可区分；UpdateTempTodo 沿用默认值（保存成功路径不消费
 * 返回值），入参经 getWailsCalls 断言。
 */
import { test, expect, getWailsCalls, openPanel } from './fixtures'

const TODO_ID = 'todo-editor-e2e'
const TODO_TEXT = '核对渠道网关的重试与熔断策略'
const TODO_TAGS = ['架构']

/** 形状对齐 model.TempTodo：createdAt 毫秒，completedAt 为 null 表示未完成 */
const TODO = {
  id: TODO_ID,
  text: TODO_TEXT,
  tags: TODO_TAGS,
  createdAt: 1735689600000,
  completedAt: null
}

const BASE_OVERRIDES = { ListTempTodos: [TODO] }

/** 打开首页并切到临时待办面板，等条目渲染（确认 ListTempTodos 已 resolve） */
async function openTodoPanel(page) {
  await page.goto('/')
  await openPanel(page, 'todo')
  await expect(page.locator('.temp-todo-panel').locator(`[data-todo-id="${TODO_ID}"]`)).toBeVisible()
}

/**
 * 进入行内编辑并打开全屏弹窗，返回弹窗定位器。
 *
 * 列表项图标按钮被 el-tooltip（show-after 300ms）包裹，点击后鼠标驻留会让浮层
 * 常驻并遮挡后续点击，故每次点击后移开鼠标（先例：prompt-library.spec.js）。
 */
async function openEditorDialog(page) {
  const item = page.locator(`[data-todo-id="${TODO_ID}"]`)
  await item.locator('[data-action="edit"]').click()
  await page.mouse.move(0, 0)

  await item.locator('[data-action="expand-editor"]').click()
  await page.mouse.move(0, 0)

  const dialog = page.locator('.todo-editor-dialog')
  await expect(dialog).toBeVisible()
  return dialog
}

test.describe('临时待办全屏编辑：打开聚焦与光标位置', () => {
  test.use({ wailsOverrides: BASE_OVERRIDES })

  test('打开后焦点在正文 textarea，光标位于原文末尾', async ({ page }) => {
    await openTodoPanel(page)
    const dialog = await openEditorDialog(page)

    const textarea = dialog.locator('textarea')
    await expect(textarea).toHaveValue(TODO_TEXT)
    // @opened 触发于打开动画结束后，toBeFocused 自动重试覆盖该时序
    await expect(textarea).toBeFocused()

    // 光标位置只能在真实浏览器里读取选择区；单测只能断言 setSelectionRange 入参
    expect(
      await textarea.evaluate(el => ({ start: el.selectionStart, end: el.selectionEnd }))
    ).toEqual({ start: TODO_TEXT.length, end: TODO_TEXT.length })

    // 首次打开时浏览器对「新元素赋值 + focus」默认就把光标放在末尾（实测 Chromium
    // 赋值后 selectionStart 即为 value.length），上面的断言只证明焦点就位，不证明
    // setSelectionRange 生效。把光标挪到 0 后经 X 关闭（草稿保留、弹窗 DOM 复用、
    // 正文值不变）再打开：此时仅 focus 会恢复失焦前的 0，只有 @opened 真正把光标
    // 定位到末尾才通过。expando 断言钉住「弹窗 DOM 复用」前提（EP destroy-on-close
    // 默认 false）：节点一旦被重建，值会重新赋值、光标默认回末尾，鉴别力随之失效。
    // 关闭必须走 X：ESC 会复位整条编辑并销毁编辑器，无法复用节点重开弹窗；
    // 「X 保留草稿」的语义由下方独立用例守护，本步骤只借它制造复用状态。
    await textarea.evaluate(el => {
      el.__e2eCaretProbe = 'kept'
      el.setSelectionRange(0, 0)
    })
    await dialog.locator('.el-dialog__headerbtn').click()
    await expect(dialog).toBeHidden()

    await page.locator(`[data-todo-id="${TODO_ID}"] [data-action="expand-editor"]`).click()
    await page.mouse.move(0, 0)
    await expect(dialog).toBeVisible()
    await expect(textarea).toBeFocused()
    expect(await textarea.evaluate(el => el.__e2eCaretProbe)).toBe('kept')
    expect(
      await textarea.evaluate(el => ({ start: el.selectionStart, end: el.selectionEnd }))
    ).toEqual({ start: TODO_TEXT.length, end: TODO_TEXT.length })
  })
})

test.describe('临时待办全屏编辑：Ctrl+S 保存', () => {
  test.use({ wailsOverrides: BASE_OVERRIDES })

  test('弹窗内 Ctrl+S 调用 UpdateTempTodo 并退出编辑态', async ({ page }) => {
    await openTodoPanel(page)
    const dialog = await openEditorDialog(page)

    const textarea = dialog.locator('textarea')
    await expect(textarea).toBeFocused()
    await textarea.fill('核对渠道网关的重试、熔断与降级策略')

    await page.keyboard.press('Control+s')

    await expect(dialog).toBeHidden()
    // 保存成功退出编辑态：行内 textarea 随 v-if 真实移除
    await expect(page.locator('.todo-item textarea')).toHaveCount(0)

    expect(await getWailsCalls(page, 'UpdateTempTodo')).toEqual([
      {
        method: 'UpdateTempTodo',
        args: [TODO_ID, '核对渠道网关的重试、熔断与降级策略', TODO_TAGS]
      }
    ])
  })
})

test.describe('临时待办全屏编辑：ESC 取消整条编辑', () => {
  test.use({ wailsOverrides: BASE_OVERRIDES })

  test('ESC 关闭弹窗并丢弃草稿，回到条目展示态', async ({ page }) => {
    await openTodoPanel(page)
    const dialog = await openEditorDialog(page)

    const textarea = dialog.locator('textarea')
    await expect(textarea).toBeFocused()
    await textarea.fill('ESC 丢弃的草稿')

    await page.keyboard.press('Escape')

    // EP 默认行为关闭弹窗，@closed 依 editorDialogCanceled 复位草稿
    await expect(dialog).toBeHidden()
    await expect(page.locator(`[data-todo-id="${TODO_ID}"] .todo-editor`)).toHaveCount(0)
    await expect(page.locator(`[data-todo-id="${TODO_ID}"] .todo-text`)).toHaveText(TODO_TEXT)
    expect(await getWailsCalls(page, 'UpdateTempTodo')).toHaveLength(0)
  })
})

test.describe('临时待办全屏编辑：X 关闭保留草稿', () => {
  test.use({ wailsOverrides: BASE_OVERRIDES })

  test('右上角 X 只关闭弹窗，行内编辑与草稿原样保留', async ({ page }) => {
    await openTodoPanel(page)
    const dialog = await openEditorDialog(page)

    const textarea = dialog.locator('textarea')
    await expect(textarea).toBeFocused()
    await textarea.fill('X 关闭后保留的草稿')

    await dialog.locator('.el-dialog__headerbtn').click()

    await expect(dialog).toBeHidden()
    // 与 ESC 分流：不置取消标记，回到行内编辑而不是条目展示态
    const inlineEditor = page.locator(`[data-todo-id="${TODO_ID}"] .todo-editor--inline`)
    await expect(inlineEditor).toBeVisible()
    await expect(inlineEditor.locator('textarea')).toHaveValue('X 关闭后保留的草稿')
    expect(await getWailsCalls(page, 'UpdateTempTodo')).toHaveLength(0)
  })
})
