# PRD：内容搜索超时收口 searchWithRipgrep 返回 E_SEARCH_TIMEOUT 引导级

## 摘要

内容搜索超时被杀后静默返「无匹配」（`nil, nil`），用户误以为关键词不存在。本任务将超时与真空结果区分：超时返回新错误码 `E_SEARCH_TIMEOUT`（warning 引导级），并同步收口 `searchWithGo` 回退路径与多目录聚合语义。

## 背景与现状勘察结论

前序 `streamCommitLog` 超时收口（b1d67f0）留笔指向本问题。勘察确认三处缺口：

### 1. searchWithRipgrep 超时静默吞错（service/content_search.go:166-172）

- `cmd.Run()` err != nil 时：`ExitCode() == 1` 分支返 `nil, nil`（无匹配）。
- rg 超时被杀：Windows TerminateProcess exit code 1 恰命中该分支；Unix signal kill exit -1 落入 `rg failed` 分支——两平台都不报超时。
- rg 手册 exit 2 = 真错误，现状走 `rg failed: <stderr>`，行为保留。

### 2. ContentSearch 循环错误全吞 + rg 失败回退掩盖（service/content_search.go:92-97, 122）

- goroutine 内 err 为局部变量，rg err 时回退 `searchWithGo`，`return groups, nil` 恒定 nil error。
- 若 rg 超时返 AppError 而分流逻辑不变，会触发 searchWithGo 回退；已超时 ctx 下 Walk 立即 SkipAll 返空——超时错误被二次掩盖。**timeout 类 err 必须禁回退**。
- ctx 为 App 方法内构造的共享 ctx（全局搜索 60s / 单目录 10s），ctx 一旦超时所有在途目录同时超时，不存在「部分目录超时其余正常继续搜」形态；已完成的目录不受影响。

### 3. searchWithGo 消费 ctx 但超时返部分结果（service/content_search.go:215-219）

- Walk 回调有 `ctx.Done() → filepath.SkipAll` 检查：超时中途停止，**返回部分结果且无 error**——部分结果看似全量，同为静默假态。
- 该函数签名无 error 返回，7 处既有测试直接消费 items。

### 4. 前端展示缺口（frontend/src/composables/useCommandPalette.js:116-117）

- `executeContentSearch` catch 静默 `contentGroups.value = []`，无任何提示。新错误码上报后前端也看不见，须同步加分流。

## 方案

### service/content_search.go

1. **searchWithRipgrep**：`cmd.Run()` err != nil 分支最前新增 `ctx.Err() != nil` 判定（先于 exit 1 与 rg failed，覆盖 Windows exit 1 / Unix exit -1 两平台）→ 返 `model.WrapAppError(model.ErrCodeSearchTimeout, "内容搜索超时已终止，可缩小范围后重试", ctx.Err())`。ctx 未超时的 exit 1 无匹配语义逐字不变（`nil, nil`），exit 2 及其他错误码既有处理不动。
2. **ContentSearch 循环分流**（:92-97）：
   - rg err 经 `ctx.Err() != nil` 判定为超时 → 不回退 searchWithGo，该目录计超时、items 丢弃不入 group；
   - 其他 err（rg failed）保持回退 searchWithGo 现状；
   - searchWithGo 返回后统一判 `ctx.Err() != nil` → 该目录计超时、items（含部分结果）丢弃——以此实现两路径对称收口，不改 searchWithGo 签名（约束：不动匹配逻辑，7 处既有测试零波及）。
3. **多目录聚合**（wg.Wait() 后）：
   - 有非空 groups → 返回 `(groups, nil)`，超时目录 slog.Warn（避免 Wails error 返回丢已有结果）；
   - groups 为空且存在超时目录 → 返回 `(nil, E_SEARCH_TIMEOUT AppError)`；
   - 无超时 → 现状。

### 三处错误码同步

| 位置 | 改动 |
|---|---|
| `model/app_error.go` | 新增 `ErrCodeSearchTimeout = "E_SEARCH_TIMEOUT"`（搜索域段，warning 注释） |
| `frontend/src/utils/error.js` | `ErrorCode.SearchTimeout` + `WARNING_CODES` 加 `SearchTimeout` |
| `docs/spec/logging-and-errors.md` | 4.3 表新增行（格式参照 E_GIT_LOCKED 行）+ 收口段补内容搜索句 |

### 前端展示分流（仅此一处，无新交互）

`useCommandPalette.js executeContentSearch` catch：`extractError` 取 code/message，code 命中 `ErrorCode.SearchTimeout` → `ElMessage.warning(message)`；其余保持现状静默。

## 不做清单（约束）

- 不动 rg exit 1 无匹配语义（ctx 未超时时 `nil, nil` 逐字不变）。
- 不动 exit 2 及其他错误码既有处理（`rg failed` 格式不变）。
- 不动 searchWithGo 匹配逻辑与函数签名。
- 不复用 `E_GIT_TIMEOUT`（跨域误引致提示文案错误）。
- 不加用户取消入口（ContentSearch 无取消 UI，ctx 仅 App 层超时驱动，Canceled 与 DeadlineExceeded 统一返 E_SEARCH_TIMEOUT）。
- 测试禁 sleep 与真实慢命令：ctx 预取消 / 1ns 超时注入驱动（参照 util/git_context_test.go、commit_history_cli_test.go 先例）。
- Wails 绑定无同步（ContentSearch 签名不变）。

## 测试计划（service/content_search_test.go 追加）

| 用例 | 构造 | 断言 |
|---|---|---|
| rg 超时返 E_SEARCH_TIMEOUT | `rgAvailable: true` + 预取消 ctx（rg 二进制在不在 PATH 均覆盖：预取消 ctx 下 Run 在 Start 前即返 ctx 错误） | `errors.As` AppError code == E_SEARCH_TIMEOUT，非 nil,nil |
| Go 路径对称收口 | `rgAvailable: false` + 预取消 ctx 调 ContentSearch | 返回 AppError code == E_SEARCH_TIMEOUT 而非 nil,nil |
| 无匹配回归 | ctx 未超时 + 真实 rg（`LookPath("rg")` miss 则 t.Skip）+ 无匹配关键词 | `nil, nil` 保持 |
| 多目录正常回归 | 既有 `TestContentSearch_MultipleDirs` | 全绿不动 |
| 聚合-全部超时报错 | `rgAvailable: false` + 预取消 ctx + 两目录 | E_SEARCH_TIMEOUT |
| 有结果不报错路径 | 共享 ctx 下「部分目录超时其余完成」无确定性构造，靠代码审查 + 全量回归覆盖 | — |

## 验收标准

1. 超时单测断言新码 AppError 而非 `nil, nil` 静默返空。
2. 无匹配回归：ctx 未超时 exit 1 仍返 `nil, nil`，既有用例全绿。
3. searchWithGo 路径经 ContentSearch 层 ctx.Err() 判定对称收口（设计决策记录于本文档，替代改签名方案）。
4. `go test ./...` + `-race` + `cd frontend && npm test` 全绿。
5. 文档同步：logging-and-errors.md 4.3 表 + 收口段；CLAUDE.md 关键规则视沉淀价值增行。
6. trellis-check + 子 agent 审核通过。

## 实施顺序

1. model/app_error.go 加常量 → service/content_search.go 收口 → service/content_search_test.go 用例。
2. frontend error.js + useCommandPalette.js 分流。
3. 全量测试 → 文档同步（logging-and-errors.md、CLAUDE.md）。
4. trellis-check + 子 agent 审核 → 提交。
