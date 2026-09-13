# 仓库列表配置导入导出

## Goal

为 WorkBench 增加仓库列表配置（工作目录 + 收藏夹）的 JSON 导出/导入能力，支持跨设备迁移与备份恢复。复用 AI 配置导入导出的对话框骨架与桥接先例（SaveFileDialog/OpenFileDialog），导入走「解析预览 → 逐项冲突决策 → 执行 → 结果汇总」流程。

## Requirements

* 导出：工作目录（name/path/isDefault）+ 收藏夹（path/alias/group/createdAt）组装为带 `manifestVersion` 的 JSON 文本；后端组装，前端桥接 SaveFileDialog+SaveFile 落盘
* 导出文件不含运行时态（IsGitRepo/HasRemote）与本机标识（Directory.ID）
* 导入：OpenFileDialog 选文件 → ReadFileBytes 读文本 → 后端 service 解析+比对分类（新增/冲突/非法，不落盘）→ 前端预览 + 冲突项逐项决策（跳过/覆盖/另存为新项）→ 合并落盘 → 结果汇总（新增/覆盖/跳过/非法计数）
* 冲突判定键：path（工作目录按规范化 path，收藏夹按 path）
* 「另存为新项」语义：保留本机条目 + 导入项以新身份插入——工作目录重新生成 ID、名称追加「（导入）」后缀；收藏夹直接共存（alias/group 不同即有意义）。导入写路径放行同 path 条目，不改现有 Add/Create 约束
* 数据校验分层：
  - 全局：JSON 结构非法 / manifestVersion 缺失或高于当前支持 → AppError 结构化错误，整体拒绝
  - 逐项：字段缺失（path/name 空）、路径在本机不存在 → 列入 invalid（含 id/路径/原因），不阻断合法项
* 覆盖工作目录时重算 IsGitRepo/HasRemote（Create 先例）；导入项 isDefault=true 时清本机其他 default
* 收藏夹导入后超 100 条上限：超出部分列入失败计数，报错不静默截断
* UI 入口：DirectoryTree.vue 工具栏加「导入/导出」小按钮（与 Plus 圆钮同区，风格一致）
* 新建 `service/repo_config.go` RepoConfigService 聚合 directorySvc + favoritesSvc，遵循 app-services-assembly（AppServices struct + NewAppServices 各一处）
* 新错误码双侧同步：model/app_error.go + frontend/src/utils/error.js（ErrorCode/WARNING_CODES）

## Acceptance Criteria

* [ ] 后端单测：导出组装含 manifestVersion 与业务字段、不含运行时字段
* [ ] 后端单测：解析分类（新增/冲突/非法）、非法 JSON、不支持版本、路径不存在、收藏夹超限
* [ ] 后端单测：三种冲突决策（跳过/覆盖/另存为新项）的合并写规则、isDefault 互斥、覆盖重算 git 状态
* [ ] 前端单测：预览对话框三类渲染 + 决策交互 + 结果汇总；覆盖率 ≥70% 门禁零回归
* [ ] E2E：导出调用参数断言 + 导入预览-决策链路（mock SaveFileDialog/OpenFileDialog/ReadFileBytes）
* [ ] wailsjs 三处手动同步（App.js/App.d.ts/models.ts）
* [ ] docs/功能说明.md 更新

## Definition of Done

* go test ./... + cd frontend && npm test + e2e 全绿；lint/typecheck 通过
* 文档同步（功能说明.md；如有新契约沉淀 docs/spec/）
* 错误码双侧同步

## Technical Approach

**导出组装前后端选择：后端组装。理由：**
1. 与 AI 配置导出先例（ExportAiFunctions）一致，前端零数据拼装
2. 结构演进（manifestVersion 升级）单点收敛在 service 层，不散落前端
3. 数据源即后端两个 JSON 文件，后端组装少两跳 IPC 语义转换

**JSON 结构（manifest v1）：**
```json
{
  "manifestVersion": 1,
  "exportedAt": "2026-09-13T12:00:00+08:00",
  "directories": [{ "name": "", "path": "", "isDefault": false }],
  "favorites": [{ "path": "", "alias": "", "group": "", "createdAt": 0 }]
}
```

**错误码（新增）：**
* `E_REPO_CONFIG_INVALID_JSON`：JSON 非法/顶层结构错（error）
* `E_REPO_CONFIG_UNSUPPORTED_VERSION`：manifestVersion 缺失或高于支持（error，提示重新导出）

**桥接：** SaveFileDialog/OpenFileDialog/SaveFile/ReadFileBytes 全部复用现有，零新增桥接方法；App 层仅加 3 个委托方法（ExportRepoConfig / PreviewRepoConfigImport / ApplyRepoConfigImport）

**导入合并语义：** merge 不替换。以本机两列表为基底，新增追加、覆盖按 path 原位替换（保留本机 ID）、另存为新项追加（新 ID+后缀）、跳过/非法忽略；两个配置文件分别落盘

## Decision (ADR-lite)

**Context**: 导出组装放前端（拿 GetDirectories+GetFavorites 拼装）还是后端；冲突「另存为新项」语义；UI 入口。
**Decision**: 后端组装（先例一致+演进单点）；另存为新项=保留本机+导入项新身份插入（新 ID+「（导入）」后缀，收藏夹同 path 共存）；入口在 DirectoryTree 工具栏。
**Consequences**: 导入写路径需独立于 Add/Create 的唯一约束实现（新逻辑，不动现有方法）；同 path 收藏夹可共存属预期行为；工具栏新增两钮视觉密度轻微上升。
**备注**: 两项偏好问题用户未答，按推荐项执行；实现前如有异议可推翻。

## Out of Scope

* 不迁移 AI 功能配置/终端设置/repo_meta 简述标签等其他数据域
* 不做自动定时备份、云同步
* 不做导出文件加密（导出前不弹共享范围确认，无敏感字段）

## Technical Notes

* 已探查：service/directory.go、service/favorites.go、model/models.go（Directory/Favorite）、model/ai_function.go（ImportPreview 先例）、model/app_error.go、app_ai.go、app_preview.go（桥接）、app_services.go（装配）、frontend/src/components/AiFunctionConfigDialog.vue（对话框骨架）、frontend/src/utils/error.js、frontend/src/components/DirectoryTree.vue（工具栏入口）
* 冲突分类纯函数设计为包内可测（输入两个本机列表+导入列表，输出 Preview），不依赖文件 IO
* 收藏夹无 ID，预览/决策以 path 为 key；工作目录预览决策 key 也用 path（导入项 ID 无意义）
