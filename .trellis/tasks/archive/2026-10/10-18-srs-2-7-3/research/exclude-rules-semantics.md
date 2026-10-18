# Research: 素材管理扫描排除规则（ExcludeDirs/ExcludeExts）匹配语义设计

- **Query**: Billfish/Eagle/Adobe Bridge/Everything 排除规则语义对比；Go filepath.Match 能力边界与第三方取舍；本仓库推荐语义；WalkDir 剪枝实现要点
- **Scope**: mixed（外部工具文档 + 仓库现状 + Go 标准库实证）
- **Date**: 2026-10-18

## 结论速览

**推荐语义**：`ExcludeDirs` 每项一个 glob 模式，对相对库根路径的**每层目录段名**做完整匹配（`filepath.Match` 单段语法 `*`/`?`/`[..]`，大小写不敏感）；**不支持** `/`、`**` 多段路径模式。`ExcludeExts` 归一为「带点小写」（`.tmp`），输入宽容 `tmp`/`.tmp`/`*.tmp` 三形态。匹配基座零新增依赖（`path/filepath` 标准库），无 `**` 需求即无引入 doublestar/gobwas 的动机。四个对标工具没有一个提供用户可配置的多段路径 glob 排除语法，单层目录段语义与业界实践一致。

## Findings

### 1. 各工具排除规则语义对比

| 工具 | 目录规则语义 | 目录 glob 支持程度 | 后缀规则形态 | 依据与置信度 |
|---|---|---|---|---|
| **Billfish** | 官方文档未公开排除规则匹配语义 | 未公开 | 未公开 | 官方帮助中心（billfish.cn/help）仅 10 篇文章，全文检索无「排除/忽略」关键词；`.damignore` 一词公网搜索引擎无任何产品文档对应，**判定为 SRS 自拟概念**（类比 `.gitignore` 造词，DAM = Digital Asset Management），非 Billfish 官方机制 |
| **Eagle** | 手册 185 页中无「排除规则」专页；文件夹维度为「自动导入」监控目录的显式指定（用户选哪些目录被监控，非 glob 黑名单） | 无 glob 排除语法证据 | 文件类型控制 = 偏好设置中的扩展名清单（**白名单**形态，与本项目「忽略后缀」黑名单方向相反） | 官方手册目录逐页枚举（cn.eagle.cool/support，185 页无 exclu/ignor 命中）；`edit-file-extensions` 页存在但正文 JS 渲染未抓到全文，白名单形态为常识级未终验 |
| **Adobe Bridge** | 无扫描排除规则（非索引型实时媒体浏览器，仅缩略图缓存，无用户可配置的扫描排除） | 不适用 | 不适用 | 否定性结论：未检索到任何 Bridge 排除规则功能的官方文档；产品形态为浏览器非索引器 |
| **Everything 1.5** | 排除文件夹 = **绝对路径列表**（`C:\Windows`，Add Folder）+ 可选通配过滤器 / `regex:` 前缀过滤器（Add Filter），分号分隔 | 绝对路径 + wildcard + regex 混合列表（非「任意层级目录名」语义，靠绝对路径锚定） | Exclude files = 分号分隔通配列表，形态 `*.db`、`*.mp3;*.mkv`（**带点带星**），可 `regex:`；另有 hidden/system 属性排除 | 官方 options 文档在线抓取全文（voidtools.com/support/everything/options/），置信度高 |
| gitignore（业界参照） | **无 `/` 模式**：匹配任意层级的目录/文件段名；**含 `/` 模式**：相对锚定的路径模式 | `*` 不跨 `/`；`**` 跨段（`**/foo`、`foo/**`、`a/**/b` 三形态）；段内 `**`（如 `a**b`）非法；`foo/` 尾斜杠 = 仅目录；`!` 前缀 = 否定规则 | 不设独立后缀规则，用模式 `*.tmp` 表达（无斜杠 → 任意层级命中） | git 官方语义；go-git 包 doc.go 完整复刻（见 §2） |
| VSCode files.exclude | 相对工作区根的 glob，模式**隐式包裹 `**/` 前缀**（`name` 等价 `**/name/**`） | `*` 单段、`**` 跨段、`{a,b}` 备选、`[a-z]` 字符类、`?` | 同 glob 机制（`**/*.tmp`） | 常识级引用，本次未在线验证（code.visualstudio.com 连接中断） |

