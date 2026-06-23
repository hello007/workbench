# 研究：在 Windows 上从 Go 后端可靠地用 Obsidian 打开指定文件夹（vault）

- **查询**：从 WorkBench（Wails v2 + Go，`os/exec`）在 Windows 上用 Obsidian 桌面版打开指定文件夹作为 vault 的最佳方式；对比 URI 协议 vs 直接 exe；隐藏控制台窗口；未安装检测；路径编码。
- **范围**：外部（官方文档/微软文档/Go 标准库）+ 内部（本项目现有实现模式）
- **日期**：2026-06-23

---

## 关键结论（TL;DR）

1. **首选「注册表预检 + URI 协议」方案**：先用 `golang.org/x/sys/windows/registry` 查 `HKEY_CLASSES_ROOT\obsidian\shell\open\command` 判断协议是否注册（即是否安装并运行过 Obsidian），再用 `cmd /c start "" "obsidian://..."` 启动。这与本项目 `OpenWithDefaultApp`（`service/fileoperation.go:179`）的 `cmd /c start` 模式一致，可复用 `util.HideCommandWindow`。
2. **URI 格式**：用绝对路径打开 vault，标准做法是 `obsidian://open?path=<URL 编码后的绝对路径>`。参数名确实是 **`path`**（非 `vault`）。还有等价简写 `obsidian:///D:/.../我的笔记`。
3. **官方文档没有「直接传文件夹打开 vault」的明确语义**，但社区大量实践（`obsidian://open?path=<vault 目录>` 或简写 `obsidian:///abs/vault/dir`）证明可用：Obsidian 会查找「包含该路径的最具体 vault」，当路径本身就是 vault 根目录（含 `.obsidian` 子目录）时即打开该 vault。
4. **不推荐照搬 VSCode 那样直接 `exec.Command("obsidian", path)`**：Obsidian 默认不加入 PATH。直接定位 `Obsidian.exe` 作为兜底可行，但其命令行参数官方未文档化（Electron 应用），不如 URI 协议稳定。
5. **未安装时**：若 `obsidian://` 协议未注册，`cmd /c start` 会弹出一个系统级「如何打开此链接」对话框（Windows ShellExecute 默认行为），而非静默失败。因此**必须预检**，否则用户体验差。

---

## 一、Obsidian URI 协议（来源：官方文档，已实读）

**官方文档原文**（`https://publish.obsidian.md/.../Extending+Obsidian/Obsidian+URI.md`，HTTP 200，9031 字节，2026-06-23 抓取）：

### 1.1 总格式

```
obsidian://action?param1=value&param2=value
```

可用 action：`open`、`new`、`daily`、`unique`、`search`、`choose-vault`。

官方对编码的明确警告：

> **Encoding**：确保值被正确进行 URI 编码。例如正斜杠 `/` 必须编码为 `%2F`，空格必须编码为 `%20`。未正确编码的「保留字符」可能破坏 URI 的解析。

### 1.2 `open` action 与 `path` 参数（本任务核心）

官方参数说明原文摘录：

- `vault`：vault 名称或 vault ID。
- `file`：文件名，或从 vault 根到文件的路径。
- **`path`**：一个绝对文件系统路径。
  - 使用此参数会**覆盖 `vault` 和 `file` 两者**。
  - 这会使应用**搜索包含该指定文件路径的最具体 vault**（the most specific vault which contains the specified file path）。
  - 然后剩余路径替换 `file` 参数。

官方示例：

- `obsidian://open?vault=my%20vault` —— 打开名为 `my vault` 的 vault，已开则聚焦窗口。
- `obsidian://open?path=%2Fhome%2Fuser%2Fmy%20vault%2Fpath%2Fto%2Fmy%20note` —— 查找包含该路径的 vault，剩余路径作为 `file`。

**简写格式**（官方）：

1. `obsidian://vault/my vault/my note` ≡ `obsidian://open?vault=my%20vault&file=my%20note`
2. `obsidian:///absolute/path/to/my note` ≡ `obsidian://open?path=%2Fabsolute%2Fpath%2Fto%2Fmy%20note`

