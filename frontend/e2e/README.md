# Playwright E2E 测试（前端 web 版 + mock Wails 后端）

WorkBench E2E 采用混合方案的前端层：Playwright 驱动 `vite preview` 提供的 web 版前端，
Wails Go 后端在页面加载前以 `window.go` / `window.runtime` mock 替换。
方案选型与决策记录见 `.trellis/tasks/09-13-e2e-key-flows/prd.md`。

## 首次准备

`@playwright/test` 不自带浏览器二进制，首次使用前需安装 Chromium（仅首次或升级版本后）：

```bash
cd frontend
npm run e2e:install
```

## 运行

```bash
cd frontend
npm run e2e          # 跑全部 E2E（webServer 自动先 vite build 再 vite preview）
npm run e2e:report   # 查看最近一次 HTML 报告（含失败 trace 入口）
```

- webServer 固定端口 `4173`（strictPort，端口被占直接失败而非漂移）
- 本地 `retries=0`；CI（`process.env.CI`）`retries=2`，并启用 github + html reporter
- 本地若已手动起 4173 preview，Playwright 直接复用（`reuseExistingServer`）

## CI 集成

`.github/workflows/ci.yml`（push master / pull_request 触发，ubuntu-latest）：CI 环境自动安装
Chromium（`npx playwright install --with-deps chromium`）后跑 `npm run e2e`，失败时上传
`playwright-report/` 与 `test-results/`（保留 7 天，含失败用例 trace）。`frontend/wailsjs/`
不入库，CI 在前端构建前经 `wails generate module` 重新生成绑定（规范见
[docs/spec/e2e-testing.md](../docs/spec/e2e-testing.md) 第 8 节）。

## 目录与文件职责

| 文件 | 职责 |
|---|---|
| `playwright.config.js` | testDir / projects（仅 Chromium）/ retries / webServer（build + preview） |
| `e2e/fixtures.js` | 扩展 `test`：每个用例自动 `addInitScript` 注入 Wails mock，用例统一从这里 import；导出 `getWailsCalls(page, method?)` 读取页面内 `window.__wailsCalls` 调用记录 |
| `e2e/wails-init.js` | 浏览器页面上下文执行的注入函数（`window.go.main.App` + `window.runtime` stub + `__wailsCalls` 调用记录 + `{__error__}` / `{__sequence__}` 返回值描述符） |
| `e2e/git-fixtures.js` | Git 三流程共享上下文：mock 工作目录常量（`GIT_FLOW_OVERRIDES`）、公共前置 `openGitRepoPage`、对话框定位 `visibleDialog` |
| `e2e/*.spec.js` | E2E 用例 |
| `../src/test/wails-mock-defaults.js` | mock 默认返回值表单一数据源（vitest setup.js 与 E2E 共用基础表，E2E 另有专属补充表） |

## mock 机制说明

- **`window.go.main.App`**：`frontend/wailsjs/go/main/App.js` 的每个 bound method
  运行时都转发到 `window['go']['main']['App']['<方法名>']`，注入按默认值表生成
  Promise 化方法；表外方法经 Proxy 兜底 resolve null（链路不断裂，但影响 UI
  断言的方法应在 `WAILS_MOCK_E2E_EXTRA_RETURN_VALUES` 给具体返回值）。
- **`window.runtime`**：`wailsjs/runtime/runtime.js` 的 EventsOn / BrowserOpenURL
  等转发到 `window.runtime`，注入 no-op stub（常驻组件 setup 同步调用，缺失即渲染中断）。
- **与单测一致性**：默认值表由 `src/test/wails-mock-defaults.js` 统一提供，
  vitest 全局兜底（`src/test/setup.js`）与 E2E 共用基础表，mock 行为不漂移。

## 新增用例约定

1. 从 `./fixtures` import `{ test, expect }`（非 `@playwright/test`），确保 mock 注入生效
2. 需要特定返回值时优先在 `wails-mock-defaults.js` 补充；用例级动态行为（序列 / 错误）
   经 `test.use({ wailsOverrides: {...} })` 覆盖，value 支持 `{__error__}` / `{__sequence__}`
   描述符——每测试独享新页面，覆盖互不影响
3. 断言用户可见的 UI 结果（Element Plus 提示 / 列表 / 对话框状态），避免断言内部实现细节；
   调用链断言用 `getWailsCalls(page, method)` 验证 bound method 收到的参数与调用次数
4. 全程依赖 `expect` 自动重试断言等待 UI，禁止 `waitForTimeout` / 固定 sleep（flaky 规避，
   见 `docs/spec/test-stability.md`）
5. mock 返回值形状须对齐 `frontend/wailsjs/go/main/App.d.ts` / `models.ts` 真实签名
   （参数个数、字段名），契约参照 `docs/spec/cross-layer-contracts.md`
