# 终端多会话契约（多 tab / 全屏 / 外观设置）

> 适用范围：底部终端面板的多 tab 会话、整窗全屏、终端外观设置三条链路的前后端契约。
> 来源：任务 `.trellis/tasks/09-22-terminal-multi-session-and-ui`（PR1-PR5，2026-09-22）。
> 关联：[cross-layer-contracts.md](cross-layer-contracts.md)（wailsjs 同步、EventsOn 闭包注销）、[design-tokens.md](design-tokens.md)（`--terminal-bg` 契约）。

## 1. xterm.js 多实例契约

| 契约 | 内容 | 违反后果 |
|---|---|---|
| 禁重挂载 | tab 切换用 `v-show` 保 DOM，xterm 实例与容器一一对应，禁 `v-if` 卸载/重挂载/搬移 DOM 节点 | 会话现场丢失（xterm buffer 不可迁移） |
| 隐藏容器 fit 静默失败 | display:none 下 `fitAddon.fit()` 不报错但尺寸算错（xterm 官方 issue #3029/#664，closed as designed） | 切回 tab 后行列错乱 |
| 激活 refit | tab 激活/面板展开时走 `ensureActiveTabReady()`：已 init 则 `resize()+focus()`，未 init 则首次 `initTerminal`（nextTick 后 fit） | — |
| 隐藏期输出不丢 | xterm 自身缓冲 `write()`；`CreateTerminal` 返回前窗口期由 useTerminal `outputBuffer`（按 sid 过滤）兜底 | — |
| 事件注销 | `EventsOn` 返回闭包保存、`destroyTerminal` 精准调用；**禁 `EventsOff('terminal-output')` 全局移除**（多实例下误删他 tab 监听器） | 其他 tab 输出永久丢失 |
| 实例回收 | 关闭 tab / 组件卸载必须 `dispose()` + `CloseTerminal`，并停掉对应 `isExited` 同步 watcher（`stopExitedWatchers` 表） | PTY 泄漏 + 监听器泄漏 |

## 2. tab 状态与实例分离

- `useTerminalTabs.js`：纯状态（`{id, title, dir, shellType, isActive, isExited}`）+ 上限常量 `MAX_TERMINAL_TABS = 8`（**单一来源在前端**，后端不重复维护，恢复截断靠 `createTab` 返回 null）。
- xterm 实例：TerminalPanel 内非响应式登记表 `Map: tabId → useTerminal()`；渲染只依赖 tab 状态。
- 关闭活动 tab 邻位激活：`min(idx, len-1)`（优先右侧、末尾取左）。
- 单 tab（`tabs.length < 2`）时隐藏 tab 栏（D1 决策）。
- 目录语义（D2）：新建 tab 继承 `uiStore.terminalDir`（文件树当前目录）；已建 tab **不自动跟随**（`watch(terminalDir) → changeDir` 已删除，勿恢复）。

## 3. 快照多 tab 契约（SessionStateVersion = "2"）

结构（model/session.go）：

```go
type TerminalSnapshot struct {
    Visible    bool                   `json:"visible,omitempty"`
    Height     int                    `json:"height,omitempty"`
    WorkDir    string                 `json:"workDir,omitempty"`      // v1 兼容字段：仅旧快照读取，新快照禁写入
    Tabs       []TerminalTabSnapshot  `json:"tabs,omitempty"`          // {workDir, shellType}
    ActiveIndex int                   `json:"activeIndex,omitempty"`   // 0 值被 omit 反序列化仍得 0，无损
    Fullscreen bool                   `json:"fullscreen,omitempty"`
}
```

归一化（`NormalizedTabs()`，Load 时执行，幂等）：

| 输入形态 | 行为 |
|---|---|
| `Tabs` 非空 | 原样使用（v2） |
| `Tabs` 空 且 `WorkDir` 非空 | 降级单 tab `{workDir, shellType:""}`，**随后必须置空 `WorkDir`**（否则 Save 把 v1 字段复写回 v2 落盘） |
| 两者全空 | 返回 nil（空快照不出幽灵 tab） |

前端恢复/保存：

