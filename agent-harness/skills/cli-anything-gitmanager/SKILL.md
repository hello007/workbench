---
name: "cli-anything-gitmanager"
description: "CLI harness for Git Manager desktop app — manage work directories, Git repos, and files from the command line"
---

# cli-anything-gitmanager

Git Manager 的命令行接口，让 AI Agent 无需 GUI 即可管理工作目录、Git 仓库和文件。

## Install

```bash
pip install -e .
# 或
pip install cli-anything-gitmanager
```

**依赖**: Git 必须安装并在 PATH 中可用。

## Usage

```bash
# 交互式 REPL
cli-anything-gitmanager

# 单次命令
cli-anything-gitmanager directory list --json
cli-anything-gitmanager git info /path/to/repo
cli-anything-gitmanager file tree /path/to/dir
```

## Command Groups

### directory (dir) — 工作目录管理

| Command | Description |
|---------|-------------|
| `directory list` | 列出所有工作目录 |
| `directory add <name> <path>` | 添加工作目录 |
| `directory remove <id>` | 删除工作目录 |
| `directory default [id]` | 查看/设置默认目录 |
| `directory use <id>` | 切换到指定目录（REPL） |

### git — Git 仓库操作

| Command | Description |
|---------|-------------|
| `git info [path]` | 查看仓库信息（分支、远程、状态） |
| `git clone <url> [path]` | 克隆仓库 |
| `git pull [path]` | 拉取更新 |
| `git branches [path]` | 查看分支列表 |
| `git checkout <branch> [path]` | 切换分支 |
| `git log [path]` | 查看提交历史 |
| `git status [path]` | 查看本地变更 |

### file — 文件/文件夹操作

| Command | Description |
|---------|-------------|
| `file tree [path]` | 浏览文件树 |
| `file preview <filepath>` | 预览文件内容 |
| `file create <parent> <name>` | 创建文件 |
| `file mkdir <parent> <name>` | 创建文件夹 |
| `file rename <path> <new-name>` | 重命名 |
| `file delete <path>` | 删除文件或文件夹 |

### open — 外部程序打开

| Command | Description |
|---------|-------------|
| `open explorer [path]` | 在资源管理器中打开 |
| `open vscode [path]` | 用 VSCode 打开 |

### batch — 批量操作

| Command | Description |
|---------|-------------|
| `batch pull <dir-path>` | 批量拉取目录下所有 Git 仓库 |
| `batch status <dir-path>` | 批量查看仓库状态 |

### session — 会话管理

| Command | Description |
|---------|-------------|
| `session status` | 查看当前会话状态 |
| `session save [name]` | 保存会话 |
| `session load [name]` | 加载会话 |
| `session undo` | 撤销操作 |
| `session redo` | 重做操作 |

## Agent-Specific Guidance

### JSON Output

所有命令支持 `--json` 标志，输出结构化 JSON：

```bash
cli-anything-gitmanager --json directory list
cli-anything-gitmanager --json git info /path/to/repo
cli-anything-gitmanager --json git branches /path/to/repo
cli-anything-gitmanager --json git log /path/to/repo -n 10
```

### Error Handling

命令失败时输出到 stderr，退出码非零。Agent 应检查退出码：

```bash
cli-anything-gitmanager git pull /path/to/repo
# 成功: exit 0, stdout 含输出
# 失败: exit 1, stderr 含错误信息
```

### Common Workflows

**检查仓库状态：**
```bash
cli-anything-gitmanager --json git info /repo
cli-anything-gitmanager --json git status /repo
cli-anything-gitmanager --json git log /repo -n 5
```

**批量更新所有仓库：**
```bash
cli-anything-gitmanager --json batch pull /workspace
cli-anything-gitmanager --json batch status /workspace
```

**管理文件：**
```bash
cli-anything-gitmanager file tree /repo
cli-anything-gitmanager file preview /repo/README.md
cli-anything-gitmanager file create /repo/src main.go
```
