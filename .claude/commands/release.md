---
description: 触发 WorkBench 发版流程
---

# /release

触发 WorkBench 发版。请执行 `.claude/skills/release/SKILL.md` 中定义的完整流程：

1. 读取 `wails.json` 当前版本
2. 取最近 tag，扫描 `$LAST_TAG..HEAD` 提交
3. 按提交类型智能推荐 bump 级别（major/minor/patch）
4. 计算新版本与 tag，向我展示并请求确认
5. 确认后调用 `./scripts/release.sh` 完成版本更新、提交、打 tag、推送

注意：push 不可逆，执行前务必让我确认版本号。
