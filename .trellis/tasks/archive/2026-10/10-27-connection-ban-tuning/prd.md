# 连接管控六期（自动封禁治理增强：参数化 + 递增封禁）

## Goal

五期 IP 认证失败自动封禁的三参数（阈值/窗口/时长）硬编码常量（`service/ip_ban_service.go` `IPBanThreshold=10/IPBanWindow=10min/IPBanDuration=5min`），运营无法按真实误伤/攻击面调参；且固定时长对持续攻击无成本递增。六期补两能力：

1. **参数设置项化**：参数入 `AppSettings`，设置页可调，clamp 收敛，段缺失全默认，改动即时生效（无需重启）。
2. **递增封禁**：同一 IP 反复触发封禁时，封禁时长按档位递增（反复触发封越久），封顶上限防失控。

## Decision (ADR-lite)

**四项决策全部采纳推荐项（2026-10-27 用户定稿）**：

| 决策点 | 定稿 |
|---|---|
| Q1 clamp 边界与封顶 | threshold 默认 10 clamp **3-100**（下限 3 防偶发误伤）；windowMinutes 默认 10 clamp **1-120**；durationMinutes 默认 5 clamp **1-60**；封顶 maxBanDurationMinutes 默认 60 clamp **5-1440**——**封顶参数化**，四项全暴露设置页 |
| Q2 档位公式 | **指数翻倍** base×2^(n-1) 封顶 → 5/10/20/40/60；实现用循环翻倍 clamp（禁 pow 防大 strikes 溢出） |
| Q3 递增记忆（strikes） | **保留至 UnbanIP / ClearAll / 重启**；到期恢复后窗口重新计数（既有剪枝语义）但下次触发按 strikes+1 递增；认证成功清失败计数**不清 strikes**（拿到令牌的合法登录不代表攻击停止）；解封/轮换联动 ClearAll 全清=显式重置；重启清零对齐内存态纪律 |
| Q4 参数变更生效语义 | **仅面向未来**——生效中封禁 bannedUntil 不变（已执行动作不追溯，改小时长不提前释放攻击者，改大不延长）；在途失败计数不清（调参不意外赦免）；threshold 改小后存量计数自然满足，下次失败即触发，无主动扫描 |

**两项技术决策（先例明确，无分叉价值，AI 直接定）**：

- **热更通道**：专用绑定方法「保存 + 生效」显式单点 `SetIPBanConfig`（对齐 `SetWebServeConfig` 先例——`SettingsService` 无观察者机制，通用 `SaveSettings` 联动会引入隐式耦合）。`IPBanService` 三常量转服务字段（mu 锁内保护），`ApplyConfig` 持锁整体替换；启动装配从 settings 注入初值。
- **段位置**：`AppSettings.IPBan *IPBanSettings` 独立指针段（对齐 WebServe/Assets 先例：段缺失全默认、`EnsureIPBanDefaults` 于 Load/Save 双链路补全、`clampInt` 零值回退默认/越界收敛）。

## Requirements

### R1 后端参数段（model）

- `model/settings.go` 新增 `IPBanSettings{Threshold, WindowMinutes, DurationMinutes, MaxBanDurationMinutes int}`（全 int，分钟口径，对齐 Assets 五项风格）
- 常量四组 Default/Min/Max（10/3/100、10/1/120、5/1/60、60/5/1440）
- `EnsureIPBanDefaults`（指针段缺失→全默认；存在→零值回退默认、越界 clamp）；挂 `Load`/`Save` 双链路（对齐 EnsureWebServeDefaults/EnsureAssetsThumbsDefaults 三处 Ensure 模式）
- `NormalizeIPBanSettings` 导出入口（nil 安全、拷贝 normalize 不改写入参——共享指针纪律）

### R2 后端参数热更（service）

- `IPBanService` 三常量转私有字段（threshold/window/duration/maxBanDuration，mu 锁内保护；导出常量保留作默认值与文档）
- `ApplyConfig(cfg model.IPBanSettings)`：mu 锁内整体替换四参数（normalize 由调用方或内部 Ensure 保证）；不动 entries（Q4 语义）
- `RecordFailure` / `auditIPBanLocked` 消费字段值（窗口剪枝、阈值判定、封禁时长、reason 文案）

### R3 递增封禁（service）

- `ipBanEntry` 扩 `strikes int`（累计触发次数）
- 触发封禁时 `strikes++`；封禁时长 = `min(base × 2^(strikes-1), maxBanDuration)`，循环翻倍 clamp 实现
- 到期恢复：既有剪枝语义（failures 失效）不变，strikes 保留
- `RecordSuccess`：清 failures 不清 strikes（既有「有封禁历史条目保留」路径天然满足，仅确认不清 strikes）
- `UnbanIP`（删条目）/ `ClearAll`（整体重建 map）：strikes 一并归零
- 内存态，重启清零（五期纪律）
- 溢出安全：翻倍循环中 clamp 到 maxBanDuration 后停止（strikes 大值恒封顶）

### R4 状态与审计扩展

- `model.IPBanStatus` 扩 `Strike int`（json `strike`，第 N 次触发）；`List`/`SetOnBanned` 载荷透传
- `IPBanAudit` 扩 `Strike int`；`AuditRecord` 扩 `strike`（omitempty，仅 ip_ban 携带；ip_unban 不带）
- ip_ban reason 文案更新：携带档位与实际封禁时长（如「10 分钟内认证失败 10 次，第 2 次触发，自动临时封禁 10 分钟」）；ip_unban 行形态不变

