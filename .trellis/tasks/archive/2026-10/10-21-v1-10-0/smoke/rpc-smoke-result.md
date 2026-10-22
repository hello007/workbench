# v1.10.0 RPC 冒烟留痕（2026-10-22）

执行方式：`workbench.exe --serve --listen=127.0.0.1:36201`（wails build 产物，独立 cwd `smoke/run/`，与桌面实例隔离）+ `/api/rpc` Bearer 驱动。
脚本：`rpc_smoke_v1100.py`（fixture 程序化构造：pptx 复刻 service 测试形态，slide 文件名与页序刻意错开）。

## 结果：35/35 PASS

### G. git 全功能回归（go-git 出清后）— 18/18 PASS

| 项 | 内容 | 结果 |
|---|---|---|
| G1.1 | GetGitInfo 分支/仓库态 | PASS |
| G1.2 | 本地变更识别（M/?） | PASS |
| G1.3 | 暂存 diff 非空 | PASS |
| G1.4 | CommitFiles 后工作区干净 | PASS |
| G1.5 | 提交历史与主题 | PASS |
| G2.1 | 分支列表 | PASS |
| G2.2 | Merge ff 快进 HEAD 前移 | PASS |
| G2.3 | rebase 冲突态识别 | PASS |
| G2.4 | AbortRebase 收口 | PASS |
| G3.1 | merge 冲突态（type=merge, files=[f1.txt]） | PASS |
| G3.2 | 解决后 ContinueMerge 完成 | PASS |
| G3.3 | CherryPick 无冲突态完成 | PASS |
| G3.4 | CherryPick 文件落地 | PASS |
| G4.1 | AddRemote 回读 | PASS |
| G4.2 | Push 后 ahead=0 | PASS |
| G4.3 | Fetch 后 behind=1 识别 | PASS |
| G4.4 | Pull 后 ahead/behind 清零、HEAD 前移 | PASS |
| G4.5 | 远程文件落工作区 | PASS |
| G4.6/4.7 | CreateTag/GetTags/PushTag/DeleteTag | PASS |

### P. PPT 域 — 9/9 PASS

| 项 | 内容 | 结果 |
|---|---|---|
| P1.1/1.2 | 按页拆分 3 页 + 逐页产物 zip 各 1 slide | PASS |
| P1.3/1.4 | 文件夹批量拆分 2 文件 + 重扫排除派生产物 | PASS |
| P2.1 | 建库扫描（含中文目录名） | PASS |
| P2.2/2.3 | 入栏 3 条 + 重复入栏幂等 | PASS |
| P2.4/2.5 | 键形态契约 + Reorder 重排 | PASS |
| P2.6/2.7 | 合并导出 3 页无跳过 + 产物 zip 验证 | PASS |
| P2.8 | stale 条目跳过并报告 SkippedItems | PASS |
| P2.9 | ClearAssetCart 清空 | PASS |
| P3.1 | COM 转 PDF 产物落盘（绝对路径） | PASS |

## 冒烟发现与处置

| # | 级别 | 问题 | 处置 |
|---|---|---|---|
| 1 | P1（已修） | `ConvertPptxForPreview` 返回 PdfPath 为相对路径，与 model 注释「缓存 PDF 绝对路径」契约漂移（前端拼 /preview-pdf 同源 URL 同 cwd 语义下碰巧可用，serve cwd 变化场景脆弱） | service 返回点 `filepath.Abs` 归一（lookup/convert 两返回点 + absCachePath helper 降级不报错）+ 新增 `TestConvertForPreview_ReturnsAbsPath`；重建产物复测 P3.1 PASS。改动：service/pptx_preview.go、service/pptx_preview_test.go |
| 2 | 记录（非缺陷） | 脚本用「已被 merge 带入的提交」做 cherry-pick 源 → git 空 pick 留 CHERRY_PICK_HEAD，`conflictExitCodes` 按冲突口径接受 exit 1 属既有取舍；GetConflictState 如实报 cherry-pick 进行中。场景修正后 PASS | 无产品改动；冒烟脚本留修正后场景（独立分支不 merge 的提交作源） |
| 3 | 记录（口径） | `GetTags` 空 tag 返回 `null` 非 `[]`（RPC 翻译层 nil slice→null）；前端消费容错 | 不改；前端以 truthy 判定 |

## UI 呈现面

见 `ui-checklist.md`（待用户手点）。
