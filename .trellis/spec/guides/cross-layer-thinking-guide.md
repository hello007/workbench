# Cross-Layer Thinking Guide

> **Purpose**: Think through data flow across layers before implementing.

---

## The Problem

**Most bugs happen at layer boundaries**, not within layers.

Common cross-layer bugs:
- API returns format A, frontend expects format B
- Database stores X, service transforms to Y, but loses data
- Multiple layers implement the same logic differently

---

## Before Implementing Cross-Layer Features

### Step 1: Map the Data Flow

Draw out how data moves:

```
Source → Transform → Store → Retrieve → Transform → Display
```

For each arrow, ask:
- What format is the data in?
- What could go wrong?
- Who is responsible for validation?

### Step 2: Identify Boundaries

| Boundary | Common Issues |
|----------|---------------|
| API ↔ Service | Type mismatches, missing fields |
| Service ↔ Database | Format conversions, null handling |
| Backend ↔ Frontend | Serialization, date formats |
| Component ↔ Component | Props shape changes |

### Step 3: Define Contracts

For each boundary:
- What is the exact input format?
- What is the exact output format?
- What errors can occur?

---

## Common Cross-Layer Mistakes

### Mistake 1: Implicit Format Assumptions

**Bad**: Assuming date format without checking

**Good**: Explicit format conversion at boundaries

### Mistake 2: Scattered Validation

**Bad**: Validating the same thing in multiple layers

**Good**: Validate once at the entry point

### Mistake 3: Leaky Abstractions

**Bad**: Component knows about database schema

**Good**: Each layer only knows its neighbors

---

## Checklist for Cross-Layer Features

Before implementation:
- [ ] Mapped the complete data flow
- [ ] Identified all layer boundaries
- [ ] Defined format at each boundary
- [ ] Decided where validation happens

After implementation:
- [ ] Tested with edge cases (null, empty, invalid)
- [ ] Verified error handling at each boundary
- [ ] Checked data survives round-trip

---

## Cross-Platform Template Consistency

In Trellis, command templates (e.g., `record-session.md`) exist in **multiple platforms** with identical or near-identical content. This is a cross-layer boundary.

### Checklist: After Modifying Any Command Template

