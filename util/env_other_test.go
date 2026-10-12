//go:build !windows

package util

import (
	"errors"
	"testing"
)

// 非 Windows 平台断言独立成文件（docs/spec/linux-platform.md §8：
// 禁两平台共享一份平台耦合断言）。

func TestSystemEnv_UnsupportedOnNonWindows(t *testing.T) {
	entries, err := SystemEnv()
	if !errors.Is(err, ErrEnvSourceUnsupported) {
		t.Fatalf("非 Windows 平台 SystemEnv 应返回 unsupported 哨兵错误: got %v", err)
	}
	if entries != nil {
		t.Errorf("unsupported 时不应有条目: got %+v", entries)
	}
}

func TestUserEnv_UnsupportedOnNonWindows(t *testing.T) {
	entries, err := UserEnv()
	if !errors.Is(err, ErrEnvSourceUnsupported) {
		t.Fatalf("非 Windows 平台 UserEnv 应返回 unsupported 哨兵错误: got %v", err)
	}
	if entries != nil {
		t.Errorf("unsupported 时不应有条目: got %+v", entries)
	}
}
