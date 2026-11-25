# 复现与根因取证记录

## 复现命令与日志

```bash
cd frontend && npx playwright test e2e/ai-function.spec.js   # 后台
go test ./... > /tmp/repro/go-N.log 2>&1                     # 前台，保留全部输出
```

原始日志保存在 `/tmp/repro/{e2e,go}-{1,2,3}.log`（本机临时目录，不入库）。

## 第 1 轮关键输出

```
# ./...
pattern ./...: open frontend\test-results: The system cannot find the file specified.
FAIL	./... [setup failed]
ok  	workbench	43.228s
```

E2E 侧 `5 passed (16.0s)`、exit 0；`frontend/dist/index.html` mtime 被刷新，证明 `vite build` 确实重建了 dist。

## 第 2 轮关键输出

```
--- FAIL: TestChatService_RunChat_FullFlow (0.07s)
    chat_service_test.go:628: 事件序列[1]: got "chat-task:output", want "chat-task:started" (all=[chat-task:queued chat-task:output chat-task:started chat-task:done])
    chat_service_test.go:628: 事件序列[2]: got "chat-task:started", want "chat-task:output" (all=[...])
FAIL	workbench/service	367.361s
```

## 第 3 轮

全部包 `ok`，exit 0。

## 隔离实验

| 实验 | 结果 | 说明 |
|---|---|---|
| `go test ./service -run TestChatService_RunChat_FullFlow -count=50` | 50/50 PASS | B 类仅在高负载下触发，非确定性单测缺陷而是实现竞态 |
| 临时写入 `frontend/go.mod` 后 `go list ./...` | `pattern all:frontend/dist: cannot embed directory frontend/dist: in different module` | Go 禁止跨模块 embed，go.mod 排除方案作废 |
| `npx vite preview --help` | 支持 `--outDir <dir>` | 可让 preview 指向独立产物目录 |

## 修复后验证数据（2026-11-25）

| 验证项 | 结果 |
|---|---|
| 并行 E2E + coverage-check，第 1/2/3 轮 | 全部 PASS（E2E 5 passed；coverage 四项阈值 98.3 / 90.3 / 81.7 / 61.0） |
| 单跑 coverage-check | PASS，覆盖率与修复前一致 |
| 单跑 E2E | PASS（多次） |
| E2E 前后 `frontend/dist/index.html` | mtime 与 size 不变（生产 dist 未被触碰） |
| `npm run build` / `go vet ./...` | PASS |
| 前端单测 `npm test -- --run` | 130 文件 / 2960 用例全过 |
| `node scripts/check-dep-consistency.mjs` | PASS |
| 失败输出保留 | 模拟失败时脚本打印 `go test` 真实错误（`directory not found` + `[setup failed]`） |
| Go 目录跳过规则实测 | `frontend/.probe-skip` 未被 `go list ./...` 列出；对照组 `frontend/probe-skip2` 被列为 `workbench/frontend/probe-skip2` |

## 代码证据

- `service/chat_service.go:1254-1297` `advanceSession`：写 stdin → `appendMessages` → emit `started`。
- `service/chat_service.go:1420-1460` `handleTurnText` / `handleTurnThinking`：pump goroutine 独立 emit `output`，与 started 无同步。
- `frontend/playwright.config.js:41`：webServer 命令 `npm run build && npm run preview -- --port 4173 --strictPort`。
- `scripts/coverage-check.sh:20`：`go test ./... -coverprofile=coverage.out >/dev/null 2>&1`。
- `.github/workflows/ci.yml:184`：artifact 路径 `frontend/playwright-report/`。
