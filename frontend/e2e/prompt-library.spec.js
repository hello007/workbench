/**
 * 提示词面板 E2E（话术库 + 占位符复制）。
 *
 * 链路：活动栏切入「提示词」→ 新建含 {{客户名称}} 的提示词 → 列表出现 →
 * 复制三条路径（填写后「替换并复制」/ 留空「替换并复制」/ 已填但点「复制原文」）→
 * 断言真实剪贴板内容 → 删除确认 → 列表消失。
 *
 * fixture 设计：
 * - 面板常驻挂载于 Home（v-show），挂载即 ListPrompts，因此用 __rules__ 跨方法
 *   状态机表达「AddPrompt 之后有条目、DeletePrompt 之后清空」，与调用次数无关
 *   （列表刷新、重试等触发点增减不会让断言失真）。
 * - 剪贴板读取真实 navigator.clipboard（localhost 属安全上下文），用例级授予
 *   clipboard-read / clipboard-write 权限，避免只断言 mock 内部调用而漏掉
 *   「写进去的内容是否被占位符替换」这一核心行为。
 */
import { test, expect, getWailsCalls, openPanel } from './fixtures'

/** 16 位小写 hex，对齐 model.Prompt 的 ID 生成口径（避免 fixture 形状偏离真实契约） */
const PROMPT_ID = '0123456789abcdef'
const PROMPT_TITLE = '客户开场'
const PROMPT_CONTENT = '尊敬的{{客户名称}}，您好！'

/** 新建后的列表条目，字段形状对齐 model.Prompt（id/title/content/createdAt/updatedAt） */
const PROMPT = {
  id: PROMPT_ID,
  title: PROMPT_TITLE,
  content: PROMPT_CONTENT,
  createdAt: 1735689600000,
  updatedAt: 1735689600000
}

/**
 * mock 基底：ListPrompts 按「AddPrompt / DeletePrompt 是否已经发生过」切换列表。
 * __rules__ 后命中覆盖前命中——删除后两规则同时命中，后声明的空列表生效；
 * 新建后只有 AddPrompt 规则命中，返回单条列表。
 *
 * 仅覆盖 ListPrompts 这一状态切换点：AddPrompt 的返回值面板不消费（保存后重新
 * ListPrompts），沿用 wails-mock-defaults.js 默认值，避免用例内重复定义静态返回值
 * （docs/spec/e2e-testing.md §3）。
 */
const BASE_OVERRIDES = {
  ListPrompts: {
    __rules__: [
      { after: 'AddPrompt', value: [PROMPT] },
      { after: 'DeletePrompt', value: [] }
    ],
    __default__: []
  }
}

/**
 * 读取页面真实剪贴板文本（依赖用例级 clipboard-read 权限）。
 *
 * 读写前都置前页面：剪贴板 API 要求文档获得焦点，headless 多 worker 并行时焦点
 * 可能被其他窗口抢走，此时 readText() 抛 NotAllowedError 或读到别的页面内容。
 * 置前放在读取动作自身而非打开面板时的一次性调用，保证每次断言前焦点都在本页
 * （对齐 docs/spec/e2e-testing.md 非 flaky 约定：不靠时序，靠动作内聚）。
 */
async function readClipboard(page) {
  await page.bringToFront()
  return page.evaluate(() => navigator.clipboard.readText())
}

/**
 * 剪贴板哨兵：每段复制前先用哨兵覆写，使后续断言的值只可能来自本段写入。
 * 必要场景：第 3 段（留空「替换并复制」）与第 4 段（已填但「复制原文」）的期望值
 * 同为正文原文（留空分支原样保留占位符，与不替换等价），不覆写时第 4 段会读到第 3
 * 段的残留值——复制原文若只关弹窗不写剪贴板，断言仍会通过（假绿）。哨兵与第 4 段的
 * 填写动作各守一侧：哨兵证明「确实发生了写入」，填写值证明「写入走原文而非替换路径」。
 */
const CLIPBOARD_SENTINEL = '__e2e_stale_clipboard__'

async function primeClipboard(page) {
  // 与 readClipboard 同因：写剪贴板同样要求文档持有焦点，多 worker 并行时必须
  // 在写入动作前重新置前，否则写入可能落到其他窗口的剪贴板上下文。
  await page.bringToFront()
  await page.evaluate((t) => navigator.clipboard.writeText(t), CLIPBOARD_SENTINEL)
}

/** 打开首页并切到提示词面板，等初始列表加载完成（空态渲染） */
async function openPromptPanel(page) {
  await page.goto('/')
  // 不在此处置前：剪贴板焦点要求在 primeClipboard / readClipboard 内部各自处理
  // （每次读写前置前），避免与助手函数重复调用。
  await openPanel(page, 'prompts')
  await expect(page.locator('.prompt-panel')).toBeVisible()
  await expect(page.locator('.prompt-panel .prompt-empty')).toContainText('暂无提示词')
}

