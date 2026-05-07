# application-startup Specification

## Purpose
TBD - created by archiving change improve-main-error-logging. Update Purpose after archive.
## Requirements
### Requirement: 应用启动失败时输出标准日志

当 Wails 运行失败时，系统 SHALL 使用标准 `log` 包输出错误信息，包含时间戳，并以非零状态码退出进程。

#### Scenario: Wails 启动失败

- **WHEN** `wails.Run()` 返回非 nil 错误
- **THEN** 系统使用 `log.Fatalf` 输出错误信息
- **AND** 输出包含错误详情和时间戳
- **AND** 进程以状态码 1 退出

