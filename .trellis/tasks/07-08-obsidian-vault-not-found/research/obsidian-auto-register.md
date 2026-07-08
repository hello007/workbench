# Research: Obsidian Vault 自动注册并打开的实操可行性

- **Query**: 在 WorkBench 中「自动把一个目录注册为 Obsidian vault 并打开」的实操可行性，作为方案 A 的增强按钮
- **Scope**: external（官方文档 + 社区论坛 + 开源实证） + 本机实证 + Go 实测
- **Date**: 2026-07-08
- **抓取时间**: 2026-07-08

---

## TL;DR

**结论：有条件可行。**

技术写入流程已被两个开源项目（`obsidian-selenium`、`heffrey/obsidian-hook`）实证可用，本机 Obsidian 配置结构也已验证；但**信任提示无法可靠绕过**（无公开方法、社区尝试失败、两个自动注册项目均未处理），且 **Obsidian 运行时写入存在覆盖风险**（需检测进程 + 引导重启）。因此可实施，但必须满足三个前提：①接受首次打开仍弹信任提示；②Obsidian 未运行时写入（或运行时写入后引导用户重启）；③原子写 + 备份。若用户期望「一键无感打开」则不可行--首次必有信任提示这一步无法消除。

---

## 1. 信任提示（"Do you trust the author of this vault"）能否绕过？

### 1.1 结论：无法可靠绕过

| 维度 | 证据 |
|---|---|
| 社区实证（失败） | 论坛 topic 76539 中 `zerkshop` 在 Linux 上做了**完全相同的尝试**--控制 `obsidian.json` 列出 vault 路径，但发现「this isn't enough to prevent the popup every time」。该话题被自动关闭，**未得到任何官方或社区的有效答案**。 |
| 信任状态存储位置不明 | 论坛 topic 45747（18 帖）讨论「信任提示重复出现」，资深用户 `rigmarole` 在 post#16 明确表示「I'm not sure where your troublesome setting is stored. It might also be stored somewhere in the **global app settings**」；`drich` 最终靠**手动删除整个 `AppData/obsidian` 目录**才彻底重置（post#18）。无人指出具体存储位置。 |
| 自动注册项目均未处理 | `heffrey/obsidian-hook`（2026 年活跃项目，auto-register repo as vault）和 `obsidian-selenium` 两个项目的源码与文档**均未提及信任提示**，也未写入任何 trust 相关字段。说明即便自动注册成功，首次打开仍会弹提示。 |
| 本机实证 | 本机两个**已信任** vault 的 `.obsidian/app.json` 中**均无** `trust`/`restricted`/`trusted` 字段（仅有 `readableLineLength`、`attachmentFolderPath` 等业务设置）。证明信任状态**不在 vault 的 `.obsidian/app.json`**。 |

### 1.2 信任状态可能存储在哪里

基于本机实证排查：

| 候选位置 | 实证情况 | 能否预写 |
|---|---|---|
| vault 的 `.obsidian/app.json` | 已排除（无 trust 字段） | - |
| `%APPDATA%\obsidian\obsidian.json` | 已排除（只有 path/ts/open） | - |
| `Local Storage/leveldb/` | 存在 per-vault 状态，key 形如 `<vaultID>-file-explorer-unfold`、`<vaultID>-recent-commands`（以 vault ID 为前缀）。**未发现明确的 trust key**（数据经 snappy 压缩，`strings`/`grep` 无法直接提取）。 | 理论上可能，但 leveldb 是二进制 KV 存储，外部进程预写需精确还原 LevelDB 物理格式（SSTable + MANIFEST + WAL），且 Obsidian 运行时持有文件锁（本机 `000048.log` 被锁定无法读取）。**极其脆弱，无实证。** |
| `IndexedDB/app_obsidian.md_0.indexeddb.leveldb/` | 存在大量 vault 数据，但搜索到的 "Trust" 字样均为聊天记录内容（如 "Trust the Model"），非配置。 | 同上，不可行。 |

