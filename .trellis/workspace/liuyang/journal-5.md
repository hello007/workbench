# Journal - liuyang (Part 5)

> Continuation from `journal-4.md` (archived at ~2000 lines)
> Started: 2026-11-06

---



## Session 228: P2 聚合视图契约与产品决策落地

**Date**: 2026-11-06
**Task**: P2 聚合视图契约与产品决策落地
**Branch**: `master`

### Summary

落地 ②Stat 失败补 Logger().Warn（path+err，临界区外写日志，不做 IsNotExist 分流、不新增 MissingReason）；④选择工作目录=退出聚合（组合收口在 Home.onDirectorySelect：序号守卫+在途不保存+选中提前写入合并重建，命令面板跳转改 await）；⑤失效收藏提示收口 MISSING_FAVORITE_HINT「收藏项已失效或不可访问，可右键移除收藏」，占位保留可右键移除、不加 Favorite.Type。同步 aggregated-workspace.md v3、AGENTS 规则行、README，销账三条待决项；新增后端 1 例 + 前端 7 例。门禁：go vet、go test service/-short、go test .-short、npm test(2631)、npm run build 全绿。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `2c1f27d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 229: 素材侧边栏体验与索引可用性修复

**Date**: 2026-11-06
**Task**: 素材侧边栏体验与索引可用性修复
**Branch**: `master`

### Summary

完成素材管理侧边栏 6 项问题修复：内置忽略 Office 锁文件（~$）并清理存量索引行；解析失败原因随 schema v12 落库并在统计行提供可点清单与复制；文件夹树新增「在资源管理器中打开」与右键菜单（命令与 ⋯ 同源）、修复折叠箭头收起回弹（EP node-collapse 早于 expanded 落值，改 nextTick 回收）、素材面板 Ctrl+V 不再串台；文件视图新增单文件删除（磁盘删 + 索引标 lost，标注保留）。沉淀 docs/spec/assets-management.md §16 与 el-tree-expand-state.md §5，AGENTS.md 关键规则补钩子；service 覆盖率 80.9%、前端 2643 用例全过、构建通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ddd0807` | (see git log) |
| `3125cbe` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 230: 素材小图缩略图分辨率提升至 640

**Date**: 2026-11-06
**Task**: 素材小图缩略图分辨率提升至 640
**Branch**: `master`

### Summary

诊断：画廊卡宽上限 420px 大于 small 档 320px，放大 1.26 倍导致发糊，用户被迫点高清；经实测否决合档为高清单档（hd 按需 COM 生成会把首次几秒等待搬到画廊）。落地：small 默认 320→640，存量配置一次性迁移（仅命中旧默认 320），Load 迁移后写回 settings.json，桌面 startup 与 --serve 各消费一次性标记走 ApplyThumbsSettingsChange 清档重建旧缓存。同步包级档位基准、前端默认值与设置 tooltip、单测与 assets-thumbs-com.md。验证：go build/vet/test 全通过，前端 vitest 2643 项全过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `96980d1` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 231: README 美化与内容校正

**Date**: 2026-11-06
**Task**: README 美化与内容校正
**Branch**: `master`

### Summary

重写 README：新增目录与简介，功能清单按域拆分为 10 个小节，长条目改为要点+子项；按 package.json/go.mod 校正技术栈版本，更新项目结构树、运行模式（--serve/--listen/--version）、文档索引与贡献说明；移除 API 方法数等易腐数字。顺带发现 docs/API参考.md 方法数与实际不一致（230 vs 278）、功能说明缺失部分新模块章节、仓库无 LICENSE 文件。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a8b629c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 232: docs 全量校验与修正

**Date**: 2026-11-06
**Task**: docs 全量校验与修正
**Branch**: `master`

### Summary

对 docs/ 与 README 做事实、链接、结构与时效性校验：API 参考补登至 278 方法，架构/功能/快速入门等按真相源校正版本与口径，修复全部失效链接（366 条 0 失效），spec 与 agent-resources 索引补全，历史快照加过时横幅。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `22221c5` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 233: 活动栏 Alt+数字快捷切换与顺序自定义

