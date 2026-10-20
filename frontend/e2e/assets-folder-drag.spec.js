/**
 * 素材管理文件夹树拖拽 E2E（PRD R4 主链路补齐，mock 驱动）。
 *
 * 覆盖四条拖拽链（此前仅单测覆盖，E2E 补真实 DOM 编排面）：
 * 1. 文件卡拖到树节点移动成功：dragstart 载荷（HTML5 DnD）→ drop 分派
 *    MoveAssetFiles（库 + fileIDs + 目标）→ 成功 toast + 统一刷新链底册重拉
 *    （ListAssetFiles 序列第二帧带新路径，条目卡片与文件夹徽标 DOM 更新）；
 * 2. 禁落点拒绝：排除命中目录节点 drop 在 FolderTree 组件内拒绝（warning toast），
 *    MoveAssetFiles 零调用（不出网）；
 * 3. 多选拖拽批量：勾选两卡后拖拽，dragstart 载荷携带全部选中 fileIds，移动成功
 *    后选中集清空（拖拽操作收口）；
 * 4. 目录拖到树节点（PRD R5，10-20-ep-2-14-5-e2e R3 增补）：先进入源目录构造
 *    folderPath 前缀语境，dragstart 在源目录树节点 → drop 分派 MoveAssetFolder
 *    （库 + 源 + 目标）→ 成功 toast + folderPath 按新前缀改写（newPath + 子路径）
 *    + 统一刷新链（底册/文件夹树重拉）。无需模拟扫描事件：后端 autoScan 同步
 *    收敛后才返回，mock 同步 resolve 与真实时序在「返回后仅显式刷新一轮」上等价
 *    （research/scan-done-dedup.md）。
 *
 * 拖拽模拟：Playwright page.dragAndDrop 依赖真实指针拖拽对 el-tree 插槽 DOM 结构
 * 生效性不稳，改 dispatchEvent 构造 dragstart/dragover/drop 事件序列——页面上下文
 * 内 new DataTransfer() 作载荷载体，dragstart 由组件 onCardDragStart / FolderTree
 * onNodeDragStart 写入 MIME 载荷（utils/assetFolder 单点 ASSET_FILE_DRAG_MIME /
 * ASSET_FOLDER_DRAG_MIME），drop 端 parseAssetDragPayload / parseFolderDragPayload
 * 消费，与真实 DnD 数据流同构。
 * mock 数据源为 wails-mock-defaults.js E2E 合并表（MoveAssetFiles/MoveAssetFolder/
 * ListAssetFolders/ListAssetSlides 等已登记）+ 本文件 overrides（形状对齐
 * model.AssetLibrary / AssetFileEntry / AssetFolderNode / AssetBatchMoveResult /
 * AssetFolderMoveResult）。
 * 本 spec 位于既有 e2e/ 通道目录，vitest.config.js test.exclude 的 'e2e/**' 已覆盖。
 */
import { test, expect, getWailsCalls } from './fixtures'

// 素材面板基础数据：单库 + 两个 ok 状态 pptx 文件（分居 方案A/方案B）+ 三节点
// 文件夹树（普通落点 归档 + 禁落点 已排除目录）；ListAssetFiles 用 __sequence__
// 两帧驱动——首帧（挂载加载）原路径、第二帧（移动成功统一刷新链重拉）文件 11
// 落 归档，验证「条目路径随刷新链更新」；AssetThumbsAvailable=false 走降级路径。
// MoveAssetFiles 移动结果按用例分档（单卡 succeeded=1 / 多选 succeeded=2，mock
// 返回值驱动 toast 文案，禁用例内硬编码与默认表重复的返回值——e2e-testing.md）
const libFixture = { id: 'lib-e2e', name: 'E2E 素材库', rootPath: 'D:/e2e-assets', createdAt: '2026-10-20 10:00:00', updatedAt: '2026-10-20 10:00:00' }
const makeFile = (id, dir, title) => ({
  id,
  path: `${dir}/${title}.pptx`,
  size: 1024 * 1024,
  modTime: '2026-10-14 10:00:00',
  status: 'ok',
  title,
  note: '',
  rating: 0,
  tags: [],
  createdAt: '2026-10-14 10:00:00',
  updatedAt: '2026-10-14 10:00:00'
})
const baseOverrides = {
  ListAssetLibraries: [libFixture],
  GetAssetLibraryStats: { libraryId: 'lib-e2e', fileCount: 2, slideCount: 0, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 },
  ListAssetFiles: {
    __sequence__: [
      [makeFile(11, '方案A', '城商行方案'), makeFile(12, '方案B', '获客方案')],
      [makeFile(11, '归档', '城商行方案'), makeFile(12, '方案B', '获客方案')]
    ]
  },
  ListAssetFolders: [
    { path: '方案A', name: '方案A', children: [], excluded: false, derived: false, fileCount: 1 },
    { path: '归档', name: '归档', children: [], excluded: false, derived: false, fileCount: 0 },
    { path: '已排除目录', name: '已排除目录', children: [], excluded: true, derived: false, fileCount: 1 }
  ],
  AssetThumbsAvailable: false
}