- [ ] Find all platforms with the same command: `find src/templates/*/commands/trellis/ -name "<command>.*"`
- [ ] Update all platform copies (Markdown `.md` and TOML `.toml`)
- [ ] For Gemini TOML: adapt line continuations (`\\` vs `\`) and triple-quoted strings
- [ ] Run `/trellis:check-cross-layer` to verify nothing was missed

**Real-world example**: Updated `record-session.md` in Claude to use `--mode record`, but forgot iFlow, Kilo, OpenCode, and Gemini — caught by cross-layer check.

---

## Generated Runtime Template Upgrade Consistency

Some generated files are both documentation and runtime input. In Trellis,
`.trellis/workflow.md` is parsed by `get_context.py`, `workflow_phase.py`,
SessionStart filters, and per-turn hooks. Template changes must be validated
against both fresh init and upgrade paths.

### Checklist: After Modifying A Runtime-Parsed Template

- [ ] Identify every runtime parser that reads the template, not just the file
  writer that installs it
- [ ] Check whether relevant syntax lives outside obvious managed regions
  such as tag blocks
- [ ] Verify fresh `init` output and a versioned `update` scenario that writes
  the older `.trellis/.version`
- [ ] Add an upgrade regression using an older pristine template fixture, then
  assert the installed file reaches the current packaged shape
- [ ] Update the backend spec that owns the runtime contract

**Real-world example**: Codex inline mode changed workflow platform markers from
`[Codex]` / `[Kilo, Antigravity, Windsurf]` to `[codex-sub-agent]` /
`[codex-inline, Kilo, Antigravity, Windsurf]`. Fresh init was correct, but
`trellis update` only merged `[workflow-state:*]` blocks and preserved stale
markers outside those blocks. Result: upgraded projects got new hook scripts
but old workflow routing, so `get_context.py --mode phase --platform codex`
could return empty Phase 2.1 detail.

---

## Mode-Detection Probe Checklist

When a CLI auto-detects a mode by probing a remote resource (e.g., checking if `index.json` exists to decide marketplace vs direct download):

### Before implementing:
- [ ] Probe runs in **ALL** code paths that use the result (interactive, `-y`, `--flag` combos)
- [ ] 404 vs transient error are distinguished — don't treat both as "not found"
- [ ] Transient errors **abort or retry**, never silently switch modes
- [ ] Shared state (caches, prefetched data) is **reset** when context changes (e.g., user switches source)
- [ ] **Shortcut paths** (e.g., `--template` skipping picker) must have the same error-handling quality as the probed path — check that downstream functions don't call catch-all wrappers

### After implementing:
- [ ] Trace every path from probe result to the mode-decision branch — no fallthrough
- [ ] External format contracts (giget URI, raw URLs) are tested or at least documented as comments
- [ ] Metadata reads consume a complete response or use a streaming parser — never parse a fixed-size prefix as full JSON
- [ ] When reconstructing a composite identifier from parsed parts, verify **all** fields are included and in the **correct position** (e.g., `provider:repo/path#ref` not `provider:repo#ref/path`)
- [ ] Verify that **action functions** called after a shortcut don't internally use the old catch-all fetch — they must use the probe-quality variant when error distinction matters

**Real-world example**: Custom registry flow had 8 bugs across 3 review rounds: (1) probe only ran in interactive mode, (2) transient errors fell through to wrong mode, (3) giget URI had `#ref` in wrong position, (4) prefetched templates leaked across source switches, (5) `--template` shortcut bypassed probe but `downloadTemplateById` internally used catch-all `fetchTemplateIndex`, turning timeouts into "Template not found".

**Real-world example**: Agent-session update hints fetched npm `latest` metadata with `response.read(4096)` and then parsed it as complete JSON. The `@mindfoldhq/trellis` package metadata exceeded 4 KB, so the JSON was truncated, parse failed silently, and the first session injection showed no update hint. Fix: read the complete response before parsing, and add a regression where `version` is followed by an 8 KB metadata tail.

---

## UI Local Refresh vs Whole-Tree Rebuild

In stateful UI components (file trees, table trees, virtualized lists), the
boundary between *external data update* and *internal component state*
(expanded/selected/scroll) is a real layer boundary. The classic Vue/React
reflex of "bump a reactive counter, change the component `key` to force
re-mount" is fast to write but **destroys all internal state** in one shot.

### The Anti-pattern

```vue
<!-- DON'T: key-bumping to "refresh" -->
<el-tree :key="treeKey" ... />
```

```js
const refreshCounter = ref(0)
const treeKey = computed(() => `${dirId}_${refreshCounter.value}`)

const refreshNode = (path) => {
  const node = tree.store.nodesMap[path]
  if (node) {
    node.loaded = false
    node.expand()
  } else {
    refreshCounter.value++   // ← whole-tree rebuild, eats expanded state
  }
}
```

The function reads as "refresh one node", but the fallback silently upgrades
to "rebuild the whole tree". Any caller that hands in an unfamiliar path
(e.g., a freshly user-typed copy-to target) detonates the user's expanded
state.

### Checklist: Refreshing State-Carrying Components

- [ ] List the internal state you'd lose on remount (expanded paths, selected
      keys, scroll position, focus, lazy-load caches). If any matter to UX,
      `:key="counter"` is off the table for routine refresh.
- [ ] Distinguish *user-initiated whole refresh* (a "Refresh" button — rebuild
      is fine, expected) from *side-effect triggered local refresh* (after
      copy/move/create/delete — must NOT rebuild).
- [ ] For local refresh, define what "the path doesn't exist in the component
      cache" means. Three usable strategies:
      1. **Walk up to the nearest expanded ancestor and refresh it** — the user
         sees what they need; deeper nodes stay collapsed naturally.
      2. **Save → rebuild → restore expanded paths** — works but causes flicker
         and depends on async re-expansion timing.
      3. **Silently no-op** — the cheapest and often correct choice when the
         path is outside the user's current viewport anyway.
- [ ] Never let "I couldn't find the node" fall through to "rebuild the
      world". That conflates two unrelated semantics.
- [ ] Reuse the path-normalization (separator handling, root containment) that
      already exists in the component (`locateNode`-style helpers); don't
      reinvent per-fallback.

### Real-world example

`FileTreePanel.vue` exposed `refreshNode(path)`. The else-branch did
`refreshCounter.value++` to force a `:key` change as the "we don't know that
path, just rebuild" fallback. Callers were:

