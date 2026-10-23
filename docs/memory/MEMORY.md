# Memory Index

- [WorkBench 数据写入约束](workbench-data-write-constraint.md) — 手改产物 JSON 前仍须关闭 workbench.exe（整体覆盖模型不变）；崩溃截断损坏已由原子写消除（2026-09-24 收口）
- [Trellis 收尾入口](trellis-finish-work-entry.md) — 任务归档走 /trellis:finish-work，禁裸跑 task.py archive
- [Agent 交互式会话真机冒烟待办](chat-interactive-smoke-pending.md) — 5b00149 归档后待 2026-10-23 人工验证：审批弹窗（漏配=无弹窗直接执行）/插话/中断后续聊
- [浏览器安全三期真机冒烟待办](browser-security-smoke-pending.md) — ec50aa4 归档后待人工验证：TLS 自签例外+wss 外溢（research 不确定项）/多 token 撤销轮换/多设备提示，与 chat-interactive 冒烟同场
