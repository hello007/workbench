## Why

`main.go` 中使用 Go 内置的 `println` 输出 Wails 启动错误，缺少时间戳和日志级别信息，不符合 Go 标准日志实践，不利于生产环境的问题排查。

## What Changes

- 将 `println("Error:", err.Error())` 替换为标准 `log.Fatalf`
- 新增 `log` 包导入

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `application-startup`: 应用启动失败时的错误输出改为标准日志格式，包含时间戳并自动退出进程

## Impact

- `main.go`: 替换错误输出方式，新增 import
