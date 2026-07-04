# 修复 Git 仓库信息偶发显示 N/A

## Goal

点击文件树中的 git 仓库节点时，右侧"Git 仓库信息"面板的"最新提交 / 提交时间 / 提交消息"偶发显示 N/A，需手动点右上角刷新才正常。定位并修复根因，使首次进入即稳定展示，无需手动刷新。

## What I already know（已查明的事实与根因）

### 数据流

```
Home.vue
  latestCommit = ref(null)   ← 全局唯一，所有仓库共享
  ├─ selectDirectory() 时清零 latestCommit
  ├─ onNodeSelect(data) 时只更新 selectedNode，【不清零 latestCommit】
  └─ <ContentPanel :latest-commit @latest-commit>
       ├─ <GitInfo :repo-path :latest-commit>          ← "仓库信息" tab（非 lazy）
       │     effectiveLatestCommit = props.latestCommit || localLatestCommit
       │     loadGitInfo(forceRefresh):
       │        缓存命中 → gitInfo = cached; return   ←【不加载、不恢复 commit】
       │        未命中  → GetGitRemoteURL + GetCommitHistory(1)
       │                  localLatestCommit = commits[0]
       │                  gitCache.set(key, info)      ←【缓存只存 info】
       └─ <CommitHistory @latest-commit>               ← "提交历史" tab（lazy）
              仅在此 tab 渲染后才 emit('latest-commit', commits[0])
```

### 根因（双病灶，同源）

1. **GitInfo 的缓存只存 `info`，不存 `latestCommit`**
   缓存命中分支 `gitInfo.value = cached; return` 直接返回，既不恢复也不重新加载 `localLatestCommit`。此时 `effectiveLatestCommit` 只能依赖 `props.latestCommit`。

2. **`props.latestCommit` 在 lazy tab 下常态为 null**
   `latestCommit` 由 `CommitHistory` emit，而"提交历史"tab 是 `lazy`——用户没点进去之前 CommitHistory 不渲染、不 emit。Home.vue 的 `latestCommit` 恒为 null（或残留值）。
   - 结合病灶 1：缓存命中 + 未访问过提交历史 tab → `effectiveLatestCommit = null` → **显示 N/A**。这就是"偶发"的本质（只在 5 分钟内重复进入某仓库且没看过提交历史时触发）。
   - 手动刷新走 `forceRefresh=true` 的非缓存分支，重新拉到 `commits[0]`，故可恢复。

3. **衍生问题：跨仓库 `latestCommit` 残留**
   `onNodeSelect` 不清零 `latestCommit`。在仓库 X 看过提交历史后（`latestCommit = X`），点击仓库 Y：`effectiveLatestCommit = props.latestCommit(X) || localLatestCommit`，会优先显示 **X 的提交**（错误仓库数据）。GitInfo 的 `watch(repoPath)` 也只清 `gitInfo`、不清 `localLatestCommit`，同样存在残留。

### 关键代码位置

- `frontend/src/components/GitInfo.vue:100-133`（`effectiveLatestCommit` / `loadGitInfo`）
- `frontend/src/components/GitInfo.vue:170-173`（`watch(repoPath)`）
- `frontend/src/views/Home.vue:262-266`（`onNodeSelect` 未清 `latestCommit`）
- `frontend/src/components/ContentPanel.vue:28-42`（GitInfo 非 lazy / CommitHistory lazy）
- `frontend/src/utils/gitCache.js`（5 分钟 TTL 的内存缓存；key=`type:path`，value 为任意对象）

## Requirements

- 点击任意 git 仓库节点，"最新提交 / 提交时间 / 提交消息"在缓存命中时也能稳定展示（含首次未访问提交历史 tab 的场景）。
- 切换不同仓库节点时，不残留上一个仓库的提交信息（覆盖 `props.latestCommit` 与 `localLatestCommit` 两条来源）。
- `GetCommitHistory` 偶发失败时，不把 `null` 提交写入缓存（避免 5 分钟内重进仍 N/A），下次进入自动重试。
- 保留现有手动刷新能力与 5 分钟缓存性能收益。
- 空仓库（无任何提交）维持当前 N/A 显示（与现状一致）。

## Acceptance Criteria

- [ ] 同一仓库 5 分钟内重复进入，提交信息不丢失、不显示 N/A（缓存命中恢复 latestCommit）。
- [ ] 仓库 X→Y 切换后，Y 面板显示的是 Y 的最新提交，不残留 X 的提交（`onNodeSelect` 清零 + `watch(repoPath)` 清 `localLatestCommit`）。
- [ ] 未访问"提交历史"tab 时，"仓库信息"tab 首次加载即显示提交信息。
- [ ] `GetCommitHistory` 失败时缓存不写入 null commit，再次进入可自动重试恢复。
- [ ] 手动刷新仍可强制更新到最新提交（`forceRefresh` 链路不破坏）。
- [ ] `onLocalChangesCommitted` 联动刷新（提交/推送后）仍正常更新仓库信息。
- [ ] 空仓库显示 N/A，不报错。

## Definition of Done

