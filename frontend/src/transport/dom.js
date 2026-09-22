/**
 * transport 层 vanilla DOM 组件（token 输入门、连接状态横幅）的样式注入。
 *
 * transport 模块不依赖 Vue 组件树（token 门须在 app.mount 前后的任意时机可用），
 * 样式以 <style> 标签一次性注入 document.head。取值全部走 style.css 设计令牌
 * 变量（语义色/间距/圆角/阴影），与 frontend-visual-conventions 保持一致；
 * 遮罩按规范用蓝灰着色（亮 rgba(15,23,42,…) / 暗 rgba(2,6,23,…)），禁纯黑。
 */

const STYLE_ID = 'wb-transport-style'

const TRANSPORT_CSS = `
.wb-token-gate-backdrop {
  position: fixed;
  inset: 0;
  z-index: 4000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.45);
}
html.dark .wb-token-gate-backdrop {
  background: rgba(2, 6, 23, 0.6);
}
.wb-token-gate {
  position: relative;
  width: min(420px, 86vw);
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  padding: var(--spacing-lg);
}
.wb-token-gate-close {
  position: absolute;
  top: var(--spacing-sm);
  right: var(--spacing-sm);
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  line-height: 1;
  color: var(--text-secondary);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}
.wb-token-gate-close:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
}
.wb-token-gate-title {
  margin: 0 0 var(--spacing-sm);
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}
.wb-token-gate-desc {
  margin: 0 0 var(--spacing-md);
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-secondary);
}
.wb-token-gate-input {
  width: 100%;
  box-sizing: border-box;
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 14px;
  color: var(--text-primary);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  outline: none;
}
.wb-token-gate-input:focus {
  border-color: var(--primary-light);
}
.wb-token-gate-submit {
  display: block;
  width: 100%;
  margin-top: var(--spacing-md);
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 14px;
  color: #ffffff;
  background: var(--primary-color);
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--transition-fast);
}
.wb-token-gate-submit:hover:not(:disabled) {
  background: var(--primary-dark);
}
.wb-token-gate-submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.wb-conn-banner {
  position: fixed;
  top: var(--spacing-md);
  left: 50%;
  transform: translateX(-50%);
  z-index: 3000;
  padding: var(--spacing-sm) var(--spacing-md);
  font-size: 13px;
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
}
.wb-conn-banner--warning {
  color: var(--warning-color);
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--warning-color) 35%, transparent);
}
.wb-conn-banner--success {
  color: var(--success-color);
  background: color-mix(in srgb, var(--success-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--success-color) 35%, transparent);
}
`

/**
 * 确保 transport 层样式已注入（幂等，重复调用不重复插入）。
 */
export function ensureTransportStyle() {
  if (document.getElementById(STYLE_ID)) return
  const style = document.createElement('style')
  style.id = STYLE_ID
  style.textContent = TRANSPORT_CSS
  document.head.appendChild(style)
}
