# Git Manager - 开发运维文档

## 环境要求

### 必需工具
- **Go:** 1.21+ （推荐 1.26.2）
- **Node.js:** 16+ （推荐 18+）
- **npm:** 8+
- **Git:** 2.x
- **Wails CLI:** v2.5+
- **WebView2 Runtime:** Windows 10+ 自带

### 验证环境
```bash
go version          # 验证 Go 安装
node --version       # 验证 Node.js
npm --version        # 验证 npm
git --version        # 验证 Git
wails version        # 验证 Wails CLI
```

## 安装依赖

### 1. 安装 Go
下载地址：https://go.dev/dl/
- 选择 Windows amd64 版本
- 安装后重启终端/IDE

### 2. 安装 Node.js
下载地址：https://nodejs.org/
- 下载 LTS 版本
- 安装后验证版本

### 3. 安装 Wails CLI
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 4. 配置环境变量
确保 Go bin 目录在 PATH 中：
```bash
# 添加到系统环境变量
D:\Program Files\Go\bin
C:\Users\YourName\go\bin
```

## 项目初始化

### 首次设置
```bash
# 1. 进入项目目录
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\git-manager

# 2. 安装前端依赖
cd frontend
npm install
cd ..

# 3. 验证项目结构
dir /b /s | findstr /V "node_modules" | findstr /V ".git"
```

## 开发调试

### 启动开发服务器
```bash
# 进入项目目录
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\git-manager

# 启动开发服务器（热重载）
wails dev
```

**开发服务器特性：**
- ✅ 前端热重载（Vite HMR）
- ✅ 后端自动重新编译
- ✅ 文件变化自动监控
- ✅ 浏览器开发者工具支持

**访问地址：**
- 应用：http://localhost:34115
- 前端 Dev Server：http://localhost:5173（自动分配）

### 仅启动前端开发服务器
```bash
cd frontend
npm run dev
```

### 运行测试
```bash
# 运行所有测试
go test ./...

# 运行模型测试（带详细输出）
go test ./model -v

# 运行特定测试
go test ./model -run TestNewDirectory
```

### 调试后端代码
```bash
# 使用 Delve 调试器
dlv debug git-manager.exe

# 或在 VSCode 中设置断点调试
# F5 启动调试
```

### 调试前端代码
1. 打开浏览器访问 http://localhost:34115
2. 按 F12 打开开发者工具
3. 在 Console 查看日志
4. 在 Sources 设置断点
5. 在 Network 监控 API 请求

## 代码构建

### 构建生产版本
```bash
cd D:\workspace\workspace_ai\demo_OpenSpec\git_tools\git-manager

# 构建完整应用
wails build
```

**构建产物：**
- 位置：`build/bin/git-manager.exe`
- 大小：约 13 MB
- 包含：前端资源已嵌入，无需额外文件

### 构建选项
```bash
# 清理后重新构建
wails build -clean

# 跳过前端构建
wails build -skipfrontend

# 仅构建前端
cd frontend
npm run build
```

## 运行应用

### 开发模式
```bash
# 启动开发服务器（推荐）
wails dev
```

### 生产模式
```bash
# 直接运行编译好的可执行文件
build\bin\git-manager.exe
```

### 发布应用
```bash
# 构建生产版本
wails build

# 发布给用户
# 整个 build/bin/ 目录打包分发
# 用户直接运行 git-manager.exe 即可
```

## 故障排除

### 问题 1: 端口被占用
```bash
# 检查端口占用
netstat -ano | findstr ":34115"

# Windows: 查找进程并终止
tasklist | findstr "git-manager"
taskkill /F /IM git-manager.exe

# 或使用其他端口
wails dev -port 40000
```

### 问题 2: 前端依赖错误
```bash
# 清理 node_modules 重新安装
cd frontend
rm -rf node_modules package-lock.json
npm install
```

### 问题 3: Wails 绑定生成失败
```bash
# 清理并重新生成绑定
cd frontend
rm -rf wailsjs
cd ..
wails dev -clean
```

### 问题 4: Go 编译错误
```bash
# 清理 Go 缓存
go clean -cache -modcache

# 重新下载依赖
go mod tidy
```

### 问题 5: WebView2 运行时缺失
下载并安装 WebView2 Runtime：
https://developer.microsoft.com/en-us/microsoft-edge/webview2/

