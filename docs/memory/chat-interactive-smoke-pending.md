---
name: chat-interactive-smoke-pending
description: Agent 交互式会话任务（5b00149）真机冒烟待办——2026-10-23 需人工验证的三条核心链路
metadata:
  type: project
---

10-22-agent-interactive-session 任务已归档（commit 5b00149，PR1+PR2 全部交付，自动化测试全绿），**真机冒烟未做**，2026-10-23 需人工验证：

1. **审批弹窗（最高优先）**：默认权限模式下让 AI 创建文件夹——须弹审批框；点「允许」执行、点「拒绝」模型改口。**若命令直接执行没弹窗 = `--permission-prompt-tool stdio` 漏配**（该漏配静默自动拒绝/放行，无报错，自动化测试测不到真机行为）。
2. **运行中插话**：长任务生成中再发消息——不报错、第二个流式气泡、按序执行。
3. **中断后续聊**：停止后进程不死，紧接着再发消息正常回复。

次要：浏览器通道（--serve）审批闭环、多会话并行、空闲 10 分钟回收后 resume 续上下文（暗号复述法）、对话进行中删除会话进程被杀。

冒烟方法细节见任务 prd.md 与 [[trellis-finish-work-entry]]。发现审批链路失效先查 `data/logs/app.log` 与 buildChatArgs 参数组装（docs/spec/ai-chat-service.md §3）。
