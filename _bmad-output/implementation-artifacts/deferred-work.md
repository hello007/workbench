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

## Deferred from: code review of Stories 2-2, 2-3, 2-4, 3-1 (2026-05-21)

- **[F1]** `_ = info` 代码异味 [service/filetree.go:34] — `os.Stat` 返回值被丢弃，仅用 `err == nil` 判断存在性。intentional for worktree support
- **[F2]** 缺少负缓存（false 值）测试 [service/filetree_test.go] — CacheHit 只验证 true 缓存命中，未验证 false 缓存
- **[F3]** gitRepoCache 无过期机制 [service/filetree.go:17] — sync.Map 永不过期（与 F27 from 2-1 重复，已记录）
- **[F4]** handleRename/handleDeleteAt 根目录子项下 parentPath 提取失败 [FileTreePanel.vue:590-593]
- **[F5]** handleRename 不阻止重命名为相同名称 [FileTreePanel.vue:577]
- **[F6]** cloneRepo 用 `result.includes('成功')` 判断成功 [ContentPanel.vue:280] — 脆弱的字符串匹配
- **[F7]** ContentPanel filePreview 切换节点不清除 [ContentPanel.vue:160-161] — 无 watch selectedNode
- **[F8]** ContentPanel activeGitTab 切换节点不重置 [ContentPanel.vue:197]
- **[F9]** FileTreePanel.vue 导入 EventsOn/EventsOff 但未使用 [FileTreePanel.vue:265] — 死代码
- **[F10]** loadTreeNode 错误处理不一致 [FileTreePanel.vue:373] — `error.message || error` vs `String(error)`
- **[F11]** 右键菜单无视口边界检查 [FileTreePanel.vue:148,449-450]
- **[F12]** ContentPanel 无 isGitRepo 相关测试 [ContentPanel.spec.js]
- **[F13]** handleCreate 不验证文件名特殊字符 [FileTreePanel.vue:534] — 路径遍历风险
- **[F14]** HasChildren 对空目录默认 true [model/models.go:59] — 与 F24 from 2-1 重复
- **[F15]** Story 2-3 AC4 隐藏项可操作性未显式测试 [FileTreePanel.spec.js]
