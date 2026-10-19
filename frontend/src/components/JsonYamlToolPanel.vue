<template>
  <div class="json-yaml-tool-panel">
    <div class="tool-panel-titlebar">
      <span class="tool-panel-title">JSON / YAML 工具</span>
      <span class="tool-panel-titlebar-hint">校验 / 格式化 / 压缩 / 互转，纯前端实现</span>
    </div>

    <!-- 格式类型 + JSON 缩进：显式选择而非自动探测，避免合法值在两种格式间歧义 -->
    <div class="jt-toolbar">
      <el-select v-model="formatType" size="small" class="jt-type-select">
        <el-option label="JSON" :value="FORMAT_JSON" />
        <el-option label="YAML" :value="FORMAT_YAML" />
      </el-select>
      <el-select
        v-if="formatType === FORMAT_JSON"
        v-model="jsonIndent"
        size="small"
        class="jt-indent-select"
      >
        <el-option label="缩进 2" :value="2" />
        <el-option label="缩进 4" :value="4" />
      </el-select>
      <span class="jt-toolbar-actions">
        <el-button size="small" @click="validateOnly">校验</el-button>
        <el-button size="small" @click="formatText">格式化</el-button>
        <el-button size="small" @click="minifyText">压缩</el-button>
        <el-button v-if="formatType === FORMAT_JSON" size="small" @click="convertToYaml">
          转 YAML
        </el-button>
        <el-button v-else size="small" @click="convertToJson">转 JSON</el-button>
      </span>
    </div>

    <el-input
      v-model="sourceText"
      type="textarea"
      :rows="16"
      :placeholder="formatType === FORMAT_JSON
        ? '粘贴 JSON 文本，校验/格式化/压缩或转为 YAML'
        : '粘贴 YAML 文本，校验/格式化或转为 JSON'"
      class="jt-textarea"
      spellcheck="false"
    />

    <!-- 结果行：成功（含格式摘要）或错误（带行号定位） -->
    <div v-if="result" class="jt-result" :class="result.ok ? 'jt-result-ok' : 'jt-result-err'">
      <template v-if="result.ok">{{ result.message }}</template>
      <template v-else>
        <span v-if="result.line !== null">第 {{ result.line }} 行：</span>{{ result.message }}
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import yaml from 'js-yaml'

const FORMAT_JSON = 'json'
const FORMAT_YAML = 'yaml'

const formatType = ref(FORMAT_JSON)
const jsonIndent = ref(2)
const sourceText = ref('')
// 操作结果：{ ok, message, line }（line 仅错误态，1 起始；null = 无法定位）
const result = ref(null)

// 切换格式类型时清空结果：上一格式的校验结论对另一格式无意义
watch(formatType, () => {
  result.value = null
})

/**
 * 解析当前文本。JSON 用 JSON.parse（错误 message 含字符位置，换算行号）；
 * YAML 用 js-yaml load（异常 mark.line 为 0 起始行号）。
 * 多文档 YAML 直接拒绝：无法映射为单个 JSON 值，静默取首文档会丢数据。
 */
const parseCurrent = (text) => {
  if (formatType.value === FORMAT_JSON) {
    return { value: JSON.parse(text) }
  }
  const docs = yaml.loadAll(text)
  if (docs.length > 1) {
    const err = new Error(`YAML 包含 ${docs.length} 个文档（--- 分隔），无法处理为单值，请拆分后重试`)
    err.line = null
    throw err
  }
  return { value: docs[0] }
}

/** 从解析异常提取行号与可读消息（JSON 位置换算行 / YAML 直接读 mark） */
const describeError = (e) => {
  if (formatType.value === FORMAT_JSON) {
    const m = /position (\d+)/.exec(e.message || '')
    let line = null
    if (m) {
      line = sourceText.value.slice(0, Number(m[1])).split('\n').length
    }
    return { message: e.message, line }
  }
  return { message: e.reason || e.message, line: e.mark ? e.mark.line + 1 : null }
}

const fail = (e) => {
  const { message, line } = describeError(e)
  result.value = { ok: false, message, line }
}

const validateOnly = () => {
  const text = sourceText.value
  if (!text.trim()) {
    result.value = { ok: false, message: '内容为空，请先粘贴文本', line: null }
    return
  }
  try {
    parseCurrent(text)
    result.value = { ok: true, message: `校验通过：合法${formatType.value === FORMAT_JSON ? ' JSON' : ' YAML'}` }
  } catch (e) {
    fail(e)
  }
}

const formatText = () => {
  try {
    const { value } = parseCurrent(sourceText.value)
    const out = formatType.value === FORMAT_JSON
      ? JSON.stringify(value, null, jsonIndent.value)
      : yaml.dump(value, { lineWidth: -1 })
    sourceText.value = out
    result.value = { ok: true, message: '格式化完成' }
  } catch (e) {
    fail(e)
  }
}

const minifyText = () => {
  try {
    const { value } = parseCurrent(sourceText.value)
    // JSON 压缩 = 序列化去空白；YAML 无对应压缩语义（流式风格即最简），按 JSON 序列化输出
    sourceText.value = JSON.stringify(value)
    result.value = { ok: true, message: '压缩完成' }
  } catch (e) {
    fail(e)
  }
}

/**
 * JSON → YAML。JSON 无注释/锚点，结构语义无损。
 */
const convertToYaml = () => {
  try {
    const { value } = parseCurrent(sourceText.value)
    sourceText.value = yaml.dump(value, { lineWidth: -1 })
    formatType.value = FORMAT_YAML
    result.value = { ok: true, message: '已转换为 YAML' }
  } catch (e) {
    fail(e)
  }
}

/**
 * YAML → JSON。转换前显式声明有损面：注释与锚点别名会被丢弃/展开，
 * 用户可取消；多文档已在 parseCurrent 拒绝，不会静默取首文档。
 */
const convertToJson = () => {
  const text = sourceText.value
  const lossy = []
  // token 级检测而非行首：锚点/别名/注释可出现在任意位置（如 `base: &b`、`a: 1 # 备注`）
  if (/(?:^|\s)#/m.test(text)) lossy.push('注释')
  if (/(?:^|\s)[&*]\S+/m.test(text)) lossy.push('锚点/别名引用')
  if (lossy.length > 0) {
    ElMessageBox.confirm(
      `检测到 YAML ${lossy.join('与')}，转换为 JSON 后将丢失（锚点引用会被展开为实际值）。确定继续？`,
      '转换可能有损',
      {
        type: 'warning',
        confirmButtonText: '继续转换',
        cancelButtonText: '取消'
      }
    ).then(() => {
      doConvertToJson(text)
    }).catch(() => {})
    return
  }
  doConvertToJson(text)
}

const doConvertToJson = (text) => {
  try {
    const { value } = parseCurrent(text)
    sourceText.value = JSON.stringify(value, null, jsonIndent.value)
    formatType.value = FORMAT_JSON
    result.value = { ok: true, message: '已转换为 JSON' }
  } catch (e) {
    fail(e)
  }
}
</script>

<style scoped>
.jt-toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-md);
}

.jt-indent-select {
  width: 104px;
}

.jt-toolbar-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.jt-textarea :deep(.el-textarea__inner) {
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.6;
}

.jt-result {
  margin-top: var(--spacing-sm);
  font-size: 13px;
  line-height: 1.5;
  word-break: break-all;
}

.jt-result-ok {
  color: var(--success-color, var(--el-color-success));
}

.jt-result-err {
  color: var(--danger-color, var(--el-color-danger));
}
</style>
