# 配置变量参考

> 本文档描述配置与构建变量现状，**内容易腐**：引用具体数值前先核对对应文件。

## 1. Go 侧配置

| 文件 | 关键项 | 说明 |
|------|--------|------|
| `go.mod` | `go 1.26.0` + `toolchain go1.26.10` | 语言版本与工具链；标准库漏洞只能升 toolchain directive |
| `go.mod` | module `workbench` | 模块名；核心依赖 wails v2.16.0、modernc.org/sqlite（纯 Go 零 CGO）、goleak |
| `wails.json` | 应用名/版本/构建信息 | 发版版本号真相源（`/release` 流程读取） |

## 2. 前端配置

| 文件 | 关键项 | 说明 |
|------|--------|------|
| `frontend/package.json` | dependencies / devDependencies | 39 顶层依赖四档钳制（exact / tilde / caret / url），新依赖须先在 `scripts/check-dep-consistency.mjs` 四清单登记 |
| `frontend/package.json` | scripts | dev/build/test/test:coverage/e2e 系列（无 lint/format） |
| `frontend/vite.config.js` | 构建与插件、同源路由前缀判定 | dev 模式自定义中间件（如 /preview-pdf 404 前置） |
| `frontend/vitest.config.js` | test 配置 | 含 `test.exclude`（新增 E2E spec 目录须同步） |

## 3. CI / 环境变量

| 位置 | 变量/配置 | 说明 |
|------|-----------|------|
| `.github/workflows/ci.yml` | push(master)/PR 触发 | Linux + Windows 双 job；依赖分档检查 → 预置 dist 占位 → Go 验证 → 前端构建/单测 → E2E |
| `.github/workflows/release.yml` | tag `v*` 触发 | Windows + ubuntu（webkit2_41，glibc 2.35 基线）双 job 发版 |
| `scripts/coverage-check.sh` | 覆盖率门禁 | model/server ≥80%、service ≥76%、util ≥40%（主包不设），阈值以仓根 CLAUDE.md 关键规则为准 |

## 4. 运行时数据目录

| 路径 | 说明 |
|------|------|
| `data/` | 运行时 JSON 持久化（不入库，`*.json` 忽略）；写入必须走 `util.SaveJSON` 原子写 |
| `build/` | 构建产物（整体忽略，仅 README-linux.md 白名单放行） |

## 5. 各层修改位置速查

- 改 Go 版本 → `go.mod`（toolchain directive），改后重跑 `govulncheck ./...`
- 加前端依赖 → 先登记 `check-dep-consistency.mjs` 四清单 → 再改 `frontend/package.json` → `npm update <pkg>` 落 lock
- 改覆盖率阈值 → `scripts/coverage-check.sh` + 仓根 CLAUDE.md 关键规则行 + `docs/测试策略.md` 三处同步
- 改 CI 门禁 → `.github/workflows/ci.yml`（改动前先读文件头前置事实注释）