**Date**: 2026-11-07
**Task**: 活动栏 Alt+数字快捷切换与顺序自定义
**Branch**: `master`

### Summary

活动栏 8 面板支持 Alt+1..8 按当前顺序切换（终端/输入框/对话框焦点防护，超 8 放行）；面板 tooltip 带编号提示；设置「快捷键」分区新增侧边栏顺序调整（上移/下移/恢复默认，即时生效）。AppSettings 增 SidebarOrder 标量切片（纯存储，前端 normalizeSidebarOrder 归一化：滤未知/补缺失/去重/空值回退默认）；store 增 SIDEBAR_PANELS 真相源 + orderedPanels computed，ActivityBar/Home/SettingsPanel 三处消费同一顺序。单测 +17 例（settings/ActivityBar/Home/SettingsPanel 四 spec），E2E 新增 sidebar-shortcuts.spec 5 例（含键盘挂载竞态 §6.1 处理与磁盘顺序恢复链路），wailsjs models.ts 已重新生成，README 已更新。已知既有失败：terminal-tabs.spec.js:153 全屏 ESC 用例 master 上即失败（stash 对照实验确认，与本任务无关），待另开任务排查。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0205ec6` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 234: 终端全屏 ESC E2E 基线修复与 FolderTree 监听随态挂载

**Date**: 2026-11-07
**Task**: 终端全屏 ESC E2E 基线修复与 FolderTree 监听随态挂载
**Branch**: `master`

### Summary

排查 terminal-tabs.spec.js:153 既有失败：根因为 ddd0807 给 FolderTree 引入 window 级菜单监听（AssetsPanel v-show 常驻挂载，页面加载期即注册）致绝对基线断言 toBe(0) 过时——非 hook 误计数、非组件缺陷。修复 A+B：用例删除绝对断言改相对增量基线；FolderTree 右键菜单 4 个 window 监听改为菜单打开期注册/关闭注销+卸载兜底。补 5 条监听生命周期单测，顺带修复测试基建两 gotcha（enableAutoUnmount 实例泄漏、attachTo 游离 DOM 传播盲区）。沉淀 e2e-testing.md §12 + AGENTS.md 关键规则行（v2.47）。验证：terminal-tabs 4/4、npm test 2671/2671、e2e 138/138

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3f93162` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 235: 修复聚合视图工作目录刷新隐式重置选中

**Date**: 2026-11-07
**Task**: 修复聚合视图工作目录刷新隐式重置选中
**Branch**: `master`

### Summary

loadDirectories 单点收口：当前选中仍存在时保留不动，不存在才回落默认→首个→清空（含目录删光清脏选中）。消除增删/重命名/设默认工作目录后刷新的隐式跳变——聚合视图下选中态与聚合模式错配（高亮跳变/clipboard 被清/快照污染），普通树下 treeKey 变化重建 el-tree。冷启动语义不变；「设为工作目录」显式退出聚合设计维持（决策 A，经用户确认）。测试：store 单测新增 5 条（3 条复现）、E2E 聚合语境用例含回滚防假绿验证，全量 2676 条单测与 file-tree 24 条 E2E 通过。temp-memory 欠账条目已出口。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6d84723` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 236: 看板补全工作树状态列

**Date**: 2026-11-07
**Task**: 看板补全工作树状态列
**Branch**: `master`

### Summary

DashboardView 看板补全工作树状态：工作区列升级改动计数（复用 status --porcelain 同源零增量 fork）、新增最后提交时间列（相对时间，formatTime.js 新 util）、新增 stash 数列（git stash list 只读计数，stash-management 数据口径先行统一）。RepoStatus 扩 3 字段同步 wailsjs models.ts；降级矩阵覆盖 unborn/detached/Missing/单项失败。复核修复 3 处：%ct 解析失败降级缺口、E2E td.nth 假绿（改 columnIndexOf 表头解析+正向断言，沉淀 e2e-testing.md §5.3+AGENTS.md 关键规则行）、README 同步。覆盖率 service 81%/前端全指标≥80%。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f843a72` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 237: Stash 管理（repo-management-ux 子任务 2/3）

