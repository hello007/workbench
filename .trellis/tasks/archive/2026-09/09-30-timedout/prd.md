# 内容搜索部分超时前端感知收口（TimedOut 标志组）

## Goal

部分目录超时仍返结果时，前端目前零感知：用户看到缺目录的结果集且显示成功，无从得知应缩小范围重试。本任务把超时目录以 `TimedOut` 标志组混入结果返回，前端对该组渲染 warning 行提示，与 `E_SEARCH_TIMEOUT` 既有 warning 分流对称。

## What I already know（勘察结论）

### 后端现状

- `model/content_search.go:14` `ContentSearchGroup{RepoName, RepoPath, Items}`，无超时元数据字段。
- `service/content_search.go:84-148` goroutine 循环内 `timedOut` 分支（L121-126）仅 `timeoutCount++` 后 return，不产组；聚合点 L154-159：`timeoutCount > 0 && len(groups) == 0` 返 `E_SEARCH_TIMEOUT`（全部超时态），否则 `slog.Warn` 只带数量不带目录名，groups 非空时返 `(groups, nil)`。
- `app_search.go:28` 多目录共享 60s/10s ctx——部分超时无确定性端到端构造（ctx 一超时所有在途目录同超时），前序留笔判断正确。

### 方案 b 评审结论（可行，有一个关键陷阱）

**陷阱**：标志组若在 goroutine 内直接 append 进 `groups`，「全部超时」态 groups 非空，聚合判定 `len(groups)==0` 失效，会误走「返结果」路径，破坏 `E_SEARCH_TIMEOUT` 语义。

**解法**：goroutine 超时分支 append 进**独立 `timedOutGroups` slice**（带 `TimedOut: true` + RepoName/RepoPath）；聚合时：
- `len(groups) == 0 && len(timedOutGroups) > 0` → 报错（语义逐字不变）；
- `len(groups) > 0` → slog.Warn 带目录名 + `groups = append(groups, timedOutGroups...)` 尾部合并。

**聚合逻辑抽独立函数**（任务书预授权勘察后定）：抽 `mergeTimeoutGroups(groups, timedOutGroups)` 纯函数，三态直测（无超时透传 / 全部超时报错 / 部分超时合并），规避不可测缝隙。goroutine 内标志组构造逻辑（8 行字段赋值）不抽——抽「单目录搜索」会把 rg/go 分流、item 补全全带出去，为测而测；其字段赋值正确性缝隙（部分超时态才可见）接受，聚合用例用真实形态数据覆盖。

### 前端影响面核查（既有消费点全部安全）

| 消费点 | 位置 | 标志组影响 | 结论 |
|---|---|---|---|
| `contentGroups.length > 0` 渲染入口 | CommandPalette.vue:141 | 部分超时必有正常组，标志组混入不改判定语义 | 安全 |
| `totalResults` reduce | CommandPalette.vue:223 | 空 items 加 0 | 安全 |
| `getContentItemIndex` 等索引遍历 | CommandPalette.vue:238-242,250 | 遍历 items，空组跳过 | 安全 |
| `:key="group.repoName"` | CommandPalette.vue:142 | repoName 仍唯一 | 安全 |
| 空态判定 `contentGroups.length === 0` | CommandPalette.vue:175 | 不受影响 | 安全 |
| `group.timedOut` 向后兼容 | — | undefined falsy | 安全 |
| useCommandPalette.js | composable 层 | **零改动**（标志组透传，渲染层分流） | 不动 |

### wailsjs 绑定同步面

方案 b 签名不变 → App.js / App.d.ts **不动**；仅 models.ts `ContentSearchGroup` class（L947-960）加 `timedOut?: boolean` + constructor 赋值。CI `wails generate module` 会自动生成，本地手改保 build 绿。

### 测试现状

- 后端既有：`TestContentSearch_MultipleDirs`（无超时回归）、`TestContentSearch_TimeoutReturnsSearchTimeout`（全超时报错守护，须保持绿）、`TestContentSearch_RgFailureFallsBackToGo`（回退分支）。
- 前端既有：`useCommandPalette.spec.js`（含 E_SEARCH_TIMEOUT warning 弹窗分流用例 L173-182，自己 vi.mock 不走全局兜底表）；`CommandPalette.spec.js`（组件级）。
- `wails-mock-defaults.js` 现**无** `ContentSearch` 条目，须补 `ContentSearch: []`。
- E2E 无 command-palette 专属 spec，任务书只要求 vitest 三态，不新增 E2E 通道目录。

