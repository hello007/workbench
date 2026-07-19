# Journal - liuyang (Part 1)

> AI development session journal
> Started: 2026-06-09

---



## Session 1: GitHub Actions 自动打包发版流水线

**Date**: 2026-06-10
**Task**: GitHub Actions 自动打包发版流水线
**Branch**: `master`

### Summary

创建 GitHub Actions Release 流水线（.github/workflows/release.yml），tag v* 触发自动构建 Windows exe 并发布到 GitHub Release。补充 README.md 发版流程说明。GitHub 作为 Gitee 镜像仓库跑 CI/CD。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `26ecf7e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: 实现检查更新与自动更新功能

**Date**: 2026-06-10
**Task**: 实现检查更新与自动更新功能
**Branch**: `master`

### Summary

通过 GitHub Releases API 实现检查更新、下载新版本（进度推送）、批处理替换重启、启动时待更新检测。新增 model/update.go、service/update.go、UpdateDialog.vue，修改 SettingsPanel.vue/Home.vue/app.go 及 Wails 绑定文件。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `88a6efe` | (see git log) |
| `fc98c91` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: 修复文件树拷贝到后展开节点被收起

**Date**: 2026-06-10
**Task**: 修复文件树拷贝到后展开节点被收起
**Branch**: `master`

### Summary

refreshNode 在路径不在 nodesMap 时回退到 refreshCounter++ 触发 <el-tree> key 变更整树重建，导致已展开节点全部丢失。改为命中即刷新 / 未命中沿父路径回溯到首个已展开祖先并刷新 / 都没有则静默放弃。refreshAll 整树语义保持不变。新增三条单测覆盖三种分支。spec 中 cross-layer-thinking-guide.md 增加 UI Local Refresh vs Whole-Tree Rebuild 章节沉淀该反模式。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `185ef0b` | (see git log) |
| `49de4e4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: 新增用 Obsidian 打开入口与文件预览渲染增强

**Date**: 2026-06-23
**Task**: 新增用 Obsidian 打开入口与文件预览渲染增强
**Branch**: `master`

### Summary

为工作目录树/文件树右键菜单及内容面板查看操作新增'用 Obsidian 打开'入口（文件夹→自身、文件→父目录作为 vault）；设置面板通用→外部应用支持自定义 Obsidian 程序路径（配置优先，否则走 obsidian:// URI + 注册表预检 + cmd /c start，未检测到时引导配置）；内置 Obsidian 图标；后端含 Windows 注册表预检（build tag 隔离）与单测。同会话交织提交了文件预览增强（FileBytes/Kind 分流 + FilePreviewRenderer + file-viewer3，支持图片/PDF/Office 内嵌预览）。研究归档于 research/obsidian-launch.md（基于官方文档核实 URI 协议）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `095a6ab` | (see git log) |
| `bead5c7` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 5: 文件类型预览功能（图片/文本/Office 内嵌 + PDF 降级）

**Date**: 2026-06-23
**Task**: 文件类型预览功能（图片/文本/Office 内嵌 + PDF 降级）
**Branch**: `master`

### Summary

