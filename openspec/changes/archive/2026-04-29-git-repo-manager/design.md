## Context

**项目背景**：
个人开发者本地工具，运行在Windows系统上。用户具有11年Java/SpringBoot开发经验，熟悉银行解决方案技术栈。目标是创建一个便捷的可视化工具，统一管理本地Git仓库和文件系统操作，减少在命令行和文件管理器之间频繁切换的低效操作。

**当前状态**：
- 现有开发环境已配置Git、Java开发环境
- 项目目录：`D:\workspace\workspace_ai\demo_OpenSpec\git_tools`
- 无现有代码库，需从零开始构建

**约束条件**：
- 必须是单体应用，便于部署和维护
- **必须是原生桌面应用**，不是Web应用
- 前端嵌入到后端二进制文件，不依赖独立的前端服务器
- 仅支持Windows文件系统路径格式
- 不提供文件上传功能
- 不支持深色模式等UI定制
- Git操作依赖系统预安装的Git
- **要求编译为单一可执行文件，无需运行时**
- **要求独立窗口运行，类似VS Code**

## Goals / Non-Goals

**Goals**：
- 提供直观的桌面应用界面来管理本地Git仓库和文件系统
- 支持工作目录的灵活配置和持久化
- 实现文件树的懒加载和高效浏览
- 集成常用Git操作（克隆、拉取、历史查看）
- 支持文件和文件夹的基础管理操作
- 提供文本文件预览功能
- **原生桌面应用体验，独立窗口**
- 单体应用部署，启动即用
- 编译为单一exe文件，双击即可运行

**Non-Goals**：
- 不提供文件编辑功能（只预览）
- 不支持文件上传
- 不提供Git冲突解决界面
- 不支持多主题切换
- 不提供文件搜索功能
- 不支持跨平台（仅Windows）
- 不提供Git历史的高级筛选或搜索
- **不提供Web访问模式**（仅桌面应用）

## Decisions

### 1. 技术栈选择：Wails + Go + Vue3 + Element Plus

**决策**：使用Wails v2.5+框架，后端逻辑用Go，前端UI用Vue3 + Element Plus

**理由**：
- Wails提供真正的桌面应用体验，独立窗口，不是浏览器
- 编译为单一exe文件（15-20MB），无需任何运行时
- 启动速度快（0.5-1秒），内存占用低（30-50MB）
- 使用系统WebView（Windows使用WebView2），比Electron的Chromium轻量得多
- Go后端直接暴露方法给前端，无需HTTP API和路由
- 用户有丰富编程经验，Go + Wails学习成本低（约1-2周）
- 部署极简：编译后单文件即可分发
- Vue3组合式API适合复杂交互逻辑
- Element Plus提供完整的UI组件库
- 原生系统集成：系统托盘、原生菜单、应用图标
- 更专业：类似VS Code、Postman等工具的形象

**替代方案考虑**：
- Spring Boot：JAR包50MB+，需要JVM，是Web应用不是桌面应用
- Go + Gin：Web应用，需要浏览器访问，不够"原生"
- Electron：体积大（100MB+），内存占用高（200MB+）
- Tauri：更轻量，但使用Rust后端，学习成本高于Go
- Python + PyQT/Pyside：需要Python环境，打包体积大

### 2. 桌面应用架构

**决策**：使用Wails框架构建原生桌面应用，前后端在同一进程中

**架构图**：
```
┌─────────────────────────────────────────────────────────────┐
│                   Git Manager.exe                             │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────┐ │
│  │              WebView2 (系统自带)                         │ │
│  │  ┌───────────────────────────────────────────────────┐  │ │
│  │  │        Vue3 + Element Plus 前端UI                 │  │ │
│  │  │                                                      │  │ │
│  │  │  - 文件树组件                                        │  │ │
│  │  │  - Git信息面板                                       │  │ │
│  │  │  - 操作对话框                                        │  │ │
│  │  └───────────────────────────────────────────────────┘  │ │
│  └─────────────────────────────────────────────────────────┘ │
│                          ↕ Wails Binding                      │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │                   Go 后端逻辑                            │ │
│  │  ┌───────────────────────────────────────────────────┐  │ │
│  │  │  App (主应用结构体)                                │  │ │
│  │  │    ├── GetDirectories()  ← 前端直接调用            │  │ │
│  │  │    ├── GetFiles(path)     ← 无需HTTP               │  │ │
│  │  │    ├── CloneRepo(...)    ← 同步/异步调用           │  │ │
│  │  │    └── ...                                        │  │ │
│  │  └───────────────────────────────────────────────────┘  │ │
│  │                                                          │ │
│  │  Services:                                               │ │
│  │    ├── DirectoryService                                 │ │
│  │    ├── FileTreeService                                  │ │
│  │    ├── FileOperationService                             │ │
│  │    └── GitService                                       │ │
│  └─────────────────────────────────────────────────────────┘ │
│                          ↕                                    │
│              os/exec (Git命令) + os (文件操作)                │
└─────────────────────────────────────────────────────────────┘
```

