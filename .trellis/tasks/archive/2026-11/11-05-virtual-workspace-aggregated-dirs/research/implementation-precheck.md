# 实现前置调研结论(2026-11-05,主会话 codegraph 收口)

PRD「实现时待确认」清单已全部收口,实现直接按本文结论执行。

## 1. useTreeState key 结构与聚合隔离方案

- `frontend/src/composables/useTreeState.js`:localStorage key = `treeState:<dirPath>`,存 `{expandedPaths(截尾200), scrollTop, selectedPath}`
- **结论**:聚合模式传固定虚拟 key `treeState:__aggregated__`(与真实目录路径天然不冲突,双下划线前缀保留),两模式状态互不污染无需改 composable 本体

## 2. workspaceStore 角色——天然兼容,无需适配

- `workspaceStore.selectedNode` 是文件树选中节点的共享出口,下游 StatsView(`selectedNode?.path`)、GitInfo 等均消费绝对路径
- **结论**:聚合树选中节点写入 workspaceStore 后,下游面板照常工作,零适配;不需要在 FileTreePanel 做任何 workspaceStore 分流

## 3. 现有收藏可含文件——聚合根必须过滤

- FileTreePanel `onNodeContextMenu` 对文件/目录节点均弹菜单,`isFavorited` 仅按 path 判定 → 收藏夹中可能存在文件收藏
- **结论**:聚合根子节点只消费目录类收藏,过滤收口在后端聚合绑定(见 §5),前端不做存在性/类型判定

## 4. GetGitInfo 既有绑定不足以独立支撑虚拟根构造

- `app_git.go:19` 已有 `GetGitInfo(path) *model.GitRepoInfo`(IsRepo/Branch/Remote/RemoteURL)
- 可推导 git 徽标(IsRepo→IsGitRepo,Remote 非空→HasRemote),**但无法判目录存在性**,且逐目录 IPC 在收藏量大时调用面碎
- **结论**:不直接采用,被 §5 聚合绑定取代

## 5. 推荐新增后端聚合绑定 `GetAggregatedRootNodes`(失效判定 + git 徽标一次收口)

**签名**:`func (a *App) GetAggregatedRootNodes(paths []string) []*model.FileTreeNode`

**实现要点**:
- service 层新方法(建议放 `service/filetree.go` 的 `FileTreeService` 上,如 `GetRootNodesForPaths`)
- 对每个 path:`os.Stat` 判存在性与 IsDir;复用既有 `isGitRepoDir`/`hasRemote`(均带缓存)填 `IsGitRepo`/`HasRemote`
- **失效表示**:不存在/非目录/Stat 失败的 path 不产出目录节点,改为产出带失效标志的占位节点(建议复用 FileTreeNode 加字段 `Missing bool json:"missing"`,name 取路径末段,type 保持 "directory",IsLeaf=false)——避免引入新 model 类型,缺失字段旧数据零值兼容
- 顺序保持与入参一致(收藏列表顺序),不排序
- **同步义务(新增 App 方法)**:`frontend/wailsjs/` 三处手动同步(App.js / App.d.ts / models.ts——`Missing` 字段加进 FileTreeNode 定义);纯桌面数据面方法须加入 `web_serve.go` `RPCHiddenMethods` 现清单(见关键规则 browser-channel ①)

**为何不做纯前端方案**:无轻量存在性检查绑定,借道 GetFileTree 逐目录全扫或 PreviewFile 借道语义均劣;一次批量 IPC 最干净,service 复用既有缓存 ~30 行。

## 6. 「设为工作目录」切换语义

- 前端 `frontend/src/store/directory.js`(directoryStore)承载工作目录列表与 `selectedDirectoryId`
- 收藏目录可能不在工作目录列表内——实现时确认 directoryStore 是否有「添加并选中」入口(后端目录管理绑定在 app_directory.go 或类似);若无添加入口,右键项文案收敛为「在普通树中打开」需先加入工作目录列表
- 该点不阻塞 PR1/PR2 主体,实现时按 store 现状择路

## 7. 虚拟根节点 key 约定

- el-tree node-key 用 path(FileTreeNode.ID=path);虚拟根本身不落盘不持久化,仅 loadTreeNode level 0 分流时前端构造,不进 GetFileTree 链路
- 截断哨兵 `truncateTreeNodes(nodes, path)` 的 path 用各收藏目录自身路径,哨兵逻辑零改动
