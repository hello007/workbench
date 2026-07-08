# Research: Obsidian Vault 注册表机制与「Vault not found」处理

- **Query**: 在调用 `obsidian://open?path=X` 前判断路径 X 是否属于已注册 vault；不属于时的处理手段
- **Scope**: external（官方文档 + 社区论坛 + 开源实证） + 本机实证
- **Date**: 2026-07-08
- **抓取时间**: 2026-07-08

---

## TL;DR（关键结论）

1. **注册表文件**就是 `%APPDATA%\obsidian\obsidian.json`（Windows），结构为 `{ "vaults": { "<vaultID>": { "path", "ts", "open" } }, ... }`。`<vaultID>` 是 16 位小写 hex 随机串，同时是同目录下 `<vaultID>.json`（Electron 窗口缓存）的文件名。本机已实证。
2. **`obsidian://open?path=X` 的语义**：Obsidian 收到后会在已注册 vault 列表中搜索「最具体的包含 X 的 vault」，找不到任何包含 X 的 vault 时**必现** `Vault not found. Unable to find a vault for the URL ...`。这是官方文档明确的行为，非 bug。
3. **路径归属判断**：需在 Obsidian 之外（WorkBench 侧）复刻同样的「祖先匹配」逻辑——遍历 `vaults`，找 `vault.Path` 等于 X 或为 X 祖先且路径段最长者。Windows 必须大小写不敏感、按完整路径段匹配、先 `EvalSymlinks`。
4. **自动注册可行但有风险**：可直接向 `obsidian.json` 追加条目 + 创建 `<vaultID>.json`（obsidian-selenium 项目已实证）。**但 Obsidian 运行时持有内存缓存，会在 vault 切换/退出时回写磁盘，可能覆盖手动修改**；建议在 Obsidian 未运行时写入，或写入后用 `choose-vault` 触发重载。首次打开新注册 vault 仍会触发「Do you trust the author of this vault」信任提示，无法通过改 json 绕过。
5. **没有官方 URI action 能创建新 vault**。`open?vault=NAME` 在 NAME 不存在时不会创建；`choose-vault` 仅打开 vault 管理器。社区唯一「绕过」方案是 CodeScript Toolkit 插件的 `vault-open` IPC，依赖已开启的 vault，非通用解。

**给 WorkBench 的推荐策略**：打开前先读 `obsidian.json` 做归属判断；若不属于任何 vault，**不要**自动改写注册表（风险高且首次信任提示无法绕过），改为降级路径——例如提示用户「该目录未注册为 Obsidian vault，是否打开 Obsidian vault 管理器手动添加」(`obsidian://choose-vault`)，或回退到系统默认 `.md` 编辑器。

---

## 1. vault 注册表文件

### 1.1 文件位置

| 平台 | 路径 |
|---|---|
| **Windows** | `%APPDATA%\obsidian\obsidian.json`（即 `C:\Users\<user>\AppData\Roaming\obsidian\obsidian.json`） |
| macOS | `~/Library/Application Support/obsidian/obsidian.json` |
| Linux | `~/.config/obsidian/obsidian.json` |

同目录下还有：`<vaultID>.json`（每个 vault 一个窗口缓存）、`lockfile`（单例锁）、`obsidian.log`、Electron 运行时目录（Cache/IndexedDB/Local Storage 等）。

