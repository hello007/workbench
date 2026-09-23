# 外部 diff 三向合并支持

## Goal

外部 diff 工具集成唯一留白（路线图 L231）：冲突解决场景一键拉起外部合并工具（base/local/remote/merged），衔接现有 merge/rebase 冲突辅助链路。

## Requirements

* **brainstorm 决策（桥接未获应答按推荐项推进，均可低成本调整）**：
  * 交互入口 → **冲突文件列表行级「外部合并」按钮**（GitMerge.vue 冲突行现有「打开/标记已解决」按钮组追加；批量拉起会多窗口并发写同一 merged 文件，行级精准）
  * 模板配置 → **预设名内置三向模板（后端 map，用户零配置）+ custom 走 settings 新字段 `DiffToolMergeArgs`**。预设模板（git mergetools 官方口径 + 工具官方 CLI 文档核实）：beyondcompare `{local} {remote} {base} {merged}`（git mergetools/bc + BC 官方论坛确认）；winmerge `-e -u -wl -wr -dl Local -dm Base -dr Remote {local} {base} {remote} -o {merged}`（WinMerge 手册三方形式 + 社区讨论 #1853）；vscode `--wait --merge {remote} {local} {base} {merged}`（VSCode CLI + git mergetools/vscode）；kdiff3 `{base} {local} {remote} -o {merged}`；meld `{local} {base} {remote} --output={merged}`
  * merged 语义 → **merged 传工作区原文件绝对路径**（外部工具直接编辑工作区文件保存，随后用户点现有「标记已解决」（git add）衔接存量链路，零新增流程）
* 外部工具配置扩展三向模板：占位符 `{base}` `{local}` `{remote}` `{merged}`（base/local/remote 恒写临时文件，复用 DiffTempRoot 生命周期策略：启动后不删、下次启动 CleanupDiffTempDir 清理）
* 冲突场景文件解析：`:1:`(base，add/add 冲突无 :1 降级空文件)/`:2:`(local)/`:3:`(remote) 经 `git show :N:file` 提取；`:2:`/`:3:` 缺失视为该文件不在冲突状态报错；工作区文件不存在（delete/modify 冲突）报错引导先恢复
* 拉起外部工具后 ElMessage 引导「保存合并结果后点标记已解决」（复用现有 `git add` 标记链路）

## Acceptance Criteria

* [x] 三向模板配置 + 占位符渲染 + 临时版本文件生命周期管理（用后清理）——预设内置模板 map（5 工具，git mergetools 官方口径核实）+ custom 走 DiffToolMergeArgs；RenderMergeArgsTemplate 四占位符渲染（util 表驱动测试）；临时文件复用 DiffTempRoot 生命周期（启动后不删、下次启动 CleanupDiffTempDir 清理）
* [x] 冲突入口 + E2E/单测（真实 merge 冲突 fixture）——GitMerge.vue 冲突行「外部合并」按钮（组件测试 2 例：调用参数 + 成功/失败分流）；service 单测 6 例基于真实 merge 冲突仓库（stage 提取内容断言、启动成功/失败、非冲突文件、工作区删除）；实现中发现并修复 stage 缺失文案口径 bug（`is in the index, but not at stage` 与 diff 场景 isRevPathMissing 不同，新增 isStageMissing 判断）
* [x] docs/功能说明.md、docs/路线图.md 同步（L231 勾选）
* [x] 收口验证：go test ./... 6 包全绿；前端 vitest 1390 全过；npm run build 通过；coverage-check 全 PASS（util 56.3%）；gofmt/vet 干净；wailsjs 绑定三处同步

## Out of Scope

* 内置三方合并编辑器；非冲突场景的普通 diff（已有）

## Technical Notes

* 现有基础：外部 diff 工具预设模板 + FileDiffDialog 一键打开（2026-09-13 交付）；冲突文件列表 + `git add` 标记已解决链路已有
