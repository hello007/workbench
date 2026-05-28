# Story 3.4: 右键菜单系统

Status: ready-for-dev

## Story

As a 开发者,
I want 通过右键菜单对文件/文件夹执行操作,
So that 我可以快速访问所有文件操作。

## Acceptance Criteria

1. **AC1 - 右键菜单显示（FR18）**：右键点击文件树中的节点，菜单弹出，根据节点类型（文件/文件夹/Git 仓库/空白区域）显示不同的操作列表
2. **AC2 - 文件夹菜单项**：文件夹节点的可用操作包含：新建文件、新建文件夹、重命名、删除、剪切、复制、粘贴、拷贝到…、复制路径、在资源管理器中打开、用 VSCode 打开、用 Warp 打开、刷新、更新仓库
3. **AC3 - 文件菜单项**：文件节点的可用操作包含：重命名、删除、剪切、复制、粘贴、拷贝到…、复制路径、复制文件名、在资源管理器中打开、用 VSCode 打开、用 Warp 打开、用默认程序打开
4. **AC4 - 空白区域菜单项**：右键点击文件树空白区域，显示新建文件、新建文件夹
5. **AC5 - 菜单互斥**：左栏目录列表与中栏文件树的右键菜单互斥，同时只能显示一个

## Tasks / Subtasks

- [ ] Task 1: 验证右键菜单显示与节点类型分发（AC: #1, #2, #3, #4）
  - [ ] 1.1 阅读 `FileTreePanel.vue:155-259` 的右键菜单模板：三种 `v-if`/`v-else-if`/`v-else` 分支（空白区域、文件夹、文件），菜单项列表与命令映射
  - [ ] 1.2 阅读 `FileTreePanel.vue:317-324` 的 `contextMenu` 状态：`visible/x/y/data/isBlankArea` 五个字段
  - [ ] 1.3 阅读 `FileTreePanel.vue:488-543` 的 `onNodeContextMenu`：`event.preventDefault/stopPropagation` → emit `'contextmenu'` → 设置坐标与数据 → `nextTick` 边界调整
  - [ ] 1.4 阅读 `FileTreePanel.vue:556-588` 的 `onBlankAreaContextMenu`：构造合成的 directory 数据 → `isBlankArea = true` → 边界调整
  - [ ] 1.5 阅读 `DirectoryTree.vue:52-79` 的右键菜单模板：无条件菜单（6 项：重命名、设为默认、在资源管理器中打开、用 VSCode 打开、用 Warp 打开、删除）
  - [ ] 1.6 阅读 `DirectoryTree.vue:160-165` 的 `contextMenu` 状态：`visible/x/y/targetDir`
  - [ ] 1.7 阅读 `DirectoryTree.vue:176-231` 的 `onContextMenu`：同样模式（preventDefault → emit → 坐标 → 边界调整）
  - [ ] 1.8 验证空白区域右键菜单的"新建文件/文件夹"功能指向当前选中目录

- [ ] Task 2: 验证菜单项命令分发（AC: #1, #2, #3）
  - [ ] 2.1 阅读 `FileTreePanel.vue:596-651` 的 `onMenuCommand(command)`：switch 分发到 createFile/createDir/rename/delete/cut/copy/paste/copyTo/copyPath/copyName/openExplorer/openInVSCode/openInWarp/openWithDefaultApp/refresh/pullRepos
  - [ ] 2.2 阅读 `DirectoryTree.vue:245-270` 的 `onMenuCommand(command)`：switch 分发到 rename/setDefault/openExplorer/openVSCode/delete
  - [ ] 2.3 验证"粘贴"菜单项的条件禁用：`clipboard.mode` 为 null 时 `is-disabled` 样式 + `@click` 守卫
  - [ ] 2.4 验证"复制路径"和"复制文件名"使用 `navigator.clipboard.writeText`（浏览器原生 API）
  - [ ] 2.5 验证"用默认程序打开"仅出现在文件菜单（非文件夹）

