# PRD：内容搜索超时收口（d8b8055）审核修复 6 项

## 摘要

对 d8b8055 内容搜索超时收口的双子 agent 审核发现 6 项可修复（0 🔴 2 🟡 4 🔵 + 并发专项 2 ⚠️ 中 1 项超范围留笔）。本任务逐项修复，不改 d8b8055 已确立的行为语义。

## 修复清单

### 🟡-1 rg 真失败回退 searchWithGo 分支无直接用例（content_search_test.go）

本提交控制流重排核心分支（原单行 `if` 改嵌套两分支）。补用例：`rgAvailable: true` + `rgPath` 指向不存在二进制 + ctx 未超时 → `exec.ErrNotFound` err 且 `ctx.Err() == nil` → 命中回退分支，断言 searchWithGo 结果正常返回（非超时路径语义不变）。

### 🟡-2 CI 恒 Skip 致「ctx 未超时 exit 1 → nil,nil」核心约束无守护（content_search_test.go）

`TestSearchWithRipgrep_NoMatchUnchanged` 依赖真实 rg，CI（ci.yml 无 rg 安装步骤）恒 t.Skip。改用 exit-1 stub：t.TempDir 写 stub 脚本（Windows `rg.bat` 内容 `@exit 1`、Unix `rg` shell 脚本 `#!/bin/sh\nexit 1` chmod 0755），svc.rgPath 直指 stub 路径（不经 LookPath），验证 `ProcessState.ExitCode() == 1` 分支返 `nil, nil`。不再依赖环境 rg，删除 t.Skip。stub 方案若 Windows 实测不可行（.bat 经 CreateProcess 的执行形态差异），降级保留真实 rg 用例 + 留笔。

### 🔵-1 4.3 表新行缺最外层右括号（docs/spec/logging-and-errors.md:94）

`warning（内容搜索超时被终止…报错会丢已有结果）` 行尾 depth 1 未闭合。补 `）`。

### 🔵-2 测试注释措辞不精确（content_search_test.go）

「cmd.Run 在 Start 前即返 ctx 错误」不精确：rg 存在时进程真实创建后被 Kill，ctx 错误是 Wait 阶段返回；rg 不存在时 Start 前返回的是 `exec.ErrNotFound`（非 ctx 错误，但 err != nil 同样命中判定）。措辞改为两路径如实描述。

### 🔵-3 CLAUDE.md 内容搜索收口句归类矛盾

内容搜索（非 git 域）句并入「Git 操作取消链路」行，语义归类错误且该行已全文最长。拆出独立关键规则行（内容搜索超时收口），链接 logging-and-errors.md。

### 🔵-4 前端 spec 未断言 handleError 分流（useCommandPalette.spec.js）

既有「异常时置空」用例只断言状态字段，`executeContentSearch` catch 新走 `handleError`（warning 码弹 warning / 真失败弹 error）零测试守护。补断言：mock element-plus `ElMessage`，断言 rejection 后 `ElMessage.error` 被调；可加 warning 码 rejection 断言 `ElMessage.warning` 被调。

## 留笔（超出本任务范围，记录不实施）

1. **部分超时前端零感知**：groups 非空返成功 + slog.Warn（不进 GUI），用户看到缺目录结果集且显示成功。正解 `model.ContentSearchGroup` 加 `timedOutDirs []string` 元数据，前端空字段判断 warning 提示（须同步 frontend/wailsjs/ 绑定三处 + wails generate，特性级）。
2. **「有结果 + 超时目录」聚合分支无确定性单测**：共享 ctx 下部分超时无确定性构造（ctx 超时则所有在途目录同时超时），靠代码审查 + 全量回归覆盖。
3. **Go fallback 微秒级误丢窗**：Walk 末次 ctx 检查后到 ContentSearch 判定间 deadline 落窗，完整结果被误标超时。保守方向（误丢不误留），可选 searchWithGo 返 `(results, aborted bool)` 精确化（违背「不改签名」约束，评估后不做）。

## 约束

- 不改 d8b8055 已确立行为语义（仅测试、注释、文档、CLAUDE.md）。
- stub 测试禁 sleep；Windows `.bat` stub 实测验证，不可行即降级留笔。
- service 生产代码零改动（本任务纯测试/文档面）。

## 验收

1. 6 项全落地：rg 失败回退用例、exit-1 stub 无匹配回归（无 rg 环境不再 Skip）、括号补齐、注释精确化、CLAUDE.md 独立行、前端 handleError 断言。
2. `go test ./service/ -run "TestSearch|TestContentSearch"` 全绿（含新用例，无 Skip 混入核心约束守护）；`go test ./...` 全绿；vitest 全绿。
3. trellis-check + 提交 + archive + journal。
