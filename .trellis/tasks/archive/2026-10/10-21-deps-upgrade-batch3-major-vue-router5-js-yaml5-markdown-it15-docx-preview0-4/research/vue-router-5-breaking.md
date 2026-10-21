# Research: vue-router 4→5 major 升级 breaking changes 与本仓适配面

- **Query**: 调研 vue-router 4→5 major 升级的 breaking changes 与本仓适配面
- **Scope**: mixed（本仓内部检索 + vue-router 官方迁移指南/GitHub release notes/npm registry）
- **Date**: 2026-10-21

## 现版本与目标版本

| 项 | 本仓现状 | 目标 | 备注 |
|---|---|---|---|
| vue-router | `~4.6.4`（v4 线最终版，v5.0.0 的官方 compare 基线即 v4.6.4） | `~5.4.0`（2026-10-07 发布，v5 线最新） | tilde 档声明 |
| vue | `~3.5.42` | 不动 | 满足 v5.4.0 peer `^3.5.34 \|\| ^4.0.0` |
| vite | `^8.0.10` | 不动 | 满足 v5.4.0 optional peer `^7.3.0 \|\| ^8.0.0` |
| pinia | `~4.1.0` | 不动 | 满足 v5.4.0 optional peer `^3.0.4 \|\| ^4.0.2` |

关键背景（官方迁移指南原话）：**Vue Router 5 是一个过渡版本（transition release），核心变更是把 unplugin-vue-router（文件式路由）合并进核心包。未使用 unplugin-vue-router 的 Vue Router 4 用户没有 breaking changes，无需任何代码改动即可升级。** 官方同时预告：Vue Router 6 将是 ESM-only 并移除 deprecated API，v5 正是为该过渡预留的准备版本。

## 本仓消费 API 清单

消费面极小，全仓仅以下位置触碰 vue-router：

| 文件 | 用到的 API / 组件 | 说明 |
|---|---|---|
| `frontend/src/router/index.js`（唯一 import 源） | `createRouter`、`createWebHashHistory`（均无参）、单条静态路由表（`path: '/'` + `name: 'Home'` + 懒加载 `component: () => import(...)`） | 无守卫、无 meta、无动态路由、无 scrollBehavior、无 params/query 传参 |
| `frontend/src/main.js:32` | `app.use(router)` | 标准插件装配 |
| `frontend/src/App.vue:3` | `<router-view />` | 裸组件，无 scoped slot、无 keep-alive 包裹 |
| `frontend/src/__tests__/App.spec.js:19` | stub `'router-view'` 为空 div | vitest 侧无 vue-router 真实依赖 |
| `frontend/e2e/smoke.spec.js:14` | 断言 hash 路由就位（`#/`） | 走真实应用，不 mock router |

**确认不存在的消费**（grep 全仓核实，vitest 的 `beforeEach/afterEach` 生命周期钩子除外，与路由守卫无关）：

- Composition API：`useRouter` / `useRoute` —— 零使用
- 组件：`<router-link>` / `<RouterLink>` —— 零使用
- 编程式导航：`router.push` / `router.replace` / `router.go` / `router.back` —— 零使用
- 实例属性：`$route` / `$router` —— 零使用
- 路由信息：`route.params` / `route.query` / `route.meta` —— 零使用
- 守卫：`beforeEach` / `afterEach`（路由语境）—— 零使用
- 动态路由：`addRoute` / `removeRoute` —— 零使用
- 配置项：`scrollBehavior` / 自定义 param parsers / `parseQuery` / `stringifyQuery` —— 零使用
- 测试基建：vitest 配置与 `src/test/` 无任何 vue-router mock 或特殊处理

## v5 breaking changes 逐条

### 1. 官方定性：无 breaking changes（针对本仓这类用户）

