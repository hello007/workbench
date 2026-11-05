---
title: 前端异常捕获与容灾规范
scope: [exception, error-handling, frontend]
targets: [api, vue, store, composable]
---

# 前端异常捕获与容灾规范

## 1. 错误处理三道防线

1. **第一道：全局异常捕获**
   - `app.config.errorHandler`：捕获 Vue 组件生命周期与渲染抛出的未捕获错误。
   - `window.addEventListener('unhandledrejection')`：兜底未 catch 的异步 Promise 拒绝。
2. **第二道：通道层统一拦截**
   - 后端调用错误在通道包装层统一解析结构化错误（错误码 + 消息），转换为友好的中文提示信息展示给用户；禁止让原始 Error 对象直接透传到 UI。
   - Wails 运行时可能把结构化错误压平为字符串，包装层必须恢复结构化语义后再分流。
3. **第三道：业务局部降级**
   - 关键业务操作（如文件删除、批量操作）必须在业务代码中使用 `try-catch` 精准捕获并提供界面降级交互。

## 2. 严禁静默吞异常

- 禁止书写空的 `catch(e) {}` 语句块，所有异常要么向用户提示，要么输出结构化日志。
- 按错误码分流处理时，未知错误码必须有兜底分支，禁止只处理已知码而遗漏其余。
