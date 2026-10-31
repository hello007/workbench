# WorkBench API 参考

> Wails 绑定方法清单与数据模型。全部方法提取自 `frontend/wailsjs/go/main/App.d.ts`（`wails generate module` 自动生成），按域分组；数据模型提取自 `frontend/wailsjs/go/models.ts`。
> 前端调用：`import { Method } from 'wailsjs/go/main/App'` 后 `await Method(args)`，运行时经 `window['go']['main']['App']['Method']` 调 Go 后端。
> 配套文档：[架构设计.md](架构设计.md) · 跨层契约见 [spec/cross-layer-contracts.md](spec/cross-layer-contracts.md)

---

## 1. 方法清单总览

共 **230** 个导出方法（以 `App.d.ts` 实时提取为准），分布在 25 个 `app_*.go` 域文件 + `app.go`，委托 `AppServices` 持有的 25 个 service/cache。按 29 个业务域分组：

| 域 | 方法数 | 实现文件 |
|---|---|---|
| 应用信息 | 1 | `app.go` |
| 目录管理 | 8 | `app_directory.go` |
| 收藏夹 | 5 | `app_favorites.go` |
| 文件树 | 8 | `app_filetree.go` |
| 文件预览与对话框 | 6 | `app_preview.go` |
| PPTX 预览转换 | 1 | `app_pptx_preview.go` |
| 文件操作与剪贴板 | 6 | `app_clipboard.go` |
| 搜索 | 2 | `app_search.go` |
| Git 仓库信息 | 8 | `app_git.go` |
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
| 网络访问 | 5 | `app_webserve.go` |
| 更新 | 4 | `app_update.go` |
| 素材管理 | 43 | `app_assets.go` |
| 素材购物栏 | 7 | `app_asset_cart.go` |

> 既有缺口（与本表合计 220 差 11）：进程管理/端口占用（`app_process.go` 3 方法）、环境变量（`app_env.go` 1 方法）、hosts 文件（`app_hosts.go` 1 方法）、PPT 拆分（`app_pptx.go` 2 方法）四个域整域未登记（合计 7），搜索域行内计数差 1（`app_search.go` 实际导出 3）、外部集成行内计数差 3（`app_external.go` 实际导出 12），方法总数以 `App.d.ts` 为准，待另案补齐。（素材购物栏 7 方法已随 2026-10 购物栏功能登记。连接管控域 `KickConnection`/`ListWebTokens`/`CreateWebToken`/`RevokeWebToken` 与审计读侧 `QueryAuditLog`（纯桌面面，浏览器 RPC 通道屏蔽）已随 2026-10 连接管控任务落地；`SetAuditConfig` 审计留存热更已随 2026-10 连接管控十一期落地（`webAuditSvc` 桥接，App.d.ts 为准），本域表格待另案一并补齐。）

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
| `OpenDirectoryDialog` | `(title) => Promise<string>` | 原生目录选择对话框（素材库新建等场景，serve 无头模式拒绝） | 选定目录路径（取消返回空串） |

### 6.1 PPTX 预览转换（`app_pptx_preview.go`）

`.pptx/.ppsx` 文件树内嵌预览：后端经本机 PowerPoint/WPS COM 整体转换为 PDF（复用素材缩略图同一 COM STA 单队列），前端把 `pdfPath` 当 kind=pdf 走现成 pdfjs viewer iframe 链路（`/preview-pdf` 同源 handler 零改动）。错误经返回结构 `error`/`errorCode` 字段下发（不走 Wails reject 通道），前端降级提示 + 「用默认程序打开」。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `ConvertPptxForPreview` | `(filePath) => Promise<PptxPreviewResult>` | 转换 pptx/ppsx 为 PDF：缓存命中（源未变）直接返回缓存路径零 COM 延迟；未命中经 COM 转换（60s 超时、瞬态失败重试 1 次、同文件在途转换防抖共享）。缓存目录 `data/pptx_preview_pdf/`，键 = 源路径+size+mtime（源变更自动失效），LRU 上限 512MB 淘汰最旧，索引 `index.json` 走 util.SaveJSON 原子写 | PptxPreviewResult |

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
| `ContentSearch` | `(keyword, fileExt, subDir, searchAll) => Promise<ContentSearchGroup[]>` | 文件内容搜索（按仓库分组，全局 60s / 单目录 10s ctx） | ContentSearchGroup 数组 |
| `ContentSearchDirs` | `(keyword, fileExt, subDir, dirs, repoNames) => Promise<ContentSearchGroup[]>` | 指定目录列表内容搜索（命令面板超时目录单目录重试入口，固定 60s ctx 给大目录第二次机会） | ContentSearchGroup 数组 |

---

## 9. Git 仓库信息

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `GetGitInfo` | `(path) => Promise<GitRepoInfo>` | 获取仓库概览（分支/远程/近期提交） | GitRepoInfo |
| `CloneRepo` | `(url, targetPath) => Promise<string>` | 克隆仓库 | 操作结果消息 |
| `PullRepo` | `(dirPath, useRebase) => Promise<string>` | 拉取（可选 rebase 模式），可被 `CancelGitOperation` 取消 | 操作结果消息；用户取消返回 `已取消` |
| `ScanAndPullRepos` | `(dirPath) => Promise<PullSummary>` | 扫描目录下仓库并批量拉取 | PullSummary |
| `ExtractRepoName` | `(url) => Promise<string>` | 从 URL 提取仓库名 | 仓库名 |
| `CancelGitOperation` | `(path) => Promise<boolean>` | 取消指定仓库在途 Git 操作（Pull/Clone/批量拉取单仓 + 冲突类 Merge/Rebase/CherryPick 及 Continue/Skip 七方法，Abort 三类不可取消），终止在途 git 子进程 | 是否存在可取消的在途操作；无在途操作幂等 false |
| `CancelAllGitOperations` | `() => Promise<number>` | 取消全部在途 Git 操作（批量拉取「取消未完成」，已完成仓库保留），返回取消仓库数 | 取消仓库数 |
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

