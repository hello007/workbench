# 素材拆分批量取消保留已完成明细

## Goal

`SplitAssetLibrary` 取消时当前整体失败返回空结果，前端只弹「拆分已取消」warning，已完成文件的拆分成果（cache/source 产物、replace 索引行）虽已落盘但对用户不可见、无法核对。本任务让取消也返回并展示已完成明细，前端给出三类收尾计数与逐文件明细，降低用户对「取消是否白跑」的不确定。

## Requirements

- **取消返回语义**：批量拆分因 ctx 取消中止时，返回已处理部分的 `AssetSplitBatchResult`（`Items` 顺序同处理序）+ 新增 `Canceled=true`，**error 为 nil**；终态发 `assets-split:done`（载荷含 canceled 结果），**不发 `assets-split:error`**。
- **取消来源统一**：用户取消（`CancelAssetSplit`）与关停/父 ctx 取消（`CancelAllSplits`）同按 `context.Canceled` 判定，均返回 `canceled=true`。
- **真系统性错误不变**：db 读写失败等非取消错误仍走 `assets-split:error` + error 返回。
- **Items 口径**：仅含已处理文件（`split` / `skipped` / `failed` 三态），不引入 `pending`；`Total` 保持启动快照总数，已完成数 = `items.length`，剩余 = `Total - items.length`。进行中的文件：cache/source 保留已产出、replace 回滚本文件，均不追加 Item。
- **竞态判定**：主循环结束后补一次 `ctx.Err()` 判定——最后一个文件处理期间收到取消信号则 `canceled=true`；循环自然结束且无取消信号则 `canceled=false`（正常完成）。
- **前端展示**：
  - 批量返回 `canceled=true` 时弹 `ElMessage.warning`：「拆分已取消：已完成 X/Y（拆分 a，跳过 b，失败 c）」；replace 落点沿用「已删除源 / 已回滚 / 源保留」三类计数。
  - `Items` 为空时提示「拆分已取消：尚无文件完成」，不开明细窗。
  - 明细窗沿用「`failed > 0` 才弹」规则，标题与顶部汇总带「已取消」上下文。
  - 进度复位沿用 `onSplitDone`；切库 `splitSeq` 守卫保持现状（旧库结果丢弃）。
- **契约同步**：`AssetSplitBatchResult` 新增字段须同步 `frontend/wailsjs/go/models.ts`（普通 struct 字段，Wails 生成后手动核对）。
- **文档更新**：`docs/API参考.md`（`SplitAssetLibrary` / `CancelAssetSplit` 行）、`docs/spec/assets-management.md:99`、`docs/spec/logging-and-errors.md:315`、`docs/spec/pptx-split.md` 相关表述、CLAUDE.md 关键规则「批量取消丢弃返回值」行（按两级分流规则改写钩子）。

## Acceptance Criteria

- [ ] 批量取消（含预取消 ctx）返回 `Items` 含已处理文件、计数正确、`Canceled=true`、error 为 nil
- [ ] 取消路径发 `assets-split:done`，不发 `assets-split:error`
- [ ] 进行中的文件不产生半成品 Item；replace 落点该文件产物与索引行已回滚
- [ ] 首个文件前取消：`Items` 为空、`Total` 不变、`Canceled=true`
- [ ] 最后文件处理期间取消：该文件 Item 存在且 `Canceled=true`
- [ ] 循环自然完成时 `Canceled=false`，行为与现状一致
- [ ] 关停收口用例改为断言 `Canceled=true` 且等待链正常返回
- [ ] 系统性错误（db 失败）仍返回 error 并发 `assets-split:error`
- [ ] 前端取消后展示 warning 计数；空 Items 给独立文案；`failed>0` 时明细窗带取消上下文
- [ ] 前端单测覆盖取消分支（warning 文案、空 Items、明细窗触发）
- [ ] `npm run build` 通过（wailsjs 绑定同步无 MISSING_EXPORT）

## Definition of Done

