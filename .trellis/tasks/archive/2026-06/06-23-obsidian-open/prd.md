# 工作目录树/文件树新增"用 Obsidian 打开"

## Goal

为 WorkBench 增加"用 Obsidian 打开"入口，覆盖工作目录树、文件树右键菜单及内容面板"查看操作"按钮组；**文件夹 → 以自身作为 vault 目录，文件 → 以父目录作为 vault 目录**。同时支持在设置中自定义 Obsidian 程序位置（配置则优先使用，未配置走系统协议方案），并使用内置图标、在未检测到 Obsidian 时引导用户去设置配置。

## Requirements

### 后端（Go）

- `model/settings.go`：`AppSettings` 新增字段 `ObsidianPath string`（Obsidian 可执行文件自定义路径，留空表示未配置）。
- `service/fileoperation.go` 新增 `OpenInObsidian(path string) error`：
  1. `os.Stat` 解析 vault 目录：文件夹 → 自身；文件 → 父目录；不存在 → 返回错误。
  2. **优先级**：若调用方传入用户配置的 `obsidianPath` 且该文件存在 → 直接用该 exe 启动（见 Decision）；否则走系统协议方案。
  3. 系统协议方案：注册表预检 `obsidian://` 是否注册（`HKCR\obsidian\shell\open\command`，兜底 `HKCU\Software\Classes\...`）→ 未注册返回友好错误；已注册则 `cmd /c start "" "obsidian://open?path=<编码路径>"` + `util.HideCommandWindow`。
  4. 路径编码：`filepath.ToSlash` → `url.QueryEscape` → `+` 替换为 `%20`。
- `app.go` 新增 `App.OpenInObsidian(path string) bool`：内部读取 `settingsSvc.Load()` 取 `ObsidianPath` 传给 service；失败 `println` 并返回 false。
- 注册表访问为 Windows 专属，需用 `//go:build windows` + `_other.go` 兜底（对齐现有 `util/exec_windows.go` 模式）。

### 前端（Vue3）

- 重新生成 wailsjs 绑定（`App.OpenInObsidian`、`AppSettings.ObsidianPath`）。
- `SettingsPanel.vue`：在"通用"tab 新增"外部应用"分区，提供"Obsidian 程序路径"输入框（模板参照现有"Git Bash 路径"），绑定 `obsidianPath`，`onSettingsChange` 时随 `SaveSettings` 持久化。
- `DirectoryTree.vue` 右键菜单新增"用 Obsidian 打开"项（目录 → 自身作 vault）。
- `FileTreePanel.vue` 文件夹/文件两套右键菜单各新增"用 Obsidian 打开"项。
- `ContentPanel.vue" 查看操作"按钮组（文件夹区、文件区）各新增"用 Obsidian 打开"按钮。
- **图标**：统一使用内置 `frontend/src/assets/icons/obsidian.png`（已内置，128×128），通过 `import obsidianIcon from '.../assets/icons/obsidian.png'` 引用，以 `<img>` 渲染（其余菜单项仍用 Element Plus 图标组件，此为特例）。
- **失败文案**：未检测到 Obsidian（未配置 exe 且系统协议未注册）时提示「未检测到 Obsidian，请在【设置 → 通用 → 外部应用】中配置 Obsidian 程序路径，或安装 Obsidian 并至少运行一次」。

## Acceptance Criteria

- [ ] 工作目录树右键 → 用 Obsidian 打开 → 以该目录为 vault 启动 Obsidian。
- [ ] 文件树"文件夹"右键 → 用 Obsidian 打开 → 以该文件夹为 vault。
- [ ] 文件树"文件"右键 → 用 Obsidian 打开 → 以父目录为 vault。
- [ ] 内容面板选中文件夹/文件 → 查看操作 → 用 Obsidian 打开（同上规则）。
- [ ] 设置 → 通用 → 外部应用：可填写/清空 Obsidian 程序路径，保存后生效（重启不丢失）。
- [ ] 配置了有效 exe 路径时，以该 exe 打开 vault（优先于系统协议）。
- [ ] 未配置 exe 且系统未注册协议时，前端弹出引导提示（指向设置项），应用不崩溃。
- [ ] 菜单项/按钮显示内置 Obsidian 图标（项目自包含，不依赖项目外路径）。
- [ ] 后端单测覆盖：vault 目录解析（文件夹/文件/不存在）、URI 编码（空格/中文/反斜杠）。

## Definition of Done

- 测试新增/更新（Go 单测必须；前端按需）。
- `wails build` 构建通过、前端类型检查通过。
- `README.md` / `docs/功能说明.md` 如有行为变化则更新（完成后确认）。
- 出错路径有可读提示。

## Technical Approach

调用 Obsidian 的两级策略（详见 Decision）：

1. **用户配置优先**：设置中填了 `ObsidianPath` 且文件存在 → `exec.Command(obsidianPath, uri)` 直接启动（`uri = obsidian://open?path=<编码路径>`），跳过注册表预检。
2. **系统协议兜底**：未配置或 exe 不存在 → 注册表预检 `obsidian://` 是否注册 → 已注册则 `cmd /c start "" uri`；未注册返回友好错误。

