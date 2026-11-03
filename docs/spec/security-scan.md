# 安全扫描

> 本文档规定 WorkBench 的依赖安全扫描方案：工具选型、本地命令、CI 集成、已知漏洞处置策略。
> 最后更新：2026-11-03 · 来源任务：11-03-npm-audit-remediation（npm undici high 治理）

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

执行口径（2026-11-03 基线）：high 0、moderate 8（全部为 dev 测试工具链，清单与接受理由见 §5.6）。

- 修复 high 用 `npm update <pkg>` 在 lockfile 内升级，保持 npmmirror resolved 风格，不改 `package.json` 顶层声明与四档登记。
- **禁 `npm audit fix` / `npm audit fix --force`**：前者会连带升级 vitest 等工具链（本轮实测可把 vitest 4.1.11 拉到 4.1.12），后者可跨 major，均触发未评估的回归面；升级按 [frontend-dep-upgrade.md](frontend-dep-upgrade.md) SOP 独立进行。
- moderate 不自动修复，按 §5.6 接受并跟踪复评触发条件。

## 4. CI 集成

`.github/workflows/ci.yml` 新增 `security` job：

- 与 `test` job 并行，不阻塞测试
- `continue-on-error: true`：发现 high 漏洞时标记 job 失败但不阻塞 PR 合并（信息性扫描，漏洞在 PR check 可见）
- Go 版本与 test job 一致（`1.26`，配合 go.mod `toolchain go1.26.10`）
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

### 5.2 已修复（2026-10-21 升级轮）

**Go（代码可达 11 → 0，全部清零；Module/Package 级 4 → 1，残留见 §5.3）**：

> 漏洞来源：GitHub 镜像仓（hello007/workbench）CI run#13 security job「Go 漏洞扫描」红点 + 本地 `govulncheck ./...` 复现；逐 ID 清单以该 run 的 security job 输出为准。

| 漏洞 ID | 模块 / 包 | 修复方式 |
|---|---|---|
| GO-2026-6809（crypto/x509，须 go1.26.10）等 11 个代码可达标准库漏洞，其余 GO-2026-6603/6605/6607/6608/6609/6610/6611/6612/6613/6617 在 go1.26.9 已修复 | crypto/x509、net/http、crypto/tls、mime/multipart、net/textproto 等标准库 | Go 工具链 go1.26.6 → go1.26.10（go.mod `toolchain` directive，唯一修法） |
| GO-2026-6600/6599（stdlib）、GO-2026-6604（os）——Package/Module 级不可达，go1.26.9 已修复 | 标准库 | 同上，随本轮工具链升级一并清零 |
| GO-2026-6603/6610/6611/6612/6617 的 x/net 模块形态（均 Module 级不可达；可达性来自同 ID 的标准库 net/http 形态，已计入上行） | golang.org/x/net v0.59.0 → v0.60.0（indirect） | 依赖升级（minor） |

残留 1 个 Module 级提示：GO-2026-5932（`golang.org/x/crypto/openpgp` 包停止维护，`Fixed in: N/A` 无修复版本），代码不可达（Symbol/Package 级均 0），计入 §5.3 接受风险。

**npm（2 low → 0，全部清零）**：

> mermaid 传递链：katex 0.16.47 落在 GHSA-238p-pmpm-9mq7（prototype pollution）影响区间 0.11.0–0.18.1，audit 另报 mermaid 10.9.0-rc.1–12.1.0「Depends on vulnerable versions of katex」衍生提示；唯一修复线为 mermaid 12 major（12.2.0 起 katex 依赖升 `^0.18.0`）。

| 漏洞 ID | 包 | 修复方式 |
|---|---|---|
| GHSA-238p-pmpm-9mq7 Prototype Pollution | katex 0.16.47 → 0.18.11（mermaid 传递链，indirect） | mermaid 11.17.2 → 12.2.0 major 升级（2026-10-21）；实测 high 0（moderate 见 §5.6） |

### 5.3 历史接受风险（已处置清零）

| 漏洞 ID | 包 | 严重级 | 处置 |
|---|---|---|---|
| GO-2026-5932（openpgp 包停止维护、设计上不安全） | golang.org/x/crypto v0.57.0（openpgp 子包，本项目未 import） | 未定级（弃用通告，Fixed in: N/A 无补丁） | ⏸ 持续接受（2026-10-21 复核：代码不可达，Symbol/Package 级均 0） |
| GHSA-4r6h-8v6p-xvw6 Prototype Pollution | xlsx 0.18.5 → 0.20.3 | high | ✅ 已修复（2026-09-23） |
| GHSA-5pgg-2g8v-p4x9 ReDoS | xlsx 0.18.5 → 0.20.3 | high | ✅ 已修复（2026-09-23） |

