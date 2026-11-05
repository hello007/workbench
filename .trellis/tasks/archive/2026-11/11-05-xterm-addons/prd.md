# 终端基础体验补强：xterm addons 与交互

## Goal

引入 xterm.js 官方四个 addon（webgl / search / unicode11 / serialize），补齐终端面板的基础交互能力：搜索、OSC 通知、Ctrl+Click 链接、Resize 防抖、scrollback 提升、多行粘贴确认。目标是大输出场景滚动流畅、历史可搜索、宽字符不错位、长任务通知可见，同时不触碰事件契约与 ConPTY 生命周期（纯前端为主 + 设置段最小跨层同步）。

## What I already know（代码库调研结论）

### 现状

| 项 | 现状 | 位置 |
|---|---|---|
| xterm 版本 | `@xterm/xterm ~6.0.0`（tilde 档） | frontend/package.json |
| 已装 addon | addon-fit `~0.11.0`、addon-web-links `~0.12.0`（均在 TILDE 清单） | frontend/package.json + scripts/check-dep-consistency.mjs |
| 终端初始化 | `initTerminal` 挂 fit + web-links，主题双 watch，事件闭包注销链完整 | frontend/src/composables/useTerminal.js:123-205 |
| Resize 链路 | TerminalPanel 的 ResizeObserver 回调**直接**调 `inst.resize()`，无防抖 | frontend/src/components/TerminalPanel.vue:518-525 |
| scrollback | 前后端常量对齐：默认 1000、范围 1000-10000、不热更（新建生效） | model/settings.go:129 + frontend/src/store/settings.js:146-154 |
| URL 点击 | WebLinksAddon 已挂（xterm.js 默认**单击**即开，无 Ctrl 要求） | useTerminal.js:140 |
| 通知 UI | `ElNotification` 已在 App.vue 使用（崩溃提示），应用内通知可复用 | frontend/src/App.vue:9,40 |
| 打开文件 | 后端已有 `OpenWithDefaultApp(path) bool`（false=失败）与 `OpenInExplorer(path) bool` | app_external.go:125,19 |
| 文件存在性 | **无 FileExists 类 RPC**（已搜 app_*.go 确认） | — |
| OSC 捕获 | xterm `parser.registerOscHandler(id, cb)` 官方 API；注意 OSC `9;4` 是进度条子类型（Windows Terminal），需放行 | 实现要点 |
| 粘贴事件 | xterm `onTextPaste` 可拦截；确认框走 ElMessageBox.confirm 单次确认形态 | docs/spec/confirm-dialog.md |
| E2E mock | `GetSettings: {}` 空对象安全回退，新增设置字段无需改 mock | frontend/src/test/wails-mock-defaults.js:570 |

### addon 版本（待确认：用户说「以 peerDependencies 为准」，实测官方 addon 均未声明 peerDependencies）

xterm.js monorepo 与 6.0.0 同批发布的配套 latest（commit 同源，兼容性由 release 对应关系保证）：

| addon | latest |
|---|---|
| @xterm/addon-webgl | 0.19.0 |
| @xterm/addon-search | 0.16.0 |
| @xterm/addon-unicode11 | 0.9.0 |
| @xterm/addon-serialize | 0.14.0 |

处理：按配套 latest 引入，tilde 声明（`~x.y.0`），与现有 `@xterm/*` 分档一致；四包名登记进 `scripts/check-dep-consistency.mjs` TILDE 清单。

## Requirements

### R1 四个 addon 引入与 WebGL 回退

- package.json 新增四依赖（tilde 档）+ check-dep-consistency.mjs TILDE 清单登记（先登记后装包）。
- **webgl**：`initTerminal` 时尝试 `loadAddon(new WebglAddon())`；加载失败（throw）或 `contextLoss` 回调触发时 `dispose()` 并回退默认 DOM 渲染，终端照常可用。`WebglAddon` 的 `onContextLoss` 官方 API 处理上下文丢失。
- **unicode11**：挂载即生效（emoji/CJK 宽度正确性），无交互面。
- **serialize**：挂载并接入序列化能力（落点见 Open Question Q1）。
- **search**：配合搜索条（R2）。
- addon 加载失败不得阻塞终端创建（try/catch 包裹，失败仅 console/slog 降级）。