test.describe('提示词面板：新建、占位符复制与删除', () => {
  test.use({
    wailsOverrides: BASE_OVERRIDES,
    permissions: ['clipboard-read', 'clipboard-write']
  })

  test('新建含占位符的提示词，替换复制与原文复制，最后删除', async ({ page }) => {
    await openPromptPanel(page)

    // ---- 1. 新建：标题 + 含 {{客户名称}} 的正文 ----
    await page.locator('.prompt-panel [data-action="create"]').click()

    const createDialog = page.locator('.el-dialog').filter({ hasText: '新建提示词' })
    await expect(createDialog).toBeVisible()

    await createDialog.locator('[data-field="title"]').fill(PROMPT_TITLE)
    await createDialog.locator('[data-field="content"]').fill(PROMPT_CONTENT)
    await createDialog.locator('[data-action="save"]').click()

    await expect(createDialog).toBeHidden()

    const item = page.locator(`[data-prompt-id="${PROMPT_ID}"]`)
    await expect(item).toBeVisible()
    await expect(item.locator('.prompt-title')).toHaveText(PROMPT_TITLE)
    // 占位符徽标：新建后列表能识别正文里的 1 个占位符
    await expect(item.locator('.prompt-badge')).toHaveText('1 个占位符')

    // 保存入参是用户输入原文（占位符不在前端保存时替换）
    const addCalls = await getWailsCalls(page, 'AddPrompt')
    expect(addCalls).toEqual([
      { method: 'AddPrompt', args: [PROMPT_TITLE, PROMPT_CONTENT] }
    ])

    // ---- 2. 填写占位符后「替换并复制」 ----
    const fillDialog = page.locator('.el-dialog').filter({ hasText: '填写占位符' })

    await item.locator('[data-action="copy"]').click()
    // 列表项的复制/编辑/删除按钮被 el-tooltip 包裹（show-after 300ms）：点击后
    // 鼠标驻留会让 tooltip 浮层常驻，可能遮挡后续操作，故点完即移开鼠标。手法与
    // assets-batch-tag.spec.js 相同（移开鼠标避免 tooltip 驻留），但坐标取 (0, 0)
    // ——活动栏顶部内边距的空白区（.activity-bar 为 48px 竖条 + padding 12px 0，
    // 该点无交互元素），而非先例的 (400, 400)；先例还断言 tooltip 收起，本处不做
    // 该断言（后续只看填窗可见性，tooltip 不参与判定）。
    await page.mouse.move(0, 0)
    await expect(fillDialog).toBeVisible()

    const fillInput = fillDialog.locator('[data-fill-name="客户名称"]')
    await expect(fillInput).toBeVisible()
    await fillInput.fill('张三')
    await primeClipboard(page)
    await fillDialog.locator('[data-action="copy-filled"]').click()

    await expect(fillDialog).toBeHidden()
    expect(await readClipboard(page)).toBe('尊敬的张三，您好！')

    // ---- 3. 占位符留空直接「替换并复制」：保留占位符原样 ----
    // 与下一步「复制原文」的路径差异：这里走 applyPlaceholders，空值分支把占位符
    // 原样保留（{{客户名称}} 不会被空串吃掉），是弹窗提示「留空的项会保留 {{名称}}
    // 原样」的实际校验点；「复制原文」则完全不进替换函数，直接写回正文。
    await item.locator('[data-action="copy"]').click()
    await page.mouse.move(0, 0)
    await expect(fillDialog).toBeVisible()

    // 不填写任何内容，直接提交
    await primeClipboard(page)
    await fillDialog.locator('[data-action="copy-filled"]').click()

    await expect(fillDialog).toBeHidden()
    expect(await readClipboard(page)).toBe(PROMPT_CONTENT)

    // ---- 4. 已填写占位符却点「复制原文」：跳过替换，剪贴板仍是正文原文 ----
    // 与第 3 段的路径差异只能靠「填了值却被忽略」证明：第 3 段走 applyPlaceholders
    // 的空值分支（占位符原样保留），本段完全不进替换函数，直接写回 fillTarget.content。
    // 若不填值，两段期望值相同，即便 copyOriginal 误接替换逻辑也会通过（假绿）。
    await item.locator('[data-action="copy"]').click()
    await page.mouse.move(0, 0)
    await expect(fillDialog).toBeVisible()

    await fillInput.fill('李四')
    await primeClipboard(page)
    await fillDialog.locator('[data-action="copy-original"]').click()

    await expect(fillDialog).toBeHidden()
    expect(await readClipboard(page)).toBe(PROMPT_CONTENT)

    // ---- 5. 删除：确认后列表项消失 ----
    await item.locator('[data-action="delete"]').click()
    await page.mouse.move(0, 0)

    // 按标题文本限定确认框：EP 2.14 起弹框关闭后实例驻留 DOM（display:none 不移除），
    // 同页出现多个确认框时裸 .el-message-box 会触发 strict mode 冲突。`:visible` 先过滤
    // 掉关闭残留的隐藏实例、再按标题文本命中当前活动框
    // （范式见 docs/spec/confirm-dialog.md §4 与 assets-backup-rules.spec.js:167-172）
    const confirmBox = page.locator('.el-message-box:visible').filter({ hasText: '删除提示词' })
    await expect(confirmBox).toBeVisible()
    await expect(confirmBox).toContainText('确定删除提示词「客户开场」吗？')
    await confirmBox.getByRole('button', { name: '删除' }).click()

    await expect(item).toHaveCount(0)
    await expect(page.locator('.prompt-panel .prompt-empty')).toContainText('暂无提示词')

    const deleteCalls = await getWailsCalls(page, 'DeletePrompt')
    expect(deleteCalls).toEqual([
      { method: 'DeletePrompt', args: [PROMPT_ID] }
    ])
  })
})