**来源**：论坛 topic 107025 post 4 明确写出 `%APPDATA%\obsidian\obsidian.json`；topic 54241 post 0 描述「AppData folder under 2 .json files: 主 obsidian.json + [vault_id].json」；本机 `C:\Users\liuyang\AppData\Roaming\obsidian\` 目录列举已实证三者俱在。

### 1.2 obsidian.json 结构（本机脱敏真实示例）

本机 `obsidian.json`（223 字节，2 个 vault）脱敏后：

```json
{
  "vaults": {
    "439a9f093c243976": {
      "path": "C:\\Users\\liuyang\\<...>\\Typora",
      "ts": 1775118231354,
      "open": true
    },
    "52bb7de88c00ed4f": {
      "path": "C:\\Users\\liuyang\\<...>\\workspace_claudcode",
      "ts": 1781263629766
    }
  },
  "updateDisabled": true
}
```

### 1.3 字段含义

| 字段 | 层级 | 类型 | 含义 |
|---|---|---|---|
| `vaults` | 顶层 | object | 已注册 vault 的映射；**key 就是 vault ID** |
| `<vaultID>` | `vaults` 的 key | string(16 hex) | 16 位小写十六进制随机串，如 `ef6ca3e3b524d22f`；同时是同目录 `<vaultID>.json` 的文件名 |
| `path` | vault 条目 | string | vault 根目录的**绝对路径**；Windows 下用反斜杠 `\`，JSON 中转义为 `\\` |
| `ts` | vault 条目 | number | 毫秒级 Unix 时间戳，表示注册/最后打开时间（如 `1775118231354`） |
| `open` | vault 条目 | boolean(可选) | 标记当前/最后打开的 vault；最多一个为 `true`，缺省视为 `false` |
| `updateDisabled` 等其他顶层字段 | 顶层 | any | Obsidian 全局设置（如禁用自动更新），与 vault 列表无直接关系 |

**vault ID 是否为随机哈希**：是。官方 Obsidian URI 文档脚注 [^1] 原文：「Vault ID is the random 16-character code assigned to the vault, for example `ef6ca3e3b524d22f`. This ID is unique per folder on your computer.」获取方式为「vault switcher → 右键 → Copy vault ID」。

**path 是否绝对**：是，全部为绝对路径。

### 1.4 `<vaultID>.json`（窗口缓存，自动注册时需一并创建）

本机 `439a9f093c243976.json` 真实内容：

```json
{"x":256,"y":56,"width":1024,"height":800,"isMaximized":true,"devTools":false,"zoom":0}
```

字段：`x`/`y`（窗口位置）、`width`/`height`（尺寸）、`isMaximized`（是否最大化）、`devTools`（是否开 DevTools）、`zoom`（缩放级别）。obsidian-selenium 与论坛 topic 63841 均确认此文件是 Electron 窗口状态缓存。

---

## 2. 路径归属判断

### 2.1 Obsidian 自身的匹配语义（官方）

Obsidian URI 文档对 `open` action 的 `path` 参数原文：

> `path` an absolute file system path to a file.
> - Using this parameter will override both `vault` and `file`.
> - **This will cause the app to search for the most specific vault which contains the specified file path.**
> - Then the rest of the path replaces the `file` parameter.

即「搜索包含该路径的**最具体**（嵌套最深）的 vault」。WorkBench 侧复刻此逻辑即可与 Obsidian 行为一致。

### 2.2 判断准则

判断绝对路径 X 是否「属于」已注册 vault P：

- **相等**：`P == X`（X 直接指向 vault 根）。
- **祖先**：`P` 是 `X` 的祖先目录，且必须是**完整路径段**匹配（`P` 后必须紧跟路径分隔符）。例：`P=C:\Vault`，`X=C:\Vault\note.md` → 属于；`X=C:\VaultChild\note.md` → **不**属于（前缀字符串相同但非完整段）。
- **「最具体」**：若多个 vault 都包含 X，取 `len(P)` 最大者。

### 2.3 Windows 专属注意点

| 问题 | 处理 |
|---|---|
| 大小写不敏感 | 盘符与路径比较均用大小写不敏感（`strings.EqualFold` 或 `ToLower`） |
| 分隔符差异 `\` vs `/` | 先 `filepath.Clean` 规范化，比较时统一 `filepath.ToSlash` 或都补 `os.PathSeparator` |
| 符号链接 | `filepath.EvalSymlinks` 先解析真实路径，否则前缀比较会失败 |
| 盘符大小写 | `C:` 与 `c:` 视作相同 |
| 末尾分隔符 | `filepath.Clean` 会去除末尾冗余分隔符，使 `C:\Vault\` 与 `C:\Vault` 一致 |
| `.obsidian` 子目录 | 存在 `.obsidian` 可作为「此目录是 vault」的辅助佐证，但**归属判断不依赖它**（一个目录可被注册为 vault 而暂无 `.obsidian`，或 `.obsidian` 被重命名） |

### 2.4 Go 实现要点（伪代码）

```go
package obsidian

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// VaultEntry obsidian.json 中单个 vault 条目
type VaultEntry struct {
	Path string `json:"path"`
	Ts   int64  `json:"ts"`
	Open bool   `json:"open,omitempty"`
}

