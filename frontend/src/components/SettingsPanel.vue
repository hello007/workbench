<template>
  <el-dialog
    v-model="uiStore.settingsVisible"
    title="设置"
    width="min(960px, 86vw)"
    :close-on-click-modal="true"
    :close-on-press-escape="true"
    class="settings-dialog"
    append-to-body
  >
    <div class="settings-body">
      <!-- 左侧导航栏 -->
      <div class="settings-nav">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          class="settings-nav-item"
          :class="{ 'is-active': activeTab === tab.id }"
          @click="activeTab = tab.id"
        >
          {{ tab.label }}
        </div>
      </div>
      <!-- 右侧内容区 -->
      <div class="settings-content">
        <!-- 通用页 -->
        <div v-show="activeTab === 'general'">
          <div class="settings-section-title">通用</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">主题</div>
              <div class="settings-item-desc">跟随系统主题，或手动切换浅色 / 暗色</div>
            </div>
            <el-radio-group v-model="settingsStore.themeMode" @change="onThemeChange">
              <el-radio value="system">跟随系统</el-radio>
              <el-radio value="light">浅色</el-radio>
              <el-radio value="dark">暗色</el-radio>
            </el-radio-group>
          </div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">GPU 加速</div>
              <div class="settings-item-desc">使用 GPU 渲染 WebView 界面，关闭后可降低 GPU 占用</div>
            </div>
            <el-switch
              v-model="gpuEnabled"
              active-text="开启"
              inactive-text="关闭"
              @change="onGpuChange"
            />
          </div>
          <div v-if="needsRestart" class="settings-restart-hint">
            <el-icon :size="14"><WarningFilled /></el-icon>
            <span>GPU 设置已变更，需重启应用后生效</span>
          </div>
          <!-- 外部应用 -->
          <div class="settings-section-title settings-section-title--spaced">外部应用</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">Obsidian 程序路径</div>
              <div class="settings-item-desc">用 Obsidian 打开时优先使用该可执行文件；留空则尝试系统已注册的 Obsidian</div>
            </div>
            <el-input v-model="obsidianPath" size="small" class="input-w-xl" placeholder="如 C:\Users\me\AppData\Local\Obsidian\Obsidian.exe" @change="onSettingsChange" />
          </div>
          <!-- 外部 diff 工具 -->
          <div class="settings-section-title settings-section-title--spaced">外部 diff 工具</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">预设</div>
              <div class="settings-item-desc">选择常用 diff 工具自动填充路径与参数，均可手动修改</div>
            </div>
            <el-select :model-value="settingsStore.diffToolName" class="diff-tool-preset-select input-w-md" size="small" @change="onDiffToolPresetChange">
              <el-option v-for="(preset, key) in diffToolPresets" :key="key" :label="preset.label" :value="key" />
            </el-select>
          </div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">可执行文件路径</div>
              <div class="settings-item-desc">留空表示未配置，diff 弹窗的「用外部工具打开」按钮将置灰</div>
            </div>
            <el-input v-model="settingsStore.diffToolPath" size="small" class="input-w-xl" placeholder="如 C:\Program Files\WinMerge\WinMergeU.exe" @change="onDiffToolChange" />
          </div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">参数模板</div>
              <div class="settings-item-desc">须包含 {left} 与 {right} 占位符，分别替换为旧版本 / 新版本文件路径</div>
            </div>
            <el-input v-model="settingsStore.diffToolArgs" size="small" class="input-w-xl" placeholder="{left} {right}" @change="onDiffToolChange" />
          </div>
          <!-- 版本与更新 -->
          <div class="settings-section-title settings-section-title--spaced">关于</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">当前版本</div>
              <div class="settings-item-desc">v{{ appVersion }}</div>
            </div>
            <el-button
              size="small"
              type="primary"
              :loading="checkingUpdate"
              @click="handleCheckUpdate"
            >检查更新</el-button>
          </div>
        </div>
        <!-- 终端页 -->
        <div v-show="activeTab === 'terminal'">
          <div class="settings-section-title">终端</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">默认 Shell</div>
              <div class="settings-item-desc">终端面板使用的 Shell 类型</div>
            </div>
            <el-select v-model="defaultShell" class="default-shell-select input-w-md" size="small" @change="onSettingsChange">
              <el-option label="PowerShell" value="powershell" />
              <el-option label="CMD" value="cmd" />
              <el-option label="Git Bash" value="gitbash" />
              <el-option label="WSL" value="wsl" />
            </el-select>
          </div>
          <div v-if="defaultShell === 'gitbash'" class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">Git Bash 路径</div>
              <div class="settings-item-desc">自定义 Git Bash 可执行文件路径</div>
            </div>
            <el-input v-model="gitBashPath" size="small" class="input-w-lg" @change="onSettingsChange" />
          </div>
          <div v-if="defaultShell === 'wsl'" class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">WSL 发行版</div>
              <div class="settings-item-desc">指定 WSL 发行版名称（留空使用默认）</div>
            </div>
            <el-input v-model="wslDistro" size="small" class="input-w-lg" @change="onSettingsChange" />
          </div>
        </div>
        <!-- 搜索页 -->
        <div v-show="activeTab === 'search'">
          <div class="settings-section-title">搜索</div>
          <div class="settings-item settings-item--column">
            <div class="settings-item-info">
              <div class="settings-item-label">排除目录</div>
              <div class="settings-item-desc">搜索时跳过这些目录</div>
            </div>
            <div class="settings-tags">
              <el-tag
                v-for="dir in excludeDirs"
                :key="dir"
                closable
                size="small"
                @close="removeExcludeDir(dir)"
              >{{ dir }}</el-tag>
              <el-input
                v-model="newExcludeDir"
                size="small"
                class="input-w-sm"
                placeholder="添加目录"
                @keyup.enter="addExcludeDir"
              />
              <el-button size="small" @click="addExcludeDir">添加</el-button>
            </div>
          </div>
          <div class="settings-item settings-item--column">
            <div class="settings-item-info">
              <div class="settings-item-label">排除文件</div>
              <div class="settings-item-desc">搜索时跳过这些扩展名的文件</div>
            </div>
            <div class="settings-tags">
              <el-tag
                v-for="file in excludeFiles"
                :key="file"
                closable
                size="small"
                @close="removeExcludeFile(file)"
              >{{ file }}</el-tag>
              <el-input
                v-model="newExcludeFile"
                size="small"
                class="input-w-sm"
                placeholder="如 .log"
                @keyup.enter="addExcludeFile"
              />
              <el-button size="small" @click="addExcludeFile">添加</el-button>
            </div>
          </div>
        </div>
        <!-- 快捷键页 -->
        <div v-show="activeTab === 'shortcuts'" ref="shortcutsTabRef" tabindex="-1" @keydown="handleRecordingKeydown">
          <div class="settings-section-header">
            <div class="settings-section-title">快捷键</div>
            <el-button size="small" @click="resetAllShortcuts">重置所有</el-button>
          </div>
          <div class="shortcut-list">
            <!-- 可自定义快捷键 -->
            <div
              v-for="item in customizableShortcuts"
              :key="item.key"
              class="shortcut-item"
              :class="{ 'shortcut-item--recording': recordingKey === item.key }"
            >
              <div class="shortcut-action">{{ item.action }}</div>
              <div class="shortcut-actions">
                <div
                  class="shortcut-keys shortcut-keys--editable"
                  @click="startRecording(item.key)"
                >
                  <template v-if="recordingKey === item.key">
                    <kbd class="recording-hint">请按下新快捷键...</kbd>
                  </template>
                  <template v-else>
                    <kbd v-for="key in item.keys" :key="key">{{ key }}</kbd>
                  </template>
                </div>
                <el-button
                  v-if="!isDefault(item.key)"
                  size="small"
                  text
                  type="primary"
                  @click="resetShortcut(item.key)"
                >重置</el-button>
              </div>
            </div>

            <!-- 固定快捷键 -->
            <div v-for="s in fixedShortcuts" :key="s.action" class="shortcut-item shortcut-item--fixed">
              <div class="shortcut-action">{{ s.action }}</div>
              <div class="shortcut-keys">
                <kbd v-for="key in s.keys" :key="key">{{ key }}</kbd>
              </div>
            </div>
          </div>
        </div>
        <!-- 网络访问页（浏览器访问通道：桌面同开 HTTP，浏览器打开同一套界面） -->
        <div v-show="activeTab === 'network'">
          <div class="settings-section-title">网络访问</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">浏览器访问</div>
              <div class="settings-item-desc">启动 HTTP 服务，浏览器打开与本机一致的界面（默认开启）</div>
            </div>
            <el-switch
              v-model="webEnabled"
              class="webserve-switch"
              active-text="开启"
              inactive-text="关闭"
              @change="onWebServeToggle"
            />
          </div>
          <div class="settings-item settings-item--column">
            <div class="settings-item-info">
              <div class="settings-item-label">绑定地址</div>
              <div class="settings-item-desc">快捷切换或手输 host:port；默认仅本机可达，改绑 0.0.0.0 会向局域网暴露本机能力</div>
            </div>
            <div class="webserve-bind-row">
              <el-radio-group :model-value="webBindMode" size="small" class="webserve-bind-mode" @change="onBindModeChange">
                <el-radio-button value="local">本地 127.0.0.1</el-radio-button>
                <el-radio-button value="public">公网 0.0.0.0</el-radio-button>
              </el-radio-group>
              <el-input
                v-model="webBindAddress"
                size="small"
                class="input-w-lg webserve-bind-input"
                placeholder="127.0.0.1:36115"
                @change="onBindAddressChange"
              />
            </div>
          </div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">运行状态</div>
              <div class="settings-item-desc">{{ webRunning ? 'HTTP 服务运行中，浏览器可访问' : '服务未运行' }}</div>
            </div>
            <el-tag :type="webRunning ? 'success' : 'info'" size="small">{{ webRunning ? '运行中' : '已停止' }}</el-tag>
          </div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">访问令牌</div>
              <div class="settings-item-desc">浏览器首次访问须输入该令牌，请勿外泄</div>
            </div>
            <div class="webserve-token-actions">
              <span class="webserve-token-text" :title="tokenVisible ? webToken : ''">{{ tokenMask }}</span>
              <el-button size="small" text type="primary" @click="tokenVisible = !tokenVisible">{{ tokenVisible ? '隐藏' : '显示' }}</el-button>
              <el-button size="small" text type="primary" @click="copyToken">复制</el-button>
              <el-button size="small" text type="danger" @click="regenerateToken">重新生成</el-button>
            </div>
          </div>
          <div class="settings-item settings-item--column">
            <div class="settings-item-info">
              <div class="settings-item-label">访问地址</div>
              <div class="settings-item-desc">服务运行时，浏览器打开以下地址；局域网地址仅同网段设备可达。复制链接内含令牌即凭据，请勿外发</div>
            </div>
            <div class="webserve-urls">
              <div v-for="url in webAccessUrls" :key="url" class="webserve-url-row">
                <code class="webserve-url">{{ url }}</code>
                <el-button size="small" text type="primary" @click="copyAccessLink(url)">复制链接</el-button>
              </div>
              <div v-if="!webAccessUrls.length" class="webserve-url-empty">暂无可用地址</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { WarningFilled, Key } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { GetSettings, SaveSettings, GetAppVersion, CheckForUpdate, GetWebServeConfig, SetWebServeConfig, GetWebServeToken, RegenerateWebToken } from '../../wailsjs/go/main/App'
