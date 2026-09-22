# Linux 平台支持改造

## Goal

当前 WorkBench 仅在 Windows 构建验证。目标：使项目可在 Linux 编译、运行（桌面 + 浏览器访问模式），补齐平台差异代码、CI 与发布产物，形成可分发的 Linux 版本。

## What I already know

调研结论（2026-09-22 会话）：

### 已就绪（无需改）
|项|说明|
|---|---|
|`util/pty_other.go`|creack/pty 完整 Linux PTY 实现，非占位|
|`util/kill_other.go`|进程组 SIGKILL，已实现|
|`util/exec_other.go`|HideCommandWindow 空实现，正确|
|`console_other.go`|已处理|
|依赖隔离|conpty 仅被 pty_windows.go（带 build tag）引用；creack/pty 仅被 pty_other.go 引用；Linux 编译无障碍|
|浏览器访问通道|Go echo + WS 跨平台，serve 模式天然支持|

### 需改造
|模块|位置|现状|改造|估算|
|---|---|---|---|---|
|Shell 配置|`model/terminal.go:43-66`|硬编码 powershell/cmd/gitbash/wsl，无 build tag|按 GOOS 分支：Linux 返回 bash/zsh/fish，默认 shell 按平台|0.5~1 天|
|文件打开|`service/fileoperation.go:242,271,305`|explorer、cmd /c start|OpenInExplorer/OpenWithDefaultApp/Obsidian URI 走 xdg-open，仿 obsidian `_other` 平台抽象|1~1.5 天|
|自更新|`service/update.go:234-281`|生成 .bat + taskkill|补 Linux .sh 生成（等价逻辑）|1 天|
|前端 shell 枚举|`useTerminal.js:110,147`|默认值硬编码 'powershell'|默认 shell 从后端设置读取，下拉数据源对齐|0.5 天|
|Linux 构建环境|构建侧|仅 Windows|webkit2gtk-4.1/GTK3 依赖，wails build 跑通，glibc 兼容|1~2 天|
|CI|`.github/workflows/ci.yml`|仅 Windows|加 ubuntu job|0.5~1 天|
|发布产物|`release.yml`|仅 Windows|tar.gz 起步，deb/AppImage 可选|1 天|
|测试适配|各 `*_test.go`|大量 D:/workspace、C:\ 路径断言|Linux CI 下修复或跳过策略|0.5~2 天|
|联调验证|整体|—|GTK 窗口、终端 PTY、事件通道实机跑|1~2 天|

### 可延后（降级已可用）
|项|说明|
|---|---|
|剪贴板|clipboard_other.go 已优雅降级；后续 xclip/wl-copy，0.5~1 天|
|Obsidian 深度集成|Linux 已降级 false；xdg-open obsidian:// 后续低成本补全|
|WSL shell 类型|Windows 专属，不涉及|

### 风险
|风险|缓解|
|---|---|
|webkit2gtk 版本碎片化|Wails v2.16 要 4.1；Ubuntu 22.04+/Debian 12+ 有包；锁定推荐系统要求|
|glibc 向后兼容|构建基线用 Ubuntu 22.04 或提供 AppImage|
|测试路径断言修复量未知|先 Linux CI 跑 go test 摸清失败面|
|Windows 回归|平台分支全走 build tag/GOOS，Windows 主路径零改动|

## Assumptions (temporary)

- 验证环境：WSL2（WSLg 支持 GUI）或独立 Linux 机器/容器
- MVP 目标形态：桌面版 + 浏览器访问模式均可用（待确认）
- 发布产物形态待确认（tar.gz / deb / AppImage）

## Decision (ADR-lite)

### D1 MVP 目标形态
**Context**: Linux 一期范围决定改造顺序与工作量分布
**Decision**: 桌面 + 浏览器访问模式都做，分两步：先浏览器模式（无 GTK 依赖，快速验证编译与核心逻辑），再补 webkit2gtk 桌面版
**Consequences**: 风险递进；总估算 7~11 人日；浏览器模式先跑通即拿到可用中间产物

### D2 验证环境
**Context**: 联调环境决定迭代速度
**Decision**: WSL2（本机 Win11，WSLg 支持 GUI 应用），Ubuntu 发行版
**Consequences**: 桌面版 GUI 可在 WSLg 直接验证；注意 WSLg 与原生桌面差异（Wayland 合成器），发布前如有条件在真机抽验

### D3 发布产物形态
**Context**: 分发方式决定 CI/发布工程量与用户安装体验
**Decision**: tar.gz（二进制 + 说明打包，解压即用）
**Consequences**: 零打包工程量；无桌面图标/菜单项；用户需自装 webkit2gtk；deb/rpm/AppImage 留二期

