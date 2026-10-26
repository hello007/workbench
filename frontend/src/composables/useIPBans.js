/**
 * 来源 IP 自动封禁 composable（连接管控五期，桌面专用消费面）。
 *
 * 后端链路：认证失败达阈值（滑动窗口 10 分钟内 10 次）由 service.IPBanService
 * 自动封禁来源 IP（5 分钟短时限），经 web_serve.go 装配的 SetOnBanned 回调以
 * Wails 出口发射桌面专用事件 `webserve-ip-banned`（对齐 webserve-connections:changed
 * 同义不同名先例）。**隐私红线**：他人 IP 明细仅走桌面出口——该事件浏览器通道
 * 永不到达（serve 无头模式 wailsEventSink 对非 Wails ctx 静默跳过），ListIPBans
 * 亦被后端 RPCHiddenMethods 对浏览器通道屏蔽（呈现「方法不存在」）；本文件
 * ListIPBans 加载面经 isBrowserChannel() 启动期判定直接跳过（隐藏入口而非点击
 * 报错，对齐审计历史入口先例），浏览器端 ipBans 恒空、自动封禁区块只显空态。
 *
 * 消费形态两支（共用同一事件名常量单点）：
 *  - useIPBanList()：ActivityBar 明细面板「自动封禁」区块数据源（列表 + 事件驱动
 *    刷新 + 手动 reload）；
 *  - useIPBanToast()：App 级一次性挂载的全局 toast（面板未开也提示管理员）。
 *
 * 监听注销遵守 cross-layer-contracts 契约：EventsOn 返回闭包在 onBeforeUnmount
 * 精准移除本组件监听器（禁 EventsOff 全局移除）。
 */

import { ref, onMounted, onBeforeUnmount } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { ListIPBans } from '../../wailsjs/go/main/App'
import { ElMessage } from 'element-plus'
import { isBrowserChannel } from '../transport'

// 桌面端 IP 自动封禁触发事件名（web_serve.go webServeIPBannedEvent，取值须与后端一致）。
// 载荷为 model.IPBanStatus 快照（{ip, bannedAt, expiresAt, failCount, strike}，json
// 小写 tag，含 expired 字段但触发时刻恒 false——列表过期态以 ListIPBans 懒判定为准）。
export const WEBSERVE_IP_BANNED_EVENT = 'webserve-ip-banned'

/**
 * 封禁到期时间格式化（unix 秒 → "M-D HH:mm"）：面板行与 toast 共用；非法值
 * （0/undefined/NaN）返回 '--'。
 * @param {number} sec unix 秒
 * @returns {string}
 */
export function formatBanExpiry(sec) {
  const t = new Date(sec * 1000)
  if (!sec || Number.isNaN(t.getTime())) {
    return '--'
  }
  const pad = (n) => String(n).padStart(2, '0')
  return `${t.getMonth() + 1}-${t.getDate()} ${pad(t.getHours())}:${pad(t.getMinutes())}`
}

/**
 * 自动封禁列表数据源（ActivityBar 明细面板「自动封禁」区块消费）。
 *
 * 加载时机三处：onMounted 首拉（指示条件扩展的数据源——被封禁 IP 无法建立连接，
 * 仅凭连接数判定面板将不可达，须在无连接态也能感知封禁记录）；外部调 reload()
 * （面板 @show 打开时与解封成功后）；`webserve-ip-banned` 事件到达时自动 reload。
 *
 * 失败降级静默：浏览器通道经 isBrowserChannel() 跳过加载（后端 RPC 面屏蔽，
 * 调用必败）；其余失败（未装配/停机态）吞错保持空列表——封禁列表为增强展示面，
 * 静默空态优于报错噪声。
 *
 * @returns {{
 *   ipBans: import('vue').Ref<Array<{ip:string, bannedAt:number, expiresAt:number, failCount:number, strike:number, expired:boolean}>>,
 *   reload: () => Promise<void>
 * }}
 */
export function useIPBanList() {
  const ipBans = ref([])
  let offBanned = null

  async function reload() {
    if (isBrowserChannel()) {
      return
    }
    try {
      const list = await ListIPBans()
      ipBans.value = Array.isArray(list) ? list : []
    } catch {
      // 加载失败保持上次值（首拉即失败为空列表空态），不弹错——展示面非正确性依赖
    }
  }

  onMounted(() => {
    offBanned = EventsOn(WEBSERVE_IP_BANNED_EVENT, () => {
      // 事件载荷为单条新封禁快照，直接整表重拉（列表量级小，免增量合并双口径）
      reload()
    })
    reload()
  })

  onBeforeUnmount(() => {
    if (offBanned) {
      offBanned()
      offBanned = null
    }
  })

  return { ipBans, reload }
}

/**
 * 自动封禁全局 toast（App.vue 根级一次性挂载，面板未开也提示管理员）。
 *
 * 收到 `webserve-ip-banned` 即弹 warning 提示（含来源 IP 与到期时间）；载荷
 * 防御性校验（ip 非空字符串 + expiresAt 为有限数字），非法载荷忽略——桌面事件
 * 浏览器端永不到达，本 composable 在浏览器通道天然静默。
 */
export function useIPBanToast() {
  let offBanned = null

  onMounted(() => {
    offBanned = EventsOn(WEBSERVE_IP_BANNED_EVENT, (status) => {
      if (!status || typeof status.ip !== 'string' || !status.ip) {
        return
      }
      if (!Number.isFinite(status.expiresAt)) {
        return
      }
      ElMessage.warning(`来源 ${status.ip} 认证失败过多，已临时封禁至 ${formatBanExpiry(status.expiresAt)}`)
    })
  })

  onBeforeUnmount(() => {
    if (offBanned) {
      offBanned()
      offBanned = null
    }
  })
}
