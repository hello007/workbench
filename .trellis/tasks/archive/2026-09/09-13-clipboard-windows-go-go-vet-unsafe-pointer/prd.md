# 修复 util/clipboard_windows.go 的 5 处 go vet unsafe.Pointer 告警

## Goal

`go vet ./...` 全绿，且 Windows 剪贴板功能行为完全不变。

## 告警现状

```
util/clipboard_windows.go:76:30   possible misuse of unsafe.Pointer
util/clipboard_windows.go:114:39  possible misuse of unsafe.Pointer
util/clipboard_windows.go:153:30  possible misuse of unsafe.Pointer
util/clipboard_windows.go:186:38  possible misuse of unsafe.Pointer
util/clipboard_windows.go:216:30  possible misuse of unsafe.Pointer
```

5 处同模式：`procGlobalLock.Call()` 返回 `uintptr` 转 `unsafe.Pointer` 构建 `unsafe.Slice`（WriteClipboardFiles / ReadClipboardFiles / WriteClipboardText 三个函数内的 GlobalLock 结果）。

## 调研结论（决定方案的关键事实）

1. **vet unsafeptr 无 syscall 豁免**：分析器源码豁免名单仅 3 类——reflect SliceHeader/StringHeader 的 `.Data` 字段、`reflect.Value.Pointer()`/`UnsafeAddr()` 无参调用、`uintptr(unsafe.Pointer(p))` 算术链。syscall 返回值转 Pointer 一律报。
2. **实测确认**：`proc.Call(...)` 与 `syscall.SyscallN(...)` 两种形态均报（本地 Go 1.26 实验验证）。
3. **x/sys/windows 无 GlobalLock/GlobalAlloc API**（v0.38.0 grep 确认），且全库刻意规避 uintptr→Pointer 反向转换；[golang/go#44836](https://github.com/golang/go/issues/44836)（请求提供返回 unsafe.Pointer 的 API）未落地。
4. **零漏检代价**：全仓 `unsafe.Pointer` 仅 `util/clipboard_windows.go` 一个文件使用，关闭 unsafeptr 分析器不会放过任何其他文件的潜在误用。
5. **go test / CI 不受影响**：`go test` 默认 vet 子集不含 unsafeptr；CI（ci.yml）无独立 vet 步骤。告警仅在手动 `go vet ./...` 出现。
6. **此处转换实质安全**：`GlobalLock` 锁定的 HGLOBAL 内存为 Windows 堆分配（非 Go 堆），GC 不移动不回收；这是 Windows 剪贴板 API 的标准使用模式。

## Requirements

**选定方案：vet 命令带 `-unsafeptr=false` + 代码安全论证注释**

- 项目 vet 命令固化为 `go vet -unsafeptr=false ./...`（文档同步 + 如有必要加脚本）
- 5 处告警位置补注释：说明为何安全（HGLOBAL 非 Go 堆内存，锁定期地址稳定）+ 为何无法消除（vet unsafeptr 无 syscall 豁免，golang/go#44836）
- 剪贴板功能行为零改动

**否决的备选**：
- reflect.SliceHeader.Data 绕行：deprecated（Go 1.20+ 强烈不推荐），本质骗 vet，引入新技术债
- cgo 重写：破坏纯 Go 现状，交叉编译复杂化
- 换第三方剪贴板库：atotto/clipboard 仅覆盖文本，CF_HDROP 文件列表仍需手写，不彻底

## Acceptance Criteria

- [ ] `go vet -unsafeptr=false ./...` 零告警
- [ ] 5 处 unsafe.Pointer 转换处均有安全论证注释
- [ ] 剪贴板相关单测全绿（行为不变）
- [ ] `go test ./...` 零回归
- [ ] 文档同步：vet 命令更新至 `docs/开发规范.md`（或 DEVELOPMENT.md），记录 `-unsafeptr=false` 原因

## Technical Notes

- `go test` 默认 vet 子集：atomic/bool/buildtags/directive/errorsas/ifaceassert/nilfunc/printf/stringintconv/tests/slog（不含 unsafeptr）
- 修复本身不动任何 Go 代码逻辑，只加注释 + 文档/命令
- 若未来 Go 落地 syscall 豁免（如 #44836 后续），可移除 `-unsafeptr=false`
