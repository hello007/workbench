# AI 功能配置导入导出

## Goal

`data/ai_functions.json` 为本地配置（schema v2 `{schemaVersion, functions}`），换机器或团队共享只能手拷文件。本任务在配置管理对话框加「导出配置」「导入配置」入口，导入复用第 5 批 `migrateFunctions` 迁移机制——旧 v1 配置或缺失字段自动补全，让 schema v2 迁移机制在导入场景再发挥一次价值。

来源计划：`docs/plans/2026-09-09-ai-config-import-export.md`（状态：待评审）。

## What I already know

- `AiFunctionService.migrateFunctions(raw []byte) (funcs, migrated, err)` 支持 v1 顶层裸数组与 v2 对象，字段补全（service/ai_function.go:195）
- `validateFunctions(funcs) (valid, invalidIDs)` 校验 ID/Name/Command/Cwd 非空（service/ai_function.go:249）
- `SaveAiFunctions(funcs) error` → `saveConfig` 写 `AiFunctionsConfig{SchemaVersion:2, Functions}`（service/ai_function.go:181/186）
- `model.AiFunctionsConfig{SchemaVersion, Functions}` 已存在，`CurrentSchemaVersion=2`（model/ai_function.go:31/27）
- 对话框 `AiFunctionConfigDialog.vue` 第 24 行「+ 新增功能」、第 31 行「从已发现 skill 导入」；已有嵌套子对话框模式 `importVisible`
- wailsjs：`App.d.ts` 已有 `GetAiFunctions/SaveAiFunctions`，`models.ts:170` 已有 `AiFunction` class
- 跨层契约：新增 App 方法须同步 App.js/App.d.ts，新增 model 须同步 models.ts（docs/spec/cross-layer-contracts.md）

## Requirements

1. 导出：当前全部功能项导出为 schema v2 JSON 文本（原样含 env/MCP 配置），导出前弹提示「配置含 env/MCP 配置，请确认共享范围」；前端用 Wails `runtime.SaveFileDialog` 选路径落盘；后端不耦合用户目录
2. 导入：选外部 JSON，后端 `ImportAiFunctions(jsonText)` 复用 `migrateFunctions` 解析迁移 + `validateFunctions` 校验，与本机已加载项比对 id 生成三类（新增/冲突/非法），返回 `ImportPreview` 不落盘
3. 导入预览：前端弹预览对话框展示三类，冲突项每项选「覆盖本机」/「跳过」，用户确认后合并落盘
4. 合并语义：保留本机已有，追加 New，Conflict 标记覆盖的替换、跳过的忽略，Invalid 不导入；合并后调既有 `SaveAiFunctions`

## Acceptance Criteria

- [ ] `model.ImportPreview` 结构新增（New/Conflict/Invalid 三类）
- [ ] `ExportAiFunctions() (string, error)` 返回 schema v2 JSON 文本
- [ ] `ImportAiFunctions(jsonText) (*ImportPreview, error)` 复用 migrate+validate，不落盘
- [ ] `app.go` 新增两 App 方法，wailsjs 三文件同步（App.js/App.d.ts/models.ts）
- [ ] 对话框加「导出配置」「导入配置」按钮 + 导入预览子对话框
- [ ] 后端测试：导出格式、导入 v1 迁移、冲突分类、非法项标记
- [ ] 前端测试：导入预览展示、冲突策略（覆盖/跳过）
- [ ] 导入非合法 JSON 走 migrateFunctions 错误返回，前端提示不落盘

## Definition of Done

- 单元测试 added/updated（后端 service + 前端组件）
- `go test ./...` 绿、`cd frontend && npm test` 绿
- 跨层绑定三文件同步
- README.md 按需更新（功能说明.md 有 AI 配置管理章节时同步）

## Technical Approach

### 导出
`ExportAiFunctions() (string, error)`：读当前 configPath 原始字节（或 LoadAiFunctions 后序列化 `AiFunctionsConfig`），返回 schema v2 JSON 文本。前端 `runtime.SaveFileDialog` 选路径后写文件。

### 导入解析
`ImportAiFunctions(jsonText string) (*model.ImportPreview, error)`：
1. `migrateFunctions([]byte(jsonText))` 解析迁移
2. `validateFunctions` 校验，非法项 id 入 Invalid
3. LoadAiFunctions 取本机现有，按 id 比对：本机无 → New，本机有 → Conflict
4. 返回 ImportPreview，不落盘

### 预览模型
```go
type ImportPreview struct {
    New      []*AiFunction `json:"new"`
    Conflict []*AiFunction `json:"conflict"`
    Invalid  []string      `json:"invalid"`
}
```

### 冲突策略
前端预览对话框冲突项每项「覆盖」/「跳过」单选。确认后前端组装 merged = 本机非冲突项 + New + Conflict 中标记覆盖项，调 `SaveAiFunctions(merged)`。

## Out of Scope

- 导出勾选部分项（先全部，配置整体共享）
- 导入历史/撤销
- 配置加密

## Decisions

- 导出范围：全部功能项（配置整体共享，简单直接）
- 导出敏感字段：原样导出 + 导出前弹提示「配置含 env/MCP 配置，请确认共享范围」（`$ENV:` 是引用非明文，headers 用户自行评估）

## Technical Notes

- 复用第 5 批 schema v2 迁移机制，不新建迁移路径
- 落盘走既有 SaveAiFunctions（app 自身 service 写 configPath，带 mu 锁，不触发外部写覆盖问题）
- 跨层契约：docs/spec/cross-layer-contracts.md