**结论**：信任状态很可能以 vault ID 或 vault 路径为 key 存于 Electron Local Storage（leveldb），但被二进制压缩且 Obsidian 运行时锁文件，**无法通过简单文件预写可靠绕过**。即便理论上可能，也属「逆向工程内部存储格式」，跨版本必然失效。

### 1.3 对「自动注册并打开」体验的影响

自动注册成功后，用户首次打开该 vault 时，Obsidian **必然**弹出：

> Do you trust the author of this vault?
> - Trust author and enable plugins
> - Browse vault in Restricted Mode

用户必须手动点选。这意味着「自动注册并打开」无法做到完全无感--点击按钮后，Obsidian 打开、弹信任提示、用户确认、方可编辑。**UI 必须提前告知用户这一步**，否则用户会以为自动注册失败。

---

## 2. 如何检测 Obsidian 是否正在运行（规避运行时覆盖）

### 2.1 推荐方法：`tasklist` 进程枚举（已 Go 实测可行）

本机实测（Obsidian 正在运行，4 个进程）：

```go
// 检测 Obsidian 进程是否运行。本机实测：Obsidian 运行时返回 4 个进程。
func isObsidianRunning() bool {
    out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
    if err != nil {
        return false // 降级：检测失败视为未运行（或保守视为运行中，按策略定）
    }
    return strings.Contains(strings.ToLower(string(out)), "obsidian.exe")
}
```

实测输出：`Obsidian.exe 进程数: 4, running: true`。

**为何用 `tasklist /FO CSV /NH` 而非 `tasklist /FI`**：`/FI "IMAGENAME eq Obsidian.exe"` 在 `cmd /c` 嵌套调用时参数解析易出错（本机实测 `exit status 1`）；`/FO CSV /NH` 全量输出再过滤更可靠，且 WorkBench 现有代码（`OpenWithDefaultApp` 等）已大量使用 `exec.Command("cmd","/c",...)` 模式。

### 2.2 lockfile 能否判断运行状态

| 属性 | 本机实证 |
|---|---|
| 路径 | `%APPDATA%\obsidian\lockfile` |
| 大小 | 0 字节（空文件） |
| 创建时间 | 2026-07-07 11:07:25.229 |
| Obsidian 进程启动时间 | 2026-07-07 11:07:24.990 ~ 11:07:25.556 |

**lockfile 创建时间与进程启动时间高度吻合**，证实它是 Obsidian 启动时创建的单例锁。但：

- **不可靠**：若 Obsidian 崩溃/被强杀，lockfile 可能残留（Electron `requestSingleInstanceLock` 的已知行为）。此时 lockfile 存在但 Obsidian 未运行，会误判。
- **0 字节**：无法从中读取 PID 做二次校验。
- **结论**：**不推荐用 lockfile 判断运行状态**。`tasklist` 进程枚举是唯一可靠方法。

### 2.3 运行时写入是否必然被覆盖

基于论坛 topic 63841（`leafstrat` 原帖）描述：

> When I have a vault already open: The cache does not update itself so it will not allow me to open this brand new vault until I have either:
> - Closed an already open vault, which triggers a cache/state update
> - Opened an already created vault which has its path already cached, triggering a new cache/state update

含义：
- Obsidian 运行时持有**内存中的 vault 列表**，磁盘修改不会被立即重读。
- Obsidian 在「关闭 vault」「打开已有 vault」「退出」等事件时**回写磁盘**，会覆盖手动修改（写入丢失）。
- **运行时写入不必然立即丢失**，但存在「Obsidian 回写时覆盖」的竞态窗口。`heffrey/obsidian-hook` 声称 "works whether or not Obsidian is running"，但其场景是「写完不立即打开，等用户下次重启 Obsidian」，并未规避运行时回写覆盖。

### 2.4 能否触发运行中 Obsidian 重载 vault 列表

