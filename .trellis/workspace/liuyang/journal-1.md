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
