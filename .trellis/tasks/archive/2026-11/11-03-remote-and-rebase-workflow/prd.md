# 远程改 GitHub HTTPS + 提交前 rebase 规范

## Goal

将仓库远程统一为 `https://github.com/hello007/workbench.git`（当前 origin=Gitee、mirror=GitHub SSH 双远端），并把「提交前先 fetch + rebase、禁 merge 提交」写入代码提交规范与相关文档/发版脚本提示，避免与他人提交产生 merge 节点或冲突。

## What I already know

- 当前远端：`origin` = `git@gitee.com:liu1204/personal-tool-set.git`；`mirror` = `git@github.com:hello007/workbench.git`。
- 现有文档/脚本以「Gitee 主仓 + 服务端自动镜像 GitHub」为前提：`docs/贡献指南.md`（§远程仓库/同步机制）、`README.md:122/130`、`.claude/skills/release/SKILL.md`、`scripts/release.sh` 注释与 CI 提示。
- `scripts/release.sh` 遍历所有 remote 推送，移除 mirror 后自然只推 origin。
- 目标 URL 为 HTTPS，推送需凭据（PAT/credential helper）；fetch 公共仓可匿名。

## Requirements

- R1 本地远端配置：`origin` 设为 `https://github.com/hello007/workbench.git`；移除冗余 `mirror`（与 origin 同一 GitHub 仓库，避免重复推送/双重凭据）。`git remote -v` 仅剩 origin；`git ls-remote origin master` 可读到 HEAD。
- R2 提交规范（`docs/开发规范.md` §Git 提交规范）：新增强制流程：
  1. 提交前 `git fetch origin`；
  2. `git rebase origin/master`（有本地未提交改动先 stash 或提交后再 rebase）；
  3. 冲突本地解决（`git status` / `git add` / `git rebase --continue`），禁产生 merge 提交；
  4. rebase 后跑必要校验，再 `git commit`；
  5. `git push origin master`；被拒（远端已前进）则重复 fetch+rebase 后再推；
  6. 禁对共享 `master` 使用 `git push --force`。
- R3 文档同步：
  - `docs/贡献指南.md`：远端表改为单一 `origin`（GitHub HTTPS）；克隆/推送命令更新；删除或改写「Gitee 自动镜像」同步机制段落，明确 CI 直接在 GitHub 触发。
  - `README.md`：发版/CI 说明中的 Gitee 镜像模型改为 GitHub origin 直接推送触发。
  - `.claude/skills/release/SKILL.md`：远程约定与结果提示改为 GitHub origin（无镜像）。
  - `scripts/release.sh`：注释与 CI 提示文案同步（逻辑不变，仍遍历 remote 推送）。
- R4 `CLAUDE.md` 关键规则新增一行提交流程钩子（fetch+rebase、禁 merge/force、origin 地址），链接 `docs/开发规范.md`；行长 ≤250。
- R5 不改业务代码、不改 CI 触发条件；历史归档文档中的 Gitee 描述不回改。

## Acceptance Criteria

- [ ] `git remote -v` 仅 `origin https://github.com/hello007/workbench.git`（fetch/push 同 URL）。
- [ ] `git ls-remote origin master` 返回远程 HEAD。
- [ ] `docs/开发规范.md` 含可复制命令序列，明确禁 merge 提交与禁 force push master。
- [ ] `grep -rn "Gitee\|gitee"` 在 `docs/贡献指南.md`、`README.md`、`.claude/skills/release/SKILL.md`、`scripts/release.sh`、`docs/开发规范.md` 中无「当前流程」式残留（历史记录/归档除外）。
- [ ] `CLAUDE.md` 新行 ≤250 字符，链接正确。
- [ ] `bash -n scripts/release.sh` 通过（若改动脚本）。

## Definition of Done

- 远端配置生效并验证可读。
- 文档口径一致，无自相矛盾的「主仓=Gitee」表述。
- 提交、归档。

## Decision (ADR-lite)

**Context**：双远端（Gitee origin + GitHub mirror）依赖 Gitee 服务端镜像，链路多一跳且 CI 触发有延迟；用户要求统一为 GitHub HTTPS，并以 rebase 保持线性历史。

**Decision**：origin 改为 GitHub HTTPS，移除 mirror；提交前强制 fetch+rebase；文档与脚本提示同步。

**Consequences**：推送需 GitHub HTTPS 凭据（PAT/credential helper）；历史 Gitee 仓库不再作为主仓（不删除远端数据，仅本地不再配置）。若日后需恢复镜像，重新 `git remote add` 即可。

## Out of Scope

- 删除 Gitee 上的远端仓库。
- CI/release workflow 逻辑修改（仅文案）。
- 历史 journal/归档任务中的 Gitee 描述回改。

## Technical Notes

- 远端配置属 `.git/config`，不入库；文档承担可复现说明。
- 涉及文档：`docs/开发规范.md`、`docs/贡献指南.md`、`README.md`、`.claude/skills/release/SKILL.md`、`scripts/release.sh`、`CLAUDE.md`。