**理由**：
- Wails自动处理前后端通信，开发者无需手动实现API
- 前端直接调用Go方法，像调用本地JavaScript函数一样简单
- 单一进程，无需启动HTTP服务器
- 原生窗口，系统集成度高
- 符合"桌面应用"的产品定位
- 用户只需双击exe，无需任何依赖

**替代方案考虑**：
- Web应用方式：需要浏览器，不够专业，用户体验差
- Electron：太重太慢
- Tauri：Rust学习成本高

### 3. 数据存储：本地JSON文件

**决策**：使用`data/directories.json`存储工作目录配置

**理由**：
- 配置数据量小，JSON文件足够
- 无需数据库依赖，降低复杂度
- 易于备份和手动编辑
- 符合"零依赖"设计原则
- Go标准库encoding/json开箱即用
- 配置文件放在exe同目录下，易于查找

**替代方案考虑**：
- SQLite数据库：引入额外依赖，对于简单配置过度设计
- BoltDB：Go原生KV存储，但对于简单配置过度设计

### 4. Git命令执行：os/exec

**决策**：使用Go标准库os/exec直接调用系统Git命令

**理由**：
- 利用系统已安装的Git，无需引入Go Git库
- 支持所有Git功能，不受第三方库限制
- 减少依赖和二进制体积
- 执行结果解析灵活
- os/exec是Go标准库，稳定可靠

**替代方案考虑**：
- go-git：功能有限，维护成本高，增加二进制体积

### 5. 前后端通信：Wails Binding

**决策**：使用Wails的绑定机制，前端直接调用Go方法

**通信示例**：

```go
// app.go - Go后端
type App struct {
    ctx *context.Context
}

// Wails自动绑定这些方法到前端
func (a *App) GetDirectories() []Directory {
    return directoryService.Load()
}

func (a *App) GetFileTree(path string) []FileTreeNode {
    return fileTreeService.GetChildren(path)
}

func (a *App) CloneRepo(url, targetPath string) error {
    return gitService.Clone(url, targetPath)
}
```

```javascript
// 前端 - Wails自动生成绑定代码
import { GetDirectories, GetFileTree, CloneRepo } from './wailsjs/go/main/App'

// 像调用本地函数一样调用Go方法
const dirs = await GetDirectories()
const tree = await GetFileTree('C:\\workspace')
await CloneRepo(url, path)
```

**理由**：
- 无需手动实现RESTful API
- 无需处理HTTP请求/响应
- 类型安全（Wails生成TypeScript类型定义）
- 开发效率高
- 性能更好（进程内通信，无需网络开销）

**替代方案考虑**：
- RESTful API：需要手动实现路由、控制器、序列化等工作量大
- GraphQL：对于简单CRUD操作过度设计

### 6. 文件树懒加载策略

**决策**：初始只加载根目录的直接子节点，用户展开时按需加载子节点

**理由**：
- 避免一次性加载大量文件导致的性能问题
- 减少内存占用
- 提升响应速度
- 符合大型目录的浏览习惯

**替代方案考虑**：
- 全量加载：对于深层目录或大型项目（如node_modules）会导致卡顿

### 7. 文件预览限制

**决策**：仅预览文本文件，限制文件大小1MB，二进制文件提示无法预览

**理由**：
- 避免加载大文件导致应用卡顿
- WebView内存有限
- 提供清晰的预览边界

**替代方案考虑**：
- 尝试预览所有文件：技术复杂度高，用户体验差

### 8. 系统集成功能

**决策**：利用Wails的系统集成能力，提供原生体验

**实现功能**：
- 独立应用图标（任务栏、桌面快捷方式）
- 窗口控制（最小化、最大化、关闭）
- 可选：系统托盘图标
- 可选：开机自启动
- 原生文件对话框（打开/保存文件）

**理由**：
- 增强专业感
- 提升用户体验
- 符合桌面应用标准

**替代方案考虑**：
- 不实现系统集成：功能完整但体验不够"原生"

### 9. 打包和分发

**决策**：使用Wails CLI打包为单一exe文件

**打包流程**：
```bash
# 开发模式（前端热重载）
wails dev

# 生产构建
wails build

# 输出：build/bin/git-manager.exe (15-20MB)
```

**理由**：
- Wails自动处理前端打包和资源嵌入
- 无需手动配置embed
- 支持Windows应用图标
- 支持数字签名（可选）

**替代方案考虑**：
- 手动配置Go embed：工作量大，易出错

## Risks / Trade-offs

