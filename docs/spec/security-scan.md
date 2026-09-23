# 安全扫描

> 本文档规定 WorkBench 的依赖安全扫描方案：工具选型、本地命令、CI 集成、已知漏洞处置策略。
> 最后更新：2026-09-14 · 来源任务：09-14-v1-4 PR1 子项4+5

## 1. 适用范围

- Go 模块漏洞扫描：govulncheck（基于代码调用链分析，非全依赖树）
- npm 依赖漏洞扫描：npm audit（全依赖树比对 advisories 数据库）
- CI 集成：`.github/workflows/ci.yml` 的 `security` job，与 `test` job 并行

## 2. 工具选型

| 工具 | 范围 | 原理 | fail 条件 |
|---|---|---|---|
| govulncheck | Go 标准库 + 第三方模块 | 静态分析代码实际调用链，只报代码可达漏洞，噪声低 | 发现任何代码受影响漏洞 exit 3 |
| npm audit | npm 全依赖树 | 比对 package-lock.json 与 GitHub Advisory 数据库 | >= `--audit-level` 漏洞 exit 1 |

## 3. 本地扫描命令

### 3.1 govulncheck

安装（二进制落到 `$GOPATH/bin`，Windows 下 `C:\Users\<user>\go\bin\govulncheck.exe`）：

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
```

扫描（项目根目录）：

```bash
govulncheck ./...
```

退出码：0 = 无漏洞；3 = 发现代码受影响漏洞。govulncheck 报告的是**当前 Go 工具链**的标准库漏洞，标准库漏洞修复依赖工具链版本（见 §6）。

### 3.2 npm audit

本地 npm registry 是 npmmirror（不支持安全端点），须显式指定官方 registry：

```bash
cd frontend && npm audit --registry=https://registry.npmjs.org --audit-level=high
```

`--audit-level=high`：仅 high 以上漏洞才使命令失败，避免 low/moderate 噪声阻塞。

## 4. CI 集成

`.github/workflows/ci.yml` 新增 `security` job：

- 与 `test` job 并行，不阻塞测试
- `continue-on-error: true`：发现 high 漏洞时标记 job 失败但不阻塞 PR 合并（信息性扫描，漏洞在 PR check 可见）
- Go 版本与 test job 一致（`1.26`，配合 go.mod `toolchain go1.26.6`）
- 步骤：安装 govulncheck → `govulncheck ./...` → 安装前端依赖 → `npm audit --registry=https://registry.npmjs.org --audit-level=high`

详细配置见 `.github/workflows/ci.yml` 的 `security` job。

## 5. 已知漏洞与处置

### 5.1 已修复（2026-09-14 升级轮）

**Go（13 → 0，全部清零）**：

| 漏洞 ID | 模块 / 包 | 修复方式 |
|---|---|---|
| GO-2026-6214、GO-2026-5496 | go-git v5.18.0 → v5.19.2 | 依赖升级（minor） |
| GO-2026-5597、GO-2026-5490 | go-billy v5.8.0 → v5.9.1（indirect） | 依赖升级（minor） |
| GO-2026-5026、GO-2026-4918 | golang.org/x/net v0.47.0 → v0.59.0（indirect） | 依赖升级（minor） |
| GO-2026-6218 等 9 个标准库漏洞 | net/url、crypto/tls、encoding/asn1、net/textproto、crypto/x509、net/http、net | Go 工具链 go1.26.2 → go1.26.6（go.mod `toolchain` directive） |

**npm（4 → 1）**：

| 漏洞 ID | 包 | 修复方式 |
|---|---|---|
| GHSA-82fw-gwwq-j7x9 @vitest/mocker Path Traversal | vitest 4.1.5 → 4.1.11 | npm update（minor） |

### 5.2 历史接受风险（已处置清零）

| 漏洞 ID | 包 | 严重级 | 处置 |
|---|---|---|---|
| GHSA-4r6h-8v6p-xvw6 Prototype Pollution | xlsx 0.18.5 → 0.20.3 | high | ✅ 已修复（2026-09-23） |
| GHSA-5pgg-2g8v-p4x9 ReDoS | xlsx 0.18.5 → 0.20.3 | high | ✅ 已修复（2026-09-23） |

**背景**：SheetJS 官方已将发布迁移至自建 CDN（https://cdn.sheetjs.com），npm registry 上的 `xlsx` 包冻结在 0.18.5，故 `npm audit` 长期报 `No fix available`。两个漏洞（CVE-2023-30533 原型污染等）均在 0.19.3+ 修复。

**处置方式（2026-09-23）**：`npm install https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz`，`package.json` 中 `xlsx` 依赖变为 CDN tarball URL。import 路径 `xlsx` 不变，业务代码零改动；`FilePreviewRenderer.vue` 的 Excel 只读渲染经全量前端测试（1369 用例）与生产构建验证兼容。

**注意事项**：
- CDN tarball URL 依赖 cdn.sheetjs.com 可达，CI（GitHub Actions Linux runner）与本地安装均需网络访问该域名。
- `npm audit` 按 lock 文件实际版本号（0.20.3）匹配 advisory 范围，实测报 0 vulnerabilities。
- 若未来需再升级，先探测 CDN 版本存在性（0.20.4+ 并不存在，当前最新即 0.20.3），勿凭版本号臆测。

### 5.3 major 版本待评（本轮跳过）

| 包 | 当前版本 | 可用 major | 说明 |
|---|---|---|---|
| go-git | v5.19.2 | v6 | major 升级涉及 API 变更，单独评估 |
| wails | v2.16.0 | v3 | v3 仍在 beta，跟踪官方稳定版发布后再评 |

## 6. Go 工具链与标准库漏洞

govulncheck 报告的标准库漏洞（`net/url`、`crypto/tls` 等）**只能通过升级 Go 工具链修复**，无法通过 `go get` 修复。

- go.mod `go 1.26.0` 为最低兼容声明
- go.mod `toolchain go1.26.6` 指定实际使用的工具链版本（GOTOOLCHAIN=auto 自动下载）
- CI `setup-go` 安装 `1.26` 后，go 命令遵循 `toolchain` directive 自动切换到 1.26.6
- 升级工具链后须重跑 `govulncheck ./...` 确认标准库漏洞清零

## 7. 维护规则

- 新增 Go 依赖：提交前跑 `govulncheck ./...` 确认无新增代码受影响漏洞
- 新增 npm 依赖：提交前跑 `npm audit --registry=https://registry.npmjs.org --audit-level=high` 确认无 high 漏洞
- Go 工具链升级：`toolchain` directive 须对齐 govulncheck 报告的最高标准库修复版本
- 季度复核：每季度跑一次全量扫描，更新本文档 §5 漏洞清单
- 新增无补丁漏洞：按 §5.2 格式记录漏洞 ID、包、严重级、接受理由、长期方案

## 8. 相关文档

- [test-coverage-gate.md](test-coverage-gate.md) — 升级后覆盖率门禁零回归验证
- [e2e-testing.md](e2e-testing.md) — CI 须先 `wails generate module` 再 build（Wails 升级后绑定重新生成）
