<!-- frontend/src/views/BranchCompareView.vue -->
<!-- 分支对比面板（活动栏一级入口，只读 batch 1）：
     R1 仓库选择（默认 currentDirPath 种子 + 浏览按钮换目录，GetGitInfo 校验）
     R2 分支 A/B 下拉（本地+远程）+ 交换 + 开始对比 + 刷新
     R3 对齐提交表 / R4 统计条 / R5 文件树 / R6 diff 查看器 / R7 提交详情 / R8 表内搜索
     R9 只读（无任何写操作入口）/ R10 错误经 handleError 分流（超时/取消/锁走 warning）
     requestSeq 串行化丢弃并发旧响应（参照 StatsView 模式）。 -->
<template>
  <div class="bc-view">
    <!-- 顶部工具行：仓库 + 分支选择 + 动作 -->
    <div class="bc-header">
      <span class="bc-title">分支对比</span>
      <span class="repo-path" :title="repoPath">{{ repoPath || '未选择仓库' }}</span>
      <el-button size="small" @click="browseRepo">浏览</el-button>
      <el-select
        v-model="branchA"
        class="branch-select"
        size="small"
        filterable
        placeholder="分支 A"
        :loading="branchesLoading"
      >
        <el-option v-for="b in branches" :key="b.name" :label="b.name" :value="b.name">
          <span>{{ b.name }}</span>
          <el-tag v-if="b.isRemote" size="small" type="info">远程</el-tag>
        </el-option>
      </el-select>
      <el-button size="small" :disabled="!branchA && !branchB" @click="swapBranches">交换</el-button>
      <el-select
        v-model="branchB"
        class="branch-select"
        size="small"
        filterable
        placeholder="分支 B"
        :loading="branchesLoading"
      >
        <el-option v-for="b in branches" :key="b.name" :label="b.name" :value="b.name">
          <span>{{ b.name }}</span>
          <el-tag v-if="b.isRemote" size="small" type="info">远程</el-tag>
        </el-option>
      </el-select>
      <el-button
        type="primary"
        size="small"
        :loading="comparing"
        :disabled="!repoPath || !branchA || !branchB"
        @click="runCompare"
      >开始对比</el-button>
      <el-button size="small" :loading="comparing" :disabled="!repoPath" @click="refreshCompare">刷新</el-button>
    </div>

    <div class="bc-body">
      <!-- 空态 1：未选仓库 -->
      <div v-if="!repoPath" class="bc-empty">
        <el-empty description="请先选择一个 Git 仓库（默认取当前工作目录，或点击「浏览」选择）" />
      </div>
      <!-- 空态 2：非 git 仓库 -->
      <div v-else-if="!isRepo" class="bc-empty">
        <el-empty description="当前目录不是 Git 仓库，请点击「浏览」切换到 Git 仓库目录" />
      </div>
      <!-- 空态 3：分支未选齐 -->
      <div v-else-if="!branchA || !branchB" class="bc-empty">
        <el-empty description="请选择要对比的两个分支（分支 A / 分支 B），然后点击「开始对比」" />
      </div>
      <!-- 空态 4：同分支 -->
      <div v-else-if="branchA === branchB" class="bc-empty">
        <el-empty description="分支 A 与分支 B 相同，请选择不同的分支" />
      </div>

      <template v-else>
        <!-- 统计条（R4）：截断 / 无共同祖先提示 -->
        <BranchCompareStatsBar
          v-if="alignment"
          :stats="alignment.stats"
          :truncated="!!alignment.truncated"
          :unrelated="!alignment.mergeBaseHash"
        />
        <!-- 空态 5：对比完成但无独有/配对提交（同内容分支） -->
        <el-alert
          v-if="alignment && !comparing && alignment.rows.length === 0"
          type="success"
          :closable="false"
          show-icon
          title="两个分支没有差异提交（提交内容一致）"
        />

        <div v-loading="comparing" class="bc-main">
          <!-- 左：对齐提交表（R3/R8） -->
          <div class="bc-left">
            <BranchCompareCommitTable
              :rows="alignment?.rows || []"
              :branch-a="branchA"
              :branch-b="branchB"
              @select-commit="onSelectCommit"
            />
          </div>
          <!-- 右：文件树 + diff（diff 区可折叠） -->
          <div class="bc-right">
            <div class="right-toolbar">
              <span class="right-title">分支差异文件</span>
              <el-button size="small" text @click="diffCollapsed = !diffCollapsed">
                {{ diffCollapsed ? '展开差异' : '收起差异' }}
              </el-button>
            </div>
            <BranchCompareFileTree
              class="bc-tree"
              :files="fileTree"
              :selected-path="selectedFile?.path || ''"
              @select-file="onSelectFile"
            />
            <div v-show="!diffCollapsed" class="bc-diff-area">
              <div v-if="diffLoading" class="bc-diff-loading" v-loading="true"></div>
              <BranchCompareDiffViewer
                v-else-if="selectedFile"
                :diff-text="diffText"
                :base-label="branchA"
                :target-label="branchB"
              />
              <div v-else class="bc-diff-placeholder">点击上方文件查看分支差异</div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- 提交详情弹层（R7） -->
    <BranchCompareCommitDetail
      v-model="detailVisible"
      :repo-path="repoPath"
      :commit="detailCommit"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  GetGitInfo,
  GetBranches,
  OpenDirectoryDialog,
  GetBranchCompareAlignment,
  GetBranchCompareFileTree,
  GetBranchCompareFileDiff
} from '../../wailsjs/go/main/App'
import { useDirectoryStore, useUiStore } from '../store'
import { gitCache } from '../utils/gitCache'
import { handleError } from '../utils/error'
import BranchCompareStatsBar from '../components/BranchCompareStatsBar.vue'
import BranchCompareCommitTable from '../components/BranchCompareCommitTable.vue'
import BranchCompareFileTree from '../components/BranchCompareFileTree.vue'
import BranchCompareDiffViewer from '../components/BranchCompareDiffViewer.vue'
import BranchCompareCommitDetail from '../components/BranchCompareCommitDetail.vue'

