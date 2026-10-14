# Research: Windows Go 环境 go-ole 调用 PowerPoint COM 导出缩略图 + 打开跳转指定页

- **Query**: go-ole v1.3.0 调用 PowerPoint COM 的完整链路、打开跳页、稳定性工程（超时/强杀/重启）、无 PowerPoint 检测、文件占用行为、性能量级、参考实现
- **Scope**: mixed（go-ole v1.3.0 本地源码实证 + 项目现有代码 + PowerPoint COM 领域知识）
- **Date**: 2026-10-14
- **任务**: `.trellis/tasks/10-14-ppt-com`（素材管理第二批——PPT COM 缩略图链路）

> 置信度标注约定：go-ole API 行为均经本地 module cache 源码（`$(go env GOMODCACHE)/github.com/go-ole/go-ole@v1.3.0/`）逐行核实，为实证结论；PowerPoint COM 侧行为来自 VBA 官方文档语义与社区长期实践（C#/PowerShell 自动化），其中少量点标「**待实测**」，已在文末集中列出 implement 阶段验证清单。

---

## 1. go-ole 调用 PowerPoint COM 完整链路

### 结论

**链路四步：`CoInitializeEx(STA)` → `CreateObject("PowerPoint.Application")` → `QueryInterface(IID_IDispatch)` → `oleutil` 属性/方法调用；goroutine 必须 `runtime.LockOSThread()`；PowerPoint Application 对象**没有 Visible 属性**（与 Excel/Word 的著名差异，写它直接抛错），「无窗口」靠 `Presentations.Open` 的 `WithWindow:=msoFalse` 控制；`Slide.Export(path, "PNG", w, h)` 的 w/h 是像素值，主流实测行为是输出保持幻灯片原始纵横比（**待实测**确认本项目 Office 版本的精确语义）。**

### 1.1 链路代码骨架（go-ole 源码实证）

```go
runtime.LockOSThread()                              // STA 线程亲和，见 1.3
defer runtime.UnlockOSThread()

hr := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED) // STA = 0x2
// 坑：线程已是 STA 时返回 S_FALSE(0x00000001)，go-ole 视 hr!=0 为错误——
// 工程上须放行 hr==1（语义为「已初始化」，但配对仍须调一次 CoUninitialize）
defer ole.CoUninitialize()

unknown, err := oleutil.CreateObject("PowerPoint.Application") // ProgID → CLSIDFromProgID → CoCreateInstance
if err != nil { /* 0x800401F3 / 0x80040154 → 无 PowerPoint，见第 4 节 */ }
defer unknown.Release()
app, err := unknown.QueryInterface(ole.IID_IDispatch)
defer app.Release()

oleutil.PutProperty(app, "DisplayAlerts", 1)          // ppAlertsNone，压弹窗（数值**待实测**）
pres, _ := oleutil.MustCallMethod(app, "Presentations").ToIDispatch()
p, _ := oleutil.MustCallMethod(pres, "Open", path, -1, 0, 0).ToIDispatch() // ReadOnly/Untitled/WithWindow 全传，见第 2 节
slides, _ := oleutil.MustGetProperty(p, "Slides").ToIDispatch()
slide, _ := oleutil.MustCallMethod(slides, "Item", 1).ToIDispatch()       // 1-based
_ = oleutil.MustCallMethod(slide, "Export", pngPath, "PNG", 480, 270)
```

### 1.2 go-ole 侧行为细节（源码实证，`go-ole@v1.3.0`）

| 事实 | 依据（源码位置） |
|---|---|
| `oleutil.CreateObject(progID)` 内部走 `CLSIDFromProgID` + `CoCreateInstance(CLSCTX_SERVER)`，**无需硬编码 PowerPoint CLSID** | `oleutil/oleutil.go:15-27`、`com.go:222-236`（`CLSCTX_SERVER` 硬编码，含 LOCAL_SERVER，PowerPoint 属 out-of-proc LocalServer） |
| `CoInitializeEx(p, coinit)` 走 ole32 syscall，`COINIT_APARTMENTTHREADED = 0x2`、`COINIT_MULTITHREADED = 0x0` 常量齐备 | `com.go:65-73,121-125`、`constants.go:15-16` |
| 方法/属性调用统一走 `IDispatch::Invoke`（late binding）：`oleutil.CallMethod` → `DISPATCH_METHOD`，`GetProperty` → `DISPATCH_PROPERTYGET`，`PutProperty` → `DISPATCH_PROPERTYPUT` | `oleutil/oleutil.go:50-94`、`idispatch.go:62-75` |
| Go 参数类型映射：`int` → `VT_I4`（即 PowerPoint 的 Long）、`bool` → `VT_BOOL`、`string` → `VT_BSTR`、`nil` → `VT_NULL`。**没有「省略可选参数」的占位机制**（无 DISP_E_PARAMNOTFOUND 支持）→ PowerPoint 可选参数只能「位置传满」或「截断尾部」，不能跳中间 | `idispatch_windows.go:81-167` |
| 参数按自然顺序传入，go-ole 内部倒排进 DISPPARAMS（`n := len(params)-i-1`），调用方无需自己倒序 | `idispatch_windows.go:74-78` |
| PowerPoint 方法运行时失败（`DISP_E_EXCEPTION 0x80020009`）时，EXCEPINFO 的描述文本可经 `err.(*ole.OleError).Description()` / `SubError()` 取到——**「文件不存在/已被打开」等业务错误识别靠它** | `idispatch_windows.go:174-192`（`NewErrorWithSubError(hr, excepInfo.description, excepInfo)`） |
| HRESULT 经 `OleError.Code()` 取 `uintptr` | `error.go:26` |

