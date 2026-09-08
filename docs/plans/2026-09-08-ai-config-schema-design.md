# AI 功能配置 Schema 版本与迁移设计

**日期**：2026-09-08
**优先级**：P2
**状态**：待评审（要点级，实施前需补充代码探查）

## 1. 概述

为 `data/ai_functions.json` 引入 `schemaVersion` 字段与加载期迁移机制，支持字段演进（如新增 `tags`/`pinned`/`metrics` 等）时旧配置自动补全，校验失败时备份回种，避免静默错误。

## 2. 现状与痛点

`LoadAiFunctions`（service/ai_function.go:62）仅判文件存在与否：

```go
func (s *AiFunctionService) LoadAiFunctions() ([]*model.AiFunction, error) {
    var funcs []*model.AiFunction
    if util.FileExists(s.configPath) {
        if err := util.LoadJSON(s.configPath, &funcs); err != nil {
            return nil, err   // 解析失败直接报错，无兜底
        }
        return funcs, nil
    }
    // 不存在 → 写入 defaults
}
```

| 痛点 | 说明 |
| --- | --- |
| 无版本 | 字段演进后旧配置缺新字段，运行时 nil/零值，行为不可预期 |
| 无字段补全 | 新增 `tags`/`pinned` 后，旧配置功能项无这些字段，前端需处处判空 |
| 无校验兜底 | 配置损坏（手改出错）时 `LoadJSON` 报错，功能菜单整体不可用，无备份回种 |
| seed 仅空触发 | 文件存在但内容非法（如部分字段缺失）不会回种 seed |

## 3. 需求总结

1. `ai_functions.json` 顶层包 `{schemaVersion, functions}` 结构
2. 加载时按版本迁移：旧版本字段补全到当前版本
3. 字段级校验，非法配置备份后回种 seed
4. 迁移与补全对用户透明，不丢失既有合法配置

## 4. 设计要点

### 4.1 配置结构升级

```json
{
  "schemaVersion": 2,
  "functions": [ ... ]
}
```

兼容旧格式：加载时若顶层是数组（无 schemaVersion），视为 v1，自动包一层并迁移。

### 4.2 迁移函数

`service/ai_function.go` 新增 `migrateFunctions(raw []byte) (funcs []*model.AiFunction, version int, err error)`：

| 迁移步骤 | 说明 |
| --- | --- |
| 解析顶层 | 判数组（v1）或对象（v2+） |
| 版本路由 | 按 `schemaVersion` 走对应迁移链：v1→v2 补 `tags`/`pinned`，v2→v3 补未来字段 |
| 字段补全 | 单个 `AiFunction` 缺字段填默认值（`permissionMode`→`bypassPermissions`，`timeoutMinutes`→10 等） |
| 校验 | 必填字段（id/name/command/cwd）缺失记录错误项 |

### 4.3 校验失败兜底

| 情况 | 处理 |
| --- | --- |
| 顶层解析失败 | 备份原文件为 `ai_functions.json.bad.<ts>`，回种 seed，提示用户 |
| 部分功能项非法 | 剔除非法项，保留合法项，备份原文件，提示被剔除的 id |
| 全部非法 | 备份后回种 seed |

### 4.4 版本常量

```go
const CurrentSchemaVersion = 2
```

每次 model 字段演进（增非兼容字段）时 +1，并新增对应迁移函数。

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `model/ai_function.go` | 新增 | `AiFunctionsConfig` 顶层结构（schemaVersion + functions） |
| `service/ai_function.go` | 修改 | `LoadAiFunctions` 走 `migrateFunctions`；`SaveAiFunctions` 写带版本结构 |
| `service/ai_function_test.go` | 修改 | v1 数组兼容、字段补全、校验失败兜底测试 |

## 6. 待确认点

1. 是否引入 JSON Schema 校验（如 `xeipuuv/gojsonschema`）？建议先用手写字段校验，校验规则复杂后再引入
2. `tags`/`pinned` 等新字段是否单独升版本，还是与既有字段补全合并到 v2？建议合并到 v2，避免版本号膨胀
3. 备份文件保留策略（数量/天数上限）？建议保留最近 5 个，超限删最旧

## 7. 关联

- 来源：[[2026-09-08-ai-skill-menu-design]] 后续优化建议
- 配套：[[2026-09-08-ai-function-search-design]]（tags/pinned 字段触发版本升级）、[[2026-09-08-ai-task-observability-design]]（metrics 不入配置文件，无版本影响）