**Date**: 2026-11-07
**Task**: Stash 管理（repo-management-ux 子任务 2/3）
**Branch**: `master`

### Summary

Git 签页新增 Stash tab：service/git_stash.go（list/push/pop/apply/drop，变更类同 Commit 先例 tryLockRepo 不注册取消）；util 新增 ExecuteWithCodesAndStderr（run 抽 runFull 收口，exit 1 白名单 + stderr 双流）；pop/apply 冲突判定走 porcelain 未合并八态、冲突态正常返回 ConflictFiles 零新增错误码；前端 GitStash.vue（GitBranches 模式）+ ContentPanel tab；service 19 用例 + 组件测试 15 用例全绿，branch E2E 6/6；spec 沉淀 stash-management.md + AGENTS.md 规则行（249 字符）；check 修复 unmergedFiles 缺 UTF-8 前缀（中文名冲突清单会坏）。遗留：父任务还剩 11-07-file-level-history（planning）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c0247ab` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 238: 文件级提交历史（--follow 跨重命名追溯）

**Date**: 2026-11-07
**Task**: 文件级提交历史（--follow 跨重命名追溯）
**Branch**: `master`

### Summary

Git 追溯闭环补全：文件树右键「查看历史」→ git log --follow 单文件演进史。后端 GetFileCommitHistory 绝对路径单参数自包含（FindGitRoot+Rel 换算），:(literal) pathspec 防 glob 元字符、utf8Prefix+%x00/%x01 解析 fail-soft；FileCommit.pathAtCommit 为 diff 查看 pathspec 关键（旧段路径漂移）；前端 FileHistoryDialog（watch immediate、关闭双时机 @closed+loadSeq、R 标记 tooltip、内嵌 FileDiffDialog）；service 8 + 组件 8 用例全绿；spec 沉淀 file-level-history.md + AGENTS.md 规则行 236 字符；trellis-check 修复 5 处（目录入口 PRD 违背/代次守卫缺口/3 假绿断言）。父任务 11-07-repo-management-ux 三子任务全部完成

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `b88d0be` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 239: 行级归属追溯（git blame）

**Date**: 2026-11-07
**Task**: 行级归属追溯（git blame）
**Branch**: `master`

### Summary

代码预览行级归属：后端 GetFileBlame（git blame --porcelain 工作树 blame，状态机解析 fail-fast/fail-soft 分层，>10000 行 E_BLAME_FILE_TOO_LARGE 降级）；前端右键行归属开关 + CodeMirror 行尾标注（Compartment 重配不重建编辑器）+ 零 SHA 未提交禁跳转 + 行数守卫；FileHistoryDialog 新增 initialSha 定位扫描（上限 500 条）。实现期证伪三假设：blame 路径为字面路径禁 :(literal)（与 log 域相反）、author-time/author-tz 分行、未跟踪文件 fatal 走错误矩阵。trellis-check 修复 6 处含双实例 document 委托互串真实 bug（RepoFilterDialog 同挂 renderer）。service 覆盖率 81.2%、前端 2721 用例全绿；契约沉淀 docs/spec/git-blame.md

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e9d4c21` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 240: 提交历史→文件级追溯入口衔接

**Date**: 2026-11-07
**Task**: 提交历史→文件级追溯入口衔接
**Branch**: `master`

### Summary

