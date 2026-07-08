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
)

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

	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), totalSize)
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
				effectBuf := unsafe.Slice((*byte)(unsafe.Pointer(effectPtr)), 4)
				binary.LittleEndian.PutUint32(effectBuf, dropEffectMove)
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

	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)

	pFiles := binary.LittleEndian.Uint32(buf[0:4])
	fWide := binary.LittleEndian.Uint32(buf[16:20])

	if fWide == 0 {
		return nil, false, errors.New("ANSI format not supported")
	}

	offset := uintptr(pFiles)
	for offset+1 < uintptr(len(buf)) {
		var chars []uint16
		for offset+1 < uintptr(len(buf)) {
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
			effectBuf := unsafe.Slice((*byte)(unsafe.Pointer(effectPtr)), 4)
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

	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), totalSize)
	for i, c := range encoded {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(c))
	}
	binary.LittleEndian.PutUint16(buf[len(encoded)*2:], 0) // 末尾 0 终止符

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