### R5 绑定与装配（main）

- 新增 `App.SetIPBanConfig(threshold, windowMinutes, durationMinutes, maxBanDurationMinutes int) error`：clamp 归一 → 复制基底设置替换 IPBan 段 → Save → `ipBanSvc.ApplyConfig`（「保存+生效」单点；服务未装配仅落盘）
- 新增 `App.GetIPBanConfig`（或经 GetSettings 透传——实现时按 SettingsPanel 既有取数路径定，倾向 GetSettings 透传零新增绑定）
- 装配层（NewAppServices / webServe 启动路径）：`ipBanSvc.ApplyConfig(*settings.IPBan)` 注入启动初值
- wailsjs 绑定三处手动同步（App.js / App.d.ts / models.ts——`IPBanSettings` 新 struct 生成 models.ts；`IPBanStatus` 加 strike 字段）
- wails-mock-defaults.js 补 mock（SetIPBanConfig / ListIPBans 返回值含 strike）

### R6 前端

- **SettingsPanel** 网络访问分区加「自动封禁」小节：四字段 `el-input-number`（min/max 对齐 clamp 边界、`:value-on-clear` 必带防 null 脏值）+ clamp 提示文案 + 「恢复默认」按钮（回填四默认值）；保存走 `SetIPBanConfig` 专用方法（不混 SaveSettings 整体保存）；保存成功提示「已生效」
- **ActivityBar** 自动封禁区块行加递增档展示：「第 N 次」徽标（failCount 旁，warning 色系）
- 前端视觉规范对齐 frontend-visual-conventions（行网格、徽标色系）

### R7 测试

- service：递增档位推进（两次触发 5→10）、封顶（多次触发恒 max）、到期恢复 strikes 保留叠加、RecordSuccess 不清 strikes、UnbanIP/ClearAll 清 strikes、ApplyConfig 热更即时生效（新参数下轮判定）、参数变更不动在途封禁与在途计数、审计行 strike 字段、溢出安全（大 strikes 恒 max）、五期既有用例全绿回归
- model：EnsureIPBanDefaults 段缺失/零值/越界 clamp 四参数×三形态
- main：SetIPBanConfig 保存+热更联动、服务未装配仅落盘
- 前端 vitest：SettingsPanel 表单 clamp/恢复默认/保存链路、ActivityBar strike 徽标、useIPBans 载荷
- E2E 回归：ip-ban.spec.js 既有链路 + 档位展示
- `go test -race` 全绿

### R8 spec 与文档

- `docs/spec/browser-channel.md`：§2.3 五期段扩六期参数化+递增封禁段、§3 契约表新行、§4 错误矩阵（无新错误码则注明）、§6 测试节
- `docs/spec/logging-and-errors.md`：§8 ip_ban 行形态更新（strike 字段+新 reason 文案）
- `CLAUDE.md` 规则行更新（browser-channel.md 行）+ 版本号 v2.33→v2.34

## Acceptance Criteria

- [ ] 设置项改动即时生效（新请求按新参数判定，无需重启）；clamp 拦截非法值；段缺失全默认
- [ ] 连续触发递增档位正确推进（5→10→20→40→60 封顶）、到期恢复后 strikes 保留重计、审计行含 strike 与实际时长、桌面区块展示「第 N 次」
- [ ] 参数变更对生效中封禁 bannedUntil 不变、在途计数不清（Q4 定稿语义，测试守护不产生半态）
- [ ] 认证成功清失败计数不清 strikes；UnbanIP/ClearAll 全清；重启清零
- [ ] `go test` 全绿（含 -race）+ vitest/E2E 回归 + spec 更新 + CLAUDE.md v2.34

## Definition of Done

- R7 全部测试落地且通过
- 五期既有测试零回归
- spec 三处更新
- wailsjs 三处同步（npm run build 过 MISSING_EXPORT 关卡）

## Out of Scope (explicit)

- IP 封禁记录持久化（仍内存态重启清零；strikes 递增记忆同此）
- 封禁黑名单导入导出
- 令牌维度封禁（四期 BanCheck）参数化
- strikes 时间衰减（Q3 C 项否决，复杂度高）
- 参数变更即时重算生效中封禁（Q4 C 项否决）
- 错误码新增（clamp 前端挡 + 后端静默收敛，无新失败分支）

## Technical Notes

- 关键文件：`service/ip_ban_service.go` / `model/settings.go` / `service/settings.go`（Ensure 双链路挂载）/ `model/ip_ban.go`（IPBanStatus 若在此）/ `app_webserve.go`（SetIPBanConfig）或新 app 文件 / `main.go`（装配注入初值）/ `frontend/src/components/SettingsPanel.vue` / `frontend/src/components/ActivityBar.vue` / `frontend/src/composables/useIPBans.js` / `frontend/src/test/wails-mock-defaults.js`
- 热更通道：「保存+生效」显式单点，对齐 SetWebServeConfig；SettingsService 无观察者机制是约束不是缺陷
- 既有导出常量 `IPBanThreshold/IPBanWindow/IPBanDuration` 保留（默认值语义+既有测试引用），新增 `IPBanMaxBanDuration = 60 * time.Minute`
- 递增公式实现：`d := base; for i := 1; i < strikes && d < max; i++ { d *= 2 }; if d > max { d = max }`——禁 math.pow 浮点
- clamp 溢出：durationMinutes 与翻倍均 int，max ≤ 1440min，2^7 内封顶，无溢出面