- 恢复：循环 `createTab`（超 8 截断）+ 活动下标双层 clamp；`markRestored` 门禁防恢复写回触发自动保存；首次可见 init 与恢复经 `tabs.length === 0` 互斥防双建。
- 保存：tab 状态经 getter 投影镜像到 ui store（见第 4 节），`useSessionState` 从镜像构建。

## 4. Vue watch 多源引用比较陷阱（通用）

**Wrong**：

```javascript
// tabs 数组 push/splice 为内部变异，引用不变 → 回调不触发
watch([tabs, activeId], syncSnapshot)
```

**Correct**：

```javascript
// getter 返回投影新数组，强制引用变化；activeId 变化作为兜底触发源
watch([() => tabs.value.map(t => ({ ...t })), activeId], syncSnapshot)
```

**Why**：`watch` 多源对 ref 做引用比较。关闭非活动 tab 时 `activeId` 不变，仅 splice —— 直接监听数组则镜像永不收缩，快照残留已关 tab。测试须覆盖「关闭非活动 tab」路径（仅此路径能暴露该缺陷）。

## 5. 整窗全屏契约（D5）

- **形态**：TerminalPanel 根元素加 `is-fullscreen` class，`position: fixed; inset: 0`，禁搬 DOM/重挂载。
- **z-index 层级表**（改动 fixed 层前先对照）：

| 层 | z-index |
|---|---|
| 普通内容 | < 1500 |
| 终端全屏层 | **1500** |
| 右键菜单 `.context-menu` | 2000 |
| Element Plus 弹窗体系（popup manager 自 2001 递增） | 2001+ |
| 浏览器通道注入 UI（仅 serve 模式） | 3000/4000 |

- **状态机不变式**：`fullscreen ⇒ visible`。`toggleTerminal` 收起时必须同时重置 `fullscreen=false`；`toggleTerminalFullscreen` 在面板隐藏时先展开。
- **ESC 过滤**（全屏态 window keydown）：

```javascript
if (e.defaultPrevented) return                                   // CommandPalette 等已消费
if (e.target.closest('.el-overlay, .xterm-helper-textarea')) return // EP 弹窗 / xterm 未消费按键
```

  xterm 正常消费 ESC（vim 等）时走 `cancel(e, true)` 已 `stopPropagation`，不会到 window。
- **动画**：仅 `opacity` keyframes。**禁尺寸 transition**——position 切换不可插值，跳变帧会连环触发 ResizeObserver → fit + `ResizeTerminal` IPC 风暴。
- **Home.vue 配合**：全屏态 `resize-bar` 用 `v-if` 移除、inline `height` 不输出（否则压住 `inset:0`）；还原后两者恢复，`terminalHeight` 全程不动。

## 6. 终端外观设置契约

| 项 | 字段 | 范围/默认 | 热更 |
|---|---|---|---|
| 字号 | `terminalFontSize` | 10-24 / 14 | `term.options.fontSize` 热更 + `resize()`（fit + `ResizeTerminal` IPC 通知 PTY） |
| 字体 | `terminalFontFamily` | 空串=Cascadia Code 栈；Consolas / Courier New 可选 | `term.options.fontFamily` 热更 + refit |
| 回滚 | `terminalScrollback` | 1000-10000 / 1000 | **不热更，新建终端生效**（设置页描述须与之一致） |

- 后端 `EnsureTerminalDefaults()`（service `Load` + App `GetSettings` 异常分支**双兜底**）：零值补默认 + 越界 clamp。
- store 合并写模式（saveTheme/saveDiffTool/saveTerminalAppearance 同款）：`GetSettings` 读盘 → 仅覆盖本组字段 → `SaveSettings`。**注意**：该模式有读盘-写回交错竞态窗口，属既有模式固有，新增保存分组须沿用而非另起炉灶。
- **`el-input-number` 必须带 `:value-on-clear`**：否则清空输入框向 store 写 null/undefined，useTerminal watch 把非法值瞬态热更进 xterm options，落盘后端反序列化为 0 留脏值。
- 工具栏 A-/A+ 步进 ±1：越界返回 false 不变不存，按钮加 disabled class 双保险。