- [ ] Task 3: 验证菜单互斥与关闭机制（AC: #5）
  - [ ] 3.1 阅读 `Home.vue:102-111` 的互斥逻辑：`onDirectoryContextMenu` 关闭 FileTreePanel 菜单，`onFileTreeContextMenu` 关闭 DirectoryTree 菜单
  - [ ] 3.2 阅读 `FileTreePanel.vue:546-552` 的 `closeContextMenu` 和 `onGlobalClick`：全局 mousedown 监听关闭
  - [ ] 3.3 阅读 `FileTreePanel.vue:591-593` 的 `onGlobalContextMenu`：全局 contextmenu 监听关闭
  - [ ] 3.4 验证 `@node-expand` 和 `@node-collapse` 事件调用 `closeContextMenu`（展开/收起节点时菜单关闭）
  - [ ] 3.5 阅读 `DirectoryTree.vue:233-243` 的 `closeContextMenu`、`onGlobalClick`、`onGlobalContextMenu`
  - [ ] 3.6 验证 `defineExpose` 暴露 `closeMenu` 方法（FileTreePanel:876, DirectoryTree:168-174）

- [ ] Task 4: 验证菜单定位与边界调整
  - [ ] 4.1 阅读 `FileTreePanel.vue:506-543` 的 `nextTick` 边界调整：`getBoundingClientRect()` 测量实际尺寸 → 溢出时 5px margin 修正
  - [ ] 4.2 阅读 `DirectoryTree.vue:193-230` 的同款边界调整逻辑
  - [ ] 4.3 验证菜单使用 `position: fixed; z-index: 2000` 满足架构约束

- [ ] Task 5: 验证全局监听器生命周期
  - [ ] 5.1 阅读 `FileTreePanel.vue:880-888` 的 `onMounted`/`onBeforeUnmount`：`document.addEventListener('mousedown'/'contextmenu')` + 对应 `removeEventListener`
  - [ ] 5.2 阅读 `DirectoryTree.vue:477-485` 的同类生命周期管理
  - [ ] 5.3 确认 `@click.stop` 和 `@mousedown.stop` 在菜单 `<ul>` 上阻止冒泡

- [ ] Task 6: 编写前端测试（AC: #1, #2, #3, #4, #5）
  - [ ] 6.1 编写 FileTreePanel 右键菜单显示测试：右键文件节点 → 菜单可见 + 包含文件特有项（复制文件名、用默认程序打开）
  - [ ] 6.2 编写 FileTreePanel 文件夹菜单测试：右键文件夹节点 → 菜单包含新建文件/文件夹、刷新、更新仓库
  - [ ] 6.3 编写 FileTreePanel 空白区域菜单测试：右键空白区域 → 仅显示新建文件、新建文件夹
  - [ ] 6.4 编写 DirectoryTree 右键菜单测试：右键目录 → 菜单包含重命名、设为默认、删除、打开操作
  - [ ] 6.5 编写菜单关闭测试：点击菜单外区域 → 菜单关闭
  - [ ] 6.6 编写粘贴禁用测试：`clipboard.mode` 为 null 时粘贴项显示禁用样式
  - [ ] 6.7 编写 Home.vue 互斥测试：FileTreePanel 打开菜单时 DirectoryTree 菜单关闭（反之亦然）
  - [ ] 6.8 编写全局监听器清理测试：unmount 时 mousedown/contextmenu 监听器被移除

- [ ] Task 7: 运行全量测试确认无回归

## Dev Notes

### 棕地项目背景

**右键菜单系统已完整实现并投产。** 本 Story 属于验证性质，确认前端右键菜单的显示逻辑、节点类型分发、命令处理、互斥机制和边界调整满足 FR18 的所有要求，并补充测试覆盖。

### 现有实现分析

**前端 — FileTreePanel 右键菜单：**

