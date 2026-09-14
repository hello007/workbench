# 文件预览编辑功能 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为文件预览文本框增加就地编辑能力，支持取消恢复和原子保存。

**Architecture:** 后端新增 `SaveFile` 方法（原子写入：临时文件 + rename），通过 `App.SaveFile` 暴露给前端。前端去掉 textarea 的 `readonly`，新增 `originalContent` 做变更检测，修改时浮现操作栏（取消/保存）。切换文件时通过 `ElMessageBox.confirm` 检查未保存修改。

**Tech Stack:** Go 1.26 (后端) / Vue 3 Composition API + Element Plus (前端) / Wails v2 (桥接)

---

## File Structure

| 文件 | 操作 | 职责 |
|------|------|------|
| `service/fileoperation.go` | 新增方法 | `SaveFile` — 原子写入文件内容 |
| `service/fileoperation_test.go` | 新增测试 | `SaveFile` 的单元测试 |
| `app.go` | 新增方法 | `SaveFile` — 暴露给前端的桥接方法 |
| `frontend/src/components/ContentPanel.vue` | 修改 | 去掉 readonly、新增编辑态响应式数据 + 操作栏 UI + 交互逻辑 |
| `README.md` | 更新 | 补充文件编辑功能描述 |

---

### Task 1: 后端 — SaveFile 单元测试

**Files:**
- Modify: `service/fileoperation_test.go` (追加)

- [ ] **Step 1: 编写 SaveFile 的失败测试**

在 `service/fileoperation_test.go` 末尾追加：

```go
func TestSaveFile_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	os.WriteFile(file, []byte("original"), 0644)

	svc := NewFileOperationService()
	err := svc.SaveFile(file, "updated content")
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if string(data) != "updated content" {
		t.Errorf("Expected 'updated content', got '%s'", string(data))
	}
}

func TestSaveFile_FileNotFound(t *testing.T) {
	svc := NewFileOperationService()
	err := svc.SaveFile(filepath.Join(t.TempDir(), "nonexistent.txt"), "content")
	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}
}

func TestSaveFile_DirectoryPath(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()
	err := svc.SaveFile(dir, "content")
	if err == nil {
		t.Fatal("Expected error when saving to a directory")
	}
}

func TestSaveFile_ContentTooLarge(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "big.txt")
	os.WriteFile(file, []byte("small"), 0644)

	svc := NewFileOperationService()
	largeContent := string(make([]byte, 1024*1024+1)) // > 1MB
	err := svc.SaveFile(file, largeContent)
	if err == nil {
		t.Fatal("Expected error for content exceeding size limit")
	}
}

func TestSaveFile_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "empty.txt")
	os.WriteFile(file, []byte("has content"), 0644)

	svc := NewFileOperationService()
	err := svc.SaveFile(file, "")
	if err != nil {
		t.Fatalf("SaveFile with empty content failed: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if string(data) != "" {
		t.Errorf("Expected empty content, got '%s'", string(data))
	}
}

func TestSaveFile_NoTempFileLeak(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "clean.txt")
	os.WriteFile(file, []byte("before"), 0644)

	svc := NewFileOperationService()
	err := svc.SaveFile(file, "after")
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "clean.txt" {
			t.Errorf("Unexpected file left behind: %s", e.Name())
		}
	}
}
```