AI 对话工作台（活动栏「AI 对话」面板）：**持续会话进程式**多会话对话（每会话一个常驻 claude 进程，stdin 流式注入驱动轮次）。会话/消息/目录/模板/配置独立持久化到 `data/ai_chat/`。对话任务异步事件流经 EventSink 推送：轮次流 `chat-task:*`（`queued` 载荷 `{taskId}`；`started` 载荷 `{taskId, chatSessionId}`；`output` 载荷 `{taskId, chatSessionId, text}`（流式文本增量）；`done` 载荷为 `model.ChatTaskRunResult` 全量结果）+ 审批流 `chat-approval:*`（`request` 载荷 `model.ChatApprovalRequest`；`cancelled` 载荷 `{requestId, chatSessionId, reason}`）。运行中插话 = 同会话再调 `RunChat`（排队注入，不再拒绝）；工具授权经 `RespondChatApproval` 弹窗应答；进程空闲/参数变更自动重启并 `--resume` 续上下文。

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
| `RunChat` | `(chatSessionID, prompt, permissionMode, modelName) => Promise<string>` | 提交一轮对话（permissionMode 仅非 default 时传 claude，modelName 空 = claude 默认），返回轮次 id；运行中再发 = 排队注入（前序轮 result 后依序执行）；prompt 为空拒绝（`E_CHAT_EMPTY_PROMPT`）；新会话进程槽满拒绝（`E_CHAT_PROCESS_LIMIT`，上限 4） | taskID |
| `GetChatTaskState` | `(taskID) => Promise<ChatTaskState>` | 查询单轮次状态（回复累积文本/claude 会话 id/运行态；不存在返回 null；多轮次恢复用 `ListChatActiveTasks`） | ChatTaskState |
| `ListChatActiveTasks` | `(chatSessionID) => Promise<ChatTaskState[]>` | 列出会话全部在途轮次（排队+执行中，按开始时间升序；chatSessionID 空 = 全部会话），面板切回恢复用 | ChatTaskState 数组 |
| `CancelChatTask` | `(taskID) => Promise<boolean>` | 取消对话轮次（执行中发 interrupt 中断当前轮 / 排队中直接摘除），被中断轮部分回复保留入档 | 是否成功 |
| `RespondChatApproval` | `(requestID, allow) => Promise<void>` | 应答工具授权审批弹窗（`chat-approval:request` 事件驱动）：allow=true 放行（回传原 input 作 updatedInput），false 拒绝（模型收到拒绝原因）；请求不存在/已失效报错 | — |

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

## 29. 素材管理

侧边栏「素材管理」本地 DAM 素材索引库（MVP）：素材库 CRUD + 目录扫描（增量索引）+ 幻灯片单页索引 + 检索 + 文件级元数据编辑 + 元数据导出（CSV / Markdown+YAML，SRS §2.6.4）+ 文件丢失标记与重链（lost 状态机 / 手动重定位 / 路径前缀替换，SRS §2.6.5）+ 素材库备份与还原（手动 + 重建/删库前自动快照，SRS §2.7.5）。索引库以 SQLite 文件落盘 `data/assets/<library-id>.db`（modernc 纯 Go 驱动 + FTS5），库注册表 `data/assets/libraries.json`；索引模式只读引用原始文件，扫描不修改源文件。导出与丢失重链契约详见 [spec/assets-management.md](spec/assets-management.md) §11。

扫描进度/终态经事件推送（EventSink 通道）：`assets-scan:progress`（`{libraryId, done, total, currentFile}`）/ `assets-scan:done`（`{libraryId, result: AssetScanResult}`）/ `assets-scan:error`（`{libraryId, message}`）。

