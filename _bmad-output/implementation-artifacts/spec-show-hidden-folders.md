---
title: '文件树显示隐藏文件夹'
type: 'feature'
created: '2026-05-15'
status: 'done'
route: 'one-shot'
---

## Intent

**Problem:** 文件树过滤掉所有 `.` 开头的文件和文件夹，导致 `.claude`、`.vscode` 等有用的隐藏目录不可见。

**Approach:** 将 `service/filetree.go` 的过滤条件从 `name == ".git" || strings.HasPrefix(name, ".")` 简化为 `name == ".git"`，仅排除 `.git` 目录。

## Suggested Review Order

1. [service/filetree.go](../../service/filetree.go) — 核心过滤逻辑变更（第 50 行）
2. [service/filetree_test.go](../../service/filetree_test.go) — 测试更新：断言隐藏项可见、.git 被跳过