- `FileTreePanel.vue:155-259` — 右键菜单模板：
  ```html
  <ul v-if="contextMenu.visible" class="context-menu"
    :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
    @click.stop @mousedown.stop>
    <!-- 空白区域 -->
    <template v-if="contextMenu.isBlankArea">
      新建文件 (createFile) | 新建文件夹 (createDir)
    </template>
    <!-- 文件夹 -->
    <template v-else-if="contextMenu.data?.type === 'directory'">
      新建文件 | 新建文件夹 | --- | 重命名 | 删除 | --- |
      剪切 | 复制 | 粘贴 | 拷贝到… | --- |
      复制路径 | 在资源管理器中打开 | 用 VSCode 打开 | 用 Warp 打开 | --- |
      刷新 | 更新仓库
    </template>
    <!-- 文件 -->
    <template v-else>
      重命名 | 删除 | --- | 剪切 | 复制 | 粘贴 | 拷贝到… | --- |
      复制路径 | 复制文件名 | 在资源管理器中打开 | 用 VSCode 打开 | 用 Warp 打开 | 用默认程序打开
    </template>
  </ul>
  ```
  - 三种模板分支：空白区域 / 文件夹 / 文件
  - `@click.stop` + `@mousedown.stop` 阻止冒泡到全局监听器
  - 菜单项使用 `@click="onMenuCommand('commandName')"` 分发命令
  - 粘贴项条件禁用：`:class="{ 'is-disabled': !clipboard.mode }"` + `@click="clipboard.mode && onMenuCommand('paste')"`

- `FileTreePanel.vue:317-324` — 菜单状态：
  ```javascript
  const contextMenu = reactive({
    visible: false,
    x: 0,
    y: 0,
    data: null,
    isBlankArea: false
  })
  ```

- `FileTreePanel.vue:488-543` — `onNodeContextMenu`：
  ```javascript
  const onNodeContextMenu = (event, data) => {
    event.preventDefault()
    event.stopPropagation()
    emit('contextmenu')  // 通知 Home.vue 关闭另一个面板的菜单
    contextMenu.x = event.clientX
    contextMenu.y = event.clientY
    contextMenu.data = data
    contextMenu.isBlankArea = false
    contextMenu.visible = true
    nextTick(() => { /* 边界调整 */ })
  }
  ```
  - 触发源：`el-tree` 的 `@node-contextmenu` 事件（`FileTreePanel.vue:22`）
  - `nextTick` 中测量菜单 DOM 尺寸，溢出视口时调整坐标（5px margin）

- `FileTreePanel.vue:556-588` — `onBlankAreaContextMenu`：
  - 触发源：`.tree-content` 的 `@contextmenu.prevent`（`FileTreePanel.vue:10`）
  - 构造合成数据：`{ path: dir.path, name: dir.name, type: 'directory' }`
  - `isBlankArea = true` → 仅显示新建文件/文件夹

- `FileTreePanel.vue:596-651` — `onMenuCommand(command)`：
  ```javascript
  const onMenuCommand = (command) => {
    closeContextMenu()
    switch (command) {
      case 'createFile': showCreateAt(contextMenu.data, 'file'); break
      case 'createDir': showCreateAt(contextMenu.data, 'directory'); break
      case 'rename': showRenameAt(contextMenu.data); break
      case 'delete': handleDeleteAt(contextMenu.data); break
      case 'cut': emit('cut', contextMenu.data); break
      case 'copy': emit('copy', contextMenu.data); break
      case 'paste': emit('paste', contextMenu.data); break
      case 'copyTo': showCopyToDialog(contextMenu.data); break
      case 'copyPath': navigator.clipboard.writeText(contextMenu.data.path); break
      case 'copyName': navigator.clipboard.writeText(contextMenu.data.name); break
      case 'openExplorer': OpenInExplorer(contextMenu.data.path); break
      case 'openInVSCode': OpenInVSCode(contextMenu.data.path); break
      case 'openInWarp': OpenInWarp(contextMenu.data.path); break
      case 'openWithDefaultApp': OpenWithDefaultApp(contextMenu.data.path); break
      case 'refresh': refreshNode(contextMenu.data.path); break
      case 'pullRepos': handleBatchPull(contextMenu.data); break
    }
  }
  ```