调研 Flyfish Viewer（POC 实测依赖树过大、resolve 停滞不可行）后回退自研拼装。实现内嵌预览：图片(jpg/png/bmp/gif/webp)、文本(txt/json/sql/md/代码，CodeMirror6 只读高亮 + Markdown 渲染)、Office(docx→docx-preview、xlsx/xls/csv→SheetJS 多 sheet 表格)；文本类编辑双模式(只读+编辑保存)。降级「用默认程序打开」：PDF、pptx、旧 .doc/.ppt、不支持/超大/损坏。PDF 内嵌因 pdfjs+Vite ESM+WebView2 系统性双实例问题(私有字段 brand-check，4 种 worker 配置均失败，详见 research)暂降级。修复：图片读取失败降级、编辑态按钮布局、docx-preview/xlsx 改静态 import 解决 wails dev 动态 import .vite/deps fetch 失败。exe 约 17.6MB。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bead5c7` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 6: PDF 内嵌预览（pdfjs viewer + iframe，规避双实例）

**Date**: 2026-06-23
**Task**: PDF 内嵌预览（pdfjs viewer + iframe，规避双实例）
**Branch**: `master`

### Summary

调研 PDF 内嵌可实现方案（research/pdf-embed-options.md），排除后端转图（GPL/AGPL 金融排除），选定方案 B：pdfjs 官方完整 viewer 作为静态资源用 iframe 加载。iframe 是独立 browsing context，pdfjs 类只在 iframe 内一份，架构上根治前端 pdfjs 双实例（之前主页面 pdfjs 4 次失败的根因），主页面不 import pdfjs。POC-1 验证 Wails AssetServer 把本地 PDF 映射成同源 URL + iframe 加载基础可行；POC-2 引入 pdfjs viewer v4.8.69（viewer.html?file= 双重 encode），自带翻页/缩放/搜索/缩略图工具栏。后端 server/preview.go：/preview-pdf?path= handler，http.ServeFile 支持 Range，路径安全（.pdf 白名单+防穿越+普通文件校验）。locale 精简为中英（en-US/zh-CN/zh-TW）。替换之前的 PDF 外部打开降级。exe 约 33.17MB（含 viewer）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `9d36bdb` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 7: README 更新：补充 PDF 内嵌项目结构

**Date**: 2026-06-23
**Task**: README 更新：补充 PDF 内嵌项目结构
**Branch**: `master`

### Summary

README 项目结构补充 PDF 内嵌预览新增的 server/（preview.go AssetServer handler）与 frontend/public/pdfjs-viewer/（pdfjs viewer 静态资源），构建章节补产物体积说明（exe 约 33MB）。文件类型预览 PDF 条目复核准确。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f9df40a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 8: 右侧操作面板布局优化

**Date**: 2026-06-23
**Task**: 右侧操作面板布局优化
**Branch**: `master`

### Summary

优化右侧操作面板文件预览态布局，使预览区更适中：操作按钮组（基本/编辑/查看）从垂直堆叠改为横向紧凑流动（flex row + wrap），字体调小(12px)、一行排列（nowrap），查看操作 6 按钮一行排下；移除文件信息区「类型」字段（路径保留原 el-descriptions 样式），「复制路径/复制文件名」迁移到 descriptions 第二列（原类型位置），查看操作组相应移除；收紧「文件操作」标题与路径间距。UI 迭代多轮（紧凑化→间距→按钮一行→移除类型→恢复路径→按钮入第二列）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `dfcab63` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 9: 文件预览区浅蓝主题底美化

**Date**: 2026-06-24
**Task**: 文件预览区浅蓝主题底美化
**Branch**: `master`

### Summary

预览区 .file-preview 改为浅蓝主题底(--primary-bg)+主题色边框+圆角阴影，header 底部分隔线+标题主题色加粗；渲染器内部(CodeMirror/图片/Markdown/Office/PDF)统一白底，形成浅蓝容器>白底内容区层次，一眼识别为文件预览。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `aa9b26b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 10: 修复预览大小判定（tooLarge 误伤图片/Office）

**Date**: 2026-06-24
**Task**: 修复预览大小判定（tooLarge 误伤图片/Office）
**Branch**: `master`

### Summary

PreviewFile（service）按 kind 分流：仅 text 判 1MB tooLarge 并读全文；image/pdf/office/unsupported 不判 size、不读内容。去掉旧的 !IsPreviewable+0x00 binary 探测（detectPreviewKind 已按扩展名分类，前端按 kind 分发）。image/office 走 ReadFileBytes 50MB；pdf 走 iframe 无限制。修复：图片/Office >1MB 不显示、PDF 大文件误弹过大提示。单测更新。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `2cc7957` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 11: 修复 WorkBench 启动后影响其他应用复制粘贴的剪贴板锁竞态

**Date**: 2026-06-25
**Task**: 修复 WorkBench 启动后影响其他应用复制粘贴的剪贴板锁竞态
**Branch**: `master`

### Summary

