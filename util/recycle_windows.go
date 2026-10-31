//go:build windows

// util/recycle_windows.go
package util

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// Windows Shell API 常量（shellapi.h / shlobj.h）。
const (
	foDelete = 3 // FO_DELETE：删除（配合 FOF_ALLOWUNDO 即移入回收站）

	fofAllowUndo       = 0x0040 // 保留撤销信息 = 移入回收站而非永久删除
	fofNoErrorUI       = 0x0400 // 不弹错误对话框（错误码经返回值交给调用方）
	fofWantNukeWarning = 0x4000 // 将被永久删除时警告（部分覆盖 FOF_NOCONFIRMATION）

	// COINIT_APARTMENTTHREADED STA 初始化，以及「此线程已按其他模式初始化」的
	// HRESULT（RPC_E_CHANGED_MODE）。后者不视为失败：回收站操作不强制 STA，
	// 忽略并照常执行可避免误判。
	coinitApartmentThreaded = 0x2
	rpcEChangedMode         = 0x80010106
)

// SHFileOperationW 返回值（shellapi.h DE_* 错误码），按「文件可能被占用或权限
// 不足」统一给出用户可读原因——这几个码在素材域对应同一处置（关闭占用程序/
// 修正只读属性后重试）。
const (
	deAccessDenied    = 0x20 // DE_ACCESSDENIED
	deAccessDeniedSrc = 0x78 // DE_ACCESSDENIEDSRC
	deShareViolation  = 0x7C // DE_SHAREVIOLATION
)

// shFileOpStructW 对应 Windows SHFILEOPSTRUCTW。字段顺序与对齐必须与 C 结构
// 一致（Go 在 64 位下按 8 字节对齐自然补齐，与 C 编译器布局相同）。
type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

var (
	shell32Proc         = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperation = shell32Proc.NewProc("SHFileOperationW")
	ole32Proc           = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx  = ole32Proc.NewProc("CoInitializeEx")
	procCoUninitialize  = ole32Proc.NewProc("CoUninitialize")
)

// moveToRecycleBinPlatform Windows 实现：SHFileOperationW + FOF_ALLOWUNDO。
// recycleFlags SHFileOperationW 的删除标志单点。
//
// 关键取舍：**禁止 FOF_NOCONFIRMATION / FOF_SILENT**。回收站关闭、文件超过
// 回收站配额或目标在网络盘时，Shell 会退化为永久删除；带 FOF_NOCONFIRMATION
// 时该退化静默发生且返回 0，调用方会误判为「已移入回收站」。改为
// FOF_WANTNUKEWARNING 后，永久删除前 Shell 弹警告，用户取消即
// fAnyOperationsAborted != 0，源文件保留并返回错误（PRD R1）。
func recycleFlags() uint16 {
	return fofAllowUndo | fofWantNukeWarning | fofNoErrorUI
}

func moveToRecycleBinPlatform(path string) error {
	// Shell API 对线程公寓模型敏感：锁定线程并在当前线程尽力初始化 STA，
	// 结束后配对反初始化（与 SHFileOperation 的文档建议一致）
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	switch uint32(hr) {
	case 0, 1: // S_OK / S_FALSE：本次调用成功初始化，须配对反初始化
		defer procCoUninitialize.Call()
	case rpcEChangedMode:
		// 线程已被初始化为其他公寓模型：忽略并照常执行，不做反初始化
	default:
		// 其他 HRESULT（如 E_OUTOFMEMORY）不在此处拦截——继续调用回收站 API，
		// 让真实失败经返回值暴露，避免把可恢复场景误判为不可用
	}

	// pFrom 必须是以双 NUL 结尾的 UTF-16 字符串（UTF16FromString 自带单 NUL，
	// 追加一个构成双 NUL 终止）
	from, err := syscall.UTF16FromString(path)
	if err != nil {
		return fmt.Errorf("无法移入回收站：路径含非法字符: %w", err)
	}
	from = append(from, 0)

	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: recycleFlags(),
	}
	ret, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("无法移入回收站：操作被中止（%s）", path)
	}
	if ret != 0 {
		return fmt.Errorf("无法移入回收站（%s）：%s，文件可能被占用或权限不足", path, windowsRecycleErrText(uint32(ret)))
	}
	return nil
}

// windowsRecycleErrText 把 SHFileOperationW 返回码翻译为用户可读原因。
func windowsRecycleErrText(code uint32) string {
	switch code {
	case deAccessDenied:
		return "拒绝访问"
	case deAccessDeniedSrc:
		return "源文件拒绝访问"
	case deShareViolation:
		return "文件共享冲突（可能被其他程序打开）"
	default:
		return fmt.Sprintf("Shell API 错误码 0x%X", code)
	}
}