| Caller | Path passed | Hit rate on `nodesMap` |
|---|---|---|
| 右键 refresh | the clicked node's own path | 100% |
| create / rename / delete | the affected parent's path | high (parent was usually expanded) |
| copy-to (Home.vue handleCopyTo) | a user-typed **target** path | ~0% |

The bug was invisible for the first two callers because they almost always
hit `nodesMap`. Copy-to surfaced it every single time. Fix: replace the
counter-bump fallback with "walk up to the nearest expanded ancestor and
refresh it; otherwise no-op". The user-initiated `refreshAll` button kept the
counter-bump path on a separate code path — that semantic is intentional.

---

## When to Create Flow Documentation

Create detailed flow docs when:
- Feature spans 3+ layers
- Multiple teams are involved
- Data format is complex
- Feature has caused bugs before

---

## 组件状态切换：避免中间态卸载与缓存命中丢字段

> 适用：Vue 组件选中状态切换、带内存缓存的组件加载。两个反模式均在本仓库真实出现过。

### 反模式 1：切换状态经历 `null` 中间态 → `v-if` 卸载再挂载

**症状**：切换选中项时内容面板先清空再加载（双刷新/闪烁），子组件销毁重建触发重复请求。

**原因**：先 `selectedNode = null` 清空、`await nextTick()` 后再设目标值。模板 `<div v-if="selectedNode">` 在 `null` 时卸载整个子树，目标值设置时重新挂载——内部子组件（如 GitInfo）销毁重建，`loadGitInfo` 重新执行。

**修正**：直接从旧值切到目标值，不经历 `null`。"切到非 git 目录需清空"由"目标值即 `null`"自然实现。

```javascript
// Wrong（双刷新）
selectedNode.value = null
await nextTick()
if (newDir.isGitRepo) selectedNode.value = { ...gitNode }

// Correct（单次刷新，与文件树切换一致）
const newDir = directories.value.find(d => d.id === dirId)
selectedNode.value = newDir?.isGitRepo ? { ...gitNode } : null
await nextTick()
fileTreePanelRef.value?.restoreTreeState(newDir.path)
```

**Checklist**：
- [ ] 模板中 `v-if` 依赖被切换状态的节点——中间值（null/undefined）是否会触发整子树卸载？
- [ ] "清空旧选中"能否由"目标值即空"自然实现，避免先 null 后设值？

**Real-world**：`Home.vue onDirectorySelect` 切换 git 工作目录双刷新（任务 07-05）。

### 反模式 2：缓存命中只恢复部分字段 → 派生渲染字段丢失

**症状**：带缓存的组件命中缓存时，部分渲染字段显示 N/A，需手动刷新才恢复。

**原因**：缓存只存部分状态（如 `info`），但渲染依赖与 `info` 同源加载的派生字段（如 `latestCommit`）。命中分支只恢复缓存字段就 `return`，派生字段丢失。常叠加"prop 数据源在 lazy tab 下常态为 null"——命中时两个数据源都为空。

**修正**：缓存结构覆盖渲染所需的**全部**派生字段，命中时一并恢复。失败字段不落缓存（用 `Promise.allSettled` 区分成败），下次进入自动重试，避免污染缓存导致持续 N/A。

```javascript
// Wrong（命中丢 latestCommit → 显示 N/A）
if (cached) { gitInfo.value = cached; return }   // cached 仅是 info
gitCache.set(key, info)

// Correct（缓存覆盖派生字段 + 失败不污染）
if (cached) {
  gitInfo.value = cached.info
  localLatestCommit.value = cached.latestCommit   // 一并恢复
  return
}
// 仅 commit 成功才落缓存；失败时本次不缓存，下次进入重拉两边
if (commitsRes.status === 'fulfilled') {
  gitCache.set(key, { info: infoRes.value, latestCommit })
}
```

**Checklist**：
- [ ] 命中分支是否恢复了组件渲染所需的全部字段（不只缓存的部分）？
- [ ] 缓存写入是否覆盖所有从该次加载派生的渲染字段？
- [ ] 某字段加载失败时，是否避免把 `null` 写入缓存（防止 TTL 内重试仍失败）？
- [ ] 组件若同时依赖缓存 + 父组件 prop 两个数据源，切换时两者是否都清理残留？

**Real-world**：`GitInfo.vue loadGitInfo` 缓存命中偶发丢失最新提交（任务 07-04）。
