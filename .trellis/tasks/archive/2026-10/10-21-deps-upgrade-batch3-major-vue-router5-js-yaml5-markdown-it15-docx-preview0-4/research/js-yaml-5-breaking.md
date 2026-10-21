# Research: js-yaml 4→5 major 升级 breaking changes 与本仓适配面

- **Query**: js-yaml 4→5 major 升级的 breaking changes 与本仓适配面调研
- **Scope**: mixed（内部代码盘点 + 外部 CHANGELOG/迁移指南/npm 元数据）
- **Date**: 2026-10-21

## 现版本与目标版本

| 项 | 值 | 出处 |
|---|---|---|
| 当前声明 | `~4.3.2`（tilde 档） | `frontend/package.json:49` |
| lock 实际落点 | 4.3.2 | `frontend/package-lock.json:2960` |
| npm dist-tags | `latest: 5.5.0`、`v4-legacy: 4.3.2`、`v3-legacy: 3.15.2` | npm registry（2026-10-21 查询） |
| 目标版本 | 5.5.0（2026-10-15 发布） | npm registry |
| v5 时间线 | 5.0.0（2026-06-20）→ 5.5.0（2026-10-15），4 个月 12 个版本，主线活跃 | npm registry time 字段 |

v5 依赖面：仅 `argparse ^3.0.3`（CLI bin 用）；`dist/js-yaml.mjs` 为自包含 bundle（已解包 tarball 验证，无 argparse 引用），Vite 打包无 node 依赖问题。

**v5 包入口（exports）**：`.` → import `dist/js-yaml.mjs` / require `dist/js-yaml.cjs.js`；浏览器专用构建移到 `js-yaml/browser` 子路径。本仓走 bundler import 条件解析到 mjs 构建，无额外适配。

## 本仓消费 API 清单

全仓 grep 确认源码 import 仅两文件，测试文件不直接 import js-yaml（仅注释提及）：

### 1. `frontend/src/components/FilePreviewRenderer.vue`（markdown frontmatter 解析）

| 位置 | 调用形态 | 说明 |
|---|---|---|
| L261 | `import jsyaml from 'js-yaml'` | **default import** |
| L485 | `jsyaml.load(raw)` 无 options | 仅此一处调用，默认 schema |
| L484-491 | try/catch 静默降级 | catch 返回 null → 降级为 YAML 代码块，不读异常字段 |
| L487 | 结果判空 + 判普通对象 | 数组/标量降级 |
| L509-514 | `v instanceof Date` → `toISOString().slice(0,10)` | 依赖 v4 把 `YYYY-MM-DD` 解析为 Date 的行为 |

### 2. `frontend/src/components/JsonYamlToolPanel.vue`（JSON/YAML 互转工具）

| 位置 | 调用形态 | 说明 |
|---|---|---|
| L58 | `import yaml from 'js-yaml'` | **default import** |
| L83 | `yaml.loadAll(text)` 返回数组形态 | 读 `docs.length` / `docs[0]`，多文档显式拒绝 |
| L129、L154 | `yaml.dump(value, { lineWidth: -1 })` | 唯一 dump 选项 |
| L102 | `e.reason \|\| e.message`、`e.mark ? e.mark.line + 1 : null` | 异常字段行号展示（0 起始转 1 起始） |

### 3. 测试断言面（间接消费，通过组件行为）

- `frontend/src/components/__tests__/FilePreviewRenderer.spec.js`（L410-556 frontmatter 11 用例）：断言面板存在性/数组 el-tag/降级代码块/非对象降级，**无 Date 类型直接断言**。
- `frontend/src/components/__tests__/JsonYamlToolPanel.spec.js`：L104 YAML 错误行号 `第 2 行`；L128/L144 dump 输出**逐字断言** `'a: 1\nb: x\n'`。

### 4. 工程联动点

- `frontend/package.json:49` 声明 `~4.3.2`。
- `scripts/check-dep-consistency.mjs:49`：js-yaml 已登记 tilde 清单，无需改清单，仅改声明版本（tilde 档 major 升级声明形态 `~5.5.0`，以 lock 实际落点为准）。

## v5 breaking changes 逐条

来源：官方 `CHANGELOG.md`（GitHub contents API，tag 5.5.0）+ `docs/migrate_v4_to_v5.md` + `dist/js-yaml.d.ts` 逐项核对。

### v5.0.0（核心重构，TypeScript 重写）

