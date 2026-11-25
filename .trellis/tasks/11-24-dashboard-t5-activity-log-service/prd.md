# T5 新增轻量活动日志 service

- 父任务：`11-24-personal-dashboard`，执行顺序第 6 项（序号 5）
- 阶段 / 优先级：阶段 3 P1 采集层地基 / P1
- 前置：T4 = Go；本项是 T6、T7 的地基
- 状态：planning（仅登记，未 start）

## 目标

补上唯一缺失的数据基础：应用自身使用行为。现状只有前端 `useRecentAccess` 的 localStorage（最多 50 条、无后端聚合、无 panel 维度），面板停留、文件预览、终端会话、命令频次均无采集。

## 要做的事

照抄 `service/audit_log.go` 的 append-only JSONL + `QueryAuditLog` + 轮转 + `readSnapshot` 多档合并范式，采集面板切换、文件预览、终端会话、命令面板、待办完成事件。

## 硬约束

`data/` 写入走 `util.SaveJSON` 原子写 + `util.BackupCorruptFile` 损坏留底；日志用 `log/slog` 禁 `println`；常驻 goroutine / 周期清理须补齐 goleak Cleanup 全链以通过 `soak_leak_test.go` 的 heap 与 goroutine 断言；隐私数据面须加入 `main.go` `RPCHiddenMethods` 并在 `isBrowserChannel()` 隐藏入口；service 覆盖率 ≥76%、前端 ≥70%。

## 验收标准（可测试）

事件可落盘、可查询、重启不丢；覆盖率与 soak 门禁通过；浏览器通道下该数据不可见。

## 不做的事

不做全局埋点 SDK，不上报云端，不采集文件内容或命令正文等敏感载荷。
