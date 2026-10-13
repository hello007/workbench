package service

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"workbench/util"
)

// TestGetHosts_Success 读取编排成功：fixture 路径注入（不依赖本机真实 hosts 内容，
// docs/spec/test-stability.md），util 条目映射为 model DTO、Error 为空。
func TestGetHosts_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	content := "127.0.0.1 localhost # 注释\n# 纯注释行\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写 fixture: %v", err)
	}

	view, err := (&HostsService{path: path}).GetHosts()
	if err != nil {
		t.Fatalf("GetHosts: %v", err)
	}
	if view.Path != path {
		t.Errorf("Path 应为注入的 fixture 路径: got %q, want %q", view.Path, path)
	}
	if view.Error != "" {
		t.Errorf("成功时 Error 应为空: got %q", view.Error)
	}
	if len(view.Entries) != 2 {
		t.Fatalf("应返回 2 条条目: got %d", len(view.Entries))
	}
	entry := view.Entries[0]
	if entry.Kind != "entry" || entry.IP != "127.0.0.1" ||
		!reflect.DeepEqual(entry.Hostnames, []string{"localhost"}) ||
		entry.Comment != "注释" || entry.Raw != "127.0.0.1 localhost # 注释" {
		t.Errorf("entry 条目映射不符: %+v", entry)
	}
	if view.Entries[1].Kind != "comment" || view.Entries[1].Raw != "# 纯注释行" {
		t.Errorf("comment 条目映射不符: %+v", view.Entries[1])
	}
}

// TestGetHosts_NotExist 文件不存在：不外抛整体 error，用户可读消息随组级 Error 字段
// 下发（对齐 EnvVarGroup.Error 单来源失败形态），条目为空数组（非 nil）。
func TestGetHosts_NotExist(t *testing.T) {
	svc := &HostsService{path: filepath.Join(t.TempDir(), "missing-hosts")}
	view, err := svc.GetHosts()
	if err != nil {
		t.Fatalf("读取失败不应外抛整体 error: %v", err)
	}
	if view.Error == "" {
		t.Error("失败时应带组级 Error 消息")
	}
	if view.Path != svc.path {
		t.Errorf("失败时 Path 仍应回填: got %q, want %q", view.Path, svc.path)
	}
	if view.Entries == nil || len(view.Entries) != 0 {
		t.Errorf("失败时条目应为非 nil 空数组: got %+v", view.Entries)
	}
}

// TestGetHosts_EmptyFile 文件存在但为空：不报错、无 Error，条目为空数组。
func TestGetHosts_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("写空 fixture: %v", err)
	}
	view, err := (&HostsService{path: path}).GetHosts()
	if err != nil {
		t.Fatalf("空文件不应报错: %v", err)
	}
	if view.Error != "" || view.Entries == nil || len(view.Entries) != 0 {
		t.Errorf("空文件应为无 Error 的空条目视图: got %+v", view)
	}
}

// TestGetHosts_DirAsPath 目录当 hosts 路径：读取必然失败并走组级 Error 字段（失败类别
// 消息在 Windows/Linux 上分类不同，平台耦合断言在 util 层按平台拆分，此处只验证编排
// 形态——不外抛、Error 非空）。
func TestGetHosts_DirAsPath(t *testing.T) {
	view, err := (&HostsService{path: t.TempDir()}).GetHosts()
	if err != nil {
		t.Fatalf("读取失败不应外抛整体 error: %v", err)
	}
	if view.Error == "" {
		t.Error("失败时应带组级 Error 消息")
	}
	if len(view.Entries) != 0 {
		t.Errorf("失败时条目应为空: got %d", len(view.Entries))
	}
}

// TestBuildHostsEntries 条目映射：Hostnames 补非 nil（前端拿空数组而非 null），
// 其余字段一一对应透传。
func TestBuildHostsEntries(t *testing.T) {
	got := buildHostsEntries([]util.HostsEntry{
		{Kind: "entry", IP: "127.0.0.1", Hostnames: []string{"localhost"}, Comment: "c", Raw: "raw1"},
		{Kind: "entry", IP: "::1", Raw: "raw2"}, // Hostnames 为 nil
	})
	if got[0].Kind != "entry" || !reflect.DeepEqual(got[0].Hostnames, []string{"localhost"}) {
		t.Errorf("条目[0] 映射不符: %+v", got[0])
	}
	if got[1].Hostnames == nil || len(got[1].Hostnames) != 0 {
		t.Errorf("nil Hostnames 应映射为空数组: got %#v", got[1].Hostnames)
	}
}
