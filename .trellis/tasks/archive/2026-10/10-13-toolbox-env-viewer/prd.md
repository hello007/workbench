# 工具箱续作第二期：环境变量查看器

## Goal

工具箱新增「环境变量」只读查看器。Windows 展示系统/用户/进程三级（注册表区分来源），Linux 展示进程级（`/proc/self/environ`）。支持关键字过滤、值脱敏展示（敏感命名打码 + 点击临时显值）、复制单值、按名称排序。提升 WorkBench 开发排障能力（排查 PATH 覆盖、代理变量、构建环境差异等高频场景）。

## What I already know

- 先例任务：`10-09-toolbox-process-manager`（停止链路/平台分叉）、`10-12-toolbox-dev-utilities`（tools 数组 + 独立 Dialog 接入模式，PRD 决策明确「环境变量查看留后续任务」）
- `golang.org/x/sys v0.48.0` 已在 go.mod，`x/sys/windows/registry` 可直接读注册表，零新增依赖
- util 平台分叉先例成熟：`util/process_windows.go` / `util/process_other.go` + `_windows_test`/`_other_test` 拆分
- 工具箱接入模式：`frontend/src/components/ToolboxPanel.vue` `tools` 数组 + `openXxx` handler + 独立 Dialog 组件（append-to-body）
- 深色容器图标规则：内联 SVG + `fill="currentColor"`

## Requirements

- 工具箱 `tools` 数组新增「环境变量」项，独立 Dialog 组件（沿用进程管理模式）
- **三级来源**（Windows）：系统（注册表 `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`）/ 用户（`HKCU\Environment`）/ 进程（`os.Environ()`，含父进程合并后的实际生效值）
- **Linux**：进程级 `/proc/self/environ`（NUL 分隔解析），UI 仅显示「进程」一级
- 关键字过滤（名称/值匹配）
- 值默认脱敏：名称命中敏感命名模式（TOKEN/KEY/PASSWORD/SECRET 等，大小写不敏感）时打码，可点击临时显值
- 复制单值（含脱敏项，复制的是真实值）
- 按名称排序展示
- 纯只读：不做编辑/新增/删除
- 零依赖（不引新 Go/前端包）

## Assumptions

- 脱敏模式匹配 = 大小写不敏感子串包含

## Decisions（2026-10-13 用户确认）

- 决策 1：Linux 进程级仅 workbench 自身进程（`/proc/self/environ`），实现最简；后续要任意进程再加 pid 参数
- 决策 2：脱敏命名模式 = `TOKEN`/`SECRET`/`PASSWORD`/`PASSWD`/`KEY`/`CREDENTIAL`，大小写不敏感子串包含
- 决策 3：Windows `REG_EXPAND_SZ` 读取时展开（`%VAR%` 还原为实际值），与进程级展示口径一致

## Open Questions

（无——三项已确认）

## Acceptance Criteria (evolving)

- [ ] Windows 三级枚举正确（注入注册表 fixture，不依赖本机真实环境变量内容）
- [ ] Linux `/proc/self/environ` 解析正确（fixture 注入，NUL 分隔）
- [ ] 平台分叉按 linux-platform spec：编译期字段独有走 `_windows`/`_other` 双文件
- [ ] 脱敏：敏感名称打码 + 点击临时显值；非敏感直接展示
- [ ] 过滤、排序、复制单值可用
- [ ] wailsjs 三处同步（App.js/App.d.ts/models.ts）；具名 string 类型手动补 models.ts 别名（MISSING_EXPORT 坑）
- [ ] 测试按 `_windows_test`/`_linux_test` 拆分，fixture 注入禁依赖本机真实环境变量
- [ ] 覆盖率门禁不回退（util ≥40% + service ≥76% + 前端 ≥70%）

## Definition of Done

- 测试全绿 + 覆盖率门禁达标
- docs/功能说明.md、README.md 更新
- CLAUDE.md 关键规则表按需追加
- 新 spec 沉淀到 docs/spec/（如适用）

## Out of Scope

- 环境变量编辑/新增/删除
- 按值搜索
- 导出全部变量
- 自动轮询刷新（打开时枚举 + 手动刷新即可）
- 跳转进程管理联动

## Technical Notes

- 先例：`.trellis/tasks/archive/2026-10/10-09-toolbox-process-manager`、`10-12-toolbox-dev-utilities`
- spec：docs/spec/linux-platform.md（平台分叉选型）、cross-layer-contracts.md（wailsjs 同步 + 具名 string 坑）、process-management.md §9.1（复用模式）、frontend-visual-conventions.md
- 候选结构：util 层平台分叉枚举（`env_windows.go` 读注册表 / `env_other.go` /proc）→ service 层编排（脱敏/过滤/排序在 service 还是前端待定）→ App 绑定
