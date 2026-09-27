# 前端内存驻留面量化与释放优化（perf-baseline §15 系列收口「及时释放对象」子项）

## Goal

路线图「应用性能 → 内存使用优化」父项最后一个未收口子项「及时释放不再使用的对象」
（未测待定）。本任务量化前端无界驻留面，按数据分流「实施释放优化 / 不动收口」，
收口 perf-baseline §16 与路线图。

## What I already know（前序任务量化锚点，勿重复测量）

- §15.1 定性：Go 侧常驻堆 0.6MB 无优化收益，真实内存风险在前端渲染面——只测前端
- §15.6 已收口文件树渲染面（截断优化）；§15.3 已收口大文件读取（非瓶颈不动，
  上限即挡板先例）
- 测量口径可复用：CDP `Performance.getMetrics` + `HeapProfiler.collectGarbage`
  强制 GC、独立 context 冷缓存、3 轮中位、serve cwd 隔离工作区（脚本先例
  scripts/frontend-memory-preview.mjs / frontend-memory-tree.mjs）
- 既有释放机制（验证是否已覆盖，勿重复建设）：
  - `CancelAiTask` + repoPath 切换重置 AI 任务态（cross-layer-contracts 规则）
  - `InvalidateCommitHistoryCache`（Go 侧缓存，非前端驻留）
  - `EventsOn` 返回闭包精准注销规范（terminal-multi-session 等已按此实现）
- Wails 桌面 WebView2 常驻进程：驻留 = 应用生命周期内不还，非页面刷新可解

## Requirements

1. 驻留面盘点（research）覆盖至少：
   - 组件切换/仓库切换后旧状态：CommitHistory 提交数组、LocalChanges diff 数据、
     AiChatPanel 消息数组、ai-task 结构化输出、FileDiffDialog 大 diff、
     ContentPanel 文件预览 base64、TerminalPanel scrollback
   - pinia store 级无界数组（aiChat 消息/会话索引、任务历史等）
   - EventsOn 监听器注销完整性（规范已有，验证执行面）
2. 测量脚本 `scripts/frontend-memory-retention.mjs`：「操作前 → 操作后 →
   切走/关闭后」三点 JSHeap 驻留差（GC 口径），参数风格沿用 `--runs`
3. 分流定性：驻留增量 MB × 触发频率 × 释放成本 → 实施 / 不动收口（对齐 §15.3 先例）
4. 实施达标项：释放点挂既有生命周期钩子（repoPath watch / 组件卸载 / EventsOn
   闭包注销），禁全局事件总线补丁、禁 WeakRef/FinalizationRegistry；Vue 响应式
   大数组释放 `splice(0)`/`value = []` 置空 + 确认无闭包持有
5. 收口：perf-baseline §16（驻留面量化表 + 分流结论 + before/after）；
   路线图「及时释放不再使用的对象」勾选或记录不动定性；三子项全收口则勾选父项

## Acceptance Criteria

- [ ] 驻留面量化表（每面：触发操作 / 驻留增量 / 分流结论 / 证据）
- [ ] 实施项 before/after 驻留对比（目标按测量数据定档，brainstorm 补录）
- [ ] vitest + E2E 全绿，覆盖率 ≥70% 门禁，新增释放逻辑带单测
- [ ] perf-baseline §16 + 路线图收口
- [ ] trellis-check + 子 agent 审核

## Definition of Done

- 同上 AC 全过；commit + journal 按工作流

## Out of Scope

- Go 侧内存（§15.1 已证无收益）
- 文件树渲染面（§15.6 已收口）
- 大文件分块读取（§15.3 已定性不动）
- 终端 scrollback 上限调整（若测量证明驻留可控则不动；ptty 缓冲属 xterm 内部）

## Technical Notes

- 契约面预期零后端改动；若动 store 字段结构须同步 `frontend/wailsjs/` 三处绑定
- 测量环境：build/bin/workbench.exe --serve + playwright chromium headless，
  Windows 11 / Ryzen 7 H 255 / 31.27 GB（同 §6/§15 测量机）