- `FileTreePanel.vue:546-552` — 关闭机制：
  ```javascript
  const closeContextMenu = () => {
    contextMenu.visible = false
    contextMenu.isBlankArea = false
  }
  const onGlobalClick = () => closeContextMenu()
  ```
  - 全局 `mousedown` 和 `contextmenu` 监听器关闭菜单
  - `@node-expand` 和 `@node-collapse` 也调用 `closeContextMenu`

- `FileTreePanel.vue:876` — defineExpose：
  ```javascript
  defineExpose({ ..., closeMenu: () => { contextMenu.visible = false }, ... })
  ```

- `FileTreePanel.vue:952-1029` — CSS：
  ```css
  .context-menu {
    position: fixed; z-index: 2000;
    background: linear-gradient(135deg, #ffffff, #f8f9fa);
    border-radius: 8px; min-width: 180px;
    animation: fadeIn 0.2s ease-out;
    backdrop-filter: blur(10px);
  }
  .context-menu-item { /* flex, hover 蓝色渐变, 3px 左侧强调条 */ }
  .context-menu-item.is-disabled { /* 灰色, cursor: not-allowed, 无 hover */ }
  .context-menu-divider { /* 1px 渐变分隔线 */ }
  ```

**前端 — DirectoryTree 右键菜单：**

- `DirectoryTree.vue:52-79` — 右键菜单模板：
  - 无条件菜单：重命名、设为默认、---、在资源管理器中打开、用 VSCode 打开、用 Warp 打开、---、删除
  - 每个目录项右键都显示相同的 6 个操作

- `DirectoryTree.vue:160-165` — 菜单状态：
  ```javascript
  const contextMenu = reactive({
    visible: false, x: 0, y: 0, targetDir: null
  })
  ```
  - 使用 `targetDir` 而非 `data`，无 `isBlankArea` 字段

- `DirectoryTree.vue:245-270` — `onMenuCommand(command)`：
  - `rename` → `showRenameDialog(dir)` → `UpdateDirectory` 后端绑定
  - `setDefault` → `SetDefaultDirectory(dir.id)` 后端绑定
  - `openExplorer/openVSCode/openWarp` → 对应后端绑定
  - `delete` → `handleDelete(dir)` → ElMessageBox.confirm + `DeleteDirectory`

- `DirectoryTree.vue:168-174` — defineExpose：
  ```javascript
  defineExpose({ closeMenu: () => { contextMenu.visible = false }, ... })
  ```

**前端 — Home.vue 互斥协调：**

- `Home.vue:102-111`：
  ```javascript
  const onDirectoryContextMenu = () => {
    fileTreePanelRef.value?.closeMenu()
  }
  const onFileTreeContextMenu = () => {
    directoryTreeRef.value?.closeMenu()
  }
  ```
  - 两个子组件都 emit `'contextmenu'`（无载荷）到 Home.vue
  - Home.vue 监听后调用另一个组件的 `closeMenu()`

- `Home.vue:90-95` — 剪贴板状态（传给 FileTreePanel 作为 prop）：
  ```javascript
  const clipboard = reactive({
    mode: null,       // 'copy' 或 'cut'
    sourcePath: '',
    sourceName: '',
    sourceType: ''
  })
  ```

### 数据流

