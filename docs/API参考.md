# WorkBench API 参考

> Wails 绑定方法清单与数据模型。全部方法提取自 `frontend/wailsjs/go/main/App.d.ts`（`wails generate module` 自动生成），按域分组；数据模型提取自 `frontend/wailsjs/go/models.ts`。
> 前端调用：`import { Method } from 'wailsjs/go/main/App'` 后 `await Method(args)`，运行时经 `window['go']['main']['App']['Method']` 调 Go 后端。
> 配套文档：[架构设计.md](架构设计.md) · 跨层契约见 [spec/cross-layer-contracts.md](spec/cross-layer-contracts.md)

---

## 1. 方法清单总览

共 **172** 个导出方法，分布在 18 个 `app_*.go` 域文件 + `app.go`，委托 `AppServices` 持有的 18 个 service/cache。按 27 个业务域分组：

| 域 | 方法数 | 实现文件 |
|---|---|---|
| 应用信息 | 1 | `app.go` |
| 目录管理 | 8 | `app_directory.go` |
| 收藏夹 | 5 | `app_favorites.go` |
| 文件树 | 8 | `app_filetree.go` |
| 文件预览与对话框 | 5 | `app_preview.go` |
| 文件操作与剪贴板 | 6 | `app_clipboard.go` |
| 搜索 | 2 | `app_search.go` |
| Git 仓库信息 | 6 | `app_git.go` |
| 提交历史与统计 | 5 | `app_git.go` |
| 本地变更与提交 | 7 | `app_git.go` |
| Diff | 5 | `app_git.go` |
| 分支 | 7 | `app_git.go` |
| 标签 | 4 | `app_git.go` |
| 远程 | 4 | `app_git.go` |
| 合并/变基/挑拣 | 12 | `app_git.go` |
| Submodule | 6 | `app_git.go` |
| 仓库元数据 | 5 | `app_repometa.go` |
| 仓库配置导入导出 | 3 | `app_repo_config.go` |
| 状态看板 | 6 | `app_dashboard.go` |
| AI 功能 | 21 | `app_ai.go` |
| AI 对话 | 19 | `app_chat.go` |
| 会话快照 | 2 | `app_session.go` |
| 终端 | 6 | `app_terminal.go` |
| 外部集成 | 9 | `app_external.go` |
| 设置 | 2 | `app_settings.go` |
| 网络访问 | 4 | `app_webserve.go` |
| 更新 | 4 | `app_update.go` |

> 签名约定：`arg1/arg2/...` 为位置参数（Wails 绑定不保留参数名），类型取自 `App.d.ts`。返回值均为 `Promise`，错误经 Wails `ErrorFormatter` 转为 `{code, message}` 或 `{message}` 对象（见 [架构设计.md](架构设计.md) 第 6 节）。

---

## 2. 应用信息

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetAppVersion` | `() => Promise<string>` | 获取应用版本号（ldflags 注入，dev 模式为 `dev`） | 版本字符串 |

---

## 3. 目录管理

工作目录配置持久化到 `data/directories.json`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetDirectories` | `() => Promise<Directory[]>` | 获取全部工作目录 | Directory 数组 |
| `RefreshDirectoriesGitFlag` | `() => Promise<Directory[]>` | 刷新各目录的 git 仓库/远程标记 | Directory 数组 |
| `AddDirectory` | `(name, path, isDefault) => Promise<Directory>` | 添加工作目录 | 新增 Directory |
| `UpdateDirectory` | `(id, name, path, isDefault) => Promise<Directory>` | 更新工作目录 | 更新后 Directory |
| `DeleteDirectory` | `(id) => Promise<boolean>` | 删除工作目录 | 是否成功 |
| `SetDefaultDirectory` | `(id) => Promise<boolean>` | 设为默认目录 | 是否成功 |
| `GetDefaultDirectory` | `() => Promise<Directory>` | 获取默认目录 | Directory |
| `ReorderDirectories` | `(ids) => Promise<boolean>` | 拖拽重排目录顺序 | 是否成功 |

---

## 4. 收藏夹

持久化到 `data/favorites.json`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetFavorites` | `() => Promise<Favorite[]>` | 获取全部收藏 | Favorite 数组 |
| `AddFavorite` | `(path, alias, group) => Promise<string>` | 添加收藏 | 操作结果消息 |
| `RemoveFavorite` | `(path) => Promise<string>` | 移除收藏 | 操作结果消息 |
| `UpdateFavoriteAlias` | `(path, alias) => Promise<string>` | 更新收藏别名 | 操作结果消息 |
| `UpdateFavoriteGroup` | `(path, group) => Promise<string>` | 更新收藏分组 | 操作结果消息 |

---

## 5. 文件树

懒加载 + mtime 缓存（`filetree_cache.go`）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetFileTree` | `(path) => Promise<FileTreeNode[]>` | 获取指定目录的一层子节点 | FileTreeNode 数组 |
| `GetFileTreeRecursive` | `(path, maxDepth) => Promise<FileTreeNode[]>` | 递归获取文件树（限深度） | FileTreeNode 数组 |
| `InvalidateFileTreeCache` | `(path) => Promise<void>` | 失效指定路径文件树缓存 | — |
| `ClearAllFileTreeCache` | `() => Promise<void>` | 清空全部文件树缓存 | — |
| `CreateDirectory` | `(parentPath, name) => Promise<boolean>` | 创建文件夹 | 是否成功 |
| `CreateFile` | `(parentPath, name, content) => Promise<boolean>` | 创建文件（含初始内容） | 是否成功 |
| `RenameFile` | `(oldPath, newName) => Promise<boolean>` | 重命名文件/文件夹 | 是否成功 |
| `DeleteFile` | `(path) => Promise<boolean>` | 删除文件/文件夹 | 是否成功 |

