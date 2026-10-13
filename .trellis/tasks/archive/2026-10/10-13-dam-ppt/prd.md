# 侧边栏素材管理——本地 DAM 素材库 + PPT 单页索引（MVP 最小纵切）

## Goal

WorkBench 侧边栏（ActivityBar）新增「素材管理」面板，落地《素材管理系统需求说明书 V1.0》（docs/原始需求/202610-素材管理系统需求说明书-V1.0.md）的 MVP 纵切片：本地 DAM 素材索引库 + PPT 幻灯片单页级索引与中文检索，对标 Billfish，原始文件不动、仅建索引。首批纯 Go 零 COM，先验证数据模型与检索链路。

## What I already know

### 代码库现状（已探明）
- 侧边栏：`frontend/src/components/ActivityBar.vue` `panels` 数组注册，现有 6 面板（directory/ai/ai-chat/toolbox/dashboard/stats）；`useSessionState.js` `VALID_PANELS` 集合；ContentPanel 按面板 id 分支渲染。新增面板 = 三处登记 + 新 SFC + 图标 SFC（深色底内联 SVG fill=currentColor，见 frontend-visual-conventions）。
- **PPT 拆分已有纯 Go OOXML 实现**（`service/pptx_split*.go`、`pptx.go`）——SRS「拆分依赖 PowerPoint COM」段可被现有 Go 实现替代。
- go.mod 无 SQLite 驱动；data/ 持久化走 `util.SaveJSON` 原子写 JSON，无数据库。
- Linux 支持约束：COM 相关必须 Windows-only（`_windows`/`_other` 双文件）。
- AppServices 装配模式：新 service 须登记 `app_services.go` 两处；带事件的 service 须同步 `web_serve.go` `applySink`。
- 绑定同步：App.js / App.d.ts / models.ts 三处。

### 复用资产
- `service/pptx*.go`：OOXML 解析能力（页文本提取可基于同链路扩展备注提取）。
- `util/testutil`：测试辅助；EventSink 事件通道；cancels 取消注册表模式；`model.AppError{Code,Message}` 错误分流。

## Decision (ADR-lite)

### D1：MVP = 最小纵切（2026-10-13 用户确认）
**Context**：SRS P0 有 12 项，全量数周量级。
**Decision**：首批只做——素材库创建 + 目录扫描 + 纯 Go PPT 文本解析 + 幻灯片单页索引 + FTS5 关键词检索（命中单页）+ 画廊网格视图（占位卡）+ 文件级标签/评分/备注。不含 COM 缩略图、拆分 UI、导出、备份、跳转打开。
**Consequences**：纯 Go 可测可交付；缩略图/导出/单页元数据编辑为第二批。

### D2：标签体系 = 平铺标签（默认值）
单级标签 + 自定义颜色 + 多选筛选；`tags.parent_id` 字段保留不建 UI，多级树二批。

### D3：素材库 = 多库切换（默认值）
`libraries` 表 + 库切换下拉，每库独立索引与标签体系。跨库检索仍 Out of Scope（SRS P1）。

### D4：查询层标签继承 MVP 形态
幻灯片无自有标签时，筛选幻灯片 = 按归属文件标签过滤（继承的唯一实现形态）；单页自有标签二批落地时同步加「仅幻灯片自身标签」开关。

### D5：扫描过滤与异常降级 MVP
后缀白名单 `pptx`；派生产物强制排除（拆分缓存目录不入索引）；`.damignore` 用户配置二批。状态机含 加密/损坏/权限不足/占用 跳过分类，单文件失败不中断批量任务。

### D6：视图 MVP
仅画廊网格视图（占位卡：幻灯片标题 + 页码 + 命中摘要），同一组件复用为检索结果视图；列表视图二批。详情侧边栏：元数据 + 标签/评分/备注编辑 + 幻灯片溯源（跳转归属文件条目）。单选；批量操作二批。

### 存储与检索选型（见 [research/sqlite-fts5-chinese.md](research/sqlite-fts5-chinese.md)）
- SQLite 驱动：`modernc.org/sqlite` v1.60.1+（纯 Go 零 CGO，`CGO_ENABLED=0` 交叉编译实测通过，FTS5 默认内置）。
- 检索：**方案 A = FTS5 + unicode61 + 应用层 CJK 按字切分 + phrase 查询 + bm25 字段加权**。10 万行实测 p95 11.6–24.9ms。trigram 对 2 字词（如「获客」）静默 0 命中，弃。
- FTS 表形态：standalone FTS 表 + 事务双写。
- 已知坑：切分函数写入/查询必须同源；FTS 表禁用隐藏列 LIKE；`detail=full` 必须保留；snippet 输出含切分空格需后处理。

