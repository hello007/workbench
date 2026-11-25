# T11 验证 DSH 旁路集成只读 PoC

- 父任务：`11-24-personal-dashboard`，执行顺序第 12 项（序号 11）
- 阶段 / 优先级：阶段 4 P2 可选验证 / P2
- 前置：T10
- 状态：planning（仅登记，未 start）

## 结论前提：不迁移

DeepSeek Harness 是 Agent 运行时底座（模型/工具/会话/沙箱/循环/UI 皆插件），与本项目的「数据看板」Job 范畴错配。**明确不迁移、不重写主链路**，只验证旁路共存（Integrate, not Migrate）。

## 已核实事实（2026-11-24，官方仓库 README）

`dsh` 为 MIT 开源、developer preview，原文标注 "THERE WILL BE COMPATIBILITY-BREAKING CHANGES"，并要求先阅读安全须知；版本 `0.2.3-alpha.2`；Node.js 门槛 24.22.3。另有二手媒体报道（**待核实**）：社区第三方插件近 1.2 万、已出现恶意插件远程操控事件。

## 要做的事

沿用现有外部 Agent CLI 外挂模式，以 `dsh headless --json` 或 `--profile sdk` 挂成又一个外部 Agent 提供者，Wails 工作台继续担任 GUI 与事实源（SoR）。PoC 场景：用 `dsh headless` 生成周报，结果回写 `ai_task_history`，对比现有链路的轨迹质量与成本。零重写、可随时摘除。

## 验收标准（可测试）

产出「继续扩大 / 停止」的对比结论及依据；现有主链路未改动；未引入全量 TypeScript/Electron 重写。

## 不做的事

不做迁移、不做多模型平台化、不安装来源不明的第三方插件。
