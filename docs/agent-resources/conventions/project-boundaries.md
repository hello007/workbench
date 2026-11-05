---
title: 工程分层架构与模块边界（Go + Vue 双栈）
scope: [architecture, module, frontend, go]
targets: [vue, store, composable, transport, go, util]
---

# 工程分层架构与模块边界（Go + Vue 双栈）

> **权威声明**：本文件是 `docs/agent-resources/` 体系内「模块边界 / 依赖方向」主题的权威；本仓深度契约与踩坑例外以 `docs/spec/` 与仓根 AGENTS.md「关键规则」为准。

## 1. 目录分层架构

**Go 后端（仓根）：**

```
├── main.go / app.go        # 入口与 App 结构体（前后端桥接，含 app_*.go 按域拆分）
├── app_services.go         # service 装配（AppServices 注册点）
├── service/                # 业务逻辑层（核心领域能力）
├── server/                 # 浏览器访问通道服务层（WS/HTTP RPC）
├── model/                  # 数据模型层（Wails 绑定契约真相源）
├── util/                   # 工具层（通用能力，含跨包测试辅助 testutil）
├── data/                   # 运行时数据目录（不入库）
└── frontend/               # Vue3 前端
    └── src/
        ├── transport/      # 后端通道封装（双通道收口，纯函数无状态）
        ├── assets/         # 静态资源
        ├── components/     # 业务与通用组件
        ├── composables/    # 组合式逻辑复用
        ├── store/          # Pinia 全局状态
        ├── router/         # 路由配置
        ├── utils/          # 通用工具函数
        └── views/          # 页面视图层
```

## 2. 依赖边界铁律

**Go 侧**：
- 依赖方向单向：`app*.go`（桥接层）→ `service/` → `model/` / `util/`；`service/` 不得反向 import 主包桥接符号。
- `service/` 向前端发事件必须经 EventSink 注入接口，不得直依赖 Wails runtime。

**前端侧**：
- **单向依赖流**：`views/` 可依赖 `components/`、`composables/`、`store/`、`transport/`、`utils/`。
- **底层严禁反向依赖**：`transport/`、`utils/` 严禁依赖任何 `views/` 或 `store/`。
- **组件独立性**：`components/` 下的公共基础组件不得直接引入具体业务 Store，需通过 Props 和 Emits 保持纯粹。
- **存量例外记录**：若发现历史遗留的反向依赖债务，必须在本文件明确记录白名单并划定「禁止新增」底线，严禁做与事实相悖的绝对化断言。

## 3. 层特有约束

- `model/` 是 Wails 绑定契约真相源——字段变更必须核对前端消费面（见 `coding-api-contracts.md`）。
- `service/` 新增服务须在 `app_services.go` 装配（本仓关键规则，细节见仓根 AGENTS.md）。
- `frontend/wailsjs/` 为生成产物不入库，任何层禁止手改。
