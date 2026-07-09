# Cross-Layer Contracts

> Executable contracts spanning Go backend (`app.go` bridge) ↔ Wails JS bindings (`frontend/wailsjs/`) ↔ Vue frontend. 本文件记录跨层签名与契约，供未来修改 App 方法签名或文件预览/保存逻辑时参照。

---

## Scenario: App 方法签名变更须手动同步 Wails 绑定

### 1. Scope / Trigger
- Trigger: 修改 `app.go` 中 `App` 结构体的导出方法签名（增删参数、改类型）。
- 原因: Wails 在 `wails dev` / `wails build` 时自动生成 `frontend/wailsjs/go/main/App.js` 与 `App.d.ts`，但 sub-agent / CI 环境无法运行 wails，绑定不会自动重新生成，必须手动同步。

### 2. Signatures（以 SaveFile 为例）
- Go: `func (a *App) SaveFile(filePath, content, encoding string) error`
- JS binding (`App.js`): `export function SaveFile(arg1, arg2, arg3) { return window['go']['main']['App']['SaveFile'](arg1, arg2, arg3) }`
- TS (`App.d.ts`): `SaveFile(arg1: string, arg2: string, arg3: string): Promise<void>`

### 3. Contracts
- 参数顺序与数量必须三处一致: Go 方法 / `App.js` / `App.d.ts`
- 前端调用点（如 `ContentPanel.vue` handleSave）须同步更新为相同参数数量
- `App.js` 与 `App.d.ts` 解耦: 只改一处会导致类型提示与运行时行为不一致

### 4. Validation & Error Matrix
- Go 改签名未同步 `App.js` -> 前端传参错位，`window['go']['main']['App']['SaveFile']` 收到错位参数，**静默错误不报错**（难排查）
- `App.js` 改但 `App.d.ts` 未改 -> TS 类型检查可能通过（两者解耦），但 IDE 类型提示错误
- 同步后未跑 `cd frontend && npm test` -> 调用点测试可能漏改

### 5. Good/Base/Bad Cases
- Good: 改 Go 签名 -> 同步 `App.js` + `App.d.ts` -> 全局 grep 调用点更新 -> 跑 `npm test`
- Base: 改 Go 签名 -> 同步 `App.js` + `App.d.ts` -> 跑 `npm test`（依赖测试覆盖调用点）
- Bad: 只改 Go 签名，未同步绑定 -> 前端调用错位

### 6. Tests Required
- 前端单测断言调用参数（如 `ContentPanel.spec.js` 断言 SaveFile 收到 `(path, content, encoding)` 三参）
- 后端单测覆盖新签名分支

### 7. Wrong vs Correct
#### Wrong
改 `SaveFile(filePath, content string)` 为 `SaveFile(filePath, content, encoding string)`，仅改 `app.go` 与 service，未同步 wailsjs 绑定。前端 `SaveFile(path, content)` 调用时 encoding 收到 undefined，保存按 UTF-8 写入，GBK 文件编码被破坏。
#### Correct
同步修改 `frontend/wailsjs/go/main/App.js`（加 `arg3`）与 `App.d.ts`（加 `arg3: string`），前端调用点改为 `SaveFile(path, content, encoding)`，回传编码。

---

## Scenario: 文件预览/保存的编码契约（UTF-8 / GBK）

### 1. Scope / Trigger
- Trigger: `PreviewFile` / `SaveFile` 涉及文件编码处理，`model.FilePreview.Encoding` 字段跨层流转（后端检测 -> 前端回传 -> 后端按编码保存）。

### 2. Signatures
- `util.DetectTextEncoding(data []byte) (encoding, content string, ok bool)`
- `service.PreviewFile(filePath string, maxSize int64) (*model.FilePreview, error)`
- `service.SaveFile(filePath, content, encoding string) error`
- `model.FilePreview.Encoding string` (`json:"encoding,omitempty"`，取值 `"utf-8"` / `"gbk"` / `""`)

### 3. Contracts
- **PreviewFile**: `text` 与 `unsupported` 类型统一走 `DetectTextEncoding`；`ok=true` -> `Kind=KindText`（unsupported 降级为 text）、`Content`=转码后 UTF-8、`Encoding`=来源编码；`ok=false` -> `IsBinary=true`、`Kind` 保留原值、`Content` 空。`image`/`pdf`/`office` 不读内容直接返回（early return 保护，避免误判 tooLarge）。
- **SaveFile**: `encoding="gbk"` -> `simplifiedchinese.GBK.NewEncoder().Bytes([]byte(content))` 转码后写入；其余（`utf-8`/空）直接写 `[]byte(content)`。保留 1MB 限制与原子写。
- **前端 handleSave**: 须回传 `filePreview.encoding`，保证按原编码保存（不改变原文件编码）。

### 4. Validation & Error Matrix
- 含 NUL(`0x00`) 字节（扫前 8KB）-> `ok=false`（二进制）
- `utf8.Valid(data)` -> `encoding="utf-8"`
- 非 UTF-8 且 GBK 解码后 U+FFFD 占比 > 5% -> `ok=false`（防 GBK 误判二进制）
- 空文件 -> `ok=true`, `encoding="utf-8"`, `content=""`
- 非文本类型(image/pdf/office) 误走编码检测 -> 已用 `Kind != text && Kind != unsupported` early return 保护

### 5. Good/Base/Bad Cases
- Good: GBK 中文文件预览显示正确中文，`Encoding=gbk`，保存后仍为 GBK
- Base: UTF-8 文件正常显示编辑保存
- Bad: 二进制文件显示乱码（应 `IsBinary=true` 降级，不进文本渲染）

### 6. Tests Required
- `util.DetectTextEncoding`: UTF-8 文本 / GBK 中文 / NUL 二进制 / 非 UTF-8 非 GBK / 空文件 / NUL 超 8KB 不误判 / 纯 ASCII
- `service.PreviewFile`: unsupported 降级文本显示 / 二进制 IsBinary / GBK 显示中文 / 过大 TooLarge / image-pdf-office 不回归
- `service.SaveFile`: UTF-8 保存 / GBK 保存（读回验证字节为 GBK 而非 UTF-8）

### 7. Wrong vs Correct
#### Wrong
GBK 文件用 `string(data)` 直接转（utf8 无效，前端显示乱码）；或保存时统一按 UTF-8 写入（改变原文件编码，破坏 GBK 文件）。
#### Correct
用 `DetectTextEncoding` 检测编码并转码为 UTF-8 给前端显示；保存时按 `Encoding` 字段转回原编码写入，原文件编码不变。
