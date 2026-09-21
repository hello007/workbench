/**
 * 浏览器模式 RPC 通道：把 window.go.main.App 的任意绑定方法调用翻译为
 * POST /api/rpc {method, args}（后端反射路由到 App 导出方法，见 server/rpc.go）。
 *
 * 应答映射（与桌面模式 Wails ErrorFormatter 的 reject 形态对齐）：
 *   - HTTP 200 + {ok:true, data}            → resolve(data)
 *   - HTTP 200 + {ok:false, error:{code,message}} → reject({code, message})
 *     （业务失败后端走 HTTP 200 应答，与桌面 reject 语义一致，前端 handleError
 *      按 code 分流，错误码见 utils/error.js ErrorCode 表）
 *   - HTTP 401                              → 弹 token 输入门，取得新令牌后重试
 *   - 其他 HTTP 拒绝（如 405，后端 writeRPCError 仍返回 {ok,error} 结构）
 *                                           → reject({code, message})
 *   - 网络失败/响应非法                      → reject({code: E_RPC_INTERNAL, ...})
 *
 * 桌面模式 reject 的是普通对象 {code?, message}（main.go formatAppError 返回
 * map 序列化产物），本模块同样 reject 普通对象，业务组件 handleError 零改动。
 */

import { getToken } from './token'
import { requestToken } from './tokenGate'
import { ErrorCode } from '../utils/error'

// 与 server/rpc.go rpcPath 一致。
const RPC_PATH = '/api/rpc'

/**
 * 构造带令牌的请求头。令牌未设置时省略 Authorization（由服务端 401 触发输入门）。
 * @returns {Record<string, string>}
 */
function buildAuthHeaders() {
  const token = getToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

/**
 * 执行一次 RPC 调用，401 时经 token 门取得新令牌后自动重试（循环而非递归，
 * 每轮都等待用户输入，无热循环风险）。
 * @param {string} method 绑定方法名（App 导出方法名）
 * @param {unknown[]} args 位置参数数组
 * @returns {Promise<unknown>} 后端返回的 data
 */
export async function invokeRpc(method, args) {
  for (;;) {
    let response
    try {
      response = await fetch(RPC_PATH, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...buildAuthHeaders() },
        body: JSON.stringify({ method, args })
      })
    } catch (err) {
      throw { code: ErrorCode.RpcInternal, message: `网络请求失败: ${err?.message ?? err}` }
    }
    if (response.status === 401) {
      // 门内提交时已持久化令牌，下一轮读取新值重试
      await requestToken()
      continue
    }
    return await parseRpcResponse(response)
  }
}

/**
 * 解析非 401 的 HTTP 应答并映射为 resolve(data) / reject({code, message})。
 * @param {Response} response
 * @returns {Promise<unknown>}
 */
async function parseRpcResponse(response) {
  let body = null
  try {
    body = await response.json()
  } catch {
    body = null
  }
  if (!response.ok) {
    // 协议层 HTTP 拒绝（如 405）：后端 writeRPCError 返回 {ok,error:{code,message}}；
    // 兜底兼容 {error:"文本"}（writePreviewError 形态）与无 body 场景
    const code = body?.error?.code ?? ErrorCode.RpcInternal
    const message = body?.error?.message ?? (typeof body?.error === 'string' ? body.error : `请求失败 (HTTP ${response.status})`)
    throw { code, message }
  }
  if (body?.ok === true) {
    return body.data
  }
  if (body?.ok === false) {
    throw {
      code: body.error?.code ?? ErrorCode.RpcInternal,
      message: body.error?.message ?? '未知错误'
    }
  }
  throw { code: ErrorCode.RpcInternal, message: '响应格式异常' }
}

/**
 * 构造 window.go.main.App 的 Proxy：任意属性访问返回「调用即发起 RPC」的
 * 函数，与 wailsjs 生成 wrapper 的调用形态（window.go.main.App[方法](args)）
 * 完全兼容。Symbol 属性（Symbol.toPrimitive 等）返回 undefined 不参与调用。
 * @returns {Proxy<object>}
 */
export function createRpcProxy() {
  return new Proxy({}, {
    get(_target, prop) {
      if (typeof prop !== 'string') return undefined
      return (...args) => invokeRpc(prop, args)
    }
  })
}
