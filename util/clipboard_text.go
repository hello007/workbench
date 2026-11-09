package util

import "errors"

// 剪贴板文本读取的两种「无文本」语义哨兵。
//
// 分开的原因是提示文案不同：用户以为复制了文字却没生效（空剪贴板），
// 与用户复制的是文件（有不含文本的内容）是两类操作预期，混成一个
// 「剪贴板为空」会让人误判为复制失败。
var (
	// ErrClipboardEmpty 剪贴板无任何可用内容。
	ErrClipboardEmpty = errors.New("clipboard is empty")
	// ErrClipboardNoText 剪贴板有内容但没有文本格式（典型为复制的文件列表）。
	ErrClipboardNoText = errors.New("clipboard has no text content")
)