import { handleError } from '../utils/error'
import { setToken } from '../transport/token'
import { useSettingsStore, useUiStore, formatDisplay, isValidShortcut, shortcutFromEvent, DEFAULTS, DIFF_TOOL_PRESETS } from '../store'

const emit = defineEmits(['update-available'])

const uiStore = useUiStore()

const tabs = [
  { id: 'general', label: '通用' },
  { id: 'terminal', label: '终端' },
  { id: 'search', label: '搜索' },
  { id: 'shortcuts', label: '快捷键' },
  { id: 'network', label: '网络访问' }
]

const activeTab = ref('general')
const gpuEnabled = ref(true)
const needsRestart = ref(false)
const defaultShell = ref('powershell')
const gitBashPath = ref('C:\\Program Files\\Git\\bin\\bash.exe')
const wslDistro = ref('')
const obsidianPath = ref('')
const excludeDirs = ref([])
const excludeFiles = ref([])
const newExcludeDir = ref('')
const newExcludeFile = ref('')
const appVersion = ref('')
const checkingUpdate = ref(false)

const settingsStore = useSettingsStore()

// 外部 diff 工具预设表（模板渲染，供 el-select 遍历）
const diffToolPresets = DIFF_TOOL_PRESETS

// 切换预设：填充该预设的默认路径与参数模板并保存（仍可手动修改）。
// 模板用 :model-value 手动赋值（change 时 v-model 已写入新值，取消时无法回退）；
// 已有非当前预设默认值的自定义配置时先确认，防误点覆盖/清空已存配置。
const onDiffToolPresetChange = async (key) => {
  const preset = DIFF_TOOL_PRESETS[key]
  if (!preset) return
  const prevName = settingsStore.diffToolName
  const hasCustomized =
    settingsStore.diffToolPath.trim() !== '' &&
    (settingsStore.diffToolPath !== preset.path || settingsStore.diffToolArgs !== preset.args)
  if (hasCustomized) {
    try {
      await ElMessageBox.confirm(
        '切换预设将覆盖当前已配置的路径与参数模板，是否继续？',
        '切换外部 diff 工具',
        { type: 'warning', confirmButtonText: '覆盖', cancelButtonText: '取消' }
      )
    } catch {
      // 取消：回退下拉选中值
      settingsStore.diffToolName = prevName
      return
    }
  }
  settingsStore.diffToolName = key
  settingsStore.diffToolPath = preset.path
  settingsStore.diffToolArgs = preset.args
  try {
    await settingsStore.saveDiffTool()
  } catch (e) {
    ElMessage.error('保存外部 diff 工具配置失败: ' + (e?.message || String(e)))
  }
}

