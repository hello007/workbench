/**
 * Playwright E2E 配置（WorkBench 前端 web 版 + mock Wails 后端，方案 C 前端层）。
 *
 * 运行链路：npm run e2e -> webServer 先 `npm run build:e2e`（产物落 .e2e-dist）
 * 再 `vite preview --outDir .e2e-dist`（固定端口），
 * Playwright 驱动 Chromium 访问 preview 服务；Wails 后端经 e2e/fixtures.js
 * 以 addInitScript 注入 window.go / window.runtime mock，不触及真实 Go 进程。
 *
 * 产物隔离（见 docs/spec/test-stability.md「E2E 与 Go 测试并发的资源隔离」）：
 * 构建产物与测试工件一律落在 `.` 开头的目录，Go 工具链展开 `./...` 时按官方
 * 规则跳过这类目录。若复用 frontend/dist，E2E 的 emptyOutDir 会与主包
 * `//go:embed all:frontend/dist` 的编译期读取竞态；若复用 frontend/test-results，
 * Playwright 重建目录时会与 go test 的目录遍历竞态（ENOENT -> [setup failed]）。
 *
 * 浏览器二进制：@playwright/test 不自带，首次运行前执行 `npm run e2e:install`
 * （CI 由 .github/workflows/ci.yml 统一安装并运行）。
 */
import { defineConfig, devices } from '@playwright/test'

// 与 webServer.command 中的 preview 端口保持一致；
// strictPort 保证端口被占时直接失败而非 +1 漂移，避免 url 探测错位
const PREVIEW_PORT = 4173
// E2E 专用构建产物目录：与 Wails 生产产物 frontend/dist 物理隔离，
// 避免 vite build 的 emptyOutDir 与主包 go:embed 读取竞态
const E2E_OUT_DIR = '.e2e-dist'
// E2E 测试工件（trace / html 报告）：Go 目录遍历跳过 `.` 前缀目录
const E2E_ARTIFACTS_DIR = './.e2e-artifacts'

export default defineConfig({
  testDir: './e2e',
  outputDir: `${E2E_ARTIFACTS_DIR}/test-results`,
  fullyParallel: true,
  // CI 上误提交 .only 时直接失败
  forbidOnly: !!process.env.CI,
  // 本地 0 重试暴露问题；CI 2 次重试吸收偶发抖动
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI
    ? [['github'], ['html', { open: 'never', outputFolder: `${E2E_ARTIFACTS_DIR}/report` }]]
    : [['list'], ['html', { open: 'never', outputFolder: `${E2E_ARTIFACTS_DIR}/report` }]],
  use: {
    baseURL: `http://localhost:${PREVIEW_PORT}`,
    // 失败用例保留 trace，本地 `npm run e2e:report` 查看
    trace: 'retain-on-failure'
  },
  // 本期仅 Chromium（与 WebView2 同内核），多浏览器不在范围
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    }
  ],
  webServer: {
    // 每次跑 e2e 前先 build 保证产物新鲜，再起 preview（复用现有 vite 构建链）。
    // build 与 preview 的 outDir 必须同为 E2E_OUT_DIR，否则 preview 会去读生产 dist。
    command: `npm run build:e2e && npm run preview -- --outDir ${E2E_OUT_DIR} --port ${PREVIEW_PORT} --strictPort`,
    url: `http://localhost:${PREVIEW_PORT}`,
    // 本地调试时已手动起 preview 则复用；CI 上必须由 Playwright 托管
    reuseExistingServer: !process.env.CI,
    timeout: 180_000
  }
})
