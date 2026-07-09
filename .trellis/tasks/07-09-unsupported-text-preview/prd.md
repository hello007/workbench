# unsupported 文件类型按文本预览（含乱码检测）

## Goal

当前文件预览对 `unsupported` 类型（无扩展名、未知扩展名、二进制等）直接放弃，仅弹"暂不支持内嵌预览"提示。本任务将其改为：**优先尝试按文本读取并显示在编辑器中**；若内容判定为乱码/二进制则按需降级（不显示）；文件过大则不显示。让用户在应用内即可查看更多类型的文件内容，而非被迫跳转外部程序。同时统一解决现有 `text` 类型 GBK 编码文件显示乱码的问题。

## What I already know

### 现有预览架构

* **后端 `service/fileoperation.go::PreviewFile(filePath, maxSize)`**：
  - 调 `detectPreviewKind(filePath)` 按扩展名分类：`image / pdf / office / text / unsupported`
  - `text`：读全文（受 `maxSize` 1MB 限制），超限标 `TooLarge`
  - `image / pdf / office / unsupported`：**不读内容**，直接返回 kind（前端各自处理）
  - `unsupported` 由前端降级提示
* **`detectPreviewKind`**：
  - image：jpg/png/bmp/gif/webp/svg/ico/tif/tiff/heic/heif/avif
  - pdf：.pdf
  - office：doc/docx/ppt/pptx/xls/xlsx/csv/odt/odp/ods/rtf 等
  - text：`util.IsPreviewable` 白名单（txt/md/json/xml/yaml/js/ts/vue/go/java/py/c/cpp/html/css/sh/bat/gitignore/env）
  - 其余全部 `unsupported`
* **`util/file.go`**：`IsPreviewable`（扩展名白名单）、`ReadFileSafe(filePath, maxSize)`（超限报错）、`FormatFileSize`
* **数据模型 `model.FilePreview`**：`Path/Name/Size/Content/IsBinary/TooLarge/Error/Kind`
  - `IsBinary` 字段已存在，但**当前 `PreviewFile` 从未设置它**
  - Kind 常量：`KindText/KindImage/KindPDF/KindOffice/KindUnsupported`
* **前端 `ContentPanel.vue::previewFile`**：
  - 调 `PreviewFile`，按 kind 分流
  - `unsupported` -> `ElMessage.warning('该文件类型暂不支持内嵌预览')`
  - `image/office` 再调 `ReadFileBytes` 取 base64；`pdf` 走 iframe；`text` 用 content
  - 文本类可切编辑态（`isEditing`），`handleSave` 走 `SaveFile`（1MB 上限，原子写）
* **前端 `FilePreviewRenderer.vue`**：
  - `kind=text` -> CodeMirror 6 只读（md 走 markdown-it 渲染）
  - `unsupported/error/tooLarge/isBinary` -> `preview-fallback` 分支，文案由 `fallbackMessage` computed 决定
  - `fallbackMessage` 已区分：error / tooLarge / isBinary / unsupported

### 关键约束

* `SaveFile` 限制 1MB，原子写（临时文件 + rename）
* `ReadFileBytes` 上限 50MB（image/office 用）
* PDF 走后端 `/preview-pdf` handler（仅放行 .pdf 扩展名，`server/preview.go`）
* 项目当前无 `golang.org/x/text` 依赖，本任务将引入

## Assumptions (temporary)

* 乱码/二进制检测放在**后端**（Go 标准库 `unicode/utf8.Valid` + NUL 字节检测 + `x/text` GBK 转码）
* 检测为可显示文本时，复用现有 `text` 渲染器（CodeMirror），不新建渲染器

## Open Questions

* 过大阈值：倾向沿用现有 text 的 1MB（与 SaveFile 限制一致），待最终确认

## Decisions (resolved)

