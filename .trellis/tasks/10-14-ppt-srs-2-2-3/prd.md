# 素材管理第三批 PPT 拆分集成（SRS §2.2.3 + §2.2.5 方式 2）

## Goal

素材管理面板内提供 PPT 按页拆分能力（单文件 + 库级批量），复用工具箱 PptxSplitService 核心链路（禁重写），补齐增量拆分、输出落点与扫描排除一致性、跳转方式 2（打开拆分单页），对齐 SRS §2.2.3 与 §2.2.5。

## Requirements

1. **入口**：单文件（画廊卡/详情侧边栏按钮）+ 库级批量「拆分全部 PPT」（全部 ok 状态文件，启动时快照圈定）。
2. **落点交互**：拆分确认弹窗带落点选择——默认「素材库缓存目录（derived 镜像源树）」；可选「源目录」（确认提示「产物将与源同目录，已自动排除索引」）。
3. **输出布局**：
   * 缓存目录：`data/assets/<library-id>/derived/<源相对目录>/<源名>/<源名>-NNN.pptx`（镜像源树，根目录文件恰为 SRS 字面 `derived/{源文件名}/`）。
   * 源目录：`<源所在目录>/<源名>/<源名>-NNN.pptx`（工具箱现有布局，isPptxSplitDerivative 已覆盖排除，零扩展）。
   * 页码命名 %03d 固定 3 位（pptx_split.go:134 一行改 + 工具箱测试同步）。
4. **增量拆分**：split_records 表（source_rel_path 主键、source_hash、output_dir、page_count、split_at）+ 输出文件存在双验。手动触发时判定：未拆/哈希变 → 拆；已拆哈希同 → 提示「已最新」跳过；哈希变重拆前清空产物目录（防页数减少残留 + 旧 %02d 产物收敛）。
5. **批量异常口径**：同名冲突/只读/无权限/磁盘不足/加密（OLE 容器 zip 打开失败）逐文件记日志跳过，批量不中断；结果汇总 `{total, split, skipped, failed, 明细}`。
6. **事件**：新增 `assets-split:progress/done/error` 前缀，素材面板独听（与工具箱 pptx-split:* 隔离）；progress 载荷 `{libraryId, done, total, currentFile, pageDone, pageTotal}`（批量文件级 + 单文件页级透传）。
7. **取消**：CancelAssetSplit 按库取消注册表（对齐扫描链路 registerCancel/unregisterCancel defer LIFO），取消错误复用素材域既有的 E_ASSETS_CANCELLED（实现期决议：素材域 warning 分流同款，不取 E_GIT_CANCELLED——git 域错误码不跨域借用，前端 error.js WARNING_CODES 已覆盖该码零改动）。
8. **跳转方式 2**：幻灯片详情已拆分时显示「打开拆分单页」按钮 → `openWithSystemHandler`（复用 fileoperation 跨平台实现）打开 `<产物>/<源名>-{slide_num:03d}.pptx`；与方式 1 JumpToAssetSlide 并存，无 PowerPoint 可用。
9. **文档**：README/功能说明/API参考/pptx-split.md 或 assets-management.md 增补。
10. **拆分体积 GC（B 档，共享核心改动，用户已授权同步修改工具箱拆分代码）**：
    * 背景：实测 10MB 源拆出 9MB/页——最小编辑集只删页 XML，媒体/母版全量携带；真文件验尸 13.44MB 解压体中媒体 9.14MB（其中 4.06MB 零页直引），5 母版/11 版式/7 主题全保留。
    * 改动：保留页真实链路（页→版式→母版→主题）+ presentation 脊柱（presProps/viewProps/tableStyles/notesMaster 等非页非母版引用）可达性闭包 GC——不可达 part 不写输出；`sldMasterIdLst` 剪枝至保留页母版（复用 dropXMLSpans 字节删除）+ presentation.xml.rels 删未用母版关系 + [Content_Types].xml 同步清被删 part Override。
    * 边界：媒体为 Default 扩展名声明无 Override；外链关系（TargetMode="External"）不入闭包；保真红线 = 保留链 part 逐字节不变。实测投影：该文件单页 13.44MB → 平均 3.12MB（A 档孤儿 GC 为 4.57MB；下限受源母版背景媒体约束，纯 Go 保真无法再降）。
    * 借鉴说明：用户 Python 脚本（COM Copy/Paste 到空白文稿）几百 K 是靠 PowerPoint 空白主题重贴、丢弃源母版背景实现的；COM 路线违反拆分核心纯 Go 硬约束（CI 不可测），仅借鉴「只保留真实链路」洞察。