诊断并修复 Win32 剪贴板独占锁抢占：Home.vue 的 window.focus 事件每次切窗都触发 ReadFromSystemClipboard → OpenClipboard(0)，与其他 Windows 应用产生数十毫秒锁竞态。方案 A：移除 focus 同步、粘贴按钮去 disabled 改延迟校验、OpenClipboard 加 3 次重试容错。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `81abd8b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 12: 文件预览区支持复制选中文本（Ctrl+C + 右键复制/全选菜单）

**Date**: 2026-07-01
**Task**: 文件预览区支持复制选中文本（Ctrl+C + 右键复制/全选菜单）
**Branch**: `master`

### Summary

修复 Home.vue 全局 Ctrl+C 拦截过宽（选中文本时放行浏览器原生复制，不再被劫持为复制文件路径）；FilePreviewRenderer 新增右键「复制/全选」菜单（仅 text/markdown，复制走 navigator.clipboard，全选用 CodeMirror selectAll / Selection API）；修复「全选后再次右键复制失效」（selectionchange 缓存预览区选区文本，右键清除 DOM 选区后用缓存回退）；复制前去除前后空格。新增 7 个测试用例，顺带修复 Home.spec.js 既有测试基础设施（runtime mock 路径、缺失 binding）。前端 136/136 通过，vite build 通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e2d4736` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 13: 右边栏操作面板支持 Git diff/提交/推送

**Date**: 2026-07-04
**Task**: 右边栏操作面板支持 Git diff/提交/推送
**Branch**: `master`

### Summary

为右侧操作面板（ContentPanel）补全 Git 工作流闭环。后端 service/git.go + util/git.go 新增 Commit/Push/GetDiff/HasUpstream（复用 GitCommand.Execute 与 FindGitRoot，新增 ExecuteWithCodes 容忍 diff 退出码 1），app.go 暴露 CommitFiles/PushRepo/GetFileDiff/HasUpstream。前端 LocalChanges.vue 重构为 IDEA commit 窗口风格：单区变动清单 + 复选框勾选要提交的文件 + 双击文件弹窗双栏左右对照 diff（FileDiffDialog.vue，前端解析 unified diff）+ 底部 commit 输入框与提交/提交并推送/推送按钮，回滚下沉到「更多」菜单。采用 pathspec 选择性提交（git add + git commit -- 选中文件），不引入暂存区语义；推送无上游时提示 set-upstream；提交/推送后联动刷新本地变动+提交历史+仓库信息。go test 与 npm test(136/136) 全绿，trellis-check 9 条验收标准全部通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `26a5c52` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 14: 修复右边栏内容面板多处显示问题

**Date**: 2026-07-04
**Task**: 修复右边栏内容面板多处显示问题
**Branch**: `master`

### Summary

