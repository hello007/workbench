# 更名设计：git-manager → WorkBench

**日期：** 2026-06-09
**状态：** 已批准

## 背景

项目当前名称"git-manager"暗示这是一个 Git 仓库管理工具，但实际功能已远超 Git 管理范畴：

- 文件树浏览与文件操作（新建/重命名/删除/预览编辑）
- 智能导航中心（Command Palette、收藏夹、内容搜索）
- 内置终端（多 Shell 支持）
- 自定义快捷键
- 设置面板

Git 功能仅占约 15%，名称与实际定位严重不匹配。

## 选定名称

**WorkBench（工作台）** — 品牌型命名风格。

理由：
- 开发者的工作台——文件、终端、Git、搜索都是台面上的工具
- 匹配多面板布局（文件树 | 内容面板 | 终端）
- 简短好记，搜索区分度良好
- 扩展性强——未来新功能都是"台面上的新工具"

## 变更范围

### 代码层（必须验证构建通过）

| 文件 | 变更 |
|------|------|
| `go.mod` | `module git-manager` → `module workbench` |
| `main.go` | import `"git-manager/service"` → `"workbench/service"` |
| 所有含 `"git-manager/..."` import 的 Go 文件 | 同上 |
| `wails.json` | name/outputfilename/productName/comments |
| `frontend/index.html` | `<title>` 标签 |

### 文档层（人可读名称全量替换）

| 文件 | 变更 |
|------|------|
| `README.md` | 标题、描述、项目结构中所有 "Git Manager" / "git-manager" |
| `CLAUDE.md` | 项目名称、描述 |
| `CHANGELOG.md` | 标题 |
| `DEVELOPMENT.md` | 所有引用 |
| `BUILD_SUMMARY.md` | 所有引用 |
| `docs/功能说明.md` | 标题 |
| `docs/开发工作流.md` | 引用 |
| `docs/开发规范.md` | 引用 |
| `docs/常见问题.md` | 引用 |
| `docs/路线图.md` | 引用 |
| `docs/测试策略.md` | 引用 |
| `docs/部署说明.md` | 引用 |
| `docs/plans/*.md` | 内容引用（不改文件名） |
| `docs/superpowers/specs/*.md` | 内容引用（不改文件名） |
| `docs/superpowers/plans/*.md` | 内容引用（不改文件名） |

### 不改的内容

- `.worktrees/` 下的旧文件（历史归档）
- `release/git-manager-v1.0.0/`（已发布历史版本）
- `openspec/changes/archive/` 下的历史文件（保持原样）
- Git 仓库名/远程 URL（用户自行决定）

## 执行顺序

1. Go 层：`go.mod` → 所有 import → 验证 `go build`
2. Wails 配置：`wails.json`
3. 前端：`frontend/index.html`
4. 文档层：所有 `.md` 文件
5. 全量验证：`wails build`

## 命名映射表

| 旧名称 | 新名称 |
|--------|--------|
| git-manager (Go module / 二进制) | workbench |
| Git Manager (人可读) | WorkBench |
| git-manager.exe | workbench.exe |
| Git仓库管理工具 | 开发者工作台 |
| Git Manager - Git仓库管理桌面应用 | WorkBench - 开发者工作台 |
