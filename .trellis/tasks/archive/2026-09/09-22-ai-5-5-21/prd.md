# AI 对话审核缺陷修复

## Goal

修复 AI 对话工作台（上一任务 09-22-ai-aichatbox）三路代码审核发现的全部缺陷：5🔴 5🟠 21🟡。发现清单见 [audit-findings.md](audit-findings.md)（逐项含位置/问题/修复方向），为准需求源。

## Requirements

- 31 项发现全部处置：🔴🟠 必修；🟡 全部修复（含文档 3 项与测试改进项）
- 修复不改变既有对外契约（wailsjs 方法签名、事件名、存储布局），行为修复以最小改动为准
- 每项修复补对应回归测试（后端 go test / 前端 vitest）；E2E mock 时序项修 mock 时序并补生产时序用例
- 修后全量验证：go build/vet/test、npm build/test:coverage/e2e

## Acceptance Criteria

- [ ] 后端 2 panic 路径消除（proc nil 守卫、queueCancel double-close）
- [ ] 同会话串行不变式无绕过窗口
- [ ] 消息/索引/模板/配置持久化原子写（temp+rename）
- [ ] 前端 IME Enter、markdown 链接拦截、乐观态时序、目录切换竞态、拖拽回滚、删除在途会话拦截全部修复
- [ ] 🟡 项逐项修复或明确记录不改理由（原则：全修）
- [ ] 全量测试绿（已知环境性失败 TestApp_StartWebServe_StartupMatrix 除外）

## Out of Scope

- 功能新增 / 重构（仅缺陷修复）
- containsPath 大小写：修后端去重键 ToLower 对齐前端（选审核建议第 1 项，非仅改注释）

## Technical Notes

- 原任务 PRD：.trellis/tasks/archive/2026-09/09-22-ai-aichatbox/prd.md
- 契约约束：docs/spec/ai-chat-service.md（事件流/存储/串行保护语义不变）
