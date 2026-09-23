# 修复 AI 对话面板布局塌陷与终端高度连锁异常

## Goal

修复「AI 对话」活动栏面板点击后布局异常：面板仅占半屏、下方残留空白；展开终端后终端显示高度被压缩异常。根因为 `AiChatPanel.vue` 根元素缺失 flex 高度约束样式，补齐后两症状一并消除。

## 根因分析

### 布局结构

`Home.vue` 的 `.main-area` 为 `flex column` 容器，纵向依次排列：

```
.main-area (flex column)
├── .main-panes        (flex:1 + min-height:0，仅 directory/toolbox 激活时显示)
├── AiFunctionPanel    (v-show activePanel==='ai'，根元素有完整约束)
├── StatsView          (v-show 'stats'，根元素 height:100%)
├── DashboardView      (v-show 'dashboard'，根元素 height:100%)
├── AiChatPanel        (v-show 'ai-chat'，根元素无任何样式 ← 缺陷点)
├── .resize-bar        (3px，flex-shrink:0)
└── TerminalPanel      (inline height: terminalHeight px，默认 200px)
```

### 根因：`.ai-chat-panel` 根元素零样式

对照 `.ai-function-panel`（正常）：

```css
.ai-function-panel {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
  overflow: hidden;
}
```

`AiChatPanel.vue` 的 scoped style 中完全没有 `.ai-chat-panel` 规则（全文仅模板第 2 行出现该 class）。其内部 `.chat-layout` 已写 `flex:1; min-height:0`，但父级既不是 flex 容器、高度也不是确定值，约束链从根断裂。

### 症状传导链

1. **「只占半屏，下面残留空白」**：根元素按 `flex: 0 1 auto` 布局，高度塌为内容自适应高度；终端收起时 TerminalPanel `v-show` 为 `display:none` 不占位，剩余高度留白在面板下方，视觉上像残留的终端位置。
2. **「打开终端后终端显示高度异常」**：展开终端（inline height 200px）后，AiChatPanel 内容高度 + resize-bar + TerminalPanel 超出 100vh 触发 flex 收缩分摊；AiChatPanel 与 TerminalPanel 均默认 `flex-shrink:1`，终端 200px 被压缩；xterm fit 基于被压缩后的容器 clientHeight，渲染高度异常。

## Requirements

### R1 补齐根元素高度约束（修复本体）

`frontend/src/components/AiChatPanel.vue` scoped style 顶部新增：

```css
/* 根元素：占满 Home 主区上半区（与 .main-panes 互斥），高度约束链从此起（对齐 .ai-function-panel） */
.ai-chat-panel {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
  overflow: hidden;
}
```

- 语义与 `.ai-function-panel` 完全一致，保持四个一级面板行为对称。
- `overflow:hidden` 使 flex 收缩时 `min-height:auto` 解析为 0，与既有面板一致可被安全压缩而不挤压 TerminalPanel。

### R2 回归验证矩阵

| 场景 | 期望 |
|---|---|
| AI 对话面板 + 终端收起 | 面板占满上半区，无残留空白 |
| AI 对话面板 + 终端展开 | 终端高度精确为 terminalHeight（拖拽分隔条可调），面板吃掉剩余全部高度 |
| AI 对话 ↔ AI 功能页 ↔ 三栏区往返切换 | 布局无残留，会话与任务态不丢失（v-show 保状态不回归） |
| 终端全屏/还原 | 全屏 fixed 覆盖整窗，还原后高度正确 |
| 三栏区（directory）+ 终端 | 原行为不回归 |

### 验证方式

- `cd frontend && npm test`（既有单测不回归，AiChatPanel.spec.js 通过）
- `wails dev` 或 serve 模式人工核对 R2 矩阵
- vitest 无法断言 flex 布局像素，R2 以人工验证为准；单测保证组件逻辑不回归

### R3 补充修复：会话快照白名单缺 ai-chat/dashboard（验证过程中发现）

