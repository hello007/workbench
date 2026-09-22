# Research: 桌面开发工具多终端 UX 惯例 + xterm.js 多实例技术实践

- **Query**: VSCode / Windows Terminal / JetBrains IDE / Hyper 多终端组织方式对比；tab 与工作目录关系；xterm.js 多实例已知问题与最佳实践；终端面板视觉惯例；对本项目的可行方案
- **Scope**: mixed（外部为主 + 本项目 `frontend/src/composables/useTerminal.js`、`frontend/src/components/TerminalPanel.vue` 现状核对）
- **Date**: 2026-09-22

---

## 1. 多终端交互形态惯例

### 1.1 四工具对比

| 维度 | VSCode | Windows Terminal | JetBrains IDE | Hyper |
|---|---|---|---|---|
| **默认形态** | 终端面板 + 右侧垂直 tab 列表（tabs view）+ split 组合 | 顶部横排 tab + pane 分屏组合 | 工具窗口内横排 tab + tab 内 split | 顶部横排 tab，官方未内置分屏（分屏靠社区插件） |
| **次要形态** | `tabs.enabled=false` 时退化为下拉切换 | —（tab 与分屏同为一等公民） | 仅 tab 或仅 split 均可用 | — |
| **单终端时 tab 栏** | **默认隐藏**（`tabs.hideCondition` 默认 `singleTerminal`，即只有一个终端时不显示 tab 列表） | 始终显示（至少一个 tab） | 始终显示 | 始终显示 |
| **tab 显示信息** | 主标题 = 进程名（`tabs.title` 默认 `'${process}'`，即 shell 类型）；右侧描述 = cwd 文件夹名（`tabs.description` 默认 `'${task}${separator}${local}${separator}${cwdFolder}'`），另有 `${cwdFolder}` 说明：仅在与初始目录不同/多根工作区时显示 | shell 名 + 目录名 + 关闭按钮 | tab 标题默认为会话名/shell 名；经「Open in Terminal」打开时显示目标目录名 | shell 名 + 关闭按钮 |
| **新建交互** | tab 列表条目 hover 出内联 split 按钮；面板右上角 `+` 动作（含下拉选择 shell/profile） | tab 栏末端 `+`（可下拉选 profile）；快捷键分屏 | 工具栏 `+` 或 `Ctrl+Shift+T` 新建 tab | tab 栏末端 `+` |
| **关闭交互** | tab hover 出关闭 icon；窗口关闭时有活跃会话可确认（`confirmOnExit` 支持 `hasChildProcesses`） | tab 上 X 直接关闭 | **关闭 tab 前确认终止运行中进程**（官方文档明确："close the tab by clicking and confirm the process termination"） | tab X 关闭 |
| **tab 重命名** | 支持（右键/context menu Rename；`sequence` 变量允许程序写入标题） | 支持（重命名 tab 标题） | 支持 | 支持有限（配置/插件） |
| **拖拽重排** | 支持（tab 列表内拖动重排、拖出成组/合并组） | 支持（tab 拖动重排、拖出成新窗口） | 支持 | 部分支持（版本相关） |

### 1.2 各来源

- VSCode `terminal.integrated.tabs.*` 全部默认值取自源码 `src/vs/workbench/contrib/terminal/common/terminalConfiguration.ts`（main 分支）：
  - `TabsEnabled` 默认 `true`，描述原文："Controls whether terminal tabs display as a list to the side of the terminal. When this is disabled a dropdown will display instead."
  - `TabsHideCondition` 默认 `'singleTerminal'`
  - `TabsShowActions` / `TabsShowActiveTerminal` 默认 `'singleTerminalOrNarrow'`
  - `TabsLocation` 默认 `'right'`
  - `TerminalTitle` 默认 `'${process}'`；`TerminalDescription` 默认 `'${task}${separator}${local}${separator}${cwdFolder}'`；分隔符默认 `' - '`
  - 链接：https://github.com/microsoft/vscode/blob/main/src/vs/workbench/contrib/terminal/common/terminalConfiguration.ts