| # | 变更 | 内容 |
|---|---|---|
| 1 | **无 ESM default export** | 官方迁移指南明说「by design」，d.ts 导出表全为 named exports（`load`/`loadAll`/`dump`/`YAMLException`/schemas/tags），无 default。default import 在 Vite ESM 下得到 `undefined`，调用即 TypeError |
| 2 | **load 默认 schema 变更** | `DEFAULT_SCHEMA`（YAML 1.1 类型集）→ `CORE_SCHEMA`（YAML 1.2 core），默认**不含 `!!merge`（`<<`）**、不含 `!!timestamp`/`!!binary`/`!!set` 等 1.1 类型。可用 `CORE_SCHEMA.withTags(mergeTag)` 或 `YAML11_SCHEMA` 加回 |
| 3 | **load('') 空输入 throw** | v4 返回 `undefined`；`loadAll('')` 不变（仍返回空数组） |
| 4 | 复杂 mapping key throw | 数组/对象作 key 从静默字符串化改为 throw（可用 `legacyMapTag` 还原，不推荐） |
| 5 | 移除导出 | `DEFAULT_SCHEMA`、嵌套 `types` 命名空间、`Type` 类、`Schema.extend()`（→`withTags()`）、`lib/` 内部路径导入 |
| 6 | 移除 loader 选项 | `onWarning`、`legacy`、`listener` |
| 7 | 移除/改名 dumper 选项 | `styles`、`replacer`、`noCompatMode`、`condenseFlow`、`quotingType`、`noArrayIndent`（→`seqNoIndent`）移除；`forceQuotes` 移除后 5.1.0 以新语义恢复（preferred 而非 force） |
| 8 | loadAll iterator 签名 deprecated | 返回数组形态保留 |
| 9 | dump 默认兼容保证 | dump 默认 `YAML11_SCHEMA` + CORE 引号检查，官方承诺「guarantee backward compatibility by default」；`lineWidth` 选项保留（d.ts L824 确认） |

### 5.1.0 ~ 5.5.0 增量 breaking

| 版本 | 变更 |
|---|---|
| 5.1.0 | `quoteStyle` 语义改为 preferred，force 场景用恢复的 `forceQuotes` |
| 5.2.0 | `maxMergeSeqLength` → `maxTotalMergeKeys`；新增 `maxAliases` |
| 5.3.0 | 自定义 tag `identify` 必填（load-only 用 `identify: () => false`）；flat 常量导出 deprecated（grouped exports 替代，旧导出保留）；`MERGE_KEY` 导出移除 |
| 5.4.0 | 低层 AST style 表示重构（仅影响直接构造/编辑 AST 节点的代码）；`sortKeys` 重写；标量风格选择重排——**whitespace-only 字符串现在双引号输出**（不改变加载值，仅序列化观感） |
| 5.4.1/5.2.2 | 安全修复：merge 序列 CPU 限制（`maxTotalMergeKeys`）、嵌套 flow 序列指数解析时间 |
| 5.5.0 | 非 printable 字符错误位置精确化；block plain scalar 起始 `,]}` 拒绝 |

### 异常结构（本仓消费点核对）

d.ts 确认 `YAMLException extends Error` 保留 `reason: string` 与 `mark?: { line, column, buffer, position, snippet }`，行号 0 起始语义不变 → JsonYamlToolPanel 行号展示逻辑兼容。

## 本仓影响判定表

