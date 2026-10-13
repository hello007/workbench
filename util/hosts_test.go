package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"workbench/model"
)

// TestParseHosts 全形态解析 fixture：IPv4 / IPv6 / 带 zone 的 link-local / 行内注释 /
// 纯注释（含缩进）/ 空行 / 纯空白行 / 未识别行 / 仅 IP 行，覆盖 PRD 验收面（禁依赖
// 本机真实 hosts 内容，docs/spec/test-stability.md）。
func TestParseHosts(t *testing.T) {
	content := strings.Join([]string{
		"# 本机 hosts 解析 fixture",
		"",
		"127.0.0.1 localhost",
		"10.20.30.40 host-a host-b # 行内注释",
		"::1 ip6-local ip6-alias",
		"fe80::1%eth0 linklocal", // 带 zone 的 link-local：net.ParseIP 不认，netip.ParseAddr 认
		"192.168.1.1",            // 仅 IP 无主机名：entry 形态，主机名为空列表
		"   # 缩进注释",
		"   ",
		"not-an-ip host-x",
	}, "\n")

	got := ParseHosts(content)
	want := []HostsEntry{
		{Kind: HostsKindComment, Raw: "# 本机 hosts 解析 fixture"},
		{Kind: HostsKindBlank, Raw: ""},
		{Kind: HostsKindEntry, IP: "127.0.0.1", Hostnames: []string{"localhost"}, Raw: "127.0.0.1 localhost"},
		{Kind: HostsKindEntry, IP: "10.20.30.40", Hostnames: []string{"host-a", "host-b"}, Comment: "行内注释", Raw: "10.20.30.40 host-a host-b # 行内注释"},
		{Kind: HostsKindEntry, IP: "::1", Hostnames: []string{"ip6-local", "ip6-alias"}, Raw: "::1 ip6-local ip6-alias"},
		{Kind: HostsKindEntry, IP: "fe80::1%eth0", Hostnames: []string{"linklocal"}, Raw: "fe80::1%eth0 linklocal"},
		{Kind: HostsKindEntry, IP: "192.168.1.1", Hostnames: []string{}, Raw: "192.168.1.1"},
		{Kind: HostsKindComment, Raw: "   # 缩进注释"},
		{Kind: HostsKindBlank, Raw: "   "},
		{Kind: HostsKindUnknown, Raw: "not-an-ip host-x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("解析结果不符:\n got %+v\nwant %+v", got, want)
	}
}

// TestParseHosts_BOM 容忍 UTF-8 BOM：BOM 剥离后首行按正常映射行解析（否则首个 token
// 携带 BOM 字节必然 ParseAddr 失败被误判为未识别行）。
func TestParseHosts_BOM(t *testing.T) {
	got := ParseHosts(hostsBOM + "127.0.0.1 localhost\n")
	want := []HostsEntry{{Kind: HostsKindEntry, IP: "127.0.0.1", Hostnames: []string{"localhost"}, Raw: "127.0.0.1 localhost"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BOM 应被剥离:\n got %+v\nwant %+v", got, want)
	}
}

// TestParseHosts_CRLFAndTrailingNewline CRLF 兼容 + 末尾换行是终止符不产生多余空行条目。
func TestParseHosts_CRLFAndTrailingNewline(t *testing.T) {
	got := ParseHosts("127.0.0.1 localhost\r\n::1 ip6-local\r\n")
	want := []HostsEntry{
		{Kind: HostsKindEntry, IP: "127.0.0.1", Hostnames: []string{"localhost"}, Raw: "127.0.0.1 localhost"},
		{Kind: HostsKindEntry, IP: "::1", Hostnames: []string{"ip6-local"}, Raw: "::1 ip6-local"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CRLF 行尾 \\r 应剥离且不产生尾部空行:\n got %+v\nwant %+v", got, want)
	}
}

// TestParseHosts_EmptyContent 空内容返回空列表（非 nil，前端拿空数组）；单个换行
// 是「终止一个空行」——文件确有一个空行，产生 1 条 blank 条目（区别于空串的零行）。
func TestParseHosts_EmptyContent(t *testing.T) {
	if got := ParseHosts(""); got == nil || len(got) != 0 {
		t.Errorf("空串应为非 nil 空列表: got %+v", got)
	}
	got := ParseHosts("\n")
	if len(got) != 1 || got[0].Kind != HostsKindBlank {
		t.Errorf("单个换行应为 1 条空行条目: got %+v", got)
	}
}

// TestReadHostsFile_Success 读文件 + 解析编排（t.TempDir fixture 注入路径）。
func TestReadHostsFile_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	content := "127.0.0.1 localhost # fixture\n# 注释行\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写 fixture: %v", err)
	}
	entries, err := ReadHostsFile(path)
	if err != nil {
		t.Fatalf("ReadHostsFile: %v", err)
	}
	want := []HostsEntry{
		{Kind: HostsKindEntry, IP: "127.0.0.1", Hostnames: []string{"localhost"}, Comment: "fixture", Raw: "127.0.0.1 localhost # fixture"},
		{Kind: HostsKindComment, Raw: "# 注释行"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("条目不符:\n got %+v\nwant %+v", entries, want)
	}
}

// TestReadHostsFile_NotExist 文件不存在包装为 AppError（E_HOSTS_READ_FAILED + 用户可读消息），
// 根因保留供 service 层取 Message 下发组级 Error 字段。
func TestReadHostsFile_NotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-hosts")
	_, err := ReadHostsFile(path)
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("应包装为 *model.AppError，实际 %T: %v", err, err)
	}
	if appErr.Code != model.ErrCodeHostsReadFailed {
		t.Errorf("错误码不符: got %q, want %q", appErr.Code, model.ErrCodeHostsReadFailed)
	}
	if appErr.Message != "hosts 文件不存在" {
		t.Errorf("消息分类不符: got %q", appErr.Message)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Error("根因 os.ErrNotExist 应保留（errors.Is 可穿透）")
	}
}

