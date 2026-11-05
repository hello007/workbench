/**
 * claude CLI 内置斜杠命令清单（静态数据，随代码维护）。
 *
 * AI 对话输入框 / 补全弹层在已装 skill 之外追加展示这些内置命令（source=builtin、
 * 来源标签「内置」），补全行为与 skill 一致（整段替换 + 尾随空格）。
 *
 * 只收录对话场景有意义的子集——纯交互面板类命令（/config、/vim、/terminal-setup、
 * /statusline 等）在嵌入面板里无操作意义，不列；CLI 后续版本新增命令时在此追加。
 * 形状对齐 SkillDescriptor 消费面（command/name/description/source/cwd）。
 */
export const CLAUDE_BUILTIN_COMMANDS = [
  { command: '/help', name: 'help', description: '查看帮助与可用命令' },
  { command: '/clear', name: 'clear', description: '清空当前会话上下文，开始新话题' },
  { command: '/compact', name: 'compact', description: '压缩会话历史，释放上下文空间（可附压缩说明）' },
  { command: '/context', name: 'context', description: '以色块视图查看当前上下文占用' },
  { command: '/cost', name: 'cost', description: '查看当前会话 token 用量与花费' },
  { command: '/model', name: 'model', description: '查看或切换模型' },
  { command: '/status', name: 'status', description: '查看系统/账号/配置状态' },
  { command: '/review', name: 'review', description: '请求审查一次代码变更' },
  { command: '/init', name: 'init', description: '扫描项目并生成 CLAUDE.md 项目记忆' },
  { command: '/memory', name: 'memory', description: '编辑 CLAUDE.md 等记忆文件' },
  { command: '/agents', name: 'agents', description: '管理子代理（创建/编辑/调用）' },
  { command: '/mcp', name: 'mcp', description: '查看 MCP 服务器连接状态' },
  { command: '/hooks', name: 'hooks', description: '管理生命周期 hooks' },
  { command: '/todos', name: 'todos', description: '查看当前会话的任务列表' },
  { command: '/resume', name: 'resume', description: '恢复历史会话' },
  { command: '/rewind', name: 'rewind', description: '回退对话或代码变更' },
  { command: '/export', name: 'export', description: '导出当前对话为文件或剪贴板' },
  { command: '/usage', name: 'usage', description: '查看套餐用量与限额' },
  { command: '/add-dir', name: 'add-dir', description: '为当前会话添加额外工作目录' },
  { command: '/doctor', name: 'doctor', description: '检查 claude 安装健康状态' }
]
