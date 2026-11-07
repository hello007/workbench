# Stash 管理契约（Git 签页「Stash」tab）

> 来源：11-07-stash-management 任务（2026-11-07）。域入口 `service/git_stash.go`（GitService 方法），App 绑定 `ListStashes / StashPush / StashPop / StashApply / StashDrop`（app_git.go），前端 `GitStash.vue`（ContentPanel stash tab，lazy）。

## 1. 绑定与模型契约

| 绑定 | 签名 | 说明 |
|---|---|---|
| `ListStashes(path)` | `[]model.StashEntry` | 空列表返回空切片非 nil（前端零值稳定） |
| `StashPush(path, message, includeUntracked)` | `error` | message 空 → git 默认 WIP 文案；includeUntracked → `-u` |
| `StashPop(path, index)` / `StashApply(path, index)` | `*model.StashApplyResult` | 冲突态走正常返回（见 §2） |
| `StashDrop(path, index)` | `error` | 无冲突语义，直接删条目 |

`model.StashEntry`（index/description/branch/shortSha/dateTime/timestamp）与 `model.StashApplyResult`（output/conflictFiles）均为普通 struct（非具名 string），wails 正常生成 models.ts class，无别名缺口。

## 2. pop/apply 冲突语义（核心决策）

git 默认：pop/apply 冲突时**保留 stash 条目（pop 亦不 drop）**，工作区留未合并文件。本域如实转译该语义，不新增错误码：

- **冲突判定**：`git status --porcelain` 的未合并 XY 八组合（DD/AU/UD/UA/DU/DA/UU/AA），`unmergedFiles()` 单点。**禁解析 stderr 的 CONFLICT 行取文件清单**——porcelain 是 git 自身标记的权威态，CONFLICT 行文案随冲突类型变体（modify/delete 等）路径后缀不可靠。
- **冲突详情输出**：`util.GitCommand.ExecuteWithCodesAndStderr`（exit 1 白名单 + 返回 stdout/stderr 双流）。**必须用此变体**：`ExecuteWithCodes` 白名单命中只返回 stdout，而 stash pop/apply 的冲突详情（CONFLICT 行、Auto-merging）走 **stderr**——这是新增该 util 变体的唯一动机（util/git.go `runFull` 抽取单点收口，`run()` 为丢 stderr 薄壳）。
- **exit 1 分流**：白名单命中且无未合并文件时，stderr 含 `error:` 行（行级 `strings.HasPrefix(TrimSpace(line), "error:")`）= 真实失败（如本地变更会被覆盖），返回 error；仅进度文案（Auto-merging）= 正常完成。
- **前端**：`ConflictFiles` 非空 → 冲突 Dialog（完整输出 pre + 冲突文件 tag + 「stash 条目已保留」提示），冲突解决引导既有「合并/变基」tab 红点链路，不重复造轮子。

## 3. 编码契约（踩坑）

- stash 域**全部** git 调用统一前置 `utf8Prefix()`（branch-compare.md 定义），**含 `unmergedFiles` 的 `git status --porcelain`**——缺 `core.quotepath=off` 时中文文件名输出八进制转义+引号包裹，冲突文件清单即坏（check 阶段发现并修复，有 `TestUnmergedFiles_ChineseFilenameRoundTrip` 回归锁）。
- 列表解析：`stash list --date=iso --format="%x00%gd%x01%gs"`；`--date=iso` 使 `%gd` 变形为 `stash@{<ISO 时间>}` 借此取创建时间，**index 退化为记录序号**（stash list 顺序即 stash@{n}）；reflog 主题三形态：`WIP on <branch>: <sha> <msg>`（自动生成）/ `On <branch>: <msg>`（**-m 自定义，无 SHA 段**，ShortSha 为空是正确行为非解析缺陷）/ `index on <branch>: ...`（伴生条目）。正则 `^(?:WIP on|index on|On) ([^:]+): ...`——注意 `On` 后直接接分支，`WIP/index` 后才是 `on`。

## 4. 取消与锁语义（对既有先例的收敛）

变更类操作（push/pop/apply/drop）与 `Commit`/`DiscardChanges` 逐字同构：`tryLockRepo`（E_GIT_IN_PROGRESS 互斥）+ `FindGitRoot` + `Execute`（30s 超时兜底，锁识别 E_GIT_LOCKED / 超时 E_GIT_TIMEOUT 经 util 既有链路透传）。**不注册取消入口**（stash 为秒级单命令，PRD 原拟 registerCancel 链路按 Commit 先例收敛，无独立取消 UI）。ListStashes 只读不抢仓级锁。

## 5. 测试要点

- **CRLF 归一**：autocrlf 环境下 stash pop 恢复的工作区文件行尾为 CRLF（git 写出时转换），内容断言须 `strings.ReplaceAll(content, "\r\n", "\n")` 归一后比较，禁裸等值。
- **中文名往返**：push 描述与冲突文件名均须中文用例（UTF-8 前缀回归面）。
- 冲突构造：stash → 同文件提交另一内容 → pop；断言 ConflictFiles + stash 条目保留（pop 冲突不 drop）。
