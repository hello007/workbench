# F5 刷新选中节点 — 设计文档

**日期**：2026-05-24
**状态**：已确认

## 需求

在界面上按 F5 时，刷新当前选中节点所在的目录（等同于右键菜单"刷新"）。

## 设计

在 `Home.vue` 已有的 `handleGlobalKeydown` 函数顶部增加 F5 判断，`e.preventDefault()` 阻止浏览器默认刷新，调用 `fileTreePanelRef.value?.refreshNode(selectedNode.value.path)` 刷新选中节点。

## 改动范围

仅 `frontend/src/views/Home.vue` — `handleGlobalKeydown` 函数（约第 201 行）
