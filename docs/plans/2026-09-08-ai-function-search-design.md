# AI 功能项搜索 / 分组 / 置顶设计

**日期**：2026-09-08
**优先级**：P2
**状态**：待评审（要点级，实施前需补充代码探查）

## 1. 概述

功能项数量增长后（当前 4 项 seed，未来十几个），平铺卡片无检索成本上升。新增模糊搜索、标签分组、手动置顶，保持高频功能快速触达。

## 2. 现状与痛点

`AiFunctionPanel.vue` 第 20-31 行，功能列表 `v-for f in functions` 平铺渲染 `.ai-card`，无搜索、无分组、无置顶：

```vue
<div class="ai-cards">
  <div v-for="f in functions" class="ai-card" @click="openFunctionTab(f)">
```

| 痛点 | 说明 |
| --- | --- |
| 无搜索 | 十几个功能需肉眼找 |
| 无分组 | 不同业务域（周报/会议/文档）混排 |
| 无置顶 | 高频功能与低频功能同序 |

## 3. 需求总结

1. 列表上方搜索框，按 `name` / `description` / `tags` 模糊匹配
2. `tags` 字段分组，支持按 tag 筛选或折叠分组渲染
3. `pinned` 字段置顶，始终排在最前
4. 配置对话框支持编辑 `tags` / `pinned`

## 4. 设计要点

### 4.1 model 扩展

`AiFunction` 增字段：

```go
Tags   []string `json:"tags,omitempty"`   // 业务域标签，如 ["周报","ABX5"]
Pinned bool     `json:"pinned,omitempty"` // 置顶
```

### 4.2 列表排序与筛选

前端计算属性 `sortedFunctions`：

1. `pinned` 优先
2. 同组内按现有顺序（配置文件顺序）
3. 搜索框有值时按 `name`/`description`/`tags` 模糊匹配过滤

### 4.3 分组渲染（两种方案待选）

| 方案 | 说明 | 适合场景 |
| --- | --- | --- |
| A. tag chips 筛选 | 顶部展示所有 tag 的 chip，点击筛选；平铺渲染命中项 | 功能数中等，跨 tag 查找 |
| B. 按 tag 折叠分组 | 按 tag 分组渲染，每组可折叠；无 tag 归「其他」 | 功能多，按业务域浏览 |

> **待确认**：选 A 还是 B，或两者结合（chips 筛选 + 剩余平铺）。建议先 A，功能数超 15 再考虑 B。

### 4.4 配置对话框

`AiFunctionConfigDialog.vue` 基础字段区增：

- `tags`：逗号分隔输入（同 `addDirs` 处理）或 chip 输入
- `pinned`：`el-switch` 开关

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `model/ai_function.go` | 修改 | `AiFunction` 增 `Tags` / `Pinned` |
| `frontend/src/components/AiFunctionPanel.vue` | 修改 | 搜索框、排序、分组渲染 |
| `frontend/src/components/AiFunctionConfigDialog.vue` | 修改 | tags / pinned 编辑 |
| `frontend/src/components/__tests__/AiFunctionPanel.spec.js` | 修改 | 搜索、置顶、分组测试 |

## 6. 待确认点

1. 分组方案 A/B/结合（4.3 节）
2. `tags` 是否预定义可选集（如「周报/会议/文档/发版」枚举）还是自由输入？建议自由输入 + 历史去重提示
3. seed 四功能是否预置 tags？建议是，作为分组示例

## 7. 关联

- 来源：[[2026-09-08-ai-skill-menu-design]] 后续优化建议
- 配套：[[2026-09-08-ai-config-form-design]]（tags/pinned 编辑入口）
