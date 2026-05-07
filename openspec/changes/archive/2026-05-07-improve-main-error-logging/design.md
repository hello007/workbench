## Context

`main.go:32` 当前使用 `println("Error:", err.Error())` 输出启动错误。`println` 是 Go 内置函数，直接写入 stderr，无时间戳、无日志级别，不符合 Go 社区标准实践。

## Goals / Non-Goals

**Goals:**
- 使用标准 `log.Fatalf` 替换 `println`，提供时间戳并自动退出

**Non-Goals:**
- 不引入第三方日志库（如 zap、zerolog）
- 不改造整体日志架构

## Decisions

### Decision 1: 使用 `log.Fatalf` 而非 `log.Fatal`

选择 `log.Fatalf` 是因为它支持格式化字符串，与 Go 标准库一致，零依赖。`log.Fatalf` 会自动追加换行符，输出时间戳到 stderr，然后调用 `os.Exit(1)`。

备选方案：
- `log.Fatal("Error: ", err)` — 也可行，但 `Fatalf` 格式化更简洁
- 引入 `slog`（Go 1.21+）— 过度设计，启动入口只需简单错误输出
