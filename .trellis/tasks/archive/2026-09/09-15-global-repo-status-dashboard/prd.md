# 全局仓库状态看板

## Goal

为个人开发者提供跨仓库状态总览:一眼看清多个仓库哪些 dirty(工作区未提交)、哪些 ahead(本地未推送)、哪些 behind(远程有新提交)、哪些无上游/未跟踪。解决多仓维护时「忘记 push、忘记 pull、工作区没收尾」的真实痛点,当前 WorkBench 无任何 ahead/behind 计算,跨仓状态全靠用户逐仓手动查看。

## What I already know

**后端现状(已勘探):**
- `GitService.ScanGitRepos(rootPath)` → `[]string` 仅路径,按工作目录根递归扫描,带 mtime 差量 + TTL 5min 缓存(`scanGitReposCached`),深层新增仓库靠 TTL+手动刷新兜底
- `GitService.HasRemotesBatch(repoPaths)` → `map[string]bool`,并发 8,go-git `PlainOpen`(非 fork git 子进程)
- `GitService.GetInfo(dirPath)` → `*GitRepoInfo{Path,Branch,Remote,RemoteURL,IsRepo}`,**无 ahead/behind/dirty 字段**
- `GitService.GetLocalChanges(dirPath)` → `[]FileChange`,单仓,fork `git status --porcelain -z --untracked-files=all`
- `GitService.HasUpstream(repoPath)` → `(bool,error)`,单仓
- `GitService.BatchPull(repos,concurrency,ctx)` → `[]PullResult`,发 `pull-progress`/`pull-complete` 事件,仓级锁串行化
- 变更类操作按仓库路径 `sync.Mutex` TryLock 串行(`tryLockRepo`),跨仓并行,只读不受影响

**Model 现状:**
- `GitRepoInfo`(model/models.go:90):Path/Branch/Remote/RemoteURL/Commits/IsRepo,无状态字段
- `RepoFilterItem`(model/repo_meta.go:29):Name/Path/Summary/Tags/ReadmeSummary/Missing/HasRemote/IsGitRepo,无状态字段
- `RepoMeta`(model/repo_meta.go:7):用户元数据(简述/标签/README 摘要/失效标记),按规范化路径持久化 `data/repo_meta.json`
- model 全仓 grep `Ahead|Behind|Dirty` 仅 submodule porcelain=2 的 `Dirty` 一处,**主仓状态字段零实现**

**前端现状:**
- ActivityBar 一级入口:directory / toolbox / ai / stats(`uiStore.activePanel`),ai/stats 占满主区三栏隐藏
- `RepoFilterDialog` = 单工作目录维度多仓列表(虚拟滚动 + 标签筛选 + 跳转定位),最接近的多仓 UI
- `StatsView` = 单仓统计(提交趋势/热力图/贡献者),一级入口范式参考
- Pinia store 拆 5 域(目录/UI/设置/AI 等)

**约束(CLAUDE.md 关键规则):**
- 新增 service 须在 `app_services.go` 的 `AppServices` struct 加字段 + `NewAppServices` 加构造行(2 处)
- `model/` 新增导出 struct 字段须手动同步 `frontend/wailsjs/` 绑定(App.js/App.d.ts/models.ts 三处)
- 后端日志用 `log/slog`,service 包内调 `Logger()`
- 写产物 JSON 前须关闭 workbench.exe(否则被整体覆盖丢数据)
- 跨仓批量并发参考 `HasRemotesBatch`/`BatchPull` 的 sem+wg 模式

## Assumptions (temporary)

- MVP 状态字段:dirty / ahead / behind / no-upstream + branch + 仓库名(已定 Q2=A)
- ahead/behind 用本地 `git rev-list --left-right --count` 计算,纯本地引用不自动 fetch(已定 Q3=A)
- 看板作为 ActivityBar 新一级入口,与 stats/ai 平级占满主区(已定 Q5=A)
- pin 列表独立持久化 `data/dashboard_pinned.json`,双入口添加:选仓器弹窗(批量)+ 右键「加入状态看板」(单仓)(已定 Q7=C)
- 刷新策略:开页即算 + 手动刷新按钮,无后台巡检/事件驱动(已定 Q4=A)
- 看板动作:纯只读 + 点击跳转该仓(切目录+定位文件树),不在看板内 commit/push/pull(已定 Q6=A)

## Open Questions

- Q3 ahead/behind 是否需 fetch(网络策略)
- Q4 刷新策略(手动/自动/开页即刷)
- Q5 UI 入口与布局
- Q6 看板可触发的动作(跳转/批量 pull/推送)
- Q7 pin 来源与首次使用流程

## Requirements (evolving)

