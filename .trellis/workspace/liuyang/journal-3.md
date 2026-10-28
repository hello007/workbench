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


## Session 121: 素材管理第五批：素材视图与选中体系（SRS §2.3）

**Date**: 2026-10-15
**Task**: 素材管理第五批：素材视图与选中体系（SRS §2.3）
**Branch**: `master`

### Summary

列表视图（三语境列集+底册join）、全屏预览翻页（视图序列D1零后端）、文件/幻灯片双模式选中（键契约禁slides.id+浮动条+清空点位矩阵）、缩放滑块（固定卡宽消突变）、双击交互（预览/文件过滤）、失败缩略图自动补恰一轮；真机反馈六项修复全过。附带D7跳转零可见修复（真机两轮定性）：owned镜像持久化+残留回收+楔死实例并回+WPS应用级不可见Visible兜底。前端1746用例+后端全量绿，覆盖率84.5%；spec沉淀assets-management §8+assets-thumbs-com §9.1

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bebaeee` | (see git log) |
| `f024a52` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 122: 素材管理第六批：批量打标签与多级标签树（SRS §2.4）

**Date**: 2026-10-16
**Task**: 素材管理第六批：批量打标签与多级标签树（SRS §2.4）
**Branch**: `master`

### Summary

批量打标签（AssetSelectionBar 入口，文件/幻灯片双模式，addTags/removeTags 双数组每条目一事务逐条目明细）+ 多级标签树（tags.parent_id 启用，ListAssetTags 独立 API，级联命中两路径同源展开）+ 标签管理弹窗（CRUD/级联删/防环/schema v6 sort_order 拖拽排序）。AssetTagTreeSelect 单组件三处复用；真机修复过滤浏览语境刷新链卡片全消（ensureFilteredSlidesLoaded 续拉兜底）；冒烟反馈四条（拖拽排序/平铺默认开/控件高度统一/radio 不堆叠）+ 小树默认全展开。spec 沉淀 assets-management.md §9 + CLAUDE.md v2.22。service 79.3%，前端 1813 用例，E2E 58 用例，两轮 cavecrew-reviewer 零实质缺陷。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `63b9d6b` | (see git log) |
| `7362962` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 123: 素材管理第七批：facet 多维筛选 + 保存检索条件（SRS §2.5）

**Date**: 2026-10-16
**Task**: 素材管理第七批：facet 多维筛选 + 保存检索条件（SRS §2.5）
**Branch**: `master`

### Summary

第七批落地 SRS §2.5：后端 AssetSearchOptions 扩 facet 字段（星级/修改时间/文件大小，文件级口径，幻灯片条目继承归属文件属性），assetsFacetFilter 条件单点三路径共用，空查询语义重定义（有 facet 走 searchByFacetsOnly 返文件条目），保存检索条件 service（data/asset_saved_searches.json 原子写 upsert + 3 App 绑定）；前端 AssetFilterPopover 三维弹层（换算单点，相对档现算 + 止日末毫秒可逆锚点）+ 双语境接入 + 空态引导 + 保存/调取/覆盖/删除与异库 tagNames 重映射。加权方向实测一致未动权重。service 覆盖 79.6%、前端 1857 用例全绿 85%。审核两轮零修复（第二轮严重项核实为误读）。spec 沉淀 assets-management.md §3/§8.2/§8.6 + CLAUDE.md v2.23 + 功能说明 14.3。真机冒烟过后 feat commit 39b8ba2。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `39b8ba2` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 124: 素材管理第八批：检索排序器 + 批量修改评分

**Date**: 2026-10-16
**Task**: 素材管理第八批：检索排序器 + 批量修改评分
**Branch**: `master`

### Summary

SRS §2.5.3/§2.6 收尾：三路径排序单点 sortEntriesByOptions（相关性档零行为变化，聚合前分页前执行），排序口径统一文件级与 facet 同源，次级键 score→sortPath→sortNum→refID 跨分页稳定，并发删除窗口 props 缺键剔除防零值异常序；AssetSavedSearch 补 sortBy/sortDir 零值向后兼容；BatchSetAssetRating 单一入口混合目标每条目一事务（页条目整行回写备注，键 (file_id,slide_num)），AssetBatchRatingDialog 复刻标签弹窗形态成功不清选中。service 79.6%/前端 85.28% 门禁绿，trellis-check + 三轮独立审核，spec 沉淀 assets-management.md §10；真机冒烟暂缓由用户后续执行。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `5def99f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 125: 素材管理第九批：导出元数据 + 文件丢失重链（SRS §2.6.4/§2.6.5）

**Date**: 2026-10-18
**Task**: 素材管理第九批：导出元数据 + 文件丢失重链（SRS §2.6.4/§2.6.5）
**Branch**: `master`

### Summary

第九批两 PR 全链路：brainstorm 定 8 项 ADR（lost 扫描标记替代删除/范围语境跟随/页行+文件行粒度/MD 每页一卡/路径口径/哈希校验重链/前缀替换预览+自动扫描/两批 PR）。PR1 导出：trellis-check 修 4（预览代次守卫/0 命中哨兵/placeholder/分页边界测试），审核 1P1 全局锁阻塞修为三段式（物化 UNION ALL 文件行独立一路+批间短会话水合+锁外写汇）+ O_EXCL 收口 + 真中止通道（exportCancels 走 opMu + 弹窗放弃语义），定向 check 全过。PR2 丢失重链：check 修 2（复活资格门槛 fileHasSlides/嵌套前缀误拒+腾位降序），审核 1🔴 探测配对下标 zip 串写（fileID map 修复+两序分化用例，缺陷还原实证）+3🟡（清理资格复验 RowsAffected/processing 永不直复活/预览输入置空），复核 2🟡（toast 先判 reparsed/写面零命中上抛）+1🔵 均修。门禁 service 80.0%/前端 85.02%，E2E+6。spec 沉淀 §11 三节 + CLAUDE.md 关键规则 2 行 + API参考 6 绑定 + README。真机冒烟暂缓，清单登记任务目录 smoke-checklist.md（含第八批遗留四项）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `404b142` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 126: 素材管理第十批：索引配置与素材库备份（SRS §2.7.3/§2.7.5/§14.16）

**Date**: 2026-10-18
**Task**: 素材管理第十批：索引配置与素材库备份（SRS §2.7.3/§2.7.5/§14.16）
**Branch**: `master`

### Summary

