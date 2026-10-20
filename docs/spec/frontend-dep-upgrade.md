# 前端依赖分档钳制与升级 SOP

> 沉淀来源：任务 10-20-ep-caret（2026-10-20）。背景：element-plus 以 `^2.13.7` 声明，lock 随依赖操作在 caret 范围内重解析漂移至 2.14.5（minor 漂移），造成两轮回归排查成本。根治手段 = 分档钳制 + CI 一致性检查 + 本 SOP 的升级规程。

## 1. 分档策略

**关键结论：漂移根源不是 lock 失效，而是 caret 范围内重解析（npm 设计内行为）——任意 `npm install` 依赖操作都可能把范围内依赖拉到最新 minor。CI 检查脚本 `scripts/check-dep-consistency.mjs` 强制「档位登记 × 声明格式 × lock 实际版本」三维一致，未登记新依赖一律失败。**

| 档位 | 声明形态 | lock 允许漂移 | 适用面 | 当前清单 |
|---|---|---|---|---|
| exact | `2.14.5` | 禁止（严格相等） | UI 组件库核心（事故源） | element-plus、@element-plus/icons-vue |
| tilde | `~x.y.0` | 仅 patch（同 minor） | 运行时 UI 面 | vue、pinia、vue-router、echarts、vue-echarts、mermaid、codemirror 全家（11 包）、@vueuse/core、xterm 三件、highlight.js、markdown-it、dayjs、js-yaml、docx-preview、splitpanes、vue-draggable-plus |
| caret | `^x.y.z` | minor + patch（范围内） | 测试/构建工具链 | vitest、@vitest/coverage-v8、vite、@vitejs/plugin-vue、jsdom、@vue/test-utils、@playwright/test、@pinia/testing |
| url | tarball URL | 天然 exact | CDN 分发依赖 | xlsx（已知无补丁漏洞接受项，见 security-scan.md） |

分档依据：

- **exact 收事故源**：element-plus 是唯一造成过回归成本的依赖（message-box 行为漂移，见 confirm-dialog.md §1），且直接构成用户可见 UI 面。升级成本前移到升级时点（显式改版本号 + 全量回归），符合「EP 升级必全量回归」的现实约束。
- **tilde 收运行时面**：patch 漂移低风险可自动；minor 升级须显式改声明，强制触发 lock 审查入口。0.x 包（xterm、docx-preview）tilde 语义 = minor+patch 全锁（npm 原生特例），风险天然更低。
- **caret 留工具链**：测试/构建环境漂移影响面小（vitest/E2E 全量在 CI 兜底），保留 npm 自动更新便利性。

## 2. CI 一致性检查

`scripts/check-dep-consistency.mjs`（零依赖 node 脚本，ci.yml test job 步骤 2.5，checkout 后立跑快速失败）：

- 校验规则：每依赖须在 exact/tilde/caret/url 四清单之一登记（未登记 → 失败强制人工归类）；声明格式与档匹配（exact 纯版本号 / tilde `~` 前缀 / caret `^` 前缀）；lock 版本满足档语义（exact 相等 / tilde 同 minor 且 ≥ 声明 patch / caret 范围内，0.x caret 特例语义已覆盖）。
- `--selftest`：注入 11 组漂移/合法 fixture 自证检查器有效（exact 错位、tilde 越 minor、caret 越 major、0.x caret 越 minor、0.0.x caret patch 漂移、未登记、lock 缺条目等必拦；patch 漂移、范围内 minor 必过）。**修改检查脚本判定逻辑后必须先过 selftest**。
- 新增依赖操作：先在脚本四清单中登记（按第 1 节分档依据选档），再改 package.json，否则 CI 失败。

## 3. 升级操作规程

### 3.1 升级前

1. 查目标版本 changelog / breaking changes：element-plus 看 [CHANGELOG.en-US.md](https://github.com/element-plus/element-plus/blob/dev/CHANGELOG.en-US.md)（minor 版本重点看 message-box / dialog / form 等交互组件行为变更）；其他依赖看 GitHub Releases。
2. 确认目标版本与本仓消费面兼容：grep 组件库变更涉及组件在本仓的使用点（EP 全仓 24 组件 48 处 confirm 消费点核对表见 `.trellis/tasks/archive/2026-10/10-20-ep-2-14-5-e2e/research/ep-confirm-consumers.md`）。

### 3.2 升级操作（按档）

| 档位 | 操作 |
|---|---|
| exact | 改 package.json 精确版本号 → `npm install --package-lock-only` 同步 lock 元数据 |
| tilde | patch 自动漂移无需操作；minor 升级改声明为 `~x.(y+1).0` 形态（**以 lock 实际落点 minor 重写**）→ 同步 lock |
| caret | 通常无需操作（范围内自动）；跨 minor/major 升级改 `^` 声明值 → 同步 lock |

同步后本机 `npm ci && npm run build` 验证可复现构建。

### 3.3 升级后必跑面（全部通过才算升级完成）

1. **一致性检查**：`node scripts/check-dep-consistency.mjs --selftest && node scripts/check-dep-consistency.mjs`
2. **vitest 全量**：`cd frontend && npm test -- --run`（约 2000+ 用例）
3. **E2E 全量**：`cd frontend && npm run e2e`（web 通道）+ `npm run e2e:desktop`（desktop 通道，Windows 本机）
4. **确认弹框哨兵子集**（EP 升级或任何含弹框行为组件库升级时必跑，其余升级随 E2E 全量覆盖）：
   - `e2e/assets-export.spec.js`（递归恒弹形态，暴露面最大）
   - `e2e/assets-backup-rules.spec.js`（顺序两段形态）
   - `e2e/assets-relink.spec.js`
   - `e2e/file-tree.spec.js`
   - `e2e/submodule.spec.js`
5. **安全扫描**：`npm audit --registry=https://registry.npmjs.org --audit-level=high`（本地 npmmirror 不支持安全端点须绕过）；升级后重跑确认无新增漏洞。

### 3.4 EP 升级专项

element-plus 升级除 3.3 常规面外，按 confirm-dialog.md §1 排查方法论执行：

1. 三形态分类学核对（递归恒弹 / 顺序两段 / 单次确认）——仅前两形态有升级暴露面；
2. vitest 面 ElMessageBox 被 spec 文件级 mock 屏蔽，断言以组件代码分支为准；
3. 真暴露面 = E2E 可达的「同链路可能弹第二次 confirm」消费点 + 裸 `.el-message-box` 选择器断言（2.14+ 实例驻留 DOM 致 strict mode 多元素命中）；
4. E2E 断言规则按 confirm-dialog.md §4（`.first()`/`.last()` 或 `:visible` + `filter({ hasText })`）。

### 3.5 回退预案

升级失败回退 = `git checkout frontend/package.json frontend/package-lock.json` + `npm ci`（lock 与声明同源回滚，无半态）。exact 档回退后版本即回到上一个已验证态。

## 4. 关联

- 确认弹框消费契约与 EP 回归排查方法论：[confirm-dialog.md](confirm-dialog.md)
- E2E 通用契约：[e2e-testing.md](e2e-testing.md)
- 依赖安全扫描：[security-scan.md](security-scan.md)
- 测试稳定性约束：[test-stability.md](test-stability.md)
