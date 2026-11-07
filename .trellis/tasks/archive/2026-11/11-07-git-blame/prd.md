# 行级归属追溯（git blame）

## Goal

代码预览场景提供行级归属：查看每行最后修改的提交（作者/时间/短 SHA/消息），点击行的归属提交可跳转该文件的提交历史并定位该提交（衔接 FileHistoryDialog），形成「仓库 → 文件 → 行」三级追溯闭环。

## Decision (ADR-lite)

**Context**：展示形态、大文件策略、未提交行、JSON 错位四项存在多方案取舍。

**Decision**（2026-11-07 用户拍板「全推荐」）：

1. **展示形态 = 行尾标注**（VS Code GitLens 式）：开关后每个 blame 连续块首行行尾追加灰色小字「作者 · 相对时间 · 短SHA」，CodeMirror Decoration 实现，零布局改动；点击标注跳转文件历史
2. **大文件策略 = 整文件 + 行数上限**：一次 `git blame` 全文件，超 10000 行（对齐 fileHistoryMaxFetch 数量级）降级——新增错误码走 warning 分流提示「文件过大，行归属不可用」；`-L` 视口懒加载出范围
3. **未提交行 = 工作树 blame + 标「未提交」禁跳转**：零 SHA 行（git 契约 000…0）展示「未提交」标识，点击不跳转；不采用 blame HEAD（行号与预览错位）
4. **JSON 错位 = 对 .json 隐藏入口**：JSON 预览是 pretty-print 衍生视图，行号与文件无对应，blame 语义不成立；菜单项对 .json 不显示

**Consequences**：超大文件（>1 万行）与压缩 JSON 无 blame 能力（可接受的边缘降级）；后续如需历史版本 blame，接口签名预留 fileAbsPath 自包含即可平滑扩展 rev 参数。

## Requirements

### 后端（service/git_blame.go，GitService 方法）

* `GetFileBlame(fileAbsPath)`（签名实现期定）：fileAbsPath 自包含——`util.FindGitRoot` + `filepath.Rel` + `:(literal)` pathspec + `filepath.ToSlash` 归一
* git 调用：`git blame --porcelain`（不传 rev = 工作树 blame）+ `utf8Prefix()`；30s `gitCmd.Execute` 超时兜底；只读不抢 tryLockRepo
* 解析：porcelain 块头 `<40位sha> <origLine> <finalLine> [<count>]` 机械可解析；author/author-mail/author-time <unixTs> <tz>/summary/filename 字段；`\t` 行内容跳过不消费（前端已有 content，GBK 字节无影响）
* 模型（model/ 契约真相源）：`BlameCommit`（SHA/ShortSHA/Author/Email/Message/DateTime/Timestamp，DateTime 按作者时区墙钟对齐 FileCommit.DateTime 口径）+ `BlameResult`（Commits 去重列表 + Lines 每行→提交下标数组，与文件行号 1:1）；零 SHA 语义由前端按 `^0+$` 判定（git 契约稳定，不加冗余字段）
* 行数上限 10000：超限返回 `E_BLAME_FILE_TOO_LARGE`（warning 分流），不截断输出
* 错误矩阵对齐 file-level-history.md §5：空路径 / 不在仓库内 / 非仓库目录 / git blame 失败；解析 fail-soft（脏记录跳过 + Warn）
* 未提交新文件（全未跟踪）行为实现期验证：预期全行未提交；若 git 报错则归入错误矩阵降级

### 前端（FilePreviewRenderer + FileHistoryDialog）

* 入口：右键菜单「行归属」开关项，仅 `kind === 'text'` 且扩展名非 json/jsonc/json5 时显示；开关状态随 filePath 切换/组件卸载复位
* 行尾标注：CodeMirror Decoration（StateField + 行尾 widget），连续同提交块首行展示「作者 · 相对时间 · 短SHA」；零 SHA 块展示「未提交」；颜色走设计令牌（蓝灰 Slate，暗色对比度合规）；相对时间前端格式化（Timestamp unix ts）
* 点击联动：点击标注 → FileHistoryDialog 打开（renderer 内自挂载，`repoPath == fileAbsPath` 对齐 FileTreePanel 先例）并定位该提交
* FileHistoryDialog 新增定位 prop（initialSha）：打开后逐页加载直到命中（上限约 500 条）滚动+高亮该行；超上限未命中提示「该提交不在最近 500 条提交中」；复用既有 loadSeq 代次守卫
* 行数守卫：blame 返回行数 ≠ 编辑器文档行数时 warning 提示「文件已变更，请刷新预览」（防预览读取与 blame 执行间外部修改）
* 契约三处同步（App.js/App.d.ts/models.ts）；错误码三处同步（app_error.go + error.js）；handleError 分流

