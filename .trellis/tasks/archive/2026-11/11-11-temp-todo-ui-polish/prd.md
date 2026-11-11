# 美化临时待办显示页面

## Goal

重做「临时待办」一级面板（`frontend/src/components/TempTodoPanel.vue`）的视觉与信息层级，解决全宽单列行长过长、操作按钮不可发现、长文本展开失控三个问题，使其在宽屏下易读、窄屏下不挤压。

## What I already know

- 面板是 Home 下的全宽独占一级面板（`v-show="uiStore.activePanel === 'todo'"`），与三栏区互斥，不是窄侧栏。
- 根元素已具备完整 flex 约束链，符合 `frontend-visual-conventions.md` §2。
- header：标题 + 常驻副标题 + 200px 搜索框 + 导出/导入/新建三个文字按钮，单行排布。
- 条目：checkbox + 正文（`-webkit-line-clamp: 2`，点击切换展开）+ 时间/标签 + 右侧 4 个 hover 才显形的图标按钮。
- 已完成区默认折叠；新建与行内编辑共用 `draftText/draftTags`；全屏编辑走 `el-dialog fullscreen`。
- 现有样式全部使用设计令牌，无硬编码色值。
- 既有测试：`frontend/src/components/__tests__/TempTodoPanel.spec.js`（含 `data-action` 语义锚）、E2E `frontend/e2e/temp-todo-import.spec.js`。

## Decisions

### D1 · 长文本展示（方案 A）

- 默认折叠 **3 行**，底部渐隐提示还有内容。
- 仅正文**实际被截断**时显示「展开」按钮（检测 `scrollHeight > clientHeight`）。
- 展开后正文区 `max-height: 50vh`，超出内部滚动；提供「收起」按钮。
- **正文点击不再切换展开态**，避免与「选中文字复制」冲突。

### D2 · 整体布局（方案 A）

- 单列卡片；内容区（含 header）限宽 `min(1000px, 100%)` 水平居中。
- 不做双列网格、不做时间分组。

### D3 · 条目操作按钮（方案 A）

- 复制 / 转提示词 / 编辑 / 删除四枚图标**常驻**卡片内（meta 行右侧或卡片右下），不再依赖 hover 显形。
- 默认 `--text-tertiary`；悬停卡片或单个图标提亮为 `--text-primary`；删除悬停转 `--el-color-danger`。
- 保留 `data-action="copy|to-prompt|edit|delete"` 语义锚。

### D4 · 顶部 header（方案 A）

- 保持单行；副标题瘦身为 `kbd` 键帽提示，窄窗口（< 900px）隐藏。
- 导入 / 导出改图标按钮 + tooltip；「新建」保持主按钮。
- 搜索框弹性宽：`flex: 1`，最小 160px、上限 240px。

### D5 · 条目卡片视觉（方案 A 轻卡片）

- `--bg-secondary` 底 + `--border-light` 描边；hover 边框升 `--border-color` + `--shadow-sm`，**不整圈变主色**。
- 已完成卡片 `--bg-tertiary` + 正文降 `--text-secondary`，不用删除线。

### D6 · 新建区（方案 A）

- 保持列表顶部内联卡片，强化视觉：主色左边条 + 浅主色背景，输入区自动聚焦，按钮右对齐并标注快捷键。
- 已完成区保持现状（默认折叠、标题点击展开），仅跟随卡片样式调整；不新增「清空已完成」。

## Requirements

1. 长文本按 D1：3 行折叠、截断才显展开、展开限高 50vh 内滚、显式收起、正文点击不切换。
2. 布局按 D2：单列 + 内容区限宽 1000px 居中。
3. 操作按钮按 D3：常驻、低对比度、hover 提亮、删除危险色。
4. header 按 D4：单行精简、kbd 提示、图标化导入导出、搜索弹性宽。
5. 卡片视觉按 D5：轻卡片 hover 阴影、不整圈主色、已完成弱化不提删除线。
6. 新建区按 D6：主色左边条 + 浅主色底、自动聚焦、按钮右对齐。
7. 纯前端改动，不改 `model/`、service、wails 绑定与数据格式。

## Acceptance Criteria

- [ ] 长正文默认显示 3 行；未截断条目无「展开」按钮；展开后正文区最高约 50vh 且可内部滚动；「收起」可还原
- [ ] 选中正文文字不触发展开/收起
- [ ] 内容区在宽屏下居中且宽度不超过 1000px，header 与列表左缘对齐
- [ ] 四个操作图标默认可见，删除悬停呈危险色
- [ ] header 在 900px 以下隐藏副标题，不出现按钮挤压重叠
- [ ] 卡片 hover 只升边框与阴影，不全圈主色
- [ ] 新建卡片有主色左边条，打开后 textarea 自动聚焦
- [ ] 既有单测与 E2E 通过，新增展开/收起与截断检测用例

## Definition of Done

- 前端单测与 E2E 通过，覆盖率不低于 70% 门禁
- `npm run build` 通过
- 符合 `docs/spec/frontend-visual-conventions.md` 与 `docs/spec/design-tokens.md`
- 行为变化同步 `docs/spec/temp-todo.md`（新增「条目展示契约」小节）与 README（如有必要）

## Out of Scope

- 后端数据模型、保留策略、导入/导出格式
- 新增待办字段（优先级、截止时间等）
- 双列网格布局、时间分组
- 「清空已完成」批量删除
- 全屏编辑弹窗的功能语义调整

## Technical Notes

- 关键文件：`frontend/src/components/TempTodoPanel.vue`、`frontend/src/components/__tests__/TempTodoPanel.spec.js`、`frontend/e2e/temp-todo-import.spec.js`
- 规范：`docs/spec/frontend-visual-conventions.md`、`docs/spec/design-tokens.md`、`docs/spec/temp-todo.md`
- 约束：禁行内 style；间距归并四档；卡片层级 `--bg-secondary` 浮于 `--bg-primary`；hover 禁主色实色填充
- 截断检测注意：面板 `v-show` 常驻，隐藏时 `clientHeight === 0`，需在面板可见时重算（watch `uiStore.activePanel` 或首次可见时刷新）
- 长文本折叠与测量：使用 `ref` + `nextTick` 逐条测量，列表变化后重算；避免每次渲染同步读取布局造成抖动

## Implementation Plan

- PR1：长文本折叠/展开（模板 + 状态 + 截断检测 + 单测）
- PR2：布局限宽 + header 精简 + 新建区强化
- PR3：卡片视觉与常驻操作按钮 + 已完成区跟随 + 文档同步
