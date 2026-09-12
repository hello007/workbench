package model

import "fmt"

// AppError 统一应用错误类型，携带机器可读错误码与用户可读消息。
//
// 设计：经 Wails ErrorFormatter（main.go 注册）将 Code/Message 序列化为
// {code, message} 对象传前端，前端按 code 分流提示（warning/error），
// 无需中文字符串匹配。Error() 仍返回可读字符串供日志与兜底场景。
//
// 与现有 sentinel（ErrOperationInProgress 等）兼容：AppError 可通过
// NewWrappedAppError 包装 sentinel 作为 Err 字段，errors.Is(err, sentinel)
// 经 Unwrap 链穿透仍成立，保留后端内部 errors.Is 判定能力。
//
// 详见 docs/spec/logging-and-errors.md。
type AppError struct {
	// Code 机器可读错误码，如 "E_GIT_IN_PROGRESS"。前端按此分流。
	Code string
	// Message 用户可读中文消息，前端展示。
	Message string
	// Err 根因 error，支持 errors.Is/As 穿透包装链。可为 nil（无根因）。
	Err error
}

// Error 实现 error 接口。返回 Code + Message 供日志与兜底字符串场景。
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 暴露根因，支持 errors.Is/As 穿透 AppError 包装链。
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError 构造无根因的 AppError（如纯业务拒绝类：操作进行中）。
func NewAppError(code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// WrapAppError 构造带根因的 AppError，保留 errors.Is/As 穿透能力。
func WrapAppError(code, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

// 错误码常量表。命名规范 E_<域>_<动作/状态>，前端按 code 映射提示级别。
// 新增错误码须同步本表 + docs/spec/logging-and-errors.md + 前端 error.js codeMap。
const (
	// Git 域
	ErrCodeGitInProgress = "E_GIT_IN_PROGRESS" // 变更类操作进行中，本次拒绝（warning）
)
