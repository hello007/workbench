# util clipboard unsafe.Pointer vet 告警修复

## 背景与问题

裸 `go vet ./util/` 报 7 处 `possible misuse of unsafe.Pointer`（源码 5 + 测试 2）：

| 位置 | 模式 |
|---|---|
| util/clipboard_windows.go:78 | `unsafe.Pointer(GlobalLock 返回的 uintptr)` 转 `unsafe.Slice` 写 DROPFILES |
| util/clipboard_windows.go:118 | 同上，写 Preferred DropEffect |
| util/clipboard_windows.go:159 | 同上，读 DROPFILES 缓冲 |
| util/clipboard_windows.go:194 | 同上，读 Preferred DropEffect |
| util/clipboard_windows.go:226 | 同上，写 CF_UNICODETEXT |
| util/clipboard_windows_test.go:33 | 同上，GlobalAlloc/Lock 往返写读校验 |
| util/clipboard_windows_test.go:45 | 同上，重锁读回 |

历史：commit d7d5b51（2026-09-13）未消除告警，实际做的是加安全论证注释 + 项目 vet 命令固化为 `go vet -unsafeptr=false` 压制。裸 `go vet` 绕过压制即「复发」。

## 实证结论（Go 1.26.6 本机探针验证）

1. **vet unsafeptr 无 syscall 白名单**：检查器（GOROOT `cmd/vendor/golang.org/x/tools/go/analysis/passes/unsafeptr/unsafeptr.go`）仅豁免三类——`reflect.Value.Pointer()/UnsafeAddr()` 结果、`reflect.SliceHeader/StringHeader.Data`、`(uintptr)(unsafe.Pointer)` 算术回转。`syscall.Syscall`/`SyscallN` 返回值转 Pointer 实测均被 flag。「SyscallN 直调」路径不可行。
2. **x/sys/windows 无封装**：v0.30/v0.38/v0.46/v0.48 全版本无 GlobalAlloc/GlobalLock/GlobalUnlock/GlobalSize 与任何 Clipboard API。「x/sys 封装」路径不存在。
3. **RtlMoveMemory 中转 vet 构造性干净**（探针验证通过）：目标地址以裸 uintptr 直传 `proc.Call` 参数，Go 侧仅 `uintptr(unsafe.Pointer(&buf[0]))` 作 Call 直传实参（unsafe 规则第 6 条 blessed 模式，同步调用内有效），全程零 `uintptr→unsafe.Pointer` 回转。

## 方案

RtlMoveMemory 中转：

- 写路径（WriteClipboardFiles/WriteClipboardText）：Go 堆 `[]byte` 编码 DROPFILES+路径 / UTF-16 文本 → GlobalAlloc/GlobalLock 得裸 uintptr → `RtlMoveMemory(dst uintptr, src &goBuf[0], len)` 一次拷入 → GlobalUnlock → SetClipboardData。
- 读路径（ReadClipboardFiles）：GlobalLock → 按 GlobalSize 拷出到 Go `[]byte` → 解析逻辑不变（偏移类型 uintptr 改 int）。
- 新增 `copyToHeap(dst uintptr, src []byte)` / `readFromHeap(src uintptr, size int)` 两个辅助 + `procRtlMoveMemory` 声明。
- 测试文件同步改造：模式写入与读回均走两辅助，保留「Windows 可移动堆往返」语义锚点，转换不再出现。

## 硬约束

1. 剪贴板行为零变化：CF_HDROP/CF_UNICODETEXT/Preferred DropEffect 字节布局逐位不变，中文与多格式复制粘贴不受影响。
2. 零新依赖（syscall 足够）。
3. Linux 零影响（`//go:build windows` 文件不参与 Linux 构建）。

## 收口目标

1. 裸 `go vet ./...` 全绿（至少 util 包），移除 `-unsafeptr=false` 压制需求。
2. docs/spec/linux-platform.md §10「known-vet-issues 已知误报」登记删除或改写为已消除。
3. docs/开发规范.md「静态检查（go vet）」章节同步改写（d7d5b51 引入的压制记录失效）。
4. `go build ./...`、`go test ./util/ ./ -count=1`、`GOOS=linux go build ./...` 通过。
5. 剪贴板行为验证：Windows 本机手工验证点清单输出（复制/粘贴文件、剪切语义、中文文本）。

## 验收

- [ ] `go vet ./...` 退出码 0（无 -unsafeptr=false）
- [ ] `go build ./...` 通过
- [ ] `go test ./util/ ./ -count=1` 通过
- [ ] `GOOS=linux go build ./...` 通过
- [ ] 两份文档收口
- [ ] 手工验证点清单交付
