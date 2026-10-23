# 桌面端多设备连接指示（浏览器连接数在桌面 UI 展示）

## Goal

2026-10-23 浏览器三期真机验收反馈：三期交付的 `ws-connections:changed` 连接数事件仅经 WSHub 广播给浏览器端（ActivityBar 多设备指示 + 终端多端写入提示行），桌面 Wails 出口不产生该事件。桌面恰是发起共享的一方，却感知不到「当前有几个浏览器设备连着」。本任务把浏览器连接数补到桌面 UI。

## What I already know

* `server/ws.go notifyConnectionCount`：mu 写锁外调用（锁序红线：Emit 内部 seqMu→mu.RLock，禁止 mu 写锁内重入），抑制态 = hub closing 与 count==0；事件经常规 Emit 占 seq 进环形缓冲
* 桌面事件出口 = `service.NewWailsEventSink(m.app.ctx)`（web_serve.go `applySink`）；hub 随 Start/Stop 重建（代际更替），stopLocked 已有 `hub.Close → applySink(nil)` 收口序
* 前端 `useDeviceConnections.js`：单监听 `ws-connections:changed`，载荷校验 `n>=1`（**0 会被忽略**），`multiDevice = deviceCount>1`；注释明确桌面模式恒初始值 1、提示不显示
* 前端无现成 serve/desktop 模式探测旗标
* server 包不知 Wails（分层红线），禁在 server 包直调 Wails runtime

## 关键语义分叉（任务描述未覆盖，brainstorm 定稿）

hub 计数口径 = 浏览器 WS 客户端数：浏览器视角含自身连接（指示条件 count>1），桌面视角 = 纯浏览器数（指示条件 count>=1）。沿用同名事件 + 现有判定会造成桌面 1 浏览器连接时指示不显示（off-by-one）；且桌面需接收 count=0（熄指示），现有前端校验 `n>=1` 恰好吞 0。

## Decision (ADR-lite)

**Context**：桌面事件名沿用同名 vs 专用名需定稿。hub 计数口径 = 浏览器 WS 客户端数，浏览器视角含自身（指示条件 count>1）、桌面视角纯浏览器数（count>=1），且桌面需接收 count=0 熄指示（现有前端校验 n>=1 吞 0）。
**Decision**：**方案 A——桌面专用事件名 `webserve-connections:changed`**。前端 useDeviceConnections 加第二监听：到达即置 `isDesktopChannel=true` + `deviceCount=count`（接受 0）；`multiDevice` 按通道分流（桌面 `count>=1`、浏览器 `count>1`）。浏览器通道逐字节不变（桌面事件浏览器端永不到达，无需模式探测旗标）。
**Consequences**：同名方案被否——分流逻辑两方案改动量相同，同名还背「同名异义」长期坑。代价：composable 多一个监听器 + 通道分流 computed。

## Assumptions (temporary)

* 后端选型：hub 加 `onCountChange` 回调（非轮询），manager 装配时注入闭包经 Wails 出口发桌面事件——Wails 绑定零新增（纯事件面）
* 回调触发条件：`!closing` 时每次 register/unregister 后触发（含 count=0——桌面最后一个浏览器断开须熄指示）；hub 内部 WS 广播抑制态不变
* Stop 收口：hub.Close 后 manager 显式经 Wails 出口发 count=0（Close 收口期回调被抑制，浏览器仍连接时停机桌面须归零）
* 回调闭包引用 manager/app（稳定引用），hub 代际更替天然免疫僵尸监听

## Requirements

* 后端：WSHub 加 `onCountChange func(int)` 回调字段（纯函数类型，server 包不知 Wails）；register/unregister 出 mu 锁后、`!closing` 时触发（**含 count=0**——桌面最后一个浏览器断开须熄指示）；hub 内部 WS 广播路径与抑制态不变
* manager 装配（startLocked）：对新 hub 注入回调闭包，经 `service.NewWailsEventSink(m.app.ctx)` 发桌面事件 `webserve-connections:changed`，data[0]=浏览器连接数
* Stop 收口（stopLocked）：hub.Close 后经 Wails 出口显式发 count=0（Close 收口期回调被抑制，浏览器仍连接时停机桌面须归零）
* 前端：useDeviceConnections 加桌面事件监听（EventsOn 返回闭包精准注销，禁 EventsOff）；`isDesktopChannel` 置位 + deviceCount 接受 0；multiDevice 通道分流（桌面 `count>=1`、浏览器 `count>1`）
* 桌面指示复用 ActivityBar 现有形态与消费面（TerminalPanel 多端提示行经同 composable 自动生效，不加改动）
* Wails 绑定零新增（纯事件面）
* `--serve` 无头模式：Wails 出口静默跳过（既有 sink 行为，无新分支）

## Acceptance Criteria

* 桌面开启浏览器访问后，浏览器设备连接/断开，桌面 ActivityBar 指示即时增减
* webServe 停止后桌面指示消失（count=0）
* 浏览器端行为回归不变（现有 vitest/E2E 全绿）
* Wails 绑定零新增（纯事件面）
* server 包不知 Wails：回调为纯 `func(int)`，闭包在 manager 层组装

## Acceptance Criteria (evolving)

* [ ] 桌面 webServe 运行中：浏览器连上/断开，桌面指示即时增减（含第 1 个浏览器连接即显示）
* [ ] 最后一个浏览器断开（count=0）桌面指示熄灭
* [ ] webServe Stop 后桌面指示消失
* [ ] 浏览器端行为逐字节不变（useDeviceConnections 浏览器通道判定回归）
* [ ] go test 全绿（hub 回调/生命周期/代际用例）
* [ ] wailsjs 绑定如有变更三处同步（本任务预期零变更）

## Definition of Done (team quality bar)

* 后端 hub 回调单测（回调触发条件/抑制态/代际）+ 前端 composable 桌面通道用例
* lint / 现有测试全绿
* docs/spec/browser-channel.md §2.3 增补桌面事件出口契约

## Out of Scope (explicit)

* 轮询方案与新增 App 绑定方法
* 浏览器端 UI/文案改动
* 设备明细列表（只给数量）

## Technical Notes

* 已核对：server/ws.go（notifyConnectionCount/register/unregister/Close）、web_serve.go（startLocked/stopLocked/applySink/multicastSink）、useDeviceConnections.js、docs/spec/browser-channel.md §2.3
* 前端消费单点：useDeviceConnections.js；UI 消费：ActivityBar.vue