// Config obsidian.json 顶层结构（仅声明需要的字段）
type Config struct {
	Vaults map[string]VaultEntry `json:"vaults"`
	// 其余顶层字段（updateDisabled 等）忽略
}

// ConfigPath Windows: %APPDATA%\obsidian\obsidian.json
func ConfigPath() string {
	appdata := os.Getenv("APPDATA") // Windows Roaming
	return filepath.Join(appdata, "obsidian", "obsidian.json")
}

// LoadVaults 读取并解析注册表
func LoadVaults() (map[string]VaultEntry, error) {
	b, err := os.ReadFile(ConfigPath())
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.Vaults == nil {
		return map[string]VaultEntry{}, nil
	}
	return cfg.Vaults, nil
}

// resolve 解析符号链接并 Clean，失败时回退到 Clean
func resolve(p string) string {
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(rp)
	}
	return filepath.Clean(p)
}

// isAncestorOrEqual 判断 parent 是否等于 child 或为 child 的祖先（完整路径段）
// Windows 大小写不敏感。比较前统一转 slash 规避 \ vs / 差异。
func isAncestorOrEqual(parent, child string) bool {
	ps := filepath.ToSlash(filepath.Clean(parent))
	cs := filepath.ToSlash(filepath.Clean(child))
	if strings.EqualFold(ps, cs) {
		return true
	}
	// child 必须以 "parent/" 开头（大小写不敏感），确保完整路径段
	return strings.HasPrefix(strings.ToLower(cs), strings.ToLower(ps)+"/")
}

// FindVaultForPath 复刻 Obsidian「最具体包含 vault」语义。
// 返回匹配的 vaultID、vaultPath；ok=false 表示无 vault 包含 X（将触发 Vault not found）。
func FindVaultForPath(vaults map[string]VaultEntry, absPath string) (vaultID, vaultPath string, ok bool) {
	target := resolve(absPath)
	bestLen := -1
	for id, v := range vaults {
		vp := resolve(v.Path)
		if !isAncestorOrEqual(vp, target) {
			continue
		}
		// 最具体：取路径最长者（嵌套最深）
		if len(vp) > bestLen {
			bestLen = len(vp)
			vaultID, vaultPath, ok = id, vp, true
		}
	}
	return
}

// IsRegistered 简化版：仅判断是否属于任何 vault
func IsRegistered(absPath string) (bool, error) {
	vaults, err := LoadVaults()
	if err != nil {
		return false, err
	}
	_, _, ok := FindVaultForPath(vaults, absPath)
	return ok, nil
}
```

**前缀比较的核心陷阱**：不能写成 `strings.HasPrefix(strings.ToLower(child), strings.ToLower(parent))`，否则 `C:\Vault` 会误匹配 `C:\VaultChild`。必须补分隔符（`parent + "/"`）并单独处理相等。

---

## 3. 自动注册 vault 的可行性与风险

### 3.1 可行性（已实证）

obsidian-selenium 项目的测试脚本 `test/run.sh` 给出了完整自动化流程（Linux 路径，Windows 同理）：

1. 读取 `obsidian.json`（不存在则建空 `{}`）；
2. 生成一个 vault ID（脚本用 `分支名-时间戳`，Obsidian 官方用 16 位 hex）；
3. 向 `vaults` 追加条目 `{ "path": "<绝对路径>", "ts": <毫秒时间戳> }`；
4. 创建同目录 `<vaultID>.json`，内容为窗口缓存 `{x,y,width,height,isMaximized,devTools,zoom}`；
5. 调用 `obsidian://open?path=<vault内某文件>`。

论坛 topic 54241 post 0 原文佐证：「you could add your own entry to the obsidian.json file, and create your own `[vault_id]`.json file, then use the obsidian uri to open the file」。

### 3.2 vault ID 生成规则

官方仅说「random 16-character code」，源码未开源。从实证样本（`ef6ca3e3b524d22f`、`439a9f093c243976`、`52bb7de88c00ed4f`）看为 16 位小写 hex（即 8 字节随机数）。Go 可用 `crypto/rand` 生成：

```go
func newVaultID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:]) // 16 位小写 hex
}
```

