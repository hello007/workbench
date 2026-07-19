# Cross-Layer Contracts

> Executable contracts spanning Go backend (`app.go` bridge) ↔ Wails JS bindings (`frontend/wailsjs/`) ↔ Vue frontend. 本文件记录跨层签名与契约，供未来修改 App 方法签名或文件预览/保存逻辑时参照。

---

## Scenario: App 方法签名变更须手动同步 Wails 绑定

### 1. Scope / Trigger
- Trigger: 修改 `app.go` 中 `App` 结构体的导出方法签名（增删参数、改类型）。
- 原因: Wails 在 `wails dev` / `wails build` 时自动生成 `frontend/wailsjs/go/main/App.js` 与 `App.d.ts`。`frontend/wailsjs/` 整目录已在 `.gitignore`（自 commit 28ca710）且不被 git 跟踪；sub-agent 环境可运行 `wails generate module` 重新生成绑定，若环境不可用（如 CI 无 wails）则需手动同步 `App.js` / `App.d.ts`。

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

## Scenario: model struct 字段变更须同步 wailsjs/go/models.ts

### 1. Scope / Trigger
- Trigger: 修改 `model/` 下任何被 Wails 暴露给前端的导出 struct 字段（增删字段、改类型、改 json tag），如 `FilePreview`、`FileBytes`、`Directory`、`FileTreeNode` 等。
- 原因: Wails 在 `wails dev`/`wails build` 时根据 Go struct 生成 `frontend/wailsjs/go/models.ts`（TS class 定义 + 构造函数赋值）。sub-agent / CI 无法运行 wails，必须手动同步，否则前端 TS 类型缺字段（JS 运行时仍能取到 JSON 字段，但类型不完整、IDE 无提示，且 vitest 用 esbuild 转译不报类型错误，极易漏）。

### 2. Signatures（以 FilePreview 加 Encoding 字段为例）
- Go: `model.FilePreview` 新增 `Encoding string` (`json:"encoding,omitempty"`)
- TS (`frontend/wailsjs/go/models.ts`): `FilePreview` class 新增 `encoding?: string;` 字段声明 + 构造函数 `this.encoding = source["encoding"];`

### 3. Contracts
- Go struct 字段（含 json tag） <-> `models.ts` 对应 class 的字段声明 + 构造函数赋值，两处须一致
- `omitempty` json tag -> TS 字段用 `?:` 可选
- 字段名按 json tag（而非 Go 字段名）映射到 TS
- `frontend/wailsjs/` 整目录在 `.gitignore` 中（自 commit 28ca710）且不被 git 跟踪，`models.ts` 由 `wails generate module` 自动生成，不提交、无需 `git add -f`

### 4. Validation & Error Matrix
- Go 加字段未同步 `models.ts` -> 前端 TS 类型缺字段，`preview.encoding` 类型检查警告（vitest esbuild 不报类型错误，测试仍过，易漏）
- 只改 `models.ts` 字段声明未改构造函数 -> 运行时该字段为 undefined
- sub-agent 须运行 `wails generate module` 重新生成绑定 -> trellis-check 须确认绑定已生成且 Go 方法签名 / `App.js` / `App.d.ts` 三处一致，并核对 `RepoFilterItem` 等 struct 的 `models.ts` 字段与 Go json tag 对齐（`frontend/wailsjs/` 已 gitignore，`git status` 不会有残留）

### 5. Good/Base/Bad Cases
- Good: 改 Go struct -> 同步 `models.ts`（字段声明+构造函数）-> `git status` 确认无 wailsjs 残留 -> 跑 `npm test`
- Base: 改 Go struct -> 同步 `models.ts` -> 跑 `npm test`
- Bad: 改 Go struct 未同步 `models.ts` -> 前端类型缺字段（测试可能仍过，运行时靠 JS 动态取值，类型完整性丢失）

### 6. Tests Required
- 后端单测覆盖新字段读写
- 前端若 TS 严格类型，补字段类型断言；vitest 下运行时断言字段值存在
- commit 前 `git status` 确认 `frontend/wailsjs/` 无未提交残留

### 7. Wrong vs Correct
#### Wrong
`model.FilePreview` 新增 `Encoding` 字段，仅改 Go 与 service，未同步 `frontend/wailsjs/go/models.ts`。前端 `preview.encoding` 在 JS 运行时能取到值（JSON 有），但 TS 类型缺失，IDE 无提示，且 `git status` 残留 models.ts 未提交。
#### Correct
同步修改 `frontend/wailsjs/go/models.ts` 的 `FilePreview` class：加 `encoding?: string;` 声明 + 构造函数 `this.encoding = source["encoding"];`，与 Go struct 的 json tag 一致。

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
