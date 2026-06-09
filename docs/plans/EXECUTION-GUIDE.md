# WorkBench 执行指南

## 快速开始

### 1. 打开新会话

在项目根目录 `D:\workspace\workspace_ai\demo_OpenSpec\git_tools` 下打开新的 Claude Code 会话。

### 2. 使用执行技能

在新会话中输入：

```
@superpowers:executing-plans docs/plans/2025-04-28-workbench-implementation.md
```

### 3. 执行技能将自动

- ✅ 读取实现计划文档
- ✅ 按阶段执行任务（0-7）
- ✅ 在每个检查点暂停等待确认
- ✅ 运行测试验证功能
- ✅ 创建 Git 提交
- ✅ 报告进度和遇到的问题

## 实现计划结构

实现计划包含 **8 个阶段，50+ 任务**：

### Phase 0: 环境准备和学习（1-2天）
- 安装 Go 1.21+、Node.js、Wails CLI
- 学习 Go 基础语法和 Wails 框架

### Phase 1: 项目初始化（0.5天）
- 创建 Wails 项目
- 配置目录结构
- 设置 .gitignore

### Phase 2: 环境配置（0.5天）
- 安装 Element Plus
- 配置前端基础布局

### Phase 3: 模型编写（0.5天）
- 实现 7 个数据结构
- 编写单元测试

### Phase 4: 中间件开发（1天）
- 实现工具类（util 包）
- 实现服务层（service 包）

### Phase 5: 接口实现（1天）
- 实现 Wails 绑定方法
- 生成前端绑定代码

### Phase 6: 入口配置（0.5天）
- 配置 main.go
- 创建前端路由和页面

### Phase 7: 接口测试（0.5天）
- 运行所有测试
- 验证功能完整性

## 检查点

执行技能会在以下关键节点暂停：

1. **Phase 0 完成后** - 确认环境准备就绪
2. **Phase 3 完成后** - 确认模型和测试通过
3. **Phase 5 完成后** - 确认绑定方法正常
4. **Phase 7 完成后** - 确认所有测试通过
5. **构建成功后** - 确认 exe 文件生成

## 预期输出

完成后将生成：

```
workbench/
├── build/bin/workbench.exe  (15-20MB 可执行文件)
├── data/directories.json       (工作目录配置)
├── main.go                     (应用入口)
├── app.go                      (Wails 绑定)
├── model/                      (数据模型)
├── service/                    (业务逻辑)
├── util/                       (工具类)
└── frontend/                   (Vue3 前端)
```

## 验证清单

完成后检查：

- [ ] `wails dev` 能正常启动开发模式
- [ ] `wails build` 生成 exe 文件
- [ ] 双击 exe 能正常启动应用
- [ ] 工作目录管理功能正常
- [ ] 文件树浏览功能正常
- [ ] Git 克隆功能正常
- [ ] 所有单元测试通过

## 故障排查

### 常见问题

1. **Wails CLI 未安装**
   ```bash
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```

2. **Go 版本过低**
   - 需要 Go 1.21 或更高版本
   - 运行 `go version` 检查

3. **Node.js 未安装**
   - 下载：https://nodejs.org/
   - 需要 Node.js 16+ 版本

4. **WebView2 缺失**
   - Windows 10/11 通常已预装
   - 下载：https://developer.microsoft.com/microsoft-edge/webview2/

## 预计时间

- Phase 0: 1-2 天（学习和环境）
- Phase 1-7: 4-5 天（开发实现）
- **总计：5-7 天**

## 下一步

现在就可以开始执行了！在新会话中使用：

```
@superpowers:executing-plans docs/plans/2025-04-28-workbench-implementation.md
```

祝开发顺利！🚀
