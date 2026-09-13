/**
 * Git 三流程 E2E（提交/推送、分支管理、合并/变基）共享上下文与辅助函数。
 *
 * 数据流：mock 后端（wails-mock-defaults.js 的 E2E 合并表）只提供方法级默认值，
 * 「工作目录」这类用例上下文数据由本文件的 GIT_FLOW_OVERRIDES 经
 * test.use({ wailsOverrides }) 注入；openGitRepoPage 负责完成
 * 「打开首页 -> 点击工作目录 -> Git 签页出现」的公共前置链路。
 */
import { expect } from './fixtures'

/** mock 仓库路径：所有用例断言 bound method 收到的 repoPath 参数均引用此常量 */
export const E2E_REPO_PATH = 'D:/e2e-demo/demo-repo'

/** mock 工作目录（git 仓库）：形状对齐 model.Directory */
export const GIT_REPO_DIRECTORY = {
  id: 'dir-e2e-demo-repo',
  name: 'demo-repo',
  path: E2E_REPO_PATH,
  isDefault: false,
  isGitRepo: true,
  hasRemote: true
}

/** Git 三流程用例的覆盖集：注入工作目录，其余方法返回值用 E2E 合并表默认值 */
export const GIT_FLOW_OVERRIDES = {
  GetDirectories: [GIT_REPO_DIRECTORY]
}

/**
 * 公共前置：打开首页并选中 mock git 仓库工作目录。
 *
 * 断言链：首页 hash 路由就位 -> 目录项渲染 -> 点击后 ContentPanel 的 Git 签页
 * 出现（workspaceStore.selectedNode 已是 isGitRepo 节点）。后续用例只需按需
 * 点击对应签页（本地变动 / 分支 / 合并/变基，均为 lazy 挂载）。
 *
 * @param {import('@playwright/test').Page} page
 */
export async function openGitRepoPage(page) {
  // 加宽视口保证 ContentPanel（右栏 50%）内 8 个 Git 签页不溢出折叠
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto('/')
  await expect(page).toHaveURL(/#\/$/)

  const dirItem = page.locator('.dir-item', { hasText: GIT_REPO_DIRECTORY.name })
  await expect(dirItem).toBeVisible()
  await dirItem.click()

  // Git 签页出现即代表选中链路贯通（仓库信息签页默认激活，GitInfo 已加载）
  await expect(page.getByRole('tab', { name: '仓库信息' })).toBeVisible()
  await expect(page.locator('.git-actions').getByRole('button', { name: '切换分支' })).toBeVisible()
}

/**
 * 定位当前可见的 el-dialog（对话框传送至 body，且页面上可能同时存在多个
 * 已渲染但隐藏的 dialog 实例，须按标题文本 + 可见性双重过滤）。
 * @param {import('@playwright/test').Page} page
 * @param {string} title 对话框标题文本（如 '新建分支'）
 */
export function visibleDialog(page, title) {
  return page.locator('.el-dialog:visible', { hasText: title })
}
