# 前端页面风格开发规范（Frontend Visual Conventions）

> 适用范围：WorkBench 前端新增页面/组件的视觉风格统一。本文是「指南层」，管页面如何用设计令牌搭出一致风格；变量定义/取值/双主题同步规则见 [design-tokens.md](design-tokens.md)（「契约层」），本文引用变量名不重述取值。

## 适用范围

新增 Vue 页面/组件、重构现有组件视觉、调整布局/弹窗尺寸时须参照本规范，保持与 ActivityBar/DashboardView/SettingsPanel/CommandPalette 已确立的视觉模式一致。

## 触发条件

满足任一即须按本规范校验：

- 新增页面或面板组件
- 新增/重构带导航的弹窗（含 nav+content 二分结构）
- 新增卡片、表格、异常提示等展示型组件
- 调整弹窗尺寸或响应式断点
- 新增 hover/active 交互态视觉

## 规范

### 1. 配色用法

#### 语义色应用场景

项目用靛蓝主色 + 四档语义辅助色，**按语义选色而非按喜好选色**：

|变量|语义|典型场景|
|---|---|---|
|`--primary-color`|主操作/选中态|主按钮、nav active 填充、链接、表格行 hover 高亮（用 `--primary-bg`）|
|`--success-color`|正向/通过|工作区「干净」标签、操作成功提示|
|`--warning-color`|注意/待处理|工作区「有改动」、未拉取计数、异常提示卡片左边框、GPU 重启提示|
|`--danger-color`|危险/失效|未推送计数、分离头指针、取消关注按钮、删除收藏按钮|
|`--info-color`|中性提示|无上游标签、已关注标签、工作目录已移除标记|

**禁混用语义**：成功态不用 primary，警告态不用 danger；`el-tag` 的 `type` 属性须与上述语义对齐（`success`/`warning`/`danger`/`info`/默认即 primary）。

#### 蓝灰中性色阶层级

背景/文字/边框各分档，**按层级递进**：

|层级|背景|文字|边框|
|---|---|---|---|
|主层（页面底）|`--bg-primary`|`--text-primary`|`--border-color`|
|次层（卡片/弹窗）|`--bg-secondary`|`--text-secondary`|`--border-light`|
|弱层（内嵌区/hover）|`--bg-tertiary`|`--text-tertiary` → `--text-placeholder`|—|

- 卡片/弹窗用 `--bg-secondary` 浮于 `--bg-primary` 之上；卡片内嵌列表区（如 DashboardView `.add-list`）用 `--bg-tertiary` 形成第三层
- 侧边栏 `--sidebar-bg` 为深蓝灰独立档（与主背景同色相，禁纯中性灰或暖灰），其文字走 `--sidebar-text`/`--sidebar-text-hover` 而非通用文字变量
- 次要文字逐档降级：`--text-tertiary` → `--text-placeholder`；完全失效行用 `opacity: 0.5` 整体灰显（参照 `.row-missing`）

#### 暗色阴影着色

阴影**禁纯黑**，须带蓝灰色相（亮色 Slate-900 着色、暗色 Slate-950 着色更深一档）。四档 `--shadow-sm/md/lg/xl` 保持单一光源方向（左上投右下），透明度递增。取值与同步规则见 [design-tokens.md](design-tokens.md) 第 4 节，本文不重述。

### 2. 布局模式

#### VSCode 式三列主框架

主界面三列横向排布，各列 `flex-shrink: 0` 固定宽或 `flex: 1` 占满：

|列|组件|宽度|角色|
|---|---|---|---|
|ActivityBar|`ActivityBar.vue`|48px|图标导航 + 底部终端入口|
|FileTreePanel|文件树/导航面板|固定宽或可折叠|当前面板内容|
|ContentPanel|主内容区|`flex: 1`|占满剩余空间|

- ActivityBar 项为 36px 方块居中于 48px bar（两侧 6px 间隙）
- 列间用 `1px solid var(--border-color)` 分隔