* **乱码检测策略 = 尝试 GBK 转码**（2026-07-09 确认）：
  - 含 NUL 字节(0x00) -> 二进制，降级不显示
  - 合法 UTF-8 -> 直接显示，encoding=utf-8
  - 非合法 UTF-8 -> 尝试 GBK->UTF-8 解码（`golang.org/x/text/encoding/simplifiedchinese`），成功则显示转码后内容(encoding=gbk)，失败则降级不显示
  - 需引入 `golang.org/x/text` 依赖

* **编辑能力 = 全部可编辑，按实际编码保存**（2026-07-09 确认）：
  - 降级文本可编辑保存
  - 保存时按原文件编码写入（UTF-8 文件按 UTF-8 保存，GBK 文件按 GBK 保存），不改变原文件编码
  - 需扩展 FilePreview 回传编码来源；SaveFile 支持按指定编码转码写入

* **应用范围 = 统一 text + unsupported**（2026-07-09 确认）：
  - 编码检测与按编码保存同时应用到 `text` 与 `unsupported` 两类
  - 顺带解决现有 `text` 类型 GBK 文件显示乱码问题，两类行为一致

## Requirements (evolving)

* `unsupported` 类型文件被选中预览时，尝试按文本读取并在编辑器中显示（检测为可显示文本时 kind 降级为 text，复用 CodeMirror）
* `text` 类型同样走编码检测，支持 GBK 文件正确显示（解决现有乱码）
* 内容判定为乱码/二进制（含 NUL 字节，或非 UTF-8 且 GBK 解码失败）时，不显示文本，降级提示"二进制文件，无法内嵌预览"+"用默认程序打开"
* 文件过大（超 1MB）时不显示，沿用 tooLarge 机制
* 文本可编辑保存，保存时按原文件编码写入（UTF-8 / GBK），不改变原文件编码
* `FilePreview` 回传 `encoding` 字段（utf-8 / gbk），供前端保存时回传

## Acceptance Criteria (evolving)

* [ ] 选中 `unsupported` 类型的小文本文件（如无扩展名 readme、.log、.conf）能以文本形式正常显示
* [ ] 选中二进制文件（如 .exe/.dll/.zip）时不显示乱码，给出降级提示与"用默认程序打开"
* [ ] 选中超大 `unsupported` 文件时不卡顿，提示文件过大
* [ ] GBK 编码的文本文件（text 或 unsupported）能正确显示中文，不出现乱码
* [ ] 编辑 GBK 文件后保存，原文件仍为 GBK 编码（用十六进制查看器验证编码未被改成 UTF-8）
* [ ] 编辑 UTF-8 文件后保存，原文件仍为 UTF-8 编码
* [ ] 现有 image/pdf/office 预览行为不受影响（回归）

## Definition of Done (team quality bar)

* 后端单测覆盖：可显示 UTF-8 文本 / GBK 文本 / 二进制（NUL）/ 非 UTF-8 且非 GBK / 过大 / 空文件 等分支
* 后端单测覆盖：SaveFile 按 UTF-8 与 GBK 编码保存的正确性
* 前端测试覆盖：unsupported 降级为文本显示的渲染分支、handleSave 回传 encoding
* `go test ./...` 与 `cd frontend && npm test` 通过
* 行为变化在 README.md / 功能说明.md 中补充

## Out of Scope (explicit)

* 除 UTF-8 与 GBK 外的其他编码（Shift-JIS / Big5 / Latin1 等）暂不支持
* 不引入通用编码自动识别库（chardet），仅 UTF-8 优先 + GBK 兜底
* unsupported 大文件不做"前 N 行分页预览"，超 1MB 直接 tooLarge
* 不改动 image / pdf / office 的预览路径

## Technical Approach

### 1. 引入依赖
`golang.org/x/text/encoding/simplifiedchinese`（GBK 编解码）。

