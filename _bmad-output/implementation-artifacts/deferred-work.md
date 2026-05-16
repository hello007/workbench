# Deferred Work

## 文件预览安全性

- `.env` 等含敏感信息的隐藏文件可被预览，密钥明文暴露在 UI 中。建议在 `IsPreviewable` 中排除 `.env`，或对含 `password`/`secret`/`key`/`token` 的文件内容做脱敏。
- `.ssh`、`.gnupg`、`.aws` 等敏感系统目录在文件树中可见且可执行删除/复制等操作。建议对系统关键路径做保护。

## GetTree 递归性能

- `node_modules`、`.cache`、`vendor` 等大型目录在递归展开时可能导致性能问题。建议增加排除规则或子节点数量上限。
