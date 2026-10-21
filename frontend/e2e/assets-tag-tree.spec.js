/**
 * 素材标签树拖拽排序与失败回滚 E2E（assets-management.md §9.5 D8#1，mock 驱动，
 * 矩阵候选 #18）。
 *
 * 覆盖（标签管理弹窗 el-tree draggable）：
 * 1. 成功链：拖子标签「城商行」到根标签「方案」（inner 落点 = 变子级组尾追加）→
 *    落点换算单点 resolveTagMoveDrop → MoveAssetTag(库, tagId, newParentID, index)
 *    参数断言 → 乐观更新树重排（DOM 嵌套关系翻转）；
 * 2. 回滚链：同款拖拽 MoveAssetTag reject → 乐观更新回滚 + 重拉保权威（恢复原
 *    嵌套位置）+ handleError 失败 toast。
 *
 * el-tree 拖拽模拟：组件 dragstart/dragover/dragend 监听挂在 .el-tree-node 根元素
 * （element-plus tree-node 渲染层），页面上下文 dispatchEvent 构造事件序列——
 * dragover 的 clientY 按落点节点 content 矩形比例定位（中段 = inner，对齐
 * element-plus useDragNode 的 25%/75% 阈值口径）；DataTransfer 载荷与真实 DnD
 * 同构（folder-drag.spec 先例）。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表（ListAssetTags 默认两级样例即
 * 本用例树形：重点客户←城商行 + 方案；MoveAssetTag 缺省 null = resolve，回滚链
 * override {__error__}）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-21 10:00:00', updatedAt: '2026-10-21 10:00:00' }

const baseOverrides = {
  ListAssetLibraries: [libFixture],
  GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 0, slideCount: 0, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 }
}

/**
 * el-tree 节点拖拽事件序列模拟（页面上下文执行，作用域限定标签管理弹窗）：
 * dragstart 于源节点 → dragover 于落点节点（clientY 按落位比例定位 dropType：
 * inner = 矩形中段 50%）→ dragend 于源节点（element-plus 在 dragend 提交
 * node-drop 事件，与真实拖拽收尾时序一致）。
 */
async function dragTagNode(page, dragKey, dropKey, position = 'inner') {
  await page.evaluate(([dragKey, dropKey, position]) => {
    const dialog = [...document.querySelectorAll('.el-dialog')]
      .find((el) => el.textContent.includes('标签管理'))
    if (!dialog) throw new Error('标签管理弹窗未找到')
    const dragEl = dialog.querySelector(`.el-tree-node[data-key="${dragKey}"]`)
    if (!dragEl) throw new Error(`拖拽源节点未找到: ${dragKey}`)
    const dropEl = dialog.querySelector(`.el-tree-node[data-key="${dropKey}"]`)
    if (!dropEl) throw new Error(`落点节点未找到: ${dropKey}`)
    const dt = new DataTransfer()
    dragEl.dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true, cancelable: true }))
    const rect = dropEl.querySelector('.el-tree-node__content').getBoundingClientRect()
    const ratio = position === 'before' ? 0.1 : position === 'after' ? 0.9 : 0.5
    dropEl.dispatchEvent(new DragEvent('dragover', {
      dataTransfer: dt,
      bubbles: true,
      cancelable: true,
      clientY: rect.top + rect.height * ratio
    }))
    dragEl.dispatchEvent(new DragEvent('dragend', { dataTransfer: dt, bubbles: true, cancelable: true }))
  }, [dragKey, dropKey, position])
}

/** 打开素材面板并进入标签管理弹窗，返回弹窗定位器（树默认全展开） */
async function openTagManager(page) {
  await page.goto('/')
  await page.locator('.activity-bar-item').nth(4).click()
  await page.mouse.move(400, 400)
  await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
  const panel = page.locator('.assets-panel')
  await expect(panel).toBeVisible()
  await panel.getByRole('button', { name: '管理标签' }).click()
  const dialog = page.locator('.el-dialog').filter({ hasText: '标签管理' })
  await expect(dialog).toBeVisible()
  const node = (key) => dialog.locator(`.el-tree-node[data-key="${key}"]`)
  // 初始树形：城商行(2) 嵌套在 重点客户(1) 下，方案(3) 为无子根标签
  await expect(node(1).locator('.el-tree-node[data-key="2"]')).toHaveCount(1)
  await expect(node(3).locator('.el-tree-node[data-key="2"]')).toHaveCount(0)
  return dialog
}

test.describe('素材标签树拖拽移动（成功重排）', () => {
  test.use({ wailsOverrides: baseOverrides })

  test('拖子标签到根标签 inner 落点：MoveAssetTag 参数断言 + 乐观更新树重排', async ({ page }) => {
    const dialog = await openTagManager(page)

    // 城商行(2) 拖到 方案(3) 中段（inner = 变子级组尾追加）
    await dragTagNode(page, 2, 3, 'inner')

    // 调用参数断言：库 + 拖拽节点 + 目标父 + 组尾 index（resolveTagMoveDrop 口径：
    // 目标组排除自身后长度 0）
    const calls = await getWailsCalls(page, 'MoveAssetTag')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', 2, 3, 0])

    // 乐观更新树重排：城商行翻转到 方案 下，原父 重点客户 下不再嵌套
    await expect(dialog.locator('.el-tree-node[data-key="3"] .el-tree-node[data-key="2"]')).toHaveCount(1)
    await expect(dialog.locator('.el-tree-node[data-key="1"] .el-tree-node[data-key="2"]')).toHaveCount(0)
  })
})

test.describe('素材标签树拖拽移动（失败回滚）', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      MoveAssetTag: { __error__: '移动失败' }
    }
  })

  test('MoveAssetTag reject：乐观更新回滚恢复原嵌套位置 + 重拉保权威 + 失败 toast', async ({ page }) => {
    const dialog = await openTagManager(page)

    await dragTagNode(page, 2, 3, 'inner')

    // 失败 toast（handleError 前缀 + reject 消息）
    await expect(page.locator('.el-message').filter({ hasText: '调整标签顺序失败: 移动失败' })).toBeVisible()

    // 回滚 + 重拉（ListAssetTags 权威数据）：城商行恢复在 重点客户 下，方案 下无嵌套
    await expect(dialog.locator('.el-tree-node[data-key="1"] .el-tree-node[data-key="2"]')).toHaveCount(1)
    await expect(dialog.locator('.el-tree-node[data-key="3"] .el-tree-node[data-key="2"]')).toHaveCount(0)

    // 重拉保权威调用计数：面板挂载筛选树 1 次 + 弹窗打开即拉 1 次 + 回滚重拉 1 次
    const listCalls = await getWailsCalls(page, 'ListAssetTags')
    expect(listCalls.length).toBe(3)
  })
})
