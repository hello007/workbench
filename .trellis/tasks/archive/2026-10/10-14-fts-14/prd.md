# 素材管理审核修复——版本守卫/分页/FTS 边界等 14 项

## Goal

修复 commit 013032e（素材管理 MVP）双路代码审核发现的 14 项问题（后端 4🟡2🔵+2🔵 已并入、前端 2🔴4🟡2🔵）。审核报告即需求来源，无开放问题。

## Requirements（审核发现逐项）

### 后端（service/ + app.go）

1. **FTS 纯标点词报 syntax error**（assets_fts.go `buildFtsQuery`）：关键词经切分后不含字母/数字/CJK rune 的词（如 `!!!`）跳过；全跳过时返回空串走空结果路径。先写复现测试确认 unicode61 对零 token phrase 的实际行为（审核标 unverified），按实测修。
2. **zip 解压无上限 OOM 风险**（assets_parse.go → 共用 readZipPackage）：解析侧加字节上限（单 entry 与全包总量，PPT 真实量级数十 MB，上限给足裕量），超限按「失败(文件损坏)」降级。**硬约束：pptx_split 拆分链路行为逐字不变**（若改共用函数须 limit 可选参数默认不限，或 assets 侧独立 wrapper）。
3. **ListAssetSlides 无分页**（assets_library.go）：加分页参数（page/limit 或 offset/limit，对齐 SearchAssets 分页风格）；前端 AssetsPanel 调用点同步改；`wails generate module` 重生成绑定三处。
4. **CreateLibrary 孤儿 db**：saveRegistryLocked 失败回删 `<id>.db`（含 -wal/-shm）再返回错误。
5. **shutdown 不等在途扫描**（app.go CloseAll）：关闭前等待/放弃在途扫描（对 assetsSvc 加收口方法，等活跃扫描结束或通知取消后等退出），消 `sql: database is closed` 噪音。
6. **`.-01.pptx` 误判派生**（assets_scan.go isPptxSplitDerivative）：`parentBase == "."` 提前 return false；补回归用例。
7. **多词关键词摘要恒空**（assets_fts.go snippetFromTexts）：逐词定位取首个命中词窗口（多词 FTS 语义为逐词 AND，原文无连续字面匹配）。

### 前端（AssetsPanel.vue + error.js）

8. **🔴 版本守卫缺失跨库串入**（onLibrarySelectionSettled/applySearch/runSearch）：模块内自增 seq，库切换与检索各自守卫（参照 CommandPalette `searchVersion` 既有模式）；await 写回前比对，不一致丢弃。切库在途时 loadStats/loadView 写回全部受守卫。
9. **🔴 幻灯片视图扫描后切文件视图空**：切到文件视图时 fileEntries 为空即触发 ListAssetFiles 加载（含库切换落定在幻灯片视图同根因）。
10. **tagOptions 幻灯片视图恒空**：消费前兜底 ensureFileEntries（文件底册未加载则拉取）。
11. **文件视图+非平铺检索命中全丢但计数矛盾**：searchTotal 按当前视图过滤后口径显示；该组合命中全为 slide 时显示引导开启平铺（或等效处理，实施时选最小改法并在注释说明）。
12. **扫描完成双刷新**：Promise 成功分支与 onScanDone 事件二选一刷新（事件回调仅复位进度态），消大库全量拉取翻倍。
13. **toggleAgg 在途守卫 + aggLoading 并发互踩**：aggLoading 改 Set；展开写回前比对发起时 activeKeyword/selectedTagIds。
14. **error.js E_ASSETS_LIB_NOT_FOUND 行为承诺未落地**：loadLibraries/loadStats 捕获该 code 时重拉库列表，或删注释承诺（实施选一，倾向前者——删库后自愈）。

## Acceptance Criteria

- [ ] 每项修复带对应测试（复现测试先行：FTS 纯标点、误判回归、分页边界、孤儿 db 回删、前端守卫用例——慢响应覆写断言）
- [ ] `go build ./...`、`go vet ./...`、`go test ./service/ ./model/ ./ -count=1` 全绿
- [ ] `GOOS=linux go build ./...` 通过
- [ ] `cd frontend && npm test`、`npm run build` 全绿
- [ ] 覆盖率门禁不回退（service ≥76%）
- [ ] pptx_split 拆分链路既有测试零改动通过（约束 2 验证）

## Out of Scope

- 二批功能（COM 缩略图等）
- 后端独立标签列表接口（本次用兜底方案）

## Technical Notes

- 审核报告：本任务来源，双子代理（cavecrew-reviewer）对 013032e 的发现，每条含修法建议
- 契约基准：docs/spec/assets-management.md、cross-layer-contracts.md（EventsOn 闭包/绑定三处）、frontend-visual-conventions.md、logging-and-errors.md
- CommandPalette searchVersion 既有守卫模式可参照（frontend/src/composables/useCommandPalette.js）
