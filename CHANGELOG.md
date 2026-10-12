# Changelog

## [Unreleased]

### Added
- 工具箱新增「环境变量」只读查看器：按来源分页展示（Windows 系统 / 用户 / 进程三级，系统与用户级读注册表、进程级为实际生效值；Linux 仅进程级），支持按名称/值过滤、按名称排序、复制单值；敏感命名（含 TOKEN/SECRET/PASSWORD/PASSWD/KEY/CREDENTIAL，大小写不敏感）默认打码、点击临时显示，复制始终为真实值

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
