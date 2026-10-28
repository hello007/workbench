# 零信息丢失对照表（migration-map）

> 原表 68 数据行（49,191 字符）→ 新表 60 数据行（15,984 字符，-67%）。
> 「处理」列：钩子保留 = 行内保留核心钩子，其余细节在承接文档原有章节；下沉 = 行内细节本轮补入承接文档；拆 = 一行拆两行；合并 = 多行并一行。
> 承接验证方法：逐行提取反引号内特征标识符 grep 目标文档（`coverage-check.txt` / `coverage-nonok2.txt`）；6 处真缺承接在阶段 1 补齐（标注「下沉」）。

| 旧行(t) | 主题 | 新行 | 处理 | 承接锚点 |
|---|---|---|---|---|
| t2 | go-git 出清 | 1 | 钩子保留 | go-git-deprecation.md（三口径细节原有） |
| t3 | 绑定三处同步 | 2 | 原样级 | cross-layer-contracts.md §绑定 Scenario |
| t4 | models.ts 别名 | 3 | 原样级 | cross-layer-contracts.md §MISSING_EXPORT |
| t5 | workflow 标签块 | 4 | 原样级 | .trellis/workflow.md Customizing |
| t6 | 覆盖率门禁 | 5 | 原样级 | test-coverage-gate.md |
| t7 | AppServices 装配 | 6 | 钩子保留（133 委托/字段提升机理下沉，文档原有） | app-services-assembly.md |
| t8 | slog/AppError | 7 | 钩子保留（v2.16 事故机理下沉，文档 §5.3 原有） | logging-and-errors.md |
| t9 | E2E | 8 | 钩子保留（11→8 钩子；deferred 先例/事件硬化细节下沉，文档原有） | e2e-testing.md |
| t10 | base64 | 9 | 原样级 | cross-layer-contracts.md §编码契约 |
| t11 | testutil | 10 | 钩子保留（函数全列→「等六个」，全列本轮补入文档） | perf-baseline.md §17（**下沉**） |
| t12 | 安全扫描 | 11 | 钩子保留 | security-scan.md |
| t13 | session.json | 12 | 钩子保留（复用 SettingsService 模式细节本轮补入文档） | cross-layer-contracts.md 快照 Scenario（**下沉**） |
| t14 | AI 结构化 | 13 | 钩子保留 | ai-structured-output.md |
| t15 | 纯 prompt 模式 | 14 | 钩子保留（细节本轮补入文档——文档原本缺失） | ai-structured-output.md §纯 prompt（**下沉**） |
| t16 | seed 白名单 | 15 | 钩子保留 | ai-structured-output.md §seed 合并 |
| t17 | 事件注销 | 16 | 钩子保留 | cross-layer-contracts.md §共听 Scenario |
| t18 | 设计令牌 | 17 | 钩子保留 | design-tokens.md |
| t19 | 视觉规范 | 18 | 钩子保留（kbd/间距细节等砍 3 钩子，文档原有） | frontend-visual-conventions.md |
| t20 | EventSink | 19 | 钩子保留（三实现/协议细节文档原有） | browser-channel.md |
| t21 | 终端多 tab | 20 | 钩子保留 | terminal-multi-session.md |
| t22 | applySink+Chat | 21 | 钩子保留（存储布局/--resume/processFactory 略去，文档原有） | ai-chat-service.md |
| t23 | Linux | 22 | 钩子保留（CGO 依赖图/资产名细节留，Ubuntu 包背景砍） | linux-platform.md |
| t24 | 测试稳定 | 23 | 钩子保留 | test-stability.md |
| t25 | 统计缓存 | 24 | 钩子保留；**文档列「—」→ perf-baseline §18** | perf-baseline.md §18（**下沉**，原全表唯一无承接行） |
| t26 | 原子写 | 25 | 钩子保留 | data-atomic-write.md |
| t27 | soak/泄漏 | 26 | 钩子保留（Linux 泄漏教训/watchProcess 修法下沉，文档 §13/14 原有） | perf-baseline.md §13/14 |
| t28 | 文件树截断 | 27 | 钩子保留（哨兵路径合成式/locateNode 细节部分砍，文档 §15.6 原有） | perf-baseline.md §15.6 |
| t29 | FileDiffDialog | 28 | 钩子保留（驻留脚本细节本轮补入文档） | perf-baseline.md §16.5（**下沉**） |
| t30 | Git 取消链路 | 29 | 钩子保留（util API 全名/超时两段收口史下沉——本轮补文档 §9） | logging-and-errors.md §9（**下沉**）+ §4.3 + 超时收口段（原有） |
| t31 | 内容搜索超时 | 30 | 钩子保留 | logging-and-errors.md（原有） |
| t32 | 进程管理 | 31 | 钩子保留（netstat/tasklist 解析细节略去，文档原有） | process-management.md |
| t33 | 批量重试 | 32 | 钩子保留 | logging-and-errors.md（原有） |
| t34 | PPT GC+素材拆分 | 33 | 钩子保留 | pptx-split.md（原有） |
| t35 | PPT 按页 | 34 | 钩子保留 | pptx-split.md（原有） |
| t36 | PPT 批量 | 35 | 钩子保留 | pptx-split.md（原有） |
| t37 | 工具箱面板化 | 36 | 钩子保留 | frontend-visual-conventions.md（原有） |
| t38 | 素材管理 | 37 | 钩子保留（迁移/事件细节微砍，文档原有） | assets-management.md |
| t39 | COM 队列 | 38+39 | **拆两行**（队列机制 / 导出跳转）；Export 尺寸/WPS 双开/jumpCh/冒烟预检/关停细节微砍 | assets-thumbs-com.md（原有） |
| t40 | 设置消费 PR3 | 40 | 钩子保留 | assets-thumbs-com.md §9（原有） |
| t41 | v5 标注 | 41 | 钩子保留 | assets-management.md（原有） |
| t42 | v6 批量标签 | 42+43 | **拆两行**（批量 API+组件 / 刷新链+D8 拖拽） | assets-management.md §9（原有） |
| t43 | facet v7 | 44 | 钩子保留 | assets-management.md §3/§8.2/§8.6（原有） |
| t44 | 排序 v8 | 45 | 钩子保留 | assets-management.md §10.1（原有） |
| t45 | 评分 v8 | 46 | 钩子保留 | assets-management.md §10.2（原有） |
| t46 | 导出元数据 | 47 | 钩子保留 | assets-management.md §11.1（原有） |
| t47 | lost 状态机 | 48 | 钩子保留 | assets-management.md §11.2/§11.3（原有） |
| t48 | 排除规则 | 49 | 钩子保留 | assets-management.md §12.1/§12.2（原有） |
| t49 | 备份还原 | 50 | 钩子保留 | assets-management.md §12.3/§12.4（原有） |
| t50 | 购物栏合并 | 51 | 钩子保留（冲突三重细节微砍） | pptx-merge.md（原有） |
| t51 | 刷新入口 | 52 | 钩子保留 | assets-management.md §8.7（原有） |
| t52 | pptx 预览 | 53 | 钩子保留（索引 SaveJSON/防抖细节微砍） | pptx-preview.md（原有） |
| t53 | confirm 三形态 | 54 | 钩子保留 | confirm-dialog.md（原有） |
| t54 | 依赖分档 | 55 | 钩子保留 | frontend-dep-upgrade.md（原有） |
| t55 | mermaid | 56 | 钩子保留 | mermaid-rendering.md（原有） |
| t56 | major 升级 | 57 | 钩子保留 | frontend-dep-upgrade.md §3.6（原有） |
| t57-t67 | 安全三期+连接管控一~十期（11 行） | 58 | **合并 1 行**（6 个跨期通用钩子 + 分期锚点；各期明细全部在 browser-channel.md 分期章节原有） | browser-channel.md §2.2/§2.3/§3/§4/§6 + logging-and-errors.md §8 |
| t68 | 审计留存十一期 | 59 | 钩子保留 | logging-and-errors.md §8.2（原有，承接极详尽） |
| t69 | 设置段双单点 | 60 | 钩子保留（2026-10-28 先例行再精简） | cross-layer-contracts.md 设置段 Scenario（原有） |

