# WorkBench API 参考

> Wails 绑定方法清单与数据模型。全部方法提取自 `frontend/wailsjs/go/main/App.d.ts`（`wails generate module` 自动生成），按域分组；数据模型提取自 `frontend/wailsjs/go/models.ts`。
> 前端调用：`import { Method } from 'wailsjs/go/main/App'` 后 `await Method(args)`，运行时经 `window['go']['main']['App']['Method']` 调 Go 后端。
> 配套文档：[架构设计.md](架构设计.md) · 跨层契约见 [spec/cross-layer-contracts.md](spec/cross-layer-contracts.md)

---

## 1. 方法清单总览

共 **139** 个导出方法，分布在 15 个 `app_*.go` 域文件 + `app.go`，委托 `AppServices` 持有的 16 个 service/cache。按 24 个业务域分组：

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
| 提交历史与统计 | 4 | `app_git.go` |
| 本地变更与提交 | 7 | `app_git.go` |
| Diff | 3 | `app_git.go` |
| 分支 | 6 | `app_git.go` |
| 标签 | 4 | `app_git.go` |
| 远程 | 4 | `app_git.go` |
| 合并/变基/挑拣 | 12 | `app_git.go` |
| Submodule | 6 | `app_git.go` |
| 仓库元数据 | 5 | `app_repometa.go` |
| 仓库配置导入导出 | 3 | `app_repo_config.go` |
| AI 功能 | 21 | `app_ai.go` |
| 会话快照 | 2 | `app_session.go` |
| 终端 | 6 | `app_terminal.go` |
| 外部集成 | 9 | `app_external.go` |
| 设置 | 2 | `app_settings.go` |
| 更新 | 4 | `app_update.go` |

> 签名约定：`arg1/arg2/...` 为位置参数（Wails 绑定不保留参数名），类型取自 `App.d.ts`。返回值均为 `Promise`，错误经 Wails `ErrorFormatter` 转为 `{code, message}` 或 `{message}` 对象（见 [架构设计.md](架构设计.md) 第 5 节）。

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

---

## 13. 分支

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetBranches` | `(path) => Promise<BranchList>` | 获取本地/远程分支列表 | BranchList |
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

## 20. AI 功能

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

## 21. 会话快照

崩溃恢复 UI 状态快照，持久化到 `data/session.json`（v1.4 PR2）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetSessionState` | `() => Promise<SessionState>` | 读取上次会话 UI 状态快照（损坏降级空快照） | SessionState |
| `SaveSessionState` | `(state) => Promise<void>` | 持久化当前 UI 状态快照 | — |

---

## 22. 终端

pty 终端会话（Windows 用 conpty）。终端不真实复用进程，崩溃恢复仅恢复工作目录。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `CreateTerminal` | `(dir, shellType, cols, rows) => Promise<string>` | 创建终端会话 | sessionID |
| `WriteTerminalInput` | `(sessionID, input) => Promise<void>` | 写入终端输入 | — |
| `ChangeTerminalDir` | `(sessionID, dir) => Promise<void>` | 切换终端工作目录 | — |
| `ResizeTerminal` | `(sessionID, cols, rows) => Promise<void>` | 调整终端尺寸 | — |
| `CloseTerminal` | `(sessionID) => Promise<void>` | 关闭终端会话 | — |
| `GetShellConfigs` | `() => Promise<ShellConfig[]>` | 获取可用 shell 配置列表 | ShellConfig 数组 |

---

## 23. 外部集成

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

## 24. 设置