## 7. 测试要点（回归必查）

- 多实例：销毁其一，另一实例监听器存活（闭包未调用）。
- 关闭非活动 tab 后快照镜像收缩（第 4 节陷阱的专属暴露路径）。
- v1 旧快照 `{workDir}` 加载 → 单 tab；Load→Save→Load round-trip 落盘纯 v2 形态（无 `workDir` 残留）。
- `ActiveIndex=0` 序列化无损；空 `Tabs` 无幽灵 tab。
- 全屏态下弹窗内按 ESC 只关弹窗不退全屏。
- 老配置（无外观三字段）加载补默认；越界 clamp。

## 8. `terminal-output` 载荷 UTF-8 完整性契约

**背景**：PTY `Read` 边界与 UTF-8 字符边界无关。输出泵若直接 `string(buf[:n])` 发事件，切片可能停在 3 字节汉字或 4 字节 emoji 中间；Wails 事件 JSON 序列化会把非法字节替换为 `U+FFFD`，前端看到永久乱码且不可恢复。

**契约**：

| 项 | 约定 |
|---|---|
| 解码位置 | 后端输出泵内 `utf8StreamDecoder`（`service/utf8_decoder.go`），**禁前端补救** |
| 残片上限 | 每会话 ≤3 字节，仅在字符完整后 emit |
| EOF 处理 | 先 `Flush()` 输出归一后的残片，再发 `terminal-exit`；**禁静默丢弃** |
| 非法字节 | `strings.ToValidUTF8` 归一为 `U+FFFD`，不得阻塞后续输出 |
| 同包 EOF | `Read` 返回 `n>0` 且 `err=io.EOF` 时先消费数据再判错 |
| 事件契约 | 名称 `terminal-output`、参数 `(sessionID, string)` 不变；载荷恒为合法 UTF-8 |

**Wrong**：

```go
if n > 0 {
    s.emitCurrent("terminal-output", sessionID, string(buf[:n]))
}
```

**Correct**：

```go
if n > 0 {
    if out := decoder.Write(buf[:n]); out != "" {
        s.emitCurrent("terminal-output", sessionID, out)
    }
}
```

**回归必查**：同一中文/emoji 串按 1 字节切分逐片喂入，拼接结果与原文一致；4096 边界切汉字无替换字符；EOF 残片只产出一个 `U+FFFD` 并随后发 `terminal-exit`；非法字节不 panic/不阻塞。

## 9. 会话级进程树回收契约（Windows）

**背景**：`ClosePseudoConsole` 只断开 ConPTY 管道并投递控制台关闭事件，不保证 shell 退出，更不回收 shell 派生的长任务（`ping -t`、`npm run dev`）。仅靠 `Close()` 关句柄会留下孤儿进程。

**契约**：

| 项 | 约定 |
|---|---|
| 绑定时机 | `conpty.Start` 成功后立即绑定，缩小「已派生子进程但未入 Job」窗口 |
| Job 配置 | 每会话独立 Job Object，`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`；仅使用 `golang.org/x/sys/windows` |
| 单点函数 | 创建绑定 `createKillOnCloseJob`；降级包装 `bindJobOrWarn`（`util/pty_windows.go`） |
| 失败降级 | `OpenProcess` / `AssignProcessToJobObject` 失败只 `slog.Warn`（带 pid、err），终端照常创建，关闭时不再保证回收 |
| 关闭顺序 | 先 `conpty.Close()` 让 shell 感知关闭，再 `CloseHandle(job)` 由内核终止整树；顺序不得颠倒 |
| 幂等 | `Close()` 由 `sync.Once` 保护，重复调用返回首次结果，禁重复关句柄 |
| Unix 差异 | Unix 侧仍只向 shell PID 发 `SIGTERM`，不杀进程组；跨平台语义不对称，排查残留时先确认平台 |

**Wrong**：

```go
// 只断 ConPTY，派生任务孤儿化
_ = p.conPty.Close()
```

**Correct**：

```go
// 先断 ConPTY，再关 Job 句柄（关闭即杀整树）
if p.conPty != nil {
    firstErr = p.conPty.Close()
}
if p.job != 0 {
    windows.CloseHandle(p.job)
}
```