PowerPoint Application 的 CLSID 为 `{91493441-5A91-11CF-8700-00AA0060263B}`（Office 家族著名 GUID，社区共识），但走 ProgID 让系统解析即可，避免硬编码出错。

### 1.3 LockOSThread 的必要性

**结论：必须加。** 依据是 COM 语义而非 go-ole 文档：

- `CoInitializeEx(COINIT_APARTMENTTHREADED)` 建立 STA——STA 对象（及通过该 STA 获取的接口指针）有**线程亲和性**，跨线程直接调用接口指针返回 `RPC_E_WRONG_THREAD (0x8001010E)`（MTA 下列集器可能替你兜住，但行为不可依赖）。
- Go goroutine 默认在线程间漂移：`CoInitializeEx` 之后 goroutine 可能被调度到另一线程，后续 Invoke 就在错的线程上。
- go-ole 官方示例（`_example/excel/main.go`）**没有** LockOSThread——单 goroutine 顺序脚本碰巧不跨线程；服务化场景（专用 goroutine + 上层超时管理分离）必须显式锁定。
- 本项目是 Wails GUI 应用，主线程已被 WebView2 占用，COM 会话必须放独立 goroutine；防御点：若锁到的线程此前被别的库初始化过 COM（如 MTA），`CoInitializeEx(STA)` 返回 `RPC_E_CHANGED_MODE (0x80010106)`——出现即放弃该线程重新起 goroutine 重试（Go 无法指定线程，靠新 goroutine 重新调度，实践中专线程首次初始化极少命中）。

### 1.4 headless 语义

- **PowerPoint Application 无 `Visible` 属性**：与 Excel 自动化不同，`PutProperty(app, "Visible", true/false)` 直接抛错。窗口存在性由每次 `Presentations.Open` 的 `WithWindow` 参数决定（微软 KB 257757 与大量社区自动化代码均印证）。
- **WithWindow:=msoFalse 打开时 Slide.Export 正常工作**（Office 2013+ 主流实证；更老版本有无窗口渲染异常的个案记录）。全新启动的 powerpnt 进程本身仍会启动（后台无窗口），`Quit` 即退。
- 替代/兜底：`Application.WindowState = ppWindowMinimized(2)` 最小化主窗口（ppWindowState：1=Normal、2=Minimized、3=Maximized）——仅在 WithWindow:=msoTrue 路径有意义。

### 1.5 Slide.Export 签名与缩放语义

```vb
Slide.Export(FileName As String, FilterName As String, _
             [ScaleWidth As Long], [ScaleHeight As Long])
' FilterName: "PNG" | "JPG" | "GIF" | "BMP" | "WMF"/"EMF"
```

- `ScaleWidth/ScaleHeight`：**导出图像的像素尺寸**（Long）。
- **纵横比语义（待实测，主流社区结论）**：输出保持幻灯片原始纵横比，传入 w/h 作为目标框约束（fit-inside，实际输出某一条边严格等于传入值、不留白）；传入与幻灯片比例不符的值不拉伸变形。implement 阶段用「读 PNG IHDR 宽高」做一次断言即可定案（PNG 宽高在文件头第 16-23 字节，`encoding/binary` 十行内搞定，可做成单测辅助）。
- 不传 w/h 时：默认导出分辨率受注册表 `HKCU\Software\Microsoft\Office\<ver>\PowerPoint\Options\ExportBitmapResolution`（默认 96，DPI 口径）影响；**显式传值时社区实测不受该注册表影响**（**待实测**确认本项目版本）。
- 双档分辨率（画廊小图 + 高清预览）**没有一次调用多尺寸的 API**，PNG 滤镜一次调用产出一个文件 → 每页 2 次 Export 调用（见第 6 节）。

---

