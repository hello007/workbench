# app.go 按域拆分

## Goal

app.go 单文件 1457 行承载约 98 个 App 方法，跨 15 业务域，单文件过载。按业务域拆分为多文件，App struct 留 app.go，方法分散到对应域文件，保持方法签名与 Wails 绑定不变。纯机械代码移动，不改逻辑。

## What I already know

- app.go 实际 1457 行（原提示词写 1436，已纠正）
- App 方法约 98 个（含 2 私有：`findDirectoryById` / `buildRepoFilterList`）
- App struct 含 14 个 service 字段：`directorySvc` / `fileTreeSvc` / `fileOpSvc` / `gitSvc` / `settingsSvc` / `terminalSvc` / `searchSvc` / `favoritesSvc` / `contentSearchSvc` / `updateSvc` / `repoMetaSvc` / `aiFuncSvc` / `skillDiscoverySvc` + `ctx`
- 私有方法调用关系已 grep 确认：`findDirectoryById` 仅被 `GetRepoFilterList` + `RefreshRepoFilterList` 调用（均在 repometa 域）；`buildRepoFilterList` 同理
- module 名：`workbench`
- 已有测试文件：
  - `app_test.go`（406 行，引用 7 个公开方法，跨 core/directory/git 三域，无私有方法引用）
  - `app_repo_filter_test.go`（458 行，全部 repometa 域，引用私有方法 `buildRepoFilterList`）
- 同 `package main`，私有方法跨文件可访问，测试不受拆分影响
- app.go import 块：`context` / `encoding/json` / `errors` / `fmt` / `os` / `path/filepath` / `strings` / `time` + `go-git`(2 包) + `wails/v2/pkg/runtime` + `workbench/model` / `workbench/service` / `workbench/util`

## Assumptions (temporary)

- 拆分后每个域文件按实际用到的包补 import（手动或 goimports，实现阶段确定）
- `wails build` 产物在 `frontend/wailsjs/`，签名未变则 diff 为空

## Open Questions

（已全部收敛）

## Requirements (evolving)

- 拆分 14 文件（1 留 app.go + 13 新建域文件）
- 仅移动代码，不改方法签名、不改逻辑、不改 Wails 绑定
- 私有方法跟随主调用域移动：`findDirectoryById` + `buildRepoFilterList` → `app_repometa.go`
- 每个拆分文件 `package main`
- 每个域文件顶部加分隔注释（如 `// ===== Git 操作域 =====`）
- 一次性拆完，三重验证收尾

## 拆分映射表

| 文件 | 域 | 方法数 | 关键方法 |
|---|---|---|---|
| `app.go` | core | 3 | App struct + NewApp + startup + shutdown + GetAppVersion |
| `app_directory.go` | 工作目录 | 8 | GetDirectories / AddDirectory / UpdateDirectory / DeleteDirectory / SetDefault / GetDefault / Reorder / RefreshGitFlag |
| `app_filetree.go` | 文件树+文件操作 | 6 | GetFileTree / GetFileTreeRecursive / CreateDirectory / CreateFile / RenameFile / DeleteFile |
| `app_git.go` | Git 操作 | 16 | GetGitInfo / GetGitLog / CloneRepo / PullRepo / ScanAndPullRepos / ExtractRepoName / GetGitRemoteURL / GetCommitHistory / GetLocalChanges / DiscardChanges / CommitFiles / PushRepo / GetFileDiff / HasUpstream / GetBranches / CheckoutBranch |
| `app_preview.go` | 文件预览+对话框 | 5 | PreviewFile / ReadFileBytes / SaveFile / SaveFileDialog / OpenFileDialog |
| `app_repometa.go` | 仓库元数据 | 7 | GetRepoFilterList / RefreshRepoFilterList / findDirectoryById(私有) / buildRepoFilterList(私有) / SaveRepoMeta / CleanMissingRepoMeta / GetRepoReadme |
| `app_external.go` | 外部工具打开 | 8 | OpenInExplorer / OpenInVSCode / OpenInWarp / OpenInObsidian / OpenObsidianVaultManager / CopyObsidianVaultPath / AutoRegisterAndOpen / OpenWithDefaultApp |
| `app_clipboard.go` | 剪贴板 | 6 | CopyItem / MoveItem / CopyTo / CopyToSystemClipboard / CutToSystemClipboard / ReadFromSystemClipboard |
| `app_settings.go` | 设置 | 2 | GetSettings / SaveSettings |
| `app_terminal.go` | 终端 | 6 | CreateTerminal / WriteTerminalInput / ChangeTerminalDir / ResizeTerminal / CloseTerminal / GetShellConfigs |
| `app_search.go` | 搜索 | 2 | SearchFiles / ContentSearch |
| `app_favorites.go` | 收藏夹 | 5 | GetFavorites / AddFavorite / RemoveFavorite / UpdateFavoriteAlias / UpdateFavoriteGroup |
| `app_update.go` | 更新 | 4 | CheckForUpdate / DownloadUpdate / CancelDownload / ApplyUpdate |
| `app_ai.go` | AI 功能 | 21 | GetAiFunctions / SaveAiFunctions / ExportAiFunctions / ImportAiFunctions / GetDiscoveredSkills / RefreshDiscoveredSkills / RunAiFunction / RunAiFollowUp / CancelAiTask / GetAiTaskState / GetAiConcurrencyStatus / RemoveAiTask / GetAiTaskOutput / GetAiTaskHistory / GetAiTaskHistoryOutput / DeleteAiTaskHistory / ClearAiTaskHistory / GetAiTaskHistoryStats / GetFunctionUsageCounts / ExportAiTaskHistoryCSV / ExportAiTaskHistoryMarkdown |