第十批完成：每库排除规则（单段 glob+后缀归一，保存即自动重扫）+ excluded 索引态（先排除后存在性/与 lost 互斥/检索不可见导出照导/直复活单事务重建 FTS）+ 备份域（VACUUM INTO+integrity_check/保留 5 份 _N 深序/重建删库前自动/删库 allowNoBackup 升级确认/还原步骤序含导出复查封竞态）+ 哈希恢复（路径优先 hash 兜底字典序后者胜）。流程：brainstorm 6 问收敛 prd → 双 research 实证（glob 语义/VACUUM INTO）→ 3 PR 实施 → check 修 3 处 → 独立审核两轮 9 项修复+复审 → spec 沉淀 §12+CLAUDE.md v2.24。门禁 service 80.0%/前端 85.14%。遗留：八/九/十批真机冒烟（清单 archive/2026-10/10-18-srs-2-7-3/smoke-checklist.md）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `411154c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 127: 素材管理八/九/十批真机冒烟（RPC 驱动面）+ 缺陷修复

**Date**: 2026-10-18
**Task**: 素材管理八/九/十批真机冒烟（RPC 驱动面）+ 缺陷修复
**Branch**: `master`

### Summary

冒烟 RPC 驱动 45/46 过（S1 建库/S2 排除规则全生命周期含 FTS 重建验证点/S3 备份还原哈希恢复/S4 导出 lost 重链前缀替换/S5 排序 facet 聚合/S6 删库失败升级链），驱动脚本归档 archive/2026-10/10-18-srs-2-7-3/rpc_smoke.py（fixture 程序化构造复刻 service 测试形态，无头 serve 独立目录端口隔离桌面实例）。唯一缺陷=排除后缀归一未去重，任务 10-18-s2-1b 修复（保序去重+大小写边界守护+修正锚定缺陷行为的既有断言），commit 0035b86。UI 呈现面（toast/弹窗/进度条/Excel/Obsidian/COM 缩略图跳转）待用户真机手点。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a492a06` | (see git log) |
| `0035b86` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 128: 素材管理八/九/十批冒烟收尾：UI 手点面通过

**Date**: 2026-10-18
**Task**: 素材管理八/九/十批冒烟收尾：UI 手点面通过
**Branch**: `master`

### Summary

用户真机手点通过全部剩余项（UI 呈现面：导出弹窗三态/Excel CSV/Obsidian MD/lost 标记与重定位入口/索引设置弹窗/重扫进度条/备份清单弹层/还原与删库强确认/已排除统计；COM 链：缩略图生成→画廊预览→单页跳转）。八/九/十批冒烟全部收口，删除 assets-smoke-pending 记忆。下一批预告：单页 OCR（SRS §2.2/§2.4，提示词已备）。

### Main Changes

(Add details)

### Git Commits

(No commits - planning session)

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 129: 工具箱全窗口面板化 + PPT 拆分文件夹批量拆分

**Date**: 2026-10-19
**Task**: 工具箱全窗口面板化 + PPT 拆分文件夹批量拆分
**Branch**: `master`

### Summary

工具箱改造为第二个独占一级面板（左功能清单+右内容画布），7 个工具 Dialog→Panel 去弹窗内嵌（@open 初始化迁 onMounted、事件监听生命周期=组件挂载/卸载），拷贝到抽为 CopyToPanel；后端新增 PptxSplitService.SplitFolder 递归扫描串行批量拆分（产物排除复用 isPptxSplitDerivative、整批可取消 ctx 键=目录、单文件失败续跑记 Item.Error、新增 pptx-split:batch:progress 文件级事件、终态仅经返回值汇总不逐文件发 done/error）；前端 PptSplitPanel 单文件/文件夹双模式+双层进度+成败汇总。go test 与 vitest 全绿（service 覆盖 80.2%、前端 85.3%），spec 沉淀 pptx-split.md/frontend-visual-conventions.md/CLAUDE.md 关键规则

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `476d647` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 130: 素材购物栏：页级收集 + 拖拽排序 + 跨库合并导出 PPT

**Date**: 2026-10-20
**Task**: 素材购物栏：页级收集 + 拖拽排序 + 跨库合并导出 PPT
**Branch**: `master`

### Summary

brainstorm 收敛 PRD（路线 A 基底注入式合并，研究确认纯 Go 无现成合并库）；PR1 拆分 GC 集合化泛化 + ExportPptxPageSubset；PR2 跨包子图注入 MergePptxPages（抓出 sldId 数字 id 被 r:id 污染的 encoding/xml 标签缺陷，parsePptxListIDs 命名空间精确解析）+ PptxMergeService 编排；PR3 购物栏存储 service + AssetCartDrawer 前端（三处加号/批量加页/溯源/SaveFileDialog 导出）+ 装配与 wailsjs 同步 + 文档 spec 沉淀（pptx-merge.md）；三轮 trellis-check 全过，后端/前端全量测试绿；真机冒烟通过，任务归档

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bc631e3` | (see git log) |
| `5f411eb` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 131: pptx 文件树预览：COM 转 PDF 复用 pdfjs viewer

**Date**: 2026-10-20
**Task**: pptx 文件树预览：COM 转 PDF 复用 pdfjs viewer
**Branch**: `master`

### Summary

文件树 .pptx/.ppsx 点击预览落地：PptxPreviewService（缓存索引+LRU 512MB+在途防抖+瞬态重试）编排，COM 转换排进 assets-thumbs 既有 STA worker（ConvertToPdf，禁双队列互杀；失败/超时 Discard 置脏；复用用户实例禁 SaveAs 走 ExportAsFixedFormat，SaveAs(32) 失败回退 FixedFormat(2)）；前端 pptx 分支复用 buildPdfViewerSrc 走 pdfjs，代次守卫+卸载复位。真机验证踩出并修复 dev 模式缺陷：Vite SPA fallback 吞 /preview-pdf 致 PreviewHandler 永不命中（dev 黑屏生产正常），vite.config.js 前置中间件 404 触发 Wails dev 回退 Handler——新契约：新增不经 Vite 静态链路的同源路由须同步加进该插件前缀判定。spec 沉淀 docs/spec/pptx-preview.md + CLAUDE.md v2.26；check 修复 P1 会话自愈缺陷等 3 项；WPS SaveAs(32) 兼容性真机通过。遗留：转换与缩略图批次共用单队列串行（大批量导图时首开延迟拉长，契约内代价）

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c6d4c27` | (see git log) |
| `b6a2852` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 132: 项目记忆迁移 docs/memory 并加自动加载 hook

**Date**: 2026-10-20
**Task**: 项目记忆迁移 docs/memory 并加自动加载 hook
**Branch**: `master`

### Summary

