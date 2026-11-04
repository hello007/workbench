package service

import (
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"workbench/model"
)

// TestUTF8StreamDecoder_EverySplit 按 1..len 的任意块大小切分同一段中文/emoji，
// 拼接结果必须与原文逐字节一致（覆盖切片落在多字节字符内部的全部位置）。
func TestUTF8StreamDecoder_EverySplit(t *testing.T) {
	const payload = "中文🙂abc—终端🀄"
	for split := 1; split <= len(payload); split++ {
		d := &utf8StreamDecoder{}
		var got strings.Builder
		for i := 0; i < len(payload); i += split {
			end := i + split
			if end > len(payload) {
				end = len(payload)
			}
			got.WriteString(d.Write([]byte(payload[i:end])))
		}
		got.WriteString(d.Flush())
		if got.String() != payload {
			t.Fatalf("split=%d: 期望 %q, 实际 %q", split, payload, got.String())
		}
	}
}

// TestUTF8StreamDecoder_BufferBoundary 4096 字节读取边界恰好切断汉字时不得产生替换字符。
func TestUTF8StreamDecoder_BufferBoundary(t *testing.T) {
	payload := strings.Repeat("中", 3000) // 9000 字节，4096 落在字符中间
	d := &utf8StreamDecoder{}
	var got strings.Builder
	for i := 0; i < len(payload); i += 4096 {
		end := i + 4096
		if end > len(payload) {
			end = len(payload)
		}
		got.WriteString(d.Write([]byte(payload[i:end])))
	}
	got.WriteString(d.Flush())
	if got.String() != payload {
		t.Fatalf("4096 边界切分后输出不一致")
	}
	if strings.ContainsRune(got.String(), utf8.RuneError) {
		t.Fatal("输出不应包含替换字符")
	}
}

// TestUTF8StreamDecoder_InvalidBytes 非法字节不得 panic/阻塞，输出须为合法 UTF-8。
func TestUTF8StreamDecoder_InvalidBytes(t *testing.T) {
	d := &utf8StreamDecoder{}
	got := d.Write([]byte{0xff, 'a', 0xfe}) + d.Flush()
	if !utf8.ValidString(got) {
		t.Fatalf("输出不是合法 UTF-8: %q", got)
	}
	if !strings.Contains(got, "a") {
		t.Fatalf("非法字节后的正常内容被吞: %q", got)
	}
	if !strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("非法字节应归一为替换字符: %q", got)
	}
}

// TestUTF8StreamDecoder_FlushIncomplete 末尾残缺序列在 Flush 时输出替换字符，
// 且此前完整内容不受影响、二次 Flush 为空。
func TestUTF8StreamDecoder_FlushIncomplete(t *testing.T) {
	d := &utf8StreamDecoder{}
	const prefix = "前缀"
	if got := d.Write([]byte(prefix)); got != prefix {
		t.Fatalf("完整前缀应立即输出, 实际 %q", got)
	}
	// "中" = E4 B8 AD，仅喂前两字节
	if got := d.Write([]byte{0xE4, 0xB8}); got != "" {
		t.Fatalf("残缺字符不应提前输出, 实际 %q", got)
	}
	tail := d.Flush()
	if tail == "" || !utf8.ValidString(tail) {
		t.Fatalf("Flush 应输出合法替换字符, 实际 %q", tail)
	}
	if again := d.Flush(); again != "" {
		t.Fatalf("二次 Flush 应为空, 实际 %q", again)
	}
}

// TestUTF8StreamDecoder_EmptyWrite 空写入不改变状态、不产生输出。
func TestUTF8StreamDecoder_EmptyWrite(t *testing.T) {
	d := &utf8StreamDecoder{}
	if got := d.Write(nil); got != "" {
		t.Fatalf("空写入应返回空串, 实际 %q", got)
	}
	if got := d.Flush(); got != "" {
		t.Fatalf("空实例 Flush 应为空, 实际 %q", got)
	}
}

// --- 输出泵集成测试辅助 ---

type recordedEvent struct {
	name string
	data []any
}

type recordingSink struct {
	events []recordedEvent
}

func (s *recordingSink) Emit(name string, data ...any) {
	s.events = append(s.events, recordedEvent{name: name, data: data})
}

