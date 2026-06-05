# 跨仓库内容搜索设计

> **日期**：2026-06-05
> **状态**：待实施
> **类型**：新功能

## 1. 概述

### 1.1 背景

Git Manager 目前支持按文件名模糊搜索，但无法搜索文件内容。日常工作中经常需要在多个仓库中定位某个类名、配置项或错误信息，目前只能逐个仓库手动 grep。

### 1.2 目标

在 Command Palette 中新增内容搜索模式，支持：

- 跨所有工作目录搜索文件内容
- 指定子目录范围精确搜索
- 文件类型过滤
- ripgrep 自动加速（Go 原生兜底）
- 搜索结果按仓库分组展示，含匹配行内容和行号
- 点击结果用 VSCode 打开并跳转到对应行

### 1.3 非目标

- 不支持正则表达式（简单文本匹配即可）
- 不做内置文件预览（点击用 VSCode 打开）
- 不搜索二进制文件

---

## 2. 交互设计

### 2.1 触发方式

在 Command Palette 中通过 `:` 前缀进入内容搜索模式。

**完整前缀表**：

| 前缀 | 模式 | 状态 |
|---|---|---|
| 无前缀 | general（文件名搜索） | ✅ 已实现 |
| `#` | workdir（工作目录切换） | ✅ 已实现 |
| `@` | favorites（收藏夹） | ✅ 已实现 |
| `>` | command（命令） | 🔲 预留 |
| **`:`** | **content（内容搜索）** | **🆕 新增** |

### 2.2 搜索范围

**所有搜索统一回车触发**，输入过程中不自动搜索，用户按 Enter 后才开始检索。

| 输入 | 范围 | 触发方式 |
|---|---|---|
| `:keyword` | 当前选中的工作目录 | Enter 触发 |
| `:path/ keyword` | 当前工作目录下指定子目录 | Enter 触发 |
| `::keyword` | 所有工作目录 | Enter 触发 |

**子目录搜索示例**：

```
:AutoBranchConfig              → 当前工作目录中搜索
:src/main/java/ AutoBranch     → src/main/java/ 下搜索
:.java AutoBranch              → .java 文件中搜索
:.java src/main/ AutoBranch    → src/main/ 下 .java 文件中搜索
::AutoBranchConfig             → 所有工作目录中搜索
```

### 2.3 输入解析规则

