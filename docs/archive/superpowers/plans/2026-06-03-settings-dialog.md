# 设置面板弹窗化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将设置面板从左侧内嵌面板改为 el-dialog 弹窗，左右双栏布局

**Architecture:** SettingsPanel.vue 重写为 el-dialog + 左右布局组件；Home.vue 移除内嵌渲染改为弹窗模式；ActivityBar.vue 设置图标点击改为发射独立事件打开弹窗

**Tech Stack:** Vue 3 Composition API, Element Plus (el-dialog, el-switch, el-select, el-input)

---

### Task 1: ActivityBar 新增设置弹窗事件

**Files:**
- Modify: `frontend/src/components/ActivityBar.vue:9,37,39-43`

将设置图标点击从 `update:modelValue` 改为发射独立事件 `openSettings`，使其不再切换 `activePanel`。

- [ ] **Step 1: 修改 ActivityBar.vue 事件定义和模板**

`ActivityBar.vue` 改动要点：

1. `defineEmits` 新增 `openSettings` 事件
2. 设置图标点击时发射 `openSettings` 而非 `update:modelValue`
3. 从 `panels` 数组中移除 `settings` 项（设置不再作为面板切换）

```vue
<!-- ActivityBar.vue 修改后 -->
<template>
  <div class="activity-bar">
    <div
      v-for="item in panels"
      :key="item.id"
      class="activity-bar-item"
      :class="{ 'is-active': modelValue === item.id }"
      @click="$emit('update:modelValue', item.id)"
    >
      <el-icon :size="20">
        <component :is="item.icon" />
      </el-icon>
    </div>
    <!-- 设置图标（不切换面板，直接开弹窗） -->
    <div
      class="activity-bar-item"
      @click="$emit('openSettings')"
    >
      <el-icon :size="20">
        <Setting />
      </el-icon>
    </div>
    <!-- 终端图标（底部） -->
    <div class="activity-bar-spacer"></div>
    <div
      class="activity-bar-item"
      :class="{ 'is-active': terminalActive }"
      @click="$emit('toggleTerminal')"
    >
      <el-icon :size="20">
        <Monitor />
      </el-icon>
    </div>
  </div>
</template>

<script setup>
import { Folder, SetUp, Setting, Monitor } from '@element-plus/icons-vue'

defineProps({
  modelValue: { type: String, default: 'directory' },
  terminalActive: { type: Boolean, default: false }
})

defineEmits(['update:modelValue', 'toggleTerminal', 'openSettings'])

const panels = [
  { id: 'directory', icon: Folder, label: '工作目录' },
  { id: 'toolbox', icon: SetUp, label: '工具箱' }
]
</script>
```

样式部分不变。

- [ ] **Step 2: 运行前端测试确认无回归**

Run: `cd frontend && npx vitest run`
Expected: 71 passed（26 failed 为预先存在，与本次改动无关）

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/ActivityBar.vue
git commit -m "refactor(settings): ActivityBar设置图标改为发射openSettings事件"
```

---

### Task 2: Home.vue 移除内嵌 SettingsPanel，改为弹窗模式

**Files:**
- Modify: `frontend/src/views/Home.vue:26-29,100,144`

- [ ] **Step 1: 修改 Home.vue 模板和脚本**

改动要点：

1. 模板中移除内嵌 `<SettingsPanel>`，改为 `<SettingsPanel v-model:visible="settingsVisible" />`（放在 `.home` 根 div 内最外层）
2. 新增 `settingsVisible` ref
3. ActivityBar 监听 `@open-settings` 事件打开弹窗
4. 移除 `closeToolbox` 中对 `settings` 的判断
5. 移除 `activePanel === 'settings'` 相关逻辑

模板改动 — 移除第 26-29 行内嵌 SettingsPanel：

```vue
<!-- 删除 -->
<SettingsPanel
  v-show="activePanel === 'settings'"
  @close="activePanel = 'directory'"
/>
```

模板改动 — 在 `.home` 根 div 内最外层添加弹窗挂载：

```vue
<!-- 在 </div> 闭合 .home 之前添加 -->
<SettingsPanel v-model:visible="settingsVisible" />
```

模板改动 — ActivityBar 新增事件监听（第 4 行）：

```vue
<ActivityBar
  v-model="activePanel"
  :terminal-active="terminalVisible"
  @toggle-terminal="toggleTerminal"
  @open-settings="settingsVisible = true"