- VSCode 分屏与状态图标（bell、任务完成 check/失败 X、hover 看状态）官方文档：https://code.visualstudio.com/docs/terminal/basics（"Groups (split panes)" 一节）
- Windows Terminal 分屏官方文档（垂直分屏在焦点窗格右侧开新窗格、水平分屏在下侧开新窗格）：https://learn.microsoft.com/en-us/windows/terminal/panes
- JetBrains 官方终端文档（新 tab = 新会话；tab 内右键 Split Right/Down；关闭 tab 确认终止进程；默认 cwd = 项目根；右键文件 Open in | Terminal）：https://www.jetbrains.com/help/idea/terminal-emulator.html
- Hyper 项目定位与形态：https://github.com/vercel/hyper（README 定位为 "beautiful and extensible" 的 web 标准终端；官方功能无内置 pane 拆分）。注意：Hyper 文档佐证较弱，此条仅作参考。

### 1.3 主流共识结论

1. **tab 是四者共同的主形态**；split 是叠加增强形态（三者有、Hyper 无），二者是"组合"而非"二选一"。
2. **单终端时隐藏 tab 栏**（VSCode 默认行为）是最值得借鉴的渐进展示：首终端体验与单终端版本完全一致，多终端后 tab 栏自然浮现。
3. **tab 信息双层结构**：主标题放 shell 类型/进程名，次要信息放 cwd 文件夹名——VSCode 用 `title + description` 两段式实现，正好匹配本项目「Shell 下拉 + 路径徽章」现有信息结构。
4. **关闭一律 hover 出 X**；运行中进程关闭需确认或退出时统一确认（`confirmOnExit`）。
5. 分屏在**底部 200px 高度面板**中可用性差（这是底部 dock 面板与独立终端应用的本质差异），主流工具的分屏主要服务于大面积终端区域。

---

## 2. tab 与工作目录关系

### 2.1 行为差异对比

| 行为 | VSCode | JetBrains IDE |
|---|---|---|
| 新建终端默认目录 | workspace root（`terminal.integrated.cwd` 默认 `undefined` → 工作区根；官方文档 "starts at the root of your workspace"） | 项目根（官方文档 "runs with the current directory set to the root directory of the current project"） |
| 指定目录新建 | Explorer 右键 "Open in Integrated Terminal"；`splitCwd` 控制 split 场景 | 右键文件 → Open in \| Terminal，以该文件所在目录打开 |
| split 新终端目录 | `terminal.integrated.splitCwd` 默认 **`'inherited'`**（macOS/Linux 继承父终端当前 cwd；**Windows 上与 `initial` 相同**，即父终端启动目录） | — |
| **已建终端是否跟随资源管理器目录变化** | **否**。终端 cwd 由 shell 进程持有，IDE 不随 Explorer/编辑器切换自动 `cd` | **否**。同理 |

来源：
- `Cwd` 配置（默认 undefined）：terminalConfiguration.ts 382-388 行
- `SplitCwd` 配置（enum `['workspaceRoot', 'initial', 'inherited']`，**默认 `'inherited'`**）：terminalConfiguration.ts 489-498 行，链接同上
- VSCode 文档：https://code.visualstudio.com/docs/terminal/basics（Working directory 一节）
- JetBrains 文档：https://www.jetbrains.com/help/idea/terminal-emulator.html

### 2.2 主流做法结论

- **"创建时决定目录，创建后不自动跟随"是主流**。目录跟随只发生在「新建/split/右键打开」这类**创建动作**上，已有终端的 shell cwd 由用户在 shell 内自行控制。
- 本项目现状（`TerminalPanel.vue` 134-138 行 `watch(terminalDir)` → `changeDir` 自动跟随）是**非主流行为**：单终端时代它是便利特性，多 tab 后会产生歧义（PRD Open Question「谁跟随」）。主流对齐做法：**新建 tab 时继承当前文件树目录；已有 tab 不跟随**；如需保留便利性，可仅让"活动 tab"跟随，但须在 UI 上可见（路径徽章实时变化），且用户手动 `cd` 过的 tab 应停止跟随（避免打架）。

---

## 3. xterm.js 多实例技术要点

### 3.1 fitAddon 在隐藏容器（display:none）上的行为

