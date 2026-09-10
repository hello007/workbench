# 测试覆盖率提升与门禁

## Goal

WorkBench 后端 `go test ./...` 全 ok 但无覆盖率阈值，前端 Vitest 测试存在（21 个 spec）但无 coverage 依赖与阈值配置。建立测试覆盖率基线与阈值门禁，补齐核心缺口用例，达成后端各包 ≥80%、前端 ≥70%。

## What I already know

### 后端覆盖率现状（`go test ./... -cover`）

| 包 | 覆盖率 | 目标差 | 源文件 | 测试文件 | 评估 |
|---|---|---|---|---|---|
| workbench（app 主包） | 26.9% | -53.1% | app.go + 13 app_*.go | 2（app_test + app_repo_filter_test） | app_*.go 多为薄包装（转发 service），补测试价值需评估 |
| model | 69.4% | -10.6% | 10 | 4 | 接近目标，缺口小 |
| server | 76.1% | -3.9% | 1 | 1 | 接近目标，缺口最小 |
| service | 63.1% | -16.9% | 20 | 18 | 业务逻辑密集，重点域 |
| util | 12.3% | -67.7% | 11 | 3 | 最差，缺口最大 |

后端测试文件共 28 个（主包 2 + model 4 + server 1 + service 18 + util 3）。

### 前端 coverage 现状

- vitest.config.js：仅基础配置（globals + jsdom + setupFiles），**无 coverage 配置**（无 provider / thresholds）
- package.json：有 `test:coverage` 脚本（`vitest --coverage`），但**未装 `@vitest/coverage-v8` 依赖**（node_modules/@vitest/ 下无 coverage-v8），脚本会失败
- 前端测试文件 21 个：
  - components 12（ActivityBar / AiFunctionConfigDialog / AiFunctionPanel / AiTaskHistoryPanel / CommandPalette / ContentPanel / DirectoryTree / FilePreviewRenderer / FileTreePanel / GitInfo / RepoFilterDialog / ToolboxPanel）
  - composables 2（useRecentAccess / useTreeState）
  - store 5（directory / favorites / settings / ui / workspace）
  - utils 1（htmlPreview）
  - views 1（Home）
- 提示词重点补：CommandPalette / FileTreePanel / RepoFilterDialog（交互组件）

## Assumptions (temporary)

- 前端 coverage provider 用 `@vitest/coverage-v8`（vitest 4.x 默认推荐）
- 前端阈值 lines/branches/functions/statements 全 ≥70%
- 后端 CI 门禁用 `-coverprofile` + 脚本核对各包百分比

## Open Questions

（已全部收敛，见下方 Decision）

## Requirements (evolving)

- 后端：各包覆盖率统计，定位 <80% 包与文件，补齐核心路径用例
- 前端：装 `@vitest/coverage-v8`，vitest.config.js 加 coverage 阈值（≥70%），统计缺口
- 补齐：优先 service 层（业务密集）+ 前端交互组件
- 每条用例覆盖明确行为或边界，不为覆盖率写无意义测试
- 后端遵循现有 `*_test.go` 模式，Mock 用现有测试工具函数
- 前端遵循 `__tests__/*.spec.js` 模式，Vue Test Utils + 现有 mock 约定

## Acceptance Criteria (evolving)

- [ ] 后端 model/server 各包覆盖率 ≥80%，service ≥76%（基线，含系统调用核心难测）
- [ ] 后端 util 覆盖率 ≥40%（排除 pty_windows.go 系统调用文件）
- [ ] workbench 主包不设门禁（app_*.go 薄包装，service 已测）
- [ ] 前端 coverage 阈值 lines/branches/functions/statements ≥70% 硬失败配置完成
- [ ] 前端 `npm run test:coverage` 通过（低于阈值 exit 非 0）
- [ ] 前端补测优先交互组件（CommandPalette / FileTreePanel / RepoFilterDialog / DirectoryTree），其次 store，再次 composables
- [ ] CI 门禁就绪：后端 `scripts/coverage-check.sh` + 前端 vitest.config.js thresholds
- [ ] 新增用例覆盖明确行为或边界，无无意义测试

## Definition of Done

- 后端各包覆盖率达标
- 前端 coverage 阈值通过
- 新增测试全部通过
- CI 门禁配置就绪（脚本或配置）
- 检查 README.md 是否需更新

## Out of Scope (explicit)

- 不重构被测代码（仅补测试，不改实现）
- 不改现有测试用例
- 不引入新测试框架
- 不补 app_*.go 薄包装测试（workbench 主包不设门禁）
- 不强测 util pty_windows.go 系统调用（排除该文件，util 阈值 ≥40%）
- 不引入第三方 coverage 工具（codecov 等）

## Decision (ADR-lite)

**Context**：后端无覆盖率阈值，前端无 coverage 依赖与配置。util 12.3%（含 pty 系统调用难测）+ workbench 主包 26.9%（app_*.go 薄包装）拖后腿，硬拉 80% 投入产出比低。

**Decision**：

1. **后端口径分层**（选 B）：
   - 重点包 ≥80%：model / server（业务核心）
   - service ≥76%（基线）：业务密集，含 RunStage/pumpOutput/pty/网络下载等系统调用核心无法单测，实际可达上限约 76%，用户已确认接受
   - util ≥40%：排除 pty_windows.go 系统调用文件，测可测部分（git_fast / file 等）
   - workbench 主包不设门禁：app_*.go 是转发 service 的薄包装，service 已测则重复验证价值低
2. **前端阈值硬失败**（选 A）：vitest.config.js 配 thresholds ≥70%，低于 exit 非 0；须先补测试到 ≥70% 再设阈值，开局即约束
3. **前端补测优先级**（选 A）：交互组件优先（CommandPalette / FileTreePanel / RepoFilterDialog / DirectoryTree），其次 store（业务逻辑），再次 composables
4. **CI 门禁**（选 A）：后端 `scripts/coverage-check.sh`（解析 coverprofile 各包百分比，超阈值 exit 1）+ 前端 vitest.config.js thresholds，无新依赖

**Consequences**：

- 前端硬失败可能开局 CI 红，须先补测到 70% 再合入阈值配置
- util 排除 pty_windows.go 需脚本按文件名剔除计算
- workbench 主包无门禁，app_*.go 后续新增方法无测试保护（可接受，薄包装）
- service 阈值定为 76%（非 80%）：用户已确认接受 76% 基线（决策 C，工程现实，含系统调用核心无法单测）

## Technical Notes

- 后端 workbench 主包覆盖率 26.9% 基于 app.go + 13 app_*.go 全部语句；app_*.go 多为转发 service 的薄包装，service 已测则 app 层补测价值低
- util 12.3% 最差，11 源文件含 pty_windows / git_fast / file 等，部分可能难测（pty 系统调用）
- service 18 个测试文件已覆盖 20 源文件大部分，63.1% → 80% 缺口约 17%
- 前端 vitest 4.1.5，coverage-v8 需匹配版本
