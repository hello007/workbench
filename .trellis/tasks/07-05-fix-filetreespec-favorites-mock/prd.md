# 修复 FileTreePanel.spec.js 缺 GetFavorites mock 致全量测试 exit 1

## Goal

全量 `npx vitest run` 因 `FileTreePanel.spec.js` 缺 `GetFavorites` 等 mock 而 exit 1（31 个 unhandled rejection）。补全缺失的 mock 导出，让全量测试恢复绿。

## What I already know

### 根因

- `FileTreePanel.vue:1222` 使用 `useFavorites()`。
- `useFavorites.js:2` 从 `wailsjs/go/main/App` import 5 个方法：`GetFavorites`、`AddFavorite`、`RemoveFavorite`、`UpdateFavoriteAlias`、`UpdateFavoriteGroup`。
- `FileTreePanel.spec.js:22-34` 的 `vi.mock('../../../wailsjs/go/main/App')` 只 mock 了 11 个方法（`GetFileTree`/`GetGitInfo`/`CreateDirectory`/...），**未包含上述 5 个 favorites 相关方法**。
- FileTreePanel mount 时 `loadFavorites()` 立即调用 `GetFavorites()` → mock 中无此 export → `[vitest] No "GetFavorites" export is defined` unhandled rejection（共 31 次，导致全量 exit 1）。

### 参考实现

`Home.spec.js:54-58` 已正确 mock 这 5 个方法，返回值约定如下（保持一致）：

```javascript
GetFavorites: vi.fn(() => Promise.resolve([])),
AddFavorite: vi.fn(() => Promise.resolve(true)),
RemoveFavorite: vi.fn(() => Promise.resolve(true)),
UpdateFavoriteAlias: vi.fn(() => Promise.resolve(true)),
UpdateFavoriteGroup: vi.fn(() => Promise.resolve(true)),
```

## Requirements

- 在 `FileTreePanel.spec.js` 的 `vi.mock('../../../wailsjs/go/main/App')` 对象内补全上述 5 个 favorites 相关方法，返回值与 `Home.spec.js` 一致。

## Acceptance Criteria

- [ ] `cd frontend && npx vitest run`（全量）exit 0，不再报 `GetFavorites` 相关 unhandled rejection。
- [ ] `FileTreePanel.spec.js` 全部用例通过。
- [ ] `npm run build` 不受影响。

## Definition of Done

- 全量 `vitest run` 绿；`npm run build` 成功。

## Technical Approach

在 `FileTreePanel.spec.js:22-34` 的 mock 对象末尾追加 5 行 favorites mock（复制 `Home.spec.js` 的返回值约定）。改动单文件、约 5 行。

## Decision (ADR-lite)

**Context**：`FileTreePanel.spec.js` 的 App mock 未跟上 `useFavorites` 的依赖，缺 5 个 export 导致 unhandled rejection。

**Decision**：按 `Home.spec.js` 既有约定补全 5 个 mock，返回值保持一致。

**Consequences**：全量测试恢复绿；不抽共享 mock 工厂（本次仅修复 exit 1，多处 spec 重复 mock 的治理列为后续）。

## Out of Scope

- 不改 `FileTreePanel.vue` 或 `useFavorites.js`（生产行为正确，仅测试 mock 缺失）。
- 不抽取共享 mock 工厂（多处 spec 重复 mock App，本次不治理）。

## Technical Notes

- 改动文件：`frontend/src/components/__tests__/FileTreePanel.spec.js`（单文件，~5 行新增）。
- mock 返回值与 `Home.spec.js` 一致以保持项目惯例。