## 2. 打开并跳转指定页

### 结论

**打开跳转场景必须 `WithWindow:=msoTrue`（无窗口则没有 View，无法跳页）；普通编辑窗口跳页的正确 API 是 `DocumentWindow.View.GotoSlide(n)`（不是 SlideShowView——那是放映视图的接口）；置前用 `window.Activate()`。缩略图导出场景则用 `WithWindow:=msoFalse`。两个场景的 Open 参数不同，不要共用一个打开封装的 WithWindow 值。**

### 2.1 Presentations.Open

```vb
Presentations.Open(FileName As String, _
                   ReadOnly As MsoTriState, _
                   Untitled As MsoTriState, _
                   WithWindow As MsoTriState) As Presentation
' MsoTriState: msoTrue = -1, msoFalse = 0（注意是 -1 不是 1）
```

- go-ole 无省略可选参数机制（第 1.2 节），**四个参数全部位置传满**，这也是 C#/PowerShell late-binding 社区的统一做法。
- `Untitled:=msoFalse`（关联原文件路径，导出/跳转场景都要）；`ReadOnly:=msoTrue` 规避写锁与改动用户文件（见第 5 节）。
- 两个防御性应用级设置（打开前设一次）：

```vb
Application.DisplayAlerts = ppAlertsNone        ' 压制改写提示等对话框（数值 ppAlertsNone=1，待实测）
Application.AutomationSecurity = msoAutomationSecurityForceDisable  ' =3，打开文件时禁宏，防宏弹窗/宏卡死
```

### 2.2 跳页 API

```vb
' 编辑窗口（DocumentWindow）的 View 为 SlideView 时：
ActiveWindow.View.GotoSlide(Index As Long, [ResetSlide As MsoTriState = msoTrue])
' Index 为幻灯片编号，1-based；Index 取 Slides.Count 边界内
```

- `DocumentWindow` 获取路径：`Application.ActiveWindow`（本队列串行独占下，最后打开的窗口即目标）或 `Presentation.Windows.Item(i)`（go-ole：`MustGetProperty(pres, "Windows").ToIDispatch()` → `CallMethod(windows, "Item", 1)`）。
- **SlideShowView.GotoSlide 是放映视图（Slide Show）的接口**，与编辑窗口 View 不是同一对象；查看型跳页（不开放映）走 `View.GotoSlide`。若窗口当前 ViewType 非幻灯片视图（`window.ViewType != ppViewSlide(1)`），先 `window.ViewType = ppViewSlide` 再 GotoSlide 更稳。
- 放映场景（本项目不涉及）才用 `Presentation.SlideShowSettings.Run()` → `SlideShowView.GotoSlide`。

### 2.3 窗口置前

```vb
window.Activate                       ' DocumentWindow.Activate，首选
Application.WindowState = ppWindowMaximized   ' 3，可选配合
```

- `Activate` 是 PowerPoint 原生置前手段，多数场景够用；若被前台锁（foreground lock）压住，用 Win32 `SetForegroundWindow(hwnd)` 兜底——需另取 PowerPoint 顶层窗口句柄（按 PID 枚举窗口），属于复杂度兜底项，MVP 不建议先做。
- 触发路径是用户点击（本应用持前台权限），`Activate` 失败概率低。

---

## 3. 稳定性工程：超时、强杀、重启

### 结论

**go-ole 的 Invoke 是阻塞 syscall，Go 无法中断、go-ole 无超时参数；正确模式 = 整个 COM 会话封装在专用 goroutine（LockOSThread）内 + 上层 `select` 等结果 channel 或超时；超时后不杀 goroutine（Go 做不到），而是杀本功能启动的 powerpnt 进程——服务器进程死亡使在途 Invoke 以 `RPC_S_SERVER_UNAVAILABLE (0x800706BA)` 返回，专用 goroutine 自然走到清理路径退出，不泄漏线程。强杀只杀「自己启动的实例」，定位手段用 CreateInstance 前后 POWERPNT.EXE 进程快照 diff（项目已有进程枚举基建可复用）；`taskkill /IM powerpnt.exe` 全杀被禁止（误杀用户实例）。**

### 3.1 实例归属判定（COM 拿不到服务器 PID）

COM API 不直接暴露 out-of-proc 服务器的 PID，且 PowerPoint 是 multi-use 服务器：**已有 PowerPoint 进程运行时，`CoCreateInstance` 返回的是用户现有实例的 Application 对象**（Office 家族统一行为，社区高置信共识）——此时导出会在用户实例里打开文件，且**绝不能 Quit/杀**。判定方案对比：