## Acceptance Criteria

* [ ] 拆分方法论不变（解析定位+字节删除/页序三跳映射/OPC Target 基准原样）；核心增强仅两处：命名 %02d→%03d + B 档 GC（编辑方法同类，删除集扩展）
* [ ] GC 断言：fixture 带孤儿媒体 + 多母版——不可达媒体/母版/版式/主题不出现于输出包；保留链 part（含母版/主题）逐字节一致；[Content_Types].xml 无悬空 Override
* [ ] 工具箱拆分回归：现有 pptx_split 测试全绿 + 体积收缩断言
* [ ] 拆分产物不被扫描重复索引（缓存目录落点库根之外天然隔离；源目录落点 isPptxSplitDerivative 命中）——真机冒烟验证重扫描
* [ ] 源文件变更后再拆分清旧页不残留；已拆且未变时秒回「已最新」不重拆
* [ ] 批量中单文件失败隔离（损坏/加密/只读逐个跳过），批量完成有汇总
* [ ] 取消：在途拆分可取消（E_GIT_CANCELLED warning 中性提示），注册表无残留
* [ ] 跳转方式 2：已拆分文件的幻灯片详情可打开对应单页 pptx
* [ ] 同库不同目录同名 pptx 拆分产物互不覆盖（镜像源树）
* [ ] GOOS=linux 编译 vet 绿；service ≥76%、前端 ≥70%
* [ ] wails 绑定三处同步（App.js/App.d.ts/models.ts）；前端 EventsOn 闭包注销禁 EventsOff；mock 改 wails-mock-defaults.js；E2E 从 fixtures.js import；禁 waitForTimeout
* [ ] SQLite 单连接池纪律（rows Close 再查/禁逐条写）；schema 迁移 user_version v3→v4

## Definition of Done

* 单测/集成测试齐（fixture 程序化构造 pptx，禁 COM/Office/sleep/mtime 时序）
* go test / npm test 全绿，覆盖率门禁达标
* 文档更新（README/功能说明/API参考/spec）
* 真机冒烟：拆分端到端 + 重扫描不重复索引 + 跳转方式 2

## Technical Approach

* **编排层**：`service/assets_split.go` 新文件，AssetLibraryService 方法组；核心调 `SplitPptxPerPageWithProgress`（页级回调透传 pageDone/pageTotal）。
* **串行保护**：同库同时仅一个拆分任务（tryLock 模式对齐扫描 tryLockScan）；拆分不写 files/slides 索引表，与扫描并发安全（单连接池串行化）。
* **schema v4**：assets_store.go 迁移链追加 split_records 建表；删库随 db 文件级联清理。
* **App 绑定**（app_assets.go 新增 5 个，wails generate 三处同步）：
  * `SplitAssetFile(libraryID, relPath string, outputToSourceDir bool) (model.AssetSplitResult, error)`
  * `SplitAssetLibrary(libraryID string, outputToSourceDir bool) (model.AssetSplitBatchResult, error)`
  * `CancelAssetSplit(libraryID string) bool`
  * `GetAssetSplitRecord(libraryID, relPath string) (model.AssetSplitRecordView, error)`
  * `OpenAssetSplitPage(libraryID, relPath string, slideNum int) error`
* **模型**：`model.AssetSplitResult{OutputDir, Files, PageCount}` 复用/扩展 PptxSplitResult 形态；`AssetSplitBatchResult{Total, Split, Skipped, Failed, Items []AssetSplitFileOutcome}`；均普通字段无具名 string 类型（不触发 models.ts 别名规则）。
* **打开文件**：FileOperationService 薄封装导出（openWithSystemHandler 已跨平台现成）。

## Decision (ADR-lite)