### 1.3 关于「打开文件夹作为 vault」的语义（重点澄清）

官方文档**字面上**把 `path` 描述为「绝对文件系统路径（指向文件）」，未直接说「指向一个文件夹」。但结合：

- 简写 `obsidian:///absolute/path` 等价于 `open?path=<该绝对路径>`；
- Obsidian 的查找逻辑是「**包含**该路径的最具体 vault」——当传入的路径**本身就是 vault 根目录**（即包含 `.obsidian` 配置子目录）时，「剩余路径」为空，效果就是打开该 vault 本身。

社区实证（forum.obsidian.md 搜索 API，2026-06-23 抓取）：

- 用户 `ksandvik`：`$ open obsidian://path/to/Vault` 用于从命令行启动 vault（实测可工作，唯一问题是重复调用会在已开 vault 窗口里再开新窗口而非切到第二个 vault）。
- 用户 `technicalpickles`（8 赞）：读取 `obsidian.json` 中 vault 的 `path` 字段，然后 `open path/to/vault/.ob...`（即用 path 方式打开）。

**结论**：用 `obsidian://open?path=<URL编码的 vault 绝对路径>` 打开一个文件夹作为 vault 是社区普遍使用且可工作的方式。对「尚未注册为 vault 的文件夹」，官方文档**未明确说明自动注册或信任提示流程**——根据 Obsidian 的 vault 机制，一个目录必须先被「添加为 vault」才会被识别（`obsidian.json` 注册表）。传入一个未注册路径时，最可能的行为是：Obsidian 启动但找不到匹配 vault，停留在 vault 选择器/空状态（**文档未保证自动注册**）。因此实现侧**不应假设未注册目录会自动变成 vault**，可在 UI 提示用户「需先在 Obsidian 中将其添加为 vault」。

> 若想强制打开 vault 管理器让用户手动选/添加，可调用 `obsidian://choose-vault`（官方 action，无参数）。

### 1.4 协议注册（官方）

> 在 Windows 和 macOS 上，运行一次应用即可在本机注册 Obsidian URI 协议。

这意味着：**安装但从未运行过**的 Obsidian，其 `obsidian://` 协议可能尚未注册——这是注册表预检要覆盖的场景（见第三节）。

**来源链接**：
- 官方 URI 文档（渲染版）：https://help.obsidian.md/Extending+Obsidian/Obsidian+URI
- 官方 URI 文档（原始 markdown，本报告实读）：`https://publish-01.obsidian.md/access/f786db9fac45774fa4f0d8112e232d67/Extending+Obsidian/Obsidian+URI.md`
- 论坛搜索（path 打开 vault 主题）：https://forum.obsidian.md/search.json?q=open%20vault%20obsidian%3A%2F%2Fopen%20path

---

## 二、从 Go 在 Windows 启动 `obsidian://` URI 的几种方式对比

本项目当前模式（`service/fileoperation.go`）：
- `OpenWithDefaultApp`（:179）：`exec.Command("cmd", "/c", "start", "", path)` + `util.HideCommandWindow(cmd)`。
- `OpenInVSCode`（:157）、`OpenInWarp`（:165）：`exec.Command(name, path)` + `HideCommandWindow`。

`util/exec_windows.go`：`HideCommandWindow` 设置 `cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}`（隐藏子控制台窗口）。

### 2.1 方式 A：`cmd /c start "" "obsidian://..."` （推荐）

- **命令构造**：`exec.Command("cmd", "/c", "start", "", uri)`。
- **第一个空串 `""`**：是 `start` 的「窗口标题」参数。微软 `start` 文档（`learn.microsoft.com/.../windows-commands/start`，HTTP 200，已抓取）明确：`"title"` 是第一个参数，用于指定窗口标题栏。**当被启动的目标（URI）含空格时，不加空标题会让 `start` 把 URI 的第一段误当标题**——这是经典坑，故必须写 `start ""`。
- **隐藏窗口**：兼容 `HideCommandWindow`。`cmd.exe` 进程本身会闪现，但 `HideWindow:true` 后不显示；`start` 本身是异步派发，`cmd /c` 派发完即退出，`Start()` 即返回。
- **优劣**：与本项目现有 `OpenWithDefaultApp` 完全同构，一致性最好；走 ShellExecute 路径，会尊重用户默认协议处理器。

