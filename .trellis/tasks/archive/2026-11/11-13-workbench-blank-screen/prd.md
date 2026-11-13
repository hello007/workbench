# brainstorm: WorkBench 白屏卡死排查（已结案：环境级前端猝死，重启恢复）

## 目标

诊断安装版 WorkBench（`D:\Program Files\WorkBench\workbench.exe`）窗口白屏假死现象，定位根因并恢复可用。

## 结论

**已恢复**（2026-11-13 14:34 杀旧进程重启，界面正常，新进程内存 165MB 正常量级）。根因定性：**WebView2 renderer 主线程同步阻塞类挂起**（非崩溃、非代码、非产物），属运行时环境级故障，已沉淀 `docs/agent-resources/troubleshooting/webview2-blank-freeze.md`。

## 最终时间线（用户证词修正后）

| 时间 | 事件 | 依据 |
|---|---|---|
| 11-11 19:18 | WebView2 Evergreen 升 .42 → .46 | 版本目录 mtime |
| 11-12 09:38 | 启动（.46），正常使用至 13:48 正常关闭 | app.log |
| 11-12 13:50 | 构建新版 exe（439a6b3 + dc47e4d） | exe.bak mtime + git log |
| 11-12 14:01:55 | 启动新版 → PID 16808；startup 正常（crash.flag 重建） | app.log / Win32_Process |
| 11-12 14:01 ~ 11-13 上午 | **正常运行使用**（用户证词：早上还在用；正常使用本就不打业务日志，与日志静默一致） | 用户证词 |
| **11-13 11:10:17** | **前端猝死**：最后快照 `session.json` 与 localStorage leveldb（001934.ldb）同秒停写 | savedAt 换算 + mtime |
| 11-13 11:10 后 | 白屏假死 3 小时+：renderer 进程未重建、CPU 零增长（21h 仅 16 秒）、无 crash dump、全部 WebView2 子进程存活 | 进程取证 |
| 11-13 14:34 | taskkill /T 杀树 → 重启 17:29 版 → **正常**（165MB、界面完整、crash flag 正确检测提示） | 实测 |

## 根因分析

**定性（实证）**：renderer 主线程同步阻塞 → UI 冻结、合成器停止出帧 → 白屏。依据：进程树全活 + renderer 未重建（非崩溃）+ CPU 零增长（纯挂起非死循环）+ 猝死时刻前后端调用链整体停摆。

**排除项**（实证）：
- WebView2 .46 升级直接破坏（09:38~13:48 同 runtime 正常 4h+）
- 13:50 新代码引发（该版猝死前已正常使用 ~21h）
- embed 产物缺失（两 exe chunk 清单完整）
- GPU（`gpuDisabled: true` 已生效仍挂；gpu-process 存活）
- 磁盘满（C 45G / D 176G 余量）
- 后端 Go 卡死（startup 完整，进程健康）

**深层机理**：未解剖（恢复优先，现场随进程终止丢失）。候选：Chromium 157 长会话 renderer/Storage Service 死锁（同步 IPC 等待），WebView2 上游问题。

**取证纠偏记录**（防再犯）：
1. `GetAndClearLastCrashFlag`（app_external.go:185）只翻转内存布尔不删文件；`crash.flag` 残留 = 未正常 shutdown 的正常运行态，**不构成前端故障证据**
2. 正常使用期 app.log 零业务日志是常态，不能据「started 后无日志」推断「启动即死」——必须结合用户证词与快照 mtime 交叉定位猝死时刻
3. `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port` 在 WebView2 157（Chromium 136+）下无效，勿再走此路；取 renderer 现场应改用 `procdump -hang`

## 遗留决策（待用户）

- 是否实现 watchdog 自愈（Go 侧周期 RuntimeEvaluate 探活 + 自动 Reload）——见 troubleshooting 条目「永久解法」；若选观察策略，复发时先 `procdump -hang workbench.exe` 留现场再杀

## 验收标准完成度

- [x] 应用恢复可用
- [x] 根因定性到实证层（renderer 主线程挂起）；深层挂点明确记录为「现场丢失未解剖」，非含糊推断
- [x] 排障沉淀：docs/agent-resources/troubleshooting/webview2-blank-freeze.md + 索引登记（新增「运行时故障」分类）

## 技术笔记

- 取证源：`data/logs/app.log`、`data/crash.flag`、`data/session.json`（savedAt=1794539417=11:10:17）、`%APPDATA%\workbench.exe\EBWebView\Default\{settings,favorites}_diagnostic.log`、Local Storage leveldb mtime、Win32_Process CreationDate
- 版本锚：运行版 = 13:50 构建（≈dc47e4d）；恢复运行版 = 17:29 构建（≈0009e48）
- 临时文件：`D:\Desktop\wb-restart-check.png`（重启验证截屏，可删）
