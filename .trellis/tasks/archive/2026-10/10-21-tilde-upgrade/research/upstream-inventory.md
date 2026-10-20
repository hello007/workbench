# tilde 档 28 项上游差距盘点（2026-10-21）

> 查询：npm view --registry=https://registry.npmjs.org（官方 registry，npmmirror 同步延迟归零）。

## 升级批（7 项）

### patch 批（5 项，tilde 范围内 patch 漂移，npm update 自动，声明不动）

| 包 | lock | latest | 说明 |
|---|---|---|---|
| @codemirror/commands | 6.11.0 | 6.11.1 | patch |
| @codemirror/state | 6.7.4 | 6.7.6 | patch |
| @codemirror/view | 6.43.11 | 6.43.15 | patch（4 个 patch 累积） |
| vue | 3.5.42 | 3.5.44 | patch |
| vue-echarts | 8.3.0 | 8.3.2 | patch |

### minor 批（2 项，须显式改声明 `~x.(y+1).0`，升前查 changelog）

| 包 | lock | latest | 声明变更 |
|---|---|---|---|
| @codemirror/language | 6.12.4 | 6.13.1 | ~6.12.4 → ~6.13.0 |
| pinia | 4.0.3 | 4.1.0 | ~4.0.3 → ~4.1.0 |

## 跳过批（5 项，留痕）

| 包 | lock | latest | 跳过理由 |
|---|---|---|---|
| js-yaml | 4.3.2 | 5.5.0 | major 4→5，Out of Scope（须专项评估 API 破坏面） |
| markdown-it | 14.3.2 | 15.0.3 | major 14→15，同上 |
| mermaid | 11.17.2 | 12.2.0 | major 11→12，同上（渲染面核心库，风险更高） |
| vue-router | 4.6.4 | 5.4.0 | major 4→5，同上（vue-router 5 为 Vue 生态大版本迁移） |
| docx-preview | 0.3.7 | 0.4.1 | 0.x 包 minor 语义 = 破坏性风险，与 major 同对待（tilde 已全锁） |

## 已最新（16 项，不动）

@codemirror/lang-css 6.3.1、lang-html 6.4.12、lang-javascript 6.2.5、lang-json 6.0.2、lang-markdown 6.5.2、lang-sql 6.10.0、codemirror 6.0.2、dayjs 1.11.23、@xterm/addon-fit 0.11.0、@xterm/addon-web-links 0.12.0、@xterm/xterm 6.0.0、echarts 6.1.0、highlight.js 11.12.0、splitpanes 4.1.2、vue-draggable-plus 0.6.1

## 升级策略

- 一次升级动作（7 项合并，批间无依赖冲突），一次全量必跑面
- 操作链：patch 批 `npm update <5 项>` + minor 批改 package.json 声明 → `npm install --package-lock-only` → `npm ci` 验证可复现 → 必跑面（一致性检查 / vitest 全量 / E2E web+desktop / npm audit）
- 升级后 lock 落点核对：minor 批声明 `~6.13.0`/`~4.1.0` 的 lock 落点须满足（若 minor 批 npm 解析出更高 patch，声明仍满足）

## 升级结果与 changelog 检查结论（2026-10-21，升级执行后追加）

### changelog 检查结论（升前逐项核对，7 项均无 breaking）