### 2.2 方式 B：`rundll32 url.dll,FileProtocolHandler "obsidian://..."`

- **命令构造**：`exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", uri)`。
- **是否文档化**：**否**。微软官方 `rundll32` 文档（`learn.microsoft.com/.../windows-commands/rundll32`，HTTP 200，已抓取）**未列出** `FileProtocolHandler` 入口点——这是 `url.dll` 的一个**未文档化但业界广泛使用**的入口，常用于「打开 URL / 默认浏览器」。
- **隐藏窗口**：可配合 `HideCommandWindow`，但 `rundll32` 不闪窗（非控制台程序），较干净。
- **优劣**：属于「事实标准」但官方不背书，长期有被弃用风险；且对自定义协议（非 http）的兼容性与 `start`/ShellExecute 一致，没有额外优势。**不推荐优先使用**。

### 2.3 方式 C：`powershell Start-Process "obsidian://..."`

- **命令构造**：`exec.Command("powershell", "-NoProfile", "-Command", "Start-Process '"+uri+"'")`。
- **隐藏窗口**：`powershell.exe` 是控制台程序，必须配合 `HideCommandWindow`，否则会闪黑窗。
- **优劣**：启动开销大（拉起 PowerShell 进程），命令注入风险（URI 拼进 `-Command` 字符串，需转义单引号），无优于 `start` 之处。**不推荐**。

### 2.4 直接 `exec.Command` 调上述

即上述三种都是通过 `exec.Command` 构造。`exec.Command("cmd","/c","start",...)` 即为本项目现成模式。

### 2.5 更优选项（本项目未用但可考虑）：`ShellExecute` Win32 API

`golang.org/x/sys/windows` 提供 `windows.ShellExecute(hwnd, verb, file, args, cwd, showCmd)`。直接调用 `ShellExecute(0, "open", "obsidian://...", "", "", windows.SW_SHOWNORMAL)` 可**完全不经 cmd.exe / rundll32**，无子进程窗口、无引号转义问题，是 Windows 上「打开 URI/文件」最干净的底层 API。代价是新增一处 syscall 调用，与现有 `cmd /c start` 风格不同。可作为「最优但偏离现有模式」的备选。

**方式对比汇总**：

| 方式 | 命令构造 | 隐藏窗口 | 文档化 | 注入风险 | 推荐度 |
|---|---|---|---|---|---|
| A. `cmd /c start "" uri` | `exec.Command("cmd","/c","start","",uri)` | 需 `HideCommandWindow` | 是（start 官方文档） | 低（URI 作为独立参数） | **推荐** |
| B. `rundll32 url.dll,FileProtocolHandler` | `exec.Command("rundll32.exe","url.dll,FileProtocolHandler",uri)` | 基本不闪窗 | **否**（未文档化入口） | 低 | 不优先 |
| C. `powershell Start-Process` | `exec.Command("powershell","-NoProfile","-Command","Start-Process '...'")` | 需 `HideCommandWindow` | 是 | **高**（字符串拼接） | 不推荐 |
| —. `windows.ShellExecute` | 直接 syscall | 无窗口 | 是（Win32 API） | 无 | 最优但偏离现模式 |

**来源**：
- 微软 `start` 文档：https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/start
- 微软 `rundll32` 文档（确认未列 `FileProtocolHandler`）：https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/rundll32
- 微软 `ShellExecute`/`ShellExecuteA` Win32 API：https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shellexecutea

---

## 三、未安装 Obsidian 时的行为与预检

### 3.1 协议未注册时 `cmd /c start` 会怎样

当 `obsidian://` 协议在系统未注册任何处理器时，`start`（底层 ShellExecute）的默认行为是：**弹出系统级对话框**「如何打开此链接 / 你想如何打开此文件」（Windows ShellExecute 对未注册协议的标准 UI），**而非静默失败、非返回错误码**。