| 方案 | 做法 | 评价 |
|---|---|---|
| **进程快照 diff（推荐）** | CreateInstance 前枚举 `POWERPNT.EXE` PID 集合 → 创建后再枚举 → 新增 PID 归自己；新增为空 = 连接的用户实例 | 轻量（两次进程枚举，毫秒级）；项目已有 `service/process.go` 枚举先例（`ListProcesses`，`model.ProcessInfo` 有 Pid/Name，无 CommandLine 字段但本方案不需要）；竞态窗口（创建瞬间用户恰好启动 PowerPoint）极小且后果只是「少杀一次」 |
| CommandLine 过滤 `-Embedding` | WMI `Win32_Process WHERE Name='POWERPNT.EXE' AND CommandLine LIKE '%-Embedding%'`（COM 本地启动的进程带 `-Embedding` 标准标志，用户手动启动没有） | 可作交叉验证备选。工程缺点：`wmic` 已在 Win11 24H2 移除；PowerShell `Get-CimInstance` 单次 0.5-1s 偏贵；Go 直调 WMI COM（go-ole 调 `WbemScripting`）代码量大 |
| `taskkill /IM powerpnt.exe` | 按映像名全杀 | **禁止**——误杀用户正开着的 PowerPoint（项目 `model/process.go` 注释已沉淀「taskkill 成功仅代表信号送达」的教训，强杀判活语义同款适用） |

注意 PowerPoint 单实例语义的推论：用户已开 PowerPoint 时，我们 CreateInstance 拿到用户实例（无新增 PID）→ 该会话禁 Quit 禁杀，只能静默用（WithWindow:=msoFalse 打开导出不干扰用户）；用户没开时，我们创建的新实例带 `-Embedding`，任务结束 `Quit` 或超时强杀均安全。

### 3.2 超时结构（推荐形态）

```go
// comWorker goroutine（LockOSThread）：跑完整 COM 会话，结果经 channel 上报
type comResult struct{ err error }
done := make(chan comResult, 1)
go func() {
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()
    defer ole.CoUninitialize()          // S_FALSE(0x1) 也要配对
    done <- comResult{runExportSession(ctx)}   // 全部 COM 调用锁在本线程内
}()
select {
case r := <-done:
    return r.err
case <-time.After(300 * time.Second):   // SRS §2.2.2 单 PPT 300s
    killOurPowerpnt(pids)               // 杀自启实例 → 在途 Invoke 返回 0x800706BA
    r := <-done                         // worker 必然很快回来，等它走完清理路径
    return fmt.Errorf("超时强杀: %w", r.err)
}
```

要点：

- **所有 COM 调用必须留在锁定线程内**——上层超时 select 只能杀进程，不能代替 worker 做任何 COM 操作（跨线程 = `RPC_E_WRONG_THREAD 0x8001010E`）。
- 卡死场景主要是 PowerPoint 弹出模态对话框（损坏文件修复提示、密码框、字体缺失去留）时 Invoke 永不返回；`DisplayAlerts=ppAlertsNone + AutomationSecurity=ForceDisable` 压掉大部分来源，300s 超时兜底剩余。
- 杀进程后 worker 的 Invoke 必然返回（COM 代理检测管道断裂），正常路径下 goroutine 不泄漏；防御性兜底：`r := <-done` 再加一层宽超时（如 +10s），超限则记 slog.Warn「COM worker 线程未按预期退出」（观测性日志，不阻塞队列）。
- COM 激活超时：`CoCreateInstance` 首次启动 LocalServer 有 COM 层激活超时（约 120s 量级，注册表 `SystemDefaultActivationTimeout` 可调），失败返回错误而非永久挂起；300s 任务级超时能覆盖。

### 3.3 强杀执行

```go
exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F")   // 精确 PID 强杀
```

- 复用项目进程管理已沉淀的语义（`model/process.go:26-32`）：kill 后**轮询实测存活**（如 5s 内每 200ms 查 PID），`taskkill` 退出码 0 仅代表信号送达；未死再补一刀或记日志。PPT 场景 powerpnt 被 TerminateProcess 强杀必死，轮询是防御性收口。
- 进程枚举复用 `service/process.go` 的枚举实现（同包内直接调 util 层函数或抽内部 helper，不绕 Wails 方法）。

### 3.4 失败后重启 COM 实例

