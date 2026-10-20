# Research: 多 PPT 选中页按序合并导出（纯 Go OOXML）技术路线

- **Query**: 纯 Go 实现「把多个 PPT 文件中选定的页按顺序合并为一个新的 pptx」的可行技术方案，对比路线 A/B/C/D
- **Scope**: mixed（仓库内现有拆分实现 + 外部库核实）
- **Date**: 2026-10-19

## 1. 摘要

**结论前置：推荐路线 A（基底注入式合并），分两步交付；路线 B/C 否决；路线 D 作为后续 Windows 桌面增强通道可选。**

- 路线 A 保真度高（等价于 PowerPoint「保留源格式」合并语义）、可完全复用现有拆分基建、纯 Go + CI 可测，工作量约 3-4 人日（核心层）。
- 路线 C 的「现成库」前提经核实不成立：纯 Go 生态唯一成熟的 OOXML 库 unioffice 是**商业 EULA**（需 license code），无开源库支持 deck 合并。
- 路线 B（重绑基底母版）保真风险不可控，实现难度反而不低。
- 路线 D（COM `Slides.InsertFromFile`）保真最高但 CI 不可测，与 PRD「不走 COM」假设一致，定位为增强通道。

## 2. Findings

### 2.1 仓库内现有可复用基建（Files Found）

| 文件 | 可复用点 |
|---|---|
| `workbench/service/pptx_split.go` | 拆分核心（约 880 行）。可直接复用：`readZipPackageWithLimits`（zip 全量解压，含炸弹防护）、`buildPptxRelsGraph`（全包 rels 图，合并注入的可达性闭包同款）、`resolveSlidePages`（页序三跳映射 sldIdLst→rId→rels→part）、`resolveRelTarget`/`relsPartOf`（OPC Target 解析）、`dropXMLSpans`（解析定位 + 字节删除，禁 Encoder 重序列化）、`danglingRelsDropPredicate`/`overrideDropPredicate`（悬挂关系与 Override 清理谓词）、`writeSplitOutput` 的原子写序列（CreateTemp→Sync→Rename） |
| `workbench/service/pptx_split_service.go` | 服务编排先例：取消注册表（registerCancel/unregisterCancel defer LIFO）、`mapCancelError`、EventSink 事件出口（pptx-split:* 前缀） |
| `workbench/service/pptx_split_test.go` | 程序化构造 pptx fixture 的测试基建（禁依赖本机 Office） |
| `workbench/service/assets_split.go` | 素材域拆分编排先例（split_records 增量判定、derived 落点镜像源树） |
| `workbench/docs/spec/pptx-split.md` | 拆分契约 spec（GC 四处联动、fixture 断言须含 part→rels 方向 r:id 可解析校验） |
| `workbench/model/`（PptxSplitResult） | 结果模型先例 |

**关键复用判断**：拆分已解决「单文件内保留页子集产出合法 pptx」的全部正确性问题（GC 闭包、四处联动、Content_Types Override 清理、悬挂关系清理）。合并 = 在此之上新增两层：① GC 从单页泛化为页集合；② 跨包子图注入（part 名/rId/ID 冲突消解）。现有 `dropXMLSpans` 的「解析定位 + 字节删除」编辑原语对两层的 XML 编辑均直接适用。

### 2.2 外部核实结论（External References）

| 事实 | 来源（已核实 2026-10-19） | 对选型的影响 |
|---|---|---|
| unioffice（unidoc/unioffice，4934 star，活跃维护，最近推送 2026-09-21）是**商业产品**：LICENSE.md 明确「commercial product and requires a license code to operate」，EULA 见 unidoc.io/eula | GitHub API + LICENSE.md 原文 | **路线 C 否决依据**。与 WorkBench 免费桌面工具/CI 的依赖引入不兼容 |
| gooxml 即 unioffice 前身：`unidoc/gooxml` 仓库重定向到 `unidoc/unioffice` | GitHub API 重定向 | 任务描述中「gooxml」不是独立备选 |
| 纯 Go 生态无开源 deck 合并库：GitHub 按 star 搜索 Go+pptx 前列均为生成/文本替换向小库（ktye/pptx 仅创建、moipa-cn/pptx 仅文本替换、Muprprpr/Go-pptx 仅创建），无一支持跨文件 slide 合并 | GitHub repository search | 路线 C「复用现成合并算法/库」不成立，退化为自研（即路线 A） |
| Apache POI 的 `XSLFSlide.importContent` 存在——Java 生态的权威先例：跨演示文稿 slide 导入采用「连同版式/母版/主题一并搬运 + 引用重写」策略 | POI javadoc（poi.apache.org apidocs 5.0 XSLFSlide） | 佐证路线 A 的注入算法可行性 |
| python-pptx 原生不支持合并（issue #934 "Merge 2 different presentations" 至今 open），社区方案均为手写「复制 slide part + rels 重写」深拷贝 hack | GitHub issues 检索 | 佐证：该问题在主流开源生态均无现成轮子，自研是行业常态 |