浏览器视觉验证时发现连带缺陷：`frontend/src/composables/useSessionState.js` 的 `VALID_PANELS` 白名单仅含 4 项，缺 `'ai-chat'` 与 `'dashboard'`（ActivityBar 实有 6 个一级入口）。保存链 `buildSessionState` 会如实写入 `activePanel: "ai-chat"`，但恢复链 `applySessionState` 按白名单忽略 → AI 对话/状态看板的面板位置无法跨会话还原（保存/恢复不对称）。

修复：白名单补齐为 `['directory', 'ai', 'ai-chat', 'toolbox', 'dashboard', 'stats']`，与 ActivityBar items 严格对齐；补恢复用例（ai-chat/dashboard 各断言一次）。

### 验证结果（2026-09-23）

- 全量前端测试 65 文件 / 1351 用例全通过（含新增白名单恢复用例）
- serve 模式（127.0.0.1:36116，1600x900 视口）浏览器实测 4 截图全通过，存 `.trellis/tasks/09-23-ai/research/`：
  - 面板收终端时满占主区（1552x900，无残留空白）
  - 终端展开 318px 与 inline height 严格一致（零压缩），面板精确让位 579px = 900 - 3 - 318
  - 分隔条拖拽逐像素吻合（318→418→258），面板同步反向伸缩
  - 面板往返切换后布局无残留；computed style 确认 `flex:1 1 0% / min-height:0` 已生效

## R4 输入框默认高度 3 倍 + 高度可拖拽（用户追加优化）

### 现状

`chat-input`（el-input type=textarea）`:autosize="{ minRows: 2, maxRows: 6 }"`，默认 2 行。

### 要求

- 默认高度 3 倍 = 6 行
- 高度可拖拽调整（浏览器原生 resize 手柄即可，`resize: vertical`）
- 实现约束：autosize 模式由 JS 接管高度与原生 resize 冲突，须去掉 `:autosize`，改 `:rows="6"` + CSS `resize: vertical` + `min-height`（约 2 行下限）/`max-height`（防拖到占满会话区，约 40vh）限幅
- Enter 发送 / Shift+Enter 换行行为不变
- 高度不持久化（拖拽态会话内有效即可，不入快照）

## R5 管理模板弹窗对齐「设置」弹窗大小与布局（用户追加优化）

### 现状

`tplDialogVisible` 弹窗 `width="min(560px, 80vw)"`（小弹窗分档），`tpl-manage-body` 横向布局但高度内容自适应。

### 目标（对照 SettingsPanel）

- 弹窗宽：`min(560px, 80vw)` → `min(960px, 86vw)`（与 settings-dialog 一致）
- 主体：`tpl-manage-body` 改 `display:flex; height: min(560px, 78vh)` + `margin: calc(-1 * var(--spacing-lg))` 负 margin 抵消 el-dialog__body padding 填满 body padding-box（对齐 `.settings-body`，注释同款说明）
- 左侧列表栏：对齐 `.settings-nav` 风格——宽 200px、`bg-secondary` 背景、右缘 1px 边框、列表项 active 左侧指示条（`left:0` 贴左缘，禁负 left）、「新增模板」项保留列表顶部
- 右侧表单区：`flex:1` 吃掉剩余宽；模板正文 textarea 随弹窗增高加大（rows 5 → 约 12 或 flex 拉伸）
- 交互逻辑（增删改 / scope radio 编辑锁定 / loading / 删除确认）零改动，纯尺寸与样式改造

### 验证

- `AiChatPanel.spec.js` 既有断言同步（如 autosize/弹窗宽度相关），npm test 通过
- serve 模式截图核对两处视觉效果与 SettingsPanel 风格一致性

## Out of Scope

- 不改 TerminalPanel / ui store / Home.vue 布局代码（根因在 AiChatPanel 自身样式缺失）
- 不调整面板内部 chat-layout 视觉（padding 等已有设计不动）
- 不改其余三个一级面板（Stats/Dashboard 用 height:100% 也能被 flex 收缩安全让位，用户未报问题，不扩散修改面）
