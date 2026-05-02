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

- **工作目录管理** — 添加、编辑、删除 Git 仓库工作目录
- **文件树浏览** — 树形展示目录结构，文件夹优先排序
- **文件操作** — 右键菜单支持打开、复制、删除等操作
- **Git 集成** — 查看提交历史、分支信息、仓库状态
- **批量更新** — 一键批量 pull 所有仓库

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
