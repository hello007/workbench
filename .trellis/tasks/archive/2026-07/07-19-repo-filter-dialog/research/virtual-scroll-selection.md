# 研究：仓库筛选器左栏虚拟滚动选型

- **查询**：RepoFilterDialog 左栏虚拟滚动技术选型（vue-virtual-scroller vs el-table-v2 vs 其他）
- **范围**：内部代码库核实 + 外部库知识对比
- **日期**：2026-07-19

## 一句话结论

**推荐 `@vueuse/core` 的 `useVirtualList`**（零新增依赖、自带 `scrollTo(index)`、布局完全自定义、足以覆盖 50~1000+ 等高项）；`vue-virtual-scroller RecycleScroller` 作为变高场景的备选；**不推荐 `el-table-v2`**（非纯表格、含 chips/失效标记的自定义项布局下显得笨重）。

## 1. 现有依赖盘点（已核实）

| 项 | 现状 | 证据 |
|---|---|---|
| vue-virtual-scroller | **未安装** | `frontend/package.json` 无该依赖；`node_modules` 无 virtual* 包 |
| Element Plus | `^2.13.7`，**自带 `el-table-v2`** | `frontend/package.json:37`；`node_modules/element-plus/es/components/table-v2/` 存在，导出 `ElTableV2` |
| @vueuse/core | `12.0.0`，**已安装且含 `useVirtualList`** | `node_modules/@vueuse/core/package.json`；`index.mjs` 中 `useVirtualList` 出现 5 次 |
| Vue | `3.5.33` | `node_modules/vue/package.json` |
| splitpanes | `4.0.4`，已在 `Home.vue` 使用 | `frontend/package.json:42`；`Home.vue:8,115-116` 三栏布局 |
| useDebounceFn（F16 防抖自动保存需要） | @vueuse 已提供 | 同上，无需额外引入 |
| @vueuse 在 src 中使用情况 | **当前 src 未引用 @vueuse** | grep 无匹配，属"已安装未用"的传递依赖，可零成本启用 |

`FileTreePanel.vue` 的 `el-tree` 懒加载 + `highlight-current` + `setCurrentKey` + `scrollBy` 滚动到当前节点模式，可作为选中态与滚动联动的现成参照（`FileTreePanel.vue:11-26, 1283-1332`，尤其 `1321-1330` 的 `scrollBy({ top: offset, behavior: 'smooth' })`）。

## 2. 三方案对比表

| 维度 | useVirtualList（@vueuse/core） | RecycleScroller（vue-virtual-scroller@2） | el-table-v2（Element Plus 自带） |
|---|---|---|---|
| 新增依赖 | **否**（@vueuse 12 已装） | 是（需装 `vue-virtual-scroller@2`） | **否**（EP 2.13.7 自带） |
| 维护活跃度 | 高（vueuse 持续更新） | **低**（v2 长期 beta，近年无新发布，存在 Vue 3.5 兼容风险，置信度中） | 高（随 EP 维护） |
| 布局灵活度 | **高**（纯 `v-for` 自定义模板） | **高**（item slot 任意布局） | 低-中（行列约束，自定义需 `cellRenderer` 返回 VNode） |
| 等高/变高 | 等高（`itemHeight` 数值）；变高需 `itemHeight` 函数 | RecycleScroller 等高；**DynamicScroller 支持变高** | 行高固定 |
| 选中态承载 | `item.data.path === selectedPath`（响应式 class） | slot 内 `selectedId === item.id`（响应式） | `row-class` / `cellRenderer`，需自行管理 |
| 滚动到指定项 | **`scrollTo(index)` 内置** | **`scrollToItem(index)` 内置** | `scrollToRow` / `scrollTo({scrollTop})` |
| 1000 项渲染性能 | 良好（slice 可视区+overscan） | 优秀（DOM 回收池） | 优秀（虚拟化） |
| 内存 | 仅可视区+overscan 项挂载 | 仅可视区+缓冲项挂载（DOM 复用） | 仅可视区挂载 |
| 实现复杂度 | 低（容器+wrapper+`v-for`） | 低（单组件+slot） | 中（列配置+cellRenderer） |
| 与项目风格一致性 | **高**（零新依赖，F16 也用 @vueuse） | 中（引入第三方） | 高（EP 原生）但场景不匹配 |
| 主要坑点 | 容器需固定高度；变高支持弱 | v2 与 Vue 3.5 兼容需实测；包不再活跃 | 非表格场景（chips/灰显）写 `cellRenderer` 笨重；列宽/选择列等表格概念是噪音 |