// 路径 / 参数模板手动修改后保存
const onDiffToolChange = async () => {
  try {
    await settingsStore.saveDiffTool()
  } catch (e) {
    ElMessage.error('保存外部 diff 工具配置失败: ' + (e?.message || String(e)))
  }
}

// ===== 浏览器访问通道（网络访问分区）=====
const webEnabled = ref(false)
const webBindAddress = ref('127.0.0.1:36115')
const webRunning = ref(false)
const webAccessUrls = ref([])
const webToken = ref('')
const tokenVisible = ref(false)

// 令牌展示：隐藏态以圆点遮蔽（等长截断，长 hex 不撑破布局）；无令牌给占位
const tokenMask = computed(() => {
  if (!webToken.value) return '（未生成）'
  return tokenVisible.value ? webToken.value : '•'.repeat(Math.min(webToken.value.length, 24))
})

/**
 * 判断绑定地址 host 部分是否为回环（仅本机可达）。
 * 与后端 isLoopbackBind 判定对齐：127.0.0.1 / localhost / ::1 三种形态。
 */
function isLoopbackAddress(addr) {
  const idx = addr.lastIndexOf(':')
  if (idx <= 0) return false
  const host = addr.slice(0, idx).replace(/^\[|\]$/g, '').toLowerCase()
  return host === '127.0.0.1' || host === 'localhost' || host === '::1'
}

