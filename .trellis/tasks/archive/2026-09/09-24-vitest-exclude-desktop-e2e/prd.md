# PRD：vitest 排除 e2e-desktop 目录修复 npm test 回归

## 背景

收口批次验证发现 `frontend/npm test` 出现 **2 个测试文件级失败**（1392 个用例本身全过）：

- `FAIL e2e-desktop/crash-flag.spec.js`
- `FAIL e2e-desktop/session-persist.spec.js`

根因：本批 desktop-e2e-cdp 功能（commit 1916756，2026-09-23）新增真桌面 E2E 通道 `frontend/e2e-desktop/`（Playwright spec + desktop-app.js），走独立 `playwright.desktop.config.js`（`npm run e2e:desktop`）正常；但 `vitest.config.js:11` 的 exclude 仅排除 `e2e/**`（2026-09-13 建 mock E2E 通道时的同类修复先例），未同步排除新目录，Playwright spec 被 vitest 误收集，顶层 `test.skip(!desktopAvailable(), ...)` 在 vitest 环境非法直接报 `test.skip() can only be called inside test`。

属 desktop-e2e-cdp 交付时的回归盲区（该任务验收只跑了 `e2e:desktop` 通道，未回归 `npm test`）。

## 需求

`frontend/vitest.config.js` exclude 数组追加 `'e2e-desktop/**'`，并更新相邻注释覆盖两个 E2E 目录（风格与既有注释一致）。

## 验收标准

- `cd frontend && npm test -- --run` 全绿：0 failed test files（68 个文件全过），1392+ 用例通过。
- `npm run e2e:desktop` 的 Playwright 通道不受影响（产物 exe 存在时仍可跑；本机验证 skip 逻辑即可——若 build/bin/workbench.exe 存在则跳过本项人工验证，说明理由）。