1. 去掉 `:` 或 `::` 前缀
2. 检查是否以 `.ext` 开头且含空格 → 提取文件类型过滤
3. 检查剩余部分是否含路径（含 `/` 或 `\` 且后跟空格）→ 提取子目录路径
4. 最后的关键词部分作为搜索关键词
5. 如果没有关键词 → 不搜索，等待用户继续输入

### 2.4 全局搜索提示

当用户输入 `::keyword` 时，按 Enter 前显示提示信息：

- 显示：「将在 N 个工作目录中搜索 "keyword"，预计耗时较长，按 Enter 确认」
- 用户按 **Enter** 后开始搜索
- 搜索过程中显示实时进度：「正在搜索 channel-ab-service (3/15)...」

单目录搜索（`:` 前缀）按 Enter 后直接搜索，无额外提示。

### 2.5 搜索结果展示

结果按**仓库分组**显示，每组包含：

- 仓库分组标题（仓库名）
- 每条结果：**文件相对路径 : 行号** + **匹配行内容**（关键词高亮）
- 每个仓库最多显示 20 条结果

### 2.6 点击行为

点击搜索结果 → 调用 `code --goto filePath:lineNum` 用 VSCode 打开文件并跳转到对应行。

### 2.7 右键菜单入口

文件树右键菜单新增「在此目录中搜索」选项：

- 点击后打开 Command Palette
- 自动填入 `:当前相对路径/ `
- 光标在路径后等待用户输入关键词

### 2.8 Placeholder 更新

输入框提示更新为：`搜索文件、目录 (#工作目录 @收藏夹 :内容搜索)`

---

## 3. 后端架构

### 3.1 新增文件

| 文件 | 说明 |
|---|---|
| `service/content_search.go` | 内容搜索服务 |
| `service/content_search_test.go` | 单元测试 |
| `model/content_search.go` | 数据模型 |

### 3.2 搜索策略：ripgrep 优先 + Go 原生降级

```
用户输入 ":keyword"
        │
        ▼
检测系统是否安装 ripgrep (rg)
        │
   ┌────┴────┐
   │ 是      │ 否
   ▼         ▼
 调用 rg   Go 原生搜索
 (高性能)  (filepath.Walk + 逐文件读取)
   │         │
   └────┬────┘
        ▼
  统一结果格式返回前端
```

### 3.3 ripgrep 调用

参数设计：

```
rg --no-heading --line-number --color never -e "keyword" [path]
```

- `--no-heading`：不按文件分组输出（后端自行分组）
- `--line-number`：输出行号
- `--color never`：纯文本输出
- `-e`：固定字符串匹配
- 可选 `--type` 或 `--glob` 用于文件类型过滤

### 3.4 Go 原生搜索

降级实现：

- `filepath.Walk` 遍历目录
- 逐文件 `os.ReadFile` + `strings.Contains` 匹配
- 跳过二进制文件（检测 NUL 字节）
- 并发搜索多个工作目录（`goroutine` + `sync.WaitGroup`）
- 读取文件时按行分割，记录匹配行号和内容

### 3.5 排除目录配置

`AppSettings` 新增字段：

```go
type AppSettings struct {
    GpuDisabled        bool     `json:"gpuDisabled"`
    DefaultShell       string   `json:"defaultShell"`
    GitBashPath        string   `json:"gitBashPath"`
    WslDistro          string   `json:"wslDistro"`
    SearchExcludeDirs  []string `json:"searchExcludeDirs"`  // 排除目录
    SearchExcludeFiles []string `json:"searchExcludeFiles"` // 排除文件模式
}
```

**默认排除目录**：`.git`、`node_modules`、`dist`、`build`、`target`、`.idea`、`__pycache__`、`.gradle`、`bin`、`.settings`

**默认排除文件**：`*.log`、`*.tmp`、`*.class`、`*.jar`、`*.war`

### 3.6 数据模型

```go
// ContentSearchResult 内容搜索结果
type ContentSearchResult struct {
    RepoName string `json:"repoName"`  // 仓库名
    RepoPath string `json:"repoPath"`  // 仓库绝对路径
    FilePath string `json:"filePath"`  // 相对路径
    LineNum  int    `json:"lineNum"`   // 行号
    LineText string `json:"lineText"`  // 匹配行内容
    IsMatch  bool   `json:"isMatch"`   // 标记匹配行（用于前端高亮）
}

// ContentSearchGroup 按仓库分组的结果
type ContentSearchGroup struct {
    RepoName string                  `json:"repoName"`
    RepoPath string                  `json:"repoPath"`
    Items    []*ContentSearchResult  `json:"items"`
}
```

### 3.7 Wails 绑定

`app.go` 新增方法：

```go
// ContentSearch 内容搜索
// query: 搜索关键词
// fileExt: 文件类型过滤（如 ".java"，为空则不过滤）
// subDir: 子目录路径（相对于工作目录，为空则搜索整个目录）
// searchAll: 是否搜索所有工作目录
func (a *App) ContentSearch(query, fileExt, subDir string, searchAll bool) ([]*model.ContentSearchGroup, error)
```

---

## 4. 前端实现

### 4.1 useCommandPalette 改动

`mode` computed 新增判断：

```js
const mode = computed(() => {
  if (input.value.startsWith('::')) return 'content-global'
  if (input.value.startsWith(':'))  return 'content'
  if (input.value.startsWith('#')) return 'workdir'
  if (input.value.startsWith('@')) return 'favorites'
  if (input.value.startsWith('>')) return 'command'
  return 'general'
})
```

新增返回值：

- `contentQuery`：解析后的关键词
- `contentFileExt`：解析后的文件类型过滤
- `contentSubDir`：解析后的子目录路径
- `contentGroups`：搜索结果（按仓库分组）
- `contentSearching`：搜索中状态
- `contentSearchProgress`：搜索进度文本

### 4.2 CommandPalette.vue 新增区域

1. **全局搜索提示**（`mode === 'content-global'` 且未开始搜索时，显示工作目录数量和耗时提示）
2. **内容搜索结果**（按仓库分组，每条显示文件路径:行号 + 匹配行内容）
3. **搜索进度**（显示当前正在搜索的仓库名和进度）

**交互逻辑**：`content` 和 `content-global` 模式下，`@keydown.enter` 触发搜索调用 `ContentSearch` 后端方法。输入过程中仅实时解析查询参数，不发请求。

### 4.3 关键词高亮

匹配行中的关键词用 `<mark>` 标签包裹高亮显示，用户可直观定位：

```js
function highlightMatch(text, keyword) {
  const escaped = keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return text.replace(new RegExp(escaped, 'gi'), '<mark>$&</mark>')
}
```

### 4.4 右键菜单集成

`FileTreePanel.vue` 的右键菜单中新增「在此目录中搜索」：

- 仅在**目录节点**上显示
- 点击后 emit `open-content-search` 事件，携带目录相对路径
- `Home.vue` 监听该事件，打开 Command Palette 并设置 `input` 为 `:path/ `

---

## 5. 设置面板

### 5.1 搜索设置区域

在 `SettingsPanel.vue` 新增「搜索」配置区域：

| 配置项 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| 排除目录 | Tag 列表 | `.git, node_modules, dist, build, target...` | 搜索时跳过的目录 |
| 排除文件 | Tag 列表 | `*.log, *.tmp, *.class...` | 搜索时跳过的文件模式 |
| 每仓库最大结果数 | 数字输入 | 20 | 每个仓库返回的最大匹配数 |

使用 Element Plus 的 `el-tag` 展示已有项，`+添加` 按钮新增，点击 `×` 删除。

---

## 6. 错误处理

| 场景 | 处理方式 |
|---|---|
| 当前目录不存在 | 提示「当前工作目录无效」 |
| 搜索无结果 | 显示「未找到匹配内容」 |
| ripgrep 未安装 | 静默降级为 Go 原生，不提示用户 |
| 文件权限不足 | 跳过该文件，继续搜索 |
| 文件为二进制 | 跳过（检测 NUL 字节） |
| 单目录搜索超时 | 10s 超时，返回已找到的结果 |
| 全局搜索超时 | 60s 超时，返回已找到的结果 |
| 关键词为空 | 不触发搜索 |

---

## 7. 性能预期

| 场景 | 文件数 | Go 原生 | ripgrep |
|---|---|---|---|
| 单目录（中小项目） | ~500 | < 1s | < 0.1s |
| 单目录（大型项目） | ~5000 | 2-5s | < 0.5s |
| 全局 15 个仓库 | ~30000 | 10-30s | 1-3s |

---

## 8. 实施要点

1. 后端先实现 `ContentSearchService`，包含 ripgrep 检测和 Go 原生降级
2. 扩展 `AppSettings` 模型和设置面板
3. 前端修改 `useCommandPalette` 支持 `:` 模式
4. 修改 `CommandPalette.vue` 新增内容搜索结果区域
5. 文件树右键菜单新增「在此目录中搜索」入口
6. 单元测试覆盖搜索逻辑、解析逻辑、ripgrep 降级逻辑