/**
 * 提取绑定地址的端口（校验 1-65535 合法性），非法返回空串。
 */
function portOf(addr) {
  const idx = addr.lastIndexOf(':')
  if (idx <= 0 || idx === addr.length - 1) return ''
  const port = addr.slice(idx + 1)
  const n = Number(port)
  return Number.isInteger(n) && n >= 1 && n <= 65535 ? port : ''
}

/**
 * 校验绑定地址为 host:port 形态且端口合法。
 */
function isValidBindAddress(addr) {
  return portOf(addr) !== ''
}

/**
 * 由绑定地址推导分段选中态：127.0.0.1 / localhost → local，
 * 0.0.0.0 → public，其他（手输自定义地址）→ ''（分段无选中）。
 */
function bindModeOf(addr) {
  const idx = addr.lastIndexOf(':')
  if (idx <= 0) return ''
  const host = addr.slice(0, idx).replace(/^\[|\]$/g, '').toLowerCase()
  if (host === '127.0.0.1' || host === 'localhost') return 'local'
  if (host === '0.0.0.0') return 'public'
  return ''
}

/**
 * 非回环绑定的风险确认弹窗：说明全盘文件/终端/agent 暴露风险与建议隧道。
 * 取消（reject）时由调用方回滚表单状态。
 */
function confirmNonLoopbackRisk() {
  return ElMessageBox.confirm(
    '绑定到非回环地址后，本机全部文件、终端与 AI agent 能力将暴露给局域网内设备，且服务未内置 TLS。远程访问建议使用 Tailscale / frp 等加密隧道。是否继续？',
    '暴露风险确认',
    { type: 'warning', confirmButtonText: '继续', cancelButtonText: '取消' }
  )
}

/**
 * 加载浏览器访问通道配置与令牌（弹窗打开 / 变更刷新时调用）。
 * 失败静默保持当前表单值：网络分区为增强信息，不阻塞其他设置页。
 */
async function loadWebServeConfig() {
  try {
    const cfg = await GetWebServeConfig()
    webEnabled.value = !!cfg?.enabled
    webBindAddress.value = cfg?.bindAddress || '127.0.0.1:36115'
    webRunning.value = !!cfg?.running
    // 防御性过滤 0.0.0.0：监听地址语义浏览器不可直连（后端 webAccessUrls 已排除，
    // 前端兜底——复制链接区不渲染、不产生 0.0.0.0 链接）
    webAccessUrls.value = (cfg?.accessUrls || []).filter(u => !u.includes('//0.0.0.0:'))
  } catch {
    // 保持默认值
  }
  try {
    webToken.value = await GetWebServeToken()
  } catch {
    webToken.value = ''
  }
}

/**
 * 启用开关变更：非回环地址先风险确认，保存后刷新运行状态；
 * 取消确认 / 保存失败均统一从后端刷新真实状态回滚表单。
 */
const onWebServeToggle = async (val) => {
  try {
    if (val && !isLoopbackAddress(webBindAddress.value)) {
      await confirmNonLoopbackRisk()
    }
    await SetWebServeConfig(val, webBindAddress.value)
    ElMessage.success(val ? '浏览器访问已开启' : '浏览器访问已关闭')
  } catch (e) {
    if (e && (e.code || e.message)) {
      handleError('保存浏览器访问设置失败: ', e)
    }
  }
  await loadWebServeConfig()
}

/**
 * 分段选中态由当前绑定地址推导：手输自定义地址 / 后端回滚刷新均自动联动，
 * 无需单独维护状态。
 */
const webBindMode = computed(() => bindModeOf(webBindAddress.value))

/**
 * 保存绑定地址并使运行时生效（改址平滑重启）：开启状态下非回环地址先走风险
 * 确认（本地/公网分段与手输三个入口共用，按目标地址自然分流）；取消确认 /
 * 保存失败均静默（取消 reject 与无 code 错误不走 handleError）并从后端刷新
 * 回滚为已保存地址。
 */
