# Journal - liuyang (Part 3)

> Continuation from `journal-2.md` (archived at ~2000 lines)
> Started: 2026-10-10

---



## Session 111: 工具箱新增进程管理（端口/关键字查进程 + 停止/强杀）

**Date**: 2026-10-10
**Task**: 工具箱新增进程管理（端口/关键字查进程 + 停止/强杀）
**Branch**: `master`

### Summary

工具箱新增「进程管理」：按端口号或进程名关键字检索本机进程，展示进程名/PID/端口/协议/状态/内存，每行可「停止」（发优雅请求→5s 轮询实测存活→未退自动转强杀）或「强杀」，进程树为默认关闭的二级选项。技术选型为零依赖方案（Windows netstat+tasklist，Linux /proc），不引入 gopsutil。

关键产出：用户手工实测推翻了调研报告中「有窗口 GUI 进程可优雅关闭、无窗口后台进程无优雅通道」的结论——实测 ping（隐藏窗口控制台）响应并退出，cmd /k（有可见窗口）报「只能强行终止」且仍存活，方向恰好相反。正确语义是「能否优雅取决于目标进程是否响应，无法按有无窗口预判」，故实现不做事前预判、统一轮询实测，且不以 taskkill 的 SUCCESS 判定已退出（仅代表信号送达）。该更正已同步到 8 个代码文件注释、用户文档、PRD 与 spec，调研报告原结论标注为被推翻并保留推理过程。

实施分 4 个 PR：后端底座（util 平台分叉+解析纯函数）、service 编排与 App 绑定与装配、前端弹窗与三级确认、文档与规范沉淀。质量检查修复 5 个问题，其中 2 个为实质缺陷：cmd.Output() 丢弃 stderr 致 taskkill 失败形态识别失效、终止命令无超时可致前端卡死。

验证：go build（Windows+Linux 交叉）通过；go test ./util/ ./model/ ./service/ 全绿（service 覆盖率 80.8%，门禁 76%）；npm test 1497 用例全绿。新增 docs/spec/process-management.md 并在 CLAUDE.md 关键规则表与 spec 索引各加一条。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6ec28e9` | (see git log) |
| `8d9cca4` | (see git log) |
| `b9e60ca` | (see git log) |
| `0614b4e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
