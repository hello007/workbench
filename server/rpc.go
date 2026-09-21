package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"

	"workbench/model"
)

// RPC 协议层错误码（传输协议错误，非业务域错误）。
//
// 响应形态与 model.AppError 经 Wails ErrorFormatter 传给前端的 {code, message}
// 对象一致，前端按 code 分流；浏览器 transport shim 属后续 PR，当前尚无前端
// 消费方，故暂不进 model/app_error.go 业务常量表，待前端 shim 落地时随
// handleError 分流映射一并同步（见 docs/spec/logging-and-errors.md）。
const (
	// ErrCodeRPCBadRequest 请求体非法（非 JSON、缺 method 字段）或方法不允许
	ErrCodeRPCBadRequest = "E_RPC_BAD_REQUEST"
	// ErrCodeRPCMethodNotFound 请求的方法不存在或未导出
	ErrCodeRPCMethodNotFound = "E_RPC_METHOD_NOT_FOUND"
	// ErrCodeRPCMethodUnsupported 方法签名形态不受支持（如变参方法）
	ErrCodeRPCMethodUnsupported = "E_RPC_METHOD_UNSUPPORTED"
	// ErrCodeRPCArgCountMismatch 参数个数与方法签名不符
	ErrCodeRPCArgCountMismatch = "E_RPC_ARG_COUNT_MISMATCH"
	// ErrCodeRPCArgTypeMismatch 参数值无法转换为签名要求的类型
	ErrCodeRPCArgTypeMismatch = "E_RPC_ARG_TYPE_MISMATCH"
	// ErrCodeRPCInternal 非 AppError 的通用内部错误（业务方法返回的普通 error）
	ErrCodeRPCInternal = "E_RPC_INTERNAL"
)

// rpcPath 通用 RPC 翻译层端点路径。
const rpcPath = "/api/rpc"

// maxRPCBodyBytes /api/rpc 请求体上限（32MB）。写方向最大合法业务负载为
// SaveFile 的 1MB 内容限制（service/fileoperation.go），32MB 留数量级余量，
// 防异常客户端超大 body 反序列化耗尽内存。
const maxRPCBodyBytes = 32 << 20

// rpcRequest /api/rpc 请求体。
//
// Method 为 Go 导出方法名，与 frontend/wailsjs/go/main/App.js 生成的绑定方法名
// 一致（即 App 及其提升自 AppServices 的导出方法）；Args 为按位置排列的 JSON
// 参数数组，与绑定方法的参数顺序一一对应。
type rpcRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

// rpcResponse /api/rpc 统一响应体。
//
// 成功：{"ok":true,"data":<结果>}；业务失败：HTTP 200 + {"ok":false,"error":...}。
// 业务失败不走 HTTP 错误状态码——调用到达业务层后的失败属于正常协议应答，
// 前端按 body.ok / error.code 统一分流，与桌面模式经 ErrorFormatter 收到
// {code, message} 对象后走 reject 的处理路径语义对齐。
type rpcResponse struct {
	OK    bool          `json:"ok"`
	Data  any           `json:"data"`
	Error *rpcErrorBody `json:"error,omitempty"`
}

// rpcErrorBody 错误详情。Code 为机器可读错误码，Message 为用户可读消息。
type rpcErrorBody struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// rpcErrorType reflect.Type 形式的 error 接口类型，用于识别方法返回值中的 error 位。
var rpcErrorType = reflect.TypeOf((*error)(nil)).Elem()

// rpcProtocolError RPC 协议层错误（方法不存在/参数个数或类型不匹配等请求侧
// 错误），携带专用错误码，应答时优先于 AppError 与通用内部错误码识别。
type rpcProtocolError struct {
	code    string
	message string
}

func (e *rpcProtocolError) Error() string { return e.message }

// newRPCError 构造协议层错误。
func newRPCError(code, format string, args ...any) *rpcProtocolError {
	return &rpcProtocolError{code: code, message: fmt.Sprintf(format, args...)}
}

// NewRPCHandler 构建通用 RPC 翻译层 handler：把 {method, args} 请求经反射路由
// 到 target 的导出方法并回传 JSON 结果。
//
// target 为 App 实例（以 any + reflect 路由，避免 server 包反向依赖 main 包）。
//
// 方法白名单策略 = target 全部导出方法，与桌面 webview 的 Bind 暴露面完全一致
// （桌面模式前端本可调用任意绑定方法，无方法级二次鉴权）；本层不做方法过滤，
// 未授权不可达由外层 token 认证中间件统一把关——信任模型与桌面一致。
func NewRPCHandler(target any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRPC(w, r, target)
	})
}