## 3. 选中状态管理（关键洞察）

**核心结论：虚拟滚动下选中高亮不会丢失，因为它由数据驱动而非 DOM 状态驱动。**

- RecycleScroller 滚动时会**复用 DOM 节点**并重新填充新 item 数据，但 slot 模板里的 `:class="{ 'is-selected': selectedId === item.id }"` 是响应式绑定——只要 `selectedId` 是 `ref`，新填入的项会立即重新计算 class，选中项滚回可视区时高亮自动恢复。
- useVirtualList 同理：`list` 是可视区 slice 的响应式结果，`item.data.path === selectedPath` 决定 class，滚动只改变 slice 内容，选中态始终正确。
- **不需要**把选中态写进 item 数据，也**不需要**手动操作 DOM class。唯一要做的是：选中项被滚出可视区后，若业务需要把它拉回可视区（如 F8 跳转、外部定位），调用 `scrollTo(index)` / `scrollToItem(index)`。
- 这与 `FileTreePanel.locateNode` 的做法一致：`tree.setCurrentKey(path)` 维护选中态（数据层），再 `scrollBy(offset)` 拉回可视区（视图层），二者分离（`FileTreePanel.vue:1321-1330`）。

## 4. 性能

- 50~1000+ 等高项，三方案均可流畅滚动：虚拟化保证 DOM 节点数恒为「可视区 + overscan/缓冲」，与总数据量无关。
- 1000 项下，`useVirtualList` 与 RecycleScroller 挂载的 DOM 数量同量级（约 15~30 个），内存占用差异可忽略；RecycleScroller 因 DOM 复用，快速滚动时略省创建开销，但 1000 项量级下人眼无感知差异。
- 真正的性能瓶颈在后端扫描（PRD NF1：100 仓库 < 3s，靠 `.git` 预筛优化），而非前端渲染——前端虚拟滚动足以应对。
- 置信度说明：以上为虚拟滚动通用规律 + 训练知识，未在本项目实测；建议落地后用 1000 条 mock 数据验证滚动帧率。

## 5. 与右栏联动

**虚拟滚动不影响事件绑定与右栏联动。**

- 点击项的 `@click="onSelect(item)"` 绑定在每个可视项上，正常 `emit('select', item)`；虚拟滚动只是控制哪些项挂载，挂载项的事件绑定与普通列表无异。
- 右栏接收选中项：父组件 `v-model:selected` 或 `@select` props 下传，与是否虚拟滚动无关。
- 「右栏编辑区固定不随左栏滚动」由布局保证而非虚拟滚动：用 `splitpanes` 把左右分到两个独立 `Pane`，左 Pane 内部有自己的滚动容器，右 Pane 是兄弟节点——左滚动不波及右 Pane，右栏输入框自然不失焦（直接满足验收项"滚动左栏时，右栏编辑区输入框不失焦"）。
- `splitpanes` 已在 `Home.vue:8-69` 验证可用，`<Pane :size :min-size>` 控制比例，`:push-other-panes="false"` 防止挤压。

## 6. 推荐方案与理由

**首选：`@vueuse/core` 的 `useVirtualList`。**

理由：
1. **零新增依赖**——@vueuse/core 12.0.0 已安装，且 F16 简述防抖自动保存本就要用 `useDebounceFn`，启用 @vueuse 与项目"最小依赖"风格一致。
2. **维护无风险**——vueuse 持续维护，无 Vue 3.5 兼容隐患（vue-virtual-scroller v2 长期 beta、近不活跃，置信度中）。
3. **布局完全自由**——`v-for` 渲染自定义项（名称/路径/chips/失效灰显），完美匹配"非纯表格"场景。
4. **`scrollTo(index)` 内置**——直接满足"选中项滚回可视区"（F8 跳转、外部定位）。
5. **足以覆盖 50~1000+ 等高项**——本场景紧凑列表项可设计为等高（路径省略、标签预览限 1 行）。

**备选：`vue-virtual-scroller RecycleScroller`**——仅当未来出现"项高度差异大、需变高虚拟化"时启用 `DynamicScroller`（useVirtualList 变高支持弱）。代价是新增依赖 + 兼容验证。