来源：官方迁移指南 [Migrating to Vue Router 5](https://router.vuejs.org/guide/migration/v4-to-v5.html) 与 [v5.0.0 release notes](https://github.com/vuejs/router/releases/tag/v5.0.0)（2026-01-29 发布）。

> "Vue Router 5 is a _boring_ release, it merges unplugin-vue-router into the core package **with no breaking changes**. The only exception is that the _iife_ build no longer includes `@vue/devtools-api`..."
>
> "If you're using Vue Router 4 **without unplugin-vue-router, there are no breaking changes** - you can upgrade without any code modifications."

官方指南给出的迁移清单（Remove unplugin-vue-router / 改 vite 插件 import / 改 data loaders import / 改 Volar 插件 / 删 client 类型引用）**全部只针对 unplugin-vue-router 用户**，本仓未使用该插件，清单逐条均不适用。

主入口 `vue-router` 的导出（`createRouter`/`createWebHashHistory`/`useRouter`/`useRoute`/`RouterLink`/`RouterView` 等）官方标注为 **"Main API (unchanged)"**。本仓消费的 3 个 API 全部落在该范围内。

### 2. 唯一的非 experimental 行为变更：iife 构建不再内嵌 @vue/devtools-api

- `@vue/devtools-api` 升级到 v8 后自身不再提供 IIFE 构建，故 v5 的 iife 产物（`vue-router.iife.js`，面向 CDN `<script>` 标签直引场景）不再包含它。
- **本仓判定：不受影响。** 本仓走 Wails + Vite 的 ESM 打包链，不消费 iife 构建。但注意：v5 将 `@vue/devtools-api` 从 v4 的 `^6.6.4` 变更为运行时 dependency `^8.1.5`（npm registry 两个版本对比核实），Vite 会将其打进依赖图；无 Vue devtools 连接时（Wails WebView 生产运行态）该库为 no-op 桥，属低风险，建议 `wails dev` 冒烟覆盖（devtools 可用场景）。

### 3. 5.x 全线带 `!`（breaking）标记的条目——全部限定在 `(experimental)` 范畴

逐条核查 v5.0.0 → v5.4.0 官方 CHANGELOG（`packages/router/CHANGELOG.md`），带 `!` 的条目共 4 条：

| 版本 | 条目 | 范畴 | 本仓判定 |
|---|---|---|---|
| 5.0.0-beta.1 | `fix(experimental)!: query params are optional by default` | 实验性 param parsers / typed routes 的类型语义 | 不受影响（本仓无 param parsers、不用 query） |
| 5.1.0 | `refactor(experimental)!: remove selectNavigationResult` | 实验性 data loaders API | 不受影响（本仓无 data loaders） |
| 5.1.0 | `refactor(experimental)!: make miss() throw internally and return never` | 实验性 data loaders API | 不受影响（同上） |
| 5.4.0 | `feat(experimental)!: normalize query values to arrays` | 实验性范畴的 query 归一化 | 不受影响（本仓不用 query） |

### 4. 其余值得知晓的行为修正（bug fixes，非 breaking，逐条判定）

| 条目 | 本仓判定 |
|---|---|
| `Runtime error on missing param parsers`（v5.0.0 feature） | 不受影响（本仓路由未定义任何自定义 param parser） |
| keep-alive 组件跨路由复活时触发导航守卫（v5.0.0 fix #2604） | 不受影响（App.vue 的 router-view 无 keep-alive 包裹，且本仓无路由守卫） |
| 路径中 `~` 转义修复 | 不受影响（路由 path 仅 `/`） |
| "Avoid breaking older browsers support"（browserslist 兼容回退修复） | 不受影响（Wails WebView2 为 Chromium 内核，非旧浏览器场景） |

### 5. peerDependencies 约束（升级的实际阻力点，均已满足）

npm registry 核实（v5.0.0 与 v5.4.0 对比）：

| peer 依赖 | v5.0.0 要求 | v5.4.0 要求 | optional | 本仓 | 判定 |
|---|---|---|---|---|---|
| vue | `^3.5.0` | `^3.5.34 \|\| ^4.0.0`（**5.1.0 起从 ^3.5.0 提高**） | 否（必选 peer） | `~3.5.42` | 满足 |
| vite | 无 | `^7.3.0 \|\| ^8.0.0` | 是 | `^8.0.10` | 满足 |
| pinia | `^3.0.4` | `^3.0.4 \|\| ^4.0.2` | 是 | `~4.1.0` | 满足 |
| @vue/compiler-sfc | `^3.5.17` | `^3.5.34 \|\| ^4.0.0` | 是 | 未直装（vite 插件链带入） | 满足 |

注意：vue-router 包自身无 `engines` 声明；其运行时依赖链（chokidar v5、unplugin v3 等，仅被 `vue-router/vite`、`vue-router/unplugin` 子入口消费）要求 node ≥ 20，CI 的 node 22 满足。这些 unplugin 相关依赖本仓不 import 对应入口，不会进入最终 bundle，仅 node_modules 体积变大。

## 本仓影响判定表

| # | 本仓消费点 | v5 对应变更 | 判定 |
|---|---|---|---|
| 1 | `createRouter({ history, routes })` | Main API unchanged | 不受影响，零改动 |
| 2 | `createWebHashHistory()`（无参） | Main API unchanged（hash 模式语义不变） | 不受影响，零改动 |
| 3 | 静态路由表 + 懒加载 `component: () => import(...)` | unchanged | 不受影响，零改动 |
| 4 | `app.use(router)` | unchanged | 不受影响，零改动 |
| 5 | `<router-view />` | unchanged | 不受影响，零改动 |
| 6 | E2E 冒烟断言 `#/` hash 就位 | hash 模式行为不变 | 不受影响（升级后回归该用例即可） |
| 7 | vitest（无 router mock，App.spec stub router-view） | import 链不变 | 不受影响 |
| 8 | 依赖树：`@vue/devtools-api` ^6.6.4 → ^8.1.5 | 唯一的非 experimental 变更 | 不受影响（ESM 打包链），dev 冒烟覆盖 |
| 9 | peer 依赖 vue/vite/pinia | 5.1.0 起 vue 要求提高到 ^3.5.34 | 已满足（vue ~3.5.42），无需连带升级 |
| 10 | 依赖分档钳制清单（`scripts/check-dep-consistency.mjs`） | vue-router 声明 `~4.6.4` → `~5.x` | **需适配（流程面）**：按 frontend-dep-upgrade 规则先在四清单中更新 vue-router 归档再改 package.json，否则 CI 步骤 2.5 失败 |

## 结论建议

**建议升级（v4.6.4 → v5.4.0，预期零代码改动）。**

理由：

1. **官方明确声明无 breaking changes**：本仓不属于 unplugin-vue-router 用户，迁移指南的适配清单逐条均不适用；官方原话为"升级依赖即完成"。
2. **本仓消费面与变更面零交集**：全部消费仅 `createRouter`/`createWebHashHistory`/`<router-view>` 三点，均属官方标注 unchanged 的主 API；5.x 全线 4 条带 `!` 条目全部在 experimental（data loaders / param parsers / typed routes）范畴，本仓一概未用。
3. **peer 依赖全部现成满足**：vue ~3.5.42 ≥ 5.1.0 起要求的 ^3.5.34，vite 8、pinia ~4.1.0 均达标，无连带升级成本。
4. **v5 是 v6 的过渡版**：v6 将 ESM-only 并移除 deprecated API，本仓无任何 deprecated API 使用，早升 v5 为后续 v6 铺路，且窗口期风险最低。

代价与验证面：

- 改动仅 `frontend/package.json`（`vue-router` 声明改 `~5.4.0`，按分档规则先更新 `scripts/check-dep-consistency.mjs` 登记）+ `package-lock.json` 重新安装。
- 验证：`npm test`（vitest 全量）、`npm run e2e`（重点 `smoke.spec.js` hash 路由断言）、`wails dev` 冒烟一次（覆盖 @vue/devtools-api v8 依赖树变更的 devtools 场景）。
- 唯一存疑点（低风险、冒烟可覆盖）：`@vue/devtools-api` v6→v8 在 Wails WebView 内的运行时行为，理论 no-op，无真机劣化先例可查，按上述冒烟兜底。

## Caveats / Not Found

- 本仓 `frontend/package.json` 中 vue 实际为 `~3.5.42`，与 CLAUDE.md 技术栈表所写 3.5.33 不一致（文档滞后），本文以 package.json 为准，建议主 agent 顺手勘误 CLAUDE.md。
- `wails dev` 真机冒烟（devtools-api v8 场景）未在本调研中执行，属实现阶段验证项。
- vue-router v6 尚无正式 release notes，"v6 将移除 deprecated API"仅引用官方迁移指南的前瞻表述，不构成本次升级依据。

## 参考来源

- 官方迁移指南：https://router.vuejs.org/guide/migration/v4-to-v5.html
- v5.0.0 release notes：https://github.com/vuejs/router/releases/tag/v5.0.0
- v5.x CHANGELOG：https://github.com/vuejs/router/blob/main/packages/router/CHANGELOG.md
- npm registry：vue-router@5.4.0 / 5.2.0 / 5.1.0 / 5.0.0 / 4.6.4 的 peerDependencies、dependencies、peerDependenciesMeta
