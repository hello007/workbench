# Changelog

## [1.15.0] - 2026-11-04

### Added

- 代码预览语法高亮按主题切换：右侧「操作面板 - 文件预览」的代码分支（`.ts`/`.js`/`.json`/`.css` 等）此前固定使用 CodeMirror 内置浅色色板，深色模式预览底 `#1e293b` 下属性名、变量名等关键 token 几乎不可见；新增深色色板（色值与 Markdown 代码块 `.hljs-*` 暗色主题同源），高亮扩展放入 Compartment 按实际生效主题重配（不重建编辑器，滚动位置与折叠状态保留），暗色下另补选中行、选区、行号槽与正文色
- 素材批量拆分取消保留已完成明细：整批取消返回已完成条目与取消标记，单文件取消语义不变
- 工具箱新增 PowerPoint COM 多实例并行与整本导出探针

### Changed

- 依赖治理：undici 升级至 7.30.1 清除 npm audit high，出清 dev-only moderate 漏洞，恢复 package-lock 平台元数据；`@lezer/highlight` 登记为顶层依赖并纳入分档校验
- 后端 CI 增加 windows-latest 测试任务；前端补强素材拆分、损坏留底备份与原子写口径断言
- 远程约定统一为 GitHub SSH，推送前强制 fetch + rebase

### Fixed

- 素材拆分 replace 落点归属：拆分前校验目标目录是否已属他源记录（冲突返回 `E_ASSETS_SPLIT_OUTPUT_CONFLICT`），防覆盖他人产物；备份让位前校验产物指纹，还原备份前校验 schema 版本，防原库与备份双失
- 素材拆分记录生命周期：拆分记录随目录重命名/移动与文件级重链同事务迁移，产物定位兼容改名重链，落点切换强制重拆
- 素材拆分旧产物清理：按产物主干分组与页覆盖判定，兼容历史两位页序，mtime 守卫避免误删用户同页序文件；库外旧落点跳过索引行清理，消除跨卷告警噪音
- 目录项选中收敛为 `@click` 单入口，消除 mousedown 与 click 双绑导致的切换链重复执行
- E2E 稳定性：活动栏与冲突弹窗断言改语义锚定，消除渲染下标漂移导致的 Linux CI 连续失败

## [1.14.0] - 2026-11-03

### Added

- AI 对话模型下拉「默认」选项同步 claude 设置文件（~/.claude/settings.json）：按 CLI 决策链优先级（env.ANTHROPIC_MODEL > model 字段）解析真实默认值并回显，未配置回退通用文案；只读容错（文件缺失/损坏回空串）
- AI 对话启动命令可配：会话工具条「启动命令」下拉——PATH 扫描候选（claude 及常见兼容 CLI 白名单，按解析目标去重）+ 指定 exe 路径（文件选择器）；解析单点 `ResolveChatCommand`（空=默认 claude、绝对路径/命令名均走 shim 解析），配置持久化进 settings.json，变更空闲时优雅重启（--resume 续上下文不丢）；tooltip 明示需兼容 claude headless 协议
- AI 对话思考过程流式展示：模型思考增量（chat-task:output 带 thinking 标记）在气泡内灰色折叠块实时呈现（流式中默认展开可收起）；仅流式零持久化，done 后丢弃，消息 schema 零改动
- 权限模式改为输入盒底栏分段式按钮组，六档对齐 claude CLI 权限模式枚举（手动 default/接受编辑/规划/自动 auto/不询问 dontAsk/跳过确认 bypass），当前档高亮、非默认档警示色强调；发送/停止按钮移入输入盒内下沿右侧（claude.ai 同款容器布局）

### Fixed

- 自签 TLS 证书对中文计算机名机器生成失败（os.Hostname 追加 SAN 违反 IA5String ASCII 约束，报「cannot be encoded as an IA5String」）：非 ASCII 主机名跳过并 Warn，localhost/回环/局域网 IP 覆盖不受影响

## [1.13.0] - 2026-11-03

### Added

- AI 对话代码块「在终端执行」（复用全局底部终端，Warp/VS Code 式对话+终端同屏）：AI 回复代码块操作条一键送入终端输入行（仅 shell 族语言或无语言单行围栏注入按钮，目录树/JSON/源码块不出现）——自动开终端、定位到当前会话工作目录对应 tab（无则新建）、命令原样写入且**不自动回车**（用户确认执行）；跨组件经 uiStore 信箱（seq 单调递增防去重、消费即清空）；时序三态全覆盖（未就绪补偿消费/无 tab 新建/进程退出重启重试）；无 cwd 会话按钮置灰带提示；命令不入会话消息，AI 对话与终端进程完全隔离；后端 TerminalService 零改动

