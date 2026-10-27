import { ElMessage } from 'element-plus'

// 错误码常量。与后端 model/app_error.go 错误码表保持一致。
// 后端经 Wails ErrorFormatter 将 AppError 序列化为 {code, message} 对象传前端，
// 前端按 code 分流提示级别（warning=预期拒绝，error=真失败）。
// 详见 docs/spec/logging-and-errors.md。
export const ErrorCode = Object.freeze({
  GitInProgress: 'E_GIT_IN_PROGRESS', // 变更类 Git 操作进行中，本次拒绝（warning）
  GitNoStagedChanges: 'E_GIT_NO_STAGED_CHANGES', // 无暂存文件，AI 提交信息生成按钮禁用（warning）
  GitCancelled: 'E_GIT_CANCELLED', // Git 操作被用户取消（warning，中性提示非 error）
  GitLocked: 'E_GIT_LOCKED', // index.lock 被占用或残留，写操作被拒（warning，引导人工确认后删除）
  GitTimeout: 'E_GIT_TIMEOUT', // Git 操作超时被终止（warning，用户可重试；超时不得误判为冲突正常返回）
  SearchTimeout: 'E_SEARCH_TIMEOUT', // 内容搜索超时被终止（warning，可缩小范围后重试；超时不得误判为无匹配）
  DiffToolNotConfigured: 'E_DIFF_TOOL_NOT_CONFIGURED', // 外部 diff 工具未配置或配置无效，引导用户去设置（warning）
  DiffToolLaunchFailed: 'E_DIFF_TOOL_LAUNCH_FAILED', // 外部 diff 工具启动失败（error）
  RepoConfigInvalidJson: 'E_REPO_CONFIG_INVALID_JSON', // 仓库列表配置导入：文件不是合法 JSON 或顶层结构缺失（error）
  RepoConfigUnsupportedVersion: 'E_REPO_CONFIG_UNSUPPORTED_VERSION', // 仓库列表配置导入：manifestVersion 缺失或高于当前支持（error）
  // AI 对话域错误码（AI 对话工作台多轮会话），取值与 model/app_error.go Chat 域段一致。
  ChatSessionNotFound: 'E_CHAT_SESSION_NOT_FOUND', // 会话不存在或已删除（error）
  ChatInProgress: 'E_CHAT_IN_PROGRESS', // 该会话已有对话进行中，本轮拒绝（warning；持续会话模型下后端已不再产生，保留映射兼容旧数据）
  ChatEmptyPrompt: 'E_CHAT_EMPTY_PROMPT', // 对话内容为空（warning）
  ChatProcessLimit: 'E_CHAT_PROCESS_LIMIT', // 同时活跃的对话会话进程已达上限，新会话无法启动（warning）
  // 进程管理域错误码（工具箱·进程管理：端口/关键字查进程 + 停止/强杀），
  // 取值与 model/app_error.go Process 域段一致。
  ProcessEnumFailed: 'E_PROCESS_ENUM_FAILED', // 进程枚举命令执行失败（error）
  ProcessEnumTimeout: 'E_PROCESS_ENUM_TIMEOUT', // 进程枚举超时被终止（warning，可重试；超时不得误判为「无进程」）
  ProcessNotFound: 'E_PROCESS_NOT_FOUND', // 目标进程已不存在，列表已过期（warning，自动刷新列表）
  ProcessAccessDenied: 'E_PROCESS_ACCESS_DENIED', // 权限不足或受保护进程（warning，引导以管理员身份运行）
  ProcessSelfProtect: 'E_PROCESS_SELF_PROTECT', // 拒绝终止本应用自身或父进程链（warning）
  ProcessKillFailed: 'E_PROCESS_KILL_FAILED', // 终止进程失败（error）
  // 环境变量查看域错误码（工具箱·环境变量），取值与 model/app_error.go 环境变量域段一致。
  EnvEnumFailed: 'E_ENV_ENUM_FAILED', // 环境变量枚举失败（error；单来源失败不进此错误，走分组 error 字段下发）
  // 素材管理域错误码（侧边栏·素材管理），取值与 model/app_error.go 素材管理域段一致。
  AssetsCancelled: 'E_ASSETS_CANCELLED', // 素材扫描被用户取消（warning，中性提示非 error）
  AssetsLibNotFound: 'E_ASSETS_LIB_NOT_FOUND', // 素材库不存在或已删除，列表已过期（error，刷新库列表）
  AssetsScanInProgress: 'E_ASSETS_SCAN_IN_PROGRESS', // 该素材库已有扫描进行中，本次拒绝（warning，可稍后重试）
  AssetsThumbsUnavailable: 'E_ASSETS_THUMBS_UNAVAILABLE', // 本机无 PowerPoint COM，缩略图/打开跳转不可用（error；前端提示条常驻 + 跳转入口隐藏）
  AssetsThumbsBusy: 'E_ASSETS_THUMBS_BUSY', // 缩略图跳转排队/执行超时（warning，COM 队列瞬态忙，可重试）
  AssetsThumbsJumpFailed: 'E_ASSETS_THUMBS_JUMP_FAILED', // 跳转执行失败：文件不存在/损坏/COM 调用失败（error，消息含可读原因）
  AssetsExportTargetExists: 'E_ASSETS_EXPORT_TARGET_EXISTS', // 导出目标已存在（CSV 文件已存在 / md 目录非空），确认覆盖后以 overwrite=true 重调（warning，确认型拒绝非真失败）
  AssetsRelinkMismatch: 'E_ASSETS_RELINK_MISMATCH', // 重链目标文件哈希与记录不一致，确认后以 force=true 重调（warning，确认型拒绝非真失败；强制重链按新文件重解析、标注保留）
  AssetsRelinkOutsideLib: 'E_ASSETS_RELINK_OUT_OF_LIB', // 重链目标位于素材根目录之外（warning，引导在库内重新选择；前缀替换新路径越界整体拒绝同码）
  AssetsPrefixNoMatch: 'E_ASSETS_PREFIX_REPLACE_NO_MATCH', // 前缀替换旧前缀零命中（warning，引导核对前缀拼写）
  AssetsDeleteBackupFailed: 'E_ASSETS_DELETE_BACKUP_FAILED', // 删库前自动备份失败（warning，确认型拒绝非真失败——删除未发生，用户确认「删除后无恢复点」后以 allowNoBackup=true 重调）
  AssetsFolderNotEmpty: 'E_ASSETS_FOLDER_NOT_EMPTY', // 非空目录 force=false 删除被拒（warning，确认型拒绝非真失败——删除未发生，前端二次确认后以 force=true 重调）
  // 素材购物栏域错误码（素材管理·购物栏），取值与 model/app_error.go 购物栏域段一致。
  CartExportInProgress: 'E_CART_EXPORT_IN_PROGRESS', // 已有合并导出任务进行中，本次拒绝（warning，等待完成或先取消）
  // 浏览器访问通道 token 管理域错误码（多 token 管理），取值与 model/app_error.go
  // token 管理域段一致，均为预期拒绝/用户操作错误（warning）。
  TokenNotFound: 'E_TOKEN_NOT_FOUND', // 目标令牌记录不存在或已被删除（列表过期，引导刷新）
  TokenNameRequired: 'E_TOKEN_NAME_REQUIRED', // 令牌名称（设备/用途标识）为空
  TokenExpiryInvalid: 'E_TOKEN_EXPIRY_INVALID', // 有效期早于当前时间（自定义过期档位校验拒绝）
  // 浏览器访问通道 TLS 域错误码（内置 TLS），取值与 model/app_error.go TLS 域段一致。
  TLSCertInvalid: 'E_TLS_CERT_INVALID', // TLS 证书加载/生成失败：路径无效、文件损坏、过期或证书私钥不配对（error，修正配置或删除自签文件后重试）
  // 浏览器访问通道连接管控域错误码（踢出连接 + 临时封禁），取值与 model/app_error.go
  // 连接管控域段一致，均为预期拒绝/引导类（warning）。
  WebServeNotRunning: 'E_WEBSERVE_NOT_RUNNING', // 浏览器访问服务未运行，踢出连接不可达（warning；停机/未启用态）
  ConnNotFound: 'E_CONNECTION_NOT_FOUND', // 目标连接不存在或已断开，列表已过期（warning，引导刷新后重试）
  TokenRevoked: 'E_TOKEN_REVOKED', // 封禁已撤销令牌被拒（撤销即凭据失效，无需封禁；warning，引导刷新列表）
  // 浏览器访问通道 IP 封禁域错误码（连接管控七期手动封禁 + 八期网段封禁），
  // 取值与 model/app_error.go IP 封禁域段一致，均为预期拒绝/引导类（warning）。
  IPBanWhitelisted: 'E_IP_BAN_WHITELISTED', // 封禁白名单/回环 IP 被拒：白名单为可信来源语义面，引导先在设置页移除对应白名单条目
  IPBanIPRequired: 'E_IP_BAN_IP_REQUIRED', // 手动封禁的来源 IP 为空（必填校验拒绝，前端表单已挡，后端防御保留）
  IPBanCidrInvalid: 'E_IP_BAN_CIDR_INVALID', // 封禁网段格式非法：须为 CIDR 串或裸 IP（前端表单已挡，后端解析兜底拒绝）
  IPBanCidrWhitelisted: 'E_IP_BAN_CIDR_WHITELISTED', // 封禁网段被可信网段白名单完全覆盖：封了也零效果，引导先移除对应白名单条目
  // PPTX 预览转换域错误码（文件树 pptx/ppsx 内嵌预览：COM 转 PDF + pdfjs viewer），
  // 取值与 model/app_error.go PPTX 预览转换域段一致。
  PptxPreviewUnavailable: 'E_PPTX_PREVIEW_UNAVAILABLE', // 环境不可用：非 Windows 或未检测到 PowerPoint/WPS COM（error，前端降级提示 + 用默认程序打开）
  PptxPreviewFailed: 'E_PPTX_PREVIEW_FAILED', // 转换失败：文件损坏/COM 调用失败/超时（error，前端降级提示 + 用默认程序打开）
  // RPC 协议层错误码（serve 浏览器模式 /api/rpc 传输协议错误，非业务域错误）。
  // 取值与 model/app_error.go RPC 协议段常量表一致（server/rpc.go 以别名引用），
  // 由 src/transport/rpc.js 在浏览器模式下 reject 出，前端按 error 分流提示。
  RpcBadRequest: 'E_RPC_BAD_REQUEST', // 请求体非法（非 JSON、缺 method）或方法不允许（error）
  RpcMethodNotFound: 'E_RPC_METHOD_NOT_FOUND', // 请求的方法不存在或未导出（error）
  RpcMethodUnsupported: 'E_RPC_METHOD_UNSUPPORTED', // 方法签名形态不受支持，如变参方法（error）
  RpcArgCountMismatch: 'E_RPC_ARG_COUNT_MISMATCH', // 参数个数与方法签名不符（error）
  RpcArgTypeMismatch: 'E_RPC_ARG_TYPE_MISMATCH', // 参数值无法转换为签名要求的类型（error）
  RpcInternal: 'E_RPC_INTERNAL' // 非 AppError 的通用内部错误/网络失败（error）
})