### 问题 6: 文件树不显示
1. 打开浏览器控制台（F12）
2. 检查 Console 日志
3. 验证目录路径是否正确
4. 检查是否有权限访问该目录

## 常用开发命令

### Git 操作
```bash
# 查看状态
git status

# 提交更改
git add .
git commit -m "描述信息"

# 推送到远程
git push origin master
```

### 分支管理
```bash
# 创建新分支
git checkout -b feature/xxx

# 切换分支
git checkout master

# 合并分支
git merge feature/xxx
```

### 依赖更新
```bash
# 更新 Go 依赖
go get -u ./...
go mod tidy

# 更新前端依赖
cd frontend
npm update
npm install
```

## 生产发布流程

### 1. 确保测试通过
```bash
go test ./...
```

### 2. 更新版本号
编辑 `wails.json` 中的 `productVersion`

### 3. 构建生产版本
```bash
wails build -clean
```

### 4. 验证构建
```bash
# 测试运行
build\bin\git-manager.exe

# 检查文件大小
ls -lh build/bin/git-manager.exe
```

### 5. 打包分发
```bash
# 创建发布包
mkdir release
copy build\bin\git-manager.exe release\
copy README.md release\
# 添加其他必需文件

# 压缩发布包
cd release
7z a -tzip git-manager-v1.0.0.zip *
```

### 6. 创建 Git 标签（可选）
```bash
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0
```

## 开发工作流

### 典型开发流程
1. 拉取最新代码
   ```bash
   git pull origin master
   ```

2. 创建功能分支
   ```bash
   git checkout -b feature/your-feature
   ```

3. 启动开发服务器
   ```bash
   wails dev
   ```

4. 编码（热重载自动生效）

5. 运行测试
   ```bash
   go test ./...
   ```

6. 提交代码
   ```bash
   git add .
   git commit -m "feat: add your feature"
   ```

7. 推送分支
   ```bash
   git push origin feature/your-feature
   ```

8. 合并到主分支（通过 PR 或直接合并）

9. 构建测试
   ```bash
   wails build
   ```

### Bug 修复流程
1. 创建 bugfix 分支
   ```bash
   git checkout -b bugfix/issue-description
   ```

2. 定位并修复问题

3. 添加回归测试（如果需要）

4. 验证修复
   ```bash
   wails dev
   ```

5. 提交并测试
   ```bash
   git add .
   git commit -m "fix: resolve issue description"
   ```

## 性能优化

### 减少构建时间
```bash
# 仅构建前端
cd frontend
npm run build

# 跳过前端构建
wails build -skipfrontend
```

### 清理缓存
```bash
# 清理 Go 缓存
go clean -cache -modcache -testcache

# 清理前端缓存
cd frontend
rm -rf node_modules/.vite
```

## 监控和日志

### 查看应用日志
开发模式下，日志会直接输出到终端：
- Go 后端日志：`println()` 输出
- 前端日志：浏览器控制台

### 生产环境日志
如果需要日志文件，可以配置日志系统。

## 安全注意事项

### 敏感信息
- 不要将 `data/` 目录中的配置文件提交到 Git
- `.gitignore` 已配置忽略 `data/*.json`

### 代码签名
Windows 可能提示 SmartScreen 警告，首次运行需要：
- 右键 → 属性 → 解除阻止

## 快速参考

### 常用命令速查

| 操作 | 命令 |
|------|------|
| 开发调试 | `wails dev` |
| 构建应用 | `wails build` |
| 运行测试 | `go test ./...` |
| 安装依赖 | `cd frontend && npm install` |
| 清理重装 | `cd frontend && rm -rf node_modules && npm install` |
| 查看端口占用 | `netstat -ano \| findstr ":34115"` |
| 停止进程 | `taskkill /F /IM git-manager.exe` |

### 目录结构
```
git-manager/
├── main.go              # 主入口
├── app.go               # 应用结构
├── model/               # 数据模型
├── service/             # 业务服务
├── util/                # 工具函数
├── data/                # 数据文件（不提交）
├── frontend/            # Vue3 前端
│   ├── src/
│   ├── wailsjs/        # Wails 绑定（自动生成）
│   └── package.json
├── build/               # 构建输出
│   └── bin/
│       └── git-manager.exe
└── wails.json          # Wails 配置
```

---

**文档版本：** v1.0
**最后更新：** 2026-04-29
**维护者：** 刘阳