// TestReadHostsFile_EmptyFile 文件存在但为空：不报错，返回空列表（非 nil）。
func TestReadHostsFile_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("写空 fixture: %v", err)
	}
	entries, err := ReadHostsFile(path)
	if err != nil {
		t.Fatalf("空文件不应报错: %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Errorf("空文件应返回非 nil 空列表: got %+v", entries)
	}
}

// TestReadHostsFile_DirAsPath 目录当 hosts 路径：读取必然失败并包为 AppError。失败类别
// 存在平台差异（Windows read 目录句柄报拒绝访问；Linux read 目录报 EISDIR），消息走向
// 由 TestHostsReadFailureMessage 以注入根因确定性覆盖，此处只断言编排包装形态，不做
// 平台耦合断言（docs/spec/linux-platform.md §8）。
func TestReadHostsFile_DirAsPath(t *testing.T) {
	_, err := ReadHostsFile(t.TempDir())
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("应包装为 *model.AppError，实际 %T: %v", err, err)
	}
	if appErr.Code != model.ErrCodeHostsReadFailed {
		t.Errorf("错误码不符: got %q", appErr.Code)
	}
	if appErr.Message == "" {
		t.Error("用户可读消息不应为空")
	}
}

// TestHostsReadFailureMessage 消息分类纯函数：注入根因错误确定性断言，不依赖
// 文件系统实际行为的平台差异。
func TestHostsReadFailureMessage(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"不存在", fmt.Errorf("open: %w", os.ErrNotExist), "hosts 文件不存在"},
		{"无权限", fmt.Errorf("open: %w", os.ErrPermission), "hosts 文件无读取权限"},
		{"其他失败", errors.New("boom"), "hosts 文件读取失败"},
	}
	for _, c := range cases {
		if got := hostsReadFailureMessage(c.err); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
