# origin 切换为 GitHub SSH

## Goal

将本地 `origin` 从 HTTPS 改为 SSH（`git@github.com:hello007/workbench.git`），解决 HTTPS 推送网络/凭据不可用问题；同步全部「当前流程」文档中的 HTTPS 表述，保持单一远端与提交规范一致。

## What I already know

- 上一任务已将 origin 统一为 `https://github.com/hello007/workbench.git`，并移除 mirror。
- HTTPS 推送实测：连接间歇重置 + 无凭据（`No anonymous write access`）；SSH（`git@github.com:hello007/workbench.git`）可正常 `ls-remote` 与推送。
- 文档中 HTTPS 远程地址共 8 处：`README.md:131`、`docs/开发规范.md:279`、`docs/贡献指南.md:24/209/217`、`.claude/skills/release/SKILL.md:14/80`、`scripts/release.sh:246/277`、`docs/快速入门.md:41`、`CLAUDE.md:132`（`origin=GitHub HTTPS`）。
- `docs/快速入门.md:103/219` 的 HTTPS 指应用自身 TLS/网络访问，**不得误改**。

## Requirements

- R1 本地远端：`git remote set-url origin git@github.com:hello007/workbench.git`；`git remote -v` 仅 origin（fetch/push 同 SSH URL）；`git ls-remote origin master` 成功。
- R2 文档同步：上述 8 处当前流程表述改为 SSH：
  - 克隆命令改 `git clone git@github.com:hello007/workbench.git workbench`，并注明需已配置 GitHub SSH key。
  - 推送/CI 说明中的 URL 与「HTTPS」字样改 SSH；无镜像中转结论不变。
  - `CLAUDE.md` 规则行 `origin=GitHub HTTPS` 改 `origin=GitHub SSH`，行长 ≤250。
- R3 `scripts/release.sh` 仅注释与提示文案；逻辑不变。
- R4 不改业务代码、CI 配置、依赖；不动应用自身 TLS/网络访问的 HTTPS 描述。
- R5 历史归档/日志中的 HTTPS 提及不回改。

## Acceptance Criteria

- [ ] `git remote -v` 为 `git@github.com:hello007/workbench.git`（fetch/push）。
- [ ] `git ls-remote origin master` 返回远端 HEAD。
- [ ] 当前流程文档中无 `https://github.com/hello007/workbench` 残留（历史归档除外）；`HTTPS` 仅保留应用 TLS 语义。
- [ ] `CLAUDE.md` 规则行 ≤250 字符。
- [ ] `bash -n scripts/release.sh` 通过。
- [ ] `git diff --check` 通过。

## Definition of Done

- 远端与文档一致；提交、归档。
- 后续 `git push origin master` 可直接走 SSH。

## Decision (ADR-lite)

**Context**：HTTPS 推送在本机网络下不可用且缺少凭据；SSH 已验证可推送。

**Decision**：origin 改 SSH，文档统一；克隆说明要求 SSH key。

**Consequences**：新环境需配置 SSH key；无 HTTPS fallback 说明（可在贡献指南注明排障时临时使用 HTTPS）。

## Out of Scope

- 配置 SSH key、修改 GitHub 仓库设置。
- 恢复 mirror 或其它 remote。
- 应用网络访问/TLS 文档。

## Technical Notes

- SSH 已验证：`git ls-remote git@github.com:hello007/workbench.git master` 成功。
- 涉及文件：`CLAUDE.md`、`README.md`、`docs/开发规范.md`、`docs/贡献指南.md`、`docs/快速入门.md`、`.claude/skills/release/SKILL.md`、`scripts/release.sh`。