### 验证

* 覆盖率门禁：service ≥76% / model ≥80% / 前端 ≥70%
* Go 测试：porcelain 解析、中文作者/消息往返、glob 元字符文件名、零 SHA、行数上限、错误矩阵（util/testutil InitTempRepo，禁重复定义辅助）
* 前端测试：菜单开关条件（text+非 json）、标注渲染与块首行收敛、零 SHA 禁跳转、点击联动传参、FileHistoryDialog 定位（含未命中）、行数守卫

## Acceptance Criteria

* [ ] 预览文本文件右键可开关行归属，连续块首行展示「作者 · 相对时间 · 短SHA」
* [ ] 点击归属提交打开 FileHistoryDialog 并滚动定位高亮该提交
* [ ] 未提交行标「未提交」且点击不跳转
* [ ] 中文作者/中文消息往返无乱码；glob 元字符文件名（笔记[1].md）可 blame
* [ ] >10000 行文件返回 E_BLAME_FILE_TOO_LARGE，前端 warning 提示
* [ ] .json 文件不显示行归属入口
* [ ] blame 行数与预览行数不一致时提示刷新
* [ ] filePath 切换/组件卸载后 blame 态复位，无串入
* [ ] go vet / go test / npm run build / npm test 全绿，覆盖率达标

## Definition of Done

* 单测按上述验证清单覆盖
* docs/spec 沉淀 blame 域契约（porcelain 解析、零 SHA 语义、对齐守卫、定位联动），AGENTS.md 关键规则表加行
* README.md 若涉及预览功能描述则同步

## Out of Scope

* 指定历史版本 blame（rev 参数）、编辑态 blame
* `-L` 视口懒加载、`-M`/`-C` 跨文件移动行检测
* 行级作者统计/聚合视图
* AiChatPanel 挂载面（无组件挂载，不涉及）

## Technical Notes

* 探索记录（2026-11-07）：codegraph_context + codegraph_explore + git_file_history.go 全文 + file-level-history.md 契约 + FilePreviewRenderer props/CodeMirror 初始化 + ContentPanel 挂载面
* FilePreviewRenderer 已有右键菜单（onContextMenu 仅 kind=text 启用）与 CodeMirror 只读编辑器（lineNumbers/foldGutter），Decoration 叠加不影响既有复制/全选
* ContentPanel 编辑态走 v-else 分支，编辑中无 renderer，天然无入口；保存后 content watch 触发 blame 整组复位（防旧归属串入），用户重开即重拉
* RepoFilterDialog.vue 中 FilePreviewRenderer 字样为注释/非预览挂载（实现期核实）

### 实现期验证结论（2026-11-07，三条假设证伪）

* ~~未跟踪文件预期全行未提交~~ → 未跟踪（未 add）文件 `git blame` 直接 fatal（`no such path in HEAD`），走错误矩阵；已 add 未 commit 文件正常输出全零 SHA；unborn 空仓（零提交）blame 直接 fatal
* ~~porcelain `author-time <ts> <tz>` 同行~~ → 实际分行：`author-time <ts>` + 独立行 `author-tz <tz>`（实现按分行解析，到齐合成墙钟）
* ~~blame pathspec 可加 `:(literal)`~~ → blame 路径参数是字面路径（非 pathspec 机制，与 git log 不同），`:(literal)` 反被当作字面文件名导致 fatal；裸路径即精确命中 glob 元字符文件名
* vitest 游离树教训：mount 未 `attachTo` 时组件渲染在游离 DOM（isConnected=false），事件冒泡到不了 document——blame 标注点击（document 委托）用例必须 `attachTo: document.body`