拆分进度/终态经事件推送（素材面板独听，与工具箱 `pptx-split:*` 隔离）：`assets-split:progress`（`{libraryId, done, total, currentFile, pageDone, pageTotal}`，文件级计数 + 当前文件页级透传）/ `assets-split:done`（`{libraryId, result: AssetSplitBatchResult}`，单文件同形状 Total=1）/ `assets-split:error`（`{libraryId, message}`，仅整体失败——批量内单文件失败走结果统计）。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `CreateAssetLibrary` | `(name, rootPath) => Promise<AssetLibrary>` | 新建素材库（校验名称非空、根目录存在且为目录；登记注册表并预建索引库文件） | AssetLibrary |
| `ListAssetLibraries` | `() => Promise<AssetLibrary[]>` | 全部素材库（注册表顺序，库切换下拉数据源） | AssetLibrary[] |
| `DeleteAssetLibrary` | `(libraryID, allowNoBackup) => Promise<void>` | 删除素材库（删库前自动备份 + 关连接 + 删 db 含 WAL/SHM + 删注册表项；备份目录不随库删除清理；扫描中拒绝，`E_ASSETS_SCAN_IN_PROGRESS`；备份失败且 `allowNoBackup=false` 返回 `E_ASSETS_DELETE_BACKUP_FAILED` 确认型拒绝，用户二次确认后 `true` 重调） | — |
| `GetAssetLibraryStats` | `(libraryID) => Promise<AssetLibraryStats>` | 库统计（文件/幻灯片总数、失败/加密跳过数） | AssetLibraryStats |
| `StartAssetScan` | `(libraryID) => Promise<AssetScanResult>` | 增量扫描（递归索引根目录 .pptx，mtime+size 判变，变更才哈希重解析；加密/损坏/权限不足单文件降级跳过不中断批量；同库并发扫描拒绝） | AssetScanResult |
| `CancelAssetScan` | `(libraryID) => Promise<boolean>` | 取消指定库在途扫描（幂等，无在途扫描返回 false） | boolean |
| `UpdateAssetLibraryRules` | `(libraryID, excludeDirs: string[], excludeExts: string[]) => Promise<AssetScanResult>` | 更新库排除规则（SRS §2.7.3 索引配置，整体替换）并保存后自动增量重扫使规则即时生效：目录模式按文件夹名匹配（单段通配 `*` `?` `[...]`，大小写不敏感，不含路径分隔符）、后缀宽容 `tmp`/`.tmp`/`*.tmp` 三形态；规则命中存量转 excluded（检索不可见、标注保留），改宽后复活；同库扫描中时规则已保存但重扫被拒（`E_ASSETS_SCAN_IN_PROGRESS`） | AssetScanResult |
| `RebuildAssetLibrary` | `(libraryID) => Promise<AssetScanResult>` | 手动全量重建（清该库文件/幻灯片/FTS 记录后全量重扫；文件标签按相对路径重绑，星级/备注不保留；索引自愈入口） | AssetScanResult |
| `ListAssetFiles` | `(libraryID) => Promise<AssetFileEntry[]>` | 文件索引条目（含标签，按相对路径序；画廊文件视图数据源） | AssetFileEntry[] |
| `ListAssetSlides` | `(libraryID, limit, offset) => Promise<AssetSlidePage>` | 幻灯片索引条目分页（标签继承归属文件，按文件路径 + 页序；页序以 sldIdLst 为准；limit ≤0 取 50 上限 200、offset<0 按 0，total 恒为总条目数） | AssetSlidePage |
| `UpdateAssetFileMeta` | `(libraryID, fileID, meta: AssetFileMetaUpdate) => Promise<void>` | 文件级元数据编辑（整体替换：星级 0-5（0 清除）、备注、标签集（不存在的标签自动创建并分配默认色）） | — |
| `SearchAssets` | `(libraryID, keyword, opts: AssetSearchOptions) => Promise<AssetSearchPage>` | 素材检索：关键词全文检索（bm25 列加权 文件名 > 幻灯片标题 > 页文本 > 备注，CJK 按字切分 phrase 匹配）与标签筛选（文件直接绑定、幻灯片继承归属文件）可组合；`flatten=false` 同 PPT 多页命中折叠为聚合条目 | AssetSearchPage |
| `AssetThumbsAvailable` | `() => Promise<boolean>` | 探测本机 PowerPoint COM 可用性（前端降级依据：false 时画廊提示条常驻 + 跳转入口隐藏） | boolean |
| `JumpToAssetSlide` | `(absPath, slideNum) => Promise<void>` | 打开 PPT 并跳到指定页（PowerPoint COM；批量生成在途时经页间插队执行；无 PowerPoint 报 `E_ASSETS_THUMBS_UNAVAILABLE`，排队/执行超时报 `E_ASSETS_THUMBS_BUSY`） | — |
| `CancelAssetThumbs` | `(libraryID) => Promise<boolean>` | 取消指定库的缩略图批量生成（幂等，无在途批次返回 false） | boolean |
| `GetAssetThumb` | `(libraryID, fileID, slideNum, tierIndex) => Promise<string>` | 读取单页缩略图 PNG（base64 标准编码裸串；tierIndex 0=small 画廊卡片 1=hd 详情大图；未生成/已淘汰返回空串非错误，前端回退占位卡；消费契约：直接拼 data URL，禁 decodeBase64Utf8 禁裸 atob） | string |
| `RequestAssetThumb` | `(libraryID, fileID, relPath, slideNum, tierIndex) => Promise<void>` | 详情侧单页单档按需生成（hd 退出批量补缺后的唯一补齐入口）：PNG 已存在或同页在途直接返回，异步入队独立按需通道（不与批量批次互相取消），完成后发 `assets-thumbs:done` 且载荷 `onDemand=true`；队列满报 `E_ASSETS_THUMBS_BUSY`，无 PowerPoint 报 `E_ASSETS_THUMBS_UNAVAILABLE` | — |
| `RetryAssetThumb` | `(libraryID, fileID, relPath) => Promise<void>` | 单文件小图重试（复用 COM 串行队列链路，触发时取消同库在途批量批次（既有取消语义）；批量链路只导 small 档，hd 走 `RequestAssetThumb`；源文件不存在报错，无 PowerPoint 返回 `E_ASSETS_THUMBS_UNAVAILABLE`） | — |
| `SplitAssetFile` | `(libraryID, relPath, target) => Promise<AssetSplitResult>` | 单文件 PPT 按页拆分（详情侧边栏「拆分此 PPT」入口；落点三选一：`cache` 缓存目录 derived 镜像源树（默认）/ `source` 源目录（自动排除索引）/ `replace` 就地替换（页序前缀命名进索引，全页成功 + 逐页校验 + 标注迁移后把源移入系统回收站；删源失败报 `E_ASSETS_SPLIT_SOURCE_DELETE_FAILED` 且产物保留））；增量命中跳过返回 `skipped=true`；核心失败（损坏/加密/只读）报错，取消复用 `E_ASSETS_CANCELLED` | AssetSplitResult |
| `SplitAssetLibrary` | `(libraryID, target) => Promise<AssetSplitBatchResult>` | 库级批量拆分（「拆分全部 PPT」入口；落点三态同 `SplitAssetFile`，`replace` 需与扫描互斥）；启动时快照圈定全部 ok 状态文件，单文件失败逐文件记日志跳过不中断批量，返回汇总与逐文件明细（明细含 `target`/`sourceRemoved`/`rolledBack`/`code`；删源失败只在 `item.code` 透出、不整体失败）；同库并发拆分拒绝 `E_ASSETS_SCAN_IN_PROGRESS` | AssetSplitBatchResult |
| `CancelAssetSplit` | `(libraryID) => Promise<boolean>` | 取消指定库在途拆分（幂等，无在途拆分返回 false；取消错误前端按 `E_ASSETS_CANCELLED` warning 中性提示） | boolean |
| `GetAssetSplitRecord` | `(libraryID, relPath) => Promise<AssetSplitRecordView>` | 查询文件拆分记录（跳转方式 2 判定面：`exists` 决定「打开拆分单页」按钮显隐，`upToDate` 指示源未变化，`target`/`sourceRemoved` 供展示与诊断；重拆不被前端禁用，源从回收站还原后由后端四验放行） | AssetSplitRecordView |
| `OpenAssetSplitPage` | `(libraryID, relPath, slideNum) => Promise<void>` | 打开拆分单页产物（SRS §2.2.5 方式 2，无 PowerPoint 依赖，系统默认关联程序；页序越界或产物缺失报错） | — |
| `ExportAssetMetadata` | `(libraryID, opts: AssetExportOptions) => Promise<AssetExportResult>` | 元数据导出（SRS §2.6.4）：CSV（UTF-8 BOM）与 Markdown+YAML（Obsidian 每页一卡）可同选；范围选中条目 > 检索条件重放 > 全库；路径口径库相对（默认）/绝对；目标已存在报 `E_ASSETS_EXPORT_TARGET_EXISTS`（warning 确认后 overwrite 重调） | AssetExportResult |
| `CancelAssetExport` | `(libraryID) => Promise<boolean>` | 取消指定库在途导出（幂等；半成品 CSV 由后端 abort 清理） | boolean |
| `RelocateAssetFile` | `(libraryID, fileID, newAbsPath, force) => Promise<AssetRelocateResult>` | lost 文件手动重链（SRS §2.6.5）：哈希相等直复活全元数据；不等报 `E_ASSETS_RELINK_MISMATCH`（force=true 强制重链并重解析）；库外目标报 `E_ASSETS_RELINK_OUT_OF_LIB` | AssetRelocateResult |
| `ClearLostAssetFiles` | `(libraryID) => Promise<number>` | 清理全部丢失记录（等效删除语义：级联标注/标签 + FTS/缩略图记账/PNG 收口；返回实际删除数） | number |
| `PreviewAssetPathPrefixReplace` | `(libraryID, oldPrefix, newPrefix) => Promise<number>` | 路径前缀替换预览（目录边界语义命中数；零命中报 `E_ASSETS_PREFIX_REPLACE_NO_MATCH`） | number |
| `ReplaceAssetPathPrefix` | `(libraryID, oldPrefix, newPrefix) => Promise<AssetPrefixReplaceResult>` | 执行前缀替换（库内子目录重组修复：单事务批量改 path + 三态收敛（匹配复活/重解析/保持 lost）+ 自动增量扫描；越界任一整体拒绝 `E_ASSETS_RELINK_OUT_OF_LIB`） | AssetPrefixReplaceResult |
| `BackupAssetLibrary` | `(libraryID) => Promise<AssetBackupEntry>` | 手动备份（SRS §2.7.5，整库 VACUUM INTO 快照 + integrity_check 校验（非 ok 删备份报错）+ 每库最近 5 份保留淘汰；落点 `data/assets/backups/<libraryId>/<yyyyMMdd-HHmmss>.db`；缩略图 PNG 与拆分产物不在备份范围；重建/删库前自动快照复用同一实现） | AssetBackupEntry |
| `ListAssetLibraryBackups` | `(libraryID) => Promise<AssetBackupEntry[]>` | 备份清单（按备份时间倒序，条目含文件名/时间/大小/绝对路径；从未备份返回空清单非错误） | AssetBackupEntry[] |
| `RestoreAssetLibrary` | `(libraryID, backupFile) => Promise<void>` | 还原备份（备份文件整体替换 `<id>.db`：取消并等待在途扫描退出 + 关连接 + 删 WAL/SHM 残留 + `os.Rename` 覆盖 + 惰性重开并复验；`backupFile` 须为该库备份目录内的备份文件（`ListAssetLibraryBackups` 返回的 `path` 原样回传）；覆盖前预检备份完整性，损坏备份不动当前库；还原后前端提示重扫） | — |
| `ListAssetFolders` | `(libraryID) => Promise<AssetFolderNode[]>` | 库根目录真实子目录树（文件夹树数据源，读文件系统非索引派生——空目录可见；排除规则命中目录整棵子树带 `excluded` 标记、拆分派生容器带 `derived` 标记，均复用扫描侧 matcher 单点；库根目录不存在返回空树非错误） | AssetFolderNode[] |
| `CreateAssetFolder` | `(libraryID, parentRelPath, name) => Promise<AssetFolderNode>` | 新建子目录（父目录磁盘 mkdir；重名（os.ErrExist 口径）/父目录不存在/父为排除命中目录或派生容器拒绝；纯磁盘操作不触碰索引，返回新建节点） | AssetFolderNode |
| `DeleteAssetFolder` | `(libraryID, relPath, force) => Promise<number>` | 删除目录（磁盘递归删除 + 索引内该前缀文件行 markFileLost 收口（标注保留待复活）+ 拆分记录与文件夹标注按前缀清理；非空目录 `force=false` 返回 `E_ASSETS_FOLDER_NOT_EMPTY` 确认型拒绝（二次确认后 `true` 重调）；排除命中目录与派生容器拒绝；返回标记 lost 的索引行数） | number |
| `RenameAssetFolder` | `(libraryID, relPath, newName) => Promise<AssetFolderRenameResult>` | 同父重命名（与目录移动共用统一核心：磁盘 rename + 索引路径前缀批量更新（前缀替换单点）+ 排除规则双向翻转（新名命中翻 excluded / 原排除解除经 reviveLostFile 复活，FTS 同事务重建）+ folder_meta/folder_tags 键同事务级联改键 + 自动增量扫描收敛；重名拒绝；子树内存在拆分记录拒绝） | AssetFolderRenameResult |
| `MoveAssetFolder` | `(libraryID, folderRelPath, targetFolder) => Promise<AssetFolderRenameResult>` | 目录级移动（移入目标目录下保持自身名称，`targetFolder` 空串 = 库根；与重命名共用统一核心；目标不得位于源目录自身子树内，目标校验与拆分记录拒绝口径同重命名） | AssetFolderRenameResult |
| `MoveAssetFiles` | `(libraryID, fileIDs: number[], targetFolder) => Promise<AssetBatchMoveResult>` | 文件批量移动（fileIDs 去重；目标目录存在/非排除命中/非派生容器整体校验；单文件跨盘/重名/源缺失/解析中拒绝；每条目一事务逐条目提交（files.path 更新 + split_records 源路径同事务重写），部分失败不中断逐条目明细返回；FTS 零重建（检索列不含路径）+ 排除规则双向翻转） | AssetBatchMoveResult |
| `GetAssetFolderMeta` | `(libraryID, folderPath) => Promise<AssetFolderMeta>` | 读文件夹标注（业务备注 + 标签集，标签 ID 口径与文件/页标签同源 tags 表；未标注目录返回零值形态；库根/排除命中目录/派生容器禁标注） | AssetFolderMeta |
| `UpdateAssetFolderMeta` | `(libraryID, folderPath, meta: AssetFolderMetaUpdate) => Promise<void>` | 文件夹标注编辑（整体替换：备注 + 标签集（不存在的标签自动创建）；纯标注层不入 FTS、不触发重解析与缩略图失效；守卫与读取同口径） | — |