### 2.3 OOXML 合并的领域约束（结合仓库拆分经验）

1. **多母版合法**：一个 pptx 允许多个 sldMasterIdLst 条目（每母版各带一个 theme）。**无需合并 theme**——母版/版式/主题整链注入即可，这是保真的根本（PowerPoint「保留源格式」即此语义）。
2. **ID 冲突面**（三处独立命名空间，分别唯一）：
   - `p:sldMasterId@id`：必须 ≥ 2147483648 且全包唯一；
   - `p:sldId@id`：必须 ≥ 256 且全包唯一（上限 2147483647）；
   - `presentation.xml.rels` 的 `rId`：全文件唯一。
3. **part 名冲突**：slide/slideLayout/slideMaster/theme/media 的文件名各源文件同构（slide1.xml 等），注入必须整体重命名并重写引用。
4. **引用重写范围有限**：OPC 关系 Target 相对关系源 part 目录解析（pptx-split.md 踩坑先例）。注入子图内 part 的 rels 随 part 迁移，只需按映射重写 Target；跨包引用只有 presentation.xml.rels 一处（slide/master/notesMaster 关系）需要新增分配 rId。
5. **presentation.xml 是基底文件本体**（不注入）：sldIdLst 按购物栏顺序追加条目、sldMasterIdLst 追加注入母版条目；`p:sldSz` 恒取基底（见风险 R1）。
6. **Content_Types**：注入 part 的 Override 随映射重写 PartName 追加；媒体 Default（png/jpeg 等）基底缺失的扩展名须补 Default。
7. **presentation 级清理**：基底经 GC 已剥 custShowLst/extLst（拆分语义现成）；注入侧源 presentation.xml 本体不注入，节信息/自定义放映天然不带。
8. **嵌入字体**（embeddedFontLst + fntdata part）挂在 presentation.xml 级：MVP 建议剥离（不嵌入时回退系统字体，风险登记）。

## 3. 路线对比

### 路线 A：基底注入式合并（推荐）

**实现步骤概要**：

1. **GC 集合化泛化**（改动中等）：`deleteSetForPage` 现按单 kept 页计算删除集；泛化为「保留页集合」——roots 含全部保留页及其链路（页→版式→母版→主题可达性闭包自动多母版化），sldIdLst 保留多条。现有单页版把「其余母版」全剪，集合版改为保留全部被引用母版及其全部被引用版式（闭包屏障逻辑不变）。此步可独立交付「多页子集导出」小能力。
2. **子图提取**：对每个非基底源文件跑集合化 GC，得到保留闭包（part 集合）+ 该文件的页序（三跳映射现成）。
3. **注入层**（全新，核心工作量）：
   - 构建 part 名映射：注入闭包内全部 part 重命名（slides/slideLayouts/slideMasters/theme/media 统一全局计数器，如 slide101.xml 起），`pptx/_rels/...` 随 part 同步改名；
   - 逐 part 拷贝字节 + 重写其 rels 的 Target（`resolveRelTarget` 归一后查映射；未命中且不在基底 = 指向未购物页的跨页跳转，走 `danglingRelsDropPredicate` 同款删除）；
   - `presentation.xml.rels` 追加 slide/master 关系，rId 取基底现有 max 递增；
   - `presentation.xml`：sldIdLst 按购物栏顺序追加（sldId 数字 id 取基底 max+1 递增，与 sldMasterId id ≥2147483648 命名空间互不相干）、sldMasterIdLst 追加注入母版条目（id 取现有 max+1）；
   - `[Content_Types].xml`：Override 按 PartName 映射追加 + 缺失扩展名补 Default（`overrideDropPredicate` 的逆向追加，编辑原语同为字节级）。
4. **编排**：基底 = 购物栏第一项所在源文件（GC 后）；后续源逐个注入。服务编排层复用取消注册表 + EventSink 模式（pptx-split_service.go 先例），data 落盘走原子写。