这意味着：若不预检、用户未装 Obsidian，点击「用 Obsidian 打开」会蹦出无意义的系统弹窗，体验差。**因此必须预检**。

### 3.2 从 Go 预检 Obsidian 是否安装（注册表）

Obsidian 在 Windows 上注册协议后，会写入：

```
HKEY_CLASSES_ROOT\obsidian\shell\open\command
```

其默认值形如 `"C:\Users\<用户>\AppData\Local\Obsidian\Obsidian.exe" --run "obsidian://%1"`（`%1` 为 URI 占位符）。

本项目 `go.mod` 已依赖 `golang.org/x/sys v0.38.0`（已核实），其子包 **`golang.org/x/sys/windows/registry`**（注意路径是 `windows/registry`，不是顶层 `registry`）提供了所需 API（已读源码 `key.go` 确认）：

- `registry.CLASSES_ROOT`（= `HKEY_CLASSES_ROOT`）
- `registry.OpenKey(k, path, access)` —— 返回 `(Key, error)`，键不存在时返回错误。
- `(*Key).ReadString(name)` —— 读取默认值传 `""`。

预检伪代码：

```go
func isObsidianRegistered() bool {
    // 1) 优先查 HKEY_CLASSES_ROOT\obsidian (系统级/全局注册)
    k, err := registry.OpenKey(registry.CLASSES_ROOT, `obsidian\shell\open\command`, registry.QUERY_VALUE)
    if err == nil {
        v, _ := k.ReadString("")
        k.Close()
        if strings.Contains(v, "Obsidian") { return true }
    }
    // 2) 兜底查当前用户级注册 HKEY_CURRENT_USER\Software\Classes\obsidian\...
    k2, err2 := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\obsidian\shell\open\command`, registry.QUERY_VALUE)
    if err2 == nil {
        v, _ := k2.ReadString("")
        k2.Close()
        if strings.Contains(v, "Obsidian") { return true }
    }
    return false
}
```

### 3.3 备选预检：默认安装路径

Obsidian 默认安装到 `%LOCALAPPDATA%\Obsidian\Obsidian.exe`（即 `C:\Users\<用户>\AppData\Local\Obsidian\Obsidian.exe`）。可用 `os.Stat(filepath.Join(os.Getenv("LOCALAPPDATA"), "Obsidian", "Obsidian.exe"))` 判断。但：

- 用户可能装到别处（自定义安装路径），**路径检测不可靠**；
- **安装存在 ≠ 协议已注册**（装了但从未运行过，协议可能没注册）。

因此**注册表检测是更可靠的「能否响应 obsidian://」判据**，路径检测仅作辅助/兜底定位 exe。

---

## 四、直接启动 `Obsidian.exe` 作为兜底

### 4.1 Obsidian.exe 的命令行参数

**官方文档未公开 `Obsidian.exe` 的命令行参数**（Obsidian 是 Electron 应用，Electron 层参数与 app 自定义参数混合，文档缺失）。从注册表 `shell\open\command` 的默认值可知，Obsidian 处理 URI 的方式是：

```
"C:\...\Obsidian.exe" --run "obsidian://open?path=..."
```

即 **exe 仍然接收一个 `obsidian://` URI 作为参数**（配合 `--run` 标志）。**没有**像 VSCode 那样的「直接传文件夹路径就打开」的稳定 CLI 约定。

### 4.2 直接 exe 方案的优劣

- **优点**：不依赖协议注册，只要文件存在就能启动进程。
- **缺点**：
  1. exe 路径需定位（`%LOCALAPPDATA%\Obsidian\Obsidian.exe` 仅是默认，可被改）；
  2. 仍需传 `obsidian://...` URI 形式的参数才能定位 vault，**绕不开 URI 构造**；
  3. 多用户/多实例时可能无法正确复用已开窗口（与协议路由不同）。

**结论**：直接 exe 的收益有限（仍要构造 URI），**不作为主方案**。仅在「协议注册表查到了 exe 路径」时，可选地用查到的 exe 全路径 + `--run` + URI 来启动（比 `cmd /c start` 多一步、收益不明显）。综合建议：**主方案用 `cmd /c start` + URI；exe 定位仅用于「判断是否安装」的辅助信号，不用于实际启动**。

