# 审核建议级问题修复（🟢 12 项）

## Goal

修复 2026-09-23 三路审核遗留的 12 个建议级（🟢）问题。全部为体验优化、健壮性增强与测试质量提升，无行为级缺陷。前序：R1-R10 应修项已闭环（eef2951）。

## Requirements

### 后端（5）

**G1 换行符路径防护 — service/terminal.go buildPosixCdCommand**
- 路径含 `\n` 时（Linux 合法文件名）单引号包裹无法阻止 PTY 行分割，cd 截断后剩余片段被当命令执行。
- 修复：检测到 `\n`/`\r` 时拒绝生成 cd 命令（返回错误提示或降级为不含路径的安全形式——读现有函数签名决定，倾向报错路径不合法）。

**G2 fish `cd --` 兼容 — service/terminal.go**
- fish 对 `cd --` 的 POSIX 处理未验证，四类 POSIX shell 共用一条命令格式有风险。
- 修复：先查证（fish 官方文档/源码，fish 的 cd 为 builtin，3.x 版本对 `--` 的处理）——查证支持则留注释记录依据；不确定则防御：shellType=fish 时省略 `--`（`cd 'path'` 形式），其余 POSIX shell 保持 `cd -- 'path'`。结论写入注释。

**G3 arm64 误导报错 — service/update.go updateAssetName/CheckForUpdate**
- linux/arm64 下仍返回 amd64 资产名，CheckForUpdate 报「未找到可下载的更新文件」无法区分架构不支持。
- 修复：资产名按 `runtime.GOARCH` 组装（`workbench-linux-amd64.tar.gz` / arm64 同构），CheckForUpdate 未命中资产且 GOARCH 非 amd64 时报「暂不支持 <GOOS>/<GOARCH> 架构的自动更新」明确提示。Windows 路径不变（GOARCH 同理平台化或仅 Linux 分支处理——读代码后择小改）。

**G4 kill -0 轮询 PID 复用误杀 — service/update.go buildUpdateSh**
- 旧进程退出后 PID 秒级被无关进程复用，kill -9 误杀。
- 修复：轮询循环内加进程身份校验——`grep -q workbench /proc/$PID/cmdline`（读现有脚本结构，在 kill -9 前校验 cmdline 含 workbench 才杀；非 Linux 或 /proc 不可用时保守跳过校验直接按原逻辑）。注意保持 POSIX（/proc 是 Linux 特性，脚本本身仅 Linux 执行，可用）。

**G5 平台断言恒真 — service/update_test.go TestUpdateAssetName_MatchesPlatform 等**
- 期望值用与实现相同的 GOOS 条件取同一常量，分支同构恒真，只验证常量绑定不验证行为。
- 修复：改为平台无关断言——Linux 分支断言 `strings.HasSuffix(name, ".tar.gz")` && `strings.Contains(name, "linux-")`；Windows 分支断言 `strings.HasSuffix(name, ".exe")`；BinaryPath 断言 Base 名与资产解包约定一致。G3 改造后断言同步。

### CI（2）

**G6 apt 依赖缓存 — .github/workflows/ci.yml**
- libgtk-3-dev/libwebkit2gtk-4.1-dev 全依赖链数百 MB 每次 run 重装（test job），约增 1-2 分钟。
- 修复：引入 apt 缓存（eg：`awalsh128/cache-apt-pkgs-action@latest` 或等效成熟 action——选维护活跃、广泛使用的；缓存 key 含包名列表 hash）。若评估风险（缓存污染/过期维护不佳）超过收益则记录结论保留现状。release.yml 不加（发版 job 低频且容器方案已解耦，缓存 action 对 container job 的兼容性存疑——核实后定）。

**G7 ci.yml 文件头注释过时 — .github/workflows/ci.yml:2**
- 头注释仍写「release.yml（windows-latest 发版）」，实际已双平台 + 容器。
- 修复：同步为当前事实（release 双 job、ubuntu-22.04 容器、webkit2_41、tar.gz 产物）。

