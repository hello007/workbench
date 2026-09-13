# E2E 测试规范（方案 C 混合架构）

> 本文档规定 WorkBench E2E 测试的分层架构、mock 约束、调用断言模式、构建标签隔离与 CI 绑定生成要求。
> 最后更新：2026-09-13 · 来源任务：09-13-e2e-key-flows、09-13-e2e-submodule-ai-mock

## 1. 适用范围

- 前端 Playwright E2E（`frontend/e2e/`，vite preview web 版 + mock Wails 后端）
- 后端 Go 集成测试（`git_flows_integration_test.go` / `git_submodule_integration_test.go`，构建标签 `integration`）
- CI 中 E2E 与集成测试的运行（`.github/workflows/ci.yml`）

## 2. 架构分层（方案 C）

| 层 | 载体 | 覆盖范围 | 明确不覆盖 |
|---|---|---|---|
| 前端 E2E | Playwright 驱动 `vite preview` web 版，`window.go` / `window.runtime` 全 mock | UI 编排、跨组件流程、路由、用户操作链、bound method 调用参数、AI 任务事件流状态机 | 真实 Go 后端、Wails 桌面桥接（WebView2 / 原生对话框） |
| 后端集成测试 | `go test -tags=integration`，`t.TempDir()` 真实 git 仓库 fixture | service 链业务逻辑（提交/推送/分支/合并/变基/冲突/submodule 全链） | Wails 绑定层、前端 UI |
| 现有单测 | go test + Vitest | 组件/函数级行为 | 跨组件编排与全链路 |

前后端交界契约靠 `docs/spec/cross-layer-contracts.md` 纪律对齐（mock 返回值形状须对齐 `App.d.ts` / `models.ts` 真实签名），非自动验证——这是方案 C 的已知缝隙。

## 3. mock 单一数据源（两端共用）

- mock 默认返回值表唯一来源：`frontend/src/test/wails-mock-defaults.js`
  - 基础表 `WAILS_MOCK_DEFAULT_RETURN_VALUES` 供 vitest 全局兜底（`src/test/setup.js`）与 E2E 共用
  - `WAILS_MOCK_E2E_EXTRA_RETURN_VALUES` 为 E2E 专属补充表（仅影响 UI 断言的方法给具体返回值）
  - `WAILS_MOCK_E2E_RETURN_VALUES` 为 E2E 实际注入的合并表（基础表 + 补充表，后者优先）
- **禁止**在用例内硬编码与默认表重复的返回值；需要新方法返回值时先改 `wails-mock-defaults.js`
- 表外方法经 Proxy 兜底 resolve null（链路不断裂），但影响 UI 断言的方法必须登记具体返回值
- mock 返回值形状对齐 `frontend/wailsjs/go/main/App.d.ts` / `models.ts` 真实签名（参数个数、字段名）

## 4. addInitScript 注入约束

`e2e/wails-init.js` 的 `injectWailsMocks` 经 `page.addInitScript` **序列化后**在浏览器页面上下文执行（早于应用任何脚本），因此：

- 函数体必须**自包含**：只能访问入参 `returnValues` 与浏览器全局，不得引用模块级 import / 闭包外变量（序列化后丢失）
- 入参必须**可结构化克隆**（纯 JSON 值），不得传函数、Symbol、循环引用
- 注入须覆盖两个页面全局：`window.go.main.App`（bound method 容器）与 `window.runtime`（EventsOn 等，常驻组件 setup 同步调用，缺失即渲染中断）

## 5. 调用断言模式（`__wailsCalls`）

- mock 后端每次 bound method 调用都 push `{method, args}` 到 `window.__wailsCalls`（按时间序）
- 用例经 `e2e/fixtures.js` 的 `getWailsCalls(page, method?)` 读取，断言「用户操作触发了正确的 bound method 与参数」
- 断言优先级：先断言用户可见 UI 结果（Element Plus 提示 / 列表 / 对话框状态），调用链断言（`getWailsCalls`）作补充，不断言组件内部实现细节

## 6. per-test 覆盖（wailsOverrides）

- 默认所有用例从 `./fixtures` import `{ test, expect }`（非 `@playwright/test`），自动注入 mock
- 用例 / describe 级动态行为经 `test.use({ wailsOverrides: { 方法名: 值 | 描述符 } })` 覆盖默认表
- value 描述符：纯值恒 resolve；`{__error__, __code__?}` reject（带 `__code__` 时对齐 Wails ErrorFormatter 的 `{code, message}` AppError 形态）；`{__sequence__: [...]}` 按序返回、耗尽后 hold last；`{__value__, __events__: [{event, payload, delayMs?}]}` resolve `__value__`（缺省 null）并按序经 `setTimeout` 派发 Wails 事件（`delayMs` 缺省 0；每次调用完整派发，无状态消费）
- `__events__` 派发机制：`wails-init.js` 的 `runtime.EventsOn/EventsOnMultiple/EventsOnce` 将回调登记到页面全局 `window.__wailsEventHandlers`（按事件名分组），描述符派发时逐一调用；`EventsOff` 移除该事件全部回调。用于模拟 AI 任务（`ai-task:queued/started/output/done`）等异步事件流——真实链路由后端 `runtime.EventsEmit` 推送，E2E 侧无需真实子进程
- 每测试独享新页面，覆盖互不影响（无跨用例状态泄漏）

