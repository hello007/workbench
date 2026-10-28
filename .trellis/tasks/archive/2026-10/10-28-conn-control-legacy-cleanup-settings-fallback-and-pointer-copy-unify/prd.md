# 连接管控遗留收口：设置降级分支与指针段拷贝统一

## Goal

十一期 check 阶段登记两处既有形态遗留（App.GetSettings 降级分支缺 IPBan/Audit 两段 Ensure；SetIPBanConfig 指针段拷贝漏 Audit 段），另据同类形态全仓盘点发现第三处（SetWebServeConfig 漏拷 IPBan/Audit 两段）。本任务一次性收口，并以单点 helper 消灭「新增设置段时手抄遗漏」的模式性风险，spec 沉淀 checklist 规则行。

## What I already know（代码盘点结论）

### AppSettings 指针段全集（model/settings.go:391-403）

`WebServe` / `Assets` / `IPBan` / `Audit` 四个指针段（`Terminal` 外观三字段为非指针标量段，不在拷贝纪律范围）。

### 设置写入口拷贝面盘点

| 写入口 | WebServe | Assets | IPBan | Audit | 缺口 |
|---|---|---|---|---|---|
| `SetWebServeConfig`（app_webserve.go:56） | 重建（保 TLS 三字段） | ✅私有拷 | ❌**共享缓存指针** | ❌**共享缓存指针** | IPBan+Audit 两段 |
| `SetIPBanConfig`（app_webserve.go:637） | ✅私有拷 | ✅私有拷 | 整体替换（天然私有） | ❌**共享缓存指针** | Audit 一段 |
| `SetAuditConfig`（app_webserve.go:687） | ✅私有拷 | ✅私有拷 | ✅私有拷 | 整体替换（天然私有） | 无 |
| `SaveSettings`（app_settings.go:31） | 前端反序列化私有对象，无共享问题 | — | — | — | 无 |

### 降级分支盘点

`App.GetSettings`（app_settings.go:17-28）settingsSvc.Load 失败降级分支仅 Ensure WebServe/Terminal/Assets 三段，**缺 EnsureIPBanDefaults（六期起）+ EnsureAuditDefaults（十一期）**。现状无 panic（nil 守卫）+ 前端回落默认，但「Load 返回的配置段始终完整可用」不变式（service/settings.go 注释明示）在降级分支破口。

### Ensure 五连链现状（手抄两处）

`service/settings.go` Load miss 分支（53-65 行）与 Save 入口（78-82 行）各手抄一遍五段 Ensure 串联。历史证明手抄必漏：六期加 IPBan 段漏降级分支、十一期加 Audit 段漏降级分支 + SetIPBanConfig、SetWebServeConfig 漏后起两段——同一模式连续漏四处。

### 竞态性质（为何无实际危害仍须修）

共享指针段经 `Save` 入口 Ensure 的 normalize 无条件字段赋值（写值=已发布值），与并发读者（GetSettings 缓存命中）构成无同步写读 data race。值恒等故现网无实际危害，但违反 service/settings.go:20-23 并发口径「消费方只读禁改」纪律。

## Decision (ADR-lite)

**Context**：手抄 Ensure 串联与指针段私有拷贝已连续漏四处（六期加段漏降级分支、十一期加段漏降级分支 + SetIPBanConfig、SetWebServeConfig 漏后起两段）——模式性风险坐实，逐处补丁下一新段大概率再漏。

**Decision**（用户任务描述已暗示 helper 方向，AI 推荐收敛形态，最终确认中）：

1. **双 helper 单点收敛**：
   - `model.AppSettings.EnsureAllDefaults()`——串联五段 Ensure；消费点三处（service/settings.go Load miss 分支 / Save 入口、app_settings.go 降级分支），新增段仅此一处加行。
   - `model.AppSettings.PrivateCopy() AppSettings`——值拷贝 + 全部四指针段私有化；消费点三处（SetWebServeConfig / SetIPBanConfig / SetAuditConfig），新增段仅此一处加行。
