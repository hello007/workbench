# 前端依赖版本漂移治理（EP caret 漂移根源收口）

## Goal

element-plus `^2.13.7` caret 范围下 lock 随依赖操作漂移至 2.14.5，minor 漂移已造成两轮回归排查成本（10-20-e2e assets-export 断言修复 + 10-20-ep-2-14-5-e2e 全仓 48 消费点排查）。收口根源：分档钳制策略（package.json 修订 + lock 同步）+ CI「声明范围 vs lock 实际版本」一致性检查防再发 + 前端依赖升级 SOP 沉淀。

## Requirements

1. **分档钳制落地**（package.json + lock 一致，详单见 research/dep-inventory.md）：
   - exact（2）：element-plus `2.14.5`、@element-plus/icons-vue `2.3.2`
   - tilde（28）：运行时 UI 面，声明值按 lock 现状 minor 重写（`~x.y.0` 形态）
   - caret（8）：测试/构建工具链维持现状（7 devDeps + @pinia/testing）
   - url（1）：xlsx CDN tarball 不动
2. **CI 一致性检查**：`scripts/check-dep-consistency.mjs`（零依赖 node 脚本）+ ci.yml test job 早期 step
   - 校验：每依赖必归档（未登记新依赖 fail 强制归类）、声明格式与档匹配、lock 版本满足档语义（exact 相等 / tilde 同 minor / caret 范围内）
   - `--selftest`：内存 fixture 注入各档漂移形态断言必拦 + 合形态必过，CI 每次跑自证检查器有效
   - 漂移注入自测：临时改 lock 漂移形态验证脚本 fail（一次性人工验证 + selftest 常驻双保险）
3. **升级 SOP 沉淀**：docs/spec/frontend-dep-upgrade.md（分档表、升级前 changelog 检查、升级操作、升级后必跑面：vitest 全量 + E2E 全量 + 确认弹框哨兵 5 spec 清单、EP 升级专项联动 confirm-dialog.md §1 方法论）+ CLAUDE.md 关键规则行

## Acceptance Criteria

- [ ] package.json 分档修订完成，lock 同步后版本零变化（tilde/exact 声明值取 lock 现状）
- [ ] `npm ci && npm run build` 通过；vitest 全量零回归；E2E 全量零回归
- [ ] CI 一致性检查 step 绿；--selftest 绿
- [ ] 漂移注入自测：临时篡改 lock（exact 版本错位 / tilde 越 minor / caret 越范围三形态）逐一触发 fail，恢复后绿
- [ ] docs/spec/frontend-dep-upgrade.md 完成，confirm-dialog.md §5 互链，CLAUDE.md 关键规则行登记

## Definition of Done

- 全部 AC 勾选；CI 全 job 绿；文档更新（README.md 如涉及）

## Technical Approach

**分档语义**（npm 范围语法原生，无新工具）：
- exact `2.14.5`：仅显式版本号升级可变，根治漂移
- tilde `~6.11.0`：patch 自动，minor 升级须显式改声明（lock 重新审查入口）
- caret `^1.63.0`：工具链保持灵活（测试环境漂移影响面小）

**脚本设计**（scripts/check-dep-consistency.mjs，node ≥18 ESM 零依赖）：
- 内置显式分档清单（EXACT/TILDE/CARET/URL 四集合，39 项全登记）；未登记依赖 → fail 强制人审归类（防再发核心：新依赖默认不静默放行）
- 解析 frontend/package.json + package-lock.json（lockfileVersion 3 `packages` 块），逐项校验档×声明格式×lock 版本三维一致
- semver 判定自实现（caret/tilde 范围语义简单，不引依赖；0.x caret 特例语义纳入）
- `--selftest`：进程内构造临时 fixture 目录（假 package.json/lock），注入越档漂移（exact 错位/tilde 越 minor/caret 越范围）断言 exit≠0，合形态断言 exit=0

**CI 插入点**：test job「检出代码」后新增 step（纯读两 JSON 文件，不依赖 npm ci/Node 安装前置，快速失败）：
```yaml
- name: 前端依赖分档一致性检查
  run: |
    node scripts/check-dep-consistency.mjs --selftest
    node scripts/check-dep-consistency.mjs
```

**lock 同步操作**：package.json 修订后 `npm install --package-lock-only` 刷新元数据（版本不变），本机 `npm ci` 验证可复现。

## Decision (ADR-lite)

**Context**：caret 全覆盖下 lock 范围内重解析是 npm 设计内行为，EP minor 漂移两轮排查成本暴露「UI 组件库不可裸漂」约束；39 依赖一刀切 exact 维护成本高（每次升级都显式改版本）。
**Decision**：三档分流——exact 只收事故源 EP 全家（2 项）；tilde 收运行时 UI 面（28 项，patch 漂移低风险可自动）；caret 留测试/构建工具链（8 项，测试环境影响面小）。EP 钳 2.14.5（lock 现状，已过全仓排查 + E2E 修复，唯一已验证状态；退 2.13.7 反而引入未验证态）。CI 未登记依赖默认 fail 强制归类。
**Consequences**：UI 面 minor 漂移根治；升级 EP 须显式改版本号 + 跑 SOP 必跑面（成本前移到升级时点，符合「EP 升级必全量回归」现实）；工具链仍可 caret 自动更新，vitest/playwright 升级随 npm 操作漂移接受（E2E/vitest 全量在 CI 兜底）。

## Out of Scope

- 后端 Go 依赖治理（govulncheck 已覆盖）
- node_modules 传递依赖版本治理（只管顶层声明）
- npm overrides /引擎钉扎（packageManager 字段、engines）等包管理器级约束
- renovate/dependabot 等自动升级机器人引入

## Research References

- [research/dep-inventory.md](research/dep-inventory.md) — 39 依赖逐项盘点、分档定案、哨兵用例清单、CI 插入点

## Technical Notes

- tilde 声明值必须取 lock 现状 minor（原 caret 声明值的 lock 已漂 minor，`~<原声明值>` 会使 lock 越档 CI 立刻红）
- 0.x 包 tilde 语义 = minor+patch 全锁（npm 特例），xterm/docx-preview 风险天然低
- 上一任务归档：.trellis/tasks/archive/2026-10/10-20-ep-2-14-5-e2e/；契约：docs/spec/confirm-dialog.md
- 48 消费点核对表：上一任务 research/ep-confirm-consumers.md
