# Windows CI 覆盖 split 折叠分支

## Goal

在 GitHub Actions CI 增加 `windows-latest` 后端测试 job，覆盖 `runtime.GOOS == "windows"` 的大小写折叠分支（此前仅 ubuntu runner，属已知覆盖缺口）。不改现有 test/security job 行为。

## Requirements

- R1：`.github/workflows/ci.yml` 新增独立 job（建议 id `test-windows`，`runs-on: windows-latest`），与既有 `test`/`security` 并行。
- R2：job 步骤最小集：checkout → 预置 `frontend/dist` 占位（满足 `//go:embed all:frontend/dist`）→ `actions/setup-go@v5`（`go-version: '1.26'`，默认缓存）→ `go vet ./...` → `go test ./...`。
- R3：不引入 Node/Playwright/Wails CLI；不带 `desktop,production` 标签（无 GTK/WebKit 依赖）。
- R4：清理已知缺口表述：`docs/spec/pptx-split.md` 中「Windows 折叠放行分支仅 Windows 本机可执行（CI 仅 ubuntu），属已知覆盖缺口」改为「由 CI `windows-latest` job 覆盖」。
- R5：若 `docs/测试策略.md` 列出 CI job 清单，同步补充 Windows job。
- R6：YAML 语法校验通过；本地 Windows 复跑 job 等价命令全绿。

## Acceptance Criteria

- [ ] `ci.yml` 含 `windows-latest` job，触发器与权限不变。
- [ ] 该 job 在 push(master) / pull_request 时执行 `go vet ./...` + `go test ./...`。
- [ ] `docs/spec/pptx-split.md` 不再声明「CI 仅 ubuntu」缺口。
- [ ] `python -c "import yaml;yaml.safe_load(open('.github/workflows/ci.yml'))"` 通过。
- [ ] 本地 Windows 执行 `go vet ./...` 与 `go test ./...` 通过。

## Definition of Done

- 仅改 CI 与文档，不改业务代码。
- 现有 `test`/`security` job 零改动（除文件头注释若需补 Windows 说明）。
- 文档同步；提交与归档。

## Decision (ADR-lite)

**Context**：Windows 折叠分支为数据安全关键路径（大小写变体产物让位），但 CI 仅 ubuntu，Windows 语义只能本机手测。

**Decision**：新增独立 `windows-latest` job 跑全量后端单测（而非只跑 split 子集）。理由：Windows 平台差异面不止 split（pty/路径/剪贴板/文件操作），全量单测成本可接受且能防其他平台回归；不跑前端/E2E（E2E 在 ubuntu 已覆盖，Windows 浏览器环境收益低）。

**Consequences**：CI 时长增加约 5–8 分钟；无 Office 环境，若存在依赖真实 COM 的用例需确认其跳过逻辑（本地验证 + 首跑观察）。集成测试（`-tags=integration`）暂不纳入 Windows job，避免范围扩大。

## Out of Scope

- Windows 跑前端构建/E2E。
- Windows 集成测试 job。
- release.yml 改动。

## Technical Notes

- `ci.yml` 现有 job：`test`（ubuntu，全链路）、`security`（ubuntu，govulncheck/npm audit）。
- `frontend/dist` 与 `frontend/wailsjs` 均 gitignore；Go 测试只需 dist 目录存在。
- Windows runner 默认 shell 为 pwsh；占位目录建议 `shell: bash` 复用 POSIX 命令。
- 文档引用：`docs/spec/pptx-split.md:184`、`docs/测试策略.md`（若含 CI 清单）。
