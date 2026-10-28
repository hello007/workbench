# 连接管控十一期：审计留存扩展与趋势回看保障（多档轮转链）

## Goal

审计日志留存上限现为 5MB×2（主 + `.1`，约 6 万行封顶），认证失败潮下数小时即轮转一轮，
十期趋势面 `TrendWindowDays` 默认 7 天的回看窗口名存实亡。十一期扩审计轮转链深度：
单一数据源（审计 JSONL）不变，读侧 `readSnapshot` 多档合并，`QueryIPBanTrend` /
`QueryAuditLog` 两消费面透明受益，趋势面跨重启回看真正撑满配置窗口。

## Requirements

* **R1 轮转链**：固定深度 N 档，主文件超限轮转时链式 rename（`.N-1`→`.N` … `.1`→`.2`，
  主→`.1`），超限淘汰最旧档（静默，设计语义非异常）；触发机制不变（纯大小触发）
* **R2 读侧多档**：`readSnapshot` 按 新→旧 全档合并（时间正序拼接语义不变），坏行跳过、
  缺档跳过、单文件 Scanner 错误截断该文件继续下一文件（既有容错口径继承）；不持 `a.mu`
  （读期间轮转「边界单行重复/漏读可接受」注释口径覆盖多档形态）
* **R3 AuditSettings 设置段**：`model/settings.go` 加 `Audit *AuditSettings`
  （`maxBytes` + `depth`），段缺失全默认向后兼容；`EnsureAuditDefaults` 挂 Load/Save
  双链路；clamp：maxBytes 1–20MB（默认 5MB 现值）、depth 2–10（默认 6）
* **R4 热更链**：显式 `SetAuditConfig`（normalize→复制基底换段→Save→生效注入
  AuditLogger 字段），禁走通用 SaveSettings 隐式联动；SettingsPanel 审计段两项
  （带 clamp 说明文案「历史合计约 N MB 封顶」）
* **R5 转正注释**：`webAuditMaxBytes` doc comment「不做多份轮转链」改为多档链语义 +
  十期 Out of Scope 转正理由
* **R6 零改动面**：`QueryIPBanTrend` 聚合口径、`QueryAuditLog` 分页与 Limit 钳、
  AuditHistoryDialog 前端、十期「数据自 X 起」提示行均零改动，透明受益
* **R7 降级口径**：写侧轮转失败 slog.Warn 继续追加（超限容忍，下条再试）在链式形态下
  复核仍成立；无新错误码（clamp 走 normalize 收敛非报错，对齐六期）

## Acceptance Criteria

* [ ] 轮转链单测：深度淘汰（超 depth 档淘汰最旧）/ 边界（恰好达限/未达限不轮转）/
      损坏行跳过 / 缺档跳过（中间档缺失链不断）
* [ ] 兼容单测：现网 `.1` 存量 + 新链混跑平滑升级（首次轮转 `.1`→`.2` 顺移），零迁移
* [ ] 读写竞速测试：读打开档文件句柄期间触发 rotateLocked rename（Windows
      MoveFileEx REPLACE_EXISTING 时序），轮转成功且读侧容错
* [ ] `QueryIPBanTrend` 既有单测（含 SnapshotDelegation）零改动通过
* [ ] `QueryAuditLog` 分页回归（含 AuditHistoryDialog spec）
* [ ] 六~十期连接管控测试零回归（`go test ./...` 全绿）
* [ ] AuditSettings 单测：段缺失默认 / clamp 收敛 / SetAuditConfig 热更生效
      （AuditLogger 字段更新 + settings.json 落盘）
* [ ] SettingsPanel 审计段 UI 用例
* [ ] spec 沉淀 logging-and-errors.md（审计域轮转链节）+ CLAUDE.md v2.39 关键规则行

## Definition of Done

* 单测覆盖新增分支；`go test ./...` 全绿；前端 `npm test` 全绿
* 文档同步：CLAUDE.md 关键规则行 + docs/spec/logging-and-errors.md
* 设置变更「保存并生效」链路真机口径与六期一致（保存后回读归一值刷新表单）

## Technical Approach

**核心改动**（单数据源不变，两消费面透明受益）：

1. `AuditLogger` 加 `depth` 字段（默认常量 6）；`rotateLocked` 改 rename 链：
   `for i := depth-1; i >= 1; i--` 逐档 `rename(path.i, path.(i+1))`（源不存在跳过），
   末尾 `rename(path, path.1)`；任一环失败即 Warn 停链（后续档保持现状，主文件继续
   追加，下条再试）——失败半态安全：链中断只影响该轮深度，不丢行不重排
2. `readSnapshot` 文件列表改按 depth 构造（`path.depth` … `path.1` + `path`），循环体
   不变；depth 变更后残留超深度旧档（如 depth 10→2 后的 `.3`~`.10`）不再入列表也不
   删除——静默滞留磁盘，容量口径以新配置为准（doc comment 落此取舍）
3. `AuditSettings`：`maxBytes`/`depth` 两 int 字段；normalize 收敛 clamp；
   `SetAuditConfig` 六期 SetIPBanConfig 同款「复制基底换段+私有拷贝其他指针段」；
   装配层 NewAppServices 启动初值注入 + SetAuditConfig 后实时更新 AuditLogger 字段
   （maxBytes/depth 均原子性不敏感：a.mu 锁内读写即可）
4. 热更生效点：depth 缩小时旧档滞留（见 2）；depth 扩大时新档位天然启用——无重排需求

**读性能量级**：默认档最坏 35MB JSONL 扫描，毫秒~几十毫秒，懒扫（面板打开）频度低，
不缓存（缓存失效依赖审计写事件，与 test-stability 手动入口纪律冲突）。