修复右侧内容面板（ContentPanel）在仓库节点下三个 tab 的一系列显示问题，纯 CSS/样式调整、无功能逻辑改动。1) 高度链断层：el-tabs/tab-pane 未参与 flex 链，CommitHistory timeline 与 LocalChanges table 用固定 max-height(600/500) 撑高，叠加上方固定区触发 content-panel 整体 overflow:auto 滚动条；改为建立 content-panel→el-tabs→tab-pane→列表区的 flex:1+min-height:0+overflow 高度链，列表区内部滚动，整面板不滚，并顺带修复 content-panel 改 overflow:hidden 后文件夹节点操作按钮被裁剪的回归。2) CommitHistory hover：.commit-item:hover 的 translateX(4px) 使提交项向右漂移触发横向滚动条，去掉 translateX 并给 timeline-container 加 overflow-x:hidden 兜底。3) 提交历史卡片右边缘贴滚动条：timeline-container 加 padding-right:var(--spacing-md)，el-card__body 左 padding 减到 12px 平衡左右留白。4) GitInfo 远程地址 http 分支缺复制按钮：统一 url-with-copy 容器，http 用 el-link、git/ssh 用 span，共用复制按钮，:deep(.el-link.url-text) 仅设字体不覆盖主题色。5) GitInfo label 列偶发换行：:deep(.el-descriptions__label) 固定 width/min-width:80px + white-space:nowrap。6) GitInfo label 居中被 EP .el-descriptions__cell{text-align:left}(特异性0,0,3,0) 覆盖：加父选择器 .git-info-card 并针对 border 模式用 .is-bordered-label(特异性0,0,4,0) 稳定覆盖。npm test(136/136) 与 npm run build 全程通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `03f580d` | (see git log) |
| `2e00152` | (see git log) |
| `89daa13` | (see git log) |
| `1d5bc4d` | (see git log) |
| `9b66378` | (see git log) |
| `539cc9c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 15: 左栏选中 git 工作目录右栏直显仓库详情

**Date**: 2026-07-04
**Task**: 左栏选中 git 工作目录右栏直显仓库详情
**Branch**: `master`

### Summary

让左侧工作目录树在选中一个本身是 git 仓库的工作目录时，右侧操作面板直接显示该仓库的 git 详情（仓库信息/提交历史/本地变动 + 拉取/切换分支），与文件树选中 git 仓库节点完全一致，并在左栏工作目录项标记 git 仓库。采用后端字段方案：model.Directory 加 IsGitRepo 字段(json:"isGitRepo")，app.go GetDirectories 用 util.NewGitCommand().IsGitRepository 遍历检测填充，AddDirectory/UpdateDirectory/GetDefaultDirectory 经 applyGitRepoFlag 一致填充（nil 安全），旧配置无该字段时 json 零值 false 天然兼容；前端 Home.onDirectorySelect 命中 newDir.isGitRepo 时构造 {id,path,name,type:'directory',isGitRepo:true} 的 selectedNode 复用 ContentPanel 现有 git 详情渲染，非 git 工作目录保持空状态，onNodeSelect 文件树选中仍优先覆盖；DirectoryTree 工作目录项 v-if=dir.isGitRepo 显示绿色 SuccessFilled 对勾（复用 FileTreePanel 标记样式 + title=Git 仓库)。后端 4 单测覆盖 git/非git/旧配置/nil 安全，前端 npm test 136/136 通过，trellis-check 5 条 AC 全过、跨层/回归/复用/边缘达标。文档同步更新 docs/功能说明.md(工作目录管理+Git 集成两节)与 README.md。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3aa7eb0` | (see git log) |
| `4497d03` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 16: 工作目录 git 标识缓存与异步刷新优化启动速度

**Date**: 2026-07-04
**Task**: 工作目录 git 标识缓存与异步刷新优化启动速度
**Branch**: `master`

### Summary

修复上轮 workdir-git-detail 在 GetDirectories 同步检测所有工作目录 IsGitRepo 导致启动延迟的问题（每个目录一次 git rev-parse 子进程，N 个目录阻塞 UI）。改为缓存+持久化+异步刷新模式：1) service.DirectoryService.Create 在 Save 前用 util.NewGitCommand().IsGitRepository(absPath) 检测并持久化 IsGitRepo 到 directories.json，Update 每次重算（覆盖 path 变化）；2) app.go GetDirectories 去掉同步检测 for 循环，直接返回 Load 结果，启动零子进程、列表秒显；3) 新增 app.go RefreshDirectoriesGitFlag 同步 API：基于最新 Load 合并（只更新 IsGitRepo 字段，保留其他字段最新值，规避与并发 AddDirectory 的竞态覆盖）→ Save 回写 → 返回新列表；4) 移除冗余 applyGitRepoFlag；5) 前端 Home.vue onMounted 改为 loadDirectories().then(() => refreshGitFlags())，先用缓存渲染列表，再 await 刷新替换 directories.value（不重置 selectedDirectoryId/selectedNode，左栏标记按 dir.isGitRepo 自动刷新，失败静默 debug.log）；6) wailsjs 补 RefreshDirectoriesGitFlag 绑定，Home.spec.js 补 mock。后端 6 个单测覆盖 GetDirectories 不检测/Create/Update 持久化/Refresh 检测+回写/Refresh 基于最新 Load 合并保留其他字段(并发竞态)/旧配置兼容；前端 npm test 136/136；trellis-check 6 条 AC 全过、启动性能/持久化/竞态/跨层/回归/边缘达标。wailsjs 绑定后被 wails 工具链自动按字母序重排，单独 chore commit。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `088226d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 17: 用 assets 图标替换操作按钮与 git 仓库标记

**Date**: 2026-07-04
**Task**: 用 assets 图标替换操作按钮与 git 仓库标记
**Branch**: `master`

### Summary

用 frontend/src/assets/icons/ 下的图片按名字对应替换操作按钮/菜单图标 + git 仓库标记，纯前端模板/样式/import 清理，无功能逻辑改动。复用 obsidian 现有 <img class=btn-img-icon/context-menu-img-icon> 模式：1) explorer.png/vscode.ico/warp.ico 替换 5 处「打开资源管理器/用 VSCode 打开/用 Warp 打开」按钮与右键菜单图标（ContentPanel 文件夹+文件两查看操作按钮组、DirectoryTree 右键、FileTreePanel 目录+文件两右键，共 15 图标）；2) git.png 替换 DirectoryTree 工作目录项与 FileTreePanel 文件树节点的 SuccessFilled 绿对勾 git 仓库标记（2 处）；3) 清理无用 EP 图标 import（Monitor/EditPen/Promotion，DirectoryTree/FileTreePanel 的 SuccessFilled 仅 git 标记用也一并清理），ContentPanel 的 SuccessFilled 保留（拉取结果表格/状态栏仍用）；4) git 标记 img 用 scoped 样式（dir-item-git-img/tree-node-git-img，14px、margin-left:5px、vertical-align:middle、object-fit:contain）对齐原 SuccessFilled 视觉。.ico 由 Vite 当静态资源处理，构建产物 vscode/warp .ico 正常产出。npm test 136/136、npm run build 通过；trellis-check 17/17 替换到位、import 正确、无回归。另发现 pre-existing 冗余文件 vscode(1).ico（非本任务，建议后续清理）。finish-work 前 obsidian.png(modified) + 13 个新文件类型图标(untracked) 阻塞脏树，按用户确认作为 chore commit 提交（e27774e）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bc1fde5` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 18: 文件树按文件类型显示图标