* **Q1 批量入口粒度 → 库级全部**：库面板操作「拆分全部 PPT」，后端圈定全部 ok 状态文件逐个处理（对齐 SRS「批量目录拆分」）。画廊多选批量留后续批。
* **Q2 增量清单 → SQLite 表 split_records**：已拆判定 = 表命中 + 输出文件存在双验（对齐缩略图补缺「PNG 存在唯一权威」先例，手删产物误判已拆被兜住）。跳转方式 2 查询走 SQL；删库随 db 文件级联清理。schema v3 → v4。
* **Q3 事件前缀 → 新增 assets-split:\***：素材面板独听，与工具箱 PptSplitDialog（pptx-split:*）完全隔离——共听面延续 pptx-split.md §8 收敛取舍；applySink 已含 assets 服务零改动。进度形状批量文件级（对齐 assets-scan:progress），单文件页级经 pageDone/pageTotal 透传。
* **Q4 derived 落点 → 镜像源树**：`derived/<源相对目录>/<源名>/<源名>-NNN.pptx`。根目录文件恰为 SRS 字面布局；子目录文件多一层镜像目录，同库同名 pptx 撞目录结构性消除；叶子保持「同名文件夹+同名-数字」布局与排除判定形态一致。
* **Q5 页码命名 → %03d**：SRS 明文固定 3 位，≥100 页字典序不错乱（%02d 下 "x-100" < "x-99" 错序）；isPptxSplitDerivative 纯数字判定两位三位通吃；两入口（工具箱/素材面板）产物命名统一。
* **Q6 拆分体积 GC → B 档（孤儿 GC + 母版链修剪）**：用户实测 10MB 源拆出 9MB/页不可接受，Python COM 参照脚本（几百 K）路线违反纯 Go 硬约束不可走，借鉴其「只保留真实链路」洞察纯 Go 实现。实测投影 13.44MB → 平均 3.12MB/页；下限受源母版背景媒体约束（image11.png 2.71MB 挂主母版，保真必须携带；Python 几百 K = 弃保真换空白主题，不采）。用户已授权同步修改工具箱拆分代码（共享核心一处改动双入口受益）。

## Out of Scope

* 自动拆分（SRS「默认关闭」，仅手动入口；不建设置开关）
* 拆分核心算法改动（最小编辑集/页序三跳映射原样）
* 方式 1 JumpToAssetSlide 改动
* 画廊多选批量拆分（后续批）

## Technical Notes

* SRS 原文：docs/原始需求/202610-素材管理系统需求说明书-V1.0.md §2.2.3（105 行）/ §2.2.5（128 行）
* 契约：docs/spec/pptx-split.md、docs/spec/assets-management.md、docs/spec/assets-thumbs-com.md §8/§9
* 关键代码：service/pptx_split.go:134（%02d 命名）、service/assets_scan.go:32（排除判定）、service/assets_thumbs.go:490（缓存目录单点）、service/fileoperation_other.go:25（openWithSystemHandler 跨平台）
* 加密 pptx = OLE(CFB) 容器非 zip，zip 打开失败即加密/损坏分类（扫描 assetParseEncrypted 先例）

## Implementation Plan (small PRs)

* **PR1 拆分核心增强（共享）**：%03d 命名 + B 档 GC（可达性闭包 + sldMasterIdLst 剪枝 + Override 清理）+ fixture 媒体/多母版断言 + 工具箱回归 + 契约 pptx-split.md §3 增补
* **PR2 素材拆分后端**：schema v4 split_records + assets_split.go 编排（单文件/库级批量/增量判定/清旧页/取消注册/事件/异常隔离）+ App 绑定 5 个 + wails generate 三处同步 + 单测
* **PR3 前端与收口**：素材面板拆分入口（库级操作项 + 单文件按钮）+ 落点选择确认弹窗 + 进度/取消（assets-split:* 监听闭包注销）+ 跳转方式 2 按钮 + mock 单一数据源 + vitest/E2E + README/功能说明/API参考 + spec 沉淀 + 独立审核修复 + 真机冒烟（拆分端到端 + 重扫描不重复索引 + 跳转方式 2 + 体积抽查）
