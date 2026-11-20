# E2E 监听计数口径收敛：helper 复用 + 防回流守护

> 原任务标题「E2E window 监听计数断言统一为相对增量口径」在取证后修正：存量用例已成相对增量，真实缺口是**复用面**与**防回流**。

## Goal

把「window 级监听注册计数」的 E2E 实现收敛为单一可复用 helper，并加一条窄而硬的静态守护，使内联计数 hook 与绝对基线断言从结构上无法再被写回，避免 §12 事故（新增合法注册点打挂旧用例）复发。

## Requirements

1. **新增 helper `frontend/e2e/listenerCounter.js`**
   - 注入方式：`page.addInitScript` 覆写 `window.addEventListener` / `removeEventListener`，按事件类型统计 `added` / `removed` 次数；注入幂等（同一 page 重复调用只生效一次，页面内以标志位守卫）。
   - 注入时机：`page.goto` 之前调用（与现用例一致）。
   - 读取 API 只暴露相对口径：`snapshot(page)` 取当前计数、`delta(page, base)` 取相对某基线（或等价设计），**不提供「断言绝对值 = N」的便捷入口**。
   - 支持多事件类型（`['keydown']`、`['pointerdown','resize']` 等），计数按类型分组，互不干扰。
   - 不依赖 `@playwright/test` 直接 import（helper 只消费传入的 `page` 对象），spec 仍从 `fixtures.js` 取 `test/expect`。
2. **`frontend/e2e/terminal-tabs.spec.js` 改为消费 helper**
   - 删除内联 `addInitScript`（`terminal-tabs.spec.js:159-172`）与 `__keydownAdded/Removed` 裸读取，断言保持原有语义与数值（`addBase + 1`、`removeBase + 2` 等）。
   - 保留并更新注释：列全当前 window 级注册点与排除面（§12.1 要求）。
   - 行为等价，不得放松任何一条既有断言。
3. **新增静态守护 `scripts/check-e2e-listener-hooks.mjs`**
   - 扫描 `frontend/e2e/**/*.js`，命中以下形态即失败（报 `file:line` 与命中片段）：
     - `window.addEventListener =` / `window.removeEventListener =`
     - `addEventListener.bind(window)` / `removeEventListener.bind(window)`
     - `EventTarget.prototype.addEventListener`
   - 唯一放行点：`frontend/e2e/listenerCounter.js` 本体。
   - 支持 `--selftest`：临时目录注入「违规样例 / 合规样例」，断言检查器分别拦截与放行，退出码语义与 `scripts/check-dep-consistency.mjs` 对齐（0 通过 / 1 违规）。
   - 支持 `<dir>` 参数指定扫描根（供 selftest 使用），默认扫描仓根 `frontend/e2e`。
4. **挂载与文档**
   - CI：`.github/workflows/ci.yml` 在既有依赖一致性检查步骤相邻位置增加 `--selftest` + 正式检查两步（copy 现有两步形态）。
   - `frontend/package.json` 增加 `check:listener-hooks` 脚本供本地直跑（指向 `../scripts/check-e2e-listener-hooks.mjs`）。
   - `docs/spec/e2e-testing.md` §12.1 追加：helper 用法、禁令落地为 `check-e2e-listener-hooks.mjs`、`FolderTree.spec.js` 单测计数工具属另一语义（见 Out of Scope）不并入。

## Acceptance Criteria

- [ ] `terminal-tabs.spec.js` 内不存在自写 `addEventListener` 覆写；全部计数经 helper 读取
- [ ] `terminal-tabs.spec.js` 既有断言语义不变（全屏注册 +1、退出 -1、再次全屏 +2、弹窗 ESC 后再 -1/-2 链路完整）
- [ ] `node scripts/check-e2e-listener-hooks.mjs --selftest` 全通过；正式检查在干净工作树通过
- [ ] 手工注入一条违规样例（临时文件）时检查器失败并定位到 `file:line`（selftest 覆盖即可）
- [ ] `frontend/e2e/terminal-tabs.spec.js` 连跑 3 次无 flaky；`npm run e2e` 全量 1 次全绿
- [ ] `npm test -- --run` 全绿，前端覆盖率不低于 70% 门禁
- [ ] `docs/spec/e2e-testing.md` §12.1 已更新

## Definition of Done

