# 提交历史服务端搜索与作者日期文件过滤

## Goal

补齐提交历史模块的服务端搜索与过滤能力，覆盖超 500 条提交（现有客户端过滤兜底不足）。延续 Git 主线，路线图 `docs/路线图.md:33-34` 移出项。

## What I already know

### 现有后端基础（app_git.go:133 GetCommitHistory）
- 签名：`func (a *App) GetCommitHistory(path string, limit int, offset int) ([]model.Commit, error)`
- 引擎：go-git v5.18.0 `repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})`，仅设 Order
- 分页：手动跳过 offset 个 + 收集 limit 个；无总数返回，前端靠 `newCommits.length === pageSize` 判 hasMore
- `getCommitFiles(repo, commit)` 已按 tree patch 计算每提交变更文件列表
- 返回 `[]model.Commit`（SHA/ShortSHA/Message/Author/Email/Timestamp/DateTime/Files）

### 现有前端基础（frontend/src/components/CommitHistory.vue）
- 客户端 `filteredCommits` computed：message/author/sha includes 关键词（小写），仅过滤已加载（≤500）
- `MAX_COMMITS = 500` 硬上限，`PAGE_SIZE = 20`，loadMore 追加
- 搜索框 `searchKeyword` 已存在，@input 触发（当前仅客户端）
- 区间 diff 勾选（selectedShas FIFO 限 2）+ 单文件 diff 弹窗已实现

### go-git v5.18.0 LogOptions 实际字段（源码核实 options.go:483）
- `From plumbing.Hash` / `Order LogOrder` / `FileName *string` / `PathFilter func(string) bool` / `All bool` / `Since *time.Time` / `Until *time.Time`
- **无 Author / Committer 字段**（旧版曾有，v5.18.0 已移除，仅留 `ErrMissingAuthor` 残留变量）
- 结论：Since/Until/FileName/PathFilter 可原生下推迭代器；**Author 与 message 关键词必须手动迭代匹配**

### 跨层契约（docs/spec/cross-layer-contracts.md）
- App 方法签名变更 → 同步 `frontend/wailsjs/go/main/App.js` + `App.d.ts`（整目录 gitignore，`wails generate module` 重生成）
- model struct 字段变更 → 同步 `frontend/wailsjs/go/models.ts`
- 具名 string 类型（`type X string`）作方法参数 → 手动补 `models.ts` `export type X = string` 别名
- `npm run build` 必跑（vitest esbuild 不报 MISSING_EXPORT，易漏）

## Requirements

- 服务端按关键词搜索提交消息（覆盖 >500 条，突破客户端 ≤500 上限）
- 按作者过滤（Name + Email 匹配，大小写不敏感，子串）
- 按日期区间过滤（起/止，含端点）
- 按文件路径过滤（子串匹配，支持目录级，如输 `src` 匹配所有 `src/` 下变更）
- 过滤条件组合语义为 AND
- 过滤结果分页（翻页仅遍历过滤后结果集）
- 跨层绑定同步（App.js / App.d.ts / models.ts）

## Acceptance Criteria

- [ ] 超过 500 条提交的仓库，按关键词过滤能返回匹配结果（客户端兜底无法覆盖的场景）
- [ ] 按作者过滤返回该作者全部提交（跨分页）
- [ ] 按日期区间过滤，起止端点当天提交包含在内
- [ ] 按文件路径过滤，目录级子串匹配命中该目录下所有变更提交
- [ ] 多条件组合 AND 语义正确
- [ ] 过滤后翻页仅遍历过滤结果集，不混入未匹配提交
- [ ] 无过滤条件时行为与原 GetCommitHistory 完全一致（向后兼容）
- [ ] 跨层绑定三处同步，`npm run build` 绿
- [ ] 后端单测覆盖各过滤维度 + 组合 + 边界（空仓/单提交/root commit）
- [ ] 前端单测覆盖服务端过滤触发与结果渲染

## Definition of Done

- 后端测试覆盖率达标（service ≥76%）
- 前端测试覆盖率 ≥70%
- `npm run build` + `go test ./...` + `cd frontend && npm test` 三绿
- README / 路线图勾选更新（路线图:33-34 两项）
- 跨层契约无回归

## Technical Approach

### 方案 A：扩签名 + go-git + 过滤内存分页（已选定）

**后端签名扩展**：
```go
func (a *App) GetCommitHistory(path string, limit, offset int, filter model.CommitFilter) ([]model.Commit, error)
```

