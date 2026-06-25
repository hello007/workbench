# 修复 Windows 复制粘贴失效问题

## Goal

修复 WorkBench (Wails 桌面应用) 启动后，Windows 系统级的 Ctrl+C / Ctrl+V 出现"有时候无法生效"的问题。关闭应用后恢复正常。

## What I already know（已从代码确认）

### 1. 应用注册了全局 `keydown` 监听（`frontend/src/views/Home.vue:356-395, 575`）

`handleGlobalKeydown` 绑定在 `document` 上，命中以下条件时会调用 `e.preventDefault()` 并把"当前选中的文件节点路径"写到系统剪贴板：

```javascript
if (!selectedNode.value) return            // 没选中节点：放行（不拦截 Ctrl+C/V）
if (!(e.ctrlKey || e.metaKey)) return
const tag = e.target.tagName
if (tag === 'INPUT' || tag === 'TEXTAREA') return   // 只排除了 INPUT/TEXTAREA

if (e.key === 'c') { e.preventDefault(); handleCopy(selectedNode.value) }
else if (e.key === 'x') { e.preventDefault(); handleCut(selectedNode.value) }
else if (e.key === 'v') { e.preventDefault(); handlePaste(selectedNode.value) }
```

**问题点**：拦截判定**没有**排除：
- `contenteditable="true"` 元素
- 预览面板里的可选中文本（普通 `<div>` / `<pre>` / `<code>`）
- iframe（PDF.js 预览器）
- 已 `getSelection()` 选中文字但焦点不在输入框时

### 2. 后端 `CopyToSystemClipboard` 写入 **CF_HDROP**，并调用 `EmptyClipboard` 清空原内容（`util/clipboard_windows.go:38-107`）

```go
procEmptyClipboard.Call()                       // 清空剪贴板！
procSetClipboardData.Call(cfHDrop, hMem)        // 只设置 CF_HDROP（文件列表格式）
```

含义：用户在浏览器/IDE 里 Ctrl+C 复制的**文本**，一旦应用前端因前面那个拦截触发 `handleCopy`，剪贴板里**之前的文本就被清空**，只剩文件路径（CF_HDROP）。换到其他应用 Ctrl+V → 拿不到文本格式 → 表现为"粘贴无效/没反应"。

### 3. 窗口 focus 也会调用 `OpenClipboard`（`Home.vue:544-564, 576`）

`handleWindowFocus` 每次窗口获得焦点都会 `ReadFromSystemClipboard`，进入 `OpenClipboard(0)`，与其他应用并发操作剪贴板时可能短暂抢占（Windows 剪贴板是独占资源，有几十毫秒锁定窗口）。

### 4. 应用使用 Wails v2（WebView2 / Edge Chromium）作为渲染层

Wails 应用本身不注册全局热键（无 `RegisterHotKey` 调用），所以问题不是系统级全局热键拦截，而是**应用窗口内的 DOM 事件拦截**。

## 确认场景（用户已反馈）

**场景 B（已确认）**：在其他应用（浏览器/VS Code 等）之间复制粘贴时失效，但与 WorkBench 启停有关联
- WorkBench 启动后：其他应用间复制粘贴偶尔无效（有时成功、有时失败）
- 关闭 WorkBench：立即恢复正常

## 场景 B 的根因分析（缩小嫌疑范围）

前端 Ctrl+C/V 拦截（根因 1）**不会影响其他应用** —— DOM 事件监听器作用域仅限应用自己窗口，不可能跨进程。

**真正嫌疑：后端 Win32 剪贴板 API 的独占竞态**

### 嫌疑点 A：`handleWindowFocus` 高频调用 `OpenClipboard`（主要嫌疑）

`Home.vue:544-564, 576` 每次窗口获得焦点都调用 `ReadFromSystemClipboard`：

```javascript
window.addEventListener('focus', handleWindowFocus)  // 用户在 WorkBench 和其他应用间切换时触发

const handleWindowFocus = async () => {
  const result = await ReadFromSystemClipboard()     // → Go 后端 OpenClipboard(0)
  // ...
}
```

