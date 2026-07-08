//go:build !windows

package util

import "errors"

// WriteClipboardText 非 Windows 平台暂不支持文本剪贴板写入，返回错误。
// 本特性当前仅面向 Windows（与现有 WriteClipboardFiles/ReadClipboardFiles 一致）。
func WriteClipboardText(text string) error {
	return errors.New("clipboard text not supported on this platform")
}
