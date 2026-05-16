---
project_name: 'git-manager'
user_name: 'Liuyang'
date: '2026-05-14'
sections_completed:
  ['technology_stack', 'language_specific_rules', 'framework_specific_rules', 'testing_rules', 'code_quality_rules', 'workflow_rules', 'critical_rules']
status: 'complete'
rule_count: 87
optimized_for_llm: true
---

# AI Agent 项目上下文

_本文件包含 AI Agent 在本项目中编写代码时必须遵循的关键规则和模式。重点关注容易被忽略的非显而易见的细节。_

---

## 技术栈与版本

### 后端 (Go)

- Go 1.24.0 | Wails v2.12.0 | go-git/v5 5.18.0
- Wails 绑定：`app.go` 中的公开方法自动生成到 `frontend/wailsjs/go/main/App.js`
- 前端资源通过 `//go:embed all:frontend/dist` 嵌入 Go 二进制
- Git 操作：go-git 用于仓库信息读取/提交历史分析，exec.Cmd("git") 用于 clone/pull
- Windows 构建标签：`//go:build windows`，需要 `HideCommandWindow(cmd)` 隐藏命令窗口
- 非 Windows 构建标签文件：`exec_other.go`，必须提供对应平台的空实现

### 前端 (Vue 3)

- Vue 3.5.33 (Composition API + `<script setup>`) | Element Plus 2.13.7
- Vue Router 4.6.4 — 必须使用 Hash 模式 (`createWebHashHistory`)，Wails 不支持 Browser History
- Vite 8.0.10 构建 | Vitest 4.1.5 + Vue Test Utils 2.4.9 测试
- 所有 Wails 绑定调用都是异步的（返回 Promise）

---

## 关键实现规则

### 语言特定规则

**Go:**

- **三层架构**：`model/`（数据结构 + 验证）→ `service/`（业务逻辑，不依赖 Wails）→ `app.go`（Wails 绑定调度层）
- **错误处理**：service 返回 error；app.go 用 `println("Error:", err.Error())` 记录后返回零值/nil
- **双 Git 引擎边界**：读操作（log/status/diff/仓库信息）优先 go-git 库；写操作（clone/pull/push）走 `exec.Cmd("git")`；同一功能内部不混用
- **exec.Cmd 规范**：必须调用 `util.HideCommandWindow(cmd)` 隐藏命令窗口；必须设置 `cmd.Dir`；必须设置超时（`context.WithTimeout`）
- **并发控制**：`sync.WaitGroup` + `chan struct{}` 信号量；goroutine 内错误通过 `sync.Mutex` 保护收集；通过 `safeEmit` 推送事件（非 Wails 上下文静默跳过）
- **JSON 序列化**：所有 model 结构体必须带 `json` 标签；配置读写统一走 `util.LoadJSON/SaveJSON`
- **路径安全**：用户输入路径必须 `filepath.Clean` 处理；敏感操作前校验路径在合法范围内
- **测试规范**：表驱动测试优先（`[]struct{ name string; ... }`）；集成测试用 `t.TempDir()` 创建真实文件系统；断言用 `t.Errorf`，格式 `"funcName(input): got %v, want %v"`

**JavaScript (Vue):**