user 级自动记忆目录（~/.claude/projects/.../memory/）迁移至项目 docs/memory/（MEMORY.md 索引 + 2 条记忆，格式不变，diff 逐字节校验后删除源目录）；新增 .claude/hooks/inject-memory.py 经 SessionStart（startup/clear/compact）注入记忆索引与权威源声明（docs/memory 唯一权威源、新记忆写入此处、user 级残留先合并再消费），索引缺失/空文件静默降级 exit 0；settings.json 三 matcher 各追加条目；CLAUDE.md 新增「项目记忆」节 + README 文档表补行。hook 回归：正常注入、降级路径单测均通过。决策记录：机制选独立 hook（方案 B）、迁移方式 move、注入深度仅索引（与原机制一致防体积膨胀）

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `1ece1c4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 133: 素材管理文件夹树与拖拽整理

**Date**: 2026-10-20
**Task**: 素材管理文件夹树与拖拽整理
**Branch**: `master`

### Summary

素材面板新增可收缩文件夹树（真实目录口径）：递归过滤+徽标+排除/派生标记；目录管理（创建/删除/重命名，lost收口+排除双向翻转）；文件/目录级拖拽移动（每条目一事务+split_records重写+FTS/排除单点）；folder_meta标注（schema v7键级联）；folderPath检索AND维度+§8.7矩阵登记。trellis-check修P0（substr字节/字符偏移中文目录名键损坏）+P1（空目录键级联跳过）；3项P2行为收口（扫描失败降级/幻灯片视图重拉/no-op文案）。service覆盖率80.1%、前端2011用例全绿。待办：真机冒烟（拖拽/目录管理/文件夹标注/中文目录名）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f93d106` | (see git log) |
| `24fcf4e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 134: 素材清单刷新链统一化 + 文件夹树拖拽 E2E 补齐

**Date**: 2026-10-20
**Task**: 素材清单刷新链统一化 + 文件夹树拖拽 E2E 补齐
**Branch**: `master`

### Summary

素材面板数据变更刷新收敛单一入口 refreshDataAfterMutation（六链迁移：批量打标签/评分/标签管理/维护清理/文件与目录拖拽/保存标注清单段），slidesStale 惰性重拉消灭文件视图停留陈旧整类问题（三直赋消费点：切视图/filterByFile/locateCartEntry）；双路续拉兜底 + onFolderDrop 补 searchSeq 守卫 + onFolderDropFolder 纳入刷新；新增拖拽 E2E 三用例；修复 assets-export 既有失败用例（EP 2.14.5 确认型错误行为变化）；§8.7 契约 + CLAUDE.md 关键规则行；vitest 2016/E2E 71 全绿

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bb7a1fa` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 135: EP 2.14.5 确认型弹框回归排查 + drop-folder E2E 增补

**Date**: 2026-10-20
**Task**: EP 2.14.5 确认型弹框回归排查 + drop-folder E2E 增补
**Branch**: `master`

### Summary

全仓排查 ElMessageBox.confirm 消费链 48 处/24 组件：三形态分类学（递归恒弹/顺序两段/单次确认），0 处现存漂移（bb7a1fa 修复本质为 E2E 断言侧漂移）；Obsidian 顺序两段链三处守护用例补齐（times(2)+静默语义，check 阶段断言面拉齐 7 处）；目录级 drop-folder E2E（MoveAssetFolder 参数+前缀改写三态断言+刷新链）；双重刷新去重评估结论=前提失实（onScanDone 仅复位进度从不刷数据），方案零维持现状，修正 onFolderDropFolder 失实注释；沉淀 docs/spec/confirm-dialog.md（三形态契约+E2E 多实例选择器两范式+EP 再升级排查方法论）；回归 vitest 2022 passed / E2E 72 passed，后端零改动

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a9330d5` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 136: 前端依赖版本漂移治理（EP caret 漂移根源收口）

**Date**: 2026-10-21
**Task**: 前端依赖版本漂移治理（EP caret 漂移根源收口）
**Branch**: `master`

### Summary

39 顶层依赖四档钳制（exact element-plus 2.14.5 全家 / tilde 运行时 UI 面 28 项锁 minor / caret 工具链 8 项 / url xlsx），lock 仅 spec 元数据同步版本零变化；新增 scripts/check-dep-consistency.mjs 三维校验（档位登记×声明格式×lock 版本，未登记即拦）+ --selftest 11 用例 + ci.yml 步骤 2.5 快速失败；漂移注入实机自测三形态（exact 错位/tilde 越 minor/caret 越范围）全拦；SOP 沉淀 docs/spec/frontend-dep-upgrade.md 与 confirm-dialog.md 互链 + CLAUDE.md 关键规则行；vitest 2022 + E2E 75 全绿；trellis-check 3 findings 全修（spec 索引登记、0.0.x caret 语义、node 版本兼容）

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `00c332a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 137: goleak 硬门禁收口 + CI Linux 三问题修复（10-21-goleak-gate）

**Date**: 2026-10-21
**Task**: goleak 硬门禁收口 + CI Linux 三问题修复（10-21-goleak-gate）
**Branch**: `master`

### Summary

goleak 两步走第二步：观察模式 26 天实为未观察（告警不改退出码+Linux 从未跑硬门禁）。WSL Ubuntu 复现 CI Linux 三问题全修——thumbQueue 8 实例泄漏（测试收口缺口，Cleanup 改 WaitThumbsDone 全链）、watchProcess 驻留（生产生命周期缺口，per-session watchDones+watchExits 双 chan 重构，trellis-check 审出共享 watchWG 多会话死锁 P0 后二次修复+补多会话回归用例）、rg stub 平台分流遗漏（Unix 分支补探针）、split 断言锚点时序脆弱（前移至确定性同步点）。TestMain 两档收紧（默认+深跑统一 VerifyTestMain 空白名单）+注入自证验拦。WSL Linux+Windows 双侧全量绿，文档三处同步（perf-baseline/CLAUDE.md v2.28/文件头）。镜像仓 CI 滞后与 security job 红点不在本任务范围

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `de14d99` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 138: tilde 档上游走查升级 7 项（10-21-tilde-upgrade）

**Date**: 2026-10-21
**Task**: tilde 档上游走查升级 7 项（10-21-tilde-upgrade）
**Branch**: `master`

### Summary

28 项 tilde 依赖上游盘点（官方 registry）：升级 7 项一次合并（patch 5 声明不动 npm update + minor 2 显式改声明，升前 changelog 逐项核对无 breaking——pinia 4.1.0 getStores 可选 API、@codemirror/language 6.13 StreamParser 拆包向后兼容带新传递依赖 streamparser）；跳过留痕 5 项 major/0.x-minor（js-yaml 5/markdown-it 15/mermaid 12/vue-router 5/docx-preview 0.4）；必跑面全项：一致性绿+vitest 2022+E2E 72+3+哨兵 32+audit 7→6 无新增 high；trellis-check 独立复跑全部必跑面证实+2 处 research 留痕精度修正。批次进度 hardening-batch [2/3]

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `254a90c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 139: E2E 五高价值链路补齐 + hardening-batch 批次收口（10-21-e2e-coverage）

**Date**: 2026-10-21
**Task**: E2E 五高价值链路补齐 + hardening-batch 批次收口（10-21-e2e-coverage）
**Branch**: `master`

### Summary

缺口矩阵五域盘点（research/coverage-matrix.md：17 spec/72 用例现状、66 绑定方法无 mock 缺省、改动频次 Top3）选 Top 5 链路落地 15 用例（E2E 72→87）：命令面板批量重试编排/终端多 tab/工具箱 Panel 化/购物栏事件链/设置保存链。mock 基建：__deferred__ 挂起解析器描述符 + EventsEmit 同步广播 + 终端 5 方法缺省补课。真缺陷修复：AssetsPanel.locateCartEntry 消费后端不存在的 filePath 字段（fixture 形状漂移掩盖，溯源恒 miss）——cartEntryRelPath 单点修复 + fixture 对齐后端 model。vitest 2022 + E2E 87 全绿。文档三处沉淀（e2e-testing §6 两范式/测试策略 22 spec 清单/CLAUDE.md 规则行）。hardening-batch 三子任务全归档（goleak-gate→tilde-upgrade→e2e-coverage），父任务同步归档收口

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3065efd` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 140: CI push 验证闭环 + 镜像仓同步规程

