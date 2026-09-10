<template>
  <div class="followups-editor">
    <!-- followUps 动态列表：每项一个卡片，标题显示 label 或 (未命名) -->
    <div v-for="(fu, i) in model" :key="fu.id || 'idx_' + i" class="fu-card">
      <div class="fu-head">
        <span class="fu-title">{{ fu.label || '(未命名)' }}</span>
        <div class="fu-actions">
          <el-button link :disabled="i === 0" @click="moveUp(i)">上移</el-button>
          <el-button link :disabled="i === model.length - 1" @click="moveDown(i)">下移</el-button>
          <el-button type="danger" link @click="remove(i)">删除</el-button>
        </div>
      </div>

      <el-form label-width="92px" size="small" @submit.prevent>
        <el-form-item label="按钮文案">
          <el-input v-model="fu.label" placeholder="如：确认落盘" />
        </el-form-item>
        <el-form-item label="prompt 模板">
          <el-input
            v-model="fu.promptTemplate"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 8 }"
            placeholder="点击后发送的 prompt，{{key}} 占位替换为 input 字段值；无 input 时直接发送"
          />
        </el-form-item>
        <el-form-item label="需要输入">
          <el-switch
            :model-value="fu.input !== null"
            @update:model-value="(v) => onInputToggle(fu, v)"
          />
          <span class="switch-hint">{{ fu.input !== null ? '点击前需录入参数' : '点击直接发送' }}</span>
        </el-form-item>
        <!-- input 嵌套 ParamsEditor：null 表示直接发送，非 null 时配置输入规格 -->
        <el-form-item v-if="fu.input !== null" label="输入配置">
          <ParamsEditor v-model="fu.input" />
        </el-form-item>
      </el-form>
    </div>

    <el-button size="small" @click="add">+ 新增后续段</el-button>

    <!-- 字段级校验告警：label 不能为空、form input 须有 promptTemplate -->
    <div v-if="validationErrors.length" class="warn-box">
      <div v-for="e in validationErrors" :key="e" class="warn-item">⚠ {{ e }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import ParamsEditor from './ParamsEditor.vue'

// v-model 绑 AiFollowUp[]。父传入 reactive 数组，子组件改元素属性 / 增删经原数组 reactive 触发更新
const model = defineModel({ type: Array, default: () => [] })

const add = () => {
  if (!model.value) model.value = []
  model.value.push({
    id: 'fu_' + Date.now().toString(36),
    label: '',
    promptTemplate: '',
    input: null
  })
}

const remove = (i) => {
  model.value.splice(i, 1)
}

// 排序：交换数组元素引用，:key 跟随 fu.id，Vue 按 key 移动 DOM 而非原地复用
const moveUp = (i) => {
  if (i <= 0) return
  const arr = model.value
  const tmp = arr[i - 1]
  arr[i - 1] = arr[i]
  arr[i] = tmp
}

const moveDown = (i) => {
  const arr = model.value
  if (i >= arr.length - 1) return
  const tmp = arr[i + 1]
  arr[i + 1] = arr[i]
  arr[i] = tmp
}

const onInputToggle = (fu, on) => {
  // 开启时给默认 text 输入规格；关闭时置 null（直接发送）
  fu.input = on ? { type: 'text', label: '输入', textFieldKey: 'input' } : null
}

// 字段级校验：label 不能为空、form input 须有 promptTemplate
const validationErrors = computed(() => {
  const errs = []
  const list = model.value || []
  for (let i = 0; i < list.length; i++) {
    const fu = list[i]
    if (!fu.label?.trim()) errs.push(`followUps[${i}].label 不能为空`)
    if (fu.input && fu.input.type === 'form' && !fu.input.promptTemplate?.trim()) {
      errs.push(`followUps[${i}].input 缺少 promptTemplate`)
    }
  }
  return errs
})

defineExpose({ validationErrors })
</script>

<style scoped>
.fu-card {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  padding: 10px;
  margin-bottom: 10px;
}
.fu-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--border-color);
}
.fu-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
}
.fu-actions {
  display: flex;
  gap: 4px;
}
.switch-hint {
  margin-left: 8px;
  font-size: 12px;
  color: var(--text-tertiary);
}
.warn-box {
  margin-top: 8px;
  padding: 6px 10px;
  background: rgba(245, 108, 108, 0.12);
  border-radius: var(--radius-sm);
  font-size: 12px;
  color: var(--danger-color, #f56c6c);
}
.warn-item {
  line-height: 1.6;
}
</style>
