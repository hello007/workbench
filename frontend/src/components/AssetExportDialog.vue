<!-- frontend/src/components/AssetExportDialog.vue -->
<!-- 素材元数据导出弹窗（素材管理第九批 PR1，SRS §2.6.4 / 附录A）：CSV（UTF-8 BOM）
     与 Markdown+YAML frontmatter 卡片两种格式可同选。
     契约（形态对齐 AssetBatchTagDialog/AssetBatchRatingDialog 宿主分派先例）：
     - 范围留宿主：本组件只收 previewCount（「将导出 N 条」预览）与
       previewScopeLabel（范围人读文案：选中 / 当前检索命中 / 全库），范围解析
       （D2 三态优先级）与检索快照构建在宿主（panel 检索状态单源）；
     - execute 分派回调 (partial) => Promise<model.AssetExportResult>：partial =
       { csvPath, mdDir, formats, pathMode, overwrite }，宿主合并范围字段后调
       ExportAssetMetadata（本层零绑定 import，便于 spec 注入 execute 断言参数）；
     - 中止契约（P3 体验收口）：提交中关闭弹窗（ESC / X / 遮罩 / footer 中止导出）
       = 用户放弃本次导出——标记 abandoned 使在途 execute 的终态静默/中性收口，
       并经 cancel 回调（宿主持有 CancelAssetExport 绑定）触发后端取消
       （exportCancels 注册表 → 批间 ctx 检查 → w.abort() 删半成品 CSV）；后端
       取消竞态下先自然完成则文件完整落盘，以「后台完成」中性提示收口；
     - 覆盖确认链路（D5）：目标已存在返回确认型错误 {code:
       E_ASSETS_EXPORT_TARGET_EXISTS} → ElMessageBox.confirm（列出后端消息含
       具体目标路径）→ 确认后以 overwrite=true 重调（成功 toast + 关闭）；取消
       什么都不发生；其他错误经 handleError 按 code 分流（warning/error）；
     - 目标路径浏览经 props 回调注入（宿主持有 SaveFileDialog /
       OpenDirectoryDialog 绑定，空串 = 用户取消 no-op）。
     视觉：弹窗 560 档（中小弹窗分档，对齐 frontend-visual-conventions 弹窗尺寸
     策略），top vh 响应式；格式勾选语义色 hint（info 灰）。 -->