```
右键点击文件树节点:
  el-tree @node-contextmenu → onNodeContextMenu(event, data)
  → event.preventDefault() + event.stopPropagation()
  → emit('contextmenu') → Home.vue 关闭 DirectoryTree 菜单
  → 设置 contextMenu { visible: true, x, y, data, isBlankArea: false }
  → nextTick 边界调整
  → 菜单渲染（根据 data.type 选择模板分支）

右键点击空白区域:
  .tree-content @contextmenu.prevent → onBlankAreaContextMenu(event)
  → 构造合成 { path: dir.path, name: dir.name, type: 'directory' }
  → isBlankArea = true → 仅显示新建文件/文件夹

菜单项点击:
  @click="onMenuCommand('command')" → closeContextMenu() → switch(command)
  → createFile/createDir: showCreateAt(data, type)
  → rename: showRenameAt(data) → 打开重命名对话框
  → delete: handleDeleteAt(data) → ElMessageBox.confirm + DeleteFile
  → cut/copy/paste: emit('cut'/'copy'/'paste', data) → Home.vue 处理
  → copyTo: showCopyToDialog(data)
  → copyPath/copyName: navigator.clipboard.writeText(path/name)
  → openExplorer/openInVSCode/openInWarp: 直接调用后端绑定
  → openWithDefaultApp: OpenWithDefaultApp(path)（仅文件菜单有此项）
  → refresh: refreshNode(data.path)
  → pullRepos: handleBatchPull(data) → emit('batchPull')

菜单关闭:
  点击菜单外区域 → document mousedown/contextmenu 监听 → closeContextMenu()
  展开/收起树节点 → @node-expand/@node-collapse → closeContextMenu()
  另一面板打开菜单 → emit('contextmenu') → Home.vue → closeMenu()

右键点击工作目录项（DirectoryTree）:
  .dir-item @contextmenu → onContextMenu(event, dir)
  → 同样模式（preventDefault → emit → 坐标 → 边界调整）
  → 菜单项：重命名/设为默认/打开操作/删除
  → onMenuCommand 分发到对应操作
```

### 数据契约

```javascript
// FileTreePanel contextMenu 状态
{
  visible: boolean,     // 菜单是否可见
  x: number,           // 左上角 x 坐标（clientX）
  y: number,           // 左上角 y 坐标（clientY）
  data: object | null, // 右键目标节点数据 { path, name, type, ... }
  isBlankArea: boolean // 是否为空白区域右键
}

// DirectoryTree contextMenu 状态
{
  visible: boolean,
  x: number,
  y: number,
  targetDir: object | null  // 右键目标目录数据 { id, name, path, ... }
}

// FileTreePanel → Home.vue 事件
emit('contextmenu')          // 无载荷，通知关闭另一面板菜单
emit('cut', data)            // 剪切操作
emit('copy', data)           // 复制操作
emit('paste', data)          // 粘贴操作
emit('batchPull', data)      // 批量拉取

// DirectoryTree → Home.vue 事件
emit('contextmenu')          // 无载荷，通知关闭另一面板菜单
emit('change')               // 数据变更（重命名/删除/设为默认后）
```

### 架构约束

- **右键菜单实现方式**：自定义 `<ul>` + `position: fixed` + `z-index >= 2000`，**禁止使用 el-dropdown**（不支持 contextmenu 事件）
- **DOM 位置**：菜单 DOM 留在组件内部，保证 `scoped` 样式生效
- **禁止**：引入新依赖、TypeScript、CSS 框架
- **事件冒泡控制**：菜单 `<ul>` 上 `@click.stop` + `@mousedown.stop` 阻止全局监听器关闭菜单
- **全局监听器生命周期**：`onMounted` 注册 `document.addEventListener`，`onBeforeUnmount` 中必须 `removeEventListener`
- **边界调整**：`nextTick` 后通过 `document.querySelector('.context-menu')` 获取实际尺寸，溢出视口时调整坐标
- **粘贴条件禁用**：`clipboard.mode` 为 null 时粘贴项灰色 + 不可点击
- **Home.vue 剪贴板**：`clipboard` reactive 对象通过 props 传给 FileTreePanel，不含 isDisabled 判断
- **`navigator.clipboard.writeText`**：复制路径/文件名使用浏览器原生 API，不走后端绑定
- **`defineExpose`**：`closeMenu` 方法暴露给 Home.vue 用于互斥关闭

