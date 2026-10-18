# 素材管理第十批：索引配置（排除规则 + excluded 态）与素材库备份（SRS §2.7.3 / §2.7.5 / §14.16）

## Goal

补齐素材管理 SRS §2.7 系统设置域两条缺口：

1. **索引配置**：每库独立的扫描排除规则（排除文件夹 / 忽略后缀），规则变更即时生效；排除命中的已入库文件引入 `excluded` 索引态无损保留。
2. **素材库备份**：手动 + 重大操作（重建索引 / 删除库）前自动快照，保留淘汰 + SQLite 完整性校验 + 还原入口；顺带解锁 §14.16 重建后哈希恢复元数据（第九批 out of scope 补链）。

**明确不做**：目录监听（SRS 定 P2 可选，fsnotify 生命周期/防抖/网络盘降级面大，独立成批）、OCR（引擎选型独立成批）、批量导出截图（§2.6.3 P1 后移）、快捷键（§2.7.4 后移）。

## Requirements

### R1 排除规则（每库独立 + 即时生效）

* `AssetLibrary` 加字段：`ExcludeDirs []string`、`ExcludeExts []string`，存 `libraries.json`（util.SaveJSON 原子写既有口径）。
* 匹配语义（research/exclude-rules-semantics.md 定稿）：`ExcludeDirs` 每项一个 glob 模式（`filepath.Match` 单段语法、大小写不敏感、**不含 `/` 与 `**`**），匹配相对路径任意单层目录段名（剪枝用目录基名匹配，规避 Windows 下 `filepath.Match` 的 `/` 不匹配 `\` 分隔符坑）；**root 自身豁免匹配**（防整库剪空）。`ExcludeExts` 归一带点小写，输入宽容（`tmp`/`.tmp`/`*.tmp` 三形态等价）。四个对标工具（Billfish/Eagle/Bridge/Everything）均无多段路径 glob 排除语法，标准库即足额零新增依赖。
* 生效点：`collectPptxEntries`（service/assets_scan.go:631）WalkDir 回调——目录段命中 `SkipDir` 剪枝，文件后缀命中不入 entries。拆分派生排除（isPptxSplitDerivative）优先级不变，先于用户规则。
* 配置 UI：库设置（排除文件夹/忽略后缀两组编辑），保存即自动触发该库增量重扫（复用现有扫描链路，可取消）。

### R2 excluded 索引态

* `AssetFileStatus` 新增 `AssetFileStatusExcluded = "excluded"`（model/assets.go 常量表 + 注释）。
* 判定顺序（扫描时）：**先判排除规则，命中即 excluded，不做存在性探测**（先排除后存在性）；规则不命中且磁盘文件消失才走 lost。
* 命中处理口径：已入库文件（任意现存态）规则命中标 `excluded`；标注/评分/备注/缩略图**全保留**（对齐 lost 无损语义）。
* 复活：规则改宽后下轮扫描，excluded 文件磁盘仍在 → 走现有增量三态（unchanged 跳过保 ok / 变更重解析），从 excluded 转回正常态；excluded 且磁盘已消失 → 保持 excluded（规则优先），规则改宽后才可见 lost。
* 可见性分叉（与 lost 相反）：**检索 / 标签浏览 / facet 全入口过滤 excluded 不可见**（lost 有意可见引导重链；excluded 语义是「用户不要」，可见即规则失效）；**导出照导**（对齐 lost 现状，行键无 status 过滤）；库统计计数含 excluded（对齐 lost 计入口径）。
* 本批不加「清理 excluded」手动入口（后批复用第九批 lost 清理链路）。

### R3 素材库备份

* 触发点：手动（库操作入口）+ 重建索引前自动 + 删除库前自动。
* 实现：`VACUUM INTO '备份路径'`（单语句，输出独立紧凑 db，天然规避 WAL 一致性问题；research 阶段验证 modernc.org/sqlite 支持）+ 完成后 `PRAGMA integrity_check` 校验，非 ok 删备份文件并报错（备份失败不静默）。
* 落点命名：`data/assets/backups/<libId>/<yyyyMMdd-HHmmss>.db`（时间戳命名天然不与既有目标冲突；VACUUM INTO 目标已存在会报 `output file already exists`）。
* 保留：每库最近 5 份，超出淘汰最旧（默认值硬编码，配置化后批）。
* 还原：确认弹窗（明示覆盖当前索引与标注）→ 关该库连接 → 备份文件替换 `<id>.db`（同删残留 `-wal/-shm`）→ 惰性重开连接 → 前端提示重扫。还原前若该库有在途扫描/缩略图批次先取消。Windows `os.Rename` 直接覆盖已存在目标（MoveFileEx REPLACE_EXISTING，TestSaveJSON_OverwriteExisting 已锚定），无需先 Remove。
* 重大操作自动备份失败策略：**告警但不中止原操作**；删库前场景例外升级——备份失败时删除确认弹窗追加「备份失败，删除后无恢复点」强提示，用户二次确认才继续（research/sqlite-backup-restore.md 零恢复点删除残余风险缓解）。

### R4 重建后哈希恢复（§14.16 补链）

* 重建备份标注时同取 `files.hash` 列（现 `fileTagBindingsByPath`/`slideAnnotationsByPath` 键为 path，扩 hash 维度）。
* 重绑匹配两级：先路径匹配（现状不变），miss 后按 hash 兜底。hash 兜底多命中取首个并日志告警（同内容文件误绑概率低，标注语义损失小）。

## Acceptance Criteria

* [ ] 排除规则：配置目录段 glob 与后缀后，扫描不再入库命中文件；已入库命中文件转 excluded 且标注/缩略图保留；规则改宽后复活转正常态
* [ ] excluded 不可见面：关键词检索 / 标签浏览 / facet 三入口均不返回 excluded 行；导出含 excluded 行；库统计含 excluded 计数
* [ ] excluded 与 lost 互斥：excluded 文件消失不标 lost；规则改宽后消失文件标 lost
* [ ] 保存规则自动触发增量重扫，重扫可取消，串行锁语义与现扫描一致
* [ ] 手动备份生成 `data/assets/backups/<libId>/<时间戳>.db`，integrity_check 通过；第 6 份触发淘汰最旧
* [ ] 重建索引前 / 删除库前自动落备份；备份失败仅告警不中止主操作
* [ ] 还原：确认后当前库被备份内容替换，重开后索引与标注回到备份时点；还原前在途任务被取消
* [ ] 哈希恢复：重建后文件改名，路径 miss 时按 hash 恢复标注
* [ ] 测试禁 sleep/mtime 时序/真实 FS 事件等待；FTS 五契约与增量三态不变式回归通过；service ≥76% / 前端 ≥70% 门禁

## Definition of Done

* 单测覆盖上述验收面（排除匹配/excluded 状态机/备份淘汰/还原/哈希重绑）
* wails generate 三处同步（App.js / App.d.ts / models.ts）；mock 改 wails-mock-defaults.js；E2E 从 fixtures.js import
* README.md / docs（API参考、功能说明、架构设计按需）更新确认
* spec 沉淀（docs/spec/assets-management.md 增第十批节）

## Technical Approach

**PR1 后端排除规则 + excluded 态**：model（AssetLibrary 字段 / AssetFileStatusExcluded）→ 收集层过滤（collectPptxEntries 带规则参数）→ 扫描层 excluded 处置（先排除后存在性）→ 检索/浏览/facet 三入口过滤 → 规则保存 API + 自动重扫 → 单测。

**PR2 后端备份域 + 哈希恢复**：备份/还原/淘汰/integrity_check service → 重建/删库挂自动备份 → 重绑 hash 兜底 → 单测（含 modernc VACUUM INTO 真实性验证）。

**PR3 前端**：库设置排除规则编辑 UI + 备份/还原入口 + wails 绑定三处 + mock + vitest + E2E 关键流。

## Decision (ADR-lite)

**Context**：SRS §2.7.3/§2.7.5 为素材管理设置域缺口；目录监听 P2 可选体量大；第九批留白 §14.16。
**Decision**（用户逐题确认）：主轴 = 排除规则 + 备份打包（监听/OCR/截图/快捷键全部不做）；排除规则每库独立 + 保存即自动重扫；排除命中标 excluded 态无损保留（非 DELETE），检索不可见、导出照导、先排除后存在性；备份完整链（手动+重建前+删库前自动 / VACUUM INTO / 保留 5 份 / 还原含确认）；哈希恢复本批搭车。
**Consequences**：状态机加一态，检索三入口须加过滤（漏加=规则失效可见，测试面覆盖）；备份还原涉及连接生命周期（关连接→换文件→重开），窗口期该库操作须拒绝；目录监听继续留白（下批独立成批，含 fsnotify 选型）。

## Out of Scope

* 目录监听（fsnotify）——下批独立
* OCR（§2.2 开关 + 引擎选型）——独立成批
* 批量导出选中幻灯片截图（§2.6.3 P1）——后批
* 快捷键体系（§2.7.4）——后批
* `.damignore` 用户侧忽略文件（§2.1.2）——本批仅 UI 配置面，文件方案后批评估
* excluded 清理入口、备份份数/开关配置化、全局默认排除规则——后批
* 前两批遗留真机冒烟清单（archive/2026-10/10-16-srs-2-6-4-srs-2-6-5/smoke-checklist.md）——本批完成后一并执行

## Technical Notes

* 现状锚点：`collectPptxEntries`（service/assets_scan.go:631）纯 WalkDir 仅排拆分派生；`AssetLibrary` 四字段（model/assets.go:12）；`AssetFileStatus` 六态（model/assets.go:33-40）；`RebuildScan` 重建重绑键 path（service/assets_scan.go:302-342）；检索路径无 status 过滤（assets_search.go）；导出行键无 status 过滤（assets_export.go:622）。
* 硬约束：FTS 五契约不破；增量三态不变式回归；备份禁裸拷贝 db 文件（WAL）；data/ 域 util.SaveJSON 原子写；COM 约束（本批不涉 COM）；frontend-visual-conventions 设置弹窗约束；监听类禁真实 FS 事件等待（本批无监听）。
* research 待办：① ✅ 排除规则语义已定稿（research/exclude-rules-semantics.md）；② ✅ SQLite 备份链已实证（research/sqlite-backup-restore.md）：modernc v1.60.1 支持 VACUUM INTO/integrity_check，integrity_check 须 Query 非 Exec（Exec 丢结果集形同没校验）判首行 != ok，单连接池下 VACUUM INTO 执行期查询排队（60 万行 127ms 影响可忽略），事务内不能包 Tx。
* 实现注意：excluded 态判定是「规则 × 存量索引 path」独立链路，与 R1 收集层磁盘过滤是两条链（规则变更重扫时：存量命中标 excluded / 解除的复活，新盘上命中文件根本不入 entries）。
