---
name: trellis-finish-work-entry
description: 任务收尾一律走 /trellis:finish-work，禁单独裸跑 task.py archive
metadata:
  node_type: memory
  type: feedback
  originSessionId: a6d351b8-f44e-47c2-a6f6-454906d30e18
  modified: 2026-10-20T03:13:52.969Z
---

用户指示（2026-10-20）：未来任务归档理论/默认入口是 `/trellis:finish-work` skill，而不是直接裸跑 `python task.py archive`。

**Why:** archive 只是 finish-work 的 Step 3 子步骤；完整收尾还含脏路径分类守门（防止任务代码未提交就归档）与 journal 记账（add_session.py），单独 archive 会跳过这两步、提交链顺序也不受 finish-work 约束。

**How to apply:** 任务实施+提交完成后，收尾统一 load `trellis:finish-work` skill 走四步（盘点 → 脏路径分类 → 归档 → journal）；仅当用户当轮明确点名「只归档、不记 journal」之类指令时才单独跑 archive 并说明跳过了什么。