defineOptions({ name: 'BranchCompareView' })

const directoryStore = useDirectoryStore()
const uiStore = useUiStore()

// ---- 仓库与分支选择态 ----
const repoPath = ref('')
const isRepo = ref(false)
// 浏览按钮手动指定过仓库后不再跟随 currentDirPath 自动重播种
let repoManual = false
const branches = ref([])
const branchesLoading = ref(false)
const branchA = ref('')
const branchB = ref('')

// ---- 对比结果态 ----
const alignment = ref(null)
const fileTree = ref([])
const comparing = ref(false)

// ---- 文件 diff 态（懒加载 + gitCache）----
const selectedFile = ref(null)
const diffLoading = ref(false)
const diffText = ref('')
const diffCollapsed = ref(false)

// ---- 提交详情弹层 ----
const detailVisible = ref(false)
const detailCommit = ref(null)

// requestSeq 请求序号：仅采纳最新一次 compare / 文件 diff 的结果，丢弃并发旧请求返回
// （连点对比/快速切分支/快速切文件时后到旧响应不覆盖新选中态，参照 StatsView 模式）
let compareSeq = 0
let diffSeq = 0

// 默认 B 启发式：优先取远程默认分支（origin/main → origin/master），无可发现则留空引导
const pickDefaultB = () => {
  const names = branches.value.map(b => b.name)
  return names.includes('origin/main') ? 'origin/main'
    : names.includes('origin/master') ? 'origin/master' : ''
}

/**
 * 拉取分支列表并落默认选择（R2）：
 * A = 当前分支（detached 时不自动选），B = origin/main|origin/master（可发现时）。
 * 返回是否成功拿到分支列表。
 */
