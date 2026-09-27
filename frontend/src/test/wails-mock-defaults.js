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
  GetGitInfo: {},
  CreateDirectory: true,
  CreateFile: true,
  RenameFile: true,
  DeleteFile: true,
  PreviewFile: { content: '', error: '' },
  PullRepo: 'Success',
  ScanAndPullRepos: { total: 0 },
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

  // ---- 仓库列表配置导入导出（形状对齐 model.RepoConfigImportPreview / Result）----
  // SaveFileDialog / OpenFileDialog 默认空串 = 用户取消，安全 no-op
  SaveFileDialog: '',
  OpenFileDialog: '',
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
  ReorderChatDirectories: true
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
