//go:build windows

package util

import (
	"bytes"
	"strconv"
	"testing"
)

// TestGlobalAllocLockView 锚定 clipboard_windows.go 的 Windows 堆内存往返语义。
// GlobalAlloc(GMEM_MOVEABLE) 的 HGLOBAL 经 GlobalLock 得到 Windows 堆地址，经 copyToHeap
// 写入确定性模式，解锁重锁后经 readFromHeap 逐字节读回校验一致——证明 RtlMoveMemory
// 中转模式下数据真实写入 Windows 可移动堆，且解锁重锁（地址可能被系统搬移）后按
// HGLOBAL 句柄重取仍一致，与生产代码「写后 Unlock、系统按句柄重取」语义一致。
// 不依赖系统剪贴板（OpenClipboard 易受其他进程独占锁影响，属环境外部状态，禁入单测）。
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
			// 锁定期经 copyToHeap 写入确定性模式（值域 1-251，避开零值内存可区分）
			want := make([]byte, size)
			for i := range want {
				want[i] = byte(i%251 + 1)
			}
			copyToHeap(ptr, want)
			procGlobalUnlock.Call(hMem)

			// 可移动内存解锁后可能被系统搬移，须重锁取新地址读回（与生产代码
			// 写后 Unlock、系统按 HGLOBAL 句柄重取的语义一致）
			ptr2, _, _ := procGlobalLock.Call(hMem)
			if ptr2 == 0 {
				t.Fatalf("GlobalLock 重锁(%d) 失败", size)
			}
			got := readFromHeap(ptr2, int(size))
			if !bytes.Equal(got, want) {
				t.Fatalf("读回不一致 size=%d", size)
			}
			procGlobalUnlock.Call(hMem)
		})
	}
}

// TestHeapTransferZeroSizeGuard 校验 copyToHeap/readFromHeap 对空缓冲的防护：
// &buf[0] 取址在空 slice 上会 panic，两辅助必须短路返回而非触达该路径。
func TestHeapTransferZeroSizeGuard(t *testing.T) {
	copyToHeap(0, nil) // 不 panic 即通过
	if got := readFromHeap(0, 0); len(got) != 0 {
		t.Fatalf("readFromHeap 零长度应返回空切片，实际 len=%d", len(got))
	}
}
