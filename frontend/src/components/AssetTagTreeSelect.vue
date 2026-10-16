<!-- frontend/src/components/AssetTagTreeSelect.vue -->
<!-- 素材标签树形多选下拉（素材管理第六批，SRS §2.4 / PRD D6）：基于 el-tree-select
     封装的单组件，标签筛选 / 详情编辑 / 批量打标签弹窗（PR3 接入）三处复用，树
     构建与节点呈现收敛本组件防三处口径漂移。
     口径契约：
     - modelValue 统一为标签 ID 数组（PRD D6 定夺：组件统一 ID 口径，消费方映射
       ——保存类入参（如 UpdateAssetFileMeta 的标签名数组）由宿主经共享标签列表
       做名称映射，组件内不做名称转换，避免两套口径在组件内分支）；
     - 数据源 = ListAssetTags(libraryId)（含未使用标签与直接绑定计数）：挂载即拉、
       下拉展开时重拉（防管理弹窗新建标签后树陈旧）、切库重拉；暴露 reload() 供
       宿主在标签 CRUD 后强制刷新；拉取成功经 loaded 事件回传平铺清单，宿主据此
       维持一份共享列表（名称映射等场景消费，免二次拉取）；
     - 级联命中（选父含全部后代）由后端检索链路展开（expandTagDescendants），
       前端不展开；check-strictly 保持逐节点独立勾选——绑定是精确动作不级联
       （PRD D2 附带决议：批量勾父标签只绑定被勾标签行本身）；
     - 拉取失败降级空树（下拉内空态提示），E_ASSETS_LIB_NOT_FOUND 静默清空
       （库列表过期自愈由宿主负责，避免误报）；重新展开下拉或 reload() 可重试；
     - loading 态刻意不透传 select（EP select loading 会隐藏下拉主体渲染 empty 区
       且 disable 关闭下拉——展开重拉会被打断）：重拉期间树保持可见、就绪后原位
       更新，仅首拉未就绪且空数据的窗口在 empty 区提示加载中。
     视觉：节点 = 色点 + 标签名 + 直接绑定计数徽标（count>0 时显示），对齐
     frontend-visual-conventions 语义色与间距档位。 -->
<template>
  <el-tree-select
    v-model="innerValue"
    class="asset-tag-tree-select"
    :data="treeData"
    :placeholder="placeholder"
    :disabled="disabled"
    :size="size || undefined"
    multiple
    filterable
    clearable
    collapse-tags
    collapse-tags-tooltip
    show-checkbox
    check-strictly
    :render-after-expand="false"
    @visible-change="onVisibleChange"
    @change="onChange"
  >
    <template #default="scope">
      <!-- scope 守卫：宿主测试环境未注册/未桩 el-tree-select 时（Vue 解析失败回退
           渲染默认插槽且不传插槽参数），解构 undefined 会炸整条渲染链（Home.spec
           三栏布局用例曾暴露），此守卫让降级形态安全渲染为空 -->
      <span v-if="scope && scope.data" class="tag-node">
        <!-- 标签色为后端数据（#RRGGBB），动态绑定是唯一呈现方式，非静态行内样式；
             颜色缺失（兜底数据形态）退化为透明占位保持行高一致 -->
        <span class="tag-node-dot" :style="{ background: scope.data.color || 'transparent' }"></span>
        <span class="tag-node-name">{{ scope.data.name }}</span>
        <!-- 直接绑定计数徽标：仅 count>0 渲染（0 = 未使用标签不显噪） -->
        <span v-if="scope.data.count > 0" class="tag-node-count">{{ scope.data.count }}</span>
      </span>
    </template>
    <template #empty>
      <!-- 加载态分流：EP select 的 loading 会隐藏下拉主体渲染 empty 区且 disable
           关闭下拉（展开重拉被打断），故 loading 不透传 select——重拉期间树保持
           可见就绪后原位更新，仅首拉未就绪且空数据的极短窗口在此提示加载中 -->
      <div class="tag-tree-empty">{{ loading ? '标签加载中…' : emptyText }}</div>
    </template>
  </el-tree-select>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ListAssetTags } from '../../wailsjs/go/main/App'
import { handleError, ErrorCode } from '../utils/error'
import { buildTagTree } from '../utils/assetTagTree'

const props = defineProps({
  // 选中标签 ID 数组（统一 ID 口径，名称映射由宿主完成，见文件头口径契约）
  modelValue: { type: Array, default: () => [] },
  // 所属素材库 ID（空 = 无库不拉取，树恒空）
  libraryId: { type: String, default: '' },
  placeholder: { type: String, default: '按标签筛选' },
  disabled: { type: Boolean, default: false },
  // EP 尺寸档（'' = 组件默认）：检索工具行传 small 与搜索输入框（small）高度
  // 统一（D8#3），详情编辑/批量弹窗保持默认档不回归
  size: { type: String, default: '' },
  // 树为空时下拉内空态文案（筛选与详情编辑语境文案不同，由消费方传入）
  emptyText: { type: String, default: '暂无标签，可在「管理标签」中新建' }
})

const emit = defineEmits(['update:modelValue', 'change', 'loaded'])

const flatTags = ref([])
const loading = ref(false)
// 拉取代次守卫：切库 / 连续重拉的旧响应写回前比对，代次已变丢弃（防旧库标签串入新库树）
let fetchSeq = 0

const treeData = computed(() => buildTagTree(flatTags.value))

// modelValue 代理：整体写回新数组（消费方 v-model 直绑响应式 ref，保持禁原地
// 变异的响应式纪律——口径与宿主 selectedKeys 的整体替换策略一致）
const innerValue = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val || [])
})

const onChange = (val) => emit('change', val || [])

const fetchTags = async () => {
  if (!props.libraryId) {
    flatTags.value = []
    emit('loaded', [])
    return
  }
  const seq = ++fetchSeq
  loading.value = true
  try {
    const items = await ListAssetTags(props.libraryId)
    if (seq !== fetchSeq) return
    flatTags.value = items || []
    // 回传平铺清单供宿主维持共享列表（名称映射 / 页自有标签回构等场景消费）
    emit('loaded', flatTags.value)
  } catch (error) {
    if (seq !== fetchSeq) return
    flatTags.value = []
    // 库不存在（已删除，宿主列表自愈重拉中）静默清空防误报；其余错误统一分流
    if (error?.code !== ErrorCode.AssetsLibNotFound) {
      handleError('加载标签列表失败: ', error)
    }
  } finally {
    if (seq === fetchSeq) loading.value = false
  }
}

// 下拉展开即重拉：管理弹窗增删改标签后不依赖宿主刷新时序，打开下拉总能看到最新树
const onVisibleChange = (visible) => {
  if (visible) fetchTags()
}

watch(() => props.libraryId, () => fetchTags())

onMounted(() => fetchTags())

// 宿主强制刷新入口（标签 CRUD 完成后调用，与下拉展开重拉同链路）
defineExpose({ reload: fetchTags })
</script>

<style scoped>
.tag-node {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  min-width: 0;
}

.tag-node-dot {
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: var(--radius-sm);
}

.tag-node-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 直接绑定计数徽标：弱化形态（tertiary 色系），字号小于正文作辅助信息 */
.tag-node-count {
  flex-shrink: 0;
  font-size: 11px;
  line-height: 1;
  padding: 2px 6px;
  color: var(--text-tertiary);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
}

.tag-tree-empty {
  padding: var(--spacing-md);
  font-size: 12px;
  color: var(--text-placeholder);
  text-align: center;
}
</style>
