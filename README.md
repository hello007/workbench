# WorkBench

开发者工作台，基于 Wails + Vue 3 构建，提供工作目录管理、文件浏览、文件操作和 Git 集成功能。

## 技术栈

| 层级 | 技术 | 版本 |
| ---- | ---- | ---- |
| 后端 | Go | 1.26.2 |
| 桌面框架 | Wails | v2.12.0 |
| 前端 | Vue 3 (Composition API) | 3.5.33 |
| UI 组件 | Element Plus | 2.13.7 |
| 路由 | Vue Router | 4.6.4 |
| 构建工具 | Vite | 8.0.10 |

## 功能

- **三列布局** — 工作目录树 | 文件树 | 操作面板，信息层次清晰
- **工作目录管理** — 左侧面板管理多个工作目录，支持添加、删除（Delete）、重命名（F2）、设为默认、复制路径、右键打开（资源管理器/VSCode/Warp/Obsidian）、批量更新仓库
- **文件树浏览** — 树形展示目录结构，文件夹优先排序，支持右键菜单操作（点击任意位置自动关闭），空白区域右键可新建文件/文件夹
- **文件操作** — 新建、重命名、删除、预览，支持在资源管理器/VSCode/Warp/Obsidian 中打开
- **文件预览编辑** — 预览文本文件时支持就地编辑，修改后可保存或取消，切换文件时自动检查未保存修改
- **文件类型预览** — 在操作面板内按文件类型直接预览，无需外部程序：
  - **图片**（jpg/jpeg/png/bmp/gif/webp）base64 内嵌显示，支持缩放
  - **文本**（txt/json/sql/md、各类代码）CodeMirror 6 只读高亮（行号、折叠、虚拟滚动）；Markdown 用 markdown-it 渲染（关闭原始 HTML 防 XSS）；GBK 编码文件自动解码为 UTF-8 显示
  - **Office** — Word `.docx` 用 docx-preview 内嵌渲染；Excel `.xlsx/.xls/.csv` 用 SheetJS 解析为只读表格（多 Sheet 标签页）
  - **PDF** — 内嵌预览（pdfjs 官方 viewer，工具栏支持翻页/缩放/搜索/缩略图）。通过 iframe 加载内嵌 viewer 静态资源、后端 `AssetServer` handler 以同源 URL 提供本地 PDF 字节（支持 HTTP Range，大 PDF 按需读取）；主页面不引入 pdfjs 库，靠 iframe 独立 browsing context 从架构上规避前端 pdfjs 双实例问题
  - **文本类「编辑」模式** — 文本类预览可一键切回就地编辑，保存复用既有 SaveFile 链路，按原文件编码（UTF-8/GBK）写入不改变原编码
  - **预览区复制选中文本** — 文本/代码/Markdown 预览态下，鼠标选中内容可直接 Ctrl+C 复制；也可右键弹出「复制 / 全选」菜单（复制项在无选中文本时禁用，全选用 CodeMirror 命令或 Selection API 实现）
  - **Markdown 链接导航** — 预览态点击 Markdown 内链接：相对引用（`./other.md`、`../readme.md`）在预览面板内切换预览；外部 http 链接用系统默认浏览器打开；同文档锚点（`#标题`）滚动定位。拦截 `<a>` 原生顶层导航，避免误触后端 `AssetServer` fallback 返回 `{"error":"缺少 path 参数"}` 并导致界面崩溃
  - **Markdown frontmatter 属性面板** — 文档开头的 YAML frontmatter（`---\n...\n---`）不再被当作普通正文渲染（`---` 变 `<hr>`、`key: value` 变段落文本），而是解析为正文上方的结构化属性表格：数组值显示为标签徽章，标量值原样展示，嵌套对象以 JSON 文本呈现；解析失败时降级为带语法高亮的 YAML 原文代码块，不影响正文
  - **预览历史回退** — Markdown 相对链接跳转后，预览头部的「后退」按钮可回到上一个预览文件；点击文件树节点（含同节点再点）始终重新加载预览，避免「链接跳转后预览体与选中节点脱钩导致空白」
  - **unsupported 按文本预览** — 无扩展名或未知扩展名的文件优先尝试按文本读取（UTF-8 优先 + GBK 兜底解码），可显示则降级为文本预览（可编辑保存）；含 NUL 字节或解码失败的二进制文件、超大文件（>1MB）走降级提示
  - **降级为「用默认程序打开」** — PowerPoint(`.pptx`)、旧版 Office(`.doc/.ppt`)、损坏/超大/不支持的类型，统一提供「用默认程序打开」按钮走系统默认程序