- **Composition API**：全部使用 `<script setup>` + `ref/reactive`，不使用 Options API
- **响应式陷阱**：禁止直接解构 `reactive` 对象（丢失响应性），需用 `toRefs` 或改用 `ref`
- **Wails 调用**：导入路径 `../../wailsjs/go/main/App`（从 .vue 文件相对）；运行时 `../../wailsjs/runtime/runtime`（EventsOn/EventsOff）
- **异步操作防护**：所有 Wails 后端调用期间必须 disable 按钮/显示 loading，防止重复提交
- **事件监听清理**：`EventsOn` 注册的监听器必须在 `onBeforeUnmount` 中通过 `EventsOff` 清理；`document.addEventListener` 同理
- **错误显示**：统一用 `ElMessage.error('描述: ' + (error.message || String(error)))`；批量操作场景配 `{ duration: 3000, showClose: true }`
- **反馈分级**：轻提示 `ElMessage` | 需确认 `ElMessageBox.confirm`（文案必须说明具体后果）| 持续通知 `ElNotification`
- **右键菜单**：使用自定义 `<div>` + 绝对定位，不用 `el-dropdown`
- **路径显示**：`path.replaceAll('\\', '/')` 统一为正斜杠；路径解析兼容 `\` 和 `/`
- **键盘事件**：检查 `e.target` 是否为 input/textarea，避免在输入框中触发快捷键
- **测试 Mock**：新增 Go 绑定方法后，必须在 `frontend/src/test/setup.js` 同步添加 mock

### 框架特定规则

**Wails v2:**

- **绑定机制**：`app.go` 公开方法（首字母大写）自动生成 JS 绑定到 `wailsjs/go/main/App`；参数通过 JSON 序列化，前端调用全部返回 Promise
- **事件系统**：Go 端 `runtime.EventsEmit(ctx, "event-name", data)` → 前端 `EventsOn("event-name", callback)`
- **无 TypeScript**：前端纯 JS，`wailsjs/` 下的绑定文件是自动生成的 `.js`，不要手动编辑
- **线程模型**：Go 绑定方法在单独 goroutine 执行，前端连续调用不会排队，必须在前端做防重复控制

**Element Plus:**

- **ElTree 懒加载**：`lazy` + `:load="loadTreeNode"` 模式，`resolve(nodes)` 回调返回子节点；`node-key="path"` 用路径做唯一标识
- **ElTree 刷新**：`node.data` 修改不触发视图更新，需 `treeNode.loaded = false; treeNode.expand()` 或改变组件 `key` 强制重建
- **表单对话框**：`el-dialog` + `el-form` + `v-model` 控制显示，异步操作期间用 `loading` ref 防重复提交

**Vue 3 组件通信：**

- **Props down, Events up**：props 传递数据，`defineEmits` + `emit` 上报事件
- **defineExpose**：子组件通过 `defineExpose({ method1, method2 })` 暴露方法给父组件 `ref` 调用
- **组件 ref**：`const childRef = ref()` + template `ref="childRef"` 获取子组件实例

### 测试规则

**Go 测试：**

- 测试文件放在同包下（`*_test.go`），使用 `package xxx` 而非 `package xxx_test`
- 表驱动测试优先：`[]struct{ name string; ... }` 模式，每个用例独立可读
- 集成测试用 `t.TempDir()` 创建临时目录和真实 git 仓库（`runGit` helper 封装为 `t.Helper()`）
- 断言用 `t.Errorf`，格式：`"funcName(input): got %v, want %v"`
- 并发测试中按路径查找结果（goroutine 执行顺序不确定），不要按索引断言

**前端测试：**

- 框架：Vitest（`globals: true`）+ Vue Test Utils + jsdom 环境
- `setup.js` 全局 mock 所有 Wails 绑定和运行时；**新增 Go 绑定方法必须同步更新此文件**
- Element Plus 组件通过 mount options 的 `stubs` 替换，不做完整渲染
- `ElMessage` 等 API 通过 `vi.mock('element-plus')` 在单测文件中 mock
- 测试文件放在 `src/views/__tests__/` 或 `src/components/__tests__/` 下

### 代码质量与风格规则

**Go:**

- **命名**：结构体/方法 PascalCase，每个导出符号一行中文注释（`// Xxx 功能描述`）
- **app.go 方法注释**必须包含返回值语义（如"失败返回空切片"），这是前后端契约的唯一文档
- **app.go 方法体**控制在 ~10 行内，只做"调用 service + 错误处理"；超过此长度的逻辑或私有辅助函数应搬到 service 层
- **校验分层**：入参校验（永远非法的：空值、非正数）放 app.go；业务校验（有条件非法的：路径存在性、重复检查）放 service
- **Service 组织**：一文件一服务（`directory.go`、`git.go`），工具函数放 `util/` 按功能分文件
- **无接口抽象**：service 直接依赖 util 层具体类型，测试以集成测试为主，不 mock service 间调用
- **数据契约**：修改 model 的 `json` 标签时必须同步检查前端调用点，前后端通过 Go `json` 标签隐式定义契约
- **中文消息**：冒号后加空格（`"描述: xxx"`），与现有代码保持一致