/**
 * 打开素材面板（活动栏第 5 项）。切面板后鼠标移离活动栏：el-tooltip 因点击后
 * 鼠标驻留会弹出「素材管理」提示并 teleport 到 body，遮挡画廊首卡交互（详见
 * assets-batch-tag.spec.js 同款注释），先移开并断言 tooltip 收起。
 */
async function openAssetsPanel(page) {
  await page.goto('/')
  await page.locator('.activity-bar-item').nth(4).click()
  await page.mouse.move(400, 400)
  await expect(page.locator('.el-popper').filter({ hasText: '素材管理' })).toBeHidden()
  const panel = page.locator('.assets-panel')
  await expect(panel).toBeVisible()
  await expect(panel.locator('.asset-card')).toHaveCount(2)
  return panel
}

/**
 * HTML5 DnD 事件序列模拟（页面上下文执行）：真实 DataTransfer 作载荷载体，
 * dragstart 由组件写入拖拽载荷（多选拖拽 = 选中集全量 fileIds，与真实数据流
 * 同构——载荷构造是组件逻辑非测试知识），drop 端组件解析分派。
 */
async function dragCardToFolder(page, cardTitle, folderName) {
  await page.evaluate(([cardTitle, folderName]) => {
    const dt = new DataTransfer()
    const card = [...document.querySelectorAll('.asset-card')]
      .find((el) => el.textContent.includes(cardTitle))
    if (!card) throw new Error(`拖拽源卡片未找到: ${cardTitle}`)
    card.dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true, cancelable: true }))
    const nameEl = [...document.querySelectorAll('.folder-node-name')]
      .find((el) => el.textContent.trim() === folderName)
    if (!nameEl) throw new Error(`落点树节点未找到: ${folderName}`)
    const node = nameEl.closest('.folder-node')
    node.dispatchEvent(new DragEvent('dragover', { dataTransfer: dt, bubbles: true, cancelable: true }))
    node.dispatchEvent(new DragEvent('drop', { dataTransfer: dt, bubbles: true, cancelable: true }))
  }, [cardTitle, folderName])
}

/**
 * 目录级拖拽（PRD R5）：dragstart 在源目录树节点（FolderTree onNodeDragStart 写入
 * ASSET_FOLDER_DRAG_MIME 载荷），drop 在目标树节点（onNodeDrop 解析分派
 * drop-folder），事件序列与文件卡拖拽同构。
 */
async function dragFolderToFolder(page, sourceName, targetName) {
  await page.evaluate(([sourceName, targetName]) => {
    const dt = new DataTransfer()
    const nameEls = [...document.querySelectorAll('.folder-node-name')]
    const srcEl = nameEls.find((el) => el.textContent.trim() === sourceName)
    if (!srcEl) throw new Error(`源目录树节点未找到: ${sourceName}`)
    srcEl.closest('.folder-node').dispatchEvent(new DragEvent('dragstart', { dataTransfer: dt, bubbles: true, cancelable: true }))
    const tgtEl = nameEls.find((el) => el.textContent.trim() === targetName)
    if (!tgtEl) throw new Error(`目标树节点未找到: ${targetName}`)
    const node = tgtEl.closest('.folder-node')
    node.dispatchEvent(new DragEvent('dragover', { dataTransfer: dt, bubbles: true, cancelable: true }))
    node.dispatchEvent(new DragEvent('drop', { dataTransfer: dt, bubbles: true, cancelable: true }))
  }, [sourceName, targetName])
}