## [1.12.1] - 2026-10-30

### Fixed

- AI 功能与 AI 对话的 claude 启动改经 stdin 传 prompt（`-p` 裸参数）：Windows 命令行上限 32K，prompt 走 argv 时「本地变动 AI 审查」等长 diff 场景 fork/exec 报「The filename or extension is too long」；实测 claude CLI 裸 `-p` + 后续 flags 下 stdin prompt 生效，全量单测回归通过
- 解析 claude `.cmd` shim 真实目标直启（原生 claude.exe / js 入口拼 node）：Go exec 经 cmd.exe `/c` 转发时参数被二次解析，`--json-schema` 的 JSON 引号被破坏致 claude exit 1（报「--json-schema is not valid JSON」）；解析失败回退旧行为，integration 真调回归覆盖

## [1.12.0] - 2026-10-26

### Added

- 内嵌冲突解决第一期（合并/变基/拣选冲突面板）：冲突文件行快捷选边「以当前为准/以传入为准」（`git checkout --ours/--theirs` + `git add` 一键解决，二进制冲突适用）；「内嵌解决」弹窗逐冲突块决策（标记块解析含 diff3 base 段展示不采用，每块 用当前/用传入/两者都保留/块内手动编辑/撤销，实时合并预览 + 全文编辑模式，保存按原编码（UTF-8/GBK）写回并标记已解决，仍含标记时软确认）；无标记文件直接进全文编辑；二进制/超 1MB 文件拦截引导选边或外部工具
- 冲突自动呈现：选中 git 仓库 / 单仓拉取完成 / 批量更新完成三个触发点检测冲突态，存在冲突自动切「合并/变基」tab 并提示，「合并/变基」tab 标题红点角标（冲突动线收口）
- 后端三个只读/变更新绑定：`ChooseConflictSide`（变更类走仓库锁 + 冲突列表成员校验）、`GetConflictFileContent`（路径逃逸防护、二进制/超大判定、编码检测复用预览口径）、`SaveConflictResolution`（按原编码原子写回 + git add）

## [1.11.0] - 2026-10-23

### Added

- 分支对比面板（活动栏一级入口，只读第一期）：A/B 分支选择（本地+远程，默认 A=当前分支）+ 一键交换；双列对齐提交表——两侧独有提交同行配对，三级匹配 hash → patch-id → 归一化 subject（patch-id 徽标即疑似 cherry-pick，merge commit 天然不误配），行按 max(a,b) 日期降序；统计条（A 独有 / 匹配含疑似拣选 / B 独有 / merge-base 之下共同历史），2000 条上限明示截断，无共同祖先显式提示
- 分支差异文件树：两侧 `ls-tree -r` 并集，状态着色与多选过滤（added/deleted/renamed/copied/typechange/modified/unchanged）+ 路径过滤，rename 展示新旧双路径，numstat 单趟统计增删行数（`-` 识别二进制）；未变更文件默认隐藏，「未变更」开关或状态过滤显式勾选时显示
- 双栏/统一 diff 查看器：选中文件解析 `git diff` 原文（解析器自 FileDiffDialog 抽取为共享 `utils/diffParse.js`，行为零变化），二进制占位与超大 diff 截断提示；点击对齐表提交行弹层查看变更文件列表 + 单文件提交 diff（复用 `GetCommitFileDiff`）
- 后端 `BranchCompareService`：对齐数据一次调用全返回（merge-base 容忍不相干历史）；patch-id 批量管道 `git log -p | git patch-id --stable` 走新增 `util.GitCommand.ExecutePipeline`（专用 2min 超时，失败降级为仅 hash/subject 匹配）；配对在后端（浏览器模式与 mock 同形状）；本服务全部 git 命令带 UTF-8 `-c` 前缀 + `%x00/%x01` 分隔解析；只读不抢仓库锁，零新增错误码（复用 E_GIT_TIMEOUT/E_GIT_CANCELLED/E_GIT_LOCKED）

## [1.10.1] - 2026-10-22

### Added

