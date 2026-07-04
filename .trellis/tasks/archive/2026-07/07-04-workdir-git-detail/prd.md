# 左栏选中 git 工作目录时右栏显示仓库详情

## Goal

当用户在左栏"工作目录树"（`DirectoryTree`）选中一个**本身是 git 仓库**的工作目录时，右侧操作面板（`ContentPanel`）直接显示该仓库的 git 详情（仓库信息 / 提交历史 / 本地变动，与在中栏文件树选中一个 git 仓库节点的显示完全一致），无需用户再到文件树里点一次仓库节点。

## What I already know（已探明的事实）

### 当前布局与数据流

```
[ActivityBar] | [DirectoryTree 工作目录] | [FileTreePanel 文件树] | [ContentPanel 操作面板]
```

- `DirectoryTree`：点击工作目录项 → `emit('select', dirId)` → `Home.onDirectorySelect(dirId)`。
- `Home.onDirectorySelect`：切换 `selectedDirectoryId` → **`selectedNode = null`** → `clearPreview`。
- `ContentPanel`：据 `selectedNode` 渲染：
  - `selectedNode.isGitRepo === true` → 显示 Git 操作按钮 + `el-tabs`（仓库信息 / 提交历史 / 本地变动）；
  - `selectedNode.type === 'directory'` → 文件夹操作区；
  - `selectedNode.type === 'file'` → 文件操作 + 预览；
  - `selectedNode === null` → 空状态"请从左侧选择"。
- `FileTreeNode`（文件树节点）**有** `IsGitRepo` 字段，文件树展开时由后端检测填充；`Directory`（工作目录）**没有**该字段。

### 后端现状

- `model.Directory{ID, Name, Path, IsDefault, CreateTime}` —— **无 IsGitRepo 字段**。
- `DirectoryService.Load()` 仅从配置文件 `data/` 读 `[]*Directory`，**不检测 git**。
- `app.go:GetDirectories` 直接返回 Load 结果。
- 已有 `util.GitCommand.IsGitRepository(path)` 与 `service.GitService` 可复用做检测。

## Assumptions（待验证）

- 用户所说"工作目录是 git 仓库"指**工作目录路径本身就是 git 仓库根**（`IsGitRepository(dir.path)`），而非"目录下含 git 仓库"。
- 期望复用 ContentPanel 现有的 git 详情渲染（不另造一套）。
- 工作目录不是 git 仓库时，保持现状（空状态或现有行为），不报错。

## Open Questions（仅阻塞/偏好类）

1. **[已定]** 实现方案 = **后端 `Directory` 加 `IsGitRepo` 字段 + `GetDirectories` 检测填充 + 前端构造 `selectedNode` + 左栏 git 标记**。✅
2. **[已定]** 左栏工作目录项显示 git 小图标（仅 git 仓库显示），与文件树 git 仓库节点视觉一致。✅

## Decision (ADR-lite)

**Context**：工作目录是否 git 仓库是其持久属性；用户希望左栏选中后右栏直接显示 git 详情，且与文件树选中 git 仓库的体验一致。

**Decision**：采用"后端字段 + 左栏标记"方案——`model.Directory` 加 `IsGitRepo`，`DirectoryService.Load`（或 `app.go:GetDirectories`）在返回前用 `util.GitCommand.IsGitRepository` 检测填充；前端 `Home.onDirectorySelect` 命中 git 工作目录时构造 `{path, name, type:'directory', isGitRepo:true}` 的 `selectedNode` 复用 ContentPanel 现有 git 详情；`DirectoryTree` 工作目录项在 git 仓库时显示小图标。

**Consequences**：
- 优点：数据层体现持久属性、前端同步无异步闪烁、左栏标记提升识别度、完全复用 ContentPanel 现有 git 渲染。
- 代价：每次 `GetDirectories` 对所有工作目录各执行一次 `git rev-parse --git-dir`（工作目录通常 <10，开销可接受）；新增字段需保证旧配置反序列化兼容（Go json 零值 false，天然兼容）。
- 边缘：检测失败按"非 git 仓库"处理（IsGitRepo=false），不报错。

## Requirements（演进中）

- 点击左栏工作目录 → 若该目录是 git 仓库 → ContentPanel 显示该仓库的 git 详情（仓库信息 / 提交历史 / 本地变动 + 拉取 / 切换分支按钮），与文件树选中 git 仓库节点一致。
- 工作目录不是 git 仓库 → 保持现有空状态行为。
- 在文件树里选中其他节点时，仍按文件树节点渲染（现有行为不回归）。

## Acceptance Criteria（演进中）

- [ ] 选中一个 git 仓库工作目录 → 右栏出现"仓库信息/提交历史/本地变动"tab 与拉取/切换分支按钮，数据对应该仓库。
- [] 选中一个非 git 工作目录 → 右栏保持空状态（不报错、不闪现 git 面板）。
- [ ] 选中工作目录后，再到文件树点其他节点 → 右栏正常切换为该节点详情（不回归）。
- [ ] 切换不同工作目录（git ↔ 非 git）→ 右栏正确切换显示。
- [ ] `go test ./...` 与 `npm test` 全绿。

## Definition of Done（团队质量基线）

- 后端如有改动（Directory 字段 / 检测）补单测。
- 前端通过 vitest（Home/ContentPanel 相关用例无回归）。
- 行为变更后确认是否需要更新 `README.md` / `docs/功能说明.md`。

## Out of Scope（明确排除）

- 工作目录下子目录的 git 扫描（已有"更新仓库"批量拉取覆盖多仓库场景）。
- 左栏工作目录项的额外信息（分支名、未提交数等）——除非选方案含左栏标记。
- ContentPanel git 详情本身的样式调整（上轮已修）。

## Technical Notes

- 关键文件：
  - 后端：`model/models.go`（Directory）、`service/directory.go`（Load/Create）、`app.go`（GetDirectories）、`util/git.go`（IsGitRepository）
  - 前端：`frontend/src/views/Home.vue`（onDirectorySelect）、`components/DirectoryTree.vue`、`components/ContentPanel.vue`
- ContentPanel 已能据 `selectedNode.isGitRepo` 渲染 git 详情；只需让 Home 在选中 git 工作目录时，构造一个 `{path, name, type:'directory', isGitRepo:true}` 的 selectedNode 传入即可复用。
- 检测成本：`IsGitRepository` = `git rev-parse --git-dir`，单次很快；工作目录通常 <10 个，全量检测可接受。
