# Journal - liuyang (Part 1)

> AI development session journal
> Started: 2026-06-09

---



## Session 1: GitHub Actions 自动打包发版流水线

**Date**: 2026-06-10
**Task**: GitHub Actions 自动打包发版流水线
**Branch**: `master`

### Summary

创建 GitHub Actions Release 流水线（.github/workflows/release.yml），tag v* 触发自动构建 Windows exe 并发布到 GitHub Release。补充 README.md 发版流程说明。GitHub 作为 Gitee 镜像仓库跑 CI/CD。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `26ecf7e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: 实现检查更新与自动更新功能

**Date**: 2026-06-10
**Task**: 实现检查更新与自动更新功能
**Branch**: `master`

### Summary

通过 GitHub Releases API 实现检查更新、下载新版本（进度推送）、批处理替换重启、启动时待更新检测。新增 model/update.go、service/update.go、UpdateDialog.vue，修改 SettingsPanel.vue/Home.vue/app.go 及 Wails 绑定文件。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `88a6efe` | (see git log) |
| `fc98c91` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: 修复文件树拷贝到后展开节点被收起

**Date**: 2026-06-10
**Task**: 修复文件树拷贝到后展开节点被收起
**Branch**: `master`

### Summary

refreshNode 在路径不在 nodesMap 时回退到 refreshCounter++ 触发 <el-tree> key 变更整树重建，导致已展开节点全部丢失。改为命中即刷新 / 未命中沿父路径回溯到首个已展开祖先并刷新 / 都没有则静默放弃。refreshAll 整树语义保持不变。新增三条单测覆盖三种分支。spec 中 cross-layer-thinking-guide.md 增加 UI Local Refresh vs Whole-Tree Rebuild 章节沉淀该反模式。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `185ef0b` | (see git log) |
| `49de4e4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: 新增用 Obsidian 打开入口与文件预览渲染增强

**Date**: 2026-06-23
**Task**: 新增用 Obsidian 打开入口与文件预览渲染增强
**Branch**: `master`

### Summary

为工作目录树/文件树右键菜单及内容面板查看操作新增'用 Obsidian 打开'入口（文件夹→自身、文件→父目录作为 vault）；设置面板通用→外部应用支持自定义 Obsidian 程序路径（配置优先，否则走 obsidian:// URI + 注册表预检 + cmd /c start，未检测到时引导配置）；内置 Obsidian 图标；后端含 Windows 注册表预检（build tag 隔离）与单测。同会话交织提交了文件预览增强（FileBytes/Kind 分流 + FilePreviewRenderer + file-viewer3，支持图片/PDF/Office 内嵌预览）。研究归档于 research/obsidian-launch.md（基于官方文档核实 URI 协议）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `095a6ab` | (see git log) |
| `bead5c7` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