- 素材管理文件夹标签级联检索：文件有效标签 = 自身标签 ∪ 归属文件夹（含全部后代目录）标签，幻灯片条目经归属文件继承；标签筛选两路径（纯标签浏览/关键词）同源扩展，excluded/lost 可见性口径不变；性能实测万级文件 × 数十标注目录 2.6ms
- 工具箱 PPT 拆分面板新增「打开文件夹」入口（单文件结果区与批量明细行）

### Fixed

- 素材文件夹树新建/重命名弹窗输入框自动聚焦（改挂 @opened——@open 时点早于 EP focus-trap 初始聚焦，真机焦点被夺回）
- pptx 预览 ConvertPptxForPreview 返回 pdfPath 归一绝对路径（对齐 model 契约，serve 模式 cwd 变化场景健壮性）

## [1.10.0] - 2026-10-22

### Added

- 素材管理全量上线（侧边栏素材库：纯 Go SQLite + FTS5 中文检索 + PPT 单页索引）：十批迭代覆盖素材拆分集成（单文件/库级批量、增量判定、落点双选）、COM 缩略图（PowerPoint/WPS 双引擎串行队列、画廊懒加载/详情高清/单页跳转、设置五项）、幻灯片独立标注（页级标签/评分/备注 + 继承合并筛选）、批量打标签与多级标签树（拖拽排序/级联/防环）、facet 多维筛选与保存检索条件（星级/时间/大小 + 命名检索）、检索排序器与批量修改评分、元数据导出与文件丢失重链（CSV/Excel/Markdown + lost 标记 + 前缀替换重绑）、排除规则与索引态（目录 glob/后缀归一 + 备份还原 VACUUM INTO/哈希恢复）、视图与选中体系（列表视图三语境列集/全屏预览翻页/文件页双模式选中/缩放）、文件夹树与拖拽整理（真实目录树/徽标/目录管理/文件与目录拖拽移动/folderPath 检索）、文件夹标签级联检索（文件有效标签含归属目录及后代继承，标签筛选两路径同源）
- 素材购物栏与 PPT 页级合并导出：跨库页级收集（条目键契约/幂等去重/排序）、合并导出冲突消解（sldId/rels 递增/数字命名空间独立）、失效条目跳过报告
- 工具箱「PPT 按页拆分」：纯 Go OOXML 拆分核心（页序三跳映射）+ 体积 GC（保留页可达性闭包剪除未用母版/版式/主题与孤儿媒体）+ 文件夹批量拆分（串行可取消 + 派生产物排除）
- 工具箱「pptx 预览」：COM 转 PDF 复用 pdfjs viewer（缓存 LRU/在途防抖/绝对路径契约）
- 工具箱全窗口面板化：7 工具 Dialog 去壳内嵌独立一级面板
- 工具箱「进程管理」：进程枚举与优雅/强杀两级停止、自我保护（PID+父进程链）、检索弹窗
- 工具箱「端口占用一览」：监听端口扁平化 + 行内终止复用进程停止链路
- 工具箱「JSON/YAML 工具」：校验/格式化/压缩/互转
- 工具箱「hosts 文件」只读查看器：解析展示本机 hosts 行结构（IP / 主机名 / 行内注释，兼容 IPv4 / IPv6 及带 zone 的 link-local，容忍 UTF-8 BOM 与 CRLF），注释与空行默认隐藏、「显示注释」开关切回，未识别行灰色标注不静默丢弃，支持按 IP / 主机名过滤（大小写不敏感）与复制单行 / 复制 IP；读取失败就地组级提示（Windows `C:\Windows\System32\drivers\etc\hosts` / Linux `/etc/hosts`）
- 工具箱「环境变量」只读查看器：按来源分页展示（Windows 系统 / 用户 / 进程三级，系统与用户级读注册表、进程级为实际生效值；Linux 仅进程级），支持按名称/值过滤、按名称排序、复制单值；敏感命名（含 TOKEN/SECRET/PASSWORD/PASSWD/KEY/CREDENTIAL，大小写不敏感）默认打码、点击临时显示，复制始终为真实值
- 文件树「复制路径」二级菜单：绝对/相对/@文件引用；内容面板路径下拉
- Git 操作取消链路：context 贯穿收口（Pull/Clone/批量拉取/冲突类 merge/rebase/cherry-pick），取消统一映射中性提示
- Git 锁与超时引导级错误：index.lock 残留探测与锁占用分流（E_GIT_LOCKED）、超时被杀不再误判冲突（E_GIT_TIMEOUT，含提交历史流式）、内容搜索超时收口（E_SEARCH_TIMEOUT，Windows/Unix 双平台）
- 内容搜索超时目录重试：单目录重试入口 + 多目录批量重试（3 并发/版本号守卫/进度横幅）+ 部分超时结果组级感知

