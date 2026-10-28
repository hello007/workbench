# 规范沉淀口径对齐：分档钩子化固化进沉淀流程

## Goal

上一任务（10-28-claude-md，commit cd5e7e2）完成关键规则表瘦身（68 行 49,191 字符 → 60 行 15,984 字符，行内 = 规则名 + 单点/禁令钩子 + 新增指引，硬顶 350 字符，细节下沉 docs/spec），但两处口径自述未同步：① CLAUDE.md「规范沉淀规则」节第 2 条仍写「一两句级每次会话必知的关键规则」；② docs/spec/README.md「两级分流规则」表判断标准列仍写「一两句话可完整表达」。与新现实（钩子级、分档上限、下沉判据）矛盾。且瘦身后每次任务收尾 trellis-update-spec 沉淀新规则时无分档口径约束，新行会按旧惯性写回数百字长行，瘦身成果回退。本任务对齐两处口径文本，并把分档自检固化进沉淀流程。

## What I already know（侦察结论）

* CLAUDE.md「规范沉淀规则」节（43-48 行）：第 1 条详细契约分流有效；第 2 条「一两句级每次会话必知的关键规则」为旧口径
* docs/spec/README.md「两级分流规则」表（9-16 行）：判断标准列「每个 session 必知、一两句话可完整表达」为旧口径
* `.claude/skills/trellis-update-spec/SKILL.md`：全篇指向废弃 `.trellis/spec/` 路径，无任何长度/分档约束；先例 = CLAUDE.md 覆盖层指令生效（skill 文本未改，以 CLAUDE.md 为准）
* `.claude/commands/trellis/finish-work.md`：4 步（survey → dirty 分类 → archive → journal），无沉淀口径自检步骤
* 分档定义在上任务 prd.md Q1 ADR（随任务归档）：A 纯操作钩子行 ≤250 / B 混合行（钩子+机理）≤200 / C 分期/史诗合并行 ≤350；硬顶 350；下沉判据「下次写代码时要照着做的（禁令、单点函数名、错误码、同步点位）留行内；为什么/怎么踩的（机理、日期、事故经过、签名演变史）下沉」
* migration-map.md 尾部：「分档上限按整行字符数（含文档列 ~50 字符链接开销）执行，硬顶 350 全表达标」
* 现存新表 60 数据行中约 34 行在 250-350 区间（钩子密集行），严格按 B 档 ≤200 归档会有口径争议
* workflow.md Phase 3.3 = trellis-update-spec 沉淀时点；Phase 3.4 提交后走 finish-work

## Open Questions

（无——全部决策已确认）

## Decision (ADR-lite)

### Q5 钩子导流验证：静态抽查纳入（2026-10-28 用户确认）

* 本任务做静态抽查：抽 5 行钩子最密的行，提取反引号内特征标识符（函数名/错误码/文件名）grep docs/spec，确认每钩子能定位到承接章节
* 复用上任务 coverage-check 方法（脚本化可重复）；动态新会话模拟验证另开任务（成本高、样本少说服力有限）

### Q4 现存 250-350 区间行：口径显式承认（随 Q1 落定）

* 两档口径中「钩子密集行（钩子 ≥7）≤350」即对现存约 34 行 250-350 区间行的显式承认，下次治理无口径争议
* 分档细则（两档定义、钩子计数口径、现存行分档归属示例）登记 docs/spec/README.md

### 超限处置：当场下沉（用户消息已给倾向，采纳）

* 沉淀时新行超分档上限 → 当场把超出细节下沉 docs/spec 再回写钩子，禁止「先落行后治理」（避免再积攒治理债）
* 写进 CLAUDE.md 覆盖层口径与 finish-work 自检步骤

### Q3 finish-work 加收尾自检步骤（2026-10-28 用户确认）

* finish-work.md Step 3 归档前加轻量检查：本会话若向 CLAUDE.md 关键规则表新增了行，逐行核两档上限，超限退回 Phase 3.3 当场下沉再归档
* 与 Phase 3.3 覆盖层口径联动闭环，防旧惯性回退的第二道闸
* 改 command 文件（.claude/commands/trellis/finish-work.md）非 workflow.md 本体，不触发 `[required · once]` 标签块同步规则（实施时核对 workflow.md 引用关系）

### Q2 沉淀流程固化点：CLAUDE.md 覆盖层（2026-10-28 用户确认）

* 分档自检指令写进 CLAUDE.md「规范沉淀规则」节，与 Q1 口径文本同处，延续既有覆盖先例（CLAUDE.md 声明覆盖 → SKILL.md 旧文本不改以 CLAUDE.md 为准）
* SKILL.md 本任务不动（全篇 `.trellis/spec/` 旧路径清理为潜在后续任务）
* 落点明确单一：CLAUDE.md 是唯一权威口径源，README 两级分流表为引用同步

### Q1 分档口径形态：简化两档内联（2026-10-28 用户确认）

* CLAUDE.md「规范沉淀规则」节第 2 条改写为内联一行式：钩子级定义（规则名 + 单点/禁令钩子 + 新增指引）+ 两档上限（普通行 ≤250、钩子密集行（钩子 ≥7）与合并行 ≤350、硬顶 350）+ 下沉判据一句（「下次要照着做的」留行内，「为什么/怎么踩的」下沉 docs/spec）
* 增量约 200 字，沉淀时自包含可执行，不依赖跳读
* 分档细则与现存行分档归属登记 docs/spec/README.md（README 两级分流表判断标准列同步改写）
* 不沿用上任务 A≤250/B≤200/C≤350 三档——B/A 执行区分度低，C 档原定义不含钩子密集行；两档口径取代三档成为唯一权威口径（上任务 ADR 保留归档不删）