CommitHistory 变更文件 tag 与 FileDiffDialog（commit 头部/range 分组头）右键新增「查看文件历史/行归属」入口：useTraceBlame 单点分流（CheckFileExists 存在性+类型预判，降级恒转 FileHistoryDialog 带 initialSha 不断链）；uiStore 追溯信箱→Home onNodeSelect→previewFile→FilePreviewRenderer autoBlameSeq 自动开启 blame。check 阶段修复 P0 竞态（autoBlameSeq 须与 filePreview 赋值同批）与 FileDiffDialog↔FileHistoryDialog 环形引用栈溢出（互嵌挂载点 v-if 外控）；新增 CheckFileExists 绑定（RPCHiddenMethods 同步）。前端 2754 用例/go 全量/build 三端绿；spec 沉淀 file-level-history §7、git-blame §8、browser-channel §2.2、AGENTS.md 关键规则行 v2.49

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `01736d3` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 241: 切换分支支持选择 git 标签

**Date**: 2026-11-08
**Task**: 切换分支支持选择 git 标签
**Branch**: `master`

### Summary

「切换分支」弹窗支持标签三组搜索选择：git checkout --detach refs/tags/<tag> 分离头检出（全路径消同名分支歧义），BranchList 新增 detachedRef 供弹窗回显分离头并置灰当前标签；wailsjs 三处同步；Go 10 用例 + 前端 5 用例 + E2E 标签切换用例全绿（service 覆盖 81.2%）。ADR 见任务 PRD：方案 A 直接检出（用户确认）、新增 CheckoutTag 独立方法零破坏兼容。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a1057bc` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 242: 临时待办工作区（剪贴板快速记录 + 完成归档）

**Date**: 2026-11-09
**Task**: 临时待办工作区（剪贴板快速记录 + 完成归档）
**Branch**: `master`

### Summary

新增侧边栏一级面板「临时待办」（Alt+9）：Ctrl+Alt+N 读系统剪贴板直接落条、自由标签、相对时间、一键复制、勾选完成进入已完成区并可回退、逐条删除；数据落 data/temp_todos.json（原子写），已完成保留 30 天且上限 500 条。新增 util.ReadClipboardText 平台分叉实现与 5 个错误码（三处同步），Alt+数字纳入保留快捷键。子 agent 审核后修复：E2E 9 面板断言、Delete 落盘失败回滚、store 快捷键单测、错误码文档表、E2E mock 登记、设计令牌归并。验证：go vet/test 全绿（service 81.3%）、前端 2781 测试全绿（覆盖率 86.9%）、双端构建通过；E2E 未实跑，剪贴板链路因本机剪贴板被占用未实机验证。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6100a2b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 243: 临时待办二期：选中文本存待办、转提示词、导出 Markdown

**Date**: 2026-11-10
**Task**: 临时待办二期：选中文本存待办、转提示词、导出 Markdown
**Branch**: `master`

### Summary

完成临时待办二期三项能力：AI 回复与代码/Markdown 预览选中文本右键存为待办（含选区缓存回退、data-action 语义锚）；条目转提示词经 uiStore 一次性信箱填入 AI 输入框（追加语义、自动切面板聚焦）；导出全部条目为流水式 Markdown 追加到指定目录当天 YYYY-MM-DD.md（桌面限定，后端方法入 RPCHiddenMethods）。新增错误码 E_TEMP_TODO_EXPORT_FAILED 并三处同步，补 service 与前端单测、README 与 docs/spec 登记。trellis-check 审核修复 7 处。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `8cf30c0` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 244: 临时待办三期：从 Markdown 导入待办

**Date**: 2026-11-10
**Task**: 临时待办三期：从 Markdown 导入待办
**Branch**: `master`

### Summary

实现面板 Markdown 导入：前端纯函数解析两类待办行（续行拼回、已完成行跳过、标签与时间归属、去空白去重），预览对话框带重复项开关，后端 AddMany 一次落盘失败整批回滚；新增 E_TEMP_TODO_IMPORT_FAILED 并沉淀 docs/spec/temp-todo.md；go test / 前端 2813 单测 / build 全绿，子 agent 审核通过

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6df358b` | (see git log) |
| `32bc61f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 245: 临时待办 Markdown 导入 E2E

**Date**: 2026-11-10
**Task**: 临时待办 Markdown 导入 E2E
**Branch**: `master`

### Summary

新增 frontend/e2e/temp-todo-import.spec.js 三条用例：预览汇总 3/2、重复开关切换到 5、确认后 AddTempTodos 入参 3/5 条且字段对齐 model.TempTodoDraft；fixture 文件名取历史日期规避未来时刻钳制。同步 docs/spec/temp-todo.md 测试锚点。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `7ba7042` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 246: 临时待办全屏编辑

**Date**: 2026-11-11
**Task**: 临时待办全屏编辑
**Branch**: `master`

### Summary

为临时待办新增全屏编辑弹窗：新建与行内编辑共享草稿，关闭保留内容、保存才退出；补组件单测 8 例并同步规范文档与 README，前端全量 2821 用例与构建通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `8d95327` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 247: 临时待办面板视觉重构

**Date**: 2026-11-11
**Task**: 临时待办面板视觉重构
**Branch**: `master`

### Summary

TempTodoPanel 视觉与信息层级重构：内容区限宽 1000px 居中、header 精简（kbd 快捷键提示 + 图标化导入导出 + 弹性搜索框）、条目正文 3 行折叠且仅截断时显示展开、展开限高 50vh 内滚、正文点击不再展开、四操作图标常驻 meta 行、轻卡片 hover 阴影、新建卡主色左边条。新增 isTextOverflowing 截断判定单点与单测，更新组件单测、temp-todo.md 展示契约与 README。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0fa0293` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 248: 修复临时待办折叠正文遮挡

