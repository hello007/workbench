# Research: go-git v5 → v6 升级 breaking changes 清单

- **Query**: go-git v5 → v6 升级的 breaking changes，逐条核验本仓消费面锚点（module path / PlainOpen / Head/Remote/Remotes / References 迭代器 / plumbing.Reference 全家 / Go 版本 / 依赖树 / 发布状态 / 官方迁移文档）
- **Scope**: external（GitHub go-git/go-git 仓库 + Go module proxy）+ 本仓 go.mod 基线对照
- **Date**: 2026-10-21
- **本仓基线**: `github.com/go-git/go-git/v5 v5.19.2`（go.mod:10），go 1.26.0 / toolchain go1.26.10（go.mod:3-5）
- **核验方法**: ① Go module proxy（goproxy.cn）拉取 `v6.0.0-beta.1` 模块 zip 与 go.mod 全文；② 本地解包后逐文件核对 v6 源码签名（含 file:line）；③ 与本地模块缓存中的 v5.19.2 源码逐点对照；④ GitHub API 拉取 releases 全列表、v6.0.0-alpha.1 / beta.1 release notes、issue #910（官方 breaking 追踪）与 #1395（版本推进 issue）正文。除明确标注「推断」外均为源码/官方仓库核实事实。

## 关键结论（前置）

| # | 锚点 | 结论 | 对本仓影响 |
|---|---|---|---|
| 1 | module path | v6 模块路径为 `github.com/go-git/go-git/v6`，**仅** `/v5` → `/v6` 一处路径变化 | 机械替换，必然项 |
| 2 | `PlainOpen` / `PlainOpenWithOptions` | **签名与 `PlainOpenOptions` 字段逐字节一致**；行为变化：底层 osfs 启用 BoundOS 路径加固 | 本仓只读打开合法仓库路径，实质影响趋零 |
| 3 | `Head()` / `Remote()` / `Remotes()` | 三者签名与返回类型**不变**；`Remotes()` 实现 v5/v6 逐行相同 | 零适配 |
| 4 | `References()` 迭代器 | `storer.ReferenceIter` 接口（Next/ForEach/Close）**不变** | 零适配 |
| 5 | `plumbing.Reference` 全家 | `Reference`/`Name()`/`Hash()`/`Type()`/`HashReference`/`ReferenceName`(Short/IsBranch/String) **全部不变**；**唯一实质 breaking：`plumbing.Hash` 从 `[20]byte` 数组改为 `ObjectID` 结构体别名**（SHA256 支持），`String()` 仍可用 | 本仓仅消费 `head.Hash().String()`，兼容 |
| 6 | Go 最低版本 | v6 go.mod `go 1.26.0`（官方政策「最后 2 个 stable 大版本」）；本仓恰好达标，**零余量** | 兼容；v6 未来 bump go directive 时本仓 toolchain 需跟进 |
| 7 | 依赖树 | go-billy v5→v6（同为 beta）、gcfg v1→gcfg/v2、go-crypto 1.4.1→1.5.2、sha1cd 0.6→0.7，并新增 socks5/ssh 服务端等依赖（传输层重构引入） | 依赖链整体换代，间接依赖面扩大 |
| 8 | **发布状态** | **v6 正式版未发布**。最新为 `v6.0.0-beta.1`（2026-10-04，官方标记 prerelease）；此前 alpha.1~alpha.5（2026-04-01 起）。v5 线最新 v5.19.3（2026-10-04） | 「升 v6」= 依赖 prerelease，直接决定路线可行性 |
| 9 | 官方迁移文档 | **无官方 migration guide、无 CHANGELOG.md**；权威依据是 breaking 追踪 issue #910（仍 open）+ 各 tag 的 release notes | 以 #910 + release notes 为准 |

**对本仓最重要的一条**（消费面盘点文档 §4.2 之外的增量发现）：v6 要求**打开的 Repository 须调用 `.Close()`**（`Repository.Close()` 为 v6 新增方法，v5 无此方法；官方 #910 明文列为潜在 breaking）。本仓 `HasRemotesBatch`（service/git.go:575）批量并发打开仓库且现状不关闭，升级 v6 后属真实适配点。

