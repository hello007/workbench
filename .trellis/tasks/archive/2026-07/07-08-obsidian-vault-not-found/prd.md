# Obsidian 打开未注册 vault 目录报错处理

## Goal

解决 WorkBench「用 Obsidian 打开」功能在目标目录未注册为 Obsidian vault 时弹出 `Vault not found` 错误的问题。打开前预检 vault 注册状态：未注册时弹确认框提供两条路径--①「打开仓库管理器」（复制目录路径到剪贴板 + 跳转 `choose-vault` 手动添加）；②「自动注册并打开」（自动写入 obsidian.json 注册表并打开，含二次确认与风险预告）。

## Requirements

### 后端（Go）

**方案 A 基础（已实现）**：归属判断纯函数 + 预检 + 哨兵错误 `ErrObsidianNotInstalled`/`ErrVaultNotRegistered` + app 状态码翻译 + `OpenObsidianVaultManager`。

**需求 1：复制路径到剪贴板**
- `util/clipboard_windows.go` 新增 `WriteClipboardText(text string) error`（CF_UNICODETEXT 格式）；新建 `util/clipboard_other.go` 非 Windows 兜底（返回不支持的 error）。
- `service/fileoperation.go` 新增 `CopyObsidianVaultPath(path string) error`：`resolveObsidianVault` 解析 vaultPath（文件夹->自身，文件->父目录）-> `util.WriteClipboardText(vaultPath)`。
- `app.go` 新增 `App.CopyObsidianVaultPath(path string) bool`。

**需求 2：自动注册并打开**
- `service/obsidian_windows.go` 新增 `isObsidianRunning() bool`（`tasklist /FO CSV /NH` 枚举，过滤 `obsidian.exe`）；`obsidian_other.go` 兜底返回 false。
- `service/obsidian_vault.go` 新增：
  - `loadFullConfig() (*obsidianConfig, error)`：读完整 obsidian.json，**保留未知顶层字段**（用 `map[string]json.RawMessage` 或等效，避免回写丢失 `updateDisabled` 等）。
  - `newVaultID(existing map[string]VaultEntry) string`：`crypto/rand` 生成 16 位 hex，冲突检测。
  - `backupConfig(cfgPath) (string, error)`：写 `.bak.<unix>` 备份。
  - `atomicWriteConfig(cfgPath, cfg) error`：临时文件（同目录）+ `os.Rename` 原子替换。
- `service/fileoperation.go` 新增 `AutoRegisterAndOpen(path, obsidianPath string) error`，新增哨兵错误 `ErrObsidianRunning`。流程：
  1. `resolveObsidianVault` -> vaultPath。
  2. useExe/协议检测（未检测到 -> `ErrObsidianNotInstalled`）。
  3. `isObsidianRunning`（true -> `ErrObsidianRunning`）。
  4. `loadFullConfig` -> 去重（按 `resolvePath` 已注册则直接发 URI 打开）。
  5. `backupConfig` -> `newVaultID` + 追加 `{id: {path, ts}}`（不建窗口缓存、不改 open、不创建 .obsidian）-> `atomicWriteConfig`。
  6. 发 URI 打开。
- `app.go` 新增 `App.AutoRegisterAndOpen(path string) string`：返回状态码 `""`（成功）/`"running"`/`"not-installed"`/`"failed"`。

### 前端（Vue3）
- 重新生成 wailsjs 绑定（`CopyObsidianVaultPath`、`AutoRegisterAndOpen`）。
- 三处 `handleOpenObsidian` 的 `not-registered` 分支改为三按钮确认框（`ElMessageBox.confirm` + `distinguishCancelAndClose`）：
  - confirmButton「自动注册并打开」-> 二次确认（预告信任提示+备份+运行中需关闭）-> 调 `AutoRegisterAndOpen(path)`，按状态码提示。
  - cancelButton「打开仓库管理器」-> 调 `CopyObsidianVaultPath(path)` + 提示已复制 -> 调 `OpenObsidianVaultManager()`。
  - close（X）-> 取消。

## Acceptance Criteria

