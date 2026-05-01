# 批量更新仓库功能设计

**日期：** 2026-05-01
**状态：** 已确认，待实现
**关联：** Git Manager v1.x 增强

## 1. 摘要

在文件树文件夹节点的右键菜单中新增"更新仓库"功能。若当前目录是 Git 仓库则直接拉取；若不是则递归扫描所有子目录，批量并行拉取所有 Git 仓库，并通过实时进度弹窗展示每个仓库的更新结果。

## 2. 功能描述

### 2.1 触发方式

- **入口：** 文件树文件夹节点的右键菜单 → "更新仓库"
- **显示条件：** 仅文件夹节点显示，文件节点不显示
- **显示位置：** "在资源管理器中打开"下方，以分隔线分隔

### 2.2 执行逻辑

1. 用户点击"更新仓库"
2. 前端调用 `ScanAndPullRepos(dirPath)` 获取待更新仓库总数
3. 打开进度弹窗，显示总量和进度条
4. Go 后端启动 goroutine 池（最大并发 5）并行拉取
5. 每完成一个仓库，通过 Wails 事件推送结果到前端
6. 前端实时更新弹窗中的表格行和进度条
7. 全部完成后显示汇总统计

### 2.3 结果展示

实时进度弹窗采用 Element Plus Dialog + Table 组件：

- **标题栏：** "更新仓库"（进行中） / "更新完成"（全部完成）
- **进度条：** 已完成数 / 总数，百分比
- **结果表格：** 状态列（等待中/进行中/成功/失败）、仓库名称、路径、结果摘要
- **失败行：** 可展开查看详细错误信息
- **底部汇总：** 成功: N, 失败: M
- **关闭按钮：** 进行中时显示为禁用态，完成后可关闭

## 3. 技术方案

### 3.1 方案选择：Wails 事件驱动

采用 Go goroutine + `runtime.EventsEmit` 事件推送机制，实现前后端实时通信。

**选择理由：**
- 用户要求实时进度展示
- 大量子仓库场景下同步阻塞会导致 UI 无响应
- Wails 原生支持事件系统，实现成本低

### 3.2 数据流

```
用户右键 → 点击"更新仓库"
        │
        ▼
前端调用 ScanAndPullRepos(dirPath)
        │
        ▼
Go 后端：
  1. 判断 dirPath 是否为 git 仓库
  2. 是 → [dirPath]
  3. 否 → 递归扫描子目录，收集所有 git 仓库路径
  4. 立即返回 PullSummary{Total: N}
  5. 启动 goroutine 池（max=5）并行 pull
  6. 每完成一个 → EventsEmit("pull-progress", PullResult)
  7. 全部完成 → EventsEmit("pull-complete", summary)
        │
        ▼
前端监听事件：
  pull-progress → 更新表格行状态 + 进度条
  pull-complete → 更新标题 + 底部汇总 + 启用关闭按钮
```

## 4. 数据模型

### 4.1 新增模型（`model/models.go`）

```go
// PullResult 单个仓库的拉取结果
type PullResult struct {
    Path    string `json:"path"`
    Name    string `json:"name"`
    Success bool   `json:"success"`
    Output  string `json:"output"`
    Error   string `json:"error,omitempty"`
}

// PullSummary ScanAndPullRepos 的初始返回值
type PullSummary struct {
    Total int `json:"total"`
}
```

### 4.2 事件协议

| 事件名 | 触发时机 | 数据结构 |
|--------|----------|----------|
| `pull-progress` | 每完成一个仓库 | `PullResult` |
| `pull-complete` | 全部完成 | `{"success": int, "failed": int}` |

## 5. 后端设计

### 5.1 `service/git.go` 新增方法

**`ScanGitRepos(rootPath string) []string`**

- 判断 `rootPath` 是否为 git 仓库（`util.GitCommand.IsGitRepository`）
- 是 → 返回 `[rootPath]`
- 否 → 递归遍历子目录，收集所有 git 仓库路径
- 递归深度无限制

**`BatchPull(repos []string, concurrency int, ctx context.Context)`**

- 使用带缓冲 channel 作为 semaphore 限制并发（默认 5）
- 每个 goroutine 调用 `util.GitCommand.Pull(repo)` 拉取
- 每完成一个仓库 → `runtime.EventsEmit(ctx, "pull-progress", result)`
- 全部完成 → `runtime.EventsEmit(ctx, "pull-complete", summary)`

### 5.2 `app.go` 新增绑定方法

```go
func (a *App) ScanAndPullRepos(dirPath string) (*model.PullSummary, error)
```

- 调用 `ScanGitRepos(dirPath)` 获取仓库列表
- 列表为空 → 返回错误
- 构造 `PullSummary` 返回给前端
- 启动 goroutine 执行 `BatchPull()`（异步，不阻塞返回）

## 6. 前端设计

### 6.1 右键菜单

在 `Home.vue` 的 `onMenuCommand` 中新增 `case 'pullRepos'`，对应新增的 `<li>` 菜单项。

### 6.2 进度弹窗

新增以下响应式状态：

```js
const pullDialogVisible = ref(false)
const pullProgress = reactive({ current: 0, total: 0 })
const pullResults = ref([])        // PullResult[]
const pullCompleted = ref(false)
```

事件监听：

```js
EventsOn("pull-progress", (result) => {
  pullResults.value.push(result)
  pullProgress.current++
})
EventsOn("pull-complete", (summary) => {
  pullCompleted.value = true
})
```

## 7. 涉及文件变更

| 文件 | 变更 |
|------|------|
| `model/models.go` | 新增 `PullResult`、`PullSummary` 结构体 |
| `service/git.go` | 新增 `ScanGitRepos()`、`BatchPull()` |
| `app.go` | 新增 `ScanAndPullRepos()` 绑定方法 |
| `frontend/src/views/Home.vue` | 右键菜单项 + 进度弹窗 + 事件监听 |

## 8. 错误处理与边界情况

| 场景 | 处理方式 |
|------|----------|
| 目录下无 git 仓库 | `ElMessage.warning("未找到任何 Git 仓库")`，不弹窗 |
| 单个仓库 pull 失败 | 标记失败，继续其他仓库 |
| git pull 超时 | 单个仓库 5 分钟超时，标记失败 |
| 用户关闭弹窗 | 后端继续执行，前端停止接收事件 |
| 仓库有本地修改 | git pull 自动处理，输出显示在结果中 |
| 嵌套 git 仓库（子模块） | 父仓库和子模块均作为独立仓库拉取 |
| 短时间多次触发 | 允许，每次独立执行 |

### 明确不做

- 取消/中断功能
- 拉取历史记录
- 自动定时拉取
