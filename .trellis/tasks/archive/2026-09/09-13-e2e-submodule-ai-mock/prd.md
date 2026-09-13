# E2E 测试扩展：submodule 管理、文件树操作、AI 功能触发（mock 外部服务）

## Goal

扩展 WorkBench E2E 测试覆盖至三个高频用户流程：文件树操作（新建/重命名/删除）、submodule 管理（前端 E2E + 后端集成测试）、AI 功能触发（mock 外部服务的流式/异步行为）。全部遵循 `docs/spec/e2e-testing.md` 方案 C 混合架构既有约定，用户选型为 A+B+C 全做。

## What I already know

- 上期任务 09-13-e2e-key-flows 已落地 E2E 体系：Playwright + vite preview + mock Wails 后端；`wails-mock-defaults.js` 为 vitest/E2E 单一数据源；`fixtures.js` 支持 `wailsOverrides` per-test 覆盖；`__wailsCalls` 断言 bound method 调用
- 文件树 bound method（`CreateFile`/`RenameFile`/`DeleteFile`/`CreateDirectory`）mock 默认值已在基础表，仅需补 `GetFileTree` 具体返回
- `FileTreePanel.vue` 交互入口：右键节点菜单（createFile/createDir/rename/delete 等）+ 右键空白区菜单（createFile/createDir）+ 创建/重命名对话框
- submodule bound methods：`GetSubmodules` / `UpdateSubmodules` / `AddSubmodule` / `RemoveSubmodule` / `CheckoutSubmoduleBranch`；`GitSubmodules.vue` 工具栏（添加/初始化/更新）+ 行内（切换跟踪分支/更新/删除）；「初始化」实际走 `UpdateSubmodules(path,'checkout',true,true,'')` 而非 `InitSubmodules`（源码注释：仅注册不检出）
- AI 执行链路：`RunAiFunction(functionID, params)` 返回 taskID，后端按序 emit `ai-task:queued` → `ai-task:started` → `ai-task:output`（流式多条）→ `ai-task:done`；`AiFunctionPanel.vue` 以 `EventsOn` 订阅四个事件
- 现有 `wails-init.js` 的 `runtime.EventsOn` 为空实现（只注册不派发），AI 事件流测试需扩展注入脚本
- 后端集成测试 fixture 模式已就绪（`git_flows_integration_test.go`：`itInitRepo` / `itBareRemote` / `itCommitAll` 等），service 层 submodule 解析逻辑已有单测（`git_submodule_test.go`）
- 用户提及的「.modules 目录」实为 git 标准 `.git/modules/<path>`（submodule clone 存放处）；集成测试用真实 `git submodule add` 产生真实结构，天然规避 fake 差异问题

## Requirements

### A. 文件树操作 E2E（新增 `frontend/e2e/file-tree.spec.js`）

- `GetFileTree` 具体返回值登记到 `WAILS_MOCK_E2E_EXTRA_RETURN_VALUES`（层级树形数据）
- 用例：
  1. 右键文件节点 → 菜单新建文件 → 对话框填名确认 → `CreateFile` 参数正确 + 成功提示 + `GetFileTree` 刷新（二次调用）
  2. 右键空白区 → 新建目录 → `CreateDirectory` 参数正确
  3. 右键节点 → 重命名 → `RenameFile(oldPath, newPath)` 参数正确
  4. 右键节点 → 删除 → 确认弹窗 → `DeleteFile` 参数正确
  5. 失败路径：`CreateFile` override 报错 → 错误提示出现且树不刷新
- 注意：实现时核对组件实际调用的 bound method 签名（`App.d.ts`）与删除确认交互形态

### B. Submodule E2E（新增 `frontend/e2e/submodule.spec.js`）+ 后端集成测试

E2E：
- `GetSubmodules` 返回值登记补充表（形状对齐 `model.GitSubmodule`：path/sha/shortSha/branch/url/initialized/shaMismatch/dirty/conflict/describe）
- 用例：
  1. 列表渲染：路径/分支/SHA/初始化态标记/脏标记
  2. 添加子模块对话框 → `AddSubmodule(url, path, branch)` 参数正确
  3. 行内「更新」→ `UpdateSubmodules(path, mode, recursive, init, subPath)` 参数正确（含 mode 选项）
  4. 工具栏「初始化」→ `UpdateSubmodules(path, 'checkout', true, true, '')`
  5. 行内「删除」→ 确认 → `RemoveSubmodule` 参数正确
  6. 失败路径：更新报错 → `handleGitError` 错误提示
- 未初始化 submodule 行操作按钮的可用性断言（以组件实际实现为准）

