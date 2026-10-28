# 连接管控十期：攻击源趋势面（审计历史聚合）

## Goal

九期网段聚合建议的数据源限定 entries 内存态——重启清零、仅窗口内事实，无法回答
「该网段攻击持续几天了」「解封后是否再犯」。十期基于审计行 `ip_ban`/`ip_unban`
做「审计历史 → 趋势洞察」：输出跨重启的攻击源时序视图（天桶粒度），补全九期
「窗口内事实」之外的长期面。

## Requirements

* 后端 service 层新增趋势聚合：读侧懒扫审计主+`.1`（复用 `readSnapshot` 单点），
  纯函数聚合核心可直测（对齐九期 `aggregateIPsToSuggestions` 先例）
* 聚合口径 doc comment 写死：
  - 主趋势计数仅 `event=ip_ban && actorSource=system`（机器观测攻击频度）；
    manual（desktop/rpc）分列「管理员处置 N 次」不混入
  - 单 IP 掩入九期固定粒度（IPv4 /24 + IPv6 /64，`::ffff:` 经 To4() 落 /24）；
    八期 CIDR 条目按规范化原串独立成桶；`ParseIP`+`ParseCIDR` 双败跳过
  - 九期 CoveredBy 剔除**不复用**（建议面=「你可能想封」故剔除；趋势面=观测面
    「谁在攻击」故保留），doc comment 写明分叉理由
  - 解封后再犯判定：桶内含被 `ip_unban` 过的 IP（unban 行 IP 掩同规则桶）且解封
    时间后该桶又有 auto `ip_ban`，`rebounded` 布尔后端计算透传，前端零判定
  - 时间桶恒为天（本地时区日界）；`earliestTs`=快照中最早 `ip_ban` 行 ts，
    前端判定 `earliest > 窗口起点` 才显示「数据自 M-D 起」提示（轮转丢历史
    诚实标注，禁静默降级）
* `IPBanSettings` 扩参 `TrendWindowDays`：默认 7 clamp 1-30（上限跟审计留存现实走）；
  `SetIPBanConfig` 六参扩七参，全仓 12 调用点同步；SettingsPanel 输入项
  （`--value-on-clear` 必带 + 保存后回读归一值刷新表单）
* App 新绑定 `QueryIPBanTrend()`（零参，窗口从 settings 读——单一来源，对齐
  `ListIPBanSuggestions` 零参先例），进 `server.RPCHiddenMethods` 屏蔽列表
* 前端 `useIPBanTrend()` composable：桌面专用 `isBrowserChannel()` 跳过、
  `webserve-ip-banned` 事件自动 reload、onMounted 首拉（对齐 `useIPBanList`/
  `useIPBanSuggestions` 三件套先例）；ActivityBar 第四区块「攻击趋势」：
  每行=桶（网段串 + 近 N 天总数 + 天级迷你条形 + manual 徽标 + rebounded 徽标），
  行点击 `openBanIPDialog(cidr)` 复用九期封禁弹窗链，区块底 earliestTs 提示行；
  `reloadBanPanel` 扩第三 reload
* wailsjs 三处同步（App.js / App.d.ts / models.ts）

## Acceptance Criteria

* [ ] 聚合口径单测：双 source 分列、双 kind 落桶（ParseIP/ParseCIDR/双败跳过）、
      `::ffff:` To4 归 /24、时间桶边界（本地时区日界、跨日）、窗口裁剪、
      earliestTs 语义、rebounded 判定三分支（unban 后再 ban / 后无 ban / 多 IP 桶）
* [ ] `SetIPBanConfig` 七参：全仓 12 调用点同步 + 六~九期测试零回归
* [ ] 隐私红线：`QueryIPBanTrend` 进 `RPCHiddenMethods` 命中测试（对齐九期先例）
* [ ] wailsjs 三处同步（models.ts 补 Trend class；无具名 string 类型别名坑，
      字段全基础类型）
* [ ] 前端组件测试 + `wails-mock-defaults.js` 补 `QueryIPBanTrend` 返回值
* [ ] spec 沉淀 browser-channel.md 十期段 + CLAUDE.md v2.38
* [ ] 无新错误码（审计旁路面，读失败走既有 error 链 + 前端吞错静默空态，
      对齐 `useIPBanList` 降级口径），确认 error.js 零改动

## Definition of Done

* 单测/组件测试覆盖聚合核心与前端分流
* 六~九期既有测试零回归
* go test / vitest / E2E 相关 spec 绿
* README.md 确认是否需更新（项目规则）

## Technical Approach

**后端**（PR1）
* `model/` 新增趋势 struct（`IPBanTrendResult`/`IPBanTrendBucket`，字段全基础
  类型：桶 key、days 计数序列、auto 总数、manual 数、rebounded、earliestTs）——
  Wails 绑定返回须走 model/