**横向结论**：业界主流形态是「**目录名单层段名匹配**（gitignore 无斜杠模式 / VSCode 隐式 `**/` 前缀）+ **后缀带点通配**（`*.tmp`）」。多段路径模式（`a/b`、`**`）仅 gitignore/VSCode 这类面向开发者的配置提供；素材管理工具（Billfish/Eagle）的可见配置面均未暴露多段 glob。

### 2. Go 标准库能力边界（go1.26.6 实证）

`path.Match` / `filepath.Match` 语法：`*`（不跨分隔符的任意字符序列）、`?`（单字符，不跨分隔符）、`[...]`（字符类，支持 `a-z` 范围与 `^` 否定）、`\` 转义。**整串完整匹配**（非子串/前缀）。以下为临时目录 `go run` 实测结果：

| 表达式 | 结果 | 含义 |
|---|---|---|
| `path.Match("a/**/b", "a/x/y/b")` | `false` | **`**` 不被支持**，退化为单段 `*`（`a/**/b` 只能匹配 `a/<单段>/b`） |
| `path.Match("a/**/b", "a/x/b")` | `true` | 同上佐证 |
| `path.Match("*", "a/b")` | `false` | `*` 单段通配，不跨 `/` |
| `filepath.Match("a/b", "a\\b")`（Windows） | `false` | **关键坑**：Windows 下 Separator 为 `\`，模式中的 `/` 只是普通字符不匹配 `\` |
| `filepath.Match("DRAFT", "draft")` | `false` | Match 本体**大小写敏感**（无 OS 特化） |
| `path.Match("*草稿*", "营销草稿2024")` | `true` | UTF-8 按 rune 匹配，中文无碍 |
| `path.Match("[a-c]x", "bx")` | `true` | 字符类范围可用 |
| `path.Match("[a", "x")` | `ErrBadPattern` | 坏模式返回语法错误，**保存时可探测校验** |

能力边界总结：单段通配完整覆盖本需求；不支持 `**`、`{}` 备选；**无预编译 API**（每次调用全量解析模式串）；无大小写开关（需调用方自行 ToLower 归一）。

#### 多段 / `**` 的四条实现路径对比

| 方案 | 依赖成本 | 能力 | 结论 |
|---|---|---|---|
| `path/filepath.Match` | 0 | 单段 | **推荐基座**（语义定为单段后即终局） |
| `go-git/v5/plumbing/format/gitignore` | 0（已在直接依赖树，go.mod `go-git v5.19.2`，可直接 import） | 完整 gitignore（`**` 跨段、`/` 锚定、dirOnly、`!` 否定、domain 前缀） | API：`ParsePattern(p, domain) Pattern` + `NewMatcher(ps) Matcher`，`Match(path []string, isDir bool)`。**隐患**：`!` 前缀被解析为「重新包含」否定规则（用户误输入 `!foo` 静默变反义）；无斜杠模式对每层段名逐一匹配（`filepath.Match`），`#` 注释仅在 `ReadPatterns` 读文件链路处理（`ParsePattern` 不处理，直接 API 无注释问题）；domain 机制对本场景纯冗余。能力超集引入了排除场景不需要且危险的行为 |
| `bmatcuk/doublestar` v4 | +1 直接依赖（纯 Go、零传递依赖） | `**`、`{}`、`Match`/`Glob`/`PathMatch` | 能力过剩，违背仓库「纯 Go 标准库优先」倾向（参照：SQLite 域明文禁 CGO 驱动保交叉编译的既有决策方向） |
| `gobwas/glob` | +1 直接依赖 | 分隔符感知 `**`、`{}` | 同上 |
| 手写展开（`a/**` → WalkDir 层级前缀判断） | 0 | 按需定制 | 需自实现模式解析 + 边界用例（段内 `**` 非法性、尾随形态）+ 测试面；仅当确需多段语义时才值得 |