### 前两个 Story 的经验教训

**Story 3-2（重命名和删除）：**
1. **handleDeleteAt 未暴露**：通过 `onMenuCommand('delete')` 间接调用，或直接调用
2. **ElMessageBox.confirm mock**：`mockResolvedValueOnce('confirm')` / `mockRejectedValueOnce('cancel')` 控制确认/取消
3. **mockClear 优于 restoreAllMocks**：避免破坏 setup.js 全局 mock
4. **findComponent $attrs**：通过 `wrapper.findComponent('.el-tree').vm.$attrs` 访问事件处理器

**Story 3-3（文件预览）：**
1. **ElMessage mock 独立配置**：ContentPanel.spec.js 需独立 mock ElMessage
2. **el-input 存根优化**：支持 `type="textarea"` 渲染文本区域
3. **previewFile 未暴露**：需通过 UI 交互触发（找到"预览"按钮并点击）
4. **clearPreview 已暴露**：可通过 `wrapper.vm.clearPreview()` 直接调用

**更早期的经验教训：**
1. **mustMkdir/mustWriteFile helpers**：已定义在 `service/filetree_test.go`
2. **el-tree stub 无 slot**：`template: '<div class="el-tree"></div>'`
3. **setup.js mock 必须同步更新**：新增 Go 绑定方法后，必须在 setup.js 添加对应 mock
4. **vi.importMock 路径**：使用 `'../../../wailsjs/go/main/App'`（三级 `../`）

### 测试注意事项

**前端测试（FileTreePanel.spec.js 扩展）：**

- 右键菜单显示测试：模拟 `el-tree` 的 `@node-contextmenu` 事件
  - 触发方式：`wrapper.findComponent('.el-tree').vm.$attrs.onNodeContextMenu(event, data)`
  - 构造 event 对象：`{ clientX: 100, clientY: 200, preventDefault: vi.fn(), stopPropagation: vi.fn() }`
  - 验证：`wrapper.find('.context-menu').exists()` 为 true
  - 验证菜单项文本内容匹配节点类型

- 空白区域右键测试：模拟 `.tree-content` 的 `@contextmenu` 事件
  - 注意：空白区域使用 `@contextmenu.prevent`，需要通过 wrapper 触发

- 菜单关闭测试：调用全局 mousedown 监听器
  - 直接模拟 document 事件比较困难，可调用 `wrapper.vm.closeContextMenu()` 或 `wrapper.vm.closeMenu()`

- 粘贴禁用测试：传入 `clipboard: { mode: null }` 的 props
  - 验证 `.is-disabled` class 存在

- **navigator.clipboard.writeText mock**：需在测试中 mock
  ```javascript
  Object.assign(navigator, {
    clipboard: { writeText: vi.fn(() => Promise.resolve()) }
  })
  ```

- `onMenuCommand` 未通过 defineExpose 暴露，需通过菜单项点击触发
  - 方式：找到对应菜单项 `<li>` 并 `trigger('click')`

**前端测试（DirectoryTree.spec.js 扩展）：**

- DirectoryTree 右键菜单测试：模拟 `.dir-item` 的 `@contextmenu` 事件
  - DirectoryTree 的存根中需要 `.dir-item` 可点击

**前端测试（Home.spec.js 扩展）：**

- 互斥测试：模拟 FileTreePanel emit `'contextmenu'` → 验证 DirectoryTree 的 `closeMenu` 被调用
  - Home.spec.js 中 DirectoryTree 和 FileTreePanel 的存根需要支持事件触发
  - 当前的简单 `<div>` 存根无法 emit 事件，需增强存根设计

### 关键验证点