| 事项 | 结论 |
|---|---|
| `CoUninitialize` 配对 | 每次成功 `CoInitializeEx`（含 S_FALSE）一次，defer 保证 panic 路径也执行；STA 属线程不属对象，**实例重启不必重做 CoInitializeEx**（同 worker 线程复用 STA），只重 `CreateInstance` |
| 断连后清理 | powerpnt 死后，对已取的 `IDispatch`（pres/slides）调 `Release()` 会失败但无害（代理已断），go-ole 的 Release 只是 vtable 调用不 panic；照常 Release 后重新 `CreateInstance` |
| `RPC_E_SERVERCALL_RETRYLATER (0x8001010A)` | 语义「目标应用忙（如正显示模态对话框/正忙于内部操作）」——**应等待重试**而非重启：sleep 500ms-1s × 重试 N 次（如 3-5 次）后再判失败。注意与 `RPC_E_CALL_CANCELED (0x80010001)`（调用被消息过滤器取消）区分 |
| `RPC_S_SERVER_UNAVAILABLE (0x800706BA)` / `RPC_E_DISCONNECTED (0x80010108)` | 服务器进程已死 → 实例报废，走「重启实例继续后续文件」的失败隔离路径 |
| `DISP_E_EXCEPTION (0x80020009)` | PowerPoint 方法级业务失败（文件不存在/格式损坏/页码越界），Description 文本取原因；当前文件标记失败，实例通常仍健康可继续 |
| 队列语义 | 单文件失败隔离 = 终止当前 COM 会话（Close/Quit 自启实例 + 本线程 STA 保留）→ 队列继续下一个文件；与 SRS「失败隔离 = 终止当前 COM 会话、重启实例继续后续」一致 |

---

## 4. 无 PowerPoint 环境检测

### 结论

**两阶段探测：先查注册表（轻、毫秒级、不启动进程），通过后再以 CreateInstance 兜底确认（首次使用时做一次）；`CoCreateInstance` 失败错误码本身就是可靠的判定依据（其第一步就是查注册表），区别只在「探测是否产生启动 PowerPoint 的副作用」。结果做包级缓存。**

| 手段 | 判据 | 评价 |
|---|---|---|
| 注册表探测（推荐预检） | `HKCR\CLSID\{91493441-5A91-11CF-8700-00AA0060263B}\LocalServer32` 存在且值非空（`golang.org/x/sys/registry` 打 `registry.CLASSES_ROOT`，已依赖 x/sys）；或查 `HKCR\PowerPoint.Application` ProgID | 不启动进程、无副作用；缺口：注册表残留（卸载不净）会误报可用 |
| CreateInstance（兜底确认） | `oleutil.CreateObject` 错误码：`CO_E_CLASSSTRING 0x800401F3`（ProgID 无效）/ `REGDB_E_CLASSNOTREG 0x80040154`（类未注册）→ **无 PowerPoint** | 最可靠；代价：若注册表通过，会真的启动 powerpnt（2-5s）——所以只做一次并缓存，不做每次扫描的常规探测 |

**缓存形态**：包级 `atomic.Bool`（或 `sync.Once` + 可覆写）缓存「可用/不可用」；不可用结果按进程生命周期缓存即可（装 Office 不热发生，应用重启重新探测；不建议持久化到磁盘）。探测顺序：注册表 miss → 直接判不可用（缓存）；注册表 hit → 缓存「可用」并放行后续首次真实调用，真实调用若仍返回 0x80040154/0x800401F3 则回写「不可用」修正残留误报。

---

## 5. PowerPoint 文件被占用时的 COM 打开行为

### 结论

**`ReadOnly:=msoTrue` 打开可绕开写锁（文件被任何进程占用也能只读打开），缩略图导出场景应恒用只读，本质规避「占用」问题；真正的坑是「连接到用户实例时该文件已被用户打开」——此时 `Presentations.Open` 同名文件报错，正确做法是打开前先遍历 `Application.Presentations` 集合按 `FullName` 匹配复用既有 Presentation。错误码层面没有稳定的专用「占用」HRESULT，业务错误原因只能从 `DISP_E_EXCEPTION (0x80020009)` 的 EXCEPINFO Description 文本识别——所以策略上应「预防」（先查集合 + 只读打开）优先于「错误识别」。**

```go
// 打开前复用检查：用户实例已打开同名文件时直接复用（只读消费，不 Close 别人的）
found, _ := oleutil.MustGetProperty(pres, "Item", fullName) // Presentation 对象（按 FullName 匹配须逐个 Get FullName）
```

- 判定口径（Windows 文件锁语义）：目标文件被**另一 PowerPoint 进程**写锁时（本机罕见，Office 默认单实例进程），`Open(ReadOnly:=msoTrue)` 仍可读；被非 Office 进程独占写锁（如正在下载/写盘）时 Open 失败 → 按业务「跳过(文件被占用)」标记，支持重试（SRS §7 要求）。
- **失败后重试**注意：同一实例上 `Presentations.Item(fullName)` 集合查找的匹配键是完整路径字符串（大小写不敏感）。

---

## 6. 每页导出的性能量级

### 结论