---

## 一、发布状态与版本事实（锚点 8）

来源：GitHub Releases API（`repos/go-git/go-git/releases`，取最近 10 条）与 goproxy.cn 版本列表，两源一致。

| tag | 发布日期（UTC） | prerelease | 备注 |
|---|---|---|---|
| v6.0.0-beta.1 | 2026-10-04 | **是** | v6 当前最新，仍为 prerelease |
| v5.19.3 | 2026-10-04 | 否 | v5 线最新补丁（本仓在 v5.19.2，落后一版） |
| v6.0.0-alpha.5 | 2026-07-29 | 是 | 与 v5.19.2 同日发布 |
| v6.0.0-alpha.4 | 2026-05-18 | 是 | |
| v6.0.0-alpha.3 | 2026-05-06 | 是 | |
| v6.0.0-alpha.2 | 2026-04-16 | 是 | |
| v6.0.0-alpha.1 | 2026-04-01 | 是 | v6 首个 alpha |
| v5.19.0 | 2026-05-06 | 否 | |

事实：**v6.0.0 正式版截至 2026-10-21 未发布**，v6 线处于 alpha（约 3 个月，5 个 alpha）→ beta.1 阶段；节奏上 alpha.5→beta.1 用时约 2 个月。

v6.0.0-alpha.1 release notes 的官方变更总览（用户面）：

- Git 兼容性：multi-ack、partial clone（filter）、sideband、shallow clone、大仓库 fetch/push 修正。
- 新 Git 特性：cherry-pick、reflog、`git reset --keep` 等价、autocrlf、linked worktree 增强、相对 submodule URL。
- **SHA-256 仓库支持**（open/init/clone）。
- config 特性补齐：`core.hooksPath`、`index.skipHash`、`worktreeConfig`。
- 性能：status 计算、packfile 处理、内存占用（LazyIndex、PackScanner、FD 池化 `CloseIdleDescriptors`）。
- 内部：**传输层完全重构**（新 API）、gocheck→testify 测试迁移。

## 二、module path（锚点 1）

事实（v6.0.0-beta.1 zip 内 go.mod:1）：

```
module github.com/go-git/go-git/v6
```

变化仅为主路径 `/v5` → `/v6`。结合下文第三、四节核验，本仓消费面无 API 重命名/搬迁（除 `plumbing.Hash` 内部类型改变，包路径未变），因此 import 改造为 6 文件机械替换。注意 go-billy 同步换主版本（`go-git/go-billy/v5` → `/v6`），但本仓源码**从不直接 import** go-billy（全仓 grep 无消费，消费面盘点亦确认），不受影响。

## 三、消费面 API 逐锚点核验（锚点 2-5）

以下 v6 行号均指本地解包的 `go-git@v6.0.0-beta.1` 源码；v5 行号指本地模块缓存 `go-git@v5.19.2`。

### 3.1 `PlainOpen` / `PlainOpenWithOptions`（本仓 6 处运行时 + 5 处测试）

| 项 | v5.19.2 | v6.0.0-beta.1 | 判定 |
|---|---|---|---|
| `PlainOpen` 签名 | `func PlainOpen(path string) (*Repository, error)`（repository.go:304） | 同（repository.go:425，实现即委托 `PlainOpenWithOptions(path, &PlainOpenOptions{})`） | 不变 |
| `PlainOpenWithOptions` 签名 | 同形（repository.go:310） | 同形（repository.go:431） | 不变 |
| `PlainOpenOptions` 字段 | `DetectDotGit bool` | `DetectDotGit bool` + `AlternatesFS billy.Filesystem`（options.go:826-834） | v6 增字段（本仓零值用法不受影响；v5 是否已有 AlternatesFS 未逐行对照，零值兼容不受影响） |
| `ErrRepositoryNotExists` | 有 | 有（repository.go:59，文案同为 "repository does not exist"） | 不变（本仓黑盒处理，仅备案） |
| **行为差异：osfs 加固** | `osfs.New(path)`（repository.go:266/268/354） | `osfs.New(path, osfs.WithBoundOS())`（repository.go:361/366/482/490/550）——BoundOS 成为 Plain 操作默认（#910 清单项） | **行为变化**：路径访问被限制在仓库根内（越界路径访问收紧）。本仓用法是「先 FindGitRoot 再 PlainOpen 传入已定位的合法根」，正常路径无感；理论上符号链接指向仓库外的边界面行为收紧（推断，未实测本仓数据集） |

