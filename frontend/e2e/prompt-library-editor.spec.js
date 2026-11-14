/**
 * 提示词编辑弹窗 E2E：真实 Element Plus 弹窗下的聚焦、快捷键与关闭复位。
 *
 * 链路：活动栏切入「提示词」→ 条目「编辑」打开弹窗 → 断言焦点在标题输入框且
 * 光标位于原标题末尾 → `Ctrl+S` 保存 → 另起用例验证 `ESC` 与右上角 `X`
 * 均取消本次编辑（统一经 `@closed="resetEditorDraft"` 复位草稿）。
 *
 * 与组件单测的分工：单测只能断言 `setSelectionRange` 被调用与手工 $emit 的
 * 标记分支；本文件验证真实浏览器中 `@opened` 动画结束后 document.activeElement
 * 确为标题 input、光标确在末尾，以及 EP 默认 ESC/X 关闭链路是否触发草稿复位。
 *
 * 与 prompt-library.spec.js 的分工：后者覆盖新建、占位符复制与删除的完整业务链；
 * 本文件只聚焦编辑弹窗的键盘与焦点交互，避免在既有长用例里堆叠断言。
 *
 * 光标断言的鉴别力边界：编辑弹窗 `@closed` 复位草稿、重开时重新赋 draftTitle，
 * 节点即使复用值也已重写，浏览器默认末尾与 setSelectionRange 同形——因此本文件的
 * 「光标位于原标题末尾 / 空串位置 0」只守焦点就位，不构成 setSelectionRange 鉴别；
 * caret 入参的鉴别职责由 PromptLibraryPanel.spec.js 的 spyOn 承担
 * （判定表与记录口径见 docs/spec/e2e-testing.md §13）。
 */
import { test, expect, getWailsCalls, openPanel } from './fixtures'

/** 16 位小写 hex，对齐 model.Prompt 的 ID 生成口径 */
const PROMPT_ID = 'abcdef0123456789'
const PROMPT_TITLE = '代码评审话术'
const PROMPT_CONTENT = '请评审以下 {{模块}} 代码'

/** 形状对齐 model.Prompt：毫秒时间戳，写操作返回 void */
const PROMPT = {
  id: PROMPT_ID,
  title: PROMPT_TITLE,
  content: PROMPT_CONTENT,
  createdAt: 1735689600000,
  updatedAt: 1735689600000
}

const BASE_OVERRIDES = { ListPrompts: [PROMPT] }

/** 打开首页并切到提示词面板，等条目渲染（确认 ListPrompts 已 resolve） */
async function openPromptPanel(page) {
  await page.goto('/')
  await openPanel(page, 'prompts')
  await expect(page.locator('.prompt-panel')).toBeVisible()
  await expect(page.locator(`[data-prompt-id="${PROMPT_ID}"]`)).toBeVisible()
}

/**
 * 按标题文本限定编辑弹窗。
 *
 * 弹窗 append-to-body 渲染在 body 下（.prompt-panel 之外），同一面板还渲染「填写
 * 占位符」弹窗（EP 2.14 关闭后实例驻留 DOM），故按标题文本限定编辑弹窗，不用裸
 * .el-dialog，避免多弹窗并存时 strict mode 冲突。
 */
function editorDialog(page, title) {
  return page.locator('.el-dialog').filter({ hasText: title })
}

/**
 * 点击条目「编辑」并返回弹窗定位器。
 *
 * 列表项图标按钮被 el-tooltip（show-after 300ms）包裹，点击后鼠标驻留会让浮层
 * 常驻并遮挡后续操作，点完即移开鼠标（先例：prompt-library.spec.js）。
 */
async function openEditDialog(page) {
  await page.locator(`[data-prompt-id="${PROMPT_ID}"] [data-action="edit"]`).click()
  await page.mouse.move(0, 0)

  const dialog = editorDialog(page, '编辑提示词')
  await expect(dialog).toBeVisible()
  return dialog
}

test.describe('提示词编辑弹窗：打开聚焦与 Ctrl+S 保存', () => {
  test.use({ wailsOverrides: BASE_OVERRIDES })

  test('编辑时焦点在标题 input 且光标位于原标题末尾，Ctrl+S 保存新正文', async ({ page }) => {
    await openPromptPanel(page)
    const dialog = await openEditDialog(page)

    const title = dialog.locator('[data-field="title"]')
    await expect(title).toHaveValue(PROMPT_TITLE)
    // @opened 触发于打开动画结束后，toBeFocused 自动重试覆盖该时序
    await expect(title).toBeFocused()
    // 光标位置只能在真实浏览器里读取选择区
    expect(
      await title.evaluate(el => ({ start: el.selectionStart, end: el.selectionEnd }))
    ).toEqual({ start: PROMPT_TITLE.length, end: PROMPT_TITLE.length })

    // 焦点移到正文后按 Ctrl+S：容器级监听覆盖标题与正文两个输入框
    await dialog.locator('[data-field="content"]').fill('请评审以下 {{模块}} 代码并给出风险清单')
    await page.keyboard.press('Control+s')

    await expect(dialog).toBeHidden()
    expect(await getWailsCalls(page, 'UpdatePrompt')).toEqual([
      {
        method: 'UpdatePrompt',
        args: [PROMPT_ID, PROMPT_TITLE, '请评审以下 {{模块}} 代码并给出风险清单']
      }
    ])
  })
})

test.describe('提示词新建弹窗：空标题的光标位置', () => {
  test.use({ wailsOverrides: BASE_OVERRIDES })

  test('新建时焦点在标题 input，空串末尾即位置 0', async ({ page }) => {
    await openPromptPanel(page)
    await page.locator('.prompt-panel [data-action="create"]').click()

    const dialog = editorDialog(page, '新建提示词')
    await expect(dialog).toBeVisible()

    const title = dialog.locator('[data-field="title"]')
    await expect(title).toBeFocused()
    await expect(title).toHaveValue('')
    expect(await title.evaluate(el => el.selectionStart)).toBe(0)
  })
})

test.describe('提示词编辑弹窗：ESC 与 X 均取消并复位草稿', () => {
  test.use({ wailsOverrides: BASE_OVERRIDES })

  test('ESC 与右上角 X 丢弃改动，重新打开回到后端原值', async ({ page }) => {
    await openPromptPanel(page)

    // ---- ESC：关闭后草稿复位，重新打开仍是原标题 ----
    let dialog = await openEditDialog(page)
    await dialog.locator('[data-field="title"]').fill('被 ESC 丢弃的标题')
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()

    dialog = await openEditDialog(page)
    await expect(dialog.locator('[data-field="title"]')).toHaveValue(PROMPT_TITLE)

    // ---- X：与 ESC 同语义，统一走 @closed 复位 ----
    await dialog.locator('[data-field="title"]').fill('被 X 丢弃的标题')
    await dialog.locator('.el-dialog__headerbtn').click()
    await expect(dialog).toBeHidden()

    dialog = await openEditDialog(page)
    await expect(dialog.locator('[data-field="title"]')).toHaveValue(PROMPT_TITLE)
    await expect(dialog.locator('[data-field="content"]')).toHaveValue(PROMPT_CONTENT)
    // 两条关闭路径都不得产生写调用
    expect(await getWailsCalls(page, 'UpdatePrompt')).toHaveLength(0)
  })
})