#### 面板内 nav+content 二分

带导航的面板/弹窗（如 SettingsPanel）内部二分：

- **nav 区**：固定宽（SettingsPanel 为 200px）+ `flex-shrink: 0` + `border-right` 分隔
- **content 区**：`flex: 1` + `overflow-y: auto`（内容溢出仅 content 滚动，nav 固定不动）

#### Dashboard 表格 + 卡片化异常提示

数据看板用 `el-table` 主展示 + 卡片化辅助提示：

- 表格行 hover 用 `--primary-bg` 轻量高亮（禁主色实色填充，避免压字降低对比度）
- 异常提示**禁裸用默认 `el-alert` 平铺**，须以 `.error-list` 卡片层包裹 `el-alert`：`border-left: 3px solid var(--warning-color)` + `--radius-md` + `--shadow-sm`（el-alert 仍在内，由外层卡片重着色提供质感，参照 `DashboardView` `.error-list`）
- 失效行整体 `opacity: 0.5` + 仓库名 `text-decoration: line-through`（line-through 仅作用于 `.repo-name.repo-missing`，非整行）

### 3. 组件视觉模式

#### nav active 左侧指示条

带导航项的 active 态用「左侧 3px 指示条 + 主色填充」双反馈，**禁用 scale**（避免与 hover 重复反馈）：

```css
.nav-item {
  position: relative;                 /* 供 ::before 定位 */
}
.nav-item.is-active {
  background: var(--primary-bg);      /* 或 --sidebar-active-bg（侧边栏场景） */
  color: var(--primary-color);        /* 或 --sidebar-active-text */
}
.nav-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;                            /* 关键：贴 item 左缘，禁用负 left */
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  background: var(--primary-light);
}
```

**`left: 0` 是硬约束**：父容器（如 `.home`）常设 `overflow: hidden`，负 `left` 会导致指示条溢出被裁剪。参照 ActivityBar `.activity-bar-item.is-active::before`（现状圆角硬编码 `2px`，待归一 `--radius-sm`）与 SettingsPanel `.settings-nav-item.is-active::before`。

#### 卡片 hover 阴影层级

卡片默认 `--shadow-sm`，hover 升 `--shadow-md` + `border-color` 切 `--primary-light`，过渡用 `--transition-fast`（规范值；现状 style.css `.card` 为 `--transition-normal`，待归一）：

```css
.card {
  box-shadow: var(--shadow-sm);
  border: 1px solid var(--border-color);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}
.card:hover {
  box-shadow: var(--shadow-md);
  border-color: var(--primary-light);
}
```

参照全局 `.card`（过渡现状 `--transition-normal`，待归一 fast）、SettingsPanel `.settings-item`、`.shortcut-item`。

#### section-title 字重梯度

标题字重对齐全局 `h1-h6` 梯度（`style.css` 已定义，自定义标题类须显式声明不依赖浏览器默认）：

|层级|font-weight|letter-spacing|场景|
|---|---|---|---|
|h1-h3 / 面板主标题|600|`-0.01em`（负字距收紧）|DashboardView `.dashboard-title`、SettingsPanel `.settings-section-title`|
|h4-h6 / 小节标题|500|默认|CommandPalette `.section-title`（12px）|
|小标签 / uppercase|500|`0.04em`（正向字距撑开）|`.label-caps` 类（11px）|

- 负字距仅用于大标题（收紧视觉），小标签用正向字距（撑开 uppercase 字母间距）

#### kbd 键帽字体族

快捷键键帽用等宽字体族，**前缀 Geist** 保证与正文同字体渲染：

```css
kbd {
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
}
```

参照 SettingsPanel `.shortcut-keys kbd`、`style.css` `.context-menu-shortcut`。

#### 行内 style 禁用

**所有样式迁语义 class**，禁用 `style="..."` 行内写法（守 redesign-skill Code Quality）。常见迁移模式：

