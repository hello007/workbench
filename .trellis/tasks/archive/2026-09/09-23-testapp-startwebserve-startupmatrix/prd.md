# PRD：修复 TestApp_StartWebServe_StartupMatrix 设置开启默认启动断言失败

## 摘要

`TestApp_StartWebServe_StartupMatrix/设置开启默认启动` 在本机稳定失败。
定性为**测试 bug（环境隔离不足）**，非产品 bug。修复：子测试注入随机端口
override，消除对本机固定端口 36115 空闲的隐式依赖。

## 根因

1. 子测试仅 `newWebServeTestApp(t)` + `app.startWebServe()`，未设
   `webServeListenOverride`，监听地址回落 `cfg.WebServe.BindAddress` =
   默认固定端口 `127.0.0.1:36115`（model/settings.go:6
   `DefaultWebServeBindAddress`）。
2. 本机运行中的 workbench.exe（实测 PID 82452）常驻监听 36115（浏览器访问
   通道默认开启），`Start` 监听失败走告警降级路径（web_serve.go:252
   slog.Warn 后 return），`Running()=false`，断言失败。
3. 解释「偶发」：仅当本机 workbench.exe 运行时挂；上一任务审核期间偶发、
   本次会话稳定复现，与应用是否开着完全相关，与代码提交无关。

## 定性依据

- 产品降级行为正确：端口占用告警降级是设计行为，第三个子测试
  「端口占用降级不阻塞」专门验证该路径。
- 测试隔离缺口：唯一受影响的是第一个子测试（另两个不监听默认端口），
  它隐式依赖「本机 36115 空闲」，违背测试隔离原则。

## 修复方案

第一个子测试注入 `app.webServeListenOverride = "127.0.0.1:0"`（随机空闲
端口），与第三个子测试的占用注入模式、`TestApp_GetWebServeConfig` 的
`Start("127.0.0.1:0")` 先例一致。仅改 web_serve_test.go 一处，不动产品代码。

## 验收

- `go test ./ -run TestApp_StartWebServe_StartupMatrix -count=1` 绿。
- `go test ./` 全绿，无新失败。

## 总结

测试对环境的隐式依赖（固定端口）是本例根因；同类测试已用
`webServeListenOverride`/`:0` 模式隔离，本次将该模式补齐到遗漏的子测试。
