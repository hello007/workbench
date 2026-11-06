# PRD：docs 全量校验与修正

## 1. 目标

对 `docs/` 全目录与仓根 `README.md` 做一次事实一致性、时效性、链接完整性、结构与格式的全量校验，并修正可取证的问题。**不修改任何业务代码、配置与 CI**。

## 2. 真相源（事实以此为准）

| 维度 | 真相源 |
|---|---|
| 版本号 | `wails.json` 的 `info.productVersion`（当前 1.15.0） |
| Go / Wails 版本 | `go.mod` |
| 前端依赖版本 | `frontend/package.json` |
| Wails 绑定方法清单 | `frontend/wailsjs/go/main/App.d.ts`（`export function` 计数） |
| 覆盖率阈值 | `scripts/coverage-check.sh` + `docs/spec/test-coverage-gate.md` |
| 构建与 CI 行为 | `.github/workflows/*.yml`、`wails.json`、`scripts/release.sh` |
| 项目规则与踩坑结论 | `AGENTS.md`「关键规则」、`docs/spec/README.md` |
| 功能行为 | `CHANGELOG.md` 最新条目 + `service/` 与 `frontend/src` 实现 |
| 目录结构 | 仓库实际目录 |

## 3. 校验维度

1. 事实一致性：版本、工具链、依赖、方法数、阈值、端口、超时、路径、命令可执行性。
2. 时效性：最近 3 个 CHANGELOG 版本是否入档；已删除/改名/废弃模块是否仍被描述。
3. 链接与交叉引用：相对链接目标存在、锚点匹配、索引完整。
4. 结构与格式：标题层级、表格分隔行、代码块语言、占位符、Mermaid 规范、简中术语一致。
5. 内容质量：空泛套话、重复、与实现矛盾的操作步骤、章节闭环。

## 4. 产出

- 第一部分：问题清单表（P0/P1/P2 排序，含证据 `文件:行` 与修正方案）。
- 第二部分：修正明细（逐份文档说明改了什么、依据哪份真相源）。
- 第三部分：待人工确认清单（仓库无法取证的问题）。
- 第四部分：回归校验（链接、表格、Mermaid、占位符扫描结果）。

## 5. 硬性约束

- 只改文档；不动 `.go` / `.vue` / `.js` / 配置与 CI。
- 不删除历史 ADR 既有结论，只在文首追加「已过时」标注并链接新文档。
- 不篡改事实：代码与文档冲突时先判定真相源，再决定改文档还是列入待确认。
- 单份文档改动超过 50% 时，先说明重构理由与保留内容。
- 不主动 `git commit` / `git push`。

## 6. 验收标准

- 相对链接失效数 = 0（历史归档 `docs/archive/` 内冻结引用除外，需显式说明）。
- `docs/spec/README.md` 索引覆盖 `docs/spec/` 全部 `.md`。
- API 方法总数与 `App.d.ts` 一致，缺失方法补登。
- 版本号 / 端口 / 阈值 / 默认值等关键数字全部可回指真相源。
- Mermaid 语法与结构检查零告警。