### Fixed

- Wails v2.16 结构化错误压扁致前端 code 分流失效（桥接恢复原对象 reject）
- 缩略图 COM 瞬态失败（页级 Export 重试 + Discard 重建落定等待）、Export 相对路径缺陷、上会话自启 COM 残留误判禁杀 + WPS 应用级不可见跳转兜底
- 排除规则后缀归一保序去重、素材清单刷新链统一化（双代次守卫/惰性重拉）

### Changed

- go-git 依赖出清（消依赖路线，Git 元数据走 ResolveHead/.git/config 解析/for-each-ref）
- 前端四 major 升级：vue-router 5 / js-yaml 5（namespace import）/ markdown-it 15 / docx-preview 0.4；Element Plus 2.14.5（依赖分档钳制 exact/tilde/caret/url 四档 + CI 一致性检查）；mermaid 12（dagre/classic 视觉保持双键，katex 传递链漏洞清零）；tilde 档 7 项上游走查
- Go toolchain 1.26.10 + x/net v0.60.0（govulncheck 可达漏洞清零）
- 稳定性基线：goleak 硬门禁（service 包 VerifyTestMain）、soak 三档轮次、E2E 高价值链路批量补齐

## [1.9.0] - 2026-09-28

### Added
- 文件树大目录体验：单层大目录展开截断优化（10 万项 49.2s→647ms，OOM 风险消除）、目录内按名筛选（定位截断层外文件）、筛选覆盖层命中项右键菜单、覆盖层粘贴/新建/拷贝到与重命名/删除统一的保语境刷新
- 冲突面板三向合并：外部合并工具一键拉起（base/local/remote/merged 四视图）
- 统计页行数分布：目录一级上卷 + 文件 Top 15 双维度切换；贡献者卡片行数排名（numstat 按作者聚合 + 独立二态缓存）
- 诊断信息导出 zip 与异常退出检测（`data/crash.flag` + 启动提示），崩溃恢复错误报告出口收口
- 性能与质量基线：GUI 冷启动自动化测量、前端内存峰值/驻留面量化测量脚本、长跑 soak 稳定性测试与内存泄漏自动检测入 CI、真桌面 E2E 通道（真实进程生命周期覆盖 mock 盲区）

### Fixed
- 补提 three-way-merge 漏交文件（GitMerge 外部合并按钮与 store 三向模板字段）
- xlsx 依赖经 SheetJS 官方 CDN 升级 0.20.3，清除两个 high 漏洞
- freePort TOCTOU 定性收尾：幂等用例改直绑 `:0`，结构性端口探测留档接受
- vitest 排除 e2e-desktop 通道目录，修复 npm test 误收集致文件级失败
- 前端内存测量脚本复审修复（早退工作区泄漏 + 文档数字口径归一）

### Changed
- data JSON 落盘收口为统一原子写（temp + fsync + rename），消除崩溃截断丢数据
- 测试加固：clipboard_windows.go unsafe.Pointer 转换语义锚定、model 包覆盖率 76.5%→97.1% 回绿门禁、gofmt 全仓 14 文件格式清零

## [1.8.0] - 2026-09-23

### Added
- Linux 平台支持：平台差异代码分层（编译期/运行时分叉）、自更新 shell 脚本与前端 shell 枚举适配、CI Linux 平台门禁、linux-amd64 发布产物
- AI 对话选择题交互卡片：chat-question 块契约渲染与事件委托提交，禁用 AskUserQuestion 自动应答

### Fixed
- Linux 平台两轮代码审核修复（R1-R10、G1-G12）
- AI 对话面板布局塌陷、弹窗主题选择器失效并补 plaintext 回归守卫
- 补提交 pdfjs-viewer 构建运行时资产
- StartupMatrix 测试用例注入随机端口，隔离本机常驻进程端口依赖

### Changed
- gitignore 忽略 AI 对话运行时目录 data/ai_chat

## [1.7.0] - 2026-09-22