|行内写法|语义类替代|
|---|---|
|`style="margin-top:24px"`|`.section-title--spaced { margin-top: var(--spacing-lg); }`|
|`style="width:280px"`|`.input-w-xl { width: 280px; }`（归并档 sm/md/lg/xl）|
|`style="color:#f59e0b"`|用 `--warning-color` 变量或 `color-mix(in srgb, var(--warning-color) 12%, transparent)` 派生|

宽度归并位（参照 SettingsPanel）：`sm=120 / md=140 / lg=240 / xl=280`，视觉等义的任意像素值归并到最近档位。

### 4. 字号/间距/圆角用法

#### 四档变量应用规则

|类型|变量档位|应用规则|
|---|---|---|
|间距|`--spacing-xs/sm/md/lg/xl`|就近归并：布局间距（容器级/组件间 margin/padding/gap）任意像素值归到最近档位，禁用 `5px`/`10px`/`12px` 等非档位值；键帽/徽章类微元素自身内边距有特例，见「间距就近归并」适用边界|
|圆角|`--radius-sm/md/lg/xl`|内紧外松：容器外圈 `--radius-lg`（较软）→ 内卡片 `--radius-md`（较紧）→ 内元素 `--radius-sm`|

#### 内紧外松层级

圆角按嵌套层级递减，形成「外软内紧」节奏：

- 弹窗外圈 `--radius-lg`（参照 `.settings-dialog .el-dialog`）
- 卡片/设置项 `--radius-md`（参照 `.settings-item`、`.error-list`）
- 键帽/小标签 `--radius-sm`（参照 `kbd`）

#### 间距就近归并

**适用边界**：四档归并约束的是**布局间距**——容器级与组件间的 margin/padding/gap。键帽（`kbd`）、徽章、tag 等最小视觉元素的**自身内边距**允许字面像素特例（如 SettingsPanel `kbd` 的 `padding: 2px 6px`）：这类元素尺寸跟随内容、且档位 4/8 跳档过宽——设置弹窗紧凑化实测 8px 水平 padding 配 12px 字号键帽明显空旷，收紧至 6px 无对应档位。

布局间距实际像素值归并到最近档位，**禁用非档位值**：

|场景|档位|示例|
|---|---|---|
|紧凑内边距（tag 等）|`--spacing-xs`（4）|tag 内边距|
|组件内间距、gap|`--spacing-sm`（8）|`.result-section` margin、`header-actions` gap|
|卡片 padding、列表项|`--spacing-md`（16）|`.settings-item` padding、`.dashboard-view` padding|
|弹窗 body padding、段落间距|`--spacing-lg`（24）|`.el-dialog__body` padding、`.section-title--spaced`|
|大段落外间距|`--spacing-xl`（32）|大区块分隔，少见|

### 5. 弹窗尺寸策略

#### min() 响应式

弹窗宽高用 `min(像素上限, 视口比例)` 双约束：大屏卡上限保聚焦、小窗缩不溢出：

```vue
width="min(960px, 86vw)"      <!-- 宽：像素上限 + 视口比例 -->
height: min(620px, 78vh);     <!-- 高：同上 -->
top="15vh"                    <!-- 距顶：vh 响应式（CommandPalette 显式设置；SettingsPanel 沿用 EP 默认 15vh） -->
```

#### 尺寸分档

按内容量分档，**禁用固定像素宽高**（小窗溢出、大屏浪费）：

|弹窗类型|宽|高|参照|
|---|---|---|---|
|设置弹窗（多 tab + nav）|`min(960px, 86vw)`|`min(620px, 78vh)`|SettingsPanel|
|命令面板（搜索 + 列表）|`min(720px, 70vw)`|内容区 `min(480px, 60vh)`|CommandPalette|
|内容多的大弹窗|`min(Npx, 85vw)` N≥900|按内容自适应|RepoFilterDialog/AiTaskHistoryPanel（目标值 900/920；现状固定 px，待迁移 min() 模式）|
|中小弹窗（添加/选择）|`min(Npx, 80vw)` N=600-700|自适应|DashboardView 添加仓库（目标值 640；现状固定 px，待迁移 min() 模式）|
|小弹窗（确认/表单）|420-600px 固定或 `min`|自适应|DirectoryTree 添加 500 / GitBranches 420|