- **官方定论：隐藏容器无法 fit，属设计行为**。
  - issue #3029 "FitAddon and display 'none'"：维护者 Tyriar："Display none means we can't pull the calculated values off of the element, just like if it wasn't attached to the dom."（issue 关闭为 closed as designed）。官方建议的 workaround：**创建时用 `terminal.resize(cols, rows)` 设一个默认尺寸，容器变为可见后再调 `fit()`**。
    https://github.com/xtermjs/xterm.js/issues/3029
  - issue #664 "fit addon and multiple xterm instances"：隐藏时 `getComputedStyle` 的宽高返回 `auto`，fit 无效；官方确认 "The fit add-on depends on the element's container element and visibility" 并补充进文档。
    https://github.com/xtermjs/xterm.js/issues/664
  - issue #4560：目标元素未显示时渲染器抛 `Cannot read properties of undefined (reading 'getRasterizedGlyph')`——隐藏期间渲染链路本身就不可靠。
    https://github.com/xtermjs/xterm.js/issues/4560
- **fit 源码行为**：`fit()` 调 `proposeDimensions()`，若返回 `undefined` 或 `isNaN(cols/rows)` 则**静默 return（不报错、不 resize）**；`proposeDimensions()` 依赖 `getComputedStyle(parentElement)` 的实际像素值。这就是"隐藏时 fit 静默失败"的机制。
  https://github.com/xtermjs/xterm.js/blob/master/addons/addon-fit/src/FitAddon.ts
- **隐藏期间 `terminal.write()` 完全正常**：数据写入与 buffer 解析不依赖可见性（issue #3029 讨论中明确用户诉求 "write to a terminal that's in the background, waiting for the user to navigate to it" 是成立场景；VSCode 后台终端即此模式）。**隐藏的代价只在渲染与尺寸测量，不在数据**。

### 3.2 VSCode 的多实例实践（VSCode 即 xterm.js 最大规模生产使用者）

- **实例常驻 + CSS class 切换显隐，绝不销毁重建**。`terminalInstance.ts` `setVisible(visible)`（main 分支 1442-1458 行）核心逻辑：
  ```ts
  setVisible(visible: boolean): void {
      const didChange = this._isVisible !== visible;
      this._isVisible = visible;
      this._wrapperElement.classList.toggle('active', visible);   // CSS 切换显隐
      if (visible && this.xterm) {
          this._open();                          // 确保 open
          this._resizeDebouncer?.flush();        // 冲刷挂起的 resize
          this._resize();                        // 重新评估尺寸（注释明确：切换到该终端时确保使用最新尺寸，含后台以缓存尺寸创建的终端）
      }
      ...
  }
  ```
  https://github.com/microsoft/vscode/blob/main/src/vs/workbench/contrib/terminal/browser/terminalInstance.ts
- **激活切换时必做三件事**：确保 open → 冲刷挂起 resize → 重新 `_resize()`（内部走 fit + 通知 PTY）。这就是"切回 tab 必须 refit"的权威实践。
- **后台创建终端是官方支持场景**：setVisible 注释明示 "when terminal is created in the background using cached dimensions of a split terminal"——后台终端以已知缓存 cols/rows 创建，激活时再校正。
- VSCode 对 **隐藏的 tab 用 `display:none` 级别的 class 切换**（wrapperElement `.active`），所有 xterm 实例 DOM 常驻。

### 3.3 内存占用与实例数量

- **scrollback 是每实例最大的可变内存项**。VSCode `terminal.integrated.scrollback` 默认 **1000 行**，配置描述原文："We pre-allocate memory based on this value in order to ensure a smooth experience. As such, as the value increases, so will the amount of memory."（terminalConfiguration.ts 317-321 行）。多实例时该项按实例数线性放大，是唯一必须主动控制的参数。
- **dispose 必须完整**：xterm.js 近期仍有 dispose 注册缺口导致泄漏的修复记录——issue #5818 "Three dispose-registration gaps leak Terminal instances past host unmount"（closed）、PR #4655 "Fix API facade memory leaks"（closed）。教训：实例销毁时须 dispose Terminal 本体 + 各 addon + 注入的事件监听器。
  https://github.com/xtermjs/xterm.js/issues/5818
- **WebGL 渲染器有 GPU 内存泄漏史**：issue #3889 "The webgl addon leaks GPU memory"（已修复）。若未来引入 `@xterm/addon-webgl` 提升渲染性能，多实例下每个实例独立 GL 上下文，浏览器 GL 上下文数量上限（通常约 16 个）会成为硬约束——实例多时应只给活动实例启用或干脆用默认 DOM/canvas 渲染。
  https://github.com/xtermjs/xterm.js/issues/3889