**Date**: 2026-07-04
**Task**: 文件树按文件类型显示图标
**Branch**: `master`

### Summary

文件树文件节点按后缀显示对应类型图标，替换统一的 EP Document。纯前端硬编码默认映射（方案 A，无后端/无 UI/无自定义），映射表抽到独立 utils/fileIconMap.js 便于下期接 AppSettings 时合并。新建 frontend/src/utils/fileIconMap.js：import 类型图标 + DEFAULT_ICON_MAP（后缀→图标）+ getIconForFile(name)（取最后一个点后缀、toLowerCase、未命中返回 null）+ getExtension；FileTreePanel.vue 文件节点 el-icon Document 改为 template v-else 分支（getIconForFile 真值→img.tree-node-file-icon 14px，否则 fallback Document）。4d430e4 首版 13 类（xlsx/docx/txt/pdf/png/md/java/py/html/js/json/yaml/jpg）；aa76c88 加 ppt(ppt/pptx)+xmind；0751df3 扩展 9 类(typescript ts/tsx、go、css scss/sass/less、vue、xml svg、shell sh/bash/zsh/fish、db sqlite/sqlite3、zip 压缩包系列、properties 配置类 ini/conf/cfg/env/toml)；f98f7ee 加 exe(msi)+tmpl(tpl/template)+gitignore 后缀(.gitignore)+license 文件名匹配(LICENSE/LICENCE/COPYING/NOTICE，新增 FILENAME_ICON_MAP，getIconForFile 重构为先查文件名再查后缀)。npm test 136/136 全程通过、npm run build 通过；trellis-check 0 问题。自定义映射（AppSettings 持久化+合并+UI）留待下期。finish-work 前 markdown.png(modified pre-existing) 阻塞脏树，作为 chore commit 提交(6c9310a)。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `4d430e4` | (see git log) |
| `aa76c88` | (see git log) |
| `0751df3` | (see git log) |
| `f98f7ee` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 19: 修复 Git 仓库信息面板两个体验问题（偶发 N/A + 工作目录切换双刷新）

**Date**: 2026-07-05
**Task**: 修复 Git 仓库信息面板两个体验问题（偶发 N/A + 工作目录切换双刷新）
**Branch**: `master`

### Summary

