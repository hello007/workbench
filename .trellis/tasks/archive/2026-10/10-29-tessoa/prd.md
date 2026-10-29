# 打开资源管理器优先使用 tessoa

## Goal

工作目录树「打开资源管理器」功能优化：优先检测本机是否可用 tessoa（https://tessoa.cn/ ，跨平台文件管理器），可用则用 tessoa 打开目标路径；不可用则降级为现有系统文件管理器行为。提升日常文件浏览体验（tessoa 支持分屏、预览、标签页）。

## What I already know

* 后端链路：前端 `OpenInExplorer`（wailsjs 绑定）→ `App.OpenInExplorer`（app_external.go:17）→ `FileOperationService.OpenInExplorer`（service/fileoperation.go:245）→ `openInFileManager`（平台分文件）
* Windows 现状（service/fileoperation_windows.go:13）：目录 `explorer path`；文件 `explorer /select,path`（定位选中）
* 非 Windows 现状（service/fileoperation_other.go:15）：`openWithSystemHandler`（Windows: `cmd /c start`；其他: `xdg-open`），文件场景打开父目录
* 前端调用点 3 处：DirectoryTree.vue:632（工作目录树）、FileTreePanel.vue:1467（文件树）、ContentPanel.vue:773（内容面板）
* 外部工具检测先例：Obsidian 走「设置 → 通用 → 外部应用」配置 `ObsidianPath` + `isObsidianProtocolRegistered` 协议检测（service/fileoperation.go:331 起一组方法）
* tessoa 为跨平台产品（Windows 10/11、macOS 12+、Linux glibc 2.35+），有「免安装」形态，官网未明确 CLI/PATH 集成细节
* 用户命令示例（Windows）：`tessoa.exe "D:/code/project/"`（目录）、`tessoa "d:/all.txt"`（文件，tessoa 自行打开到所在目录）

## Assumptions (temporary)

* tessoa 文件场景直接传文件原路径，由 tessoa 自行定位所在目录（与用户示例一致），不再做 `/select` 等价物
* tessoa 启动失败（检测到但启动报错）时降级系统文件管理器

## Decision (ADR-lite)

**Context**: 两个决策点——影响范围（仅工作目录树 vs 后端统一）与 tessoa 检测方式（仅 PATH vs 设置可配 + 探测兜底）。
**Decision**: 用户确认按推荐执行：
1. **后端统一生效**：改 `openInFileManager`（windows/other 双文件）一处，工作目录树 / 文件树 / 内容面板三入口全部跟随，行为一致且改动最小。
2. **PATH 探测 + 设置可配**：默认 `exec.LookPath("tessoa")` 零配置生效；沿用 ObsidianPath 先例在 model.AppSettings 增加顶层 `TessoaPath string` 字段（`json:"tessoaPath"`，零值即未配置，不涉 EnsureAllDefaults/PrivateCopy 两单点），设置面板「外部应用」区可手动指定（覆盖免安装不在 PATH 的场景）。
**Consequences**: 三入口行为统一变化（含未要求的内容面板）；service `OpenInExplorer` 签名加参（App 层读设置传入，前端绑定签名不变，wailsjs 无需同步 App.js/App.d.ts，仅 models.ts 补 AppSettings 字段）；tessoa 启动失败静默降级 explorer，用户无感知。

## Requirements (evolving)

* 调「打开资源管理器」时优先检测 tessoa 可用性；可用则 `tessoa <路径>` 打开
* tessoa 不可用 / 启动失败时，降级现有系统文件管理器行为（Windows: explorer /select 语义不变）
* 现有 explorer 主路径零回归（fileoperation_windows.go 注释红线）

## Acceptance Criteria (evolving)

* [ ] `model.AppSettings` 新增 `TessoaPath string`（`json:"tessoaPath"`），models.ts 三处同步（仅 models.ts，App 签名不变）
* [ ] `service.OpenInExplorer` 签名加 `tessoaPath` 参数；`App.OpenInExplorer` 读设置传入，前端绑定签名不变
* [ ] 新增 `service/tessoa.go`（无 tag 跨平台）：`resolveTessoaExecutable`——配置路径非空校验存在性优先，否则 `exec.LookPath("tessoa")`；不可用返回空串
* [ ] `openInFileManager`（windows/other 双文件）优先 tessoa 打开（目录与文件均传原路径），启动失败降级现有系统打开
* [ ] 本机无 tessoa 时行为与现状逐字节一致（Windows explorer /select, 语义不变）
* [ ] 单测：resolveTessoaExecutable 三分支（配置存在/配置缺失/PATH 探测注入）；GUI 启动链路测试按现有口径 Skip 标注手动验证
* [ ] SettingsPanel.vue「外部应用」区新增 tessoa 路径输入（照 obsidianPath 模式，占位符分平台）
* [ ] `go test ./service/` 通过，覆盖率门禁不回退；前端 `npm test` 通过

## Definition of Done (team quality bar)

* 测试更新（service 层单测，windows 分支用可注入探测函数规避真实环境依赖）
* `go test ./service/` 通过；覆盖率门禁不回退
* 文档更新（docs/功能说明.md 若行为面向用户可见）
* wailsjs 绑定无需变更（后端签名不变，除非选择前端分流方案）

## Out of Scope (explicit)

* 不改动 OpenInVSCode / OpenInWarp / OpenInObsidian 等其他外部打开功能
* 不做 tessoa 的注册表/协议检测（对比 Obsidian 的 obsidian:// 协议），除非探测方案确认需要

## Technical Notes

* 已查文件：app_external.go、service/fileoperation.go、service/fileoperation_windows.go、service/fileoperation_other.go、DirectoryTree.vue（handleOpenExplorer）
* 平台抽象约定（linux-platform.md）：编译期分叉走 `_windows/_other` 双文件；本改动探测逻辑若双平台复用则抽无 tag 文件
* 测试禁隐式依赖环境外部状态（test-stability.md）：tessoa 探测必须可注入 stub，禁真实 LookPath
* 测试平台断言拆双平台文件禁共享（linux-platform.md）
