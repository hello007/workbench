# Linux 平台支持契约

> 来源：2026-09 Linux 支持改造（任务 09-22-linux，PR0-PR4）。本文沉淀 Linux 平台差异的执行契约，Windows 侧行为零回归为永久红线。

## 1. Scope / Trigger

- 触发：任何跨平台（Windows/Linux）编译、构建、分发、平台差异代码编写。
- 适配现状：`*_windows.go` / `*_other.go`（`!windows` tag）双文件模式 + 运行时 `runtime.GOOS` 分支并存，选型标准见 §2。

## 2. 平台抽象选型标准

| 场景 | 方案 | 实例 |
|---|---|---|
| API 字段平台独有（编译期不存在） | build tag 双文件 | `hideWindow()`：`hidewindow_windows.go`（SysProcAttr{HideWindow,CreationFlags}）/ `hidewindow_other.go`（返回 nil） |
| 同签名不同实现、逻辑短 | build tag 双文件 | `openInFileManager`/`openWithSystemHandler`：`fileoperation_windows.go`（explorer、cmd /c start）/ `fileoperation_other.go`（xdg-open） |
| 同函数内行为分叉、逻辑短（几行） | 运行时 `runtime.GOOS` | `buildUpdateSh` vs `buildUpdateBat` 选择、`updateAssetName()`、`buildCdCommand` POSIX 分支 |
| 纯逻辑（格式构造/解析） | 无 tag 独立文件，两平台同测 | `util/clipboard_payload.go`（gnome-copied-files 构造/解析） |

**判定口诀**：字段在对方平台编译不过 → 双文件；编译都过只是行为不同且分支短 → GOOS；可抽纯函数 → 无 tag 纯逻辑文件（利于双侧测试）。

## 3. Wails Linux 桌面编译依赖（关键 gotcha）

> **Warning**：Wails v2.16 Linux 桌面 CGO 默认绑定 `webkit2gtk-4.0`，Ubuntu 24.04 apt 仓库已移除该包（仅存 4.1）。桌面构建**必须**带 `webkit2_41` 标签切换到 `webkit2gtk-4.1`（`internal/frontend/desktop/linux/gtk.go` 内 `#cgo webkit2_41 pkg-config: webkit2gtk-4.1`）。

```bash
# 桌面构建（wails build 的 tag 集合 = outputType(desktop) + production + 用户 tag）
wails build -tags webkit2_41
# 等价裸 go 编译验证
go build -tags "desktop,production,webkit2_41" ./...
```

**依赖图分层**（决定装不装 GTK 依赖）：

| 操作 | 依赖图 | 需要 GTK/webkit |
|---|---|---|
| `go build/test ./model/ ./service/ ./util/ ./server/` | 无标签，`internal/app` 走桩实现（`app_default_unix.go`） | 不需要 |
| `go build ./...`（含主包） | 同上，主包不触桌面前端 CGO | 不需要（2026-09 WSL 24.04 无 webkit 实证） |
| `wails build` / 带 `desktop,production` tag | 桌面前端 CGO 进图 | **需要** + `webkit2_41` |
| 运行 serve 模式 | 纯 Go echo+WS | 不需要（运行态） |

## 4. Linux Shell 枚举与终端

- `model.GetShellConfigs()`/`ResolveShellConfig()` 按 GOOS：Linux 返回 `{bash /bin/bash}{zsh /bin/zsh}{fish /usr/bin/fish}{sh /bin/sh}`，默认 bash；Windows 四项（powershell/cmd/gitbash/wsl）逐字节保留。
- `buildCdCommand` POSIX 输出 `cd -- '<path>'\r`：`--` 防选项解析、单引号转义 `'\''`、`\r` 是 PTY 回车执行语义两平台都保留。`isUnixShellType` 白名单外非 Windows 兜底 POSIX。
- 前端默认 shell 兜底链（`store/settings.js` 单一常量 `FALLBACK_SHELL` + 单一解析函数 `resolveDefaultShell`，消费方统一引用）：用户设置 `defaultShell`（须在平台列表内）→ 后端 `GetShellConfigs` 首项（Windows=powershell / Linux=bash）→ `'powershell'`。用户设置为跨平台残留值（如 Windows 时期的 `gitbash`/`wsl` 在 Linux 主机）时收敛到首项，列表不可用（RPC 失败）时保留用户设置——保证下拉显示值 / 条件渲染 / tab 副标签 / 实际 PTY shellType 四处一致。`TerminalPanel` 有 `settingsReady` 门控防闪现；新增 shell 消费方禁止再写 `'powershell'` 字面量。
- 终端兜底目录平台判定（`TerminalPanel` `fallbackTerminalDir`）：列表首项 type 为 `powershell` → `'C:\'`，否则 `'/'`。禁用 `navigator.userAgent`——serve 模式 PTY 跑在服务端主机，UA 反映浏览器端设备，方向相反。

## 5. 剪贴板契约（Linux）

- 会话判定：`WAYLAND_DISPLAY` 非空 → `wl-copy`/`wl-paste`；否则 → `xclip`（X11）。
- 文件列表 target：`x-special/gnome-copied-files`，载荷格式（`util/clipboard_payload.go`）：首行 `copy`/`cut` + `\0`，随后 `file://` URI（百分号编码）逐行。cut 语义天然携带（对应 Windows `Preferred DropEffect`）。
- 读取降级：target 不可读/载荷无效 → `(nil, false, nil)`，与 Windows「无文件」语义对齐；工具未安装（`errors.Is(err, exec.ErrNotFound)`）→ 中文安装提示错误。

