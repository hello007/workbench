# 备份去重审核建议修复（P2）

## Goal

落实 f86ae31（损坏留底按内容摘要去重）代码审核的 5 条建议修复项：补强单测判别力、消除 spec 字面冲突、限定 CLAUDE 规则适用范围。不改生产逻辑。

## Requirements

1. `util/backup_test.go` 异内容用例按路径映射断言：`pathA` 内容必须等于 `rawA`，`pathB` 必须等于 `rawB`，防止 A/B 互换仍通过。
2. `util/backup_test.go` 增加摘要格式守护：备份名匹配 `\.bak\.[0-9a-f]{12}$`；空内容用例断言后缀为 `e3b0c44298fc`。
3. `docs/spec/data-atomic-write.md` 明确豁免：损坏留底为原始字节留底，不是主数据 JSON 持久化，直接 `os.WriteFile`，不适用 `util.SaveJSON` 强制口径。
4. 同文档补充 12 位摘要截断的极小碰撞概率与后果：碰撞时旧留底被覆盖，功能不受影响，但证据丢失。
5. `CLAUDE.md` 关键规则行限定范围：「损坏留底禁时间戳命名（写前保险不适用）」，保持 ≤250 字符。

## Acceptance Criteria

* [x] `go test ./util/ -run TestBackupCorruptFile -count=1` 通过，且新增断言能捕获 A/B 互换与摘要格式错误。
* [x] spec 中「禁 os.WriteFile 直写」与损坏留底实现的字面冲突消除。
* [x] CLAUDE.md 规则不再被误读为全仓禁时间戳备份。
* [x] `go test ./...` 与 `scripts/coverage-check.sh` 通过。

## Definition of Done

* 仅测试与文档变更，不触碰 `util/backup.go` 与 service 生产代码。
* 注释与文档中文、自解释。

## Out of Scope

* 可选建议不纳入本轮：留底失败补 `Logger().Warn`、`assertBakFileCount` glob 转义、旧时间戳备份残留说明、gofmt 漂移清理。

## Technical Notes

* 审核结论来源：提交 f86ae31 的静态审核，阻断项 0。
* 关键文件：`util/backup_test.go`、`docs/spec/data-atomic-write.md`、`CLAUDE.md`。