<template>
  <el-dialog
    v-model="visible"
    title="导出元数据"
    width="min(560px, 86vw)"
    top="15vh"
    :close-on-click-modal="false"
    :before-close="guardClose"
    append-to-body
  >
    <div class="asset-export-body">
      <!-- 范围预览（D2）：N 由宿主解析（选中数 / 检索重放全集 total / 库统计合计），
           -1 视为统计中占位（0 = 零命中/空库合法终态，正常显示） -->
      <p class="export-scope">
        将导出 <span class="export-count">{{ previewCount >= 0 ? previewCount : '…' }}</span> 条
        <span class="export-scope-label">（{{ previewScopeLabel }}）</span>
      </p>

      <div class="export-section">
        <span class="export-label">导出格式（可同选）</span>
        <el-checkbox v-model="csvEnabled" :disabled="submitting">CSV（Excel 兼容，UTF-8 BOM）</el-checkbox>
        <el-checkbox v-model="mdEnabled" :disabled="submitting">Markdown + YAML（每条一张卡片，Obsidian 可直接使用）</el-checkbox>
      </div>

      <div class="export-section">
        <span class="export-label">CSV 目标文件</span>
        <div class="export-target-row">
          <el-input
            v-model="csvPath"
            placeholder="例如 D:\导出\素材元数据.csv"
            :disabled="!csvEnabled || submitting"
            @keyup.enter="onSubmit"
          />
          <el-button :disabled="!csvEnabled || submitting" @click="onPickCsvPath">浏览…</el-button>
        </div>
      </div>

      <div class="export-section">
        <span class="export-label">Markdown 导出目录</span>
        <div class="export-target-row">
          <el-input
            v-model="mdDir"
            placeholder="例如 D:\导出\素材卡片"
            :disabled="!mdEnabled || submitting"
            @keyup.enter="onSubmit"
          />
          <el-button :disabled="!mdEnabled || submitting" @click="onPickMdDir">浏览…</el-button>
        </div>
      </div>

      <div class="export-section">
        <span class="export-label">路径口径</span>
        <el-radio-group v-model="pathMode" :disabled="submitting">
          <el-radio value="relative">素材库相对路径</el-radio>
          <el-radio value="absolute">绝对路径</el-radio>
        </el-radio-group>
        <span class="export-hint">
          相对路径以素材库根目录为基准（Obsidian 端口更稳）；缩略图路径恒为绝对路径；Markdown 内路径自动转为 /。
        </span>
      </div>
    </div>

    <template #footer>
      <el-button @click="onCancelClick">{{ submitting ? '中止导出' : '取消' }}</el-button>
      <el-button type="primary" :loading="submitting" @click="onSubmit">导出</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { handleError, ErrorCode } from '../utils/error'

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  // 「将导出 N 条」预览数（宿主解析：选中数 / 检索重放全集 / 库统计合计；
  // -1 = 统计中占位，0 = 零命中合法终态）
  previewCount: { type: Number, default: -1 },
  // 范围人读文案（「选中 3 项」/「当前检索命中」/「全库」）
  previewScopeLabel: { type: String, default: '' },
  // 分派执行回调 (partial) => Promise<model.AssetExportResult>：宿主合并范围字段
  //（fileIds/slideTargets/keyword/search）后调 ExportAssetMetadata
  execute: { type: Function, default: null },
  // CSV 目标文件浏览回调 () => Promise<string>（保存文件对话框；空串 = 用户取消）
  pickCsvPath: { type: Function, default: null },
  // Markdown 目标目录浏览回调 () => Promise<string>（目录对话框；空串 = 用户取消）
  pickMdDir: { type: Function, default: null },
  // 中止回调（宿主持有 CancelAssetExport 绑定，本层零绑定 import）：提交中关闭
  // 弹窗 = 放弃本次导出，经此触发后端取消（exportCancels 注册表命中 → 批间
  // ctx 检查 → w.abort() 删半成品 CSV）；无在途导出幂等 false
  cancel: { type: Function, default: null }
})

const emit = defineEmits(['update:modelValue', 'exported'])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

// 中止通道（P3 体验收口，替换原「提交中硬阻断关闭」形态）：abandoned 标记提交中
// 被用户放弃——在途 execute 的后续终态按放弃语义收口（成功 = 后端先于取消自然
// 完成、文件完整落盘 → 中性提示；失败 = 静默，用户已放弃等待，错误提示属噪音）。
// 后端取消对齐扫描取消形态（exportCancels 注册表 + 批间 ctx 检查 + w.abort() 删
// 半成品 CSV），一次性语义无跨调用重入，再次导出 = 新调用
const abandoned = ref(false)

// 放弃本次导出（提交中才有语义；非提交期 no-op 返回 false）：标记 + 经宿主回调
// 触发后端取消。cancel 调用失败静默（注册表 miss 幂等 false / 绑定异常均无用户
// 语义——后端要么已结束要么自然完成）
const abandonExport = () => {
  if (!submitting.value) return false
  abandoned.value = true
  if (typeof props.cancel === 'function') {
    Promise.resolve(props.cancel()).catch(() => {})
  }
  return true
}

// 关闭守卫：提交中关闭（ESC / X 按钮 / 遮罩点击——EP 的 before-close 对三类入口
// 统一生效）= 用户放弃本次导出，标记后放行；非提交期直接放行（ESC/X 便利保留）。
// footer 按钮直接置 v-model 绕过 before-close，其中止分支显式走 abandonExport
// 单点（onCancelClick）
const guardClose = (done) => {
  abandonExport()
  done()
}

// ---- 表单态（跨开关保留：用户重复导出免重输目标路径）----
const csvEnabled = ref(true)
const mdEnabled = ref(false)
const csvPath = ref('')
const mdDir = ref('')
const pathMode = ref('relative')
const submitting = ref(false)

const currentFormats = () => {
  const formats = []
  if (csvEnabled.value) formats.push('csv')
  if (mdEnabled.value) formats.push('md')
  return formats
}