### R2 Ctrl+F 搜索条

- `attachCustomKeyEventHandler` 捕获 Ctrl+F（焦点在终端时），弹出搜索条（叠加于终端区，参照 VSCode 风格：输入框 + 上一处/下一处 + 区分大小写 toggle + 匹配计数 + 关闭按钮）。
- SearchAddon：`findNext` / `findPrevious` / `dispose`；关闭时 dispose 释放（「关闭释放」）。
- Esc 关闭搜索条（输入框内 Esc 不冒泡到全屏退出链路，须 stopPropagation——对齐 TerminalPanel ESC 过滤契约）。
- Enter=下一处、Shift+Enter=上一处。
- 搜索条遵循 frontend-visual-conventions（语义色、设计令牌）与终端主题（不侵占终端区域，叠加定位）。

### R3 OSC 9/777/99 通知转应用内通知

- `registerOscHandler(9/777/99)` 捕获通知类转义序列，转 `ElNotification` 应用内提示（title=终端 tab 标题或「终端通知」，message=OSC 载荷）。
- OSC 9 放行 `9;4`（进度条）子类型给 xterm 内部处理，仅消费通知形态（`9;<message>`）。
- OSC 777（`777;notify;title;body`）与 OSC 99（kitty notify）解析载荷。
- 总开关：AppSettings 新增字段（Open Question Q2 确认默认值），设置页开关控件；关闭时不注册 handler 或 handler 内直接放行。
- 同一命令重复通知做节流（防刷屏：同 tab 同内容 N 秒窗口内只提示一次，阈值实现期定，默认 5s 量级）。

### R4 Ctrl+Click 打开 URL 与文件路径

- URL：web-links 现状为单击打开；本任务统一为 **Ctrl+Click**（自定义 handler 检查 modifier，或配置 addon 对应选项——实现期按 addon API 定），与文件路径行为一致。
- 文件路径：自定义 link provider（`registerLinkProvider`）检测选中文本为路径形态（绝对路径 + 常见相对路径前缀），Ctrl+Click 调 `OpenWithDefaultApp`。
- **路径不存在时不弹错误**：`OpenWithDefaultApp` 返回 false 静默（不提示、不中断）。（是否需要存在性预检见 Open Question Q3）
- 打开成功与否均不打断终端焦点。

### R5 Resize 防抖 + scrollback 默认提升

- TerminalPanel 的 ResizeObserver 回调加 **100ms debounce**（拖拽分隔条/全屏切换期间不风暴；防抖尾部触发一次 refit + ResizeTerminal IPC）。
- 全屏切换既有 `ensureActiveTabReady` 兜底路径保留（防抖不影响显式 resize 调用：tab 切换、字号热更不走 RO）。
- scrollback 默认 1000 → **5000**：改前后端默认值常量（model/settings.go `DefaultTerminalScrollback` + settings.js `TERMINAL_APPEARANCE_DEFAULTS.scrollback` + 同步测试断言）；范围边界 MIN=1000/MAX=10000 不动；「不热更，新建生效」语义不变。
- 老用户已持久化值是否跟随提升见 Open Question Q4。

### R6 多行粘贴确认

- `onTextPaste` 拦截：粘贴文本含换行（`\r`/`\n`）时弹 ElMessageBox.confirm 确认（单次确认形态），确认才 `write`，取消丢弃。
- 单行粘贴直通（零打扰）。
- 确认框文案说明风险（多行命令 shell 立即逐行执行）。

## Acceptance Criteria