**Date**: 2026-11-11
**Task**: 修复临时待办折叠正文遮挡
**Branch**: `master`

### Summary

去掉折叠正文底部 20px 渐隐遮罩（实测覆盖第三行整片高度），改用 line-clamp 自带省略号 +「展开」按钮表达截断；has-overflow 改为仅由 overflowIDs 驱动并加 8px 底部留白，正文底到按钮间距由 4px 提升到 12px。同步 temp-todo.md §8 展示契约。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `204f1ee` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 249: 临时待办行内编辑自适应高度

**Date**: 2026-11-11
**Task**: 临时待办行内编辑自适应高度
**Branch**: `master`

### Summary

行内编辑 textarea 由固定 3 行改为 EP autosize（minRows 3 / maxRows 15），展开读完再编辑时不必反复滚动；超 15 行框内滚动，更长内容仍走全屏编辑，新建卡保持固定 3 行可拖拽。E2E 实测 5 行 115px 无滚动、30 行 325px（15 行）内滚；同步 temp-todo.md §7 与 README（并修正渐隐旧表述）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6468493` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 250: 提示词管理面板（话术库 + 占位符复制）

**Date**: 2026-11-11
**Task**: 提示词管理面板（话术库 + 占位符复制）
**Branch**: `master`

### Summary

新增第 10 个侧边栏面板「提示词」：PromptService（data/prompts.json 原子写）+ PromptLibraryPanel（增删改查、标题/正文检索）+ 前端 {{名称}} 占位符解析与复制弹窗（未填保留原样）；自定义快捷键补位（默认空）；双 agent 审核修复设置键名漂移等 8 项问题，补齐 4 条体验改进

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f0cb6e5` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 251: 提示词面板 Playwright E2E

**Date**: 2026-11-12
**Task**: 提示词面板 Playwright E2E
**Branch**: `master`

### Summary

新增 prompt-library.spec.js，覆盖打开面板、新建含占位符条目、三条复制路径与删除确认；剪贴板真实读写并用哨兵+差异填写值三重守卫防假绿；同步 e2e README 与 docs/spec/e2e-testing.md §5.4（EP 输入框 data-* 落点）。两轮子 agent 审查后修复断言与选择器精度。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `56f78e2` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 252: 占位符默认值语法

**Date**: 2026-11-12
**Task**: 占位符默认值语法
**Branch**: `master`

### Summary