### 29.1 素材购物栏（跨库页级收集与合并导出）

素材管理「购物栏」：跨库把任意 PPT 的任意页加入购物栏（键 `<libraryId>|<fileId>|<slideNum>`，fileId 为 files.id 跨重解析稳定），抽屉内拖拽调整顺序，导出按购物栏顺序将全部页合并为单一 .pptx（纯 Go OOXML 基底注入式合并，契约见 [spec/pptx-merge.md](spec/pptx-merge.md)）。导出进度/终态经事件推送（前端闭包注销监听）：`assets-cart:export-progress` / `assets-cart:export-done` / `assets-cart:export-error`。

| 方法 | 签名 | 语义 | 返回值 |
|---|---|---|---|
| `ListAssetCartItems` | `() => Promise<AssetCartItem[]>` | 购物栏清单（按栏内顺序；逐条 `os.Stat` 探测源文件存在性，缺失条目 `stale=true` 置灰展示、不自动移除） | AssetCartItem[] |
| `AddAssetCartItems` | `(items: AssetCartItem[]) => Promise<AssetCartAddResult>` | 批量加入（追加语义，顺序=入参顺序；按条目键幂等去重，重复项计入 duplicates） | AssetCartAddResult |
| `RemoveAssetCartItems` | `(keys: string[]) => Promise<void>` | 按条目键移除（容忍未知键） | — |
| `ClearAssetCart` | `() => Promise<void>` | 清空购物栏 | — |
| `ReorderAssetCart` | `(orderedKeys: string[]) => Promise<void>` | 拖拽排序回写（须为当前栏内键的完整排列，缺失/多余报错防丢条目） | — |
| `MergeAssetCartToPptx` | `(outPath) => Promise<PptxMergeResult>` | 按购物栏顺序合并导出新 PPT（基底=清单第一项源文件 GC 后，其余源整链注入；失效条目跳过计入 SkippedItems，基底缺失整体失败；导出中重复触发报 `E_CART_EXPORT_IN_PROGRESS`；可取消，取消无半态产物） | PptxMergeResult |
| `CancelAssetCartExport` | `() => boolean` | 取消在途导出（幂等，无在途返回 false） | boolean |

