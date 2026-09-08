# AI skill 自动发现与一键导入 + P0-1 表单联调（第 4 批）

## 目标

扫描 WorkBench 已管理工作目录与用户级目录下的 `.claude/skills/` 及已安装插件，解析 `SKILL.md` frontmatter，在 `AiFunctionConfigDialog` 配置对话框提供「从已发现 skill 导入」入口，自动回填 `command` / `cwd` / `description` / `name`，消除手记命名空间与目录的负担；导入后经第 2 批落地的四块表单补全 `params` / `completion` / `followUps` / `env` / `mcp` 等字段，端到端验证「导入 → 表单补全 → 保存 → RunStage 起进程」链路，兑现总览第 4 节 P1-1 → P0-1 依赖。

## 已知信息（抓样定论）

### 三类 skill 的实际目录结构与命名空间

| 来源 | 扫描路径 | command 拼接 | cwd |
| --- | --- | --- | --- |
| 用户级 | `~/.claude/skills/<name>/SKILL.md` | `/<name>` | 空（任意 cwd 生效，导入后用户在表单补填） |
| 项目级 | `<工作目录>/.claude/skills/<name>/SKILL.md` | `/<name>` | 工作目录 |
| 插件 | `~/.claude/plugins/cache/<marketplace>/<plugin>/<version>/skills/<name>/SKILL.md` | `/<plugin>:<name>` | scope=user 空；scope=project/local 填 projectPath |

抓样实例：用户级 `~/.claude/skills/drawio/SKILL.md`（command `/drawio`）；项目级 `git-manager/.claude/skills/release/SKILL.md`（command `/release`，cwd=git-manager）；插件 `~/.claude/plugins/cache/ab-internal-plugins/ab-office/0.3.0/skills/agree-slides/SKILL.md`（command `/ab-office:agree-slides`）。

### SKILL.md frontmatter 格式

YAML 头以 `---` 包裹，顶层含 `name`、`description`（多为双引号包裹的长字符串），可选 `version` / `metadata` / `argument-hint` / `allowed-tools` / `license` 等。解析只取 `name` + `description` 两字段。

### 前置待确认点定论（design 第 7 节遗留）

1. **插件 skill 目录结构与命名空间**：抓样纠正 design 4.1 假设——插件 skill 不在工作目录的 `.claude/plugins/` 下，而在用户级 `~/.claude/plugins/cache/` 下，经 `~/.claude/plugins/installed_plugins.json` 索引（key 格式 `<plugin>@<marketplace>`，每条含 `scope` / `projectPath` / `installPath`）。命名空间 `/<plugin>:<name>`，plugin 取 key 中 `@` 前部分（如 `ab-office`）。
2. **嵌套子目录**：抓样确认 skills 均为一级子目录（`<root>/skills/<name>/SKILL.md`），无嵌套。按一级子目录实现，与 Claude Code CLI 发现规则对齐（design 7.2 建议正确）。
3. **frontmatter 解析失败**：无 YAML 头或格式错误的 skill 跳过并日志记录，不阻断发现（design 7.3 建议正确）。

### 复用点（第 1-3 批已完成，勿重复改动）

- `AiFunctionConfigDialog.vue` 四块表单（params/followUps/env/mcp）+ MCP stdio（第 2 批 P0-1）：本批只在其顶部加导入入口，不动表单内部
- `AiFunction` / `AiParamSpec` / `AiMcpConfig` model（第 2 批）：本批新增 `SkillDescriptor`，不改既有
- `DirectoryService.Load()`（工作目录列表）：复用，不重写
- `ScanCacheManager` / `GetRepoFilterList` mtime 缓存模式：复用其「mtime 变化则重扫 + 手动刷新强制重扫」思路
- `AiTaskState` / `AiTaskRunResult` / `AiTaskMetrics` / `AiTaskHistory` / `pumpOutput` / `GetAiTaskOutput` / 历史归档（第 1/3 批）：不动
- `concurrencySem` 并发上限（第 1 批）：不动

## 需求

### 后端

1. `model/skill_discovery.go` 新增 `SkillDescriptor`（`name` / `description` / `command` / `cwd` / `source` / `sourceDir` / `plugin`，json tag 对齐前端）
2. `service/skill_discovery.go` 新增 `SkillDiscoveryService`：
   - 输入复用 `DirectoryService.Load()` 工作目录列表
   - 扫描用户级 `~/.claude/skills/` + 每个工作目录 `.claude/skills/` + 读 `~/.claude/plugins/installed_plugins.json` 遍历各插件 `installPath/skills/`
   - 每个含 `SKILL.md` 的一级子目录解析 frontmatter（引入 `gopkg.in/yaml.v3`）
   - 去重键 `source + sourceDir`；同名不同来源都保留，来源标注区分（user/project/plugin）
   - command 拼接按上表规则；cwd 按来源与 scope 填建议值
   - frontmatter 解析失败的 skill 跳过并日志，不阻断
3. `app.go` 新增 `GetDiscoveredSkills()` 绑定（无参数，返回 `[]*model.SkillDescriptor`）
4. 缓存：内存缓存 + 各源目录 mtime 摘要 fingerprint 比对，二次打开瞬时；`RefreshDiscoveredSkills()` 强制重扫

