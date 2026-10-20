# 前端 tilde 档依赖上游 minor 走查升级

> 父任务：10-21-hardening-batch（顺序 2）

## Goal

10-20-ep-caret 将 28 项运行时 UI 面依赖钳制为 tilde。本任务逐项盘点上游最新版本（已完成，见 research/upstream-inventory.md），对有价值项升级吸收修复，major 级跳过留痕。

## 盘点结论（2026-10-21）

- 升级批 7 项：patch 5（@codemirror/commands、state、view、vue、vue-echarts——tilde 范围内自动）+ minor 2（@codemirror/language 6.13.1、pinia 4.1.0——显式改声明）
- 跳过批 5 项：js-yaml 5 / markdown-it 15 / mermaid 12 / vue-router 5（major）+ docx-preview 0.4.1（0.x minor 视同 major）——理由留痕 research
- 已最新 16 项不动

## Requirements

1. **升前 changelog 检查**：minor 批 2 项拉 changelog（GitHub Releases）确认无 breaking；若发现 breaking 影响本仓消费面，停下报告不得强升。patch 批 5 项快速过一眼变更摘要
2. **升级操作**：patch 批 `npm update <5 项>`；minor 批改 package.json（`~6.13.0` / `~4.1.0` 形态，以升级后 lock 落点核对满足）→ `npm install --package-lock-only` → `npm ci` 验证可复现
3. **必跑面**（SOP §3.3 全项）：一致性检查 + selftest、vitest 全量、E2E web + desktop 全量、npm audit（官方 registry）；pinia 是状态管理核心且 EP 弹框链路涉及 store，确认弹框哨兵 5 spec 一并跑
4. **lock 核对**：升级后声明与 lock 三维一致（检查脚本绿即证）

## Acceptance Criteria

- [ ] 7 项升级完成，`node scripts/check-dep-consistency.mjs` 绿
- [ ] vitest 全量 + E2E web/desktop 全量 + 哨兵 5 spec 零回归
- [ ] npm ci 可复现构建
- [ ] npm audit 无新增 high 漏洞
- [ ] research 盘点表完整（含跳过理由与 changelog 检查结论）

## Definition of Done

- AC 全勾；CI 全 job 绿（push 后）；无新坑则文档零改动（SOP 已覆盖本流程）

## Decision (ADR-lite)

**Context**：28 项盘点后升级面仅 7 项（patch 5 + minor 2），major 5 项破坏面大须专项。批间无依赖冲突。
**Decision**：一次升级动作合并 7 项，一次全量必跑面（不按域分批——原 PRD 设想的 codemirror 批/散项批合并）；major 5 项跳过留痕不评估。
**Consequences**：改动面与验证成本最小化；major 升级（vue-router 5/mermaid 12/js-yaml 5/markdown-it 15/docx-preview 0.4）留待专项任务按需启动。

## Out of Scope

- major 升级评估（vue-router 5、mermaid 12、js-yaml 5、markdown-it 15、docx-preview 0.4）
- element-plus（exact 档）升级
- caret 档（工具链）版本变动

## Research References

- [research/upstream-inventory.md](research/upstream-inventory.md) — 28 项盘点全表（升级批/跳过批/已最新）与升级策略

## Technical Notes

- tilde 语义：patch 自动漂移（npm update 拉 patch 不改声明）；minor 须显式改声明 `~x.(y+1).0`
- 0.x 包 tilde = minor+patch 全锁；0.x minor 视同 major 处置
- 检查脚本未登记依赖会红——本任务不新增依赖，无归类动作