// chunkedReader 按预设块逐次返回数据，块耗尽后返回 io.EOF。
type chunkedReader struct {
	chunks [][]byte
	index  int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.index])
	r.index++
	return n, nil
}

// eofWithDataReader 模拟允许在最后一次读取同时返回数据和 io.EOF 的 Reader。
type eofWithDataReader struct {
	data []byte
	sent bool
}

func (r *eofWithDataReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	return copy(p, r.data), io.EOF
}

// TestStartOutputPump_DataWithEOF 最后一次读取同时返回数据和 io.EOF 时，
// 数据不得被丢弃。
func TestStartOutputPump_DataWithEOF(t *testing.T) {
	const payload = "尾包中文"
	sink := &recordingSink{}
	svc := NewTerminalService(nil)
	svc.SetEventSink(sink)
	svc.startOutputPump("term-test", &eofWithDataReader{data: []byte(payload)})

	var got strings.Builder
	for _, ev := range sink.events {
		if ev.name == "terminal-output" {
			got.WriteString(ev.data[1].(string))
		}
	}
	if got.String() != payload {
		t.Fatalf("同包 EOF 数据丢失: 期望 %q, 实际 %q", payload, got.String())
	}
}

// TestStartOutputPump_FlushTailAtEOF 流在多字节字符中途结束时，输出泵须先吐出
// 归一后的残片（替换字符）再发 terminal-exit，不得静默丢弃，也不得发出半个字符。
func TestStartOutputPump_FlushTailAtEOF(t *testing.T) {
	// "中" = E4 B8 AD，只喂前两字节后 EOF
	reader := &chunkedReader{chunks: [][]byte{{0xE4, 0xB8}}}
	sink := &recordingSink{}
	svc := NewTerminalService(nil)
	svc.SetEventSink(sink)
	// 会话须在册，输出泵才会在其结束时补发 terminal-exit
	svc.sessions["term-test"] = &model.TerminalSession{ID: "term-test", Running: true}
	svc.startOutputPump("term-test", reader)

	var outputs []string
	sawExit := false
	for _, ev := range sink.events {
		switch ev.name {
		case "terminal-output":
			text, ok := ev.data[1].(string)
			if !ok {
				t.Fatalf("载荷类型应为 string, 实际 %T", ev.data[1])
			}
			if !utf8.ValidString(text) {
				t.Fatalf("残片载荷不是合法 UTF-8: %q", text)
			}
			outputs = append(outputs, text)
		case "terminal-exit":
			sawExit = true
		}
	}
	if len(outputs) != 1 || outputs[0] != "�" {
		t.Fatalf("EOF 残片应输出单个替换字符, 实际 %q", outputs)
	}
	if !sawExit {
		t.Fatal("流结束后应发送 terminal-exit")
	}
}

// TestStartOutputPump_SplitUTF8AcrossReads 逐字节喂入中文/emoji，输出泵拼回的
// 字符串必须与原文一致，且每条 terminal-output 载荷都是合法 UTF-8、非空。
func TestStartOutputPump_SplitUTF8AcrossReads(t *testing.T) {
	const payload = "第一行：中文\n第二行：🙂🙂\n"
	chunks := make([][]byte, 0, len(payload))
	for i := 0; i < len(payload); i++ {
		chunks = append(chunks, []byte{payload[i]})
	}

	sink := &recordingSink{}
	svc := NewTerminalService(nil)
	svc.SetEventSink(sink)
	svc.startOutputPump("term-test", &chunkedReader{chunks: chunks})

	var got strings.Builder
	outputs := 0
	for _, ev := range sink.events {
		if ev.name != "terminal-output" {
			continue
		}
		outputs++
		text, ok := ev.data[1].(string)
		if !ok {
			t.Fatalf("载荷类型应为 string, 实际 %T", ev.data[1])
		}
		if text == "" {
			t.Fatal("不应发送空输出事件")
		}
		if !utf8.ValidString(text) {
			t.Fatalf("载荷不是合法 UTF-8: %q", text)
		}
		got.WriteString(text)
	}
	if outputs == 0 {
		t.Fatal("未收到任何 terminal-output 事件")
	}
	if got.String() != payload {
		t.Fatalf("输出泵拼接结果不一致: 期望 %q, 实际 %q", payload, got.String())
	}
}
