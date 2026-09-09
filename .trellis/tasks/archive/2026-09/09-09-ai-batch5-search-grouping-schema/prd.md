# AI 功能项搜索/分组/置顶 + 配置 schema 版本与迁移（第 5 批）

## Goal

合并实施 P2-2（功能项搜索/分组/置顶）与 P2-3（配置 schema 版本与迁移）。
功能项数量增长后保持高频功能快速触达：列表加模糊搜索（name/description/tags）、
tags 分组筛选、pinned 置顶；同时为 `data/ai_functions.json` 引入 `schemaVersion`
与加载期迁移机制，旧配置自动补全 tags/pinned 等新字段，校验失败备份回种 seed，
字段演进不再靠运行时处处判空。

P2-2 的 tags/pinned 字段升级直接走 P2-3 的 schema v2 迁移，合并做避免返工
（总览第 4 节依赖：P2-2→P2-3）。

## What I already know

### 现状（代码探查结论）

- `model/ai_function.go:5-20`：`AiFunction` struct 当前 13 字段，无 Tags/Pinned
- `service/ai_function.go:129-144`：`LoadAiFunctions` 当前 `util.LoadJSON(s.configPath, &funcs)`
  直接解析 `[]*AiFunction` 数组，空数组自愈写默认四项；无版本、无字段补全、无校验兜底
- `service/ai_function.go:147-149`：`SaveAiFunctions(funcs)` 当前 `util.SaveJSON(s.configPath, funcs)`
  写裸数组
- `service/ai_function.go:1067-1155`：`defaultAiFunctions` 四个 seed（speech-doc/weekly-report/
  meeting-book/meeting-list），未带 tags/pinned
- `AiFunctionPanel.vue:25-38`：`v-for f in functions` 平铺渲染 `.ai-card`，无搜索/分组/置顶
- `AiFunctionConfigDialog.vue:34-72`：基础字段区 10 项，无 tags/pinned 编辑
- **关键**：`LoadAiFunctions`/`SaveAiFunctions` 签名稳定可行——内部改读写
  `{schemaVersion, functions}` 结构，对外仍返回 `[]*model.AiFunction`，
  前端 `GetAiFunctions`/`SaveAiFunctions` 调用点与 wailsjs 绑定无需改动，
  跨层同步只剩 `models.ts` 加 Tags/Pinned

### 第 1-4 批已完成衔接点（勿重复改动）

- AiFunctionConfigDialog 四块表单 + MCP stdio + 第 4 批导入入口：本批只在基础字段区加 tags/pinned
- AiFunction/AiParamSpec/AiMcpConfig/SkillDescriptor model（第 2/4 批）：本批只给 AiFunction 加 Tags/Pinned
- LoadAiFunctions/SaveAiFunctions（第 2 批）：本批改读写结构（schema v2），保持签名稳定
- DirectoryService/ScanCacheManager/SkillDiscoveryService（第 4 批）：不动
- AiTaskState/AiTaskRunResult/AiTaskMetrics/AiTaskHistory/pumpOutput/GetAiTaskOutput/历史归档（第 1/3 批）：不动
- concurrencySem 并发上限（第 1 批）：不动

## Requirements

### P2-3 schema 版本与迁移（先做，P2-2 字段依赖其迁移补全）

1. model：`ai_functions.json` 顶层结构从 `[]*AiFunction` 升级为
   `{schemaVersion, functions}`（新增 `AiFunctionsConfig` struct）
2. service：`LoadAiFunctions`/`SaveAiFunctions` 改读写新结构；加载时按版本迁移：
   旧版本（无 schemaVersion 视为 v1）补全 tags/pinned 等字段到当前版本
3. 校验兜底：字段级校验（id/name/command/cwd 必填等），非法配置备份
   （`ai_functions.json.bak.<timestamp>`）后回种 seed，不整体不可用
4. 迁移对用户透明：保留既有合法配置，仅补全新字段

### P2-2 搜索/分组/置顶

5. model：`AiFunction` 增 `Tags []string` / `Pinned bool`（json omitempty）
6. `AiFunctionPanel.vue`：列表上方搜索框（name/description/tags 模糊匹配）；
   tags 分组（tag chips 筛选，方案待确认）；pinned 置顶排序
7. `AiFunctionConfigDialog.vue`：基础字段区加 tags（多标签输入）与 pinned（开关）编辑

### 跨层同步

8. `AiFunction` 加 Tags/Pinned 字段同步 `frontend/wailsjs/go/models.ts`
9. `LoadAiFunctions`/`SaveAiFunctions` 签名不变（已确认稳定），App.js/App.d.ts 无需改动

## Acceptance Criteria

- [ ] `ai_functions.json` 升级为 `{schemaVersion, functions}` 结构，旧配置加载时自动
  迁移补全 tags/pinned，不丢既有合法配置
- [ ] 非法配置（手改出错）备份后回种 seed，功能菜单可用不整体崩
- [ ] 列表搜索框按 name/description/tags 模糊匹配生效
- [ ] tags 分组筛选生效（方案按前置确认点 1 定）
- [ ] pinned 功能项始终排在列表最前
- [ ] 配置对话框可编辑 tags（多标签）与 pinned（开关），保存后生效
- [ ] `AiFunction` 的 Tags/Pinned 字段同步 models.ts；LoadAiFunctions/SaveAiFunctions
  签名不变（App.js/App.d.ts 无需改动）
- [ ] 前后端双绿（`go test ./...` && `cd frontend && npm test`），wailsjs 同步完成，
  git status 确认 frontend/wailsjs 无残留

## Definition of Done

