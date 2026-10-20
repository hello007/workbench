# goleak CI 观察期收口（观察模式切硬门禁）

> 父任务：10-21-hardening-batch（顺序 1，最先执行）

## Goal

执行 goleak 两步走策略第二步：观察模式切 VerifyTestMain 硬门禁。**brainstorm 复现发现 Linux 侧三问题（1 真泄漏 + 2 平台失败测试），作为前置一并收口**——收紧的前提是 Linux 侧先绿。

## Requirements（按序）

1. **修泄漏**：`(*thumbQueue).run` 8 实例未收口（assets_thumbs.go:204/218，详证 research/ci-linux-repro.md）——定位 8 个漏收口测试，补 Cleanup 收口（对齐 assets_thumbs_test.go:98 先例）；若泄漏面在生产代码（queue 生命周期缺 Shutdown 入口）则修生产代码
2. **修平台失败 1**：`TestSearchWithRipgrep_NoMatchUnchanged`（content_search_test.go:346）——rg.bat stub Linux 失效，stub 按 GOOS 分流（.sh）或测试平台 skip（选型看 content_search_test.go 既有 rg stub 结构）
3. **修平台失败 2**：`TestAssetLibrary_ShutdownCancelsInFlightSplit`（assets_split_test.go:847）——先读 WaitSplitsDone/Shutdown 实现判定：真实顺序保证缺失（修生产代码）vs 断言时序脆弱（修测试）
4. **WSL 复验**：Linux 全量 `go test ./...` 绿（含 stderr 零 goleak 告警）；Windows 本机回归绿
5. **收紧硬门禁**：TestMain 默认档与深跑档统一 VerifyTestMain（同一白名单，预期白名单为空）；`-short` 跳过不变
6. **收紧自证**：临时注入良性 goroutine 验证硬门禁能红（本机一次性验证，不入库）
7. **文档同步**：perf-baseline.md 第 13/14 节 + CLAUDE.md 关键规则行（观察期 → 已收口，登记 Linux 泄漏教训）+ soak_leak_test.go 文件头注释改写

## Acceptance Criteria

- [ ] 三个失败全修复，WSL Linux 全量 `go test ./...` 绿且 stderr 零 goleak 告警
- [ ] Windows 本机全量回归绿
- [ ] 硬门禁生效 + 注入验证能拦
- [ ] 文档三处同步
- [ ] research 核查结论完整（Windows/Linux 双侧证据链）

## Definition of Done

- AC 全勾；文档更新；WSL + Windows 双侧绿

## Decision (ADR-lite)

**Context**：两步走约定「观察期无新告警后收紧」，但 Linux 侧此前从未跑过硬门禁——镜像仓 CI 观察模式从不暴露泄漏（观察不改退出码且日志无人看），观察期 26 天实为「未观察」。WSL 复现证明 Linux 有真泄漏。
**Decision**：收紧前置 = 先修 Linux 泄漏与平台失败（修完收紧时白名单预期为空）；WSL Linux 本地复现作为 CI Linux 等价证据通道（go test 口径一致）；镜像仓滞后不治理（另行处理）。
**Consequences**：CI Linux 测试自本次起可信；goleak 三档简化为两档（硬门禁/short 跳过）；后续 Linux CI 若暴露新泄漏即时红（安全网内建）。

## Out of Scope

- 主包泄漏门禁调整（16MB 口径不变）
- soak 轮次/阈值调整
- 镜像仓同步机制治理
- security job 红点（govulncheck/npm audit 信息性扫描，continue-on-error 不阻塞）

## Research References

- [research/ci-linux-repro.md](research/ci-linux-repro.md) — WSL 复现全套证据 + CI run 核对 + 观察期结论

## Technical Notes

- TestMain 收紧后结构：深跑档/默认档统一 VerifyTestMain，short 档 os.Exit(m.Run()) 跳过（soak_leak_test.go:77-100）
- thumbQueue 泄漏栈形态：run 卡 select（:218），created by start（:204）；8 实例 = 8 测试各建一 queue
- rg.bat stub 失效信息自带解释（TEMP 空格截断）——Windows 下 TEMP 带空格也可能踩，修法须同时治 Windows 隐患
- WSL 复现命令形态：PATH=~/toolchain/go/bin、GOPROXY=goproxy.cn、GOFLAGS=-buildvcs=false
