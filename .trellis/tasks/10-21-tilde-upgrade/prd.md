# 前端 tilde 档依赖上游 minor 走查升级

> 父任务：10-21-hardening-batch（顺序 2，在 goleak-gate 归档后启动）

## Goal

10-20-ep-caret 已将 28 项运行时 UI 面依赖钳制为 tilde（锁 minor 放 patch），声明值取 lock 当时现状（如 @vueuse/core ~12.8.2、mermaid ~11.17.2、@codemirror/state ~6.7.4）。逐项查上游最新版本，有价值的分批升级吸收修复，无价值的记录跳过理由。

## Requirements

1. **盘点**：逐项查 npm registry 上游最新版本，输出「当前 lock vs 上游最新」差距表（research 持久化），标注 changelog 价值项（bug 修复/安全/性能）
2. **分批升级**：有价值项按依赖域分批（codemirror 全家一批 / EP 无关散项一批等，批次划分 brainstorm 定）；每批 tilde 声明值以升级后 lock 落点 minor 重写（`~x.(y+1).0` 形态）+ npm install 同步 lock
3. **每批必跑 SOP 必跑面**（docs/spec/frontend-dep-upgrade.md §3.3）：一致性检查 + vitest 全量 + E2E web/desktop 全量；涉及弹框行为组件的批次加跑确认弹框哨兵 5 spec
4. **跳过决策留痕**：无价值项（纯内部重构上游版本）记录跳过理由，不升级

## Acceptance Criteria

- [ ] 上游差距盘点表落 research（28 项全覆盖）
- [ ] 升级批次全部过 SOP 必跑面，vitest + E2E 零回归
- [ ] package.json/lock 一致且 `node scripts/check-dep-consistency.mjs` 绿
- [ ] 跳过项理由留痕 research
- [ ] 升级中若发现新坑（如有行为漂移），按 trellis-update-spec 沉淀（无坑不强制）

## Definition of Done

- AC 全勾；CI 全 job 绿；文档按需更新

## Open Questions（brainstorm 待定）

- 批次划分粒度（按依赖域几批合适）
- 上游 major 跳变项（如有）是否顺带评估（默认 Out of Scope，major 需专项）

## Out of Scope

- caret 档（工具链 8 项）版本对齐（ADR 已定策保持灵活）
- 上游 major 版本升级评估
- element-plus（exact 档）升级（须走 SOP §3.4 专项，单独立任务）

## Technical Notes

- 声明值重写规则：`~<升级后 lock 落点 minor>.0`；`~<原声明值>` 会因越档 CI 红
- 0.x 包 tilde = minor+patch 全锁（npm 特例），升级须显式改声明
- 升级命令链：改 package.json → `npm install --package-lock-only` → `npm ci` 验证可复现
