//go:build windows

package util

import (
	"strconv"
	"testing"
	"unsafe"
)

// TestGlobalAllocLockView 锚定 clipboard_windows.go 的 uintptr→unsafe.Pointer 转换语义。
// GlobalAlloc(GMEM_MOVEABLE) 的 HGLOBAL 经 GlobalLock 得到 Windows 堆地址，转 unsafe.Slice
// 视图写入确定性模式，解锁重锁后逐字节读回校验一致——证明该转换产生的内存视图真实可写可读，
// 即 go vet unsafeptr 报告（对 syscall 返回值无豁免，见 util/clipboard_windows.go 注释与
// docs/开发规范.md）的实质安全性经验锚点。不依赖系统剪贴板（OpenClipboard 易受其他进程
// 独占锁影响，属环境外部状态，禁入单测）。
func TestGlobalAllocLockView(t *testing.T) {
	procGlobalFree := kernel32.NewProc("GlobalFree")
	sizes := []uintptr{4, 64, 4096} // 覆盖 DropEffect(4B)/文本短串/DROPFILES 大缓冲三档
	for _, size := range sizes {
		t.Run("size_"+strconv.FormatUint(uint64(size), 10), func(t *testing.T) {
			hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
			if hMem == 0 {
				t.Fatalf("GlobalAlloc(%d) 失败", size)
			}
			defer procGlobalFree.Call(hMem)

			ptr, _, _ := procGlobalLock.Call(hMem)
			if ptr == 0 {
				t.Fatalf("GlobalLock(%d) 失败", size)
			}
			// 锁定期经转换视图写入确定性模式（值域 1-251，避开零值内存可区分）
			buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)
			for i := range buf {
				buf[i] = byte(i%251 + 1)
			}
			procGlobalUnlock.Call(hMem)

			// 可移动内存解锁后可能被系统搬移，须重锁取新地址读回（与生产代码
			// 写后 Unlock、系统按 HGLOBAL 句柄重取的语义一致）
			ptr2, _, _ := procGlobalLock.Call(hMem)
			if ptr2 == 0 {
				t.Fatalf("GlobalLock 重锁(%d) 失败", size)
			}
			buf2 := unsafe.Slice((*byte)(unsafe.Pointer(ptr2)), size)
			for i := range buf2 {
				if want := byte(i%251 + 1); buf2[i] != want {
					t.Fatalf("读回不一致 size=%d offset=%d: 期望 %d, 实际 %d", size, i, want, buf2[i])
				}
			}
			procGlobalUnlock.Call(hMem)
		})
	}
}
