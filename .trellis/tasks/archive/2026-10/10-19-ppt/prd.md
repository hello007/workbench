# 工具箱全窗口面板化 + PPT 分割文件夹批量扫描

## Goal

1. 工具箱从「卡片列表 + 弹窗」改造为与 AI 功能面板一致的全窗口布局：左侧功能清单、右侧选中功能内容面板，点击功能不再弹窗。
2. PPT 拆分支持扫描文件夹（递归子目录），对文件夹下所有 .pptx 逐个串行拆分，并展示最终分割结果汇总。

## Requirements

### R1 工具箱全窗口面板化

- ToolboxPanel 移出 `.main-panes`，独占整个窗口区域（与 AiFunctionPanel 同级同互斥语义，激活时文件树三栏隐藏）。
- 布局对齐 AiFunctionPanel：顶部 panel-header（渐变标题栏）+ 左侧功能清单 + 右侧选中功能内容面板。
- 7 个工具全部去弹窗化：各 Dialog 组件内容迁为内嵌面板组件，点击左侧清单项右侧切换展示；默认选中首个工具。
- 关闭按钮语义对齐其他独占面板（切回 directory）。
- 根元素带完整 flex 约束链（`flex:1`+`min-height:0`+`min-width:0`+`display:flex`+`flex-direction:column`+`overflow:hidden`，Home 一级面板规范）。

### R2 PPT 拆分文件夹批量扫描

- PPT 拆分面板支持文件夹模式：选择/输入文件夹路径，**递归扫描**其下所有 .pptx 文件。
- **排除拆分产物**：递归扫描须排除历史拆分产物（`<源文件名>/<源文件名>-NN.pptx` 同名文件夹形态，参考 assets 域 `isPptxSplitDerivative` 判定），防止对同一文件夹重复批量拆分时产物被再拆。
- 逐个**串行**拆分（单文件全量解压进内存，并发内存翻倍；顺序可预期）。
- 进度展示两层：文件级（第 N/M 个文件 + 当前文件名）+ 页级（当前文件 done/total 页，复用现有 pptx-split:progress）。
- **单文件失败不中断**：记录失败原因后继续拆下一个，最终汇总成功/失败清单（每项：文件路径、状态、页数、输出目录、失败原因）。
- 批量取消：复用 CancelPptxSplit 全量取消语义，取消后当前文件中止、剩余文件不再开始，已拆完的保留。
- 已存在输出按现有单文件语义覆盖。
- 空扫描结果（递归后无 .pptx）给出引导提示。

## Acceptance Criteria

- [ ] 点击侧边栏「工具箱」后独占整个窗口区域，文件树三栏隐藏；再次点击或关闭按钮切回 directory
- [ ] 左侧功能清单展示 7 个工具，点击某项右侧内嵌展示该工具完整功能，全程无弹窗
- [ ] 7 个工具功能行为与改造前等价（拷贝/终止进程/端口终止/格式化互转/环境变量/hosts/单文件拆分）
- [ ] PPT 拆分文件夹模式：递归扫描 → 串行拆分 → 双层进度 → 结果汇总展示
- [ ] 递归扫描排除历史拆分产物（同名文件夹内 NN 序号文件不再拆）
- [ ] 批量拆分可取消：当前文件中止、剩余不开始、已完成保留
- [ ] 单文件失败不阻塞批量，汇总含失败原因
- [ ] 文件树右键单文件拆分入口行为不变
- [ ] wailsjs 三处同步（新方法 + 新 model 类型）
- [ ] 后端 go test：批量扫描（含产物排除）、串行编排、取消、失败续跑
- [ ] 前端 vitest：ToolboxPanel 布局切换、批量拆分编排、事件过滤

## Definition of Done

- `go test ./...` 与 `cd frontend && npm test` 全绿
- 覆盖率门禁满足（service ≥76% 基线、前端 ≥70%）
- wailsjs 绑定三处手动同步（App.js / App.d.ts / models.ts）
- README.md / docs/功能说明.md 行为变化同步

## Decision (ADR-lite)

**Context**: 三个设计分叉需用户决策：扫描深度、失败策略、布局互斥。
**Decision**: 递归子目录（覆盖全）；失败继续+汇总（坏文件不阻塞整批）；独占窗口（对齐 AI 功能面板互斥语义）。
**Consequences**: 递归引入「产物被再拆」风险，须复用拆分产物判定排除；独占布局改变现有「工具箱+文件树共存」行为，Home.vue 面板互斥逻辑统一。

## Out of Scope

- 工具清单新增其他工具
- 批量拆分并发执行
- 文件树右键单文件拆分入口的改动
- 已拆分输出的增量/跳过逻辑（恒覆盖，与单文件语义一致）
- 批量拆分历史记录持久化（结果仅当前会话展示）

## Technical Notes

### 已核查文件

- frontend/src/components/ToolboxPanel.vue（现状弹窗编排，7 工具清单）
- frontend/src/components/PptSplitDialog.vue（事件监听模型：watch(modelValue) 注册/注销，内嵌化后改为随工具选中态注册）
- frontend/src/components/AiFunctionPanel.vue（布局参照：panel-header + ai-layout）
- frontend/src/components/ActivityBar.vue（panels 清单）
- frontend/src/views/Home.vue:10/23-25（main-panes v-show 条件 + ToolboxPanel 挂载点）
- service/pptx_split_service.go（取消注册表 + 事件契约 + provider 抽象）
- service/pptx_split.go（拆分核心：全量解压进内存，`SplitPptxPerPageWithProgress`）
- app_pptx.go（Wails 绑定 2 方法）
- FileTreePanel.vue:1581（文件树右键独立入口，不走弹窗，不受影响）

### 设计要点

- 批量编排加在 PptxSplitService（复用现有取消注册表/事件出口/provider 抽象），新 App 方法 `SplitPptxFolder`；批量任务整体持一个可取消 ctx（键=normalizeRepoKey(目录路径)），循环内逐文件调 provider.Split 继承该 ctx——CancelPptxSplit 命中即整批中止。
- 新事件 `pptx-split:batch:progress`（文件级）独立于现有 `pptx-split:progress`（页级），单文件消费方不受影响；终态经 Promise 返回汇总（批量单次调用天然有返回值，不强依赖 done 事件）。
- model 新增：`PptxBatchResult{ TotalSucceeded, TotalFailed, Items []PptxBatchItem }`，Item 含 SrcPath/OutputDir/PageCount/Error。
- 前端 PptSplitDialog → PptSplitPanel 内嵌化：单文件/文件夹两模式并存（tab 或分区），事件监听注册改挂工具选中态（activeTool watch）。
- 各 Dialog 组件去壳策略：el-dialog 壳换成面板容器 div，内部逻辑（loading/事件监听/错误分流）不动；事件注册时机从「弹窗打开」改「工具选中」（或组件常驻挂载 + 工具选中时刷新）。

### 约束

- service 层发事件走 EventSink（emitEvent(s.eventSink(), ...)），禁直调 runtime.EventsEmit
- Wails 事件多组件共监听须 EventsOn 返回闭包精准注销，禁 EventsOff
- 复用 PptxSplitService 加方法，app_services.go 零改动
- 新增 Wails 方法/类型同步 frontend/wailsjs/ 三处
- 递归扫描目录遍历注意 Windows 权限错误降级（跳过不可读子目录记告警，不中断扫描）
