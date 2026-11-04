package service

import (
	"strings"
	"unicode/utf8"
)

// utf8StreamDecoder 将任意切分的字节流还原为不含截断字符的字符串。
//
// PTY 的 Read 边界与 UTF-8 字符边界无关：一次读取可能恰好停在 3 字节汉字
// 或 4 字节 emoji 中间。若直接 string(buf[:n]) 经 Wails 事件发送，JSON
// 序列化会把非法字节替换为 U+FFFD，前端看到的是永久乱码。本类型按会话持有
// 最多 3 字节残片，只在字符完整后输出；非法字节按 U+FFFD 归一，保证返回值
// 始终是合法 UTF-8。
//
// 非并发安全：每个输出泵 goroutine 独占一个实例。
type utf8StreamDecoder struct {
	carry []byte
}

// Write 追加一段字节，返回可安全发送的完整前缀。末尾不完整的字符留待下次调用。
func (d *utf8StreamDecoder) Write(p []byte) string {
	if len(p) == 0 {
		return ""
	}
	// 快路径：无残片且整段合法，无需拼接与逐字符扫描
	if len(d.carry) == 0 && utf8.Valid(p) {
		return string(p)
	}

	buf := p
	if len(d.carry) > 0 {
		buf = make([]byte, 0, len(d.carry)+len(p))
		buf = append(buf, d.carry...)
		buf = append(buf, p...)
	}

	// 推进到最后一个完整字符边界；utf8.FullRune 对非法编码返回 true，
	// 因此非法字节会按 width-1 的错误字符越过，不会阻塞后续输出。
	i := 0
	for i < len(buf) {
		if !utf8.FullRune(buf[i:]) {
			break
		}
		_, size := utf8.DecodeRune(buf[i:])
		i += size
	}

	d.carry = append(d.carry[:0], buf[i:]...)

	out := buf[:i]
	if !utf8.Valid(out) {
		return strings.ToValidUTF8(string(out), "�")
	}
	return string(out)
}

// Flush 在流结束时输出残留的不完整序列（按 U+FFFD 归一），并清空内部状态。
func (d *utf8StreamDecoder) Flush() string {
	if len(d.carry) == 0 {
		return ""
	}
	out := strings.ToValidUTF8(string(d.carry), "�")
	d.carry = d.carry[:0]
	return out
}
