# dev-only moderate 依赖治理

## Goal

出清 npm audit 当前 8 个 dev-only moderate 漏洞：vitest 4.1.12 patch 修复 @vitest/mocker 两条 advisory；glob / minimatch 通过 lockfile 内 overrides 越过上游未放宽的依赖声明。目标 `npm audit --audit-level=moderate` 归零，且前端单测、构建、E2E 无回归。

## Requirements

* 升级 vitest 工具链至 4.1.12（`vitest` / `@vitest/coverage-v8` / 传递 `@vitest/mocker`）；caret 声明随安装更新为 `^4.1.12`（仍按 caret 档管理）。
* 在 `frontend/package.json` 增加 overrides：`glob ^14.0.3`、`minimatch ^11.0.4`，强制替换 js-beautify / editorconfig 的过期传递依赖。
* 升级后 `npm audit --registry=https://registry.npmjs.org --audit-level=moderate` 必须 0 条。
* 更新 `docs/spec/security-scan.md` §5.6：清单改为已出清，记录 overrides 依据、上游复评条件与回退方法。
* 更新 `docs/spec/frontend-dep-upgrade.md`：新增「传递依赖 overrides 使用口径」小节（仅安全修复、必须全量回归、上游放宽后移除）。
* 不修改顶层依赖分档登记；`check-dep-consistency.mjs` 保持通过。

## Acceptance Criteria

* [ ] `npm audit --audit-level=moderate` 输出 `found 0 vulnerabilities`。
* [ ] `node scripts/check-dep-consistency.mjs --selftest && node scripts/check-dep-consistency.mjs` 通过。
* [ ] `cd frontend && npm test -- --run` 全绿（约 2400 用例）。
* [ ] `cd frontend && npm run build` 成功。
* [ ] `cd frontend && npm run e2e` 全绿；`npm run e2e:desktop` 在 Windows 本机构建后全绿（若桌面构建环境不可用，须明确记录未验证项，不得声称通过）。
* [ ] overrides 生效后 `node_modules/glob/package.json` 为 14.0.x、`node_modules/minimatch/package.json` 为 11.0.x。
* [ ] 文档口径与 package.json / lockfile 一致，无「8 个 moderate 接受」残留。

## Definition of Done

* 安全扫描零 moderate；测试、构建、E2E 回归通过。
* 文档记录 overrides 原因、风险、移除条件与回退命令。
* 不引入新的顶层依赖。

## Technical Approach

1. **vitest patch**：`cd frontend && npm update vitest @vitest/coverage-v8`，lock 内升到 4.1.12，`@vitest/mocker` 随同。
2. **overrides**：

   ```json
   "overrides": {
     "glob": "^14.0.3",
     "minimatch": "^11.0.4"
   }
   ```

   `npm install` 重写 lock；验证 `npm ls glob minimatch` 与实际 node_modules 版本。
3. **兼容性取证（升前）**：
   * js-beautify 2.0.3 仅 CLI 使用 `globSync`，glob 14 保留 `globSync` 导出；
   * editorconfig 使用 `new minimatch.Minimatch()`、`minimatch.escape()`，minimatch 11 保留两个导出；
   * 上游最新 js-beautify 仍声明 `glob ^13.0.6`、editorconfig 仍声明 `minimatch ~10.2.4`，无上游修复版本。
4. **回归**：一致性检查 → vitest 全量 → build → E2E web/desktop。
5. **收尾**：更新安全扫描与升级 SOP，CLAUDE.md 规则行仅在需要时补「overrides 仅安全修复」钩子。

## Decision (ADR-lite)

**Context**: 8 个 moderate 全在 dev 测试链；vitest 有 patch 修复，glob/minimatch 上游未放宽声明，只能选择继续接受或 overrides 强制升级。

**Decision**: vitest 升 4.1.12；glob/minimatch 采用根级 overrides 强制到修复版，以全量回归证明兼容。若任一必跑面失败，撤回 overrides 并恢复 §5.6 接受清单（保留 vitest 升级收益）。

**Consequences**: 漏洞清零、无需人工跟踪；overrides 绕过上游 semver 声明，升级 js-beautify/editorconfig 时须复核并移除 override；风险由全量测试与 E2E 兜底。

## Out of Scope

* vitest 5 / glob 15 等 major 升级。
* 替换 js-beautify 或 @vue/test-utils。
* 生产依赖调整。
* 修改 CI `--audit-level=high` 策略。

## Technical Notes

* 当前：vitest 4.1.11、@vitest/mocker 4.1.11、glob 13.0.6、minimatch 10.2.6、js-beautify 2.0.3、editorconfig 3.0.2、@vue/test-utils 2.5.0。
* 上游最新：vitest 5.1.0（major，不升）、glob 14.0.3、minimatch 11.0.4、js-beautify 2.0.3、@vue/test-utils 2.5.1。
* Node 约束：glob 14 `18 || 20 || >=22`；minimatch 11 `20 || >=22`；本机 Node 24，CI ≥22.12。
* 策略文档：`docs/spec/security-scan.md` §3.2/§5.6；`docs/spec/frontend-dep-upgrade.md` §3.2/§3.3。
* 禁 `npm audit fix` / `--force`（既有规则）。
