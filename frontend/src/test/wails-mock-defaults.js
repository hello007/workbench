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
  OpenInWarp: true,
  OpenWithDefaultApp: true,
  CopyTo: '',
  GetFavorites: [],
  AddFavorite: '',
  RemoveFavorite: '',
  UpdateFavoriteAlias: '',
  UpdateFavoriteGroup: ''
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
  GetRepoStats: null,

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
          canceled: false
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
  }
}

/** E2E 实际注入 window.go.main.App 的合并表（基础表 + E2E 补充表，后者优先） */
export const WAILS_MOCK_E2E_RETURN_VALUES = {
  ...WAILS_MOCK_DEFAULT_RETURN_VALUES,
  ...WAILS_MOCK_E2E_EXTRA_RETURN_VALUES
}