**一个 Presentation 打开一次、循环导出全部页，是唯一合理形态（打开是秒级大头，单页 Export 是几十到几百毫秒）；每页重新打开属数量级劣化，无任何取舍空间。双档分辨率 = 每页 2 次 Export 调用（PNG 滤镜一次一个文件，无「一次多尺寸」API），第二次调用仅有重新编码+写盘的边际开销，且比重新打开 Presentation 便宜几个数量级——双档应同会话连出。**

| 操作 | 量级（经验值，须以本项目基准为准） |
|---|---|
| `Presentations.Open` | 小文件（<5MB）0.5-2s；大文件（50MB+/数百页）5-15s |
| `Slide.Export` 单页 480-640px 小图 | 约 20-100ms |
| `Slide.Export` 单页 1920px 高清 | 约 100-300ms |
| 100 页 PPT 双档全导 | 约 10-40s（远低于单 PPT 300s 超时——**超时防的是模态弹窗卡死，不是性能瓶颈**） |
| `Presentation.SaveAs(path, ppSaveAsPNG=18)` | 一次导出全部页 PNG 到文件夹，但尺寸不可控（固定 96dpi 口径，受 ExportBitmapResolution 影响）→ 不满足双档需求，仅作背景知识 |

推导：SRS 的 300s 单 PPT 超时覆盖数百页双档全导绰绰有余；真正消耗队列时长的是「文件数量 × 打开时间」，串行队列下缩略图批量应排在扫描完成后异步跑（不阻塞扫描链路本身）。

---

## 7. 参考实现

> 调研期间无可用外部检索通道，以下为可本地核验 + 领域内权威参考；GitHub 具体仓库名不凭记忆罗列（避免编造），给出可直接复跑的检索关键词。

| 参考 | 价值 | 位置 |
|---|---|---|
| go-ole 官方示例 `_example/excel/main.go` | Invoke 模式最小骨架（CreateObject → QueryInterface → PutProperty/CallMethod → Quit/Release → CoUninitialize），本项目链路照此扩 PowerPoint 语义即可 | 本地 module cache（见 1.1 路径） |
| go-ole `idispatch_windows.go` invoke 实现 | 参数映射/EXCEPINFO 透出的**唯一权威**（本文档 1.2 全部源码实证均出于此） | 同上 |
| Microsoft Learn VBA 参考：`Presentations.Open` / `Slide.Export` / `View.GotoSlide` / `Application.WindowState` / `PpAlertLevel` / `MsoTriState` | API 签名权威来源；implement 时逐个核对可选参数默认值 | learn.microsoft.com（VBA PowerPoint Object Model） |
| 微软 KB 257757《Considerations for server-side Automation of Office》 | PowerPoint 无 Visible 属性、Office 自动化在无桌面会话环境（Windows 服务）不受官方支持的官方立场——本项目是桌面 GUI 进程（有交互桌面）不受限，但该 KB 是「为什么用 WithWindow 而非 Visible」的权威出处 | 微软支持知识库 |
| Office 注册表参考 `ExportBitmapResolution` | 默认导出分辨率语义（96 DPI） | learn.microsoft.com / Office 相关注册表文档 |
| 检索关键词（implement 阶段复核用） | `go-ole powerpoint export slide png`（Go gist 级参考）、`PowerPoint View.GotoSlide C# automation`、`Slide.Export ScaleWidth aspect ratio`（纵横比行为复核）、`POWERPNT -Embedding command line`（进程归属判定复核） | GitHub / Stack Overflow |
| 免 COM 替代路线（背景） | LibreOffice headless（`soffice --headless --convert-to png/pdf`）是 DAM 类项目跨平台常用方案，但 Windows 上渲染保真度/字体依赖弱于本机 PowerPoint COM；纯 Go OOXML 库（unioffice 类）只读写不渲染，**Windows 下高保真缩略图只有 COM 一条路**——与 SRS 选型一致 | 领域共识 |

---

## Caveats / 待实测清单（implement 阶段逐项定案）

以下各项在源码/文档层面无法闭环，须在本机 Office（现装版本）上跑一次定案，均为低成本一次性验证：

1. **Slide.Export 纵横比行为**：传 480×270 与 1920×1080 对 16:9 幻灯片、传 480×480 对 16:9 幻灯片，读 PNG IHDR 断言实际输出尺寸（决定缩略图占位 CSS 宽高比）。
2. **WithWindow:=msoFalse 下 Slide.Export 可用性**：本机版本确认无窗口导出不黑图/不报错（2013+ 社区共识可用，个案兜底方案为 msoTrue + WindowState=Minimized）。
3. **ppAlertsNone 数值**（预判 1）与 `DisplayAlerts` 在 PowerPoint（非 Excel）上是否生效。
4. **显式 ScaleWidth/Height 是否绕开 ExportBitmapResolution**。
5. **CoCreateInstance 复用用户实例行为**：用户开着一个空 PowerPoint 时创建后进程数是否不变（决定「归属判定」中「新增 PID 为空 → 用户实例」判据的成立性）。
6. **`Presentation.Windows.Item(1)` 在 WithWindow:=msoFalse 时必然不存在**（无窗口无 Window 集合成员）——跳转场景才访问，导出场景代码路径不得触碰 Windows 集合。
7. COM 激活超时上限（注册表默认 120s 量级）与任务级 300s 的先后关系实测确认。