### 3.2 `Repository.Head()` / `Remote()` / `Remotes()`

| 方法 | v6 签名（v6.0.0-beta.1） | 判定 |
|---|---|---|
| `Head()` | `func (r *Repository) Head() (*plumbing.Reference, error)`（repository.go:1834） | 不变 |
| `Remote(name string)` | `func (r *Repository) Remote(name string) (*Remote, error)`（repository.go:768） | 不变 |
| `Remotes()` | `func (r *Repository) Remotes() ([]*Remote, error)`（repository.go:783） | 不变 |
| `Remote.Config()` | `func (r *Remote) Config() *config.RemoteConfig`（remote.go:66；本仓 app_git.go:140 消费 `Config().URLs[0]`） | 不变 |
| `Remotes()` 实现 | 与 v5（repository.go:619-632）**逐行相同**：`len(cfg.Remotes)` 计数、无 url 过滤行为一致（v6 repository.go:783-797） | 不变。**这直接回答消费面盘点 §5.1 的待核验项**：v6 对「config 有 remote 段但无 url」的计数口径与 v5 完全一致，A2 方案（手写 config 解析）语义对齐目标不变 |

### 3.3 `Repository.References()` 迭代器（本仓 dashboard.go:351-374）

事实：v6 `References()` 返回 `(storer.ReferenceIter, error)`（repository.go:1851）；接口定义于 `plumbing/storer/reference.go:33-37`：

```go
type ReferenceIter interface {
    Next() (*plumbing.Reference, error)
    ForEach(func(*plumbing.Reference) error) error
    Close()
}
```

三方法集与 v5 相同（`ForEach`/`Close` 消费形态 dashboard.go:351-355 直接兼容）。**判定：零适配**。

### 3.4 `plumbing.Reference` / `ReferenceName` / `HashReference` / `Hash`

| 项 | v6 位置与定义 | 判定 |
|---|---|---|
| `type Reference`（struct） | plumbing/reference.go:426 | 不变 |
| `Name() ReferenceName` | reference.go:471 | 不变 |
| `Hash()` | reference.go:476 | 不变（返回类型见下行） |
| `Type() ReferenceType` | reference.go:466 | 不变 |
| `HashReference ReferenceType = 1` | reference.go:50-51（全仓唯一 go-git 枚举常量消费，dashboard.go:357） | 不变 |
| `type ReferenceName string` | reference.go:70；`IsBranch()` :103、`Short()` :256、`String()` :251 均在 | 不变（app_git.go:151-152 与 dashboard.go:349-350 消费面直接兼容） |
| **`plumbing.Hash`** | **v6：`type Hash = ObjectID`（plumbing/hash.go:15）**；`ObjectID` 是 struct `{hash [32]byte; format ObjectFormat}`（plumbing/objectid.go:61-64）。v5：`type Hash [hash.Size]byte`（plumbing/hash.go:13，即 [20]byte 数组） | **实质 breaking（v6 唯一触及本仓消费面的类型变更）** |

`plumbing.Hash` 变更的逐点影响（对照本仓消费）：

- `head.Hash().String()`：v6 `ObjectID.String()` 返回十六进制（objectid.go:127 附近），本仓三处消费（app_git.go:192-201/249-257、dashboard.go:312）**语义不变，兼容**。
- `Hash` 作为 map key / `==` 比较：v6 仍可比较（struct 全可比较字段），但语义含 `format` 字段，SHA1 对象的 format 为 `UnsetObjectFormat`（FromBytes 推断逻辑 objectid.go:41-48）——跨来源构造的 Hash 直接 `==` 比较存在 format 位不一致的理论风险（**推断，未实测**）。本仓不消费任何 Hash 比较/构造，无实际影响。
- 官方 #910 对此项的预告：「SHA256 支持可能要求 `plumbing.Hash` 或其使用处 API 变更」——已落地为上述别名+结构体方案。

