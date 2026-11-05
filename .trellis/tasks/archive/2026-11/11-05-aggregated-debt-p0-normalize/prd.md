# P0 路径与缓存口径归一（5 条）

## Goal

消除路径写法与缓存生命周期造成的不一致，全部为低风险改动，不新增契约字段。

## Requirements

### ⑨ isFavorited 路径归一比较

- `frontend/src/components/FileTreePanel.vue:625` 的 `f.path === path` 改为使用同文件 `:895` 的 `normalizeComparablePath` 比较。
- 验收：收藏路径为 `D:\repo\`、节点路径为 `D:\repo` 时，右键显示「取消收藏」。

### ⑩ onSetAsWorkDir 第 4 步空操作

- 删除 `frontend/src/views/Home.vue:313` 的 `locateNode(data.path)` 调用（普通树下 relative 为空恒早退），并把 `:279-286` 的四步注释改为三步。
- 验收：设为工作目录后仍正常切换与恢复树状态，无残留无效调用。

### ⑪ 收藏路径尾分隔符全链路统一

- 后端 `service/filetree.go` `GetRootNodesForPaths` 入口对 `p` 执行 `filepath.Clean`，Stat、Base、git 缓存键、返回 `Path` 全部使用清洗后的路径。
- 收藏写入侧 `service/favorites.go` 的 `Add/Remove/UpdateAlias/UpdateGroup` 统一 `filepath.Clean` 后比较与落盘；`Load` 对旧数据做内存归一（不强制落盘迁移）。
- 验收：`D:\repo\` 与 `D:\repo` 产出同一节点 key，不重复缓存、不重复收藏。

### ③ 脏收藏路径展示名

- `GetRootNodesForPaths` 统一跳过空串与纯空白串（当前空白串产出占位，口径不一致）。
- 卷根（`C:\`、`/`）用 `filepath.VolumeName` 回退展示名，`filepath.Base` 结果不可读时不产出空名。
- 验收：空白串不产节点；`C:\` 显示可读名称且节点可展开。

### ① git 徽标缓存失效通道

- `service/filetree.go` 的 `InvalidateCache` 联动 `gitRepoCache`/`gitRemoteCache`：删除该路径项，并 `Range` 前缀匹配删除其直接子目录项。
- `ClearAllCache` 逐键清空两个 `sync.Map`。
- 不引入 TTL（避免刷新后仍陈旧的观感）。
- 验收：真仓库 `git init` 或 `git remote add` 后，手动刷新文件树徽标即时更新。

## Acceptance Criteria

- [ ] 后端新增单测覆盖：尾分隔符归一、空白串跳过、卷根展示名、刷新后 git 徽标更新
- [ ] 前端单测覆盖：isFavorited 归一比较
- [ ] `go vet ./service/ .`、`go test ./service/ -short`、`npm test` 通过
- [ ] 完成后删除清单中这 5 条

## Definition of Done

- 不修改 `model/` 导出 struct 字段，不同步 wailsjs
- 若实现中确认必须新增字段，退回父任务重新评估
