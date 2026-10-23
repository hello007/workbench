# 分支对比

## Goal

跨分支评审、cherry-pick 追踪与发布核对在现有 Git 面板中缺少载体：现有提交历史只能看单分支线性历史，无法回答「这两个分支各自独有哪些提交、哪些提交是互相拣选的等价提交、分支间文件差异全貌如何」。本任务移植 git-branch-compare（Electron 项目）已验证的对比算法（merge-base 双侧区间日志 + patch-id 批量管道 + 贪心三级配对），在活动栏新增一级面板「分支对比」，第一期做**纯只读对比**，操作类功能复用既有 Git 面板能力。

## Requirements

| # | 需求 | 说明 |
|---|------|------|
| R1 | 仓库选择 | 默认取当前工作目录（`currentDirPath`，非 git 仓库时空态引导）；支持浏览按钮换任意目录（复用 `OpenDirectoryDialog`） |
| R2 | 分支 A/B 选择与交换 | A/B 两个下拉（本地+远程分支，remote 名 `origin/xxx` 直接可作 rev）；默认 A=当前分支（detached 留空）；一键交换 A/B 并重新拉取 |
| R3 | 双列对齐提交表 | 左列 A 独有提交、右列 B 独有提交；配对提交同行显示；三级匹配 hash → patch-id → 归一化 subject；匹配徽标区分三种匹配方式（patch-id 即疑似 cherry-pick） |
| R4 | 统计条 | A 独有 / 匹配（含疑似拣选数）/ B 独有 / merge-base 之下共同历史计数；任一侧触及 2000 条上限时明示截断 |
| R5 | 分支差异文件树 | 两侧 `ls-tree -r` 并集（**含 unchanged**），状态着色（added/deleted/renamed/copied/typechange/modified/unchanged）+ 状态过滤 + 路径过滤；rename 显示新旧双路径 |
| R6 | 双栏/统一 diff 查看器 | 选中文件展示 `git diff base target -- file` 原文解析结果，双栏/统一视图切换；二进制文件占位提示 |
| R7 | 提交详情 | 点击对齐表提交行弹层展示该提交变更文件列表；点文件查看该提交的单文件 diff（复用 `GetCommitFileDiff`） |
| R8 | 对齐表搜索过滤 | 按 hash / subject / author 子串过滤，任一侧命中即整行显示；纯前端 |
| R9 | 只读 | 不提供任何写操作（cherry-pick/merge/rebase 按钮均不做，走既有 Git 面板） |
| R10 | 错误分流 | 超时（E_GIT_TIMEOUT）/ 取消（E_GIT_CANCELLED）/ 锁（E_GIT_LOCKED）走既有 handleError warning 分流；中文提示 |

## 已定决策

- **配对在后端**：`pairCommits` 纯函数放 service 层，浏览器模式与 E2E mock 拿到同一形状，避免前端复刻算法。
- **日志记录格式**：沿用 `commit_history_cli.go` 的 `%x00` 记录 / `%x01` 字段分隔（格式串 `"%x00%H%x01%h%x01%an%x01%ae%x01%ct%x01%s%x01%b%x01%P"`），比源项目 `|` 分隔健壮（subject/body 含 `|` 不炸）。
- **UTF-8 前缀参数**：本服务所有 git 命令带 `-c i18n.logOutputEncoding=UTF-8 -c i18n.commitEncoding=UTF-8 -c core.quotepath=off`（现有 `util.GitCommand` 不默认带，属既有独立问题，本任务不扩大范围去改全局）。
- **patch-id 管道专用长超时**：`NewGitCommandWithTimeout(2 * time.Minute)`；管道失败降级为仅 hash/subject 匹配（patchId 留空），不阻塞主流程。
- **不持久化**：面板 v-show 常驻已保留会话内状态；跨会话记住上次 A/B 选择留 batch 2（届时走 `util.SaveJSON` 原子写）。
- **App 方法不带 ctx**：对齐 `GetCommitHistory` 等现有模式，内部 `context.Background()` 下传，service 层 ctx 全贯穿为 batch 2 取消预留。
- **numstat 单趟**：文件树行数统计用一趟 `git diff --numstat`，不做源项目的 N+1 逐文件调用。

## 非目标

- 对比面板内嵌 cherry-pick / merge / rebase 入口与冲突解决器（WorkBench 已有 `GitMerge.vue` 与 service 层 Merge/Rebase/CherryPick/冲突链路）。
- 文件级提交历史对比（源项目 FileCommitLogView）。
- SVG DAG 提交图（源项目已弃用该实现）。
- 分支选择持久化、对比结果导出。

## 验收标准

- R1–R10 全部落地并可手动冒烟验证。
- `go test ./...` 通过且 service 包覆盖率 ≥76%；`go test -tags=integration ./...` 通过（真实 git fixture 覆盖 cherry-pick 同 patch-id、rename 双路径、二进制、无共同祖先）。
- 前端 `npx vitest run` 通过且覆盖率 ≥70%（新增 diffParse 抽取后既有 FileDiffDialog spec 不回归）。
- Playwright e2e（mock channel）新增 branch-compare 用例通过。
- 文档同步：`docs/功能说明.md`、`docs/spec/branch-compare.md`（新 spec）、`CHANGELOG.md`、`README.md`、`路线图.md`。
