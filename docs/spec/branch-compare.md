# 分支对比契约（侧边栏「分支对比」）

> 来源：移植 git-branch-compare（Electron 项目）的成熟对比算法；第一期纯只读（2026-10-23，任务 `10-22-branch-compare`）。操作类功能（cherry-pick/merge/rebase/冲突）复用既有 Git 面板，本面板不提供。

## 方法签名（4 个 App 绑定，全部只读）

```
GetBranchCompareAlignment(repoPath, branchA, branchB string) *model.BranchCompareAlignment
GetBranchCompareFileTree(repoPath, branchA, branchB string) []model.BranchDiffFile
GetBranchCompareFileDiff(repoPath, base, target, filePath string) string
GetCommitChangedFiles(repoPath, sha string) []model.FileChange   // 复用 model/commit.go 的 FileChange
```

App 方法不带 ctx（对齐 `GetCommitHistory` 模式），内部 `context.Background()` 下传；service 层 ctx 全贯穿，为 batch 2 取消预留。**方向语义**：文件树 diff 方向为 A → B（仅 A 有 ⇒ `deleted`，仅 B 有 ⇒ `added`）；diff 文本为 `git diff <base> <target> -- <file>` 原始 unified 输出（空串 = 无差异或二进制占位）。

## 核心算法契约

1. **对齐数据一次调用全返回**：`git merge-base A B`（失败容忍 → 空串 = 无共同祖先）→ `git rev-list --count <mergeBase>`（容忍 → 0）→ 并发 `git log B..A` / `A..B`（`--max-count=2000 --topo-order`，达上限 `Truncated=true` 显式标注不静默）→ 并发两条 patch-id 管道 → 后端配对 + 统计。
2. **配对在后端**（`pairCommits` 纯函数）：贪心一配一无回溯，三级匹配 hash → patch-id → 归一化 subject（剥离 `(cherry picked from commit ...)` 尾巴 + trim + lowercase）；行按 `max(a.date, b.date)` 降序（同秒 hash 决胜保证确定性）。浏览器模式与 E2E mock 拿同一形状。
3. **patch-id 管道**：`git log <range> -p --format=%H` | `git patch-id --stable`，走 `util.GitCommand.ExecutePipeline`（cmd1.StdoutPipe → cmd2.Stdin，错误归一化镜像 run()：取消/超时 E_GIT_TIMEOUT/锁 E_GIT_LOCKED）。merge commit 在 `git log -p` 无 patch 输出 → 无 patch-id → 天然不被误配。**管道失败降级为仅 hash/subject 匹配（Logger().Warn + patchId 留空），不阻塞主流程**。
4. **文件树并集**：两侧 `git ls-tree -r --name-only` + `git diff --name-status`（R/C 双路径）+ **单趟** `git diff --numstat`（`-` ⇒ Binary，禁 N+1 逐文件调用）；不在 diff map ⇒ unchanged。numstat rename 行须处理 git 花括号形式 `dir/{old => new}`（公共前缀/后缀在括号外），新旧路径都注册。
5. **日志记录格式**：`"%x00%H%x01%h%x01%an%x01%ae%x01%ct%x01%s%x01%b%x01%P"`（8 字段，沿用 `commit_history_cli.go` 的 %x00/%x01 范式）；解析以末段锚定 parents + 40 位 hex 头校验，subject/body 中的注入字段折叠回 body。

## git 调用硬约束

- **每个 git 命令必须带 UTF-8 前缀**：`-c i18n.logOutputEncoding=UTF-8 -c i18n.commitEncoding=UTF-8 -c core.quotepath=off`（现有 `util.GitCommand` 不默认带，属既有独立问题，勿假设全局已覆盖）。
- patch-id 管道专用 `NewGitCommandWithTimeout(2 * time.Minute)`（30s 默认对大仓不够）。
- 只读查询**不抢** `tryLockRepo`（对齐 ListTags 惯例）；本功能零新增错误码（复用 E_GIT_TIMEOUT / E_GIT_CANCELLED / E_GIT_LOCKED）。

## 前端接入

- 面板 id `branchcompare`（ActivityBar panels 数组 + `store/ui.js` 注释并集 + `Home.vue` v-show 常驻挂载）；仓库默认取 `useDirectoryStore().currentDirPath`，非仓库空态引导浏览（`OpenDirectoryDialog` + `GetGitInfo` 校验）。
- diff 解析复用 `utils/diffParse.js`（自 `FileDiffDialog.vue` 逐字抽取的 `parseDiff`/`parseRangeDiff`，FileDiffDialog 改 import，行为零变化）；文件树构建在 `utils/branchFileTree.js` 纯函数。
- 每个异步请求带 `requestSeq` 守卫丢旧响应；文件 diff 走 `gitCache`（键 `branchCompare:<repo>:<a>:<b>:<file>`），刷新按钮清缓存强拉。
- 文件树 UI 行为：**unchanged 默认隐藏**（工具栏「未变更」开关或状态过滤显式勾选「未变更」时显示，两者 OR）；目录行点击收放——展开态 Map 必须 `reactive`（渲染函数闭包内的普通 Map 变更不触发重渲染，v1.11.0 冒烟曾因此点击无反应）。
- UI 文案中文硬编码；状态色全部走 design tokens（typechange 无青色 token，用 `--info-color`）。

## 测试分层

- 纯函数单测：`service/branch_compare_test.go`（配对三级/降级/一配一/排序/解析注入防御/花括号 rename）；`util/git_pipeline_test.go`（真 git 小仓管道）。
- 集成测试：`branch_compare_integration_test.go`（`//go:build integration`）——真实 fixture 覆盖真 cherry-pick 同 patch-id、rename 双路径、二进制、unchanged 并集、**无共同祖先（同一仓库 orphan 分支表达，两个独立仓库无法在一条命令里引用）**、中文错误分流。
- mock 单一数据源 `src/test/wails-mock-defaults.js`（`GetCommitChangedFiles` 的 `staged` 恒 false，历史提交无暂存态）。

## batch 2 预留（非目标备忘）

cherry-pick/merge/rebase 入口须校验当前分支（源项目语义：拣选作用于当前 HEAD，绝不自动 checkout）；分支选择持久化走 `util.SaveJSON` 原子写独立 `data/branch_compare.json`；取消/进度事件须同步 `web_serve.go applySink`。
