# 仓库筛选器：文件树 Git 仓库管理弹窗

## 目标

解决 FileTreePanel 右侧文件树中 Git 仓库数量膨胀不便查看的问题，提供仓库筛选、标签管理、简述编辑等功能，帮助用户快速定位和管理嵌套仓库。

## 我已了解的内容

### 现有代码结构

**前端组件：**
- `DirectoryTree.vue` - 左侧工作目录树（管理顶级工作目录条目）
- `FileTreePanel.vue` - 右侧文件树（懒加载模式，展示选中工作目录下的文件/目录树）
- 文件树节点有 `isGitRepo` 和 `hasRemote` 属性标识 git 仓库

**后端服务：**
- `DirectoryService` - 工作目录的 CRUD
- `model.Directory` - 工作目录数据模型（持久化到 JSON 文件）
- `model.FileTreeNode` - 文件树节点模型（运行时懒加载）
- `util.GitCommand.IsGitRepository()` - 检测目录是否为 Git 仓库
- `app.go.ScanAndPullRepos()` - 扫描目录下所有 Git 仓库并批量拉取

**现有对话框模式：**
- `UpdateDialog.vue` / `FileDiffDialog.vue` - 可参考的 el-dialog 实现

### Git 仓库检测机制

- 文件树节点加载时（`loadTreeNode`），检测每个目录节点是否为 Git 仓库
- `IsGitRepository()` 执行 `git rev-parse --git-dir` 检测
- `hasRemote` 通过 `git remote -v` 检测是否配置了远程仓库

### 用户需求要点（已确认）

1. ✅ **操作对象**：FileTreePanel 右侧文件树中扫描到的 Git 仓库
2. 右键菜单弹出仓库筛选窗口
3. 查看仓库清单（路径、简述、标签）
4. 简述支持查看 README 或手动录入
5. 自定义多个标签
6. 按标签筛选
7. 点击跳转到文件树对应节点
8. 缓存自定义信息（简述、标签）
9. 自动扫描差量新增仓库
10. 两个 tab 页：已编辑 / 未编辑

## 待确认的关键问题

### 问题 2：入口位置确认 ✅ 已确认

**入口位置**：FileTreePanel 顶部工具栏按钮 + 空白区域右键菜单（多入口组合）

### 问题 3：仓库扫描范围 ✅ 已确认

**扫描范围**：默认当前工作目录，提供下拉切换查看其他工作目录的仓库。

### 问题 4：简述来源机制 ✅ 已确认

**简述机制**：自动解析 README.md 作为"默认简述"，用户手动输入的作为"自定义简述"优先显示。两个信息分开展示，互不覆盖。

### 问题 5：标签管理机制 ✅ 已确认

**标签机制**：纯自由输入，用户完全自定义标签文本，支持多个标签。无预设标签库。

### 问题 6：Tab 页分类逻辑 ✅ 已确认

**已编辑定义**：添加过至少一个标签即为已编辑。简述填写不影响分类。

### 问题 7：跳转功能行为 ✅ 已确认

**跳转行为**：自动展开文件树到目标仓库节点，选中并滚动到可视区域，同时关闭筛选器弹窗。

### 问题 8：数据存储方案 ✅ 已确认

**存储方案**：JSON 文件存储（`repo_meta.json`），与现有 `directories.json` 风格一致，简单直观。

### 问题 9：差量扫描时机 ✅ 已确认

**扫描时机**：打开弹窗时自动扫描 + 提供手动刷新按钮。

## 扩展思考

### 1. 未来演化

- **标签分组/着色**：用户可能希望对标签进行分组或设置颜色，方便视觉区分
- **仓库收藏/置顶**：常用仓库可能需要置顶或收藏功能
- **批量操作**：选中多个仓库进行批量更新、批量添加标签
- **导出/导入**：导出仓库清单和元数据，便于迁移或备份

### 2. 相关场景

- 与 **"更新仓库"** 功能的联动：筛选结果中可直接触发更新
- 与 **搜索功能** 的一致性：筛选结果样式应与文件搜索结果保持一致
- 标签筛选可能与 **收藏夹** 功能有交集，需考虑信息结构一致性

### 3. 边缘情况