### 2. 新增工具函数 `util/file.go`
`DetectTextEncoding(data []byte) (encoding, content string, ok bool)`：
- `bytes.ContainsRune(data, 0)` 或扫到 NUL 字节 -> 二进制，ok=false
- `utf8.Valid(data)` -> encoding="utf-8", content=string(data), ok=true
- 否则尝试 `simplifiedchinese.GBK.NewDecoder().Bytes(data)`：
  - err==nil 且结果中 U+FFFD（replacement char）比例低于阈值 -> encoding="gbk", content=解码字符串, ok=true
  - 否则 ok=false
- 空文件 -> ok=true, encoding="utf-8", content=""（空文本可显示）

### 3. `PreviewFile` 改造（统一 text + unsupported）
- 大小 > maxSize -> TooLarge（不读）
- 读字节 -> `DetectTextEncoding`
  - ok=true -> Kind=KindText（unsupported 降级为 text），Content=转码后内容，Encoding=来源编码
  - ok=false -> IsBinary=true，Kind 保留原值，Content 空，前端走降级提示
- image/pdf/office 分支不变

### 4. 数据模型 `model/models.go`
`FilePreview` 新增 `Encoding string` 字段（json:"encoding,omitempty"），取值 "utf-8"/"gbk"/""。

### 5. `SaveFile` 改造
签名变为 `SaveFile(filePath, content, encoding string) error`：
- encoding="gbk" -> `simplifiedchinese.GBK.NewEncoder().Bytes([]byte(content))` 转码后写入
- 其余（utf-8/空）-> 直接写 `[]byte(content)`
- 保留 1MB 限制（针对 content）与原子写（临时文件 + rename）

### 6. `app.go` 桥接
`App.SaveFile` 签名同步增加 encoding 参数；Wails 重新生成 wailsjs 绑定（`frontend/wailsjs/go/main/App.js`）。

### 7. 前端 `ContentPanel.vue`
- `previewFile`：记录 `filePreview.encoding = preview.encoding`
- `handleSave`：调用 `SaveFile(path, content, encoding)` 回传编码
- unsupported 降级为 text 后，kind=text，正常走编辑/保存流程

### 8. 前端 `FilePreviewRenderer.vue`
- kind=text 复用现有 CodeMirror（无需新渲染器）
- 可选：在文本区顶部显示编码标识（如"编码: GBK"），便于用户感知

### 9. 测试与文档
- 后端 `service/fileoperation_test.go` / `util` 补充编码与二进制分支用例
- 前端 `ContentPanel.spec.js` 补充降级文本与保存传 encoding 用例
- README.md / 功能说明.md 补充 unsupported 按文本预览与编码保存说明

## Implementation Plan (small PRs)

* **PR1**：引入 `x/text` 依赖 + `util.DetectTextEncoding` 工具函数 + 单测（纯后端，无行为变化）
* **PR2**：`PreviewFile` 统一 text/unsupported 走编码检测 + `FilePreview.Encoding` 字段 + 后端单测
* **PR3**：`SaveFile` 支持按编码保存 + `app.go`/wailsjs 绑定 + 前端 `handleSave` 回传 encoding + 单测
* **PR4**：前端降级渲染适配 + 编码标识 + 前端测试 + 文档更新

## Technical Notes

* **GBK 误判风险**：GBK 是双字节编码，某些二进制数据恰好构成合法 GBK 序列会被误判为文本。用解码后 U+FFFD 比例阈值兜底（如 >5% 视为失败）。可接受少量误判，用户仍可"用默认程序打开"。
* **NUL 字节检测范围**：扫前 8KB 即可（git heuristic），无需全文扫描，性能好。
* **影响文件**：`service/fileoperation.go`、`util/file.go`、`model/models.go`、`app.go`、`frontend/src/components/ContentPanel.vue`、`frontend/src/components/FilePreviewRenderer.vue`、`frontend/wailsjs/go/main/App.js`（自动生成）、对应 _test 文件、go.mod / go.sum
