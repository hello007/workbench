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
	ErrCodeGitCancelled       = "E_GIT_CANCELLED"         // 操作被用户取消（warning，中性提示非 error）
	ErrCodeGitLocked          = "E_GIT_LOCKED"            // index.lock 被占用或残留，写操作被拒（warning，引导人工确认后删除，禁自动删）
	ErrCodeGitTimeout         = "E_GIT_TIMEOUT"           // Git 操作超时被终止（warning，用户可重试；超时不得误判为冲突正常返回）

	// 内容搜索域
	ErrCodeSearchTimeout = "E_SEARCH_TIMEOUT" // 内容搜索超时被终止，结果不完整或为空（warning，用户可缩小范围后重试；超时不得误判为无匹配）

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

	// 进程管理域（工具箱·进程管理）
	ErrCodeProcessEnumFailed   = "E_PROCESS_ENUM_FAILED"   // 进程枚举命令执行失败（error）
	ErrCodeProcessEnumTimeout  = "E_PROCESS_ENUM_TIMEOUT"  // 进程枚举超时被终止，可重试（warning；须先判 ctx.Err() 再判命令退出码，不得误判为「无进程」）
	ErrCodeProcessNotFound     = "E_PROCESS_NOT_FOUND"     // 目标进程已不存在，列表已过期（warning）
	ErrCodeProcessAccessDenied = "E_PROCESS_ACCESS_DENIED" // 权限不足或受保护进程（PPL），引导以管理员身份运行（warning）
	ErrCodeProcessSelfProtect  = "E_PROCESS_SELF_PROTECT"  // 拒绝终止本应用自身或父进程链（warning）
	ErrCodeProcessKillFailed   = "E_PROCESS_KILL_FAILED"   // 终止进程失败（error）

	// 环境变量查看域（工具箱·环境变量）
	ErrCodeEnvEnumFailed = "E_ENV_ENUM_FAILED" // 环境变量枚举失败（error，引导重试；单来源失败不进此错误——走分组 Error 字段下发，仅全部来源失败时返回）

	// hosts 文件查看域（工具箱·hosts 文件）
	// 读取失败不整体外抛：错误消息随 model.HostsView.Error 组级字段下发（对齐
	// EnvVarGroup.Error 单来源失败形态），错误码不经 ErrorFormatter 到前端，
	// 故不同步 frontend/src/utils/error.js（该表仅收「前端按 code 分流」的错误码）。
	ErrCodeHostsReadFailed = "E_HOSTS_READ_FAILED" // hosts 文件读取失败（不存在 / 无权限等，用户可读消息按根因分类）

	// 素材管理域（侧边栏「素材管理」：本地 DAM 素材库 + PPT 单页索引）
	ErrCodeAssetsCancelled         = "E_ASSETS_CANCELLED"          // 素材扫描被用户取消（warning，中性提示非 error；对齐 E_GIT_CANCELLED 模式）
	ErrCodeAssetsLibNotFound       = "E_ASSETS_LIB_NOT_FOUND"      // 素材库不存在或已删除，列表已过期（error，前端刷新库列表）
	ErrCodeAssetsScanInProgress    = "E_ASSETS_SCAN_IN_PROGRESS"   // 该素材库已有扫描进行中，本次拒绝（warning，用户可稍后重试）
	ErrCodeAssetsThumbsUnavailable = "E_ASSETS_THUMBS_UNAVAILABLE" // 本机无 PowerPoint COM，缩略图/打开跳转不可用（error，前端提示条常驻 + 跳转入口隐藏）
	ErrCodeAssetsThumbsBusy        = "E_ASSETS_THUMBS_BUSY"        // 缩略图跳转请求排队/执行超时（warning，COM 队列瞬态忙，可重试；禁复用 UNAVAILABLE——语义为环境无 PowerPoint，前端会误触降级 UI）

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
