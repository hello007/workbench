# 文件级提交历史

## Goal

CommitHistory 只有仓库级历史（已核对 2026-11-07），单文件演进史无法追查；重命名文件历史断链（需 --follow）。提供文件级提交历史：文件树右键「查看历史」→ 该文件的 commit 列表（可查看任一条的 diff），与既有仓库级 CommitHistory 形成追溯闭环。

## Decisions（2026-11-07 已敲定）

| 决策点 | 结论 |
|---|---|
| 入口形态 | 仅文件树右键（FileTreePanel 文件节点菜单加「查看历史」项），预览工具栏入口后续按需补 |
| 组件形态 | 轻量独立 `FileHistoryDialog.vue` 弹窗：SHA/作者/时间/消息列表 + rename 标记 + FileDiffDialog 看 diff，不搬 BranchSyncBar/AI review/range diff 等仓库级功能 |
| 目录入口 | 本任务不做目录级入口（--follow 仅支持单一路径）；目录场景用户可在既有「提交历史」tab 用 filePath 过滤框 |
| rename 展示 | 提交行加「R」小标记，tooltip 显示「old → new」 |

## 现状核对（2026-11-07）

- `GetCommitHistory(path, limit, offset, CommitFilter)` 已有分页 + author/keyword/since/until/filePath 过滤；`CommitFilter.FilePath` 是目录级子串匹配、无 --follow，**不能直接复用**——文件级历史需新增后端方法（--follow 精确单路径）
- `CommitHistory.vue` 是重组件（BranchSyncBar 集成、AI review、range diff 勾选、refs badge），均为仓库级专属，参数化复用得不偿失（已否决）
- 右键扩展点：`FileTreePanel.vue:1532 onMenuCommand`，文件节点分支已有 openExplorer/copyPath 等先例
- diff 展示复用 `FileDiffDialog`（关闭释放双时机契约见 perf-baseline.md §16.5）

## Requirements

1. **后端**：新增 App 方法（如 `GetFileCommitHistory(repoPath, filePath string, limit, offset int)`）：
   - git 调用自带 UTF-8 `-c` 前缀（`i18n.logOutputEncoding=UTF-8 -c i18n.commitEncoding=UTF-8 -c core.quotepath=off`，`GitCommand` 不默认带）
   - `git log --follow --name-status` + `%x00` 记录 `%x01` 字段分隔（branch-compare.md 规范）
   - 返回 `model.FileCommit`（或复用 Commit + 扩展 rename 字段），含 rename 检出（R 行 old→new）
   - 分页返回 + 总数（或 hasMore 标志），与 GetCommitHistory 分页语义对齐
   - wailsjs 三处手动同步（App.js/App.d.ts/models.ts）
2. **前端**：`FileHistoryDialog.vue` + FileTreePanel 右键接线：
   - 右键「查看历史」→ 弹窗加载该文件 commit 列表（倒序），滚动分页或加载更多
   - 任一提交点击 → FileDiffDialog 展示该提交对文件的 diff
   - rename 提交行「R」标记 + tooltip「old → new」
   - 中文名文件全程无乱码（后端 UTF-8 前缀 + 前端 decodeBase64Utf8 规范如有涉及）
   - 事件监听闭包注销、组件卸载复位加载态
3. **测试**：组件单测（列表渲染/rename 标记/分页/diff 触发）+ 后端 service 层测试（中文路径、rename 追溯、分页）；覆盖率门禁遵循 test-coverage-gate.md

## Acceptance Criteria

- [ ] 文件树右键「查看历史」→ FileHistoryDialog 列表 + 任一提交 diff 查看
- [ ] 中文名文件历史无乱码；重命名历史不断链（--follow 生效）
- [ ] rename 提交行「R」标记 + tooltip 展示 old → new
- [ ] 组件单测 + service 测试 + 契约同步（wailsjs 三处）无回归
- [ ] `go vet ./...`、`go test ./...`、`npm run build`、`npm test` 全绿

## Technical Notes

- 调研定位：高价值候选 #5，与候选 #4（blame）同属追溯域，blame 未立项可后续接力
- 文件级历史按需拉取（弹窗打开时请求），不走 CommitHistoryCache（该缓存按 gitRoot+headRef 存全量，与 --follow 单路径语义不匹配）
- 服务层新增独立文件（如 `service/file_history.go`），不与 stash/commit history 现有文件耦合；若需 AppServices 装配遵循 app-services-assembly.md
