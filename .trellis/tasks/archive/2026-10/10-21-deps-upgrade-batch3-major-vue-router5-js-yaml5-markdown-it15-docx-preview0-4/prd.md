# deps-upgrade-batch3：前端四 major 收尾（vue-router 5 / js-yaml 5 / markdown-it 15 / docx-preview 0.4）

## Goal

收尾前端依赖升级 PRD 遗留的四个 major：vue-router 4→5、js-yaml 4→5、markdown-it 14→15、docx-preview 0.3→0.4。
背景：mermaid 12 批次已归档（镜像 CI run#20/21 全绿），npm audit 基线 0 vulnerabilities；npm audit fix 后 dompurify 3.4.17 / markdown-it 14.3.3 / vite 8.3.4 已在位。本批完成后前端 major 遗留出清。

## What I already know

### 当前版本（升前基线）

| 包 | package.json 声明 | lock 落点 | 目标 major |
|---|---|---|---|
| vue-router | ~4.6.4 | 4.6.4 | 5 |
| js-yaml | ~4.3.2 | 4.3.2 | 5 |
| markdown-it | ~14.3.2 | 14.3.3 | 15 |
| docx-preview | ~0.3.7 | 0.3.7 | 0.4 |

### 消费点（grep 全量）

| 包 | 消费文件 | 用途 |
|---|---|---|
| vue-router | `frontend/src/router/index.js`（唯一） | createRouter + createWebHashHistory |
| markdown-it | `frontend/src/utils/chatMarkdown.js` + `frontend/src/components/FilePreviewRenderer.vue` | 双渲染链（AI 对话气泡 / 文件预览） |
| js-yaml | `FilePreviewRenderer.vue`（frontmatter 解析） + `JsonYamlToolPanel.vue`（JSON/YAML 互转工具） | load/dump |
| docx-preview | `FilePreviewRenderer.vue`（唯一） | renderAsync docx 预览分支 |

### 约束

- 四包均在 `scripts/check-dep-consistency.mjs` tilde 清单（46/49/50/57 行）既有登记，升级仅改版本声明，四清单零新增
- tilde 声明以 lock 实际落点重写（声明 `~<lock落点>`，防 CI 红的既有规则）
- markdown-it 15 影响**双渲染链**，FilePreviewRenderer 与 AI 对话气泡各验一轮（代码高亮/TOC/frontmatter/mermaid 协同）
- mermaid 渲染链锁定 `import('mermaid')` + `run({suppressErrors:true})` + `layout:'dagre'`+`look:'classic'` 双键（mermaid-rendering.md），markdown-it 升级不得破坏该链
- 回归走 SOP §3.3 全项：一致性脚本 / vitest 全量 / E2E web+desktop / npm audit 清零核对
- 任一包 breaking 过大或收益低可单独判「跳过留痕」，不阻塞他包

## Requirements

1. **升前评估**：四包 changelog/breaking 逐条核对（评估产出落 `research/`）：
   - vue-router：createRouter / 守卫 / 历史模式 API
   - js-yaml：load/dump safe 系与 DEFAULT_SCHEMA
   - markdown-it：渲染规则 / 插件 API / 类型面
   - docx-preview：渲染选项与样式隔离
   - 每包对照本仓消费点出 API 消费清单；任一包可判「跳过留痕」
2. **升级**：tilde 声明以 lock 落点重写 → npm install → 消费面按 breaking 清单适配
3. **回归**：SOP §3.3 全项 + markdown 预览专项（代码高亮/TOC/frontmatter/mermaid 协同渲染，双渲染链各验一轮）
4. **沉淀**：消费面坑按 trellis-update-spec 沉淀；security-scan.md §5.4 评估表同步（四 major 出清后该节只剩 go-git v6 / wails v3）

## Acceptance Criteria

- [ ] 四包各自有明确结论：升级完成 或 跳过留痕（含理由）
- [ ] 每包 breaking 评估落 `research/<topic>.md`
- [ ] 升级包 package.json tilde 声明 = lock 落点版本
- [ ] `node scripts/check-dep-consistency.mjs` 通过（四清单零新增）
- [ ] vitest 全量通过；E2E web + desktop 通过
- [ ] npm audit 保持 0 vulnerabilities
- [ ] markdown 预览专项双链验证通过（代码高亮/TOC/frontmatter/mermaid）
- [ ] security-scan.md §5.4 评估表同步
- [ ] 消费面坑沉淀到 docs/spec/

## Definition of Done

- 测试全绿（vitest + E2E web/desktop）
- 一致性脚本通过
- 文档/spec 同步
- 镜像 CI 全绿

## Out of Scope

- go-git v6 / wails v3 升级（security-scan.md §5.4 剩余项）
- 其余依赖的 minor/patch 漂移（除非 breaking 适配连带）

## Open Questions

- （已收敛：research 四包结论全部「建议升级」，无跳过项，见 Research References）

## Research References

* [`research/vue-router-5-breaking.md`](research/vue-router-5-breaking.md) — 建议 ~5.4.0，官方明示零代码改动；唯一流程面动作 = 先同步四清单登记；`@vue/devtools-api` 随升 ^8，wails dev 冒烟兜底
* [`research/js-yaml-5-breaking.md`](research/js-yaml-5-breaking.md) — 建议 ~5.5.0；唯一硬改点 = 2 处 default import 改 namespace import；dump 标量风格对 JsonYamlToolPanel.spec.js 逐字断言影响实施时 npm test 证实
* [`research/markdown-it-15-breaking.md`](research/markdown-it-15-breaking.md) — 建议 ~15.0.3（tarball 全量 diff 取证）；本仓零插件依赖，消费 API 逐项不变；mermaid 协同链不受影响；CommonMark 边缘微调靠三层测试面兜底
* [`research/docx-preview-04-breaking.md`](research/docx-preview-04-breaking.md) — 建议 ~0.4.1（0.4.1 无 release 条目，tarball 逐文件 diff 确认）；renderAsync/6 options/DOM 产物全兼容；真机多页/表格/页眉页脚 docx 冒烟兜底

## Decision (ADR-lite)

**Context**：PRD 遗留四 major 是否可升、是否需跳过留痕，依赖升前 breaking 评估。
**Decision**：四包全部升级（vue-router ~5.4.0 / js-yaml ~5.5.0 / markdown-it ~15.0.3 / docx-preview ~0.4.1）。js-yaml 是唯一需代码适配项（2 处 namespace import）；其余三包零代码改动。
**Consequences**：换得 major 出清 + docx-preview 0.4 bug fix 集（嵌套 smart tags/evenAndOddHeaders/pageBreakBefore 等）；风险敞口集中在各 research 列明的无法静态确证项（dump 断言/CommonMark 边缘/0.4.0 edge cases），均有对应测试面或真机冒烟兜底；`@vue/devtools-api` 进运行时依赖树。

## Technical Notes

- 消费面极小是本批有利条件：vue-router 单文件、docx-preview 单点、js-yaml 两文件、markdown-it 双链
- 相关 spec：docs/spec/frontend-dep-upgrade.md（SOP + 分档钳制）、docs/spec/mermaid-rendering.md、docs/spec/security-scan.md