---

## 30. 数据模型

全部提取自 `frontend/wailsjs/go/models.ts`（`model` namespace；`frontend` namespace 含 `FileFilter`）。字段名按 Go json tag 映射，`omitempty` 对应 TS `?:` 可选。以下列出关键 struct，完整定义见 `models.ts`。

### 30.1 目录与文件

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

**PptxPreviewResult** — pptx 预览转换结果（`ConvertPptxForPreview`）

| 字段 | 类型 | 说明 |
|---|---|---|
| `pdfPath` | string? | 缓存 PDF 绝对路径（空串 = 未产出）；前端拼 `/preview-pdf?path=` 同源 URL 交 pdfjs viewer |
| `error` | string? | 用户可读中文错误消息（非空 = 失败，前端降级提示 + 用默认程序打开） |
| `errorCode` | string? | 机器可读错误码：`E_PPTX_PREVIEW_UNAVAILABLE`（非 Windows / 未检测到 PowerPoint/WPS）/ `E_PPTX_PREVIEW_FAILED`（文件损坏 / COM 调用失败 / 超时） |

**FileChange** — 本地变更文件

| 字段 | 类型 | 说明 |
|---|---|---|
| `path` | string | 相对路径 |
| `status` | string | 变更状态：M/A/D/R/? |
| `staged` | boolean | 是否已暂存 |

