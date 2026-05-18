# Deferred Work

## 文件预览安全性

- `.env` 等含敏感信息的隐藏文件可被预览，密钥明文暴露在 UI 中。建议在 `IsPreviewable` 中排除 `.env`，或对含 `password`/`secret`/`key`/`token` 的文件内容做脱敏。
- `.ssh`、`.gnupg`、`.aws` 等敏感系统目录在文件树中可见且可执行删除/复制等操作。建议对系统关键路径做保护。

## Deferred from: code review of 1-1-three-column-layout-framework (2026-05-18)

- 跨 describe 块重复/不一致的 stub 定义 — 所有 describe 块各自定义 stub，存在微妙差异（如 el-tree stub 在不同块中定义不同），属于预存模式问题
- Stub 耦合过高（PascalCase 硬编码）— 测试将 Element Plus 组件名硬编码为 stub 键，若组件名变更则测试失效，属于预存测试模式

## Deferred from: code review of 1-2-directory-add-remove (2026-05-18)

- Update 方法缺少测试覆盖 — 非本 Story AC 范围（重命名/更新属于 DirectoryTree.vue 附加功能），建议在后续 Story 中补充

## Deferred from: code review of 1-3-default-directory-persistence (2026-05-19)

- setup.js UpdateDirectory mock 返回 `true` 但 app.go 实际返回 `*model.Directory`（对象类型），mock 类型不一致，非本 Story AC 范围
- Update 方法缺少后端测试覆盖（同 1-2 遗留项，仍未覆盖）

## Deferred from: code review of 2-1-file-tree-lazy-loading (2026-05-18)

- HasChildren 对空目录返回 true（设计决策：懒加载模式后端无法确定目录是否为空，统一设为 true，前端二次修正）
- 前端 hasChildren=false isLeaf 测试场景生产中不可能发生（后端 HasChildren 始终为 true）
- buildTree 静默吞没子目录错误 [service/filetree.go:98-105]
- gitRepoCache sync.Map 无淘汰/过期机制 [service/filetree.go:17-18]
- 前端路径分隔符处理脆弱 [FileTreePanel.vue:518-521,554-557]
- 缺少 GetTree/buildTree 递归测试 [service/filetree.go:82-108]
- 缺少 AC3 性能基准测试
- 前端测试未验证端到端排序/过滤契约
- isGitRepoDir 不识别 git worktree（.git 为文件而非目录）[service/filetree.go:28-36]
- app.go GetFileTree 吞没错误 [app.go:111-118]
- 缺少符号链接和权限拒绝场景测试

## GetTree 递归性能

- `node_modules`、`.cache`、`vendor` 等大型目录在递归展开时可能导致性能问题。建议增加排除规则或子节点数量上限。
