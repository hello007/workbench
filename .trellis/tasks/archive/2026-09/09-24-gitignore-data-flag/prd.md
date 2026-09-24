# PRD：gitignore 补齐 data/crash.flag 落盘规则

## 背景

本轮 error-report-export 任务（2026-09-23 归档）新增了 `data/crash.flag` 异常退出标记落盘点（`app.go` startup 检测 / `service/diagnostics.go` flag 生命周期管理），但 `.gitignore` 未同步补规则，导致 `git status` 出现未跟踪的 `data/crash.flag`。本任务对 `data/` 目录全部运行期落盘文件与 `.gitignore` 规则做一次完整盘点，补齐缺失项。

## 盘点结论（2026-09-24 实测 data/ 目录）

| data/ 落盘 | .gitignore 规则 | 状态 |
|---|---|---|
| 各运行期 JSON（settings/directories/favorites/repo_meta/repo_scan_cache/ai_functions/ai_task_history/session） | `data/*.json` | ✅ 已覆盖 |
| directories.json.template | `!data/*.template` 白名单 | ✅ 已覆盖 |
| logs/（slog+lumberjack 结构化日志） | `data/logs/` | ✅ 已覆盖 |
| ai_task_history/、ai_task_output/ | `data/ai_task*` | ✅ 已覆盖 |
| ai_chat/（ChatService 会话存储） | `data/ai_chat/` | ✅ 已覆盖 |
| web_token（serve 模式访问令牌） | `data/web_token` | ✅ 已覆盖 |
| **crash.flag（异常退出标记）** | **无** | ❌ **本任务补齐** |

## 附带核查项（结论：均无需改动）

- **build/bin/**：`build/*` 整体忽略 + `!build/README-linux.md` 白名单放行 → 覆盖完整。
- **frontend/test-results/**：已有 `frontend/test-results/`，另有 `frontend/playwright-report/`、`frontend/coverage/` → 覆盖完整。

## 需求

1. `.gitignore` 的 data 区块新增 `data/crash.flag` 一行，附中文注释说明（异常退出标记，指向 app.go startup 检测逻辑），风格与既有 `data/logs/`、`data/ai_chat/`、`data/web_token` 条目一致。
2. 除上述一行外不改动其他 .gitignore 规则。

## 验收标准

- `git status` 中不再出现未跟踪的 `data/crash.flag`（用 `git check-ignore -v data/crash.flag` 验证命中规则）。
- `git status --porcelain` 工作区干净（除本任务自身改动）。
- `go test ./...` 全绿（.gitignore 改动不影响编译，属收口回归确认）。