async function saveBindAddress(addr) {
  try {
    if (webEnabled.value && !isLoopbackAddress(addr)) {
      await confirmNonLoopbackRisk()
    }
    await SetWebServeConfig(webEnabled.value, addr)
    ElMessage.success('绑定地址已保存，服务已按新地址生效')
  } catch (e) {
    if (e && (e.code || e.message)) {
      handleError('保存浏览器访问设置失败: ', e)
    }
  }
  await loadWebServeConfig()
}

/**
 * 分段切换：本地直接保存（无需确认）；公网（非回环）经 saveBindAddress 内置
 * 风险确认，取消则不保存。模板用 :model-value 手动赋值（不经 v-model 写入
 * 选中态），确认取消时选中态由地址推导自然回弹，无需手动恢复（参照
 * onDiffToolPresetChange 同款理由）。端口沿用当前绑定地址端口。
 */
async function onBindModeChange(mode) {
  if (mode !== 'local' && mode !== 'public') return
  const port = portOf(webBindAddress.value) || '36115'
  const addr = (mode === 'local' ? '127.0.0.1' : '0.0.0.0') + ':' + port
  if (addr === webBindAddress.value.trim()) return
  await saveBindAddress(addr)
}

/**
 * 绑定地址变更：格式校验 → 保存并使运行时生效（非回环风险确认由
 * saveBindAddress 内置）；格式非法 / 取消 / 失败均从后端刷新回滚为已保存地址。
 */
const onBindAddressChange = async () => {
  const addr = webBindAddress.value.trim()
  if (!isValidBindAddress(addr)) {
    ElMessage.error('绑定地址格式无效，须为 host:port（如 127.0.0.1:36115）')
    await loadWebServeConfig()
    return
  }
  await saveBindAddress(addr)
}

/**
 * 复制令牌到剪贴板。
 */
async function copyToken() {
  if (!webToken.value) return
  try {
    await navigator.clipboard.writeText(webToken.value)
    ElMessage.success('访问令牌已复制')
  } catch (e) {
    ElMessage.error('复制失败: ' + (e?.message || String(e)))
  }
}

/**
 * 复制带 token 的完整访问链接：http://<addr>:<port>/?token=<令牌>，粘贴浏览器即用。
 * 0.0.0.0 双保险拦截（列表加载时已过滤一次，此处防直接传入）；令牌未生成时
 * 警示不复制；成功提示附带安全提示（链接即凭据，勿外发）。
 */
async function copyAccessLink(url) {
  if (!url || url.includes('//0.0.0.0:')) return
  if (!webToken.value) {
    ElMessage.warning('访问令牌未生成，无法复制链接')
    return
  }
  try {
    await navigator.clipboard.writeText(`${url}/?token=${webToken.value}`)
    ElMessage.success('链接已复制；链接内含访问令牌即凭据，请勿外发')
  } catch (e) {
    ElMessage.error('复制失败: ' + (e?.message || String(e)))
  }
}

/**
 * 重新生成令牌：二次确认（已连接浏览器需重输）→ 轮换 → 回填展示新令牌。
 */