/>
```

脚本改动 — 新增状态（在终端状态区域后）：

```js
// ---- 设置弹窗状态 ----
const settingsVisible = ref(false)
```

脚本改动 — closeToolbox 移除 settings 判断：

```js
const closeToolbox = () => {
  if (activePanel.value === 'toolbox') {
    activePanel.value = 'directory'
  }
}
```

- [ ] **Step 2: 运行前端测试确认无回归**

Run: `cd frontend && npx vitest run`
Expected: 71 passed

- [ ] **Step 3: 提交**

```bash
git add frontend/src/views/Home.vue
git commit -m "refactor(settings): Home.vue移除内嵌SettingsPanel，改为弹窗模式挂载"
```

---

### Task 3: 重写 SettingsPanel.vue 为 el-dialog 弹窗

**Files:**
- Rewrite: `frontend/src/components/SettingsPanel.vue`

这是核心任务，将 SettingsPanel 从内嵌面板重写为 el-dialog 弹窗，内部左右双栏布局。

- [ ] **Step 1: 重写 SettingsPanel.vue**

完整新文件内容：

```vue
<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="设置"
    width="760px"
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
        </div>
        <!-- 终端页 -->
        <div v-show="activeTab === 'terminal'">
          <div class="settings-section-title">终端</div>
          <div class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">默认 Shell</div>
              <div class="settings-item-desc">终端面板使用的 Shell 类型</div>
            </div>
            <el-select v-model="defaultShell" size="small" style="width: 140px;" @change="onSettingsChange">
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
            <el-input v-model="gitBashPath" size="small" style="width: 240px;" @change="onSettingsChange" />
          </div>
          <div v-if="defaultShell === 'wsl'" class="settings-item">
            <div class="settings-item-info">
              <div class="settings-item-label">WSL 发行版</div>
              <div class="settings-item-desc">指定 WSL 发行版名称（留空使用默认）</div>
            </div>
            <el-input v-model="wslDistro" size="small" style="width: 240px;" @change="onSettingsChange" />
          </div>
        </div>
        <!-- 快捷键页 -->
        <div v-show="activeTab === 'shortcuts'">
          <div class="settings-section-title">快捷键</div>
          <div class="settings-empty">
            <el-icon :size="32" color="#555"><Keyboard /></el-icon>
            <p>暂无可配置快捷键</p>
          </div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, watch, onMounted } from 'vue'
import { WarningFilled, Keyboard } from '@element-plus/icons-vue'
import { GetSettings, SaveSettings } from '../../wailsjs/go/main/App'

const props = defineProps({
  visible: { type: Boolean, default: false }
})

defineEmits(['update:visible'])

const tabs = [
  { id: 'general', label: '通用' },
  { id: 'terminal', label: '终端' },
  { id: 'shortcuts', label: '快捷键' }
]

const activeTab = ref('general')
const gpuEnabled = ref(true)
const needsRestart = ref(false)
const defaultShell = ref('powershell')
const gitBashPath = ref('C:\\Program Files\\Git\\bin\\bash.exe')
const wslDistro = ref('')

// 弹窗打开时加载设置
watch(() => props.visible, async (val) => {
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
  } catch {
    gpuEnabled.value = true
  }
}

const onGpuChange = async (val) => {
  try {
    await SaveSettings({
      gpuDisabled: !val,
      defaultShell: defaultShell.value,
      gitBashPath: gitBashPath.value,
      wslDistro: wslDistro.value
    })
    needsRestart.value = true
  } catch {
    gpuEnabled.value = !gpuEnabled.value
  }
}

const onSettingsChange = async () => {
  try {
    await SaveSettings({
      gpuDisabled: !gpuEnabled.value,
      defaultShell: defaultShell.value,
      gitBashPath: gitBashPath.value,
      wslDistro: wslDistro.value
    })
  } catch {
    // 回滚
  }
}
</script>

<style scoped>
/* 弹窗内容区背景 */
.settings-body {
  display: flex;
  height: 420px;
  margin: -20px;  /* 抵消 el-dialog 默认 padding */
}

/* 左侧导航栏 */
.settings-nav {
  width: 200px;
  flex-shrink: 0;
  background: #252526;
  border-right: 1px solid #3c3c3c;
  padding: 12px 0;
}

.settings-nav-item {
  padding: 10px 20px;
  font-size: 14px;
  color: #888;
  cursor: pointer;
  border-left: 2px solid transparent;
  transition: all 0.15s;
}

.settings-nav-item:hover {
  background: #2d2d2d;
  color: #ccc;
}

