# Error Handling

> How errors are handled in this project.

---

## Overview

<!--
Document your project's error handling conventions here.

Questions to answer:
- What error types do you define?
- How are errors propagated?
- How are errors logged?
- How are errors returned to clients?
-->

(To be filled by the team)

---

## Error Types

<!-- Custom error classes/types -->

本项目用**哨兵错误（sentinel error）**区分外部工具调用的不同失败状态。定义在 service 层，用 `errors.New`，匹配用 `errors.Is`。

```go
// service/fileoperation.go
var (
    ErrObsidianNotInstalled  = errors.New("未检测到 Obsidian")
    ErrVaultNotRegistered    = errors.New("目标目录未注册为 Obsidian vault")
)
```

约定：
- 哨兵错误仅用于「需要前端区分并分别处理」的失败状态（如未安装 vs 未注册）。单一失败状态仍用普通 `fmt.Errorf`。
- 哨兵错误名用 `Err<主体><状态>` 形式，置于对应 service 文件顶部、紧邻使用它的方法。

---

## Error Handling Patterns

<!-- Try-catch patterns, error propagation -->

### Pattern: 外部工具调用多状态处理（service 哨兵 -> app 状态码 -> 前端分流）

**Problem**：调用外部工具（如 Obsidian）时需区分多种失败状态（未安装 / 未配置 / 目标不满足前置条件），单一 `bool` 返回值不足以让前端分别引导用户。

**Solution**：三层分工--
1. **service 层**返回 `error`，用哨兵错误标记各类失败状态；
2. **app 桥接层**用 `errors.Is` 将哨兵翻译为状态码字符串；
3. **前端**按状态码分流（不同提示 / 确认框 / 跳转）。

**Signatures**（以 Obsidian 为例）：

| 层 | 签名 | 返回 |
|---|---|---|
| service | `OpenInObsidian(path, obsidianPath string) error` | `nil` / 哨兵 error / 普通 error |
| app | `OpenInObsidian(path string) string` | `""`(成功) / `"not-installed"` / `"not-registered"` |
| 前端绑定 | `OpenInObsidian(path): Promise<string>` | 同上状态码 |

**Validation & Error Matrix**（Obsidian 预检）：

| 条件 | service 返回 | app 状态码 | 前端行为 |
|---|---|---|---|
| 路径不存在 | `fmt.Errorf("路径不存在...")` | `"not-installed"`（兜底） | 错误提示 |
| 未配置 exe 且协议未注册 | `ErrObsidianNotInstalled` | `"not-installed"` | 提示去设置配置 |
| 目录不在任何已注册 vault 内 | `ErrVaultNotRegistered` | `"not-registered"` | 确认框引导跳转 vault 管理器 |
| 成功打开 | `nil` | `""` | 无提示 |
| obsidian.json 读取失败 | `nil`（降级直接发 URI + 记日志） | `""` | 无提示（不比现状差） |

**Example**（app 层翻译）：

```go
func (a *App) OpenInObsidian(path string) string {
    var obsidianPath string
    if settings, err := a.settingsSvc.Load(); err == nil {
        obsidianPath = settings.ObsidianPath
    }
    err := a.fileOpSvc.OpenInObsidian(path, obsidianPath)
    if err == nil {
        return ""
    }
    switch {
    case errors.Is(err, service.ErrObsidianNotInstalled):
        return "not-installed"
    case errors.Is(err, service.ErrVaultNotRegistered):
        return "not-registered"
    default:
        println("Error:", err.Error())
        return "not-installed" // 兜底：未知错误按"未检测到"处理
    }
}
```

**Why**：哨兵错误让 service 层保持「返回 error」的统一签名（不破坏现有 OpenIn* 模式），状态码字符串对前端最友好（直接 switch，无需解析 error 文案）。降级路径保证预检本身失败时不阻塞主流程。

**Extensibility**：未来其他外部工具（VSCode/Warp）若需区分多状态，照此模式：service 加哨兵 -> app 翻译状态码 -> 前端分流。

### Pattern: 修改外部应用配置文件（原子写 + 备份 + 保留未知字段）

**Problem**：自动注册 Obsidian vault 需改写 `%APPDATA%\obsidian\obsidian.json`。直接覆写有三大风险：①外部应用运行时可能回写覆盖（内存缓存）；②写入中途崩溃导致文件损坏；③结构化解析后回写丢失未声明的顶层字段（如 `updateDisabled`）。

**Solution**（三件套）：
1. **进程检测**：写入前用 `tasklist /FO CSV /NH` 检测目标应用是否运行，运行中则中止并返回哨兵错误引导用户手动关闭（不强杀），规避回写覆盖。检测失败时**保守视为运行中**（数据安全优先）。
2. **原子写 + 备份**：先备份原文件到 `<path>.bak.<unix_ts>`；写同目录临时文件 `<path>.tmp.<pid>` -> `os.Rename` 原子替换（同目录保证同分区原子）；失败清理临时文件。
3. **保留未知字段**：用 `map[string]json.RawMessage` 解析顶层，仅修改已知键（`vaults`），其余 RawMessage 原样回写，避免丢失外部应用新增的顶层字段（跨版本兼容）。

**Signatures**（Obsidian 自动注册为例）：`AutoRegisterAndOpen(path, obsidianPath) error` -> 哨兵 `ErrObsidianRunning`/`ErrObsidianNotInstalled`，app 翻译状态码 `""`/`"running"`/`"not-installed"`/`"failed"`。

**Why**：外部应用配置文件是其私有状态，WorkBench 作为外部写入者必须最小侵入、可回滚、不损坏。原子写保证「全有或全无」，保留未知字段保证跨版本兼容，进程检测规避运行时覆盖。

**Related**：`research/obsidian-auto-register.md`。

---

## API Error Responses

<!-- Standard error response format -->

(To be filled by the team)

---

## Common Mistakes

<!-- Error handling mistakes your team has made -->

### Common Mistake: `cmd /c start` 不感知外部应用内部错误

**Symptom**：用 `cmd /c start "" "obsidian://open?path=X"` 启动 Obsidian，目标目录未注册为 vault 时 Obsidian 弹「Vault not found」错误，但 Go 侧 `cmd.Start()` 返回 `nil`（成功），前端拿到成功状态、无感知。

**Cause**：`start`（底层 ShellExecute）只负责「派发 URI 给默认处理器」，处理器（Obsidian）内部的报错不会回传给启动方。`cmd.Start()` 的成功仅代表「成功派发」，不代表「外部应用成功打开目标」。

**Fix**：对「目标是否满足外部应用前置条件」做**打开前预检**（如读 obsidian.json 判断目录是否在已注册 vault 内），不依赖启动返回值判断成功。

**Prevention**：凡是用 `cmd /c start` / `ShellExecute` 启动外部应用并传 URI/参数的场景，若外部应用有「目标需满足前置条件」的语义（vault 注册、文件存在性、协议参数合法性等），一律在 Go 侧预检，不要假设「启动成功 = 操作成功」。