// 预期拒绝类错误码（用户可重试），弹 warning 而非 error。
const WARNING_CODES = new Set([
  ErrorCode.GitInProgress,
  ErrorCode.GitNoStagedChanges,
  ErrorCode.GitCancelled,
  ErrorCode.GitLocked,
  ErrorCode.GitTimeout,
  ErrorCode.SearchTimeout,
  ErrorCode.DiffToolNotConfigured,
  ErrorCode.ChatInProgress,
  ErrorCode.ChatProcessLimit,
  ErrorCode.ChatEmptyPrompt,
  ErrorCode.ProcessEnumTimeout,
  ErrorCode.ProcessNotFound,
  ErrorCode.ProcessAccessDenied,
  ErrorCode.ProcessSelfProtect,
  ErrorCode.AssetsCancelled,
  ErrorCode.AssetsScanInProgress,
  ErrorCode.AssetsThumbsBusy,
  ErrorCode.AssetsExportTargetExists,
  ErrorCode.AssetsRelinkMismatch,
  ErrorCode.AssetsRelinkOutsideLib,
  ErrorCode.AssetsPrefixNoMatch,
  ErrorCode.AssetsDeleteBackupFailed,
  ErrorCode.AssetsFolderNotEmpty,
  ErrorCode.CartExportInProgress,
  ErrorCode.TokenNotFound,
  ErrorCode.TokenNameRequired,
  ErrorCode.TokenExpiryInvalid,
  ErrorCode.WebServeNotRunning,
  ErrorCode.ConnNotFound,
  ErrorCode.TokenRevoked,
  ErrorCode.IPBanWhitelisted,
  ErrorCode.IPBanIPRequired,
  ErrorCode.IPBanCidrInvalid,
  ErrorCode.IPBanCidrWhitelisted
])