**Date**: 2026-10-21
**Task**: CI push 验证闭环 + 镜像仓同步规程
**Branch**: `master`

### Summary

29 提交推 gitee 并实证服务端自动镜像至 GitHub（非手动）；push 前 Windows/WSL 双侧 + vitest + E2E web/desktop 组合态自检绿（3 处并发负载偶发单跑定性）。CI 四轮闭环：run#13 暴露 govulncheck 11 标准库可达漏洞（toolchain 升 go1.26.10 + x/net v0.60.0 修复），run#14 分层暴露前端 3 high（npm audit fix lock-only 修复），run#15/16 test+security 双 job 全绿（2.5 首跑绿 + goleak Linux 首跑绿 + run 11/12 旧红点收口）。规程沉淀贡献指南 5.3（同步机制/三步验证/红点处置/责任口）。残留：KaTeX 2 low 归 mermaid-12 专项；GO-2026-5932 接受风险登记 security-scan.md §5.3。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `57f17d6` | (see git log) |
| `5cd6b65` | (see git log) |
| `12e7299` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 141: mermaid 12.2.0 升级清零 katex 传递链漏洞

**Date**: 2026-10-21
**Task**: mermaid 12.2.0 升级清零 katex 传递链漏洞
**Branch**: `master`

### Summary

mermaid ~11.17.2 → ~12.2.0（lock 落 12.2.0，传递 katex 0.18.11 出 GHSA-238p-pmpm-9mq7 漏洞区间，npm audit 2 low → 0 清零）；FilePreviewRenderer initialize 加视觉保持双键 layout:'dagre'+look:'classic' 钉回 v11 观感（v12 默认 ELK/redux-color+neo）；CI node 20→22 四处对齐 engines >=22.12；新增 E2E 冒烟 markdown-mermaid.spec.js 三类型图真实 SVG 断言；沉淀 docs/spec/mermaid-rendering.md + security-scan.md §5.2 基线 + 根 README Node 版本；回归全绿（一致性 selftest / vitest 2022 / E2E web 88 / desktop 3 / npm ci / build），镜像仓 CI run#18 test+security 双 job success

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `098a3c1` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 142: E2E 第二批 12 条落地 + wails-init 事件派发三硬化

**Date**: 2026-10-21
**Task**: E2E 第二批 12 条落地 + wails-init 事件派发三硬化
**Branch**: `master`

### Summary

矩阵候选 #8-#20 时效复核（13 项全未变）后第二批落地 12 条：看板钉选/终端全屏 ESC 过滤/facet 筛选参数换算/排序透传/批量评分/幻灯片标注双代次守卫/缩略图补缺恰一轮/标签树拖拽回滚/保存检索 upsert 异库重映射/端口一览行级防重/AI 选择题卡片/PPT 拆分批量链；wails-init 三硬化收口（dispatch 快照正序 per-handler try/catch、EventsOn/OnMultiple/Once 返回注销闭包对齐真实 runtime、once 自移除）+ wails-mock.spec.js 3 条自证；两遗留面均实施收口（EventsOn 闭包契约 / KillProcess 强杀链）；GetRepoFilterList mock 缺省补齐；web 通道 87→107 用例全绿，vitest 2022 零回归；文档同步四处（测试策略 32 spec/107、e2e-testing §6、归档矩阵 §4.2、AssetFilterPopover 注释纠偏）；镜像 CI run#20 test+security 双绿

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bf5b2f7` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 143: deps-upgrade-batch3 前端四 major 出清

**Date**: 2026-10-21
**Task**: deps-upgrade-batch3 前端四 major 出清
**Branch**: `master`

### Summary

vue-router ~5.4.0 / js-yaml ~5.5.0 / markdown-it ~15.0.3 / docx-preview ~0.4.1 全出清；唯一代码适配 = js-yaml 2 处 namespace import（v5 无 ESM default export）；回归全绿（一致性+selftest / vitest 2022 / E2E web 107 + desktop 3 / audit 0 / build）；4 份 breaking research 落任务目录；沉淀 frontend-dep-upgrade.md §3.6 + security-scan.md §5.4 出清记录 + CLAUDE.md 关键规则与技术栈勘误；遗留人工项 = docx 真机目检（多页/表格/页眉页脚）

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `9b80f8f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 144: go-git v5 依赖出清（消依赖路线）

**Date**: 2026-10-21
**Task**: go-git v5 依赖出清（消依赖路线）
**Branch**: `master`

### Summary