- **用 Obsidian 打开** — 工作目录树/文件树右键菜单及操作面板「查看操作」均支持以 Obsidian 打开：文件夹以自身作为仓库（vault）、文件以父目录作为仓库；可在「设置 → 通用 → 外部应用」自定义 Obsidian 程序路径（配置后优先使用，否则走 `obsidian://` 协议 + 注册表预检 + `cmd /c start`，未检测到时引导用户配置或安装；打开前读取 Obsidian 仓库注册表（%APPDATA%\obsidian\obsidian.json）判断目录是否属于已注册 vault，未注册时弹确认框引导跳转 Obsidian 仓库管理器手动添加（避免 Obsidian 弹出 "Vault not found"），注册表读取失败则降级为现状尽力打开）
- **Git 集成** — 查看提交历史、分支信息、仓库状态；双击变动文件弹窗双栏对照查看 diff、选择性提交（commit）、推送到远程（push）；选中 git 工作目录即可在右栏直接查看仓库详情，左栏工作目录列表标记 git 仓库
- **批量更新** — 一键批量 pull 所有仓库，自动跳过未配置远程的本地仓库
- **工具箱** — 左侧活动栏提供工具箱入口，将"拷贝到"等全局工具集中管理，点击其他面板自动关闭
- **内置终端** — 底部面板集成完整终端，支持 PowerShell / CMD / Git Bash / WSL 切换，工作目录自动跟随文件树，可拖拽调节高度，Ctrl+` 快速切换，首次打开即显示 Shell 提示符
- **自定义快捷键** — 支持自定义「打开命令面板（默认 Ctrl+P）」「切换终端（默认 Ctrl+`）」「重命名（默认 F2）」「删除（默认 Delete）」快捷键，点击录制新快捷键（支持组合键与 F1-F12/Delete 等功能键单键），右键菜单显示快捷键提示，F2/Del 作用于当前选中节点（输入框/对话框/终端聚焦时不触发），可单个或批量重置
- **检查更新** — 设置面板显示当前版本号，一键检查 GitHub Releases 新版本，下载进度实时显示，支持取消下载；更新完成后提示重启，用户可选择立即重启或下次启动时自动替换

## 快速开始

### 环境要求

- Go 1.26+
- Node.js 18+
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2

### 开发

```bash
# 安装前端依赖
cd frontend && npm install

# 启动开发模式（热重载）
wails dev
```

### 构建

```bash
wails build
```

构建产物位于 `build/bin/` 目录，`workbench.exe` 约 33MB（含 pdfjs viewer 静态资源）。

### 发版

#### 一键发版（推荐）

使用 `scripts/release.sh` 一键完成「读取当前版本 → 递增 → 改写 `wails.json` → 提交 → 打 tag → 推送」，推送后由 CI 自动构建发布：

```bash
# 预览计划（不执行任何写操作）
./scripts/release.sh --dry-run --bump patch --allow-dirty

# 次版本递增并直接推送（如 1.0.9 -> 1.1.0）
./scripts/release.sh --bump minor --yes

# 手动指定版本号
./scripts/release.sh --version 1.2.3 --yes
```