---

## 五、路径编码（含空格/中文/反斜杠）

vault 绝对路径示例：`C:\Users\张三\我的笔记`。

### 5.1 正确做法

URI 中 `path` 值需要 **URL/百分号编码**。Go 标准库：

- **`net/url`**：`url.QueryEscape(path)` —— 会把空格编码为 `+`（表单编码风格），把 `/` 编码为 `%2F`。**但 `+` 表示空格在 query 里通常可接受，Obsidian 官方示例用的是 `%20`**。为与官方示例完全一致，避免歧义，**推荐手动用 `strings.ReplaceAll(url.QueryEscape(path), "+", "%20")`**，或直接用百分号编码。
- 更稳妥：`net/url` 没有公开 `PathEscape` 用于 query value 的全字符编码，但可用：

```go
func encodeVaultPath(p string) string {
    // 先转成正斜杠，避免反斜杠被当转义
    p = filepath.ToSlash(p)
    e := url.QueryEscape(p)
    return strings.ReplaceAll(e, "+", "%20")
}
```

### 5.2 坑

| 坑 | 说明 | 处理 |
|---|---|---|
| **反斜杠 `\`** | Windows 路径含 `\`，`QueryEscape` 不处理 `\`（保留原样），导致 URI 非法 | **先 `filepath.ToSlash` 转成 `/`** 再编码 |
| **空格** | `QueryEscape` 输出 `+`，官方文档示例用 `%20` | `+`→`%20` 替换，与官方一致 |
| **中文（多字节 UTF-8）** | `QueryEscape` 会逐字节百分号编码，正确 | 直接编码即可 |
| **盘符冒号 `C:`** | `QueryEscape` 会把 `:` 编码为 `%3A`（query 值里安全） | 可接受；`obsidian:///C:/...` 简写场景下冒号不编码也常可用，但 query 形式编码更安全 |
| **`#` `?` `&`** | 保留字符，必须编码，否则破坏 URI 解析 | `QueryEscape` 会编码（`#`→`%23` 等），正确 |

构造最终 URI：

```go
uri := "obsidian://open?path=" + encodeVaultPath(vaultPath)
// 或简写（注意三斜杠 + 编码路径）
// uri := "obsidian:///" + encodeVaultPath(vaultPath)
```

---

## 六、推荐方案（含取舍）

### 6.1 主方案：注册表预检 + URI 协议（`cmd /c start`）

**取舍依据**：
- 与本项目 `OpenWithDefaultApp`（`service/fileoperation.go:179`）模式一致，复用 `util.HideCommandWindow`。
- `cmd /c start "" uri` 走 ShellExecute，尊重用户默认协议处理器，文档化、无注入风险。
- 注册表预检避免未安装时弹出系统「如何打开」对话框。
- 直接 exe 方案收益有限（仍需构造 URI）且官方未文档化 CLI，故不作主方案。

### 6.2 不推荐「先协议、失败兜底 exe」的组合

理由：`cmd /c start` 在协议未注册时**不会返回有用错误码**（而是弹系统对话框），Go 侧拿不到可靠「失败」信号来触发兜底。因此**应改为「先预检注册表，未注册直接走『未安装』提示分支」，而非运行时试错兜底**。

### 6.3 Go 实现伪代码要点