## 展示形态决策（任务书预授权勘察后定）

**面板内组级 warning 行**（组标题下、items 前），非 ElMessage.warning。理由：

1. 持久可见——命令面板打开期间一直在，ElMessage 3 秒消失易错过；
2. 与超时目录对号入座——标志组自带 repoName，ElMessage 只能说「N 个目录超时」；
3. 命令面板已有类 hint 块先例（`content-search-confirm` / `content-search-hint`），风格一致。

文案：`此目录搜索超时，结果可能不完整`。

## Requirements

1. `model/content_search.go` `ContentSearchGroup` 加 `TimedOut bool` 字段（`json:"timedOut,omitempty"`）。
2. `service/content_search.go`：goroutine 超时分支改 append 标志组进独立 `timedOutGroups`；聚合逻辑抽 `mergeTimeoutGroups` 纯函数；slog.Warn 改带目录名数组；`timeoutCount` 变量由 `len(timedOutGroups)` 替代。
3. `E_SEARCH_TIMEOUT` 报错路径语义逐字不变（全部超时且结果空仍返错误）。
4. `frontend/wailsjs/go/models.ts` `ContentSearchGroup` class 补 `timedOut` 字段（App.js/App.d.ts 零改动）。
5. `CommandPalette.vue` TimedOut 组组标题下渲染 warning 行；补组级 warning 样式。
6. `wails-mock-defaults.js` 补 `ContentSearch: []`。
7. 文档：`docs/spec/logging-and-errors.md` 收口段补句；CLAUDE.md 关键规则行去「留笔」改实现描述。

## Acceptance Criteria

- [ ] 后端：`TestMergeTimeoutGroups_*` 三态单测（无超时透传 / 全部超时报错且错误码 `E_SEARCH_TIMEOUT` / 部分超时正常组在前标志组在后标志位 true）全绿
- [ ] 后端：既有 `TestContentSearch_MultipleDirs` / `TestContentSearch_TimeoutReturnsSearchTimeout` / `TestContentSearch_RgFailureFallsBackToGo` 回归全绿（标志组不混入既有 groups 计数断言）
- [ ] 后端：`go test ./service/ -race` 全绿
- [ ] 前端：部分超时态（正常组+标志组 mock）warning 行渲染断言；无超时态无 warning 行断言；全部超时态走既有 error/warning 分流不变
- [ ] 前端：`npm test` 全绿，`npm run build` 绿（models.ts 同步验证）
- [ ] 文档同步完成

## Definition of Done

- 测试三态覆盖（后端聚合 + 前端渲染）
- go test + race + vitest 全绿
- 文档更新（logging-and-errors.md + CLAUDE.md）
- trellis-check + 子 agent 审核

## Out of Scope

- 不动 `searchWithRipgrep` / `searchWithGo` 搜索逻辑（9e1c96c 刚收口）
- 不改 `ContentSearch` Wails 方法签名（方案 b 前提）
- 不改 `useCommandPalette.js` composable
- 不新增 E2E 通道
- 不做 `timedOutDirs` 顶层字段方案（方案 a 已否决）
- 不解决「部分超时端到端确定性构造」（共享 ctx 结构性不可行，抽聚合函数覆盖）

## Technical Notes

- 勘察文件：`model/content_search.go`、`service/content_search.go`、`app_search.go`、`frontend/src/components/CommandPalette.vue`、`frontend/src/composables/useCommandPalette.js`、`frontend/src/utils/error.js`、`frontend/src/test/wails-mock-defaults.js`、`frontend/wailsjs/go/models.ts`、`service/content_search_test.go`、`frontend/src/composables/__tests__/useCommandPalette.spec.js`
- 部分超时不可端到端确定性构造的根因：`app_search.go:75` 单 ctx 贯穿全部目录，`ctx.Err() != nil` 时所有在途目录同时判超时
- 标志组尾部合并：groups 顺序本为 goroutine 完成序（非目录序），标志组无 items 不占结果交互位
- slog 数组值直接传 `[]string`，slog 原生支持
