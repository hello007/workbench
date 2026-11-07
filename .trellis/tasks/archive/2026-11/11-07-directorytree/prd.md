# 修复聚合视图 DirectoryTree 增删/重命名工作目录隐式重置选中

## Goal

聚合视图（`__aggregated__`）下经 DirectoryTree 增删/重命名/设默认工作目录时，`loadDirectories` 无条件重置 `selectedDirectoryId`（temp-memory 欠账条目），造成选中态与聚合模式错配（高亮跳变、clipboard 被清、快照污染、切回目录树显示错目录）；普通树下同一跳变还会触发 treeKey 变化 → el-tree 整体重建。修复收口在 `loadDirectories` 单点：保留仍存在的当前选中，不存在时才回落。

## Requirements

1. `loadDirectories` 单点收口（`store/directory.js`，禁各调用点散 patch）：
   1. 当前 `selectedDirectoryId` 仍存在于新列表 → 保留不动（增删/重命名/设默认/拖拽排序后的刷新均不打断选中）
   2. 不存在时回落：默认目录 → 首个 → 空串
2. 冷启动语义不变：初始选中 `''` 必然走回落分支，`directory.spec.js` 既有三断言不回归
3. 删除当前选中目录后回落 default/首个；重命名（id 不变）选中保持；「设为默认」不再抢当前选中；目录删光后清空脏选中（现状残留脏值）
4. E2E 补一条聚合语境用例：进入聚合 → 增删工作目录 → 断言聚合未退出 + 高亮未跳变

## Acceptance Criteria

- [x] directory.spec.js 新增用例锁定「保留仍存在的选中」「选中不存在时回落（default/首个/空列表清空）」——新增 5 条，其中 3 条（保留选中/列表重排保留/删光清空）在现状代码下失败复现
- [x] 既有 directory.spec.js 三断言不回归；`npm test` 全绿（119 文件 2676 条）
- [x] E2E 聚合语境用例通过（file-tree.spec.js 聚合 describe 子组「增删工作目录后聚合保持」）；回滚修复验证用例真实捕获回归（防假绿）；file-tree.spec.js 全套 24 条无回归
- [x] `npm run build` 通过
- [x] 聚合视图验证：增删工作目录后聚合不退出、高亮不跳变（由 E2E 用例自动化覆盖；重命名链路 id 不变与「仍存在」同构，单测锁定同一 loadDirectories 单点）
- [x] 处理后删除 temp-memory 对应欠账条目（TEMP-MEM-activeContext.md 已出口）

## Definition of Done

- 单测 + E2E 新增且全绿
- npm run build 通过
- temp-memory 欠账条目出口
- 确认是否需更新 README.md（本修复为行为修正，无接口/文档面变化，预期不需要）

## Technical Approach

`store/directory.js` loadDirectories 改造（唯一改动点）：

```js
async function loadDirectories() {
  try {
    const dirs = await GetDirectories()
    directories.value = dirs || []

    // 保留仍存在的当前选中：增删/重命名/设默认/排序后的刷新不打断用户选中态
    // （隐式跳变在聚合视图下造成选中与聚合模式错配，普通树下触发 treeKey 变化重建树）
    if ((dirs || []).some(d => d.id === selectedDirectoryId.value)) return

    // 当前选中不存在（含冷启动空串、目录被删光）才回落：默认目录 → 首个 → 清空
    const defaultDir = (dirs || []).find(d => d.isDefault)
    if (defaultDir) {
      selectedDirectoryId.value = defaultDir.id
    } else if (dirs.length > 0) {
      selectedDirectoryId.value = dirs[0].id
    } else {
      selectedDirectoryId.value = ''
    }
  } catch (error) {
    debug.log('加载目录失败:', error)
  }
}
```

## Decision (ADR-lite)

**Context**：用户任务描述「346 设为工作目录链路…修复后应保持聚合不退出」与代码既有设计存在两种解读；E2E 用例范围需定夺；空列表脏选中处理需确认。两次询问用户未即时响应。

**Decision**（按代码证据最充分的解读推进，最终确认环节可否决）：