完成两个前端体验 bug 修复。07-04：GitInfo 缓存命中偶发丢失最新提交（N/A）——缓存升级为 {info,latestCommit}，Promise.allSettled 区分 info/commit 成败，commit 失败不落缓存避免污染，watch(repoPath) 与 onNodeSelect 清空 latestCommit/localLatestCommit 残留。07-05：工作目录切换 git 仓库内容面板双刷新——onDirectorySelect 重构，按 newDir.isGitRepo 直接设目标 selectedNode，消除 null 中间态导致的 content-inner 卸载再挂载。新增 GitInfo.spec.js（3 用例），Home.spec.js 增补跨仓库切换与 onDirectorySelect 用例，共 34 测试通过，build 成功。两个任务均已 archive。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ed19192` | (see git log) |
| `74df079` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 20: 修复 FileTreePanel.spec.js 缺失 favorites mock 致全量测试 exit 1

**Date**: 2026-07-05
**Task**: 修复 FileTreePanel.spec.js 缺失 favorites mock 致全量测试 exit 1
**Branch**: `master`

### Summary

FileTreePanel.vue 经 useFavorites 调用 GetFavorites/AddFavorite/RemoveFavorite/UpdateFavoriteAlias/UpdateFavoriteGroup 5 个方法，但 FileTreePanel.spec.js 的 App mock 未提供这些 export，导致全量 vitest run 报 31 个 unhandled rejection 并 exit 1。按 Home.spec.js 既定约定补全 5 个 mock（GetFavorites→[]，其余→true）。全量测试恢复 143/143 绿、exit 0。改动单文件 5 行。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6265f5b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 21: 工作目录复制路径 + F2/Del 重命名删除快捷键

**Date**: 2026-07-05
**Task**: 工作目录复制路径 + F2/Del 重命名删除快捷键
**Branch**: `master`

### Summary

为工作目录列表(DirectoryTree)右键新增'复制路径'；为工作目录列表与文件树(FileTreePanel)的重命名/删除操作增加默认 F2/Del 快捷键并支持设置面板自定义。扩展 useShortcuts 放开功能键单键白名单与 AppSettings 两个新字段；Home.vue 全局 keydown 增严格焦点判定（输入框/对话框/终端聚焦时不触发），避免 Del 误永久删除文件；SettingsPanel 拦截与固定快捷键(F5/Ctrl+C/X/V)冲突的录制。新增 useShortcuts.spec.js 19 例、DirectoryTree 复制路径 2 例，go test 与 vitest(164) 全绿。同步 README 与功能说明。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `5aa49d9` | (see git log) |
| `484a03d` | (see git log) |
| `17211a3` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 22: 修复 md 预览相对链接点击崩溃与应用内导航

**Date**: 2026-07-06
**Task**: 修复 md 预览相对链接点击崩溃与应用内导航
**Branch**: `master`

### Summary

修复 markdown 预览中点击相对链接触发顶层导航导致 SPA 崩溃的严重 bug（AssetServer fallback 返回错误 JSON 整体替换页面）。FilePreviewRenderer 拦截 <a> 点击，相对引用（含 ./ ../ 中文文件名 percent-encoding）解析为绝对路径在预览面板内切换，外部 http 链接走系统浏览器，同文档锚点滚动定位。后退按钮采用单步语义（预览脱离文件树选中节点时可返回，按需显隐）。Home.onNodeSelect 显式传 path 驱动预览，根治同节点再点空白与切换预览上一文件（props 异步更新时序）两个回归。新增前端单测 176 用例覆盖链接分发、路径解析、后退语义、props 时序。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `b724d1e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 23: markdown frontmatter 预览优化

**Date**: 2026-07-07
**Task**: markdown frontmatter 预览优化
**Branch**: `master`

### Summary

优化文件预览 markdown 的 YAML frontmatter 展示：正则剥离 frontmatter + js-yaml 解析为结构化属性面板（数组 el-tag、标量原值、嵌套对象 JSON.stringify、多行字符串原样），解析失败降级为复用 hljs yaml 的代码块。新增 js-yaml 依赖，属性面板置于 markdownBodyRef 容器内复用既有右键复制/全选逻辑。补充 11 个 frontmatter 测试用例，201 测试全通过，vite build 成功。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e25e51a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 24: Obsidian 未注册 vault 报错处理：预检、引导与自动注册

