# 修复工作目录切换 git 仓库内容面板双刷新

## Goal

左侧"工作目录树"切换到一个 git 仓库工作目录时，右侧内容面板会刷新两次（先清空再加载，视觉闪烁）；而文件树切换 git 仓库节点时仅刷新一次。消除双刷新，使工作目录切换与文件树切换体验一致。

## What I already know（已查明的事实与根因）

### 触发路径对比

```
onDirectorySelect（工作目录树切换，Home.vue:227-259）
  selectedDirectoryId = dirId
  selectedNode = null            ← (B) 第一次状态变化
  latestCommit = null
  clearPreview()
  await nextTick()               ← DOM 更新，用户看到空白
  if (newDir.isGitRepo) {
    selectedNode = {git节点}     ← (C) 第二次状态变化
  }

onNodeSelect（文件树切换，Home.vue:262-269）
  selectedNode = data            ← 一次设置（旧值→新值，无 null 中间态）
  latestCommit = null
  clearPreview()
```

### 根因

ContentPanel 模板（ContentPanel.vue:3）：

```html
<div v-if="selectedNode" class="content-inner">
  ...
  <el-tabs v-if="selectedNode.isGitRepo">
    <GitInfo :repo-path="selectedNode.path" ... />
```

- `selectedNode=null` → `v-if="selectedNode"` 失败 → **整个 content-inner 卸载**（GitInfo 组件销毁）→ 第一次刷新（清空）。
- `nextTick` 后 `selectedNode={git节点}` → `v-if` 通过 → **content-inner 重新挂载**（GitInfo 重建、`loadGitInfo` 重新执行）→ 第二次刷新（加载）。
- 文件树切换无 `null` 中间态，content-inner 始终挂载，GitInfo 仅 `repoPath` 变化触发 `watch` 单次 `loadGitInfo` → 单次刷新。

### 关键确认

- ContentPanel **无 `watch(selectedNode)`**（仅 props 透传给 GitInfo/CommitHistory/LocalChanges），跳过 `null` 中间态不会触发副作用，安全。
- `gitCache` 为模块单例，组件销毁/重建不影响缓存命中。

### 关键代码位置

- `frontend/src/views/Home.vue:227-259`（`onDirectorySelect`，含 `null` 中间态）
- `frontend/src/components/ContentPanel.vue:3`（`v-if="selectedNode"` 卸载/挂载边界）

## Requirements

- 工作目录树切换到 git 仓库时，内容面板单次刷新（无"先空后载"闪烁），体验与文件树切换 git 仓库节点一致。
- git 工作目录 A→B 切换，GitInfo 显示 B 的信息（不残留 A）。
- 切到非 git 工作目录时，内容面板正常清空（单次）。
- 保留现有行为：`saveCurrentState`/`restoreTreeState`（文件树状态保存恢复）、`clearPreview`、`latestCommit` 清零。
- 文件树切换行为不受影响（回归）。

## Acceptance Criteria

- [ ] 工作目录树切换 git 仓库，内容面板单次刷新，无闪烁。
- [ ] gitA → gitB 工作目录切换，GitInfo 显示 B 的远程/分支/最新提交。
- [ ] git → 非 git 工作目录切换，内容面板清空（无残留 git 面板）。
- [ ] 文件树切换 git 仓库行为不受影响（单次刷新）。
- [ ] 文件树状态保存/恢复正常；`clearPreview` 仍生效。

## Definition of Done

- `Home.spec.js` 增补 `onDirectorySelect` 用例：切换到 git 工作目录时 `selectedNode` 被直接设为目标节点（无 `null` 中间态）；切到非 git 工作目录时置 `null`。
- `cd frontend && npm run build` 通过；相关测试（Home/GitInfo）全绿。
- 手动验证 Acceptance Criteria。

## Technical Approach

**方案 A（重构 onDirectorySelect，消除 null 中间态）**

先查 `newDir`（`directories.value` 已存在，无需等 `nextTick`），按 `isGitRepo` 直接设置 `selectedNode` 目标值：

```javascript
const onDirectorySelect = async (dirId) => {
  // 1. 保存当前工作目录的树状态（保持原逻辑）
  if (selectedDirectoryId.value) {
    const currentDir = directories.value.find(d => d.id === selectedDirectoryId.value)
    if (currentDir) fileTreePanelRef.value?.saveCurrentState(currentDir.path)
  }

  // 2. 先查目标目录（directories 列表已就绪，不依赖 nextTick）
  const newDir = directories.value.find(d => d.id === dirId)

  // 3. 直接切到目标 selectedNode，避免 null 中间态导致 content-inner 卸载再挂载（双刷新）
  selectedDirectoryId.value = dirId
  latestCommit.value = null
  contentPanelRef.value?.clearPreview()
  selectedNode.value = newDir?.isGitRepo
    ? { id: newDir.id, path: newDir.path, name: newDir.name, type: 'directory', isGitRepo: true }
    : null

  // 4. 等文件树按新 selectedDirectoryId 重渲染后恢复树状态
  await nextTick()
  if (newDir) fileTreePanelRef.value?.restoreTreeState(newDir.path)
}
```

效果：
- gitA → gitB：`selectedNode` 由 A-git 直接切到 B-git，content-inner 不卸载，GitInfo `repoPath` 变化触发单次 `loadGitInfo`（与文件树切换完全一致）。
- 任意 → 非 git：`selectedNode = null`，content-inner 卸载（单次）。

## Decision (ADR-lite)

**Context**：`onDirectorySelect` 用"先 `null` 再设目标"的清空模式保证切到非 git 目录时残留被清掉，但 `null` 中间态触发 `v-if="selectedNode"` 卸载整个 content-inner，导致 git 仓库切换时双刷新。

**Decision**：方案 A —— 按 `newDir.isGitRepo` 直接计算并设置目标 `selectedNode`，消除 `null` 中间态。非 git 目录的清空由"目标值即 `null`"自然实现。

**Consequences**：
- 改动集中在 `onDirectorySelect` 单函数（~10 行）。
- `selectedNode` 不再有"先 null 后赋值"的两阶段，ContentPanel 不再中途卸载。
- 保留了树状态保存/恢复、`clearPreview`、`latestCommit` 清零等全部既有语义。
- 不引入 transition 动画（治标方案），从渲染机制上根治。

## Implementation Plan

- **步骤 1**：重构 `onDirectorySelect`（提前查 `newDir`、按 `isGitRepo` 设 `selectedNode` 目标值、`nextTick` 后仅 `restoreTreeState`）。
- **步骤 2**：`Home.spec.js` 增补 `onDirectorySelect` 用例（git 目录直设、非 git 目录置 null）。
- **步骤 3**：回归验证（手动 + 文件树切换不受影响）。

## Out of Scope

- `recordAccess`（工作目录切换不计访问记录，与文件树切换的既有差异，不在本任务范围）。
- ContentPanel 的 `v-if="selectedNode"` 渲染策略重构。
- git 面板 transition 过渡动画（治标，不采用）。

## Technical Notes

- **依赖**：建议先 commit `fix-git-repo-info-na`（同改 `Home.vue`，避免两任务改动叠加在同一文件）。
- ContentPanel 无 `watch(selectedNode)`，仅 props 透传，跳过 `null` 中间态安全。
- `directories.value` 为已加载的工作目录列表，`newDir` 查询无需等 `nextTick`；`nextTick` 仅为 `restoreTreeState` 等文件树按新 `selectedDirectoryId` 重渲染。

## Research References

- 内部代码根因定位，无需外部调研。