| 机制 | 是否有效 |
|---|---|
| `obsidian://choose-vault` | 仅打开 vault 管理器 UI，**不重读 `obsidian.json`**（内存缓存未变） |
| `obsidian://open?path=X` | 若 X 在内存缓存中不存在，仍报 Vault not found |
| 信号/IPC | 无公开机制。CodeScript Toolkit 插件的 `vault-open` IPC 依赖已开启的 vault，非通用解 |
| 重启 Obsidian | **唯一可靠方式**。关闭所有窗口→回写磁盘→此时若已写入会被覆盖；故正确顺序是：先确认 Obsidian 完全退出→再写入→再启动 |

**结论**：运行中 Obsidian 无法被外部触发重读 vault 列表。最安全策略是「Obsidian 未运行时写入」。

---

## 3. 完整注册流程的实操细节

### 3.1 vault ID 生成（已实证与官方一致）

`heffrey/obsidian-hook` 源码使用 `randomBytes(8).toString('hex')` 生成 16 位小写 hex，与本机样本（`439a9f093c243976`、`52bb7de88c00ed4f`）及官方文档示例（`ef6ca3e3b524d22f`）格式完全一致。Go 等价实现：

```go
func newVaultID(existing map[string]VaultEntry) string {
    var b [8]byte
    for {
        _, _ = rand.Read(b[:])
        id := hex.EncodeToString(b[:]) // 16 位小写 hex
        if _, dup := existing[id]; !dup {
            return id
        }
    }
}
```

**冲突检测必须做**：`heffrey` 用 `while (cfg.vaults[id])` 循环避免与现有 ID 冲突。虽 8 字节随机碰撞概率极低，但成本极小，应做。

### 3.2 是否必须创建 `<vaultID>.json` 窗口缓存文件

**不必须**。两个项目策略不同：

| 项目 | 是否创建 `<vaultID>.json` | 结果 |
|---|---|---|
| `obsidian-selenium` | 创建（`{x,y,width,height,isMaximized,devTools,zoom}`） | 可用 |
| `heffrey/obsidian-hook` | **不创建** | 可用（README 称正常工作） |

`heffrey` 不创建窗口缓存也能工作，说明 **Obsidian 缺失该文件时会用默认值或自动创建**。本机两个窗口缓存文件内容完全相同（`{"x":256,"y":56,"width":1024,"height":800,"isMaximized":true,"devTools":false,"zoom":0}`），也佐证 Obsidian 有默认值机制。

**建议**：不创建。理由：①减少写入面；②避免与 Obsidian 自生窗口状态冲突；③Obsidian 会自动补建。若要创建，字段默认值取：`{"x":0,"y":0,"width":1024,"height":768,"isMaximized":true,"devTools":false,"zoom":0}`。

### 3.3 写入后是否需重启 Obsidian 才识别

| Obsidian 状态 | 写入后直接发 `obsidian://open?path=X` | 说明 |
|---|---|---|
| **未运行** | **可成功**（逻辑推断，建议实测） | URI 触发系统启动 Obsidian → Obsidian 启动时读取 `obsidian.json` → 新 vault 已在列表 → 打开 X。`heffrey` README 要求"restart Obsidian"是针对「Obsidian 已运行」场景。 |
| **运行中** | 失败（Vault not found） | 内存缓存未更新。需关闭 Obsidian→写入→重启。 |

**关键实操建议**：若检测到 Obsidian 未运行，写入后直接发 URI 应能成功（无需先启动再发）。但此路径**未经本机实测**（本机 Obsidian 始终运行），建议 WorkBench 实施时实测验证：关闭 Obsidian→运行自动注册→发 URI→观察是否直接打开。

### 3.4 是否需更新已有 vault 的 `open` 字段

**不建议更新**。`open: true` 标记「最后打开的 vault」，最多一个。自动注册时若把新 vault 设为 `open: true`，需把其他 vault 改为 `false`，这是对用户记忆状态的侵入。`heffrey` 不设 `open` 字段（只写 `{path, ts}`），让 Obsidian 在用户实际打开时自行维护。建议沿用此策略。

### 3.5 是否创建 `.obsidian` 子目录