### 前端（5）

**G8 DirectoryTree 占位符 — frontend/src/components/DirectoryTree.vue:123**
- Linux 下输入框占位符显示「例如： C:\workspace」。
- 修复：复用既有平台信号（TerminalPanel 的 shellConfigs 首项模式或 store 缓存——读组件数据流择一），Windows 保持现文案，非 Windows 显示中性示例（如 `/home/user/projects`）。实现方式与 R7 的判定来源保持一致（禁 UA）。

**G9 SettingsPanel 占位符 — frontend/src/components/SettingsPanel.vue:63,81**
- obsidianPath（63 行）/diffToolPath（81 行）占位符为 Windows 专属路径。
- 修复：同 G8 平台信号：Windows 保持，非 Windows 用对应平台示例（obsidianPath：`/opt/Obsidian/obsidian` 之类真实可行路径；diffToolPath：`meld`）。占位符仅提示不影响功能。

**G10 DIFF_TOOL_PRESETS 平台化 — frontend/src/store/settings.js:114**
- Beyond Compare/WinMerge 预设路径均为 Windows 专有，Linux 选中即写入不存在路径。
- 修复：预设表按平台信号分支：Windows 保留现表；非 Windows 提供 meld/kdiff3 等常见 Linux diff 工具预设（路径用 `command -v` 可命中的裸命令名或标准安装路径）。SettingsPanel 下拉自动随表。

**G11 SettingsPanel spec 脆断 — frontend/src/components/__tests__/SettingsPanel.spec.js:243**
- 新增用例断言 `wrapper.vm.$.setupState` 内部状态而非渲染 DOM，'fail' 用例无 DOM 断言，setupState 结构变化即脆断。
- 修复：改断言渲染结果（`findAll('.el-select-dropdown__item')` 文本列表），'fail' 用例补 DOM 层断言（残留值不出现/收敛值出现）。

**G12 FALLBACK_SHELL 恒真断言 — frontend/src/store/__tests__/settings.spec.js:71**
- `expect(FALLBACK_SHELL).toBe('powershell')` 导入常量与字面量恒真，不构成行为验证。
- 修复：删除该行或并入上一行 defaultShell 断言语义（断言「列表不可用时最终解析值」而非常量自身）。

## Acceptance Criteria

- [ ] G1-G5 后端落地：换行路径拒绝/降级、fish 结论有据（查证或防御分支）、arm64 明确报错、kill -9 前身份校验、平台断言非恒真；各配测试
- [ ] G6 完成缓存方案评估与实施（或记录保留现状结论）；G7 头注释更新
- [ ] G8-G10 前端平台化落地，判定来源与 R7 一致（禁 UA）；G11/G12 测试质量修复
- [ ] Windows `go build ./...` + `go test ./service/ ./util/` 全绿；`cd frontend && npx vitest run` 全绿
- [ ] WSL `go build ./...` + `go test ./service/ ./util/` 全绿
- [ ] Windows 用户可见行为零变化（所有平台分支 Windows 侧保持原文案/原预设/原路径）

## Definition of Done

- 双侧测试绿 + 前端绿
- 12 项逐条对照本 PRD 关闭
- G2/G6 两处「查证/评估型」任务有明确结论记录（注释或 PRD 勾选处）

## Out of Scope

- CI 推送触发与首跑终验（待推送）
- pdfjs-viewer 遗留（已处置）

## Technical Notes

- 来源：2026-09-23 三路审核 🟢 项汇总（后端 5 / CI 2 / 前端 5）
- R7 既有平台信号模式：TerminalPanel shellConfigs 首项判定（G8-G10 复用）
- R1 既有 shellQuote 惯用法（G1/G4 相关脚本改动对齐）
- docs/spec/linux-platform.md §4/§6 若有契约变化须同步
