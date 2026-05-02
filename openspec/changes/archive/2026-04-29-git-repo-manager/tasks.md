## 0. 环境准备和学习

- [ ] 0.1 安装Go开发环境（go1.21+）
- [ ] 0.2 配置Go环境变量（GOPATH、PATH）
- [ ] 0.3 安装Node.js（用于前端开发）
- [ ] 0.4 安装Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- [ ] 0.5 学习Go基础语法（变量、函数、结构体、方法、接口）
- [ ] 0.6 学习Go错误处理模式
- [ ] 0.7 学习Go标准库（os、io、json、exec、filepath）
- [ ] 0.8 学习Wails框架基础概念
- [ ] 0.9 理解Wails绑定机制
- [ ] 0.10 阅读Wails官方示例项目（至少2个）
- [ ] 0.11 学习Vue3组合式API
- [ ] 0.12 学习Element Plus组件库

## 1. 基础框架搭建

- [ ] 1.1 初始化Wails项目：`wails init -n git-manager -t vue3`
- [ ] 1.2 理解Wails项目结构（main.go, app.go, frontend/, wails.json）
- [ ] 1.3 配置应用基本信息（wails.json）
- [ ] 1.4 创建App主应用结构体（app.go）
- [ ] 1.5 实现App结构体的Startup和Shutdown方法
- [ ] 1.6 配置窗口标题、大小、图标（wails.json）
- [ ] 1.7 安装Element Plus：`npm install element-plus`
- [ ] 1.8 在main.js中引入Element Plus
- [ ] 1.9 删除Wails默认示例代码
- [ ] 1.10 创建Vue3路由配置
- [ ] 1.11 创建基础页面布局（主界面、文件树、操作面板）
- [ ] 1.12 测试Wails开发模式：`wails dev`
- [ ] 1.13 确认前端页面正常显示
- [ ] 1.14 测试一个简单的Go方法绑定

## 2. 工作目录管理

- [ ] 2.1 创建Directory结构体（id、name、path、isDefault、createTime）
- [ ] 2.2 创建model包的models.go文件
- [ ] 2.3 创建util包，实现JSON配置读写（encoding/json）
- [ ] 2.4 创建directories.json配置文件，添加默认工作目录示例
- [ ] 2.5 创建service包的DirectoryService
- [ ] 2.6 实现加载配置方法LoadDirectories()
- [ ] 2.7 实现保存配置方法SaveDirectories()
- [ ] 2.8 实现创建目录方法CreateDirectory()
- [ ] 2.9 实现更新目录方法UpdateDirectory()
- [ ] 2.10 实现删除目录方法DeleteDirectory()
- [ ] 2.11 实现设置默认目录方法SetDefaultDirectory()
- [ ] 2.12 实现路径验证方法ValidatePath()
- [ ] 2.13 在App结构体中添加GetDirectories()方法
- [ ] 2.14 在App结构体中添加AddDirectory()方法
- [ ] 2.15 在App结构体中添加UpdateDirectory()方法
- [ ] 2.16 在App结构体中添加DeleteDirectory()方法
- [ ] 2.17 在App结构体中添加SetDefaultDirectory()方法
- [ ] 2.18 运行wails dev生成前端绑定代码
- [ ] 2.19 前端实现目录选择器组件DirectorySelector.vue
- [ ] 2.20 前端实现添加/编辑目录弹窗DirectoryDialog.vue
- [ ] 2.21 前端实现目录列表展示和删除功能
- [ ] 2.22 测试工作目录的增删改查和JSON持久化

## 3. 文件树和基础操作