实现 {{名称:默认值}}：前后端统一三级回退（填写值→默认值→保留原文）；前端解析/分段单点与填写弹窗默认值提示，Go renderPrompt 同步并放行 form Required；AiFunctionRunner 必填门禁识别模板默认值，ParamsEditor 复用占位符解析单点；同步文档与两端测试。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `439a6b3` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 253: 提示词面板弹窗与替换预览优化

**Date**: 2026-11-12
**Task**: 提示词面板弹窗与替换预览优化
**Branch**: `master`

### Summary

编辑弹窗加宽至 1040px/94vw 并修正 textarea 下限落点；填写弹窗新增实时替换预览与三类高亮（填写值/默认值/未填保留），深浅色适配；新增 segmentPlaceholders 单点保证预览与复制同源；沉淀 Vue scoped :global 踩坑、尺寸分档与对比度口径。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `dc47e4d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 254: 待办与提示词面板编辑焦点与快捷键

**Date**: 2026-11-12
**Task**: 待办与提示词面板编辑焦点与快捷键
**Branch**: `master`

### Summary

临时待办与提示词面板新建/编辑统一为：打开即聚焦首个可编辑框并定位文末；Ctrl/Cmd+S 保存（兼容 Ctrl+Enter）；ESC 取消编辑（全屏弹窗取消整条，X/返回编辑保留草稿）。新增 utils/focusInput.js 单点；引入草稿/弹窗双会话代次防在途保存竞态，快速记录拆独立 saving。补焦点、快捷键、取消与竞态单测。经三轮子代理审核并修复。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0009e48` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 255: WorkBench 白屏卡死排查与恢复

**Date**: 2026-11-13
**Task**: WorkBench 白屏卡死排查与恢复
**Branch**: `master`

### Summary

诊断安装版白屏假死：定性为 WebView2 renderer 主线程运行中猝死（11:10:17 锚定，非崩溃/非代码/非产物），taskkill 重启恢复正常。沉淀排障条目至 docs/agent-resources/troubleshooting/（临时解法已验证），索引新增「运行时故障」分类。遗留决策：watchdog 自愈或复发时 procdump -hang 留现场。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ee0219e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 256: E2E 补齐待办全屏编辑与提示词编辑弹窗交互

**Date**: 2026-11-14
**Task**: E2E 补齐待办全屏编辑与提示词编辑弹窗交互
**Branch**: `master`

### Summary