- **实例数量上限**：xterm.js 本身不设上限、VSCode 也不设硬上限；真实约束来自 ①每实例 PTY 是独立 OS 进程，②scrollback 内存线性增长。常见工程做法即"不硬限渲染、软限会话数 + 压低 scrollback"（VSCode 用首选项让用户自己控制；Terminal 类产品多在 10 个会话量级做软提示）。PRD 假设的 8-10 上限与生态实践一致，属合理保护值。

### 3.4 对接本项目现状的关键结论

- 本项目 `TerminalPanel.vue` 已有的两处处理恰好与官方实践对齐，多实例改造时保留：
  - 「首次 visible 才 `initTerminal`」（112-131 行注释 "xterm 在 display:none 下无法正确 fit"）——对应 #3029 结论；
  - 「展开时 `resize()` + `focus()`」——对应 VSCode setVisible 的 refit 实践。
- 本项目当前缺失、多实例必须补的：**每个 tab 的 DOM 容器常驻（`v-for` + `v-show` 切 active），切换时对目标实例 refit**；**事件监听从 `EventsOff` 全局注销改为 `EventsOn` 返回闭包精准注销**（本项目 spec 已有此规则，`useTerminal.js` 161-162/208-209 行现违反应在本次一并修）；**销毁时 addon 随 Terminal 一起 dispose**。

---

## 4. 终端面板美化惯例

### 4.1 视觉要点汇总

| 要点 | 惯例 | 佐证 |
|---|---|---|
| tab 栏位置 | 独立终端应用（WT/Hyper）顶部横排；**底部 dock 面板（VSCode）用右侧垂直列表或并入顶部动作行**；JetBrains 用工具窗口顶部横排 | 各官方文档/源码（见第 1 节） |
| 高度 | 单行紧凑，约 30-40px 区间；与面板工具栏同高可合并成一行，避免"工具栏 + tab 栏"双层叠高 | VSCode tab 行为紧凑单行列表；本项目工具栏 36px 可直接容纳 |
| 图标 | 每个 tab 一个 shell/终端图标（VSCode tab 有 icon 且支持按 profile 着色）；状态类图标（bell/运行/退出）叠加其上 | VSCode docs terminal/basics："Hover the icon to read status information, which may contain actions" |
| 状态指示 | 进程退出：tab 图标变灰/状态图标变化 + 终端区内可再显式提示；任务运行中：check/X/进度（VSCode 支持 tab 状态动画 `tabs.enableAnimation` 默认 true） | terminalConfiguration.ts `TabsEnableAnimation`；VSCode docs |
| 关闭按钮 | hover 才出现，常驻仅活动 tab 或全 hover；避免一排常驻 X 的视觉噪音 | WT/VSCode/JetBrains 交互一致 |
| 单行信息密度 | 主文字（shell 名/进程名）+ 次要文字（目录名，弱色、更小字号、超长省略）；VSCode 的 title + description 两段式 | `TerminalTitle`/`TerminalDescription` 默认值 |
| 新建按钮 | tab 末端 `+`，常驻（这是唯一常驻的操作图标） | WT/Hyper/VSCode 一致 |
| 主题适配 | tab 栏背景与面板工具栏一致、活动 tab 与终端背景形成层级差（active 更亮或加 accent 指示条） | 各工具主题系统；本项目 design-tokens 体系可直接映射 |

### 4.2 对 200px 级底部面板的特别结论

- 底部低矮面板里，**"右侧垂直 tab 列表"（VSCode）比"顶部横排 tab"更省纵向空间**：横排 tab 会再吃掉一行高度（200px 面板里占 8-15%），右侧列表吃横向（路径徽章通常有富余）。
- 但右侧列表与本项目"36px 工具栏 + shell 下拉"结构冲突较大；**把 tab 做进现有 36px 工具栏单行内**（tab 段 + 操作段）是 VSCode "面板顶行动作区 + tab 融合"的等价简化，是低高度面板的常见取舍。

---

## 5. 对本项目的建议

