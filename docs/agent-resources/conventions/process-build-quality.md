---
title: 构建质量门禁与已知缺口
scope: [build, quality, tooling, ci]
targets: [package.json, config, go]
---

# 构建质量门禁与已知缺口

本仓质量门禁以 **CI（`.github/workflows/ci.yml`）为准**，本地无 ESLint / Prettier / golangci-lint / husky 等静态检查与格式化工具——这是现状守护决策，**严禁未经用户授权擅自引入新检查工具或改写构建配置**。

## 1. 真实门禁清单（CI 强制）

| 门禁 | 命令 | 生效范围 |
|------|------|----------|
| 前端依赖分档一致性 | `node scripts/check-dep-consistency.mjs`（配 `--selftest`） | `frontend/package.json` + lock 三维一致 |
| Go 静态检查 | `go vet ./...` | 全部 Go 包 |
| Go 单元测试 | `go test ./...` | 全部 Go 包（含覆盖率门禁：model/server ≥80%、service ≥76%、util ≥40%，阈值见仓根 AGENTS.md） |
| Go 集成测试 | `go test -tags=integration ./...` | 带 integration 标签的测试 |
| 前端构建 | `npm run build`（frontend/ 下） | vite 构建成功 |
| 前端单测 | `npm test`（frontend/ 下） | vitest 全量 |
| E2E | `npm run e2e`（frontend/ 下） | Playwright 关键主链路 |

## 2. 命令前置依赖（必须成组执行，否则假失败）

`frontend/wailsjs/` 与 `frontend/dist/` 均不入库：

- **干净检出后**，任何 `go build` / `go vet` / `go test` / `npm run build` 之前必须先 `wails generate module` 生成绑定，并预置 `frontend/dist/` 占位（含 `index.html` 与 `assets/app.js`），否则 `//go:embed all:frontend/dist` 报 no matching files found。
- CI 步骤顺序即标准前置链，本地复现失败时先对照 CI 步骤补前置。

## 3. 提交前检查

- 提交前至少运行与本次改动相关的门禁（Go 改动跑 `go vet` + 相关包测试；前端改动跑 `npm run build` + 相关 spec）。
- 本仓无格式化工具，**不做自动格式化**；改动保持与周边代码风格一致即可。
- Git hooks：本仓未配置 husky/lint-staged/commitlint，无 hook 校验；提交规范遵循用户既有提交习惯（Conventional Commits 风格，历史记录可查）。
