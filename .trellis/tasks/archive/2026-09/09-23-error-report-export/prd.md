# 错误报告导出/发送

## Goal

崩溃恢复唯一未勾子项「错误报告发送」（路线图 L275）。日志已落盘 `data/logs/app.log`（slog + lumberjack 5MB×5 轮转），差用户侧出口。桌面单用户无后端，「发送」实为导出诊断包由用户手动提交。

## Requirements

* **brainstorm 决策（桥接未获应答按推荐项推进，均可低成本调整）**：
  * 「发送」语义 → **导出 zip 诊断包到用户选择路径**（SaveFileDialog 已有；桌面单用户无后端，mailto 依赖邮件客户端且附件超限，复制摘要丢日志上下文）。后端 `ExportDiagnostics(zipPath)` 直接写二进制 zip（现有 SaveFile 仅支持 string content，不适用）。包内容：`diagnostics.txt`（版本/构建时间/OS/Go 版本/导出时间/文件清单）+ `logs/`（app.log 与轮转备份原样收纳）+ `session.json`（存在时）
  * 入口位置 → **设置面板「关于」节「导出诊断信息」按钮**（应用无帮助菜单，关于节是版本/日志信息天然归属）；成功后提示 zip 路径引导用户提交
  * 可选增强崩溃检测 → **做，但用独立 `data/crash.flag` 标记文件而非 session.json 字段**：session.json 被前端 debounce 频繁 Save 覆盖写，crashed 标记放该链路会被正常快照保存覆盖，边界纠缠；独立 flag 生命周期为「startup 检测旧 flag（存在=上次异常退出）→ 重建 flag 标记本次运行 → 正常 shutdown 删除」，前端 App.vue 挂载时 `GetAndClearLastCrashFlag()` 读后即清，true 则 ElNotification 提示可从设置导出诊断
* 导出诊断包：日志文件 + 版本号 + OS 信息打包 zip 到用户选择路径

## Acceptance Criteria

* [x] 诊断包导出功能 + 单测（zip 内容完整性、日志缺失降级）——`service.BuildDiagnosticsZip` 纯函数：3 组测试覆盖完整收纳（4 条目内容断言）、日志+session 双缺失降级（注明缺失）、目标不可写报错
* [x] 入口 UI + 组件测试——设置-关于「导出诊断信息」按钮（SaveFileDialog 选路径 → ExportDiagnostics → 成功提示路径，取消不调导出）；崩溃检测：startup 检测 `data/crash.flag` + App.vue 挂载提示（读后即清）；wails-mock-defaults 补 GetAndClearLastCrashFlag 默认值（E2E 挂载即调）
* [x] docs/功能说明.md、docs/路线图.md 同步（L275 勾选）
* [x] 收口验证：go test ./... 6 包全绿；前端 vitest 1392 全过；npm run build 通过；coverage 全 PASS（service 80.1%）；gofmt/vet 干净；wailsjs 绑定三处同步

## Out of Scope

* 自动上传远端服务（无后端）；遥测埋点

## Technical Notes

* 基础：崩溃恢复 session.json 链路已有（Load 损坏降级模式先例）；lumberjack 轮转日志路径固定