也可对 Claude 说「发版」，或使用 `/release` 触发智能推荐流程：skill 会按 `$LAST_TAG..HEAD` 范围内的提交类型（`major:` / `feat:` / `fix:` 等）推荐 `patch` / `minor` / `major` 级别，确认后自动调用脚本。

#### GitHub Actions 自动打包发版（底层机制）

项目使用 GitHub Actions 自动打包发版。推送 `v*` 格式的 tag 即可触发：

```bash
# 创建并推送 tag（例如 v1.0.8）
git tag v1.0.8
git push origin v1.0.8
```

流水线会自动完成以下步骤：
1. 在 Windows runner 上安装 Go 1.24 + Node.js 20 + Wails CLI
2. 执行 `wails build`（自动注入版本号和构建时间）
3. 创建 GitHub Release 并上传 `workbench.exe`

> **CI 机制**：远程 `origin` 为 Gitee，是唯一主仓库；推送后 Gitee 会自动镜像到 GitHub，`release.yml` 据此自动构建发布，无需在本仓库配置 github remote，也无需手动同步 tag 到 GitHub。

### 测试

```bash
# 后端测试
go test ./...

# 前端测试
cd frontend && npm test
```

## 项目结构

```text
├── main.go          # 主入口
├── app.go           # 应用结构体，前后端桥接
├── model/           # 数据模型层
├── service/         # 业务逻辑层
├── util/            # 工具层
├── server/          # PDF 预览 AssetServer handler（/preview-pdf 同源提供本地 PDF，支持 Range，路径安全校验）
├── frontend/        # Vue 3 前端
│   ├── public/
│   │   └── pdfjs-viewer/          # pdfjs 官方 viewer v4.8.69 静态资源（web/+build/+locale 中英），iframe 加载，go:embed 打包
│   └── src/
│       ├── views/Home.vue              # 上下分区布局容器 + 状态中枢
│       ├── composables/
│       │   ├── useTerminal.js           # 终端逻辑（xterm + PTY 通信）
│       │   ├── useCommandPalette.js     # 命令面板搜索/导航
│       │   ├── useFavorites.js          # 收藏夹管理
│       │   ├── useRecentAccess.js       # 最近访问历史
│       │   ├── useTreeState.js          # 文件树状态持久化
│       │   └── useShortcuts.js          # 快捷键解析与匹配
│       └── components/
│           ├── ActivityBar.vue           # 活动栏（目录/工具箱/终端切换）
│           ├── DirectoryTree.vue        # 工作目录树面板
│           ├── FileTreePanel.vue        # 文件树面板
│           ├── ContentPanel.vue         # 操作面板（含文件预览编辑）
│           ├── CommandPalette.vue       # 命令面板（搜索/收藏/切换目录）
│           ├── ToolboxPanel.vue         # 工具箱面板
│           ├── GitInfo.vue              # Git 仓库信息
│           ├── CommitHistory.vue        # 提交历史
│           ├── LocalChanges.vue         # 本地变更列表
│           ├── FileDiffDialog.vue       # 变动文件双栏 diff 弹窗
│           ├── TerminalPanel.vue        # 终端面板（Shell 选择/目录显示/拖拽调高）
│           ├── SettingsPanel.vue        # 设置弹窗（通用/终端/快捷键，左右双栏布局）
│           └── UpdateDialog.vue         # 更新弹窗（版本信息/下载进度/确认重启）
├── data/            # 运行时数据（不提交）
├── docs/            # 项目文档
└── wails.json       # Wails 配置
```

## 文档

| 文档 | 说明 |
| ---- | ---- |
| [功能说明](docs/功能说明.md) | 功能详情 |
| [开发工作流](docs/开发工作流.md) | 开发、测试、构建流程 |
| [测试策略](docs/测试策略.md) | 测试规范 |
| [部署说明](docs/部署说明.md) | 生产构建与分发 |
| [开发规范](docs/开发规范.md) | 代码风格与提交规范 |
| [路线图](docs/路线图.md) | 发展规划 |

## License

MIT
