---
title: 约定文件（conventions/）编写与变更同步规范
scope: [docs, conventions]
targets: [convention]
---

# 约定文件（conventions/）编写与变更同步规范

> 适用场景：在 `docs/agent-resources/conventions/` 下新增、修改或删除约定文件。
> 约定是所有 coding agent 共享的「该怎么做」，同时是规则派生物（`.claude/rules/`、`.opencodereview/rule.json`）的唯一来源——改了源文件就必须同步，否则 agent 读到的规则会与约定漂移。

| 项 | 值 |
|----|----|
| 类型 | 约定 |
| 创建时间 | 2026-11-05 |

## 文件格式

每个约定文件是一篇 Markdown，结构必须为：

1. **frontmatter（必填）**，三字段齐全：

   ```yaml
   ---
   title: 约定标题
   scope: [适用范围标签]
   targets: [目标对象标签]
   ---
   ```

   | 字段 | 说明 |
   |------|------|
   | `title` | 约定的完整标题，与正文一级标题一致 |
   | `scope` | 适用范围标签（如 `docs`、`vue`、`build`） |
   | `targets` | 规则作用的目标对象标签（如 `vue`、`store`、`transport`、`go`、`convention`）；同步时据此推导规则生效的文件 glob，**填错会导致规则挂到错误的文件上** |

2. **正文**：一级标题与 `title` 一致；开头用引用块说明适用场景；规则以祈使句书写，能判定「这样写违规」。参考同目录其他约定文件的写法。

3. 文件名用 **kebab-case + 域前缀**（`coding-*` / `process-*` / `project-*` / `testing-*`），见名知义。

## 变更后必须同步

约定文件的**新增、修改、删除**都必须同步派生物（本仓未安装约定同步 skill，由 AI 会话内直接执行编译）：

- `.claude/rules/<同名>.md`——带 `paths` frontmatter 的规则指针；删除约定文件时对应指针一并清理
- `.opencodereview/rule.json`——按文件 glob 分组的评审规则

同步操作：读取每个 convention 文件的 frontmatter `targets`，结合本仓实际目录（`frontend/src/store/`、`frontend/src/transport/` 等）推导 glob，全量重写两类派生物（内容未变的也要重写一遍，保持口径一致）。