### Risk 1: Git命令执行失败
**风险**：系统未安装Git或Git版本过低，导致功能无法使用
**缓解措施**：
- 在README中明确说明Git前置依赖
- 启动时不强制检测Git（按需求设计）
- 执行Git命令时捕获异常，提供清晰的错误提示

### Risk 2: 文件树性能问题
**风险**：深层嵌套目录或包含大量文件的目录（如node_modules）导致加载缓慢
**缓解措施**：
- 实现懒加载，仅加载一层子节点
- 过滤.git隐藏文件夹
- 未来可考虑添加目录黑名单（如node_modules）

### Risk 3: 路径安全问题
**风险**：文件操作可能访问系统敏感目录，导致安全风险
**缓解措施**：
- 仅在用户配置的工作目录范围内操作
- 路径拼接时使用filepath.Join防止路径遍历攻击
- 使用filepath.Clean清理路径
- 未来可考虑添加目录白名单机制

### Risk 4: 并发文件操作冲突
**风险**：用户在工具中操作文件时，外部工具同时修改文件，导致状态不一致
**缓解措施**：
- 每次文件操作前刷新文件树状态
- 关键操作失败时提供明确错误信息
- 建议用户避免在工具使用时同时使用其他文件管理器

### Risk 5: 克隆冲突处理
**风险**：克隆目标目录已存在，可能导致数据覆盖或操作失败
**缓解措施**：
- 克隆前严格检查目录是否存在
- 区分Git仓库和普通目录，提供针对性错误提示
- 冲突时中止操作，不进行任何覆盖

### Risk 6: Go和Wails学习曲线
**风险**：用户从Java转向Go和Wails，需要适应新的语法和框架
**缓解措施**：
- 用户有11年编程经验，Go学习成本低（约1周）
- Wails概念简单，主要是学习绑定机制（1-2天）
- Go语法简洁，比Java简单
- 提供详细的代码注释和文档
- 参考Wails官方示例项目

### Risk 7: WebView2依赖
**风险**：Wails on Windows依赖WebView2运行时
**缓解措施**：
- Windows 10/11默认已安装WebView2
- Wails启动时自动检测，如未安装会提示用户
- 提供安装指引链接（Microsoft官方下载）
- WebView2体积小（约100MB），由微软维护

### Trade-off 1: 功能完整性 vs 开发效率
**权衡**：不支持文件编辑、上传等功能，以加快开发速度
**理由**：个人工具，核心功能满足即可，避免过度工程化

### Trade-off 2: 跨平台支持 vs 开发成本
**权衡**：仅支持Windows，不处理跨平台路径差异
**理由**：明确个人工具定位，减少兼容性测试成本

### Trade-off 3: 桌面应用 vs Web应用
**权衡**：选择Wails桌面应用而非Go+Gin Web应用
**理由**：
- 原生体验更好（独立窗口、系统集成）
- 更专业（类似VS Code而非网页）
- 稍多学习成本（1-2周），但用户体验提升显著

### Trade-off 4: 体积和性能
**权衡**：Wails（15-20MB）比Go+Gin（8MB）稍大，但换来桌面体验
**理由**：15-20MB仍然非常轻量，远小于Electron（100MB+）

## Migration Plan

### 开发阶段

**Phase 0: 环境准备和学习**（1-2天）
1. 安装Go开发环境（go1.21+）
2. 安装Node.js（用于前端开发）
3. 安装Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
4. 学习Go基础语法（1天）
5. 学习Wails框架基础（0.5天）
6. 阅读Wails官方示例项目

**Phase 1: 基础框架搭建**（1天）
1. 初始化Wails项目：`wails init -n git-manager -t vue3`
2. 理解Wails项目结构
3. 创建App主应用结构体（app.go）
4. 配置应用基本信息（窗口标题、大小、图标）
5. 测试Wails开发模式：`wails dev`
6. 确认前后端绑定工作正常
7. 安装Element Plus：`npm install element-plus`
8. 配置Vue3路由和状态管理

**Phase 2: 工作目录管理**（1天）
1. 创建Directory结构体
2. 创建util包的JSON配置读写（encoding/json）
3. 创建directories.json配置文件，添加默认工作目录示例
4. 创建service包的DirectoryService
5. 实现加载、保存、增删改查方法
6. 实现路径验证
7. 在App结构体中添加方法：GetDirectories(), AddDirectory(), UpdateDirectory(), DeleteDirectory()
8. Wails自动生成前端绑定代码
9. 前端实现目录选择器组件DirectorySelector.vue
10. 前端实现添加/编辑目录弹窗DirectoryDialog.vue
11. 测试前后端通信和JSON持久化