---

## 6. 文件预览与对话框

`SaveFileDialog` / `OpenFileDialog` 经后端桥接 Wails runtime（前端 runtime 不导出文件对话框 API）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `PreviewFile` | `(filePath) => Promise<FilePreview>` | 预览文件（检测文本/二进制/编码 UTF-8/GBK） | FilePreview |
| `ReadFileBytes` | `(filePath) => Promise<FileBytes>` | 读取文件原始字节（base64 编码，前端用 `decodeBase64Utf8` 还原文本） | FileBytes |
| `SaveFile` | `(filePath, content, encoding) => Promise<void>` | 保存文件（按 encoding 转回原编码写入） | — |
| `SaveFileDialog` | `(defaultFilename, filters) => Promise<string>` | 原生保存对话框 | 选定路径（取消返回空串） |
| `OpenFileDialog` | `(title, filters) => Promise<string>` | 原生打开对话框 | 选定路径（取消返回空串） |

---

## 7. 文件操作与剪贴板

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `CopyItem` | `(sourcePath, targetDir) => Promise<string>` | 复制文件/文件夹到目标目录 | 操作结果消息 |
| `MoveItem` | `(sourcePath, targetDir) => Promise<string>` | 移动文件/文件夹到目标目录 | 操作结果消息 |
| `CopyTo` | `(sourcePath, targetPath, targetName, copyWholeDir) => Promise<string>` | 复制到指定路径并改名 | 操作结果消息 |
| `CopyToSystemClipboard` | `(path) => Promise<string>` | 复制文件内容到系统剪贴板 | 操作结果消息 |
| `CutToSystemClipboard` | `(path) => Promise<string>` | 剪切文件内容到系统剪贴板 | 操作结果消息 |
| `ReadFromSystemClipboard` | `() => Promise<string>` | 从系统剪贴板读取文本 | 剪贴板文本 |

---

## 8. 搜索

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `SearchFiles` | `(rootDir, query, maxResults) => Promise<SearchResult[]>` | 文件名搜索 | SearchResult 数组 |
| `ContentSearch` | `(keyword, fileExt, subDir, searchAll) => Promise<ContentSearchGroup[]>` | 文件内容搜索（按仓库分组） | ContentSearchGroup 数组 |

---

## 9. Git 仓库信息

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetGitInfo` | `(path) => Promise<GitRepoInfo>` | 获取仓库概览（分支/远程/近期提交） | GitRepoInfo |
| `CloneRepo` | `(url, targetPath) => Promise<string>` | 克隆仓库 | 操作结果消息 |
| `PullRepo` | `(dirPath, useRebase) => Promise<string>` | 拉取（可选 rebase 模式） | 操作结果消息 |
| `ScanAndPullRepos` | `(dirPath) => Promise<PullSummary>` | 扫描目录下仓库并批量拉取 | PullSummary |
| `ExtractRepoName` | `(url) => Promise<string>` | 从 URL 提取仓库名 | 仓库名 |
| `GetGitRemoteURL` | `(path) => Promise<GitRemoteInfo>` | 获取远程 URL 与当前分支 | GitRemoteInfo |

---

## 10. 提交历史与统计

提交历史全量快照缓存（`CommitHistoryCache`，纯内存，HEAD SHA 增量 + TTL + 手动刷新）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetCommitHistory` | `(path, limit, offset, filter) => Promise<Commit[]>` | 分页获取提交历史（服务端过滤） | Commit 数组 |
| `GetRepoStats` | `(path, rangeKey) => Promise<RepoStats>` | 获取仓库统计（趋势/贡献者/热力图） | RepoStats |
| `InvalidateCommitHistoryCache` | `(path) => Promise<void>` | 失效指定仓库提交历史缓存 | — |
| `ClearAllCommitHistoryCache` | `() => Promise<void>` | 清空全部提交历史缓存 | — |
| `GetRecentCommitSubjects` | `(path, limit) => Promise<string[]>` | 取最近 limit 条 commit subject（过滤归档噪声，limit<=0 默认 3），供 AI 提交信息生成 few-shot | commit subject 数组 |

---

## 11. 本地变更与提交

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetLocalChanges` | `(path) => Promise<FileChange[]>` | 获取本地变更文件列表 | FileChange 数组 |
| `DiscardChanges` | `(path, filePaths) => Promise<void>` | 丢弃指定文件变更 | — |
| `CommitFiles` | `(path, message, filePaths) => Promise<void>` | 提交指定文件 | — |
| `PushRepo` | `(path, setUpstream) => Promise<string>` | 推送（可选设置上游） | 操作结果消息 |
| `StageFiles` | `(path, files) => Promise<void>` | 暂存文件 | — |
| `UnstageFiles` | `(path, files) => Promise<void>` | 取消暂存 | — |
| `HasUpstream` | `(path) => Promise<boolean>` | 当前分支是否有上游 | 是否有上游 |

---

## 12. Diff

返回 diff 文本（git diff 输出）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetFileDiff` | `(path, file) => Promise<string>` | 工作区文件 diff | diff 文本 |
| `GetCommitFileDiff` | `(path, sha, file) => Promise<string>` | 指定提交中某文件的 diff | diff 文本 |
| `GetRangeDiff` | `(path, baseSHA, headSHA) => Promise<string>` | 两提交区间 diff | diff 文本 |
| `GetStagedDiffText` | `(path) => Promise<string>` | 聚合暂存区全部文件 diff 为单段文本（含截断保护），供 AI 提交信息生成；暂存区为空返回 `E_GIT_NO_STAGED_CHANGES` | diff 文本 |
| `GetUncommittedDiffText` | `(path) => Promise<string>` | 聚合未提交变更 diff 为单段文本（含截断保护），供 AI 代码审查；无任何本地变更返回 `E_GIT_NO_STAGED_CHANGES` | diff 文本 |

