/**
 * Playwright 真桌面 E2E 配置（WebView2 CDP 通道，方案 C 补充通道）。
 *
 * 运行链路：npm run e2e:desktop -> 前置确认 build/bin/workbench.exe 存在
 * （缺失则 wails build 先行，见 e2e-desktop/README.md）-> 用例 spawn 桌面应用
 * （CDP 调试端口 + 隔离 data 目录）-> connectOverCDP 接管页面。
 *
 * 与 mock 通道（playwright.config.js + vite preview + wails mock）完全共存：
 * testDir 独立，互不影响；产物 exe 缺失/非 Windows 平台用例自动 skip
 * （CI Linux runner 天然跳过，无 Windows runner 成本）。
 */
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e-desktop',
  fullyParallel: false, // 用例各自 spawn 独立应用实例，串行避免资源竞争与端口漂移
  forbidOnly: !!process.env.CI,
  retries: 0,
  // 真桌面启动/关闭周期长，用例级超时在 spec 内按链路放宽（180s）
  timeout: 120000,
  reporter: [['list']],
  use: {
    trace: 'retain-on-failure'
  }
})