**不推荐：`el-table-v2`**——本场景不是表格，chips/失效标记/灰显用 `cellRenderer` + `h()` 写 VNode 反而笨重，列宽/选择列等表格概念是噪音。仅当未来需求收敛为纯多列表格时才考虑。

## 7. 最小可用代码框架（推荐方案 useVirtualList）

```vue
<!-- RepoFilterDialog.vue 左栏虚拟滚动核心骨架 -->
<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="emit('update:visible', $event)"
    title="仓库筛选器" width="900px" style="height:650px"
    append-to-body :close-on-click-modal="false"
  >
    <!-- 顶部工具区：工作目录下拉 + 搜索框 + 标签筛选 + 刷新（省略） -->

    <Splitpanes class="default-theme" :push-other-panes="false">
      <!-- 左栏：虚拟滚动列表 -->
      <Pane :size="55" :min-size="35">
        <!-- containerProps 自带 ref/onScroll/高度溢出样式；需给容器定高 -->
        <div v-bind="containerProps" class="repo-list">
          <div v-bind="wrapperProps">
            <div
              v-for="item in list"
              :key="item.data.path"
              class="repo-item"
              :class="{
                'is-selected': item.data.path === selectedPath,
                'is-missing': item.data.missing
              }"
              @click="onSelect(item.data)"
            >
              <div class="repo-item__name">{{ item.data.name }}</div>
              <div class="repo-item__path">{{ item.data.path }}</div>
              <div class="repo-item__tags">
                <!-- 标签预览：限 1 行，超出 +N，保证等高 -->
                <el-tag v-for="t in item.data.tags.slice(0, 3)" :key="t" size="small">{{ t }}</el-tag>
                <el-tag v-if="item.data.tags.length > 3" size="small" type="info">
                  +{{ item.data.tags.length - 3 }}
                </el-tag>
              </div>
              <el-tag v-if="item.data.missing" size="small" type="danger">失效</el-tag>
            </div>
          </div>
        </div>
      </Pane>

      <!-- 右栏：详情编辑区，独立 Pane，不随左栏滚动 -->
      <Pane :size="45" :min-size="30">
        <div class="repo-detail">
          <!-- 仓库名/路径/README 摘要/自定义简述(防抖)/标签编辑/跳转按钮 -->
        </div>
      </Pane>
    </Splitpanes>
  </el-dialog>
</template>

<script setup>
import { ref, watch } from 'vue'
import { useVirtualList, useDebounceFn } from '@vueuse/core'
import { Splitpanes, Pane } from 'splitpanes'
import { GetRepoFilterList, SaveRepoMeta } from '../../wailsjs/go/main/App'

const props = defineProps({ visible: { type: Boolean, default: false } })
const emit = defineEmits(['update:visible', 'select', 'locate']) // locate 触发 FileTreePanel.locateNode

const ITEM_HEIGHT = 72 // 等高项：名称(20)+路径(18)+chips(28)+间距 ≈ 72px
const allItems = ref([])           // GetRepoFilterList 返回的全部仓库
const selectedPath = ref('')       // 选中态主键（数据驱动高亮）

// 虚拟列表：list=可视区slice[{index,data}]，scrollTo(index) 滚到指定项
const { list, containerProps, wrapperProps, scrollTo } = useVirtualList(
  allItems,
  { itemHeight: ITEM_HEIGHT, overscan: 10 }
)

// 选中 -> 通知右栏
function onSelect(item) {
  selectedPath.value = item.path
  emit('select', item)
}

// 选中项滚回可视区（F8 跳转 / 外部定位 / Tab 切换后保持选中可见）
function scrollSelectedIntoView(path) {
  const idx = allItems.value.findIndex(i => i.path === path)
  if (idx >= 0) {
    selectedPath.value = path
    scrollTo(idx)
  }
}

// 简述防抖 800ms 自动保存（F16）
const saveSummary = useDebounceFn((path, summary, tags) => {
  SaveRepoMeta(path, summary, tags)
}, 800)

// 打开弹窗 + 自动扫描
watch(() => props.visible, async (v) => {
  if (!v) return
  allItems.value = await GetRepoFilterList(/* dirId */)
})
</script>

<style scoped>
.repo-list { height: 100%; overflow-y: auto; }       /* 容器必须定高，overscroll 由 useVirtualList 接管 */
.repo-item { height: 72px; box-sizing: border-box; }  /* 必须与 itemHeight 一致 */
.repo-item.is-selected { background: rgba(64,158,255,.1); border-left: 3px solid var(--primary-color); }
.repo-item.is-missing { opacity: .5; }                /* 失效灰显（F15） */
</style>
```