## 7. 构建标签隔离（后端集成测试）

- 集成测试文件头 `//go:build integration`，默认 `go test ./...` 不编译，覆盖率门禁（`scripts/coverage-check.sh` 跑默认标签）不受影响
- CI 显式跑 `go test -tags=integration ./...`
- fixture 确定性约定（`git_flows_integration_test.go`）：
  - `t.TempDir()` 临时仓库 + 本地 bare 远程，不依赖网络
  - 显式 `symbolic-ref HEAD refs/heads/master`（规避 `init.defaultBranch` 差异）
  - 局部配置 `user.name/email`、`core.autocrlf=false`、`commit.gpgsign=false`（规避环境差异）
  - git 不在 PATH 时 skip 不 fail（CI 保证有 git）
- submodule fixture 约定（`git_submodule_integration_test.go`）：
  - 真实 `git submodule add <本地路径>` 产生标准 `.git/modules/<path>` clone 结构，无 fake 数据（单测侧 fake 与真实布局的差异在集成层天然规避）
  - `t.Setenv(GIT_CONFIG_COUNT/KEY_0/VALUE_0)` 注入 `protocol.file.allow=always`——git ≥2.38.1 默认禁 file transport，且 submodule clone 子进程不读父仓库局部 config（安全设计），仅环境变量可覆盖 fixture 命令与被测 App 链两类子进程
  - `RemoveSubmodule` 前置校验工作区干净，Add 后须 `itCommitAll` 提交再删

## 8. CI 绑定生成要求（wailsjs 不入库）

**事实**：`frontend/wailsjs/` 整目录在 `.gitignore`（自 commit 28ca710），CI 干净环境没有绑定文件，`vite build` 缺 `wailsjs/go/main/App` 模块必失败。

**CI 步骤**（`.github/workflows/ci.yml`；绑定生成本地 Windows 实测复现，`generate module` 仅解析 Go 代码产出 JS/TS 绑定文件，不构建前端、不依赖桌面 / WebView2 环境，ubuntu runner 适用）：

```yaml
- run: go install github.com/wailsapp/wails/v2/cmd/wails@v2
- run: wails generate module   # 重建 frontend/wailsjs/（App.js / App.d.ts / models.ts / runtime/）
```

**本地复现验证证据**（2026-09-13，wails CLI v2.12.0）：`rm -rf frontend/wailsjs && wails generate module` 完整重建绑定，`git status` 确认 wailsjs 仍被 ignore；重建后 `npm run build`、`npm test -- --run`、`npm run e2e` 全部通过。

**具名 string 类型别名（MergeMode 等）实测结论**：`wails generate module` 生成的 `models.ts` 不含 `export type MergeMode = string` 等别名，但 CI 无需补——前端运行时源码不 import `wailsjs/go/models`（纯 JS 项目，类型仅在 IDE 层消费），vite build 打包不涉及类型导出，实测 build 通过。仅当未来前端源码显式 `import { model } from wailsjs/go/models` 并以运行时位置引用缺失导出时，才会触发 MISSING_EXPORT（见 `cross-layer-contracts.md` 场景记录）。

## 9. 非 flaky 约定

- **禁止** `waitForTimeout` / 固定 sleep；全程依赖 `expect` 自动重试断言等待 UI（Playwright web-first assertion 自带轮询）
- 文件系统时序类断言遵循 `test-stability.md`（注入明确不等的时间值，不依赖 mtime 精度）
- 端口固定 4173 + strictPort（被占直接失败而非漂移，避免 url 探测错位）
- 本地 `retries=0` 暴露问题；CI `retries=2` 吸收偶发抖动 + `forbidOnly` 误提交 `.only` 直接失败
- 失败用例 `trace: 'retain-on-failure'`，本地 `npm run e2e:report` 复盘

## 10. CI 集成（.github/workflows/ci.yml）

| 项 | 取值 |
|---|---|
| 触发 | push(master) + pull_request；与 release.yml（tag v*）不冲突 |
| Runner | ubuntu-latest（git 预装，集成测试可跑；Playwright headless 原生支持） |
| 步骤顺序 | checkout → Go 1.24 → Node 20 → wails CLI + `wails generate module` → go test → go test integration → npm ci → npm run build → playwright install → e2e → npm test（快的在前，失败即停） |
| 失败产物 | `playwright-report/` + `test-results/` 上传 artifact（`if: failure()`，保留 7 天） |
| 版本对齐 | Go 1.24 / Node 20 / npm cache 与 release.yml 一致 |

## 11. 相关文档

- [cross-layer-contracts.md](cross-layer-contracts.md) — mock 返回值形状契约依据（App.d.ts / models.ts 签名同步）
- [test-stability.md](test-stability.md) — 文件系统时序 flaky 规避
- [test-coverage-gate.md](test-coverage-gate.md) — 覆盖率门禁（E2E 不纳入门禁，构建标签 integration 隔离保证不影响）
- [frontend/e2e/README.md](../../frontend/e2e/README.md) — E2E 运行方式与用例约定（日常参考入口）