- [ ] 目标目录已注册 vault（或在其内）-> 正常打开。
- [ ] 未注册 -> 弹三按钮确认框；「打开仓库管理器」复制路径到剪贴板+提示+跳转 choose-vault；「自动注册并打开」二次确认后自动注册并打开。
- [ ] 自动注册：Obsidian 未运行 -> 写入 obsidian.json + 打开成功；Obsidian 运行中 -> 提示先关闭重试（不 taskkill）。
- [ ] 自动注册前自动备份 obsidian.json；原子写不损坏配置；保留未知顶层字段。
- [ ] 未检测到 Obsidian -> 提示去设置配置。
- [ ] obsidian.json 读取失败 -> 降级到现状尽力打开。
- [ ] 后端单测：归属判断、注册表读取、`isAncestorOrEqual`、`newVaultID`（格式+冲突）、`atomicWriteConfig`（原子性/保留未知字段）、`loadFullConfig`（防御性）。

## Definition of Done

- Go 单测新增/更新；`wails build` 通过；前端测试通过。
- `README.md` / `docs/功能说明.md` 更新（完成后确认）。
- 出错路径有可读提示。

## Technical Approach

方案 A（已实现）+ 需求 1 复制路径（`WriteClipboardText` CF_UNICODETEXT）+ 需求 2 自动注册（进程检测 + 原子写 + 备份 + 去重）。自动注册三前提：信任提示预告、Obsidian 未运行写入、原子写+备份。

## Decision (ADR-lite)

- **方案 A（预检+引导）**：已实现，基础不变。
- **需求 1（复制路径）**：点击「打开仓库管理器」时复制 vaultPath 文本到剪贴板，方便用户在 Obsidian 路径栏粘贴。
- **需求 2（自动注册）**：有条件可行（见 `research/obsidian-auto-register.md`）。采用「默认显示+二次确认」：未注册确认框并列两按钮；自动注册前二次确认预告信任提示+备份；Obsidian 运行中引导手动关闭重试（不 taskkill）；不创建 `.obsidian`、不建窗口缓存、不改 open 字段。
- **Consequences**：自动注册改 Obsidian 配置（有备份+原子写），首次打开必弹信任提示（UI 预告）；obsidian.json 结构非官方公开，防御性解析、保留未知字段。

## Out of Scope

- 跨平台（仅 Windows）。
- 绕过信任提示（不可行，UI 预告）。
- `taskkill` 强杀 Obsidian（风险高，引导手动关闭）。
- 创建 `.obsidian/app.json` 或窗口缓存文件。
- vault 列表可视化管理。

## Research References

- [`research/obsidian-vault-registry.md`](research/obsidian-vault-registry.md) - obsidian.json 结构、归属判断、自动注册初步风险。
- [`research/obsidian-auto-register.md`](research/obsidian-auto-register.md) - 自动注册实操可行性、Go 伪代码、信任提示不可绕过、进程检测、原子写。

## Technical Notes

- 现状代码：`service/fileoperation.go`、`service/obsidian_vault.go`、`service/obsidian_windows.go`/`obsidian_other.go`、`app.go`、`service/obsidian_test.go`、`util/clipboard_windows.go`、前端三组件 + wailsjs 绑定。
- 新增文件：`util/clipboard_other.go`（非 Windows 兜底）。
- `obsidian_vault.go` 增加 `loadFullConfig`/`newVaultID`/`backupConfig`/`atomicWriteConfig`。
- `obsidian_windows.go` 增加 `isObsidianRunning`、`obsidian_other.go` 兜底。
- `go.mod` 已依赖 `golang.org/x/sys`；新增仅标准库（`crypto/rand`/`encoding/hex`/`encoding/json`/`os`/`path/filepath`/`strings`/`time`/`syscall`）。

## Implementation Plan（小步）

- **Step1 需求1**：`WriteClipboardText` + `CopyObsidianVaultPath` + App 桥接 + 前端「打开仓库管理器」复制+提示。
- **Step2 需求2 后端**：`isObsidianRunning` + `loadFullConfig`/`newVaultID`/`backupConfig`/`atomicWriteConfig` + `AutoRegisterAndOpen` + App 状态码 + 单测。
- **Step3 需求2 前端**：三按钮确认框 + 二次确认 + 状态码分流（三处组件统一）。
- **Step4 收尾**：实测 + `README.md`/`docs/功能说明.md` 更新。