## 四、Go 版本要求（锚点 6）

事实：v6.0.0-beta.1 go.mod 第 4 行 `go 1.26.0`；文件头注释「go-git supports the last 2 stable Go versions」。

官方政策已 RFC 化（模块内 `rfcs/0001-supported-go-versions.md`）：支持最后 2 个 stable Go 大版本，**go.mod 的 go directive 恒为 N-1**（N=最新 stable）；文中明言该政策「已宽松适用于 v6 的 go-git 与 go-billy，RFC 使其永久化」。

对本仓判定：

- 本仓 `go 1.26.0` / `toolchain go1.26.10`（go.mod:3-5）**恰好满足且零余量**——v6 当前 directive 正是 1.26.0。
- 未来 v6 正式版或后续版本把 directive 升到 1.27 时，本仓须同步升 toolchain（RFC 政策决定了这是每半年节奏的固定跟进项）。
- CI 的 Go 版本矩阵须 ≥ 1.26.0（与现仓一致，无新增动作）。

## 五、依赖树变化（锚点 7）

来源：v6.0.0-beta.1 zip 内 go.mod 全文，与本仓 go.mod（v5.19.2 链）对照。

| 依赖 | 本仓现状（v5.19.2 链） | v6.0.0-beta.1 要求 | 变化性质 |
|---|---|---|---|
| go-billy | `go-git/go-billy/v5 v5.9.1`（go.mod:33） | `go-git/go-billy/v6 v6.0.0-beta.1` | **主版本换代**（且同为 beta） |
| gcfg | `go-git/gcfg v1.5.1-0.2023...`（go.mod:32） | `go-git/gcfg/v2 v2.0.2` | 主版本换代；v6 中被 `plumbing/format/config/decoder.go`（非测试代码）真实使用，为运行时依赖 |
| ProtonMail/go-crypto | v1.4.1（go.mod:26） | v1.5.2 | minor 上抬 |
| pjbgf/sha1cd | v0.6.0（go.mod:50） | v0.7.0 | minor 上抬 |
| armon/go-socks5 | 无 | 新增（v6 直接依赖） | 传输层重构引入 |
| gliderlabs/ssh | 无 | 新增 v0.3.8 | v6 新增内置 git server（internal/server）与 SSH 传输 |
| golang.org/x/sync | 无（直接项） | 新增 v0.23.0 | 传输层引入 |
| go-git-fixtures/v6 | 无 | `v6.0.0-alpha.1`（require 主块） | 测试 fixture 模块；被 v6 的 `internal/fixtureutil`、`internal/server` 等非 _test 文件引用。是否进入本仓二进制构建图取决于 Go module graph 剪枝与本仓 import 路径（**推断**：至少进 go.sum，构建图大概率被剪掉，升级后以 `go mod graph`/`go build` 实际输出为准） |
| emirpasic/gods | v1.18.1（go.mod:31） | v1.18.1（v6 直接依赖） | 延续 |
| kevinburke/ssh_config | v1.6.0（go.mod:39） | v1.6.0 | 延续 |

结构性说明：v6 的传输层完全重构（alpha.1 notes「Major: Transport Overhaul」）是新增 socks5/ssh/server 类依赖的根因；本仓从不消费 go-git 传输能力（fetch/push/clone 全 CLI），这部分依赖属「被迫携带」。此外 `stretchertestify`/`go-shlex` 等出现在 v6 go.mod 属其自身测试/服务端依赖，经 module graph 剪枝后大概率不进本仓构建（**推断**）。

## 六、对本仓消费面的行为级影响汇总

综合 #910（官方「API and Behaviour Changes for next major release」，仍 open，即**清单未冻结**）与源码核验：