---

## 13. 分支

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetBranches` | `(path) => Promise<BranchList>` | 获取本地/远程分支列表 | BranchList |
| `GetBranchSyncInfo` | `(path) => Promise<BranchSyncInfo>` | 当前分支与上游同步摘要（branch/ahead/behind/hasUpstream/detached/headSha）+ refs 位置映射（refs: `[{sha, kind, name}]`，kind 取 head/local/remote），供提交历史摘要条与提交行 badge；refs 不缓存每次现算 | BranchSyncInfo |
| `CheckoutBranch` | `(path, branchName, isRemote) => Promise<void>` | 切换分支（可选远程分支） | — |
| `CreateBranch` | `(path, name) => Promise<void>` | 创建分支 | — |
| `DeleteBranch` | `(path, name, force) => Promise<void>` | 删除分支（可选强制） | — |
| `RenameBranch` | `(path, oldName, newName) => Promise<void>` | 重命名分支 | — |
| `SetBranchUpstream` | `(path, branch, remote) => Promise<void>` | 设置分支上游 | — |

---

## 14. 标签

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetTags` | `(path) => Promise<GitTag[]>` | 获取标签列表 | GitTag 数组 |
| `CreateTag` | `(path, name, message) => Promise<void>` | 创建标签（带注释消息） | — |
| `DeleteTag` | `(path, name) => Promise<void>` | 删除标签 | — |
| `PushTag` | `(path, name) => Promise<string>` | 推送标签到远程 | 操作结果消息 |

---

## 15. 远程

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetRemotes` | `(path) => Promise<GitRemote[]>` | 获取远程仓库列表 | GitRemote 数组 |
| `AddRemote` | `(path, name, url) => Promise<void>` | 添加远程 | — |
| `RemoveRemote` | `(path, name) => Promise<void>` | 移除远程 | — |
| `FetchRepo` | `(path, remote, prune) => Promise<string>` | 拉取远程引用（可选 prune） | 操作结果消息 |

---

## 16. 合并/变基/挑拣

`MergeMode` 为具名 string 类型（`ff` / `no-ff` / `squash`）。冲突态经 `GetConflictState` 查询，`ConflictState.Type` 标识 `merge` / `rebase` / `cherry-pick`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `Merge` | `(path, branch, mode) => Promise<string>` | 合并分支（指定策略） | 操作结果消息 |
| `Rebase` | `(path, branch) => Promise<string>` | 变基到指定分支 | 操作结果消息 |
| `CherryPick` | `(path, sha) => Promise<string>` | 挑拣指定提交 | 操作结果消息 |
| `GetConflictState` | `(path) => Promise<ConflictState>` | 查询当前冲突态 | ConflictState |
| `ResolveConflict` | `(path, file) => Promise<void>` | 标记冲突文件已解决 | — |
| `ContinueMerge` | `(path) => Promise<string>` | 继续合并（解决冲突后） | 操作结果消息 |
| `ContinueRebase` | `(path) => Promise<string>` | 继续变基 | 操作结果消息 |
| `ContinueCherryPick` | `(path) => Promise<string>` | 继续挑拣 | 操作结果消息 |
| `AbortMerge` | `(path) => Promise<void>` | 中止合并 | — |
| `AbortRebase` | `(path) => Promise<void>` | 中止变基 | — |
| `AbortCherryPick` | `(path) => Promise<void>` | 中止挑拣 | — |
| `SkipRebase` | `(path) => Promise<string>` | 跳过当前变基提交 | 操作结果消息 |

---

## 17. Submodule

`SubmoduleUpdateMode` 为具名 string 类型（`checkout` / `merge` / `rebase` / `remote`）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetSubmodules` | `(path) => Promise<GitSubmodule[]>` | 获取 submodule 列表 | GitSubmodule 数组 |
| `InitSubmodules` | `(path) => Promise<string>` | 初始化 submodule | 操作结果消息 |
| `UpdateSubmodules` | `(path, mode, recursive, init, subPath) => Promise<string>` | 更新 submodule（指定策略/递归/初始化/子路径） | 操作结果消息 |
| `AddSubmodule` | `(path, url, subPath, branch) => Promise<string>` | 添加 submodule | 操作结果消息 |
| `RemoveSubmodule` | `(path, subPath) => Promise<void>` | 移除 submodule | — |
| `CheckoutSubmoduleBranch` | `(path, subPath, branch) => Promise<string>` | 切换 submodule 分支 | 操作结果消息 |

---

## 18. 仓库元数据