## Requirements

* CLAUDE.md「规范沉淀规则」节第 2 条改写：删「一两句级」，改为两档钩子级口径（普通行 ≤250、钩子密集行（钩子 ≥7）与合并行 ≤350、硬顶 350）+ 下沉判据一句 + 超限当场下沉处置 + 分档自检指令
* （执行期发现，2026-10-29）`.trellis/workflow.md` Phase 3.3 正文（~570 行）亦写「一两句级每个 session 必知」——第三处旧口径，须同步改写为两档口径引用 CLAUDE.md（正文描述修订，不触 `[required · once]` 标签块规则）；否则违反验收底线「口径不与 workflow.md 冲突」。改动面 3 → 4 文件
* docs/spec/README.md「两级分流规则」表判断标准列同步改写为同口径，并登记分档细则（两档定义、钩子计数口径、现存行分档归属说明）
* finish-work.md Step 3 前加收尾自检：本会话新增表行逐行核两档上限，超限退回 Phase 3.3 当场下沉
* 静态导流抽查：抽 5 行钩子最密行，特征标识符 grep docs/spec 验证承接章节可定位，结果存 research/
* 核对口径与 workflow.md 无冲突（finish-work 改动不触发标签块同步）
* 零代码改动纯文档；CLAUDE.md 版本号 v2.42；journal 记录收益度量

## Acceptance Criteria

* [ ] CLAUDE.md「规范沉淀规则」节与 docs/spec/README.md 两级分流表口径一致，均为两档钩子级口径，不再出现「一两句级/一两句话」旧表述
* [ ] 两档上限（250/350）与下沉判据在两处文本中表述一致
* [ ] 沉淀分档自检固化点唯一落在 CLAUDE.md 覆盖层（SKILL.md 本任务零改动）
* [ ] finish-work.md 含归档前分档自检步骤，与 Phase 3.3 退回路径闭环
* [ ] 超限处置规则（当场下沉，禁先落行后治理）写进覆盖层口径
* [ ] README.md 登记分档细则与现存 250-350 区间行归属说明（钩子 ≥7 承认条款）
* [ ] 静态导流抽查 5 行通过（每钩子可定位承接章节），结果存 research/
* [ ] 口径与 workflow.md 无冲突（含 [required · once] 标签块规则核对记录）
* [ ] 零代码改动；CLAUDE.md 版本 v2.42；journal 记录

## Technical Approach

**改写文本草案**（CLAUDE.md「规范沉淀规则」第 2 条，实施时微调）：

> **钩子级关键规则**：行内 = 规则名 + 单点/禁令钩子 + 新增行指引。两档上限——普通行整行 ≤250 字符，钩子密集行（钩子 ≥7 个）与分期/合并行整行 ≤350 字符（含文档列链接开销）；「下次写代码要照着做的」（禁令、单点函数名、错误码、同步点位）留行内，「为什么/怎么踩的」（机理、日期、事故经过、签名演变史）下沉 docs/spec。新行超上限时当场下沉超出细节再回写钩子，禁先落行后治理。

**改动面**（3 文件 + 1 归档产物）：

| 文件 | 改动 |
|---|---|
| CLAUDE.md | 「规范沉淀规则」节第 2 条改写 + 新增分档自检一句；版本 v2.42；最后更新日期 |
| docs/spec/README.md | 两级分流表判断标准列改写 + 新增「分档细则」小节（两档定义/钩子计数/现存行归属） |
| .claude/commands/trellis/finish-work.md | Step 3 前插归档前自检段 |
| docs/spec/README.md（或 CLAUDE.md 指引处） | 关键规则表行格式微调指引如需 |

**实施顺序**：① workflow.md 冲突核对 → ② CLAUDE.md 口径改写 → ③ README 口径同步 + 分档细则 → ④ finish-work 自检步骤 → ⑤ 静态导流抽查（脚本复用上任务 table-audit 模式）→ ⑥ 版本号 + journal。

## Implementation Plan（纯文档任务不拆 PR）

* 阶段 1 口径落点核对：workflow.md 引用关系核对（finish-work 改动是否触碰标签块规则）、CLAUDE.md/README 两处旧口径文本精确定位
* 阶段 2 三文件改写：按 Technical Approach 执行
* 阶段 3 验证与收尾：静态导流抽查 5 行、两处口径一致性通读、版本 v2.42、finish-work + journal

## Out of Scope (explicit)

* 不改任何代码
* 不重构 docs/spec 现有 26 个主题文档内容
* SKILL.md 全篇 `.trellis/spec/` 旧路径清理（如需另行任务）——除非 Q2 决定改 skill

## Technical Notes

* CLAUDE.md 关键规则表 60 数据行，每行格式 `|规则|详细文档|`
* 分档定义源：`.trellis/tasks/archive/2026-10/10-28-claude-md/prd.md` Q1 ADR（已归档，日常不可达——口径固化后分档定义须落在常驻可达位置）
* 上任务验证方法可复用：逐行特征标识符 grep docs/spec（table-audit.py / coverage-check.txt 模式）