持久化到 `data/settings.json`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetSettings` | `() => Promise<AppSettings>` | 获取应用设置 | AppSettings |
| `SaveSettings` | `(settings) => Promise<void>` | 保存应用设置 | — |

---

## 25. 更新

检查更新与自动更新，pending 机制（批处理脚本替换 exe 后重启）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `CheckForUpdate` | `() => Promise<UpdateInfo>` | 检查是否有新版本 | UpdateInfo |
| `DownloadUpdate` | `(downloadUrl) => Promise<void>` | 下载更新包 | — |
| `CancelDownload` | `() => Promise<void>` | 取消下载 | — |
| `ApplyUpdate` | `() => Promise<void>` | 应用更新（替换 exe 并重启） | — |

---

## 26. 数据模型

全部提取自 `frontend/wailsjs/go/models.ts`（`model` namespace，54 个 class；`frontend` namespace 含 `FileFilter`）。字段名按 Go json tag 映射，`omitempty` 对应 TS `?:` 可选。以下列出关键 struct，完整定义见 `models.ts`。

### 26.1 目录与文件

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

### 26.2 Git

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

### 26.3 会话快照

**SessionState** — 崩溃恢复 UI 状态快照

| 字段 | 类型 | 说明 |
|---|---|---|
| `selectedDirectoryId` | string? | 当前工作目录 ID |
| `activePanel` | string? | 活动面板：directory/ai/stats/toolbox |
| `terminal` | TerminalSnapshot? | 终端面板快照 |
| `version` | string? | 快照格式版本 |
| `savedAt` | number? | 保存时间戳 |

**TerminalSnapshot**（`visible` / `height` / `workDir`）

### 26.4 设置与更新

**AppSettings** — 应用设置（`gpuDisabled` / `defaultShell` / `gitBashPath` / `wslDistro` / `searchExcludeDirs` / `searchExcludeFiles` / `shortcutCommandPalette` / `shortcutToggleTerminal` / `shortcutRename` / `shortcutDelete` / `obsidianPath` / `themeMode` / `diffToolName` / `diffToolPath` / `diffToolArgs`）

**UpdateInfo** — 更新信息（`hasUpdate` / `currentVer` / `latestVer` / `downloadUrl` / `releaseNotes` / `publishedAt` / `fileSize`）

### 26.5 AI

**AiFunction** — AI 功能配置（`id` / `name` / `description` / `icon` / `command` / `cwd` / `addDirs` / `env` / `mcp` / `permissionMode` / `timeoutMinutes` / `completion` / `params` / `followUps` / `tags` / `pinned`）

**AiTaskState** — 任务实时状态（`taskId` / `functionId` / `running` / `queued` / `sessionId` / `prompt` / `output` / `outputSize` / `outputFile` / `tableExtracted` / `error` / `startedAt` / `metrics`）

**AiTaskHistory** — 任务历史（`id` / `functionId` / `name` / `prompt` / `startedAt` / `finishedAt` / `status` / `exitCode` / `error` / `sessionId` / `metrics` / `outputFile` / `outputSize`）

**AiTaskHistoryFilter**（`functionId` / `status` / `from` / `to`）

**AiTaskHistoryClearCriteria**（`olderThanDays` / `keepRecent` / `functionId`）

**AiConcurrencyStatus**（`running` / `queued` / `max`）

**AiTaskHistoryStats**（`totalCount` / `successCount` / `totalCostUsd` / `totalInputTokens` / `totalOutputTokens` / `totalCacheReadTokens` / `totalCacheCreationTokens` / `totalDurationMs` / `byFunction`: FunctionStat[]）

**ImportPreview** — AI 功能导入预览（`new` / `conflict`: AiFunction[]，`invalid`: string[]）

### 26.6 其他

**Favorite**（`path` / `alias` / `group` / `createdAt`）

**RepoFilterItem**（`name` / `path` / `summary` / `tags` / `readmeSummary` / `missing` / `hasRemote` / `isGitRepo`）

**RepoConfigImportPreview**（`newDirectories` / `conflictDirectories`: RepoConfigDirectoryPreview[]，`newFavorites` / `conflictFavorites`: RepoConfigFavoritePreview[]，`invalid`: RepoConfigInvalidItem[]）

**RepoConfigImportDecisions**（`directories` / `favorites`: Record<string, string>，决策值 skip/overwrite/saveAsNew）

**RepoConfigImportResult**（`added` / `overwritten` / `skipped` / `failed` / `failedReasons`）

**PullSummary**（`total`）

**SearchResult**（`name` / `path` / `type`）

**ContentSearchGroup**（`repoName` / `repoPath` / `items`: ContentSearchResult[]）

**ShellConfig**（`type` / `executable` / `args` / `displayName`）

**SkillDescriptor**（`name` / `description` / `command` / `cwd` / `source` / `sourceDir` / `plugin`）

### 26.7 具名 string 类型

`model/commit.go` 定义的两个具名 string 类型，作方法参数时 `App.d.ts` 保留 `model.X` 引用：

| 类型 | 取值 | 用于 |
|---|---|---|
| `MergeMode` | `ff` / `no-ff` / `squash` | `Merge(path, branch, mode)` |
| `SubmoduleUpdateMode` | `checkout` / `merge` / `rebase` / `remote` | `UpdateSubmodules(path, mode, ...)` |

`models.ts` 不为具名 string 类型生成 `export type X = string` 别名；当前前端纯 JS 不 import `models.ts`，运行时打包不触发 `MISSING_EXPORT`。若未来前端显式 import 并运行时引用，须手动补别名，见 [spec/cross-layer-contracts.md](spec/cross-layer-contracts.md)。

---

## 27. 深度查询入口

本清单为静态快照。需要查询方法调用链、service 实现、字段影响面等动态结构信息时，使用项目内置的 CodeGraph 索引（`.codegraph/`，SQLite 知识图谱，307 文件 / 6151 节点 / 11282 边）：

| 查询意图 | 工具 |
|---|---|
| 某方法被哪些前端组件调用 | `codegraph_callers` |
| 某 service 方法的实现源码 | `codegraph_node` / `codegraph_explore` |
| 某 struct 字段变更的影响面 | `codegraph_impact` |
| 某前端操作到后端的完整调用路径 | `codegraph_trace` |
| 按符号名定位定义 | `codegraph_search` |

索引健康检查：`codegraph status`（CLI）。索引滞后约 500ms，编辑文件后勿立即查询。

---

**最后更新：** 2026-09-14