**背景**：SheetJS 官方已将发布迁移至自建 CDN（https://cdn.sheetjs.com），npm registry 上的 `xlsx` 包冻结在 0.18.5，故 `npm audit` 长期报 `No fix available`。两个漏洞（CVE-2023-30533 原型污染等）均在 0.19.3+ 修复。

**处置方式（2026-09-23）**：`npm install https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz`，`package.json` 中 `xlsx` 依赖变为 CDN tarball URL。import 路径 `xlsx` 不变，业务代码零改动；`FilePreviewRenderer.vue` 的 Excel 只读渲染经全量前端测试（1369 用例）与生产构建验证兼容。

**注意事项**：
- CDN tarball URL 依赖 cdn.sheetjs.com 可达，CI（GitHub Actions Linux runner）与本地安装均需网络访问该域名。
- `npm audit` 按 lock 文件实际版本号（0.20.3）匹配 advisory 范围，实测 high 0（moderate 见 §5.6）。
- 若未来需再升级，先探测 CDN 版本存在性（0.20.4+ 并不存在，当前最新即 0.20.3），勿凭版本号臆测。

### 5.4 major 版本待评（本轮跳过）

前端遗留四 major 已于 2026-10-21 全部出清（vue-router ~5.4.0 / js-yaml ~5.5.0 / markdown-it ~15.0.3 / docx-preview ~0.4.1，breaking 评估与适配记录见 frontend-dep-upgrade.md §3.6），升级后 `npm audit` high 0（moderate 见 §5.6）。

**go-git 已于 2026-10-21 出清**：未升 v6（正式版未发布，最新 v6.0.0-beta.1 为 prerelease，生产不可依赖），而是评估后**整体移除 go-git 依赖**——消费面 5 处活跃点（HEAD SHA/分支名/ref 枚举/remote 列表四类元数据读取 + object 死代码删除）全部改为 CLI/纯 Go 等价实现（`util.ResolveHead` helper + `git for-each-ref` + 手写 .git/config `[remote "x"]` 子段解析，语义对齐 v5.19.2 `PlainOpen`/`Remotes()` 行为，含 `EnableDotGitCommonDir=false` 与「裸 `[remote]` 段不计」两处口径），go-git/go-billy/gcfg 依赖链全部出清，`govulncheck ./...` 清零。详见 `.trellis/tasks/10-21-go-git-v6-upgrade/research/`（消费面盘点 + v6 breaking 核对）。

本节剩余仅框架侧一项：

| 包 | 当前版本 | 可用 major | 说明 |
|---|---|---|---|
| wails | v2.16.0 | v3 | v3 仍在 beta，跟踪官方稳定版发布后再评 |

### 5.5 已修复（2026-11-03 npm audit 治理轮）

**npm（1 high → 0；moderate 保持 8，见 §5.6）**：

| 漏洞 ID | 包 | 修复方式 |
|---|---|---|
| GHSA-4qv8-2p9j-2685、GHSA-f72v-6w45-q472、GHSA-xjj6-f33q-929c、GHSA-4w7w-5xrc-9h26、GHSA-2m5v-pch6-74hx、GHSA-qrxm-fqhp-jwvm、GHSA-9fm3-97px-rjf6、GHSA-55wc-3pg8-jfw6（undici 聚合条目下的 8 个 advisory：4 high + 4 moderate，影响版本 <7.30.1） | undici 7.29.1 → 7.30.1（`vitest → jsdom` 传递链，dev-only，indirect） | `npm update undici`（jsdom 声明 `^7.25.0` 范围内）；lockfile 仅 version/resolved/integrity 三行变化，`package.json` 顶层声明未动 |

验证：`npm audit --registry=https://registry.npmjs.org --audit-level=high` 退出码 0；`npm ci` 可复现；前端单测（103 文件 / 2275 用例）与生产构建通过；`node scripts/check-dep-consistency.mjs` 通过。

### 5.6 当前接受风险：npm dev-only moderate（2026-11-03）

**结论：8 个 moderate 全部位于 devDependencies（vitest 测试框架、@vue/test-utils 测试工具链），不进入生产构建产物；`--audit-level=high` 刻意忽略，CI security job 不再因它们失败。**