**保真度**：高。保留每页完整渲染链路（字体/主题色/占位符/背景/动画页内部分原样字节），等价 PowerPoint「保留源格式」。

**工作量粗估**：GC 集合化 0.5-1 人日；注入层 2-3 人日；fixture 全链路测试 1 人日（基建现成）。合计约 3-5 人日（不含购物栏 UI/编排前端）。

**风险**：
- R1 `p:sldSz` 比例不一致（16:9 与 4:3 混合）→ 外来页按基底尺寸显示变形（纯 XML 合并不自动缩放；PowerPoint 粘贴会缩放）。MVP 方案：检测不一致时前端 warning 提示，不做缩放。
- R2 跨页超链接指向未购物页 → 删除关系（可接受，拆分同语义）。
- R3 备注页 → MVP 可选剔除（复用拆分删备注语义）；完整搬运需处理 notesMaster 注入（presentation.xml.rels 挂关系），列为增强。
- R4 嵌入字体剥离 → 字体缺失机器上回退，风险登记。
- R5 chart/SmartArt/嵌入 OLE → rels 闭包自动覆盖（`buildPptxRelsGraph` 通用，无需特判）。

### 路线 B：所有外来页重绘到基底母版/版式（否决）

仅搬 slide part 内容、rels 重指基底 layout。**否决理由**：
- 保真风险不可控：占位符继承（slide XML 按 type/idx 匹配 layout placeholder，基底版式缺对应占位符即丢内容框）、主题色 `schemeClr` 映射全变、背景/字体整页漂移；
- 实现难度不低：占位符匹配、母版缺类型版式兜底、inheritance 链模拟——工作量接近路线 A 而保真远差；
- PowerPoint 自家「使用目标主题」也是经完整排版引擎完成，纯 XML 模拟无先例可循。

### 路线 C：逐文件 GC 后用现成库合并（否决，前提不成立）

已核实（见 2.2）：unioffice 商业 EULA + license code 机制；其余 Go pptx 库均无合并能力；python-pptx 亦不支持。**C 退化为 A**。若未来购入 unioffice 商业授权可作为参考实现对照，但不构成本项目依赖候选。

### 路线 D：COM 通道（后续增强，可选）

`Presentations.Open(基底)` → `Slides.InsertFromFile(src, index, start, end)` 逐购物项插入。保真最高（PowerPoint 排版引擎，自动处理尺寸缩放）。
- **与 assets-thumbs COM 队列架构兼容**：STA 专用 goroutine（LockOSThread + CoInitializeEx）、进程集合 {powerpnt.exe, wpp.exe} 超时强杀、owned PIDs 持久化识别、WPS 接管 ProgID 等既有机制直接复用；跳转链路 jumpInSession 的「应用级 Visible」经验同样适用。
- **不兼容面**：CI 无 Office 不可测（核心功能不可 COM-only 的项目原则）；Linux 不可用；WPS 兼容层对 `InsertFromFile` 的支持情况**未核实，须真机验证**（不能假设定全兼容）。
- **定位**：PRD 假设已定「纯 Go 为主」；D 可作为 Windows 桌面端可选增强通道（检测 Office/WPS 可用时提供「高保真导出」开关），实现代价 1-2 人日 + 真机验证。

## 4. 推荐结论

1. **主路线 A**，两步交付：先做 GC 集合化泛化（独立可测），再做注入层。
2. **测试策略**：程序化构造多源 pptx fixture（复用 pptx_split_test.go 基建），断言面必须含 part→rels 方向 r:id 可解析校验（pptx-split.md 教训：fixture 母版空元素曾结构性掩蔽悬空引用）；PowerPoint/WPS 真机打开无修复弹窗为验收线。
3. **MVP 边界建议**：剔除备注页、剥离嵌入字体、sldSz 不一致 warning、跨页跳转删除；notesMaster 搬运与 COM 双通道列为后续增强。
4. **架构对齐**：服务编排复用取消注册表/EventSink/原子写三先例；核心禁 COM 依赖（与拆分同策略）。

## Caveats / Not Found

- WPS 兼容层是否支持 `Slides.InsertFromFile` 未核实（无本机验证条件），若路线 D 立项须真机先行。
- unioffice 对「跨演示文稿 slide 复制」的具体 API 支持程度未深查（商业许可已构成否决，无深入必要）。
- 工作量为按拆分档（约 880 行核心 + fixture 测试）比例的粗估，非承诺值。