**JavaScript (Vue):**

- **SFC 区块顺序**：`<template>` → `<script setup>` → `<style scoped>`
- **隐式风格**：2 空格缩进、单引号、无分号、字符串用 `+` 拼接（不用模板字符串）
- **代码密度**：偏好简洁链式表达（`dirs || []`），避免过度防御性判断
- **异步模式**：100% async/await，不用 `.then/.catch`
- **依赖管理**：不引入新 npm 依赖，用原生 JS 解决问题
- **颜色**：使用 Element Plus 标准色值（`#409EFF` 主色、`#67C23A` 成功、`#909399` 灰色、`#E6F7FF` 浅蓝、`#F5F7FA` 灰白），优先用 CSS 变量
- **不引入**：TypeScript、CSS 框架（Tailwind/UnoCSS）、ESLint/Prettier 配置文件
- **数据契约**：前端解析 Wails 返回值前，必须参考 Go 端对应方法确认字段结构，不靠猜测
- **右键菜单**：`position: fixed` + `z-index >= 2000` + DOM 留在组件内部（保证 scoped 样式生效）
- **匹配现有风格**：新增代码遵循相邻代码写法，不做"顺手改进"

### 开发工作流规则

**Git 提交：**

- 格式：`type: 中文描述`（feat/chore/fix/refactor/docs）
- 单 master 分支直接开发，无 PR 流程
- 每个功能完成后确认是否需要更新 README.md（CLAUDE.md 项目要求）

**构建与测试：**

- 开发：`wails dev`（前后端热重载）
- 后端测试：`go test ./...`
- 前端测试：`cd frontend && npm test`
- 构建：`wails build`（输出到 `build/` 目录）

**不提交的文件：**

- `data/` — 运行时数据（directories.json 等）
- `frontend/wailsjs/` — Wails 自动生成的绑定文件（`wails dev` 时自动生成）

### 关键禁止规则

**架构级禁止：**

- **禁止**手动编辑 `frontend/wailsjs/` 下的文件 — Wails 自动生成，`wails dev` 会覆盖
- **禁止**在 service 层导入 `github.com/wailsapp/wails/v2` — service 应独立于 Wails，只有 app.go 可依赖
- **禁止**在同一功能中混用 go-git 和 exec.Cmd 两个 Git 引擎 — 状态可能不一致
- **禁止**直接调用 `runtime.EventsEmit` — 必须通过 `safeEmit` 包装，防止非 Wails 上下文崩溃
- **禁止**在 app.go 写超过 ~10 行的私有辅助函数 — 逻辑应搬到 service 层

**前端禁止：**

- **禁止**遗忘 `EventsOff` 清理 — `EventsOn` 必须在 `onBeforeUnmount` 中配对清理，否则内存泄漏
- **禁止**用 `el-dropdown` 替代自定义右键菜单 — 不支持 `contextmenu` 事件
- **禁止**直接解构 `reactive` 对象 — 丢失响应性，必须用 `toRefs`
- **禁止**引入 TypeScript、CSS 框架（Tailwind/UnoCSS）、新 npm 依赖 — 项目约定纯 JS + 手写 CSS
- **禁止**异步操作期间不 disable 按钮 — Wails 调用不排队，重复点击会触发并发操作

**平台禁止：**

- **禁止**在 Windows 上调用 `exec.Cmd` 时遗漏 `HideCommandWindow(cmd)` — 否则弹出黑色命令窗口
- **禁止**删除 `exec_other.go` — 跨平台构建需要非 Windows 平台的空实现文件

---

## 使用指南

**AI Agent 使用：**

- 实现代码前必须阅读本文件
- 严格遵循所有规则，不要"灵活变通"
- 遇到规则冲突时，选择更保守的选项
- 关键禁止规则是底线，优先于其他规则

**维护指南：**

- 技术栈变更时更新版本号
- 新增架构模式时补充对应规则
- 定期审查：移除已过时的规则，合并重复项
- 保持精简：每条规则必须提供独特价值

**最后更新：** 2026-05-14