- [ ] 3.1 创建FileTreeNode结构体
- [ ] 3.2 创建service包的FileTreeService
- [ ] 3.3 实现文件系统遍历（os/ioutil, filepath）
- [ ] 3.4 实现获取直接子节点方法GetChildren()
- [ ] 3.5 实现递归获取完整子树方法GetTree()
- [ ] 3.6 实现Git仓库检测方法IsGitRepository()
- [ ] 3.7 实现过滤.git文件夹逻辑
- [ ] 3.8 在App结构体中添加GetFileTree()方法
- [ ] 3.9 在App结构体中添加GetFileTreeRecursive()方法
- [ ] 3.10 在App结构体中添加GetGitInfo()方法
- [ ] 3.11 创建service包的FileOperationService
- [ ] 3.12 实现创建文件夹方法CreateDirectory()
- [ ] 3.13 实现创建文件方法CreateFile()
- [ ] 3.14 实现重命名方法Rename()
- [ ] 3.15 实现删除方法Delete()
- [ ] 3.16 实现文件预览方法PreviewFile()
- [ ] 3.17 实现文件类型判断方法IsPreviewable()
- [ ] 3.18 使用os.Mkdir创建文件夹
- [ ] 3.19 使用os.Create创建文件
- [ ] 3.20 使用os.Rename重命名
- [ ] 3.21 使用os.RemoveAll删除
- [ ] 3.22 使用ioutil.ReadFile读取文件内容
- [ ] 3.23 在App结构体中添加CreateDirectory()方法
- [ ] 3.24 在App结构体中添加CreateFile()方法
- [ ] 3.25 在App结构体中添加Rename()方法
- [ ] 3.26 在App结构体中添加Delete()方法
- [ ] 3.27 在App结构体中添加PreviewFile()方法
- [ ] 3.28 运行wails dev更新绑定代码
- [ ] 3.29 前端实现FileTree.vue组件
- [ ] 3.30 实现文件树懒加载逻辑
- [ ] 3.31 实现节点展开/收起交互
- [ ] 3.32 实现节点选择高亮
- [ ] 3.33 实现Git仓库图标标识
- [ ] 3.34 实现全部展开/收起按钮功能
- [ ] 3.35 前端实现右键菜单组件
- [ ] 3.36 前端实现文件预览组件FilePreview.vue
- [ ] 3.37 前端实现创建/重命名/删除对话框
- [ ] 3.38 实现文件操作后的文件树自动刷新
- [ ] 3.39 测试所有文件操作功能

## 4. Git集成

- [ ] 4.1 创建util包的GitCommandUtil
- [ ] 4.2 实现执行Git命令方法ExecuteCommand()
- [ ] 4.3 使用os/exec包执行命令
- [ ] 4.4 实现解析Git命令输出方法ParseOutput()
- [ ] 4.5 实现错误处理和异常捕获
- [ ] 4.6 使用context实现命令超时控制
- [ ] 4.7 创建GitCommit结构体
- [ ] 4.8 创建GitRepoInfo结构体
- [ ] 4.9 创建PageResult分页结果结构体
- [ ] 4.10 创建service包的GitService
- [ ] 4.11 实现获取仓库信息方法GetRepoInfo()
- [ ] 4.12 实现获取提交历史方法GetLog()
- [ ] 4.13 实现克隆仓库方法CloneRepo()
- [ ] 4.14 实现拉取更新方法PullRepo()
- [ ] 4.15 实现提取仓库名称方法ExtractRepoName()
- [ ] 4.16 实现克隆冲突检测方法CheckCloneConflict()
- [ ] 4.17 实现git命令：git rev-parse --git-dir（检测仓库）
- [ ] 4.18 实现git命令：git branch --show-current（获取分支）
- [ ] 4.19 实现git命令：git remote -v（获取远程）
- [ ] 4.20 实现git命令：git log --pretty=format（获取历史）
- [ ] 4.21 在App结构体中添加GetGitInfo()方法
- [ ] 4.22 在App结构体中添加GetGitLog()方法（支持分页）
- [ ] 4.23 在App结构体中添加CloneRepo()方法
- [ ] 4.24 在App结构体中添加PullRepo()方法
- [ ] 4.25 运行wails dev更新绑定代码
- [ ] 4.26 前端实现Git信息面板组件GitPanel.vue
- [ ] 4.27 实现仓库信息展示（分支、远程、URL）
- [ ] 4.28 实现克隆弹窗组件CloneDialog.vue
- [ ] 4.29 实现克隆URL输入和验证
- [ ] 4.30 实现克隆冲突检测和错误提示
- [ ] 4.31 实现克隆成功后文件树刷新
- [ ] 4.32 实现拉取按钮和进度提示
- [ ] 4.33 实现提交历史组件CommitHistory.vue
- [ ] 4.34 实现提交列表展示
- [ ] 4.35 实现分页控件（上一页、下一页、页码显示）
- [ ] 4.36 实现Git操作禁用状态（并发控制）
- [ ] 4.37 测试所有Git操作功能