新增 temp-todo-editor.spec.js 与 prompt-library-editor.spec.js 共 7 条 Playwright 用例：打开后焦点与光标末尾、Ctrl+S 保存、ESC 取消整条编辑与 X 保留草稿分流；光标断言经「置 0 + 复用节点重开」加固消除假绿，并经「仅保留 focus」变异验证鉴别力。同步 e2e README 与 docs/spec 测试锚点，产品代码零改动。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ad18036` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 257: 沉淀 E2E 焦点与光标断言鉴别力范式

**Date**: 2026-11-14
**Task**: 沉淀 E2E 焦点与光标断言鉴别力范式
**Branch**: `master`

### Summary

docs/spec/e2e-testing.md 新增 §13：判定表（草稿保留/DOM复用/值非空）、四步加固（置0→保留草稿关闭→重开+expando 探针钉复用）、不可鉴别的三条记录口径、实现形态对照、先例索引，并补 EP dialog focus-trap 时序约束（须 @opened+nextTick，E2E 只能 toBeFocused 重试）。AGENTS.md 新增关键规则行（201 字符）。排查 23 个『打开即聚焦』入口：实质结论为 AI 对话输入框并无打开即聚焦；可鉴别缺口是重命名三入口（FileTreePanel:1765/DirectoryTree:557/AiChatPanel:658 的 focus+select），建议 P2 补 3 条同构用例；其余空值新建类不补并给理由。两路子 agent 复核（trellis-check 规范 + cavecrew-reviewer 只读事实核查），共修 17 条问题（含清单漏 FolderTree、AiChatPanel 单测无 caret 断言、『唯一落位单点』绝对化等），终态通过。E2E temp-todo-editor + prompt-library-editor 7/7 绿，产品代码零改动。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `8fbc7a8` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 258: E2E 监听计数 helper 与静态守护

**Date**: 2026-11-20
**Task**: E2E 监听计数 helper 与静态守护
**Branch**: `master`

### Summary

新增 frontend/e2e/listenerCounter.js（仅相对增量，缺基线抛错）与 scripts/check-e2e-listener-hooks.mjs（5+2 类禁令、15 例 selftest、CI 接入）；terminal-tabs 改走 helper 且断言语义不变；顺手修复 sidebar-shortcuts 既有 9→10 失效断言；沉淀 e2e-testing.md §12.1 与 AGENTS 关键规则行。验证：全量 E2E 152 passed、npm test 2932 passed、覆盖率 87.17%。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `865e334` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 259: E2E 侧边栏面板顺序同源守护

**Date**: 2026-11-20
**Task**: E2E 侧边栏面板顺序同源守护
**Branch**: `master`

### Summary

SIDEBAR_PANELS 抽零依赖 sidebarPanels.js，E2E sidebar-shortcuts 全部断言派生并断渲染序列；新增 check-e2e-sidebar-panels.mjs（--selftest 11 例、CLI 位置参数与阈值边界经审核修正）接入 CI 2.7；沉淀 e2e-testing.md §12.4、AGENTS 规则行、门禁清单；登记 useSessionState 白名单缺项与组件单测硬编码两条待办。验证：npm test 2932 passed、E2E 全量 152 passed + sidebar 3 连跑、覆盖率 4 指标 81%-90%。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ac234c1c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 260: FileHistoryDialog 覆盖率插桩下的用例超时修复

**Date**: 2026-11-20
**Task**: FileHistoryDialog 覆盖率插桩下的用例超时修复
**Branch**: `master`

### Summary

「initialSha 超定位上限未命中」用例在 v8 插桩 + 全量并行下实测 5.5~6.5s，越过 vitest 默认 5s 上限，定性为环境敏感 flaky（裸跑稳定通过；单文件无覆盖率 1.36s、覆盖率 2.7s、全量并行最坏 6.45s）。修复用双层超时：用例级 { timeout: 20000 } + vi.waitFor 显式 { timeout: 15000, interval: 50 }，断言零改动（调用 25 次、终点文案全等、目标 SHA 负断言保留）；未动全局 testTimeout 与组件逻辑。验证：覆盖率全量连跑 3 次全绿（该用例 6453/5494/6274ms）、npm test 2932 用例全绿、四指标 87.17/81.04/85.38/89.82%。沉淀 docs/spec/test-stability.md §8（适用范围/根因实测/方案对照/推荐写法/判定要点/反例/断言要求）+ AGENTS.md 钩子行 202 字符 + README 索引。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a6ee2aea` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 261: 崩溃恢复面板白名单与 SIDEBAR_PANELS 同源

**Date**: 2026-11-20
**Task**: 崩溃恢复面板白名单与 SIDEBAR_PANELS 同源
**Branch**: `master`

### Summary