1. **节点类型分发准确性**：文件/文件夹/空白区域三种菜单项列表各不相同，无遗漏无多余
2. **粘贴条件禁用**：`clipboard.mode` 为 null 时粘贴项灰色不可点击，有值时可正常点击
3. **菜单互斥**：两面板菜单同时只能显示一个，打开一个时另一个自动关闭
4. **边界调整**：菜单靠近视口边缘时不溢出，自动调整到可视区域内
5. **全局监听器清理**：unmount 时 `mousedown` 和 `contextmenu` 监听器被移除，无内存泄漏
6. **`@click.stop` + `@mousedown.stop`**：菜单内部点击/鼠标按下不触发全局关闭
7. **展开/收起节点关闭菜单**：`@node-expand` 和 `@node-collapse` 调用 `closeContextMenu`
8. **`navigator.clipboard.writeText`**：复制路径/文件名使用浏览器原生 API
9. **DirectoryTree 菜单无 `isBlankArea`**：只有目录项右键，无空白区域菜单
10. **用默认程序打开仅文件菜单**：文件夹菜单无此项

### 已知问题（不在本 Story 范围）

1. **两面板菜单代码重复**：FileTreePanel 和 DirectoryTree 各自独立实现完整右键菜单，show/hide/定位/CSS 高度重复
2. **`document.querySelector('.context-menu')` 全局选择风险**：两面板都使用同一选择器，理论上 v-if 互斥应避免冲突
3. **DirectoryTree.spec.js 缺少 contextmenu 监听器清理测试**：当前只验证 click 监听器移除
4. **命令分发使用字符串 switch**：无类型安全，拼写错误不会在编译期发现
5. **后端无新增方法**：所有右键菜单操作对应的后端绑定均已实现

### References

- [Source: frontend/src/components/FileTreePanel.vue:155-259] — 右键菜单模板（三种分支）
- [Source: frontend/src/components/FileTreePanel.vue:317-324] — contextMenu 状态
- [Source: frontend/src/components/FileTreePanel.vue:488-543] — onNodeContextMenu
- [Source: frontend/src/components/FileTreePanel.vue:556-588] — onBlankAreaContextMenu
- [Source: frontend/src/components/FileTreePanel.vue:596-651] — onMenuCommand 命令分发
- [Source: frontend/src/components/FileTreePanel.vue:546-552] — closeContextMenu + onGlobalClick
- [Source: frontend/src/components/FileTreePanel.vue:591-593] — onGlobalContextMenu
- [Source: frontend/src/components/FileTreePanel.vue:876] — defineExpose（含 closeMenu）
- [Source: frontend/src/components/FileTreePanel.vue:880-888] — 全局监听器生命周期
- [Source: frontend/src/components/FileTreePanel.vue:952-1029] — 右键菜单 CSS
- [Source: frontend/src/components/DirectoryTree.vue:52-79] — 右键菜单模板
- [Source: frontend/src/components/DirectoryTree.vue:160-165] — contextMenu 状态
- [Source: frontend/src/components/DirectoryTree.vue:176-231] — onContextMenu
- [Source: frontend/src/components/DirectoryTree.vue:245-270] — onMenuCommand 命令分发
- [Source: frontend/src/components/DirectoryTree.vue:168-174] — defineExpose（含 closeMenu）
- [Source: frontend/src/components/DirectoryTree.vue:477-485] — 全局监听器生命周期
- [Source: frontend/src/views/Home.vue:102-111] — 互斥协调逻辑
- [Source: frontend/src/views/Home.vue:90-95] — 剪贴板状态
- [Source: frontend/src/components/__tests__/FileTreePanel.spec.js] — 现有前端测试
- [Source: frontend/src/components/__tests__/DirectoryTree.spec.js] — 现有前端测试
- [Source: frontend/src/views/__tests__/Home.spec.js] — 现有前端测试
- [Source: frontend/src/test/setup.js] — 全局 mock 配置
- [Source: _bmad-output/implementation-artifacts/3-2-rename-delete.md] — 前一个 Story
- [Source: _bmad-output/implementation-artifacts/3-3-file-preview.md] — 前一个 Story

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
