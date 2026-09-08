# AI 功能菜单（Skill 聚合触发器）

## 目标

将常用 Claude Code skills 聚合为 WorkBench（本项目 git-manager）工具箱内的可视化「AI 功能」页：一键触发，Go 侧子进程执行 `claude -p` headless 调用，流式回显输出，含交互确认环节的功能以多段编排完成，产物可预览或打开目录。

## 需求

### 功能菜单

* 工具箱新增「AI 功能」页：功能卡片列表（名称、描述、图标），点击运行
* 首批四个功能：

| 功能 | skill/插件 | cwd | 参数 | 交互模式 |
| ---- | ---- | ---- | ---- | ---- |
| 文档转 HTML 发言稿 | agree-slides（ab-office@ab-internal-plugins 插件） | `D:/workspace/workspace_ai/all_in_ai/workspace_claudcode/u51_ppt生成` | file：选源文件 | 单发 |
| 生成周报 | ab-weekly-report（依赖 pmo-weekly-report 链） | `D:/workspace/workspace_ai/all_in_ai/workspace_claudcode/u63_项目管理` | none | 多段：草稿+亮点候选 → 面板确认 → 落盘终稿 |
| 会议预约 | tencent-meeting-mcp（MCP） | `D:/workspace/workspace_ai/all_in_ai/workspace_claudcode/u54_tencent` | 表单：开始时间/时长/主题等 | 单发 → 会议号复制剪贴板 |
| 会议查看/取消 | tencent-meeting-mcp（MCP） | 同上 | none | 多段：查列表 → 选中 → 确认取消 |

### 执行器（Go 后端）

* 构建 `claude -p "/skill名"` 子进程：cwd 指功能项配置的项目根；`--add-dir` 授权额外目录；`--permission-mode` 按功能项配置；`--settings` 注入 env（如 TENCENT_MEETING_TOKEN，值存本地配置不进代码库）
* 流式读取 stdout 回显前端（长任务实时滚动）
* 多段编排：第一段结束后取 session id，第二段 `--resume <session-id>` 续会话保留上下文（已实测 CLI 2.1.158 支持）
* 取消：杀子进程；超时：默认上限可按功能项配置

### 输出面板

* 多任务并行，每任务一个 Tab；流式滚动；取消按钮
* 周报/会议取消的「待确认」状态：面板展示候选内容 + 确认按钮，确认后触发第二段

### 参数输入

* `file` 类型：复用 WorkBench 文件树选择源文件
* `form` 类型：字段化表单（会议预约：开始时间、时长、主题等）
* `none`：直跑

### 配置管理

* 功能项 JSON 注册表，存 WorkBench 现有 `data/` 配置体系（字段：id/名称/描述/skill 或插件/cwd/addDirs/参数定义/permission-mode/超时/完成动作/图标）
* **完整增删改表单**（列表 + 表单页），读写配置 JSON；新增 skill 只加配置不改代码

### 完成动作

* `none` / `open_dir`（打开产物目录）/ `preview`（复用现有 HTML 预览）/ `copy`（输出复制剪贴板，会议号场景）

## 验收标准

* [ ] 「AI 功能」页展示四个功能项，全部可运行
* [ ] 发言稿：选文件 → 运行 → HTML 预览
* [ ] 周报：直跑出草稿 → 面板确认亮点 → 落盘终稿
* [ ] 会议预约：填表单 → 运行 → 会议号入剪贴板
* [ ] 会议查看/取消：查列表 → 选中 → 确认 → 取消成功
* [ ] 输出面板多 Tab 并行、流式回显、可取消（子进程被杀）
* [ ] 失败任务展示错误信息；超时可配置
* [ ] 配置表单可增删改功能项，改配置后菜单生效，无需改代码
* [ ] `go test ./...` 与前端测试通过

## 完成定义

* 后端单测：命令构建（参数拼接/引号转义）、进程取消、超时、resume 会话 id 提取
* 前端组件测试：功能列表、输出面板、参数表单
* Wails 绑定三处同步（App.js / App.d.ts / models.ts，见 docs/spec/cross-layer-contracts.md）
* README.md / docs/功能说明.md 更新

## 技术方案（关键决策汇总）

1. **多段编排**而非 stream-json 双向流：每段独立 `claude -p`，第二段用 `--resume` 续会话；交互确认由 WorkBench 面板按钮承载，保留 skill 内置安全规范（敏感操作须确认）
2. **不迁移 skills**：各功能项 cwd 指向 u63/u51/u54 项目根，项目级 .claude/skills 与 settings.json（env、enabledPlugins）天然生效；零迁移零风险
3. **插件 skill 经 cwd=u51 项目**触发（enabledPlugins 已启用）；headless 可用性列入先导实测
4. **凭证经 `--settings` 注入**或随项目 settings.json 生效，配置 JSON 只存引用不存明文 token
5. Windows 下 claude 为 WinGet Links 可执行链接，Go exec.Command 调用方式列入先导实测

## 决策记录（ADR-lite）

**背景**：`claude -p` 单发非交互，而周报（亮点确认）、会议取消（敏感操作确认）含交互回合。
**决策**：多段编排 + `--resume` 续会话 + 面板确认按钮（替代单发放行 / stream-json 全交互面板两方案）。
**后果**：保留 skill 安全规范、复杂度适中；代价是执行器需管理会话 id 与段间状态，面板需「待确认」状态机。若未来需要自由对话式交互，可升级 stream-json 桥，多段编排是其子集，无返工。

**背景**：设计文档原定 skills 全迁用户级。
**决策**：不迁移，cwd 指项目根。
**后果**：零迁移工作量、项目依赖（Portal 认证、MCP token、插件 marketplace）原样生效；代价是功能项强绑定三个存量项目路径，项目移动需改配置（可接受）。

## 明确不做

* 不新建独立 Web 页面、不做多端访问（设计文档已决策）
* 不实现 stream-json 双向流自由对话面板（未来可扩展）
* work-summary 等其他候选 skill 不入首批（配置就绪后加配置即可）
* 不做任务历史持久化（会话输出关闭即失，后续按需加）

## 技术备注

* 设计文档：`docs/plans/2026-09-08-ai-skill-menu-design.md`（4.2/4.4 节按本 PRD 修订：cwd 值、收拢策略、参数模型 form 类型）
* 真身路径（2026-09-08 侦察）：
  * 周报：`u63_项目管理/.claude/skills/ab-weekly-report`（链式依赖同目录 pmo-weekly-report；ab-pm-report-xlsx 已废弃不用）
  * 发言稿：u51_ppt生成 项目 settings.json `enabledPlugins: ab-office@ab-internal-plugins`（marketplace: 内网 git 192.168.71.100:30088）
  * 会议：`u54_tencent/.claude/skills/tencent-meeting-mcp`（MCP baseUrl mcp.meeting.tencent.com；token 在 u54 settings.json env）
* claude CLI 2.1.158 已验证参数：`--add-dir` / `--permission-mode` / `--settings` / `--plugin-dir` / `--continue` / `--resume` / `--output-format`
* **先导实测清单**（实施第一步）：
  1. Windows Go exec.Command 直接调 `claude` 是否需绝对路径/`.cmd` 解析
  2. cwd=u51 headless 下 agree-slides 插件 skill 能否触发
  3. cwd=u63 headless 下 ab-weekly-report 单发能否出草稿（Portal 认证随项目生效？）
  4. cwd=u54 + token env 下 MCP 工具可调；`--resume` 第二段续会话上下文保留验证
* 周报数据源实为 Portal API（非设计文档所述碎碎念库）；碎碎念库为设计早期假设，本 PRD 已修正