test.describe('素材管理文件夹树拖拽（单卡）', () => {
  // 形状对齐 model.AssetBatchMoveResult（Item 含 fileId/status/label）
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      MoveAssetFiles: {
        total: 1,
        succeeded: 1,
        failed: 0,
        items: [{ fileId: 11, status: 'ok', label: '方案A/城商行方案.pptx' }]
      }
    }
  })

  test('文件卡拖到树节点移动成功：MoveAssetFiles 分派 + 成功 toast + 刷新链底册重拉条目路径与徽标更新', async ({ page }) => {
    const panel = await openAssetsPanel(page)
    // 初始路径（序列首帧）
    await expect(panel.locator('.asset-card').filter({ hasText: '方案A/城商行方案.pptx' })).toHaveCount(1)

    await dragCardToFolder(page, '城商行方案', '归档')

    // 成功 toast（单文件全成功口径）
    await expect(page.locator('.el-message').filter({ hasText: '已移动 1 个文件到「归档」' })).toBeVisible()
    // 调用参数断言：库 ID + 拖拽载荷 fileIds + 目标目录
    const calls = await getWailsCalls(page, 'MoveAssetFiles')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', [11], '归档'])
    // 统一刷新链底册重拉（序列第二帧）：条目卡片路径更新为 归档/城商行方案.pptx，
    // 原路径卡片消失；文件夹徽标随底册前缀聚合更新（归档 = 1）
    await expect(panel.locator('.asset-card').filter({ hasText: '归档/城商行方案.pptx' })).toHaveCount(1)
    await expect(panel.locator('.asset-card').filter({ hasText: '方案A/城商行方案.pptx' })).toHaveCount(0)
    const archiveNode = page.locator('.folder-node').filter({ has: page.locator('.folder-node-name', { hasText: /^归档$/ }) })
    await expect(archiveNode.locator('.folder-node-badge')).toHaveText('1')
  })

  test('禁落点拒绝：排除命中目录节点 drop 组件内拒绝（warning toast），MoveAssetFiles 零调用', async ({ page }) => {
    const panel = await openAssetsPanel(page)

    await dragCardToFolder(page, '城商行方案', '已排除目录')

    // FolderTree onNodeDrop 内拒绝：warning 中性提示，请求不出网
    await expect(page.locator('.el-message').filter({ hasText: '命中索引排除规则的目录不能作为移动目标' })).toBeVisible()
    const calls = await getWailsCalls(page, 'MoveAssetFiles')
    expect(calls.length).toBe(0)
    // 拒绝路径不触发刷新链：条目保持原路径
    await expect(panel.locator('.asset-card').filter({ hasText: '方案A/城商行方案.pptx' })).toHaveCount(1)
  })
})

test.describe('素材管理文件夹树拖拽（多选）', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      MoveAssetFiles: {
        total: 2,
        succeeded: 2,
        failed: 0,
        items: [
          { fileId: 11, status: 'ok', label: '方案A/城商行方案.pptx' },
          { fileId: 12, status: 'ok', label: '方案B/获客方案.pptx' }
        ]
      }
    }
  })

  test('多选拖拽批量：勾选两卡后拖拽载荷携带全部选中 fileIds，成功后选中集清空', async ({ page }) => {
    const panel = await openAssetsPanel(page)
    // 勾选两张文件卡：浮动操作条浮出
    await panel.locator('.asset-card-check').nth(0).click()
    await panel.locator('.asset-card-check').nth(1).click()
    const bar = panel.locator('.asset-selection-bar')
    await expect(bar).toBeVisible()
    await expect(bar.locator('.selection-count')).toHaveText('已选 2 项')

    // 拖拽选中集内卡片：dragstart 载荷 = 整个选中集文件 ID（组件 onCardDragStart 构造）
    await dragCardToFolder(page, '城商行方案', '归档')

    await expect(page.locator('.el-message').filter({ hasText: '已移动 2 个文件到「归档」' })).toBeVisible()
    const calls = await getWailsCalls(page, 'MoveAssetFiles')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', [11, 12], '归档'])
    // 拖拽多选收口：移动完成清空选中集（浮动条隐藏）
    await expect(bar).toBeHidden()
  })
})