**决策**：语义定为「单层目录段名匹配」后，标准库即足额，四条扩展路径全部不需要。将「不支持 `**`」作为 v1 明确边界写入 UI 文案（「按文件夹名匹配，支持 `*` `?` 通配」），比引依赖或手写展开更符合本仓库约束。

### 3. 本仓库现状锚点（内部检索）

| 文件 | 说明 |
|---|---|
| `service/assets_scan.go:631` | `collectPptxEntries(root)`：WalkDir 收集清单；目录分支仅 err 降级 SkipDir；文件分支 `strings.EqualFold(path.Ext(p), ".pptx")`（后缀大小写不敏感先例）+ `filepath.Rel` + `ToSlash` + `isPptxSplitDerivative`（拆分派生强制排除） |
| `model/assets.go:12` | `AssetLibrary` 现四字段（ID/Name/RootPath/CreatedAt/UpdatedAt），本批加 `ExcludeDirs []string`、`ExcludeExts []string` |
| `.trellis/tasks/10-18-srs-2-7-3/prd.md` R1 | 已倾向「glob 模式匹配相对路径任意层级目录段」，多段支持留 research 定夺（本文档结论：不支持） |
| `go.mod` | 无 glob 类第三方依赖；go-git v5 为直接依赖（其 gitignore 包可用但见 §2 隐患） |
| SRS `docs/原始需求/202610-素材管理系统需求说明书-V1.0.md` §2.1.2/§2.7.3 | 「过滤配置：忽略目录、忽略后缀」；`.damignore` 引述经查证为自拟概念（见 §1） |

### 4. 最终推荐语义

#### ExcludeDirs（每项一个 glob 模式）

1. **匹配对象**：相对库根路径的**每一层目录段名**（等价 gitignore「无斜杠模式」语义 / VSCode files.exclude 隐式 `**/` 前缀语义）。例：`*草稿*` 命中任意层级的「营销草稿2024」目录；`archive` 命中任意层级的 archive 目录。
2. **语法**：`filepath.Match` 单段（`*` `?` `[...]`）；**不支持** `/`、`**`（UI 文案明确「按文件夹名匹配」）。
3. **大小写**：不敏感。实现 = 模式与段名各自 `strings.ToLower` 后 `Match`（Go 无 locale 陷阱；中文 ToLower 无副作用；与 Windows FS 大小写不敏感及资源管理器输入习惯一致；仓库内 `EqualFold` 先例对齐）。字符类 `[a-z]` 经双侧归一小写后语义仍成立。
4. **接受的能力损失**：无法表达「仅根下 archive」（锚定多段）。判定可接受：四个对标工具无一提供该能力；素材库主诉求是「排除同名文件夹」（草稿/归档/拆分产物），段名语义一句话可解释，列表式 UI 下每行一模式心智负担最低。
5. **保存时校验**：`filepath.Match(pat, "")` 仅用于探测 `ErrBadPattern` 拒绝坏模式；空串/空白项 TrimSpace 后丢弃。

#### ExcludeExts（后缀黑名单）

1. **存储归一**：带点小写 `.tmp`。归一链 = `TrimSpace` → 剥 `*.` 或前导 `.` → 补前导 `.` → `strings.ToLower`。
2. **输入宽容**：`tmp` / `.tmp` / `*.tmp` 三形态归一为同一存储值（Everything `*.db` 形态是用户常见输入习惯）。
3. **匹配**：`strings.EqualFold(path.Ext(p), ext)`（`path.Ext` 返回带点；EqualFold 与现有 `.pptx` 判断同构，或存储已小写 + ToLeft 比较，二选一保持单点）。
4. **方向**：黑名单（忽略后缀），非 Eagle 的白名单扩展名模型；与 PRD/SRS 一致。

