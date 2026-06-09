# 文件预览编辑功能设计

> **创建日期**：2026-06-08
> **状态**：待实现
> **涉及文件**：`service/fileoperation.go`、`app.go`、`frontend/src/components/ContentPanel.vue`

## 1. 摘要

为文件预览区域的文本框增加就地编辑能力。用户在预览文本框中直接修改内容后，底部自动浮现"已修改"状态标记和"取消/保存"操作按钮。取消操作恢复原始内容，保存操作通过后端 `SaveFile` 接口写入文件并刷新文件树节点信息。

## 2. 交互设计

### 2.1 状态流转

| 状态 | 触发条件 | 界面表现 |
|------|----------|----------|
| **预览态** | 选中文件 / 保存成功 / 取消编辑 | 文本框可编辑，操作栏隐藏 |
| **已修改态** | 用户修改文本框内容 | 底部浮现"已修改"标记 + 取消/保存按钮 |
| **保存中** | 点击保存按钮 | 按钮显示 loading，禁止重复操作 |
| **保存成功** | 后端返回成功 | 操作栏消失，刷新文件树节点 |
| **保存失败** | 后端返回错误 | 显示错误提示，保留编辑内容 |

### 2.2 界面布局

```
┌─────────────────────────────────────┐
│  文件内容                            │
│  ┌─────────────────────────────────┐ │
│  │  (textarea - 可编辑)             │ │
│  │  ...文件内容...                  │ │
│  │                                 │ │
│  └─────────────────────────────────┘ │
│  ┌─────────────────────────────────┐ │  ← 仅 isContentModified=true 时显示
│  │  ● 已修改   [取消] [保存]       │ │
│  └─────────────────────────────────┘ │
└─────────────────────────────────────┘
```

## 3. 前端改动

**文件**：`frontend/src/components/ContentPanel.vue`

### 3.1 新增响应式数据

```javascript
const originalContent = ref('')    // 原始预览内容，用于变更检测和取消恢复
const isSaving = ref(false)        // 保存中状态

const isContentModified = computed(() => {
  return filePreview.value.content !== originalContent.value
})
```

### 3.2 修改模板

- 去掉 `el-input` 的 `readonly` 属性
- 在 `.file-preview` 容器底部增加条件渲染的操作栏

```html
<div v-if="isContentModified" class="preview-actions">
  <span class="modified-indicator">● 已修改</span>
  <el-button size="small" @click="handleCancelEdit">取消</el-button>
  <el-button size="small" type="primary" :loading="isSaving" @click="handleSave">保存</el-button>
</div>
```

### 3.3 新增方法

| 方法 | 职责 |
|------|------|
| `handleSave()` | 调用 `SaveFile` 后端接口，成功后刷新文件树并更新 `originalContent` |
| `handleCancelEdit()` | 将 `filePreview.content` 恢复为 `originalContent` |
| `checkUnsavedChanges()` | 切换文件前检查是否有未保存修改，弹出确认框 |

### 3.4 调整现有方法

- `previewFile()`：加载预览后同步设置 `originalContent = filePreview.content`
- `clearPreview()`：清空时同时重置 `originalContent`

### 3.5 样式

```css
.preview-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: var(--bg-secondary);
  border-radius: var(--radius-sm);
}

.modified-indicator {
  color: var(--warning-color, #e6a23c);
  font-size: 12px;
  margin-right: auto;
}
```

## 4. 后端改动

### 4.1 新增 `SaveFile` 方法

**文件**：`service/fileoperation.go`

```go
// SaveFile 保存文件内容（原子写入）
func (s *FileOperationService) SaveFile(filePath string, content string) error {
    // 1. 校验路径存在且为普通文件
    info, err := os.Stat(filePath)
    if err != nil {
        return fmt.Errorf("文件不存在: %w", err)
    }
    if info.IsDir() {
        return fmt.Errorf("不能保存目录")
    }

    // 2. 大小限制（与预览一致：1MB）
    const maxSize = 1024 * 1024
    if int64(len(content)) > maxSize {
        return fmt.Errorf("内容超过1MB限制")
    }

    // 3. 原子写入：先写临时文件再 rename
    dir := filepath.Dir(filePath)
    tmpFile, err := os.CreateTemp(dir, ".git-manager-save-*")
    if err != nil {
        return fmt.Errorf("创建临时文件失败: %w", err)
    }
    tmpPath := tmpFile.Name()

    _, err = tmpFile.WriteString(content)
    tmpFile.Close()
    if err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("写入临时文件失败: %w", err)
    }

    err = os.Rename(tmpPath, filePath)
    if err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("替换文件失败: %w", err)
    }

    return nil
}
```

**文件**：`app.go`

```go
// SaveFile 保存文件
func (a *App) SaveFile(filePath string, content string) error {
    return a.fileOpSvc.SaveFile(filePath, content)
}
```

## 5. 边界场景处理

| 场景 | 处理方式 |
|------|----------|
| 二进制文件 / 文件过大 | 不显示编辑区域，保持当前提示不变 |
| 切换选中文件时有未保存修改 | 弹出 `ElMessageBox.confirm`："当前文件已修改未保存，是否放弃修改？" |
| 保存失败 | `ElMessage.error` 显示错误信息，保留编辑内容，用户可重试 |
| 文件被外部删除 | 保存时后端返回"文件不存在"错误，前端提示用户 |

## 6. 文件变更清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `service/fileoperation.go` | 新增方法 | `SaveFile` — 原子写入文件内容 |
| `app.go` | 新增方法 | `SaveFile` — 暴露给前端的桥接方法 |
| `frontend/src/components/ContentPanel.vue` | 修改 | 去掉 readonly、新增编辑态 UI 和交互逻辑 |
| `README.md` | 更新 | 补充文件编辑功能描述 |

## 7. 不做的事情

- **不做**行号显示、语法高亮（超出当前范围）
- **不做**文件编码转换（保持原始编码）
- **不做**自动保存（用户明确点击保存才生效）
- **不做**撤销/重做（依赖浏览器原生 textarea 能力）
