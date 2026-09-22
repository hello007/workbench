/**
 * token 输入门（浏览器首访 401 时的极简输入浮层）。
 *
 * 触发时机：RPC 请求返回 401（rpc.js）或 WS 首次握手未成功/连续重连失败
 * 达阈值（events.js）。并发触发去重：模块级单 Promise，多个请求共享同一
 * 次输入结果；输入提交后经 token.js setToken 持久化再 resolve，等待方
 * （RPC 重试循环/WS 重连）各自以新令牌继续。
 *
 * 取消出口：ESC（焦点在输入框内经冒泡触发）或右上角关闭按钮可关闭浮层，
 * Promise 以 {code, message} 形态 reject（与 rpc.js reject 形态一致，业务
 * 组件 handleError 可直接分流）。取消后等待方自行收尾（RPC 终止本次请求
 * 报错、WS 停止重连循环），用户下一次任意 RPC 401 或 WS 建连尝试会重新
 * 弹门，不会永久锁死认证入口。
 *
 * UI 为 vanilla DOM（不依赖 Vue 实例），风格守 frontend-visual-conventions：
 * 卡片 --bg-secondary + --radius-lg + --shadow-lg，主操作 --primary-color，
 * 间距/圆角全部走设计令牌（样式见 dom.js 统一注入）。
 */

import { setToken } from './token'
import { ensureTransportStyle } from './dom'
import { ErrorCode } from '../utils/error'

// 进行中的输入门 Promise；null 表示当前无浮层。
let gatePromise = null

/**
 * 请求用户输入访问令牌。已有浮层时返回同一 Promise（并发去重）。
 * 提交非空令牌后 resolve 该令牌字符串（已持久化）；用户经 ESC 或关闭
 * 按钮取消时以 {code, message} reject。
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
 * 构建浮层 DOM 并等待用户提交或取消。内部实现，外部统一走 requestToken。
 * @returns {Promise<string>}
 */
function openGate() {
  return new Promise((resolve, reject) => {
    ensureTransportStyle()
    const backdrop = document.createElement('div')
    backdrop.className = 'wb-token-gate-backdrop'
    // 静态模板，不含任何外部数据，innerHTML 安全
    backdrop.innerHTML = `
      <div class="wb-token-gate">
        <button class="wb-token-gate-close" type="button" aria-label="关闭" title="关闭">&times;</button>
        <h3 class="wb-token-gate-title">访问令牌验证</h3>
        <p class="wb-token-gate-desc">请输入 WorkBench 服务端的访问令牌（见服务器 data/web_token 文件），验证通过后将记住本次输入。</p>
        <input class="wb-token-gate-input" type="password" placeholder="访问令牌" autocomplete="off" />
        <button class="wb-token-gate-submit" type="button" disabled>确认连接</button>
      </div>
    `
    document.body.appendChild(backdrop)

    const input = backdrop.querySelector('.wb-token-gate-input')
    const submit = backdrop.querySelector('.wb-token-gate-submit')
    const close = backdrop.querySelector('.wb-token-gate-close')

    // 统一收尾：移除浮层并注销键盘监听。ESC 监听挂在浮层节点（焦点在输入框内
    // 经冒泡到达），浮层移除后监听随之失效，不产生 document 级悬挂监听器
    const teardown = () => {
      backdrop.removeEventListener('keydown', onKeydown)
      backdrop.remove()
    }
    const onSubmit = () => {
      const value = input.value.trim()
      if (!value) return
      teardown()
      setToken(value)
      resolve(value)
    }
    const onCancel = () => {
      teardown()
      // code 取现有 RPC 协议层通用码：取消非业务域错误，经 rpc.js 透传后
      // 由业务组件 handleError 以 error 级提示，无需新增共享错误码
      reject({ code: ErrorCode.RpcInternal, message: '已取消访问令牌输入' })
    }
    const onKeydown = (event) => {
      if (event.key === 'Escape') onCancel()
    }
    submit.addEventListener('click', onSubmit)
    close.addEventListener('click', onCancel)
    backdrop.addEventListener('keydown', onKeydown)
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
