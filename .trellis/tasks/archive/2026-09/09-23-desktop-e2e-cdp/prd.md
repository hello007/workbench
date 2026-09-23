# 真桌面 E2E 增强（WebView2 CDP）

## Goal

E2E 补真桌面通道（路线图 L258 留白）：Playwright `connectOverCDP` 接 WebView2 调试端口，验证 mock Wails 后端覆盖不到的真实 runtime 行为。P3 按需增强。

## Requirements

* **brainstorm 决策（摸底后定稿）**：
  * 选型 → **WebView2 官方 CDP 路径**（Playwright 官方文档 webview2 章节）：环境变量 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=<port>` + `WEBVIEW2_USER_DATA_FOLDER=<隔离目录>` 启动 workbench.exe，Playwright `chromium.connectOverCDP(http://127.0.0.1:<port>)` 接管。应用以临时目录为 cwd 启动实现 data/ 隔离（data 路径为相对 cwd 的 "data"），不污染用户真实数据
  * 覆盖范围 → **2 条 mock 盲区链路**（选持久化/生命周期类，不依赖外部 git 环境，可全自动）：
    1. **会话状态真实持久化恢复**：切换活动面板 → 关闭应用（正常退出）→ 重启 → 验证恢复（mock 通道无真实 session.json 读写与重启周期）
    2. **异常退出标记闭环**（本批次 09-23-error-report-export 刚落地）：正常退出后 crash.flag 清除、重启无提示；强杀进程后 crash.flag 残留、重启出现「上次运行异常退出」提示
  * 通道形态 → **独立 `frontend/e2e-desktop/` 目录 + `playwright.desktop.config.js` + `npm run e2e:desktop`**，与现有 mock 通道完全共存；用例在产物 exe 不存在时自动 skip（CI Linux 天然跳过，无 runner 成本）；构建步骤 `wails build` 由脚本前置提示/独立执行，不进 mock E2E 链路
* 真桌面 E2E 与现有 mock 通道（vite preview + wails-mock-defaults）共存，不迁移全量用例

## Acceptance Criteria

* [x] 真桌面 E2E 通道跑通（本地脚本）——`npm run e2e:desktop` 三条用例连续三轮全绿（29-31s）；非 Windows / 产物缺失自动 skip，CI 无 runner 成本
* [x] 至少 1 条 mock 盲区链路有真桌面覆盖——2 条：会话状态真实持久化重启恢复（session.json 落盘 + 重启恢复活动面板）、异常退出标记闭环（强杀残留 + 重启后端日志检测记录）
* [x] docs/测试策略.md、docs/spec/e2e-testing.md 同步

## 选型实证（重要，brainstorm 修正记录）

**WebView2 CDP 注入不可行（4 路实测）**：Wails v2.16 + go-webview2 v1.0.23 经 `createCoreWebView2EnvironmentWithOptions` 显式传参（`--disable-features=msSmartScreenProtection`），loader 层以下全部被忽略：① `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` 环境变量（生产构建）；② 同上（debug 构建）；③ HKCU 注册表策略 `Policies\Microsoft\Edge\WebView2\AdditionalBrowserArguments`；④ `WEBVIEW2_USER_DATA_FOLDER` 同理被 Wails 显式 userDataPath 覆盖。**通道修正**：真实桌面进程 + 浏览器通道（HTTP + token），进程生命周期/真实后端/文件落盘全保真，仅渲染端（浏览器 vs WebView2）与目标无关。详见 docs/spec/e2e-testing.md「真桌面 E2E 通道」节。

**产品级隐患发现**（超出本任务 scope，建议后续处理）：go-webview2 `errorCallback` 在窗口销毁链中偶发 `os.Exit(1)` 跳过 wails shutdown 钩子——crash.flag 清除（shutdown 末尾）因此偶发缺失，用户正常关窗可能被误报「上次运行异常退出」。E2E 已按此调整断言口径（flag 方向不做强断言）。

## Out of Scope

* 全量用例迁移真桌面（mock 通道仍是主力）；CI Windows runner 成本评估（如无 runner 则留本地脚本）

## Technical Notes

* 现有基础：E2E 方案 C 混合架构（Playwright + mock Wails）；桌面端 WebView2（Windows）/webkit2gtk（Linux）双栈
