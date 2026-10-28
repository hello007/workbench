# 连接管控九期：攻击源网段聚合建议

## Goal

七期落地 IP 手动封禁 + 白名单豁免，八期落地 CIDR 网段手动封禁，但「同网段多攻击源」
需管理员自行发现——分散的单 IP 自动封禁记录不产生网段级洞察。九期做「自动观测 →
手动处置」建议闭环：同聚合网段内自动封禁条目达到阈值时，在处置面（ActivityBar
IP 封禁区块）提示「该网段 N 个来源已被封禁，建议网段封禁」，一键预填 BanIPDialog
网段档完成处置。闭环止于预填，处置仍需管理员确认提交（不自动封禁）。

## Requirements

1. **聚合口径**：固定粒度 IPv4 /24、IPv6 /64（写死）；新增独立 App 绑定方法
   `ListIPBanSuggestions()` 返回聚合建议，`ListIPBans` 签名不动
2. **数据源口径**（写死进聚合函数 doc comment）：`s.entries` 内存态锁内遍历，
   仅 `source=auto` 且 `bannedAt` 非零条目参与（**含已过期**，与 List 可见条目
   同口径——所见即所计）；CoveredBy 命中网段封禁的条目剔除（`cidrCoveredBy`
   复用，防重复建议）；ParseIP 失败条目跳过（防御，normalizeIP 后理论不可达）；
   不用审计行（含已解封历史 + 轮转边界漂移，口径糊）
3. **阈值**：网段内参与条目数 ≥ threshold 产出建议；`IPBanSettings` 扩
   `aggregateSuggestThreshold`（默认 3，clamp 2-50），经 ApplyConfig 链注入
   service（对齐六期四参数先例，参数形态实现时对齐现状）
4. **展示**：ActivityBar IP 封禁区块内独立「聚合建议」小节（封禁列表上方）；
   排序计数降序 + 网段串字典序次级；不加展示上限；文案「该网段 N 个来源已被
   封禁，建议网段封禁」
5. **一键预填**：建议行点击 → `openBanIPDialog(initialCidr)` → BanIPDialog
   `targetKind='cidr'` + 网段输入预填；时长档不预填（默认永久档）；白名单预检
   三态复用网段档现成逻辑；`initialIP` 与 `initialCidr` 互斥（每次打开只传其一）
6. **隐私红线**：`ListIPBanSuggestions` 进 `RPCHiddenMethods`（浏览器通道与
   「方法不存在」同响应，整体不可达无降级 UI）；载荷仅网段串 + 计数，无具体
   IP 明细
7. **无新增错误码**：纯只读查询面，空态返回空切片

## Acceptance Criteria

* [ ] 聚合纯函数单测：IPv4 聚合 / IPv6 聚合 / 单条目不触发（阈值 N-1/N 边界）/
      CoveredBy 命中剔除 / manual 条目剔除 / ParseIP 失败跳过 / 排序计数降序
      + 字典序次级
* [ ] 服务查询面单测：entries 混合 auto/manual/cidrBans 场景口径正确；阈值热更
      生效（ApplyConfig 链）
* [ ] 隐私红线：RPCHiddenMethods 命中测试（对齐 app_audit_history_test.go:199 先例）
* [ ] SetIPBanConfig 扩六参全仓调用点同步，六期/七期测试零回归
* [ ] BanIPDialog initialCidr 预填 + 档位自动切网段档（vitest）；initialIP 路径
      零回归
* [ ] ActivityBar 建议小节渲染 + 点击预填链路（vitest；E2E 如适用按桌面通道
      事件时序先例）
* [ ] wailsjs 三处同步（`wails generate module`；`IPBanAggregateSuggestion` 为
      struct 会自动生成 class，无需手补别名——与 `type X string` 具名别名规则区分）
* [ ] spec 三文档更新（browser-channel.md §2.3 九期段 + settings 相关 +
      frontend-visual-conventions 如涉及）+ CLAUDE.md v2.37

## Definition of Done

