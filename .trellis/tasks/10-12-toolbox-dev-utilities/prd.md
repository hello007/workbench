# 工具箱续作：端口占用一览 + JSON/YAML 格式化校验

## Goal

工具箱（ToolboxPanel）已有「拷贝到」「进程管理」两项。本任务新增两个开发者高频小工具：「端口占用一览」（看一眼定位端口冲突并可直接终止）与「JSON/YAML 格式化校验」（校验/格式化/压缩/互转），提升 WorkBench 日常开发粘性。

## Requirements

- 工具箱 `tools` 数组新增两项，各配独立 Dialog 组件（沿用进程管理模式）
- **端口占用一览**：
  - 只读表格：端口/协议/本地地址/PID/进程名/状态
  - 打开时自动枚举 + 手动刷新
  - 行内「终止」：复用进程管理停止链路（优雅请求→5s 轮询实测存活→未退强杀）+ 确认交互
  - 后端复用/扩展现有进程枚举底座，不重复实现
- **JSON/YAML 格式化校验**：
  - 粘贴文本 → 校验（实时或按钮）→ 合法显示成功，非法显示错误消息+行号
  - 格式化：JSON 缩进 2/4 可调；YAML 输出标准格式
  - 压缩（minify）：去空白输出
  - JSON↔YAML 互转
  - 纯前端实现，引入 `js-yaml`
- 决策记录：
  - 决策 1（2026-10-12）：MVP 只收这两项；环境变量查看、hosts 查看/编辑留后续任务
  - 决策 2（2026-10-12）：端口一览只读表格 + 行内终止（方案 2）
  - 决策 3（2026-10-12）：JSON/YAML 含压缩与互转（方案 3），编辑器形态 EP textarea，不引 CodeMirror
  - 决策 4（2026-10-12）：两 PR 拆分（方案 1）

## Acceptance Criteria

- [ ] 端口一览：Windows/Linux 双平台枚举正确，行内终止走通停止链路
- [ ] 端口一览：自我保护判据与进程管理一致（PID + 父进程链，`IsSelf`/`IsSystem` 后端填充）
- [ ] JSON/YAML：校验错误带行号；格式化/minify/互转各有用例
- [ ] JSON↔YAML 互转对有损场景（YAML 锚点/多文档/注释）给出明确提示或拒绝转换，不静默丢数据
- [ ] 后端 service/util 单元测试 + 前端组件测试覆盖
- [ ] Windows/Linux 交叉编译通过
- [ ] 覆盖率门禁不回退（service ≥76%，前端 ≥70%）

## Definition of Done

- 测试全绿 + 覆盖率门禁达标
- docs/功能说明.md、README.md 更新
- CLAUDE.md 关键规则表按需追加（端口枚举复用规则等）
- 新 spec 沉淀到 docs/spec/（如适用）

## Technical Approach

- **PR1 端口占用一览**：后端在进程管理 service 底座上加「监听端口视图」方法（netstat/tasklist、/proc 复用，平台分叉模式 `_windows/_other`）+ App 绑定 + `frontend/wailsjs/` 三处同步 + 前端 `PortListDialog.vue`（表格 + 行内终止，复用 ProcessManagerDialog 的终止编排与三级确认范式）
- **PR2 JSON/YAML 工具**：`npm install js-yaml`；`JsonYamlToolDialog.vue` 纯前端组件（校验/格式化/minify/互转，textarea + EP 表单），零后端改动；ToolboxPanel `tools` 数组接入
- 工具图标：内联 SVG + `fill="currentColor"`（深色容器规则）
- 新 service 方法如有事件不涉及；App 方法签名变更须同步 `frontend/wailsjs/` 绑定三处（App.js/App.d.ts/models.ts）

## Decision (ADR-lite)

**Context**：工具箱续作方向，候选四类工具（端口/环境变量/hosts/JSON-YAML）。
**Decision**：MVP 收端口一览 + JSON/YAML 两项；端口一览带行内终止；JSON/YAML 含压缩与互转；两 PR 交付。
**Consequences**：hosts/环境变量推迟（后续任务可复用本次骨架）；互转有损场景须显式处理（验收项）；`js-yaml` 为新增前端依赖。

## Out of Scope

- 环境变量查看（含脱敏策略）
- hosts 查看/编辑（提权链路复杂）
- JSON↔YAML 编辑器升级（CodeMirror/monaco）
- 端口一览跳转进程管理弹窗联动
- 端口一览自动轮询刷新

## Technical Notes

- 先例任务：`.trellis/tasks/archive/`（10-09-toolbox-process-manager）——停止链路、平台分叉、错误形态识别均已验证
- spec：docs/spec/process-management.md、frontend-visual-conventions.md、linux-platform.md、cross-layer-contracts.md（wailsjs 同步）
- 进程管理关键约束（CLAUDE.md 关键规则）：禁用 taskkill SUCCESS 当已退出；`cmd.Output()` 丢 stderr 须拼 `ExitError.Stderr`；终止命令必带超时
