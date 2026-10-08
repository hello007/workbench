# 内容搜索多超时目录批量重试

## Goal

承接 fc5c733 单目录重试 MVP，扩展为多超时目录批量重试。多目录同时超时（如全局搜索 5 个大目录 3 个超时）时，用户当前需逐个点 warning 行重试，体验差。提供批量重试入口，多目录并发重试，逐目录原位替换标志组。

## What I already know

* 单目录重试 MVP 已落地（fc5c733）：
  - `ContentSearchDirs` 后端方法（多目录并发，但 service 层共享 60s ctx）
  - 前端 `retryingRepoPaths` / `isGroupRetrying` 防重复机制
  - `retryTimedOutGroup(group)` 单目录调用 `ContentSearchDirs([repoPath], [repoName])` —— 每目录独立一次后端调用 = 独立 60s ctx
  - warning 行整行可点击 + is-retrying class loading 态
  - `lastContentQuery` 快照发起搜索时参数（防取实时 contentQuery 已改词）
* service 层 `ContentSearch` 多目录共享 ctx（ctx 一超时所有在途目录同超时）—— 单目录重试绕开此结构性限制
* AI 功能全局信号量 3 并发在后端（service/ai_function.go sem chan），前端只展示 running/queued/max
* BatchPull/HasRemotesBatch/Dashboard 后端 sem+wg 范式（并发 8）

## Architecture Decision

**路径 B：前端编排 N 个单目录重试并发 + 前端信号量限流**

不走"一次调 ContentSearchDirs(allDirs, allRepoNames)"——后者走 service 层共享 60s ctx，一目录超时全超时，不满足"每目录独立 60s ctx 互不阻塞"。路径 B 每目录独立一次后端调用 = 独立 60s ctx，且复用现有重试替换逻辑，只加批量编排层 + 前端并发限流。

## Requirements

* 多超时目录批量重试入口：结果区顶部 banner（timedOut 组 ≥2 时显示）
  - 静态态：「⚠ 检测到 N 个目录搜索超时 [全部重试]」
  - 进行中态：按钮 loading + 禁用，文案「重试中 (done/total)...」
  - 完成态（仍有再超时）：「仍有 N 个目录超时 [重试剩余 N 个]」
  - 完成态（全成功）：banner 消失（timedOut 组归 0）
  - timedOut 组 <2 时不显示（单目录走 warning 行单重试，避免冗余）
* 复用单目录重试核心逻辑，前端编排 N 个并发 + 信号量限流
  - 并发上限 3（对齐 AI 功能全局信号量模式，rg 进程数可控，IO 抢占温和）
* 每目录独立 60s ctx（路径 B：每目录独立一次 ContentSearchDirs 调用）
* 逐目录原位替换标志组（复用现有 splice 替换逻辑）
* 部分成功部分再超时：成功目录替换结果组，再超时目录保留 warning 行
* 批量启动预先把所有目标目录加入 retryingRepoPaths——排队中目录 warning 行也显"重试中..."，状态一致且防手动重复触发
* 搜索版本号守卫：executeContentSearch/openContentSearch/close 自增 searchVersion，批量编排闭包捕获起始版本号，每目录完成写回前比对不匹配则丢弃结果

## Acceptance Criteria

* [ ] timedOut 组 ≥2 时结果区顶部显示 banner，<2 时不显示
* [ ] 点击「全部重试」后多目录并发执行（3 并发上限），互不阻塞
* [ ] 批量进行中 banner 显示进度「重试中 (done/total)...」，按钮禁用
* [ ] 批量进行中所有目标目录 warning 行显示"重试中..."（预先加入 retryingRepoPaths）
* [ ] 成功目录原位替换为结果组，再超时目录保留 warning 行
* [ ] 批量完成后全成功 banner 消失；有剩余 banner 变「重试剩余 N 个」可再轮重试
* [ ] 批量进行中改词重搜/关闭面板，在途请求写回被版本号守卫丢弃，不污染新结果
* [ ] 复用 retryingRepoPaths/isGroupRetrying 防重复机制
* [ ] service 层 mergeTimeoutGroups/searchWithRipgrep/searchWithGo 零改动

## Definition of Done

* 测试新增/更新（composable 单测 + 组件用例）
* Lint / typecheck / CI green
* README.md / docs 视情况更新
* spec 沉淀（如有新约束）

## Technical Approach

### 前端 useCommandPalette.js

抽核心逻辑成内部函数 `retryOneDir(group, version)`，单目录入口 `retryTimedOutGroup` 与批量编排 `retryAllTimedOutGroups` 共用：

