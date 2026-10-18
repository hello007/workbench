# 素材管理第九批：导出元数据（SRS §2.6.4）+ 文件丢失与重链（SRS §2.6.5）

## Goal

补齐素材管理 SRS §2.6.4（导出元数据 CSV / Markdown+YAML）与 §2.6.5（文件丢失检测、手动重链、根目录前缀替换）两大能力，打通 Obsidian 互通与文件移动后的元数据保全链路。拆两批 PR：PR1 导出（纯只读面）、PR2 丢失与重链（扫描语义变更面）。

## Requirements

### PR1 导出元数据（SRS §2.6.4）
- 新增 `service/assets_export.go` + App 绑定 `ExportAssetMetadata`（wails 三处同步 + mock）。
- 范围口径（D2）：有选中条目导选中（fileIDs），检索语境导当前检索命中（AssetSearchOptions 快照后端重放查询取全集），浏览语境导全库；弹窗显示「将导出 N 条」预览。
- 行粒度（D3）：每页一行（slide_num≥1，note/rating 页级，tags 继承合并口径）+ 文件行 slide_num=0（文件级 note/rating/tags）；排序 path + slide_num 稳定输出。
- 字段（SRS 11 列）：`source_file_path, slide_num, slide_title, slide_text, speaker_notes, ocr_text, thumbnail_path, tags, summary, note, single_pptx_path`；`ocr_text`/`summary` 保留列恒空。
- CSV：UTF-8 BOM、`encoding/csv`（UseCRLF）全字段引号保护；单测覆盖中文/逗号/引号/换行/反斜杠路径。
- MD：导出目标为目录，每页一个 `<3位序号>-<标题清洗>.md`（YAML frontmatter + 正文标题/文本/备注段落）；文件行同样一卡；流式逐文件写。
- 路径口径（D5）：`source_file_path` UI 选项默认库相对（基准 root_path）可选绝对；`thumbnail_path` 恒绝对（缓存档位目录实际位置）；`single_pptx_path` 跟随 source_file_path 口径（探测产物存在，缺省导空）；YAML 路径反斜杠转 `/`。
- 覆盖策略：目标已存在（CSV 文件 / md 目录非空）返回确认型错误码，前端确认后 overwrite=true 重调。
- 大库流式写：分批查询 + 逐行/逐文件写，1 万文件级不走全量内存拼接。
- 前端：工具行「导出」按钮 + 弹窗（范围预览/格式勾选 CSV+MD 可同选/路径口径/目标路径）。

### PR2 文件丢失与重链（SRS §2.6.5）
- lost 态（D1）：`AssetFileStatusLost` 常量；scanLibrary 磁盘消失分支 DELETE 改 UPDATE status=lost（标签/标注/评分/缩略图记账与 PNG 全保留）；「清理丢失记录」手动入口（等效旧删除语义）。
- lost 复活：扫描增量判定补 lost 行分支——磁盘命中且 mtime+size 未变 → 直接复活 ok；已变 → 走重解析。
- 统计与呈现：统计行加丢失数；画廊卡片 lost 标记；列表状态列；（scope 最小化：不新增独立状态筛选器）。
- 手动重链（D6）：`RelocateAssetFile`——目标必须库内（IsAbs+Rel 越界遏制），算哈希与记录 hash 比对：相等直接改 path 复活 ok；不等前端确认后 force=true 重链（标注保留，重解析按新文件）；hash 空跳过校验 + 提示。
- 前缀替换（D7）：`PreviewAssetPathPrefixReplace`（命中数预览）+ `ReplaceAssetPathPrefix`（校验旧前缀命中非空 + 新路径逐条越界遏制 → 单事务批量 UPDATE → 即时探测新位置：匹配复活 ok / 已变触发重解析 / 不存在保持 lost）→ 自动触发增量扫描收敛。二次确认列出命中数与新旧前缀。
- 前端：重链入口（lost 卡片操作/详情面板，选库内文件）、前缀替换弹窗（工具行菜单）、清理丢失记录入口（确认交互）。

## Acceptance Criteria

### PR1
- [ ] CSV 含 UTF-8 BOM，Excel 打开中文不乱码；中文/逗号/引号/换行/反斜杠数据转义单测通过
- [ ] YAML frontmatter 路径无反斜杠（`/` 口径）单测通过
- [ ] 页行/文件行（slide_num=0）粒度正确，note/rating/tags 来源层级正确
- [ ] ocr_text/summary 空列存在
- [ ] 三种范围（选中/检索命中/全库）导出行集正确；检索命中后端重放全集非前端已加载页
- [ ] 目标已存在拒绝，overwrite=true 后成功
- [ ] 流式写验证（分批查询无全量拼接）
- [ ] 新绑定 wails 三处同步 + wails-mock-defaults.js + E2E（fixtures.js import）
### PR2
- [ ] 扫描发现文件消失 → status=lost，标签/标注/评分/缩略图存续（不再 CASCADE 删除）
- [ ] 文件回来（mtime+size 未变）自动复活 ok；已变走重解析
- [ ] 统计行/画廊卡片/列表状态列 lost 呈现
- [ ] 重链哈希相等复活全部元数据；不等需 force 确认；库外目标拒绝
- [ ] 前缀替换：越界路径拒绝、命中预览数正确、执行后状态收敛（复活/重解析/保持 lost 三态）
- [ ] 清理丢失记录删除行与关联数据（旧删除语义等价）
- [ ] 既有扫描增量回归（未变化跳过/变更重解析/删除→标记 lost 语义迁移）全绿
- [ ] service ≥76% / 前端 ≥70% 门禁