## 6. 自更新资产与解包

- 资产名（`service/update.go` `updateAssetName()`）：Windows `workbench.exe` / Linux `workbench-linux-amd64.tar.gz`。**与 release.yml 打包名严格一致**，改任一侧必须同步（常量注释互指 + `TestUpdateAssetName_MatchesPlatform` 锁定）。
- Linux 下载流：tar.gz 落盘 → `extractUpdateTarGz`（`update_extract.go`）取顶层 `workbench` 二进制 → 走既有 .sh 替换流程。
- 解包安全约束：仅提取 `Typeflag == TypeReg` 且 `filepath.Base(name) == "workbench"` 的成员（symlink/hardlink/目录全跳过）；输出路径硬编码 `destDir/workbench` 不消费 `hdr.Name`（路径穿越免疫）；写入经 `LimitReader` 限成员解压体积上限 `maxUpdateBinarySize`（512MB 常量，不用 asset FileSize——压缩包大小约束不了解压产物体积），超限删半成品报错（解压炸弹防御）；落盘后显式 `Chmod 0755`（防 umask 去位）。
- 更新脚本：Linux `buildUpdateSh`/`buildApplySh`（POSIX：kill -0 轮询、mv -f 失败 `|| exit 1` 中止防假更新（清理/重启仅在成功分支，路径经 `shellQuote` 单引号转义防注入）、nohup 脱离、`rm -f -- "$0"` 自删；apply.sh 失败分支额外清除 pending 并拉起旧版本——app.go 命中 pending 即 `os.Exit(0)`，不拉起应用将死循环打不开），`update_sh_syntax_test.go` 用 `sh -n` 静态校验拦截 bashism；`.sh` 必须 LF（CR 会坏 shebang，有测试断言）。

## 7. 构建与分发

- CI（ci.yml）：ubuntu-latest 跑全部门禁；`frontend/dist` 被 gitignore 但 `main.go` 有 `go:embed all:frontend/dist` —— CI 需预置占位 `index.html` + `assets/app.js`（冒烟测试 `fs.Glob(distFS, "assets/*.js")` 需真实文件名），vite build 会 `emptyOutDir` 重建真实产物。
- Release（release.yml）：`release-linux` job 经 `needs: release-windows` 串行（消除两 job 并发创建 Release 的首建竞态，Windows 成功 Linux 失败可单独 Re-run failed jobs 补传资产）；glibc 基线经 `container: ubuntu:22.04`（2.35，产物兼容老发行版）与 runner 镜像解耦（runner 用 ubuntu-latest，规避 22.04 runner 镜像 EOL 退役；容器内 root 无 sudo、镜像无 git 须 checkout 前显式安装）。产物 `workbench-linux-amd64.tar.gz` 扁平布局（二进制 + `build/README-linux.md`）。**`build/` 在 gitignore 中，白名单 `!build/README-linux.md`——新增需分发文件须同步白名单，否则发版必挂**。

## 8. 测试平台拆分模式

- Windows 专属断言 → `*_windows_test.go`（`//go:build windows`），实例：`hidewindow_windows_test.go`、`terminal_windows_test.go`。
- Linux 专属 → `*_linux_test.go`（`//go:build linux`），实例：`update_extract_linux_test.go`、`update_sh_syntax_test.go`。
- 跨平台参数化：同一测试内 `runtime.GOOS` 分支断言（附理由）或平台构造期望值，禁止两平台共享一份平台耦合断言（曾致 Linux 6 测试失败）。
- 测试路径断言禁硬编码 `D:/`、`C:\`，用 `t.TempDir()`（`util/testutil` 辅助）。

## 9. Wrong vs Correct

### Wrong
```go
// 主包代码直接内联 Windows 专属 API（update.go 旧 hideWindow 写法）
func hideWindow() *syscall.SysProcAttr {
    return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
} // Linux 编译失败：SysProcAttr 无此字段

// 前端兜底硬编码
const shellType = 'powershell' // Linux 用户默认终端变 PowerShell

// CI 占位只放 index.html
echo "" > frontend/dist/index.html // 冒烟测试 Glob assets/*.js 必红
```

### Correct
```go
// 平台双文件，调用点零感知
// hidewindow_windows.go: hideWindow() → &SysProcAttr{...}
// hidewindow_other.go:   hideWindow() → nil
```

## 10. Tests Required（平台改造验收面）

| 面 | Windows | Linux |
|---|---|---|
| `go build ./...` | 必绿 | 必绿（WSL 实机） |
| `go test ./service/ ./util/ ./model/` | 必绿 | 必绿 |
| `go vet ./...` | 手动跑须 `-unsafeptr=false`（clipboard_windows.go 既有误报） | 裸跑即可（编译图不含 windows 文件） |
| `wails build -tags webkit2_41` | — | 发布前必验 |
| `npm test` | 必绿（含平台分支用例） | 同 |
| CI ubuntu job | — | 推送后绿 |

## 相关

- PRD：`.trellis/tasks/09-22-linux/prd.md`（四决策：先浏览器后桌面、WSL2 验证、tar.gz、剪贴板+Obsidian 进一期）
- [cross-layer-contracts.md](cross-layer-contracts.md)：wailsjs 绑定同步规则（本次未触，因平台化均为实现内部）
- [browser-channel.md](browser-channel.md)：serve 模式无 GUI 依赖的运行态契约