### D4 剪贴板与 Obsidian 集成
**Context**: 两者已有优雅降级，可延后也可一期补齐
**Decision**: 都进一期。剪贴板用 xclip（X11）/ wl-copy（Wayland）实现文本+文件复制；Obsidian 走 xdg-open obsidian:// URI（Linux Obsidian 同样注册该协议）
**Consequences**: 一期功能对等 Windows（除 WSL/注册表专属）；总估算上调约 1.5 天，8.5~12.5 人日；剪贴板需检测 Wayland/X11 会话类型选工具

## Requirements

- Linux 下 `wails build` 编译通过（桌面版）+ serve 模式编译通过（浏览器模式）
- Shell 配置平台化：Linux 返回 bash/zsh/fish，默认 shell 按 GOOS 决定
- 文件打开跨平台：OpenInExplorer/OpenWithDefaultApp/Obsidian URI 走 xdg-open
- 剪贴板 Linux 实现：xclip（X11）/ wl-copy（Wayland），文本 + 文件
- Obsidian Linux 集成：xdg-open obsidian:// URI，vault 归属判断复用现有逻辑
- 自更新 Linux 版 .sh 脚本（等进程退、替换、重启、自删）
- 前端 shell 枚举适配：默认值从后端设置读取，数据源对齐平台 shell 列表
- CI 增加 ubuntu job；release 产出 Linux tar.gz
- 平台差异代码走 build tag / GOOS 分支，Windows 主路径零改动

## Acceptance Criteria

- [ ] WSL2 环境 `wails build` 产出可运行二进制；serve 模式可浏览器访问
- [ ] Linux 桌面启动，终端可开 bash/zsh，关闭/杀进程无残留
- [ ] 文件打开/资源管理器定位/默认程序打开在 Linux 走 xdg-open 正常
- [ ] Linux 剪贴板复制文本与文件可用（X11 与 Wayland 至少 X11 实测，Wayland 有实现）
- [ ] Linux Obsidian 打开 vault 走 xdg-open 正常（未装时返回 ErrObsidianNotInstalled）
- [ ] 自更新流程 Linux 版脚本可用
- [ ] `go test ./...` 在 Linux 通过（含路径断言修复）
- [ ] Windows 侧全量测试回归通过，行为无变化
- [ ] CI 增加 Linux job；release 产出 Linux tar.gz
- [ ] README/部署文档补 Linux 章节（依赖安装、运行方式）

## Definition of Done

- 单测/集成测试覆盖新增平台分支
- lint / CI 绿
- README/部署文档更新 Linux 章节
- Windows 行为零回归

## Out of Scope (explicit)

- macOS 支持（build/darwin 目录存在但未验证）
- WSL shell 类型（Windows 专属）
- deb/rpm/AppImage 打包（二期）
- 原生 Linux 桌面（非 WSLg）实机深度验证（有条件抽验）

## Technical Approach

- 平台抽象沿用既有 `*_windows.go` / `*_other.go`（`!windows` tag）模式；需平台三分（win/linux/darwin 兜底）的场景用 GOOS 运行时判断或新增 `_linux.go`
- Shell 配置：`model/terminal.go` GetShellConfigs/ResolveShellConfig 加 GOOS 分支
- 文件打开：新增平台抽象层（如 service 文件内 `openInFileManager`/`openWithDefaultApp` 按平台实现），`fileoperation.go` 调抽象层
- 剪贴板：`util/clipboard_other.go` 改为真实现，`XDG_SESSION_TYPE` 或 wl-copy 探测决定 xclip/wl-copy
- 自更新：`service/update.go` buildUpdateBat/buildApplyBat 平行加 buildUpdateSh/buildApplySh，按 GOOS 选择
- 前端：shell 列表与默认值从后端设置流读取，删除硬编码 'powershell' 兜底（保留作为 Windows 默认）

## Implementation Plan (small PRs)

- PR0: WSL2 环境搭建 + 浏览器模式编译跑通（风险验证，不产代码或仅小修）
- PR1: 平台差异代码（shell 配置 + 文件打开 + 剪贴板 + Obsidian）
- PR2: 自更新 .sh + 前端 shell 枚举适配
- PR3: CI ubuntu job + Linux 测试路径断言修复
- PR4: 桌面版 webkit2gtk 联调 + release tar.gz 产物 + 文档

## Technical Notes

- 前次调研会话已完成代码级摸底，位置：本 PRD「What I already know」
- Wails v2.16 Linux 依赖：libwebkit2gtk-4.1-dev、libgtk-3-dev 等
- 平台抽象既有模式：`*_windows.go` / `*_other.go`（!windows tag），新代码沿用
