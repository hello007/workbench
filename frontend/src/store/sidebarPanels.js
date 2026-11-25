/**
 * 侧边栏面板元数据（id + label 真相源；icon 组件引用留 ActivityBar.vue 按 id 映射）。
 * 数组顺序即默认顺序。ActivityBar 渲染与 Home 全局快捷键（Alt+数字按序号切换）
 * 共用 orderedPanels，保证两处消费同一顺序、设置区调整后即时一致。
 *
 * 本模块刻意零依赖：E2E 侧（frontend/e2e/sidebar-shortcuts.spec.js）与静态守护
 * （scripts/check-e2e-sidebar-panels.mjs）直接导入，不得经由 store/settings.js
 * 间接引入 vue / pinia / wailsjs 生成产物（生成产物在 E2E node 侧无法解析）。
 */
export const SIDEBAR_PANELS = [
  { id: 'directory', label: '工作目录' },
  { id: 'ai', label: 'AI 功能' },
  { id: 'ai-chat', label: 'AI 对话' },
  { id: 'toolbox', label: '工具箱' },
  { id: 'assets', label: '素材管理' },
  { id: 'dashboard', label: '状态看板' },
  { id: 'stats', label: '仓库统计' },
  { id: 'branchcompare', label: '分支对比' },
  { id: 'todo', label: '临时待办' },
  // 第 10 个面板：Alt+1..9 固定映射只覆盖前 9 项（sidebarShortcutIndex 只认 1-9），
  // 本面板可达性由可自定义快捷键（DEFAULTS.openPromptLibrary）补位。
  { id: 'prompts', label: '提示词' },
  // 第 11 个面板：个人数据看板「概览」（只读聚合 M1–M5）。同样不在 Alt+1..9
  // 固定映射内，可达性由可自定义快捷键（DEFAULTS.openOverview）补位。
  { id: 'overview', label: '概览' }
]

/** 默认顺序 id 列表（SIDEBAR_PANELS 派生，顺序与面板定义序严格一致） */
export const SIDEBAR_DEFAULT_ORDER = SIDEBAR_PANELS.map(p => p.id)
