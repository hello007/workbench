# 行级归属追溯契约（git blame）

> 来源：11-07-git-blame 任务（2026-11-07）。域入口 `service/git_blame.go`（GitService 方法 `GetFileBlame`），App 绑定 `GetFileBlame`（app_git.go），前端 `FilePreviewRenderer.vue`（右键「行归属」开关 + CodeMirror 行尾标注）→ `FileHistoryDialog.vue`（`initialSha` 定位跳转）。与文件级历史（git_file_history.go）同构：fileAbsPath 自包含 + FindGitRoot + utf8Prefix，形成「仓库 → 文件 → 行」追溯闭环，但不共享缓存与解析。

## 1. 绑定与模型契约

| 绑定 | 签名 | 说明 |
|---|---|---|
| `GetFileBlame(fileAbsPath)` | `*model.BlameResult` | **入参为文件绝对路径**（单参数自包含）：后端 `FindGitRoot` 定位仓库根并 `filepath.Rel` 换算仓库相对路径；前端预览文件直传 `props.filePath` |

`model.BlameResult`：

- **`Commits []BlameCommit`**：去重提交列表（porcelain 首次出现序），字段 SHA/ShortSHA/Message/Author/Email/Timestamp/DateTime。DateTime = 作者时区墙钟（口径对齐 FileCommit.DateTime）
- **`Lines []int`**：每行 → Commits 下标，**下标 = 文件行号-1**，与工作树文件行号 1:1。前端行数守卫依赖此对齐语义

**零 SHA 语义**：未提交行（工作树 blame）归属 SHA 为全零（git 契约），后端不加冗余标志字段，前端按 `^0+$` 判定并禁跳转（标「未提交」）。

wailsjs 三处已手动同步（App.js 单参绑定 / App.d.ts `Promise<model.BlameResult>` / models.ts BlameCommit + BlameResult class，BlameResult 含嵌套 convertValues）。

## 2. git 调用契约（坑位，与 log 域的差异）

```
git -c i18n.logOutputEncoding=UTF-8 -c i18n.commitEncoding=UTF-8 -c core.quotepath=off \
    blame --porcelain -- <relPath>
```

- **路径参数是字面路径，禁加 `:(literal)`**（本域最重要差异）：blame 不走 pathspec 机制，`:(literal)x.txt` 会被当作字面文件名直接 fatal `no such path`；裸路径即按字节精确命中，glob 元字符文件名（笔记[1].md）天然安全。**与 git_file_history 的 log 域相反，勿混淆**
- **工作树 blame（不传 rev）**：输出行号与预览内容恒对齐；不用 blame HEAD——文件有未提交增删行时行号错位
- **porcelain 详情分行**：`author-time <unixTs>` 与 `author-tz <tz>` 是**两行**（非同行 `<ts> <tz>`）；DateTime 须两行到齐后按 `ts + tz偏移` UTC 格式化合成（非本地时区，避免转换漂移）
- **行内容 `\t` 前缀行跳过不消费**：前端已有 content；GBK 文件行内容为原始字节，不消费即无乱码风险
- **大文件双保险**：前端不预判（后端权威），后端 bufio 流式数行超 `blameMaxLines=10000` 直接返回 `E_BLAME_FILE_TOO_LARGE`（warning），不跑 blame 零进程开销
- 只读不抢 tryLockRepo、不注册取消（对齐 GetFileCommitHistory 只读先例）；30s 超时由 `gitCmd.Execute` 兜底
- **行为边界（实测）**：未跟踪（未 add）文件 fatal（`no such path in HEAD`）→ 走错误矩阵；已 add 未 commit 文件正常输出全零 SHA；unborn 空仓（零提交）fatal

## 3. 解析分层容错

porcelain 输出按行状态机解析（`blameSHAHeaderRe`：40 位 hex + orig/final 行号 + 可选 count）：

- **sha 行头非法 / 存在未归属行（跳号）→ 整体报错**（fail-fast）：porcelain 为纯机械格式，输出契约破坏时行号对齐不可信，静默错位比失败更糟；「有输出但零行头记录」同报错（与空文件区分：`TrimSpace(out)==""` 才是合法空结果）
- **单条详情字段脏（author-time 非法）→ Warn 跳过该字段**，提交条目保留（fail-soft，对齐 parseFileCommitHistory 先例）
- `boundary` / `previous` / `filename` / `committer*` 等未消费字段静默跳过（防御 git 新版本新增字段）

