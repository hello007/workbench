<!-- frontend/src/components/AssetBackupDialog.vue -->
<!-- 素材库备份还原弹窗（素材管理第十批 SRS §2.7.5，工具行「还原」入口）：
     备份清单（时间/大小倒序表格）+ 选份还原确认链。
     契约（形态对齐 AssetPrefixReplaceDialog 宿主分派先例）：
     - 清单拉取与还原分派留宿主：本组件收 listBackups / executeRestore 回调
       props（本层零绑定 import，便于 spec 注入断言）；打开即拉取清单（List
       恒权威，本地不缓存），「刷新」按钮重拉；
     - 还原确认（PRD R3）：ElMessageBox 明示「将覆盖当前库全部索引与标注」，
       确认后按条目绝对路径调 executeRestore（后端校验路径必须位于该库备份
       目录内，防任意文件覆盖库 db）；成功 emit restored——宿主刷新库数据
       视图并提示重新扫描（备份点之后的磁盘新增文件不在索引）；
     - 行级防重：还原在途条目路径占位（restoringPath），防同行连点重复提交；
     - 错误分流：备份文件损坏（还原前完整性预检失败）等经 handleError 按码
       分流，弹窗保持打开可另选备份。 -->
<template>
  <el-dialog
    v-model="visible"
    :title="`备份还原 — ${libraryName}`"
    width="min(680px, 90vw)"
    top="12vh"
    :close-on-click-modal="false"
    append-to-body
  >
    <div class="backup-body">
      <p class="backup-hint">
        备份为整库索引快照（含文件清单、标签、标注、评分），最多保留 5 份。还原将以所选备份整体覆盖当前库索引，原始素材文件不受影响。
      </p>
      <div class="backup-toolbar">
        <el-button size="small" :loading="loading" @click="reload">刷新</el-button>
      </div>
      <div v-if="loading" class="backup-state">加载备份清单…</div>
      <div v-else-if="backups.length === 0" class="backup-state">
        暂无备份，点工具行「备份」生成当前库快照
      </div>
      <el-table v-else :data="backups" size="small" max-height="360">
        <el-table-column prop="createdAt" label="备份时间" width="170" />
        <el-table-column prop="fileName" label="备份文件" min-width="180" show-overflow-tooltip />
        <el-table-column label="大小" width="90">
          <template #default="{ row }">{{ formatSize(row.sizeBytes) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button
              size="small"
              text
              type="primary"
              :loading="restoringPath === row.path"
              :disabled="restoringPath !== ''"
              @click="onRestore(row)"
            >还原</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>
    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { handleError } from '../utils/error'
// 大小格式化共享单点（对齐详情面板 formatSize 口径）
import { formatAssetSize as formatSize } from '../utils/assetFormat'

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  // 库 ID 与名称（清单拉取参数 + 标题展示）
  libraryId: { type: String, default: '' },
  libraryName: { type: String, default: '' },
  // 清单拉取回调 () => Promise<model.AssetBackupEntry[]>（倒序）：宿主持有
  // ListAssetLibraryBackups 绑定（本层零绑定 import）
  listBackups: { type: Function, default: null },
  // 还原分派回调 (backupFilePath) => Promise<void>：宿主持有 RestoreAssetLibrary
  // 绑定（条目 path 绝对路径原样回传，后端校验必须位于该库备份目录内）
  executeRestore: { type: Function, default: null }
})

const emit = defineEmits(['update:modelValue', 'restored'])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const backups = ref([])
const loading = ref(false)
// 行级防重占位：还原在途条目路径（空串 = 空闲），防同表连点并发还原
const restoringPath = ref('')

const reload = async () => {
  if (typeof props.listBackups !== 'function' || !props.libraryId) return
  loading.value = true
  try {
    backups.value = (await props.listBackups(props.libraryId)) || []
  } catch (error) {
    handleError('加载备份清单失败: ', error)
  } finally {
    loading.value = false
  }
}

// 打开即拉取最新清单（List 恒权威，本地不缓存）；immediate 覆盖「挂载即打开」
// 形态（宿主 v-model 常量 true 时 watch 默认不触发）；库切换语境下弹窗由宿主
// 关闭，重开按新库重拉
watch(
  () => props.modelValue,
  (opened) => {
    if (opened) reload()
  },
  { immediate: true }
)

const onRestore = async (entry) => {
  if (restoringPath.value || typeof props.executeRestore !== 'function') return
  // 确认弹窗（PRD R3 明示覆盖后果）：还原整体替换当前库索引与标注，备份点
  // 之后的索引变更丢失；原始素材文件不受影响
  try {
    await ElMessageBox.confirm(
      `将用「${entry.createdAt}」的备份覆盖当前库全部索引与标注（标签、评分、备注一并回退），此操作不可撤销。还原后建议重新扫描以收录新增文件。确定还原？`,
      '还原备份',
      { type: 'warning', confirmButtonText: '还原', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  restoringPath.value = entry.path
  try {
    await props.executeRestore(entry.path)
    ElMessage.success(`已还原备份「${entry.fileName}」，建议重新扫描以同步索引`)
    visible.value = false
    emit('restored', entry)
  } catch (error) {
    handleError('还原备份失败: ', error)
  } finally {
    restoringPath.value = ''
  }
}
</script>

<style scoped>
.backup-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.backup-hint {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
  line-height: 1.6;
}

.backup-toolbar {
  display: flex;
  justify-content: flex-end;
}

.backup-state {
  font-size: 12px;
  color: var(--text-tertiary);
  padding: var(--spacing-sm) 0;
}
</style>