- [ ] **Step 2: 运行测试，确认全部失败**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./service/ -run TestSaveFile -v`
Expected: 编译失败或 `undefined: NewFileOperationService.SaveFile`

---

### Task 2: 后端 — 实现 SaveFile 方法

**Files:**
- Modify: `service/fileoperation.go` (在 `PreviewFile` 方法之后追加)
- Modify: `app.go` (在 `PreviewFile` 方法之后追加)

- [ ] **Step 1: 在 `service/fileoperation.go` 的 `PreviewFile` 方法后面追加 `SaveFile`**

在第 99 行（`PreviewFile` 方法结束的 `}` 之后）追加：

```go
// SaveFile 保存文件内容（原子写入：先写临时文件再 rename）
func (s *FileOperationService) SaveFile(filePath string, content string) error {
	// 校验路径存在且为普通文件
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("文件不存在: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("不能保存目录")
	}

	// 大小限制（与预览一致：1MB）
	const maxSize = 1024 * 1024
	if int64(len(content)) > maxSize {
		return fmt.Errorf("内容超过1MB限制")
	}

	// 原子写入：先写临时文件再 rename
	dir := filepath.Dir(filePath)
	tmpFile, err := os.CreateTemp(dir, ".workbench-save-*")
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

- [ ] **Step 2: 在 `app.go` 的 `PreviewFile` 方法后面追加桥接方法**

在第 218 行（`PreviewFile` 方法结束的 `}` 之后）追加：

```go
// SaveFile 保存文件内容
func (a *App) SaveFile(filePath string, content string) error {
	return a.fileOpSvc.SaveFile(filePath, content)
}
```

- [ ] **Step 3: 运行后端测试，确认全部通过**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./service/ -run TestSaveFile -v`
Expected: PASS（6 个测试全部通过）

- [ ] **Step 4: 运行全量后端测试，确认无回归**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./...`
Expected: PASS（无失败测试）

- [ ] **Step 5: 提交**

```bash
git add service/fileoperation.go service/fileoperation_test.go app.go
git commit -m "feat(file-preview): add SaveFile backend API with atomic write"
```

---

### Task 3: 前端 — 生成 Wails 绑定

**Files:**
- Auto-generated: `frontend/wailsjs/go/main/App.js` 和 `App.d.ts`

- [ ] **Step 1: 重新生成 Wails 前端绑定**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && wails dev`
Expected: 启动后 Wails 会自动检测到新增的 `SaveFile` 方法并生成前端绑定

> **注意**：等待前端编译完成后（看到 "Development server running" 即可），按 Ctrl+C 停止。绑定文件会自动保留在 `frontend/wailsjs/go/main/App.js` 和 `App.d.ts` 中。

- [ ] **Step 2: 确认绑定已生成**

Run: `grep -c "SaveFile" d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench/frontend/wailsjs/go/main/App.js`
Expected: 输出 `1` 或更多

---

### Task 4: 前端 — ContentPanel 编辑态响应式数据和 import

**Files:**
- Modify: `frontend/src/components/ContentPanel.vue`

- [ ] **Step 1: 在 import 中加入 `SaveFile` 和 `ElMessageBox`**

将第 320-324 行：

```javascript
import {
  PreviewFile, PullRepo, CloneRepo, OpenWithDefaultApp,
  OpenInExplorer, OpenInVSCode, OpenInWarp,
  GetBranches, CheckoutBranch
} from '../../wailsjs/go/main/App'
```

改为：

```javascript
import {
  PreviewFile, SaveFile, PullRepo, CloneRepo, OpenWithDefaultApp,
  OpenInExplorer, OpenInVSCode, OpenInWarp,
  GetBranches, CheckoutBranch
} from '../../wailsjs/go/main/App'
```

将第 314 行：

```javascript
import { ElMessage } from 'element-plus'
```

改为：

```javascript
import { ElMessage, ElMessageBox } from 'element-plus'
```

- [ ] **Step 2: 新增编辑态响应式数据**

将第 357-360 行：

```javascript
const filePreview = ref({
  content: '',
  error: ''
})
```

改为：

```javascript
const filePreview = ref({
  content: '',
  error: ''
})
const originalContent = ref('')
const isSaving = ref(false)

const isContentModified = computed(() => {
  return filePreview.value.content !== originalContent.value
})
```

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/ContentPanel.vue
git commit -m "feat(file-preview): add edit state reactive data and imports"
```

---

### Task 5: 前端 — 编辑交互方法

**Files:**
- Modify: `frontend/src/components/ContentPanel.vue`

- [ ] **Step 1: 修改 `previewFile` 方法，同步 originalContent**

将第 532-545 行的 `previewFile` 方法：

```javascript
const previewFile = async () => {
  if (!props.selectedNode) return

  const preview = await PreviewFile(props.selectedNode.path)
  filePreview.value = preview

  if (preview.error) {
    ElMessage.error('预览失败: ' + preview.error)
  } else if (preview.tooLarge) {
    ElMessage.warning('文件过大，无法预览')
  } else if (preview.isBinary) {
    ElMessage.warning('二进制文件，无法预览')
  }
}
```

改为：

```javascript
const previewFile = async () => {
  if (!props.selectedNode) return

  const preview = await PreviewFile(props.selectedNode.path)
  filePreview.value = preview

  if (preview.error) {
    ElMessage.error('预览失败: ' + preview.error)
  } else if (preview.tooLarge) {
    ElMessage.warning('文件过大，无法预览')
  } else if (preview.isBinary) {
    ElMessage.warning('二进制文件，无法预览')
  }

  // 同步原始内容，用于编辑态变更检测
  originalContent.value = preview.content || ''
}
```

- [ ] **Step 2: 修改 `clearPreview` 方法，重置 originalContent**

将第 606-611 行的 `clearPreview` 方法：

```javascript
const clearPreview = () => {
  filePreview.value = {
    content: '',
    error: ''
  }
}
```

改为：

```javascript
const clearPreview = () => {
  filePreview.value = {
    content: '',
    error: ''
  }
  originalContent.value = ''
}
```

- [ ] **Step 3: 修改 watch 逻辑，增加未保存修改检查**

将第 547-551 行的 watch：

```javascript
watch(() => props.selectedNode, async (newNode) => {
  if (newNode && newNode.type === 'file') {
    await previewFile()
  }
})
```

改为：

```javascript
watch(() => props.selectedNode, async (newNode, oldNode) => {
  // 切换文件前检查是否有未保存修改
  if (oldNode && oldNode.type === 'file' && isContentModified.value) {
    try {
      await ElMessageBox.confirm(
        '当前文件已修改未保存，是否放弃修改？',
        '未保存的修改',
        { confirmButtonText: '放弃', cancelButtonText: '继续编辑', type: 'warning' }
      )
    } catch {
      // 用户选择"继续编辑"，阻止切换
      return
    }
  }
  if (newNode && newNode.type === 'file') {
    await previewFile()
  }
})
```

- [ ] **Step 4: 在 `previewFile` 方法后面追加 `handleSave` 和 `handleCancelEdit`**

在 `previewFile` 方法结束的 `}` 之后追加：

```javascript
const handleSave = async () => {
  if (!props.selectedNode || !isContentModified.value) return

  isSaving.value = true
  try {
    await SaveFile(props.selectedNode.path, filePreview.value.content)
    ElMessage.success('文件保存成功')
    originalContent.value = filePreview.value.content
    emit('refreshNode', props.selectedNode.path)
  } catch (error) {
    ElMessage.error('保存失败: ' + (error.message || String(error)))
  } finally {
    isSaving.value = false
  }
}

const handleCancelEdit = () => {
  filePreview.value.content = originalContent.value
}
```

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/ContentPanel.vue
git commit -m "feat(file-preview): add save/cancel edit handlers and unsaved changes check"
```

---

### Task 6: 前端 — 编辑态 UI 模板

**Files:**
- Modify: `frontend/src/components/ContentPanel.vue`

- [ ] **Step 1: 去掉 textarea 的 readonly，增加操作栏**

将第 120-129 行：

```html
        <div v-if="filePreview.content" class="file-preview">
          <h4>文件内容</h4>
          <el-input
            v-model="filePreview.content"
            type="textarea"
            :rows="15"
            readonly
            class="preview-textarea"
          />
        </div>
```

改为：

```html
        <div v-if="filePreview.content" class="file-preview">
          <h4>文件内容</h4>
          <el-input
            v-model="filePreview.content"
            type="textarea"
            :rows="15"
            class="preview-textarea"
          />
          <div v-if="isContentModified" class="preview-actions">
            <span class="modified-indicator">● 已修改</span>
            <el-button size="small" @click="handleCancelEdit">取消</el-button>
            <el-button size="small" type="primary" :loading="isSaving" @click="handleSave">保存</el-button>
          </div>
        </div>
```

- [ ] **Step 2: 追加操作栏样式**

在 `<style scoped>` 中，`.preview-textarea:hover` 规则之后（约第 759 行）追加：

```css
.preview-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-color);
}

