# PRD：freePort TOCTOU 竞态修复

## 背景

journal Session 88（修复 StartupMatrix 测试端口依赖）时定性：`freePort` 类辅助先探测空闲端口、返回后实际监听之间存在 TOCTOU 竞态窗口（端口可能被本机其他进程抢占，如常驻 workbench.exe）。当时为既有残留未动，本任务收尾。

## 需求

1. 定位 freePort 辅助的所有调用点（web serve 启动测试等）。
2. 评估修复方案：优先「直接让监听方绑定 `:0` 由内核分配端口再回读实际地址」消除探测步骤；若调用方结构无法直接绑定 :0，则文档化接受竞态并说明原因。
3. 实施修复或留档接受，同步 `docs/spec/test-stability.md` 相应章节。
4. 相关测试全量通过（`go test ./...`）。

## 验收

- [ ] freePort TOCTOU 竞态消除（绑定 :0 方案）或明确留档接受且理由充分。
- [ ] `go test ./...` 全绿。
- [ ] test-stability.md 与实现一致。

## 非目标

- 不处理生产代码端口分配（仅测试辅助/serve 启动链路）。
