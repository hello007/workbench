//go:build windows

package util

import (
	"encoding/binary"
	"errors"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	cfHDrop        = 15
	cfUnicodeText  = 13
	gmemMoveable   = 0x0002
	dropEffectMove = 2
	dropFilesSize  = 20
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procRegisterClipboardFormatW   = user32.NewProc("RegisterClipboardFormatW")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
	procRtlMoveMemory              = kernel32.NewProc("RtlMoveMemory")
)

// copyToHeap 将 Go 缓冲内容整体拷入 dst 指向的 Windows 堆内存（GlobalLock 返回地址）。
//
// 数据搬运统一走 RtlMoveMemory：GlobalLock 返回的 Windows 堆地址以裸 uintptr 直接作
// proc.Call 实参传递；Go 侧仅将 &src[0] 转为 uintptr 作同步 Call 的直传实参（unsafe.Pointer
// 安全规则中「Pointer 转 uintptr 作为系统调用实参」的合法用法，调用期间指针存活），全程
// 不发生 uintptr→unsafe.Pointer 回转。go vet 的 unsafeptr 分析器仅豁免 reflect.Value.Pointer/
// UnsafeAddr 结果、reflect.SliceHeader.Data 与 (uintptr)(unsafe.Pointer) 指针算术回转三类，
// 对 syscall 返回的 uintptr（含 syscall.Syscall/SyscallN 与 LazyProc.Call）一律报告
// "possible misuse of unsafe.Pointer"（golang/go#44836 未落地豁免）；本模式不出现该转换，
// 裸 go vet 即无告警。
func copyToHeap(dst uintptr, src []byte) {
	if len(src) == 0 {
		return
	}
	procRtlMoveMemory.Call(dst, uintptr(unsafe.Pointer(&src[0])), uintptr(len(src)))
}

// readFromHeap 从 src 指向的 Windows 堆内存（GlobalLock 返回地址）拷出 size 字节到新建
// Go 缓冲。拷贝与解析分离后，剪贴板数据的全部解析均在 Go 内存上进行。
func readFromHeap(src uintptr, size int) []byte {
	buf := make([]byte, size)
	if size == 0 {
		return buf
	}
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&buf[0])), src, uintptr(size))
	return buf
}

// openClipboardWithRetry 尝试以独占方式打开剪贴板，失败时短暂重试，最多 3 次。
// 用于规避 Windows 剪贴板独占锁与其他应用的瞬时竞态。
func openClipboardWithRetry() bool {
	for i := 0; i < 3; i++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r != 0 {
			return true
		}
		if i < 2 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return false
}

// WriteClipboardFiles 将文件路径写入系统剪贴板（CF_HDROP 格式）
func WriteClipboardFiles(paths []string, isCut bool) error {
	if len(paths) == 0 {
		return errors.New("no paths provided")
	}

	totalSize := uintptr(dropFilesSize)
	for _, p := range paths {
		totalSize += uintptr((len(utf16.Encode([]rune(p))) + 1) * 2)
	}
	totalSize += 2

	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, totalSize)
	if hMem == 0 {
		return errors.New("GlobalAlloc failed")
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return errors.New("GlobalLock failed")
	}

	// 在 Go 堆上按 DROPFILES 结构组装：pFiles 指向文件列表起始偏移，fWide=1 表示 UTF-16 路径
	buf := make([]byte, totalSize)
	binary.LittleEndian.PutUint32(buf[0:4], dropFilesSize)
	binary.LittleEndian.PutUint32(buf[16:20], 1) // fWide=1

	offset := dropFilesSize
	for _, p := range paths {
		encoded := utf16.Encode([]rune(p))
		for _, c := range encoded {
			binary.LittleEndian.PutUint16(buf[offset:], uint16(c))
			offset += 2
		}
		binary.LittleEndian.PutUint16(buf[offset:], 0)
		offset += 2
	}
	binary.LittleEndian.PutUint16(buf[offset:], 0)

	copyToHeap(ptr, buf)
	procGlobalUnlock.Call(hMem)

	if !openClipboardWithRetry() {
		return errors.New("OpenClipboard failed after retries")
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	r, _, _ := procSetClipboardData.Call(cfHDrop, hMem)
	if r == 0 {
		return errors.New("SetClipboardData failed")
	}

	if isCut {
		fmtName, _ := syscall.UTF16PtrFromString("Preferred DropEffect")
		cfDropEffect, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(fmtName)))

		hEffect, _, _ := procGlobalAlloc.Call(gmemMoveable, 4)
		if hEffect != 0 {
			effectPtr, _, _ := procGlobalLock.Call(hEffect)
			if effectPtr != 0 {
				effectBuf := make([]byte, 4)
				binary.LittleEndian.PutUint32(effectBuf, dropEffectMove)
				copyToHeap(effectPtr, effectBuf)
				procGlobalUnlock.Call(hEffect)
				procSetClipboardData.Call(cfDropEffect, hEffect)
			}
		}
	}

	return nil
}

