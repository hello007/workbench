# Linux 改造审核缺陷修复

## Goal

修复 2026-09-23 三路并行代码审核（后端/CI/前端文档）对 Linux 改造（09-22-linux 任务，提交 e5ed0a3..de948bf）发现的 10 个应修级（🟡）缺陷。零 🔴。审核建议级（🟢）本任务不修（记录于文末备查）。

## Requirements（修复清单）

### 后端（4）

**R1 shell 注入 — service/update.go（buildUpdateSh/buildApplySh）**
- 问题：脚本用双引号直插路径，安装路径含 `"` 或 `$()` 时脚本破坏或以当前用户身份执行任意命令。
- 修复：路径统一单引号包裹 + `'\''` 转义（`service/terminal.go` buildPosixCdCommand 已有同款惯用法，复用或对齐）。仅 Linux 分支脚本，Windows .bat 不动。

**R2 mv 失败假更新 — service/update.go（同两脚本）**
- 问题：`mv -f` 失败（如安装目录只读）仍继续 `rm -rf updateDir` + 删 pending 标记 — 新二进制被删、旧版本静默继续、用户以为已更新。
- 修复：mv 后 `|| exit 1`（或 `|| { echo ...; exit 1; }`），清理与重启放成功分支。两脚本（update/apply）都要。配测试断言关键行。

**R3 解压炸弹 — service/update_extract.go:55**
- 问题：`io.Copy(out, tr)` 无上限，超大成员可写满磁盘；注释声称按不信任输入处理但缺实施。
- 修复：`io.Copy` 包 `io.LimitReader`，上限取 GitHub release asset FileSize（若下载链路可拿到）或保守常量（如 512MB），超限返回错误。配 linux tag 测试（构造超小上限用例验证截断报错）。

**R4 file URI round-trip 破损 — util/clipboard_payload.go:67**
- 问题：`pathToFileURI` 对 Windows 盘符路径产出 `file://C:/a/b`（`net/url` Host 为空时不补第三斜杠），`fileURIToPath` 解析回 Host="C:" 直接拒绝，round-trip 破损；现有「Windows 风格路径」测试用 `filepath.Join` 构造根相对路径绕开盘符场景，未覆盖声称行为。
- 修复：路径不以 `/` 开头时前置 `/`（产出 `file:///C:/...`），解析侧对应兼容；补盘符路径 round-trip 测试用例（`C:\Vault\note.md` → URI → 还原）。

### CI/发布（2）

**R5 release 首建竞态 — .github/workflows/release.yml:141**
- 问题：release-windows 与 release-linux 并行，新 tag 首次发版时两 job 各自调 action-gh-release，「同时检测不存在→并发 create」竞态窗口，失败方资产缺失且自更新客户端查不到 Linux 资产。
- 修复（最小改动）：`release-linux` 加 `needs: release-windows` 串行化。评估并记录 Linux 失败时「Release 已建缺资产可 Re-run」语义不变。

**R6 ubuntu-22.04 runner EOL 定时炸弹 — release.yml:76**
- 问题：GitHub 惯例 OS EOL 前提前退役 runner 镜像（20.04 先例），22.04 EOL 2027-04，届时 release-linux 无可用 runner；glibc 基线动机（兼容老发行版）未留迁移路径。
- 修复：评估 `container: ubuntu:22.04` 方案（glibc 基线与 runner 镜像解耦，24.04 镜像跑 22.04 容器同产 2.35 基线产物）——可行则直接采用并注释；有阻塞（apt 源/wails 依赖在容器内的已知坑）则保留 22.04 runner 并加迁移期限注记（EOL 前必须迁移）。

### 前端（2）

