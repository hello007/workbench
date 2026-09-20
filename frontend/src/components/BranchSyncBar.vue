<template>
  <div v-if="syncInfo" class="branch-sync-bar">
    <el-tooltip
      content="远程位置基于上次 fetch 快照，点右侧刷新获取最新远程状态"
      placement="top"
    >
      <span class="branch-name" :class="{ 'is-detached': syncInfo.detached }">
        {{ syncInfo.branch }}
      </span>
    </el-tooltip>

    <template v-if="syncInfo.detached">
      <el-tag type="danger" size="small">分离头指针</el-tag>
    </template>

    <template v-else>
      <!-- 计数语义对齐 DashboardView：未推送(ahead)=danger、未拉取(behind)=warning -->
      <el-tag
        v-if="syncInfo.hasUpstream && syncInfo.ahead > 0"
        type="danger"
        size="small"
        class="count-tag"
      >↑{{ syncInfo.ahead }}</el-tag>
      <el-tag
        v-if="syncInfo.hasUpstream && syncInfo.behind > 0"
        type="warning"
        size="small"
        class="count-tag"
      >↓{{ syncInfo.behind }}</el-tag>

      <!-- 无上游（引导 set-upstream）或有未推送提交时显示推送按钮 -->
      <el-button
        v-if="showPush"
        type="danger"
        plain
        size="small"
        :loading="pushing"
        class="sync-btn"
        @click="doPush"
      >推送</el-button>

      <!-- split 下拉：主按钮拉取(rebase)，下拉项拉取(merge)；无上游时隐藏 -->
      <el-dropdown
        v-if="syncInfo.hasUpstream && syncInfo.behind > 0"
        split-button
        type="warning"
        size="small"
        class="pull-dropdown"
        :disabled="pulling"
        @click="doPull(true)"
        @command="onPullCommand"
      >
        拉取(rebase)
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="merge">拉取(merge)</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>

      <el-tooltip content="从远程刷新引用（fetch origin），失败不影响本地操作" placement="top">
        <el-button
          size="small"
          circle
          :icon="Refresh"
          :loading="fetching"
          class="fetch-btn"
          @click="doFetch"
        />
      </el-tooltip>
    </template>
  </div>

  <!-- 推送结果弹窗：超长 output 完整展示（对齐 LocalChanges 推送范式） -->
  <PushResultDialog v-model="pushResultVisible" :output="pushResultOutput" />
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import {
  GetBranchSyncInfo,
  HasUpstream,
  PushRepo,
  PullRepo,
  FetchRepo
} from '../../wailsjs/go/main/App'
import PushResultDialog from './PushResultDialog.vue'
import { handleGitError } from '../utils/error'

const props = defineProps({
  repoPath: { type: String, required: true }
})

// update:info：每次摘要刷新成功（挂载/切仓库/操作后）携带数据上抛，父组件取 refs 渲染提交行 badge
// synced：push/pull/fetch 操作成功后携带来源标识（'push'|'pull'|'fetch'）上抛，
// 父组件按来源分流——push/pull 改变 HEAD 清缓存重载，fetch 仅更新远程引用不重载
const emit = defineEmits(['synced', 'update:info'])

const syncInfo = ref(null)
const pushing = ref(false)
const pulling = ref(false)
const fetching = ref(false)

// 推送结果弹窗状态：output > 200 字符时弹 Dialog 完整展示，对齐 LocalChanges 推送范式
const pushResultVisible = ref(false)
const pushResultOutput = ref('')

// 无上游（引导 set-upstream）或有未推送提交时显示推送按钮；detached 无分支不可推
const showPush = computed(() => {
  if (!syncInfo.value || syncInfo.value.detached) return false
  return !syncInfo.value.hasUpstream || syncInfo.value.ahead > 0
})