test.describe('素材管理文件夹树拖拽（目录级）', () => {
  test.use({
    wailsOverrides: {
      ...baseOverrides,
      // 底册两帧：第二帧按目录级移动后的新前缀布局（方案A 整体挪入 归档 下）
      ListAssetFiles: {
        __sequence__: [
          [makeFile(11, '方案A', '城商行方案'), makeFile(12, '方案B', '获客方案')],
          [makeFile(11, '归档/方案A', '城商行方案'), makeFile(12, '方案B', '获客方案')]
        ]
      },
      // 树两帧：第二帧为移动后布局（顶层 方案A 节点消失、归档 徽标归位），节点
      // 结构差异同时充当 refreshDataAfterMutation({reloadFolders:true}) 树重拉的
      // 确定性 DOM 信号
      ListAssetFolders: {
        __sequence__: [
          [
            { path: '方案A', name: '方案A', children: [], excluded: false, derived: false, fileCount: 1 },
            { path: '归档', name: '归档', children: [], excluded: false, derived: false, fileCount: 0 },
            { path: '已排除目录', name: '已排除目录', children: [], excluded: true, derived: false, fileCount: 1 }
          ],
          [
            { path: '归档', name: '归档', children: [], excluded: false, derived: false, fileCount: 1 },
            { path: '方案B', name: '方案B', children: [], excluded: false, derived: false, fileCount: 1 },
            { path: '已排除目录', name: '已排除目录', children: [], excluded: true, derived: false, fileCount: 1 }
          ]
        ]
      },
      // 形状对齐 model.AssetFolderMoveResult（wails-mock-defaults.js 同形默认值；
      // newPath 落在目标目录下 = 目录级移动真实语义）
      MoveAssetFolder: { oldPath: '方案A', newPath: '归档/方案A', replaced: 2, excluded: 0, revived: 0 }
    }
  })

  test('目录拖到树节点：MoveAssetFolder 分派 + 成功 toast + folderPath 前缀改写 + 统一刷新链', async ({ page }) => {
    const panel = await openAssetsPanel(page)
    // 先进入源目录（folderPath = 方案A，画廊按前缀过滤为 1 卡），构造「选中目录
    // 处于源前缀下」的前缀改写语境
    await page.locator('.folder-node-name', { hasText: /^方案A$/ }).click()
    await expect(panel.locator('.asset-card')).toHaveCount(1)
    await expect(panel.locator('.asset-card').filter({ hasText: '方案A/城商行方案.pptx' })).toHaveCount(1)

    await dragFolderToFolder(page, '方案A', '归档')

    // 成功 toast（replaced 计数口径）
    await expect(page.locator('.el-message').filter({ hasText: '已移动到「归档」（索引同步 2 条）' })).toBeVisible()
    // 调用参数断言：库 ID + 源目录 + 目标目录
    const calls = await getWailsCalls(page, 'MoveAssetFolder')
    expect(calls.length).toBe(1)
    expect(calls[0].args).toEqual(['lib-e2e', '方案A', '归档'])

    // 统一刷新链：底册重拉（序列第二帧）+ 树重拉（ListAssetFolders 首载 +
    // reloadFolders 共 2 次，顶层 方案A 节点随第二帧消失）；folderPath 按新前缀
    // 改写（newPath + 子路径）——画廊仍恰 1 卡且路径为新前缀（若 folderPath 残留
    // 旧前缀「方案A」则 0 卡、若复位根则 2 卡，三态可分辨）；归档徽标随底册前缀
    // 聚合更新
    await expect(panel.locator('.asset-card').filter({ hasText: '归档/方案A/城商行方案.pptx' })).toHaveCount(1)
    await expect(panel.locator('.asset-card')).toHaveCount(1)
    await expect(page.locator('.folder-node-name', { hasText: /^方案A$/ })).toHaveCount(0)
    const folderCalls = await getWailsCalls(page, 'ListAssetFolders')
    expect(folderCalls.length).toBe(2)
    const archiveNode = page.locator('.folder-node').filter({ has: page.locator('.folder-node-name', { hasText: /^归档$/ }) })
    await expect(archiveNode.locator('.folder-node-badge')).toHaveText('1')
  })
})