- [ ] 大输出（如 `cat` 大文件 / 持续日志）滚动流畅，无明显卡顿（WebGL 生效时）
- [ ] WebGL 不可用（加载抛错或 context loss）时终端自动回退 DOM 渲染，功能不受损
- [ ] Ctrl+F 呼出搜索条，可搜历史缓冲（上/下一处、区分大小写、Esc/按钮关闭）；关闭后 addon 释放
- [ ] emoji / CJK 混排输出列对齐不错位（unicode11 生效）
- [ ] OSC 9/777/99 通知转为应用内 ElNotification；总开关默认关、开启后生效、关闭后不再出现
- [ ] Ctrl+Click URL 打开系统默认浏览器；Ctrl+Click 文件路径调系统默认程序；路径不存在静默无报错
- [ ] tab「重新启动」后旧缓冲内容（scrollback 范围内）在新会话中回放
- [ ] 窗口/分隔条拖拽 Resize 期间 ResizeTerminal IPC 按防抖收口（100ms 尾部一次）
- [ ] 新装/零值默认 scrollback=5000；已持久化 1000 的配置不被覆盖；设置页可改（1000-10000）；已有终端不受热更影响（新建生效语义不变）
- [ ] 含换行粘贴弹确认框；确认后写入、取消后不写入；单行粘贴不弹
- [ ] `npm test`（vitest）通过：新增单测覆盖搜索开关（Ctrl+F 呼出/关闭释放）、通知开关、debounce 时序、WebGL 加载失败回退
- [ ] 后端 `go test ./model/...`（或相关包）通过：默认值变更断言同步
- [ ] `node scripts/check-dep-consistency.mjs` 通过（四依赖登记 TILDE 且 lock 一致）

## Definition of Done

- 单元测试新增/更新（搜索、通知开关、debounce、WebGL 回退四项为硬性覆盖点）
- vitest + 后端相关包测试 + check-dep-consistency 全绿
- docs/spec/terminal-multi-session.md 沉淀本任务新契约（addon 生命周期、OSC handler 注册面、搜索条与 ESC 过滤契约）
- CLAUDE.md 关键规则表补行（若形成「下次写代码要照着做」级规则）
- 不改 `terminal-output`/`terminal-exit` 事件契约；不动输出泵与 ConPTY 生命周期（约束验证）

## Technical Approach（草案）

**纯前端主战场**：所有 addon 挂载、搜索条、OSC handler、link provider、粘贴确认、debounce 全部在 useTerminal.js / TerminalPanel.vue / 新组件（搜索条、通知服务）内完成，零后端事件改动。

**跨层最小面**：仅 AppSettings 一个新字段（OSC 通知开关）+ DefaultTerminalScrollback 常量变更。新设置段走 `EnsureAllDefaults()` + `PrivateCopy()` 双单点（cross-layer-contracts 规则）；wailsjs models.ts 若为具名类型需检查（int 标量字段通常无需）。

**分层**：
- `useTerminal.js`：addon 挂载/回退/释放、OSC handler 注册、link provider、onTextPaste、serialize 接入
- `TerminalPanel.vue`：Ctrl+F 分发、搜索条挂载、RO debounce、通知开关消费
- 新组件 `TerminalSearchBar.vue`（搜索条 UI）
- 新 composable 或 utils：`terminalOscNotify.js`（OSC 载荷解析 + 节流）、`terminalLinkProvider.js`（路径正则与验证）
- `settings.js` + `SettingsPanel.vue`：通知开关 + scrollback 默认值
- 后端 `model/settings.go`：字段 + 默认值常量

## Decision (ADR-lite)

> 四问均按推荐项落入；确认 PRD 时任一项可推翻，改动面见各问选项描述。

**Q1 serialize 接入落点**
- Context：验收要求「重启后 tab 内容可由 serialize 恢复（只接入，不强制 UI）」。
- Decision：**tab 重启自动恢复**——`restartTerminal` 时 `serialize()` 旧实例缓冲（含 scrollback），新会话创建后 `write` 回放；无任何新 UI。
- Consequences：验收项自然成立；serialize 面保持内存态，不落盘、不进 session.json；回放内容为纯文本序列，不恢复光标位与暂态（OSC 状态）。

