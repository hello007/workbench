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


## Session 117: 工具箱 PPT 按页拆分功能全流程落地

**Date**: 2026-10-13
**Task**: 工具箱 PPT 按页拆分功能全流程落地
**Branch**: `master`

### Summary

头脑风暴+调研收敛方案 B 纯 Go OOXML 拆分（四方案对比，COM 因 CI 不可测+无 Office 保证被排除）；PR1 拆分核心（最小编辑集六项 + 页序三跳映射 + 解析定位字节删除保真 + 原子写，16 用例）；PR2 取消注册表（复用 E_GIT_CANCELLED）+ EventSink 进度事件 pptx-split:* + App 绑定 + applySink 第六服务；PR3 前端双入口（文件树右键 Promise 直返规避共听面 + PptSplitDialog 闭包注销）+ 文档。后端 22 + 前端 23 新用例，service 覆盖 81.3%。trellis-check 修复 gofmt+过时注释；cavecrew-reviewer 独立审核零发现。沉淀 docs/spec/pptx-split.md + CLAUDE.md 关键规则。遗留：docs/API参考.md 建议单独任务全量重对

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `915888a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 118: 侧边栏素材管理 MVP——纯 Go SQLite+FTS5 中文检索 + PPT 单页索引

**Date**: 2026-10-13
**Task**: 侧边栏素材管理 MVP——纯 Go SQLite+FTS5 中文检索 + PPT 单页索引
**Branch**: `master`

### Summary

落地素材管理 V1.0 SRS 的 MVP 纵切片（纯 Go 零 COM）：modernc/sqlite 每库一 db + user_version 迁移；OOXML 解析页文本/标题回退链/演讲者备注，加密/损坏/权限分类降级；mtime+size 增量扫描 + 派生产物按真实拆分布局排除 + 可取消；FTS5 unicode61+CJK 按字切分同源+phrase+bm25 加权检索，聚合折叠/标签继承，10 万页基准 45ms/op；ActivityBar 素材管理面板（库管理/画廊双视图/检索/详情编辑/溯源/扫描进度）+ OpenDirectoryDialog 桥接。后端覆盖率 81.5%，前端 1627 用例，Linux 编译过。质量检查修复派生产物排除口径错（derived/ 凭空目录致拆分副本重复索引）等 5 项。spec 沉淀 docs/spec/assets-management.md（FTS 五契约/单连接池自锁/rowid 编码）+ CLAUDE.md 关键规则。遗留：聚合展开超 200 命中子页可能不全、COM 缩略图/导出/单页元数据编辑为二批。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `013032e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 119: 素材管理审核修复 14 项——写回守卫/分页/FTS 边界/关停收口

**Date**: 2026-10-14
**Task**: 素材管理审核修复 14 项——写回守卫/分页/FTS 边界/关停收口
**Branch**: `master`

### Summary

双路子代理审核 013032e（后端 cavecrew-reviewer + 前端 cavecrew-reviewer）发现 14 项：后端 FTS 纯标点词（实测零 token phrase 恒 0 行非报错，防御跳过）、zip 解压上限（共用函数透传不设限保拆分链路零变化）、ListAssetSlides 分页、建库孤儿 db 回滚、WaitScansDone 关停收口、.-01.pptx 误判、多词摘要逐词定位；前端双代次守卫（librarySeq/searchSeq）、切视图空兜底、tagOptions 预载、检索空态引导、扫描单刷新、聚合在途守卫、LIB_NOT_FOUND 自愈。质量核查读代码逐项验证另抓出 4 处守卫缺口（滚动追加/溯源路径无库代次守卫、contextChanged 缺库代次、loadingMore 防重失效）一并修复。覆盖率 service 81.6%，前端 1636 用例，Linux 编译过，spec 同步 assets-management.md。遗留：util/clipboard_windows.go 存量 vet 告警 7 处（先于本次，另案）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `5bbf04e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 120: util 剪贴板 unsafeptr 告警真修复：RtlMoveMemory 中转收口

**Date**: 2026-10-14
**Task**: util 剪贴板 unsafeptr 告警真修复：RtlMoveMemory 中转收口
**Branch**: `master`

### Summary

裸 go vet ./util/ 7 处 unsafeptr 告警（源码5+测试2）真修复。实证三结论：vet unsafeptr 无 syscall 白名单（Syscall/SyscallN 结果转换实测均 flag，检查器仅豁免 reflect.Value.Pointer/UnsafeAddr、SliceHeader.Data、指针算术回转三类）；x/sys v0.30-v0.48 全版本无 GlobalLock/GlobalUnlock/Clipboard API，封装路径不存在；RtlMoveMemory 中转模式 vet 构造性干净。d7d5b51 复发真相=当年未修，仅加注释+固化 -unsafeptr=false 压制。修法：Go 堆 []byte 编码 DROPFILES/UTF-16 + GlobalLock 裸 uintptr + RtlMoveMemory 单次拷入拷出，全程零 uintptr→unsafe.Pointer 回转，字节布局逐位不变，调用方签名零变化；新增 copyToHeap/readFromHeap 单点辅助（空缓冲防护）+零长防护用例，测试改走辅助锚定可移动堆重锁往返语义。压制移除四处同步：开发规范.md 静态检查章节、贡献指南 PR 清单、linux-platform.md §10、ci.yml 注释。验证全绿：go vet ./... 零告警、双平台 build、全量 6 包 go test、gofmt。Windows 手工验证点（本机未验，用户可验）：文件复制/剪切粘贴（含 Preferred DropEffect 剪切语义）、中文文本复制粘贴、资源管理器间跨应用拖拽源。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0a2040d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
