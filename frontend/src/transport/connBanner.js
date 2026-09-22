/**
 * 连接状态横幅（浏览器模式下 WS 断连/恢复的最小 UI 提示）。
 *
 * 由 events.js 在 WS onopen/onclose 时调用 notifyConnected/notifyDisconnected，
 * 连续重连失败达阈值或用户取消重连时调用 notifyUnreachable。
 * 仅在「曾成功连接过」后才提示断连：首访令牌门之前的连不上属正常初始化路径，
 * 由 token 输入门负责引导，不弹断连横幅造成双重打扰。
 *
 * vanilla DOM 实现，样式走设计令牌（dom.js 统一注入）；断连横幅常驻直至
 * 恢复，恢复提示展示数秒后自动消失。
 */

// 恢复提示自动消失时长。
const RECOVER_HINT_MS = 3000

let bannerEl = null
let everConnected = false
let hideTimer = null

/**
 * WS 连接成功（含重连成功）时调用。
 */
export function notifyConnected() {
  const wasDisconnected = everConnected && bannerEl !== null
  everConnected = true
  if (wasDisconnected) {
    show('连接已恢复', 'success')
    scheduleHide()
  } else {
    hide()
  }
}

/**
 * WS 连接断开时调用。首次成功连接前的断开不提示（初始化路径由 token 门覆盖）。
 */
export function notifyDisconnected() {
  if (!everConnected) return
  show('与后端连接已断开，正在自动重连…', 'warning')
}

/**
 * 服务不可达时调用（连续重连失败达阈值、或用户取消 token 门重连收尾，
 * 见 events.js）。与断连横幅同语义：常驻直至重连成功——曾连接过时由
 * notifyConnected 收尾显示「连接已恢复」，未连接过时静默移除不误报恢复。
 */
export function notifyUnreachable() {
  show('服务不可达，请确认 WorkBench 正在运行', 'warning')
}

/**
 * 展示横幅（同类提示复用同一 DOM 节点）。
 * @param {string} text 提示文案
 * @param {'warning'|'success'} kind 语义色档位
 */
function show(text, kind) {
  ensureBanner()
  clearTimeout(hideTimer)
  hideTimer = null
  bannerEl.textContent = text
  bannerEl.className = `wb-conn-banner wb-conn-banner--${kind}`
}

/**
 * 移除横幅 DOM。
 */
function hide() {
  clearTimeout(hideTimer)
  hideTimer = null
  if (bannerEl) {
    bannerEl.remove()
    bannerEl = null
  }
}

/**
 * 延时移除恢复提示。
 */
function scheduleHide() {
  hideTimer = setTimeout(hide, RECOVER_HINT_MS)
}

/**
 * 确保横幅节点存在并挂载到 body。
 */
function ensureBanner() {
  if (bannerEl) return
  bannerEl = document.createElement('div')
  document.body.appendChild(bannerEl)
}