- 单元/E2E 测试按上述补充；无新增 `waitForTimeout`、无裸 `@playwright/test` import
- CI 配置与 npm script 同步
- 文档同步（e2e-testing.md §12.1）
- 收尾时确认 `README.md` 无需更新

## Technical Approach

### helper 设计（`frontend/e2e/listenerCounter.js`）

```js
// spec 侧用法
import { test, expect } from './fixtures'
import { installListenerCounter, listenerSnapshot, listenerDelta } from './listenerCounter'

await installListenerCounter(page, ['keydown'])   // goto 之前
const panel = await openTerminal(page)            // 内部 goto('/')
const base = await listenerSnapshot(page)         // { keydown: { added, removed } }
...
const d = await listenerDelta(page, base)         // { keydown: { added, removed } }
expect(d.keydown.added).toBe(1)
```

- 页面内统计结构：`window.__listenerStats = { [type]: { added, removed } }`，单一覆写实现按 type 累加。
- 幂等：init script 内 `if (window.__listenerCounterInstalled) return`，重复 `installListenerCounter` 不重复包装。
- 不在 helper 内做断言、不读 `expect`——保持纯数据，断言留在 spec。

### 守护脚本设计（`scripts/check-e2e-listener-hooks.mjs`）

- 纯 Node（`fs`/`path`/`process`），零依赖零网络，与 `check-dep-consistency.mjs` 同风格。
- 递归收集 `frontend/e2e` 下 `.js` 文件（跳过 `listenerCounter.js`），逐行正则匹配禁令形态；命中收集 `{file, line, snippet}`，非空即退出 1。
- `--selftest` 用 `fs.mkdtempSync` 造 fixture：一个违规文件（三种形态各一行）+ 一个合规文件（普通 `window.addEventListener('keydown', fn)` 不绑定、不覆写）+ helper 本体文件（放行），断言「违规必拦、合规必过」。
- 退出码：0 通过 / 1 违规（与 CI 失败语义一致）。

### 为什么选 R1（窄而硬）

R2 启发式（计数变量 + `toBe(整数)`）在仓内会命中数十处 `getWailsCalls(...).length).toBe(1)` 正常断言，误报不可收敛；R1 只禁一种可机械识别的实现形态，误报近零，且把「怎么写」锁死在 helper 内。

## Decision (ADR-lite)

**Context**：§12 事故根因是合法新增监听注册点导致绝对基线断言失效。存量用例已改相对增量，但禁令仅存在于文档，新用例仍可写回内联 hook + `toBe(0)`；同时 helper 不可复用，新计数场景只能再抄一份。

**Decision**：A + R1——抽公共 helper 并把相对口径编码进 API，同时以窄规则静态检查禁止 E2E 自写监听覆写 hook，接入 CI。

**Consequences**：新增一次 CI 检查（毫秒级）；未来若确实需要更复杂的监听观测（如 per-listener 身份追踪），须扩展 helper 而非绕过。漏检面：`EventTarget.prototype` 之外的间接包装（如第三方库内部），已在禁令中覆盖最可能的形态。

## Scope Amendment（2026-11-20，用户批准 A）

实现完成后的全量 E2E 发现**既有**失败：`frontend/e2e/sidebar-shortcuts.spec.js:37` 的 `DEFAULT_ORDER` 仍为 9 项、`:123` 断言 `toHaveLength(9)`，而 `f0cb6e5`（提示词面板）已在 `frontend/src/store/settings.js:48` 新增第 10 个面板 `prompts`，该 spec 在 master 上即为红。

用户批准本任务顺手修复（仅测试文件，约 2 行）：`DEFAULT_ORDER` 末尾补 `'prompts'`、`toHaveLength(9)` 改为 10；不改业务代码、不改 Alt 映射语义（Alt+1..9 仍对应默认顺序前 9 项，Alt+9 仍指向临时待办）。修复后须给出「全量 `npm run e2e` 全绿」的真实结论。

## Out of Scope

- 后端 Go 代码、`frontend/wailsjs/`、`wails-mock-defaults.js` 数据形状
- `getWailsCalls(...).length).toBe(...)` 等页面内确定性调用日志断言（不属同类脆弱面，批量改写会削弱断言强度）
- `frontend/src/components/__tests__/FolderTree.spec.js:623-707` 的单测计数工具：其 `expect(counter.added).toBe(0)` 是 §12.2「常驻组件禁常驻注册」的**设计校验**，语义与「增量配对」不同，本次不迁移、不合并；仅在文档注明差异
- `wails-mock.spec.js:86/90` 的 `__wailsEventHandlers` 绝对断言（独占探针事件名，无跨用例污染面）
- 新增 `Alt+数字` 类快捷键或面板改动

