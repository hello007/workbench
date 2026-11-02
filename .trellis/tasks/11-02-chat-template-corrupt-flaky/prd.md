# 修复聊天模板损坏回退用例偶发失败（P1）

## Goal

`service/chat_config_test.go:178 TestChatService_TemplatesCorruptFallback` 跨秒偶发失败：

* 损坏模板每次读取都会写 `.bak.<秒>` 备份；
* 用例先 `NewChatService` 再 `ListChatTemplates`，两次读跨越秒边界时产生 2 个备份文件；
* 断言 `len(matches) != 1` 变红（实测日志时间 `15:35:59` 与 `15:36:00`）。

与拆分/备份系列改动无关，但会污染 CI 与全量测试结果。

## Requirements

1. 消除时间依赖：优先注入可控时钟（若实现支持）或固定备份文件名生成（同秒递增策略需可测）；若改造成本过高，可放宽断言为 `>= 1`，但须在注释说明「多次读取可能产生多份备份」的真实语义，且不得掩盖「重复备份无上限」的缺陷。
2. 若发现实现层缺陷（同一损坏内容每次读取都新建备份，备份无限增长），单独评估：是否应按内容哈希去重或同秒覆盖。
3. 连跑验证：`go test ./service/ -run TestChatService -count=20` 无偶发。
4. 文档：若改变测试语义，在 `docs/spec/logging-and-errors.md` 或对应 spec 记录备份命名/去重口径。

## Acceptance Criteria

* [ ] `-count=20` 连续通过。
* [ ] 断言仍能捕获「损坏模板未正确回退」的真实回归（不得改成永真断言）。
* [ ] `go test ./...` 与覆盖率门禁通过。

## Out of Scope

* 不重构聊天配置整体存储，只处理备份命名/测试时序。