## Requirements

1. ActivityBar 新增「素材管理」入口面板（assets）。
2. 素材库管理：新建库（选目录）、多库切换、删除库；索引模式（不动原始文件）。
3. 目录扫描：递归扫 `pptx`，增量（mtime+size 判变，变更才哈希）；手动全量重建；单文件异常降级（加密/损坏/权限/占用 → 跳过标记，不中断批量）；扫描进度事件（EventSink，可取消——复用 cancels 注册表模式）。
4. PPT 解析（纯 Go OOXML）：每页文本、标题（回退链：标题占位符 → 首行短文本(>3字) → 文件名+页码，空标题 `[无标题]`）、演讲者备注；删除文件同步清幻灯片记录。
5. 数据层：SQLite（modernc 纯 Go），表 libraries/files/slides/tags/file_tags + FTS5 standalone 双写表；库文件落 `data/assets/`。
6. 检索：关键词全文检索（文件名/标题/页文本/备注，CJK 按字切分 phrase + bm25 加权：文件名 > 标题 > 文本 > 备注）；标签筛选（文件标签继承至幻灯片）；结果区分 文件/幻灯片 类型标记；同 PPT 多页命中聚合折叠（可切平铺）。
7. 视图：画廊网格（占位卡）、详情侧边栏（元数据/标签/星级/备注编辑、幻灯片溯源）；文件/幻灯片两种浏览模式。
8. 元数据编辑（文件级）：标签多选、星级 1-5、备注文本。

## Acceptance Criteria

对照 SRS 第 8 节裁剪（MVP 可测子集）：

- [ ] 新建素材库选择目录，扫描后 pptx 文件与全部幻灯片入索引，原始文件未被修改（mtime/hash 不变）
- [ ] 画廊视图可切换【文件视图】/【幻灯片视图】，占位卡展示标题+页码
- [ ] 输入「城商行」等中文词可命中幻灯片页文本，结果区分文件/幻灯片类型，命中页展示标题与摘要
- [ ] 同一 PPT 多页命中可折叠为「N 页命中」，可展开；可切平铺
- [ ] 文件级标签（带颜色）/星级/备注可编辑并在详情侧边栏回显；按标签筛选文件与幻灯片（继承）生效
- [ ] 加密/损坏 pptx 被标记跳过，不中断批量扫描，日志可查
- [ ] 重新扫描仅处理新增/变更文件（mtime+size 未变跳过）；删除文件后其幻灯片记录同步清除
- [ ] 扫描进度事件前端可见，扫描可取消
- [ ] 多素材库创建/切换/删除，各库标签体系隔离
- [ ] 检索 P95：10 万幻灯片行 < 1s（用 research 同规格基准测试守护）
- [ ] Linux（`GOOS=linux`）编译通过，COM/缩略图代码不参与编译

## Definition of Done

- 新 service 单测覆盖 ≥76% 门禁；FTS 切分函数与检索链路有 10 万行级基准测试（`-short` 缩减档）
- `wails generate module` 绑定三处同步；`npm run build` 通过
- Linux 构建通过（`_other` 桩）
- README.md / docs/功能说明.md 更新
- 新增错误码同步 `model/app_error.go` + `frontend/src/utils/error.js`

## Out of Scope (explicit)

- PowerPoint COM 缩略图、打开跳转页码（第二批）
- 幻灯片单页独立标签/评分/备注编辑（P1；本批只读继承文件标签）
- 拆分 UI（素材库内复用已有 pptx_split service 的入口二批再加）
- 导出 CSV/Markdown YAML、数据库备份
- OCR、颜色检索、以图搜图、目录监听、素材库加密、跨库检索（P1/P2）
- 列表视图、批量操作、`.damignore` 自定义忽略、复制模式导入
- 文件丢失检测与哈希重链（P1）

## Technical Notes

- SRS：docs/原始需求/202610-素材管理系统需求说明书-V1.0.md
- FTS5 中文方案：research/sqlite-fts5-chinese.md（含 10 万行实测数据、坑清单、建表建议）
- 建议拆分（子任务）：
  - PR1：数据层 + 素材库/扫描/解析 service + AppServices 装配（纯后端 + 测试）
  - PR2：检索链路（FTS5 + 切分 + bm25 + 聚合）+ 基准测试
  - PR3：前端面板（ActivityBar 登记、画廊视图、详情侧边栏、检索交互、绑定同步）
- 风险：modernc/sqlite 库文件体积（~267MB/10万行 FTS，缓存目录管理）；Wails 桥大结果集分页策略
