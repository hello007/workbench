/**
 * token 输入门（浏览器首访 401 时的极简输入浮层）。
 *
 * 触发时机：RPC 请求返回 401（rpc.js）或 WS 首次握手未成功（events.js）。
 * 并发触发去重：模块级单 Promise，多个请求共享同一次输入结果；输入提交后
 * 经 token.js setToken 持久化再 resolve，等待方（RPC 重试循环/WS 重连）
 * 各自以新令牌继续。
 *
 * UI 为 vanilla DOM（不依赖 Vue 实例），风格守 frontend-visual-conventions：
 * 卡片 --bg-secondary + --radius-lg + --shadow-lg，主操作 --primary-color，
 * 间距/圆角全部走设计令牌（样式见 dom.js 统一注入）。
 */

import { setToken } from './token'
import { ensureTransportStyle } from './dom'

// 进行中的输入门 Promise；null 表示当前无浮层。
let gatePromise = null

/**
 * 请求用户输入访问令牌。已有浮层时返回同一 Promise（并发去重）。
 * 提交非空令牌后 resolve 该令牌字符串（已持久化）；当前无取消路径——
 * 浏览器模式下无有效令牌则全部功能不可用，输入是唯一出路。
 * @returns {Promise<string>}
 */
export function requestToken() {
  if (!gatePromise) {
    gatePromise = openGate().finally(() => {
      gatePromise = null
    })
  }
  return gatePromise
}

/**
 * 构建浮层 DOM 并等待用户提交。内部实现，外部统一走 requestToken。
 * @returns {Promise<string>}
 */
function openGate() {
  return new Promise((resolve) => {
    ensureTransportStyle()
    const backdrop = document.createElement('div')
    backdrop.className = 'wb-token-gate-backdrop'
    // 静态模板，不含任何外部数据，innerHTML 安全
    backdrop.innerHTML = `
      <div class="wb-token-gate">
        <h3 class="wb-token-gate-title">访问令牌验证</h3>
        <p class="wb-token-gate-desc">请输入 WorkBench 服务端的访问令牌（见服务器 data/web_token 文件），验证通过后将记住本次输入。</p>
        <input class="wb-token-gate-input" type="password" placeholder="访问令牌" autocomplete="off" />
        <button class="wb-token-gate-submit" type="button" disabled>确认连接</button>
      </div>
    `
    document.body.appendChild(backdrop)

    const input = backdrop.querySelector('.wb-token-gate-input')
    const submit = backdrop.querySelector('.wb-token-gate-submit')

    const onSubmit = () => {
      const value = input.value.trim()
      if (!value) return
      setToken(value)
      backdrop.remove()
      resolve(value)
    }
    submit.addEventListener('click', onSubmit)
    input.addEventListener('keydown', (event) => {
      if (event.key === 'Enter') onSubmit()
    })
    // 空输入禁止提交，避免空令牌发起必然失败的请求
    input.addEventListener('input', () => {
      submit.disabled = input.value.trim() === ''
    })
    input.focus()
  })
}