### 前端

5. `AiFunctionConfigDialog.vue` 基础字段区顶部（「斜杠命令」字段上方）新增「导入 skill」按钮：
   - 点击调 `GetDiscoveredSkills()` 拉列表
   - `el-dialog` 表格：名称 | 描述 | 来源 | 命令 | 目录，支持名称/描述模糊搜索
   - 选中后回填 `editing.command` / `editing.cwd` / `editing.description` / `editing.name`（name 作功能项 name 初值）
   - 已在编辑的功能项也可点「导入」覆盖 command/cwd（修正填错配置）
   - 导入对话框内「刷新」按钮调 `RefreshDiscoveredSkills()` 强制重扫

### 跨层同步

6. 新增 `GetDiscoveredSkills` / `RefreshDiscoveredSkills` App 方法同步 `frontend/wailsjs/go/main/App.js` + `App.d.ts`
7. `model.SkillDescriptor` struct 同步 `frontend/wailsjs/go/models.ts`（字段声明 + 构造函数赋值，`omitempty` → `?:`）

### P0-1 联调

8. 导入 skill 后用第 2 批四块表单补全 params（none/file/text/form）、completion、followUps、env、mcp，端到端验证导入→表单→保存→RunStage 起进程链路；确认导入回填的 command/cwd 经表单保存后 RunStage 能正确起进程

## 验收标准

- [ ] 导入对话框列出用户级 + 所有工作目录 + 所有已安装插件的 skill，含名称/描述/来源/命令/目录
- [ ] 选中后 command/cwd/description 正确回填（插件 skill 命名空间 `/<plugin>:<name>` 拼接正确，如 `/ab-office:agree-slides`）
- [ ] 同名多来源 skill 均展示，来源标注区分（用户级/项目级/插件）
- [ ] 模糊搜索（名称/描述）过滤生效
- [ ] mtime 缓存生效，二次打开瞬时；「刷新」按钮强制重扫
- [ ] 导入→表单补全 params→保存→RunStage 起进程端到端跑通（P0-1 联调）
- [ ] frontmatter 解析失败的 skill 跳过不阻断发现
- [ ] 前后端双绿（`go test ./...` + `cd frontend && npm test`），wailsjs 同步完成（App.js / App.d.ts / models.ts 三处）

## 完成定义

- 后端单测覆盖 `SkillDiscoveryService`（三源扫描、frontmatter 解析、去重、命名空间拼接、解析失败跳过）
- 前端单测覆盖导入对话框（拉列表、模糊搜索、回填字段、刷新）
- `go test ./...` 与 `cd frontend && npm test` 双绿
- wailsjs 三处同步，`git status` 确认 `frontend/wailsjs/` 无残留
- README.md 按需更新（CLAUDE.md 要求功能完成后确认）

## 技术方案

### 实施顺序

1. `model/skill_discovery.go`：`SkillDescriptor` struct
2. `service/skill_discovery.go`：`SkillDiscoveryService`（扫描 + frontmatter 解析 + 去重 + 缓存）
3. `app.go`：`GetDiscoveredSkills()` / `RefreshDiscoveredSkills()` 绑定 + `startup()` 注入 service
4. wailsjs 同步：`App.js` / `App.d.ts` / `models.ts` 三处
5. `AiFunctionConfigDialog.vue`：导入按钮 + 导入对话框 + 模糊搜索 + 回填
6. 缓存：mtime fingerprint 比对 + 刷新按钮
7. P0-1 表单联调：导入→补全 params→保存→RunStage 起进程验证
8. 测试：后端单测 + 前端单测，双绿后提交

### 关键实现点

- **frontmatter 解析**：`SKILL.md` 读取后定位首尾 `---`，截取 YAML 头用 `yaml.v3` Unmarshal 到 `struct{Name, Description string}`；无 YAML 头或解析失败跳过并 `println` 日志
- **插件 skill 扫描**：读 `~/.claude/plugins/installed_plugins.json`（`map[string][]struct{Scope,ProjectPath,InstallPath string}`），对每个插件 key 按 `@` 拆分得 plugin 名，遍历 `installPath/skills/*/SKILL.md`，scope 决定 cwd（user 空、project/local 填 projectPath）
- **去重**：`map[string]*SkillDescriptor`，key = `source + ":" + sourceDir`，同路径不重复；同名不同来源 source 不同故都保留
- **缓存 fingerprint**：`用户级 skills 目录 mtime` + `installed_plugins.json mtime` + `各工作目录 .claude/skills mtime`（按工作目录 ID 排序）拼接为字符串；与缓存 fingerprint 一致则返回缓存，不一致重扫；不落盘（内存缓存，重启重扫成本可接受）

## 决策（ADR-lite）

### ADR-1：插件 skill 扫描路径纠正 design 4.1 假设