const doSubmit = (overwrite) => props.execute({
  csvPath: csvPath.value.trim(),
  mdDir: mdDir.value.trim(),
  formats: currentFormats(),
  pathMode: pathMode.value,
  overwrite
})

const onPickCsvPath = async () => {
  if (typeof props.pickCsvPath !== 'function') return
  try {
    const path = await props.pickCsvPath()
    if (path) csvPath.value = path
  } catch (error) {
    handleError('选择导出文件失败: ', error)
  }
}

const onPickMdDir = async () => {
  if (typeof props.pickMdDir !== 'function') return
  try {
    const dir = await props.pickMdDir()
    if (dir) mdDir.value = dir
  } catch (error) {
    handleError('选择导出目录失败: ', error)
  }
}

// footer 关闭/中止按钮：提交中文案切「中止导出」并启用（关闭 = 放弃本次导出），
// 非提交期为普通取消。直接置 v-model 绕过 before-close，故放弃标记显式走
// abandonExport 单点
const onCancelClick = () => {
  abandonExport()
  visible.value = false
}

// 导出成功收口：toast（CSV 行数 + MD 卡数）→ 关闭 → 上报 exported（宿主按需刷新）
const finishExport = (result) => {
  const rows = result?.rows ?? 0
  const mdFiles = result?.mdFiles ?? 0
  ElMessage.success(mdFiles > 0 ? `已导出 ${rows} 条元数据（${mdFiles} 张卡片）` : `已导出 ${rows} 条元数据`)
  emit('exported', result)
  visible.value = false
}

// 终态收口单点（首次提交与覆盖重调共用）：放弃后到达的成功终态 = 后端先于取消
// 自然完成、文件已完整落盘——中性提示 + 照常上报（纯只读导出，宿主无刷新义务）；
// 不走成功 toast 与关闭（弹窗已关，防误导性「已导出」文案）
const settleResult = (result) => {
  if (abandoned.value) {
    ElMessage.info('已中止等待，导出已在后台完成')
    emit('exported', result)
    return
  }
  finishExport(result)
}

const runExport = async (overwrite) => {
  try {
    const result = await doSubmit(overwrite)
    settleResult(result)
  } catch (error) {
    if (abandoned.value) return // 放弃后的失败终态静默（含后端确认的取消错误）
    if (error?.code === ErrorCode.AssetsExportTargetExists) {
      // 确认型拒绝（D5）：后端消息含具体目标路径，确认后 overwrite=true 重调
      try {
        await ElMessageBox.confirm(
          `${error.message || '导出目标已存在'}。是否覆盖写入？`,
          '覆盖确认',
          { type: 'warning', confirmButtonText: '覆盖写入', cancelButtonText: '取消' }
        )
      } catch {
        return // 用户取消覆盖：不做任何事
      }
      if (abandoned.value) return // 确认期间放弃：不再重调
      await runExport(true)
      return
    }
    handleError('导出失败: ', error)
  }
}

const onSubmit = async () => {
  if (submitting.value || typeof props.execute !== 'function') return
  // 前端第一防线：至少一个格式 + 选中格式目标非空（后端同样校验，双防线）
  if (currentFormats().length === 0) {
    ElMessage.warning('请至少选择一种导出格式（CSV / Markdown）')
    return
  }
  if (csvEnabled.value && !csvPath.value.trim()) {
    ElMessage.warning('请填写 CSV 导出文件路径')
    return
  }
  if (mdEnabled.value && !mdDir.value.trim()) {
    ElMessage.warning('请填写 Markdown 导出目录')
    return
  }
  abandoned.value = false
  submitting.value = true
  try {
    await runExport(false)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.asset-export-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.export-scope {
  margin: 0;
  font-size: 13px;
  color: var(--text-primary);
}

.export-count {
  font-weight: 600;
  color: var(--primary-color);
}

.export-scope-label {
  font-size: 12px;
  color: var(--text-tertiary);
}

.export-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.export-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.export-target-row {
  display: flex;
  gap: var(--spacing-sm);
  align-items: center;
}

.export-hint {
  font-size: 12px;
  color: var(--text-tertiary);
}
</style>