// ReadClipboardFiles 读取系统剪贴板中的文件路径列表
func ReadClipboardFiles() (paths []string, isCut bool, err error) {
	if !openClipboardWithRetry() {
		return nil, false, nil
	}
	defer procCloseClipboard.Call()

	available, _, _ := procIsClipboardFormatAvailable.Call(cfHDrop)
	if available == 0 {
		return nil, false, nil
	}

	hData, _, _ := procGetClipboardData.Call(cfHDrop)
	if hData == 0 {
		return nil, false, errors.New("GetClipboardData failed")
	}

	size, _, _ := procGlobalSize.Call(hData)
	if size == 0 {
		return nil, false, errors.New("GlobalSize failed")
	}

	ptr, _, _ := procGlobalLock.Call(hData)
	if ptr == 0 {
		return nil, false, errors.New("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(hData)

	buf := readFromHeap(ptr, int(size))

	pFiles := binary.LittleEndian.Uint32(buf[0:4])
	fWide := binary.LittleEndian.Uint32(buf[16:20])

	if fWide == 0 {
		return nil, false, errors.New("ANSI format not supported")
	}

	offset := int(pFiles)
	if offset < 0 {
		// 剪贴板内容来自外部进程，pFiles 越过 int 正域时跳过文件列表解析
		offset = len(buf)
	}
	for offset+1 < len(buf) {
		var chars []uint16
		for offset+1 < len(buf) {
			c := binary.LittleEndian.Uint16(buf[offset:])
			offset += 2
			if c == 0 {
				break
			}
			chars = append(chars, c)
		}
		if len(chars) == 0 {
			break
		}
		paths = append(paths, string(utf16.Decode(chars)))
	}

	fmtName, _ := syscall.UTF16PtrFromString("Preferred DropEffect")
	cfDropEffect, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(fmtName)))

	hEffect, _, _ := procGetClipboardData.Call(cfDropEffect)
	if hEffect != 0 {
		effectPtr, _, _ := procGlobalLock.Call(hEffect)
		if effectPtr != 0 {
			effectBuf := readFromHeap(effectPtr, 4)
			effect := binary.LittleEndian.Uint32(effectBuf)
			procGlobalUnlock.Call(hEffect)
			if effect == dropEffectMove {
				isCut = true
			}
		}
	}

	return paths, isCut, nil
}

// WriteClipboardText 将文本写入系统剪贴板（CF_UNICODETEXT 格式）。
// UTF-16 编码文本 + 末尾 0 终止符；复用 WriteClipboardFiles 的 GlobalAlloc/GlobalLock/OpenClipboard 模式。
// 供「打开 Obsidian 仓库管理器」前复制 vault 路径，便于用户在 Obsidian 路径栏粘贴。
func WriteClipboardText(text string) error {
	// UTF-16 编码，末尾补 0 终止符（占 2 字节）
	encoded := utf16.Encode([]rune(text))
	totalSize := uintptr(len(encoded)+1) * 2

	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, totalSize)
	if hMem == 0 {
		return errors.New("GlobalAlloc failed")
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return errors.New("GlobalLock failed")
	}

	buf := make([]byte, totalSize)
	for i, c := range encoded {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(c))
	}
	binary.LittleEndian.PutUint16(buf[len(encoded)*2:], 0) // 末尾 0 终止符

	copyToHeap(ptr, buf)
	procGlobalUnlock.Call(hMem)

	if !openClipboardWithRetry() {
		return errors.New("OpenClipboard failed after retries")
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	r, _, _ := procSetClipboardData.Call(cfUnicodeText, hMem)
	if r == 0 {
		return errors.New("SetClipboardData failed")
	}
	return nil
}
