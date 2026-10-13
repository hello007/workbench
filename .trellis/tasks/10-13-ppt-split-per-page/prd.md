# brainstorm: 工具箱 PPT 按页分割工具

## Goal

新增工具箱工具：对一个 PPT（.pptx）按页拆分，在 PPT 同级目录生成同名文件夹，内含每页一个独立的新 PPT。

## What I already know

* 用户需求：按页分割 → 同级 `<ppt名>/` 文件夹 → 每页一个新 PPT
* 工具箱现有模式（repo 实测）：
  * 前端 `frontend/src/components/ToolboxPanel.vue` — 工具项数组 + 独立 Dialog 组件（ProcessManagerDialog / PortListDialog / JsonYamlToolDialog / EnvVarDialog / HostsDialog 先例）
  * 后端 `service/process.go` 等 — Provider 抽象 + 测试注入 fake（禁环境外部状态）
  * 新增 service 须同步 `app_services.go` 两处 + `web_serve.go` applySink（如有事件）
* 技术栈：Go 1.26.6 + Wails v2.16 + Vue3/Element Plus；Windows 为主，Linux 支持已存在
* PPT 页拆分技术选型未定：COM 自动化（需 Office）/ 纯 Go 操作 pptx zip 结构 / 外部工具 —— 待 research

## Assumptions (temporary)

* 仅支持 .pptx（不支持 .ppt 老格式）——待确认
* 工具入口：工具箱对话框手动输入路径 vs 文件树右键 .pptx 文件——待确认

## Open Questions

* （等待用户确认默认决策，见 Decision）

## Requirements

* 对指定 .pptx 按页拆分，每页生成一个独立新 PPT
* 输出目录：PPT 同级、以 PPT 文件名命名的文件夹
* 双入口：文件树右键 .pptx「按页拆分」+ 工具箱对话框（手动输入/浏览路径）
* 命名规则：`<原文件名>-<两位页序号>.pptx`（如 `汇报-01.pptx`）
* 输出文件夹已存在：覆盖重生成（同名单文件覆盖）
* 拆分为长操作：`context.WithCancel` + 取消注册表接入（`registerCancel`/`unregisterCancel` defer LIFO）+ EventSink 进度事件（`emitEvent`）
* .ppt 老二进制格式明确拒绝，报「仅支持 .pptx」

## Acceptance Criteria

* [ ] N 页 PPT 拆出 N 个新 PPT，各含对应单页
* [ ] 输出到同级同名文件夹，命名 `原名-NN.pptx`
* [ ] 拆分不破坏页内容（母版/版式/主题/媒体/内嵌字体逐字节保留）
* [ ] 页序按 sldIdLst 三跳映射（slide 文件名 ≠ 页序），fixture 覆盖乱序命名
* [ ] 含节（sections）/自定义放映/备注页 fixture 拆分后 PowerPoint 可正常打开（无修复弹窗）
* [ ] 跨页超链接指向已删页时清理悬空关系
* [ ] 输出原子写（CreateTemp 同目录 → Rename）
* [ ] 拆分可取消，取消后 `E_GIT_CANCELLED` 中性提示
* [ ] .ppt 输入报明确错误
* [ ] service 层单测全链路（fixture 程序化构造 pptx，禁依赖本机 Office）

## Decision (ADR-lite)

**Context**：技术选型须同时满足「无 Office 安装保证」+「单测禁依赖本机 Office」；入口与命名须对齐工具箱既有模式。

**Decision**（用户未答复 AskUserQuestion，按推荐默认收敛，待确认）：
1. 方案 B 纯 Go OOXML（标准库 archive/zip + encoding/xml），最小编辑集见 research
2. 双入口：文件树右键 + 工具箱对话框
3. .ppt 明确 Out of Scope
4. 命名 `原名-NN.pptx`

**Consequences**：后手充足——未来可按 Provider 接口加 COM 可选增强（go-ole 已在依赖树）；畸异文件（加密/损坏 pptx）报错路径须友好。

## Definition of Done (team quality bar)

* 单测覆盖（service 层 Provider 注入 fake，禁依赖本机 Office）
* 覆盖率门禁达标（service ≥76%）
* 文档更新（功能说明.md / README.md 确认）
* wailsjs 绑定三处同步（App.js / App.d.ts / models.ts）

## Out of Scope (explicit)

* .ppt 老二进制格式（OOXML 时代前，需 COM 引擎，明确拒绝并提示）
* 媒体 GC（仅被已删页引用的图片/视频残留包内，正确性无损仅体积偏大，后续优化项）
* 按范围拆分（如 3-5 页合一个）、PDF 拆分——未来演进，Provider 接口已留扩展位
* 加密 pptx 拆分

## Research References

* [`research/pptx-split-approaches.md`](research/pptx-split-approaches.md) — 四方案对比（COM/纯Go OOXML/外部依赖/商业库），推荐方案 B 纯 Go 标准库：零依赖、保真度最高（未触碰 part 逐字节拷贝）、CI 可全链路测；最小编辑集 = sldIdLst + presentation.xml.rels + [Content_Types].xml + 删 slide/notesSlide part + 剥离 sections/custShows；已知坑：页序三跳映射（slide 文件名 ≠ 页序）、跨页超链接清理、输出复刻原子写惯例

## Technical Notes

* repo 结构：本目录即 workbench 源码根
* 工具箱先例：`service/process.go`（Provider 抽象 + 时间注入）、`ToolboxPanel.vue`
* research 进行中：pptx 按页拆分技术方案
