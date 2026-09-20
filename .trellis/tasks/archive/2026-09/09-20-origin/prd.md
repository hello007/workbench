# 提交历史 origin 远程分支节点醒目色标识

## Goal

提交历史行内 refs badge 中，origin 远程分支（如 `origin/master`）节点改用醒目颜色，让用户扫一眼即定位远程头所在提交。

## Requirements

* 提交行 remote 类 badge（`CommitRefKindRemote`，如 `origin/master`）由 info 蓝 plain 改为 **warning 橙 + 实底**（el-tag `type="warning" effect="dark"`）。
* 仅改提交行 badge；本地分支（primary plain）、HEAD（danger plain）不变；BranchSyncBar 摘要条不动。
* 同步更新受影响组件测试断言（badge type/effect）。

## Acceptance Criteria

* [ ] 提交行 origin/xxx badge 呈 warning 橙实底，视觉显著区别于本地/HEAD badge。
* [ ] 同步共标（本地+远程同 commit）时两 badge 并列可辨。
* [ ] `cd frontend && npm test` 绿。

## Out of Scope

* BranchSyncBar 摘要条配色、badge 形状/布局、后端任何改动。

## Technical Notes

* 改动点预估仅 `frontend/src/components/CommitHistory.vue` badge 渲染处 + `CommitHistory.spec.js` 断言。
* 视觉依据：frontend-visual-conventions 语义色选色（warning=注意类）+ design-tokens；user 已确认 warning 橙实底方案（候选 danger 红因常驻误暗示异常被否）。
