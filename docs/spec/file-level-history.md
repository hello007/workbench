# 文件级提交历史契约（文件树右键「查看历史」）

> 来源：11-07-file-level-history 任务（2026-11-07）。域入口 `service/git_file_history.go`（GitService 方法），App 绑定 `GetFileCommitHistory`（app_git.go），前端 `FileHistoryDialog.vue`（FileTreePanel 文件节点右键挂载）。与仓库级 CommitHistory（commit_history_cli.go + CommitHistoryCache）形成追溯闭环，但不共享缓存与组件。

## 1. 绑定与模型契约

| 绑定 | 签名 | 说明 |
|---|---|---|
| `GetFileCommitHistory(fileAbsPath, limit, offset)` | `[]model.FileCommit` | **入参为文件绝对路径**（单参数自包含）：后端 `FindGitRoot(fileAbsPath)` 定位仓库根并 `filepath.Rel` 换算仓库相对路径；前端文件树右键直传 `data.path` |

`model.FileCommit`：SHA/ShortSHA/Message/Author/Email/Timestamp/DateTime + 三个文件级专有字段：

- **`pathAtCommit`**（必有）：该提交**之后**文件的仓库相对路径。--follow 历史中随重命名逐段变化——这是 diff 查看的正确 pathspec（`GetCommitFileDiff` 在 gitRoot 下执行 `git diff sha^ sha -- <file>`，用当前路径查旧段提交会得到空 diff）
- **`renameFrom` / `renameTo`**（omitempty）：该提交重命名/复制了目标文件（--name-status R/C 行）时的变更前后路径

wailsjs 三处已手动同步（App.js 三参绑定 / App.d.ts 返回 `Array<model.FileCommit>` / models.ts class；omitempty → `renameFrom?` 可选字段）。

## 2. git 调用契约（坑位）

```
git -c i18n.logOutputEncoding=UTF-8 -c i18n.commitEncoding=UTF-8 -c core.quotepath=off \
    log --follow --name-status --format="%x00%H%x01%an%x01%ae%x01%at%x01%ai%x01%B%x01" \
    -n <offset+limit> -- ":(literal)<relPath>"
```

- **`:(literal)` pathspec 魔术前缀（本域新增先例）**：文件名可含 glob 元字符（如「笔记[1].md」），裸 pathspec 的 glob 语义会把 `[...]` 解释为字符组导致匹配不到；literal 按字节精确匹配。路径须 `filepath.ToSlash` 归一（Windows 下 `filepath.Rel` 返回反斜杠）
- **utf8Prefix + %x00/%x01**：与 branch-compare.md 范式一致（中文文件名/提交消息）；`--name-status` 的 R/C 行为 `R100\t<old>\t<new>` 双路径 tab 分隔，普通行 `<status>\t<path>`
- 格式串无 `%P`（parents 对文件级历史无消费），消息终止锚点 `\x01\n\n` 反向定位（`LastIndex`，消化消息内 0x01），与 `parseCommitLogChunk` 同构但字段数不同（头部 5 字段非 6）
- **只读不抢 tryLockRepo**；30s 超时由 `gitCmd.Execute` 兜底
- 解析 fail-soft：单条脏记录跳过 + `Logger().Warn`（列表展示场景，对齐 parseNameStatus 畸形行跳过先例），非 fail-fast——与 commit_history_cli 批量统计场景（错数据毁统计必须 fail-fast）的取舍不同

## 3. 分页语义

- `-n offset+limit` 批量读取 + 内存窗口截取（钳制上限 `fileHistoryMaxFetch=10000`）；--follow 无法廉价取总数
- 前端 hasMore = **返回满 limit 条即有下一页**（与仓库级 CommitHistory `PAGE_SIZE=20` 同语义）
- offset 越界返回空切片非 nil；不存在路径返回空切片不报错（git log 对无历史路径空输出）

## 4. 前端契约（FileHistoryDialog）

- **加载驱动**：`watch(modelValue, ..., { immediate: true })`——immediate 覆盖「以 visible=true 初始挂载」形态（重挂载残留开启态也正常拉取；非 immediate 时初始 true 不触发是真实缺口）
- **关闭双时机（perf-baseline §16.5 对齐）**：`@closed` 清列表/加载态/内嵌 diff 状态（第一时机）+ `loadSeq` 代次守卫（第二时机）：watch false 分支即时 `loadSeq++`，在途 loadMore 响应写回、错误提示、finally 复位三处判 seq 丢弃——防渐隐期在途响应写回已清空列表
- **内嵌 diff**：FileDiffDialog mode=commit，`:file="commit.pathAtCommit"`（禁传当前路径，见 §1）；点击行打开，行级语义锚 `data-action="file-history-row"` / `data-action="file-history-load-more"`
- **rename 标记**：提交行 `R` 小 tag + el-tooltip `重命名：<old> → <new>`

### 入口判定（近似，有边界）

菜单项显示条件 = `type === 'file'`（**目录不提供入口**：--follow 仅支持单一路径，目录级追溯语义不成立）&& 仓库归属近似判定：

- 普通模式：复用 `blankAreaIsGitRepo`（当前工作目录 isGitRepo）
- 聚合模式：**恒显示**（收藏根的 isGitRepo 在懒加载根层节点上，无廉价单文件归属判定），非 git 目录点击后由后端 `FindGitRoot` 失败错误提示兜底

## 5. 验证与错误矩阵

| 条件 | 行为 |
|---|---|
| fileAbsPath 空 | `文件路径不能为空`（App/GitService 双层守卫） |
| 文件不在 gitRoot 内（Rel 越界） | `文件不在 Git 仓库内: <path>` |
| 非仓库目录 | `无法定位 Git 仓库根目录`（FindGitRoot 失败，前端 handleError 分流） |
| git log 失败 | `获取文件提交历史失败: %w` |
| 记录头部不完整/SHA 非法/终止符缺失 | 跳过该条 + Warn（不阻断整体） |

## 6. 测试要点

- **--follow 铁证断言**：重命名前旧段提交的 `pathAtCommit` 必须呈现旧路径（file.txt）——这是 follow 生效的可观察证据，仅断言条数不够
- 中文往返：中文文件名 rename（设计文档.md → 评审稿.md）与中文消息往返；glob 元字符文件名（笔记[1].md）锁 :(literal)
- 分页窗口：两页 SHA 交集为空的正向断言（禁 `A.SHA != B.SHA && A.msg == B.msg` 型恒假组合——check 阶段发现的假绿）
- 组件测试：el-dialog stub 须 `watch modelValue → emit('closed')` 驱动 releaseState（stub 不发 closed 则关闭清态逻辑从未执行，属假绿）；tooltip 断言精确文案非存在性
- 倒序语义：rename 提交下标随提交数变化，断言前先想清倒序（最新在前）再定位
