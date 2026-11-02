# 备份去重审核收尾打磨（P2）

## Goal

落实提交 46e26d7 二次审核的 4 条低优先建议：收窄 spec 豁免适用范围、收紧测试断言判别力、精确碰撞表述。不改生产逻辑。

## Requirements

1. `docs/spec/data-atomic-write.md` 的原子写豁免段首句显式限定：豁免仅适用于 `util.BackupCorruptFile` 生成的 `<路径>.bak.<digest>`；`saveChatJSON` 与 `obsidian_vault.backupConfig` 的写前保险不属于本论证范围。
2. `util/backup_test.go` 同内容用例增加期望摘要强绑定断言：按 `raw` 计算 SHA-256 前 12 位 hex，断言 `first` 与 glob 结果均等于 `src + ".bak." + 期望摘要`，不再只依赖 12 位 hex 正则（纯数字时间戳也能通过格式检查）。
3. `util/backup_test.go` 异内容用例的 `wantByPath` map 改为切片 `[]struct{ path string; want []byte }`，消除路径作 map 键的隐式去重语义。
4. `docs/spec/data-atomic-write.md` 碰撞说明收窄：碰撞面为「同一源文件的不同损坏内容」，而非「同目录少量留底文件」；跨源文件因文件名含完整源路径不会碰撞。

## Acceptance Criteria

* [x] 同内容用例能捕获「格式合法但摘要错误」的实现退化。
* [x] 异内容用例不依赖 map 键去重兜底。
* [x] spec 豁免范围与碰撞面表述准确，无泛化误读。
* [x] `go test ./util/ -run TestBackupCorruptFile -count=5`、`go test ./...`、`scripts/coverage-check.sh` 通过。

## Definition of Done

* 仅测试与文档变更，禁止改 `util/backup.go` 与 service 生产代码。
* 中文注释、自解释。

## Out of Scope

* 生产代码调整、覆盖率阈值调整、gofmt 漂移清理。

## Technical Notes

* 审核来源：提交 46e26d7 只读复核。
* 关键文件：`util/backup_test.go`、`docs/spec/data-atomic-write.md`。
