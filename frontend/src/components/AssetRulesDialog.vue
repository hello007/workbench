<!-- frontend/src/components/AssetRulesDialog.vue -->
<!-- 素材库索引设置弹窗（素材管理第十批 SRS §2.7.3，工具行「索引设置」入口）：
     排除文件夹（ExcludeDirs，目录段 glob 模式）与忽略后缀（ExcludeExts，后缀
     黑名单）两组规则的增删编辑。
     契约（形态对齐 AssetPrefixReplaceDialog 宿主分派先例）：
     - 规则编辑与保存分派留宿主：本组件收 library（回填数据源）与 execute 回调
       prop（本层零绑定 import，便于 spec 注入断言）；保存整体替换语义，后端
       保存成功后自动触发该库增量重扫（Promise 返回时重扫已完成，返回扫描汇总）；
     - 目录规则：每项一个 glob 模式（单段语法 * ? [...]，匹配任意层级同名目录，
       不含路径分隔符），保留输入原样大小写回传（匹配侧后端归一，与存储口径
       一致）；含路径分隔符为前端第一道防线拦截（后端同口径校验双防线）；
     - 后缀规则：添加即归一带点小写（tmp / .tmp / *.tmp 三形态等价，与后端
       normalizeAssetExcludeExt 存储口径一致），查重基于归一值；
     - 保存成功提示扫描汇总（新排除/复活计数）并 emit saved——宿主刷新库列表
       （规则回显同步）与库数据视图（excluded/复活生效）；
     - 错误分流：非法规则 / 扫描进行中拒绝（规则已保存）经 handleError 按码
       warning 分流，弹窗保持打开可改后重试。 -->
<template>
  <el-dialog
    v-model="visible"
    title="索引设置"
    width="min(560px, 86vw)"
    top="12vh"
    :close-on-click-modal="false"
    append-to-body
  >
    <div class="rules-body">
      <p class="rules-hint">
        扫描时跳过命中的文件与文件夹；已入库的命中文件转为「已排除」（检索不可见，标注与缩略图保留），放宽规则后重新扫描即恢复。
      </p>

      <!-- 排除文件夹：目录段 glob 模式列表 -->
      <div class="rules-field">
        <span class="rules-label">排除文件夹</span>
        <div class="rules-input-row">
          <el-input
            v-model="dirInput"
            placeholder="按文件夹名匹配任意层级目录，支持 * ? 通配，如「*草稿*」「archive」"
            :disabled="saving"
            @keyup.enter="addDir"
          />
          <el-button :disabled="saving" @click="addDir">添加</el-button>
        </div>
        <div class="rules-tags">
          <el-tag
            v-for="dir in dirRules"
            :key="dir"
            class="rules-tag"
            closable
            :disable-transitions="true"
            @close="removeDir(dir)"
          >{{ dir }}</el-tag>
          <span v-if="dirRules.length === 0" class="rules-empty">未配置</span>
        </div>
      </div>

      <!-- 忽略后缀：归一带点小写黑名单 -->
      <div class="rules-field">
        <span class="rules-label">忽略后缀</span>
        <div class="rules-input-row">
          <el-input
            v-model="extInput"
            placeholder="后缀自动归一为带点小写，tmp / .tmp / *.tmp 等价"
            :disabled="saving"
            @keyup.enter="addExt"
          />
          <el-button :disabled="saving" @click="addExt">添加</el-button>
        </div>
        <div class="rules-tags">
          <el-tag
            v-for="ext in extRules"
            :key="ext"
            class="rules-tag"
            closable
            :disable-transitions="true"
            @close="removeExt(ext)"
          >{{ ext }}</el-tag>
          <span v-if="extRules.length === 0" class="rules-empty">未配置</span>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button :disabled="saving" @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="onSave">保存并重新扫描</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { handleError } from '../utils/error'

const props = defineProps({
  // 弹窗显隐（v-model）
  modelValue: { type: Boolean, default: false },
  // 回填数据源（model.AssetLibrary：excludeDirs / excludeExts 归一存储值）；
  // null 安全（无库时入口禁用，正常不触达）
  library: { type: Object, default: null },
  // 保存分派回调 (dirs, exts) => Promise<model.AssetScanResult>：宿主持有
  // UpdateAssetLibraryRules 绑定（本层零绑定 import），后端保存成功后自动
  // 增量重扫，Promise 返回时重扫已完成
  execute: { type: Function, default: null }
})