- **仓库路径失效**：用户删除或移动了仓库目录，缓存的元数据仍存在
- **README 解析失败**：非 UTF-8 编码、二进制文件、空文件等情况
- **大量仓库性能**：工作目录下有数百个仓库时，扫描和渲染性能
- **并发编辑**：多个弹窗同时编辑同一仓库的元数据（低概率，可暂不处理）

---

## 已确认的完整需求

### 功能需求

| 编号 | 需求 | 说明 |
|------|------|------|
| F1 | 多入口触发 | FileTreePanel 工具栏按钮 + 空白区域右键菜单 |
| F2 | 工作目录切换 | 默认当前工作目录，下拉切换其他工作目录 |
| F3 | 双 Tab 页 | "已编辑"（有标签）和"未编辑"（无标签）两个 Tab |
| F4 | master-detail 两栏 | 左栏紧凑列表（名称/路径/标签预览/失效标记）+ 右栏详情编辑区，编辑区固定不随滚动失焦 |
| F5 | 简述双区域 | 自动解析 README 作为默认简述 + 用户可输入自定义简述 |
| F6 | 标签自由输入 | 纯自由输入标签，支持多个标签，无预设库 |
| F7 | 标签筛选 | 上方提供标签筛选器，支持多选筛选 |
| F8 | 跳转定位 | 点击跳转按钮 → 展开文件树 → 选中节点 → 滚动到可视 → 关闭弹窗 |
| F9 | 差量扫描 | 打开弹窗时自动扫描 + 提供手动刷新按钮 |
| F10 | 数据持久化 | 用户自定义信息（简述、标签）存储到 `repo_meta.json` |

### 非功能需求

| 编号 | 需求 | 说明 |
|------|------|------|
| NF1 | 响应速度 | 扫描 100 个仓库 < 3 秒 |
| NF2 | 兼容性 | 支持 Windows/macOS/Linux |
| NF3 | 容错性 | README 解析失败不阻塞，显示占位文本 |

### 审核补充需求（复用与风险修正）

| 编号 | 需求 | 说明 |
|------|------|------|
| F11 | 跳转复用 | 复用 `FileTreePanel.locateNode`，跨工作目录时先切换再定位 |
| F12 | 扫描复用+优化 | 复用 `GitService.ScanGitRepos`，加 `.git` 预筛 + mtime 缓存 |
| F13 | 路径规范化 | 元数据主键统一 `filepath.Abs`，与 DirectoryService 一致 |
| F14 | 标签筛选语义 | 多选筛选采用 **OR（任一匹配）** |
| F15 | 失效仓库处理 | 灰显 + 标记，提供"清理失效记录"手动入口，不自动删除 |
| F16 | 编辑保存时机 | 简述防抖 800ms 自动保存，标签增删即时保存 |
| F17 | README 摘要缓存 | 解析结果缓存在 `ReadmeSummary`，避免重复读盘 |
| F18 | 搜索框 | 顶部搜索框，按仓库名/路径模糊匹配，海量仓库第一道闸门 |
| F19 | 虚拟滚动 | 左栏列表虚拟滚动，上万仓库不卡顿；右栏编辑区固定 |

---

## 验收标准

- [ ] 工具栏按钮点击可打开仓库筛选器弹窗
- [ ] 空白区域右键菜单包含"仓库筛选器"选项
- [ ] 弹窗标题显示当前工作目录名称
- [ ] 工作目录下拉可切换，切换后自动刷新仓库列表
- [ ] 已编辑/未编辑两个 Tab 正确分类
- [ ] 仓库列表显示：名称、路径、默认简述（README）、自定义简述、标签
- [ ] README 自动解析正确，解析失败显示占位文本
- [ ] 用户可输入自定义简述，保存后优先显示
- [ ] 标签输入框支持自由输入，支持删除已添加标签
- [ ] 标签筛选器支持多选，筛选结果实时更新
- [ ] 点击跳转按钮，文件树正确展开并选中目标节点，弹窗自动关闭
- [ ] 打开弹窗时自动扫描新增仓库
- [ ] 手动刷新按钮可触发重新扫描
- [ ] 用户编辑信息持久化到 `repo_meta.json`，重启应用后数据保留
- [ ] 跨工作目录跳转：目标在其他工作目录时，自动切换并定位成功
- [ ] 标签多选筛选为 OR 语义，选中多个标签时任一命中即显示
- [ ] 失效仓库灰显标记，"清理失效记录"可移除其元数据
- [ ] 简述防抖自动保存，标签增删即时保存，无需点保存按钮
- [ ] 扫描 100 个仓库在 3 秒内完成（`.git` 预筛优化后）
- [ ] 左栏虚拟滚动，500 个仓库滚动流畅不卡顿
- [ ] 搜索框按仓库名/路径模糊匹配，实时过滤左栏
- [ ] 左栏选中项后，右栏详情区正确展示该仓库信息
- [ ] 滚动左栏时，右栏编辑区输入框不失焦