## Decision (ADR-lite)

### D1 轮转链形态 = 固定深度 N 档

**Context**：容量/读性能/实现复杂度三角权衡；十期 Out of Scope「多份轮转链不做」转正。
**Decision**：固定深度 N 档，超限链式 rename，淘汰最旧档；触发机制不变（纯大小触发）。
否决时间双触发（失败潮形态大小先爆，时间触发只产空档碎片档+时钟注入复杂度）与
.gz 压缩（压缩持 a.mu 阻塞审计写、读侧解压开销、容错复杂化，磁盘压力本可由 depth
控制）。
**Consequences**：容量确定 (depth+1)×maxBytes 封顶；淘汰最旧与趋势面 earliestTs
自然前移收敛吻合，十期前端提示行无需改口径；最坏 N 次 MoveFileEx 锁内执行（本地
同盘微秒级，可忽略）。

### D2 参数化分域 = AuditSettings 独立设置段

**Context**：审计域现无 settings 段；六期 IPBanSettings 已确立参数化先例；写入速率
用户不可预知，固定常量下「撑满 TrendWindowDays」无法保证。
**Decision**：`Audit *AuditSettings` 段（maxBytes + depth），段缺失全默认向后兼容；
`EnsureAuditDefaults` 挂 Load/Save 双链路；clamp maxBytes 1–20MB、depth 2–10，
最坏总容量 200MB 封顶；热更走显式 `SetAuditConfig`。
**Consequences**：无新错误码；SettingsPanel 补用例；生效点 AuditLogger 字段锁内更新。

### D3 QueryAuditLog 分页体验 = 维持现状（派生）

**Context**：更大历史下翻页深度上升（30MB≈18 万行，page-size 100 最坏 1800 页）。
**Decision**：不改。AuditHistoryDialog 已有完整分页 UI（el-pagination + Offset/Limit
透传 + Total 显示 + 越界页回退），筛选（event/token/时间）天然收敛翻页深度；
「翻页点击多」为既有 UX 特性非本任务引入。
**Consequences**：零改动；验收仅回归现有分页测试。

### D4 读性能 = 不做 readSnapshot 缓存（派生）

**Context**：懒扫每次面板打开全量扫 N 档，最坏 35MB（默认档）。
**Decision**：不缓存。扫描毫秒~几十毫秒、懒扫频度低；缓存失效必然依赖审计写事件
（每行都变文件），与「缓存失效由手动入口驱动不依赖 mtime 时序」纪律冲突。
**Consequences**：无新增失效链路；性能以量级评估备注（非测试断言）。

### D5 淘汰与兼容 = 静默淘汰，零迁移（派生）

**Context**：现网已存 `.1`；淘汰最旧档后趋势面 earliestTs 自然前移。
**Decision**：rename 链天然衔接（`.1` 存在时首次轮转即 `.1`→`.2` 顺移，目标不存在时
rename 等价平移），零迁移脚本；淘汰最旧档静默无专门告警；趋势面「数据自 X 起」提示
自然收敛，十期前端零改动——验证口径而非改口径。
**Consequences**：验收含 `.1` 存量混跑兼容单测；十期提示行行为断言零 diff。

### D6 磁盘守护 + 默认档位 = 无专门告警 + depth 默认 6

**Context**：淘汰最旧档是否需要观测痕迹；默认总容量预算与 7 天窗口撑满性。
**Decision**：无淘汰告警（设计语义非异常），保留既有「轮转失败 slog.Warn」唯一降级
痕迹；设置面板项带 clamp 说明文案。默认 depth=6（30MB 历史 + 主 5MB = 35MB）：
正常频度（每天几十~几百 KB）7 天窗口撑满无压力；失败潮（约 45MB/天）下撑不满，
「数据自 X 起」诚实降级为十期设计语义，用户自调 maxBytes×depth 应对（上限 200MB）。
**Consequences**：失败潮极端形态下提示行仍会出现（诚实语义，非缺陷）；日志零新增噪音。

## Out of Scope

* 审计行外部归档（导出/同步到外部存储）
* 趋势面聚合口径改动（十期语义原样）；审计快照持久化（十期已否决，维持）
* readSnapshot 结果缓存
* QueryAuditLog 分页 UI 增强
* .gz 压缩归档 / 时间触发轮转
* depth 缩小时残留旧档的自动清扫（静默滞留，容量口径以新配置为准）

## Technical Notes

* 关键文件：`service/audit_log.go`（rotateLocked/readSnapshot/常量/depth 字段）、
  `model/settings.go`（AuditSettings）、`service/settings*.go`（EnsureAuditDefaults/
  SetAuditConfig）、`app_services.go`（装配注入）、`frontend/src/components/
  SettingsPanel.vue`（审计段 UI）
* 读侧不持 `a.mu`：Windows 下 Go os.Open 使用 FILE_SHARE_DELETE，rename-over-open
  可成功；既有「边界单行重复/漏读可接受」注释覆盖多档形态
* 审计写路径持 `a.mu`：rename 链在锁内但微秒级；.gz 方案否决依据即锁内压缩不可长持
* 六期纪律参照：`EnsureIPBanDefaults` 现四处 Ensure（Load/Save 双链路）；`SetIPBanConfig`
  「复制基底设置换段+私有拷贝其他指针段」形态；SettingsPanel `el-input-number` 必带
  `:value-on-clear` 防清空写 null 脏值；保存后回读归一值刷新表单防展示漂移
* 单文件 5MB 约 3 万行（现有注释口径）；30MB≈18 万行
