# npm audit high 治理（undici）

## Goal

清除 CI `security` job 的 npm audit high 失败；对 8 个 moderate 做归因与文档登记，避免 major 升级风险。

## What I already know

- CI `security` job 失败步骤为「前端漏洞扫描」：`npm audit --registry=https://registry.npmjs.org --audit-level=high`。
- 本地复现：9 vulnerabilities（8 moderate, 1 high）。
- 唯一 high：`undici 7.29.1`，dev 依赖，链路 `vitest@4.1.11 → jsdom@29.1.1 → undici@^7.25.0`；advisory 范围 7.0.0–7.30.0，修复版 7.30.1（仍在 jsdom `^7.25.0` 范围内）。
- moderate 归因：
  - `@vitest/mocker` / `vitest` / `@vitest/coverage-v8`：需 vitest 5.x（major）才出清。
  - `@vue/test-utils` / `js-beautify` / `editorconfig` / `glob` / `minimatch`：链路 `@vue/test-utils → js-beautify → editorconfig → minimatch/glob`，出清需 js-beautify 2.x 等 major 变更。
  - 全部为 devDependencies / 测试工具链，不进入生产构建产物。
- 项目策略：`--audit-level=high` 刻意忽略 moderate 噪声；security job `continue-on-error` 不阻塞 PR。

## Requirements

- R1：`frontend/package-lock.json` 将 `undici` 从 7.29.1 升到 7.30.1（`npm update undici`，不改 `package.json` 顶层依赖与四档登记）。
- R2：不改动 vitest / @vue/test-utils 等 major 链路；8 个 moderate 保持现状。
- R3：`docs/spec/security-scan.md` 更新：
  - 「已知接受风险」补 dev-only moderate 清单（包名、advisory、阻塞升级原因、复评触发条件）。
  - 修正「升级后 npm audit 0 vulnerabilities」的过期表述（改为 high 0、moderate N 且均为 dev 工具链）。
- R4：验证：`npm audit --registry=... --audit-level=high` 退出码 0；`npm ci` 可复现；前端单测与构建通过；`node scripts/check-dep-consistency.mjs` 通过。

## Acceptance Criteria

- [ ] 本地 `npm audit --registry=https://registry.npmjs.org --audit-level=high` 退出码 0。
- [ ] `npm ls undici` 显示 7.30.1，且 `package.json` 顶层依赖未变。
- [ ] lockfile diff 仅 undici 版本/完整性/resolved 变化（若 npm 同时调整其传递条目，需逐条说明）。
- [ ] `cd frontend && npm test -- --run` 通过。
- [ ] `cd frontend && npm run build` 通过。
- [ ] `node scripts/check-dep-consistency.mjs` 通过。
- [ ] security-scan.md 记录 moderate 清单与升级阻塞原因。

## Definition of Done

- 仅动 lockfile 与文档；无业务代码改动。
- CI security job 复跑时应仅剩 moderate（不再失败）。
- 提交、归档。

## Decision (ADR-lite)

**Context**：high 依赖可无损修复；moderate 全部位于 dev 测试工具链，出清需 vitest 5 / js-beautify 2 等 major，收益低且回归风险高（vitest 5 影响全部前端单测）。

**Decision**：MVP 只修 high（`undici` 7.30.1，lockfile 内升级），moderate 文档登记并给出复评触发条件（上游 jsdom/vitest 适配或安全等级提升时再评估）。

**Consequences**：CI high 清零；moderate 长期存在但被 `--audit-level=high` 忽略；若未来策略改为 moderate 阻塞，需启动 vitest 5 / js-beautify 2 升级专项。

## Out of Scope

- vitest 5 / @vue/test-utils / js-beautify major 升级。
- npm audit 阈值调整、CI security job 阻塞策略调整。
- Go 侧 govulncheck（本次已通过）。

## Technical Notes

- 相关文档：`docs/spec/security-scan.md` §3.2、§已知接受风险；`docs/spec/frontend-dep-upgrade.md`（四档钳制与升级 SOP）。
- 升级命令：`cd frontend && npm update undici`（保持 lock 的 npmmirror resolved 风格）。
- 验证命令见 AC；不要用 `npm audit fix`（会连带 major/其它工具链变更）。
