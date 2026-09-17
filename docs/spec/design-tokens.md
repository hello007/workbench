# 设计令牌体系（Design Tokens）

> 适用范围：WorkBench 前端全局 CSS 变量层、深浅双主题、Element Plus 主题同步、终端背景联动。所有视觉层改动须遵循本契约，避免组件割裂与主题同步失效。

## 触发条件

满足任一即须按本文档校验：
- 修改 `frontend/src/style.css` 的 `:root` 或 `html.dark` 变量
- 新增/修改全局 CSS 变量
- 改主色 `--primary-color` 或 Element Plus `--el-color-primary`
- 改背景色 `--bg-primary` / `--bg-secondary` / `--bg-tertiary`
- 改终端相关变量 `--terminal-bg`

## 契约

### 1. 双主题变量分层

|选择器|特异性|职责|
|---|---|---|
|`:root`|(0,1,0)|亮色基线变量|
|`html.dark`|(0,1,1)|暗色覆盖（须高于 :root 才生效）|

- **非颜色变量**（间距 `--spacing-*` / 圆角 `--radius-*` / 动画 `--transition-*`）只在 `:root` 声明一次，`html.dark` **不重复声明**（主题切换不应改尺寸节奏）
- `html.dark` 仅覆盖颜色变量

### 2. 主色与 Element Plus 强制同步

改 `--primary-color` **必须**同步覆盖 Element Plus 派生变量，否则 `el-button`/`el-tag`/`el-dialog` 等组件仍用默认蓝，与全局主色割裂：

```css
:root {
  --primary-color: #2563eb;
  /* Element Plus 主色派生（light-3/5/7/8/9 + dark-2）须同步 */
  --el-color-primary: #2563eb;
  --el-color-primary-light-3: #4f80ed;
  /* ... light-5/7/8/9 + dark-2 ... */
}
html.dark {
  --el-color-primary: #3b82f6;        /* 暗色主色提亮一档保对比度 */
  --el-color-primary-light-9: rgba(59, 130, 246, 0.15);  /* 暗色 light-9 用透明度非实色 */
}
```

- 亮色 light-3~9 为实色阶
- 暗色 light-9 改 `rgba(...,0.15)` 透明度（暗底实色 light-9 会过亮）

### 3. 冷暖灰统一色相

**禁混用冷暖灰**。本项目统一 **蓝灰（Slate）色相**：
- 亮色背景：`#f8fafc` / `#ffffff` / `#f1f5f9`（Slate-50/white/Slate-100）
- 暗色背景：`#0f172a` / `#1e293b` / `#334155`（Slate-900/800/700）
- 文字：`#0f172a` → `#94a3b8` 四档（Slate-900 → Slate-400）
- 边框：`#e2e8f0` / `#cbd5e1`（Slate-200/300）

侧边栏 `--sidebar-bg` 须与主背景同色相（深蓝灰 `#0f172a`/`#020617`），禁用纯中性灰或暖灰。

### 4. 阴影着色规则

**禁纯黑 `rgba(0,0,0,…)` 低透明阴影**。阴影须带背景色相：
- 亮色：`rgba(15, 23, 42, …)`（Slate-900 着色）
- 暗色：`rgba(2, 6, 23, …)`（更深 Slate-950 着色，暗色阴影比亮色更深一档补偿）

四档阴影 `--shadow-sm/md/lg/xl` 须保持单一光源方向（左上投右下），透明度递增。

### 5. 终端背景锁定契约

`--terminal-bg` **须与** `frontend/src/composables/useTerminal.js` 的 `LIGHT_TERMINAL_THEME.background` / `DARK_TERMINAL_THEME.background` **严格一致**：

|主题|`--terminal-bg`|`useTerminal.js` 常量|
|---|---|---|
|亮|`#ffffff`|`LIGHT_TERMINAL_THEME.background`|
|暗|`#1d1e1f`|`DARK_TERMINAL_THEME.background`|

