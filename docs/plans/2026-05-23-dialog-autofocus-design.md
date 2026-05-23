# 弹窗自动聚焦设计文档

**日期：** 2026-05-23
**状态：** 已确认

## 摘要

为所有含输入框/选择器的弹窗添加自动聚焦功能，提升键盘操作体验。参照已有的重命名弹窗实现模式（ref + nextTick + focus），统一应用到其余弹窗。

## 背景

当前项目共有 6 个含输入组件的弹窗，其中重命名文件/文件夹和重命名工作目录已实现自动聚焦，其余 4 个弹窗缺少此功能。切换分支弹窗使用 el-select，同样需要自动聚焦。

## 涉及弹窗

| 弹窗 | 所在文件 | 输入组件 | 当前状态 |
|------|----------|----------|----------|
| 新建文件/文件夹 | FileTreePanel.vue | el-input | 缺少 autofocus |
| 拷贝到 | FileTreePanel.vue | el-input | 缺少 autofocus |
| 添加工作目录 | DirectoryTree.vue | el-input | 缺少 autofocus |
| 克隆仓库 | ContentPanel.vue | el-input | 缺少 autofocus |
| 切换分支 | ContentPanel.vue | el-select | 缺少 autofocus |
| 重命名文件/文件夹 | FileTreePanel.vue | el-input | 已有 autofocus |
| 重命名工作目录 | DirectoryTree.vue | el-input | 已有 autofocus |

## 实现方案

**方案选择：** 独立添加（方案 A），参照现有重命名弹窗模式。

**实现模式：**

```vue
<!-- 模板 -->
<el-input ref="xxxInputRef" ... />

<!-- 脚本 -->
<script setup>
const xxxInputRef = ref(null)

watch(xxxDialogVisible, (val) => {
  if (val) {
    nextTick(() => {
      xxxInputRef.value?.focus()
    })
  }
})
</script>
```

### 各弹窗改动明细

**FileTreePanel.vue：**

- 新建文件/文件夹弹窗：el-input 加 `ref="createInputRef"`，watch `createDialogVisible`
- 拷贝到弹窗：目标地址 el-input 加 `ref="copyToInputRef"`，watch `copyToDialogVisible`

**DirectoryTree.vue：**

- 添加工作目录弹窗：目录名称 el-input 加 `ref="addNameInputRef"`，watch `addDialogVisible`

**ContentPanel.vue：**

- 克隆仓库弹窗：el-input 加 `ref="cloneInputRef"`，watch `cloneDialogVisible`
- 切换分支弹窗：el-select 加 `ref="branchSelectRef"`，watch `branchDialogVisible`

## 不做的事

- 不修改已有 autofocus 的重命名弹窗
- 不封装自定义指令或公共组件
- 不添加自动选中文本逻辑