**现状锚点**：Vue3 + `@xterm/xterm` + fit/web-links addon；底部面板约 200px 高；36px 工具栏（左：shell 下拉 + 路径徽章，右：重启 + 收起）；v-show 保 DOM、ResizeObserver、首次可见才 init（已对齐官方实践）；后端 `sessions` map + `terminal-output(sid)`/`terminal-exit(sid)` 事件已天然多会话。

### 方案 A：tab 融入现有 36px 工具栏（推荐）

- 布局：工具栏左段改为 **tab 序列**（每 tab = shell 图标 + shell 名 + 目录名（弱色省略）+ hover 关闭 X，活动 tab accent 指示条 `left:0`），`+` 新建按钮置于 tab 末端；shell 类型选择收敛为「新建下拉」（`+` 的下拉菜单选 shell 类型）或 tab 右键菜单；路径徽章保留显示**活动 tab** 的 cwd；右侧保留重启/收起。
- 单 tab 时 tab 序列退化为当前样子（仅徽章），对齐 VSCode `hideCondition: singleTerminal` 渐进展示。
- 技术要点：`useTerminal` 重构为多实例管理器（`v-for` 渲染容器，`v-show` 切 active）；激活切换 refit + focus（对齐 VSCode `setVisible`）；`EventsOn` 闭包按 tab 精准注销（替换现有全局 `EventsOff`）；scrollback 显式设 1000；目录跟随改为「新建时继承活动目录，已建 tab 不自动跟随」（主流对齐，解决 PRD Open Question）。
- 工作量：中；一次性解决 PRD 的多会话 + 美化两个目标。

### 方案 B：右侧垂直 tab 列表（VSCode 完整形态）

- 终端区域右侧加一列窄 tab 列表（icon + 两行文字），面板顶部动作行保留现状。
- 优点：最贴 VSCode、纵向零占用；缺点：200px 高的面板再削横向空间，与现有 36px 工具栏信息重复（shell/路径在两处显示），需重构工具栏信息归属；工作量高于 A。
- 适合作为「终端面板未来加高/可最大化」之后的演进形态。

### 方案 C：下拉切换器（最小改动，VSCode `tabs.enabled=false` 形态）

- 不加 tab 栏：工具栏 shell 下拉旁加「会话下拉」（shell 名 + 目录名 + 关闭按钮内嵌 option）+ `+` 新建按钮。
- 优点：工作量最小；缺点：会话状态不可见、切换两次点击，不满足"美化"目标。仅作为兜底方案。

### 跨方案通用技术约束（来自第 3 节）

1. 多实例事件监听：`EventsOn` 返回闭包精准注销，禁止 `EventsOff('terminal-output')` 全局注销（本项目 spec 已有此规则，现有代码需一并修正）。
2. 隐藏 tab 的 `write()` 正常缓冲（数据不依赖可见性），激活时 refit + focus；**不要**在隐藏时调 `fit()`（静默失败，#3029/#664）。
3. 每实例 `scrollback: 1000`（对齐 VSCode 默认，控制多实例内存线性增长）；会话数软上限 8-10（保护 PTY 进程资源，PRD 假设成立）。
4. 销毁路径完整：`term.dispose()` 连同 addon、PTY `CloseTerminal(sid)`、事件闭包三者同步（防 #5818/#4655 类泄漏）。
5. 不引入 webgl addon（多实例 GL 上下文上限风险 + 历史泄漏问题）；当前默认渲染器对 200px 面板足够。
6. 崩溃恢复快照扩展为多终端数组时，`TerminalSnapshot` 结构变更须同步 `frontend/wailsjs/` 三处（spec 契约）；旧单终端快照降级还原。

## Caveats / Not Found

- Hyper 的「官方不内置分屏」仅依据官方仓库 README 与功能页未列出该能力，未逐版本验证，佐证较弱。
- 各工具 tab 栏的精确像素高度（如 WT tab 高度）官方未公布规格值，本文只给"约 30-40px 单行紧凑"的区间判断，未虚构精确数字。
- VSCode tab 状态图标的具体配色/样式随主题变化，未逐主题核对；结论只到"有状态图标 + hover 说明"这一层。
- `terminal.integrated.tabs.title` 默认值 `'${process}'` 与 `tabs.description` 默认值取自 main 分支源码，较旧版本（如 1.7x 之前）默认值可能不同（早期版本默认 `shellProcess`），引用时注意版本。