合计 14 文件，方法总数约 98（含 2 私有）。

## Acceptance Criteria (evolving)

- [ ] `go build ./...` 通过
- [ ] `go test ./...` 全过（app_test.go + app_repo_filter_test.go 现有用例不变）
- [ ] `grep -cE "^func \(.*App\)" app.go app_*.go` 方法总数 = 98（含 2 私有）
- [ ] `wails build` 通过，`frontend/wailsjs/` 绑定产物 `git diff` 为空
- [ ] 每个域文件 `package main`，顶部有分隔注释

## Definition of Done

- `go build` + `go test` + `wails build` 三重验证通过
- Wails 绑定零变更
- 现有测试用例全部通过
- 检查 README.md 是否需更新（项目 CLAUDE.md 要求）

## Out of Scope (explicit)

- 不改方法签名、不改逻辑、不改 Wails 绑定
- 不重构方法内部实现
- 不补新测试用例（属任务3「测试覆盖提升」）
- 不改 service / model 层
- 不拆 AI 域为多文件（已决定单文件）

## Technical Approach

一次性拆完再验证，不分批 PR。理由：

- 纯机械移动，无逻辑变更，风险相互独立
- 分批每次都要保证 build 通过，中间态 app.go 残留 + 新文件并存，import 管理更乱
- 一次拆完 → 三重验证收尾

git 域方法在 app.go 中散布 4 个区间（L226-238 / L324-393 / L648-697 / L993-1060），机械抽取易漏方法。拆前建方法清单 checklist，拆后用 `grep -cE "^func \(.*App\)" app_*.go` 核对总数。

## Decision (ADR-lite)

**Context**：app.go 1457 行单文件过载，需按域拆分；提示词原方案漏 5 域且 git 域少列 7 个本地操作方法，私有方法归属判断有误。

**Decision**：

1. `findDirectoryById` 归 `app_repometa.go`（调用者 `GetRepoFilterList` + `RefreshRepoFilterList` 均在该域，原提示词建议归 directory 有误）
2. AI 域单文件 `app_ai.go`，不过度拆分（21 方法 189 行，内部注释分段即可）
3. 每个域文件顶部加分隔注释

**Consequences**：

- repometa 单文件约 250 行仍偏重（含 2 个大私有方法），但约束「不改逻辑」禁止内部再拆，可接受
- AI 单文件 21 方法，可接受
- git 域跨 4 区间抽取，需 checklist 防漏

## Technical Notes

- 私有方法跨文件同 `package main` 可访问，无编译问题
- `app_repo_filter_test.go` 引用 `buildRepoFilterList` 私有方法，拆分后移到 `app_repometa.go`，测试仍可访问
- `app_test.go` 跨 core/directory/git 三域，无私有方法引用
- import 块逐文件按实际依赖补，优先 goimports 自动化，无则手动