```js
const searchVersion = ref(0)

// 核心逻辑：调 ContentSearchDirs + splice 原位替换 + 版本号守卫
async function retryOneDir(group, version) {
  const { keyword, fileExt, subDir } = lastContentQuery.value
  if (!keyword) return
  try {
    const groups = await ContentSearchDirs(keyword, fileExt, subDir, [group.repoPath], [group.repoName])
    if (version !== searchVersion.value) return  // 版本不匹配丢弃
    const idx = contentGroups.value.findIndex(g => g.repoPath === group.repoPath)
    if (idx >= 0) {
      const next = contentGroups.value.slice()
      next.splice(idx, 1, ...(groups || []))
      contentGroups.value = next
      resetSelection()
    }
  } catch (e) {
    if (version !== searchVersion.value) return
    handleError('内容搜索失败: ', e)
  }
}

// 单目录入口（既有签名零变更，内部改调 retryOneDir 获版本守卫）
async function retryTimedOutGroup(group) {
  if (!group?.timedOut || retryingRepoPaths.value.includes(group.repoPath)) return
  retryingRepoPaths.value = [...retryingRepoPaths.value, group.repoPath]
  try {
    await retryOneDir(group, searchVersion.value)
  } finally {
    retryingRepoPaths.value = retryingRepoPaths.value.filter(p => p !== group.repoPath)
  }
}

// 批量入口：预先全加入 retryingRepoPaths + 信号量 3 并发 + 版本号守卫
async function retryAllTimedOutGroups() {
  const timedOutGroups = contentGroups.value.filter(g => g.timedOut)
  if (timedOutGroups.length < 2) return  // banner 仅 ≥2 显示，防御
  if (retryingRepoPaths.value.length > 0) return  // 批量/单目录进行中防重复

  const version = ++searchVersion.value
  retryingRepoPaths.value = [...retryingRepoPaths.value, ...timedOutGroups.map(g => g.repoPath)]
  batchRetrying.value = true
  batchRetryTotal.value = timedOutGroups.length
  batchRetryDone.value = 0

  const CONCURRENCY = 3
  let cursor = 0
  const workers = Array.from({ length: Math.min(CONCURRENCY, timedOutGroups.length) }, async () => {
    while (cursor < timedOutGroups.length) {
      const group = timedOutGroups[cursor++]
      try {
        await retryOneDir(group, version)
      } finally {
        if (version === searchVersion.value) {
          retryingRepoPaths.value = retryingRepoPaths.value.filter(p => p !== group.repoPath)
          batchRetryDone.value++
        }
      }
    }
  })
  await Promise.all(workers)
  if (version === searchVersion.value) {
    batchRetrying.value = false
  }
}
```

版本号自增点（既有 executeContentSearch/openContentSearch/close 加 `searchVersion.value++`，与 retryingRepoPaths 清空同步）。

### 前端 CommandPalette.vue

* 结果区顶部 banner（contentGroups 渲染区上方）：
  - 显示条件：`timedOutCount >= 2 || batchRetrying`
  - 文案 computed：`batchRetrying ? '重试中 (${batchRetryDone}/${batchRetryTotal})...' : '⚠ 检测到 ${timedOutCount} 个目录搜索超时'`
  - 按钮文案 computed：`batchRetrying ? '' : (timedOutCount >= 2 ? '全部重试' : '重试剩余 ' + timedOutCount + ' 个')`
  - 按钮禁用：`batchRetrying`
* 既有 warning 行逻辑零改动（is-retrying 天然覆盖批量态）

### 后端

零改动。复用 `ContentSearchDirs` 既有实现。

## Decision (ADR-lite)

**Context**: 多超时目录批量重试，需并发且每目录独立 ctx 互不阻塞。service 层 ContentSearch 共享 ctx 无法满足。

**Decision**: 路径 B——前端编排 N 个单目录重试并发 + 前端信号量限流（3 并发）。每目录独立一次 ContentSearchDirs 调用 = 独立 60s ctx。抽 retryOneDir 核心函数共用，单目录重试顺带获版本号守卫。

**Consequences**:
* 前端编排多请求，需搜索版本号守卫防改词重搜/关闭污染
* 批量进行中 retryingRepoPaths 预占所有目标目录，单目录手动重试被防重复挡住（设计如此）
* 单目录重试行为增强（版本守卫），非破坏性
* service 层零改动，约束守住

## Out of Scope (explicit)

* service 层 mergeTimeoutGroups/searchWithRipgrep/searchWithGo 改动
* 超时阈值调整
* 重试策略改写（指数退避等）
* 批量重试取消机制（单目录重试也无取消，对齐既有）

## Technical Notes

* fc5c733 单目录重试实现：useCommandPalette.js L140-166 / CommandPalette.vue L149-165
* AI 功能信号量：service/ai_function.go sem chan（后端）；前端展示 AiFunctionPanel.vue L602
* service 层 ContentSearch 共享 ctx：service/content_search.go L99（ctx 传入多目录 goroutine）
* 约束：不动 service 层，复用现有 retryingRepoPaths/isGroupRetrying 防重复机制
* 单测稳定构造：ContentSearchDirs mock 按目录返回不同结果/超时，验证并发编排 + 版本守卫