`heffrey` 创建 `<vault>/.obsidian/app.json`，内容为 `{ "userIgnoreFilters": ["node_modules/", ".git/", ...] }`。其文档说明这是为了让 vault "tidy"（避免源码目录污染文件树），**不是为了绕过信任提示**。

| 目标目录状态 | 对注册/打开的影响 |
|---|---|
| 已有 `.obsidian`（已是 vault） | 注册仅追加 obsidian.json 条目；打开时 Obsidian 识别为已有 vault，**可能跳过信任提示**（因已初始化过）。但本机已信任 vault 的 app.json 无 trust 字段，故「.obsidian 存在=已信任」仅为推测，不保证。 |
| 无 `.obsidian`（新目录） | 注册追加条目；打开时 Obsidian 首次进入，**必弹信任提示**并创建 `.obsidian`。 |

**建议**：不主动创建 `.obsidian/app.json`。理由：①WorkBench 不应侵入目标目录（用户可能不希望被注册目录出现 `.obsidian`）；②无法可靠绕过信任提示，创建 `.obsidian` 意义有限；③若用户后续在 Obsidian 内手动配置，可能冲突。`heffrey` 创建它是因其场景是「把 git 仓库当 vault」，需要 ignore 源码目录；WorkBench 通用场景无此刚需。

---

## 4. 失败回滚与并发安全

### 4.1 备份策略

写入前先备份 `obsidian.json` 到同目录 `.bak`（带时间戳）：

```go
func backupConfig(cfgPath string) (string, error) {
    b, err := os.ReadFile(cfgPath)
    if err != nil { return "", err }
    bak := cfgPath + ".bak." + strconv.FormatInt(time.Now().Unix(), 10)
    return bak, os.WriteFile(bak, b, 0644)
}
```

### 4.2 原子写（防 Obsidian 并发回写导致损坏）

`heffrey` 源码的原子写模式（Go 等价）：

```go
func atomicWriteConfig(cfgPath string, cfg *obsidianConfig) error {
    tmp := cfgPath + ".tmp." + strconv.Itoa(os.Getpid())
    data, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil { return err }
    if err := os.WriteFile(tmp, data, 0644); err != nil { return err }
    return os.Rename(tmp, cfgPath) // Windows 上 rename 会原子替换
}
```

**关键**：`os.Rename` 在 Windows 上对同分区文件是原子替换（覆盖目标）。这样即便 Obsidian 在写入过程中启动并回写，`obsidian.json` 也只会是「旧版」或「新版」之一，**不会损坏**（无半写状态）。但「最后写者胜」仍可能导致丢失--若 Obsidian 回写晚于我们的 rename，我们的写入会丢；反之 Obsidian 的回写会丢。故仍需配合「Obsidian 未运行时写入」。

### 4.3 失败回滚

| 失败点 | 回滚动作 |
|---|---|
| 读 obsidian.json 失败 | 中止，不写入，提示用户（可能 Obsidian 未安装/未运行过） |
| 备份失败 | 中止，不写入 |
| 临时文件写入失败 | 删除临时文件，中止 |
| rename 失败 | 删除临时文件，提示用户（可能权限不足/Obsidian 锁定） |
| rename 成功但后续发 URI 失败 | 不回滚（vault 已注册是有效状态，用户可手动打开） |

**不建议「发 URI 失败就回滚 obsidian.json」**：注册本身是有效操作，URI 失败可能只是协议问题，回滚反而丢失已完成的注册。

### 4.4 目标目录 `.obsidian` 存在性对注册/打开的影响

见 3.5 节。注册流程本身不依赖目标目录是否有 `.obsidian`（obsidian.json 追加条目即可）；打开时若已有 `.obsidian` 可能减少首次提示概率但非保证。

---

## 5. 综合结论与推荐实现方案

### 5.1 三档判断

| 档位 | 是否达到 | 说明 |
|---|---|---|
| 完全可行 | 否 | 信任提示无法绕过，做不到「一键无感」 |
| **有条件可行** | **是** | 满足：①接受首次信任提示；②Obsidian 未运行时写入或引导重启；③原子写+备份 |
| 不可行 | 否 | 无不可克服的关键障碍 |