| 包 | 检查来源 | 结论 |
|---|---|---|
| pinia 4.1.0 | GitHub Releases（vuejs/pinia） | **无 breaking**：新增 `getStores()` API（可选消费）；「dispose 被 replace 的 store」为 dev 告警（仅错误用法触发，`createTestingPinia` 正规用法不受影响）；其余为 shallow refs patch 触发 / hmr / nuxt / testing 的 bug 修复。本仓 6 基 Pinia setup store 常规消费面不受影响 |
| @codemirror/language 6.13.0/6.13.1 | npm tarball 包内 CHANGELOG（仓库 master 分支 CHANGELOG 滞后于 npm 发布，6.12.3 后未更新） | **无 breaking**：6.13.0 将 StreamParser 相关类型拆分到新包 `@codemirror/streamparser` 并 re-export（API 向后兼容）；6.13.1 修正 streamparser 为正式 dependency。副作用：lock 新增传递依赖 @codemirror/streamparser 6.0.0 |
| @codemirror/commands 6.11.1 | npm tarball 包内 CHANGELOG | **无 breaking**：`cursorLineEnd`/`cursorLineStart` 改用 `moveToLineBoundary` 实现 bidi 感知（bug fix） |
| @codemirror/state 6.7.5/6.7.6 | npm tarball 包内 CHANGELOG | **无 breaking**：range set 比较误重绘修复、`changeByRange` 产出光标与插入文本关联修复 |
| @codemirror/view 6.43.12-6.43.15 | npm tarball 包内 CHANGELOG | **无 breaking**：全部 bug fixes（滚动事件与选区错位、bidi isolate 嵌套解析、`showDialog` promise 不 resolve、`scrollbar-gutter` 下 tooltip 定位、height scaling 除零等） |
| vue 3.5.43/3.5.44 | GitHub 仓库 CHANGELOG.md（vuejs/core） | **无 breaking**：两版均为纯 Bug Fixes（reactivity watcher 嵌套追踪、shallow collection 语义、KeepAlive 卸载、hydration 等） |
| vue-echarts 8.3.1/8.3.2 | GitHub Releases（ecomfe/vue-echarts） | **无 breaking**：全部 fixes（autoresize 在分数像素容器中断动画、TS 声明错误、graphic 跨组移动、flex-column 收缩等） |

### 升级执行结果

| 包 | 声明（升前 → 升后） | lock 落点 |
|---|---|---|
| @codemirror/commands | ~6.11.0（不变） | 6.11.0 → **6.11.1** |
| @codemirror/state | ~6.7.4（不变） | 6.7.4 → **6.7.6** |
| @codemirror/view | ~6.43.11（不变） | 6.43.11 → **6.43.15** |
| vue | ~3.5.42（不变） | 3.5.42 → **3.5.44** |
| vue-echarts | ~8.3.0（不变） | 8.3.0 → **8.3.2** |
| @codemirror/language | ~6.12.4 → **~6.13.0** | 6.12.4 → **6.13.1** |
| pinia | ~4.0.3 → **~4.1.0** | 4.0.3 → **4.1.0** |

- lock 新增传递依赖：`@codemirror/streamparser 6.0.0`（@codemirror/language 6.13.1 正式依赖）；`@pinia/testing` 保持 2.0.1（caret 档未漂移）
- 传递依赖附带漂移（caret 范围内自动）：postcss 8.5.28→8.5.31，带动 nanoid 3.3.20 / source-map-js 1.2.2 / @babel/parser 7.29.9（node engine 告警为升级前既有现状，仅 warn 不 fail）
- `npm ci` 可复现通过；`npm run build` 通过

### 必跑面结果（SOP §3.3 全项）

| 项 | 结果 |
|---|---|
| check-dep-consistency --selftest | 11 用例全过 |
| check-dep-consistency | 三维一致绿 |
| vitest 全量 | 96 文件 / **2022 用例全过** |
| E2E web 全量 | **72 passed**（46.2s） |
| E2E desktop 全量 | **3 passed**（wails build 重新构建后跑，exe 内嵌升级后前端 bundle） |
| 哨兵 5 spec 显式跑 | **32 passed** |
| npm audit（官方 registry） | **无新增 high**：6 vulnerabilities (3 low, 3 high)，对比 HEAD 基线 7 (3 low, 4 high)——少 1 个 high（source-map-js 在 caret 范围内 patch 升至 1.2.2，漏洞版本区间不再命中），剩余 5 组均为升前既有且在本次范围外（dompurify/katex 系 mermaid 11 传递、markdown-it 修复在 15 major、vite 系 caret 工具链、brace-expansion 系 @vue/test-utils dev 传递） |

### audit 剩余 high 与范围外升级项对应关系（留痕）

| 漏洞包 | 引入链 | 对应范围外项 |
|---|---|---|
| dompurify <=3.4.15 | mermaid 11.17.2 传递 | mermaid 12 major 专项 |
| katex 0.11.0-0.18.1 | mermaid 11.17.2 传递（audit fix --force = mermaid@12） | mermaid 12 major 专项 |
| markdown-it <=14.3.2 | 直接依赖 tilde 档 | markdown-it 15 major 专项 |
| vite 8.3.0-8.3.2（launch-editor） | caret 档工具链 | caret 档不动 |
| brace-expansion 4.0.0-5.0.11 | @vue/test-utils → js-beautify → editorconfig → minimatch | dev 传递依赖，非运行时面 |