// handleRPC 处理单次 RPC 调用：解析请求 → 反射调用 → 按统一结构应答。
//
// 仅接受 POST + JSON 请求体（静态资产与预览路由各自只读，RPC 是唯一的
// 写入口，收紧方法可降低误用面）。
func handleRPC(w http.ResponseWriter, r *http.Request, target any) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeRPCError(w, http.StatusMethodNotAllowed, ErrCodeRPCBadRequest, "仅支持 POST 请求")
		return
	}

	var req rpcRequest
	// 请求体上限防护：超出 maxRPCBodyBytes 时 Decode 报 *http.MaxBytesError，
	// 给出区别于普通非法 JSON 的可读错误
	r.Body = http.MaxBytesReader(w, r.Body, maxRPCBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeRPCResult(w, rpcFailure(ErrCodeRPCBadRequest, fmt.Sprintf("请求体超过大小上限 %d 字节", maxErr.Limit)))
			return
		}
		writeRPCResult(w, rpcFailure(ErrCodeRPCBadRequest, "请求体不是合法 JSON: "+err.Error()))
		return
	}
	if req.Method == "" {
		writeRPCResult(w, rpcFailure(ErrCodeRPCBadRequest, "缺少 method 字段"))
		return
	}

	data, callErr := invokeRPC(target, req.Method, req.Args)
	if callErr != nil {
		// 只记方法名与错误，不记参数：参数可能含文件路径等敏感内容
		slog.Warn("rpc call failed", "method", req.Method, "err", callErr)
		writeRPCResult(w, rpcFailureFromCall(callErr))
		return
	}
	writeRPCResult(w, rpcResponse{OK: true, Data: data})
}

// invokeRPC 按方法名反射查找 target 的导出方法，转换参数后调用，返回
// 非 error 的返回值（data）与 error 返回值。
//
// 参数转换：每个 JSON 参数按方法签名逐位反序列化（encoding/json 自带数字
// 边界检查——浮点转整型、整型溢出均报错），支持 string/bool/数字（含
// uint16/uint 等终端 resize 参数）/struct/slice/指针等 JSON 可表达类型。
func invokeRPC(target any, method string, rawArgs []json.RawMessage) (data any, err error) {
	if target == nil {
		return nil, errors.New("RPC 目标未装配")
	}
	m := reflect.ValueOf(target).MethodByName(method)
	if !m.IsValid() {
		// MethodByName 仅能命中导出方法，未导出方法同样落此分支
		return nil, newRPCError(ErrCodeRPCMethodNotFound, "方法不存在: %s", method)
	}
	mt := m.Type()
	if mt.IsVariadic() {
		return nil, newRPCError(ErrCodeRPCMethodUnsupported, "不支持变参方法: %s", method)
	}
	if len(rawArgs) != mt.NumIn() {
		return nil, newRPCError(ErrCodeRPCArgCountMismatch, "参数个数不匹配: 方法 %s 需要 %d 个参数，请求提供 %d 个", method, mt.NumIn(), len(rawArgs))
	}

	in := make([]reflect.Value, len(rawArgs))
	for i, raw := range rawArgs {
		ptr := reflect.New(mt.In(i))
		if uerr := json.Unmarshal(raw, ptr.Interface()); uerr != nil {
			return nil, newRPCError(ErrCodeRPCArgTypeMismatch, "第 %d 个参数类型不匹配: %v", i+1, uerr)
		}
		in[i] = ptr.Elem()
	}

	results := m.Call(in)

	var callErr error
	for i, rv := range results {
		if mt.Out(i) == rpcErrorType {
			if !rv.IsNil() {
				callErr = rv.Interface().(error)
			}
			continue
		}
		data = rv.Interface()
	}
	if callErr != nil {
		return nil, callErr
	}
	return data, nil
}

// rpcFailure 构造指定错误码的失败响应。
func rpcFailure(code, message string) rpcResponse {
	return rpcResponse{OK: false, Error: &rpcErrorBody{Code: code, Message: message}}
}

// rpcFailureFromCall 把调用侧错误转为响应体，识别优先级：
//  1. rpcProtocolError（请求侧协议错误：方法不存在/参数不匹配等）→ 专用错误码；
//  2. AppError 透传 {code, message}（与桌面 ErrorFormatter 形态一致，前端按
//     code 分流提示级别，见 docs/spec/logging-and-errors.md）；
//  3. 其他错误包装为通用内部错误码，仅透出错误文本。
func rpcFailureFromCall(err error) rpcResponse {
	var protoErr *rpcProtocolError
	if errors.As(err, &protoErr) {
		return rpcFailure(protoErr.code, protoErr.message)
	}
	var appErr *model.AppError
	if errors.As(err, &appErr) {
		return rpcFailure(appErr.Code, appErr.Message)
	}
	return rpcFailure(ErrCodeRPCInternal, err.Error())
}

// writeRPCResult 以 HTTP 200 输出 RPC 统一响应体。
func writeRPCResult(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// writeRPCError 以指定 HTTP 状态码输出失败响应（仅协议层拒绝使用，如 405）。
func writeRPCError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(rpcFailure(code, message))
}
