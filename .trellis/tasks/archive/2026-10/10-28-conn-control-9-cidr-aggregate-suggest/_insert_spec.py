# -*- coding: utf-8 -*-
"""一次性脚本：browser-channel.md 插入九期段（八期段后、§2.4 前）。用完即删。"""
import io

path = 'docs/spec/browser-channel.md'
with io.open(path, 'r', encoding='utf-8') as f:
    content = f.read()

new_para = u"**网段聚合建议（2026-10-28 连接管控九期）**：自动观测到手动手置的建议闭环——同聚合网段内自动封禁条目达阈值即在处置面提示「建议网段封禁」，处置仍须管理员确认提交（**闭环止于预填，不自动封禁**）。**聚合口径（写死 doc comment，禁漂移）**：固定粒度 IPv4 /24 + IPv6 /64（禁动态推导——粒度可预期、管理员可理解、攻击者不可探测边界）；数据源 `s.entries` 内存态 mu 锁内遍历，仅 `source=auto` 且 `bannedAt` 非零条目参与（**含已过期**，与 List 可见条目同口径——列表所见即建议所计，零口径漂移）；CoveredBy 命中**生效中**网段封禁的条目剔除（`cidrCoveredBy` 复用防重复建议；已过期网段封禁不剔除，语义与 List 覆盖标注同源）；ParseIP 失败剔除（防御，normalizeIP 后理论不可达）；manual 条目不参与（管理员已知悉处置非自动观测证据）；**禁用审计行作数据源**（含已解封历史 + manual 混杂 + 5MB 轮转边界漂移，趋势面留十期候选）；IPv4-mapped IPv6（`::ffff:a.b.c.d`）经 `To4()` 判定落 /24 桶不误入 /64（专项用例钉住）。**查询面**：纯函数 `aggregateIPsToSuggestions(ips, threshold)`（分桶计数、阈值严格 ≥ 即 N-1 不产出 N 产出、规范化 CIDR 串 `ipnet.String()` 复用八期 BanCIDR 解析口径、排序计数降序 + 网段串字典序次级且 `sort.SliceStable` 对齐 List 先例）+ `IPBanService.AggregateSuggestions()`（锁内过滤+纯函数一次完成，白名单命中理论不可达不特判——RecordFailure 早退不建条目）；`model.IPBanAggregateSuggestion{CIDR, Count}` 全基本类型仅桌面出口（struct 走 wails generate 自动生成 class，无需手补 models.ts 别名）。**阈值**：`IPBanSettings.AggregateSuggestThreshold`（默认 3，clamp 2-50，normalize 复用 clampIPBanInt）经 `ApplyConfig`（整段入参现状）锁内注入 `suggestThreshold` 字段；`SetIPBanConfig` 五参扩六参（**聚合阈值第五位、whitelist 移第六位**，全仓 12 调用点同步：生产 1 + 前端 1 + 测试 10）。**隐私红线**：App 绑定 `ListIPBanSuggestions()` 进 `RPCHiddenMethods`（生产值扩为 `["QueryAuditLog","ListIPBans","ListIPBanSuggestions"]`，命中与「方法不存在」同响应）；载荷仅网段串 + 计数无 IP 明细，仍整体隐藏无降级 UI（对齐「他人 IP 明细仅走桌面出口」红线）；前端 `useIPBanSuggestions` 经 `isBrowserChannel()` 跳过加载（与 useIPBanList 同口径）。**前端闭环**：ActivityBar IP 封禁区块封禁列表上方独立「聚合建议」小节（空建议整体不渲染；建议行文案「该网段 N 个来源已被封禁，建议网段封禁」，前端零排序——后端已排）；`openBanIPDialog` 改可选参 `openBanIPDialog(cidr = '')`，**头部按钮必须显式 `@click.stop=\"openBanIPDialog()\"` 空参调用**（方法引用形态把 MouseEvent 传入 cidr 形参，测试已捕获该缺陷）；BanIPDialog 加 `initialCidr` prop（与 `initialIP` 互斥使用每次打开只传其一；watch 分流三分支：**双非空 initialIP 优先**、initialCidr 非空切网段档 + 预填、双空默认 ip 档；时长档不预填维持默认永久；白名单预检三态复用网段档现成逻辑）；`useIPBanList` 与 `useIPBanSuggestions` 共听 `webserve-ip-banned`（各自 EventsOn 返回闭包精准注销；**vitest 的 EventsOn mock 必须支持同名多监听器分发**——单回调形态后注册覆盖前者会静默丢监听器，ActivityBar.spec.js 已升级按名收集 + 分发函数形态并加文件级注册表清理）。无新增错误码（纯只读查询面，恒非 nil 空切片）。"

anchor = u"### 2.4 断线重连续传（server/ws.go，2026-10 新增）"
assert anchor in content, u"锚点未找到"
assert new_para[:60] not in content, u"九期段已存在，勿重复插入"

content = content.replace(anchor, new_para + u"\n\n" + anchor, 1)
with io.open(path, 'w', encoding='utf-8') as f:
    f.write(content)
print(u"插入完成")
