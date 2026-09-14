# AI 功能配置导入导出

**日期**：2026-09-09
**优先级**：P2
**状态**：待评审

## 1. 概述

`data/ai_functions.json` 是本地配置（第 5 批已升级 schema v2 `{schemaVersion, functions}`），
换机器或团队共享时只能手动拷文件。本设计在配置管理对话框加「导出配置」与「导入配置」入口，
导入时复用第 5 批的 `migrateFunctions` 迁移机制——旧 v1 配置或缺失字段自动补全，
让 schema v2 迁移机制在导入场景再发挥一次价值。

## 2. 现状与痛点

`AiFunctionConfigDialog.vue` 第 4 批加了「从已发现 skill 导入」（单功能项导入），
但无整份配置的导入导出。配置迁移机器只能手拷 `data/ai_functions.json`，且手拷不触发
迁移校验（旧机器导出的 v1 配置拷到新机器加载时虽会迁移，但用户无感知且无冲突处理）。

| 痛点 | 说明 |
| --- | --- |
| 无导出 | 换机器/备份只能手拷文件 |
| 无导入 | 团队共享配置需手拷，无冲突处理（id 重复等） |
| 迁移机制未复用 | 第 5 批 migrateFunctions 只在加载本机文件时触发，导入外部配置未走 |

## 3. 需求总结

1. 导出：当前全部功能项导出为 schema v2 JSON 文件（用户选保存路径）
2. 导入：选外部 JSON 文件，解析后走 migrateFunctions 迁移补全，冲突（id 重复）给策略
3. 导入预览：导入前展示将新增/将冲突的项，用户确认后落盘
4. 导入不直接覆盖：合并而非替换（保留本机已有、追加新项），冲突项用户决策

## 4. 设计要点

### 4.1 导出

`AiFunctionService` 新增 `ExportAiFunctions() (string, error)`：返回当前配置的 schema v2 JSON 文本
（`{schemaVersion, functions}` 序列化）。前端拿到文本后用 Wails `runtime.SaveFileDialog` 让用户选路径落盘。
避免后端直接写文件耦合用户目录。

### 4.2 导入解析与迁移

`AiFunctionService` 新增 `ImportAiFunctions(jsonText string) (*model.ImportPreview, error)`：
1. 复用 `migrateFunctions([]byte(jsonText))` 解析并迁移（v1 数组/v2 对象均支持，字段补全）
2. 复用 `validateFunctions` 校验，非法项标记
3. 与当前已加载功能项比对 id：生成新增项、冲突项（id 已存在）、非法项三类
4. 返回 `ImportPreview` 不直接落盘——前端展示预览，用户决策冲突策略后再调 SaveAiFunctions

### 4.3 预览模型

```go
// ImportPreview 导入预览：解析+迁移+比对后的分类结果，供前端展示后确认
type ImportPreview struct {
    New      []*AiFunction `json:"new"`      // 本机不存在的，将新增
    Conflict []*AiFunction `json:"conflict"` // id 已存在，待用户决策覆盖/跳过
    Invalid  []string      `json:"invalid"`  // 校验失败的 id（不会导入）
}
```

### 4.4 冲突策略

前端预览对话框展示三类，冲突项每项可选「覆盖本机」/「跳过」。用户确认后：
- New 全部追加
- Conflict 中标记「覆盖」的替换本机同名项，「跳过」的忽略
- Invalid 不导入
合并后调既有 `SaveAiFunctions(merged)` 落盘（走 schema v2 保存，无需新方法）。

### 4.5 UI 入口

`AiFunctionConfigDialog.vue` 功能项列表区顶部（「+ 新增功能」按钮旁）加：
- 「导出配置」按钮 → 调 ExportAiFunctions → SaveFileDialog 落盘
- 「导入配置」按钮 → OpenFileDialog 选文件 → 读文本调 ImportAiFunctions → 弹预览对话框 → 确认后 SaveAiFunctions

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `model/ai_function.go` | 新增 | `ImportPreview` 结构 |
| `service/ai_function.go` | 修改 | 新增 `ExportAiFunctions` / `ImportAiFunctions`（复用 migrateFunctions + validateFunctions） |
| `service/ai_function_test.go` | 修改 | 导出格式、导入 v1 迁移、冲突分类、非法项标记测试 |
| `app.go` | 修改 | 新增 `ExportAiFunctions` / `ImportAiFunctions` App 方法 |
| `frontend/wailsjs/go/main/App.js` + `App.d.ts` | 同步 | 两新方法签名 + ImportPreview models.ts |
| `frontend/wailsjs/go/models.ts` | 同步 | `ImportPreview` class |
| `frontend/src/components/AiFunctionConfigDialog.vue` | 修改 | 导入导出按钮 + 导入预览对话框 |
| `frontend/src/components/__tests__/AiFunctionConfigDialog.spec.js` | 修改 | 导入预览、冲突策略测试 |

## 6. 待确认点

1. 导入合并 vs 替换？建议合并（保留本机已有，避免误覆盖用户自定义），冲突项用户逐项决策
2. 导出范围：全部功能项 vs 支持勾选部分？建议先全部（简单，配置项通常整体共享）
3. 导出是否含敏感字段（env 的 `$ENV:` 引用、MCP headers）？建议原样导出——
   `$ENV:` 是引用非明文（第 7.3 节探查确认 token 不落配置），headers 用户自行评估；
   可加导出前提示「配置含 env/MCP 配置，请确认共享范围」
4. 导入文件格式校验失败（非合法 JSON 或非 ai_functions 结构）的兜底？
   建议走 migrateFunctions 错误返回，前端提示具体原因，不落盘

## 7. 关联

- 来源：[[2026-09-08-ai-optimization-overview]] 后续优化建议
- 依赖：第 5 批 schema v2 迁移机制（`migrateFunctions` / `validateFunctions` 已实施，
  导入直接复用）、第 4 批「从已发现 skill 导入」（单功能导入已实现，本设计是整份配置导入，互不冲突）
- 跨层契约：[[cross-layer-contracts]]（新增 App 方法须同步 App.js/App.d.ts，新增 model 须同步 models.ts）
