# 设置面板弹窗化设计

> 日期：2026-06-03
> 状态：待审核

## 摘要

将设置面板从左侧内嵌面板改为 el-dialog 弹窗，采用左右双栏布局：左侧分类导航，右侧设置项内容。

## 1. 改动范围

| 文件 | 变更 |
|------|------|
| `SettingsPanel.vue` | 重写：从内嵌面板 → el-dialog 弹窗，内部左右布局 |
| `Home.vue` | 移除 SettingsPanel 内嵌渲染，改为弹窗模式挂载 |
| `ActivityBar.vue` | 设置图标点击逻辑不变（已发射事件） |
| `model/settings.go` | 无变更 |
| `service/settings.go` | 无变更 |

## 2. 组件架构

```
SettingsPanel.vue (el-dialog)
├── 左侧导航栏 (200px, #252526)
│   ├── 通用
│   ├── 终端
│   └── 快捷键
└── 右侧内容区
    ├── 通用页：GPU 加速开关
    ├── 终端页：默认 Shell / Git Bash 路径 / WSL 发行版
    └── 快捷键页：占位提示"暂无可配置快捷键"
```

## 3. 弹窗规格

- **尺寸**：760×500（宽松型）
- **位置**：居中弹出
- **容器**：Element Plus `el-dialog`
- **遮罩**：半透明黑色遮罩层，点击可关闭

## 4. 交互行为

| 操作 | 行为 |
|------|------|
| 打开 | ActivityBar 设置图标点击 → `visible=true` → 弹窗弹出 |
| 关闭 | 点击右上角 ✕ / 按 ESC / 点击遮罩层 → `visible=false` |
| 分类切换 | 点击左侧分类项 → 右侧内容切换 |
| 设置保存 | 即时保存（`@change` 触发 `SaveSettings`，与现有逻辑一致） |
| GPU 重启提示 | 保存后显示黄色提示条（与现有逻辑一致） |

## 5. 左侧导航栏

- 固定宽度 200px，背景色 `#252526`，右边框 `1px solid #3c3c3c`
- 选中项：蓝色左边框指示条 `2px solid #409eff` + 深蓝背景 `#094771`，文字 `#ccc`
- 未选中项：灰色文字 `#888`，hover 背景变亮 `#2d2d2d`
- 分类项间距：`padding: 10px 20px`，`font-size: 14px`

## 6. 右侧内容区

- 分类标题：`font-size: 18px`，`font-weight: 600`，颜色 `#d4d4d4`，底部间距 `20px`
- 设置项卡片：`padding: 14px 16px`，`background: #2d2d2d`，`border-radius: 8px`，`border: 1px solid #3c3c3c`，卡片间距 `12px`
- hover 效果：`border-color` 变亮
- 控件：Element Plus 标准组件（el-switch / el-select / el-input）

### 6.1 通用页

| 设置项 | 控件 | 说明 |
|--------|------|------|
| GPU 加速 | el-switch | 开启/关闭，变更后显示重启提示 |

### 6.2 终端页

| 设置项 | 控件 | 说明 |
|--------|------|------|
| 默认 Shell | el-select | PowerShell / CMD / Git Bash / WSL |
| Git Bash 路径 | el-input | 仅 `defaultShell=gitbash` 时显示 |
| WSL 发行版 | el-input | 仅 `defaultShell=wsl` 时显示 |

### 6.3 快捷键页

- 占位显示："暂无可配置快捷键"
- 后续扩展时在 `AppSettings` 中新增 `Shortcuts` 字段

## 7. Home.vue 改动

**改前**（内嵌模式）：

```vue
<SettingsPanel v-show="activePanel === 'settings'" @close="activePanel = 'directory'" />
```

**改后**（弹窗模式）：

```vue
<SettingsPanel v-model:visible="settingsVisible" />
```

- 新增 `settingsVisible` ref，由 ActivityBar 的设置图标点击事件控制
- 移除左侧面板中的 `activePanel === 'settings'` 相关渲染和逻辑
- ActivityBar 设置图标点击时：打开设置弹窗，不切换左侧面板内容

## 8. 数据模型

**无变更**。`AppSettings` 结构体保持不变：

```go
type AppSettings struct {
    GpuDisabled  bool   `json:"gpuDisabled"`
    DefaultShell string `json:"defaultShell"`
    GitBashPath  string `json:"gitBashPath"`
    WslDistro    string `json:"wslDistro"`
}
```

## 9. 非目标

- 不新增设置项
- 不修改设置保存逻辑
- 不实现快捷键配置功能（仅占位）
- 不修改后端数据模型