### 5.2 推荐实现方案

**前置条件检查**：

1. 目标路径不属于任何已注册 vault（沿用方案 A 的 `findVaultForPath`）。
2. Obsidian 已安装（`isObsidianProtocolRegistered` 或配置了 exe）。
3. **检测 Obsidian 进程是否运行**（`tasklist`）。

**分流策略**：

| Obsidian 状态 | 行为 |
|---|---|
| 运行中 | **不直接写入**。弹确认框：「Obsidian 正在运行，自动注册需要先关闭 Obsidian。是否关闭并继续？」用户确认→提示手动关闭（或 WorkBench 调 `taskkill`，但杀进程风险高，建议让用户手动关闭）→等待进程退出→写入→发 URI。 |
| 未运行 | 备份→原子写 obsidian.json（追加 `{newID: {path, ts}}`，不建窗口缓存、不改 open）→发 `obsidian://open?path=X`→Obsidian 启动并读取新 vault→打开。 |

**Go 伪代码（完整流程）**：

```go
package service

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "time"
)

// AutoRegisterAndOpen 自动将 vaultPath 注册为 Obsidian vault 并打开。
// 返回哨兵错误区分状态：
//   - ErrObsidianRunning: Obsidian 运行中，需用户关闭后重试
//   - ErrObsidianNotInstalled: 未检测到 Obsidian
//   - 其他: 写入/打开失败
func (s *FileOperationService) AutoRegisterAndOpen(vaultPath, obsidianPath string) error {
    // 1. 路径检查
    if _, err := os.Stat(vaultPath); err != nil {
        return fmt.Errorf("路径不存在: %w", err)
    }

    // 2. Obsidian 可用性检查（复用方案 A 逻辑）
    useExe := false
    if strings.TrimSpace(obsidianPath) != "" {
        if _, err := os.Stat(obsidianPath); err == nil {
            useExe = true
        }
    }
    if !useExe && !isObsidianProtocolRegistered() {
        return ErrObsidianNotInstalled
    }

    // 3. 检测 Obsidian 进程（核心：规避运行时覆盖）
    if isObsidianRunning() {
        return ErrObsidianRunning // 前端引导用户关闭后重试
    }

    // 4. 读取现有 obsidian.json
    cfgPath := obsidianConfigPath()
    cfg, err := loadFullConfig(cfgPath) // 读完整结构，保留 updateDisabled 等
    if err != nil {
        return fmt.Errorf("读取 Obsidian 配置失败: %w", err)
    }

    // 5. 去重检查（按 realpath，复用 resolvePath）
    resolvedTarget := resolvePath(vaultPath)
    for _, v := range cfg.Vaults {
        if resolvePath(v.Path) == resolvedTarget {
            // 已注册，直接打开
            uri := "obsidian://open?path=" + encodeObsidianPath(vaultPath)
            return launchObsidianURI(uri, obsidianPath, useExe)
        }
    }

    // 6. 备份
    if _, err := backupConfig(cfgPath); err != nil {
        return fmt.Errorf("备份配置失败: %w", err)
    }

    // 7. 追加新 vault 条目
    id := newVaultID(cfg.Vaults)
    cfg.Vaults[id] = VaultEntry{
        Path: vaultPath,
        Ts:   time.Now().UnixMilli(),
        // 不设 Open，不创建 <id>.json 窗口缓存
    }

    // 8. 原子写
    if err := atomicWriteConfig(cfgPath, cfg); err != nil {
        return fmt.Errorf("写入配置失败: %w", err)
    }

    // 9. 发 URI 打开（Obsidian 未运行，启动时读取新 vault）
    uri := "obsidian://open?path=" + encodeObsidianPath(vaultPath)
    return launchObsidianURI(uri, obsidianPath, useExe)
}

// isObsidianRunning 检测 Obsidian 进程是否运行（tasklist 枚举，已实测）。
func isObsidianRunning() bool {
    out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
    if err != nil {
        return false
    }
    return strings.Contains(strings.ToLower(string(out)), "obsidian.exe")
}

// newVaultID 生成 16 位 hex vault ID，避免与现有 ID 冲突。
func newVaultID(existing map[string]VaultEntry) string {
    var b [8]byte
    for {
        _, _ = rand.Read(b[:])
        id := hex.EncodeToString(b[:])
        if _, dup := existing[id]; !dup {
            return id
        }
    }
}

// loadFullConfig 读取完整 obsidian.json（保留未知顶层字段，避免回写时丢失 updateDisabled 等）。
type obsidianConfig struct {
    Vaults map[string]VaultEntry `json:"vaults"`
    // 其余顶层字段用 RawMessage 保留，或直接用 map[string]interface{}
}

func loadFullConfig(path string) (*obsidianConfig, error) {
    b, err := os.ReadFile(path)
    if err != nil { return nil, err }
    // 建议用 map[string]json.RawMessage 保留未知字段，这里简化
    var cfg obsidianConfig
    if err := json.Unmarshal(b, &cfg); err != nil { return nil, err }
    if cfg.Vaults == nil { cfg.Vaults = map[string]VaultEntry{} }
    return &cfg, nil
}

func backupConfig(cfgPath string) (string, error) {
    b, err := os.ReadFile(cfgPath)
    if err != nil { return "", err }
    bak := cfgPath + ".bak." + strconv.FormatInt(time.Now().Unix(), 10)
    return bak, os.WriteFile(bak, b, 0644)
}

func atomicWriteConfig(cfgPath string, cfg *obsidianConfig) error {
    tmp := cfgPath + ".tmp." + strconv.Itoa(os.Getpid())
    data, err := json.MarshalIndent(cfg, "", "  ")
    if err != nil { return err }
    if err := os.WriteFile(tmp, data, 0644); err != nil { return err }
    return os.Rename(tmp, cfgPath)
}
```

