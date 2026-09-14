# 开发者工作台 - 渐进式开发计划

**项目名称**：WorkBench  
**技术栈**：Wails v2.5+ + Go 1.21+ + Vue3 + Element Plus  
**平台**：Windows 10/11  
**文档版本**：1.0  
**创建日期**：2025-04-28  
**作者**：Claude  

---

## 目录

1. [Phase 1: 项目初始化](#phase-1-项目初始化)
2. [Phase 2: 环境配置](#phase-2-环境配置)
3. [Phase 3: 模型编写](#phase-3-模型编写)
4. [Phase 4: 中间件开发](#phase-4-中间件开发)
5. [Phase 5: 接口实现](#phase-5-接口实现)
6. [Phase 6: 入口配置](#phase-6-入口配置)
7. [Phase 7: 接口测试](#phase-7-接口测试)
8. [附录](#附录)

---

## Phase 1: 项目初始化

### 目标
创建Wails项目骨架，建立基本目录结构，配置基础开发环境。

### 前置条件检查

在开始之前，请确认以下环境已安装：

```bash
# 检查Go版本（需要1.21+）
go version

# 检查Node.js版本（需要16+）
node --version

# 检查Wails CLI
wails version

# 检查Git
git --version
```

如果任何命令失败，请先完成Phase 0的环境准备。

---

### 1.1 创建Wails项目

**操作步骤**：

1. 在工作目录执行：
```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools
wails init -n workbench -t vue3
```

2. 选择模板时确认使用 `vue3` 模板。

3. 等待项目初始化完成。

**生成的目录结构**：
```
workbench/
├── main.go                 # Go入口文件
├── app.go                  # 应用结构体
├── wails.json              # Wails配置文件
├── frontend/               # Vue3前端
│   ├── src/
│   │   ├── App.vue
│   │   ├── main.js
│   │   └── wailsjs/        # Wails绑定代码（自动生成）
│   ├── package.json
│   ├── wailsjs/
│   └── index.html
├── build/                  # 构建输出
├── go.mod                  # Go模块定义
└── go.sum
```

**验证方法**：
```bash
cd workbench
dir  # Windows命令，查看目录结构
```

**预期输出**：应该看到上述目录和文件。

**常见问题**：
- **问题**：`wails: command not found`
- **解决**：确认Go bin目录在PATH中，或使用完整路径：`go run github.com/wailsapp/wails/v2/cmd/wails@latest init`

---

### 1.2 清理默认示例代码

**目标**：移除Wails生成的示例代码，为项目开发做准备。

**操作步骤**：

1. 打开 `frontend/src/App.vue`，清空模板：
```vue
<template>
  <div id="app">
    <!-- WorkBench -->
  </div>
</template>

<script setup>
import { ref } from 'vue'
</script>

<style>
#app {
  width: 100%;
  height: 100vh;
  margin: 0;
  padding: 0;
}
</style>
```

2. 打开 `main.go`，保留基本结构：
```go
package main

import (
    "embed"
    
    "github.com/wailsapp/wails/v2"
    "github.com/wailsapp/wails/v2/pkg/options"
    "github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
    // 创建应用实例
    app := NewApp()
    
    // 运行应用
    err := wails.Run(&options.App{
        Title:  "WorkBench",
        Width:  1280,
        Height: 800,
        AssetServer: &assetserver.Options{
            Assets: assets,
        },
        OnStartup: app.startup,
        OnShutdown: app.shutdown,
        Bind: []interface{}{
            app,
        },
    })
    
    if err != nil {
        println("Error:", err.Error())
    }
}
```

3. 打开 `app.go`，保留基本结构：
```go
package main

import (
    "context"
)

type App struct {
    ctx context.Context
}

func NewApp() *App {
    return &App{}
}

func (a *App) startup(context.Context) {
    println("WorkBench starting up...")
}

func (a *App) shutdown(context.Context) {
    println("WorkBench shutting down...")
}
```

**验证方法**：
```bash
wails dev
```

**预期输出**：
- Wails开发服务器启动
- 自动打开浏览器（或应用窗口）
- 控制台显示 "WorkBench starting up..."
- 窗口显示空白页面（只有 "WorkBench" 文字）

**常见问题**：
- **问题**：端口被占用
- **解决**：修改 `wails.json` 中的端口配置，或关闭占用端口的程序

---

### 1.3 配置wails.json

**目标**：配置应用的基本信息。

**文件路径**：`workbench/wails.json`

**完整配置**：
```json
{
  "name": "workbench",
  "outputfilename": "workbench",
  "frontend:install": "npm install",
  "frontend:build": "npm run build",
  "frontend:dev:watcher": "npm run dev",
  "frontend:dev:serverUrl": "auto",
  "author": {
    "name": "Your Name",
    "email": "your.email@example.com"
  },
  "info": {
    "companyName": "Personal",
    "productName": "WorkBench",
    "productVersion": "1.0.0",
    "copyright": "Copyright ............ 2025",
    "comments": "开发者工作台"
  },
  "wailsjsdir": "./frontend",
  "version": "2",
  "outputType": "desktop"
}
```

**验证方法**：
```bash
wails build -clean
```

**预期输出**：
- 在 `build/bin/` 目录生成 `workbench.exe`
- 双击exe文件能启动应用（显示空白窗口）

---

### 1.4 创建项目目录结构

**目标**：创建Go代码的组织结构。

**操作步骤**：

1. 创建目录：
```bash
cd workbench
mkdir model
mkdir service
mkdir util
mkdir data
```

2. 创建基础文件：
```bash
# 模型文件
type nul > model\models.go

# 服务文件
type nul > service\directory.go
type nul > service\filetree.go
type nul > service\fileoperation.go
type nul > service\git.go

# 工具文件
type nul > util\json.go
type nul > util\git.go
type nul > util\file.go

# 配置文件
type nul > data\directories.json
```

**目录结构**：
```
workbench/
├── main.go
├── app.go
├── model/              # 数据模型
│   └── models.go
├── service/            # 业务逻辑
│   ├── directory.go
│   ├── filetree.go
│   ├── fileoperation.go
│   └── git.go
├── util/               # 工具函数
│   ├── json.go
│   ├── git.go
│   └── file.go
├── data/               # 数据文件
│   └── directories.json
└── frontend/           # Vue3前端
```

**验证方法**：
```bash
dir /s /b
```

**预期输出**：显示所有创建的文件和目录。

---

### 1.5 创建.gitignore

**目标**：配置Git忽略文件。

**文件路径**：`workbench/.gitignore`

**完整内容**：
```gitignore
# Binaries for programs and plugins
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test

# Test binary, built with `go test -c`
*.out

# Go workspace file
go.work

# Dependency directories
vendor/

# Wails build output
build/
frontend/dist/
frontend/wailsjs/

# IDE specific files
.idea/
.vscode/
*.swp
*.swo
*~

# OS specific files
.DS_Store
Thumbs.db

# Application specific
data/*.json
!data/directories.json.template
```

**验证方法**：
```bash
git init
git add .gitignore
git commit -m "Initial commit: Add .gitignore"
```

---

### 1.6 创建README.md

**目标**：创建项目说明文档。

**文件路径**：`workbench/README.md`

**完整内容**：
```markdown
# WorkBench

一个基于Wails的开发者工作台，用于可视化管理和操作本地Git仓库。

## 技术栈

- **后端**：Go 1.21+
- **前端**：Vue3 + Element Plus
- **框架**：Wails v2.5+
- **平台**：Windows 10/11

## 功能特性

- 工作目录管理（JSON持久化）
- 文件树浏览（懒加载、Git仓库检测）
- 文件操作（创建、删除、重命名、预览）
- Git集成（克隆、拉取、提交历史）

## 开发

### 环境要求

- Go 1.21+
- Node.js 16+
- Wails CLI
- Git

### 开发模式

\`\`\`bash
wails dev
\`\`\`

### 构建

\`\`\`bash
wails build
\`\`\`

## 许可证

MIT
```

---

### Phase 1 检查点

**验证清单**：

- [ ] Wails项目成功创建
- [ ] 目录结构正确
- [ ] `wails dev` 能启动应用
- [ ] `wails build` 能生成exe文件
- [ ] Git仓库初始化完成

**成功标志**：
✅ 能运行 `wails dev` 并看到空白窗口  
✅ 能运行 `wails build` 并生成可执行文件  
✅ 项目目录结构完整

---

## Phase 2: 环境配置

### 目标
配置开发环境和基础依赖，包括Element Plus安装、Vue配置、应用图标准备。

### 2.1 安装Element Plus

**操作步骤**：

1. 进入前端目录：
```bash
cd workbench/frontend
```

2. 安装Element Plus：
```bash
npm install element-plus
```

3. 验证安装：
```bash
npm list element-plus
```

**预期输出**：
```
element-plus@2.x.x
```

---

### 2.2 配置Vue3 main.js

**目标**：在Vue应用中引入Element Plus。

**文件路径**：`workbench/frontend/src/main.js`

**完整代码**：
```javascript
import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'

const app = createApp(App)

app.use(ElementPlus)
app.mount('#app')
```

---

### 2.3 测试Element Plus安装

**目标**：验证Element Plus组件可用。

**文件路径**：`workbench/frontend/src/App.vue`

**测试代码**：
```vue
<template>
  <div id="app">
    <el-button type="primary">Element Plus按钮</el-button>
    <el-date-picker
      v-model="date"
      type="date"
      placeholder="选择日期">
    </el-date-picker>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const date = ref('')
</script>

<style>
#app {
  padding: 20px;
}
</style>
```

**验证方法**：
```bash
cd workbench
wails dev
```

**预期输出**：
- 应用窗口显示Element Plus按钮和日期选择器
- 组件样式正确，交互正常

---

### 2.4 创建基础页面布局

**目标**：创建应用的主布局结构。

**文件路径**：`workbench/frontend/src/App.vue`

**完整代码**：
```vue
<template>
  <div id="app">
    <el-container style="height: 100vh">
      <!-- 顶部工具栏 -->
      <el-header style="background-color: #545c64; display: flex; align-items: center;">
        <span style="color: white; font-size: 18px;">开发者工作台</span>
        <el-divider direction="vertical" />
        <el-select 
          v-model="selectedDirectory" 
          placeholder="选择工作目录"
          style="width: 300px;">
          <el-option
            v-for="dir in directories"
            :key="dir.id"
            :label="dir.name"
            :value="dir.id">
          </el-option>
        </el-select>
        <el-button @click="addDirectory" style="margin-left: 10px;">添加目录</el-button>
      </el-header>

      <!-- 主体内容 -->
      <el-container>
        <!-- 左侧文件树 -->
        <el-aside width="300px" style="border-right: 1px solid #e6e6e6;">
          <div style="padding: 10px;">
            <el-button-group style="margin-bottom: 10px;">
              <el-button size="small">全部展开</el-button>
              <el-button size="small">全部收起</el-button>
            </el-button-group>
          </div>
          <el-tree
            :data="fileTree"
            :props="treeProps"
            lazy
            :load="loadNode"
            @node-click="handleNodeClick">
            <template #default="{ node, data }">
              <span class="custom-tree-node">
                <span>{{ node.label }}</span>
                <span v-if="data.isGitRepo" style="color: #67C23A; margin-left: 5px;">✓</span>
              </span>
            </template>
          </el-tree>
        </el-aside>

        <!-- 右侧操作面板 -->
        <el-main>
          <div v-if="selectedNode">
            <h3>{{ selectedNode.name }}</h3>
            <p>路径: {{ selectedNode.path }}</p>
            <p>类型: {{ selectedNode.type }}</p>
            <!-- 更多内容将在后续阶段实现 -->
          </div>
          <div v-else>
            <el-empty description="请选择文件或文件夹" />
          </div>
        </el-main>
      </el-container>
    </el-container>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const selectedDirectory = ref('')
const directories = ref([])
const fileTree = ref([])
const selectedNode = ref(null)

const treeProps = {
  label: 'name',
  children: 'children',
  isLeaf: 'isLeaf'
}

// 添加目录
const addDirectory = () => {
  console.log('添加目录功能待实现')
}

// 加载节点（懒加载）
const loadNode = (node, resolve) => {
  console.log('加载节点功能待实现')
  resolve([])
}

// 节点点击事件
const handleNodeClick = (data) => {
  selectedNode.value = data
}

onMounted(() => {
  // 初始化数据
  console.log('应用已启动')
})
</script>

<style>
#app {
  margin: 0;
  padding: 0;
  font-family: 'Microsoft YaHei', Arial, sans-serif;
}

.custom-tree-node {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 14px;
  padding-right: 8px;
}

.el-header {
  padding: 0 20px;
}

.el-aside {
  background-color: #f5f7fa;
}

.el-main {
  padding: 20px;
}
</style>
```

**验证方法**：
```bash
wails dev
```

**预期输出**：
- 显示完整的应用布局
- 顶部有工具栏和下拉框
- 左侧有文件树区域
- 右侧有操作面板
- 所有Element Plus组件正常显示

---

### 2.5 准备应用图标

**目标**：为应用准备图标文件。

**操作步骤**：

1. 使用在线工具或设计软件创建图标：
   - 尺寸：512x512 像素
   - 格式：PNG（透明背景）
   - 设计：建议使用Git相关的图标（如分支、仓库图标）

2. 将图标保存到：
```
workbench/build/icon.png
```

3. Windows图标还需要.ico格式，可以使用在线转换工具：
   - 访问：https://convertico.com/
   - 上传PNG文件，转换为ICO
   - 保存到：`workbench/build/app.ico`

4. 更新 `wails.json`：
```json
{
  ...,
  "author": {
    ...
  },
  "info": {
    ...
  },
  "icon": "build/app.ico"
}
```

**验证方法**：
```bash
wails build
```

**预期输出**：
- 生成的exe文件显示自定义图标
- 任务栏图标正确显示

---

### 2.6 创建配置文件模板

**目标**：创建directories.json的模板文件。

**文件路径**：`workbench/data/directories.json.template`

**完整内容**：
```json
{
  "directories": [
    {
      "id": "default-1",
      "name": "默认工作空间",
      "path": "C:\\Users\\YourName\\workspace",
      "isDefault": true,
      "createTime": "2025-04-28T10:00:00+08:00"
    }
  ]
}
```

**使用说明**：
- 首次运行时，如果 `data/directories.json` 不存在，复制模板文件
- 用户需要修改路径为实际的工作目录

---

### Phase 2 检查点

**验证清单**：

- [ ] Element Plus安装成功
- [ ] Vue应用能正常启动
- [ ] 基础页面布局显示正常
- [ ] 应用图标配置完成
- [ ] 所有Element Plus组件可用

**成功标志**：
✅ `wails dev` 启动后显示完整的应用界面  
✅ 所有Element Plus组件正常工作  
✅ 应用图标正确显示  

---

## Phase 3: 模型编写

### 目标
定义所有数据模型和结构体，包括工作目录、文件树节点、Git信息等。

### 3.1 创建model包

**操作步骤**：

1. 打开 `model/models.go`

2. 导入必要的包：
```go
package model

import "time"
```

---

### 3.2 Directory结构体

**目标**：定义工作目录模型。

**代码实现**：

```go
// Directory 工作目录配置
type Directory struct {
    ID         string    `json:"id"`
    Name       string    `json:"name"`
    Path       string    `json:"path"`
    IsDefault  bool      `json:"isDefault"`
    CreateTime time.Time `json:"createTime"`
}

// NewDirectory 创建新的工作目录
func NewDirectory(name, path string, isDefault bool) *Directory {
    return &Directory{
        ID:         generateID(),
        Name:       name,
        Path:       path,
        IsDefault:  isDefault,
        CreateTime: time.Now(),
    }
}

// Validate 验证工作目录配置
func (d *Directory) Validate() error {
    if d.Name == "" {
        return fmt.Errorf("目录名称不能为空")
    }
    if d.Path == "" {
        return fmt.Errorf("目录路径不能为空")
    }
    return nil
}

// generateID 生成唯一ID
func generateID() string {
    return fmt.Sprintf("dir-%d", time.Now().UnixNano())
}
```

**需要添加的导入**：
```go
import (
    "fmt"
    "time"
)
```

---

### 3.3 FileTreeNode结构体

**目标**：定义文件树节点模型。

**代码实现**：

```go
// FileTreeNode 文件树节点
type FileTreeNode struct {
    ID         string           `json:"id"`
    Name       string           `json:"name"`
    Path       string           `json:"path"`
    Type       string           `json:"type"`        // "file" 或 "directory"
    IsGitRepo  bool             `json:"isGitRepo"`
    HasChildren bool             `json:"hasChildren"`
    Children   []*FileTreeNode  `json:"children,omitempty"`
    IsLeaf     bool             `json:"isLeaf"`
}

// NewFileTreeNode 创建文件树节点
func NewFileTreeNode(name, path, fileType string) *FileTreeNode {
    return &FileTreeNode{
        ID:         path,
        Name:       name,
        Path:       path,
        Type:       fileType,
        IsGitRepo:  false,
        HasChildren: fileType == "directory",
        IsLeaf:     fileType == "file",
    }
}
```

---

### 3.4 GitCommit结构体

**目标**：定义Git提交记录模型。

**代码实现**：

```go
// GitCommit Git提交记录
type GitCommit struct {
    Hash    string    `json:"hash"`
    Author  string    `json:"author"`
    Date    time.Time `json:"date"`
    Message string    `json:"message"`
}

// ShortHash 返回短哈希（前7位）
func (c *GitCommit) ShortHash() string {
    if len(c.Hash) > 7 {
        return c.Hash[:7]
    }
    return c.Hash
}
```

---

### 3.5 GitRepoInfo结构体

**目标**：定义Git仓库信息模型。

**代码实现**：

```go
// GitRepoInfo Git仓库信息
type GitRepoInfo struct {
    Path     string       `json:"path"`
    Branch   string       `json:"branch"`
    Remote   string       `json:"remote"`
    RemoteURL string      `json:"remoteUrl"`
    Commits  []GitCommit  `json:"commits"`
    IsRepo   bool         `json:"isRepo"`
}
```

---

### 3.6 PageResult结构体

**目标**：定义分页结果模型。

**代码实现**：

```go
// PageResult 分页结果
type PageResult struct {
    Records interface{} `json:"records"`
    Total   int64       `json:"total"`
    Current int         `json:"current"`
    Size    int         `json:"size"`
    Pages   int         `json:"pages"`
}

// NewPageResult 创建分页结果
func NewPageResult(records interface{}, total int64, current, size int) *PageResult {
    pages := int(total) / size
    if int(total)%size != 0 {
        pages++
    }
    
    return &PageResult{
        Records: records,
        Total:   total,
        Current: current,
        Size:    size,
        Pages:   pages,
    }
}
```

---

### 3.7 FilePreview结构体

**目标**：定义文件预览模型。

**代码实现**：

```go
// FilePreview 文件预览
type FilePreview struct {
    Path      string `json:"path"`
    Name      string `json:"name"`
    Size      int64  `json:"size"`
    Content   string `json:"content,omitempty"`
    IsBinary  bool   `json:"isBinary"`
    TooLarge  bool   `json:"tooLarge"`
    Error     string `json:"error,omitempty"`
}
```

---

### 3.8 完整的models.go文件

**文件路径**：`model/models.go`

**完整代码**：

```go
package model

import (
    "fmt"
    "time"
)

// Directory 工作目录配置
type Directory struct {
    ID         string    `json:"id"`
    Name       string    `json:"name"`
    Path       string    `json:"path"`
    IsDefault  bool      `json:"isDefault"`
    CreateTime time.Time `json:"createTime"`
}

// NewDirectory 创建新的工作目录
func NewDirectory(name, path string, isDefault bool) *Directory {
    return &Directory{
        ID:         generateID(),
        Name:       name,
        Path:       path,
        IsDefault:  isDefault,
        CreateTime: time.Now(),
    }
}

// Validate 验证工作目录配置
func (d *Directory) Validate() error {
    if d.Name == "" {
        return fmt.Errorf("目录名称不能为空")
    }
    if d.Path == "" {
        return fmt.Errorf("目录路径不能为空")
    }
    return nil
}

// FileTreeNode 文件树节点
type FileTreeNode struct {
    ID         string           `json:"id"`
    Name       string           `json:"name"`
    Path       string           `json:"path"`
    Type       string           `json:"type"`
    IsGitRepo  bool             `json:"isGitRepo"`
    HasChildren bool             `json:"hasChildren"`
    Children   []*FileTreeNode  `json:"children,omitempty"`
    IsLeaf     bool             `json:"isLeaf"`
}

// NewFileTreeNode 创建文件树节点
func NewFileTreeNode(name, path, fileType string) *FileTreeNode {
    return &FileTreeNode{
        ID:         path,
        Name:       name,
        Path:       path,
        Type:       fileType,
        IsGitRepo:  false,
        HasChildren: fileType == "directory",
        IsLeaf:     fileType == "file",
    }
}

// GitCommit Git提交记录
type GitCommit struct {
    Hash    string    `json:"hash"`
    Author  string    `json:"author"`
    Date    time.Time `json:"date"`
    Message string    `json:"message"`
}

// ShortHash 返回短哈希（前7位）
func (c *GitCommit) ShortHash() string {
    if len(c.Hash) > 7 {
        return c.Hash[:7]
    }
    return c.Hash
}

// GitRepoInfo Git仓库信息
type GitRepoInfo struct {
    Path      string      `json:"path"`
    Branch    string      `json:"branch"`
    Remote    string      `json:"remote"`
    RemoteURL string      `json:"remoteUrl"`
    Commits   []GitCommit `json:"commits"`
    IsRepo    bool        `json:"isRepo"`
}

// PageResult 分页结果
type PageResult struct {
    Records interface{} `json:"records"`
    Total   int64       `json:"total"`
    Current int         `json:"current"`
    Size    int         `json:"size"`
    Pages   int         `json:"pages"`
}

// NewPageResult 创建分页结果
func NewPageResult(records interface{}, total int64, current, size int) *PageResult {
    pages := int(total) / size
    if int(total)%size != 0 {
        pages++
    }
    
    return &PageResult{
        Records: records,
        Total:   total,
        Current: current,
        Size:    size,
        Pages:   pages,
    }
}

// FilePreview 文件预览
type FilePreview struct {
    Path     string `json:"path"`
    Name     string `json:"name"`
    Size     int64  `json:"size"`
    Content  string `json:"content,omitempty"`
    IsBinary bool   `json:"isBinary"`
    TooLarge bool   `json:"tooLarge"`
    Error    string `json:"error,omitempty"`
}

// generateID 生成唯一ID
func generateID() string {
    return fmt.Sprintf("dir-%d", time.Now().UnixNano())
}
```

---

### 3.9 编写模型单元测试

**目标**：测试模型的构造和验证方法。

**文件路径**：`model/models_test.go`

**完整代码**：

```go
package model

import (
    "testing"
    "time"
)

func TestNewDirectory(t *testing.T) {
    dir := NewDirectory("测试目录", "C:\\test", true)
    
    if dir.Name != "测试目录" {
        t.Errorf("期望名称为 '测试目录', 实际为 '%s'", dir.Name)
    }
    
    if dir.Path != "C:\\test" {
        t.Errorf("期望路径为 'C:\\test', 实际为 '%s'", dir.Path)
    }
    
    if !dir.IsDefault {
        t.Error("期望 IsDefault 为 true")
    }
    
    if dir.ID == "" {
        t.Error("ID 不应为空")
    }
}

func TestDirectoryValidate(t *testing.T) {
    tests := []struct {
        name    string
        dir     *Directory
        wantErr bool
    }{
        {
            name: "有效目录",
            dir: &Directory{
                Name: "测试",
                Path: "C:\\test",
            },
            wantErr: false,
        },
        {
            name: "空名称",
            dir: &Directory{
                Name: "",
                Path: "C:\\test",
            },
            wantErr: true,
        },
        {
            name: "空路径",
            dir: &Directory{
                Name: "测试",
                Path: "",
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.dir.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}

func TestNewFileTreeNode(t *testing.T) {
    node := NewFileTreeNode("test.txt", "C:\\test.txt", "file")
    
    if node.Name != "test.txt" {
        t.Errorf("期望名称为 'test.txt', 实际为 '%s'", node.Name)
    }
    
    if node.Type != "file" {
        t.Errorf("期望类型为 'file', 实际为 '%s'", node.Type)
    }
    
    if !node.IsLeaf {
        t.Error("文件节点 IsLeaf 应为 true")
    }
    
    if node.HasChildren {
        t.Error("文件节点 HasChildren 应为 false")
    }
}

func TestGitCommitShortHash(t *testing.T) {
    commit := &GitCommit{
        Hash: "abc1234567890",
    }
    
    shortHash := commit.ShortHash()
    if shortHash != "abc1234" {
        t.Errorf("期望短哈希为 'abc1234', 实际为 '%s'", shortHash)
    }
}

func TestNewPageResult(t *testing.T) {
    records := []int{1, 2, 3}
    result := NewPageResult(records, 25, 2, 10)
    
    if result.Records == nil {
        t.Error("Records 不应为 nil")
    }
    
    if result.Total != 25 {
        t.Errorf("期望 Total 为 25, 实际为 %d", result.Total)
    }
    
    if result.Current != 2 {
        t.Errorf("期望 Current 为 2, 实际为 %d", result.Current)
    }
    
    if result.Size != 10 {
        t.Errorf("期望 Size 为 10, 实际为 %d", result.Size)
    }
    
    if result.Pages != 3 {
        t.Errorf("期望 Pages 为 3, 实际为 %d", result.Pages)
    }
}
```

**验证方法**：
```bash
cd workbench
go test ./model -v
```

**预期输出**：
```
=== RUN   TestNewDirectory
--- PASS: TestNewDirectory (0.00s)
=== RUN   TestDirectoryValidate
--- PASS: TestDirectoryValidate (0.00s)
=== RUN   TestNewFileTreeNode
--- PASS: TestNewFileTreeNode (0.00s)
=== RUN   TestGitCommitShortHash
--- PASS: TestGitCommitShortHash (0.00s)
=== RUN   TestNewPageResult
--- PASS: TestNewPageResult (0.00s)
PASS
ok      workbench/model    0.002s
```

---

### Phase 3 检查点

**验证清单**：

- [ ] 所有结构体定义完成
- [ ] 所有构造函数实现
- [ ] 所有验证方法实现
- [ ] 单元测试通过
- [ ] 代码编译无错误

**成功标志**：
✅ `go build` 编译成功  
✅ `go test ./model -v` 所有测试通过  
✅ 所有模型字段有正确的JSON标签  

---

## Phase 4: 中间件开发

### 目标
实现工具类和服务层，包括JSON配置读写、文件操作、Git命令执行等。

### 4.1 创建util包 - json.go

**目标**：实现JSON配置文件的读写功能。

**文件路径**：`util/json.go`

**完整代码**：

```go
package util

import (
    "encoding/json"
    "os"
    "path/filepath"
)

// ConfigFile 配置文件结构
type ConfigFile struct {
    Directories interface{} `json:"directories"`
}

// LoadJSON 加载JSON文件
func LoadJSON(filePath string, v interface{}) error {
    data, err := os.ReadFile(filePath)
    if err != nil {
        return err
    }
    
    return json.Unmarshal(data, v)
}

// SaveJSON 保存到JSON文件
func SaveJSON(filePath string, v interface{}) error {
    // 确保目录存在
    dir := filepath.Dir(filePath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    
    data, err := json.MarshalIndent(v, "", "  ")
    if err != nil {
        return err
    }
    
    return os.WriteFile(filePath, data, 0644)
}

// FileExists 检查文件是否存在
func FileExists(filePath string) bool {
    _, err := os.Stat(filePath)
    return !os.IsNotExist(err)
}
```

---

### 4.2 创建util包 - git.go

**目标**：实现Git命令执行工具。

**文件路径**：`util/git.go`

**完整代码**：

```go
package util

import (
    "bufio"
    "bytes"
    "context"
    "fmt"
    "os"
    "os/exec"
    "strings"
    "time"
)

// GitCommand Git命令执行器
type GitCommand struct {
    timeout time.Duration
}

// NewGitCommand 创建Git命令执行器
func NewGitCommand() *GitCommand {
    return &GitCommand{
        timeout: 30 * time.Second,
    }
}

// Execute 执行Git命令
func (g *GitCommand) Execute(workDir string, args ...string) (string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, "git", args...)
    cmd.Dir = workDir
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    err := cmd.Run()
    if err != nil {
        return "", fmt.Errorf("git %v failed: %s", args, stderr.String())
    }
    
    return stdout.String(), nil
}

// ExecuteWithOutput 执行Git命令并返回带行分割的输出
func (g *GitCommand) ExecuteWithOutput(workDir string, args ...string) ([]string, error) {
    output, err := g.Execute(workDir, args...)
    if err != nil {
        return nil, err
    }
    
    lines := strings.Split(strings.TrimSpace(output), "\n")
    return lines, nil
}

// IsGitRepository 检查目录是否是Git仓库
func (g *GitCommand) IsGitRepository(dir string) bool {
    cmd := exec.Command("git", "rev-parse", "--git-dir")
    cmd.Dir = dir
    
    err := cmd.Run()
    return err == nil
}

// GetBranch 获取当前分支名
func (g *GitCommand) GetBranch(dir string) (string, error) {
    return g.Execute(dir, "branch", "--show-current")
}

// GetRemote 获取远程仓库URL
func (g *GitCommand) GetRemote(dir string) (string, string, error) {
    lines, err := g.ExecuteWithOutput(dir, "remote", "-v")
    if err != nil {
        return "", "", err
    }
    
    if len(lines) == 0 {
        return "", "", fmt.Errorf("no remote configured")
    }
    
    // 解析远程信息：origin  https://github.com/user/repo.git (fetch)
    parts := strings.Fields(lines[0])
    if len(parts) < 2 {
        return "", "", fmt.Errorf("invalid remote format")
    }
    
    remoteName := parts[0]
    remoteURL := strings.TrimSuffix(parts[1], " (fetch)")
    
    return remoteName, remoteURL, nil
}

// GetLog 获取提交历史
func (g *GitCommand) GetLog(dir string, offset, limit int) ([]string, error) {
    args := []string{
        "log",
        "--pretty=format:%H|%an|%ad|%s",
        "--date=iso",
        fmt.Sprintf("--skip=%d", offset),
        fmt.Sprintf("-n=%d", limit),
    }
    
    lines, err := g.ExecuteWithOutput(dir, args...)
    if err != nil {
        return nil, err
    }
    
    return lines, nil
}

// GetTotalCommits 获取总提交数
func (g *GitCommand) GetTotalCommits(dir string) (int, error) {
    output, err := g.Execute(dir, "rev-list", "--count", "HEAD")
    if err != nil {
        return 0, err
    }
    
    var count int
    _, err = fmt.Sscanf(strings.TrimSpace(output), "%d", &count)
    return count, err
}

// Clone 克隆仓库
func (g *GitCommand) Clone(url, targetPath string) (string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, "git", "clone", url, targetPath)
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    // 创建实时输出的管道
    stdoutPipe, err := cmd.StdoutPipe()
    if err != nil {
        return "", err
    }
    
    if err := cmd.Start(); err != nil {
        return "", err
    }
    
    // 实时读取输出
    scanner := bufio.NewScanner(stdoutPipe)
    var output strings.Builder
    for scanner.Scan() {
        line := scanner.Text()
        output.WriteString(line + "\n")
        println(line) // 打印到控制台
    }
    
    if err := cmd.Wait(); err != nil {
        return "", fmt.Errorf("clone failed: %s", stderr.String())
    }
    
    return output.String(), nil
}

// Pull 拉取更新
func (g *GitCommand) Pull(dir string) (string, error) {
    return g.Execute(dir, "pull")
}
```

---

### 4.3 创建util包 - file.go

**目标**：实现文件操作工具函数。

**文件路径**：`util/file.go`

**完整代码**：

```go
package util

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
)

// IsPreviewable 判断文件是否可预览
func IsPreviewable(filename string) bool {
    ext := strings.ToLower(filepath.Ext(filename))
    
    previewableExts := []string{
        ".txt", ".md", ".markdown",
        ".json", ".xml", ".yaml", ".yml", ".properties", ".conf", ".config",
        ".js", ".ts", ".jsx", ".tsx", ".vue",
        ".java", ".py", ".go", ".rs", ".c", ".cpp", ".h", ".cs",
        ".html", ".css", ".scss", ".less",
        ".sh", ".bat", ".ps1", ".cmd",
        ".gitignore", ".env", "dockerfile",
    }
    
    for _, pe := range previewableExts {
        if ext == pe || strings.ToLower(filepath.Base(filename)) == pe {
            return true
        }
    }
    
    return false
}

// FormatFileSize 格式化文件大小
func FormatFileSize(size int64) string {
    const unit = 1024
    if size < unit {
        return fmt.Sprintf("%d B", size)
    }
    
    div, exp := int64(unit), 0
    for n := size / unit; n >= unit; n /= unit {
        div *= unit
        exp++
    }
    
    return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// ReadFileSafe 安全读取文件（限制大小）
func ReadFileSafe(filePath string, maxSize int64) ([]byte, error) {
    info, err := os.Stat(filePath)
    if err != nil {
        return nil, err
    }
    
    if info.Size() > maxSize {
        return nil, fmt.Errorf("file too large: %d bytes (max %d bytes)", info.Size(), maxSize)
    }
    
    return os.ReadFile(filePath)
}

// CreateDirectory 创建目录
func CreateDirectory(path string) error {
    return os.MkdirAll(path, 0755)
}

// CreateFile 创建文件
func CreateFile(path string, content string) error {
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    
    file, err := os.Create(path)
    if err != nil {
        return err
    }
    defer file.Close()
    
    if content != "" {
        _, err = file.WriteString(content)
        if err != nil {
            return err
        }
    }
    
    return nil
}

// RenamePath 重命名文件或目录
func RenamePath(oldPath, newPath string) error {
    return os.Rename(oldPath, newPath)
}

// RemovePath 删除文件或目录
func RemovePath(path string) error {
    return os.RemoveAll(path)
}

// CopyFile 复制文件
func CopyFile(src, dst string) error {
    sourceFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer sourceFile.Close()
    
    // 创建目标目录
    dir := filepath.Dir(dst)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    
    destFile, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer destFile.Close()
    
    _, err = io.Copy(destFile, sourceFile)
    return err
}

// IsDirectory 检查是否是目录
func IsDirectory(path string) (bool, error) {
    info, err := os.Stat(path)
    if err != nil {
        return false, err
    }
    return info.IsDir(), nil
}
```

---

### 4.4 创建service包 - directory.go

**目标**：实现工作目录管理服务。

**文件路径**：`service/directory.go`

**完整代码**：

```go
package service

import (
    "fmt"
    "os"
    "path/filepath"
    
    "workbench/model"
    "workbench/util"
)

// DirectoryService 工作目录服务
type DirectoryService struct {
    configPath string
}

// NewDirectoryService 创建工作目录服务
func NewDirectoryService(configPath string) *DirectoryService {
    return &DirectoryService{
        configPath: configPath,
    }
}

// Config 配置结构
type Config struct {
    Directories []*model.Directory `json:"directories"`
}

// Load 加载工作目录配置
func (s *DirectoryService) Load() ([]*model.Directory, error) {
    if !util.FileExists(s.configPath) {
        // 返回空列表
        return []*model.Directory{}, nil
    }
    
    var config Config
    err := util.LoadJSON(s.configPath, &config)
    if err != nil {
        return nil, fmt.Errorf("加载配置失败: %w", err)
    }
    
    return config.Directories, nil
}

// Save 保存工作目录配置
func (s *DirectoryService) Save(directories []*model.Directory) error {
    config := Config{
        Directories: directories,
    }
    
    return util.SaveJSON(s.configPath, config)
}

// Create 创建工作目录
func (s *DirectoryService) Create(name, path string, isDefault bool) (*model.Directory, error) {
    // 验证路径是否存在
    if !util.FileExists(path) {
        return nil, fmt.Errorf("路径不存在: %s", path)
    }
    
    // 转换为绝对路径
    absPath, err := filepath.Abs(path)
    if err != nil {
        return nil, fmt.Errorf("获取绝对路径失败: %w", err)
    }
    
    // 检查是否已存在
    directories, _ := s.Load()
    for _, dir := range directories {
        if dir.Path == absPath {
            return nil, fmt.Errorf("该目录已添加")
        }
    }
    
    // 创建新目录
    newDir := model.NewDirectory(name, absPath, isDefault)
    
    // 如果设置为默认，取消其他默认
    if isDefault {
        for _, dir := range directories {
            dir.IsDefault = false
        }
    }
    
    directories = append(directories, newDir)
    
    // 保存
    if err := s.Save(directories); err != nil {
        return nil, err
    }
    
    return newDir, nil
}

// Update 更新工作目录
func (s *DirectoryService) Update(id, name, path string, isDefault bool) (*model.Directory, error) {
    directories, err := s.Load()
    if err != nil {
        return nil, err
    }
    
    var target *model.Directory
    for _, dir := range directories {
        if dir.ID == id {
            target = dir
            break
        }
    }
    
    if target == nil {
        return nil, fmt.Errorf("工作目录不存在")
    }
    
    // 验证路径
    if path != target.Path {
        if !util.FileExists(path) {
            return nil, fmt.Errorf("路径不存在: %s", path)
        }
        absPath, err := filepath.Abs(path)
        if err != nil {
            return nil, err
        }
        target.Path = absPath
    }
    
    target.Name = name
    
    // 如果设置为默认，取消其他默认
    if isDefault && !target.IsDefault {
        for _, dir := range directories {
            dir.IsDefault = false
        }
        target.IsDefault = true
    }
    
    // 保存
    if err := s.Save(directories); err != nil {
        return nil, err
    }
    
    return target, nil
}

// Delete 删除工作目录
func (s *DirectoryService) Delete(id string) error {
    directories, err := s.Load()
    if err != nil {
        return err
    }
    
    var newDirs []*model.Directory
    found := false
    for _, dir := range directories {
        if dir.ID != id {
            newDirs = append(newDirs, dir)
        } else {
            found = true
        }
    }
    
    if !found {
        return fmt.Errorf("工作目录不存在")
    }
    
    return s.Save(newDirs)
}

// SetDefault 设置默认目录
func (s *DirectoryService) SetDefault(id string) error {
    directories, err := s.Load()
    if err != nil {
        return err
    }
    
    found := false
    for _, dir := range directories {
        if dir.ID == id {
            dir.IsDefault = true
            found = true
        } else {
            dir.IsDefault = false
        }
    }
    
    if !found {
        return fmt.Errorf("工作目录不存在")
    }
    
    return s.Save(directories)
}

// GetDefault 获取默认目录
func (s *DirectoryService) GetDefault() (*model.Directory, error) {
    directories, err := s.Load()
    if err != nil {
        return nil, err
    }
    
    for _, dir := range directories {
        if dir.IsDefault {
            return dir, nil
        }
    }
    
    // 如果没有默认目录，返回第一个
    if len(directories) > 0 {
        return directories[0], nil
    }
    
    return nil, fmt.Errorf("没有配置工作目录")
}
```

---

### Phase 4 检查点（中间）

**验证清单**：

- [ ] util/json.go实现完成
- [ ] util/git.go实现完成
- [ ] util/file.go实现完成
- [ ] service/directory.go实现完成
- [ ] 代码编译无错误

**成功标志**：
✅ `go build` 编译成功  
✅ 所有工具函数语法正确  

---

由于篇幅限制，我将继续编写剩余的Phase 4部分和Phase 5-7。让我继续...

### 4.5 创建service包 - filetree.go

**目标**：实现文件树服务。

**文件路径**：`service/filetree.go`

**完整代码**：

```go
package service

import (
    "os"
    "path/filepath"
    "strings"
    
    "workbench/model"
    "workbench/util"
)

// FileTreeService 文件树服务
type FileTreeService struct {
    gitCmd *util.GitCommand
}

// NewFileTreeService 创建文件树服务
func NewFileTreeService() *FileTreeService {
    return &FileTreeService{
        gitCmd: util.NewGitCommand(),
    }
}

// GetChildren 获取直接子节点（懒加载）
func (s *FileTreeService) GetChildren(dirPath string) ([]*model.FileTreeNode, error) {
    // 读取目录内容
    entries, err := os.ReadDir(dirPath)
    if err != nil {
        return nil, err
    }
    
    var nodes []*model.FileTreeNode
    
    for _, entry := range entries {
        name := entry.Name()
        
        // 过滤.git文件夹
        if name == ".git" {
            continue
        }
        
        // 过滤隐藏文件（可选）
        if strings.HasPrefix(name, ".") && name != ".gitignore" {
            continue
        }
        
        fullPath := filepath.Join(dirPath, name)
        
        var fileType string
        if entry.IsDir() {
            fileType = "directory"
        } else {
            fileType = "file"
        }
        
        node := model.NewFileTreeNode(name, fullPath, fileType)
        
        // 检测是否是Git仓库
        if entry.IsDir() {
            node.IsGitRepo = s.gitCmd.IsGitRepository(fullPath)
        }
        
        nodes = append(nodes, node)
    }
    
    return nodes, nil
}

// GetTree 递归获取完整子树
func (s *FileTreeService) GetTree(dirPath string, maxDepth int) ([]*model.FileTreeNode, error) {
    return s.buildTree(dirPath, 0, maxDepth)
}

// buildTree 递归构建树
func (s *FileTreeService) buildTree(dirPath string, currentDepth, maxDepth int) ([]*model.FileTreeNode, error) {
    if currentDepth >= maxDepth {
        return nil, nil
    }
    
    nodes, err := s.GetChildren(dirPath)
    if err != nil {
        return nil, err
    }
    
    for _, node := range nodes {
        if node.Type == "directory" {
            children, err := s.buildTree(node.Path, currentDepth+1, maxDepth)
            if err != nil {
                continue
            }
            node.Children = children
        }
    }
    
    return nodes, nil
}

// GetGitInfo 获取Git仓库信息
func (s *FileTreeService) GetGitInfo(dirPath string) (*model.GitRepoInfo, error) {
    info := &model.GitRepoInfo{
        Path:   dirPath,
        IsRepo: s.gitCmd.IsGitRepository(dirPath),
    }
    
    if !info.IsRepo {
        return info, nil
    }
    
    // 获取分支
    branch, err := s.gitCmd.GetBranch(dirPath)
    if err != nil {
        return info, nil
    }
    info.Branch = strings.TrimSpace(branch)
    
    // 获取远程
    remote, remoteURL, err := s.gitCmd.GetRemote(dirPath)
    if err == nil {
        info.Remote = remote
        info.RemoteURL = remoteURL
    }
    
    return info, nil
}
```

---

### 4.6 创建service包 - fileoperation.go

**目标**：实现文件操作服务。

**文件路径**：`service/fileoperation.go`

**完整代码**：

```go
package service

import (
    "os"
    "path/filepath"
    
    "workbench/model"
)

// FileOperationService 文件操作服务
type FileOperationService struct{}

// NewFileOperationService 创建文件操作服务
func NewFileOperationService() *FileOperationService {
    return &FileOperationService{}
}

// CreateDirectory 创建文件夹
func (s *FileOperationService) CreateDirectory(parentPath, name string) error {
    fullPath := filepath.Join(parentPath, name)
    
    // 检查是否已存在
    if _, err := os.Stat(fullPath); err == nil {
        return os.ErrExist
    }
    
    return util.CreateDirectory(fullPath)
}

// CreateFile 创建文件
func (s *FileOperationService) CreateFile(parentPath, name, content string) error {
    fullPath := filepath.Join(parentPath, name)
    
    // 检查是否已存在
    if _, err := os.Stat(fullPath); err == nil {
        return os.ErrExist
    }
    
    return util.CreateFile(fullPath, content)
}

// Rename 重命名文件或文件夹
func (s *FileOperationService) Rename(oldPath, newName string) error {
    // 获取父目录
    dir := filepath.Dir(oldPath)
    newPath := filepath.Join(dir, newName)
    
    // 检查新名称是否已存在
    if _, err := os.Stat(newPath); err == nil {
        return os.ErrExist
    }
    
    return util.RenamePath(oldPath, newPath)
}

// Delete 删除文件或文件夹
func (s *FileOperationService) Delete(path string) error {
    return util.RemovePath(path)
}

// PreviewFile 预览文件
func (s *FileOperationService) PreviewFile(filePath string, maxSize int64) (*model.FilePreview, error) {
    preview := &model.FilePreview{
        Path: filePath,
        Name: filepath.Base(filePath),
    }
    
    // 获取文件信息
    info, err := os.Stat(filePath)
    if err != nil {
        preview.Error = err.Error()
        return preview, err
    }
    
    preview.Size = info.Size()
    
    // 检查文件大小
    if preview.Size > maxSize {
        preview.TooLarge = true
        return preview, nil
    }
    
    // 判断是否可预览
    if !util.IsPreviewable(filePath) {
        // 尝试读取以判断是否是二进制
        data, err := util.ReadFileSafe(filePath, 1024)
        if err != nil {
            preview.Error = err.Error()
            return preview, err
        }
        
        // 简单的二进制检测
        for _, b := range data {
            if b == 0 {
                preview.IsBinary = true
                return preview, nil
            }
        }
    }
    
    // 读取文件内容
    data, err := util.ReadFileSafe(filePath, maxSize)
    if err != nil {
        preview.Error = err.Error()
        return preview, err
    }
    
    preview.Content = string(data)
    return preview, nil
}
```

---

### 4.7 创建service包 - git.go

**目标**：实现Git操作服务。

**文件路径**：`service/git.go`

**完整代码**：

```go
package service

import (
    "bufio"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"
    
    "workbench/model"
    "workbench/util"
)

// GitService Git服务
type GitService struct {
    gitCmd *util.GitCommand
}

// NewGitService 创建Git服务
func NewGitService() *GitService {
    return &GitService{
        gitCmd: util.NewGitCommand(),
    }
}

// GetInfo 获取仓库信息
func (s *GitService) GetInfo(dirPath string) (*model.GitRepoInfo, error) {
    info := &model.GitRepoInfo{
        Path:   dirPath,
        IsRepo: s.gitCmd.IsGitRepository(dirPath),
    }
    
    if !info.IsRepo {
        return info, nil
    }
    
    // 获取分支
    branch, err := s.gitCmd.GetBranch(dirPath)
    if err == nil {
        info.Branch = strings.TrimSpace(branch)
    }
    
    // 获取远程
    remote, remoteURL, err := s.gitCmd.GetRemote(dirPath)
    if err == nil {
        info.Remote = remote
        info.RemoteURL = remoteURL
    }
    
    return info, nil
}

// GetLog 获取提交历史（分页）
func (s *GitService) GetLog(dirPath string, page, pageSize int) (*model.PageResult, error) {
    if !s.gitCmd.IsGitRepository(dirPath) {
        return nil, fmt.Errorf("不是Git仓库")
    }
    
    // 获取总数
    total, err := s.gitCmd.GetTotalCommits(dirPath)
    if err != nil {
        return nil, err
    }
    
    // 计算偏移
    offset := (page - 1) * pageSize
    
    // 获取提交
    lines, err := s.gitCmd.GetLog(dirPath, offset, pageSize)
    if err != nil {
        return nil, err
    }
    
    // 解析提交
    commits := make([]model.GitCommit, 0, len(lines))
    for _, line := range lines {
        commit, err := s.parseCommit(line)
        if err != nil {
            continue
        }
        commits = append(commits, *commit)
    }
    
    return model.NewPageResult(commits, total, page, pageSize), nil
}

// parseCommit 解析提交行
func (s *GitService) parseCommit(line string) (*model.GitCommit, error) {
    // 格式：hash|author|date|message
    parts := strings.SplitN(line, "|", 4)
    if len(parts) < 4 {
        return nil, fmt.Errorf("invalid commit format")
    }
    
    date, err := time.Parse("2006-01-02T15:04:05-07:00", parts[2])
    if err != nil {
        return nil, err
    }
    
    return &model.GitCommit{
        Hash:    parts[0],
        Author:  parts[1],
        Date:    date,
        Message: parts[3],
    }, nil
}

// Clone 克隆仓库
func (s *GitService) Clone(url, targetPath string) (string, error) {
    // 检查目标路径
    if _, err := os.Stat(targetPath); err == nil {
        return "", fmt.Errorf("目标路径已存在: %s", targetPath)
    }
    
    // 执行克隆
    return s.gitCmd.Clone(url, targetPath)
}

// Pull 拉取更新
func (s *GitService) Pull(dirPath string) (string, error) {
    if !s.gitCmd.IsGitRepository(dirPath) {
        return "", fmt.Errorf("不是Git仓库")
    }
    
    return s.gitCmd.Pull(dirPath)
}

// ExtractRepoName 从URL提取仓库名称
func (s *GitService) ExtractRepoName(url string) string {
    // 移除.git后缀
    url = strings.TrimSuffix(url, ".git")
    
    // 提取最后一部分
    parts := strings.Split(url, "/")
    if len(parts) > 0 {
        return parts[len(parts)-1]
    }
    
    return "repo"
}

// CheckCloneConflict 检查克隆冲突
func (s *GitService) CheckCloneConflict(targetPath string) error {
    if _, err := os.Stat(targetPath); err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return err
    }
    
    // 路径存在，检查类型
    info, err := os.Stat(targetPath)
    if err != nil {
        return err
    }
    
    if info.IsDir() {
        if s.gitCmd.IsGitRepository(targetPath) {
            return fmt.Errorf("Git仓库已存在")
        }
        return fmt.Errorf("目录已存在")
    }
    
    return fmt.Errorf("文件已存在")
}
```

---

### Phase 4 检查点

**验证清单**：

- [ ] 所有service实现完成
- [ ] 所有util工具实现完成
- [ ] 代码编译无错误
- [ ] 基本逻辑验证

**验证方法**：
```bash
cd workbench
go build
```

**预期输出**：编译成功，无错误。

---

由于篇幅限制，我将在下一部分继续编写Phase 5、6、7的内容...

