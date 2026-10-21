# mermaid 12 major 升级专项（audit 传递漏洞修复线）

> 父任务：10-21-verify-upgrade-batch（顺序 2，在 ci-push-loop 归档后启动）

## Goal

mermaid 11.17.2→12.2.0 major 升级。npm audit 剩余 3 组 high 中 2 组（dompurify<=3.4.15、katex 0.11.0-0.18.1）在 mermaid 11 传递链，mermaid 12 是唯一修复线。

## Requirements

1. **升前评估**：mermaid 12 changelog/breaking changes 逐条核对（渲染 API `mermaid.initialize`/`mermaid.render`、配置项、ESM 导出面）；grep 本仓消费点（markdown 预览内嵌图表链路 + 懒加载——perf-baseline 有 mermaid 懒加载先例），列 API 消费清单落 research
2. **升级操作**：tilde 档声明 `~12.2.0`（以 lock 落点重写）→ npm install 同步 → npm ci 验证；消费面代码按 breaking 清单适配
3. **回归面**：
   - SOP §3.3 全项必跑面（一致性检查 / vitest 全量 / E2E web+desktop / npm audit）
   - mermaid 专项：markdown 预览渲染三类型图（流程图/时序图/甘特图）人工验证；若无自动化用例补 1 条 E2E 冒烟（markdown 文件预览含 mermaid 代码块 → SVG 渲染出现）
   - katex/dompurify 漏洞清零核对：audit 预期 6→≤4
4. **沉淀**：12.x 消费面坑（配置迁移/渲染时机）按 trellis-update-spec 沉淀；security-scan.md「已知接受风险」节同步 audit 新基线

## Acceptance Criteria

- [x] mermaid 12.2.0 落 lock 且检查脚本三维一致绿
- [x] 消费面适配完成，渲染专项验证通过（三类型图：E2E 冒烟 markdown-mermaid.spec.js，真实 Chromium SVG 断言）
- [x] audit 无 mermaid 传递链漏洞（实际 2 low → **0**，优于 ≤2 口径；katex 0.18.11 出漏洞区间）
- [x] vitest / E2E 全量零回归（vitest 96 文件 2022 用例 / E2E web 88 / desktop 3）
- [x] security-scan.md 同步；消费面坑已沉淀 docs/spec/mermaid-rendering.md

## Definition of Done

- AC 全勾；文档更新；CI 全 job 绿

## Open Questions（brainstorm 待定）

- mermaid 12 breaking 清单实际内容与本仓命中面
- E2E mermaid 冒烟用例是否补（视现有 markdown 预览 E2E 覆盖）

## Out of Scope

- vue-router 5 / js-yaml 5 / markdown-it 15 / docx-preview 0.4（各自专项）
- markdown 预览其他渲染链改动

## Technical Notes

- mermaid 懒加载先例：perf-baseline（首屏 eager 1.8MB/gzip 550KB 减载）
- audit 留痕：archive/2026-10/10-21-tilde-upgrade/research/upstream-inventory.md「audit 剩余 high 与范围外升级项对应关系」表
- tilde 声明重写规则：`~<lock 落点 minor>.0`
