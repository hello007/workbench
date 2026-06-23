---
name: release
description: WorkBench 发版流程。当用户说"发版/发布版本/打 tag/版本发布/release"时使用：根据 wails.json 当前版本和近期提交智能推荐新版本号与 tag，确认后调用 scripts/release.sh 完成版本更新、提交、打 tag、推送，触发 GitHub Actions 自动构建发布。
---

# WorkBench 发版流程

本 skill 负责将 WorkBench 从当前版本发布到下一版本，触发 GitHub Actions（`.github/workflows/release.yml`）自动构建并发布 Release。

## 核心约定

- **版本号唯一来源**：`wails.json` 的 `info.productVersion`。
- **严禁修改** `wails.json` 顶层 `"version": "2"`（那是 wails schema 版本）。
- **远程约定**（见 README）：远程 `origin` 为 Gitee，是唯一主仓库。推送后 Gitee 会自动镜像到 GitHub，从而触发运行在 GitHub Actions 上的 `release.yml`（监听 GitHub 上的 `v*` tag）自动构建并发布。脚本遍历所有已配置 remote 推送 commit 与 tag（当前仅有 `origin`），**无需在本仓库配置 github remote，也无需手动同步 tag 到 GitHub**。
- **push 不可逆**，执行前必须向用户确认版本号。

## 流程

### 1. 读取当前版本

读取 `wails.json` 的 `info.productVersion`，作为 `CURRENT_VERSION`。

### 2. 取最近 tag 并扫描提交

```bash
# 取最近 tag；无 tag 时 LAST_TAG 为空，下方自动退化为扫描全部提交
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || true)
if [ -n "$LAST_TAG" ]; then
    git log "${LAST_TAG}"..HEAD --oneline
else
    # 无历史 tag：扫描全部提交用于智能推荐
    git log --oneline
fi
```

> 上述无 tag 分支仅影响"智能推荐"的扫描范围，不阻断发版；版本号递增级别仍可由用户手动指定。

### 3. 按提交类型推荐 bump 级别

扫描 `$LAST_TAG..HEAD` 范围内的提交消息，按下表自上而下匹配（命中即停）：

| 提交特征                                | bump 级别 |
| --------------------------------------- | --------- |
| 含 `BREAKING CHANGE` 或提交前缀 `major:` | major     |
| 含前缀 `feat:`                          | minor     |
| `fix:` / `chore:` / `docs:` / `style:` / `refactor:` / `perf:` / `test:` | patch     |

> 此处为智能推荐，属于 skill 职责；脚本本身保持确定性，不做推荐。

### 4. 计算新版本并请求确认

- 由推荐级别计算 `NEW_VERSION`（major：高位 +1，后位清零；minor：中位 +1，末位清零；patch：末位 +1）。
- 目标 tag = `v$NEW_VERSION`。
- 向用户展示：`当前版本` → `新版本`、推荐 bump 级别、目标 tag，并请求：
  - 确认推荐；或
  - 手动指定版本号（形如 `X.Y.Z`）。

### 5. 调用发版脚本

确认后执行（加 `--yes` 由 skill 已完成确认）：

```bash
# 按推荐级别
./scripts/release.sh --bump <级别> --yes

# 或手动指定
./scripts/release.sh --version <指定版本> --yes
```

脚本内部校验链：分支为 master、工作区干净、版本格式合法、tag 不存在；随后提交 `wails.json`、打 tag、推送。

> 脚本可独立运行，详见 `./scripts/release.sh --help`。

### 6. 报告结果

向用户报告：

- 新版本号与 tag
- 各 remote 的 push 结果（成功/失败）
- CI 提示：已推送至 `origin`（Gitee），Gitee 会自动镜像到 GitHub，`.github/workflows/release.yml` 据此自动构建并发布 Release，可在 GitHub 对应仓库的 Actions 页面观察进度；无需手动同步 tag 到 GitHub。

## 注意事项

- **版本号必须由用户确认**，push 后无法撤销。
- 若工作区不干净，先引导用户提交改动；除非用户明确要求，不要使用 `--allow-dirty`。
- 若 push 失败，按脚本输出的回滚指引处理：`git tag -d v$NEW && git reset --hard HEAD~1`；若部分 remote 已成功，回滚前需先到对应平台删除已推送的 tag。