- 后端单测更新：`TestAssetSplit_CancelPreCancelled`、`TestAssetSplit_InFlightMutexAndCancel`、`TestAssetLibrary_ShutdownCancelsInFlightSplit`，新增竞态与空 Items 用例
- 前端单测更新：`AssetsPanel.spec.js` 拆分取消用例
- 覆盖率门禁通过（service ≥76%，前端 ≥70%）
- 文档与 CLAUDE.md 关键规则同步
- README 无需更新（用户可见行为变化仅提示文案，除非实现中出现新入口）

## Technical Approach

1. 主循环取消检查点（`service/assets_split.go:281-283`）改为：置 `batch.Canceled = true`、`break`，跳出循环后统一走「收尾」：日志（含 canceled）+ 发 `assets-split:done` + 返回 `(batch, nil)`。
2. 循环结束后补一次 `splitCtx.Err()` 判定竞态，置 `Canceled=true`。
3. `splitOneFileWithHash` 返回的取消错误仍以 `errors.Is(err, context.Canceled)` 识别（单文件内部不中断；replace 由 `splitOneFileReplace` 自回滚）。
4. `abortAssetSplit` 只保留真系统性错误路径（日志 + `assets-split:error` + error 返回）；取消不再经过它。
5. 单文件入口 `SplitAssetFile` 取消语义不变（仍 `E_ASSETS_CANCELLED` + error 事件）。
6. 前端 `onConfirmSplit` 库级分支：先判 `batch.canceled` → warning 摘要 + 按 `items.length` / `failed` 决定明细窗；再判 `total` 为空；else 原成功路径。`splitOutcomeItems` 明细窗标题/汇总按 canceled 分支渲染。
7. Wails 绑定：`model.ts` 的 `AssetSplitBatchResult` 增加 `canceled: boolean`。

## Decision (ADR-lite)

**Context**：取消当前走 error 通道，Wails Promise reject 丢弃已完成结果，用户无法核对。
**Decision**：
- 方案 A —— 取消返回 partial 结果 + `Canceled=true` + nil error，终态发 done 事件，对齐 `AssetThumbsResult.Canceled` 先例。
- Items 仅含已处理文件，不引入 `pending`。
- 前端取消弹 warning 计数，明细窗沿用「有失败才弹」并带取消上下文。
- 不做「继续拆分剩余」入口（重开拆分全部即新快照，增量判定 skipped）。
- 取消来源统一按 `context.Canceled` 判定，不区分用户取消与关停。
**Consequences**：
- 批量取消从错误通道降级为带标记的正常完成；`E_ASSETS_CANCELLED` 在批量拆分路径不再使用（单文件入口保留）。
- 现有 Go 断言（取消 error 码 / error 事件）需改写为 `canceled=true` + done 事件。
- 文档契约改写：spec 中「整批取消丢弃返回值（勿承诺保留部分结果）」表述失效。

## Out of Scope

- `PptxSplitService.SplitFolder`（PPT 工具拆分）取消语义
- 单文件拆分 `SplitAssetFile` 取消语义
- 「继续拆分剩余」入口（新快照重跑已足够）
- 素材扫描 `RunAssetScan` 取消明细（仍走 error 通道）

## Technical Notes

- `service/assets_split.go:230-324`（批量主循环、`abortAssetSplit`）
- `service/assets_split.go:481-560`（replace 取消回滚）
- `model/assets.go:430-440`（`AssetSplitBatchResult`）
- `service/assets_thumbs.go:300-415`（`Canceled` 字段与 done 事件先例）
- `frontend/src/components/AssetsPanel.vue:4512-4582`（onConfirmSplit/onCancelSplit）、`4849-4877`（事件监听）、`4669-4685`（汇总文案）
- `service/assets_split_test.go:681-790`（取消与关停用例）
- `frontend/src/components/__tests__/AssetsPanel.spec.js:3457-3475`（取消用例）
- `docs/API参考.md:515-517`、`docs/spec/assets-management.md:99`、`docs/spec/logging-and-errors.md:315`、CLAUDE.md 关键规则行