---

## Research References

* [`research/virtual-scroll-selection.md`](research/virtual-scroll-selection.md) - 推荐 `@vueuse/core useVirtualList`（零新增依赖、自带 scrollTo、选中态数据驱动），右栏用 splitpanes 独立 Pane 固定
* [`research/git-scan-optimization.md`](research/git-scan-optimization.md) - 推荐 `.git` 存在性预筛（os.Stat 不要求 IsDir）+ mtime 缓存，100 仓库首次 <0.5s
* [`research/cross-workdir-locate.md`](research/cross-workdir-locate.md) - 跨工作目录跳转由 Home.vue 层编排，无需改 FileTreePanel 接口；注意 locateNode 路径 startsWith 未处理大小写

### 研究关键结论（影响实现）

1. **虚拟滚动**：用 `@vueuse/core` 的 `useVirtualList`（已安装 12.0.0），F16 防抖同用 `useDebounceFn`，无需新增依赖。左栏项强制等高（路径省略+标签限 1 行），右栏用 `splitpanes` 独立 Pane。
2. **扫描优化**：`.git` 存在性预筛必须用 `os.Stat` 无错判定（**不要求 `IsDir()`**），否则 worktree/submodule 漏判（现有 `util/git.go:142 FindGitRoot` 已有此 bug，本任务避免重蹈）。mtime 缓存激进跳过子树有深层新增漏扫风险，靠 F9 手动刷新 + 5 分钟 TTL 兜底。
3. **跨工作目录跳转**：时序为「规范化查找 targetDir（toLowerCase）→ 关弹窗 → await onDirectorySelect(targetDir.id) 触发 treeKey 变化与 el-tree 重建 → await locateNode(repoPath)（内部 await treeReadyPromise 兜底）」。同工作目录直接 locateNode。

---

## 技术方案

### 后端新增

1. **数据模型** (`model/repo_meta.go`)
   ```go
   // RepoMeta 仓库用户元数据，按规范化路径（filepath.Abs）作主键
   type RepoMeta struct {
       Path          string    `json:"path"`          // 规范化后的绝对路径（主键）
       Summary       string    `json:"summary,omitempty"`        // 用户自定义简述
       Tags          []string  `json:"tags,omitempty"`           // 用户自定义标签
       ReadmeSummary string    `json:"readmeSummary,omitempty"`  // 自动解析的 README 摘要（缓存，避免重复读盘）
       Missing       bool      `json:"missing,omitempty"`        // 扫描时路径已失效，前端灰显
       UpdatedAt     time.Time `json:"updatedAt"`                // 元数据最后更新时间
       LastScanAt    time.Time `json:"lastScanAt,omitempty"`     // 最后一次扫描命中时间
   }
   ```

2. **服务层** (`service/repo_meta.go`)
   - `RepoMetaService` - 元数据 CRUD，**所有路径键统一 `filepath.Abs` 规范化**（与 `DirectoryService` 一致，规避大小写/分隔符歧义）
   - `Load() map[string]*RepoMeta`
   - `Upsert(meta *RepoMeta) error` - 按规范化路径写入
   - `Delete(path string) error`
   - `DeleteMissing() (int, error)` - 清理失效记录

3. **扫描层**（**复用现有** `GitService.ScanGitRepos`，`service/git.go:214`）
   - 现状：对每个目录 fork `git rev-parse`，目录多时慢
   - 优化：新增 `.git` 目录预筛（`os.Stat`），仅疑似目录调 git 校验，降低 90%+ 子进程开销
   - 扫描结果带目录 mtime 缓存，二次打开走缓存差量