#### 优先级（重申 PRD 既定）

`isPptxSplitDerivative` 强制排除（系统内置）先于用户规则，用户规则不可覆盖（对齐 SRS「派生文件必须强制排除」）。

### 5. WalkDir 剪枝实现要点

1. **规则编译一次为闭包**：`collectPptxEntries` 签名扩展为携带规则（如 `collectPptxEntries(root, dirMatcher, extSet)`），匹配器在扫描入口构建一次（规则预 ToLower 存储），WalkDir 回调闭包捕获。**禁每文件重编译/重复 ToLower**。量级评估：规则个位数 × 目录数千级次 `Match`，总量微秒级，性能非瓶颈——「编译一次」是自然形态而非硬性优化。
2. **目录剪枝点**：`d.IsDir()` 分支、`p != root` 时对 `d.Name()`（目录基名）做匹配，命中 `return filepath.SkipDir`（该子树全部文件与子目录不再进入，排除语义正确）。**必须豁免 root 自身**：`p == root` 时 `d.Name()` 为根目录名，若参与匹配会把整库剪空（如根目录名恰好含「草稿」）。用基名匹配无需 `filepath.Rel`，天然与分隔符无关（规避 §2 的 Windows `\`/`/` 坑）。
3. **文件分支**：后缀命中 `return nil` 不入 entries。注意收集层过滤与 excluded 态是**两条独立链**：excluded 判定输入是「规则 × 存量索引 path」（处置层做，PRD R2 先排除后存在性），不依赖本次磁盘扫描是否发现该文件；收集层只负责「磁盘扫描结果不含命中文件」。
4. **目录不可入分支**（err != nil 的 SkipDir 权限降级）保持现状，与规则剪枝互不干扰；两处 SkipDir 返回值语义相同无冲突。
5. **坏规则防御**：扫描运行时 `Match` 返回 `ErrBadPattern` 一律视为不命中 + `slog.Warn`（防手改 libraries.json 塞坏规则致整库扫空；保存 API 已校验为第一道防线）。
6. **未来扩展预留**：若后批确需多段路径模式，优先评估 go-git gitignore 包（零新增依赖，但须先封死 `!` 否定语义）或手写展开；届时路径匹配须 `ToSlash` 后统一用 `path.Match`（`/` 模式 + `path.Match` 配对，禁与 `filepath.Match` 混用）。

## Caveats / Not Found

- Billfish 排除规则匹配语义**未获任何官方文档佐证**（帮助中心全文检索无命中）；`.damignore` 为 SRS 自拟概念，其「支持 glob 规则」的具体语法不存在可参照的官方定义，语义设计不受其约束。
- Eagle 结论基于官方手册目录枚举（无排除规则专页）；扩展名白名单形态未获正文终验（JS 渲染）。
- Adobe Bridge 为否定性结论（未检索到排除规则功能存在证据），非穷尽性证明。
- Everything 依据为官网当前 options 文档（对应 1.5）；1.4 排除对话框形态类似但未单独验证。
- VSCode files.exclude 行未在线验证（官方文档站连接中断），为常识级引用。

## Related Specs

- `docs/spec/assets-management.md` — assets 域主规范（第十批节随实现沉淀）
- `model/assets.go` — `AssetLibrary`（libraries.json 持久化，util.SaveJSON 原子写口径）
- `service/assets_scan.go` — `collectPptxEntries`（WalkDir 收集链路）、`isPptxSplitDerivative`（强制排除先例）
- `.trellis/tasks/10-18-srs-2-7-3/prd.md` — R1 排除规则需求与生效点定义