* 后端单测：聚合纯函数 + 服务查询面 + App 委托层 + RPCHiddenMethods 红线
* 前端 vitest：BanIPDialog 预填/档位切换 + ActivityBar 建议小节
* lint / go test / npm test 全绿；八期测试零回归
* docs/spec + CLAUDE.md 同步；README 如涉及确认更新

## Technical Approach

**后端**：
* `model.IPBanAggregateSuggestion` struct（`CIDR string` + `Count int`，全基本
  类型；仅桌面出口不进浏览器通道）
* service 层：锁内过滤 entries（auto + bannedAt 非零 + ParseIP 成功 +
  cidrCoveredBy 未命中）→ 纯函数 `aggregateIPsToSuggestions(ips, threshold)`
  分桶计数（IPv4 /24、IPv6 /64）→ 阈值过滤 → 排序；阈值经 ApplyConfig 注入
  service 字段（七字段，对齐六期先例）
* App 绑定 `ListIPBanSuggestions()` 委托 service（app_webserve.go，对齐
  ListIPBans 形态）+ `RPCHiddenMethods` 追加方法名（main.go 装配处）
* `SetIPBanConfig` 五参扩六参 + `ApplyConfig` 注入 + SettingsService 无观察者
  约束不变（保存即生效单点延续）

**前端**：
* `useIPBanSuggestions` composable（对齐 useIPBanList；桌面通道专用）
* ActivityBar IP 封禁区块「聚合建议」小节：建议行（网段串 + N + 提示文案）+
  点击回调 `openBanIPDialog(initialCidr)`
* BanIPDialog：加 `initialCidr` prop；watch modelValue 打开时按 prop 分流预填
  + `targetKind` 自动切网段档（对齐 initialIP 先例）

## Decision (ADR-lite)

**Context**：七/八期覆盖单 IP 与网段手动封禁，缺「同网段多源自动封禁」的网段级
洞察；管理员需自行交叉比对分散条目。

**Decision**（brainstorm 七决策点结论）：
1. 固定粒度 /24、/64 + 独立查询面（非动态推导/非并入 List——粒度可预期、
   不破签名、隐私面整体收口）
2. ActivityBar 建议行（非 toast——常驻可见；toast 转瞬即逝且污染事件契约）
3. entries 内存态（非审计行——所见即所计、重启清零自洽；仅 auto、含已过期、
   CoveredBy 剔除）
4. 阈值可配置（扩 IPBanSettings，六期先例机械活）
5. RPCHiddenMethods 整体隐藏（非 count 降级——对齐他人 IP 明细红线）
6. initialCidr 预填 + 档位自动切（对齐 initialIP 先例）
7. CoveredBy 剔除防重复建议

**Consequences**：ListIPBans 签名与 webserve-ip-banned 事件零变更（兼容面零）；
SetIPBanConfig 签名变更需全仓同步（机械）；审计行历史聚合（趋势面）留待后续
期次；建议闭环止于预填，网段误封风险由管理员确认环节兜底（白名单预检三态
现成）。

## Out of Scope

* 自动执行网段封禁（止于预填，处置须管理员确认）
* 审计行历史聚合 / 时序趋势分析（数据源 B 留待十期候选）
* 聚合建议推送通知（webhook 等）
* 令牌封禁建议（同令牌多源失败语义不同源，不并）
* 聚合建议的 webserve-ip-banned 类事件推送（面板打开时拉取即可）

## Technical Notes

* 参与口径边界：白名单网段天然无 entries（RecordFailure 早退不建条目），
  无需特判；normalizeIP 后 ParseIP 失败理论不可达仍防御跳过
* `IPBanAggregateSuggestion` 为 struct，wails generate 自动生成 models.ts
  class，无需手补别名（区别于 `type X string` 具名别名规则）
* 六期测试时钟注入教训不适用（本功能无时钟驱动面；已过期判定与 List 同源）
* ActivityBar 建议小节涉及面板可见条件（panelVisible 现为
  `multiDevice || ipBans.length>0`，有建议无条目的形态理论不可达——建议派生自
  条目，不扩条件）
* 相关 spec：docs/spec/browser-channel.md §2.3（五~八期全记录）+ §2.2
  （HiddenMethods 三期定稿）+ model/settings.go IPBanSettings（六/七期注释）
