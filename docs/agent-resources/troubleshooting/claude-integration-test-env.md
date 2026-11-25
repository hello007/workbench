# claude 集成测试报「Not logged in」（settings.json env 未注入）

| 项 | 值 |
|----|----|
| 分类 | 构建 / 环境 |
| 创建时间 | 2026-11-24 |

## 现象

运行真实调用 claude CLI 的集成测试（如 `service/claude_launcher_integration_test.go` 的 `TestIntegration_ClaudeLaunchWithJsonSchema`）时，claude 子进程输出 `Not logged in · Please run /login` 并退出非 0；而同一台机器上交互式 claude 可正常使用。

```bash
go test -tags=integration ./service/ -run TestIntegration_ClaudeLaunchWithJsonSchema -v
# ... claude 退出非 0: exit status 1
# 输出末尾: … Not logged in · Please run /login …
```

## 原因

本机 claude 通过**用户级设置文件** `%USERPROFILE%/.claude/settings.json`（Linux/macOS 为 `~/.claude/settings.json`）的 `env` 段配置第三方网关与凭据，典型键为 `ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN`（亦可能用 `ANTHROPIC_API_KEY`）。

集成测试用 `exec.Command` 直接拉起 claude 子进程，子进程环境**只继承父进程环境**，不会自动加载 settings.json 的 `env` 段。当父进程环境缺少 `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY`（常见于 GUI 启动的测试运行器、IDE 或本仓库 agent 终端）时，claude 拿不到凭据，即报未登录。

注意：父进程环境里可能已有 `ANTHROPIC_BASE_URL` / `ANTHROPIC_MODEL`（来自 shell 配置），**有地址无凭据**仍会触发本症状，不能用「地址已存在」判定凭据就绪。

## 临时解法（agent 行动手册）

集成测试已内置修复：启动 claude 前读取 settings.json 的 `env` 段并合并进子进程环境（同 key 以 settings 为准），仅在凭据缺失时 `t.Skip`。验证步骤：

```bash
# 1. 只确认键存在，不打印敏感值（凭据/地址不得进入日志与提交）
python -I -c "
import os, json
home = os.environ.get('USERPROFILE') or os.path.expanduser('~')
p = os.path.join(home, '.claude', 'settings.json')
cfg = json.load(open(p, encoding='utf-8')) if os.path.exists(p) else {}
print('env keys:', sorted((cfg.get('env') or {}).keys()))
"

# 2. 跑集成测试（正常应出现「已注入 N 项 … env」日志并真调通过；
#    若无凭据则 skip 并说明「本机未配置 claude 凭据」）
go test -tags=integration ./service/ -run TestIntegration_ClaudeLaunchWithJsonSchema -v
```

已在本机验证（2026-11-24）：注入后集成测试真实通过。若仍失败，先核对 settings.json 的 `env` 段确有 `ANTHROPIC_AUTH_TOKEN` 或 `ANTHROPIC_API_KEY`，再看输出末尾的 claude 原始报错。

## 永久解法（待人为决策）

- 方案 1（已实施）：集成测试显式读取 `%USERPROFILE%/.claude/settings.json` 的 `env` 段并覆盖注入子进程环境，凭据缺失时 `t.Skip` 而非失败；日志只输出 key 名与注入计数，杜绝凭据泄露。
- 方案 2：在 CI/测试环境通过环境变量或密钥管理显式提供 `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_API_KEY`，测试不感知 settings.json——适用于无人值守 CI；本地开发仍需方案 1 兜底。
- 附注：`ANTHROPIC_BASE_URL` 一律以占位符描述，真实网关地址与 token 不得写入仓库文档。
