# AI 对话工作台页面（AIChatBox）

## Goal

在 ActivityBar 新增「AI 对话」面板：以常用目录为入口的多会话持续式 AI 对话界面。区别于现有 AI 功能（一次性技能触发），提供选目录、发需求、看流式回复、续会话（类 claude /resume）的多轮对话体验。默认走 claude code CLI，架构预留多 CLI 接入。

## Requirements

- 命名：面板中文名「AI 对话」，面板 id `ai-chat`，前端组件 `AiChatPanel.vue`，与「AI 功能」命名对称
- ActivityBar panels 数组新增入口；激活时面板占满 ActivityBar 右侧全部区域（与 ai/toolbox 等面板同机制，非全屏）
- 左侧常用目录栏：
  - DirectoryTree（工作目录）右键「添加到 AI 对话」加整个工作目录
  - FileTreePanel（文件树）右键目录节点「添加到 AI 对话」加该子目录
  - 支持自定义显示名（如「项目管理」）、可移除、可排序
- 右侧对话区上下分栏：
  - 上：对话历史消息区，markdown 渲染，流式输出增量展示
  - 下：输入框 + 模板下拉；模板为纯文本片段，点击整段填入输入框可再修改；下拉分「本目录模板 / 全局模板」两组，尾带「管理模板」打开弹窗增删改
- 会话模型：每目录多个会话；新建/切换/恢复；对话历史上方下拉按钮查看会话列表并选择进入（类 /resume）；claude 侧 `--resume <sid>` 续上下文；WorkBench 持久化会话元数据与每轮消息
- 执行配置：对话页顶部下拉切换权限模式（default / acceptEdits / plan / bypassPermissions，默认 default）与模型；选择持久化记忆
- 后端：ChatSession model/service（会话 CRUD、消息持久化、RunChat 自由 prompt 入口）；provider 接口预留多 CLI，MVP 仅实现 claude

## Acceptance Criteria

- [ ] ActivityBar 出现「AI 对话」入口，点击后面板占满 ActivityBar 右侧区域
- [ ] DirectoryTree 右键工作目录可添加到 AI 对话侧栏；FileTreePanel 右键目录节点可添加该目录
- [ ] 侧栏目录项支持自定义显示名、移除、排序
- [ ] 输入需求后 claude 启动执行，回复流式增量展示且 markdown 渲染
- [ ] 同会话二次提问经 `--resume` 续同一 claude 会话，上下文连贯
- [ ] 每目录可新建多个会话；历史会话下拉可查看、切换、恢复
- [ ] 模板点击后内容填入输入框可修改后发送；模板弹窗支持本目录/全局两组增删改
- [ ] 权限模式与模型下拉切换后按所选值执行，重启后记忆
- [ ] 在途任务时切换目录/会话有提示且可取消（CancelAiTask + 监听器精准注销）

## Definition of Done

- 前端 vitest 单测覆盖核心组件与 store 逻辑；后端 go test 覆盖新增 service 方法
- 覆盖率门禁达标（前端 ≥70%）
- 新增 App 方法同步 frontend/wailsjs 三处（App.js / App.d.ts / models.ts）
- README.md 与 docs（功能说明/API参考）更新
- lint / build 绿

## Technical Approach

- **复用链路**：service/ai_function.go 的任务队列（RunStage 模式：queued/started/done 事件、并发信号量、超时、流式输出文件、session_id 提取）。新增 ChatService 参照该模式实现 RunChat（自由 prompt，不绑定 AiFunction 配置），buildClaudeArgs 扩展 --permission-mode 与 --model。
- **provider 抽象**：接口定义启动参数组装与输出解析分派点；MVP 单一 claude 实现，不过度设计。
- **持久化**：`data/ai_chat/` 独立目录（会话索引 + 每会话消息 JSON），不与 RepoMetaService 文件交叉；写前关 workbench.exe（开发期约束）。
- **前端结构**：AiChatPanel（activePanel='ai-chat' 渲染分支）内部分左目录栏右对话区；目录收藏数据持久化（新 App 方法或复用现有收藏机制扩展）；ai-task:* 事件监听用 EventsOn 返回闭包精准注销。
- **右键入口**：DirectoryTree / FileTreePanel 右键菜单各加一项，emit 事件驱动 uiStore 打开面板并写入目录项。

