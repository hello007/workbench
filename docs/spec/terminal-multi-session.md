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
