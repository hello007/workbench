# 工作目录路径显示与面板拖拽调整宽度

**日期：** 2026-05-19
**状态：** 已确认

## 需求概述

1. 左侧工作目录列表中，每个目录项名称下方显示完整路径
2. 工作目录面板和文件树面板支持拖拽调整宽度
3. 使用 worktree 开发

## 功能一：路径显示

**修改文件：** `frontend/src/components/DirectoryTree.vue`

**模板变更：** 在目录项名称下方增加路径行

```html
<div class="dir-item">
  <div class="dir-info">
    <div class="dir-row">
      <el-icon><Folder /></el-icon>
      <span class="dir-name">{{ dir.name }}</span>
      <el-icon v-if="dir.isDefault"><Star /></el-icon>
    </div>
    <div class="dir-path" :title="dir.path">{{ dir.path }}</div>
  </div>
</div>
```

**样式：**

- `.dir-path`：`font-size: 11px; color: #909399; line-height: 1.2; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-top: 2px;`
- hover 时 `title` 属性显示完整路径
- `.dir-info` 纵向排列，`.dir-row` 保持原有图标+名称+星标的横向排列

## 功能二：面板拖拽调整宽度

**新增依赖：** `splitpanes`（Vue 3 兼容版本）

**修改文件：** `frontend/src/views/Home.vue`

**布局变更：** 将 `el-container` 三栏固定宽度改为 splitpanes 布局

```html
<Splitpanes class="default-theme">
  <Pane :size="15" :min-size="10" :max-size="30">
    <DirectoryTree ... />
  </Pane>
  <Pane :size="22" :min-size="15" :max-size="35">
    <FileTreePanel ... />
  </Pane>
  <Pane :size="63" :min-size="30">
    <ContentPanel ... />
  </Pane>
</Splitpanes>
```

**关键点：**

- 三个 Pane 对应原来的三栏，初始比例 15%/22%/63%
- min-size/max-size 限制拖拽范围（百分比）
- 实时拖拽，跟随鼠标

**样式覆盖：**

- 分隔条背景色 `#e6e6e6`，hover 时 `#c0c4cc`
- 面板背景色保持 `#f5f7fa`
- 移除原有固定 `width="200px"` / `width="280px"` 及 `border-right` 样式

## 技术选型

| 项目 | 选择 | 理由 |
|------|------|------|
| 拖拽方案 | splitpanes 库 | 用户选择，功能完善、键盘可访问 |
| 路径样式 | 小字灰色 | 与目录名区分，不喧宾夺主 |
| 拖拽模式 | 实时生效 | 体验更流畅 |
