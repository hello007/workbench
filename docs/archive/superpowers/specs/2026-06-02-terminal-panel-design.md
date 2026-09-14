# 内置终端面板设计

> **文档类型**：功能设计规格
> **日期**：2026-06-02
> **版本**：v1.0
> **状态**：已评审

---

## 目录

1. [摘要](#摘要)
2. [需求概述](#需求概述)
3. [整体架构](#整体架构)
4. [后端架构](#后端架构)
5. [前端架构](#前端架构)
6. [设置与错误处理](#设置与错误处理)
7. [测试策略](#测试策略)
8. [总结](#总结)

---

## 摘要

为 WorkBench 添加内置终端面板功能，采用 **xterm.js + Go PTY** 方案，在应用底部嵌入一个完整的终端实例。终端工作目录自动跟随文件树选中项，支持多种 Shell 类型配置，面板可拖拽调节高度、可收起/展开。此功能将 WorkBench 从单一的 Git 仓库管理工具向开发者效率平台方向扩展。

---

## 需求概述

| 需求项 | 说明 |
|--------|------|
| 内置完整终端 | 在应用内嵌入一个完整的终端窗口，可执行任意命令 |
| 单终端实例 | 只显示一个终端，保持界面简洁 |
| 底部面板布局 | 类似 VS Code，终端面板位于底部，不挤占文件树和内容区 |
| 目录自动跟随 | 终端工作目录自动跟随左侧文件树选中的目录 |
| Shell 类型可配置 | 支持 PowerShell / CMD / Git Bash / WSL 等多种 Shell |
| 面板可拖拽 | 用户可拖拽调节终端面板高度 |
| 面板可收起/展开 | 通过快捷键或按钮切换终端面板显隐 |

---

## 整体架构

### 布局变化

**当前布局：**

```
┌──────────┬────────────────────────────────────────┐
│          │ DirectoryTree │ FileTree │  Content     │
│ Activity │    (20%)      │  (30%)   │  (50%)       │
│   Bar    │              │          │               │
│          │              │          │               │
└──────────┴────────────────────────────────────────┘
```

**新增终端后布局：**

```
┌──────────┬────────────────────────────────────────┐
│          │ DirectoryTree │ FileTree │  Content     │
│ Activity │    (20%)      │  (30%)   │  (50%)       │
│   Bar    │              │          │               │
├──────────┴────────────────────────────────────────┤
│  Terminal Panel（可拖拽调节高度，可收起/展开）       │
└───────────────────────────────────────────────────┘
```

### 数据流

```
用户点击文件树节点
    │
    ▼
Home.vue 更新 selectedDirectoryId / selectedNode
    │
    ▼
TerminalPanel.vue 监听路径变化
    │
    ▼
调用 Go 后端 ChangeTerminalDir(newPath)
    │
    ▼
Go 后端向 PTY 进程发送 cd 命令
```

### 前后端通信

| 方向 | 机制 | 数据 |
|------|------|------|
| 前端 → 后端 | Wails Bindings（Go 方法调用） | 创建终端、切换目录、调整大小、销毁终端 |
| 后端 → 前端 | Wails Events（事件推送） | PTY 输出数据流 |
| 前端 → 后端 | Wails Bindings | 用户键盘输入 |

---

## 后端架构

### 新增文件结构

```
workbench/
├── service/
│   └── terminal.go          # 终端服务（PTY 管理）
├── util/
│   └── pty.go               # PTY 底层封装（平台相关）
├── model/
│   └── terminal.go           # 终端相关数据模型
```

### 核心模型（model/terminal.go）

| 结构体 | 字段 | 说明 |
|--------|------|------|
| `TerminalSession` | `id string` | 终端会话唯一标识 |
| | `pty *os.File` | PTY 文件描述符 |
| | `cmd *exec.Cmd` | Shell 子进程 |
| | `dir string` | 当前工作目录 |
| | `shellType string` | Shell 类型（powershell/cmd/gitbash/wsl） |
| | `running bool` | 运行状态 |

### 终端服务（service/terminal.go）

| 方法 | 签名 | 说明 |
|------|------|------|
| `CreateTerminal` | `(dir, shellType string) → sessionID` | 创建 PTY 进程，返回会话 ID |
| `WriteInput` | `(sessionID, input string) → error` | 向 PTY 写入用户输入 |
| `ChangeDir` | `(sessionID, dir string) → error` | 向 PTY 发送 cd 命令切换目录 |
| `Resize` | `(sessionID string, rows, cols uint16) → error` | 调整 PTY 窗口大小 |
| `CloseTerminal` | `(sessionID string) → error` | 关闭 PTY 进程 |
| `startOutputPump` | `(sessionID string)` | 后台 goroutine，持续读取 PTY 输出并通过 Events 推送到前端 |

### PTY 工具层（util/pty.go）

平台适配策略：

| 平台 | 实现 | 说明 |
|------|------|------|
| Windows | `ConPTY` API | 使用 `github.com/iamacarpet/go-winpty` 或直接调用 Windows ConPTY API |
| Linux/macOS | `github.com/creack/pty` | 标准 Unix PTY |

Shell 启动路径配置：

| shellType | Windows 路径 | 说明 |
|-----------|-------------|------|
| `powershell` | `powershell.exe` / `pwsh.exe` | 默认 |
| `cmd` | `cmd.exe` | 传统命令行 |
| `gitbash` | `C:\Program Files\Git\bin\bash.exe` | Git Bash |
| `wsl` | `wsl.exe` | Windows Subsystem for Linux |

### 生命周期

```
应用启动 → 不创建终端（按需创建）
用户点击打开终端 → CreateTerminal → PTY 进程启动 → 输出泵运行
用户切换目录 → ChangeDir → 向 PTY 发送 cd 命令
用户调整面板 → Resize → 通知 PTY 窗口大小变化
用户关闭终端 → CloseTerminal → PTY 进程退出，资源释放
应用退出 → shutdown 中 CloseTerminal → 确保清理
```

---

## 前端架构

### 新增文件结构

```
frontend/src/
├── components/
│   └── TerminalPanel.vue      # 终端面板组件（xterm.js 渲染）
├── composables/
│   └── useTerminal.js         # 终端逻辑复用（PTY 通信、事件监听）
```

### TerminalPanel.vue 职责

| 职责 | 说明 |
|------|------|
| xterm.js 实例管理 | 初始化、挂载、销毁 |
| 输入转发 | 监听 xterm `onData` 事件，调用 Go `WriteInput` |
| 输出渲染 | 监听 Wails Events `terminal-output`，写入 xterm |
| 面板尺寸 | 监听容器 resize，调用 Go `Resize` + xterm `fitAddon` |
| 目录跟随 | 监听 props `currentDir`，调用 Go `ChangeDir` |
| 面板显隐 | 通过 props `visible` 控制展开/收起 |

### useTerminal.js 组合式函数

核心接口：

```javascript
{
  initTerminal,      // 创建 xterm 实例 + Go PTY 会话
  destroyTerminal,   // 销毁 xterm 实例 + 关闭 PTY 会话
  changeDir,         // 切换终端工作目录
  isActive,          // 终端是否运行中
}
```

### Home.vue 布局改造

当前布局是水平 Splitpanes 三栏，改造为上下两部分：

```
┌─────────────────────────────────────────────────┐
│  上部：原有 Splitpanes 三栏（flex: 1）           │
│  ActivityBar | DirTree | FileTree | Content      │
├─────────────────────────────────────────────────┤
│  拖拽分隔条（1px，同现有 Splitpanes 风格）        │
├─────────────────────────────────────────────────┤
│  下部：TerminalPanel（默认 200px，可拖拽）        │
│  ┌──────────────────────────────────────────┐   │
│  │ 工具栏：Shell类型选择 | 收起按钮          │   │
│  ├──────────────────────────────────────────┤   │
│  │ xterm.js 终端区域                        │   │
│  └──────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
```

**实现方式**：不使用 Splitpanes 做上下分割（Splitpanes 嵌套会增加复杂度），改为纯 CSS flex 布局 + 自定义拖拽分隔条，与现有代码风格一致。

### 终端显隐交互

| 操作 | 效果 |
|------|------|
| `Ctrl+`` | 切换终端面板显隐 |
| 终端面板右上角 `×` 按钮 | 收起终端面板 |
| ActivityBar 终端图标 | 切换终端面板显隐 |
| 收起时 | PTY 进程保持运行，面板隐藏 |
| 展开时 | 恢复显示，无需重新创建 PTY |

### 前端依赖新增

| 依赖 | 版本 | 用途 |
|------|------|------|
| `xterm` | ^5.x | 终端渲染核心 |
| `xterm-addon-fit` | ^0.8.x | 自适应容器尺寸 |
| `xterm-addon-web-links` | ^0.9.x | 可点击链接识别 |

---

## 设置与错误处理

### Shell 类型设置

在现有 `SettingsPanel` 中新增终端配置区域：

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `defaultShell` | 下拉选择 | `powershell` | 默认 Shell 类型 |
| `gitBashPath` | 文本输入 | `C:\Program Files\Git\bin\bash.exe` | Git Bash 路径（仅 shellType=gitbash 时显示） |
| `wslDistro` | 文本输入 | 空（默认发行版） | WSL 发行版名称（仅 shellType=wsl 时显示） |

存储在现有 `data/settings.json` 中，随应用持久化。

### 终端面板工具栏

```
┌─────────────────────────────────────────────────┐
│ ⚡ PowerShell ▾  │  D:\workspace\workspace_ai    │  ─  × │
└─────────────────────────────────────────────────┘
```

| 元素 | 说明 |
|------|------|
| Shell 类型下拉 | 运行中可切换（切换时销毁旧 PTY，创建新 PTY） |
| 当前目录显示 | 只读，显示终端当前工作目录 |
| 收起按钮 `─` | 收起面板，PTY 保持运行 |
| 收起按钮 `×` | 收起面板，PTY 保持运行（不提供单独的销毁终端按钮，仅在切换 Shell 类型时销毁旧 PTY） |

### 错误处理

| 场景 | 处理方式 |
|------|---------|
| Shell 进程意外退出 | 前端显示退出提示，提供「重新启动」按钮 |
| PTY 创建失败（Shell 不存在） | ElMessage 错误提示，面板显示错误信息 |
| 目录切换失败 | 终端内自然显示 cd 错误，不额外弹窗 |
| 终端输出泵 goroutine 异常 | 记录日志，标记 session 为非运行状态，前端显示断开提示 |
| 应用退出时 PTY 未关闭 | `shutdown` 中遍历所有 session 调用 `CloseTerminal`，设置 3 秒超时 |

---

## 测试策略

| 层级 | 测试内容 |
|------|---------|
| 后端单元测试 | `TerminalService` 创建/写入/调整大小/关闭逻辑 |
| 后端单元测试 | Shell 路径解析（各 shellType → 可执行文件路径） |
| 后端单元测试 | 目录跟随 cd 命令生成 |
| 前端组件测试 | `TerminalPanel` 挂载/销毁、显隐切换 |
| 前端组件测试 | `useTerminal` 组合式函数的事件监听和调用 |
| 集成测试 | 端到端：创建终端 → 输入命令 → 验证输出 |

---

## 总结

本设计为 WorkBench 新增内置终端面板功能，采用 **xterm.js + Go PTY** 技术方案，核心特性包括：

1. **底部面板布局**，可拖拽调节高度，可收起/展开
2. **目录自动跟随**文件树选中项
3. **Shell 类型可配置**，支持 PowerShell / CMD / Git Bash / WSL
4. **单终端实例**，保持界面简洁
5. **完善的生命周期管理**，按需创建、收起不销毁、退出时清理

后端新增 3 个文件（model/terminal.go、service/terminal.go、util/pty.go），前端新增 2 个文件（TerminalPanel.vue、useTerminal.js），改造 Home.vue 布局结构。Windows 平台使用 ConPTY API，Linux/macOS 使用 creack/pty，确保跨平台兼容性。