// refresh 拉取摘要。失败静默降级为隐藏摘要条（不阻塞提交历史主功能，不弹错误）。
// 过期响应防护：await 前捕获发起时的 repoPath，resolve 后若已切仓库则丢弃——
// 旧仓库在途请求晚于新仓库返回时，不写状态不 emit，防旧仓库 refs 经 update:info 污染父组件 badge。
const refresh = async () => {
  if (!props.repoPath) {
    syncInfo.value = null
    return
  }
  const requestedPath = props.repoPath
  try {
    const info = await GetBranchSyncInfo(requestedPath)
    if (requestedPath !== props.repoPath) return
    syncInfo.value = info || null
    if (syncInfo.value) {
      emit('update:info', syncInfo.value)
    }
  } catch {
    if (requestedPath !== props.repoPath) return
    // 摘要获取失败（非仓库/损坏库等）降级隐藏，不阻塞不报错
    syncInfo.value = null
  }
}

/**
 * 推送：先 HasUpstream 判断；无上游弹确认是否 set-upstream（对齐 LocalChanges 推送流程）。
 */
const doPush = async () => {
  pushing.value = true
  try {
    let setUpstream = false
    try {
      const has = await HasUpstream(props.repoPath)
      if (!has) {
        try {
          await ElMessageBox.confirm(
            '当前分支无上游，是否设置上游（git push --set-upstream origin <当前分支>）并推送？',
            '无上游分支',
            { confirmButtonText: '设置并推送', cancelButtonText: '取消', type: 'warning' }
          )
          setUpstream = true
        } catch {
          // 用户取消
          ElMessage.info('已取消推送')
          return
        }
      }
    } catch {
      // HasUpstream 探测失败：按常规推送（不 set-upstream），让 git 报错透传
      ElMessage.warning('无法判断上游分支，将尝试常规推送')
    }

    const output = await PushRepo(props.repoPath, setUpstream)
    const text = (output || '').trim()
    if (text.length > 200) {
      // 超长输出弹 Dialog 完整展示，不截断
      pushResultOutput.value = text
      pushResultVisible.value = true
    } else {
      ElMessage.success(text || '推送完成')
    }
    emit('synced', 'push')
    await refresh()
  } catch (error) {
    handleGitError('推送失败: ', error)
  } finally {
    pushing.value = false
  }
}

/**
 * 拉取：useRebase=true 走 pull --rebase（主按钮默认），false 走普通 pull（下拉项）。
 */
const doPull = async (useRebase) => {
  pulling.value = true
  try {
    const output = await PullRepo(props.repoPath, useRebase)
    const text = (output || '').trim()
    ElMessage.success(text ? text.split('\n')[0] : (useRebase ? '拉取(rebase)完成' : '拉取(merge)完成'))
    emit('synced', 'pull')
    await refresh()
  } catch (error) {
    handleGitError('拉取失败: ', error)
  } finally {
    pulling.value = false
  }
}

const onPullCommand = (command) => {
  doPull(command !== 'merge')
}

/**
 * fetch：刷新远程引用后重算摘要与 badge（refresh 成功经 update:info 上抛，父组件 refs badge 随之更新）。
 * 失败仅提示，不阻塞（behind 保持上次 fetch 快照语义）。
 */
const doFetch = async () => {
  fetching.value = true
  try {
    await FetchRepo(props.repoPath, 'origin', false)
    ElMessage.success('fetch 完成，远程引用已更新')
    emit('synced', 'fetch')
    await refresh()
  } catch (error) {
    handleGitError('fetch 失败: ', error)
  } finally {
    fetching.value = false
  }
}

watch(() => props.repoPath, () => {
  // 切仓库：清旧摘要（防旧仓库数据闪现）后重新拉取
  syncInfo.value = null
  refresh()
})

onMounted(() => {
  refresh()
})

defineExpose({ refresh })
</script>

<style scoped>
.branch-sync-bar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
}
.branch-name {
  font-weight: 600;
  font-size: 13px;
  color: var(--text-primary);
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
}
.branch-name.is-detached {
  color: var(--danger-color);
}
.count-tag {
  flex-shrink: 0;
}
.sync-btn {
  flex-shrink: 0;
}
.pull-dropdown {
  flex-shrink: 0;
}
.fetch-btn {
  flex-shrink: 0;
}
</style>
