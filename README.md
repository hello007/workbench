# Git Manager

Git 仓库管理桌面应用，基于 Wails + Vue 3 构建，提供工作目录管理、文件浏览、文件操作和 Git 集成功能。

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
- **工作目录管理** — 左侧面板管理多个工作目录，支持添加、删除、重命名、设为默认、右键打开（资源管理器/VSCode/Warp）、批量更新仓库
- **文件树浏览** — 树形展示目录结构，文件夹优先排序，支持右键菜单操作（点击任意位置自动关闭），空白区域右键可新建文件/文件夹
- **文件操作** — 新建、重命名、删除、预览，支持在资源管理器/VSCode/Warp 中打开
- **文件预览编辑** — 预览文本文件时支持就地编辑，修改后可保存或取消，切换文件时自动检查未保存修改
- **Git 集成** — 查看提交历史、分支信息、仓库状态
- **批量更新** — 一键批量 pull 所有仓库
- **工具箱** — 左侧活动栏提供工具箱入口，将"拷贝到"等全局工具集中管理，点击其他面板自动关闭
- **内置终端** — 底部面板集成完整终端，支持 PowerShell / CMD / Git Bash / WSL 切换，工作目录自动跟随文件树，可拖拽调节高度，Ctrl+` 快速切换，首次打开即显示 Shell 提示符
- **自定义快捷键** — 支持自定义「打开命令面板（默认 Ctrl+P）」和「切换终端（默认 Ctrl+`）」快捷键，点击录制新快捷键，右键菜单显示快捷键提示，可单个或批量重置

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

构建产物位于 `build/bin/` 目录。

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
├── frontend/        # Vue 3 前端
│   └── src/
│       ├── views/Home.vue              # 上下分区布局容器 + 状态中枢
│       ├── composables/
│       │   └── useTerminal.js           # 终端逻辑（xterm + PTY 通信）
│       └── components/
│           ├── ActivityBar.vue           # 活动栏（目录/工具箱/终端切换）
│           ├── DirectoryTree.vue        # 工作目录树面板
│           ├── FileTreePanel.vue        # 文件树面板
│           ├── ContentPanel.vue         # 操作面板
│           ├── ToolboxPanel.vue         # 工具箱面板
│           ├── GitInfo.vue              # Git 仓库信息
│           ├── CommitHistory.vue        # 提交历史
│           ├── TerminalPanel.vue        # 终端面板（Shell 选择/目录显示/拖拽调高）
│           └── SettingsPanel.vue        # 设置弹窗（通用/终端/快捷键，左右双栏布局）
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
