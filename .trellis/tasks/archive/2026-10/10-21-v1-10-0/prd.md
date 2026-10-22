# v1.10.0 发版收口

## Goal

完成 v1.10.0 正式发版：真机冒烟 → 问题分级修复 → CHANGELOG/里程碑表补全 → release.sh 发版。

## What I already know

* **事实修正**：v1.9.0 已于 2026-09-28 发版（git tag v1.9.0 存在 + CHANGELOG [1.9.0] - 2026-09-28），未发版窗口实为 v1.9.0 → HEAD（180 commits）
* wails.json `productVersion` 已升至 1.10.0，未收口
* CHANGELOG `[Unreleased]` 仅 2 项（hosts 查看器/环境变量查看器），v1.9.0 后大量功能未登记，需补全
* docs/路线图.md 里程碑表停在 v1.8.0，缺 v1.9.0 行（补登）+ v1.10.0 行（新增）
* 一键发版脚本 `scripts/release.sh` 存在（tag 推送后 release.yml 自动构建发布）
* journal 悬置真机待办：
  * 素材面板：文件/目录拖拽移动、目录管理（创建/删除/重命名）、文件夹标注、中文目录名（Session: 10-20-assets-folder-tree-drag）
  * docx 真机目检（多页/表格/页眉页脚，四 major 出清遗留）
  * EP 2.14.5 确认型弹框回归已排查（10-20 完成）

## Assumptions (temporary)

* 冒烟执行方式沿用先例（Session 127/128 混合模式）：可脚本化面（RPC 驱动）AI 跑，UI 呈现面用户手点
* 发版走 scripts/release.sh 既有流程，不新造

## Decision (ADR-lite)

**Context**: 范围口径 / 冒烟执行方式 / P1 处置三决策点
**Decision**（用户 2026-10-22 全按推荐确认）:
1. 范围 = v1.9.0 → v1.10.0（v1.9.0 已发 tag；里程碑表补登 v1.9.0 行）
2. 冒烟混合模式：RPC 可驱动面 AI 脚本跑（先例 rpc_smoke.py），UI 手点面列清单用户执行
3. P1 当版修完再发，P2 记 backlog
**Consequences**: 冒烟留痕分两份（RPC 驱动面 + UI 手点清单）；P1 修复走完整门禁后再发版

## Open Questions

（已全部收敛，见 Decision）

## Requirements

* 真机冒烟清单（journal 悬置优先）：
  * 素材面板：文件/目录拖拽移动、目录管理（创建/删除/重命名）、文件夹标注、中文目录名
  * PPT 拆分/合并导出端到端
  * pptx 预览（COM 转 PDF）
  * 浏览器访问模式回归
  * go-git 出清后 git 全功能回归（提交/推送/历史/分支/合并/冲突）
  * docx 真机目检（多页/表格/页眉页脚）
* 冒烟问题分级修复（P0 阻塞发版，P1 当版修，P2 记 backlog）
* CHANGELOG Unreleased 补全 v1.9.0 后全部功能（按域分组）
* 路线图.md 里程碑表补 v1.9.0 行（补登）+ v1.10.0 行（含 CHANGELOG 摘要）
* 发版走 scripts/release.sh 既有流程

## Acceptance Criteria

* [ ] 冒烟清单逐项留痕（通过/问题/处理），归档任务目录
* [ ] go test ./... + integration + 前端 vitest + E2E 全绿
* [ ] govulncheck + npm audit 清零
* [ ] CHANGELOG Unreleased → [1.10.0] 完整条目
* [ ] 路线图.md 里程碑表含 v1.9.0 + v1.10.0 行
* [ ] v1.10.0 产物构建成功（release.yml 发布）

## Definition of Done

* 冒烟留痕文档入任务目录
* 门禁全绿
* tag v1.10.0 推送，release.yml 构建成功

## Out of Scope

* 单页 OCR（SRS §2.2/§2.4，下一批功能开发）
* P2 问题修复（记 backlog）

## Technical Notes

* 发版链：release.sh → tag 推 Gitee → 镜像 GitHub → release.yml 自动构建
* 冒烟先例：archive/2026-10/10-18-srs-2-7-3/rpc_smoke.py（RPC 驱动面无头 serve 冒烟）
