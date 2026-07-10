# fix: 本地变动未跟踪目录折叠导致显示不完整

## 目标（Goal）

修复右栏「本地变动」面板对 Git 仓库显示条目不完整的问题：未跟踪目录被折叠为单行，导致实际变动文件数远多于展示数。让面板如实展示工作区全部变动文件（尊重 `.gitignore`，被忽略文件仍不显示）。

## 复现与根因（已验证）

**复现仓库**：`D:/workspace/workspace_ai/all_in_ai`（用户传入的 `workspace_claudcode` 并非 git 根，`GetLocalChanges` 内部经 `util.FindGitRoot` 自动定位到上一级仓库根）。

**数据对比**（同一仓库、同一时刻）：

| git 参数 | 变动条目数 | 说明 |
| --- | --- | --- |
| `status --porcelain -z`（当前实现，默认 `--untracked-files=normal`） | 36 | 未跟踪目录被折叠为一行 `?? dir/` |
| `status --porcelain -z -uall`（`--untracked-files=all`） | 116 | 展开未跟踪目录内每个文件 |

- 默认模式下未跟踪条目 30 个，其中大量是目录（如 `?? workspace_claudcode/u05_VibeCoding/AICodingGuide/`）。
- `-uall` 模式下未跟踪条目 110 个，差值 80 全是被折叠进目录的未跟踪文件。
- 该仓库存在 `.gitignore`（49 字节），`-uall` 仍不显示被忽略文件，符合预期。

**根因**：`service/git.go:257` 使用 `git status --porcelain -z`，默认 `--untracked-files=normal` 会把未跟踪目录折叠。前端 `LocalChanges.vue` 直接渲染后端返回的 `result`，无截断，问题完全在后端取数。

## 假设（待确认）

- 用户期望「完整展示」= 展开未跟踪目录内的每个文件（与 IDE/SourceTree 等默认行为一致）。
- `.gitignore` 已忽略文件继续不显示（保持 git 语义）。

## 需求（Requirements，演进中）

- [R1] `GetLocalChanges` 返回全部变动文件，未跟踪目录内的每个文件单独成条。
- [R2] 仍尊重 `.gitignore`，被忽略文件不出现。
- [R3] 解析逻辑兼容 `-uall` 后的路径形态（中文/空格路径已由 `-z` 原样保留）。
- [R4] 修复重命名/复制（R/C）路径解析：`git status -z` 实测格式为 `XY <目标路径> NUL <源路径> NUL`（目标在前、源在后），目标路径在 `seg[3:]`，下一段为源路径（仅 `i++` 跳过，不取作 filePath）。面板须显示当前路径（目标），而非工作区已不存在的旧路径。

## 技术方案（Technical Approach）

**核心改动**：`service/git.go:257`

```go
// 由
output, err := s.gitCmd.Execute(gitRoot, "status", "--porcelain", "-z")
// 改为
output, err := s.gitCmd.Execute(gitRoot, "status", "--porcelain", "-z", "--untracked-files=all")
```

**连带复核**：

- `service/git.go:302 DiscardChanges`：复用 `GetLocalChanges` 构建 `untrackedSet`，再用 `git clean -fd -- <paths>` 回滚。展开后 `untrackedSet` 含目录内每个文件路径，`clean -fd` 逐个删除仍成立，需验证「回滚选中/全部」行为不变。
- `util/git.go:172 HasLocalChanges`：仅判空，折叠不影响，不改。

## 验收标准（Acceptance Criteria）

- [ ] 对上述复现仓库，面板显示变动条目数与 `git status --porcelain -uall` 一致（本例 116）。
- [ ] 未跟踪目录内的文件单独成行，状态标签为「未跟踪」。
- [ ] 勾选未跟踪文件 → 提交、回滚选中、全部回滚均正常工作（`clean -fd` 路径正确）。
- [ ] 双击查看差异对未跟踪文件仍可用（新文件 diff）。
- [ ] 重命名文件（`git mv old new`）在面板显示为目标路径 `new`；双击 diff / 回滚 / 提交均按新路径工作，不出现工作区已不存在的旧路径。
- [ ] 已忽略文件（如 `node_modules`）仍不出现。
- [ ] 新增/更新后端单测覆盖 `-uall` 解析（含未跟踪目录展开、中文路径）。

## 完成定义（Definition of Done）

- 后端单测通过（`go test ./...`）。
- 真实仓库手工验证条目数一致。
- 如行为对外可见，更新 `docs/功能说明.md` 与 `README.md`（按项目规范确认）。

## 决策（ADR-lite）

**Context**：本地变动因 git 默认折叠未跟踪目录而显示不完整（36 vs 116）。
**Decision**：采用方案 A——后端 `GetLocalChanges` 增加 `--untracked-files=all`，全量展开未跟踪目录，不加上限、不改前端展示形态。
**Consequences**：
- 正面：核心改动一行，立即如实展示全部变动，且尊重 `.gitignore`。
- 代价/风险：未配 `.gitignore` 的巨型仓库可能出现列表膨胀与 `git status` 变慢，留待后续按需加阈值。

**追加决策（scope 扩展，用户确认）**：实现期实测确认一处既有 bug——`git status -z` 重命名格式为「目标在前、源在后」（`R  new.txt\0old.txt\0`），而 `GetLocalChanges` 误用下一段（源路径 old.txt）覆盖 `seg[3:]`（目标 new.txt），导致重命名文件在面板显示为旧路径，双击 diff/回滚/提交因路径不存在出错。该 bug 与「本地变动路径正确性」同源、改动极小，经用户确认本任务顺带修复，不另开任务。

## 演进思考（Expansion）

1. **未来演进**：巨型仓库（未跟踪文件极多、且未配 `.gitignore`）下 `-uall` 会拖慢 `git status` 并使列表膨胀，后续可加上限阈值 + 提示，或分组（已暂存/已修改/未跟踪）。
2. **相关场景一致性**：`DiscardChanges` 回滚未跟踪文件的准确性；`isUntracked` 对目录路径的处理；左栏文件树对未跟踪文件的标记是否与右栏口径一致。
3. **失败/边界**：路径含中文/空格（`-z` 已处理）；重命名 `R/C` 在 `-uall` 下解析不变；超大未跟踪目录列表卡顿。

## 范围外（Out of Scope）

- 前端分组/折叠树展示（除非 MVP 选定）。
- 性能上限/截断策略（除非 MVP 选定）。
- 修改 `.gitignore` 语义或显示被忽略文件。

## 技术备注（Technical Notes）

- 关键文件：`service/git.go:249-299 GetLocalChanges`、`service/git.go:302-360 DiscardChanges`、`util/git.go:34 Execute`、`util/git.go:172 HasLocalChanges`、`frontend/src/components/LocalChanges.vue:148 loadChanges`。
- git 语义：`-uall`/`--untracked-files=all` 展开未跟踪目录但**不**显示被忽略文件。