### 30.2 Git

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

### 30.3 会话快照

**SessionState** — 崩溃恢复 UI 状态快照

| 字段 | 类型 | 说明 |
|---|---|---|
| `selectedDirectoryId` | string? | 当前工作目录 ID |
| `activePanel` | string? | 活动面板：directory/ai/ai-chat/toolbox/assets/dashboard/stats |
| `terminal` | TerminalSnapshot? | 终端面板快照 |
| `version` | string? | 快照格式版本 |
| `savedAt` | number? | 保存时间戳 |

**TerminalSnapshot**（`visible` / `height` / `workDir`（旧版单终端兼容读取源，新快照不写入）/ `tabs`（v2 多终端 tab 列表 `TerminalTabSnapshot[]`：`workDir` / `shellType`）/ `activeIndex` / `fullscreen`）

### 30.4 设置与更新

**AppSettings** — 应用设置（`gpuDisabled` / `defaultShell` / `gitBashPath` / `wslDistro` / `terminalFontSize` / `terminalFontFamily` / `terminalScrollback` / `searchExcludeDirs` / `searchExcludeFiles` / `shortcutCommandPalette` / `shortcutToggleTerminal` / `shortcutRename` / `shortcutDelete` / `obsidianPath` / `tessoaPath`（tessoa 自定义路径，「打开资源管理器」优先使用）/ `themeMode` / `diffToolName` / `diffToolPath` / `diffToolArgs` / `webServe` / `assets`（AssetThumbsSettings 缩略图设置段））。终端外观三字段加载时经 `EnsureTerminalDefaults` 补默认值并收敛越界值（字号 10-24 默认 14、回滚 1000-10000 默认 1000，字体空串走默认 Cascadia Code 栈）

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

### 30.5 AI

**AiFunction** — AI 功能配置（`id` / `name` / `description` / `icon` / `command` / `cwd` / `addDirs` / `env` / `mcp` / `permissionMode` / `timeoutMinutes` / `completion` / `params` / `followUps` / `tags` / `pinned`）

**AiTaskState** — 任务实时状态（`taskId` / `functionId` / `running` / `queued` / `sessionId` / `prompt` / `output` / `outputSize` / `outputFile` / `tableExtracted` / `error` / `startedAt` / `metrics`）

**AiTaskHistory** — 任务历史（`id` / `functionId` / `name` / `prompt` / `startedAt` / `finishedAt` / `status` / `exitCode` / `error` / `sessionId` / `metrics` / `outputFile` / `outputSize`）

**AiTaskHistoryFilter**（`functionId` / `status` / `from` / `to`）

**AiTaskHistoryClearCriteria**（`olderThanDays` / `keepRecent` / `functionId`）

**AiConcurrencyStatus**（`running` / `queued` / `max`）

**AiTaskHistoryStats**（`totalCount` / `successCount` / `totalCostUsd` / `totalInputTokens` / `totalOutputTokens` / `totalCacheReadTokens` / `totalCacheCreationTokens` / `totalDurationMs` / `byFunction`: FunctionStat[]）

**ImportPreview** — AI 功能导入预览（`new` / `conflict`: AiFunction[]，`invalid`: string[]）

### 30.6 AI 对话

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

**ChatApprovalRequest** — 工具授权请求（`chat-approval:request` 事件 payload；`requestId`（claude 侧 UUID，应答匹配键）/ `chatSessionId` / `taskId`（所属在途轮次）/ `toolName` / `input`（工具入参 map，弹窗展示） / `description`）

### 30.7 素材管理

**AssetLibrary** — 素材库注册信息（`id`（随机 hex，同时是库 db 文件名）/ `name` / `rootPath` / `excludeDirs`?: string[]（排除文件夹 glob 规则，单段通配按文件夹名匹配）/ `excludeExts`?: string[]（忽略后缀，带点小写归一）/ `createdAt` / `updatedAt`）

**AssetTag** — 标签（`id` / `name`（库内唯一）/ `color`（#RRGGBB，自动创建走默认色板）/ `parentId`（预留多级树，恒 0））

