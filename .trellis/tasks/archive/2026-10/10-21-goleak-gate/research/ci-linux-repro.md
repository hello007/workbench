# CI Linux 后端测试失败复现与观察期核查（2026-10-21）

## 复现环境

- WSL Ubuntu + Go 1.26.6 linux/amd64（~/toolchain/go），GOPROXY=goproxy.cn，GOFLAGS=-buildvcs=false
- 代码：本地 master HEAD（含 00c332a 依赖分档提交）
- 口径：`go test ./...`（CI test job「后端单元测试」step 同口径）

## 复现结果（EXIT=1，service 包 FAIL）

### 失败 1：TestAssetLibrary_ShutdownCancelsInFlightSplit

```
assets_split_test.go:847: WaitSplitsDone 应等待拆分退出后返回（顺序 [batch wait]），实际 [wait batch]
```

期望 Shutdown 链路顺序：batch（拆分批完成）先于 wait（WaitSplitsDone 返回）。Linux 实际 wait 先返回。Windows 本机稳定绿。定性候选：真实顺序保证缺失（Shutdown 链竞态）或测试断言依赖调度时序。

### 失败 2：TestSearchWithRipgrep_NoMatchUnchanged

```
content_search_test.go:346: rg stub 未按预期执行（stdout="" exit=1），TEMP 路径含空格等环境会使 .bat 截断失效
```

Windows 专属 rg.bat stub 在 Linux 失效（错误信息自述）。平台适配缺口：stub 应按 GOOS 分流或测试 skip。

### goleak 泄漏告警（观察模式输出，Linux 侧首次暴露）

8 个 goroutine 泄漏，全部同形态：

```
workbench/service.(*thumbQueue).run 卡在 select（assets_thumbs.go:218）
created by (*thumbQueue).start（assets_thumbs.go:204）
```

- 8 个不同 thumbQueue 实例（8 个测试各建一 queue）未 Shutdown 收口 worker
- Windows 本机全量跑 stderr 零告警（观察模式）；深跑档硬门禁历史绿均在 Windows——**Linux 侧此前从未跑过硬门禁**
- 相关线索：assets_thumbs_test.go:98 有「Cleanup 收口 flusher goroutine」先例，部分测试收口部分漏收口

### Windows/Linux 差异说明

COM 队列 Windows only（linux-platform 规则），thumbQueue 在 Linux 走无 COM 分支（exporter 不可用），泄漏面为 queue 生命周期管理，与 COM 无关。

## CI 侧核对（github.com/hello007/workbench，镜像仓）

- run 11（2026-10-13，head 320ab082）test job「后端单元测试」FAIL；security job govulncheck FAIL
- run 12（2026-10-20，head 19910fcd）test job「后端单元测试」FAIL；security job npm audit FAIL
- run 10（2026-09-28，head cac805e3）success——CI 上一次绿
- 两次红 head 均为本地 master 祖先（镜像手动同步制，滞后于本地 HEAD）
- 日志正文不可达（未认证）；step 级结论足以定位挂点

## 观察期核查结论

- Windows 侧：本机全量绿 stderr 零告警 + 深跑档 2000 轮硬门禁历史绿 ✓
- Linux 侧：**存在真泄漏**（thumbQueue 8 实例）——两步走前提「观察期无新告警」在 Linux 不成立，收紧前必须先修泄漏
- 结论：修泄漏 + 修 2 个平台失败 → WSL 复验全绿 → 收紧硬门禁（预期收紧后白名单为空或极少）

## 修复结果回填（2026-10-21，收口任务执行后）

三个失败 + 一个新增发现全收口，Windows/Linux 双侧证据链闭合：

### 失败 1 修复：thumbQueue 8 实例泄漏（测试收口缺口，非生产队列缺陷）