调用链：`ReadFromSystemClipboard()` → Go `util.ReadClipboardFiles()` → `OpenClipboard(0)`

**Windows 剪贴板是独占资源**：
- `OpenClipboard(0)` 持有期间（几十毫秒到上百毫秒，取决于读取操作复杂度），其他进程调用 `OpenClipboard` 会立即失败返回
- WorkBench 在用户频繁切换窗口时反复抢占剪贴板锁
- 其他应用（Chrome、VS Code）恰好在这个窗口尝试访问剪贴板 → 失败 → 表现为"复制粘贴无效"

### 嫌疑点 B：`OpenClipboard` 持有时间过长（次要）

`util/clipboard_windows.go:111-137` 在 `OpenClipboard` 和 `CloseClipboard` 之间执行了多次操作：
- 检查 CF_HDROP 格式可用性
- 读取 CF_HDROP 数据（可能包含多个文件路径，需遍历解码）
- 读取 Preferred DropEffect（剪切标记）
- 如果 GC 或内存分配在这期间触发，锁定时间会延长

### 嫌疑点 C：`EmptyClipboard` 清空剪贴板（理论可能，但场景 B 不太匹配）

`util/clipboard_windows.go:83` 在写入文件路径时调用 `EmptyClipboard()`，会清空系统剪贴板里其他应用放入的内容。但这只在用户**在 WorkBench 窗口内**按 Ctrl+C 时触发，不太符合"其他应用间复制粘贴失效"的场景（除非用户误操作）。

## 决策（用户已确认）

**选定方案 A**：移除 focus 时的剪贴板同步 + OpenClipboard 失败容错

**用户补充要求**：粘贴按钮无需设置 `disabled` 态，在用户点击时才判断能否粘贴（延迟校验）

## Requirements（已确定 - 方案 A）

### 核心目标
* 应用启动期间，**其他 Windows 应用之间**的复制粘贴不受影响
* 应用内"复制文件路径到剪贴板 / 文件粘贴"功能继续可用
* 应用窗口失焦/获焦切换时，不抢占系统剪贴板独占锁

### 实施细节
1. **移除 `window.focus` 事件中的剪贴板同步**
   - 删除 `Home.vue` 的 `handleWindowFocus` 及其绑定
   - 删除内部 `clipboard` 对象的窗口聚焦同步逻辑

2. **粘贴按钮 UI 调整**（用户要求）
   - 移除基于 `clipboard.mode` 的 `disabled` 绑定
   - 用户点击"粘贴"按钮时才调用 `ReadFromSystemClipboard` 判断是否可粘贴
   - 若剪贴板为空/无文件格式 → 弹提示"剪贴板中没有可粘贴的内容"

3. **OpenClipboard 失败容错**（`util/clipboard_windows.go`）
   - `OpenClipboard` 返回 0 时，`Sleep(50ms)` 重试最多 2 次
   - 仍失败 → 返回 `nil, false, nil`（不返回 error，避免误判为异常）

4. **保留现有 Ctrl+C/X/V 快捷键逻辑**
   - `Home.vue:356-395` 的 `handleGlobalKeydown` 不动
   - 用户在文件树按 Ctrl+C 仍调用 `handleCopy` → `CopyToSystemClipboard`

## Acceptance Criteria

* [ ] 启动 WorkBench 后，在两个外部应用（如 Chrome ↔ VS Code）之间反复复制粘贴文本 100 次，全部成功
* [ ] 在外部应用复制文本 → 切到 WorkBench → 切回外部应用 Ctrl+V → 文本仍在
* [ ] 在 WorkBench 文件树上按 Ctrl+C 复制文件 → 切到资源管理器 Ctrl+V → 文件粘贴成功
* [ ] 关闭 WorkBench 后行为与启动前一致

## 修复方案（候选）

### 方案 A：移除 focus 时的剪贴板同步 + Open 失败容错（推荐）

