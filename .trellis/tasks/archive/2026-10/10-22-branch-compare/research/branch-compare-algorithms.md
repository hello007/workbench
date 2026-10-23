# 研究：git-branch-compare 算法移植摘要

源项目：`D:\claudecode\git-branch-compare`（Electron + React，纯 `execFile('git')`，零 Electron 耦合，可直译 Go）。设计文档：`D:\claudecode\docs\plans\2026-06-22-git-branch-compare-design.md`。

## 1. 对齐数据 getCommitGraph()（src/main/git-service.ts:559-635）

1. `git merge-base A B` → 分叉点（失败/无共同祖先 → 空串容忍）
2. `git rev-list --count <mergeBase>` → 共同历史条数
3. `git log B..A --format=... --max-count=2000 --topo-order` → A 独有；`A..B` → B 独有
   - 源项目用 `%H|%h|%an|%ae|%ct|%s|%b|%P%x00`（NUL 记录、`|` 字段）；**移植改用 WorkBench 的 `%x00`/`%x01` 范式**（见 research/workbench-git-plumbing.md）
4. 返回 `AlignmentData { mergeBaseHash, commonHistoryCount, branchA[], branchB[] }`

## 2. patch-id 批量管道 getPatchIdMap()（git-service.ts:506-536）

- 两进程管道：`git log <range> -p --format=%H` stdout → `git patch-id --stable` stdin（Go 用 `cmd1.StdoutPipe` → `cmd2.Stdin`，无 shell）
- 输出行 `<patchId> <commitHash>` → map hash→patchId
- 关键性质：
  - `--stable` 使 patch-id 对 hunk 偏移/顺序不敏感 → 跨分支等价提交（含真 cherry-pick）同 id
  - merge commit 在 `git log -p` 默认无 patch 输出 → 无 patch-id → 天然不被误配
- 所有 git 调用须带 `-c i18n.logOutputEncoding=UTF-8 -c i18n.commitEncoding=UTF-8 -c core.quotepath=off` + `LC_ALL=en_US.UTF-8`（Windows 中文路径/文件名关键）

## 3. 配对算法（renderer/src/components/CommitGraph.tsx:49-100，客户端纯逻辑）

1. 两侧按日期降序
2. 对每个 A 提交依序找第一个未使用的 B 提交：(1) hash 精确 → (2) patchId 相等 → (3) 归一化 subject；都不中 → a-only
3. 剩余 B → b-only 行
4. 全部行按 `max(a.date, b.date)` 降序

归一化：剥离 `\s*\(cherry picked from commit [^)]+\)\s*`，trim，lowercase。贪心一配一、无回溯（非全局最优，行为可预测）。

## 4. 文件树并集 getFullFileTree()（git-service.ts:240-301）

- 两侧 `git ls-tree -r --name-only <rev>` + `git diff --name-status base target`
- 状态码 A/D/M/T + R/C（R、C 行格式 `status \t oldPath \t newPath`）
- 全路径并集；不在 diff map ⇒ unchanged；rename 旧路径缺失则补回；按 path 排序

## 5. diff 统计 getDiffStats()（git-service.ts:305-363）

源项目逐文件 `git diff --numstat base target -- path`（N+1 次调用）；**移植优化为单趟** `git diff --numstat base target`，输出 `add \t del \t path`，`-` ⇒ binary。

## 6. 文件 diff getFileDiff()（git-service.ts:365）

`git diff base target -- <file>` 原始 unified 文本；二进制经 stderr 含 "binary" 判定（Go 移植：stdout 含 `Binary files` 即可，numstat 亦有 `-` 信号）。

## 7. 提交详情

- 变更文件：`git diff-tree --no-commit-id --name-status -r <sha>`（回退 `git show --format= --name-status <sha>`）
- 单文件提交 diff：`git show --format= <sha> -- <file>`（WorkBench 已有 `GetCommitFileDiff`，root commit 对空树）

## 8. 前端 unified diff 解析（DiffView.tsx:32-98 → WorkBench 已有等价物）

`diff/index/---/+++/@@` 头行 → 双栏公共；空格前缀 → 双栏；`-` → 左列；`+` → 右列。WorkBench `FileDiffDialog.vue` 的 `parseDiff/parseRangeDiff` 已覆盖此逻辑（含二进制段、`\ No newline`），**抽取复用而非重写**。

## 9. 仓库状态检测 getRepoStatus()（git-service.ts:112-164）

`git status --porcelain -b` + `REBASE_HEAD`/`MERGE_HEAD`/`CHERRY_PICK_HEAD` 存在性判定。**第一期只读不需要**；batch 2 做操作时再移植（WorkBench 亦有等价能力）。

## 保留的行为语义

- cherry-pick 作用于当前 HEAD、绝不自动 checkout（第一期不实装操作，batch 2 须校验当前分支）
- 2000 条上限：`--max-count=2000` + Truncated 显式标注，不静默截断
- 匹配徽标：hash / patch-id（疑似拣选）/ subject 三级展示