const regenerateToken = async () => {
  try {
    await ElMessageBox.confirm(
      '重新生成后，当前令牌立即失效：已连接的浏览器在下次连接时须重新输入新令牌。是否继续？',
      '重新生成访问令牌',
      { type: 'warning', confirmButtonText: '重新生成', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    webToken.value = await RegenerateWebToken()
    // 浏览器（serve）模式下同步更新本页令牌，避免旧令牌轮换后本页后续请求
    // 401 自锁；桌面模式该写入仅落 localStorage，不参与 Wails 通道，无条件调用无副作用
    setToken(webToken.value)
    tokenVisible.value = true
    ElMessage.success('访问令牌已重新生成')
  } catch (e) {
    handleError('重新生成访问令牌失败: ', e)
  }
}

const shortcutsTabRef = ref(null)
const recordingKey = ref(null)
const recordingText = ref('')

const fixedShortcuts = [
  { action: '刷新当前节点', keys: ['F5'] },
  { action: '复制选中项', keys: ['Ctrl', 'C'] },
  { action: '剪切选中项', keys: ['Ctrl', 'X'] },
  { action: '粘贴', keys: ['Ctrl', 'V'] }
]

const shortcutLabels = {
  commandPalette: '打开命令面板',
  toggleTerminal: '切换终端面板',
  rename: '重命名',
  delete: '删除'
}

const customizableShortcuts = computed(() => [
  { action: '打开命令面板', key: 'commandPalette', keys: formatDisplay(settingsStore.shortcutCommandPalette), customizable: true },
  { action: '切换终端面板', key: 'toggleTerminal', keys: formatDisplay(settingsStore.shortcutToggleTerminal), customizable: true },
  { action: '重命名', key: 'rename', keys: formatDisplay(settingsStore.shortcutRename), customizable: true },
  { action: '删除', key: 'delete', keys: formatDisplay(settingsStore.shortcutDelete), customizable: true }
])

function startRecording(key) {
  recordingKey.value = key
  recordingText.value = ''
  nextTick(() => {
    shortcutsTabRef.value?.focus()
  })
}

function cancelRecording() {
  recordingKey.value = null
  recordingText.value = ''
}

function isDefault(key) {
  if (key === 'commandPalette') return settingsStore.shortcutCommandPalette === DEFAULTS.commandPalette
  if (key === 'toggleTerminal') return settingsStore.shortcutToggleTerminal === DEFAULTS.toggleTerminal
  if (key === 'rename') return settingsStore.shortcutRename === DEFAULTS.rename
  if (key === 'delete') return settingsStore.shortcutDelete === DEFAULTS.delete
  return true
}

function resetShortcut(key) {
  if (key === 'commandPalette') settingsStore.shortcutCommandPalette = DEFAULTS.commandPalette
  else if (key === 'toggleTerminal') settingsStore.shortcutToggleTerminal = DEFAULTS.toggleTerminal
  else if (key === 'rename') settingsStore.shortcutRename = DEFAULTS.rename
  else if (key === 'delete') settingsStore.shortcutDelete = DEFAULTS.delete
  settingsStore.saveShortcuts()
}

function resetAllShortcuts() {
  settingsStore.shortcutCommandPalette = DEFAULTS.commandPalette
  settingsStore.shortcutToggleTerminal = DEFAULTS.toggleTerminal
  settingsStore.shortcutRename = DEFAULTS.rename
  settingsStore.shortcutDelete = DEFAULTS.delete
  settingsStore.saveShortcuts()
}

function handleRecordingKeydown(e) {
  if (!recordingKey.value) return

  // 录制模式下拦截所有按键，阻止冒泡到 Home.vue 的全局处理器
  e.preventDefault()
  e.stopPropagation()

  if (e.key === 'Escape') {
    cancelRecording()
    return true
  }

  const shortcut = shortcutFromEvent(e)
  if (!shortcut || !isValidShortcut(shortcut)) return true

  const conflict = settingsStore.checkConflict(shortcut, recordingKey.value)
  if (conflict) {
    ElMessage.warning(`快捷键冲突：与"${shortcutLabels[conflict.key] || conflict.key}"相同`)
    return true
  }

  // 固定快捷键（F5 / Ctrl+C/X/V）不可覆盖
  const fixedValues = fixedShortcuts.map(s => s.keys.join('+'))
  if (fixedValues.includes(shortcut)) {
    ElMessage.warning(`快捷键冲突：与固定快捷键 "${shortcut}" 相同，不可覆盖`)
    return true
  }

  if (recordingKey.value === 'commandPalette') {
    settingsStore.shortcutCommandPalette = shortcut
  } else if (recordingKey.value === 'toggleTerminal') {
    settingsStore.shortcutToggleTerminal = shortcut
  } else if (recordingKey.value === 'rename') {
    settingsStore.shortcutRename = shortcut
  } else if (recordingKey.value === 'delete') {
    settingsStore.shortcutDelete = shortcut
  }

  recordingKey.value = null
  recordingText.value = ''
  settingsStore.saveShortcuts()
  return true
}

// 弹窗打开时加载设置
watch(() => uiStore.settingsVisible, async (val) => {
  if (val) {
    await loadSettings()
  }
})

onMounted(async () => {
  await loadSettings()
})

async function loadSettings() {
  try {
    const settings = await GetSettings()
    gpuEnabled.value = !settings.gpuDisabled
    defaultShell.value = settings.defaultShell || 'powershell'
    gitBashPath.value = settings.gitBashPath || 'C:\\Program Files\\Git\\bin\\bash.exe'
    wslDistro.value = settings.wslDistro || ''
    obsidianPath.value = settings.obsidianPath || ''
    excludeDirs.value = settings.searchExcludeDirs || []
    excludeFiles.value = settings.searchExcludeFiles || []
    await settingsStore.loadShortcuts()
    await settingsStore.loadDiffTool()
  } catch {
    gpuEnabled.value = true
  }
  // 浏览器访问通道配置（独立加载，失败静默不阻塞其他设置页）
  await loadWebServeConfig()
  // 加载版本号
  try {
    appVersion.value = await GetAppVersion()
  } catch {
    appVersion.value = 'dev'
  }
}

async function handleCheckUpdate() {
  checkingUpdate.value = true
  try {
    const info = await CheckForUpdate()
    if (!info || !info.hasUpdate) {
      ElMessage.success(`当前已是最新版本 v${info?.currentVer || appVersion.value}`)
    } else {
      // 有新版本，通知父组件弹出更新弹窗
      emit('update-available', info)
    }
  } catch (e) {
    ElMessage.error('检查更新失败: ' + (e.message || String(e)))
  } finally {
    checkingUpdate.value = false
  }
}

const addExcludeDir = () => {
  const val = newExcludeDir.value.trim()
  if (val && !excludeDirs.value.includes(val)) {
    excludeDirs.value.push(val)
    onSettingsChange()
  }
  newExcludeDir.value = ''
}

const removeExcludeDir = (tag) => {
  excludeDirs.value = excludeDirs.value.filter(d => d !== tag)
  onSettingsChange()
}

const addExcludeFile = () => {
  const val = newExcludeFile.value.trim()
  if (val && !excludeFiles.value.includes(val)) {
    excludeFiles.value.push(val)
    onSettingsChange()
  }
  newExcludeFile.value = ''
}

const removeExcludeFile = (tag) => {
  excludeFiles.value = excludeFiles.value.filter(f => f !== tag)
  onSettingsChange()
}

// 通用设置保存（合并写）：以磁盘现有设置为基底，仅覆盖本面板管理的通用字段。
// themeMode / diffTool 三字段不经此处写回（各走 saveTheme / saveDiffTool 合并写）——
// 否则 loadSettings 启动失败时 store 留默认值，任一次通用保存都会把磁盘配置静默清空。
async function saveGeneralSettings(gpuDisabled) {
  const settings = await GetSettings()
  settings.gpuDisabled = gpuDisabled
  settings.defaultShell = defaultShell.value
  settings.gitBashPath = gitBashPath.value
  settings.wslDistro = wslDistro.value
  settings.obsidianPath = obsidianPath.value
  settings.searchExcludeDirs = excludeDirs.value
  settings.searchExcludeFiles = excludeFiles.value
  await SaveSettings(settings)
}

const onGpuChange = async (val) => {
  try {
    await saveGeneralSettings(!val)
    needsRestart.value = true
  } catch {
    gpuEnabled.value = !gpuEnabled.value
  }
}

const onSettingsChange = async () => {
  try {
    await saveGeneralSettings(!gpuEnabled.value)
  } catch {
    // 回滚
  }
}

// 主题切换：走 store 合并写（GetSettings → 覆盖 themeMode → SaveSettings），
// 保留其他字段；切换即生效（App.vue watch resolvedTheme 已应用 dark class）
const onThemeChange = async () => {
  try {
    await settingsStore.saveTheme()
  } catch {
    // 保存失败不回滚 UI 状态：内存态主题已切换并即时生效，下次启动回退
  }
}
</script>

<style scoped>
/* 弹窗内容区背景
 * margin 负值与 el-dialog__body padding（同为 --spacing-lg）配对抵消，
 * 使 settings-body 填满 dialog body padding-box（边缘到边缘） */
.settings-body {
  display: flex;
  height: min(560px, 78vh);
  margin: calc(-1 * var(--spacing-lg));
}

/* 左侧导航栏 */
.settings-nav {
  width: 200px;
  flex-shrink: 0;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border-color);
  padding: var(--spacing-sm) 0;
}

/* nav-item：position:relative 供 ::before 指示条定位；
 * 指示条 left:0 贴 item 左缘——.settings-body 虽 margin 负值但无 overflow:hidden，
 * 不会裁剪溢出（参照 ActivityBar 同款约束，禁用负 left） */
.settings-nav-item {
  position: relative;
  padding: var(--spacing-sm) var(--spacing-lg);
  font-size: 14px;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.settings-nav-item:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

/* active：左侧指示条 + 主色填充；
 * hover 不再额外反馈（避免与 active 状态重复，参照 ActivityBar 去冗余） */
.settings-nav-item.is-active {
  color: var(--primary-color);
  background: var(--primary-bg);
}

.settings-nav-item.is-active:hover {
  background: var(--primary-bg);
  color: var(--primary-color);
}

.settings-nav-item.is-active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  background: var(--primary-light);
}

/* 右侧内容区 */
.settings-content {
  flex: 1;
  padding: var(--spacing-lg);
  overflow-y: auto;
  background: var(--bg-secondary);
}

/* section-title：对齐全局 h3 字重梯度（600 + 负字距）；
 * margin-bottom 取 sm（布局紧凑化：标题与内容贴紧，段间留白由卡片自身 padding 补足） */
.settings-section-title {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--text-primary);
  margin-bottom: var(--spacing-sm);
}