| #910 清单项 | 本仓相关性 |
|---|---|
| **须对 Repository 调 `.Close()`**（PR 1906；v6 `Repository.Close()` 在 repository.go:690，委托 Storer 的 io.Closer；v5 无此方法） | **高**。`HasRemotesBatch`（service/git.go:575，并发 8 打开最多 100 仓库）与 app_git.go/dashboard.go 各打开点现状均不关闭；v6 的 filesystem storage 引入 pack FD 池化（alpha.5 PR 2132/2133/2154），不 Close 有句柄驻留风险（Windows 上句柄占用还会阻碍外部 git 写操作，与本项目「锁残留告警禁自动删」的既有约束叠加） |
| osfs.BoundOS 成为 Plain 操作默认 | 低（见 §3.1；本仓传入的是 FindGitRoot 定位过的合法根） |
| config 校验放宽：`branch.*.merge` 无 `refs/` 前缀不再拒绝（#1923） | 低：本仓只读 config，放宽方向不触发新错误 |
| `.gitignore` 匹配逻辑对齐上游 git（PR 1940） | 无（本仓不用 worktree status/gitignore） |
| `utils/sync` zlib 类型接口化（#2012）、idxfile.Index 新增 `.Close()`（#2132）、传输层新 API、tags refspec 变化（PR 2250/1459） | 无（均在本仓未消费的面：pack/idx/传输/fetch） |
| #910 中其余 repository.go/worktree_status.go/difftree.go/server.go 挂钩项 | 未逐项展开（均不在本仓消费面）；#910 仍 open，v6 正式版冻结前清单可能继续增减 |

## 七、官方文档与链接（锚点 9）

| 资源 | 链接 | 状态说明 |
|---|---|---|
| 官方 breaking 变更追踪 issue | https://github.com/go-git/go-git/issues/910 | **open**（2023-11 创建，持续滚动），标题「API and Behaviour Changes for next major release」——事实上的迁移清单权威源 |
| 版本推进主 issue | https://github.com/go-git/go-git/issues/1395 | closed（2025-07 关闭，内容为指向 #910 的 v6 分支切换说明） |
| v6.0.0-beta.1 release | https://github.com/go-git/go-git/releases/tag/v6.0.0-beta.1 | prerelease 标记；正文为 PR 清单 |
| v6.0.0-alpha.1 release | https://github.com/go-git/go-git/releases/tag/v6.0.0-alpha.1 | v6 用户面变更总览最完整的一份 |
| v6 开发分支 | 仓库 `main` 分支（#1395 正文确认 breaking 变更并入 main） | |
| 仓库内文档 | `HISTORY.md`（项目编年史，**非**变更日志）、`COMPATIBILITY.md`（git 命令兼容性矩阵）、`EXTENDING.md`、`rfcs/0001-supported-go-versions.md` | 模块内**无 CHANGELOG.md、无 migration guide**（2026-10-21 解包核实） |
| go-billy v6 | https://github.com/go-git/go-billy（v6 线同为 beta） | 与 go-git v6 配套 |

## Caveats / Not Found

- **未查到**：官方专门的 v6 migration guide / COMPATIBILITY 之外的迁移文档。GitHub 站内搜索「migration v6 in:title」零命中；权威依据只能是 issue #910 + 各 release notes。
- #910 仍为 open 状态，v6.0.0 正式版发布前 breaking 清单**不具冻结效力**；本报告反映的是 v6.0.0-beta.1 时点事实。
- `plumbing.Hash` 变更对「跨来源构造 Hash 后 `==` 比较」的 format 位风险为**推断**（源码结构推得，未实测）；本仓无此类消费，不影响结论。
- go-git-fixtures/v6 等测试类依赖是否进入本仓构建图为**推断**（基于 Go module graph 剪枝规则），升级后须以 `go mod graph` 实测为准。
- v5.19.2 → v5.19.3 的补丁内容未展开（若走「留 v5」缓冲路线，v5.19.3 与 v6.0.0-beta.1 同日发布，大概率含同批修复，未逐 PR 核验）。
- BoundOS 对本仓真实仓库数据集（含符号链接 worktree 等边界面）的行为差异未实测，属 §3.1 已标注的推断项。
