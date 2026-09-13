/**
 * Playwright E2E 配置（WorkBench 前端 web 版 + mock Wails 后端，方案 C 前端层）。
 *
 * 运行链路：npm run e2e -> webServer 先 `vite build` 再 `vite preview`（固定端口），
 * Playwright 驱动 Chromium 访问 preview 服务；Wails 后端经 e2e/fixtures.js
 * 以 addInitScript 注入 window.go / window.runtime mock，不触及真实 Go 进程。
 *
 * 浏览器二进制：@playwright/test 不自带，首次运行前执行 `npm run e2e:install`
 * （CI 由 .github/workflows/ci.yml 统一安装并运行）。
 */
import { defineConfig, devices } from '@playwright/test'

// 与 webServer.command 中的 preview 端口保持一致；
// strictPort 保证端口被占时直接失败而非 +1 漂移，避免 url 探测错位
const PREVIEW_PORT = 4173

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  // CI 上误提交 .only 时直接失败
  forbidOnly: !!process.env.CI,
  // 本地 0 重试暴露问题；CI 2 次重试吸收偶发抖动
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI
    ? [['github'], ['html', { open: 'never' }]]
    : [['list'], ['html', { open: 'never' }]],
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
    // 每次跑 e2e 前先 build 保证产物新鲜，再起 preview（复用现有 vite 构建链）
    command: `npm run build && npm run preview -- --port ${PREVIEW_PORT} --strictPort`,
    url: `http://localhost:${PREVIEW_PORT}`,
    // 本地调试时已手动起 preview 则复用；CI 上必须由 Playwright 托管
    reuseExistingServer: !process.env.CI,
    timeout: 180_000
  }
})
