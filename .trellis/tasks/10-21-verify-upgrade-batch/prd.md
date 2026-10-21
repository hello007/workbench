# 验证闭环与升级专项批（CI push 闭环 + mermaid 12 + E2E 第二批）

> 父任务（epic 编排视图）：按序推进三个子任务，本任务自身无代码改动。

## Goal

收口 hardening-batch 遗留验证面（5 个工作 commit 零 CI 验证）+ 修复线升级（audit high 清零）+ 矩阵留痕收尾。

## 子任务与执行顺序

| 顺序 | 任务 | 目录 | 一句话 |
|---|---|---|---|
| 1 | CI push 验证闭环 + 镜像仓同步规程 | 10-21-ci-push-loop | 5 个工作 commit 推送 + CI 实跑验证 + 同步规程入文档（小任务快收口） |
| 2 | mermaid 12 major 升级专项 | 10-21-mermaid-12 | audit 剩余 high 修复线（dompurify/katex 传递链），渲染面专项回归 |
| 3 | E2E 覆盖第二批 + wails-init 硬化 | 10-21-e2e-batch2 | 矩阵候选 #8-#20 复核后落地 ≤12 用例 + 基建硬化 |

## 执行约定

- 子任务各自走完整 trellis 流程，独立提交与归档；前一归档再 start 下一
- 顺序依据：任务 1 是刚需（未验证面最多且体量最小）；任务 2 价值最实（安全清零）；任务 3 体量最大可后置
- 任务 1 的 push 动作涉及外发，brainstorm 时先与用户确认 push 授权范围
- 父任务在三子任务全归档后收口

## Out of Scope

- vue-router 5 / js-yaml 5 / markdown-it 15 / docx-preview 0.4 其余 major 升级（各自专项按需）
- E2E 矩阵之外的测试面扩展
