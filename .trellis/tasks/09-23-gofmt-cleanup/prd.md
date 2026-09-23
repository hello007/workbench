# gofmt 存量未格式化文件清理

## Goal

`gofmt -l` 存量约 17 个未格式化文件清零（model/models.go、service/directory.go、service/repo_stats_test.go 等），消除后续每次 check 的噪音输出。

## Requirements

* `gofmt -w` 全仓（排除 vendor/generated）格式化
* 逐文件核对 git diff 为纯格式变更（对齐/缩进/import 排序），无语义改动

## Acceptance Criteria

* [ ] `gofmt -l .` 无输出
* [ ] `go build ./...` + `go test ./...` 全绿
* [ ] diff 复核确认纯格式变更

## Out of Scope

* 前端 prettier/eslint 格式；代码重构；CI 加 gofmt 门禁（可另议）
