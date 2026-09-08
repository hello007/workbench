<template>
  <div class="env-editor">
    <!-- 键值对行：键可改（删旧加新），值支持 $ENV:VAR 引用提示 -->
    <div v-for="(row, i) in rows" :key="i" class="env-row">
      <el-input
        v-model="row.key"
        placeholder="变量名"
        class="col-key"
        @change="syncToModel"
      />
      <el-input
        v-model="row.value"
        placeholder="值，支持 $ENV:VAR 引用宿主环境变量"
        class="col-val"
        @change="syncToModel"
      />
      <el-button type="danger" link @click="removeRow(i)">删除</el-button>
    </div>
    <el-button size="small" @click="addRow">+ 新增变量</el-button>
    <div class="env-hint">
      值以 <code>$ENV:VAR</code> 开头时，运行时展开为宿主环境变量（token 等凭证不落配置文件）。
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'

// v-model 绑 map[string]string（env 或 mcp stdio env）。map 键可改，用行数组中间层：
// rows 镜像 model 的 entries 供模板 v-model 直接绑行对象属性（避开 reactive map 下标绑定），
// 行变更手动 syncToModel 回写 map（删旧键加新键支持键重命名）
const model = defineModel({ type: Object, default: null })

const rows = ref([])

// model → rows 同步：父切换功能项改 model 时重建 rows
const syncFromModel = () => {
  const m = model.value || {}
  rows.value = Object.keys(m).map((k) => ({ key: k, value: m[k] }))
}

// rows → model 回写：以 rows 为准重建 map（键可能被改，用最新 rows）
const syncToModel = () => {
  if (!model.value) model.value = {}
  const next = {}
  for (const r of rows.value) {
    const k = (r.key || '').trim()
    if (k) next[k] = r.value || ''
  }
  // 清空旧键后写入新键（处理键重命名/删除）
  for (const k of Object.keys(model.value)) delete model.value[k]
  Object.assign(model.value, next)
}

// rows 是否已与 model 一致：syncToModel 后 watch model 触发，但内容一致则跳过重建，避免重渲丢焦点
const rowsMatchModel = () => {
  const m = model.value || {}
  const mkeys = Object.keys(m)
  if (mkeys.length !== rows.value.length) return false
  for (const r of rows.value) {
    if (m[r.key] !== r.value) return false
  }
  return true
}

watch(
  () => model.value,
  () => {
    if (!rowsMatchModel()) syncFromModel()
  },
  { immediate: true, deep: true }
)

const addRow = () => {
  rows.value.push({ key: '', value: '' })
}
const removeRow = (i) => {
  rows.value.splice(i, 1)
  syncToModel()
}
</script>

<style scoped>
.env-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}
.col-key {
  width: 160px;
}
.col-val {
  flex: 1;
}
.env-hint {
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.6;
}
.env-hint code {
  font-family: Consolas, 'Cascadia Code', 'Courier New', monospace;
  background: var(--bg-tertiary);
  padding: 1px 4px;
  border-radius: 3px;
}
</style>
