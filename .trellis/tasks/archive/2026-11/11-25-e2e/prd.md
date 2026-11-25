# PRD：E2E 与后端覆盖率脚本并行的资源隔离

## 1. 背景与现象

同时执行以下两条命令时，第二条命令稳定间歇失败：

```bash
cd frontend && npx playwright test e2e/ai-function.spec.js
bash scripts/coverage-check.sh          # 报 FAIL: go test 失败
```

`coverage-check.sh` 用 `go test ./... -coverprofile=coverage.out >/dev/null 2>&1` 且失败时吞掉全部输出，只打印一句结论，因此现场看不到真实错误。

## 2. 复现证据（3 轮，2026-11-25）

| 轮次 | go test 结果 | 失败文本 |
|---|---|---|
| 1 | FAIL（setup failed） | `pattern ./...: open frontend\test-results: The system cannot find the file specified.` 同时主包 `ok workbench 43.228s` |
| 2 | FAIL（service 包） | `TestChatService_RunChat_FullFlow`：`事件序列[1]: got "chat-task:output", want "chat-task:started"`（实际 `queued → output → started → done`） |
| 3 | PASS | — |

补充证据：

- `TestChatService_RunChat_FullFlow` 单跑 `-count=50` 全通过，说明 B 类失败仅在高负载（E2E 构建 + 5 worker + Go 覆盖率插桩并行）下暴露，属于被放大的既有竞态。
- `go list ./...` 仅返回 7 个业务包，但 Go 的目录遍历会进入 `frontend/` 下所有非 `.`/`_` 开头目录（含 `test-results`、`playwright-report`、`dist`、`node_modules`）。
- `frontend/playwright.config.js` 的 webServer 每次执行 `npm run build`（vite 默认 `emptyOutDir`，会先清空 `frontend/dist`）。

## 3. 根因

### 3.1 根因 A：`./...` 目录遍历与 Playwright 重建输出目录竞态（本轮主因）

Go 工具链展开 `./...` 时逐目录 `open`，Playwright 每轮开始会删除并重建 `frontend/test-results`（html reporter 同理写 `playwright-report`）。遍历到该目录的瞬间目录不存在 → `ENOENT` → 整个 `go test ./...` 以 `[setup failed]` 结束，**任何用例都没有机会运行**。

### 3.2 根因 B：`chat-task:started` 与 `chat-task:output` 无顺序保证

`service/chat_service.go` 中 `advanceSession` 的顺序是：写 stdin → `appendMessages`（磁盘 IO）→ emit `started`；而 `pumpSession` goroutine 在 stdin 写入成功后可立即读取子进程输出并 emit `output`。高负载下 pump 先于 started 完成 emit，事件序列颠倒。真实 claude 进程响应快时同样可能触发，属于实现层竞态，而非测试断言过严。

### 3.3 根因 C：`frontend/dist` 与 `//go:embed` 的构建期竞态（结构性隐患）

E2E 每次重建 `frontend/dist`，主包编译期 `//go:embed all:frontend/dist` 需读取该目录。若编译与 `vite build` 的 `emptyOutDir` 重叠，主包编译可能失败或嵌入半态产物。本轮未命中（主包已编译缓存/竞态窗口未撞上），但结构上必然存在。

### 3.4 已排除项

| 候选 | 证据 | 结论 |
|---|---|---|
| a. 端口冲突 4173 / 36115 | 运行前后 `netstat` 检查：4173、36115 均无第三方占用；Go 测试代码无任何 `4173` 引用；36115 相关单测已按 `test-stability.md` §7 使用 `127.0.0.1:0` 或注入 | 排除 |
| c. CPU/内存竞争 | 竞争仅放大了 B 类既有竞态，未产生超时类失败；失败文本均为确定性错误（ENOENT / 顺序断言） | 非独立根因 |
| d. 共享临时目录 / 缓存 | Go 测试统一使用 `t.TempDir()`；Go build cache 并发安全；唯一共享路径是 `frontend/dist`（见根因 C） | 仅 `dist` |

## 4. 修复方案（按优先级）

### 4.1 E2E 产物目录隔离（解决 A、C）

Go 目录遍历规则：跳过以 `.` 或 `_` 开头的目录、`testdata`、含 `go.mod` 的子目录。**已验证 `frontend/go.mod` 不可用**——会让 `//go:embed all:frontend/dist` 报 `cannot embed directory frontend/dist: in different module`，故采用 `.` 前缀方案：

| 项 | 现状 | 目标 |
|---|---|---|
| E2E 构建产物 | `frontend/dist`（与 Wails 生产产物同名，被 embed） | `frontend/.e2e-dist` |
| test-results | `frontend/test-results` | `frontend/.e2e-artifacts/test-results` |
| html report | `frontend/playwright-report` | `frontend/.e2e-artifacts/report` |

同步面：`frontend/playwright.config.js`、`frontend/package.json`（`build:e2e`、`e2e:report`）、`.gitignore`、`.github/workflows/ci.yml` artifact 路径。

### 4.2 `chat-task:started` 顺序保证（解决 B）

把 `started` 的 emit 前移到 stdin 写入之前（轮次已在锁内置为 `executing`，语义为「本轮开始执行」）。`output` 只能由 stdin 写入后的子进程响应触发，因此 started 必然先于 output；同时更新 `RunChat` 事件序列注释与前端 store 注释。

### 4.3 端口

无需改动：E2E 独占 4173，Go 测试不引用该端口；`test-stability.md` 既有 `:0` 规则足够。

### 4.4 可观测性

`coverage-check.sh` 失败时保留并打印 `go test` 的真实输出（写入临时文件后 tail），避免后续同类问题再次被一句结论掩盖。

> 禁止项：不使用 sleep、不放大超时、不删除/放宽断言、不跳过测试。

## 5. 验收标准

1. 并行执行两条命令连续 ≥3 轮，全部通过（E2E exit 0、coverage exit 0）。
2. 两条命令各自单独执行仍通过，覆盖率数值不低于修复前（model 98.3 / server 90.3 / service 81.7 / util 61.0）。
3. `go vet ./...`、`go test ./...` 单跑通过。
4. 失败时 `coverage-check.sh` 能打印真实失败输出。
5. `docs/spec/test-stability.md` 新增「E2E 与 Go 测试并发的资源隔离」一节；AGENTS.md 关键规则按两档上限追加。
