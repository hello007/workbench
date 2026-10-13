# Journal - liuyang (Part 3)

> Continuation from `journal-2.md` (archived at ~2000 lines)
> Started: 2026-10-10

---



## Session 111: 工具箱新增进程管理（端口/关键字查进程 + 停止/强杀）

**Date**: 2026-10-10
**Task**: 工具箱新增进程管理（端口/关键字查进程 + 停止/强杀）
**Branch**: `master`

### Summary

工具箱新增「进程管理」：按端口号或进程名关键字检索本机进程，展示进程名/PID/端口/协议/状态/内存，每行可「停止」（发优雅请求→5s 轮询实测存活→未退自动转强杀）或「强杀」，进程树为默认关闭的二级选项。技术选型为零依赖方案（Windows netstat+tasklist，Linux /proc），不引入 gopsutil。

关键产出：用户手工实测推翻了调研报告中「有窗口 GUI 进程可优雅关闭、无窗口后台进程无优雅通道」的结论——实测 ping（隐藏窗口控制台）响应并退出，cmd /k（有可见窗口）报「只能强行终止」且仍存活，方向恰好相反。正确语义是「能否优雅取决于目标进程是否响应，无法按有无窗口预判」，故实现不做事前预判、统一轮询实测，且不以 taskkill 的 SUCCESS 判定已退出（仅代表信号送达）。该更正已同步到 8 个代码文件注释、用户文档、PRD 与 spec，调研报告原结论标注为被推翻并保留推理过程。

实施分 4 个 PR：后端底座（util 平台分叉+解析纯函数）、service 编排与 App 绑定与装配、前端弹窗与三级确认、文档与规范沉淀。质量检查修复 5 个问题，其中 2 个为实质缺陷：cmd.Output() 丢弃 stderr 致 taskkill 失败形态识别失效、终止命令无超时可致前端卡死。

验证：go build（Windows+Linux 交叉）通过；go test ./util/ ./model/ ./service/ 全绿（service 覆盖率 80.8%，门禁 76%）；npm test 1497 用例全绿。新增 docs/spec/process-management.md 并在 CLAUDE.md 关键规则表与 spec 索引各加一条。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6ec28e9` | (see git log) |
| `8d9cca4` | (see git log) |
| `b9e60ca` | (see git log) |
| `0614b4e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 112: 工具箱图标替换为内联 SVG 组件

**Date**: 2026-10-10
**Task**: 工具箱图标替换为内联 SVG 组件
**Branch**: `master`

### Summary

sidebar 与工具箱面板标题栏图标由 element-plus SetUp 换为新增的 ToolboxIcon.vue（内联 SVG + fill=currentColor），三态由容器文字色自动驱动。原需求的 工具箱.png 为纯黑单色，在深色 sidebar（--sidebar-bg #0f172a）上零像素可见且不吃 currentColor，故改 SVG。规范沉淀：frontend-visual-conventions.md 新增「图标着色与接入」章节 + 正反示例；CLAUDE.md 关键规则表追加子句；docs/常见问题.md 新增「wails dev 后任务栏图标变通用图标」条目（根因=Windows 图标缓存陈旧，非代码问题）。验证：1497 用例通过、覆盖率四指标 ≥70%、build 通过、headless 三态渲染实测。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `b0b9fec` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 113: 工具箱续作：端口占用一览 + JSON/YAML 格式化校验

**Date**: 2026-10-13
**Task**: 工具箱续作：端口占用一览 + JSON/YAML 格式化校验
**Branch**: `master`

### Summary

工具箱新增两个工具：①端口占用一览（PortListDialog）——零后端新增，复用 ListProcesses 空关键字监听态默认视图前端扁平化为一行一端口按端口号升序，行内终止复用两级停止链路（优雅→5s 轮询→强杀）与确认交互，行级防重按 PID 使同进程多端口行联动禁用；②JSON/YAML 工具（JsonYamlToolDialog）——纯前端 js-yaml（既有依赖零安装），显式选格式不做自动探测，校验错误带行号定位（JSON position 换算 / YAML mark.line+1），格式化缩进 2/4、压缩、互转，有损场景显式处理：多文档拒绝转换、注释/锚点 token 级检测后确认再转。PRD 偏离记录：原方案后端加端口视图方法改为前端扁平化（枚举底座已含全部所需字段），复用约束沉淀 process-management.md §9.1。质量门禁：1532 前端用例全绿（新增 53）、覆盖率四指标 83.6/76.7/80.5/86.1 全过 ≥70 门禁、build 通过、后端零改动。代码审核（cavecrew-reviewer）0 critical/major，3 minor 中采纳 1 条（e.line 死代码清理 96af911），2 条驳回并留评估理由（pendingPids 复合键违背终止单位语义、# 误报为保守方向有意取舍）。文档同步：功能说明 13.3/13.4、README 工具箱条目、CLAUDE.md 关键规则追加子句。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `623e132` | (see git log) |
| `c97cb05` | (see git log) |
| `96af911` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 114: 工具箱环境变量查看器

**Date**: 2026-10-13
**Task**: 工具箱环境变量查看器
**Branch**: `master`

### Summary

工具箱第 5 项「环境变量」只读查看器：Windows 系统/用户/进程三级（注册表 HKLM/HKCU + REG_EXPAND_SZ 展开，进程级 os.Environ 双平台共享省 /proc 分叉），Linux 仅进程级；util provider 注入对齐 ProcessService，单来源失败写 Group.Error 不拖垮整体、全失败 E_ENV_ENUM_FAILED；前端 EnvVarDialog 动态 tab + 过滤 + 敏感命名打码临时显值 + 复制恒真实值；wailsjs 三处同步（Source 普通 string 避 MISSING_EXPORT）；测试 fixture 注入零依赖本机环境，覆盖率全档达标；check 修注册表 fixture 句柄泄漏；文档功能说明/README/CHANGELOG 更新，无新 spec 沉淀

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a6d0b1e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 115: 工具箱第三期：hosts 查看器（只读）

**Date**: 2026-10-13
**Task**: 工具箱第三期：hosts 查看器（只读）
**Branch**: `master`

### Summary

沿用 env-viewer 模式新增 hosts 只读查看器：util 无 tag 共享解析（netip 兼容 IPv6 zone/BOM/CRLF/未识别行）+ 路径常量 _windows/_other 分叉；service 组级 Error 形态复用 sourceFailureMessage；HostsDialog 720 档弹窗带过滤/注释开关/复制单行复制 IP/loadSeq 守卫；测试 t.TempDir fixture 全注入零依赖本机 hosts；覆盖率 service 80.9% util 65.9% 门禁全达标； Kind 用普通 string 规避 MISSING_EXPORT 坑。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f2a6803` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 116: 文件树复制路径二级菜单——绝对/相对/@文件引用 + 内容面板路径下拉

**Date**: 2026-10-13
**Task**: 文件树复制路径二级菜单——绝对/相对/@文件引用 + 内容面板路径下拉
**Branch**: `master`

### Summary

文件树右键「复制路径」改二级菜单（绝对/相对/@引用/文件名，纯 CSS hover 飞出，目录节点补齐复制文件名）；新增 utils/relativePath.js（分隔符归一 + 根外防御分支）；内容面板路径行改 el-dropdown 三项同口径；vitest 全量 1580 绿；spec 沉淀自绘菜单二级飞出子菜单模式（overflow:hidden 裁剪陷阱 + hover 不脱范围原理）至 frontend-visual-conventions.md

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6c5a987` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