自生成的 ID 与 Obsidian 自身生成格式一致即可，无需向官方注册。

### 3.3 运行时覆盖风险（核心风险）

论坛 topic 63841 post 0（标题「Updating the configuration cache manually, Singleton files?」）原文：

> When I have a vault already open: The cache does not update itself so it will not allow me to open this brand new vault until I have either:
> - Closed an already open vault, which triggers a cache/state update
> - Opened an already created vault which has its path already cached, triggering a new cache/state update
> I believe what is interfering with this is these <singleton files>

含义：

- **Obsidian 运行时持有内存中的 vault 列表**，磁盘 `obsidian.json` 的修改不会被立即重读。
- Obsidian 在「关闭 vault」「打开已有 vault」「退出」等事件时**回写磁盘**，会覆盖手动修改（造成写入丢失）。
- 本机 `%APPDATA%\obsidian\lockfile`（单例锁）存在，佐证其 singleton 机制。

**结论**：自动注册应在 **Obsidian 未运行**时写入最安全。若 Obsidian 已运行，写入后需触发其重载（见 3.5）。

### 3.4 信任提示（无法绕过）

首次打开某 vault 时，Obsidian 弹「Do you trust the author of this vault?」，选项为「Trust author and enable plugins」或「Browse vault in Restricted Mode」。论坛 topic 76539 询问该状态存于何处，**未得到官方明确答复**（话题已自动关闭）；topic 45747 反映该提示可能异常重复出现。综合判断：信任状态很可能存于 Electron 的 Local Storage / IndexedDB（以 vault 路径为 key），而非简单 json 文件——故**无法通过改 `obsidian.json` 绕过**。自动注册的 vault 首次打开仍会触发此提示，需用户手动确认。

### 3.5 社区实践与反对意见

- **支持方**：obsidian-selenium（自动化测试）、topic 63841（程序化建 vault）证明可行。
- **反对方/风险提示**：topic 107025 post 5 明确「`obsidian://` URI scheme looks up information that your vault has indexed (as you found in appdata). So it only works while Obsidian knows about the vault and while the vault is at the known location」；topic 107025 post 4 指出可移动驱动器上的 vault 会被 Obsidian **主动从 `obsidian.json` 移除**，说明 Obsidian 会自行维护此文件，外部写入存在被清理风险。
- **官方态度**：无官方 API/URI 支持「创建新 vault」（见第 4 节），属未公开内部机制，升级版本后结构可能变更。

### 3.6 是否需要重启

是。若 Obsidian 未运行，写入后启动即可生效；若已运行，需关闭所有 vault 窗口触发状态回写后再写入，或重启 Obsidian。直接写入后立即调用 URI 不会生效（内存缓存未更新）。

---

## 4. 替代 URI action

官方文档列出全部 actions：`open`、`new`、`daily`、`unique`、`search`、`choose-vault`、`hook-get-address`。**逐一评估**：

| Action | 能否创建/打开未注册 vault | 说明 |
|---|---|---|
| `obsidian://open?vault=NAME` | 否 | `vault` 接受 vault 名或 ID；NAME 不存在时报错，不创建 |
| `obsidian://open?path=X` | 否 | X 不在任何已注册 vault 内时报 `Vault not found`（即本课题场景） |
| `obsidian://open?vault=ID` | 否 | 同上，ID 不存在则失败 |
| `obsidian://choose-vault` | 间接 | 仅打开 vault 管理器（让用户手动「Open folder as vault」），不直接注册 |
| `obsidian://new` / `daily` / `unique` / `search` | 否 | 均要求 `vault` 已注册 |
| `obsidian://hook-get-address` | 否 | Hook 集成用，依赖已聚焦 vault |

**结论**：无官方 action 可直接「打开或添加新 vault」。

**社区绕过方案**（topic 112323 post 1，mnaoumov 给出）：CodeScript Toolkit 插件提供 `obsidian://CodeScriptToolkit?vault=Foo&code=window.electron.ipcRenderer.sendSync('vault-open','C%3A%5Cpath%5Cto%5Cvault',false)`，通过 Electron IPC 调用 Obsidian 内部 `vault-open`。**限制**：需已有一个安装并启用该插件的 vault，非通用解，不适合 WorkBench 这种通用工具内置依赖。

