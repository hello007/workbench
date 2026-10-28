# CLAUDE.md 关键规则表瘦身：详情下沉与钩子化

## Goal

CLAUDE.md「关键规则」表 68 行数据行、总 49,191 字符，多行数百字，远超规范沉淀规则自述的「一两句级每次会话必知」定位；CLAUDE.md 每会话全量注入，token 成本与信噪比双差。本任务系统性瘦身全表：行内保留「规则名 + 单点/禁令钩子 + 新增段加行指引」，机理/历史/坑/签名细节/测试教训下沉 docs/spec 对应文档；瘦身 = 搬家非删除，零信息丢失。

2026-10-28 遗留收口任务已示范单行精简先例（设置段双单点行 ~800 字 → ~230 字，细节下沉 cross-layer-contracts.md），本次推广至全表。

## What I already know（侦察结论）

* 表格现状：68 数据行，49,191 字符（含表头行 9 字符；实测脚本 `.trellis/tasks/10-28-claude-md/research/table-audit.txt`）
* 超长行 TOP：行39 assets-thumbs COM（2154 字）、行30 Git 取消链路（1853）、行65 八期 CIDR（1620）、行67 十期趋势面（1552）、行57 安全三期（1544）、行19 frontend-visual（1229）、行42 assets v6（1192）、行9 E2E（1073）
* 连接管控相关（行57-68）12 行合计 ~14.7k 字符，占全表 30%，用户提议合并为「连接管控总行 + browser-channel.md 分期锚点」形态
* 无承接文档行：仅行25（统计类派生聚合缓存，文档列「—」），须新建文档或并入邻近主题（commit history 缓存主题，候选并入 logging-and-errors.md 或 perf-baseline.md，brainstorm 定）
* docs/spec/README.md 索引缺 4 个已存在文档：ai-chat-service.md、assets-thumbs-com.md、linux-platform.md、pptx-preview.md（验收底线要求同步）
* docs/spec 现有 26 个主题文档，行→文档映射大部分已存在

## Assumptions (temporary)

* 行内目标长度上限 ≤150 字（用户建议，brainstorm 确认）
* 瘦身后规则本体语义零变化：禁令、单点名称、错误码、方法签名等规则性内容一字不改，只挪位置
* 对照表逐行登记「行 → 承接文档锚点」，CLAUDE.md 与 docs/spec 不得矛盾

## Decision (ADR-lite)

### Q1 长度上限：分档制（2026-10-28 用户确认）

* A 纯操作钩子行（本身 ≤250 字）：原样保留或轻度压缩，上限 ≤250 字
* B 混合行（钩子 + 机理/历史/坑）：钩子全留行内，机理/历史/坑/测试教训下沉，上限 ≤200 字
* C 分期/史诗合并行（连接管控类）：钩子清单 + 分期锚点，上限 ≤350 字
* 下沉判据：「下次写代码时要照着做的」（禁令、单点函数名、错误码、同步点位）留行内；「为什么/怎么踩的」（机理、日期、事故经过、签名演变史）下沉

### Q2 连接管控合并形态：11+1 两行（2026-10-28 用户确认）

* 行57-67（安全三期 + 一~十期，11 行）合并为 1 条 C 档总行「浏览器通道安全与连接管控」，行内保留跨期最高频钩子（① 新增纯桌面数据面绑定方法必须同步 `RPCHiddenMethods` 现清单 ② 两认证点 HTTP requireToken + WS ServeHTTP 新增判定必须同接 ③ 新错误码三处同步）+ 锚点指向 browser-channel.md
* 行68（十一期审计留存）独立 B 档瘦身，承接 logging-and-errors.md §8.2
* 执行期核对：browser-channel.md 分期导航完整性，缺则先补分期导航节再瘦身

### Q3 对照表落点：任务目录 research/（2026-10-28 用户确认）

* 对照表写 `.trellis/tasks/10-28-claude-md/research/migration-map.md`，随任务归档；docs/spec 不留一次性迁移地图

