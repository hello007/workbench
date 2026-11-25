<template>
  <div
    class="overview-card"
    :class="{ clickable, highlighted, errored: !!error }"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable ? 0 : undefined"
    @click="onActivate"
    @keydown.enter="onActivate"
  >
    <div class="card-head">
      <span class="card-title">{{ title }}</span>
      <el-icon v-if="clickable" class="card-arrow"><ArrowRight /></el-icon>
    </div>

    <div v-if="error" class="card-error">
      <span class="error-text">{{ error }}</span>
      <el-button link size="small" @click.stop="$emit('retry')">重试</el-button>
    </div>

    <template v-else>
      <div class="card-value">
        <span class="value-num">{{ value }}</span>
        <span v-if="unit" class="value-unit">{{ unit }}</span>
      </div>
      <div v-if="hint" class="card-hint">{{ hint }}</div>
      <slot />
    </template>
  </div>
</template>

<script setup>
import { ArrowRight } from '@element-plus/icons-vue'

/**
 * 概览卡片通用外壳：标题 + 主数值 + 提示 + 错误降级。
 *
 * 只负责展示与点击语义，不感知业务 store；具体数值由五个业务卡片计算后传入。
 * error 非空时只渲染降级提示，主数值不展示——避免把「查询失败」伪装成 0。
 */
const props = defineProps({
  title: { type: String, required: true },
  value: { type: [String, Number], default: '' },
  unit: { type: String, default: '' },
  hint: { type: String, default: '' },
  error: { type: String, default: '' },
  highlighted: { type: Boolean, default: false },
  clickable: { type: Boolean, default: true }
})

const emit = defineEmits(['drill', 'retry'])

function onActivate() {
  if (!props.clickable) return
  emit('drill')
}
</script>

<style scoped>
.overview-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  min-height: 132px;
  padding: var(--spacing-md);
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.overview-card.clickable {
  cursor: pointer;
}

/* 悬停按视觉规范用主色浅一档描边 + 阴影层级，禁自定义 rgba 着色 */
.overview-card.clickable:hover {
  border-color: var(--primary-light);
  box-shadow: var(--shadow-md);
}

.overview-card.clickable:focus-visible {
  outline: 2px solid var(--primary-color);
  outline-offset: 1px;
}

.overview-card.highlighted {
  border-color: var(--warning-color);
}

.overview-card.errored {
  border-color: var(--danger-color);
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
}

.card-arrow {
  font-size: 12px;
  color: var(--text-tertiary);
}

.card-value {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-xs);
}

.value-num {
  font-size: 28px;
  font-weight: 600;
  line-height: 1.1;
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

.value-unit {
  font-size: 13px;
  color: var(--text-tertiary);
}

.card-hint {
  font-size: 12px;
  color: var(--text-tertiary);
  line-height: 1.5;
}

.card-error {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-size: 12px;
  color: var(--danger-color);
}

.error-text {
  flex: 1;
  line-height: 1.5;
  word-break: break-word;
}
</style>