const loadBranches = async () => {
  branchesLoading.value = true
  try {
    const result = await GetBranches(repoPath.value)
    branches.value = (result && result.branches) || []
    const current = branches.value.find(b => b.isCurrent && !b.isRemote)
    branchA.value = current ? current.name : ''
    branchB.value = pickDefaultB()
    return true
  } catch (e) {
    handleError('加载分支列表失败: ', e)
    branches.value = []
    return false
  } finally {
    branchesLoading.value = false
  }
}

/**
 * 初始化仓库元信息：GetGitInfo 校验是否 git 仓库（R1），
 * 是则拉分支并按需自动对比（仅面板可见时，避免常驻挂载面板白打后端）。
 */
const initRepo = async () => {
  const path = repoPath.value
  if (!path) {
    isRepo.value = false
    branches.value = []
    return
  }
  try {
    const info = await GetGitInfo(path)
    // 仓库可能在等待期间被切换
    if (repoPath.value !== path) return
    isRepo.value = !!(info && info.isRepo)
    if (!isRepo.value) {
      branches.value = []
      alignment.value = null
      fileTree.value = []
      return
    }
    await loadBranches()
    // A/B 均可解析时自动对比；面板不可见时等激活 watcher 触发
    if (branchA.value && branchB.value && uiStore.activePanel === 'branchcompare') {
      runCompare()
    }
  } catch (e) {
    handleError('读取仓库信息失败: ', e)
    isRepo.value = false
  }
}

// 浏览按钮（R1）：原生目录选择 → 校验 git 仓库（非仓库 warning 引导，不落地）
const browseRepo = async () => {
  const dir = await OpenDirectoryDialog('选择 Git 仓库目录')
  if (!dir) return // 用户取消（空串 no-op）
  try {
    const info = await GetGitInfo(dir)
    if (!info || !info.isRepo) {
      ElMessage.warning('所选目录不是 Git 仓库，请重新选择')
      return
    }
    repoManual = true
    repoPath.value = dir
    isRepo.value = true
    // 重置上一仓库的对比结果
    alignment.value = null
    fileTree.value = []
    selectedFile.value = null
    diffText.value = ''
    await loadBranches()
    if (branchA.value && branchB.value) runCompare()
  } catch (e) {
    handleError('读取仓库信息失败: ', e)
  }
}

// 交换 A/B（R2）：交换后重新拉取
const swapBranches = () => {
  const a = branchA.value
  branchA.value = branchB.value
  branchB.value = a
  // 已有结果时立即重取；未对比过则等用户点开始
  if (alignment.value) runCompare()
}

// 对比入口（R2）：同步对齐 + 文件树（并发），文件 diff 懒加载
const runCompare = async () => {
  if (!repoPath.value || !branchA.value || !branchB.value) return
  if (branchA.value === branchB.value) {
    ElMessage.warning('分支 A 与分支 B 相同，请选择不同的分支')
    return
  }
  const seq = ++compareSeq
  comparing.value = true
  // 新一轮对比清空旧文件选中态（文件树即将整体替换）
  selectedFile.value = null
  diffText.value = ''
  try {
    const [align, tree] = await Promise.all([
      GetBranchCompareAlignment(repoPath.value, branchA.value, branchB.value),
      GetBranchCompareFileTree(repoPath.value, branchA.value, branchB.value)
    ])
    if (seq !== compareSeq) return
    alignment.value = align
    fileTree.value = Array.isArray(tree) ? tree : []
  } catch (e) {
    if (seq !== compareSeq) return
    // E_GIT_TIMEOUT / E_GIT_CANCELLED / E_GIT_LOCKED 经 handleError 自动走 warning 分流（R10）
    handleError('分支对比失败: ', e)
    alignment.value = null
    fileTree.value = []
  } finally {
    if (seq === compareSeq) {
      comparing.value = false
    }
  }
}