### 5.3 风险提示文案（UI 建议措辞）

**按钮文案**：「自动注册并打开」

**点击后确认框**（未运行场景）：

> 即将把该目录注册为 Obsidian vault 并打开。
> - 首次打开时 Obsidian 会弹出「Do you trust the author of this vault?」信任提示，请选择「Trust author and enable plugins」以正常使用。
> - 会修改 Obsidian 配置文件（已自动备份）。
> 是否继续？

**Obsidian 运行中场景**：

> 检测到 Obsidian 正在运行。自动注册需要先关闭 Obsidian（运行时写入可能被覆盖丢失）。
> 请手动关闭所有 Obsidian 窗口后，再次点击「自动注册并打开」。
> [我已关闭 Obsidian，重试]

**注册成功后轻提示**：

> 已注册为 Obsidian vault。首次打开会弹出信任提示，请点击「Trust author」。

### 5.4 UI 交互建议

1. **按钮位置**：在方案 A 的「未注册」确认框中，除「打开仓库管理器」外，新增「自动注册并打开」选项（作为高级/增强按钮，非默认）。
2. **风险标识**：按钮旁加 tooltip「会修改 Obsidian 配置，首次打开需手动确认信任」。
3. **不可逆说明**：注册后 obsidian.json 已变更（虽有备份），告知用户可在 Obsidian vault 管理器中手动移除。
4. **Obsidian 运行中禁用**：检测到运行时，按钮置灰或点击后引导关闭，避免写入被覆盖。
5. **信任提示预期管理**：必须在点击前告知用户「首次会弹信任提示」，否则用户会误判为失败。

### 5.5 不建议纳入的特性

| 特性 | 不建议原因 |
|---|---|
| 创建 `.obsidian/app.json` | 侵入目标目录；无法可靠绕过信任提示；WorkBench 通用场景无 ignore 源码需求 |
| 创建 `<vaultID>.json` 窗口缓存 | 不必要（Obsidian 自动补建）；增加写入面 |
| 设置 `open: true` | 侵入用户最后打开状态 |
| `taskkill` 强杀 Obsidian | 数据丢失风险高，让用户手动关闭 |
| 预写 Local Storage/leveldb 绕过信任 | 二进制格式脆弱，跨版本失效，无实证 |