**shorthand 格式**（官方）：
- `obsidian://vault/<vault名>/<文件>` ≡ `open?vault=...&file=...`
- `obsidian:///</绝对/路径>` ≡ `open?path=...`
- 均不改变「需已注册」的前提。

---

## 5. Vault not found 触发条件

### 5.1 确切触发条件

当 `obsidian://open?path=X`（或 shorthand `obsidian:///X`）中的 X **不被任何已注册 vault 包含**时，Obsidian 弹错误窗：

```
Vault not found
Unable to find a vault for the URL obsidian://open?path=...
```

复现案例（论坛 topic 73480 post 0）：`obsidian://open?path=W:/Labore/Smoker/Referenzmessungen/Druckkurven.md`，Obsidian v1.4.16 / Windows 10，`W:` 下目录未注册为 vault → 必现该错误。topic 112323 post 0 进一步明确：「if that vault has not been opened / cached before」，即「从未被 Obsidian 注册过」即触发。

### 5.2 与版本的关系

- 错误行为在 v1.0.0 ~ v1.4.16（topic 73480、107025）及至 2025-10（topic 107025 post 4）均稳定复现，属长期稳定行为，非回归。
- 官方文档当前版本仍描述同样的 `path` 搜索语义，未提供「自动注册」选项。
- 不排除未来版本提供「path 未命中时自动注册」的开关（topic 112323 即为相关 feature request，截至抓取日未实现）。

### 5.3 编码无关性

topic 73480 post 0 指出：无论 `/` → `%2F`、`:` → `%3A` 是否编码，只要路径不在已注册 vault 内，错误一致。即**触发条件与编码无关，仅与「是否被已注册 vault 包含」有关**。Windows 路径推荐编码：`obsidian://open?path=C%3A%5CUsers%5Cjimmyone%5Cmy%20vault%5Cexample%20note.md`（topic 107025 post 3）。

---

## 6. Go 实现伪代码（注册表读取 + 归属判断）

见第 2.4 节 `LoadVaults` / `FindVaultForPath` / `IsRegistered`。调用示例：

```go
// 打开前预检
func OpenInObsidian(absPath string) error {
    // 1. 协议是否注册（WorkBench 已有 isObsidianProtocolRegistered）
    if !isObsidianProtocolRegistered() {
        return errors.New("Obsidian 未安装或未注册 obsidian:// 协议")
    }
    // 2. 路径归属判断
    ok, err := IsRegistered(absPath)
    if err != nil {
        // 读不到 obsidian.json（可能未安装/未运行过）→ 降级
        return openViaChooseVaultOrFallback(absPath)
    }
    if !ok {
        // 不属于任何 vault：不自动改注册表，降级处理
        return promptUserToRegisterVault(absPath) // 见下
    }
    // 3. 属于某 vault：正常发起 URI
    return launchObsidianURI(absPath)
}

func promptUserToRegisterVault(absPath string) error {
    // 选项 A：打开 Obsidian vault 管理器，让用户手动添加
    //   exec.Command("cmd","/c","start","obsidian://choose-vault")
    // 选项 B：回退到系统默认 .md 编辑器
    // 选项 C（高风险，不推荐默认开启）：见 3.x 自动注册
}
```

**自动注册（可选/高风险，不建议默认）**：在 Obsidian 未运行时，向 `obsidian.json` 的 `vaults` 追加 `{newVaultID: {path, ts}}`，并创建 `<newVaultID>.json` 窗口缓存。注意首次打开仍会触发信任提示。

---

## 来源链接

