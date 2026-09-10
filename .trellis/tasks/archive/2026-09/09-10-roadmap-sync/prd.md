# 路线图与功能说明同步核对

## Goal

WorkBench 经多版本迭代后,`docs/路线图.md` 与实际代码实现状态、`docs/功能说明.md` 描述出现偏差:路线图多项 Git 基础功能仍标 `[ ]` 未完成,但功能说明与代码显示已实现。本任务核对实际代码,补勾选已完成项、修正未完成项描述,使路线图反映真实进度,并为后续迭代给出真缺口清单。

## What I already know

### 代码实现状态(已查 `service/git.go` GitService 方法)

| 路线图项 | 路线图标记 | 代码实际 | 判定 |
|---|---|---|---|
| Git 提交和推送 | [ ] | `Commit`(选择性 pathspec)/`Push`(setUpstream) | ✅ 已实现 |
| 查看所有分支(本地+远程) | [ ] | `GetBranches` | ✅ 已实现 |
| 切换分支 | [ ] | `CheckoutBranch`(含 isRemote) | ✅ 已实现 |
| 创建新分支 | [ ] | 无 `CreateBranch` | ❌ 未实现 |
| 删除分支 | [ ] | 无 `DeleteBranch` | ❌ 未实现 |
| 重命名分支 | [ ] | 无 `RenameBranch` | ❌ 未实现 |
| 查看未暂存更改 | [ ] | `GetLocalChanges` | ✅ 已实现 |
| 查看已暂存更改 | [ ] | `GetLocalChanges` 返回 `Staged` 字段(数据层区分);前端变动面板单区展示未分双栏 | ⚠️ 部分 |
| 行级 diff | [ ] | `GetDiff`(双栏对照) | ✅ 已实现 |
| 暂存/取消暂存单文件 | [ ] | 无 `Stage`/`Unstage` | ❌ 未实现 |
| 丢弃本地更改 | [ ] | `DiscardChanges` | ✅ 已实现 |
| 克隆远程仓库 | [ ] | `Clone` | ✅ 已实现 |
| 拉取更新 | [ ] | `Pull`/`BatchPull` | ✅ 已实现 |
| 批量拉取(多仓库) | [ ] | `BatchPull` 并发+进度 | ✅ 已实现 |
| 提交历史分页查看 | [ ] | `GetCommitHistory`(go-git Log,limit/offset 分页);`GetLog` 为 stub 死代码未用 | ✅ 已实现 |
| 提交详情(作者/日期/文件) | [ ] | `GetCommitHistory` 返回 Author/Email/SHA/Message;日期 When 待核;单条 commit 文件列表未实现 | ⚠️ 部分 |
| commit 间 diff | [ ] | 无 | ❌ 未实现 |
| 搜索提交历史 | [ ] | 无 | ❌ 未实现 |
| 按作者/日期/文件过滤 | [ ] | 无 | ❌ 未实现 |
| 合并/变基 | [ ] | 无 | ❌ 未实现 |
| 标签管理 | [ ] | 无 | ❌ 未实现 |
| 远程仓库管理(增删/fetch) | [ ] | 仅 `HasRemote`/`HasRemotesBatch` 查询,无管理 | ❌ 未实现 |
| Submodule | [ ] | 无 | ❌ 未实现 |

### 功能说明第 4 节已列(印证)

功能说明.md 第 4 节「Git 集成」明确列出:克隆、拉取、切换分支、本地变动列表、diff、选择性提交、提交并推送、回滚、丢弃、分页历史、批量更新。与代码一致,但路线图未同步勾选。

## Open Questions

(无——范围边界已决,见 Decision)

## Decision (ADR-lite)

**Context**: 真缺口(创建/删除/重命名分支、暂存单文件、历史搜索过滤、Merge/Rebase、Submodule)不在 git-tag-remote 兄弟任务覆盖范围,需定本任务是否吞下。

**Decision**: 选项 A——本任务保持纯文档核对,不实现代码。真缺口清单记入父任务 `09-10-wb-phase-iteration` 的 prd 作为后续迭代候选,父任务维持 4 子任务。标签+远程由 git-tag-remote 覆盖,其余真缺口单独排期。

**Consequences**: 范围不蔓延,本任务快速收口;真缺口不丢失,留待父任务收尾后评估优先级再立项。

## Requirements

1. 逐项核对 `docs/路线图.md` 中 Git 增强功能区块,标注实际实现状态
2. 已实现项补勾选 `[x]` 并修正描述使其与功能说明.md 一致
3. 部分实现项(如「查看已暂存更改」「提交详情」)标注现状与缺口
4. 真未实现项保留 `[ ]`,描述精确化
5. 同步「已完成的里程碑」表与最后更新日期
6. 真缺口清单记入父任务 prd「后续迭代候选」段,不在本任务实现

## Acceptance Criteria

* [ ] 路线图.md Git 增强功能区块标记与 `service/git.go` 实际实现一致
* [ ] 已实现项全部 `[x]`,无遗漏
* [ ] 部分实现项有明确现状标注
* [ ] 真未实现项描述精确,可作后续迭代输入
* [ ] 里程碑表与功能说明.md 无矛盾
* [ ] 最后更新日期更新为 2026-09-10

## Definition of Done

* 路线图.md 修订提交,commit message 符合规范
* 功能说明.md 与路线图.md 交叉引用无矛盾
* 真缺口清单已分流(拆任务或记入父任务)

## Out of Scope

* 任何代码实现(纯文档核对任务)
* 非 Git 增强区块的路线图项(本次聚焦 Git 域失同步)
* 性能/测试/AI 等其他区块(父任务其他子任务覆盖)

## Technical Notes

* 实现核对源:`service/git.go` GitService 方法集
* 描述对照源:`docs/功能说明.md` 第 4 节
* 父任务:`09-10-wb-phase-iteration`
* 兄弟任务:`09-10-git-tag-remote`(标签+远程管理已在该任务覆盖,核对时注意边界)
* 技术债:`GetGitLog`(app_git.go:29)+ `GitService.GetLog`(service/git.go:67)为 stub 返回空,前端无调用,属死代码,记入父任务后续清理候选(纯文档任务不删代码)