**AssetFileEntry** — 文件索引条目（`id` / `path`（相对库根目录，正斜杠分隔，库内唯一）/ `size` / `modTime` / `status`（`processing`/`ok`/`failed`/`encrypted`/`denied`/`lost`/`excluded`）/ `title`（源文件名去扩展名）/ `note` / `rating`（0-5，0 = 未评分）/ `tags`: AssetTag[] / `createdAt` / `updatedAt`）

**AssetSlideEntry** — 幻灯片单页索引条目（`id` / `fileId`（归属文件记录 ID）/ `filePath` / `slideNum`（展示页序从 1 计）/ `title`（回退链：标题占位符 → 首行短文本 → 文件名+页码，空页兜底「[无标题]」）/ `text` / `speakerNotes` / `tags`（继承归属文件标签））

**AssetLibraryStats** — 库统计（`libraryId` / `fileCount`（含各状态）/ `slideCount` / `failedCount` / `encryptedCount` / `lostCount` / `excludedCount`）

**AssetScanProgress** — 扫描进度事件载荷（`libraryId` / `done` / `total` / `currentFile`）

**AssetScanResult** — 扫描结果汇总（`total` / `new` / `updated` / `unchanged` / `failed` / `encrypted` / `denied` / `lost`（本次新标记丢失）/ `revived`（直接复活未重解析：lost 回归或 excluded 解除）/ `excluded`（本次新标记排除）/ `removed`（兼容保留恒 0））

**AssetFileMetaUpdate** — 元数据编辑入参（`rating` / `note` / `tags`: string[]，整体替换语义）

**AssetSearchOptions** — 检索入参（`flatten`（true 平铺 / false 聚合折叠）/ `tagIds`: number[] / `folderPath`（文件夹维度筛选，相对库根目录前缀，与关键词/标签/facet AND 叠加，空串 = 全库）/ `limit`（默认 50，上限 200）/ `offset`）

**AssetFolderNode** — 文件夹树节点（`path`（相对库根正斜杠路径，库根不返回节点——前端固定「全部素材」根节点以空串语义表达）/ `name` / `children`: AssetFolderNode[]（文件系统字典序，空目录恒非 nil 空切片）/ `excluded`（排除规则命中，整棵子树继承）/ `derived`（拆分派生容器）/ `fileCount`（直属 PPT 磁盘口径计数））

**AssetFolderMeta** — 文件夹级标注（`folderPath` / `note`（业务备注，不入 FTS）/ `tags`: AssetTag[]（ID 口径与文件/页标签同源）/ `updatedAt`；键 = folder_path，重命名/移动目录时同事务级联改键，删除目录级联删标注）

**AssetFolderMetaUpdate** — 文件夹标注编辑入参（`note` / `tagNames`: string[]，整体替换语义，不存在的标签自动创建）

**AssetBatchMoveResult** — 文件批量移动汇总（`total`（fileID 去重后）/ `succeeded` / `failed` / `items`: AssetBatchMoveItem[]（`fileId` / `status`（`ok`/`failed`）/ `message`（失败原因）/ `label`（移动前路径定位展示）））

**AssetFolderRenameResult** — 目录重命名/移动结果（`oldPath` / `newPath`（正斜杠）/ `replaced`（索引路径更新数）/ `excluded`（新命中排除翻转数）/ `revived`（排除解除复活数））

**AssetSearchHit** — 检索命中条目（`refType`（`file`/`slide`）/ `refId`（文件条目 = files.id，幻灯片条目 = slides.id）/ `fileId` / `filePath` / `title` / `slideNum` / `snippet`（原文子串窗口含省略号）/ `score`（bm25 负值口径，越小越相关）/ `hitCount`（聚合条目命中页数，其余恒 1）/ `tags`）

**AssetSearchPage** — 检索结果页（`items`: AssetSearchHit[] / `total`（过滤/聚合后总条目数，翻页以 offset 递增驱动，Total 不随页变化）/ `keyword` / `flattened`）

**AssetThumbsSettings** — 缩略图设置（`AppSettings.assets` 段，老配置段缺失走默认值；素材管理面板「缩略图设置」消费，随 `SaveSettings` 整体持久化）

| 字段 | 类型 | 说明 |
|---|---|---|
| `auto` | boolean | 扫描完成后自动生成缩略图（默认 true；关闭时扫描后不再自动入队） |
| `smallWidth` | number | 画廊小图宽度 px（默认 320，clamp 120-800；变更自动清该档缓存并按新分辨率重新生成） |
| `hdWidth` | number | 详情高清大图宽度 px（默认 1600，clamp 480-3840；变更自动清该档缓存并按新分辨率重新生成） |
| `smallQuotaMB` | number | 小图缓存上限 MB（默认 1024，clamp 64-8192；变更触发惰性 LRU 淘汰最久未使用的缩略图） |
| `hdQuotaMB` | number | 高清缓存上限 MB（默认 1024，clamp 64-8192；变更触发惰性 LRU 淘汰最久未使用的缩略图） |

**AssetSplitResult** — 单文件拆分结果（`outputDir`（产物目录绝对路径）/ `files`: string[]（本次生成的单页文件路径，按页序）/ `pageCount`（拆出页数）/ `skipped`（增量命中被跳过）/ `skipReason`（跳过原因，用户可读）/ `target`（本次落点 `cache`/`source`/`replace`）/ `sourceRemoved`（源文件是否已移入回收站，仅 replace 成功删源为 true））

