# 实施笔记：session-start hook 旧路径死代码出清（2026-10-29）

## hook 实跑调用方式

```bash
echo '{"cwd":"D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager"}' \
  | python .claude/hooks/session-start.py > baseline-output.txt 2> baseline-stderr.txt
```

- 空 JSON `{}` 也可跑通（`cwd` 缺省回落 `.`）；显式传 `cwd` 保证解析到仓库根。
- 两次基线实跑 diff=0（输出无时间戳/session id 噪声，可逐字节比对）。
- 改后同命令实跑存 `after-output.txt`，exit=0。

## Requirement 9 前置核实结果

grep `is_mono|packages|scope_config|allowed_pkgs|task_pkg|default_pkg` 于 session-start.py：
全部消费方集中在 `_load_trellis_config` / `_check_legacy_spec` / `_resolve_spec_scope` 三函数及 main() 672-677 级联加载、748 行 `allowed_pkgs` 消费（#5 扫描块）内，无其他消费方。全仓 grep 三函数名仅本文件命中。

**实施决策（prd Requirement 9 延伸）**：`_load_trellis_config`（397-438）是 672-677「级联加载」的实现载体，#8/#9 删除后成为零调用死函数，一并删除。符合「死代码出清」任务目标。

## 实施过程中的两个技术事实

1. **文件为混合行尾**（CRLF 与孤立 CR 混杂），`splitlines` 与 Read 工具行号错位（Read 797 行 vs splitlines 796 行），行号脚本断言失败后改为**锚文本定位脚本**（`edit-session-start.py`，按 strip 前缀匹配 + 逐行断言后删除），对行尾风格免疫，未触碰未改动行的字节。
2. 删除三函数后残留 4 空行（前函数尾 2 + 保留 2），收敛为 PEP8 的 2 空行（588 → 586 行）。

## diff 比对结论（baseline-output.txt vs after-output.txt）

additionalContext 结构化逐行 diff：removed 6 / added 7，逐条对应：

| 差异 | 对应 Requirement |
|---|---|
| first-reply-notice 删「、spec 索引已加载」 | R2 |
| PATHS 段 `Spec: .trellis/spec/` 行消失 | R10 |
| guidelines 段头「Project spec indexes are listed by path below...」句消失（随删 1 空行） | R3 |
| 「Discover more via: --mode packages」行消失 | R7 |
| git status 噪声 5 行（`1→6 uncommitted changes`、3 个 `M` 行、`?? __pycache__/`） | 改动生效的必然反映，非功能差异；`__pycache__` 已清理 |

- Phase 1.3 文案（R1）未出现在 diff：当前任务状态 READY，该分支不触发；源文件级 grep 确认新文案已是 `docs/spec/**/*.md` + `docs/spec/README.md`，`.trellis/spec` 零命中。
- R4/R5/R6/R8/R9 为死代码删除，输出本就不出现，diff 无差异即符合预期。

## grep 复核（verify-grep.txt，243 行命中分类）

改动三文件 `.claude/hooks/session-start.py`、`.trellis/scripts/common/session_context.py`、`.trellis/scripts/task.py` 全部零命中。

剩余命中全部落在白名单或说明性引用：

- 历史记录：`tasks/archive/**`、`workspace/*/journal-*`、`workspace/liuyang/index.md`、`CHANGELOG.md`、本任务目录（prd/baseline-run2.txt）
- 前任务白名单：`trellis-meta/**`、`trellis-spec-bootstarp/**`、before-dev line 12（`--mode packages` 文件引用，无 spec 字样）
- 上游脚本白名单：`paths.py`（get_spec_dir docstring）、`packages_context.py`（198/207 spec 路径打印）
- 说明性/纠偏性引用（非活引导，保留）：`workflow.md`（29/34/135/390/566/686，废弃声明与 informational 引用）、`CLAUDE.md`（47/142，纠偏规则本体）、`docs/spec/README.md`（3/26/67，迁移记录与禁令）、`docs/贡献指南.md`（329，纠偏说明）、`AGENTS.md`（9 块内原文 + 23 本任务新增纠偏行）

## 改动面

```
 .claude/hooks/session-start.py             | 219 行变化（净 -211，797 → 586）
 .trellis/scripts/common/session_context.py |   2 -（DIR_SPEC import + Spec: 行）
 .trellis/scripts/task.py                   |   2 +-（help 示例路径 + 描述文本同步改 Cross-layer contracts）
 AGENTS.md                                  |   2 +（TRELLIS:END 块外追加纠偏行）
```

workflow.md / CLAUDE.md / docs/spec/ / workbench 业务代码零改动；白名单文件（packages_context.py / paths.py / get_context.py / trellis-meta / spec-bootstarp / before-dev）零改动。

## 遗留提醒（主会话收尾）

- journal-3.md 已 1941/2000，finish-work 记录 session 时大概率触发 journal-4 轮转（add_session.py 自动创建带 Continuation 头的新文件），按 AC 确认并记录。
- `.template-hashes.json` 中 4 个改动文件的 hash 失配为既定实践（上游 `trellis update` 消费），无本地阻塞。
