# 内嵌冲突解决契约（合并/变基/拣选冲突面板，第一期）

> 2026-10-26，任务 `10-26-conflict-resolve-inapp`。外部三向工具链路（git_mergetool.go）原样保留，本期是标记块级内嵌解决的增量；CodeMirror MergeView 三向合并为二期非目标。

## 方法签名（3 个新 App 绑定）

```
ChooseConflictSide(path, file, side string) error            // side ∈ ours|theirs；checkout --<side> + git add 单链路
GetConflictFileContent(path, file string) *model.ConflictFileContent  // {content, encoding, binary, tooLarge}
SaveConflictResolution(path, file, content, encoding string) error    // 按原编码写回 + git add
```

## 硬约束与语义

- **变更类走 `tryLockRepo`**（ChooseConflictSide/SaveConflictResolution），`GetConflictFileContent` 只读不抢锁（对齐 GetConflictState）
- **冲突列表成员校验前置**：三方法都先确认冲突态在途（MERGE_HEAD/REBASE_HEAD/CHERRY_PICK_HEAD，同 GetConflictState 口径）且文件在 `git diff --name-only --diff-filter=U` 列表内，否则中文报错——防止对已解决/未冲突文件误操作
- **路径防护 `isSafeRelativePath` 故意从严**：拒绝 `..` 子串与全部冒号（只门禁 git 自己返回的冲突路径，误杀面可接受）；空格/中文路径放行
- **大小/二进制口径**：读侧原始字节 >1MB ⇒ `TooLarge`（内容留空）；二进制判定与预览同口径；写侧复用 `FileOperationService.SaveFile`（原子写 + GBK 编码，继承其 1MB 内容上限）——**已知缝隙**：GBK 683KB–1MB 区间文件解码后可能超写侧上限报「内容超过1MB限制」，罕见且有明确提示，接受
- **编码往返**：读侧用预览同源编码检测（UTF-8/GBK），content 恒为解码后 UTF-8，原编码随 `encoding` 回传，写侧按原编码写回不改变编码
- **rebase 的 ours/theirs 语义反转**：后端按字面传 `--ours`/`--theirs`；「以当前为准」在 rebase 冲突中指变基目标侧——UI 文案已按冲突类型附提示，后续改动不得丢该提示

## 前端标记解析契约（utils/conflictMarkers.js）

- 行首匹配（列 0）：`<<<<<<< ` 开块 → 可选 `||||||| ` base 段（diff3）→ `=======` 分隔 → `>>>>>>> ` 收块；游离 `=======` 是普通行；EOF 未闭合块防御性吸收
- **行尾 `\r` 必须保留**：解析按 `\n` 切分、`\r` 归属行尾，重组 `\r\n` 文件须逐字节还原（分隔行正则须容忍 `\r?`，有回归测试）；块间上下文经每块 `lead` 行保留（多块文件合并结果不丢序）
- 无标记文件（`hasMarkers=false`）直接进全文编辑模式（外部改干净后回写场景）
- 保存软校验：合并结果仍含行首 `<<<<<<<`/`>>>>>>>` 时确认框提醒，可强制保存

## 冲突动线（ContentPanel）

三个触发点检测 `GetConflictState`（无轮询）：选中 git 仓库节点（watch immediate）、单仓拉取完成（activePullPath 快照防中途导航串台）、批量更新完成（pull-complete 事件，作用于当前选中节点）；命中 ⇒ `activeGitTab='merge'` + warning 提示 + 「合并/变基」tab `el-badge is-dot`；GitMerge 每次 `refreshConflictState` 后 emit `conflict-state` 供角标实时更新。

## 测试分层

- 单测：`service/git_conflict_resolve_test.go`（路径防护/side 校验/成员拒绝）；`conflictMarkers.spec.js` 20 用例（含 CRLF 逐字节往返 4 条回归）
- 集成：`conflict_resolve_integration_test.go`（ours/theirs + ContinueMerge 闭环、GBK 字节级回写、二进制冲突选边、>1MB TooLarge、路径逃逸、非冲突仓拒绝）
- mock 单一数据源 `wails-mock-defaults.js` +3；models.ts 须含 `ConflictFileContent` 类（cross-layer-contracts 三处同步）
