<template>
  <el-card class="git-merge-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>Git 合并 / 变基 / 拣选</span>
        <div class="header-actions">
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="refreshConflictState"
            :loading="loadingConflict"
          />
        </div>
      </div>
    </template>

    <!-- 操作区 -->
    <div class="merge-section">
      <el-form label-position="top">
        <el-form-item label="操作类型">
          <el-radio-group v-model="opType">
            <el-radio value="merge">合并</el-radio>
            <el-radio value="rebase">变基</el-radio>
            <el-radio value="cherry-pick">拣选</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="targetLabel">
          <el-input
            v-model="target"
            :placeholder="targetPlaceholder"
            @keyup.enter="execute"
          />
        </el-form-item>
        <el-form-item v-if="opType === 'merge'" label="合并策略">
          <el-select v-model="mergeMode" style="width: 100%">
            <el-option label="快进合并（可快进时直接前移指针）" value="ff" />
            <el-option label="保留合并记录 (--no-ff)" value="no-ff" />
            <el-option label="压缩合并 (--squash)" value="squash" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="execute-actions">
        <el-button type="primary" @click="execute" :loading="executing">执行</el-button>
      </div>
      <div v-if="lastOutput" class="last-output" :title="lastOutput">{{ lastOutput }}</div>
    </div>

    <!-- 冲突态面板 -->
    <div v-if="conflictState.type !== 'none'" class="conflict-panel">
      <el-divider />
      <div class="conflict-header">
        <el-tag type="warning">冲突未解决</el-tag>
        <el-text class="conflict-type-text">{{ conflictTypeLabel }}</el-text>
        <el-text class="conflict-tip" type="info" size="small">
          用外部编辑器修改冲突标记后回此标记已解决，全部解决后点继续
        </el-text>
      </div>
      <div v-if="conflictState.files.length > 0" class="conflict-list">
        <div v-for="file in conflictState.files" :key="file" class="conflict-row">
          <el-text class="conflict-file" :title="file">{{ file }}</el-text>
          <div class="conflict-row-actions">
            <el-button size="small" text @click="openFile(file)">打开</el-button>
            <el-button
              size="small"
              text
              type="success"
              @click="resolveFile(file)"
              :loading="resolvingFile === file"
            >标记已解决</el-button>
          </div>
        </div>
      </div>
      <el-empty v-else description="无未解决冲突文件，可直接继续" />
      <div class="conflict-actions">
        <el-button type="primary" @click="continueOp" :loading="continuing">继续</el-button>
        <el-button
          v-if="conflictState.type === 'rebase'"
          @click="skipOp"
          :loading="skipping"
        >跳过</el-button>
        <el-button type="danger" @click="abortOp" :loading="aborting">中止</el-button>
      </div>
    </div>
  </el-card>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import {
  Merge, Rebase, CherryPick, GetConflictState, ResolveConflict,
  ContinueMerge, ContinueRebase, ContinueCherryPick,
  AbortMerge, AbortRebase, AbortCherryPick, SkipRebase,
  OpenInVSCode
} from '../../wailsjs/go/main/App'
import { handleGitError } from '../utils/gitError'

const props = defineProps({
  repoPath: { type: String, required: true }
})

// 操作类型：merge / rebase / cherry-pick
const opType = ref('merge')
// 目标分支名（merge/rebase）或提交 SHA（cherry-pick）
const target = ref('')
// 合并策略，仅 merge 时生效
const mergeMode = ref('ff')

// 冲突态快照：{ type: 'none'|'merge'|'rebase'|'cherry-pick', files: [] }
const conflictState = ref({ type: 'none', files: [] })

const executing = ref(false)
const loadingConflict = ref(false)
const continuing = ref(false)
const aborting = ref(false)
const skipping = ref(false)
const resolvingFile = ref('')
// 最近一次操作输出（git stdout），超长截断展示
const lastOutput = ref('')

const targetLabel = computed(() => {
  if (opType.value === 'cherry-pick') return '提交 SHA'
  return '目标分支'
})

const targetPlaceholder = computed(() => {
  if (opType.value === 'cherry-pick') return '输入要拣选的提交 SHA，如 a1b2c3d'
  if (opType.value === 'rebase') return '输入变基目标分支，如 main'
  return '输入要合并的目标分支，如 feature/x'
})

const conflictTypeLabel = computed(() => {
  switch (conflictState.value.type) {
    case 'merge': return '合并冲突'
    case 'rebase': return '变基冲突'
    case 'cherry-pick': return '拣选冲突'
    default: return ''
  }
})

// 仓库根路径与冲突文件相对路径拼接为绝对路径，交给外部编辑器打开。
// conflictState.files 为相对仓库根的路径（git diff --name-only --diff-filter=U 输出）。
const joinPath = (base, rel) => `${base.replace(/[/\\]+$/, '')}/${rel}`

const refreshConflictState = async () => {
  loadingConflict.value = true
  try {
    const state = await GetConflictState(props.repoPath)
    conflictState.value = state || { type: 'none', files: [] }
  } catch (error) {
    ElMessage.error('获取冲突状态失败: ' + (error.message || String(error)))
  } finally {
    loadingConflict.value = false
  }
}

