/**
 * 多设备连接数感知 composable（双通道事件驱动，D1 决策：架构零改动只提示）。
 *
 * 消费两个后端连接数事件，暴露 deviceCount ref 供 UI 呈现「N 个设备已连接」
 * 提示与终端面板多端写入提示。
 *
 * 通道语义（hub 计数口径 = 浏览器 WS 客户端数，两视角判定刻意分叉）：
 *  - 浏览器（serve）通道 `ws-connections:changed`（server/ws.go wsConnectionsEvent）：
 *    事件经 transport events.js 广播分发（含 resume 补发，计数为幂等快照，最后
 *    到达值即当前态）；计数含自身连接，指示条件为 count>1（有其他设备接入）。
 *  - 桌面通道 `webserve-connections:changed`（web_serve.go webServeConnectionsEvent，
 *    hub SetOnCountChange 回调经 Wails 出口发射）：桌面即共享发起方，计数为纯
 *    浏览器连接数，指示条件为 count>=1（第 1 个浏览器连接即显示），且须接受
 *    count=0（最后一个浏览器断开 / webServe 停机熄指示）。桌面事件浏览器端
 *    永不到达，浏览器通道逐字节不变，无需模式探测旗标——任一通道首个事件到达
 *    即锁定通道归属（isDesktopChannel）。
 *
 * 监听注销遵守 cross-layer-contracts 契约：EventsOn 返回闭包在 onBeforeUnmount
 * 精准移除本组件监听器（多组件共听同事件，禁 EventsOff 全局移除）。
 */

import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// 后端 WSHub 连接数变化事件名（server/ws.go wsConnectionsEvent，取值须与后端一致）。
export const WS_CONNECTIONS_EVENT = 'ws-connections:changed'

// 桌面端浏览器连接数变化事件名（web_serve.go webServeConnectionsEvent，取值须与后端一致）。
export const WEBSERVE_CONNECTIONS_EVENT = 'webserve-connections:changed'

/**
 * 订阅连接数变化事件（浏览器/桌面双通道）。
 * @returns {{ deviceCount: import('vue').Ref<number>, multiDevice: import('vue').ComputedRef<boolean> }}
 *   deviceCount 当前已连接设备数（浏览器视角含本连接，桌面视角为纯浏览器数）；
 *   multiDevice 是否显示多设备指示（桌面通道 count>=1 / 浏览器通道 count>1）。
 */
export function useDeviceConnections() {
  // 初始 1 = 仅本连接（桌面通道首个事件到达前，两端均按「无其他设备」处理）
  const deviceCount = ref(1)
  // 桌面通道归属：桌面事件到达即置位（其后判定走桌面口径），未到达走浏览器口径
  const isDesktopChannel = ref(false)
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
        deviceCount.value = n
      }
    })
    offDesktop = EventsOn(WEBSERVE_CONNECTIONS_EVENT, (count) => {
      const n = Number(count)
      // 桌面通道接受 0（最后一个浏览器断开 / webServe 停机熄指示）；非法载荷
      // 忽略且不锁定通道归属
      if (Number.isFinite(n) && n >= 0) {
        isDesktopChannel.value = true
        deviceCount.value = n
      }
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

  return { deviceCount, multiDevice }
}