.settings-nav-item.is-active {
  color: #ccc;
  background: #094771;
  border-left-color: #409eff;
}

/* 右侧内容区 */
.settings-content {
  flex: 1;
  padding: 20px 28px;
  overflow-y: auto;
  background: #1e1e1e;
}

.settings-section-title {
  font-size: 18px;
  font-weight: 600;
  color: #d4d4d4;
  margin-bottom: 20px;
}

.settings-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background: #2d2d2d;
  border-radius: 8px;
  border: 1px solid #3c3c3c;
  margin-bottom: 12px;
  transition: border-color 0.15s;
}

.settings-item:hover {
  border-color: #555;
}

.settings-item-info {
  flex: 1;
  min-width: 0;
  margin-right: 16px;
}

.settings-item-label {
  font-size: 14px;
  font-weight: 500;
  color: #d4d4d4;
}

.settings-item-desc {
  font-size: 12px;
  color: #888;
  margin-top: 4px;
}

.settings-restart-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
  padding: 8px 12px;
  background: rgba(230, 162, 60, 0.1);
  border: 1px solid rgba(230, 162, 60, 0.3);
  border-radius: 8px;
  color: #e6a23c;
  font-size: 12px;
}

/* 快捷键空状态 */
.settings-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  color: #555;
}

.settings-empty p {
  margin-top: 12px;
  font-size: 14px;
}
</style>

<style>
/* 全局：el-dialog 暗色主题覆盖 */
.settings-dialog .el-dialog {
  background: #1e1e1e;
  border: 1px solid #3c3c3c;
  border-radius: 8px;
}

.settings-dialog .el-dialog__header {
  background: #252526;
  border-bottom: 1px solid #3c3c3c;
  border-radius: 8px 8px 0 0;
  padding: 14px 20px;
}

.settings-dialog .el-dialog__title {
  color: #d4d4d4;
  font-size: 16px;
  font-weight: 600;
}

.settings-dialog .el-dialog__headerbtn .el-dialog__close {
  color: #888;
}

.settings-dialog .el-dialog__headerbtn:hover .el-dialog__close {
  color: #d4d4d4;
}

.settings-dialog .el-dialog__body {
  padding: 20px;
}

.settings-dialog .el-overlay {
  background-color: rgba(0, 0, 0, 0.5);
}
</style>
```

- [ ] **Step 2: 运行前端测试确认无回归**

Run: `cd frontend && npx vitest run`
Expected: 71 passed

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/SettingsPanel.vue
git commit -m "feat(settings): 重写SettingsPanel为el-dialog弹窗，左右双栏布局"
```

---

### Task 4: 清理残留代码

**Files:**
- Modify: `frontend/src/views/Home.vue`

- [ ] **Step 1: 清理 Home.vue 中 activePanel 对 settings 的残留引用**

检查 Home.vue 中是否还有 `activePanel === 'settings'` 或 `settings` 相关的残留引用。确保：
1. `activePanel` 的可选值仅为 `'directory'` 和 `'toolbox'`
2. `closeToolbox` 函数中不再检查 `settings`
3. 没有其他对 SettingsPanel `@close` 事件的引用

如果在 Task 2 中已清理完毕，则此步骤为验证步骤。

- [ ] **Step 2: 运行全量测试**

Run: `cd frontend && npx vitest run`
Expected: 71 passed

- [ ] **Step 3: 提交（如有改动）**

```bash
git add frontend/src/views/Home.vue
git commit -m "refactor(settings): 清理activePanel对settings的残留引用"
```

---

### Task 5: 验证与文档更新

**Files:**
- Verify: 手动运行 `wails dev` 验证弹窗功能
- Modify: `README.md`（如项目结构描述需要更新）

- [ ] **Step 1: 手动验证弹窗功能**

Run: `wails dev`

验证清单：
1. 点击活动栏设置图标 → 弹窗弹出
2. 左侧三个分类可切换，右侧内容对应显示
3. ESC / 点击遮罩 / 点击 ✕ → 弹窗关闭
4. 修改默认 Shell → 保存生效
5. 修改 GPU 开关 → 黄色重启提示出现
6. 快捷键页显示"暂无可配置快捷键"占位
7. 活动栏设置图标不再高亮切换（不改变 activePanel）

- [ ] **Step 2: 更新 README.md（如项目结构描述变更）**

检查 README.md 中 SettingsPanel 的描述是否需要从"设置面板"更新为"设置弹窗"。

- [ ] **Step 3: 提交**

```bash
git add -A
git commit -m "docs: 更新README设置面板描述"
```