**回归必查**：会话内启动 `ping -t` 后 `Close()`，子进程 5 秒内退出；仅关 Job 句柄（保持伪控制台打开）也须终止子进程，证明回收归因于 Job 而非控制台关闭事件；重复 `Close()` 不 panic；Job 绑定失败时终端可用且日志含 pid/err。

## 10. 退出检测契约（Windows 事件驱动 / Unix 轮询兜底 / 单次 emit）

**背景**：`watchProcess` 原按 500ms 轮询 `IsProcessRunning`，用户 `exit` 后退出提示最坏延迟半秒；同时输出泵 `Read` 结束分支也会判定退出并发送 `terminal-exit`，两通道只判会话在册、不判运行状态，存在重复发送窗口。

| 项 | 约定 |
|---|---|
| Windows 等待原语 | `util.PtyProcess.startExitWaiter`（`util/pty_windows.go`）：`OpenProcess(SYNCHRONIZE)` 独立进程句柄 + 手动重置停止事件 `CreateEvent`，goroutine 调 `WaitForMultipleObjects(进程句柄, 停止事件)`，任一触发即 `close(exitCh)` |
| 通道暴露 | `PtyProcess.ExitCh() <-chan struct{}`；nil 表示事件链路不可用（Unix 或句柄/事件创建失败），调用方回退轮询 |
| 失败降级 | `OpenProcess`/`CreateEvent` 失败只 `slog.Warn`（带 pid、err）并关闭已建句柄，`exitCh` 保持 nil，终端照常创建 |
| Close 顺序 | 断 ConPTY → 关 Job → `SetEvent(stopEvent)` 唤醒等待（Job 降级时进程可能未退出，无停止事件则 goroutine 与两个句柄永久泄漏）→ `<-waitDone` 等 goroutine 收尾 → 关闭 `waitHandle`/`stopEvent` 并置零；全程受 `sync.Once` 保护，**禁止与等待 goroutine 并发关句柄** |
| watch 分叉 | `watchProcess` 先取 `ExitCh()`：非 nil 则 `select { <-done; <-exitCh }` 后收口；nil 保留 500ms 轮询。`done` 仍保证关闭路径零轮询周期返回 |
| 单次 emit | `model.TerminalSession.MarkExited() bool` 持 `s.mu` 做「运行中 → 已退出」原子跃迁，仅返回 true 才发送 `terminal-exit`；`service.emitExitIfRunning` 为唯一发送点，**在册检查与 `MarkExited` 必须同持服务 `s.mu`**（先解锁再跃迁会与 `CloseTerminal` 的删除+`SetRunning(false)` 形成误发窗口），输出泵 EOF 与 watch 均只调它 |
| 关闭静默 | `CloseTerminal`/`CloseAll` 先从会话表移除再 Close PTY，事件通道触发时 `emitExitIfRunning` 因不在册跳过，不误发退出事件 |
| Unix 差异 | `ExitCh()` 恒返回 nil：`Close` 仅发 SIGTERM 可能不退出，引入 `cmd.Wait` goroutine 有僵尸/泄漏风险，事件化另立项 |

**Wrong**：

```go
// 两通道各自无条件发送 + 关闭轮询周期，重复事件与延迟同时存在
if exists {
    session.SetRunning(false)
    s.emitCurrent("terminal-exit", sessionID)
}
```

**Correct**：

```go
if exitCh := ptyProc.ExitCh(); exitCh != nil {
    select {
    case <-done:
        return
    case <-exitCh:
    }
}
// 唯一发送点：在册检查 + 原子跃迁去重
s.emitExitIfRunning(sessionID)
```

**回归必查**：进程自然退出后 `ExitCh` 关闭且 shell 已退出；长睡眠进程 `Close` 在 3 秒内返回、`ExitCh` 关闭、两个句柄置零；零值 `PtyProcess.ExitCh()` 为 nil 且 `watchProcess` 轮询兜底不 panic；`emitExitIfRunning`（含输出泵与 watch 竞态）只产出一条 `terminal-exit`；重复 `Close` 幂等。
