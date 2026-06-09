# 拷贝到对话框路径互换 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在"拷贝到"对话框的"原地址"和"目标地址"输入框之间添加互换按钮，点击后交换两个字段的值。

**Architecture:** 纯前端 UI 增强。在两个 Vue 组件（FileTreePanel.vue、ToolboxPanel.vue）的 `<el-form>` 中插入一行居中的互换按钮，通过一个 swap 函数交换两个 ref 的值，配合 CSS 动画提供视觉反馈。

**Tech Stack:** Vue 3 Composition API、Element Plus、@element-plus/icons-vue

---

## 文件变更清单

| 文件 | 操作 | 职责 |
|------|------|------|
| `frontend/src/components/FileTreePanel.vue` | 修改 | 添加互换按钮（模板 + 脚本 + 样式） |
| `frontend/src/components/ToolboxPanel.vue` | 修改 | 同上，保持体验一致 |

---

### Task 1: FileTreePanel.vue — 添加互换按钮

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue:117-142`（模板：el-form 区域）
- Modify: `frontend/src/components/FileTreePanel.vue:290-292`（脚本：import 区域）
- Modify: `frontend/src/components/FileTreePanel.vue`（样式：scoped CSS 区域）

- [ ] **Step 1: 添加 Sort 图标 import**

在文件脚本区找到 `import { ElMessage, ElMessageBox } from 'element-plus'`（约 292 行），在其后添加图标 import：

```javascript
import { Sort } from '@element-plus/icons-vue'
```

- [ ] **Step 2: 添加 swapCopyToPaths 函数**

在拷贝到对话框状态声明区（`copyToPreview` computed 之后，约 416 行之后），添加互换函数：

```javascript
// 互换原地址与目标地址
const swapCopyToPaths = () => {
  const temp = copyToSourcePath.value
  copyToSourcePath.value = copyToTargetPath.value
  copyToTargetPath.value = temp
}
```

- [ ] **Step 3: 在模板中插入互换按钮行**

在"原地址" `el-form-item`（约 line 118-124）和"目标地址" `el-form-item`（约 line 125-133）之间，插入互换按钮行。定位 `</el-form-item>` 结束标签后、`<el-form-item label="目标地址">` 之前：

```vue
        <el-form-item label="原地址">
          <el-input
            v-model="copyToSourcePath"
            placeholder="请输入原文件或文件夹路径"
            :disabled="copyToLoading"
          />
        </el-form-item>
        <div class="swap-row">
          <el-button
            text
            size="small"
            :disabled="copyToLoading"
            @click="swapCopyToPaths"
          >
            <el-icon class="swap-icon"><Sort /></el-icon>
            互换
          </el-button>
        </div>
        <el-form-item label="目标地址">
```

- [ ] **Step 4: 添加互换按钮样式**

在 `<style scoped>` 区域末尾（拷贝预览样式之后），添加：

```css
/* 互换按钮行 */
.swap-row {
  display: flex;
  justify-content: center;
  margin: -8px 0 0;
}

.swap-row .el-button {
  color: var(--text-tertiary, #909399);
  font-size: 12px;
}

.swap-row .el-button:hover {
  color: var(--primary-color, #409eff);
}

.swap-row .el-button:hover .swap-icon {
  transform: rotate(180deg);
}

.swap-icon {
  transition: transform 0.3s ease;
}
```

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat: 拷贝到对话框添加路径互换按钮 (FileTreePanel)"
```

---

### Task 2: ToolboxPanel.vue — 添加互换按钮

**Files:**
- Modify: `frontend/src/components/ToolboxPanel.vue:29-51`（模板：el-form 区域）
- Modify: `frontend/src/components/ToolboxPanel.vue:69`（脚本：import 区域）
- Modify: `frontend/src/components/ToolboxPanel.vue`（样式：scoped CSS 区域）

- [ ] **Step 1: 添加 Sort 图标 import**

修改现有图标 import 行（约 69 行）：

```javascript
// 之前：
import { CopyDocument, SetUp } from '@element-plus/icons-vue'
// 之后：
import { CopyDocument, SetUp, Sort } from '@element-plus/icons-vue'
```

- [ ] **Step 2: 添加 swapCopyToPaths 函数**

在 `copyToPreview` computed（约 line 94-104）之后，`handleCopyTo` 函数之前，添加：

```javascript
// 互换原地址与目标地址
const swapCopyToPaths = () => {
  const temp = copyToSourcePath.value
  copyToSourcePath.value = copyToTargetPath.value
  copyToTargetPath.value = temp
}
```

- [ ] **Step 3: 在模板中插入互换按钮行**

在"原地址" `el-form-item` 和"目标地址" `el-form-item` 之间插入。定位 `</el-form-item>` 后、`<el-form-item label="目标地址">` 前：

```vue
        <el-form-item label="原地址">
          <el-input
            v-model="copyToSourcePath"
            placeholder="请输入原文件或文件夹路径"
            :disabled="copyToLoading"
          />
        </el-form-item>
        <div class="swap-row">
          <el-button
            text
            size="small"
            :disabled="copyToLoading"
            @click="swapCopyToPaths"
          >
            <el-icon class="swap-icon"><Sort /></el-icon>
            互换
          </el-button>
        </div>
        <el-form-item label="目标地址">
```

- [ ] **Step 4: 添加互换按钮样式**

在 `<style scoped>` 区域末尾添加（与 Task 1 相同样式）：

```css
/* 互换按钮行 */
.swap-row {
  display: flex;
  justify-content: center;
  margin: -8px 0 0;
}

.swap-row .el-button {
  color: var(--text-tertiary, #909399);
  font-size: 12px;
}

.swap-row .el-button:hover {
  color: var(--primary-color, #409eff);
}

.swap-row .el-button:hover .swap-icon {
  transform: rotate(180deg);
}

.swap-icon {
  transition: transform 0.3s ease;
}
```

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/ToolboxPanel.vue
git commit -m "feat: 拷贝到对话框添加路径互换按钮 (ToolboxPanel)"
```

---

## 自审清单

| 检查项 | 结果 |
|--------|------|
| Spec 覆盖：互换按钮位置、图标、样式、禁用态、动画 | ✅ 全部覆盖 |
| 占位符扫描：无 TBD/TODO/"类似 Task N" | ✅ 全部为完整代码 |
| 类型一致性：`copyToSourcePath`、`copyToTargetPath` ref 变量名两文件一致 | ✅ 一致 |
| 后端无改动 | ✅ 纯前端 |
