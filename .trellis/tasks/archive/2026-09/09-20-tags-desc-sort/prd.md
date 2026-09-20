# 标签页签倒序排列（版本语义）

## Goal

标签列表从 refname 升序（字典序）改为**版本语义倒序**：新版本在上（v1.1 于 v1.0 之上，v1.10 正确大于 v1.2）。

## Requirements

* `service/git.go:954 ListTags` 的 `git for-each-ref` 加 `--sort=-version:refname`。
* 前端/wailsjs 零改动（顺序纯后端决定）。
* 测试：覆盖 v1.10 > v1.2 的版本序锚定（字典序会错排的场景）+ 非版本名标签（如 `release-2024`）退回字典序可正常列出 + 混合场景。

## Acceptance Criteria

* [ ] 标签列表新版本在上；v1.10 排在 v1.2 之上（非字典序错排）。
* [ ] 轻量/注释标签混合仓库排序一致。
* [ ] `go test ./service/` 绿（TestOpenInExternalDiff_* 预存环境失败除外）。

## Out of Scope

* 前端任何改动；创建/删除/推送标签行为。

## Technical Notes

* git `version:refname`（= `version:short` 同族）按版本段数值比较；非版本前缀退回字典序——user 已确认该取舍（备选 creatordate 被否：创建序 ≠ 版本序）。
* git for-each-ref 多 sort 键可用逗号叠加，若需稳定次序可 `--sort=-version:refname --sort=refname`。