/* 段落间距：紧凑化后取 md（段落分组间仍保留一档呼吸感，标题内聚用 sm） */
.settings-section-title--spaced {
  margin-top: var(--spacing-md);
}

/* settings-item 卡片：默认 --shadow-sm，hover 升 --shadow-md（参照 DashboardView .card 模式）；
 * padding 上下收至 sm、左右保持 md（紧凑化：卡片内敛），卡片间距收至 sm */
.settings-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-color);
  margin-bottom: var(--spacing-sm);
  box-shadow: var(--shadow-sm);
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.settings-item:hover {
  border-color: var(--primary-light);
  box-shadow: var(--shadow-md);
}

.settings-item-info {
  flex: 1;
  min-width: 0;
  margin-right: var(--spacing-md);
}

.settings-item-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-primary);
}

.settings-item-desc {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: var(--spacing-xs);
}

/* restart-hint：用 --warning-color + color-mix 透明度叠加
 * （替代原 rgba(230,162,60,…) 硬编码，纯变量驱动，随主题切换） */
.settings-restart-hint {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  background: color-mix(in srgb, var(--warning-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--warning-color) 35%, transparent);
  border-radius: var(--radius-md);
  color: var(--warning-color);
  font-size: 12px;
}

/* 快捷键空状态 */
.settings-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px var(--spacing-lg);
  color: var(--text-tertiary);
}

