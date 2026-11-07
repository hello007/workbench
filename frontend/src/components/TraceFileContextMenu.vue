<template>
  <!-- 文件追溯右键菜单（提交历史变更文件 / 文件 diff 文件名共用）：查看文件历史 + 行归属。
       样式复用全局 .context-menu（position:fixed + z-index:2000），追加 .trace-context-menu
       作为自身测量与样式作用域锚（禁用 document.querySelector('.context-menu') 全局查——
       页面可能同时存在多处同类菜单，查询会误命中他菜单）。 -->
  <ul
    v-if="state.visible"
    ref="menuRef"
    class="context-menu trace-context-menu"
    :style="{ left: state.x + 'px', top: state.y + 'px' }"
    @click.stop
    @mousedown.stop
    @contextmenu.prevent.stop
  >
    <li class="context-menu-item" data-action="trace-file-history" @click="emitCommand('history')">
      <el-icon><Clock /></el-icon>查看文件历史
    </li>
    <li
      v-if="state.blameAvailable"
      class="context-menu-item"
      data-action="trace-file-blame"
      @click="emitCommand('blame')"
    >
      <el-icon><Location /></el-icon>行归属
    </li>
  </ul>
</template>

<script setup>
import { reactive, ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Clock, Location } from '@element-plus/icons-vue'

const emit = defineEmits(['command'])

const state = reactive({
  visible: false,
  x: 0,
  y: 0,
  blameAvailable: true
})

const menuRef = ref(null)

/**
 * 打开菜单（调用方在目标元素 @contextmenu.prevent 中触发）。
 * @param {MouseEvent} event 原生右键事件（取 clientX/clientY 定位）
 * @param {object} opts { blameAvailable: boolean } — 行归属菜单项显隐
 *   （JSON 系/markdown/HTML 等无行对齐语义的文件类型只保留「查看文件历史」）
 */
const open = (event, opts = {}) => {
  state.x = event.clientX
  state.y = event.clientY
  state.blameAvailable = opts.blameAvailable !== false
  state.visible = true
  // 菜单渲染后测量并做视口边界回退（对齐 FilePreviewRenderer 先例）
  nextTick(() => {
    const el = menuRef.value
    if (!el) return
    const rect = el.getBoundingClientRect()
    let ax = state.x
    let ay = state.y
    if (ax + rect.width > window.innerWidth) ax = window.innerWidth - rect.width - 5
    if (ay + rect.height > window.innerHeight) ay = window.innerHeight - rect.height - 5
    if (ax < 5) ax = 5
    if (ay < 5) ay = 5
    if (ax !== state.x || ay !== state.y) {
      state.x = ax
      state.y = ay
    }
  })
}

const close = () => {
  state.visible = false
}

const emitCommand = (command) => {
  close()
  emit('command', command)
}

// 全局点击/右键关闭。contextmenu 用 capture 阶段：其他组件右键即便 stopPropagation
// 也能关闭本菜单实现互斥（对齐 FilePreviewRenderer 先例）。菜单根元素的 .stop 只能
// 阻断 target 冒泡阶段、阻不住 document capture 先触发——菜单内右键实际会先关闭
// 菜单（互斥行为可接受），调用方场景需要重新定位时由其 @contextmenu 重开。
const onGlobalClick = () => close()
const onGlobalContextMenu = () => close()

onMounted(() => {
  document.addEventListener('click', onGlobalClick)
  document.addEventListener('contextmenu', onGlobalContextMenu, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onGlobalClick)
  document.removeEventListener('contextmenu', onGlobalContextMenu, true)
})

defineExpose({ open, close })
</script>