/**
 * 从错误对象提取 code 与 message。
 *
 * Wails ErrorFormatter 返回结构化对象 {code, message}（非 Error 实例），
 * 故优先读对象属性；兼容旧路径裸 Error（无 code，走 message 字符串）。
 *
 * @param {unknown} error
 * @returns {{code: string|null, message: string}}
 */
function extractError(error) {
  if (!error) return { code: null, message: '' }
  const code = error?.code ?? null
  const rawMessage = error?.message
  if (typeof rawMessage === 'string') {
    return { code, message: rawMessage }
  }
  // 兜底：message 字段缺失或非字符串的对象形态（如 reject 值为无 message 字段
  // 的普通对象）禁止产出 "[object Object]"——plain object 走 JSON.stringify
  // 保留可诊断字段，其余（字符串/数字等原始值）走 String()。
  try {
    const message = typeof error === 'object' ? JSON.stringify(error) : String(error)
    return { code, message }
  } catch {
    // 循环引用等 stringify 失败：退回 String()（至多 "[object Object]"，不抛错）
    return { code, message: String(error) }
  }
}

/**
 * 判断错误是否为「仓库已有 Git 操作进行中」拒绝。
 * 优先读 code（新路径），兜底文案匹配（未迁移路径）。
 * @param {unknown} error
 * @returns {boolean}
 */
