---
name: browser-security-smoke-pending
description: 浏览器模式三期（ec50aa4）真机冒烟待办——TLS 自签例外/wss、token 轮换重验证、多设备提示三条链路，与 chat-interactive 冒烟同场做
metadata:
  type: project
---

浏览器模式三期任务 10-22-tls-token 已归档（commit ec50aa4，go/vitest/Playwright 全绿），**真机冒烟未做**，与 [[chat-interactive-smoke-pending]] 约定同场进行（2026-10-23 起）：

1. **TLS 自签链路**：设置开 TLS（证书路径留空自签）→ 浏览器 https 访问出现证书警告 → 手动「继续前往」后页面正常、同源 wss 正常升级（**research 标注不确定项：https 证书例外是否外溢到同源 wss 无正式契约，重点验证**）；关 TLS 回 http 正常；证书路径填错保存 → toast `E_TLS_CERT_INVALID` 且服务不断（旧实例存活）。
2. **多 token 链路**：新建命名令牌（明文仅一次回显）→ 新设备/隐身窗口用新令牌登录 → 撤销该令牌 → 该会话重连 401 弹令牌输入门；「重新生成默认令牌」→ 旧 cookie 失效、`?token=` 新令牌重验证恢复（回归既有轮换恢复路径）；`data/web_audit.log` 有认证与生命周期记录且无明文 token。
3. **多设备提示**：两台设备同连 → ActivityBar 出现「N 个设备已连接」指示 + 终端面板多端写入提示行；断开一台即时消失。

涉及规范：docs/spec/browser-channel.md（§2.3 连接数事件、§3 TLS/自签/token verify 各行）。冒烟发现 TLS 握手异常先查 `webServeManager.Start` 证书加载序与 `data/tls_cert.pem` 状态。
