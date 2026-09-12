// gitError.js 已迁移至 error.js（通用错误处理 helper）。
// 本文件保留重导出，兼容现有组件 import { handleGitError, isGitOperationInProgress } from '@/utils/gitError'。
// 新代码请直接 import from '@/utils/error'。详见 docs/spec/logging-and-errors.md。
export { handleGitError, isGitOperationInProgress, handleError, ErrorCode } from './error'