**Date**: 2026-07-08
**Task**: Obsidian 未注册 vault 报错处理：预检、引导与自动注册
**Branch**: `master`

### Summary

为「用 Obsidian 打开」增加打开前预检：读 obsidian.json 复刻「最具体包含 vault」归属判断，未注册时弹三按钮确认框--「自动注册并打开」（tasklist 进程检测+原子写+备份 obsidian.json 保留未知字段+发URI，二次确认预告首次信任提示，运行中引导手动关闭）或「打开仓库管理器」（复制路径到剪贴板+跳转 choose-vault）。引入哨兵错误 ErrObsidianNotInstalled/ErrVaultNotRegistered/ErrObsidianRunning + app 状态码翻译 + 前端分流；obsidian.json 读取失败降级到现状尽力打开。spec 记录外部工具多状态处理与修改外部应用配置文件原子写+备份+保留未知字段模式。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f3308f1` | (see git log) |
| `d44e9d0` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 25: 使用系统默认浏览器打开http链接

**Date**: 2026-07-08
**Task**: 使用系统默认浏览器打开http链接
**Branch**: `master`

### Summary

GitInfo 仓库远程地址 http(s) 链接由内置 webview 改为系统默认浏览器打开（BrowserOpenURL）；新增 GitInfo 点击行为单测；docs/开发规范.md 沉淀外部链接打开规范。前端单测 203 passed，wails build 通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `37e3dff` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 26: 修复本地变动未跟踪目录折叠显示不完整

**Date**: 2026-07-10
**Task**: 修复本地变动未跟踪目录折叠显示不完整
**Branch**: `master`

### Summary

右栏本地变动面板对 Git 仓库显示不完整。根因：GetLocalChanges 用 git status --porcelain -z（默认 --untracked-files=normal）会把未跟踪目录折叠为单行 ?? dir/。改为追加 --untracked-files=all 展开目录内每个文件（复现仓库 all_in_ai 36→116，仍尊重 .gitignore）。顺带修复实测发现的既有 bug：git status -z 重命名格式为目标在前、源在后，原代码误用源路径覆盖目标路径导致重命名文件显示旧路径，修正后显示当前路径。补 3 个单测，trellis-check 全绿，文档无需更新。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f845d71` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 27: 文件节点刷新自动刷新其所在目录

**Date**: 2026-07-13
**Task**: 文件节点刷新自动刷新其所在目录
**Branch**: `master`

### Summary

FileTreePanel.refreshNode 命中文件节点时上溯到父目录（根下即 store.root），修复对叶子节点 expand() 无效导致刷新无反应；文件右键菜单新增「刷新 F5」项，F5 入口因逻辑内聚于 refreshNode 自动受益。新增 2 个单测，全量 210 测试通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bbd19e0` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 28: fix: 文件树点击已展开文件夹保持展开

**Date**: 2026-07-19
**Task**: fix: 文件树点击已展开文件夹保持展开
**Branch**: `master`

### Summary

优化文件树交互：已展开未选中的文件夹点击后仅选中不收起，已选中时才收起

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `cee33c3` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 29: 仓库筛选器：文件树 Git 仓库管理弹窗

**Date**: 2026-07-19
**Task**: 仓库筛选器：文件树 Git 仓库管理弹窗
**Branch**: `master`

### Summary

实现仓库筛选器弹窗（master-detail 两栏 + useVirtualList 虚拟滚动），支持工作目录切换、已编辑/未编辑 Tab 分类、标签 OR 筛选、README 智能续取摘要 + 二级弹窗渲染、跨工作目录跳转、失效仓库清理。后端 .git 预筛 + mtime 缓存扫描优化（100 仓库 <0.5s）+ go-git 并发检测远程。修复切换工作目录后旧仓库未隔离、左栏点击强制滚动、滚动条被父 Pane 裁剪等 bug。沉淀虚拟滚动滚动条陷阱到 docs/常见问题.md，Git 仓库检测规范到 docs/开发规范.md。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `889cc8c` | (see git log) |
| `9a95d82` | (see git log) |
| `6ca219c` | (see git log) |
| `3f00ba6` | (see git log) |
| `3254668` | (see git log) |
| `409fd31` | (see git log) |
| `13b8a68` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
