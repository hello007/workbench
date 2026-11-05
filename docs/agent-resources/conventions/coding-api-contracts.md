---
title: 前后端接口契约规范（Wails 绑定与数据模型）
scope: [api, frontend, contract, go]
targets: [api, transport, go]
---

# 前后端接口契约规范（Wails 绑定与数据模型）

## 1. 契约组织结构

- **前后端调用统一收口**：前端所有对后端 Go 方法的调用统一经 `frontend/src/transport/` 封装层，禁止在视图组件中直接调用 `wailsjs/go/main/App` 生成方法。
- **契约的真相源在 Go 侧**：`app*.go` 中 App 结构体的导出方法签名与 `model/` 导出 struct 即接口契约；`frontend/wailsjs/`（`App.js` / `App.d.ts` / `models.ts`）是由 `wails generate module` 生成的绑定产物，**不入库**，禁止手改。
- 按业务域分文件组织 Go 桥接层，如 `app_git.go`、`app_terminal.go`、`app_assets.go`。

## 2. 契约同步规则（跨层签名一致）

- 修改 App 方法签名或 `model/` 导出 struct 字段时，必须核对前端消费面，确认绑定调用点（transport 封装与组件层）的参数、返回值结构与新契约一致。
- 传递给 Wails 绑定的自定义具名类型（如 `type X string`）在 JS 侧以基础类型呈现，前端按基础类型消费，禁止假设存在同名包装类型。
- 返回给前端的结构体字段命名以 Go 导出字段的 JSON 序列化结果为准；前端消费字段名必须与真实序列化形状一致，禁止凭记忆臆造字段。

## 3. 数据结构与字段防御

- 后端返回的列表/分页结构，前端消费前必须判空（空数组、null 字段），禁止直接下标访问未校验的返回值。
- 后端新增返回字段对前端是增量兼容；前端不得依赖未确认存在的字段——先在调用点确认后端已返回再消费。
- 二进制数据经 base64 传输时，前端按约定解码（含 UTF-8 多字节字符时禁止裸 `atob`），并在契约注释中标注编码方式。
