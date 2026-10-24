/**
 * Wails bound method 全局 mock 默认返回值表（单一数据源）。
 *
 * 消费方：
 * 1. vitest 全局兜底：src/test/setup.js 以本表生成 vi.fn 默认实现（组件单测基线）
 * 2. Playwright E2E：e2e/wails-init.js 经 addInitScript 将合并表注入 window.go.main.App
 *
 * 约束：值必须可结构化克隆（纯 JSON），供 Playwright 序列化后传入浏览器页面上下文。
 * 两端共用同一份数据，保证「mock 后端」行为在单测与 E2E 间一致，
 * 契约对齐真实 Go 服务见 docs/spec/cross-layer-contracts.md。
 */
export const WAILS_MOCK_DEFAULT_RETURN_VALUES = {
  GetDirectories: [],
  AddDirectory: { id: 'dir-test', name: 'test', path: '/test', isDefault: false },
  UpdateDirectory: true,
  DeleteDirectory: true,
  SetDefaultDirectory: true,
  GetFileTree: [],
  // 内容搜索（命令面板 : / :: 前缀）：默认空结果 = 未找到匹配内容空态。
  // 元素形状对齐 model.ContentSearchGroup（repoName/repoPath/items/timedOut），
  // 部分超时标志组形态（timedOut: true + 空 items）由组件 spec 按需 override
  ContentSearch: [],
  // 超时目录单目录重试（命令面板 warning 行点击）：默认空结果 = 重试后零命中移除标志组
  ContentSearchDirs: [],
  GetGitInfo: {},
  CreateDirectory: true,
  CreateFile: true,
  RenameFile: true,
  DeleteFile: true,
  PreviewFile: { content: '', error: '' },
  PullRepo: 'Success',
  ScanAndPullRepos: { total: 0, cancelled: 0 },
  // Git 操作取消（默认无可取消在途操作：false / 0 = 幂等 no-op）
  CancelGitOperation: false,
  CancelAllGitOperations: 0,
  ExtractRepoName: 'repo',
  GetAppVersion: 'dev',
  // 崩溃检测（App.vue 挂载即调，E2E 浏览器环境必须有默认值；false=无异常退出提示）
  GetAndClearLastCrashFlag: false,
  ExportDiagnostics: true,
  OpenInWarp: true,
  OpenWithDefaultApp: true,
  OpenInExternalDiff: true,
  CopyTo: '',
  GetFavorites: [],
  AddFavorite: '',
  RemoveFavorite: '',
  UpdateFavoriteAlias: '',
  UpdateFavoriteGroup: '',

  // ---- 全局状态看板（DashboardView「添加关注仓库」弹窗按工作目录扫描仓库候选）----
  // 消费方 DashboardView.vue scanReposForAdd：await GetRepoFilterList(dirId) 后直接
  // 遍历填充候选列表，Proxy 兜底 resolve null 会 TypeError 中断添加链，基础表必须
  // 登记空数组（空态 = 候选列表为空）；条目形状对齐 model.RepoFilterItem，钉选链
  // 用例（e2e/dashboard.spec.js）按需 override 具体清单
  GetRepoFilterList: [],

  // ---- 进程管理（工具箱·进程管理：端口/关键字查进程 + 停止/强杀）----
  // 形状对齐 model.ProcessInfo / ProcessStopResult（见 frontend/wailsjs/go/models.ts）。
  // ListProcesses 默认空列表 = 未匹配到进程的空态；StopProcess 默认 forced 手段
  // （Windows 上目标进程常忽略优雅请求、最终由强杀生效的真实常见形态）；
  // KillProcess 返回 void，null 为安全 no-op。
  ListProcesses: [],
  StopProcess: { stopped: true, method: 'forced' },
  KillProcess: null,

  // ---- 环境变量查看器（工具箱·环境变量：多来源只读枚举 + 脱敏标记）----
  // 形状对齐 model.EnvVarGroup / EnvVarInfo（见 frontend/wailsjs/go/models.ts）。
  // 默认 Windows 三组形态（Linux 实机仅 process 一组）；组内按名称大小写不敏感排序
  // 对齐后端 buildEnvVarInfos；sensitive=true 项（FAKE_TOKEN / ANTHROPIC_API_KEY）
  // 供前端打码与点击临时显值场景测试——value 恒为真实值（打码是前端展示层职责）。
  ListEnvVars: [
    {
      source: 'system',
      name: '系统环境变量',
      vars: [
        { name: 'JAVA_HOME', value: 'C:\\Program Files\\Java\\jdk-17', sensitive: false },
        { name: 'PATH', value: 'C:\\Windows\\system32;C:\\Windows', sensitive: false }
      ],
      error: ''
    },
    {
      source: 'user',
      name: '用户环境变量',
      vars: [
        { name: 'ANTHROPIC_API_KEY', value: 'sk-ant-fake-123456', sensitive: true },
        { name: 'TEMP', value: 'C:\\Users\\demo\\AppData\\Local\\Temp', sensitive: false }
      ],
      error: ''
    },
    {
      source: 'process',
      name: '进程环境变量',
      vars: [
        { name: 'FAKE_TOKEN', value: 'tok-fake-abcdef', sensitive: true },
        { name: 'NUMBER_OF_PROCESSORS', value: '8', sensitive: false }
      ],
      error: ''
    }
  ],

  // ---- hosts 文件查看器（工具箱·hosts 文件：只读解析 + 过滤 + 复制）----
  // 形状对齐 model.HostsView / HostsEntry（见 frontend/wailsjs/go/models.ts）。
  // 默认 Windows 路径形态（Linux 实机为 /etc/hosts）；entries 覆盖 entry/comment/
  // unknown/blank 四种行类别，供过滤与「显示注释」开关场景断言；error 空串 = 读取成功。
  GetHosts: {
    path: 'C:\\Windows\\System32\\drivers\\etc\\hosts',
    entries: [
      { kind: 'entry', ip: '127.0.0.1', hostnames: ['localhost'], comment: '', raw: '127.0.0.1 localhost' },
      { kind: 'entry', ip: '::1', hostnames: ['ip6-local'], comment: 'IPv6 回环', raw: '::1 ip6-local # IPv6 回环' },
      { kind: 'entry', ip: '10.20.30.40', hostnames: ['dev.internal.example.com'], comment: '', raw: '10.20.30.40 dev.internal.example.com' },
      { kind: 'comment', ip: '', hostnames: [], comment: '', raw: '# 屏蔽广告域名示例' },
      { kind: 'unknown', ip: '', hostnames: [], comment: '', raw: 'broken-line-without-ip' },
      { kind: 'blank', ip: '', hostnames: [], comment: '', raw: '' }
    ],
    error: ''
  },

  // ---- 仓库列表配置导入导出（形状对齐 model.RepoConfigImportPreview / Result）----
  // SaveFileDialog / OpenFileDialog / OpenDirectoryDialog 默认空串 = 用户取消，安全 no-op
  SaveFileDialog: '',
  OpenFileDialog: '',
  OpenDirectoryDialog: '',
  SaveFile: true,
  ReadFileBytes: { base64: '', error: '', tooLarge: false },
  ExportRepoConfig: '{"manifestVersion":1,"exportedAt":"2026-09-13T00:00:00Z","directories":[],"favorites":[]}',
  PreviewRepoConfigImport: {
    newDirectories: [],
    conflictDirectories: [],
    newFavorites: [],
    conflictFavorites: [],
    invalid: []
  },
  ApplyRepoConfigImport: { added: 0, overwritten: 0, skipped: 0, failed: 0, failedReasons: [] },

  // ---- 会话快照（崩溃恢复 UI 状态，data/session.json）----
  // GetSessionState 默认 null = 无快照（首次启动 / 崩溃后冷启动），用例按需 override 注入快照验恢复
  GetSessionState: null,
  // SaveSessionState 默认 true = mock no-op（debounce / beforeunload 调用，失败静默不阻塞 UI）
  SaveSessionState: true,

  // ---- 终端多 tab（TerminalPanel + useTerminal：PTY 编排的前端消费面）----
  // 形状对齐 service 层签名：CreateTerminal(dir, shellType, cols, rows) 返回 sessionId
  // 字符串；WriteTerminalInput/ResizeTerminal/CloseTerminal/ChangeTerminalDir 返回
  // void（null 安全 no-op）。真实 pty 在后端，E2E 只验前端编排（tab 生命周期与调用
  // 序列），断言不深入终端缓冲区内容；需要区分会话 id 的用例（如断言 CloseTerminal
  // 收到被关 tab 的 sid）经 E2E 用 __sequence__ 描述符 override（按调用序分配 id）。
  CreateTerminal: 'session-e2e',
  WriteTerminalInput: null,
  ResizeTerminal: null,
  CloseTerminal: null,
  ChangeTerminalDir: null,

  // ---- AI 提交信息生成（PR2）----
  // GetStagedDiffText 默认示例 diff（LocalChanges 用户点「AI 生成」才调，挂载不触发）
  // GetRecentCommitSubjects 默认 3 条 few-shot 示例（形状对齐 []string）
  GetStagedDiffText: '=== src/app.js ===\n+const x = 1\n',
  GetRecentCommitSubjects: ['feat: 新增某功能', 'fix: 修复某缺陷', 'docs: 更新文档'],

  // ---- AI 代码审查（PR3）----
  // GetUncommittedDiffText 默认示例 diff（LocalChanges 用户点「AI 审查」才调，挂载不触发）
  GetUncommittedDiffText: '=== src/app.js ===\n+const x = 1\n',

  // ---- 分支同步摘要（提交历史 BranchSyncBar 挂载即调）----
  // 默认 null = 摘要条隐藏（BranchSyncBar 对空值有 v-if 防护）；E2E extra 表另有完整对象版本
  GetBranchSyncInfo: null,

  // ---- 浏览器访问通道（设置页「网络访问」分区，PR5）----
  // GetWebServeConfig 形状对齐 model.WebServeConfig；默认关闭 + 未运行，
  // 对 UI 主断言无影响（组件按 cfg?.enabled 防御性读取）
  GetWebServeConfig: { enabled: false, bindAddress: '127.0.0.1:36115', running: false, accessUrls: ['http://127.0.0.1:36115'] },
  // GetWebServeToken 默认空串 = 未生成（token 展示区遮蔽为占位符）
  GetWebServeToken: '',
  SetWebServeConfig: true,
  // RegenerateWebToken 返回新令牌（64 位 hex 形态缩写，仅断言被调用与回填展示）
  RegenerateWebToken: 'a1b2c3d4e5f6a7b8a1b2c3d4e5f6a7b8a1b2c3d4e5f6a7b8a1b2c3d4e5f6a7b8',
  // 多 token 管理（设置页「网络访问」令牌管理区，PR1）：条目形状对齐
  // model.WebTokenStatus（record + onlineConns，连接管控任务新增——record 为
  // WebTokenRecord：id/name/hash/createdAt/lastUsedAt/expiresAt 均为 unix 秒，
  // 0 = 永不过期/未使用，revoked 标记已撤销；onlineConns 为该令牌在途 WS 连接数，
  // 0 = 无在线连接）。默认「默认」令牌单条 + 长期保存；明文回显（CreateWebToken）
  // 与撤销（RevokeWebToken）仅在用户操作时调用，null 为安全 no-op
  ListWebTokens: [
    {
      record: {
        id: 'tok-default',
        name: '默认',
        hash: '0'.repeat(64),
        createdAt: 1735689600,
        lastUsedAt: 0,
        expiresAt: 0,
        revoked: false
      },
      onlineConns: 0
    }
  ],
  CreateWebToken: {
    plainText: 'b1b2c3d4e5f6a7b8a1b2c3d4e5f6a7b8b1b2c3d4e5f6a7b8b1b2c3d4e5f6a7b8',
    record: {
      id: 'tok-new',
      name: '新设备',
      hash: '1'.repeat(64),
      createdAt: 1735776000,
      lastUsedAt: 0,
      expiresAt: 0,
      revoked: false
    }
  },
  RevokeWebToken: null,
  // 踢出指定浏览器连接（桌面明细面板处置入口，连接管控任务）：void 返回 null
  // 安全 no-op（命中即断开对端连接；错误经 {__error__, __code__} 描述符注入）
  KickConnection: null,
  // 审计历史（连接管控三期，桌面审计历史弹窗）：形状对齐 service.AuditQueryResult
  // （外层 Total/Records 为 Go 导出字段直序列化大写驼峰；records 内为 json 小写
  // tag：ts/event/tokenId/name/remoteAddr/userAgent/connId/actorSource/
  // actorTokenId），时间倒序、恒数组。默认两条代表性记录供渲染断言；参数
  // （AuditQueryOptions：事件/时间窗过滤 + Offset/Limit 分页）mock 不实现过滤
  // 分页语义，E2E extra 表可覆盖
  QueryAuditLog: {
    Total: 2,
    Records: [
      { ts: 1735689700, event: 'conn_kick', tokenId: 'tok-default', name: '默认', remoteAddr: '192.168.1.5', connId: 1, actorSource: 'desktop' },
      { ts: 1735689600, event: 'auth_success', tokenId: 'tok-default', remoteAddr: '192.168.1.5', userAgent: 'Mozilla/5.0' }
    ]
  },

  // ---- AI 对话工作台（PR1 ChatService 会话链路 + PR2 目录项 CRUD + PR3 对话闭环）----
  // 会话方法：选中目录后调 ListChatSessions，选中会话后调 GetChatSession；
  // 返回空 = 空态降级；其余方法仅在用户操作时调用，null/true 为安全 no-op
  // （形状对齐 model.ChatSession / ChatTaskState / ChatDirectory / ChatTemplate
  // / ChatSettings，见 frontend/wailsjs/go/models.ts）。
  CreateChatSession: null,
  ListChatSessions: [],
  GetChatSession: null,
  DeleteChatSession: true,
  UpdateChatSessionTitle: true,
  RunChat: null,
  CancelChatTask: false,
  ListChatActiveTasks: [],
  RespondChatApproval: null,
  GetChatTaskState: null,
  // PR3 模板与执行配置：GetChatSettings 给默认值（default + 空 model），
  // 组件挂载即调，空对象会破坏权限模式下拉的取值绑定
  AddChatTemplate: null,
  ListChatTemplates: [],
  UpdateChatTemplate: true,
  RemoveChatTemplate: true,
  GetChatSettings: { permissionMode: 'default', modelName: '' },
  SaveChatSettings: true,
  AddChatDirectory: null,
  ListChatDirectories: [],
  UpdateChatDirectory: true,
  RemoveChatDirectory: true,
  ReorderChatDirectories: true,

  // ---- PPT 按页拆分（工具箱·PPT 拆分面板 + 文件树右键入口）----
  // 形状对齐 model.PptxSplitResult / PptxBatchResult（见 frontend/wailsjs/go/models.ts）；
  // 默认空结果 = 拆分仅在用户操作时调用，空态安全 no-op，用例按需 override
  SplitPptxPerPage: { outputDir: '', files: [], pageCount: 0 },
  // 形状对齐 model.PptxBatchResult { dirPath, total, items }（item 含 srcPath/outputDir/pageCount/error）
  SplitPptxFolder: { dirPath: '', total: 0, items: [] },
  // 默认无可取消在途拆分：false = 幂等 no-op（对齐 CancelGitOperation 语义）
  CancelPptxSplit: false,

  // ---- 素材管理（侧边栏·素材管理面板，AssetsPanel 挂载即调 ListAssetLibraries）----
  // 形状对齐 model.AssetLibrary / AssetLibraryStats / AssetScanResult / AssetSearchPage
  // （见 frontend/wailsjs/go/models.ts）。默认空库 = 无库空态（面板引导建库），
  // 检索/扫描/编辑仅在用户操作时调用；void 返回（DeleteAssetLibrary/
  // UpdateAssetFileMeta）用 null 表示安全 no-op。
  CreateAssetLibrary: { id: 'lib-test', name: '素材库', rootPath: '/assets', createdAt: '', updatedAt: '', excludeDirs: [], excludeExts: [] },
  ListAssetLibraries: [],
  // 删库第二参数 allowNoBackup（第十批 SRS §2.7.5 删库前自动备份）：false = 备份
  // 失败返回 E_ASSETS_DELETE_BACKUP_FAILED 确认型拒绝，true = 用户二次确认后强制
  // 删除；void 返回 null 安全 no-op
  DeleteAssetLibrary: null,

  // ---- 素材库备份与还原（第十批 SRS §2.7.5：手动备份 + 清单 + 还原）----
  // 形状对齐 model.AssetBackupEntry；默认空清单 = 从未备份空态（还原入口空态
  // 安全 no-op，用例按需 override），备份/还原仅在用户操作时调用
  BackupAssetLibrary: { libraryId: '', fileName: '', path: '', sizeBytes: 0, createdAt: '' },
  ListAssetLibraryBackups: [],
  RestoreAssetLibrary: null,
  // 库排除规则保存（第十批 SRS §2.7.3，整体替换 + 保存后自动增量重扫）：返回
  // 重扫汇总（形状对齐 model.AssetScanResult）；仅用户操作时调用，用例按需 override
  UpdateAssetLibraryRules: { total: 0, new: 0, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, excluded: 0, lost: 0, revived: 0, removed: 0 },
  GetAssetLibraryStats: { libraryId: '', fileCount: 0, slideCount: 0, failedCount: 0, encryptedCount: 0, lostCount: 0, excludedCount: 0 },
  // 扫描默认零结果汇总（total/new/updated/unchanged/failed/encrypted/denied/
  // excluded/lost/revived/removed——lost/revived 为第九批 SRS §2.6.5 lost 态、
  // excluded 为第十批 SRS §2.7.3 排除态新增计数，旧数据 undefined falsy 向后兼容）
  StartAssetScan: { total: 0, new: 0, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, excluded: 0, lost: 0, revived: 0, removed: 0 },
  RebuildAssetLibrary: { total: 0, new: 0, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, excluded: 0, lost: 0, revived: 0, removed: 0 },
  // 默认无可取消在途扫描：false = 幂等 no-op（对齐 CancelGitOperation 语义）
  CancelAssetScan: false,
  ListAssetFiles: [],
  // 文件夹树默认空树（形状对齐 model.AssetFolderNode 数组，PRD PR1）：默认无库
  // 语境不拉取；需要树节点的用例按需 override（节点形状 {path,name,children,
  // excluded,fileCount}）
  ListAssetFolders: [],
  // 文件夹管理（PRD PR2：新增/重命名/删除）：默认安全形态（新建返回空节点、
  // 删除 0 条 lost、重命名零同步），用例按需 override
  CreateAssetFolder: { path: '', name: '', children: [], excluded: false, derived: false, fileCount: 0 },
  DeleteAssetFolder: 0,
  RenameAssetFolder: { oldPath: '', newPath: '', replaced: 0, excluded: 0, revived: 0 },
  // 文件批量移动（PRD PR3 拖拽移动）：默认空汇总（形状对齐 model.AssetBatchMoveResult，
  // Item 含 fileId/status/message/label），用例按需 override
  MoveAssetFiles: { total: 0, succeeded: 0, failed: 0, items: [] },
  // 目录级移动与文件夹标注（PRD PR4）：默认安全形态（移动零同步返回、标注读取
  // 零值 / 保存 no-op），用例按需 override
  MoveAssetFolder: { oldPath: '', newPath: '', replaced: 0, excluded: 0, revived: 0 },
  GetAssetFolderMeta: { folderPath: '', note: '', tags: [], updatedAt: '' },
  UpdateAssetFolderMeta: null,
  // 幻灯片清单默认空分页页（items/total 对齐 model.AssetSlidePage，2026-10-14 起分页）。
  // 幻灯片条目形状对齐 model.AssetSlideEntry：基础字段（id/fileId/filePath/slideNum/
  // title/text/speakerNotes/tags）+ 独立标注三字段（SRS §2.2.4：ownTags 页自有标签
  // 数组 / note 业务备注 / rating 星级 0-5），用例注入条目 fixture 时须带全三字段
  ListAssetSlides: { items: [], total: 0 },
  UpdateAssetFileMeta: null,
  // 幻灯片页级标注保存（整体替换：note/rating/tagNames）：void 返回 null 安全 no-op；
  // 纯标注层写入，不触发重解析与缩略图失效
  UpdateAssetSlideMeta: null,
  // 检索默认空结果页（items/total/keyword/flattened 四字段对齐 model.AssetSearchPage）。
  // 幻灯片命中条目（refType='slide'）自带 ownTags/note/rating 三字段（文件条目恒零值），
  // opts 含 slideTagOnlyOwn 继承开关（默认 false = 合并归属文件标签命中）+ facet
  // 三维字段（第七批 SRS §2.5：ratings/mtimeFrom/mtimeTo/sizeFrom/sizeTo，档位→
  // 区间换算在前端 AssetFilterPopover 模块单点，mock 经调用参数断言消费不校验值）
  // + 排序档位字段（SRS §2.5.3：sortBy/sortDir，mock 固定返回集合不消费排序键，
  // 调用参数断言消费透传值）
  SearchAssets: { items: [], total: 0, keyword: '', flattened: false },

  // ---- 素材标签树与批量打标签（第六批 SRS §2.4：标签树 + 标签管理 + 批量打标签）----
  // ListAssetTags 默认两级标签样例（根 + 子级；形状对齐 model.AssetTagListItem：
  // id/name/color/parentId/count，count 为直接绑定行数，含未使用标签），标签树
  // 构建 / 名称映射 / 删除影响面用例以此为基础；Create/Update/Delete 仅在用户
  // 操作时调用，默认值安全 no-op（DeleteAssetTag 0 = 无绑定行解除）；Batch×2
  // 为批量打标签弹窗（AssetBatchTagDialog 经宿主 execute 分派）后端链，默认
  // 全零空明细（形状对齐 model.AssetBatchTagResult），用例按需 override
  ListAssetTags: [
    { id: 1, name: '重点客户', color: '#2563eb', parentId: 0, count: 3, sortOrder: 0 },
    { id: 2, name: '城商行', color: '#0891b2', parentId: 1, count: 2, sortOrder: 0 },
    { id: 3, name: '方案', color: '#16a34a', parentId: 0, count: 1, sortOrder: 1 }
  ],
  CreateAssetTag: { id: 99, name: '新标签', color: '#7c3aed', parentId: 0, sortOrder: 0 },
  UpdateAssetTag: null,
  // 拖拽排序/移动父级（D8#1）：void 返回 null 安全 no-op；管理弹窗乐观更新失败
  // 回滚链路用例按需 override reject
  MoveAssetTag: null,
  DeleteAssetTag: 0,
  BatchUpdateAssetFileTags: { total: 0, succeeded: 0, failed: 0, items: [] },
  BatchUpdateAssetSlideTags: { total: 0, succeeded: 0, failed: 0, items: [] },
  // 批量评分弹窗（第八批，SRS §2.6 P0，AssetBatchRatingDialog 经宿主 execute
  // 分派）后端链：默认全零空明细（形状对齐 model.AssetBatchRatingResult，条目
  // 含后端拼装 label 字段），用例按需 override
  BatchSetAssetRating: { total: 0, succeeded: 0, failed: 0, items: [] },

  // ---- 素材保存检索条件（第七批 SRS §2.5：命名/调取/删除/覆盖，全局存储）----
  // ListAssetSavedSearches 默认空列表（条目形状对齐 model.AssetSavedSearch：
  // id/name/libraryId/keyword/tagIds/tagNames/ratings/mtimeFrom/mtimeTo/
  // sizeFrom/sizeTo/flatten/slideTagOnlyOwn/createdAt）；SaveAssetSavedSearch
  // 为 upsert（返回带生成 ID 的条件对象，前端 PR3 已接入消费——保存/覆盖成功
  // toast 后 List 重拉，返回值本身不参与渲染，默认 null 安全 no-op，用例按需
  // override 具体对象）；DeleteAssetSavedSearch void 安全 no-op
  ListAssetSavedSearches: [],
  SaveAssetSavedSearch: null,
  DeleteAssetSavedSearch: null,

  // ---- 素材缩略图（PR3：画廊/详情懒加载 + 能力探测 + 打开跳转 + 失败重试 + 设置）----
  // GetAssetThumb 默认空串 = 未生成回退「无预览」占位（契约见 docs/spec/assets-thumbs-com.md
  // 第 8 节：返回串直接拼 data:image/png;base64,<串>；base64 大载荷不经 mock 回传，
  // 画廊一律 tier=0 懒加载禁整库预取，hd 仅详情单张按需）
  GetAssetThumb: '',
  // 能力探测默认降级态（本机无 PowerPoint）：占位卡 + 降级提示条路径稳定，用例按需 override true
  AssetThumbsAvailable: false,
  // 打开并跳到指定页：void 返回 null 安全 no-op（PPT 窗口由后端 COM 打开）
  JumpToAssetSlide: null,
  // 失败缩略图重新入队：void 返回 null 安全 no-op
  RetryAssetThumb: null,
  // 默认无可取消在途生成：false = 幂等 no-op（对齐 CancelAssetScan 语义）
  CancelAssetThumbs: false,

  // ---- 素材 PPT 拆分（SRS §2.2.3 / §2.2.5 方式 2：单文件与库级批量拆分 +
  // 取消 + 方式 2 打开拆分单页）----
  // 形状对齐 model.AssetSplitResult / AssetSplitBatchResult / AssetSplitRecordView
  // （见 frontend/wailsjs/go/models.ts）。拆分仅在用户操作时调用，零结果/零页为
  // 安全 no-op；GetAssetSplitRecord 默认无记录（exists=false = 方式 2 按钮隐藏），
  // 用例按需 override；OpenAssetSplitPage void 返回 null 安全 no-op
  SplitAssetFile: { outputDir: '', files: [], pageCount: 0, skipped: false, skipReason: '' },
  SplitAssetLibrary: { total: 0, split: 0, skipped: 0, failed: 0, items: [] },
  // 默认无可取消在途拆分：false = 幂等 no-op（对齐 CancelAssetScan 语义）
  CancelAssetSplit: false,
  GetAssetSplitRecord: { exists: false, outputDir: '', pageCount: 0, splitAt: '', upToDate: false },
  OpenAssetSplitPage: null,

  // ---- 素材元数据导出（第九批 SRS §2.6.4：CSV / Markdown+YAML，工具行「导出」入口）----
  // 形状对齐 model.AssetExportResult；默认零行空回执（导出仅在用户操作时调用，
  // 用例按需 override；目标已存在确认型拒绝经 {__error__, __code__} 描述符注入）
  ExportAssetMetadata: { libraryId: '', csvPath: '', mdDir: '', rows: 0, mdFiles: 0 },
  // 默认无可取消在途导出：false = 幂等 no-op（对齐 CancelAssetScan 语义）
  CancelAssetExport: false,

  // ---- 素材文件丢失与重链（第九批 SRS §2.6.5：lost 呈现 + 重链 + 前缀替换 +
  // 清理丢失）----
  // 形状对齐 model.AssetRelocateResult / AssetPrefixReplaceResult；重链默认哈希
  // 匹配直复活形态、前缀替换默认零替换空扫描（均仅在用户操作时调用，用例按需
  // override；哈希不等确认型拒绝经 {__error__, __code__} 描述符注入）；清理丢失
  // 默认 0 = 无丢失记录可清；预览默认 0 = 零命中合法终态
  RelocateAssetFile: { fileId: 0, oldPath: '', newPath: '', hashMatched: true, skippedHashCheck: false, reparsed: false },
  ClearLostAssetFiles: 0,
  PreviewAssetPathPrefixReplace: 0,
  ReplaceAssetPathPrefix: { libraryId: '', oldPrefix: '', newPrefix: '', replaced: 0, revived: 0, reparsed: 0, stillLost: 0, scan: { total: 0, new: 0, updated: 0, unchanged: 0, failed: 0, encrypted: 0, denied: 0, lost: 0, revived: 0, removed: 0 } },

  // ---- 素材购物栏（PR3：跨库页级收集 + 拖拽排序 + 合并导出）----
  // 形状对齐 model.AssetCartItem / AssetCartAddResult / PptxMergeResult（见
  // frontend/wailsjs/go/models.ts）。ListAssetCartItems 默认空购物栏（挂载即拉，
  // 空态 = 抽屉空态 + 徽标隐藏）；AddAssetCartItems 默认全去重空新增（幂等 no-op，
  // 用例按需 override {added, duplicates}）；Remove/Clear/Reorder void 返回 null
  // 安全 no-op；MergeAssetCartToPptx 默认 null（仅用户操作触发，用例按需 override
  // 完整结果）；CancelAssetCartExport 默认无可取消在途导出 false（对齐取消绑定
  // 语义）
  ListAssetCartItems: [],
  AddAssetCartItems: { added: 0, duplicates: 0 },
  RemoveAssetCartItems: null,
  ClearAssetCart: null,
  ReorderAssetCart: null,
  MergeAssetCartToPptx: null,
  CancelAssetCartExport: false,

  // 设置保存（AppSettings.assets 段缩略图设置入口，整体提交语义）：void 返回 null 安全
  // no-op；GetSettings 在 E2E 补充表（{}，settings store 按 themeMode 判定安全回退）
  SaveSettings: null,

  // ---- PPTX 预览转换（文件树 pptx/ppsx 内嵌预览：COM 转 PDF + pdfjs viewer）----
  // 形状对齐 model.PptxPreviewResult { pdfPath, error, errorCode }（见
  // frontend/wailsjs/go/models.ts）。默认降级态（CI 无 PowerPoint）：
  // errorCode = E_PPTX_PREVIEW_UNAVAILABLE → 渲染器显示降级提示 + 用默认程序
  // 打开按钮；用例按需 override { pdfPath: 'D:/cache/xxx.pdf' } 验证 viewer 链路
  ConvertPptxForPreview: { pdfPath: '', error: '未检测到 PowerPoint/WPS，无法内嵌预览 PPT 文件', errorCode: 'E_PPTX_PREVIEW_UNAVAILABLE' }
}