- 后端单测：v1 数组兼容、字段补全（tags/pinned）、校验失败兜底（备份+回种）
- 前端单测：搜索过滤、pinned 置顶、tag chips 筛选
- `go test ./...` && `cd frontend && npm test` 双绿
- `models.ts` 同步 Tags/Pinned 字段（声明 + 构造函数赋值）
- `git status` 确认 `frontend/wailsjs/` 无残留（目录已 gitignore）
- README.md 视情况更新

## Technical Approach

### P2-3 schema v2 结构与迁移

```go
// model/ai_function.go 新增
const CurrentSchemaVersion = 2

type AiFunctionsConfig struct {
    SchemaVersion int              `json:"schemaVersion"`
    Functions     []*AiFunction    `json:"functions"`
}
```

`LoadAiFunctions` 加载流程：
1. 文件不存在 → 写默认 seed（包 `{schemaVersion: 2, functions: defaults}`）返回
2. 文件存在 → 先读原始 bytes，判顶层是数组（v1）或对象（v2+）：
   - v1（数组）：包一层，走 v1→v2 迁移补全 tags/pinned
   - v2+（对象）：取 schemaVersion，按版本走迁移链
3. 字段补全：单个 AiFunction 缺字段填默认（permissionMode→bypassPermissions，
   timeoutMinutes→10，tags→[]，pinned→false）
4. 字段级校验：id/name/command/cwd 必填，非法项剔除并记录 id
5. 校验失败兜底：备份原文件 `ai_functions.json.bak.<timestamp>`，
   全部非法则回种 seed，返回合法项（或 seed）

`SaveAiFunctions` 写 `{schemaVersion: CurrentSchemaVersion, functions: funcs}`。
保持签名 `(funcs []*model.AiFunction) error` 不变。

### P2-2 搜索/分组/置顶

`AiFunctionPanel.vue` 增计算属性：
- `allTags`：从 functions 聚合所有 tag（去重）
- `filteredFunctions`：搜索 keyword（name/description/tags 模糊匹配）+
  选中 tag 筛选 + pinned 优先排序
- 搜索框 clearable，tag chips 点击切换选中态

`AiFunctionConfigDialog.vue` 基础字段区：
- tags：逗号分隔输入（复用 addDirsText 模式），保存时 split 成 []string
- pinned：el-switch 开关

## Open Questions

（已全部决策，见下方 Decision）

## Decision (ADR-lite)

**Context**：design 第 6 节遗留 3 个前置待确认点，需实施前拍板。
**Decision**：均采用 design 推荐方案——
1. **分组方案**：tag chips 筛选。顶部展示所有 tag chip，点击切换筛选，与搜索框并存；
   平铺渲染命中项。折叠分组（方案 B）留待功能数超 15 再考虑，列入 Out of Scope。
2. **校验方式**：手写校验。id/name/command/cwd 必填等既有规则直接写 Go 代码，
   与前端 save() 校验一致；不引入 gojsonschema 依赖。
3. **备份策略**：仅校验失败时备份单份 `ai_functions.json.bak.<timestamp>`。
   正常版本迁移/字段补全不备份（透明补全，原配置仍合法）；仅当字段级校验
   发现非法项需回种 seed 时备份留底。不设保留份数上限（校验失败属异常，不会频繁触发）。

**Consequences**：
- tag chips 方案轻量，当前 4 项 seed 收益有限但为增长预留；后续转折叠分组需重写渲染层。
- 手写校验规则简单时足够，若未来字段规则复杂化（如正则/跨字段约束）需引入 gojsonschema。
- 单份备份在极端反复损坏场景下会被覆盖，但校验失败已回种 seed 保证可用性，旧文件
  仅作排查留底，可接受。

## Out of Scope

- 折叠分组渲染（方案 B，功能数超 15 再考虑）
- tags 预定义可选集枚举（自由输入 + 历史去重提示，本批仅自由输入）
- gojsonschema 引入（手写校验足够）
- 第 1-4 批已完成部分的任何改动
- 危险动作二次确认、多端访问、历史导出报告（总览第 8 节不建议现阶段做）

## Technical Notes

### 必读文档

- `docs/plans/2026-09-08-ai-optimization-overview.md` — 总览，第 5 节第 5 批、第 4 节依赖
- `docs/plans/2026-09-08-ai-function-search-design.md` — P2-2 主体设计
- `docs/plans/2026-09-08-ai-config-schema-design.md` — P2-3 主体设计
- `docs/spec/cross-layer-contracts.md` — 跨层契约（AiFunction 字段变更须同步 models.ts）

### 待改文件

| 文件 | 改动 |
| --- | --- |
| `model/ai_function.go` | AiFunction 加 Tags/Pinned；新增 AiFunctionsConfig + CurrentSchemaVersion |
| `service/ai_function.go` | LoadAiFunctions 走迁移；SaveAiFunctions 写新结构；defaultAiFunctions 加 tags/pinned 示例 |
| `service/ai_function_test.go` | v1 兼容、字段补全、校验兜底测试 |
| `frontend/src/components/AiFunctionPanel.vue` | 搜索框、tag chips、pinned 置顶计算属性 |
| `frontend/src/components/AiFunctionConfigDialog.vue` | tags/pinned 编辑 |
| `frontend/src/components/__tests__/AiFunctionPanel.spec.js` | 搜索、置顶、分组测试 |
| `frontend/wailsjs/go/models.ts` | AiFunction class 加 tags/pinned |

### 跨层契约关键点

- `LoadAiFunctions`/`SaveAiFunctions` 签名保持稳定 → App.js/App.d.ts 不动
- `AiFunction` 加字段 → models.ts 字段声明 + 构造函数赋值两处同步
- `frontend/wailsjs/` 已 gitignore，git status 无残留即同步完成

## Decision (ADR-lite)

待 3 个 Open Questions 确认后补充。