- 定位：`newThumbTestService`（assets_thumbs_test.go:99）只 `t.Cleanup(s.thumbLedger.Shutdown)`，未收口惰性启动的 queue worker——`EnqueueAssetThumbs`/`JumpToSlide` 触发 `q.start()` 后 run goroutine 驻留。9 个 available=true 的队列测试中 8 个漏收口（`TestThumbQueue_WaitThumbsDone` 自收口），与本文件 8 实例告警逐一对上。
- 修复：单点 Cleanup 改 `s.WaitThumbsDone`（worker 退出 → exporter.Shutdown → ledger.Shutdown 全链幂等收口）。
- **新增发现（硬门禁生效后确定性暴露）**：收紧后 Windows 首跑即红——`(*TerminalService).watchProcess` 驻留（观察期 perf-baseline 14.3 登记过的同一形态，此前「复跑未复现」）。根因：`TestTerminalService_CreateTerminal_InvalidShellFallback` 在套件尾部创建真实 conpty，`PtyProcess.Close()` 只断 ConPTY 句柄不杀交互 shell 进程，watchProcess 仅依赖 `IsProcessRunning` 翻转自证退出 → 永驻。定性为**生产代码生命周期缺口**（watch goroutine 不感知会话关闭），非测试清理遗漏：修复 = `TerminalService` 加会话级停止/确认双 chan（`watchDones` + `watchExits` per-session map；审查发现共享 WaitGroup 形态下双 tab 关其一的等待会被在册会话 watch 永久阻塞，已改定），`CloseTerminal`/`CloseAll` 锁内摘除会话条目后锁外 close(done) 并 `<-exit`（关闭返回时该会话监视 goroutine 已收口，对齐 `WaitThumbsDone` 形态）；watchProcess 保留会话存在性检查兜底 close 前窗口。该修复对 Linux 无行为影响（Linux pty 断开即杀进程），Linux 全量绿佐证。

### 失败 2 修复：rg stub 平台失败（分流后探针未适配，非 stub 结构缺陷）

- 复核：`writeExitOneRgStub` 已按 GOOS 分流（.bat / .sh + 0o755），失败根因是 Unix 分支 stub 只 `exit 1` 不打印探针标记，而 `probeRgStub` 断言 stdout 含 `ok`——Linux 上探针恒红，属分流遗漏非结构缺陷。
- 修复：Unix 分支 stub 补 `echo ok`（两侧探针语义一致：stdout 标记 + exit 1）。Windows 侧 TEMP 空格隐患维持既有探针甄别口径不变。

### 失败 3 修复：ShutdownCancelsInFlightSplit（断言锚点时序脆弱，非生产顺序缺陷）

- 判定依据：生产 join 契约的可断言面止于 `splitWG.Done()`（`WaitSplitsDone` 在计数归零后返回）；原断言锚点 `record("batch")` 发生在 `SplitAssetLibrary` 返回之后，而 `Done()` 是函数内 defer——「Done 后测试 goroutine 记录」与「Wait 侧记录」之间无任何同步，先后纯靠调度（Windows 碰巧 batch 先，Linux 翻转即红）。生产顺序保证成立，测试锚点晚于 join 点。
- 修复：锚点前移至核心退出路径（`splitCoreFn` 闭包内 `blocker.split` 返回后同步 `record("core-exit")`，先于生产链 `splitOneFileWithHash` 返回与 `Done()`），断言改 `[core-exit, wait]`；`batchDone` 保留承担取消错误码断言。

### 收紧与自证

- TestMain 两档：Short 跳过 / 默认档与深跑档统一 `goleak.VerifyTestMain`（空白名单），文件头注释同步改写。
- 注入自证：临时 `select{}` goroutine → `go test ./service/ -run TestLeakGateSelfCheckTmp` EXIT=1 且打印泄漏栈（已还原）。
- 复验：WSL Linux `go test -count=1 ./...` EXIT=0 stderr 零 goleak 告警；Windows `go test ./...` 与 `go test -short ./service/` 全绿；`go vet ./...` 绿。
