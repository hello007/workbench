# 冒烟缺陷修复：chat 输入未绑会话 + 审批弹窗未生效

## Goal

修复 2026-10-23 A 组真机冒烟（commit 5b00149 chat 持续会话）暴露的两条缺陷，补齐 chat-interactive-smoke-pending 待办中未过的审批链路。

## 根因定性（已诊断）

### 缺陷 1：AI 对话输入框内容不随会话切换
- 现象：会话 a 输入文字后切到 b/c，文字仍在输入框
- 根因：`AiChatPanel.vue:754` `inputText` 单 ref，无按会话草稿存储
- 修复：按会话草稿（`selectedChatSessionId` 切换时保存旧会话草稿、载入新会话草稿；Map<sessionId, draft>，会话删除时清理可选）

### 缺陷 2：A1 审批弹窗未生效（工具直接执行）
- 现象：AI 对话让 agent 建文件夹，无审批弹窗直接成功
- 根因链：用户全局 `~/.claude/settings.json` 配 `"permissions": {"defaultMode": "auto"}`（CLI auto 模式走 classifier 判安全，不发 can_use_tool 审批请求——2026-10-23 真机日志零审批痕迹 + A2 报错 "Classifier unavailable" 佐证）→ workbench `buildChatArgs`（service/chat_service.go:760）在会话权限模式为「默认」时**省略** `--permission-mode` 参数 → CLI 继承全局 auto → 审批协议被静默绕过
- 修复：「默认」模式也**显式传** `--permission-mode default`（CLI flag 优先级高于 settings defaultMode，审批协议恢复生效）；附带收益——default 模式不走 classifier，A2 观察到的「插话无响应/classifier unavailable」一类挂起同步消除
- 边界：`permissionMode` 为空串（老数据/未选）同归「默认」处理显式传参

## Requirements

* R1 前端：AiChatPanel 输入框按会话隔离草稿（切换保存/恢复；发送成功清当前会话草稿）
* R2 后端：buildChatArgs 显式传 `--permission-mode default`（含空串归一）；spawn 参数快照注释同步
* R3 测试：buildChatArgs 单测更新（default 显式传参断言）；前端草稿切换用例（vitest）
* R4 文档：docs/spec/ai-chat-service.md 补「默认显式传参防全局 defaultMode 覆盖」契约；chat-interactive-smoke-pending 记忆更新冒烟结论

## Acceptance Criteria

* [ ] 会话 a 输入文字切到 b/c 输入框为空，切回 a 文字恢复
* [ ] 用户全局 CLI defaultMode=auto 环境下，AI 对话「默认」权限模式执行写操作弹审批框（复测 A1 场景）
* [ ] `go test ./...` / vitest / build 全绿

## Out of Scope

* 桌面端「N 个浏览器连接」指示（B 组反馈项，设计如此为预期；如需做另行任务）
* A2 插话链路的独立修复（根因同 2 号，修复后复测；若仍异常再立项）
* webServe 开关不启动（上一轮反馈 4 号，本轮反馈未复现，待复测确认；复现再立项）

## Technical Notes

* 诊断证据：~/.claude/settings.json `permissions.defaultMode: "auto"`；data/logs/app.log 零 chat/permission 痕迹；buildChatArgs:760-766 现状（default 分支不传参）
* chat-approval 链路本身完整（parse → registerApproval → chat-approval:request → 前端弹窗监听齐备），缺的是 CLI 侧请求发起
