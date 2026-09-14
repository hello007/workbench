# 切换分支功能设计

> 日期：2026-05-22
> 状态：已批准

## 概述

在 Git 仓库的"操作面板"中，"拉取更新"按钮旁新增"切换分支"按钮。点击后弹出对话框，以分组下拉列表展示本地分支和远程分支，用户选择目标分支后执行切换。

## 需求

- 展示本地分支和远程分支，分组显示
- 选中远程分支时，自动在本地创建同名分支并跟踪远程分支
- 有未提交变更时拒绝切换并提示
- 切换成功后刷新 Git 信息面板和提交历史面板
- 切换失败展示具体错误信息

## 后端设计

### 新增数据模型

文件：`model/git.go`

```go
type BranchInfo struct {
    Name      string
    IsRemote  bool
    IsCurrent bool
}

type BranchList struct {
    Branches []BranchInfo
}
```

### 新增 Wails 绑定方法

文件：`app.go`

**1. `GetBranches(repoPath string) (*model.BranchList, error)`**

- 调用 `git branch -a` 获取所有分支
- 解析输出，区分本地分支和远程分支
- 远程分支去掉 `remotes/` 前缀，过滤 `HEAD ->` 等特殊引用
- 标记当前分支（以 `*` 开头的行）

**2. `CheckoutBranch(repoPath string, branchName string, isRemote bool) error`**

- 先执行 `git status --porcelain` 检查是否有未提交变更
- 有变更则返回错误："当前有未提交的变更，请先提交或暂存后再切换分支"
- 本地分支：执行 `git checkout <branchName>`
- 远程分支：执行 `git checkout -b <localName> <remoteBranch>`

### 新增 Service 方法

文件：`service/git.go`

- `GetBranches(dirPath string) (*model.BranchList, error)`
- `CheckoutBranch(dirPath string, branchName string, isRemote bool) error`

### 新增 Git 命令

文件：`util/git.go`

- 在 `GitCommand` 上新增获取分支列表和切换分支的底层命令方法

## 前端设计

### 按钮位置

在 `ContentPanel.vue` 中，"拉取更新"按钮旁并排放置"切换分支"按钮。

### 弹窗结构

使用 `el-dialog` + `el-select`：

- 弹窗标题："切换分支"
- 显示当前分支名
- `el-select`：启用 `filterable`（搜索过滤）、`group`（分组）
  - 本地分支组
  - 远程分支组
  - 当前分支 `disabled`
- 底部按钮：取消 / 切换

### 交互流程

1. 点击"切换分支"按钮 → 调用 `GetBranches` 获取分支列表 → 弹出对话框
2. 用户选择分支 → 点击"切换" → 按钮进入 loading 状态
3. 调用 `CheckoutBranch` → 成功则关闭弹窗、`ElMessage.success`、刷新面板
4. 失败则 `ElMessage.error` 展示错误信息

## 错误处理

| 场景 | 处理方式 |
|------|---------|
| 有未提交变更 | `ElMessage.error` 提示先提交或暂存 |
| 分支不存在 | 展示 git 原始错误 |
| IO/网络错误 | 展示异常信息 |
| 已在目标分支 | 下拉中 `disabled` + 前端预校验 |

## 涉及文件

| 文件 | 变更类型 |
|------|---------|
| `model/git.go` | 新增 `BranchInfo`、`BranchList` 结构体 |
| `util/git.go` | 新增分支列表和切换分支的底层命令方法 |
| `service/git.go` | 新增 `GetBranches`、`CheckoutBranch` 方法 |
| `app.go` | 新增 `GetBranches`、`CheckoutBranch` 绑定方法 |
| `frontend/src/components/ContentPanel.vue` | 新增按钮、弹窗、交互逻辑 |
| `frontend/wailsjs/go/main/App.js` | 自动生成 |
