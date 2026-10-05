# 内容搜索超时目录单目录重试

## Goal

承接 4ccccc6 TimedOut 标志组收口：命令面板对超时目录渲染 warning 行「此目录搜索超时，结果可能不完整」后，用户唯一出路是整体重搜（多目录重搜可能再次部分超时）。本任务提供「只重搜超时目录」便捷出口：warning 行可点击触发单目录搜索 + loading 态 + 结果组原位替换。

## What I already know（勘察结论）

- `service/content_search.go` `ContentSearch` 已支持 `dirs []string` 任意子集，本任务 service 层零改动（约束：不动 mergeTimeoutGroups / searchWithRipgrep / searchWithGo）。
- `app_search.go` `ContentSearch(keyword, fileExt, subDir, searchAll)` 只有「全部 / 仅默认目录」两态；无指定目录列表入口。
- `mergeTimeoutGroups` 语义：单目录重试再超时时 `len(groups)==0` 返 `E_SEARCH_TIMEOUT` error，前端拿不到标志组——前端 catch 时保留原标志组即满足「再超时 warning 行保留」。
- `CommandPalette.vue` 标志组 warning 行现无交互；正常组行点击 = openContentResultInVSCode；`getContentItemIndex`/`getContentItemByIndex` 按 contentGroups 数组 offset 遍历，组增删自动适配，键盘导航零改动兼容。
- `useCommandPalette.js` `close()`/`onClose()` 清 contentGroups 语义保留；`onInput` 改词只清 contentSearchExecuted 不清 contentGroups（既有行为）——重试参数不能用实时 contentQuery（用户可能已改词），须在 executeContentSearch 时快照 `{keyword,fileExt,subDir}`。
- wails 绑定三处：App.js:125 / App.d.ts:66 / models.ts:947（ContentSearchGroup 已含 timedOut）。
- mock defaults：`wails-mock-defaults.js:22` `ContentSearch: []`，新增绑定方法须补默认值。
- 测试面：`CommandPalette.spec.js` / `useCommandPalette.spec.js` 已存在；无 command-palette 专属 E2E spec（本任务不新增 E2E 通道，避开 vitest exclude 坑）。

## Requirements

1. 后端：App 层新增指定目录列表搜索入口（签名方案见 Open Questions）。
2. 前端：TimedOut 组 warning 行升级为可点击重试——文案 + 图标 + loading + 结果替换。
3. 重试复用 60s ctx 语义（单目录重试给大目录第二次机会，10s 会立刻再超时）。
4. 重试成功：标志组移除，返回组（含 items）插入；返回空（零命中）：标志组移除后展示「未找到匹配内容」空态。
5. 重试失败（再超时 E_SEARCH_TIMEOUT）：保留原标志组 + handleError 分流提示。
6. 测试：composable 重试用例 + 组件标志组交互用例 + 后端指定目录搜索用例；文档同步。

## Acceptance Criteria

- [ ] 单目录重试端到端可用：重试成功标志组消失、有命中插入结果组、零命中展示空态
- [ ] 再超时 warning 行保留 + warning 级提示
- [ ] 重试 loading 态防重复触发
- [ ] 键盘导航（offset 链）在组增删后仍正确
- [ ] 既有 1452 vitest + go test 回归全绿
- [ ] wailsjs 三处绑定同步（若采用改签名方案）或纯新增（若新增方法方案）
- [ ] 测试禁 sleep

## Definition of Done

- 单测/组件测试覆盖新链路
- lint / typecheck / 回归绿
- README.md / docs/spec 文档按改动面同步

## Out of Scope

- 不动 mergeTimeoutGroups / searchWithRipgrep / searchWithGo
- 不改 E_SEARCH_TIMEOUT 全部超时报错语义
- 不新增 E2E 通道目录
- 不做「多超时目录批量重试」（MVP 单组单目录，逐个点）

## Decision (ADR-lite)

**Context**：两个决策点已提问用户未答（cc-connect 异步场景），按勘察推荐方案定，用户可随时推翻。

**Decision**：
1. 签名方案 A——新增独立方法 `ContentSearchDirs(keyword, fileExt, subDir string, dirs, repoNames []string)`。保 ContentSearch 旧签名零变更；绑定为纯新增（App.js/App.d.ts 增行，models.ts 无新类型零改动）；既有调用点与 mock 签名零风险；App 层抽公共 helper 两方法共用。
2. 交互形态——整行可点击 + 行尾「重试图标+文字」标识。热区大，与既有 result-item hover 语言一致；loading 时行内图标转圈禁重复点击。

**Consequences**：App 层多一个方法（153→154 绑定，API参考.md 须同步）；未来若更多入口需要指定目录语义，ContentSearchDirs 即扩展位。

## Open Questions

（已收敛，见 Decision）

## Technical Notes

- 重试时序关键点：重试参数快照自 executeContentSearch 时刻（lastContentQuery），不取实时 contentQuery。
- ContentSearchDirs 透传 dirs/repoNames 给 service.ContentSearch，60s ctx。
- App 层抽公共 helper（目录解析 + ctx + 调 service），ContentSearch 与 ContentSearchDirs 共用。