**Phase 3: 文件树和基础操作**（2天）
1. 创建FileTreeNode结构体
2. 创建service包的FileTreeService
3. 实现文件系统遍历（os/ioutil, filepath）
4. 实现Git仓库检测（os.Stat）
5. 实现获取子节点方法GetChildren()
6. 实现递归获取完整子树方法GetTree()
7. 在App中添加方法：GetFileTree(), ExpandAll(), CollapseAll()
8. 创建service包的FileOperationService
9. 实现创建、删除、重命名、预览方法
10. 在App中添加方法：CreateDirectory(), CreateFile(), Rename(), Delete(), PreviewFile()
11. 前端实现FileTree.vue组件
12. 实现文件树懒加载逻辑
13. 实现节点展开/收起交互
14. 实现全部展开/收起功能
15. 前端实现右键菜单组件
16. 前端实现文件预览组件FilePreview.vue
17. 测试所有文件操作功能

**Phase 4: Git集成**（2天）
1. 创建util包的GitCommandUtil
2. 实现执行Git命令方法ExecuteCommand()
3. 使用os/exec包执行命令
4. 实现解析Git命令输出方法
5. 创建GitCommit、GitRepoInfo、PageResult结构体
6. 创建service包的GitService
7. 实现获取仓库信息、日志、克隆、拉取方法
8. 实现冲突检测
9. 在App中添加方法：GetGitInfo(), GetGitLog(), CloneRepo(), PullRepo()
10. 前端实现Git信息面板组件GitPanel.vue
11. 前端实现克隆弹窗组件CloneDialog.vue
12. 前端实现提交历史组件CommitHistory.vue
13. 实现分页控件
14. 测试所有Git操作功能

**Phase 5: 系统集成和完善**（1天）
1. 设计并添加应用图标（.ico和.png格式）
2. 配置wails.json中的窗口选项
3. 实现操作确认对话框（删除等危险操作）
4. 完善错误处理和提示
5. 添加文件大小显示
6. 实现文件类型图标映射
7. 优化UI交互体验（loading状态、按钮禁用等）
8. 添加操作日志提示（Toast消息）
9. 集成测试

**Phase 6: 打包部署**（0.5天）
1. 使用Wails CLI打包：`wails build`
2. 测试编译后的exe文件
3. 验证应用包含前端资源
4. 测试在无开发环境的机器上运行
5. 编写README.md使用文档
6. 创建安装说明（前置依赖：Git、WebView2）
7. （可选）配置数字签名

### 部署步骤

**开发者打包**：
```bash
# 1. 确保依赖已安装
# - Go 1.21+
# - Node.js 16+
# - Wails CLI
# - Git (用于wails build)

# 2. 生产构建
wails build

# 3. 输出文件
# build/bin/git-manager.exe (15-20MB)

# 4. 分发
# 将exe文件发给用户，双击即可运行
```

**用户使用**：
1. 确保已安装Git
2. 双击 `git-manager.exe`
3. 应用自动打开独立窗口

**一键构建脚本**（build.bat）：
```batch
@echo off
echo Building Git Manager...
wails build
echo Build complete: build/bin/git-manager.exe
pause
```

### 回滚策略

- 保留前一版本的exe文件备份（git-manager-v1.0.0.exe）
- 如遇严重问题，恢复旧版本exe并重启
- JSON配置文件向后兼容，不影响数据
- Wails支持版本回退，使用旧版本源码重新编译

## Open Questions

1. **Git认证处理**：用户表示SSH密钥自行处理，工具是否需要支持HTTPS Token认证？
   - **决定**：暂不支持，依赖系统Git配置（SSH密钥或凭据助手）

2. **文件预览格式支持**：除了常见的文本格式，是否需要支持Markdown语法高亮？
   - **决定**：初始版本仅纯文本展示，高亮功能作为未来增强

3. **大文件处理阈值**：1MB的预览限制是否合理？
   - **决定**：初始版本使用1MB，根据实际使用反馈调整

4. **日志记录**：是否需要记录用户操作日志（审计追踪）？
   - **决定**：按需求设计，不记录操作日志

5. **多语言支持**：是否需要英文界面？
   - **决定**：初始版本仅中文，个人工具无需国际化

6. **并发限制**：是否限制同时执行的Git操作数量？
   - **决定**：初始版本不限制，用户手动控制操作频率

7. **系统托盘**：是否需要系统托盘图标和最小化到托盘功能？
   - **决定**：初始版本不实现，作为未来增强功能

8. **开机自启**：是否需要开机自动启动功能？
   - **决定**：初始版本不实现，作为未来增强功能

9. **WebView2检测**：启动时如何处理WebView2未安装的情况？
   - **决定**：Wails会自动检测，如未安装显示错误提示和下载链接

10. **窗口状态记忆**：是否记住窗口大小和位置？
    - **决定**：初始版本不实现，可使用默认窗口尺寸