**R7 UA 判平台方向性错误 — frontend/src/components/TerminalPanel.vue:184**
- 问题：`fallbackTerminalDir()` 用 `navigator.userAgent` 判平台 — serve 模式 PTY 跑在服务端主机、UA 是浏览器端：Windows 主机 + Mac/手机浏览器 → 兜底 `'/'` 传给 Windows PTY（cd 失败落 cwd）；反向 Linux 主机 + Windows 浏览器 → `'C:\'` 同坏。
- 修复：平台判定改后端信号 — 复用已加载的 `GetShellConfigs`（settings store 链路）：首项 type 为 `powershell` 即 Windows 主机（Windows 分支仍回 `'C:\\'`），否则 `'/'`。UA 检测移除或仅作 RPC 未就绪时兜底（评估后择一，保持行为简单）。配测试（含 Windows/Linux 主机两分支 + serve 语义注释）。

**R8 老用户 shell 设置值跨平台残留 — frontend/src/components/SettingsPanel.vue:804**
- 问题：settings.json 存 `gitbash`/`wsl`（Windows 时期设置）在 Linux：el-select 无匹配 option 显示裸值；`v-if="defaultShell === 'gitbash'"` 误渲染 Windows Git Bash 路径配置项；TerminalPanel tab 副标签回退裸「gitbash」。后端 ResolveShellConfig 兜底 bash 不崩，但 UI 全链路混乱，无测试覆盖。
- 修复：settings store 加载时校验 `defaultShell` 不在 `shellConfigs` 列表 → 改取首项（与三层兜底链正交，落入既有 loadDefaultShell 链路最自然——评估实现位置，保证 SettingsPanel 显示值、条件渲染、tab 副标签、实际 PTY shellType 四处一致）。配测试：存量 gitbash 设置 + Linux 列表 → 全链路收敛 bash。

### 文档（2）

**R9 依赖边界矛盾 — docs/开发工作流.md:27**
- 问题：「跑 go test ./... 须先装 GTK/WebKit」与 docs/spec/linux-platform.md §3 实证口径（后端四子包无需 GTK；主包无标签不触桌面 CGO 亦无需；带 desktop,production tag 才需要）矛盾。
- 修复：按 spec §3 口径改写本文（装依赖的判定以 spec 为单一事实源，本文链接过去不重复展开）。

**R10 运行库包名错误 — docs/部署说明.md:95**
- 问题：`apt install libgtk-3 libwebkit2gtk-4.1` 包名不存在（Ubuntu/Debian 无此名）。
- 修复：改运行时包 `libgtk-3-0` / `libwebkit2gtk-4.1-0`（开发包 `-dev` 与运行包区分表述）。

## Acceptance Criteria

- [ ] R1-R4 后端修复全部落地且各配测试（注入转义、mv 失败分支、解压上限截断、盘符 round-trip）
- [ ] Windows 侧 `go build ./...` + `go test ./service/ ./util/` 全绿（零回归）
- [ ] WSL 侧 `go build ./...` + `go test ./service/ ./util/` 全绿
- [ ] R5 release.yml 串行化，YAML 语法通过，Linux job 失败可单独 Re-run 语义保留
- [ ] R6 完成容器方案评估并有结论（采用或注记），产出可追溯
- [ ] R7-R8 前端修复落地，`cd frontend && npm test` 全绿，新增用例覆盖 Windows/Linux 主机两分支与存量设置迁移
- [ ] R9-R10 文档修正，与 linux-platform.md §3 口径一致
- [ ] 10 项逐条对照审核报告原文复核关闭

## Definition of Done

- 单测覆盖每个修复点（后端 4 + 前端 2）
- 双平台测试绿 + 前端测试绿
- YAML 语法自查通过
- 审核发现逐条闭环（本文档 AC 勾选）

## Out of Scope

- 审核建议级（🟢）12 项：fish cd -- 实测、arm64 提示、PID 复用、占位符平台化、DIFF_TOOL_PRESETS 平台化、apt 缓存、ci.yml 头注释、恒真断言清理等 — 择机另行处理
- macOS 支持

## Technical Notes

- 审核来源：三路并行 cavecrew-reviewer（2026-09-23），后端 0🔴4🟡5🟢 / CI 0🔴2🟡2🟢 / 前端文档 0🔴4🟡5🟢
- 前任务沉淀：docs/spec/linux-platform.md（R9 以其为单一事实源）
- terminal.go buildPosixCdCommand 的单引号转义惯用法可复用于 R1
