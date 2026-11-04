# 岛仔 Dz 项目调研与借鉴清单

## Goal

调研 GitHub 项目 `smallmb/Dz`（岛仔，Rust + egui 的 Windows 桌面信息岛），提炼对 WorkBench 可落地、有价值的借鉴项，形成文档沉淀；本次不涉及代码改动。

## What I already know

- Dz 为 `Renly1994/xiaotang-daozai` 的 fork，版本 0.6.39，主程序为无边框透明置顶窗口。
- WorkBench 当前技术栈：Go 1.26 + Wails v2.16.0 + Vue 3；已有 AI 功能（`claude -p` 子进程）、Git 操作、素材管理、浏览器通道。
- WorkBench 现状核实：无托盘、无系统通知、无 AI 额度展示；`data/` 为相对 exe 目录。
- Wails v2.16.0 源码核实：无 `TrayIcon` 与多窗口 API；内置 `SingleInstanceLock`、`HideWindowOnClose`、原生通知（`runtime.SendNotification`）、`ScreenGetAll`。

## Scope

- 产出：`docs/research/2026-11-04-岛仔Dz项目调研与借鉴清单.md`。
- 覆盖：功能矩阵、架构特征、10 项借鉴清单（P0/P1/P2）、灵动岛可行性专题（三条技术路线与还原度边界）、实施排期。
- 不覆盖：任何代码实现、依赖引入、配置变更。

## Acceptance Criteria

- [x] 文档落盘 `docs/research/`，文件名带日期前缀。
- [x] 借鉴项标注 WorkBench 落点、粗略成本、风险。
- [x] Wails 能力结论均经本地源码核实，不引二手文档。
- [x] 明确列出不建议借鉴的项及原因。
- [x] 文档已提交（`376c912`）。

## Definition of Done

- 调研文档入库，任务归档。
