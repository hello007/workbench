# 已知风险留笔（本次范围外，审核发现）

## 取消杀 `git commit` 的 index.lock 残留面（🟡，未处理）

**发现**：ContinueMerge 的取消实现为终止 `git commit --no-edit` 子进程（Windows TerminateProcess）。commit 写窗口全程持 `.git/index.lock`，进程被杀后锁文件残留，该仓后续全部写操作报 `index.lock exists`，UI 无探测无引导。

**评估**：
- 该风险非本次引入——既有 Pull/Clone 取消（866b7a1）同样 TerminateProcess 杀 git 子进程（git pull 内部同样有 merge/commit 写窗口），取消机制共性面。
- ContinueMerge（显式 commit）被杀概率高于 pull 路径，暴露面略大。
- 修复方向（若后续立项）：取消路径收口时探测 `<gitroot>/index.lock` 残留并 `slog.Warn`（不自动删——stale lock 判定不可靠），或前端取消提示文案补排查引导。
- Abort 三类（中止）不走取消链路（刻意无 ctx 变体），`git merge --abort` 秒级完成，无此面。

**决策**：本次 PRD 明确「复用既有取消注册表，禁另起路径」，探测/引导属行为新增，超出范围。留笔待后续任务评估。
