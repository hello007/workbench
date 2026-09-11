# 提交历史增强 - commit diff 与 range diff

## Goal

补齐提交历史模块 diff 能力：单 commit 文件改动 diff、commit 间 range diff。延续 Git 主线（merge/rebase/cherry-pick 之后），使提交历史从「只读列表」升级为「可对比」。

## Decision (ADR-lite)

**Context**：提交历史四子项（单 commit 文件 diff / range diff / 服务端搜索 / 作者日期文件过滤）工作量大，需切分。
**Decision**：本次只做 diff 两项（单 commit + range）。搜索/过滤移出范围，独立任务后续做。
**Consequences**：快速交付最高价值 diff 能力；搜索/过滤延后，客户端过滤（≤500 条）临时兜底。

### 交互与技术决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 单 commit 文件 diff 展示 | 弹窗（复用 FileDiffDialog） | 与工作区 diff 体验一致，空间大适合长 diff |
| range diff 选 commit | 列表勾选两个 +「对比」按钮 | 可视化，不易输错 SHA |
| diff 文本生成 | git CLI | 沿用 GetDiff 风格，unified diff 直接喂前端 parseDiff，性能好 |

## What I already know

- `model.Commit`（model/commit.go:4）已含 SHA/ShortSHA/Message/Author/Email/Timestamp/DateTime/Files 字段，数据层完备
- `GetCommitHistory(path, limit, offset)`（app_git.go:133）go-git Log 分页已实现，`getCommitFiles`（app_git.go:199）已填充变更文件列表
- 前端 `CommitHistory.vue` 已有：展开详情面板（完整 SHA/邮箱/时间/文件 tag）、客户端关键词过滤（message/author/sha）、加载更多分页（PAGE_SIZE=20, MAX=500）
- 现有 diff 仅工作区单文件：`GetDiff(repoPath, file)`（service/git.go:580，git CLI `git diff HEAD -- file`）、`GetFileDiff(path, file)`（app_git.go:283）—— 均无 commit 维度
- 前端 `FileDiffDialog.vue` 已有双栏 diff 渲染 + `parseDiff` unified diff 解析 + 二进制兜底提示，可复用
- 服务层 `gitCmd.Execute` / `ExecuteWithCodes` 封装已就绪（util/git.go）
- 路线图文档（docs/路线图.md:29-34）标记详情/搜索「未实现」系过时，详情实际已做

## Requirements

- 单 commit 展开详情中，点击变更文件 tag 弹出该 commit 相对其 parent 的双栏 diff
- 支持在提交列表勾选两个 commit，生成 range diff（双栏对照，按文件分组）
- root commit（无 parent）显示为全增
- 二进制文件无文本 diff 时明确提示「二进制文件无法展示」
- 前端 FileDiffDialog 泛化为可接收任意 diff 文本来源（工作区 / 单 commit / range）

## Acceptance Criteria

- [ ] 展开任一 commit，点击变更文件 tag 弹出该 commit 此文件的双栏 diff
- [ ] 提交列表可勾选两个 commit，点「对比」按钮生成 range diff，按文件分组双栏展示
- [ ] root commit 文件 diff 显示为全增内容
- [ ] 二进制文件显示「二进制文件无法展示」提示
- [ ] 后端单测覆盖：单 commit 文件 diff、range diff、root commit、二进制兜底
- [ ] 前端组件测试覆盖：文件 diff 弹窗交互、range 勾选对比交互
- [ ] wailsjs 绑定三处同步（App.js/App.d.ts/models.ts）
- [ ] 同步更新 docs/路线图.md（勾选 commit 间 diff）+ README.md（如涉及）

## Definition of Done

- 后端测试达分层门禁（model/server ≥80%、service ≥76%、util ≥40%）
- 前端测试 ≥70%（exclude wailsjs）
- wailsjs 绑定三处同步（App.js/App.d.ts/models.ts）
- 文档更新（路线图勾选 + README 如需）

## Out of Scope (explicit)

- 服务端搜索（消息/作者）与按作者/日期/文件过滤（本次延后，独立任务）
- 提交历史本地缓存（属性能优化独立任务，路线图.md:91 另列）
- 并发操作控制（路线图.md:96 另列）
- 三向合并编辑器（路线图.md:203 差异工具集成另列）
- 分支对比（两分支总览差异，range diff 未来演进项）

## Technical Approach

### 后端（service/git.go + app_git.go）

新增 GitService 方法（git CLI 路线，复用 gitCmd）：

- `GetCommitFileDiff(repoPath, sha, file string) (string, error)`
  - 普通提交：`git diff <sha>^ <sha> -- <file>`
  - root commit（无 parent）：`git diff --root <sha> -- <file>` 或对比空树 `git diff <EMPTY_TREE> <sha> -- <file>`
  - 返回 unified diff 文本，空串=无差异/二进制，前端 parseDiff 处理
- `GetRangeDiff(repoPath, baseSHA, headSHA string) (string, error)`
  - `git diff <baseSHA> <headSHA>`
  - 全文件 unified diff，前端按 `diff --git` 头拆分文件分组展示

App 层薄封装：`GetCommitFileDiff` / `GetRangeDiff` 转调 service。

root commit 判定：`git rev-parse <sha>^` 失败即 root，走 `--root` 或空树对比。

### 前端

- `FileDiffDialog.vue` 泛化：props 增 `diffText`（直接传入）或 `diffSource`（commit/range + 参数），内部分发调用对应后端方法；保留 `parseDiff` 与双栏渲染、二进制兜底
- `CommitHistory.vue`：
  - 变更文件 tag 加点击事件 → 弹 FileDiffDialog（传 sha + file，调 `GetCommitFileDiff`）
  - 列表行加 el-checkbox，维护 selectedSet（限选 2 个），头部加「对比」按钮 → 调 `GetRangeDiff` → 弹 FileDiffDialog 展示 range diff（按文件分组）

### 实施切分（小 PR）

- **PR1**：后端 `GetCommitFileDiff` + `GetRangeDiff` + service 单测（含 root/二进制兜底）+ wailsjs 绑定同步
- **PR2**：前端 FileDiffDialog 泛化 + CommitHistory 文件 tag diff 弹窗 + 组件测试
- **PR3**：CommitHistory range 勾选对比 UI + 组件测试 + 路线图/README 更新

## Technical Notes

- 现有 `GetDiff` 用 `git diff HEAD -- file`，commit diff 沿用同模式换 SHA 区间
- `FileDiffDialog.parseDiff`（前端）已解析 `@@ -l,l +r,l @@` hunk 头为左右栏行号，range diff 多文件场景需按 `diff --git a/ b/` 头拆分后逐文件喂 parseDiff
- 空树 SHA（root commit 对比基准）：`4b825dc642cb6eb9a060e54bf8d69288fbee4904`，git 通用空树哈希
- git CLI 对二进制文件输出 `Binary files ... differ`，前端 FileDiffDialog 已有 `/^Binary files /m` 兜底
- 跨平台路径：`os.DevNull` 已用于未跟踪 diff，commit diff 不涉及

## Research References

（本任务沿用现有 git CLI 模式，无需外部研究）
