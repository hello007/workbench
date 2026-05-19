# Story 3.3: 文件预览

Status: review

## Story

As a 开发者,
I want 点击文件预览其内容,
So that 我无需离开应用即可查看文件。

## Acceptance Criteria

1. **AC1 - 文本文件预览（FR16）**：点击文本文件（≤1MB），右侧面板直接渲染文件内容，预览渲染时间 < 500ms（NFR3）
2. **AC2 - 大文件提示（FR17）**：点击大于 1MB 的文件，提示"文件过大，无法预览"
3. **AC3 - 二进制文件提示（FR16）**：点击二进制文件（图片、压缩包等），提示"二进制文件，无法预览"

## Tasks / Subtasks

- [x] Task 1: 验证后端 PreviewFile 服务（AC: #1, #2, #3）
  - [x] 1.1 阅读 `service/fileoperation.go:62-99` 的 `PreviewFile(filePath, maxSize)` 方法：os.Stat 获取文件信息 → 大小检查 → IsPreviewable 扩展名检查 → ReadFileSafe 读取内容
  - [x] 1.2 阅读 `util/file.go:12-31` 的 `IsPreviewable(filename)` 方法：扩展名白名单匹配（.txt, .md, .json, .go, .js, .vue 等）
  - [x] 1.3 阅读 `util/file.go:50-61` 的 `ReadFileSafe(filePath, maxSize)` 方法：大小检查 + os.ReadFile 安全读取
  - [x] 1.4 阅读 `app.go:194-201` 的 `PreviewFile(filePath)` Wails 绑定：maxSize=1MB 常量 + 调用 service + 错误处理
  - [x] 1.5 阅读 `model/models.go:116-124` 的 `FilePreview` 结构体：Path/Name/Size/Content/IsBinary/TooLarge/Error 字段
  - [x] 1.6 确认 `fileoperation_test.go` 中无 PreviewFile 测试（需补充）

- [x] Task 2: 验证前端预览交互（AC: #1, #2, #3）
  - [x] 2.1 阅读 `ContentPanel.vue:250-263` 的 `previewFile()` 方法：调用 PreviewFile API → 处理 error/tooLarge/isBinary 三种状态
  - [x] 2.2 阅读 `ContentPanel.vue:71-80` 的预览渲染模板：`v-if="filePreview.content"` 条件渲染 + el-input textarea readonly
  - [x] 2.3 阅读 `ContentPanel.vue:201-204` 的 `filePreview` 状态：ref({ content: '', error: '' })
  - [x] 2.4 阅读 `ContentPanel.vue:304-309` 的 `clearPreview()` 方法：重置 filePreview，通过 defineExpose 暴露
  - [x] 2.5 阅读 `ContentPanel.vue:65` 的"预览"按钮：`@click="previewFile"`，仅在文件类型节点可见
  - [x] 2.6 验证 Home.vue 中 `onNodeSelect` 调用 `clearPreview()` 清空旧预览（`Home.vue:116-119`）

- [x] Task 3: 编写后端测试（AC: #1, #2, #3）
  - [x] 3.1 编写 `TestPreviewFile_TextFile` 测试：文本文件 ≤1MB，验证 Content 正确、TooLarge=false、IsBinary=false
  - [x] 3.2 编写 `TestPreviewFile_TooLarge` 测试：文件 >1MB，验证 TooLarge=true、Content 为空
  - [x] 3.3 编写 `TestPreviewFile_Binary` 测试：含 null 字节的二进制文件，验证 IsBinary=true
  - [x] 3.4 编写 `TestPreviewFile_NotFound` 测试：不存在的文件路径，验证 Error 非空
  - [x] 3.5 编写 `TestPreviewFile_PreviewableExtension` 测试：验证 .go/.js/.vue/.md/.json 等扩展名文件被识别为可预览（不触发二进制检测）

- [x] Task 4: 编写前端测试（AC: #1, #2, #3）
  - [x] 4.1 编写 `previewFile` 成功测试：PreviewFile 返回内容 → filePreview.content 更新 → textarea 显示
  - [x] 4.2 编写 `previewFile` 大文件测试：PreviewFile 返回 tooLarge=true → ElMessage.warning('文件过大，无法预览')
  - [x] 4.3 编写 `previewFile` 二进制测试：PreviewFile 返回 isBinary=true → ElMessage.warning('二进制文件，无法预览')
  - [x] 4.4 编写 `previewFile` 错误测试：PreviewFile 返回 error → ElMessage.error
  - [x] 4.5 编写 `clearPreview` 测试：调用后 filePreview 重置为 { content: '', error: '' }

- [x] Task 5: 运行全量测试确认无回归

## Dev Notes

### 棕地项目背景