## 行数变化

* 拆分：t39 → 38+39；t42 → 42+43（+2 行）
* 合并：t57-t67（11 行）→ 58（-10 行）
* 68 + 2 - 10 = 60 行 ✓

## 顺带修复

* t42/t62（原表）规则列含未转义 `||`（`slideNum||0`、`multiDevice||ipBans.length>0`）致 markdown 表格多切列——新表已转义 `\|\|`

## 阶段 1 补齐的承接（先补文档后瘦身）

| 位置 | 内容 |
|---|---|
| logging-and-errors.md §9（新） | Git 取消链路 util 层 ctx 变体 API、注册表模式、覆盖清单、单测构造、锁残留探测（原 §9 相关文档顺移 §10） |
| ai-structured-output.md §纯 prompt（新） | 纯 prompt 模式契约（Command 空 + PromptTemplate、validateFunctions 放宽、BuildStagePrompt） |
| cross-layer-contracts.md 快照 Scenario（新） | data/session.json 崩溃恢复快照契约 |
| perf-baseline.md §16.5（新） | FileDiffDialog 关闭释放双时机契约 |
| perf-baseline.md §17（新） | util/testutil 测试基建单一数据源 |
| perf-baseline.md §18（新） | 统计类派生聚合缓存两入口联动失效（原行 25 文档列「—」的承接落点） |

## docs/spec/README.md 索引同步

* 补 4 缺失条目：ai-chat-service.md、assets-thumbs-com.md、linux-platform.md、pptx-preview.md
* 更新 3 条摘要：cross-layer-contracts.md（快照+设置段双单点）、logging-and-errors.md（取消链路 §9+审计 §8）、perf-baseline.md（§15.6/§16.5/§17/§18）

## 字数度量口径说明

* 「字符」= python `len()`（UTF-8 码点），非字节
* 旧表 49,191 字符（数据 68 行）；新表 15,984 字符（数据 60 行），**-67%**
* CLAUDE.md 全文件：87,340 字节 → 29,438 字节（-66%）
* 长度口径：Q1 分档上限按「整行字符数（含文档列 ~50 字符链接开销）」执行，硬顶 350 全表达标