仓库筛选器列表（简述/标签/readme 摘要），持久化到 `data/repo_meta.json`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetRepoFilterList` | `(dirId) => Promise<RepoFilterItem[]>` | 获取目录下仓库筛选列表 | RepoFilterItem 数组 |
| `RefreshRepoFilterList` | `(dirId) => Promise<RepoFilterItem[]>` | 强制刷新仓库筛选列表 | RepoFilterItem 数组 |
| `SaveRepoMeta` | `(path, summary, tags) => Promise<void>` | 保存仓库简述与标签 | — |
| `CleanMissingRepoMeta` | `() => Promise<number>` | 清理已丢失仓库的元数据 | 清理条数 |
| `GetRepoReadme` | `(repoPath) => Promise<string>` | 获取仓库 README 内容 | README 文本 |

---

## 19. 仓库配置导入导出

聚合工作目录 + 收藏夹两个数据源，带 `manifestVersion` 的 JSON。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `ExportRepoConfig` | `() => Promise<string>` | 导出工作目录 + 收藏夹为 JSON | JSON 文本 |
| `PreviewRepoConfigImport` | `(jsonText) => Promise<RepoConfigImportPreview>` | 预览导入（新增/冲突/非法，不落盘） | RepoConfigImportPreview |
| `ApplyRepoConfigImport` | `(jsonText, decisions) => Promise<RepoConfigImportResult>` | 按决策应用导入并落盘 | RepoConfigImportResult |

---

## 20. 状态看板

全局状态看板（活动栏一级入口）：手动 pin 关注的核心仓库跨仓状态总览，pin 列表持久化到 `data/dashboard_pinned.json`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetDashboardPinned` | `() => Promise<string[]>` | 获取 pin 仓库路径列表 | 路径数组 |
| `AddDashboardPin` | `(path) => Promise<void>` | 加入看板（路径规范化去重） | — |
| `RemoveDashboardPin` | `(path) => Promise<void>` | 从看板移除 | — |
| `IsDashboardPinned` | `(path) => Promise<boolean>` | 查询仓库是否已 pin | 是否已 pin |
| `GetDashboardStatuses` | `() => Promise<RepoStatus[]>` | 计算 pin 仓库状态（分支/工作区/ahead/behind/上游） | RepoStatus 数组 |
| `RefreshDashboardStatuses` | `() => Promise<RepoStatus[]>` | 强制刷新 pin 仓库状态 | RepoStatus 数组 |

---

## 21. AI 功能

skill 聚合触发 AI 任务，并发控制 + 历史归档。AI 任务异步事件流经 Wails `runtime.EventsEmit` 推送（`ai-task:queued/started/output/done`）。持久化到 `data/ai_functions.json`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetAiFunctions` | `() => Promise<AiFunction[]>` | 获取 AI 功能配置列表 | AiFunction 数组 |
| `SaveAiFunctions` | `(funcs) => Promise<void>` | 保存 AI 功能配置 | — |
| `ExportAiFunctions` | `() => Promise<string>` | 导出 AI 功能为 JSON | JSON 文本 |
| `ImportAiFunctions` | `(jsonText) => Promise<ImportPreview>` | 导入 AI 功能（预览新增/冲突/非法） | ImportPreview |
| `GetDiscoveredSkills` | `() => Promise<SkillDescriptor[]>` | 获取自动发现的 skill 列表 | SkillDescriptor 数组 |
| `RefreshDiscoveredSkills` | `() => Promise<SkillDescriptor[]>` | 刷新 skill 发现 | SkillDescriptor 数组 |
| `RunAiFunction` | `(functionID, params) => Promise<string>` | 运行 AI 功能（返回 taskID） | taskID |
| `RunAiFollowUp` | `(taskID, followUpID, params) => Promise<string>` | 运行追问（返回新 taskID） | taskID |
| `CancelAiTask` | `(taskID) => Promise<boolean>` | 取消 AI 任务 | 是否成功 |
| `GetAiTaskState` | `(taskID) => Promise<AiTaskState>` | 获取任务实时状态 | AiTaskState |
| `GetAiConcurrencyStatus` | `() => Promise<AiConcurrencyStatus>` | 获取并发状态（运行/排队/上限） | AiConcurrencyStatus |
| `RemoveAiTask` | `(taskID) => Promise<boolean>` | 移除任务（不中止运行） | 是否成功 |
| `GetAiTaskOutput` | `(taskID) => Promise<string>` | 获取任务输出 | 输出文本 |
| `GetAiTaskHistory` | `(filter) => Promise<AiTaskHistory[]>` | 查询任务历史 | AiTaskHistory 数组 |
| `GetAiTaskHistoryOutput` | `(id) => Promise<string>` | 获取历史任务输出 | 输出文本 |
| `DeleteAiTaskHistory` | `(id) => Promise<boolean>` | 删除单条历史 | 是否成功 |
| `ClearAiTaskHistory` | `(criteria) => Promise<number>` | 按条件清理历史（按天/保留数/功能 ID） | 清理条数 |
| `GetAiTaskHistoryStats` | `(filter) => Promise<AiTaskHistoryStats>` | 获取历史统计 | AiTaskHistoryStats |
| `GetFunctionUsageCounts` | `() => Promise<Record<string, number>>` | 获取各功能使用次数 | 功能 ID -> 次数 |
| `ExportAiTaskHistoryCSV` | `(filter) => Promise<string>` | 导出历史为 CSV | CSV 文本 |
| `ExportAiTaskHistoryMarkdown` | `(filter) => Promise<string>` | 导出历史为 Markdown | Markdown 文本 |

---

## 22. AI 对话

AI 对话工作台（活动栏「AI 对话」面板）：多会话持续式对话。会话/消息/目录/模板/配置独立持久化到 `data/ai_chat/`。对话任务异步事件流经 EventSink 推送（`chat-task:queued/started/output/done`，payload 形状对齐 `model.ChatTaskRunResult`）；`RunChat` 自动在会话已有 claude session id 时追加 `--resume` 续上下文。

### 会话 CRUD

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `CreateChatSession` | `(directoryID, title, cwd) => Promise<ChatSession>` | 创建会话（title 空 = 默认「新会话」，cwd 为 claude 子进程工作目录） | 新建 ChatSession |
| `ListChatSessions` | `(directoryID) => Promise<ChatSession[]>` | 列出目录的会话（按最近活跃降序，不含消息；directoryID 空 = 全部目录） | ChatSession 数组 |
| `GetChatSession` | `(sessionID) => Promise<ChatSession>` | 读取会话完整内容（元数据 + 消息按时间升序；不存在返回 `E_CHAT_SESSION_NOT_FOUND`） | ChatSession |
| `DeleteChatSession` | `(sessionID) => Promise<void>` | 删除会话（索引项 + 消息文件） | — |
| `UpdateChatSessionTitle` | `(sessionID, title) => Promise<void>` | 修改会话标题（并刷新活跃时间，列表排序前移） | — |

### 目录 CRUD

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `AddChatDirectory` | `(path, displayName) => Promise<ChatDirectory>` | 添加侧栏常用目录项（displayName 空 = 取目录名；重复路径幂等返回既有项） | ChatDirectory |
| `ListChatDirectories` | `() => Promise<ChatDirectory[]>` | 列出侧栏常用目录项（按 SortOrder 升序） | ChatDirectory 数组 |
| `UpdateChatDirectory` | `(id, displayName) => Promise<void>` | 修改目录项显示名 | — |
| `RemoveChatDirectory` | `(id) => Promise<void>` | 移除目录项（不影响已产生的会话与消息数据） | — |
| `ReorderChatDirectories` | `(ids) => Promise<void>` | 按 ids 顺序持久化目录项排序（侧栏拖拽重排） | — |

### 对话执行与任务状态

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `RunChat` | `(chatSessionID, prompt, permissionMode, modelName) => Promise<string>` | 执行一轮对话（permissionMode 仅非 default 时传 claude，modelName 空 = claude 默认），返回任务 id；prompt 为空拒绝（`E_CHAT_EMPTY_PROMPT`），同会话已有进行中/排队任务拒绝（`E_CHAT_IN_PROGRESS`） | taskID |
| `GetChatTaskState` | `(taskID) => Promise<ChatTaskState>` | 查询对话任务状态（回复累积文本/claude 会话 id/运行态，切页恢复用；不存在返回 null） | ChatTaskState |
| `CancelChatTask` | `(taskID) => Promise<boolean>` | 取消运行中/排队中的对话任务（杀 claude 及其子进程树） | 是否成功 |

### 模板 CRUD

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `AddChatTemplate` | `(scope, directoryID, name, content) => Promise<ChatTemplate>` | 新增模板（scope 取 `global`/`directory`：global 忽略 directoryID，directory 时必填归属目录项） | ChatTemplate |
| `ListChatTemplates` | `(directoryID) => Promise<ChatTemplate[]>` | 「该目录的目录模板 + 全局模板」合并列表（directoryID 空 = 仅全局） | ChatTemplate 数组 |
| `UpdateChatTemplate` | `(id, name, content) => Promise<void>` | 修改模板名与内容（归属域不支持修改，删旧建新） | — |
| `RemoveChatTemplate` | `(id) => Promise<void>` | 删除模板 | — |

### 执行配置

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetChatSettings` | `() => Promise<ChatSettings>` | 读对话执行配置（文件缺失/损坏回默认值，不报错） | ChatSettings |
| `SaveChatSettings` | `(permissionMode, modelName) => Promise<void>` | 持久化对话执行配置（permissionMode 须在可选集内） | — |

