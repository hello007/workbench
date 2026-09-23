# data/*.json 原子写收口

## Goal

data/ 目录 JSON 落盘改为 temp+rename 原子写，消除崩溃/写入中断场景整体覆盖丢数据。历史教训（memory 已沉淀）：写产物 JSON 前必须手动关闭 workbench.exe，否则 RepoMetaService 整体覆盖写丢数据——原子写收口后该手动约束应可消除。

## Brainstorm 摸底产出（落盘点清单）

### A 类：经 util.SaveJSON 的截断写落盘点（9 处，收口核心——改 util 一处全量切换，签名不变零调用方改动）

|#|落盘文件|服务|调用点|
|---|---|---|---|
|1|data/directories.json|DirectoryService|service/directory.go:44|
|2|data/settings.json|SettingsService|service/settings.go:34|
|3|data/favorites.json|FavoritesService|service/favorites.go:42|
|4|data/repo_meta.json|RepoMetaService|service/repo_meta.go:53（memory 教训来源）|
|5|data/ai_functions.json|AiFunctionService|service/ai_function.go:194|
|6|data/ai_task_history_meta.json|AiTaskHistoryService|service/ai_task_history.go:60|
|7|data/repo_scan_cache.json|GitServiceWithCache|service/repo_scan_cache.go:90|
|8|data/session.json|SessionService|service/session.go:57|
|9|data/dashboard_pinned.json|DashboardService|service/dashboard.go:63|

### B 类：已自建原子写实现（收敛到统一核心，删除重复实现）

|#|实现|覆盖文件|处理|
|---|---|---|---|
|1|saveChatJSON（service/chat_service.go:50）|ai_chat/sessions.json、ai_chat/directories.json、ai_chat/messages/*.json、ai_chat/templates.json|保留 .bak 上一好版本语义，原子核心委托 util.SaveJSON，删自建 temp/rename 管道|
|2|atomicWriteConfig（service/obsidian_vault.go:186）|data/obsidian.json（vault 注册表）|整体委托 util.SaveJSON，删 pid 后缀 temp 自建实现|

注：B1 的 temp 命名模式 `.<base>.tmp-*` 被 chat_service_test.go:1253 glob 断言依赖，统一核心沿用同模式。

### C 类：非 data JSON 持久化写入（明确排除，不动）

|位置|内容|排除理由|
|---|---|---|
|server/token.go:59|data/token（明文 token 单行）|非 JSON|
|service/update.go:447/503/540|更新脚本/pending 标记|脚本与标记非 JSON|
|util/difftool.go:65|OS 临时目录 diff 脚本|非 data 目录、消费后即删|
|service/ai_task_history.go:114 copyFile|任务输出 .txt 跨卷复制退化路径|非 JSON，语义为复制|
|各 backupConfig / 损坏留底 .bak 写（ai_function.go:380、chat_config.go:60、obsidian_vault.go:180、chat_service.go:260/450/652）|时间戳备份文件|备份产物非主数据，截断无害|
|util/testutil.WriteFile|测试 fixture|测试专用|

## Requirements

* **先 brainstorm 摸底**：✅ 清单见上节
* 统一收口一个原子写 util：写同目录临时文件 → fsync → rename 覆盖目标 → util.SaveJSON 原位改造（CreateTemp `.<base>.tmp-*` 隐藏临时文件 + Write + Sync + Close + os.Rename + defer 清理残留）
* Windows 语义确认：os.Rename 在 Windows 走 MoveFileEx(MOVEFILE_REPLACE_EXISTING)，对已存在目标为同卷原子替换（Go 标准库自 1.5 起行为，obsidian_vault 既有实现已依赖）；以单测锚定覆盖语义
* 全部落盘点切换至统一 util：A 类零改动自动切换；B 类删重复实现委托核心；grep 复核无遗漏
* fsync 口径：临时文件级 Sync（FlushFileBuffers/fsync）；目录级 fsync 不做（Windows 不支持，且本任务目标为防进程崩溃截断而非掉电持久化），doc comment 记录该边界
* 完成后更新 memory「WorkBench 数据写入约束」条目

## 关键定性发现（brainstorm）：memory 约束「改述」而非「消除」

PRD 原期望「原子写收口后手动关 workbench.exe 约束应可消除」——**定性结论：不成立，约束只能改述**。原子写消除的是「写入中断 → 目标截断为半截 JSON」这一损坏形态；而 memory 教训的根因是 RepoMetaService 的**整体覆盖模型**（应用内存态 → 全量序列化 → 覆盖写文件），用户手改 data/*.json 后，应用任意一次保存（原子与否）都会以内存态覆盖手改内容——last-writer-wins 与写入原子性正交。改述后的约束：「手改 data/*.json 前仍须关闭 workbench.exe（整体覆盖模型不变）；崩溃/写入中断导致的半截文件损坏已由原子写消除」。

## Acceptance Criteria

* [x] 落盘点清单入 prd（摸底产出）
* [x] 统一原子写 util + 全落盘点切换，无遗漏（grep 复核：A 类 9 处经 SaveJSON 原位切换，B 类 2 实现收敛委托，C 类按清单排除）
* [x] 单测：模拟中断（临时文件残留）不破坏原文件（TestSaveJSON_StaleTempResidueDoesNotBreakTarget）；Windows 覆盖语义有测试锚定（TestSaveJSON_OverwriteExisting）；另有成功无残留、序列化失败不碰目标两锚点
* [x] `go test ./...` 全绿（6 包 -count=1 全新跑）
* [x] 完成后更新 memory「WorkBench 数据写入约束」条目（改述：整体覆盖模型仍在，崩溃截断风险消除）

## Out of Scope

* 多实例并发写文件锁（单用户桌面场景）；SQLite 等存储替换；JSON schema 变更
* C 类非 JSON 写入的原子化；.bak 时间戳备份产物的生命周期管理

## Technical Notes

* 教训来源：用户实操中 workbench.exe 运行时写 data/*.json 被 RepoMetaService 整体覆盖丢数据
* 相关规范：logging-and-errors（降级 slog.Warn 模式先例）
