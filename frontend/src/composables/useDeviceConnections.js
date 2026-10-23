/**
 * 多设备连接感知 composable（双通道事件驱动）。
 *
 * 消费两个后端连接事件，暴露 deviceCount ref（「N 个设备已连接」提示与终端面板
 * 多端写入提示）与 connections ref（连接明细列表，桌面明细面板消费）。
 *
 * 通道语义（hub 计数口径 = 浏览器 WS 客户端数，两视角判定刻意分叉）：
 *  - 浏览器（serve）通道 `ws-connections:changed`（server/ws.go wsConnectionsEvent）：
 *    载荷恒为 count 数字——**隐私红线**：他人 IP/token 明细不进浏览器通道，载荷
 *    逐字节不变；事件经 transport events.js 广播分发（含 resume 补发，计数为幂等
 *    快照，最后到达值即当前态）；计数含自身连接，指示条件为 count>1（有其他设备
 *    接入）。浏览器端无明细数据，connections 恒空（明细面板显示引导文案）。
 *  - 桌面通道 `webserve-connections:changed`（web_serve.go webServeConnectionsEvent，
 *    hub SetOnConnectionsChange 回调经 Wails 出口发射）：载荷为连接明细快照数组
 *    （{id, ip, tokenId, tokenName, connectedAt}，TokenName 已由装配层经令牌名册
 *    映射补全，令牌已撤销的连接该字段为空串）；桌面即共享发起方，明细为纯浏览器
 *    连接列表，指示条件为 len>=1（第 1 个浏览器连接即显示），且须接受空数组
 *    （最后一个浏览器断开 / webServe 停机熄指示并清空明细）。桌面事件浏览器端
 *    永不到达，浏览器通道逐字节不变，无需模式探测旗标——任一通道首个事件到达
 *    即锁定通道归属（isDesktopChannel）。
 *
 * 明细粒度为逐连接快照（D1）：每行一个 WS 连接（同 IP 多标签页多行，接入时间
 * 区分），deviceCount = len(明细)（桌面通道），重连即新条目不跨会话去重。
 *
 * 监听注销遵守 cross-layer-contracts 契约：EventsOn 返回闭包在 onBeforeUnmount
 * 精准移除本组件监听器（多组件共听同事件，禁 EventsOff 全局移除）。
 */

import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// 后端 WSHub 连接数变化事件名（server/ws.go wsConnectionsEvent，取值须与后端一致）。
export const WS_CONNECTIONS_EVENT = 'ws-connections:changed'

// 桌面端浏览器连接明细变化事件名（web_serve.go webServeConnectionsEvent，取值须与后端一致）。
export const WEBSERVE_CONNECTIONS_EVENT = 'webserve-connections:changed'

/**
 * 订阅连接变化事件（浏览器/桌面双通道）。
 * @returns {{
 *   deviceCount: import('vue').Ref<number>,
 *   multiDevice: import('vue').ComputedRef<boolean>,
 *   connections: import('vue').Ref<Array<{id:number, ip:string, tokenId:string, tokenName:string, connectedAt:string}>>,
 *   isDesktopChannel: import('vue').Ref<boolean>
 * }}
 *   deviceCount 当前已连接设备数（浏览器视角含本连接，桌面视角 = len(明细)）；
 *   multiDevice 是否显示多设备指示（桌面通道 len>=1 / 浏览器通道 count>1）；
 *   connections 连接明细快照（仅桌面通道有数据，浏览器端恒空——隐私红线）；
 *   isDesktopChannel 通道归属（桌面事件到达即锁定）。
 */
export function useDeviceConnections() {
  // 初始 1 = 仅本连接（桌面通道首个事件到达前，两端均按「无其他设备」处理）
  const browserCount = ref(1)
  // 桌面通道明细快照（后端全量数组原样落地；浏览器端恒空）
  const connections = ref([])
  // 桌面通道归属：桌面事件到达即置位（其后判定走桌面口径），未到达走浏览器口径
  const isDesktopChannel = ref(false)
  // deviceCount 分流派生：桌面 = len(明细)（与后端 count 口径严格一致），浏览器 = count
  const deviceCount = computed(() =>
    isDesktopChannel.value ? connections.value.length : browserCount.value
  )
  const multiDevice = computed(() =>
    isDesktopChannel.value ? deviceCount.value >= 1 : deviceCount.value > 1
  )

  let offChanged = null
  let offDesktop = null

  onMounted(() => {
    offChanged = EventsOn(WS_CONNECTIONS_EVENT, (count) => {
      const n = Number(count)
      // 防御非法载荷（老服务端/异常帧）：仅接受正整数，忽略后保持上次值
      if (Number.isFinite(n) && n >= 1) {
        browserCount.value = n
      }
    })
    offDesktop = EventsOn(WEBSERVE_CONNECTIONS_EVENT, (details) => {
      // 载荷须为明细数组（空数组合法 = 无浏览器连接，熄指示并清空列表）；非数组
      // 载荷忽略且不锁定通道归属（对齐既有非法载荷防御模式）
      if (!Array.isArray(details)) {
        return
      }
      isDesktopChannel.value = true
      connections.value = details
    })
  })

  onBeforeUnmount(() => {
    if (offChanged) {
      offChanged()
      offChanged = null
    }
    if (offDesktop) {
      offDesktop()
      offDesktop = null
    }
  })

  return { deviceCount, multiDevice, connections, isDesktopChannel }
}