// 刷新：清本仓库相关 gitCache（分支对比文件 diff 缓存）后重取对齐 + 文件树
const refreshCompare = async () => {
  const prefix = `branchCompare:${repoPath.value}:`
  for (const key of Array.from(gitCache.cache.keys())) {
    if (typeof key === 'string' && key.startsWith(prefix)) {
      gitCache.delete(key)
    }
  }
  if (branchA.value && branchB.value) {
    await runCompare()
  }
}

// 点文件：懒加载分支 diff（gitCache 优先，键含 repo/a/b/file）
const onSelectFile = async (file) => {
  if (!file) return
  selectedFile.value = file
  diffCollapsed.value = false
  const cacheKey = `branchCompare:${repoPath.value}:${branchA.value}:${branchB.value}:${file.path}`
  const cached = gitCache.get(cacheKey)
  if (cached !== null) {
    diffText.value = cached
    return
  }
  const seq = ++diffSeq
  diffLoading.value = true
  try {
    const text = await GetBranchCompareFileDiff(repoPath.value, branchA.value, branchB.value, file.path)
    if (seq !== diffSeq) return
    gitCache.set(cacheKey, text || '')
    diffText.value = text || ''
  } catch (e) {
    if (seq !== diffSeq) return
    handleError('加载文件差异失败: ', e)
    diffText.value = ''
  } finally {
    if (seq === diffSeq) {
      diffLoading.value = false
    }
  }
}

// 点提交行：打开提交详情弹层（R7），取该行有提交的一侧
const onSelectCommit = (row) => {
  const commit = row?.a || row?.b
  if (!commit) return
  detailCommit.value = commit
  detailVisible.value = true
}

// 面板可见时补载：常驻挂载（v-show），首次激活才自动对比，避免启动白打后端
watch(() => uiStore.activePanel, (panel) => {
  if (panel === 'branchcompare' && isRepo.value && branchA.value && branchB.value && !alignment.value && !comparing.value) {
    runCompare()
  }
})

// 工作目录变化跟随（未手动浏览过时重播种）：对齐 StatsView「选中节点变化重载」语义
watch(() => directoryStore.currentDirPath, (path) => {
  if (repoManual) return
  if (path && path !== repoPath.value) {
    repoPath.value = path
    isRepo.value = false
    alignment.value = null
    fileTree.value = []
    initRepo()
  }
})

// 挂载：种子仓库路径为当前工作目录（R1），面板激活前仅做轻量元信息加载
onMounted(() => {
  repoPath.value = directoryStore.currentDirPath
  initRepo()
})
</script>

<style scoped>
/* Home 一级面板根元素 flex 约束链（docs/spec/frontend-visual-conventions.md §Home 一级面板） */
.bc-view {
  flex: 1;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg-primary);
}

.bc-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm, 8px);
  padding: var(--spacing-sm, 8px) var(--spacing-md, 16px);
  border-bottom: 1px solid var(--border-color);
  flex-wrap: wrap;
}

.bc-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: -0.01em;
}

.repo-path {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-tertiary);
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.branch-select {
  width: 200px;
}

.bc-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: var(--spacing-sm, 8px) var(--spacing-md, 16px);
  gap: var(--spacing-sm, 8px);
  overflow: hidden;
}

.bc-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.bc-main {
  flex: 1;
  min-height: 0;
  display: flex;
  gap: var(--spacing-sm, 8px);
}

.bc-left {
  flex: 1 1 52%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.bc-right {
  flex: 1 1 48%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm, 8px);
  overflow: hidden;
}

.right-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.right-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
}

.bc-tree {
  flex: 0 1 45%;
  min-height: 120px;
}

.bc-diff-area {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.bc-diff-loading,
.bc-diff-placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100px;
  border: 1px dashed var(--border-color);
  border-radius: var(--radius-sm, 4px);
  color: var(--text-tertiary);
  font-size: 13px;
}
</style>
