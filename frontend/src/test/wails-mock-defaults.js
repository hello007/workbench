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
  ResolveConflict: true
}

/** E2E 实际注入 window.go.main.App 的合并表（基础表 + E2E 补充表，后者优先） */
export const WAILS_MOCK_E2E_RETURN_VALUES = {
  ...WAILS_MOCK_DEFAULT_RETURN_VALUES,
  ...WAILS_MOCK_E2E_EXTRA_RETURN_VALUES
}