1. **「设为工作目录」维持既有设计**：聚合树右键「设为工作目录」= 添加/复用后经 onDirectorySelect 显式选中并退出聚合（onSetAsWorkDir 注释 328-333、AGENTS.md「选工作目录=退出聚合」显式入口收口规则、欠账原文修复建议三处一致）。修复只消除 Home.vue:346 loadDirectories 的隐式跳变中间态（现状先跳 default 再被 onDirectorySelect 改写，期间 clipboard 被清、快照被污染）。
2. **补一条 E2E 聚合语境用例**：本 bug 本质是跨组件编排错配（@change → store → DirectoryTree 高亮 / FileTreePanel 聚合态），store 单测无法覆盖完整链路；file-tree.spec.js 已有聚合 describe 与 `toggle-aggregated-view` 基建，成本可控。
3. **空列表清空脏选中**：目录删光后 selectedDirectoryId 残留指向已删除目录显然错误，纳入回落链末档；与既有断言「返回空列表时不选中任何目录」语义一致。

**Consequences**：「设为默认」不再切换当前选中（现状会跳到新默认）——属修复目标行为；「设为工作目录」退出聚合语义维持，无特例新增。

## Out of Scope

- 不改 onDirectorySelect 聚合退出判定链（显式入口语义维持）
- 不动 refreshGitFlags（已有「不动 selectedDirectoryId」语义）
- 不新增聚合退出的隐式 watch
- 不处理 DirectoryTree 拖拽排序失败分支的 emit('change')（修复后同受益于保留选中，无需单独处理）

## Technical Notes

### 现状行为链（修复前，已核对代码 2026-11-07）

| 触发链路 | loadDirectories 调用点 | 聚合模式影响 | 普通模式影响 |
|---|---|---|---|
| DirectoryTree 添加(532)/重命名(582)/设默认(599)/删除(628) → `emit('change')` → Home.vue:17 `@change="directoryStore.loadDirectories"` | store 单点（唯一收口点） | 选中隐式跳变、**不退出聚合**（aggregatedMode 无隐式 watch）：高亮跳变、clipboard 被清（Home.vue:749）、terminalDir 可能被改（Home.vue:627）、useSessionState 快照污染 | selectedDirectoryId 跳变 → treeKey 变化（FileTreePanel.vue:599）→ el-tree 整体重建 |
| FileTreePanel addAsWorkDir（普通树右键 → Home.vue:318） | Home.vue:318 | N/A | 同上 |
| FileTreePanel setAsWorkDir（聚合树右键 → Home.vue:340-360） | Home.vue:346 | loadDirectories 先隐式跳变（中间态），随后 onDirectorySelect 显式选中+退出聚合（设计预期） | onDirectorySelect 正常切换 |
| 启动加载（Home.vue:761） | Home.vue:761 | 冷启动语义（三个既有单测锁定） | 同 |

### 关键代码事实

- `store/directory.js:29-44`：default 优先 → 首个 → 空列表分支不动作（残留脏选中）
- 聚合翻转仅两显式入口：`toggleAggregatedMode`（FileTreePanel.vue:1271）、`onDirectorySelect` → `exitAggregatedMode`（Home.vue:251）；全仓无隐式 watch
- temp-memory 欠账原文（TEMP-MEM-activeContext.md:21）：「隐式重置 `selectedDirectoryId` 而**不触发退出聚合**（选中态与聚合模式错配）」——错配问题，非「聚合意外退出」
- 既有三断言均在初始 `''` 语境，修复后仍走回落分支，兼容
- E2E 聚合基建：file-tree.spec.js:502 起 describe + `[data-action="toggle-aggregated-view"]`

### 实施计划

1. 单测先行（复现）：directory.spec.js 新增「保留仍存在的选中」「选中不存在回落 default」「选中不存在回落首个」「列表清空清空选中」「删光前有选中后清空」等用例，确认现状代码下失败
2. 修复 loadDirectories（单点）
3. 单测全绿 + npm test 全绿 + npm run build
4. E2E 聚合语境用例（file-tree.spec.js 聚合 describe 内）
5. temp-memory 欠账条目出口 + journal 记录