/**
 * E2E 专属补充表：Home.vue 的常驻挂载子组件（v-show 常驻，如 AiFunctionPanel /
 * StatsView / TerminalPanel / SettingsPanel）在真实浏览器中启动即调用，
 * 但 vitest 侧这些组件的 spec 各自显式 vi.mock、不依赖全局兜底，故与上表分开维护。
 *
 * 返回值形状须与消费组件的防御性预期一致，例如：
 * - GetAiConcurrencyStatus 返回值被 AiFunctionPanel 模板直接读 `.running`（无空值防护），
 *   返回 null 会中断整个组件树渲染，必须给完整对象。
 * - GetSettings 被 settings store 按 `THEME_MODES.has(settings.themeMode)` 判定，
 *   空对象时安全回退 system 主题。
 */
export const WAILS_MOCK_E2E_EXTRA_RETURN_VALUES = {
  GetSettings: {},
  GetShellConfigs: [],
  GetAiFunctions: [],
  GetAiConcurrencyStatus: { running: 0, queued: 0, max: 3 },
  GetFunctionUsageCounts: {},
  CheckForUpdate: null,
  // 形状对齐 model.RepoStats（见 frontend/wailsjs/go/models.ts）；contributors 元素含
  // insertions/deletions 行数字段（贡献者行数排名维度切换的数据源），统计页相关断言
  // 以此表为单一数据源。stats 非 null 时统计页图表正常渲染（ECharts 真实浏览器可用）。
  // dirLineStats/topFileLineStats 为行数分布图目录/文件双维度数据源（dirLineStats 形状
  // 对齐 model.PathLineStat）。
  GetRepoStats: {
    trend: [{ date: '2026-09-09', count: 1 }, { date: '2026-09-10', count: 1 }],
    contributors: [
      { author: 'e2e', email: 'e2e@example.com', count: 2, insertions: 120, deletions: 30 },
      { author: 'alice', email: 'alice@example.com', count: 1, insertions: 50, deletions: 80 }
    ],
    dirLineStats: [
      { path: 'service', insertions: 120, deletions: 30 },
      { path: 'web', insertions: 50, deletions: 20 },
      { path: '(根目录)', insertions: 5, deletions: 2 }
    ],
    topFileLineStats: [
      { path: 'service/repo_stats.go', insertions: 80, deletions: 10 },
      { path: 'web/ui/StatsView.vue', insertions: 50, deletions: 20 },
      { path: 'README.md', insertions: 5, deletions: 2 }
    ],
    heatmap: [{ date: '2026-09-09', count: 1 }, { date: '2026-09-10', count: 1 }],
    totalCommits: 3,
    dateRange: '最近 30 天',
    granularity: 'day',
    sampled: false
  },

  // ---- 全局状态看板 ----
  // 形状对齐 model.RepoStatus（见 frontend/wailsjs/go/models.ts）。
  // 单测/E2E 兜底返回空，具体用例在测试内局部 vi.mock 覆盖。
  GetDashboardPinned: [],
  AddDashboardPin: true,
  RemoveDashboardPin: true,
  IsDashboardPinned: false,
  GetDashboardStatuses: [],
  RefreshDashboardStatuses: [],


  // ---- Git 三流程（提交/推送、分支管理、合并/变基）E2E 默认值 ----
  // 形状对齐 model.BranchList / FileChange / Commit / GitRemoteInfo / ConflictState
  // （见 frontend/wailsjs/go/models.ts）。这些方法仅在 ContentPanel 选中 git 仓库节点
  // 后挂载的组件中调用，对未涉及 Git 的 E2E 用例完全惰性。
  // 工作目录本身（GetDirectories）不入本表：目录是 Git 用例的上下文数据，
  // 由 e2e/git-fixtures.js 的 GIT_FLOW_OVERRIDES 按需注入，冒烟等用例保持空列表语义。
  GetBranches: {
    branches: [
      { name: 'main', isRemote: false, isCurrent: true },
      { name: 'feature/login', isRemote: false, isCurrent: false },
      { name: 'origin/main', isRemote: true, isCurrent: false }
    ]
  },
  GetLocalChanges: [
    { path: 'src/app.js', status: 'M', staged: false },
    { path: 'src/utils.js', status: 'A', staged: true }
  ],
  GetCommitHistory: [
    {
      sha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0',
      shortSha: 'a1b2c3d',
      message: 'feat: 支持分支管理面板',
      author: 'e2e',
      email: 'e2e@example.com',
      timestamp: 1757400000,
      dateTime: '2026-09-09 12:00:00',
      files: ['src/components/GitBranches.vue']
    },
    {
      sha: '0f9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b3a2f1e',
      shortSha: '0f9e8d7',
      message: 'chore: 初始化仓库',
      author: 'e2e',
      email: 'e2e@example.com',
      timestamp: 1757313600,
      dateTime: '2026-09-08 12:00:00',
      files: ['README.md']
    }
  ],
  // ---- 分支同步摘要（提交历史摘要条 BranchSyncBar）----
  // 形状对齐 model.BranchSyncInfo（refs: [{sha, kind, name}]，kind 取 head/local/remote）。
  // 默认同步态（ahead/behind=0，local 与 remote 共标 HEAD 同 commit），用例按需 override。
  GetBranchSyncInfo: {
    branch: 'main',
    ahead: 0,
    behind: 0,
    hasUpstream: true,
    detached: false,
    headSha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0',
    refs: [
      { sha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0', kind: 'local', name: 'main' },
      { sha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0', kind: 'remote', name: 'origin/main' }
    ]
  },
  GetGitRemoteURL: {
    remoteUrl: 'https://github.com/demo/demo-repo.git',
    branch: 'main',
    isDetached: false
  },
  GetConflictState: { type: 'none', files: [] },
  HasUpstream: true,
  CommitFiles: true,
  PushRepo: 'To github.com/demo/demo-repo.git\n   main -> main',
  StageFiles: true,
  UnstageFiles: true,
  CreateBranch: true,
  DeleteBranch: true,
  RenameBranch: true,
  CheckoutBranch: true,
  Merge: 'Merge completed',
  Rebase: 'Rebase completed',
  ContinueMerge: 'Merge completed',
  AbortMerge: '',
  ResolveConflict: true,

  // ---- 文件树操作（新建/重命名/删除）E2E 默认值 ----
  // GetFileTree 形状对齐 model.FileTreeNode（id/name/path/type/isGitRepo/hasRemote/
  // hasChildren/children/isLeaf）；el-tree 懒加载：根节点与展开子节点各调一次 GetFileTree(path)。
  // 注入两个工作目录子节点（src 目录 + README.md 文件），目录展开断言与右键菜单操作都以此为基础。
  GetFileTree: [
    {
      id: 'D:/e2e-demo/demo-repo/src',
      name: 'src',
      path: 'D:/e2e-demo/demo-repo/src',
      type: 'dir',
      isGitRepo: false,
      hasRemote: false,
      hasChildren: true,
      isLeaf: false,
      children: []
    },
    {
      id: 'D:/e2e-demo/demo-repo/README.md',
      name: 'README.md',
      path: 'D:/e2e-demo/demo-repo/README.md',
      type: 'file',
      isGitRepo: false,
      hasRemote: false,
      hasChildren: false,
      isLeaf: true
    }
  ],

  // ---- 子模块管理 E2E 默认值 ----
  // 形状对齐 model.GitSubmodule（path/sha/shortSha/describe/branch/url/initialized/
  // shaMismatch/dirty/conflict/detached）。三条数据覆盖状态标签四色映射：
  // 干净（success）/已修改（warning）/未初始化（info）。
  GetSubmodules: [
    {
      path: 'libs/logger',
      sha: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0',
      shortSha: 'a1b2c3d4',
      describe: '',
      branch: 'main',
      url: 'https://github.com/demo/logger.git',
      initialized: true,
      shaMismatch: false,
      dirty: false,
      conflict: false,
      detached: false
    },
    {
      path: 'libs/sdk',
      sha: '0f9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b3a2f1e',
      shortSha: '0f9e8d7c',
      describe: '',
      branch: '',
      url: 'https://github.com/demo/sdk.git',
      initialized: true,
      shaMismatch: false,
      dirty: true,
      conflict: false,
      detached: false
    },
    {
      path: 'libs/vendored',
      sha: '11223344556677889900aabbccddeeff00112233',
      shortSha: '11223344',
      describe: '',
      branch: '',
      url: 'https://github.com/demo/vendored.git',
      initialized: false,
      shaMismatch: false,
      dirty: false,
      conflict: false,
      detached: false
    }
  ],
  UpdateSubmodules: 'Submodule update completed',
  AddSubmodule: 'Submodule added',
  RemoveSubmodule: null,
  CheckoutSubmoduleBranch: 'Switched to branch main',

  // ---- AI 功能触发 E2E 默认值 ----
  // GetAiFunctions 形状对齐 model.AiFunction；单功能无参数（params=null）直跑，
  // completion=none 避免完成动作触达 GetAiTaskOutput/OpenInExplorer 等次级方法。
  GetAiFunctions: [
    {
      id: 'ai-fmt-review',
      name: '代码评审',
      description: '对当前变更做一次代码评审并输出建议',
      icon: 'MagicStick',
      command: '/e2e:review',
      cwd: '',
      addDirs: [],
      env: {},
      mcp: null,
      permissionMode: 'bypassPermissions',
      timeoutMinutes: 10,
      completion: 'none',
      params: null,
      followUps: [],
      tags: [],
      pinned: false
    }
  ],
  // RunAiFunction 返回 taskID 并派发成功事件流（queued→started→output→done），
  // 形状对齐 service/ai_function.go emit 数据与 model.AiTaskRunResult。
  RunAiFunction: {
    __value__: 'task-e2e-1',
    __events__: [
      { event: 'ai-task:queued', payload: { taskId: 'task-e2e-1' } },
      { event: 'ai-task:started', payload: { taskId: 'task-e2e-1' } },
      {
        event: 'ai-task:output',
        payload: { taskId: 'task-e2e-1', text: '评审开始：检查 2 个文件\n发现 1 个问题\n' }
      },
      {
        event: 'ai-task:done',
        payload: {
          taskId: 'task-e2e-1',
          sessionId: 'session-e2e-1',
          exitCode: 0,
          error: '',
          output: '评审开始：检查 2 个文件\n发现 1 个问题\n',
          outputSize: 42,
          outputFile: 'ai_task_history/task-e2e-1.txt',
          canceled: false,
          structuredOutput: {
            candidates: [
              { type: 'feat', scope: 'auth', description: '新增登录校验' },
              { type: 'fix', scope: '', description: '修复空指针异常' },
              { type: 'refactor', scope: '', description: '抽取公共校验逻辑' }
            ],
            // code-review skill OutputSchema 的 issues 数组示例（PR3）：
            // 含多级别（critical/warning/info）多类别（bug/security/performance/style）问题，
            // 供前端 CodeReviewResult 单一数据源。与 candidates 共存：commit-message E2E 读 .candidates，
            // code-review E2E 读 .issues，互不干扰。
            issues: [
              { file: 'src/auth.go', line: 42, severity: 'critical', category: 'bug', confidence: 0.9, description: '空指针解引用：user 为 nil 时访问 user.Name', suggestion: '访问前判空 if user != nil' },
              { file: 'src/auth.go', line: 88, severity: 'warning', category: 'security', confidence: 0.8, description: '密码明文打印到日志', suggestion: '日志脱敏或移除该日志' },
              { file: 'src/util.go', line: 12, severity: 'warning', category: 'performance', confidence: 0.6, description: '循环内重复查询数据库', suggestion: '预加载后内存匹配' },
              { file: 'src/util.go', line: 30, severity: 'info', category: 'style', confidence: 0.5, description: '变量名 s 含义不清', suggestion: '改为 sessionToken' }
            ],
            summary: '审查 2 个文件，发现 4 个问题（1 严重 / 2 警告 / 1 提示）'
          }
        }
      }
    ]
  },
  CancelAiTask: true,
  // GetAiTaskState 形状对齐 model.AiTaskState（前端恢复面板用，E2E 内运行链不轮询）
  GetAiTaskState: {
    taskId: 'task-e2e-1',
    functionId: 'ai-fmt-review',
    running: false,
    queued: false,
    sessionId: 'session-e2e-1',
    prompt: '',
    output: '',
    outputSize: 0,
    outputFile: '',
    error: '',
    startedAt: 1757400000000
  },

  // ---- AI 对话工作台（PR3 对话闭环）E2E 默认值 ----
  // RunChat 返回任务 id 并派发 chat-task:* 事件流（queued→started→output→done），
  // 形状对齐 service/chat_service.go emit 数据与 model.ChatTaskRunResult。
  // queued/started 经 __preEvents__ 在 resolve 前派发（真实时序：先 emit 再返回），
  // output/done 经 __events__ 在 resolve 后派发，真实链路由后端 runtime 推送。
  RunChat: {
    __value__: 'chattask-e2e-1',
    __preEvents__: [
      { event: 'chat-task:queued', payload: { taskId: 'chattask-e2e-1' } },
      { event: 'chat-task:started', payload: { taskId: 'chattask-e2e-1', chatSessionId: 'chatsession-e2e-1' } }
    ],
    __events__: [
      {
        event: 'chat-task:output',
        payload: { taskId: 'chattask-e2e-1', chatSessionId: 'chatsession-e2e-1', text: '这是 AI 的回复：**加粗要点**。\n' }
      },
      {
        event: 'chat-task:done',
        payload: {
          taskId: 'chattask-e2e-1',
          chatSessionId: 'chatsession-e2e-1',
          claudeSessionId: 'claude-e2e-1',
          reply: '这是 AI 的回复：**加粗要点**。\n',
          exitCode: 0,
          error: '',
          canceled: false
        }
      }
    ]
  },
  // ListChatSessions 形状对齐 model.ChatSession（不含消息）；空 directoryId 过滤语义下
  // E2E 注入单条会话供会话下拉断言
  ListChatSessions: [
    {
      id: 'chatsession-e2e-1',
      directoryId: 'chatdir-1',
      title: '新会话',
      cwd: 'D:/e2e-demo/demo-repo',
      claudeSessionId: '',
      createdAt: 1757400000000,
      updatedAt: 1757400000000
    }
  ],
  // GetChatSession 含完整消息数组（气泡渲染断言数据源）
  GetChatSession: null,
  GetChatTaskState: null,
  ListChatTemplates: [],
  GetChatSettings: { permissionMode: 'default', modelName: '' }
}

/** E2E 实际注入 window.go.main.App 的合并表（基础表 + E2E 补充表，后者优先） */
export const WAILS_MOCK_E2E_RETURN_VALUES = {
  ...WAILS_MOCK_DEFAULT_RETURN_VALUES,
  ...WAILS_MOCK_E2E_EXTRA_RETURN_VALUES
}

/**
 * 大目录 fixture 生成器（目录内按名筛选 E2E 用，perf-baseline §15.6 截断配套）。
 *
 * 生成 count 项扁平文件节点（对齐「扁平 node_modules 超 2000 项触发截断」盲区场景），
 * 供 file-tree.spec.js 经 wailsOverrides 覆盖 GetFileTree，验证筛选可定位
 * 2000 名外文件。独立导出不进默认表——默认表被全部用例共享，2000+ 项数组
 * 会拖慢无关用例的树渲染。路径挂 E2E_REPO_PATH（demo-repo 根层）。
 */
export function makeLargeDirNodes(count = 2050, rootPath = 'D:/e2e-demo/demo-repo') {
  return Array.from({ length: count }, (_, i) => ({
    id: `${rootPath}/chunk-${i}.dat`,
    name: `chunk-${i}.dat`,
    path: `${rootPath}/chunk-${i}.dat`,
    type: 'file',
    isGitRepo: false,
    hasRemote: false,
    hasChildren: false,
    isLeaf: true
  }))
}
