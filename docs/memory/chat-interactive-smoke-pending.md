---
name: chat-interactive-smoke-pending
description: Agent 交互式会话任务（5b00149）真机冒烟待办——2026-10-23 首轮冒烟已定性两条缺陷并修复，待复测确认
metadata:
  type: project
---

10-22-agent-interactive-session 任务已归档（commit 5b00149，PR1+PR2 全部交付，自动化测试全绿）。2026-10-23 首轮真机冒烟结论与待复测项：

**已定性并修复（10-23-chat-webserve 任务，待真机复测确认）：**

1. **审批弹窗未生效（A1 缺陷，已修复待复测）**：让 agent 建文件夹无审批弹窗直接成功。根因 = 用户全局 `~/.claude/settings.json` 配 `permissions.defaultMode: "auto"`，workbench「默认」权限模式省略 `--permission-mode` 参数 → CLI 继承全局 auto（classifier 判安全，不下发 can_use_tool）。修复 = buildChatArgs 恒传 `--permission-mode`（default 也显式传，空串归一）。复测：默认模式执行写操作须弹审批框。
2. **运行中插话无响应（A2 缺陷，根因同上待复测）**：观察到 "Classifier unavailable" 报错——default 模式不走 classifier，修复后应同步消除；若仍异常再立项。

**首轮已过项**：输入框草稿已按会话隔离（切换保存/恢复、发送成功清空）。

**仍待验证**：中断后续聊（停止后进程不死，紧接着再发消息正常回复）；次要——浏览器通道（--serve）审批闭环、多会话并行、空闲 10 分钟回收后 resume 续上下文（暗号复述法）、对话进行中删除会话进程被杀。

冒烟方法细节见任务 prd.md 与 [[trellis-finish-work-entry]]。发现审批链路失效先查 `data/logs/app.log` 与 buildChatArgs 参数组装（docs/spec/ai-chat-service.md §3，注意 CLI flag 与全局 settings defaultMode 的优先级）。