go-git v6 升级评估胜出消依赖路线：整体移除 go-git/v5，remote 探测改纯 Go 解析 .git/config（含 worktree gitdir 重定向），HEAD 元数据改 git symbolic-ref/rev-parse（ResolveHead）。util 层 exec 封装语义零变更，回归全绿，govulncheck 清零，沉淀 go-git-deprecation.md，security-scan §5.4 收尾仅剩 wails v3 跟踪项。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ddb6bfe` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 145: v1.10.0 发版收口：RPC 冒烟 35 项 + UI 手点三问题修复 + 发版文档

**Date**: 2026-10-22
**Task**: v1.10.0 发版收口：RPC 冒烟 35 项 + UI 手点三问题修复 + 发版文档
**Branch**: `master`

### Summary

v1.10.0 发版收口（v1.9.0→HEAD 180 commits）：RPC 驱动冒烟 35/35 PASS（serve 无头 /api/rpc，git 全功能 18 项 go-git 出清无回归/PPT 拆分合并购物栏 9 项/COM 转 PDF），留痕归档任务目录 smoke/；UI 手点 28/31 过，三问题当版修复——P1 PdfPath 相对路径契约漂移（service 返回点 absCachePath 归一+测试）、A2 弹窗聚焦（首修 @open 被 EP focus-trap 覆盖真机暴露，改挂 @opened——jsdom 无 focus-trap 测不出的坑留痕）、A9 文件夹标签级联检索（文件有效标签含归属目录及后代 folder_tags 继承，两检索路径同源谓词单点 substr 前缀防通配符，性能实测万级 2.6ms）；B2 拆分面板补打开输出文件夹入口。门禁全绿：go test+integration+vitest 2027+E2E+govulncheck 0+npm audit 0。CHANGELOG [1.10.0] 补全（54 条功能提交核验，纠 1 行跨版错登）+ 路线图里程碑表补 v1.9.0/v1.10.0。check 代理自查引入的 listTagFilteredFiles 参数错位 P0 被既有 facet 用例兜底暴露即修。发版走 release.sh v1.10.0 tag。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `238642c` | (see git log) |
| `27e8798` | (see git log) |
| `9d701c9` | (see git log) |
| `cfe4e2c` | (see git log) |
| `1f97e20` | (see git log) |
| `fe25132` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 146: 素材与预览遗留收口：聚合子页分页 + PPT 预览 STA 插队

**Date**: 2026-10-22
**Task**: 素材与预览遗留收口：聚合子页分页 + PPT 预览 STA 插队
**Branch**: `master`

### Summary

子任务 1/3（legacy-trio-cleanup）完成。R1 聚合展开子页不全修复：AssetSearchOptions.FilePath 汇聚层单点过滤（三路径一致，先于排序/聚合/分页）+ toggleAgg 单文件窗口与加载更多分页续拉，双代次守卫增强不削弱，check 抓 P1 保存链竞态（清空后在途响应复活旧缓存）已修并有回归用例。R2 PPT 预览 STA 队列 urgent 通道插队：urgentCh + submitTo/receiveNextTask 双单点，单 worker 契约不破，ConvertToPdf 走 urgent，优先级单测 -count=10 稳定。原第三项 clipboard vet 告警经查已被 0a2040d 修复，从 PRD 移除。spec 沉淀：assets-management.md FilePath 过滤节、assets-thumbs-com.md urgent 通道契约。验证：vet 零告警 / go test 全绿 / vitest 2030 用例 / build 过 / service 覆盖率 80.4%。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `32df32d` | (see git log) |
| `008d583` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 147: 浏览器断线重连续传（10-22-ws-reconnect-resume）

**Date**: 2026-10-22
**Task**: 浏览器断线重连续传（10-22-ws-reconnect-resume）
**Branch**: `master`

### Summary

WSHub 事件帧加全局单调 seq + 2000 帧环形缓冲，readPump 解析 resume 握手按 lastSeq 精确补发，缓冲溢出与 hub 重建两形态发 ws-gap 断层帧；前端 transport 记 lastSeq、重连 resume、ws-synced 同步模式按 seq 升序去重落地，connBanner 断层横幅。后端 -race 8 新用例 + 前端 7 新用例全绿。沉淀 browser-channel.md §2.4、CLAUDE.md 关键规则 v2.29、功能说明 §11.4。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `2c8e5f8` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 148: Agent 交互式多轮会话（持续会话进程模型）

**Date**: 2026-10-22
**Task**: Agent 交互式多轮会话（持续会话进程模型）
**Branch**: `master`

### Summary

AI 对话执行模型从每轮一进程重构为每会话常驻 claude 进程（stdin 流式注入）：运行中插话（Go 侧 FIFO 排队注入，E_CHAT_IN_PROGRESS 退役）、权限审批（control_request 协议+chat-approval 事件+前端弹窗 FIFO 应答，--permission-prompt-tool stdio 漏配守护）、生命周期（watchdog 轮超时 interrupt/空闲回收/参数变更优雅重启/respawn 自动 --resume/进程槽位上限 4）、多会话并行原生隔离+删除会话进程回收。后端全量+race 全绿、vitest 2041、ai-chat E2E 8/8。trellis-check 修复同轮双 finalizer 竞态（finished 认领字段）。spec 沉淀 ai-chat-service.md 重写+browser-channel.md WS 上行边界。遗留：真机冒烟 2026-10-23（记忆 chat-interactive-smoke-pending：审批弹窗/插话/中断后续聊三核心），父批次 10-22-experience-browser-batch 待冒烟通过后归档。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `5b00149` | (see git log) |
| `46b0cd5` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 149: 浏览器模式三期：多 token 管理+认证审计、内置 TLS、多设备连接语义明示

**Date**: 2026-10-23
**Task**: 浏览器模式三期：多 token 管理+认证审计、内置 TLS、多设备连接语义明示
**Branch**: `master`

### Summary

三期批次交付（任务 10-22-tls-token，PRD 四项 ADR 决策：共享广播语义明示/审计仅认证事件级/自签+用户证书双形态/永久默认+短命档）。PR1 认证链 verify 化（TokenVerifier 注入，SetToken/RotateToken 删除）+ 多 token SHA-256 哈希落盘（存量 web_token 迁移）+ 审计 JSONL 5MB 轮转；PR2 内置 TLS（tls.NewListener 钉 ALPN http/1.1、幂等键 listen+TLS、证书加载前置无半死态、自签 ECDSA P-256 fail-fast）；PR3 ws-connections:changed 连接数事件（mu 写锁外 Emit）+ 前端令牌管理/TLS 设置/多设备提示。trellis-check 修复 1 处审计顺序缺陷（RevokeAllExcept 落盘失败不得留已发审计行）。测试全绿（go test+race / vitest 2069 / Playwright 113）。遗留：真机冒烟（TLS 浏览器例外+token 轮换重验证）连同 chat-interactive-smoke-pending 欠账一起做。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ec50aa4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 150: 冒烟缺陷修复：chat 草稿会话隔离 + 审批模式显式传参

**Date**: 2026-10-23
**Task**: 冒烟缺陷修复：chat 草稿会话隔离 + 审批模式显式传参
**Branch**: `master`

### Summary

A 组真机冒烟两条缺陷修复（任务 10-23-chat-webserve）：① AI 对话输入框单 ref 改按会话草稿 Map（切换存旧载新/发送清键/空串键覆盖无会话态）；② 审批弹窗未生效根因=用户全局 ~/.claude/settings.json defaultMode:auto（classifier 不发 can_use_tool）+ buildChatArgs 默认模式省略参数继承之——改为恒传 --permission-mode（default 显式+空串归一），同源消除插话挂起 classifier 依赖。trellis-check 修 3 处（冗余 watch/2 处断言区分力）。B 组浏览器安全三期冒烟除桌面多设备提示（设计预期）全过。遗留复测：A1 审批弹窗/A2 插话链路；上一轮反馈 webServe 开关不启动未在本轮复现，复现再立项。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `2feb5ee` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 151: 桌面端多设备连接指示：webserve-connections:changed 桌面事件出口

**Date**: 2026-10-23
**Task**: 桌面端多设备连接指示：webserve-connections:changed 桌面事件出口
**Branch**: `master`

### Summary

浏览器连接数补到桌面 UI（任务 10-23-desktop-device-connection-indicator）：hub 加 SetOnCountChange 纯回调 func(int)（server 包不知 Wails，register/unregister 出 mu 写锁后 !closing 触发且 count=0 透传，WS 广播侧抑制态逐字节不变）；manager startLocked 注入闭包经 Wails 出口发桌面事件 webserve-connections:changed，stopLocked 在 applySink(nil) 后显式补发 count=0 归零；前端 useDeviceConnections 双通道消费（isDesktopChannel 分流桌面 >=1/浏览器 >1，桌面接受 0），ActivityBar/TerminalPanel 零改动，Wails 绑定零新增。ADR：桌面专用事件名与浏览器通道同义不同名刻意分叉（同名致桌面 1 浏览器连接 off-by-one + 前端 n>=1 校验吞 0）。测试全绿（go test 全量 + race + server 覆盖率 90.8% / vitest 2075），check 修 2 处 gofmt；spec 沉淀 browser-channel.md §2.3 桌面出口段 + CLAUDE.md 关键规则行。真机冒烟通过（连接/断开/停机三态指示变化），遗留清零。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `226cdac` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 152: 多设备连接明细（连接数升级为设备明细列表）

**Date**: 2026-10-23
**Task**: 多设备连接明细（连接数升级为设备明细列表）
**Branch**: `master`

### Summary

桌面事件 webserve-connections:changed 载荷升级为 ConnDetail 明细数组（逐连接快照，count=len 派生）：server 包 ConnDetail 元数据采集（ServeHTTP 升级时 ID/IP/tokenID/接入时间）+ SetOnConnectionsChange(func([]ConnDetail)) 全量快照回调（快照 mu 写锁内构造出锁后触发，锁序红线保持）+ 装配层 webTokenNamedDetails 令牌名映射（miss 空串降级）；浏览器通道载荷恒为 count 数字（隐私红线）；前端 useDeviceConnections 明细 ref + ActivityBar tooltip+popover 双层明细面板（EP popover 套 tooltip 嵌套失效实证：forwardRef 落多根 fragment，改 tooltip 外层+popover 直挂原生 div）；.gitignore 补 web_tokens.json/web_audit.log；browser-channel.md §2.3 重写 + CLAUDE.md 规则行同步；go 全量/-race/前端 2079 用例/E2E 全绿，真机验收通过

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `d298597` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 153: 连接管控：踢出连接 + 令牌在线状态标记

**Date**: 2026-10-24
**Task**: 连接管控：踢出连接 + 令牌在线状态标记
**Branch**: `master`

### Summary

桌面明细面板踢出单个连接（WSHub.DisconnectClient 对齐 Close 只关 conn 模式，零新增事件幂等；KickConnection 绑定停机/miss 两错误码）+ 令牌列表在线连接数（App 层明细缓存回调写入/停机显式清零，ListWebTokens 返 WebTokenStatus）。踢出非封禁与撤令牌互补；浏览器 UI 零改动 RPC 面同构不特判。go test 含 -race 全绿，vitest 2084 例全过，E2E 回归 2/2；spec 沉淀 browser-channel.md §2.3 连接管控段 + CLAUDE.md v2.30。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ce7fe94` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 154: 连接管控二期：踢出审计 + 被踢端感知提示

