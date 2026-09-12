<template>
  <el-card class="git-tags-card" shadow="hover">
    <template #header>
      <div class="card-header">
        <span>Git 标签</span>
        <div class="header-actions">
          <el-button size="small" type="primary" @click="openCreateDialog">新建标签</el-button>
          <el-button
            size="small"
            @click="pushAllTags"
            :loading="pushingAll"
            :disabled="tags.length === 0"
          >
            推送全部标签
          </el-button>
          <el-button
            :icon="Refresh"
            circle
            size="small"
            @click="loadTags"
            :loading="loading"
          />
        </div>
      </div>
    </template>

    <div v-loading="loading" class="tags-container">
      <div v-if="tags.length > 0" class="tag-list">
        <div v-for="tag in tags" :key="tag.name" class="tag-row">
          <div class="tag-main">
            <el-text class="tag-name" :title="tag.name">{{ tag.name }}</el-text>
            <el-tag size="small" :type="tag.type === 'annotated' ? 'warning' : 'info'">
              {{ tag.type === 'annotated' ? '注释' : '轻量' }}
            </el-tag>
            <el-text class="tag-sha" :title="tag.sha" @click="copyText(tag.sha)">
              {{ tag.shortSha }}
            </el-text>
            <span v-if="tag.tagger" class="tag-tagger">{{ tag.tagger }}</span>
            <span v-if="tag.date" class="tag-date">{{ tag.date }}</span>
          </div>
          <div class="tag-message" v-if="tag.message">{{ tag.message }}</div>
          <div class="tag-actions">
            <el-button
              size="small"
              text
              @click="pushTag(tag)"
              :loading="pushingName === tag.name"
            >推送</el-button>
            <el-button
              size="small"
              text
              type="danger"
              @click="deleteTag(tag)"
              :loading="deletingName === tag.name"
            >删除</el-button>
          </div>
        </div>
      </div>

      <el-empty v-else-if="!loading" description="暂无标签" />
    </div>

    <!-- 新建标签对话框 -->
    <el-dialog
      v-model="createVisible"
      title="新建标签"
      width="420px"
      :close-on-click-modal="false"
    >
      <el-form label-position="top">
        <el-form-item label="标签名">
          <el-input v-model="createForm.name" placeholder="如 v1.0.0" />
        </el-form-item>
        <el-form-item label="注释消息（可选，留空创建轻量标签）">
          <el-input
            v-model="createForm.message"
            type="textarea"
            :rows="3"
            placeholder="填写后创建注释标签（-a -m）"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" @click="submitCreate" :loading="creating">创建</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { GetTags, CreateTag, DeleteTag, PushTag } from '../../wailsjs/go/main/App'
import { handleGitError } from '../utils/gitError'

const props = defineProps({
  repoPath: { type: String, required: true }
})

const tags = ref([])
const loading = ref(false)
const createVisible = ref(false)
const createForm = ref({ name: '', message: '' })
const creating = ref(false)
const deletingName = ref('')
const pushingName = ref('')
const pushingAll = ref(false)

const loadTags = async () => {
  loading.value = true
  try {
    tags.value = await GetTags(props.repoPath) || []
  } catch (error) {
    ElMessage.error('加载标签列表失败: ' + (error.message || String(error)))
  } finally {
    loading.value = false
  }
}

const openCreateDialog = () => {
  createForm.value = { name: '', message: '' }
  createVisible.value = true
}

const submitCreate = async () => {
  const name = createForm.value.name.trim()
  if (!name) {
    ElMessage.warning('标签名不能为空')
    return
  }
  creating.value = true
  try {
    await CreateTag(props.repoPath, name, createForm.value.message)
    ElMessage.success('标签创建成功')
    createVisible.value = false
    await loadTags()
  } catch (error) {
    handleGitError('创建标签失败: ', error)
  } finally {
    creating.value = false
  }
}

const deleteTag = async (tag) => {
  try {
    await ElMessageBox.confirm(`确定删除标签「${tag.name}」？`, '删除标签', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch {
    return // 用户取消
  }
  deletingName.value = tag.name
  try {
    await DeleteTag(props.repoPath, tag.name)
    ElMessage.success('标签已删除')
    await loadTags()
  } catch (error) {
    handleGitError('删除标签失败: ', error)
  } finally {
    deletingName.value = ''
  }
}

const pushTag = async (tag) => {
  pushingName.value = tag.name
  try {
    const out = await PushTag(props.repoPath, tag.name)
    ElMessage.success('标签推送成功' + (out ? `\n${out}` : ''))
  } catch (error) {
    handleGitError('推送标签失败: ', error)
  } finally {
    pushingName.value = ''
  }
}

// 推送全部标签：复用 PushTag 逐个推送（遵循 9 方法约束，未新增后端方法）。
// 逐个推送便于定位失败标签，汇总成功/失败计数。
const pushAllTags = async () => {
  if (tags.value.length === 0) return
  pushingAll.value = true
  let ok = 0
  let fail = 0
  for (const tag of tags.value) {
    try {
      await PushTag(props.repoPath, tag.name)
      ok++
    } catch {
      fail++
    }
  }
  pushingAll.value = false
  if (fail === 0) {
    ElMessage.success(`已推送 ${ok} 个标签`)
  } else {
    ElMessage.warning(`推送完成：成功 ${ok} 个，失败 ${fail} 个`)
  }
}

const copyText = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

watch(() => props.repoPath, () => {
  tags.value = []
  loadTags()
})

loadTags()

defineExpose({ loadTags })
</script>

<style scoped>
.git-tags-card {
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
.tags-container {
  min-height: 60px;
}
.tag-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}
.tag-row {
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--border-color);
  border-left: 3px solid var(--border-light);
  border-radius: var(--radius-md);
  background: var(--bg-secondary);
  transition: all var(--transition-fast);
}
.tag-row:hover {
  box-shadow: var(--shadow-md);
  border-left-color: var(--primary-color);
}
.tag-main {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}
.tag-name {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  font-weight: 600;
  color: var(--primary-color);
}
.tag-sha {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  color: var(--text-secondary);
  cursor: pointer;
}
.tag-sha:hover {
  text-decoration: underline;
}
.tag-tagger,
.tag-date {
  font-size: 12px;
  color: var(--text-tertiary);
}
.tag-message {
  margin-top: 6px;
  font-size: 13px;
  color: var(--text-primary);
  word-break: break-word;
  white-space: pre-wrap;
}
.tag-actions {
  margin-top: 6px;
  display: flex;
  gap: var(--spacing-xs);
  justify-content: flex-end;
}
</style>
