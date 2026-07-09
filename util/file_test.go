package util

import (
	"testing"
)

// TestDetectTextEncoding_UTF8 合法 UTF-8 文本应原样返回，encoding=utf-8
func TestDetectTextEncoding_UTF8(t *testing.T) {
	data := []byte("Hello, 世界！UTF-8 文本")
	enc, content, ok := DetectTextEncoding(data)
	if !ok {
		t.Fatal("UTF-8 文本应判定为可显示")
	}
	if enc != "utf-8" {
		t.Errorf("编码期望 utf-8, 实际 %s", enc)
	}
	if content != string(data) {
		t.Errorf("内容应原样返回, 实际 '%s'", content)
	}
}

// TestDetectTextEncoding_GBK 非 UTF-8 的 GBK 中文文本应解码为 UTF-8，encoding=gbk
func TestDetectTextEncoding_GBK(t *testing.T) {
	// "中文GBK文本" 的 GBK 编码字节
	// 中=D6D0 文=CEC4 G=47 B=42 K=4B 文=CEC4 本=B1BE
	data := []byte{0xD6, 0xD0, 0xCE, 0xC4, 0x47, 0x42, 0x4B, 0xCE, 0xC4, 0xB1, 0xBE}
	enc, content, ok := DetectTextEncoding(data)
	if !ok {
		t.Fatal("GBK 文本应判定为可显示")
	}
	if enc != "gbk" {
		t.Errorf("编码期望 gbk, 实际 %s", enc)
	}
	if content != "中文GBK文本" {
		t.Errorf("解码内容期望 '中文GBK文本', 实际 '%s'", content)
	}
}

// TestDetectTextEncoding_BinaryWithNUL 含 NUL 字节(0x00)应判为二进制不可显示
func TestDetectTextEncoding_BinaryWithNUL(t *testing.T) {
	data := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x00, 0x77, 0x6F, 0x72, 0x6C, 0x64}
	_, _, ok := DetectTextEncoding(data)
	if ok {
		t.Error("含 NUL 字节应判定为二进制不可显示")
	}
}

// TestDetectTextEncoding_NULBeyond8KB NUL 字节出现在 8KB 之后不应被判为二进制（仅扫前 8KB）
func TestDetectTextEncoding_NULBeyond8KB(t *testing.T) {
	// 前 8KB 为合法 UTF-8（'a' 重复），第 8193 字节为 NUL
	data := make([]byte, 8193)
	for i := 0; i < 8192; i++ {
		data[i] = 'a'
	}
	data[8192] = 0x00
	enc, content, ok := DetectTextEncoding(data)
	if !ok {
		t.Fatal("NUL 在 8KB 之后不应判为二进制")
	}
	if enc != "utf-8" {
		t.Errorf("编码期望 utf-8, 实际 %s", enc)
	}
	if len(content) != 8193 {
		t.Errorf("内容长度期望 8193, 实际 %d", len(content))
	}
}

// TestDetectTextEncoding_NonUTF8NonGBK 非 UTF-8 且 GBK 解码后替换字符占比高应判为不可显示
func TestDetectTextEncoding_NonUTF8NonGBK(t *testing.T) {
	// 0xFF 不是合法 GBK 引导字节（范围 0x81-0xFE），每个 0xFF 解码为 U+FFFD，占比 100%
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	_, _, ok := DetectTextEncoding(data)
	if ok {
		t.Error("非 UTF-8 非 GBK 字节应判定为不可显示")
	}
}

// TestDetectTextEncoding_Empty 空文件应判定为可显示，encoding=utf-8，内容为空
func TestDetectTextEncoding_Empty(t *testing.T) {
	enc, content, ok := DetectTextEncoding(nil)
	if !ok {
		t.Error("空文件应判定为可显示")
	}
	if enc != "utf-8" {
		t.Errorf("空文件编码期望 utf-8, 实际 %s", enc)
	}
	if content != "" {
		t.Errorf("空文件内容期望空串, 实际 '%s'", content)
	}
}

// TestDetectTextEncoding_PureASCII 纯 ASCII 是合法 UTF-8，应返回 utf-8
func TestDetectTextEncoding_PureASCII(t *testing.T) {
	data := []byte("plain ascii text 12345")
	enc, content, ok := DetectTextEncoding(data)
	if !ok {
		t.Fatal("纯 ASCII 应判定为可显示")
	}
	if enc != "utf-8" {
		t.Errorf("编码期望 utf-8, 实际 %s", enc)
	}
	if content != string(data) {
		t.Errorf("内容应原样返回, 实际 '%s'", content)
	}
}