**Date**: 2026-10-24
**Task**: 连接管控二期：踢出审计 + 被踢端感知提示
**Branch**: `master`

### Summary

踢出动作审计留痕（conn_kick 事件，App 层单咽喉，DisconnectClient 返回 (ConnDetail,bool)）+ 被踢端感知（hub 首个单播面：ws-kicked 帧 + nil 哨兵经 send 队列 writePump flush 后自关，禁进环形缓冲防 resume 幻影；队列满降级直接 Close，best-effort）+ 前端 connBanner 被踢态（帧到达零启发式判定、消费一次、重连清除）。go test -race 全绿 + vitest 2089 用例全绿；spec 更新 browser-channel.md/logging-and-errors.md §8/README/CLAUDE.md v2.31

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `5024ca4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 155: 连接管控三期（审计可见性：actor 归因 + 审计历史查看）

**Date**: 2026-10-24
**Task**: 连接管控三期（审计可见性：actor 归因 + 审计历史查看）
**Branch**: `master`

### Summary

补全二期两处审计可见性缺口：①actor 归因——requireToken 注入 tokenID 进 ctx + RPCTargetForActor per-request 装饰 target（main 包 rpcAuditTarget embed override KickConnection 委托未导出 kickConnectionAs，导出面/wailsjs/RPC 协议零变更），AuditRecord 加 actorSource（desktop|rpc）+ actorTokenId，LogConnKick 收编 KickAudit 结构化参数；②审计历史——AuditLogger.QueryAuditLog 读侧（过滤/倒序分页/.1+主文件合并/坏行跳过/快照语义）+ App.QueryAuditLog 绑定 + 设置页审计日志弹窗（AuditHistoryDialog）。隐私红线：RPCHiddenMethods 屏蔽（对浏览器呈现「方法不存在」，同构暴露不特判原则首个有据偏离）+ 前端 isBrowserChannel 入口隐藏双防线。check 自修 8 处（含 handleError 双参签名真实缺陷）。验证：go build/vet/-race 全绿、vitest 2098、E2E 114、npm run build 过、写侧契约现有测试零改动通过。spec：browser-channel.md §2.2/§2.3/§6 + logging-and-errors.md §8.1 + CLAUDE.md 关键规则行 + README/功能说明/API参考联动。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `555489c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 156: 连接管控四期：临时封禁（踢出防重连 + 封禁管理）

**Date**: 2026-10-24
**Task**: 连接管控四期：临时封禁（踢出防重连 + 封禁管理）
**Branch**: `master`

### Summary

补齐连接管控时限性拒连档位：封禁状态机（tokenID 维度，data/web_bans.json 持久化，永久+懒过期双形态，撤销联动清理，幂等对齐 Revoke）+ BanCheck 独立注入面两认证点接入（HTTP requireToken + WS 升级前，verify 契约不动）+ ban/unban 审计 actor 归因（rpcAuditTarget 扩 override）+ 前端共享 BanTokenDialog（SettingsPanel/ActivityBar 两入口）+ tokenBan.js 联动踢出 fail-soft 编排；服务端语义恒仅拒新连（对齐撤销热生效语义）。brainstorm 定稿四决策（D1 在途连接/D2 持久化/D3 条目内嵌宿主/D4 组合判定）。测试：go test -race 三包全绿 + vitest 2121 + E2E 封禁链路；两轮 trellis-check（PR1 零缺陷、PR2 修 1 处文档表述）。spec 沉淀：browser-channel.md §2.2/§2.3/§3/§4/§6 + logging-and-errors.md §4.3/§8 + CLAUDE.md 关键规则 v2.32

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `4fab5c8` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 157: 连接管控五期：认证失败自动封禁

**Date**: 2026-10-26
**Task**: 连接管控五期：认证失败自动封禁
**Branch**: `master`

### Summary

