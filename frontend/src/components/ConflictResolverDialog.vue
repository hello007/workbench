<template>
  <el-dialog
    :model-value="visible"
    title="内嵌解决冲突"
    width="min(900px, 90vw)"
    top="10vh"
    append-to-body
    @update:model-value="(v) => emit('update:visible', v)"
  >
    <!-- rebase 语义反转提示：ours = 变基目标侧，容易选错，入口先提示 -->
    <el-alert
      v-if="conflictType === 'rebase'"
      type="warning"
      :closable="false"
      show-icon
      class="rebase-alert"
      title="变基冲突中「当前分支」指变基目标侧（git ours 语义反转），请谨慎选边"
    />

    <div v-if="loading" class="resolver-tip">加载冲突文件内容中...</div>

    <template v-else>
      <!-- 二进制 / 超大文件：不进编辑器，引导走选边或外部工具（R4） -->
      <el-empty
        v-if="binary"
        description="二进制冲突文件不支持内嵌编辑，请用「以当前为准 / 以传入为准」或外部合并工具"
      />
      <el-empty
        v-else-if="tooLarge"
        description="文件过大（超过 1MB），不支持内嵌编辑，请用「以当前为准 / 以传入为准」或外部合并工具"
      />

      <div v-else-if="loadError" class="resolver-tip resolver-error">{{ loadError }}</div>

      <template v-else>
        <!-- 无标记文件（R3）：外部已改干净，直接全文编辑 -->
        <el-alert
          v-if="!hasMarkers"
          type="info"
          :closable="false"
          show-icon
          title="文件中未检测到冲突标记（可能已被外部修改），直接进入全文编辑，保存即标记已解决"
        />

        <!-- 块列表视图（有标记时展示） -->
        <div v-if="hasMarkers" class="block-list">
          <div
            v-for="(block, i) in parsed.blocks"
            :key="i"
            class="conflict-block"
            :class="{ 'is-resolved': !!block.resolved }"
          >
            <div class="block-header">
              <span class="block-title">冲突块 {{ i + 1 }}/{{ parsed.blocks.length }}</span>
              <el-tag v-if="block.resolved" size="small" type="success">
                {{ resolvedLabel(block.resolved.kind) }}
              </el-tag>
              <el-button
                v-if="block.resolved"
                size="small" text
                @click="undoResolve(i)"
              >撤销决策</el-button>
            </div>
            <div class="block-panes">
              <div class="block-pane">
                <div class="pane-label">当前分支（ours）</div>
                <pre class="pane-code">{{ block.ours.join('\n') || '（空）' }}</pre>
              </div>
              <div class="block-pane">
                <div class="pane-label">传入分支（theirs）</div>
                <pre class="pane-code">{{ block.theirs.join('\n') || '（空）' }}</pre>
              </div>
              <!-- diff3 base 段（merge.conflictStyle=diff3 时出现）：展示但默认不参与合并结果 -->
              <div v-if="block.base.length > 0" class="block-pane">
                <div class="pane-label">共同祖先（base，不采用）</div>
                <pre class="pane-code">{{ block.base.join('\n') }}</pre>
              </div>
            </div>
            <div class="block-actions">
              <el-button size="small" @click="resolveBlock(i, 'ours')">用当前</el-button>
              <el-button size="small" @click="resolveBlock(i, 'theirs')">用传入</el-button>
              <el-button size="small" @click="resolveBlock(i, 'both')">两者都保留</el-button>
              <el-button size="small" @click="toggleBlockEdit(i)">
                {{ blockEditing === i ? '收起编辑' : '手动编辑' }}
              </el-button>
            </div>
            <!-- 块内手动编辑：以当前合并行（决策或 ours+theirs）为初稿 -->
            <el-input
              v-if="blockEditing === i"
              type="textarea"
              :rows="4"
              :model-value="blockDraft"
              class="block-textarea"
              @update:model-value="(v) => (blockDraft = v)"
            />
            <div v-if="blockEditing === i" class="block-edit-actions">
              <el-button size="small" @click="blockEditing = -1">取消</el-button>
              <el-button size="small" type="primary" @click="applyBlockEdit(i)">应用编辑</el-button>
            </div>
          </div>
        </div>

        <!-- 合并预览（可折叠，实时更新） -->
        <el-collapse v-if="!fullEditing" class="preview-collapse">
          <el-collapse-item title="合并预览" name="preview">
            <pre class="preview-code">{{ mergedPreview }}</pre>
          </el-collapse-item>
        </el-collapse>

        <!-- 全文编辑模式 -->
        <div v-if="fullEditing" class="full-edit">
          <el-input
            type="textarea"
            :rows="14"
            :model-value="fullText"
            class="full-textarea"
            @update:model-value="(v) => (fullText = v)"
          />
        </div>
        <div class="mode-actions">
          <el-button size="small" @click="toggleFullEditing">
            {{ fullEditing ? '返回块视图' : '全文编辑' }}
          </el-button>
          <el-text size="small" type="info">
            保存将按原编码（{{ encoding || 'UTF-8' }}）写回并标记已解决
          </el-text>
        </div>
      </template>
    </template>

    <template #footer>
      <el-button @click="emit('update:visible', false)">关闭</el-button>
      <el-button
        v-if="!loading && !binary && !tooLarge && !loadError"
        type="primary"
        :loading="saving"
        @click="saveResolution"
      >保存并标记已解决</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  GetConflictFileContent, SaveConflictResolution
} from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import {
  parseConflictMarkers, mergeConflictResult, containsConflictMarkers
} from '../utils/conflictMarkers'