---

## 23. 会话快照

崩溃恢复 UI 状态快照，持久化到 `data/session.json`（v1.4 PR2）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetSessionState` | `() => Promise<SessionState>` | 读取上次会话 UI 状态快照（损坏降级空快照） | SessionState |
| `SaveSessionState` | `(state) => Promise<void>` | 持久化当前 UI 状态快照 | — |

---

## 24. 终端

pty 终端会话（Windows 用 conpty）。终端不真实复用进程，崩溃恢复按快照循环重建全部 tab（每 tab 一次 `CreateTerminal`）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `CreateTerminal` | `(dir, shellType, cols, rows) => Promise<string>` | 创建终端会话 | sessionID |
| `WriteTerminalInput` | `(sessionID, input) => Promise<void>` | 写入终端输入 | — |
| `ChangeTerminalDir` | `(sessionID, dir) => Promise<void>` | 切换终端工作目录 | — |
| `ResizeTerminal` | `(sessionID, cols, rows) => Promise<void>` | 调整终端尺寸 | — |
| `CloseTerminal` | `(sessionID) => Promise<void>` | 关闭终端会话 | — |
| `GetShellConfigs` | `() => Promise<ShellConfig[]>` | 获取可用 shell 配置列表 | ShellConfig 数组 |

---

## 25. 外部集成

`OpenInExternalDiff` 启动外部 diff 工具（配置见设置面板，未配置返回 `E_DIFF_TOOL_NOT_CONFIGURED`）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `OpenInExplorer` | `(path) => Promise<boolean>` | 在资源管理器中打开 | 是否成功 |
| `OpenInVSCode` | `(path) => Promise<boolean>` | 用 VSCode 打开 | 是否成功 |
| `OpenInWarp` | `(path) => Promise<boolean>` | 用 Warp 打开 | 是否成功 |
| `OpenInObsidian` | `(path) => Promise<string>` | 用 Obsidian 打开（以该目录为仓库） | 操作结果消息 |
| `OpenObsidianVaultManager` | `() => Promise<boolean>` | 打开 Obsidian 仓库管理器 | 是否成功 |
| `CopyObsidianVaultPath` | `(path) => Promise<boolean>` | 复制 Obsidian 仓库路径 | 是否成功 |
| `AutoRegisterAndOpen` | `(path) => Promise<string>` | 自动注册 Obsidian 仓库并打开 | 操作结果消息 |
| `OpenWithDefaultApp` | `(path) => Promise<boolean>` | 用系统默认应用打开 | 是否成功 |
| `OpenInExternalDiff` | `(path, mode, file, sha, baseSha, headSha) => Promise<void>` | 启动外部 diff 工具 | — |

---

## 26. 设置

持久化到 `data/settings.json`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetSettings` | `() => Promise<AppSettings>` | 获取应用设置 | AppSettings |
| `SaveSettings` | `(settings) => Promise<void>` | 保存应用设置 | — |

