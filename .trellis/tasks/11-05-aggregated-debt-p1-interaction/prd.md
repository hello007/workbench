# P1 聚合交互与并发（3 组）

## Goal

修正聚合视图下仍沿用「工作目录」语义的交互，并消除收藏首帧串行探测的延迟。

## Requirements

### ⑥ 聚合视图菜单项沿用工作目录语义

- `frontend/src/components/FileTreePanel.vue:361/443` 的「复制相对路径」基准改为 `findAggregatedRootFor(节点路径)`，无命中时回退节点自身。
- 「在此目录中搜索」同口径取所属收藏根。
- 聚合模式下空白区右键：**隐藏新建文件/文件夹**（虚拟根不落盘，无明确落点），保留刷新与粘贴。
- 验收：聚合树内对收藏根子节点「复制相对路径」得到相对该收藏根的路径。

### ⑦ 收藏外部入口变更不刷新聚合根

- 组件内新增 `watch(() => favoritesStore.favorites, ..., { deep: true })`，聚合模式下 100ms 去抖后调 `refreshAggregatedRoot()`。
- 必须带 `modeSwitchSeq` 代次守卫（与既有重建链一致），避免与模式切换竞态。
- 覆盖来源：命令面板取消收藏、配置导入后 `loadFavorites`、收藏别名修改。
- 验收：聚合视图下取消收藏，该根目录自动消失；修改别名后显示名更新且展开态保持。

### ⑧ + ⑫ 批量存在性探测并发化

- `service/filetree.go` `GetRootNodesForPaths` 改两阶段：
  1. 并发 `os.Stat`，信号量 4；
  2. 仅对 git 仓库并发 `hasRemote`，信号量 4。
- 结果按入参下标回填切片，保持入参顺序，不引入锁。
- **不做整体超时**：Wails 绑定签名无 `ctx`，且网络盘 `os.Stat` 为不可中断的阻塞系统调用；强行超时只能靠独立 goroutine 丢弃结果，存在泄漏风险。本任务仅通过并发化降低总时长。
- 验收：100 个收藏时首帧耗时明显低于串行实现；输出顺序与入参一致。

## Acceptance Criteria

- [ ] 后端新增并发测试：100 路径顺序保持、结果正确
- [ ] 前端新增单测：收藏变化触发重建（代次守卫）、空白区新建隐藏
- [ ] E2E 覆盖：取消收藏后聚合根消失、别名修改后显示名更新
- [ ] `go test ./service/ -short`、`npm test`、`npm run build` 通过
- [ ] 完成后删除清单中这 3 组条目（⑧与⑫合并销账）

## Definition of Done

- 并发实现使用 `sync.WaitGroup` + 下标写入，不使用共享可变切片追加
- 不做绑定签名变更