.settings-empty p {
  margin-top: var(--spacing-sm);
  font-size: 14px;
}

.settings-item--column {
  flex-direction: column;
  align-items: flex-start;
  gap: var(--spacing-sm);
}

.settings-tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
  width: 100%;
}

/* input/select 宽度语义类（替代行内 style="width:..."）
 * 归并位：sm=120 / md=140(140 与 180 归并) / lg=240 / xl=280，保持视觉等义 */
.input-w-sm { width: 120px; }
.input-w-md { width: 140px; }
.input-w-lg { width: 240px; }
.input-w-xl { width: 280px; }

/* ===== 网络访问分区 ===== */

/* 分段切换 + 自定义地址输入横排（窄窗换行防挤压） */
.webserve-bind-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
  width: 100%;
}

/* 令牌操作区：遮蔽文本 + 显示/复制/重新生成横向排布 */
.webserve-token-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
}

/* 令牌文本：等宽字体键帽风（对齐 kbd 字体族约定），超长截断防撑破布局 */
.webserve-token-text {
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 2px 8px;
  font-size: 12px;
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  color: var(--text-primary);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
}

/* 访问地址列表：纵向排布的地址行（地址 code + 复制链接按钮横排） */
.webserve-urls {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  width: 100%;
}

.webserve-url-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  width: 100%;
}

.webserve-url {
  padding: 4px 8px;
  font-size: 12px;
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  width: fit-content;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.webserve-url-empty {
  font-size: 12px;
  color: var(--text-tertiary);
}

/* 快捷键列表 */
.shortcut-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

/* 快捷键列表标题行 */
.settings-section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--spacing-sm);
}

.settings-section-header .settings-section-title {
  margin-bottom: 0;
}

.shortcut-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.shortcut-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-color);
  transition: border-color var(--transition-fast);
}

.shortcut-item:hover {
  border-color: var(--primary-light);
}

.shortcut-action {
  font-size: 14px;
  color: var(--text-primary);
}

.shortcut-keys {
  display: flex;
  gap: var(--spacing-xs);
}

/* kbd 键帽：Geist 字体前缀 + 变量驱动阴影（禁硬编码色值）；
 * padding 收紧（2px 6px）贴合 12px 字号，消除键帽与文字间的空旷感 */
.shortcut-keys kbd {
  display: inline-block;
  padding: 2px 6px;
  font-size: 12px;
  font-family: 'Geist', 'Consolas', 'Monaco', monospace;
  color: var(--text-primary);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-sm);
  box-shadow: 0 1px 0 var(--border-color);
}

.shortcut-keys--editable {
  cursor: pointer;
  padding: var(--spacing-xs) var(--spacing-sm);
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast);
}

.shortcut-keys--editable:hover {
  background: var(--primary-bg);
}

.shortcut-item--recording {
  border-color: var(--primary-color) !important;
  background: var(--primary-bg);
}

.shortcut-item--fixed .shortcut-keys kbd {
  opacity: 0.7;
}

.recording-hint {
  color: var(--primary-color);
  font-style: italic;
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
  animation: blink 1.2s ease-in-out infinite;
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>

<style>
/* 全局：el-dialog 主题适配（浅色/暗色经 CSS 变量随 resolvedTheme 切换）
 * 圆角 --radius-lg（容器外圈较软，内卡片 --radius-md，外松内紧层级） */
.settings-dialog .el-dialog {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-lg);
}

.settings-dialog .el-dialog__header {
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-color);
  border-radius: var(--radius-lg) var(--radius-lg) 0 0;
  padding: var(--spacing-md) var(--spacing-lg);
}

.settings-dialog .el-dialog__title {
  color: var(--text-primary);
  font-size: 16px;
  font-weight: 600;
}

.settings-dialog .el-dialog__headerbtn .el-dialog__close {
  color: var(--text-tertiary);
}

.settings-dialog .el-dialog__headerbtn:hover .el-dialog__close {
  color: var(--text-primary);
}

.settings-dialog .el-dialog__body {
  padding: var(--spacing-lg);
}

/* el-overlay 蓝灰着色（禁纯黑）——:has() 仅作用于含 settings-dialog 的遮罩，
 * 不影响其他弹窗；亮色 Slate-900 着色，暗色更深一档 Slate-950 着色 */
.el-overlay:has(.settings-dialog) {
  background-color: rgba(15, 23, 42, 0.5);
}

html.dark .el-overlay:has(.settings-dialog) {
  background-color: rgba(2, 6, 23, 0.6);
}
</style>