const execute = async () => {
  const t = target.value.trim()
  if (!t) {
    ElMessage.warning(opType.value === 'cherry-pick' ? '提交 SHA 不能为空' : '目标分支不能为空')
    return
  }
  executing.value = true
  lastOutput.value = ''
  try {
    let output = ''
    if (opType.value === 'merge') {
      output = await Merge(props.repoPath, t, mergeMode.value)
    } else if (opType.value === 'rebase') {
      output = await Rebase(props.repoPath, t)
    } else {
      output = await CherryPick(props.repoPath, t)
    }
    lastOutput.value = output || ''
    // 操作完成（含冲突态 exit 1 但后端已返回）后刷新冲突态：有冲突则展示面板
    await refreshConflictState()
    if (conflictState.value.type === 'none') {
      ElMessage.success('操作完成' + (output ? `\n${output}` : ''))
      target.value = ''
    } else {
      ElMessage.warning('存在冲突，请在冲突面板解决后继续')
    }
  } catch (error) {
    // 冲突时后端可能以 exit 1 返回，ExecuteWithCodes 已吸收为正常返回；
    // 此处捕获的是真错误（前置校验失败、命令异常、并发拒绝等）
    handleGitError('操作失败: ', error)
    await refreshConflictState()
  } finally {
    executing.value = false
  }
}

const openFile = async (file) => {
  try {
    const ok = await OpenInVSCode(joinPath(props.repoPath, file))
    if (!ok) {
      ElMessage.error('打开 VSCode 失败，请确认已安装 VSCode 并将 code 命令加入 PATH')
    }
  } catch (error) {
    ElMessage.error('打开文件失败: ' + (error.message || String(error)))
  }
}

const resolveFile = async (file) => {
  resolvingFile.value = file
  try {
    await ResolveConflict(props.repoPath, file)
    await refreshConflictState()
  } catch (error) {
    handleGitError('标记已解决失败: ', error)
  } finally {
    resolvingFile.value = ''
  }
}

const continueOp = async () => {
  continuing.value = true
  lastOutput.value = ''
  try {
    const type = conflictState.value.type
    let output = ''
    if (type === 'merge') {
      output = await ContinueMerge(props.repoPath)
    } else if (type === 'rebase') {
      output = await ContinueRebase(props.repoPath)
    } else if (type === 'cherry-pick') {
      output = await ContinueCherryPick(props.repoPath)
    }
    lastOutput.value = output || ''
    await refreshConflictState()
    if (conflictState.value.type === 'none') {
      ElMessage.success('冲突已解决，操作完成' + (output ? `\n${output}` : ''))
    }
  } catch (error) {
    handleGitError('继续操作失败: ', error)
    await refreshConflictState()
  } finally {
    continuing.value = false
  }
}

const abortOp = async () => {
  aborting.value = true
  try {
    const type = conflictState.value.type
    if (type === 'merge') {
      await AbortMerge(props.repoPath)
    } else if (type === 'rebase') {
      await AbortRebase(props.repoPath)
    } else if (type === 'cherry-pick') {
      await AbortCherryPick(props.repoPath)
    }
    ElMessage.success('已中止操作')
    await refreshConflictState()
  } catch (error) {
    handleGitError('中止操作失败: ', error)
    await refreshConflictState()
  } finally {
    aborting.value = false
  }
}

const skipOp = async () => {
  skipping.value = true
  lastOutput.value = ''
  try {
    const output = await SkipRebase(props.repoPath)
    lastOutput.value = output || ''
    await refreshConflictState()
    if (conflictState.value.type === 'none') {
      ElMessage.success('已跳过当前提交，变基完成' + (output ? `\n${output}` : ''))
    }
  } catch (error) {
    handleGitError('跳过失败: ', error)
    await refreshConflictState()
  } finally {
    skipping.value = false
  }
}

watch(() => props.repoPath, () => {
  target.value = ''
  lastOutput.value = ''
  conflictState.value = { type: 'none', files: [] }
  refreshConflictState()
})

// 挂载即检测冲突态：仓库可能正处于 merge/rebase 中途
refreshConflictState()

defineExpose({ refreshConflictState })
</script>

<style scoped>
.git-merge-card {
  border-radius: var(--radius-md);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 600;
  font-size: 16px;
  color: var(--text-primary);
}
.header-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
.merge-section {
  min-height: 60px;
}
.execute-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--spacing-sm);
}
.last-output {
  margin-top: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.conflict-panel {
  margin-top: var(--spacing-sm);
}
.conflict-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-sm);
}
.conflict-type-text {
  font-weight: 600;
  color: var(--text-primary);
}
.conflict-tip {
  margin-left: auto;
}
.conflict-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}
.conflict-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--el-color-warning);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: all var(--transition-fast);
}
.conflict-row:hover {
  box-shadow: var(--shadow-md);
}
.conflict-file {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: var(--primary-color);
  word-break: break-all;
}
.conflict-row-actions {
  display: flex;
  gap: var(--spacing-xs);
  flex-shrink: 0;
}
.conflict-actions {
  display: flex;
  gap: var(--spacing-sm);
  justify-content: flex-end;
  margin-top: var(--spacing-md);
}
</style>