### Added
- 浏览器访问模式：--serve 无头模式与 token 认证、/api/rpc 通用翻译层、WebSocket 事件 hub 与 serve 模式 sink 切换、前端 transport polyfill、桌面同开 HTTP 与网络访问设置
- AI 对话工作台：ChatService 后端与 AiChatPanel 前端（--resume 会话续接、chat-task 事件流、data/ai_chat 存储布局）
- 网络访问绑定地址快捷切换与带 token 链接复制
- 标签列表版本语义倒序排列（--sort=-version:refname）

### Fixed
- 浏览器首访资产 401：cookie 会话贯通
- 浏览器通道代码审核缺陷修复（代际 Wait、Hub Close、并发轮换、同源矩阵、持锁投递）
- 提交历史解析强校验防静默错、失败落日志、时间基锚定

### Removed
- 移除误提交的 bash.exe.stackdump

## [1.6.0] - 2026-09-17

### Added
- 全局仓库状态看板：手动 pin 关注仓库、dirty/ahead/behind 状态计算、活动栏一级入口、dashboard_pinned.json 持久化
- 侧边栏图标悬浮提示并调整顺序

### Changed
- 前端视觉现代化重做：靛蓝主色、Geist 字体、暗色阴影着色，设置弹窗视觉精修对齐 design-tokens 契约
- 设置弹窗与命令面板尺寸响应式优化、垂直布局紧凑化
- gitignore 忽略 data/ai_task*

### Fixed
- 状态看板跳转仓库无反应、嵌套工作目录选错、孤立条目标记
- 仓库信息面板样式并新增仓库统计跳转
- 分支/标签签页文字与按钮同行对齐

## [1.5.0] - 2026-09-15

### Added
- AI 开发辅助：结构化输出链路（--json-schema tool use 强制）、AI 提交信息生成（暂存 diff 生成候选提交信息）、AI 代码审查（diff 生成问题清单分级）、seed skill 按 ID 合并白名单、code-review 规则内嵌
- 推送结果面板：推送输出超 200 字符独立面板完整展示与复制

### Fixed
- AI 任务态泄漏与事件监听器跨组件误删（EventsOn 返回闭包精准注销）

## [1.4.0] - 2026-09-14

### Added
- Git 高级操作：合并（ff/no-ff/squash 三策略）、变基（continue/abort/skip）、Cherry-pick、标签管理、远程仓库管理、分支增删改、单文件暂存
- Submodule 完整闭环管理：状态四色展示、初始化、四种更新模式、添加、删除、detached HEAD 切换跟踪分支
- 仓库统计功能：提交趋势、贡献者排名、活跃度热力图
- 外部 diff 工具集成：预设模板配置（Beyond Compare/WinMerge/VSCode diff/自定义）与一键外部打开
- 仓库列表配置导入导出：工作目录与收藏夹 manifest v1，冲突逐项决策
- 暗色主题：系统跟随三态切换、全场景覆盖、持久化
- 文件树缓存：mtime 差量失效 + TTL 5 分钟 + 手动刷新
- 崩溃恢复：UI 状态快照持久化（data/session.json，debounce 2s）
- E2E 测试体系：Playwright 六关键流程 + Go 集成测试（integration 标签）+ CI 回归自动化
- AI 功能配置导入导出：复用 schema v2 迁移
- 性能基线 benchmark（service/perf_bench_test.go）与 docs/spec/perf-baseline.md

### Changed
- 前端状态管理迁移 Pinia（6 域 setup store）
- app.go 按业务域拆分为 13 个域文件
- AppServices 集中装配依赖注入，App 内嵌字段提升
- slog 结构化日志 + AppError 统一错误处理，lumberjack 落盘 data/logs/app.log
- 抽取 util/testutil 跨包测试辅助，收敛三包重复代码
- 依赖升级：Go 1.26.6、Wails v2.16.0、go-git v5.19.2 等，govulncheck 13→0，安全扫描入 CI
- mermaid 懒加载，首屏 eager 体积减 1.8MB（gzip 550KB）
- 文档完善：快速入门、架构设计、API 参考、贡献指南

### Fixed
- 测试覆盖率分层门禁建立并补齐缺口用例（后端分层 + 前端 70% 硬失败）
- clipboard_windows.go 的 go vet unsafe.Pointer 误报告警
- 文件树缓存代码审核修复 4 项（TTL 驱逐、字段同步断言等）