- 改 `--bg-primary` 暗色时，**不要**为追求统一而改 `--terminal-bg`（会破坏 xterm 容器与视口背景同步，导致暗色残留）
- 终端背景可与主背景略有差异（终端略深形成层次），可接受
- 若确须改终端背景色，须**同时**改 `useTerminal.js` 常量 + `--terminal-bg` 两处，并回归验证 xterm 渲染

### 6. 字体梯度

字体族 `Geist`，四字重梯度（`assets/fonts/geist-*.woff2`）：

|字重|用途|文件|
|---|---|---|
|400 Regular|正文|`geist-regular.woff2`|
|500 Medium|h4-h6、标签|`geist-medium.woff2`|
|600 SemiBold|h1-h3、标题、强调|`geist-semibold.woff2`|
|700 Bold|数据数字强调|`geist-bold.woff2`|

- `@font-face` 须加 `font-display: swap`（避免 FOIT 阻塞首屏）
- `font-family` 须保留 fallback 链：`"Geist", "Microsoft YaHei", -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Helvetica Neue", sans-serif`
- 数据密集界面（`el-tag`/`el-progress__text`/`.sha-text`）须 `font-variant-numeric: tabular-nums`

### 7. 右键菜单变量独立成组

`--menu-*` 变量独立于主背景/文字变量，因右键菜单常需与主界面对比度更强的背景（如毛玻璃 `backdrop-filter: blur`）。改主色时 `--menu-text-hover` 联动 `--primary-color` 即可，其余 menu 变量不随主色变。

## 测试要求

- 改变量层后须跑 `cd frontend && npm test`（前端 ≥70% 覆盖率门禁，exclude wailsjs）
- 视觉改动不触发 wailsjs 绑定层变动（纯前端 CSS/组件，无需同步 `App.js`/`App.d.ts`/`models.ts`）
- 深浅双主题切换须手动验证（`wails dev` → 设置切主题，无残留、无亮闪）

## 正反示例

### ✅ 正确：改主色同步 Element Plus

```css
:root {
  --primary-color: #2563eb;
  --el-color-primary: #2563eb;
  --el-color-primary-light-9: #eff6ff;
  /* ...其余派生... */
}
```

### ❌ 错误：只改 --primary-color 不同步 --el-*

```css
:root {
  --primary-color: #2563eb;
  /* 缺 --el-color-primary 同步 → el-button 仍默认蓝 #409eff，割裂 */
}
```

### ❌ 错误：改终端背景未同步 useTerminal.js

```css
html.dark {
  --terminal-bg: #0f172a;  /* 与 useTerminal.js DARK_TERMINAL_THEME.background #1d1e1f 不一致 */
}
```
→ xterm 容器 `#0f172a` vs 视口 `#1d1e1f`，暗色主题下可见色差残留。

### ❌ 错误：暗色用纯黑阴影

```css
html.dark {
  --shadow-md: 0 4px 6px rgba(0, 0, 0, 0.35);  /* 纯黑无色相，与蓝灰背景不协调 */
}
```

## 关联文档

- [cross-layer-contracts.md](cross-layer-contracts.md) — 终端 `--terminal-bg` 与 `useTerminal.js` 同步规则的原始来源
- [test-coverage-gate.md](test-coverage-gate.md) — 前端 ≥70% 覆盖率门禁
- [e2e-testing.md](e2e-testing.md) — `wailsjs/` 不入库，视觉改动不引入 wailsjs 耦合

## 沉淀来源

2026-09-16 WorkBench 界面视觉现代化重做（任务 `.trellis/tasks/09-16-workbench`）：主色换靛蓝系、Geist 字体、暗色阴影着色、Element Plus 同步。过程中发现 `--terminal-bg` 与 `useTerminal.js` 锁定关系、Element Plus `--el-*` 须同步、冷暖灰统一三条硬约束值得固化为契约。