const props = defineProps({
  visible: { type: Boolean, default: false },
  repoPath: { type: String, required: true },
  file: { type: String, required: true },
  conflictType: { type: String, default: 'none' }
})

const emit = defineEmits(['update:visible', 'resolved'])

const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const binary = ref(false)
const tooLarge = ref(false)
// 原文件编码（UTF-8/GBK），保存时原样回传，不改变编码（R5）
const encoding = ref('')
const parsed = ref({ before: [], blocks: [], after: [], hasMarkers: false })

// 全文编辑模式（无标记文件 R3 直进此模式）
const fullEditing = ref(false)
const fullText = ref('')

// 块内手动编辑状态：正在编辑的块下标 + 编辑草稿
const blockEditing = ref(-1)
const blockDraft = ref('')

// 请求序号守卫：快速关闭/重开时丢弃迟到的旧响应
let requestSeq = 0

const hasMarkers = computed(() => parsed.value.hasMarkers && parsed.value.blocks.length > 0)

// 合并预览：全文编辑模式下显示编辑内容，块视图下实时拼接
const mergedPreview = computed(() => {
  if (fullEditing.value) return fullText.value
  return mergeConflictResult(parsed.value)
})

const loadContent = async () => {
  if (!props.visible) return
  const seq = ++requestSeq
  loading.value = true
  loadError.value = ''
  binary.value = false
  tooLarge.value = false
  fullEditing.value = false
  blockEditing.value = -1
  try {
    const result = await GetConflictFileContent(props.repoPath, props.file)
    if (seq !== requestSeq) return
    encoding.value = result.encoding || ''
    if (result.binary) {
      binary.value = true
      return
    }
    if (result.tooLarge) {
      tooLarge.value = true
      return
    }
    parsed.value = parseConflictMarkers(result.content || '')
    fullText.value = result.content || ''
    // R3：无标记文件直接进全文编辑模式
    if (!parsed.value.hasMarkers) {
      fullEditing.value = true
    }
  } catch (error) {
    if (seq !== requestSeq) return
    loadError.value = error?.message || String(error)
    handleError('加载冲突文件失败: ', error)
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

const resolvedLabel = (kind) => {
  switch (kind) {
    case 'ours': return '已用当前'
    case 'theirs': return '已用传入'
    case 'both': return '已保留两者'
    default: return '已手动编辑'
  }
}

// 块决策：ours/theirs/both
const resolveBlock = (index, kind) => {
  const block = parsed.value.blocks[index]
  const lines =
    kind === 'ours' ? [...block.ours]
      : kind === 'theirs' ? [...block.theirs]
        : [...block.ours, ...block.theirs]
  block.resolved = { kind, lines }
  // 决策后收起该块的手动编辑
  if (blockEditing.value === index) blockEditing.value = -1
}

const undoResolve = (index) => {
  parsed.value.blocks[index].resolved = null
}

// 块内手动编辑：草稿初值 = 当前合并行
const toggleBlockEdit = (index) => {
  if (blockEditing.value === index) {
    blockEditing.value = -1
    return
  }
  const block = parsed.value.blocks[index]
  blockDraft.value = block.resolved
    ? block.resolved.lines.join('\n')
    : [...block.ours, ...block.theirs].join('\n')
  blockEditing.value = index
}

const applyBlockEdit = (index) => {
  const lines = blockDraft.value.split('\n')
  parsed.value.blocks[index].resolved = { kind: 'custom', lines }
  blockEditing.value = -1
}

// 全文编辑切换：进入时以最新合并结果为初稿
const toggleFullEditing = () => {
  if (!fullEditing.value) {
    fullText.value = mergeConflictResult(parsed.value)
  }
  fullEditing.value = !fullEditing.value
}

// 保存并标记已解决：仍含冲突标记时软确认（可强制保存）
const saveResolution = async () => {
  const content = fullEditing.value ? fullText.value : mergeConflictResult(parsed.value)
  if (containsConflictMarkers(content)) {
    try {
      await ElMessageBox.confirm(
        '合并结果中仍包含冲突标记，确认要强制保存吗？',
        '仍有未解决冲突块',
        { confirmButtonText: '强制保存', cancelButtonText: '返回修改', type: 'warning' }
      )
    } catch {
      return
    }
  }
  saving.value = true
  try {
    await SaveConflictResolution(props.repoPath, props.file, content, encoding.value)
    ElMessage.success(`已保存并标记 ${props.file} 已解决`)
    emit('resolved')
    emit('update:visible', false)
  } catch (error) {
    handleError('保存冲突解决失败: ', error)
  } finally {
    saving.value = false
  }
}

// 打开时加载内容，关闭时重置状态（下次打开重新加载）；immediate 兼容初始即可见挂载
watch(() => props.visible, (v) => {
  if (v) {
    loadContent()
  } else {
    requestSeq++
    blockEditing.value = -1
    fullEditing.value = false
  }
}, { immediate: true })
</script>

<style scoped>
.rebase-alert {
  margin-bottom: var(--spacing-md);
}
.resolver-tip {
  padding: var(--spacing-lg);
  text-align: center;
  color: var(--text-tertiary);
}
.resolver-error {
  color: var(--danger-color);
}
.block-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-md);
}
.conflict-block {
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--el-color-warning);
  border-radius: var(--radius-md);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--bg-secondary);
}
.conflict-block.is-resolved {
  border-left-color: var(--success-color);
}
.block-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-sm);
}
.block-title {
  font-weight: 600;
  color: var(--text-primary);
  font-size: 13px;
}
.block-panes {
  display: flex;
  gap: var(--spacing-sm);
}
.block-pane {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  background: var(--bg-tertiary);
  overflow: hidden;
}
.pane-label {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: 12px;
  color: var(--text-tertiary);
  border-bottom: 1px solid var(--border-color);
}
.pane-code {
  margin: 0;
  padding: var(--spacing-sm);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 160px;
  overflow-y: auto;
}
.block-actions {
  display: flex;
  gap: var(--spacing-xs);
  margin-top: var(--spacing-sm);
  flex-wrap: wrap;
}
.block-textarea {
  margin-top: var(--spacing-sm);
  font-family: Consolas, 'Courier New', monospace;
}
.block-edit-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-xs);
  margin-top: var(--spacing-xs);
}
.preview-collapse {
  margin-bottom: var(--spacing-sm);
}
.preview-code {
  margin: 0;
  padding: var(--spacing-sm);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 220px;
  overflow-y: auto;
}
.full-edit {
  margin-bottom: var(--spacing-sm);
}
.full-textarea {
  font-family: Consolas, 'Courier New', monospace;
}
.mode-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}
</style>