## Technical Notes

- 关键证据位置：
  - `frontend/e2e/terminal-tabs.spec.js:153-225`（现相对增量实现与注册点注释）
  - `frontend/e2e/fixtures.js:122-125`（导出面，spec 统一从这里取 `test/expect`）
  - `docs/spec/e2e-testing.md` §12.1 / §12.2 / §12.3
  - `scripts/check-dep-consistency.mjs`（零安装检查脚本 + `--selftest` + `<dir>` 参数先例）
  - `.github/workflows/ci.yml:52-57`（两步检查挂载点）
  - `frontend/package.json`（`scripts` 段）
- 约束：禁 `waitForTimeout`；E2E 从 `frontend/e2e/fixtures.js` import `{ test, expect }`；新增 spec 目录同步 vitest `test.exclude`（本任务不新增 spec 目录）；spy 须 `afterEach` restore。
- 已有 E2E 计数只覆盖 `keydown`；本 helper 首版即支持多类型，但本次只迁移 `keydown` 用例，其余类型暂无消费点（保持 API 通用、不为假想需求加断言）。
- **新增检查工具的授权依据**：`docs/agent-resources/conventions/process-build-quality.md` 规定「严禁未经用户授权擅自引入新检查工具或改写构建配置」。本任务新增 `check-e2e-listener-hooks.mjs` 与 CI 步骤已获用户明确授权（2026-11-20 brainstorm 选 A + R1 并确认 PRD），只做「加一条检查」，不改动/不替换任何既有门禁。

## 实现偏差与残余风险（2026-11-20）

- **API 形态**：实现为 `listenerSnapshot(page)` / `listenerDelta(page, base)` 返回按类型分组的映射，与初稿示例的「带 type 入参」不同；满足「只暴露相对口径 + 多类型分组」硬要求，本 PRD 示例与 `docs/spec/e2e-testing.md` 均按实现书写。
- **覆盖率插桩下的既有时序**：`frontend/src/components/__tests__/FileHistoryDialog.spec.js:229` 在 `npm run test:coverage` 下偶发 5s 超时；裸跑 `npm test -- --run` 稳定通过。该文件不在本任务改动面，未处理。
- **业务注释过期**：`frontend/src/store/settings.js:78` 注释仍写「当前 9 个面板」（实际 10 个）。PRD 明确不改业务代码，未处理，建议后续顺手更正。
- **守护覆盖面（独立审核后扩展）**：已覆盖 `window` / `globalThis` / `self` / `Window.prototype` 的赋值覆写、`addEventListener.bind(window)` / `removeEventListener.bind(window)`、`EventTarget.prototype.addEventListener`；扫描扩展名覆盖 js/ts/jsx/tsx/mjs/cjs/mts/cts（`SCRIPT_FILE_RE`）。
- **仍接受的漏检面**：`Object.defineProperty(window, 'addEventListener', ...)`、`window['addEventListener'] =`、`Reflect.set` 等间接形态；逐行匹配不检测跨行赋值、不剥离注释与字符串（注释中引用禁令字样会误报）。出现新形态时扩展检查器并补 selftest。
- **计数口径**：helper 统计 add/remove **调用次数**而非有效注册数——原生对同 type+listener+capture 去重、对未注册监听的 remove 为 no-op，均各计 1 次；`TerminalPanel.vue` 全屏 watch 的 `immediate` 挂载会产生一次 no-op remove，因基线在挂载后取而不影响断言。本用例内调用与有效注册成对等价；该语义已写入 helper 头注释与 §12.1。
- **基线约束**：`listenerDelta` 对空基线、非对象基线、缺少某类型统计一律抛错（不退化按 0 起算）；基线与增量必须在同一 document 内读取（goto/reload 后统计归零）。
- **绝对断言残余通道**：`listenerSnapshot` 返回绝对计数，理论上仍可直接写 `expect((await listenerSnapshot(page)).keydown.added).toBe(0)`，静态守护识别不了（它只拦覆写 hook）。这是 API 设计取舍：snapshot 必须可序列化才能作基线；依靠「snapshot 仅作 delta 基线」的文档约定 + review 把关，不进一步封装为不透明对象。
