package model

import (
	"errors"
	"testing"
)

func TestAppError_Error_NoErr(t *testing.T) {
	e := NewAppError("E_TEST", "测试错误")
	want := "[E_TEST] 测试错误"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAppError_Error_WithErr(t *testing.T) {
	root := errors.New("root cause")
	e := WrapAppError("E_TEST", "测试错误", root)
	want := "[E_TEST] 测试错误: root cause"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAppError_Unwrap_PreservesSentinel(t *testing.T) {
	sentinel := errors.New("sentinel")
	e := WrapAppError("E_TEST", "包装", sentinel)
	// errors.Is 经 Unwrap 链穿透，AppError 包装后仍可识别根因 sentinel
	if !errors.Is(e, sentinel) {
		t.Error("errors.Is 应穿透 AppError 识别根因 sentinel")
	}
}

func TestAppError_As_ExtractsAppError(t *testing.T) {
	root := errors.New("root")
	e := WrapAppError("E_TEST", "包装", root)
	var target *AppError
	// errors.As 提取 AppError 类型字段（Code/Message）
	if !errors.As(e, &target) {
		t.Fatal("errors.As 应提取 *AppError")
	}
	if target.Code != "E_TEST" || target.Message != "包装" {
		t.Errorf("提取字段错: code=%q message=%q", target.Code, target.Message)
	}
}

func TestAppError_NilErr_UnwrapNil(t *testing.T) {
	e := NewAppError("E_TEST", "无根因")
	if e.Unwrap() != nil {
		t.Error("无根因 AppError Unwrap 应返 nil")
	}
}