---

## 27. 网络访问

浏览器访问通道（桌面同开 HTTP / `--serve` 无头模式共用），配置段持久化到 `data/settings.json` 的 `webServe`，令牌持久化到 `data/web_token`（0600）。设置页「网络访问」分区消费这组方法。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetWebServeConfig` | `() => Promise<WebServeConfig>` | 获取浏览器访问通道配置与运行状态（含候选访问地址） | WebServeConfig |
| `SetWebServeConfig` | `(enabled, bindAddress) => Promise<void>` | 保存配置并应用（关闭停机 / 改址平滑重启 / 改绑非回环须前端先风险确认） | — |
| `GetWebServeToken` | `() => Promise<string>` | 读取当前访问令牌（设置页遮蔽展示） | 令牌字符串 |
| `RegenerateWebToken` | `() => Promise<string>` | 重新生成令牌并热轮换（旧令牌立即失效，不断监听与在途连接） | 新令牌字符串 |

---

## 28. 更新

检查更新与自动更新，pending 机制（批处理脚本替换 exe 后重启）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `CheckForUpdate` | `() => Promise<UpdateInfo>` | 检查是否有新版本 | UpdateInfo |
| `DownloadUpdate` | `(downloadUrl) => Promise<void>` | 下载更新包 | — |
| `CancelDownload` | `() => Promise<void>` | 取消下载 | — |
| `ApplyUpdate` | `() => Promise<void>` | 应用更新（替换 exe 并重启） | — |

---

## 29. 数据模型

全部提取自 `frontend/wailsjs/go/models.ts`（`model` namespace；`frontend` namespace 含 `FileFilter`）。字段名按 Go json tag 映射，`omitempty` 对应 TS `?:` 可选。以下列出关键 struct，完整定义见 `models.ts`。

### 29.1 目录与文件

**Directory** — 工作目录

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 目录 ID |
| `name` | string | 显示名称 |
| `path` | string | 绝对路径 |
| `isDefault` | boolean | 是否默认目录 |
| `createTime` | time | 创建时间 |
| `isGitRepo` | boolean | 是否 git 仓库 |
| `hasRemote` | boolean | 是否配置远程 |

**FileTreeNode** — 文件树节点

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 节点 ID |
| `name` / `path` / `type` | string | 名称 / 路径 / 类型 |
| `isGitRepo` / `hasRemote` | boolean | git 仓库 / 远程标记 |
| `hasChildren` | boolean | 是否有子节点（懒加载） |
| `children` | FileTreeNode[]? | 子节点 |
| `isLeaf` | boolean | 是否叶子节点 |

**FilePreview** — 文件预览结果

| 字段 | 类型 | 说明 |
|---|---|---|
| `path` / `name` | string | 路径 / 名称 |
| `size` | number | 文件大小 |
| `content` | string? | 文本内容（转码后 UTF-8） |
| `isBinary` / `tooLarge` | boolean | 是否二进制 / 超大（>1MB） |
| `error` | string? | 错误信息 |
| `kind` | string? | 类型（text/image/pdf/office/unsupported） |
| `encoding` | string? | 来源编码（utf-8/gbk/空） |

**FileBytes** — 文件字节（`ReadFileBytes`）

| 字段 | 类型 | 说明 |
|---|---|---|
| `path` / `name` / `size` | string/number | 路径 / 名称 / 大小 |
| `kind` | string? | 类型 |
| `base64` | string? | 原始字节 base64（前端用 `decodeBase64Utf8` 还原文本） |
| `tooLarge` | boolean | 是否超大 |
| `error` | string? | 错误信息 |

**FileChange** — 本地变更文件

| 字段 | 类型 | 说明 |
|---|---|---|
| `path` | string | 相对路径 |
| `status` | string | 变更状态：M/A/D/R/? |
| `staged` | boolean | 是否已暂存 |

### 29.2 Git

**GitRepoInfo** — 仓库概览

| 字段 | 类型 | 说明 |
|---|---|---|
| `path` / `branch` / `remote` / `remoteUrl` | string | 路径 / 分支 / 远程名 / 远程 URL |
| `commits` | GitCommit[] | 近期提交（hash/author/date/message） |
| `isRepo` | boolean | 是否仓库 |

**Commit** — 提交记录

| 字段 | 类型 | 说明 |
|---|---|---|
| `sha` / `shortSha` | string | 完整 SHA / 前 8 位 |
| `message` / `author` / `email` | string | 消息 / 作者 / 邮箱 |
| `timestamp` | number | Unix 时间戳 |
| `dateTime` | string | 格式化时间 |
| `files` | string[] | 变更文件路径 |

**CommitFilter** — 提交历史过滤（各字段 AND 组合，空字段不参与）

| 字段 | 类型 | 说明 |
|---|---|---|
| `author` / `keyword` | string? | 作者子串 / 消息子串 |
| `since` / `until` | string? | 起止日期 YYYY-MM-DD |
| `filePath` | string? | 文件路径子串 |

**BranchList** / **BranchInfo** — 分支列表（`branches: BranchInfo[]`，含 `name` / `isRemote` / `isCurrent`）

**GitTag** — 标签（`name` / `type` / `sha` / `shortSha` / `message` / `tagger` / `date`）

**GitRemote** — 远程（`name` / `url`）

**GitRemoteInfo** — 远程信息（`remoteUrl` / `branch` / `isDetached`）

**ConflictState** — 冲突态（`type`: none/merge/rebase/cherry-pick，`files`: string[]）

**GitSubmodule** — submodule（`path` / `sha` / `shortSha` / `describe` / `branch` / `url` / `initialized` / `shaMismatch` / `dirty` / `conflict` / `detached`）

**RepoStats** — 仓库统计（`trend`: TimeBucket[]，`contributors`: Contributor[]，`heatmap`: DayCount[]，`totalCommits` / `dateRange` / `granularity` / `sampled`）

### 29.3 会话快照

**SessionState** — 崩溃恢复 UI 状态快照

| 字段 | 类型 | 说明 |
|---|---|---|
| `selectedDirectoryId` | string? | 当前工作目录 ID |
| `activePanel` | string? | 活动面板：directory/ai/stats/toolbox |
| `terminal` | TerminalSnapshot? | 终端面板快照 |
| `version` | string? | 快照格式版本 |
| `savedAt` | number? | 保存时间戳 |

**TerminalSnapshot**（`visible` / `height` / `workDir`（旧版单终端兼容读取源，新快照不写入）/ `tabs`（v2 多终端 tab 列表 `TerminalTabSnapshot[]`：`workDir` / `shellType`）/ `activeIndex` / `fullscreen`）

### 29.4 设置与更新

**AppSettings** — 应用设置（`gpuDisabled` / `defaultShell` / `gitBashPath` / `wslDistro` / `terminalFontSize` / `terminalFontFamily` / `terminalScrollback` / `searchExcludeDirs` / `searchExcludeFiles` / `shortcutCommandPalette` / `shortcutToggleTerminal` / `shortcutRename` / `shortcutDelete` / `obsidianPath` / `themeMode` / `diffToolName` / `diffToolPath` / `diffToolArgs` / `webServe`）。终端外观三字段加载时经 `EnsureTerminalDefaults` 补默认值并收敛越界值（字号 10-24 默认 14、回滚 1000-10000 默认 1000，字体空串走默认 Cascadia Code 栈）

**WebServeSettings** — 浏览器访问通道配置（`AppSettings.webServe` 段；指针形态区分「段缺失」与「显式关闭」，加载时经 `EnsureWebServeDefaults` 补默认值）

| 字段 | 类型 | 说明 |
|---|---|---|
| `enabled` | boolean | 桌面模式是否同开 HTTP 服务（默认 true） |
| `bindAddress` | string | HTTP 监听地址 host:port（默认 `127.0.0.1:36115`） |

**WebServeConfig** — 浏览器访问通道配置与运行状态（`GetWebServeConfig` 返回，持久化配置之上附带派生信息）

| 字段 | 类型 | 说明 |
|---|---|---|
| `enabled` | boolean | 是否启用（持久化配置） |
| `bindAddress` | string | 监听地址 host:port（持久化配置） |
| `running` | boolean | HTTP 服务当前是否运行中 |
| `accessUrls` | string[] | 候选访问地址（含回环与本机局域网 IPv4，同网段设备可达） |

**UpdateInfo** — 更新信息（`hasUpdate` / `currentVer` / `latestVer` / `downloadUrl` / `releaseNotes` / `publishedAt` / `fileSize`）

### 29.5 AI

**AiFunction** — AI 功能配置（`id` / `name` / `description` / `icon` / `command` / `cwd` / `addDirs` / `env` / `mcp` / `permissionMode` / `timeoutMinutes` / `completion` / `params` / `followUps` / `tags` / `pinned`）

**AiTaskState** — 任务实时状态（`taskId` / `functionId` / `running` / `queued` / `sessionId` / `prompt` / `output` / `outputSize` / `outputFile` / `tableExtracted` / `error` / `startedAt` / `metrics`）

**AiTaskHistory** — 任务历史（`id` / `functionId` / `name` / `prompt` / `startedAt` / `finishedAt` / `status` / `exitCode` / `error` / `sessionId` / `metrics` / `outputFile` / `outputSize`）

**AiTaskHistoryFilter**（`functionId` / `status` / `from` / `to`）

**AiTaskHistoryClearCriteria**（`olderThanDays` / `keepRecent` / `functionId`）

**AiConcurrencyStatus**（`running` / `queued` / `max`）

**AiTaskHistoryStats**（`totalCount` / `successCount` / `totalCostUsd` / `totalInputTokens` / `totalOutputTokens` / `totalCacheReadTokens` / `totalCacheCreationTokens` / `totalDurationMs` / `byFunction`: FunctionStat[]）

**ImportPreview** — AI 功能导入预览（`new` / `conflict`: AiFunction[]，`invalid`: string[]）

### 29.6 AI 对话

**ChatSession** — 对话会话（元数据入 `data/ai_chat/sessions.json` 索引；消息逐会话存 `data/ai_chat/messages/<sessionId>.json`，`GetChatSession` 加载时填充）

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 会话 ID（`chatsession-<unixnano>`） |
| `directoryId` | string | 所属侧栏目录项 ID |
| `title` | string | 会话标题（默认「新会话」，用户可改） |
| `cwd` | string | claude 子进程工作目录 |
| `claudeSessionId` | string? | claude CLI 会话 id，空 = 尚未产生（首轮对话），下一轮 `--resume` 用 |
| `createdAt` / `updatedAt` | number | 创建 / 最近活跃时间（unix 毫秒，列表按 updatedAt 降序） |
| `messages` | ChatMessage[]? | 会话消息（索引文件不落此字段） |

**ChatMessage** — 单条消息

| 字段 | 类型 | 说明 |
|---|---|---|
| `role` | string | `user` / `assistant` |
| `content` | string | 消息文本（markdown） |
| `timestamp` | number | unix 毫秒 |
| `taskId` | string? | 产生本条消息的对话任务 id（历史消息无任务关联时为空） |

**ChatDirectory** — 侧栏常用目录项（持久化 `data/ai_chat/directories.json`）

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 目录项 ID（`chatdir-<unixnano>`） |
| `path` | string | 目录绝对路径（规范化，claude 子进程 cwd） |
| `displayName` | string | 侧栏显示名（空 = 取目录名） |
| `sortOrder` | number | 侧栏排序（拖拽重排后按列表序重写） |
| `createdAt` | number | 创建时间 unix 毫秒 |

**ChatTemplate** — 输入框纯文本模板片段（持久化 `data/ai_chat/templates.json`）

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 模板 ID（`chattpl-<unixnano>`） |
| `scope` | string | `global`（跨目录共享）/ `directory`（归属侧栏目录项） |
| `directoryId` | string? | scope=directory 时的归属目录项 id，global 恒空 |
| `name` / `content` | string | 模板名（下拉展示）/ 模板正文（纯文本） |
| `createdAt` / `updatedAt` | number | 创建 / 修改时间 unix 毫秒 |

**ChatSettings** — 执行配置（持久化 `data/ai_chat/settings.json`；`permissionMode` 取 `default`/`acceptEdits`/`plan`/`bypassPermissions`，`modelName` 空 = claude 默认模型）

**ChatTaskState** — 对话任务当前状态（`GetChatTaskState` 拉取，切页恢复/轮询兜底；`taskId` / `chatSessionId` / `running` / `queued` / `claudeSessionId` / `reply`（本轮回复累积文本）/ `error` / `startedAt`）

**ChatTaskRunResult** — 单轮对话执行结果（`chat-task:done` 事件 payload；`taskId` / `chatSessionId` / `claudeSessionId` / `reply`（assistant 回复全文）/ `exitCode` / `error` / `canceled`）

### 29.7 其他

**Favorite**（`path` / `alias` / `group` / `createdAt`）

**RepoFilterItem**（`name` / `path` / `summary` / `tags` / `readmeSummary` / `missing` / `hasRemote` / `isGitRepo`）

**RepoStatus** — 状态看板仓库状态（`path` / `name` / `branch` / `dirty` / `ahead` / `behind` / `hasUpstream` / `detached` / `isRepo` / `missing` / `error`）

**RepoConfigImportPreview**（`newDirectories` / `conflictDirectories`: RepoConfigDirectoryPreview[]，`newFavorites` / `conflictFavorites`: RepoConfigFavoritePreview[]，`invalid`: RepoConfigInvalidItem[]）

**RepoConfigImportDecisions**（`directories` / `favorites`: Record<string, string>，决策值 skip/overwrite/saveAsNew）

**RepoConfigImportResult**（`added` / `overwritten` / `skipped` / `failed` / `failedReasons`）

**PullSummary**（`total`）

**SearchResult**（`name` / `path` / `type`）

**ContentSearchGroup**（`repoName` / `repoPath` / `items`: ContentSearchResult[]）

**ShellConfig**（`type` / `executable` / `args` / `displayName`）

**SkillDescriptor**（`name` / `description` / `command` / `cwd` / `source` / `sourceDir` / `plugin`）

### 29.8 具名 string 类型

`model/commit.go` 定义的两个具名 string 类型，作方法参数时 `App.d.ts` 保留 `model.X` 引用：

| 类型 | 取值 | 用于 |
|---|---|---|
| `MergeMode` | `ff` / `no-ff` / `squash` | `Merge(path, branch, mode)` |
| `SubmoduleUpdateMode` | `checkout` / `merge` / `rebase` / `remote` | `UpdateSubmodules(path, mode, ...)` |

`models.ts` 不为具名 string 类型生成 `export type X = string` 别名；当前前端纯 JS 不 import `models.ts`，运行时打包不触发 `MISSING_EXPORT`。若未来前端显式 import 并运行时引用，须手动补别名，见 [spec/cross-layer-contracts.md](spec/cross-layer-contracts.md)。

---

## 30. 深度查询入口

本清单为静态快照。需要查询方法调用链、service 实现、字段影响面等动态结构信息时，使用项目内置的 CodeGraph 索引（`.codegraph/`，SQLite 知识图谱，全仓库符号/调用边/文件索引，规模以 `codegraph status` 实时查询为准）：

| 查询意图 | 工具 |
|---|---|
| 某方法被哪些前端组件调用 | `codegraph_callers` |
| 某 service 方法的实现源码 | `codegraph_node` / `codegraph_explore` |
| 某 struct 字段变更的影响面 | `codegraph_impact` |
| 某前端操作到后端的完整调用路径 | `codegraph_trace` |
| 按符号名定位定义 | `codegraph_search` |

索引健康检查：`codegraph status`（CLI）。索引滞后约 500ms，编辑文件后勿立即查询。

---

**最后更新：** 2026-09-22