4. **API 层** (`app.go` 新增方法)
   - `GetRepoFilterList(dirId string) ([]*RepoFilterItem, error)` - 扫描+合并元数据，返回列表（含 ReadmeSummary、Missing、Tags）
   - `SaveRepoMeta(path, summary string, tags []string) error` - 保存元数据（路径内部规范化）
   - `CleanMissingRepoMeta() (int, error)` - 清理失效记录

4. **存储文件**
   - 路径：`data/repo_meta.json`
   - 格式：`{"repos": [...RepoMeta...]}`

### 前端新增

1. **组件** (`frontend/src/components/RepoFilterDialog.vue`) — master-detail 两栏布局
   - el-dialog 弹窗容器（建议宽 900px、高 650px）
   - 顶部：工作目录下拉 + **搜索框**（仓库名/路径模糊匹配） + 标签筛选器（多选，**OR 语义**） + 刷新按钮 + 清理失效按钮
   - Tab 切换：已编辑（有标签）/ 未编辑（无标签），Tab 标题显示计数
   - **左栏**：紧凑列表（名称、路径、标签预览 chips、失效标记），**虚拟滚动**（el-table-v2 或 vue-virtual-scroller），点击项高亮选中
   - **右栏**：选中项详情编辑区（固定，不随左栏滚动）
     - 仓库名、完整路径
     - README 摘要（只读，缺失显示"暂无 README"）
     - 自定义简述输入框（**防抖 800ms 自动保存**）
     - 标签编辑组件（增删即时保存）
     - 跳转按钮
   - 失效仓库左栏灰显 + 标记

2. **跳转实现**（**复用现有** `FileTreePanel.locateNode`，已 `defineExpose`，`FileTreePanel.vue:1283`）
   - 同工作目录：直接 `fileTreePanelRef.locateNode(path)` + 关闭弹窗
   - **跨工作目录衔接**：若目标不在当前 `selectedDirId`，先 emit 切换工作目录 -> 等待 `treeReadyPromise` 完成 -> 再 `locateNode` -> 关闭弹窗

3. **README 摘要策略**（后端解析，前端展示）
   - 文件名匹配：`README.md` / `README.MD` / `readme.md` / `README` / `README.rst`（按优先级）
   - 截取规则：去除 Markdown 标记后取首段非空文本，上限 200 字
   - 容错：非 UTF-8 按 GBK 降级、空文件/无 README 显示占位文本"暂无 README"
   - 解析结果缓存在 `RepoMeta.ReadmeSummary`，避免重复读盘

4. **API 调用**
   - `GetRepoFilterList(dirId)`
   - `SaveRepoMeta(path, summary, tags)`
   - `CleanMissingRepoMeta()`

### 文件结构

```
model/
├── repo_meta.go          # 新增：RepoMeta 模型

service/
├── repo_meta.go          # 新增：RepoMetaService

frontend/src/components/
├── RepoFilterDialog.vue  # 新增：仓库筛选器弹窗

data/
├── repo_meta.json        # 新增：元数据存储
```

---

## 决策（ADR-lite）

**Context**: 需要管理文件树中大量嵌套的 Git 仓库，支持标签分类和简述备注。

**Decision**: 采用 JSON 文件存储 + 前端弹窗方案，与现有架构风格一致。**复用** `GitService.ScanGitRepos` 与 `FileTreePanel.locateNode`，不重复造轮子。

**Consequences**:
- ✅ 简单直观，易于实现和维护
- ✅ 与现有 `directories.json` 风格统一
- ✅ 复用已有扫描/跳转能力，实现量降低约 40%
- ✅ `.git` 预筛 + mtime 缓存规避子进程性能瓶颈
- ⚠️ 跨工作目录跳转需处理文件树重新加载衔接（已纳入 F11）
- ⚠️ 后期若仓库规模达千级，可演进为 SQLite

---

## 超出范围（明确）

- DirectoryTree 左侧工作目录的管理（已有完整 CRUD）
- 标签预设库功能（纯自由输入）
- 批量操作仓库（批量更新、批量标签）
- 仓库收藏/置顶功能