后端集成测试（`git_flows_integration_test.go` 追加）：
- 新 fixture `itSetupSubmoduleRepo`：父仓库 + 子仓库，`git submodule add <本地子仓库路径>`（免网络），提交引入
- 新用例 `TestIntegration_SubmoduleFlow`：走 App 链验证 List（解析 .gitmodules/status/porcelain 融合）→ Update（checkout 模式）→ Add → Remove 全链
- fixture 注意：真实 `git submodule add` 产生 `.git/modules/<path>` 真实 clone 结构，无需 fake；沿用既有确定性约定（symbolic-ref、局部 user.name/email、autocrlf=false）

### C. AI 功能触发 E2E（新增 `frontend/e2e/ai-function.spec.js`）

`wails-init.js` 扩展（保持 addInitScript 自包含约束）：
- `runtime.EventsOn` 改为记录回调至页面全局 `window.__wailsEventHandlers`（按事件名分组）
- 新返回值描述符 `{__events__: [{ event, payload, delayMs? }]}`：bound method 被调用后经 `setTimeout` 按序派发事件到已注册回调
- 描述符在 `e2e-testing.md` 第 6 节扩展登记

mock 数据（登记 `WAILS_MOCK_E2E_EXTRA_RETURN_VALUES`）：
- `RunAiFunction` 默认返回 taskID + 成功事件序列（queued/started/output×N/done）
- `GetAiTaskState` 返回形状对齐 `model.AiTaskState`
- `GetAiFunctions` 返回含一个可运行功能的列表（现有空数组改造或 override）

用例：
1. 点击运行 → `RunAiFunction(functionID, params)` 参数正确（含参数输入对话框场景）
2. 成功流状态机：运行中按钮禁用/状态标识 → 流式输出逐步渲染 → done 后完成提示/followUps 按钮（以组件实际行为为准）
3. 失败流：`__events__` 序列含失败 done（error 状态）→ 错误提示
4. 取消：运行中点击取消 → `CancelAiTask(taskID)` 参数正确
- 事件 payload 形状对齐 `service/ai_function.go` emit 数据结构（契约对齐）

## Acceptance Criteria

- [ ] 新增 E2E 用例全绿，本地 `retries=0` 连跑 3 次结果一致（非 flaky）
- [ ] 后端集成测试（`go test -tags=integration`）submodule 用例全绿
- [ ] 878 单测零回归，覆盖率门禁（model/server ≥80% + service ≥76% + util ≥40% + 前端 ≥70%）不破
- [ ] mock 纪律：返回值一律登记 `wails-mock-defaults.js`，无用例内硬编码；形状对齐 `App.d.ts`/`models.ts`
- [ ] 禁 `waitForTimeout`，全程 expect 自动重试
- [ ] 文档同步：`docs/spec/e2e-testing.md`（`__events__` 描述符 + 新用例清单）、`docs/测试策略.md`、`frontend/e2e/README.md`、README.md（如需）

## Definition of Done

- 全部测试链（go test / go test integration / vitest / E2E）本地全绿
- CI 全量测试链通过（新用例自动纳入，无需改 ci.yml）
- 文档更新完成

## Technical Approach

三层扩展复用既有模式，不引入新框架：
1. 文件树/submodule E2E 纯粹是「补充表登记 + 用例编写」，零机制改动
2. AI 事件流扩展 `wails-init.js` 注入脚本（EventsOn 回调注册 + `__events__` 描述符），机制改动收敛在 e2e 层，vitest 侧不受影响
3. 后端集成测试沿用 fixture 模式追加 submodule 场景

## Decision (ADR-lite)

**Context**：AI 功能触发依赖外部 claude CLI 子进程与流式事件，是否纳入 E2E 需评估成本。
**Decision**：用户选型 A+B+C 全做。AI 块通过 `__events__` 描述符 mock 事件序列，不 mock 真实子进程（子进程执行属 service 单测职责）。
**Consequences**：`wails-init.js` 复杂度上升（事件派发机制）；换来 AiFunctionPanel 异步状态机的 E2E 覆盖；后续终端等事件流场景可复用该机制。

## Out of Scope

- AI 真实子进程执行链路（service 层单测已覆盖）
- 终端事件流 E2E
- 剪贴板 syscall 行为验证
- `AiTaskHistoryPanel` 历史/导出流程

## Technical Notes

- `FileTreePanel.vue` 右键菜单命令：createFile/createDir/rename/delete/cut/copy/paste/copyTo/copyPath/openExplorer/openInVSCode/openInWarp
- `GitSubmodules.vue` 错误处理走 `handleGitError`（utils/gitError）
- AI 事件名常量：`ai-task:queued` / `ai-task:started` / `ai-task:output` / `ai-task:done`（`service/ai_function.go:359,393,1196,1244`）
- `wails-init.js` 注入函数自包含约束：只能访问入参与浏览器全局，入参须可结构化克隆（`docs/spec/e2e-testing.md` 第 4 节）
- 集成测试既有 fixture 命名前缀 `it*`，App 构造用 `itNewApp()`