| # | 来源 | URL | 关键点 | 抓取时间 |
|---|---|---|---|---|
| 1 | Obsidian URI 官方文档 | https://help.obsidian.md/Extending+Obsidian/Obsidian+URI | `path` 参数「搜索最具体包含 vault」语义；vault ID 定义；actions 列表 | 2026-07-08 |
| 2 | Configuration folder 官方文档 | https://help.obsidian.md/Files+and+folders/Configuration+folder | `.obsidian` 配置目录位置 | 2026-07-08 |
| 3 | 论坛 topic 73480 | https://forum.obsidian.md/t/73480 | `Vault not found` 复现（v1.4.16/Win10） | 2026-07-08 |
| 4 | 论坛 topic 107025 | https://forum.obsidian.md/t/107025 | `%APPDATA%\obsidian\obsidian.json` 确认；可移动驱动器 vault 被移除；URI 依赖 appdata 索引 | 2026-07-08 |
| 5 | 论坛 topic 54241 | https://forum.obsidian.md/t/54241 | obsidian.json + `[vault_id].json` 双文件结构；手动追加条目可行 | 2026-07-08 |
| 6 | 论坛 topic 63841 | https://forum.obsidian.md/t/63841 | 运行时内存缓存、singleton 文件、磁盘写入被覆盖风险 | 2026-07-08 |
| 7 | 论坛 topic 112323 | https://forum.obsidian.md/t/112323 | 「用 filepath 通过 URI 打开新 vault」需求；CodeScript Toolkit 绕过方案 | 2026-07-08 |
| 8 | 论坛 topic 53102 | https://forum.obsidian.md/t/53102 | 同名 vault 区分需 vault ID；ID 位于 `%appdata%/obsidian/obsidian.json` | 2026-07-08 |
| 9 | 论坛 topic 45747 | https://forum.obsidian.md/t/45747 | 「Do you trust the author」信任提示行为 | 2026-07-08 |
| 10 | 论坛 topic 76539 | https://forum.obsidian.md/t/76539 | 信任状态存储位置未明确（话题关闭） | 2026-07-08 |
| 11 | obsidian-selenium run.sh | https://raw.githubusercontent.com/smartguy1196/obsidian-selenium/f9b77e0ac63586532ca1b5b301ab7b57e1523f17/test/run.sh | 自动注册完整脚本实证（追加 vaults + 建 vault_id.json + 调 URI） | 2026-07-08 |
| 12 | 本机实证 | `C:\Users\liuyang\AppData\Roaming\obsidian\` | obsidian.json 真实结构、`<vaultID>.json` 窗口缓存、lockfile 单例锁 | 2026-07-08 |

**官方 vs 社区标注**：Obsidian 桌面端闭源，**官方未公开 `obsidian.json` 结构说明**。第 1.2/1.3 节结构基于「本机实证 + 多个社区话题 + 开源脚本」交叉验证，属**社区/实证**来源，非官方文档。第 2.1 节 `path` 语义为**官方文档**明文。第 5 节触发条件为**官方语义 + 社区复现**双重佐证。

---

## Caveats（未完全确定项）

1. **`open` 字段语义细节**：本机示例仅一个 vault 有 `"open": true`，推测为「最后打开」，但官方未公开；多窗口场景下是否允许多个 `true` 未验证。
2. **vault ID 生成算法**：官方仅说「random 16-character」，源码闭源；样本符合「8 字节随机 hex」，但未获官方确认是否为 `crypto.randomBytes(8).toString('hex')`。自生成同格式 ID 实证可用（obsidian-selenium 用任意字符串亦可用）。
3. **信任提示存储位置**：topic 76539 未解决；推测在 Electron Local Storage/IndexedDB，但未实证。无法确认能否通过文件操作绕过首次信任提示（目前看不能）。
4. **Obsidian 运行时回写时机**：topic 63841 描述为「关闭/打开 vault 时回写」，但完整回写触发事件清单（退出？窗口切换？定时？）未获官方确认。自动写入的最安全窗口仍是「Obsidian 完全未运行」。
5. **Obsidian 版本兼容**：`obsidian.json` 结构为内部机制，升级版本（尤其大版本）可能变更字段或位置。建议 WorkBench 对解析做防御性编码（未知字段忽略、结构不符时降级而非报错）。
6. **macOS/Linux 路径**：本课题聚焦 Windows，但 `obsidian.json` 结构跨平台一致（仅路径格式与 AppData 位置不同）；WorkBench 若跨平台需按平台取 `ConfigPath`。
7. **path 编码边界**：topic 73480 反映部分情况下 Windows 路径编码可能影响解析（用户报告编码后仍失败，但被指出是路径未注册所致）。建议 WorkBench 始终对 `path` 做 URL 编码（`url.QueryEscape` 或等效），并用正斜杠 shorthand `obsidian:///C:/...` 作为兼容备选。
