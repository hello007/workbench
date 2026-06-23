# 发版 skill 与脚本

## Goal

为 WorkBench 新建一个"发版"能力（skill + 脚本），把当前手动的版本发布流程自动化：从 `wails.json` 当前版本号和近期提交计算下一个默认版本号与 git tag（`v` + 版本号），更新版本号、提交、打 tag 并推送，最终由已有的 `release.yml` CI 自动构建与发布。在用户说"发版"时自动执行；也支持手动指定版本号。

## Requirements

* 新建确定性脚本 `scripts/release.sh`，封装完整发版编排
* 支持三种版本指定方式：`--bump patch|minor|major`（默认智能推荐）、`--version X.Y.Z`（手动指定）、无参数交互
* 自动更新 `wails.json` 的 `info.productVersion`
* commit 信息：`chore: bump version to X.Y.Z`
* 打 tag：`vX.Y.Z`（纯版本号，不带日期后缀）
* `git push` + `git push --tags` 触发 CI
* 新建 skill `.claude/skills/release/SKILL.md`，`description` 含"发版/发布/release"关键词，支持自然语言"发版"触发
* skill 智能层：扫描自上个 tag 以来的提交，按 `feat:`→minor / `fix,chor:`→patch / `BREAKING`→major 推荐递增级别
* 新建 `/release` slash command 作为显式入口

**已确认决策：**

| 维度 | 决策 |
|---|---|
| 发版范围 | 算版本号 → 改 wails.json → commit → tag → push（含 --tags），push 前确认版本号；构建发布交给 release.yml CI |
| 版本递增 | skill 默认按提交类型智能推荐；脚本提供 patch/minor/major/指定版本号 确定性参数 |
| 触发方式 | skill `description` 关键词实现自然语言触发；`/release` slash command 作显式入口 |
| 健壮性 | 工作区干净检查 + 当前分支检查（master）+ dry-run 预览 + push 失败回滚提示 + 版本格式校验 + tag 冲突检查 |

## Acceptance Criteria

* [ ] `./scripts/release.sh --dry-run --bump patch` 正确打印 `1.0.9 → 1.0.10` 的完整计划且不执行任何写操作
* [ ] `./scripts/release.sh --bump minor` 将 productVersion 改为 `1.1.0`，并完成 commit/tag/push
* [ ] `./scripts/release.sh --version 2.0.0` 支持手动指定版本号
* [ ] 在非 master 分支或有未提交改动时，被检查拦截（`--allow-dirty` 可绕过工作区检查）
* [ ] 目标 tag 已存在时报错退出
* [ ] 版本号格式非法（如 `1.0`）报错退出
* [ ] 说"发版"能触发 skill，并基于近期提交智能推荐 bump 级别
* [ ] push 失败时打印明确回滚指引（已执行到哪步、如何撤销）

## Definition of Done

* `release.sh` 通过 `bash -n` 语法检查，至少一次 dry-run 端到端验证
* skill 能被"发版"语义触发，`/release` 可显式调用
* wails.json 版本号更新格式正确（JSON 合法）
* 功能说明/README 按需补充发版流程说明

## Technical Approach

**两层架构：**

* **`scripts/release.sh`（确定性层）**：纯 bash，接受参数，执行原子步骤，做所有校验。可脱离 skill 独立运行。复用 `build.sh` 的 wails.json 解析方式（grep `productVersion`）。
* **`.claude/skills/release/SKILL.md`（智能编排层）**：Claude 在"发版"触发时，先用 `git log <last-tag>..HEAD` 扫描提交并智能推荐 bump 级别，与用户确认版本号后调用 `release.sh` 执行，最后报告结果与 CI 链接。

**脚本关键参数：**

```
./scripts/release.sh [--bump patch|minor|major] [--version X.Y.Z] [--dry-run] [--yes] [--allow-dirty]
```

**校验顺序（任一失败即中止）：**

1. 当前分支 = master（否则报错，提示 `--allow-dirty` 不影响此项）
2. 工作区干净（否则报错，`--allow-dirty` 绕过）
3. 新版本号格式合法（`^\d+\.\d+\.\d+$`）
4. 目标 tag `vX.Y.Z` 不存在
5. dry-run 则只打印计划并退出

**执行步骤：** 改 wails.json → `git add wails.json` → `git commit` → `git tag` → `git push` → `git push --tags`。每步出错打印回滚指引。

## Decision (ADR-lite)

* **Context**：发版后半段（tag → CI → Release）已由 `release.yml` 全自动，缺前半段本地编排；手动改版本号+commit+tag+push 易错且重复。
* **Decision**：用「确定性脚本 + 智能 skill」两层。脚本可独立运行保证可重复与可测试；skill 提供自然语言触发与智能版本推荐。
* **Consequences**：依赖 git-bash 环境（与现有 `build.sh` 一致）；不依赖 commit message 严格规范（智能推荐仅作参考，用户可手动覆盖）。

## Out of Scope

* 本地预构建验证（CI 已在 Windows 构建，避免重复）
* 自动同步 README/文档中的版本号标注（文档版本更新遵循既有"功能完成后确认是否更新 README"惯例）
* prerelease 版本特殊逻辑（如 `1.1.0-beta.1`）——可通过 `--version` 手动指定实现，不单独建模
* 跨远程多仓库发版（当前仅 Gitee origin）

## Technical Notes

* 版本号唯一来源：`wails.json` 的 `info.productVersion`
* 现有 tag：v1.0.0～v1.0.9；最新 `v1.0.9` 已推送至 origin
* CI 触发条件：`push tags: v*`（`.github/workflows/release.yml`）
* 远程：`git@gitee.com:liu1204/personal-tool-set.git`
* 脚本风格对齐 `scripts/build.sh`（中文注释、`set -e`、grep 解析 wails.json）
