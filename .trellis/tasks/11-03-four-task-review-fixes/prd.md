# 四项子任务审核整改

## Goal

修复四个已归档子任务只读审核发现的全部 P2/P3 问题：retained 目录锚迁移与删除安全、ActivityBar 活跃态断言范围、依赖治理文档口径、目录选中交互文档归因；补齐缺失单测，使每处修复有测试守护。

## Requirements

### retained 目录锚（P2/P3）

* `migrateSplitRecordsTx` 的「仅落点命中」分支：把 `migrateRetainedAnchorDirsTx` 调用提前到 `!outHit` 判定之前，覆盖「output_dir 不在旧前缀、但 retained 锚在旧前缀」的记录。
* `splitArtifactDirWithinLibrary` 增加符号链接/junction 解析：root 与 dir 经 `EvalSymlinks` 后重新 `Rel` 判定；无法解析（不存在/权限）按不可判定拒绝。
* `cleanupRetainedSplitArtifacts` 的 `os.Lstat` 错误分流：仅 `os.IsNotExist` 丢弃条目（自愈），其他错误保留条目并 `Warn`。
* 旧格式空锚语义钉死：空锚随 `record.output_dir` 动态解析（目录迁移后指向新目录），不物化；补直测与文档说明。
* 补单测：`mergeSplitFingerprintsByName` 同名异目录同时保留；`splitArtifactDirWithinLibrary` 库外/相对/`..`/symlink 越界拒绝；`retainedAnchorsRewrittenTx` 显式锚改写与空锚保留；`migrateRetainedAnchorDirsTx` 在 `output_dir` 不在前缀时的锚迁移。

### ActivityBar 测试语义化（P2/P3）

* `activePanelIds` 改为扫描 `.activity-bar-item.is-active` 并映射 `data-panel-id || data-action`，覆盖终端活跃态。
* 新增断言：`terminalVisible=true` 时活跃项包含 `terminal`。
* 语义属性总览用例补 `data-panel-id` 唯一性断言。
* 入口总数断言注释注明「设备指示隐藏时」。

### 依赖治理（P2/P3）

* `overrides` 改嵌套：`js-beautify.glob`、`editorconfig.minimatch`，避免未来新依赖被全局静默强制。
* `security-scan.md` §5.5 旧「moderate 保持 8」改为已出清；§5.6 补历史清单位置、连带 minor/major 升级记录、engines 收窄说明。
* `frontend-dep-upgrade.md` §3.7：回退命令改为 `git checkout HEAD^ -- ...` 或 `git revert`；audit 必跑面写明 `--audit-level=moderate`；补 engines 说明。
* `merge.spec.js` 冲突态 mock 改 `__rules__`（after Merge / after AbortMerge），删除两处精确调用次数断言，保留行为断言。

### 目录选中文档（P3）

* `frontend-visual-conventions.md`：记录「右键/拖拽不改变当前选中，F2/Del 仍作用于上次 click 选中项」；修正拖拽抑制 click 的归因（原生 drag 由浏览器抑制，SortableJS 仅 fallback/touch 路径）。

## Acceptance Criteria

* [ ] 新增/修改单测覆盖上述每个修复点，变异可验证（至少：迁移前置、symlink 拒绝、Lstat 非 ENOENT 保留、同名异目录合并不吞并、终端活跃态）。
* [ ] `go test ./service/` 全绿；覆盖率门禁通过；gofmt / go vet / `GOOS=linux go vet ./service/` 通过。
* [ ] `npm test -- --run` 全绿。
* [ ] Web E2E 全绿（merge.spec 改 rules 后稳定）。
* [ ] `npm audit --audit-level=moderate` 仍为 0；嵌套 overrides 解析后 glob 14 / minimatch 11 单一实例。
* [ ] 文档无「moderate 8 接受」残留；回退命令可执行。

## Definition of Done

* 审核发现逐条有修复或明确的设计性说明，无未处理 P2。
* 测试、构建、E2E、audit、一致性检查全部通过。
* 文档与代码口径一致。

## Technical Approach

1. retained：调整迁移调用位置；`EvalSymlinks` 双侧解析；Lstat 错误分叉；补 5 组直测。
2. ActivityBar：helper 扩到 action 项，补终端活跃态与 ID 唯一性断言。
3. 依赖：package.json overrides 嵌套化 → `npm install` → 验证 `npm ls` 与 audit；文档四处修正；merge.spec 改 `__rules__`。
4. 目录选中：仅文档修正，不改生产代码（click 单入口为既定决策）。

## Decision (ADR-lite)

**Context**: 审核发现 9 个 P2 与 12 个 P3；其中「旧格式空锚物化」与「右键选中行为」涉及语义选择。

**Decision**: 空锚保持「随 output_dir 动态解析」契约，不物化（物化会在手工改库场景下误导删除）；右键/拖拽不改变选中，文档明确快捷键目标语义；其余全部按建议修复。

**Consequences**: 迁移缺口与删除越界风险消除；ActivityBar 断言覆盖终端；overrides 作用域收窄；两个既定语义以文档钉死，后续变更需走新任务。

## Out of Scope

* 设备区块 `rows[n]` 下标语义化（另开任务）。
* 重写 merge E2E 全部调用链断言。
* 修改生产交互行为（右键选中、拖拽选中）。

## Technical Notes

* 审核结论来源：四路只读 subagent（dir-select 首跑 403 后重跑）。
* 关键文件：`service/assets_db.go`、`service/assets_split_fingerprint.go`、`service/assets_split.go`、`service/assets_split_*_test.go`、`frontend/src/components/__tests__/ActivityBar.spec.js`、`frontend/e2e/merge.spec.js`、`frontend/package.json`、`docs/spec/{security-scan,frontend-dep-upgrade,frontend-visual-conventions,e2e-testing}.md`、`CLAUDE.md`。
