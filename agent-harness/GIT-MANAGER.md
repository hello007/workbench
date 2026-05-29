# Git Manager CLI — 架构分析与 SOP

## 1. 后端引擎

Git Manager 的"后端引擎"是 **git CLI**。所有 Git 操作（clone、pull、branch、checkout、status）均通过 `git` 命令行执行。

| 功能 | 底层调用 |
|------|---------|
| 克隆仓库 | `git clone <url> <path>` |
| 拉取更新 | `git pull` |
| 分支列表 | `git branch -a` |
| 切换分支 | `git checkout <branch>` |
| 检测仓库 | `git rev-parse --git-dir` |
| 当前分支 | `git branch --show-current` |
| 远程地址 | `git remote -v` |
| 本地变更 | `git status --porcelain` |

额外使用 **go-git** 库（纯 Go 实现）进行提交历史读取和仓库元数据查询，无需 git CLI。

## 2. 数据模型

### 2.1 工作目录配置 (`data/directories.json`)

```json
{
  "directories": [
    {
      "id": "dir-1777381884515193600",
      "name": "workspace_ai",
      "path": "D:\\workspace\\workspace_ai",
      "isDefault": true,
      "createTime": "2026-04-28T21:11:24.5151936+08:00"
    }
  ]
}
```

### 2.2 文件树节点

```json
{
  "id": "path",
  "name": "filename",
  "path": "full/path",
  "type": "file|directory",
  "isGitRepo": false,
  "hasChildren": true,
  "isLeaf": true
}
```

### 2.3 Git 信息

```json
{
  "path": "/repo/path",
  "branch": "master",
  "remote": "origin",
  "remoteUrl": "https://...",
  "isRepo": true,
  "commits": []
}
```

### 2.4 提交记录

```json
{
  "sha": "abc123...",
  "shortSHA": "abc12345",
  "message": "commit msg",
  "author": "name",
  "email": "a@b.com",
  "timestamp": 1714300800,
  "dateTime": "2026-04-28 12:00:00",
  "files": ["path/to/file.go"]
}
```

## 3. GUI 到 CLI 映射

| GUI 操作 | CLI 命令 |
|----------|---------|
| 添加工作目录 | `directory add <name> <path>` |
| 删除工作目录 | `directory remove <id>` |
| 设置默认目录 | `directory default <id>` |
| 浏览文件树 | `file tree <path>` |
| 预览文件 | `file preview <path>` |
| 新建文件 | `file create <parent> <name>` |
| 新建文件夹 | `file mkdir <parent> <name>` |
| 重命名 | `file rename <path> <new-name>` |
| 删除 | `file delete <path>` |
| 克隆仓库 | `git clone <url> [path]` |
| 拉取更新 | `git pull <path>` |
| 查看信息 | `git info <path>` |
| 查看分支 | `git branches <path>` |
| 切换分支 | `git checkout <path> <branch>` |
| 提交历史 | `git log <path>` |
| 本地变更 | `git status <path>` |
| 在资源管理器打开 | `open explorer <path>` |
| 用 VSCode 打开 | `open vscode <path>` |
| 批量拉取 | `batch pull <dir-path>` |

## 4. 命令分组

```
cli-anything-gitmanager
├── directory (dir)    工作目录管理
├── git               Git 仓库操作
├── file              文件/文件夹操作
├── open              外部程序打开
├── batch             批量操作
├── session           会话管理（REPL）
└── repl              交互式 REPL 模式
```

## 5. 会话模型

- **当前工作目录**: 用户通过 `directory use <id>` 切换上下文
- **选中路径**: REPL 中维护当前选中路径，简化后续命令
- **持久化**: 会话状态保存为 JSON 文件，支持恢复

## 6. 输出格式

- 人类可读：表格、彩色文本、树形结构
- 机器可读：`--json` 标志输出结构化 JSON