**新增 model.CommitFilter struct**（`model/commit.go`）：
```go
type CommitFilter struct {
    Author   string `json:"author,omitempty"`   // 作者 Name+Email 子串匹配
    Keyword  string `json:"keyword,omitempty"`  // 提交消息子串匹配
    Since    string `json:"since,omitempty"`    // 起始日期 YYYY-MM-DD（含当天 00:00:00）
    Until    string `json:"until,omitempty"`    // 截止日期 YYYY-MM-DD（含当天 23:59:59）
    FilePath string `json:"filePath,omitempty"` // 文件路径子串匹配（目录级）
}
```

**引擎策略**（go-git LogOptions 原生 + 手动迭代分层）：
- `Since` / `Until`：解析为 `*time.Time`，下推 `LogOptions.Since/Until`（原生，迭代器层过滤）
- `FilePath`：构造 `PathFilter func(string) bool`（`strings.Contains(path, filePath)`），下推 LogOptions（原生）
- `Author`：go-git v5.18.0 无 Author 字段，迭代内手动匹配 `strings.Contains(strings.ToLower(name+email), keyword)`
- `Keyword`（消息）：迭代内手动 `strings.Contains(strings.ToLower(message), keyword)`

**过滤 + 分页一次遍历**：
```
迭代 commit:
  if 不匹配 Author/Keyword: continue      // Since/Until/FilePath 已由迭代器过滤
  if 过滤后已收集数 < offset: 计数++; continue
  收集进结果; if len == limit: break
```
- offset 语义从"绝对偏移"变"过滤后偏移"；前端 loadMore 传 `commits.value.length` 仍正确（已加载=过滤后已收集数）

**前端改造**（CommitHistory.vue）：
- 头部加过滤条件行：作者输入、日期区间（el-date-picker daterange）、文件路径输入
- 搜索框 `searchKeyword` 改为服务端过滤的关键词入口
- 过滤条件变更 → 重置分页（reset=true）调 `GetCommitHistory(path, pageSize, 0, filter)`
- loadMore 传当前 filter
- 移除/保留客户端 `filteredCommits`：服务端过滤已覆盖，客户端 computed 可移除（避免双重过滤混淆）；MAX_COMMITS 上限在服务端过滤场景下可放宽或移除

### 跨层同步清单
- `model/commit.go` 加 `CommitFilter` struct
- `app_git.go` GetCommitHistory 签名加 filter 参数
- `wails generate module` 重生成 `App.js` / `App.d.ts` / `models.ts`
- 核对 `models.ts` 是否生成 `CommitFilter` class（struct → class 正常生成，无具名 string 类型不触发别名规则）
- 前端调用点 `GetCommitHistory` 传 filter 对象

## Decision (ADR-lite)

**Context**：需补齐服务端搜索/过滤，覆盖 >500 条提交。go-git v5.18.0 LogOptions 无 Author 字段（源码核实，推翻原假设），Author 与消息关键词须手动迭代。

**Decision**：方案 A——扩 GetCommitHistory 签名加 `model.CommitFilter` 参数，go-git LogOptions 原生下推 Since/Until/PathFilter，Author/Keyword 迭代内手动匹配，过滤与分页一次遍历。

**Consequences**：
- 优点：同引擎（不割裂 go-git 主线）、签名最小扩展（仅加一个 struct 参数）、复用现有 getCommitFiles、跨层仅 struct 同步
- 代价：offset 语义变过滤后偏移（前端逻辑恰好兼容）；Author/Keyword 手动迭代遍历全量匹配提交（大仓 + 无日期区间时性能依赖迭代器已下推的过滤剪枝）
- 风险：无过滤时须保持原行为（filter 全空走原路径）；大仓无区间过滤时手动迭代 Author/Keyword 成本高，MVP 可接受，后续可加缓存或换 CLI

## Out of Scope

- 正则表达式搜索（MVP 用子串 Contains）
- 多文件路径过滤（MVP 单路径，PathFilter 可扩展）
- 保存/复用过滤预设
- 提交内容（diff 文本）搜索
- 提交总数返回（保持 hasMore 页满判定）

## Technical Notes

- 引擎现状：app_git.go GetCommitHistory 用 go-git；service/git.go 中 Commit/Push/GetDiff/GetLocalChanges 等用 git CLI——两种引擎项目内并存，本任务沿用 go-git
- go-git v5.18.0 无 Author 字段，Author/grep 必须手动迭代（关键约束）
- 现有分页无总数，前端 hasMore 靠页满判定
- 日期解析：前端传 YYYY-MM-DD 串，后端 `time.ParseInLocation("2006-01-02", s, time.Local)`；Since 补 00:00:00，Until 补 23:59:59