- 用户手动 pin 关注的仓库(非全局扫描所有仓库),开源仓等不关注的不进看板
- 看板每行展示:仓库名 + 当前分支 + dirty/ahead/behind 数值 + no-upstream 标记
- 个人开发者多仓维护场景,低摩擦一眼看清核心仓库状态

## Decision (ADR-lite)

### Q1 聚合范围 = 选项 C(用户手动 pin 关注仓库)

**Context**: 用户维护大量开源仓库,理论不需关注,全局扫描所有工作目录噪声过大。真痛点是少数核心仓库(工作/个人项目)的状态跟踪。
**Decision**: 看板只展示用户手动 pin 的仓库,不自动扫全工作目录。pin 列表持久化(复用 RepoMeta 模式落 data/)。
**Consequences**:
- 看板轻量、信号噪声比高,只看核心仓库
- 首次使用需选仓库 pin 一次(摩擦点,UX 需低摩擦,待 Q7)
- 不与 RepoFilterDialog(单工作目录全仓浏览)重叠 — RepoFilter 浏览全仓,看板盯核心仓状态
- pin 列表增删需持久化方案(待 Q7 确认来源)

### Q2 MVP 状态字段 = 选项 A(核心 4 态)

**Context**: 看板信息量需命中个人开发者三大痛点(未提交/未推送/未拉取)且不过载。
**Decision**: 每行展示仓库名 + 当前分支 + dirty/ahead/behind 数值 + no-upstream 标记。last commit 时间(僵尸仓)不进 MVP,留后续。
**Consequences**:
- ahead/behind 依赖上游存在,no-upstream 标记覆盖无上游场景
- behind 准确性依赖 fetch,时效问题由 Q3 策略兜底
- 字段密度适中,一眼可读

### Q3 ahead/behind 网络策略 = 选项 A(纯本地引用,不 fetch)

**Context**: behind 准确性理论上需 fetch,但桌面工具开页联网违背低打扰,且个人开发者核心痛点(ahead/dirty)纯本地即可准。
**Decision**: ahead/behind 基于 `git rev-list --left-right --count @{u}...HEAD` 算本地已有远程引用(上次 fetch/clone 快照),看板不主动 fetch。behind 标注时效,用户主动点刷新可 fetch(作为后续增强,MVP 不强制实现 fetch 入口)。
**Consequences**:
- behind 是「相对上次 fetch」的近似值,可能落后真实远程 — 前端时效标注兜底
- 零网络、开页快、无超时/限流风险
- 「忘了 pull」场景用户会主动刷新,按需 fetch 留后续 C 增强空间

### Q4 刷新策略 = 选项 A(开页即算 + 手动刷新)

**Context**: 状态时效性强但 pin 仓数量少,需平衡实时性与实现复杂度。
**Decision**: 进看板自动算一次全部 pin 仓状态,手动刷新按钮触发重算。无后台定时巡检、无事件驱动增量。
**Consequences**:
- 实现简单,与 StatsView/CommitHistory「进页加载 + 手动刷新」范式统一
- 看板停留期间外部操作不自动同步,需手动点刷新 — pin 仓少可接受
- 事件驱动增量(B)与后台巡检(C)留后续增强,非 MVP 必须

### Q5 UI 入口与布局 = 选项 A(ActivityBar 新一级入口)

**Context**: 看板是高频监控型功能,需高可见性入口,且 WorkBench 已有 `activePanel` 占满主区范式(StatsView/AiFunctionPanel)。
**Decision**: ActivityBar 加第 5 个图标(DataBoard/Grid,区别于 stats 的 TrendCharts),`activePanel='dashboard'` 占满主区。布局:顶部工具条(刷新 + 添加 pin 仓 + 时效说明) + 主体表格(每行一仓:名/分支/dirty/ahead/behind/上游/操作) + 空状态引导。复用 Home.vue `v-show` 分支 + 新 DashboardView 组件。
**Consequences**:
- 一级入口可见性匹配监控定位
- ActivityBar 达 5 图标(可接受),复用现有占满主区范式改动小
- 不与 RepoFilterDialog(弹窗单工作目录)冲突

### Q6 看板动作 = 选项 A(纯只读 + 跳转)

**Context**: 看板职责边界 — 纯监控总览 vs 可操作控制台。MVP 需避免与现有单仓面板功能重叠。
**Decision**: 每行点击跳转该仓(切工作目录 + 展开文件树定位,复用 RepoFilterDialog 跳转范式),不在看板内直接 commit/push/pull。操作走现有 LocalChanges/CommitHistory 等成熟单仓面板。
**Consequences**:
- 看板单一监控职责,实现轻
- 行内快捷动作(B:ahead 直推/behind 直拉)与批量操作(C:批量 push)留后续增强
- 跳转闭环够用 — 看到状态点进去操作,用户流程熟悉