IP 认证失败自动封禁落地：service.IPBanService 内存态状态机（滑动窗口 10min≥10 次封 5min 固定、封禁中不累计、期满重计、回环豁免、WS/HTTP 单例合并计数、单调时钟防回拨、重启清零）；server.IPBanCheck 注入 verify 前两认证点同接（命中不 verify/不发认证事件/不计数）；装配层 onAuth 闭包计数驱动 + 轮换/撤销联动 ClearAll；审计 ip_ban(system 归因+failCount)/ip_unban（实际解除才写），App.UnbanIP 归因分叉 desktop/rpc（rpcAuditTarget 装饰器先例补齐 check 缺口）；webserve-ip-banned 桌面专用事件 + ListIPBans 进 RPCHiddenMethods（他人 IP 不进浏览器通道）；前端 ActivityBar 自动封禁区块 + 全局 toast + 审计历史过滤。check 修复：README 缺描述、AssetsPanel flaky 用例改手动解析器、文档载荷描述漂移。spec：browser-channel.md §2.2/§2.3/§3/§4/§6 + logging-and-errors.md §8 + CLAUDE.md 关键规则行 v2.33。验证：go test 三包 -race 全绿、vitest 2144 全绿、build/generate 零 diff、E2E ip-ban 2 例过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c257fc2` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 158: 连接管控六期:自动封禁参数化+递增封禁

**Date**: 2026-10-27
**Task**: 连接管控六期:自动封禁参数化+递增封禁
**Branch**: `master`

### Summary

IP 自动封禁治理增强落地:AppSettings 扩 IPBan 四参数段(threshold 3-100/window 1-120/duration 1-60/maxBan 5-1440,段缺失全默认,normalize 收敛封顶≥基础时长,Ensure 双链路现四处);热更走 SetIPBanConfig「保存+生效」显式单点+启动初值同口 ApplyConfig 注入,ApplyConfig 原子换参不动 entries(生效中封禁不追溯/在途计数不清/threshold 改小自然收敛);递增封禁 escalateBanDuration base×2^(n-1) 封顶(循环翻倍禁 Pow),strikes 保留至 UnbanIP/ClearAll/重启、RecordSuccess 不清;AuditRecord/IPBanStatus 扩 strike,ip_ban reason 档位化;SettingsPanel 四参表单(:value-on-clear+回读归一值+恢复默认),ActivityBar strike>1 徽标;go test 全量+race 绿/vitest 2150 绿/build 过/E2E 过,trellis-check 七项全过;spec 更新 browser-channel §2.3 六期段+logging-and-errors §8+CLAUDE.md v2.34(含时钟注入闭包禁内联持锁方法调用死锁教训)

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `542f7a1` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 159: 连接管控七期：IP 手动封禁+可信网段豁免

**Date**: 2026-10-27
**Task**: 连接管控七期：IP 手动封禁+可信网段豁免
**Branch**: `master`

### Summary

IP 手动封禁与 CIDR 白名单豁免落地：IPBanService.Ban 手动封禁入口（同 map 写入即生效 server 零改动、覆盖自动封禁置 source=manual、failures/strikes 不动、expiresAt 0=永久 banActive 单点）；手动条目持久化 data/ip_bans.json（SetBansPath 注入即时载入仅未过期条目、manualVersion 版本复核重写收敛、落盘失败字段级回滚不写审计、ClearAll 不落盘）；IPBanSettings.Whitelist（normalize 剔除非法保留原串恒非 nil）+ isWhitelistedIP 判定单点（回环收编/裸 IP 补 /32 //128 双栈/ApplyConfig 预解析）三处消费（RecordFailure 早退/RejectMessage 追溯放行/Ban 拒绝）；SetIPBanConfig 扩第五参；BanIP 绑定+rpcAuditTarget 归因分叉；审计 ip_ban 单事件复用（LogIPBan 补 ActorTokenID）；两新错误码三处同步；前端 BanIPDialog+ActivityBar「IP 封禁」区块改造+SettingsPanel 白名单编辑器+utils/ipWhitelist.js 前端同口径复刻；测试 service v7 18 用例+App 4 用例+vitest 2182 绿+E2E 手动封禁链路+全量 race 绿；spec browser-channel/logging-and-errors/CLAUDE.md v2.35/功能说明/README 五文档更新。两教训：ipWhitelist IPv6 压缩段 tail 字节取错区间（vitest 暴露）；E2E 桌面事件须晚于组件挂载派发（早于 EventsOn 注册永久丢失）

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `8434dc0` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 160: 连接管控八期：CIDR 网段手动封禁

**Date**: 2026-10-27
**Task**: 连接管控八期：CIDR 网段手动封禁
**Branch**: `master`

### Summary

七期 Out of Scope「CIDR 封禁网段」转正：IPBanService.cidrBans 独立列表（禁混 entries map）+ RejectMessage 判定链三级（白名单→精确→网段，RecordFailure 零改动——网段内 IP 照常计数与白名单「信任不计数」刻意分叉）+ BanCIDR/UnbanCIDR 入口（解析复用 parseIPBanWhitelistEntry 单点；白名单相交放行仅完全覆盖拒绝，单条目口径拼片放行为有意取舍）+ 持久化 data/ip_bans.json v2（Kind 字段 v1 零特判兼容）+ IPBanStatus.Kind/CoveredBy（后端计算透传，掩码方向与文案判定刻意相反）+ 两新 warning 错误码 + 前端 BanIPDialog 档位切换/白名单预检三态/全网段应急锁文案 + ActivityBar 徽标与解封分流 + parseIPv6('::') 放宽。双实现 agent 并行 + trellis-check 修 3 处（CoveredBy 掩码方向反/错误码级别误登/注释口径），go test 全量绿 vitest 2206 绿七期零回归，spec 三文档（browser-channel/logging-and-errors/CLAUDE.md v2.36）已沉淀。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `19a67cf` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 161: 连接管控九期：攻击源网段聚合建议

**Date**: 2026-10-28
**Task**: 连接管控九期：攻击源网段聚合建议
**Branch**: `master`

### Summary

七决策点 brainstorm 收口（固定粒度 /24 /64 + 独立查询面 + entries 内存态口径 + ActivityBar 建议行 + IPBanSettings 扩阈值 + RPCHiddenMethods 整体隐藏 + initialCidr 预填）。三 PR 实施：后端聚合纯函数+查询面 16 用例、SetIPBanConfig 六参+绑定+SettingsPanel、前端建议小节+BanIPDialog 分流预填 16 用例+E2E。trellis-check 修 6 处（含双非空 initialIP 优先级 + EventsOn mock 多监听器分发）。spec 沉淀 browser-channel.md 九期段 + CLAUDE.md v2.37。全量测试绿（前端 2224 用例 + race + build），八期零回归。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `47484fc` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 162: 连接管控十期：攻击源趋势面（审计历史聚合）

**Date**: 2026-10-28
**Task**: 连接管控十期：攻击源趋势面（审计历史聚合）
**Branch**: `master`

### Summary

落地连接管控十期：基于审计行 ip_ban/ip_unban 的跨重启攻击源时序视图。读侧懒扫 readSnapshot 单点（否决聚合快照第二数据源）；聚合口径写死 doc comment（auto 主计数/manual 分列、单 IP 掩 /24·/64、CIDR 原串独立桶、CoveredBy 不剔除语义分叉、rebounded 桶级观测语义）；TrendWindowDays 扩参默认 7 clamp 1-30（SetIPBanConfig 六扩七参 12 调用点）；QueryIPBanTrend 进 RPCHiddenMethods 载荷桶粒度封顶；前端 useIPBanTrend 三件套 + ActivityBar 第四区块（迷你条形/双徽标/行点击复用九期预填链）。附带修复 StopClosesWSClients 既有 flaky（广播帧滞留缓冲与 Close 竞速，断言改循环读至终态）。service 覆盖率 81.4%、vitest 2238 全绿、E2E 5/5。spec 沉淀 browser-channel.md 十期段 + CLAUDE.md v2.38 + README/功能说明补记。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `467760d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 163: 连接管控十一期：审计留存多档轮转链