**Context**：design 4.1 假设插件 skill 在 `<工作目录>/.claude/plugins/<plugin>/skills/`，抓样发现工作目录下无 `.claude/plugins/`，插件 skill 全在用户级 `~/.claude/plugins/cache/<marketplace>/<plugin>/<version>/skills/`，经 `installed_plugins.json` 索引（key=`<plugin>@<marketplace>`，含 scope/projectPath/installPath）。

**Decision**：插件 skill 扫描改为读 `~/.claude/plugins/installed_plugins.json`，对每个插件遍历其 `installPath/skills/<name>/SKILL.md`；命名空间 `/<plugin>:<name>`（plugin 取 key 中 `@` 前部分）；cwd 按 scope 填（user 空、project/local 填 projectPath）。列出所有已安装插件 skill，不按工作目录过滤（与 design 4.4 无参数 `GetDiscoveredSkills()` 一致）。

**Consequences**：插件 skill 扫描不绑定工作目录（与 design 假设不同），但符合 Claude Code CLI 实际发现规则；列表含全部已安装插件 skill，用户可见全貌；scope=project/local 的插件 skill cwd 自动填 projectPath，导入后若该工作目录不在已管理列表则用户在表单微调。

### ADR-2：frontmatter 解析引入 gopkg.in/yaml.v3

**Context**：`go.mod` 无 `yaml.v3` 依赖；SKILL.md frontmatter 的 description 可能含引号、长字符串。

**Decision**：引入 `gopkg.in/yaml.v3`（design 4.3 明确允许），稳健解析 frontmatter，规避手写正则处理引号/多行的边界缺陷。

**Consequences**：新增一个直接依赖；解析逻辑简单可靠。

### ADR-3：cwd 留空与前端必填校验

**Context**：用户级 skill 与 scope=user 的插件 skill 任意 cwd 生效，导入时无建议 cwd 值；`AiFunctionConfigDialog` 必填校验含 cwd。

**Decision**：导入时这类 skill 的 cwd 回填为空，用户在表单补填 cwd 后保存（必填校验拦截空值并提示）。符合「导入填初值、用户微调」设计，cwd 本属运行时目录由用户决定。

**Consequences**：导入后必须补 cwd 才能保存，符合预期。

### ADR-4：缓存用内存 + fingerprint，不落盘

**Context**：skill 发现是只读无副作用操作，结果不大（数十项），重启重扫成本低。

**Decision**：内存缓存 + 各源目录 mtime fingerprint 比对，不落盘（与 `repo_scan_cache.json` 落盘不同）。

**Consequences**：实现简化，重启后首次打开重扫（可接受）；二次打开瞬时。

## 范围外

- 不改第 2 批四块表单内部逻辑（只在其顶部加导入入口）
- 不改 `AiFunction` / `AiParamSpec` / `AiMcpConfig` 既有 model
- 不按工作目录过滤插件 skill 可见性（scope=project/local 仅影响建议 cwd，不过滤）
- 不扫描 skills 子目录嵌套（按一级子目录，与 CLI 对齐）
- 不做 skill 启用/禁用、版本对比、插件 marketplace 管理等扩展

## 技术备注

### 关键文件

| 文件 | 改动 |
| --- | --- |
| `model/skill_discovery.go` | 新增 `SkillDescriptor` |
| `service/skill_discovery.go` | 新增 `SkillDiscoveryService`（扫描 + 解析 + 去重 + 缓存） |
| `app.go` | 新增 `GetDiscoveredSkills()` / `RefreshDiscoveredSkills()`，struct 加 `skillDiscoverySvc`，`startup()` 注入 |
| `go.mod` / `go.sum` | 引入 `gopkg.in/yaml.v3` |
| `frontend/src/components/AiFunctionConfigDialog.vue` | 顶部「导入 skill」按钮 + 导入对话框 + 模糊搜索 + 回填 |
| `frontend/src/components/__tests__/AiFunctionConfigDialog.spec.js` | 导入流程、字段回填、刷新测试 |
| `frontend/wailsjs/go/main/App.js` | 增 `GetDiscoveredSkills` / `RefreshDiscoveredSkills` 绑定 |
| `frontend/wailsjs/go/main/App.d.ts` | 增两方法 TS 声明 |
| `frontend/wailsjs/go/models.ts` | 增 `SkillDescriptor` class |

### 跨层契约

- 新增 App 方法（`GetDiscoveredSkills` / `RefreshDiscoveredSkills`）须同步 `App.js` + `App.d.ts`（参数顺序数量三处一致）
- `SkillDescriptor` struct 字段须同步 `models.ts`（字段声明 + 构造函数赋值，`omitempty` → `?:`，字段名按 json tag）
- `frontend/wailsjs/` 已 gitignore，`git status` 不应有残留

### 参考

- `docs/plans/2026-09-08-ai-skill-discovery-design.md` — P1-1 主体设计
- `docs/plans/2026-09-08-ai-optimization-overview.md` — 总览（第 5 节第 4 批、第 4 节 P1-1→P0-1 依赖、第 1/2/3 批衔接点）
- `docs/plans/2026-09-08-ai-config-form-design.md` — P0-1 表单（导入后表单补全联调对照）
- `docs/spec/cross-layer-contracts.md` — 跨层契约（wailsjs 三处同步）
