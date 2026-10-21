# go-git v6 升级评估与执行

## Goal

security-scan.md §5.4 收尾项：go-git v5.19.2 → v6 major 升级评估与执行。升前逐条核对 breaking changes 与消费面，按评估胜出路线执行（改 v6 API 或消依赖移除），回归全绿后沉淀。允许结论为「跳过留痕 / 降级观察」，不阻塞收尾。

## What I already know

* go.mod：`go 1.26.0` + `toolchain go1.26.10`；直接依赖 `github.com/go-git/go-git/v5 v5.19.2`；`go-billy/v5 v5.9.1` 为 indirect
* 消费面 6 文件（grep 已确认，与任务描述一致）：
  * `app_git.go`（import go-git / plumbing / object）
  * `service/git.go`（go-git 主包）
  * `service/dashboard.go`（go-git / plumbing）
  * `app_git_stats_test.go`、`app_git_cache_test.go`、`commit_history_integration_test.go`（测试消费）
* 历史背景：`commit_history_cli.go` 采集层已 CLI 化替代 go-git 逐条树 diff（注释留档）；`service/git.go:553` remote 读取刻意用 go-git（读 .git/config 免子进程开销）
* 前端四 major 已出清（batch3，2026-10-21 归档）；本节剩 go-git v6 与 wails v3（beta 不立项）
* 上轮升级先例（§5.1）：GO-2026-6214/5496 经 v5.18.0 → v5.19.2 minor 修复

## Requirements

1. **升前评估**：
   * go-git v6 changelog / breaking 逐条核对（module path、API 迁移面、go.mod 最低 Go 版本与 toolchain 联动）
   * grep 六消费文件出 API 消费清单，落 `research/`
   * 逐消费点盘点：可改走既有 util 层 exec 封装（消依赖）vs 必须留库调用（改 v6 API），两路线分别给工作量与风险
2. **升级执行**：按评估胜出路线执行；期间不动 util 层 exec 封装语义（超时收口 / 锁残留探测 / E_GIT_TIMEOUT 链路逐字保持）
3. **回归**：`go test ./...` 全量 + `-tags=integration` + `govulncheck ./...` 清零核对 + coverage 门禁（model/server ≥80% + service ≥76%）不回退；soak 档按需
4. **沉淀**：security-scan.md §5.4 同步（go-git 出清后仅剩 wails v3 跟踪项）；消费面坑按 trellis-update-spec 沉淀；README.md 按需确认

## Acceptance Criteria

* [x] research/ 落消费面 API 清单 + v6 breaking 核对结论
* [x] 两路线（消依赖 vs 改 v6 API）工作量与风险有对比结论，胜出路线有判据
* [x] 按胜出路线执行完成（或明确「跳过留痕/降级观察」结论落 security-scan.md）
* [x] `go test ./...` 全绿
* [x] `go test -tags=integration` 全绿
* [x] `govulncheck ./...` 清零
* [x] coverage 门禁不回退（model/server ≥80%，service ≥76%）——实测 model/server 94.2%、service 80.1%、util 65.7%
* [x] security-scan.md §5.4 更新（仅剩 wails v3）
* [x] 消费面坑沉淀 docs/spec/（docs/spec/go-git-deprecation.md + CLAUDE.md 关键规则 + spec/README 索引）
* [x] README.md 按需确认（内部依赖出清，用户向功能文档无涉及，不更新）

## Definition of Done

* 测试/回归全绿（含 integration 与 coverage 门禁）
* security-scan.md 与实际依赖状态一致
* spec 沉淀完成（如产生新契约）

## Out of Scope

* wails v3（beta，持续跟踪不立项）
* util 层 exec 封装语义变更（超时收口/锁残留/E_GIT_TIMEOUT 链路冻结）
* 其他依赖升级（本任务仅 go-git）

## Technical Notes

* 关键规则约束：Go 依赖升级后须重跑 govulncheck 确认清零（security-scan.md）
* 测试稳定性：禁隐式依赖环境外部状态（固定端口/mtime 时序）
* util 层 exec 封装见 `util/`（ExecuteWithContext 等，取消链路 2026-09-29 收口）

## Research References

* `research/go-git-v6-migration.md` — 待研究产出
* `research/consumer-api-inventory.md` — 待研究产出

## Open Questions

* 胜出路线判定（待研究完成后按工作量/风险对比定，用户已授权按评估结果执行）