const emit = defineEmits(['update:modelValue', 'saved'])

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

// 两组规则的可编辑列表（打开时以 library 当前值回填；库切换由回填覆盖）
const dirRules = ref([])
const extRules = ref([])
const dirInput = ref('')
const extInput = ref('')
const saving = ref(false)

// 后缀归一（与后端 normalizeAssetExcludeExt 存储口径一致）：trim → 剥「*.」
// 前缀 → 剥前导点 → 补前导点 → 小写；空串（含输入「.」）返回空由调用方丢弃
const normalizeExt = (raw) => {
  const e = String(raw || '').trim()
  if (!e) return ''
  const stripped = e.replace(/^\*\./, '').replace(/^\./, '')
  if (!stripped) return ''
  return `.${stripped.toLowerCase()}`
}

// 回填时机 = 「关闭→打开」沿：弹窗打开瞬间以当前库权威规则回填一次。刻意不在
// 弹窗保持打开期间跟随 library 对象替换重填——宿主刷新链（规则保存成功后
// loadLibraries、备份/还原后库列表重拉等）会整体替换 library 对象引用，打开
// 期间重填会静默覆盖用户未保存的编辑。当前宿主刷新触发点均在弹窗关闭后
//（watch 早退无害），守卫为未来宿主刷新链路扩展的缺陷预防；关闭→重开沿照常
// 回填最新权威值（编辑态不跨打开会话保留，回显以后端存储为准）
let prevOpened = false
watch(
  () => [props.modelValue, props.library],
  ([opened]) => {
    const opening = opened && !prevOpened
    prevOpened = opened
    if (!opening) return
    dirRules.value = [...(props.library?.excludeDirs || [])]
    extRules.value = [...(props.library?.excludeExts || [])]
    dirInput.value = ''
    extInput.value = ''
  },
  { immediate: true }
)

const addDir = () => {
  const d = dirInput.value.trim()
  if (!d) return
  // 前端第一道防线（后端同口径校验，双防线）：单段语义下路径分隔符永不命中，
  // 拦截防误配静默失效
  if (d.includes('/') || d.includes('\\')) {
    ElMessage.warning('排除文件夹按名称匹配，不能包含路径分隔符')
    return
  }
  // 查重忽略大小写（匹配侧大小写不敏感，同形异写视为重复）
  if (dirRules.value.some(x => x.toLowerCase() === d.toLowerCase())) {
    ElMessage.warning('该规则已存在')
    return
  }
  dirRules.value = dirRules.value.concat(d)
  dirInput.value = ''
}

const removeDir = (dir) => {
  dirRules.value = dirRules.value.filter(x => x !== dir)
}

const addExt = () => {
  const ext = normalizeExt(extInput.value)
  if (!ext) return
  if (extRules.value.includes(ext)) {
    ElMessage.warning('该后缀已存在')
    return
  }
  extRules.value = extRules.value.concat(ext)
  extInput.value = ''
}

const removeExt = (ext) => {
  extRules.value = extRules.value.filter(x => x !== ext)
}

const onSave = async () => {
  if (saving.value || typeof props.execute !== 'function') return
  saving.value = true
  try {
    const result = await props.execute([...dirRules.value], [...extRules.value])
    // 后端同步重扫完成后返回：成功提示带排除/复活汇总（零值省略）
    const parts = []
    if (result?.excluded) parts.push(`新排除 ${result.excluded} 个文件`)
    if (result?.revived) parts.push(`恢复 ${result.revived} 个文件`)
    ElMessage.success(parts.length > 0 ? `规则已保存并重新扫描（${parts.join('，')}）` : '规则已保存并重新扫描')
    visible.value = false
    emit('saved', result)
  } catch (error) {
    // 非法规则 / 同库扫描进行中拒绝（规则已保存）等经 handleError 按码分流，
    // 弹窗保持打开可改后重试
    handleError('保存排除规则失败: ', error)
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.rules-body {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.rules-hint {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
  line-height: 1.6;
}

.rules-field {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.rules-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.rules-input-row {
  display: flex;
  gap: var(--spacing-xs);
}

.rules-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--spacing-xs);
  min-height: 24px;
}

.rules-tag {
  max-width: 100%;
}

.rules-empty {
  font-size: 12px;
  color: var(--text-placeholder);
}
</style>