## Decision (ADR-lite)

### D1 会话模型：多会话
**Context**：每目录会话组织方式决定数据模型与后端改造量。
**Decision**：每目录可新建多个会话，下拉列表切换/恢复（完整 /resume 体验）；WorkBench 持久化会话元数据+每轮消息，claude 侧 `--resume` 续上下文。
**Consequences**：需新增会话存储 model/service；上下文互不污染；数据量随会话数增长（可后续加清理策略）。

### D2 模型接入：多 CLI 架构，MVP 仅 claude code
**Context**：「配置页选其他模型」的语义边界。
**Decision**：架构按多 CLI 适配设计（provider 抽象：启动参数、输出解析、会话恢复按 CLI 类型分派），MVP 仅实现 claude code provider；gemini/codex 等后续扩展。
**Consequences**：service 层 provider 接口 MVP 保持薄（单一实现）；配置页模型下拉 MVP 仅列 claude 系模型。

### D3 展示形态：标准面板占满
**Context**：对话页的空间形态。
**Decision**：非全屏。activePanel='ai-chat' 时面板占满 ActivityBar 右侧全部区域（与 ai/toolbox/dashboard/stats 同机制），面板内部分左右两列。
**Consequences**：零新增布局机制，ActivityBar panels 加一项 + Home.vue 加渲染分支。

### D4 模板：纯文本片段 + 就近管理
**Context**：模板内容形态与管理入口位置。
**Decision**：模板为纯文本片段，点击整段填入输入框可再修改（无占位符注入）。工作空间=AI 对话侧栏目录项。管理入口在输入框旁模板下拉：「本目录模板 / 全局模板」两组+「管理模板」弹窗。
**Consequences**：前端填充零后端参与；模板数据归属目录 ID 或全局，随配置持久化。

### D5 执行权限：可配置多模式
**Context**：claude headless 模式下权限决定 AI 能否修改代码。
**Decision**：权限模式可配置（default / acceptEdits / plan / bypassPermissions），默认 default。切换入口在对话页顶部下拉，选择持久化记忆。
**Consequences**：buildClaudeArgs 支持追加 --permission-mode 与 --model；bypassPermissions 加提示。

## Out of Scope

- 其他 CLI（gemini/codex 等）的实际实现（仅接口预留）
- 会话导出、token 用量统计展示
- 工具调用过程可视化（只展示最终回复文本）
- 对话内图片/文件附件上传

## Implementation Plan (small PRs)

- PR1：后端——ChatSession model + ChatService（会话 CRUD、消息持久化、RunChat）+ App 绑定 + wailsjs 三处同步
- PR2：前端——AiChatPanel 骨架（左右分栏）+ 目录收藏（右键入口+显示名+排序）
- PR3：对话闭环（流式渲染+多会话切换/resume）+ 模板系统 + 权限/模型配置
- PR4：测试补齐 + 文档 + README

## Technical Notes

- service/ai_function.go:395 RunStage(functionID, prompt, resumeSessionID) —— resume 能力参考
- service/ai_function.go:850 buildClaudeArgs：-p <prompt> [--resume <sid>] --output-format stream-json --verbose
- 关键规则：EventsOn 闭包注销、service 层 EventSink 发事件、AppServices 两处注册（app_services.go）、slog 日志 + AppError 错误码分流、wailsjs 绑定三处同步、具名 string 类型 models.ts 手动补别名
- 测试：后端 util/testutil 辅助、前端 wails-mock-defaults 单一数据源、E2E fixtures.js 的 {test,expect}
