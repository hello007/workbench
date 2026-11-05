<template>
  <!-- 终端搜索条：叠加于终端区右上角（absolute 定位不侵占终端 flex 布局），
       Ctrl+F 经 xterm attachCustomKeyEventHandler 呼出，Esc/关闭按钮关闭并释放 SearchAddon -->
  <div class="terminal-search-bar" data-search-bar>
    <el-icon :size="13" class="search-icon"><Search /></el-icon>
    <input
      ref="inputRef"
      v-model="queryText"
      class="search-input"
      type="text"
      placeholder="搜索终端缓冲"
      spellcheck="false"
      @input="$emit('query', queryText)"
      @keydown.enter.exact.prevent="$emit('next')"
      @keydown.enter.shift.prevent="$emit('prev')"
      @keydown.esc.stop.prevent="$emit('close')"
    />
    <span class="search-count" data-search-count>{{ countText }}</span>
    <!-- 区分大小写 toggle（Aa 语义）：切换后向上通知重搜 -->
    <span
      class="search-btn case-toggle"
      :class="{ 'is-active': caseSensitive }"
      :title="caseSensitive ? '区分大小写：开' : '区分大小写：关'"
      data-action="toggle-case"
      @click="toggleCase"
    >Aa</span>
    <span class="search-btn" title="上一处（Shift+Enter）" data-action="prev" @click="$emit('prev')">
      <svg width="11" height="11" viewBox="0 0 10 10" fill="none">
        <path d="M2 6L5 3L8 6" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </span>
    <span class="search-btn" title="下一处（Enter）" data-action="next" @click="$emit('next')">
      <svg width="11" height="11" viewBox="0 0 10 10" fill="none">
        <path d="M2 4L5 7L8 4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </span>
    <span class="search-btn close-btn" title="关闭（Esc）" data-action="close" @click="$emit('close')">
      <svg width="11" height="11" viewBox="0 0 10 10" fill="none">
        <path d="M2 2L8 8M8 2L2 8" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>
      </svg>
    </span>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Search } from '@element-plus/icons-vue'

const props = defineProps({
  // 匹配计数：resultCount > 0 时展示「当前/总数」（resultIndex 0 基），
  // 无匹配且有关键词时展示「无结果」
  resultIndex: { type: Number, default: -1 },
  resultCount: { type: Number, default: 0 }
})

const emit = defineEmits(['query', 'case', 'next', 'prev', 'close'])

const inputRef = ref(null)
const queryText = ref('')
const caseSensitive = ref(false)

const countText = computed(() => {
  if (props.resultCount > 0 && props.resultIndex >= 0) {
    return `${props.resultIndex + 1}/${props.resultCount}`
  }
  return queryText.value ? '无结果' : ''
})

function toggleCase() {
  caseSensitive.value = !caseSensitive.value
  emit('case', caseSensitive.value)
}

// 挂载即聚焦输入框（呼出后直接输入，省一次点击）
onMounted(() => {
  if (inputRef.value) {
    inputRef.value.focus()
  }
})
</script>

<style scoped>
/* 叠加定位：锚定父容器（.terminal-area，position:relative）右上角，不侵占终端 flex 布局；
   z-index 高于终端容器低于工具箱弹窗体系（面板内局部层，无全局层级竞争） */
.terminal-search-bar {
  position: absolute;
  top: var(--spacing-sm, 8px);
  right: var(--spacing-md, 16px);
  z-index: 20;
  display: flex;
  align-items: center;
  gap: var(--spacing-xs, 4px);
  padding: 4px var(--spacing-sm, 8px);
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md, 8px);
  box-shadow: var(--shadow-md);
}

.search-icon {
  color: var(--text-tertiary);
  flex-shrink: 0;
}

.search-input {
  width: 180px;
  border: none;
  outline: none;
  background: transparent;
  color: var(--text-primary);
  font-size: 12px;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  caret-color: var(--primary-color);
}

.search-input::placeholder {
  color: var(--text-placeholder);
}

.search-count {
  min-width: 36px;
  text-align: center;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  color: var(--text-tertiary);
  white-space: nowrap;
}

/* Aa toggle 与上/下一处/关闭按钮共用键帽风：hover 反馈，active 主色 */
.search-btn {
  height: 22px;
  min-width: 22px;
  padding: 0 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm, 4px);
  color: var(--text-tertiary);
  font-size: 11px;
  font-weight: 500;
  cursor: pointer;
  user-select: none;
  transition: background var(--transition-fast, 0.15s ease), color var(--transition-fast, 0.15s ease);
}

.search-btn:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.case-toggle.is-active {
  background: var(--primary-bg);
  color: var(--primary-color);
}

.close-btn:hover {
  color: var(--text-primary);
}
</style>
