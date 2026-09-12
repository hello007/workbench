package main

import (
	"errors"
	"testing"

	"workbench/model"
)

func TestFormatAppError_AppError(t *testing.T) {
	err := model.NewAppError("E_GIT_IN_PROGRESS", "该仓库有 Git 操作进行中")
	result := formatAppError(err)

	m, ok := result.(map[string]string)
	if !ok {
		t.Fatalf("formatAppError 应返回 map[string]string, got %T", result)
	}
	if m["code"] != "E_GIT_IN_PROGRESS" {
		t.Errorf("code = %q, want E_GIT_IN_PROGRESS", m["code"])
	}
	if m["message"] != "该仓库有 Git 操作进行中" {
		t.Errorf("message = %q", m["message"])
	}
}

func TestFormatAppError_WrappedAppError(t *testing.T) {
	root := errors.New("lock busy")
	err := model.WrapAppError("E_GIT_IN_PROGRESS", "操作进行中", root)
	result := formatAppError(err)

	m, ok := result.(map[string]string)
	if !ok {
		t.Fatalf("应返回 map, got %T", result)
	}
	// 包装链 AppError 仍提取 code/message，不含根因字符串（根因留后端日志）
	if m["code"] != "E_GIT_IN_PROGRESS" {
		t.Errorf("code = %q", m["code"])
	}
	if m["message"] != "操作进行中" {
		t.Errorf("message = %q, 应只含 Message 不含根因", m["message"])
	}
}

func TestFormatAppError_PlainError(t *testing.T) {
	err := errors.New("普通错误")
	result := formatAppError(err)

	m, ok := result.(map[string]string)
	if !ok {
		t.Fatalf("应返回 map, got %T", result)
	}
	if m["message"] != "普通错误" {
		t.Errorf("message = %q", m["message"])
	}
	if _, hasCode := m["code"]; hasCode {
		t.Error("普通 error 不应有 code 字段")
	}
}

func TestFormatAppError_NilPanicGuard(t *testing.T) {
	// formatAppError 接收非 nil error（Wails 仅在有 error 时调）。
	// 此用例验证普通 error 路径稳定，不触发 nil 解引用。
	err := errors.New("")
	result := formatAppError(err)
	if result == nil {
		t.Error("空消息 error 仍应返回非 nil map")
	}
}