关键约束：
- `.repo-list` 容器**必须定高**（`height:100%` + 父 `Pane` 撑满），否则 `useVirtualList` 无法计算可视区。
- `.repo-item` 高度**必须**与 `itemHeight` 严格一致，否则滚动定位偏移。
- 标签预览限 1 行 + 路径 `text-overflow:ellipsis`，保证等高；若必须变高，切到备选 `DynamicScroller`。

## 8. 坑点与注意事项

1. **vue-virtual-scroller + Vue 3.5 兼容性**：v2 长期 beta、维护停滞，与 Vue 3.5.33 搭配可能触发 peer 警告或运行时问题，**选用前必须实测**（置信度中，未联网核实）。这是不选它作首选的主因之一。
2. **el-table-v2 的 `cellRenderer`**：返回 VNode 需用 `h()` 手写，chips + 失效标记 + 条件灰显会让 cellRenderer 函数冗长，且仍受列宽/行高表格约束，性价比低。
3. **等高约束**：useVirtualList 变高支持弱（`itemHeight` 函数模式需自行提供准确高度测量）。设计上应强制等高（路径省略、标签预览限 1 行、失效标记内联）。
4. **`scrollTo(index)` 时机**：数据异步加载后需 `await nextTick()` 再 `scrollTo`，否则 `allItems` 尚未填充，索引无效。
5. **右栏失焦**：必须用 `splitpanes` 把右栏放在独立 `Pane`（兄弟节点），切勿把右栏放进左栏滚动容器内——这是满足"滚动左栏时右栏输入框不失焦"的结构前提。
6. **`append-to-body`**：`el-dialog` 需 `append-to-body`（`UpdateDialog.vue:11` 同款），避免被父级 `overflow` 裁切虚拟列表。
7. **键盘导航**（若需 ↑↓ 选中）：虚拟列表下需手动实现——监听 keydown 调整 `selectedPath` 并 `scrollTo(index)`，无内置支持（el-table-v2 内置键盘导航，但代价是表格化）。
8. **Tab 切换/筛选后选中项保留**：切换"已编辑/未编辑"Tab 或标签筛选后，`allItems` 重新计算，若选中项仍在列表中应保持 `selectedPath` 并 `scrollTo` 到它；若已被筛掉，按业务决定清空或保留右栏只读。

## 9. 相关文件清单

| 文件 | 说明 |
|---|---|
| `frontend/package.json` | 依赖核实：EP 2.13.7 / @vueuse 12 / splitpanes 4 / 无 vue-virtual-scroller |
| `frontend/src/components/FileTreePanel.vue` | el-tree 懒加载 + `locateNode` 选中滚动模式（`:11-26`, `:1283-1332`, `:1346-1362` defineExpose） |
| `frontend/src/components/UpdateDialog.vue` | el-dialog v-model 模式参照（`:1-12`） |
| `frontend/src/views/Home.vue` | splitpanes 三栏布局参照 + import 方式（`:8-69`, `:115-116`） |
| `.trellis/tasks/07-19-repo-filter-dialog/prd.md` | 需求源头：F19 虚拟滚动、NF 验收、技术方案候选 |

## 10. 未决/外部知识置信度声明

- 本环境**未提供 `mcp__exa` 联网检索工具**，"vue-virtual-scroller 维护停滞/Vue 3.5 兼容风险""useVirtualList API 返回 `{list, containerProps, wrapperProps, scrollTo}`"等外部库特性基于训练知识，**置信度中**，落地前建议各用 1000 条 mock 数据做一次滚动帧率与 `scrollTo` 行为实测。
- 代码库内事实（依赖版本、组件存在性、现有模式）均为实读核实，置信度高。

---

**一行摘要**：左栏虚拟滚动推荐 `@vueuse/core useVirtualList`（零新增依赖、自带 scrollTo、自定义布局、选中态数据驱动不丢失），右栏用 splitpanes 独立 Pane 固定不失焦；备选 vue-virtual-scroller，不推荐 el-table-v2。