---

## 来源链接

| # | 来源 | URL | 关键点 | 类型 | 抓取时间 |
|---|---|---|---|---|---|
| 1 | heffrey/obsidian-hook（核心实证项目） | https://github.com/heffrey/obsidian-hook | 自动注册 vault 的 Claude Code hook；源码 `hooks/obsidian-vault.mjs` 用 `randomBytes(8).toString('hex')` 生成 ID、原子写 `tmp+rename`、不创建窗口缓存、不检测进程、不处理信任提示、创建 `.obsidian/app.json`；README 明确「restart Obsidian so it picks up newly registered vaults」 | 社区/实证 | 2026-07-08 |
| 2 | obsidian-hook README | https://raw.githubusercontent.com/heffrey/obsidian-hook/main/README.md | "works whether or not Obsidian is running"、idempotent by realpath、需重启 Obsidian | 社区/实证 | 2026-07-08 |
| 3 | obsidian-hook how-it-works | https://raw.githubusercontent.com/heffrey/obsidian-hook/main/docs/how-it-works.md | 原子写细节、dedupe 机制、failure philosophy（never blocks） | 社区/实证 | 2026-07-08 |
| 4 | obsidian-selenium run.sh | https://raw.githubusercontent.com/smartguy1196/obsidian-selenium/f9b77e0ac63586532ca1b5b301ab7b57e1523f17/test/run.sh | 用 node 解析 obsidian.json 追加 vaults、创建 `<id>.json` 窗口缓存、`xdg-open` 发 URI；vault ID 用 `分支名-时间戳`（非 hex 也可用） | 社区/实证 | 2026-07-08 |
| 5 | 论坛 topic 76539 | https://forum.obsidian.md/t/76539 | `zerkshop` 控制 obsidian.json **无法阻止信任提示**；话题关闭无答案 | 社区/实证 | 2026-07-08 |
| 6 | 论坛 topic 45747 | https://forum.obsidian.md/t/45747 | 18 帖讨论信任提示重复；`rigmarole` post#16 称信任状态存储位置不明，「maybe global app settings」；`drich` post#18 靠删除整个 AppData/obsidian 重置 | 社区/实证 | 2026-07-08 |
| 7 | 论坛 topic 63841 | https://forum.obsidian.md/t/63841 | `leafstrat` 描述运行时内存缓存、singleton 文件、磁盘写入被覆盖；程序化建 vault 的覆盖风险 | 社区/实证 | 2026-07-08 |
| 8 | 论坛 topic 54241 | https://forum.obsidian.md/t/54241 | `smartguy1196` 确认 obsidian.json + `<vault_id>.json` 双文件结构 | 社区/实证 | 2026-07-08 |
| 9 | Obsidian URI 官方文档 | https://help.obsidian.md/Extending+Obsidian/Obsidian+URI | `path` 参数「搜索最具体包含 vault」语义；vault ID 为 16 位随机 hex；actions 列表无「创建 vault」 | 官方 | 2026-07-08 |
| 10 | 本机实证：obsidian.json | `C:\Users\liuyang\AppData\Roaming\obsidian\obsidian.json` | 真实结构：`{vaults:{id:{path,ts,open?}}, updateDisabled}`；2 个 vault，ID 为 16 位 hex | 本机实证 | 2026-07-08 |
| 11 | 本机实证：lockfile | `C:\Users\liuyang\AppData\Roaming\obsidian\lockfile` | 0 字节；创建时间 11:07:25.229 与 Obsidian 进程启动 11:07:24.990~25.556 吻合；崩溃残留风险 | 本机实证 | 2026-07-08 |
| 12 | 本机实证：Local Storage/leveldb | `C:\Users\liuyang\AppData\Roaming\obsidian\Local Storage\leveldb\` | per-vault key 以 vault ID 为前缀（`439a9f093c243976-file-explorer-unfold`）；未发现明确 trust key（snappy 压缩）；运行时 `.log` 被锁 | 本机实证 | 2026-07-08 |
| 13 | 本机实证：已信任 vault 的 app.json | `D:\工作\Typora\.obsidian\app.json` 等 | 两个已信任 vault 的 app.json **均无** trust/restricted 字段，证明信任状态不在 vault 配置 | 本机实证 | 2026-07-08 |
| 14 | Go 进程检测实测 | 本机 `tasklist /FO CSV /NH` | 检测到 4 个 Obsidian.exe 进程；`tasklist /FI` 在 cmd 嵌套下 exit status 1 不可靠 | 本机实证 | 2026-07-08 |

**官方 vs 社区标注**：
- **官方**：仅第 9 项（Obsidian URI 文档）明确 `path` 搜索语义与 vault ID 格式。Obsidian 桌面端闭源，**官方未公开 `obsidian.json` 结构说明、未公开信任提示存储位置、未提供创建 vault 的 API/URI**。
- **社区/实证**：第 1-8 项为开源项目与论坛讨论。第 10-14 项为本机直接观测。所有「自动注册」流程细节均属社区/实证，非官方支持，升级版本后结构可能变更。

---

## Caveats（未完全确定项）

1. **信任提示存储位置未最终定位**：本机排查排除 vault 的 `.obsidian/app.json` 与 `obsidian.json`；Local Storage/leveldb 有 per-vault key 但 trust key 未明确发现（snappy 压缩 + 运行时文件锁阻碍深入解析）。推测在 Electron Local Storage，但无法确认能否通过文件操作绕过（目前所有证据指向「不能」）。建议实施时**默认不能绕过**，UI 预告信任提示。
2. **「Obsidian 未运行时写入后直接发 URI 能否成功」未经本机实测**：本机 Obsidian 始终运行，无法在不影响用户使用的情况下关闭验证。逻辑推断可行（URI 启动 Obsidian→读 obsidian.json→新 vault 可见），但 WorkBench 实施时需实测：关闭 Obsidian→自动注册→发 URI→观察是否直接打开新 vault 而非报 Vault not found。
3. **`.obsidian` 子目录存在能否跳过信任提示**：本机已信任 vault 均有 `.obsidian`，但无法区分「因 .obsidian 存在而跳过」与「因 Local Storage 有信任记录而跳过」。`heffrey` 创建 `.obsidian/app.json` 但其文档未声称能跳过信任提示。建议**不依赖此机制**。
4. **Obsidian 运行时回写的完整触发事件清单未获官方确认**：topic 63841 描述为「关闭/打开 vault 时回写」，但退出、窗口切换、定时等场景未完全验证。最安全窗口仍是「Obsidian 完全未运行」。
5. **`os.Rename` 在 Windows 上的原子性边界**：同分区原子替换可靠；跨分区（如临时文件写在 C 盘、目标在 D 盘）则非原子。建议临时文件与 `obsidian.json` 同目录（本伪代码已如此），保证同分区。
6. **Obsidian 版本兼容**：本研究基于 Obsidian v1.12.7（本机日志显示 2026-04-26 检测到 1.12.7 为最新且已更新）。`obsidian.json` 结构为内部机制，大版本升级可能变更。建议 WorkBench 防御性解析（未知字段用 `json.RawMessage` 保留、结构异常时降级而非报错）。
7. **macOS/Linux 适配**：本课题聚焦 Windows。`heffrey` 脚本跨平台计算 `obsidian.json` 路径（macOS `~/Library/Application Support/obsidian/`、Linux `$XDG_CONFIG_HOME/obsidian/`），但 WorkBench 现有 Obsidian 功能仅 Windows（`obsidian_other.go` 空实现），自动注册若跨平台需同步适配进程检测（macOS 用 `pgrep`/`ps`、Linux 同理）。
8. **`tasklist` 依赖**：`tasklist` 是 Windows 内置命令（所有桌面版 Windows 均有），但 Server Core 等精简环境可能缺失。WorkBench 桌面场景可接受此依赖；检测失败时降级为「保守视为运行中，引导用户关闭」更安全。
