# 聚合视图欠账清理：HasRemote 覆盖与 RPC 屏蔽回归测试

## Goal

清理 `docs/agent-resources/temp-memory/TEMP-MEM-activeContext.md`「杂项欠账」中两条**零业务行为变更**的回归测试缺口，使聚合视图的两处关键契约有测试兜底，防止后续重构静默回退。

## Requirements

1. **`GetRootNodesForPaths` 的 `HasRemote` 分支覆盖**
   - 现状：既有测试用伪造 `.git` 目录，`git remote -v` 恒失败，`HasRemote` 恒为 false；删除 `node.HasRemote = s.hasRemote(p)` 后测试仍全绿。
   - 要求：用 `util/testutil.RunGit` 初始化真实仓库 `git init` + `git remote add origin <url>`，断言 `HasRemote=true`；同时断言无 remote 的真实仓库 `HasRemote=false`。

2. **`GetAggregatedRootNodes` 的浏览器 RPC 屏蔽回归测试**
   - 现状：该方法已登记于 `main.go` 的 `RPCHiddenMethods`，但无专项测试。
   - 要求：对齐 `app_webserve_ipban_v10_test.go` 先例，经生产装配（`startLocked` 真实 handler）断言：
     - 浏览器 RPC 命中返回 `server.ErrCodeRPCMethodNotFound`，消息形态与真不存在方法一致（不暴露存在性）；
     - 同 handler 非屏蔽方法不受影响；
     - 桌面 Wails 直调不受屏蔽影响。

## Acceptance Criteria

- [ ] `go test ./service/ -run TestGetRootNodesForPaths` 通过，且新增用例在删除 `HasRemote` 赋值后失败（具备回归能力）
- [ ] `go test . -run TestServeRPC_GetAggregatedRootNodesHiddenForBrowser` 通过
- [ ] `go vet ./...` 通过
- [ ] 不修改任何业务代码，仅新增/追加测试文件

## Definition of Done

- 两条测试均使用项目既有辅助（`util/testutil.RunGit`、`newWebServeTestAppDir`），不重复自建
- 测试注释用中文说明守护的契约与失效后果
- 完成后从 `TEMP-MEM-activeContext.md` 删除已解决的这两条欠账
- 确认 README.md 无需更新（纯测试补充，无功能面变化）
