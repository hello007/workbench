# 实现检查更新与自动更新功能

## Goal

为 WorkBench 桌面应用添加检查更新和自动更新能力，通过 GitHub Releases API 检测新版本，引导用户下载并替换当前可执行文件，实现从手动检查到一键更新的完整闭环。

## What I already know

### 项目技术栈
* **桌面框架**: Wails v2.12.0（Go 后端 + Vue3 前端）
* **版本管理**: `main.go` 中 `version` 和 `buildTime` 变量，通过 `-ldflags` 在构建时注入
* **当前版本**: wails.json `productVersion: 1.0.8`
* **已有方法**: `App.GetAppVersion()` 返回当前版本号
* **前端 UI**: Element Plus 组件库，设置面板（SettingsPanel.vue）含通用/终端/搜索/快捷键四个 tab
* **设置模型**: `model/settings.go` — `AppSettings` 结构体

### GitHub Release 信息
* **仓库**: `hello007/workbench`
* **Release API**: `https://api.github.com/repos/hello007/workbench/releases/latest`
* **资产命名**: `workbench.exe`（单个 Windows 可执行文件，约 16MB）
* **下载 URL**: `https://github.com/hello007/workbench/releases/download/v{version}/workbench.exe`
* **CI 流水线**: `.github/workflows/release.yml`，tag 触发构建上传

### 版本比较逻辑
* Release API 返回 `tag_name: "v1.0.8"`，需去掉 `v` 前缀后与本地 `version` 做语义化版本比较

## Assumptions (temporary)

* 仅支持 Windows 平台（当前 Release 只有 `workbench.exe`）
* 更新方式为"下载新 exe → 替换旧 exe → 重启"，无需增量更新
* 用户需要手动触发"检查更新"，不会在后台静默自动下载
* 下载过程中显示进度条

## Decisions

* **更新入口 UI** → 设置面板"通用"tab 底部，显示当前版本 + "检查更新"按钮
* **更新执行策略** → 下载完成 → 弹窗提示"更新已就绪，是否立即重启？" → 用户确认后替换并重启
* **跳过版本** → 不需要，MVP 阶段简化，用户不想更新直接关掉弹窗即可
* **用户取消重启后** → 保留已下载 exe 在临时目录，下次启动时检测到待更新文件，自动替换后启动新版本

## Open Questions

（全部已解决）

## Requirements (evolving)

* 后端：调用 GitHub Releases API (`/repos/hello007/workbench/releases/latest`) 获取最新版本信息
* 后端：语义化版本比较（`tag_name` 去掉 `v` 前缀 vs 本地 `version`）
* 后端：下载新版本 exe 到临时目录，通过 Wails Events 推送下载进度
* 后端：替换当前运行中的 exe（批处理脚本 + 延迟替换），并重启应用
* 前端：设置面板"通用"tab 底部，展示当前版本号 + "检查更新"按钮
* 前端：检查中 loading 状态，已是最新版本时提示"当前已是最新版本"
* 前端：发现新版本时弹窗展示版本号、更新日志摘要，"立即更新"按钮
* 前端：下载过程弹窗显示进度条，支持取消下载
* 前端：下载完成后弹窗提示"更新已就绪，是否立即重启？"，确认后执行替换重启

## Acceptance Criteria (evolving)

* [ ] 设置面板通用 tab 底部显示当前版本号和"检查更新"按钮
* [ ] 点击"检查更新"调用 GitHub API 获取最新版本
* [ ] 已是最新版本时提示"当前已是最新版本 v1.0.x"
* [ ] 发现新版本时弹窗展示版本号和更新日志
* [ ] 点击"立即更新"开始下载，显示进度条
* [ ] 下载过程中可取消
* [ ] 下载完成后弹窗确认"是否立即重启？"
* [ ] 确认重启后替换 exe 并启动新版本
* [ ] 用户取消重启时保留已下载 exe，下次启动自动替换
* [ ] 网络异常/下载失败时给出友好提示
* [ ] 版本比较支持语义化版本（1.0.8 < 1.0.9，1.0.8 < 1.1.0）

## Definition of Done

* Tests added/updated（后端版本比较、API 解析的单元测试）
* Lint / typecheck / CI green
* Docs/notes updated if behavior changes
* Rollout/rollback considered（用户可取消下载、下载失败不影响当前版本）

## Out of Scope (explicit)

* 静默后台自动更新（不需要在后台悄悄下载）
* 多平台支持（仅 Windows）
* 增量/差分更新
* 强制更新/最低版本限制

## Technical Notes

### 涉及文件
* `app.go` — 添加更新相关 Wails 绑定方法
* `service/update.go`（新建）— 更新服务：版本检查、下载、替换重启
* `model/update.go`（新建）— 更新相关数据模型
* `frontend/src/components/SettingsPanel.vue` — 设置面板通用 tab 底部添加版本信息和检查更新
* `frontend/src/components/UpdateDialog.vue`（新建）— 更新弹窗（版本信息 + 下载进度 + 确认重启）

### 关键约束
* Windows 下运行中的 exe 无法直接被覆盖，需要使用批处理脚本或 `MoveFileEx` 延迟替换
* GitHub API 未认证时限制 60 次/小时，对于桌面应用检查更新频率足够
* Wails v2 的 `runtime.EventsEmit` 可用于向前端推送下载进度

### 更新替换流程（Windows）
1. 下载新 exe 到 `%TEMP%/workbench-update/workbench.exe`
2. **用户确认重启时**：生成批处理脚本 → 等待当前进程退出 → `move /Y` 新 exe 到原路径 → 启动新 exe → 删除批处理脚本自身
3. **用户取消重启时**：保留临时目录中的 exe，应用下次启动时检测到待更新文件 → 自动执行替换后启动新版本

### 参考资源
* GitHub Releases API: `GET /repos/{owner}/{repo}/releases/latest`
* Wails Events: `runtime.EventsEmit(ctx, "event-name", data)`

## Implementation Plan

* **Step 1**: 后端 — 创建 `model/update.go`（数据模型）+ `service/update.go`（版本检查、下载、替换）
* **Step 2**: 后端 — `app.go` 添加 Wails 绑定方法 + 单元测试
* **Step 3**: 前端 — `SettingsPanel.vue` 通用 tab 底部添加版本信息和检查更新按钮
* **Step 4**: 前端 — `UpdateDialog.vue` 更新弹窗（新版本信息、下载进度、确认重启）
* **Step 5**: 集成测试 — 端到端验证检查更新流程