* `service/ip_ban_trend.go`：纯函数 `aggregateIPBanTrend(records, windowDays, now)`
  + 查询入口（挂 `IPBanService`，`AuditLogger` 装配注入——域归属 IP 封禁治理面；
  同包直接用 `readSnapshot` 私有方法，无需导出）
* `SetIPBanConfig` 扩七参（trendWindowDays 插位实现期定，倾向语义分组插
  whitelist 前）；`EnsureIPBanDefaults` 挂 Load/Save 双链路（现四处 Ensure，
  新增段同步两处——六期纪律）
* `app_webserve.go` 绑定 + `WebOptions.RPCHiddenMethods` 加 `QueryIPBanTrend`
* 前端签名零变更，仅 wailsjs 三处

**前端**（PR2）
* `useIPBans.js` 加 `useIPBanTrend()`（三 composable 共听同名事件，vitest
  EventsOn mock 按名收集+分发已升级可承载）
* ActivityBar 第四区块 + SettingsPanel 输入项

**收尾**（PR3）
* spec 沉淀 + CLAUDE.md v2.38 + README 确认

## Decision (ADR-lite)

**决策 1（数据面）**：读侧懒扫——面板打开时单次扫审计主+`.1` 流式聚合（复用
`readSnapshot` 单点）。零第二数据源零口径漂移；留存上限=审计留存（5MB×2 约
6 万行），轮转丢历史以 earliestTs 诚实标注；未来需更长回看时升级快照，接口
签名不变（演进路径预留）。已否决聚合快照持久化（第二数据源校准复杂度 +
双口径漂移教训）。

**决策 2（时间窗）**：扩参 `TrendWindowDays` 默认 7 clamp 1-30（上限跟审计留存
现实走，超留存窗口纯虚设）。对齐九期 `aggregateSuggestThreshold` 参数化先例。

**决策 3（过滤口径）**：主趋势仅 auto，manual 分列（auto=攻击频度、manual=
人类决策频度，混则口径失真）；单 IP 掩入固定粒度 /24·/64，CIDR 原串独立桶
（管理员封禁范围保真，kind 从 key 形态天然推导，无额外字段；CIDR 桶恒 manual
——自动封禁面无 CIDR）。

**决策 4（展示形态）**：ActivityBar 面板内独立「攻击趋势」小节（第四区块）。
行点击复用九期封禁弹窗链。时间桶恒为天（面板空间约束，小时粒度 168 桶不可容）。
已否决：九期建议行扩展（放不下天级分布）、独立 Dialog（多一层入口）。

**决策 5（九期联动）**：MVP 不动九期建议行，`ListIPBanSuggestions` 模型/前端
零改动，六~九期测试零回归最稳。附写死项：CoveredBy 剔除不复用（语义分叉）；
`rebounded` 再犯判定纳入 MVP（后端计算透传）。

**决策 6（隐私粒度）**：桶粒度封顶——载荷仅桶 key + 天级计数序列 + auto 总数 +
manual 数 + rebounded 布尔，无单 IP 明细。新方法进 `RPCHiddenMethods`，前端
`isBrowserChannel()` 跳过。单 IP 历史全量明细桌面已有入口（审计弹窗），趋势面
桶聚合正是差异化价值；对齐九期「仅网段串+计数无 IP 明细」纪律。

## Out of Scope

* 九期建议行叠加历史计数（联动留后续，接口不冲突）
* 聚合快照持久化 / 多份轮转链（审计留存上限不变，更久历史不做外部归档）
* 小时粒度桶、图表化 Dialog
* 趋势面单 IP 明细行（隐私粒度封顶决策）
* 新错误码（旁路面既有降级口径够用）

## Technical Notes

* 关键文件：service/audit_log.go（readSnapshot 复用点，同包私有可达）、
  service/ip_ban_aggregate.go（九期先例：纯函数+doc comment 口径纪律）、
  service/ip_ban.go（IPBanSettings 扩参位 + SetIPBanConfig）、app_webserve.go
  （绑定 + RPCHiddenMethods 装配）、model/ip_ban.go（九期
  IPBanAggregateSuggestion 先例）、frontend/src/composables/useIPBans.js、
  frontend/src/components/ActivityBar.vue、SettingsPanel（IPBan 段输入项先例）
* 审计写入时序纪律：仅动作生效后落盘——聚合读侧天然只见已生效事实
* `::ffff:x.x.x.x` 经 To4() 判定落 /24 桶（九期专项用例先例，趋势面同接）
* 天桶本地时区先例：BanIPDialog 自定义档「当日 23:59:59 本地时区与后端 nowFn
  同基准」；聚合函数注入 `now` 便于测试时钟驱动（禁 sleep）
* 测试禁隐式依赖环境外部状态（test-stability.md）：时间相关用例注入 now/时钟，
  禁真 sleep 等待
* 浏览器通道安全三期纪律：新增纯桌面数据面绑定方法必须同步
  `WebOptions.RPCHiddenMethods`（命中与「方法不存在」同响应）
