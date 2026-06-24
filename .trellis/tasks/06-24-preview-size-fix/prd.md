# 修复预览大小判定（tooLarge 误伤图片/Office）

## Bug

`PreviewFile`（`service/fileoperation.go`）的 `maxSize=1MB` 对**所有文件类型**判 `tooLarge`。图片/PDF/docx 通常 >1MB → 全被标 `tooLarge`，导致：
- **图片**（>1MB）：前端 `!tooLarge` 才取 base64 → 跳过 → **不显示**
- **Office**（>1MB）：同图片，不显示（<1MB 才正常）
- **PDF**（>1MB）：走 iframe 不依赖 tooLarge → 能显示，但**误弹"文件过大"**
- 文本（>1MB）：tooLarge 合理（读全内容）

不一致 + 误伤。

## 根因

`PreviewFile` 在判 `IsPreviewable`（text 白名单）之前就 `size > maxSize → tooLarge return`，导致 1MB 限制（本意只针对 text 读全内容）误伤 image/pdf/office。

## 修复

**`PreviewFile` 的 tooLarge 只对 `kind=text` 判**（text 才需把全文读成 string）；image/pdf/office 不判 size、不读内容（kind 正常返回）。各类型大小由各自路径处理：

| 类型 | 大小判定 |
|---|---|
| text | PreviewFile 1MB tooLarge（读全内容） |
| image/office | ReadFileBytes 50MB tooLarge（前端 base64 路径） |
| pdf | 无限制（iframe + AssetServer Range 流式） |

## 实施

- `service/fileoperation.go` `PreviewFile`：先 `detectPreviewKind`，仅 `kind==text` 时判 `size > maxSize → tooLarge` 并读全文；image/pdf/office/unsupported 返回 kind（不判 size、不读内容）。
- 前端 `ContentPanel.vue previewFile`：逻辑基本兼容（PreviewFile 不再对 image/pdf/office tooLarge；image/office 走 ReadFileBytes 50MB 判；pdf 走 iframe）。确认无需改或微调。
- 补 `PreviewFile` 单测：text >1MB tooLarge / image 不 tooLarge（kind 正常）。

## 验证

* [ ] 图片 >1MB（<50MB）能显示
* [ ] docx >1MB（<50MB）能显示
* [ ] pdf 大文件能显示（iframe，无误提示）
* [ ] text >1MB 提示过大（合理）
* [ ] `go test ./...`、`npm run build` 通过

## 约束

- 主要改 `service/fileoperation.go`（+ 单测），前端按需微调
- 不破坏现有功能（预览/编辑/降级）
- 不 commit