useSessionState 的 VALID_PANELS 由硬编码 7 面板改为自 SIDEBAR_PANELS 派生，修复 branchcompare/todo/prompts 崩溃恢复被回退 directory；补 4 组单测（含非空守卫防假绿、空值路径回归）；契约沉淀至 cross-layer-contracts 会话快照 §3 与 AGENTS 关键规则行；全量 2936 用例通过、覆盖率四指标 87.17/81.04/85.38/89.82。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6ddc58cd` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 262: 侧边栏面板图标映射对齐守护

**Date**: 2026-11-24
**Task**: 侧边栏面板图标映射对齐守护
**Branch**: `master`

### Summary

新增 scripts/check-sidebar-panel-icons.mjs 静态守护，双向比对 ActivityBar 的 panelIconMap 与 SIDEBAR_PANELS；CI 2.8 + package.json 入口；--selftest 22 用例；同步 process-build-quality 门禁表、prompt-library §5、AGENTS 规则行（347 字符）。两轮 subagent 审核修复行号偏移、注释内花括号 fail-open、顶层非对象项静默跳过、模板引号干扰等缺陷。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `62125234` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 263: AI 功能自然语言自动生成配置

**Date**: 2026-11-25
**Task**: AI 功能自然语言自动生成配置
**Branch**: `master`

### Summary

配置管理新增自然语言创建：独立生成链路（固定 JSON schema、禁写执行工具、非用户目录运行）、静态校验与 ID 避让、前端弹窗与编辑回写修复；审查 19 项问题全修；集成测试注入用户级 claude settings env；后端/前端/E2E/覆盖率全通过

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `af13027d` | (see git log) |
| `818f479d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 264: 排查 E2E 与后端覆盖率并行资源冲突

**Date**: 2026-11-25
**Task**: 排查 E2E 与后端覆盖率并行资源冲突
**Branch**: `master`

### Summary

定位并修复两类并行失败根因：Go ./... 遍历 frontend 非点号目录与 Playwright 重建 test-results 竞态（E2E 产物改落 .e2e-dist/.e2e-artifacts）、chat-task:started 与 output 顺序竞态（started 前移至写 stdin 前）。coverage-check.sh 失败保留真实输出；并行 4 轮全绿，覆盖率基线不变。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `331bb207` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 265: T0 个人看板指标口径锁定

**Date**: 2026-11-25
**Task**: T0 个人看板指标口径锁定
**Branch**: `master`

### Summary

完成 T0 阶段 0 决策：确认两条前提假设（本仓即 WorkBench、仅聚合工作台内数据），用 Signal Check 三问锁定 5 个上板指标 M1 待收尾仓库/M2 久未推进仓库/M3 待办积压/M4 AI 成本/M5 AI 成功率，排除清单 12 项。新增 docs/spec/dashboard-metrics.md 口径契约并登记 README 索引与 AGENTS 关键规则钩子。子 agent 审核纠正 AI 历史保留期 30 天→90 天（aiTaskHistoryMaxDays），同步 T1/父任务 4 路→3 域 5 卡，标注 useRecentAccess 不上卡。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `64cb180d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 266: T1 个人看板概览面板实现

**Date**: 2026-11-25
**Task**: T1 个人看板概览面板实现
**Branch**: `master`

### Summary

新增第 11 个侧边栏面板「概览」：后端 OverviewService 单次 IPC 聚合 M1–M5（5 分钟 TTL、三域分区降级），前端 3 域 5 卡可下钻，新增 StaleRepoDays 设置项与 overviewDrillRequest 信箱。trellis-check 审核发现 12 项问题（4 阻断：M2 取整口径错误、App 签名缺 error、设置注释错位、M1/M2 空态缺失）并全部修复。验证：go build/vet、service 覆盖 81.8%、前端 2977 单测、覆盖率 87.06%、E2E 5 新用例 + 17 回归全部通过。父任务 T2 范围已收缩并回写。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ab2aaa93` | (see git log) |
| `81c59610` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 267: T2 AI 成本口径统一与排行视图

**Date**: 2026-11-26
**Task**: T2 AI 成本口径统一与排行视图
**Branch**: `master`

### Summary

AI 任务历史统计补齐四状态计数（success/failed/canceled/timeout）与 Filter.Statuses 多状态匹配；概览 AI 域改走 Stats 同源聚合，成功率分母与卡片文案同式；历史抽屉排行支持次数/成本/token 排序、成本占比与取消数，M5 下钻预选失败+超时（全取消周次预选取消）；导出改用上次查询生效的筛选快照。两轮独立子代理审核后修复 5 项（下钻矛盾、token 口径标注、分母防御、导出一致性、测试接线），全量门禁通过：go test ./...、前端 2987 用例、service 覆盖率 81.8%、前端 87.1%、npm run build。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3b5c1c9f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
