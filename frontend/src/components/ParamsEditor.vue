<template>
  <div class="params-editor">
    <el-form label-width="92px" size="small" @submit.prevent>
      <el-form-item label="参数类型">
        <el-select :model-value="type" style="width: 200px" @update:model-value="onTypeChange">
          <el-option label="无参数（none）" value="none" />
          <el-option label="文件选择（file）" value="file" />
          <el-option label="单行文本（text）" value="text" />
          <el-option label="字段表单（form）" value="form" />
        </el-select>
      </el-form-item>

      <!-- file：选一个文件作为输入源 -->
      <template v-if="type === 'file'">
        <el-form-item label="标题">
          <el-input v-model="model.label" placeholder="如：选择源文档" />
        </el-form-item>
        <el-form-item label="参数键名">
          <el-input v-model="model.textFieldKey" placeholder="默认 file" />
        </el-form-item>
        <el-form-item label="起始目录">
          <el-input v-model="model.startDir" placeholder="文件选择起始路径" />
        </el-form-item>
        <el-form-item label="扩展名">
          <el-input v-model="extensionsText" placeholder="逗号分隔，如 .md, .docx" />
        </el-form-item>
      </template>

      <!-- text：单行文本追加到命令尾部 -->
      <template v-else-if="type === 'text'">
        <el-form-item label="标题">
          <el-input v-model="model.label" placeholder="如：要取消的会议" />
        </el-form-item>
        <el-form-item label="参数键名">
          <el-input v-model="model.textFieldKey" placeholder="默认 text" />
        </el-form-item>
      </template>

      <!-- form：字段化表单，渲染 PromptTemplate 模板 -->
      <template v-else-if="type === 'form'">
        <el-form-item label="标题">
          <el-input v-model="model.label" placeholder="如：会议信息" />
        </el-form-item>
        <el-form-item label="模板">
          <el-input
            v-model="model.promptTemplate"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 10 }"
            placeholder="prompt 模板，{{key}} 占位替换为字段值"
          />
        </el-form-item>
        <el-form-item label="字段列表">
          <div class="fields-list">
            <div v-for="(f, i) in model.fields || []" :key="i" class="field-row">
              <el-input v-model="f.key" placeholder="key" class="col-key" />
              <el-input v-model="f.label" placeholder="标签" class="col-label" />
              <el-select v-model="f.type" class="col-type">
                <el-option label="文本" value="text" />
                <el-option label="日期时间" value="datetime" />
                <el-option label="数字" value="number" />
              </el-select>
              <el-input v-model="f.placeholder" placeholder="提示" class="col-ph" />
              <el-checkbox v-model="f.required">必填</el-checkbox>
              <el-button type="danger" link @click="removeField(i)">删除</el-button>
            </div>
            <el-button size="small" @click="addField">+ 新增字段</el-button>
          </div>
        </el-form-item>
        <!-- form 模式占位符一致性告警（不阻断保存，允许用户先存后调） -->
        <div v-if="placeholderWarnings.length" class="warn-box">
          <div v-for="w in placeholderWarnings" :key="w" class="warn-item">⚠ {{ w }}</div>
        </div>
      </template>

      <div v-else class="none-hint">该功能无需参数，点击运行直接执行。</div>
    </el-form>
  </div>
</template>

<script setup>
import { computed } from 'vue'

// v-model 绑 AiParamSpec 对象（params 或 followUp.input）；父传入 reactive 对象，
// 子组件直接改其属性触发响应式，整体替换（如 type 切换、null→新建）经 model.value = 走 emit
const model = defineModel({ type: Object, default: null })

const type = computed(() => model.value?.type || 'none')

// type 切换：null 时新建对象，已有则保留其他字段只更 type（不丢用户已录数据）
const onTypeChange = (v) => {
  if (!model.value) {
    model.value = { type: v }
    return
  }
  model.value = { ...model.value, type: v }
}

// extensions 数组与逗号分隔文本互转（file 模式 UI 用文本输入，model 存数组）
const extensionsText = computed({
  get: () => (model.value?.extensions || []).join(', '),
  set: (v) => {
    if (!model.value) return
    model.value = {
      ...model.value,
      extensions: v.split(/[,，]/).map((s) => s.trim()).filter(Boolean)
    }
  }
})

const addField = () => {
  if (!model.value) return
  const fields = model.value.fields || []
  model.value = {
    ...model.value,
    fields: [...fields, { key: '', label: '', type: 'text', placeholder: '', required: false }]
  }
}
const removeField = (i) => {
  if (!model.value?.fields) return
  const fields = model.value.fields.slice()
  fields.splice(i, 1)
  model.value = { ...model.value, fields }
}

// form 模式 promptTemplate 的 {{key}} 与 fields.key 一致性校验（告警不阻断）：
// 模板出现的占位符无对应 field 定义、field 的 key 未在模板出现，均提示用户核对
const placeholderWarnings = computed(() => {
  if (!model.value || model.value.type !== 'form') return []
  const template = model.value.promptTemplate || ''
  const fields = model.value.fields || []
  const usedKeys = new Set([...template.matchAll(/\{\{(\w+)\}\}/g)].map((m) => m[1]))
  const definedKeys = new Set(fields.map((f) => f.key).filter(Boolean))
  const warns = []
  for (const k of usedKeys) {
    if (!definedKeys.has(k)) warns.push(`模板占位符 {{${k}}} 未在字段列表中定义`)
  }
  for (const k of definedKeys) {
    if (!usedKeys.has(k)) warns.push(`字段「${k}」未在模板中使用`)
  }
  return warns
})

// 暴露告警给父组件，供保存时收集字段级问题（占位符不一致不阻断，仅提示）
defineExpose({ placeholderWarnings })
</script>

<style scoped>
.params-editor :deep(.el-form-item) {
  margin-bottom: 12px;
}
.fields-list {
  width: 100%;
}
.field-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.col-key {
  width: 120px;
}
.col-label {
  width: 140px;
}
.col-type {
  width: 110px;
}
.col-ph {
  flex: 1;
  min-width: 120px;
}
.none-hint {
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  padding: 8px 12px;
}
.warn-box {
  margin: 4px 0 8px 92px;
  padding: 6px 10px;
  background: rgba(230, 162, 60, 0.12);
  border-radius: var(--radius-sm);
  font-size: 12px;
  color: var(--warning-color, #e6a23c);
}
.warn-item {
  line-height: 1.6;
}
</style>
