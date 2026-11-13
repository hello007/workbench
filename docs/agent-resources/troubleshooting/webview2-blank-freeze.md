# WorkBench 白屏假死（运行中前端猝死，重启即恢复）

| 项 | 值 |
|----|----|
| 分类 | 其他（运行时故障） |
| 创建时间 | 2026-11-13 |

## 现象

桌面版 WorkBench 窗口打开后内容区纯白（标题栏按钮仍可点击），形似卡死；点任务栏图标无响应。实际为**前一会话进程长期运行后前端猝死**：后端 Go 进程、WebView2 全部子进程（browser/renderer/gpu/network/storage）均存活，但 renderer 无任何输出。2026-11-13 实测一例：进程运行约 21 小时后于 11:10 猝死，CPU 时间 21 小时仅 16 秒（纯挂起非死循环），无 Crashpad dump。

## 原因

定性（实证到层）：**WebView2 renderer 主线程同步阻塞类挂起**，导致 UI 冻结、合成器停止出帧，窗口重绘为白。判定依据：

- renderer 进程未重建（创建时间仍为会话启动时刻）→ 非渲染进程崩溃
- 会话启动与猝死之间 WebView2 Evergreen 版本未变、exe 未变，且同一构建版在猝死前已正常使用数小时 → 非代码/版本直接引发
- 最后一次成功写 `data/session.json` 与 localStorage leveldb 时间戳相同（猝死时刻），之后 Go 侧无任何前端触发的日志 → 前端对后端调用链整体停摆
- `data/crash.flag` 残留 = 进程未走正常 shutdown（`GetAndClearLastCrashFlag` 只翻转内存布尔，flag 文件仅由 shutdown 删除，**残留本身不构成前端故障证据**）

更深层机理（renderer 挂在哪个调用上）因恢复优先、进程已终止，未解剖。候选：Chromium 157 长会话 renderer/Storage Service 死锁（同步 localStorage/IPC 等待），属 WebView2 运行时上游问题。

## 临时解法（agent 行动手册）

杀假死进程树并重启（`data/` 目录原子写，无损）：

```bash
# 1. 确认假死进程（内存远低于正常值 ~165MB，CPU 时间近乎不增长）
tasklist //FI "IMAGENAME eq workbench.exe" //V

# 2. 杀进程树（/T 连带 WebView2 子进程）
taskkill //F //T //IM workbench.exe

# 3. 从安装目录重启（cwd 必须是安装目录，data/ 为相对路径）
cd "/d/Program Files/WorkBench" && ./workbench.exe &
```

已验证：2026-11-13 重启后正常（新进程内存 165MB，界面完整）。下次启动日志出现 `last session exited abnormally (crash flag detected)` 属预期（正确识别上次异常退出）。

## 永久解法（待人为决策）

- 方案 1：前端心跳 watchdog——Go 侧周期 `RuntimeEvaluate`（如每 60s evaluate `1`）超时无响应则自动 `window.Reload()` 或提示用户重启；能自愈同类挂起。
- 方案 2：仅观察——若复发频率低（首例），不动代码，复发时保留进程现场（procdump -hang workbench.exe）解剖挂点后再定。
- 附注：排查时注入 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222` 无效（WebView2 157 / Chromium 136+ 对该参数有限制），勿再走此路；取 renderer 现场改用 procdump -hang。