2. **helper 落 model 层**（非 main 包私有函数）：五个 EnsureXxx 已在 model/settings.go，串联同层自然；model 层单测直接断言「全段非 nil / 全指针段私有」，新增段测试同文件钉住；跨包可用（service 层 Load/Save 亦消费 EnsureAllDefaults）。
3. **spec 规则行**：CLAUDE.md 关键规则表加行 + 详细 checklist 落 `docs/spec/cross-layer-contracts.md`（设置段增删的跨层同步面：EnsureAllDefaults + PrivateCopy + 前端 models.ts 三处单点清单）。

**Consequences**：diff 面略大于补丁式（含 helper 抽取与三处消费点改写 + 注释同步），但「新增设置段遗漏」从结构性必漏变为单点防漏；六期 doc comment「现五处 Ensure，新增段须同步两处」表述须随代码更新。

## 测试策略转折（盘点发现）

`SettingsService.Load` 现实现**无任何路径返回非 nil error**（磁盘读失败也吞错降级空设置，service/settings.go:46-50），且 `App.settingsSvc` 为具体类型非接口——验收底线的「Load 失败注入」不可直接构造。对策：降级分支体缩为一行 `settings.EnsureAllDefaults()`，分支正确性由 model 层单测钉住（零值 `AppSettings{}` 调 `EnsureAllDefaults` 后五段全非 nil 且默认值正确）；App 层不再为该分支单独造失败注入设施。

## Requirements（evolving）

- `model.AppSettings.EnsureAllDefaults()`：串联五段 Ensure 单点；Load miss / Save / GetSettings 降级分支三处消费点改调
- `model.AppSettings.PrivateCopy() AppSettings`：值拷贝 + 四指针段（WebServe/Assets/IPBan/Audit）全部私有化；三个设置写入口消费点改调（SetWebServeConfig 拷贝后仍重建 WebServe 段保 TLS 语义不变）
- GetSettings 降级分支五段全补全（IPBan + Audit 纳入，经 EnsureAllDefaults）
- SetIPBanConfig 补 Audit 段私有拷贝；SetWebServeConfig 补 IPBan/Audit 段私有拷贝（经 PrivateCopy 自动达成）
- 相关 doc comment 同步（「现五处 Ensure，新增段须同步两处」等表述随单点化更新）

## Acceptance Criteria（evolving）

- [ ] model 层单测：零值 `AppSettings{}` 调 `EnsureAllDefaults()` → 五段（WebServe/Terminal/Assets/IPBan/Audit）全非 nil 且默认值正确
- [ ] model 层单测：`PrivateCopy()` 返回值四指针段全非 nil 且与原对象指针不同址（`&copy.WebServe != &orig.WebServe` 等价断言）；新增段时同文件补断言
- [ ] SetIPBanConfig 补拷后 Audit 段独立性断言（改返回值不影响缓存指针内容）
- [ ] SetWebServeConfig 补拷后 IPBan/Audit 段独立性断言；TLS 三字段保留语义回归
- [ ] 六~十一期既有测试零回归
- [ ] spec 沉淀规则行（CLAUDE.md 关键规则表 + docs/spec/cross-layer-contracts.md checklist）
- [ ] CLAUDE.md 更新 v2.40
- [ ] 无新错误码

## Definition of Done

- 单测覆盖上述断言面；lint/CI 绿
- docs/spec 规则行 + CLAUDE.md 版本号推进

## Out of Scope

- 不改 SettingsService 缓存语义与并发口径本体（只收敛其消费侧纪律）
- 不动 SaveSettings 前端整体保存链路（无共享问题）
- 无新错误码、无前端改动

## Technical Notes

- 关键文件：app_settings.go / app_webserve.go / service/settings.go / model/settings.go
- EnsureXxx 五函数现位于 model/settings.go（EnsureWebServeDefaults:412 / EnsureTerminalDefaults:431 / EnsureAssetsThumbsDefaults:100 / EnsureIPBanDefaults:265 / EnsureAuditDefaults:344）
- 竞态纪律出处：service/settings.go:20-23 并发口径注释
