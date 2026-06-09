# Git Manager 项目

> 所有文档一律使用中文描述，且每次功能完成后都需要确认是否需要更新 README.md

## 项目概述

**项目名称：** Git Manager - Git仓库管理桌面应用
**项目路径：** `d:\workspace\workspace_ai\demo_OpenSpec\git_tools\git-manager\`
**Git 仓库：** `d:\workspace\workspace_ai\demo_OpenSpec\git_tools\git-manager\`
**开始时间：** 2026-04-28
**当前状态：** 活跃开发中（v1.0.6+，持续迭代新功能）

## 技术栈

|层级|技术|版本|
|---|---|---|
|后端语言|Go|1.26.2|
|桌面框架|Wails|v2.12.0|
|前端框架|Vue 3 (Composition API)|3.5.33|
|UI组件|Element Plus|2.13.7|
|路由|Vue Router|4.6.4|
|构建工具|Vite|8.0.10|
|后端测试|Go testing|-|
|前端测试|Vitest + Vue Test Utils|-|

## 项目结构

```text
git-manager/
├── main.go              # 主入口
├── app.go               # 应用结构体（前后端桥接）
├── model/               # 数据模型层
├── service/             # 业务逻辑层
├── util/                # 工具层
├── data/                # 数据配置（不提交Git）
├── frontend/            # Vue3前端
├── build/               # 构建输出
├── wails.json           # Wails配置
├── DEVELOPMENT.md       # 开发运维文档
└── BUILD_SUMMARY.md     # 构建摘要
```

## 文档索引

|文档|说明|
|---|---|
|[功能说明.md](docs/功能说明.md)|工作目录管理、文件树、文件操作、Git集成、导航中心、终端、快捷键|
|[开发工作流.md](docs/开发工作流.md)|启动开发、运行测试、构建、运行应用|
|[测试策略.md](docs/测试策略.md)|单元测试、集成测试、测试覆盖|
|[部署说明.md](docs/部署说明.md)|生产构建、分发、配置文件|
|[开发规范.md](docs/开发规范.md)|代码风格、调试、错误处理、提交规范|
|[常见问题.md](docs/常见问题.md)|常见问题|
|[路线图.md](docs/路线图.md)|发展路线图|
|[项目上下文.md](docs/project-context.md)|AI Agent 编码规则和模式|
|[开发运维.md](git-manager/DEVELOPMENT.md)|开发运维详细文档|
|[构建摘要.md](git-manager/BUILD_SUMMARY.md)|构建摘要|

## 常用命令

|操作|命令|
|---|---|
|开发调试|`wails dev`|
|构建应用|`wails build`|
|后端测试|`go test ./...`|
|前端测试|`cd frontend && npm test`|
|安装依赖|`cd frontend && npm install`|
|查看端口|`netstat -ano \| findstr ":34115"`|
|停止进程|`taskkill /F /IM git-manager.exe`|

---

**最后更新：** 2026-06-09
**文档版本：** v2.2
