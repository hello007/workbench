/**
 * 多设备连接数感知 composable（浏览器通道事件驱动，D1 决策：架构零改动只提示）。
 *
 * 消费后端 WSHub 广播的 `ws-connections:changed` 事件（server/ws.go，data[0] 为
 * 事件后连接总数），暴露 deviceCount ref 供 UI 呈现「N 个设备已连接」提示与
 * 终端面板多端写入提示。
 *
 * 通道语义：
 *  - 浏览器（serve）模式：事件经 transport events.js 广播分发（含 resume 补发，
 *    计数为幂等快照，最后到达值即当前态）；
 *  - 桌面模式：后端不产生该事件（仅 hub 广播），监听器静默无回调，deviceCount
 *    恒为初始值 1，提示不显示——空安全，无需调用方分流。
 *
 * 监听注销遵守 cross-layer-contracts 契约：EventsOn 返回闭包在 onBeforeUnmount
 * 精准移除本组件监听器（多组件共听同事件，禁 EventsOff 全局移除）。
 */

import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// 后端 WSHub 连接数变化事件名（server/ws.go wsConnectionsEvent，取值须与后端一致）。
export const WS_CONNECTIONS_EVENT = 'ws-connections:changed'

/**
 * 订阅连接数变化事件。
 * @returns {{ deviceCount: import('vue').Ref<number>, multiDevice: import('vue').ComputedRef<boolean> }}
 *   deviceCount 当前已连接设备数（含本连接）；multiDevice 连接数 > 1（存在其他设备接入）。
 */
export function useDeviceConnections() {
  // 初始 1 = 仅本连接（桌面模式恒此值；浏览器模式首个 changed 事件到达前同样按 1 处理）
  const deviceCount = ref(1)
  const multiDevice = computed(() => deviceCount.value > 1)

  let offChanged = null

  onMounted(() => {
    offChanged = EventsOn(WS_CONNECTIONS_EVENT, (count) => {
      const n = Number(count)
      // 防御非法载荷（老服务端/异常帧）：仅接受正整数，忽略后保持上次值
      if (Number.isFinite(n) && n >= 1) {
        deviceCount.value = n
      }
    })
  })

  onBeforeUnmount(() => {
    if (offChanged) {
      offChanged()
      offChanged = null
    }
  })

  return { deviceCount, multiDevice }
}