vault 目录解析与 URI 编码为纯函数，便于单测。

## Decision (ADR-lite)

- **Context**：Obsidian 桌面版默认不在 PATH，不能照搬 VSCode/Warp 的 `exec.Command(name, path)`；且用户希望可自定义 Obsidian 程序位置，未检测到时引导配置。
- **Decision**：
  1. 调用走 `obsidian://open?path=<编码绝对路径>` URI 协议（参数名 `path`，已查官方文档核实）。
  2. 两级策略：用户配置的 exe 优先（`exec.Command(exe, uri)`），否则注册表预检 + `cmd /c start "" uri` + `util.HideCommandWindow`。
  3. 未检测到（exe 未配置/不存在 且 协议未注册）→ 返回友好错误，前端引导去设置。
  4. 设置项挂在"通用 → 外部应用"分区，`AppSettings.ObsidianPath` 持久化。
  5. 图标内置为 `frontend/src/assets/icons/obsidian.png`，不依赖外部路径。
- **Consequences**：
  - 与现有 `OpenWithDefaultApp` 同构、零新增依赖（`golang.org/x/sys` 已在 go.mod）。
  - 用户配置 exe 的启动参数暂定直接传 URI（`exec.Command(exe, uri)`）；**若实测发现需 `--run` 等标志，实现期微调**（注册表 `shell\open\command` 实测值为 `--run "obsidian://%1"`）。
  - 文件夹→自身、文件→父目录的解析在 service 层完成。
  - 目标目录尚未注册为 vault 时照常打开、交由 Obsidian 自行处理（尽力打开风格）。

## Out of Scope

- 跨平台（macOS/Linux）支持（暂仅 Windows，与现有一致；注册表预检用 build tag 隔离）。
- 在 Obsidian vault 内定位/打开具体文件（仅打开 vault）。
- 对 VSCode/Warp 等既有外部工具增设可配置路径（本次仅 Obsidian）。

## Research References

- [`research/obsidian-launch.md`](research/obsidian-launch.md) — Windows 下从 Go 可靠调用 Obsidian 打开 vault 的方式（已抓取官方文档核实）。

## Open Questions

- 用户配置 exe 时的确切启动参数（直接传 URI vs 需 `--run`）→ 实现实测确认，不阻塞（兜底方案已定）。

## Technical Notes

- `service/fileoperation.go:142-182` 现有 Open* 方法（实现模板）；`app.go:601-612` `GetSettings/SaveSettings`；`app.go:464-492` 现有 `App.OpenIn*` 包装模板。
- `model/settings.go` `AppSettings` 结构体（新增 `ObsidianPath` 字段）；`service/settings.go` `SettingsService.Load/Save`。
- `frontend/src/components/SettingsPanel.vue` "终端"tab "Git Bash 路径"输入框（设置项模板，行 76-82）；`onSettingsChange`/`SaveSettings`（行 400-413）。
- `DirectoryTree.vue:51-83, 256-284` 右键菜单与 `onMenuCommand`。
- `FileTreePanel.vue:175-293` 两套右键菜单；`705-748` onMenuCommand；`947-973` handleOpenIn*。
- `ContentPanel.vue:46-117` 查看操作按钮组；`478-510` handleOpenIn*；`326` 绑定导入。
- `frontend/src/assets/icons/obsidian.png`（已内置，128×128，源自用户提供的图标）。
- `util/exec_windows.go` `HideCommandWindow`（隐藏子控制台窗口的现成工具）。

## Implementation Plan（小步）

- **Step1 后端**：`AppSettings.ObsidianPath` + `OpenInObsidian`（含注册表预检 build-tag 隔离、路径编码纯函数）+ `App.OpenInObsidian`（读设置）+ Go 单测。
- **Step2 设置项**：`SettingsPanel.vue` "通用 → 外部应用"加 Obsidian 路径输入框 + 持久化。
- **Step3 前端入口**：重新生成绑定；三处组件加菜单项/按钮 + handler + 内置图标 import；未检测到文案。
- **Step4 收尾**：实测 exe 启动参数（必要时加 `--run`）；按需更新 `docs/功能说明.md`/`README.md`。
