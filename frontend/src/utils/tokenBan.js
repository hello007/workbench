/**
 * 令牌封禁联动踢出编排（连接管控四期，桌面专用路径）。
 *
 * 封禁服务端语义 = 仅拒新连（HTTP requireToken / WS 握手两认证点组合判定），
 * 已建立连接不受影响（对齐撤销热生效语义）；「同时断开在途连接」为前端编排：
 * 按被封禁 tokenID 过滤连接明细缓存逐个 KickConnection。仅桌面通道有明细数据
 * （隐私红线：浏览器通道 connections 恒空），浏览器端本函数天然无操作面。
 */

import { KickConnection } from '../../wailsjs/go/main/App'
import { ElMessage } from 'element-plus'

/**
 * 封禁生效后断开该令牌的全部在途连接（fail-soft：单个失败不中断其余，收集后
 * 统一 warning 提示——封禁本身已生效，断开失败仅为处置不彻底，引导手动补踢）。
 *
 * @param {Array<{id:number, tokenId:string}>|undefined} connections 连接明细快照
 *   （useDeviceConnections 的 connections ref 值；undefined/非数组按空处理）
 * @param {string} tokenId 被封禁的令牌记录 ID
 * @returns {Promise<void>} 全部踢出尝试完成后 resolve（不抛错——断开失败已收敛
 *   为 warning 提示，调用方无需再分流）
 */
export async function kickTokenConnections(connections, tokenId) {
  const targets = (Array.isArray(connections) ? connections : []).filter((c) => c?.tokenId === tokenId)
  if (!targets.length) return
  let failed = 0
  for (const c of targets) {
    try {
      await KickConnection(c.id)
    } catch {
      // fail-soft：单连接断开失败（已断开/服务停机等）不中断其余连接的踢出
      failed++
    }
  }
  if (failed > 0) {
    ElMessage.warning(`封禁已生效，但 ${failed}/${targets.length} 个在途连接断开失败，可在连接明细中手动踢出`)
  }
}