### Q4 行25 统计类缓存承接：并入 perf-baseline.md（2026-10-28 用户确认）

* perf-baseline.md 新增小节承接「统计类派生聚合缓存两入口联动失效」契约（该文档已是性能与驻留混合域，且行27 已引用其缓存失效上下文）；CLAUDE.md 行25 文档列由「—」改为指向该小节

### Q5 表格行序：保持现有顺序（2026-10-28 用户确认）

* 只瘦行不改序；最小 diff、验收简单、AI 消费无差异

## Requirements

* 全表 68 行逐行瘦身：行内保留规则名 + 单点/禁令钩子 + 新增段/新增 X 加行指引，按 Q1 分档上限执行
* 机理、历史、坑、签名细节、测试教训下沉 docs/spec 对应文档的语义对应章节（不集中堆放污染文档结构）
* 每行详情须有 docs/spec 承接，缺失的先补文档再瘦身（禁删详情无承接）
* 行57-67 合并为 1 条 C 档总行；行68 独立 B 档瘦身
* 逐行对照表 `research/migration-map.md` 登记「原行 → 瘦身后行 → 承接文档锚点 → 下沉内容清单」
* docs/spec/README.md 索引补齐 4 个缺失文档（ai-chat-service / assets-thumbs-com / linux-platform / pptx-preview）+ 受影响文档条目摘要更新
* CLAUDE.md 版本号升至 v2.41
* 表格行序保持现状
* 纯文档任务，无代码改动

## Acceptance Criteria

* [ ] 全部详情在 docs/spec 有承接，无孤儿细节（逐行核对 migration-map）
* [ ] docs/spec/README.md 索引与文档实际清单一致（补 4 缺失 + 更新受影响条目）
* [ ] 零信息丢失对照表 `research/migration-map.md` 完成，逐行可查
* [ ] 瘦身零语义变化：规则有效性集合不变——行内压缩重写允许（表述变、内容不变），下沉内容原文搬家不删改；CLAUDE.md 与 docs/spec 无矛盾
* [ ] 行内长度符合分档上限（A ≤250 / B ≤200 / C ≤350）
* [ ] CLAUDE.md 版本 v2.41
* [ ] 收益度量（瘦身前后字数/行数对比）记录进 journal

## Implementation Plan（阶段制，纯文档任务不拆 PR）

* **阶段 1 承接核对**：逐行对照 68 行与 docs/spec 现有文档，核对承接完整性；缺失处先补文档（已知：行25 → perf-baseline.md 新小节；browser-channel.md 分期导航完整性核对；README.md 索引补 4 缺失）；产出 migration-map.md 初版
* **阶段 2 全表瘦身**：按分档口径重写 68 行（行57-67 合并总行），migration-map.md 补下沉内容清单终版
* **阶段 3 验收度量**：逐行对照核对零信息丢失、前后字数对比统计、journal 记录、CLAUDE.md 版本 v2.41

## Definition of Done (team quality bar)

* 纯文档任务：无测试要求；CI 无文档门禁
* README.md（项目根文档索引）如受影响须同步
* journal 记录收益度量

## Out of Scope (explicit)

* 不改任何代码
* 不重构 docs/spec 文档内部结构（只做承接性补充，避免范围爆炸）
* 不动「规范沉淀规则」分流标准本身的定义（如需修订另行任务）
* 不处理 user 级 CLAUDE.md（~/.claude/CLAUDE.md）——仅项目级

## Technical Notes

* CLAUDE.md 关键规则表位于 `### 关键规则` 节，紧跟「规范沉淀规则」说明
* 每行格式：`|规则|详细文档|`，详细文档列含 markdown 链接
* 2026-10-28 先例行：设置段双单点行（行69），~800 字 → ~230 字，细节下沉 cross-layer-contracts.md「设置段增删 Scenario checklist」
* 侦察数据：`research/table-audit.txt`（逐行字符数 + 规则列前 80 字）

## Research References

* [`research/table-audit.txt`](research/table-audit.txt) — 68 行逐行字符数与内容清单
