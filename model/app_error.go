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
	ErrCodeGitInProgress      = "E_GIT_IN_PROGRESS"       // 变更类操作进行中，本次拒绝（warning）
	ErrCodeGitNoStagedChanges = "E_GIT_NO_STAGED_CHANGES" // 无暂存文件，AI 提交信息生成按钮禁用（warning）

	// 外部 diff 工具域
	ErrCodeDiffToolNotConfigured = "E_DIFF_TOOL_NOT_CONFIGURED" // 未配置或配置无效（缺路径/参数模板占位符），引导用户去设置（warning）
	ErrCodeDiffToolLaunchFailed  = "E_DIFF_TOOL_LAUNCH_FAILED"  // 可执行文件不存在或启动失败（error）

	// 仓库列表配置导入导出域
	ErrCodeRepoConfigInvalidJSON        = "E_REPO_CONFIG_INVALID_JSON"        // 导入文件不是合法 JSON 或顶层结构缺失（error）
	ErrCodeRepoConfigUnsupportedVersion = "E_REPO_CONFIG_UNSUPPORTED_VERSION" // manifestVersion 缺失或高于当前支持，须用兼容版本应用重新导出（error）

	// AI 对话域（AI 对话工作台多轮会话）
	ErrCodeChatSessionNotFound = "E_CHAT_SESSION_NOT_FOUND" // 会话不存在或已删除（error）
	ErrCodeChatInProgress      = "E_CHAT_IN_PROGRESS"       // 该会话已有对话进行中，本轮拒绝（warning）
	ErrCodeChatEmptyPrompt     = "E_CHAT_EMPTY_PROMPT"      // 对话内容为空（warning）

	// RPC 协议层错误码（serve 浏览器模式 /api/rpc 传输协议错误，非业务域错误，
	// 不走 AppError 包装）。取值唯一来源为本表；server/rpc.go 以别名引用本表
	// 常量，前端 src/transport/rpc.js 按 code 透传、error.js ErrorCode 表按值
	// 同步（2026-09-22 浏览器 transport shim 落地时迁移，兑现 rpc.go 原注释
	// 「待前端 shim 落地时同步常量表」的迁移计划）。
	ErrCodeRPCBadRequest        = "E_RPC_BAD_REQUEST"        // 请求体非法（非 JSON、缺 method）或方法不允许（error）
	ErrCodeRPCMethodNotFound    = "E_RPC_METHOD_NOT_FOUND"   // 请求的方法不存在或未导出（error）
	ErrCodeRPCMethodUnsupported = "E_RPC_METHOD_UNSUPPORTED" // 方法签名形态不受支持，如变参方法（error）
	ErrCodeRPCArgCountMismatch  = "E_RPC_ARG_COUNT_MISMATCH" // 参数个数与方法签名不符（error）
	ErrCodeRPCArgTypeMismatch   = "E_RPC_ARG_TYPE_MISMATCH"  // 参数值无法转换为签名要求的类型（error）
	ErrCodeRPCInternal          = "E_RPC_INTERNAL"           // 非 AppError 的通用内部错误（error）
)
