# 系统架构参考（Wails 双栈桌面应用）

> 本文档描述项目架构现状，**内容易腐**：引用具体数字与清单前先核对代码。

## 1. 架构总览

本项目（WorkBench）是基于 **Wails v2.16** 的桌面开发者工作台：Go 1.26 后端 + Vue 3 前端，前后端同仓，经 Wails 桥接通信；另支持浏览器访问模式（WS + HTTP RPC 通道，`web_serve.go` / `server/`）。

- 交付形态：Windows/Linux 桌面可执行文件（`wails build`，Linux 需 `-tags webkit2_41`）
- 主语言：Go（后端）、JavaScript（前端，非 TypeScript）

## 2. 目录分层职责

**Go 后端：**

| 目录 | 职责 |
|------|------|
| `main.go` / `app.go` | 入口与 App 结构体；`app_*.go` 按域拆分桥接方法（git/terminal/search/assets 等） |
| `app_services.go` | AppServices 装配：service 注册与构造 |
| `service/` | 业务逻辑层（git 操作、终端会话、素材管理、AI 任务等） |
| `server/` | 浏览器访问通道服务层（WSHub、RPC、认证） |
| `model/` | 数据模型（Wails 绑定契约真相源） |
| `util/` | 工具层（原子写、git 命令封装、跨包测试辅助 `testutil`） |
| `scripts/` | 构建/发版/基线脚本（release.sh、coverage-check.sh、check-dep-consistency.mjs） |

**前端（`frontend/src/`）：**

| 目录 | 职责 |
|------|------|
| `transport/` | 后端通道封装（桌面 wailsjs / 浏览器 RPC 双通道，启动前安装） |
| `components/` | 业务与通用组件 |
| `composables/` | 组合式逻辑复用 |
| `store/` | Pinia 全局状态 |
| `router/` | 路由 |
| `utils/` | 通用工具 |
| `views/` | 页面视图 |
| `test/` | 测试辅助（setup.js、wails-mock-defaults.js 单一 mock 数据源） |

**生成产物（不入库）：** `frontend/wailsjs/`（Wails 绑定）、`frontend/dist/`（vite 构建产物，`//go:embed` 消费）。

## 3. 通信与数据流向

```
Vue 组件 → transport（双通道收口）
   ├─ 桌面模式：window.go.main.App（Wails 绑定，wailsjs 生成）
   └─ 浏览器模式：WebSocket + HTTP RPC（server/ 通道，token 认证）
        ↓
Go App 桥接层（app_*.go）→ AppServices → service/ 业务层
        ↓
事件回推：service 经 EventSink → Wails EventsEmit / WS 广播 → 前端 EventsOn
```

- 前端组件不直连通道实现，统一经 transport；事件监听持闭包精准注销。
- 后端 service 发事件必须走 EventSink 注入接口，不直依赖 Wails runtime。
- 错误链路：后端结构化错误（code + message）→ ErrorFormatter → 前端按 code 分流。

## 4. 构建拓扑（命令前置链）

```
干净检出
  → wails generate module      # 生成 frontend/wailsjs/
  → 预置 frontend/dist/ 占位   # 满足 //go:embed all:frontend/dist
  → go build / vet / test / vite build 才可执行
```

CI（`.github/workflows/ci.yml`）步骤顺序即标准前置链；release.yml 双 job（Windows + Linux webkit2_41）发版。

详细分层规则见 `docs/agent-resources/conventions/project-boundaries.md`；深度契约见 `docs/spec/`。
