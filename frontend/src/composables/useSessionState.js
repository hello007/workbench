/**
 * 会话快照（崩溃恢复 UI 状态）前端编排。
 *
 * 职责拆分：
 *   - buildSessionState(): 从 Pinia store 读取可恢复状态构建快照对象（字段对齐后端 model.SessionState）
 *   - applySessionState(state): 将快照写回 store（仅恢复有效值，非法值忽略走默认冷启动）
 *   - restoreSession(): 调 GetSessionState 读取后端快照，空/失败返回 null（调用方走冷启动）
 *   - startSessionAutoSave(): debounce 自动保存 + beforeunload 最终保存，返回 { markRestored, dispose }
 *
 * 设计依据：
 *   - 文件树展开状态由 useTreeState 持久化到 localStorage（按工作目录隔离），恢复 selectedDirectoryId
 *     后由 Home.vue 调 onDirectorySelect 触发 localStorage 还原，不纳入本快照避免重复。
 *   - 终端不真实复用进程，仅恢复可见性/高度/工作目录，由 TerminalPanel 用 WorkDir 新建会话。
 *   - 启动恢复须在 directories 加载后由 Home.vue 编排（恢复 selectedDirectoryId 须校验目录仍存在）。
 *
 * 详见 docs/spec/cross-layer-contracts.md 与 .trellis/tasks/09-14-v1-4/prd.md PR2。
 */
import { watch } from 'vue'
import { useDirectoryStore, useUiStore } from '../store'
import { GetSessionState, SaveSessionState } from '../../wailsjs/go/main/App'

/** activePanel 合法值集合（与 ActivityBar.vue items 对齐），非法值忽略走默认 directory。 */
const VALID_PANELS = new Set(['directory', 'ai', 'stats', 'toolbox'])

/** debounce 写入间隔（毫秒），避免高频 IO（每次 store 变化合并为一次写入）。 */
const SAVE_DEBOUNCE_MS = 2000

/**
 * 从 Pinia store 构建会话快照对象（字段对齐后端 model.SessionState）。
 * @returns {Object} SessionState 快照（始终含 terminal 子对象，由后端 Save 补 version/savedAt）
 */
export function buildSessionState() {
  const directoryStore = useDirectoryStore()
  const uiStore = useUiStore()
  return {
    selectedDirectoryId: directoryStore.selectedDirectoryId || '',
    activePanel: uiStore.activePanel || '',
    terminal: {
      visible: !!uiStore.terminalVisible,
      height: uiStore.terminalHeight || 0,
      workDir: uiStore.terminalDir || ''
    }
  }
}

/**
 * 将快照写回 Pinia store（仅恢复 activePanel 与终端面板状态）。
 * selectedDirectoryId 的恢复须由 Home.vue 在 directories 加载后校验存在性并调 onDirectorySelect
 * （需 fileTreePanelRef 触发文件树重载 + localStorage 展开状态还原），不在本函数处理。
 * @param {Object|null} state 后端快照
 * @returns {boolean} 是否恢复了任意 UI 状态（用于决定是否提示「已恢复上次会话」）
 */
export function applySessionState(state) {
  if (!state) return false
  const uiStore = useUiStore()
  let touched = false

  // 恢复活动面板（非法值忽略，保持默认 directory）
  if (state.activePanel && VALID_PANELS.has(state.activePanel)) {
    uiStore.activePanel = state.activePanel
    touched = true
  }

  // 恢复终端面板状态
  if (state.terminal) {
    if (state.terminal.visible) {
      uiStore.terminalVisible = true
      touched = true
    }
    if (state.terminal.height > 0) {
      uiStore.terminalHeight = state.terminal.height
      touched = true
    }
    if (state.terminal.workDir) {
      uiStore.terminalDir = state.terminal.workDir
      touched = true
    }
  }

  return touched
}

/**
 * 读取后端会话快照。空快照（首次启动 / 崩溃后无快照）或读取失败返回 null，
 * 调用方据此走默认冷启动。读取失败静默（后端 Load 损坏已降级返回空，不阻塞启动）。
 * @returns {Promise<Object|null>}
 */
export async function restoreSession() {
  try {
    const state = await GetSessionState()
    // 空快照判定：所有可恢复字段都缺省 → 视为无快照
    if (!state) return null
    if (!state.selectedDirectoryId && !state.activePanel && !state.terminal) return null
    return state
  } catch {
    // 读取失败不阻塞启动，静默走冷启动
    return null
  }
}

/**
 * 启动 debounce 自动保存 + beforeunload 最终保存。
 *
 * 仅在调用方完成初始恢复并调 markRestored() 后生效，避免恢复阶段 store 写入触发保存
 * （恢复写回与快照内容一致，保存无意义且可能覆盖尚未读取的旧快照）。
 *
 * beforeunload 最终保存为 best-effort：Wails IPC 异步，unload 不 await，
 * 实际依赖 debounce 在状态变化后 2s 内落盘；shutdown 前最后一次状态变化若距关闭不足 2s
 * 可能丢失（MVP 可接受，崩溃恢复覆盖大部分场景）。
 *
 * @returns {{ markRestored: () => void, dispose: () => void }}
 *   markRestored 标记恢复完成启用自动保存；dispose 移除监听（组件卸载时调用）
 */
export function startSessionAutoSave() {
  const directoryStore = useDirectoryStore()
  const uiStore = useUiStore()
  let restored = false
  let saveTimer = null

  const flushSave = () => {
    if (saveTimer) {
      clearTimeout(saveTimer)
      saveTimer = null
    }
    // best-effort 同步触发（beforeunload 场景不等 debounce）
    try {
      SaveSessionState(buildSessionState())
    } catch {
      // 保存失败静默（后端已 slog 记录），不阻塞 UI
    }
  }

  const triggerSave = () => {
    if (!restored) return
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(async () => {
      saveTimer = null
      try {
        await SaveSessionState(buildSessionState())
      } catch {
        // 保存失败静默（后端已 slog 记录），不阻塞 UI
      }
    }, SAVE_DEBOUNCE_MS)
  }

  // 监听可恢复状态变化（selectedDirectoryId / activePanel / 终端三态）
  const stopWatch = watch(
    () => [
      directoryStore.selectedDirectoryId,
      uiStore.activePanel,
      uiStore.terminalVisible,
      uiStore.terminalHeight,
      uiStore.terminalDir
    ],
    triggerSave
  )

  // beforeunload 最终保存（best-effort，不 await）
  const onBeforeUnload = () => {
    if (!restored) return
    flushSave()
  }
  window.addEventListener('beforeunload', onBeforeUnload)

  const markRestored = () => {
    restored = true
  }

  const dispose = () => {
    // 停止 watch 防止组件卸载后仍响应 store 变化触发保存
    stopWatch()
    if (saveTimer) {
      clearTimeout(saveTimer)
      saveTimer = null
    }
    window.removeEventListener('beforeunload', onBeforeUnload)
  }

  return { markRestored, dispose }
}