**文件预览功能已完整实现并投产。** 本 Story 属于验证性质，确认后端 `PreviewFile` 服务和前端预览交互满足 FR16/FR17/NFR3 的所有要求，并补充测试覆盖。

### 现有实现分析

**Go 后端 — 预览服务：**

- `service/fileoperation.go:62-99` — `PreviewFile(filePath, maxSize)`：
  ```go
  func (s *FileOperationService) PreviewFile(filePath string, maxSize int64) (*model.FilePreview, error) {
      preview := &model.FilePreview{
          Path: filePath,
          Name: filepath.Base(filePath),
      }
      info, err := os.Stat(filePath)
      if err != nil {
          preview.Error = err.Error()
          return preview, err
      }
      preview.Size = info.Size()
      if preview.Size > maxSize {
          preview.TooLarge = true
          return preview, nil
      }
      if !util.IsPreviewable(filePath) {
          data, _ := util.ReadFileSafe(filePath, 1024)
          for _, b := range data {
              if b == 0 {
                  preview.IsBinary = true
                  return preview, nil
              }
          }
      }
      data, err := util.ReadFileSafe(filePath, maxSize)
      if err != nil {
          preview.Error = err.Error()
          return preview, err
      }
      preview.Content = string(data)
      return preview, nil
  }
  ```
  - `os.Stat` 获取文件信息（大小、是否存在）
  - 大小检查：`preview.Size > maxSize` → `TooLarge = true`
  - 扩展名检查：`!util.IsPreviewable(filePath)` → 对非可预览扩展名读前 1024 字节检测 null 字节
  - 内容读取：`util.ReadFileSafe(filePath, maxSize)` 安全读取

- `util/file.go:12-31` — `IsPreviewable(filename)`：
  ```go
  func IsPreviewable(filename string) bool {
      ext := strings.ToLower(filepath.Ext(filename))
      previewableExts := []string{
          ".txt", ".md", ".markdown",
          ".json", ".xml", ".yaml", ".yml",
          ".js", ".ts", ".vue", ".go",
          ".java", ".py", ".c", ".cpp",
          ".html", ".css", ".sh", ".bat",
          ".gitignore", ".env",
      }
      for _, pe := range previewableExts {
          if ext == pe { return true }
      }
      return false
  }
  ```
  - 白名单包含 20+ 常见文本扩展名
  - 可预览扩展名跳过二进制检测，直接读取内容

- `util/file.go:50-61` — `ReadFileSafe(filePath, maxSize)`：
  ```go
  func ReadFileSafe(filePath string, maxSize int64) ([]byte, error) {
      info, err := os.Stat(filePath)
      if err != nil { return nil, err }
      if info.Size() > maxSize {
          return nil, fmt.Errorf("file too large: %d bytes", info.Size())
      }
      return os.ReadFile(filePath)
  }
  ```

**Go 后端 — Wails 绑定：**

- `app.go:194-201` — `PreviewFile(filePath) *model.FilePreview`：
  ```go
  func (a *App) PreviewFile(filePath string) *model.FilePreview {
      const maxSize = 1024 * 1024 // 1MB
      preview, err := a.fileOpSvc.PreviewFile(filePath, maxSize)
      if err != nil {
          preview.Error = err.Error()
      }
      return preview
  }
  ```
  - 标准调度层：定义常量 → 调用 service → 错误处理 → 返回

**数据模型：**

- `model/models.go:116-124` — `FilePreview` 结构体：
  ```go
  type FilePreview struct {
      Path     string `json:"path"`
      Name     string `json:"name"`
      Size     int64  `json:"size"`
      Content  string `json:"content,omitempty"`
      IsBinary bool   `json:"isBinary"`
      TooLarge bool   `json:"tooLarge"`
      Error    string `json:"error,omitempty"`
  }
  ```

**前端 — 预览功能：**

- `ContentPanel.vue:250-263` — `previewFile()`：
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
  - 调用 `PreviewFile` API 获取预览结果
  - 三种状态处理：error → ElMessage.error，tooLarge → ElMessage.warning，isBinary → ElMessage.warning
  - 成功时：`filePreview.value = preview` 包含 content，触发模板渲染

- `ContentPanel.vue:71-80` — 预览渲染模板：
  ```html
  <div v-if="filePreview.content" style="margin-top: 20px;">
    <h4>文件内容</h4>
    <el-input
      v-model="filePreview.content"
      type="textarea"
      :rows="10"
      readonly
      style="font-family: monospace;"
    />
  </div>
  ```
  - 条件渲染：`v-if="filePreview.content"` 有内容时才显示
  - 只读 textarea，等宽字体，10 行高度

- `ContentPanel.vue:201-204` — 状态：
  ```javascript
  const filePreview = ref({
    content: '',
    error: ''
  })
  ```

