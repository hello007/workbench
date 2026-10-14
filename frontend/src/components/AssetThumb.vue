<!-- frontend/src/components/AssetThumb.vue -->
<!-- 素材缩略图懒加载组件（画廊卡片与详情侧边栏共用）。
     读取链路：GetAssetThumb(libraryId, fileId, slideNum, tier) 返回 base64 裸串，
     命中直接拼 data:image/png;base64,<串> 交给 <img>（禁 decodeBase64Utf8 / 裸 atob——
     解码工具面向文本还原，图片二进制串直接作 data URL 载荷），未生成/已淘汰返回
     空串回退「无预览」占位（后端契约：空串非错误）。
     量级口径：画廊一律 tier=0（small 档）可视区懒加载，禁整库预取；hd（tier=1）
     仅详情侧边栏单张按需加载。
     状态机：idle → loading → ready(dataUrl) / miss(占位)；请求序号守卫——props 变更
     （切库/切卡）使在途响应写回前比对丢弃，防慢响应把旧卡数据串入新卡。
     epoch 外部失效代次：后台批次生成完成（assets-thumbs:done 后面板 thumbEpoch 自增）
     或重试完成后，miss 卡自动重新加载补图。
     读取失败静默回退占位不弹错：画廊批量懒加载逐卡弹错误噪音大，后台 slog 已有
     日志；详情大图同口径。 -->
<template>
  <div v-if="rendered" ref="rootRef" class="asset-thumb">
    <img v-if="status === 'ready'" :src="dataUrl" :alt="alt" />
    <div v-else class="asset-thumb-placeholder">无预览</div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { GetAssetThumb } from '../../wailsjs/go/main/App'

const props = defineProps({
  libraryId: { type: String, default: '' },
  fileId: { type: Number, default: 0 },
  slideNum: { type: Number, default: 1 },
  // 档位序号（0=small / 1=hd，序号非宽度——分辨率可配后宽度漂移，序号稳定）
  tier: { type: Number, default: 0 },
  // 外部失效代次：值变化且当前 miss 时重新加载（后台批次完成后的自动补图入口）
  epoch: { type: Number, default: 0 },
  // 详情场景：未生成时整个组件不渲染（loading 中渲染占位防布局跳动）
  hideOnMiss: { type: Boolean, default: false },
  alt: { type: String, default: '' }
})

const rootRef = ref(null)
const status = ref('idle') // 'idle' | 'loading' | 'ready' | 'miss'
const dataUrl = ref('')

let reqSeq = 0 // 请求序号守卫：props 变更/卸载自增，在途响应写回前比对丢弃
let observer = null
let intersected = false // 可视区触发标记（触发一次即停止观察）

// hideOnMiss 下 idle/miss 前态不渲染（loading 中渲染占位防布局跳动）
const rendered = computed(
  () => !props.hideOnMiss || status.value === 'loading' || status.value === 'ready'
)

const load = async () => {
  // 参数不完整（详情条目无底册记录 fileId=0 等）直接 miss，不发无谓请求
  if (!props.libraryId || !props.fileId || props.fileId < 1 || props.slideNum < 1) {
    status.value = 'miss'
    return
  }
  const seq = ++reqSeq
  status.value = 'loading'
  try {
    const base64 = await GetAssetThumb(props.libraryId, props.fileId, props.slideNum, props.tier)
    if (seq !== reqSeq) return // props 已变更/已卸载：旧响应丢弃防串卡
    if (base64) {
      dataUrl.value = `data:image/png;base64,${base64}`
      status.value = 'ready'
    } else {
      // 空串 = 未生成/已淘汰，回退占位
      dataUrl.value = ''
      status.value = 'miss'
    }
  } catch {
    if (seq !== reqSeq) return
    // 读取失败静默回退占位（画廊批量懒加载逐卡弹错误噪音大，后台 slog 已有日志；
    // 详情大图同口径）
    dataUrl.value = ''
    status.value = 'miss'
  }
}

// 可视区触发：IO 存在则进可视区才加载（threshold 0.05，触发一次即停止观察）；
// 无 IO 环境（jsdom/旧内核）或 hideOnMiss（根元素 miss/idle 前态不渲染无宿主可
// 观察，且详情面板打开即用户可见）挂载即加载
const observeOrLoad = () => {
  if (intersected || props.hideOnMiss || typeof IntersectionObserver === 'undefined') {
    load()
    return
  }
  if (observer) observer.disconnect()
  observer = new IntersectionObserver(
    (entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return
      intersected = true
      if (observer) {
        observer.disconnect()
        observer = null
      }
      load()
    },
    { threshold: 0.05 }
  )
  if (rootRef.value) observer.observe(rootRef.value)
}

// 切库/切卡（props 变更）：重置展示态后按可视区状态重新加载（旧在途响应经序号
// 守卫丢弃）；NUL 分隔拼接避免多字段组合歧义
watch(
  () => `${props.libraryId}\u0000${props.fileId}\u0000${props.slideNum}\u0000${props.tier}`,
  () => {
    reqSeq++
    status.value = 'idle'
    dataUrl.value = ''
    observeOrLoad()
  }
)

// 后台批次生成/重试完成：miss 卡自动补图（ready 卡不重拉，避免无谓大载荷传输）
watch(
  () => props.epoch,
  () => {
    if (status.value === 'miss') load()
  }
)

onMounted(observeOrLoad)

onBeforeUnmount(() => {
  reqSeq++ // 在途响应写回前比对丢弃（组件已卸载不写回）
  if (observer) {
    observer.disconnect()
    observer = null
  }
})
</script>

<style scoped>
.asset-thumb {
  width: 100%;
  aspect-ratio: 16 / 9;
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.asset-thumb img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.asset-thumb-placeholder {
  font-size: 12px;
  color: var(--text-placeholder);
}
</style>