**Date**: 2026-10-28
**Task**: 连接管控十一期：审计留存多档轮转链
**Branch**: `master`

### Summary

审计轮转链 5MB×2 扩固定 6 档（默认 35MB，clamp 1-20MB×2-10 档上限 220MB）：rotateLocked 链式 rename 失败停链自愈、readSnapshot 锁内快照 depth 多档合并，QueryIPBanTrend/QueryAuditLog 零改动透明受益；AuditSettings 段 + EnsureAuditDefaults（现五处 Ensure）+ SetAuditConfig 保存即生效 + SettingsPanel 审计段；竞速实测 Windows 读句柄 sharing violation 停链 / Unix 不受阻，断言分侧 _windows/_linux 双文件规避 ubuntu CI 必挂；淘汰静默 earliestTs 自然前移，十期前端零改动。无新错误码。spec 沉淀 logging-and-errors.md §8.2 + CLAUDE.md v2.39 + API参考/功能说明/部署说明。后端三包+前端 2244 全绿。遗留登记：GetSettings 降级分支缺 IPBan/Audit 段补全（六期起遗留）、SetIPBanConfig 漏拷 Audit 段（既有形态）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e8e8156` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 164: 连接管控遗留收口：设置段 Ensure 与指针段拷贝收敛双单点

**Date**: 2026-10-28
**Task**: 连接管控遗留收口：设置段 Ensure 与指针段拷贝收敛双单点
**Branch**: `master`

### Summary

收口三处设置域遗留（GetSettings 降级分支缺 IPBan/Audit 两段、SetIPBanConfig 漏拷 Audit、盘点新增 SetWebServeConfig 漏拷 IPBan/Audit），抽 AppSettings.EnsureAllDefaults（三消费点）与 AppSettings.PrivateCopy（三写入口）双单点，新增设置段收敛为两单点各加一行；model/主包新增断言含变异验证；spec 落 cross-layer-contracts.md 设置段增删 checklist Scenario；CLAUDE.md v2.40 规则行精简；六~十一期测试零回归

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `402f69a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete

---

## 2026-10-28 CLAUDE.md 关键规则表瘦身：详情下沉与钩子化（10-28-claude-md）

### 收益度量（瘦身前后对比）

| 指标 | 瘦身前 | 瘦身后 | 变化 |
|---|---|---|---|
| 关键规则表数据行 | 68 | 60 | -8（拆 2 行 + 合并连接管控 11 行为 1） |
| 关键规则表字符数 | 49,191 | 15,984 | **-67%** |
| CLAUDE.md 全文件字节 | 87,340 | 29,438 | **-66%** |
| 超 350 字符行 | 22 行（最长 2154） | 0 | 全表达标 |
| ≤250 字符行 | 少数 | 26 | 多数行进入目标档 |

### 关键动作

1. 承接核对：逐行提取反引号内特征标识符 grep 目标文档，62 行承接完好（写法差异甄别后），6 处真缺（testutil 全列/session.json 快照/纯 prompt 模式/FileDiffDialog 释放/Git 取消 util API/统计缓存无承接行）先补文档再瘦身
2. docs 补齐：logging-and-errors.md §9（Git 取消链路 util API 面）、ai-structured-output.md §纯 prompt、cross-layer-contracts.md 快照 Scenario、perf-baseline.md §16.5/§17/§18 三节
3. 连接管控 t57-t67（安全三期+一~十期 11 行）合并为 1 条 C 档总行（6 个跨期通用钩子 + browser-channel.md 分期锚点）；审计留存十一期独立行
4. 顺带修复：原表 2 处未转义 `||`（slideNum||0 / multiDevice||ipBans.length>0）致表格多切列
5. docs/spec/README.md 索引补 4 缺失条目（ai-chat-service/assets-thumbs-com/linux-platform/pptx-preview）+ 更新 3 条摘要
6. 零信息丢失对照表：`.trellis/tasks/10-28-claude-md/research/migration-map.md`（随任务归档）
7. CLAUDE.md 版本 v2.40 → v2.41

### 经验

* 瘦身口径执行中细化：行内钩子 = 触发回忆的路标（规则名+单点/禁令+新增指引），非操作手册；函数全名与细节让文档承载。Q1 分档上限按整行字符数执行（含文档列 ~50 字符链接开销），硬顶 350
* python 替换表格行时索引偏移踩坑一次（数据行 N 对应 lines[N+1]），靠四压版快照恢复


## Session 165: CLAUDE.md 关键规则表瘦身：详情下沉与钩子化

**Date**: 2026-10-28
**Task**: CLAUDE.md 关键规则表瘦身：详情下沉与钩子化
**Branch**: `master`

### Summary

规则表 68 行 49,191 字符瘦身为 60 行 15,984 字符（-67%），全文件 -66%，版本 v2.41；承接核对 62 行完好 + 6 处真缺先补 docs/spec（logging §9 Git 取消 util API/纯 prompt 模式/快照 Scenario/perf §16.5-18）再瘦身；连接管控 11 行合并总行 + 分期锚点；README 索引补 4 缺失；零信息丢失对照表 migration-map.md 随任务归档

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `cd5e7e2` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 166: 规范沉淀口径对齐：两档钩子级口径固化进沉淀流程

**Date**: 2026-10-29
**Task**: 规范沉淀口径对齐：两档钩子级口径固化进沉淀流程
**Branch**: `master`

### Summary

对齐瘦身后两处旧口径自述（CLAUDE.md 规范沉淀规则节 + docs/spec/README 两级分流表）为两档钩子级口径（普通行 ≤250 / 钩子密集行(≥7 钩子)与合并行 ≤350、硬顶 350）；超限当场下沉禁先落行后治理；finish-work Step 3 归档前加分档自检；workflow.md Phase 3.3 第三处旧口径同步指向 CLAUDE.md 唯一权威。静态导流抽查 hook-audit.py：钩子最多 5 行全钩子可定位承接；60 行实测 26 普通+34 密集+0 超顶。附带写法漂移两处对齐（truncation-hint-node class 补入 perf-baseline §15.6、E2E 行 {test,expect} 空格对齐）。CLAUDE.md v2.42

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `851c4e8` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
