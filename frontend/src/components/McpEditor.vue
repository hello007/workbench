<template>
  <div class="mcp-editor">
    <!-- server 列表：v-for 遍历 mcpServers map，server 为 value（reactive 对象），
         v-model 直接绑 server.xxx 属性；name 是 map key，改时删旧加新 -->
    <div v-for="(server, name) in serversMap" :key="name" class="server-card">
      <div class="server-head">
        <el-input
          :model-value="name"
          placeholder="server 标识（如 fs、tencent）"
          class="col-name"
          @change="(v) => onNameChange(name, v)"
        />
        <el-select
          :model-value="server.type || 'http'"
          class="col-type"
          @update:model-value="(v) => onTypeChange(server, v)"
        >
          <el-option label="HTTP" value="http" />
          <el-option label="stdio" value="stdio" />
        </el-select>
        <el-button type="danger" link @click="removeServer(name)">删除</el-button>
      </div>

      <!-- http 分支：url + headers -->
      <template v-if="(server.type || 'http') === 'http'">
        <el-input
          v-model="server.url"
          placeholder="server URL（如 https://mcp.example.com/v1）"
        />
        <div class="sub-label">请求头</div>
        <EnvEditor v-model="server.headers" />
      </template>

      <!-- stdio 分支：command + args + env -->
      <template v-else>
        <el-input
          v-model="server.command"
          placeholder="启动命令（如 npx、python）"
        />
        <el-input
          :model-value="(server.args || []).join(', ')"
          placeholder="参数，逗号分隔（如 -y, @modelcontextprotocol/server-filesystem, D:\\docs）"
          @change="(v) => (server.args = splitArgs(v))"
        />
        <div class="sub-label">环境变量（运行时走 $ENV:VAR 展开）</div>
        <EnvEditor v-model="server.env" />
      </template>
    </div>

    <el-button size="small" @click="addServer">+ 新增 server</el-button>

    <!-- 校验告警：http 须 url、stdio 须 command；交叉字段忽略不报错 -->
    <div v-if="validationErrors.length" class="warn-box">
      <div v-for="e in validationErrors" :key="e" class="warn-item">⚠ {{ e }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import EnvEditor from './EnvEditor.vue'

// v-model 绑 AiMcpConfig（{mcpServers: {name: AiMcpServer}}）。父传入 reactive 对象，
// 子组件改 server 属性 / 增删 map key 经原对象 reactive 触发更新
const model = defineModel({ type: Object, default: null })

const serversMap = computed(() => model.value?.mcpServers || {})

const ensureConfig = () => {
  if (!model.value) model.value = { mcpServers: {} }
  if (!model.value.mcpServers) model.value.mcpServers = {}
  return model.value.mcpServers
}

// name 改（map key 重命名）：删旧加新，server 引用保持不变
const onNameChange = (oldName, newName) => {
  const m = ensureConfig()
  const nk = (newName || '').trim()
  if (oldName === nk || !nk) return
  if (m[nk] !== undefined) return // 重名忽略，避免覆盖
  const server = m[oldName]
  delete m[oldName]
  m[nk] = server
}

const onTypeChange = (server, v) => {
  server.type = v
}

const splitArgs = (v) => v.split(/[,，]/).map((s) => s.trim()).filter(Boolean)

const addServer = () => {
  const m = ensureConfig()
  let base = 'server'
  let i = 1
  while (m[base] !== undefined) base = `server${i++}`
  m[base] = { type: 'http', url: '', headers: {} }
}

const removeServer = (name) => {
  if (!model.value?.mcpServers) return
  delete model.value.mcpServers[name]
}

// 校验：http 须 url、stdio 须 command；交叉字段（http 配了 command 等）忽略不报错
const validationErrors = computed(() => {
  const errs = []
  const m = model.value?.mcpServers || {}
  for (const [name, server] of Object.entries(m)) {
    const type = server.type || 'http'
    if (type === 'http' && !(server.url || '').trim()) {
      errs.push(`mcp server「${name}」http 类型须填 url`)
    }
    if (type === 'stdio' && !(server.command || '').trim()) {
      errs.push(`mcp server「${name}」stdio 类型须填 command`)
    }
  }
  return errs
})

defineExpose({ validationErrors })
</script>

<style scoped>
.server-card {
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  padding: 10px;
  margin-bottom: 10px;
}
.server-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.col-name {
  flex: 1;
}
.col-type {
  width: 120px;
}
.sub-label {
  font-size: 12px;
  color: var(--text-tertiary);
  margin: 8px 0 4px;
}
.warn-box {
  margin-top: 8px;
  padding: 6px 10px;
  background: #fef0f0;
  border-radius: var(--radius-sm);
  font-size: 12px;
  color: #f56c6c;
}
.warn-item {
  line-height: 1.6;
}
</style>