## 4. 前端契约（FilePreviewRenderer）

- **入口条件 `blameAvailable`**：`kind==='text'` && 非 markdown && 非 HTML 渲染视图 && 扩展名非 json/jsonc/json5——JSON 预览是 pretty-print 派生视图（显示行号与文件行号无对应），blame 语义不成立，**入口隐藏**（不采用「开启时按原文渲染」方案）
- **展示形态**：blame 连续块（`lines[i] !== lines[i-1]`）仅块首行行尾 widget decoration，文案「作者 · 相对时间 · 短SHA」（零 SHA →「未提交」斜杠弱化），title 悬浮给全信息（作者/墙钟时间/消息首行）
- **装饰注入机制**：`blameComp`（Compartment）+ `EditorView.decorations` + `RangeSetBuilder`，开关/数据更新走 `reconfigure` **不重建编辑器**（滚动位置/折叠状态保留，与 `cmHighlightCompartment` 同机制）；编辑器因内容变化重建时尾部重放（htmlMode 切回源码态场景）
- **点击链路**：widget `ignoreEvent()=false` + **document 级 click 委托**（`closest('.blame-annot')`）；带 sha 才跳转（未提交无 dataset.sha）。**handler 首行须做实例域判定**（`previewRootRef.contains(e.target)`）：组件存在双挂载面（ContentPanel 主预览 + RepoFilterDialog README 渲染），两实例都注册 document 委托而 closest 是全局 DOM 查询与实例无关，漏判定则点击本实例标注连带打开他实例弹窗。弹窗已开时先关后开（nextTick）走完整重挂载链路，确保新 sha 触发定位
- **跳转出口**：FileHistoryDialog **renderer 内自挂载**，`repoPath == fileAbsPath == props.filePath`（对齐 FileTreePanel 先例），`initialSha` 传目标提交
- **行数守卫**：blame 行数 ≠ `cmView.state.doc.lines` → warning「文件已变更…请刷新」并整组复位（预览读取与 blame 执行间文件被外部修改的对齐防御，禁静默错位标注）
- **状态复位**：`watch([kind, fileName, content, filePath])` 变化即 `resetBlameState()`（blameSeq++ 失效在途响应 + 清装饰）；htmlMode 不在列（同文件视图切换，数据面有效）

## 5. FileHistoryDialog 定位联动（initialSha）

- 新增 prop `initialSha`（空串不定位）；打开（watch modelValue immediate）首页加载完成后执行 `locateCommit`
- **定位扫描**：目标提交不在已加载页时逐页 `loadHistory(false)` 追加，上限 `locateMaxCommits=500` 条；命中滚动居中（`scrollIntoView block:center`）+ `is-located` 高亮（主色左条 + 淡主色底，历史行新增 `data-sha` 属性）；超限未命中 `ElMessage.info('该提交不在最近 N 条提交中')`
- **中止判定用 `modelValue` 而非 loadSeq**：loadHistory 内部自增 loadSeq，外部保存的代次号每轮失效不能复用；循环另设「页数不增长即停」兜底，防失败页 hasMore 恒真死循环
- releaseState 清 `locatedSha`

## 6. 测试要点

- **端到端铁证**：真实 git 仓库两次提交 → 前两行归属首建提交、第三行归属追加提交（Lines 映射 + Commits 去重 + 中文往返）；工作树修改后修改行零 SHA、未修改行归属不变
- **手写 porcelain fixture 矩阵**：连续块 count 字段、详情仅首行出现、CRLF、零 SHA 未提交块、author-time 脏字段 fail-soft、sha 行头脏 fail-fast、跳号报错、空输出
- **路径契约**：glob 元字符文件名裸路径命中（锁「禁 :(literal)」）；`:(literal)` 前缀必 fatal（回归哨兵）
- **时区矩阵**：+0800 / -0530 / +0000 / 非法 tz 回退 UTC（`blameDateTime` 纯函数直测）
- **前端**：入口条件（json/markdown 隐藏）、块首行标注收敛、零 SHA 禁跳转、点击带 sha 打开 stub 并传 initialSha、行数守卫 warning、切换文件复位
- **前端挂载教训（假绿陷阱）**：mount 未 `attachTo` 时组件渲染在**游离 DOM**（isConnected=false），事件冒泡到不了 document——document 级委托监听（blame 标注点击）永远收不到事件。点击委托类交互的用例必须 `attachTo: document.body`