- 新增 `GitInfo.spec.js`，覆盖：缓存命中恢复 latestCommit、切换仓库清空残留、commit 失败不污染缓存三条用例。
- `Home.spec.js` 增补：跨仓库节点切换时 `latestCommit` 被清零。
- `cd frontend && npm run build` 与 lint 通过；`npm test` 全绿。
- README / 功能说明如涉及行为变化则同步（本次为 bug 修复，预期无需改文档）。

## Technical Approach

缓存结构升级 + 状态生命周期清理（方案 A + 防御性失败处理）。

1. **`GitInfo.vue` `loadGitInfo`**
   - 改用 `Promise.allSettled` 并发 `GetGitRemoteURL` 与 `GetCommitHistory(path, 1, 0)`，区分成功/失败。
   - 缓存 value 由 `info` 升级为 `{ info, latestCommit }`。
   - 缓存命中：`gitInfo = cached.info`、`localLatestCommit = cached.latestCommit`。
   - 写缓存：`info` 成功即写；`latestCommit` 仅在 `GetCommitHistory` 成功时写入，失败时该字段不落缓存（命中时若该字段缺失则视为需重试，仅在本次展示为 N/A，下次进入重拉）。
2. **`GitInfo.vue` `watch(repoPath)`**：切换时同步清空 `gitInfo` 与 `localLatestCommit`。
3. **`Home.vue` `onNodeSelect`**：切换文件树节点时 `latestCommit.value = null`。

### 缓存 key 影响范围

GitInfo 使用 `'git-info:' + path` 作为 cacheKey。缓存 value 结构升级仅影响该 key 的读写，均在 GitInfo 内部闭环。实现前已 grep 确认 `gitCache` 全部消费方（见 Technical Notes），无其他组件读写 `'git-info:'` key。

## Decision (ADR-lite)

**Context**：GitInfo 缓存只存 `info` 不存 `latestCommit`，且 `latestCommit` prop 在 lazy tab 下常态为 null，导致缓存命中时提交信息丢失显示 N/A；跨仓库切换 `latestCommit` 还会残留导致显示错误仓库提交。手动刷新因走非缓存分支可临时恢复。

**Decision**：采用方案 A ——
1. 缓存结构升级为 `{ info, latestCommit }`，命中时一并恢复；
2. `watch(repoPath)` 与 `onNodeSelect` 切换时清空相关状态；
3. `GetCommitHistory` 失败时不把 null commit 写入缓存（防御性，避免污染导致重进仍 N/A）。

**Consequences**：
- 改动集中（`GitInfo.vue` + `Home.vue`，约 20 行），保留 5 分钟缓存性能收益。
- 缓存内 `latestCommit` 最多陈旧 5 分钟（可接受，手动刷新可强制更新）。
- 空仓库仍显示 N/A（维持现状，友好文案显式排除）。
- 未做"统一 git info 数据源"的架构重构（YAGNI）。

## Implementation Plan

- **步骤 1（GitInfo.vue 缓存与状态）**：`loadGitInfo` 改 `Promise.allSettled`；缓存 value 升级 `{info, latestCommit}`；命中分支恢复双字段；commit 失败不写 latestCommit。
- **步骤 2（GitInfo.vue watch 清理）**：`watch(repoPath)` 增加清空 `localLatestCommit`。
- **步骤 3（Home.vue 节点切换）**：`onNodeSelect` 增加清零 `latestCommit`。
- **步骤 4（测试）**：新增 `GitInfo.spec.js` 三条用例；`Home.spec.js` 增补跨仓库切换用例。
- **步骤 5（回归验证）**：手动验证 Acceptance Criteria；确认 `onLocalChangesCommitted` 联动刷新正常。

## Out of Scope（显式）

- 不改动后端 `GetGitRemoteURL` / `GetCommitHistory` 接口与实现。
- 不重构 `CommitHistory` 的 lazy 加载策略，不调整 `gitCache` 的 5 分钟 TTL。
- 不做空仓库"暂无提交"友好文案与失败轻提示（列为后续体验优化任务）。
- 不推动 GitInfo 向"父组件统一喂数、纯展示"架构演进。

## Technical Notes

- 后端 `app.go:336 GetGitRemoteURL` 返回 `GitRemoteInfo{remoteUrl, branch, isDetached}`；`app.go:386 GetCommitHistory(path, 1, 0)` 返回最新 1 条 `Commit`（含 sha/shortSha/message/timestamp/dateTime）。
- `model.GitRepoInfo`（models.go:84）与前端 `Commit`（models.ts:77）是两套并存模型；GitInfo 实际用的是 `Commit`（来自 GetCommitHistory），与 `GitRepoInfo.Commits` 无关。
- `gitCache`（`frontend/src/utils/gitCache.js`）为通用内存缓存，`getCacheKey(type, path)` 生成 `type:path` 键。已 grep 确认全仓库仅 `GitInfo.vue` 消费 `gitCache`（`CommitHistory` 不走缓存），value 结构升级无跨组件回归风险。
- 测试基建：Vitest + Vue Test Utils，组件测试位于 `frontend/src/**/__tests__/*.spec.js`，已有 `ContentPanel.spec.js`、`Home.spec.js` 可参考。

## Research References

- 本任务为内部代码根因定位，无需外部调研，未产生 `research/*.md`。