根因 advisory（其余 audit 条目为依赖链衍生提示）：

| GHSA | 受影响包（当前版本） | advisory 范围 | 出清所需版本 | 阻塞原因 |
|---|---|---|---|---|
| GHSA-qxg6-89mg-rqv8（Code injection via unauthenticated manual mock registration） | @vitest/mocker 4.1.11 | >=2.1.0-beta.7 <4.1.12 | vitest / @vitest/mocker / @vitest/coverage-v8 4.1.12（patch） | 本轮范围收敛于 high；`^4.1.5` 已覆盖 4.1.12，lockfile 内 `npm update` 即可出清，但仍须全量单测回归，按 frontend-dep-upgrade.md SOP 独立升级轮处理 |
| GHSA-xp26-3454-mjgp（Windows backslash traversal bypasses @vitest/mocker redirect allowlist fix） | @vitest/mocker 4.1.11 | >=4.1.11 <4.1.12 | 同上 4.1.12 | 同上 |
| GHSA-3g3m-2fjr-6pw4（Glob AbortSignal cancellation bypass allows post-timeout filesystem traversal） | glob 13.0.6 | >=9.0.0 <=14.0.1 | glob 14.0.2+ | js-beautify 2.0.3（npm 最新）声明 `glob: ^13.0.6`，修复版 14.0.2 超出声明范围；须上游放宽依赖或强制 overrides |
| GHSA-g3hv-75wg-m249（minimatch DoS via globparts amplification） | minimatch 10.2.6 | <11.0.1 | minimatch 11.0.1+ | editorconfig 3.0.2 声明 `minimatch: ~10.2.4`、glob 13.0.6 声明 `minimatch: ^10.2.2`，修复版 11.x 超出声明范围 |

链路与衍生条目：

- vitest 链：`vitest 4.1.11 → @vitest/mocker 4.1.11`，audit 另报 `vitest`、`@vitest/coverage-v8`（依赖 vulnerable 版本）。
- js-beautify 链：`@vue/test-utils 2.5.0 → js-beautify 2.0.3 → editorconfig 3.0.2（minimatch ~10.2.4）+ glob 13.0.6（minimatch ^10.2.2）`，audit 另报 `js-beautify`、`editorconfig`、`@vue/test-utils`（依赖 vulnerable 版本）。

复评触发条件（满足任一即重评）：

1. js-beautify 或 @vue/test-utils 上游发布依赖 `glob ^14` / `minimatch ^11` 的版本；
2. 前端依赖升级轮（frontend-dep-upgrade.md §3.3 必跑面）顺带处理 vitest 4.1.12 patch 升级；
3. npm audit 策略改为 moderate 阻塞（调整 CI `--audit-level` 或失败语义）时启动专项；
4. 季度复核（§7）发现根因包发布修复版，或任一 advisory 升级为 high。

## 6. Go 工具链与标准库漏洞

govulncheck 报告的标准库漏洞（`net/url`、`crypto/tls` 等）**只能通过升级 Go 工具链修复**，无法通过 `go get` 修复。

- go.mod `go 1.26.0` 为最低兼容声明
- go.mod `toolchain go1.26.10` 指定实际使用的工具链版本（GOTOOLCHAIN=auto 自动下载）
- CI `setup-go` 安装 `1.26` 后，go 命令遵循 `toolchain` directive 自动切换到 1.26.10
- 升级工具链后须重跑 `govulncheck ./...` 确认标准库漏洞清零

## 7. 维护规则

- 新增 Go 依赖：提交前跑 `govulncheck ./...` 确认无新增代码受影响漏洞
- 新增 npm 依赖：提交前跑 `npm audit --registry=https://registry.npmjs.org --audit-level=high` 确认无 high 漏洞
- Go 工具链升级：`toolchain` directive 须对齐 govulncheck 报告的最高标准库修复版本
- 季度复核：每季度跑一次全量扫描，更新本文档 §5 漏洞清单（含 §5.6 dev-only moderate 清单）
- 新增无补丁漏洞：按 §5.3 格式记录漏洞 ID、包、严重级、接受理由、长期方案

## 8. 相关文档

- [test-coverage-gate.md](test-coverage-gate.md) — 升级后覆盖率门禁零回归验证
- [e2e-testing.md](e2e-testing.md) — CI 须先 `wails generate module` 再 build（Wails 升级后绑定重新生成）