**AssetSplitFileOutcome** — 批量拆分逐文件处理结果（`relPath`（相对库根路径）/ `status`（`split`/`skipped`/`failed`）/ `message`（跳过/失败原因，成功为空）/ `target`（该文件落点）/ `sourceRemoved`（源已移入回收站）/ `rolledBack`（replace 失败/取消时产物与索引行已回滚）/ `code`（失败错误码，如 `E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`；成功为空））

**AssetSplitBatchResult** — 批量拆分汇总（`total`（快照圈定文件总数）/ `split` / `skipped` / `failed` / `items`: AssetSplitFileOutcome[]；单文件入口同形状 Total=1，done 事件载荷复用）

**AssetSplitRecordView** — 拆分记录查询视图（`exists`（是否已有拆分记录，方式 2 按钮显隐判定）/ `outputDir` / `pageCount` / `splitAt`（最近拆分时间）/ `upToDate`（记录哈希与当前源哈希一致）/ `target`（记录落点，后端选产物命名规则；前端仅展示）/ `sourceRemoved`（源已移入回收站；前端不禁用重拆，还原后由后端放行））

**AssetSplitProgress** — 拆分进度事件载荷（`libraryId` / `done` / `total`（文件级计数）/ `currentFile` / `pageDone` / `pageTotal`（当前文件页级透传））

**AssetBackupEntry** — 素材库备份条目（SRS §2.7.5：`libraryId` / `fileName`（`<yyyyMMdd-HHmmss>[_序号].db`）/ `path`（备份文件绝对路径，还原时原样回传）/ `sizeBytes` / `createdAt`（由文件名时间戳解析）；备份为整库 SQLite 快照（VACUUM INTO 产物），缩略图 PNG 与拆分产物不在范围）

### 30.8 其他

**Favorite**（`path` / `alias` / `group` / `createdAt`）

**RepoFilterItem**（`name` / `path` / `summary` / `tags` / `readmeSummary` / `missing` / `hasRemote` / `isGitRepo`）

**RepoStatus** — 状态看板仓库状态（`path` / `name` / `branch` / `dirty` / `ahead` / `behind` / `hasUpstream` / `detached` / `isRepo` / `missing` / `error`）

**RepoConfigImportPreview**（`newDirectories` / `conflictDirectories`: RepoConfigDirectoryPreview[]，`newFavorites` / `conflictFavorites`: RepoConfigFavoritePreview[]，`invalid`: RepoConfigInvalidItem[]）

**RepoConfigImportDecisions**（`directories` / `favorites`: Record<string, string>，决策值 skip/overwrite/saveAsNew）

**RepoConfigImportResult**（`added` / `overwritten` / `skipped` / `failed` / `failedReasons`）

**PullSummary**（`total`，`cancelled`: 取消仓库数，随 pull-complete 事件透出）

**SearchResult**（`name` / `path` / `type`）

**ContentSearchGroup**（`repoName` / `repoPath` / `items`: ContentSearchResult[] / `timedOut?`: 该目录搜索超时时为 true，items 恒空，前端渲染 warning 行供单目录重试）

**ShellConfig**（`type` / `executable` / `args` / `displayName`）

**SkillDescriptor**（`name` / `description` / `command` / `cwd` / `source` / `sourceDir` / `plugin`）

**AssetCartItem** — 素材购物栏条目（`libraryId` / `fileId`（files.id，重解析稳定）/ `slideNum`（1 起展示页序）/ `srcPath`（源文件绝对路径快照，失效探测基准）/ `fileName` / `slideTitle?` / `libraryName?`（展示快照，入栏时定格不回写）/ `addedAt`（RFC3339，后端权威）/ `stale?`（List 探测态非持久化字段））

**AssetCartAddResult** — 加入购物栏结果（`added` 新增数 / `duplicates` 键已在栏去重跳过数）

**PptxMergeItem** — 合并导出页项（`srcPath` / `pageNum`，按购物栏顺序排列）

**PptxMergedPage** — 合并产物页明细（`srcPath` / `srcPageNum` / `outPageNum`，按输出页序排列）

**PptxMergeSkipped** — 跳过页项（`srcPath` / `pageNum` / `reason`；单源打不开/页越界只跳该项继续，基底失败为整体错误）

**PptxMergeResult** — 合并导出结果（`outputPath` / `pages`: PptxMergedPage[]（购物栏顺序）/ `pageCount` / `skippedItems`: PptxMergeSkipped[] / `sldSzMixed`（各源尺寸不一致，16:9/4:3 混排不自动缩放，前端 warning））

**PptxSubsetResult** — 多页子集导出结果（`outputPath` / `pages`: number[]（去重升序）/ `pageCount`；单文件内页集合导出核心，供合并层复用）

### 30.9 具名 string 类型

`model/commit.go` 定义的两个具名 string 类型，作方法参数时 `App.d.ts` 保留 `model.X` 引用：

| 类型 | 取值 | 用于 |
|---|---|---|
| `MergeMode` | `ff` / `no-ff` / `squash` | `Merge(path, branch, mode)` |
| `SubmoduleUpdateMode` | `checkout` / `merge` / `rebase` / `remote` | `UpdateSubmodules(path, mode, ...)` |

`models.ts` 不为具名 string 类型生成 `export type X = string` 别名；当前前端纯 JS 不 import `models.ts`，运行时打包不触发 `MISSING_EXPORT`。若未来前端显式 import 并运行时引用，须手动补别名，见 [spec/cross-layer-contracts.md](spec/cross-layer-contracts.md)。

---

## 31. 深度查询入口

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

**最后更新：** 2026-10-14