```go
import (
    "os/exec"
    "path/filepath"
    "strings"
    "net/url"

    "golang.org/x/sys/windows/registry"  // go.mod 已有 golang.org/x/sys v0.38.0

    "<项目>/util"
)

// 1) 预检：obsidian:// 协议是否注册（优先 HKCR，兜底 HKCU）
func obsidianProtocolRegistered() (string, bool) {
    for _, root := range []struct{ k registry.Key; sub string }{
        {registry.CLASSES_ROOT, `obsidian\shell\open\command`},
        {registry.CURRENT_USER, `Software\Classes\obsidian\shell\open\command`},
    } {
        if key, err := registry.OpenKey(root.k, root.sub, registry.QUERY_VALUE); err == nil {
            val, _ := key.ReadString(""); key.Close()
            if strings.Contains(strings.ToLower(val), "obsidian") {
                return val, true
            }
        }
    }
    return "", false
}

// 2) 路径编码（反斜杠→正斜杠，空格用 %20）
func encodeObsidianPath(p string) string {
    e := url.QueryEscape(filepath.ToSlash(p))
    return strings.ReplaceAll(e, "+", "%20")
}

// 3) 打开
func OpenInObsidian(vaultPath string) error {
    if _, ok := obsidianProtocolRegistered(); !ok {
        // 未安装/未注册：返回友好错误，由前端弹提示（引导用户安装并运行一次 Obsidian）
        return fmt.Errorf("未检测到 Obsidian，请先安装并至少运行一次以注册 obsidian:// 协议")
    }
    uri := "obsidian://open?path=" + encodeObsidianPath(vaultPath)
    // 复用现有 cmd /c start 模式，第一个 "" 是窗口标题（防 URI 含空格被当标题）
    cmd := exec.Command("cmd", "/c", "start", "", uri)
    util.HideCommandWindow(cmd)  // 隐藏 cmd 子窗口
    return cmd.Start()
}
```

### 6.4 要点清单

1. **用 URI 协议，不用直接 exe**：`obsidian://open?path=<编码后绝对路径>`；参数名是 `path`。
2. **`cmd /c start "" uri` + `util.HideCommandWindow`**：与 `OpenWithDefaultApp` 同构，`""` 必须保留。
3. **注册表预检**：`golang.org/x/sys/windows/registry` 查 `HKCR\obsidian\shell\open\command`（兜底 `HKCU\Software\Classes\...`），未注册则提示安装，**不要试错兜底**（`start` 失败时弹系统对话框而非返回错误）。
4. **路径编码**：`filepath.ToSlash`（去反斜杠）→ `url.QueryEscape` → `+` 替换为 `%20`，与官方示例一致。
5. **未注册为 vault 的文件夹**：官方未保证自动注册；UI 可提示「需先在 Obsidian 中将该目录添加为 vault」，或调用 `obsidian://choose-vault` 打开 vault 管理器。

---

## 相关内部文件

| 文件路径 | 说明 |
|---|---|
| `service/fileoperation.go:155-182` | 现有 `OpenInVSCode`/`OpenInWarp`/`OpenWithDefaultApp` 实现模式，新方法应与之一致 |
| `util/exec_windows.go` | `HideCommandWindow(cmd)`，设置 `SysProcAttr.HideWindow` |
| `go.mod:10` | 已依赖 `golang.org/x/sys v0.38.0`，含 `windows/registry` 子包，零新增依赖 |

## Caveats / 未完全确定项

- **Obsidian.exe CLI 参数官方未文档化**：第二节结论基于「注册表 `shell\open\command` 实际取值 `--run "obsidian://%1"`」与社区经验，非官方明确声明；故不建议直接 exe 作为主路径。
- **「文件夹直接作为 vault」的自动注册行为官方未明确**：本报告基于官方 `path` 语义描述 + 社区实证推断「传入 vault 根目录即打开该 vault」可用，但「传入尚未注册为 vault 的文件夹」是否会自动注册，官方未保证，建议实现侧提示用户。
- **`rundll32 url.dll,FileProtocolHandler` 为未文档化入口**：不建议依赖。
- **`start` 在协议未注册时弹系统对话框**这一行为，是基于 Windows ShellExecute 对未注册协议的通用机制推断（官方未对 `start` 单独说明），属高度可靠但非逐字引用；实现侧用注册表预检规避即可，不依赖该失败信号。
- 本报告基于 2026-06-23 抓取的官方文档（HTTP 200、原始 markdown）与本项目源码（`go.mod`、`util/exec_windows.go`、`service/fileoperation.go`），未在真实 Windows 环境实测每一命令；建议落地时在 `cmd /c start ""` 与 `ShellExecute` 两种之间做一次实测取舍。