- `ContentPanel.vue:304-309` — `clearPreview()`：
  ```javascript
  const clearPreview = () => {
    filePreview.value = {
      content: '',
      error: ''
    }
  }
  ```
  - 通过 `defineExpose` 暴露（`ContentPanel.vue:337-340`）
  - Home.vue 在 `onNodeSelect` 和 `onDirectorySelect` 时调用

- `ContentPanel.vue:65` — "预览"按钮：
  ```html
  <el-button @click="previewFile">预览</el-button>
  ```
  - 仅在 `selectedNode.type === 'file'` 条件块内可见

### 数据流

```
用户点击"预览"按钮:
  ContentPanel.vue → previewFile() → PreviewFile(path) Wails API
  → app.go PreviewFile(path) → fileOpSvc.PreviewFile(path, 1MB)
  → os.Stat 获取文件信息
  → 大小检查（>1MB → TooLarge）
  → 扩展名检查（IsPreviewable）
  → 非可预览扩展名 → 二进制检测（前1024字节含null → IsBinary）
  → 可预览扩展名 → ReadFileSafe 读取内容
  → 返回 FilePreview{Content/TooLarge/IsBinary/Error}

  成功: filePreview.value = preview → v-if="filePreview.content" 渲染 textarea
  过大: ElMessage.warning('文件过大，无法预览')
  二进制: ElMessage.warning('二进制文件，无法预览')
  失败: ElMessage.error('预览失败: ' + error)

选中节点切换:
  Home.vue onNodeSelect/onDirectorySelect → contentPanelRef.clearPreview()
  → filePreview.value = { content: '', error: '' } → textarea 隐藏
```

### 数据契约

```go
// Go → Wails 绑定
PreviewFile(filePath string) *model.FilePreview

// FilePreview 结构
type FilePreview struct {
    Path     string `json:"path"`      // 文件完整路径
    Name     string `json:"name"`      // 文件名
    Size     int64  `json:"size"`      // 文件大小（字节）
    Content  string `json:"content,omitempty"` // 文本内容（成功时）
    IsBinary bool   `json:"isBinary"`  // 是否二进制文件
    TooLarge bool   `json:"tooLarge"`  // 是否超出大小限制
    Error    string `json:"error,omitempty"` // 错误信息
}
```

### 架构约束

- **app.go 调度层**：PreviewFile 方法调用 `fileOpSvc.PreviewFile`，≤10 行
- **禁止**：引入新依赖、TypeScript、CSS 框架
- **禁止**：在 service 中导入 Wails 包
- **错误处理链**：service 返回 error → app.go 设置 Error 字段 → 前端 ElMessage 提示
- **大小限制**：1MB 常量定义在 app.go（`1024 * 1024`）
- **二进制检测**：仅对非可预览扩展名执行，读前 1024 字节检查 null 字节
- **安全读取**：`ReadFileSafe` 双重保护（调用前检查 + 内部再次检查）

### 前一个 Story 的经验教训（Story 3-2）

1. **handleDeleteAt 暴露**：将方法添加到 defineExpose 以支持测试调用
2. **createWrapperWithStore**：已有辅助函数可复用，用于测试 refreshNode
3. **ElMessageBox.confirm mock**：`mockResolvedValueOnce('confirm')` / `mockRejectedValueOnce('cancel')` 控制确认/取消
4. **mockClear 优于 restoreAllMocks**：避免破坏 setup.js 全局 mock
5. **findComponent $attrs**：通过 `wrapper.findComponent('.el-tree').vm.$attrs` 访问事件处理器

### 更早期的经验教训

1. **mustMkdir/mustWriteFile helpers**：已定义在 `service/filetree_test.go`，本 Story 可复用
2. **el-tree stub 无 slot**：`template: '<div class="el-tree"></div>'`
3. **setup.js mock 必须同步更新**：新增 Go 绑定方法后，必须在 setup.js 添加对应 mock（PreviewFile 已存在于 setup.js:20）
4. **vi.importMock 路径**：使用 `'../../../wailsjs/go/main/App'`（三级 `../`）
5. **ElMessage mock**：在 FileTreePanel.spec.js 中 mock，ContentPanel.spec.js 需独立 mock
6. **ContentPanel.spec.js 已有 3 个测试**：节点信息展示（文件/文件夹/未选中）

### 测试注意事项

**后端测试（fileoperation_test.go 扩展）：**

- 当前无 PreviewFile 相关测试，需全部补充
- 测试场景：文本文件成功、文件过大、二进制文件、文件不存在、可预览扩展名
- 使用 `t.TempDir()` 创建临时文件
- 大文件测试：写入 >1MB 数据验证 TooLarge 标志
- 二进制测试：写入含 null 字节的数据验证 IsBinary 标志
- 注意：PreviewFile 对可预览扩展名文件跳过二进制检测，测试需覆盖此分支