### Q7 pin 来源与首次使用 = 选项 C(选仓器 + 右键双入口)

**Context**: pin 仓需低摩擦建立,首次批量选仓与日常单仓随手 pin 两种场景并存。
**Decision**: 双入口添加 pin 仓 — ①看板「添加」按钮弹选仓器(复用 RepoFilterDialog 扫描+虚拟列表,跨工作目录批量勾选);②工作目录树/文件树 git 仓库节点右键「加入状态看板」(单仓随手 pin)。pin 列表独立持久化 `data/dashboard_pinned.json`(纯路径数组,`filepath.Abs` 规范化,与 RepoMeta 职责分离)。移除 pin:看板每行「取消关注」按钮删路径。
**Consequences**:
- 双入口覆盖首次批量建立 + 日常单仓添加两场景
- 选仓器复用现成 RepoFilterDialog 能力,右键菜单复用现有右键框架,新增 UI 成本可控
- MVP 分 PR:选仓器入口先做(核心),右键入口次做(增量)
- `dashboard_pinned.json` 结构:`{ paths: ["abs/path1",...], updatedAt: "..." }`

## Acceptance Criteria

- [ ] 看板展示 pin 仓状态,dirty/ahead/behind 数值与命令行 `git rev-list`/`git status` 一致
- [ ] 双入口添加 pin(选仓器批量 + 右键单仓)均持久化,重启后保留
- [ ] 点击仓库行跳转:切工作目录 + 展开文件树定位
- [ ] 无上游仓库标 no-upstream,ahead/behind 不报错降级显示(0/0 或「无上游」)
- [ ] 失效仓库(pin 后路径删除)标 Missing 灰显,不阻塞看板
- [ ] 开页即算不阻塞 UI(pin 仓少,并发算)
- [ ] 单测覆盖状态计算纯函数 + 集成测试(`//go:build integration`)覆盖真实 git fixture(ahead/behind/dirty/no-upstream/detached HEAD 场景)
- [ ] 前端单测覆盖空状态/跳转/pin 增删交互
- [ ] `frontend/wailsjs/` 绑定三处同步,覆盖率达标

## Definition of Done

- 后端单测 + 集成测试(`//go:build integration`)覆盖
- 前端单测覆盖关键交互
- `frontend/wailsjs/` 绑定三处同步
- 覆盖率达标(model/server ≥80% / service ≥76% / util ≥40% / 前端 ≥70%)
- docs/功能说明.md + README.md 同步

## Out of Scope (explicit)

- 行内快捷动作(ahead 直推 / behind 直拉 / dirty 直跳变动)— 留后续增强
- 批量 push/pull 工具条(批量 pull 已有 BatchPull,批量 push 留后续)
- 后台定时巡检 / 事件驱动增量刷新 — 留后续
- 自动 fetch 准实时 behind(behind 时效标注兜底,按需 fetch 留后续)
- last commit 时间(僵尸仓识别)— 留后续
- 团队协作 / 云集成 — 路线图暂缓项

## Implementation Plan (分 PR)

- **PR1**: 后端状态计算 + pin 持久化 + 单测/集成测试(核心链路)
  - `DashboardService`(或 GitService 扩展):批量并发算多仓状态
  - `model.RepoStatus` + `model.PinnedRepos` 结构
  - `data/dashboard_pinned.json` 持久化(RepoMetaService 模式)
  - App 方法 + wailsjs 绑定三处
- **PR2**: 前端 DashboardView + ActivityBar 入口 + 选仓器添加 pin + 跳转 + 单测
  - ActivityBar 第 5 图标 + Home.vue `v-show` 分支
  - DashboardView.vue(工具条 + 表格 + 空状态)
  - 选仓器弹窗复用 RepoFilterDialog 范式
  - 跳转复用「切目录 + 展开文件树定位」
- **PR3**: 右键「加入状态看板」单仓入口 + 文档(功能说明/README)+ 边界完善

## Technical Notes

- ahead/behind 计算候选命令:`git rev-list --left-right --count @{u}...HEAD`(左=behind 远程新,右=ahead 本地新),需仓库已配上游且本地有 fetch 过的远程引用
- dirty 检测候选:`git status --porcelain` 非空即 dirty(比 GetLocalChanges 轻,不需解析文件列表)
- 批量并发范式:`HasRemotesBatch`(go-git 纯内存)/`BatchPull`(fork git),状态计算偏只读可走 go-git 或轻量 git 子进程
- 缓存:状态时效性强(dirty/ahead 随操作变化),不宜长 TTL,参考提交历史缓存的 SHA 增量思路不适用