| 调用点 | 涉及变更 | 判定 | 改法 |
|---|---|---|---|
| FilePreviewRenderer.vue L261、JsonYamlToolPanel.vue L58（default import） | 变更 #1 无 default export | **需适配（唯一硬改点）** | 改 namespace import：`import * as jsyaml from 'js-yaml'` / `import * as yaml from 'js-yaml'`——调用点 `jsyaml.load`/`yaml.loadAll`/`yaml.dump` 零改动；或改 named import（改动面更大，不推荐） |
| FilePreviewRenderer.vue L485 `load(raw)` 默认 schema | 变更 #2 CORE_SCHEMA | 不受影响（有兜底） | `!!timestamp` 移除：`date: 2026-10-21` 从 Date 变字符串，`formatFmValue` 字符串分支输出同为 `YYYY-MM-DD` 原样，展示等价；`!!set`/`!!binary` 等 1.1 tag 出现时 v4 出结构化面板、v5 throw → 走既有 catch 降级 YAML 代码块（frontmatter 场景极罕见） |
| FilePreviewRenderer.vue L509-514 `v instanceof Date` 分支 | 变更 #2 | 不受影响（分支成死代码） | 保留无害；可顺手清理（CORE_SCHEMA 下不再产生 Date） |
| FilePreviewRenderer.vue 空白 frontmatter | 变更 #3 load('') throw | 不受影响 | L483 `!raw` 前置拦截空串；仅空白 raw（如 `\n`）v4 返回 undefined 降级、v5 throw 被 catch 降级，殊途同归 |
| JsonYamlToolPanel.vue L83 `loadAll(text)` | 变更 #3/#8 | 不受影响 | `loadAll('')` 仍返回空数组；返回数组形态保留；空文本已在 validateOnly L110-113 trim 拦截，其余入口空文本时 v4/v5 均产生错误结果行 |
| JsonYamlToolPanel 用户输入 `<<` merge | 变更 #2 默认无 merge | **低风险行为变化（可接受）** | v4 展开 merge、v5 `<<` 成普通键。工具已有「锚点/别名引用有损确认」弹窗覆盖该语义；如需严格保 v4 行为可 `loadAll(text, { schema: CORE_SCHEMA.withTags(mergeTag) })`，建议不加 |
| JsonYamlToolPanel 用户输入复杂 key（`? [a]` 等） | 变更 #4 throw | 可接受 | 从静默有损字符串化变报错行展示，更安全，无需改 |
| JsonYamlToolPanel.vue L129/L154 `dump(value, { lineWidth: -1 })` | 变更 #9/#7/5.4.0 风格重排 | 不受影响（需回归验证） | `lineWidth` 保留；`styles` 等移除选项本仓未用；dump 官方向后兼容保证；spec.js L128/L144 逐字断言 `'a: 1\nb: x\n'` 预期不变（`x` 非 whitespace-only），升级后以测试证实 |
| JsonYamlToolPanel.vue L102 `e.reason`/`e.mark.line` | 异常结构 | 不受影响 | d.ts 确认 `reason`/`mark.line` 保留且 0 起始语义不变 |
| `DEFAULT_SCHEMA`/`types`/`safeLoad`/`Type`/`Schema.extend`/移除的 loader/dumper 选项 | 变更 #5/#6/#7 | 不受影响 | 全仓 grep 确认均未使用 |
| 自定义 tag / `identify` / 常量导出 / AST / `sortKeys` / `quoteStyle` | 5.3/5.4 变更 | 不受影响 | 本仓零自定义 tag、零常量/AST 消费 |
| Vite 打包 | browser 构建移至 `js-yaml/browser` | 不受影响 | 主入口 `.` 的 import 条件即浏览器可用 mjs（自包含无 argparse），bundler 自动解析 |
| `frontend/package.json` 声明 | major 升级 | 需适配 | `~4.3.2` → `~5.5.0`；`check-dep-consistency.mjs` 清单已登记无需改 |

## 结论建议

**建议升级**（改动面 2 处 import + 1 处声明，无逻辑改动）。

理由：

1. **必改点极小且机械**：两文件 default import → namespace import，调用点零改动；这是全部硬适配。
2. **行为变化面均有兜底或等价输出**：frontmatter 日期展示等价（Date 与字符串分支同输出）、解析失败场景本就有 catch 降级链、dump 官方向后兼容保证。
3. **跳过代价持续增长**：v4 已挂 `v4-legacy` tag 仅收安全 backport，主线安全修复（5.2.2 指数解析 DoS、5.4.1 merge CPU 限制）与 bugfix 均只进 v5；v5 四个月 12 版说明实现进入快速收敛期，早升级早脱离 legacy 分支。
4. **消费面是本批四 major 中最小的**：两文件、四类 API（load/loadAll/dump/load 选项零配置），测试断言面清晰（两组件 spec）。

代价与验证面：

- 改动：2 处 import、package.json 声明；可选清理 `formatFmValue` 的 Date 死分支。
- 必跑：`cd frontend && npm test`（重点 FilePreviewRenderer.spec.js frontmatter 11 用例 + JsonYamlToolPanel.spec.js dump 逐字断言/行号用例）、`npx vite build`（验证 mjs 入口解析）、`node scripts/check-dep-consistency.mjs`。
- 唯一「需跑测试证实」项：dump 标量风格重排（5.4.0）对本仓逐字断言的影响，推理预期不变（官方兼容保证 + 断言样本不含 whitespace-only 字符串），不能纯纸面确认。

## Caveats / Not Found

- GitHub Releases 页面为空（该仓只用 tag + CHANGELOG 发版），变更信息以仓库 CHANGELOG.md（tag 5.5.0）为准，已逐条核对。
- `load` 在 v4 对「仅空白字符输入」返回 undefined 的确切值未实测（依据 CHANGELOG「empty input 返回 undefined」与迁移指南推断），但因两分支（undefined 与 throw）在本仓均走降级，结论不受影响。
- dump 逐字断言在 5.5.0 下的实际输出未在本机跑通验证（研究阶段未改依赖），已列入升级时必跑验证面。
