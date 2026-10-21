# CI push 验证闭环 + 镜像仓同步规程

> 父任务：10-21-verify-upgrade-batch（顺序 1，最先执行）

## Goal

本地 master 领先远端多个工作提交未推送（gitee origin），GitHub 镜像仓（hello007/workbench）手动同步且滞后。近两日 5 个工作 commit（依赖分档 + CI 一致性检查 step + goleak 硬门禁 + tilde 升级 + E2E 补齐）CI 从未实跑——包括 ci.yml 步骤 2.5 线上首跑与 goleak 硬门禁 Linux 首跑。本任务推送验证 + 同步规程沉淀。

## Requirements

1. **push 前全量自检**：HEAD 组合态双侧复跑——本机 Windows + WSL Linux `go test ./...` 全量、前端 vitest 全量 + E2E web/desktop 全量（各任务已分别跑过，合并态复跑一次）
2. **push 授权确认**（brainstorm 与用户确认）：推送范围（仅 master？含 tag？）、是否同步镜像仓
3. **push gitee origin master** + 观察远端 CI（gitee 侧无 Actions，实际 CI 在 GitHub 镜像仓）
4. **镜像仓同步**：盘点现同步机制（手动 push？脚本？谁负责）→ 同步至 HEAD → 观察 CI run：
   - 步骤 2.5（依赖分档一致性检查）线上首跑绿
   - goleak 硬门禁 Linux 首跑绿
   - 旧红点（run 11/12「后端单元测试」FAIL）应转绿
5. **CI 红处置**：如有红点按 step 定位修复（WSL 是等价复现通道）
6. **同步规程沉淀**：镜像仓同步操作规程入 docs（开发工作流.md 或贡献指南.md，含触发时机与责任口）

## Acceptance Criteria

- [ ] HEAD 组合态双侧全量绿（push 前自检）
- [ ] 远端 master 含全部工作提交
- [ ] 镜像仓同步至 HEAD，CI run 全 job 绿（含 2.5 首跑 + goleak 首跑）
- [ ] 同步规程入文档
- [ ] 如有 CI 红点已修复并复验绿

## Definition of Done

- AC 全勾；CI 实跑绿截图/链接留 research

## Open Questions（brainstorm 待定）

- push 授权范围（master only / 含 tag；用户确认）
- 镜像仓同步机制现状（手动/脚本/负责人）

## Out of Scope

- gitee 侧 CI 接入（仓库无 Gitee Go 配置）
- 镜像仓自动化同步搭建（本任务只文档化现有机制，自动化另行评估）

## Technical Notes

- 镜像仓 run 11/12 红点分析见 .trellis/tasks/archive/2026-10/10-21-goleak-gate/research/ci-linux-repro.md（旧 commit Linux 失败，goleak-gate 已修）
- WSL 复现命令形态：PATH=~/toolchain/go/bin、GOPROXY=goproxy.cn、GOFLAGS=-buildvcs=false
- CI 红点核对：actions runs API jobs/steps 结论公开可见（日志正文需认证）