export function isGitOperationInProgress(error) {
  const { code, message } = extractError(error)
  if (code === ErrorCode.GitInProgress) return true
  return message.includes('该仓库有 Git 操作进行中，请稍后重试')
}

/**
 * 判断错误是否为预期拒绝类（用户可重试），应弹 warning。
 * 优先 code 命中 WARNING_CODES（新路径），兜底 GitInProgress 文案匹配（未迁移裸 Error）。
 * @param {string|null} code
 * @param {string} message
 * @returns {boolean}
 */
function isWarningError(code, message) {
  if (code && WARNING_CODES.has(code)) return true
  return message.includes('该仓库有 Git 操作进行中，请稍后重试')
}

/**
 * 统一处理错误反馈。按 code 分流：
 * - 预期拒绝类（如 GitInProgress）→ ElMessage.warning(message)，不拼前缀
 * - 真失败 → ElMessage.error(prefix + message)
 *
 * 用法：在组件 catch 块中用 `handleError('提交失败: ', error)` 替代裸 ElMessage.error。
 *
 * @param {string} prefix 失败前缀（如 '提交失败: '）
 * @param {unknown} error 捕获的错误对象（Wails 结构化 {code, message} 或裸 Error）
 */
export function handleError(prefix, error) {
  const { code, message } = extractError(error)
  if (isWarningError(code, message)) {
    // 预期拒绝类，用 warning 引导用户重试，不拼前缀（后端 message 已自解释）
    ElMessage.warning(message)
    return
  }
  ElMessage.error(prefix + message)
}

/**
 * handleGitError 是 handleError 的 git 域别名，保留兼容现有调用点。
 * 新代码直接用 handleError。
 * @param {string} prefix
 * @param {unknown} error
 */
export function handleGitError(prefix, error) {
  handleError(prefix, error)
}