**Q2 OSC 通知总开关默认值**
- Context：「提供总开关，默认不打扰」。
- Decision：**默认关**（`terminalOscNotifyEnabled: false`）。设置页显式开启后 handler 才生效（未开启时不注册，零开销）。
- Consequences：功能默认静默，无刷屏风险；代价是默认安装用户不感知该功能（接受，文档/设置页描述承担可见性）。

**Q3 文件路径存在性预检**
- Context：「路径不存在时不弹错误」。
- Decision：**不预检**——Ctrl+Click 调 `OpenWithDefaultApp`，返回 false 静默；link provider 的路径形态正则决定 hover 链接态（形态像路径即可点，点击失败无感）。零新增 RPC。
- Consequences：不存在路径 hover 也呈链接态（轻微体验折扣，可接受）；浏览器通道 RPCHiddenMethods 零同步、wailsjs 零改动。

**Q4 scrollback 默认值迁移**
- Context：默认 1000 → 5000。
- Decision：**不覆盖已持久化值**——仅改前后端默认值常量，新装/零值补默认路径生效 5000；已存 1000 的老用户保持不变，设置页可自改。范围边界 MIN=1000 / MAX=10000 不动。
- Consequences：标准默认值语义，无数据副作用；存量用户需手动调整（设置页描述更新为默认 5000）。

**addon 版本来源**
- Context：需求写「版本以 peerDependencies 为准」，npm 实查官方 addon 均未声明 peerDependencies。
- Decision：按 xterm 6.0.0 monorepo 同批发布配套 latest 引入（webgl 0.19.0 / search 0.16.0 / unicode11 0.9.0 / serialize 0.14.0），tilde 档声明 + check-dep-consistency.mjs TILDE 清单登记。
- Consequences：与现有 `@xterm/*` 分档一致；升 xterm major 时四 addon 须同批升级（同源配套），登记进依赖升级要点。

## Out of Scope

- 不改 `terminal-output` / `terminal-exit` 事件契约、输出泵、ConPTY 生命周期（硬约束）
- 不做 OSC 通知的系统级推送（仅应用内 ElNotification）
- 不做 serialize 快照的持久化落盘与崩溃恢复 UI（「只接入，不强制 UI」= 仅 tab 重启回放内存态，不入 session.json）
- 不动终端主题色板与 `--terminal-bg` 契约（沿用 useTerminal.js 双主题）
- 不新增浏览器通道可见面（新 RPC 若有须同步 RPCHiddenMethods——Q3 选 a 则零新增）
- 不做搜索高亮样式自定义（用 SearchAddon 默认）

## Open Questions

（已全部决断，见 Decision (ADR-lite)；确认 PRD 时可推翻任一项。）

## Research References

- 无外部研究文件；版本与 peerDependencies 事实来自 npm registry 实查（`npm view @xterm/addon-*` 2026-11-05），代码现状来自仓内直读（文件与行号见上表）。

## Technical Notes

- xterm.js 官方 addon 不声明 peerDependencies（npm view 实测为空），兼容性由 monorepo 同批发布保证；「版本以 peerDependencies 为准」按配套 latest 落地，tilde 档与现有 `@xterm/*` 一致。
- OSC 9;4 为 Windows Terminal 进度条子类型，xterm 内部有处理，注册 9 handler 时须放行，勿整段消费。
- WebLinksAddon 单击打开为 xterm.js 默认行为，与本任务「Ctrl+Click」目标语义不一致，R4 中统一。
- 搜索条 Esc 关闭须 stopPropagation，防止冒泡触发整窗全屏退出（TerminalPanel ESC 过滤契约，terminal-multi-session.md §5）。
- ElMessageBox 在 EP 2.14 关闭后实例驻留 DOM，单次确认形态裸选择器安全（confirm-dialog.md）。
- 崩溃恢复快照（session.json）不涉及本任务字段；serialize 不入快照（Out of Scope）。