.modified-indicator {
  color: #e6a23c;
  font-size: 12px;
  margin-right: auto;
}
```

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/ContentPanel.vue
git commit -m "feat(file-preview): enable textarea editing and add save/cancel action bar"
```

---

### Task 7: 文档更新

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 在 README.md 的功能列表中补充文件编辑功能描述**

在文件预览相关描述位置追加：

```markdown
- 文件预览支持就地编辑，修改后可保存或取消
```

- [ ] **Step 2: 提交**

```bash
git add README.md
git commit -m "docs: add file preview edit feature to README"
```

---

### Task 8: 集成验证

- [ ] **Step 1: 运行全量后端测试**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./...`
Expected: PASS

- [ ] **Step 2: 运行前端测试**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench/frontend && npm test`
Expected: PASS

- [ ] **Step 3: 启动开发环境进行手动验证**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && wails dev`

验证清单：
1. 选中一个文本文件 → 自动预览内容
2. 修改文本框内容 → 底部出现"● 已修改" + 取消/保存按钮
3. 点击"取消" → 内容恢复为原始值，操作栏消失
4. 再次修改 → 点击"保存" → 提示"文件保存成功"，操作栏消失
5. 用外部编辑器打开文件 → 确认内容已更新
6. 修改后切换到另一个文件 → 弹出确认框"当前文件已修改未保存，是否放弃修改？"
7. 选择"继续编辑" → 停留在当前文件
8. 选择"放弃" → 切换到新文件
9. 选中一个二进制文件 → 不显示编辑区域