## 5. 系统集成和完善

- [ ] 5.1 设计应用图标（使用在线工具或设计软件）
- [ ] 5.2 准备图标文件（.ico和.png格式）
- [ ] 5.3 配置wails.json中的图标路径
- [ ] 5.4 配置窗口选项（宽度、高度、是否可调整大小）
- [ ] 5.5 配置窗口标题和样式
- [ ] 5.6 实现操作确认对话框（删除等危险操作）
- [ ] 5.7 完善错误提示信息（所有操作的错误处理）
- [ ] 5.8 添加Element Plus的ElMessage组件用于提示
- [ ] 5.9 添加文件大小显示和格式化
- [ ] 5.10 实现文件类型图标映射（根据扩展名）
- [ ] 5.11 优化文件树加载性能（大目录处理）
- [ ] 5.12 实现响应式布局调整
- [ ] 5.13 优化UI交互体验（loading状态、按钮禁用等）
- [ ] 5.14 添加操作日志提示（Toast消息）
- [ ] 5.15 优化Git命令执行超时处理
- [ ] 5.16 添加goroutine池控制并发（可选）
- [ ] 5.17 编写README.md使用文档
- [ ] 5.18 进行集成测试和Bug修复

## 6. 打包部署

- [ ] 6.1 清理开发缓存
- [ ] 6.2 运行wails build生产构建
- [ ] 6.3 等待编译完成（生成build/bin/git-manager.exe）
- [ ] 6.4 检查生成的exe文件大小（应该在15-20MB）
- [ ] 6.5 测试exe文件启动（双击运行）
- [ ] 6.6 验证应用包含前端资源
- [ ] 6.7 测试在无开发环境的机器上运行
- [ ] 6.8 验证WebView2依赖检查
- [ ] 6.9 创建build.bat一键构建脚本
- [ ] 6.10 编写用户安装说明（前置依赖）
- [ ] 6.11 编写部署文档和注意事项
- [ ] 6.12 创建Git仓库并提交代码
- [ ] 6.13 打包发布版本（可选）

## 7. 验收测试

- [ ] 7.1 测试工作目录管理完整流程
- [ ] 7.2 测试文件树浏览和懒加载
- [ ] 7.3 测试文件创建、删除、重命名
- [ ] 7.4 测试文件预览功能
- [ ] 7.5 测试Git克隆功能（HTTPS和SSH）
- [ ] 7.6 测试Git拉取功能
- [ ] 7.7 测试Git提交历史和分页
- [ ] 7.8 测试错误处理和边界情况
- [ ] 7.9 测试并发操作控制
- [ ] 7.10 测试编译后的exe文件启动
- [ ] 7.11 测试exe文件在不同Windows版本运行
- [ ] 7.12 性能测试（启动时间、内存占用、响应速度）
- [ ] 7.13 用户体验测试（UAT）
- [ ] 7.14 文档完整性检查

## 8. 可选增强功能（未来版本）

- [ ] 8.1 实现系统托盘图标和菜单
- [ ] 8.2 实现最小化到托盘功能
- [ ] 8.3 实现开机自启动选项
- [ ] 8.4 实现窗口状态记忆（大小、位置）
- [ ] 8.5 实现文件预览的语法高亮
- [ ] 8.6 添加深色模式支持
- [ ] 8.7 实现快捷键支持
- [ ] 8.8 实现文件搜索功能
- [ ] 8.9 添加应用更新检查功能
- [ ] 8.10 实现导出配置功能