## [1.3.0] - 2026-09-08

### Added
- AI 功能全量上线：AI 功能菜单（Claude Skills 聚合触发器）、执行链路增强（日志状态追踪、并发上限、输出截断保护）、配置表单化（含 MCP stdio）、运行历史归档与大输出流式落盘、skill 自动发现与一键导入、搜索/分组/置顶、任务历史统计与 CSV/Markdown 导出、使用频次排序

### Changed
- 新增 .gitattributes 统一行尾策略
- 规范沉淀迁出 .trellis/spec 至 docs/spec

## [1.2.9] - 2026-09-02

### Added
- HTML 文件支持渲染预览

## [1.2.8] - 2026-08-17

### Added
- 拷贝到自定义目标支持自定义文件名

## [1.2.7] - 2026-07-19

### Added
- 仓库筛选 master-detail 弹窗
- README 渲染

### Fixed
- 虚拟滚动修复

## [1.2.6] - 2026-07-08

### Changed
- 一键更新跳过无远程仓库

## [1.2.5] - 2026-07-08

### Fixed
- 隐藏 tasklist 进程检测的 cmd 弹窗

## [1.2.4] - 2026-07-08

### Changed
- Obsidian 注册二次确认文案

## [1.2.3] - 2026-07-08

### Added
- Obsidian vault 自动注册
- markdown frontmatter 面板

## [1.2.2] - 2026-07-06

### Added
- markdown mermaid 图与目录导航
- 复制路径
- F2/Del 快捷键

## [1.2.1] - 2026-07-05

### Fixed
- 仓库信息缓存与双刷新修复

## [1.2.0] - 2026-07-04

### Added
- 文件树按类型显示图标（exe/license/gitignore 等）
- 仓库详情直显

## [1.1.0] - 2026-06-24

### Added
- 文件预览增强：Office/PDF/markdown 内嵌预览
- Obsidian 打开入口
- 一键发版脚本

## [1.0.9] - 2026-06-10

### Added
- 检查更新与自动更新功能

## [1.0.8] - 2026-06-10

### Added
- 拷贝到路径互换按钮

### Fixed
- 终端 ResizeObserver 修复
- workbench 二进制更名

## [1.0.7] - 2026-06-09

### Changed
- 版本维护（无新功能）

## [1.0.6] - 2026-06-09

> 注：v1.0.4/v1.0.5 未单独打 tag，其内容并入本节记录。

### Added
- 智能导航中心、内置终端、设置面板弹窗化（原 v1.0.5 内容）
- 自定义快捷键体系：设置面板录制 UI、动态匹配、单个/全部重置、右键菜单快捷键提示
- 内容搜索：ripgrep 与 Go 回退双引擎、命令面板入口、指定目录搜索、排除规则配置
- 文件预览编辑：textarea 就地编辑与保存/取消，SaveFile 原子写入
- 收藏夹增强：命令面板收藏列表内联移除按钮、文件树右键取消收藏

### Fixed
- 收藏夹空值防护与导航定位（跨目录切换定位、nextTick 替代 setTimeout）
- 文件树导航修复（treeReady 信号、按深度并行恢复树状态）
- 快捷键录制无法捕获按键（绑定 keydown、聚焦容器、阻止冒泡）
- useShortcuts 解构 DEFAULTS 导致的 ReferenceError
- 搜索结果面板滚动条位置与内容搜索状态清理

## [1.0.3] - 2026-05-24

### Added
- 工作目录树右键菜单新增"在资源管理器中打开"、"用 VSCode 打开"、"用 Warp 打开"操作

### Fixed
- 修复文件树右键菜单被内容面板覆盖的问题（移除 splitpanes 面板的 z-index 干扰）

## [1.0.0] - 2026-04-30

### Added
- File tree styling with icons and color themes
- Git repository information display component
- Commit history viewer with timeline layout
- Search and filter functionality for commits
- Pagination for commit history (20 per page)
- Copy-to-clipboard for commit SHA and remote URLs
- Collapse all button for file tree
- Refresh buttons for Git info and commit history
- Simple in-memory cache mechanism (5-min expiration)

### Changed
- Improved visual hierarchy with color-coded nodes
- Enhanced hover effects on tree nodes
- Better error handling and user feedback

### Fixed
- Removed expand all button as requested
- Fixed Wails binding import paths for components