**改动**：
1. 删除 `Home.vue` 的 `handleWindowFocus` 中对 `ReadFromSystemClipboard` 的调用（不再在窗口聚焦时同步剪贴板）
2. `handlePaste` 调用 `ReadFromSystemClipboard` 之前的逻辑不变（用户明确要粘贴时才读取）
3. `util/clipboard_windows.go` 给 `OpenClipboard` 加重试：失败时 `Sleep(50ms)` 重试 2 次，仍失败返回 nil（不阻塞）

**Pros**：
- 一次性根除"WorkBench 与其他应用抢剪贴板"的高频竞态
- 改动小，只动 2 个文件
- 应用功能完全保留

**Cons**：
- 失去"窗口聚焦时实时同步内部 `clipboard` 状态"的功能 —— 但实际只影响 UI 上"粘贴按钮"的 disabled 态准确性，下次粘贴时仍会读取最新剪贴板内容，**对用户操作流无实质影响**

### 方案 B：仅给 OpenClipboard 加重试容错（最小改动）

**改动**：
- 不动 `handleWindowFocus`，只在 Go 层给 `OpenClipboard` 失败时静默重试
- 失败后 ReadClipboardFiles 返回 nil 而非 error

**Pros**：改动最小

**Cons**：
- 没解决根本问题（仍然频繁抢锁），只是把"抢锁失败"从对方视角的"复制粘贴无效"换成 WorkBench 自己的"读取失败"
- 实际上**对方应用的失败概率不变**（WorkBench 持有锁的时间窗口没变）

### 方案 C：完全移除剪贴板的 focus 同步 + 改用 Wails 原生剪贴板 API

**改动**：
- 移除 `util/clipboard_windows.go` 自定义实现
- 使用 Wails 提供的 `runtime.ClipboardGetText/SetText`（仅支持文本格式，不支持 CF_HDROP 文件列表）
- "复制文件路径"功能改为写路径文本到剪贴板（用户再到资源管理器手动操作）

**Pros**：彻底绕开 Win32 API 调用

**Cons**：
- 砍掉"在文件树 Ctrl+C → 资源管理器 Ctrl+V 直接粘贴文件"功能（功能回退）
- 与历史 plan `2026-05-13-copy-cut-paste-plan.md` 的目标冲突

## Definition of Done

* 单元 / 组件测试覆盖新的拦截判定逻辑
* 手动按 Acceptance Criteria 逐条复测
* CHANGELOG 增加修复条目，README/文档无需调整

## Out of Scope

* 重写整个剪贴板服务层
* 支持"文件 + 文本"同时写入剪贴板的混合格式
* 修复前端 Ctrl+C/V 拦截过宽问题（场景 A）—— 用户当前未报告此问题，作为独立 Bug 处理

## Technical Notes

### 关键文件
* `frontend/src/views/Home.vue:356-395` — `handleGlobalKeydown` 拦截逻辑
* `frontend/src/views/Home.vue:454-523` — `handleCopy/Cut/Paste`
* `frontend/src/views/Home.vue:544-564` — `handleWindowFocus`
* `util/clipboard_windows.go:38-181` — Windows API 剪贴板读写
* `service/clipboard.go` — 服务层薄封装
* `frontend/src/composables/useShortcuts.js` — 快捷键解析工具（未覆盖 Ctrl+C/X/V，是 Home.vue 自己硬编码的）

### 历史参考（实施时再读）
* `docs/plans/2026-05-13-copy-cut-paste-plan.md`
* `docs/plans/2026-05-13-copy-cut-paste-design.md`
* `docs/plans/2026-05-14-clipboard-integration-plan.md`
* `docs/plans/2026-05-14-clipboard-integration-design.md`

### 可能的修复方向（提示，不是决策）
1. **缩小拦截范围**：判断 `window.getSelection().toString()` 非空时不拦截；或要求事件 `target` 必须落在文件树/活动栏内才拦截。
2. **改用专门的快捷键**：把"复制文件"快捷键改为不与系统 Ctrl+C 冲突的组合（如 Ctrl+Shift+C），与 useShortcuts 统一管理。
3. **剪贴板内容兼容**：写 CF_HDROP 时不调用 `EmptyClipboard`，或同时保留 CF_UNICODETEXT（路径文本）。