## Definition of Done

- 两批 PR 各自全量测试通过 + trellis-check + cavecrew-reviewer 两轮审核
- spec 沉淀（docs/spec/assets-management.md 增补）+ README/文档确认
- 真机冒烟：导出中文特殊字符文件 + 丢失标记呈现 + 重链恢复 + 第八批遗留四项（排序档位切换/旧快照调取/批量评分 facet 联动/聚合展开）

## Technical Approach

- 导出纯只读：禁碰索引写链路；数据收集复用检索查询层（searchAssetsInStore / 文件清单查询），写面走用户目标路径（非 data/ 域，不受 util.SaveJSON 约束）。
- lost 态是 status TEXT 值域扩展（五态→六态），schema 无 CHECK 约束则零迁移（实现时确认 assets_schema.go）。
- scanLibrary 消失分支改造保留增量链路三态不变式：未变化跳过 / 变更重解析 / 消失标记 lost（原删除）。
- 重链/前缀替换写面走每条目（或单事务批量）先例；越界遏制对齐 RetryAssetThumb IsAbs+Rel。
- 错误码：导出目标冲突、重链库外、前缀替换零命中等新增码同步 `model/app_error.go` + `frontend/src/utils/error.js` 两处。

## Decision (ADR-lite)

### D1 lost 态语义 = 扫描标记 lost 替代删除
**Context**：现状扫描时磁盘消失文件直接 deleteFilesByPaths（CASCADE 标签/标注/缩略图记账），与 SRS「检测文件丢失，标记状态」冲突。
**Decision**：消失文件记录保留 status=lost，元数据全存续；重链 = 改 path + 哈希校验零恢复成本；文件回来（同 path 且 mtime+size 未变）自动复活 ok；「清理丢失记录」手动入口兜底。哈希匹配恢复退居重链校验场景。
**Consequences**：scanLibrary 清理分支 DELETE 改 UPDATE；增量判定补 lost 复活分支；统计/呈现面扩展。

### D2 导出入口与范围口径 = 工具行入口 + 语境自动跟随
**Decision**：工具行「导出」按钮；范围规则选中 > 检索命中 > 全库，与 collectSelectionTargets 一致；弹窗预览 N 条。

### D3 字段口径与导出粒度 = 页行 + 文件行 slide_num=0
**Decision**：见 Requirements；ocr_text/summary 保留列恒空向前兼容。

### D4 MD 形态 = 每页一个 .md 到导出目录
**Decision**：`<3位序号>-<标题清洗>.md`；CSV 与 MD 同批可同选；流式逐文件写。

### D5 路径口径与覆盖策略
**Decision**：source_file_path 默认库相对可选绝对；thumbnail_path 恒绝对；single_pptx_path 跟随 source 口径且探测存在性；目标已存在确认型错误 + overwrite 重调。

### D6 重链校验 = 必校验哈希 + 无自动匹配
**Decision**：哈希相等直接复活；不等 force 确认；hash 空跳过校验。不做扫描自动哈希匹配；批量移动靠前缀替换。「重建后哈希恢复」（§14.16）依赖备份，out of scope。

### D7 前缀替换 = 预览 + 越界遏制 + 执行后自动扫描
**Decision**：见 Requirements；定位专治库内子目录重组（库整体挪动只需改 root_path）。

### D8 PR 粒度 = 两批
**Decision**：PR1 导出（纯只读）+ PR2 丢失重链（扫描语义变更，lost 无重链为半成品态故同批完整交付）。

## Out of Scope

- OCR 实现本身（§2.7；字段留空位）。
- 批量导出幻灯片截图（§2.6.3 P1）。
- 素材库备份/加密（§2.7.5/6）；「重建后哈希恢复」随之 out of scope。
- 扫描时自动哈希匹配重链。
- lost 独立状态筛选器（仅统计/标记/详情呈现）。

## Technical Notes

- SRS 出处：docs/原始需求/202610-素材管理系统需求说明书-V1.0.md §2.6.4/§2.6.5/附录A/§14。
- 扫描链路：service/assets_scan.go scanLibrary（增量判定 mtime+size、消失清理 429-459 行区域）。
- schema：service/assets_schema.go（files 含 hash 列）；写入面 setFileOutcome/replaceFileSlides（assets_db.go）。
- 选中体系：assetSelectionKey（'file:<id>'/'slide:<fileId>:<slideNum>'）；批量目标收集 collectSelectionTargets 先例。
- 越界遏制先例：RetryAssetThumb IsAbs+Rel。
- 缩略图缓存键 `<fileID>-<3位页>.png`，档位目录 small/hd（导出取 small 档）；拆分产物 `<源文件名>/<源文件名>-NNN.pptx`。
- FTS 五契约不破；新 App 绑定 wails generate 三处同步；测试禁 sleep/mtime 时序/固定端口。
