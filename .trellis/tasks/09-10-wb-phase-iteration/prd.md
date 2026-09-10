# WorkBench 收尾期四阶段迭代

## Goal

v1.3.0 AI 全量上线后的收尾稳定期,按序推进四个子任务,补全 Git 域缺口、打磨体验、夯实性能。父任务作为容器,子任务按序独立 brainstorm + 实现 + check;父任务承载跨子任务核对出的真缺口与技术债评估,留待收尾后分流。

## 子任务清单（按序迭代）

| 序 | 子任务 | 优先级 | 状态 | 内容 |
|---|---|---|---|---|
| 1 | `09-10-roadmap-sync` | P2 | in_progress | 路线图与功能说明同步核对（纯文档） |
| 2 | `09-10-dark-theme` | P1 | planning | 暗色主题支持（系统跟随+手动切换+持久化） |
| 3 | `09-10-git-tag-remote` | P1 | planning | Git 标签与远程仓库管理 |
| 4 | `09-10-filetree-cache` | P2 | planning | 文件树缓存机制（内存+持久化+失效策略） |

## 后续迭代候选（来自 roadmap-sync 核对真缺口）

Git 增强域真未实现项,父任务四阶段未覆盖,留待收尾后评估优先级单独立项:

- 创建/删除/重命名分支（无 `CreateBranch`/`DeleteBranch`/`RenameBranch`）
- 暂存/取消暂存单个文件（无 `Stage`/`Unstage`）
- 查看提交之间的 diff
- 搜索提交历史 + 按作者/日期/文件过滤
- 合并/变基 + 冲突解决辅助
- Submodule 支持
- 已暂存/未暂存双栏展示（数据层 `Staged` 字段已有,前端变动面板单区）

## 技术债候选（核对副产物）

- **Git 死代码**:`App.GetGitLog`（app_git.go:29）+ `GitService.GetLog`（service/git.go:67）stub 返回空,前端无调用,提交历史实际走 `GetCommitHistory`。已记入路线图技术债务段。
- **Pinia 失同步**:路线图技术债务段「前端状态管理」仍标 `[ ]` 考虑引入 Pinia,但功能说明第 10 节显示 Pinia 已迁移完成（PR1-6 已合）。需补勾选。
- **测试覆盖率门禁失同步**:路线图「测试覆盖」段标 `[ ]` 后端>80% 前端>70%,但 CLAUDE.md 关键规则显示分层门禁已建（model/server≥80% + service≥76% + util≥40% + 前端≥70% 硬失败）。需补勾选并精确化阈值。

## Definition of Done

- 四子任务按序完成（各自 archive）
- 后续迭代候选与技术债候选已评估并分流（立项或明确搁置）

## Out of Scope

- 父任务不直接实现代码,实现下沉子任务
- 后续迭代候选不在本父任务周期内实现