## 对本项目的落地建议

1. **COM 能力抽接口 + `_windows`/`_other` 双文件**：`type PptComProvider interface` 定义 `Available(ctx) bool`、`ExportThumbnails(ctx, path, outDir, specs []SizeSpec, onProgress) error`、`OpenAtSlide(ctx, path, slideNum) error`；windows 实现（go-ole）放 `service/xxx_windows.go`，`_other` 返回「不可用」恒定值（对齐 `fileoperation_windows.go` 分叉先例与 `PptxSplitProvider` 的 fake 注入模式，单测零 PowerPoint 依赖，满足 CI 约束）。
2. **专用 COM worker goroutine**：`runtime.LockOSThread` + `CoInitializeEx(COINIT_APARTMENTTHREADED)` + `defer CoUninitialize`；**放行 S_FALSE(0x1)**（go-ole 会当错误返回）；`RPC_E_CHANGED_MODE(0x80010106)` 时弃线程重起新 goroutine 重试一次。worker 常驻（串行队列天然满足 SRS「全局最多 1 个 COM 任务」），任务经带缓冲 channel 进入。
3. **超时 = 杀进程，不是杀 goroutine**：每任务 `select { done / time.After(300s) }`；超时路径 `taskkill /PID <自启PID> /F` + 轮询存活 5s，等 worker channel 回报清理完成（再兜一层宽超时记 slog.Warn）。杀进程让在途 Invoke 以 0x800706BA 返回、worker 走 defer 清理自然退出。
4. **实例归属判定用进程快照 diff**：CreateInstance 前后两次枚举 `POWERPNT.EXE` PID（复用 `service/process.go` 枚举实现），新增 PID 归自己（可 Quit/可强杀）；新增为空 = 复用用户实例（禁 Quit 禁杀，静默只读消费）。**禁止 `taskkill /IM` 按映像名全杀**。
5. **两个场景分开的 Open 参数**：导出 = `Open(path, -1, 0, 0)`（ReadOnly + WithWindow:false）；打开跳转 = `Open(path, -1, 0, -1)`（WithWindow:true）+ `ActiveWindow.View.GotoSlide(n)` + `window.Activate()` 置前。MsoTriState 用 -1/0 字面量并注释语义。
6. **防弹窗三板斧先于一切 Open**：`DisplayAlerts=ppAlertsNone` + `AutomationSecurity=msoAutomationSecurityForceDisable(3)` + 全程 ReadOnly；残余卡死由 300s 超时兜底。
7. **占用规避走「预防」不走「错误识别」**：恒 ReadOnly 打开；打开跳转前按 `FullName` 遍历 `Presentations` 集合复用已开文件；错误识别统一走 `OleError.Code()`（HRESULT）+ `Description()`（EXCEPINFO 文本）双通道，0x8001010A 延迟重试、0x800706BA/0x80010108 判实例死亡走失败隔离重启。
8. **可用性探测两阶段 + 包级缓存**：注册表预检（x/sys/registry 查 CLSID LocalServer32，不启动进程）→ 首次真实 CreateInstance 兜底修正 → `atomic.Bool` 缓存按进程生命周期；「不可用」态驱动 SRS 的整体降级（跳过缩略图仅文本索引、跳转入口禁用/提示）。
9. **性能形态定死**：一次 Open 循环导全部页 + 双档每页 2 次 Export 同会话连出；尺寸经「读 PNG IHDR」断言后写进单测辅助；progress 事件按页粒度（沿用 `assets-scan:progress` 带 libraryId 契约，新增 `assets-thumbs:*` 前缀）。
10. **复用既有基建清单**：cancels 注册表 + `normalizeRepoKey` 键 + defer LIFO（`pptx_split_service.go` 同款）；`sinkHolder` 事件出口（记得 `web_serve.go` `applySink` 补 `SetEventSink` 行，新服务漏加 = 浏览器通道静默丢事件的既有坑）；新错误码（如 E_PPT_COM_UNAVAILABLE / E_PPT_COM_TIMEOUT）同步 `model/app_error.go` + `frontend/src/utils/error.js` 双处。

---

## 探针定案（2026-10-14 本机实测，scripts/ppt-com-probe 六轮迭代）