- `top` 用 vh 响应式（如 `15vh`），禁固定 px（小屏顶距过大、大屏过小）
- 内容区 `max-height` 用 `min(像素, vh)` 约束，超出 `overflow-y: auto`
- `el-dialog` 外圈圆角 `--radius-lg`，body padding `--spacing-lg`，遮罩用蓝灰着色（亮 `rgba(15,23,42,…)` / 暗 `rgba(2,6,23,…)`，禁纯黑）

## 正反示例

### 正确：nav active 指示条 left:0

```css
.nav-item.is-active::before {
  left: 0;                         /* 贴 item 左缘，overflow:hidden 不裁剪 */
  width: 3px;
  background: var(--primary-light);
}
```

### 错误：指示条用负 left

```css
.nav-item.is-active::before {
  left: -6px;                      /* 溢出 item 左缘，父容器 overflow:hidden 裁剪 → 指示条不可见 */
}
```

### 正确：弹窗 min() 响应式

```vue
width="min(960px, 86vw)"           <!-- 大屏 960px 保聚焦，小屏 86vw 不溢出 -->
```

### 错误：弹窗固定像素

```vue
width="960px"                      <!-- 1024×768 小屏溢出，2560 屏偏小不响应 -->
```

### 正确：卡片 hover 阴影层级

```css
.card { box-shadow: var(--shadow-sm); }
.card:hover { box-shadow: var(--shadow-md); border-color: var(--primary-light); }
```

### 错误：卡片 hover 用主色实色填充

```css
.card:hover { background: var(--primary-color); }   /* 压字、文字对比度失衡 */
```

### 正确：异常提示卡片化

```css
.error-list {
  border-left: 3px solid var(--warning-color);      /* 左边框语义色 */
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-sm);
}
```

### 错误：用默认 el-alert 平铺

```html
<el-alert type="warning" />   <!-- 无左边框强调、无圆角阴影，与卡片体系割裂 -->
```

### 正确：宽度用语义类

```html
<el-input class="input-w-xl" />   <!-- 归并档，可复用 -->
```

### 错误：行内 style 写宽度

```html
<el-input style="width:280px" />   <!-- 非语义、难复用、守 redesign-skill 禁令 -->
```

### 正确：语义色派生用 color-mix + 变量

```css
.hint {
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--warning-color) 35%, transparent);
}
```

### 错误：硬编码 rgba 色值

```css
.hint {
  background: rgba(230, 162, 60, 0.12);   /* 不随主题切换、脱离变量体系 */
}
```

## 关联文档

- [design-tokens.md](design-tokens.md) — 设计令牌契约层：变量定义/取值/双主题分层/Element Plus 同步/阴影着色/终端背景锁定（本文引用不重述）
- [cross-layer-contracts.md](cross-layer-contracts.md) — 视觉改动属纯前端 CSS/组件，不触发 wailsjs 绑定层变动
- [e2e-testing.md](e2e-testing.md) — `wailsjs/` 不入库，视觉改动不引入 wailsjs 耦合
- [测试策略.md](../测试策略.md) — 前端 ≥70% 覆盖率门禁（视觉改动通常无测试影响）

## 沉淀来源

2026-09-17 WorkBench 前端视觉现代化重做（任务 `.trellis/tasks/archive/2026-09/09-16-workbench` 系列三任务）：ActivityBar/DashboardView/SettingsPanel/CommandPalette 视觉模式已确立并稳定，新增页面无统一参照易风格漂移，故将「怎么用变量搭风格」沉淀为指南层规范，与 design-tokens.md 契约层形成双子。