**前端测试（ContentPanel.spec.js 扩展）：**

- `previewFile` 测试：`previewFile` 未通过 defineExpose 暴露，需通过 UI 交互触发
  - 找到"预览"按钮（`buttons.find(b => b.text() === '预览')`）并点击
  - PreviewFile mock 返回不同状态验证不同分支
- `clearPreview` 测试：`clearPreview` 已通过 defineExpose 暴露，可直接调用 `wrapper.vm.clearPreview()`
- ContentPanel stub 中 PreviewFile 已在 mock 中配置（`ContentPanel.spec.js:6`）
- ElMessage 需要在 ContentPanel.spec.js 中添加 mock（当前未 mock ElMessage）
- 预览内容显示验证：检查 `wrapper.find('textarea')` 是否存在及内容

### 关键验证点

1. **大小限制精确性**：`> 1MB`（1024*1024）标记 TooLarge，等于 1MB 不标记
2. **扩展名白名单**：20+ 扩展名在白名单中的文件跳过二进制检测
3. **二进制检测准确性**：前 1024 字节中含 null 字节（`\x00`）标记 IsBinary
4. **非可预览扩展名但非二进制**：如 .log 文件不在白名单，但不含 null → 也能读取内容
5. **错误传递链**：os.Stat 失败 → preview.Error + error 返回 → app.go 追加 Error → 前端 ElMessage.error
6. **clearPreview 清空**：切换节点时清空 filePreview，避免旧内容残留
7. **预览按钮仅文件可见**：文件夹和 Git 仓库节点无预览按钮

### 已知问题（不在本 Story 范围）

1. **预览按钮需手动点击**：不像 AC 描述的"点击文件即预览"，需先选中文件再点"预览"按钮
2. **textarea 行数固定**：硬编码 `:rows="10"`，不根据内容自适应
3. **二进制提示文案不一致**：AC 说"该文件类型不支持预览"，实际前端提示"二进制文件，无法预览"
4. **非白名单非二进制文件**：如 .log 文件，不在白名单但也不是二进制，仍能预览但走了二进制检测路径
5. **无编码检测**：直接 `string(data)` 转换，不检测文件编码

### References

- [Source: service/fileoperation.go:62-99] — PreviewFile 方法
- [Source: util/file.go:12-31] — IsPreviewable 扩展名检查
- [Source: util/file.go:50-61] — ReadFileSafe 安全读取
- [Source: app.go:194-201] — PreviewFile Wails 绑定
- [Source: model/models.go:116-124] — FilePreview 结构体
- [Source: frontend/src/components/ContentPanel.vue:250-263] — previewFile 方法
- [Source: frontend/src/components/ContentPanel.vue:71-80] — 预览渲染模板
- [Source: frontend/src/components/ContentPanel.vue:201-204] — filePreview 状态
- [Source: frontend/src/components/ContentPanel.vue:304-309] — clearPreview 方法
- [Source: frontend/src/components/ContentPanel.vue:337-340] — defineExpose（含 clearPreview）
- [Source: frontend/src/components/ContentPanel.vue:65] — 预览按钮
- [Source: frontend/src/views/Home.vue:116-119] — onNodeSelect clearPreview
- [Source: service/fileoperation_test.go] — 现有后端测试（无 PreviewFile 测试）
- [Source: frontend/src/components/__tests__/ContentPanel.spec.js] — 现有前端测试（3 个）
- [Source: frontend/src/test/setup.js] — 全局 mock 配置

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7

### Debug Log References

### Completion Notes List

- 验证后端 PreviewFile 服务：完整实现了大小检查、扩展名白名单、二进制检测和安全读取
- 验证前端预览交互：previewFile() 方法处理 error/tooLarge/isBinary 三种状态，clearPreview() 通过 defineExpose 暴露
- 后端新增 5 个 PreviewFile 测试：TextFile、TooLarge、Binary、NotFound、PreviewableExtension（覆盖 8 种常见文本扩展名）
- 前端 ContentPanel 新增 5 个预览测试：previewFile 成功/大文件/二进制/错误、clearPreview 清空
- ContentPanel.spec.js 存根优化：el-input 支持 type="textarea" 渲染文本区域
- 全量测试通过：Go 全绿，前端 82 个组件测试全绿（ContentPanel 从 3 个增加到 8 个）

### File List

- `service/fileoperation_test.go` — 新增 5 个 PreviewFile 测试
- `frontend/src/components/__tests__/ContentPanel.spec.js` — 新增 5 个预览测试，优化 el-input 存根