> 本机环境：Windows 11 + **WPS Office**（`PowerPoint.Application` ProgID 由 WPS 接管——标准 MS CLSID 键存在但 `LocalServer32` 缺失，`CoCreateInstance` 实际启动 `wpp.exe`）。探针程序：`scripts/ppt-com-probe/main.go`（一次性工具，结论如下后归档）。

### 待实测清单逐项结论

| # | 清单项 | 结论 |
|---|---|---|
| 1 | Slide.Export 纵横比 | **显式传参精确生效**（480x270 出 480x270、480x480 出 480x480）——不保持纵横比不拉伸校验，**调用方须自行按幻灯片纵横比计算高度**（从 presentation.xml sldSz 读，parse 链路已有）；fit-inside 假设不成立 |
| 2 | WithWindow:=msoFalse 导出可用性 | **完全可用且渲染正常**（三形态非白占比一致 28.1%：无窗口/最小化/移屏外全渲染正常）。初判「黑图/白图」为 fixture 空白页误判（AddTextbox err 被吞 + 白底页采样全白）——渲染验证必须用带色块标记的 fixture |
| 3 | ppAlertsNone 数值 | =1 被接受（回读一致）；DisplayAlerts=2 同接受；AutomationSecurity=3 被接受 |
| 4 | 显式尺寸绕开 ExportBitmapResolution | **绕开成立**（默认导出 2560x1440 受注册表口径影响，显式传值精确生效） |
| 5 | CoCreateInstance 复用用户实例 | （交互项跳过）代码按「diff 为空 = 复用用户实例禁 Quit 禁杀」防御，语义安全 |
| 6 | WithWindow:=msoFalse 时 Windows 集合 | `Presentations.Count` 在 WPS 下**计数不可靠**（实测开着 1 个 Presentation 时 Count=0）——FullName 复用检查不能依赖集合完整枚举，生产策略 = 恒先直接 Open（WPS 实测**同名双开允许**），报「已打开」错时再遍历集合兜底复用 |
| 7 | COM 激活超时与任务超时 | 强杀后阻塞 Invoke **2.1-5.5s 返回**（`RPC_S_SERVER_UNAVAILABLE` 形态，error 描述「远程过程调用失败」）——300s 任务超时 + 10s worker 收尾宽限成立 |

### 实测新增结论（原清单外）

1. **Presentations 是属性不是方法**：go-ole `CallMethod` 只走 `DISPATCH_METHOD`，对 `Presentations` 报「找不到成员」（DISP_E_MEMBERNOTFOUND）——**必须 `GetProperty`**（`DISPATCH_PROPERTYGET`）。research §1.1 骨架的 `CallMethod(app, "Presentations")` 为笔误，生产代码已按 GetProperty 实现。
2. **WPS 接管 ProgID 的环境识别与归属判定**：进程快照 diff 的过滤集合须为 `{powerpnt.exe, wpp.exe}`（不能硬编码 powerpnt——WPS 环境启动的是 wpp.exe，硬编码会「枚举恒空 → 误判复用用户实例 → 禁杀」，超时路径失去强杀能力）。
3. **`0x8001010A`（RPC_E_SERVERCALL_RETRYLATER）可为持续态**：强杀 wpp.exe 后 COM 激活管线卡死数秒——`CreateInstance` 须带 5 次 × 1s 重试（research §3.4 的「等待重试」从建议升级为必须）。
4. **WPS 同名双开允许**：同文件已开（带窗口）再 Open（ReadOnly 无窗口）成功——MS Office 的「已打开报错」行为在 WPS 不复现；生产 Open 失败兜底逻辑保留（双引擎兼容）。
5. **手写 minimal OOXML 被 WPS 拒开**（「发生意外」desc 空）——探针 fixture 改 COM 自产（Presentations.Add + AddShape 红矩形 + SaveAs）；生产链路开用户真实 PPT 不受影响。
6. **渲染验证方法**：像素判定须用「非白像素占比」（50×50 网格抽样）+ **带色块标记的 fixture**——白底页 + 小文字的「多样性」判定恒误报纯色。

### 对生产代码的影响核对（assets_thumbs_windows.go 已对齐）

- GetProperty 取 Presentations/Slides/PageSetup ✓
- 归属判定进程集合 {powerpnt.exe, wpp.exe} ✓（`officeServerProcNames`）
- CreateInstance 0x8001010A 重试 5×1s ✓
- 导出恒 ww=0（本机 WPS 验证渲染正常）✓
- FullName 复用检查保留但按「先 Open 后兜底」语义兼容（WPS 集合计数不可靠下遍历空集自然直开）✓
- 强杀路径秒级返回 + worker 10s 宽限 ✓
